# Sub-spec RuneScript S5b: VARP + VARS Infrastructure — Design

**Status:** Draft → ready for plan
**Scope:** Full VARP (per-player integer variable) support — cache config loading, per-Player int32 storage, PUSH_VARP / POP_VARP handlers, wire sync via `VARP_SMALL` / `VARP_LARGE`. World-shared VARS (int + string) support — cache config loading, server-scoped storage, PUSH_VARS / POP_VARS handlers. VARN stubs so real scripts don't abort; full VARN implementation waits on S6 (active_npc).
**Out of scope:** VARBIT (not implemented in TS either; compiler bakes varbit refs into VARP+bitmask at compile time), secondary active_player (operand high bit ignored), SCOPE_PERM persistence (all varps session-local), initial-login varp bulk sync (all varps start at 0).

---

## Goal

After S5b:

- Cache scripts that read or write VARP (the ~1500 named integer variables per player) run without `unknown opcode` errors. Writes to `transmit=true` VARPs reach the Java client as `VARP_SMALL` or `VARP_LARGE` packets.
- Cache scripts that read or write VARS (world-shared int/string variables) run against `Server.vars[]` / `Server.varsStrings[]` arrays.
- Cache scripts that touch VARN don't crash — the stub handlers return 0 for reads and drop writes.
- Demo: a synthetic `%foo = 42; mes(%foo)` script writes to the player's varps array, emits a VARP_SMALL(foo_id, 42) packet on the wire, and prints `42` via MessageGame. Verified by unit test.

## Architecture

```
pkg/objtype/
├── varptype.go                     (new) VarPlayerType + LoadVarpTypes
├── varptype_test.go                (new)
├── varstype.go                     (new) VarSharedType + LoadVarsTypes
└── varstype_test.go                (new)

pkg/script/
├── active.go                       + Varp / SetVarp methods on ActivePlayer
├── state.go                        + World WorldVars field for VARS access
├── handlers_vars.go                (new) 6 handlers
└── handlers_vars_test.go           (new)

pkg/io/protocol/game/server/
└── prot.go                         + OpVarpSmall (150, 3), OpVarpLarge (175, 6)

modules/world/
├── player.go                       + varps []int32 field
├── player_script.go                + Varp(id) / SetVarp(id, val) impls
├── player_varp.go                  (new) writeVarp wire encoder
├── server.go                       + varpTypes, varsTypes, vars, varsStrings, fields + NewServer loads
├── server_varp.go                  (new) Server implements script.WorldVars
├── tick.go                         (processLogins) allocate p.varps
└── script_test.go                  + E2E VARP wire test
```

## Components

### 1. `pkg/objtype/varptype.go`

```go
type VarPlayerType struct {
    ConfigType
    Scope      uint8         // 0 = TEMP, 1 = PERM. S5b treats both as TEMP.
    Type       ScriptVarType // default INT
    Protect    bool          // default true
    ClientCode uint16
    Transmit   bool          // default false; only transmit=true writes go to the wire
}

const (
    VarpScopeTemp = 0
    VarpScopePerm = 1
)

func (v *VarPlayerType) Decode(code uint8, dat *packet.Packet) error {
    switch code {
    case 1: v.Scope = dat.G1()
    case 2: v.Type = ScriptVarType(dat.G1())
    case 4: v.Protect = false
    case 5: v.ClientCode = uint16(dat.G2())
    case 6: v.Transmit = true
    case 250: v.DebugName = dat.GJStrLF()
    default: return fmt.Errorf("varp: unknown code %d", code)
    }
    return nil
}

type VarpTypeConfigs struct {
    ConfigNames map[string]int
    Configs     []*VarPlayerType
}

func LoadVarpTypes(cacheDir string) (*VarpTypeConfigs, error) { ... }
```

File format per TS: `data/pack/server/varp.dat` starts with `u16 count`, followed by N config blocks each terminated by code 0.

### 2. `pkg/objtype/varstype.go`

```go
type VarSharedType struct {
    ConfigType
    Type ScriptVarType // INT or STRING
}

func (v *VarSharedType) Decode(code uint8, dat *packet.Packet) error {
    switch code {
    case 1: v.Type = ScriptVarType(dat.G1())
    case 250: v.DebugName = dat.GJStrLF()
    default: return fmt.Errorf("vars: unknown code %d", code)
    }
    return nil
}

type VarsTypeConfigs struct {
    ConfigNames map[string]int
    Configs     []*VarSharedType
}

func LoadVarsTypes(cacheDir string) (*VarsTypeConfigs, error) { ... }
```

