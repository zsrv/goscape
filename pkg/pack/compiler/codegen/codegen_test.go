// pkg/pack/compiler/codegen/codegen_test.go
package codegen

import (
	"testing"

	"github.com/zsrv/goscape/pkg/pack/compiler/diagnostics"
	"github.com/zsrv/goscape/pkg/pack/compiler/parser"
	"github.com/zsrv/goscape/pkg/pack/compiler/semantics"
	"github.com/zsrv/goscape/pkg/pack/compiler/symbol"
	"github.com/zsrv/goscape/pkg/pack/compiler/trigger"
	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

// makeProcTrigger is the codegen-test analogue of semantics' helper.
// Cannot import the semantics test fixture (it's package-private).
func makeProcTrigger() *trigger.TriggerType {
	return &trigger.TriggerType{
		ID:              0,
		Identifier:      "proc",
		SubjectMode:     trigger.ModeName,
		AllowParameters: true,
		AllowReturns:    true,
	}
}

// compileForTest parses + registers + type-checks + codegens src.
// Fatals if parse, diagnostics, or script count is unexpected.
// Mirrors semantics' newTestFixture / smoke_test pattern.
func compileForTest(t *testing.T, src string) *RuneScript {
	t.Helper()
	tm := typ.NewTypeManager()
	for _, p := range typ.PrimitiveAll {
		_ = tm.RegisterByRepresentation(p)
	}
	// Identity checker: identical types are assignable. Required for the type
	// checker to accept `int = int`, `int < int`, etc. in conditions.
	// Mirrors semantics.newBasicCheckingFixture (type_checking_test.go L23).
	tm.AddTypeChecker(func(left, right typ.Type) bool { return left == right })
	trm := trigger.NewTriggerManager()
	_ = trm.RegisterTrigger(makeProcTrigger())
	root := symbol.NewSymbolTable(nil)
	d := &diagnostics.Diagnostics{}

	p := parser.NewScriptFileParser(src, "test.rs2")
	sf := p.ParseScriptFile()
	if sf == nil {
		t.Fatalf("parse failed (nil ScriptFile)")
	}

	sr := semantics.NewScriptRegistration(tm, trm, root, d, semantics.StrictFeatureLevel{})
	sr.Visit(sf)
	tc := semantics.NewTypeChecker(tm, trm, root, map[string]semantics.DynamicCommandHandler{}, d, semantics.StrictFeatureLevel{})
	tc.Visit(sf)

	cg := NewCodeGenerator(root, map[string]semantics.DynamicCommandHandler{}, d)
	cg.Visit(sf)

	if d.HasErrors() {
		t.Fatalf("unexpected diagnostics: %+v", d.List())
	}
	scripts := cg.Scripts()
	if len(scripts) != 1 {
		t.Fatalf("Scripts: got %d, want 1", len(scripts))
	}
	return scripts[0]
}

// compileForTestOptional is like compileForTest but returns nil instead of
// fataling when there are zero codegen scripts (e.g. command-trigger skip).
func compileForTestOptional(t *testing.T, src string) *RuneScript {
	t.Helper()
	tm := typ.NewTypeManager()
	for _, p := range typ.PrimitiveAll {
		_ = tm.RegisterByRepresentation(p)
	}
	tm.AddTypeChecker(func(left, right typ.Type) bool { return left == right })
	trm := trigger.NewTriggerManager()
	_ = trm.RegisterTrigger(makeProcTrigger())
	// Register CommandTrigger for the skip test.
	_ = trm.RegisterTrigger(trigger.CommandTrigger)
	root := symbol.NewSymbolTable(nil)
	d := &diagnostics.Diagnostics{}

	p := parser.NewScriptFileParser(src, "test.rs2")
	sf := p.ParseScriptFile()
	if sf == nil {
		t.Fatalf("parse failed (nil ScriptFile)")
	}

	sr := semantics.NewScriptRegistration(tm, trm, root, d, semantics.StrictFeatureLevel{})
	sr.Visit(sf)
	tc := semantics.NewTypeChecker(tm, trm, root, map[string]semantics.DynamicCommandHandler{}, d, semantics.StrictFeatureLevel{})
	tc.Visit(sf)

	cg := NewCodeGenerator(root, map[string]semantics.DynamicCommandHandler{}, d)
	cg.Visit(sf)

	if d.HasErrors() {
		t.Fatalf("unexpected diagnostics: %+v", d.List())
	}
	if got := len(cg.Scripts()); got == 0 {
		return nil
	}
	return cg.Scripts()[0]
}

// --- helpers ---------------------------------------------------------------

func opcodesOf(b *Block) []Opcode {
	out := make([]Opcode, 0, len(b.Instructions))
	for _, in := range b.Instructions {
		out = append(out, in.Opcode)
	}
	return out
}

func sameOps(a, b []Opcode) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func names(ops []Opcode) []string {
	out := make([]string, len(ops))
	for i, o := range ops {
		out[i] = o.Name
	}
	return out
}

// --- tests -----------------------------------------------------------------

// TestCodeGenerator_EmptyScript pins the smallest valid output: one script with
// one entry block containing exactly [Return]. No declared return type → only
// Return is emitted (no default-value push).
func TestCodeGenerator_EmptyScript(t *testing.T) {
	src := "[proc,foo]\n"
	rs := compileForTest(t, src)

	if rs.FullName != "[proc,foo]" {
		t.Errorf("FullName: got %q, want %q", rs.FullName, "[proc,foo]")
	}
	if len(rs.Blocks) != 1 {
		t.Fatalf("Blocks: got %d, want 1", len(rs.Blocks))
	}
	if rs.Blocks[0].Label.Name != "entry" {
		t.Errorf("entry-block label: got %q, want %q", rs.Blocks[0].Label.Name, "entry")
	}
	if got := len(rs.Blocks[0].Instructions); got != 1 {
		t.Fatalf("entry-block insn count: got %d, want 1 (just Return)", got)
	}
	if rs.Blocks[0].Instructions[0].Opcode != Return {
		t.Errorf("entry-block insn[0]: got %v, want Return", rs.Blocks[0].Instructions[0].Opcode)
	}
}

// TestCodeGenerator_DefaultReturns pins default returns for each return type.
//
// "int" → [PushConstantInt(0), Return]   (special-cased to 0, not -1)
// "string" → [PushConstantString(""), Return]
// "coord" → [PushConstantInt(-1), Return]  (BaseVarInteger, default -1)
//
// Grammar note: `[proc,foo]()(int)` is params=empty, returns=int.
// `[proc,foo](int)` would be parsed as a parameter list (IDENTIFIER after
// first LPAREN → isParamList=true), so we use `()` to prefix the return list.
//
// Note: "obj" is not registered in the minimal fixture (PrimitiveAll has no obj).
// "coord" is used as the BaseVarInteger-with-default-(-1) representative.
func TestCodeGenerator_DefaultReturns(t *testing.T) {
	cases := []struct {
		name string
		ret  string // RuneScript return-type declaration (second parens group)
		want []Opcode
	}{
		{"int returns 0", "()(int)", []Opcode{PushConstantInt, Return}},
		{"string returns empty string", "()(string)", []Opcode{PushConstantString, Return}},
		{"coord returns -1 (BaseVarInteger non-int)", "()(coord)", []Opcode{PushConstantInt, Return}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			src := "[proc,foo]" + c.ret + "\n"
			rs := compileForTest(t, src)
			if len(rs.Blocks) != 1 {
				t.Fatalf("Blocks: got %d, want 1", len(rs.Blocks))
			}
			got := opcodesOf(rs.Blocks[0])
			if !sameOps(got, c.want) {
				t.Errorf("opcodes: got %v, want %v", names(got), names(c.want))
			}
		})
	}
}

