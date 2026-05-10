# NAI-150 — PROJANIM cluster port (PROJANIM_MAP / PROJANIM_NPC / PROJANIM_PL)

## 1. Scope

Port the 3-handler PROJANIM cluster from TS Engine-TS `ServerOps.ts:171-210` to goscape. Cluster shares the existing `Zone.MapProjAnim` infrastructure (`pkg/zone/zone.go:355`, `modules/world/world_zone.go:164`); the only foundation gaps are two new `script.WorldVars` methods.

Closes 1 of the 4 user-reported "no handler for X" WARN classes from 2026-05-09/10 smoke logs (PROJANIM_NPC, opcode 2546). Bundle also retires PROJANIM_MAP (1018) and PROJANIM_PL (2091) from the cascade-tail. Cascade-tail: 39 → 36 unhandled.

## 2. Tech stack

Go 1.26+. Standard goscape packages: `pkg/script`, `modules/world`. No new dependencies.

## 3. TS source — `Engine-TS/src/engine/script/handlers/ServerOps.ts:171-210`

```ts
PROJANIM_PL: state => {
    const [srcCoord, uid, spotanim, srcHeight, dstHeight, delay, duration, peak, arc] = state.popInts(9);
    const srcPos: CoordGrid = check(srcCoord, CoordValid);
    const spotanimType: SpotanimType = check(spotanim, SpotAnimTypeValid);
    const player = World.getPlayerByUid(uid);
    if (!player) throw new Error(`attempted to use invalid player uid: ${uid}`);
    World.mapProjAnim(srcPos.level, srcPos.x, srcPos.z, player.x, player.z,
        -player.slot - 1, spotanimType.id, srcHeight * 4, dstHeight * 4,
        delay, duration, peak, arc);
}

PROJANIM_NPC: state => {
    const [srcCoord, npcUid, spotanim, srcHeight, dstHeight, delay, duration, peak, arc] = state.popInts(9);
    const srcPos: CoordGrid = check(srcCoord, CoordValid);
    const spotanimType: SpotanimType = check(spotanim, SpotAnimTypeValid);
    const slot = npcUid & 0xffff;
    // const _expectedType = (npcUid >> 16) & 0xffff;
    const npc = World.getNpc(slot);
    if (!npc) throw new Error(`attempted to use invalid npc uid: ${npcUid}`);
    World.mapProjAnim(srcPos.level, srcPos.x, srcPos.z, npc.x, npc.z,
        npc.nid + 1, spotanimType.id, srcHeight * 4, dstHeight * 4,
        delay, duration, peak, arc);
}

PROJANIM_MAP: state => {
    const [srcCoord, dstCoord, spotanim, srcHeight, dstHeight, delay, duration, peak, arc] = state.popInts(9);
    const spotanimType: SpotanimType = check(spotanim, SpotAnimTypeValid);
    const srcPos: CoordGrid = check(srcCoord, CoordValid);
    const dstPos: CoordGrid = check(dstCoord, CoordValid);
    World.mapProjAnim(srcPos.level, srcPos.x, srcPos.z, dstPos.x, dstPos.z,
        0, spotanimType.id, srcHeight * 4, dstHeight * 4,
        delay, duration, peak, arc);
}
```

Note PROJANIM_NPC uses **slot-only** lookup (`World.getNpc(slot)`); the `_expectedType` check is commented out in TS. PROJANIM_MAP validates spotanim **before** coords; PROJANIM_PL/NPC validate coord first then spotanim. The goscape port pins each handler's exact validation order against TS.

## 4. Existing surface (no change)

- `pkg/zone/zone.go:355` `Zone.MapProjAnim(srcX, srcZ, dstX, dstZ, target, spotanim, srcHeight, dstHeight, startDelay, endDelay, peak, arc)` — implementation already exists, no change.
- `modules/world/world_zone.go:164` `Server.MapProjAnim(level, srcX, srcZ, dstX, dstZ, target, spotanim, srcHeight, dstHeight, startDelay, endDelay, peak, arc)` — already exists, no change.
- `pkg/script/handlers_npc.go:13` `checkCoord(v, op) (level, x, z, err)` — reused.
- `pkg/script/handlers_map.go:213` `checkSpotAnimType(s, id, op)` — reused.
- `pkg/script/active.go:430` `ActivePlayer.Slot()`, `:449 X()`, `:454 Z()`, `:444 UID()` — reused for PROJANIM_PL.
- `pkg/script/active.go:670+` `ActiveNpc.NpcX()/NpcZ()/NpcLevel()/Nid()` — reused for PROJANIM_NPC.
- `pkg/script/state.go:121-126` `WorldVars.LookupPlayerByUID(uid) ActivePlayer` — reused for PROJANIM_PL.
- `pkg/script/opcode.go` — `OpProjAnimMap = 1018` (line 92), `OpProjAnimPl = 2091` (line 191), `OpProjAnimNpc = 2546` (line 283); name-table entries at lines 599 / 789 / 965. **All three already declared, none currently dispatched.** Confirmed via `grep -n PROJANIM pkg/script/handlers*.go pkg/script/dispatch*.go` returning no matches at HEAD `000d974`.

