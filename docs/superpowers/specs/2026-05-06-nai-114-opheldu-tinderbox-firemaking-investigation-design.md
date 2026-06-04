# NAI-114 — OPHELDU tinderbox-on-logs no-effect investigation

**Date:** 2026-05-06
**Status:** spec — investigation sub-spec (Bundle 0 has narrowed scope to H3; Stage 1 audit + Stage 2 fix + smoke per `investigation_subspec_cadence`).
**Predecessor:** NAI-113 smoke residual (close commit `881e899`, smoke 2026-05-06).
**Cadence:** Bundle 0 controller pre-flight (already complete; findings embedded below) → Stage 1.1 controller disasm extension → Stage 1.2 Sonnet audit subagent (opcode-coverage walk) → controller HEAD-verification → Stage 2 TDD fix → user-launched smoke handoff → conditional Stage 3 per `cascade_theory_smoke_binding`.
**Tech stack:** Go 1.26+.
**Upstream sources:** `LostCityRS/Engine-TS` (TS engine, per `ts_source_canonical_path`); `LostCityRS/Server` (content-side `firemaking.rs2` + `light_source.rs2`); `LostCityRS/Client-Java` rev-225 (Java client wire reference).

---

## 1. Symptom

Surfaced 2026-05-06 at NAI-113 smoke. After NAI-113 closure (inventory side panel now displays bronze axe + tinderbox correctly), the next Tutorial Island step instructs the player to use the tinderbox on logs in inventory to light a fire. Java client sends inbound game packet OPHELDU but **no in-game effect** observed:

- No fire (loc) spawns
- No firemaking animation (anim 733)
- No skill grant
- No chatbox advance to next tutorial step
- **No server warn log emitted**

User-side observation caveat: Tutorial Island message-box overlay covers the chatbox area, so any `MessageGame("Nothing interesting happens.")` or similar would be **invisible** to the user. Optical H1-vs-H3 discrimination is not possible; static binding is required.

---

## 2. Bundle 0 controller pre-flight (already complete)

Disasm pass via temporary probe (`cmd/probe-opheldu/main.go`, deleted after spec write) loaded `data/pack/server/script.dat` + `data/pack/server/obj.dat` and enumerated:

### 2.1 OPHELDU script registration

- `Provider.Load("data/pack/server")` decoded **8032 scripts** at compiler version 26.
- `[opheldu, <objType>]` type-specific scripts registered: **205**, including key entries:
  - `obj 590  tinderbox        key=0x93a91`  ← present
  - `obj 1511 logs             key=0x179e91` ← **NOT present**
  - `obj 2511 newbielogs       key=...`      ← NOT present
- Category-fallback registration: cat 212 (logs) maps to `[opheldu,_category_22]` at key=0x35191. Other categories registered (arrows, bolts, weapons, etc.).
- `[opheldu,_]` global lookup key 145: NOT registered.

### 2.2 `[opheldu,tinderbox]` source + body

- **Source file:** `LostCityRS/Server/content/scripts/skill_firemaking/scripts/firemaking.rs2`.
- **Lookup key:** `0x93a91` = `LookupKeyForType(TriggerOpHeldU=145, tinderbox=590)`. Verified.
- **Bytecode (verbatim):**

  ```
  PC  Opcode                    Operand
   0: LAST_USEITEM              0
   1: PUSH_CONSTANT_INT         2511   ; newbielogs
   2: BRANCH_EQUALS             1      ; if equal → jump +1 to PC 4
   3: BRANCH                    3      ; else → jump +3 to PC 7
   4: LAST_USESLOT              0
   5: JUMP_WITH_PARAMS          7904   ; [label,?_newbielogs]
   6: BRANCH                    24     ; → PC 31 (RETURN)
   7: LAST_USEITEM              0
   8: PUSH_CONSTANT_INT         55
   9: OC_PARAM                  0      ; oc_param(LAST_USEITEM, 55)
  10: PUSH_CONSTANT_INT         1
  11: BRANCH_EQUALS             1      ; if oc_param==1 → jump +1 to PC 13
  12: BRANCH                    3      ; else → jump +3 to PC 16
  13: LAST_USEITEM              0
  14: JUMP_WITH_PARAMS          6460   ; [label,?_logs_param]
  15: BRANCH                    14     ; → PC 30
  16: LAST_USEITEM              0
  17: OC_CATEGORY               0
  18: SWITCH                    0      ; switch table 0 — CASE TABLE NOT YET DUMPED
  19: BRANCH                    6      ; default → PC 26
  20: LAST_USESLOT              0
  21: JUMP_WITH_PARAMS          7356   ; [label,light_logs_inv]
  22: BRANCH                    6      ; → PC 29
  23: LAST_USEITEM              0
  24: JUMP_WITH_PARAMS          7360   ; [label,ignite_light_source]
  25: BRANCH                    3      ; → PC 29
  26: PUSH_CONSTANT_INT         0
  27: GOSUB_WITH_PARAMS         2130   ; [proc,displaymessage] arg=0
  28: BRANCH                    0
  29: BRANCH                    0
  30: BRANCH                    0
  31: RETURN                    0
  ```

