# NAI-138 Stage 1 — cs1 re-eval timing investigation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Execute Bundle 0 (controller pre-flight) + Bundle 1 (parallel Stage 1 audit) + synthesis for NAI-138 (cs1 re-eval timing on bare varp echo). Produce a binding fix-layer verdict (Engine port / Content trigger / Engine ad-hoc / Encoder defect) recorded in spec §6 and a Stage 2 handoff doc that names the fix layer plus citations needed for the Stage 2 plan.

**Architecture:** Investigation-only Stage. Bundle 0 is controller-executed read+grep (Tasks 1-3). Bundle 1 dispatches three Explore subagents in parallel against three independent codebases — Engine-TS, Client-Java #225, Content (Tasks 4-7). Synthesis verifies each substage's load-bearing claims via independent grep+Read per `audit_subagent_fabrication`, then routes to the synthesis matrix (spec §5). Handoff doc closes Stage 1 (Task 9).

**Stage 2 is NOT in this plan.** Bundle 2 fix tasks are data-dependent on Stage 1 verdict (TS source citations from 1.A, client trigger sites from 1.B, content trigger pattern from 1.C). After T9 emits the handoff, `/clear` and author a separate `2026-05-XX-nai-138-stage-2-<layer>.md` plan per `superpowers_clear_between_spec_and_impl`.

**Tech Stack:** Go 1.26+ (no production code in Stage 1). Reference repos: `$HOME/Code/github.com/LostCityRS/Engine-TS`, `$HOME/Code/github.com/LostCityRS/Client-Java`, `$HOME/Code/github.com/LostCityRS/Content`, `$HOME/Code/github.com/LostCityRS/RuneScriptKt`. Spec doc: `docs/superpowers/specs/2026-05-09-nai-138-cs1-reeval-investigation-design.md` at commit `cf5d6ed`.

---

## File Structure

| File | Responsibility | Status |
|------|----------------|--------|
| `docs/superpowers/specs/2026-05-09-nai-138-cs1-reeval-investigation-design.md` | Append Bundle 0 verdict (§6.1, §6.2) and Stage 1 synthesis verdict (§6.3) inline. | Modify (committed at cf5d6ed) |
| `docs/superpowers/handoffs/2026-05-09-nai-138-stage-1-binding.md` | Stage 1 close note: synthesis verdict, chosen fix layer, citations for Stage 2 plan author, paste-ready Stage 2 resume prompt. | Create at T9 |

No production files are modified in Stage 1. No tests are added.

---

## Pre-flight context for the implementer

**This plan is controller-driven, not implementer-driven.** Tasks 1-3 (Bundle 0) are direct controller reads using `grep` and `Read`; no subagent dispatched. Tasks 4-6 (Bundle 1) dispatch three parallel Explore subagents in a SINGLE message (per `dispatching-parallel-agents`). Task 7 verifies load-bearing claims independently. Tasks 8-9 author the synthesis verdict and handoff.

**Reference paths (verified present at plan-write):**

```
$HOME/Code/github.com/LostCityRS/Engine-TS/src/                  — TS engine source
$HOME/Code/github.com/LostCityRS/Client-Java/src/                — Java client #225 source
$HOME/Code/github.com/LostCityRS/Client-Java/ref/                — deobfuscation reference
$HOME/Code/github.com/LostCityRS/Content/scripts/                — RuneScript content scripts
$HOME/Code/github.com/LostCityRS/Content/scripts/interface_controls/scripts/player_controls.rs2  — click-path scripts
$HOME/Code/github.com/LostCityRS/Content/scripts/interface_controls/configs/player_controls.varp — option_run config
$HOME/Code/github.com/LostCityRS/RuneScriptKt/                   — RuneScript compiler
```

**Spec §6 placeholder shape (filled at T3, T8):**

```markdown
### 6.1 Engine-TS re-grep verdict
- TS emit sites at energy=0 transition: <enumerated file:line list>
- TS emit sites at click-toggle pathway: <enumerated file:line list>
- Per-pathway packet sequence comparison: <table or prose>
- Verdict: MISSING_TS_EMIT(site, signal) | TS_BARE_SETVAR_CONFIRMED | INCONCLUSIVE

### 6.2 `%v = %v` semantics verdict
- Compiler/runtime evidence: <citations>
- Self-write opcode behavior: <emits | no-op | conditional>
- Other Content uses of the idiom: <count, sample sites>
- Verdict: SELF_WRITE_EMITS_OP_VARP | SELF_WRITE_NOOP | INCONCLUSIVE

### 6.3 Stage 1 synthesis verdict
- 1.A verdict: <verbatim from substage output>
- 1.B verdict: <verbatim from substage output>
- 1.C verdict: <verbatim from substage output>
- Pre-Bundle-2 verification: <list of independently-verified citations>
- Synthesis matrix row matched: <row from spec §5 table>
- Chosen fix layer: <§7.1 | §7.2 | §7.3 | §7.4>
- Stage 2 plan handoff: <link to handoff doc>
```

