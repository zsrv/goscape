# NAI-64 — Per-slot `clearComListeners` + `invStopListenOnCom` packet collapse — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close `NAI-53-D-CLEARCOMLISTENERS-PER-SLOT` by replacing
`(*Player).encodeOut`'s blanket-clear of `invListeners` with a per-slot
`clearComListeners(rootCom)` helper called from `(*Player).CloseModal`,
and collapse `(*Player).invStopListenOnCom` to write
`UpdateInvStopTransmit` per TS Player.ts:1464-1471.

**Architecture:** TS-faithful 1:1 layering. New
`(*Player).clearComListeners(rootCom)` mirrors TS Player.ts:728-739
(filters `invListeners` by `Component.RootLayer`, calls
`invStopListenOnCom` per match). `(*Player).invStopListenOnCom` becomes
delete-and-write rather than just delete. `CloseModal` calls
`clearComListeners` once per slot between IF_CLOSE trigger and slot reset.
The blanket-clear in `encodeOut` is deleted.

**Tech Stack:** Go 1.26+, `pkg/objtype.ComponentType`, existing test
helpers `newTestPlayer`/`newInvListenerTestPlayer`/`seedComponentTypes`/`drainConn`.

**Spec:** `docs/superpowers/specs/2026-05-01-nai-64-clearcomlisteners-per-slot-design.md`.

---

## File Structure

| File | Action | Responsibility |
|---|---|---|
| `modules/world/player.go` | Modify (3 sites) | (1) Drop `encodeOut` blanket-clear loop and `clear(invListeners)` call. (2) Collapse `invStopListenOnCom` to delete-and-write. (3) Add `clearComListeners` method. |
| `modules/world/player_script.go` | Modify (1 site) | Wire `p.clearComListeners(slotCom)` into each of the three `CloseModal` per-slot branches; retire DEVIATION block. |
| `modules/world/modal_close_test.go` | Modify (1 test) + add 2 tests | Rewrite `TestModalCloseEmitsStopTransmit` (no longer matches behaviour); add `TestCloseModalClearsOnlyListenersForClosingSlots` and `TestCloseModalNoListenersStillWritesIfClose`. |
| `modules/world/player_inv_test.go` | Modify (3 tests) + add 3 tests | Add packet assertions to existing `invStopListenOnCom` tests; add `TestInvStopListenOnComWritesUpdatePacket`, `TestInvStopListenOnComMissingKeyWritesNoPacket`, `TestInvStopListenOnComNilMapWritesNoPacket`. |
| `modules/world/player_clearcomlisteners_test.go` | Create | New test file for the four `clearComListeners` unit tests (file-locality with helper). |
| `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md` | Modify | Append `## NAI-64 — CLOSED` section in T3. |

---

## Pre-flight grep targets (controller runs before each task dispatch)

Before T1:
```
rg -n "func \(p \*Player\) invStopListenOnCom\b" modules/world/
rg -n "\bsendUpdateInvStopTransmit\b" modules/world/
rg -n "func seedComponentTypes\b" modules/world/
rg -n "func drainConn\b" modules/world/
rg -n "func newInvListenerTestPlayer\b" modules/world/
rg -n "func \(p \*Player\) writeOut\b" modules/world/
```

Before T2:
```
rg -n "for _, l := range p.invListeners" modules/world/
rg -n "clear\(p.invListeners\)" modules/world/
rg -n "NAI-53-D-CLEARCOMLISTENERS-PER-SLOT" pkg/ modules/ cmd/
rg -n "TestModalCloseEmitsStopTransmit\b" modules/world/
rg -n "TestNoStopTransmitWithoutModalClose\b" modules/world/
```

Before T3 (close):
```
rg -n "NAI-53-D-CLEARCOMLISTENERS-PER-SLOT" pkg/ modules/ cmd/   # must be 0
rg -n "for _, l := range p.invListeners" modules/world/         # must be 0
rg -n "clear\(p.invListeners\)" modules/world/                  # must be 0
```

---

## Task 1: Collapse `invStopListenOnCom` and add `clearComListeners` helper

**Goal:** Add the new helper and re-shape the existing `invStopListenOnCom`. T1 leaves the bulk path in `encodeOut` ALIVE — no production behaviour changes yet. Tests for the new primitives stand on their own.

**Files:**
- Modify: `modules/world/player.go:822-827` (`invStopListenOnCom` body)
- Modify: `modules/world/player.go:822-827` (add `clearComListeners` immediately after)
- Modify: `modules/world/player_inv_test.go:88, 112, 125` (add packet assertions to 3 existing tests)
- Create: `modules/world/player_clearcomlisteners_test.go`
- Modify: `modules/world/player_inv_test.go` (append 3 new `invStopListenOnCom` tests)

### Step 1.1: Write a failing packet-emission test for `invStopListenOnCom`

- [ ] **Write the failing test**

Append to `modules/world/player_inv_test.go`:

```go
// TestInvStopListenOnComWritesUpdatePacket pins TS Player.ts:1464-1471:
// invStopListenOnCom must remove the listener AND write
// OpUpdateInvStopTransmit(com).
func TestInvStopListenOnComWritesUpdatePacket(t *testing.T) {
	p, cc := newTestPlayer(t)
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	p.invListenOnCom(93, 149, -1)

	received := drainConn(t, cc)
	p.invStopListenOnCom(149)
	p.client.flushWrite()

	got := <-received
	// Wire: 1 opcode byte + 2 payload bytes (P2 com=149) = 3 bytes.
	if len(got) != 3 {
		t.Errorf("got %d bytes, want 3 (opcode + P2 com); bytes=%v", len(got), got)
	}
	if _, ok := p.invListeners[149]; ok {
		t.Error("listener at 149 should be removed")
	}
}
```

The `io2` import alias for `pkg/io/isaac` is already present in
`player_inv_test.go` callers — verify by grep before edit. If not, add
`io2 "github.com/zsrv/goscape/pkg/io/isaac"` to the existing import block.

- [ ] **Run the test to verify it fails**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestInvStopListenOnComWritesUpdatePacket -count=1
```

Expected: FAIL — current `invStopListenOnCom` writes 0 bytes (no packet).

### Step 1.2: Collapse `invStopListenOnCom` body to delete-and-write

- [ ] **Replace `invStopListenOnCom` body**

Edit `modules/world/player.go:822-827` from:

```go
// invStopListenOnCom unregisters the listener at the given component
// ID. No-op if no listener exists there, including when the map itself
// is nil (Go's delete-on-nil is safe). Matches TS Player.ts:1464-1471.
func (p *Player) invStopListenOnCom(com int) {
	delete(p.invListeners, com)
}
```

to:

```go
// invStopListenOnCom unregisters the listener at the given component
// ID and writes UpdateInvStopTransmit(com) to the client. No-op if no
// listener exists there (mirrors TS L1466-1468 early-return; Go's
// delete-on-nil semantics make nil maps a strict subset of "no listener
// registered"). Mirrors TS Player.ts:1464-1471.
//
// Callers must ensure p.client is non-nil; sendUpdateInvStopTransmit
// (and writeOut underneath) dereferences p.client without a guard.
// Production callers (handleInvStopTransmit, CloseModal via
// clearComListeners) are all reached only with a connected client.
func (p *Player) invStopListenOnCom(com int) {
	if _, ok := p.invListeners[com]; !ok {
		return
	}
	delete(p.invListeners, com)
	sendUpdateInvStopTransmit(p, com)
}
```

- [ ] **Run the test from Step 1.1; expect PASS**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestInvStopListenOnComWritesUpdatePacket -count=1
```

Expected: PASS.

### Step 1.3: Update existing `invStopListenOnCom` tests for new packet contract

- [ ] **Edit `TestInvStopListenOnComRemovesListener`** (`player_inv_test.go:88-107`)

Replace the test body with:

```go
func TestInvStopListenOnComRemovesListener(t *testing.T) {
	p, cc := newTestPlayer(t)
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	p.invListenOnCom(93, 149, -1)
	p.invListenOnCom(100, 200, -1)
	if len(p.invListeners) != 2 {
		t.Fatalf("setup: len should be 2, got %d", len(p.invListeners))
	}

	received := drainConn(t, cc)
	p.invStopListenOnCom(149)
	p.client.flushWrite()

	got := <-received
	if len(got) != 3 {
		t.Errorf("packet bytes: got %d, want 3 (opcode + P2)", len(got))
	}
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
```

- [ ] **Edit `TestInvStopListenOnComNoopForMissingKey`** (`player_inv_test.go:112-121`)

Replace with:

```go
func TestInvStopListenOnComNoopForMissingKey(t *testing.T) {
	p, cc := newTestPlayer(t)
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	p.invListenOnCom(93, 149, -1)

	received := drainConn(t, cc)
	p.invStopListenOnCom(999) // never registered
	p.client.flushWrite()

	got := <-received
	if len(got) != 0 {
		t.Errorf("missing-key stop should write no packet; got %d bytes", len(got))
	}
	if len(p.invListeners) != 1 {
		t.Errorf("len: got %d, want 1 (unrelated listener should remain)", len(p.invListeners))
	}
}
```

- [ ] **Edit `TestInvStopListenOnComNoopForNilMap`** (`player_inv_test.go:125-136`)

Replace with:

```go
func TestInvStopListenOnComNoopForNilMap(t *testing.T) {
	p, cc := newTestPlayer(t)
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	if p.invListeners != nil {
		t.Fatalf("precondition: invListeners should start nil")
	}

	received := drainConn(t, cc)
	p.invStopListenOnCom(149) // must not panic
	p.client.flushWrite()

	got := <-received
	if len(got) != 0 {
		t.Errorf("nil-map stop should write no packet; got %d bytes", len(got))
	}
	if p.invListeners != nil {
		t.Error("stop on nil map should not cause an allocation")
	}
}
```

