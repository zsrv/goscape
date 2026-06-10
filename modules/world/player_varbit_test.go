package world

import (
	"log/slog"
	"testing"

	io2 "github.com/zsrv/goscape/pkg/io/isaac"
	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
	"github.com/zsrv/goscape/pkg/objtype"
)

// newVarbitTestPlayer wires a Player to a Server whose varbit registry
// holds a single varbit: id 0 → basevar 0, bits [4,7] (mask 0xF).
// varps is sized 1; the base varp's type config is non-transmit unless
// the test flips it.
func newVarbitTestPlayer(t *testing.T) (*Player, *Server) {
	t.Helper()
	p, _ := newTestPlayer(t)
	s := newTestServer(t)
	s.varpTypes = &objtype.VarpTypeConfigs{
		Configs: []*objtype.VarPlayerType{{}},
	}
	s.varbitTypes = &objtype.VarBitTypeConfigs{
		Configs: []*objtype.VarBitType{
			{ConfigType: objtype.ConfigType{ID: 0}, Basevar: 0, Startbit: 4, Endbit: 7},
		},
	}
	p.client.server = s
	p.varps = make([]int32, 1)
	return p, s
}

func TestSetVarBit_WritesBitRange(t *testing.T) {
	p, _ := newVarbitTestPlayer(t)

	p.SetVarBit(0, 0xB)

	if p.varps[0] != 0xB0 {
		t.Errorf("varps[0]: got %#x, want 0xB0", p.varps[0])
	}
	if got := p.GetVarBit(0); got != 0xB {
		t.Errorf("GetVarBit(0): got %#x, want 0xB", got)
	}
}

// TestSetVarBit_OutOfRangeWritesZero pins the TS clamp at Player.ts:1771-1773
// @43e02957: values outside [0,mask] write 0 — NOT a saturate-to-mask.
func TestSetVarBit_OutOfRangeWritesZero(t *testing.T) {
	cases := []struct {
		name  string
		value int32
	}{
		{"above_mask", 0x10}, // mask is 0xF
		{"negative", -1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, _ := newVarbitTestPlayer(t)
			p.varps[0] = 0xB0 // pre-set so the zero-write is observable

			p.SetVarBit(0, tc.value)

			if p.varps[0] != 0 {
				t.Errorf("varps[0]: got %#x, want 0 (TS clamps to 0)", p.varps[0])
			}
			if got := p.GetVarBit(0); got != 0 {
				t.Errorf("GetVarBit(0): got %#x, want 0", got)
			}
		})
	}
}

func TestSetVarBit_PreservesBitsOutsideRange(t *testing.T) {
	p, _ := newVarbitTestPlayer(t)
	p.varps[0] = 0xF0F // bits 0-3 and 8-11 set; varbit range [4,7] clear

	p.SetVarBit(0, 0xB)

	if p.varps[0] != 0xFBF {
		t.Errorf("varps[0]: got %#x, want 0xFBF (outside bits preserved)", p.varps[0])
	}
	if got := p.GetVarBit(0); got != 0xB {
		t.Errorf("GetVarBit(0): got %#x, want 0xB", got)
	}
	if low := p.varps[0] & 0xF; low != 0xF {
		t.Errorf("bits [0,3]: got %#x, want 0xF", low)
	}
	if high := p.varps[0] >> 8 & 0xF; high != 0xF {
		t.Errorf("bits [8,11]: got %#x, want 0xF", high)
	}
}

func TestVarBit_UnknownIDNoop(t *testing.T) {
	p, _ := newVarbitTestPlayer(t)
	p.varps[0] = 0xF0F

	if got := p.GetVarBit(99); got != 0 {
		t.Errorf("GetVarBit(99): got %#x, want 0", got)
	}
	p.SetVarBit(99, 5)
	if p.varps[0] != 0xF0F {
		t.Errorf("varps[0] after SetVarBit(99): got %#x, want 0xF0F (no-op)", p.varps[0])
	}
}

// TestVarBit_UnconfiguredGuard pins the Go-side defensive guard: a varbit
// with only a debugname (code-1 fields all -1) must read 0 / no-op
// instead of panicking on the negative shift. TS would NaN/crash here;
// content never ships such varbits. See the PORTING-EXCEPTION note on
// GetVarBit/SetVarBit.
func TestVarBit_UnconfiguredGuard(t *testing.T) {
	p, s := newVarbitTestPlayer(t)
	s.varbitTypes = &objtype.VarBitTypeConfigs{
		Configs: []*objtype.VarBitType{
			{ConfigType: objtype.ConfigType{ID: 0, DebugName: "name_only"}, Basevar: -1, Startbit: -1, Endbit: -1},
		},
	}
	p.varps[0] = 0xF0F

	if got := p.GetVarBit(0); got != 0 {
		t.Errorf("GetVarBit(unconfigured): got %#x, want 0", got)
	}
	p.SetVarBit(0, 1)
	if p.varps[0] != 0xF0F {
		t.Errorf("varps[0] after SetVarBit(unconfigured): got %#x, want 0xF0F (no-op)", p.varps[0])
	}
}

// TestSetVarBit_RoutesThroughSetVarp asserts the write goes through
// SetVarp (TS Player.ts:1776 routes through this.setVar) so the
// VARP_SMALL/LARGE client resync fires for transmit varps. Uses the
// nai138.write_varp probe, same binding as the writeVarp probe tests.
func TestSetVarBit_RoutesThroughSetVarp(t *testing.T) {
	p, s := newVarbitTestPlayer(t)
	s.varpTypes.Configs[0] = &objtype.VarPlayerType{Transmit: true}
	s.cfg.NodeDebug = true
	rec := &nai138WorldHandler{}
	s.log = slog.New(rec)
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	p.SetVarBit(0, 0xB)

	var probe *slog.Record
	for i := range rec.records {
		if rec.records[i].Message == "nai138.write_varp" {
			probe = &rec.records[i]
			break
		}
	}
	if probe == nil {
		t.Fatalf("nai138.write_varp record absent (SetVarBit did not route through SetVarp); got %d records", len(rec.records))
	}
	got := recordAttrs138(*probe)
	if got["id"] != int64(0) {
		t.Errorf(`attr "id": got %v, want 0 (the basevar)`, got["id"])
	}
	if got["value"] != int64(0xB0) {
		t.Errorf(`attr "value": got %v, want %d (0xB0)`, got["value"], 0xB0)
	}
	// 0xB0 = 176 > 127 → the composed varp value rides VARP_LARGE
	// (writeVarp small/large split on the FULL varp value, not the
	// varbit slice — same as TS writeVarp on the setVar result).
	if got["opcode"] != int64(gameserver.OpVarpLarge.Opcode) {
		t.Errorf(`attr "opcode": got %v, want %d (OpVarpLarge)`, got["opcode"], gameserver.OpVarpLarge.Opcode)
	}
}
