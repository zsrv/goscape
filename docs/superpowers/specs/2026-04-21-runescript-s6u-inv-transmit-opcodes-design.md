# S6u — `inv_transmit` / `inv_stoptransmit` Script Opcodes Design

> **Sub-spec context:** Nineteenth runescript sub-spec. Wires S6p-2's `Player.invListenOnCom` / `invStopListenOnCom` runtime API as script opcodes. Gives RuneScript the ability to register/unregister UI inventory listeners — the first script consumer of the S6p API.

> **TS-faithfulness gate:** Matches TS `InvOps.ts` `inv_transmit` / `inv_stoptransmit` opcodes. **Zero new deviations.**

> **Scope boundary:** `invother_transmit` (OpInvOtherTransmit, 4332) is deferred because it requires `$active_player2` (secondary-player-pointer) infrastructure that doesn't exist in goscape yet. This is a scope boundary, not a deviation — the TS opcode works; we just lack the prerequisite to wire it. Opcode stays reserved in the enum and disasm; no handler.

## 1. Goal

Implement two RuneScript opcodes that let scripts register / unregister inventory listeners on the active player:

- `inv_transmit(com, inv)` — register listener for world-shared inventory `inv` on component `com`. Source = -1 (world-shared).
- `inv_stoptransmit(com)` — unregister listener at component `com`.

Both delegate to S6p-2's `Player.invListenOnCom` / `invStopListenOnCom` via two new `ActivePlayer` interface methods.

Observable gain: S6p-2's API is no longer dead code — scripts that open UI modals can now wire listeners at the same tick. The S6m-D3 / S6o-D3 validation gates (OpLocU, OpNpcU) now have a way to be satisfied at runtime, not just in test fixtures.

## 2. Architecture

Four layers touched:

1. **`pkg/script/active.go`** — add 2 interface methods to `ActivePlayer`
2. **`pkg/script/handlers_player.go`** — add 2 handler functions
3. **`pkg/script/handlers.go`** — register the 2 handlers in the opcode→handler map
4. **`modules/world/player_script.go`** — implement the interface methods on `*Player` (thin wrappers around existing unexported `invListenOnCom` / `invStopListenOnCom`)

### 2.1 TS opcode semantics

From TS InvOps.ts:

```typescript
[ScriptOpcode.INV_TRANSMIT]: (state) => {
    const inv = state.popInt();
    const com = state.popInt();
    state.activePlayer.invListenOnCom(inv, com, -1);
},

[ScriptOpcode.INV_STOPTRANSMIT]: (state) => {
    const com = state.popInt();
    state.activePlayer.invStopListenOnCom(com);
},
```

Operand order: script compiler pushes args in declaration order, handler pops LIFO:
- `inv_transmit(com, inv)` → push com, push inv → pop inv first, com second.
- `inv_stoptransmit(com)` → push com → pop com.

### 2.2 Scope boundary: `invother_transmit`

TS signature: `invother_transmit(com, inv)` operating on `$active_player2` (secondary player pointer set by e.g. trade/shop initiator scripts). Goscape has no secondary-player-pointer yet — the ScriptState carries a single `Self` (`ActivePlayer`) pointer only.

This sub-spec does NOT wire `OpInvOtherTransmit`. The opcode enum + disasm remain as-is (already present from earlier sub-specs). A future sub-spec that adds `$active_player2` infrastructure will wire it.

## 3. File Map

| File | Action | Sites | Task |
|---|---|---|---|
| `pkg/script/active.go` | Modify | Add `InvListenOnCom` + `InvStopListenOnCom` interface methods | 1 |
| `modules/world/player_script.go` | Modify | Add `(*Player).InvListenOnCom` + `.InvStopListenOnCom` methods (exported wrappers) | 1 |
| `pkg/script/handlers_player.go` | Modify | Add `handleInvTransmit` + `handleInvStopTransmit` | 1 |
| `pkg/script/handlers.go` | Modify | Register both handlers | 1 |
| `pkg/script/handlers_player_test.go` | Modify | 4 new tests (2 per opcode: formula + error path) | 1 |

## 4. Component Details

### 4.1 ActivePlayer interface extension

Add after the S6p-era methods (or at the natural grouping point for inv-related methods):

