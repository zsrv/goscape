# Sub-spec 6a: Interaction State Machine — Implementation Plan

> Compact 2-task plan. Impl first, then tests — standard TDD inversion used here because the state machine has many small pieces that are easier to review as one cohesive change than individually.

**Goal:** Wire OpNpc1..5 → SetInteraction → processInteraction tick hook.
**Spec:** `docs/superpowers/specs/2026-04-21-interaction-state-machine-design.md`.
**Build prefix:** `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go ...`
**Commit flag:** `--no-gpg-sign`.

---

## Task 1: Production code

**Files:**
- Modify: `pkg/io/protocol/game/server/prot.go` (+ OpUnsetMapFlag)
- Modify: `modules/world/player.go` (+ `interactionKind` field)
- Create: `modules/world/interaction.go`
- Create: `modules/world/handler_opnpc.go`
- Modify: `modules/world/handlers_game.go` (+ 5 registrations)
- Modify: `modules/world/tick.go` (+ processInteractions phase)

- [ ] **Step 1.1: Add `OpUnsetMapFlag` to prot.go**

Inside the existing `var (...)` block:
```go
OpUnsetMapFlag = Op{Opcode: 19, PayloadSize: 0}
```

- [ ] **Step 1.2: Add `interactionKind` field to Player**

In `modules/world/player.go`, inside the interaction block (near `target`, `targetOp`):
```go
interactionKind InteractionKind
```

- [ ] **Step 1.3: Create `modules/world/interaction.go`**

```go
package world

import (
	"github.com/zsrv/goscape/pkg/coordgrid"
	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
)

// InteractionKind distinguishes engine-triggered from script-queued
// interactions. Only InteractionEngine is used in sub-spec 6a;
// InteractionScript is reserved for the RuneScript integration.
type InteractionKind int

const (
	InteractionEngine InteractionKind = iota
	InteractionScript
)

// sendUnsetMapFlag clears the client's pending map-click indicator.
func sendUnsetMapFlag(p *Player) {
	p.writeOut(gameserver.OpUnsetMapFlag, nil)
}

// SetInteraction anchors the interaction state machine on a target entity.
func (p *Player) SetInteraction(kind InteractionKind, target entity, op int) {
	p.target = target
	p.targetOp = op
	p.interactionKind = kind
	p.apRange = 10
	p.apRangeCalled = false
	p.interacted = false
	p.repathed = false
}

// ClearInteraction resets interaction state to idle.
func (p *Player) ClearInteraction() {
	p.target = nil
	p.targetOp = -1
	p.apRangeCalled = false
	p.interacted = false
	p.repathed = false
}

// ClearPendingAction cancels any queued action before a fresh interaction
// is set. For 6a it's an alias for ClearInteraction.
func (p *Player) ClearPendingAction() {
	p.ClearInteraction()
}

// processInteraction runs once per tick per player after pathing.
//   - No target: no-op.
//   - Delayed: no-op.
//   - Target on different level: clear + UnsetMapFlag.
//   - In operable distance: face target, interacted=true.
//   - Out of range: set waypoint toward target.
func (p *Player) processInteraction() {
	if p.target == nil {
		return
	}
	if p.client == nil || p.client.server == nil {
		return
	}
	s := p.client.server
	if p.delayed && s.currentTick < p.delayedUntil {
		return
	}

	tx, tz, tlevel := p.target.Coords()
	if tlevel != p.level {
		p.ClearInteraction()
		sendUnsetMapFlag(p)
		return
	}

	if inOperableDistance(p.x, p.z, tx, tz) {
		if npc, ok := p.target.(*Npc); ok {
			p.SetFaceEntity(npc.nid)
		}
		p.interacted = true
		return
	}

	if !p.repathed {
		p.pathToTarget(tx, tz)
		p.repathed = true
	}
}

// inOperableDistance is Chebyshev <= 1 between (px,pz) and (tx,tz),
// excluding the same tile. Adjacent (including diagonals) counts as
// operable for 1x1 targets. Multi-tile + strict-adjacency come with
// real combat.
func inOperableDistance(px, pz, tx, tz int) bool {
	dx := px - tx
	if dx < 0 {
		dx = -dx
	}
	dz := pz - tz
	if dz < 0 {
		dz = -dz
	}
	if dx > 1 || dz > 1 {
		return false
	}
	return !(dx == 0 && dz == 0)
}

// pathToTarget sets a waypoint to (tx, tz) via the existing move-click
// pathing pipeline so pathfinding (or direct-step mode) applies uniformly.
func (p *Player) pathToTarget(tx, tz int) {
	packed := []int{coordgrid.PackCoord(p.level, tx, tz)}
	needsFinding := !p.client.server.cfg.NodeClientRoutefinder
	p.pathToMoveClick(packed, needsFinding)
}
```

