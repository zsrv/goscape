# NAI-128 Stage 3 — Production-residual binding probe (design)

**Predecessor:** Stage 2 design at `2026-05-08-nai-128-cascade-fix-stage2-design.md` (`350a982`); plan at `2026-05-08-nai-128-cascade-fix-stage2.md` (`f5e3ff8`); fixture work landed in commits `2e2f341`, `b8e20c0`, `e47c1d3`.
**HEAD at brainstorm:** `e47c1d3`.
**Tech stack:** Go 1.26+, `log/slog`, `world.Config.NodeDebug` (default `true`), no new deps.

## §1 Goal

Bind the NAI-128 production residual via **live-smoke gateway instrumentation** of the OPNPC2-attack → death → loot pipeline. After Stage 2's fixture-side fix made `TestNAI128_RatLootCascade/GroundObjs` GREEN, smoke (Man in Lumbridge, 2026-05-08) confirmed the production bug stands: the Man dies, no loot drops, no warn-level frames. Per `cascade_theory_smoke_binding`, smoke binds; fixture-only success means the production pipeline diverges from the test in a layer the test doesn't model.

**Close criterion:** smoke log captured + binding identified to one of the layers L0–L7 (§2). Stage 3 close ships a routing decision (Stage 4 of NAI-128 if scope-tight, NAI-129 if cross-cutting). Stage 3 itself does NOT ship the production fix.

**Out of scope:**
- Production fix for whatever layer binds.
- Generalizing to other NPC types — the fix in the follow-up sub-spec generalizes.
- Reverting any Stage 2 fixture work — `2e2f341` (view parity), `b8e20c0` (T6), `e47c1d3` (multi-tick driver) are correct fixture code regardless of binding.

## §2 Pre-flight findings (controller grep + read at HEAD `e47c1d3`)

- `Npc.Damage` (`modules/world/npc_masks.go:165`) is a "pure output op — no death / auto-retaliate / aggro logic. The AI sub-spec will later ship a real combat loop." (lines 162–164). Decrements HP + emits damage mask, nothing else.
- No production caller of `npc.heroPoints.AddHero` exists outside `Npc.AddHeroPoints` (`modules/world/npc_script.go:74`), which is itself only called from `handleNpcHeroPoints` (`pkg/script/handlers_npc.go:1089`) — the NPC_HEROPOINTS opcode handler.
- No production caller of `Npc.EnqueueScriptForTrigger(TriggerAiQueue2, ...)` exists. Existing callers fire `TriggerAiQueue1` via walktrigger (`npc_interaction.go:299`) or `TriggerAiQueueN` from hunt (`npc_hunt.go:340`) or via `handleNpcQueue` (NPC_QUEUE opcode at `handlers_npc.go:428`).
- `NodeDebug` defaults `true` (`modules/world/config.go:76`). User's smoke runs with it on by default — no config tweak needed.
- `Npc` carries a back-ref `server *Server` (`modules/world/npc.go:81`), set by `Server.addNpc`. Available at all G1–G3 sites.
- `worldVarsView` (`modules/world/server_varp.go:164`) holds `s *Server` and is the AddObj entry point (G6).
- `ScriptState` (`pkg/script/state.go:218`) has `NodeDebug bool` but **no `Log` field**. G5 (handleNpcFindHero) requires adding one — see §4.

The user reports the Man dies via OPNPC2 attack. Some path therefore deals damage. The smoke-binding probe will identify which.

## §3 Layer-routing table

Six gateways instrumented; binding inferred from log shape.

| # | Site | File:Line | Log key | Captures |
|---|---|---|---|---|
| **G1** | `Npc.Damage` entry | `modules/world/npc_masks.go:165` | `nai128.npc.damage` | `npc.uid`, `amount`, `dmgType`, `cur` (pre-hit HP), `new` (post-hit HP) |
| **G2** | `Npc.AddHeroPoints` entry | `modules/world/npc_script.go:74` | `nai128.heropoints.add` | `npc.uid`, `playerUID`, `amount` |
| **G3** | `Npc.EnqueueScriptForTrigger` entry | `modules/world/npc.go:329` | `nai128.npc.enqueue` | `npc.uid`, `trigger`, `delay`, `lastInt`, `queueLen` (post-append) |
| **G4** | `processNpcQueue` per-fire (post `GetByTrigger != nil`) | `modules/world/npc_script.go:521` | `nai128.npc.queuefire` | `npc.uid`, `sf.Name`, `lastInt` |
| **G5** | `handleNpcFindHero` exit | `pkg/script/handlers_npc.go:1130` | `nai128.npc.findhero` | `topUID`, `lookupNonNil`, `pushed` (0 or 1) |
| **G6** | `worldVarsView.AddObj` post-zone-write | `modules/world/server_varp.go:164` | `nai128.obj.add` | `level`, `x`, `z`, `typeID`, `count`, `duration`, `receiverID` |

