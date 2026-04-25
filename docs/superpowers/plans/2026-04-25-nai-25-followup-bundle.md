# NAI-25 follow-up bundle Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port the three TS-faithfulness divergences in `(*Player).invListenOnCom` (Bundle 1), then sweep the 13 unaudited handler files for NumberNotNull coverage (Bundle 2), closing the From-NAI-24 and From-NAI-23 tracker entries.

**Architecture:** Two-bundle sequential follow-up. Bundle 1 is a single-method TS-faithfulness audit in `modules/world/player.go`: three branches added (α invType=-1 early-out, β same-type+same-com dedup, γ scope-shared rewrite), plus a test helper, plus four new tests, plus two doc-comment updates. Bundle 2 is a multi-file audit-completion sweep in `pkg/script/handlers_*.go`: per-file audit table per the NAI-23 Bundle 4 cadence; per-file commits where WRAPs are added; one rollup commit for confirm-zero files. Sequential dispatch — Bundle 2 only after Bundle 1's commit lands.

**Tech Stack:** Go 1.26+. Existing helpers: `newTestPlayer` (`modules/world/player_test.go:14`), `newTestServer` (`modules/world/server_test.go:308`), `discardLogger` (referenced from existing tests), `checkNotNull` (`pkg/script/handlers_player.go:61`), `objtype.InvTypeScopeShared` constant (`pkg/objtype/invtype.go:13`), `invLookupView.Get` scope-aware lookup pattern (`modules/world/server_invs.go:26-32`). TS source root at `/home/owner/Code/github.com/LostCityRS/Engine-TS/src/engine/`. Reference TS method body at `Engine-TS/src/engine/entity/Player.ts:1441-1462`.

**Spec reference:** `docs/superpowers/specs/2026-04-25-nai-25-followup-bundle-design.md`.

---

## Task 1 — Bundle 1: `(*Player).invListenOnCom` TS-faithfulness audit

**Files:**
- Modify: `modules/world/player.go:627-646` (doc-comment + method body — three new branches)
- Modify: `modules/world/player_inv_test.go:36-38` (existing-test docstring tighten); append 4 new tests at file end
- Modify: `modules/world/server_test.go:308-316` (add new helper `newTestPlayerWithInvTypes` after `newTestServer`)
- Modify: `pkg/script/active.go:277-283` (interface contract doc-comment update — no signature change)

**Pre-flight context:**
- HEAD `0db9f2a` (after spec polish commit). Verify all line numbers via re-grep at task time per `controller_preflight` memory.
- `(*Player).invListenOnCom` body at `modules/world/player.go:636-646` enters NAI-25 with NO TS-faithfulness branches. The method just does lazy-init + map overwrite. The three new branches are the entire surface change.
- `objtype` import in `modules/world/player.go`: verify at task time via `head -20 modules/world/player.go`. If absent, add `"github.com/zsrv/goscape/pkg/objtype"` to the import block.
- `(*Server).invTypes` field path: verified accessible via `p.client.server.invTypes` based on `invLookupView.Get` reading `v.s.invTypes` at `modules/world/server_invs.go:16`. The field type is `*objtype.InvTypeConfigs` (per `pkg/objtype/invtype.go:89-96`), with `Configs []*InvType` slice indexed by typeID.
- Existing test helper `newTestPlayer(t *testing.T) (*Player, net.Conn)` at `modules/world/player_test.go:14-27` constructs a Player with a client but **no server** wired (`p.client.server` is nil). Existing tests in `player_inv_test.go` rely on this — they pass `-1` directly as `source` and never reach the (γ) scope-rewrite branch. The new helper `newTestPlayerWithInvTypes` extends this by wiring a Server with `invTypes` populated.
- Cross-package pin pre-flight at spec-write confirmed only 4 files in `modules/world/` reference the listener path: `player_inv_test.go` (controlled, will gain 4 new tests), `player.go` (production, will be edited), `inv_update_test.go:90` (struct-literal `InventoryListener`, NOT an `invListenOnCom` call — unaffected), `modal_close_test.go:14` (struct-literal `InventoryListener`, NOT an `invListenOnCom` call — unaffected). Re-run cross-package grep at task time.

