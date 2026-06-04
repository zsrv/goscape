# S6p — InvListener Keyed-Map Refactor + OpLocU/OpNpcU Item Validation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Refactor `Player.invListeners` from a slice to a `Com`-keyed map, add `invListenOnCom` / `invStopListenOnCom` registration methods, and wire item-in-slot validation into `handleOpLocU` + `handleOpNpcU` (closing S6m-D3 + S6o-D3).

**Architecture:** Three tasks, each shippable on its own. Task 1 is a pure behavior-preserving data-structure refactor (slice → map). Task 2 adds the runtime registration API. Task 3 consumes Tasks 1+2 by inserting a listener-lookup gate (`p.invListeners[useCom]` → `resolveListenerInv` → `inv.HasAt`) into the two OP*U handlers, mirroring TS `OpLocUHandler.ts:50-66` / `OpNpcUHandler.ts:35-50`.

**Tech Stack:** Go 1.26 (`clear(map)` builtin available), standard `testing` package, no external deps.

**Spec:** `docs/superpowers/specs/2026-04-21-runescript-s6p-invlistener-map-item-validation-design.md` (commit `155f27a`).

**TS-faithfulness gate active:** User requires "true to TS." S6p introduces **zero** new deviations. Two closed: S6m-D3, S6o-D3.

---

## File Structure

### Production files

| File | Lines (current) | Changes | Task |
|---|---|---|---|
| `modules/world/player.go` | `179` | Field type slice → `map[int]InventoryListener` | 1 |
| `modules/world/player.go` | `227-230` | Modal-close iteration: `for _, l := range` + `clear()` | 1 |
| `modules/world/player.go` | `415-443` | `updateInvs` iteration: read-modify-write pattern (FirstSeen flips) | 1 |
| `modules/world/player.go` | end | Add `invListenOnCom` + `invStopListenOnCom` methods | 2 |
| `modules/world/handler_opnpc.go` | top | Add `resolveListenerInv` helper | 3 |
| `modules/world/handler_oploc.go` | `183-202`, `234`, `260-269` | Listener lookup + validation; collapse S6m-D3 comment | 3 |
| `modules/world/handler_opnpc.go` | `128-150`, `170`, `186-191` | Same for NPC; collapse S6o-D3 comment | 3 |

### Test files

| File | Action | Purpose | Task |
|---|---|---|---|
| `modules/world/inv_update_test.go` | Modify | 4 fixture migrations (lines 32, 43, 57, 89, 108) | 1 |
| `modules/world/modal_close_test.go` | Modify | 2 fixture migrations (lines 12, 38) | 1 |
| `modules/world/player_inv_test.go` | Create | 6 listener-lifecycle tests | 2 |
| `modules/world/handler_oploc_test.go` | Modify | `TestHandleOpLocUSetsInteraction` gains listener+inv setup; add 4 validation tests | 3 |
| `modules/world/handler_opnpc_test.go` | Modify | `TestHandleOpNpcUSetsInteraction` gains listener+inv setup; add 4 validation tests | 3 |

### What's out of scope

- `ActivePlayer` interface extensions for script opcodes (YAGNI — no script consumer yet)
- UI-modal-open call sites registering listeners (no UI-modal-open handlers in goscape yet)
- Component registry (S6m-D1/D2, S6o-D1/D2 — different track)
- Members-config (S6m-D4, S6o-D4 — different track)

---

## Task 1: `invListeners` slice → map refactor

**Goal:** Change the data type, update the 2 iteration sites, migrate 6 test fixtures. Build + all existing tests green at end.

**Files:**
- Modify: `modules/world/player.go:179`
- Modify: `modules/world/player.go:227-230` (modal-close iteration)
- Modify: `modules/world/player.go:415-448` (`updateInvs`)
- Modify: `modules/world/inv_update_test.go` (4 fixture sites + 1 indexed access)
- Modify: `modules/world/modal_close_test.go` (2 fixture sites)

### TDD context

This is a refactor, not a feature add. The existing tests in `inv_update_test.go` and `modal_close_test.go` ARE the test coverage. Failing→green cycle: migrate field type + iterations → ALL existing tests fail to compile → migrate test fixtures → build green → tests pass.

