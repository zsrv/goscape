# RuneScript S5b: VARP + VARS Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Register PUSH_VARP / POP_VARP / PUSH_VARS / POP_VARS handlers against loaded VarPlayerType and VarSharedType config, with VARP writes going to the wire as VARP_SMALL (i8) or VARP_LARGE (i32) packets. VARN stubbed until S6.

**Architecture:** Add `pkg/objtype/varptype.go` and `pkg/objtype/varstype.go` following the existing `invtype.go` pattern. Extend `script.ActivePlayer` with `Varp` / `SetVarp` and `script.ScriptState` with a `World WorldVars` hook. Wire VARP_SMALL (opcode 150) and VARP_LARGE (opcode 175) in the server proto. Server implements WorldVars via `worldVarsView`.

**Tech Stack:** Go 1.22+, existing objtype loader pattern, existing `pkg/io/packet`, existing `script.Provider` wiring.

**Spec:** [`docs/superpowers/specs/2026-04-21-runescript-s5b-varp-vars-design.md`](../specs/2026-04-21-runescript-s5b-varp-vars-design.md)

---

## Task 1: VarPlayerType + VarSharedType config loaders

**Files:**
- Create: `pkg/objtype/varptype.go`
- Create: `pkg/objtype/varstype.go`
- Create: `pkg/objtype/varptype_test.go`
- Create: `pkg/objtype/varstype_test.go`

- [ ] **Step 1: Create `pkg/objtype/varptype.go`**

```go
package objtype

import (
	"fmt"
	"path/filepath"

	packet2 "github.com/zsrv/goscape/pkg/io/packet"
)

const (
	VarpScopeTemp = 0
	VarpScopePerm = 1
)

type VarPlayerType struct {
	ConfigType
	Scope      int
	Type       ScriptVarType
	Protect    bool
	ClientCode uint16
	Transmit   bool
}

func (v *VarPlayerType) Decode(code uint8, dat *packet2.Packet) error {
	switch code {
	case 1:
		v.Scope = int(dat.G1())
	case 2:
		v.Type = ScriptVarType(dat.G1())
	case 4:
		v.Protect = false
	case 5:
		v.ClientCode = uint16(dat.G2())
	case 6:
		v.Transmit = true
	case 250:
		v.DebugName = dat.GJStrLF()
	default:
		return fmt.Errorf("unrecognized varp config code %d", code)
	}
	return nil
}

func NewVarPlayerType(id int) *VarPlayerType {
	return &VarPlayerType{
		ConfigType: ConfigType{ID: id},
		Scope:      VarpScopeTemp,
		Type:       ScriptVarTypeInt,
		Protect:    true,
		Transmit:   false,
	}
}

type VarpTypeConfigs struct {
	ConfigNames map[string]int
	Configs     []*VarPlayerType
}

func LoadVarpTypes(dir string) (*VarpTypeConfigs, error) {
	server, err := packet2.Load(filepath.Join(dir, "server", "varp.dat"), false)
	if err != nil {
		return nil, err
	}
	return parseVarpTypes(server)
}

func parseVarpTypes(server *packet2.Packet) (*VarpTypeConfigs, error) {
	count := int(server.G2())

	configs := make([]*VarPlayerType, count)
	configNames := make(map[string]int, count)

	for id := range count {
		config := NewVarPlayerType(id)
		if err := DecodeType(server, config); err != nil {
			return nil, err
		}
		configs[id] = config
		if config.DebugName != "" {
			configNames[config.DebugName] = id
		}
	}

	return &VarpTypeConfigs{
		ConfigNames: configNames,
		Configs:     configs,
	}, nil
}
```

- [ ] **Step 2: Create `pkg/objtype/varstype.go`**

