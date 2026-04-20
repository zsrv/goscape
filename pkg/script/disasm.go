package script

import (
	"fmt"
	"strings"
)

// Disassemble formats a ScriptFile as a human-readable listing.
//
// Example output:
//
//	name: [proc,example]
//	source: example.rs2
//	lookup_key: 0xabcd1234
//	int_locals: 2  string_locals: 0  int_args: 1  string_args: 0
//
//	  0:  PUSH_CONSTANT_INT         42
//	  1:  PUSH_CONSTANT_STRING      "hello"
//	  2:  MES                       0
//	  3:  RETURN                    0
func Disassemble(f *ScriptFile) string {
	var b strings.Builder

	fmt.Fprintf(&b, "name: %s\n", f.Name)
	fmt.Fprintf(&b, "source: %s\n", f.SourceFile)
	fmt.Fprintf(&b, "lookup_key: %#x\n", f.LookupKey)
	fmt.Fprintf(&b, "int_locals: %d  string_locals: %d  int_args: %d  string_args: %d\n\n",
		f.IntLocalCount, f.StringLocalCount, f.IntArgCount, f.StringArgCount)

	for i, op := range f.Opcodes {
		if op == OpPushConstantString {
			fmt.Fprintf(&b, "%3d:  %-25s %q\n", i, op.String(), f.StringOperands[i])
		} else {
			fmt.Fprintf(&b, "%3d:  %-25s %d\n", i, op.String(), f.IntOperands[i])
		}
	}

	return b.String()
}
