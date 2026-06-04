# NAI-139 Stage 1 — tutorial-completion cascade audit Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Produce a Stage 1 audit deliverable that classifies every opcode + proc transitively reachable from `LostCityRS/Content/scripts/tutorial/scripts/tutorial.rs2:296-330` as `WIRED | STUB | MISSING | UNKNOWN` against goscape's `pkg/script/` handler dispatch + cache loaders. Output names the Stage 2 fix scope (compressed / full / audit-clean / reframed-NAI-140).

**Architecture:** Single controller Pre-flight (Task 1) → parallel dispatch of 4 Sonnet `general-purpose` audit subagents B1+B2+B3+B4 in ONE Agent block (Task 2) per `dispatching-parallel-agents` → controller verification gate per `audit_subagent_fabrication` (Task 3) → controller-authored rollup + verdict (Tasks 4-5) → Stage 2 handoff (Task 6). No production code changes in Stage 1.

**Stage 2 is NOT in this plan.** Per `superpowers_clear_between_spec_and_impl`: after Task 6 emits the Stage 2 resume prompt, the user `/clear`s and a fresh session authors the Stage 2 plan (`docs/superpowers/plans/2026-05-09-nai-139-stage-2-<short-tag>.md`) per spec §3 routing decision.

**Tech Stack:** Go 1.26+ (no production code in Stage 1). Reference repos: `/home/owner/Code/github.com/LostCityRS/Content` (RuneScript content), `/home/owner/Code/github.com/LostCityRS/Engine-TS` (TS engine reference). Spec doc: `docs/superpowers/specs/2026-05-09-nai-139-tutorial-completion-cascade-design.md` at commit `68fa3fa`.

---

## File Structure

| File | Responsibility | Status |
|------|----------------|--------|
| `docs/superpowers/specs/2026-05-09-nai-139-tutorial-completion-cascade-design.md` | Spec — read-only in Stage 1. | Read-only (committed at 68fa3fa) |
| `docs/superpowers/findings/2026-05-09-nai-139-stage1-bundle-b1.md` | B1 deliverable (line-296-330 immediate ops). | Create at T2 |
| `docs/superpowers/findings/2026-05-09-nai-139-stage1-bundle-b2.md` | B2 deliverable (~stat_reset_all subtree). | Create at T2 |
| `docs/superpowers/findings/2026-05-09-nai-139-stage1-bundle-b3.md` | B3 deliverable (~initalltabs subtree). | Create at T2 |
| `docs/superpowers/findings/2026-05-09-nai-139-stage1-bundle-b4.md` | B4 deliverable (~update_all subtree). | Create at T2 |
| `docs/superpowers/specs/2026-05-09-nai-139-stage-1-audit.md` | Controller-merged unified defect list + Stage 2 routing verdict. | Create at T4 |

No production files modified in Stage 1. No tests added.

---

## Pre-flight context for the controller

**This plan is controller-driven for Tasks 1, 3, 4, 5, 6.** Task 2 dispatches FOUR `general-purpose` subagents on Sonnet IN PARALLEL (single Agent block, 4 tool-call content blocks).

**Audit-subagent risk profile** per `audit_subagent_fabrication`:
- Subagents may cite real line numbers most of the time but drift by ±5 lines or invent plausible line numbers.
- Subagents may confidently mis-attribute a goscape dispatch site (e.g. claim an opcode is wired in `handlers.go` when actually wired in `handlers_inv.go`, or vice versa).
- Subagents may frame WIRED status favorably even when the cited handler is a partial / stub implementation.

The Task 3 verification gate exists to catch these. Do not skip it.

**Reference paths verified present at plan-write (HEAD = `68fa3fa`):**

```
/home/owner/Code/github.com/LostCityRS/Content/scripts/tutorial/scripts/tutorial.rs2
/home/owner/Code/github.com/LostCityRS/Content/scripts/player/scripts/stat.rs2
/home/owner/Code/github.com/LostCityRS/Content/scripts/player/scripts/appearance.rs2
/home/owner/Code/github.com/LostCityRS/Content/scripts/login_logout/login.rs2
/home/owner/Code/github.com/zsrv/goscape/pkg/script/handlers.go
/home/owner/Code/github.com/zsrv/goscape/pkg/script/handlers_inv.go
/home/owner/Code/github.com/zsrv/goscape/pkg/script/opcode.go
/home/owner/Code/github.com/zsrv/goscape/pkg/script/handlers_inv_test.go
/home/owner/Code/github.com/zsrv/goscape/modules/world/script_test.go
```