```go
package objtype

import (
	"fmt"
	"path/filepath"

	packet2 "github.com/zsrv/goscape/pkg/io/packet"
)

type VarSharedType struct {
	ConfigType
	Type ScriptVarType
}

func (v *VarSharedType) Decode(code uint8, dat *packet2.Packet) error {
	switch code {
	case 1:
		v.Type = ScriptVarType(dat.G1())
	case 250:
		v.DebugName = dat.GJStrLF()
	default:
		return fmt.Errorf("unrecognized vars config code %d", code)
	}
	return nil
}

func NewVarSharedType(id int) *VarSharedType {
	return &VarSharedType{
		ConfigType: ConfigType{ID: id},
		Type:       ScriptVarTypeInt,
	}
}

type VarsTypeConfigs struct {
	ConfigNames map[string]int
	Configs     []*VarSharedType
}

func LoadVarsTypes(dir string) (*VarsTypeConfigs, error) {
	server, err := packet2.Load(filepath.Join(dir, "server", "vars.dat"), false)
	if err != nil {
		return nil, err
	}
	return parseVarsTypes(server)
}

func parseVarsTypes(server *packet2.Packet) (*VarsTypeConfigs, error) {
	count := int(server.G2())

	configs := make([]*VarSharedType, count)
	configNames := make(map[string]int, count)

	for id := range count {
		config := NewVarSharedType(id)
		if err := DecodeType(server, config); err != nil {
			return nil, err
		}
		configs[id] = config
		if config.DebugName != "" {
			configNames[config.DebugName] = id
		}
	}

	return &VarsTypeConfigs{
		ConfigNames: configNames,
		Configs:     configs,
	}, nil
}
```

- [ ] **Step 3: Create `pkg/objtype/varptype_test.go`**

```go
package objtype

import (
	"testing"

	packet2 "github.com/zsrv/goscape/pkg/io/packet"
)

// buildVarpDat assembles a varp.dat wire blob matching the TS format:
//
//	u16 count
//	for each id: sequence of (code, payload) pairs terminated by code=0
func buildVarpDat(entries []struct {
	debugName string
	scope     int
	transmit  bool
}) []byte {
	pkt := packet2.NewPacket(nil)
	pkt.P2(uint16(len(entries)))
	for _, e := range entries {
		if e.scope != 0 {
			pkt.P1(1)
			pkt.P1(uint8(e.scope))
		}
		if e.transmit {
			pkt.P1(6)
		}
		if e.debugName != "" {
			pkt.P1(250)
			pkt.PJStrLF(e.debugName)
		}
		pkt.P1(0) // terminator
	}
	return pkt.Bytes()
}

func TestParseVarpTypes(t *testing.T) {
	entries := []struct {
		debugName string
		scope     int
		transmit  bool
	}{
		{"coins", 0, true},
		{"quest_state", 1, false},
		{"anon", 0, false},
	}

	blob := buildVarpDat(entries)
	pkt := packet2.NewPacket(blob)

	cfgs, err := parseVarpTypes(pkt)
	if err != nil {
		t.Fatalf("parseVarpTypes: %v", err)
	}
	if len(cfgs.Configs) != 3 {
		t.Fatalf("configs: got %d, want 3", len(cfgs.Configs))
	}
	if cfgs.Configs[0].DebugName != "coins" || !cfgs.Configs[0].Transmit {
		t.Errorf("coins: got %+v", cfgs.Configs[0])
	}
	if cfgs.Configs[1].Scope != VarpScopePerm {
		t.Errorf("quest_state scope: got %d, want %d", cfgs.Configs[1].Scope, VarpScopePerm)
	}
	if cfgs.ConfigNames["coins"] != 0 {
		t.Errorf("ConfigNames[coins]: got %d, want 0", cfgs.ConfigNames["coins"])
	}
}
```

- [ ] **Step 4: Create `pkg/objtype/varstype_test.go`**

