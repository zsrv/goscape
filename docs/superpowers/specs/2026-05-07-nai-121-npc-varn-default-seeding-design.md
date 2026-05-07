# NAI-121 — NPC varn / Player varp per-type default-seeding + STRING parallel arrays + opcode dispatch + V-PARTIAL investigation

**Date:** 2026-05-07
**Status:** spec — investigation sub-spec (Bundle 1 fix → Bundle 2 V-PARTIAL Stage 1 audit → Bundle 3 V-PARTIAL Stage 2 fix sized from findings → smoke) per `investigation_subspec_cadence`.
**Predecessor:** NAI-120 close (commit `527259a`, smoke 2026-05-07). NAI-120 SECONDARY routed forward; smoke produced "It's not after you." gate-fire on Tutorial Island giant-rat attack. Adjacent V-PARTIAL on `%npc_combat_xp_multiplier` reading 0 also pulled in (NAI-120 followups parked item).
**Cadence:** Bundle 1 (single subagent-driven-development cycle, smoke-binding fix) → Bundle 2 (Sonnet Explore investigation subagent, audit-only, no production code) → Bundle 3 (sized from Bundle 2 findings — possibly subagent-driven-development, possibly direct edit if ≤30 LOC) → user-launched smoke handoff → conditional close.
**Tech stack:** Go 1.26+.
**Upstream sources:** `LostCityRS/Engine-TS` (TS engine, per `ts_source_canonical_path`); `LostCityRS/Content/scripts/**/*.varn` (4 declared `.varn` config files, 5 vars total).

---

## 1. Symptom

Surfaced 2026-05-07 at NAI-120 smoke. After NAI-120 closure (combat-init inner-ring opcode handlers ported; dispatcher reaches `[proc,player_in_combat_check]` without missing-handler errors), the Java client's first attack on a Tutorial Island giant rat produces:

```
"It's not after you."
return(false);
```

Origin: `LostCityRS/Content/scripts/skill_combat/scripts/player/player_combat.rs2:107-110`

```
if (%npc_macro_event_target ! null & %npc_macro_event_target ! uid) {
    mes("It's not after you."); return(false);
}
```

**Engine-side root cause** (per NAI-120 smoke investigation, recorded in `npc_varn_default_seed_per_type.md`):

`%npc_macro_event_target` is declared in `Content/scripts/macro events/configs/antimacro.varn:1-2` with `type=player_uid`. Per TS `Npc.ts:296-303` (the `resetEntity(true)` per-type seed loop), `player_uid`-typed varn slots seed to `-1` (TS sentinel for `null` in scripts). Goscape's `(*Npc).NpcVarN` at `modules/world/npc_script.go:81-86` returns **`0`** for unset slots regardless of varn type — because goscape has no varn-type registry and `varns []int32` is type-blind. So the gate's `%n != null` (`0 != -1`) returns true and the deny-message fires on every fresh-spawn NPC.

**Adjacent V-PARTIAL** (`nai_followups.md` NAI-120 parked):

`%npc_combat_xp_multiplier` (declared in `Content/scripts/npc/configs/ai_spawn.varn`, INT default) reads as `0` after the Tutorial Island giant-rat attack lands. The `[ai_spawn,_]` global trigger at `Content/scripts/npc/scripts/ai_spawn.rs2:1-3` is supposed to populate it from `npc_param(combat_xp_multiplier)`. Effect: combat XP multiplies to 0; no XP awarded on hit. Goscape's AI_SPAWN dispatch IS already wired (`modules/world/npc_registry.go:82-99` enqueues; `processNpcEventQueue` dispatches). One of: (a) `[ai_spawn,_]` script not in goscape's compiled `script.dat`; (b) dispatch wired but breaks downstream; (c) `npc_param` opcode missing/broken; (d) timing/ordering mismatch with first-tick player interaction. Investigation-cadence sized from findings.

---

## 2. Goal

Bind the smoke (combat-init proceeds to first hit on Tutorial Island giant rat) AND port full TS-fidelity per-type default-seeding for both NPC varns and Player varps including STRING parallel arrays, registry, and opcode dispatch with Protect gate. Investigate and (if findings warrant) fix the V-PARTIAL combat-XP path.

---

## 3. Scope

### 3.1 In scope

