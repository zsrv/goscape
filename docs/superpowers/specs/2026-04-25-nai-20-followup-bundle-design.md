# NAI-20 — follow-up bundle (test scaffolding + Npc geometry snapshot + cache/removeNpc polish + NumberNotNull on NPC handlers + size-aware DistanceTo rewire)

- **Sub-spec**: NAI-20
- **Date**: 2026-04-25
- **Scope label**: C (cross-package follow-up bundle — `pkg/cache`, `pkg/script`, `pkg/coordgrid` (consumer-side only), `modules/world`; ~105 LOC production + ~250 LOC tests; closes 11 distinct follow-up entries across NAI-2/3/7/12/18/19)
- **Predecessors**: NAI-19 (PATH B follow-up bundle) — last on `main` as `1b52859`
- **TS source root**: `LostCityRS/Engine-TS`

## Motivation

Eleven tracked NAI-series follow-ups have accumulated in `nai_followups.md` across NAI-2/3/7/12/18/19. NAI-19 closed PATH B; NAI-20 closes a follow-up *bundle* drawn from across the series. The 8 numbered groups below cluster into 5 tasks (C bundle = 3 items in one task; D bundle = 3 NPC handlers in one task; E bundle = multiple call sites closing two memo entries):

1. **A — Npc geometry snapshot (closes NAI-19 cross-task collision-toggle/snapshot interaction).** Stash `blockWalk` + `size` on `*Npc` at constructor time; read from snapshot in collision toggle paths instead of `n.typ.*`. Mirrors TS `PathingEntity` constructor-snapshot pattern (`World.ts:1271, 1302`). Fixes a *latent* size-changing-morph bug that the heavy revertType path would otherwise expose if a multi-tile changetype ever shipped in production data.
2. **A-fold (item 5) — first-spawn `NpcMaskChangeType` gate (closes NAI-19 NpcMaskChangeType-on-first-spawn).** Move the mask raise inside `resetEntityForRespawn`'s existing `if n.typeId != n.baseType` block. Currently the mask is raised on every NPC at server boot; cleared by `ResetMasks` before the first info-pass, so non-observable, but a TS divergence the next mask-state audit would flag.
3. **B — `newRegisteredNpc` test helper (closes NAI-19 deferred test-scaffolding extraction).** Extract the `s := newTestServer(t); typ := &objtype.NpcType{...}; n := NewNpc(...); register-into-s.npcs/npcLoop` boilerplate from ~11 existing tests across `npc_registry_test.go` and `npc_test.go`. Stage-1 final review for NAI-19 confirmed threshold met (~40 LOC of repetition).
4. **C1 — `cache.ResetCRCState()` helper (closes NAI-19 deferred cache-polish).** Export a re-init helper in `pkg/cache/crctable.go` so the inline reset in `modules/world/world_test.go` doesn't drift from the package-init expression.
5. **C2 — `cache.MakeCRCs()` silent-error observability (closes NAI-19 deferred error-handling).** Add `slog.Default().Warn(...)` on `os.Stat` / `packet.Load` failure in `pkg/cache/crctable.go:makeCrc`. Currently silent; failures manifest as undersized `CrcBuffer` with no log line.
6. **C3 — Lazy `adjustedDuration` in `removeNpc` (closes NAI-19 deferred micro-opt).** Lift `s.scaleByPlayerCount(duration)` inside the `RESPAWN && duration > -1` branch. Matches TS short-circuit at `World.ts:1316-1318`. Most callers pass `duration = -1` (DESPAWN path), making the eager 2048-element sweep wasted work.
7. **D — `checkNotNull` on three NPC opcode handlers (closes NAI-2 / NAI-3 / NAI-7 fidelity-audit follow-ups).** Wrap popped count parameters in `checkNotNull` (existing helper at `pkg/script/handlers_player.go:58`) for `handleNpcDelay`, `handleNpcQueue`, `handleNpcSetHunt`. Mirrors TS `check(state.popInt(), NumberNotNull)` pattern. The S7b back-fill closed `handleNpcSetTimer` already (2026-04-24); this sub-spec extends the pattern to the three remaining NPC handlers identified in the audit.
8. **E — Size-aware distance via existing `coordgrid.DistanceTo` (closes NAI-12 / NAI-18 size-approximation follow-ups).** Rewire flagged size-approximated `DistanceToSW` and inline `max(|dx|,|dz|)` call sites to the size-aware `coordgrid.DistanceTo` function that already exists at `coordgrid.go:118`. **No library change required** — the helper has been in place since pre-NAI-12; the follow-up memos that asked us to "add a sized companion" pre-dated visibility on its existence.

