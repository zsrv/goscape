# NAI-117 — Run-mode handler pair (P_RUN + RUNENERGY)

**Status:** Spec authored 2026-05-06 (post-NAI-116 close).
**Cadence:** Standard (separate spec → plan), single bundle, two tasks.
**Routing:** Adjacent residuals from NAI-116 close-commit smoke (`6658a0d`); routed per `smoke_surfaces_adjacent_divergences` because each handler-port exceeds the 30-LOC in-scope-stretch threshold once tests are included.

## 1. Problem

Two script opcodes have declared identifiers in `pkg/script/opcode.go` but no
dispatch entries in `pkg/script/handlers.go`. Tutorial Island content fails
with `script: no handler at <site> pc=<n>` errors:

- **P_RUN (2085)** — surfaces in `[proc,tutorial_step_enable_run]` pc=6 when
  the tutorial prompts the run-mode toggle.
- **RUNENERGY (2096)** — surfaces in `[if_button,controls:com_5]` pc=7 when
  the player clicks the controls tab.

Both are direct line-by-line ports of TS handlers in
`Engine-TS/src/engine/script/handlers/PlayerOps.ts`. Underlying state
(`p.run`, `p.runenergy`, `SetVarp`) already exists in goscape; only the
script-side handler surface and a small interface addition are missing.

## 2. TS source (canonical reference)

Per `ts_source_canonical_path` memory: `LostCityRS/Engine-TS` is the
canonical TS reference.

### P_RUN — `PlayerOps.ts:1204-1209`

```ts
[ScriptOpcode.P_RUN]: checkedHandler(ProtectedActivePlayer, state => {
    state.activePlayer.run = state.popInt();

    // todo: better way to sync engine varp
    state.activePlayer.setVar(VarPlayerType.RUN, state.activePlayer.run);
}),
```

### RUNENERGY — `PlayerOps.ts:1175-1178`

```ts
[ScriptOpcode.RUNENERGY]: checkedHandler(ActivePlayer, state => {
    const player = state.activePlayer;
    state.pushInt(player.runenergy);
}),
```

### `VarPlayerType.RUN` constant — `cache/config/VarPlayerType.ts:18`

```ts
static RUN = 0;
```

Hard-coded id 0; goscape ports as a named constant `VarPlayerRun = 0` (see
§4 below) to keep the magic-number reference grep-discoverable.

## 3. Goscape state (HEAD verification)

Per `controller_preflight` memory; verified against HEAD `6658a0d`.

| Surface | Status | Location |
|---|---|---|
| `OpPRun = 2085`, `OpRunEnergy = 2096` (Opcode constants) | ✅ exists | `pkg/script/opcode.go:185, 196` |
| `Opcode.String()` cases for `OpPRun` / `OpRunEnergy` | ✅ exists | `pkg/script/opcode.go:777, 799` |
| `p.run`, `p.tempRun int` on `Player` | ✅ exists | `modules/world/player.go:190` |
| `p.runenergy int` on `Player` | ✅ exists | `modules/world/player.go:191` |
| `p.SetVarp(id int, val int32)` | ✅ exists | `modules/world/player_script.go:317` |
| `Player.run` field is read by `defaultMoveSpeed` equivalent (movement.go) | ✅ wired | `modules/world/movement.go:67` reads `p.moveSpeed`; the `p.run` field is the field a future `defaultMoveSpeed` port will gate on. P_RUN write is sufficient for the current sub-spec. |
| Dispatch entry for `OpPRun` / `OpRunEnergy` in handler map | ❌ **missing** | `pkg/script/handlers.go` |
| `Self.SetRun` / `Self.RunEnergy` on `ActivePlayer` interface | ❌ **missing** | `pkg/script/active.go` |
| `VarPlayerRun` named constant | ❌ **missing** | (this spec adds it) |

## 4. Design (approved approach A)

**Approach A: separate write + varp mirror (TS-literal).** Rejected
approach B (combined `SetRun` that internally calls `SetVarp`) because it
hides the multi-effect TS sequence inside a helper, violating
`ts_helper_method_bundles` memory (TS helper-method bundles must port as
explicit multi-line sequences when the TS author kept the sequence
explicit at the call site — and TS itself flags the duplication with
`// todo: better way to sync engine varp`, which is itself a marker that
collapsing is intentional-future-work, not desirable today).

