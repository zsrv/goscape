# NAI-139 — Tutorial-completion cascade audit + fix

**Date:** 2026-05-09
**Type:** Investigation sub-spec (per `investigation_subspec_cadence`)
**Predecessor:** NAI-111 (closed PRIMARY 2026-05-09 at commits `3ee0213` + `d2adc2d`)
**Tech Stack:** Go 1.26+ (per `go_version` memory)
**TS source canonical path:** `LostCityRS/Engine-TS` (engine) + `LostCityRS/Content` (scripts) (per `ts_source_canonical_path`)

## §1 Scope & success criteria

### Symptom framing

NAI-111 restored the `P_TELEJUMP` gate at `[label,tutorial_complete]`. Magic Instructor "Yes, go to mainland" now executes `tut_close;if_close;p_telejump` cleanly and the player teleports to Lumbridge spawn (3222, 3222, level 0). PRIMARY smoke-bound at NAI-111 close.

NAI-111's smoke did NOT extend past the teleport to verify the post-teleport content cascade in `LostCityRS/Content/scripts/tutorial/scripts/tutorial.rs2:296-330`. That cascade — 3 inv_clears, 19 inv_adds, 3 proc calls (`~stat_reset_all`, `~initalltabs`, `~update_all`) — is the NAI-139 surface area.

Per `tracker_entry_framing_can_be_incomplete`: the carry-forward note at `nai_followups.md` line 6458 cited line range 303-327 and "16 inv_add". The actual cascade is at line 296-330 with 18 inv_adds to inv + 1 inv_add(bank, coins, 25) + a third proc call (`~update_all(inv_getobj(worn, ^wearpos_rhand))` at line 330) that the carry-forward note omitted. Spec re-derived from primary source.

### In scope

Every opcode + proc reachable transitively (depth-N) from `tutorial.rs2:296-330`. Audit each against goscape's `pkg/script/` handler dispatch, cache loaders, and proc-by-name resolution. Fix all engine-side blockers in Stage 2.

The literal cascade (line 296-330):

```
[label,tutorial_complete]
tut_close();
if_close;

%tutorial = ^tutorial_complete;
p_telejump(0_50_50_22_22);

inv_clear(inv);
inv_add(inv, bronze_axe, 1);
inv_add(inv, tinderbox, 1);
inv_add(inv, net, 1);
inv_add(inv, shrimp, 1);
inv_add(inv, bucket_empty, 1);
inv_add(inv, pot_empty, 1);
inv_add(inv, bread, 1);
inv_add(inv, bronze_pickaxe, 1);
inv_add(inv, bronze_dagger, 1);
inv_add(inv, bronze_sword, 1);
inv_add(inv, wooden_shield, 1);
inv_add(inv, shortbow, 1);
inv_add(inv, bronze_arrow, 25);
inv_add(inv, airrune, 25);
inv_add(inv, mindrune, 15);
inv_add(inv, waterrune, 6);
inv_add(inv, earthrune, 4);
inv_add(inv, bodyrune, 2);

inv_clear(worn);
inv_clear(bank);
inv_add(bank, coins, 25);

~stat_reset_all;

~initalltabs;
~update_all(inv_getobj(worn, ^wearpos_rhand));
```

Depth-N transitive surface (proc bodies — re-derive at audit dispatch):
- `~stat_reset_all` (`Content/scripts/player/scripts/stat.rs2:71`) → `~stat_reset` body, `enum_getoutputcount`, `enum`, `stat`, `stat_base`, `stat_sub`, `stat_add`, `abs`, `sub`, `calc`.
- `~initalltabs` (`Content/scripts/login_logout/login.rs2:62`) → `if_settab` ×13, `inv_transmit` ×2, `inv_getobj`, `~update_weapon_category`, `~update_questlist`, `lowmem` branch.
- `~update_all` (`Content/scripts/player/scripts/appearance.rs2:98`) → `~update_weight_equipment`, `~update_bas`, `~update_bonuses`, `~update_weight`, `~update_weapon_category` (shared), `~player_combat_stat`, `p_finduid`, `p_animprotect`, `staffmodlevel`, `inv_getobj` (shared), `stat_add`, `%tutorial>^newbie_combat_instructor_unequipping_items` gate.

### Out of scope

- Pre-teleport tutorial flow (already smoke-bound by NAI-111).
- The teleport itself (`p_telejump`) — closed by NAI-111.
- Non-tutorial-completion code paths that share opcodes (handler may be wired correctly for tutorial-completion semantics but broken for combat semantics — separate sub-spec).
- Visual / client-side rendering bugs not caused by goscape engine (route to NAI-N+1 per `smoke_surfaces_adjacent_divergences`).
- TS-asymmetries that are not blockers for tutorial completion (track as deviations per `true_to_ts_gate`, defer).