**Bundle rationale**: each item individually fits compressed-cadence (≤15 LOC), but together they exceed the threshold and cluster naturally around the NPC lifecycle / collision / fidelity surfaces. Bundling amortizes one cadence pass + one final review across 5 tasks.

## Tech stack

- Go 1.26+
- Existing packages touched:
  - `pkg/cache/crctable.go` (new exported `ResetCRCState`; `slog.Warn` in `makeCrc`)
  - `pkg/script/handlers_npc.go` (3 handlers gain `checkNotNull` wrap; helper already exists at `handlers_player.go:58`)
  - `modules/world/npc.go` (`*Npc` gains 2 fields; ctor seeds them)
  - `modules/world/npc_registry.go` (collision toggles read snapshot fields; `resetEntityForRespawn` mask gate fold; `removeNpc` lazy `adjustedDuration`)
  - `modules/world/npc_interaction.go`, `modules/world/npc_player_modes.go` (DistanceTo rewire at flagged sites)
  - `modules/world/world_test.go` (uses `cache.ResetCRCState`)
  - Multiple `_test.go` files (use new `newRegisteredNpc` helper)
- New file: `modules/world/npc_test_helpers.go` (Task 1 helper)
- No new packages; no new exported types.

## Scope (C)

### Task 1 — `newRegisteredNpc` helper extraction (~30 LOC helper, ~-40 LOC dup)

**Goal**: extract repeated test prologue from ~11 callers in `modules/world/npc_registry_test.go` and `modules/world/npc_test.go`. Lands first so all subsequent tasks' new tests use the helper from inception.

**New file**: `modules/world/npc_test_helpers.go` — test-only file (build-tag-free; the `_test.go` filename is too restrictive because the helper is shared across multiple `*_test.go` files in the same package, which already see each other without a special build tag).

Helper signature:

```go
package world

import (
    "testing"
    "github.com/zsrv/goscape/pkg/objtype"
)

// newRegisteredNpc constructs and optionally registers a synthetic NPC.
//
// Defaults: coords (3200, 3200, 0), typeId = 1.
//
// When register=true, allocates a slot via s.allocNpcSlot, calls
// s.addNpc(n, -1, true) — equivalent to the production first-spawn
// path. Required for tests that exercise s.npcs[] / s.npcLoop[]
// bookkeeping or run any code path that relies on n.server being set.
//
// When register=false, skips slot allocation and addNpc; returns a
// bare *Npc suitable for unit tests of constructor / mask / stats
// behavior.
//
// typ must not be nil (callers vary Size, BlockWalk, Stats etc per test).
// s.npcTypes is allocated to a [nil, typ] slice if currently nil so
// resetEntityForRespawn's lookupType call resolves correctly.
func newRegisteredNpc(t *testing.T, s *Server, typ *objtype.NpcType, register bool) *Npc {
    t.Helper()
    if typ == nil {
        t.Fatalf("newRegisteredNpc: typ must not be nil")
    }
    if s.npcTypes == nil {
        s.npcTypes = []*objtype.NpcType{nil, typ}
    }
    nid := 1
    if register {
        if got := s.allocNpcSlot(); got > 0 {
            nid = got
        } else {
            t.Fatalf("newRegisteredNpc: allocNpcSlot returned %d", got)
        }
    }
    n := NewNpc(nid, 1, 3200, 3200, 0, typ)
    if register {
        // addNpc allocates its own nid via allocNpcSlot — we already
        // burned one above, so put it back. The simpler approach is to
        // skip the pre-allocation and let addNpc do it. Refactor here:
        n.nid = 0  // sentinel; addNpc(firstSpawn=true) re-allocs
        if err := s.addNpc(n, -1, true); err != nil {
            t.Fatalf("newRegisteredNpc: addNpc: %v", err)
        }
    }
    return n
}
```

