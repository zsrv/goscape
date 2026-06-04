# NAI-124 — Damage magnitude investigation (Stage 1)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. Stage 1 is **controller-only** (no implementer / audit-subagent dispatch); Stage 2 lives in a separate plan written after Stage 1 binds.

**Goal:** Produce a Stage-1 findings doc that classifies each of the six risk-ranked surfaces (S1-S6 from spec §2) as DIVERGENT / ALIGNED / N/A, with binding TS-source line refs per verdict, and emits a Stage-2 scope statement.

**Architecture:** Controller-direct probe per `bundle0_short_circuits_stage1_audit` and `audit_subagent_fabrication` — controller reads TS sources directly, greps goscape at HEAD, and writes verdicts. No audit subagent dispatched. No production code changes in this plan. Final task commits the findings doc and emits a Stage-2-plan resume prompt.

**Tech Stack:** Go 1.26+; ripgrep for token discovery; read-only access to `LostCityRS/Engine-TS/` (TS reference) + `LostCityRS/Content/scripts/skill_combat/` (content cross-ref) + `LostCityRS/Content/data/src/scripts/configs/` (param.dat / npc.dat / obj.dat .pack sources).

**Spec:** `docs/superpowers/specs/2026-05-08-nai-124-damage-magnitude-investigation-design.md` (commit `f8c3401`).

**Predecessor:** NAI-123 close commit `b7c16b0`; residual #1 carry-forward (NAI-123 §7).

---

## File Structure

| Path | Role |
|---|---|
| `docs/superpowers/findings/2026-05-08-nai-124-stage1-findings.md` | Stage-1 deliverable: per-surface verdict (DIVERGENT / ALIGNED / N/A), TS-source line refs, refutation evidence per ALIGNED row, final §scope statement that defines Stage 2. Committed in Task 8. |

No production code files modified in this plan.

---

## Task 1: Sanity-check pre-Stage-1 state

**Why:** Per `controller_preflight`, verify the spec's HEAD references before probing.

**Files:** read-only.

- [ ] **Step 1.1: Verify HEAD is at NAI-124 spec commit**

```bash
git log --oneline -3
```

Expected: top line `f8c3401 spec(nai-124): damage magnitude investigation — pre-cap input reads huge`. Second line `b7c16b0 chore(close): NAI-123 …`.

- [ ] **Step 1.2: Verify the spec line refs at HEAD**

```bash
sed -n '49,52p' pkg/script/handlers_config.go
sed -n '108,115p;178,186p' pkg/objtype/paramtype.go
```

Expected:
- `handlers_config.go:51` is `s.PushInt(int(int32(iv)))` (the NAI-122 set-branch fix is intact at line 43); the *default* branch line 51 is `s.PushInt(int(pt.DefaultInt))`.
- `paramtype.go:111` declares `DefaultInt    uint32`.
- `paramtype.go:183` carries `//DefaultInt: -1, // this is -1 in js, default 0 here`.

If any line ref drifted, update the spec and this plan before proceeding.

- [ ] **Step 1.3: Create the findings doc skeleton**

Write `docs/superpowers/findings/2026-05-08-nai-124-stage1-findings.md` with this exact content:

```markdown
# NAI-124 — Stage 1 findings

**Date:** 2026-05-08
**Spec:** `docs/superpowers/specs/2026-05-08-nai-124-damage-magnitude-investigation-design.md` (`f8c3401`)
**Cadence:** controller-direct Stage 1 audit per `bundle0_short_circuits_stage1_audit`. No audit subagent dispatched.

## Per-surface verdicts

### S1 — paramLookup default branch sign-extension

**Verdict:** TBD-by-Task-2

### S2 — Remaining unsigned-int Params consumer sites

**Verdict:** TBD-by-Task-3

### S3 — `%com_maxhit` varp store/read sign discipline

**Verdict:** TBD-by-Task-4

### S4 — `Self.Stat()` return-type signedness

**Verdict:** TBD-by-Task-5

### S5 — TS-vs-goscape arithmetic int32-truncation discipline

**Verdict:** TBD-by-Task-6

### S6 — Strength-bonus aggregation chain end-to-end

**Verdict:** TBD-by-Task-7

## §scope — Stage 2 dispatch shape

TBD-by-Task-8
```

(The TBD markers are filled inline at each surface task.)

---