- [ ] **Step 1: Run the existing test suite to capture the green baseline.**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...`
Expected: PASS

Record the count of passing tests so the same count is green at Step 8.

- [ ] **Step 2: Change the field type.**

In `modules/world/player.go:179`, replace:

```go
invListeners []InventoryListener
```

With:

```go
// invListeners maps UI component ID (Com) to an InventoryListener.
// Registered via invListenOnCom (S6p); unregistered via
// invStopListenOnCom or cleared on modal close. Keyed structure
// enables O(1) lookup in handleOpLocU / handleOpNpcU's item-match
// validation (S6p closure of S6m-D3 / S6o-D3). Nil until first
// listener registers; safe to read, range, len-check while nil.
invListeners map[int]InventoryListener
```

- [ ] **Step 3: Update the modal-close iteration site.**

In `modules/world/player.go:227-230`, replace:

```go
for _, l := range p.invListeners {
    sendUpdateInvStopTransmit(p, l.Com)
}
p.invListeners = p.invListeners[:0]
```

With:

```go
for _, l := range p.invListeners {
    sendUpdateInvStopTransmit(p, l.Com)
}
clear(p.invListeners) // Go 1.21+ map reset; keeps allocated buckets
```

Note: range iteration over a map gives undefined order. The original slice had insertion order. TS does not depend on iteration order for `UpdateInvStopTransmit` emission (each listener gets its own packet; order is irrelevant to client state). Safe to switch.

- [ ] **Step 4: Update `updateInvs` with read-modify-write.**

The existing loop at `modules/world/player.go:415-448` mutates `l.FirstSeen = false` through a slice pointer. Map values aren't addressable, so the pattern becomes read-modify-write. Replace the whole function body:

```go
func (p *Player) updateInvs() {
	if p.client == nil || p.client.server == nil {
		return
	}
	// Collect all observed invs so we can clear Update after all listeners fire.
	observed := make([]*inventory.Inventory, 0, len(p.invListeners))
	for com, l := range p.invListeners {
		var inv *inventory.Inventory
		if l.Source == -1 {
			inv = p.client.server.invs[l.Type]
		} else {
			other := p.client.server.players[l.Source]
			if other == nil {
				continue
			}
			inv = other.invs[l.Type]
		}
		if inv == nil {
			continue
		}

		if inv.Update || l.FirstSeen {
			sendUpdateInvFullCom(p, l.Com, inv)
			if l.FirstSeen {
				// Flip via read-modify-write — map values are not addressable.
				l.FirstSeen = false
				p.invListeners[com] = l
			}
		}
		observed = append(observed, inv)
	}
	// Clear inv.Update AFTER all listeners (multiple listeners can share an inv).
	for _, inv := range observed {
		inv.Update = false
	}
}
```

Key changes from original:
- Loop variable `i` → `com, l`
- `l := &p.invListeners[i]` pointer → plain value copy `l`
- Mutation `l.FirstSeen = false` → guarded by `if l.FirstSeen { ...; p.invListeners[com] = l }` (only write back when we actually changed something, avoids unnecessary map writes every tick)

- [ ] **Step 5: Run build to find test breakage.**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./modules/world/...`
Expected: FAIL with compile errors at the 6 fixture sites (slice-literal assignments to a map-typed field) and at `inv_update_test.go:43` (indexed access `[0]`).

- [ ] **Step 6: Migrate `modules/world/inv_update_test.go` fixtures.**

Replace each of the 4 slice-literal assignments with a keyed-map literal, and replace the one indexed access at line 43:

Line 32 (`TestUpdateInvsFirstSeenFires`):

```go
viewer.invListeners = map[int]InventoryListener{
    149: {Type: 93, Com: 149, Source: 2, FirstSeen: true},
}
```

Line 43 (same test, assertion):

```go
if viewer.invListeners[149].FirstSeen {
    t.Error("FirstSeen should flip to false after first send")
}
```

Line 57 (`TestUpdateInvsRespectsDirty`):

```go
viewer.invListeners = map[int]InventoryListener{
    149: {Type: 93, Com: 149, Source: 2, FirstSeen: false},
}
```

Line 89 (`TestUpdateInvsWorldSource`):

```go
viewer.invListeners = map[int]InventoryListener{
    200: {Type: 0, Com: 200, Source: -1, FirstSeen: true},
}
```

Line 108 (`TestUpdateInvsSkipsMissingSource`):

```go
viewer.invListeners = map[int]InventoryListener{
    149: {Type: 93, Com: 149, Source: 99, FirstSeen: true},
}
```

- [ ] **Step 7: Migrate `modules/world/modal_close_test.go` fixtures.**

Line 12 (`TestModalCloseEmitsStopTransmit`):

```go
p.invListeners = map[int]InventoryListener{
    149: {Type: 93, Com: 149, Source: 2, FirstSeen: false},
    150: {Type: 93, Com: 150, Source: -1, FirstSeen: false},
}
```

Line 38 (`TestNoStopTransmitWithoutModalClose`):

```go
p.invListeners = map[int]InventoryListener{
    149: {Type: 93, Com: 149, Source: 2, FirstSeen: false},
}
```

The existing `len(p.invListeners) != 0` and `len(p.invListeners) != 1` assertions work unchanged on maps (`len` is defined for both).

- [ ] **Step 8: Run the full test suite.**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...`
Expected: PASS — same test count green as Step 1's baseline.

- [ ] **Step 9: Run `go vet` to catch any pointer-address holdovers.**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./modules/world/...`
Expected: no diagnostics.

- [ ] **Step 10: Commit.**

```bash
git add modules/world/player.go modules/world/inv_update_test.go modules/world/modal_close_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
refactor(world): invListeners slice → Com-keyed map (S6p-1)

Change Player.invListeners from []InventoryListener to
map[int]InventoryListener, keyed by UI component ID. Enables O(1)
lookup for the item-in-slot validation gate landing in S6p-3.

Modal-close iteration now uses clear(p.invListeners) instead of
slicing to zero length. updateInvs iteration switches from indexed-
pointer to read-modify-write — map values aren't addressable, so
the FirstSeen flip writes the updated value back under the same key.

Behavior-preserving: 4 inv_update tests + 2 modal_close tests
migrated from slice-literal to map-literal fixtures. All remain green.

Part of S6p; prerequisite for S6m-D3 / S6o-D3 closure in S6p-3.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: `invListenOnCom` + `invStopListenOnCom` registration methods

**Goal:** Add two methods to `*Player` that lazy-init the map and add/remove listeners by component ID. Matches TS `Player.ts:1441-1471`. 6 new lifecycle tests cover the API surface.

**Files:**
- Modify: `modules/world/player.go` (append methods)
- Create: `modules/world/player_inv_test.go`

### TDD context

Classic red-green cycle: write 6 failing tests → implement → green.

- [ ] **Step 1: Write 6 failing tests in a new file.**

Create `modules/world/player_inv_test.go`:

```go
package world

import (
	"testing"
)

