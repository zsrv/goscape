package objtype

import (
	"testing"

	packet2 "github.com/zsrv/goscape/pkg/io/packet"
)

type structEntry struct {
	debugName string
	intParams map[uint32]uint32
	strParams map[uint32]string
}

// buildStructDat assembles a struct.dat wire blob:
//
//	u16 count
//	for each entry: sequence of (code, payload) pairs terminated by code 0.
func buildStructDat(entries []structEntry) []byte {
	pkt := packet2.NewPacket(nil)
	pkt.P2(uint16(len(entries)))
	for _, e := range entries {
		total := len(e.intParams) + len(e.strParams)
		if total > 0 {
			pkt.P1(249)
			pkt.P1(uint8(total))
			for k, v := range e.intParams {
				// key is a u24
				pkt.P3(k)
				pkt.PBool(false) // int
				pkt.P4(v)
			}
			for k, v := range e.strParams {
				pkt.P3(k)
				pkt.PBool(true) // string
				pkt.PJStrLF(v)
			}
		}
		if e.debugName != "" {
			pkt.P1(250)
			pkt.PJStrLF(e.debugName)
		}
		pkt.P1(0)
	}
	return pkt.Bytes()
}

func TestParseStructTypes(t *testing.T) {
	entries := []structEntry{
		{
			debugName: "npc_template_generic",
			intParams: map[uint32]uint32{1: 42},
			strParams: map[uint32]string{2: "hello"},
		},
		{
			debugName: "empty_struct",
		},
	}

	blob := buildStructDat(entries)
	cfgs, err := parseStructTypes(packet2.NewPacket(blob))
	if err != nil {
		t.Fatalf("parseStructTypes: %v", err)
	}
	if len(cfgs.Configs) != 2 {
		t.Fatalf("configs: got %d, want 2", len(cfgs.Configs))
	}

	s := cfgs.Configs[0]
	if s.DebugName != "npc_template_generic" {
		t.Errorf("DebugName[0]: got %q", s.DebugName)
	}
	if got, _ := s.Params[1].(uint32); got != 42 {
		t.Errorf("Params[1]: got %v, want 42", s.Params[1])
	}
	if got, _ := s.Params[2].(string); got != "hello" {
		t.Errorf("Params[2]: got %v, want hello", s.Params[2])
	}

	if cfgs.ConfigNames["npc_template_generic"] != 0 {
		t.Errorf("ConfigNames: got %d, want 0", cfgs.ConfigNames["npc_template_generic"])
	}
	if cfgs.ConfigNames["empty_struct"] != 1 {
		t.Errorf("ConfigNames[empty_struct]: got %d, want 1", cfgs.ConfigNames["empty_struct"])
	}
}

func TestStructUnknownCode(t *testing.T) {
	pkt := packet2.NewPacket(nil)
	pkt.P2(1)
	pkt.P1(77) // bogus
	pkt.P1(0)
	_, err := parseStructTypes(packet2.NewPacket(pkt.Bytes()))
	if err == nil {
		t.Fatal("expected error on unknown struct code, got nil")
	}
}