**Plan-time refinement note**: the slot-allocation interaction above is awkward. Final shape will be determined at plan-time after re-reading `(*Server).addNpc` and `allocNpcSlot` to pick the cleanest two-mode contract. Possible alternatives: (a) helper accepts a `register bool` and never pre-allocs (simplest); (b) helper has two variants (`newRegisteredNpc` and `newUnregisteredNpc`).

**Conversion audit**: at plan-time, grep `modules/world/*_test.go` for the three-statement prologue pattern. Convert each in the same commit as the helper extraction (single TDD cycle, single commit, single review).

**Test pin pattern**: helper is exercised by every converted test; no dedicated unit test for the helper itself.

### Task 2 — `*Npc` geometry snapshot + first-spawn mask gate (Items A + 5, ~25 LOC prod, ~80 LOC tests)

**Goal**: stash `blockWalk` and `size` on `*Npc` at constructor time and use them in collision toggle paths so a size-changing morph→revert cycle leaves correct base-size collision flags. Also fold item 5's mask gate to stop first-spawn from raising `NpcMaskChangeType` spuriously.

#### Struct addition (`modules/world/npc.go`)

Add to the `*Npc` struct between the `// === interaction ===` block and `// === masks ===` block (or in a new dedicated subsection — placement determined at plan-time):

```go
// === geometry snapshot (NAI-20) ===
// Captured at NewNpc; UNCHANGED by changetype to mirror TS PathingEntity
// (World.ts:1271, 1302). Read by addNpc/removeNpc collision toggles
// instead of n.typ.Size / n.typ.BlockWalk so a size-changing morph→revert
// cycle leaves base-size collision flags rather than morph-size flags.
blockWalk objtype.BlockWalk
size      int
```

#### Constructor seed (`NewNpc`)

In the `&Npc{...}` literal at the top of `NewNpc`:

```go
n := &Npc{
    // ... existing fields unchanged ...
    blockWalk: typ.BlockWalk,
    size:      int(typ.Size),
    // ... existing fields unchanged ...
}
```

#### Collision toggle reads (`npc_registry.go`)

Replace `n.typ.BlockWalk` and `int(n.typ.Size)` references in two collision-toggle blocks:

**`addNpc:65-72`** (currently):
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

After:
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

The `n.typ != nil` guard is removed because `n.blockWalk` and `n.size` are valid as long as `NewNpc` set them — they don't require live `n.typ` access. Plan-time check: confirm no other code path constructs `*Npc` literals (the answer should be "no, only `NewNpc`"); if any do, they must be updated to seed the snapshot fields.

**`removeNpc:140-147`** — identical pattern with `false` toggle.

#### Mask gate fold (item 5, `npc_registry.go:99-123`)

Move the existing `n.masks |= rsbuf.NpcMaskChangeType` line (currently at line 116, unconditional) inside the existing `if n.typeId != n.baseType` block at the top:

```go
func (s *Server) resetEntityForRespawn(n *Npc) {
    if n.typeId != n.baseType {
        n.typeId = n.baseType
        n.uid = (n.typeId << 16) | n.nid
        if newTyp := n.lookupType(n.baseType); newTyp != nil {
            n.typ = newTyp
        }
        n.masks |= rsbuf.NpcMaskChangeType  // moved into this block
    }
    // (delete the standalone `n.masks |= rsbuf.NpcMaskChangeType` line)
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

**Why**: matches TS semantics that the change-type mask signals "type CHANGED," not "type SET." TS `Npc.resetEntity(true)` (`Npc.ts:280-317`) does not raise this mask; the morph-direction (`changeType`) does, and `revertType` triggers `resetEntity(true)` AFTER the typeId mismatch is already reverted, so the mask raise must guard on the pre-reset state — which the existing `if n.typeId != n.baseType` block captures (it fires before `n.typeId = n.baseType` writes).

#### Tests (use Task 1's helper)

1. **`TestNewNpcSnapshotsBlockWalkAndSize`** — construct NPC with `Size: 2, BlockWalk: BlockWalkAll`; assert `n.blockWalk == BlockWalkAll && n.size == 2` post-ctor.
2. **`TestChangeTypeDoesNotMutateBlockWalkOrSize`** — construct size=1 NPC; call `n.ChangeType(typeID2 with size=2)`; assert `n.blockWalk` and `n.size` unchanged (still reflect baseType). Pins the TS-faithful snapshot semantics against future regressions.
3. **`TestSizeMorphRevertRestoresBaseFootprint`** — synthetic 2-tile morphed NpcType, base size=1. Spawn size=1, morph to size=2, revert via heavy path (`s.removeNpc(n, -1); s.addNpc(n, -1, false)` — mirrors NAI-19 Task 5e). Assert collision flags at `(startX+1, startZ)`, `(startX, startZ+1)`, `(startX+1, startZ+1)` are all FALSE (size=1 footprint only); assert flag at `(startX, startZ)` is TRUE. Requires a wired `s.gamemap` — uses the same `newTestServer` pattern that NAI-19's Task 5e collision tests use.
4. **`TestResetEntityForRespawnFirstSpawnDoesNotRaiseChangeTypeMask`** — first-spawn an NPC via `newRegisteredNpc(t, s, typ, true)`; assert `n.masks & rsbuf.NpcMaskChangeType == 0` immediately after.
5. **`TestResetEntityForRespawnRevertRaisesChangeTypeMask`** — set up a morphed NPC (`n.typeId = n.baseType + 1`); call `s.resetEntityForRespawn(n)` directly; assert `n.masks & rsbuf.NpcMaskChangeType != 0`.

### Task 3 — Cache + removeNpc polish bundle (Items C1 + C2 + C3, ~25 LOC prod, ~50 LOC tests)

**Goal**: three independent micro-fixes. Single commit, single review pass; commit message itemizes the three changes.

#### C1 — `cache.ResetCRCState()` helper

In `pkg/cache/crctable.go`, add an exported helper that re-initializes both `CrcBuffer` and `CrcTable` to their package-init shape:

```go
// ResetCRCState restores CrcBuffer and CrcTable to their package-init
// shape. Test-only convenience to avoid drift between init expressions
// and inline test resets.
func ResetCRCState() {
    CrcBuffer = packet.NewPacket(make([]byte, 0, 4*9))
    CrcTable = [9]int32{}
}
```

**Plan-time check**: read the actual init expressions for `CrcBuffer` and `CrcTable` at `pkg/cache/crctable.go:10` to confirm the helper body matches verbatim. If `CrcTable` has a non-zero init (e.g., a precomputed table), mirror that.

In `modules/world/world_test.go`, replace the two inline `cache.CrcBuffer = packet.NewPacket(make([]byte, 0, 4*9))` resets (test entry + `t.Cleanup`) with `cache.ResetCRCState()`.

#### C2 — `cache.MakeCRCs()` silent-error observability

In `pkg/cache/crctable.go:makeCrc`, replace the silent `return` on failure paths with `slog.Default().Warn`:

```go
import "log/slog"

