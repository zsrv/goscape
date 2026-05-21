package script

import (
	"strings"
	"testing"
)

// TestNAI162B0StubsReturnUnimplemented pins the 5 TS-unimplemented
// stubs (PUSH_VARBIT, POP_VARBIT, LC_OP, OC_IOP, OC_OP). Each returns
// an error containing "unimplemented" without mutating any pointer
// state. Mirrors NAI-161 P_OPHELD stub-with-pin shape.
func TestNAI162B0StubsReturnUnimplemented(t *testing.T) {
	cases := []struct {
		name string
		op   Opcode
		want string
	}{
		{"PUSH_VARBIT", OpPushVarbit, "PUSH_VARBIT: unimplemented"},
		{"POP_VARBIT", OpPopVarbit, "POP_VARBIT: unimplemented"},
		{"LC_OP", OpLcOp, "LC_OP: unimplemented"},
		{"OC_IOP", OpOcIop, "OC_IOP: unimplemented"},
		{"OC_OP", OpOcOp, "OC_OP: unimplemented"},
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