**Bundle 1 (smoke-binding fix):**
- New `pkg/objtype/varntype.go` mirroring `varptype.go` / `varstype.go`. `VarNpcType` struct + `LoadVarnTypes(dir)` + `parseVarnTypes(*Packet)`.
- `modules/world/server.go` registry load wire-up; `s.varnTypes *objtype.VarnTypeConfigs` field.
- `modules/world/npc.go`: replace `varns []int32` (lazy-grown, capped at 1024) with registry-sized `varns []int32` + parallel `varnsString []string`.
- `modules/world/npc_registry.go::resetEntityForRespawn`: append per-type seed loop (TS `Npc.ts:296-306`). This path covers BOTH initial spawn and respawn — `Server.addNpc` calls `resetEntityForRespawn` at line 79, mirroring TS `World.addNpc` calling `npc.resetEntity(true)` at `World.ts:1281`. No separate `NewNpc`-side seed needed; raw `&Npc{}` test literals keep nil slices and rely on read-path defensive guards.
- `modules/world/player.go`: add `varpsString []string` field.
- `modules/world/tick.go:105`: per-type seed loop on player `varps` allocation (TS `Player.ts:418-432`).
- `pkg/script/active.go`: extend `ActivePlayer` with `VarpString` / `SetVarpString`; extend `ActiveNpc` with `NpcVarNString` / `SetNpcVarNString`.
- `modules/world/npc_script.go` and `modules/world/player_script.go`: implement the four new accessors.
- `pkg/script/handlers_vars.go`: type-aware dispatch on `PUSH_VARP` / `POP_VARP` / `PUSH_VARN` / `POP_VARN`. Add `Protect` gate to `POP_VARP` (TS `CoreOps.ts:50-52`).
- `pkg/script/active.go`: extend ScriptState's `World` view with `VarpType(id) (ScriptVarType, protect bool)` + `VarnType(id) ScriptVarType`. Goscape `(*Server)` implements via `s.varpTypes` / `s.varnTypes`.
- Retire `npcVarnCap = 1024` constant + its check site (slice now sized to registry).
- Test surface enumerated in §10.

**Bundle 2 (V-PARTIAL Stage 1 investigation):**
- Read-only audit subagent (Sonnet, Explore-style). No production code.
- Output: `docs/superpowers/investigations/2026-05-07-nai-121-vpartial-findings.md`.
- Trace `%npc_combat_xp_multiplier` end-to-end: script-pack inclusion → `Provider.GetByTrigger(TriggerAiSpawn, _, _)` → `processNpcEventQueue` dispatch → `npc_param` opcode handler → varn write.

**Bundle 3 (V-PARTIAL Stage 2 fix):**
- Sized from Bundle 2 findings. Likely shapes:
  - ≤30 LOC (one-line wiring fix or single missing handler stub) → direct edit.
  - 50-150 LOC (single missing opcode port) → subagent-driven-development.
  - 200+ LOC (content-pack pipeline issue or deeper missing infrastructure) → carry forward to NAI-122 with documented findings; NAI-121 closes on PRIMARY only.

### 3.2 Out of scope

- Any varn / varp reader or writer that goes through a code path other than `PUSH_VARN` / `POP_VARN` / `PUSH_VARP` / `POP_VARP`. (E.g., direct varn save/restore in player save format — Player.ts:213-228 — is a separate save-format concern, not blocking smoke.)
- VarBit handling (declared but not dispatched per NAI-120 followups). Defer to first downstream consumer.
- VarP transmit-on-write logic changes. The existing `(*Player).writeVarp(id, val)` wire-send path stays unchanged; the new `SetVarpString` does NOT add a wire-send (TS `Player.setVar` only writes server-side for STRING — no varp_string wire opcode in this protocol revision).
- Refactoring how scripts are loaded / compiled into `script.dat` (Bundle 2 may surface this; if so, route to NAI-122).
- Player save/load round-trip of varpsString. Initial impl writes "" on init; persistence is a separate concern.

### 3.3 Anti-scope

- No new opcode beyond the four (`PUSH_VARP`, `POP_VARP`, `PUSH_VARN`, `POP_VARN`) listed.
- No retire of `varns []int32` / `varps []int32` int-shape (TS keeps the int-shape too; STRING is parallel, not replacement).
- No content (`*.rs2`) fixes.

---

## 4. Tracked deviations

### DEVIATION-NAI-121-D1 — `varn.dat` missing fails server boot (TS no-ops)

TS `VarNpcType.load(dir)` at `VarNpcType.ts:13-19` silently returns when `${dir}/server/varn.dat` is missing. Goscape `LoadVarnTypes` returns a wrapped error → server boot aborts.

**Why:** existing goscape pattern for varp/vars (`modules/world/server.go:220-229`) fail-loud at boot. Diverging would surface an inconsistency in a future cache-pipeline-gap diagnosis. The cost of failing loud is small (one `return` from `LoadVarnTypes`); the benefit is consistency with all other type loaders.

**How to apply:** if a future content-pack scenario surfaces where `varn.dat` legitimately doesn't ship, retire by inverting to TS shape (return nil, nil for ENOENT). No content-side test currently exercises absence.

### DEVIATION-NAI-121-D2 — `varnsString[i] = ""` on STRING reset (TS sets undefined)

TS `Npc.resetEntity(true)` at `Npc.ts:298-300` does `continue` on STRING, then `varsString.fill(undefined)` after the loop. Goscape uses zero-value `""`.

**Why:** Go has no nil-string. `""` is the natural zero. Observable behavior is identical for current Content (zero STRING-typed varns).

**How to apply:** if a future Content `.varn` declares STRING and any consumer depends on `undefined`-vs-`""` distinction in scripts, revisit. Likely impossible — TS scripts coerce `undefined` to `""` on string ops anyway.

### DEVIATION-NAI-121-D3 — `s.varnTypes == nil` falls through to int-only opcode dispatch