### 2.3 Linked scripts disassembled

- **id 7356 `[label,light_logs_inv]`** (logs path): 105 PCs. Reads `oc_param(logs_obj, 86)` for firemaking-level requirement, runs STAT check, COORD/MAP_BLOCKED check, INV_TOTAL tinderbox check, P_STOPACTION, ANIM 733, SOUND_SYNTH 195, INV_DROPSLOT, MAP_CLOCK + VARP 58 timer for repeat-light delay, GOSUB 7359 (loc_add fire), GOSUB 7357 (xp grant), P_OPOBJ 4. **Source:** same file.
- **id 7360 `[label,ignite_light_source]`** (light_source.rs2): reached from `[opheldu,tinderbox]` PC 23-25 sibling branch (SWITCH case mapping to PC 23, e.g. unstrung-candle category); handles candle/torch lighting. Disasm captured for completeness; not on the logs path.
- **id 2130 `[proc,displaymessage]`** (general/misc): `enum(105, 115, 11, $arg0).MES`. Default arg=0 from PC 26-27 of `[opheldu,tinderbox]`.
- **id 7904 (newbielogs path), id 6460 (oc_param=1 logs path)**: not yet disassembled — Stage 1.1 deliverable.

### 2.4 ObjType cache state

- Logs: id=1511, Category=212, Members=false, Params={86: 1, 132: 400}. **Param 55 absent → `oc_param(logs, 55)` returns default value (0)**.
- Newbielogs: id=2511, Category=-1, no params.
- Tinderbox: id=590.

### 2.5 Reconstructed control flow for "tinderbox on logs"

1. Java client sends OPHELDU. Wire order (per `OpHeldUDecoder.ts`, to be re-pinned in Stage 1.1 R1 mitigation): presumably `obj=logs(1511), useObj=tinderbox(590)`.
2. Handler: arm (a) lookup `[opheldu,1511]` MISSES (not registered) → arm (b) lookup `[opheldu,590]` HITS at `[opheldu,tinderbox]` → unconditional swap → `p.lastUseItem = logs`.
3. Script PC 0-2: `LAST_USEITEM(=1511) == newbielogs(2511)` → false.
4. PC 7-11: `oc_param(logs, 55) == 1` → 0 == 1 → false.
5. PC 16-19: `SWITCH on OC_CATEGORY(logs) = 212`.
6. **EXPECTED:** SWITCH case 212 maps to PC 20 → `LAST_USESLOT` + `JUMP_WITH_PARAMS 7356 ([label,light_logs_inv])` → fire-creation flow.
7. **DEFAULT FALLBACK:** PC 26-27 → `[proc,displaymessage]` arg=0 → `enum(105, 115, 11, 0).MES`.

If the SWITCH case-212 routes correctly, control reaches `[label,light_logs_inv]` whose body produces all four observable effects (fire spawn, anim, inv_dropslot, xp). The fact that the user observes ZERO effects means execution either:

- **(a)** falls into the SWITCH default → `[proc,displaymessage](0)` (visible only as a chat MES, hidden by overlay); OR
- **(b)** reaches `[light_logs_inv]` but a downstream opcode aborts.

### 2.6 Hypothesis-status outcomes

