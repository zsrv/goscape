# S6y — `invother_transmit` Script Opcode Design

> **Sub-spec context:** Twenty-fourth runescript sub-spec. Closes S6u-SB1 (which was misdiagnosed as needing `$active_player2` infrastructure; turns out to just be a 3-arg version of `inv_transmit`).

> **TS-faithfulness gate:** Matches TS `InvOps.ts` `INVOTHER_TRANSMIT`. **Zero new deviations.**

> **Scope:** Tiny single-task sub-spec. ~25 LOC impl + ~30 LOC tests.

## 1. Goal

Wire the `OpInvOtherTransmit` opcode (4332) as the 3-arg variant of `inv_transmit` where the listener `source` is an arbitrary player slot (uid) rather than hardcoded `-1` (world-shared).

## 2. TS reference

`src/engine/script/handlers/InvOps.ts` (around the `INVOTHER_TRANSMIT` entry):

```typescript
[ScriptOpcode.INVOTHER_TRANSMIT]: checkedHandler(ActivePlayer, state => {
    const [uid, inv, com] = state.popInts(3);

    check(uid, NumberNotNull);
    const invType: InvType = check(inv, InvTypeValid);
    check(com, NumberNotNull);

    state.activePlayer.invListenOnCom(invType.id, com, uid);
}),
```

Pop order: `popInts(3)` returns `[uid, inv, com]` — TS popInts returns the 3 values in PUSH order (not reverse), so uid was pushed first, com last (top of stack).

Calls the same `invListenOnCom(invType, com, source)` method that `inv_transmit` uses. The only difference is `source = uid` (the popped int) instead of the hardcoded `-1` in `inv_transmit`.

## 3. Architecture

Single handler function in `pkg/script/handlers_inv.go` (next to `handleInvTransmit`, `handleInvStopTransmit`). Registered in `pkg/script/handlers.go`. No interface changes — uses the existing `ActivePlayer.InvListenOnCom(invType, com, source)` method from S6u.

Goscape's `PopInt()` returns top-of-stack, so 3 sequential pops gives `[com, inv, uid]` in that order (reverse of push order). This matches the TS `popInts(3)` destructure to `[uid, inv, com]` because TS destructures from push order while Go pops from top.

## 4. File map

| File | Action |
|---|---|
| `pkg/script/handlers_inv.go` | Add `handleInvOtherTransmit` |
| `pkg/script/handlers.go` | Register `OpInvOtherTransmit` → `handleInvOtherTransmit` |
| `pkg/script/handlers_inv_test.go` | Add 2 tests (happy path + no-active-player error) |

## 5. Handler

```go
// handleInvOtherTransmit implements INVOTHER_TRANSMIT (opcode 4332).
// 3-arg variant of INV_TRANSMIT: registers a listener on the active
// player at UI component `com` tracking inv type `invType` with source
// = `uid` (another player's server slot). Used by trade/shop/bank-view
// flows where the viewer watches another player's inventory.
//
// TS: InvOps.ts INVOTHER_TRANSMIT — popInts(3) → [uid, inv, com];
// activePlayer.invListenOnCom(invType.id, com, uid).
func handleInvOtherTransmit(s *ScriptState) error {
	if err := requireActivePlayer(s, "INVOTHER_TRANSMIT"); err != nil {
		return err
	}
	com := s.PopInt()
	invType := s.PopInt()
	uid := s.PopInt()
	s.Self.InvListenOnCom(invType, com, uid)
	return nil
}
```

## 6. Tests

```go
// TestInvOtherTransmitRegistersListenerWithUid — happy path.
func TestInvOtherTransmitRegistersListenerWithUid(t *testing.T) {
    mp := &mockPlayer{}
    sf := &ScriptFile{
        Name: "invother_transmit",
        Opcodes: []Opcode{
            OpPushConstantInt, // uid
            OpPushConstantInt, // inv
            OpPushConstantInt, // com (top)
            OpInvOtherTransmit,
            OpReturn,
        },
        IntOperands:      []int32{42, 93, 149, 0, 0},
        StringOperands:   []string{"", "", "", "", ""},
        InstructionCount: 5,
    }
    state := Init(sf, mp, false, nil, nil)
    if err := Execute(state); err != nil {
        t.Fatalf("Execute: %v", err)
    }
    if len(mp.lastInvListenOnCom) != 1 {
        t.Fatalf("expected 1 call, got %d", len(mp.lastInvListenOnCom))
    }
    got := mp.lastInvListenOnCom[0]
    if got.InvType != 93 || got.Com != 149 || got.Source != 42 {
        t.Errorf("args: got %+v, want {InvType:93, Com:149, Source:42}", got)
    }
}

// TestInvOtherTransmitNoActivePlayerErrors — requireActivePlayer gate.
func TestInvOtherTransmitNoActivePlayerErrors(t *testing.T) {
    sf := newSingleOp("invother_transmit_no_player", OpInvOtherTransmit)
    state := Init(sf, nil, false, nil, nil)
    state.PushInt(42)  // uid
    state.PushInt(93)  // inv
    state.PushInt(149) // com

    err := Execute(state)
    if err == nil || err.Error() != "INVOTHER_TRANSMIT: no active player" {
        t.Errorf("expected 'INVOTHER_TRANSMIT: no active player', got %v", err)
    }
}
```

## 7. Deviations

| ID | Status |
|---|---|
| **S6u-SB1** | **✅ CLOSED in S6y** — OpInvOtherTransmit wired as 3-arg opcode |

No new deviations. TS uses `checkedHandler(ActivePlayer)` (not ProtectedActivePlayer) — goscape's `requireActivePlayer` matches exactly.

## 8. Scope

- Impl: ~15 LOC
- Tests: ~40 LOC
- 1 commit
