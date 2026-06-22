# NAI-5 Lifecycle State Machine + NpcEventQueue + `ai_despawn` — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace `Npc.turn()`'s simplified `dead`-bool respawn with a full TS-faithful Events block (`Engine-TS/.../Npc.ts:121-151`) covering RESPAWN (respawn + revertType) and DESPAWN (remove + queue `ai_despawn`) paths, plus `NpcEventQueue` infrastructure and `revertType()` method.

**Architecture:** Four tasks. (1) Add `baseType` + `revertType` on `*Npc`. (2) Add `NpcEventType`/`NpcEventRequest`/`processNpcEventQueue` infrastructure and simplify existing `removeNpc` to just `n.dead = true`. (3) Restructure `Npc.turn()` with Events block + isValid gate, removing the old simplified lifecycle code. (4) Wire `processNpcEventQueue` into the tick driver before `processNpcs`, update `nai_followups.md` memory, close NAI-5.

**Tech Stack:** Go 1.26+, existing `pkg/script` runtime, existing tick loop.

**Spec:** `docs/superpowers/specs/2026-04-22-nai-5-lifecycle-design.md`

**Roadmap:** `docs/superpowers/specs/2026-04-22-npc-ai-tick-decomposition-design.md`

---

## File Structure

**Created:**
- `modules/world/npc_event_queue.go` — `NpcEventType`, `NpcEventRequest`, `processNpcEventQueue`
- `modules/world/npc_event_queue_test.go` — tests for all 4 tasks

**Modified:**
- `modules/world/npc.go` — `+baseType` field, `+revertType` method, NewNpc seeds baseType
- `modules/world/npc_registry.go` — simplify `removeNpc` body to `n.dead = true` only
- `modules/world/npc_ai.go` — new Events block + isValid gate in turn(); remove old simplified dead-block trailer
- `modules/world/server.go` — `+npcEventQueue` field
- `modules/world/tick.go` — `+s.processNpcEventQueue()` call before `s.processNpcs()`

Four tasks. Task 4 closes NAI-5.

---

## Task 1: baseType field + revertType method + 5 unit tests

**Files:**
- Modify: `modules/world/npc.go` (add field + method + NewNpc seed)
- Create: `modules/world/npc_event_queue_test.go` (5 unit tests)

- [ ] **Step 1: Write the failing tests**

Create `modules/world/npc_event_queue_test.go`:

```go
package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/rsbuf"
	"github.com/zsrv/goscape/pkg/script"
)

func newNpcForLifecycleTest(t *testing.T) *Npc {
	t.Helper()
	typ := &objtype.NpcType{
		ConfigType: objtype.ConfigType{ID: 0, DebugName: "test_npc"},
		Stats:      []uint16{0, 0, 0, 10, 0, 0}, // HP=10 at NpcStatHitpoints (3)
		Category:   -1,
	}
	return NewNpc(1, 0, 3094, 3106, 0, typ)
}

func TestNewNpcSeedsBaseType(t *testing.T) {
	n := NewNpc(1, 42, 3094, 3106, 0, &objtype.NpcType{ConfigType: objtype.ConfigType{ID: 42}})
	if n.baseType != 42 {
		t.Errorf("baseType: got %d, want 42 (seeded from typeId)", n.baseType)
	}
}

func TestNpcRevertTypeRestoresBaseType(t *testing.T) {
	n := newNpcForLifecycleTest(t)
	// Simulate a prior changetype: typeId now 99, uid recomputed.
	n.typeId = 99
	n.uid = (99 << 16) | n.nid

	n.revertType()

	if n.typeId != n.baseType {
		t.Errorf("typeId: got %d, want %d (baseType)", n.typeId, n.baseType)
	}
	wantUID := (n.baseType << 16) | n.nid
	if n.uid != wantUID {
		t.Errorf("uid: got %d, want %d", n.uid, wantUID)
	}
}

func TestNpcRevertTypeClearsQueue(t *testing.T) {
	n := newNpcForLifecycleTest(t)
	n.queue = []script.NpcQueueRequest{{Trigger: script.TriggerAiQueue1, Delay: 5, IntArg: 0}}

	n.revertType()

	if len(n.queue) != 0 {
		t.Errorf("queue: got %d entries, want 0 (cleared)", len(n.queue))
	}
}

func TestNpcRevertTypeClearsWaypoints(t *testing.T) {
	n := newNpcForLifecycleTest(t)
	n.waypointIndex = 3

	n.revertType()

	if n.waypointIndex != -1 {
		t.Errorf("waypointIndex: got %d, want -1 (cleared)", n.waypointIndex)
	}
}

func TestNpcRevertTypeRaisesTeleAndMask(t *testing.T) {
	n := newNpcForLifecycleTest(t)
	n.tele = false
	n.masks = 0

	n.revertType()

	if !n.tele {
		t.Errorf("tele: got false, want true")
	}
	if n.masks&rsbuf.NpcMaskChangeType == 0 {
		t.Errorf("masks: NpcMaskChangeType bit not set")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:
```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestNewNpcSeedsBaseType|TestNpcRevertType' -v
```

Expected: compile errors — `n.baseType undefined`, `n.revertType undefined`.

- [ ] **Step 3: Add `baseType` field to `*Npc`**

In `modules/world/npc.go`, locate the `// === lifecycle ===` block in the `Npc` struct (around line 34):