Audit subagents must locate any other relevant files (other `pkg/script/handlers_*.go` files, cache loaders in `pkg/objtype/`, script-cache loader, interface-config loader) by `Grep` / `Glob`.

---

## Task 1: Controller pre-flight

**Why:** Per `controller_preflight`, verify spec premises against HEAD before audit dispatch. Re-derive the cascade enumeration independently to set up Task 3's cross-foot validation.

**Files:** read-only.

- [ ] **Step 1.1: Verify HEAD is at the NAI-139 spec commit**

```bash
git log --oneline -3
```

Expected top commit: `68fa3fa spec(nai-139): tutorial-completion cascade audit + fix design`. Second commit: `d0e88bd chore(nai-111): cleanup adjacent stale doc-comment + blank line`.

If HEAD has drifted, abort.

- [ ] **Step 1.2: Verify cascade source files exist and line ranges hold**

Run in parallel:

```bash
sed -n '296,330p' /home/owner/Code/github.com/LostCityRS/Content/scripts/tutorial/scripts/tutorial.rs2
sed -n '60,90p' /home/owner/Code/github.com/LostCityRS/Content/scripts/login_logout/login.rs2
sed -n '65,82p' /home/owner/Code/github.com/LostCityRS/Content/scripts/player/scripts/stat.rs2
sed -n '95,150p' /home/owner/Code/github.com/LostCityRS/Content/scripts/player/scripts/appearance.rs2
```

Expected:
- `tutorial.rs2:296` line reads `[label,tutorial_complete]`.
- `login.rs2:62` line reads `[proc,initalltabs]`.
- `stat.rs2:71` line reads `[proc,stat_reset_all]`.
- `appearance.rs2:98` line reads `[proc,update_all](obj $previous_weapon)`.

If any line ref has drifted, update spec and re-commit before proceeding.

- [ ] **Step 1.3: Independent baseline enumeration**

Controller writes a scratch list of unique opcodes + procs reachable from each bundle's surface (per spec §2 Bundle decomposition). This is the cross-foot baseline used in Task 3 Step 3.3.

Format the scratch list inline in the controller's working notes (do not commit; this is for verification only):

```
B1 baseline tokens (from spec §2 + tutorial.rs2:296-330):
  opcodes: tut_close, if_close, varp-set ($), inv_clear, inv_add
  (instances: 1 + 1 + 1 + 3 + 19 = 25)
  procs: (none — B1 is depth-0 on procs)

B2 baseline tokens (from stat.rs2:71-82 ~stat_reset_all + ~stat_reset bodies):
  opcodes: enum_getoutputcount, enum, stat, stat_base, stat_sub, stat_add, abs, sub, calc, while-loop, if-condition, def_int
  procs: ~stat_reset

B3 baseline tokens (from login.rs2:62-89 ~initalltabs body):
  opcodes: if_settab (×13), inv_transmit (×2), inv_getobj, lowmem-branch, ^tab_* constants
  procs: ~update_weapon_category, ~update_questlist
  (Read login.rs2:62-89 to confirm full token set)

B4 baseline tokens (from appearance.rs2:98-118 ~update_all body):
  opcodes: staffmodlevel, inv_getobj, stat_add, p_finduid, p_animprotect, %tutorial-read, ^constants
  procs: ~update_weight_equipment, ~update_bas, ~update_bonuses, ~update_weight, ~update_weapon_category (shared B3), ~player_combat_stat
```

Use this list to spot bundle-deliverable claims that fall outside their declared surface in Task 3.

- [ ] **Step 1.4: Verify goscape dispatch root files exist**

```bash
ls pkg/script/handlers*.go pkg/script/opcode.go
```

Expected: at least `handlers.go`, `handlers_inv.go`, `opcode.go` present. List any additional `handlers_*.go` files for subagents to use.

---

## Task 2: Parallel-bundle audit dispatch

**Why:** Per spec §2, Stage 1 audit decomposes into 4 independent subtree audits dispatched in parallel.

**Files:** subagents create the 4 deliverable docs at the paths in §File Structure.

- [ ] **Step 2.1: Dispatch B1 + B2 + B3 + B4 in a SINGLE Agent block**

Per `dispatching-parallel-agents`: ONE message containing 4 Agent tool calls. NOT 4 sequential messages.

All four use `subagent_type: general-purpose` on Sonnet (do NOT use Opus per `superpowers_code_reviewer_model`).

**B1 prompt (paste verbatim into the Agent call):**