// TestInvListenOnComRegistersNewListener verifies a fresh call on a
// Player with no prior listeners creates a map entry with the expected
// Type, Com, Source, and FirstSeen=true.
func TestInvListenOnComRegistersNewListener(t *testing.T) {
	p, _ := newTestPlayer(t)

	p.invListenOnCom(93, 149, -1)

	if len(p.invListeners) != 1 {
		t.Fatalf("len: got %d, want 1", len(p.invListeners))
	}
	got, ok := p.invListeners[149]
	if !ok {
		t.Fatal("listener at com=149 should exist")
	}
	if got.Type != 93 {
		t.Errorf("Type: got %d, want 93", got.Type)
	}
	if got.Com != 149 {
		t.Errorf("Com: got %d, want 149", got.Com)
	}
	if got.Source != -1 {
		t.Errorf("Source: got %d, want -1", got.Source)
	}
	if !got.FirstSeen {
		t.Error("FirstSeen should be true for new listener")
	}
}

// TestInvListenOnComReplacesExisting verifies that a second call with
// the same com overwrites the first entry and resets FirstSeen to true,
// matching TS Player.ts:1441-1462 add-or-replace semantics.
func TestInvListenOnComReplacesExisting(t *testing.T) {
	p, _ := newTestPlayer(t)

	p.invListenOnCom(93, 149, -1)
	// Simulate a first-seen emit flipping FirstSeen to false.
	l := p.invListeners[149]
	l.FirstSeen = false
	p.invListeners[149] = l

	// Re-register with a different Type/Source at the same com.
	p.invListenOnCom(100, 149, 2)

	if len(p.invListeners) != 1 {
		t.Fatalf("len: got %d, want 1 (re-register should not add a second entry)", len(p.invListeners))
	}
	got := p.invListeners[149]
	if got.Type != 100 {
		t.Errorf("Type: got %d, want 100 (replace should overwrite)", got.Type)
	}
	if got.Source != 2 {
		t.Errorf("Source: got %d, want 2 (replace should overwrite)", got.Source)
	}
	if !got.FirstSeen {
		t.Error("FirstSeen should reset to true on replace")
	}
}

// TestInvListenOnComLazyInitializesMap verifies that the first call on
// a Player whose invListeners field is nil allocates the map.
func TestInvListenOnComLazyInitializesMap(t *testing.T) {
	p, _ := newTestPlayer(t)
	if p.invListeners != nil {
		t.Fatalf("precondition: invListeners should start nil, got %v", p.invListeners)
	}

	p.invListenOnCom(93, 149, -1)

	if p.invListeners == nil {
		t.Fatal("invListenOnCom should allocate the map on first call")
	}
}

// TestInvStopListenOnComRemovesListener verifies that calling stop on
// a registered com deletes the entry and decreases len by 1.
func TestInvStopListenOnComRemovesListener(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.invListenOnCom(93, 149, -1)
	p.invListenOnCom(100, 200, -1)
	if len(p.invListeners) != 2 {
		t.Fatalf("setup: len should be 2, got %d", len(p.invListeners))
	}

	p.invStopListenOnCom(149)

	if len(p.invListeners) != 1 {
		t.Errorf("len: got %d, want 1", len(p.invListeners))
	}
	if _, ok := p.invListeners[149]; ok {
		t.Error("listener at 149 should be removed")
	}
	if _, ok := p.invListeners[200]; !ok {
		t.Error("listener at 200 should remain")
	}
}

// TestInvStopListenOnComNoopForMissingKey verifies calling stop on a
// com that was never registered is a no-op (does not panic, does not
// mutate map).
func TestInvStopListenOnComNoopForMissingKey(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.invListenOnCom(93, 149, -1)

	p.invStopListenOnCom(999) // never registered

	if len(p.invListeners) != 1 {
		t.Errorf("len: got %d, want 1 (unrelated listener should remain)", len(p.invListeners))
	}
}