| H | Status | Justification |
|---|---|---|
| H1.a | REFUTED | `[opheldu,tinderbox]` registered (Bundle 0 §2.1) |
| H1.b | REFUTED | Lookup key arithmetic verified (Bundle 0 §2.1) |
| H2 | REFUTED at HEAD | Goscape `handler_opheld.go:271-400` matches TS `OpHeldUHandler.ts` line-by-line; pop order, 4-arm fallback, unconditional swaps all aligned (NAI-71 closure intact) |
| **H3** | **PRIMARY** | Narrowed to one of: SWITCH case-212 mismatch, opcode silent-abort in 7356, or chain-opcode silent-abort |

---

## 3. Scope

### In scope

- **Stage 1.1 (controller disasm extension, no production change):**
  - Dump `[opheldu,tinderbox]` SWITCH-table operand (PC 18) — confirm whether case 212 maps to PC 20 (logs path) or default.
  - Disasm scripts id 7942, 7359, 7357, 2120, 6460, 7904.
  - Verify bytecode compiler version (`CompilerVersion=26`) — flag any opcode appearing in disasm that isn't in `pkg/script/opcodes.go` enumeration.
  - Re-pin Java client OPHELDU wire ordering (`obj` vs `useObj` semantics) by reading `Engine-TS/src/network/game/client/codec/OpHeldUDecoder.ts` AND `Client-Java/src/main/java/.../OpHeldUEncoder.java` rev-225.
  - Output: `docs/superpowers/investigations/2026-05-06-nai-114-stage1-bundle0-findings.md`. Committed.
- **Stage 1.2 (Sonnet audit subagent):** opcode-coverage walk for the firemaking chain against `pkg/script/handlers_*.go` registration tables and against TS `Engine-TS/src/lostcity/engine/script/handlers/`. Subagent writes audit report to `docs/superpowers/investigations/2026-05-06-nai-114-stage1-audit.md`. Controller HEAD-verifies per `audit_subagent_fabrication.md` + `verify_implementer_claims.md`.
- **Stage 2:** TDD fix landing in NAI-114. Shape determined by Stage 1 binding (see §5).
- **Stage 3 (conditional):** cascade re-investigation per `cascade_theory_smoke_binding` if Stage-2 smoke fails.
- **User-launched smoke** per `smoke_test_server_handoff` after Stage 2 commits land.
- `nai_followups.md` close entry with cascade attribution.
- `Closes memory:` trailer on close commit per `close_commit_memory_trailer`.

### Out of scope

- **NAI-111** — P_TELEJUMP `[label,tutorial_complete]` "script not protected"; pre-NAI-110 baseline; remains queued.
- **Audit of other 204 type-specific OPHELDU scripts** — only the firemaking chain is in scope.
- **Java client patch** — H2 refuted; behavior matches TS at HEAD; fix is goscape-side.
- **Adjacent divergences** surfaced by Stage-2 smoke route per `smoke_surfaces_adjacent_divergences` (≤30 LOC stretch in-scope; else NAI-115).

---

## 4. Stage 1 audit — opcode-coverage binding

### 4.1 Stage 1.1 — controller disasm extension (no production change)

**Probe restoration.** `cmd/probe-opheldu/main.go` was deleted after Bundle 0; controller will recreate it (or equivalent) under `cmd/probe-opheldu/main.go` for Stage 1.1, then delete after the investigation note commits. Probe must additionally:

- Dump `f.SwitchOperands[18]` for `[opheldu,tinderbox]` (script id resolved by name lookup `prov.GetByName("[opheldu,tinderbox]")`). If the probe's existing `Disassemble` doesn't print switch-table cases, add a `dumpSwitchTable(f, pc)` helper inline.
- Disasm scripts by ID: 7942, 7359, 7357, 2120, 6460, 7904. Use `prov.GetByID(uint32(id))`.
- Loop over goscape's enumerated opcodes (`pkg/script/opcodes.go`) and confirm every opcode appearing in the captured disasm chain has a corresponding `String()` value (sanity: no opcode renders as `Op(NN)` raw int — that would be a missing enum entry).

