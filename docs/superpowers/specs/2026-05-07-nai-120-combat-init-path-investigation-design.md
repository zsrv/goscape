# NAI-120 — Combat-init path missing-handler investigation + port

**Date:** 2026-05-07
**Status:** spec — investigation sub-spec (Stage 1 audit → Stage 2 per-file TDD ports → smoke → conditional Stage 3) per `investigation_subspec_cadence`.
**Predecessor:** NAI-119 smoke residual #2 (close commit `0e8ffc5`, smoke 2026-05-07). Smoke surfaced `[proc,player_in_combat_check]` failing at pc=1 with "no handler for MAP_MULTIWAY (opcode 1014)" when attacking rats in tutorial; pc=1 means downstream handlers along the combat-init chain are likely also missing — sizing on MAP_MULTIWAY alone is naive (per `enumerate_all_sites`).
**Cadence:** Bundle 0 controller pre-flight (no commits, no subagent) → Bundle 1 Stage 1 audit (one Explore subagent, Sonnet) → Bundle 2..N Stage 2 TDD ports (one bundle per inner-ring file, full TDD) → user-launched smoke handoff → conditional Stage 3.
**Tech stack:** Go 1.26+.
**Upstream sources:** `LostCityRS/Engine-TS` (TS engine, per `ts_source_canonical_path`); `LostCityRS/Content/scripts/skill_combat/scripts/player/*.rs2` (8 inner-ring `.rs2` files, ~1200 lines total).

---

## 1. Symptom

Surfaced 2026-05-07 at NAI-119 smoke. After NAI-119 closure (LOC iterator family + LOC_CATEGORY wired; tutorial mining gate path-find unblocked), the next Tutorial Island step instructs the player to attack a rat. Java client sends `OPNPC2` (attack); server-side dispatch reaches `[opnpc2,_] @player_combat_start` (or `[apnpc2,_] @player_combat_start_ap` if out of melee range), which `gosub`s `[proc,player_in_combat_check]`.

Server emits warn log: `no handler for MAP_MULTIWAY (opcode 1014)` at pc=1 of `[proc,player_in_combat_check]`. Combat does not initiate; the rat is not attacked.

The pc=1 framing is significant: MAP_MULTIWAY is the second opcode in the proc body (pc=0 likely pushes `npc_coord`, pc=1 consumes it via MAP_MULTIWAY). Because the proc is short-circuited at pc=1, **no other missing handler in this proc body has been surfaced yet**. Furthermore, even with MAP_MULTIWAY ported, the proc returns true → control falls through to one of `@player_melee_attack` / `@player_ranged_attack` / `@player_magic_attack`, whose bodies are entirely unexercised at HEAD. Naive sizing on MAP_MULTIWAY alone would close pc=1 but leave the next missing handler unsurfaced for a separate sub-spec cycle.

Per the user-confirmed scope decision, NAI-120 walks the **full combat-init chain reachable from `[opnpc2,_]` and `[apnpc2,_]` whose body lives inside `skill_combat/scripts/player/*.rs2`**, enumerates every missing opcode handler / var registration / opcode declaration, and ports them. This bounds the sub-spec at 8 `.rs2` files (~1200 source lines) with a clean escape hatch: any token whose definition lives outside this directory is recorded as a frontier item and routed to NAI-121+.

---

## 2. Goal

Enumerate and port every opcode handler, var registration, or PUSH_VAR/POP_VAR family wiring required to execute the combat-init chain from `[opnpc2,_]` / `[apnpc2,_]` on Tutorial Island giant rats end-to-end. Bind the close on a smoke that reaches "first hit applied" (or, if engine-side gaps surface beyond the inner ring, "combat dispatcher reaches `@player_melee_attack` without missing-handler errors").

---

## 3. Scope

### 3.1 Inner ring (in NAI-120)

Every label / proc reachable from `[opnpc2,_]` or `[apnpc2,_]` whose body lives in:

```
LostCityRS/Content/scripts/skill_combat/scripts/player/
  player_combat.rs2           (149 lines) — entry labels + in_combat_check + roll procs + attackrange
  player_melee.rs2            ( 59 lines) — [label,player_melee_attack]
  player_ranged.rs2           (144 lines) — [label,player_ranged_attack]
  player_magic.rs2            (280 lines) — [label,player_magic_attack]
  auto_cast.rs2               ( 60 lines) — [proc,player_autocast_enabled] etc.
  auto_retaliate.rs2          ( 46 lines)
  player_attackstyles.rs2     (201 lines)
  player_combat_stat.rs2      (256 lines)
```