```
Audit goscape's handler dispatch coverage for the immediate-cascade opcodes at LostCityRS/Content/scripts/tutorial/scripts/tutorial.rs2:296-330. Surface (do NOT descend into proc bodies — depth-0 on procs):

  - tut_close (opcode, no args, called as `tut_close();`)
  - if_close (opcode, no args, called as `if_close;`)
  - varp set: `%tutorial = ^tutorial_complete;` (varp write opcode)
  - inv_clear (opcode, 1 arg = inv-type id) — 3 call sites: inv, worn, bank
  - inv_add (opcode, 3 args = inv-type id, obj id, count) — 19 call sites (18 to inv + 1 to bank)

You must:

1. Read tutorial.rs2:296-330 verbatim with the Read tool. Confirm token list above.

2. Locate each opcode's dispatch in goscape. Start at pkg/script/handlers.go, pkg/script/handlers_inv.go, pkg/script/opcode.go. Use Grep + Read; do not infer "by analogy with neighboring opcode."

3. For each opcode, classify status: WIRED | STUB | MISSING | UNKNOWN.
   - WIRED = handler exists and looks complete (not a no-op, not a panic).
   - STUB = handler exists but is no-op / panic / TODO. Quote the stub line verbatim.
   - MISSING = no handler found. Document the grep commands you ran proving absence.
   - UNKNOWN = handler found but you cannot tell if it's complete (e.g., complex multi-branch dispatch). Explain.

4. For varp write specifically: locate the OpVarpSmall / VarPlayer write path. The runtime tutorial varp must reach the player's varp slot AND wire-emit. Cross-reference NAI-138's varp client-stream-load fix at commit 8fc06d5 — varp 4 (`tutorial`) and varp 173 (`option_run`) load from the same path.

5. For inv_clear / inv_add specifically: NAI-130/131/132/133/134 ported many inv ops. Verify these handlers actually clear/add for the runtime "inv", "worn", "bank" inv-type symbols. Check that the inv-type id resolution from symbol name works.

Output: write findings to docs/superpowers/findings/2026-05-09-nai-139-stage1-bundle-b1.md.

Required output format (Markdown table per row):

| token | kind | ts_ref | goscape_dispatch | status | evidence |
|-------|------|--------|------------------|--------|----------|
| tut_close | opcode | tutorial.rs2:297 | <file>:<line> or MISSING-IN-GOSCAPE | WIRED/STUB/MISSING/UNKNOWN | <verbatim quote or grep evidence> |

For STUB rows, include the verbatim stub line in the evidence column.
For MISSING rows, include the exact grep command(s) that returned no results.
For WIRED rows, include the case-branch line or registration-table entry showing dispatch.

Anti-fabrication mandate: every row cites file:line for both ts_ref AND goscape_dispatch (or explicit MISSING-IN-GOSCAPE). Use Read on cited lines. NO claims based on file existence alone.

Report under 800 words in your final response (the deliverable doc is the substantial artifact).
```

**B2 prompt (paste verbatim into the Agent call):**

```
Audit goscape's handler dispatch coverage for the ~stat_reset_all proc subtree.

TS source:
  - LostCityRS/Content/scripts/player/scripts/stat.rs2:71-82 — `[proc,stat_reset_all]` body and `[proc,.stat_reset]` body (the `.` prefix is the npc-context variant; ~stat_reset_all calls the player-context ~stat_reset which is ALSO in this file — locate by reading the surrounding region).

Surface (depth-N from `~stat_reset_all`):

  - ~stat_reset_all proc body: while-loop, $i counter, def_int, calc, enum_getoutputcount, enum, ~stat_reset call
  - ~stat_reset proc body: stat($stat), stat_base($stat), sub, abs, stat_sub, stat_add, if-condition (>0 / <0)
  - opcodes: enum_getoutputcount, enum, stat, stat_base, stat_sub, stat_add, abs, sub, calc, def_int, while, if (these may be runscript constructs implemented at the script-runner level, not as opcodes — distinguish carefully).

You must:

1. Read stat.rs2:65-82 with the Read tool. Confirm proc bodies.

2. Locate the script-cache loader in goscape (grep `pkg/` for "script.dat" / "ProcLookup" / similar). Verify ~stat_reset_all and ~stat_reset are loaded from the cache. If proc resolution by name uses a hash table, confirm the lookup keys would resolve.

3. For each opcode in the surface, locate dispatch in pkg/script/handlers*.go. enum / enum_getoutputcount likely live in a handlers_enum.go or similar. stat / stat_base / stat_add / stat_sub likely in handlers_stat.go. abs / sub / calc / def_int / while / if are language-level — confirm script runner supports them.

4. For each, classify WIRED | STUB | MISSING | UNKNOWN with evidence per the same format as B1.

5. Pay special attention to ~stat_reset semantics: `def_int $d = sub(.stat($stat), .stat_base($stat))` then conditional stat_sub/stat_add to drive current level back to base. Verify goscape stat_sub/stat_add support the 3-arg form (stat, amount, percent-or-flag — check exact arg signature against TS).

6. Note: stat_reset does NOT zero XP. It only resets temporary boost. Audit must verify goscape's stat_sub/stat_add do not accidentally also clear XP.

Output: write findings to docs/superpowers/findings/2026-05-09-nai-139-stage1-bundle-b2.md.

Use the same Markdown table format as B1. Anti-fabrication: file:line citations on every row, Read on every cited line, no fabrication based on neighboring opcode patterns.

Cross-bundle overlap declared: stat_add appears in B4 too. Just audit it — do not coordinate with other bundles.

Report under 800 words in final response.
```