// TestInvStopListenOnComNoopForNilMap verifies calling stop on a Player
// whose map is still nil does not panic (Go's delete-on-nil semantic).
func TestInvStopListenOnComNoopForNilMap(t *testing.T) {
	p, _ := newTestPlayer(t)
	if p.invListeners != nil {
		t.Fatalf("precondition: invListeners should start nil")
	}

	p.invStopListenOnCom(149) // must not panic

	if p.invListeners != nil {
		t.Error("stop on nil map should not cause an allocation")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail.**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestInvListenOnCom -v`
Expected: FAIL with "p.invListenOnCom undefined" compile error. Same for `TestInvStopListenOnCom`.

- [ ] **Step 3: Implement both methods.**

Append to `modules/world/player.go` (end of file, after the last existing method — find the last `func (p *Player)` and add after it):

```go
// invListenOnCom registers an inventory listener at the given interface
// component ID. If a listener already exists at com, it's replaced and
// FirstSeen resets to true (matches TS Player.ts:1441-1462 add-or-
// replace semantics).
//
// Source = -1 → world-shared inventory (Server.invs[Type]).
// Source >= 0 → another player's slot (Server.players[Source].invs[Type]).
//
// Lazy-initializes the invListeners map on first call.
func (p *Player) invListenOnCom(invType, com, source int) {
	if p.invListeners == nil {
		p.invListeners = make(map[int]InventoryListener)
	}
	p.invListeners[com] = InventoryListener{
		Type:      invType,
		Com:       com,
		Source:    source,
		FirstSeen: true,
	}
}

// invStopListenOnCom unregisters the listener at the given component
// ID. No-op if no listener exists there, including when the map itself
// is nil (Go's delete-on-nil is safe). Matches TS Player.ts:1464-1471.
func (p *Player) invStopListenOnCom(com int) {
	delete(p.invListeners, com)
}
```

- [ ] **Step 4: Run tests to verify they pass.**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestInvListenOnCom|TestInvStopListenOnCom' -v`
Expected: PASS × 6

- [ ] **Step 5: Run the full world test suite to catch regressions.**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...`
Expected: PASS (previous count + 6).

- [ ] **Step 6: Commit.**

```bash
git add modules/world/player.go modules/world/player_inv_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): invListenOnCom / invStopListenOnCom runtime registration (S6p-2)

Add two methods on *Player mirroring TS Player.ts:1441-1471:
- invListenOnCom(invType, com, source) — register or replace a
  listener at the given UI component ID; FirstSeen resets to true on
  replace; lazy-initializes the map on first call.
- invStopListenOnCom(com) — remove the listener at com; safe on a
  nil map (Go's delete-on-nil).

6 lifecycle tests cover: register-new, replace-existing, lazy-init,
remove-present, remove-missing-key, remove-nil-map.

No consumers yet — S6p-3 is the first caller (via test fixture setup
for the item-validation tests). Future sub-specs wire UI-modal-open
handlers and a script opcode.

Part of S6p; prerequisite for S6m-D3 / S6o-D3 closure in S6p-3.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: `handleOpLocU` + `handleOpNpcU` validation gates — closes S6m-D3 + S6o-D3

**Goal:** Insert listener-lookup + inventory-resolution + `HasAt` gates into both handlers. Update the S6m-D3 and S6o-D3 deviation comments to closure form. Migrate the 2 happy-path fixtures to set up a listener + inv. Add 4 validation tests per handler.

**Files:**
- Modify: `modules/world/handler_opnpc.go` — add `resolveListenerInv` helper near top; wire gate + update deviation comment
- Modify: `modules/world/handler_oploc.go` — wire gate + update deviation comment
- Modify: `modules/world/handler_oploc_test.go` — update `TestHandleOpLocUSetsInteraction`; add 4 validation tests
- Modify: `modules/world/handler_opnpc_test.go` — update `TestHandleOpNpcUSetsInteraction`; add 4 validation tests

### TDD context

TDD flow here has a subtlety: the 3 new "rejection" tests require the handler's new gate to pass (they currently would falsely PASS because the handler ignores listeners and falls through to the happy-path rejection at later gates — but crucially, the *reasons* the tests expect rejection don't match). Specifically:

- **MissingListenerRejected**: currently passes (happy path sets target because no listener gate) — the test's `p.target != nil` assertion would FAIL after test creation, before impl.
- **InvalidSlotRejected**: same.
- **ItemMismatchRejected**: same.
- **HappyPathWithRealInv**: currently the existing test passes because happy-path works without the gate.

**And** the existing `TestHandleOpLocUSetsInteraction` / `TestHandleOpNpcUSetsInteraction` will BREAK after impl because they have no listener registered — they'll hit the new gate and get UnsetMapFlag instead of interaction.

Order of operations:
1. Add the `resolveListenerInv` helper (safe: no consumers yet)
2. Write the 4 new tests per handler (3 fail in unexpected ways, 1 passes)
3. Update the 2 existing happy-path tests to register listener + inv (they still pass — pre-impl the gate is absent)
4. Implement both handlers' new gates
5. All tests pass

- [ ] **Step 1: Add the `resolveListenerInv` helper.**

At the top of `modules/world/handler_opnpc.go`, after the existing imports and before `handleOpNpc`, add:

```go
// resolveListenerInv returns the inventory the given listener observes,
// or nil if it can't be resolved. Source = -1 → world-shared inventory
// (Server.invs[Type]); otherwise the source is another player's slot,
// and the inventory is that player's local invs[Type]. Mirrors TS
// getInventoryFromListener in Player.ts.
func resolveListenerInv(s *Server, listener InventoryListener) *inventory.Inventory {
	if listener.Source == -1 {
		return s.invs[listener.Type]
	}
	if listener.Source < 0 || listener.Source >= len(s.players) {
		return nil
	}
	other := s.players[listener.Source]
	if other == nil {
		return nil
	}
	return other.invs[listener.Type]
}
```

Check the top of `handler_opnpc.go` — if `inventory` isn't already imported, add `"github.com/zsrv/goscape/pkg/inventory"` to the import block.

- [ ] **Step 2: Run build to confirm the helper compiles.**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./modules/world/...`
Expected: PASS (unused function warning is fine — vet may flag it, but go build won't fail on unused functions at package scope).

- [ ] **Step 3: Update the existing `TestHandleOpLocUSetsInteraction` fixture to register a listener + inv.**

In `modules/world/handler_oploc_test.go`, find `TestHandleOpLocUSetsInteraction` (around line 432). Replace its body with:

```go
func TestHandleOpLocUSetsInteraction(t *testing.T) {
	s, p, loc, _ := makeOpLocFixture(t)

	// Register listener at com=149 pointing at world-shared inv type 93,
	// and populate that inv with the claimed item (1511) at the claimed
	// slot (3). Required after S6p-3's validation gate.
	if s.invs == nil {
		s.invs = make(map[int]*inventory.Inventory)
	}
	inv := inventory.New(93, 28, inventory.StackNormal)
	inv.Items[3] = &inventory.Item{Id: 1511, Count: 1}
	s.invs[93] = inv
	p.invListenOnCom(93, 149, -1)

	if err := handleOpLocU(p, p2x6Payload(100, 100, 42, 1511, 3, 149)); err != nil {
		t.Fatalf("handleOpLocU: %v", err)
	}

	if p.target != loc {
		t.Errorf("target: got %v, want loc", p.target)
	}
	if p.targetOp != targetOpLocU {
		t.Errorf("targetOp: got %d, want targetOpLocU (%d)", p.targetOp, targetOpLocU)
	}
	if p.lastUseItem != 1511 {
		t.Errorf("lastUseItem: got %d, want 1511 (useObj)", p.lastUseItem)
	}
	if p.lastUseSlot != 3 {
		t.Errorf("lastUseSlot: got %d, want 3", p.lastUseSlot)
	}
	if p.targetSubject.com != -1 {
		t.Errorf("targetSubject.com: got %d, want -1 (OpLocU passes -1)", p.targetSubject.com)
	}
	if p.targetSubject.typ != 42 || p.targetSubject.x != 100 || p.targetSubject.z != 100 || p.targetSubject.level != 0 {
		t.Errorf("targetSubject snapshot: got (typ=%d,x=%d,z=%d,level=%d), want (42,100,100,0)",
			p.targetSubject.typ, p.targetSubject.x, p.targetSubject.z, p.targetSubject.level)
	}
}
```

Ensure `handler_oploc_test.go`'s import block includes `"github.com/zsrv/goscape/pkg/inventory"`. If not, add it.

**Verified `pkg/inventory` shape:** `inventory.New(typeId, capacity, stackType int) *Inventory`, `Inventory.Items []*Item` (length == `Capacity`), `Item{Id int, Count int}`, `Inventory.HasAt(slot, id int) bool`. There is no `Set` method — use direct slice assignment `inv.Items[slot] = &inventory.Item{Id: ..., Count: ...}` exactly as shown above.

- [ ] **Step 4: Add 4 new OpLocU validation tests.**

Append to `modules/world/handler_oploc_test.go`:

```go
// TestHandleOpLocUMissingListenerRejected verifies that a useCom with no
// registered listener → UnsetMapFlag, no state change.
// S6p closes S6m-D3: per TS OpLocUHandler.ts:50-66, the handler must
// reject wire tuples whose useCom isn't in the player's registered
// listeners.
func TestHandleOpLocUMissingListenerRejected(t *testing.T) {
	s, p, _, cc := makeOpLocFixture(t)
	// NO invListenOnCom — the map stays empty.
	if s.invs == nil {
		s.invs = make(map[int]*inventory.Inventory)
	}
	s.invs[93] = inventory.New(93, 28, inventory.StackNormal)

	received := drainConn(t, cc)
	_ = handleOpLocU(p, p2x6Payload(100, 100, 42, 1511, 3, 149))
	p.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for missing listener, got nothing")
	}
	if p.target != nil {
		t.Error("target should remain nil for missing listener")
	}
}

