# Sub-spec 6a: Interaction State Machine — Design

**Status:** Draft → ready for plan
**Scope:** Wire the client's right-click-NPC flow (opcodes 194 / 8 / 27 / 113 / 100) to a per-player interaction state machine that walks the player toward the target and faces it when adjacent. No damage semantics.
**Out of scope:** Combat damage tick (RuneScript will drive this); NPC retaliation / aggro; loot drops; player-vs-player; OpObj / OpLoc / OpPlayer opcodes.

---

## Goal

Lay the engine-side hook point every downstream NPC-script will fire into. After this ships, right-clicking an NPC and picking an option walks the player to the NPC and faces it — no more, no less. RuneScript integration later replaces the "face and return" stub with script-driven behaviour.

## Architecture

One new outbound opcode (`UnsetMapFlag`), two new files in `modules/world/` (interaction state machine and OpNpc handlers), one new tick phase `processInteractions` running after `processPathing`. Zero new packages.

```
pkg/io/protocol/game/server/prot.go      + OpUnsetMapFlag (19, 0)
modules/world/interaction.go  (new)      InteractionKind + SetInteraction / ClearInteraction
                                         / ClearPendingAction / processInteraction
                                         / inOperableDistance / pathToTarget
                                         / sendUnsetMapFlag
modules/world/handler_opnpc.go  (new)    handleOpNpc + 5 wrappers (op1..op5)
modules/world/handlers_game.go           gameHandlers[194/8/27/113/100] registration
modules/world/tick.go                    + processInteractions phase
modules/world/player.go                  + interactionKind field
```

## Components

### 1. Outer opcode — `pkg/io/protocol/game/server/prot.go`

```go
OpUnsetMapFlag = Op{Opcode: 19, PayloadSize: 0}
```

Tells the client to clear its pending map-click path indicator when the server rejects an interaction.

### 2. Interaction state machine — `modules/world/interaction.go`

```go
package world

import (
    "github.com/zsrv/goscape/pkg/coordgrid"
    gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
)

// InteractionKind distinguishes engine-triggered (right-click, walk-to,
// op_npc) from script-queued interactions. For sub-spec 6a only
// InteractionEngine exists; InteractionScript is reserved for the
// RuneScript integration that lands in a later sub-spec.
type InteractionKind int

const (
    InteractionEngine InteractionKind = iota
    InteractionScript
)

// sendUnsetMapFlag clears the client's pending map-click indicator.
func sendUnsetMapFlag(p *Player) {
    p.writeOut(gameserver.OpUnsetMapFlag, nil)
}

// SetInteraction anchors the player's tick-loop interaction state machine
// on a target entity. Resets per-interaction flags.
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
// is set. For 6a it's an alias for ClearInteraction; hooks for queued
// actions (e.g., pending item-use) land with the action-queue sub-spec.
func (p *Player) ClearPendingAction() {
    p.ClearInteraction()
}

// processInteraction runs once per tick per player after pathing has
// stepped the player. Behaviour:
//   - No target: no-op.
//   - Delayed player: no-op (prevents cheese during death/transitions).
//   - Target on different level: clear + UnsetMapFlag.
//   - In operable distance (Chebyshev 1): face target, mark interacted.
//   - Out of range: set waypoint toward target (once per tick).
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

// inOperableDistance is Chebyshev distance == 1 between (px,pz) and
// (tx,tz). Adjacent (including diagonals) = operable for 1×1 targets.
// Same-tile returns false (standing inside the target isn't "operable").
// Multi-tile targets + strict-adjacency land with real combat.
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

// pathToTarget sets a waypoint to (tx, tz). Reuses the existing
// pathToMoveClick so pathfinding (or direct-step mode) applies uniformly.
func (p *Player) pathToTarget(tx, tz int) {
    packed := []int{coordgrid.PackCoord(p.level, tx, tz)}
    needsFinding := !p.client.server.cfg.NodeClientRoutefinder
    p.pathToMoveClick(packed, needsFinding)
}
```

### 3. `Player` field — `modules/world/player.go`

Add to the existing interaction block (near `target`, `targetOp`):

```go
interactionKind InteractionKind
```

### 4. OpNpc handlers — `modules/world/handler_opnpc.go`

```go
package world

import (
    "github.com/zsrv/goscape/pkg/io/packet"
)

// handleOpNpc is the shared implementation for OPNPC1..OPNPC5. op is 1..5.
// Payload is p2(slot). Rejection paths emit UnsetMapFlag; success calls
// SetInteraction.
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

    // NpcType.Op[op-1] must be a non-empty, non-"hidden" string. RuneScript
    // will later replace this with trigger-existence lookup.
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

### 5. Handler registrations — `modules/world/handlers_game.go`

In the existing `init()`:

```go
gameHandlers[194] = handleOpNpc1 // OPNPC1
gameHandlers[8]   = handleOpNpc2 // OPNPC2
gameHandlers[27]  = handleOpNpc3 // OPNPC3
gameHandlers[113] = handleOpNpc4 // OPNPC4
gameHandlers[100] = handleOpNpc5 // OPNPC5
```

### 6. Tick phase — `modules/world/tick.go`

Insert after `processPathing`, before `processNpcs`:

```go
s.processClientsIn()
s.processPathing()
s.processInteractions()   // ← NEW
s.processNpcs()
...
```

Implementation:

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

## Data Flow

```
client right-clicks NPC → "Attack" (or op 2..5)
    │
    ▼
opcode 194/8/27/113/100 with payload = p2(slot)
    │
    ▼