**B3 prompt (paste verbatim into the Agent call):**

```
Audit goscape's handler dispatch coverage for the ~initalltabs proc subtree.

TS source:
  - LostCityRS/Content/scripts/login_logout/login.rs2:62-89 — `[proc,initalltabs]` body.
  - LostCityRS/Content/scripts/player/scripts/appearance.rs2 (or similar — locate) — `[proc,update_weapon_category]` body.
  - Locate `[proc,update_questlist]` by `grep -rn "\[proc,update_questlist\]" /home/owner/Code/github.com/LostCityRS/Content/scripts/`.

Surface (depth-N from `~initalltabs`):

  - ~initalltabs body: 13× if_settab (skills, quest, inventory, worn, prayer, magic, friends, ignore, logout, controls, options, music — incl. lowmem branch with options_ld + music_ld), 2× inv_transmit (inv → inventory:inv, worn → wornitems:wear), inv_getobj(worn, ^wearpos_rhand), ~update_weapon_category(...), ~update_questlist
  - ~update_weapon_category body: locate, audit transitively.
  - ~update_questlist body: locate, audit transitively.
  - opcodes: if_settab, inv_transmit, inv_getobj, lowmem (== check)
  - interface IDs: stats, questlist, inventory, wornitems, prayer, magic, friends, ignore, logout, controls, options, options_ld, music, music_ld
  - tab constants: ^tab_skills, ^tab_quest_journal, ^tab_inventory, ^tab_wornitems, ^tab_prayer, ^tab_magic, ^tab_friends, ^tab_ignore, ^tab_logout, ^tab_player_controls, ^tab_game_options, ^tab_musicplayer

You must:

1. Read login.rs2:60-90 with the Read tool. Confirm proc body.

2. Locate ~update_weapon_category and ~update_questlist proc definitions. Read each body.

3. For each opcode (if_settab, inv_transmit, inv_getobj, lowmem read), locate dispatch in goscape pkg/script/handlers*.go. Cite file:line. if_settab is the most consequential — verify it actually emits the IF_OPENSUB or equivalent server packet to bind the interface to the side-tab slot.

4. For interface IDs: locate goscape's interface-config loader (grep pkg/objtype/ for "if.dat" or "interface" or similar). Verify the production cache loads the 14 interface configs cited above. If any are missing, that's MISSING.

5. For tab constants (^tab_*): these are runscript-language constants in a `.constant` file. Locate the constant file (grep `engine.constant` or `tab.constant`). Verify each constant resolves and matches the expected sidebar slot index.

6. lowmem branch: `if (lowmem = true)` — verify goscape exposes a "lowmem" varp/property. lowmem is typically client-state (low-memory mode). Check what goscape returns for this.

Output: write findings to docs/superpowers/findings/2026-05-09-nai-139-stage1-bundle-b3.md.

Markdown table format same as B1. Anti-fabrication: file:line on every row.

Cross-bundle overlaps declared: inv_getobj appears in B1+B4; ~update_weapon_category appears in B4. Just audit; no coordination.

Report under 800 words in final response.
```

**B4 prompt (paste verbatim into the Agent call):**