### PRIMARY success — strict per-line TS-faithful, smoke-bound

User runs Tutorial Island → Magic Instructor "Yes, mainland" → at Lumbridge spawn (3222, 3222, level 0), verifies:

1. **Inventory:** exactly 18 items in TS-faithful slot ordering matching `tutorial.rs2:304-321` declaration order. Counts: bronze_axe×1, tinderbox×1, net×1, shrimp×1, bucket_empty×1, pot_empty×1, bread×1, bronze_pickaxe×1, bronze_dagger×1, bronze_sword×1, wooden_shield×1, shortbow×1, bronze_arrow×25, airrune×25, mindrune×15, waterrune×6, earthrune×4, bodyrune×2.
2. **Worn:** equipment slots all empty.
3. **Bank:** exactly `coins×25`. Verified by walking from Lumbridge to Varrock west bank and opening the booth (per Q4 user choice). Fallback if pathfinding/run-energy blocks the walk: NodeDebug-gated `s.log.Info` gateway on the inv-mutation path captures bank contents at cascade exit (per `nodedebug_gateway_probe_pattern`).
4. **Stats:** Hitpoints 10/1154 XP per Lost City convention. Trained skills (cooking/fishing/mining/smithing/woodcutting/firemaking/range/magic from tutorial flow) at level matching `[proc,tutorial_give_xp]` cap of base 3. Untrained skills level 1 / 0 XP. NOT a full XP zero — `~stat_reset_all` resets temporary boosts to base, not XP.
5. **Tab functionality:** all 14 UI tabs (skills, quest, inventory, worn-items, prayer, magic, friends, ignore, logout, controls, options, music — incl. lowmem variants) clickable AND populated (not blank, not error).
6. **Weapon-category UI:** attack-tab weapon-category section reflects empty rhand correctly (likely "Unarmed").

### SECONDARY (informational, non-blocking)

Any client-side or content-side divergence observed in smoke that doesn't affect §1 strict criteria — captured in close commit, routed to NAI-N+1 per `smoke_surfaces_adjacent_divergences`.

## §2 Stage 1 — parallel-bundle audit architecture

### Shared briefing (every bundle subagent gets this)

- Canonical TS source: `LostCityRS/Content/scripts/tutorial/scripts/tutorial.rs2:296-330`.
- Goscape handler-dispatch root: `pkg/script/` (`handlers.go`, `handlers_inv.go`, `opcode.go`, etc.).
- Cache loaders: `pkg/objtype/` (varp, loctype, etc.); script-cache loader to be located by grep.
- Anti-fabrication mandate (per `audit_subagent_fabrication`): every "implemented / missing / stub" assertion MUST cite `file:line` from goscape AND `file:line` from TS reference. Use `Read` tool on cited lines, not just grep.
- Output format per row: `{kind: opcode|proc, name, ts_def_path:line, goscape_dispatch_path:line, status: WIRED|STUB|MISSING|UNKNOWN, evidence_notes}`.
- Subagent model: Sonnet (per `superpowers_code_reviewer_model` — applies to audit subagents too; never Opus).

### Bundle decomposition

| Bundle | Surface (depth-N) |
|---|---|
| **B1** | Line 296-330 immediate ops only: `tut_close`, `if_close`, varp set `%tutorial = ^tutorial_complete`, `inv_clear` ×3, `inv_add` ×19. NO descent into the 3 procs. |
| **B2** | `~stat_reset_all` subtree: `~stat_reset` body, `enum_getoutputcount`, `enum`, `stat`, `stat_base`, `stat_sub`, `stat_add`, `abs`, `sub`, `calc`. |
| **B3** | `~initalltabs` subtree: `if_settab` ×13, `inv_transmit` ×2, `inv_getobj`, `~update_weapon_category` body, `~update_questlist` body, `lowmem` branch, all referenced interface IDs. |
| **B4** | `~update_all` subtree: `~update_weight_equipment`, `~update_bas`, `~update_bonuses`, `~update_weight`, `~update_weapon_category` (shared with B3 — flag for dedup), `~player_combat_stat`, `p_finduid`, `p_animprotect`, `staffmodlevel`, `inv_getobj` (shared with B3), `stat_add`, `%tutorial>^newbie_combat_instructor_unequipping_items` gate, full proc body. |

### Known cross-bundle overlaps (declared upfront for controller dedup)