// TestHandleOpLocUInvalidSlotRejected verifies useSlot outside the
// registered inv's capacity → UnsetMapFlag. HasAt returns false when
// slot is OOB (pkg/inventory.Inventory.Get bounds-checks).
func TestHandleOpLocUInvalidSlotRejected(t *testing.T) {
	s, p, _, cc := makeOpLocFixture(t)
	if s.invs == nil {
		s.invs = make(map[int]*inventory.Inventory)
	}
	s.invs[93] = inventory.New(93, 28, inventory.StackNormal) // capacity 28
	p.invListenOnCom(93, 149, -1)

	received := drainConn(t, cc)
	// useSlot = 99, OOB for a capacity-28 inv.
	_ = handleOpLocU(p, p2x6Payload(100, 100, 42, 1511, 99, 149))
	p.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for invalid slot, got nothing")
	}
	if p.target != nil {
		t.Error("target should remain nil for invalid slot")
	}
}

// TestHandleOpLocUItemMismatchRejected verifies that when slot 3 holds
// a different item id than useObj, the handler rejects.
func TestHandleOpLocUItemMismatchRejected(t *testing.T) {
	s, p, _, cc := makeOpLocFixture(t)
	if s.invs == nil {
		s.invs = make(map[int]*inventory.Inventory)
	}
	inv := inventory.New(93, 28, inventory.StackNormal)
	inv.Items[3] = &inventory.Item{Id: 9999, Count: 1} // NOT 1511
	s.invs[93] = inv
	p.invListenOnCom(93, 149, -1)

	received := drainConn(t, cc)
	_ = handleOpLocU(p, p2x6Payload(100, 100, 42, 1511, 3, 149)) // claims 1511
	p.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for item mismatch, got nothing")
	}
	if p.target != nil {
		t.Error("target should remain nil for item mismatch")
	}
}

// TestHandleOpLocUHappyPathWithOtherPlayerInv verifies Source != -1
// (other-player-inv, e.g., bank/trade viewer) also works through
// resolveListenerInv.
func TestHandleOpLocUHappyPathWithOtherPlayerInv(t *testing.T) {
	s, p, loc, _ := makeOpLocFixture(t)

	// Create a second player at slot 2 with inv type 93 containing
	// the claimed item at the claimed slot.
	other, _ := newTestPlayer(t)
	other.invs = map[int]*inventory.Inventory{}
	inv := inventory.New(93, 28, inventory.StackNormal)
	inv.Items[3] = &inventory.Item{Id: 1511, Count: 1}
	other.invs[93] = inv
	s.players[2] = other

	// Register listener pointing at slot 2's inv type 93.
	p.invListenOnCom(93, 149, 2)

	if err := handleOpLocU(p, p2x6Payload(100, 100, 42, 1511, 3, 149)); err != nil {
		t.Fatalf("handleOpLocU: %v", err)
	}
	if p.target != loc {
		t.Errorf("target: got %v, want loc", p.target)
	}
}
```

- [ ] **Step 5: Run the new OpLocU tests to verify they fail in the expected ways.**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestHandleOpLocU -v`
Expected:
- `TestHandleOpLocUSetsInteraction` — PASS (the pre-impl gate is absent; listener setup is benign)
- `TestHandleOpLocUMissingListenerRejected` — FAIL (`p.target` is not nil; the handler accepts without checking listener)
- `TestHandleOpLocUInvalidSlotRejected` — FAIL (same reason)
- `TestHandleOpLocUItemMismatchRejected` — FAIL (same reason)
- `TestHandleOpLocUHappyPathWithOtherPlayerInv` — PASS
- Other existing OpLocU tests (`DelayedPlayerRejected`, `ShortPayloadRejected`, `OutOfViewportRejected`, `MissingLocRejected`, `MissingLocTypeRejected`) — PASS