```
Audit goscape's handler dispatch coverage for the ~update_all proc subtree.

TS source:
  - LostCityRS/Content/scripts/player/scripts/appearance.rs2:98-118 — `[proc,update_all](obj $previous_weapon)` body.

Surface (depth-N from `~update_all`):

  - ~update_all body:
    - staffmodlevel read
    - inv_getobj(worn, ^wearpos_rhand)
    - obj equality check (= poisoned_dagger_p)
    - 7× stat_add (hitpoints, attack, strength, defence, ranged, magic, prayer) with calc(255 - stat($skill)) — staff cheat branch
    - p_finduid(uid)
    - p_animprotect(^false)
    - ~update_weight_equipment proc call
    - ~update_bas proc call
    - ~update_bonuses proc call
    - ~update_weight proc call
    - %tutorial > ^newbie_combat_instructor_unequipping_items gate
    - ~update_weapon_category($previous_weapon) proc call (gated)
    - ~player_combat_stat proc call

  - opcodes: staffmodlevel, inv_getobj (shared B3), stat_add (shared B2), p_finduid, p_animprotect, %tutorial-read, varp comparison
  - procs (depth-1 only — descend ONE level into each, not full transitive into their callees):
    - ~update_weight_equipment — locate body, list its top-level opcodes/proc calls. Status: does the proc resolve in goscape's script cache? Are its top-level opcodes WIRED?
    - ~update_bas — same.
    - ~update_bonuses — same.
    - ~update_weight — same. NAI-136 ported this surface (commit 1b5ae51). Verify the ~update_weight call lines up with NAI-136's wiring (engine-side calculateRunWeight + OpUpdateRunWeight emit).
    - ~update_weapon_category — same.
    - ~player_combat_stat — same.

You must:

1. Read appearance.rs2:95-150 with the Read tool. Confirm ~update_all proc body.

2. For each opcode in the surface, locate dispatch in goscape pkg/script/handlers*.go. Cite file:line. p_finduid and p_animprotect are protect-related ops — NAI-111 (commits b96f22a + d0e88bd) modified the surrounding lifecycle. Verify p_finduid resolves the active player's uid and p_animprotect sets the animation-protect flag.

3. For each depth-1 proc, locate the proc definition in `LostCityRS/Content/scripts/`, Read the body, list top-level opcodes/calls, confirm each top-level opcode is WIRED in goscape. Do NOT recurse beyond depth-1 procs (avoid combinatorial blowup; Stage 2 / NAI-N+1 binds tail).

4. For the ~update_weight subproc specifically: cross-reference NAI-136 close at commit 1b5ae51 + 581ae2a. Verify (*Player).calculateRunWeight in modules/world/player_runweight.go is wired and that updating weight propagates correctly.

5. For the staff cheat branch (`if (staffmodlevel >= 3 & inv_getobj(worn, ^wearpos_rhand) = poisoned_dagger_p)`): verify staffmodlevel opcode exists and returns the correct value for a normal player (should be 0 for a fresh tutorial-completion player, so this branch never fires). If staffmodlevel is unwired, classify accordingly but note this branch is dormant for the smoke path.

6. For the %tutorial > ^newbie_combat_instructor_unequipping_items gate: check the `engine.constant` or `tutorial.constant` file for both ^newbie_combat_instructor_unequipping_items and ^tutorial_complete values. If ^tutorial_complete > ^newbie_combat_instructor_unequipping_items, then ~update_weapon_category WILL fire on tutorial completion. Record both numeric values.

Output: write findings to docs/superpowers/findings/2026-05-09-nai-139-stage1-bundle-b4.md.

Markdown table format same as B1. Anti-fabrication: file:line on every row.

Cross-bundle overlaps declared: stat_add appears in B2; inv_getobj in B1+B3; ~update_weapon_category in B3. Just audit independently.

Report under 800 words in final response.
```

- [ ] **Step 2.2: Wait for all 4 subagents to return**

Auto mode: subagents run; controller waits. Each returns a summary; the substantive deliverable is the Markdown file.

After all 4 return, verify all 4 deliverable docs exist:

```bash
ls -la docs/superpowers/findings/2026-05-09-nai-139-stage1-bundle-b*.md
```

Expected: 4 files present, all non-empty.

If any subagent failed to write the deliverable, re-dispatch THAT bundle individually (do not re-dispatch the others).

- [ ] **Step 2.3: Commit the 4 deliverables together**

```bash
git add docs/superpowers/findings/2026-05-09-nai-139-stage1-bundle-b1.md \
        docs/superpowers/findings/2026-05-09-nai-139-stage1-bundle-b2.md \
        docs/superpowers/findings/2026-05-09-nai-139-stage1-bundle-b3.md \
        docs/superpowers/findings/2026-05-09-nai-139-stage1-bundle-b4.md
git status
```

Expected: 4 new files staged.

