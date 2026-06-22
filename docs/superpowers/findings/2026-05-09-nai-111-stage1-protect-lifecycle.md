# NAI-111 — Stage 1 findings: protect-flag lifecycle audit

**Date:** 2026-05-09
**Spec:** docs/superpowers/specs/2026-05-09-nai-111-protect-lifecycle-investigation-design.md (b13394b)
**Cadence:** general-purpose Sonnet audit subagent per investigation_subspec_cadence.

---

## §G1 — TS Player.protect lifecycle

All sites found by `rg -n "this\.protect\b|\.protect = |\.protect;" $HOME/Code/github.com/LostCityRS/Engine-TS/src/`, with context read for each hit.

### Player.ts sites

| # | File:Line | Verbatim line | Classification | Phase |
|---|-----------|---------------|----------------|-------|
| 1 | `Player.ts:460` | `this.protect = false;` | CLEAR_ON_RESET | external (resetEntity, called at respawn) |
| 2 | `Player.ts:746` | `this.protect = false;` | CLEAR_ON_CLOSEMODAL | in-flight or external (inside `if (!this.delayed)`) |
| 3 | `Player.ts:810` | `return !this.protect && !this.busy();` | READ_AS_GATE | external (canAccess gate) |
| 4 | `Player.ts:1062` | `if (this.walktrigger !== -1 && !this.protect && !this.delayed) {` | READ_AS_GATE | external (processWalktrigger gate) |
| 5 | `Player.ts:2095` | `if (!force && protect && (this.protect || this.delayed)) {` | READ_AS_GATE | runScript entry reentry-early-return |
| 6 | `Player.ts:2097` | `// printDebug('No protected access:', script.script.name, protect, this.protect);` | READ_OTHER | (debug comment only, not a read site) |
| 7 | `Player.ts:2103` | `this.protect = true;` | SET_ON_RUN_ENTRY | initial-execution / resumed (runScript, inside `if (protect)`) |
| 8 | `Player.ts:2109` | `this.protect = false;` | CLEAR_ON_RUN_EXIT | initial-execution / resumed (runScript, inside `if (protect)`, after ScriptRunner.execute returns) |
| 9 | `Player.ts:2114` | `script._activePlayer.protect = false;` | CLEAR_AT_RUN_END_VIA_POINTER_REMOVE | post-runScript exit, clears the `_activePlayer`'s flag (foreign player slot-0 via nested script) |
| 10 | `Player.ts:2119` | `script._activePlayer2.protect = false;` | CLEAR_AT_RUN_END_VIA_POINTER_REMOVE | post-runScript exit, clears the `_activePlayer2`'s flag (foreign player slot-1 via nested script) |
| 11 | `Player.ts:2130` | `// printDebug('Script did not run', script.script.name, protect, this.protect);` | READ_OTHER | (debug comment only, not a live read site) |
| 12 | `Player.ts:2141` | `script.activePlayer.protect = protect; // preserve protected access when delayed` | RESTORE_AT_SUSPEND | suspend-exit (executeScript, inside `else` branch for SUSPENDED/PAUSEBUTTON/COUNTDIALOG) |

### Cross-file reads/writes: Npc.ts

`Npc.ts` contains analogous pointer-remove + protect-clear when an NPC-driven script finishes. These are CLEAR_AT_RUN_END_VIA_POINTER_REMOVE sites on **the affected player**, not on the NPC entity:

| # | File:Line | Verbatim line | Classification | Phase |
|---|-----------|---------------|----------------|-------|
| 13 | `Npc.ts:231` | `script._activePlayer.protect = false;` | CLEAR_AT_RUN_END_VIA_POINTER_REMOVE | post-NPC-runScript exit, foreign player slot-0 |
| 14 | `Npc.ts:236` | `script._activePlayer2.protect = false;` | CLEAR_AT_RUN_END_VIA_POINTER_REMOVE | post-NPC-runScript exit, foreign player slot-1 |

### Non-Player cross-file reads: canAccess-related files

`VarPlayerType.ts:94` and `InvType.ts:108` write `this.protect = false` on their own config-object `protect` fields — these are **unrelated config fields** with the same name, not reads of `Player.protect`. Excluded from enumeration.

### Summary