## Task 2: Probe S1 — paramLookup default branch sign-extension

**Why:** Spec §2 surface S1, highest-prior. Mirror of NAI-122 set-branch sign-ext fix on the default path.

**Files:**
- Read-only: `pkg/objtype/paramtype.go`, `pkg/script/handlers_config.go`, `LostCityRS/Engine-TS/src/engine/script/ParamHelper.ts`, `LostCityRS/Engine-TS/src/engine/config/ParamType.ts`.
- Modify: `docs/superpowers/findings/2026-05-08-nai-124-stage1-findings.md` (replace `TBD-by-Task-2`).

- [ ] **Step 2.1: Read TS ParamHelper.ts default branch**

```bash
find /home/owner/Code/github.com/LostCityRS/Engine-TS -name "ParamHelper.ts"
```

Read the file, locate `getIntParam` and `getStringParam`. Capture the exact lines that handle the "param not in map" path: does TS return `paramType.defaultInt` directly? What's its type in TS?

- [ ] **Step 2.2: Read TS ParamType.ts defaultInt declaration**

```bash
find /home/owner/Code/github.com/LostCityRS/Engine-TS -name "ParamType.ts"
```

Read the file. Locate the `defaultInt` field declaration. What's the JS default value when no wire `code 2` is encountered? (Spec hypothesizes `-1`.)

- [ ] **Step 2.3: Confirm goscape divergence at the default branch**

Re-Read `pkg/script/handlers_config.go:46-54`. Confirm: line 51 is `s.PushInt(int(pt.DefaultInt))` with NO `int32` cast. The set branch at line 43 IS sign-extended (`int(int32(iv))`).

- [ ] **Step 2.4: Decode `max_dealt` ParamType from cache**

Locate the param config source for `max_dealt`. Most likely path:

```bash
find /home/owner/Code/github.com/LostCityRS/Content -name "*.param" -path "*max_dealt*" 2>/dev/null
grep -rln "^\[max_dealt\]" /home/owner/Code/github.com/LostCityRS/Content/data/src/scripts/configs 2>/dev/null
grep -rln "\[max_dealt\]" /home/owner/Code/github.com/LostCityRS/Content 2>/dev/null | head -5
```

Locate the `[max_dealt]` definition. Capture: does it set `default = ...`? If yes, what value? If no, the unset default applies.

- [ ] **Step 2.5: Verify giant-rat NPC config does NOT set max_dealt explicitly**

```bash
grep -rln "^\[giant_rat\]" /home/owner/Code/github.com/LostCityRS/Content/data/src/scripts/configs 2>/dev/null
grep -rln "\[giant_rat" /home/owner/Code/github.com/LostCityRS/Content 2>/dev/null | head -5
```

Read the giant_rat NPC config. Does it set a `param=max_dealt,...` line? If yes, the param is set on the npc and S1's default branch isn't the bug for the rat case. If no, the default branch IS hit when the script reads `npc_param(max_dealt)` on a rat.

- [ ] **Step 2.6: Land S1 verdict in findings doc**

Edit `docs/superpowers/findings/2026-05-08-nai-124-stage1-findings.md`. Replace `TBD-by-Task-2` under `### S1` with:

```markdown
**Verdict:** <DIVERGENT | ALIGNED | N/A>

**TS reference:** `LostCityRS/Engine-TS/src/engine/script/ParamHelper.ts:<line>` returns `paramType.defaultInt` (JS Number, signed). `LostCityRS/Engine-TS/src/engine/config/ParamType.ts:<line>` declares `defaultInt: number = <value>`.

**Goscape divergence:** `pkg/script/handlers_config.go:51` does `s.PushInt(int(pt.DefaultInt))` with `pt.DefaultInt uint32` (no int32 cast); `int(uint32(0xFFFFFFFF))` = 4294967295 instead of -1.

**`max_dealt` ParamType wire default:** <unset = uses goscape NewParamType zero-init = 0 | set explicitly = uint32(0x...) = signed -...> at <path>:<line>.

**`giant_rat` NPC param.max_dealt:** <set = wire-encodes <value> | unset = falls through to ParamType default>.

**Production effect on bronze-dagger-vs-rat smoke:** <combine the above into the actual numeric path through `min(1, npc_param(max_dealt))`>.
```

- [ ] **Step 2.7: Determine if S1 alone explains "always 3"**

