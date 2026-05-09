package world

import (
	"encoding/hex"
	"log/slog"
	"testing"

	io2 "github.com/zsrv/goscape/pkg/io/isaac"
	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
	"github.com/zsrv/goscape/pkg/objtype"
)

func TestWriteVarp_Probe_SmallValueShape(t *testing.T) {
	p, _ := newTestPlayer(t)
	s := newTestServer(t)
	s.varpTypes = &objtype.VarpTypeConfigs{
		Configs: make([]*objtype.VarPlayerType, 174),
		RunID:   173,
	}
	s.varpTypes.Configs[173] = &objtype.VarPlayerType{Transmit: true}
	s.cfg.NodeDebug = true
	rec := &nai138WorldHandler{}
	s.log = slog.New(rec)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	p.writeVarp(173, 0)

	var probe *slog.Record
	for i := range rec.records {
		if rec.records[i].Message == "nai138.write_varp" {
			probe = &rec.records[i]
			break
		}
	}
	if probe == nil {
		t.Fatalf("nai138.write_varp record absent; got %d records", len(rec.records))
	}
	got := recordAttrs138(*probe)
	if got["id"] != int64(173) {
		t.Errorf(`attr "id": got %v, want 173`, got["id"])
	}
	if got["value"] != int64(0) {
		t.Errorf(`attr "value": got %v, want 0`, got["value"])
	}
	if got["opcode"] != int64(gameserver.OpVarpSmall.Opcode) {
		t.Errorf(`attr "opcode": got %v, want %d (OpVarpSmall)`,
			got["opcode"], gameserver.OpVarpSmall.Opcode)
	}
	// Payload: P2(173) + P1(0) = 0x00, 0xAD, 0x00 (3 bytes)
	wantHex := hex.EncodeToString([]byte{0x00, 0xAD, 0x00})
	if got["payload_hex"] != wantHex {
		t.Errorf(`attr "payload_hex": got %q, want %q`, got["payload_hex"], wantHex)
	}
	if got["payload_len"] != int64(3) {
		t.Errorf(`attr "payload_len": got %v, want 3`, got["payload_len"])
	}
}

func TestWriteVarp_Probe_LargeValueShape(t *testing.T) {
	p, _ := newTestPlayer(t)
	s := newTestServer(t)
	s.varpTypes = &objtype.VarpTypeConfigs{
		Configs: make([]*objtype.VarPlayerType, 174),
		RunID:   173,
	}
	s.varpTypes.Configs[100] = &objtype.VarPlayerType{Transmit: true}
	s.cfg.NodeDebug = true
	rec := &nai138WorldHandler{}
	s.log = slog.New(rec)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	p.writeVarp(100, 200) // 200 > 127 → VARP_LARGE

	var probe *slog.Record
	for i := range rec.records {
		if rec.records[i].Message == "nai138.write_varp" {
			probe = &rec.records[i]
			break
		}
	}
	if probe == nil {
		t.Fatalf("nai138.write_varp record absent; got %d records", len(rec.records))
	}
	got := recordAttrs138(*probe)
	if got["opcode"] != int64(gameserver.OpVarpLarge.Opcode) {
		t.Errorf(`attr "opcode": got %v, want %d (OpVarpLarge)`,
			got["opcode"], gameserver.OpVarpLarge.Opcode)
	}
	if got["payload_len"] != int64(6) {
		t.Errorf(`attr "payload_len": got %v, want 6 (P2 id + P4 value)`, got["payload_len"])
	}
}

func TestWriteVarp_Probe_SuppressedWhenNodeDebugFalse(t *testing.T) {
	p, _ := newTestPlayer(t)
	s := newTestServer(t)
	s.varpTypes = &objtype.VarpTypeConfigs{
		Configs: make([]*objtype.VarPlayerType, 174),
		RunID:   173,
	}
	s.varpTypes.Configs[173] = &objtype.VarPlayerType{Transmit: true}
	s.cfg.NodeDebug = false
	rec := &nai138WorldHandler{}
	s.log = slog.New(rec)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	p.writeVarp(173, 0)

	for _, r := range rec.records {
		if r.Message == "nai138.write_varp" {
			t.Errorf("probe emitted under NodeDebug=false")
			return
		}
	}
}

func TestWriteVarp_Probe_NotEmittedForNonTransmitVarp(t *testing.T) {
	p, _ := newTestPlayer(t)
	s := newTestServer(t)
	s.varpTypes = &objtype.VarpTypeConfigs{
		Configs: make([]*objtype.VarPlayerType, 174),
		RunID:   173,
	}
	s.varpTypes.Configs[173] = &objtype.VarPlayerType{Transmit: false}
	s.cfg.NodeDebug = true
	rec := &nai138WorldHandler{}
	s.log = slog.New(rec)
	p.client.server = s

	p.writeVarp(173, 0)

	for _, r := range rec.records {
		if r.Message == "nai138.write_varp" {
			t.Errorf("probe emitted for non-transmit varp (writeVarp early-returned, " +
				"so probe must not fire)")
			return
		}
	}
}

// TestWriteVarp_Probe_SmallValueShape_SignExtension pins the
// P1(uint8(int8(value))) sign-extension path in the OpVarpSmall branch.
// The probe is the binding for Hypothesis C (encoder-byte divergence);
// the original 0-only pin did not exercise negative values.
//
// Varp id 50 (0x32) is used so payload bytes are unambiguous:
//
//	P2(50)  = 0x00 0x32
//	P1(v)   = uint8(int8(v))
func TestWriteVarp_Probe_SmallValueShape_SignExtension(t *testing.T) {
	cases := []struct {
		name      string
		value     int32
		wantLastB byte
	}{
		{"pos_max_int8", 127, 0x7F},
		{"neg_one_sign_ext", -1, 0xFF},
		{"neg_min_int8", -128, 0x80},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, _ := newTestPlayer(t)
			s := newTestServer(t)
			s.varpTypes = &objtype.VarpTypeConfigs{
				Configs: make([]*objtype.VarPlayerType, 51),
				RunID:   50,
			}
			s.varpTypes.Configs[50] = &objtype.VarPlayerType{Transmit: true}
			s.cfg.NodeDebug = true
			rec := &nai138WorldHandler{}
			s.log = slog.New(rec)
			p.client.server = s
			p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

			p.writeVarp(50, tc.value)

			var probe *slog.Record
			for i := range rec.records {
				if rec.records[i].Message == "nai138.write_varp" {
					probe = &rec.records[i]
					break
				}
			}
			if probe == nil {
				t.Fatalf("nai138.write_varp record absent; got %d records", len(rec.records))
			}
			got := recordAttrs138(*probe)

			if got["opcode"] != int64(gameserver.OpVarpSmall.Opcode) {
				t.Errorf(`attr "opcode": got %v, want %d (OpVarpSmall)`,
					got["opcode"], gameserver.OpVarpSmall.Opcode)
			}
			if got["payload_len"] != int64(3) {
				t.Errorf(`attr "payload_len": got %v, want 3`, got["payload_len"])
			}
			// Payload: P2(50) + P1(uint8(int8(value))) = 0x00, 0x32, <last byte>
			wantHex := hex.EncodeToString([]byte{0x00, 0x32, tc.wantLastB})
			if got["payload_hex"] != wantHex {
				t.Errorf(`attr "payload_hex": got %q, want %q (value=%v)`,
					got["payload_hex"], wantHex, tc.value)
			}
		})
	}
}