- `stat_add` (B2, B4)
- `inv_getobj` (B1 line-330, B3, B4)
- `~update_weapon_category` (B3, B4)
- `stat`, `stat_base` (potentially B2, B4 — verify in rollup)

### Controller responsibilities (post-bundle return)

1. Merge 4 deliverables into single defect list at `docs/superpowers/specs/2026-05-09-nai-139-stage-1-audit.md`.
2. Cross-foot the math (per `audit_arithmetic_correction_in_rollup`): total unique opcodes/procs claimed should match a controller-side enumeration of unique tokens in `tutorial.rs2:296-330` + transitively-included proc bodies. Any discrepancy → re-dispatch.
3. Pre-flight verification (per `controller_preflight`): 100% of MISSING/STUB rows — Read each cited line, confirm. 20% random sample of WIRED rows — Read, confirm dispatch path actually executes for the cited opcode.
4. Per `audit_subagent_fabrication`: any subagent claim that fails pre-flight → re-dispatch that bundle with the contradiction in the prompt.

### Stage 1 commit shape

- `spec(nai-139): brainstorm + design doc` (this doc, plus initial commit)
- `plan(nai-139): Stage 1 — parallel-bundle audit dispatch`
- `audit(nai-139): Stage 1 bundles B1+B2+B3+B4 returns` (single rollup commit per `audit_arithmetic_correction_in_rollup`)
- `chore(nai-139): Stage 1 audit verdict + Stage 2 routing decision`

## §3 Stage 2 — fix routing

### Decision tree at audit close

```
audit defect list
├── 0 blockers (all WIRED)
│   → close NAI-139 as audit-clean
│   → user runs smoke
│   ├── smoke PRIMARY-met → close NAI-139 (rare path: theory + observation both clean)
│   └── smoke FAILS → reframe: cascade has runtime defect not visible to static audit;
│                     route to NAI-140 fresh investigation
├── 1-N blockers, all engine-side (MISSING / STUB opcodes/procs)
│   ├── total fix ≤~15 LOC (per `compressed_cadence`)
│   │   → Stage 2 = combined plan+fix doc, single Sonnet implementer, single Sonnet reviewer
│   └── total fix >15 LOC OR multiple touched files
│       → Stage 2 = full plan doc + subagent-driven-development dispatch
│   → batch ALL blockers in ONE Stage 2 commit
│   → smoke binds; adjacent residuals → NAI-N+1
└── blockers include cache-data issues (proc not loaded, interface ID missing)
    → mirror NAI-138 precedent (cache-loader fix pattern)
    → batch with engine fixes if same surface; split if loader is independent subsystem
```

### Stage 2 fix-doc location

`docs/superpowers/plans/2026-05-09-nai-139-stage-2-<short-tag>.md` where `<short-tag>` reflects the actual blocker shape (e.g., `missing-stat-handlers`, `proc-resolver-fix`, `multi-blocker-batch`). Tag chosen at Stage 2 plan-write time, not pre-committed.

### Stage 2 invariants (compressed or full)

- TDD per `superpowers:test-driven-development`: every fix has at least one test that fails pre-fix and passes post-fix.
- TS-fidelity per `true_to_ts_gate`: any divergence from TS gets a tracked deviation comment + DEVIATION-NAI-139-D<N> tag.
- Implementer commit content verified per `implementer_commit_content_verify` (`git show <SHA> --stat` post-commit).
- Code-review on Sonnet per `superpowers_code_reviewer_model`.
- `Closes memory: <name>` trailer on close commit if any generalizable principle is captured (per `close_commit_memory_trailer`).
- Working tree clean per `feedback_subagent_wt_path` (`git status` after each commit).

### Close-commit triggers

- (PRIMARY met) smoke confirms all 6 strict criteria from §1.
- (Audit-clean) audit finds 0 blockers AND smoke PRIMARY-met.
- (Reframed) audit finds 0 blockers but smoke fails — close NAI-139 as audit deliverable, reframe to NAI-140 with smoke output as the binding signal.

## §4 Smoke handoff

### Server start (user-side, per `smoke_test_server_handoff`)

```
CGO_ENABLED=0 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go run \
  -trimpath ./cmd/goscape --config.file config.yaml
```

Client: Java client at `/home/owner/Code/github.com/LostCityRS/Client-Java`, configured for cache version 225 against the local goscape server.

### Fresh-account requirement

Smoke MUST start from a brand-new tutorial-stage account (not a saved mid-tutorial state). Tutorial Island progress is varp-driven (`%tutorial`), and a partially-progressed account could enter `[label,tutorial_complete]` from non-canonical state and mask defects. Use a fresh username on each smoke run.

### Smoke sequence