```go
	// === lifecycle ===
	lifecycle                  int
	lifecycleTick              int
	respawnRate                int
	dead                       bool
	startX, startZ, startLevel int
```

Add `baseType` at the end of the block:

```go
	// === lifecycle ===
	lifecycle                  int
	lifecycleTick              int
	respawnRate                int
	dead                       bool
	startX, startZ, startLevel int
	baseType                   int
```

(gofmt may realign columns — let it win.)

- [ ] **Step 4: Seed `baseType` in `NewNpc`**

In `modules/world/npc.go`, locate the `NewNpc` struct literal. The existing `typeId: typeId,` line is around line 98. Add a companion line immediately after:

```go
		typeId:          typeId,
		baseType:        typeId,
```

- [ ] **Step 5: Add `revertType` method**

In `modules/world/npc.go`, append at the bottom (after the existing `Slot()` method and any NAI-2/3/4 methods):

```go
// revertType restores the NPC to its baseline type and resets state
// that should not persist across a respawn or revert-from-changetype.
// Matches TS Npc.resetEntity at Engine-TS/.../Npc.ts:280-317, minus
// hunt-field resets (deferred to NAI-7 per the NAI roadmap).
//
// What revertType does:
//   - restores typeId to baseType (for changetype'd NPCs)
//   - recomputes uid from the restored typeId
//   - resets the typ pointer to the baseType's NpcType config (when
//     server + npcTypes are wired)
//   - reseeds curHP/baseHP from typ.Stats via initialHP
//   - clears the script queue
//   - clears waypoints
//   - sets tele = true + raises NpcMaskChangeType
//
// What revertType does NOT do (intentional):
//   - hunt-field resets (NAI-7 scope; those fields don't exist yet)
//   - varn resets (future; VarNpc subsystem not yet wired)
//   - activeScript clear (TS behaviour: a revert does not cancel an
//     in-flight script)
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

**Note on imports:** `modules/world/npc.go` already imports `pkg/script` (from NAI-2 onwards) and `pkg/rsbuf` (for masks). Verify via `head -15 modules/world/npc.go`. If `rsbuf` is missing from imports (unlikely since masks like `NpcMaskChangeType` are used elsewhere in the package, but the npc.go file specifically may not import it), add `"github.com/zsrv/goscape/pkg/rsbuf"` to the import block.

- [ ] **Step 6: Run tests to verify they pass + full suite**

Run:
```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestNewNpcSeedsBaseType|TestNpcRevertType' -v
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -count=1
```

Expected: clean build; all 5 new tests PASS; all prior tests still PASS (the new field/method are additive and unused by existing code).

- [ ] **Step 7: Commit**

```bash
git add modules/world/npc.go modules/world/npc_event_queue_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-5 Npc.baseType + revertType method

