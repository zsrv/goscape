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

type noopBinaryOutput struct {
	writeCount int
}

func (n *noopBinaryOutput) OutputScript(s *codegen.RuneScript, data []byte) { n.writeCount++ }
