# NAI-111 Stage 1 — protect-lifecycle audit Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Produce a Stage 1 findings doc that enumerates TS `Player.protect` lifecycle (G1), TS `script.pointers&ProtectedActivePlayer` lifecycle (G2), and every goscape consumer of either flag (G3) classified by site type plus correctness verdict. Output names a Stage 2 fix scenario (A minimal / B full TS port / C other) with citations the Stage 2 plan author will need.

**Architecture:** Stage 1 is a single Sonnet `general-purpose` audit subagent dispatch (Tasks 2-3) bracketed by controller pre-flight (Task 1) and controller verification (Task 4) per `audit_subagent_fabrication`. Findings doc is authored by the subagent at a controller-specified path; controller spot-checks 3 random TS cites + 3 random goscape cites by direct Read/Grep before accepting. Outcome routes to Stage 2 plan write (separate plan, fresh session).

**Stage 2 is NOT in this plan.** Per `superpowers_clear_between_spec_and_impl`: after Task 5 emits the Stage 2 resume prompt, the user `/clear`s and a fresh session authors `2026-05-XX-nai-111-stage-2-<scenario>.md` per the §scope statement.

**Tech Stack:** Go 1.26+ (no production code in Stage 1). Reference repos: `/home/owner/Code/github.com/LostCityRS/Engine-TS` (TS engine source). Spec doc: `docs/superpowers/specs/2026-05-09-nai-111-protect-lifecycle-investigation-design.md` at commit `b13394b`.

---

## File Structure

| File | Responsibility | Status |
|------|----------------|--------|
| `docs/superpowers/specs/2026-05-09-nai-111-protect-lifecycle-investigation-design.md` | Spec — read-only in Stage 1. | Read-only (committed at b13394b) |
| `docs/superpowers/findings/2026-05-09-nai-111-stage1-protect-lifecycle.md` | Stage 1 deliverable: G1 + G2 + G1×G2 drift table + G3 consumer decision table + §scope Stage 2 dispatch shape. Authored by audit subagent at T2; mutations after that step are controller-only. | Create at T2 |

No production files are modified in Stage 1. No tests are added.

---

## Pre-flight context for the controller

**This plan is controller-driven for Tasks 1, 3, 4, 5.** Task 2 dispatches one general-purpose subagent on Sonnet to perform the audit and author the findings doc.