func makeCrc(idx int, path string) {
    info, err := os.Stat(path)
    if err != nil {
        slog.Default().Warn("cache: makeCrc Stat failed",
            "idx", idx, "path", path, "err", err)
        return
    }
    p, err := packet.Load(path)
    if err != nil {
        slog.Default().Warn("cache: makeCrc Load failed",
            "idx", idx, "path", path, "err", err)
        return
    }
    // ... existing CRC computation unchanged ...
}
```

`slog.Default()` rather than threading a logger through `pkg/cache` — the package has zero logger plumbing today, and adding one would be a wider refactor than the follow-up warrants.

#### C3 — Lazy `adjustedDuration` in `removeNpc`

In `modules/world/npc_registry.go:135-156`, lift the `s.scaleByPlayerCount(duration)` call inside the RESPAWN+duration>-1 branch:

```go
func (s *Server) removeNpc(n *Npc, duration int) {
    // (drop the eager `adjustedDuration := s.scaleByPlayerCount(duration)` line)
    n.dead = true
    if s.gamemap != nil {  // updated for Task 2 snapshot
        switch n.blockWalk {
        case objtype.BlockWalkNPC:
            s.gamemap.ChangeNPCCollision(n.size, n.x, n.z, n.level, false)
        case objtype.BlockWalkAll:
            s.gamemap.ChangeNPCCollision(n.size, n.x, n.z, n.level, false)
            s.gamemap.ChangePlayerCollision(n.size, n.x, n.z, n.level, false)
        }
    }
    if n.lifecycle == NpcLifecycleDespawn {
        // ... unchanged ...
    } else if n.lifecycle == NpcLifecycleRespawn && duration > -1 {
        n.lifecycleTick = s.scaleByPlayerCount(duration)
    }
}
```

**Coordination with Task 2**: the collision toggle block in Task 2 also touches `removeNpc`. Both edits land in the same task or are separated by commit (Task 2's collision toggle changes land first; Task 3's lazy `adjustedDuration` is a follow-up commit). Plan-time decision: keep them in their respective task commits to preserve clean review-locality.

#### Tests

1. **`TestResetCRCStateRestoresInitialBuffer`** — write a non-empty `CrcBuffer` payload; call `ResetCRCState`; assert `cache.CrcBuffer.Len() == 0` and `cap(...) == 4*9`. Live in `pkg/cache/crctable_test.go` (new file).
2. **`TestMakeCrcWarnsOnMissingFile`** — capture `slog.Default()` output via a `slog.NewJSONHandler(&buf, ...)` swap with `t.Cleanup` to restore. Call `makeCrc(0, "/nonexistent/path")`; assert buf contains `"makeCrc Stat failed"` and `"path=/nonexistent/path"`. Live in `pkg/cache/crctable_test.go`.
3. **C3 has no dedicated test**. Per the followup memo, it's a micro-opt with no observable behavioral change; the existing `removeNpc` test coverage proves parity. Test-skip rationale documented inline in the spec; not a deviation.

### Task 4 — `checkNotNull` on three NPC opcode handlers (Item D, ~15 LOC prod, ~60 LOC tests)

**Goal**: wrap popped count parameters in `checkNotNull` (existing helper at `pkg/script/handlers_player.go:58`) for `handleNpcDelay` (NPC_DELAY, opcode 2511), `handleNpcQueue` (NPC_QUEUE, opcode 2530), and `handleNpcSetHunt` (NPC_SETHUNT, opcode 2533).

**Plan-time TS verification**: re-read each handler's TS counterpart at `Engine-TS/.../NpcOps.ts` to confirm the exact wrapped argument and argument order. The followup memo (NAI-2/3/7 entries) names the wrapped arguments as `delay` (handleNpcDelay), `delay` (handleNpcQueue — queueId already has explicit 1..20 range check), `range` (handleNpcSetHunt). Verify before code commits.

**Pattern** (per handler):

```go
func handleNpcDelay(s *ScriptState) error {
    delay, err := checkNotNull(s.popInt())
    if err != nil {
        return err
    }
    // ... existing suspension logic with `delay` ...
}
```

Same shape for `handleNpcSetHunt` (replace `delay` with `huntRange`). For `handleNpcQueue`, preserve the existing 1..20 range check on `queueId`; wrap only the `delay` pop:

```go
func handleNpcQueue(s *ScriptState) error {
    delay, err := checkNotNull(s.popInt())
    if err != nil {
        return err
    }
    queueId := s.popInt()
    if queueId < 1 || queueId > 20 {
        return fmt.Errorf("npc_queue: queueId %d out of range [1,20]", queueId)
    }
    // ... existing enqueue logic ...
}
```

**Plan-time pop-order check**: TS pop order is top-of-stack-first (last-pushed). Re-verify each handler's existing pop order at HEAD, then place the `checkNotNull` wrap at the correct pop site. The Go ordering may already match TS — confirm before reordering.

#### Tests (use existing `mockNpc` from `handlers_npc_test.go`)

Three negative-pin tests, mirroring the `TestHandleNpcSetTimerRejectsNegative` shape from S7b:

1. **`TestHandleNpcDelayRejectsNegativeDelay`** — push `-1`, run handler, assert error is non-nil and `mockNpc.Delay` was NOT called.
2. **`TestHandleNpcQueueRejectsNegativeDelay`** — push valid queueId + delay=`-1`, assert error and no enqueue.
3. **`TestHandleNpcSetHuntRejectsNegativeRange`** — push `-1`, assert error and no `SetHuntRange` call.

### Task 5 — Size-aware distance via existing `coordgrid.DistanceTo` (Item E, ~25 LOC prod, ~60 LOC tests)

**Goal**: rewire flagged size-approximated `DistanceToSW` and inline `max(|dx|,|dz|)` call sites to the size-aware `coordgrid.DistanceTo` function that already exists at `pkg/coordgrid/coordgrid.go:118`. **No library change** required.

#### Library API (already exists — referenced for clarity)

```go
// pkg/coordgrid/coordgrid.go:118
func DistanceTo(posX, posZ, posWidth, posLength,
    otherX, otherZ, otherWidth, otherLength int) int
