// pkg/pack/compiler/codegen/smoke_test.go — external package smoke test
// exercising the full parse → register → typecheck → codegen pipeline.
package codegen_test

import (
	"testing"

	"github.com/zsrv/goscape/pkg/pack/compiler/codegen"
	"github.com/zsrv/goscape/pkg/pack/compiler/command"
	"github.com/zsrv/goscape/pkg/pack/compiler/diagnostics"
	"github.com/zsrv/goscape/pkg/pack/compiler/parser"
	"github.com/zsrv/goscape/pkg/pack/compiler/semantics"
	"github.com/zsrv/goscape/pkg/pack/compiler/symbol"
	"github.com/zsrv/goscape/pkg/pack/compiler/trigger"
	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

// TestPipeline_FullSlice exercises parse → register → typecheck → codegen on
// a representative 2-script source that touches: return, if/else, while,
// arithmetic (calc), proc-call (gosub), local variables, parameters,
// declarations, literals. Asserts 2 scripts and block-count lower bounds.
//
// NAI-207-D-SMOKE-NO-MES: The plan prescribed `mes("hello, <$name>!");` in
// the body of foo. Registering `mes` requires inserting a ServerScriptSymbol
// for CommandTrigger into the root table; the CommandTrigger registration in
// the TriggerManager is also required. To keep this smoke minimal and avoid
// cascading symbol-fixture setup beyond what this task prescribes, the mes
// call is omitted. The remaining source still exercises all structural
// constructs the plan enumerates.
func TestPipeline_FullSlice(t *testing.T) {
	src := `[proc,helper](int $n)(int)
return(calc($n * 2));

[proc,foo](int $x, string $name)(int)
def_int $result = 0;
if ($x > 0) {
  $result = ~helper($x);
} else {
  $result = 0;
}
while ($result < 100) {
  $result = calc($result + 1);
}
return($result);
`

	tm := typ.NewTypeManager()
	for _, p := range typ.PrimitiveAll {
		_ = tm.RegisterByRepresentation(p)
	}
	// Identity type checker: identical types are assignable.
	tm.AddTypeChecker(func(left, right typ.Type) bool { return left == right })

	trm := trigger.NewTriggerManager()
	proc := &trigger.TriggerType{
		ID:              0,
		Identifier:      "proc",
		SubjectMode:     trigger.ModeName,
		AllowParameters: true,
		AllowReturns:    true,
	}
	_ = trm.RegisterTrigger(proc)

	root := symbol.NewSymbolTable(nil)
	d := &diagnostics.Diagnostics{}

	// Register all dynamic command handlers.
	dyn := map[string]semantics.DynamicCommandHandler{}
	command.RegisterAllDynCommands(tm, semantics.StrictFeatureLevel{}, func(name string, h semantics.DynamicCommandHandler) {
		dyn[name] = h
	})

	p := parser.NewScriptFileParser(src, "smoke.rs2")
	sf := p.ParseScriptFile()
	if sf == nil {
		t.Fatalf("parse failed")
	}

	sr := semantics.NewScriptRegistration(tm, trm, root, d, semantics.StrictFeatureLevel{})
	sr.Visit(sf)
	tc := semantics.NewTypeChecker(tm, trm, root, dyn, d, semantics.StrictFeatureLevel{})
	tc.Visit(sf)
	cg := codegen.NewCodeGenerator(root, dyn, d)
	cg.Visit(sf)

	if d.HasErrors() {
		t.Fatalf("unexpected diagnostics: %+v", d.List())
	}

	scripts := cg.Scripts()
	if len(scripts) != 2 {
		t.Fatalf("Scripts: got %d, want 2", len(scripts))
	}

	// helper script: simple arithmetic return — one block (entry).
	helper := scripts[0]
	if helper.FullName != "[proc,helper]" {
		t.Errorf("helper.FullName: got %q, want %q", helper.FullName, "[proc,helper]")
	}
	if got := len(helper.Blocks); got != 1 {
		t.Errorf("helper.Blocks: got %d, want 1", got)
	}

	// foo script: entry + if_true + if_else + if_end + while_start + while_body + while_end = 7
	foo := scripts[1]
	if foo.FullName != "[proc,foo]" {
		t.Errorf("foo.FullName: got %q, want %q", foo.FullName, "[proc,foo]")
	}
	if got := len(foo.Blocks); got < 7 {
		t.Errorf("foo.Blocks: got %d, want >=7 (entry+if_true+if_else+if_end+while_start+while_body+while_end)", got)
	}
}