| Observed | Bound | Likely root cause |
|---|---|---|
| No `nai128.` lines at all | **L−1**: NodeDebug not on, or smoke didn't hit a Man-attack — re-run |
| No G1 | **L0: damage path** — Man isn't dying through `Npc.Damage` (some other mechanism) |
| G1 yes, no G2 | **L1: heroPoints credit gap** — combat damage path bypasses ledger |
| G1+G2 yes, no G3(`TriggerAiQueue2=118`) | **L2: ai_queue2 enqueue gap** — death-cascade trigger never queued |
| G3(118) yes, no G4(`[ai_queue2,…]`) | **L3: queue-dispatch gap** — entry queued but `processNpcQueue` doesn't fire it |
| G4(ai_queue2) yes, no G3(`TriggerAiQueue3=119`) | **L4: cascade enqueue gap** — `npc_default_damage` doesn't enqueue ai_queue3 |
| G4(ai_queue3) yes, G5 fires `pushed=0` | **L5: NPC_FINDHERO gap** — ledger empty or lookup miss at cascade time |
| G5 fires `pushed=1`, no G6 | **L6: obj_add never reached** — opcode between FINDHERO and OBJ_ADD blocked |
| G6 fires, but client shows no loot | **L7: post-add zone-broadcast gap** — obj added but never broadcast to client |

L7 is the most ambiguous: G6's log line proves obj entered server-side zone state, but the client may not see it for reasons unrelated to NAI-128 (zone-update encoder, rsbuf state, client filter). If L7 binds, follow-up routes to a separate sub-spec scoped to zone-broadcast.

## §4 Architecture

**Plumbing change (one-line additions, except G5 which adds a field):**

- G1, G2, G3, G6: each site adds `if s.cfg.NodeDebug { s.log.Info("nai128.<key>", attrs...) }` (or `n.server.cfg.NodeDebug` / `w.s.cfg.NodeDebug` per site).
- G4: same shape inside `processNpcQueue` after the `sf == nil` check.
- G5: requires plumbing because `pkg/script` has no logger field. Add `Log *slog.Logger` to `ScriptState` (`pkg/script/state.go`) right next to `NodeDebug`. Wire it from the existing `s.log` field in `buildNpcScriptState` (`npc_script.go:310-320`) and `buildPlayerScriptState` (counterpart in `script.go`). G5 then uses `if s.NodeDebug && s.Log != nil { s.Log.Info(...) }`.

The `Log` field is wired **only** in goscape state-builders. pkg/script tests can leave it nil — the `s.Log != nil` guard handles that. No fan-out into existing pkg/script tests.

**Lifecycle:** all six gateways ship as **permanent NodeDebug-gated diagnostics**. They are zero-cost when NodeDebug is off (single nil-check + bool branch). Future debugging benefits from the same diagnostics being available without re-instrumentation.

**Smoke procedure:**
1. User runs server with `tee` to capture log:
   ```
   CGO_ENABLED=0 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go run -trimpath ./cmd/goscape --config.file config.yaml 2>&1 | tee /tmp/nai128-stage3.log
   ```
2. User connects with Client-Java #225, walks to a Lumbridge Man (any newbieman at e.g. (3221, 3219, 0) or similar), attacks until death.
3. User filters log: `grep nai128 /tmp/nai128-stage3.log` → pastes back to controller.
4. Controller matches against §3 table → Stage 3 close commit cites bound layer.

**Expected log volume:** ~5–20 lines per Man kill (one G1 per damage tick over ~3–5 ticks, one or two G2/G3, one G4 per queue fire, one or two G5 per cascade, two G6 if loot fires).

## §5 Test strategy

