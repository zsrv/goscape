# NAI-121 Implementation Plan — NPC varn / Player varp per-type default-seeding + STRING parallel arrays + opcode dispatch + V-PARTIAL investigation

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Bind the NAI-120 smoke residual ("It's not after you." gate-fire on Tutorial Island giant-rat attack) by porting full TS-fidelity per-type default-seeding for NPC varns and Player varps, including STRING parallel arrays, type-aware opcode dispatch, and `Protect` gate on POP_VARP. Then audit and (if findings warrant) fix the V-PARTIAL `%npc_combat_xp_multiplier` reads-as-zero residual.

**Architecture:** New `pkg/objtype/varntype.go` mirrors the existing `varptype.go` / `varstype.go` registry shape. NPC varns and Player varps grow parallel `[]string` arrays for STRING-typed slots. Per-type seed loops run inside `(*Server).resetEntityForRespawn` (covers initial spawn + respawn since `Server.addNpc` calls it) for NPC and inside `tick.go:105` player init block. Opcode handlers `PUSH_VARN`/`POP_VARN`/`PUSH_VARP`/`POP_VARP` consult `Configs.VarnType(id)` / `Configs.VarpType(id)` to dispatch on STRING vs INT and (POP_VARP only) gate Protect=true varps on `state.Protect`.

**Tech Stack:** Go 1.26+. All `go` invocations use the project's `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache` prefix.

**Spec:** `docs/superpowers/specs/2026-05-07-nai-121-npc-varn-default-seeding-design.md` (commit `88b6da1`).

---

## Bundle 1 — Smoke-binding fix (9 tasks)

Single subagent-driven-development cycle. Sonnet implementer per task; Sonnet code-reviewer between bundles per `superpowers_code_reviewer_model`. Each task is one logical commit.

### Task 1: Add missing ScriptVarType constants + create varntype registry + server wire-up

**Files:**
- Modify: `pkg/objtype/paramtype.go:29-49`
- Create: `pkg/objtype/varntype.go`
- Create: `pkg/objtype/varntype_test.go`
- Modify: `modules/world/server.go` (add field declaration + load wire-up)

- [ ] **Step 1.1: Add missing ScriptVarType constants**

Insert after line 48 of `pkg/objtype/paramtype.go` (inside the existing `const (...)` block, before the closing paren):

```go
ScriptVarTypeVarp      ScriptVarType = 86  // V
ScriptVarTypePlayerUid ScriptVarType = 112 // p
ScriptVarTypeNpcUid    ScriptVarType = 78  // N
ScriptVarTypeNpcStat   ScriptVarType = 254 // þ
ScriptVarTypeIdkit     ScriptVarType = 75  // K
ScriptVarTypeDbrow     ScriptVarType = 208 // Ð
```

These mirror TS `ScriptVarType.ts:7-26`. Used by the seed-loop tests in T3/T4 and the registry-decode path. `ScriptVarTypeAutoInt` already exists at line 31.

- [ ] **Step 1.2: Create `pkg/objtype/varntype.go`**

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
	return &VarNpcType{
		ConfigType: ConfigType{ID: id},
		Type:       ScriptVarTypeInt,
	}
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

	return &VarnTypeConfigs{
		ConfigNames: configNames,
		Configs:     configs,
	}, nil
}
```

- [ ] **Step 1.3: Create `pkg/objtype/varntype_test.go`**

```go
package objtype

import (
	"strings"
	"testing"

	"github.com/zsrv/goscape/pkg/io/packet"
)

// buildVarnPacket emits a server-side varn.dat-shaped packet with the
// given (type, name) tuples. type=0 means "default INT" and the type
// code is omitted from the per-config block.
func buildVarnPacket(entries []struct {
	Type int
	Name string
}) *packet.Packet {
	p := packet.New(nil)
	p.P2(uint16(len(entries)))
	for _, e := range entries {
		if e.Type != 0 {
			p.P1(1)
			p.P1(uint8(e.Type))
		}
		if e.Name != "" {
			p.P1(250)
			p.PJStrLF(e.Name)
		}
		p.P1(0) // terminator
	}
	return p
}

func TestParseVarnTypes_DefaultIsInt(t *testing.T) {
	p := buildVarnPacket([]struct {
		Type int
		Name string
	}{{Type: 0, Name: "default_int_var"}})
	cfg, err := parseVarnTypes(p)
	if err != nil {
		t.Fatalf("parseVarnTypes: %v", err)
	}
	if len(cfg.Configs) != 1 {
		t.Fatalf("Configs length: got %d, want 1", len(cfg.Configs))
	}
	if cfg.Configs[0].Type != ScriptVarTypeInt {
		t.Errorf("default Type: got %d, want ScriptVarTypeInt(%d)", cfg.Configs[0].Type, ScriptVarTypeInt)
	}
	if cfg.ConfigNames["default_int_var"] != 0 {
		t.Errorf("ConfigNames[default_int_var]: got %d, want 0", cfg.ConfigNames["default_int_var"])
	}
}

func TestParseVarnTypes_TypeCode1_SetsType(t *testing.T) {
	p := buildVarnPacket([]struct {
		Type int
		Name string
	}{{Type: int(ScriptVarTypePlayerUid), Name: "antimacro"}})
	cfg, err := parseVarnTypes(p)
	if err != nil {
		t.Fatalf("parseVarnTypes: %v", err)
	}
	if cfg.Configs[0].Type != ScriptVarTypePlayerUid {
		t.Errorf("Type: got %d, want ScriptVarTypePlayerUid(%d)", cfg.Configs[0].Type, ScriptVarTypePlayerUid)
	}
}

func TestParseVarnTypes_DebugNameCode250_SetsName(t *testing.T) {
	p := buildVarnPacket([]struct {
		Type int
		Name string
	}{{Type: 0, Name: "npc_macro_event_target"}})
	cfg, err := parseVarnTypes(p)
	if err != nil {
		t.Fatalf("parseVarnTypes: %v", err)
	}
	if cfg.Configs[0].DebugName != "npc_macro_event_target" {
		t.Errorf("DebugName: got %q, want %q", cfg.Configs[0].DebugName, "npc_macro_event_target")
	}
}

func TestParseVarnTypes_UnknownCode_ReturnsError(t *testing.T) {
	// Build packet with an unrecognized config code (99).
	p := packet.New(nil)
	p.P2(1) // count
	p.P1(99) // unrecognized
	p.P1(0)  // would be content, but Decode will error first
	_, err := parseVarnTypes(p)
	if err == nil {
		t.Fatal("parseVarnTypes: want error for unknown code")
	}
	if !strings.Contains(err.Error(), "unrecognized varn config code") {
		t.Errorf("error: got %q, want substring 'unrecognized varn config code'", err.Error())
	}
}

func TestParseVarnTypes_AntimacroFixture(t *testing.T) {
	// Mirrors Content/scripts/macro events/configs/antimacro.varn:
	// [npc_macro_event_target] type=player_uid
	p := buildVarnPacket([]struct {
		Type int
		Name string
	}{{Type: int(ScriptVarTypePlayerUid), Name: "npc_macro_event_target"}})
	cfg, err := parseVarnTypes(p)
	if err != nil {
		t.Fatalf("parseVarnTypes: %v", err)
	}
	if cfg.Configs[0].Type != ScriptVarTypePlayerUid {
		t.Errorf("Type: got %d, want ScriptVarTypePlayerUid", cfg.Configs[0].Type)
	}
	if cfg.ConfigNames["npc_macro_event_target"] != 0 {
		t.Errorf("ConfigNames lookup failed")
	}
}
```

- [ ] **Step 1.4: Run tests to verify they pass**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./pkg/objtype/ -run TestParseVarnTypes -v
```

Expected: 5 tests PASS.

- [ ] **Step 1.5: Wire `LoadVarnTypes` in `modules/world/server.go`**

Add field to `*Server` struct (next to `varpTypes` declaration; grep `varpTypes` to find the exact line in your version):

```go
varnTypes *objtype.VarnTypeConfigs
```

After the `LoadVarsTypes` block at lines 224-229, append:

```go
varnTypes, err := objtype.LoadVarnTypes(cfg.CachePath)
if err != nil {
	return nil, fmt.Errorf("load varn types: %w", err)
}
s.varnTypes = varnTypes
```

