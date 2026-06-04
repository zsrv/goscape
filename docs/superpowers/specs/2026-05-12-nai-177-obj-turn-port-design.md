# NAI-177 — Obj.turn port + AddObj producer wiring (compressed cadence)

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` to implement this combined spec+plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

## Goal

Port TS `Obj.turn` (Engine-TS `src/engine/entity/Obj.ts:27-50`) to goscape, wiring the Obj-side lifecycle/reveal countdowns that have been dead in goscape since NAI-86 closed. Retire two tracked deviation tags:

- **NAI-86 D-N86-3** (placeholder at `modules/world/tick.go:621`) — consumer side.
- **NAI-115-D2 (AddObj half)** — producer side: `Server.AddObj` accepts `duration` and initializes `obj.LifecycleTick` + `obj.Reveal`.

Fold a drive-by stale-comment cleanup at `pkg/script/handlers_loc.go:262-266`.

## Tech stack

Go 1.26+ (per `go_version.md`). Sources of truth pinned at HEAD `0650edd`:

- TS canonical: `LostCityRS/Engine-TS/` (per `ts_source_canonical_path.md`).
- Spec/plan combined per `compressed_cadence.md`.

## Cadence

**Compressed** — single combined spec+plan doc, subagent-driven-development for T1..T6 + close. Estimated ~75 LOC production + ~210 LOC tests = ~285 LOC total. Right at the upper edge of compressed cadence; if a task exceeds plan scope mid-TDD, the controller may split T-N off into a per-task plan rather than expanding this doc.

## Test command prefix

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/zone/... ./pkg/entity/... ./modules/world/... ./pkg/script/...
```

## Commit prefix

`git commit --no-gpg-sign ...` (per global CLAUDE.md).

---

## §1. TS source of truth

### Obj.turn (Engine-TS `entity/Obj.ts:27-50`)

```ts
turn() {
    if (this.reveal > -1 && --this.reveal === 0) {
        World.revealObj(this);
    }

    // Decrement lifecycle tick
    --this.lifecycleTick;

    if (this.lifecycleTick === 0) {
        if (this.lifecycle === EntityLifeCycle.DESPAWN && this.isActive) {
            World.removeObj(this, 0);
        } else if (this.lifecycle === EntityLifeCycle.RESPAWN && !this.isActive) {
            World.addObj(this, Obj.NO_RECEIVER, 0);
        } else {
            this.setLifeCycle(-1);
            console.error('Obj is tracked but has no event');
        }
    } else if (this.lifecycleTick < 0) {
        this.setLifeCycle(-1);
        console.error('Obj is tracked but has a negative lifecycle tick');
    }
}
```

### World.addObj receiver-targeted block (Engine-TS `World.ts:1471-1484`)

```ts
if (receiver64 !== Obj.NO_RECEIVER) {
    obj.setLifeCycle(duration);
    obj.receiver64 = receiver64;
    obj.reveal = Obj.REVEAL;
} else {
    obj.reveal = -1;
    obj.setLifeCycle(duration);
}
```

### Reference constants

- `Obj.REVEAL = 100` (TS `Obj.ts:9`) ↔ goscape `entity.ObjReveal = 100` (`pkg/entity/obj.go:5`).
- `Obj.NO_RECEIVER = -1n` (TS) ↔ goscape `zone.PublicReceiver = -1` (`pkg/zone/zone.go`).

---

## §2. Existing goscape state (HEAD `0650edd`)

### Already in place

