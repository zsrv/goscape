package objtype

import (
	"testing"

	packet2 "github.com/zsrv/goscape/pkg/io/packet"
)

func TestHuntTypeDefaults(t *testing.T) {
	ht := NewHuntType(42)

	if ht.ID != 42 {
		t.Errorf("ID: got %d, want 42", ht.ID)
	}
	if ht.Type != HuntModeOff {
		t.Errorf("Type: got %d, want HuntModeOff (%d)", ht.Type, HuntModeOff)
	}
	if ht.CheckVis != HuntVisOff {
		t.Errorf("CheckVis: got %d, want HuntVisOff", ht.CheckVis)
	}
	if ht.CheckNotTooStrong != HuntCheckNotTooStrongOff {
		t.Errorf("CheckNotTooStrong: got %d, want HuntCheckNotTooStrongOff", ht.CheckNotTooStrong)
	}
	if ht.CheckNotBusy {
		t.Errorf("CheckNotBusy: got true, want false")
	}
	if ht.FindKeepHunting {
		t.Errorf("FindKeepHunting: got true, want false")
	}
	if ht.FindNewMode != NPCModeNull {
		t.Errorf("FindNewMode: got %d, want NPCModeNull (%d)", ht.FindNewMode, NPCModeNull)
	}
	if ht.NobodyNear != HuntNobodyNearPauseHunt {
		t.Errorf("NobodyNear: got %d, want HuntNobodyNearPauseHunt", ht.NobodyNear)
	}
	if ht.CheckNotCombat != -1 {
		t.Errorf("CheckNotCombat: got %d, want -1", ht.CheckNotCombat)
	}
	if ht.CheckNotCombatSelf != -1 {
		t.Errorf("CheckNotCombatSelf: got %d, want -1", ht.CheckNotCombatSelf)
	}
	if !ht.CheckAfk {
		t.Errorf("CheckAfk: got false, want true")
	}
	if ht.Rate != 1 {
		t.Errorf("Rate: got %d, want 1", ht.Rate)
	}
	for name, got := range map[string]int{
		"CheckCategory": ht.CheckCategory,
		"CheckNpc":      ht.CheckNpc,
		"CheckObj":      ht.CheckObj,
		"CheckLoc":      ht.CheckLoc,
		"CheckInv":      ht.CheckInv,
		"CheckObjParam": ht.CheckObjParam,
		"CheckInvVal":   ht.CheckInvVal,
	} {
		if got != -1 {
			t.Errorf("%s: got %d, want -1", name, got)
		}
	}
	if ht.CheckInvCondition != "" {
		t.Errorf("CheckInvCondition: got %q, want empty", ht.CheckInvCondition)
	}
	if ht.CheckVars != nil {
		t.Errorf("CheckVars: got %v, want nil", ht.CheckVars)
	}

}