### 3. `pkg/script/active.go` — interface extension

```go
type ActivePlayer interface {
    // ... existing methods ...

    // Varp returns the player's current value for varp id. Returns 0 on OOB.
    Varp(id int) int32

    // SetVarp writes val to the player's varp storage. If the varp type
    // is transmit=true, SetVarp also queues a VARP_SMALL/VARP_LARGE
    // packet to the client. OOB writes are silently dropped.
    SetVarp(id int, val int32)
}
```

### 4. `pkg/script/state.go` — WorldVars hook

```go
// WorldVars is the minimal surface that pkg/script needs from the
// hosting world to resolve PUSH_VARS / POP_VARS. The VM itself owns
// no world state; this decouples the two packages.
type WorldVars interface {
    VarsInt(id int) int32
    SetVarsInt(id int, val int32)
    VarsString(id int) string
    SetVarsString(id int, val string)
}

type ScriptState struct {
    // ... existing fields ...
    World WorldVars
}
```

`Init` takes an optional world hook (or callers set `state.World = s` after `Init`). Since VARS is rare and the VM already has a loose Provider hook pattern, we wire it the same way: callers set it explicitly. `runScript` in `modules/world/script.go` becomes:

```go
state := script.Init(sf, self, protect, intArgs, stringArgs)
state.Provider = s.scriptProvider
state.World = s.worldVarsView // new
```

### 5. `pkg/script/handlers_vars.go` — 6 handlers

Per TS `CoreOps.ts`:

- Operand = `IntOperands[PC]` (u32). Bits 0–15 are the varp/vars/varn ID. Bit 16 is the "secondary" flag (active_player2); **S5b ignores bit 16** and uses `operand & 0xffff`.
- `ScriptVarType`: VARS has both INT and STRING variants; the handler dispatches by type.

```go
func handlePushVarp(s *ScriptState) error {
    if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
        return errors.New("PUSH_VARP: no active player")
    }
    id := int(uint32(s.Script.IntOperands[s.PC]) & 0xffff)
    s.PushInt(int(s.Self.Varp(id)))
    return nil
}

func handlePopVarp(s *ScriptState) error {
    if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
        return errors.New("POP_VARP: no active player")
    }
    val := int32(s.PopInt())
    id := int(uint32(s.Script.IntOperands[s.PC]) & 0xffff)
    s.Self.SetVarp(id, val)
    return nil
}

func handlePushVars(s *ScriptState) error {
    if s.World == nil {
        return errors.New("PUSH_VARS: no world")
    }
    id := int(uint32(s.Script.IntOperands[s.PC]) & 0xffff)
    // Design call: VARS type determines int vs string pop. For MVP we
    // always push int (string vars are rare); revisit if tests reveal
    // real scripts need string VARS.
    s.PushInt(int(s.World.VarsInt(id)))
    return nil
}

func handlePopVars(s *ScriptState) error {
    if s.World == nil {
        return errors.New("POP_VARS: no world")
    }
    val := int32(s.PopInt())
    id := int(uint32(s.Script.IntOperands[s.PC]) & 0xffff)
    s.World.SetVarsInt(id, val)
    return nil
}

// VARN stubs — real impl ships with S6 (active_npc).
func handlePushVarn(s *ScriptState) error {
    s.PushInt(0)
    return nil
}

func handlePopVarn(s *ScriptState) error {
    _ = s.PopInt()
    return nil
}
```

Registered in `handlers.go` with an "S5b" comment block.

### 6. Wire opcodes — `pkg/io/protocol/game/server/prot.go`

```go
OpVarpSmall = Op{Opcode: 150, PayloadSize: 3} // u16 id + i8 value
OpVarpLarge = Op{Opcode: 175, PayloadSize: 6} // u16 id + i32 value
```

### 7. `modules/world/player_varp.go` — wire encoder

