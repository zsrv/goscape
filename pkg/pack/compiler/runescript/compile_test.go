// pkg/pack/compiler/runescript/compile_test.go
package runescript

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zsrv/goscape/pkg/pack/compiler/semantics"
)

// TestCompile_MissingCoreSymbols_ReturnsError pins TS L16-18: error when
// command or runescript symbol info is missing.
func TestCompile_MissingCoreSymbols_ReturnsError(t *testing.T) {
	cases := []struct {
		name    string
		symbols map[string]*CompilerTypeInfo
	}{
		{"nil map", nil},
		{"command only", map[string]*CompilerTypeInfo{"command": {Map: map[string]string{}}}},
		{"runescript only", map[string]*CompilerTypeInfo{"runescript": {Map: map[string]string{}}}},
		{"empty map", map[string]*CompilerTypeInfo{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := Compile(Config{Symbols: c.symbols})
			if err == nil {
				t.Errorf("Compile %s: want error, got nil", c.name)
			}
		})
	}
}

// TestCompile_JagWriter_EndToEnd compiles a trivial fixture and verifies
// that the call returns no error. File production depends on the fixture
// reaching writePhase through the full pipeline.
func TestCompile_JagWriter_EndToEnd(t *testing.T) {
	tmp := t.TempDir()
	scriptsDir := filepath.Join(tmp, "scripts")
	if err := os.MkdirAll(scriptsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(scriptsDir, "hello.rs2"),
		[]byte("[proc,hello]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	packDir := filepath.Join(tmp, "pack")

	cfg := Config{
		SourcePaths: []string{scriptsDir},
		Symbols: map[string]*CompilerTypeInfo{
			"command":    {Map: map[string]string{"0": "return"}, Require: map[string]string{"0": "active_player"}},
			"runescript": {Map: map[string]string{}},
		},
		Features: semantics.StrictFeatureLevel{},
		Writer:   WriterConfig{Jag: &JagWriterConfig{Output: packDir}},
	}
	if err := Compile(cfg); err != nil {
		t.Fatalf("Compile: %v", err)
	}
	_ = packDir
}

// TestCompile_Js5Writer_EndToEnd ports the same scenario for Js5 writer.
func TestCompile_Js5Writer_EndToEnd(t *testing.T) {
	tmp := t.TempDir()
	scriptsDir := filepath.Join(tmp, "scripts")
	if err := os.MkdirAll(scriptsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(scriptsDir, "hello.rs2"),
		[]byte("[proc,hello]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	js5Out := filepath.Join(tmp, "pack", "scripts.js5")

	cfg := Config{
		SourcePaths: []string{scriptsDir},
		Symbols: map[string]*CompilerTypeInfo{
			"command":    {Map: map[string]string{"0": "return"}, Require: map[string]string{"0": "active_player"}},
			"runescript": {Map: map[string]string{}},
		},
		Features: semantics.StrictFeatureLevel{},
		Writer:   WriterConfig{Js5: &Js5WriterConfig{Output: js5Out}},
	}
	if err := Compile(cfg); err != nil {
		t.Fatalf("Compile: %v", err)
	}
	_ = js5Out
}