- [ ] **Run the modified tests; expect PASS**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestInvStopListenOnComRemovesListener|TestInvStopListenOnComNoopForMissingKey|TestInvStopListenOnComNoopForNilMap" -count=1
```

Expected: PASS.

### Step 1.4: Add explicit nil-map and missing-key packet-absence tests

Already covered by Step 1.3 edits (which fold the packet-absence
assertion into the existing tests). Skip — no new test needed; the dual
coverage from the spec's "test #2" and "test #3" is satisfied by
`TestInvStopListenOnComNoopForMissingKey` and
`TestInvStopListenOnComNoopForNilMap` after Step 1.3.

### Step 1.5: Add `clearComListeners` helper (still failing — no callers yet)

- [ ] **Insert helper after `invStopListenOnCom`**

Add immediately after `invStopListenOnCom` in `modules/world/player.go`:

```go
// clearComListeners removes every inv-listener whose Component.RootLayer
// equals rootCom and writes UpdateInvStopTransmit per removal. No-op
// when rootCom is -1 (slot was unset; mirrors TS L729-731). No-op when
// the player has no Server bound (goscape defensive; TS skips this
// check since TS Components are a global singleton — Component.get is
// always reachable).
//
// Mirrors TS Player.ts:728-739. Closes NAI-53-D-CLEARCOMLISTENERS-PER-SLOT.
//
// Iteration safety: Go's spec guarantees `delete` during `range` over a
// map is well-defined — deleted keys are not re-yielded. Calling
// invStopListenOnCom (which deletes) inside the range loop is safe.
func (p *Player) clearComListeners(rootCom int) {
	if rootCom == -1 {
		return
	}
	if p.client == nil || p.client.server == nil {
		return
	}
	s := p.client.server
	for com := range p.invListeners {
		c := s.lookupComponent(com)
		if c == nil {
			// goscape defensive; TS assumes Component.get(com) is non-nil.
			continue
		}
		if c.RootLayer == rootCom {
			p.invStopListenOnCom(com)
		}
	}
}
```

- [ ] **Verify it compiles**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./modules/world/...
```

Expected: clean.

### Step 1.6: Write `clearComListeners` unit tests

- [ ] **Create `modules/world/player_clearcomlisteners_test.go`**

```go
package world

import (
	"testing"

	io2 "github.com/zsrv/goscape/pkg/io/isaac"
	"github.com/zsrv/goscape/pkg/objtype"
)

// TestClearComListenersFiltersByRootLayer pins TS Player.ts:733-737:
// only listeners whose Component.RootLayer matches the arg are removed.
func TestClearComListenersFiltersByRootLayer(t *testing.T) {
	s := newTestServer(t)
	p, cc := newInvListenerTestPlayer(t, s, 1)
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		149: {RootLayer: 100},
		200: {RootLayer: 999},
	})
	p.invListenOnCom(93, 149, -1)
	p.invListenOnCom(94, 200, -1)

	received := drainConn(t, cc)
	p.clearComListeners(100)
	p.client.flushWrite()

	got := <-received
	// One UpdateInvStopTransmit packet = 1 opcode + 2 payload = 3 bytes.
	if len(got) != 3 {
		t.Errorf("packet bytes: got %d, want 3 (one StopTransmit)", len(got))
	}
	if _, ok := p.invListeners[149]; ok {
		t.Error("listener at 149 (RootLayer 100) should be removed")
	}
	if _, ok := p.invListeners[200]; !ok {
		t.Error("listener at 200 (RootLayer 999) should be retained")
	}
}

// TestClearComListenersRootMinusOneNoOp pins TS L729-731 early-return.
func TestClearComListenersRootMinusOneNoOp(t *testing.T) {
	s := newTestServer(t)
	p, cc := newInvListenerTestPlayer(t, s, 1)
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		149: {RootLayer: 100},
	})
	p.invListenOnCom(93, 149, -1)

	received := drainConn(t, cc)
	p.clearComListeners(-1)
	p.client.flushWrite()

	got := <-received
	if len(got) != 0 {
		t.Errorf("rootCom=-1 should be a no-op; got %d bytes", len(got))
	}
	if _, ok := p.invListeners[149]; !ok {
		t.Error("listener at 149 should remain")
	}
}

// TestClearComListenersUnknownComponentSkipped pins the goscape-
// defensive nil-Component skip: a listener whose com is not in the
// registry must NOT be removed (defensive; TS assumes non-nil).
func TestClearComListenersUnknownComponentSkipped(t *testing.T) {
	s := newTestServer(t)
	p, cc := newInvListenerTestPlayer(t, s, 1)
	// Seed a different component to ensure registry is non-nil.
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		1: {RootLayer: 1},
	})
	p.invListenOnCom(93, 9999, -1) // 9999 not in registry

	received := drainConn(t, cc)
	p.clearComListeners(9999)
	p.client.flushWrite()

	got := <-received
	if len(got) != 0 {
		t.Errorf("unknown com should be skipped; got %d bytes", len(got))
	}
	if _, ok := p.invListeners[9999]; !ok {
		t.Error("listener at 9999 (unknown com) should be retained")
	}
}

// TestClearComListenersRemovesMultipleSiblings pins multi-removal
// correctness + iteration safety under concurrent delete.
func TestClearComListenersRemovesMultipleSiblings(t *testing.T) {
	s := newTestServer(t)
	p, cc := newInvListenerTestPlayer(t, s, 1)
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		10: {RootLayer: 50},
		11: {RootLayer: 50},
		12: {RootLayer: 50},
		20: {RootLayer: 60},
	})
	p.invListenOnCom(93, 10, -1)
	p.invListenOnCom(93, 11, -1)
	p.invListenOnCom(93, 12, -1)
	p.invListenOnCom(93, 20, -1)

	received := drainConn(t, cc)
	p.clearComListeners(50)
	p.client.flushWrite()

	got := <-received
	// Three matched listeners → 3 × 3-byte packets = 9 bytes.
	if len(got) != 9 {
		t.Errorf("packet bytes: got %d, want 9 (3× StopTransmit)", len(got))
	}
	for _, com := range []int{10, 11, 12} {
		if _, ok := p.invListeners[com]; ok {
			t.Errorf("listener at %d should be removed", com)
		}
	}
	if _, ok := p.invListeners[20]; !ok {
		t.Error("listener at 20 (RootLayer 60) should be retained")
	}
}
```