```go
package objtype

import (
	"testing"

	packet2 "github.com/zsrv/goscape/pkg/io/packet"
)

func TestParseVarsTypes(t *testing.T) {
	pkt := packet2.NewPacket(nil)
	pkt.P2(2) // count
	// entry 0: int var "counter"
	pkt.P1(1)
	pkt.P1(uint8(ScriptVarTypeInt))
	pkt.P1(250)
	pkt.PJStrLF("counter")
	pkt.P1(0)
	// entry 1: string var "motd"
	pkt.P1(1)
	pkt.P1(uint8(ScriptVarTypeString))
	pkt.P1(250)
	pkt.PJStrLF("motd")
	pkt.P1(0)

	cfgs, err := parseVarsTypes(packet2.NewPacket(pkt.Bytes()))
	if err != nil {
		t.Fatalf("parseVarsTypes: %v", err)
	}
	if cfgs.Configs[0].Type != ScriptVarTypeInt {
		t.Errorf("counter type: got %v", cfgs.Configs[0].Type)
	}
	if cfgs.Configs[1].Type != ScriptVarTypeString {
		t.Errorf("motd type: got %v", cfgs.Configs[1].Type)
	}
	if cfgs.ConfigNames["motd"] != 1 {
		t.Errorf("ConfigNames[motd]: got %d, want 1", cfgs.ConfigNames["motd"])
	}
}
```

- [ ] **Step 5: Run tests + full build**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype/... -run 'TestParseVarp|TestParseVars' -v
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
```

Expected: PASS + clean build.

- [ ] **Step 6: Commit**

```bash
git add pkg/objtype/varptype.go pkg/objtype/varstype.go pkg/objtype/varptype_test.go pkg/objtype/varstype_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(objtype): VarPlayerType + VarSharedType cache loaders

Decodes data/pack/server/varp.dat and vars.dat following the existing
invtype/objtype pattern. Fields match TS VarPlayerType.decode / VarSharedType.decode
(scope/type/transmit/protect/clientcode for VARP; type for VARS).
Tests round-trip a synthetic blob through the DecodeType helper.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: Wire VARP_SMALL / VARP_LARGE server proto ops

**Files:**
- Modify: `pkg/io/protocol/game/server/prot.go`

- [ ] **Step 1: Add the two Op entries**

Locate the existing `OpMessageGame` / `OpLogout` entries and add nearby:

```go
OpVarpSmall = Op{Opcode: 150, PayloadSize: 3}
OpVarpLarge = Op{Opcode: 175, PayloadSize: 6}
```

Payload sizes match TS ServerGameProt (VARP_SMALL = 3 bytes: u16 id + i8 val; VARP_LARGE = 6 bytes: u16 id + i32 val).

- [ ] **Step 2: Build**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
```

Expected: clean build.

- [ ] **Step 3: Commit**

```bash
git add pkg/io/protocol/game/server/prot.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(proto/server): OpVarpSmall (150) + OpVarpLarge (175)

Server ops for per-player VARP sync. Fixed 3-byte payload (u16 id + i8)
and 6-byte payload (u16 id + i32). Used by S5b's SetVarp wire encoder.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: Extend `script.ActivePlayer` + `ScriptState.World` hook

**Files:**
- Modify: `pkg/script/active.go`
- Modify: `pkg/script/state.go`

- [ ] **Step 1: Add `Varp` / `SetVarp` to `ActivePlayer`**

```go
	// S5b additions.

	// Varp returns the player's current value for varp id. Returns 0 on OOB.
	Varp(id int) int32

	// SetVarp writes val to the player's varp storage. If the varp type
	// has transmit=true the write is also sent to the client via
	// VARP_SMALL / VARP_LARGE. OOB writes are dropped silently.
	SetVarp(id int, val int32)
```

- [ ] **Step 2: Add `WorldVars` interface + `World` field to `ScriptState`**

In `pkg/script/state.go`, before the `ScriptState` struct:

```go
// WorldVars is the minimal surface that pkg/script needs from the
// hosting world to resolve PUSH_VARS / POP_VARS. Decouples the VM
// from concrete server types.
type WorldVars interface {
	VarsInt(id int) int32
	SetVarsInt(id int, val int32)
	VarsString(id int) string
	SetVarsString(id int, val string)
}
```

Inside `ScriptState` struct, add after `Provider`:

```go
	// World is the world-scoped variable store. Callers set this
	// after Init if the script uses PUSH_VARS / POP_VARS.
	World WorldVars
```

- [ ] **Step 3: Update mockPlayer in `pkg/script/runner_test.go`**

Add to `mockPlayer`:

```go
	varps map[int]int32
```

And the methods:

```go
func (m *mockPlayer) Varp(id int) int32 {
	if m.varps == nil {
		return 0
	}
	return m.varps[id]
}
func (m *mockPlayer) SetVarp(id int, val int32) {
	if m.varps == nil {
		m.varps = make(map[int]int32)
	}
	m.varps[id] = val
}
```

- [ ] **Step 4: Build**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`
Expected: compile errors at `modules/world/player.go` for the unimplemented interface methods. Task 5 fixes these.

- [ ] **Step 5: Commit**

```bash
git add pkg/script/active.go pkg/script/state.go pkg/script/runner_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): S5b ActivePlayer.Varp/SetVarp + WorldVars interface

Extends ActivePlayer with varp read/write methods. Adds WorldVars
interface on ScriptState so PUSH_VARS / POP_VARS handlers can reach
the hosting world's var store without pkg/script importing modules/world.
mockPlayer fixture gains a varps map for unit tests.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: VAR handlers

**Files:**
- Create: `pkg/script/handlers_vars.go`
- Create: `pkg/script/handlers_vars_test.go`
- Modify: `pkg/script/handlers.go`

- [ ] **Step 1: Create `pkg/script/handlers_vars.go`**

```go
package script

import "errors"

// varOperandID returns the low 16 bits of the int operand at the
// current PC — that's the VAR id. The high bit (0x10000) flags the
// "secondary active player" (a.k.a. _activePlayer2) for future
// expansion; S5b ignores it.
func varOperandID(s *ScriptState) int {
	return int(uint32(s.Script.IntOperands[s.PC]) & 0xffff)
}

func handlePushVarp(s *ScriptState) error {
	if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
		return errors.New("PUSH_VARP: no active player")
	}
	s.PushInt(int(s.Self.Varp(varOperandID(s))))
	return nil
}

func handlePopVarp(s *ScriptState) error {
	if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
		return errors.New("POP_VARP: no active player")
	}
	val := int32(s.PopInt())
	s.Self.SetVarp(varOperandID(s), val)
	return nil
}

func handlePushVars(s *ScriptState) error {
	if s.World == nil {
		return errors.New("PUSH_VARS: no world")
	}
	// MVP always pushes int. Real string VARS are rare; dispatch by
	// VarSharedType.Type if we see them in telemetry.
	s.PushInt(int(s.World.VarsInt(varOperandID(s))))
	return nil
}

func handlePopVars(s *ScriptState) error {
	if s.World == nil {
		return errors.New("POP_VARS: no world")
	}
	val := int32(s.PopInt())
	s.World.SetVarsInt(varOperandID(s), val)
	return nil
}

// handlePushVarn is a stub until S6's active_npc lands.
func handlePushVarn(s *ScriptState) error {
	s.PushInt(0)
	return nil
}

// handlePopVarn is a stub until S6's active_npc lands.
func handlePopVarn(s *ScriptState) error {
	_ = s.PopInt()
	return nil
}
```

- [ ] **Step 2: Register 6 handlers in `pkg/script/handlers.go`**

Add after the S5a array/switch block:

```go
	// S5b: VAR ops.
	OpPushVarp:  handlePushVarp,
	OpPopVarp:   handlePopVarp,
	OpPushVars:  handlePushVars,
	OpPopVars:   handlePopVars,
	OpPushVarn:  handlePushVarn, // stub until S6
	OpPopVarn:   handlePopVarn,  // stub until S6
```

- [ ] **Step 3: Create `pkg/script/handlers_vars_test.go`**

```go
package script

import "testing"

// mockWorld implements WorldVars for tests.
type mockWorld struct {
	ints    map[int]int32
	strings map[int]string
}

func newMockWorld() *mockWorld {
	return &mockWorld{
		ints:    make(map[int]int32),
		strings: make(map[int]string),
	}
}

func (m *mockWorld) VarsInt(id int) int32          { return m.ints[id] }
func (m *mockWorld) SetVarsInt(id int, val int32)  { m.ints[id] = val }
func (m *mockWorld) VarsString(id int) string      { return m.strings[id] }
func (m *mockWorld) SetVarsString(id int, val string) { m.strings[id] = val }

func TestPushVarp(t *testing.T) {
	sf := &ScriptFile{
		Name:             "push_varp",
		Opcodes:          []Opcode{OpPushVarp, OpReturn},
		IntOperands:      []int32{0x42, 0}, // id = 0x42, no secondary
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	mp := &mockPlayer{varps: map[int]int32{0x42: 99}}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := state.PopInt(); got != 99 {
		t.Errorf("PushVarp: got %d, want 99", got)
	}
}

func TestPushVarpIgnoresSecondaryBit(t *testing.T) {
	// Operand high bit = secondary flag; S5b masks it off.
	sf := &ScriptFile{
		Name:             "push_varp_secondary",
		Opcodes:          []Opcode{OpPushVarp, OpReturn},
		IntOperands:      []int32{0x10042, 0}, // secondary=1, id=0x42
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	mp := &mockPlayer{varps: map[int]int32{0x42: 99}}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := state.PopInt(); got != 99 {
		t.Errorf("PushVarp(secondary masked): got %d, want 99", got)
	}
}

func TestPopVarpWritesToSelf(t *testing.T) {
	sf := &ScriptFile{
		Name: "pop_varp",
		Opcodes: []Opcode{
			OpPushConstantInt, // push 77
			OpPopVarp,         // write varp 5 = 77
			OpReturn,
		},
		IntOperands:      []int32{77, 5, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	mp := &mockPlayer{}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := mp.Varp(5); got != 77 {
		t.Errorf("mp.Varp(5): got %d, want 77", got)
	}
}

func TestPushVarpRequiresActivePlayer(t *testing.T) {
	sf := &ScriptFile{
		Name:             "push_varp_noself",
		Opcodes:          []Opcode{OpPushVarp, OpReturn},
		IntOperands:      []int32{0, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	state := Init(sf, nil, false, nil, nil)
	if err := Execute(state); err == nil {
		t.Fatal("Execute: want error")
	}
}

func TestPushVars(t *testing.T) {
	w := newMockWorld()
	w.SetVarsInt(7, 123)

	sf := &ScriptFile{
		Name:             "push_vars",
		Opcodes:          []Opcode{OpPushVars, OpReturn},
		IntOperands:      []int32{7, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	state := Init(sf, nil, false, nil, nil)
	state.World = w
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := state.PopInt(); got != 123 {
		t.Errorf("PushVars: got %d, want 123", got)
	}
}

func TestPopVarsWritesToWorld(t *testing.T) {
	w := newMockWorld()
	sf := &ScriptFile{
		Name: "pop_vars",
		Opcodes: []Opcode{
			OpPushConstantInt,
			OpPopVars,
			OpReturn,
		},
		IntOperands:      []int32{55, 3, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	state := Init(sf, nil, false, nil, nil)
	state.World = w
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := w.VarsInt(3); got != 55 {
		t.Errorf("w.VarsInt(3): got %d, want 55", got)
	}
}

func TestVarnStubs(t *testing.T) {
	sf := &ScriptFile{
		Name:             "varn_stubs",
		Opcodes:          []Opcode{OpPushVarn, OpReturn},
		IntOperands:      []int32{0, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	state := Init(sf, nil, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("PushVarn stub: %v", err)
	}
	if got := state.PopInt(); got != 0 {
		t.Errorf("PushVarn stub: got %d, want 0", got)
	}
}
```