In test paths that don't seed `s.varnTypes`, `PUSH_VARN` / `POP_VARN` opcode handlers fall back to int-only behavior (no STRING dispatch).

**Why:** lets existing test fixtures continue using minimal `*Server` without a Configs round-trip. Documented `(goscape defensive; TS skips this check)` per `defensive_gate_doc_comment_label`.

**How to apply:** retire by audit when every Configs-touching test seeds `varnTypes` (probably never).

### DEVIATION-NAI-121-D4 — VarPlayerType `Protect` gate is new in goscape

TS `CoreOps.ts:50-52` enforces `varpType.protect` on `POP_VARP`. Goscape's existing handler doesn't. Bundle 1 adds the gate; pre-existing tests must stay green (no protected-varp test fixtures exist; TS `getProtectedActivePlayer` semantics map to goscape's `protectedScriptActive` helper at `player_script.go:302`).

**Why:** TS-fidelity per the §3.1 scope decision. The gate prevents content scripts from popping protected varps outside protected-script context — a real engine-side invariant.

**How to apply:** none expected to retire. If Content surfaces a script that pops a protected varp without protected context, that's a Content bug, not an engine deviation.

### DEVIATION-NAI-121-D5 — `varns`/`varps` size pinned at server start, not dynamically

Once `s.varnTypes.Configs` is read and entities are sized, mid-run reload of the registry would not resize live entities. Goscape doesn't support cache hot-reload anyway (no precedent across NAI-1..NAI-120). Tracked for parity audit; no retirement path.

---

## 5. Architecture

### 5.1 Subsystem overview

```
┌─────────────────────────────────────────────────────────────────────┐
│ pkg/objtype/                                                         │
│   varntype.go (NEW)                                                  │
│     ├── VarNpcType struct                                            │
│     ├── LoadVarnTypes(dir) → *VarnTypeConfigs                        │
│     └── parseVarnTypes(*Packet)                                      │
│   varptype.go / varstype.go (UNCHANGED)                              │
└─────────────────────────────────────────────────────────────────────┘
                                  │
                                  ▼
┌─────────────────────────────────────────────────────────────────────┐
│ modules/world/server.go                                              │
│   LoadVarnTypes wired alongside LoadVarpTypes/LoadVarsTypes          │
│   s.varnTypes *objtype.VarnTypeConfigs (NEW field)                   │
│   s.configsView extended with VarpType(id), VarnType(id)             │
└─────────────────────────────────────────────────────────────────────┘
                                  │
                ┌─────────────────┴─────────────────┐
                ▼                                   ▼
┌──────────────────────────────┐   ┌──────────────────────────────┐
│ modules/world/npc.go         │   │ modules/world/player.go      │
│   varns []int32 (sized)      │   │   varps []int32 (sized)      │
│   varnsString []string (NEW) │   │   varpsString []string (NEW) │
└──────────────────────────────┘   └──────────────────────────────┘
                │                                   │
                ▼                                   ▼
┌──────────────────────────────┐   ┌──────────────────────────────┐
│ resetEntityForRespawn        │   │ tick.go:105 (player init)    │
│   per-type seed loop         │   │   per-type seed loop         │
│   (TS Npc.ts:296-303)        │   │   (TS Player.ts:424-432)     │
│ NewNpc: same loop guarded    │   │                              │
└──────────────────────────────┘   └──────────────────────────────┘
                │                                   │
                └─────────────────┬─────────────────┘
                                  ▼
┌─────────────────────────────────────────────────────────────────────┐
│ pkg/script/handlers_vars.go                                          │
│   handlePushVarp / handlePopVarp — type-aware + Protect gate         │
│   handlePushVarn / handlePopVarn — type-aware                        │
│ pkg/script/active.go                                                 │
│   ActivePlayer / ActiveNpc extended with String accessors            │
│   World view extended with VarpType(id), VarnType(id)                │
└─────────────────────────────────────────────────────────────────────┘
```

### 5.2 Data flow — smoke-binding path

```
NewNpc(...) → varns = make([]int32, count); varnsString = make([]string, count)
                ↓
addNpc(n) → resetEntityForRespawn(n)
                ↓
            for each (i, vt) in s.varnTypes.Configs:
                STRING → varnsString[i] = ""
                INT    → varns[i] = 0
                else   → varns[i] = -1                            ★ THE FIX
                ↓
            AI_SPAWN trigger queued (existing)
                ↓
[opnpc2,giant_rat] → @player_melee_attack → [proc,player_in_combat_check]
   → player_combat.rs2:107
       if (%npc_macro_event_target ! null & %npc_macro_event_target ! uid) {
         PUSH_VARN(id=N where Type=PLAYER_UID)
                ↓
         s.World.VarnType(N) → ScriptVarTypePlayerUid
                ↓
         s.PushInt(int(npc.NpcVarN(N)))  →  pushes -1            ★ pre-fix: pushed 0
                ↓
         BRANCH_NOT_EQUALS(-1) → -1 ≠ -1 false → gate skips
                ↓
         "It's not after you." NOT printed → continues to first hit  ✅
```

### 5.3 Data flow — STRING-typed varn (no current content driver, tested for parity)

```
POP_VARN(id) where VarnType(id) = STRING:
  popString → npc.SetNpcVarNString(id, str)

PUSH_VARN(id) where VarnType(id) = STRING:
  pushString(npc.NpcVarNString(id))
```

### 5.4 Data flow — POP_VARP with Protect gate

```
POP_VARP(id):
  typ, protect = s.World.VarpType(id)
  if protect && !s.protectedScriptActive():
    return error "POP_VARP: %{id} requires protected access"
  if typ == STRING:
    p.SetVarpString(id, s.PopString())
  else:
    p.SetVarp(id, int32(s.PopInt()))
```

---

## 6. Components

### 6.1 `pkg/objtype/varntype.go` (NEW, ~70 LOC)

```go
package objtype

import (
    "fmt"
    "path/filepath"
    packet2 "github.com/zsrv/goscape/pkg/io/packet"
)

type VarNpcType struct {
    ConfigType
    Type ScriptVarType
}

func (v *VarNpcType) Decode(code uint8, dat *packet2.Packet) error {
    switch code {
    case 1:
        v.Type = ScriptVarType(dat.G1())
    case 250:
        v.DebugName = dat.GJStrLF()
    default:
        return fmt.Errorf("unrecognized varn config code %d", code)
    }
    return nil
}

func NewVarNpcType(id int) *VarNpcType {
    return &VarNpcType{ConfigType: ConfigType{ID: id}, Type: ScriptVarTypeInt}
}

type VarnTypeConfigs struct {
    ConfigNames map[string]int
    Configs     []*VarNpcType
}

func LoadVarnTypes(dir string) (*VarnTypeConfigs, error) {
    server, err := packet2.Load(filepath.Join(dir, "server", "varn.dat"), false)
    if err != nil {
        return nil, err
    }
    return parseVarnTypes(server)
}

func parseVarnTypes(server *packet2.Packet) (*VarnTypeConfigs, error) {
    count := int(server.G2())
    configs := make([]*VarNpcType, count)
    configNames := make(map[string]int, count)
    for id := range count {
        config := NewVarNpcType(id)
        if err := DecodeType(server, config); err != nil {
            return nil, err
        }
        configs[id] = config
        if config.DebugName != "" {
            configNames[config.DebugName] = id
        }
    }
    return &VarnTypeConfigs{ConfigNames: configNames, Configs: configs}, nil
}
```

### 6.2 `modules/world/server.go` wire-up

After `LoadVarsTypes` block (line 224-229):

```go
varnTypes, err := objtype.LoadVarnTypes(cfg.CachePath)
if err != nil {
    return nil, fmt.Errorf("load varn types: %w", err)
}
s.varnTypes = varnTypes
```

Add `varnTypes *objtype.VarnTypeConfigs` field to `*Server` struct.

### 6.3 `modules/world/npc.go` field shape

```go
type Npc struct {
    // existing...
    varns       []int32   // nil at NewNpc; sized + seeded by resetEntityForRespawn
    varnsString []string  // NEW; parallel
    // ...
}
```

**Sizing decision:** Seed materialization happens in `resetEntityForRespawn` (called inside `Server.addNpc`). `NewNpc` itself initializes `varns`/`varnsString` to nil; reads via `NpcVarN(id)` defensively return 0 / "" on `len == 0`. This keeps `&Npc{}` test literals working (player_script_test.go:1388, npc_masks_test.go:139) without panic. **Production NPC creation always goes through `Server.addNpc` → `resetEntityForRespawn`, which is where slice allocation and per-type seeding happen.** `NewNpc` does not size the slices because it has no back-reference to the server registry.

### 6.4 `modules/world/npc_registry.go::resetEntityForRespawn` (after hunt-field reset, ~14 LOC)

```go
// TS Npc.ts:296-306 — per-type varn seed at respawn (and at fresh
// spawn since World.addNpc → resetEntity(true) inside TS as well).
// goscape defensive: nil varnTypes (test paths) → no-op.
if s.varnTypes != nil {
    if len(n.varns) != len(s.varnTypes.Configs) {
        n.varns = make([]int32, len(s.varnTypes.Configs))
        n.varnsString = make([]string, len(s.varnTypes.Configs))
    }
    for i, vt := range s.varnTypes.Configs {
        switch vt.Type {
        case objtype.ScriptVarTypeString:
            n.varnsString[i] = ""
        case objtype.ScriptVarTypeInt:
            n.varns[i] = 0
        default:
            n.varns[i] = -1
        }
    }
}
```

### 6.5 `modules/world/player.go` + `tick.go:105`

```go
type Player struct {
    // existing...
    varps       []int32
    varpsString []string  // NEW
    // ...
}
```

`tick.go:105` block (current is single line):

```go
// Per-type seed loop — TS Player.ts:418-432.
p.varps = make([]int32, len(s.varpTypes.Configs))
p.varpsString = make([]string, len(s.varpTypes.Configs))
for i, vt := range s.varpTypes.Configs {
    switch vt.Type {
    case objtype.ScriptVarTypeString:
        // varpsString[i] = "" already
    case objtype.ScriptVarTypeInt:
        // varps[i] = 0 already
    default:
        p.varps[i] = -1
    }
}
```

### 6.6 NPC accessor methods (npc_script.go, ~30 LOC delta)

```go
func (n *Npc) NpcVarN(id int) int32 {
    if id < 0 || id >= len(n.varns) {
        return 0
    }
    return n.varns[id]
}

func (n *Npc) SetNpcVarN(id int, val int32) {
    if id < 0 || id >= len(n.varns) {
        return
    }
    n.varns[id] = val
}

// NEW
func (n *Npc) NpcVarNString(id int) string {
    if id < 0 || id >= len(n.varnsString) {
        return ""
    }
    return n.varnsString[id]
}

// NEW
func (n *Npc) SetNpcVarNString(id int, val string) {
    if id < 0 || id >= len(n.varnsString) {
        return
    }
    n.varnsString[id] = val
}
```

`npcVarnCap = 1024` constant + its bound check in `SetNpcVarN` are retired (slice now sized to registry; OOB ids silently ignored, identical observable behavior).

### 6.7 Player accessor methods (player_script.go, ~25 LOC delta)

```go
// existing Varp / SetVarp unchanged in shape

// NEW
func (p *Player) VarpString(id int) string {
    if id < 0 || id >= len(p.varpsString) {
        return ""
    }
    return p.varpsString[id]
}

// NEW
func (p *Player) SetVarpString(id int, val string) {
    if id < 0 || id >= len(p.varpsString) {
        return
    }
    p.varpsString[id] = val
    // No wire-send: this protocol revision has no varp_string opcode.
}
```

### 6.8 `pkg/script/active.go` interface extensions

```go
type ActivePlayer interface {
    // existing...
    Varp(id int) int32
    SetVarp(id int, val int32)
    VarpString(id int) string                   // NEW
    SetVarpString(id int, val string)           // NEW
}

type ActiveNpc interface {
    // existing...
    NpcVarN(id int) int32
    SetNpcVarN(id int, val int32)
    NpcVarNString(id int) string                // NEW
    SetNpcVarNString(id int, val string)        // NEW
}

// World view (already exists with VarsInt/SetVarsInt etc.); extend:
type World interface {
    // existing...
    VarpType(id int) (typ objtype.ScriptVarType, protect bool)  // NEW
    VarnType(id int) objtype.ScriptVarType                      // NEW
}
```

`(*Server)` (or `worldVarsView` / `serverConfigsView`) implements via the loaded registries; out-of-range id returns `(ScriptVarTypeInt, false)` and `ScriptVarTypeInt` respectively.

### 6.9 Opcode handler dispatch (handlers_vars.go, full rewrite ~70 LOC)

```go
func handlePushVarp(s *ScriptState) error {
    if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
        return errors.New("PUSH_VARP: no active player")
    }
    id := varOperandID(s)
    typ, _ := s.varType(id, true /* varp */)
    if typ == objtype.ScriptVarTypeString {
        s.PushString(s.Self.VarpString(id))
    } else {
        s.PushInt(int(s.Self.Varp(id)))
    }
    return nil
}

func handlePopVarp(s *ScriptState) error {
    if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
        return errors.New("POP_VARP: no active player")
    }
    id := varOperandID(s)
    typ, protect := s.varType(id, true /* varp */)
    if protect && !s.protectedActivePlayer() {
        return fmt.Errorf("POP_VARP: %%%d requires protected access", id)
    }
    if typ == objtype.ScriptVarTypeString {
        s.Self.SetVarpString(id, s.PopString())
    } else {
        s.Self.SetVarp(id, int32(s.PopInt()))
    }
    return nil
}

func handlePushVarn(s *ScriptState) error {
    if s.ActiveNpc == nil {
        return errors.New("PUSH_VARN: no active npc")
    }
    id := varOperandID(s)
    typ, _ := s.varType(id, false /* varn */)
    if typ == objtype.ScriptVarTypeString {
        s.PushString(s.ActiveNpc.NpcVarNString(id))
    } else {
        s.PushInt(int(s.ActiveNpc.NpcVarN(id)))
    }
    return nil
}

func handlePopVarn(s *ScriptState) error {
    if s.ActiveNpc == nil {
        return errors.New("POP_VARN: no active npc")
    }
    id := varOperandID(s)
    typ, _ := s.varType(id, false /* varn */)
    if typ == objtype.ScriptVarTypeString {
        s.ActiveNpc.SetNpcVarNString(id, s.PopString())
    } else {
        s.ActiveNpc.SetNpcVarN(id, int32(s.PopInt()))
    }
    return nil
}
```

The `s.varType(id, isVarp)` helper centralizes the World view lookup with degraded-mode fallback to `ScriptVarTypeInt` when World is nil or registry isn't seeded (DEVIATION-D3).

`s.protectedActivePlayer()` reads `s.Self.protectedScriptActive()` via an interface bridge; mirrors goscape's existing `(*Player).protectedScriptActive` at `player_script.go:302`.

### 6.10 `npcVarnCap` retirement

```go
// Delete from npc_script.go:13-15:
const npcVarnCap = 1024
```

Single-site retirement; no other callers. Mention in commit message.

---

## 7. V-PARTIAL Stage 1 investigation (Bundle 2)

### 7.1 Goal

Identify the precise step that drops `%npc_combat_xp_multiplier` so its read after `[ai_spawn,_]` returns 0 instead of the npc_param value.

### 7.2 Method (read-only)

Sonnet Explore subagent. Scope:

1. **Script-pack inclusion check.** Verify `Content/scripts/npc/scripts/ai_spawn.rs2` is compiled into goscape's loaded `script.dat`. Method: `rg` for the script's signature in goscape's `pkg/script/provider.go`-loaded data, OR run a read-only goscape boot and dump the registered global-trigger keys.
2. **Provider lookup verification.** With a typical NPC type id (e.g. tutorial-island giant rat), call `s.scriptProvider.GetByTrigger(script.TriggerAiSpawn, typeId, category)`. Should return a non-nil ScriptFile if (1) holds.
3. **Dispatch path verification.** Trace from `npc_registry.go:88-99` through `tick.go:42 → processNpcEventQueue → npc_event_queue.go::processNpcEventQueue` to the actual ScriptRunner invocation. Confirm no early-skip / delay branch fires for fresh-spawn NPCs.
4. **`npc_param` opcode verification.** Locate the opcode handler for `npc_param(combat_xp_multiplier)` in `pkg/script/handlers_*.go`. Verify it reads the right field from `n.typ`.

### 7.3 Output

`docs/superpowers/investigations/2026-05-07-nai-121-vpartial-findings.md` enumerating:
- Which of (1)-(4) holds and which breaks.
- The precise file:line of the breaking step.
- Sized recommendation for Bundle 3 (one-line fix / single opcode port / cross-system port / route-to-NAI-122).

### 7.4 Cadence

Single Sonnet subagent, no production code. Commit findings doc separately.

---

## 8. V-PARTIAL Stage 2 fix (Bundle 3)

### 8.1 Sizing

Determined entirely by Bundle 2 findings. Pre-Bundle-2 sketch:

| Finding | Likely fix | LOC | Cadence |
|---|---|---|---|
| `[ai_spawn,_]` not in script.dat | Content-pack pipeline rebuild + commit refresh | 0-20 (mostly cache regen) | Direct edit |
| `Provider.GetByTrigger` lookup-key bug | Lookup-key fix | ≤30 | Direct edit |
| `processNpcEventQueue` skip-condition bug | Branch fix | ≤30 | Direct edit |
| `npc_param` opcode missing for `combat_xp_multiplier` param | Opcode port | 50-150 | subagent-driven |
| Cross-system pipeline issue | Out of NAI-121 scope | n/a | Carry forward to NAI-122 |

### 8.2 Close criteria

- If Bundle 3 ships: smoke confirms combat XP > 0 on hit AND varn-seed bind from Bundle 1 holds.
- If Bundle 3 routes to NAI-122: NAI-121 closes on Bundle 1 PRIMARY (smoke binds the "It's not after you." gate); V-PARTIAL re-routes with detailed findings doc.

---

## 9. Risk register

### R1 — Player varp default change from 0 to -1 regresses tests
Player varps in goscape have been zero-init for NAI-1..NAI-120. Tests / handlers may implicitly rely on 0-reads for non-INT varps.

**Mitigation:** Bundle 1 controller pre-flight greps `pkg/script/handlers_*.go` and `modules/world/*_test.go` for non-INT varp reads with implicit-0 assertions. Any hits → reframe as deviation. Real-fixture `varp.dat` is loaded via `objtype.LoadVarpTypes` from the actual cache; the registry size + default rule applies as the smoke binds.

### R2 — `&Npc{}` test literals panic on STRING dispatch path
Raw `&Npc{}` literals at `player_script_test.go:1388` and `npc_masks_test.go:139` have nil `varns`/`varnsString` slices. `NpcVarN(id) → 0` and `NpcVarNString(id) → ""` defensively, but if any test reaches these via PUSH_VARN with a STRING-typed registry id, the dispatch path needs to gracefully fall back.

**Mitigation:** Bundle 1 includes a unit test `TestPushVarn_RawNpcLiteral_NoPanic` exercising this exact path. Defensive guards in §6.6 cover the read.

### R3 — `s.World.VarpType(id) / VarnType(id)` not implemented on test mock-worlds
Existing test scaffolds may use partial mock-World implementations.

**Mitigation:** Bundle 1 identifies all mock-World implementations (`pkg/script/runner_test.go::mockWorld`, etc.) and adds the two new methods returning the int-default sentinel. Listed in plan task list.

### R4 — V-PARTIAL Bundle 2 surfaces deeper pipeline issue
Bundle 2 might find that Content's `[ai_spawn,_]` script isn't in goscape's `script.dat` due to a pipeline gap.

**Mitigation:** Pre-declared escape hatch in §8.1: NAI-121 closes on Bundle 1 PRIMARY; V-PARTIAL routes to NAI-122 with findings doc. No scope creep.

### R5 — Protect-gate breaks an existing test silently
The new POP_VARP Protect gate (DEVIATION-D4) might fire on a test fixture that pops a Protect=true varp without protected context.

**Mitigation:** Bundle 1 controller pre-flight loads `varpTypes` from real-fixture and iterates `Configs` for `Protect=true` entries + cross-references with all `POP_VARP` test fixtures (search `pkg/script/handlers_vars_test.go` and `modules/world/*_test.go` for `POP_VARP` opcode and `SetVarp` test calls). Pre-existing test on Protect-flagged varp without protected context → expected behavior change; update test or document deviation expansion.

### R6 — Test uses `&Npc{}` then writes via SetNpcVarN(id=0) and reads back; pre-fix returned 0, post-fix would still return the written value
Currently `SetNpcVarN(0, val)` lazy-grows the slice and writes; post-fix `&Npc{}` literal has nil slice and the write is silently dropped.

**Mitigation:** This IS a behavioral change for raw-literal tests. Grep all `&Npc{}.SetNpcVarN(...)` sites; convert to `NewNpc(...)` or seed varns explicitly. Bundle 1 plan task lists every site for conversion.

---

## 10. Test plan

### 10.1 Bundle 1 unit tests

**`pkg/objtype/varntype_test.go`** (new, ~100 LOC):
- `TestParseVarnTypes_DefaultIsInt`
- `TestParseVarnTypes_TypeCode1_SetsType`
- `TestParseVarnTypes_DebugNameCode250_SetsName`
- `TestParseVarnTypes_UnknownCode_ReturnsError`
- `TestParseVarnTypes_TerminatorCode0_StopsParse`
- `TestLoadVarnTypes_FileMissing_ReturnsError`
- `TestLoadVarnTypes_RealFixture_AntimacroVarn` (mirrors antimacro.varn: 1 entry, type=player_uid)

**`modules/world/npc_registry_test.go`** (new tests, ~80 LOC):
- `TestResetEntityForRespawn_SeedsIntToZero`
- `TestResetEntityForRespawn_SeedsPlayerUidToMinusOne`
- `TestResetEntityForRespawn_SeedsCoordToMinusOne`
- `TestResetEntityForRespawn_SeedsNpcUidToMinusOne`
- `TestResetEntityForRespawn_SeedsStringToEmpty`
- `TestResetEntityForRespawn_NilVarnTypes_NoOp`
- `TestAddNpc_FreshSpawn_PlayerUidVarnReadsMinusOne` ★ smoke-binding test
- `TestAddNpc_RespawnAfterChangeType_ReseedsVarns`

**`modules/world/tick_test.go` or `player_login_test.go`** (~50 LOC):
- `TestPlayerInit_VarpsSeededByType_IntZero`
- `TestPlayerInit_VarpsSeededByType_NpcUidMinusOne`
- `TestPlayerInit_VarpsSeededByType_StringEmpty`
- `TestPlayerInit_VarpsLengthMatchesRegistry`

**`pkg/script/handlers_vars_test.go`** (extend, ~120 LOC):
- `TestPushVarp_IntType_ExistingBehavior` (existing — confirm green)
- `TestPushVarp_StringType_PushesString` (NEW)
- `TestPopVarp_StringType_PopsString` (NEW)
- `TestPopVarp_ProtectGate_DeniesUnprotected` (NEW)
- `TestPopVarp_ProtectGate_AllowsProtected` (NEW)
- `TestPopVarp_NonProtect_NoGate` (NEW)
- `TestPushVarn_IntType_ExistingBehavior` (existing — confirm green)
- `TestPushVarn_StringType_PushesString` (NEW)
- `TestPopVarn_StringType_PopsString` (NEW)
- `TestPushVarn_PlayerUidDefault_PushesMinusOne` ★ unit smoke-bind pin
- `TestPushVarn_RawNpcLiteralNoPanic` (R2 mitigation)
- `TestPushVarn_NilWorldFallsBackToInt` (DEVIATION-D3 pin)

### 10.2 Bundle 2 — no tests (read-only investigation)

### 10.3 Bundle 3 — sized from Bundle 2

Test surface determined by fix shape. At minimum: a unit pin that the fixed code path produces a non-zero `%npc_combat_xp_multiplier` read after `[ai_spawn,_]` runs.

### 10.4 Smoke (user-launched)

After Bundle 1 close at minimum:
- ✅ Tutorial Island giant-rat melee attack: "It's not after you." NO LONGER prints.
- ✅ Player attacks land; combat-init proceeds to first hit.
- ⚠️ Combat XP may still be 0 (V-PARTIAL) — recorded but not blocking Bundle 1 close.

After Bundle 3 (if shipped):
- ✅ Combat XP > 0 on hit.

### 10.5 Cross-package green check

After every bundle: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...` (full repo, not just `./modules/world/...`) per `verify_implementer_claims`.

---

## 11. Bundle structure

**Bundle 1 — Smoke-binding fix.**
- One subagent-driven-development cycle. Sonnet implementer, Sonnet code-reviewer between bundles per `superpowers_code_reviewer_model`.
- Plan-author crosschecks: every `&Npc{}` test literal site (R6); every mock-World implementation (R3); every Protect=true varp in real-fixture `varp.dat` (R5); every `npcVarnCap` reference (single site expected).
- LOC target: 250-400 (registry + entity field + opcode dispatch + tests).
- Estimated split contingency: if LOC exceeds 400, split into Bundle 1A (registry + per-type seed) and Bundle 1B (opcode dispatch + Protect gate). Decided at plan-write.

**Bundle 2 — V-PARTIAL Stage 1 audit.**
- Single Sonnet Explore subagent, read-only.
- LOC target: 0 (production), ~150-300 lines findings doc.
- Output commit: findings doc only.

**Bundle 3 — V-PARTIAL Stage 2 fix.**
- Sized from Bundle 2 findings per §8.1.
- May be direct edit (≤30 LOC), subagent-driven-development (50-150 LOC), or carry-forward to NAI-122 (>200 LOC or out-of-scope).

---

## 12. Close criteria

1. **Bundle 1:** `go test ./...` green at HEAD; controller verifies via independent fresh run per `verify_implementer_claims`.
2. **Bundle 2:** Findings doc committed and routes Bundle 3 sizing.
3. **Bundle 3 (if shipped):** `go test ./...` green; targeted unit pin for the V-PARTIAL fix.
4. **User-launched smoke:** Tutorial Island giant-rat attack does NOT print "It's not after you." and proceeds to first hit. (Combat XP > 0 only required if Bundle 3 ships.)
5. **Memory:** `Closes memory:` trailer on close commit listing every memory entry referenced (`npc_varn_default_seed_per_type.md`, `nai_followups.md` NAI-120 carry-forward V-PARTIAL).
6. **`nai_followups.md` updated:** NAI-120 V-PARTIAL parked entry retired (or re-parked with NAI-122 reference if Bundle 3 routes forward); NAI-121 entry added with full close summary.

---

## 13. Pattern memories applied

- `investigation_subspec_cadence` — Bundle 1 fix → Bundle 2 audit → conditional Bundle 3 fix → smoke.
- `controller_preflight` — every bundle gated on a controller pre-flight grep pass before implementer dispatch.
- `verify_implementer_claims` — fresh `go test ./...` after every implementer dispatch; cross-package green check.
- `superpowers_code_reviewer_model` — every reviewer subagent on Sonnet.
- `defensive_gate_doc_comment_label` — DEVIATION-D3 fall-through path labeled.
- `true_to_ts_gate` — full TS-fidelity scope choice (per Q4 (A)); D1, D2, D5 tracked deviations with retire conditions.
- `dispatch_correct_reach_blocked` — Bundle 1 PRIMARY closes on smoke-bind even if Bundle 3 routes forward.
- `cascade_theory_smoke_binding` — Bundle 1 smoke binds; Bundle 3 smoke (if shipped) binds combat-XP residual.
- `smoke_test_server_handoff` — user-launched smoke; controller waits on stderr report.
- `close_commit_memory_trailer` — applied on close commit.
- `dead_api_polish` — STRING accessors land with TS-fidelity rationale despite zero current consumers (per scope-(A) Q4 choice — accepted with eyes open).
- `verify_followup_tracker_freshness` — NAI-120 V-PARTIAL entry re-verified at spec-write (AI_SPAWN dispatch confirmed wired in `npc_registry.go:82-99`; `[ai_spawn,_]` script content confirmed in `Content/scripts/npc/scripts/ai_spawn.rs2`; root cause is downstream).
- `plan_runnable_test_fixtures` — every plan-codified test mentally executed before dispatch.
- `audit_full_method_when_restructuring` — when Bundle 1 reviews `resetEntityForRespawn`, full-method audit (not just the new lines) per `tracker_entry_framing_can_be_incomplete`.

---

## 14. References

- TS `Engine-TS/src/cache/config/VarNpcType.ts` — registry shape.
- TS `Engine-TS/src/cache/config/ScriptVarType.ts` — type constants.
- TS `Engine-TS/src/engine/entity/Npc.ts:78-104, 195-208, 280-317` — ctor (no seed) + `getVar`/`setVar` + `resetEntity` (per-type seed).
- TS `Engine-TS/src/engine/entity/Player.ts:418-432, 1710-1730` — ctor (per-type seed) + `getVar`/`setVar`.
- TS `Engine-TS/src/engine/script/handlers/CoreOps.ts:21-91` — opcode dispatch.
- TS `Engine-TS/src/engine/World.ts:1268-1294` — `addNpc` calls `resetEntity(true)`.
- Goscape `modules/world/npc_script.go:79-103` — current type-blind read/write.
- Goscape `modules/world/npc_registry.go:115-145` — current `resetEntityForRespawn` (missing seed loop).
- Goscape `modules/world/tick.go:105` — current player varps zero-init.
- Goscape `pkg/script/handlers_vars.go` — current type-blind opcode handlers.
- Goscape `pkg/objtype/varptype.go`, `varstype.go` — registry templates to mirror.
- Memory: `npc_varn_default_seed_per_type.md`, `nai_followups.md` (NAI-120 V-PARTIAL parked entry).
- Smoke 2026-05-07 (NAI-120 close commit `527259a` log).
