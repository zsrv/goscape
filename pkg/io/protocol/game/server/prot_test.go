package server

import "testing"

func TestServerOpValues(t *testing.T) {
	cases := []struct {
		op     Op
		opcode byte
		size   int
	}{
		{OpIfClose, 129, 0},
		{OpIfOpenMain, 168, 2},
		{OpIfOpenChat, 14, 2},
		{OpIfOpenSide, 195, 2},
		{OpIfOpenMainSide, 28, 4},
		{OpLogout, 142, 0},
	}
	for _, tc := range cases {
		if tc.op.Opcode != tc.opcode {
			t.Errorf("%v: Opcode = %d, want %d", tc.op, tc.op.Opcode, tc.opcode)
		}
		if tc.op.PayloadSize != tc.size {
			t.Errorf("%v: PayloadSize = %d, want %d", tc.op, tc.op.PayloadSize, tc.size)
		}
	}
}

func TestSubSpec3AOpcodes(t *testing.T) {
	cases := []struct {
		op     Op
		opcode byte
		size   int
	}{
		{OpRebuildNormal, 237, -2},
		{OpUpdateInvFull, 98, -2},
		{OpUpdateInvPartial, 213, -2},
	}
	for _, tc := range cases {
		if tc.op.Opcode != tc.opcode {
			t.Errorf("%+v: Opcode = %d, want %d", tc.op, tc.op.Opcode, tc.opcode)
		}
		if tc.op.PayloadSize != tc.size {
			t.Errorf("%+v: PayloadSize = %d, want %d", tc.op, tc.op.PayloadSize, tc.size)
		}
	}
}

func TestSubSpec3BOpcodes(t *testing.T) {
	if OpPlayerInfo.Opcode != 184 {
		t.Errorf("OpPlayerInfo.Opcode = %d, want 184", OpPlayerInfo.Opcode)
	}
	if OpPlayerInfo.PayloadSize != -2 {
		t.Errorf("OpPlayerInfo.PayloadSize = %d, want -2", OpPlayerInfo.PayloadSize)
	}
}