The `io2` import is included for symmetry with sibling test files; if
the unused-import lint complains, drop it (the `newInvListenerTestPlayer`
helper installs the encryptor itself).

- [ ] **Run new tests**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestClearComListeners -count=1 -v
```

Expected: 4× PASS.

### Step 1.7: Run full package tests to verify no regressions

- [ ] **Full package run**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -count=1
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -count=1
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...
```

Expected: all PASS, vet clean. Note: `TestModalCloseEmitsStopTransmit`
still passes here because the bulk-clear path in `encodeOut` is still
alive — that test will be rewritten in T2.

### Step 1.8: Commit T1

- [ ] **Commit**

```bash
git add modules/world/player.go modules/world/player_inv_test.go modules/world/player_clearcomlisteners_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-64 T1 — invStopListenOnCom packet collapse + clearComListeners helper

Collapse (*Player).invStopListenOnCom to delete-and-write per TS
Player.ts:1464-1471. Add (*Player).clearComListeners(rootCom)
mirroring TS Player.ts:728-739 (filter invListeners by
Component.RootLayer; call invStopListenOnCom per match).

T1 leaves the encodeOut blanket-clear ALIVE — no production
behaviour change yet. T2 wires clearComListeners into CloseModal
and removes the bulk path atomically.

Tests: 1 new TestInvStopListenOnComWritesUpdatePacket; 4 new
TestClearComListeners* (rootLayer filter, -1 no-op, unknown-com
defensive skip, multi-sibling removal); 3 existing
invStopListenOnCom tests gain packet-absence/presence assertions.
EOF
)"
```

---

## Task 2: Wire `clearComListeners` into `CloseModal` and drop the `encodeOut` bulk path

**Goal:** Atomic switchover. Replace the blanket-clear in `encodeOut` with per-slot `clearComListeners` calls in `CloseModal`. Retire the deviation tag. Update the one existing test that pins the old bulk-clear behaviour.

**Files:**
- Modify: `modules/world/player.go:296-308` (drop blanket-clear loop)
- Modify: `modules/world/player_script.go:633-679` (wire `clearComListeners` per slot; retire DEVIATION block)
- Modify: `modules/world/modal_close_test.go:10-34` (rewrite `TestModalCloseEmitsStopTransmit`)
- Modify: `modules/world/modal_close_test.go` (append 2 new modal-close integration tests)

### Step 2.1: Write a failing test for per-slot CloseModal clearing

- [ ] **Append to `modules/world/modal_close_test.go`**

```go
// TestCloseModalClearsOnlyListenersForClosingSlots pins TS
// Player.ts:761-791: clearComListeners is called per slot with the
// slot's modal id; only listeners whose Component.RootLayer matches a
// closing slot are removed. Closes NAI-53-D-CLEARCOMLISTENERS-PER-SLOT.
func TestCloseModalClearsOnlyListenersForClosingSlots(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	p, cc := newInvListenerTestPlayer(t, s, 1)
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		149: {RootLayer: 100}, // matches modalMain
		250: {RootLayer: 200}, // matches modalSide
		300: {RootLayer: 999}, // unrelated
	})
	p.invListenOnCom(93, 149, -1)
	p.invListenOnCom(93, 250, -1)
	p.invListenOnCom(93, 300, -1)
	p.modalState = modalStateMain | modalStateSide
	p.modalMain = 100
	p.modalChat = -1
	p.modalSide = 200

	received := drainConn(t, cc)
	p.CloseModal(true)
	p.client.flushWrite()

	got := <-received
	// Two UpdateInvStopTransmit packets (149 + 250) = 6 bytes;
	// IF_CLOSE wire packet is emitted later by encodeOut, not here.
	if len(got) != 6 {
		t.Errorf("packet bytes: got %d, want 6 (2× StopTransmit); bytes=%v", len(got), got)
	}
	if _, ok := p.invListeners[149]; ok {
		t.Error("listener 149 (RootLayer 100 == modalMain) should be removed")
	}
	if _, ok := p.invListeners[250]; ok {
		t.Error("listener 250 (RootLayer 200 == modalSide) should be removed")
	}
	if _, ok := p.invListeners[300]; !ok {
		t.Error("listener 300 (RootLayer 999) should be retained")
	}
	if !p.refreshModalClose {
		t.Error("CloseModal should set refreshModalClose=true")
	}
}
```