---

### Task 1: Bundle 0 — Engine-TS comprehensive re-grep

**Files:**
- Read-only: `$HOME/Code/github.com/LostCityRS/Engine-TS/src/**`

- [ ] **Step 1: Comprehensive Engine-TS grep for run-varp / energy refresh emitters**

Run all of these in parallel (single message, multiple Bash calls):

```bash
rg -n "VarPlayerType\.RUN" $HOME/Code/github.com/LostCityRS/Engine-TS/src/
```

```bash
rg -n "setVar\b" $HOME/Code/github.com/LostCityRS/Engine-TS/src/ | rg -i "run"
```

```bash
rg -n "option_run" $HOME/Code/github.com/LostCityRS/Engine-TS/src/
```

```bash
rg -n "(processEnergy|updateEnergy|drainRunEnergy)" $HOME/Code/github.com/LostCityRS/Engine-TS/src/
```

```bash
rg -n "(IF_RESYNC|IF_OPENMAIN|IF_OPENMODAL|IF_OPENBOTTOM|IF_RUNSCRIPT|IF_SETSCRIPT|IF_RESETANIMS|VARP_SMALL|VARP_LARGE|VARP_RESET)" $HOME/Code/github.com/LostCityRS/Engine-TS/src/
```

```bash
rg -n "(pushVarp|writeVarp|flushVarp|setVarp)" $HOME/Code/github.com/LostCityRS/Engine-TS/src/
```

```bash
rg -n "this\.run\s*=" $HOME/Code/github.com/LostCityRS/Engine-TS/src/
```

- [ ] **Step 2: Read each hit's surrounding context**

For each matched site, `Read` the file with `offset=hit_line-10, limit=30` to capture surrounding context. Annotate which sites are:
- Run-varp WRITERS (`setVar(VarPlayerType.RUN, ...)`)
- Run-state WRITERS that may indirectly trigger varp emit (`this.run = X`)
- Refresh-signal EMITTERS unrelated to run varp (record but exclude from energy=0 sequence)

- [ ] **Step 3: Trace per-pathway packet sequences**

Two pathways to compare. For each, list the packets emitted in order:

**Pathway A — energy=0 transition:**
- Trigger: `this.runenergy <= 0` in `processEnergy` (or wherever).
- Trace: enumerate every `setVar` / `writeVarp` / `IF_*` emit reachable from this trigger.

**Pathway B — click-toggle:**
- Trigger: `[if_button,controls:com_5]` runs `if_close; ...; p_run; %option_run = %option_run;`.
- Trace: enumerate every server-side emit reachable from `if_close` opcode handler + `p_run` (PlayerOps.ts:1208) + the `%v = %v` opcode handler.

Tabulate side-by-side. Identify any packet emitted in B but NOT in A — that's the candidate `MISSING_TS_EMIT(site, signal)` finding.

- [ ] **Step 4: Determine 1.A verdict (Bundle 0 portion)**

One of:
- `MISSING_TS_EMIT(site, signal)` — TS emits packet X in pathway B that's absent from pathway A; goscape's `(*Player).updateEnergy` should port it. **Bundle 0 short-circuit triggers — skip Bundle 1, proceed to Stage 2 handoff with fix-layer = §7.1 Engine port.**
- `TS_BARE_SETVAR_CONFIRMED` — comprehensive grep confirms no missed emit; pathways A and B both end at a single `setVar(RUN, ...)` server-side. Bundle 1 dispatches as planned.
- `INCONCLUSIVE` — unable to fully trace one or both pathways from grep+read alone (e.g., dynamic dispatch through script handler tables); Bundle 1 dispatches with sharper substage prompts.

Hold the verdict in working memory; do not write to file yet (T3 commits the spec edit).

---

### Task 2: Bundle 0 — Click-path `%v = %v` semantic re-read

**Files:**
- Read-only: `$HOME/Code/github.com/LostCityRS/RuneScriptKt/**`
- Read-only: `$HOME/Code/github.com/LostCityRS/Engine-TS/src/**`
- Read-only: `$HOME/Code/github.com/LostCityRS/Content/scripts/**`

- [ ] **Step 1: Locate the `%v = %v` compilation target**