- [ ] **Step 4: Run tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run 'TestPushVarp|TestPopVarp|TestPushVars|TestPopVars|TestVarnStubs' -v
```

Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/script/handlers_vars.go pkg/script/handlers_vars_test.go pkg/script/handlers.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): S5b VAR handlers (PUSH/POP_VARP/_VARS + VARN stubs)

PUSH_VARP / POP_VARP route through ActivePlayer.Varp/SetVarp.
PUSH_VARS / POP_VARS route through ScriptState.World (WorldVars
interface). PUSH_VARN returns 0 and POP_VARN drops; real VARN support
ships with S6's active_npc. Operand is masked with & 0xffff so the
secondary active_player bit is ignored for S5b.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: Player VARP storage + wire sync

**Files:**
- Modify: `modules/world/player.go`
- Modify: `modules/world/player_script.go`
- Create: `modules/world/player_varp.go`

- [ ] **Step 1: Add `varps` field to `Player`**

In `modules/world/player.go`, near the other session-state fields:

```go
	// varps holds the per-player int values for every registered VarPlayerType.
	// Allocated in processLogins after VarpTypeConfigs is available.
	varps []int32
```

- [ ] **Step 2: Add `Varp` / `SetVarp` impls in `modules/world/player_script.go`**

Append:

```go
// Varp implements script.ActivePlayer.Varp.
func (p *Player) Varp(id int) int32 {
	if id < 0 || id >= len(p.varps) {
		return 0
	}
	return p.varps[id]
}

// SetVarp implements script.ActivePlayer.SetVarp. Writes the server-
// side value then wire-sends via VARP_SMALL / VARP_LARGE if the varp
// type is transmit=true.
func (p *Player) SetVarp(id int, val int32) {
	if id < 0 || id >= len(p.varps) {
		return
	}
	p.varps[id] = val
	p.writeVarp(id, val)
}
```

- [ ] **Step 3: Create `modules/world/player_varp.go`**

```go
package world

import (
	"github.com/zsrv/goscape/pkg/io/packet"
	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
	"github.com/zsrv/goscape/pkg/objtype"
)

// writeVarp queues a VARP_SMALL or VARP_LARGE packet for the given
// varp change. Gated by VarPlayerType.Transmit — non-transmit varps
// stay server-only.
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

// varpTypeConfig returns the VarPlayerType for id, or nil if the
// server hasn't loaded configs or the id is out of range.
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

- [ ] **Step 4: Build (expects missing `s.varpTypes` — Task 6 adds)**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./modules/world/...
```

Expected: fails on `p.client.server.varpTypes` — Task 6 adds the field.

- [ ] **Step 5: Commit**

```bash
git add modules/world/player.go modules/world/player_script.go modules/world/player_varp.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): Player.varps storage + VARP wire encoder

Adds varps []int32 field (allocated on login once varpTypes is loaded).
SetVarp writes server state then calls writeVarp, which emits
VARP_SMALL if the value fits in int8 else VARP_LARGE. Only varps with
Transmit=true reach the wire.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: Server config load + WorldVars view + tick wiring

**Files:**
- Modify: `modules/world/server.go`
- Create: `modules/world/server_varp.go`
- Modify: `modules/world/tick.go`

- [ ] **Step 1: Add fields + loader calls in `server.go`**

Add to the `Server` struct near other objtype fields:

```go
	varpTypes   *objtype.VarpTypeConfigs
	varsTypes   *objtype.VarsTypeConfigs
	vars        []int32
	varsStrings []string
	worldVars   worldVarsView // cached value implementing script.WorldVars
```

