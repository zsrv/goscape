# NAI-128 Stage 2 — Cascade-fix design

**Predecessor:** Stage 1 binding probe at `2026-05-08-nai-128-rat-loot-cascade-investigation-design.md` (`6f59550`); findings at `docs/superpowers/findings/2026-05-08-nai-128-stage1-findings.md`.
**HEAD at brainstorm:** `e61b5f8`.
**Tech stack:** Go 1.26+, no new deps.

## §1 Goal

Bind the Stage-1 candidate-E failure to a specific production root cause and apply the fix. Close criterion: `TestNAI128_RatLootCascade/GroundObjs` GREEN at HEAD, plus Java-client smoke confirming live rat loot drops in Lumbridge.

T6 (`CascadeDispatchTrace`) is added as a permanent layered diagnostic regression gate alongside T5.

## §2 Stage-1 binding recap

T1–T4 PASS, T5 FAIL: `z.Objs` length = 0 at rat coord post-cascade. Stage 1 refuted candidates B, C, D, E1, NAI-127-§6, and confirmed phase-collapse (single `processNpcQueue` drains both ai_queue2 and re-entered ai_queue3).

The Stage-1 findings doc framed the residual as two sub-candidates (E2a, E2b). **Controller pre-flight for this Stage 2 brainstorm expanded the residual to three:**

- **E0** — `s.scriptProvider.GetByTrigger(TriggerAiQueue3, ratTypeID, ratType.Category)` returns nil. `processNpcQueue` (`modules/world/npc_script.go:514-519`) silently `continue`s on nil — no log, no error.
- **E2a** — Script runs, `NPC_FINDHERO` returns 0 silently. Both `[ai_queue3,newbiegiantrat]` and the `[proc,npc_default_death]` fallback gate `obj_add` behind `if (npc_findhero = ^true)`.
- **E2b** — Script runs, `NPC_FINDHERO` returns 1, but a downstream opcode errors. `resumeOrFinishNpc` (`npc_script.go:380`) logs at Warn level and falls through; the test fixture's `discardLogger` swallows this signal.

## §3 Pre-flight finding: fixture-parity gap

`newTestServer` (`modules/world/server_test.go:311`) and `nai128CacheFixture` (`modules/world/nai128_rat_loot_test.go:18`) **do not initialize the script-side adapter views** that production `NewServer` (`modules/world/server.go:236-252`) wires before tick-loop start:

```go
// Production NewServer init, missing from test fixtures:
s.vars        = make([]int32, len(varsTypes.Configs))
s.varsStrings = make([]string, len(varsTypes.Configs))
s.worldVars   = worldVarsView{s: s}
s.configsView = serverConfigsView{s: s}
s.invLookup   = invLookupView{s: s}
s.npcLookup   = serverNpcLookup{s: s}
```

`buildNpcScriptState` (`npc_script.go:315-319`) wires these into `ScriptState.World/Configs/Inv/Npcs`. With zero-valued `worldVarsView{s: nil}`, `worldVarsView.LookupPlayerByUID` short-circuits at `server_varp.go:178-180` (`if w.s == nil { return nil }`) regardless of `playerLoop` state. This **fully explains the observed T5 failure as fixture noise**, not a production bug.

The smoke confirms a real production bug exists, but T6 against the current fixture would bind against the fixture issue, not the production issue. Phase A below remediates this before any binding work.

## §4 Architecture / phases

### Phase A — Fixture parity (always runs)

Patch `nai128CacheFixture` to mirror production view init. Required additions, in order matching `server.go:215-252`:

1. Load `varsTypes`, `varnTypes` via `objtype.LoadVarsTypes`, `objtype.LoadVarnTypes` (server.go:225-231).
2. `s.varsTypes = varsTypes`; `s.varnTypes = varnTypes`.
3. `s.vars = make([]int32, len(varsTypes.Configs))`; `s.varsStrings = make([]string, len(varsTypes.Configs))`.
4. `s.worldVars = worldVarsView{s: s}`.
5. `s.configsView = serverConfigsView{s: s}`.
6. `s.invLookup = invLookupView{s: s}`.
7. `s.npcLookup = serverNpcLookup{s: s}`.

`s.invTypes` load (`objtype.LoadInvTypes`) is **optional**: include only if Phase B re-run exposes a cascade dependency on it. `s.renderer` is omitted — cascade does not exercise rsbuf rendering.

### Phase B — Re-run T5 (gate)