- **10 Player.ts production sites** (sites 1–10 above, excluding comments).
- **2 Npc.ts production sites** (sites 13–14).
- **0 cross-file reads** of another entity's `.protect` field inside handler code (only clear-at-end writes).

---

## §G2 — TS script.pointers & ProtectedActivePlayer lifecycle

All sites found by `rg -n "ProtectedActivePlayer\b"` and `rg -n "pointerAdd|pointerRemove|pointerGet"` across `Engine-TS/src/`.

### ScriptPointer.ts definition

| File:Line | Verbatim | Classification |
|-----------|----------|----------------|
| `ScriptPointer.ts:10` | `ProtectedActivePlayer,` | Enum-value definition |
| `ScriptPointer.ts:24` | `[ScriptPointer.ProtectedActivePlayer, 'ProtectedActivePlayer'],` | Name-map entry |
| `ScriptPointer.ts:39` | `export const ProtectedActivePlayer: ScriptPointer[] = [ScriptPointer.ProtectedActivePlayer, ScriptPointer.ProtectedActivePlayer2];` | Exported array alias used by handlers |

### Player.ts pointer sites

| # | File:Line | Verbatim | Classification | Phase |
|---|-----------|----------|----------------|-------|
| P1 | `Player.ts:2102` | `script.pointerAdd(ScriptPointer.ProtectedActivePlayer);` | POINTER_ADD | runScript entry, inside `if (protect)` |
| P2 | `Player.ts:2112` | `if (script.pointerGet(ScriptPointer.ProtectedActivePlayer) && script._activePlayer) {` | POINTER_GET | runScript end guard |
| P3 | `Player.ts:2113` | `script.pointerRemove(ScriptPointer.ProtectedActivePlayer);` | POINTER_REMOVE | runScript end, inside above guard |
| P4 | `Player.ts:2117` | `if (script.pointerGet(ScriptPointer.ProtectedActivePlayer2) && script._activePlayer2) {` | POINTER_GET | runScript end guard (slot-1) |
| P5 | `Player.ts:2118` | `script.pointerRemove(ScriptPointer.ProtectedActivePlayer2);` | POINTER_REMOVE | runScript end (slot-1) |

### Npc.ts pointer sites

| # | File:Line | Verbatim | Classification | Phase |
|---|-----------|----------|----------------|-------|
| N1 | `Npc.ts:230` | `if (script.pointerGet(ScriptPointer.ProtectedActivePlayer) && script._activePlayer) {` | POINTER_GET | NPC runScript end guard |
| N2 | `Npc.ts:232` | `script.pointerRemove(ScriptPointer.ProtectedActivePlayer);` | POINTER_REMOVE | NPC runScript end |
| N3 | `Npc.ts:235` | `if (script.pointerGet(ScriptPointer.ProtectedActivePlayer2) && script._activePlayer2) {` | POINTER_GET | NPC runScript end guard (slot-1) |
| N4 | `Npc.ts:237` | `script.pointerRemove(ScriptPointer.ProtectedActivePlayer2);` | POINTER_REMOVE | NPC runScript end (slot-1) |

### World.ts pointer site

| # | File:Line | Verbatim | Classification | Phase |
|---|-----------|----------|----------------|-------|
| W1 | `World.ts:796` | `state.pointerAdd(ScriptPointer.ProtectedActivePlayer);` | POINTER_ADD | Logout processing: direct ScriptRunner path (no runScript wrapper; pointer added manually) |

### PlayerOps.ts CHECKED_HANDLER sites

`checkedHandler(ProtectedActivePlayer, ...)` calls `state.pointerCheck(pointer[state.intOperand])`, which **throws** if the pointer is not set (not a simple read gate — it throws a PointerError). All these fire during in-flight script execution.

