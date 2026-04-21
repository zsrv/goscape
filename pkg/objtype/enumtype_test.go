package objtype

import (
	"testing"

	packet2 "github.com/zsrv/goscape/pkg/io/packet"
)

type enumEntry struct {
	debugName     string
	inputType     ScriptVarType
	outputType    ScriptVarType
	defaultInt    int32
	defaultString string
	intValues     map[int32]int32
	strValues     map[int32]string
}

// buildEnumDat assembles an enum.dat wire blob:
//
//	u16 count
//	for each entry: sequence of (code, payload) pairs terminated by code 0.
func buildEnumDat(entries []enumEntry) []byte {
	pkt := packet2.NewPacket(nil)
	pkt.P2(uint16(len(entries)))
	for _, e := range entries {
		if e.inputType != 0 {
			pkt.P1(1)
			pkt.P1(uint8(e.inputType))
		}
		if e.outputType != 0 {
			pkt.P1(2)
			pkt.P1(uint8(e.outputType))
		}
		if e.defaultString != "" {
			pkt.P1(3)
			pkt.PJStrLF(e.defaultString)
		}
		if e.defaultInt != 0 {
			pkt.P1(4)
			pkt.P4(uint32(e.defaultInt))
		}
		if len(e.strValues) > 0 {
			pkt.P1(5)
			pkt.P2(uint16(len(e.strValues)))
			// Iteration order in Go maps is randomised; the decoder is
			// insensitive to order so this is fine for the round-trip.
			for k, v := range e.strValues {
				pkt.P4(uint32(k))
				pkt.PJStrLF(v)
			}
		}
		if len(e.intValues) > 0 {
			pkt.P1(6)
			pkt.P2(uint16(len(e.intValues)))
			for k, v := range e.intValues {
				pkt.P4(uint32(k))
				pkt.P4(uint32(v))
			}
		}
		if e.debugName != "" {
			pkt.P1(250)
			pkt.PJStrLF(e.debugName)
		}
		pkt.P1(0) // terminator
	}
	return pkt.Bytes()
}

func TestParseEnumTypes(t *testing.T) {
	entries := []enumEntry{
		{
			debugName:  "stat_name",
			inputType:  ScriptVarTypeInt,
			outputType: ScriptVarTypeString,
			strValues:  map[int32]string{0: "Attack", 1: "Defence", 2: "Strength"},
		},
		{
			debugName:  "stat_xp",
			inputType:  ScriptVarTypeInt,
			outputType: ScriptVarTypeInt,
			defaultInt: -1,
			intValues:  map[int32]int32{0: 100, 1: 200},
		},
		{
			debugName:     "stat_default_string",
			inputType:     ScriptVarTypeInt,
			outputType:    ScriptVarTypeString,
			defaultString: "unknown",
		},
	}

	blob := buildEnumDat(entries)
	pkt := packet2.NewPacket(blob)

	cfgs, err := parseEnumTypes(pkt)
	if err != nil {
		t.Fatalf("parseEnumTypes: %v", err)
	}
	if len(cfgs.Configs) != 3 {
		t.Fatalf("configs: got %d, want 3", len(cfgs.Configs))
	}

	stat := cfgs.Configs[0]
	if stat.DebugName != "stat_name" {
		t.Errorf("DebugName[0]: got %q, want %q", stat.DebugName, "stat_name")
	}
	if stat.InputType != ScriptVarTypeInt {
		t.Errorf("InputType[0]: got %d, want %d", stat.InputType, ScriptVarTypeInt)
	}
	if stat.OutputType != ScriptVarTypeString {
		t.Errorf("OutputType[0]: got %d, want %d", stat.OutputType, ScriptVarTypeString)
	}
	if got, _ := stat.Values[0].(string); got != "Attack" {
		t.Errorf("Values[0][0]: got %q, want %q", got, "Attack")
	}
	if got, _ := stat.Values[2].(string); got != "Strength" {
		t.Errorf("Values[0][2]: got %q, want %q", got, "Strength")
	}

	xp := cfgs.Configs[1]
	if xp.DefaultInt != -1 {
		t.Errorf("DefaultInt[1]: got %d, want -1", xp.DefaultInt)
	}
	if got, _ := xp.Values[0].(int32); got != 100 {
		t.Errorf("Values[1][0]: got %d, want 100", got)
	}
	if got, _ := xp.Values[1].(int32); got != 200 {
		t.Errorf("Values[1][1]: got %d, want 200", got)
	}

	ds := cfgs.Configs[2]
	if ds.DefaultString != "unknown" {
		t.Errorf("DefaultString[2]: got %q, want %q", ds.DefaultString, "unknown")
	}

	if cfgs.ConfigNames["stat_name"] != 0 {
		t.Errorf("ConfigNames[stat_name]: got %d, want 0", cfgs.ConfigNames["stat_name"])
	}
	if cfgs.ConfigNames["stat_xp"] != 1 {
		t.Errorf("ConfigNames[stat_xp]: got %d, want 1", cfgs.ConfigNames["stat_xp"])
	}
}

func TestEnumDefaults(t *testing.T) {
	// Empty entry — every field should take its NewEnumType default.
	blob := buildEnumDat([]enumEntry{{debugName: "empty"}})
	cfgs, err := parseEnumTypes(packet2.NewPacket(blob))
	if err != nil {
		t.Fatalf("parseEnumTypes: %v", err)
	}
	e := cfgs.Configs[0]
	if e.InputType != ScriptVarTypeInt {
		t.Errorf("InputType default: got %d, want %d", e.InputType, ScriptVarTypeInt)
	}
	if e.OutputType != ScriptVarTypeInt {
		t.Errorf("OutputType default: got %d, want %d", e.OutputType, ScriptVarTypeInt)
	}
	if e.DefaultString != "null" {
		t.Errorf("DefaultString default: got %q, want %q", e.DefaultString, "null")
	}
	if e.DefaultInt != 0 {
		t.Errorf("DefaultInt default: got %d, want 0", e.DefaultInt)
	}
	if len(e.Values) != 0 {
		t.Errorf("Values default: got %v, want empty", e.Values)
	}
}

func TestEnumUnknownCode(t *testing.T) {
	pkt := packet2.NewPacket(nil)
	pkt.P2(1)
	pkt.P1(99) // bogus
	pkt.P1(0)
	_, err := parseEnumTypes(packet2.NewPacket(pkt.Bytes()))
	if err == nil {
		t.Fatal("expected error on unknown enum code, got nil")
	}
}
