package runescript

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zsrv/goscape/pkg/pack/compiler/codegen"
	"github.com/zsrv/goscape/pkg/pack/compiler/symbol"
	"github.com/zsrv/goscape/pkg/pack/compiler/trigger"
)

// TestJs5PackScriptWriter_HappyPath pins file existence + GZIP byte-9 zero.
func TestJs5PackScriptWriter_HappyPath(t *testing.T) {
	tmp := t.TempDir()
	out := filepath.Join(tmp, "pack.js5")
	mapper := NewSymbolMapper(nil)
	procTrig := &trigger.TriggerType{ID: 0, Identifier: "proc", SubjectMode: trigger.ModeName}
	mapper.PutScript(0, "[proc,s0]")
	mapper.PutScript(1, "[proc,s1]")

	w, err := NewJs5PackScriptWriter(out, mapper)
	if err != nil {
		t.Fatalf("NewJs5PackScriptWriter: %v", err)
	}
	w.OutputScript(&codegen.RuneScript{
		Symbol:  &symbol.ServerScriptSymbol{ScriptSymbolFields: symbol.ScriptSymbolFields{Trigger: procTrig, Name: "s0"}},
		Trigger: procTrig,
	}, []byte{0xAA, 0xBB})
	w.OutputScript(&codegen.RuneScript{
		Symbol:  &symbol.ServerScriptSymbol{ScriptSymbolFields: symbol.ScriptSymbolFields{Trigger: procTrig, Name: "s1"}},
		Trigger: procTrig,
	}, []byte{0xCC, 0xDD, 0xEE})
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	body, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(body) < 20 {
		t.Fatalf(".js5 too short: %d bytes", len(body))
	}

	// First byte: compression type = 2 (GZIP) for the packed-index group.
	if body[0] != 0x02 {
		t.Errorf("packed index group compression byte: got %d, want 2", body[0])
	}

	// NAI-210-D-GZIP-OS-BYTE-ZEROED: GZIP magic 0x1F 0x8B starts at offset 9
	// (after compression=1 + compressedLen=4 + uncompressedLen=4 = 9 bytes
	// of prefix). OS byte sits at gzip-offset 9 → file offset 18.
	if body[18] != 0x00 {
		t.Errorf("GZIP OS byte at offset 18: got 0x%02x, want 0x00 (NAI-210-D-GZIP-OS-BYTE-ZEROED)", body[18])
	}
}

// TestJs5PackScriptWriter_MissingParentDirIsCreated pins TS L33-37.
func TestJs5PackScriptWriter_MissingParentDirIsCreated(t *testing.T) {
	tmp := t.TempDir()
	deep := filepath.Join(tmp, "a", "b", "pack.js5")
	mapper := NewSymbolMapper(nil)
	w, err := NewJs5PackScriptWriter(deep, mapper)
	if err != nil {
		t.Fatalf("NewJs5PackScriptWriter: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := os.Stat(deep); err != nil {
		t.Errorf("output not created: %v", err)
	}
}

// TestJs5PackScriptWriter_OutputScript_ClonesData pins retain-semantics.
// TS L46-49: Buffer.from(data) is a copy; mutating the input afterward must
// not affect what gets written.
func TestJs5PackScriptWriter_OutputScript_ClonesData(t *testing.T) {
	tmp := t.TempDir()
	out := filepath.Join(tmp, "pack.js5")
	mapper := NewSymbolMapper(nil)
	procTrig := &trigger.TriggerType{ID: 0, Identifier: "proc", SubjectMode: trigger.ModeName}
	mapper.PutScript(0, "[proc,s0]")
	w, err := NewJs5PackScriptWriter(out, mapper)
	if err != nil {
		t.Fatalf("NewJs5PackScriptWriter: %v", err)
	}
	data := []byte{0x11, 0x22, 0x33}
	w.OutputScript(&codegen.RuneScript{
		Symbol:  &symbol.ServerScriptSymbol{ScriptSymbolFields: symbol.ScriptSymbolFields{Trigger: procTrig, Name: "s0"}},
		Trigger: procTrig,
	}, data)
	data[0] = 0xFF // mutate after handoff
	stored := w.buffers[0]
	if stored[0] != 0x11 {
		t.Errorf("OutputScript did not clone: stored[0]=0x%02x, want 0x11", stored[0])
	}
}
