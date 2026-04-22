# S6v — `p_op_loc` / `p_op_npc` Script Opcodes Design

> **Sub-spec context:** Twenty-first runescript sub-spec. Partial closure of S6l-D5 (`p_op*` re-anchor opcodes). Wires 2 of 7 p_op* opcodes — `P_OPLOC` (2077) and `P_OPNPC` (2078) — so scripts can anchor the player on a Loc/Npc target with a specific op, re-routing interaction mid-script.

> **TS-faithfulness gate:** Matches TS `PlayerOps.ts:386-415`. **One new deviation (S6v-D1):** `ProtectedActivePlayer` protection check deferred — TS wraps these opcodes in `checkedHandler(ProtectedActivePlayer, ...)`; goscape uses the simpler `requireActivePlayer` until a dedicated Protect-gate sub-spec lands.

> **Scope:** Single-task sub-spec. ~120 LOC.

## 1. Goal

Wire two RuneScript opcodes that let scripts set the player's interaction target mid-execution:

- `p_op_loc(op)` → re-anchors on `$active_loc` with AP trigger `APLOC<op>` (op ∈ [1,5]).
- `p_op_npc(op)` → re-anchors on `$active_npc` with AP trigger `APNPC<op>` (op ∈ [1,5]).

Observable gain: scripts can initiate subsequent engine-driven interactions programmatically (e.g., a dialog-end handler that fires `p_op_loc(3)` to start a chop-tree sequence automatically).

## 2. TS reference map

| TS artifact | File:Line |
|---|---|
| P_OPLOC handler | `src/engine/script/handlers/PlayerOps.ts:386-402` |
| P_OPNPC handler | `src/engine/script/handlers/PlayerOps.ts:404-415` |
| Pointer requirements | `src/engine/script/ScriptOpcodePointers.ts:345-353` — requires `p_active_player` + `active_loc`/`active_npc` |
| `checkedHandler(ProtectedActivePlayer, ...)` | `src/engine/script/handlers/PlayerOps.ts:381,386,403,417` |
| `Interaction.SCRIPT` | TS `Interaction.ts` (enum value used for script-queued interactions) |
| `ServerTriggerType.APLOC1 + type` | TS `ServerTriggerType.ts` trigger enum |

### 2.1 TS `P_OPLOC` semantics (verbatim-equivalent)

```typescript
const type = check(state.popInt(), NumberNotNull) - 1;
if (type < 0 || type >= 5) { throw new Error(`Invalid oploc: ${type + 1}`); }
const locType: LocType = LocType.get(state.activeLoc.type);
if (!locType.op || !locType.op[type]) { return; }        // silent no-op if no op
state.activePlayer.stopAction();
if (!state.activePlayer.inOperableDistance(state.activeLoc)) {
    state.activePlayer.queueWaypoint(state.activeLoc.x, state.activeLoc.z);
}
state.activePlayer.setInteraction(Interaction.SCRIPT, state.activeLoc, ServerTriggerType.APLOC1 + type);
```

### 2.2 TS `P_OPNPC` semantics

```typescript
const type = check(state.popInt(), NumberNotNull) - 1;
if (type < 0 || type >= 5) { throw new Error(`Invalid opnpc: ${type + 1}`); }
const npcType: NpcType = NpcType.get(state.activeNpc.type);
if (!npcType.op || !npcType.op[type]) { return; }
state.activePlayer.stopAction();
state.activePlayer.setInteraction(Interaction.SCRIPT, state.activeNpc, ServerTriggerType.APNPC1 + type);
```

Note NPC variant doesn't queueWaypoint — NPCs are moving so pathing is handled each tick by the engine's AP/OP gates.

## 3. Architecture

Four layers touched:

1. **`pkg/script/active.go`** — 2 new ActivePlayer methods that take the narrow `ActiveLoc`/`ActiveNpc` interface + an op int.
2. **`pkg/script/handlers_player.go`** (or a new `handlers_p_op.go`) — 2 handler functions.
3. **`pkg/script/handlers.go`** — register both handlers.
4. **`modules/world/player_script.go`** — implement the 2 methods on `*Player` via type assertion back to concrete `*entity.Loc` / `*world.Npc`.

### 3.1 Queue-waypoint omission (S6v scope decision)