```bash
git commit --no-gpg-sign -m "$(cat <<'EOF'
audit(nai-139): Stage 1 bundles B1+B2+B3+B4 returns

Per spec §2 parallel-bundle audit — 4 Sonnet general-purpose subagents
dispatched in single Agent block. Each subagent classified opcodes +
procs in its declared surface as WIRED/STUB/MISSING/UNKNOWN against
goscape pkg/script handler dispatch + cache loaders. Pre-merge per
audit_subagent_fabrication: controller verification gate at Task 3.

Bundle scope:
  B1 — line 296-330 immediate ops (tut_close, if_close, varp-set,
       inv_clear ×3, inv_add ×19)
  B2 — ~stat_reset_all subtree (~stat_reset, enum, enum_getoutputcount,
       stat, stat_base, stat_sub, stat_add, abs, sub, calc, while, if)
  B3 — ~initalltabs subtree (if_settab ×13, inv_transmit ×2, inv_getobj,
       ~update_weapon_category, ~update_questlist, lowmem, interface IDs,
       tab constants)
  B4 — ~update_all subtree (~update_weight_equipment, ~update_bas,
       ~update_bonuses, ~update_weight, ~update_weapon_category shared,
       ~player_combat_stat, p_finduid, p_animprotect, staffmodlevel,
       inv_getobj shared, stat_add shared, %tutorial gate)

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

Expected: commit succeeds.

---

## Task 3: Controller verification gate

**Why:** Per `audit_subagent_fabrication`, every audit deliverable gets controller-side spot-checks before binding to Stage 2 fix scope. Per `controller_preflight`, MISSING claims drive fix scope so they get 100% verification; WIRED claims get 20% sample.

**Files:** read-only verification.

- [ ] **Step 3.1: Read all 4 deliverables in parallel**

```
Read docs/superpowers/findings/2026-05-09-nai-139-stage1-bundle-b1.md
Read docs/superpowers/findings/2026-05-09-nai-139-stage1-bundle-b2.md
Read docs/superpowers/findings/2026-05-09-nai-139-stage1-bundle-b3.md
Read docs/superpowers/findings/2026-05-09-nai-139-stage1-bundle-b4.md
```

For each deliverable, count rows by status: WIRED, STUB, MISSING, UNKNOWN.

Record totals:

```
B1: <W>w / <S>s / <M>m / <U>u
B2: <W>w / <S>s / <M>m / <U>u
B3: <W>w / <S>s / <M>m / <U>u
B4: <W>w / <S>s / <M>m / <U>u
```

- [ ] **Step 3.2: 100% Read-verification on every MISSING / STUB row**

For each MISSING row across all 4 deliverables:
- Read the cited TS reference (`ts_ref` column). Confirm the opcode/proc is actually called there.
- Run the grep command(s) the subagent cited as evidence of absence in goscape. Confirm zero matches.
- If the row is fabricated (TS doesn't reference it OR goscape DOES have a handler), flag for re-dispatch.

For each STUB row:
- Read the cited goscape dispatch line. Confirm the verbatim stub quote (panic, TODO, no-op).
- If the cited line is actually a complete implementation, flag as fabricated.

If any MISSING / STUB row is fabricated:
- Re-dispatch THAT bundle's subagent with a prompt that quotes the contradiction. Do not re-dispatch all 4.
- Wait for re-dispatch return; re-run Step 3.1 + 3.2 on the corrected deliverable.

- [ ] **Step 3.3: 20% sample of WIRED rows + cross-foot total count**

For each bundle's WIRED rows, sample 20% randomly (round up; minimum 1 per bundle):
- Read the cited goscape dispatch line.
- Confirm dispatch path actually executes for the cited opcode (not just "the file exists" or "a switch case for SOMETHING is there").
- If WIRED claim is fabricated, flag bundle for re-dispatch.

Cross-foot total token count:
- Compare each bundle's row count to Task 1 Step 1.3 baseline.
- B1 baseline: ~5 unique opcodes. Subagent total should be in [5, 7] range (small over-count tolerable for detail).
- B2 baseline: ~12 unique opcodes + 1 proc. Subagent total in [13, 17].
- B3 baseline: ~4 unique opcodes + ~14 interface IDs + ~12 tab constants + 2 procs. Subagent total in [25, 35].
- B4 baseline: ~6 unique opcodes + 6 procs. Subagent total in [12, 16].

If a bundle's row count is far outside its range (e.g., B1 with 20 rows, or B4 with 4 rows), the subagent likely either over-counted (cross-bundle bleed) or skipped surface — re-dispatch with the discrepancy noted.

- [ ] **Step 3.4: Verification verdict**

If all 4 deliverables pass Steps 3.1-3.3, proceed to Task 4.

If any deliverable failed and required re-dispatch, document the re-dispatch reason in Task 4's rollup verdict.

---

## Task 4: Controller-merged audit rollup

**Why:** Spec §2 controller-responsibility 1: merge 4 deliverables into single defect list at the canonical audit-doc path.

**Files:** create `docs/superpowers/specs/2026-05-09-nai-139-stage-1-audit.md`.

- [ ] **Step 4.1: Author the merged audit doc**

Use the Write tool. Structure:

```markdown
# NAI-139 Stage 1 — tutorial-completion cascade audit (merged)