## 5. New surface

### 5.1. `script.WorldVars` extensions (`pkg/script/state.go`)

Append two methods to the `WorldVars` interface — `MapProjAnim` placed adjacent to the existing `AnimMap` (sibling broadcast op), `LookupNpcBySlot` placed adjacent to the existing `LookupPlayerByUID` (sibling entity-resolution op):

```go
// MapProjAnim broadcasts a projectile event from (level, srcX, srcZ) to
// (dstX, dstZ). target encodes the receiver: 0 = none (MAP→MAP),
// npc.nid+1 = NPC target, -player.slot-1 = player target.
// srcHeight/dstHeight are pre-scaled by the handler (×4).
// Mirrors TS World.mapProjAnim. Used by PROJANIM_MAP, PROJANIM_NPC,
// PROJANIM_PL. NAI-150.
MapProjAnim(level, srcX, srcZ, dstX, dstZ, target, spotanim,
    srcHeight, dstHeight, startDelay, endDelay, peak, arc int)

// LookupNpcBySlot resolves the NPC slot to its live ActiveNpc, or nil
// if the slot is out of range / unoccupied. Slot-only — does NOT verify
// the high-16 type bits, unlike NpcLookup.FindNpcByUID. Mirrors TS
// World.getNpc(slot). Used by PROJANIM_NPC. NAI-150.
LookupNpcBySlot(slot int) ActiveNpc
```

### 5.2. Production wiring (`modules/world/server_varp.go`)

Two methods on `worldVarsView`:

```go
// MapProjAnim implements script.WorldVars.MapProjAnim. Delegates to
// Server.MapProjAnim (modules/world/world_zone.go:164). NAI-150.
func (w worldVarsView) MapProjAnim(
    level, srcX, srcZ, dstX, dstZ, target, spotanim,
    srcHeight, dstHeight, startDelay, endDelay, peak, arc int,
) {
    if w.s == nil { return }
    w.s.MapProjAnim(level, srcX, srcZ, dstX, dstZ, target, spotanim,
        srcHeight, dstHeight, startDelay, endDelay, peak, arc)
}

// LookupNpcBySlot implements script.WorldVars.LookupNpcBySlot.
// Returns s.npcs[slot] cast to script.ActiveNpc, or nil for OOB slot
// or empty slot. Slot-only (does NOT verify type). NAI-150.
func (w worldVarsView) LookupNpcBySlot(slot int) script.ActiveNpc {
    if w.s == nil { return nil }
    if slot < 0 || slot >= len(w.s.npcs) { return nil }
    n := w.s.npcs[slot]
    if n == nil { return nil }
    return n
}
```

The `w.s == nil` guard mirrors the existing convention in `worldVarsView.LookupPlayerByUID` and other methods.

### 5.3. Mock WorldVars consumers (test files) — must compile-fix

Adding methods to `WorldVars` breaks every existing test impl. Three consumers identified via `grep -rln "func.*AnimMap(level" --include="*.go"` at HEAD `000d974`:

1. **`pkg/script/handlers_vars_test.go:51`** — `mockWorld` minimal stub. Add empty `MapProjAnim` (no-op) and `LookupNpcBySlot` (return nil).
2. **`pkg/script/handlers_map_test.go:423`** — `spotAnimMapWorld` recording mock. Add empty stubs (these tests do not exercise PROJANIM).
3. **`pkg/script/handlers_projanim_test.go`** (new) — `projAnimWorld` recording mock that captures both `MapProjAnim` 13-arg call and resolves `LookupNpcBySlot` from a slot→ActiveNpc map; see §6.3.

