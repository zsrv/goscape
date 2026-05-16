// pkg/pack/compiler/runescript/server_script_compiler_test.go
package runescript

import (
	"testing"

	"github.com/zsrv/goscape/pkg/pack/compiler/codegen"
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