**Date:** 2026-05-09
**Spec:** docs/superpowers/specs/2026-05-09-nai-139-tutorial-completion-cascade-design.md @ 68fa3fa
**Plan:** docs/superpowers/plans/2026-05-09-nai-139-stage-1-cascade-audit.md
**Bundles:** B1 + B2 + B3 + B4 (4 parallel Sonnet general-purpose subagents)
**Verification:** 100% MISSING/STUB Read-verified, 20% WIRED sampled, cross-foot totals in range.

## §1 Defect summary

| Status | Count |
|--------|-------|
| WIRED  | <total across bundles, deduped> |
| STUB   | <total> |
| MISSING| <total> |
| UNKNOWN| <total> |

**Stage 2 fix scope:** <STUBs + MISSINGs estimated LOC>

## §2 Cross-bundle deduplication

Tokens that appeared in multiple bundles, with reconciled status:

| token | bundles | reconciled status | notes |
|-------|---------|-------------------|-------|
| stat_add | B2, B4 | <merged> | <if statuses differ, controller resolution> |
| inv_getobj | B1, B3, B4 | <merged> | |
| ~update_weapon_category | B3, B4 | <merged> | |
| stat | B2, B4 (if applicable) | <merged> | |
| stat_base | B2, B4 (if applicable) | <merged> | |

## §3 Defect list (sorted by severity)

### MISSING (Stage 2 priority 1 — must fix)

<table of MISSING rows from all bundles, deduped>

### STUB (Stage 2 priority 2 — verify if blocker)

<table of STUB rows>

### UNKNOWN (Stage 2 investigation candidates)

<table of UNKNOWN rows + recommended Stage 2 verification approach>

### WIRED (no action — reference only)

<compact per-bundle list, no full table>

## §4 Stage 2 routing decision

Per spec §3 decision tree:

- Total blockers (MISSING + STUB confirmed-as-blocker): <N>
- Estimated LOC for fix: <est>
- Verdict: [audit-clean | compressed | full | reframed-NAI-140]

Reasoning: <controller verdict>

## §5 Anti-fabrication ledger

- Re-dispatched bundles: <list, or "none">
- Reasons: <quote contradictions, or "n/a">
- WIRED sample size: <n>
- MISSING/STUB Read-verified: <n>
- Cross-foot result: <pass/fail per bundle>

## §6 Notes for Stage 2 plan author

<any context the Stage 2 plan-author needs that isn't obvious from the defect list — e.g., NAI-138 cache-loader pattern applies, NAI-136 weight-update pattern relevant, ~update_weapon_category gate condition resolved as fires/skips, etc.>
```

Fill every `<...>` placeholder with concrete content from the verified bundle deliverables. NO placeholders left.

- [ ] **Step 4.2: Commit the rollup doc**

```bash
git add docs/superpowers/specs/2026-05-09-nai-139-stage-1-audit.md
git status
```

Expected: 1 new file staged.

```bash
git commit --no-gpg-sign -m "$(cat <<'EOF'
chore(nai-139): Stage 1 audit verdict + Stage 2 routing decision

Controller-merged rollup of B1+B2+B3+B4 audit deliverables, deduped
across declared cross-bundle overlaps (stat_add B2/B4, inv_getobj
B1/B3/B4, ~update_weapon_category B3/B4). Cross-foot validation per
audit_arithmetic_correction_in_rollup: total claim count matches
controller-side independent enumeration within tolerance per bundle.

Stage 2 routing verdict: <audit-clean | compressed | full | reframed>
Total blockers: <N>
Estimated fix LOC: <est>

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

Replace the verdict placeholder fields with concrete values from §4.

Expected: commit succeeds.

---

## Task 5: Verify working tree clean

**Why:** Per `feedback_subagent_wt_path`, verify subagents did not write to main working tree.

- [ ] **Step 5.1: Run git status**

```bash
git status
```