gameHandlers[op] → handleOpNpc(p, payload, opN)
    ├─ delayed? → UnsetMapFlag
    ├─ bad slot / dead / missing NpcType.Op → UnsetMapFlag
    └─ ClearPendingAction → SetInteraction(Engine, npc, opN)
    │
    ▼  (later in the tick)
processPathing → resolveMovement (consumes prior waypoint if any)
processInteractions → processInteraction
    ├─ level mismatch → ClearInteraction + UnsetMapFlag
    ├─ in operable distance → SetFaceEntity(npc.nid), interacted=true
    └─ out of range → pathToTarget (sets next-tick waypoint)
    │
    ▼  (subsequent ticks repeat until interacted or cleared)
```

## Error Handling

Silent drops + `UnsetMapFlag` on any rejection. No error opcodes beyond `UnsetMapFlag`. Matches TS.

## Testing

### `modules/world/interaction_test.go`

- `TestSetInteractionPopulatesFields` — SetInteraction sets target/targetOp/interactionKind/apRange=10 and resets flags.
- `TestClearInteractionResetsAll` — Clear leaves target==nil, targetOp==-1, flags==false.
- `TestProcessInteractionNoTargetNoop` — nil target → no face mask, no waypoint, no bytes written.
- `TestProcessInteractionInRangeFacesTarget` — target adjacent → `p.faceEntity == npc.nid`, `p.masks & MaskFaceEntity != 0`, `p.interacted == true`.
- `TestProcessInteractionOutOfRangePaths` — target 5 tiles away → waypoint set (waypointIndex ≥ 0).
- `TestProcessInteractionDifferentLevelClears` — target on level 1, player on level 0 → target nil, UnsetMapFlag wire byte emitted.
- `TestProcessInteractionDelayedPlayerSkipped` — delayed → no writes, no state change.
- `TestInOperableDistanceTable` — cases: (0,0)-(0,0)=false, (0,0)-(1,0)=true, (0,0)-(1,1)=true, (0,0)-(2,0)=false, (0,0)-(0,2)=false.

### `modules/world/handler_opnpc_test.go`

- `TestHandleOpNpc1SetsInteraction` — valid slot/op → `p.target == npc`, `p.targetOp == 1`, no UnsetMapFlag emitted.
- `TestHandleOpNpc1InvalidSlotSendsUnsetMapFlag` — slot out of bounds → UnsetMapFlag emitted, `p.target == nil`.
- `TestHandleOpNpc1DeadNpcSendsUnsetMapFlag` — npc.dead=true → UnsetMapFlag, no interaction.
- `TestHandleOpNpc1HiddenOpSendsUnsetMapFlag` — NpcType.Op[0] == "hidden" → UnsetMapFlag.
- `TestHandleOpNpc2RoutesToOp2` — opcode 8 → `p.targetOp == 2`.
- `TestHandleOpNpcShortPayloadSendsUnsetMapFlag` — payload < 2 bytes → UnsetMapFlag.
- `TestHandleOpNpcDelayedPlayerRejected` — p.delayed + delayedUntil future → UnsetMapFlag, no state change.

### `modules/world/tick_interactions_test.go`

- `TestProcessInteractionsRunsPerPlayer` — 2 players both set up with adjacent targets → both get `p.interacted == true` after one `s.processInteractions()` call.

## Acceptance Criteria

1. `go test ./...` passes.
2. `go vet ./...` clean.
3. `go test -race ./...` passes.
4. `gameHandlers[194]`, `[8]`, `[27]`, `[113]`, `[100]` all non-nil.
5. `OpUnsetMapFlag` registered with (19, 0).
6. The 3-phase tick order is preserved: pathing → interactions → info.

## LOC Estimate

| File | LOC |
|---|---|
| `pkg/io/protocol/game/server/prot.go` | +2 |
| `modules/world/interaction.go` | ~135 |
| `modules/world/interaction_test.go` | ~180 |
| `modules/world/handler_opnpc.go` | ~80 |
| `modules/world/handler_opnpc_test.go` | ~160 |
| `modules/world/tick.go` | +15 |
| `modules/world/tick_interactions_test.go` | ~60 |
| `modules/world/handlers_game.go` | +5 |
| `modules/world/player.go` | +1 |
| **Total** | **~640** |

## Dependencies & Risks

- **No new external packages.**
- **No wire-format compatibility risk** — `UnsetMapFlag` verified against Java client `SERVERPROT_SIZES[19] == 0`.
- **Risk: `pathToMoveClick` pathfinding cost** — each processInteraction for an out-of-range target calls the pathfinder. For normal gameplay (few players, few out-of-range ticks) this is cheap. Profile if visible.
- **Adjacency semantics**: Chebyshev-1 includes diagonals; strict-adjacency (4-neighbor) is the TS rule for 1×1 NPCs. 6a accepts the simpler check; refine when real combat math lands.
- **TS `hasNpc` visibility check skipped**: our rsbuf doesn't expose an equivalent yet. Consequence: a laggy client that sees an NPC removed server-side but still displays it locally can send a right-click that gets accepted on the server. Low stakes; revisit if it matters.

## Deferred

- **6b (next)**: melee damage tick, NPC HP, death/respawn delay, player attack XP. Replaces the "face and return" stub in `processInteraction` with a damage-and-cadence loop.
- **6c**: NPC retaliation + aggro.
- **6d**: loot drop tables.
- **RuneScript**: replaces `processInteraction`'s stub entirely with script-trigger dispatch. `InteractionKind == InteractionScript` enum slot is reserved for this.