| # | File:Line | Wrapped opcode | Classification |
|---|-----------|----------------|----------------|
| H1 | `PlayerOps.ts:352` | `P_APRANGE` | CHECKED_HANDLER |
| H2 | `PlayerOps.ts:358` | `P_ARRIVEDELAY` | CHECKED_HANDLER |
| H3 | `PlayerOps.ts:368` | `P_COUNTDIALOG` | CHECKED_HANDLER |
| H4 | `PlayerOps.ts:375` | `P_DELAY` | CHECKED_HANDLER |
| H5 | `PlayerOps.ts:381` | `P_OPHELD` | CHECKED_HANDLER |
| H6 | `PlayerOps.ts:386` | `P_OPLOC` | CHECKED_HANDLER |
| H7 | `PlayerOps.ts:403` | `P_OPNPC` | CHECKED_HANDLER |
| H8 | `PlayerOps.ts:417` | `P_OPNPCT` | CHECKED_HANDLER |
| H9 | `PlayerOps.ts:424` | `P_PAUSEBUTTON` | CHECKED_HANDLER |
| H10 | `PlayerOps.ts:429` | `P_STOPACTION` | CHECKED_HANDLER |
| H11 | `PlayerOps.ts:434` | `P_CLEARPENDINGACTION` | CHECKED_HANDLER |
| H12 | `PlayerOps.ts:439` | `P_TELEJUMP` | CHECKED_HANDLER |
| H13 | `PlayerOps.ts:447` | `P_TELEPORT` | CHECKED_HANDLER |
| H14 | `PlayerOps.ts:455` | `P_WALK` | CHECKED_HANDLER |
| H15 | `PlayerOps.ts:622` | `P_LOGOUT` | CHECKED_HANDLER |
| H16 | `PlayerOps.ts:626` | `P_PREVENTLOGOUT` | CHECKED_HANDLER |
| H17 | `PlayerOps.ts:882` | `P_EXACTMOVE` | CHECKED_HANDLER |
| H18 | `PlayerOps.ts:922` | `P_LOCMERGE` | CHECKED_HANDLER |
| H19 | `PlayerOps.ts:990` | `P_OPOBJ` | CHECKED_HANDLER |
| H20 | `PlayerOps.ts:1009` | `P_OPPLAYER` | CHECKED_HANDLER |
| H21 | `PlayerOps.ts:1127` | `P_OPPLAYERT` | CHECKED_HANDLER |
| H22 | `PlayerOps.ts:1171` | `P_ANIMPROTECT` | CHECKED_HANDLER |
| H23 | `PlayerOps.ts:1180` | `WEIGHT` | CHECKED_HANDLER |
| H24 | `PlayerOps.ts:1204` | `P_RUN` | CHECKED_HANDLER |

### PlayerOps.ts non-handler POINTER_GET sites

| # | File:Line | Verbatim | Classification |
|---|-----------|----------|----------------|
| Q1 | `PlayerOps.ts:79` | `if (state.pointerGet(ProtectedActivePlayer[state.intOperand]) && state.activePlayer.uid === uid) {` | POINTER_GET — P_FINDUID fast-path (same protected player, no re-bind needed) |
| Q2 | `PlayerOps.ts:92` | `state.pointerAdd(ProtectedActivePlayer[state.intOperand]);` | POINTER_ADD — P_FINDUID success path (adds protect flag for newly-bound player) |

### InvOps.ts POINTER_GET (inline, not via checkedHandler)

InvOps uses direct `state.pointerGet(ProtectedActivePlayer[state.intOperand])` rather than `checkedHandler`. Pattern: `if (!state.pointerGet(...) && invType.protect && invType.scope !== InvType.SCOPE_SHARED)`. Sites: InvOps.ts:64, 91, 119, 136, 149, 172, 197, 220, 329, 333, 359, 363, 393, 397, 507, 511, 543, 547, 578, 582, 607, 692, 733.

### CoreOps.ts POINTER_GET

| # | File:Line | Verbatim | Classification |
|---|-----------|----------|----------------|
| C1 | `CoreOps.ts:50` | `if (!state.pointerGet(ProtectedActivePlayer[secondary]) && varpType.protect) {` | POINTER_GET — POP_VARP protect gate |

### Key observation: no TS CLOSEMODAL path touches script.pointers

`closeModal()` in Player.ts (lines 741–794) clears `this.protect = false` (L746) but contains **zero** calls to `pointerRemove`, `pointerAdd`, or `pointerGet` on `ProtectedActivePlayer`. The `script.pointers&PAP` bitmask is therefore **never touched by closeModal** in TS.

---

## §G1×G2 — drift table