Add baseType field to *Npc lifecycle block; seed in NewNpc from
typeId. Add revertType() method restoring typ to baseType, reseeding
stats via initialHP, clearing queue + waypoints, raising tele +
NpcMaskChangeType. Matches TS Npc.resetEntity at Npc.ts:280-317
with hunt-field resets explicitly deferred to NAI-7 (per roadmap).

No consumer yet — Task 3 wires revertType from the new Events block.
This task ships infrastructure that Task 3 consumes, matching the
established NAI pattern of structural-first / integration-second.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: NpcEventQueue infrastructure + simplify removeNpc + 1 test

**Files:**
- Create: `modules/world/npc_event_queue.go`
- Modify: `modules/world/server.go` (add `npcEventQueue` field)
- Modify: `modules/world/npc_registry.go` (simplify `removeNpc`)
- Modify: `modules/world/npc_event_queue_test.go` (add 1 integration test)

- [ ] **Step 1: Write the failing test**

Append to `modules/world/npc_event_queue_test.go`:

```go
func TestProcessNpcEventQueueSkipsDelayedNpcs(t *testing.T) {
	s := newServerForScriptTest(t)
	s.currentTick = 100
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.delayed = true
	n.delayedUntil = s.currentTick + 999

	sf := &script.ScriptFile{
		Name:    "ai_despawn_stub",
		Opcodes: []script.Opcode{script.OpReturn},
	}
	s.npcEventQueue = append(s.npcEventQueue, NpcEventRequest{
		Type:   NpcEventDespawn,
		Script: sf,
		Npc:    n,
	})

	s.processNpcEventQueue()

	if len(s.npcEventQueue) != 1 {
		t.Errorf("npcEventQueue: got len %d, want 1 (delayed NPC's event must be skipped, not removed)", len(s.npcEventQueue))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:
```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestProcessNpcEventQueueSkipsDelayedNpcs -v
```

Expected: compile errors — `undefined: NpcEventRequest`, `s.npcEventQueue undefined`, `s.processNpcEventQueue undefined`, `undefined: NpcEventDespawn`.

- [ ] **Step 3: Create `modules/world/npc_event_queue.go`**

Create the file with full content:

```go
package world

import "github.com/zsrv/goscape/pkg/script"

// NpcEventType mirrors TS NpcEventType at
// Engine-TS/src/engine/entity/NpcEventRequest.ts.
// NpcEventSpawn is reserved for TS fidelity but has no producer in
// NAI-5 (no script-driven NPC creation yet); NpcEventDespawn is
// queued by the DESPAWN branch of the Npc.turn() Events block.
type NpcEventType int

