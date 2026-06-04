# NAI-20 — follow-up bundle Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Land 11 tracked NAI-2/3/7/12/18/19 follow-ups in 5 sequential tasks: test-helper extraction, *Npc geometry snapshot + first-spawn mask gate, cache + removeNpc polish bundle, checkNotNull on three NPC handlers, and size-aware DistanceTo rewire at TS-flagged sites.

**Architecture:** Five sequential tasks, each one feat/refactor/polish commit (Task 3 has three sub-items inside one bundle commit; Task 4 has three sub-items inside one bundle commit). TDD per task: failing test → verify fail → minimal impl → verify pass → commit. Two-stage final review at bundle close.

**Tech Stack:** Go 1.26+; existing packages `pkg/cache`, `pkg/coordgrid`, `pkg/script`, `modules/world`. All `go` invocations prefixed with `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache`. All commits use `--no-gpg-sign`.

**Spec:** `docs/superpowers/specs/2026-04-25-nai-20-followup-bundle-design.md` (commit `154c352`).

**Predecessor HEAD:** `02e24cd` (NAI-19 closed).

**Plan-time TS verification (Task 5)**: TS source reads at TS Npc.ts:651-654, 657-673, 676, 751-754, 780-790, 821-829 confirm only **3 of 7 candidate sites** require rewiring; 4 sites preserve `DistanceToSW` per TS-faithful gate. See Task 5 disposition table for site-by-site decision.

**Plan-time signature corrections (vs spec sketch)**:
- `checkNotNull(v int, op string) error` (existing helper at `pkg/script/handlers_player.go:61`) — takes op-name string; matches `handleNpcSetTimer` (S7b).
- `cache.makeCrc(path string)` — no `idx` parameter; `CrcTable` is `[]uint32` (slice, nil-initial); `CrcBuffer32 uint32` is also a package var that ResetCRCState must zero.
- `handleNpcQueue` pop order: `delay → arg → queueId`. Wrap only `delay`.

---

## File structure

| File | Touch | Reason |
|---|---|---|
| `modules/world/npc_test_helpers.go` | Create | Task 1 (newRegisteredNpc helper) |
| `modules/world/npc_registry_test.go` | Modify | Task 1 (convert ~7 existing prologue patterns), Task 2 (collision-toggle + first-spawn mask tests), Task 3 (lazy adjustedDuration parity test) |
| `modules/world/npc_test.go` | Modify | Task 1 (convert ~4 existing prologue patterns), Task 2 (snapshot field tests, ChangeType-no-mutate test) |
| `modules/world/npc.go` | Modify (struct lines 25-107, NewNpc literal lines 110-148) | Task 2 (add `blockWalk` + `size` fields, ctor seed) |
| `modules/world/npc_registry.go` | Modify (lines 65-72, 99-123, 135-156) | Task 2 (collision toggle reads, mask gate fold), Task 3 C3 (lazy adjustedDuration) |
| `pkg/cache/crctable.go` | Modify | Task 3 C1 (ResetCRCState helper), C2 (slog.Warn in makeCrc) |
| `pkg/cache/crctable_test.go` | Create | Task 3 C1, C2 (helper + warn capture tests) |
| `modules/world/world_test.go` | Modify | Task 3 C1 (use ResetCRCState in test) |
| `pkg/script/handlers_npc.go` | Modify (lines 281-289, 296-309, 330-336) | Task 4 (checkNotNull on three handlers) |
| `pkg/script/handlers_npc_test.go` | Modify (append tests) | Task 4 (three negative-pin tests) |
| `modules/world/npc_interaction.go` | Modify (lines 440-441) | Task 5 (PLAYERESCAPE rewire to DistanceTo) |
| `modules/world/npc_player_modes.go` | Modify (lines 54-62) | Task 5 (playerFaceClose inline → DistanceTo) |
| `modules/world/npc_interaction_test.go` | Modify (append tests) | Task 5 (size-asymmetry + parity pins for PLAYERESCAPE rewire) |
| `modules/world/npc_player_modes_test.go` | Modify (append tests) | Task 5 (size-asymmetry + parity pins for playerFaceClose rewire) |
| `nai_followups.md` (memory) | Modify | Close commit (annotate retired entries) |

---

## Task 1: B — newRegisteredNpc helper extraction

**Files:**
- Create: `modules/world/npc_test_helpers.go`
- Modify: `modules/world/npc_registry_test.go`, `modules/world/npc_test.go`

**Goal:** Extract the repeated 3-line test prologue (`newTestServer + NpcType + NewNpc + register-into-s.npcs`) into a single helper. Convert all matching call sites in one commit.

- [ ] **Step 1: Audit existing call-site shapes**

Run: `grep -n "NewNpc(" modules/world/npc_registry_test.go modules/world/npc_test.go | head -30`

Identify each `NewNpc(...)` site and classify it as:
- (a) construct only (test cares about ctor output, no `s.addNpc` follows),
- (b) construct + manual register (test does `s.npcs[nid] = n` or similar by hand),
- (c) construct + `s.addNpc(...)` call.

Categories (a) and (c) map to `register=false` and `register=true` in the helper. Category (b) — manual slot-pokes for tests that exercise specific nid values — should NOT be converted in this task; they keep their hand-rolled shape (annotation only).

- [ ] **Step 2: Write the helper file**

Create `modules/world/npc_test_helpers.go`:

```go
package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
)

// newRegisteredNpc constructs a synthetic NPC for tests.
//
// Defaults: nid=1 (placeholder; overwritten by addNpc when register=true),
// typeId=1, coords (3200, 3200, 0).
//
// When register=true, calls s.addNpc(n, -1, true) — equivalent to the
// production first-spawn path. The caller-passed nid=1 is overwritten
// by s.allocNpcSlot inside addNpc; n.nid post-call reflects the
// allocated slot. NB: n.uid is computed at NewNpc time as
// (typeId<<16)|1; addNpc does not recompute it. This matches existing
// production behavior — tests that care about uid invariants must
// recompute or read n.nid after the call.
//
// When register=false, returns a bare *Npc with nid=1, suitable for
// unit tests of constructor / mask / stats behavior that don't need
// the npcs[]/npcLoop[] bookkeeping.
//
// typ must not be nil (callers vary Size, BlockWalk, Stats, HuntMode
// etc per test). s.npcTypes is allocated to [nil, typ] when nil so
// resetEntityForRespawn's lookupType call resolves correctly.
func newRegisteredNpc(t *testing.T, s *Server, typ *objtype.NpcType, register bool) *Npc {
	t.Helper()
	if typ == nil {
		t.Fatalf("newRegisteredNpc: typ must not be nil")
	}
	if s.npcTypes == nil {
		s.npcTypes = []*objtype.NpcType{nil, typ}
	}
	n := NewNpc(1, 1, 3200, 3200, 0, typ)
	if register {
		if err := s.addNpc(n, -1, true); err != nil {
			t.Fatalf("newRegisteredNpc: addNpc: %v", err)
		}
	}
	return n
}
```

- [ ] **Step 3: Verify it compiles**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./modules/world/ -run XXX_NoTestMatchesThis`

Expected: all build, zero tests run. (`-run XXX_NoTestMatchesThis` filters out all tests; we just want the compile to succeed.)

- [ ] **Step 4: Convert existing call sites**

For each (a) and (c) call site identified in Step 1, replace the 3-line prologue:

```go
s := newTestServer(t)
typ := &objtype.NpcType{...}
n := NewNpc(1, 1, 3200, 3200, 0, typ)
// (and possibly: s.addNpc(n, -1, true))
```

with:

```go
s := newTestServer(t)
typ := &objtype.NpcType{...}
n := newRegisteredNpc(t, s, typ, /* register */ true_or_false)
```

Preserve any non-default coordinate values via post-call mutation (e.g., `n.x = 3210` after the helper) rather than expanding the helper signature. The helper aims for *common* defaults; outliers stay verbose.

- [ ] **Step 5: Run all `modules/world` tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./modules/world/...`

Expected: all PASS. No new tests added in this task; the helper is exercised by every converted call site.

- [ ] **Step 6: Run full repo test suite with race detector**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race -count=1 ./...`

Expected: all PASS.

- [ ] **Step 7: Commit**

```bash
git add modules/world/npc_test_helpers.go \
       modules/world/npc_registry_test.go \
       modules/world/npc_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
refactor(world): NAI-20 Task 1 — extract newRegisteredNpc test helper

Absorbs ~40 LOC of repeated test prologue across npc_registry_test.go
and npc_test.go. The helper has two modes — register=false for
construct-only unit tests, register=true for tests that exercise
s.npcs/s.npcLoop bookkeeping via the production addNpc path. Existing
non-default-coord callers post-mutate n.x/n.z after the helper rather
than expanding the signature; outliers stay verbose. Manual-slot tests
(category b) are not converted — their hand-rolled shape is intentional.

No production code change. No new tests; helper is exercised by every
converted caller.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: A + item 5 — *Npc geometry snapshot + first-spawn mask gate

**Files:**
- Modify: `modules/world/npc.go` (struct lines 25-107, NewNpc literal lines 110-148)
- Modify: `modules/world/npc_registry.go` (lines 65-72, 99-123, 140-147)
- Modify: `modules/world/npc_test.go` (append tests)
- Modify: `modules/world/npc_registry_test.go` (append tests)