Run `go test ./modules/world/ -run TestNAI128_RatLootCascade`.

- **T5 GREEN** → fixture parity was the only obstacle. Commit Phase A. Skip Phase C. Proceed to Phase D.
- **T5 RED** → cascade fails against parity-faithful fixture. Production bug confirmed. Proceed to Phase C.

### Phase C — T6 probe + fix (conditional on Phase B failure)

#### T6 design

New subtest `CascadeDispatchTrace`, inserted **after** `AiQueueCascade` (so it observes captured cascade records) and **before** `GroundObjs` (so binding diagnostics print regardless of T5 state).

Recorder setup lives in the parent test body (between `Preconditions` registration and the `t.Run("AiQueueCascade", …)` call), so the recording handler is wired into `s.log` before `processNpcQueue` runs. T6 itself only reads the recorder + does the static GetByTrigger probe + asserts.

```go
// Sketch — exact form during plan-write:
type recordingHandler struct {
    mu      sync.Mutex
    records []slog.Record
}
func (h *recordingHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }
func (h *recordingHandler) Handle(_ context.Context, r slog.Record) error {
    h.mu.Lock(); h.records = append(h.records, r.Clone()); h.mu.Unlock(); return nil
}
func (h *recordingHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }
func (h *recordingHandler) WithGroup(_ string) slog.Handler     { return h }
```

Parent test body (before `t.Run("AiQueueCascade", …)`):
```go
rec := &recordingHandler{}
s.log = slog.New(rec)
```

T6 subtest body (after `AiQueueCascade` returns):

1. **Static probe (pre-cascade-equivalent):** `sf := s.scriptProvider.GetByTrigger(script.TriggerAiQueue3, ratTypeID, ratType.Category)`. Assert `sf != nil`. Log resolved script name. **nil → E0 bound.**
2. **Recorder readout:** filter `rec.records` for `r.Level == slog.LevelWarn && r.Message == "npc script execute error"`. Log each record's `err` attr.
3. **Binding logic:**
   - `sf == nil` → **E0 bound.**
   - `sf != nil` AND warn-records non-empty → **E2b bound** (err string identifies opcode).
   - `sf != nil` AND warn-records empty AND T5 still 0 objs → **E2a bound.**

First run is diagnostic-only (`t.Logf` + `t.Errorf` on the binding outcome). After fix, T6 flips to positive contract:
- `sf != nil`
- zero Warn-level "npc script execute error" records during cascade

#### Conditional fix sketches

| Bound | Likely root cause | Fix locus |
|---|---|---|
| **E0** | Trigger ID mismatch / category lookup gap. `[ai_queue3,newbiegiantrat]` registered in cache but `Provider.GetByTrigger` keys miss it. | `pkg/script/provider.go` lookup keys; verify `script.TriggerAiQueue3` value matches cached trigger ID; verify category matching for `_` wildcard fallthrough across both specific and generic scripts |
| **E2a** | After fixture parity, `LookupPlayerByUID` *should* resolve. If still false: rat ledger `TopContributor` returns 0 mid-cascade (cleared by ai_queue2 / npc_default_damage?), or uid composition mismatch. | `pkg/script/handlers_npc.go` (NPC_FINDHERO) or `pkg/entity/npc.go` rat-ledger lifecycle |
| **E2b** | Warn err identifies the failing opcode. Most likely: `OBJ_ADD: no active player` (NPC_FINDHERO sets `Self2` instead of `Self`), `checkObjType` (configsView ObjType lookup miss), or `objType.Members && MapMembers()==0` short-circuit on raw_rat_meat. | `pkg/script/handlers_*.go` per err opcode |

Per earlier brainstorm answer: a fix that addresses a shared root cause (e.g. uid-lookup bug, NPC_FINDHERO bug) naturally generalizes from the `[ai_queue3,newbiegiantrat]` specific path to the `[ai_queue3,_]` / `[proc,npc_default_death]` generic path. T5 only asserts the specific path; the generic path is content-coverage scope (smoke).

### Phase D — Smoke gate

Per `smoke_test_server_handoff` memory: user launches the server. Claude does not.

Smoke procedure:
1. User runs `CGO_ENABLED=0 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go run -trimpath ./cmd/goscape --config.file config.yaml` from a non-sandboxed shell.
2. User connects with Client-Java #225 build, logs in.
3. User walks to a Lumbridge rat (e.g. (3094, 3106, 0) — same coord as the probe), attacks until death.
4. User observes whether bones / rat meat / death-drop obj appear on the ground.

