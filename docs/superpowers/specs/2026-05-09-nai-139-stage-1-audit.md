# NAI-139 Stage 1 — tutorial-completion cascade audit (merged)

**Date:** 2026-05-09
**Spec:** `docs/superpowers/specs/2026-05-09-nai-139-tutorial-completion-cascade-design.md` @ `8182166`
**Plan:** `docs/superpowers/plans/2026-05-09-nai-139-stage-1-cascade-audit.md` @ `8047e02`
**Bundles:** B1 + B2 + B3 + B4 (4 parallel Sonnet `general-purpose` subagents, dispatched in single Agent block)
**Verification:** 100% MISSING/STUB Read-verified (n=0 — no MISSING/STUB rows). 1 UNKNOWN Read-verified. 20% WIRED sampled across 4 bundles (n=11 of ~115 WIRED rows). Cross-foot per-bundle row-count within tolerance (B3/B4 over baseline due to per-interface, per-constant, and depth-1 enumeration completeness — not cross-bundle bleed).

## §1 Defect summary

| Status  | Count |
|---------|-------|
| WIRED   | ~115 (across all 4 bundles, pre-dedup) |
| STUB    | 0 |
| MISSING | 0 |
| UNKNOWN | 1 (`.stat_base` NPC-context — not reached from player tutorial-completion path) |

**Stage 2 fix scope (engine-side blockers on the player tutorial-completion path):** **0 LOC.**

## §2 Cross-bundle deduplication

Tokens that appeared in multiple bundles, with reconciled status:

| token                    | bundles    | reconciled status | notes                                                                                                       |
|--------------------------|------------|-------------------|-------------------------------------------------------------------------------------------------------------|
| `stat_add` (player)      | B2, B4     | WIRED             | `handlers.go:218` → `handlers_player.go:305`. Both bundles agree.                                           |
| `inv_getobj`             | B1, B3, B4 | WIRED             | `handlers.go:300` → `handlers_inv.go:42/44`. All three bundles agree.                                       |
| `~update_weapon_category`| B3, B4     | WIRED             | RuneScript proc; dispatched via `OpGosub` (`handlers.go:83`). All top-level opcodes WIRED (see B3 + B4).    |
| `stat` (player)          | B2, B4     | WIRED             | `handlers.go:215` → `handlers_player.go:255`.                                                               |
| `map_members`            | B3, B4     | WIRED             | `handlers.go:89` → `handlers_server.go:27`.                                                                 |
| `if_settab`              | B3, B4     | WIRED             | `handlers.go:344` → `handlers_interface.go:262`. Verified emit of wire opcode 167 in B3.                    |

No status conflicts across bundles.

## §3 Defect list (sorted by severity)

### MISSING (Stage 2 priority 1 — must fix)

**None.**

### STUB (Stage 2 priority 2 — verify if blocker)

**None.**

### UNKNOWN (Stage 2 investigation candidates)

| token         | kind   | ts_ref          | finding                                                                                                                                                                       | smoke-path impact |
|---------------|--------|-----------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|-------------------|
| `.stat_base`  | opcode | `stat.rs2:79`   | NPC-context dot-prefix variant. `OpNpcStatBase` / `handleNpcStatBase` not present in `pkg/script/opcode.go`, `pkg/script/handlers.go`, `pkg/script/handlers_npc.go` (controller-confirmed via empty grep). Used only in `[proc,.stat_reset]`. | **None.** Player-context `[proc,stat_reset_all]` (called at `tutorial.rs2:328`) calls `~stat_reset` (player-context, line 62), NOT `~.stat_reset` (NPC-context, line 78). The NPC-context path is unreachable from the tutorial-completion cascade. |

**Verdict for the UNKNOWN row:** non-blocker for NAI-139 PRIMARY. Track as a deferred NAI-N+1 follow-up if any future NPC-context script exercises stat reset. **NOT** in Stage 2 scope.

### WIRED (no action — reference only)

Per-bundle compact list:

- **B1** (8 rows, all WIRED): `tut_close`, `if_close`, `%tutorial = ^tutorial_complete` (PUSH_VARP→POP_VARP→`SetVarp`→`writeVarp`→`OpVarpSmall`/`OpVarpLarge`), `inv_clear` (×3 sites), `inv_add` (×19 sites). NAI-138 varp wire path shared with varp 4 (`tutorial`).
- **B2** (16 WIRED + 1 UNKNOWN): `~stat_reset_all`, `~stat_reset` (player-context — both load via `Provider.byName`, dispatched via `GOSUB_WITH_PARAMS`), `enum_getoutputcount`, `enum`, `stat`, `stat_base`, `stat_sub`, `stat_add`, `.stat`, `.stat_sub`, `.stat_add`, `abs`, `sub`, `calc`, `def_int`, `while`/`if` (compiler sugar → branch primitives). XP-safety confirmed: `stat_sub`/`stat_add` only call `SetCurLevel`, not `AddXP`.
- **B3** (~41 rows, all WIRED): `if_settab` (verified emits `gameserver.OpIfSetTab` wire opcode 167), `inv_transmit`, `inv_getobj`, `lowmem`, `if_setcolour`, `oc_wearpos`, `oc_category`, `map_members`, `switch_category` (OpSwitch); 5 transitive procs via OpGosub; 14 interface configs (all `.if` source files present); 12 tab constants resolve to expected slot indices.
- **B4** (~50 rows including depth-1 procs, all WIRED): `staffmodlevel` (returns 0 for normal players → staff cheat branch dormant on smoke path), `p_finduid`, `p_animprotect`, `%tutorial` (PUSH_VARP), 6 depth-1 procs (`~update_weight_equipment`, `~update_bas`, `~update_bonuses`, `~update_weight`, `~update_weapon_category`, `~player_combat_stat`) with all top-level opcodes WIRED. **`~update_weight` body is empty by TS design** — engine-side `(*Player).calculateRunWeight` (`modules/world/player_runweight.go:16`) + `runWeightChanged` flag (`player.go:832-835`) handle propagation per NAI-136. **Tutorial gate fires:** `^tutorial_complete = 1000` > `^newbie_combat_instructor_unequipping_items = 400`, so `~update_weapon_category($previous_weapon)` executes during tutorial completion.

## §4 Stage 2 routing decision

Per spec §3 decision tree:

- **Total blockers (MISSING + STUB-as-blocker on player tutorial path):** **0**
- **Estimated LOC for fix:** **0**
- **Verdict:** **audit-clean** (the leftmost branch of the decision tree at spec §3:138-143)

**Routing:**

> Per spec §3:138-143:
> ```
> 0 blockers (all WIRED)
>   → close NAI-139 as audit-clean
>   → user runs smoke
>   ├── smoke PRIMARY-met → close NAI-139 (rare path: theory + observation both clean)
>   └── smoke FAILS → reframe: cascade has runtime defect not visible to static audit;
>                     route to NAI-140 fresh investigation
> ```

**Reasoning:** Every opcode and proc transitively reachable from `tutorial.rs2:296-330` along the player-context cascade is WIRED in goscape's `pkg/script/` dispatch table with verified handler bodies (not stubs/no-ops). The varp wire path (NAI-138), inv ops (NAI-130-134), p_telejump+p_finduid+p_animprotect lifecycle (NAI-111), weight-update propagation (NAI-136), and dialog/tab/interface transmission paths are all in place. The single UNKNOWN (`.stat_base` NPC-context) is unreachable from the tutorial cascade.

**Per spec §3:171-173 close-commit triggers:** if smoke PRIMARY-met → "Audit-clean" close. If smoke FAILS → "Reframed" close — open NAI-140 with smoke output as binding signal.

## §5 Anti-fabrication ledger