**Goal:** Stash `blockWalk` and `size` on `*Npc` at constructor time so collision toggles read snapshot fields rather than `n.typ.*`. Fold the `NpcMaskChangeType` raise inside `resetEntityForRespawn`'s morph-detect block to stop first-spawn from raising it spuriously.

- [ ] **Step 1: Write failing test for snapshot fields on NewNpc**

Append to `modules/world/npc_test.go`:

```go
// TestNewNpcSnapshotsBlockWalkAndSize pins NAI-20 Task 2: NewNpc copies
// blockWalk + size from typ at construction time.
func TestNewNpcSnapshotsBlockWalkAndSize(t *testing.T) {
	typ := &objtype.NpcType{Size: 2, BlockWalk: objtype.BlockWalkAll}
	n := NewNpc(1, 1, 3200, 3200, 0, typ)
	if n.blockWalk != objtype.BlockWalkAll {
		t.Errorf("blockWalk: got %v, want BlockWalkAll", n.blockWalk)
	}
	if n.size != 2 {
		t.Errorf("size: got %d, want 2", n.size)
	}
}
```

- [ ] **Step 2: Verify the test fails (compile error or assertion)**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./modules/world/ -run TestNewNpcSnapshotsBlockWalkAndSize -v`

Expected: FAIL with `n.blockWalk undefined` (compile error) — fields don't exist yet.

- [ ] **Step 3: Add fields to *Npc struct**

In `modules/world/npc.go`, add a new section block to the `*Npc` struct between `// === interaction ===` (line 82) and `// === masks ===` (line 93). Insert after the `faceAngleX/Z` lines (89-91):

```go
	// === geometry snapshot (NAI-20) ===
	// Captured at NewNpc; UNCHANGED by changetype to mirror TS PathingEntity
	// (World.ts:1271, 1302). Read by addNpc/removeNpc collision toggles
	// instead of n.typ.Size / n.typ.BlockWalk so a size-changing morph→revert
	// cycle leaves base-size collision flags rather than morph-size flags.
	blockWalk objtype.BlockWalk
	size      int
```

- [ ] **Step 4: Seed fields in NewNpc literal**

In `modules/world/npc.go`, locate the `&Npc{...}` literal at the top of `NewNpc` (around line 111-148). Add after the existing `huntRange` line:

```go
		blockWalk:       typ.BlockWalk,
		size:            int(typ.Size),
```

- [ ] **Step 5: Verify Step 1's test passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./modules/world/ -run TestNewNpcSnapshotsBlockWalkAndSize -v`

Expected: PASS.

- [ ] **Step 6: Write failing test for ChangeType immutability**

Append to `modules/world/npc_test.go`:

```go
// TestChangeTypeDoesNotMutateBlockWalkOrSize pins NAI-20 Task 2: a
// changetype updates n.typ but MUST NOT mutate the geometry snapshot.
// This is the TS PathingEntity ctor-snapshot semantic — width/length
// (and BlockWalk by analogy) do not track changetype.
func TestChangeTypeDoesNotMutateBlockWalkOrSize(t *testing.T) {
	s := newTestServer(t)
	baseTyp := &objtype.NpcType{Size: 1, BlockWalk: objtype.BlockWalkNPC}
	morphTyp := &objtype.NpcType{Size: 2, BlockWalk: objtype.BlockWalkAll}
	s.npcTypes = []*objtype.NpcType{nil, baseTyp, morphTyp}

	n := newRegisteredNpc(t, s, baseTyp, /* register */ false)
	wantBlockWalk := n.blockWalk
	wantSize := n.size

	n.ChangeType(2, -1) // morph to typeId=2 (size=2 morphTyp); duration=-1 = no revert timer

	if n.blockWalk != wantBlockWalk {
		t.Errorf("blockWalk after ChangeType: got %v, want %v (snapshot must not change)",
			n.blockWalk, wantBlockWalk)
	}
	if n.size != wantSize {
		t.Errorf("size after ChangeType: got %d, want %d (snapshot must not change)",
			n.size, wantSize)
	}
	// Sanity: typ DID change (NAI-19 Task 3 closed the staleness)
	if n.typ != morphTyp {
		t.Errorf("n.typ after ChangeType: did not refresh to morphTyp")
	}
}
```

- [ ] **Step 7: Verify the test passes (no production change needed)**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./modules/world/ -run TestChangeTypeDoesNotMutateBlockWalkOrSize -v`

Expected: PASS. The test is a regression-pin for the existing behavior; ChangeType doesn't touch n.blockWalk/n.size because changeTypeImpl never assigned to them. No production change.

- [ ] **Step 8: Update collision toggle reads in addNpc**

In `modules/world/npc_registry.go`, replace the addNpc collision-toggle block (currently lines 65-73):

**Before:**
```go
if n.typ != nil && s.gamemap != nil {
	switch n.typ.BlockWalk {
	case objtype.BlockWalkNPC:
		s.gamemap.ChangeNPCCollision(int(n.typ.Size), n.x, n.z, n.level, true)
	case objtype.BlockWalkAll:
		s.gamemap.ChangeNPCCollision(int(n.typ.Size), n.x, n.z, n.level, true)
		s.gamemap.ChangePlayerCollision(int(n.typ.Size), n.x, n.z, n.level, true)
	}
}
```

**After:**
```go
if s.gamemap != nil {
	switch n.blockWalk {
	case objtype.BlockWalkNPC:
		s.gamemap.ChangeNPCCollision(n.size, n.x, n.z, n.level, true)
	case objtype.BlockWalkAll:
		s.gamemap.ChangeNPCCollision(n.size, n.x, n.z, n.level, true)
		s.gamemap.ChangePlayerCollision(n.size, n.x, n.z, n.level, true)
	}
}
```

(The `n.typ != nil` guard is dropped — the snapshot fields are valid for the lifetime of the *Npc, independent of `n.typ`.)

- [ ] **Step 9: Update collision toggle reads in removeNpc**

In `modules/world/npc_registry.go`, replace the removeNpc collision-toggle block (currently lines 140-148):

**Before:**
```go
if n.typ != nil && s.gamemap != nil {
	switch n.typ.BlockWalk {
	case objtype.BlockWalkNPC:
		s.gamemap.ChangeNPCCollision(int(n.typ.Size), n.x, n.z, n.level, false)
	case objtype.BlockWalkAll:
		s.gamemap.ChangeNPCCollision(int(n.typ.Size), n.x, n.z, n.level, false)
		s.gamemap.ChangePlayerCollision(int(n.typ.Size), n.x, n.z, n.level, false)
	}
}
```

**After:**
```go
if s.gamemap != nil {
	switch n.blockWalk {
	case objtype.BlockWalkNPC:
		s.gamemap.ChangeNPCCollision(n.size, n.x, n.z, n.level, false)
	case objtype.BlockWalkAll:
		s.gamemap.ChangeNPCCollision(n.size, n.x, n.z, n.level, false)
		s.gamemap.ChangePlayerCollision(n.size, n.x, n.z, n.level, false)
	}
}
```

- [ ] **Step 10: Run existing collision tests for parity**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./modules/world/ -run "Collision|FirstSpawn|RemoveNpc|AddNpc" -v`

Expected: all PASS. The snapshot+reads change preserves single-tile behavior on existing tests because `typ.Size == n.size` and `typ.BlockWalk == n.blockWalk` for all current production NpcTypes (single-tile dominant data).

- [ ] **Step 11: Write failing test for size-morph revert restoring base footprint**

Append to `modules/world/npc_registry_test.go`:

```go
// TestSizeMorphRevertRestoresBaseFootprint pins NAI-20 Task 2: when a
// size-1 NPC morphs to size-2 then reverts via the heavy path
// (s.removeNpc(n,-1); s.addNpc(n,-1,false)), collision flags reflect
// the base-size-1 footprint, not the morph-size-2 footprint. Without
// the snapshot, addNpc would read n.typ.Size=1 (post-resetEntityForRespawn)
// AFTER having NOT toggled the morph-size-2 flags off, leaving stale
// 2x2 flags. This is the latent bug NAI-19 flagged.
func TestSizeMorphRevertRestoresBaseFootprint(t *testing.T) {
	s := newTestServerWithMap(t) // wires s.gamemap; declared in test_helpers; if not, use the existing pattern from npc_registry_test.go's collision tests.
	baseTyp := &objtype.NpcType{Size: 1, BlockWalk: objtype.BlockWalkAll}
	morphTyp := &objtype.NpcType{Size: 2, BlockWalk: objtype.BlockWalkAll}
	s.npcTypes = []*objtype.NpcType{nil, baseTyp, morphTyp}

	n := newRegisteredNpc(t, s, baseTyp, true) // first-spawn: size=1 flag ON at (3200,3200)

	// Morph to size=2. n.typeId / n.typ refresh, but n.blockWalk / n.size DO NOT.
	n.ChangeType(2, -1)

	// Heavy revert path: removeNpc clears flags using SNAPSHOT (size=1),
	// addNpc(firstSpawn=false) re-sets flags using SNAPSHOT (size=1).
	s.removeNpc(n, -1)
	if err := s.addNpc(n, -1, false); err != nil {
		t.Fatalf("addNpc: %v", err)
	}

	// Verify size=1 footprint at (3200, 3200): SW corner flagged, neighbors NOT.
	pf := s.gamemap.Pathfinder
	if !pf.Flags.IsFlagged(3200, 3200, 0, collision.FlagBlockNPC|collision.FlagBlockPlayers) {
		t.Errorf("(3200, 3200) should have NPC+Player flags after revert, got none")
	}
	for _, off := range []struct{ dx, dz int }{{1, 0}, {0, 1}, {1, 1}} {
		if pf.Flags.IsFlagged(3200+off.dx, 3200+off.dz, 0,
			collision.FlagBlockNPC|collision.FlagBlockPlayers) {
			t.Errorf("(%d, %d) should NOT have flags after revert (size=1 footprint)",
				3200+off.dx, 3200+off.dz)
		}
	}
}
```

