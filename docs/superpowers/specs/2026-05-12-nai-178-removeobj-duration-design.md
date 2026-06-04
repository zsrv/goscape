# NAI-178 — RemoveObj duration port (NAI-115-D2 RemoveObj half close)

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` to implement this combined spec+plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

## Goal

Close the **RemoveObj half** of NAI-115-D2 (the AddObj half closed in NAI-177 at `20473cb`). Extend `(*Server).RemoveObj`, `script.WorldVars.RemoveObj`, and the adapter chain with a `duration int` parameter; port the full TS `World.removeObj` body (IsActive guard, scaleByPlayerCount, lifecycle-gated SetLifeCycle); wire OBJ_DEL/OBJ_TAKEITEM handlers to pass `ObjType.RespawnRate` per TS.

Fold a drive-by stale-comment cleanup at `pkg/script/state.go:130-138` (EnqueueObjDelayed claims "discarded at drain" — NAI-177 B0 closed that gap; the comment lies at HEAD).

User-visible behavior unlocked: **RESPAWN-lifecycle drops re-spawn after pickup or OBJ_DEL**. Mindrune ground spawns (and any other RESPAWN-cycle obj) will re-appear at `scaleByPlayerCount(RespawnRate)` ticks after removal.

## Tech stack

Go 1.26+ (per `go_version.md`). Sources of truth pinned at HEAD `49bfb09`:

- TS canonical: `LostCityRS/Engine-TS/` (per `ts_source_canonical_path.md`).
- Spec/plan combined per `compressed_cadence.md`.

## Cadence

**Compressed** — single combined spec+plan doc, subagent-driven-development for T1..T5 + close. Estimated ~50 LOC production + ~140 LOC tests = ~190 LOC total. Well under NAI-177's 285-LOC precedent.

## Test command prefix

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/zone/... ./pkg/entity/... ./modules/world/... ./pkg/script/...
```

## Commit prefix

`git commit --no-gpg-sign ...` (per global CLAUDE.md).

---

## §1. TS source of truth

### World.removeObj (Engine-TS `World.ts:1500-1518`)

```ts
// Dev note: this function is slightly awkward, might need reworked
removeObj(obj: Obj, duration: number): void {
    // Obj must be active to remove it from the world. An inactive Obj is already removed
    if (!obj.isActive) {
        return;
    }
    // printDebug(`[World] removeObj => name: ${ObjType.get(obj.type).name}, duration: ${duration}`);
    const zone: Zone = this.gameMap.getZone(obj.x, obj.z, obj.level);
    const adjustedDuration = this.scaleByPlayerCount(duration);
    zone.removeObj(obj);
    this.trackZone(zone);

    // If the duration is positive and the Obj is a static obj, queue the Obj to respawn
    if (duration > 0 && obj.lifecycle === EntityLifeCycle.RESPAWN) {
        obj.setLifeCycle(adjustedDuration);
    } else {
        obj.setLifeCycle(-1);
    }
}
```

### OBJ_DEL (Engine-TS `ObjOps.ts:112-119`)

```ts
[ScriptOpcode.OBJ_DEL]: state => {
    const duration: number = ObjType.get(state.activeObj.type).respawnrate;
    if (state.pointerGet(ActivePlayer[state.intOperand])) {
        World.removeObj(state.activeObj, duration);
    } else {
        World.removeObj(state.activeObj, duration);
    }
},
```

Both arms are identical (TS-side oddity — the `pointerGet` gate has no functional effect). Goscape collapses to a single unconditional call; documented inline rather than as a separate deviation tag.

### OBJ_TAKEITEM lifecycle branch (Engine-TS `ObjOps.ts:156-160`)

```ts
if (obj.lifecycle === EntityLifeCycle.RESPAWN) {
    World.removeObj(obj, objType.respawnrate);
} else if (obj.lifecycle === EntityLifeCycle.DESPAWN) {
    World.removeObj(obj, 0);
}
```

Note TS branches `RESPAWN` and `DESPAWN` explicitly; `FOREVER`-lifecycle objs fall through with no removal. Goscape preserves the same shape via `IsRespawnLifecycle()` (see §3 B1).

---

