# NAI-137 — Run varp clientcode-7 dynamic discovery

**Status:** spec
**Date:** 2026-05-09
**Predecessors:** NAI-136 (runweight propagation) closed at `a96862b`. NAI-135 carryover queue (line 6423 of `nai_followups.md`) named this candidate verbatim: "run-toggle UI varp UI-binding."
**Tech stack:** Go 1.26+
**Cadence:** Compressed (combined spec+plan, single-dispatch implementer + final Sonnet code-reviewer per `compressed_cadence`). Bundle 0 short-circuit applied — static audit disambiguated root cause without Stage 1 instrumentation.

## 1. Goal

Port TS's `VarPlayerType.RUN` dynamic-discovery (`Engine-TS/src/cache/config/VarPlayerType.ts:50-53`) so that engine-side server→client run-toggle echoes write to the cache-resolved `clientcode==7` varp id (typically 173 = `option_run` per `Content/pack/varp.pack:174`) instead of the hardcoded `0`.

Closes the **NAI-135 SECONDARY residual**: at runenergy=0, `(*Player).updateEnergy` writes `p.run = 0` AND `SetVarp(VarPlayerRun, 0)` per `Player.ts:697-699`. Server-side state is correct (pinned by `TestUpdateEnergy_EnergyZeroResetsRunAndVarp`); the wire-send fires via `writeVarp → OpVarpSmall`. User smoke (2026-05-09 NAI-135 close) reported the player correctly auto-walks at energy=0, but the run-toggle UI button does not visually clear. Toggle works in the OTHER direction (UI→server P_RUN) — likely because the client locally optimistic-updates the toggle on click and the server's wrong-id echo is lost in the wash.

## 2. TS source — anchored

- **`VarPlayerType.RUN` placeholder default:** `Engine-TS/src/cache/config/VarPlayerType.ts:18` — `static RUN = 0`.
- **`VarPlayerType.RUN` dynamic discovery (the load-bearing block):** `Engine-TS/src/cache/config/VarPlayerType.ts:50-53`:
  ```ts
  if (config.clientcode === 7) {
      // unused in client so my best guess is that this was used to find the engine varp
      VarPlayerType.RUN = config.id;
  }
  ```
- **Energy=0 consumer:** `Engine-TS/src/engine/entity/Player.ts:699` — `this.setVar(VarPlayerType.RUN, this.run);`.
- **P_RUN handler consumer:** `Engine-TS/src/engine/script/handlers/PlayerOps.ts:1208` — `state.activePlayer.setVar(VarPlayerType.RUN, state.activePlayer.run);`.
- **Content config — `option_run`:** `Content/scripts/interface_controls/configs/player_controls.varp:5-8`:
  ```
  [option_run]
  protect=no
  clientcode=7
  transmit=yes
  ```
- **Content packed id:** `Content/pack/varp.pack:174` — `173=option_run`.

## 3. Static disambiguation evidence (Bundle 0)

**TS:** dynamic discovery — `RUN` is a runtime-resolved id, the static `0` is a placeholder overwritten at parse-time.

**goscape (HEAD `a96862b`):**
- `pkg/script/active.go:7` — `const VarPlayerRun = 0` (compile-time constant, no resolution).
- `pkg/objtype/varptype.go:30-33` — `case 5: v.ClientCode = uint16(dat.G2())` parses the field but no scan for `ClientCode==7`.
- `pkg/objtype/varptype.go:66-87` — `parseVarpTypes` populates `VarpTypeConfigs.{ConfigNames, Configs}` only.

**Content:** `option_run` (clientcode=7) is at id 173, not 0.

**Symptom-fit:** `(*Player).updateEnergy` and `handlePRun` both write `OpVarpSmall` for varp id 0; client's run-toggle UI binds to varp 173. Server→UI (energy=0) fails because nothing pushes to id 173. UI→Server appears to work because the client locally updates the toggle on click without waiting for server echo.

The misleading doc-comment at `pkg/script/active.go:4` ("Mirrors TS VarPlayerType.RUN = 0") read TS's L18 placeholder as the canonical value; it's the L50-53 dynamic overwrite that actually carries the load.