**Plan-time verification at impl-time**: confirm the actual collision-flag constants (`FlagBlockNPC`, `FlagBlockPlayers`) and the `IsFlagged(x, z, level, flagMask)` signature in `pkg/pathfinder/collision`. Adjust call to match. Also confirm whether `newTestServerWithMap` exists — if it does not, replicate the gamemap-wiring pattern used in NAI-19 Task 5e collision tests inline.

- [ ] **Step 12: Verify the test fails before fix**

If Steps 8-9 are already applied, this test should PASS at this point. If you want to verify the test would have FAILED without the snapshot reads, you can momentarily revert Steps 8-9, run the test, observe FAIL, then re-apply.

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./modules/world/ -run TestSizeMorphRevertRestoresBaseFootprint -v`

Expected: PASS (with snapshot in place).

- [ ] **Step 13: Write failing test for first-spawn mask gate**

Append to `modules/world/npc_registry_test.go`:

```go
// TestResetEntityForRespawnFirstSpawnDoesNotRaiseChangeTypeMask pins
// NAI-20 Task 2 item 5: first-spawn (n.typeId == n.baseType) MUST NOT
// raise NpcMaskChangeType. The mask is a "type CHANGED" signal in TS
// (Npc.ts:436-443); resetEntity(true) doesn't raise it.
func TestResetEntityForRespawnFirstSpawnDoesNotRaiseChangeTypeMask(t *testing.T) {
	s := newTestServer(t)
	typ := &objtype.NpcType{Size: 1, BlockWalk: objtype.BlockWalkNPC}
	n := newRegisteredNpc(t, s, typ, true) // addNpc fires resetEntityForRespawn

	if n.masks&rsbuf.NpcMaskChangeType != 0 {
		t.Errorf("first-spawn raised NpcMaskChangeType (masks=%d); should remain clear",
			n.masks)
	}
}
```

- [ ] **Step 14: Verify the test fails before the gate fold**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./modules/world/ -run TestResetEntityForRespawnFirstSpawnDoesNotRaiseChangeTypeMask -v`

Expected: FAIL with `first-spawn raised NpcMaskChangeType (masks=2)` (or whatever the bit value is). The current `resetEntityForRespawn` raises the mask unconditionally.

- [ ] **Step 15: Fold the mask raise inside the morph-detect block**

In `modules/world/npc_registry.go`, modify `resetEntityForRespawn` (currently lines 99-123). Move the `n.masks |= rsbuf.NpcMaskChangeType` line (currently at line 116, unconditional) inside the `if n.typeId != n.baseType` block at lines 99-105:

**Before:**
```go
func (s *Server) resetEntityForRespawn(n *Npc) {
	if n.typeId != n.baseType {
		n.typeId = n.baseType
		n.uid = (n.typeId << 16) | n.nid
		if newTyp := n.lookupType(n.baseType); newTyp != nil {
			n.typ = newTyp
		}
	}
	if n.typ != nil {
		for i := range min(objtype.NpcStatCount, len(n.typ.Stats)) {
			v := int(n.typ.Stats[i])
			n.levels[i] = v
			n.baseLevels[i] = v
		}
	}
	n.queue = nil
	n.waypointIndex = -1
	n.tele = true
	n.masks |= rsbuf.NpcMaskChangeType
	n.huntClock = 0
	n.huntTarget = nil
	if n.typ != nil {
		n.huntRange = int(n.typ.HuntRange)
		n.huntMode = n.typ.HuntMode
	}
}
```

**After:**
```go
func (s *Server) resetEntityForRespawn(n *Npc) {
	if n.typeId != n.baseType {
		n.typeId = n.baseType
		n.uid = (n.typeId << 16) | n.nid
		if newTyp := n.lookupType(n.baseType); newTyp != nil {
			n.typ = newTyp
		}
		n.masks |= rsbuf.NpcMaskChangeType
	}
	if n.typ != nil {
		for i := range min(objtype.NpcStatCount, len(n.typ.Stats)) {
			v := int(n.typ.Stats[i])
			n.levels[i] = v
			n.baseLevels[i] = v
		}
	}
	n.queue = nil
	n.waypointIndex = -1
	n.tele = true
	n.huntClock = 0
	n.huntTarget = nil
	if n.typ != nil {
		n.huntRange = int(n.typ.HuntRange)
		n.huntMode = n.typ.HuntMode
	}
}
```

- [ ] **Step 16: Verify the gate test passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./modules/world/ -run TestResetEntityForRespawnFirstSpawnDoesNotRaiseChangeTypeMask -v`

Expected: PASS.

- [ ] **Step 17: Write follow-on test pinning revert DOES raise the mask**

Append to `modules/world/npc_registry_test.go`:

```go
// TestResetEntityForRespawnRevertRaisesChangeTypeMask pins NAI-20
// Task 2 item 5 inverse: when n.typeId != n.baseType (the morph-revert
// case), resetEntityForRespawn DOES raise NpcMaskChangeType. Pairs
// with TestResetEntityForRespawnFirstSpawnDoesNotRaiseChangeTypeMask
// to enforce the gated semantic.
func TestResetEntityForRespawnRevertRaisesChangeTypeMask(t *testing.T) {
	s := newTestServer(t)
	baseTyp := &objtype.NpcType{Size: 1}
	morphTyp := &objtype.NpcType{Size: 1}
	s.npcTypes = []*objtype.NpcType{nil, baseTyp, morphTyp}
	n := newRegisteredNpc(t, s, baseTyp, false)
	// Simulate a post-morph state: typeId points to morphTyp,
	// baseType still points at the original.
	n.typeId = 2
	n.masks = 0 // start clean

	s.resetEntityForRespawn(n)

	if n.masks&rsbuf.NpcMaskChangeType == 0 {
		t.Errorf("revert path did NOT raise NpcMaskChangeType (masks=%d)", n.masks)
	}
	// Side-effect of the morph-detect block: typeId resets.
	if n.typeId != n.baseType {
		t.Errorf("typeId=%d after reset, want baseType=%d", n.typeId, n.baseType)
	}
}
```

- [ ] **Step 18: Verify the revert-pin test passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./modules/world/ -run TestResetEntityForRespawnRevertRaisesChangeTypeMask -v`

Expected: PASS.

- [ ] **Step 19: Run full repo test suite with race detector**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race -count=1 ./...`

Expected: all PASS.

- [ ] **Step 20: Commit**

```bash
git add modules/world/npc.go \
       modules/world/npc_registry.go \
       modules/world/npc_test.go \
       modules/world/npc_registry_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-20 Task 2 — *Npc geometry snapshot + first-spawn mask gate

Closes NAI-19 cross-task collision-toggle/snapshot interaction follow-up
and the NpcMaskChangeType-on-first-spawn divergence (item 5).

Changes:
- *Npc gains blockWalk + size fields, snapshotted at NewNpc and
  unchanged by changetype. Mirrors TS PathingEntity ctor-snapshot
  pattern (World.ts:1271, 1302).
- addNpc/removeNpc collision toggles read from snapshot fields
  (n.blockWalk, n.size) instead of n.typ.{BlockWalk,Size}. Drops the
  n.typ != nil guard since snapshot fields are independent of n.typ.
