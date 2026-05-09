# NAI-111 — P_TELEJUMP `[label,tutorial_complete]` "script not protected" investigation

**Status:** Stage 1 audit pending.
**Cadence:** `investigation_subspec_cadence` — Stage 1 Sonnet Explore audit → Stage 2 fix shape selection from decision table → smoke binding.
**Tracker entry:** `nai_followups.md:5568` (queued since NAI-109 close 2026-05-05; carried through ~17 brainstorm queues).

## §1 Background & symptom

**Symptom:** Tutorial Island terminal teleport never fires. The player completes the Magic Instructor sub-quest, picks "Yes, go to mainland" at the multi-choice dialog, the script JUMPs through `@multi2_header → newbie_magic_instructor_to_mainland → @tutorial_complete` (`Content/scripts/tutorial/scripts/tutorial.rs2:296`), the `tut_close;` and `if_close;` opcodes call `(*Player).CloseModal`, and the next opcode `p_telejump(0_50_50_22_22)` aborts with the runtime error `"P_TELEJUMP: script not protected"`. The player remains on Tutorial Island.

**Reproduction path (content):** Magic Instructor → cast Wind Strike on chicken successfully → re-talk to Magic Instructor → "Do you want to go to the mainland?" → "Yes" → server log emits the abort.

**Static binding (informal pre-Stage-1; subject to Stage 1 verification):**

`modules/world/player_script.go:728-730` strips `PtrProtectedActivePlayer` from the in-flight `p.activeScript.Pointers` whenever `(*Player).CloseModal` is called and the player is not delayed:

```go
if !p.delayed && p.activeScript != nil {
    p.activeScript.Pointers &^= script.PtrProtectedActivePlayer
}
```

This line was introduced by **NAI-53 T3** (commit `eee564a`, 2026-05-01) under the **NAI-52 convergence** premise: "TS `this.protect` ↔ goscape `activeScript.Protect`". The convergence collapses two TS fields whose lifecycles drift:

- TS `this.protect` (Player-level bool): set in `Player.runScript` (Player.ts:2103), cleared at `runScript` return (Player.ts:2109), restored at `executeScript` suspend exit (Player.ts:2141), cleared by `Player.closeModal` (Player.ts:746). Also gates the `runScript` reentry early-return (Player.ts:2095).
- TS `script.pointers & ScriptPointer.ProtectedActivePlayer` (script-state bitmask): added at `runScript` entry (Player.ts:2102), removed at `runScript` exit (Player.ts:2113). **Read by every `checkedHandler(ProtectedActivePlayer, ...)` handler** (e.g. `PlayerOps.ts:439` for P_TELEJUMP, `:447` for P_TELEPORT).

**Crucial drift case:** TS `Player.closeModal` (Player.ts:741-794) clears `this.protect = false` BUT does **not** touch `script.pointers`. So when an in-flight protected script calls `closeModal()` (via `tut_close` or `if_close` opcode), the script's `pointers&PAP` is preserved. The next protected-handler opcode in the same script passes its gate.

Goscape's NAI-52 convergence collapses these into a single field (`script.Pointers&PtrProtectedActivePlayer`), so applying TS line 2746's `this.protect = false` to the convergence incorrectly affects in-flight execution — stripping the gate from subsequent opcodes in the same script.

**Why this only surfaces on resumed scripts:** `p.activeScript` is only set when a script suspends (StoreActiveScript). Initial-execution scripts have `p.activeScript == nil` so line 728's nil-guard short-circuits. The bug manifests specifically when:
1. Script starts protected (e.g. `[opnpc1,_]` fires with `protect=true`).
2. Script suspends (e.g. `p_pausebutton` inside `~p_choice2_header`).
3. RESUME_PAUSEBUTTON arrives → `s.resumeOrFinish(p.activeScript, p)` → `p.activeScript` stays bound to the running state during execution.
4. Mid-flight `CloseModal` (from `tut_close`/`if_close`/etc.) strips the pointer from `p.activeScript.Pointers` (which IS the in-flight `s.Pointers`).
5. Next protected-required opcode (`P_TELEJUMP`, `P_TELEPORT`, etc.) aborts.