Plan-author preflight will re-grep at plan-write to catch any new consumer added between spec-write and plan-execution.

### 5.4. New file — `pkg/script/handlers_projanim.go`

Contains 3 handlers, each ~25-30 LOC. Stubbed code blocks per §6.

### 5.5. Dispatch entries (`pkg/script/dispatch.go`)

Three new entries appended under a new `// Server projectile ops` sub-comment:

```go
case OpProjAnimMap:
    return handleProjAnimMap(s)
case OpProjAnimNpc:
    return handleProjAnimNpc(s)
case OpProjAnimPl:
    return handleProjAnimPl(s)
```

Plan-author preflight: confirm the existing dispatch file path / switch shape (`case`-based vs map-based) at HEAD before codifying.

## 6. Handler bodies

### 6.1. `handleProjAnimMap` (opcode 1018)

```go
// handleProjAnimMap (PROJANIM_MAP, opcode 1018) queues a tile→tile
// projectile event broadcast to all players in the source zone.
// Mirrors TS ServerOps.ts:202-210.
func handleProjAnimMap(s *ScriptState) error {
    arc       := s.PopInt()
    peak      := s.PopInt()
    duration  := s.PopInt()
    delay     := s.PopInt()
    dstHeight := s.PopInt()
    srcHeight := s.PopInt()
    spotanim  := s.PopInt()
    dstCoord  := s.PopInt()
    srcCoord  := s.PopInt()

    // Validation order mirrors TS ServerOps.ts:205-207: spotanim, then srcCoord, then dstCoord.
    if err := checkSpotAnimType(s, spotanim, "PROJANIM_MAP"); err != nil {
        return err
    }
    srcLevel, srcX, srcZ, err := checkCoord(srcCoord, "PROJANIM_MAP")
    if err != nil {
        return err
    }
    _, dstX, dstZ, err := checkCoord(dstCoord, "PROJANIM_MAP")
    if err != nil {
        return err
    }

    s.World.MapProjAnim(srcLevel, srcX, srcZ, dstX, dstZ, 0,
        spotanim, srcHeight*4, dstHeight*4, delay, duration, peak, arc)
    return nil
}
```

Note: TS PROJANIM_MAP validates spotanim **first** (line 205), then srcCoord (206), then dstCoord (207) — different order from PROJANIM_PL/NPC. Pinned by test.

### 6.2. `handleProjAnimNpc` (opcode 2546)

```go
// handleProjAnimNpc (PROJANIM_NPC, opcode 2546) queues a tile→NPC
// projectile event with the NPC encoded as receiver via npc.Nid()+1.
// Slot-only NPC lookup — does NOT verify the high-16 expectedType bits
// (mirrors TS comment-out at ServerOps.ts:192). Mirrors TS
// ServerOps.ts:185-200.
func handleProjAnimNpc(s *ScriptState) error {
    arc       := s.PopInt()
    peak      := s.PopInt()
    duration  := s.PopInt()
    delay     := s.PopInt()
    dstHeight := s.PopInt()
    srcHeight := s.PopInt()
    spotanim  := s.PopInt()
    npcUid    := s.PopInt()
    srcCoord  := s.PopInt()

    // Validation order mirrors TS ServerOps.ts:188-189: srcCoord then spotanim.
    srcLevel, srcX, srcZ, err := checkCoord(srcCoord, "PROJANIM_NPC")
    if err != nil {
        return err
    }
    if err := checkSpotAnimType(s, spotanim, "PROJANIM_NPC"); err != nil {
        return err
    }

    slot := npcUid & 0xffff
    npc := s.World.LookupNpcBySlot(slot)
    if npc == nil {
        return fmt.Errorf("PROJANIM_NPC: invalid npc uid: %d", npcUid)
    }

    s.World.MapProjAnim(srcLevel, srcX, srcZ, npc.NpcX(), npc.NpcZ(),
        npc.Nid()+1, spotanim, srcHeight*4, dstHeight*4,
        delay, duration, peak, arc)
    return nil
}
```

### 6.3. `handleProjAnimPl` (opcode 2091)