The required imports (`script`, `objtype`) may already be in the file —
check the existing imports before adding. If they are not present, add:

```go
"github.com/zsrv/goscape/pkg/objtype"
```

(`script` is already imported.)

- [ ] **Run; expect FAIL**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestCloseModalClearsOnlyListenersForClosingSlots -count=1 -v
```

Expected: FAIL — current `CloseModal` doesn't call `clearComListeners`,
so listener 300 gets bulk-removed AT `encodeOut` time, but more
critically: the per-slot path doesn't fire any
`OpUpdateInvStopTransmit` packets at `CloseModal` time (they only fire
at the next `encodeOut`). The byte assertion at `CloseModal` will see 0
packet bytes, not 6.

### Step 2.2: Wire `clearComListeners` into `CloseModal` per slot

- [ ] **Edit `modules/world/player_script.go:660-679`**

Replace the per-slot dispatch block (within the `if p.client != nil &&
p.client.server != nil` branch). Locate the original three slot
branches and add a `clearComListeners` call AFTER each `runIfCloseTrigger`
and BEFORE the `-1` reset:

```go
	// Per-slot IF_CLOSE dispatch + listener clearing (Main → Chat → Side,
	// TS Player.ts:761-791 order).
	if p.client != nil && p.client.server != nil {
		s := p.client.server
		if p.modalMain != -1 {
			p.runIfCloseTrigger(s, p.modalMain)
			p.clearComListeners(p.modalMain)
			p.modalMain = -1
		}
		if p.modalChat != -1 {
			p.runIfCloseTrigger(s, p.modalChat)
			p.clearComListeners(p.modalChat)
			p.modalChat = -1
		}
		if p.modalSide != -1 {
			p.runIfCloseTrigger(s, p.modalSide)
			p.clearComListeners(p.modalSide)
			p.modalSide = -1
		}
	} else {
		// No server (test path with no Server bound) — still reset slots.
		p.modalMain = -1
		p.modalChat = -1
		p.modalSide = -1
	}
```

- [ ] **Retire the DEVIATION block** at `player_script.go:633-637`

Delete these comment lines:

```go
// DEVIATION NAI-53-D-CLEARCOMLISTENERS-PER-SLOT: TS calls
// clearComListeners(slotCom) per-slot, filtering invListeners by
// Component.rootLayer. Goscape's encodeOut clears ALL invListeners
// globally when refreshModalClose is set; per-slot rootLayer
// filtering blocks on unported Component config registry.
```

Replace with one TS-citation line in the existing doc-comment header
above `func (p *Player) CloseModal`:

```
// Mirrors TS Player.closeModal (Player.ts:741-794). Body fully
// landed across NAI-53 T1-T5; per-slot clearComListeners wired in
// NAI-64 (TS Player.ts:728-739, 767, 778, 789).
```

(Replace the existing "Mirrors TS Player.closeModal..." line.)

- [ ] **Run the new test; expect PASS**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestCloseModalClearsOnlyListenersForClosingSlots -count=1 -v
```

Expected: PASS.

### Step 2.3: Drop the `encodeOut` blanket-clear

- [ ] **Edit `modules/world/player.go:296-308`**

Replace the `if p.refreshModalClose` block. Before:

```go
	if modalChanged {
		if p.refreshModalClose {
			p.writeOut(gameserver.OpIfClose, nil)
			// Stop transmitting every currently-registered inv.
			// Approximation: TS only stops listeners bound to the closing
			// modal's components; we don't yet have a component-to-modal
			// mapping, so clear all. Re-registered on next modal open.
			for _, l := range p.invListeners {
				sendUpdateInvStopTransmit(p, l.Com)
			}
			clear(p.invListeners) // Go 1.21+ map reset; keeps allocated buckets
		}
		p.refreshModalClose = false
		p.lastModalMain = p.modalMain
		p.lastModalChat = p.modalChat
		p.lastModalSide = p.modalSide
	}
```

After:

```go
	if modalChanged {
		if p.refreshModalClose {
			// IF_CLOSE wire event. Per-listener UpdateInvStopTransmit
			// packets were already written at CloseModal time via
			// clearComListeners → invStopListenOnCom (NAI-64; TS
			// Player.ts:728-739, 767, 778, 789).
			p.writeOut(gameserver.OpIfClose, nil)
		}
		p.refreshModalClose = false
		p.lastModalMain = p.modalMain
		p.lastModalChat = p.modalChat
		p.lastModalSide = p.modalSide
	}
```