## §2 Stage 1 audit (Sonnet Explore subagent, single pass)

A Sonnet Explore subagent enumerates the protect lifecycle on both sides and produces a decision table.

### G1 — TS `Player.protect` lifecycle

Enumerate every read/write of `this.protect` in `Engine-TS/src/engine/entity/Player.ts`. Capture, for each:
- Line range + verbatim snippet.
- Site classification: init / set-on-entry / clear-on-exit / restore-at-suspend / clear-on-closeModal / read-as-gate / read-other.
- TS lifecycle phase (initial-execution / suspended / resumed / external).

Known sites from preliminary read (Stage 1 must verify exhaustively):
- L460: `this.protect = false;` (Player.reset / cleanup).
- L746: `this.protect = false;` (closeModal, !delayed branch).
- L810: `return !this.protect && !this.busy();` (canAccess gate).
- L1062: `if (this.walktrigger !== -1 && !this.protect && !this.delayed)` (walktrigger gate).
- L2095: `if (!force && protect && (this.protect || this.delayed))` (runScript reentry early-return).
- L2103: `this.protect = true;` (runScript protect-entry).
- L2109: `this.protect = false;` (runScript protect-exit).
- L2114: `script._activePlayer.protect = false;` (runScript end, conditional on script pointer remove).
- L2119: `script._activePlayer2.protect = false;` (same for slot-2).
- L2141: `script.activePlayer.protect = protect;` (executeScript suspend-exit restore).

Audit must produce the complete list with citations. If sites exist outside Player.ts (e.g. cross-file readers), enumerate those too.

### G2 — TS `script.pointers & ProtectedActivePlayer` lifecycle

Enumerate every add/remove/read site for `ScriptPointer.ProtectedActivePlayer` and `ScriptPointer.ProtectedActivePlayer2` across `Engine-TS/src/`:
- Line range + verbatim snippet.
- Site classification: pointerAdd / pointerRemove / pointerGet / `checkedHandler(ProtectedActivePlayer, ...)` wrapper.
- For each handler reader: confirm the wrapper reads the pointer at handler-dispatch time.

Known sites from preliminary read (Stage 1 must verify exhaustively):
- Player.ts:2102 — `script.pointerAdd(ScriptPointer.ProtectedActivePlayer);` (runScript entry).
- Player.ts:2112-2113 — `if (script.pointerGet(...)) script.pointerRemove(...);` (runScript end).
- Player.ts:2117-2118 — same for slot-2.
- All `checkedHandler(ProtectedActivePlayer, ...)` and `checkedHandler([ProtectedActivePlayer, ...], ...)` wrappers in `script/handlers/*.ts`.

**G1×G2 drift table (deliverable):** a table with columns `[lifecycle phase, this.protect, script.pointers&PAP]` showing where they differ. Pre-Stage-1 hypothesis: they differ at exactly these phases:
- During mid-flight `closeModal()`: `this.protect → false`; `script.pointers&PAP` unchanged.
- At suspend exit (Player.ts:2113-2141): `script.pointers&PAP → cleared`; `this.protect → restored to protect arg`.
- At resume entry (Player.ts:2102-2103): both → set.

Audit confirms or refutes the hypothesis with verbatim cites.

### G3 — Goscape consumer audit + decision table