In `NewServer`, after the existing `invTypes` load:

```go
varpTypes, err := objtype.LoadVarpTypes(cfg.CachePath)
if err != nil {
	return nil, fmt.Errorf("load varp types: %w", err)
}
varsTypes, err := objtype.LoadVarsTypes(cfg.CachePath)
if err != nil {
	return nil, fmt.Errorf("load vars types: %w", err)
}
s.varpTypes = varpTypes
s.varsTypes = varsTypes
s.vars = make([]int32, len(varsTypes.Configs))
s.varsStrings = make([]string, len(varsTypes.Configs))
s.worldVars = worldVarsView{s: s}
```

- [ ] **Step 2: Create `modules/world/server_varp.go`**

```go
package world

// worldVarsView adapts *Server to script.WorldVars. Kept value-typed so
// tests can construct it without a running server.
type worldVarsView struct {
	s *Server
}

func (w worldVarsView) VarsInt(id int) int32 {
	if w.s == nil || id < 0 || id >= len(w.s.vars) {
		return 0
	}
	return w.s.vars[id]
}

func (w worldVarsView) SetVarsInt(id int, val int32) {
	if w.s == nil || id < 0 || id >= len(w.s.vars) {
		return
	}
	w.s.vars[id] = val
}

func (w worldVarsView) VarsString(id int) string {
	if w.s == nil || id < 0 || id >= len(w.s.varsStrings) {
		return ""
	}
	return w.s.varsStrings[id]
}

func (w worldVarsView) SetVarsString(id int, val string) {
	if w.s == nil || id < 0 || id >= len(w.s.varsStrings) {
		return
	}
	w.s.varsStrings[id] = val
}
```

- [ ] **Step 3: Wire `state.World` in `runScript` (`modules/world/script.go`)**

```go
state := script.Init(sf, self, protect, intArgs, stringArgs)
state.Provider = s.scriptProvider
state.World = s.worldVars
```

- [ ] **Step 4: Allocate `p.varps` in `processLogins` (`modules/world/tick.go`)**

In the existing per-new-player block, near `p.buildArea = buildarea.New()`:

```go
if s.varpTypes != nil {
	p.varps = make([]int32, len(s.varpTypes.Configs))
}
```

- [ ] **Step 5: Full build + test**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
```

Expected: clean build and all tests pass.

- [ ] **Step 6: Commit**

```bash
git add modules/world/server.go modules/world/server_varp.go modules/world/script.go modules/world/tick.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): Server loads VARP/VARS types; wires WorldVars into script

NewServer now loads varp.dat + vars.dat from the cache path, sizes
world var storage from the config count, and exposes a worldVarsView
that satisfies script.WorldVars. runScript sets state.World so VARS
handlers resolve. processLogins allocates p.varps once configs are
available.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 7: End-to-end VARP wire sync test

**Files:**
- Modify: `modules/world/script_test.go`

- [ ] **Step 1: Add three VARP wire tests**

Append to the end of `modules/world/script_test.go`:

```go
// seedVarpTypes installs a minimal VarpTypeConfigs on s with a single
// varp (id 0, debugname "test", transmit as given) so player_varp.go
// wire logic has a config to consult.
func seedVarpTypes(s *Server, transmit bool) {
	t0 := objtype.NewVarPlayerType(0)
	t0.DebugName = "test"
	t0.Transmit = transmit
	s.varpTypes = &objtype.VarpTypeConfigs{
		ConfigNames: map[string]int{"test": 0},
		Configs:     []*objtype.VarPlayerType{t0},
	}
}

// popVarpScript builds: push_constant_int N, pop_varp 0, return.
func popVarpScript(value int32) *script.ScriptFile {
	return &script.ScriptFile{
		Name: "[popvarp,test]",
		Opcodes: []script.Opcode{
			script.OpPushConstantInt,
			script.OpPopVarp,
			script.OpReturn,
		},
		IntOperands:      []int32{value, 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
}

func TestVarpWireSyncSmall(t *testing.T) {
	s := newTestServer(t)
	seedVarpTypes(s, true)
	p, cc := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	p.varps = make([]int32, 1)

	received := drainConn(t, cc)
	s.runScript(popVarpScript(42), p, false, nil, nil)
	p.client.flushWrite()
	got := <-received

	// VARP_SMALL wire = opcode(1) + P2(id=0)(2) + P1(val=42)(1) = 4 bytes.
	if len(got) != 4 {
		t.Fatalf("VARP_SMALL wire: got %d bytes, want 4", len(got))
	}
	if got[1] != 0 || got[2] != 0 {
		t.Errorf("varp id bytes: got %v, want [0 0]", got[1:3])
	}
	if got[3] != 42 {
		t.Errorf("varp value byte: got %d, want 42", got[3])
	}
	if p.varps[0] != 42 {
		t.Errorf("server varps[0]: got %d, want 42", p.varps[0])
	}
}

func TestVarpWireSyncLarge(t *testing.T) {
	s := newTestServer(t)
	seedVarpTypes(s, true)
	p, cc := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	p.varps = make([]int32, 1)

	received := drainConn(t, cc)
	s.runScript(popVarpScript(10000), p, false, nil, nil)
	p.client.flushWrite()
	got := <-received

	// VARP_LARGE wire = opcode(1) + P2(id=0)(2) + P4(val=10000)(4) = 7 bytes.
	if len(got) != 7 {
		t.Fatalf("VARP_LARGE wire: got %d bytes, want 7", len(got))
	}
	if got[1] != 0 || got[2] != 0 {
		t.Errorf("varp id bytes: got %v, want [0 0]", got[1:3])
	}
	// P4(10000) big-endian = 0x00002710.
	want := []byte{0x00, 0x00, 0x27, 0x10}
	for i, b := range want {
		if got[3+i] != b {
			t.Errorf("varp value byte %d: got %#x, want %#x", i, got[3+i], b)
		}
	}
}

func TestVarpTransmitFalseNoWire(t *testing.T) {
	s := newTestServer(t)
	seedVarpTypes(s, false)
	p, cc := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	p.varps = make([]int32, 1)

	received := drainConn(t, cc)
	s.runScript(popVarpScript(42), p, false, nil, nil)
	p.client.flushWrite()
	got := <-received

	if len(got) != 0 {
		t.Errorf("transmit=false varp: got %d wire bytes, want 0", len(got))
	}
	if p.varps[0] != 42 {
		t.Errorf("server varps[0]: got %d, want 42 (server-side write must still happen)", p.varps[0])
	}
}
```

- [ ] **Step 2: Add `objtype` import at the top of the test file if missing**

Ensure `import "github.com/zsrv/goscape/pkg/objtype"` is present.

- [ ] **Step 3: Run tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestVarp -v
```

Expected: all 3 PASS.

- [ ] **Step 4: Full repo race + vet**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...
```

Expected: all green.

- [ ] **Step 5: Commit**

```bash
git add modules/world/script_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
test(world): end-to-end S5b VARP wire sync tests

Three scenarios: small-value VARP_SMALL (opcode 150), large-value
VARP_LARGE (opcode 175), and transmit=false (server-only, no wire
bytes). All use POP_VARP via runScript to exercise the full pipeline
from opcode dispatch through Player.SetVarp through writeVarp wire
encoder.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Self-Review Checklist

After completing all tasks:

- [ ] `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...` — clean build
- [ ] `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...` — all tests pass
- [ ] `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...` — no race warnings
- [ ] `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...` — no vet issues
- [ ] Handler count in `handlers.go` now reads 78 (72 after S5a + 6 S5b).