```go
// InvListenOnCom registers an inventory listener at UI component id
// `com` tracking inv type `invType`. `source == -1` means the
// world-shared inventory (Server.invs[invType]); source >= 0 means
// the player at that server slot (Server.players[source].invs[invType]).
// If a listener already exists at com, it is replaced (FirstSeen
// resets to true). Safe to call with invListeners map still nil —
// implementations must lazy-init.
InvListenOnCom(invType, com, source int)

// InvStopListenOnCom unregisters the listener at UI component id com.
// No-op when no listener exists there. Must be safe when the
// invListeners map is nil.
InvStopListenOnCom(com int)
```

### 4.2 Player wrapper methods

In `modules/world/player_script.go`:

```go
// InvListenOnCom implements script.ActivePlayer. Thin wrapper
// delegating to the internal unexported method landed in S6p-2.
func (p *Player) InvListenOnCom(invType, com, source int) {
	p.invListenOnCom(invType, com, source)
}

// InvStopListenOnCom implements script.ActivePlayer.
func (p *Player) InvStopListenOnCom(com int) {
	p.invStopListenOnCom(com)
}
```

### 4.3 Handlers

In `pkg/script/handlers_player.go` (alongside other stat/player ops; could also live in a new `handlers_inv.go` but the inv ops are already scattered across `handlers_inv.go` from prior sub-specs — **verify the file name before writing**). If `handlers_inv.go` exists, write there; if not, add to `handlers_player.go`.

```go
// handleInvTransmit implements INV_TRANSMIT. Registers a listener on
// the active player for UI component `com` tracking world-shared
// inventory type `invType` (source=-1).
// TS: InvOps.ts INV_TRANSMIT — popInt(inv), popInt(com),
// activePlayer.invListenOnCom(inv, com, -1).
func handleInvTransmit(s *ScriptState) error {
	if err := requireActivePlayer(s, "INV_TRANSMIT"); err != nil {
		return err
	}
	invType := s.PopInt()
	com := s.PopInt()
	s.Self.InvListenOnCom(invType, com, -1)
	return nil
}

// handleInvStopTransmit implements INV_STOPTRANSMIT. Unregisters the
// listener at UI component `com`. Safe when no listener exists there.
// TS: InvOps.ts INV_STOPTRANSMIT — popInt(com),
// activePlayer.invStopListenOnCom(com).
func handleInvStopTransmit(s *ScriptState) error {
	if err := requireActivePlayer(s, "INV_STOPTRANSMIT"); err != nil {
		return err
	}
	com := s.PopInt()
	s.Self.InvStopListenOnCom(com)
	return nil
}
```

### 4.4 Handler registration

In `pkg/script/handlers.go`, the opcode→handler map. Find other `OpInv...: handle...` entries and add:

```go
OpInvTransmit:     handleInvTransmit,
OpInvStopTransmit: handleInvStopTransmit,
```

(Keep alphabetical ordering if the surrounding entries follow it.)

## 5. Test Plan

4 new tests in `pkg/script/handlers_player_test.go`. Use the existing test pattern from `TestStatBoostClampsToBasePlusBoost` / `TestStatDrainUsesCurrentNotBase`:

1. **`TestInvTransmitRegistersListener`** — run a script that pushes (com, inv) then OpInvTransmit; assert mock ActivePlayer recorded `InvListenOnCom(inv, com, -1)`.

2. **`TestInvTransmitNoActivePlayerErrors`** — run with `PtrActivePlayer` bit clear; assert `INV_TRANSMIT: no active player` error.

3. **`TestInvStopTransmitUnregistersListener`** — run a script that pushes com then OpInvStopTransmit; assert mock ActivePlayer recorded `InvStopListenOnCom(com)`.

4. **`TestInvStopTransmitNoActivePlayerErrors`** — similar error-path test.

The mock ActivePlayer already exists (used by other stat opcode tests); extend it with the 2 new methods that record their arg tuples.

## 6. Task Split

**Single task.** Small, tightly-coupled changes across 5 files (interface, Player wrapper, 2 handlers, registration, tests).

Commit: `feat(script,world): inv_transmit / inv_stoptransmit opcodes — wire S6p-2 API (S6u)`

## 7. Deviations

**Zero new deviations.** Zero closures (S6p's S6m-D3 / S6o-D3 already closed). `invother_transmit` is a scope boundary — filed as S6u-SB1 for tracker clarity:

| ID | Status | Reason |
|---|---|---|
| S6u-SB1 (scope boundary) | Deferred | `invother_transmit` needs `$active_player2` infra not yet in goscape |

## 8. Scope

- Impl: ~35 LOC (2 interface methods + 2 Player wrappers + 2 handlers + 2 registration lines)
- Tests: ~80 LOC (4 tests + mock ActivePlayer extension)
- 1 commit
- Total: ~115 LOC
