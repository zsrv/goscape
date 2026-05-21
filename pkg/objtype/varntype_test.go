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
	p := packet.NewPacket(nil)
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
	if idx, ok := cfg.ConfigNames["default_int_var"]; !ok || idx != 0 {
		t.Errorf("ConfigNames[default_int_var]: got (%d, %v), want (0, true)", idx, ok)
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
	p := packet.NewPacket(nil)
	p.P2(1)  // count
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
	if idx, ok := cfg.ConfigNames["npc_macro_event_target"]; !ok || idx != 0 {
		t.Errorf("ConfigNames[npc_macro_event_target]: got (%d, %v), want (0, true)", idx, ok)
	}
}

func TestParseVarnTypes_MultiEntry_ConfigNamesIndexed(t *testing.T) {
	// Two entries: anonymous at id 0, named at id 1. Catches a bug
	// where ConfigNames always inserts index 0 regardless of loop id.
	p := buildVarnPacket([]struct {
		Type int
		Name string
	}{
		{Type: int(ScriptVarTypeInt), Name: ""},
		{Type: int(ScriptVarTypePlayerUid), Name: "named_var"},
	})
	cfg, err := parseVarnTypes(p)
	if err != nil {
		t.Fatalf("parseVarnTypes: %v", err)
	}
	if len(cfg.Configs) != 2 {
		t.Fatalf("Configs length: got %d, want 2", len(cfg.Configs))
	}
	if idx, ok := cfg.ConfigNames["named_var"]; !ok || idx != 1 {
		t.Errorf("ConfigNames[named_var]: got (%d, %v), want (1, true)", idx, ok)
	}
	if _, ok := cfg.ConfigNames[""]; ok {
		t.Error("ConfigNames should not contain empty-string key for anonymous entry")
	}
}

func TestLoadVarnTypes_FileMissing_ReturnsError(t *testing.T) {
	// DEVIATION-NAI-121-D1 pin: TS VarNpcType.load silently returns when
	// varn.dat is missing; goscape fails loud at boot, matching the
	// existing varp/vars loader pattern.
	dir := t.TempDir() // empty — no server/varn.dat under it
	_, err := LoadVarnTypes(dir)
	if err == nil {
		t.Fatal("LoadVarnTypes: want error for missing varn.dat, got nil")
	}
}