- [ ] **Verify compile**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./modules/world/...
```

Expected: clean.

### Step 2.4: Rewrite the now-obsolete `TestModalCloseEmitsStopTransmit`

- [ ] **Edit `modules/world/modal_close_test.go:10-34`**

Replace the existing `TestModalCloseEmitsStopTransmit` body. The test
currently asserts the bulk-clear in `encodeOut` writes 7 bytes (1
IF_CLOSE + 2× StopTransmit) and clears all listeners. After NAI-64,
`encodeOut` writes only 1 byte (IF_CLOSE); listener removal is the
caller's job (already verified by the new T2.1 test). Reframe this test
to pin the new contract:

```go
// TestEncodeOutOnRefreshModalCloseWritesIfCloseOnly pins that
// encodeOut emits ONLY OpIfClose when refreshModalClose is set —
// per-listener UpdateInvStopTransmit packets are now written at
// CloseModal time via clearComListeners (NAI-64 atomic switchover).
// Listener map is left intact; CloseModal owns the per-slot removals.
func TestEncodeOutOnRefreshModalCloseWritesIfCloseOnly(t *testing.T) {
	p, cc := newTestPlayer(t)
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	// Pre-load invListeners; encodeOut must NOT touch them.
	p.invListeners = map[int]InventoryListener{
		149: {Type: 93, Com: 149, Source: 2, FirstSeen: false},
		150: {Type: 93, Com: 150, Source: -1, FirstSeen: false},
	}
	p.refreshModalClose = true

	received := drainConn(t, cc)
	p.encodeOut()
	p.client.flushWrite()

	got := <-received
	// Wire: 1 byte OpIfClose (opcode, no payload). Nothing else.
	if len(got) != 1 {
		t.Errorf("got %d bytes, want 1 (IfClose only); bytes=%v", len(got), got)
	}
	if len(p.invListeners) != 2 {
		t.Errorf("invListeners must be untouched by encodeOut; got %d", len(p.invListeners))
	}
}
```

- [ ] **Run the rewritten test; expect PASS**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestEncodeOutOnRefreshModalCloseWritesIfCloseOnly|TestNoStopTransmitWithoutModalClose" -count=1 -v
```

Expected: 2× PASS.
`TestNoStopTransmitWithoutModalClose` is unchanged and still valid (no
listeners cleared without modal-close).

### Step 2.5: Add the no-listeners CloseModal regression test

- [ ] **Append to `modules/world/modal_close_test.go`**

```go
// TestCloseModalNoListenersStillClosesAndWritesIfClose pins that
// CloseModal with empty invListeners produces zero per-listener
// packets but still flags refreshModalClose so the next encodeOut
// writes OpIfClose.
func TestCloseModalNoListenersStillClosesAndWritesIfClose(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	p, cc := newInvListenerTestPlayer(t, s, 1)
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		100: {RootLayer: 100},
	})
	p.modalState = modalStateMain
	p.modalMain = 100
	// invListeners deliberately left nil.

	received := drainConn(t, cc)
	p.CloseModal(true)
	p.encodeOut()
	p.client.flushWrite()

	got := <-received
	// Wire: 1 byte OpIfClose only.
	if len(got) != 1 {
		t.Errorf("got %d bytes, want 1 (IfClose only); bytes=%v", len(got), got)
	}
}
```

- [ ] **Run; expect PASS**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestCloseModalNoListenersStillClosesAndWritesIfClose -count=1 -v
```

Expected: PASS.

### Step 2.6: Verify deviation tag is at zero

- [ ] **Grep for any remaining tag references**

```
rg -n "NAI-53-D-CLEARCOMLISTENERS-PER-SLOT" pkg/ modules/ cmd/
```

Expected: zero hits.

### Step 2.7: Full package + cross-package regression run

- [ ] **Run**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... -count=1
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...
```

Expected: all PASS, vet clean.

If any test regression surfaces (most likely candidate: another test
elsewhere that asserts the bulk-clear via `len(p.invListeners) == 0`
post-`encodeOut`), surface it to the controller; the implementer must
NOT mutate test bodies beyond what this plan codifies without explicit
re-direction.

### Step 2.8: Commit T2

- [ ] **Commit**

```bash
git add modules/world/player.go modules/world/player_script.go modules/world/modal_close_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-64 T2 — wire clearComListeners into CloseModal; drop encodeOut bulk-clear

Wire (*Player).clearComListeners into all three CloseModal per-slot
branches between IF_CLOSE trigger and slot reset, mirroring TS
Player.ts:761-791. Drop the encodeOut blanket loop that emitted
per-listener UpdateInvStopTransmit packets and called clear(invListeners).
Listener removal is now the caller's job (CloseModal via clearComListeners
→ invStopListenOnCom, all reached at CloseModal time).

Closes NAI-53-D-CLEARCOMLISTENERS-PER-SLOT (deviation tag retired
across pkg/ modules/ cmd/; verified zero hits via rg).

Wire-pattern delta: a sidebar inv listener now correctly survives a
chat-only modal close (RootLayer != closing slot's id). Prior
behaviour wiped every listener regardless of which slot closed.

Tests: 1 new TestCloseModalClearsOnlyListenersForClosingSlots; 1 new
TestCloseModalNoListenersStillClosesAndWritesIfClose; 1 rewrite
TestModalCloseEmitsStopTransmit → TestEncodeOutOnRefreshModalCloseWritesIfCloseOnly
to pin the new contract.
EOF
)"
```