### 4.1 New named constant

```go
// pkg/script/active.go (or a new pkg/script/varp_ids.go — implementer's
// call; if other VarPlayerType ID constants are added in the same place
// later, group them).

// VarPlayerRun is the varp id for the run-mode toggle (`run` varp).
// Mirrors TS VarPlayerType.RUN = 0 (see Engine-TS
// cache/config/VarPlayerType.ts:18).
const VarPlayerRun = 0
```

### 4.2 New `ActivePlayer` interface methods

```go
// pkg/script/active.go — appended to the existing ActivePlayer interface.

// SetRun writes the player's run-mode toggle (0=walk, 1=run). Mirrors
// the field write `state.activePlayer.run = state.popInt()` from TS
// PlayerOps.ts:1205. The varp-mirror side-effect remains explicit at
// the handler call site (see handlePRun) per TS PlayerOps.ts:1208.
SetRun(value int)

// RunEnergy returns the player's current run-energy value as an int,
// scaled identically to TS Player.runenergy (range [0, 10000]).
// Used by the RUNENERGY opcode (PlayerOps.ts:1177).
RunEnergy() int
```

### 4.3 New handlers — `pkg/script/handlers_player.go`

```go
// handlePRun implements P_RUN (opcode 2085). Pops the run-mode int (0
// or 1), writes it to the player's run field, and mirrors the value to
// VarPlayerRun. Mirrors TS PlayerOps.ts:1204-1209 line-for-line.
//
//   // todo: better way to sync engine varp
//   state.activePlayer.setVar(VarPlayerType.RUN, state.activePlayer.run);
//
// The two-step (field write + varp mirror) is preserved per
// ts_helper_method_bundles memory.
func handlePRun(s *ScriptState) error {
    if err := requireProtectedActivePlayer(s, "P_RUN"); err != nil {
        return err
    }
    v := s.PopInt()
    s.Self.SetRun(v)
    // todo: better way to sync engine varp (mirrored from TS PlayerOps.ts:1207)
    s.Self.SetVarp(VarPlayerRun, int32(v))
    return nil
}

// handleRunEnergy implements RUNENERGY (opcode 2096). Pushes the active
// player's current runenergy (range [0, 10000]). Mirrors TS
// PlayerOps.ts:1175-1178.
func handleRunEnergy(s *ScriptState) error {
    if err := requireActivePlayer(s, "RUNENERGY"); err != nil {
        return err
    }
    s.PushInt(s.Self.RunEnergy())
    return nil
}
```

### 4.4 Dispatch wiring — `pkg/script/handlers.go`

Add to the `handlers` map (alongside `OpPTeleport`, `OpPTeleJump`, etc.):

```go
OpPRun:       handlePRun,
OpRunEnergy:  handleRunEnergy,
```

Place near the other player ops; ordering is alphabetic-by-opcode-name in
some sections of the map and by category in others — implementer should
place near the existing P_TELEPORT / P_TELEJUMP entries and near
RUNANIM, respectively.

### 4.5 World-side surface — `modules/world/player_script.go`

```go
// SetRun writes the run-mode toggle (0=walk, 1=run) to the player.
// Backs ActivePlayer.SetRun used by the P_RUN opcode handler.
func (p *Player) SetRun(v int) {
    p.run = v
}

// RunEnergy returns the player's current run-energy as an int.
// Backs ActivePlayer.RunEnergy used by the RUNENERGY opcode handler.
func (p *Player) RunEnergy() int {
    return p.runenergy
}
```

## 5. Test strategy

### 5.1 P_RUN handler dispatch test (NEW)

