package versionlist

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zsrv/goscape/pkg/io/filestream"
	"github.com/zsrv/goscape/pkg/io/jagfile"
	"github.com/zsrv/goscape/pkg/io/packet"
)

// buildVersionlistCache writes a versionlist jagfile containing the given members
// into cacheDir (archive 0 / file 5).
func buildVersionlistCache(t *testing.T, cacheDir string, members map[string][]byte) {
	t.Helper()
	vl := jagfile.NewEmptyJagfile(false)
	for name, data := range members {
		vl.Write(name, packet.NewPacket(data))
	}
	tmp := filepath.Join(t.TempDir(), "vl.jag")
	if err := vl.Save(tmp); err != nil {
		t.Fatalf("vl.Save: %v", err)
	}
	vlBytes, err := os.ReadFile(tmp)
	if err != nil {
		t.Fatalf("readFile vl: %v", err)
	}
	fs2 := filestream.New(cacheDir, true, false)
	if !fs2.Write(0, 5, vlBytes, 0) {
		t.Fatal("write versionlist to cache failed")
	}
	fs2.Close()
}

// ---- AnimIndex tests ----

// TestAnimIndex_BasicOutput verifies the exact console.log format for a few
// representative flag values.
func TestAnimIndex_BasicOutput(t *testing.T) {
	// Build anim_index: g2 values [0, 1, 255, 256, 65535].
	// Each g2 is big-endian 2 bytes.
	flags := []uint16{0, 1, 255, 256, 65535}
	indexBytes := make([]byte, len(flags)*2)
	for i, f := range flags {
		indexBytes[i*2] = byte(f >> 8)
		indexBytes[i*2+1] = byte(f)
	}

	cacheDir := t.TempDir()
	buildVersionlistCache(t, cacheDir, map[string][]byte{"anim_index": indexBytes})

	var out bytes.Buffer
	if err := AnimIndex(cacheDir, &out); err != nil {
		t.Fatalf("AnimIndex: %v", err)
	}

	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	if len(lines) != len(flags) {
		t.Fatalf("got %d lines, want %d", len(lines), len(flags))
	}

	// Expected format: "%d %08b %d\n"
	// i=0: flags=0     → "0 00000000 0"
	// i=1: flags=1     → "1 00000001 1"
	// i=2: flags=255   → "2 11111111 255"
	// i=3: flags=256   → "3 100000000 256"  (>8 bits: no truncation, padStart is no-op)
	// i=4: flags=65535 → "4 1111111111111111 65535"
	want := []string{
		"0 00000000 0",
		"1 00000001 1",
		"2 11111111 255",
		"3 100000000 256",
		"4 1111111111111111 65535",
	}
	for i, w := range want {
		if lines[i] != w {
			t.Errorf("line %d: got %q, want %q", i, lines[i], w)
		}
	}
}

// TestAnimIndex_NilOut verifies that nil out does not panic.
func TestAnimIndex_NilOut(t *testing.T) {
	cacheDir := t.TempDir()
	buildVersionlistCache(t, cacheDir, map[string][]byte{"anim_index": []byte{0, 0}})
	if err := AnimIndex(cacheDir, nil); err != nil {
		t.Fatalf("AnimIndex with nil out: %v", err)
	}
}

// ---- MidiIndex tests ----

// TestMidiIndex_WithRegistry verifies that midi names from the pack registry
// are rendered correctly and absent entries emit "undefined".
func TestMidiIndex_WithRegistry(t *testing.T) {
	// midi_index: 3 entries with prefetch bytes [0, 1, 0].
	midiIndexBytes := []byte{0, 1, 0}

	cacheDir := t.TempDir()
	buildVersionlistCache(t, cacheDir, map[string][]byte{"midi_index": midiIndexBytes})

	// Build srcDir with midi.pack: id=0 → "guthix", id=2 absent.
	srcDir := t.TempDir()
	packDir := filepath.Join(srcDir, "pack")
	if err := os.MkdirAll(packDir, 0o755); err != nil {
		t.Fatalf("mkdir pack: %v", err)
	}
	// Only id=0 registered; id=1 and id=2 absent.
	if err := os.WriteFile(filepath.Join(packDir, "midi.pack"), []byte("0=guthix\n"), 0o644); err != nil {
		t.Fatalf("write midi.pack: %v", err)
	}

	var out bytes.Buffer
	if err := MidiIndex(cacheDir, srcDir, &out); err != nil {
		t.Fatalf("MidiIndex: %v", err)
	}

	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("got %d lines, want 3", len(lines))
	}

	// i=0: registered "guthix", prefetch=0 → "0 guthix 0"
	// i=1: absent → "undefined", prefetch=1 → "1 undefined 1"
	// i=2: absent → "undefined", prefetch=0 → "2 undefined 0"
	want := []string{
		"0 guthix 0",
		"1 undefined 1",
		"2 undefined 0",
	}
	for i, w := range want {
		if lines[i] != w {
			t.Errorf("line %d: got %q, want %q", i, lines[i], w)
		}
	}
}