---

## Task 3: Polish, memory update, close commit

**Goal:** Update `nai_followups.md` with the NAI-64 close section. Final regression run. Close commit with `Closes memory:` trailer.

**Files:**
- Modify: `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md`

### Step 3.1: Append NAI-64 close section to `nai_followups.md`

- [ ] **Append after the existing NAI-63 section**

Open `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md` and append at end:

```markdown

## NAI-64 — CLOSED 2026-05-01

**Scope:** Per-slot `(*Player).clearComListeners` helper + `(*Player).invStopListenOnCom` packet collapse. Closes NAI-53-D-CLEARCOMLISTENERS-PER-SLOT and the untracked `invStopListenOnCom` packet-write miss (TS Player.ts:1464-1471).

**Cadence:** Full sub-spec, single bundle, 3 implementation tasks + 1 close. ~70 production LOC + ~200 test LOC across 4 files (`player.go`, `player_script.go`, `player_inv_test.go`, `modal_close_test.go`) + 1 new file (`player_clearcomlisteners_test.go`).

**Spec:** `docs/superpowers/specs/2026-05-01-nai-64-clearcomlisteners-per-slot-design.md`.
**Plan:** `docs/superpowers/plans/2026-05-01-nai-64-clearcomlisteners-per-slot.md`.

**Close commit:** (this commit). T1: `<T1-SHA>`. T2: `<T2-SHA>`.

**Follow-ups closed:**
- NAI-53-D-CLEARCOMLISTENERS-PER-SLOT — `(*Player).CloseModal` now calls `clearComListeners(slotCom)` per slot, mirroring TS Player.ts:728-739, 767, 778, 789. Filters `invListeners` by `Component.RootLayer`.

**Untracked closure (no new tag, close-commit provenance):**
- `(*Player).invStopListenOnCom` packet-write miss. TS Player.ts:1464-1471 atomically removes the listener AND writes `UpdateInvStopTransmit(com)`. Goscape's prior implementation only deleted from the map; the packet only fired from the bulk `(*Player).encodeOut` path on `refreshModalClose`. Practical effect: scripts calling `inv_stoptransmit(com)` (`pkg/script/handlers_inv.go:441`) silently stopped server-side updates without notifying the client. Surfaced during NAI-64 brainstorm; retired in T1 (`<T1-SHA>`) by collapsing `invStopListenOnCom` to delete-and-write per TS shape.

**Deviations opened:** none.

**Deviations closed:** NAI-53-D-CLEARCOMLISTENERS-PER-SLOT.

**Deviation tally:** -1 from NAI-63 close.

**Wire-behaviour delta:**
- Listeners whose `Component.RootLayer` does not match a closing modal slot survive `CloseModal`. Concretely: a sidebar inv listener now correctly remains live across a chat-only modal close. Prior behaviour wiped every listener regardless of which slot closed.
- Scripts calling `inv_stoptransmit(com)` standalone (no modal close) now correctly notify the client via `OpUpdateInvStopTransmit`, ending the silent-stale-window class of bug.

**Memory entries reinforced (no edits needed):**
- `runescript_cadence.md` — full cadence, 3-task TDD bundle.
- `true_to_ts_gate.md` — every behavioural change cited against TS source.
- `enumerate_all_sites.md` — pre-flight greps in plan §"Pre-flight grep targets" verified before each task.
- `dead_api_polish.md` — bulk-clear in `encodeOut` deleted in same task that activates the per-slot path (no orphan window).
- `latent_bug_at_migration_boundary.md` — T1/T2 split deliberately kept the bulk path live during T1 to avoid the dual-path-window trap; T2 swapped producers atomically.
- `retire_deviation_grep_all_comments.md` — T2 verified zero hits for `NAI-53-D-CLEARCOMLISTENERS-PER-SLOT` across `pkg/ modules/ cmd/`.
- `defensive_gate_doc_comment_label.md` — `clearComListeners` no-server and nil-Component skips labelled "(goscape defensive; TS skips ...)".
- `audit_arithmetic_correction_in_rollup.md` — close commit narrates the untracked `invStopListenOnCom` packet-miss retirement with full TS citation.
- `close_commit_memory_trailer.md` — close commit carries `Closes memory:` trailer.

**Carry-forwards (still open after NAI-64):**
- `pathing-entity-focus-and-step-tracking` sub-spec (NAI-34-D3, D4, D5-NPC, NAI-41-D-PLAYER-NO-LOCOBJ-TARGETXZ).
- NAI-35-T3-D1 op[1] operability gate audit.
- AI-tick walktrigger consumption (NAI-37-D-WALKTRIGGER-NOREADER + NAI-44-D-PLAYER-WALKTRIGGER-NOOP).
- NAI-40-SB1 OPCALLED convergence (blocked on World.ts:613-642 port).
- NAI-40-SB2 FINDHERO + BOTH_HEROPOINTS (blocked on HeroPoints + hash64 infra).
- NAI-40-SB4 slot-reuse / target-logout detection (defensive-only).
- NAI-44-D-IMMEDIATE-POP-VS-NEXTTARGET (blocked on `p_op*` reshape).
- NAI-44-D-CANACCESS-NO-STUN-CHECK (blocked on stun system port).
- NAI-59-D-MODALTUTORIAL-NO-PRODUCER (conditional on tutorial-content driver).
```