The following table maps each lifecycle phase against both fields. TS line numbers cited verbatim.

| Phase | `this.protect` | `script.pointers&PAP` |
|-------|---------------|----------------------|
| **initial-execution entry** (`runScript` with `protect=true`, L2101–2103) | → `true` (L2103) | → set (L2102: `pointerAdd`) |
| **initial-execution mid-flight, before any closeModal** | `true` (unchanged) | set (unchanged) |
| **initial-execution mid-flight, after closeModal** (`!delayed` branch, L745–747) | → `false` (L746) | **unchanged** — still set. `closeModal` has zero pointer operations. **Drift confirmed.** |
| **initial-execution exit** (ScriptRunner.execute returns, L2108–2113) | → `false` (L2109, inside `if (protect)`) | → cleared (L2113: `pointerRemove`, inside pointer-get guard L2112). Both clear, no drift. |
| **suspend exit** (`executeScript` stores state, L2134–2141; `protect` local was captured before L2103) | `→ restored to original protect arg` (L2141: `script.activePlayer.protect = protect`) | → **cleared** by prior L2113 in the same `runScript` call that just returned. **Drift confirmed**: script.pointers&PAP cleared; this.protect restored to original protect arg (true for a protected script). |
| **resume entry** (caller fires another `runScript` call with `protect=true`, L2102–2103) | → `true` (L2103) | → re-set (L2102) |
| **resumed mid-flight, after closeModal** (`!delayed` branch, L745–747) | → `false` (L746) | **unchanged** — still set. Same drift as initial-execution mid-flight. **Drift confirmed.** |
| **runScript end (Finished or Aborted)** (L2108–2119) | → `false` (L2109) | → cleared (L2113) |

### Drift summary

- **Pre-Stage-1 hypothesis confirmed on all three claimed drift points:**
  1. Mid-flight `closeModal()`: `this.protect → false`; `script.pointers&PAP` **unchanged**.
  2. Suspend exit: `script.pointers&PAP → cleared` (L2113); `this.protect → restored to protect arg` (L2141).
  3. Resume entry: both → set (L2102 + L2103).

- **No additional drift points discovered** that the spec did not anticipate.

---

## §G3 — Goscape consumer decision table

Excludes `_test.go` files. Consumers found via:
- `rg -n "PtrProtectedActivePlayer\b"` in `pkg/` and `modules/`
- `rg -n "protectedScriptActive\b|requireProtectedActivePlayer\b"` across repo

Non-test production consumers:

| Site (file:line) | Consumer kind | Currently reads | Should map to TS | Status |
|------------------|---------------|-----------------|------------------|--------|
| `pkg/script/handlers_player.go:61` (inside `requireProtectedActivePlayer`) | IN_FLIGHT_HANDLER_GATE | `s.Pointers&PtrProtectedActivePlayer` | TS `script.pointers&PAP` | **correct** — in-flight script state. The over-clear at L728-730 corrupts the in-flight `s.Pointers` (same struct pointer) only on resumed scripts; deletion of L728-730 restores correctness. |
| `pkg/script/handlers_player.go:154` (`P_ANIMPROTECT`) | IN_FLIGHT_HANDLER_GATE | calls `requireProtectedActivePlayer` → `s.Pointers&PAP` | TS `script.pointers&PAP` | **broken (over-clear)** — if `CloseModal` fires mid-resumed-script, pointer cleared before this handler executes |
| `pkg/script/handlers_player.go:570` (`P_TELEPORT`) | IN_FLIGHT_HANDLER_GATE | calls `requireProtectedActivePlayer` → `s.Pointers&PAP` | TS `script.pointers&PAP` | **broken (over-clear)** |
| `pkg/script/handlers_player.go:607` (`P_TELEJUMP`) | IN_FLIGHT_HANDLER_GATE | calls `requireProtectedActivePlayer` → `s.Pointers&PAP` | TS `script.pointers&PAP` | **broken (over-clear)** — this is the confirmed symptom site |
| `pkg/script/handlers_player.go:636` (`P_RUN`) | IN_FLIGHT_HANDLER_GATE | calls `requireProtectedActivePlayer` → `s.Pointers&PAP` | TS `script.pointers&PAP` | **broken (over-clear)** |
| `pkg/script/handlers_player.go:802` (`P_STOPACTION`) | IN_FLIGHT_HANDLER_GATE | calls `requireProtectedActivePlayer` → `s.Pointers&PAP` | TS `script.pointers&PAP` | **broken (over-clear)** |
| `pkg/script/handlers_player.go:810` (`P_CLEARPENDINGACTION`) | IN_FLIGHT_HANDLER_GATE | calls `requireProtectedActivePlayer` → `s.Pointers&PAP` | TS `script.pointers&PAP` | **broken (over-clear)** |
| `pkg/script/handlers_player.go:836` (`P_LOGOUT`) | IN_FLIGHT_HANDLER_GATE | calls `requireProtectedActivePlayer` → `s.Pointers&PAP` | TS `script.pointers&PAP` | **broken (over-clear)** |
| `pkg/script/handlers_player.go:853` (`P_APRANGE`) | IN_FLIGHT_HANDLER_GATE | calls `requireProtectedActivePlayer` → `s.Pointers&PAP` | TS `script.pointers&PAP` | **broken (over-clear)** |
| `pkg/script/handlers_player.go:873` (`P_OPLOC`) | IN_FLIGHT_HANDLER_GATE | calls `requireProtectedActivePlayer` → `s.Pointers&PAP` | TS `script.pointers&PAP` | **broken (over-clear)** |
| `pkg/script/handlers_player.go:897` (`P_OPNPC`) | IN_FLIGHT_HANDLER_GATE | calls `requireProtectedActivePlayer` → `s.Pointers&PAP` | TS `script.pointers&PAP` | **broken (over-clear)** |
| `pkg/script/handlers_player.go:968` (`P_FINDUID` fast-path, `s.Pointers&PtrProtectedActivePlayer != 0`) | IN_FLIGHT_HANDLER_GATE | direct `s.Pointers&PAP` | TS `script.pointers&PAP` | **broken (over-clear)** — fast-path short-circuit fails if pointer cleared by CloseModal mid-resumed-script |
| `pkg/script/handlers_player.go:991` (`P_FINDUID` success path, `s.Pointers \|= PtrActivePlayer \| PtrProtectedActivePlayer`) | MUTATOR | adds `PtrProtectedActivePlayer` on lookup success | TS `pointerAdd(ProtectedActivePlayer[slot])` | **correct** — adds, does not wrongly clear |
| `pkg/script/handlers_player.go:1239` (`P_OPOBJ`) | IN_FLIGHT_HANDLER_GATE | calls `requireProtectedActivePlayer` → `s.Pointers&PAP` | TS `script.pointers&PAP` | **broken (over-clear)** |
| `pkg/script/handlers_player.go:1318` (`P_OPNPCT`) | IN_FLIGHT_HANDLER_GATE | calls `requireProtectedActivePlayer` → `s.Pointers&PAP` | TS `script.pointers&PAP` | **broken (over-clear)** |
| `pkg/script/handlers_player.go:1352` (`P_OPPLAYER`) | IN_FLIGHT_HANDLER_GATE | calls `requireProtectedActivePlayer` → `s.Pointers&PAP` | TS `script.pointers&PAP` | **broken (over-clear)** |
| `pkg/script/handlers_player.go:1482` (`P_PREVENTLOGOUT`) | IN_FLIGHT_HANDLER_GATE | calls `requireProtectedActivePlayer` → `s.Pointers&PAP` | TS `script.pointers&PAP` | **broken (over-clear)** |
| `pkg/script/handlers_dialog.go:10` (`P_PAUSEBUTTON`) | IN_FLIGHT_HANDLER_GATE | calls `requireProtectedActivePlayer` → `s.Pointers&PAP` | TS `script.pointers&PAP` | **broken (over-clear)** |
| `pkg/script/handlers_dialog.go:20` (`P_COUNTDIALOG`) | IN_FLIGHT_HANDLER_GATE | calls `requireProtectedActivePlayer` → `s.Pointers&PAP` | TS `script.pointers&PAP` | **broken (over-clear)** |
| `pkg/script/handlers.go:732` (`P_DELAY`) | IN_FLIGHT_HANDLER_GATE | calls `requireProtectedActivePlayer` → `s.Pointers&PAP` | TS `script.pointers&PAP` | **broken (over-clear)** |
| `pkg/script/handlers.go:755` (`P_ARRIVEDELAY`) | IN_FLIGHT_HANDLER_GATE | calls `requireProtectedActivePlayer` → `s.Pointers&PAP` | TS `script.pointers&PAP` | **broken (over-clear)** |
| `pkg/script/handlers_inv.go:341` (`INV_ADD` protect gate) | IN_FLIGHT_HANDLER_GATE | `s.Pointers&PtrProtectedActivePlayer` | TS `script.pointers&PAP` | **broken (over-clear)** |
| `pkg/script/handlers_inv.go:431` (`INV_DEL` protect gate) | IN_FLIGHT_HANDLER_GATE | `s.Pointers&PtrProtectedActivePlayer` | TS `script.pointers&PAP` | **broken (over-clear)** |
| `pkg/script/handlers_inv.go:460` (`INV_DROPSLOT` protect gate) | IN_FLIGHT_HANDLER_GATE | `s.Pointers&PtrProtectedActivePlayer` | TS `script.pointers&PAP` | **broken (over-clear)** |
| `pkg/script/handlers_inv.go:502` | IN_FLIGHT_HANDLER_GATE | `s.Pointers&PtrProtectedActivePlayer` | TS `script.pointers&PAP` | **broken (over-clear)** |
| `pkg/script/handlers_inv.go:533` | IN_FLIGHT_HANDLER_GATE | `s.Pointers&PtrProtectedActivePlayer` | TS `script.pointers&PAP` | **broken (over-clear)** |
| `pkg/script/handlers_inv.go:583`, `:587` | IN_FLIGHT_HANDLER_GATE | `s.Pointers&PtrProtectedActivePlayer` | TS `script.pointers&PAP` | **broken (over-clear)** |
| `pkg/script/handlers_inv.go:639`, `:642` | IN_FLIGHT_HANDLER_GATE | `s.Pointers&PtrProtectedActivePlayer` | TS `script.pointers&PAP` | **broken (over-clear)** |
| `pkg/script/handlers_inv.go:896`, `:900` | IN_FLIGHT_HANDLER_GATE | `s.Pointers&PtrProtectedActivePlayer` | TS `script.pointers&PAP` | **broken (over-clear)** |
| `pkg/script/handlers_inv.go:958` | IN_FLIGHT_HANDLER_GATE | `s.Pointers&PtrProtectedActivePlayer` | TS `script.pointers&PAP` | **broken (over-clear)** |
| `pkg/script/handlers_inv.go:1026`, `:1030` | IN_FLIGHT_HANDLER_GATE | `s.Pointers&PtrProtectedActivePlayer` | TS `script.pointers&PAP` | **broken (over-clear)** |
| `pkg/script/handlers_inv.go:1106`, `:1110` | IN_FLIGHT_HANDLER_GATE | `s.Pointers&PtrProtectedActivePlayer` | TS `script.pointers&PAP` | **broken (over-clear)** |
| `pkg/script/handlers_inv.go:1180` | IN_FLIGHT_HANDLER_GATE | `s.Pointers&PtrProtectedActivePlayer` | TS `script.pointers&PAP` | **broken (over-clear)** |
| `pkg/script/handlers_inv.go:1265`, `:1272` (BOTH_MOVEINV, uses flag variable) | IN_FLIGHT_HANDLER_GATE | `s.Pointers&fromProtectedFlag/toProtectedFlag` (resolves to `PtrProtectedActivePlayer`) | TS `script.pointers&PAP` | **broken (over-clear)** |
| `pkg/script/handlers_inv.go:1398` (INV_DROPITEM_DELAYED, `protectFlag := PtrProtectedActivePlayer`) | IN_FLIGHT_HANDLER_GATE | `s.Pointers&protectFlag` (resolves to `PtrProtectedActivePlayer`) | TS `script.pointers&PAP` | **broken (over-clear)** |
| `pkg/script/handlers_vars.go:69` (`POP_VARP` protect gate) | IN_FLIGHT_HANDLER_GATE | `s.Pointers&PtrProtectedActivePlayer` | TS `script.pointers&PAP` | **broken (over-clear)** |
| `pkg/script/runner.go:35` (`Init`, `s.Pointers \|= PtrProtectedActivePlayer`) | MUTATOR | adds flag at script init | TS `pointerAdd(PAP)` in runScript | **correct** — initialization, not a runtime gate |
| `modules/world/player_script.go:303` (`protectedScriptActive`, reads `p.activeScript.Pointers&PAP`) | EXTERNAL_CANACCESS | `p.activeScript.Pointers&PAP` | TS `this.protect` (Player-level) | **drift-tolerant** — reads from a *suspended* (stored) `activeScript`. At suspension, TS clears `script.pointers&PAP` (L2113) AND restores `this.protect = protect` (L2141). Goscape's `resumeOrFinish` calls `self.StoreActiveScript(state)` (script.go:131) which stores the in-flight `*ScriptState` **without** clearing its Pointers. So `p.activeScript.Pointers&PAP` is `true` while the script is suspended — matching TS `this.protect = protect (true)`. The mapping holds for the external "is a protected script suspended?" question. |
| `modules/world/player_script.go:290` (`CanAccess`, calls `protectedScriptActive()`) | EXTERNAL_CANACCESS | delegates to `protectedScriptActive` | TS `this.protect` | **drift-tolerant** — per above |
| `modules/world/interaction.go:312` (`processWalktrigger`, `p.protectedScriptActive()`) | EXTERNAL_CANACCESS | delegates to `protectedScriptActive` | TS `!this.protect` | **drift-tolerant** — per above. Note: once L728-730 is deleted, `protectedScriptActive()` correctly returns true throughout a resumed-script's in-flight execution (the pointer is no longer stripped mid-flight), so the walktrigger gate blocks correctly for the entire duration. This is the correct TS behavior. |
| `modules/world/interaction.go:617` (`pathToPathingTarget`, `p.protectedScriptActive()`) | EXTERNAL_CANACCESS | delegates to `protectedScriptActive` | TS canAccess approximation | **drift-tolerant** — per above. Same argument: without the over-clear, the flag stays set for the whole resumed run. |
| `modules/world/player_script.go:728-730` (CloseModal over-clear: `p.activeScript.Pointers &^= script.PtrProtectedActivePlayer`) | MUTATOR | clears `p.activeScript.Pointers&PAP` | TS clears `this.protect` only — not `script.pointers&PAP` | **broken (over-clear)** — root-cause site. The `p.activeScript` pointer during a resumed run IS the in-flight `s.Pointers`; stripping it corrupts all downstream handler gates. |
| `modules/world/tick.go:589` (comment only, `protect → activeScript.Pointers&PtrProtectedActivePlayer`) | OTHER | comment explaining convergence mapping | N/A (comment) | N/A |