**Wire-order pin.** Read `Engine-TS/src/network/game/client/codec/OpHeldUDecoder.ts` (decoder) and `Engine-TS/src/network/game/client/handler/OpHeldUHandler.ts:14-15` (destructuring order). Cross-reference with `Client-Java` rev-225 OPHELDU encoder. Confirm: does `obj` field correspond to the click-target (logs) or the held-use-item (tinderbox)?

**Output:** `docs/superpowers/investigations/2026-05-06-nai-114-stage1-bundle0-findings.md`, structure:

1. SWITCH-table dump for PC 18 — enumerate every (case, target-PC) pair.
2. Full disasm of 7942, 7359, 7357, 2120, 6460, 7904.
3. Wire-order pin verdict: `obj = ?, useObj = ?`.
4. Opcode-set enumeration (deduplicated) appearing across the chain.
5. Bytecode-version sanity verdict.

**Commit:** `docs(investigation): NAI-114 Stage 1.1 — Bundle 0 findings + procs disasm`. Probe code remains uncommitted (working tree only); deleted after Stage 1.2 dispatches.

### 4.2 Stage 1.2 — Sonnet audit subagent dispatch

**Subagent:** one Sonnet audit subagent. Read-only.

**Tools:** Glob, Grep, LS, Read, NotebookRead, TodoWrite, KillShell, BashOutput. Read access to all four repos: goscape (this), `LostCityRS/Engine-TS`, `LostCityRS/Server`, `LostCityRS/Client-Java`.

**Inputs (controller-prepared):**
- This spec.
- Stage 1.1 investigation note.
- Pointer to `pkg/script/handlers_*.go` (handler registration tables).
- Pointer to `Engine-TS/src/lostcity/engine/script/handlers/` (TS handler reference).
- Pointer to `LostCityRS/Server/content/scripts/skill_firemaking/scripts/firemaking.rs2` + `light_source.rs2` (content source for context, not for translation — the bytecode is canonical).

**Deliverable:** `docs/superpowers/investigations/2026-05-06-nai-114-stage1-audit.md` containing:

1. **Opcode-coverage matrix.** One row per unique opcode appearing in the Stage 1.1 disasm chain (`[opheldu,tinderbox]` + 7356 `[light_logs_inv]` + 7942 + 7359 + 7357 + 2120 + 2130 + 6460 + 7904):

   | Opcode | TS handler (file:line) | Goscape handler (file:line) | Behavior diff (none / silent-abort / wrong-value / missing) | Bound to symptom? |
   |---|---|---|---|---|

2. **SWITCH decode audit.** Read goscape's SWITCH opcode handler (`pkg/script/handlers_*.go`, grep `OpSwitch`). Cross-check decode arithmetic against TS `Engine-TS/src/lostcity/engine/script/handlers/CoreOps.ts` SWITCH handler. Confirm whether case 212 in `[opheldu,tinderbox]` PC 18's switch table maps to the correct case-target PC offset on goscape's side.

3. **H3 binding verdict** — name the SINGLE divergent opcode (or class) responsible for the silent abort. Cite TS file:line + goscape file:line for every claim. NO "by analogy" — re-disasm if unsure (per `audit_subagent_fabrication`).

4. **Fix shape recommendation** — port new handler / fix existing handler / fix SWITCH decoder. With LOC estimate.

**If subagent reports inconclusive:** Stage 1.3 controller instrumentation round (instrument `s.runScript` to log each PC + opcode + stack-top during a real "tinderbox on logs" smoke). Per NAI-112 cadence pattern. Adds ~1 day; flag as scope risk.

**Commit:** `docs(investigation): NAI-114 Stage 1.2 — opcode-coverage audit + H3 binding`.

### 4.3 Controller HEAD-verification (post-Stage 1.2)

Per `verify_implementer_claims.md` + `audit_subagent_fabrication.md`:

- Re-grep every cited file:line against HEAD; confirm shape exists.
- Run targeted `go test` on any handler the audit names.
- For "missing handler" claims: confirm via `rg "OpXxx|opXxxOpcode" pkg/script/`.
- For "behavior diff" claims: read both TS and goscape handler bodies; confirm divergence.

If any claim fails verification → push back to subagent or re-derive controller-side.

---

