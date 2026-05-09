# NAI-134 — INV_DROPITEM_DELAYED + objDelayedQueue infra

**Date:** 2026-05-09
**Branch base:** `main` @ `b386dec`
**Predecessor:** NAI-133 (BOTH_MOVEINV + Pointer-flag Protect refactor + FINDUID/P_FINDUID slot routing)
**Tracker source:** `~/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md` line 6416 — "**NAI-134+ candidate (NAI-132 deferred):** `INV_DROPITEM_DELAYED` opcode 4310."

## 1. Goal

Port `INV_DROPITEM_DELAYED` (script opcode 4310) plus the supporting `World.objDelayedQueue` tick-driven drop-spawn queue from TS Engine-TS to goscape, with full TS fidelity. Leaves the parallel `NAI-115-D2` foundation gap (duration-despawn scheduler) unaddressed — `duration` is plumbed through to `Server.AddObj` and ignored there as today.

## 2. TS source — anchored

- `Engine-TS/src/engine/script/handlers/InvOps.ts:188-209` — handler.
- `Engine-TS/src/engine/entity/ObjDelayedRequest.ts:1-22` — request type (extends `Linkable`; goscape uses a slice).
- `Engine-TS/src/engine/World.ts:157` — queue declaration.
- `Engine-TS/src/engine/World.ts:563-573` — drain loop, post-decrement.

Goscape divergences from TS surface (intentional, justified):

| Tag | Locus | Divergence | Rationale |
|---|---|---|---|
| **DEVIATION-NAI-134-D1** | `obj_delayed_queue.go` storage | Slice instead of TS `LinkList`. | Mirrors goscape's existing `worldScriptQueue` (`world_script_queue.go`); behavior identical. |
| **DEVIATION-NAI-115-D2 (sibling reuse)** | drain `AddObj(...)` | `duration` arg accepted but not honored (no despawn-after-N scheduler). | Carried verbatim from `worldVarsView.AddObj` at `server_varp.go:171`. NAI-134 is delay-honoring only. |
| **DEVIATION-NAI-130-D2 (sibling reuse)** | handler | Defensive nil-World guard returns clean error. | TS lacks; goscape mirrors `handleInvDropItem`. |

## 3. Non-goals

- Closing NAI-115-D2 (despawn scheduler).
- Closing NAI-86 D-N86-3 (Obj.Turn for tracked-obj per-tick state).
- Adding any other `INV_*` handler.
- Smoke harness work — `INV_DROPITEM_DELAYED` is not exercised by any current goscape smoke target (Tutorial Island is melee-only).

## 4. Architecture

```
pkg/script (script-engine layer)
  ├── ScriptState.WorldVars adds method:
  │       EnqueueObjDelayed(level,x,z,typeID,count,duration,delay,receiverID int)
  └── handleInvDropItemDelayed(s) — InvOps.ts:188-209 port
         calls s.World.EnqueueObjDelayed(...)

modules/world (server-state-aware impl)
  ├── obj_delayed_queue.go (NEW)
  │     type objDelayedRequest struct { obj *entitypkg.Obj; receiverID, duration, delay int }
  │     Server.objDelayedQueue []objDelayedRequest
  │     Server.enqueueObjDelayed(...)        // package-internal, called by worldVarsView
  │     Server.processObjDelayedQueue()      // drain step
  ├── tick_recovery.go +recoverObjDelayed(req, *slog.Logger)
  │                    // panic recovery, mirrors recoverWorldScript
  │
  ├── server.go +objDelayedQueue field on Server (next to worldScriptQueue)
  ├── server_varp.go +worldVarsView.EnqueueObjDelayed(...) — constructs Obj, appends entry,
  │                  optional NodeDebug log gateway "nai134.obj.delayed.enqueue"
  └── tick.go +processObjDelayedQueue() between processActiveScripts and processNpcs
```

No new interfaces. No new traits. No new sub-packages.

## 5. Data flow

### 5.1 Enqueue path (handler)