In the findings doc, add under S1:

```markdown
**Sole-contributor analysis:** S1 alone <can | cannot> produce damage = 3 every hit. <Reasoning>.
```

If S1 alone cannot produce the symptom (the spec hypothesizes this is the case), Stage 1 must continue past S1 to find the additional contributor. Do not short-circuit.

---

## Task 3: Probe S2 — Remaining unsigned-int Params consumer sites

**Why:** Spec §2 surface S2. NAI-122 may have missed sites that read `Params[k]` and convert via plain `int(iv)`.

**Files:**
- Read-only: all `.go` files matching `Params\[` pattern.
- Modify: `docs/superpowers/findings/2026-05-08-nai-124-stage1-findings.md`.

- [ ] **Step 3.1: Enumerate all Params[k] reader sites at HEAD**

```bash
rg -n "Params\[" pkg/ modules/ --type go | grep -v "_test.go"
```

Capture the full list. Already-known fixed sites (per NAI-122 `92ca5c4`): `pkg/script/handlers_config.go:38` (paramLookup set branch), `pkg/script/handlers_inv.go:249` (INV_TOTALPARAM), `modules/world/npc_hunt.go:291` (sumPlayerInvParam).

- [ ] **Step 3.2: For each site not in the NAI-122-fixed list, classify the int conversion**

For each unfixed site:
- Read the function context.
- Identify the conversion line: `int(...)` vs `int(int32(...))`.
- Identify whether the site is on a hot path for combat or any int-bonus aggregation.

- [ ] **Step 3.3: Verify the giant_rat / bronze_dagger param-read paths are fully covered**

For combat to work, the params read in `~equip_get_bonuses` (oc_param strengthbonus + 12 others) must all sign-extend correctly. Trace:
- `Content/scripts/player/scripts/equip.rs2:221-232` — 13 oc_param calls.
- These dispatch through `OpOcParam` → `handleObjParam` → `paramLookup` (already fixed at set branch line 43).

Verify by re-reading `pkg/script/handlers_config.go:455-462` (handleObjParam):

```bash
sed -n '450,465p' pkg/script/handlers_config.go
```

Confirm it delegates to `paramLookup(s, ot.Params, paramID)`.

- [ ] **Step 3.4: Land S2 verdict**

Edit findings doc. Under `### S2`:

```markdown
**Verdict:** <DIVERGENT | ALIGNED>

**Sites enumerated:** <count> via `rg "Params\[" pkg/ modules/ --type go | grep -v _test.go`.

**Sites already fixed by NAI-122 92ca5c4:** <list with line refs>.

**Sites unfixed:** <list with line refs and classification>.

**Production effect:** <does any unfixed site appear on the bronze-dagger-vs-rat smoke path? Y/N with reasoning>.
```

---

## Task 4: Probe S3 — `%com_maxhit` varp store/read sign discipline

**Why:** Spec §2 surface S3. If varp round-trip narrows to int32 on store but reads back wider/unsigned, %com_maxhit could blow.

**Files:**
- Read-only: `pkg/script/handlers_vars.go`, `modules/world/player.go` (or wherever `Player.Varp` / `Player.SetVarp` live).
- Modify: `docs/superpowers/findings/2026-05-08-nai-124-stage1-findings.md`.

- [ ] **Step 4.1: Locate Player.Varp and Player.SetVarp signatures**

```bash
rg -n "func \(\w+ \*?Player\) (Varp|SetVarp)\b" modules/world/ pkg/
```

Capture the exact return type of `Varp(id)` and the parameter type of `SetVarp(id, ...)`. The handler at `pkg/script/handlers_vars.go:75` writes `int32(s.PopInt())`; the read at line 51 does `int(s.Self.Varp(id))`.

- [ ] **Step 4.2: Verify int32 round-trip at varp boundary**

If `Varp(id)` returns `int32`, then `int(int32)` sign-extends (ALIGNED).
If `Varp(id)` returns `uint32`, `int(uint32)` zero-extends (DIVERGENT for negative-storage cases).
If `Varp(id)` returns `int` directly, depends on internal storage.

- [ ] **Step 4.3: Cross-check handlePushVarp against TS PUSH_VARP**

```bash
find /home/owner/Code/github.com/LostCityRS/Engine-TS -name "CoreOps.ts" -o -name "CoreOpsHandler.ts" 2>/dev/null
```