Expected: nothing to commit, working tree clean (modulo the pre-existing sandbox-artifact untracked files: `.bash_profile`, `.bashrc`, `.claude/`, `.gitconfig`, `.gitmodules`, `.mcp.json`, `.profile`, `.ripgreprc`, `.vscode`, `.zprofile`, `.zshrc`, `test_typed_nil.go` — these were present at HEAD `68fa3fa`, not introduced by Stage 1).

If unexpected new files appear, investigate before proceeding (subagent may have accidentally written to wrong path).

- [ ] **Step 5.2: Verify final commit log**

```bash
git log --oneline -6
```

Expected (newest at top):
```
<sha>  chore(nai-139): Stage 1 audit verdict + Stage 2 routing decision
<sha>  audit(nai-139): Stage 1 bundles B1+B2+B3+B4 returns
68fa3fa spec(nai-139): tutorial-completion cascade audit + fix design
d0e88bd chore(nai-111): cleanup adjacent stale doc-comment + blank line
b96f22a fix(nai-111): Stage 2 — delete CloseModal protect over-clear (PRIMARY)
7baa79d plan(nai-111): Stage 2 — minimal delete over-clear (compressed cadence)
```

Note: the `plan(nai-139): Stage 1 ...` plan-doc commit (this file) goes in BEFORE Task 1 starts (committed by the controller as part of plan-write). Update the expected log to include it if so.

---

## Task 6: Stage 2 handoff

**Why:** Per `superpowers_clear_between_spec_and_impl`, end Stage 1 with a paste-ready resume prompt. User /clears; fresh session writes the Stage 2 plan with full context preserved on disk.

- [ ] **Step 6.1: Emit resume prompt for the user**

The controller writes (in chat, not to a file) a paste-ready Stage 2 resume prompt. Template:

```
Stage 2 dispatch for NAI-139.

**Audit verdict:** <audit-clean | compressed | full | reframed-NAI-140>
**Total blockers:** <N>
**Estimated fix LOC:** <est>
**Audit doc:** docs/superpowers/specs/2026-05-09-nai-139-stage-1-audit.md @ <commit sha>
**Spec:** docs/superpowers/specs/2026-05-09-nai-139-tutorial-completion-cascade-design.md @ 68fa3fa

**Routing path:**

[Pick one based on verdict:]

[A — audit-clean] Skip Stage 2 plan. User runs smoke per spec §4 to confirm theory-clean. If smoke PRIMARY-met → close NAI-139. If smoke fails → reframe to NAI-140 fresh investigation.

[B — compressed] Author combined Stage 2 plan + fix doc at docs/superpowers/plans/2026-05-09-nai-139-stage-2-<short-tag>.md per `compressed_cadence`. Single Sonnet implementer, single Sonnet code reviewer per `superpowers_code_reviewer_model`. Batch all blockers in one fix commit.

[C — full] Author Stage 2 plan at docs/superpowers/plans/2026-05-09-nai-139-stage-2-<short-tag>.md per `superpowers:writing-plans` skill. Then dispatch via `superpowers:subagent-driven-development`.

[D — reframed] Document the audit-clean / smoke-fail divergence; open NAI-140 fresh investigation spec with smoke output as binding signal.

**Pattern memories to apply at Stage 2 plan-write:**
- controller_preflight (re-grep audit assertions at HEAD before plan-write)
- spec_followup_tracker_freshness (re-Read every audit assertion)
- mock_recorder_field_naming_check (verify any plan-prescribed mock fields)
- plan_var_name_collision (mentally compile every code block)
- plan_type_name_grep (verify struct-literal type names)
- true_to_ts_gate (track every divergence with DEVIATION-NAI-139-D<N>)
- implementer_commit_content_verify (git show post-commit)
- feedback_subagent_wt_path (git status on main after each commit)
- close_commit_memory_trailer (Closes memory: <name> on close)

**Smoke after Stage 2 fix:** per spec §4 — fresh-account Tutorial Island playthrough → strict per-line verification of 6 PRIMARY criteria.
```

Replace all `<...>` placeholders with the concrete verdict values from Task 4 §4.

Output the prompt to the user as the final controller message of Stage 1.

- [ ] **Step 6.2: Mark Stage 1 complete**

No commit needed for Step 6.1 (chat-only). Stage 1 is now complete.

---

## Self-review checklist

After all tasks above are complete, the controller verifies:

- [ ] All 6 tasks marked complete.
- [ ] 4 bundle deliverable docs committed at `audit(nai-139)` commit.
- [ ] Merged audit doc committed at `chore(nai-139): Stage 1 audit verdict` commit.
- [ ] No production code modified.
- [ ] No tests added.
- [ ] Working tree clean (only sandbox-artifact untracked files remain).
- [ ] Resume prompt output to user.