## 5. Stage 2 — TDD fix (shape determined by Stage 1 binding)

### 5.1 Likely fix shapes

**Shape A — missing/buggy script-opcode handler** (most likely; precedent: NAI-110 TEXT_GENDER, NAI-87 SOUND_SYNTH, NAI-83 LOC_ANGLE).

- Red unit test in `pkg/script/handlers_<area>_test.go` using `&ScriptState{StackCapacity: N}` per `scriptstate_test_fixture_idioms`. Pin: pre-state, opcode call, post-state assertion (stack push/pop count, side-effect on Self/world, error return).
- Green: implement handler in `pkg/script/handlers_<area>.go`. Register in opcode table (typically `init()` block). Mirror TS `Engine-TS/src/lostcity/engine/script/handlers/<Area>Ops.ts` line-by-line.
- Integration: if the handler interacts with `Player`/world state, add an integration test in `modules/world/` using the existing test fixtures.
- LOC: 5-50 production + tests.

**Shape B — SWITCH decode bug.**

- Red unit test using a hand-rolled `ScriptFile` with a known switch table; assert decoded case-target PC matches.
- Green: fix decoder (likely `pkg/script/decode.go` or `pkg/script/handlers_core.go`).
- Regression scan: `rg "SWITCH" pkg/script/` in disasm output for any other affected scripts. If found → in-scope (scope is "fix the bug", not "fix every script that triggered it").
- LOC: ~20 production + tests.

**Shape C — content-loader gap** (e.g., enum 105 not loaded → ENUM opcode silently fails).

- Red test: load enum.dat fixture; assert enum 105 is in the configs slice.
- Green: fix loader (likely `pkg/objtype/enumtype.go`).
- LOC: 10-30 production + tests.

**Shape D — escalation.** If Stage 1 binds a chain of >3 missing opcodes OR a multi-handler refactor (>80 LOC), close NAI-114 as Stage-1-only and route fix to NAI-115 per `scope_gate_prerequisite_chain`. Decision-criterion at controller pre-Stage-2 review: estimate total LOC; if >80, escalate.

### 5.2 TDD discipline

Per goscape convention + `superpowers:test-driven-development`:

- One RED → GREEN → REFACTOR cycle per opcode handler.
- Pre-test the test: confirm it fails with stated reason before implementing.
- Defensive gates labeled per `defensive_gate_doc_comment_label.md`.
- TS-line citations in doc comments per `true_to_ts_gate.md`.
- Plan-author Go-name-collision check per `plan_var_name_collision.md` for any test fixtures.

### 5.3 Plan-author dispatch

Per `superpowers_clear_between_spec_and_impl.md`: brainstorming sub-spec ends with this spec doc. Next: `superpowers:writing-plans` produces the implementation plan. After plan commits, emit resume prompt and stop. User /clear before implementing.

---

## 6. Smoke handoff

User-launched per `smoke_test_server_handoff`. Goscape server running locally; user connects via Java client rev-225 and walks the Tutorial Island fire-making step.

**Pass criteria** (visible outside chat overlay):

- Fire (loc) spawns on the tile under player.
- Logs disappear from inventory side panel.
- Firemaking animation 733 plays.

**Bonus signal** (chat overlay covers): chatbox advances to next tutorial instruction.

**Fail routing:**

- **Symptom unchanged** → `smoke_unchanged_means_multiple_blockers` — multiple cascade-blockers; Stage 3 re-brainstorm.
- **Symptom shape changed (partial fire-making)** → adjacent-divergence per `smoke_surfaces_adjacent_divergences` — ≤30 LOC stretch in-scope; else NAI-115.
- **New blocker on subsequent tutorial step** → record as next NAI residual; queue NAI-115.

---

## 7. Risk register

