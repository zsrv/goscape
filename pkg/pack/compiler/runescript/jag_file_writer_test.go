package runescript

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/zsrv/goscape/pkg/pack/compiler/codegen"
	"github.com/zsrv/goscape/pkg/pack/compiler/symbol"
	"github.com/zsrv/goscape/pkg/pack/compiler/trigger"
)

// TestJagFileScriptWriter_HappyPath pins close-time output for ids {0, 2}:
//   - script.idx: P4(3) + P4(len[0]) + P4(0 gap) + P4(len[2])
//   - script.dat: P4(3) + P4(27 version) + data[0] + data[2]
//
// Per TS L42-72.
func TestJagFileScriptWriter_HappyPath(t *testing.T) {
	tmp := t.TempDir()
	mapper := NewSymbolMapper(nil)

	procTrig := &trigger.TriggerType{ID: 0, Identifier: "proc", SubjectMode: trigger.ModeName}
	for _, id := range []int{0, 2} {
		mapper.PutScript(id, "[proc,s"+strconv.Itoa(id)+"]")
	}

	w, err := NewJagFileScriptWriter(tmp, mapper)
	if err != nil {
		t.Fatalf("NewJagFileScriptWriter: %v", err)
	}
	w.OutputScript(&codegen.RuneScript{
		Symbol:  &symbol.ServerScriptSymbol{ScriptSymbolFields: symbol.ScriptSymbolFields{Trigger: procTrig, Name: "s0"}},
		Trigger: procTrig,
	}, []byte{0xAA, 0xBB})
	w.OutputScript(&codegen.RuneScript{
		Symbol:  &symbol.ServerScriptSymbol{ScriptSymbolFields: symbol.ScriptSymbolFields{Trigger: procTrig, Name: "s2"}},
		Trigger: procTrig,
	}, []byte{0xCC, 0xDD, 0xEE})

	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	idx, err := os.ReadFile(filepath.Join(tmp, "script.idx"))
	if err != nil {
		t.Fatal(err)
	}
	wantIdx := []byte{
		0x00, 0x00, 0x00, 0x03, // count = lastID+1 = 3
		0x00, 0x00, 0x00, 0x02, // len[0] = 2
		0x00, 0x00, 0x00, 0x00, // gap at id=1
		0x00, 0x00, 0x00, 0x03, // len[2] = 3
	}
	if !bytes.Equal(idx, wantIdx) {
		t.Errorf("script.idx: got %x, want %x", idx, wantIdx)
	}

	dat, err := os.ReadFile(filepath.Join(tmp, "script.dat"))
	if err != nil {
		t.Fatal(err)
	}
	wantDat := []byte{
		0x00, 0x00, 0x00, 0x03, // count
		0x00, 0x00, 0x00, 0x1A, // version = 26
		0xAA, 0xBB, // data[0]
		0xCC, 0xDD, 0xEE, // data[2]
	}
	if !bytes.Equal(dat, wantDat) {
		t.Errorf("script.dat: got %x, want %x", dat, wantDat)
	}
}

// TestJagFileScriptWriter_EmptyClose pins behavior when no scripts written.
// TS L51 fallback: lastId = 0 when keys empty. Loop runs once at i=0:
// buffers.get(0) undefined → idx gets P4(0), dat unchanged.
func TestJagFileScriptWriter_EmptyClose(t *testing.T) {
	tmp := t.TempDir()
	mapper := NewSymbolMapper(nil)
	w, err := NewJagFileScriptWriter(tmp, mapper)
	if err != nil {
		t.Fatalf("NewJagFileScriptWriter: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	idx, err := os.ReadFile(filepath.Join(tmp, "script.idx"))
	if err != nil {
		t.Fatal(err)
	}
	wantIdx := []byte{0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00} // count(1) + gap[0]
	if !bytes.Equal(idx, wantIdx) {
		t.Errorf("empty idx: got %x, want %x", idx, wantIdx)
	}
	dat, err := os.ReadFile(filepath.Join(tmp, "script.dat"))
	if err != nil {
		t.Fatal(err)
	}
	wantDat := []byte{0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x1A} // count(1) + version(26)
	if !bytes.Equal(dat, wantDat) {
		t.Errorf("empty dat: got %x, want %x", dat, wantDat)
	}
}