```go
func TestPRunDispatch(t *testing.T) {
    for _, v := range []int{0, 1} {
        t.Run(fmt.Sprintf("v=%d", v), func(t *testing.T) {
            mp := &mockPlayer{lastSetRun: -2}  // sentinel != 0 and != 1
            sf := &ScriptFile{
                Name: "p_run_dispatch",
                Opcodes: []Opcode{
                    OpPushConstantInt, OpPRun, OpReturn,
                },
                IntOperands:      []int32{int32(v), 0, 0},
                StringOperands:   []string{"", "", ""},
                InstructionCount: 3,
            }
            state := Init(sf, mp, true, nil, nil)  // protect=true
            if err := Execute(state); err != nil {
                t.Fatalf("Execute: %v", err)
            }
            if mp.lastSetRun != v {
                t.Errorf("SetRun: got %d, want %d", mp.lastSetRun, v)
            }
            if got := mp.varps[VarPlayerRun]; int(got) != v {
                t.Errorf("varp[VarPlayerRun]: got %d, want %d", got, v)
            }
        })
    }
}
```

### 5.2 RUNENERGY handler dispatch test (NEW)

```go
func TestRunEnergyDispatch(t *testing.T) {
    mp := &mockPlayer{runenergyValue: 7250}
    sf := &ScriptFile{
        Name: "runenergy_dispatch",
        Opcodes: []Opcode{
            OpRunEnergy, OpReturn,
        },
        IntOperands:      []int32{0, 0},
        StringOperands:   []string{"", ""},
        InstructionCount: 2,
    }
    state := Init(sf, mp, false, nil, nil)
    if err := Execute(state); err != nil {
        t.Fatalf("Execute: %v", err)
    }
    if got := state.PopInt(); got != 7250 {
        t.Errorf("RUNENERGY: got %d, want 7250", got)
    }
}
```

### 5.3 Gate (protection) tests (NEW)

P_RUN is `ProtectedActivePlayer`; RUNENERGY is `ActivePlayer`.

```go
// P_RUN — script not protected
func TestPRunRequiresProtected(t *testing.T) {
    sf := newSingleOp("p_run_unprotected", OpPRun)
    state := Init(sf, &mockPlayer{}, false, nil, nil)  // protect=false
    err := Execute(state)
    if err == nil || err.Error() != "P_RUN: script not protected" {
        t.Errorf("expected 'P_RUN: script not protected', got %v", err)
    }
}
```

(No `TestPRunRequiresActive` / `TestRunEnergyRequiresActive` are needed
as standalone cases — both opcodes get added to the existing
`TestHandlersRequireActivePlayer` table at `handlers_player_test.go:769`,
which exercises every handler against `Self == nil` and asserts a
non-nil error.)

### 5.4 Table-coverage extensions (NEW)

Add `{"P_RUN", OpPRun}` and `{"RUNENERGY", OpRunEnergy}` to the
`TestHandlersRequireActivePlayer` table at `handlers_player_test.go:769-806`
— the file's single shared "every handler must error on `Self == nil`"
enumeration. (Verified at HEAD `6658a0d`: no second parallel
all-handlers-dispatch enumeration exists in the test file.)

### 5.5 mockPlayer extensions — `pkg/script/runner_test.go`

```go
// In the mockPlayer struct:
lastSetRun     int  // -1 default; track most recent SetRun(value)
runenergyValue int  // configurable return for RunEnergy()

// Methods (place near SetRunAnim at line 440):
func (m *mockPlayer) SetRun(v int)    { m.lastSetRun = v }
func (m *mockPlayer) RunEnergy() int  { return m.runenergyValue }
```

`lastSetRun` initialises to `-1` (or another sentinel != 0 and != 1) so a
write of value 0 is observably distinct from "never called."

### 5.6 Smoke binding (post-merge, user-launched)

Per `smoke_test_server_handoff` memory:

- **Tutorial Island Master Chef → exit room** (existing NAI-116 binding) →
  tutorial step that prompts the run-mode toggle. Confirm absence of
  `no handler at [proc,tutorial_step_enable_run] pc=6` in server log.
- **Click controls tab** → confirm absence of
  `no handler at [if_button,controls:com_5] pc=7` in server log.
- Bonus: any in-game observation that toggling run mode changes movement
  speed is welcome but not required for binding (movement-loop wiring is
  already in place via `p.moveSpeed` per `movement.go:67`).