Enumerate every goscape consumer of:
- `s.Pointers & PtrProtectedActivePlayer` (script-state direct reads inside handlers).
- `p.activeScript.Pointers & PtrProtectedActivePlayer` (Player-side reads of the suspended script's flag).
- `(*Player).protectedScriptActive()` (the NAI-52 helper).
- `requireProtectedActivePlayer` / `requireProtectedActivePlayer2` (helper functions wrapping the gate).

For each consumer, produce a row in the decision table:

| Site (file:line) | Consumer kind | Currently reads | Should map to TS | Status |
|---|---|---|---|---|
| `pkg/script/handlers_player.go:607` (handlePTeleJump) | in-flight handler gate | `s.Pointers&PAP` | `script.pointers&PAP` | ? |
| `pkg/script/handlers_inv.go:341` (and ~15 InvType.Protect sites) | in-flight handler gate | `s.Pointers&PAP` | `script.pointers&PAP` | ? |
| `modules/world/player_script.go:303` (protectedScriptActive) | external CanAccess | `p.activeScript.Pointers&PAP` | TS `this.protect` | ? |
| `modules/world/player_script.go:283` (CanAccess) | external | calls protectedScriptActive | TS `this.protect` | ? |
| `modules/world/interaction.go:~308` (processWalktrigger) | external | calls protectedScriptActive | TS `!this.protect` | ? |
| `modules/world/player_script.go:728-730` (CloseModal clear) | mutator | clears `p.activeScript.Pointers&PAP` | TS clears `this.protect` only | broken |

Audit fills "Currently reads" + "Status" for every grep hit, and must enumerate sites I haven't pre-listed.

**Status verdict per row:**
- "correct under current goscape" — the consumer's mapping holds in the absence of the line 728-730 clear.
- "broken (over-clear)" — the line 728-730 clear corrupts this consumer's read.
- "broken (under-restore)" — the consumer needs TS `this.protect` semantics that the goscape mapping doesn't provide.
- "drift-tolerant" — both readings produce identical observable behavior on the smoke path.

### Stage 1 deliverable

`docs/superpowers/findings/2026-05-09-nai-111-stage1-protect-lifecycle.md` containing:
- G1 enumeration table.
- G2 enumeration table.
- G1×G2 drift table.
- G3 decision table (every consumer, classified).
- **Fix-shape recommendation:** Scenario A (minimal) if every "broken" row in G3 is corrected by deleting lines 728-730. Scenario B (full TS port) if any consumer requires TS `this.protect` semantics that the script-state mapping cannot provide. Scenario C (other) if the audit surfaces unexpected drift not contemplated by the pre-Stage-1 hypothesis.

### Verification gate (per `audit_subagent_fabrication`)

Controller spot-checks the audit before accepting:
- 3 random TS line cites verified via direct `Read` of the cited file:line.
- 3 random goscape grep claims verified via direct `rg`.
- Drift table cross-checked against my preliminary read in §1 (audit should confirm or refute, not silently re-frame).

## §3 Stage 2 fix scenarios

The Stage 1 decision table picks one of these. Stage 2 spec/plan is written after Stage 1 lands.

### Scenario A — Minimal: delete the over-clear (~3 LOC + test churn)

**Production diff:**
- Delete `modules/world/player_script.go:728-730` (the 3-line `if !p.delayed && p.activeScript != nil { ... }` block in `(*Player).CloseModal`).
- Update doc-comments at `player_script.go:712-723, 270-282, 296-301` to revise the NAI-52 convergence claim. New rationale: `script.Pointers&PtrProtectedActivePlayer` maps **only** to TS `script.pointers&PAP` (the in-flight script flag). The TS `this.protect` Player-level field has no goscape analog because:
  - TS reads `this.protect` only for the runScript reentry early-return (Player.ts:2095) and for canAccess/walktrigger gates (Player.ts:810, 1062).
  - Goscape's `protectedScriptActive` reads the suspended `p.activeScript`'s pointer — the pointer persists across suspensions on the same `*ScriptState` struct, so the "is a protected script suspended" question maps cleanly to `p.activeScript != nil && pointer-set`.
  - The runScript reentry early-return is satisfied by the existing `protectedScriptActive`-driven CanAccess gate.

**Test diff:**
- Retire the tests in `modules/world/modal_close_test.go` that pin the broken behavior:
  - `TestCloseModalClearsActiveScriptProtectWhenNotDelayed` (lines ~100-119).
  - `TestCloseModalPreservesActiveScriptProtectWhenDelayed` (lines ~126-143).
  - `TestCloseModalNoneEarlyReturnStillRunsClearWeakQueueAndProtect` (lines ~213-236).
  - The literal at line ~288 (Stage 2 plan resolves exact line ranges from HEAD).
- Add 1-2 regression tests:
  - `TestCloseModalPreservesInFlightProtectOnResumedScript` — pin: a script with `Pointers=PtrActivePlayer|PtrProtectedActivePlayer`, stored on `p.activeScript`, when CloseModal is called, the script's `Pointers&PAP` is preserved. (The active script struct is the in-flight state during a resumed run.)
  - `TestResumePauseButton_TutCloseThenPTeleJump_GateHolds` (or equivalent integration-style test) — pin the smoke path at unit level: build a script that is PauseButton-suspended with PAP set, call CloseModal mid-resume, verify the next opcode (or a `requireProtectedActivePlayer` invocation) does not abort.

**DEVIATION-NAI-111-D1:** NAI-52 convergence narrowed. `script.Pointers&PtrProtectedActivePlayer` no longer maps to TS `this.protect` (Player-level). It maps only to TS `script.pointers&PAP` (script-state). The forward-looking "is a protected script suspended" question is answered by `protectedScriptActive`, which preserves correctness because the pointer persists on `*ScriptState` across suspensions (TS strips and re-adds; goscape preserves; observable behavior identical for canAccess + walktrigger gates).

**Cadence:** `compressed_cadence` — combined spec+plan (Stage 2 doc), single TDD bundle on Sonnet via subagent-driven-development.

### Scenario B — Full TS port (~30-50 LOC)

**Production diff (broad strokes):**
- Add `Player.protect bool` field.
- Mirror TS lifecycle exactly:
  - At protected `runScript` entry (a new `(*Server).runScriptProtected` wrapper or extension to `runScript`): add pointer + set `p.protect = true` (Player.ts:2102-2103).
  - At runScript exit (post-Execute in `resumeOrFinish`): clear pointer from script + clear `p.protect` (Player.ts:2109-2114).
  - At suspend exit (in `resumeOrFinish` Suspended/PauseButton/CountDialog branches): restore `p.protect = protect` (Player.ts:2141). Requires threading `protect` through `resumeOrFinish` — currently the function doesn't know whether the script was originally protected.
  - At resume entry (in `handleResumePauseButton`, `handleResumeCountDialog`, `processActiveScripts`): re-add pointer + set `p.protect = true` if the script was originally protected.
- `CloseModal` clears `p.protect`, NOT script pointer.
- `protectedScriptActive` / `CanAccess` read `p.protect` instead of script pointer.

**Why this is bigger:** Threading `protect` through `resumeOrFinish` means `script.Init`'s `protect` arg can no longer be the only authority — the per-suspend "originally protected" flag must be persisted somewhere (either on the ScriptState as a separate `WasProtected bool` or on the Player). Test fixture migration is also broader: ~30+ ScriptState literals in test files set `Pointers: script.PtrProtectedActivePlayer` directly; under Scenario B, production callers can't replicate that pattern without going through the `runScript` wrapper.

**Cadence:** `subagent_driven_development` T1-T3 (T1 add Player.protect + threading; T2 update consumers; T3 update CloseModal + tests). Sonnet code-reviewer at end per `superpowers_code_reviewer_model`.

### Decision rule

- If Stage 1 G3 shows every "broken" row resolves with deletion of lines 728-730 → **Scenario A**.
- If Stage 1 G3 shows at least one consumer needs TS `this.protect` semantics that script-state-pointer can't provide → **Scenario B**.
- If Stage 1 surfaces unexpected drift → write **Scenario C** in Stage 2 spec.

Pre-Stage-1 hypothesis: Scenario A is sufficient. Stage 1 binds it.

## §4 Smoke binding

User-launched server + Java client per `smoke_test_server_handoff`.

**Path:**
1. Login fresh character on Tutorial Island.
2. Progress through tutorial to Magic Instructor (Terrova).
3. Cast Wind Strike on a chicken successfully.
4. Re-talk to Magic Instructor; the dialog "Do you want to go to the mainland?" appears.
5. Pick "Yes."
6. Server log captures: `tut_close`, `if_close`, `p_telejump` execute without `"P_TELEJUMP: script not protected"` error.
7. Java client renders the player at world coord (3222, 3222) level 0 — the `0_50_50_22_22` packed-coord (`level=0, square=50,50, local=22,22`) Lumbridge spawn destination.

**Pre-fix expected log:** `script "[label,tutorial_complete]": P_TELEJUMP: script not protected at pc=N` immediately after `if_close;`.
**Post-fix expected log:** clean execution; player teleports.

PRIMARY closes on smoke-bind per `cascade_theory_smoke_binding`. Adjacent residuals (e.g. `inv_clear`, `inv_add` failures, `~stat_reset_all` opcode gaps, `~initalltabs` gaps in `tutorial.rs2:303-327`) route to NAI-N+1 per `smoke_surfaces_adjacent_divergences`.

## §5 Non-goals

- Re-porting `(*Player).CanAccess` from scratch — only adjust if Stage 1 G3 surfaces a broken consumer.
- Porting the TS `runScript` per-invocation pointer-add/remove protocol unless Scenario B is chosen.
- Adjusting `clearWeakQueue` semantics (NAI-53 separate concern).
- Tutorial-Island content correctness post-teleport (`inv_clear`, `inv_add` ×16, `~stat_reset_all`, `~initalltabs` at `tutorial.rs2:303-327` are downstream content; if any abort, route forward).
- Renaming or refactoring `protectedScriptActive` — keep signature stable for blame-history continuity.
- Investigating whether NAI-52's "convergence" approach should be reverted entirely. Scenario A narrows it; Scenario B reverts it. Either choice closes NAI-111. The deeper "should goscape track Player.protect at all" question is out-of-scope.

## §6 Risk register

- **R1 — Stage 1 audit fabrication.** Pattern: `audit_subagent_fabrication`. Mitigation: controller verifies 3 random TS cites + 3 random goscape cites via direct Read/Grep before accepting findings. Drift table cross-checked against §1 preliminary read.

- **R2 — Hidden goscape consumer that needs TS `this.protect` semantics.** If Stage 1 G3 surfaces one — e.g. a CanAccess-style external check that fires while a non-suspended initial-execution protected script is in flight (where `p.activeScript == nil` but `s.Pointers&PAP` is set on the local state) — Scenario A breaks it. Mitigation: G3 enumeration is exhaustive; route to Scenario B if surfaced.

- **R3 — Smoke surfaces adjacent residuals downstream of `tutorial_complete`.** `tutorial.rs2:303-327` has `inv_clear, inv_add×16, ~stat_reset_all, ~initalltabs` and the trailing label flow. Any opcode missing or divergent will abort the script after `p_telejump` succeeds. Per `smoke_surfaces_adjacent_divergences`: PRIMARY (P_TELEJUMP gate restored) closes regardless; route adjacent surfaces to NAI-N+1.

- **R4 — Pre-existing test cohort pins wrong behavior.** `modal_close_test.go` has 3-4 tests landed by NAI-53 T3 that fail with the fix. Per `tracker_expected_value_premise_pretrace`: re-derive each test's expected behavior against TS reference at fix-author time before flipping the assertion. Don't blanket-invert.

- **R5 — `processWalktrigger` semantics change.** `interaction.go:~308` reads `protectedScriptActive`. Under Scenario A, `protectedScriptActive` returns true throughout an in-flight resumed protected script (currently transitions false after CloseModal mid-flight). This blocks walktrigger fires from reaching that script — matching TS `!this.protect` behavior. Stage 1 G3 audits this consumer specifically; if Scenario A's behavior change breaks any current test, Stage 2 plan addresses it.

- **R6 — Scenario A retains the NAI-52 convergence partially.** The narrowed convergence (script-state-only) is documented in DEVIATION-NAI-111-D1. Future TS divergences in the `this.protect` lifecycle (e.g. a TS change to runScript reentry semantics) won't propagate cleanly. Mitigation: D1 retire condition — port Scenario B if a future TS change requires it.

## §7 Pattern memories applied

- `investigation_subspec_cadence` — Stage 1 audit → Stage 2 fix shape selection → smoke binding.
- `audit_subagent_fabrication` — controller verification gate on G1/G2/G3 cites.
- `controller_preflight` — pre-fix grep+Read of every modal_close_test.go assertion against HEAD before plan-author dispatch.
- `tracker_expected_value_premise_pretrace` — invert test assertions only after re-deriving each test's expected behavior against TS reference.
- `superpowers_code_reviewer_model` — final reviewer Sonnet, never Opus.
- `smoke_test_server_handoff` — user-launched smoke; controller waits for binding.
- `smoke_surfaces_adjacent_divergences` — route post-teleport content residuals to NAI-N+1.
- `defensive_gate_doc_comment_label` — DEVIATION-NAI-111-D1 labeled in CloseModal doc-comment if Scenario A.
- `cascade_theory_smoke_binding` — close PRIMARY on smoke-bind.
- `dispatch_correct_reach_blocked` — close PRIMARY on TS-faithful semantic restoration regardless of downstream content gates.
- `bundle0_short_circuits_stage1_audit` — does NOT apply here. The §1 informal binding is intentionally not treated as a Stage-1 short-circuit; the user chose to run a formal audit subagent for fresh-eyes verification. Stage 2 spec-write proceeds only after Stage 1 findings land.
- `superpowers_clear_between_spec_and_impl` — after the Stage 2 plan is written, emit resume prompt and stop; user `/clear`s before implementer dispatch.

## §8 Cross-references

- TS source: `LostCityRS/Engine-TS/src/engine/entity/Player.ts` (closeModal at L741-794; runScript at L2094-2123; executeScript at L2125-2151). `LostCityRS/Engine-TS/src/engine/script/handlers/PlayerOps.ts:439` (P_TELEJUMP `checkedHandler(ProtectedActivePlayer, ...)`).
- Content: `LostCityRS/Content/scripts/tutorial/scripts/tutorial.rs2:296-327` (`[label,tutorial_complete]`). `LostCityRS/Content/scripts/tutorial/scripts/guides/magic_instructor.rs2:85` (`@tutorial_complete` jump). `LostCityRS/Content/scripts/interface_chat/scripts/chat.rs2:182-185` (`[label,multi2_header]`).
- Goscape divergence anchor (pre-fix): `modules/world/player_script.go:712-794` (CloseModal); `modules/world/player_script.go:296-304` (protectedScriptActive); `modules/world/resume_dialog.go:18-27` (handleResumePauseButton); `modules/world/script.go:107-153` (resumeOrFinish).
- NAI-53 T3 introducing the bug: commit `eee564a`, spec at `docs/superpowers/specs/2026-05-01-nai-53-closemodal-full-port-design.md`.
- NAI-52 convergence rationale: doc-comments at `modules/world/player_script.go:268-282` (CanAccess), `:296-304` (protectedScriptActive).
- NAI-133 refactor that renamed `s.Protect bool` → `Pointers&PtrProtectedActivePlayer`: commit `c641385`. Behavior-preserving rename; did not change the bug.