```bash
rg -n "(POP_VARP|PUSH_VARP|SET_VARP|VARP_BAS|varp.*assign)" $HOME/Code/github.com/LostCityRS/RuneScriptKt/
```

```bash
rg -n "(POP_VARP|PUSH_VARP|case 60|case 61|case 62)" $HOME/Code/github.com/LostCityRS/Engine-TS/src/engine/script/
```

The RuneScript opcode for `%v = X` is typically `POP_VARP_BAS`/`POP_VARP_OBJ` (or numeric equivalent). Identify:
- Compiler: which opcode `%option_run = %option_run` lowers to (likely a `PUSH_VARP option_run` followed by `POP_VARP option_run`).
- Runtime handler: whether the `POP_VARP` handler in `Engine-TS/src/engine/script/handlers/` calls `setVar` unconditionally, or short-circuits on equal-value.

- [ ] **Step 2: Read the `setVar` (or equivalent) implementation**

Find the function the handler calls. Determine whether it:
- Always emits `OpVarpSmall(id, value)` to the client
- Only emits if the value differs from the cached current value
- Has a "force-emit" / "always-resync" flag

- [ ] **Step 3: Grep Content for other `%v = %v` self-write usages**

```bash
rg -n "%(\w+)\s*=\s*%\1" $HOME/Code/github.com/LostCityRS/Content/scripts/
```

Catalog hits. If `%option_run = %option_run` is unique, the idiom may be ad-hoc; if there are several sibling uses, it's an established pattern worth porting carefully.

- [ ] **Step 4: Determine 1.C verdict (Bundle 0 portion)**

One of:
- `SELF_WRITE_EMITS_OP_VARP` — `%v = %v` does emit on the wire (handler calls `setVar` unconditionally). Supports content-fix or engine-double-emit fix layers.
- `SELF_WRITE_NOOP` — compiled-away or short-circuited at runtime; the `// resync varp` comment is misleading; click-path refresh comes from elsewhere (likely `if_close`).
- `INCONCLUSIVE` — unable to determine from compiler+runtime read alone.

Hold the verdict in working memory; T3 commits.

---

### Task 3: Bundle 0 — Append verdict to spec §6 + commit

**Files:**
- Modify: `docs/superpowers/specs/2026-05-09-nai-138-cs1-reeval-investigation-design.md` (replace §6 placeholder with concrete verdict)

- [ ] **Step 1: Edit spec §6 to fill in §6.1 and §6.2**

Replace the §6 placeholder block with concrete verdicts from T1 and T2. Format:

```markdown
## 6. Bundle 0 verdict

### 6.1 Engine-TS re-grep verdict

**TS emit sites at energy=0 transition (Pathway A):**
- `<file:line>` — `<emit description>`
- ...

**TS emit sites at click-toggle pathway (Pathway B):**
- `<file:line>` — `<emit description>`
- ...

**Per-pathway packet sequence:**

| Step | Pathway A (energy=0) | Pathway B (click-toggle) |
|---|---|---|
| 1 | `<packet>` | `<packet>` |
| 2 | (none) | `<packet>` |
| 3 | (none) | `<packet>` |

**Verdict:** `<MISSING_TS_EMIT(site, signal) | TS_BARE_SETVAR_CONFIRMED | INCONCLUSIVE>`

### 6.2 `%v = %v` semantics verdict

**Compiler evidence:** `<file:line>` — opcode lowered to `<OPCODE>`.

**Runtime handler:** `<Engine-TS file:line>` — handler `<calls setVar unconditionally | short-circuits on equal value | other>`.

**Other Content uses of the idiom:**
- Total hits: `<N>`
- Sample sites: `<file:line>`, `<file:line>`, ...

**Verdict:** `<SELF_WRITE_EMITS_OP_VARP | SELF_WRITE_NOOP | INCONCLUSIVE>`

### 6.3 Stage 1 synthesis verdict

(filled at T8)
```

If T1 returned `MISSING_TS_EMIT`, ALSO add a "**Bundle 0 short-circuit triggered**" subsection at the end of §6.1 stating: "Bundle 1 skipped; fix layer = §7.1 Engine port. Stage 2 plan to be authored against this finding."

- [ ] **Step 2: Commit the spec edit**

```bash
git add docs/superpowers/specs/2026-05-09-nai-138-cs1-reeval-investigation-design.md
git commit --no-gpg-sign -m "$(cat <<'EOF'
spec(nai-138): Bundle 0 verdict — TS re-grep + %v=%v semantics

Appends §6.1 (Engine-TS re-grep verdict) and §6.2 (%v = %v semantics
verdict) to the NAI-138 spec doc. Verdicts: <1.A verdict>; <1.C
verdict>. <If short-circuit: "Bundle 0 short-circuit triggered — Bundle
1 skipped, fix-layer = §7.1.">

Bundle 1 (parallel Stage 1 audit) <dispatches as planned | skipped per
short-circuit>.
EOF
)"
```

