package script

import (
	"strings"
	"testing"
)

func TestDisassembleHandRolledScript(t *testing.T) {
	f := &ScriptFile{
		Name:             "[proc,test]",
		SourceFile:       "test.rs2",
		LookupKey:        0xABCD1234,
		IntLocalCount:    2,
		StringLocalCount: 0,
		IntArgCount:      1,
		StringArgCount:   0,
		Opcodes:          []Opcode{OpPushConstantString, OpMes, OpReturn},
		IntOperands:      []int32{0, 0, 0},
		StringOperands:   []string{"hi", "", ""},
	}

	out := Disassemble(f)

	checks := []string{
		"name: [proc,test]",
		"source: test.rs2",
		"lookup_key: 0xabcd1234",
		"int_locals: 2",
		"string_locals: 0",
		`PUSH_CONSTANT_STRING`,
		`"hi"`,
		"MES",
		"RETURN",
	}
	for _, want := range checks {
		if !strings.Contains(out, want) {
			t.Errorf("Disassemble output missing %q\nfull output:\n%s", want, out)
		}
	}
}

func TestDisassembleUnknownOpcode(t *testing.T) {
	f := &ScriptFile{
		Name:           "test",
		SourceFile:     "test.rs2",
		Opcodes:        []Opcode{9999},
		IntOperands:    []int32{0},
		StringOperands: []string{""},
	}

	out := Disassemble(f)
	if !strings.Contains(out, "opcode_9999") {
		t.Errorf("Disassemble should fall back to opcode_9999, got:\n%s", out)
	}
}
