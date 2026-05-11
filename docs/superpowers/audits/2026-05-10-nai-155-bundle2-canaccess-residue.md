# NAI-155 Bundle 2 — CanAccess Residue Audit

**Date:** 2026-05-10  
**Symptom:** Second OPNPC1 on Survival Expert (NPC #943) → `CanAccess()=false` across ticks 222 and 223.  
**Frame B data:** `branch_pre=0 && branch_post=0` for both ticks; `interacted=false`; tick 223 `target_still_set=false`.

---

## 1. Verdict

**PRIMARY CANDIDATE: `modalState & modalStateChat` is residually set on tick 222.**

The `[31 76 43 44]` packet arriving 383 ms after the second OPNPC1 is **not** a dialog-close packet. Opcode 31 is `INV_BUTTON1` (`gameHandlers[31] = handleInvButton1`). The actual dialog-dismiss opcode is **235** (`RESUME_PAUSEBUTTON`) or **231** (`CLOSE_MODAL`). Neither `handleResumePauseButton` nor `handleCloseModal` was the arriving packet.

The packet ordering is therefore:

```
tick 222 start: processClientsIn reads second OPNPC1 → SetInteraction on Survival Expert
  → processActiveScripts: activeScript suspended (PauseButton from first dialog) is still stored
    → RESUME_PAUSEBUTTON (opcode 235) must have NOT yet arrived this tick
  → processInteractions: p.modalState == modalStateChat → CanAccess()=false → branch_pre=0
tick 222–223 inter-tick window: opcode-235 or opcode-231 arrives in client buffer
tick 223 start: processClientsIn handles the dismiss → resumeOrFinish → OnScriptFinishedOrAborted
  → but now target was already cleared at tick 223 end (target_still_set=false)
```

**Field verdict:**
- `p.delayed`: clear — no chatnpc sets a delay on the Survival Expert first-contact path.
- `p.modalState & modalStateChat`: **RESIDUALLY SET** on tick 222 — the chatnpc opened a chat dialog that has not been dismissed yet when the second OPNPC1 arrives.
- `p.protectedScriptActive()`: likely also residually true — the chatnpc script suspended with `PauseButton` execution and `PtrProtectedActivePlayer` set (it was run with `protect=true` from the OPNPC1 handler). This compounds the gate block.

---

## 2. Evidence Per Field

### Field 1: `p.delayed`

**Goscape clear-site:** `tick.go:277` — `if p.delayed && s.currentTick >= p.delayedUntil { p.delayed = false }` inside `processActiveScripts`. This is conditional on `currentTick >= delayedUntil` — not unconditional, but correctly mirrors TS.

**TS counterpart:** `World.ts:708` — `if (player.delayed && this.currentTick >= player.delayedUntil) player.delayed = false;` — identical conditional form.

**Conclusion:** The `delayed` field lifecycle is TS-faithful. Chatnpc (`OPNPC1` → NPC dialog) does not set `p.delayed`; the `p_delay` opcode is not part of the Survival Expert's introductory chat script. **`p.delayed` is not the residual field.**

### Field 2: `p.modalState & modalStateChat`

**Setter:** `player_script.go:867` — `OpenChat` sets `p.modalState = modalStateChat`.

**Sole clearer:** `player_script.go:799` inside `CloseModal` — `p.modalState = modalStateNone`.

**CloseModal callers relevant to chatnpc dismiss:**
- `tick.go:313` — inside `processPlayerQueue`, gated on `p.requestModalClose`.
- `player_script.go:995` — via `ClearPendingAction`.
- `player_script.go:176` — inside `OnScriptFinishedOrAborted` when no MAIN modal open.
- `resume_dialog.go:25` — `handleResumePauseButton` calls `resumeOrFinish` → if script Finishes, routes via `OnScriptFinishedOrAborted` → `CloseModal(false)`.

**Packet ordering analysis:**  
`processClientsIn` runs at `tick.go:35`, BEFORE `processActiveScripts` (`:47`) and `processInteractions` (`:59`). The packet buffer is drained in `processClientsIn` → `processIn`. If the dismiss packet (`RESUME_PAUSEBUTTON` opcode 235 or `CLOSE_MODAL` opcode 231) arrives **after** tick 222's `processClientsIn` window closes, it will not be processed until tick 223's `processClientsIn`. The second OPNPC1 at 21:09:51.833 triggers tick 222; the 4-byte packet at 21:09:52.216 arrives **between** tick 222 and tick 223 (each tick = 600 ms, so tick 223 starts at ~21:09:52.033). The 4-byte packet arrives 383 ms after the OPNPC1 — within the inter-tick window — and would be processed in tick 223's `processClientsIn`.

Therefore on tick 222: `modalState == modalStateChat` → `CanAccess()=false` → `tryInteract` returns false → `branch_pre=0`.

**TS counterpart:** TS `Player.ts:745-746` — `closeModal` clears `this.protect = false` only when `!this.delayed`. Goscape's `CloseModal` at `player_script.go:790-834` does **not** have a `!p.delayed` guard before nulling `p.activeScript`, but this is the NAI-111-D1 tracked deviation. The modal-state clear itself at `player_script.go:799` is unconditional once `modalState != modalStateNone`.

**Conclusion:** `modalState & modalStateChat` is **residually set** on tick 222. This is NOT a missing clear-site — it is correct behavior: the dialog is genuinely still open when tick 222 runs. The packet that would dismiss it arrives in the inter-tick window and is processed at tick 223.

### Field 3: `p.protectedScriptActive()`

**Setter chain:** `runScript` (with `protect=true`) → `resumeOrFinish` → `StoreActiveScript` (when script suspends to `PauseButton`) → `p.activeScript` retains the `PtrProtectedActivePlayer` pointer bit (preserved by `StoreActiveScript` per NAI-111-D1).

**`protectedScriptActive()` definition:** `player_script.go:346-347` — returns `p.activeScript != nil && p.activeScript.Pointers&PtrProtectedActivePlayer != 0`.

The chatnpc OPNPC1 handler runs the script with `protect=true` (confirmed by the `tryFireOpTrigger` → `runScript` → protect=true path for OPNPC1). The script suspends at `p_pausebutton` with `Execution=PauseButton` and `Pointers&PAP != 0`. `StoreActiveScript` at `player_script.go:146-148` stores `p.activeScript = state`.

On tick 222's `processActiveScripts` (`:280-286`), the resume branch only fires for `Execution == script.Suspended` — NOT `PauseButton`. So the PAP-flagged `PauseButton`-suspended script remains stored as `p.activeScript`. Both `modalState & modalStateChat` AND `p.protectedScriptActive()` are true simultaneously.

**TS counterpart behavior:** TS `executeScript` at `Player.ts:2141` stores `script.activePlayer.protect = protect` on suspension — identical result. The `protect=true` is preserved on the Player.protect bool until `closeModal` clears it (with the `!this.delayed` guard).

**TS NAI-111-D1 divergence point:** TS `closeModal` (`Player.ts:745-746`) does `if (!this.delayed) { this.protect = false; }` **before** checking `modalState == NONE`. This means on a `requestModalClose` path (triggered by `processQueues`) even when `modalState` is still set, TS would clear `protect` from the player. Goscape does NOT have this unconditional `protect`-clearing behavior in `CloseModal` — correctly per NAI-111-D1 (which established that clearing PAP from `activeScript.Pointers` in `CloseModal` breaks in-flight handlers). However: in the chatnpc case, `closeModal` is only called AFTER the script finishes or is dismissed, at which point `activeScript` would be nulled anyway.

**Conclusion:** `p.protectedScriptActive()` is also residually true on tick 222. Both field 2 and field 3 are blocking `CanAccess()`, for the same root reason: the first-contact dialog script has not yet been dismissed.

---

## 3. Root Cause

**Packet-ordering bug (no missing clear-site).**

The Survival Expert OPNPC1 dialog on first contact suspends the script mid-execution (`PauseButton` state, `modalState=Chat`). After dialog dismiss (client sends opcode 235 `RESUME_PAUSEBUTTON`), the script resumes and finishes — calling `OnScriptFinishedOrAborted` → `CloseModal(false)` → clearing `modalState` and `activeScript`.

However, on the **second** OPNPC1 click, if the dismiss packet has NOT yet arrived by the time `processClientsIn` drains for that tick, the player still has `modalState=Chat` AND a `PauseButton`-suspended `activeScript` with PAP set. Both fields return false from `CanAccess()` → `tryInteract` early-returns branch 0 → `branch_pre=0 && branch_post=0`.

The real question is: **why is the dialog still open on the second OPNPC1 click?**

Two sub-hypotheses:
- **H-A (timing):** The user dismissed the dialog (via "Click here to continue"), but the client batched the dismiss + second OPNPC1 click in overlapping 600 ms windows such that the dismiss packet arrives in tick 222's inter-tick window. The smoke packet log `[31 76 43 44]` at +383 ms is an `INV_BUTTON1` (opcode 31), not a dismiss — confirming the dialog was NOT dismissed before the second click arrived.
- **H-B (RESUME_PAUSEBUTTON not sent):** The client did not send opcode 235 at all before the second click (the user did not click the "Continue" button first). The smoke shows no opcode 235 packet in the relevant window.

**H-A is the most likely root cause given the memory note `java_client_coord_chat_suppression.md`:** the client suppresses some packets on Tutorial Island. More critically: the Java client opcode `[31 76 43 44]` = INV_BUTTON1 + 3-byte payload `[76 43 44]` = component 0x764344 is likely the "Click here to continue" button via INV_BUTTON1 rather than RESUME_PAUSEBUTTON. If the client sends INV_BUTTON1 instead of RESUME_PAUSEBUTTON for the chatnpc dismiss, `handleResumePauseButton` would never be called, and the script would remain suspended. **This is a candidate root-cause stub for the NAI-155 investigation.**

---

## 4. Proposed Frame B Instrumentation Extension

Add 3 fields to `emitInteractionTickFrame` in `modules/world/interaction_debug.go`:

```go
"modal_state",   p.modalState,
"active_script_exec", activeScriptExec(p),   // -1 if nil, int(p.activeScript.Execution) otherwise
"protected_active", p.protectedScriptActive(),
```

Where `activeScriptExec(p)` is a local helper:
```go
func activeScriptExec(p *Player) int {
    if p.activeScript == nil { return -1 }
    return int(p.activeScript.Execution)
}
```

These three fields directly disambiguate which `CanAccess()` branch is blocking on each tick without requiring a second smoke run.

---

## 5. Routing Recommendation

**Bundle 2 is load-bearing; it cannot wait for NAI-156.**

The root cause is NOT a missing clear-site in goscape's state machine — the state machine is correct. The blocking issue is that the Java client sends `INV_BUTTON1` (opcode 31) as the chatnpc dismiss, not `RESUME_PAUSEBUTTON` (opcode 235). If that is confirmed, the fix lives in `handleInvButton1` or a new dispatch path that checks whether a `PauseButton`-suspended `activeScript` should be resumed when opcode 31 arrives on the chatnpc continue button.

**Recommended next step:** Add the 3 Frame B fields above; run a targeted smoke where you log ALL opcodes received per tick (not just the Frame B fields) to confirm whether opcode 235 or 231 ever arrives after the first-contact dialog dismiss. If opcode 235 is absent and only opcode 31 arrives, the fix is to wire the chatnpc continue-button component into `resumeButtons` at script-registration time, OR to handle the `PauseButton`-resume path inside `handleInvButton1`'s `resumeButtons` loop.

If Bundle 1's gate-parity fix does not change the smoke outcome (Frame B still shows `branch_pre=0`), Bundle 2 is the load-bearing investigation.