1. Log in fresh → spawn at Tutorial Island starting room.
2. Complete Tutorial Island fully (all sub-quests through Magic Instructor).
3. Magic Instructor → choose "Yes, go to mainland" → observe `[label,tutorial_complete]` execution.
4. Verify teleport completed (NAI-111 regression check).
5. Strict per-line verification — capture pass/fail + observation note for each:
   - **Inv slot-by-slot:** verify exact 18 items in slot ordering, exact counts.
   - **Worn:** all slots empty.
   - **Bank:** walk Lumbridge → Varrock west bank → open booth → verify `coins×25`. Fallback to NodeDebug log gateway if walk blocked.
   - **Stats:** Hitpoints 10/1154 XP; trained skills at base ≤3 from tutorial XP; untrained at level 1.
   - **Tab functionality:** click each of 14 tabs, verify renders + populated.
   - **Weapon-category UI:** verify reflects empty rhand.
6. Capture server log: any `script not protected`, `proc not found`, opcode-dispatch warnings, panics.

### Smoke deliverable

User pastes back into a smoke handoff doc:
- Per §1 verification item: `PASS / FAIL / N/A` + observation.
- Server log excerpts (warnings, errors, stack traces).

Handoff doc path: `docs/superpowers/handoffs/2026-05-09-nai-139-stage-<N>-smoke.md` (Stage 1 audit-clean smoke + Stage 2 post-fix smoke as separate docs if both occur).

## §5 Anti-fabrication & verification gates

Per memories `audit_subagent_fabrication`, `controller_preflight`, `verify_implementer_claims`, `tracker_entry_framing_can_be_incomplete`, `spec_test_runtime_behavior_verify`, `risk_register_premise_grep`.

### Audit-subagent output requirements (mandate; reject deliverable if missing)

1. Every status row cites BOTH `<ts_path>:<line>` AND `<goscape_path>:<line>` (or explicit `MISSING-IN-GOSCAPE` for the goscape side).
2. Every WIRED claim cites the exact dispatch site (case branch, table entry) — not just "the file exists".
3. Every STUB claim quotes the stub line verbatim (panic, TODO, no-op return).
4. No claims based on "by analogy with neighboring opcode" — must Read the actual handler.

### Controller pre-flight (post-bundle, pre-merge)

- 100% sample of MISSING/STUB rows: Read each cited line, confirm.
- 20% random sample of WIRED rows: Read, confirm dispatch path executes for the cited opcode.
- Cross-foot total opcode + proc count against an independent controller-side enumeration (grep cascade region for opcode patterns + transitive proc bodies).
- Any discrepancy → re-dispatch THAT bundle with the contradiction quoted in the prompt.

### Plan-author pre-flight (Stage 2)

- Re-grep every audit-doc assertion at HEAD before plan-write (per `controller_preflight`, `spec_followup_tracker_freshness`).
- Verify no plan-prescribed test fixture relies on inferred mock fields (per `mock_recorder_field_naming_check`).
- Verify all Go variable names in code blocks compile (per `plan_var_name_collision`).
- Verify struct-literal type names against actual package (per `plan_type_name_grep`).

### Implementer-commit verification (post-fix)

- Controller runs `git show <SHA> --stat` and `git show <SHA>` after each fix commit (per `implementer_commit_content_verify`).
- Confirm scope of commit matches plan task scope; reject if implementer drifted.
- Verify on main working tree (per `feedback_subagent_wt_path`): `git status` clean, no stray files in main.

### Smoke-result verification (binding)

- User-pasted smoke output is the binding signal.
- If smoke claims PASS but a §1 criterion is unaddressed, controller asks for the missing check before close.
- Any FAIL on §1 criteria → NAI-139 does NOT close — route to fix iteration or NAI-N+1 reframe.

## §6 Deliverables & file layout

### Stage 1
- `docs/superpowers/specs/2026-05-09-nai-139-tutorial-completion-cascade-design.md` — this spec.
- `docs/superpowers/specs/2026-05-09-nai-139-stage-1-audit.md` — controller-merged defect list (post-bundle dispatch).
- 4 commits: spec / plan / audit-rollup / verdict.

### Stage 2 (only if blockers found)
- `docs/superpowers/plans/2026-05-09-nai-139-stage-2-<short-tag>.md` — fix plan or compressed combined doc.
- Test commits + fix commits per cadence (compressed = 1-2 commits; full = TDD per task).

### Smoke handoff(s)
- `docs/superpowers/handoffs/2026-05-09-nai-139-stage-<N>-smoke.md` — one per smoke run.

### Close commit
- `chore(close): NAI-139 — PRIMARY met; <one-line summary>` with `Closes memory: <name>` trailer if applicable.