```go
// writeVarp sends VARP_SMALL (if value fits in i8) or VARP_LARGE.
// Gated by VarPlayerType.Transmit — untransmitted writes are server-
// only and do not reach the wire.
func (p *Player) writeVarp(id int, value int32) {
    cfg := p.varpTypeConfig(id)
    if cfg == nil || !cfg.Transmit {
        return
    }
    buf := packet.NewPacket(nil)
    buf.P2(uint16(id))
    if value >= -128 && value <= 127 {
        buf.P1(uint8(int8(value)))
        p.writeOut(gameserver.OpVarpSmall, buf.Bytes())
    } else {
        buf.P4(uint32(value))
        p.writeOut(gameserver.OpVarpLarge, buf.Bytes())
    }
}

// varpTypeConfig returns the VarPlayerType for id, or nil if OOB or
// the server hasn't loaded configs.
func (p *Player) varpTypeConfig(id int) *objtype.VarPlayerType {
    if p.client == nil || p.client.server == nil || p.client.server.varpTypes == nil {
        return nil
    }
    if id < 0 || id >= len(p.client.server.varpTypes.Configs) {
        return nil
    }
    return p.client.server.varpTypes.Configs[id]
}
```

### 8. `modules/world/player_script.go` — Varp / SetVarp impls

```go
func (p *Player) Varp(id int) int32 {
    if id < 0 || id >= len(p.varps) {
        return 0
    }
    return p.varps[id]
}

func (p *Player) SetVarp(id int, val int32) {
    if id < 0 || id >= len(p.varps) {
        return
    }
    p.varps[id] = val
    p.writeVarp(id, val)
}
```

### 9. `modules/world/server_varp.go` — WorldVars impl

```go
// worldVarsView is the read/write surface pkg/script uses to resolve
// VARS opcodes. Satisfies script.WorldVars.
type worldVarsView struct{ s *Server }

func (w worldVarsView) VarsInt(id int) int32 {
    if id < 0 || id >= len(w.s.vars) { return 0 }
    return w.s.vars[id]
}
func (w worldVarsView) SetVarsInt(id int, val int32) {
    if id < 0 || id >= len(w.s.vars) { return }
    w.s.vars[id] = val
}
func (w worldVarsView) VarsString(id int) string {
    if id < 0 || id >= len(w.s.varsStrings) { return "" }
    return w.s.varsStrings[id]
}
func (w worldVarsView) SetVarsString(id int, val string) {
    if id < 0 || id >= len(w.s.varsStrings) { return }
    w.s.varsStrings[id] = val
}
```

### 10. Server init + tick wiring

```go
// In NewServer (after other config loads):
s.varpTypes, err = objtype.LoadVarpTypes(cfg.CachePath)
if err != nil { return nil, fmt.Errorf("load varp types: %w", err) }
s.varsTypes, err = objtype.LoadVarsTypes(cfg.CachePath)
if err != nil { return nil, fmt.Errorf("load vars types: %w", err) }
s.vars = make([]int32, len(s.varsTypes.Configs))
s.varsStrings = make([]string, len(s.varsTypes.Configs))

// In processLogins, when allocating per-player state:
p.varps = make([]int32, len(s.varpTypes.Configs))
```

## Data flow

```
Script `%foo = 42`:
  handlePopVarp pops 42, reads operand (e.g. 0x00A7) → id=0xA7
  calls p.SetVarp(0xA7, 42)
    p.varps[0xA7] = 42
    p.writeVarp(0xA7, 42)
      cfg.Transmit true → VARP_SMALL wire packet
      payload = P2(0xA7) + P1(42) = 3 bytes
      writeOut queues to p.client.out

Next processClientsOut → flush to socket → client updates its varp table.
```

## Error handling

- **OOB VARP id**: `Varp` returns 0; `SetVarp` drops silently. Real scripts reference valid varps by name (resolved at compile time), so OOB only happens on malformed scripts — not worth aborting for.
- **OOB VARS id**: same treatment.
- **No `ActivePlayer` for PUSH_VARP / POP_VARP**: returns error, aborts script (matches TS).
- **No `World` for PUSH_VARS / POP_VARS**: returns error. In practice `runScript` always sets `state.World = s.worldVarsView`, so this only fires in hand-rolled tests.
- **`Transmit=false` VARP writes**: server-side only; wire write skipped. This is TS's exact behavior.

## Testing

### `pkg/objtype/varptype_test.go`

- Synthetic varp.dat with 3 entries: one transmit, one perm, one with debugname. Round-trip decode via LoadVarpTypes using a tempdir.
- Verify Transmit flag, Scope values, DebugName, ConfigNames map.

### `pkg/objtype/varstype_test.go`

- Synthetic vars.dat with INT + STRING entries. Verify type discrimination.

### `pkg/script/handlers_vars_test.go`