**Audit-subagent risk profile:** the audit is open-ended cross-file analysis (TS lifecycle × goscape consumers × correctness mapping). Per `audit_subagent_fabrication`, expect the subagent to:
- Cite real line numbers most of the time but occasionally drift by ±5 lines or invent plausible line numbers.
- Sometimes confidently mis-attribute a TS site (e.g. claim a `pointerAdd` site that doesn't exist, or miss one that does).
- Frame the verdict in terms favorable to the pre-Stage-1 hypothesis even when evidence is mixed.

The Task 4 verification gate exists to catch these. Do not skip it.

**Reference paths (verified present at plan-write):**

```
/home/owner/Code/github.com/LostCityRS/Engine-TS/src/engine/entity/Player.ts
/home/owner/Code/github.com/LostCityRS/Engine-TS/src/engine/script/handlers/PlayerOps.ts
/home/owner/Code/github.com/LostCityRS/Engine-TS/src/engine/script/ScriptState.ts (or similar — subagent locates)
/home/owner/Code/github.com/zsrv/goscape/modules/world/player_script.go
/home/owner/Code/github.com/zsrv/goscape/modules/world/script.go
/home/owner/Code/github.com/zsrv/goscape/modules/world/resume_dialog.go
/home/owner/Code/github.com/zsrv/goscape/modules/world/interaction.go
/home/owner/Code/github.com/zsrv/goscape/modules/world/interaction_trigger.go
/home/owner/Code/github.com/zsrv/goscape/modules/world/modal_close_test.go
/home/owner/Code/github.com/zsrv/goscape/pkg/script/handlers_player.go
/home/owner/Code/github.com/zsrv/goscape/pkg/script/handlers_inv.go
/home/owner/Code/github.com/zsrv/goscape/pkg/script/runner.go
/home/owner/Code/github.com/zsrv/goscape/pkg/script/state.go
```

---

## Task 1: Controller pre-flight

**Why:** Per `controller_preflight`, verify spec premises against HEAD before audit dispatch.

**Files:** read-only.

- [ ] **Step 1.1: Verify HEAD is at the NAI-111 spec commit**

```bash
git log --oneline -3
```

Expected: top commit `b13394b spec(nai-111): protect-lifecycle investigation — Stage 1 audit + 2 fix scenarios`. Second commit `6f36fb2 chore(close): NAI-138 …`.

If HEAD has drifted, abort and re-spec.

- [ ] **Step 1.2: Verify the spec's pre-Stage-1 line refs at HEAD**

Run in parallel:

```bash
sed -n '725,735p' /home/owner/Code/github.com/zsrv/goscape/modules/world/player_script.go
```

```bash
sed -n '603,615p' /home/owner/Code/github.com/zsrv/goscape/pkg/script/handlers_player.go
```

```bash
sed -n '15,30p' /home/owner/Code/github.com/zsrv/goscape/modules/world/resume_dialog.go
```

Expected:
- `player_script.go:728-730` is the 3-line `if !p.delayed && p.activeScript != nil { p.activeScript.Pointers &^= script.PtrProtectedActivePlayer }` block inside `(*Player).CloseModal`.
- `handlers_player.go:606-613` is `func handlePTeleJump` with `requireProtectedActivePlayer(s, "P_TELEJUMP")` at the top.
- `resume_dialog.go:18-27` is `(*Server).handleResumePauseButton` calling `s.resumeOrFinish(p.activeScript, p)`.

If any line ref drifted, fix the spec inline before dispatching the audit. The plan stays valid as long as the audit prompt cites the spec by path (not by line numbers).

- [ ] **Step 1.3: Verify TS reference files exist**

```bash
ls -la /home/owner/Code/github.com/LostCityRS/Engine-TS/src/engine/entity/Player.ts /home/owner/Code/github.com/LostCityRS/Engine-TS/src/engine/script/handlers/PlayerOps.ts
```

Expected: both files present.

If absent, abort and ask the user to refresh the TS submodule before continuing.

- [ ] **Step 1.4: Confirm `docs/superpowers/findings/` directory exists**

```bash
ls -d /home/owner/Code/github.com/zsrv/goscape/docs/superpowers/findings/
```

If missing:

```bash
mkdir -p /home/owner/Code/github.com/zsrv/goscape/docs/superpowers/findings/
```

---

## Task 2: Dispatch audit subagent

**Why:** The audit subagent enumerates TS + goscape protect-flag sites, builds the G1×G2 drift table, classifies every goscape consumer, and authors the findings doc. Controller verifies in Task 4.

**Files:**
- Created by subagent: `docs/superpowers/findings/2026-05-09-nai-111-stage1-protect-lifecycle.md`.

- [ ] **Step 2.1: Dispatch the audit subagent**

Use a single `Agent` tool call with `subagent_type: "general-purpose"`, `model: "sonnet"`, and the following exact prompt:

```
You are the Stage 1 audit subagent for NAI-111 — a goscape sub-spec investigating
why P_TELEJUMP at `[label,tutorial_complete]` aborts with "script not protected"
on resume from a multi-choice dialog at the end of Tutorial Island.

The full spec lives at `docs/superpowers/specs/2026-05-09-nai-111-protect-lifecycle-investigation-design.md`.
READ IT FIRST. Section §1 contains a pre-Stage-1 informal binding hypothesis;
your job is to confirm or refute it via three exhaustive enumerations (G1, G2, G3)
followed by a decision table.

This is goscape — a Go port of LostCityRS/Engine-TS. The goscape repo is
`/home/owner/Code/github.com/zsrv/goscape/`. The canonical TS reference is
`/home/owner/Code/github.com/LostCityRS/Engine-TS/`. The Java client and Content
repos are NOT relevant for this audit.

═══════════════════════════════════════════════════════════════════════════
GOAL G1 — TS `Player.protect` lifecycle enumeration
═══════════════════════════════════════════════════════════════════════════

Read `LostCityRS/Engine-TS/src/engine/entity/Player.ts` exhaustively. Use:

  rg -n "this\.protect\b|\.protect = |\.protect;" /home/owner/Code/github.com/LostCityRS/Engine-TS/src/

(Then read each hit's surrounding context — typically ±5 lines.)

Enumerate every read or write of `this.protect` AND every cross-file read of
`<player>.protect` if any exist. Cross-file readers in `Engine-TS/src/engine/`
(World.ts, Inventory.ts, etc.) count.

For each site, capture:
  - Verbatim file:line and the exact line content.
  - Site classification (one of):
      * INIT_DEFAULT — field declaration / construct-time default.
      * SET_ON_RUN_ENTRY — runScript entry sets `this.protect = true`.
      * CLEAR_ON_RUN_EXIT — runScript exit clears `this.protect = false`.
      * RESTORE_AT_SUSPEND — executeScript suspend-exit restores
        `script.activePlayer.protect = protect`.
      * CLEAR_AT_RUN_END_VIA_POINTER_REMOVE — clears nested
        `script._activePlayer.protect = false` after pointerRemove.
      * CLEAR_ON_CLOSEMODAL — closeModal()'s `this.protect = false`.
      * CLEAR_ON_RESET — Player.reset() / cleanup paths.
      * READ_AS_GATE — read in a conditional that gates an action
        (e.g. canAccess, walktrigger, runScript reentry early-return).
      * READ_OTHER — anything else (e.g. logging, debug print).
  - TS execution phase the site fires in (initial-execution / suspended
    / resumed / external).

═══════════════════════════════════════════════════════════════════════════
GOAL G2 — TS `script.pointers & ProtectedActivePlayer` lifecycle
═══════════════════════════════════════════════════════════════════════════

Find every site that adds, removes, or reads `ScriptPointer.ProtectedActivePlayer`
(and its slot-2 sibling `ProtectedActivePlayer2`) across `Engine-TS/src/`. Use:

  rg -n "ProtectedActivePlayer\b" /home/owner/Code/github.com/LostCityRS/Engine-TS/src/
  rg -n "pointerAdd|pointerRemove|pointerGet" /home/owner/Code/github.com/LostCityRS/Engine-TS/src/engine/

For each site, capture:
  - Verbatim file:line and exact line content.
  - Site classification (one of):
      * POINTER_ADD — `script.pointerAdd(ProtectedActivePlayer)`.
      * POINTER_REMOVE — `script.pointerRemove(ProtectedActivePlayer)`.
      * POINTER_GET — `script.pointerGet(ProtectedActivePlayer)`.
      * CHECKED_HANDLER — `checkedHandler(ProtectedActivePlayer, ...)` or
        `checkedHandler([ProtectedActivePlayer, ...], ...)` wrapper around
        a script handler.
      * OTHER — anything else.

For every CHECKED_HANDLER site, list the wrapped handler name (e.g. P_TELEJUMP,
P_TELEPORT, INV_ADD, etc.). The reader cares which opcodes gate on this pointer
because those are the goscape consumers in G3.

═══════════════════════════════════════════════════════════════════════════
G1×G2 drift table
═══════════════════════════════════════════════════════════════════════════

Build a single table comparing `this.protect` and `script.pointers&PAP` at each
TS execution phase the spec hypothesizes drift:

  | Phase                              | this.protect          | script.pointers&PAP    |
  | initial-execution mid-flight       |                       |                        |
  | initial-execution post-closeModal  |                       |                        |
  | suspend exit                       |                       |                        |
  | resumed mid-flight                 |                       |                        |
  | resumed post-closeModal            |                       |                        |
  | runScript end (Finished/Aborted)   |                       |                        |

Fill each cell with `true` / `false` / `unchanged-from-prior-row` and cite the
specific Player.ts line that effects the transition. The spec §1 hypothesizes:

  - Mid-flight closeModal: `this.protect → false` BUT `script.pointers&PAP`
    UNCHANGED (the divergence that the goscape NAI-52 convergence collapses).
  - Suspend exit: `script.pointers&PAP → cleared` (Player.ts:2113);
    `this.protect → restored to original protect arg` (Player.ts:2141).
  - Resume entry: both → set (Player.ts:2102-2103).

Confirm or refute with verbatim cites.

═══════════════════════════════════════════════════════════════════════════
GOAL G3 — Goscape consumer audit + decision table
═══════════════════════════════════════════════════════════════════════════

Find every goscape consumer of the protect flag. Use:

  rg -n "PtrProtectedActivePlayer\b" /home/owner/Code/github.com/zsrv/goscape/pkg/ /home/owner/Code/github.com/zsrv/goscape/modules/
  rg -n "protectedScriptActive\b|requireProtectedActivePlayer\b" /home/owner/Code/github.com/zsrv/goscape/

Exclude `_test.go` files from the consumer audit (tests pin behavior; they're a
Stage 2 concern, not a correctness consumer). DO note any test files that
reference these tokens — Stage 2 will need to revisit them.

For each non-test consumer site, build a row in the decision table:

  | Site (file:line)                         | Consumer kind          | Currently reads     | Should map to TS         | Status |
  |------------------------------------------|------------------------|---------------------|--------------------------|--------|

Consumer kinds:
  * IN_FLIGHT_HANDLER_GATE — `requireProtectedActivePlayer(s, ...)` or direct
    `s.Pointers&PtrProtectedActivePlayer` check inside an in-flight script handler.
    These map to TS `script.pointers&PAP`.
  * EXTERNAL_CANACCESS — `(*Player).protectedScriptActive()` or read of
    `p.activeScript.Pointers&PAP` from outside an in-flight script (e.g.
    interaction.go's processWalktrigger). These map to TS `this.protect`.
  * MUTATOR — a site that WRITES `script.Pointers &^= PtrProtectedActivePlayer`
    or `|= PtrProtectedActivePlayer` outside of `script.Init`. These need their
    own per-site analysis.
  * OTHER — anything else.

Status verdict per row:
  * "correct" — the consumer's mapping holds in the absence of the line 728-730
    over-clear in CloseModal.
  * "broken (over-clear)" — the line 728-730 clear in CloseModal corrupts this
    consumer's read mid-flight.
  * "broken (under-restore)" — the consumer needs TS `this.protect` semantics
    that goscape's script.Pointers mapping doesn't provide (e.g. a check that
    fires while a non-suspended initial-execution protected script is running,
    where `p.activeScript == nil` but the in-flight `s.Pointers&PAP` is set).
  * "drift-tolerant" — both readings produce identical observable behavior on
    the Tutorial Island smoke path described in spec §4.

═══════════════════════════════════════════════════════════════════════════
Fix-shape recommendation
═══════════════════════════════════════════════════════════════════════════

Based on the G3 decision table, recommend ONE of:

  * Scenario A (minimal) — every "broken" row is correctable by deleting
    `modules/world/player_script.go:728-730`. No consumer needs Player.protect
    bool. Spec §3 Scenario A binds.
  * Scenario B (full TS port) — at least one "broken (under-restore)" row
    requires Player.protect bool with TS-faithful runScript-entry / runScript-exit
    / suspend-restore lifecycle. Spec §3 Scenario B binds.
  * Scenario C (other) — surfaced unexpected drift the spec didn't contemplate.
    Describe the new shape; the controller will write a new Stage 2 spec.

═══════════════════════════════════════════════════════════════════════════
DELIVERABLE
═══════════════════════════════════════════════════════════════════════════

Author the findings doc at this exact path:

  docs/superpowers/findings/2026-05-09-nai-111-stage1-protect-lifecycle.md

Use this exact section structure:

  # NAI-111 — Stage 1 findings: protect-flag lifecycle audit

  **Date:** 2026-05-09
  **Spec:** docs/superpowers/specs/2026-05-09-nai-111-protect-lifecycle-investigation-design.md (b13394b)
  **Cadence:** general-purpose Sonnet audit subagent per investigation_subspec_cadence.

  ## §G1 — TS Player.protect lifecycle

  <enumeration table per spec format>

  ## §G2 — TS script.pointers & ProtectedActivePlayer lifecycle

  <enumeration table per spec format>

  ## §G1×G2 — drift table

  <6-row drift table per spec format>

  ## §G3 — Goscape consumer decision table

  <decision table per spec format>

  ### G3 test-file inventory (advisory)

  <bullet list of `_test.go` files that reference the protect tokens — Stage 2 will revisit>

  ## §scope — Stage 2 dispatch shape

  **Recommendation:** Scenario A | Scenario B | Scenario C

  **Rationale:** <2-4 sentences citing specific G3 rows that drove the choice>

  **Stage 2 plan path (proposed):** docs/superpowers/plans/2026-05-09-nai-111-stage-2-<descriptor>.md

  ## §provenance

  - All TS cites verified by `Read`-ing the cited file:line.
  - All goscape cites verified by `rg`-ing the cited token.
  - Audit ran <approximate duration>; visited <approximate file count>.
  - Self-flagged uncertainty: <list any lines you're unsure about so the
    controller verification gate prioritizes them>.

═══════════════════════════════════════════════════════════════════════════
RULES OF CONDUCT
═══════════════════════════════════════════════════════════════════════════

1. Read full functions, not just grep hits. Function signatures and surrounding
   context matter for site classification.
2. Cite verbatim. Never paraphrase a line of source code.
3. If you can't decide a Status verdict for a G3 row, mark it "uncertain" and
   list it under §provenance self-flagged uncertainty. Don't guess.
4. Do NOT modify production code. Do NOT touch any `.go` file.
5. Do NOT run `go test`, `go build`, or any non-read command on the goscape repo.
6. The findings doc is the only file you create. Do not commit; the controller
   commits in a later task.
7. Return a concise summary (under 300 words) describing: scenario recommended,
   key drift-table row(s) that drove the recommendation, count of G3 rows by
   status, count of TS sites enumerated in G1/G2.
```

- [ ] **Step 2.2: Wait for the subagent to complete**

The subagent will return a single message summarizing what it did. Read the summary and the findings doc before proceeding to Task 3.

If the subagent reports it could not complete the audit (e.g. couldn't locate ScriptState.ts), do not retry — instead, abort to user and ask whether to switch the cadence to controller-direct (Scenario C in the brainstorm options).

---

## Task 3: Controller verification gate

**Why:** Per `audit_subagent_fabrication`, the audit's claims must be spot-checked before being treated as binding. Three TS cites + three goscape cites verified directly.

**Files:** read-only.

- [ ] **Step 3.1: Read the findings doc**

```
Read docs/superpowers/findings/2026-05-09-nai-111-stage1-protect-lifecycle.md
```

Note the §provenance "self-flagged uncertainty" list — start there if non-empty.

- [ ] **Step 3.2: Spot-check 3 random TS cites from G1+G2**

Pick 3 cites at random (e.g. one from each of G1's SET_ON_RUN_ENTRY, CLEAR_ON_CLOSEMODAL, READ_AS_GATE classifications, OR one from G2's POINTER_ADD, POINTER_REMOVE, CHECKED_HANDLER if G1 doesn't span all three). For each:

```bash
sed -n '<line-1>,<line+1>p' <cited-file>
```

Verify:
- The cited line content matches verbatim.
- The site classification is correct (e.g. a SET_ON_RUN_ENTRY cite IS at runScript entry, not somewhere else).
- The execution phase classification is plausible.

If any cite is wrong:
- Note it.
- Do NOT proceed to Task 4 yet. Re-dispatch the audit (back to Task 2) with the wrong cites called out in the prompt as red flags to re-verify, OR abort to user with the discrepancy if re-dispatch isn't warranted.

- [ ] **Step 3.3: Spot-check 3 random goscape cites from G3**

Pick 3 G3 rows at random (one from each consumer kind if possible: IN_FLIGHT_HANDLER_GATE, EXTERNAL_CANACCESS, MUTATOR). For each:

```bash
sed -n '<line-1>,<line+1>p' <cited-file>
```

Verify:
- The line content matches verbatim.
- The "Currently reads" column is correct (i.e. the site really does read what the audit claims).
- The "Should map to TS" column is reasonable given the consumer kind.

Same response protocol as Step 3.2 if any cite is wrong.

- [ ] **Step 3.4: Verify the §G1×G2 drift table against §1 hypothesis**

The pre-Stage-1 hypothesis in spec §1 predicts three specific drift cells:

  1. mid-flight `closeModal()`: `this.protect → false`; `script.pointers&PAP` UNCHANGED.
  2. suspend exit: `script.pointers&PAP → cleared` (Player.ts:2113); `this.protect → restored` (Player.ts:2141).
  3. resume entry: both → set (Player.ts:2102-2103).

Confirm the drift table either:
- Matches all three predictions verbatim (✓), OR
- Refutes one or more with a verbatim cite that explains the divergence (✓ — note as a hypothesis revision in the controller's mental model and in the post-Task-4 commit body).

If the drift table contradicts the hypothesis without offering a verbatim cite, re-dispatch the audit (back to Task 2) calling out the specific drift cell that needs better evidence.

- [ ] **Step 3.5: Cross-check `requireProtectedActivePlayer` consumers against the gate count**

The audit's G3 should enumerate every `requireProtectedActivePlayer(s, ...)` call site in the goscape codebase. Independently:

```bash
rg -n "requireProtectedActivePlayer\b" /home/owner/Code/github.com/zsrv/goscape/pkg/ /home/owner/Code/github.com/zsrv/goscape/modules/ | grep -v "_test.go" | grep -v "^.*//.*requireProtectedActivePlayer"
```

Count the production hits. The audit's G3 IN_FLIGHT_HANDLER_GATE row count should match this. If it doesn't, the audit missed a consumer; re-dispatch.

---

## Task 4: Determine Stage 2 scenario from audit

**Why:** The audit's recommendation is advisory. Controller picks the final scenario after weighing the verification gate result.

**Files:** read-only.

- [ ] **Step 4.1: Re-read the findings doc §scope section**

Capture: the audit's recommended scenario (A/B/C), the rationale, and the G3 rows it cites as load-bearing.

- [ ] **Step 4.2: Apply the spec §3 decision rule**

  - If every G3 "broken" row is "broken (over-clear)" with no "broken (under-restore)" or "uncertain" rows → **Scenario A**.
  - If at least one "broken (under-restore)" row exists → **Scenario B**.
  - If any row is "uncertain" or marked Scenario C → flag for user check.

- [ ] **Step 4.3: Reconcile with audit recommendation**

If the controller's reading and the audit's recommendation agree → proceed.

If they disagree:
- Document the disagreement in the Task 5 commit body.
- Default to the more conservative choice (B over A; C over either) unless the audit cites a specific binding row that resolves the conflict.

---

## Task 5: Commit findings + emit Stage 2 resume prompt

**Why:** Land the findings doc on main; emit a paste-ready resume prompt for the user's fresh-session Stage 2 plan write per `superpowers_clear_between_spec_and_impl` and `post_task_handoff`.

**Files:**
- Add: `docs/superpowers/findings/2026-05-09-nai-111-stage1-protect-lifecycle.md`.

- [ ] **Step 5.1: Verify clean working tree**

```bash
git status
```

Expected: only the findings doc is new/modified. If other files appear, investigate (per `feedback_subagent_wt_path` — audit subagents have been observed to write outside their stated file scope).

If a stray write exists, stash or remove it before committing.

- [ ] **Step 5.2: Commit the findings doc**

```bash
git add docs/superpowers/findings/2026-05-09-nai-111-stage1-protect-lifecycle.md
git commit --no-gpg-sign -m "$(cat <<'EOF'
investigation(nai-111): Stage 1 — <one-line scenario verdict>

Audit subagent (general-purpose, Sonnet) enumerated TS Player.protect
lifecycle (G1, <N> sites), TS script.pointers&ProtectedActivePlayer
lifecycle (G2, <M> sites), and goscape consumers (G3, <K> rows).
Verification gate per audit_subagent_fabrication: 3 random TS cites
+ 3 random goscape cites + drift-table cross-check + requireProtectedActivePlayer
gate-count cross-check — all <held | flagged: <details>>.

Drift summary:
- Mid-flight closeModal: <verdict per drift table>
- Suspend exit: <verdict>
- Resume entry: <verdict>

Decision: <Scenario A | Scenario B | Scenario C>. <One-sentence rationale
citing the load-bearing G3 row(s).>

No production code changes. Stage 2 plan to be authored in a fresh
session per superpowers_clear_between_spec_and_impl.

Closes memory: investigation_subspec_cadence (Stage 1 audit phase),
audit_subagent_fabrication (verification gate fired).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

Replace the angle-bracket placeholders with actual values from the findings doc and verification gate.

- [ ] **Step 5.3: Verify the commit landed**

```bash
git log --oneline -3
git status
```

Expected: top commit is the Stage 1 investigation commit; working tree clean.

- [ ] **Step 5.4: Emit the Stage 2 resume prompt**

Print the following block verbatim to the user, replacing `<…>` with concrete values:

```
NAI-111 Stage 1 closed at <commit-sha>. Verdict: Scenario <A|B|C> —
<one-line scenario summary>.

Findings doc: docs/superpowers/findings/2026-05-09-nai-111-stage1-protect-lifecycle.md
Spec: docs/superpowers/specs/2026-05-09-nai-111-protect-lifecycle-investigation-design.md (b13394b)

Resume in a fresh session:

  Read the findings doc and spec §3 (Scenario <A|B|C>) — write the
  Stage 2 plan at docs/superpowers/plans/2026-05-09-nai-111-stage-2-<descriptor>.md
  per the §scope statement and Scenario <A|B|C> production diff. Dispatch
  via subagent-driven-development per execution_mode_default.

  Pre-flight: re-grep + re-Read every Stage-1-cited line ref in the
  findings doc against HEAD before implementer dispatch (controller_preflight).
  Sonnet implementer + Sonnet reviewer per superpowers_code_reviewer_model.
  Post-merge git status on main per feedback_subagent_wt_path.

  Smoke binds at: Tutorial Island Magic Instructor "Yes, go to mainland"
  → server log shows tut_close + if_close + p_telejump executing without
  "P_TELEJUMP: script not protected" abort; player teleports to world
  coord (3222, 3222) level 0 (Lumbridge spawn).
```

Stop here. Do not begin Stage 2.

---

## Self-Review Checklist (controller-only, run after Task 5)

- [ ] G1 enumerated every TS `this.protect` site in `Engine-TS/src/`. No missed cross-file readers.
- [ ] G2 enumerated every TS `ProtectedActivePlayer` add/remove/get site + every `checkedHandler(ProtectedActivePlayer, ...)` wrapper. No missed handlers.
- [ ] G1×G2 drift table has 6 rows, each cell with verbatim cite or `unchanged-from-prior-row`.
- [ ] G3 decision table has every non-test goscape consumer; each row has Status verdict.
- [ ] §scope recommendation matches Spec §3 decision rule.
- [ ] Verification gate spot-checked 3 TS + 3 goscape cites + drift-table predictions + `requireProtectedActivePlayer` gate-count.
- [ ] Commit body cites verification gate result (held / flagged with details).
- [ ] Stage 2 resume prompt is paste-ready and quotes the actual Stage 1 closing commit SHA.
- [ ] No production `.go` files modified in Stage 1.
- [ ] No tests added in Stage 1.
- [ ] Working tree clean post-commit.