- [ ] **Step 1.6: Run full build to verify no compile errors**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
```

Expected: clean build. (Real-fixture `varn.dat` will exist in the cache dir; if absent in CI, document — but goscape's cache pipeline includes it.)

- [ ] **Step 1.7: Commit**

```
git add pkg/objtype/paramtype.go pkg/objtype/varntype.go pkg/objtype/varntype_test.go modules/world/server.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(objtype,world): NAI-121 T1 — VarNpcType registry + load wire-up

Adds pkg/objtype/varntype.go mirroring varptype.go / varstype.go
shape (code 1=type, 250=debugname). Wires LoadVarnTypes alongside
the existing varp/vars loaders in modules/world/server.go.

Also fills in the 6 missing ScriptVarType constants
(Varp=86, PlayerUid=112, NpcUid=78, NpcStat=254, Idkit=75, Dbrow=208)
to mirror the TS ScriptVarType enum and let downstream test fixtures
reference player_uid by name rather than numeric literal.

Foundation for T3/T4 per-type seed loops.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 2: Extend `Configs` interface with `VarpType` + `VarnType`

**Files:**
- Modify: `pkg/script/configs.go` (interface)
- Modify: `pkg/script/handlers_config_test.go:11-37` (mockConfigs)
- Modify: `modules/world/server_configs.go` (serverConfigsView)

- [ ] **Step 2.1: Add interface methods to `pkg/script/configs.go`**

After line 33 (`FindDbRowsStr(query string, packed int) []int`) and before the closing brace, add:

```go

	// VarpType returns the type and protect bit for a player-var id.
	// Out-of-range or unloaded id returns (ScriptVarTypeInt, false) —
	// degraded mode lets opcode dispatch fall through to int-side
	// (DEVIATION-NAI-121-D3; goscape defensive; TS check() throws).
	VarpType(id int) (typ objtype.ScriptVarType, protect bool)

	// VarnType returns the type for an NPC-var id. Out-of-range or
	// unloaded id returns ScriptVarTypeInt (DEVIATION-NAI-121-D3).
	VarnType(id int) objtype.ScriptVarType
```

- [ ] **Step 2.2: Update `mockConfigs` in `pkg/script/handlers_config_test.go`**

Add to the struct definition (after line 20 `spotAnimTypes` field):

```go
	varps map[int]*objtype.VarPlayerType
	varns map[int]*objtype.VarNpcType
```

After line 37 (last existing method), add:

```go
func (m *mockConfigs) VarpType(id int) (objtype.ScriptVarType, bool) {
	v, ok := m.varps[id]
	if !ok || v == nil {
		return objtype.ScriptVarTypeInt, false
	}
	return v.Type, v.Protect
}

func (m *mockConfigs) VarnType(id int) objtype.ScriptVarType {
	v, ok := m.varns[id]
	if !ok || v == nil {
		return objtype.ScriptVarTypeInt
	}
	return v.Type
}
```

- [ ] **Step 2.3: Add methods to `serverConfigsView` in `modules/world/server_configs.go`**

Append after the last existing method (FindDbRowsStr or similar — grep for the last `func (c serverConfigsView)` in the file):

```go
func (c serverConfigsView) VarpType(id int) (objtype.ScriptVarType, bool) {
	if c.s == nil || c.s.varpTypes == nil {
		return objtype.ScriptVarTypeInt, false
	}
	if id < 0 || id >= len(c.s.varpTypes.Configs) {
		return objtype.ScriptVarTypeInt, false
	}
	cfg := c.s.varpTypes.Configs[id]
	if cfg == nil {
		return objtype.ScriptVarTypeInt, false
	}
	return cfg.Type, cfg.Protect
}

func (c serverConfigsView) VarnType(id int) objtype.ScriptVarType {
	if c.s == nil || c.s.varnTypes == nil {
		return objtype.ScriptVarTypeInt
	}
	if id < 0 || id >= len(c.s.varnTypes.Configs) {
		return objtype.ScriptVarTypeInt
	}
	cfg := c.s.varnTypes.Configs[id]
	if cfg == nil {
		return objtype.ScriptVarTypeInt
	}
	return cfg.Type
}
```

- [ ] **Step 2.4: Verify build is green**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./pkg/script/ -run TestConfigs
```

Expected: clean build. (Pre-existing tests still green; the new methods aren't called yet.)

- [ ] **Step 2.5: Commit**

```
git add pkg/script/configs.go pkg/script/handlers_config_test.go modules/world/server_configs.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script,world): NAI-121 T2 — Configs interface VarpType/VarnType

Extends the script.Configs interface with VarpType(id) and VarnType(id)
for opcode-dispatch-by-type lookup. mockConfigs and serverConfigsView
both implement; degraded mode (nil registry / OOB id) returns
(ScriptVarTypeInt, false) per DEVIATION-NAI-121-D3.

Foundation for T7/T8 opcode dispatch.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 3: NPC varnsString field + per-type seed loop in resetEntityForRespawn

**Files:**
- Modify: `modules/world/npc.go` (add varnsString field; doc-comment on varns)
- Modify: `modules/world/npc_registry.go::resetEntityForRespawn` (append seed loop)
- Create or extend: `modules/world/npc_registry_test.go` (new tests in this file if it exists; grep first)

- [ ] **Step 3.1: Add `varnsString []string` field on `*Npc`**

Locate the `varns []int32` declaration in `modules/world/npc.go` (around line 35). Replace the doc-comment and add the parallel field:

```go
// varns is per-NPC int-typed vars; sized to len(server.varnTypes.Configs)
// at first resetEntityForRespawn (called inside Server.addNpc). Nil for
// raw &Npc{} test literals; reads via NpcVarN return 0 defensively. Per-
// type seeded by resetEntityForRespawn (TS Npc.ts:296-303) — INT→0,
// non-INT-non-STRING→-1.
varns []int32
// varnsString is the parallel STRING-typed slot array. Sized identically
// to varns; nil for raw &Npc{} test literals; reads via NpcVarNString
// return "" defensively. Mirrors TS Npc.varsString.
varnsString []string
```

- [ ] **Step 3.2: Append per-type seed loop to `resetEntityForRespawn`**

Locate the end of `resetEntityForRespawn` body in `modules/world/npc_registry.go` (after the hunt-field reset, before the closing brace; around line 144). Insert before the closing brace:

```go
// TS Npc.resetEntity(true) varn re-seed loop (Npc.ts:296-306).
// Per-type defaults: STRING → "" (TS uses undefined; goscape uses
// zero-value string per DEVIATION-NAI-121-D2); INT → 0; everything
// else → -1. Defensive (DEVIATION-NAI-121-D3): if s.varnTypes is nil
// (test path) the loop is a no-op and reads fall back to slice
// defaults. (goscape defensive; TS skips this check.)
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

- [ ] **Step 3.3: Write failing tests**

Add to `modules/world/npc_registry_test.go` (create the file if absent — check with `ls modules/world/npc_registry_test.go`):

```go
package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
)

// seedVarnTypes installs a minimal VarnTypeConfigs on s with the given
// (type, name) tuples for resetEntityForRespawn seed-loop tests.
func seedVarnTypes(s *Server, entries []struct {
	Type objtype.ScriptVarType
	Name string
}) {
	configs := make([]*objtype.VarNpcType, len(entries))
	configNames := make(map[string]int, len(entries))
	for i, e := range entries {
		c := objtype.NewVarNpcType(i)
		c.Type = e.Type
		c.DebugName = e.Name
		configs[i] = c
		configNames[e.Name] = i
	}
	s.varnTypes = &objtype.VarnTypeConfigs{ConfigNames: configNames, Configs: configs}
}

func TestResetEntityForRespawn_SeedsIntToZero(t *testing.T) {
	s := newTestServer(t)
	seedVarnTypes(s, []struct {
		Type objtype.ScriptVarType
		Name string
	}{
		{Type: objtype.ScriptVarTypeInt, Name: "int_var"},
	})

	npcType := &objtype.NpcType{ConfigType: objtype.ConfigType{ID: 0}}
	n := NewNpc(1, 0, 100, 100, 0, npcType)
	n.varns = []int32{42} // pre-write to verify reset overwrites
	s.resetEntityForRespawn(n)

	if got := n.NpcVarN(0); got != 0 {
		t.Errorf("INT-typed varn after reset: got %d, want 0", got)
	}
}

