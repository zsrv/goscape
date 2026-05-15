package script

import (
	"strings"
	"testing"
)

// TestScriptOpcodeMap_LengthParity pins spec §7.4: 393 entries verified
// at plan-write against TS ScriptOpcode.ts via
//
//	awk '/^export const ScriptOpcodeMap/,/^]\)/' ScriptOpcode.ts |
//	grep -c "^\s*\['"
//
// If TS upstream adds opcodes, this count rises and the test fails —
// implementer updates the count after re-running the awk against
// LostCityRS/Engine-TS HEAD.
func TestScriptOpcodeMap_LengthParity(t *testing.T) {
	const wantLen = 393
	if got := len(ScriptOpcodeMap); got != wantLen {
		t.Fatalf("len(ScriptOpcodeMap) = %d, want %d (re-verify against TS ScriptOpcode.ts:445)", got, wantLen)
	}
}

// TestScriptOpcodeMap_NoDuplicates pins spec §7.6: no two distinct
// uppercase names map to the same Opcode value. Catches copy-paste
// regression during the 393-entry literal port.
func TestScriptOpcodeMap_NoDuplicates(t *testing.T) {
	seen := make(map[Opcode]string, len(ScriptOpcodeMap))
	for name, op := range ScriptOpcodeMap {
		if other, dup := seen[op]; dup {
			t.Errorf("Opcode %d mapped from BOTH %q and %q", op, other, name)
		}
		seen[op] = name
	}
}

// TestScriptOpcodeMap_NamesUppercase pins the convention that every
// key is ALL-UPPERCASE (no mixed case). TS uses UPPER_SNAKE_CASE
// uniformly; goscape mirrors. Catches typos like "Push_constant_int".
func TestScriptOpcodeMap_NamesUppercase(t *testing.T) {
	for name := range ScriptOpcodeMap {
		if name != strings.ToUpper(name) {
			t.Errorf("name %q is not uppercase (TS source is UPPER_SNAKE_CASE)", name)
		}
		if name == "" {
			t.Errorf("empty key in ScriptOpcodeMap")
		}
	}
}

// TestScriptOpcodeMap_SpotChecks pins spec §7.5: ~13 representative
// entries from across the file.
func TestScriptOpcodeMap_SpotChecks(t *testing.T) {
	cases := []struct {
		name string
		want Opcode
	}{
		{"PUSH_CONSTANT_INT", OpPushConstantInt},
		{"PUSH_CONSTANT_STRING", OpPushConstantString},
		{"BRANCH", OpBranch},
		{"RETURN", OpReturn},
		{"GOSUB", OpGosub},
		{"JUMP", OpJump},
		{"ALLOWDESIGN", OpAllowDesign},
		{"ANIM", OpAnim},
		{"FINDUID", OpFindUID},
		{"HUNTALL", OpHuntAll},
		{"HUNTNEXT", OpHuntNext},
		{"GETTIMESPENT", OpGetTimeSpent},
		{"TIMESPENT", OpTimeSpent},
	}
	for _, c := range cases {
		got, present := ScriptOpcodeMap[c.name]
		if !present {
			t.Errorf("ScriptOpcodeMap[%q]: missing", c.name)
			continue
		}
		if got != c.want {
			t.Errorf("ScriptOpcodeMap[%q]: got %d, want %d", c.name, got, c.want)
		}
	}
}
