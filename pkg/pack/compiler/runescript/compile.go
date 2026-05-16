// pkg/pack/compiler/runescript/compile.go
package runescript

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/zsrv/goscape/pkg/pack/compiler/diagnostics"
	"github.com/zsrv/goscape/pkg/pack/compiler/pointer"
	"github.com/zsrv/goscape/pkg/pack/compiler/semantics"
	"github.com/zsrv/goscape/pkg/pack/compiler/symbol"
	"github.com/zsrv/goscape/pkg/pack/compiler/trigger"
	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

// Config drives Compile. Mirrors the optional `config` argument of TS
// CompileServerScript (ServerScriptCompilerApplication.ts L16-30).
type Config struct {
	SourcePaths   []string
	ExcludePaths  []string
	Symbols       map[string]*CompilerTypeInfo
	CheckPointers *bool // nil → default true
	Features      semantics.StrictFeatureLevel
	Writer        WriterConfig
	// Handler receives the per-phase Diagnostics from each phase of
	// ServerScriptCompiler.Run. Nil defaults to
	// &diagnostics.BaseDiagnosticsHandler{} (TS-faithful: TS uses
	// BaseDiagnosticsHandler when CompileServerScript is invoked without
	// an override).
	Handler diagnostics.Handler
}

// WriterConfig selects between the Jag-format and JS5-format writer sinks.
// Mirrors TS L22-29.
type WriterConfig struct {
	Jag *JagWriterConfig
	Js5 *Js5WriterConfig
}

// JagWriterConfig configures the JagFileScriptWriter output directory.
type JagWriterConfig struct{ Output string }

// Js5WriterConfig configures the Js5PackScriptWriter output file path.
type Js5WriterConfig struct{ Output string }

// Compile drives the full ServerScriptCompiler pipeline. Mirrors TS
// CompileServerScript (ServerScriptCompilerApplication.ts L13-91): validates
// core symbols, applies defaults for sourcePaths / checkPointers / writer,
// constructs SymbolMapper + writer + pointer map, then invokes Setup + Run.
func Compile(cfg Config) error {
	if cfg.Symbols == nil || cfg.Symbols["command"] == nil || cfg.Symbols["runescript"] == nil {
		return errors.New("core symbols missing from compiler: provide command and runescript symbols")
	}

	sourcePaths := cfg.SourcePaths
	if len(sourcePaths) == 0 {
		sourcePaths = []string{"../content/scripts"}
	}
	excludePaths := cfg.ExcludePaths
	checkPointers := true
	if cfg.CheckPointers != nil {
		checkPointers = *cfg.CheckPointers
	}

	jag := cfg.Writer.Jag
	js5 := cfg.Writer.Js5
	if jag != nil && js5 != nil {
		return errors.New("only one of writer.jag / writer.js5 may be set")
	}
	if jag == nil && js5 == nil {
		jag = &JagWriterConfig{Output: "./data/pack/server"}
	}

	absSources, err := absAll(sourcePaths)
	if err != nil {
		return err
	}
	absExcludes, err := absAll(excludePaths)
	if err != nil {
		return err
	}

	mapper := NewSymbolMapper(nil)
	var writer BinaryOutput

	if jag != nil {
		absOut, err := filepath.Abs(jag.Output)
		if err != nil {
			return err
		}
		w, err := NewJagFileScriptWriter(absOut, mapper)
		if err != nil {
			return err
		}
		writer = w
	} else {
		absOut, err := filepath.Abs(js5.Output)
		if err != nil {
			return err
		}
		w, err := NewJs5PackScriptWriter(absOut, mapper)
		if err != nil {
			return err
		}
		writer = w
	}

	commandPointers := map[string]*pointer.PointerHolder{}
	if err := LoadSpecialSymbols(cfg.Symbols["command"], cfg.Symbols["runescript"], mapper, commandPointers, checkPointers); err != nil {
		return fmt.Errorf("LoadSpecialSymbols: %w", err)
	}

	handler := cfg.Handler
	if handler == nil {
		handler = &diagnostics.BaseDiagnosticsHandler{}
	}
	c := &ServerScriptCompiler{
		SourcePaths:     absSources,
		ExcludePaths:    absExcludes,
		TypeManager:     typ.NewTypeManager(),
		Triggers:        trigger.NewTriggerManager(),
		RootTable:       symbol.NewSymbolTable(nil),
		DynHandlers:     map[string]semantics.DynamicCommandHandler{},
		CompilerSymbols: cfg.Symbols,
		Mapper:          mapper,
		CommandPointers: commandPointers,
		Features:        cfg.Features,
		Writer:          writer,
		Handler:         handler,
	}
	c.Setup()
	c.BinaryWriter = NewBinaryScriptWriter(mapper, c.Writer)

	return c.Run("rs2")
}

// absAll resolves each path to its absolute form. Mirrors TS L68-69
// `sourcePaths.map(p => resolve(p))`.
func absAll(paths []string) ([]string, error) {
	out := make([]string, len(paths))
	for i, p := range paths {
		abs, err := filepath.Abs(p)
		if err != nil {
			return nil, err
		}
		out[i] = abs
	}
	return out, nil
}