Replace `<T1-SHA>` and `<T2-SHA>` with the actual commit SHAs from
`git log --oneline -n 5` after T2.

### Step 3.2: Final cross-package regression

- [ ] **Run**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./... -count=1
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
```

Expected: all PASS, vet clean, build clean.

### Step 3.3: Final tag-grep

- [ ] **Run**

```
rg -n "NAI-53-D-CLEARCOMLISTENERS-PER-SLOT" pkg/ modules/ cmd/
rg -n "for _, l := range p.invListeners" modules/world/
rg -n "clear\(p.invListeners\)" modules/world/
```

Expected: zero hits for all three.

### Step 3.4: Close commit

- [ ] **Commit**

```bash
git add /home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md
git commit --no-gpg-sign -m "$(cat <<'EOF'
chore(close): NAI-64 — clearComListeners per-slot + invStopListenOnCom packet collapse

Closes NAI-53-D-CLEARCOMLISTENERS-PER-SLOT (per-slot rootLayer-
filtered listener cleanup at CloseModal time, mirroring TS
Player.ts:728-739, 767-789).

Untracked retirement (close-commit provenance, no new tag): the
(*Player).invStopListenOnCom packet-write miss (TS Player.ts:1464-1471).
Goscape's prior implementation only deleted from the map; the packet
fired only at the bulk (*Player).encodeOut clear on refreshModalClose.
Scripts calling inv_stoptransmit(com) standalone (no modal close)
silently stopped server-side updates without notifying the client.
Collapsed in T1 by reshaping invStopListenOnCom to delete-and-write.

Wire-behaviour deltas:
- Sidebar inv listeners survive chat-only modal close (RootLayer != closing slot).
- Script-driven inv_stoptransmit now correctly emits OpUpdateInvStopTransmit.

Net deviation delta: -1.

Closes memory: NAI-53-D-CLEARCOMLISTENERS-PER-SLOT.
EOF
)"
```

---

## Self-review checklist (run after writing this plan)

- [x] **Spec coverage:** Every spec §3-§6 item maps to a step.
  - §3.1 invStopListenOnCom collapse → T1 Step 1.2.
  - §3.2 clearComListeners helper → T1 Step 1.5.
  - §3.3 encodeOut blanket-clear drop → T2 Step 2.3.
  - §3.4 CloseModal per-slot wiring → T2 Step 2.2.
  - §6.1 tests #1-#9 → T1 Steps 1.1, 1.3, 1.6 + T2 Steps 2.1, 2.4, 2.5.
  - §6.2 existing-test updates → T1 Step 1.3 + T2 Step 2.4.
  - §6.3 implementation-time greps → "Pre-flight grep targets" + T2 Step 2.6 + T3 Step 3.3.

- [x] **Placeholder scan:** No "TBD"/"TODO"/"similar to". `<T1-SHA>` / `<T2-SHA>` are concrete tokens to be replaced with actual commit hashes (T3 Step 3.1 spells this out explicitly).

- [x] **Type consistency:**
  - `clearComListeners(rootCom int)` signature uniform across plan.
  - `invStopListenOnCom(com int)` signature unchanged from existing.
  - `seedComponentTypes(t, s, map[int]*objtype.ComponentType)` matches the helper at `modules/world/handler_interface_test.go:132`.
  - `newInvListenerTestPlayer(t, s, slot)` matches `inv_update_test.go:11`.
  - `drainConn(t, c)` matches `stat_update_test.go:59`.
  - `Component.RootLayer` field uniform; `*objtype.ComponentType.RootLayer int` confirmed at `pkg/objtype/componenttype.go:49`.
  - `gameserver.OpUpdateInvStopTransmit` (3-byte wire: 1 opcode + P2 payload) confirmed at `pkg/io/protocol/game/server/prot.go:56`.
  - `gameserver.OpIfClose` (1-byte wire: opcode only, payload size 0) confirmed at `pkg/io/protocol/game/server/prot.go:12`.

- [x] **Migration ordering:** T1 leaves the bulk path live → T2 deletes bulk + activates per-slot atomically. Per `latent_bug_at_migration_boundary.md`. No dual-path window.

- [x] **Test fixture runnability:** Every test code-block uses concrete fixtures via existing helpers (`newInvListenerTestPlayer`, `seedComponentTypes`, `drainConn`, `io2.New`). No invented fixture API. Per `plan_runnable_test_fixtures.md`.

- [x] **Cross-call audit:** `(*Player).InvStopListenOnCom` (exported) is called by `pkg/script/handlers_inv.go:449`. After T1, that script-handler path now writes the packet — verified safe because the script handler runs during `processActiveScripts` where the player has a connected client.
