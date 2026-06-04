# NAI-122 — AI_SPAWN dispatch ordering: V-PARTIAL `%npc_combat_xp_multiplier` zero-read fix

**Date:** 2026-05-07
**Status:** Spec (pre-plan)
**Predecessor:** NAI-121 (closed `a17ed5d`); Bundle 2 V-PARTIAL audit `4837354`.
**Cadence:** investigation sub-spec (`investigation_subspec_cadence`); Bundle-0 short-circuit (`bundle0_short_circuits_stage1_audit`); cascade-bound close (`cascade_theory_smoke_binding`).
**Tech stack:** Go 1.26+.

---

## 1. Problem & PRIMARY close

NAI-121 user-launched smoke (2026-05-07) confirmed PRIMARY (`"It's not after you."` gate no longer fires) but surfaced 4 adjacent residuals. The load-bearing one carried forward as NAI-122 is the V-PARTIAL parked since NAI-120: on Tutorial Island giant-rat first-tick attack, the global varn `%npc_combat_xp_multiplier` reads `0`, so combat XP multiplies to 0 and (per smoke residual #1) damage formula multiplies to 0. The `[ai_spawn,_]` global trigger at `Content/scripts/npc/scripts/ai_spawn.rs2:1-3` is supposed to populate this varn from `npc_param(combat_xp_multiplier)` on every NPC spawn.

NAI-121 Bundle 2 audit (`docs/superpowers/investigations/2026-05-07-nai-121-vpartial-findings.md`) verified the dispatch path holds and identified two un-disambiguated scenarios:

- **Scenario A (tick-phase ordering bug):** `tick.go:40 processInteractions` runs before `tick.go:42 processNpcEventQueue`. On the first tick after an NPC spawns, combat reads the varn before AI_SPAWN writes it. The audit-claimed TS contrast is `World.ts:1284-1289` running AI_SPAWN inline within `addNpc` (un-verified by controller as of NAI-121 close, flagged for NAI-122 brainstorm).
- **Scenario B (content/data):** `combat_xp_multiplier` may be absent from giant-rat's `NpcType.Params` map; `paramLookup` falls back to `DefaultInt = 0`. AI_SPAWN writes 0 correctly because the source is 0.

**PRIMARY close criteria (engine-fix path):** `%npc_combat_xp_multiplier` reads its content-pack-defined non-zero value before any combat read on the first tick after spawn. User-launched smoke confirms non-zero damage on Tutorial Island giant-rat first attack.

**PRIMARY close criteria (content path):** if Bundle 0 confirms Scenario B, NAI-122 closes near-zero-LOC with the engine deemed correct; the symptom routes to a content-pack rebuild item outside this sub-spec.

**SECONDARY (cascade-bound, per `cascade_theory_smoke_binding`):** smoke binds which NAI-121 residuals are cascade-resolved by the dispatch fix:

- **#1 — zero damage with XP awarded.** Most likely cascades from the V-PARTIAL itself (combat-damage formula multiplies by `%npc_combat_xp_multiplier`).
- **#2 — "Someone else is fighting that" single-attacker contention.** Possibly cascades; possibly separate.
- **#3 — NPC non-retaliation.** Almost certainly separate engine subsystem (AI_HUNT / AI_ATTACK retaliation triggers).

In-scope-stretch any cascade-resolved residual at ≤30 LOC per `smoke_surfaces_adjacent_divergences`. Route persistent residuals to NAI-123+.

**Out of scope:** combat damage roll, single-attacker engagement gating, NPC retaliation AI subsystem, weapon-equip rendering (NAI-119), run-mode visuals (NAI-117), firemaking ashes (NAI-115), `inv_dropitem_delayed` (NAI-120 Bundle 2E parked).

---

## 2. Cadence & bundle structure

Cadence (α): Bundle 0 (controller pre-flight, no commits) → Bundle 1 (Stage 2 fix, subagent-driven TDD) → smoke handoff → conditional Bundle 2 templated for smoke-failure residuals.

### Bundle 0 — Controller pre-flight (no commits, no subagent dispatch)

Two parallel reads:

**B0.1 — Static disambiguation probe (Scenario A vs B).**

1. Author a throwaway Go test, e.g. `modules/world/aispawn_probe_test.go`. Test loads `NpcType` registry from the production data path and prints, for the giant-rat typeId:
   - `Params[paramKey("combat_xp_multiplier")]` (present/absent + value if present).
   - `ParamType("combat_xp_multiplier").DefaultInt` (the fallback value).
   - For sanity: dump the same for a second NPC (e.g. Lumbridge man) to verify the param is set somewhere in the content pack.
2. Run with `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run AiSpawnProbe -v ./modules/world/`.
3. Capture output to `docs/superpowers/investigations/2026-05-07-nai-122-bundle0-findings.md`. Delete the throwaway test before any production commit.
4. Outcome:
   - **Scenario A confirmed** — param present with non-zero value → engine bug; proceed to B0.2 + Bundle 1.
   - **Scenario B confirmed** — param absent OR value=0 → not an engine bug; close NAI-122 with no production change; route to content-pack rebuild as a separate item.
   - **Inconclusive** (e.g., loader signature mismatch, param-key encoding ambiguity) — fall back to a temporary production probe gated behind a debug flag; revisit cadence.

**B0.2 — TS-source verification (only if Scenario A).**

1. Read `LostCityRS/Engine-TS/.../World.ts` at the audit-claimed line range (`1284-1289`) and the `addNpc` neighborhood (typically a few dozen lines around the AI_SPAWN dispatch site).
2. Capture verbatim TS code + line numbers in the Bundle 0 findings doc.
3. Three branches:
   - TS sync-inline dispatch within `addNpc` → fix shape **(a) sync dispatch** is TS-fidelity. Lock for Bundle 1.
   - TS uses a deferred queue but flushes pre-interactions → fix shape **(c) split-queue** is TS-fidelity. Lock (c); (a) becomes a deviation if chosen.
   - TS matches goscape's current structural-lag shape → V-PARTIAL is **not** an engine bug; close NAI-122 with no-op; symptom must have another root cause; route #1 to NAI-123 as a fresh damage-formula investigation.
4. Per `audit_subagent_fabrication`, the controller does this read directly (no subagent), captures verbatim excerpts, and does not rely on any earlier audit's line citations.

### Bundle 1 — Stage 2 fix (subagent-driven TDD)

Materialized only if B0.1 confirms Scenario A AND B0.2 locks an engine-side fix shape. Default shape **(a) sync dispatch in `addNpc`**:

**B1.T1 — Replace queue-append with sync dispatch.**

- File: `modules/world/npc_registry.go:88-99`.
- Before:
  ```go
  if s.scriptProvider != nil && n.typ != nil {
      sf := s.scriptProvider.GetByTrigger(
          script.TriggerAiSpawn, n.typeId, n.typ.Category)
      if sf != nil {
          s.npcEventQueue = append(s.npcEventQueue,
              NpcEventRequest{
                  Type:   NpcEventSpawn,
                  Script: sf,
                  Npc:    n,
              })
      }
  }
  ```
- After (target shape; plan-author finalizes signature against HEAD):
  ```go
  if s.scriptProvider != nil && n.typ != nil {
      sf := s.scriptProvider.GetByTrigger(
          script.TriggerAiSpawn, n.typeId, n.typ.Category)
      if sf != nil {
          s.runNpcScript(sf, n, nil, nil, nil)
      }
  }
  ```
- Comment update: replace `AI_SPAWN trigger queue (matches TS World.ts:1284-1289)` block with `AI_SPAWN sync dispatch (TS World.ts:<verified-lines>)`.
- Tests pinned by B1.T2.

**B1.T2 — TDD pin: sync execution observable.**

- New test (e.g. `TestAddNpc_FreshSpawn_RunsAiSpawnSynchronously`) registers a fake AI_SPAWN script that mutates an observable (e.g. `*Npc.varnsString[0]` via `SetNpcVarNString`); calls `addNpc`; asserts mutation observable *immediately on return*, before any tick advance.
- Companion test: `TestAddNpc_FreshSpawn_AiSpawnNotEnqueued` — assert `len(s.npcEventQueue) == 0` for a typeId with a registered AI_SPAWN script (confirms the queue producer is removed).
- Red→green→commit cycle.

**B1.T3 — Re-verify NAI-121 PRIMARY pin under new code path.**

- `TestAddNpc_FreshSpawn_PlayerUidVarnReadsMinusOne` (existing, NAI-121-T3 commit `4b88eaf`) must still pass. The order `resetEntityForRespawn` → AI_SPAWN dispatch is preserved (lines 79 → 88 of `npc_registry.go`); sync replacement keeps that order. No code change in this task; verification only. Bundle 1 reviewer flags green.

**B1.T4 — Reentrancy / boot-storm audit (controller pre-flight, no commit).**

- Grep `s\.addNpc\(` to enumerate call sites; verify none is reachable from inside `runNpcScript` (i.e., no opcode handler that AI_SPAWN scripts could invoke calls back into `addNpc`).
- Grep `npc_add` opcode handler. If it calls `s.addNpc` directly and AI_SPAWN scripts can invoke it, escalate: either (i) recursion guard via a "during-this-call deferred queue" pattern, or (ii) pivot to fix shape (c).
- If clean, document in Bundle 1 plan-author findings; no code change.

**B1.T5 — Boot-storm timing sanity check (controller, no commit).**

- One-shot: `time.Now()` instrument the world-spawn boot path (or run with existing logging). Assert <1s overhead added by sync AI_SPAWN dispatches at world load. If overhead is unacceptable, escalate to fix shape (c) where boot dispatches still go through a queue but flush before `processInteractions`.

**Bundle 1 close gate:**

- Cross-package `go test ./...` + `go vet ./...` + `go build ./...` green at HEAD.
- All NAI-121 tests still green.
- Sonnet code-reviewer (`superpowers_code_reviewer_model`) pass over Bundle 1 commits — conditional ✅ or fixes landed pre-smoke.
- Tracked deviation D1 declared with retire condition.

### Fallback — Bundle 1 fix shape (c) (split-queue + pre-flush phase)

Materialized only if B0.2 picks (c) OR B1.T4/T5 escalate.

- New `npcSpawnQueue []NpcEventRequest` field on `*Server`, separate from `npcEventQueue`.
- `addNpc` enqueues to `npcSpawnQueue` (replaces the current queue-append target).
- New `(*Server).processNpcSpawnQueue()` method, structurally mirroring `processNpcEventQueue` for the SPAWN subset.
- `tick.go` insertion: `s.processNpcSpawnQueue()` between `s.processWorldQueue()` (line 36) and `s.processActiveScripts()` (line 37). Rationale comment: "AI_SPAWN must drain before `processInteractions` so combat reads on the spawn tick see varns populated."
- `processNpcEventQueue` retains AI_DESPAWN exclusively (re-typed or guarded as Type==NpcEventDespawn only).
- Tracked deviation D2: SPAWN-vs-DESPAWN dispatch asymmetry; retire on TS unifies.
- Tests: replace B1.T2 sync-observable assertion with "queue drained before tick.processInteractions" — e.g. test that runs one full tick cycle and asserts AI_SPAWN side-effect observable when `processInteractions` would read it.

### Bundle 2 — Conditional, materialized only on smoke failure

Templated, not pre-decomposed. If smoke shows residual #1 (zero-damage) persists despite `%npc_combat_xp_multiplier` reading correctly:

- Controller decides at smoke-failure between (i) probe inside damage-formula handler chain (`Content/scripts/player/scripts/player_combat.rs2`-adjacent + engine-side `combat`-named handlers), or (ii) route to NAI-123.
- Per `smoke_surfaces_adjacent_divergences`: in-scope-stretch only if ≤30 LOC fix surface; else NAI-123.
- Same routing applies to residuals #2 (single-attacker) and #3 (no-retaliation), expected to be route-forward.

### Smoke handoff

- User-launched per `smoke_test_server_handoff` (Claude's sandboxed `go run` is not reachable from the host Java client).
- Build instructions in plan: `CGO_ENABLED=0 go build -trimpath -o /go/bin/goscape ./cmd/goscape`.
- Smoke flow: log in on Tutorial Island → walk to giant rat → attack → bind on first hit's damage value + XP awarded.
- Smoke binds cascade scope. Per `cascade_theory_smoke_binding`, residuals that do not silence on this fix are not cascades and route forward.

---

## 3. Tracked deviations

Provisional; finalized by Bundle 0 outcome.

- **DEVIATION-NAI-122-D1 (provisional, fix shape (a)):** Pre-tick synchronous AI_SPAWN script execution at server boot. Goscape sync-dispatches inside `addNpc` while `s.currentTick == 0` and before any `processX` phase has run. Observable: AI_SPAWN side-effects visible immediately on `addNpc` return. Pinned by `TestAddNpc_FreshSpawn_RunsAiSpawnSynchronously`. Retire condition: only if a future content-pack AI_SPAWN script needs phase-dependent state (e.g., reading `%world_currenttick` or depending on `processWorldQueue` having flushed); evidence will surface as a content-script test failure or a new V-PARTIAL.
- **DEVIATION-NAI-122-D2 (provisional, fix shape (c) only):** AI_SPAWN-vs-AI_DESPAWN dispatch asymmetry — AI_SPAWN drains in a dedicated phase before `processInteractions`; AI_DESPAWN remains in the original `processNpcEventQueue` slot at `tick.go:42`. Retire condition: TS unifies SPAWN/DESPAWN dispatch shape, OR AI_DESPAWN gets re-ordered for an unrelated reason.
- **DEVIATION-NAI-122-D3 (provisional, only if TS verification disagrees with NAI-121 audit claim):** Reframe of NAI-121 audit attribution; updated TS line citations recorded in Bundle 0 findings.

Pre-existing deviations carried forward: NAI-121 D1–D5 unchanged; NAI-119/NAI-117/NAI-115/NAI-120 carryover items unchanged.

---

## 4. Risks

- **R1 — Pre-tick script side effects (fix (a)).** AI_SPAWN scripts at server boot run before `processWorldQueue` has fired. Bundle 1 plan-author must `rg "\[ai_spawn," LostCityRS/Content/scripts/` (the canonical Content path under `LostCityRS`) and read each AI_SPAWN script to verify pre-tick safety. Specifically: scripts must not read `%world_currenttick` and must not depend on `processWorldQueue` having flushed. Evidence captured in plan-author findings.
- **R2 — World-spawn boot storm.** `addNpc` is called for every world-spawn NPC at NewServer; sync dispatch runs every AI_SPAWN script inline at boot. B1.T5 timing sanity check gates: <1s overhead acceptable; >1s escalates to fix shape (c).
- **R3 — `runNpcScript` reentrancy.** If `npc_add` opcode handler calls `s.addNpc` and AI_SPAWN scripts can invoke `npc_add`, sync dispatch creates recursion. B1.T4 grep+audit gates: clean → proceed (a); reachable → recursion guard or pivot (c).
- **R4 — TS verification disagrees with NAI-121 audit claim.** Mitigated by Bundle 0 explicit controller-side TS read; Bundle 0 findings doc captures verbatim TS excerpts; outcome routes the spec.
- **R5 — Bundle 0 returns Scenario B.** NAI-122 closes near-zero-LOC; symptom routes to content-pack work; residual cluster not fixed by this sub-spec; NAI-121 V-PARTIAL re-park entry retires.
- **R6 — Recurrence of NAI-121 PRIMARY pin under new dispatch path.** Sync dispatch order vs `resetEntityForRespawn`: current code preserves the order (reset first, AI_SPAWN second); B1.T3 verifies the existing test still passes.
- **R7 — Bundle 1 implementer claim drift.** Per `verify_implementer_claims` and `controller_preflight`: every Bundle 1 task gated on grep+Read pass at HEAD before implementer dispatch; independent fresh `go build ./...` + `go test ./...` after each implementer commit; commit-content vs claimed-diff verified via `git show <SHA> --stat` per `implementer_commit_content_verify`.

---

## 5. File inventory (provisional)

Finalized in plan after Bundle 0 outcome.

**Engine-fix path (a):**

- `modules/world/npc_registry.go` — sync-dispatch replacement at lines 88-99; comment update.
- `modules/world/npc_registry_test.go` (or new `aispawn_dispatch_test.go`) — `TestAddNpc_FreshSpawn_RunsAiSpawnSynchronously`, `TestAddNpc_FreshSpawn_AiSpawnNotEnqueued`. Existing NAI-121 tests verified, not modified.
- Bundle 0 throwaway: `modules/world/aispawn_probe_test.go` (deleted before any commit).
- Bundle 0 findings: `docs/superpowers/investigations/2026-05-07-nai-122-bundle0-findings.md` (commits with Bundle 0 closure).

**Engine-fix path (c) fallback (additional/alternative):**

- `modules/world/server.go` (or wherever `Server` struct is defined) — add `npcSpawnQueue []NpcEventRequest` field.
- `modules/world/npc_event_queue.go` — factor or add `processNpcSpawnQueue()`; AI_DESPAWN-only guard on `processNpcEventQueue`.
- `modules/world/tick.go` — insert `s.processNpcSpawnQueue()` between lines 36-37.
- `modules/world/npc_registry.go` — queue target switched from `npcEventQueue` to `npcSpawnQueue` (still queue-based).
- Tests adjusted: pin "drained before processInteractions" rather than sync-on-return.

**Content path (B):**

- No production code change. Close NAI-122 with a no-op commit referencing Bundle 0 findings; route content rebuild as a separate sub-spec (NAI-123 or content-pack-driven item).

---

## 6. Test strategy

Per `plan_test_coverage_crosscheck`: every plan task that ships a code change has a test pin; every test below has a corresponding plan task.

**Engine-fix path (a):**

- **T1 sync-execution pin** (`TestAddNpc_FreshSpawn_RunsAiSpawnSynchronously`): fake AI_SPAWN script + observable mutation; assert mutation visible on `addNpc` return.
- **T2 queue-removal pin** (`TestAddNpc_FreshSpawn_AiSpawnNotEnqueued`): `len(s.npcEventQueue) == 0` after `addNpc` for typeId with AI_SPAWN registered.
- **T3 NAI-121 PRIMARY pin re-verified**: `TestAddNpc_FreshSpawn_PlayerUidVarnReadsMinusOne` passes against new code path (no test change).
- **T4 AI_DESPAWN coverage holds**: existing `processNpcEventQueue` AI_DESPAWN tests green (verify, don't modify).
- **T5 reentrancy guard** (only if B1.T4 surfaces a path): test that exercises `addNpc` reachable from `runNpcScript`; pin guard behavior.

**Engine-fix path (c):**

- **T1' drain-before-interactions pin**: full-tick simulation; assert AI_SPAWN side-effect observable when `processInteractions` reads.
- **T2' queue-routing pin**: `addNpc` writes to `npcSpawnQueue`, not `npcEventQueue`.
- **T3' AI_DESPAWN routing pin**: AI_DESPAWN producer (npc_ai.go:47-58) writes to `npcEventQueue`, not `npcSpawnQueue`.
- **T4' phase-ordering pin**: `processNpcSpawnQueue` runs before `processInteractions` in tick (test against tick.go ordering, e.g. via observability counters or test-injected hooks).
- T3 + T4 from path (a) carry over.

**Plan-coverage crosscheck:** controller diffs spec test list against each plan task's test block at plan-write time per `plan_test_coverage_crosscheck`.

---

## 7. Memory entries applied

- `investigation_subspec_cadence` — Bundle 0 controller pre-flight → Bundle 1 fix → smoke → conditional Bundle 2.
- `bundle0_short_circuits_stage1_audit` — NAI-121 Bundle 2 already did Stage 1; no audit subagent in NAI-122.
- `audit_subagent_fabrication` — Bundle 0 TS read done by controller, verbatim excerpts; NAI-121 audit's `World.ts:1284-1289` claim explicitly re-verified.
- `controller_preflight` — every Bundle 1 task gated on grep+Read pass at HEAD before implementer dispatch.
- `verify_implementer_claims` — independent fresh `go build ./...` + `go test ./...` after each implementer.
- `implementer_commit_content_verify` — `git show <SHA> --stat` after each commit.
- `cascade_theory_smoke_binding` — close criteria binds on smoke; SECONDARY = whatever cascade-resolves.
- `smoke_surfaces_adjacent_divergences` — residuals routed by 30-LOC threshold at smoke-failure time.
- `smoke_test_server_handoff` — smoke is user-launched.
- `dispatch_correct_reach_blocked` — close PRIMARY on engine-side dispatch fix even if downstream subsystems (residuals #2/#3) remain broken.
- `superpowers_code_reviewer_model` — every reviewer subagent on Sonnet, never Opus.
- `subagent_driven_development` — Bundle 1 cadence (fresh Sonnet implementer per task; spec-then-quality reviewer subagent per task).
- `true_to_ts_gate` — Bundle 0 explicit TS read before locking fix shape; D1/D2/D3 declared with retire conditions.
- `defensive_gate_doc_comment_label` — any goscape-only defensive checks added in Bundle 1 labeled per the convention.
- `plan_test_coverage_crosscheck` — controller diffs spec test list against plan task test blocks at plan-write.
- `plan_runnable_test_fixtures` — every plan-codified test mentally pre-executed before dispatch.
- `dead_api_polish` — sync-dispatch replacement deletes the queue-append branch; if `NpcEventSpawn` becomes a dead enum value (no other producer), retire it in the same task.
- `close_commit_memory_trailer` — applied on close commit.

---

## 8. Close criteria (summary)

**Bundle 0 close:**
- Bundle 0 findings doc committed with verbatim probe output + verbatim TS excerpts.
- Outcome routing locked: (a) sync, (c) split-queue, (B) close-no-op, or escalate.

**Bundle 1 close (conditional on engine-fix path):**
- Cross-package green (`go test ./...`, `go vet ./...`, `go build ./...`).
- New tests green; NAI-121 tests still green.
- Sonnet code-reviewer pass.
- Tracked deviation declared.

**Smoke close (PRIMARY):**
- User-launched smoke confirms `%npc_combat_xp_multiplier` reads correctly OR (Scenario B path) closes near-zero-LOC.
- Tutorial Island giant-rat first-tick attack deals non-zero damage.
- Cascade binding: residual #1 closes if cascade-resolved; residuals #2/#3 routed per smoke evidence + 30-LOC threshold.

**Carry-forward from NAI-122 close:**
- Whichever residuals remain unsilenced (likely #2, #3, possibly #1 if non-cascade).
- NAI-119 / NAI-117 / NAI-115 carryovers unchanged.
- D1 (or D2/D3) retire conditions registered for grep at future NAI brainstorms.
