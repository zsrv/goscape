package objtype

import (
	"testing"

	packet2 "github.com/zsrv/goscape/pkg/io/packet"
)

type varpEntry struct {
	debugName string
	scope     int
	transmit  bool
}

// buildVarpDat assembles a varp.dat wire blob:
//
//	u16 count
//	for each entry: sequence of (code, payload) pairs terminated by code 0.
func buildVarpDat(entries []varpEntry) []byte {
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
	entries := []varpEntry{
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

func TestVarpProtectDefaultTrue(t *testing.T) {
	// No code 4 → Protect stays true.
	blob := buildVarpDat([]varpEntry{{"x", 0, false}})
	cfgs, err := parseVarpTypes(packet2.NewPacket(blob))
	if err != nil {
		t.Fatalf("parseVarpTypes: %v", err)
	}
	if !cfgs.Configs[0].Protect {
		t.Errorf("Protect default: got false, want true")
	}
}