```

Returns the Chebyshev distance between the closest pair of tiles in two rectangles. Per TS `CoordGrid.distanceTo(source, target)` semantics.

#### Site disposition table

| # | File:Line | Branch / TS Ref | Disposition |
|---|---|---|---|
| 1 | `npc_interaction.go:440` | PLAYERESCAPE retreat / TS Npc.ts:657-673 | **Rewire**: `DistanceTo(n.x, n.z, n.size, n.size, n.startX, n.startZ, 1, 1)` (start-coord is a point) |
| 2 | `npc_interaction.go:441` | PLAYERESCAPE retreat / TS Npc.ts:657-673 | **Rewire**: `DistanceTo(tx, tz, tw, tl, n.startX, n.startZ, 1, 1)` where `tw, tl` come from `approachEntitySize`-style helper |
| 3 | `npc_interaction.go:471` | checkApTrigger / TS Npc.ts:651-654 | **TBD at plan-time**: read TS source line; rewire if size-aware, else preserve `DistanceToSW` and pin reasoning in test |
| 4 | `npc_interaction.go:476` | default / TS Npc.ts:676 | **TBD at plan-time**: same as #3 |
| 5 | `npc_player_modes.go:54-62` | playerFaceCloseMode / TS Npc.ts:821-829 | **Rewire**: replace inline `max(|dx|, |dz|)` with `DistanceTo(n.x, n.z, n.size, n.size, tx, tz, tw, tl)`; `> 1` retained |
| 6 | `npc_player_modes.go:154` | playerEscapeMode 25-tile / TS Npc.ts:751-754 | **Rewire**: `DistanceTo(n.x, n.z, n.size, n.size, tx, tz, tw, tl)` |
| 7 | `npc_player_modes.go:172` | playerEscapeMode within-maxrange / TS Npc.ts:780-790 | **TBD at plan-time**: `(mx, mz)` is a candidate flee tile (the NPC's prospective position). Read TS to determine if it's compared with NPC size or as a point |

**Plan-time TS verification gate**: before any code commit, read TS source at each of the cited line refs (3 TBD rows). If the gate finds new size-aware sites, plan adjusts to match. If the TBD sites are intrinsically single-tile, document in spec amendment.

#### Sizing source

Reuse NAI-18's `approachEntitySize(target entity) int` helper for target dimensions (already in `modules/world/npc_interaction.go`); NPC self uses `n.size` (Task 2 snapshot). For non-entity targets (start-coord points, candidate tiles), use literal `1, 1`.

If `approachEntitySize` returns only width and TS distance-checks need both width and length, add a paired `approachEntityLength` helper at plan-time. For NPCs, TS `PathingEntity.width` and `PathingEntity.length` are always equal (= `NpcType.Size`), so width and length come from the same source — verify at plan-time.

#### Tests (per rewired site, ~2 tests each)

For each rewired site, add:
- One **size-asymmetry pin**: e.g., size=2 NPC at (3200,3200) vs target at (3201,3200) — `DistanceTo` returns 0 (overlap), `DistanceToSW` would return 1. Pins the TS-faithful behavior.
- One **size-1 parity pin**: where both ents are size=1, `DistanceTo(...)` result equals the prior `DistanceToSW(...)` result. Proves no regression on the dominant-case data (single-tile NPCs, which are 100% of current data).

Plus library-level coverage in `pkg/coordgrid/coordgrid_test.go` if not already exhaustive: assert `DistanceTo` size-asymmetry for adjacent rectangles (already tested if NAI-18 added them; verify at plan-time).

## Test Strategy

**Helper-first ordering**: Task 1 lands first; Tasks 2-5 use `newRegisteredNpc` from inception. No churn-rewrite within the sub-spec.

**Per-task test fixture summary**:

| Task | New tests | Helper usage | Fixture novelty |
|---|---|---|---|
| 1 (B) | 0 (helper exercised by ~11 conversions) | self | none |
| 2 (A) | 5 | yes | synthetic 2-tile NpcType (`Size: 2, BlockWalk: BlockWalkAll`) |
| 3 (C) | 2 (C1, C2; C3 untested) | partial | `slog.Default()` swap with `t.Cleanup` |
| 4 (D) | 3 | yes | reuse existing `mockNpc` from `handlers_npc_test.go` |
| 5 (E) | ~10-12 (2 per rewired site × 5+ sites) | yes | `coordgrid_test.go` size-asymmetry cases (verify NAI-18 didn't already cover) |

**Items without dedicated tests**:
- Task 3 C3 — micro-opt with no observable behavioral change. Existing `removeNpc` test coverage (NAI-19 Task 5c suite) proves parity. Test-skip rationale documented in spec.

**TS-source verification gates** (per `controller_preflight` memory):
- Task 5: 3 TBD rows in disposition table need TS reads at plan-time before code commits.
- Task 4: 3 handler shapes — verify exact wrapped argument names and pop order at TS source before code commits.

## Tracked Deviations

**Expected count: 0 new deviations.** Active deviation count remains 16 at NAI-20 close.

Every item retires a follow-up; nothing introduces a new TS divergence. If implementation discovers a previously-unflagged divergence (per `controller_preflight` rule), surface for spec amendment before silently adding it.

## Out of Scope

Explicitly punted:

- **huntPlayers deferred filters** (checkNotBusy / checkNotTooStrong / checkInv) — each blocked on missing infra (`Player.Busy`, wilderness detection, `InvTotal`). Separate per-filter sub-specs.
- **NAI-17-D1 closure** (revertType despawn+respawn alignment, ~150-250 LOC) — too big for this bundle. NAI-19 already partially addressed by structural-port path B; full alignment remains.
- **S7c-D1 reader** (`appearanceInv`) — needs smoke-test cadence; separate sub-spec.
- **NAI-17 cosmetic** (`NewNpc` stats-seeding loop modernization) — drive-by, not worth a dedicated commit. Pick up on next test-touching pass.
- **NAI-3 weak-form test strengthening** (NPC queue speedup quirk) — needs test-fixture infrastructure (`RegisterForTest` on `script.Provider` or test-only opcode). Separate sub-spec.
- **NAI-5 / NAI-19 fidelity audit** of `NumberNotNull`-equivalent gates beyond the 3 NPC handlers in Task 4 — sweep unscoped; may need separate audit pass.

## Cadence + Commit Shape

Per `runescript_cadence` memory (NAI series follows the same shape).

**Implementation mode**: subagent-driven-development (default per `execution_mode_default` memory).

**Per-task cycle** (5 tasks):
1. Plan-time TDD: write failing tests, verify-fail, implement, verify-pass, commit.
2. Two-stage review (spec compliance via opus → code quality via opus).
3. Polish commit if reviewer flags worth-fixing minors.

**Final review**: whole-impl review after Task 5 lands (spec-compliance + TS-fidelity).

**Commit message format** (per `runescript_cadence`):

```
<type>(world): NAI-20 Task <n> — <one-line summary>

