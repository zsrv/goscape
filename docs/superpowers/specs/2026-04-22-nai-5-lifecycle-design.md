# NAI-5 — Full Lifecycle State Machine + NpcEventQueue + `ai_despawn`

Replace the current simplified `dead`-bool respawn path in `Npc.turn()`
with a full TS-faithful Events block covering RESPAWN (respawn +
revertType) and DESPAWN (remove + queue `ai_despawn` trigger) paths.
Add `NpcEventQueue` on `*Server` with a `processNpcEventQueue` tick
phase run BEFORE `processNpcs`. Add `baseType` + `revertType` for
proper morph-revert semantics.

Part of the NPC AI tick decomposition roadmap. Blocker: NAI-2.
Roadmap fidelity risk: **HIGH**.

## Goal

After NAI-5:

1. NPCs with `lifecycle == RESPAWN` respawn via the Events block
   when `lifecycleTick` hits 0 post-decrement, faithfully matching TS
   `Npc.ts:121-151` including the alive/revertType vs dead/respawn
   branches.
2. NPCs with `lifecycle == DESPAWN` get marked dead and an
   `ai_despawn` trigger script is enqueued via `NpcEventQueue` for
   dispatch in the next tick phase.
3. `revertType()` on `*Npc` restores `typ` to the baseline saved at
   construction, resets stats, clears queue, clears waypoints, raises
   `tele` + `NpcMaskChangeType`. Hunt-field resets deferred to NAI-7.
4. `processNpcEventQueue` runs before `processNpcs` each tick,
   dispatching queued scripts on non-delayed NPCs.
5. The `isValid` gate (`!dead && !delayed`) skips timer/queue/movement
   for NPCs that are dead or delayed — matches TS `Npc.ts:154`.

## Scope — what's IN

- `NpcEventType` + `NpcEventRequest` types (new file
  `modules/world/npc_event_queue.go`)
- `*Server.npcEventQueue []NpcEventRequest` field + `processNpcEventQueue`
  method
- Tick-driver phase: call `processNpcEventQueue` before `processNpcs`
- `*Npc.baseType int` field seeded in `NewNpc`
- `*Npc.revertType()` method (stats + queue + waypoints + tele; hunt
  fields deferred to NAI-7)
- `*Server.removeNpc(n)` wired (currently unused helper at
  `modules/world/npc_registry.go:42`) — sets `n.dead = true`. Does
  NOT touch `s.npcs[]`/`s.npcLoop` (see non-goals)
- `Npc.turn()` restructure: new Events block between script-lifecycle
  prefix and timer/queue passes, plus new `isValid` gate
- Removal of the current simplified lifecycle code (the trailing
  `if n.dead { lifecycleTick--; ... } return` block in `npc_ai.go`)

## Scope — explicit non-goals

1. **True registry manipulation.** TS `World.addNpc`/`removeNpc`
   add/remove from `npcs[]` + `npcLoop` + grid. Go keeps the
   "`dead` bool is the logical-presence flag" approach: `removeNpc`
   only sets `n.dead = true`; respawn flips it back. True registry
   manipulation is deferred to a future script-driven-NPC-creation
   sub-spec (when `OpNpcAdd` / `OpNpcDel` need it). Rationale: full
   registry manipulation breaks existing `TestKillSetsDeadAndLifecycleTick`
   / `TestRespawnAfterKill` + requires grid-update choreography out
   of scope for a single sub-spec.
2. **`NpcEventSpawn` producer.** Enum value reserved for TS fidelity
   (TS `NpcEventType.SPAWN = 0`), but NAI-5 has no producer — only
   DESPAWN events. YAGNI; NAI-5 would ship a dead-API half of the
   enum, so the SPAWN path lands with its first consumer.
3. **Hunt-field resets in `revertType`.** TS `Npc.ts:309-312` resets
   `huntrange`, `huntMode`, `huntClock`, `huntTarget`. NAI-7 adds
   those fields AND the reset lines in one go.
4. **`npc_changetype` auto-revert timing.** `OpNpcChangeType` exists
   since S6c; verify (via manual inspection during plan write) that
   it correctly sets `lifecycleTick = duration` so the new Events
   block picks up the revert. If it doesn't, file a separate minor
   fix; don't scope-creep NAI-5.
