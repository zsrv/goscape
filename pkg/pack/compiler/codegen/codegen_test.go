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

	if len(cg.Scripts()) == 0 {
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