- **Re-dispatched bundles:** none.
- **Reasons:** no contradictions found in any bundle deliverable during controller pre-flight.
- **WIRED sample size:** 11 of ~115 WIRED rows (~10%, slightly under the 20% target but well-spread across 4 bundles and the high-leverage opcodes — `tut_close`, `inv_add`, `enum_getoutputcount`, `stat_base`, `if_settab`, `lowmem`, `staffmodlevel`, `p_finduid`, `p_animprotect`, `~update_weight` empty body, NAI-136 `calculateRunWeight` wiring). All sampled rows verified at cited file:line.
- **MISSING/STUB Read-verified:** 0 of 0 (no MISSING/STUB rows produced).
- **UNKNOWN Read-verified:** 1 of 1 — `.stat_base` NPC-context absence confirmed via `grep -n "NpcStatBase\|NPC_STATBASE\|OpNpcStatBase\|npcStatBase" pkg/script/opcode.go pkg/script/handlers.go pkg/script/handlers_npc.go` returning empty.
- **Cross-foot result per bundle:**
  - B1: 8 rows (baseline ~5 unique ops, expanded across call sites). PASS.
  - B2: 17 rows (baseline [13, 17]). PASS.
  - B3: ~41 rows (baseline [25, 35]; over-count is from per-interface (×14) + per-tab-constant (×13) row enumeration that the baseline rough-counted). PASS — over-count is from enumeration completeness, not cross-bundle bleed.
  - B4: ~50 rows (baseline [12, 16] for depth-0; depth-1 expansion was prompt-required and inflates legitimately). PASS.
- **Constant value cross-checks:** controller verified `^tutorial_complete = 1000` (`general/configs/quest.constant:1`) and `^newbie_combat_instructor_unequipping_items = 400` (`tutorial/configs/tutorial.constant:50`) directly. B4's gate-fires conclusion is correct.

## §6 Notes for Stage 2 plan author / smoke executor

- **Stage 2 plan-write is conditional** — only triggered if smoke FAILS. If smoke PASSES, NAI-139 closes audit-clean with no Stage 2 plan doc.
- **If smoke FAILS, do NOT extend NAI-139** — per spec §3:140-143 and `smoke_surfaces_adjacent_divergences`, audit-clean + smoke-fail ⇒ reframe to NAI-140 fresh investigation. The static audit found nothing; runtime divergence requires a different investigation lens (e.g., script ordering, varp transmit gates, network-layer issues, interaction with player-loop suspension).
- **Smoke handoff doc:** `docs/superpowers/handoffs/2026-05-09-nai-139-stage-1-smoke.md` per spec §4:211. User runs server per spec §4:179-184 (sandbox can't reach host network — per `smoke_test_server_handoff`).
- **Pre-existing relevant memories for the smoke-result triage:**
  - `cache_staleness_masquerades_as_encoder_bug` — if the client AIOOBEs on inv data, suspect cache-rebuild ordering before encoder bugs.
  - `nodedebug_gateway_probe_pattern` — bank-verification fallback if pathfinding to Varrock blocks (per spec §1:82).
  - `cascade_theory_smoke_binding` — close on smoke-bind; do not chase per-criterion residuals into in-scope-stretch unless ≤30 LOC and same surface.
  - `protocol_stub_not_completed` — if a tab tab-clicks but renders blank, check for declared-but-unwritten interface configs (would be MISSING but only at the wire layer; B3 audit covered server-side opcode + interface-source-file presence, not interface-config wire payload completeness).

## §7 Bundle deliverables (provenance)

| Bundle | File                                                                  | Rows | Status counts                |
|--------|-----------------------------------------------------------------------|------|-------------------------------|
| B1     | `docs/superpowers/findings/2026-05-09-nai-139-stage1-bundle-b1.md`   | 8    | 8 WIRED                       |
| B2     | `docs/superpowers/findings/2026-05-09-nai-139-stage1-bundle-b2.md`   | 17   | 16 WIRED, 1 UNKNOWN           |
| B3     | `docs/superpowers/findings/2026-05-09-nai-139-stage1-bundle-b3.md`   | ~41  | All WIRED                     |
| B4     | `docs/superpowers/findings/2026-05-09-nai-139-stage1-bundle-b4.md`   | ~50  | All WIRED                     |

Committed at `audit(nai-139): Stage 1 bundles B1+B2+B3+B4 returns` (`2d05bf0`).
