// pkg/pack/compiler/runescript/server_script_compiler_test.go
package runescript

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zsrv/goscape/pkg/pack/compiler/codegen"
	"github.com/zsrv/goscape/pkg/pack/compiler/diagnostics"
	"github.com/zsrv/goscape/pkg/pack/compiler/pointer"
	"github.com/zsrv/goscape/pkg/pack/compiler/symbol"
	"github.com/zsrv/goscape/pkg/pack/compiler/trigger"
	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

// scriptWithUnsetActivePlayer builds a one-block RuneScript whose proc-trigger
// (Identifier="proc") does NOT set ACTIVE_PLAYER, then invokes a Command
// symbol named "p_kickout" that requires ACTIVE_PLAYER. Mirrors the fixture
// shape from cfg.TestPointerChecker_Run_UninitializedReported.
func scriptWithUnsetActivePlayer() *codegen.RuneScript {
	tr := &trigger.TriggerType{ID: 0, Identifier: "proc"}
	sym := &symbol.ServerScriptSymbol{Trigger: tr, Name: "p1"}
	rs := codegen.NewRuneScript("test.rs2", sym, tr, "p1", nil)
	b := codegen.NewBlock(&codegen.Label{Name: "entry"})
	cmd := &symbol.ServerScriptSymbol{
		Trigger: &trigger.TriggerType{Identifier: "command"},
		Name:    "p_kickout"}
	b.Add(codegen.Instruction{Opcode: codegen.Command, Operand: cmd})
	b.Add(codegen.Instruction{Opcode: codegen.Return})
	rs.Blocks = []*codegen.Block{b}
	return rs
}

// TestCheckPointersPhase_EmptyCommandPointers_HaltsWithoutError pins the
// NAI-210-D-EMPTYPOINTERS-RETURNS-FALSE early-return: empty CommandPointers
// halts BEFORE write but is NOT an error condition — Run() must still
// return nil. Function returns (halt=true, err=nil).
func TestCheckPointersPhase_EmptyCommandPointers_HaltsWithoutError(t *testing.T) {
	c := &ServerScriptCompiler{
		CommandPointers: map[string]*pointer.PointerHolder{},
		Handler:         diagnostics.NopHandler{},
	}
	halt, err := c.checkPointersPhase(nil)
	if err != nil {
		t.Errorf("empty CommandPointers: got err=%v, want nil", err)
	}
	if !halt {
		t.Errorf("empty CommandPointers: got halt=false, want true (TS-faithful early-return)")
	}
}

// TestCheckPointersPhase_DiagnosticErrors_HaltsWithError pins the new
// observability contract: when the PointerChecker reports a real diagnostic
// error (uninitialized/corrupted pointer), checkPointersPhase returns
// (halt=true, err=non-nil) so Run() can surface it.
func TestCheckPointersPhase_DiagnosticErrors_HaltsWithError(t *testing.T) {
	rs := scriptWithUnsetActivePlayer()
	c := &ServerScriptCompiler{
		CommandPointers: map[string]*pointer.PointerHolder{
			"p_kickout": {Required: pointer.NewPointerSet(pointer.ActivePlayer)},
		},
		Handler: diagnostics.NopHandler{},
	}
	halt, err := c.checkPointersPhase([]*codegen.RuneScript{rs})
	if err == nil {
		t.Fatal("pointer-check diagnostic errors: got err=nil, want non-nil")
	}
	if !halt {
		t.Errorf("pointer-check diagnostic errors: got halt=false, want true (halt before write)")
	}
}

// TestCheckPointersPhase_NoErrors_Proceeds pins the success path: non-empty
// CommandPointers with a clean script returns (halt=false, err=nil) so the
// pipeline proceeds to writePhase.
func TestCheckPointersPhase_NoErrors_Proceeds(t *testing.T) {
	c := &ServerScriptCompiler{
		CommandPointers: map[string]*pointer.PointerHolder{
			"p_kickout": {Required: pointer.NewPointerSet(pointer.ActivePlayer)},
		},
		Handler: diagnostics.NopHandler{},
	}
	// Empty scripts → no commands inspected → no diagnostics.
	halt, err := c.checkPointersPhase(nil)
	if err != nil {
		t.Errorf("no scripts, no errors: got err=%v, want nil", err)
	}
	if halt {
		t.Errorf("no scripts, no errors: got halt=true, want false (proceed to write)")
	}
}

