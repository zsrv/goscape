package script

import (
	"fmt"
	"strings"
	"testing"
)

// TestScriptOpcodeMap_LengthParity pins 400 entries at the 274
// pin-advance dee467c8 (adds MAP_LOC, MINIMAP_TOGGLE, SET_SKILL_LEVEL and
// NPC_DESTINATION; renames SETSKINCOLOUR→SETIDKCOLOUR in place — see
// opcode_map_274_pin_test.go for the full table).
// pin-advance 1d25566c (adds P_TEMPRUN, P_TRANSMOGRIFY, DATE_MINUTES and
// DATE_RUNEDAY: 400 -> 404).
// History: 413 at 244 pin 9aadcec4; 414 at 245.2 (adds IF_SETSCROLLPOS);
// 418 at 254 pin 43e02957 (PUSH_VARBIT/POP_VARBIT/STAT_TOTAL/
// SET_PLAYER_OP); 396 at 254 pin 2e3bcf43 (enum restructure 418 -> 396).
func TestScriptOpcodeMap_LengthParity(t *testing.T) {
	const wantLen = 404
	if got := len(ScriptOpcodeMap); got != wantLen {
		t.Fatalf("len(ScriptOpcodeMap) = %d, want %d (re-verify against TS ScriptOpcode.ts at pin 1d25566c)", got, wantLen)
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

// excludedOpcodes lists Op* constants intentionally absent from
// ScriptOpcodeMap (e.g., internal-only opcodes the script source can
// never reference by name). Empty at NAI-202 land. Entries get added
// only with a justifying comment.
var excludedOpcodes = map[Opcode]string{}

// TestScriptOpcodeMap_ReverseCoverage pins: every Op* constant declared
// in pkg/script/opcode.go either appears as a value in ScriptOpcodeMap
// or is explicitly listed in excludedOpcodes with rationale. Catches
// the failure mode where a new Op* constant is added (e.g., during
// NAI-203+ work) without the corresponding ScriptOpcodeMap entry.
//
// Detection strategy: walks the closed range [0, OpTimeSpent] and uses
// Opcode(i).String() — named opcodes return UPPER_SNAKE_CASE; unnamed
// values return "opcode_N". A named opcode missing from both
// ScriptOpcodeMap (values) and excludedOpcodes is a coverage gap.
func TestScriptOpcodeMap_ReverseCoverage(t *testing.T) {
	// Build the set of opcodes present in ScriptOpcodeMap as values.
	mapped := make(map[Opcode]struct{}, len(ScriptOpcodeMap))
	for _, op := range ScriptOpcodeMap {
		mapped[op] = struct{}{}
	}

	missing := []Opcode{}
	// OpTimeSpent (10003) is the highest opcode at the 254 pin 2e3bcf43.
	for i := 0; i <= int(OpTimeSpent); i++ {
		op := Opcode(i)
		name := op.String()
		if strings.HasPrefix(name, "opcode_") {
			continue // unnamed slot in the sparse enum
		}
		if _, ok := mapped[op]; ok {
			continue
		}
		if _, excluded := excludedOpcodes[op]; excluded {
			continue
		}
		missing = append(missing, op)
	}

	if len(missing) > 0 {
		// Format the missing list for the failure message.
		lines := make([]string, 0, len(missing))
		for _, op := range missing {
			lines = append(lines, fmt.Sprintf("\t%s (Opcode=%d)", op.String(), uint16(op)))
		}
		t.Fatalf("ReverseCoverage: %d named Op* constants are absent from ScriptOpcodeMap AND not listed in excludedOpcodes:\n%s\n\nFix: either add the entry to ScriptOpcodeMap (preferred — opcodes are reachable from script source) or add an excludedOpcodes entry with a justifying comment.", len(missing), strings.Join(lines, "\n"))
	}
}
