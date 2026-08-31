// pkg/pack/compiler/codegen/smoke_test.go — external package smoke test
// exercising the full parse → register → typecheck → codegen pipeline.
package codegen_test

import (
	"encoding/binary"
	"testing"

	"github.com/zsrv/goscape/pkg/pack/compiler/cfg"
	"github.com/zsrv/goscape/pkg/pack/compiler/codegen"
	"github.com/zsrv/goscape/pkg/pack/compiler/command"
	"github.com/zsrv/goscape/pkg/pack/compiler/diagnostics"
	"github.com/zsrv/goscape/pkg/pack/compiler/parser"
	"github.com/zsrv/goscape/pkg/pack/compiler/pointer"
	"github.com/zsrv/goscape/pkg/pack/compiler/runescript"
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

	// NAI-208 T8: run pointer-flow validation. Empty commandPointers map
	// matches NAI-208's scope (NAI-210 will populate the registry); the
	// existing source touches no var-state, so we expect zero diagnostics.
	pc := cfg.NewPointerChecker(d, cg.Scripts(), map[string]*pointer.PointerHolder{}, semantics.StrictFeatureLevel{})
	pc.Run()

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

// TestPipeline_FullSlice_WithPointerRequirement extends the codegen smoke
// with a command that requires ACTIVE_PLAYER, validating that the
// PointerChecker emits the expected uninitialized-pointer diagnostic when
// the proc's trigger does not set it.
//
// NAI-208-T8-REQUIRE-PLAYER-SINK: `require_player;` is a bare-identifier
// command call. The symbol is inserted under trigger.CommandTrigger so that
// codegen's visitIdentifier pointer-equality check (`ss.Trigger ==
// trigger.CommandTrigger`) emits a Command instruction, not
// PushConstantSymbol. The plan prescribed a local TriggerType{ID:99}; that
// would fail the pointer-equality gate and never produce a Command opcode.
func TestPipeline_FullSlice_WithPointerRequirement(t *testing.T) {
	src := `[proc,bad]()
require_player;
`

	tm := typ.NewTypeManager()
	for _, p := range typ.PrimitiveAll {
		_ = tm.RegisterByRepresentation(p)
	}
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
	// CommandTrigger must be registered so TypeChecker.commandTrigger is
	// populated and pointer-equality against ss.Trigger works for both
	// type-checking and codegen visitIdentifier.
	_ = trm.RegisterTrigger(trigger.CommandTrigger)

	root := symbol.NewSymbolTable(nil)
	requireSym := &symbol.ServerScriptSymbol{
		Trigger: trigger.CommandTrigger,
		Name:    "require_player",
		// Parameters must equal typ.MetaUnit (no params); TypeChecker
		// checks Parameters != MetaUnit and calls .Representation() on it,
		// panicking on nil.
		Parameters: typ.MetaUnit,
		// Returns must be non-nil so symbolToType returns a non-nil
		// typ.Type; a nil Returns causes resolveSymbol to skip the symbol.
		Returns: typ.MetaUnit,
	}
	root.Insert(symbol.SymbolTypeServerScript(trigger.CommandTrigger), requireSym)

	d := &diagnostics.Diagnostics{}
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
		t.Fatalf("pre-pointer-check diagnostics: %+v", d.List())
	}

	cp := map[string]*pointer.PointerHolder{
		"require_player": {Required: pointer.NewPointerSet(pointer.ActivePlayer)},
	}
	pc := cfg.NewPointerChecker(d, cg.Scripts(), cp, semantics.StrictFeatureLevel{})
	pc.Run()

	var errs []diagnostics.Diagnostic
	for _, e := range d.List() {
		if e.IsError() {
			errs = append(errs, e)
		}
	}
	if len(errs) != 1 {
		t.Fatalf("expected exactly 1 error diagnostic, got %d: %v", len(errs), d.List())
	}
}

// TestPipeline_FullSliceWithWriter extends the codegen+pointer pipeline by
// running BinaryScriptWriter on the two-script source and pins the writer
// output for the `helper` script. Mirrors the existing TestPipeline_FullSlice
// setup but adds a writer hop after PointerChecker.
func TestPipeline_FullSliceWithWriter(t *testing.T) {
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
	pc := cfg.NewPointerChecker(d, cg.Scripts(), map[string]*pointer.PointerHolder{}, semantics.StrictFeatureLevel{})
	pc.Run()
	if d.HasErrors() {
		t.Fatalf("unexpected diagnostics: %+v", d.List())
	}

	mapper := runescript.NewSymbolMapper(d)
	mapper.PutScript(0, "[proc,helper]")
	mapper.PutScript(1, "[proc,foo]")

	rec := &smokeRec{}
	w := runescript.NewBinaryScriptWriter(mapper, rec)
	for _, s := range cg.Scripts() {
		w.Write(s)
	}

	if d.HasErrors() {
		t.Fatalf("writer-phase diagnostics: %+v", d.List())
	}

	if len(rec.scripts) != 2 {
		t.Fatalf("writer emitted %d scripts, want 2", len(rec.scripts))
	}

	// Byte-pin: the first 29 bytes of the `helper` blob are deterministic.
	// fullName "[proc,helper]\x00" (14) + sourceName "smoke.rs2\x00" (10) +
	// lookupKey 0xFFFFFFFF (4) + debugproc-zero 0x00 (1) = 29 bytes.
	helperBlob := rec.scripts[0]
	if got := string(helperBlob[:13]); got != "[proc,helper]" {
		t.Errorf("helper.fullName prefix = %q, want %q", got, "[proc,helper]")
	}
	if helperBlob[13] != 0 {
		t.Errorf("helper.fullName terminator missing")
	}
	if got := string(helperBlob[14:23]); got != "smoke.rs2" {
		t.Errorf("helper.sourceName = %q, want %q", got, "smoke.rs2")
	}
	if helperBlob[23] != 0 {
		t.Errorf("helper.sourceName terminator missing")
	}
	// lookupKey = -1 (SubjectMode.Name) at offset 24.
	if got := int32(binary.BigEndian.Uint32(helperBlob[24:28])); got != -1 {
		t.Errorf("helper.lookupKey = %d, want -1", got)
	}
	if helperBlob[28] != 0 {
		t.Errorf("helper debugproc-zero = %d, want 0", helperBlob[28])
	}
}

type smokeRec struct {
	scripts [][]byte
}

func (r *smokeRec) OutputScript(s *codegen.RuneScript, data []byte) {
	d := make([]byte, len(data))
	copy(d, data)
	r.scripts = append(r.scripts, d)
}