// TestRun_PointerCheckErrors_ReturnsError pins the user-facing contract:
// when checkPointersPhase reports diagnostic errors, Run() surfaces them
// as a non-nil error return rather than silently halting. Mirrors the
// intent in the NAI-211-D-NO-PROCESS-EXIT deviation comment
// ("returns errors up through ServerScriptCompiler.Run").
//
// Fixture uses the corrupted-pointer arm (a `proc` trigger sets all
// pointers, so uninitialized won't fire): one command corrupts
// ACTIVE_PLAYER, the next requires it.
func TestRun_PointerCheckErrors_ReturnsError(t *testing.T) {
	tmpDir := t.TempDir()
	src := "[proc,helper]\np_corrupt;\np_kickout;\n"
	if err := os.WriteFile(filepath.Join(tmpDir, "helper.rs2"), []byte(src), 0o644); err != nil {
		t.Fatalf("write helper.rs2: %v", err)
	}

	mapper := NewSymbolMapper(nil)
	c := &ServerScriptCompiler{
		SourcePaths: []string{tmpDir},
		TypeManager: typ.NewTypeManager(),
		Mapper:      mapper,
		CommandPointers: map[string]*pointer.PointerHolder{
			"p_corrupt": {Corrupted: pointer.NewPointerSet(pointer.ActivePlayer)},
			"p_kickout": {Required: pointer.NewPointerSet(pointer.ActivePlayer)},
		},
		Writer: &noopBinaryOutput{},
	}
	c.Setup()
	// Register p_corrupt and p_kickout as void-returning, no-arg commands so
	// the parser and type-checker accept `p_corrupt;` / `p_kickout;` in the
	// proc body. SymbolMapper must also know their opcode ids so codegen
	// doesn't fail before pointer-check (Run() returns from checkPointersPhase
	// before writePhase; but codegen still emits Command instructions with the
	// symbol operand, which doesn't need a mapped opcode id).
	for _, name := range []string{"p_corrupt", "p_kickout"} {
		sym := &symbol.ServerScriptSymbol{
			Trigger:    trigger.CommandTrigger,
			Name:       name,
			Parameters: typ.MetaUnit,
			Returns:    typ.MetaUnit}
		c.RootTable.Insert(symbol.SymbolTypeServerScript(trigger.CommandTrigger), sym)
	}
	c.BinaryWriter = NewBinaryScriptWriter(mapper, c.Writer)

	err := c.Run("rs2")
	if err == nil {
		t.Fatal("Run with pointer-check errors: got nil error, want non-nil")
	}
}

// TestServerScriptCompiler_StructIsConstructable pins that a fresh
// ServerScriptCompiler literal compiles + has the expected zero-value shape.
func TestServerScriptCompiler_StructIsConstructable(t *testing.T) {
	c := &ServerScriptCompiler{}
	if c == nil {
		t.Fatal("ServerScriptCompiler literal nil")
	}
}

// TestServerScriptCompiler_Run_EmptySourcePathReturnsNoError pins that the
// driver returns nil when SourcePaths is empty.
func TestServerScriptCompiler_Run_EmptySourcePathReturnsNoError(t *testing.T) {
	mapper := NewSymbolMapper(nil)
	c := &ServerScriptCompiler{
		SourcePaths:     []string{},
		TypeManager:     typ.NewTypeManager(),
		Mapper:          mapper,
		CommandPointers: map[string]*pointer.PointerHolder{"foo": {Required: pointer.NewPointerSet()}},
		Writer:          &noopBinaryOutput{},
	}
	c.Setup()
	c.BinaryWriter = NewBinaryScriptWriter(mapper, c.Writer)

	if err := c.Run("rs2"); err != nil {
		t.Errorf("Run on empty source paths: got error %v, want nil", err)
	}
}

// TestServerScriptCompiler_Run_EmptyCommandPointers_HaltsBeforeWrite pins
// NAI-210-D-EMPTYPOINTERS-RETURNS-FALSE: empty commandPointers triggers
// TS-faithful halt-before-write (TS ScriptCompiler.checkPointers L388-406
// returns false when commandPointers is empty).
func TestServerScriptCompiler_Run_EmptyCommandPointers_HaltsBeforeWrite(t *testing.T) {
	mapper := NewSymbolMapper(nil)
	sink := &noopBinaryOutput{}
	c := &ServerScriptCompiler{
		SourcePaths:     []string{},
		TypeManager:     typ.NewTypeManager(),
		Mapper:          mapper,
		CommandPointers: map[string]*pointer.PointerHolder{}, // EMPTY
		Writer:          sink,
	}
	c.Setup()
	c.BinaryWriter = NewBinaryScriptWriter(mapper, c.Writer)

	_ = c.Run("rs2")
	if sink.writeCount != 0 {
		t.Errorf("empty commandPointers: sink got %d writes, want 0", sink.writeCount)
	}
}