## 4. Non-goals

- **No login-time push of the run varp.** TS `Player.ts:418-425` initializes the local var array but does not push per-id varps on login. Goscape matches.
- **No new opcode handler.** Both consumer call sites already exist; this sub-spec only changes the id they pass.
- **No varp-id resolution for non-run engine varps.** Other clientcode values (1, 2, 3, 4, 5, 6, 8 all visible in `Content/scripts/interface_options/configs/game_options.varp`) have separate engine semantics; sweeping them is out of scope. If a future smoke binds another, route to NAI-138+.
- **No retire of dispatch_correct_reach_blocked SECONDARY pieces from earlier NAI sub-specs** (firemaking ashes, LOWMEM trace, P_TELEJUMP, weapon-equip rendering, combat-init cascade) — all remain queued.
- **No retire of NAI-115-D1/D2 deviations.**
- **No content-side mirror.** The carryover note mentioned "whether content-side script needs to mirror to a different UI varp/component" — root cause is engine, not content. No content edits.
- **Smoke harness work is user-driven** per `smoke_test_server_handoff`.

## 5. Architecture — full call-graph

```
config-load:
  parseVarpTypes  (pkg/objtype/varptype.go)
    runID := 0                                              ← TS-faithful default
    for each config in varp.dat:
      Decode → ClientCode (case 5)
      if config.ClientCode == 7: runID = config.id          ← NEW (mirrors VarPlayerType.ts:50-53)
    return VarpTypeConfigs{Configs, ConfigNames, RunID}     ← RunID field new

runtime — server→client echo (energy=0):
  (*Server).processEnergy → (*Player).updateEnergy
    if runenergy == 0:
      p.run = 0
      p.SetVarp(p.RunVarpID(), 0)                           ← was: script.VarPlayerRun

runtime — server→client echo (P_RUN script handler):
  handlePRun(s)
    requireProtectedActivePlayer
    s.Self.SetRun(v)
    s.Self.SetVarp(s.Self.RunVarpID(), int32(v))            ← was: VarPlayerRun

ActivePlayer interface:
  RunVarpID() int                                           ← NEW method

(*Player).RunVarpID():
  return p.client.server.varpTypes.RunID                    ← NEW shim
```

`const VarPlayerRun = 0` retired. The misleading "Mirrors TS VarPlayerType.RUN = 0" comment replaced with a doc-comment on `RunVarpID()` referencing `parseVarpTypes` clientcode-7 discovery.

## 6. Implementation — code blocks

### 6.1 `pkg/objtype/varptype.go`

```go
type VarpTypeConfigs struct {
    ConfigNames map[string]int
    Configs     []*VarPlayerType
    RunID       int // varp id of the engine run-mode varp (config with ClientCode==7); defaults to 0 when no such config exists, mirroring TS VarPlayerType.RUN placeholder default at Engine-TS/src/cache/config/VarPlayerType.ts:18.
}
```

In `parseVarpTypes` loop, after `configs[id] = config`:

```go
if config.ClientCode == 7 {
    runID = id
}
```

with `runID := 0` declared above the loop, and the return value updated to include `RunID: runID`.

### 6.2 `pkg/script/active.go`

Retire `const VarPlayerRun = 0`. Add to `ActivePlayer` interface:

```go
// RunVarpID returns the varp id discovered at config-load time as the
// engine-level run-mode varp (the config with ClientCode==7). Mirrors TS
// VarPlayerType.RUN dynamic discovery at Engine-TS/src/cache/config/VarPlayerType.ts:50-53.
// Returns 0 as a TS-faithful placeholder default when no clientcode-7
// config exists in the loaded cache.
RunVarpID() int
```

### 6.3 `pkg/script/handlers_player.go`

Replace `s.Self.SetVarp(VarPlayerRun, int32(v))` (line 641) with:

```go
s.Self.SetVarp(s.Self.RunVarpID(), int32(v))
```

Update the function-level doc-comment at line 626 to reference `RunVarpID()` rather than the retired constant.

### 6.4 `pkg/script/handlers_player_test.go` + `pkg/script/runner_test.go`

`mockPlayer` (runner_test.go) gains:

```go
runVarpID int // seeded by tests; mockPlayer.RunVarpID returns this.
```

with method:

```go
func (m *mockPlayer) RunVarpID() int { return m.runVarpID }
```

`TestPRunDispatch` (handlers_player_test.go:772-799) seeds `mp := &mockPlayer{lastSetRun: -1, runVarpID: 173, varps: map[int]int32{}}` and asserts `mp.varps[173] == int32(v)`.

`TestPRunRequiresProtect` (handlers_player_test.go:1229-1235) — no change needed; the protected-gate test does not reach the SetVarp branch.

### 6.5 `modules/world/player_script.go`

Add:

```go
// RunVarpID implements script.ActivePlayer.RunVarpID. Returns the
// varp id discovered at config-load time as the engine run-mode varp
// (the config with ClientCode==7). Mirrors TS VarPlayerType.RUN at
// Engine-TS/src/cache/config/VarPlayerType.ts:50-53. Returns 0 if the
// server has no varpTypes loaded (test-fixture / pre-config-load).
func (p *Player) RunVarpID() int {
    if p.client == nil || p.client.server == nil || p.client.server.varpTypes == nil {
        return 0
    }
    return p.client.server.varpTypes.RunID
}
```

### 6.6 `modules/world/player_run.go`

Replace `p.SetVarp(script.VarPlayerRun, 0)` (line 45) with:

```go
p.SetVarp(p.RunVarpID(), 0)
```

Update the function-level doc-comment at line 22-24 to reference `RunVarpID()` and the dynamic-discovery semantics. Drop the `script` import if no longer used.

### 6.7 `modules/world/player_run_test.go`

`TestUpdateEnergy_EnergyZeroResetsRunAndVarp` (line 253) updated:

- Build a `varpTypes` config (via test helper or inline `*objtype.VarpTypeConfigs` with `RunID: 173` and `Configs` sized to 174 entries with the entry at 173 having `Transmit: true, Type: ScriptVarTypeInt`).
- Wire it onto the test server (`p.client.server.varpTypes = vt`).
- Size `p.varps = make([]int32, 174)`; pre-state `p.varps[173] = 1`.
- Assert `p.varps[173] == 0` after `updateEnergy()`.
- Assert `p.varps[0] == 0` (sanity: no write hit the wrong id).

## 7. Tracked deviations

None new.

## 8. Risks / pre-flight verified at spec-write

- ✅ `parseVarpTypes` is the only varp loader (`grep -rn "parseVarpTypes\|VarpTypeConfigs{" --include='*.go'`).
- ✅ `writeVarp` has exactly one caller — `(*Player).SetVarp` at `modules/world/player_script.go:322` — so all transmit gating funnels through there.
- ✅ Two production call sites of `VarPlayerRun`: `pkg/script/handlers_player.go:641` (P_RUN) and `modules/world/player_run.go:45` (updateEnergy). No other engine path writes the run varp at runtime; TS doesn't push it on login either.
- ✅ `option_run` in `Content/scripts/interface_controls/configs/player_controls.varp:5-8` carries `clientcode=7` and `transmit=yes`. Packed id 173 per `Content/pack/varp.pack:174`.
- ✅ `cfg.Transmit` gate in `writeVarp` will pass for varp 173 (transmit=yes).
- ✅ `Server.varpTypes` is reachable from `Player` via `p.client.server.varpTypes` — same access path used by the existing `varpTypeConfig` helper at `modules/world/player_varp.go:30-38`.
- ⚠️  `(*Player).RunVarpID()` returns 0 when `p.client == nil || p.client.server == nil || p.client.server.varpTypes == nil` — needed for test fixtures that don't seat a server. This matches the existing `varpTypeConfig` defensive pattern. Doc-comment labels it goscape-only-defensive (TS skips because the fixture is `static`-resolved). Per `defensive_gate_doc_comment_label`.

## 9. Test plan