### Memory entries (post-close, contingent)
- Any cascade-binding lesson learned (e.g., a goscape opcode-dispatch convention that surprised the audit, a TS asymmetry pattern in tutorial-completion content).
- Will not pre-commit memory-entry shape; let actual close-time learnings determine.

## §7 Pattern memories applied

- `investigation_subspec_cadence` — Stage 1 audit → conditional Stage 2 fix → smoke.
- `tracker_entry_framing_can_be_incomplete` — re-derived cascade from primary source; corrected line range and item count from carry-forward note.
- `cascade_theory_smoke_binding` — smoke binds cascade attribution; close on smoke-bind.
- `smoke_surfaces_adjacent_divergences` — adjacent residuals route to NAI-N+1 rather than in-scope-stretch.
- `audit_subagent_fabrication` — anti-fabrication mandates on every audit-subagent deliverable; controller pre-flight on 100% of MISSING/STUB and 20% of WIRED.
- `audit_arithmetic_correction_in_rollup` — controller cross-foots total claim count against independent enumeration.
- `controller_preflight` — 30-second grep+Read pass against HEAD before each implementer dispatch.
- `dispatching-parallel-agents` — 4 independent bundle subagents in single Agent block.
- `superpowers_code_reviewer_model` — Sonnet for all audit + reviewer subagents, never Opus.
- `compressed_cadence` — Stage 2 compressed if total fix ≤~15 LOC.
- `nodedebug_gateway_probe_pattern` — bank-verification fallback if pathfinding blocks Varrock walk.
- `smoke_test_server_handoff` — user-launched server; sandbox can't reach host network.
- `feedback_subagent_wt_path` — `git status` on main after every commit.
- `close_commit_memory_trailer` — `Closes memory: <name>` trailer on close commit.
- `true_to_ts_gate` — every divergence gets a tracked deviation; DEVIATION-NAI-139-D<N> tag.
- `implementer_commit_content_verify` — `git show <SHA> --stat` post-commit.
- `ts_source_canonical_path` — `LostCityRS/Engine-TS` + `LostCityRS/Content` only.
- `chatnpc_pipe_line_break` — relevant if any chatnpc UI surface in cascade (audit B3 may surface).

## §8 Risk register

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Audit subagent fabricates a WIRED claim that masks a real defect | Medium | High (smoke fails after Stage 2 fix, reroute) | 20% sample of WIRED rows + Read at controller pre-flight; 100% sample of MISSING/STUB |
| Audit cross-bundle dedup misses a shared opcode, producing inconsistent status | Low-Medium | Medium (controller re-dispatches on contradiction) | Pre-declared overlap list in §2; rollup commit cross-foots math |
| Pathfinding/run-energy blocks Varrock walk, smoke can't verify bank | Medium | Low (fallback to NodeDebug gateway) | NodeDebug log gateway as backup; documented in §1 criterion 3 |
| Tutorial Island playthrough exposes pre-cascade defect (smoke can't even reach line 296) | Low | High (cannot bind NAI-139 at all) | NAI-111 regression check at smoke step 4; if fails, escalate to NAI-111 reopen |
| `~update_all` proc gates on `%tutorial>^newbie_combat_instructor_unequipping_items` — unclear if `^tutorial_complete` >= that constant | Medium | Low (mis-skipping ~update_weapon_category branch) | Audit B4 must verify constant ordering: read `tutorial.constant` or equivalent enum file; record actual gate result |
| Cache-data divergence (proc not loaded into goscape's script cache) | Low-Medium | High (proc resolution fails at runtime) | Audit checks proc loading from cache, not just opcode dispatch; mirrors NAI-138 cache-loader pattern |
| Multiple cascade-blockers chain (fixing inv subtree surfaces stat-subtree defect only after) | Low (Q4 batch-fix mitigates) | Medium (one extra smoke iteration) | Stage 2 batches all known blockers; smoke binds remaining tail to NAI-N+1 |
| Audit total surface exceeds Sonnet's reliable context (~60+ unique opcodes/procs across 4 bundles) | Low | Medium (reliability degrades) | Bundle decomposition caps each subagent at ~20-30 surface tokens; declared overlaps avoid rework |

## §9 Open follow-ups deferred to NAI-N+1

(Populated during smoke; placeholders here.)

- Any client-side rendering bug not caused by goscape engine.
- Any TS-asymmetry in cascade procs that doesn't block §1 criteria.
- Any pre-existing pathfinding / run-energy issue surfaced by Lumbridge → Varrock walk.
- Any non-tutorial-completion code path sharing cascade opcodes that has a separate defect.
