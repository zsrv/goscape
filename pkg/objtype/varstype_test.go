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