- [ ] **Step 1.4: Create `modules/world/handler_opnpc.go`**

```go
package world

import (
	"github.com/zsrv/goscape/pkg/io/packet"
)

// handleOpNpc is the shared implementation for OPNPC1..OPNPC5.
// op is 1..5. Payload = p2(slot).
func handleOpNpc(p *Player, payload []byte, op int) error {
	if p.client == nil || p.client.server == nil {
		return nil
	}
	s := p.client.server

	if p.delayed && s.currentTick < p.delayedUntil {
		sendUnsetMapFlag(p)
		return nil
	}

	if len(payload) < 2 {
		sendUnsetMapFlag(p)
		return nil
	}

	r := packet.NewPacket(payload)
	slot := int(r.G2())

	if slot < 0 || slot >= len(s.npcs) {
		sendUnsetMapFlag(p)
		return nil
	}
	npc := s.npcs[slot]
	if npc == nil || !npc.active || npc.dead {
		sendUnsetMapFlag(p)
		return nil
	}

	// NpcType.Op[op-1] must be a non-empty, non-"hidden" string.
	// RuneScript will later replace this with trigger-existence lookup.
	if npc.npcType == nil || len(npc.npcType.Op) < op ||
		npc.npcType.Op[op-1] == "" || npc.npcType.Op[op-1] == "hidden" {
		sendUnsetMapFlag(p)
		return nil
	}

	p.ClearPendingAction()
	p.SetInteraction(InteractionEngine, npc, op)
	return nil
}

func handleOpNpc1(p *Player, payload []byte) error { return handleOpNpc(p, payload, 1) }
func handleOpNpc2(p *Player, payload []byte) error { return handleOpNpc(p, payload, 2) }
func handleOpNpc3(p *Player, payload []byte) error { return handleOpNpc(p, payload, 3) }
func handleOpNpc4(p *Player, payload []byte) error { return handleOpNpc(p, payload, 4) }
func handleOpNpc5(p *Player, payload []byte) error { return handleOpNpc(p, payload, 5) }
```

- [ ] **Step 1.5: Register handlers in `modules/world/handlers_game.go`**

In the existing `init()`:
```go
gameHandlers[194] = handleOpNpc1 // OPNPC1
gameHandlers[8]   = handleOpNpc2 // OPNPC2
gameHandlers[27]  = handleOpNpc3 // OPNPC3
gameHandlers[113] = handleOpNpc4 // OPNPC4
gameHandlers[100] = handleOpNpc5 // OPNPC5
```

- [ ] **Step 1.6: Add `processInteractions` tick phase to `modules/world/tick.go`**

In `runTickLoopWithRate`, insert after `s.processPathing()`:
```go
s.processInteractions()
```

Add method at the bottom of `tick.go`:
```go
func (s *Server) processInteractions() {
	s.playersMu.RLock()
	players := make([]*Player, len(s.playerLoop))
	copy(players, s.playerLoop)
	s.playersMu.RUnlock()
	for _, p := range players {
		p.processInteraction()
	}
}
```