## §2. Existing goscape state (HEAD `49bfb09`)

### Already in place

- `(*Server).RemoveObj(obj *entitypkg.Obj)` at `modules/world/world_zone.go:148-152` — current 4-line body (no duration, no IsActive guard, no SetLifeCycle).
- `(*Server).scaleByPlayerCount(rate int) int` at `modules/world/server.go:764` — TS-pinned at `server_test.go:707`.
- `(*Zone).RemoveObj(obj, currentTick)` at `pkg/zone/zone.go:302` — sets `obj.IsActive = false`; respects `LastLifecycleTick == currentTick` no-OBJ_DEL-event short-circuit.
- `s.locObjTracker` — shared registry; `NonPathing.SetLifeCycle(duration, currentTick, tracker)` at `pkg/entity/nonpathing.go:41` handles register/unregister via `duration > 0` vs `duration <= 0`.
- `entity.Lifecycle` typed-int + `LifecycleRespawn` / `LifecycleDespawn` constants at `pkg/entity/lifecycle.go:5-11`.
- `script.ActiveObj` interface at `pkg/script/active.go:1031-1036` — narrow, 4 methods. `mockActiveObj` test fixture at `handlers_npc_test.go:2672-2690`.
- `objtype.ObjType.RespawnRate int` field — confirmed populated by cache loader at `pkg/objtype/objtype.go:166` (referenced by NAI-177 spec §2).

### Call sites that pass `RemoveObj(obj)` today (verified via `grep -rn "\.RemoveObj(" modules/ pkg/`)

| Site | Caller intent | TS-equivalent duration |
|---|---|---|
| `modules/world/server_varp.go:141` | adapter sink | plumbed from interface caller |
| `modules/world/world_zone.go:150` | zone-side sink (recursive — `z.RemoveObj`, not `s.RemoveObj`); n/a | — |
| `modules/world/obj_turn.go:42` | DESPAWN-arm of turnObj | `0` (TS `Obj.ts:39`) |
| `pkg/script/handlers_obj.go:154` | OBJ_DEL | `ObjType.respawnrate` (TS `ObjOps.ts:113`) |
| `pkg/script/handlers_obj.go:283` | OBJ_TAKEITEM | RESPAWN → `respawnrate`; DESPAWN → `0` (TS `ObjOps.ts:156-160`) |

### Test mocks that satisfy `WorldVars.RemoveObj` today

| File:Line | Owner |
|---|---|
| `pkg/script/handlers_obj_test.go:40` | `fakeWorldRemoveObj.RemoveObj(obj)` |
| `pkg/script/handlers_obj_test.go:448` | `fakeWorldTakeItem.RemoveObj(obj)` |
| `pkg/script/handlers_vars_test.go:73` | `mockWorld.RemoveObj(obj)` (no-op) |

### NAI-115-D2 doc-comment sites (retire on close)

```
pkg/script/state.go:98          → RemoveObj interface body
pkg/script/handlers_obj.go:140  → OBJ_DEL header
pkg/script/handlers_obj.go:229  → OBJ_TAKEITEM header
modules/world/server_varp.go:130 → worldVarsView.RemoveObj header
```

### Stale doc-comment (NAI-177 cleanup miss, drive-by here)

`pkg/script/state.go:130-138` says: *"duration is plumbed through but currently discarded at drain (NAI-115-D2 foundation gap; mirrors worldVarsView.AddObj's existing `_ = duration`)"*. NAI-177 B0 closed the drain at `20473cb`; the comment now misrepresents HEAD. Folded into B3 below.

---

## §3. Bundle map

### B0 — Producer body port (`Server.RemoveObj`)

**Modify** `modules/world/world_zone.go:148-152`:

```go
// RemoveObj routes an obj removal and reschedules respawn (RESPAWN) or
// untracks (DESPAWN/FOREVER). Mirrors TS World.removeObj
// (Engine-TS/src/engine/World.ts:1500-1518).
//
// IsActive=false is written by the called Zone.RemoveObj (pkg/zone/zone.go:312),
// matching TS Zone.removeObj.
//
// duration > 0 + RESPAWN lifecycle → schedules respawn via
// NonPathing.SetLifeCycle (registering the obj in s.locObjTracker for
// per-tick processing). All other shapes (DESPAWN, FOREVER, or
// duration<=0) untrack with SetLifeCycle(-1, ...).
//
// duration is scaled by current player count (low-pop worlds get faster
// respawns) — see scaleByPlayerCount at server.go:764.
func (s *Server) RemoveObj(obj *entitypkg.Obj, duration int) {
    if !obj.IsActive {
        return
    }
    adjustedDuration := s.scaleByPlayerCount(duration)
    z := s.zoneMap.Get(obj.Level, obj.X, obj.Z)
    z.RemoveObj(obj, s.currentTick)
    s.TrackZone(z)
    if duration > 0 && obj.Lifecycle == entitypkg.LifecycleRespawn {
        obj.SetLifeCycle(adjustedDuration, s.currentTick, s.locObjTracker)
    } else {
        obj.SetLifeCycle(-1, s.currentTick, nil)
    }
}
```

**Update call sites:**
- `modules/world/obj_turn.go:42` — `s.RemoveObj(o)` → `s.RemoveObj(o, 0)` (TS `Obj.ts:39`: `World.removeObj(this, 0)`).
- `modules/world/server_varp.go:141` — plumb the new `duration` from the adapter signature (see B1).

### B1 — Interface extension (`ActiveObj.IsRespawnLifecycle`)

**Modify** `pkg/script/active.go:1031-1036`:

```go
// ActiveObj is the surface that OBJ_* and AI_APOBJ/AI_OPOBJ handlers
// use to read obj state. Narrow by design — extend as future sub-specs
// wire more obj script opcodes.
type ActiveObj interface {
    ObjType() int                  // underlying ObjType id
    Coords() (x, z, level int)     // world position
    ObjCount() int                 // current stack size
    IsValidFor(playerUID int) bool // private-receiver + count>0
    // IsRespawnLifecycle reports whether the obj is RESPAWN-lifecycle
    // (engine-spawned, comes back after a timer). Used by OBJ_TAKEITEM
    // to gate the respawn-duration arg passed to WorldVars.RemoveObj
    // per TS ObjOps.ts:156-160. NAI-178.
    IsRespawnLifecycle() bool
}
```

**Rationale for `IsRespawnLifecycle() bool` over `ObjLifecycle() int`:** pkg/script does not import pkg/entity (the narrow-interface boundary held since NAI-153). Returning the typed-int would force script-side lifecycle constants and create a silent coupling to entity's `iota` ordering. The bool is purpose-built for the OBJ_TAKEITEM RESPAWN gate — the only TS handler that branches on lifecycle in the obj-side opcode family.

**Add to `pkg/entity/obj.go`** (after `IsValidFor`, line 85+):

```go
// IsRespawnLifecycle reports whether o is engine-spawned RESPAWN
// lifecycle. Satisfies script.ActiveObj. NAI-178.
func (o *Obj) IsRespawnLifecycle() bool {
    return o.Lifecycle == LifecycleRespawn
}
```

### B2 — Adapter extension (`worldVarsView.RemoveObj`)

**Modify** `pkg/script/state.go:97-102`:

```go
// RemoveObj despawns / removes the given obj from its zone. Mirrors
// TS World.removeObj. duration drives the RESPAWN-after-pickup
// re-spawn timer when obj.IsRespawnLifecycle (else untracks). Used by
// OBJ_DEL, OBJ_TAKEITEM.
RemoveObj(obj ActiveObj, duration int)
```

**Modify** `modules/world/server_varp.go:126-142`:

```go
// RemoveObj implements script.WorldVars.RemoveObj. Type-asserts the
// script-side ActiveObj to the world-side *entitypkg.Obj and routes
// via Server.RemoveObj with the caller's respawn duration.
func (w worldVarsView) RemoveObj(obj script.ActiveObj, duration int) {
    if w.s == nil {
        return
    }
    realObj, ok := obj.(*entitypkg.Obj)
    if !ok {
        return
    }
    w.s.RemoveObj(realObj, duration)
}
```

### B3 — Handler wiring + drive-by

**Modify** `pkg/script/handlers_obj.go:137-156` (OBJ_DEL):