- `(*Server).turnLoc(l *entitypkg.Loc, now int)` at `modules/world/loc_turn.go:15-33` — sibling pattern to mirror.
- `(*Server).AddObj(obj *entitypkg.Obj, receiverID int)` at `modules/world/world_zone.go:128-132`.
- `(*Server).RemoveObj(obj *entitypkg.Obj)` at `modules/world/world_zone.go:142-146`.
- `(*Server).RevealObj(obj *entitypkg.Obj, receiverSlot int)` at `modules/world/world_zone.go:149-153`.
- `(*NonPathing).SetLifeCycle(duration, currentTick, tracker)` at `pkg/entity/nonpathing.go:41-59` — `duration > 0` registers tracker + sets `LifecycleTick = currentTick + duration`; `duration <= 0` unregisters + sets `LifecycleTick = -1`.
- `s.locObjTracker` — the lifecycle registry used by `processZones` (covers both Locs and Objs by interface).
- `entity.ObjReveal = 100` constant at `pkg/entity/obj.go:5`.
- `Obj.Reveal int` field at `pkg/entity/obj.go:21` (defaults `-1` in constructor).
- `Server.LookupPlayerByUID(uid int) script.ActivePlayer` at `modules/world/server.go:819`.
- `processZones` iteration at `modules/world/tick.go:611-625`.

### Production sites that pass duration today but discard it

| Site | What flows in | What's missing |
|---|---|---|
| `worldVarsView.AddObj` at `server_varp.go:195-202` | `duration int` param from OBJ_ADD handler | `_ = duration`; never reaches `obj.SetLifeCycle` |
| `processObjDelayedQueue` at `obj_delayed_queue.go:68-69` | `req.duration` | `_ = req.duration` after `s.AddObj` |
| `OBJ_ADD` handler `handlers_obj.go:109, 116` | Passes `duration` from `objType.RespawnRate` (verified: `pkg/objtype/objtype.go:166`) | Plumbed but discarded at sink |

### What does NOT exist (out of scope for NAI-177)

- `RemoveObj` duration parameter — `(*Server).RemoveObj` has no duration; `script.WorldVars.RemoveObj(obj)` interface has no duration. Closing this is the **other half** of NAI-115-D2, deferred to NAI-177+1.
- RESPAWN-after-pickup wiring — depends on the deferred RemoveObj-duration work.

### NAI-115-D2 references (the partial-close target)

`rg "NAI-115-D2" pkg/ modules/` enumeration at HEAD `0650edd` (verified):

| File:Line | Side | Disposition after NAI-177 B0 |
|---|---|---|
| `server_varp.go:129` | RemoveObj doc-comment | unchanged (RemoveObj duration deferred) |
| `server_varp.go:191, 202` | AddObj body | DELETED (duration honored) |
| `server_varp.go:226-229` | EnqueueObjDelayed doc-comment | DELETED (plumbing now end-to-end) |
| `obj_delayed_queue.go:13-17, 21, 69` | drain body + field | DELETED (duration honored at sink) |
| `state.go:98, 136` | script-side WorldVars doc-comments | unchanged (deferred) |
| `handlers_obj.go:140, 229-231` | script-side OBJ_DEL/OBJ_TAKEITEM | unchanged (deferred) |

Plus 2 spec-doc references in `docs/superpowers/specs/` — historical, unchanged.

---

## §3. Bundle map

### B0 — Producer wiring (NAI-115-D2 partial close)

**Modify** `modules/world/world_zone.go`:
- `(*Server).AddObj(obj, receiverID)` → `(*Server).AddObj(obj, receiverID, duration)`. New body sets `obj.SetLifeCycle(duration, s.currentTick, s.locObjTracker)` **before** `z.AddObj(obj, receiverID)` (TS order: World.ts:1467-1484 sets lifecycle around the zone-side call).

**Modify** `modules/world/server_varp.go:195-209`:
- `worldVarsView.AddObj` removes `_ = duration` line; passes `duration` through to `w.s.AddObj`.
- Add receiver-targeted reveal init: `if receiverID != zone.PublicReceiver { obj.Reveal = entity.ObjReveal }` placed **after** `obj.ReceiverID = receiverID` and **before** `w.s.AddObj(...)`.
- Remove the `NAI-115-D2` doc-comment block at lines 191-194.

**Modify** `modules/world/obj_delayed_queue.go`:
- Drop `_ = req.duration` at L69; replace `s.AddObj(req.obj, req.receiverID)` with `s.AddObj(req.obj, req.receiverID, req.duration)`.
- Remove the `DEVIATION-NAI-115-D2 (sibling)` doc-comment block at L13-17 and the `// see DEVIATION-NAI-115-D2 above` comment at L21.