func TestResetEntityForRespawn_SeedsPlayerUidToMinusOne(t *testing.T) {
	s := newTestServer(t)
	seedVarnTypes(s, []struct {
		Type objtype.ScriptVarType
		Name string
	}{
		{Type: objtype.ScriptVarTypePlayerUid, Name: "npc_macro_event_target"},
	})

	npcType := &objtype.NpcType{ConfigType: objtype.ConfigType{ID: 0}}
	n := NewNpc(1, 0, 100, 100, 0, npcType)
	s.resetEntityForRespawn(n)

	if got := n.NpcVarN(0); got != -1 {
		t.Errorf("player_uid-typed varn: got %d, want -1", got)
	}
}

func TestResetEntityForRespawn_SeedsCoordToMinusOne(t *testing.T) {
	s := newTestServer(t)
	seedVarnTypes(s, []struct {
		Type objtype.ScriptVarType
		Name string
	}{
		{Type: objtype.ScriptVarTypeCoord, Name: "npc_start_coord"},
	})

	npcType := &objtype.NpcType{ConfigType: objtype.ConfigType{ID: 0}}
	n := NewNpc(1, 0, 100, 100, 0, npcType)
	s.resetEntityForRespawn(n)

	if got := n.NpcVarN(0); got != -1 {
		t.Errorf("coord-typed varn: got %d, want -1", got)
	}
}

func TestResetEntityForRespawn_SeedsNpcUidToMinusOne(t *testing.T) {
	s := newTestServer(t)
	seedVarnTypes(s, []struct {
		Type objtype.ScriptVarType
		Name string
	}{
		{Type: objtype.ScriptVarTypeNpcUid, Name: "rantz_attacking_chompy"},
	})

	npcType := &objtype.NpcType{ConfigType: objtype.ConfigType{ID: 0}}
	n := NewNpc(1, 0, 100, 100, 0, npcType)
	s.resetEntityForRespawn(n)

	if got := n.NpcVarN(0); got != -1 {
		t.Errorf("npc_uid-typed varn: got %d, want -1", got)
	}
}

func TestResetEntityForRespawn_SeedsStringToEmpty(t *testing.T) {
	s := newTestServer(t)
	seedVarnTypes(s, []struct {
		Type objtype.ScriptVarType
		Name string
	}{
		{Type: objtype.ScriptVarTypeString, Name: "string_var"},
	})

	npcType := &objtype.NpcType{ConfigType: objtype.ConfigType{ID: 0}}
	n := NewNpc(1, 0, 100, 100, 0, npcType)
	// Note: NpcVarNString accessor lands in T5; test reads field
	// directly here. After T5, this can switch to NpcVarNString.
	s.resetEntityForRespawn(n)

	if len(n.varnsString) != 1 {
		t.Fatalf("varnsString length: got %d, want 1", len(n.varnsString))
	}
	if got := n.varnsString[0]; got != "" {
		t.Errorf("string-typed varn: got %q, want \"\"", got)
	}
}

func TestResetEntityForRespawn_NilVarnTypes_NoOp(t *testing.T) {
	s := newTestServer(t)
	// Do NOT seed varnTypes; leave nil.

	npcType := &objtype.NpcType{ConfigType: objtype.ConfigType{ID: 0}}
	n := NewNpc(1, 0, 100, 100, 0, npcType)
	s.resetEntityForRespawn(n)

	if n.varns != nil {
		t.Errorf("varns: got non-nil slice, want nil (defensive no-op)")
	}
}