- `TestPushVarp`: mockPlayer returns preset varp; handler pushes the value.
- `TestPopVarp`: mockPlayer captures SetVarp call; handler calls with correct id + val.
- `TestPushVars` + `TestPopVars`: mockWorld implementing WorldVars.
- `TestVarnStubs`: PUSH_VARN returns 0, POP_VARN drops without error.
- `TestPushVarpRequiresActivePlayer`: no-Self case returns error.

### `modules/world/script_test.go`

- `TestVarpWireSync`: seed `varpTypes` with one transmit=true varp at id=0. Run script `push_constant_int 42, pop_varp 0, return`. Drain conn; expect VARP_SMALL wire bytes (opcode 150 + `P2(0)` + `P1(42)`).
- `TestVarpWireSyncLarge`: same but write value 10000 → expect VARP_LARGE (opcode 175 + `P2(0)` + `P4(10000)`).
- `TestVarpTransmitFalseNoWire`: write to transmit=false varp → drain expects zero bytes.

## LOC estimate

| File | LOC |
|---|---|
| `pkg/objtype/varptype.go` | 110 |
| `pkg/objtype/varstype.go` | 60 |
| `pkg/objtype/varptype_test.go` | 60 |
| `pkg/objtype/varstype_test.go` | 40 |
| `pkg/script/active.go` (diff) | +6 |
| `pkg/script/state.go` (diff) | +15 (WorldVars interface + field) |
| `pkg/script/handlers.go` (diff) | +8 (register 6 handlers) |
| `pkg/script/handlers_vars.go` | 70 |
| `pkg/script/handlers_vars_test.go` | 100 |
| `pkg/io/protocol/game/server/prot.go` (diff) | +2 |
| `modules/world/player.go` (diff) | +1 |
| `modules/world/player_script.go` (diff) | +20 |
| `modules/world/player_varp.go` | 55 |
| `modules/world/server.go` (diff) | +20 |
| `modules/world/server_varp.go` | 45 |
| `modules/world/tick.go` (diff) | +3 |
| `modules/world/script_test.go` (diff) | +100 |
| **Total** | **~715** |

## Key design calls

- **`WorldVars` interface on `ScriptState`** instead of a concrete server ref. Keeps pkg/script pure; the interface has 4 methods (VarsInt/SetVarsInt/VarsString/SetVarsString) and nothing else.
- **`Transmit` gate in `SetVarp`**, not in the handler. Handler always updates server state; wire sync is an implementation detail of the Player adapter.
- **VARN stubs rather than deferred opcodes**. Registering stubs means scripts don't abort; `PushInt(0)` semantics are TS-compatible for a nonexistent NPC context.
- **Operand mask `& 0xffff`** drops the secondary active_player bit cleanly. Write it explicitly in the handler so the scope stays visible to future readers.
- **`worldVarsView` as a struct** (not a `*Server` method set) so we can unit-test handlers without a full Server. `server_varp.go` defines the view and `Server` exposes it via `s.worldVarsView`.
- **VARS always pushes int for MVP**. Real scripts using string VARS are rare; if we see them in telemetry later, dispatch by `VarSharedType.Type` — a 5-line change.

## Gotchas

- **`isLargeOperand`**: verified — VARP (1), POP_VARP (2), VARN (4,5), VARS (11,12) all ≤100 and not in the small-operand exception set, so the decoder reads u32. No file-decoder change required.
- **ConfigType.Decode loop**: the existing `DecodeType(buf, cfg)` helper expects `code == 0` to terminate. Our decode functions must return without consuming extra bytes when they see code 0 — actually `DecodeType` handles that itself, so each `Decode` just needs to handle its own codes.
- **VarPlayerType's `Protect` default is `true`**: make sure the struct zero value reflects that. Either init explicitly in `NewVarPlayerType` or handle `code == 4` as "set Protect = false" (which matches TS where 4 is a disable-flag). Going with the latter — simpler.
- **Player resets**: when a player logs out and reconnects, the `varps` slice is freshly allocated zeroes. That matches TS's SCOPE_TEMP behavior. SCOPE_PERM varps need a persistence layer later.
- **Zero-value int32 vs nil slice**: `p.varps` is nil pre-login; `len(nil) == 0` so all OOB checks work. But the allocation in `processLogins` is required — put it next to the existing `p.invs = map[int]*inventory.Inventory{}` allocation for cohesion.
- **VARS int/string sharing an ID space**: TS uses separate arrays `World.vars` and `World.varsString`. Our `Server.vars` and `Server.varsStrings` mirror this — size both to `len(varsTypes.Configs)` so IDs are directly indexable.