<body>

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
```

Use `git commit --no-gpg-sign` for every commit. Types: `refactor` for Task 1 (no behavior change), `feat` for Task 2 (behavior tightening + mask gate), `polish` for Task 3 (cosmetic + observability + micro-opt bundle), `feat` for Task 4 (rejection paths gain validation), `polish` for Task 5 (size-aware rewires).

**Close commit** (per `close_commit_memory_trailer` memory):

```
chore(world): NAI-20 closed — follow-up bundle (5 tasks)

Closes 8 follow-up entries:
- nai_followups.md "From NAI-2"  → handleNpcDelay NumberNotNull
- nai_followups.md "From NAI-3"  → handleNpcQueue NumberNotNull
- nai_followups.md "From NAI-7"  → handleNpcSetHunt NumberNotNull
- nai_followups.md "From NAI-12" → size-aware DistanceTo at flagged sites
- nai_followups.md "From NAI-18" → orphaned DistanceToSW size approximations
- nai_followups.md "From NAI-19" cross-task collision-toggle/snapshot interaction
- nai_followups.md "From NAI-19" NpcMaskChangeType-on-first-spawn divergence
- nai_followups.md "From NAI-19" deferred test-scaffolding extraction
- nai_followups.md "From NAI-19" deferred cache-polish (ResetCRCState)
- nai_followups.md "From NAI-19" deferred error-handling (MakeCRCs slog.Warn)
- nai_followups.md "From NAI-19" deferred micro-opt (lazy adjustedDuration)

