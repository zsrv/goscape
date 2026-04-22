# S6x — Full ProtectedActivePlayer Gate Migration (closes S6l-D3) Design

> **Sub-spec context:** Twenty-third runescript sub-spec. Completes S6l-D3 closure by migrating the remaining 8 TS-protected handlers to `requireProtectedActivePlayer`. After S6w shipped the helper + gated 2 handlers (P_OPLOC/P_OPNPC), this sub-spec batch-migrates the rest.

> **TS-faithfulness gate:** Matches TS `PlayerOps.ts`/`DialogOps.ts`'s `checkedHandler(ProtectedActivePlayer, ...)` wrapper for all 8 opcodes. **Zero new deviations.**

> **Scope:** Single-task, ~200 LOC across mostly-mechanical changes.

## 1. Goal

Migrate 8 existing goscape handlers from `requireActivePlayer` (or inline `s.Pointers&PtrActivePlayer == 0` checks) to the `requireProtectedActivePlayer` helper shipped in S6w. Update existing tests to pass `protect=true` in their `Init` calls. Add unprotected-rejected tests for each.

Observable gain: unprotected scripts calling any of these opcodes now get a `"<OP>: script not protected"` error — matching TS `checkedHandler(ProtectedActivePlayer, ...)` semantics exactly.

## 2. TS reference

All 8 goscape opcodes verified as `checkedHandler(ProtectedActivePlayer, ...)` in TS:
- `src/engine/script/handlers/PlayerOps.ts` — P_DELAY, P_TELEPORT, P_TELEJUMP, P_APRANGE, P_STOPACTION, P_CLEARPENDINGACTION
- `src/engine/script/handlers/DialogOps.ts` — P_PAUSEBUTTON, P_COUNTDIALOG

## 3. Architecture

Pure mechanical migration. Three edit patterns:

### Pattern A — handlers currently using `requireActivePlayer`

`handlePTeleport`, `handlePTeleJump`, `handlePApRange`, `handlePStopAction`, `handlePClearPendingAction`

Change `requireActivePlayer(s, "OP_NAME")` → `requireProtectedActivePlayer(s, "OP_NAME")`.

### Pattern B — handlers with inline `PtrActivePlayer` bitmask check

`handlePDelay` (handlers.go:549), `handlePPauseButton` (handlers_dialog.go:9), `handlePCountDialog` (handlers_dialog.go:~15)

Replace:
```go
if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
    return errors.New("OP_NAME: no active player")
}
```
with:
```go
if err := requireProtectedActivePlayer(s, "OP_NAME"); err != nil {
    return err
}
```

### Pattern C — test fixture updates

For each handler, find its existing tests and change `Init(sf, mp, false, nil, nil)` → `Init(sf, mp, true, nil, nil)`. Keep the existing "no active player" error-path tests unchanged (the chained helper still produces the same error when `Self` is nil).

### Pattern D — new rejection tests

Add one `TestXxxUnprotectedRejected` per handler. Each asserts that invoking the handler with `protect=false` produces `"<OP>: script not protected"`.

## 4. File map

| File | Action |
|---|---|
| `pkg/script/handlers.go` | Migrate `handlePDelay` (Pattern B) |
| `pkg/script/handlers_dialog.go` | Migrate `handlePPauseButton`, `handlePCountDialog` (Pattern B) |
| `pkg/script/handlers_player.go` | Migrate `handlePTeleport`, `handlePTeleJump`, `handlePApRange`, `handlePStopAction`, `handlePClearPendingAction` (Pattern A) |
| `pkg/script/handlers_test.go` | `TestPDelaySuspends` fixture → protect=true; add unprotected-rejected test for PDelay |
| `pkg/script/handlers_player_test.go` | Fixtures for PTeleJump/PTeleport/PStopAction/PClearPendingAction/PApRange tests → protect=true; add 5 unprotected-rejected tests |
| `pkg/script/handlers_dialog_test.go` | Fixtures for PPauseButton/PCountDialog → protect=true; add 2 unprotected-rejected tests |

## 5. Test considerations

Some tests may call these handlers indirectly (through script bytecode that includes the opcode as part of a larger flow). Those tests ALSO need `protect=true` in their Init calls. The implementer must:

1. Grep for each test file referencing the migrated opcodes.
2. For each test that calls `handleP<op>` directly OR runs a `ScriptFile` containing an `OpP<op>`, ensure its `Init` uses `protect=true`.
3. Tests that pass `nil` as Self and assert the "no active player" error path stay UNCHANGED — the chained helper preserves that error message.

Edge case: tests for Protect field (`runner_test.go:57`) that ASSERT `Protect == true` after `Init(sf, nil, true, ...)` — these don't need migration; they already test the protect=true path.

## 6. Task split

**Single task.** Pure mechanical migration. ~70 LOC impl (8 handler edits) + ~130 LOC tests (~15 fixture updates + 8 new rejection tests).

Commit: `feat(script): migrate 8 handlers to requireProtectedActivePlayer — closes S6l-D3 (S6x)`

## 7. Deviations

| ID | Status |
|---|---|
| **S6l-D3** | **✅ FULLY CLOSED in S6x** — all TS-protected goscape handlers now gate on ScriptState.Protect |

Still-open related: handlePWalk is a stub (no gate, no real body) — future P_WALK implementation sub-spec will add the gate at that point.

No new deviations.