```
runescript: inv_dropitem_delayed inv,coord,obj,count,duration,delay
  → opcode 4310 dispatch via pkg/script/handlers.go
  → handleInvDropItemDelayed(s):
      1. requireActivePlayer (TS checkedHandler ActivePlayer)
      2. PopInt ×6 reverse:
           delay     := s.PopInt()
           duration  := s.PopInt()
           count     := s.PopInt()
           obj       := s.PopInt()
           coord     := s.PopInt()
           invID     := s.PopInt()
      3. validators (mirror handleInvDropItem):
           checkInvType, checkCoord, checkObjType, checkObjStack, checkDuration
           (NO DelayValid check — TS InvOps.ts:188-195 omits it; we mirror)
      4. operand-aware protect gate (NAI-133 slot routing):
           operand := s.Script.IntOperands[s.PC]
           operand ∈ {0,1} else "invalid intOperand"
           protectFlag := PtrProtectedActivePlayer  if operand==0
                          PtrProtectedActivePlayer2 if operand==1
           if invType.Protect && Scope!=Shared && Pointers&protectFlag == 0:
               err "$inv requires protected access: <debugname>"
      5. inv := resolveInv(s, invID) → nil ⇒ err
      6. tx := inv.Remove(obj, count, BeginSlot:-1)
         completed := tx.Completed
         if completed == 0 → return nil  (TS InvOps.ts:203-205)
      7. defensive nil-World guard (DEVIATION-NAI-130-D2 sibling)
      8. s.World.EnqueueObjDelayed(level, x, z, obj, completed, duration, delay, s.Self.UID())
      9. NO ActiveObj writeback / NO PtrActiveObj — TS InvOps.ts:206-208 deliberately
         omits; the obj does not exist yet at enqueue time.
```

### 5.2 Drain path (tick)

```
modules/world tick.go runTickLoop iteration order (NEW step marked):
  processClientsIn
  processWorldQueue
  processNpcEventQueue
  processActiveScripts
  processObjDelayedQueue       ← NEW — TS World.ts:563
  processPlayerTimers
  processPathing
  processInteractions
  processWalkTriggerFallbacks
  processNpcs
  ...
```

```go
// processObjDelayedQueue body (mirrors worldScriptQueue idiom):
func (s *Server) processObjDelayedQueue() {
    i := 0
    for i < len(s.objDelayedQueue) {
        e := &s.objDelayedQueue[i]
        // POST-decrement: capture, then decrement. Mirrors TS World.ts:564
        // (`const delay = request.delay--;`).
        delay := e.delay
        e.delay--
        if delay > 0 {
            i++
            continue
        }
        req := *e
        s.objDelayedQueue = append(s.objDelayedQueue[:i], s.objDelayedQueue[i+1:]...)
        func(req objDelayedRequest) {
            defer recoverObjDelayed(req, s.log)
            s.AddObj(req.obj, req.receiverID)
            _ = req.duration // NAI-115-D2 sibling — duration not honored here
        }(req)
        // Don't advance i — slice contracted under us (mirrors processWorldQueue).
    }
}
```

### 5.3 Tick semantics (post-decrement)

Captured-then-decremented, identical to TS:

| User `delay` | Drain calls until fire | Notes |
|---|---|---|
| `0` | 1st drain after enqueue | captured 0; 0>0 false; fires. Same-tick if enqueue runs in `processActiveScripts`. |
| `1` | 2nd drain | captured 1 (skip); next drain captured 0 (fire). |
| `N` | N+1-th drain | TS-faithful per `World.ts:564`. |

No `+1` enqueue offset (unlike `EnqueueWorldScript`) — TS `objDelayedQueue.addTail` does not add one (`InvOps.ts:208` stores `delay` verbatim).

## 6. Component contracts

### 6.1 `obj_delayed_queue.go`

