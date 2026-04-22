# S6w — ProtectedActivePlayer Gate (closes S6v-D1) Design

> **Sub-spec context:** Twenty-second runescript sub-spec. Adds a `requireProtectedActivePlayer` helper and applies it to S6v's `P_OPLOC` / `P_OPNPC` handlers. Closes S6v-D1 directly. Partial closure of S6l-D3 — the broader migration of the other 6 TS-protected handlers (PDelay, PTeleport, PTeleJump, PStopAction, PClearPendingAction, PPauseButton) to the new gate is deferred to a follow-up sub-spec because those would require ~15 existing tests to update their `protect=false` fixture setups.

> **TS-faithfulness gate:** Matches TS `PlayerOps.ts`'s `checkedHandler(ProtectedActivePlayer, ...)` wrapper for the 2 p_op* opcodes. **Zero new deviations.**

> **Scope:** Single-task, ~40 LOC total.

## 1. Goal

Add a reusable `requireProtectedActivePlayer(s, opName) error` guard. Apply it to `handleP_OpLoc` and `handleP_OpNpc`, replacing their current `requireActivePlayer` gate. Unprotected scripts calling p_op_* now get an error — matching TS `checkedHandler(ProtectedActivePlayer, ...)` semantics.

## 2. TS reference

- `src/engine/script/handlers/PlayerOps.ts:386,403` — `checkedHandler(ProtectedActivePlayer, state => {...})` wraps both P_OPLOC and P_OPNPC.
- `src/engine/entity/Player.ts:2095-2103` — `runScript` sets `ProtectedActivePlayer` pointer + `this.protect = true` when invoking with `protect=true`. The pointer is on the script, not on the player flag directly — but for gate-purposes, checking "did this script start with protect=true" maps to goscape's existing `ScriptState.Protect` field.
- `src/engine/script/ScriptPointer.ts:10` — `ProtectedActivePlayer` enum value.

## 3. Architecture

1. **Add helper** in `pkg/script/handlers_player.go`:
   ```go
   func requireProtectedActivePlayer(s *ScriptState, op string) error {
       if err := requireActivePlayer(s, op); err != nil {
           return err
       }
       if !s.Protect {
           return errors.New(op + ": script not protected")
       }
       return nil
   }
   ```
   Chains through to `requireActivePlayer` first (so the existing "no active player" error message still fires for scripts without `Self`), then adds the protect check.

2. **Update 2 handlers**: swap `requireActivePlayer(s, "P_OP...")` → `requireProtectedActivePlayer(s, "P_OP...")` in `handleP_OpLoc` and `handleP_OpNpc`.

3. **Update existing S6v tests** to use `Init(sf, mp, true, nil, nil)` (protect=true). The 4 current positive-path S6v tests were written with protect=false.

4. **Add 2 new tests**: `TestPOpLocUnprotectedRejected`, `TestPOpNpcUnprotectedRejected` — script run with protect=false gets `P_OPLOC: script not protected` error.

## 4. File map

| File | Action |
|---|---|
| `pkg/script/handlers_player.go` | Add `requireProtectedActivePlayer`; swap the 2 handler gates |
| `pkg/script/handlers_player_test.go` | 4 existing S6v tests → protect=true in `Init`; add 2 new unprotected-rejected tests |
| `modules/world/handler_oploc.go` deviation table | N/A — deviation was filed in S6v spec, not in code comments that need updating beyond the handler docstrings |

Handler docstrings for `handleP_OpLoc` and `handleP_OpNpc` currently say:

> DEVIATION S6v-D1: TS wraps this in checkedHandler(ProtectedActivePlayer); goscape uses requireActivePlayer until a ProtectedActivePlayer gate sub-spec lands.

Replace with:

> S6v-D1 closed in S6w: uses requireProtectedActivePlayer to gate execution on ScriptState.Protect.

## 5. Test plan

**Existing tests updated (protect=false → protect=true):**
- `TestPOpLocAnchorsOnActiveLoc`
- `TestPOpLocNoActiveLocErrors`
- `TestPOpLocInvalidOpErrors`
- `TestPOpNpcAnchorsOnActiveNpc`
- `TestPOpNpcInvalidOpErrors`

Keep `TestPOpLocNoActivePlayerErrors` unchanged — it passes `nil` as Self and already asserts the `requireActivePlayer`-style error, which the chained helper still produces. `TestPOpLocNoActiveLocErrors`: needs protect=true so it reaches the no-active-loc check.

**New tests (2):**

```go
// TestPOpLocUnprotectedRejected verifies a script started without
// protection gets an error from P_OPLOC — matches TS
// checkedHandler(ProtectedActivePlayer, ...) semantics.
func TestPOpLocUnprotectedRejected(t *testing.T) {
    mp := &mockPlayer{}
    loc := &mockActiveLoc{locType: 42}
    sf := newSingleOp("p_op_loc_unprotected", OpPOpLoc)
    state := Init(sf, mp, false, nil, nil) // protect=false
    state.ActiveLoc = loc
    state.Pointers |= PtrActiveLoc
    state.PushInt(3)

    err := Execute(state)
    if err == nil || err.Error() != "P_OPLOC: script not protected" {
        t.Errorf("expected 'P_OPLOC: script not protected', got %v", err)
    }
}

// TestPOpNpcUnprotectedRejected — symmetric.
func TestPOpNpcUnprotectedRejected(t *testing.T) { ... }
```

## 6. Task split

**Single task.** ~40 LOC.

Commit: `feat(script): requireProtectedActivePlayer gate for P_OPLOC/P_OPNPC — closes S6v-D1 (S6w)`

## 7. Deviations

| ID | Status |
|---|---|
| **S6v-D1** | **✅ CLOSED in S6w** — P_OPLOC / P_OPNPC now gate on ScriptState.Protect |
| **S6l-D3** | Still open (partial closure) — 6 other TS-protected opcodes (PDelay, PTeleport, PTeleJump, PStopAction, PClearPendingAction, PPauseButton) still use requireActivePlayer. Follow-up sub-spec can batch-update. |

No new deviations.