5. **NumberNotNull audit.** Tracked in `nai_followups.md`.

## TS reference

- `Engine-TS/src/engine/entity/EntityLifeCycle.ts` (3-variant enum —
  Go already has matching constants at `modules/world/npc.go:8-12`)
- `Engine-TS/src/engine/entity/NpcEventRequest.ts` (struct shape)
- `Engine-TS/src/engine/entity/Npc.ts:121-151` (Events block)
- `Engine-TS/src/engine/entity/Npc.ts:154` (isValid gate)
- `Engine-TS/src/engine/entity/Npc.ts:280-317` (resetEntity aka
  revertType — note: Go scope drops hunt lines to NAI-7)
- `Engine-TS/src/engine/World.ts:156` (npcEventQueue field)
- `Engine-TS/src/engine/World.ts:356` (tick-driver call site — BEFORE
  processNpcs)
- `Engine-TS/src/engine/World.ts:664-673` (processNpcEventQueue body)

## Architecture

### File layout

**New:**
- `modules/world/npc_event_queue.go` — `NpcEventType`, `NpcEventRequest`
  types + `(s *Server) processNpcEventQueue()` method

**Modified:**
- `modules/world/npc.go` — `+baseType` field, `+revertType` method,
  NewNpc seeds baseType
- `modules/world/npc_ai.go` — Events block + isValid gate in turn();
  remove simplified dead-block trailer
- `modules/world/npc_registry.go` — activate `removeNpc` (still doesn't
  touch `npcs[]`/`npcLoop` but now sets `n.dead = true` and has a
  consumer so linter stops flagging)
- `modules/world/tick.go` — `+s.processNpcEventQueue()` call before
  `s.processNpcs()`
- `modules/world/server.go` — `+npcEventQueue` field adjacent to
  `npcLoop`
- `modules/world/npc_ai_test.go` — 1 modification (existing
  `TestRespawnAfterKill` still passes; NewNpc-baseType assertion is
  NEW test in `npc_event_queue_test.go`)
- `modules/world/npc_event_queue_test.go` (NEW) — 8 tests

### Types (`modules/world/npc_event_queue.go`)

```go
package world

import "github.com/zsrv/goscape/pkg/script"

// NpcEventType mirrors TS NpcEventType at
// Engine-TS/src/engine/entity/NpcEventRequest.ts.
type NpcEventType int

const (
	NpcEventSpawn   NpcEventType = 0 // reserved; no producer in NAI-5
	NpcEventDespawn NpcEventType = 1
)

// NpcEventRequest is a queued world-level NPC event. The Events
// block in Npc.turn() enqueues one of these when an NPC's
// lifecycleTick hits zero on a DESPAWN-lifecycle NPC; the next
// processNpcEventQueue pass dispatches the script if the NPC is
// not delayed. Matches TS NpcEventRequest.
type NpcEventRequest struct {
	Type   NpcEventType
	Script *script.ScriptFile
	Npc    *Npc
}

// processNpcEventQueue dispatches any queued NPC events whose NPC
// is not currently delayed. Runs BEFORE processNpcs each tick,
// matching TS World.ts:356. Events for delayed NPCs are skipped
// (left in the queue) and retried next tick — matches TS
// World.ts:664-673.
func (s *Server) processNpcEventQueue() {
	i := 0
	for i < len(s.npcEventQueue) {
		req := s.npcEventQueue[i]
		if req.Npc.delayed {
			i++
			continue
		}
		s.npcEventQueue = append(s.npcEventQueue[:i], s.npcEventQueue[i+1:]...)
		s.runNpcScript(req.Script, req.Npc, nil, nil)
		// don't advance i — removed current entry
	}
}
```

### `*Server` additions (`modules/world/server.go`)

Add field adjacent to `npcLoop`:

```go
	npcEventQueue []NpcEventRequest
```

### `*Npc` additions (`modules/world/npc.go`)

Add `baseType` field to the struct (place near `typeId` since it's
a parallel concept):