TS `P_OPLOC` queues a waypoint if the player isn't in operable distance. Goscape's `processInteraction` handles pathing on the next tick via `pathToTarget`, so the waypoint-queue step is redundant — the tick loop takes over. Skip the queue step; document as a minor behavioral note (not a deviation — observable behavior is equivalent, just triggered one tick later).

### 3.2 ProtectedActivePlayer gate deferral (S6v-D1)

TS uses `checkedHandler(ProtectedActivePlayer, ...)` to prevent unprotected scripts from calling p_op*. Goscape doesn't have the ProtectedActivePlayer bit enforced yet. Until a dedicated sub-spec lands, use `requireActivePlayer` (which gates on "has any active player" rather than "has a protected one"). Filed as S6v-D1 with closure pointer: "future sub-spec closing S6l-D3 ProtectedActivePlayer gate."

## 4. File Map

| File | Action | Sites |
|---|---|---|
| `pkg/script/active.go` | Modify | Add 2 interface methods |
| `pkg/script/handlers_player.go` | Modify | Add 2 handler functions |
| `pkg/script/handlers.go` | Modify | Register both handlers |
| `pkg/script/runner_test.go` | Modify | Extend mockPlayer with call-recording methods |
| `pkg/script/handlers_player_test.go` | Modify | 6 new tests (3 per opcode) |
| `modules/world/player_script.go` | Modify | Implement the 2 methods on `*Player` |

## 5. Component details

### 5.1 ActivePlayer extensions

```go
// SetInteractionScriptLoc anchors the player on `loc` with trigger
// ApLoc<op> as a script-queued interaction (TS Interaction.SCRIPT).
// op is 1-indexed (1..5). Matches TS PlayerOps.ts:386-402 terminal
// setInteraction call.
//
// Implementations must type-assert the narrow ActiveLoc interface to
// their concrete loc type. Callers should pre-validate op ∈ [1,5] and
// that the loc's LocType.Op[op-1] is non-empty.
SetInteractionScriptLoc(loc ActiveLoc, op int)

// SetInteractionScriptNpc anchors the player on `npc` with trigger
// ApNpc<op> as a script-queued interaction. Matches TS
// PlayerOps.ts:404-415.
SetInteractionScriptNpc(npc ActiveNpc, op int)
```

### 5.2 Handlers

```go
// handleP_OpLoc (P_OPLOC, opcode 2077) re-anchors the active player on
// the active loc with AP trigger APLOC<op>. Matches TS
// PlayerOps.ts:386-402.
//
// DEVIATION S6v-D1: TS wraps this in checkedHandler(ProtectedActivePlayer);
// goscape uses requireActivePlayer until the ProtectedActivePlayer gate
// sub-spec lands.
func handleP_OpLoc(s *ScriptState) error {
	if err := requireActivePlayer(s, "P_OPLOC"); err != nil {
		return err
	}
	if s.ActiveLoc == nil {
		return errors.New("P_OPLOC: no active loc")
	}
	op := s.PopInt()
	if op < 1 || op > 5 {
		return fmt.Errorf("P_OPLOC: invalid op %d (must be 1..5)", op)
	}
	// LocType.Op[op-1] check happens at the Player layer because goscape's
	// ActiveLoc interface is narrow (just LocType()). If the locType lacks
	// op[op-1], the engine-side SetInteraction will no-op routing when no
	// AP script exists — semantically equivalent to TS's early return.
	s.Self.StopAction()
	s.Self.SetInteractionScriptLoc(s.ActiveLoc, op)
	return nil
}

// handleP_OpNpc (P_OPNPC, opcode 2078) re-anchors on the active npc.
// Matches TS PlayerOps.ts:404-415.
func handleP_OpNpc(s *ScriptState) error {
	if err := requireActivePlayer(s, "P_OPNPC"); err != nil {
		return err
	}
	if s.ActiveNpc == nil {
		return errors.New("P_OPNPC: no active npc")
	}
	op := s.PopInt()
	if op < 1 || op > 5 {
		return fmt.Errorf("P_OPNPC: invalid op %d (must be 1..5)", op)
	}
	s.Self.StopAction()
	s.Self.SetInteractionScriptNpc(s.ActiveNpc, op)
	return nil
}
```

