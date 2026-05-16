// pkg/pack/compiler/runescript/compile_test.go
package runescript

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
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

// TestCompile_JagWriter_PinsScriptHeader runs the full Compile() pipeline
// against a single-proc fixture, then asserts deterministic header bytes
// in the produced script.dat + script.idx files. Replaces the prior
// header-only smoke; the [proc,helper] body exercises codegen
// (PushLocalVar + PushConstantInt + Multiply + Return) so writePhase
// emits a non-empty blob.
//
// Closes NAI-210 follow-up #1 (RICHER-DRIVER-SMOKE) for the Jag sink.
//
// Two driver-only deviations from the codegen smoke test fixture:
//
//   - The runescript CompilerTypeInfo Map registers id 0 → "[proc,helper]"
//     so the SymbolMapper resolves the compiled script to a non-negative
//     id (otherwise IdProvider.Get returns -1, lastID = -1, and the
//     written header bytes collapse to zero).
//   - The sourceName slot in the blob carries the absolute path that
//     filepath.Walk hands to the parser, not the basename. We therefore
//     locate the NUL terminator dynamically and pin only that the slot
//     ends with "helper.rs2".
func TestCompile_JagWriter_PinsScriptHeader(t *testing.T) {
	tmp := t.TempDir()
	scriptsDir := filepath.Join(tmp, "scripts")
	if err := os.MkdirAll(scriptsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(scriptsDir, "helper.rs2"),
		[]byte("[proc,helper](int $n)(int)\nreturn(calc($n * 2));\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	packDir := filepath.Join(tmp, "pack")

	cfg := Config{
		SourcePaths: []string{scriptsDir},
		Symbols: map[string]*CompilerTypeInfo{
			"command":    {Map: map[string]string{"0": "return"}, Require: map[string]string{"0": "active_player"}},
			"runescript": {Map: map[string]string{"0": "[proc,helper]"}},
		},
		Features: semantics.StrictFeatureLevel{},
		Writer:   WriterConfig{Jag: &JagWriterConfig{Output: packDir}},
	}
	if err := Compile(cfg); err != nil {
		t.Fatalf("Compile: %v", err)
	}

	dat, err := os.ReadFile(filepath.Join(packDir, "script.dat"))
	if err != nil {
		t.Fatalf("read script.dat: %v", err)
	}
	idx, err := os.ReadFile(filepath.Join(packDir, "script.idx"))
	if err != nil {
		t.Fatalf("read script.idx: %v", err)
	}

	// script.dat: BE32(lastID+1) + BE32(jagFileVersion=27) + helper blob.
	if got := binary.BigEndian.Uint32(dat[0:4]); got != 1 {
		t.Errorf("script.dat[0:4] lastID+1 = %d, want 1", got)
	}
	if got := binary.BigEndian.Uint32(dat[4:8]); got != 27 {
		t.Errorf("script.dat[4:8] jagFileVersion = %d, want 27", got)
	}
	// Helper blob starts at offset 8.
	if got := string(dat[8:21]); got != "[proc,helper]" {
		t.Errorf("script.dat[8:21] fullName = %q, want %q", got, "[proc,helper]")
	}
	if dat[21] != 0 {
		t.Errorf("script.dat[21] fullName NUL = %#x, want 0", dat[21])
	}
	// sourceName: variable-length absolute path terminated by NUL. Locate
	// the terminator dynamically and pin the basename + NUL position.
	sourceStart := 22
	nulIdx := bytes.IndexByte(dat[sourceStart:], 0)
	if nulIdx < 0 {
		t.Fatalf("script.dat sourceName: no NUL terminator after offset %d", sourceStart)
	}
	sourceName := string(dat[sourceStart : sourceStart+nulIdx])
	if !strings.HasSuffix(sourceName, "helper.rs2") {
		t.Errorf("script.dat sourceName = %q, want suffix %q", sourceName, "helper.rs2")
	}
	lookupOff := sourceStart + nulIdx + 1
	if lookupOff+4 > len(dat) {
		t.Fatalf("script.dat: lookupKey offset %d out of bounds (len=%d)", lookupOff, len(dat))
	}
	if got := int32(binary.BigEndian.Uint32(dat[lookupOff : lookupOff+4])); got != -1 {
		t.Errorf("script.dat[%d:%d] lookupKey = %d, want -1 (ModeName)", lookupOff, lookupOff+4, got)
	}
	debugOff := lookupOff + 4
	if debugOff >= len(dat) {
		t.Fatalf("script.dat: debugproc-zero offset %d out of bounds (len=%d)", debugOff, len(dat))
	}
	if dat[debugOff] != 0 {
		t.Errorf("script.dat[%d] debugproc-zero = %#x, want 0", debugOff, dat[debugOff])
	}
	if len(dat) <= 50 {
		t.Errorf("script.dat len = %d, want > 50 (non-empty opcode stream)", len(dat))
	}

	// script.idx: BE32(lastID+1) + BE32(blobLen).
	if got := binary.BigEndian.Uint32(idx[0:4]); got != 1 {
		t.Errorf("script.idx[0:4] lastID+1 = %d, want 1", got)
	}
	if got := binary.BigEndian.Uint32(idx[4:8]); got <= 30 {
		t.Errorf("script.idx[4:8] blobLen = %d, want > 30", got)
	}
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

// TestCompile_HandlerInjectionUsedDuringRun pins that Config.Handler is
// threaded into the constructed ServerScriptCompiler and receives the
// phase callbacks. Uses an empty SourcePaths + non-empty Symbols to
// reach Run without doing any real work.
func TestCompile_HandlerInjectionUsedDuringRun(t *testing.T) {
	tmpDir := t.TempDir()
	rh := newRecordingHandler()

	// Minimal Symbols sufficient to pass Compile's required-symbols check.
	syms := map[string]*CompilerTypeInfo{
		"command":    {Map: map[string]string{}},
		"runescript": {Map: map[string]string{}},
	}

	err := Compile(Config{
		SourcePaths: []string{tmpDir}, // empty dir → parsePhase walks zero files
		Symbols:     syms,
		Writer:      WriterConfig{Jag: &JagWriterConfig{Output: filepath.Join(tmpDir, "out")}},
		Handler:     rh,
	})
	// CommandPointers stays empty after LoadSpecialSymbols on these empty
	// CompilerTypeInfo maps; HandlePointerChecking is NOT called per
	// NAI-210-D-EMPTYPOINTERS-RETURNS-FALSE.
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	want := []string{"HandleParse", "HandleTypeChecking", "HandleCodeGeneration"}
	if !equalStrings(rh.calls, want) {
		t.Errorf("Compile dispatch order: got %v, want %v", rh.calls, want)
	}
}

// TestCompile_NilHandlerDefaultsToBase pins that a nil Config.Handler is
// replaced with *BaseDiagnosticsHandler before Run() is invoked. Asserts
// no panic + completion; the printed output goes to BaseDiagnosticsHandler's
// default os.Stdout (acceptable for this test — there will be zero
// diagnostics from an empty SourcePaths).
func TestCompile_NilHandlerDefaultsToBase(t *testing.T) {
	tmpDir := t.TempDir()
	syms := map[string]*CompilerTypeInfo{
		"command":    {Map: map[string]string{}},
		"runescript": {Map: map[string]string{}},
	}
	err := Compile(Config{
		SourcePaths: []string{tmpDir},
		Symbols:     syms,
		Writer:      WriterConfig{Jag: &JagWriterConfig{Output: filepath.Join(tmpDir, "out")}},
		Handler:     nil,
	})
	if err != nil {
		t.Fatalf("Compile with nil Handler: %v", err)
	}
}
