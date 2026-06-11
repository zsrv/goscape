// pkg/pack/compiler/run_server_compiler.go
package compiler

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/zsrv/goscape/pkg/pack/compiler/runescript"
	"github.com/zsrv/goscape/pkg/pack/compiler/semantics"
)

// RunServerCompiler is the goscape equivalent of TS runServerCompiler's
// final CompileServerScript({symbols}) call (Compiler.ts:337-376).
//
// It chains:
//  1. BuildSymbols(srcDir, dataPackDir) — assembles the 33-key
//     symbol-category dict (NAI-202; "varbit" joined at rev-254).
//  2. ToCompilerTypeInfo per entry — bridges NAI-200 dual-map shape
//     into NAI-210 single-map shape (NAI-212 T1).
//  3. runescript.Compile(cfg) — drives parse → analyze → codegen →
//     pointer-check → write (NAI-210).
//
// srcDir: directory containing scripts/ and pack/ subdirs.
// outDir: directory under which <outDir>/server/script.{dat,idx} land.
// dataPackDir: cache directory with the 8 .dat/.idx pairs BuildSymbols
// reads (InvType, Component, VarP, VarBit, VarN, VarS, Param, DbTableType).
// In practice callers pass outDir for dataPackDir (i.e. read back the
// cache PackConfigs just wrote).
//
// NAI-212-D-EXPLICIT-SOURCEPATHS: TS CompileServerScript defaults
// sourcePaths to "../content/scripts". goscape parameterizes srcDir
// so it cannot rely on a CWD-relative default; SourcePaths is passed
// explicitly. Permanent deviation.
func RunServerCompiler(srcDir, outDir, dataPackDir string) error {
	loaders, err := loadConfigs(dataPackDir)
	if err != nil {
		return fmt.Errorf("RunServerCompiler: %w", err)
	}
	return runServerCompilerCore(srcDir, outDir, loaders)
}

// runServerCompilerCore is the testable seam under RunServerCompiler,
// mirroring buildSymbolsCore precedent. Takes pre-loaded *configLoaders
// so unit tests can pass synthetic in-memory configs without writing
// binary cache fixtures.
func runServerCompilerCore(srcDir, outDir string, loaders *configLoaders) error {
	bridged, err := bridgedSymbolsCore(srcDir, loaders)
	if err != nil {
		return fmt.Errorf("RunServerCompiler: %w", err)
	}

	serverOut := filepath.Join(outDir, "server")
	cfg := runescript.Config{
		SourcePaths: []string{filepath.Join(srcDir, "scripts")},
		Symbols:     bridged,
		// Match Engine-TS's bundled @lostcityrs/runescript@0.9.4 leniency:
		// disable the two RuneScriptTS-HEAD pointer-checker features that
		// post-date v0.9.4 (overlay-aware IF_BUTTON P_ACTIVE_PLAYER
		// override + opportunistic static-label-arg propagation). Direct
		// runescript.Compile callers keep the strict (zero-value)
		// RuneScriptTS-HEAD defaults; the goscape compile boundary picks
		// leniency because real Content scripts were authored against
		// v0.9.4. See memory pointer_checker_runescriptts_version_divergence.
		Features: semantics.StrictFeatureLevel{
			DisableOverlayInterfaceProtection: true,
			DisableStaticLabelArgPropagation:  true,
		},
		Writer: runescript.WriterConfig{
			Jag: &runescript.JagWriterConfig{Output: serverOut},
		},
	}
	if err := runescript.Compile(cfg); err != nil {
		return fmt.Errorf("RunServerCompiler: %w", err)
	}
	return nil
}

// pointerNameAliases maps the "_2"-suffixed names from pkg/script.Pointers
// to the dot-prefixed Representation strings used by pointer.ForName.
// NAI-212-D-POINTER-NAME-TRANSLATION.
var pointerNameAliases = map[string]string{
	"active_player2":   ".active_player",
	"active_npc2":      ".active_npc",
	"active_loc2":      ".active_loc",
	"active_obj2":      ".active_obj",
	"p_active_player2": ".p_active_player",
}