Active deviation count: 16 (unchanged)

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
```

**Post-close memory updates**:
- `nai_followups.md` — annotate each retired entry with `**Resolved 2026-04-25 (NAI-20 Task N)**` header pointing to the close commit hash.
- No new memory entries expected (this sub-spec doesn't establish new patterns).

## Risk Audit

| Risk | Mitigation |
|---|---|
| Task 5's TBD rows turn out to be size-aware once TS source is read, expanding scope mid-plan | Plan-time TS verification gate before any code commit; if scope grows >25 LOC, surface for spec amendment |
| Task 1's helper API doesn't fit all ~11 callers cleanly | Two-mode helper (`register bool`); plan-time audit pass enumerates each caller's needs before extraction |
| Task 2's collision-flag tests need a wired `s.gamemap` | Use `newTestServer` pattern that NAI-19 Task 5e collision tests use (already-proven fixture) |
| Task 3's C2 `slog.Default()` swap leaks across tests if `t.Cleanup` is forgotten | Test pattern includes `t.Cleanup(func() { slog.SetDefault(prev) })`; codified in plan |
| Task 1's helper conflicts with addNpc's internal slot allocation | Plan-time refinement: re-read addNpc + allocNpcSlot before finalizing helper signature |
| Plan-time TS reads disagree with the followup memo's claimed line refs | Per `controller_preflight` and `verify_implementer_claims` rules — verify against current Engine-TS HEAD; update plan accordingly. The followup memo's references are 1+ days old. |

## Spec Self-Review Checklist (inline)

- ✅ Placeholder scan: no TBDs except plan-time-resolution gates (Task 5 disposition table 3 rows; Task 4 pop-order verification). All TBDs have explicit resolution gates.
- ✅ Internal consistency: Task 2's `n.typ != nil` guard removal is mentioned and justified. Task 3's interaction with Task 2's `removeNpc` edit is called out.
- ✅ Scope check: 5 tasks × ~50-80 LOC each = ~250-400 LOC total (production + tests). Within typical NAI sub-spec ceiling.
- ✅ Ambiguity check: Task 5's TBD rows are explicitly flagged as plan-time-resolved, not silent ambiguity. Task 4's pop-order check is the same.
