// pkg/pack/compiler/runescript/server_script_compiler.go
package runescript

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/zsrv/goscape/pkg/pack/compiler/ast"
	"github.com/zsrv/goscape/pkg/pack/compiler/codegen"
	"github.com/zsrv/goscape/pkg/pack/compiler/diagnostics"
	"github.com/zsrv/goscape/pkg/pack/compiler/parser"
	"github.com/zsrv/goscape/pkg/pack/compiler/pointer"
	"github.com/zsrv/goscape/pkg/pack/compiler/semantics"
	"github.com/zsrv/goscape/pkg/pack/compiler/symbol"
	"github.com/zsrv/goscape/pkg/pack/compiler/trigger"
	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

// ServerScriptCompiler is the goscape port of TS ScriptCompiler +
// ServerScriptCompiler (single struct, no inheritance). Mirrors TS
// src/runescript/ServerScriptCompiler.ts.
//
// Setup() installs default type checkers + server triggers + script var
// types + dynamic command handlers + sym loaders; Run(ext) orchestrates
// parse → analyze → codegen → check-pointers → write.
type ServerScriptCompiler struct {
	SourcePaths  []string
	ExcludePaths []string

	TypeManager     *typ.TypeManager
	Triggers        *trigger.TriggerManager
	RootTable       *symbol.SymbolTable
	DynHandlers     map[string]semantics.DynamicCommandHandler
	SymbolLoaders   []symbol.SymbolLoader
	CompilerSymbols map[string]*CompilerTypeInfo
	Mapper          *SymbolMapper
	CommandPointers map[string]*pointer.PointerHolder
	Features        semantics.StrictFeatureLevel

	DiagHandler *diagnostics.Diagnostics

	BinaryWriter *BinaryScriptWriter
	Writer       BinaryOutput
}

// Types satisfies symbol.CompilerContext: returns the underlying TypeManager
// so symbol loaders (T10/T11) can perform type lookups during Load().
func (c *ServerScriptCompiler) Types() *typ.TypeManager {
	return c.TypeManager
}

// Compile-time assertion that *ServerScriptCompiler satisfies the loader
// callback contract.
var _ symbol.CompilerContext = (*ServerScriptCompiler)(nil)

// Run executes the compile pipeline. Mirrors TS ScriptCompiler.run + compile
// L220-265: loadSymbols → parse → analyze → codegen → checkPointers → write,
// followed by Writer.Close() if the sink implements io.Closer
// (TS L221-223 `'close' in this.scriptWriter`).
//
// NAI-210-D-EMPTYPOINTERS-RETURNS-FALSE: TS checkPointers L388-406 returns
// false when commandPointers is empty, which halts the pipeline before
// write(). This is TS-faithful but produces no output when CommandPointers
// is empty.
func (c *ServerScriptCompiler) Run(ext string) error {
	if c.DiagHandler == nil {
		c.DiagHandler = &diagnostics.Diagnostics{}
	}

	if err := c.loadSymbols(); err != nil {
		return err
	}

	files, err := c.parsePhase(ext)
	if err != nil {
		return err
	}
	if c.DiagHandler.HasErrors() {
		return fmt.Errorf("parse: diagnostics reported errors")
	}

	if err := c.analyzePhase(files); err != nil {
		return err
	}

	scripts, err := c.codegenPhase(files)
	if err != nil {
		return err
	}

	if c.checkPointersPhase(scripts) {
		return nil // NAI-210-D-EMPTYPOINTERS-RETURNS-FALSE early-return
	}

	c.writePhase(scripts)

	if closer, ok := c.Writer.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}

// loadSymbols invokes every registered SymbolLoader against the root table.
// Mirrors TS ScriptCompiler.loadSymbols L229-233.
func (c *ServerScriptCompiler) loadSymbols() error {
	for _, l := range c.SymbolLoaders {
		if err := l.Load(c.RootTable, c); err != nil {
			return err
		}
	}
	return nil
}

// parsePhase walks SourcePaths recursively, parses every file with the given
// extension, and returns the resulting ScriptFile nodes. Mirrors TS
// ScriptCompiler.parse L270-337 (macro support deferred — goscape ports
// macros in a future slice).
func (c *ServerScriptCompiler) parsePhase(ext string) ([]*ast.ScriptFile, error) {
	var files []*ast.ScriptFile
	for _, sourcePath := range c.SourcePaths {
		err := filepath.Walk(sourcePath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() || !strings.HasSuffix(path, "."+ext) {
				return nil
			}
			content, rerr := os.ReadFile(path)
			if rerr != nil {
				return rerr
			}
			p := parser.NewScriptFileParser(string(content), path)
			node := p.ParseScriptFile()
			if node != nil {
				files = append(files, node)
			}
			return nil
		})
		if err != nil && !os.IsNotExist(err) {
			return nil, err
		}
	}
	return files, nil
}