- [ ] **Step 3: Decide Bundle 1 dispatch**

If T1 verdict is `MISSING_TS_EMIT`, **skip to Task 9 (Stage 2 handoff)**. Bundle 1 is unnecessary because the fix layer is already determined.

Otherwise, proceed to Task 4.

---

### Task 4: Bundle 1 — Dispatch three parallel Explore subagents

**Files:**
- No file changes in this task (subagents return reports, not edits).

- [ ] **Step 1: Send a SINGLE message with three parallel Agent calls**

Use `subagent_type: "Explore"`. Each subagent gets `search breadth: "very thorough"`. Send all three in one message per `dispatching-parallel-agents`.

**Substage 1.A prompt (Engine-TS deep audit):**

```
Audit the LostCityRS/Engine-TS server source to determine whether goscape's
(*Player).updateEnergy energy=0 emit path is missing any TS-side packet
emitter that the click-toggle path implicitly receives.

Inputs: $HOME/Code/github.com/LostCityRS/Engine-TS/src/

The carryover hypothesis (from goscape NAI-137 close commit 1bc1800) asserts
that the ONLY engine-side VarPlayerType.RUN consumers in TS are
Player.ts:697-699 (energy=0 reset) and PlayerOps.ts:1208 (P_RUN handler),
both bare setVar calls with no extra refresh signal. THIS IS A LOAD-BEARING
ASSERTION I NEED YOU TO REFUTE OR CONFIRM COMPREHENSIVELY.

Method: trace two pathways' server-side emitted packet sequences:

Pathway A — energy=0 transition:
- Trigger: this.runenergy reaches 0 in processEnergy / drainRunEnergy /
  updateEnergy (whichever exists in TS).
- Enumerate every setVar / writeVarp / IF_* / OpVarp* / refresh-signal
  emit reachable from this trigger.

Pathway B — click-toggle:
- Trigger: [if_button,controls:com_5] in
  Content/scripts/interface_controls/scripts/player_controls.rs2:34-49
  runs: if_close; ...; p_run(^player_run_off OR ^player_run_on); ...;
  %option_run = %option_run;
- Enumerate every server-side emit reachable from:
  (a) the IF_CLOSE opcode handler in Engine-TS,
  (b) p_run (which calls into PlayerOps.ts:1208 P_RUN handler),
  (c) the POP_VARP (or whichever opcode %option_run = %option_run lowers to)
      handler.

Tabulate the two sequences side-by-side. Identify any packet emitted in B
but NOT in A.

Output one of three verdicts:

1. MISSING_TS_EMIT(site=<file:line>, signal=<packet name>) — TS emits a
   packet in pathway B that is absent from pathway A. Cite the exact file
   and line(s).

2. TS_BARE_SETVAR_CONFIRMED — comprehensive grep+trace confirms no missed
   emit. Both pathways emit identical packet sequences server-side.

3. INCONCLUSIVE — unable to fully trace one or both pathways (e.g.,
   dynamic dispatch through script handler tables). Specify what blocked
   the trace.

For each citation, give file:line. Do not paraphrase TS code — quote it.
Report under 500 words. The controller will independently verify your
citations before acting on the verdict.
```