```go
// handleObjDel (OBJ_DEL, opcode 3504) removes the active obj. Mirrors
// TS ObjOps.ts:112-119.
//
// TS branches on `pointerGet(ActivePlayer)` but both arms call identical
// World.removeObj(activeObj, duration) — collapsed here to a single
// unconditional call (TS-side oddity).
//
// duration is ObjType.RespawnRate; Server.RemoveObj gates on
// lifecycle+duration to decide between respawn-scheduling and untrack.
func handleObjDel(s *ScriptState) error {
    if err := requireActiveObj(s, "OBJ_DEL"); err != nil {
        return err
    }
    if s.World == nil {
        return fmt.Errorf("OBJ_DEL: no world surface")
    }
    duration := 0
    if s.Configs != nil {
        if objCfg := s.Configs.ObjType(s.ActiveObj.ObjType()); objCfg != nil {
            duration = objCfg.RespawnRate
        }
    }
    s.World.RemoveObj(s.ActiveObj, duration)
    return nil
}
```

**Modify** `pkg/script/handlers_obj.go:212-285` (OBJ_TAKEITEM doc header + RemoveObj call):

Strip the NAI-115-D2 doc block at L229-231 from the header.

Replace the final `s.World.RemoveObj(s.ActiveObj, 0)`-equivalent call (currently `s.World.RemoveObj(s.ActiveObj)` at L283) with the TS lifecycle branch:

```go
duration := 0
if s.ActiveObj.IsRespawnLifecycle() {
    if objCfg := s.Configs.ObjType(s.ActiveObj.ObjType()); objCfg != nil {
        duration = objCfg.RespawnRate
    }
}
s.World.RemoveObj(s.ActiveObj, duration)
```

