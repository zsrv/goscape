// pkg/pack/compiler/runescript/server_script_compiler_test.go
package runescript

import (
	"testing"

	"github.com/zsrv/goscape/pkg/pack/compiler/codegen"
	"github.com/zsrv/goscape/pkg/pack/compiler/diagnostics"
	"github.com/zsrv/goscape/pkg/pack/compiler/pointer"
	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

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