**Substage 1.B prompt (Client-Java #225 cs1 re-eval audit):**

```
Audit the LostCityRS/Client-Java #225 source to determine what triggers
Component.script1 re-evaluation, specifically for buttontype=select
components.

Inputs:
- $HOME/Code/github.com/LostCityRS/Client-Java/src/
- $HOME/Code/github.com/LostCityRS/Client-Java/ref/  (deobfuscation
  reference; may help name unobfuscated symbols)

Background: in OSRS-style interface grammar, components have cs1 binding
scripts (script1, scriptComparator1, scriptOperand1) that the client
re-evaluates to compute display state (e.g., selecting which button graphic
to show). For buttontype=select, the binding scripts read varps via
pushvar,option_run; eq,N. The question: does receiving an OpVarpSmall(id,
value) packet trigger the client to re-evaluate cs1 bindings on
buttontype=select components, or does that re-eval require an additional
event (interface refresh, tab-switch, modal close, frame redraw)?

Method:
1. Locate the Component class (deobfuscated or obfuscated). Find the field
   reads for script1 / scriptComparator1 / scriptOperand1 (or equivalent
   numeric field IDs if obfuscated).
2. Identify all call sites that re-evaluate these fields. Common candidates:
   varp packet handler, interface-state events (IF_OPEN*, IF_CLOSE,
   IF_RESYNC), frame redraw / dirty-flag system, tab-switch handler.
3. For each re-eval call site, record the trigger event and any
   buttontype-specific filters.
4. Determine: when the client receives OpVarpSmall(173, 0) AS A STANDALONE
   PACKET (no surrounding interface event), does the run-toggle button
   (a buttontype=select component bound to option_run) re-evaluate its cs1
   and update its visual state?

Output one of three verdicts:

1. BARE_VARP_RE_EVALS — yes, OpVarpSmall on a varp bound to a cs1 script
   triggers component re-eval. If goscape smoke shows the button doesn't
   update, that points at a goscape encoder defect (wire bytes diverge
   from TS). Cite the dispatch site (file:line).

2. BARE_VARP_NO_RE_EVAL(needs_event=<event name>) — no, re-eval requires
   event X. Cite both the varp handler (which only updates the underlying
   varp) AND the event handler (which triggers re-eval). The fix point is
   "emit X alongside the varp."

3. INCONCLUSIVE — unable to determine due to obfuscation, missing
   deobfuscation reference, or path-dependent control flow. Specify what
   blocked the trace.

For each citation, give file:line. Quote relevant code. Report under 500
words. The controller will independently verify your citations before
acting on the verdict.
```

**Substage 1.C prompt (Content script audit):**

```
Audit the LostCityRS/Content scripts to characterize the
[varp,*] trigger pattern and the %v = %v self-write idiom's semantics.

Inputs:
- $HOME/Code/github.com/LostCityRS/Content/scripts/  (RuneScript
  source)
- $HOME/Code/github.com/LostCityRS/RuneScriptKt/  (compiler;
  determines what %v = %v lowers to)
- $HOME/Code/github.com/LostCityRS/Engine-TS/src/engine/script/
  handlers/  (runtime; determines whether the lowered opcode emits on the
  wire)

Background: Content/scripts/interface_controls/scripts/player_controls.rs2
lines 32 and 49 contain `%option_run = %option_run; // resync varp`
self-writes. The "resync varp" comment suggests the line forces a
client-bound OpVarpSmall emit. This sub-spec needs to know:

(a) What opcode does `%X = %X;` lower to in RuneScriptKt? (Likely a
PUSH_VARP followed by POP_VARP, but verify.)
(b) Does the runtime POP_VARP handler in Engine-TS unconditionally emit
OpVarpSmall to the client, short-circuit on equal-value, or have a
force-resync flag?
(c) Are there other [varp,X] triggers in Content/scripts/ that the engine
auto-fires when varp X changes? If so, document the pattern (one example
with file:line).
(d) Are there other %X = %X self-write usages across Content/scripts/?
Count them, list a representative sample.

Output one of three verdicts:

1. SELF_WRITE_EMITS_OP_VARP — %v = %v unconditionally emits OpVarpSmall on
   the wire. Cite the runtime handler (file:line). Supports content-fix or
   engine-double-emit fix layers.

2. SELF_WRITE_NOOP — compiled away by RuneScriptKt OR runtime
   short-circuits on equal-value. Cite the relevant line. The "resync varp"
   comment is misleading; click-path refresh comes from elsewhere.

3. INCONCLUSIVE — unable to determine. Specify what blocked the trace.

Also output:
- Whether [varp,X] auto-fired triggers exist in Content (yes/no, sample
  cite).
- Count of other %X = %X usages across Content + sample cites.

For each citation, give file:line. Quote relevant code. Report under 500
words. The controller will independently verify your citations before
acting on the verdict.
```

- [ ] **Step 2: Wait for all three subagent reports**

Subagents run in foreground. Once all three return, hold the verdicts in working memory; do not yet write to file.

---

### Task 5: Verify each substage's load-bearing claims (controller pre-trace)

**Files:**
- Read-only verification across the same reference repos.

Per `audit_subagent_fabrication` and §5 of the spec: controller MUST independently grep+Read each cited file:line BEFORE acting on a verdict. Stage 1 is the audit-stage where this matters most.

- [ ] **Step 1: Verify 1.A citations**

For each `file:line` cited in 1.A's verdict (especially any `MISSING_TS_EMIT(site, signal)` finding):
- `Read` the cited file at the cited line range.
- Confirm the cited code matches the verdict's description.
- If the verdict claims "TS emits packet X here" — confirm the line actually emits packet X.

If ANY citation fails verification, mark 1.A's verdict as `INCONCLUSIVE` and note the failed citation.

- [ ] **Step 2: Verify 1.B citations**

Same pattern for 1.B. For any `BARE_VARP_NO_RE_EVAL(needs_event=X)` finding, confirm both the varp handler AND the event handler citations exist and match descriptions.

If any citation fails, mark 1.B as `INCONCLUSIVE`.

- [ ] **Step 3: Verify 1.C citations**

Same pattern for 1.C. For any `SELF_WRITE_EMITS_OP_VARP` or `SELF_WRITE_NOOP` finding, confirm both compiler and runtime citations.

```bash
rg -n "%(\w+)\s*=\s*%\1" $HOME/Code/github.com/LostCityRS/Content/scripts/
```

Confirm 1.C's claimed count of `%X = %X` self-writes against this fresh grep.

If any citation fails, mark 1.C as `INCONCLUSIVE`.

- [ ] **Step 4: Record verified verdicts**

Hold the verified verdicts in working memory for T8. Note any citations that were marked unverifiable.

---

### Task 6: Conditional — Bundle 3 Template α (TS empirical smoke)

**Files:**
- No file changes in this task.

This task is a NO-OP unless any verdict from T5 is `INCONCLUSIVE` AND the inconclusive substage's verdict is load-bearing for fix-layer choice (per spec §5 last row).

- [ ] **Step 1: Decide whether to escalate**

If all three verdicts (1.A, 1.B, 1.C) are conclusive (any combination of non-`INCONCLUSIVE` values), **skip this task**.

If 1.A is `INCONCLUSIVE`, escalate per spec §9.1 Template α: TS empirical smoke can disambiguate by showing whether TS-engine's energy=0 path produces a visually de-toggling button or not.

If 1.B or 1.C is `INCONCLUSIVE` and that determines fix-layer choice, the synthesis matrix (spec §5) row "any INCONCLUSIVE → Bundle 3 Template α or β" routes here.

- [ ] **Step 2: Hand off TS empirical smoke to user**

Per `smoke_test_server_handoff` — sandbox cannot launch TS server. Send paste-ready instructions to user:

```
Bundle 3 Template α — TS empirical smoke escalation triggered.

Substage <1.A | 1.B | 1.C> returned INCONCLUSIVE; need empirical TS data
to bind the verdict.

Please:
1. Launch Engine-TS server: `cd $HOME/Code/github.com/LostCityRS/Engine-TS && bun start` (or `bun dev`)
2. Connect Java client #225 (same client used for goscape smoke)
3. Log in as a test character with full run energy.
4. Walk a long path (50+ tiles) until run energy depletes to 0.
5. Observe the run-toggle button at the moment energy reaches 0.
6. Report:
   (a) Does the button visually de-toggle? (yes/no)
   (b) If no, does it de-toggle when you click another tab and return?
   (c) If no, does it de-toggle on if_close events?

Outcome interpretation:
- TS button DOES de-toggle → goscape engine port has a hidden defect 1.A
  missed; re-dispatch 1.A with sharper prompt.
- TS button does NOT de-toggle → cascade attribution is to client/content
  layer; 1.B/1.C are decisive and any INCONCLUSIVE there blocks Stage 2.
```

- [ ] **Step 3: Wait for user smoke result, update verified verdicts**

Once user reports, update T5's working-memory verdicts. May force a re-dispatch of 1.A (sharper prompt) or a re-audit of 1.B/1.C.

---

### Task 7: Synthesis matrix routing

**Files:**
- No file changes in this task.

- [ ] **Step 1: Match verified verdicts against spec §5 synthesis matrix**

The matrix (verbatim from spec §5):

| 1.A | 1.B | 1.C | → Fix layer |
|---|---|---|---|
| `MISSING_TS_EMIT` | * | * | §7.1 Engine port (TS-faithful) |
| `TS_BARE` | `BARE_VARP_NO_RE_EVAL` | `SELF_WRITE_EMITS` | §7.2 Content trigger OR §7.3 engine double-emit (R4 deferral) |
| `TS_BARE` | `BARE_VARP_NO_RE_EVAL` | `SELF_WRITE_NOOP` | §7.3 Engine ad-hoc refresh (drift; deviation tag) |
| `TS_BARE` | `BARE_VARP_RE_EVALS` | * | §7.4 Goscape encoder defect |
| any `INCONCLUSIVE` | | | Bundle 3 Template α or β (Task 6) |

- [ ] **Step 2: Apply R4 deferral if matrix row 2 matches**

If matrix row 2 matches, choose between §7.2 and §7.3 per spec §10 R4:
- TS server smokeable by user → §7.2 Content trigger (verifies non-breakage on TS)
- TS server NOT smokeable → §7.3 engine double-emit (avoids breaking TS)

- [ ] **Step 3: Apply spec §12 last-bullet default if no row matches**

If no row matches (e.g., 1.A=`TS_BARE`, 1.B=`BARE_VARP_RE_EVALS`, 1.C=`SELF_WRITE_NOOP` — encoder defect AND no self-write emit), default to §7.4 (encoder defect) per "any `BARE_VARP_RE_EVALS` overrides 1.C." If even §7.4 doesn't fit (e.g., all signals say "OSRS client #225 has known bug"), default to §7.3 per spec §12 last bullet.

- [ ] **Step 4: Hold the chosen fix layer in working memory**

Verified verdicts + matched row + chosen fix layer all go to T8.

---

### Task 8: Append §6.3 synthesis verdict to spec + commit

**Files:**
- Modify: `docs/superpowers/specs/2026-05-09-nai-138-cs1-reeval-investigation-design.md` (replace §6.3 placeholder with concrete verdict)

- [ ] **Step 1: Edit spec §6.3 with concrete synthesis**

Replace the §6.3 placeholder with:

```markdown
### 6.3 Stage 1 synthesis verdict

**Substage verdicts (verified at T5):**
- 1.A: `<verbatim verdict>` — citations: `<file:line>`, `<file:line>`
- 1.B: `<verbatim verdict>` — citations: `<file:line>`, `<file:line>`
- 1.C: `<verbatim verdict>` — citations: `<file:line>`, `<file:line>`

**Pre-Bundle-2 verification (per audit_subagent_fabrication):**
- 1.A citations independently verified: `<count>` of `<count>` PASS
- 1.B citations independently verified: `<count>` of `<count>` PASS
- 1.C citations independently verified: `<count>` of `<count>` PASS

**Synthesis matrix row matched:** Row `<N>` (`<row description>`).

**R4 deferral applied:** `<yes — chose §7.X because <reason> | no — single layer matched>`.

**Chosen fix layer:** `<§7.1 Engine port | §7.2 Content trigger | §7.3 Engine ad-hoc refresh | §7.4 Goscape encoder defect>`.

**Stage 2 plan handoff:** `docs/superpowers/handoffs/2026-05-09-nai-138-stage-1-binding.md`
```

- [ ] **Step 2: Commit the spec edit**

```bash
git add docs/superpowers/specs/2026-05-09-nai-138-cs1-reeval-investigation-design.md
git commit --no-gpg-sign -m "$(cat <<'EOF'
spec(nai-138): Stage 1 synthesis verdict — fix-layer = <§7.X>

Appends §6.3 (Stage 1 synthesis verdict) to the NAI-138 spec doc.
Substage verdicts: 1.A=<X>, 1.B=<Y>, 1.C=<Z>. Synthesis matrix row
<N> matches; chosen fix layer is §7.X (<layer name>).

All <count> load-bearing citations independently verified per
audit_subagent_fabrication. Stage 2 plan handoff queued in
docs/superpowers/handoffs/2026-05-09-nai-138-stage-1-binding.md.
EOF
)"
```

---

### Task 9: Stage 2 handoff doc + commit

**Files:**
- Create: `docs/superpowers/handoffs/2026-05-09-nai-138-stage-1-binding.md`

- [ ] **Step 1: Author the handoff doc**

```markdown
# NAI-138 Stage 1 → Stage 2 handoff