```go
package world

import (
    entitypkg "github.com/zsrv/goscape/pkg/entity"
)

// objDelayedRequest is one INV_DROPITEM_DELAYED request awaiting drain.
// Mirrors TS ObjDelayedRequest (Engine-TS/.../entity/ObjDelayedRequest.ts).
//
// DEVIATION-NAI-134-D1: TS uses LinkList<ObjDelayedRequest> (Linkable mixin).
// Goscape uses a slice on Server, mirroring worldScriptQueue. Behavior identical.
type objDelayedRequest struct {
    obj        *entitypkg.Obj
    receiverID int
    duration   int // forwarded to Server.AddObj at drain; honored only when NAI-115-D2 closes
    delay      int // ticks remaining; post-decrement per TS World.ts:564
}

func (s *Server) enqueueObjDelayed(obj *entitypkg.Obj, receiverID, duration, delay int) {
    s.objDelayedQueue = append(s.objDelayedQueue, objDelayedRequest{
        obj: obj, receiverID: receiverID, duration: duration, delay: delay,
    })
}

func (s *Server) processObjDelayedQueue() { /* per §5.2 */ }

// (lives in tick_recovery.go — same file as recoverWorldScript)
func recoverObjDelayed(req objDelayedRequest, log *slog.Logger) {
    r := recover()
    if r == nil {
        return
    }
    typeID := -1
    if req.obj != nil {
        typeID = req.obj.Type
    }
    log.Error("panic in objDelayedQueue fire",
        "typeID", typeID,
        "receiverID", req.receiverID,
        "duration", req.duration,
        "err", r,
        "stack", string(debug.Stack()))
}
```

### 6.2 `server.go` field

```go
type Server struct {
    // ...
    worldScriptQueue   []worldScriptQueueEntry
    objDelayedQueue    []objDelayedRequest // NAI-134 — adjacent to worldScriptQueue
    // ...
}
```

### 6.3 `server_varp.go` worldVarsView surface

```go
// EnqueueObjDelayed implements script.WorldVars.EnqueueObjDelayed.
// Constructs a DESPAWN-lifecycle Obj at (level,x,z) with typeID/count, sets
// ReceiverID, and appends an objDelayedRequest. Mirrors TS InvOps.ts:207-208
// where `new Obj(...)` is constructed at enqueue time (not drain time).
//
// NAI-115-D2 sibling: duration is plumbed through to the eventual AddObj
// at drain; AddObj currently ignores it (no despawn-after-N scheduler).
func (w worldVarsView) EnqueueObjDelayed(level, x, z, typeID, count, duration, delay, receiverID int) {
    if w.s == nil {
        return
    }
    obj := entitypkg.NewObj(level, x, z, entitypkg.LifecycleDespawn, typeID, count)
    obj.ReceiverID = receiverID
    w.s.enqueueObjDelayed(obj, receiverID, duration, delay)
    if w.s.cfg.NodeDebug && w.s.log != nil {
        w.s.log.Info("nai134.obj.delayed.enqueue",
            "level", level, "x", x, "z", z,
            "typeID", typeID, "count", count,
            "duration", duration, "delay", delay,
            "receiverID", receiverID,
        )
    }
}
```

### 6.4 `tick.go` integration

Add one line after `processActiveScripts()` and before `processPlayerTimers()`:

```go
s.processActiveScripts()
s.processObjDelayedQueue() // NAI-134 — TS World.ts:563 ordering
s.processPlayerTimers()
```

### 6.5 `pkg/script/state.go` WorldVars surface

Add to the existing `WorldVars` interface (same block as `AddObj`):

```go
type WorldVars interface {
    // ... existing ...
    AddObj(level, x, z, typeID, count, duration, receiverID int) ActiveObj
    // NAI-134: enqueue a delayed obj-spawn request. The Obj is constructed
    // at the impl side; this signature mirrors AddObj plus a `delay` param.
    EnqueueObjDelayed(level, x, z, typeID, count, duration, delay, receiverID int)
    // ... existing ...
}
```

### 6.6 `pkg/script/handlers_inv.go` — handleInvDropItemDelayed

Per §5.1 (full body codified in plan).

### 6.7 `pkg/script/handlers.go` dispatch wire

