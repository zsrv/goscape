package runescript

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zsrv/goscape/pkg/pack/compiler/codegen"
	"github.com/zsrv/goscape/pkg/pack/compiler/symbol"
	"github.com/zsrv/goscape/pkg/pack/compiler/trigger"
)

// TestBinaryFileScriptWriter_OutputScript_WritesFile pins TS L26-30:
// each script is written to <output>/<id> via fs.writeFileSync.
func TestBinaryFileScriptWriter_OutputScript_WritesFile(t *testing.T) {
	tmp := t.TempDir()
	mapper := NewSymbolMapper(nil)

	procTrig := &trigger.TriggerType{ID: 0, Identifier: "proc", SubjectMode: trigger.ModeName}
	scriptSym := &symbol.ServerScriptSymbol{
		ScriptSymbolFields: symbol.ScriptSymbolFields{Trigger: procTrig, Name: "hello"},
	}
	mapper.PutScript(42, "[proc,hello]")

	w, err := NewBinaryFileScriptWriter(tmp, mapper)
	if err != nil {
		t.Fatalf("NewBinaryFileScriptWriter: %v", err)
	}
	rs := &codegen.RuneScript{Symbol: scriptSym, Trigger: procTrig}
	data := []byte{0x01, 0x02, 0x03}
	w.OutputScript(rs, data)

	got, err := os.ReadFile(filepath.Join(tmp, "42"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != string(data) {
		t.Errorf("file contents: got %x, want %x", got, data)
	}
}

// TestBinaryFileScriptWriter_RejectsNonDirectory pins TS L21-23.
func TestBinaryFileScriptWriter_RejectsNonDirectory(t *testing.T) {
	tmp := t.TempDir()
	regularFile := filepath.Join(tmp, "regular")
	if err := os.WriteFile(regularFile, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	mapper := NewSymbolMapper(nil)
	_, err := NewBinaryFileScriptWriter(regularFile, mapper)
	if err == nil {
		t.Fatal("NewBinaryFileScriptWriter on regular file: want error, got nil")
	}
}

// TestBinaryFileScriptWriter_MkdirAll pins TS L17-19.
func TestBinaryFileScriptWriter_MkdirAll(t *testing.T) {
	tmp := t.TempDir()
	deep := filepath.Join(tmp, "a", "b", "c")
	mapper := NewSymbolMapper(nil)
	_, err := NewBinaryFileScriptWriter(deep, mapper)
	if err != nil {
		t.Fatalf("NewBinaryFileScriptWriter: %v", err)
	}
	info, err := os.Stat(deep)
	if err != nil {
		t.Fatalf("Stat created dir: %v", err)
	}
	if !info.IsDir() {
		t.Error("created path is not a directory")
	}
}