### G3 row status counts (non-test, non-comment production sites)

- **broken (over-clear):** 35 rows (all share the same root: L728-730 corrupts the single `s.Pointers` bitmask used by all in-flight gates)
- **drift-tolerant:** 4 rows (`protectedScriptActive`, `CanAccess`, `processWalktrigger`, `pathToPathingTarget`)
- **correct:** 3 rows (`requireProtectedActivePlayer` definition body, `P_FINDUID` add path, `Init` add path)
- **broken (under-restore):** 0 rows

**No consumer requires TS `this.protect` (Player-level bool) semantics that the script-state-pointer mapping cannot provide.** The external consumers (`protectedScriptActive`, `CanAccess`, `processWalktrigger`, `pathToPathingTarget`) read the suspended script's stored Pointers — which correctly preserves the PAP flag across suspensions in goscape's `StoreActiveScript` path (unlike TS, which clears and re-adds, but the net observable result is the same: flag is set while script is suspended and protected).

### G3 test-file inventory (advisory)

Test files that reference the protect tokens — Stage 2 will need to revisit them:

- `modules/world/modal_close_test.go` — contains the 3-4 tests that pin the broken behavior:
  - `TestCloseModalClearsActiveScriptProtectWhenNotDelayed` (lines ~100–124): pins L728-730 clearing behavior — **must be retired**.
  - `TestCloseModalPreservesActiveScriptProtectWhenDelayed` (lines ~126–145): pins the `delayed` branch of L728-730 — **must be retired**.
  - `TestCloseModalNoneEarlyReturnStillRunsClearWeakQueueAndProtect` (lines ~213–238): pins the `NONE` early-return over-clear — **must be retired**.
  - `TestCloseModalPreservesActiveScriptOnSuspended` (line ~280–297): uses `p.delayed = true` as a workaround to prevent the over-clear from firing — the workaround becomes unnecessary after deletion; the test body still tests valid behavior (Suspended script preserved), but the `delayed=true` setup note needs updating.