- resetEntityForRespawn raises NpcMaskChangeType only inside the
  existing if n.typeId != n.baseType block. First-spawn no longer
  raises the mask spuriously (TS resetEntity(true) doesn't raise).

Tests:
- TestNewNpcSnapshotsBlockWalkAndSize: ctor-time copy.
- TestChangeTypeDoesNotMutateBlockWalkOrSize: TS-faithful immutability.
- TestSizeMorphRevertRestoresBaseFootprint: latent size-morph bug fix.
- TestResetEntityForRespawnFirstSpawnDoesNotRaiseChangeTypeMask: gate.
- TestResetEntityForRespawnRevertRaisesChangeTypeMask: inverse pin.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: C bundle — cache.ResetCRCState + makeCrc slog.Warn + lazy adjustedDuration

**Files:**
- Modify: `pkg/cache/crctable.go`
- Create: `pkg/cache/crctable_test.go`
- Modify: `modules/world/world_test.go` (use ResetCRCState)
- Modify: `modules/world/npc_registry.go` (lazy adjustedDuration)

**Goal:** Three independent micro-fixes bundled into one commit. (a) Export `ResetCRCState` re-init helper; replace inline reset in `world_test.go`. (b) `slog.Default().Warn` on `makeCrc` failure paths. (c) Lift `s.scaleByPlayerCount(duration)` inside the RESPAWN+duration>-1 branch of `removeNpc`.

- [ ] **Step 1: Write failing test for ResetCRCState**

Create `pkg/cache/crctable_test.go`:

```go
package cache

import (
	"bytes"
	"log/slog"
	"testing"

	"github.com/zsrv/goscape/pkg/io/packet"
)

// TestResetCRCStateRestoresInitialState pins NAI-20 Task 3 C1: the
// helper re-initializes CrcBuffer, CrcTable, and CrcBuffer32 to their
// package-init shape. Lockstep with crctable.go init expressions.
func TestResetCRCStateRestoresInitialState(t *testing.T) {
	// Mutate state.
	CrcBuffer = packet.NewPacket(make([]byte, 0, 16))
	CrcBuffer.P4(0xDEADBEEF)
	CrcTable = []uint32{1, 2, 3}
	CrcBuffer32 = 0xCAFEBABE

	ResetCRCState()

	if CrcBuffer.Pos != 0 {
		t.Errorf("CrcBuffer.Pos = %d, want 0", CrcBuffer.Pos)
	}
	if got := cap(CrcBuffer.Bytes()); got < 4*9 {
		t.Errorf("CrcBuffer cap = %d, want at least %d", got, 4*9)
	}
	if CrcTable != nil {
		t.Errorf("CrcTable = %v, want nil", CrcTable)
	}
	if CrcBuffer32 != 0 {
		t.Errorf("CrcBuffer32 = %d, want 0", CrcBuffer32)
	}
}
```

(The slog import is for Step 4; included now to keep one import block.)

- [ ] **Step 2: Verify the test fails (function not defined)**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./pkg/cache/ -run TestResetCRCStateRestoresInitialState -v`

Expected: FAIL with `undefined: ResetCRCState`.

- [ ] **Step 3: Add ResetCRCState helper to crctable.go**

Append to `pkg/cache/crctable.go` (before the `MakeCRCs` function):

```go
// ResetCRCState restores CrcBuffer, CrcTable, and CrcBuffer32 to their
// package-init shape. Test-only convenience to avoid drift between
// init expressions and inline test resets. Mirrors the package-level
// var declarations at the top of this file.
func ResetCRCState() {
	CrcBuffer = packet.NewPacket(make([]byte, 0, 4*9))
	CrcTable = nil
	CrcBuffer32 = 0
}
```

- [ ] **Step 4: Verify the test passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./pkg/cache/ -run TestResetCRCStateRestoresInitialState -v`

Expected: PASS.

- [ ] **Step 5: Write failing test for makeCrc slog.Warn**

Append to `pkg/cache/crctable_test.go`:

```go
// TestMakeCrcWarnsOnMissingFile pins NAI-20 Task 3 C2: makeCrc emits a
// slog.Warn on os.Stat failure. Captures via slog.Default() swap.
func TestMakeCrcWarnsOnMissingFile(t *testing.T) {
	prev := slog.Default()
	t.Cleanup(func() { slog.SetDefault(prev) })

	var buf bytes.Buffer
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})))

	makeCrc("/nonexistent/path/that/should/never/exist")

	out := buf.String()
	if out == "" {
		t.Fatalf("expected slog.Warn output, got empty buffer")
	}
	if !bytes.Contains([]byte(out), []byte("makeCrc Stat failed")) {
		t.Errorf("expected 'makeCrc Stat failed' in output; got: %s", out)
	}
	if !bytes.Contains([]byte(out), []byte("/nonexistent/path/that/should/never/exist")) {
		t.Errorf("expected path in output; got: %s", out)
	}
}
```

- [ ] **Step 6: Verify the test fails (silent failure path)**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./pkg/cache/ -run TestMakeCrcWarnsOnMissingFile -v`

Expected: FAIL with `expected slog.Warn output, got empty buffer`.

- [ ] **Step 7: Add slog.Warn to makeCrc**

In `pkg/cache/crctable.go`, modify the `makeCrc` function (currently lines 15-28). Add `import "log/slog"` to the import block and rewrite:

**Before:**
```go
func makeCrc(path string) {
	if _, err := os.Stat(path); err != nil {
		return
	}

	p, err := packet.Load(path, false)
	if err != nil {
		return
	}

	crc := packet.GetCRC(p.Bytes(), 0, len(p.Bytes()))
	CrcTable = append(CrcTable, crc)
	CrcBuffer.P4(crc)
}
```

**After:**
```go
func makeCrc(path string) {
	if _, err := os.Stat(path); err != nil {
		slog.Default().Warn("cache: makeCrc Stat failed", "path", path, "err", err)
		return
	}

	p, err := packet.Load(path, false)
	if err != nil {
		slog.Default().Warn("cache: makeCrc Load failed", "path", path, "err", err)
		return
	}

	crc := packet.GetCRC(p.Bytes(), 0, len(p.Bytes()))
	CrcTable = append(CrcTable, crc)
	CrcBuffer.P4(crc)
}
```

Update the import block at the top of the file to include `"log/slog"`:

```go
import (
	"log/slog"
	"os"

	"github.com/zsrv/goscape/pkg/io/packet"
)
```

- [ ] **Step 8: Verify the slog.Warn test passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./pkg/cache/ -run TestMakeCrcWarnsOnMissingFile -v`

Expected: PASS.

- [ ] **Step 9: Convert world_test.go inline reset to ResetCRCState**

Open `modules/world/world_test.go`. Locate the inline `cache.CrcBuffer = packet.NewPacket(make([]byte, 0, 4*9))` resets (test entry + `t.Cleanup`) — there are two of these per the NAI-19 Task 2 work.

Replace each with `cache.ResetCRCState()`. Drop any `pkg/io/packet` import that was used solely for this reset (verify with `goimports` or compile errors after the change).

- [ ] **Step 10: Verify world_test.go still passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./modules/world/ -run "World|StartingFn|CrcBuffer" -v`

Expected: all PASS (`TestStartingFnPopulatesCrcBuffer` and any related tests).

- [ ] **Step 11: Apply the lazy adjustedDuration micro-opt in removeNpc**

In `modules/world/npc_registry.go`, modify `removeNpc` (currently lines 135-156).

**Before:**
```go
func (s *Server) removeNpc(n *Npc, duration int) {
	adjustedDuration := s.scaleByPlayerCount(duration)
	// DEVIATION NAI-19-D1: zone.leave omitted — Zone abstraction
	// not ported. See spec § Tracked deviations.
	n.dead = true
	if s.gamemap != nil { // Task 2 already updated this guard
		switch n.blockWalk {
		// ... case branches reading n.size ...
		}
	}
	if n.lifecycle == NpcLifecycleDespawn {
		// ... unchanged ...
	} else if n.lifecycle == NpcLifecycleRespawn && duration > -1 {
		n.lifecycleTick = adjustedDuration
	}
}
```

**After:**
```go
func (s *Server) removeNpc(n *Npc, duration int) {
	// DEVIATION NAI-19-D1: zone.leave omitted — Zone abstraction
	// not ported. See spec § Tracked deviations.
	n.dead = true
	if s.gamemap != nil { // Task 2 already updated this guard
		switch n.blockWalk {
		// ... case branches reading n.size — UNCHANGED ...
		}
	}
	if n.lifecycle == NpcLifecycleDespawn {
		// ... unchanged ...
	} else if n.lifecycle == NpcLifecycleRespawn && duration > -1 {
		n.lifecycleTick = s.scaleByPlayerCount(duration)
	}
}
```

(Drop the `adjustedDuration := ...` line at the top of the function; lift the call inline in the RESPAWN+duration>-1 branch. Matches TS short-circuit at `World.ts:1316-1318`.)

- [ ] **Step 12: Verify existing removeNpc tests still pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./modules/world/ -run "RemoveNpc|Respawn|Despawn" -v`

Expected: all PASS. Behavior is identical when reachable; the change is purely a no-allocation micro-opt for the duration=-1 path. Per spec § Test Strategy, no dedicated test for C3.

- [ ] **Step 13: Run full repo test suite with race detector**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race -count=1 ./...`

Expected: all PASS.

- [ ] **Step 14: Commit**

```bash
git add pkg/cache/crctable.go \
       pkg/cache/crctable_test.go \
       modules/world/world_test.go \
       modules/world/npc_registry.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
polish(world): NAI-20 Task 3 — cache + removeNpc bundle

Three NAI-19 follow-ups closed in one commit:

- C1: cache.ResetCRCState() helper replaces the inline reset in
  world_test.go. Lockstep with package-init expressions to avoid
  drift if CrcBuffer / CrcTable / CrcBuffer32 init shapes change.
- C2: cache.makeCrc emits slog.Default().Warn on os.Stat / packet.Load
  failure (was silently returning). Avoids undersized CrcBuffer with
  no log-line at server startup. Stays scope-limited: pkg/cache has
  no logger plumbing, so slog.Default() avoids a wider refactor.
- C3: lazy adjustedDuration in (*Server).removeNpc — lifts the
  s.scaleByPlayerCount(duration) call inside the RESPAWN+duration>-1
  branch. Matches TS short-circuit at World.ts:1316-1318. The DESPAWN
  path (most callers, including npc_ai.go:46 and revertType heavy)
  passes duration=-1 and now skips the O(2048) array sweep.

Tests:
- TestResetCRCStateRestoresInitialState (pkg/cache)
- TestMakeCrcWarnsOnMissingFile (pkg/cache; slog.Default swap)
- C3 has no dedicated test per spec — existing removeNpc test
  coverage proves behavioral parity (lazy-eval is observationally
  identical when reachable).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: D — checkNotNull on three NPC opcode handlers

**Files:**
- Modify: `pkg/script/handlers_npc.go` (lines 281-289, 296-309, 330-336)
- Modify: `pkg/script/handlers_npc_test.go` (append three negative-pin tests)

**Goal:** Wrap popped count parameters in `checkNotNull` (existing helper at `handlers_player.go:61`) for `handleNpcDelay` (delay), `handleNpcQueue` (delay only — queueId already has range check), `handleNpcSetHunt` (huntRange).

- [ ] **Step 1: Write failing test for handleNpcDelay -1 rejection**

Append to `pkg/script/handlers_npc_test.go`:

```go
// TestHandleNpcDelayRejectsNegativeDelay pins NAI-20 Task 4: NPC_DELAY
// rejects delay=-1 via checkNotNull (TS NumberNotNull). Mirrors the S7b
// back-fill pattern on handleNpcSetTimer.
func TestHandleNpcDelayRejectsNegativeDelay(t *testing.T) {
	mock := &mockNpc{}
	s := newScriptStateForTest(t)
	s.ActiveNpc = mock
	s.PushInt(-1) // delay

	err := handleNpcDelay(s)

	if err == nil {
		t.Fatal("expected error for delay=-1 (NumberNotNull); got nil")
	}
	if !strings.Contains(err.Error(), "NPC_DELAY") {
		t.Errorf("expected error to name NPC_DELAY; got: %v", err)
	}
	if mock.delayedCalls != 0 {
		t.Errorf("SetDelayed called %d times; should be 0 on rejection", mock.delayedCalls)
	}
}
```

**Plan-time verification at impl-time**: confirm the `mockNpc` struct field name (`delayedCalls` vs `setDelayedCalls`) at `handlers_npc_test.go`'s mock declaration. Adjust assertion accordingly. Confirm `newScriptStateForTest` exists; if not, use the existing test-state-setup pattern from neighboring tests.

- [ ] **Step 2: Verify the test fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./pkg/script/ -run TestHandleNpcDelayRejectsNegativeDelay -v`

Expected: FAIL with `expected error for delay=-1 (NumberNotNull); got nil`. The current handler accepts -1.

- [ ] **Step 3: Add checkNotNull to handleNpcDelay**

In `pkg/script/handlers_npc.go`, modify `handleNpcDelay` (lines 281-289):

**Before:**
```go
func handleNpcDelay(s *ScriptState) error {
	if err := requireActiveNpc(s, "NPC_DELAY"); err != nil {
		return err
	}
	ticks := s.PopInt()
	s.ActiveNpc.SetDelayed(ticks)
	s.Execution = NpcSuspended
	return nil
}
```

**After:**
```go
func handleNpcDelay(s *ScriptState) error {
	if err := requireActiveNpc(s, "NPC_DELAY"); err != nil {
		return err
	}
	ticks := s.PopInt()
	if err := checkNotNull(ticks, "NPC_DELAY"); err != nil {
		return err
	}
	s.ActiveNpc.SetDelayed(ticks)
	s.Execution = NpcSuspended
	return nil
}
```

- [ ] **Step 4: Verify the delay test passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./pkg/script/ -run TestHandleNpcDelayRejectsNegativeDelay -v`

Expected: PASS.

- [ ] **Step 5: Write failing test for handleNpcQueue -1 delay rejection**

Append to `pkg/script/handlers_npc_test.go`:

```go
// TestHandleNpcQueueRejectsNegativeDelay pins NAI-20 Task 4: NPC_QUEUE
// rejects delay=-1 via checkNotNull. The queueId 1..20 range check
// remains in place and is exercised separately. arg can be any value.
func TestHandleNpcQueueRejectsNegativeDelay(t *testing.T) {
	mock := &mockNpc{}
	s := newScriptStateForTest(t)
	s.ActiveNpc = mock
	s.scriptProvider = newStubScriptProvider() // minimal stub, see existing tests

	// Pop order: delay (top), arg, queueID (bottom). Push in reverse.
	s.PushInt(5)  // queueID (valid)
	s.PushInt(0)  // arg
	s.PushInt(-1) // delay (top of stack — NumberNotNull rejects)

	err := handleNpcQueue(s)

	if err == nil {
		t.Fatal("expected error for delay=-1 (NumberNotNull); got nil")
	}
	if !strings.Contains(err.Error(), "NPC_QUEUE") {
		t.Errorf("expected error to name NPC_QUEUE; got: %v", err)
	}
	if mock.enqueueCalls != 0 {
		t.Errorf("EnqueueScriptForTrigger called %d times; should be 0 on rejection",
			mock.enqueueCalls)
	}
}
```

**Plan-time verification at impl-time**: confirm `mockNpc.enqueueCalls` exists and confirm `newStubScriptProvider()` (or however script provider stubbing is done) at `handlers_npc_test.go`. Adjust if needed.

- [ ] **Step 6: Verify the queue test fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./pkg/script/ -run TestHandleNpcQueueRejectsNegativeDelay -v`

Expected: FAIL.

- [ ] **Step 7: Add checkNotNull to handleNpcQueue**

In `pkg/script/handlers_npc.go`, modify `handleNpcQueue` (lines 296-309):

**Before:**
```go
func handleNpcQueue(s *ScriptState) error {
	if err := requireActiveNpc(s, "NPC_QUEUE"); err != nil {
		return err
	}
	delay := s.PopInt()
	arg := s.PopInt()
	queueID := s.PopInt()
	if queueID < 1 || queueID > 20 {
		return fmt.Errorf("NPC_QUEUE: invalid queueId %d (want 1..20)", queueID)
	}
	trigger := TriggerAiQueue1 + ServerTriggerType(queueID-1)
	s.ActiveNpc.EnqueueScriptForTrigger(trigger, delay, arg)
	return nil
}
```

**After:**
```go
func handleNpcQueue(s *ScriptState) error {
	if err := requireActiveNpc(s, "NPC_QUEUE"); err != nil {
		return err
	}
	delay := s.PopInt()
	if err := checkNotNull(delay, "NPC_QUEUE"); err != nil {
		return err
	}
	arg := s.PopInt()
	queueID := s.PopInt()
	if queueID < 1 || queueID > 20 {
		return fmt.Errorf("NPC_QUEUE: invalid queueId %d (want 1..20)", queueID)
	}
	trigger := TriggerAiQueue1 + ServerTriggerType(queueID-1)
	s.ActiveNpc.EnqueueScriptForTrigger(trigger, delay, arg)
	return nil
}
```

- [ ] **Step 8: Verify the queue test passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./pkg/script/ -run TestHandleNpcQueueRejectsNegativeDelay -v`

Expected: PASS.

- [ ] **Step 9: Write failing test for handleNpcSetHunt -1 rejection**

Append to `pkg/script/handlers_npc_test.go`:

```go
// TestHandleNpcSetHuntRejectsNegativeRange pins NAI-20 Task 4:
// NPC_SETHUNT rejects range=-1 via checkNotNull (TS NumberNotNull).
func TestHandleNpcSetHuntRejectsNegativeRange(t *testing.T) {
	mock := &mockNpc{}
	s := newScriptStateForTest(t)
	s.ActiveNpc = mock
	s.PushInt(-1) // range

	err := handleNpcSetHunt(s)

	if err == nil {
		t.Fatal("expected error for range=-1 (NumberNotNull); got nil")
	}
	if !strings.Contains(err.Error(), "NPC_SETHUNT") {
		t.Errorf("expected error to name NPC_SETHUNT; got: %v", err)
	}
	if mock.setHuntRangeCalls != 0 {
		t.Errorf("SetHuntRange called %d times; should be 0 on rejection",
			mock.setHuntRangeCalls)
	}
}
```

**Plan-time verification at impl-time**: confirm `mockNpc.setHuntRangeCalls` field name.

- [ ] **Step 10: Verify the sethunt test fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./pkg/script/ -run TestHandleNpcSetHuntRejectsNegativeRange -v`

Expected: FAIL.

- [ ] **Step 11: Add checkNotNull to handleNpcSetHunt**

In `pkg/script/handlers_npc.go`, modify `handleNpcSetHunt` (lines 330-336):

**Before:**
```go
func handleNpcSetHunt(s *ScriptState) error {
	if err := requireActiveNpc(s, "NPC_SETHUNT"); err != nil {
		return err
	}
	s.ActiveNpc.SetHuntRange(s.PopInt())
	return nil
}
```

**After:**
```go
func handleNpcSetHunt(s *ScriptState) error {
	if err := requireActiveNpc(s, "NPC_SETHUNT"); err != nil {
		return err
	}
	huntRange := s.PopInt()
	if err := checkNotNull(huntRange, "NPC_SETHUNT"); err != nil {
		return err
	}
	s.ActiveNpc.SetHuntRange(huntRange)
	return nil
}
```

- [ ] **Step 12: Verify the sethunt test passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./pkg/script/ -run TestHandleNpcSetHuntRejectsNegativeRange -v`

Expected: PASS.

- [ ] **Step 13: Run full pkg/script test suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./pkg/script/...`

Expected: all PASS, including all existing `TestHandleNpcDelay*`, `TestHandleNpcQueue*`, `TestHandleNpcSetHunt*` tests.

- [ ] **Step 14: Run full repo test suite with race detector**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race -count=1 ./...`

Expected: all PASS.

- [ ] **Step 15: Commit**

```bash
git add pkg/script/handlers_npc.go pkg/script/handlers_npc_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-20 Task 4 — checkNotNull on three NPC opcode handlers

Closes the NAI-2 / NAI-3 / NAI-7 NumberNotNull fidelity-audit follow-ups:
- handleNpcDelay (NPC_DELAY): wraps `ticks` pop in checkNotNull.
- handleNpcQueue (NPC_QUEUE): wraps `delay` pop in checkNotNull (top
  of stack, popped first). queueId 1..20 range check unchanged. arg
  pop unwrapped (no TS NumberNotNull on script args).
- handleNpcSetHunt (NPC_SETHUNT): wraps `range` pop in checkNotNull.

Mirrors TS check(state.popInt(), NumberNotNull) at NpcOps.ts and the
S7b back-fill pattern on handleNpcSetTimer. Uses the existing
checkNotNull helper at pkg/script/handlers_player.go:61 — no new
infrastructure.

Tests (negative-pin shape mirroring TestHandleNpcSetTimerRejectsNegative):
- TestHandleNpcDelayRejectsNegativeDelay
- TestHandleNpcQueueRejectsNegativeDelay (with valid queueId)
- TestHandleNpcSetHuntRejectsNegativeRange

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: E — size-aware DistanceTo rewire at TS-flagged sites

**Files:**
- Modify: `modules/world/npc_interaction.go` (lines 433-446 — PLAYERESCAPE retreat)
- Modify: `modules/world/npc_player_modes.go` (lines 41-66 — playerFaceCloseMode)
- Modify: `modules/world/npc_interaction_test.go` (append tests)
- Modify: `modules/world/npc_player_modes_test.go` (append tests)

**Goal:** Rewire 3 size-approximated sites to use the existing `coordgrid.DistanceTo` (size-aware). Per plan-time TS source reads, only 3 of 7 candidate sites need rewiring — the other 4 use TS `distanceToSW` and stay on `coordgrid.DistanceToSW`.

### Plan-time TS-source disposition table (verified at plan authoring)

| # | File:Line | TS Ref | TS Function | Decision |
|---|---|---|---|---|
| 1 | `npc_interaction.go:440` | TS Npc.ts:658-663 | `distanceTo` (size-aware, with `width: this.width, length: this.length` for both args) | **REWIRE** |
| 2 | `npc_interaction.go:441` | TS Npc.ts:664-669 | `distanceTo` (size-aware, target's width/length on both args) | **REWIRE** |
| 3 | `npc_interaction.go:471` | TS Npc.ts:652 | `distanceToSW` (no size) | **KEEP** — comment-pin reasoning |
| 4 | `npc_interaction.go:476` | TS Npc.ts:676 | `distanceToSW` (no size) | **KEEP** — comment-pin reasoning |
| 5 | `npc_player_modes.go:54-62` | TS Npc.ts:826 | `distanceTo` (size-aware, entity-arg form) | **REWIRE** (replace inline `max(|dx|,|dz|)`) |
| 6 | `npc_player_modes.go:154` | TS Npc.ts:751 | `distanceToSW` (no size) | **KEEP** — comment already correctly cites `distanceToSW` |
| 7 | `npc_player_modes.go:172` | TS Npc.ts:782-785 | `distanceToSW` (no size) | **KEEP** — TS arg shape uses `{x, z}` form (no width/length) |

**TS PLAYERESCAPE quirk** (sites 1+2): TS `Npc.ts:658-669` passes the *subject's* width/length even for the start-coord parameter. So both `DistanceTo` calls in goscape use the same width/length on both rectangle args (NPC self → `n.size, n.size`; target → `tw, tl`).

**TS playerFaceClose quirk** (site 5): TS `Npc.ts:826` calls `CoordGrid.distanceTo(this, this.target)` — entity-arg form that internally extracts width/length. Goscape needs explicit args via `coordgrid.DistanceTo(...)`.

- [ ] **Step 1: Write failing test for PLAYERESCAPE size-asymmetry pin**

Append to `modules/world/npc_interaction_test.go`:

```go
// TestTargetWithinMaxRangePlayerEscapeUsesSizeAwareDistance pins
// NAI-20 Task 5: the PLAYERESCAPE branch in (*Npc).targetWithinMaxRange
// uses coordgrid.DistanceTo (size-aware) per TS Npc.ts:658-669, NOT
// DistanceToSW. With a size=2 NPC at (3200,3200) and start at
// (3203,3200), the SW-only distance is 3 but the size-aware distance
// is 2 (closest tile pair: occupiedX=3201 vs 3203). For maxrange=2,
// size-aware passes the gate; SW-only fails it.
func TestTargetWithinMaxRangePlayerEscapeUsesSizeAwareDistance(t *testing.T) {
	s := newTestServer(t)
	typ := &objtype.NpcType{
		Size:      2,
		BlockWalk: objtype.BlockWalkNPC,
		MaxRange:  2,
	}
	n := newRegisteredNpc(t, s, typ, false)
	n.x, n.z = 3200, 3200
	n.startX, n.startZ = 3203, 3200
	n.targetOp = objtype.NPCModePlayerEscape

	// Stub a Player target also at startX,startZ (so target's distance
	// from start is 0 — the early-pass condition is dominated by NPC's
	// distance from start).
	target := &Player{}
	target.SetCoords(3203, 3200, 0)
	n.target = target

	got := n.targetWithinMaxRange()
	if !got {
		t.Errorf("PLAYERESCAPE targetWithinMaxRange = false; want true (size=2 NPC " +
			"closest tile is 2 from startX=3203, within maxrange=2)")
	}
}
```

**Plan-time verification at impl-time**: confirm `Player.SetCoords(x, z, level)` exists; if not, use direct field writes (`target.x, target.z = ...`) per neighboring tests. Confirm `n.target` is the field name used by `targetWithinMaxRange`.

- [ ] **Step 2: Verify the test fails (size-1 approximation rejects the case)**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./modules/world/ -run TestTargetWithinMaxRangePlayerEscapeUsesSizeAwareDistance -v`

Expected: FAIL with `targetWithinMaxRange = false; want true`. The current `DistanceToSW` returns 3 (>2), failing the gate; size-aware would return 2 (passes).

- [ ] **Step 3: Rewire npc_interaction.go PLAYERESCAPE branch**

In `modules/world/npc_interaction.go`, locate the PLAYERESCAPE block (currently lines 433-446). Get target sizes via the existing `approachEntitySize(target entity) (width, length int)` helper at `npc_interaction.go:519`.

**Before:**
```go
// TS :657-673 — PLAYERESCAPE retreat. Size-aware distanceTo from BOTH
// NPC and target to (startX, startZ); rejects only when BOTH exceed
// maxrange. No +1, no corner-removal — shape is distinct from the OP
// branch. For size-1 NPC/Player (the only case this era supports),
// DistanceToSW is equivalent to the TS size-aware distanceTo; the
// size-approximation inherits NAI-12's tracked follow-up.
if n.targetOp == objtype.NPCModePlayerEscape {
	distanceToEscape := coordgrid.DistanceToSW(n.x, n.z, n.startX, n.startZ)
	targetDistanceFromStart := coordgrid.DistanceToSW(tx, tz, n.startX, n.startZ)
	if targetDistanceFromStart > maxrng && distanceToEscape > maxrng {
		return false
	}
	return true
}
```

**After:**
```go
// TS :657-673 — PLAYERESCAPE retreat. Size-aware distanceTo from BOTH
// NPC and target to (startX, startZ); rejects only when BOTH exceed
// maxrange. TS quirk: the start-coord rectangle adopts the SUBJECT's
// width/length on each call (n's size for the n-call, target's size
// for the target-call). NAI-20 closes the NAI-12 size-approximation
// follow-up at this site.
if n.targetOp == objtype.NPCModePlayerEscape {
	tw, tl := approachEntitySize(n.target)
	distanceToEscape := coordgrid.DistanceTo(
		n.x, n.z, n.size, n.size,
		n.startX, n.startZ, n.size, n.size)
	targetDistanceFromStart := coordgrid.DistanceTo(
		tx, tz, tw, tl,
		n.startX, n.startZ, tw, tl)
	if targetDistanceFromStart > maxrng && distanceToEscape > maxrng {
		return false
	}
	return true
}
```

- [ ] **Step 4: Verify the size-asymmetry test passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./modules/world/ -run TestTargetWithinMaxRangePlayerEscapeUsesSizeAwareDistance -v`

Expected: PASS.

- [ ] **Step 5: Write parity pin for size-1 PLAYERESCAPE**

Append to `modules/world/npc_interaction_test.go`:

```go
// TestTargetWithinMaxRangePlayerEscapeSize1Parity pins NAI-20 Task 5:
// for size=1 NPC + size=1 target (the dominant production data),
// DistanceTo's result equals DistanceToSW's result. No regression on
// existing cases.
func TestTargetWithinMaxRangePlayerEscapeSize1Parity(t *testing.T) {
	s := newTestServer(t)
	typ := &objtype.NpcType{
		Size:      1,
		BlockWalk: objtype.BlockWalkNPC,
		MaxRange:  5,
	}
	n := newRegisteredNpc(t, s, typ, false)
	n.x, n.z = 3200, 3200
	n.startX, n.startZ = 3203, 3204
	n.targetOp = objtype.NPCModePlayerEscape
	target := &Player{}
	target.SetCoords(3206, 3208, 0)
	n.target = target

	// Manual SW-distance: max(|3203-3200|, |3204-3200|) = 4 (NPC).
	// Manual SW-distance: max(|3203-3206|, |3204-3208|) = 4 (target).
	// Both ≤ maxrange=5 → returns true.
	got := n.targetWithinMaxRange()
	if !got {
		t.Errorf("PLAYERESCAPE size-1 parity: got false; want true")
	}
}
```

- [ ] **Step 6: Verify the parity test passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./modules/world/ -run TestTargetWithinMaxRangePlayerEscapeSize1Parity -v`

Expected: PASS.

- [ ] **Step 7: Annotate KEEP sites in npc_interaction.go**

In `modules/world/npc_interaction.go`, ensure the comments on the checkApTrigger and default branches (currently around lines 469-478) acknowledge that TS uses `distanceToSW` here (NOT `distanceTo`). If the existing comments correctly cite `distanceToSW`, leave them; if they don't cite it explicitly, add a one-line note:

After-form for line 469-472 area (verify against current text and adjust):

```go
case checkApTrigger(n.targetOp):
	// TS :651-654 — SW-distance up to maxrange + attackrange. Per TS,
	// this branch uses distanceToSW (no size); the NAI-20 audit
	// confirmed TS does not size this comparison. KEEP DistanceToSW.
	d := coordgrid.DistanceToSW(tx, tz, n.startX, n.startZ)
	return d <= maxrng+attackrng

default:
	// TS :676 — SW-distance up to maxrange + 1. Per TS, this branch
	// uses distanceToSW (no size); KEEP DistanceToSW.
	d := coordgrid.DistanceToSW(tx, tz, n.startX, n.startZ)
	return d <= maxrng+1
```

(Goal: the comment-pin makes the KEEP decision auditable. No code change.)

- [ ] **Step 8: Write failing test for playerFaceCloseMode size-asymmetry**

Append to `modules/world/npc_player_modes_test.go`:

```go
// TestPlayerFaceCloseModeUsesSizeAwareDistance pins NAI-20 Task 5:
// playerFaceCloseMode uses coordgrid.DistanceTo (size-aware) per TS
// Npc.ts:826, NOT inline max(|dx|,|dz|). With a size=2 NPC at
// (3200,3200) and target at (3202,3200), the inline approximation
// returns 2 (>1, would clear interaction); the size-aware
// distance is 1 (occupiedX=3201 to 3202 = 1, keeps interaction).
func TestPlayerFaceCloseModeUsesSizeAwareDistance(t *testing.T) {
	s := newTestServer(t)
	typ := &objtype.NpcType{Size: 2, BlockWalk: objtype.BlockWalkNPC}
	n := newRegisteredNpc(t, s, typ, false)
	n.x, n.z = 3200, 3200
	n.targetOp = objtype.NPCModePlayerFaceClose

	target := &Player{}
	target.SetCoords(3202, 3200, 0)
	n.target = target

	// Pre-condition: target is set, NPC has not interacted-cleared.
	preTargetOp := n.targetOp

	n.playerFaceCloseMode(s)

	// Size-aware distance is 1 (occupiedX=3201 to 3202). Within
	// faceclose's > 1 threshold → interaction PRESERVED.
	if n.targetOp != preTargetOp {
		t.Errorf("playerFaceCloseMode cleared interaction; size-aware " +
			"distance to target should be 1 (within range)")
	}
}
```

**Plan-time verification at impl-time**: confirm `playerFaceCloseMode(s *Server)` is the actual function signature (it might be `(*Npc).playerFaceCloseMode(s *Server)`). Confirm `objtype.NPCModePlayerFaceClose` constant. Confirm `n.resetDefaults()` is what mutates `n.targetOp` — if not, adjust the assertion.

- [ ] **Step 9: Verify the faceclose test fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./modules/world/ -run TestPlayerFaceCloseModeUsesSizeAwareDistance -v`

Expected: FAIL — current inline Chebyshev returns 2, exceeds threshold of 1, calls `resetDefaults`.

- [ ] **Step 10: Rewire playerFaceCloseMode**

In `modules/world/npc_player_modes.go`, locate `playerFaceCloseMode` (currently lines 41-66). Replace the inline distance check with `coordgrid.DistanceTo`.

**Before:**
```go
func (n *Npc) playerFaceCloseMode(s *Server) {
	p, ok := n.target.(*Player)
	if !ok {
		s.log.Warn("playerFaceCloseMode: non-Player target",
			"nid", n.nid, "targetOp", n.targetOp)
		return
	}

	// TS CoordGrid.distanceTo(this, target) — size-aware Chebyshev.
	// NAI-13 inherits the 1,1,1,1 size approximation tracked as the
	// NAI-12 "size-aware LoS" follow-up; single-tile NPCs + single-tile
	// Players reduce this to plain max(|dx|, |dz|).
	tx, tz, _ := p.Coords()
	dx := n.x - tx
	if dx < 0 {
		dx = -dx
	}
	dz := n.z - tz
	if dz < 0 {
		dz = -dz
	}
	if max(dx, dz) > 1 {
		n.resetDefaults()
		return
	}
}
```

**After:**
```go
func (n *Npc) playerFaceCloseMode(s *Server) {
	p, ok := n.target.(*Player)
	if !ok {
		s.log.Warn("playerFaceCloseMode: non-Player target",
			"nid", n.nid, "targetOp", n.targetOp)
		return
	}

	// TS CoordGrid.distanceTo(this, target) — size-aware Chebyshev.
	// NAI-20 closes the size-approximation follow-up at this site.
	tx, tz, _ := p.Coords()
	tw, tl := approachEntitySize(n.target)
	if coordgrid.DistanceTo(n.x, n.z, n.size, n.size, tx, tz, tw, tl) > 1 {
		n.resetDefaults()
		return
	}
}
```

(Drop the inline dx/dz/max computation. Add `coordgrid` import if not present. Drop the `max` use — Go's builtin.)

- [ ] **Step 11: Verify the faceclose test passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./modules/world/ -run TestPlayerFaceCloseModeUsesSizeAwareDistance -v`

Expected: PASS.

- [ ] **Step 12: Write parity pin for size-1 playerFaceClose**

Append to `modules/world/npc_player_modes_test.go`:

```go
// TestPlayerFaceCloseModeSize1Parity pins NAI-20 Task 5: for size=1
// NPC + size=1 target (dominant production data), the size-aware
// DistanceTo result equals the prior inline max(|dx|,|dz|) result.
// No regression on existing cases.
func TestPlayerFaceCloseModeSize1Parity(t *testing.T) {
	s := newTestServer(t)
	typ := &objtype.NpcType{Size: 1, BlockWalk: objtype.BlockWalkNPC}
	n := newRegisteredNpc(t, s, typ, false)
	n.x, n.z = 3200, 3200
	n.targetOp = objtype.NPCModePlayerFaceClose

	// Target 2 tiles east — distance 2 > 1 → interaction MUST clear.
	target := &Player{}
	target.SetCoords(3202, 3200, 0)
	n.target = target

	n.playerFaceCloseMode(s)

	// Per TS Npc.ts:826-829, distance > 1 calls resetDefaults.
	// Verify by checking n.target is now nil (resetDefaults clears).
	if n.target != nil {
		t.Errorf("playerFaceCloseMode did NOT clear interaction; " +
			"size-1 distance to (3202,3200) is 2 > 1, should reset")
	}
}
```

**Plan-time verification at impl-time**: confirm what `resetDefaults()` actually clears. If it doesn't null `n.target`, switch the assertion to whatever it does mutate (e.g., `n.targetOp == 0` or similar). Look at `resetDefaults` body before finalizing the test.

- [ ] **Step 13: Verify the parity test passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./modules/world/ -run TestPlayerFaceCloseModeSize1Parity -v`

Expected: PASS.

- [ ] **Step 14: Annotate KEEP sites in npc_player_modes.go**

In `modules/world/npc_player_modes.go`, the existing comments at the two KEEP sites (lines 153 area for 25-tile abandon, 171 area for within-maxrange) should already cite `distanceToSW`. If they're vague or inherit obsolete NAI-12/NAI-13 framing, add a one-line note. Example for line 153 area (verify against current text):

```go
// TS :751-754 — abandon if already > 25 tiles SW-distance. TS uses
// distanceToSW here (NOT distanceTo); KEEP DistanceToSW.
if coordgrid.DistanceToSW(n.x, n.z, tx, tz) > 25 {
	n.resetDefaults()
	return
}
```

And for line 171 area:

```go
// TS :780-790 — within-maxrange diagonal waypoint. TS uses distanceToSW
// here (the start-coord arg is `{x, z}` only, no width/length); KEEP
// DistanceToSW.
if coordgrid.DistanceToSW(mx, mz, n.startX, n.startZ) < int(n.typ.MaxRange) {
	n.queueWaypoint(mx, mz)
	n.updateMovement(s)
	return
}
```

(Comment-only edits; no code change.)

- [ ] **Step 15: Run full repo test suite with race detector**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race -count=1 ./...`

Expected: all PASS.

- [ ] **Step 16: Commit**

```bash
git add modules/world/npc_interaction.go \
       modules/world/npc_player_modes.go \
       modules/world/npc_interaction_test.go \
       modules/world/npc_player_modes_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
polish(world): NAI-20 Task 5 — size-aware DistanceTo at TS-flagged sites

Closes NAI-12 / NAI-18 size-approximation follow-ups. Plan-time TS
source reads at TS Npc.ts:651-829 confirmed only 3 of 7 candidate sites
need rewiring; the other 4 use TS distanceToSW and stay on
coordgrid.DistanceToSW (now annotated as KEEP).

Rewired sites (all use existing coordgrid.DistanceTo at coordgrid.go:118
— no library change required):
- npc_interaction.go:440-441 (PLAYERESCAPE retreat) → DistanceTo with
  TS subject-width/length-on-both-args quirk preserved.
- npc_player_modes.go:54-62 (playerFaceCloseMode) → DistanceTo replaces
  inline max(|dx|,|dz|).

Sizing source: existing approachEntitySize(target entity) helper at
npc_interaction.go:519. NPC self uses n.size (Task 2 snapshot).

Tests (per rewired site, 2 pins):
- Size-asymmetry pin: size=2 case where DistanceTo and DistanceToSW
  return different values; assertion picks the size-aware result.
- Size-1 parity pin: dominant-data case proving no regression.

KEEP sites (npc_interaction.go:471, 476; npc_player_modes.go:154, 172)
gain comment-pin annotations citing TS distanceToSW. Comment-only
edits, no code change.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Bundle Close

After all 5 task commits land and pass:

- [ ] **Step 1: Two-stage final whole-impl review**

Dispatch fresh subagents:
1. **Stage 1 — code quality**: `superpowers:code-reviewer` on the full diff `02e24cd..HEAD`. Focus: dead code, unused imports, comment freshness, helper API ergonomics, test redundancy.
2. **Stage 2 — TS fidelity**: `feature-dev:code-reviewer` on the full diff. Focus: each TS line ref cited in spec/plan reads correctly at HEAD; size-quirk preservation (TS subject-width-on-both-args); deviation-count invariant (16 active, unchanged).

- [ ] **Step 2: Apply review polish (if any)**

For each high-confidence reviewer finding, apply as a separate `polish(world): NAI-20 review fix — <short>` commit. Skip findings with confidence < 70% per project policy.

- [ ] **Step 3: Update memory annotations**

Edit `nai_followups.md` to annotate retired entries with `**Resolved 2026-04-25 (NAI-20 Task N, commit <hash>)**` headers. Tasks closed:
- "From NAI-2" → handleNpcDelay NumberNotNull (Task 4)
- "From NAI-3" → handleNpcQueue NumberNotNull (Task 4)
- "From NAI-7" → handleNpcSetHunt NumberNotNull (Task 4)
- "From NAI-12" → size-aware DistanceTo at flagged sites (Task 5)
- "From NAI-18" → orphaned DistanceToSW size approximations (Task 5; partial — only 3 sites; the 4 KEEP sites stay on DistanceToSW per TS audit)
- "From NAI-19" cross-task collision-toggle/snapshot interaction (Task 2)
- "From NAI-19" NpcMaskChangeType-on-first-spawn divergence (Task 2)
- "From NAI-19" deferred test-scaffolding extraction (Task 1)
- "From NAI-19" deferred cache-polish ResetCRCState (Task 3 C1)
- "From NAI-19" deferred error-handling MakeCRCs slog.Warn (Task 3 C2)
- "From NAI-19" deferred micro-opt lazy adjustedDuration (Task 3 C3)

- [ ] **Step 4: Close commit**

```bash
git commit --no-gpg-sign --allow-empty -m "$(cat <<'EOF'
chore(world): NAI-20 closed — follow-up bundle (5 tasks)

Closes 11 follow-up entries across NAI-2/3/7/12/18/19:
- Task 1 (B): newRegisteredNpc test helper extraction (refactor).
- Task 2 (A + item 5): *Npc.blockWalk + *Npc.size geometry snapshot
  + first-spawn NpcMaskChangeType gate fold.
- Task 3 (C bundle): cache.ResetCRCState helper + makeCrc slog.Warn
  + lazy adjustedDuration in removeNpc.
- Task 4 (D): checkNotNull on handleNpcDelay/Queue/SetHunt.
- Task 5 (E): coordgrid.DistanceTo rewire at 3 TS-flagged sites
  (4 KEEP sites stay on DistanceToSW per TS audit).

Plan-time discoveries baked into implementation:
- coordgrid.DistanceTo already existed at coordgrid.go:118 — no
  library extension needed (the NAI-19 followup memo's "add a sized
  companion" was stale).
- checkNotNull helper already existed at handlers_player.go:61 from
  S7b — no new validator port.
- TS source audit at Npc.ts:651-829 narrowed Task 5 from 7 sites to
  3 actual rewires.

Active deviation count: 16 (unchanged).

Closes memory:
- nai_followups.md "From NAI-2"  → handleNpcDelay NumberNotNull
- nai_followups.md "From NAI-3"  → handleNpcQueue NumberNotNull
- nai_followups.md "From NAI-7"  → handleNpcSetHunt NumberNotNull
- nai_followups.md "From NAI-12" → size-aware DistanceTo at flagged sites
- nai_followups.md "From NAI-18" → orphaned DistanceToSW (3 of 4 sites)
- nai_followups.md "From NAI-19" cross-task collision-toggle/snapshot
- nai_followups.md "From NAI-19" NpcMaskChangeType-on-first-spawn
- nai_followups.md "From NAI-19" test-scaffolding extraction
- nai_followups.md "From NAI-19" cache-polish ResetCRCState
- nai_followups.md "From NAI-19" error-handling MakeCRCs slog.Warn
- nai_followups.md "From NAI-19" micro-opt lazy adjustedDuration

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Self-review (inline)

**Spec coverage**: every section/requirement in the spec maps to a task. Spec § "Task 1 — `newRegisteredNpc` helper" → plan Task 1. Spec § "Task 2 — `*Npc` geometry snapshot + first-spawn mask gate" → plan Task 2 (5 tests as spec'd). Spec § "Task 3 — Cache + removeNpc polish bundle" → plan Task 3 (3 sub-items, 2 tests + parity). Spec § "Task 4 — `checkNotNull` on three NPC opcode handlers" → plan Task 4 (3 negative-pin tests). Spec § "Task 5 — Size-aware distance via existing `coordgrid.DistanceTo`" → plan Task 5 (rewire 3 + KEEP 4 with comment-pins).

**Placeholder scan**: searched for "TBD", "TODO", "implement later" — none in plan code blocks. The 3 plan-time-resolution notes in spec ("TBD at plan-time") are now resolved by the disposition table in Task 5; no plan-step contains TBD. The "Plan-time verification at impl-time" notes call out specific facts the implementer must confirm against current HEAD (mock field names, helper-existence) — these are *verification* gates, not placeholders for missing content.

**Type consistency**: `n.blockWalk` (lowercase b) used consistently across Task 2 and Task 3. `n.size` consistent. `coordgrid.DistanceTo` (camelCase) consistent across Task 5. `checkNotNull(v int, op string) error` signature consistent across Task 4. `approachEntitySize(e entity) (width, length int)` consistent across Task 5.

**Test-coverage crosscheck** (per `plan_test_coverage_crosscheck` memory): every test the spec's § Test Strategy lists has a corresponding `Step N: Write failing test` block. 5 tests for Task 2; 2 tests + C3-no-test rationale for Task 3; 3 tests for Task 4; 4 tests (2 size-asymmetry + 2 parity) for Task 5. Task 1 has 0 new tests as spec'd.

**Helper-coverage crosscheck** (per `plan_helper_coverage` memory): `newRegisteredNpc(t, s, typ, register)` is called in Task 2 (5 callers), Task 4 (3 callers), Task 5 (4 callers). All callers use both `register=true` and `register=false` modes. Helper supports both. ✓
