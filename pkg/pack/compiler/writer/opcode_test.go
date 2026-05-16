// pkg/pack/compiler/writer/opcode_test.go
package writer

import "testing"

// TestServerScriptOpcode_IDs pins the numeric ID of every opcode singleton.
// Verbatim from TS src/runescript/ServerScriptOpcode.ts.
func TestServerScriptOpcode_IDs(t *testing.T) {
	cases := []struct {
		name  string
		op    *ServerScriptOpcode
		id    uint16
		large bool
	}{
		{"PushConstantInt", OpPushConstantInt, 0, true},
		{"PushVarp", OpPushVarp, 1, true},
		{"PopVarp", OpPopVarp, 2, true},
		{"PushConstantString", OpPushConstantString, 3, true},
		{"PushVarn", OpPushVarn, 4, true},
		{"PopVarn", OpPopVarn, 5, true},
		{"Branch", OpBranch, 6, true},
		{"BranchNot", OpBranchNot, 7, true},
		{"BranchEquals", OpBranchEquals, 8, true},
		{"BranchLessThan", OpBranchLessThan, 9, true},
		{"BranchGreaterThan", OpBranchGreaterThan, 10, true},
		{"PushVars", OpPushVars, 11, true},
		{"PopVars", OpPopVars, 12, true},
		{"Return", OpReturn, 21, false},
		{"Gosub", OpGosub, 22, false},
		{"Jump", OpJump, 23, false},
		{"Switch", OpSwitch, 24, true},
		{"PushVarbit", OpPushVarbit, 25, true},
		{"PopVarbit", OpPopVarbit, 27, true},
		{"BranchLessThanOrEquals", OpBranchLessThanOrEquals, 31, true},
		{"BranchGreaterThanOrEquals", OpBranchGreaterThanOrEquals, 32, true},
		{"PushIntLocal", OpPushIntLocal, 33, true},
		{"PopIntLocal", OpPopIntLocal, 34, true},
		{"PushStringLocal", OpPushStringLocal, 35, true},
		{"PopStringLocal", OpPopStringLocal, 36, true},
		{"JoinString", OpJoinString, 37, true},
		{"PopIntDiscard", OpPopIntDiscard, 38, false},
		{"PopStringDiscard", OpPopStringDiscard, 39, false},
		{"GosubWithParams", OpGosubWithParams, 40, true},
		{"JumpWithParams", OpJumpWithParams, 41, true},
		{"DefineArray", OpDefineArray, 44, true},
		{"PushArrayInt", OpPushArrayInt, 45, true},
		{"PopArrayInt", OpPopArrayInt, 46, true},
		{"Add", OpAdd, 4600, false},
		{"Sub", OpSub, 4601, false},
		{"Multiply", OpMultiply, 4602, false},
		{"Divide", OpDivide, 4603, false},
		{"Modulo", OpModulo, 4611, false},
		{"And", OpAnd, 4614, false},
		{"Or", OpOr, 4615, false},
	}
	for _, c := range cases {
		if c.op == nil {
			t.Errorf("%s: singleton is nil", c.name)
			continue
		}
		if c.op.ID != c.id {
			t.Errorf("%s.ID = %d, want %d", c.name, c.op.ID, c.id)
		}
		if c.op.LargeOperand != c.large {
			t.Errorf("%s.LargeOperand = %v, want %v", c.name, c.op.LargeOperand, c.large)
		}
	}
	if len(All) != len(cases) {
		t.Errorf("len(All) = %d, want %d", len(All), len(cases))
	}
}

// TestServerScriptOpcode_AllUniqueIDs pins that no two singletons share an ID.
func TestServerScriptOpcode_AllUniqueIDs(t *testing.T) {
	seen := map[uint16]int{} // ID → first index in All
	for i, op := range All {
		if first, ok := seen[op.ID]; ok {
			t.Errorf("duplicate ID %d: All[%d] and All[%d]", op.ID, first, i)
		}
		seen[op.ID] = i
	}
}