- `modules/world/player_test.go` — tests for `protectedScriptActive` (lines ~721–731): behavior preserved under Scenario A; no changes needed.
- `modules/world/interaction_test.go` — tests for walktrigger / `processWalktrigger` protect gate (lines ~772–815): behavior preserved (actually more correct after fix); no changes needed.
- `modules/world/server_test.go` — references `PtrProtectedActivePlayer` in a player setup fixture (line ~620): no changes needed.
- `pkg/script/handlers_player_test.go` — numerous references to `PtrProtectedActivePlayer`; all gate tests for P_TELEJUMP, P_TELEPORT, etc.: behavior preserved under Scenario A; no changes needed.
- `pkg/script/handlers_player_run_probe_test.go` — 3 occurrences; probe tests: no changes needed.
- `pkg/script/handlers_inv_test.go` — numerous references; inv protect-gate tests: behavior preserved; no changes needed.
- `pkg/script/handlers_vars_test.go` — 1 reference; varp protect-gate test: no changes needed.
- `pkg/script/runner_test.go` — 2 references; Init + protect flag test: no changes needed.

---

## §scope — Stage 2 dispatch shape

**Recommendation:** Scenario A

**Rationale:** The G3 audit found **zero "broken (under-restore)"** rows. Every broken consumer is an IN_FLIGHT_HANDLER_GATE or the MUTATOR at L728-730 itself. All 35 broken rows share a single root cause: `CloseModal` strips `PtrProtectedActivePlayer` from `p.activeScript.Pointers`, which is the **same struct pointer** that the in-flight `s.Pointers` field refers to during a resumed script execution. Deleting L728-730 restores the invariant that `s.Pointers&PAP` remains set for the entire duration of the resumed script, which is the correct TS `script.pointers&PAP` semantics. The four EXTERNAL_CANACCESS consumers (`protectedScriptActive`, `CanAccess`, `processWalktrigger`, `pathToPathingTarget`) are already drift-tolerant because goscape's `StoreActiveScript` preserves the pointer on the `*ScriptState` across suspensions — no `Player.protect` bool is needed to answer the "is a protected script suspended?" question correctly.