const (
	NpcEventSpawn   NpcEventType = 0
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
// matching TS World.ts:356. Events for delayed NPCs are left in
// the queue and retried next tick — matches TS World.ts:664-673.
//
// Iteration uses the same removal-before-fire + don't-advance-i
// pattern as processNpcQueue (NAI-3) so a fired script that
// appends a new event sees it in the same pass.
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

- [ ] **Step 4: Add `npcEventQueue` field to `*Server`**

In `modules/world/server.go`, locate the block around line 82-85 (where `npcTypes` and `huntTypes` live from NAI-1):

```go
	npcTypes    *objtype.NPCTypeConfigs
	huntTypes   *objtype.HuntTypeConfigs
	npcs        [8192]*Npc
	npcLoop     []*Npc
	nextNpcSlot int
```

Add `npcEventQueue` after `npcLoop`:

```go
	npcTypes      *objtype.NPCTypeConfigs
	huntTypes     *objtype.HuntTypeConfigs
	npcs          [8192]*Npc
	npcLoop       []*Npc
	npcEventQueue []NpcEventRequest
	nextNpcSlot   int
```

(gofmt realigns column widths.)

- [ ] **Step 5: Simplify `removeNpc`**

In `modules/world/npc_registry.go`, the current `removeNpc` body is:

```go
// removeNpc clears the npc's slot and removes from npcLoop.
func (s *Server) removeNpc(n *Npc) {
	if n.nid < 1 || n.nid >= len(s.npcs) || s.npcs[n.nid] != n {
		return
	}
	s.npcs[n.nid] = nil
	for i, ln := range s.npcLoop {
		if ln == n {
			s.npcLoop = append(s.npcLoop[:i], s.npcLoop[i+1:]...)
			return
		}
	}
}
```

Replace with:

```go
// removeNpc marks n as logically absent from the world by setting
// n.dead = true. Does NOT remove n from s.npcs[] or s.npcLoop —
// that registry manipulation is deferred to a future sub-spec
// when script-driven NPC creation/deletion lands. The old
// registry-manipulation body was unused pre-NAI-5 and was
// mid-tick-iteration-unsafe (spliced npcLoop during processNpcs
// iteration), so replacing it with the dead-bool model is also a
// correctness improvement.
func (s *Server) removeNpc(n *Npc) {
	n.dead = true
}
```

- [ ] **Step 6: Run test to verify it passes + full suite**

Run:
```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestProcessNpcEventQueueSkipsDelayedNpcs -v
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -count=1
```

Expected: clean build; new test PASS; no regressions (the old `removeNpc` body was unused, so changing it is safe).

- [ ] **Step 7: Commit**

```bash
git add modules/world/npc_event_queue.go modules/world/server.go modules/world/npc_registry.go modules/world/npc_event_queue_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-5 NpcEventQueue infrastructure + simplify removeNpc

Add NpcEventType (Spawn=0 reserved, Despawn=1) + NpcEventRequest
struct + *Server.processNpcEventQueue method (skips delayed NPCs,
remove-before-fire + don't-advance-i iteration matching NAI-3's
processNpcQueue). Add s.npcEventQueue field.

Simplify removeNpc from registry manipulation (mid-tick-iteration-
unsafe, unused pre-NAI-5) to just n.dead = true. True registry
manipulation deferred to a future script-driven-NPC sub-spec.

No producer yet — Task 3 wires the DESPAWN branch of the Events
block that enqueues ai_despawn scripts via this queue.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: Npc.turn() Events block + isValid gate + 3 integration tests

**Files:**
- Modify: `modules/world/npc_ai.go` (Events block + isValid gate, remove old simplified lifecycle)
- Modify: `modules/world/npc_event_queue_test.go` (add 3 integration tests)

- [ ] **Step 1: Write the failing tests**

Append to `modules/world/npc_event_queue_test.go`:

```go
func TestNpcTurnEventsRespawnPathAfterKill(t *testing.T) {
	s := newServerForScriptTest(t)
	s.currentTick = 100
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.respawnRate = 5
	n.lifecycle = NpcLifecycleRespawn
	n.x, n.z = n.startX+3, n.startZ+3 // moved away from spawn before death

	n.Kill() // sets n.dead=true, n.lifecycleTick=respawnRate=5

	// Tick respawnRate times; lifecycleTick goes 5→4→3→2→1→0 on the 5th call.
	for i := 0; i < 5; i++ {
		n.turn(s)
	}

	if n.dead {
		t.Errorf("dead: got true, want false (should have respawned)")
	}
	if n.x != n.startX || n.z != n.startZ {
		t.Errorf("pos: got (%d,%d), want (%d,%d) (should reset to spawn)", n.x, n.z, n.startX, n.startZ)
	}
	if !n.tele {
		t.Errorf("tele: got false, want true (revertType raises it)")
	}
}

func TestNpcTurnEventsDoesNotFireWhileDelayed(t *testing.T) {
	s := newServerForScriptTest(t)
	s.currentTick = 100
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.delayed = true
	n.delayedUntil = s.currentTick + 999
	n.lifecycleTick = 1
	n.lifecycle = NpcLifecycleRespawn

	for i := 0; i < 5; i++ {
		n.turn(s)
	}

	if n.lifecycleTick != 1 {
		t.Errorf("lifecycleTick: got %d, want 1 (no decrement while delayed)", n.lifecycleTick)
	}
}

func TestNpcTurnEventsDespawnEnqueuesEvent(t *testing.T) {
	s := newServerForScriptTest(t)
	s.currentTick = 100
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.lifecycle = NpcLifecycleDespawn
	n.lifecycleTick = 2

	// No scriptProvider registered → GetByTrigger returns nil → no enqueue,
	// but n.dead must flip true.
	n.turn(s)
	n.turn(s)

	if !n.dead {
		t.Errorf("dead: got false, want true (DESPAWN should have fired removeNpc)")
	}
	if len(s.npcEventQueue) != 0 {
		t.Errorf("npcEventQueue: got len %d, want 0 (no ai_despawn script registered)", len(s.npcEventQueue))
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:
```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestNpcTurnEvents' -v
```

Expected: tests fail because `Npc.turn()` still has the old simplified lifecycle. Specifically:
- `TestNpcTurnEventsRespawnPathAfterKill` may pass-or-fail depending on whether the old `if n.dead { lifecycleTick--; if <= 0 && Respawn { flip } }` happens to work the same way — it actually uses `<= 0` not `== 0`, and doesn't call `revertType`, so `n.tele` may still go true but via a different path. Subtle.
- `TestNpcTurnEventsDoesNotFireWhileDelayed` FAILS because current code has no delayed-gate on lifecycleTick decrement (current code only decrements while `dead`; `dead=false` means no decrement, but the test expects lifecycleTick preserved — which IS the current behavior since `dead=false`). Actually this may PASS by coincidence — verify during run.
- `TestNpcTurnEventsDespawnEnqueuesEvent` FAILS because current code has no DESPAWN handling at all; lifecycleTick decrements only in `dead` block.

Even if some tests pass coincidentally, Step 3-4 will make them genuinely correct.

- [ ] **Step 3: Restructure `Npc.turn()`**

In `modules/world/npc_ai.go`, the current `turn` method (after NAI-4) looks like:

```go
// turn runs once per tick from processNpcs.
func (n *Npc) turn(s *Server) {
	// Script-lifecycle prefix runs only for active (non-dead) NPCs —
	// matches TS Npc.ts:112 `if (this.isActive)` guard.
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
		// Timer pass. Matches TS Npc.ts:178 (turn calls processTimers).
		s.processNpcTimer(n)
		// Queue pass. Matches TS Npc.ts:180 (turn calls processQueue).
		s.processNpcQueue(n)
	}

	if n.dead {
		n.lifecycleTick--
		if n.lifecycleTick <= 0 && n.lifecycle == NpcLifecycleRespawn {
			n.dead = false
			n.x, n.z, n.level = n.startX, n.startZ, n.startLevel
			n.tele = true
			n.masks |= rsbuf.NpcMaskChangeType
		}
		return
	}
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

Replace the entire function body with:

```go
// turn runs once per tick from processNpcs.
func (n *Npc) turn(s *Server) {
	// === Script-lifecycle prefix (NAI-2..NAI-4) ===
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

	// === Events block (NAI-5 — matches TS Npc.ts:121-151) ===
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
					// Revert morphed NPC (post-changetype).
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

	// === isValid gate (NAI-5 — matches TS Npc.ts:154) ===
	if n.dead || n.delayed {
		return
	}

	// === Timer + queue (NAI-3, NAI-4) ===
	s.processNpcTimer(n)
	s.processNpcQueue(n)

	// === Movement / wander / patrol ===
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

**Key changes:**
- Timer + queue moved OUT of the `!n.dead` block — now gated by the new `isValid` block (`if n.dead || n.delayed { return }`)
- New Events block between script-prefix and isValid gate
- Old `if n.dead { lifecycleTick--; ... return }` block is GONE — the Events block + isValid gate replace it

- [ ] **Step 4: Run tests to verify they pass + full suite + race**

Run:
```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestNpc' -v -count=1
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -count=1
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...
```

Expected: all 3 new `TestNpcTurnEvents*` tests PASS; existing `TestKillSetsDeadAndLifecycleTick`, `TestTeleportHomeAfterStuck`, `TestRespawnAfterKill` tests still PASS.

**If `TestRespawnAfterKill` FAILS:** it currently asserts `n.masks & rsbuf.NpcMaskChangeType != 0` after respawn (from the old hand-rolled mask-set). The new code raises the same mask via `revertType()`. If the test calls turn() exactly `respawnRate` times, the new strict `== 0` gate fires on the respawnRate-th turn (decrement goes 5→4→3→2→1→0 on the 5th call, if respawnRate=5). Verify timing by reading the test.

If `TestRespawnAfterKill` uses `respawnRate` like `for range n.respawnRate { n.turn(s) }`, that's `respawnRate` iterations (Go 1.22+ `range int` idiom). With `respawnRate=50` from `newWanderNpc`, lifecycleTick goes 50→49→...→1→0 on the 50th call. Fires revertType, flips dead, sets tele+mask. Test should PASS.

- [ ] **Step 5: Commit**

```bash
git add modules/world/npc_ai.go modules/world/npc_event_queue_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-5 Npc.turn() Events block + isValid gate

Replace the simplified "if n.dead { lifecycleTick--; flip if Respawn }"
trailer with a TS-faithful Events block (Npc.ts:121-151) firing on
strict --lifecycleTick == 0 post-decrement, handling three paths:
RESPAWN+dead → respawn+revertType, RESPAWN+alive → revertType (for
post-changetype reverts), DESPAWN+alive → removeNpc + enqueue
ai_despawn trigger via npcEventQueue.

Add isValid gate (Npc.ts:154): skip timer+queue+movement when
n.dead || n.delayed. Timer + queue passes moved out of the !n.dead
block to be gated by the new isValid instead.

Remove the old simplified dead-block trailer. Its semantics are now
in the Events block (Respawn-dead path) with strict-equality gate.
Existing TestKillSetsDeadAndLifecycleTick / TestRespawnAfterKill
tests still pass — Kill() sets lifecycleTick=respawnRate which the
new Events block handles identically after respawnRate turns.

Three new integration tests cover the RESPAWN+dead path, the
delayed-gate on lifecycleTick decrement, and the DESPAWN path's
n.dead flip + (no-enqueue-when-no-script) observable.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: Tick-driver wire + memory update + close NAI-5

**Files:**
- Modify: `modules/world/tick.go` (add `s.processNpcEventQueue()` call)
- Modify: `$HOME/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md` (track npc_changetype duration wiring)

- [ ] **Step 1: Wire `processNpcEventQueue` in the tick driver**

In `modules/world/tick.go`, locate the `runTickLoopWithRate` method — specifically the phase-call sequence around lines 35-47:

```go
		s.processClientsIn()
		s.processActiveScripts()
		s.processPlayerTimers()
		s.processPathing()
		s.processInteractions()
		s.processNpcs()
		s.processLogouts()
```

Insert `s.processNpcEventQueue()` immediately BEFORE `s.processNpcs()`:

```go
		s.processClientsIn()
		s.processActiveScripts()
		s.processPlayerTimers()
		s.processPathing()
		s.processInteractions()
		s.processNpcEventQueue() // NAI-5: matches TS World.ts:356
		s.processNpcs()
		s.processLogouts()
```

- [ ] **Step 2: Run full suite + race**

Run:
```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -count=1
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...
```

Expected: clean build; all tests PASS; race suite clean.

- [ ] **Step 3: Update `nai_followups.md` memory with the `npc_changetype` duration follow-up**

Open `$HOME/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md`. After the existing "## From NAI-3 (2026-04-22)" block, append:

```markdown

## From NAI-5 (2026-04-22)

### Unassigned (small fix): wire `npc_changetype` duration into new Events block

NAI-5 adds `revertType()` and the Events block that calls it when
`lifecycleTick == 0` on a RESPAWN-lifecycle NPC. But
`handleNpcChangeType` at `pkg/script/handlers_npc.go:176-184`
currently DISCARDS the `duration` param (comment explicitly says
"S6c discards duration — timed revert is deferred to a future AI
sub-spec").

Wiring is a small change with moderate reach:

1. `ActiveNpc.ChangeType` signature grows a second param:
   `ChangeType(newType, duration int)`.
2. `*Npc.ChangeType` sets `n.typeId = newType` AND
   `n.lifecycleTick = duration` (if duration > 0).
3. `handleNpcChangeType` pops duration, passes to
   `ActiveNpc.ChangeType(newType, duration)`.
4. `mockNpc.changeTypeCalls` records both values.
5. `mockActiveNpc.ChangeType` stub updated.

Not in NAI-5 scope because it requires touching the `ActiveNpc`
interface + 2 mocks + handler + 1 test. Defer to either a polish
pass or fold into NAI-7 (which already needs interface changes).
```

- [ ] **Step 4: Commit, closing NAI-5**

```bash
git add modules/world/tick.go $HOME/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-5 wire processNpcEventQueue in tick loop — closes NAI-5

Add s.processNpcEventQueue() call between processInteractions and
processNpcs in runTickLoopWithRate. Matches TS World.ts:356 ordering
(NPC events dispatch before per-NPC turn() work, so despawn triggers
run as the world starts the NPC frame, not during it).

Also tracks one NAI-5 follow-up in nai_followups memory: wiring
npc_changetype's duration param into the new Events-block-driven
revert path. Deferred because it requires ActiveNpc interface
changes + mock updates that cascade beyond NAI-5's core scope.

Closes NAI-5 (lifecycle state machine + NpcEventQueue + ai_despawn).
The full NAI-5 vertical slice: RESPAWN NPCs that die (n.dead=true
+ lifecycleTick=respawnRate) now respawn through revertType, which
restores the baseline typ, reseeds stats, clears queue+waypoints,
raises tele + NpcMaskChangeType. DESPAWN NPCs with ai_despawn scripts
registered will enqueue events dispatched before the next
processNpcs pass. Five NAI-5 tests + three existing NAI tick tests
all green under -race.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Self-review checklist results

**1. Spec coverage:**

| Spec section | Task |
|---|---|
| `*Npc.baseType` field | Task 1 |
| `NewNpc` seeds baseType | Task 1 |
| `*Npc.revertType()` method | Task 1 |
| `NpcEventType` + `NpcEventRequest` types | Task 2 |
| `*Server.npcEventQueue` field | Task 2 |
| `processNpcEventQueue` method | Task 2 |
| `removeNpc` body simplified to `n.dead = true` | Task 2 |
| `Npc.turn()` Events block | Task 3 |
| `Npc.turn()` isValid gate | Task 3 |
| Tick-driver wire | Task 4 |
| nai_followups.md memory update | Task 4 |
| Test: `TestNewNpcSeedsBaseType` | Task 1 |
| Test: `TestNpcRevertTypeRestoresBaseType` | Task 1 |
| Test: `TestNpcRevertTypeClearsQueue` | Task 1 |
| Test: `TestNpcRevertTypeClearsWaypoints` | Task 1 |
| Test: `TestNpcRevertTypeRaisesTeleAndMask` | Task 1 |
| Test: `TestProcessNpcEventQueueSkipsDelayedNpcs` | Task 2 |
| Test: `TestNpcTurnEventsRespawnPathAfterKill` | Task 3 |
| Test: `TestNpcTurnEventsDoesNotFireWhileDelayed` | Task 3 |
| Test: `TestNpcTurnEventsDespawnEnqueuesEvent` | Task 3 |

All 9 spec-listed tests have tasks. No gaps.

**2. Placeholder scan:** No TBDs/TODOs/vague steps. Every code step contains complete code. Every run step has exact command + expected output. Task 3 Step 4 discusses contingency test behavior (stale field timing) — that's guidance for the implementer, not a placeholder.

**3. Type consistency:** `baseType` (lowercase, unexported int), `revertType` (lowercase unexported method), `NpcEventType`/`NpcEventRequest` (exported), `npcEventQueue` (unexported slice field), `processNpcEventQueue` (unexported method), `NpcLifecycleRespawn`/`NpcLifecycleDespawn` (existing exported constants at `modules/world/npc.go:8-12`), `script.TriggerAiDespawn` (existing symbol) — all consistent across the 4 tasks.

---

## Commit trail (for reference)

Four commits close NAI-5:

1. `feat(world): NAI-5 Npc.baseType + revertType method`
2. `feat(world): NAI-5 NpcEventQueue infrastructure + simplify removeNpc`
3. `feat(world): NAI-5 Npc.turn() Events block + isValid gate`
4. `feat(world): NAI-5 wire processNpcEventQueue in tick loop — closes NAI-5`

Each commit leaves the tree green; the final one closes NAI-5 and tracks one follow-up item in `nai_followups.md`.