Read TS PUSH_VARP / POP_VARP. Confirm the int representation TS uses (typically signed Number).

- [ ] **Step 4.4: Land S3 verdict**

Edit findings doc. Under `### S3`:

```markdown
**Verdict:** <DIVERGENT | ALIGNED>

**Player.Varp signature:** `<func sig>` at `<path>:<line>`.
**Player.SetVarp signature:** `<func sig>` at `<path>:<line>`.

**Round-trip analysis:** <SetVarp narrows to <type>; Varp returns <type>; cast at handlePushVarp:51 is `int(<type>)` which <sign-extends | zero-extends>>.

**TS reference:** `LostCityRS/Engine-TS/<path>:<line>`. <ALIGNED|DIVERGENT>.
```

---

## Task 5: Probe S4 — `Self.Stat()` return-type signedness

**Why:** Spec §2 surface S4. If stat-level read corrupts via unsigned-byte misread, effective_strength explodes.

**Files:**
- Read-only: `pkg/script/handlers_player.go:240-265`, `modules/world/player.go` for `Player.Stat`.
- Modify: `docs/superpowers/findings/2026-05-08-nai-124-stage1-findings.md`.

- [ ] **Step 5.1: Locate Player.Stat signature and storage backing**

```bash
rg -n "func \(\w+ \*?Player\) Stat\b" modules/world/ pkg/
```

Capture return type. Find the underlying field (`p.levels[id]`, etc.) and its element type (`uint8` / `int` / `int32`).

- [ ] **Step 5.2: Cross-check TS Player.stat()**

Read `LostCityRS/Engine-TS/src/engine/entity/Player.ts` Stat / level read. Capture the type representation.

- [ ] **Step 5.3: Determine if a level-1 base stat could read as anything other than 1**

If storage is `uint8` and goscape correctly initializes level-1, the read should always be 1 for a fresh char. If storage is `int32` with a sign-bit corruption upstream, possible to read negative or huge.

- [ ] **Step 5.4: Land S4 verdict**

Edit findings doc. Under `### S4`:

```markdown
**Verdict:** <DIVERGENT | ALIGNED>

**Player.Stat signature:** `<sig>` at `<path>:<line>`.
**Storage:** `<type>` at `<path>:<line>`.

**TS reference:** `LostCityRS/Engine-TS/src/engine/entity/Player.ts:<line>`.

**Analysis:** <can|cannot> a fresh tutorial char's level-1 strength read produce a non-1 value via this surface.
```

---

## Task 6: Probe S5 — TS-vs-goscape arithmetic int32-truncation discipline

**Why:** Spec §2 surface S5. TS uses 32-bit Math.imul-style truncation in some opcodes; goscape may diverge.

**Files:**
- Read-only: `pkg/script/handlers_number.go:84-150` (Multiply / Divide / Scale / AddPercent), `pkg/script/handlers.go:614-628` (Add / Sub).
- Modify: `docs/superpowers/findings/2026-05-08-nai-124-stage1-findings.md`.

- [ ] **Step 6.1: Re-read each goscape arithmetic handler**

```bash
sed -n '84,150p' pkg/script/handlers_number.go
sed -n '614,628p' pkg/script/handlers.go
```

Note which handlers truncate to int32:
- `handleMultiply`: explicit `int32(lhs) * int32(rhs)` (truncates).
- `handleAdd` / `handleSub`: plain Go int arithmetic (no truncation).
- `handleScale`: `floorDiv(a*b, c)` (no int32 truncation on `a*b`).
- `handleAddPercent`: `lhs + (lhs*rhs)/100` (no truncation).

- [ ] **Step 6.2: Read TS counterparts**

Find TS `CoreOps` / `MathOps` for ADD, SUB, MULTIPLY, DIVIDE, SCALE, ADD_PERCENT. Capture the int-truncation discipline TS applies (e.g. `(a + b) | 0` = int32).

- [ ] **Step 6.3: Determine plausibility for combat-formula chain**

For the chain `combat_stat($effective_strength, $strengthbonus) * (...) / 640`:
- Maximum `$effective_stat` in a fresh char = 9.
- Maximum `$strengthbonus` post-NAI-122 sign-fix = +4 (bronze dagger) + 0 (other tutorial gear).
- `9 * (4 + 64) = 612`. No overflow; well below 2^31.