func TestHuntTypeDecodeAllOpcodes(t *testing.T) {
	tests := []struct {
		name   string
		build  func(*packet2.Packet)
		verify func(*testing.T, *HuntType)
	}{
		{
			name:  "code 1 Type",
			build: func(p *packet2.Packet) { p.P1(1); p.P1(uint8(HuntModePlayer)) },
			verify: func(t *testing.T, ht *HuntType) {
				if ht.Type != HuntModePlayer {
					t.Errorf("Type: got %d, want HuntModePlayer", ht.Type)
				}
			},
		},
		{
			name:  "code 2 CheckVis",
			build: func(p *packet2.Packet) { p.P1(2); p.P1(uint8(HuntVisLineOfSight)) },
			verify: func(t *testing.T, ht *HuntType) {
				if ht.CheckVis != HuntVisLineOfSight {
					t.Errorf("CheckVis: got %d, want HuntVisLineOfSight", ht.CheckVis)
				}
			},
		},
		{
			name:  "code 3 CheckNotTooStrong",
			build: func(p *packet2.Packet) { p.P1(3); p.P1(uint8(HuntCheckNotTooStrongOutsideWilderness)) },
			verify: func(t *testing.T, ht *HuntType) {
				if ht.CheckNotTooStrong != HuntCheckNotTooStrongOutsideWilderness {
					t.Errorf("CheckNotTooStrong: got %d", ht.CheckNotTooStrong)
				}
			},
		},
		{
			name:  "code 4 CheckNotBusy",
			build: func(p *packet2.Packet) { p.P1(4) },
			verify: func(t *testing.T, ht *HuntType) {
				if !ht.CheckNotBusy {
					t.Errorf("CheckNotBusy: got false, want true")
				}
			},
		},
		{
			name:  "code 5 FindKeepHunting",
			build: func(p *packet2.Packet) { p.P1(5) },
			verify: func(t *testing.T, ht *HuntType) {
				if !ht.FindKeepHunting {
					t.Errorf("FindKeepHunting: got false, want true")
				}
			},
		},
		{
			name:  "code 6 FindNewMode",
			build: func(p *packet2.Packet) { p.P1(6); p.P1(uint8(NPCModeWander)) },
			verify: func(t *testing.T, ht *HuntType) {
				if ht.FindNewMode != NPCModeWander {
					t.Errorf("FindNewMode: got %d", ht.FindNewMode)
				}
			},
		},
		{
			name:  "code 7 NobodyNear",
			build: func(p *packet2.Packet) { p.P1(7); p.P1(uint8(HuntNobodyNearKeepHunting)) },
			verify: func(t *testing.T, ht *HuntType) {
				if ht.NobodyNear != HuntNobodyNearKeepHunting {
					t.Errorf("NobodyNear: got %d", ht.NobodyNear)
				}
			},
		},
		{
			name:  "code 8 CheckNotCombat",
			build: func(p *packet2.Packet) { p.P1(8); p.P2(1234) },
			verify: func(t *testing.T, ht *HuntType) {
				if ht.CheckNotCombat != 1234 {
					t.Errorf("CheckNotCombat: got %d", ht.CheckNotCombat)
				}
			},
		},
		{
			name:  "code 9 CheckNotCombatSelf",
			build: func(p *packet2.Packet) { p.P1(9); p.P2(5678) },
			verify: func(t *testing.T, ht *HuntType) {
				if ht.CheckNotCombatSelf != 5678 {
					t.Errorf("CheckNotCombatSelf: got %d", ht.CheckNotCombatSelf)
				}
			},
		},
		{
			name:  "code 10 CheckAfk=false",
			build: func(p *packet2.Packet) { p.P1(10) },
			verify: func(t *testing.T, ht *HuntType) {
				if ht.CheckAfk {
					t.Errorf("CheckAfk: got true, want false")
				}
			},
		},
		{
			name:  "code 11 Rate",
			build: func(p *packet2.Packet) { p.P1(11); p.P2(42) },
			verify: func(t *testing.T, ht *HuntType) {
				if ht.Rate != 42 {
					t.Errorf("Rate: got %d", ht.Rate)
				}
			},
		},
		{
			name:  "code 12 CheckCategory",
			build: func(p *packet2.Packet) { p.P1(12); p.P2(7) },
			verify: func(t *testing.T, ht *HuntType) {
				if ht.CheckCategory != 7 {
					t.Errorf("CheckCategory: got %d", ht.CheckCategory)
				}
			},
		},
		{
			name:  "code 13 CheckNpc",
			build: func(p *packet2.Packet) { p.P1(13); p.P2(99) },
			verify: func(t *testing.T, ht *HuntType) {
				if ht.CheckNpc != 99 {
					t.Errorf("CheckNpc: got %d", ht.CheckNpc)
				}
			},
		},
		{
			name:  "code 14 CheckObj",
			build: func(p *packet2.Packet) { p.P1(14); p.P2(100) },
			verify: func(t *testing.T, ht *HuntType) {
				if ht.CheckObj != 100 {
					t.Errorf("CheckObj: got %d", ht.CheckObj)
				}
			},
		},
		{
			name:  "code 15 CheckLoc",
			build: func(p *packet2.Packet) { p.P1(15); p.P2(55) },
			verify: func(t *testing.T, ht *HuntType) {
				if ht.CheckLoc != 55 {
					t.Errorf("CheckLoc: got %d", ht.CheckLoc)
				}
			},
		},
		{
			name: "code 16 CheckInv + CheckObj + cond + val",
			build: func(p *packet2.Packet) {
				p.P1(16)
				p.P2(10) // CheckInv
				p.P2(20) // CheckObj
				p.PJStrLF(">")
				v := int32(-5)
				p.P4(uint32(v)) // signed -5
			},
			verify: func(t *testing.T, ht *HuntType) {
				if ht.CheckInv != 10 {
					t.Errorf("CheckInv: got %d", ht.CheckInv)
				}
				if ht.CheckObj != 20 {
					t.Errorf("CheckObj: got %d", ht.CheckObj)
				}
				if ht.CheckInvCondition != ">" {
					t.Errorf("CheckInvCondition: got %q", ht.CheckInvCondition)
				}
				if ht.CheckInvVal != -5 {
					t.Errorf("CheckInvVal: got %d", ht.CheckInvVal)
				}
			},
		},
		{
			name: "code 17 CheckInv + CheckObjParam + cond + val",
			build: func(p *packet2.Packet) {
				p.P1(17)
				p.P2(11) // CheckInv
				p.P2(22) // CheckObjParam
				p.PJStrLF("<")
				p.P4(uint32(int32(7)))
			},
			verify: func(t *testing.T, ht *HuntType) {
				if ht.CheckInv != 11 {
					t.Errorf("CheckInv: got %d", ht.CheckInv)
				}
				if ht.CheckObjParam != 22 {
					t.Errorf("CheckObjParam: got %d", ht.CheckObjParam)
				}
				if ht.CheckInvCondition != "<" {
					t.Errorf("CheckInvCondition: got %q", ht.CheckInvCondition)
				}
				if ht.CheckInvVal != 7 {
					t.Errorf("CheckInvVal: got %d", ht.CheckInvVal)
				}
			},
		},
		{
			name: "code 18 single CheckVar",
			build: func(p *packet2.Packet) {
				p.P1(18)
				p.P2(33) // VarID
				p.PJStrLF("=")
				v := int32(-1)
				p.P4(uint32(v))
			},
			verify: func(t *testing.T, ht *HuntType) {
				if len(ht.CheckVars) != 1 {
					t.Fatalf("CheckVars: got %d entries, want 1", len(ht.CheckVars))
				}
				v := ht.CheckVars[0]
				if v.VarID != 33 || v.Condition != "=" || v.Val != -1 {
					t.Errorf("CheckVars[0]: got %+v", v)
				}
			},
		},
		{
			name:  "code 250 DebugName",
			build: func(p *packet2.Packet) { p.P1(250); p.PJStrLF("boss_hunt") },
			verify: func(t *testing.T, ht *HuntType) {
				if ht.DebugName != "boss_hunt" {
					t.Errorf("DebugName: got %q", ht.DebugName)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pkt := packet2.NewPacket(nil)
			tc.build(pkt)
			pkt.P1(0) // terminator

			reader := packet2.NewPacket(pkt.Bytes())
			ht := NewHuntType(0)
			if err := DecodeType(reader, ht); err != nil {
				t.Fatalf("DecodeType: %v", err)
			}
			tc.verify(t, ht)
		})
	}
}

func TestHuntTypeDecodeUnknownOpcode(t *testing.T) {
	pkt := packet2.NewPacket(nil)
	pkt.P1(42) // undefined opcode
	reader := packet2.NewPacket(pkt.Bytes())

	err := DecodeType(reader, NewHuntType(0))
	if err == nil {
		t.Fatalf("DecodeType: want error, got nil")
	}
	if got := err.Error(); got != "unrecognized hunt config code 42" {
		t.Errorf("error message: got %q", got)
	}
}

func TestHuntTypeDecodeCheckVarsAppend(t *testing.T) {
	pkt := packet2.NewPacket(nil)
	// Three consecutive CheckVars via codes 18, 19, 20.
	for i, code := range []uint8{18, 19, 20} {
		pkt.P1(code)
		pkt.P2(uint16(100 + i))
		pkt.PJStrLF("=")
		pkt.P4(uint32(int32(i + 1)))
	}
	pkt.P1(0) // terminator

	reader := packet2.NewPacket(pkt.Bytes())
	ht := NewHuntType(0)
	if err := DecodeType(reader, ht); err != nil {
		t.Fatalf("DecodeType: %v", err)
	}
	if len(ht.CheckVars) != 3 {
		t.Fatalf("CheckVars: got %d entries, want 3", len(ht.CheckVars))
	}
	for i, v := range ht.CheckVars {
		wantID := 100 + i
		wantVal := i + 1
		if v.VarID != wantID || v.Condition != "=" || v.Val != wantVal {
			t.Errorf("CheckVars[%d]: got %+v, want {VarID:%d Cond:= Val:%d}", i, v, wantID, wantVal)
		}
	}
}