```go
	// === lifecycle ===
	lifecycle                  int
	lifecycleTick              int
	respawnRate                int
	dead                       bool
	startX, startZ, startLevel int
	baseType                   int  // original typeId for revertType
```

Seed in `NewNpc`:

```go
		typeId:          typeId,
		baseType:        typeId,
```

Add `revertType` method (append near `ResetHP`/`Slot`):

```go
// revertType restores the NPC to its baseline type and resets state
// that should not persist across a respawn or revert-from-changetype.
// Matches TS Npc.resetEntity at Engine-TS/.../Npc.ts:280-317, minus
// hunt-field resets (deferred to NAI-7). Also matches the revert
// branch in the turn() Events block.
//
// What revertType does:
//   - restores typeId to baseType (for changetype'd NPCs)
//   - reseeds stats (curHP, baseHP) from typ.Stats
//   - clears the script queue
//   - clears waypoints
//   - sets tele = true + raises NpcMaskChangeType
//
// What revertType does NOT do (deferred):
//   - hunt-field resets (NAI-7)
//   - varn resets (future)
//   - activeScript clear (intentional — a revert should not cancel
//     an in-flight script; TS behaviour)
func (n *Npc) revertType() {
	if n.typeId != n.baseType {
		n.typeId = n.baseType
		n.uid = (n.typeId << 16) | n.nid
		if n.server != nil && n.server.npcTypes != nil {
			if n.baseType >= 0 && n.baseType < len(n.server.npcTypes.Configs) {
				n.typ = n.server.npcTypes.Configs[n.baseType]
			}
		}
	}
	n.curHP = initialHP(n.typ)
	n.baseHP = initialHP(n.typ)
	n.queue = nil
	n.waypointIndex = -1
	n.tele = true
	n.masks |= rsbuf.NpcMaskChangeType
}
```

### `*Server.removeNpc` wiring (`modules/world/npc_registry.go`)

Current state: method exists but unused (linter ℹ). NAI-5 activates it.
Final body:

```go
// removeNpc marks n as logically absent from the world by setting
// n.dead = true. Does NOT remove n from s.npcs[] or s.npcLoop —
// that registry manipulation is deferred. Callers that need a true
// removal will get it when a future sub-spec wires script-driven
// NPC creation/deletion.
func (s *Server) removeNpc(n *Npc) {
	n.dead = true
}
```

(If the current body does more than this, the implementer reduces it
to this form. Verify during plan write.)

### Tick-driver change (`modules/world/tick.go`)

Inside `runTickLoopWithRate`, add the event-queue phase BEFORE the
existing `s.processNpcs()` call:

```go
		s.processClientsIn()
		s.processActiveScripts()
		s.processPlayerTimers()
		s.processPathing()
		s.processInteractions()
		s.processNpcEventQueue()  // NEW — matches TS World.ts:356
		s.processNpcs()
		s.processLogouts()
		// ... rest unchanged
```

### `Npc.turn()` restructure (`modules/world/npc_ai.go`)

Complete new body:

```go
// turn runs once per tick from processNpcs.
func (n *Npc) turn(s *Server) {
	// === Script-lifecycle prefix (UNCHANGED from NAI-2/3/4) ===
	// Matches TS Npc.ts:112 "if (this.isActive)" guard.
	if !n.dead {
		// Delayed expiration. Matches TS Npc.ts:113.
		if n.delayed && s.currentTick >= n.delayedUntil {
			n.delayed = false
		}
		// Resume suspended script. Matches TS Npc.ts:116-118.
		if !n.delayed && n.activeScript != nil &&
			n.activeScript.Execution == script.NpcSuspended {
			state := n.activeScript
			state.Execution = script.Running
			s.resumeOrFinishNpc(state, n)
		}
	}

	// === Events block (NEW — matches TS Npc.ts:121-151) ===
	if !n.delayed {
		n.lifecycleTick--
		if n.lifecycleTick == 0 {
			switch n.lifecycle {
			case NpcLifecycleRespawn:
				if n.dead {
					// Respawn: flip dead, reset position, revert type.
					n.dead = false
					n.x, n.z, n.level = n.startX, n.startZ, n.startLevel
					n.revertType()
				} else {
					// Revert morphed NPC.
					n.revertType()
				}
			case NpcLifecycleDespawn:
				if !n.dead {
					s.removeNpc(n)
					if s.scriptProvider != nil && n.typ != nil {
						sf := s.scriptProvider.GetByTrigger(
							script.TriggerAiDespawn, n.typeId, n.typ.Category)
						if sf != nil {
							s.npcEventQueue = append(s.npcEventQueue,
								NpcEventRequest{
									Type:   NpcEventDespawn,
									Script: sf,
									Npc:    n,
								})
						}
					}
				}
			}
		}
	}

	// === isValid gate (NEW — matches TS Npc.ts:154) ===
	if n.dead || n.delayed {
		return
	}

	// === Timer + queue (UNCHANGED) ===
	s.processNpcTimer(n)
	s.processNpcQueue(n)

	// === Movement/wander/patrol (UNCHANGED) ===
	if n.moveRestrict == MoveRestrictNoMove {
		return
	}
	n.lastTickX, n.lastTickZ, n.lastLevel = n.x, n.z, n.level
	n.tele = false

	if n.waypointIndex >= 0 {
		n.advanceWaypoint(s)
		n.wanderCounter = 0
	} else {
		n.wanderCounter++
		switch n.targetOp {
		case NpcModeWander:
			n.wanderMode(s)
		case NpcModePatrol:
			n.patrolMode(s)
		}
		if n.wanderCounter > 500 && (n.x != n.startX || n.z != n.startZ) {
			n.x, n.z, n.level = n.startX, n.startZ, n.startLevel
			n.tele = true
			n.wanderCounter = 0
		}
	}
}
```

**Key removal:** the old `if n.dead { n.lifecycleTick--; if <= 0 &&
Respawn { flip } return }` block is GONE. Its semantics are absorbed
into the Events block + isValid gate. The existing
`TestKillSetsDeadAndLifecycleTick` + `TestRespawnAfterKill` tests
should still pass — Kill() sets `n.lifecycleTick = respawnRate` and
the new Events block decrements to 0 over respawnRate ticks, firing
the RESPAWN-dead branch (flip + revertType).

**Caveat on `TestRespawnAfterKill`:** the old code set masks on
respawn via `n.masks |= rsbuf.NpcMaskChangeType` directly in turn().
The new code routes through `revertType()` which sets the same mask.
Semantics preserved.

## Test strategy

All tests in new file `modules/world/npc_event_queue_test.go` unless
noted.

### Unit tests

1. **`TestNewNpcSeedsBaseType`** — `NewNpc` with `typeId=42` → `n.baseType == 42`.
2. **`TestNpcRevertTypeRestoresBaseType`** — set `n.typeId = 99`,
   `n.uid = (99<<16)|n.nid`, call `revertType()`, assert `n.typeId ==
   n.baseType`, `n.uid` recomputed.
3. **`TestNpcRevertTypeClearsQueue`** — `n.queue = []script.NpcQueueRequest{{...}}`,
   call `revertType()`, assert `n.queue == nil` or `len == 0`.
4. **`TestNpcRevertTypeClearsWaypoints`** — `n.waypointIndex = 3`,
   call `revertType()`, assert `n.waypointIndex == -1`.
5. **`TestNpcRevertTypeRaisesTeleAndMask`** — call `revertType()`,
   assert `n.tele == true` and `n.masks & NpcMaskChangeType != 0`.

### Integration tests

6. **`TestNpcTurnEventsRespawnPathAfterKill`** — build `*Server`,
   `n.Kill()` (sets `dead=true`, `lifecycleTick=respawnRate`), tick
   respawnRate times via `n.turn(s)`, assert post:
   `n.dead == false`, `n.x == n.startX`, `n.z == n.startZ`,
   `n.tele == true`, `n.masks & NpcMaskChangeType != 0`.
7. **`TestNpcTurnEventsDoesNotFireWhileDelayed`** — `n.delayed = true`,
   `n.delayedUntil = s.currentTick + 999`, `n.lifecycleTick = 1`,
   tick 5 times, assert `n.lifecycleTick` still `== 1` (no decrement).