// TestCodeGenerator_Parameters pins that parameter symbols populate LocalTable.
func TestCodeGenerator_Parameters(t *testing.T) {
	src := "[proc,foo](int $a, string $b)\n"
	rs := compileForTest(t, src)
	if got := len(rs.Locals.Parameters); got != 2 {
		t.Fatalf("Parameters: got %d, want 2", got)
	}
	if got := len(rs.Locals.All); got != 2 {
		t.Fatalf("All: got %d, want 2 (parameters are also in All)", got)
	}
}

// TestCodeGenerator_SkipsCommandScripts pins that scripts with trigger==command
// are skipped by Visit. TS CodeGenerator.visitScript guards on
// `script.triggerType == CommandTrigger`.
func TestCodeGenerator_SkipsCommandScripts(t *testing.T) {
	// Grammar: [command,foo]()(int) — empty params, returns int.
	// Use `()` prefix so the return type is correctly parsed as a type list
	// not a parameter list (LPAREN+RPAREN → isParamList=true with empty list;
	// second LPAREN then holds the type list).
	src := "[command,foo]()(int)\n"
	rs := compileForTestOptional(t, src)
	if rs != nil {
		t.Fatalf("Scripts: want 0 (command-trigger skipped), got 1: %v", rs)
	}
}

// TestCodeGenerator_ReturnEmpty pins `return;` ⇒ Return only.
func TestCodeGenerator_ReturnEmpty(t *testing.T) {
	rs := compileForTest(t, "[proc,foo]\nreturn;\n")
	got := opcodesOf(rs.Blocks[0])
	want := []Opcode{Return, Return} // explicit return + generateDefaultReturns trailing
	if !sameOps(got, want) {
		t.Errorf("opcodes: got %v, want %v", names(got), names(want))
	}
}