- **`TestParseVarpTypes_DiscoversRunIDFromClientCode7`** (NEW, pkg/objtype) — pack three minimal varp configs, the middle one with `ClientCode == 7`; assert `vt.RunID == middle.id`.
- **`TestParseVarpTypes_RunIDDefaultsZeroWhenNoClientCode7`** (NEW, pkg/objtype) — pin TS-faithful default-0 fallback.
- **`TestUpdateEnergy_EnergyZeroResetsRunAndVarp`** (UPDATED, modules/world) — fixture seeds `varpTypes.RunID = 173`; asserts `p.varps[173] == 0` and `p.varps[0] == 0`.
- **`TestPRunDispatch`** (UPDATED, pkg/script) — mockPlayer exposes `runVarpID = 173`; asserts `mp.varps[173] == int32(v)`.
- **`TestUpdateEnergy_EnergyZeroNoEmitWhenRunIDZero`** (NEW, modules/world) — sanity: when fixture has `RunID: 0`, the energy=0 path still writes to `p.varps[0] == 0` (matches TS placeholder behavior).

All other existing tests that touch `p.varps` or `mp.varps` keep their existing fixture shape (single entry, id 0) — those tests don't exercise run-varp dispatch.

## 10. Order of operations (compressed cadence — single implementer dispatch)

1. **T1 — `pkg/objtype` foundation:** add `RunID` field; populate on `ClientCode==7`; add 2 new pkg/objtype tests (RED→GREEN). One commit.
2. **T2 — `pkg/script` interface + handler:** retire `const VarPlayerRun`; add `RunVarpID()` to `ActivePlayer`; mockPlayer shim in `runner_test.go`; update `handlePRun` + `TestPRunDispatch`. One commit.
3. **T3 — `modules/world` shim + consumer:** add `(*Player).RunVarpID()`; update `player_run.go:45`; update `TestUpdateEnergy_EnergyZeroResetsRunAndVarp`; add `TestUpdateEnergy_EnergyZeroNoEmitWhenRunIDZero`. One commit.
4. **T4 — final code-review:** Sonnet code-reviewer subagent (per `superpowers_code_reviewer_model`) over the 3 commits before user-launched smoke handoff.

**Smoke (user-driven):**
- **PRIMARY:** log in, run-step until run-energy depletes to 0, observe run-toggle button visually de-toggles in the same tick the player reverts to walking.
- **SECONDARY:** click the run toggle while running, confirm server-side `p.run` flips AND varp echo reaches the correct id (toggle remains state-consistent across client redraws driven by server-side varp pushes; verified by re-engaging run after the click without a second click).

## 11. Pattern memories applied

- `bundle0_short_circuits_stage1_audit` — static evidence (TS dynamic discovery + Content clientcode-7 at id 173 + goscape hardcoded 0) disambiguates without Stage 1 instrumentation.
- `compressed_cadence` — combined spec+plan, single implementer dispatch.
- `superpowers_code_reviewer_model` — Sonnet, never Opus.
- `true_to_ts_gate` — dynamic discovery is a structural mirror of TS; no deviation tracked.
- `helper_as_oracle_test_anti_pattern` — non-zero clientcode-7 id (173) in fixtures pins both discovery AND dispatch independently; using id 0 would let the bug pass silently.
- `audit_subagent_fabrication` — root-cause was verified via direct file reads (TS source + Content varp + goscape parse path), not delegated audit.
- `defensive_gate_doc_comment_label` — `RunVarpID()` nil-server fallback labeled as goscape-only-defensive.
- `enumerate_all_sites` — both production call sites and both bug-pinning tests enumerated and updated.
- `consume_reserved_constant` — retiring `const VarPlayerRun` audited downstream; no other consumers found.
- `smoke_test_server_handoff` — user-launched smoke after T4 reviewer subagent.

## 12. Cross-references

- **NAI-135** (run-mode visible-effect wiring) — predecessor; introduced `(*Player).updateEnergy` energy=0 reset path that exposed the bug.
- **NAI-117** (run-mode handler pair) — introduced the `const VarPlayerRun = 0` and the P_RUN handler being patched.
- **NAI-136** (runweight propagation) — immediate predecessor; closed cleanly with no carryovers; ran the same compressed cadence shape.
- **`memory/nai_followups.md` line 6423** — the verbatim NAI-135 carryover entry being closed.