func TestAddNpc_FreshSpawn_PlayerUidVarnReadsMinusOne(t *testing.T) {
	// THE smoke-bind unit pin. After Server.addNpc, a fresh-spawn NPC's
	// player_uid-typed varn must read as -1 so the player_combat.rs2
	// "It's not after you." gate skips.
	s := newTestServer(t)
	seedVarnTypes(s, []struct {
		Type objtype.ScriptVarType
		Name string
	}{
		{Type: objtype.ScriptVarTypePlayerUid, Name: "npc_macro_event_target"},
	})

	npcType := &objtype.NpcType{ConfigType: objtype.ConfigType{ID: 0}}
	n := NewNpc(1, 0, 100, 100, 0, npcType)
	if err := s.addNpc(n, -1); err != nil {
		t.Fatalf("addNpc: %v", err)
	}

	if got := n.NpcVarN(0); got != -1 {
		t.Errorf("smoke-bind: %%npc_macro_event_target on fresh-spawn NPC: got %d, want -1", got)
	}
}
```

> Note: `newTestServer(t)` is the existing helper used across `modules/world/*_test.go`. If it isn't directly callable from `npc_registry_test.go` for some reason, grep for `func newTestServer` to find the constructor pattern and copy/adapt.

- [ ] **Step 3.4: Run failing tests to verify**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./modules/world/ -run 'TestResetEntityForRespawn|TestAddNpc_FreshSpawn_PlayerUid' -v
```

Expected: depending on order — pre-step-3.1/3.2 these would fail; post-3.1/3.2 they should pass. Expected: ALL PASS.

- [ ] **Step 3.5: Run full goscape tests to ensure no regressions**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./modules/world/...
```

Expected: ALL PASS. Pay particular attention to `npc_hunt_test.go:679, 746` (existing `n.SetNpcVarN(0, ...)` sites — verify they still pass since seedVarnTypes wasn't called by those tests, so the slice stays nil and `SetNpcVarN(0, ...)` falls through the `id >= len(varns)` defensive guard. Their assertions on subsequent reads should still work via `NpcVarN(0)` returning 0).

If `npc_hunt_test.go:679, 746` regress: those tests need a `seedVarnTypes(s, [{Type: Int, Name: "test"}])` call before the SetNpcVarN. Diagnose first; the lazy-grow vs sized-slice change may force the test to seed the registry.

- [ ] **Step 3.6: Commit**

```
git add modules/world/npc.go modules/world/npc_registry.go modules/world/npc_registry_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-121 T3 — NPC varn per-type seed loop in resetEntityForRespawn

Adds *Npc.varnsString []string parallel to varns. resetEntityForRespawn
appends the TS Npc.ts:296-306 seed loop: STRING→"", INT→0, else→-1.
Slice sized to len(server.varnTypes.Configs); resize on first reset
covers both fresh-spawn and respawn paths.

7 unit tests pin per-type defaults (INT/PlayerUid/Coord/NpcUid/String),
nil-varnTypes no-op (DEVIATION-D3), and the smoke-bind end-to-end
addNpc → NpcVarN(player_uid_id) == -1.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 4: Player varpsString field + per-type seed loop in tick.go:105

**Files:**
- Modify: `modules/world/player.go` (add varpsString field; doc-comment on varps)
- Modify: `modules/world/tick.go:105` (replace single-line varps allocation with seed loop)
- Create or extend: `modules/world/tick_test.go` (player init tests; grep first for the existing file)

- [ ] **Step 4.1: Add `varpsString []string` field on `*Player`**

Locate `varps []int32` in `modules/world/player.go` (line 157). Replace the doc-comment and add the parallel field:

```go
// varps holds per-player int-typed vars; sized to
// len(server.varpTypes.Configs) at login (tick.go:105). Per-type
// seeded — INT→0, non-INT-non-STRING→-1 (TS Player.ts:418-432).
varps []int32
// varpsString is the parallel STRING-typed slot array. Sized identically
// to varps. Mirrors TS Player.varsString.
varpsString []string
```

- [ ] **Step 4.2: Update `tick.go:105` block**

Locate the existing single-line allocation (line 105):

```go
p.varps = make([]int32, len(s.varpTypes.Configs))
```

Replace with:

```go
// Per-type seed loop — TS Player.ts:418-432.
//   STRING → varpsString[i] = "" (already)
//   INT    → varps[i] = 0        (already)
//   else   → varps[i] = -1
p.varps = make([]int32, len(s.varpTypes.Configs))
p.varpsString = make([]string, len(s.varpTypes.Configs))
for i, vt := range s.varpTypes.Configs {
	switch vt.Type {
	case objtype.ScriptVarTypeString:
		// varpsString[i] = "" already (Go zero-value)
	case objtype.ScriptVarTypeInt:
		// varps[i] = 0 already (Go zero-value)
	default:
		p.varps[i] = -1
	}
}
```

> Note: confirm the import for `objtype` is present at the top of `tick.go`. If not, add `"github.com/zsrv/goscape/pkg/objtype"`.

- [ ] **Step 4.3: Write tests**

Either extend an existing `modules/world/tick_test.go` (grep first) or add a new section. If the file exists, append; if not, create:

```go
package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
)

// seedVarpTypesByType installs a VarpTypeConfigs with the given
// (type, name, protect) entries on s, for tick-init seed-loop tests.
// Distinct from seedVarpTypes(s, transmit) at script_test.go:368, which
// only handles a single transmit-flag varp.
func seedVarpTypesByType(s *Server, entries []struct {
	Type    objtype.ScriptVarType
	Name    string
	Protect bool
}) {
	configs := make([]*objtype.VarPlayerType, len(entries))
	configNames := make(map[string]int, len(entries))
	for i, e := range entries {
		c := objtype.NewVarPlayerType(i)
		c.Type = e.Type
		c.DebugName = e.Name
		c.Protect = e.Protect
		configs[i] = c
		configNames[e.Name] = i
	}
	s.varpTypes = &objtype.VarpTypeConfigs{ConfigNames: configNames, Configs: configs}
}

func TestPlayerInit_VarpsSeededByType_IntZero(t *testing.T) {
	s := newTestServer(t)
	seedVarpTypesByType(s, []struct {
		Type    objtype.ScriptVarType
		Name    string
		Protect bool
	}{
		{Type: objtype.ScriptVarTypeInt, Name: "int_var", Protect: false},
	})
	p, _ := newTestPlayer(t)
	p.client.server = s

	// Drive the init block at tick.go:105. The exact entry-point depends
	// on the existing testing pattern — grep for how other tests trigger
	// it (e.g., a login helper or direct call).
	s.initPlayerVarps(p) // OR: inline the seed loop in the test if no helper exists.

	if got := p.Varp(0); got != 0 {
		t.Errorf("INT varp: got %d, want 0", got)
	}
}

func TestPlayerInit_VarpsSeededByType_NpcUidMinusOne(t *testing.T) {
	s := newTestServer(t)
	seedVarpTypesByType(s, []struct {
		Type    objtype.ScriptVarType
		Name    string
		Protect bool
	}{
		{Type: objtype.ScriptVarTypeNpcUid, Name: "npc_uid_var", Protect: false},
	})
	p, _ := newTestPlayer(t)
	p.client.server = s

	s.initPlayerVarps(p)

	if got := p.Varp(0); got != -1 {
		t.Errorf("npc_uid varp: got %d, want -1", got)
	}
}

func TestPlayerInit_VarpsSeededByType_StringEmpty(t *testing.T) {
	s := newTestServer(t)
	seedVarpTypesByType(s, []struct {
		Type    objtype.ScriptVarType
		Name    string
		Protect bool
	}{
		{Type: objtype.ScriptVarTypeString, Name: "string_var", Protect: false},
	})
	p, _ := newTestPlayer(t)
	p.client.server = s

	s.initPlayerVarps(p)

	if len(p.varpsString) != 1 {
		t.Fatalf("varpsString length: got %d, want 1", len(p.varpsString))
	}
	if got := p.varpsString[0]; got != "" {
		t.Errorf("string varp: got %q, want \"\"", got)
	}
}

func TestPlayerInit_VarpsLengthMatchesRegistry(t *testing.T) {
	s := newTestServer(t)
	seedVarpTypesByType(s, []struct {
		Type    objtype.ScriptVarType
		Name    string
		Protect bool
	}{
		{Type: objtype.ScriptVarTypeInt, Name: "a", Protect: false},
		{Type: objtype.ScriptVarTypePlayerUid, Name: "b", Protect: false},
		{Type: objtype.ScriptVarTypeString, Name: "c", Protect: false},
	})
	p, _ := newTestPlayer(t)
	p.client.server = s

	s.initPlayerVarps(p)

	if len(p.varps) != 3 || len(p.varpsString) != 3 {
		t.Errorf("lengths: varps=%d varpsString=%d, want both 3", len(p.varps), len(p.varpsString))
	}
}
```

> Note: `s.initPlayerVarps(p)` is a hypothetical helper. If `tick.go:105` is buried inside a larger function, factor it out into a method `(s *Server) initPlayerVarps(p *Player)` for testability. If that's too invasive, use a different test entry-point — e.g., call the existing tick-init function with a minimal player. The implementer should choose the lightest path that lets the assertions pass.

- [ ] **Step 4.4: Verify tests pass**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./modules/world/ -run TestPlayerInit_Varps -v
```

Expected: ALL PASS.

- [ ] **Step 4.5: Run full module tests**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./modules/world/...
```

Expected: ALL PASS. The existing `seedVarpTypes(s, transmit)` helper at `script_test.go:368` only sets a single varp (id 0, INT), so nothing should break.

- [ ] **Step 4.6: Commit**

```
git add modules/world/player.go modules/world/tick.go modules/world/tick_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-121 T4 — Player varp per-type seed loop at tick init

Adds *Player.varpsString []string parallel to varps. tick.go:105
replaces zero-init varps allocation with TS Player.ts:418-432 per-type
seed: STRING→"", INT→0, else→-1.

4 unit tests pin per-type defaults across INT, NpcUid, String, plus
length parity. seedVarpTypesByType test helper introduced (distinct
from existing seedVarpTypes which targets a single transmit-flag varp).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 5: NpcVarNString / SetNpcVarNString accessors + ActiveNpc interface + mockNpc

**Files:**
- Modify: `pkg/script/active.go` (extend ActiveNpc interface)
- Modify: `pkg/script/handlers_npc_test.go:199-301` (extend mockNpc)
- Modify: `modules/world/npc_script.go` (add accessor methods on *Npc)

- [ ] **Step 5.1: Extend `ActiveNpc` interface**

Locate the `ActiveNpc` interface in `pkg/script/active.go` (around line 600+, search for `NpcVarN`). After the `SetNpcVarN(id int, val int32)` line, add:

```go
	// NpcVarNString reads the per-NPC STRING-typed var at id. Returns
	// "" defensively for OOB or never-written ids. Mirrors TS
	// Npc.getVar dispatched on STRING type.
	NpcVarNString(id int) string

	// SetNpcVarNString writes the per-NPC STRING-typed var at id. OOB
	// silently dropped (slice sized to varnTypes.Configs at spawn).
	SetNpcVarNString(id int, val string)
```

- [ ] **Step 5.2: Add accessor methods on `*Npc`**

Append to `modules/world/npc_script.go` (after the existing `SetNpcVarN` at line 90-103):

```go
// NpcVarNString implements script.ActiveNpc.NpcVarNString. Returns the
// STRING-typed per-NPC var at id, or "" on OOB / unsized slice.
func (n *Npc) NpcVarNString(id int) string {
	if id < 0 || id >= len(n.varnsString) {
		return ""
	}
	return n.varnsString[id]
}

// SetNpcVarNString implements script.ActiveNpc.SetNpcVarNString. OOB
// silently dropped (slice sized to varnTypes.Configs at spawn).
func (n *Npc) SetNpcVarNString(id int, val string) {
	if id < 0 || id >= len(n.varnsString) {
		return
	}
	n.varnsString[id] = val
}
```

- [ ] **Step 5.3: Extend `mockNpc` in `pkg/script/handlers_npc_test.go`**

Locate the `mockNpc` struct definition (line 199). After the `varns map[int]int32` field (line 213), add:

```go
	varnsString map[int]string
```

After the `SetNpcVarN` method (line 297-301), add:

```go
func (m *mockNpc) NpcVarNString(id int) string {
	if m.varnsString == nil {
		return ""
	}
	return m.varnsString[id]
}

func (m *mockNpc) SetNpcVarNString(id int, val string) {
	if m.varnsString == nil {
		m.varnsString = make(map[int]string)
	}
	m.varnsString[id] = val
}
```

- [ ] **Step 5.4: Update `mockActiveNpc` stub in `pkg/script/handlers_player_test.go`**

There's a smaller stub at `pkg/script/handlers_player_test.go:47-48` (`type mockActiveNpc`) that exists alongside mockNpc. Grep `func (m \*mockActiveNpc)` and add the two new methods returning the zero values:

```go
func (m *mockActiveNpc) NpcVarNString(id int) string         { return "" }
func (m *mockActiveNpc) SetNpcVarNString(id int, val string) {}
```

- [ ] **Step 5.5: Verify build is green**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./pkg/script/ ./modules/world/
```

Expected: clean build, all existing tests pass. (No new test added; T7 will exercise the path.)

- [ ] **Step 5.6: Commit**

```
git add pkg/script/active.go pkg/script/handlers_npc_test.go pkg/script/handlers_player_test.go modules/world/npc_script.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script,world): NAI-121 T5 — NpcVarNString/SetNpcVarNString accessors

Extends script.ActiveNpc with NpcVarNString(id) / SetNpcVarNString(id, val)
to mirror the TS Npc.getVar/setVar STRING-typed dispatch. *Npc backs both
methods with defensive OOB guards reading n.varnsString. mockNpc and
mockActiveNpc stubs updated.

Foundation for T7 PUSH_VARN/POP_VARN type-aware dispatch.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 6: VarpString / SetVarpString accessors + ActivePlayer interface + mockPlayer

**Files:**
- Modify: `pkg/script/active.go` (extend ActivePlayer interface)
- Modify: `pkg/script/runner_test.go:99-394` (extend mockPlayer)
- Modify: `modules/world/player_script.go` (add accessor methods on *Player)

- [ ] **Step 6.1: Extend `ActivePlayer` interface**

Locate the `ActivePlayer` interface in `pkg/script/active.go` (around line 79-84). After `SetVarp(id int, val int32)`, add:

```go
	// VarpString reads the per-player STRING-typed var at id. Returns
	// "" defensively for OOB or never-written ids. Mirrors TS
	// Player.getVar dispatched on STRING type.
	VarpString(id int) string

	// SetVarpString writes the per-player STRING-typed var at id. OOB
	// silently dropped. No wire-send (this protocol revision has no
	// varp_string opcode); server-side state only.
	SetVarpString(id int, val string)
```

- [ ] **Step 6.2: Add accessor methods on `*Player`**

Append to `modules/world/player_script.go` (after the existing `SetVarp` at line 317-323):

```go
// VarpString implements script.ActivePlayer.VarpString. Returns the
// STRING-typed per-player var at id, or "" on OOB / unsized slice.
func (p *Player) VarpString(id int) string {
	if id < 0 || id >= len(p.varpsString) {
		return ""
	}
	return p.varpsString[id]
}

// SetVarpString implements script.ActivePlayer.SetVarpString. OOB
// silently dropped (slice sized to varpTypes.Configs at login). No
// wire-send: this protocol revision has no varp_string opcode.
func (p *Player) SetVarpString(id int, val string) {
	if id < 0 || id >= len(p.varpsString) {
		return
	}
	p.varpsString[id] = val
}
```

- [ ] **Step 6.3: Extend `mockPlayer` in `pkg/script/runner_test.go`**

Locate the `mockPlayer` struct (line 99). After the `varps map[int]int32` field, add:

```go
	varpsString map[int]string
```

After the existing `SetVarp` method at line 389-394, add:

```go
func (m *mockPlayer) VarpString(id int) string {
	if m.varpsString == nil {
		return ""
	}
	return m.varpsString[id]
}
func (m *mockPlayer) SetVarpString(id int, val string) {
	if m.varpsString == nil {
		m.varpsString = make(map[int]string)
	}
	m.varpsString[id] = val
}
```

- [ ] **Step 6.4: Verify build + tests**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./pkg/script/ ./modules/world/
```

Expected: clean build, all existing tests pass.

- [ ] **Step 6.5: Commit**

```
git add pkg/script/active.go pkg/script/runner_test.go modules/world/player_script.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script,world): NAI-121 T6 — VarpString/SetVarpString accessors

Extends script.ActivePlayer with VarpString(id) / SetVarpString(id, val)
to mirror the TS Player.getVar/setVar STRING-typed dispatch. *Player
backs both methods with defensive OOB guards reading p.varpsString.
No wire-send on SetVarpString — this protocol revision has no
varp_string opcode (server-side state only). mockPlayer updated.

Foundation for T8 PUSH_VARP/POP_VARP type-aware dispatch.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 7: PUSH_VARN / POP_VARN type-aware dispatch

**Files:**
- Modify: `pkg/script/handlers_vars.go` (rewrite handlePushVarn / handlePopVarn)
- Modify: `pkg/script/handlers_vars_test.go` (extend with STRING + smoke-bind tests)

- [ ] **Step 7.1: Rewrite `handlePushVarn` and `handlePopVarn`**

Replace the existing handlers in `pkg/script/handlers_vars.go:52-69` with:

```go
// handlePushVarn reads per-NPC variable `id` from the active NPC and
// pushes it. Dispatches on Configs.VarnType(id): STRING → pushString,
// else → pushInt. Returns an error if no ActiveNpc is bound. High
// operand bit (secondary-NPC flag) is ignored — same convention as VARP.
func handlePushVarn(s *ScriptState) error {
	if s.ActiveNpc == nil {
		return errors.New("PUSH_VARN: no active npc")
	}
	id := varOperandID(s)
	typ := s.varnType(id)
	if typ == objtype.ScriptVarTypeString {
		s.PushString(s.ActiveNpc.NpcVarNString(id))
	} else {
		s.PushInt(int(s.ActiveNpc.NpcVarN(id)))
	}
	return nil
}

// handlePopVarn pops the top of the appropriate stack and writes it to
// per-NPC variable `id` on the active NPC. Dispatches on
// Configs.VarnType(id): STRING → popString, else → popInt. Returns an
// error if no ActiveNpc is bound.
func handlePopVarn(s *ScriptState) error {
	if s.ActiveNpc == nil {
		return errors.New("POP_VARN: no active npc")
	}
	id := varOperandID(s)
	typ := s.varnType(id)
	if typ == objtype.ScriptVarTypeString {
		s.ActiveNpc.SetNpcVarNString(id, s.PopString())
	} else {
		s.ActiveNpc.SetNpcVarN(id, int32(s.PopInt()))
	}
	return nil
}
```

Add the `objtype` import to the file's import block:

```go
"github.com/zsrv/goscape/pkg/objtype"
```

- [ ] **Step 7.2: Add `varnType` helper on `ScriptState`**

In `pkg/script/handlers_vars.go`, after the existing `varOperandID` helper, add:

```go
// varnType returns the type of NPC-var id from Configs, falling back
// to ScriptVarTypeInt when Configs is nil (test paths). Mirrors
// DEVIATION-NAI-121-D3.
func (s *ScriptState) varnType(id int) objtype.ScriptVarType {
	if s.Configs == nil {
		return objtype.ScriptVarTypeInt
	}
	return s.Configs.VarnType(id)
}
```

- [ ] **Step 7.3: Write tests for STRING + smoke-bind**

Append to `pkg/script/handlers_vars_test.go`:

```go
func TestPushVarn_StringType_PushesString(t *testing.T) {
	sf := &ScriptFile{
		Name:             "push_varn_str",
		Opcodes:          []Opcode{OpPushVarn, OpReturn},
		IntOperands:      []int32{3, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	npc := &mockNpc{varnsString: map[int]string{3: "hello"}}
	state := Init(sf, nil, false, nil, nil)
	state.ActiveNpc = npc
	state.Configs = &mockConfigs{
		varns: map[int]*objtype.VarNpcType{
			3: {Type: objtype.ScriptVarTypeString},
		},
	}
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := state.PopString(); got != "hello" {
		t.Errorf("PushVarn(STRING): got %q, want %q", got, "hello")
	}
}

func TestPopVarn_StringType_PopsString(t *testing.T) {
	sf := &ScriptFile{
		Name: "pop_varn_str",
		Opcodes: []Opcode{
			OpPushConstantString, // push "abc"
			OpPopVarn,            // write varn 7 = "abc"
			OpReturn,
		},
		IntOperands:      []int32{0, 7, 0},
		StringOperands:   []string{"abc", "", ""},
		InstructionCount: 3,
	}
	npc := &mockNpc{}
	state := Init(sf, nil, false, nil, nil)
	state.ActiveNpc = npc
	state.Configs = &mockConfigs{
		varns: map[int]*objtype.VarNpcType{
			7: {Type: objtype.ScriptVarTypeString},
		},
	}
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := npc.NpcVarNString(7); got != "abc" {
		t.Errorf("npc.varnsString[7]: got %q, want %q", got, "abc")
	}
}

func TestPushVarn_PlayerUidDefault_PushesMinusOne(t *testing.T) {
	// Smoke-bind unit pin. A fresh-spawn NPC's player_uid varn reads -1
	// (set by resetEntityForRespawn in T3) — combat gate skips.
	// Here we mock the seeded state directly: mockNpc.varns[N] = -1.
	sf := &ScriptFile{
		Name:             "push_varn_pid",
		Opcodes:          []Opcode{OpPushVarn, OpReturn},
		IntOperands:      []int32{0, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	npc := &mockNpc{varns: map[int]int32{0: -1}}
	state := Init(sf, nil, false, nil, nil)
	state.ActiveNpc = npc
	state.Configs = &mockConfigs{
		varns: map[int]*objtype.VarNpcType{
			0: {Type: objtype.ScriptVarTypePlayerUid},
		},
	}
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := state.PopInt(); got != -1 {
		t.Errorf("PushVarn(PLAYER_UID, default-seeded -1): got %d, want -1", got)
	}
}

func TestPushVarn_NilConfigsFallsBackToInt(t *testing.T) {
	// DEVIATION-NAI-121-D3 pin: nil Configs → int dispatch.
	sf := &ScriptFile{
		Name:             "push_varn_nilconfigs",
		Opcodes:          []Opcode{OpPushVarn, OpReturn},
		IntOperands:      []int32{0, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	npc := &mockNpc{varns: map[int]int32{0: 99}}
	state := Init(sf, nil, false, nil, nil)
	state.ActiveNpc = npc
	state.Configs = nil // explicit
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := state.PopInt(); got != 99 {
		t.Errorf("PushVarn(nil Configs fallback): got %d, want 99", got)
	}
}
```

> Note: The existing `TestPushVarnReadsActiveNpc` (line 175) and `TestPopVarnWritesActiveNpc` (line 214) and `TestVarnRequireActiveNpc` (line 237) and `TestPushVarnIgnoresSecondaryBit` (line 194) MUST still pass. They don't set `state.Configs`, so the new `varnType` helper falls through to `ScriptVarTypeInt` → existing int-side dispatch. **Verify** by running them post-change.

- [ ] **Step 7.4: Verify all tests pass**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./pkg/script/ -run 'TestPushVarn|TestPopVarn|TestVarn' -v
```

Expected: All existing + 4 new tests PASS.

- [ ] **Step 7.5: Run full pkg/script tests**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./pkg/script/...
```

Expected: ALL PASS.

- [ ] **Step 7.6: Commit**

```
git add pkg/script/handlers_vars.go pkg/script/handlers_vars_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-121 T7 — PUSH_VARN/POP_VARN type-aware dispatch

Rewrites handlePushVarn / handlePopVarn to dispatch on
Configs.VarnType(id) — STRING → push/popString, else → push/popInt.
Mirrors TS CoreOps.ts:61-91. Adds varnType helper with nil-Configs
fallback to int-only (DEVIATION-NAI-121-D3 pin).

4 new unit tests pin STRING dispatch (push + pop), the smoke-bind
PLAYER_UID-default-seeded read (== -1), and the nil-Configs fall-
through. Existing int-side tests stay green via int-default fallback.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 8: PUSH_VARP / POP_VARP type-aware dispatch + Protect gate

**Files:**
- Modify: `pkg/script/handlers_vars.go` (rewrite handlePushVarp / handlePopVarp)
- Modify: `pkg/script/handlers_vars_test.go` (extend with STRING + Protect tests)

- [ ] **Step 8.1: Rewrite `handlePushVarp` and `handlePopVarp`**

Replace the existing handlers in `pkg/script/handlers_vars.go:13-28` with:

```go
func handlePushVarp(s *ScriptState) error {
	if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
		return errors.New("PUSH_VARP: no active player")
	}
	id := varOperandID(s)
	typ, _ := s.varpType(id)
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
	typ, protect := s.varpType(id)
	// DEVIATION-NAI-121-D4: enforce TS CoreOps.ts:50-52 Protect gate.
	if protect && !s.Protect {
		return fmt.Errorf("POP_VARP: %%%d requires protected access", id)
	}
	if typ == objtype.ScriptVarTypeString {
		s.Self.SetVarpString(id, s.PopString())
	} else {
		s.Self.SetVarp(id, int32(s.PopInt()))
	}
	return nil
}
```

- [ ] **Step 8.2: Add `varpType` helper on `ScriptState`**

In `pkg/script/handlers_vars.go`, after the `varnType` helper added in T7, add:

```go
// varpType returns (type, protect) for player-var id from Configs,
// falling back to (ScriptVarTypeInt, false) when Configs is nil
// (test paths). Mirrors DEVIATION-NAI-121-D3.
func (s *ScriptState) varpType(id int) (objtype.ScriptVarType, bool) {
	if s.Configs == nil {
		return objtype.ScriptVarTypeInt, false
	}
	return s.Configs.VarpType(id)
}
```

Add the `fmt` import to the file's import block (the existing handlers don't use it):

```go
import (
	"errors"
	"fmt"

	"github.com/zsrv/goscape/pkg/objtype"
)
```

- [ ] **Step 8.3: Write tests for STRING + Protect gate**

Append to `pkg/script/handlers_vars_test.go`:

```go
func TestPushVarp_StringType_PushesString(t *testing.T) {
	sf := &ScriptFile{
		Name:             "push_varp_str",
		Opcodes:          []Opcode{OpPushVarp, OpReturn},
		IntOperands:      []int32{2, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	mp := &mockPlayer{varpsString: map[int]string{2: "hello"}}
	state := Init(sf, mp, false, nil, nil)
	state.Configs = &mockConfigs{
		varps: map[int]*objtype.VarPlayerType{
			2: {Type: objtype.ScriptVarTypeString},
		},
	}
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := state.PopString(); got != "hello" {
		t.Errorf("PushVarp(STRING): got %q, want %q", got, "hello")
	}
}

func TestPopVarp_StringType_PopsString(t *testing.T) {
	sf := &ScriptFile{
		Name: "pop_varp_str",
		Opcodes: []Opcode{
			OpPushConstantString, // push "xyz"
			OpPopVarp,            // write varp 4 = "xyz"
			OpReturn,
		},
		IntOperands:      []int32{0, 4, 0},
		StringOperands:   []string{"xyz", "", ""},
		InstructionCount: 3,
	}
	mp := &mockPlayer{}
	state := Init(sf, mp, false, nil, nil)
	state.Configs = &mockConfigs{
		varps: map[int]*objtype.VarPlayerType{
			4: {Type: objtype.ScriptVarTypeString},
		},
	}
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := mp.VarpString(4); got != "xyz" {
		t.Errorf("mp.varpsString[4]: got %q, want %q", got, "xyz")
	}
}

func TestPopVarp_ProtectGate_DeniesUnprotected(t *testing.T) {
	sf := &ScriptFile{
		Name: "pop_varp_protected_unprot",
		Opcodes: []Opcode{
			OpPushConstantInt,
			OpPopVarp,
			OpReturn,
		},
		IntOperands:      []int32{77, 5, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	mp := &mockPlayer{}
	state := Init(sf, mp, false /* protect=false */, nil, nil)
	state.Configs = &mockConfigs{
		varps: map[int]*objtype.VarPlayerType{
			5: {Type: objtype.ScriptVarTypeInt, Protect: true},
		},
	}
	err := Execute(state)
	if err == nil {
		t.Fatal("Execute: want Protect-gate error, got nil")
	}
	if !strings.Contains(err.Error(), "requires protected access") {
		t.Errorf("error: got %q, want substring 'requires protected access'", err.Error())
	}
}