// TestCodeGenerator_ReturnExpression pins `return(42);`.
func TestCodeGenerator_ReturnExpression(t *testing.T) {
	rs := compileForTest(t, "[proc,foo]()(int)\nreturn(42);\n")
	got := opcodesOf(rs.Blocks[0])
	// PushConstantInt(42) Return PushConstantInt(0) Return
	want := []Opcode{PushConstantInt, Return, PushConstantInt, Return}
	if !sameOps(got, want) {
		t.Errorf("opcodes: got %v, want %v", names(got), names(want))
	}
}

// TestCodeGenerator_IfElse_StraightCondition pins the if/else block layout.
func TestCodeGenerator_IfElse_StraightCondition(t *testing.T) {
	src := `[proc,foo]
if (1 = 2) {
} else {
}
`
	rs := compileForTest(t, src)
	// Expected blocks: entry, if_true_0, if_else_0, if_end_0
	if len(rs.Blocks) != 4 {
		t.Fatalf("Blocks: got %d, want 4 (entry+if_true+if_else+if_end)", len(rs.Blocks))
	}
	labels := []string{"entry", "if_true_0", "if_else_0", "if_end_0"}
	for i, name := range labels {
		if rs.Blocks[i].Label.Name != name {
			t.Errorf("Blocks[%d].Label: got %q, want %q", i, rs.Blocks[i].Label.Name, name)
		}
	}
	// entry block: PushConstantInt 1, PushConstantInt 2, BranchEquals if_true_0, Branch if_else_0
	got := opcodesOf(rs.Blocks[0])
	want := []Opcode{PushConstantInt, PushConstantInt, BranchEquals, Branch}
	if !sameOps(got, want) {
		t.Errorf("entry opcodes: got %v, want %v", names(got), names(want))
	}
}

// TestCodeGenerator_While pins the loop block layout.
func TestCodeGenerator_While(t *testing.T) {
	src := `[proc,foo]
while (1 < 2) {
}
`
	rs := compileForTest(t, src)
	// Expected blocks: entry, while_start_0, while_body_0, while_end_0
	if len(rs.Blocks) != 4 {
		t.Fatalf("Blocks: got %d, want 4 (entry+while_start+while_body+while_end)", len(rs.Blocks))
	}
	labels := []string{"entry", "while_start_0", "while_body_0", "while_end_0"}
	for i, name := range labels {
		if rs.Blocks[i].Label.Name != name {
			t.Errorf("Blocks[%d].Label: got %q, want %q", i, rs.Blocks[i].Label.Name, name)
		}
	}
	// while_body: Branch while_start_0 (back-edge)
	bodyOps := opcodesOf(rs.Blocks[2])
	if len(bodyOps) != 1 || bodyOps[0] != Branch {
		t.Errorf("while_body opcodes: got %v, want [Branch]", names(bodyOps))
	}
}

// TestCodeGenerator_GenerateCondition_LogicalAnd pins the recursive chain.
func TestCodeGenerator_GenerateCondition_LogicalAnd(t *testing.T) {
	src := `[proc,foo]
if (1 = 1 & 2 = 2) {
}
`
	rs := compileForTest(t, src)
	// Expected blocks: entry, condition_and_0, if_true_0, if_end_0
	labels := []string{"entry", "condition_and_0", "if_true_0", "if_end_0"}
	if len(rs.Blocks) != len(labels) {
		t.Fatalf("Blocks: got %d, want %d", len(rs.Blocks), len(labels))
	}
	for i, name := range labels {
		if rs.Blocks[i].Label.Name != name {
			t.Errorf("Blocks[%d].Label: got %q, want %q", i, rs.Blocks[i].Label.Name, name)
		}
	}
}

// TestCodeGenerator_Declaration_WithInitializer pins `def_int $x = 42;`.
func TestCodeGenerator_Declaration_WithInitializer(t *testing.T) {
	src := "[proc,foo]\ndef_int $x = 42;\n"
	rs := compileForTest(t, src)
	got := opcodesOf(rs.Blocks[0])
	want := []Opcode{PushConstantInt, PopLocalVar, Return}
	if !sameOps(got, want) {
		t.Errorf("opcodes: got %v, want %v", names(got), names(want))
	}
	// The local symbol is recorded in LocalTable.
	if got := len(rs.Locals.All); got != 1 {
		t.Fatalf("Locals.All: got %d, want 1", got)
	}
}