No new automated tests in Stage 3. The probe is live-smoke binding only. Verification:
- Existing `TestNAI128_RatLootCascade` (incl. `CascadeDispatchTrace` regression gate) stays GREEN — gateways add log calls, no behavior change.
- Existing `TestNAI128_RatLootCascade/CascadeDispatchTrace` continues to assert no warn-level "npc script execute error" frames during the test's manual cascade — provides crash-side regression coverage for the new ScriptState.Log field.
- Full `./...` regression sweep clean.

The Stage-2 close criterion (`T6 + T5 GREEN`) remains intact at end of Stage 3.

## §6 Risks

- **R1 — Log floods other tick output.** The `nai128.` prefix on every key makes filtering trivial via `grep nai128`. Mitigation: spec procedure pre-specifies the grep.
- **R2 — Multiple deaths overwrite the binding signal.** Mitigation: smoke procedure says "attacks ONE Man only"; if log is unclear, restart server between attempts.
- **R3 — `NodeDebug=false` accidentally.** Default is `true`; user's `config.yaml` doesn't override it; spec procedure includes a one-line check.
- **R4 — Smoke-side bug masks server signal** (e.g. user disconnects mid-fight). Mitigation: server log is the binding signal; robust to client-side issues.
- **R5 — `s.log` nil at one of the gateway sites.** Production wires it via `dskit/services` setup. Pre-flight grep confirms `s.log` non-nil for the running server. Test fixtures use `discardLogger`, also non-nil. Defensive `if s.log != nil` guards add resilience without cost.
- **R6 — `ScriptState.Log` field breaks pkg/script unit tests.** Mitigation: `Log` initialized via state-builders only; nil-friendly guard at G5 site (`s.Log != nil`); existing tests continue to work with zero-value state.
- **R7 — Bound layer is L7 (zone-broadcast)**, which routes to a separate sub-spec rather than cleanly closing Stage 3. Mitigation: §3 explicitly accepts L7 routing; the close commit body cites the layer regardless.

## §7 Out of scope / deferred

- Production fix for any layer binding (Stage 4 / NAI-129).
- Generalizing the fix beyond Man (the bound fix carries over).
- Reverting Stage 2 fixture work.
- Combat-system completeness audit. The gateways may surface that combat itself is incomplete (e.g. L0 binds because `Npc.Damage` is never called by combat — implying combat opcodes are missing). That broader question routes to a future combat sub-spec; Stage 3 closes on layer-identification only.

## §8 Close criterion

1. Six gateway probes shipped, NodeDebug-gated, behind `s.log.Info`.
2. `ScriptState.Log` field added, wired from goscape state-builders, nil-friendly at G5.
3. `TestNAI128_RatLootCascade` (all 6 subtests) GREEN at HEAD.
4. `./...` regression sweep clean.
5. User smoke captured: `grep nai128 /tmp/nai128-stage3.log` output pasted to controller.
6. Bound layer identified (L0–L7 per §3 table) and recorded in close commit body.
7. `Closes memory:` trailer per `close_commit_memory_trailer`.
8. Routing call shipped: Stage 4 (NAI-128) or NAI-129 fresh sub-spec.

## §9 Memory entries to apply

Active during this work:
- `cascade_theory_smoke_binding` — smoke binds residual; this whole sub-spec exists because Stage 2 fixture passed but smoke failed.
- `controller_preflight` — exercised already at brainstorm time; surfaced the no-production-caller findings in §2.
- `verify_implementer_claims` — implementer must show `grep nai128`-clean fixture-test runs (no spurious gateway logs from automated tests; gateways are NodeDebug-gated and tests use `discardLogger`).
- `smoke_test_server_handoff` — user launches server.
- `close_commit_memory_trailer` — Stage 3 close trailer.
- `investigation_subspec_cadence` — Stage 3 of "Stage 1 audit → Stage 2 fix → smoke → conditional Stage 3 on smoke failure" reference cadence.
- `post_task_handoff` — at close, save bound-layer info to memory + emit resume prompt for Stage 4 / NAI-129.

New memory at Stage 3 close (one of):
- "NodeDebug-gated gateway probe pattern" — generalizable for future smoke-binding investigations: identify N suspected gateways in a pipeline, log one line each behind an existing debug gate, route smoke output back via `tee`+`grep`. Reusable shape.

(The specific bound layer + production fix is a follow-up sub-spec memory, not Stage 3's.)