```go
OpInvDropItem:        handleInvDropItem,
OpInvDropItemDelayed: handleInvDropItemDelayed, // NAI-134
OpInvDropSlot:        handleInvDropSlot,
```

## 7. Error handling

- **Validators**: every check returns a typed error string prefixed `INV_DROPITEM_DELAYED:`. Pattern matches `handleInvDropItem`.
- **Bad operand**: `operand ∉ {0,1}` → `"INV_DROPITEM_DELAYED: invalid intOperand %d"`. Mirrors `handleBothMoveInv`.
- **nil World**: defensive guard returns clean error rather than nil-deref. DEVIATION-NAI-130-D2 sibling.
- **Drain panic**: `recoverObjDelayed` swallows + logs; tick continues. Mirrors `recoverWorldScript`.
- **completed == 0**: handler returns `nil` (TS InvOps.ts:203-205) — silent no-op when `inv.Remove` finds nothing.

## 8. Testing strategy

### 8.1 `modules/world/obj_delayed_queue_test.go` (NEW)

| Test | Pins |
|---|---|
| `TestObjDelayedQueue_FiresAfterDelayTicks` | `delay=2` → drain calls 1,2 skip; call 3 fires. Captures post-decrement semantics. |
| `TestObjDelayedQueue_DelayZeroFiresImmediately` | `delay=0` → fires on first drain call. |
| `TestObjDelayedQueue_DrainCallsServerAddObj` | After fire: zoneMap-resolved zone has the obj, receiverID propagated. |
| `TestObjDelayedQueue_DurationStoredAtEnqueueDroppedAtDrain` | Queue entry's `duration` field set verbatim at enqueue; drain runs without observable side-effect from duration (NAI-115-D2 parity — `Server.AddObj` does not yet accept duration). |
| `TestObjDelayedQueue_MultipleEntriesIndependentDelays` | Enqueue {0,1,2}; fire in {0, then 1, then 2} drain order. |
| `TestObjDelayedQueue_RemoveBeforeFire_PanicRecovery` | Force AddObj panic → recoverObjDelayed swallows; queue length post = pre - 1. |

Fixtures: real `Server` from `newTestServer(t)` (per `test_fixture_view_parity` memory) — needs zonesTracking, worldVars, configsView, invLookup, npcLookup all initialized. Direct `s.processObjDelayedQueue()` calls (no full tick loop) — matches `world_script_queue_test.go` pattern.

### 8.2 `pkg/script/handlers_inv_test.go` (extended)

| Test | Pins |
|---|---|
| `TestInvDropItemDelayed_NoActivePlayer_Errors` | Missing PtrActivePlayer → "no active player". |
| `TestInvDropItemDelayed_BadInv_Errors` | Invalid invID → "invalid inv id". |
| `TestInvDropItemDelayed_BadCoord_Errors` | coord=-1 → "coord out of range". |
| `TestInvDropItemDelayed_BadObj_Errors` | obj=-1 → "invalid obj type". |
| `TestInvDropItemDelayed_BadCount_Errors` | count=0 → "invalid count". |
| `TestInvDropItemDelayed_BadDuration_Errors` | duration=0 / >5000 → "duration out of range". |
| `TestInvDropItemDelayed_BadOperand_Errors` | operand=2 → "invalid intOperand". |
| `TestInvDropItemDelayed_ProtectGate_Operand0_Errors` | Protect + no PtrProtectedActivePlayer → error citing debugname. |
| `TestInvDropItemDelayed_ProtectGate_Operand0_PassesWithFlag` | Protect + PtrProtectedActivePlayer set → success. |
| `TestInvDropItemDelayed_ProtectGate_Operand1_UsesPtrProtectedActivePlayer2` | Protect + operand=1 + only PtrProtectedActivePlayer set (not …2) → error. With PtrProtectedActivePlayer2 set → success. NAI-133 slot-routing parity. |
| `TestInvDropItemDelayed_SharedScopeBypassesProtect` | Protect + Scope=Shared → no protect-gate error. |
| `TestInvDropItemDelayed_RemoveCompletedZero_NoEnqueue` | Empty inv → handler returns nil; mock recorder shows zero EnqueueObjDelayed calls. |
| `TestInvDropItemDelayed_HappyPath_EnqueueArgs` | All-good. Recorder pins (level,x,z,typeID,count,duration,delay,receiverID) match expected. |
| `TestInvDropItemDelayed_DoesNotSetActiveObj` | After success: s.ActiveObj == nil and Pointers&PtrActiveObj == 0. TS-asymmetry vs INV_DROPITEM. |
| `TestInvDropItemDelayed_NilWorld_DefensiveError` | s.World = nil → DEVIATION-NAI-130-D2 sibling clean error. |

