package objtype

import (
	"testing"

	packet2 "github.com/zsrv/goscape/pkg/io/packet"
)

type varpEntry struct {
	debugName  string
	scope      int
	transmit   bool
	clientCode uint16
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
		if e.clientCode != 0 {
			pkt.P1(5)
			pkt.P2(e.clientCode)
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
		{debugName: "coins", scope: 0, transmit: true},
		{debugName: "quest_state", scope: 1},
		{debugName: "anon"},
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
	blob := buildVarpDat([]varpEntry{{"x", 0, false, 0}})
	cfgs, err := parseVarpTypes(packet2.NewPacket(blob))
	if err != nil {
		t.Fatalf("parseVarpTypes: %v", err)
	}
	if !cfgs.Configs[0].Protect {
		t.Errorf("Protect default: got false, want true")
	}
}

// TestParseVarpTypes_DiscoversRunIDFromClientCode7 mirrors TS VarPlayerType.ts:50-53:
// the varp config with ClientCode==7 is recorded as VarpTypeConfigs.RunID.
func TestParseVarpTypes_DiscoversRunIDFromClientCode7(t *testing.T) {
	entries := []varpEntry{
		{debugName: "other_a", clientCode: 0},
		{debugName: "option_run", clientCode: 7}, // id=1 — the run varp
		{debugName: "other_b", clientCode: 0},
	}
	cfgs, err := parseVarpTypes(packet2.NewPacket(buildVarpDat(entries)))
	if err != nil {
		t.Fatalf("parseVarpTypes: %v", err)
	}
	if cfgs.RunID != 1 {
		t.Errorf("RunID: got %d, want 1", cfgs.RunID)
	}
	if cfgs.Configs[1].ClientCode != 7 {
		t.Errorf("Configs[1].ClientCode: got %d, want 7", cfgs.Configs[1].ClientCode)
	}
}

// TestParseVarpTypes_RunIDDefaultsZeroWhenNoClientCode7 pins the TS-faithful
// default-0 fallback (VarPlayerType.ts:18) when no clientcode-7 config exists.
func TestParseVarpTypes_RunIDDefaultsZeroWhenNoClientCode7(t *testing.T) {
	entries := []varpEntry{
		{debugName: "alpha", clientCode: 1},
		{debugName: "beta", clientCode: 3},
	}
	cfgs, err := parseVarpTypes(packet2.NewPacket(buildVarpDat(entries)))
	if err != nil {
		t.Fatalf("parseVarpTypes: %v", err)
	}
	if cfgs.RunID != 0 {
		t.Errorf("RunID: got %d, want 0 (default)", cfgs.RunID)
	}
}