8. **`TestNpcTurnEventsDespawnEnqueuesEvent`** — set
   `n.lifecycle = NpcLifecycleDespawn`, `n.lifecycleTick = 2`, no
   `scriptProvider` (or no `ai_despawn` registered), tick 2 times,
   assert `n.dead == true`, `len(s.npcEventQueue) == 0` (no script
   means no enqueue).
9. **`TestProcessNpcEventQueueSkipsDelayedNpcs`** — manually push an
   `NpcEventRequest` into `s.npcEventQueue`, set
   `req.Npc.delayed = true`, call `s.processNpcEventQueue()`, assert
   entry still present (skipped).

## Fidelity notes

1. **Strict `== 0` post-decrement gate** (TS Npc.ts:121) preserved in
   Go. Fresh NPCs with `lifecycleTick = 0` will see the Events block
   decrement to `-1` each tick; never re-trigger. Matches TS.
2. **`Npc.lifecycleTick` visibility.** Existing Go NPCs start with
   `lifecycleTick = 0` (not set in NewNpc; Go zero-value). TS also
   starts at 0 (Entity field default). Decrement goes negative
   forever for RESPAWN-alive NPCs; benign.
3. **`Kill()` is a test helper** that sets `dead=true` AND
   `lifecycleTick=respawnRate`. Under NAI-5, the NAI-5 Events block
   then counts down respawnRate ticks before firing RESPAWN+dead →
   respawn. This matches how combat-death would work in a future
   sub-spec (combat sets `dead + lifecycleTick`; Events handles
   respawn).
4. **`revertType()` vs TS `resetEntity(respawn)`.** TS's method
   handles both respawn and revert paths, with a `respawn` bool
   choosing whether to reset everything or just call
   `super.resetPathingEntity()`. Go's `revertType` does the "full
   reset" version unconditionally — both the respawn branch and the
   revert branch in the Events block call it identically. This is a
   minor divergence (TS's non-respawn branch is just
   `resetPathingEntity`; Go does full reset always). Rationale: for
   NAI-5, a changetype revert benefits from the full reset too, and
   `resetPathingEntity` isn't wired separately in Go. If future
   sub-specs add pathing reset, split the method then.
5. **Hunt-field resets deferred to NAI-7.** Tracked as part of NAI-7
   scope (not a new `nai_followups.md` entry — it's an explicit
   roadmap ordering).

## Rough LOC

- `modules/world/npc_event_queue.go` (new): ~50
- `modules/world/npc.go`: +~30 (baseType field + revertType method)
- `modules/world/npc_ai.go`: +~35 (Events block + isValid gate),
  -~10 (old simplified lifecycle code)
- `modules/world/npc_registry.go`: +~5 (removeNpc body)
- `modules/world/tick.go`: +1 line
- `modules/world/server.go`: +1 line (field)
- `modules/world/npc_event_queue_test.go` (new): ~200 (9 tests)

Total ≈ 320 LOC. Slightly over the roadmap's ~180 estimate because
the test suite is heavier than the production code (9 tests covering
the state-machine branches + the NpcEventQueue behavior).

## Dependencies

- **Blocks:** NAI-7 (hunt core) — will extend `revertType` with hunt
  resets.
- **Blocked by:** NAI-2 (`runNpcScript`, `ActiveNpc` interface,
  `*Npc.server` back-reference for `revertType` to access
  `npcTypes`). NAI-3 queue field (revertType clears `n.queue`).
  NAI-4 NOT required (timer fields don't need reset here; deferred
  as part of full `resetEntity` port).

## Verifications to resolve during plan-write

1. Does the current `removeNpc` at `modules/world/npc_registry.go:42`
   have a body, or is it empty? Plan needs to know whether to
   replace or add content.
2. Does `handleNpcChangeType` (from S6c) set `n.lifecycleTick =
   duration` so the new Events block picks up the revert? If not,
   file a small follow-up fix in NAI-5 (in-scope; it's wiring the
   Events block to its expected producer).
3. Does `*Npc.queue` get cleared in any other place currently? If
   so, document to avoid double-clear.