(`s.Configs` is non-nil at this point — happy-path guarded by the wealth-event block at L268 which already reads `s.Configs.ObjType`. But the `if objCfg != nil` nested guard matches the wealth-event block's defensiveness one-for-one. The DESPAWN arm naturally falls through to `duration = 0` without an explicit `else`.)

**Modify** `pkg/script/state.go:130-138` (B3 drive-by, NAI-177 cleanup miss):

```go
// EnqueueObjDelayed appends an INV_DROPITEM_DELAYED request to the
// world's per-tick spawn-delay queue. The Obj is constructed at the
// implementation side (worldVarsView in modules/world). Mirrors TS
// World.objDelayedQueue.addTail at InvOps.ts:208. Used by INV_DROPITEM_DELAYED.
EnqueueObjDelayed(level, x, z, typeID, count, duration, delay, receiverID int)
```

(4 lines deleted: the "duration is plumbed through but currently discarded at drain (NAI-115-D2 foundation gap...)" block. NAI-177 B0 closed the drain at `20473cb`.)

### B4 — Test mock updates

- `pkg/script/handlers_obj_test.go:35-42` — `fakeWorldRemoveObj.RemoveObj` adds `duration int`. Record durations alongside obj refs: replace `removed []ActiveObj` with `removed []removeObjCall{obj ActiveObj; duration int}` to support B5 duration-assertion tests.
- `pkg/script/handlers_obj_test.go:442-450` — `fakeWorldTakeItem.RemoveObj` same shape change.
- `pkg/script/handlers_vars_test.go:73` — `mockWorld.RemoveObj(obj ActiveObj, duration int) {}`.
- `pkg/script/handlers_npc_test.go:2672-2690` — `mockActiveObj` adds `respawnLifecycle bool` field + `IsRespawnLifecycle() bool` method.
- All existing call-site assertions reading `removed[0] != active` become `removed[0].obj != active` (~3 sites: `:54`, `:501`, plus any B5 additions).

---

## §4. Test surface (10 new pins)

### B0 producer tests — `modules/world/world_zone_obj_lifecycle_test.go` (extend NAI-177's file)

1. **`TestServerRemoveObj_InactiveObjEarlyReturns`** — set `obj.IsActive=false`; call `s.RemoveObj(o, 50)`; assert `o.LifecycleTick` unchanged (still constructor's `-1`); assert tracker.Has(o) unchanged.
2. **`TestServerRemoveObj_RespawnLifecycleSetsAdjustedLifecycleTick`** — active RESPAWN obj, `duration=50`, fixture playerCount=0 (empty `s.players`); call `s.RemoveObj`; assert `o.LifecycleTick == s.currentTick + 50` (scale identity).
3. **`TestServerRemoveObj_RespawnLifecycleScalesByPlayerCount`** — RESPAWN obj, `duration=4000`, fixture playerCount=2000 (populate `s.players` with 2000 stubs OR use a TS-formula assertion `((4000-pc)*4000)/4000` against an arbitrary `pc`); assert `o.LifecycleTick == s.currentTick + 2000` (halved).
4. **`TestServerRemoveObj_DespawnLifecycleSetsLifecycleTickNegOne`** — DESPAWN-lifecycle active obj, `duration=50`; assert `o.LifecycleTick == -1` (gate denies).
5. **`TestServerRemoveObj_ZeroDurationSetsLifecycleTickNegOne`** — RESPAWN-lifecycle active obj, `duration=0`; assert `o.LifecycleTick == -1` (`duration > 0` gate denies).
6. **`TestServerRemoveObj_RespawnLifecycleRegistersTracker`** — RESPAWN obj, `duration>0`; assert `s.locObjTracker` contains the obj after the call (mirrors `SetLifeCycle` Register branch at `nonpathing.go:50-55`).

### B2 adapter test — `modules/world/server_varp_test.go` (extend)

7. **`TestWorldVarsViewRemoveObj_PlumbsDuration`** — construct active RESPAWN obj, call `w.RemoveObj(obj, 42)` via adapter (no scale, single-player world); assert `obj.LifecycleTick == s.currentTick + 42`.

### B3 handler tests — `pkg/script/handlers_obj_test.go` (extend)

8. **`TestHandleObjDel_PassesRespawnRateFromObjType`** — register ObjType with `RespawnRate=200`; set active obj; call OBJ_DEL; assert `w.removed[0].duration == 200`.
9. **`TestHandleObjTakeItem_RespawnLifecyclePassesRespawnRate`** — RESPAWN-lifecycle active obj, ObjType.RespawnRate=300; happy path; assert `w.removed[0].duration == 300`.
10. **`TestHandleObjTakeItem_DespawnLifecyclePassesZero`** — DESPAWN-lifecycle active obj, ObjType.RespawnRate=300; happy path; assert `w.removed[0].duration == 0`.

### No new tests for B4

Test-mock updates are mechanical signature changes consumed by tests 1-10.

---

## §5. Deviations

### New deviations
**None.** All TS arms map 1:1. The OBJ_DEL `pointerGet` collapse is a TS-side oddity (both arms identical); inline note in the handler header documents.

### Retired tags after close

- **NAI-115-D2 (RemoveObj half)** — `Server.RemoveObj` and `worldVarsView.RemoveObj` and OBJ_DEL and OBJ_TAKEITEM now honor duration. Combined with NAI-177's AddObj-half retirement, the full `NAI-115-D2` tag fully retires; no remaining production/test/doc sites reference it. (Spec-doc references in `docs/superpowers/specs/` remain historical.)

### Design choice — `IsRespawnLifecycle() bool` (not `ObjLifecycle() int`)

Documented at §3 B1. Not a TS deviation (TS only branches on RESPAWN); a Go-side API-shape choice to preserve the pkg/script ↔ pkg/entity boundary.

---

## §6. Implementation tasks

### T1 — B0 producer body port (RED → GREEN)

- [ ] T1.1 — Write failing producer tests 1-6 from §4 in `modules/world/world_zone_obj_lifecycle_test.go`.
- [ ] T1.2 — Modify `(*Server).RemoveObj` signature + body per §3 B0.
- [ ] T1.3 — Update non-test call sites: `obj_turn.go:42` (`s.RemoveObj(o, 0)`), `server_varp.go:141` (plumb new param).
- [ ] T1.4 — Verify tests 1-6 pass; verify `go build ./modules/world/...` clean post-signature change.

### T2 — B1 interface + entity method

- [ ] T2.1 — Extend `script.ActiveObj` with `IsRespawnLifecycle() bool` per §3 B1.
- [ ] T2.2 — Add `(*entity.Obj).IsRespawnLifecycle()` method per §3 B1.
- [ ] T2.3 — Update `mockActiveObj` (`pkg/script/handlers_npc_test.go:2672`) with `respawnLifecycle bool` field + method.
- [ ] T2.4 — Verify `go build ./pkg/...` clean.

### T3 — B2 adapter wiring

- [ ] T3.1 — Update `script.WorldVars.RemoveObj` interface signature + doc per §3 B2.
- [ ] T3.2 — Update `worldVarsView.RemoveObj` adapter per §3 B2.
- [ ] T3.3 — Update test mocks: `fakeWorldRemoveObj`, `fakeWorldTakeItem`, `mockWorld.RemoveObj` per §3 B4. Convert `removed []ActiveObj` → `removed []removeObjCall`. Migrate the 3 existing assertion sites that read `removed[0]`/`removed[0] != active` to `removed[0].obj`.
- [ ] T3.4 — Write adapter test 7 from §4.
- [ ] T3.5 — Verify all packages build + all existing tests still green.

### T4 — B3 handler wiring + drive-by

- [ ] T4.1 — Write failing handler tests 8-10 from §4.
- [ ] T4.2 — Update OBJ_DEL handler body per §3 B3 (read respawnrate, pass to RemoveObj). Strip NAI-115-D2 header block.
- [ ] T4.3 — Update OBJ_TAKEITEM final call per §3 B3 (lifecycle-branched duration). Strip NAI-115-D2 header block.
- [ ] T4.4 — Verify tests 8-10 pass; verify existing OBJ_TAKEITEM happy-path tests still green (`HappyPath` uses mindrune which is RESPAWN-lifecycle in TS — fixture must set `respawnLifecycle: true`).
- [ ] T4.5 — Strip stale EnqueueObjDelayed doc-comment at `state.go:130-138` per §3 B3 drive-by.
- [ ] T4.6 — `rg "NAI-115-D2" pkg/ modules/` should return zero hits.

### T5 — Code review pass

- [ ] T5.1 — End-of-impl reviewer pass on Sonnet (per `superpowers_code_reviewer_model`).

### Close — NAI-178

- [ ] CLOSE.1 — Update `nai_followups.md`: add NAI-178 close section; mark `NAI-115-D2` fully retired (RemoveObj half + AddObj half both closed).
- [ ] CLOSE.2 — Close commit with `Closes memory:` trailer.

---

## §7. Plan-author pre-flight reminders

Per `controller_preflight.md`, the controller should re-verify these at HEAD before each task dispatch:

1. **`Server.RemoveObj` call-site enumeration** (T1.3): `rg "\\.RemoveObj\\(" modules/ pkg/ | rg -v "z\\.RemoveObj"` — the §2 enumeration is a HEAD `49bfb09` snapshot; mid-NAI-178 additions may exist. Exclude `z.RemoveObj` (zone-side, different sig).
2. **`mockActiveObj` consumers** (T2.3): `rg "mockActiveObj\\{" pkg/script/` — every `&mockActiveObj{...}` literal must still compile after adding `respawnLifecycle` (zero-value `false` is the natural default for existing tests that don't care about lifecycle). Verify no struct uses unkeyed positional literals.
3. **Test 3 (player-count scaling)**: easiest to assert via `s.scaleByPlayerCount(50)` as the expected value rather than re-deriving the arithmetic — keeps the test from rotting if the scaling formula ever changes. Sister-test precedent: `server_test.go:707 TestScaleByPlayerCountFormula`.
4. **`s.Configs` nil-guard in OBJ_DEL** (T4.2): pkg/script tests routinely have `s.Configs == nil` (e.g., `TestHandleObjDelNilActive`). The body must guard; otherwise existing tests crash. Mirror OBJ_TAKEITEM's existing `if objCfg := s.Configs.ObjType(...); objCfg != nil` pattern from the wealth-event block at L268-281.
5. **Existing OBJ_TAKEITEM `HappyPath` fixture** (T4.4): `newTakeItemFixture` at L461 uses mindrune (id 558). Mindrune is RESPAWN-lifecycle in TS. Setting `mockActiveObj{... respawnLifecycle: true}` and `mindrune.RespawnRate = X` will assert that the existing happy path still passes through the respawn arm. The existing `expected 1 RemoveObj call with active` assertion changes to `w.removed[0].obj != active`.
6. **`s.locObjTracker` test fixture** (T1.1): tests 2/5/6 need `s.locObjTracker` populated on the test Server. Constructor is `newLocObjTracker()` at `modules/world/loc_tracker.go:23`. NAI-177's existing `newLocTurnTestServer` pattern (or equivalent) likely already wires this — reuse it for the new B0 tests.
7. **Zone.RemoveObj LastLifecycleTick gate** (T1.1): `Zone.RemoveObj` at `pkg/zone/zone.go:314-316` early-returns the OBJ_DEL event emit if `obj.LastLifecycleTick == currentTick`. Tests should NOT trigger this gate (would obscure assertions); ensure `obj.LastLifecycleTick != s.currentTick` in fixtures (default `-1` from constructor — safe).
8. **Lifecycle field defaults on `*entity.Obj`** (T1.1, test 4): `NewObj(level, x, z, lc, typ, count)` requires explicit lifecycle. Test fixtures must pass `LifecycleRespawn` / `LifecycleDespawn` explicitly — there is no zero-value lifecycle (`LifecycleForever = 0`, which would fall through the RESPAWN gate as expected, but be explicit).
9. **`fakeWorldRemoveObj.removed` struct migration** (T3.3): the field rename `[]ActiveObj` → `[]removeObjCall` ripples to existing assertions `len(w.removed) != 1 || w.removed[0] != active` at handlers_obj_test.go L54, L501, L522, L540. Migrate all four; assertion shape becomes `len(w.removed) != 1 || w.removed[0].obj != active`.

---

## §8. Self-review checklist

Run after writing this doc (per brainstorming skill §Spec Self-Review):

### Placeholder scan

- [x] No "TBD"/"TODO"/"XXX" placeholders.
- [x] All file paths exist at HEAD `49bfb09`.
- [x] All line numbers verified at HEAD via the exploration grep enumeration.

### Internal consistency

- [x] §1 TS source matches §3 Go translation 1:1 (IsActive guard, scale, SetLifeCycle gate).
- [x] B0 producer body change and B1-B3 adapter/handler changes are independently testable (T1.1 RED before T2-T4 dispatch).
- [x] §3 file map matches §6 task list — T1 = B0, T2 = B1, T3 = B2+B4, T4 = B3.
- [x] All 10 tests in §4 trace to a specific bundle in §3.

### Scope check

- [x] Compressed cadence (~190 LOC) — well under NAI-177's 285-LOC precedent.
- [x] NAI-115-D2 fully retires after close. No partial-close state.
- [x] B3 drive-by (stale EnqueueObjDelayed comment) is 4-line text strip; no scope creep.
- [x] B1 interface extension is purpose-built (IsRespawnLifecycle); not a speculative addition.

### Ambiguity check

- [x] §3 B1 spells out the `IsRespawnLifecycle() bool` vs `ObjLifecycle() int` design choice with rationale.
- [x] §3 B3 OBJ_TAKEITEM body shows the nested `objCfg != nil` guard pattern explicitly (matches wealth-event block at L268).
- [x] §5 explicitly notes "no new deviations" — saves the implementer a hunt.
- [x] §7 #5 explicitly calls out that `newTakeItemFixture`'s mindrune is RESPAWN-lifecycle and tests need updating.

---

## §9. Smoke binding (post-close, optional)

User-launched server + Java client (per `smoke_test_server_handoff`):

- **Respawn after pickup:** Log in. Walk to a known RESPAWN-lifecycle ground spawn (e.g., mindrune ground tile — already in NAI-152 smoke set). Pick up the obj. Wait `scaleByPlayerCount(mindrune.RespawnRate)` ticks (~30-60s single-player). Confirm the obj reappears on the tile.
- **DESPAWN drop stays gone:** Drop a tradeable item (DESPAWN-lifecycle); pick it back up; confirm it does NOT reappear.

Non-binding per `cascade_theory_smoke_binding.md` — if no content path reaches the RESPAWN-cycle obj during smoke, the producer-side unit tests are sufficient.