- [ ] **Step 6: Update the existing `TestHandleOpNpcUSetsInteraction` fixture.**

In `modules/world/handler_opnpc_test.go`, find `TestHandleOpNpcUSetsInteraction` (around line 332). Replace its body with:

```go
func TestHandleOpNpcUSetsInteraction(t *testing.T) {
	s, p, npc := makeOpNpcFixture(t)

	// Register listener + populate the inv with the claimed item.
	if s.invs == nil {
		s.invs = make(map[int]*inventory.Inventory)
	}
	inv := inventory.New(93, 28, inventory.StackNormal)
	inv.Items[3] = &inventory.Item{Id: 1511, Count: 1}
	s.invs[93] = inv
	p.invListenOnCom(93, 149, -1)

	if err := handleOpNpcU(p, p2x4NpcUPayload(1, 1511, 3, 149)); err != nil {
		t.Fatalf("handleOpNpcU: %v", err)
	}

	if p.target != npc {
		t.Errorf("target: got %v, want npc", p.target)
	}
	if p.targetOp != targetOpNpcU {
		t.Errorf("targetOp: got %d, want targetOpNpcU (%d)", p.targetOp, targetOpNpcU)
	}
	if p.lastUseItem != 1511 {
		t.Errorf("lastUseItem: got %d, want 1511 (useObj)", p.lastUseItem)
	}
	if p.lastUseSlot != 3 {
		t.Errorf("lastUseSlot: got %d, want 3", p.lastUseSlot)
	}
	if p.targetSubject.com != -1 {
		t.Errorf("targetSubject.com: got %d, want -1 (OpNpcU passes -1)", p.targetSubject.com)
	}
}
```

Ensure `handler_opnpc_test.go` imports `"github.com/zsrv/goscape/pkg/inventory"`. If not, add it.

- [ ] **Step 7: Add 4 new OpNpcU validation tests.**

Append to `modules/world/handler_opnpc_test.go`:

```go
// TestHandleOpNpcUMissingListenerRejected — S6p closes S6o-D3. A useCom
// with no registered listener rejects with UnsetMapFlag.
func TestHandleOpNpcUMissingListenerRejected(t *testing.T) {
	s, p, _ := makeOpNpcFixture(t)
	if s.invs == nil {
		s.invs = make(map[int]*inventory.Inventory)
	}
	s.invs[93] = inventory.New(93, 28, inventory.StackNormal)
	// NO invListenOnCom.

	_ = handleOpNpcU(p, p2x4NpcUPayload(1, 1511, 3, 149))

	if p.target != nil {
		t.Error("target should remain nil for missing listener")
	}
}

// TestHandleOpNpcUInvalidSlotRejected verifies useSlot OOB of the
// registered inv → UnsetMapFlag.
func TestHandleOpNpcUInvalidSlotRejected(t *testing.T) {
	s, p, _ := makeOpNpcFixture(t)
	if s.invs == nil {
		s.invs = make(map[int]*inventory.Inventory)
	}
	s.invs[93] = inventory.New(93, 28, inventory.StackNormal)
	p.invListenOnCom(93, 149, -1)

	// useSlot=99, OOB.
	_ = handleOpNpcU(p, p2x4NpcUPayload(1, 1511, 99, 149))

	if p.target != nil {
		t.Error("target should remain nil for invalid slot")
	}
}

// TestHandleOpNpcUItemMismatchRejected verifies slot N holds a
// different id than useObj → UnsetMapFlag.
func TestHandleOpNpcUItemMismatchRejected(t *testing.T) {
	s, p, _ := makeOpNpcFixture(t)
	if s.invs == nil {
		s.invs = make(map[int]*inventory.Inventory)
	}
	inv := inventory.New(93, 28, inventory.StackNormal)
	inv.Items[3] = &inventory.Item{Id: 9999, Count: 1} // NOT 1511
	s.invs[93] = inv
	p.invListenOnCom(93, 149, -1)

	_ = handleOpNpcU(p, p2x4NpcUPayload(1, 1511, 3, 149))

	if p.target != nil {
		t.Error("target should remain nil for item mismatch")
	}
}

// TestHandleOpNpcUHappyPathWithOtherPlayerInv verifies Source != -1
// path works through resolveListenerInv.
func TestHandleOpNpcUHappyPathWithOtherPlayerInv(t *testing.T) {
	s, p, npc := makeOpNpcFixture(t)

	other, _ := newTestPlayer(t)
	other.invs = map[int]*inventory.Inventory{}
	inv := inventory.New(93, 28, inventory.StackNormal)
	inv.Items[3] = &inventory.Item{Id: 1511, Count: 1}
	other.invs[93] = inv
	s.players[2] = other

	p.invListenOnCom(93, 149, 2)

	if err := handleOpNpcU(p, p2x4NpcUPayload(1, 1511, 3, 149)); err != nil {
		t.Fatalf("handleOpNpcU: %v", err)
	}
	if p.target != npc {
		t.Errorf("target: got %v, want npc", p.target)
	}
}
```