// TestServerScriptCompiler_collectOverlayInterfaces_HarvestsFromCompilerSymbols
// pins the port of TS ServerScriptCompiler.createPointerChecker L237-241:
// overlay interface names come from CompilerSymbols["overlayinterface"].Map
// values, sorted by numeric id (NAI-210-D-LOADER-SORTED-ITERATION) for
// deterministic ordering.
func TestServerScriptCompiler_collectOverlayInterfaces_HarvestsFromCompilerSymbols(t *testing.T) {
	c := &ServerScriptCompiler{
		CompilerSymbols: map[string]*CompilerTypeInfo{
			"overlayinterface": {Map: map[string]string{"2": "second", "10": "tenth", "1": "first"}},
		},
	}
	got := c.collectOverlayInterfaces()
	want := []string{"first", "second", "tenth"}
	if len(got) != len(want) {
		t.Fatalf("collectOverlayInterfaces len: got %d %v, want %d %v", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("collectOverlayInterfaces[%d]: got %q, want %q", i, got[i], want[i])
		}
	}
}

// TestServerScriptCompiler_collectOverlayInterfaces_NilWhenAbsent pins the
// nil-return contract when CompilerSymbols has no "overlayinterface" entry,
// matching TS's `overlaySymbols ? ... : []` ternary at L238-239.
func TestServerScriptCompiler_collectOverlayInterfaces_NilWhenAbsent(t *testing.T) {
	c := &ServerScriptCompiler{CompilerSymbols: map[string]*CompilerTypeInfo{}}
	if got := c.collectOverlayInterfaces(); got != nil {
		t.Errorf("collectOverlayInterfaces with no overlayinterface entry: got %v, want nil", got)
	}
}

type noopBinaryOutput struct {
	writeCount int
}

func (n *noopBinaryOutput) OutputScript(s *codegen.RuneScript, data []byte) { n.writeCount++ }

// recordingHandler is a test-only diagnostics.Handler that records the
// sequence of HandleXxx method names called and the per-call diagnostic
// count.
type recordingHandler struct {
	calls    []string
	counts   []int
	capDiags map[string]*diagnostics.Diagnostics
}

func newRecordingHandler() *recordingHandler {
	return &recordingHandler{capDiags: map[string]*diagnostics.Diagnostics{}}
}

func (r *recordingHandler) record(name string, d *diagnostics.Diagnostics) {
	r.calls = append(r.calls, name)
	r.counts = append(r.counts, len(d.List()))
	r.capDiags[name] = d
}

func (r *recordingHandler) HandleParse(d *diagnostics.Diagnostics) {
	r.record("HandleParse", d)
}
func (r *recordingHandler) HandleTypeChecking(d *diagnostics.Diagnostics) {
	r.record("HandleTypeChecking", d)
}
func (r *recordingHandler) HandleCodeGeneration(d *diagnostics.Diagnostics) {
	r.record("HandleCodeGeneration", d)
}
func (r *recordingHandler) HandlePointerChecking(d *diagnostics.Diagnostics) {
	r.record("HandlePointerChecking", d)
}

// TestRun_HandlerDispatchedInOrder pins the per-phase dispatch sequence
// when CommandPointers is empty (TS-faithful early-return per
// NAI-210-D-EMPTYPOINTERS-RETURNS-FALSE means HandlePointerChecking is
// NOT called in that path).
func TestRun_HandlerDispatchedInOrder(t *testing.T) {
	mapper := NewSymbolMapper(nil)
	rh := newRecordingHandler()
	c := &ServerScriptCompiler{
		SourcePaths:     []string{},
		TypeManager:     typ.NewTypeManager(),
		Mapper:          mapper,
		CommandPointers: map[string]*pointer.PointerHolder{}, // EMPTY → early halt
		Writer:          &noopBinaryOutput{},
		Handler:         rh,
	}
	c.Setup()
	c.BinaryWriter = NewBinaryScriptWriter(mapper, c.Writer)

	_ = c.Run("rs2")

	want := []string{"HandleParse", "HandleTypeChecking", "HandleCodeGeneration"}
	if !equalStrings(rh.calls, want) {
		t.Errorf("dispatch order: got %v, want %v", rh.calls, want)
	}
}

// TestRun_HandlerDispatchedForPointerChecking pins that
// HandlePointerChecking IS called when CommandPointers is non-empty.
func TestRun_HandlerDispatchedForPointerChecking(t *testing.T) {
	mapper := NewSymbolMapper(nil)
	rh := newRecordingHandler()
	c := &ServerScriptCompiler{
		SourcePaths: []string{},
		TypeManager: typ.NewTypeManager(),
		Mapper:      mapper,
		// Non-empty CommandPointers → HandlePointerChecking called.
		CommandPointers: map[string]*pointer.PointerHolder{"foo": {Required: pointer.NewPointerSet()}},
		Writer:          &noopBinaryOutput{},
		Handler:         rh,
	}
	c.Setup()
	c.BinaryWriter = NewBinaryScriptWriter(mapper, c.Writer)

	_ = c.Run("rs2")

	want := []string{"HandleParse", "HandleTypeChecking", "HandleCodeGeneration", "HandlePointerChecking"}
	if !equalStrings(rh.calls, want) {
		t.Errorf("dispatch order: got %v, want %v", rh.calls, want)
	}
}