```go
// handleProjAnimPl (PROJANIM_PL, opcode 2091) queues a tile→player
// projectile event with the player encoded as receiver via
// -player.Slot()-1. Mirrors TS ServerOps.ts:171-183.
func handleProjAnimPl(s *ScriptState) error {
    arc       := s.PopInt()
    peak      := s.PopInt()
    duration  := s.PopInt()
    delay     := s.PopInt()
    dstHeight := s.PopInt()
    srcHeight := s.PopInt()
    spotanim  := s.PopInt()
    uid       := s.PopInt()
    srcCoord  := s.PopInt()

    // Validation order mirrors TS ServerOps.ts:174-175: srcCoord then spotanim.
    srcLevel, srcX, srcZ, err := checkCoord(srcCoord, "PROJANIM_PL")
    if err != nil {
        return err
    }
    if err := checkSpotAnimType(s, spotanim, "PROJANIM_PL"); err != nil {
        return err
    }

    pl := s.World.LookupPlayerByUID(uid)
    if pl == nil {
        return fmt.Errorf("PROJANIM_PL: invalid player uid: %d", uid)
    }

    s.World.MapProjAnim(srcLevel, srcX, srcZ, pl.X(), pl.Z(),
        -pl.Slot()-1, spotanim, srcHeight*4, dstHeight*4,
        delay, duration, peak, arc)
    return nil
}
```

## 7. Test strategy

### 7.1. Per-handler tests (`pkg/script/handlers_projanim_test.go`)

Recording mock `projAnimWorld` implementing `script.WorldVars` with:
- `lastMapProjAnim` field of type `*projAnimCall` (struct mirroring the 13-arg shape) — captures the last call.
- `npcsBySlot map[int]script.ActiveNpc` — drives `LookupNpcBySlot`.
- `playersByUID map[int]script.ActivePlayer` — drives `LookupPlayerByUID`.

Tests (~12 cases, one per behavior pin):

**PROJANIM_MAP:**
1. `TestProjAnimMap_HappyPath` — push 9 ints, run, assert `lastMapProjAnim` matches the expected 13-arg shape exactly (including `target=0`, `srcHeight*4`, `dstHeight*4`).
2. `TestProjAnimMap_InvalidSrcCoord` — `srcCoord=-1` → error string `"PROJANIM_MAP: coord out of range (-1)"`, `lastMapProjAnim==nil`.
3. `TestProjAnimMap_InvalidDstCoord` — valid src, `dstCoord=2147483648` → error, no MapProjAnim.
4. `TestProjAnimMap_UnregisteredSpotanim` — id absent from `fakeConfigs.spotanimTypes` → error from `checkSpotAnimType`, no MapProjAnim.
5. `TestProjAnimMap_ValidationOrder` — invalid spotanim AND invalid srcCoord; assert spotanim error returns first (TS order at lines 205-207 is spotanim → src → dst).

**PROJANIM_NPC:**
6. `TestProjAnimNpc_HappyPath` — push `npcUid = (99 << 16) | 7`, register npc at slot 7 with `(typeId=42, nid=7, x=300, z=400)`. Assert lookup hit, `target = 7+1 = 8`, dst x/z match `300/400` (NOT the popped src), and **the lookup hit despite typeId mismatch** (pin TS comment-out of expectedType).
7. `TestProjAnimNpc_NilNpc` — slot empty → error `"PROJANIM_NPC: invalid npc uid: <uid>"`, no MapProjAnim.
8. `TestProjAnimNpc_ValidationOrder` — invalid src + invalid spotanim + nil npc; assert src-coord error first (TS order at lines 188-189 is src → spotanim → npc lookup).

**PROJANIM_PL:**
9. `TestProjAnimPl_HappyPath` — push `uid=12345`, register player with `(slot=4, x=500, z=600)`. Assert `target = -4-1 = -5`, dst x/z match `500/600`.
10. `TestProjAnimPl_NilPlayer` — uid not registered → error, no MapProjAnim.
11. `TestProjAnimPl_TargetEncodingPinSlotZero` — slot=0 → target=-1 (pin the off-by-one).
12. `TestProjAnimPl_HeightScaling` — `srcHeight=2, dstHeight=3` → recorded `srcHeight=8, dstHeight=12`.

### 7.2. Production wiring tests (`modules/world/server_varp_test.go` extension)

Two tests:
1. `TestWorldVarsView_MapProjAnim_Delegates` — construct minimal `Server` with a `zoneMap`, call `worldVarsView{s}.MapProjAnim(level, 3, 4, 5, 7, 0, 100, 10, 0, 0, 50, 40, 30)`, assert the same `(level, srcX, srcZ)` zone now has a tracked event of type `ZoneOpMapProjAnim` (delegate path through `Server.MapProjAnim` → `Zone.MapProjAnim`, mirroring the existing `TestMapProjAnimEnclosed` pattern at `pkg/zone/zone_test.go:307`).
2. `TestWorldVarsView_LookupNpcBySlot` — table-driven: empty server returns nil for any slot; OOB slot returns nil; populated slot returns the registered NPC; slot occupied by `nil` entry returns nil.

