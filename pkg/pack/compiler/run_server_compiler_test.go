package compiler

import (
	"os"
	"path/filepath"
	"testing"
)

// writeFile is a local test helper mirroring pkg/pack's writeFile.
// We can't import pkg/pack from pkg/pack/compiler (cycle), so it's
// duplicated here.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestRunServerCompilerCore_HappyPath_WritesJagOutput pins NAI-212 spec
// §7 RunServerCompiler test 1 (Mitigation A: test seam).
//
// Sets up a minimal scripts dir with one [proc,helper] body, seeds
// script.pack with the matching id, then invokes runServerCompilerCore.
// Asserts the Jag sink wrote both server-side outputs.
func TestRunServerCompilerCore_HappyPath_WritesJagOutput(t *testing.T) {
	tmp := t.TempDir()

	// scripts/helper.rs2 — a proc body that compiles cleanly.
	writeFile(t, filepath.Join(tmp, "scripts", "helper.rs2"),
		"[proc,helper]\nreturn;\n")

	// pack/script.pack — registers id 0 → [proc,helper] so the
	// SymbolMapper resolves the compiled script to a non-negative id.
	writeFile(t, filepath.Join(tmp, "pack", "script.pack"),
		"0=[proc,helper]\n")

	outDir := filepath.Join(tmp, "out")

	if err := runServerCompilerCore(tmp, outDir, emptyConfigLoaders()); err != nil {
		t.Fatalf("runServerCompilerCore: %v", err)
	}

	for _, name := range []string{"script.dat", "script.idx"} {
		p := filepath.Join(outDir, "server", name)
		fi, err := os.Stat(p)
		if err != nil {
			t.Fatalf("missing %s: %v", p, err)
		}
		if fi.Size() == 0 {
			t.Errorf("%s is empty (0 bytes)", p)
		}
	}
}

// TestRunServerCompilerCore_MissingScriptsDir pins NAI-212 spec §7
// RunServerCompiler test 2: tolerance of absent scripts/ dir.
//
// Plan premise (constants-walk error from buildSymbolsCore) did NOT hold:
// loadCompilerConstants returns an empty map (not an error) for a missing
// scripts/ dir (symbols.go:47-51), and filepath.Walk in parsePhase swallows
// ErrNotExist (server_script_compiler.go:152). So runServerCompilerCore
// returns nil and writes header-only Jag output, just like
// TestRunServerCompilerCore_EmptyScriptPack_HeaderOnly.
//
// This test pins the actual no-scripts-dir tolerance contract:
// runServerCompilerCore returns nil and script.dat exists (non-zero size
// since the Jag header always writes 8 bytes).
//
// NAI-212-D-MISSING-SCRIPTS-DIR-TOLERANT: TS
// ServerScriptCompilerApplication.ts would throw if sourcePaths points to a
// missing directory; goscape silently tolerates it (see parsePhase ErrNotExist
// swallow). Permanent deviation — consistent with runescript package contract.
func TestRunServerCompilerCore_MissingScriptsDir(t *testing.T) {
	tmp := t.TempDir()
	// Intentionally no scripts/ or pack/ subdir.
	outDir := filepath.Join(tmp, "out")

	if err := runServerCompilerCore(tmp, outDir, emptyConfigLoaders()); err != nil {
		t.Fatalf("want nil (missing-scripts-dir is tolerated), got: %v", err)
	}

	dat := filepath.Join(outDir, "server", "script.dat")
	fi, err := os.Stat(dat)
	if err != nil {
		t.Fatalf("missing %s: %v", dat, err)
	}
	if fi.Size() == 0 {
		t.Errorf("script.dat is empty (0 bytes); want at least 8 for the Jag header")
	}
}

// TestRunServerCompilerCore_EmptyScriptPack_HeaderOnly pins the
// degenerate path: no pack/script.pack on disk. Per [[plan_compile_facade_runescript_map_seeding]],
// Load returns an empty *TypeInfo, bridge produces an empty Map,
// SymbolMapper.Get returns -1 for the lone compiled proc, and the
// Jag writer emits an 8-byte zero-header file with no blob. Compile
// returns nil. Asserts header-only output shape.
func TestRunServerCompilerCore_EmptyScriptPack_HeaderOnly(t *testing.T) {
	tmp := t.TempDir()
	// Scripts dir present, one parseable proc, but no pack/script.pack.
	writeFile(t, filepath.Join(tmp, "scripts", "helper.rs2"),
		"[proc,helper]\nreturn;\n")

	outDir := filepath.Join(tmp, "out")
	if err := runServerCompilerCore(tmp, outDir, emptyConfigLoaders()); err != nil {
		t.Fatalf("runServerCompilerCore: %v", err)
	}

	dat := filepath.Join(outDir, "server", "script.dat")
	fi, err := os.Stat(dat)
	if err != nil {
		t.Fatalf("missing %s: %v", dat, err)
	}
	if fi.Size() != 8 {
		t.Errorf("script.dat size = %d, want 8 (header-only when SymbolMapper has no mapping)", fi.Size())
	}
}