// translatePointerNameList rewrites each comma-separated name in s using
// pointerNameAliases. Names with no alias are preserved as-is.
func translatePointerNameList(s string) string {
	if s == "" {
		return s
	}
	parts := strings.Split(s, ",")
	for i, name := range parts {
		if alias, ok := pointerNameAliases[strings.TrimSpace(name)]; ok {
			parts[i] = alias
		}
	}
	return strings.Join(parts, ",")
}

// translateCommandPointerNames rewrites all pointer-name values in the
// six pointer maps of a command CompilerTypeInfo.
func translateCommandPointerNames(cmd *runescript.CompilerTypeInfo) {
	for k, v := range cmd.Require {
		cmd.Require[k] = translatePointerNameList(v)
	}
	for k, v := range cmd.Require2 {
		cmd.Require2[k] = translatePointerNameList(v)
	}
	for k, v := range cmd.Set {
		cmd.Set[k] = translatePointerNameList(v)
	}
	for k, v := range cmd.Set2 {
		cmd.Set2[k] = translatePointerNameList(v)
	}
	for k, v := range cmd.Corrupt {
		cmd.Corrupt[k] = translatePointerNameList(v)
	}
	for k, v := range cmd.Corrupt2 {
		cmd.Corrupt2[k] = translatePointerNameList(v)
	}
}

// LoadCompilerSymbols assembles the symbol map the runescript compiler
// needs to type-check and codegen. Identical to the prep stages
// RunServerCompiler runs before invoking runescript.Compile.
//
// srcDir: directory containing scripts/ and pack/ subdirs.
// dataPackDir: cache directory with the 8 .dat/.idx pairs (read back
// the cache PackConfigs writes).
//
// The NAI-212-D-POINTER-NAME-TRANSLATION translation is applied to the
// "command" entry so callers can invoke runescript.Compile directly
// with the returned map.
func LoadCompilerSymbols(srcDir, dataPackDir string) (map[string]*runescript.CompilerTypeInfo, error) {
	loaders, err := loadConfigs(dataPackDir)
	if err != nil {
		return nil, fmt.Errorf("LoadCompilerSymbols: %w", err)
	}
	bridged, err := bridgedSymbolsCore(srcDir, loaders)
	if err != nil {
		return nil, fmt.Errorf("LoadCompilerSymbols: %w", err)
	}
	return bridged, nil
}

// bridgedSymbolsCore runs buildSymbolsCore and converts each entry into the
// runescript.CompilerTypeInfo shape runescript.Compile consumes, applying
// the NAI-212-D-POINTER-NAME-TRANSLATION rewrite to the "command" entry.
// Shared prep chain between LoadCompilerSymbols and runServerCompilerCore.
//
// NAI-212-D-POINTER-NAME-TRANSLATION: pkg/script.Pointers uses
// "active_player2" / "active_npc2" / "active_loc2" / "active_obj2" /
// "p_active_player2" naming (matching TS ScriptOpcodePointers.ts), but
// pkg/pack/compiler/pointer.ForName resolves by Representation
// (".active_player", ".active_npc", etc.) rather than by static-property
// name ("active_player2"). Translate the "_2" suffixed names in the
// bridged command TypeInfo's pointer maps so LoadSpecialSymbols can
// resolve them. Permanent deviation: the mismatch lives across two
// independently-ported packages; fixing it in pointer/type.go would
// require adding a secondary alias map there.
func bridgedSymbolsCore(srcDir string, loaders *configLoaders) (map[string]*runescript.CompilerTypeInfo, error) {
	symbols, err := buildSymbolsCore(srcDir, loaders)
	if err != nil {
		return nil, err
	}
	bridged := make(map[string]*runescript.CompilerTypeInfo, len(symbols))
	for k, v := range symbols {
		bridged[k] = ToCompilerTypeInfo(v)
	}
	if cmd, ok := bridged["command"]; ok {
		translateCommandPointerNames(cmd)
	}
	return bridged, nil
}