Total: ~1195 source lines. Inter-file references inside the directory (e.g., `player_combat.rs2`'s `~player_attackrange` proc-call) stay in scope.

### 3.2 Outer ring (out of NAI-120; Stage 1 frontier list, routed to NAI-121+)

- `skill_combat/scripts/player/spells/` subdir (separate dispatch, magic-spell content).
- Any label / proc whose body lives outside the inner ring (e.g., system-wide damage-roll / NPC-hit / hit-splat encoders).
- Engine-side wiring beyond opcode handlers (e.g., new ScriptState fields, new NPC stat field setters, new server vars beyond bytecode pushes) — except where required to compile a Stage 2 handler (then in scope).
- Any opcode whose TS impl requires substantial cross-system plumbing (split: handler stub in NAI-120, plumbing in NAI-121).

### 3.3 Anti-scope

- No refactor of existing wired handlers unless their current signature blocks a Stage 2 handler.
- No content fixes (rs2 logic bugs); only port.
- No extension into NPC update / hit splat encoding beyond what compiles.

---

## 4. Bundle 0 — Controller pre-flight (no commits, no subagent)

The controller pre-flight is the load-bearing step (catches false "X already exists" claims before any subagent dispatch — `risk_register_premise_grep`; NAI-119 spec §1 had 6/7 hit rate on existence claims).

### 4.1 Token extraction (per inner-ring file)

For each of the 8 `.rs2` files:

1. Strip comments (`//...`) + string literals (`"..."`) before extraction.
2. `rg -o '\b[a-z_][a-z_0-9]*\b\s*\(' file.rs2` → list every function-call-shape token (intrinsics + procs).
3. `rg -o '%[a-z_][a-z_0-9]*'` → list every var read.
4. `rg -o '@[a-z_][a-z_0-9]*'` → list every label-jump.
5. `rg -o '~[a-z_][a-z_0-9]*'` → list every proc-call.
6. `rg -o '\^[a-z_][a-z_0-9]*'` → list every constant reference (for enum/inv-pack registration sanity).
7. Symbolic operators (`=`, `!`, `>`, `<`, `&`, `|`, `>=`, `<=`) compile to BRANCH opcodes; controller spot-checks the BRANCH_* family (R2 in §9) but does not enumerate per-instance.

### 4.2 Cross-reference matrix

For each unique extracted token, controller fills three columns and one TS-source column, appending to the plan doc:

| Token | `pkg/script/opcode.go` declared? (line) | `pkg/script/handlers*.go` dispatched? (line) | TS impl (file:line) |

Categorization tags:
- **(W) Wired** — declared + dispatched.
- **(D) Declared-only** — `OpFoo Opcode = N` exists but no `OpFoo: handleFoo` entry in any `handlers*.go`.
- **(U) Undeclared** — name not in `opcode.go` at all.
- **(V) Var-read** — `%name` access; tracked separately because it depends on PUSH_VAR* opcode wiring AND var-id registry.
- **(F) Frontier** — label/proc reference whose body lives outside inner ring.

### 4.3 Var-handling subcase

RS2 `%name` reads compile to a typed PUSH_VARP / PUSH_VARN / PUSH_VARS / PUSH_VARBIT opcode + var-id constant. Two failure modes:

- **(a) PUSH_VAR opcode itself not wired (handler missing)** — at HEAD `0e8ffc5`: PUSH_VARP / POP_VARP / PUSH_VARS / POP_VARS / PUSH_VARN / POP_VARN all wired (`handlers.go:203-208`, with PUSH_VARN/POP_VARN flagged as "stub until S6"). Controller verifies stub vs. real impl per actual TS-fidelity at HEAD. PUSH_VARBIT / POP_VARBIT not yet checked at spec-write — Bundle 0 will pin.
- **(b) The named var isn't registered in goscape's var registry** — affects only that var. Controller grep-checks goscape's var-registry path (TBD in Bundle 0 — not plan-author convention-inferred per `mock_recorder_field_naming_check`) for each `%name`.

Bundle 0 produces a `(V)` sub-table:

| Var | Type (server/player/npc/varbit) | Var-id (TS) | Goscape registry: present? |

### 4.4 Label/proc reference handling

For each `@label` / `~proc` reference:
- If body lives in inner-ring `.rs2`, mark as in-scope (Stage 2 covers reachability transitively).
- If body lives outside (e.g., grep across `LostCityRS/Content/scripts/`), mark as **(F) Frontier**. Frontier items are surfaced in the Stage 1 deliverable but NOT ported in NAI-120.

### 4.5 Bundle 0 deliverable

A complete table appended to the plan doc with every extracted token classified. Bundle 0 ships when:
- Every distinct token (intrinsic, proc, label, var) has a row.
- Every (D)/(U)/(V) token has a TS reference cited.
- Every (F) token has its actual definition file path cited.
- Controller has independently HEAD-verified §9 risk-register entries.

No commit (Bundle 0 produces plan-doc edits only; the plan-doc commit happens at writing-plans skill close).

---

## 5. Bundle 1 — Stage 1 audit (one Explore subagent, Sonnet)

Independent verification of Bundle 0's enumeration. Subagent prompt (drafted at writing-plans):

- Input: Bundle 0's table + the 8 `.rs2` files + the Engine-TS handler dirs (`Engine-TS/src/engine/script/handlers/`).
- Task: for each (D)/(U)/(V) entry, audit:
  - **TS impl signature** (pop/push types, side effects, state mutations).
  - **Goscape's nearest analog handler** (e.g., a sibling MAP_* handler whose pattern this should follow).
  - **Edge cases** (null handling, OOB, nullity sentinel).
- Output: a per-entry markdown stanza appended to the plan doc — TS file:line range + signature summary + recommended goscape impl + test-case skeletons.
- Constraints: Sonnet model cap per `superpowers_code_reviewer_model`; read-only (no edits/writes); cite TS source verbatim, no inferred behavior.

After subagent completes:
- Controller independently spot-checks at least 3 audit verdicts against TS source (per `audit_subagent_fabrication` — NAI-31 near-miss precedent).
- Controller validates frontier list by grepping each (F) reference against `LostCityRS/Content/scripts/` to confirm the body's true location.
- Controller appends final missing-handler list (with dependency edges between handlers, e.g. "handler X needs new ScriptState field Y, port Y first") and per-file Stage 2 bundle assignment.

No commit at Bundle 1 (audit verdicts are plan-doc additions; commit at plan close).

---

## 6. Bundle 2..N — Stage 2 TDD ports

### 6.1 Decomposition rule

One bundle per inner-ring `.rs2` file, merged where the file contributes ≤2 missing handlers. Anticipated split (firms up after Bundle 1):

- **Bundle 2A — `player_combat.rs2`:** at minimum MAP_MULTIWAY (1014); plus any PUSH_VAR-target var-id registrations needed for `%lastcombat_pvp` / `%lastcombat` / `%aggressive_npc` / `%npc_lastcombat` / `%npc_aggressive_player` / `%npc_macro_event_target`; plus any (D)/(U) tokens used by entry labels and roll procs.
- **Bundle 2B — `player_melee.rs2`:** likely the next-most-missing; melee is the default branch from `player_combat_start` after autocast=false.
- **Bundle 2C — `player_ranged.rs2`** and **Bundle 2D — `player_magic.rs2`:** branched-under-style-flag bundles. Magic is the largest (280 lines); may sub-split if Bundle 1 surfaces >5 missing handlers.
- **Bundle 2E — small files:** `auto_cast.rs2` / `auto_retaliate.rs2` / `player_attackstyles.rs2` / `player_combat_stat.rs2` — merged into one or two bundles depending on Bundle 1's count.

### 6.2 Per-bundle TDD shape

Per `test-driven-development`:

- **T1 (RED):** unit tests against the new handler, asserting pop/push signature + behavior per TS reference. Use `&ScriptState{}` fixtures with explicit `StackCapacity` per `scriptstate_test_fixture_idioms`. Test fixtures pre-codified in plan doc and mentally-executed at plan-write per `plan_runnable_test_fixtures`.
- **T2 (GREEN):** handler impl + dispatch entry in `pkg/script/handlers.go`. Where a handler needs a new ScriptState field or a new entity-side getter, plumb it inside the same task.
- **T3 (VERIFY):** controller runs `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...` from main working tree (per `verify_implementer_claims` 30-second protocol — fresh independent run, not implementer-reported pass).
- **T4 (REVIEW):** one Sonnet reviewer subagent per bundle (not compressed) — signature parity, TS-fidelity citations, edge cases. Per `superpowers_code_reviewer_model` cap.

### 6.3 Inter-bundle dependencies

If Bundle 2B's tests need an opcode ported in 2A (e.g., a shared MAP_* op called from both melee and ranged), Bundle 1's audit produces the dependency edges and controller orders dispatch sequentially. No parallel dispatch within Stage 2 unless edges confirm independence.

### 6.4 Per-handler test seeds (illustrative, firms after Bundle 1)

- **MAP_MULTIWAY (1014):** TS at `Engine-TS/src/engine/script/handlers/ServerOps.ts:376-380`. Pop coord, push `gameMap.IsMulti(coord) ? 1 : 0`. Tests: multi-zone true; non-multi false; coord-edge cases. TS-faithful nullity inheritance (TS doesn't OOB-check; goscape inherits, label as `// TS-faithful: no coord-OOB check; matches Engine-TS`).
- **PUSH_VAR* (if Bundle 0 finds a missing family):** per opcode-id, push var by id, exercise typed read paths. Sibling tests in `pkg/script/handlers_vars_test.go` are the analog.
- **Per-NPC-stat readers (e.g., `%npc_lastcombat`):** assert against an NPC fixture with explicit field set, verify push-int. New NPC fields plumbed inside same task.
- Each entry firms when Bundle 1 cites TS file:line.

---

## 7. Smoke handoff (out-of-band, no commit)

Per `smoke_test_server_handoff`: user runs server with the latest binary against Java client #225. Smoke target:

- **Primary:** Tutorial Island — attack a rat. Combat dispatcher reaches `@player_melee_attack` without missing-handler errors. Either first hit applied, OR the next missing handler beyond inner ring is surfaced (frontier item, not failure).
- **Secondary:** none (NAI-120 is dispatch-correctness; downstream NPC-hit / damage-roll lands in later sub-specs per `dispatch_correct_reach_blocked`).

Smoke surfaces:
- Inner-ring residual → in-scope-stretch fix per `smoke_surfaces_adjacent_divergences` (≤30 LOC) or Stage 3.
- Outer-ring (frontier) residual → route to NAI-121.

---

## 8. Conditional Stage 3 (only if smoke surfaces an inner-ring residual >30 LOC)

Template-pre-written at plan close, materialized only on smoke failure (per `investigation_subspec_cadence` NAI-31 precedent). Shape depends on the residual:
- If smoke gives a wire-traceable error (e.g., specific opcode warn at pc=N), Stage 3 is another targeted audit + fix.
- If smoke is silent, Stage 3 is gated runtime instrumentation per `nai_114_stage3_instrumentation_probe`.

---

## 9. Risk register (HEAD-verified at spec-write)

All §9 entries grep+Read-verified at HEAD `0e8ffc5` per `risk_register_premise_grep`:

- **R1 — ADD math op wired:** `OpAdd Opcode = 4600` declared at `pkg/script/opcode.go:434`; dispatched at `pkg/script/handlers.go:27`. ✅ Verified.
- **R2 — BRANCH_* opcodes wired:** `OpBranch / OpBranchNot / OpBranchEquals / OpBranchLessThan / OpBranchGreaterThan / OpBranchLessThanOrEquals / OpBranchGreaterThanOrEquals` declared at `pkg/script/opcode.go:39-43, 55-56`; representative subset (`OpBranch` / `OpBranchEquals` / `OpBranchNot`) dispatched at `pkg/script/handlers.go:21-23`. ✅ Verified for the family.
- **R3 — PUSH_VAR* / POP_VAR* opcodes wired:** PUSH_VARP/POP_VARP/PUSH_VARS/POP_VARS/PUSH_VARN/POP_VARN dispatched at `pkg/script/handlers.go:203-208`; PUSH_VARN/POP_VARN flagged "stub until S6" — Bundle 0 verifies whether stub semantics suffice for inner-ring NPC-stat reads. PUSH_VARBIT / POP_VARBIT declared at `pkg/script/opcode.go:52-53`; **dispatch entries not yet checked** — Bundle 0 task. ⚠ Partial.
- **R4 — `^constant` enum/inv pack registration:** `^wearpos_rhand`, `^stab_style`, `^slash_style`, `^crush_style`, `^ranged_style`, `^magic_style`, `^style_ranged_longrange` — Bundle 0 spot-checks one constant from each family against goscape's enum/inv pack loader. ⚠ Bundle 0 task.
- **R5 — Frontier label/proc bodies actually live outside inner ring:** Bundle 0 grep-confirms each `@label` / `~proc` reference's true definition file. ⚠ Bundle 0 task.
- **R6 — `p_aprange` opcode wired:** Bundle 0 task — `p_aprange($attackrange)` is called in entry labels.
- **R7 — Gosub / Jump opcodes wired:** `OpGosub` / `OpGosubWithParams` / `OpJump` / `OpJumpWithParams` dispatched at `pkg/script/handlers.go:30, 83, 346-347`. ✅ Verified.
- **R8 — `npc_coord` / `npc_uid` / `uid` / `mes` / `map_clock` wired:** `OpNpcCoord` at `handlers.go:385`, `OpMapClock` at `handlers.go:86`. `OpNpcUid` / `OpCoord` (player uid / coord) / `OpMes` — Bundle 0 grep tasks. ✅ partial; ⚠ Bundle 0 finishes.

**Audit-claim provenance** — Bundle 1's audit subagent verdicts are subject to `audit_subagent_fabrication`. Controller spot-checks at least 3 verdicts against TS source independently before Stage 2 dispatch.

---

## 10. Lessons applied (memory references)

- `investigation_subspec_cadence` — overall structure (Stage 1 audit → Stage 2 fix → smoke → conditional Stage 3).
- `controller_preflight` — Bundle 0 IS the structural answer.
- `verify_implementer_claims` — 30-second independent verification at every T3.
- `audit_subagent_fabrication` — Bundle 1 verdicts independently spot-checked.
- `mock_recorder_field_naming_check` — Bundle 0 grep-verifies var-registry path/symbol names; no convention-inference.
- `plan_runnable_test_fixtures` — every plan-codified test mentally-executed at plan-write.
- `scriptstate_test_fixture_idioms` — `&ScriptState{}` fixtures init StackCapacity + correct push order + Pointers flag.
- `enumerate_all_sites` — Stage 1 enumerates ALL inner-ring tokens, not just MAP_MULTIWAY; re-grep at HEAD post-Stage-1.
- `smoke_test_server_handoff` — user runs server.
- `smoke_surfaces_adjacent_divergences` — post-smoke routing by 30-LOC threshold.
- `dispatch_correct_reach_blocked` — primary close = TS-faithful inner-ring port; secondary = "first hit applied" deferred if engine-side gap blocks.
- `risk_register_premise_grep` — every §9 entry HEAD-verified at spec-write.
- `superpowers_code_reviewer_model` — Sonnet cap on review subagents.
- `iterator_state_pattern` — enumerate-before-scoping lesson (NAI-33 reference; applied here at the cross-file token-enumeration level).

---

## 11. Out-of-scope follow-ups (anticipated)

Will firm up after Bundle 1; tracked in `nai_followups.md` at NAI-120 close commit:

- Inner-ring opcode handlers whose TS impl requires NPC-stat-system plumbing not yet ported — split: handler + minimal field in NAI-120, broader plumbing in NAI-121.
- Var-id registry entries for vars that don't exist in goscape — register stubs returning zero/null with TS-faithful nullity, deviation-tag for follow-up.
- `spells/` subdir — entirely separate dispatch; routes to NAI-121+ if combat-magic flow surfaces.
- Outer-ring labels/procs (damage-roll system, NPC hit, hit-splat encoding) — frontier items routed to NAI-121+.
- Any opcode declared "stub until S6" (PUSH_VARN/POP_VARN per `handlers.go:207-208`) whose stub semantics block inner-ring reads — promote to real impl in NAI-120 if blocking, else defer.

---

## 12. Estimated scope

Subject to Bundle 0 + Bundle 1 firming:

- Inner-ring source: ~1195 rs2 lines across 8 files.
- Anticipated missing handler count: 5-15 (MAP_MULTIWAY confirmed; per-NPC-stat var registrations + maybe damage-roll arithmetic + ranged/magic-specific intrinsics).
- Total impl LOC: 200-800 production + comparable test LOC.
- Bundles: Bundle 0 (no commit) + Bundle 1 (no commit) + 3-5 Stage 2 bundles + conditional Stage 3. Multi-session if needed.

---

## 13. Close criteria

NAI-120 closes when:
1. Every (D)/(U)/(V) item from Bundle 1's final list is either ported (W) or deviation-tagged with rationale + follow-up routing.
2. `go test ./...` green at HEAD post-merge.
3. User-launched smoke confirms combat dispatcher reaches `@player_melee_attack` (or downstream branch) without missing-handler errors against inner-ring opcodes.
4. Frontier list + any in-scope deviations recorded in `nai_followups.md` with NAI-N+ routing.
5. Close commit includes `Closes memory:` trailer per `close_commit_memory_trailer`.