## 6. Risk register

Per `risk_register_premise_grep` memory; each premise has been
HEAD-grepped.

| ID | Premise | Verification |
|---|---|---|
| R1 | `OpPRun` / `OpRunEnergy` are declared but unhandled | ✅ `grep -n "OpPRun\\|OpRunEnergy" pkg/script/*.go` finds only the const decl + `String()` case; no entry in `handlers` map. |
| R2 | `p.run` and `p.runenergy` exist on `Player` | ✅ `player.go:190-191`. |
| R3 | `SetVarp` exists on `Player` | ✅ `player_script.go:317`. |
| R4 | `requireProtectedActivePlayer` and `requireActivePlayer` exist with the expected error-string shapes | ✅ `handlers_player.go:35, 58`; format `"<OP>: no active player"` / `"<OP>: script not protected"`. |
| R5 | TS `runenergy` is integer-shaped at the script-stack boundary | ✅ TS `state.pushInt(player.runenergy)` (`PlayerOps.ts:1177`); goscape decay is `int -= int` (`movement.go:127-130`); no fractional truncation issue. |
| R6 | Handler ordering / dispatch-map placement is non-load-bearing | ✅ `pkg/script/handlers.go` is a flat map; placement is for human readability only. |
| R7 | `VarPlayerType.RUN` is hard-coded id 0 in TS, not a runtime varp lookup | ✅ `Engine-TS/src/cache/config/VarPlayerType.ts:18` `static RUN = 0;`. Reading by name from `VarpTypeConfigs` would be a deviation; rejected. |

## 7. Deviations from TS

**None.** Both handlers port line-for-line. The `// todo: better way to
sync engine varp` comment from `PlayerOps.ts:1207` is ported verbatim
into the goscape `handlePRun` body.

## 8. Out of scope (NAI-117)

The following are explicitly *not* part of this sub-spec and route to
later NAI numbers per the NAI-116 close-commit carry-forward queue:

- **Firemaking ashes-no-drop after fire despawn** (NAI-118 candidate) —
  investigation sub-spec, applies `investigation_subspec_cadence`.
- **LOWMEM byte-alignment trace** (NAI-119 candidate) — investigation
  sub-spec; symptom is server pushes `1` when client high-mem.
- **NAI-111 P_TELEJUMP `[label,tutorial_complete]`** — still queued.
- **TS `defaultMoveSpeed` port / `tempRun` reset on energy<100** — TS
  `Player.ts:701-703` resets `tempRun = 0` on low energy; goscape
  movement.go applies the comparable check inline at `handlers_game.go:294`.
  No additional port required for this sub-spec to bind.

## 9. Cadence pattern memories applied

- `smoke_surfaces_adjacent_divergences` — both residuals routed from
  NAI-116 close, each >30 LOC including tests, so they go to a fresh
  sub-spec rather than in-scope-stretch.
- `controller_preflight` — file-paths and line-numbers in §3 verified
  against HEAD `6658a0d` before plan dispatch.
- `ts_helper_method_bundles` — rejected combined `SetRun` (which would
  internally call `SetVarp`) in favor of explicit two-step at handler
  call site.
- `ts_source_canonical_path` — TS source path is `LostCityRS/Engine-TS`.
- `true_to_ts_gate` — zero deviations; both handlers port line-for-line.
- `disasm_reframes_inferred_binding` — not applicable (binding is from
  smoke log error messages, not inferred bytecode pcs; opcode IDs are
  authoritative).
- `bundle0_short_circuits_stage1_audit` — not applicable (no Bundle 0
  TS-source diff; this is a forward port of two missing handlers, not
  a fix-an-existing-divergence sub-spec).

## 10. Definition of done

- T1 (P_RUN): handler + interface method + dispatch entry + tests; RED →
  GREEN; `go test ./...` green.
- T2 (RUNENERGY): handler + interface method + dispatch entry + tests;
  RED → GREEN; `go test ./...` green.
- Bundle close: both opcodes silenced in user-launched smoke (§5.6).
- Final-close commit with `Closes memory:` trailer per
  `close_commit_memory_trailer` memory.