**Stage 2 plan path (proposed):** `docs/superpowers/plans/2026-05-09-nai-111-stage-2-minimal-delete-over-clear.md`

---

## §provenance

- All TS cites verified by `Read`-ing the cited file:line (Player.ts L460, L741-794, L805-812, L1057-1069, L2094-2151; Npc.ts L225-238; World.ts L788-800; ScriptPointer.ts; PlayerOps.ts L60-461; ScriptState.ts L160-190).
- All goscape cites verified by `rg`-ing the cited tokens across `pkg/` and `modules/`.
- Audit ran approximately 30 minutes; visited approximately 18 files.
- Self-flagged uncertainty:
  - **`modules/world/player_script.go:303` drift-tolerant verdict**: the claim rests on goscape's `StoreActiveScript` preserving `Pointers` without clearing. This was inferred from `resumeOrFinish` (script.go:130-131: `self.StoreActiveScript(state)`). If `StoreActiveScript` clears the pointer before storing, the drift-tolerant verdict would change to "broken (under-restore)". Controller should verify `StoreActiveScript` implementation does not clear PAP on the stored `*ScriptState`.
  - **`TestCloseModalPreservesActiveScriptOnSuspended` (modal_close_test.go:280-297)**: the test uses `p.delayed = true` as a workaround to prevent L728-730 from firing. It is **not** a test of the broken behavior — it tests that `Suspended` (non-dialog) scripts are preserved on CloseModal. After fixing, the test body remains valid; only the setup note changes. Controller should confirm whether this test should be updated or left as-is.