**Stage 1 close commit:** `<sha from T8>`
**Spec doc:** `docs/superpowers/specs/2026-05-09-nai-138-cs1-reeval-investigation-design.md`
**Date:** 2026-05-09

## Synthesis verdict

- 1.A: `<verbatim>`
- 1.B: `<verbatim>`
- 1.C: `<verbatim>`
- Matched matrix row: `<N>`
- Chosen fix layer: `<§7.X>`

## Citations needed for Stage 2 plan author

For Engine port (§7.1):
- TS source line range with the missed emit: `<file:line-range>`
- Existing goscape sibling site to mirror: `<file:line>`
- Test seam (which test extends): `<test name and file:line>`

For Content trigger (§7.2):
- Sibling [varp,X] trigger to use as template: `<file:line>`
- Target file: `Content/scripts/interface_controls/scripts/player_controls.rs2`
- Suggested trigger body: `<concrete RuneScript>`

For Engine ad-hoc refresh (§7.3):
- Signal to emit (per 1.B): `<packet name>`
- Existing goscape encoder for that packet: `<file:line>`
- Deviation tag: `NAI-138-DEV-RUN-VARP-<SIGNAL-SUFFIX>`
- Doc-comment template: `// (goscape deviation; TS does not emit this — see NAI-138 spec §7.3)`