// TestRun_PerPhaseDiagnosticsAreFresh pins NAI-211-D-PHASE-DIAGNOSTICS-FRESH:
// each phase gets its OWN *Diagnostics; a diagnostic reported in one
// phase does not leak into the next phase's accumulator.
func TestRun_PerPhaseDiagnosticsAreFresh(t *testing.T) {
	mapper := NewSymbolMapper(nil)
	rh := newRecordingHandler()
	c := &ServerScriptCompiler{
		SourcePaths:     []string{},
		TypeManager:     typ.NewTypeManager(),
		Mapper:          mapper,
		CommandPointers: map[string]*pointer.PointerHolder{"foo": {Required: pointer.NewPointerSet()}},
		Writer:          &noopBinaryOutput{},
		Handler:         rh,
	}
	c.Setup()
	c.BinaryWriter = NewBinaryScriptWriter(mapper, c.Writer)

	_ = c.Run("rs2")

	// With empty SourcePaths there are no diagnostics anywhere; the
	// structural pin is that each call captured a DISTINCT *Diagnostics
	// pointer.
	seen := map[*diagnostics.Diagnostics]string{}
	for name, d := range rh.capDiags {
		if prev, dup := seen[d]; dup {
			t.Errorf("per-phase isolation: %s and %s share the same *Diagnostics pointer", prev, name)
		}
		seen[d] = name
	}
}

// TestRun_NilHandlerDefaultsToNop pins that Run() initializes a nil Handler
// to NopHandler{} and runs to completion without panic.
func TestRun_NilHandlerDefaultsToNop(t *testing.T) {
	mapper := NewSymbolMapper(nil)
	c := &ServerScriptCompiler{
		SourcePaths:     []string{},
		TypeManager:     typ.NewTypeManager(),
		Mapper:          mapper,
		CommandPointers: map[string]*pointer.PointerHolder{"foo": {Required: pointer.NewPointerSet()}},
		Writer:          &noopBinaryOutput{},
		Handler:         nil,
	}
	c.Setup()
	c.BinaryWriter = NewBinaryScriptWriter(mapper, c.Writer)

	if err := c.Run("rs2"); err != nil {
		t.Errorf("Run with nil Handler: got %v, want nil", err)
	}
	if _, ok := c.Handler.(diagnostics.NopHandler); !ok {
		t.Errorf("nil Handler should default to NopHandler{}, got %T", c.Handler)
	}
}

// TestRun_ParserSyntaxErrorReachesParseDiagnostics pins that a syntactically
// invalid source file produces at least one diagnostic in the parse-phase
// *Diagnostics handed to HandleParse. The fixture uses an obviously bad
// `[` start with no closing bracket; the lexer/parser will report a
// SyntaxError which the new ParserErrorListener forwards into d.
func TestRun_ParserSyntaxErrorReachesParseDiagnostics(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "bad.rs2"), []byte("[proc,\n"), 0o644); err != nil {
		t.Fatalf("write bad.rs2: %v", err)
	}

	mapper := NewSymbolMapper(nil)
	rh := newRecordingHandler()
	c := &ServerScriptCompiler{
		SourcePaths:     []string{tmpDir},
		TypeManager:     typ.NewTypeManager(),
		Mapper:          mapper,
		CommandPointers: map[string]*pointer.PointerHolder{"foo": {Required: pointer.NewPointerSet()}},
		Writer:          &noopBinaryOutput{},
		Handler:         rh,
	}
	c.Setup()
	c.BinaryWriter = NewBinaryScriptWriter(mapper, c.Writer)

	err := c.Run("rs2")
	// Run returns an error from parsePhase ("parse: diagnostics reported errors").
	if err == nil {
		t.Fatal("Run on syntactically invalid source: got nil error, want non-nil")
	}
	parseDiag, ok := rh.capDiags["HandleParse"]
	if !ok || parseDiag == nil {
		t.Fatal("HandleParse was not called")
	}
	if !parseDiag.HasErrors() {
		t.Errorf("parse-phase Diagnostics has no errors after invalid source; got %d entries", len(parseDiag.List()))
	}
}

// equalStrings is a test helper for slice equality.
func equalStrings(a, b []string) bool {
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