Fixture extensions:
- `mockWorld` in `handlers_vars_test.go` — add `enqueueObjDelayedCalls []enqueueObjDelayedCall` recorder + `EnqueueObjDelayed(...)` method.
- `fakeWorldAddObj` (extends mockWorld) inherits the new method automatically.
- Test helpers in `handlers_inv_test.go` (`runInvOpExpectErr`, `runInvOpExpectErrAsPlayer`) reused unchanged.

### 8.3 Smoke

No smoke target — content does not exercise opcode 4310 in any current goscape smoke. Test coverage is unit-only.

Per `cascade_theory_smoke_binding`: NAI-134 close criterion is **green tests + verified no smoke regression** (re-run firemaking smoke if any tick-ordering change is suspected; the new step is additive between existing phases and does not reorder existing work).

## 9. Risk register

| Risk | Mitigation |
|---|---|
| Tick-ordering shift breaks adjacent processors | New step is purely additive between `processActiveScripts` and `processPlayerTimers`. No existing call moves. Verify with NAI-122 NPC-event ordering tests pass. |
| WorldVars interface widening breaks test mocks | All test mocks of WorldVars in `pkg/script/...` and `modules/world/...` enumerated by `grep -rn "WorldVars\|AddObj.*receiverID" --type go`. Plan §3 covers per-mock additions of `EnqueueObjDelayed` no-op or recorder. |
| Same-tick fire of `delay=0` violates TS | Verified: TS handler runs in script-firing phase, drain runs at line 563 of same `World.cycle`. Goscape mirrors this exactly: handler in `processActiveScripts`, drain immediately after. |
| Operand=1 path untested in production | `INV_DROPITEM_DELAYED` content invocations always use operand=0 (single-player). Operand=1 path is defensively covered for parity with NAI-133 slot routing; no real content exercises it. |
| Duration silently dropped | DEVIATION-NAI-115-D2 sibling — explicit `_ = req.duration` in drain + doc comment. NAI-115-D2 closure will pick this up at one site (drain). |

## 10. Open questions

None.

## 11. Acceptance

- All new tests green.
- `go test ./...` passes.
- `go test -race ./...` passes.
- `go build ./...` clean.
- No regression in NAI-122 / NAI-127 / NAI-130 / NAI-131 / NAI-132 / NAI-133 tick-order or queue-related tests.
- Doc-comment cross-references to `InvOps.ts:188-209` and `World.ts:563-573` present in both new files (`obj_delayed_queue.go`, `handleInvDropItemDelayed`).
- DEVIATION tags present at all three documented sites.

## 12. Carry-forward routing

- **NAI-115-D2** remains open. NAI-134 stores `duration` on the queue entry but discards it at drain (because `Server.AddObj(obj, receiverID)` does not yet accept duration; the discard mirrors `worldVarsView.AddObj`'s existing `_ = duration`). Single-point retire when NAI-115-D2 closes — at that point both `Server.AddObj` and `processObjDelayedQueue`'s drain stop discarding.
- **NAI-86 D-N86-3** unchanged (Obj.Turn deferred).
- **NAI-135+ candidate**: enumerate at NAI-134 close from `nai_followups.md` and from any spec-deferred tags surfaced by review.
