package script

import (
	"strings"
	"testing"
)

// TestNAI162B0StubsReturnUnimplemented pins the TS-unimplemented stubs.
// PUSH_VARBIT (25) and POP_VARBIT (27) were deleted from the 244 enum
// (ScriptOpcode.ts:20-21) — those stubs are removed.
// Remaining stubs: LC_OP, OC_IOP, OC_OP (TS-unimplemented per NAI-162).
// New 244 stubs: IF_MULTIZONE, IF_OPENMAINOVERLAY, PLAYER_FINDALLZONE,
// PLAYER_FINDNEXT, LAST_COORD (TS-declared with no handler body).
func TestNAI162B0StubsReturnUnimplemented(t *testing.T) {
	cases := []struct {
		name string
		op   Opcode
		want string
	}{
		{"LC_OP", OpLcOp, "LC_OP: unimplemented"},
		{"OC_IOP", OpOcIop, "OC_IOP: unimplemented"},
		{"OC_OP", OpOcOp, "OC_OP: unimplemented"},
		// 244 new stubs
		{"IF_MULTIZONE", OpIfMultizone, "IF_MULTIZONE: unimplemented"},
		{"IF_OPENMAINOVERLAY", OpIfOpenMainOverlay, "IF_OPENMAINOVERLAY: unimplemented"},
		{"PLAYER_FINDALLZONE", OpPlayerFindAllZone, "PLAYER_FINDALLZONE: unimplemented"},
		{"PLAYER_FINDNEXT", OpPlayerFindNext, "PLAYER_FINDNEXT: unimplemented"},
		{"LAST_COORD", OpLastCoord, "LAST_COORD: unimplemented"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			handler, ok := handlers[tc.op]
			if !ok {
				t.Fatalf("opcode %d (%s) has no dispatch entry", tc.op, tc.name)
			}
			s := &ScriptState{}
			err := handler(s)
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q: want substring %q", err.Error(), tc.want)
			}
		})
	}
}