func TestPopVarp_ProtectGate_AllowsProtected(t *testing.T) {
	sf := &ScriptFile{
		Name: "pop_varp_protected_prot",
		Opcodes: []Opcode{
			OpPushConstantInt,
			OpPopVarp,
			OpReturn,
		},
		IntOperands:      []int32{77, 5, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	mp := &mockPlayer{}
	state := Init(sf, mp, true /* protect=true */, nil, nil)
	state.Configs = &mockConfigs{
		varps: map[int]*objtype.VarPlayerType{
			5: {Type: objtype.ScriptVarTypeInt, Protect: true},
		},
	}
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := mp.Varp(5); got != 77 {
		t.Errorf("mp.Varp(5): got %d, want 77", got)
	}
}

func TestPopVarp_NonProtect_NoGate(t *testing.T) {
	// Confirm Protect=false varps don't gate even when state.Protect=false.
	sf := &ScriptFile{
		Name: "pop_varp_unprot",
		Opcodes: []Opcode{
			OpPushConstantInt,
			OpPopVarp,
			OpReturn,
		},
		IntOperands:      []int32{42, 5, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	mp := &mockPlayer{}
	state := Init(sf, mp, false, nil, nil)
	state.Configs = &mockConfigs{
		varps: map[int]*objtype.VarPlayerType{
			5: {Type: objtype.ScriptVarTypeInt, Protect: false},
		},
	}
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := mp.Varp(5); got != 42 {
		t.Errorf("mp.Varp(5): got %d, want 42", got)
	}
}
```

> Note: confirm `Init(sf, mp, protect bool, ...)` signature — the third arg in test calls is the `Protect` flag for ScriptState. Verify by reading `pkg/script/runner.go::Init` if uncertain.

- [ ] **Step 8.4: Verify all tests pass**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./pkg/script/ -run 'TestPushVarp|TestPopVarp' -v
```

Expected: All existing + 5 new tests PASS.

- [ ] **Step 8.5: Pre-flight check on real-fixture varp.dat Protect=true entries**

This is a **risk-register R5 mitigation**. Before declaring T8 done, audit any real-fixture impact:

```
# Build a one-shot test that loads varp.dat from cache and lists Protect=true entries:
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./modules/world/ -run 'TestVarpReal' -v
```

If no such test exists, write a one-time scratch check (or grep `objtype.LoadVarpTypes` callers for an existing list-dump); the goal is to know which varps in production set Protect=true and whether any goscape script test exercises them.

If Protect=true varps exist and a test case pops one without `state.Protect=true`, that test must be updated to set Protect=true OR the deviation must be expanded with the affected varp. **Document findings in the commit message.**

If no scripts pop Protect=true varps in tests: state "no test fixture exercises Protect=true POP_VARP — gate is dormant in goscape's test surface; production smoke validates" in the commit message.

- [ ] **Step 8.6: Run full pkg/script and modules/world tests**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./...
```

Expected: ALL PASS. **Cross-package green check** per `verify_implementer_claims`.

- [ ] **Step 8.7: Commit**

```
git add pkg/script/handlers_vars.go pkg/script/handlers_vars_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-121 T8 — PUSH_VARP/POP_VARP type-aware dispatch + Protect gate

Rewrites handlePushVarp / handlePopVarp to dispatch on
Configs.VarpType(id) — STRING → push/popString, else → push/popInt.
POP_VARP additionally enforces TS CoreOps.ts:50-52 Protect gate via
state.Protect (DEVIATION-NAI-121-D4 — new gate in goscape).

Adds varpType helper with nil-Configs fallback. 5 new unit tests pin
STRING dispatch (push + pop), Protect gate (denies unprotected,
allows protected), and Protect=false short-circuit. Pre-flight check
(R5 mitigation) verified no existing test fixture exercises a
Protect=true POP_VARP — gate is dormant in test surface; production
smoke validates.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 9: Retire `npcVarnCap` + cross-package final green check

**Files:**
- Modify: `modules/world/npc_script.go:13-15` (delete const + usage)

- [ ] **Step 9.1: Delete `npcVarnCap` constant + check site**

Locate `pkg/objtype/npc_script.go:13-15`:

```go
// npcVarnCap caps the per-NPC var slice so a rogue script cannot grow
// it unboundedly. Matches the engine-wide soft cap used in S6a.
const npcVarnCap = 1024
```

Delete this declaration entirely. Locate the usage in `SetNpcVarN` (around line 94-95, depending on diff state):

```go
if id >= npcVarnCap {
	return
}
```

Delete this branch. The remaining `if id < 0 || id >= len(n.varns) { return }` guard (which `SetNpcVarN` already has) is sufficient — slice is now sized to `len(s.varnTypes.Configs)`, so OOB writes are silently dropped, identical observable behavior.

- [ ] **Step 9.2: Verify no other references to `npcVarnCap`**

```
rg -n npcVarnCap pkg/ modules/ cmd/
```

Expected: zero matches.

- [ ] **Step 9.3: Final cross-package green check**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
```

Expected: ALL PASS, ALL vet-clean, build clean. **This is the Bundle 1 close gate.**

- [ ] **Step 9.4: Commit**

```
git add modules/world/npc_script.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
chore(world): NAI-121 T9 — retire npcVarnCap (slice now sized to registry)

Deletes the 1024 lazy-grow soft-cap on (*Npc).varns. Now redundant:
the slice is sized to len(server.varnTypes.Configs) at first
resetEntityForRespawn, and OOB writes are silently dropped by the
existing slice-bounds defensive guard. Identical observable behavior
on registry-bound writes.

Bundle 1 close. Cross-package go test ./... green.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Bundle 1 close gate

After Task 9 commit:
1. **Independent fresh test run** per `verify_implementer_claims`:
   ```
   GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./...
   ```
   Verify ALL PASS.
2. **Sonnet code-reviewer** dispatched with full diff context (every commit since `88b6da1` — the spec commit) per `superpowers_code_reviewer_model`. Reviewer scope: all 9 task commits, T3 + T8 deviations, R5 + R6 mitigations.
3. **Reviewer fixes** (if any) ship as a separate commit before Bundle 2 dispatch.

---

## Bundle 2 — V-PARTIAL Stage 1 audit (subagent dispatch)

### Task 2.1: Dispatch read-only investigation subagent

**Files:**
- Create: `docs/superpowers/investigations/2026-05-07-nai-121-vpartial-findings.md` (subagent output)

- [ ] **Step 2.1.1: Compose subagent prompt**

Use Sonnet `Explore` subagent type. Self-contained prompt:

> **Read-only investigation. Do not modify any code.**
>
> `%npc_combat_xp_multiplier` (declared in `LostCityRS/Content/scripts/npc/configs/ai_spawn.varn`, INT default) reads as 0 after a Tutorial Island giant-rat attack lands in goscape. The `[ai_spawn,_]` global trigger at `LostCityRS/Content/scripts/npc/scripts/ai_spawn.rs2:1-3` is supposed to populate it from `npc_param(combat_xp_multiplier)` on every NPC spawn. AI_SPAWN dispatch is wired in goscape — see `modules/world/npc_registry.go:82-99` (queues to `npcEventQueue`) and `modules/world/npc_event_queue.go:37-48` (dispatches every tick).
>
> Identify which step drops the value. Trace end-to-end:
>
> 1. **Script-pack inclusion.** Verify `[ai_spawn,_]` in `Content/scripts/npc/scripts/ai_spawn.rs2` is compiled into goscape's loaded `script.dat`. Methods: read `pkg/script/provider.go` and trace its load path; check whether the script provider's loaded scripts include the `ai_spawn,_` global key. Use `script.LookupKeyForGlobal(script.TriggerAiSpawn)` (TriggerAiSpawn = 166) as the lookup key.
> 2. **Provider lookup.** With NpcType.id = giant_rat (or any tutorial-island rat type), call `s.scriptProvider.GetByTrigger(script.TriggerAiSpawn, typeID, category)`. Should return a non-nil ScriptFile if (1) holds.
> 3. **Dispatch path.** Confirm `processNpcEventQueue` actually dispatches the queued ScriptFile for fresh-spawn NPCs (not delayed/dropped). Walk from the npcEventQueue append at `npc_registry.go:88-99` through the dispatcher.
> 4. **`npc_param` opcode handler.** Locate the handler for the `npc_param` opcode (likely in `pkg/script/handlers_npc.go` or `handlers_config.go`). Verify it reads the right field from `n.typ` for the `combat_xp_multiplier` param. If the param isn't read from a param table, identify what's missing.
> 5. **Var write path.** Confirm `pkg/script/handlers_vars.go::handlePopVarn` (post-NAI-121 T7) reaches the var with the right id and type.
>
> **Output:** write findings to `docs/superpowers/investigations/2026-05-07-nai-121-vpartial-findings.md`. Document:
> - Which of (1)-(5) holds and which breaks. Cite file:line for each.
> - The exact code path that drops the value (or the most likely candidate if it requires a runtime trace to confirm).
> - **Sized recommendation for Bundle 3:** one of (a) ≤30 LOC fix (direct edit), (b) 50-150 LOC fix (subagent-driven), (c) >200 LOC or out-of-scope (carry forward to NAI-122 with NAI-121 closing on Bundle 1 PRIMARY only).
> - Concrete fix sketch when (a) or (b).
>
> Do not write any production code. Do not commit your findings — return them as your final message and the controller will commit.

- [ ] **Step 2.1.2: Receive findings, controller-reviews, commits**

Controller reads findings markdown, sanity-checks against HEAD via `rg`/`Read` (per `audit_subagent_fabrication` — if findings cite specific file:line, verify they're real before relying on them), then commits:

```
git add docs/superpowers/investigations/2026-05-07-nai-121-vpartial-findings.md
git commit --no-gpg-sign -m "$(cat <<'EOF'
investigation(world): NAI-121 Bundle 2 — V-PARTIAL Stage 1 audit

%npc_combat_xp_multiplier reads-as-zero root cause investigation.
Subagent-traced end-to-end from script.dat inclusion through
provider lookup, npcEventQueue dispatch, npc_param opcode handler,
to var write path.

Findings recorded in docs/superpowers/investigations/2026-05-07-nai-121-vpartial-findings.md.
Routes Bundle 3 sizing per spec §8.1.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Bundle 3 — V-PARTIAL Stage 2 fix (sized from Bundle 2)

> **Cannot pre-write task-level steps without Bundle 2 findings.** The plan-author returns to update this section after Bundle 2 commits. Sketch routing per spec §8.1:

| Bundle 2 finding | Bundle 3 shape |
|---|---|
| `[ai_spawn,_]` not in `script.dat` | Content-pack pipeline diagnosis (`docs/superpowers/investigations/...rebuild.md`); fix may require pack-side rebuild outside engine scope |
| `Provider.GetByTrigger` lookup-key bug | One-line fix in `pkg/script/provider.go` + 1 unit test |
| `processNpcEventQueue` skip-condition bug | Branch fix + 1-2 unit tests in `modules/world/npc_event_queue_test.go` |
| `npc_param(combat_xp_multiplier)` opcode missing | Opcode handler port (50-150 LOC) — full TDD cycle in `pkg/script/handlers_config.go` or similar |
| Cross-system pipeline issue | Out-of-scope; route forward to NAI-122; NAI-121 closes Bundle 1 PRIMARY only |

**Update procedure:** After Bundle 2 commits, the plan-author re-edits this section with concrete tasks (Tasks 3.1, 3.2, ...) following the same 2-5-minute-per-step cadence. Commit the plan update separately, then dispatch Bundle 3 implementer.

---

## Bundle 3 close gate (if shipped)

After Bundle 3 commits (if any):
1. `go test ./...` green at HEAD.
2. Smoke handoff to user (per `smoke_test_server_handoff`): user launches goscape, attacks Tutorial Island giant rat, verifies combat XP > 0 on hit.
3. Sonnet code-reviewer between Bundle 3 commits if multi-task.

---

## NAI-121 close

**Triggered by either:**
- Bundle 1 + Bundle 2 + Bundle 3 all complete + smoke binds combat XP > 0; OR
- Bundle 1 + Bundle 2 complete; Bundle 3 routes forward to NAI-122 with documented findings; smoke binds Bundle 1 PRIMARY ("It's not after you." gate skipped).

**Close commit pattern** (per `close_commit_memory_trailer`):

```
chore(close): NAI-121 — final close

PRIMARY met (smoke 2026-05-XX): "It's not after you." gate no longer
fires on fresh-spawn NPCs. [Optional: SECONDARY V-PARTIAL met /
forwarded to NAI-122 — see findings doc.]

Closes memory: nai_followups.md NAI-120 SECONDARY (varn default-seed),
nai_followups.md NAI-120 V-PARTIAL parked,
npc_varn_default_seed_per_type.md (PRIMARY closed) [retire entire memory file if all open follow-ups close],
[any new memory entries created during NAI-121].

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
```

**Memory updates** at close (per `post_task_handoff`):
- `nai_followups.md`: add NAI-121 close section mirroring the NAI-120 template.
- Save any NEW non-derivable patterns surfaced during NAI-121 to memory (e.g., if Bundle 2 audit reveals a script-pack pipeline gap pattern worth pinning).
- Retire `npc_varn_default_seed_per_type.md` if PRIMARY met cleanly (mark with closure date in MEMORY.md or delete entirely).

---

## Self-review

### Spec coverage
- §3.1 Bundle 1 every item → mapped to T1-T9. ✓
- §3.1 Bundle 2 → Task 2.1. ✓
- §3.1 Bundle 3 → Bundle 3 section (sized post-B2). ✓
- §4 deviations D1-D5 → noted in commit messages (D1 boot-fail in T1, D2 string-zero in T3, D3 fall-through in T2/T7/T8, D4 Protect gate in T8, D5 size-pinning implicit). ✓
- §6 components 6.1-6.10 → T1, T2, T3, T4, T5, T6, T6, T7, T8, T9. ✓
- §9 risk register R1-R6:
  - R1 (Player varp default change) — addressed implicitly in T4 cross-module test pass + T8 cross-package green check. **Should add explicit pre-flight grep step in T4.**
  - R2 (raw &Npc{} panic) — `TestPushVarn_NilConfigsFallsBackToInt` in T7. ✓
  - R3 (mock-World methods) — T2 covers via mockConfigs; **mockNpc/mockPlayer get methods added in T5/T6**. The "mock-World" framing in spec was loose — actual mock surface is mockConfigs/mockNpc/mockPlayer. ✓
  - R4 (V-PARTIAL deeper issue) — Bundle 2 findings drive sizing; handled. ✓
  - R5 (Protect gate breaks test) — T8 Step 8.5 pre-flight. ✓
  - R6 (raw &Npc{}.SetNpcVarN sites) — T3 Step 3.5 verifies pre-existing tests; only 2 sites in npc_hunt_test.go and they construct via NewNpc. ✓
- §10 test plan items → T1, T3, T4, T7, T8 cover all listed unit tests. ✓

### Placeholder scan
- No "TBD" / "TODO" / "implement later" found.
- Bundle 3 is intentionally deferred-with-routing (not a placeholder; conditional on B2 findings).
- All code blocks complete.

### Type consistency
- `NpcVarNString` / `SetNpcVarNString` — used consistently across T5 (interface, mock, accessor) and T7 (handler).
- `VarpString` / `SetVarpString` — used consistently across T6 and T8.
- `varnsString` / `varpsString` — used consistently across T3, T4, T5, T6.
- `seedVarnTypes` / `seedVarpTypesByType` — disambiguated from existing `seedVarpTypes(s, transmit)` at script_test.go:368.
- `state.Protect` (ScriptState field) vs `vt.Protect` (VarPlayerType field) — both exist; named consistently in T8.

**One issue found in self-review:** R1 mitigation is listed but not enforced in T4. **Adding an explicit pre-flight step:**

> **T4 Step 4.0 (insert before 4.1):** Pre-flight grep for non-INT varps with implicit-zero test assertions:
> ```
> rg -n 'p.varps\[|p.Varp\(|mp.Varp\(|mp.varps\[' modules/world/*_test.go pkg/script/*_test.go
> ```
> Cross-reference each hit against `varpTypes.Configs[id].Type` to identify any test that asserts `Varp(id) == 0` for a non-INT varp. Document findings; if any, escalate to deviation expansion or test update.

(Add this as a controller-side pre-flight step before T4 implementer dispatch — not a checkbox in the task itself.)

---

## Summary

**Bundle 1:** 9 tasks, 9 commits. Smoke-binding fix.
**Bundle 2:** 1 audit subagent dispatch + 1 commit.
**Bundle 3:** Conditional, sized from Bundle 2.

Total estimated LOC: 250-400 (Bundle 1) + 0 production (Bundle 2) + ?-? (Bundle 3).

Estimated wall-clock at standard cadence: Bundle 1 ~9 implementer cycles + 1 reviewer; Bundle 2 ~1 explore subagent; Bundle 3 ~1-3 cycles.