- [ ] **Step 1: Pre-flight verification — file paths, line numbers, helper init state**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go version
grep -n "objtype" /home/owner/Code/github.com/zsrv/goscape/modules/world/player.go | head -5
grep -n "func (p \*Player) invListenOnCom" /home/owner/Code/github.com/zsrv/goscape/modules/world/player.go
grep -n "func (p \*Player) updateInvs" /home/owner/Code/github.com/zsrv/goscape/modules/world/player.go
grep -n "InvListenOnCom" /home/owner/Code/github.com/zsrv/goscape/pkg/script/active.go
grep -n "func newTestPlayer\|func newTestServer" /home/owner/Code/github.com/zsrv/goscape/modules/world/*.go
grep -rn "invListenOnCom\|InvListenOnCom" /home/owner/Code/github.com/zsrv/goscape/modules/world/ /home/owner/Code/github.com/zsrv/goscape/pkg/
```

Record: confirmed line numbers for the 4 files in scope; confirmed cross-package pin set has not grown since spec-write (`player_inv_test.go`, `player.go`, `active.go`, plus the 2 struct-literal-only files).

If any line number drifted, update the rest of the steps' line citations accordingly. If a new cross-package pin appears, ESCALATE — the spec did not anticipate it.

- [ ] **Step 2: TDD cycle for (α) — write the failing early-out test**

Append to `modules/world/player_inv_test.go` (after the existing `TestInvStopListenOnComNoopForNilMap` at line 131):

```go
// TestInvListenOnComEarlyOutOnInvalidInvType pins the TS Player.ts:1442-1444
// early-out: invType=-1 means invalid; the listener registration is a no-op
// and the map is not allocated.
func TestInvListenOnComEarlyOutOnInvalidInvType(t *testing.T) {
	p, _ := newTestPlayer(t)

	p.invListenOnCom(-1, 149, 0)

	if len(p.invListeners) != 0 {
		t.Errorf("len: got %d, want 0 (early-out should not register)", len(p.invListeners))
	}
	if p.invListeners != nil {
		t.Error("invListeners should remain nil — early-out should not allocate the map")
	}
}
```

Run to verify FAIL:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run "TestInvListenOnComEarlyOutOnInvalidInvType" ./modules/world/ -v
```

Expected: **FAIL** — the existing method does not have the early-out, so calling `invListenOnCom(-1, 149, 0)` will allocate the map and store an entry with `Type: -1`. The first assertion `len != 0` will trip.

- [ ] **Step 3: Implement (α) early-out and verify the test PASSES**

Edit `modules/world/player.go`. The current `invListenOnCom` body at lines 636-646 is:

```go
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
```

Add the early-out as the first branch (matching TS Player.ts:1442-1444 order):

```go
func (p *Player) invListenOnCom(invType, com, source int) {
	if invType == -1 {
		return
	}
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
```

Run to verify PASS:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run "TestInvListenOnComEarlyOutOnInvalidInvType" ./modules/world/ -v
```

Expected: **PASS**.

Run all existing tests in the file to verify no regression:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run "TestInvListenOnCom\|TestInvStopListenOnCom" ./modules/world/ -v
```

Expected: all 5 existing tests PASS + the 1 new test PASSES.

- [ ] **Step 4: TDD cycle for (β) — write the failing same-type+same-com dedup test**

Append to `modules/world/player_inv_test.go`:

```go
// TestInvListenOnComDedupsSameTypeSameCom pins the TS Player.ts:1446-1449
// dedup: a second invListenOnCom call with the same (Type, Com) is a no-op
// — FirstSeen state is preserved across redundant calls so that a redundant
// inv_transmit does not force a re-emit.
func TestInvListenOnComDedupsSameTypeSameCom(t *testing.T) {
	p, _ := newTestPlayer(t)

	p.invListenOnCom(93, 149, -1)
	// Simulate a first-seen emit flipping FirstSeen to false.
	l := p.invListeners[149]
	l.FirstSeen = false
	p.invListeners[149] = l

	// Re-register with the SAME Type and Com — should be a no-op (preserves FirstSeen=false).
	p.invListenOnCom(93, 149, -1)

	if len(p.invListeners) != 1 {
		t.Fatalf("len: got %d, want 1 (dedup should not add a second entry)", len(p.invListeners))
	}
	got := p.invListeners[149]
	if got.FirstSeen {
		t.Error("FirstSeen should remain false after redundant invListenOnCom on same (Type, Com)")
	}
	if got.Type != 93 {
		t.Errorf("Type: got %d, want 93", got.Type)
	}
}
```

Run to verify FAIL:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run "TestInvListenOnComDedupsSameTypeSameCom" ./modules/world/ -v
```

Expected: **FAIL** — the existing method (post-Step-3, with only the (α) early-out) still unconditionally replaces the entry, so the second call resets `FirstSeen` to true.

- [ ] **Step 5: Implement (β) same-type+same-com dedup and verify the test PASSES**

Edit `modules/world/player.go` `invListenOnCom`. Insert the dedup branch after the (α) early-out, before the lazy-init (matching TS Player.ts:1446-1449 order):

```go
func (p *Player) invListenOnCom(invType, com, source int) {
	if invType == -1 {
		return
	}
	if existing, ok := p.invListeners[com]; ok && existing.Type == invType {
		return
	}
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
```

The map-keyed lookup naturally returns `ok=false` when `p.invListeners` is nil, so the dedup check is safe to run before the lazy-init. Re-ordering preserves both the TS source order (early-out → dedup → push) AND the existing nil-map safety.

Run to verify PASS:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run "TestInvListenOnComDedupsSameTypeSameCom" ./modules/world/ -v
```

Expected: **PASS**.

Run the full invListen test set to verify no regression:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run "TestInvListenOnCom\|TestInvStopListenOnCom" ./modules/world/ -v
```

Expected: all 6 existing tests + 2 new tests PASS. In particular `TestInvListenOnComReplacesExisting` (which calls with DIFFERENT types: 93 then 100) still PASSES because the dedup only fires on SAME types.

- [ ] **Step 6: Add the `newTestPlayerWithInvTypes` test helper in `server_test.go`**

Edit `modules/world/server_test.go`. Insert the new helper immediately after the existing `newTestServer` function (currently ends at line 316):

```go
// newTestPlayerWithInvTypes constructs a Player wired to a Server whose
// invTypes is populated with the given configs. Used by scope-aware tests
// of (*Player).invListenOnCom that exercise the SCOPE_SHARED rewrite
// branch (γ). For tests that don't need invTypes wiring, use newTestPlayer.
func newTestPlayerWithInvTypes(t *testing.T, configs []*objtype.InvType) (*Player, net.Conn) {
	t.Helper()
	p, conn := newTestPlayer(t)
	s := newTestServer(t)
	s.invTypes = &objtype.InvTypeConfigs{Configs: configs}
	p.client.server = s
	return p, conn
}
```

Add the `objtype` import to `server_test.go` if not already present. Run a quick check:

```bash
grep -n "objtype" /home/owner/Code/github.com/zsrv/goscape/modules/world/server_test.go | head -3
```

If absent, add `"github.com/zsrv/goscape/pkg/objtype"` to the existing import block in `server_test.go`.

Compile-check:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./modules/world/...
```

Expected: build succeeds. The helper is unused so far (consumers come in Step 7); a compile failure here means the import is missing or the field name `invTypes` is wrong — verify `(*Server).invTypes` field via `grep -n "invTypes" /home/owner/Code/github.com/zsrv/goscape/modules/world/server.go` and adjust.

- [ ] **Step 7: TDD cycle for (γ) — write the failing scope-rewrite tests (positive + negative)**

Append to `modules/world/player_inv_test.go`:

```go
// TestInvListenOnComRewritesSourceForSharedScope pins the TS Player.ts:1456-1459
// scope-rewrite: when invType has SCOPE_SHARED scope, the registration method
// rewrites source = -1 internally regardless of what the caller passed.
func TestInvListenOnComRewritesSourceForSharedScope(t *testing.T) {
	configs := make([]*objtype.InvType, 50)
	configs[42] = &objtype.InvType{Scope: objtype.InvTypeScopeShared}
	p, _ := newTestPlayerWithInvTypes(t, configs)

	// Caller passes source=99; the SCOPE_SHARED rewrite should override to -1.
	p.invListenOnCom(42, 149, 99)

	if len(p.invListeners) != 1 {
		t.Fatalf("len: got %d, want 1", len(p.invListeners))
	}
	got := p.invListeners[149]
	if got.Source != -1 {
		t.Errorf("Source: got %d, want -1 (SCOPE_SHARED should rewrite)", got.Source)
	}
	if got.Type != 42 {
		t.Errorf("Type: got %d, want 42", got.Type)
	}
}

// TestInvListenOnComKeepsSourceForNonSharedScope pins the negative case of the
// scope-rewrite: when invType has non-SCOPE_SHARED scope (perm/temp), the
// caller-passed source is preserved.
func TestInvListenOnComKeepsSourceForNonSharedScope(t *testing.T) {
	configs := make([]*objtype.InvType, 50)
	configs[42] = &objtype.InvType{Scope: objtype.InvTypeScopePerm}
	p, _ := newTestPlayerWithInvTypes(t, configs)

	p.invListenOnCom(42, 149, 99)

	if len(p.invListeners) != 1 {
		t.Fatalf("len: got %d, want 1", len(p.invListeners))
	}
	got := p.invListeners[149]
	if got.Source != 99 {
		t.Errorf("Source: got %d, want 99 (SCOPE_PERM should preserve caller source)", got.Source)
	}
}
```

Run to verify FAIL:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run "TestInvListenOnComRewritesSourceForSharedScope\|TestInvListenOnComKeepsSourceForNonSharedScope" ./modules/world/ -v
```

Expected: **`TestInvListenOnComRewritesSourceForSharedScope` FAILS** with `Source: got 99, want -1` (no scope rewrite yet). **`TestInvListenOnComKeepsSourceForNonSharedScope` PASSES** because the existing method already preserves the caller-passed source when no rewrite happens (negative test passes for the wrong reason currently — but it pins the post-implementation behavior).

If both pass before implementation: a previous step accidentally added scope-aware logic. Investigate before continuing.

- [ ] **Step 8: Implement (γ) scope-shared rewrite and verify both tests PASS**

Add the `objtype` import to `modules/world/player.go` if not already present:

```bash
grep -n "objtype" /home/owner/Code/github.com/zsrv/goscape/modules/world/player.go | head -3
```

If absent, add `"github.com/zsrv/goscape/pkg/objtype"` to the existing import block in `player.go`.

Edit `modules/world/player.go` `invListenOnCom`. Insert the scope-rewrite branch after the dedup, before the lazy-init (matching TS Player.ts:1456-1459 order):

```go
func (p *Player) invListenOnCom(invType, com, source int) {
	if invType == -1 {
		return
	}
	if existing, ok := p.invListeners[com]; ok && existing.Type == invType {
		return
	}
	if p.client != nil && p.client.server != nil && p.client.server.invTypes != nil {
		if cfg := p.client.server.invTypes.Configs[invType]; cfg != nil && cfg.Scope == objtype.InvTypeScopeShared {
			source = -1
		}
	}
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
```

The lookup chain `p.client != nil && p.client.server != nil && p.client.server.invTypes != nil` gracefully degrades when server wiring is absent (the `newTestPlayer` direct-call paths). The nil-cfg guard handles `Configs[]` indexing where the type ID exceeds the configured range. The pattern mirrors `invLookupView.Get` at `modules/world/server_invs.go:26-32`.

**Bounds-check note**: Go panics on `Configs[invType]` when `invType >= len(Configs)`. The (α) early-out handles `invType == -1`. For positive `invType` exceeding `len(Configs)`, the index expression panics. Add a bounds check defensively:

```go
if p.client != nil && p.client.server != nil && p.client.server.invTypes != nil {
	configs := p.client.server.invTypes.Configs
	if invType < len(configs) {
		if cfg := configs[invType]; cfg != nil && cfg.Scope == objtype.InvTypeScopeShared {
			source = -1
		}
	}
}
```

Run the (γ) tests to verify PASS:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run "TestInvListenOnComRewritesSourceForSharedScope\|TestInvListenOnComKeepsSourceForNonSharedScope" ./modules/world/ -v
```

Expected: **both PASS**.

Run the full invListen test set:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run "TestInvListenOnCom\|TestInvStopListenOnCom" ./modules/world/ -v
```

Expected: all 6 existing tests + 4 new tests PASS (10 total).

- [ ] **Step 9: Update `(*Player).invListenOnCom` doc-comment**

Edit `modules/world/player.go` lines 627-635 (the doc-comment immediately preceding the method). Replace existing brevity:

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
```

with:

```go
// invListenOnCom registers an inventory listener at the given interface
// component ID, matching TS Player.ts:1441-1462 line-by-line.
//
// Behavior:
//   - invType == -1 → no-op (early-out matches TS).
//   - existing listener at com with same Type → no-op (preserves
//     FirstSeen state across redundant inv_transmit calls).
//   - SCOPE_SHARED inv-type → source rewritten to -1 (world-shared
//     dispatch); requires p.client.server.invTypes wired. Graceful
//     no-op when wiring is absent (test-direct-call paths).
//   - Otherwise → store {Type, Com, Source, FirstSeen=true}; the map
//     overwrite naturally implements TS's same-com-different-type
//     splice.
//
// Source = -1 → world-shared inventory (Server.invs[Type]).
// Source >= 0 → another player's slot (Server.players[Source].invs[Type]).
//
// Lazy-initializes the invListeners map on first call.
func (p *Player) invListenOnCom(invType, com, source int) {
```

Compile-check:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./modules/world/...
```

Expected: build succeeds.

- [ ] **Step 10: Update `ActivePlayer.InvListenOnCom` interface contract doc-comment**

Edit `pkg/script/active.go`. The current contract at lines 277-283 reads:

```go
	// InvListenOnCom registers an inventory listener at UI component id
	// `com` tracking inv type `invType`. `source == -1` means the
	// world-shared inventory; `source >= 0` means the player at that server
	// slot. Replaces any existing listener at com; resets FirstSeen=true
	// on replace. Safe when the implementation's listener map is still nil
	// — it must lazy-init.
	InvListenOnCom(invType, com, source int)
```

Replace with:

```go
	// InvListenOnCom registers an inventory listener at UI component id
	// `com` tracking inv type `invType`. Callers pass the player's own
	// UID (via ActivePlayer.UID()) or a popped uid for INV_OTHERTRANSMIT
	// scenarios; the implementation rewrites source to -1 internally
	// when invType has SCOPE_SHARED scope (matches TS Player.ts:1456-1459).
	// On the dispatch side, source == -1 routes to the world-shared
	// inventory; source >= 0 routes to the player at that server slot.
	// Replaces any existing listener at com unless the existing entry
	// has the same type (in which case the call is a no-op preserving
	// FirstSeen state). Safe when the implementation's listener map is
	// still nil — it must lazy-init.
	InvListenOnCom(invType, com, source int)
```

No signature change. Doc-only.

Compile-check:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
```

Expected: build succeeds (interface-implementation conformance unchanged).

- [ ] **Step 11: Tighten `TestInvListenOnComReplacesExisting` docstring**

Edit `modules/world/player_inv_test.go` lines 36-38. The current docstring reads:

```go
// TestInvListenOnComReplacesExisting verifies that a second call with
// the same com overwrites the first entry and resets FirstSeen to true,
// matches TS Player.ts:1441-1462 add-or-replace semantics.
func TestInvListenOnComReplacesExisting(t *testing.T) {
```

Replace with:

```go
// TestInvListenOnComReplacesExisting verifies that a second call with
// the same com but a DIFFERENT type overwrites the first entry and
// resets FirstSeen to true. Matches TS Player.ts:1457-1460 same-com-
// different-type splice (the (β) dedup at TS:1446-1449 does NOT apply
// because the types differ; that's pinned separately by
// TestInvListenOnComDedupsSameTypeSameCom).
func TestInvListenOnComReplacesExisting(t *testing.T) {
```

The test body is unchanged — calls with type=93 then type=100 still test the splice path correctly.

- [ ] **Step 12: Run the full repo test suite for cross-package regression check**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
```

Expected: **PASS** across the whole repo. Per `verify_implementer_claims` memory: package-scoped green can mask cross-package breakage; the full-repo run is the cross-check.

If any pre-existing test fails: investigate. Likely candidate is a test that (i) registered an invListener twice with the same type and com expecting a re-emit (relied on the pre-NAI-25 (β) behavior — would now fail because the second call is no-op'd), or (ii) assumed `Source` would be preserved as caller-passed for an inv-type that turns out to be SCOPE_SHARED in test fixtures (the (γ) rewrite would now flip `Source` to -1). For (i): update the test to call `invStopListenOnCom` first or assert the new TS-faithful no-op behavior. For (ii): file a new tracker entry (this is the cascade scenario from spec § Risks) and ESCALATE to the controller.

- [ ] **Step 13: Commit Bundle 1**

```bash
git add modules/world/player.go modules/world/player_inv_test.go modules/world/server_test.go pkg/script/active.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-25 Bundle 1 — (*Player).invListenOnCom TS-faithfulness audit

Port the three missing TS-faithfulness divergences in
(*Player).invListenOnCom to match Engine-TS Player.ts:1441-1462
line-by-line:

(α) Early-out on invType=-1 (TS Player.ts:1442-1444). Matches the
    invalid-inv sentinel TS short-circuits on. Defended upstream by
    InvTypeValid via NAI-23 Bundle 4b but the internal-method
    defense is TS-faithful at trivial cost.
(β) Same-type+same-com dedup (TS Player.ts:1446-1449). Preserves
    FirstSeen state across redundant inv_transmit calls; previously
    every call reset FirstSeen=true and forced a re-emit.
(γ) Scope-shared rewrite (TS Player.ts:1456-1459). When invType has
    SCOPE_SHARED scope, source is rewritten to -1 internally so the
    dispatch reader at updateInvs (player.go:471-479) routes to the
    world-shared store. Lookup gracefully degrades when server
    wiring is absent (newTestPlayer direct-call paths).

Brainstorm reframing: the From-NAI-24 tracker entry framed the -1
API surface as "dead production code; decide retract vs preserve."
Investigation against TS source revealed the API surface is NOT
dead — its write-side scope-rewrite was never ported. The -1 branch
on the read side at updateInvs has no live producer because the
producing transformer (the scope rewrite) was missing. After this
fix, the -1 source-routing branch has a live producer for
SCOPE_SHARED inv-types.

Files:
- modules/world/player.go: invListenOnCom body (3 new branches with
  bounds-check on Configs[invType] indexing); doc-comment narrates
  all four branches and the graceful-degradation behavior.
- modules/world/player_inv_test.go: 4 new tests
  (TestInvListenOnComEarlyOutOnInvalidInvType,
  TestInvListenOnComDedupsSameTypeSameCom,
  TestInvListenOnComRewritesSourceForSharedScope,
  TestInvListenOnComKeepsSourceForNonSharedScope); existing
  TestInvListenOnComReplacesExisting docstring tightened to clarify
  it tests the same-com-DIFFERENT-type splice path.
- modules/world/server_test.go: new helper newTestPlayerWithInvTypes
  for scope-aware tests; co-located with newTestServer.
- pkg/script/active.go: ActivePlayer.InvListenOnCom interface
  contract docstring updated to narrate the rewrite behavior; no
  signature change.

Closes the From-NAI-24 tracker entry's intent ("API surface decision")
via TS-faithful porting (not retraction). Tracker entry at
nai_followups.md:1471-1514 to be marked Resolved at NAI-25 close
commit.

Net deviation count unchanged (14).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2 — Bundle 2: NumberNotNull audit-completion sweep across remaining handler files

**Files:**
- Modify (per audit yield): `pkg/script/handlers_config.go`, `pkg/script/handlers_dialog.go`, `pkg/script/handlers_timer.go` (production WRAPs)
- Modify (per audit yield): `pkg/script/handlers_config_test.go`, `pkg/script/handlers_dialog_test.go`, `pkg/script/handlers_timer_test.go`, `pkg/script/handlers_lastinput_test.go` (per-WRAP `TestHandle<OpName>NullRejected` tests)
- Read-only audit: `pkg/script/handlers_loc.go`, `handlers_obj.go`, `handlers_db.go`, `handlers_string.go`, `handlers_server.go`, `handlers_core.go`, `handlers_debug.go`, `handlers_number.go`, `handlers_vars.go`, `handlers_array.go` (confirm-zero rollup; no production change)

**Pre-flight context:**
- HEAD `<Bundle 1 commit hash>` (post-Task-1). Verify line numbers via re-grep at task time per `controller_preflight` memory.
- Existing `checkNotNull` helper at `pkg/script/handlers_player.go:61` is the canonical wrap; reuse without redefining. Op-name string convention (verified across handlers_npc/inv/interface/player_test.go): underscored uppercase opcode mnemonic, e.g., `"NPC_FINDALLZONE"`, `"SETTIMER"`, `"LAST_COM"`.
- Test naming convention (project standard): `TestHandle<OpName>NullRejected`. Use this exact form.
- TS file mapping (per spec § Bundle 2 § "TS source canonical paths"):
  - `handlers_config.go` → `NpcConfigOps.ts` (2 NumberNotNull) + `LocConfigOps.ts` (0) + `ObjConfigOps.ts` (0)
  - `handlers_dialog.go` → `PlayerOps.ts` subset (P_PAUSEBUTTON, P_COUNTDIALOG, LAST_*, CAM_*, STAFFMODLEVEL, UID)
  - `handlers_timer.go` → `PlayerOps.ts` subset (SETTIMER, SOFTTIMER, CLEARTIMER, CLEARSOFTTIMER, GETTIMER)
  - `handlers_vars.go` → `CoreOps.ts` subset (PUSH/POP_VAR{P,S,N}); 0 NumberNotNull
  - `handlers_array.go` → `CoreOps.ts` subset (DEFINE_ARRAY, PUSH/POP_ARRAY_INT, SWITCH); 0 NumberNotNull
  - `handlers_loc.go` → `LocOps.ts`; 0 NumberNotNull
  - `handlers_obj.go` → `ObjOps.ts`; 0 NumberNotNull
  - `handlers_db.go` → `DbOps.ts`; 0 NumberNotNull
  - `handlers_string.go` → `StringOps.ts`; 0 NumberNotNull
  - `handlers_server.go` → `ServerOps.ts`; 0 NumberNotNull
  - `handlers_core.go` → `CoreOps.ts` (file-wide minus var/array subsets); 0 NumberNotNull
  - `handlers_debug.go` → `DebugOps.ts`; 0 NumberNotNull
  - `handlers_number.go` → `NumberOps.ts`; 0 NumberNotNull
  - `handlers_lastinput_test.go` → handlers physically live in `handlers_dialog.go`; covered by dialog audit.
- PlayerOps.ts cross-file residue math: PlayerOps.ts has 56 NumberNotNull; NAI-24 Bundle 1 (commit `85da016`) audited 47 popInt sites in `handlers_player.go`. The 9-site delta lives in PlayerOps.ts opcodes that goscape dispatches from a different file. Bundle 2 implementer enumerates and assigns each delta opcode to its goscape handler file.

**Per-pop-site decision rubric** (verbatim from spec § Bundle 2 § "Per-file audit-pass cadence"):

1. **TS wraps with `check(state.popInt(), NumberNotNull)`** → **WRAP**. Add `if err := checkNotNull(v, "OP_NAME"); err != nil { return err }` immediately after the `s.PopInt()` that produces the value. Add a `TestHandle<OpName>NullRejected` test.
2. **TS wraps with a typed validator** (InvTypeValid, CoordValid, PlayerStatValid, LocAngleValid, SeqTypeValid, EnumTypeValid, NpcTypeValid, etc.) → **SKIP**. goscape's existing path already routes through an equivalent typed validator. Document in audit table.
3. **TS does not wrap the popped value at all** → **SKIP (TS not wrapped)**. Preserve TS tolerance.
4. **Popped value is semantically signed** (coord delta, search-relative offset, arithmetic operand) → **SKIP (signed value)**. `-1` is legitimate.
5. **TS uses NumberNotNull but goscape's existing path is structurally different** (e.g., goscape pre-validates upstream) → **ESCALATE**. File as `NAI-25-D<n>` deviation tag with rationale.

- [ ] **Step 1: Pre-flight verification + PlayerOps.ts cross-file residue enumeration**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go version
git log --oneline -3
ls /home/owner/Code/github.com/zsrv/goscape/pkg/script/handlers_*.go
grep -n "checkNotNull" /home/owner/Code/github.com/zsrv/goscape/pkg/script/handlers_player.go | head -5
```

Verify Bundle 1 commit landed. Verify the 13 Bundle-2 production files all exist. Verify `checkNotNull` helper still at `handlers_player.go:61`.

Enumerate all 56 PlayerOps.ts NumberNotNull sites with their opcode names:

```bash
grep -n "NumberNotNull" /home/owner/Code/github.com/LostCityRS/Engine-TS/src/engine/script/handlers/PlayerOps.ts > /tmp/playerops_numnull_sites.txt
wc -l /tmp/playerops_numnull_sites.txt
cat /tmp/playerops_numnull_sites.txt
```

Expected: 56 lines. For each line, identify the enclosing TS opcode case (`case ScriptOpcode.OP_NAME:`) by reading TS upward from the NumberNotNull site.

Cross-reference NAI-24 Bundle 1's audit table:

```bash
git show 85da016 | grep -E "^\| handle" | head -60
```

The 47 audited handlers map to opcodes whose TS NumberNotNull sites NAI-24 visited. Subtract from the 56-site enumeration → produces the 9-site delta.

For each delta opcode, find its goscape handler-file home:

```bash
# For each delta opcode <OPCODE_NAME>:
grep -l "Op<OpcodeName>" /home/owner/Code/github.com/zsrv/goscape/pkg/script/handlers_*.go
grep -n "Op<OpcodeName>" /home/owner/Code/github.com/zsrv/goscape/pkg/script/handlers.go
```

Record: per-delta-opcode home file. Most are expected to land in `handlers_dialog.go` (LAST_*, CAM_*, P_PAUSE*, etc.) or `handlers_timer.go` (SETTIMER, SOFTTIMER, CLEAR*, GETTIMER). If any delta opcode has no matching `func handle<OpName>` in any goscape handler file, that means goscape hasn't ported the opcode yet — record as "not yet ported in goscape; out of scope."

Output: a delta-opcode table with columns `(TS opcode, TS file:line, goscape handler file, status)`. This table feeds Steps 3 and 4.

- [ ] **Step 2: Audit `handlers_config.go` against NpcConfigOps.ts (2 NumberNotNull sites expected)**

Enumerate popInt sites:

```bash
grep -nE "s\.PopInt\(\)|s\.PopInts\(" /home/owner/Code/github.com/zsrv/goscape/pkg/script/handlers_config.go
```

Read `NpcConfigOps.ts` NumberNotNull sites:

```bash
grep -n "NumberNotNull" /home/owner/Code/github.com/LostCityRS/Engine-TS/src/engine/script/handlers/NpcConfigOps.ts
grep -n "NumberNotNull" /home/owner/Code/github.com/LostCityRS/Engine-TS/src/engine/script/handlers/LocConfigOps.ts
grep -n "NumberNotNull" /home/owner/Code/github.com/LostCityRS/Engine-TS/src/engine/script/handlers/ObjConfigOps.ts
```

For each `handlers_config.go` popInt site, apply the rubric. Build the audit table:

| Handler | popInt context | TS wraps? | Decision | Rationale (TS file:line) |
|---------|---------------|-----------|----------|-------------------------|
| handleSomeConfigOp | configID | NumberNotNull | WRAP | NpcConfigOps.ts:NN |
| ... | ... | ... | ... | ... |

For each WRAP row, write the failing null-pin test in `handlers_config_test.go`. Follow the existing test-file scaffolding (read 5 existing tests in the file as templates):

```bash
head -40 /home/owner/Code/github.com/zsrv/goscape/pkg/script/handlers_config_test.go
grep -n "func Test" /home/owner/Code/github.com/zsrv/goscape/pkg/script/handlers_config_test.go | head -10
```

If the file uses the `mp := &mockPlayer{}` + inline `ScriptFile` pattern (matching the NAI-24 Bundle 1 precedent in `handlers_player_test.go`), adapt the test template:

```go
func TestHandle<OpName>NullRejected(t *testing.T) {
    mp := &mockPlayer{}
    sf := &ScriptFile{
        Name: "<lowercase_opname>_null",
        Opcodes: []Opcode{
            OpPushConstantInt, // value = -1
            Op<OpEnumValue>,
            OpReturn,
        },
        IntOperands: []int32{-1, 0, 0},
    }
    state := Init(sf, mp, false, nil, nil)
    err := Execute(state)
    if err == nil {
        t.Fatalf("Execute: want error for value=-1, got nil")
    }
    want := "<OP_NAME>: input number was null(-1)"
    if !strings.Contains(err.Error(), want) {
        t.Errorf("error: got %q, want substring %q", err.Error(), want)
    }
}
```

Verify all new tests FAIL:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run "NullRejected" ./pkg/script/ -v
```

Expected: every newly added test fails with no-error-returned. (Existing NullRejected tests for already-wrapped sites continue to PASS.)

Add `checkNotNull` wraps to `handlers_config.go` per WRAP rows. Wrap shape:

```go
v := s.PopInt()
if err := checkNotNull(v, "OP_NAME"); err != nil {
    return err
}
```

Verify the new tests PASS + no regression:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -v
```

Expected: **PASS** across `pkg/script/`.

**If audit produced 0 WRAPs** (both sites SKIP-eligible, e.g., NpcTypeValid covers them): no production change to handlers_config.go; no new tests; this file folds into the rollup commit at Step 6 instead of getting its own commit. Skip the next two sub-steps and proceed to Step 3.

**If audit produced ≥1 WRAP** — commit `handlers_config.go` as its own commit:

```bash
git add pkg/script/handlers_config.go pkg/script/handlers_config_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-25 Bundle 2 — handlers_config.go NumberNotNull audit

Audit pass per NAI-25 spec § Bundle 2: every s.PopInt() in
handlers_config.go is checked against its TS counterpart in
NpcConfigOps.ts / LocConfigOps.ts / ObjConfigOps.ts. Sites where
TS wraps with check(state.popInt(), NumberNotNull) gain a goscape
checkNotNull wrap; signed-value sites and TS-unwrapped sites stay
raw with recorded rationale.

N net new wraps across M handlers; K sites SKIPped (rationale per
audit table). N new TestHandle<OpName>NullRejected tests follow
the handlers_player_test.go shape (post-NAI-24).

Per-handler audit table:
[paste the audit table built in Step 2]

Skip-reason breakdown:
- Typed-validator (NpcTypeValid / etc.): K1
- Signed sentinel: K2
- TS does not wrap: K3

Net deviation count unchanged (14).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

- [ ] **Step 3: Audit `handlers_dialog.go` against PlayerOps.ts cross-file residue**

Read the Step 1 delta-opcode table. Filter to opcodes whose goscape home is `handlers_dialog.go`. These are the candidate audit rows for this file.

Enumerate popInt sites in `handlers_dialog.go`:

```bash
grep -nE "s\.PopInt\(\)|s\.PopInts\(" /home/owner/Code/github.com/zsrv/goscape/pkg/script/handlers_dialog.go
```

For each popInt site whose enclosing handler matches a delta opcode (per Step 1 table), apply the rubric anchored to the corresponding PlayerOps.ts NumberNotNull site. For popInt sites whose enclosing handler is NOT in the delta table (i.e., the opcode's TS counterpart has no NumberNotNull), the audit row is SKIP (TS not wrapped) by definition.

Build the audit table per the standard format. Identify WRAP candidates.

For each WRAP candidate:
1. Write the failing test in `handlers_dialog_test.go` (or `handlers_lastinput_test.go` if the handler's test scaffold is partitioned there — check the existing partition first via `grep -l "func TestHandle<OpName>" pkg/script/handlers_dialog_test.go pkg/script/handlers_lastinput_test.go`).
2. Verify FAIL.
3. Add the `checkNotNull` wrap to `handlers_dialog.go`.
4. Verify PASS.

**If audit produced 0 WRAPs** (no delta opcodes land in dialog, or all delta opcodes' sites SKIP-eligible): defer this file to the rollup commit at Step 6.

**If audit produced ≥1 WRAP** — commit `handlers_dialog.go` as its own commit:

```bash
git add pkg/script/handlers_dialog.go pkg/script/handlers_dialog_test.go
# Add handlers_lastinput_test.go to the stage if any of the new tests landed there:
git add pkg/script/handlers_lastinput_test.go 2>/dev/null || true
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-25 Bundle 2 — handlers_dialog.go NumberNotNull audit

Audit pass per NAI-25 spec § Bundle 2 § "PlayerOps.ts cross-file
residue cross-check". Re-grep of PlayerOps.ts confirmed N
NumberNotNull sites for opcodes that goscape dispatches from
handlers_dialog.go — the cross-file residue NAI-24 Bundle 1's
file-scoped audit (commit 85da016) didn't visit.

N net new wraps across M handlers; K sites SKIPped per audit
table. N new TestHandle<OpName>NullRejected tests.

Per-handler audit table:
[paste the audit table built in Step 3]

Skip-reason breakdown:
- Typed-validator: K1
- Signed sentinel: K2
- TS does not wrap: K3

Net deviation count unchanged (14).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

- [ ] **Step 4: Audit `handlers_timer.go` against PlayerOps.ts cross-file residue**

Same procedure as Step 3, but filtered to opcodes whose goscape home is `handlers_timer.go` (SETTIMER, SOFTTIMER, CLEARTIMER, CLEARSOFTTIMER, GETTIMER).

Enumerate popInt sites:

```bash
grep -nE "s\.PopInt\(\)|s\.PopInts\(" /home/owner/Code/github.com/zsrv/goscape/pkg/script/handlers_timer.go
```

Apply the rubric. Build the audit table.

For each WRAP candidate: write failing test in `handlers_timer_test.go`, verify FAIL, add wrap, verify PASS.

**If audit produced 0 WRAPs**: defer this file to the rollup commit at Step 6.

**If audit produced ≥1 WRAP** — commit `handlers_timer.go` as its own commit (commit message template: same shape as Step 3, substituting `handlers_timer.go` for `handlers_dialog.go`).

- [ ] **Step 5: Confirm-zero audit pass on the 10 zero-density files**

For each of the following files, verify that the corresponding TS file has 0 NumberNotNull (or, for `handlers_core.go` / `handlers_vars.go` / `handlers_array.go` whose TS counterpart is a CoreOps.ts subset, that the relevant TS subset has 0 NumberNotNull) and that every goscape popInt site is consequently a confirm-zero row.

Files to audit:
- `handlers_loc.go` ↔ LocOps.ts
- `handlers_obj.go` ↔ ObjOps.ts
- `handlers_db.go` ↔ DbOps.ts
- `handlers_string.go` ↔ StringOps.ts
- `handlers_server.go` ↔ ServerOps.ts
- `handlers_core.go` ↔ CoreOps.ts (file-wide minus var/array subsets)
- `handlers_debug.go` ↔ DebugOps.ts
- `handlers_number.go` ↔ NumberOps.ts
- `handlers_vars.go` ↔ CoreOps.ts (PUSH/POP_VAR{P,S,N} subset)
- `handlers_array.go` ↔ CoreOps.ts (DEFINE_ARRAY, PUSH/POP_ARRAY_INT, SWITCH subset)

For each file:

```bash
# 1. Re-confirm TS NumberNotNull count
grep -c "NumberNotNull" /home/owner/Code/github.com/LostCityRS/Engine-TS/src/engine/script/handlers/<TS_File>.ts
# 2. Count goscape popInt sites
grep -cE "s\.PopInt\(\)|s\.PopInts\(" /home/owner/Code/github.com/zsrv/goscape/pkg/script/<file>.go
```

Build the rollup table:

```
| goscape file        | TS counterpart   | popInt sites | TS NumberNotNull | Conclusion |
|---------------------|------------------|--------------|------------------|------------|
| handlers_loc.go     | LocOps.ts        | <N>          | 0                | No-op      |
| handlers_obj.go     | ObjOps.ts        | <N>          | 0                | No-op      |
| handlers_db.go      | DbOps.ts         | <N>          | 0                | No-op      |
| handlers_string.go  | StringOps.ts     | <N>          | 0                | No-op      |
| handlers_server.go  | ServerOps.ts     | <N>          | 0                | No-op      |
| handlers_core.go    | CoreOps.ts       | <N>          | 0                | No-op      |
| handlers_debug.go   | DebugOps.ts      | <N>          | 0                | No-op      |
| handlers_number.go  | NumberOps.ts     | <N>          | 0                | No-op      |
| handlers_vars.go    | CoreOps.ts       | <N>          | 0                | No-op      |
| handlers_array.go   | CoreOps.ts       | <N>          | 0                | No-op      |
```

If any file's TS counterpart turns out to have a NumberNotNull (i.e., an upstream TS change since spec-write): ESCALATE. Apply the audit rubric to that file as in Steps 2-4 (write failing test, add wrap, verify PASS) and add a row to the rollup commit's body explaining the surprise + remediation.

If Steps 3 and 4 deferred their files to the rollup (zero WRAPs found), append `handlers_dialog.go` and/or `handlers_timer.go` rows to the rollup table per the same format.

No production changes to any file in this step. The rollup commit captures the audit archaeology only.

- [ ] **Step 6: Run the full repo test suite for cross-package regression check**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
```

Expected: **PASS** across the whole repo.

If any pre-existing test fails: investigate. The most likely cause is a wrap that was inserted in the wrong logical position relative to a side-effect, or an op-name string collision with another handler's test fixture. Do NOT silence the failure; restore TS-faithful behavior or ESCALATE.

- [ ] **Step 7: Rollup commit (confirm-zero archaeology + any deferred files)**

Stage any unstaged production files (none expected unless a Step 5 ESCALATE produced a wrap):

```bash
git status --short
```

If Step 5 produced a wrap-and-test for an upstream-TS-changed file, stage those changes here. Otherwise this commit is doc-only (the audit table lives in the commit body).

```bash
git commit --no-gpg-sign --allow-empty -m "$(cat <<'EOF'
feat(script): NAI-25 Bundle 2 — confirm-zero rollup across N handler files

Audit-completion sweep per NAI-25 spec § Bundle 2. Closes the
From-NAI-23 NumberNotNull-sweep tracker entry by completing the
audit pass on every unaudited goscape handler file.

Per-file confirm-zero results:

[paste the rollup table built in Step 5 — include any deferred
files from Steps 3-4 if their audits produced 0 WRAPs]

Per-file wrap-and-test commits in this bundle (own commits):
- handlers_config.go (Step 2): N WRAPs / commit <hash>
- handlers_dialog.go (Step 3, if non-zero): N WRAPs / commit <hash>
- handlers_timer.go (Step 4, if non-zero): N WRAPs / commit <hash>

PlayerOps.ts cross-file residue accounting:
- 56 NumberNotNull in PlayerOps.ts
- 47 audited in NAI-24 Bundle 1 (commit 85da016) for handlers_player.go
- 9-site delta enumerated and assigned (per Step 1 table):
  - <K1> sites in handlers_dialog.go (Step 3 audit)
  - <K2> sites in handlers_timer.go (Step 4 audit)
  - <K3> sites in <other files> (handled at <Step 5 audit row OR ESCALATE>)
  - <K4> sites for opcodes not yet ported in goscape (out of scope)

Net deviation count unchanged (14).

Closes the From-NAI-23 entry at nai_followups.md:1409-1446 — to be
marked Resolved at the NAI-25 close commit.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Two-stage review checkpoint (post-Bundle-2)

After both bundles land, dispatch two-stage review per `runescript_cadence` memory:

- **Stage 1 (spec compliance)** — fresh opus subagent compares each bundle's commit(s) against the spec § Bundle N section. Bundle 1: each of α/β/γ branches lands in correct order (early-out → dedup → splice (implicit via map overwrite) → scope rewrite → push); the four new tests pin the new behaviors with the helper or no-helper as the test table specifies; doc-comment narration matches spec. Bundle 2: per-pop-site audit decisions cross-checked against TS (every WRAP row's TS file:line opens to a `check(..., NumberNotNull)`); PlayerOps.ts cross-file residue distribution sanity-checked against the 56-47=9 math; rollup table is complete (all 10 confirm-zero files + any deferred dialog/timer files have rows).
- **Stage 2 (code quality)** — fresh opus subagent reviews for naming consistency (`TestHandle<OpName>NullRejected` form everywhere), idiomatic Go (the (γ) bounds-check chain reads cleanly), test-helper reuse (`newTestPlayerWithInvTypes` has 2 named consumers; no shipped-with-zero-consumers helpers), audit-table consistency across files, missed cross-package pins, doc-comment narrative consistency, dead-API leftovers.

Each stage is a single subagent dispatch. Polish commits land **before** the close commit if review surfaces remediable findings (per NAI-23 / NAI-24 precedent: `polish(world): NAI-25 close polish` style; `polish(script):` if Bundle 2 polish only).

---

## Close commit

Once both bundles + reviews + any polish commits have landed, append the close commit:

1. **Update `nai_followups.md`**:
   - Mark the From-NAI-24 entry at `:1471-1514` Resolved with the Bundle 1 commit hash. Resolution narrative: "API surface not dead — write-side scope-rewrite was missing in `(*Player).invListenOnCom`. Ported to match TS Player.ts:1441-1462 line-by-line; (α) early-out, (β) same-type+same-com dedup, (γ) scope-shared rewrite. The `-1` source-routing branch in `updateInvs` now has a live producer for SCOPE_SHARED inv-types." Preserve original body.
   - Mark the From-NAI-23 entry at `:1409-1446` Resolved with the Bundle 2 commit hashes. Resolution narrative: "All 13 unaudited handler files swept. Per-file results: handlers_config.go (<N> WRAPs / commit <hash>); handlers_dialog.go (<N> WRAPs / commit <hash>); handlers_timer.go (<N> WRAPs / commit <hash>); 10 confirm-zero files in rollup commit `<hash>`. PlayerOps.ts cross-file residue audit confirmed: <delta>/9 sites accounted for in {dialog, timer, <other>}; remaining sites <reason>." Preserve original body.
   - Append a new `## From NAI-25 (2026-04-25)` section if any tracker-worthy items surfaced during the bundles (e.g., upstream-TS-changed file ESCALATE in Step 5; cascade from Bundle 1 (γ) port). If no new items, omit this section.

2. **Save memory entries** per `post_task_handoff` memory. Re-evaluate the brainstorm-time pre-flagged candidates:
   - `audit_full_method_against_ts` — likely save (generalizable; (α/β/γ) discovery pattern).
   - `file_scoped_audits_miss_cross_file_ts` — likely save (directly actionable; PlayerOps.ts 56-47=9 math).
   - `prenarrowed_candidates_benefit_from_fresh_density_data` — re-evaluate vs `controller_preflight` overlap.
   - `tracker_entry_framing_can_be_incomplete` — re-evaluate vs `spec_followup_tracker_freshness` overlap.

3. **Stage and commit** (memory file is outside the working tree; no git stage):

```bash
git commit --no-gpg-sign --allow-empty -m "$(cat <<'EOF'
chore(world,script): NAI-25 closed — two-bundle follow-up

Closes the From-NAI-24 (*Player).invListenOnCom API-surface deferral
and the From-NAI-23 NumberNotNull audit-completion sweep target list.

Bundle 1 (feat): (*Player).invListenOnCom TS-faithfulness audit.
Three TS-faithfulness divergences ported (α invType=-1 early-out,
β same-type+same-com dedup, γ scope-shared rewrite); 4 new tests +
1 helper; doc-comments updated on player.go and active.go. Brainstorm
reframing: API surface not dead — write-side rewrite was missing.
Bundle 2 (feat): NumberNotNull audit-completion sweep across 13
unaudited handler files. <N> per-file commits where audit produced
WRAPs; 1 rollup commit for confirm-zero archaeology. PlayerOps.ts
cross-file residue (56-47=9 sites) enumerated and assigned.

Net deviation count: 14 → 14.

Closes memory: nai_followups.md:1471-1514 (From-NAI-24 Source = -1 API surface, Resolved by NAI-25 Bundle 1)
Closes memory: nai_followups.md:1409-1446 (From-NAI-23 NumberNotNull sweep targets, Resolved by NAI-25 Bundle 2)

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Plan self-review

**Spec coverage:** Every spec section maps to a task or step:
- Spec § Bundle 1 (α/β/γ divergences + tests + helper + doc-comments) → Task 1 Steps 1-13.
- Spec § Bundle 1 § "Test strategy" (4 named tests) → Task 1 Steps 2, 4, 7 (failing-test code blocks for each).
- Spec § Bundle 1 § "Doc-comment update" → Task 1 Step 9.
- Spec § Bundle 1 § "Interface contract update" → Task 1 Step 10.
- Spec § Bundle 1 § "Touch points" #2 (tighten existing test docstring) → Task 1 Step 11.
- Spec § Bundle 1 § "Touch points" #5 (tracker resolution) → Close commit § "Update nai_followups.md" #1.
- Spec § Bundle 2 (per-file audit + rollup) → Task 2 Steps 1-7.
- Spec § Bundle 2 § "PlayerOps.ts cross-file residue cross-check" → Task 2 Step 1 (the math step) + Steps 3-4 (audit application).
- Spec § Bundle 2 § "Per-file audit-pass cadence" → Task 2 Steps 2-5 (the cadence applied per file).
- Spec § Bundle 2 § "Commit organization" → Task 2 Steps 2/3/4 (own-commit decisions) + Step 7 (rollup commit).
- Spec § "Out-of-scope" #1-5 → no plan tasks (correctly deferred per spec).
- Spec § "Risks & mitigations" → Task 1 Step 1 + Task 2 Step 1 pre-flight verifications; Task 1 Step 12 + Task 2 Step 6 full-repo regression checks; Task 1 Step 8 graceful-degradation lookup chain; Task 2 ESCALATE rubric (rubric rule 5).
- Spec § "Review structure" → Two-stage review checkpoint section.
- Spec § "NAI-25 close" → Close commit section.
- Spec § "Memory entry candidates pre-flagged" → Close commit § "Save memory entries" #2.

**Placeholder scan:** No forbidden patterns ("TBD", "TODO", "implement later", "fill in details", "appropriate error handling"). The Bundle 2 commit message templates use `N` / `M` / `K` / `K1` / `K2` / `K3` / `[paste the audit table from Step N]` placeholders intentionally because the audit IS the work — the implementer fills them with discovered counts at task time (matches NAI-23 Bundle 4 plan and NAI-24 Bundle 1 plan precedent). Op-name string templates (`<OpName>`, `<OP_NAME>`, `<OpEnumValue>`) are template variables for the per-WRAP test generation. The `<Bundle 1 commit hash>` in Task 2 pre-flight context is a forward-reference filled at Bundle 2 dispatch time after Bundle 1 lands.

**Type consistency:** Across both tasks, the production call signature `invListenOnCom(invType, com, source int)` and its consumers are referenced consistently. The new helper signature `newTestPlayerWithInvTypes(t *testing.T, configs []*objtype.InvType) (*Player, net.Conn)` is consistent across the spec, the helper definition in Step 6, and the test-table consumers in Step 7. The four new test names (`TestInvListenOnComEarlyOutOnInvalidInvType`, `TestInvListenOnComDedupsSameTypeSameCom`, `TestInvListenOnComRewritesSourceForSharedScope`, `TestInvListenOnComKeepsSourceForNonSharedScope`) are spelled identically across spec, plan steps, and commit message. The `objtype.InvTypeScopeShared` and `objtype.InvTypeScopePerm` constants are referenced consistently. The (γ) production-code lookup chain matches the spec verbatim including the bounds-check refinement explicitly added in Step 8.

**Plan-test-coverage crosscheck** (per `plan_test_coverage_crosscheck` memory):
- Bundle 1: spec mandates "4 new tests pinning α, β, γ-positive, γ-negative" → plan codifies all 4 tests with full Go source in Steps 2/4/7. Production fixes are 3 (α, β, γ); γ gets 2 tests (positive + negative); test count is 4 / fixes 3 = sound coverage with γ getting both directions.
- Bundle 2: spec mandates "1 negative-pin test per WRAP" → plan codifies this in Step 2's WRAP-loop with the test template. Test count is bounded by the audit-table WRAP count (per-file). Per-file expected-test-count: handlers_config.go ≤ 2 (NpcConfigOps NumberNotNull count); handlers_dialog.go + handlers_timer.go ≤ 9 combined (PlayerOps cross-file residue distribution).

**Plan-runnable-test-fixture crosscheck** (per `plan_runnable_test_fixtures` memory):
- Task 1 Steps 2, 4, 7 codify the failing tests as runnable Go blocks. The (γ) tests in Step 7 use `make([]*objtype.InvType, 50)` to allocate a slice large enough that index 42 is safe; index 42 is then populated with a concrete `&objtype.InvType{Scope: ...}` literal. Other indices remain nil — the (γ) production code's `cfg != nil` guard handles this correctly.
- Task 2 Step 2 test template uses the verified existing pattern (`mp := &mockPlayer{}` + inline `ScriptFile` + `Init` + `Execute`). The `IntOperands: []int32{-1, 0, 0}` 3-slot layout matches the NAI-24 Bundle 1 template precedent. `InstructionCount` is omitted from the template — verify whether existing tests in each handlers_*_test.go file include it before generating per-file tests; if so, add the field.
- Task 1 Step 7 helper has 2 named consumers (the 2 (γ) tests in the same step). Per `plan_helper_coverage` memory: not a flag-set verification target.

**Helper-pattern crosscheck** (per `plan_grep_helper_patterns` memory): 
- Bundle 1 reuses `newTestPlayer` (exists), `newTestServer` (exists), `discardLogger` (exists, transitively via the helpers). The new `newTestPlayerWithInvTypes` is a NEW helper, justified because its 2 consumers exercise a scope-aware path that no existing helper supports. No inline boilerplate is prescribed when a helper exists.
- Bundle 2 reuses `checkNotNull` (exists at `handlers_player.go:61`); the WRAP shape is verbatim from the existing handlers_player.go pattern.

**Enumerate-all-sites crosscheck** (per `enumerate_all_sites` memory): 
- Bundle 1: Task 1 Step 1's `grep -rn "invListenOnCom\|InvListenOnCom" pkg/ modules/` re-runs the cross-package pin search at task time. Spec-write-time grep results are recorded in the pre-flight context for comparison.
- Bundle 2: Task 2 Step 1's PlayerOps.ts re-enumeration covers the cross-file residue. Steps 2-4 grep each TS counterpart fresh at audit time.

**Spec-followup-tracker-freshness** (per `spec_followup_tracker_freshness` memory): tracker entry assertions verified at HEAD `0db9f2a` (post-spec polish commit, pre-Bundle-1). Re-verified again at task dispatch via Step 1 grep commands.

**Controller-preflight discipline**: every implementer dispatch begins with a Step 1 pre-flight verification (file paths, line numbers, helper init state, cross-package pins).

No issues found.
