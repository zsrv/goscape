package world

// Tests for the MidiPack name→id registry (B3).
// TS ref: tools/pack/PackFileBase.ts:50-71 (load), Player.ts:1919-1933 (producers).
//
// Step-1 failing pins — written before implementation, each must FAIL
// initially, then PASS after the registry lands.

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLoadMidiPackParsesIDEqualsName verifies the local pack parser handles
// "id=name" lines per PackFileBase.ts:58-69. Fixture uses the real format
// from Content/pack/midi.pack (spaces in names, 0-based ids).
func TestLoadMidiPackParsesIDEqualsName(t *testing.T) {
	dir := t.TempDir()
	content := "0=scape main\n1=iban\n2=autumn voyage\n"
	if err := os.WriteFile(filepath.Join(dir, "midi.pack"), []byte(content), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	got := loadMidiPack(filepath.Join(dir, "midi.pack"))

	cases := []struct {
		name string
		id   int
	}{
		{"scape main", 0},
		{"iban", 1},
		{"autumn voyage", 2},
	}
	for _, tc := range cases {
		id, ok := got[tc.name]
		if !ok {
			t.Errorf("loadMidiPack: key %q missing from map", tc.name)
			continue
		}
		if id != tc.id {
			t.Errorf("loadMidiPack[%q] = %d; want %d", tc.name, id, tc.id)
		}
	}
	if _, ok := got["ghost town"]; ok {
		t.Errorf("loadMidiPack: unexpected key %q in map", "ghost town")
	}
}

// TestLoadMidiPackAbsentFileReturnsEmpty mirrors PackFileBase.ts:53-55:
// if file does not exist, load returns without populating the map.
// Go: absent file → empty registry, every lookup returns -1.
func TestLoadMidiPackAbsentFileReturnsEmpty(t *testing.T) {
	got := loadMidiPack("/nonexistent/path/midi.pack")
	if len(got) != 0 {
		t.Errorf("loadMidiPack(absent) = map len %d; want 0", len(got))
	}
}

// TestLoadMidiPackIgnoresNonIDLines verifies that lines not matching
// "^\d+=" (comment lines, blank lines) are skipped per PackFileBase.ts:60.
func TestLoadMidiPackIgnoresNonIDLines(t *testing.T) {
	dir := t.TempDir()
	content := "# comment\n0=scape main\n\n1=iban\nbad line\n"
	if err := os.WriteFile(filepath.Join(dir, "midi.pack"), []byte(content), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	got := loadMidiPack(filepath.Join(dir, "midi.pack"))

	if id, ok := got["scape main"]; !ok || id != 0 {
		t.Errorf("expected scape main→0, got ok=%v id=%d", ok, id)
	}
	if id, ok := got["iban"]; !ok || id != 1 {
		t.Errorf("expected iban→1, got ok=%v id=%d", ok, id)
	}
	if len(got) != 2 {
		t.Errorf("map len = %d; want 2 (only valid id=name lines)", len(got))
	}
}

// TestServerMidiIDByNameFound verifies that (*Server).midiIDByName returns
// the correct id when the name is in the registry.
// PackFileBase.ts:129-131: getByName returns nameToId.get(name) ?? -1.
func TestServerMidiIDByNameFound(t *testing.T) {
	s := &Server{
		midiPack: map[string]int{
			"scape main":     0,
			"sailing voyage": 7,
		},
	}
	if got := s.midiIDByName("scape main"); got != 0 {
		t.Errorf("midiIDByName(%q) = %d; want 0", "scape main", got)
	}
	if got := s.midiIDByName("sailing voyage"); got != 7 {
		t.Errorf("midiIDByName(%q) = %d; want 7", "sailing voyage", got)
	}
}

// TestServerMidiIDByNameNotFound verifies that an unknown name returns -1.
func TestServerMidiIDByNameNotFound(t *testing.T) {
	s := &Server{
		midiPack: map[string]int{"scape main": 0},
	}
	if got := s.midiIDByName("no such song"); got != -1 {
		t.Errorf("midiIDByName(unknown) = %d; want -1", got)
	}
}

// TestServerMidiIDByNameNilRegistry verifies that a nil midiPack (no file
// loaded, degrade path) returns -1 for every lookup without panicking.
func TestServerMidiIDByNameNilRegistry(t *testing.T) {
	s := &Server{midiPack: nil}
	if got := s.midiIDByName("anything"); got != -1 {
		t.Errorf("midiIDByName with nil registry = %d; want -1", got)
	}
}

// TestPlaySongWithRegistryWritesMidiSong pins that PlaySong writes a
// MidiSong packet when the server has a registry entry for the normalized
// name. Player.ts:1922-1925: if (id !== -1) this.write(new MidiSong(id)).
//
// "Scape Main" normalizes to "scape_main"; registry key is "scape main"
// (pack file format). Normalization maps spaces→underscores before lookup,
// so this exercises the TS-faithful asymmetry: PlaySong "Scape Main" →
// "scape_main" which does NOT match "scape main" in the pack. We use
// a registry key without spaces to make the test exercise a successful hit.
func TestPlaySongWithRegistryWritesMidiSong(t *testing.T) {
	p, _ := newTestPlayer(t)
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	p.client.encryptor = enc
	// Use a name that normalizeSongName produces exactly (no spaces in pack key).
	// "fanfare" → lowercase → "fanfare" → no spaces → "fanfare" → strip → "fanfare"
	p.client.server = &Server{
		log: discardLogger(),
		midiPack: map[string]int{
			"fanfare": 42,
		},
	}
	p.PlaySong("Fanfare")
	if n := p.client.bufw.Buffered(); n == 0 {
		t.Errorf("PlaySong(known song) wrote 0 bytes; want >0 (MidiSong packet)")
	}
}

// TestPlaySongUnknownSongNoWrite pins that PlaySong is a silent no-op when
// the normalized name is absent from the registry (id == -1 guard).
func TestPlaySongUnknownSongNoWrite(t *testing.T) {
	p, _ := newTestPlayer(t)
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	p.client.encryptor = enc
	p.client.server = &Server{
		log:      discardLogger(),
		midiPack: map[string]int{"other song": 7},
	}
	p.PlaySong("unknown song")
	if n := p.client.bufw.Buffered(); n != 0 {
		t.Errorf("PlaySong(unknown) wrote %d bytes; want 0", n)
	}
}

// TestPlayJingleWithRegistryWritesMidiJingle pins that PlayJingle writes a
// MidiJingle packet when the name resolves to a valid id.
// Player.ts:1929-1932: if (id !== -1) this.write(new MidiJingle(id, delay)).
// Jingle normalization: lowercase only; "Sailing Journey" → "sailing journey"
// which matches the pack key "sailing journey".
func TestPlayJingleWithRegistryWritesMidiJingle(t *testing.T) {
	p, _ := newTestPlayer(t)
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	p.client.encryptor = enc
	p.client.server = &Server{
		log: discardLogger(),
		midiPack: map[string]int{
			"sailing journey": 225,
		},
	}
	p.PlayJingle(500, "Sailing Journey")
	if n := p.client.bufw.Buffered(); n == 0 {
		t.Errorf("PlayJingle(known jingle) wrote 0 bytes; want >0 (MidiJingle packet)")
	}
}

// TestPlayJingleUnknownNoWrite pins that PlayJingle is a silent no-op when
// the name is absent from the registry.
func TestPlayJingleUnknownNoWrite(t *testing.T) {
	p, _ := newTestPlayer(t)
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	p.client.encryptor = enc
	p.client.server = &Server{
		log:      discardLogger(),
		midiPack: map[string]int{"sailing journey": 225},
	}
	p.PlayJingle(500, "unknown jingle")
	if n := p.client.bufw.Buffered(); n != 0 {
		t.Errorf("PlayJingle(unknown) wrote %d bytes; want 0", n)
	}
}

// TestPlaySongNoServerIsNoOp pins that PlaySong silently degrades when
// p.client.server is nil (bare test player, no server wired).
func TestPlaySongNoServerIsNoOp(t *testing.T) {
	p, _ := newTestPlayer(t)
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	p.client.encryptor = enc
	// p.client.server is nil — degrade path
	p.PlaySong("fanfare")
	if n := p.client.bufw.Buffered(); n != 0 {
		t.Errorf("PlaySong(nil server) wrote %d bytes; want 0", n)
	}
}

// TestPlayJingleNoServerIsNoOp pins that PlayJingle silently degrades when
// p.client.server is nil.
func TestPlayJingleNoServerIsNoOp(t *testing.T) {
	p, _ := newTestPlayer(t)
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	p.client.encryptor = enc
	p.PlayJingle(500, "sailing journey")
	if n := p.client.bufw.Buffered(); n != 0 {
		t.Errorf("PlayJingle(nil server) wrote %d bytes; want 0", n)
	}
}