For Goscape encoder defect (§7.4):
- Encoder under suspicion: `<file:line>`
- Reference encoder (TS or rsbuf): `<file:line>`
- Suspected divergence: `<bit-order | field-order | value-range | other>`

(Fill ONLY the section corresponding to the chosen fix layer; mark others "n/a".)

## Stage 2 plan author resume prompt

```
/clear

Author the NAI-138 Stage 2 implementation plan at
docs/superpowers/plans/2026-05-XX-nai-138-stage-2-<layer>.md.

Predecessor: Stage 1 closed at <sha from T8>; verdict in
docs/superpowers/specs/2026-05-09-nai-138-cs1-reeval-investigation-design.md
§6.3. Handoff doc:
docs/superpowers/handoffs/2026-05-09-nai-138-stage-1-binding.md.

Fix layer: §7.<N> (<layer name>).

Use the citations in the handoff doc to write a single-task TDD plan
(red → green → commit) per the layer's spec section. Include smoke
handoff + Bundle 3 conditional templates from spec §9.

Cadence: subagent-driven-development per execution_mode_default.
```

## Smoke handoff at Stage 2 close

Per spec §8, goscape smoke binds Stage 2 close. Decision tree:
- Button visually de-toggles at energy=0 → close NAI-138 PRIMARY met
- Button stays stuck-on, click toggles still work → Bundle 3 Template α/β
- New regression → revert + open NAI-138 stretch

