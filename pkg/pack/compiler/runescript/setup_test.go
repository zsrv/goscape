// pkg/pack/compiler/runescript/setup_test.go
package runescript

import (
	"testing"

	"github.com/zsrv/goscape/pkg/pack/compiler/semantics"
	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

func newSetupTestCompiler(features semantics.StrictFeatureLevel) *ServerScriptCompiler {
	return &ServerScriptCompiler{
		TypeManager:     typ.NewTypeManager(),
		Triggers:        nil, // populated in Setup
		CompilerSymbols: map[string]*CompilerTypeInfo{},
		Features:        features,
		DynHandlers:     map[string]semantics.DynamicCommandHandler{},
	}
}

func TestSetup_RegistersCorePrimitives(t *testing.T) {
	c := newSetupTestCompiler(semantics.StrictFeatureLevel{})
	c.Setup()
	if c.TypeManager.FindOrNil("int", false) == nil {
		t.Error("'int' not registered after Setup")
	}
	if c.TypeManager.FindOrNil("string", false) == nil {
		t.Error("'string' not registered after Setup")
	}
	if c.TypeManager.FindOrNil("category", false) == nil {
		t.Error("'category' not registered after Setup")
	}
}

func TestSetup_RegistersAnyAndTypeAlias(t *testing.T) {
	c := newSetupTestCompiler(semantics.StrictFeatureLevel{})
	c.Setup()
	if c.TypeManager.FindOrNil("any", false) == nil {
		t.Error("'any' not registered")
	}
	if c.TypeManager.FindOrNil("type", false) == nil {
		t.Error("'type' alias not registered")
	}
}

func TestSetup_ProcsGate_DisableSkipsProc(t *testing.T) {
	c := newSetupTestCompiler(semantics.StrictFeatureLevel{DisableProcs: true})
	c.Setup()
	if c.TypeManager.FindOrNil("proc", false) != nil {
		t.Error("DisableProcs=true: 'proc' type should NOT be registered")
	}
}

func TestSetup_ProcsGate_DefaultRegistersProc(t *testing.T) {
	c := newSetupTestCompiler(semantics.StrictFeatureLevel{})
	c.Setup()
	if c.TypeManager.FindOrNil("proc", false) == nil {
		t.Error("default: 'proc' type SHOULD be registered")
	}
}

func TestSetup_AddsSymLoadersForKnownCompilerSymbols(t *testing.T) {
	c := newSetupTestCompiler(semantics.StrictFeatureLevel{})
	c.CompilerSymbols["loc"] = &CompilerTypeInfo{Map: map[string]string{}}
	c.Setup()
	hasLoc := false
	for _, l := range c.SymbolLoaders {
		if _, ok := l.(*CompilerTypeInfoLoader); ok {
			hasLoc = true
			break
		}
	}
	if !hasLoc {
		t.Error("CompilerSymbols['loc'] present: expected at least one CompilerTypeInfoLoader")
	}
}

func TestSetup_AddsSymConstantLoader(t *testing.T) {
	c := newSetupTestCompiler(semantics.StrictFeatureLevel{})
	c.CompilerSymbols["constant"] = &CompilerTypeInfo{
		Map: map[string]string{"MAX": "99"},
	}
	c.Setup()
	hasConstantLoader := false
	for _, l := range c.SymbolLoaders {
		if _, ok := l.(*CompilerTypeInfoConstantLoader); ok {
			hasConstantLoader = true
			break
		}
	}
	if !hasConstantLoader {
		t.Error("CompilerSymbols['constant'] present: expected CompilerTypeInfoConstantLoader")
	}
}