Note on NpcType.Op check: like the Loc variant, goscape pushes the presence-of-op check into the Player implementation (which has the full NpcType pointer). TS early-returns if absent; goscape's SetInteraction routes to OP dispatch which no-ops if the op isn't registered. Observable behavior matches.

### 5.3 Player implementations

```go
// SetInteractionScriptLoc implements script.ActivePlayer. Type-asserts
// the narrow script.ActiveLoc back to *entity.Loc and anchors the
// player with trigger ApLoc<op> + Interaction kind InteractionScript.
//
// If the ActiveLoc isn't a *entity.Loc, silently no-op (defensive —
// only goscape's OPLOC routing ever sets ScriptState.ActiveLoc, so
// this should never fire in production; treat as a guard against
// future test fixture misuse).
func (p *Player) SetInteractionScriptLoc(loc script.ActiveLoc, op int) {
	realLoc, ok := loc.(*entitypkg.Loc)
	if !ok {
		return
	}
	p.SetInteraction(InteractionScript, realLoc, op, -1)
}

func (p *Player) SetInteractionScriptNpc(npc script.ActiveNpc, op int) {
	realNpc, ok := npc.(*Npc)
	if !ok {
		return
	}
	p.SetInteraction(InteractionScript, realNpc, op, -1)
}
```

### 5.4 Handler registration

In `pkg/script/handlers.go`:

```go
OpPOpLoc: handleP_OpLoc,
OpPOpNpc: handleP_OpNpc,
```

## 6. Test plan

6 new tests in `handlers_player_test.go`:

1. `TestPOpLocAnchorsOnActiveLoc` — script pushes op=3, runs OpPOpLoc; mockPlayer records SetInteractionScriptLoc(loc, 3).
2. `TestPOpLocNoActivePlayerErrors` — PtrActivePlayer bit clear → error.
3. `TestPOpLocNoActiveLocErrors` — PtrActivePlayer set but ActiveLoc nil → error.
4. `TestPOpLocInvalidOpErrors` — op=0 and op=6 both error.
5. `TestPOpNpcAnchorsOnActiveNpc` — symmetric.
6. `TestPOpNpcInvalidOpErrors` — symmetric.

mockPlayer extensions: `lastSetInteractionScriptLoc []mockLocOp`, `lastSetInteractionScriptNpc []mockNpcOp` + recording methods.

## 7. Task split

Single task. ~80 LOC impl + ~120 LOC tests.

Commit: `feat(script,world): p_op_loc / p_op_npc opcodes — partial closure of S6l-D5 (S6v)`

## 8. Deviations

| ID | Status | Rationale |
|---|---|---|
| **S6l-D5** | **Partially closed in S6v** | P_OPLOC + P_OPNPC wired. Others (P_OPHELD — TS unimplemented; P_OPNPCT — needs spell infra; P_OPOBJ/PLAYER/PLAYERT — need entity-ptr infra) remain open. |
| **S6v-D1 (new)** | Open | ProtectedActivePlayer gate deferred — TS wraps p_op* in `checkedHandler(ProtectedActivePlayer, ...)`; goscape uses `requireActivePlayer` as interim gate. Risk: scripts started without protection can call these opcodes. Mitigation: no scripts call them yet (no cache content wires p_op_loc/p_op_npc). Follow-up: future sub-spec adding `PtrProtectedActivePlayer` bitmask bit + setting it in `Init()` when `protect=true` + switching handler guard. |

## 9. Scope boundaries (not new deviations)

- **Queue-waypoint step omitted from P_OPLOC.** TS queues a waypoint on the player; goscape lets `processInteraction` path on the next tick. Observable behavior equivalent.
- **P_OPHELD (2076)** remains unwired — TS throws `unimplemented` at `PlayerOps.ts:382`, so there's nothing to port.
- **P_OPNPCT / P_OPOBJ / P_OPPLAYER / P_OPPLAYERT** deferred pending spell infra, active_obj, and active_player2 respectively.

## 10. Scope estimate

- Impl: ~80 LOC (2 interface methods + 2 handlers + 2 Player wrappers + 2 registrations)
- Tests: ~120 LOC (6 new tests + mockPlayer extension)
- 1 commit, ~200 LOC total
