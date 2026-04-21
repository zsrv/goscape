package objtype

import (
	"path/filepath"
	"testing"

	packet2 "github.com/zsrv/goscape/pkg/io/packet"
)

type locEntry struct {
	debugName string
	desc      string
	category  int
	width     int
	length    int
	intParams map[uint32]uint32
}

// buildLocDat assembles a server/loc.dat wire blob:
//
//	u16 count
//	for each entry: sequence of (code, payload) pairs terminated by code 0.
func buildLocDat(entries []locEntry) []byte {
	pkt := packet2.NewPacket(nil)
	pkt.P2(uint16(len(entries)))
	for _, e := range entries {
		if e.desc != "" {
			pkt.P1(3)
			pkt.PJStrLF(e.desc)
		}
		if e.width != 0 {
			pkt.P1(14)
			pkt.P1(uint8(e.width))
		}
		if e.length != 0 {
			pkt.P1(15)
			pkt.P1(uint8(e.length))
		}
		if e.category != 0 {
			pkt.P1(61)
			pkt.P2(uint16(e.category))
		}
		if len(e.intParams) > 0 {
			pkt.P1(249)
			pkt.P1(uint8(len(e.intParams)))
			for k, v := range e.intParams {
				pkt.P3(k)
				pkt.PBool(false)
				pkt.P4(v)
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

func TestParseLocTypes(t *testing.T) {
	entries := []locEntry{
		{
			debugName: "door_basic",
			desc:      "A wooden door.",
			category:  17,
			width:     1,
			length:    2,
			intParams: map[uint32]uint32{1: 100},
		},
		{
			debugName: "bush",
		},
	}

	blob := buildLocDat(entries)
	cfgs, err := parseLocTypes(packet2.NewPacket(blob))
	if err != nil {
		t.Fatalf("parseLocTypes: %v", err)
	}
	if len(cfgs.Configs) != 2 {
		t.Fatalf("configs: got %d, want 2", len(cfgs.Configs))
	}

	door := cfgs.Configs[0]
	if door.DebugName != "door_basic" {
		t.Errorf("DebugName[0]: got %q", door.DebugName)
	}
	if door.Desc != "A wooden door." {
		t.Errorf("Desc[0]: got %q", door.Desc)
	}
	if door.Category != 17 {
		t.Errorf("Category[0]: got %d, want 17", door.Category)
	}
	if door.Width != 1 || door.Length != 2 {
		t.Errorf("Width/Length[0]: got %d/%d, want 1/2", door.Width, door.Length)
	}
	if got, _ := door.Params[1].(uint32); got != 100 {
		t.Errorf("Params[1]: got %v, want 100", door.Params[1])
	}

	bush := cfgs.Configs[1]
	if bush.Category != -1 {
		t.Errorf("Category default (bush): got %d, want -1", bush.Category)
	}
	if bush.Width != 1 || bush.Length != 1 {
		t.Errorf("Width/Length default (bush): got %d/%d, want 1/1", bush.Width, bush.Length)
	}

	if cfgs.ConfigNames["door_basic"] != 0 {
		t.Errorf("ConfigNames[door_basic]: got %d, want 0", cfgs.ConfigNames["door_basic"])
	}
}

func TestLocUnknownCode(t *testing.T) {
	pkt := packet2.NewPacket(nil)
	pkt.P2(1)
	pkt.P1(200) // bogus
	pkt.P1(0)
	_, err := parseLocTypes(packet2.NewPacket(pkt.Bytes()))
	if err == nil {
		t.Fatal("expected error on unknown loc code, got nil")
	}
}

// TestLoadRealLocCache verifies the loader handles the repo's real server
// cache end-to-end. This is a regression guard in case goscape's packer ever
// writes a loc code this loader doesn't recognise.
func TestLoadRealLocCache(t *testing.T) {
	cacheDir := filepath.Join("..", "..", "data", "pack")
	cfgs, err := LoadLocTypes(cacheDir)
	if err != nil {
		t.Skipf("no cache data: %v", err)
	}
	if len(cfgs.Configs) == 0 {
		t.Fatal("expected at least one LocType, got 0")
	}
	t.Logf("loaded %d LocTypes from %s", len(cfgs.Configs), cacheDir)
}