If no plausible overflow exists in the bronze-dagger-vs-rat smoke path, S5 is N/A for this investigation.

- [ ] **Step 6.4: Land S5 verdict**

Edit findings doc. Under `### S5`:

```markdown
**Verdict:** <DIVERGENT | ALIGNED | N/A>

**Goscape handlers:** <handleAdd | handleScale | …>: <truncates to int32 | does NOT truncate>.
**TS counterparts:** <ADD | SCALE | …> at `<path>:<line>`: <truncates to int32 | does NOT>.

**Plausibility:** <does any combat-formula chain on the bronze-dagger-vs-rat smoke path plausibly exceed 2^31 such that TS-vs-goscape truncation diverges? Y/N with arithmetic>.
```

---

## Task 7: Probe S6 — Strength-bonus aggregation chain end-to-end

**Why:** Spec §2 surface S6. Trace one full `equip_get_bonuses` execution to pin the running sum at each step.

**Files:**
- Read-only: `Content/scripts/player/scripts/equip.rs2:195-236`, `pkg/script/handlers_inv.go`, tutorial-starting-inv NPC config.
- Modify: `docs/superpowers/findings/2026-05-08-nai-124-stage1-findings.md`.

- [ ] **Step 7.1: Locate tutorial-starting worn-item set**

```bash
grep -rln "starting\|tutorial.*inv\|new_player" /home/owner/Code/github.com/LostCityRS/Content/scripts/_tutorial 2>/dev/null | head -5
```

Find the script that gives a fresh tutorial char their starting inventory + worn items. List each worn-slot obj id.

- [ ] **Step 7.2: Look up each worn item's strengthbonus param**

For each worn obj from Step 7.1, find its config and capture the `param=strengthbonus,X` entry (or absence).

```bash
grep -rln "^\[<obj_name>\]" /home/owner/Code/github.com/LostCityRS/Content/data/src/scripts/configs 2>/dev/null
```

- [ ] **Step 7.3: Manually compute running sum**

For each worn slot in order, accumulate `$strengthbonus = calc($strengthbonus + oc_param($obj, strengthbonus))`. Each `oc_param` goes through `paramLookup` set-branch (already NAI-122-fixed) or default-branch (S1; if S1 binds, this is contaminated).

- [ ] **Step 7.4: Land S6 verdict**

Edit findings doc. Under `### S6`:

```markdown
**Verdict:** <DIVERGENT | ALIGNED | depends-on-S1>

**Tutorial starting worn slots:** <list>.
**Per-slot strengthbonus:** <table>.
**Running-sum trace:** <step-by-step arithmetic>.
**Final $strengthbonus:** <value>.
**Final $melee_strength = combat_stat(9, $strengthbonus):** <value>.
**Final %com_maxhit = combat_maxhit($melee_strength):** <value>.

**Cross-reference to S1:** <if S1 binds and any oc_param call hits the default branch, the running sum is contaminated. Otherwise, S6 is independent.>
```

---

## Task 8: Write Stage-2 scope statement and commit

**Why:** Convert per-surface verdicts into a single Stage-2 dispatch shape; commit findings; emit Stage-2-plan resume prompt.

**Files:**
- Modify: `docs/superpowers/findings/2026-05-08-nai-124-stage1-findings.md`.

- [ ] **Step 8.1: Aggregate verdicts into scope statement**

Edit the `## §scope — Stage 2 dispatch shape` section. Pick the matching template:

**Template A: only S1 binds, sole contributor identified**

```markdown
**Single root cause: S1.** Stage 2 fixes paramLookup default branch sign-extension + (optionally) NewParamType DefaultInt initialization to match TS. Single bundle. Single Sonnet implementer.

**Stage 2 plan path:** `docs/superpowers/plans/2026-05-08-nai-124-stage2-paramlookup-default-signext.md` (to be written).
```

**Template B: S1 + one other binds (multi-contributor)**

```markdown
**Multi-contributor: S1 + S<N>.** <Description of additional contributor>. Stage 2 fixes both. Bundle split: <single bundle if mechanically correlated | bundle 1 = S1, bundle 2 = S<N> if independent>. <Bundle count> Sonnet implementer dispatches.

**Stage 2 plan path:** `docs/superpowers/plans/2026-05-08-nai-124-stage2-<descriptor>.md`.
```