Smoke success → Stage 2 closes. Smoke failure (loot still missing) → cascade attribution for Stage 2 stands; reroute to NAI-129+ per `cascade_theory_smoke_binding` decision tree.

## §5 Test strategy

| Test | Layer | Assertion | Lifetime |
|---|---|---|---|
| `T5 GroundObjs` (existing) | Zone state | `len(atRat) == 2` (death_drop + raw_rat_meat) | Permanent close gate |
| `T6 CascadeDispatchTrace` (new) | Dispatch | (a) `GetByTrigger(ai_queue3, ratID, category) != nil`, (b) zero Warn `"npc script execute error"` records | Permanent regression gate |

T6's two assertions catch dispatch-path regressions earlier than T5: a future provider routing bug or silent script error surfaces at T6 with a precise diagnostic, while T5's coord-filtered count goes red without identifying the layer.

## §6 Risks

- **R1 — Phase A may surface other zero-init fixture gaps.** E.g. an opcode in the cascade may read `s.invTypes`, `s.enumTypes`, or `s.structTypes`, which are not in the Phase A list. Mitigation: Phase B failure routes to Phase C, where T6's err-string output identifies the missing layer; remediate fixture inline (it is fixture-correctness, not a production fix).
- **R2 — Phase B GREEN may mask a separate production bug.** If fixture parity alone makes T5 pass but smoke still fails, the production bug is in code path the fixture exercises differently than production (e.g. tick-loop ordering, player input dispatch). Mitigation: Phase D smoke is mandatory; residuals route to NAI-129+.
- **R3 — slog.Handler interface signature drift.** Mitigation: implement minimal `Enabled/Handle/WithAttrs/WithGroup`; spot-check Go 1.26 `log/slog`.
- **R4 — T6 may pass while T5 fails.** Possible if obj_add succeeds but writes to a different zone or coord than T5's filter expects (`o.X == rat.x && o.Z == rat.z && o.Level == rat.level` at `nai128_rat_loot_test.go:247`). T5's coord filter is the safety net; if this happens it indicates a coord-encoding or zone-routing bug — additional in-scope binding.
- **R5 — Recording handler races.** If any code path called from `processNpcQueue` logs concurrently (e.g. via a goroutine), the recorder's mutex must guard. Mitigation: `sync.Mutex` already in sketch; cascade is single-goroutine in this probe.

## §7 Out of scope / deferred

- Generic `[ai_queue3,_]` / `[proc,npc_default_death]` path: in-scope only if shared root cause; T5 doesn't assert it; smoke is the content-side check.
- Other NPC types (`man`, `goblin`, etc.): if fix is at NPC_FINDHERO or provider-lookup level, generalizes automatically. No additional probes.
- `lootdrop_duration`-based despawn: NAI-115-D2 deviation; out of scope.
- Public-receiver `obj_add` semantics: not exercised by `[ai_queue3,newbiegiantrat]` (uses private receiver via `obj_add` not `obj_addall`).

## §8 Close criterion

1. Phase A fixture patch committed (always).
2. T5 + T6 GREEN at HEAD.
3. Phase D smoke confirmed by user (rat loot visible on ground in Java client).
4. `Closes memory:` trailer on close commit per `close_commit_memory_trailer` memory.
5. New memory entry capturing the **fixture-parity gap pattern** — generalizable lesson for future tests that exercise script dispatch via `processNpcQueue` / `runScript` / `runNpcScript`. Title sketch: *"Test fixtures must mirror NewServer view inits before exercising script dispatch"*.

## §9 Memory entries to apply

Active during this work:
- `cascade_theory_smoke_binding` — smoke binds residual attribution.
- `disasm_reframes_inferred_binding` — predecessor binding cites cascade flow; static disasm + reach probes confirmed Stage-1 cascade ran. No re-disasm needed at Stage 2 start.
- `controller_preflight` — already exercised; surfaced fixture-parity gap (§3).
- `verify_implementer_claims` — implementer must run T5 + T6 + check warn record count post-fix; do not trust "tests pass on my machine".
- `close_commit_memory_trailer` — required on Stage 2 close commit.
- `smoke_test_server_handoff` — user launches server for Phase D.
- `investigation_subspec_cadence` — pattern reused (Stage 1 audit → Stage 2 fix → smoke → conditional Stage 3).

New memory at Stage 2 close: fixture-parity gap pattern (§8 item 5).