// TestCodeGenerator_Declaration_NoInitializer_IntDefault pins `def_int $x;`.
// No-initializer int default is 0 (typ.PrimitiveInt.DefaultValue==0).
func TestCodeGenerator_Declaration_NoInitializer_IntDefault(t *testing.T) {
	src := "[proc,foo]\ndef_int $x;\n"
	rs := compileForTest(t, src)
	got := opcodesOf(rs.Blocks[0])
	want := []Opcode{PushConstantInt, PopLocalVar, Return}
	if !sameOps(got, want) {
		t.Errorf("opcodes: got %v, want %v", names(got), names(want))
	}
	if rs.Blocks[0].Instructions[0].Operand.(int) != 0 {
		t.Errorf("default-int operand: got %v, want 0", rs.Blocks[0].Instructions[0].Operand)
	}
}

// TestCodeGenerator_ArrayDeclaration pins array declaration codegen.
//
// NAI-207-D-ARRAY-DECL-SYNTAX: The plan prescribed `def_intarray $arr(10)` as
// the test source. The RuneScript parser tokenises `def_intarray` as DEF_TYPE
// and the type-checker strips the `def_` prefix, yielding typeName="intarray".
// TypeManager.FindOrNil("intarray", allowArray=false) returns nil (the name-map
// only holds the base-type names; array expansion requires allowArray=true which
// the declaration checker doesn't pass). The correct source syntax is
// `def_int $arr(10)` — the DEF_TYPE carries the base-type name; the LPAREN
// suffix is what signals an array declaration to the parser.
func TestCodeGenerator_ArrayDeclaration(t *testing.T) {
	src := "[proc,foo]\ndef_int $arr(10);\n"
	rs := compileForTest(t, src)
	got := opcodesOf(rs.Blocks[0])
	// Visit init (PushConstantInt 10) then DefineArray.
	want := []Opcode{PushConstantInt, DefineArray, Return}
	if !sameOps(got, want) {
		t.Errorf("opcodes: got %v, want %v", names(got), names(want))
	}
}

// TestCodeGenerator_Assignment_SingleLocal pins `$x = 99;` for a parameter.
func TestCodeGenerator_Assignment_SingleLocal(t *testing.T) {
	src := "[proc,foo](int $x)\n$x = 99;\n"
	rs := compileForTest(t, src)
	got := opcodesOf(rs.Blocks[0])
	want := []Opcode{PushConstantInt, PopLocalVar, Return}
	if !sameOps(got, want) {
		t.Errorf("opcodes: got %v, want %v", names(got), names(want))
	}
}

// TestCodeGenerator_ExpressionStatement_DiscardsInt pins that a bare integer
// expression statement emits PushConstantInt + Discard. The warning for
// "no side effect" is a DiagnosticWarning (not an error) so compileForTest
// does not fatal on it.
func TestCodeGenerator_ExpressionStatement_DiscardsInt(t *testing.T) {
	src := "[proc,foo]\n42;\n"
	rs := compileForTest(t, src)
	got := opcodesOf(rs.Blocks[0])
	want := []Opcode{PushConstantInt, Discard, Return}
	if !sameOps(got, want) {
		t.Errorf("opcodes: got %v, want %v", names(got), names(want))
	}
}

// TestCodeGenerator_Switch pins a one-case switch.
func TestCodeGenerator_Switch(t *testing.T) {
	src := `[proc,foo](int $x)
switch_int ($x) {
  case 1 :
}
`
	rs := compileForTest(t, src)
	if len(rs.SwitchTables) != 1 {
		t.Fatalf("SwitchTables: got %d, want 1", len(rs.SwitchTables))
	}
	st := rs.SwitchTables[0]
	cases := st.Cases()
	if len(cases) != 1 {
		t.Fatalf("Cases: got %d, want 1", len(cases))
	}
	if len(cases[0].Keys) != 1 || cases[0].Keys[0].(int32) != 1 {
		t.Errorf("Cases[0].Keys: got %v, want [1]", cases[0].Keys)
	}
	// Expected block names: entry, switch_0_case_0, switch_end_0
	wantLabels := []string{"entry", "switch_0_case_0", "switch_end_0"}
	if len(rs.Blocks) != len(wantLabels) {
		t.Fatalf("Blocks: got %d, want %d", len(rs.Blocks), len(wantLabels))
	}
	for i, name := range wantLabels {
		if rs.Blocks[i].Label.Name != name {
			t.Errorf("Blocks[%d].Label: got %q, want %q", i, rs.Blocks[i].Label.Name, name)
		}
	}
}