**Template C: a different surface or non-leaf root cause binds**

```markdown
**Root cause refuted hypothesis-S1:** Actual root cause is <surface or new finding>. Stage 2 ports/fixes <description>.

**Stage 2 plan path:** `docs/superpowers/plans/2026-05-08-nai-124-stage2-<descriptor>.md`.
```

**Template D: no surface binds (refutation)**

```markdown
**No surface binds.** All hypotheses refuted. Investigation reopens with new surfaces; Stage 2 deferred. Issue probably lives in <speculation>; new spec required.
```

- [ ] **Step 8.2: Add cross-references and provenance footer**

Append to the findings doc:

```markdown
## Cross-references

- Spec: `docs/superpowers/specs/2026-05-08-nai-124-damage-magnitude-investigation-design.md` (`f8c3401`).
- NAI-123 close: `b7c16b0` (smoke that surfaced this residual).
- NAI-122 set-branch sign-ext fix: `92ca5c4`.
- TS sources read: <list of TS files with line refs touched in Tasks 2-7>.
- Goscape line refs verified at HEAD: <list of goscape files with line refs>.

## Provenance

Stage 1 conducted controller-direct per `bundle0_short_circuits_stage1_audit`. No audit subagent dispatched. All TS-source verdicts produced via direct Read of `LostCityRS/Engine-TS/`; no fabrication risk surface (`audit_subagent_fabrication`).
```

- [ ] **Step 8.3: Commit findings doc**

```bash
git add docs/superpowers/findings/2026-05-08-nai-124-stage1-findings.md
git commit --no-gpg-sign -m "$(cat <<'EOF'
investigation(nai-124): Stage 1 — <root-cause summary>

Controller-direct probe of spec §2 surfaces S1-S6.

Verdicts:
- S1 (paramLookup default branch sign-ext): <verdict>
- S2 (remaining unsigned Params consumers): <verdict>
- S3 (%com_maxhit varp round-trip): <verdict>
- S4 (Player.Stat() signedness): <verdict>
- S5 (arithmetic int32-truncation): <verdict>
- S6 (strength-bonus aggregation): <verdict>

Stage 2 scope: <one-line>.

No production code changes. Audit subagent skipped per
bundle0_short_circuits_stage1_audit (controller produced binding
TS-source diff at S<N>).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

Replace the angle-bracket placeholders with actual values per the findings doc.

- [ ] **Step 8.4: Verify the commit landed cleanly**

```bash
git log --oneline -3
git status
```

Expected: top commit is the Stage-1 investigation commit; working tree clean.

- [ ] **Step 8.5: Emit Stage-2-plan resume prompt**

Per `superpowers_clear_between_spec_and_impl` and `post_task_handoff`, controller writes a resume prompt for the user to paste in a fresh session. Format:

```
Resume NAI-124 — Stage 2 implementation.

Stage 1 closed at <commit-sha> with verdict: <one-line root-cause summary>.

Read the Stage 1 findings doc at `docs/superpowers/findings/2026-05-08-nai-124-stage1-findings.md`, then write the Stage 2 plan per the §scope statement. Path: `docs/superpowers/plans/2026-05-08-nai-124-stage2-<descriptor>.md`. Dispatch via subagent-driven-development per execution_mode_default.

Pre-flight: re-grep + re-Read every Stage-1-cited line ref against HEAD before implementer dispatch (controller_preflight). Sonnet implementer + Sonnet reviewer (superpowers_code_reviewer_model). Post-merge git status on main (feedback_subagent_wt_path).

Smoke binds at: damage distribution shifts from 3-every-time to ≥5/15 zero hits with non-zero hits at 0 or 1 only (spec §6).
```

---

## Self-Review Checklist (controller-only, run after Task 8)

- [ ] All six surfaces (S1-S6) have a verdict landed in the findings doc (not a `TBD-by-Task-N` placeholder).
- [ ] Each DIVERGENT verdict cites a TS-source line ref.
- [ ] Each ALIGNED verdict cites refutation evidence.
- [ ] §scope statement uses one of Templates A-D verbatim, with concrete substitutions.
- [ ] Stage-2 plan path is named (or marked deferred under Template D).
- [ ] Resume prompt is paste-ready and quotes the actual Stage-1 closing commit SHA.