- [ ] **Step 8: Run the new OpNpcU tests to verify failure mode.**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestHandleOpNpcU -v`
Expected:
- `TestHandleOpNpcUSetsInteraction` — PASS (pre-impl the gate is absent)
- `TestHandleOpNpcUMissingListenerRejected` — FAIL
- `TestHandleOpNpcUInvalidSlotRejected` — FAIL
- `TestHandleOpNpcUItemMismatchRejected` — FAIL
- `TestHandleOpNpcUHappyPathWithOtherPlayerInv` — PASS
- Other existing OpNpcU tests — PASS

- [ ] **Step 9: Wire the validation gate into `handleOpLocU`.**

In `modules/world/handler_oploc.go`:

(a) Replace the deviation comment block at lines 183-202 (from `// Validation gates` through `// "InvListener keyed-map refactor..."`). Find:

```go
// Validation gates (subset of TS OpLocUHandler.ts:~79):
//  1. delayed player → UnsetMapFlag
//  2. payload too short → UnsetMapFlag
//  3. coords outside viewport → UnsetMapFlag
//  4. Server.GetLoc returns nil → UnsetMapFlag
//  5. LocType not registered → UnsetMapFlag
//
// DEVIATION (S6m-D2): TS validates useCom references a usable, visible
// interface component (OpLocUHandler.ts:~25-35). Skipped — no component
// registry yet.
//
// DEVIATION (S6m-D3): TS does an inventory-listener lookup by useCom to
// verify the player has an inv listening at that interface, plus
// slot-bounds + item-at-slot-matches-useObj validation
// (OpLocUHandler.ts:~45-70). Goscape's invListeners is a slice, not a
// keyed map, so this lookup shape doesn't translate directly. Skip;
// scripts reading p.LastUseItem()/p.LastUseSlot() get raw wire values.
// Security risk: client can claim any item/slot. Real scripts
// defensively re-check via inv_getobj-style opcodes. Follow-up:
// "InvListener keyed-map refactor + OpLocU item validation" sub-spec.
//
// DEVIATION (S6m-D4): TS checks members-only items against NODE_MEMBERS
// server config (OpLocUHandler.ts:~71-77). Skipped because goscape has
// no members-config surface yet. Follow-up: "members-config + item-
// gating" sub-spec.
```

Replace with:

```go
// Validation gates (subset of TS OpLocUHandler.ts:~79):
//  1. delayed player → UnsetMapFlag
//  2. payload too short → UnsetMapFlag
//  3. coords outside viewport → UnsetMapFlag
//  4. Server.GetLoc returns nil → UnsetMapFlag
//  5. LocType not registered → UnsetMapFlag
//  6. useCom not in invListeners → UnsetMapFlag (S6p)
//  7. listener's inventory unresolved or slot/item mismatch → UnsetMapFlag (S6p)
//
// DEVIATION (S6m-D2): TS validates useCom references a usable, visible
// interface component (OpLocUHandler.ts:~25-35). Skipped — no component
// registry yet.
//
// S6m-D3 closed in S6p: per-op useCom listener lookup + slot/item
// validation gates added below, mirroring TS OpLocUHandler.ts:50-66.
//
// DEVIATION (S6m-D4): TS checks members-only items against NODE_MEMBERS
// server config (OpLocUHandler.ts:~71-77). Skipped because goscape has
// no members-config surface yet. Follow-up: "members-config + item-
// gating" sub-spec.
```

(b) At line 234, change the useCom decode:

```go
_ = int(r.G2()) // useCom — deliberately discarded (S6m-D2/D3)
```

To:

```go
useCom := int(r.G2())
```

(c) After the existing LocType gate (currently ending line 258) and BEFORE `p.lastUseItem = useObj` (line 260), insert:

```go
// S6m-D3 closed: verify the player has an inv listener at useCom
// and that the claimed item lives at the claimed slot (TS
// OpLocUHandler.ts:50-66).
listener, ok := p.invListeners[useCom]
if !ok {
	sendUnsetMapFlag(p)
	return nil
}
inv := resolveListenerInv(s, listener)
if inv == nil {
	sendUnsetMapFlag(p)
	return nil
}
if !inv.HasAt(useSlot, useObj) {
	sendUnsetMapFlag(p)
	return nil
}
```

- [ ] **Step 10: Wire the validation gate into `handleOpNpcU`.**

In `modules/world/handler_opnpc.go`:

(a) Replace the deviation comment block at lines 128-147. Find:

```go
// Validation gates (subset of TS OpNpcUHandler.ts):
//  1. delayed player → UnsetMapFlag
//  2. payload too short → UnsetMapFlag
//  3. slot out of range → UnsetMapFlag
//  4. NPC nil or dead → UnsetMapFlag
//  5. NpcType nil → UnsetMapFlag
//
// DEVIATION S6o-D2: TS validates useCom references a usable, visible
// interface component. Skipped — no component registry. (Mirrors S6m-D2.)
//
// DEVIATION S6o-D3: TS does an inventory-listener lookup by useCom +
// slot-bounds + item-at-slot-matches-useObj validation. Goscape's
// invListeners is a slice not keyed map, so this lookup shape doesn't
// translate. Skip; scripts reading p.LastUseItem()/p.LastUseSlot() get
// raw wire values. (Mirrors S6m-D3.)
//
// DEVIATION S6o-D4: TS checks members-only items against NODE_MEMBERS
// config. Skipped — no members-config surface. (Mirrors S6m-D4.)
```

Replace with:

```go
// Validation gates (subset of TS OpNpcUHandler.ts):
//  1. delayed player → UnsetMapFlag
//  2. payload too short → UnsetMapFlag
//  3. slot out of range → UnsetMapFlag
//  4. NPC nil or dead → UnsetMapFlag
//  5. NpcType nil → UnsetMapFlag
//  6. useCom not in invListeners → UnsetMapFlag (S6p)
//  7. listener's inventory unresolved or slot/item mismatch → UnsetMapFlag (S6p)
//
// DEVIATION S6o-D2: TS validates useCom references a usable, visible
// interface component. Skipped — no component registry. (Mirrors S6m-D2.)
//
// S6o-D3 closed in S6p: per-op useCom listener lookup + slot/item
// validation gates added below, mirroring TS OpNpcUHandler.ts:35-50.
//
// DEVIATION S6o-D4: TS checks members-only items against NODE_MEMBERS
// config. Skipped — no members-config surface. (Mirrors S6m-D4.)
```

(b) At line 170, change:

```go
_ = int(r.G2()) // useCom — deliberately discarded (S6o-D2/D3)
```

To:

```go
useCom := int(r.G2())
```

(c) After the `npc.typ == nil` gate (currently ending line 184) and BEFORE `p.lastUseItem = useObj` (line 186), insert:

```go
// S6o-D3 closed: verify the player has an inv listener at useCom
// and that the claimed item lives at the claimed slot (TS
// OpNpcUHandler.ts:35-50).
listener, ok := p.invListeners[useCom]
if !ok {
	sendUnsetMapFlag(p)
	return nil
}
inv := resolveListenerInv(s, listener)
if inv == nil {
	sendUnsetMapFlag(p)
	return nil
}
if !inv.HasAt(useSlot, useObj) {
	sendUnsetMapFlag(p)
	return nil
}
```

- [ ] **Step 11: Run all OpLocU and OpNpcU tests.**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestHandleOpLocU|TestHandleOpNpcU' -v`
Expected: All pass — the 4 new rejection tests per handler now correctly reject through the new gate; happy-path tests still pass with their listener+inv setup.

- [ ] **Step 12: Run the full world test suite.**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...`
Expected: PASS

- [ ] **Step 13: Run full repo tests + vet + race detector.**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`
Expected: PASS

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...`
Expected: no diagnostics

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/...`
Expected: PASS

- [ ] **Step 14: Commit.**

```bash
git add modules/world/handler_oploc.go modules/world/handler_opnpc.go \
        modules/world/handler_oploc_test.go modules/world/handler_opnpc_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): OpLocU/OpNpcU listener lookup + item validation — closes S6m-D3/S6o-D3 (S6p-3)

Add resolveListenerInv(server, listener) helper (shared between the two
handlers) that returns the inventory a listener observes:
  Source=-1 → server's world-shared inv map[Type]
  Source>=0 → that slot's player's local invs[Type]
  any unresolvable path → nil

Wire three new validation gates into both handleOpLocU and handleOpNpcU,
between the type/dead gate and the interaction-setting step:
  1. listener := p.invListeners[useCom]; reject if absent
  2. inv := resolveListenerInv(s, listener); reject if nil
  3. inv.HasAt(useSlot, useObj); reject if slot/item mismatch

Mirrors TS OpLocUHandler.ts:50-66 and OpNpcUHandler.ts:35-50.

Updates to S6m-D3 and S6o-D3 deviation blocks: both flipped to closure
form. Still-open deviations after S6p: S6m-D1/D2/D4, S6o-D1/D2/D4
(component-visibility + members-only — separate infra tracks).

Tests: 2 existing happy-path fixtures (TestHandleOpLocUSetsInteraction,
TestHandleOpNpcUSetsInteraction) updated to register a listener + inv
before the call. 8 new tests (4 per handler):
  - MissingListenerRejected
  - InvalidSlotRejected
  - ItemMismatchRejected
  - HappyPathWithOtherPlayerInv (exercises Source != -1 branch)

Other existing OP*U tests (delayed/short-payload/OOB-slot/dead-npc/
missing-LocType) still pass because their early gates fire before the
new listener check.

Closes S6m-D3 and S6o-D3. No new deviations.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Final whole-implementation review

After all 3 tasks commit, dispatch a final code-reviewer subagent over the full S6p diff range (Task 1 through Task 3 commits). Scope:

- All changes match spec `docs/superpowers/specs/2026-04-21-runescript-s6p-invlistener-map-item-validation-design.md`
- Zero undocumented TS deviations
- S6m-D3 and S6o-D3 closed (comments updated, gates present, tests cover missing-listener + invalid-slot + item-mismatch paths)
- No leftover `invListeners []InventoryListener` references anywhere
- `updateInvs` map iteration correctly flips `FirstSeen` via read-modify-write (no lost mutations)
- `resolveListenerInv` handles all 4 resolution branches (Source=-1 with hit, Source=-1 with nil, Source>=0 with hit, Source>=0 with missing player)
- `go test ./...` + `go vet ./...` + `go test -race ./modules/world/...` all green

---

## Deviation table (unchanged from spec)

| ID | Status | Reason |
|---|---|---|
| S6m-D1 / S6o-D1 | Open | spellCom component visibility — needs component registry |
| S6m-D2 / S6o-D2 | Open | useCom component visibility — same |
| **S6m-D3** | **✅ CLOSED in S6p** | OpLocU item-in-slot validation wired |
| **S6o-D3** | **✅ CLOSED in S6p** | OpNpcU item-in-slot validation wired |
| S6m-D4 / S6o-D4 | Open | members-only item gate — needs members-config surface |
| S6l-D1 | Open | apRange=-1 sentinel |
| S6l-D3 | Open | ProtectedActivePlayer gate |
| S6l-D4 | Open | LOS/collision in distance checks |
| S6l-D5 | Open | p_op* / nextTarget re-anchor opcodes |

**Zero new deviations introduced by S6p.**