**Modify** `modules/world/server_varp.go:226-229` (EnqueueObjDelayed doc-comment):
- Remove the `NAI-115-D2 sibling: duration is plumbed onto the queue entry but the drain discards it ...` block (4 lines). Replace with no doc-comment (the function's purpose is already explained at L218-224). The plumbing this comment described is now end-to-end honored after B0.

**Update test call sites** for the new signature:
- `modules/world/player_zone_test.go:179, 180, 216, 246` — append `, 0` (no lifecycle — same as today's behavior).
- `modules/world/world_zone_test.go:34, 35` — append `, 0`.

### B1 — Consumer (NAI-86 D-N86-3 close)

**Create** `modules/world/obj_turn.go`:
- `func (s *Server) turnObj(o *entitypkg.Obj, now int)` — body per §4 below.

**Modify** `modules/world/tick.go:617-623`:
- Replace `// TODO(NAI-86 D-N86-3): Obj.Turn ports later.` + `_ = p` with `s.turnObj(p, now)`.

### B2 — Drive-by

**Modify** `pkg/script/handlers_loc.go:262-266`:
- Strip the stale "currently uses DebugName with a stale comment..." and "follow-up (NAI-N+1 — fix LC_NAME ...)" lines. Replace with a single forward-looking note that LC_NAME is TS-correct at `handlers_config.go:159-165` (Name → DebugName → "null").

---

## §4. `(*Server).turnObj` body

```go
// turnObj is the per-tick dispatch for a tracked Obj. Called from
// Server.processZones for each NonPathing in s.locObjTracker whose
// Parent() is a *Obj. Mirrors TS Obj.turn (Engine-TS/.../Obj.ts:27-50).
//
// Two independent arms:
//   1. Reveal countdown — fires every tick the obj is tracked,
//      independent of the lifecycle arm.
//   2. Lifecycle arm — fires only when LifecycleTick == now per
//      DEVIATION-NAI-86-D-N86-4 (absolute tick vs TS decrement;
//      observably identical; see (*Server).turnLoc for the matching
//      label on the Loc side).
func (s *Server) turnObj(o *entitypkg.Obj, now int) {
    // Arm 1: reveal countdown
    if o.Reveal > -1 {
        o.Reveal--
        if o.Reveal == 0 {
            slot := 0
            if p := s.LookupPlayerByUID(o.ReceiverID); p != nil {
                if pp, ok := p.(*Player); ok {
                    slot = pp.slot
                }
            }
            s.RevealObj(o, slot)
        }
    }

    // Arm 2: lifecycle (absolute-tick gate)
    if o.LifecycleTick != now {
        return
    }
    switch {
    case o.Lifecycle == entitypkg.LifecycleDespawn && o.IsActive:
        s.RemoveObj(o)
    case o.Lifecycle == entitypkg.LifecycleRespawn && !o.IsActive:
        s.AddObj(o, zone.PublicReceiver, 0)
    default:
        s.log.Error("obj tracked but no event matched",
            "type", o.Type, "x", o.X, "z", o.Z,
            "lifecycle", o.Lifecycle, "active", o.IsActive)
        o.SetLifeCycle(-1, now, nil)
    }
}
```

Notes:
- **No negative-tick arm.** TS `Obj.ts:45-48` (`else if (this.lifecycleTick < 0)`) is impossible under goscape's absolute-tick model (DEVIATION-NAI-86-D-N86-4 already documented on the Loc side).
- **RESPAWN re-add passes duration=0** (TS L39: `World.addObj(this, Obj.NO_RECEIVER, 0)`). The static-loaded RESPAWN-cycle obj that this branch services has its `LifecycleTick` already set by an earlier `SetLifeCycle` (during initial spawn or post-pickup). Passing `0` here matches TS — caller responsibility is to set the next lifecycle via subsequent `SetLifeCycle` if it should despawn again.
- **`p.slot` is a private field** on `*Player`. Type-assert pattern matches existing code in `modules/world/server_varp.go:136-140`. If the receiver has logged out or never existed, `LookupPlayerByUID` returns `nil` → slot stays `0`, matching TS `?? 0`.

---

## §5. Test surface

Mirror `loc_turn_test.go` structure. All new tests in `modules/world/obj_turn_test.go` unless noted.

### B0 tests (producer wiring)

`modules/world/world_zone_obj_lifecycle_test.go` (new):
- **`TestServerAddObj_DurationSetsLifecycleTick`** — call `s.AddObj(o, PublicReceiver, 50)`; assert `o.LifecycleTick == s.currentTick + 50`; assert tracker contains `o`.
- **`TestServerAddObj_ZeroDurationLeavesLifecycleTickNegOne`** — call `s.AddObj(o, PublicReceiver, 0)`; assert `o.LifecycleTick == -1`.

`modules/world/server_varp_test.go` (extend) or new test file:
- **`TestWorldVarsViewAddObj_ReceiverTargetedSetsReveal100`** — call `w.AddObj(level, x, z, type, count, duration, 12345)` with non-public receiver; assert the returned `*entitypkg.Obj` has `Reveal == entity.ObjReveal` (=100).
- **`TestWorldVarsViewAddObj_PublicReceiverLeavesRevealNegOne`** — call with `receiverID == zone.PublicReceiver`; assert `Reveal == -1`.

`modules/world/obj_delayed_queue_test.go` (modify):
- Update the existing `TestObjDelayedQueue_DurationStoredAtEnqueueDroppedAtDrain` (`obj_delayed_queue_test.go:100-114`) — rename to `TestObjDelayedQueue_DurationDrainsToServerAddObj` and assert that after drain, `req.obj.LifecycleTick == s.currentTick + duration` (was: "no observable side-effect from duration today").

### B1 tests (consumer)

1. **`TestTurnObj_RevealCountdownDecrementsAcrossTicks`** — `o.Reveal=2`, call `turnObj`; assert `Reveal==1`. Tick again; assert `Reveal==0` + `RevealObj` emitted at zone-side + `ReceiverID == PublicReceiver`.
2. **`TestTurnObj_RevealAtZero_UsesReceiverPlayerSlot`** — add a `*Player` with UID=42, slot=3, to `s.playerLoop`; set `o.ReceiverID=42`, `o.Reveal=1`; call; assert the zone-side OBJ_REVEAL event encodes slot=3.
3. **`TestTurnObj_RevealAtZero_LoggedOutReceiverPassesSlotZero`** — set `o.ReceiverID=99999` (no matching player); set `o.Reveal=1`; call; assert encoded slot=0.
4. **`TestTurnObj_RevealNegOneIsNoOp`** — `o.Reveal=-1`; call; assert `Reveal` still `-1` + no zone event.
5. **`TestTurnObj_DespawnAtScheduledTick_FiresRemove`** — `o.Lifecycle=DESPAWN`, `IsActive=true`, `LifecycleTick=now`; call `turnObj(o, now)`; assert `RemoveObj` invoked (obj inactive after).
6. **`TestTurnObj_RespawnAtScheduledTick_FiresAdd`** — `o.Lifecycle=RESPAWN`, `IsActive=false`, `LifecycleTick=now`; call; assert `AddObj` invoked (obj active after).
7. **`TestTurnObj_BeforeScheduledTickIsLifecycleNoOp`** — `LifecycleTick=now+1`; call `turnObj(o, now)`; assert obj state unchanged; reveal arm independently exercised.
8. **`TestTurnObj_NoMatchingLifecycle_UntracksAndLogs`** — DESPAWN+`!IsActive` at scheduled tick (or RESPAWN+active); call; assert `LifecycleTick==-1` + tracker no longer has obj. (Log capture optional.)
9. **`TestTurnObj_RevealAndLifecycleIndependent`** — set `Reveal=1` and `LifecycleTick=now+5`; call; assert `Reveal` fired (RevealObj emitted) AND lifecycle did NOT fire.
10. **`TestProcessZones_DispatchesObjToTurnObj`** — integration: place a tracked `*entitypkg.Obj` in `s.locObjTracker`; call `processZones`; assert the obj's state advanced (e.g., `Reveal` decremented).

### B2 tests

No tests for B2 (comment-only change).

---

## §6. Deviations

### Existing deviation labels reused (no new tags)

- **NAI-86-D-N86-4** — absolute `LifecycleTick` vs TS decrement. Already documented in `turnLoc`. Reuse the label inline in `turnObj`'s doc-comment header.

### New deviations

**None** — all TS arms map 1:1.

### Retired tags after close

- `NAI-86-D-N86-3` — placeholder branch in `processZones` (replaced by real dispatch in B1).
- `NAI-115-D2` (AddObj half only) — `Server.AddObj` and `worldVarsView.AddObj` and `processObjDelayedQueue` now honor duration. `RemoveObj` duration extension stays deferred.

---

## §7. Implementation tasks

### T1 — B0 producer wiring (RED → GREEN)

- [ ] T1.1 — Write failing producer tests (`TestServerAddObj_*`, `TestWorldVarsViewAddObj_*`) per §5.
- [ ] T1.2 — Modify `(*Server).AddObj` signature to add `duration int`; call `obj.SetLifeCycle(duration, s.currentTick, s.locObjTracker)` before `z.AddObj`.
- [ ] T1.3 — Update `worldVarsView.AddObj` to plumb `duration` through and to set `obj.Reveal = entity.ObjReveal` for non-public receivers. Strip NAI-115-D2 doc-comment.
- [ ] T1.4 — Update `processObjDelayedQueue` drain to pass `req.duration`. Strip NAI-115-D2 sibling doc-comments in BOTH `obj_delayed_queue.go` (drain) AND `server_varp.go:226-229` (EnqueueObjDelayed).
- [ ] T1.5 — Update test call sites in `player_zone_test.go`, `world_zone_test.go` to pass `, 0` for unchanged-behavior tests.
- [ ] T1.6 — Update `TestObjDelayedQueue_DurationStoredAtEnqueueDroppedAtDrain` per §5 to assert `LifecycleTick` post-drain.

**Verify:** `go test ./modules/world/... ./pkg/zone/...` green. `rg "NAI-115-D2" modules/world/` returns only the script-side hits (state.go, handlers_obj.go).

### T2 — B1 turnObj RED

- [ ] T2.1 — Write `obj_turn_test.go` with all 10 cases from §5 B1. **All RED** until T3 lands the function.

### T3 — B1 turnObj GREEN

- [ ] T3.1 — Create `modules/world/obj_turn.go` with body per §4.
- [ ] T3.2 — Verify all B1 tests pass.

### T4 — B1 wire dispatch

- [ ] T4.1 — `modules/world/tick.go:617-623`: replace `_ = p` with `s.turnObj(p, now)`. Strip TODO(NAI-86 D-N86-3) comment.
- [ ] T4.2 — Verify `TestProcessZones_DispatchesObjToTurnObj` passes.

### T5 — B2 LC_NAME drive-by

- [ ] T5.1 — Update `pkg/script/handlers_loc.go:262-266` per §3 B2.
- [ ] T5.2 — Verify `go vet ./...` clean. (No test impact — comment-only.)

### T6 — Code review pass

- [ ] T6.1 — Combined end-of-impl reviewer pass (Sonnet, per `superpowers_code_reviewer_model`).

### Close — NAI-177

- [ ] CLOSE.1 — Update `nai_followups.md` with NAI-177 close section.
- [ ] CLOSE.2 — Close commit with `Closes memory:` trailer: retire NAI-86-D-N86-3, partial-close NAI-115-D2.

---

## §8. Plan-author pre-flight reminders

Per `controller_preflight.md`, the controller should re-verify these at HEAD before each task dispatch:

1. **Sig change ripple** (T1.2): `rg "\.AddObj\(" modules/world/ pkg/` — enumerate ALL test + production call sites; the §3 enumeration is a snapshot. Mid-TDD additions may exist.
2. **Tracker registration semantics** (T1.2): `SetLifeCycle(0, ...)` calls `Unregister` (`nonpathing.go:42-44`) — so passing duration=0 from existing callers leaves obj untracked, matching pre-NAI-177 behavior. Verify the `Unregister` path is safe to call when the obj was never registered (idempotent — first conditional checks `np.tracker != nil`).
3. **`Player.slot` field** (T3.1): private field at `modules/world/player.go:79`. Type-assert pattern at `server_varp.go:136-140` is the precedent. Avoid promoting `slot` to the `script.ActivePlayer` interface — keep the cross-package adapter pattern.
4. **Existing reveal-arm tests** (T2.1): `rg "Reveal" modules/world/*_test.go pkg/zone/*_test.go` — any pre-existing test that depended on `Reveal == -1` after `AddObj` for a receiver-targeted drop will need updating in B0.
5. **`obj.IsActive` after AddObj** (T2.6): the zone-side `Zone.AddObj` sets `IsActive=true` for DESPAWN-lifecycle but RESPAWN-lifecycle objs have `IsActive=false` by default. Verify the B1 test #6 sets the RESPAWN preconditions explicitly (don't rely on prior AddObj).
6. **`s.log` nil in fixtures** (T3.1): `s.log.Error(...)` in the fallthrough arm — verify test fixtures use a non-nil logger (the existing `loc_turn_test.go` newLocTurnTestServer pattern handles this).
7. **`zone.PublicReceiver` is `int` not bigint** (T3.1): goscape uses `int` (-1) while TS uses `bigint` (-1n). Confirm `s.AddObj(o, zone.PublicReceiver, 0)` typechecks.

---

## §9. Self-review checklist

Run after writing this doc (per brainstorming skill §Spec Self-Review):

### Placeholder scan

- [x] No "TBD"/"TODO"/"XXX" placeholders in body text. (The `_ = p` line being REPLACED at tick.go:621 is the only TODO; that's a code state to be fixed, not a spec placeholder.)
- [x] All file paths exist at HEAD.
- [x] All line numbers verified at HEAD `0650edd`.

### Internal consistency

- [x] §1 TS source matches §4 Go translation 1:1.
- [x] B0 producer changes and B1 consumer arms are independently testable (verified via T1.1 + T2.1 RED-first ordering).
- [x] §3 file map matches §7 task list.

### Scope check

- [x] Compressed cadence (~285 LOC) appropriate.
- [x] NAI-115-D2 split point is clean — AddObj half closes; RemoveObj half stays open with retained doc-comments at `state.go:98, 136` and `handlers_obj.go:140, 229-231`.
- [x] Drive-by (LC_NAME) is comment-only — no scope creep.

### Ambiguity check

- [x] §4 spells out the `*Player` type-assert pattern explicitly so the implementer doesn't try to promote `slot` to the interface.
- [x] §4 calls out duration=0 in the RESPAWN re-add arm with TS reference.
- [x] §6 explicitly notes "no new deviations" — saves the implementer a hunt.

---

## §10. Smoke binding (post-close, optional)

User-launched server + Java client (per `smoke_test_server_handoff`):

- **Drop visibility:** Two clients log in. Client A drops a tradeable item near Client B. Client B does NOT see the item for ~60 seconds (100 ticks). At tick 100, Client B sees the obj appear.
- **Bones decay:** Single client drops bones (or any DESPAWN-lifecycle drop). Confirm bones disappear at the scheduled tick (~60s for bones in TS content; verify objType.RespawnRate plumbing.)
- **Non-binding** per `cascade_theory_smoke_binding.md` — if no content path reaches the receiver-targeted reveal flow during smoke, dispatch-correctness via unit tests is sufficient. Smoke is a sanity confirmation, not a contract pin.