- [ ] **Step 1.7: Build + existing tests**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
```
Expected: PASS (no regressions; new functions not yet exercised).

- [ ] **Step 1.8: Commit**

```bash
git add pkg/io/protocol/game/server/prot.go modules/world/player.go modules/world/interaction.go modules/world/handler_opnpc.go modules/world/handlers_game.go modules/world/tick.go
git commit --no-gpg-sign -m "feat(world): interaction state machine + OpNpc1-5 handlers

Right-click NPC in the client now routes through opcode 194/8/27/113/100
to handleOpNpc -> SetInteraction(Engine, npc, op). A new processInteractions
tick phase (between processPathing and processNpcs) steps the state machine
each tick: in-range targets get SetFaceEntity + interacted=true; out-of-
range targets get a fresh waypoint via pathToMoveClick. Level mismatch,
dead/invalid NPCs, delayed players, and missing NpcType.Op entries all
emit UnsetMapFlag and no state change (matches TS semantics).

No damage semantics - this is the hook point for RuneScript's op_npc
triggers when the scripting engine lands. See 6b/RuneScript spec for
the follow-up."
```

---

## Task 2: Tests

**Files:**
- Create: `modules/world/interaction_test.go`
- Create: `modules/world/handler_opnpc_test.go`
- Create: `modules/world/tick_interactions_test.go`

- [ ] **Step 2.1: Write interaction_test.go, handler_opnpc_test.go, tick_interactions_test.go**

Full test bodies per the spec's "Testing" section. Use `newTestPlayer`, `newTestServer`, existing `drainConn` helper, and `setupNpc` pattern from `player_npc_test.go`.

Key fixture mechanics:
- Initialise `s.zoneMap = zone.NewZoneMap()` + `s.zonesTracking = map[*zone.Zone]struct{}{}` if any test exercises paths that touch zones.
- Initialise `s.npcs` array and `s.grid` for tests that need NPC lookup.
- Use `s.cfg.NodeClientRoutefinder = true` to avoid invoking the real pathfinder (makes `pathToTarget` a direct-step path).
- For `TestProcessInteractionDifferentLevelClears`, seed a Player at level 0 and a target at level 1, then assert UnsetMapFlag byte on the wire.
- For handler tests, seed `s.npcs[slot] = npc` and `s.npcLoop = append(s.npcLoop, npc)`; create NPC with `objtype.NpcType{Op: []string{"Attack"}}` so Op[0] is valid.

- [ ] **Step 2.2: Run tests**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestSetInteraction|TestClearInteraction|TestProcessInteraction|TestInOperableDistance|TestHandleOpNpc|TestProcessInteractionsRun' -v
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...
```
Expected: all green.

- [ ] **Step 2.3: Commit**

```bash
git add modules/world/interaction_test.go modules/world/handler_opnpc_test.go modules/world/tick_interactions_test.go
git commit --no-gpg-sign -m "test(world): cover interaction state machine + OpNpc handlers

16 tests across 3 files: interaction state (Set/Clear, range, level, delay,
adjacency table), handlers (valid + all rejection paths for op1..5),
tick-phase integration."
```

---

## Final Verification

- [ ] `go test -race ./...` — PASS
- [ ] `go vet ./...` — clean
- [ ] `grep 'gameHandlers\[194\]' modules/world/handlers_game.go` — non-empty
- [ ] `grep 'OpUnsetMapFlag' pkg/io/protocol/game/server/prot.go` — non-empty
- [ ] `grep 'processInteractions' modules/world/tick.go` — two matches (def + call)

## Spec Coverage

| Spec item | Task |
|---|---|
| `OpUnsetMapFlag` Op entry | 1 |
| `sendUnsetMapFlag` | 1 |
| `InteractionKind` enum + `interactionKind` field | 1 |
| `SetInteraction` / `ClearInteraction` / `ClearPendingAction` | 1 |
| `processInteraction` / `inOperableDistance` / `pathToTarget` | 1 |
| `handleOpNpc` shared + 5 wrappers | 1 |
| Handler registrations | 1 |
| `processInteractions` tick phase | 1 |
| State machine tests | 2 |
| Handler tests | 2 |
| Tick-phase integration test | 2 |
| Acceptance criteria | Final |