| # | Risk | Probability | Impact | Mitigation |
|---|---|---|---|---|
| R1 | Bundle 0 wire-order assumption (`obj=logs, useObj=tinderbox`) wrong | LOW | Symptom-equivalent: arm (a) hits without swap, but BRANCH-EQUALS-newbielogs with `LAST_USEITEM=tinderbox` is also false → SWITCH on category(tinderbox) → still ends up routing through SWITCH cases. **Symptom shape unchanged.** Stage 1.1 still pins exact wire ordering. | Stage 1.1 §4.1 wire-order pin step. |
| R2 | Subagent fabricates a binding (per `audit_subagent_fabrication`) | MED | Wasted Stage-2 cycle on wrong fix; smoke would catch it but burns a day. | Controller HEAD-verification protocol §4.3. Subagent prompt explicitly forbids "by analogy" reasoning. |
| R3 | Stage-2 fix touches a hot script-opcode handler with broad reach → regressions | LOW | Test failures elsewhere | Full `go test ./pkg/script/... ./modules/world/...` per `verify_implementer_claims`. |
| R4 | Adjacent-divergence creep | MED | Scope balloon | ≤30 LOC stretch in-scope cap; >30 LOC → NAI-115 per `smoke_surfaces_adjacent_divergences`. |
| R5 | Stage 1.2 inconclusive → Stage 1.3 instrumentation needed | LOW | +1 day | Pre-budgeted; flagged as scope risk. NAI-112 has the precedent shape. |
| R6 | Multi-tile fire loc footprint regression (touches NAI-100 territory) if `[loc_add_fire]` (id 7359) is part of the binding | LOW | Test churn in `pkg/gamemap` | NAI-100 closed multi-tile loc footprint; treat any fire-loc width/length probe as out-of-scope unless directly bound. |

---

## 8. Tech stack & deliverables

- **Go 1.26+** per `go_version`.
- **TS source:** `LostCityRS/Engine-TS` per `ts_source_canonical_path`.
- **Content source:** `LostCityRS/Server` (firemaking.rs2 + light_source.rs2 located).
- **Java client:** `LostCityRS/Client-Java` rev-225 (R1 mitigation).

**Commit sequence:**

1. `docs(spec): NAI-114 — opheldu tinderbox-on-logs investigation` ← this commit.
2. `docs(plan): NAI-114 — Stage 1.1 + 1.2 + Stage 2 implementation plan` ← from `writing-plans`.
3. `docs(investigation): NAI-114 Stage 1.1 — Bundle 0 findings + procs disasm`.
4. `docs(investigation): NAI-114 Stage 1.2 — opcode-coverage audit + H3 binding`.
5. `feat/fix(script): NAI-114 Stage 2 — <fix shape A/B/C>` (TDD-staged sub-commits).
6. `chore(close): NAI-114 — Closes memory: <list>`.

**Memory updates on close** (`Closes memory:` trailer):

- Cascade-attribution entry → `nai_followups.md`.
- Any new memory entries from Stage 1/2 surfaces (e.g., new opcode-handler pattern, SWITCH decoder gotcha, content-loader gap).
- Update existing memory entries that surface as relevant during execution (e.g., `disasm_reframes_inferred_binding`, `scriptstate_test_fixture_idioms`).

---

## 9. Memory anchors

Memories that shape this spec (already applied during brainstorm):

- `investigation_subspec_cadence` — Stage 1 → Stage 2 → smoke template.
- `disasm_reframes_inferred_binding` — Bundle 0 disasm-first approach.
- `bundle0_short_circuits_stage1_audit` — Bundle 0 narrowed scope but did NOT fully bind H3, so Stage 1.2 audit still required.
- `audit_subagent_fabrication` — controller HEAD-verification gate.
- `controller_preflight` — pre-flight grep+Read against HEAD.
- `cascade_theory_smoke_binding` — Stage 3 conditional re-investigation.
- `smoke_surfaces_adjacent_divergences` — Stage 2 smoke fail routing.
- `smoke_test_server_handoff` — user-launched smoke.
- `true_to_ts_gate` — every behavioral divergence tracked.
- `scriptstate_test_fixture_idioms` — Stage 2 test fixture pattern.
- `defensive_gate_doc_comment_label` — Stage 2 doc-comment labeling.
- `superpowers_clear_between_spec_and_impl` — spec → plan → /clear → implement.
- `close_commit_memory_trailer` — `Closes memory:` trailer.
- `scope_gate_prerequisite_chain` — Shape D escalation criterion.
- `smoke_unchanged_means_multiple_blockers` — Stage 3 routing.
- `verify_implementer_claims` — post-fix verification.