### 7.3. Pin against `WorldVars` interface conformance

Reuse the existing `var _ script.WorldVars = worldVarsView{}` assertion (or add one if absent) in `modules/world/server_varp.go` — this catches any future missing-method regression at compile time.

## 8. Cadence

Per `runescript_cadence.md` mid-band (100-300 LOC):

- Production: ~80 LOC (3 handlers + 2 worldVarsView methods).
- Tests: ~170 LOC (12 handler tests + 2 prod-wiring tests + recording mock).
- Mock-fix LOC: ~10 (3-line stubs in `mockWorld` and `spotAnimMapWorld`).
- **Total: ~260 LOC.**

Workflow: separate spec + plan, subagent-driven impl with TDD red→green per handler tier, single end-of-impl reviewer subagent on **Sonnet** (per `superpowers_code_reviewer_model.md`).

Plan tasks (preview):
1. T1 — extend `script.WorldVars` interface; fix-compile in `worldVarsView` + 2 mocks (stubs).
2. T2 — write 12 handler tests RED.
3. T3 — implement 3 handlers + dispatch entries → tests GREEN.
4. T4 — write 2 prod-wiring tests RED → fill out `worldVarsView.MapProjAnim`/`LookupNpcBySlot` with real logic → GREEN.
5. T5 — `go test ./...` + `go vet ./...` repo-wide.
6. T6 — final reviewer pass (Sonnet).

## 9. TS-fidelity deviations

None expected at spec-write time. Two TS quirks documented inline in handler doc-comments:

- **PROJANIM_NPC `_expectedType` skip** — TS comments out the type-check; goscape mirrors. Doc-comment at `handleProjAnimNpc` references `ServerOps.ts:192`.
- **PROJANIM_MAP validation-order quirk** — TS validates spotanim before coords (different from PROJANIM_PL/NPC); goscape mirrors. Pinned by `TestProjAnimMap_ValidationOrder`.

If the implementer surfaces a divergence during T2/T3, open as `NAI-150-D-<TAG>` and add to spec §9 before the close commit.

## 10. Cascade attribution

Closes user-reported smoke WARN: `script="<unknown>" err="no handler for PROJANIM_NPC (opcode 2546) at pc=N"` (2026-05-09/10 logs). Bundle close also retires PROJANIM_MAP and PROJANIM_PL from the unhandled-opcode list — neither was a confirmed smoke trigger but content scripts using `projanim_map` / `projanim_pl` will now execute instead of aborting.

Cascade-tail: 39 → 36 unhandled (post-NAI-149 baseline per nai_followups.md "From NAI-149" close note).

## 11. Risk register

- **R1 (low):** `Server.npcs[slot]` access from `worldVarsView.LookupNpcBySlot` — must be on the world goroutine. Existing `worldVarsView.LookupPlayerByUID` makes the same assumption (server.go:791 iterates `s.playerLoop` directly), so the convention is established. No locking added.
- **R2 (low):** `*Npc` satisfies both `pkg/zone.NpcLike` and `pkg/script.ActiveNpc` per the type assertion at `npc_script.go` (per the existing `serverNpcLookup.ZoneNpcs` pattern). Plan-author confirms at preflight.
- **R3 (low):** Smoke surfaces *additional* PROJANIM-adjacent divergences (e.g., `mapProjAnim` argument-shape mismatch with the client-side reader). Mitigated by `TestWorldVarsView_MapProjAnim_Delegates` reusing the existing `TestMapProjAnimEnclosed` zone-encoding test (pkg/zone/zone_test.go:307) which already pins the wire bytes.

## 12. Out of scope / non-goals

- Retiring `NpcLookup.FindNpcByUID` to live alongside `LookupNpcBySlot` on `WorldVars`. Symmetric refactor; touches NPC_FINDUID and risks NAI-120 regression. Open as a follow-up if it bothers us.
- Smoke-test execution. User runs the smoke after NAI-150 close per `smoke_test_server_handoff.md`.
- Any other unhandled opcodes from the cascade-tail (36 remaining post-NAI-150).