// TestMidiIndex_EmptyRegistry verifies that all ids emit "undefined" when
// midi.pack is absent.
func TestMidiIndex_EmptyRegistry(t *testing.T) {
	midiIndexBytes := []byte{0, 1}
	cacheDir := t.TempDir()
	buildVersionlistCache(t, cacheDir, map[string][]byte{"midi_index": midiIndexBytes})

	srcDir := t.TempDir() // no midi.pack

	var out bytes.Buffer
	if err := MidiIndex(cacheDir, srcDir, &out); err != nil {
		t.Fatalf("MidiIndex: %v", err)
	}

	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	want := []string{"0 undefined 0", "1 undefined 1"}
	for i, w := range want {
		if i >= len(lines) || lines[i] != w {
			t.Errorf("line %d: got %q, want %q", i, lines[i], w)
		}
	}
}

// ---- ModelIndex tests ----

// TestModelIndex_FlagDecoding verifies that each individual bit flag produces
// the correct readable name and that multi-bit combos are decoded in order.
func TestModelIndex_FlagDecoding(t *testing.T) {
	// One entry per flag bit + zero + a multi-bit combo.
	flags := []byte{
		0x00,                      // none
		0x01,                      // tutorial
		0x02,                      // dynamic
		0x04,                      // static
		0x08,                      // wornf2p
		0x10,                      // worn
		0x20,                      // invf2p
		0x40,                      // inv
		0x80,                      // player
		0x01 | 0x02 | 0x80,        // tutorial dynamic player
		0x08 | 0x10 | 0x20 | 0x40, // wornf2p worn invf2p inv
	}

	cacheDir := t.TempDir()
	buildVersionlistCache(t, cacheDir, map[string][]byte{"model_index": flags})

	srcDir := t.TempDir() // no model.pack → ids fall back to numeric

	if err := ModelIndex(cacheDir, srcDir, nil); err != nil {
		t.Fatalf("ModelIndex: %v", err)
	}

	// Read model_index.txt.
	txtPath := filepath.Join(cacheDir, "model_index.txt")
	got, err := os.ReadFile(txtPath)
	if err != nil {
		t.Fatalf("read model_index.txt: %v", err)
	}

	lines := strings.Split(strings.TrimRight(string(got), "\n"), "\n")
	if len(lines) != len(flags) {
		t.Fatalf("got %d lines, want %d\n%s", len(lines), len(flags), got)
	}

	// Expected: "<i>=<readable>, 0x<hex2> (0b<binary8>)"
	want := []string{
		"0=none, 0x00 (0b00000000)",
		"1=tutorial, 0x01 (0b00000001)",
		"2=dynamic, 0x02 (0b00000010)",
		"3=static, 0x04 (0b00000100)",
		"4=wornf2p, 0x08 (0b00001000)",
		"5=worn, 0x10 (0b00010000)",
		"6=invf2p, 0x20 (0b00100000)",
		"7=inv, 0x40 (0b01000000)",
		"8=player, 0x80 (0b10000000)",
		"9=tutorial dynamic player, 0x83 (0b10000011)",
		"10=wornf2p worn invf2p inv, 0x78 (0b01111000)",
	}
	for i, w := range want {
		if lines[i] != w {
			t.Errorf("line %d: got %q, want %q", i, lines[i], w)
		}
	}
}

// TestModelIndex_NameFromPack verifies that ModelPack names replace numeric ids
// in the output, and that absent ids still fall back to the numeric form.
func TestModelIndex_NameFromPack(t *testing.T) {
	flags := []byte{0x00, 0x01} // id=0 has no pack name; id=1 has "player_body"

	cacheDir := t.TempDir()
	buildVersionlistCache(t, cacheDir, map[string][]byte{"model_index": flags})

	srcDir := t.TempDir()
	packDir := filepath.Join(srcDir, "pack")
	if err := os.MkdirAll(packDir, 0o755); err != nil {
		t.Fatalf("mkdir pack: %v", err)
	}
	if err := os.WriteFile(filepath.Join(packDir, "model.pack"), []byte("1=player_body\n"), 0o644); err != nil {
		t.Fatalf("write model.pack: %v", err)
	}

	if err := ModelIndex(cacheDir, srcDir, nil); err != nil {
		t.Fatalf("ModelIndex: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(cacheDir, "model_index.txt"))
	if err != nil {
		t.Fatalf("read model_index.txt: %v", err)
	}

	lines := strings.Split(strings.TrimRight(string(got), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2: %s", len(lines), got)
	}
	// id=0: no name → numeric "0"
	if want := "0=none, 0x00 (0b00000000)"; lines[0] != want {
		t.Errorf("line 0: got %q, want %q", lines[0], want)
	}
	// id=1: name "player_body"
	if want := "player_body=tutorial, 0x01 (0b00000001)"; lines[1] != want {
		t.Errorf("line 1: got %q, want %q", lines[1], want)
	}
}

// TestModelIndex_WritesRawIndex verifies that model_index (the raw bytes file)
// is written with the exact jagfile member bytes.
func TestModelIndex_WritesRawIndex(t *testing.T) {
	flags := []byte{0x00, 0x42, 0xFF}
	cacheDir := t.TempDir()
	buildVersionlistCache(t, cacheDir, map[string][]byte{"model_index": flags})
	srcDir := t.TempDir()

	if err := ModelIndex(cacheDir, srcDir, nil); err != nil {
		t.Fatalf("ModelIndex: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(cacheDir, "model_index"))
	if err != nil {
		t.Fatalf("read model_index: %v", err)
	}
	if !bytes.Equal(got, flags) {
		t.Errorf("model_index: got %v, want %v", got, flags)
	}
}