## Compressed-cadence eligibility

Per spec §10 R5 + `compressed_cadence`: if the Stage 2 fix is ≤~15 LOC AND
only one layer is touched, the Stage 2 plan author MAY collapse spec+plan
into a single doc and skip formal review. Estimate before authoring:

- §7.1 Engine port: typically 5-15 LOC (one TS-faithful emit added to one
  function); compressed-eligible if simple.
- §7.2 Content trigger: typically 1-5 lines of RuneScript in one file;
  compressed-eligible.
- §7.3 Engine ad-hoc refresh: 5-15 LOC with deviation tag + memory entry;
  compressed-eligible if signal shape is simple.
- §7.4 Encoder defect: variable; usually NOT compressed-eligible (encoder
  changes carry roundtrip-test obligation).
```

- [ ] **Step 2: Create handoffs directory if needed and write the file**

```bash
mkdir -p docs/superpowers/handoffs/
```

(Then `Write` the handoff doc per Step 1 content.)

- [ ] **Step 3: Commit the handoff**

```bash
git add docs/superpowers/handoffs/2026-05-09-nai-138-stage-1-binding.md
git commit --no-gpg-sign -m "$(cat <<'EOF'
chore(nai-138): Stage 1 handoff — fix-layer = <§7.X>

Stage 1 close handoff for NAI-138. Synthesis verdict (spec §6.3):
fix-layer = §7.X (<layer name>). Stage 2 plan author should use the
citations in this handoff to write a single-task TDD plan against the
chosen layer.

Closes Stage 1; opens Stage 2.
EOF
)"
```

- [ ] **Step 4: Emit paste-ready user resume prompt**

End-of-session message to user (per `post_task_handoff`):

```
NAI-138 Stage 1 closed at <handoff sha>. Synthesis: fix-layer = §7.X
(<layer name>).

To continue with Stage 2:

1. /clear (per superpowers_clear_between_spec_and_impl)
2. Paste:

   Author the NAI-138 Stage 2 implementation plan at
   docs/superpowers/plans/2026-05-XX-nai-138-stage-2-<layer>.md per the
   handoff at
   docs/superpowers/handoffs/2026-05-09-nai-138-stage-1-binding.md.
   Fix layer is §7.X. Use citations in the handoff. Cadence:
   subagent-driven-development per execution_mode_default.
```

---

## Stage 1 close criteria

All of the following before declaring Stage 1 done:
- T3 commit lands with §6.1 + §6.2 verdicts.
- T8 commit lands with §6.3 synthesis verdict (or T3 records Bundle 0 short-circuit and T8 is skipped).
- T9 handoff doc exists and is committed.
- Chosen fix layer is one of §7.1/§7.2/§7.3/§7.4 (not "TBD").
- All load-bearing citations in the synthesis verdict are independently verified per T5.
- Paste-ready Stage 2 resume prompt is emitted to user.

Stage 1 does NOT include any production code changes, any tests, or any goscape smoke. Stage 2 plan owns all of those.