// analyzePhase drives ScriptRegistration then registerSecondaryCommands then
// TypeChecker over every parsed file. Mirrors TS ScriptCompiler.analyze
// L343-382.
func (c *ServerScriptCompiler) analyzePhase(files []*ast.ScriptFile) error {
	reg := semantics.NewScriptRegistration(c.TypeManager, c.Triggers, c.RootTable, c.DiagHandler, c.Features)
	for _, f := range files {
		reg.Visit(f)
	}

	c.registerSecondaryCommands()

	tc := semantics.NewTypeChecker(c.TypeManager, c.Triggers, c.RootTable, c.DynHandlers, c.DiagHandler, c.Features)
	for _, f := range files {
		tc.Visit(f)
	}

	if c.DiagHandler.HasErrors() {
		return fmt.Errorf("analyze: diagnostics reported errors")
	}
	return nil
}

// registerSecondaryCommands inserts alias ServerScriptSymbols for every
// "." -prefixed entry in CommandPointers whose unprefixed base name already
// resolves to a registered ServerScriptSymbol. Mirrors TS
// ScriptCompiler.registerSecondaryCommands L384-412.
func (c *ServerScriptCompiler) registerSecondaryCommands() {
	if len(c.CommandPointers) < 1 {
		return
	}
	commandType := symbol.SymbolTypeServerScript(trigger.CommandTrigger)
	for name := range c.CommandPointers {
		if !strings.HasPrefix(name, ".") {
			continue
		}
		baseName := name[1:]
		if baseName == "" {
			continue
		}
		baseSym := c.RootTable.Find(commandType, baseName)
		if baseSym == nil {
			continue
		}
		base, ok := baseSym.(*symbol.ServerScriptSymbol)
		if !ok {
			continue
		}
		if c.RootTable.Find(commandType, name) != nil {
			continue
		}
		alias := &symbol.ServerScriptSymbol{
			ScriptSymbolFields: symbol.ScriptSymbolFields{
				Trigger:    trigger.CommandTrigger,
				Name:       name,
				Parameters: base.Parameters,
				Returns:    base.Returns,
			},
		}
		c.RootTable.Insert(commandType, alias)
	}
}

// codegenPhase runs a fresh CodeGenerator per file and gathers the emitted
// RuneScripts. Mirrors TS ScriptCompiler.codegen L418-446.
func (c *ServerScriptCompiler) codegenPhase(files []*ast.ScriptFile) ([]*codegen.RuneScript, error) {
	var scripts []*codegen.RuneScript
	for _, f := range files {
		gen := codegen.NewCodeGenerator(c.RootTable, c.DynHandlers, c.DiagHandler)
		gen.Visit(f)
		scripts = append(scripts, gen.Scripts()...)
	}
	if c.DiagHandler.HasErrors() {
		return nil, fmt.Errorf("codegen: diagnostics reported errors")
	}
	return scripts, nil
}

// checkPointersPhase reports whether the pipeline should halt before write.
// Mirrors TS ScriptCompiler.checkPointers L388-406. Returns true ("halt")
// when CommandPointers is empty (NAI-210-D-EMPTYPOINTERS-RETURNS-FALSE) or
// when the PointerChecker reported diagnostic errors.
func (c *ServerScriptCompiler) checkPointersPhase(scripts []*codegen.RuneScript) (halt bool) {
	if len(c.CommandPointers) < 1 {
		return true
	}
	checker := NewServerPointerChecker(c.DiagHandler, scripts, c.CommandPointers, c.Features, nil)
	checker.Run()
	return c.DiagHandler.HasErrors()
}

// writePhase writes every emitted script through BinaryWriter, skipping any
// script whose source path is under an ExcludePaths entry. Mirrors TS
// ScriptCompiler.write L450-465.
func (c *ServerScriptCompiler) writePhase(scripts []*codegen.RuneScript) {
	for _, s := range scripts {
		if c.isExcluded(s.SourceName) {
			continue
		}
		c.BinaryWriter.Write(s)
	}
}

// isExcluded reports whether sourceName lives under any ExcludePaths entry.
func (c *ServerScriptCompiler) isExcluded(sourceName string) bool {
	abs, err := filepath.Abs(sourceName)
	if err != nil {
		return false
	}
	for _, excluded := range c.ExcludePaths {
		excludedClean := filepath.Clean(excluded)
		if abs == excludedClean {
			return true
		}
		if strings.HasPrefix(abs, excludedClean+string(filepath.Separator)) {
			return true
		}
	}
	return false
}
