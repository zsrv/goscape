package sound

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

// --- Low-level encoding helpers matching TS Packet write ops ---

// pEnvelope appends a single Envelope payload to buf:
//
//	form    g1
//	start   g4s (big-endian int32)
//	end     g4s
//	length  g1 (number of shape segments)
//	per segment: shapeDelta g2, shapePeak g2
func pEnvelope(buf *packet.Packet, form uint8, start, end int32, shape [][2]uint16) {
	buf.P1(form)
	// g4s: write big-endian int32.
	v := uint32(start)
	buf.P1(uint8(v >> 24))
	buf.P1(uint8(v >> 16))
	buf.P1(uint8(v >> 8))
	buf.P1(uint8(v))
	v = uint32(end)
	buf.P1(uint8(v >> 24))
	buf.P1(uint8(v >> 16))
	buf.P1(uint8(v >> 8))
	buf.P1(uint8(v))
	buf.P1(uint8(len(shape)))
	for _, seg := range shape {
		buf.P2(seg[0])
		buf.P2(seg[1])
	}
}

// pSmartS appends an unsigned smart value (GSmartS / gsmarts):
//   - 0..127  → 1 byte
//   - 128..32767 → 2 bytes (high bit set, value + 32768)
func pSmartS(buf *packet.Packet, val uint16) {
	if val < 128 {
		buf.P1(uint8(val))
	} else {
		buf.P2(val + 32768)
	}
}

// pSmart appends a signed smart value (GSmart / gsmart):
//   - shifted such that val+64 in 0..127 → 1 byte
//   - otherwise 2 bytes (val+49152 as uint16)
func pSmart(buf *packet.Packet, val int16) {
	shifted := int(val) + 64
	if shifted >= 0 && shifted <= 127 {
		buf.P1(uint8(shifted))
	} else {
		buf.P2(uint16(int(val) + 49152))
	}
}

// pTone appends a complete Tone payload to buf, mirroring Tone.unpack read sequence.
//
// Optional pairs are controlled by withFreqMod, withAmpMod, withRelAtk.
//
// Crucially, for the optional-pair gate the TS does:
//
//	if (buf.g1() != 0) { buf.pos--; envelope.unpack(buf); … }
//
// g1() reads the byte; if non-zero, pos-- and Envelope.unpack reads the SAME byte
// as its form field.  So to encode "pair present", we write the first envelope's
// form byte directly (it doubles as the non-zero probe).  To encode "pair absent",
// we write a zero probe byte.
//
// harmonics is a slice of (volume>0, semitone, delay).  The loop is terminated by
// a zero GSmartS (volume==0).
func pTone(
	buf *packet.Packet,
	withFreqMod, withAmpMod, withRelAtk bool,
	harmonics [][3]int, // (volume int > 0, semitone int signed, delay int >= 0)
	reverbDelay, reverbVolume uint16,
	length, start uint16,
) {
	// frequencyBase (always present).
	pEnvelope(buf, 1, 0, 100, nil)
	// amplitudeBase (always present).
	pEnvelope(buf, 2, 0, 200, nil)

	// Optional frequencyMod pair.
	if withFreqMod {
		// form=3 is non-zero → acts as probe; pos-- in TS re-reads it as form.
		pEnvelope(buf, 3, 0, 300, nil)
		pEnvelope(buf, 4, 0, 400, nil)
	} else {
		buf.P1(0)
	}

	// Optional amplitudeMod pair.
	if withAmpMod {
		pEnvelope(buf, 5, 0, 500, nil)
		pEnvelope(buf, 6, 0, 600, nil)
	} else {
		buf.P1(0)
	}

	// Optional release/attack pair.
	if withRelAtk {
		pEnvelope(buf, 7, 0, 700, nil)
		pEnvelope(buf, 8, 0, 800, nil)
	} else {
		buf.P1(0)
	}

	// Harmonics loop: emit (volume, semitone, delay) tuples; terminate with volume=0.
	for _, h := range harmonics {
		pSmartS(buf, uint16(h[0]))
		pSmart(buf, int16(h[1]))
		pSmartS(buf, uint16(h[2]))
	}
	pSmartS(buf, 0) // terminator

	// reverbDelay, reverbVolume, length, start.
	pSmartS(buf, reverbDelay)
	pSmartS(buf, reverbVolume)
	buf.P2(length)
	buf.P2(start)

	// rev-274 Filter: every Tone ends with at least the filter count byte
	// (TS Tone.unpack @dee467c8 unconditionally calls Filter.unpack). count=0
	// means "no filter" — the parser consumes exactly the one count byte.
	buf.P1(0)
}

// pFilter appends a Filter payload to buf, mirroring TS Filter.unpack
// (tools/unpack/sound/Unpack.ts @dee467c8).
//
// pairs0/pairs1 are the per-direction pair counts (0..15 each); they are packed
// into the count byte as (pairs0<<4)|pairs1.  migration is the per-direction/per-pair
// migration bitmask.  withShape forces an Envelope.unpackShape payload (length=0,
// no segments) when count != 0 — TS emits it when migration != 0 || unities differ.
//
// To keep the encoder deterministic and the extent math simple, every g2 value
// written here is a fixed placeholder (the unpacker only advances buf.Pos; it
// does not assert values).
func pFilter(buf *packet.Packet, pairs0, pairs1 int, migration uint8, unity0, unity1 uint16, withShape bool) {
	count := (pairs0 << 4) | pairs1
	buf.P1(uint8(count))
	if count == 0 {
		return
	}

	buf.P2(unity0) // unities[0]
	buf.P2(unity1) // unities[1]
	buf.P1(migration)

	pairs := [2]int{pairs0, pairs1}

	// Phase 1: for each direction, pairs[direction] × (g2 freq, g2 range).
	for direction := range 2 {
		for range pairs[direction] {
			buf.P2(1000) // frequencies[direction][0][pair]
			buf.P2(2000) // ranges[direction][0][pair]
		}
	}

	// Phase 2: for each direction/pair, if the migration bit is set, g2 freq + g2 range.
	for direction := range 2 {
		for pair := range pairs[direction] {
			if migration&((1<<(direction*4))<<pair) != 0 {
				buf.P2(3000) // frequencies[direction][1][pair]
				buf.P2(4000) // ranges[direction][1][pair]
			}
		}
	}

	// Trailing envelope shape: TS emits envelope.unpackShape when
	// migration != 0 || unities[1] != unities[0].
	if withShape {
		buf.P1(0) // shape length = 0 (no segments)
	}
}

// toneBytesWithFilter builds a Tone whose trailing Filter is the supplied
// custom payload (instead of the default count=0).  It reuses pTone's body up
// to the reverb/length/start tail, then overrides the final filter byte.
func toneBytesWithFilter(
	withFreqMod, withAmpMod, withRelAtk bool,
	harmonics [][3]int,
	reverbDelay, reverbVolume, length, start uint16,
	filter []byte,
) []byte {
	buf := &packet.Packet{}
	pTone(buf, withFreqMod, withAmpMod, withRelAtk, harmonics, reverbDelay, reverbVolume, length, start)
	// pTone appends a trailing count=0 filter byte; strip it and substitute the
	// custom filter payload.
	buf.Data = buf.Data[:len(buf.Data)-1]
	buf.Write(filter)
	return buf.Data
}

// toneBytes returns the encoded bytes for one Tone by calling pTone.
func toneBytes(
	withFreqMod, withAmpMod, withRelAtk bool,
	harmonics [][3]int,
	reverbDelay, reverbVolume, length, start uint16,
) []byte {
	buf := &packet.Packet{}
	pTone(buf, withFreqMod, withAmpMod, withRelAtk, harmonics, reverbDelay, reverbVolume, length, start)
	return buf.Data
}

// pWave appends a complete Wave payload to buf.
// toneData is indexed 0..9; nil entries emit a zero probe byte (absent tone).
func pWave(buf *packet.Packet, toneData [10][]byte, loopBegin, loopEnd uint16) {
	for _, td := range toneData {
		if td == nil {
			buf.P1(0) // probe == 0 → skip
		} else {
			// The first byte of td is the form byte of the tone's frequencyBase
			// envelope, which acts as the non-zero probe (form=1 → 1 != 0).
			buf.Write(td)
		}
	}
	buf.P2(loopBegin)
	buf.P2(loopEnd)
}

// soundSpec describes one sound for buildSoundsDat.
type soundSpec struct {
	id        uint16
	tones     [10][]byte
	loopBegin uint16
	loopEnd   uint16
}

// buildSoundsDat constructs the raw sounds.dat bytes for a list of sounds.
// Each entry provides id, the 10 tone slots (nil = absent), and loop params.
func buildSoundsDat(sounds []soundSpec) []byte {
	buf := &packet.Packet{}
	for _, s := range sounds {
		buf.P2(s.id)
		pWave(buf, s.tones, s.loopBegin, s.loopEnd)
	}
	buf.P2(65535) // terminator
	return buf.Data
}

// buildTestCache writes a minimal FileStream cache with archive 0, file 8
// containing a Jagfile whose 'sounds.dat' member is soundsDatBytes.
// No decompression needed — Unpack calls cache.Read(0, 8, false).
func buildTestCache(t *testing.T, cacheDir string, soundsDatBytes []byte) {
	t.Helper()

	jf := jagfile.NewEmptyJagfile(false)
	jf.Write("sounds.dat", packet.NewPacket(soundsDatBytes))

	tmp := filepath.Join(t.TempDir(), "sounds.jag")
	if err := jf.Save(tmp); err != nil {
		t.Fatalf("jf.Save: %v", err)
	}
	jagBytes, err := os.ReadFile(tmp)
	if err != nil {
		t.Fatalf("ReadFile jag: %v", err)
	}

	fs2 := filestream.New(cacheDir, true, false)
	if !fs2.Write(0, 8, jagBytes, 0) {
		t.Fatal("write archive 0/file 8 to cache failed")
	}
	fs2.Close()
}

// --- Unit tests ---

// TestUnpack_TwoSounds verifies that two sounds are extracted with exact byte
// boundaries: each .synth file's content must equal the corresponding slice of
// sounds.dat (start..end around the wave instance unpack, NOT including the g2 id).
func TestUnpack_TwoSounds(t *testing.T) {
	tone0 := toneBytes(false, false, false, nil, 0, 0, 100, 0)
	tone1 := toneBytes(false, false, false, nil, 0, 0, 200, 10)

	var tones0, tones1 [10][]byte
	tones0[0] = tone0
	tones1[0] = tone1

	soundsDat := buildSoundsDat([]soundSpec{
		{id: 0, tones: tones0, loopBegin: 1, loopEnd: 2},
		{id: 1, tones: tones1, loopBegin: 3, loopEnd: 4},
	})

	// Walk the soundsDat bytes manually to find the extent of each sound:
	// skip the g2 id (2 bytes); extent is the wave instance payload.
	buf := packet.NewPacket(soundsDat)
	var extents [][2]int // [start, end) pairs
	for buf.Len() > 0 {
		id := buf.G2()
		if id == 65535 {
			break
		}
		start := buf.Pos
		if err := parseWave(buf); err != nil {
			t.Fatalf("parseWave: %v", err)
		}
		extents = append(extents, [2]int{start, buf.Pos})
	}

	if len(extents) != 2 {
		t.Fatalf("expected 2 extents, got %d", len(extents))
	}

	cacheDir := t.TempDir()
	srcDir := t.TempDir()
	buildTestCache(t, cacheDir, soundsDat)

	if err := Unpack(Options{CacheDir: cacheDir, SrcDir: srcDir}); err != nil {
		t.Fatalf("Unpack: %v", err)
	}

	// sound_0.synth should equal soundsDat[extents[0][0]:extents[0][1]].
	want0 := soundsDat[extents[0][0]:extents[0][1]]
	got0, err := os.ReadFile(filepath.Join(srcDir, "synth", "sound_0.synth"))
	if err != nil {
		t.Fatalf("read sound_0.synth: %v", err)
	}
	if !bytes.Equal(got0, want0) {
		t.Errorf("sound_0.synth bytes mismatch\n  want len=%d %x\n   got len=%d %x", len(want0), want0, len(got0), got0)
	}

	// sound_1.synth should equal soundsDat[extents[1][0]:extents[1][1]].
	want1 := soundsDat[extents[1][0]:extents[1][1]]
	got1, err := os.ReadFile(filepath.Join(srcDir, "synth", "sound_1.synth"))
	if err != nil {
		t.Fatalf("read sound_1.synth: %v", err)
	}
	if !bytes.Equal(got1, want1) {
		t.Errorf("sound_1.synth bytes mismatch\n  want len=%d %x\n   got len=%d %x", len(want1), want1, len(got1), got1)
	}
}

// TestUnpack_Terminator verifies that the 65535 terminator stops the loop and
// that no extra files are written.
func TestUnpack_Terminator(t *testing.T) {
	// A sounds.dat with only the terminator — zero sounds.
	soundsDat := buildSoundsDat(nil)

	cacheDir := t.TempDir()
	srcDir := t.TempDir()
	buildTestCache(t, cacheDir, soundsDat)

	if err := Unpack(Options{CacheDir: cacheDir, SrcDir: srcDir}); err != nil {
		t.Fatalf("Unpack: %v", err)
	}

	entries, err := os.ReadDir(filepath.Join(srcDir, "synth"))
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("ReadDir synth: %v", err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".synth") {
			t.Errorf("unexpected .synth file written: %s", e.Name())
		}
	}

	// synth.order should be written with zero entries → just a trailing newline.
	orderData, err := os.ReadFile(filepath.Join(srcDir, "pack", "synth.order"))
	if err != nil {
		t.Fatalf("read synth.order: %v", err)
	}
	if string(orderData) != "\n" {
		t.Errorf("synth.order: want \"\\n\", got %q", string(orderData))
	}
}

// TestUnpack_FallbackNaming verifies that IDs without a pre-registered name
// fall back to "sound_<id>", and that an existing name in synth.pack is used.
func TestUnpack_FallbackNaming(t *testing.T) {
	tone := toneBytes(false, false, false, nil, 0, 0, 50, 0)
	var tones [10][]byte
	tones[0] = tone

	soundsDat := buildSoundsDat([]soundSpec{
		{id: 5, tones: tones},
		{id: 7, tones: tones},
	})

	cacheDir := t.TempDir()
	srcDir := t.TempDir()

	// Seed synth.pack naming id=5 "mysynth" — under 274 suspendAutoReload the
	// SynthPack singleton is constructed empty (TS PackFile.ts:276 @dee467c8),
	// so this MUST be ignored and id=5 falls back to sound_5.
	packDir := filepath.Join(srcDir, "pack")
	if err := os.MkdirAll(packDir, 0o755); err != nil {
		t.Fatalf("mkdir pack: %v", err)
	}
	if err := os.WriteFile(filepath.Join(packDir, "synth.pack"), []byte("5=mysynth\n"), 0o644); err != nil {
		t.Fatalf("write synth.pack: %v", err)
	}

	buildTestCache(t, cacheDir, soundsDat)

	if err := Unpack(Options{CacheDir: cacheDir, SrcDir: srcDir}); err != nil {
		t.Fatalf("Unpack: %v", err)
	}

	// synth.pack's "mysynth" must be ignored under suspendAutoReload.
	if _, err := os.Stat(filepath.Join(srcDir, "synth", "mysynth.synth")); err == nil {
		t.Errorf("synth/mysynth.synth exists: 274 suspendAutoReload should ignore synth.pack and use sound_5")
	}
	// id=5 falls back to sound_5.synth.
	if _, err := os.Stat(filepath.Join(srcDir, "synth", "sound_5.synth")); err != nil {
		t.Errorf("synth/sound_5.synth not found: %v", err)
	}
	// id=7 falls back to sound_7.synth.
	if _, err := os.Stat(filepath.Join(srcDir, "synth", "sound_7.synth")); err != nil {
		t.Errorf("synth/sound_7.synth not found: %v", err)
	}
}

// TestUnpack_SynthOrderFormat verifies the synth.order file format:
// IDs joined by newlines with a trailing newline, in the order they appear in
// sounds.dat.
func TestUnpack_SynthOrderFormat(t *testing.T) {
	tone := toneBytes(false, false, false, nil, 0, 0, 10, 0)
	var tones [10][]byte
	tones[0] = tone

	soundsDat := buildSoundsDat([]soundSpec{
		{id: 3, tones: tones},
		{id: 7, tones: tones},
		{id: 1, tones: tones},
	})

	cacheDir := t.TempDir()
	srcDir := t.TempDir()
	buildTestCache(t, cacheDir, soundsDat)

	if err := Unpack(Options{CacheDir: cacheDir, SrcDir: srcDir}); err != nil {
		t.Fatalf("Unpack: %v", err)
	}

	orderData, err := os.ReadFile(filepath.Join(srcDir, "pack", "synth.order"))
	if err != nil {
		t.Fatalf("read synth.order: %v", err)
	}

	want := "3\n7\n1\n"
	if string(orderData) != want {
		t.Errorf("synth.order: want %q, got %q", want, string(orderData))
	}
}

// TestUnpack_OptionalEnvelopePairs verifies that the three optional envelope
// pairs (freqMod, ampMod, relAtk) are correctly gated.  Two tones are built
// in the SAME Wave — one with all pairs enabled, one with none.  The extracted
// .synth bytes must match the manually computed extent.
func TestUnpack_OptionalEnvelopePairs(t *testing.T) {
	// Tone 0: all three optional pairs enabled.
	toneAllOpts := toneBytes(true, true, true, nil, 5, 10, 100, 20)
	// Tone 1: no optional pairs.
	toneNoPairs := toneBytes(false, false, false, nil, 0, 0, 50, 0)

	var tones [10][]byte
	tones[0] = toneAllOpts
	tones[1] = toneNoPairs

	soundsDat := buildSoundsDat([]soundSpec{
		{id: 42, tones: tones, loopBegin: 10, loopEnd: 20},
	})

	// Verify the parser produces the right extent.
	buf := packet.NewPacket(soundsDat)
	id := buf.G2()
	if id != 42 {
		t.Fatalf("expected id=42, got %d", id)
	}
	start := buf.Pos
	if err := parseWave(buf); err != nil {
		t.Fatalf("parseWave: %v", err)
	}
	end := buf.Pos
	wantBytes := soundsDat[start:end]

	cacheDir := t.TempDir()
	srcDir := t.TempDir()
	buildTestCache(t, cacheDir, soundsDat)

	if err := Unpack(Options{CacheDir: cacheDir, SrcDir: srcDir}); err != nil {
		t.Fatalf("Unpack: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(srcDir, "synth", "sound_42.synth"))
	if err != nil {
		t.Fatalf("read sound_42.synth: %v", err)
	}
	if !bytes.Equal(got, wantBytes) {
		t.Errorf("sound_42.synth bytes differ\n  want len=%d\n   got len=%d", len(wantBytes), len(got))
	}
}

// TestUnpack_SignedHarmonicSemitone verifies that a negative harmonicSemitone
// (signed smart) is correctly encoded and parsed so the extent boundary is exact.
func TestUnpack_SignedHarmonicSemitone(t *testing.T) {
	// One harmonic with negative semitone (-10).
	harmonics := [][3]int{{5, -10, 3}}
	tone := toneBytes(false, false, false, harmonics, 0, 0, 100, 0)

	var tones [10][]byte
	tones[0] = tone

	soundsDat := buildSoundsDat([]soundSpec{
		{id: 99, tones: tones},
	})

	// Parse extent.
	buf := packet.NewPacket(soundsDat)
	buf.G2() // id
	start := buf.Pos
	if err := parseWave(buf); err != nil {
		t.Fatalf("parseWave: %v", err)
	}
	end := buf.Pos
	wantBytes := soundsDat[start:end]

	cacheDir := t.TempDir()
	srcDir := t.TempDir()
	buildTestCache(t, cacheDir, soundsDat)

	if err := Unpack(Options{CacheDir: cacheDir, SrcDir: srcDir}); err != nil {
		t.Fatalf("Unpack: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(srcDir, "synth", "sound_99.synth"))
	if err != nil {
		t.Fatalf("read sound_99.synth: %v", err)
	}
	if !bytes.Equal(got, wantBytes) {
		t.Errorf("sound_99.synth bytes differ\n  want len=%d\n   got len=%d", len(wantBytes), len(got))
	}
}

// TestUnpack_MultipleHarmonics verifies the harmonic loop: multiple entries
// terminated by volume=0.  The signed semitone path includes negative values.
func TestUnpack_MultipleHarmonics(t *testing.T) {
	harmonics := [][3]int{
		{1, 0, 0},   // volume=1, semitone=0, delay=0
		{5, -3, 10}, // negative semitone
		{10, 7, 2},
	}
	tone := toneBytes(false, false, false, harmonics, 2, 3, 60, 5)

	var tones [10][]byte
	tones[0] = tone

	soundsDat := buildSoundsDat([]soundSpec{
		{id: 77, tones: tones},
	})

	buf := packet.NewPacket(soundsDat)
	buf.G2()
	start := buf.Pos
	if err := parseWave(buf); err != nil {
		t.Fatalf("parseWave: %v", err)
	}
	end := buf.Pos
	wantBytes := soundsDat[start:end]

	cacheDir := t.TempDir()
	srcDir := t.TempDir()
	buildTestCache(t, cacheDir, soundsDat)

	if err := Unpack(Options{CacheDir: cacheDir, SrcDir: srcDir}); err != nil {
		t.Fatalf("Unpack: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(srcDir, "synth", "sound_77.synth"))
	if err != nil {
		t.Fatalf("read sound_77.synth: %v", err)
	}
	if !bytes.Equal(got, wantBytes) {
		t.Errorf("sound_77.synth bytes differ\n  want len=%d\n   got len=%d", len(wantBytes), len(got))
	}
}

// TestUnpack_EnvelopeWithShape verifies that shape segments in an Envelope
// (length > 0) are correctly consumed by the extent parse.
func TestUnpack_EnvelopeWithShape(t *testing.T) {
	buf := &packet.Packet{}
	// Manually build a tone whose frequencyBase has 3 shape segments.
	shape := [][2]uint16{{100, 200}, {300, 400}, {500, 600}}
	pEnvelope(buf, 1, -1000, 2000, shape) // frequencyBase with 3 segs
	pEnvelope(buf, 2, 0, 100, nil)        // amplitudeBase no segs
	buf.P1(0)                             // freqMod absent
	buf.P1(0)                             // ampMod absent
	buf.P1(0)                             // relAtk absent
	pSmartS(buf, 0)                       // no harmonics (terminator)
	pSmartS(buf, 0)                       // reverbDelay
	pSmartS(buf, 0)                       // reverbVolume
	buf.P2(50)                            // length
	buf.P2(0)                             // start
	buf.P1(0)                             // rev-274 Filter count=0 (no filter)

	var tones [10][]byte
	tones[0] = buf.Data

	soundsDat := buildSoundsDat([]soundSpec{
		{id: 11, tones: tones},
	})

	wavBuf := packet.NewPacket(soundsDat)
	wavBuf.G2()
	start := wavBuf.Pos
	if err := parseWave(wavBuf); err != nil {
		t.Fatalf("parseWave: %v", err)
	}
	wantBytes := soundsDat[start:wavBuf.Pos]

	cacheDir := t.TempDir()
	srcDir := t.TempDir()
	buildTestCache(t, cacheDir, soundsDat)

	if err := Unpack(Options{CacheDir: cacheDir, SrcDir: srcDir}); err != nil {
		t.Fatalf("Unpack: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(srcDir, "synth", "sound_11.synth"))
	if err != nil {
		t.Fatalf("read sound_11.synth: %v", err)
	}
	if !bytes.Equal(got, wantBytes) {
		t.Errorf("sound_11.synth bytes differ\n  want len=%d %x\n   got len=%d %x", len(wantBytes), wantBytes, len(got), got)
	}
}

// TestUnpack_NilOut verifies that passing Out=nil does not panic.
func TestUnpack_NilOut(t *testing.T) {
	soundsDat := buildSoundsDat(nil)
	cacheDir := t.TempDir()
	srcDir := t.TempDir()
	buildTestCache(t, cacheDir, soundsDat)

	if err := Unpack(Options{CacheDir: cacheDir, SrcDir: srcDir, Out: nil}); err != nil {
		t.Fatalf("Unpack with nil Out: %v", err)
	}
}

// TestUnpack_SynthPackSaved verifies that synth.pack is written with the
// registered id→name entries after Unpack completes.
func TestUnpack_SynthPackSaved(t *testing.T) {
	tone := toneBytes(false, false, false, nil, 0, 0, 10, 0)
	var tones [10][]byte
	tones[0] = tone

	soundsDat := buildSoundsDat([]soundSpec{
		{id: 2, tones: tones},
		{id: 9, tones: tones},
	})

	cacheDir := t.TempDir()
	srcDir := t.TempDir()
	buildTestCache(t, cacheDir, soundsDat)

	if err := Unpack(Options{CacheDir: cacheDir, SrcDir: srcDir}); err != nil {
		t.Fatalf("Unpack: %v", err)
	}

	packData, err := os.ReadFile(filepath.Join(srcDir, "pack", "synth.pack"))
	if err != nil {
		t.Fatalf("read synth.pack: %v", err)
	}

	lines := strings.Split(strings.TrimRight(string(packData), "\n"), "\n")
	wantLines := map[string]bool{
		"2=sound_2": true,
		"9=sound_9": true,
	}
	for _, line := range lines {
		if line == "" {
			continue
		}
		if !wantLines[line] {
			t.Errorf("unexpected line in synth.pack: %q", line)
		}
		delete(wantLines, line)
	}
	for missing := range wantLines {
		t.Errorf("missing line in synth.pack: %q", missing)
	}
}

// TestUnpack_LargeSmartValues verifies the two-byte GSmartS path (value >= 128).
func TestUnpack_LargeSmartValues(t *testing.T) {
	// reverbDelay=200 (≥128 → 2-byte smart), reverbVolume=150 (2-byte smart).
	tone := toneBytes(false, false, false, nil, 200, 150, 1000, 500)

	var tones [10][]byte
	tones[0] = tone

	soundsDat := buildSoundsDat([]soundSpec{
		{id: 55, tones: tones},
	})

	buf := packet.NewPacket(soundsDat)
	buf.G2()
	start := buf.Pos
	if err := parseWave(buf); err != nil {
		t.Fatalf("parseWave: %v", err)
	}
	wantBytes := soundsDat[start:buf.Pos]

	cacheDir := t.TempDir()
	srcDir := t.TempDir()
	buildTestCache(t, cacheDir, soundsDat)

	if err := Unpack(Options{CacheDir: cacheDir, SrcDir: srcDir}); err != nil {
		t.Fatalf("Unpack: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(srcDir, "synth", "sound_55.synth"))
	if err != nil {
		t.Fatalf("read sound_55.synth: %v", err)
	}
	if !bytes.Equal(got, wantBytes) {
		t.Errorf("sound_55.synth bytes differ\n  want len=%d\n   got len=%d", len(wantBytes), len(got))
	}
}

// TestUnpack_ToneFilter_NonTrivial verifies the rev-274 Filter parse appended to
// Tone.unpack.  A tone is built with a non-empty filter (pairs0=2, pairs1=1,
// migration with one bit set, distinct unities → trailing shape).  The extracted
// .synth bytes must equal the full tone including the filter; the extent is
// computed independently of parseTone via the encoder so this is a real RED→GREEN
// (a parser that stops before the filter would under-read).
func TestUnpack_ToneFilter_NonTrivial(t *testing.T) {
	// Build the custom filter payload independently.
	// count = (2<<4)|1 = 0x21.
	// unities differ (10 vs 20) AND migration != 0 → trailing shape emitted.
	fbuf := &packet.Packet{}
	pFilter(fbuf, 2, 1, 0b0000_0001, 10, 20, true)
	filterBytes := fbuf.Data

	// Independently expected filter length:
	//   1 (count) + 2 + 2 (unities) + 1 (migration)
	//   + phase1: (pairs0+pairs1) * 4 = 3 * 4 = 12
	//   + phase2: 1 set migration bit * 4 = 4
	//   + shape: 1 (length=0)
	wantFilterLen := 1 + 2 + 2 + 1 + 12 + 4 + 1
	if len(filterBytes) != wantFilterLen {
		t.Fatalf("encoder produced filter len=%d, want %d", len(filterBytes), wantFilterLen)
	}

	tone := toneBytesWithFilter(false, false, false, nil, 0, 0, 100, 0, filterBytes)

	var tones [10][]byte
	tones[0] = tone

	soundsDat := buildSoundsDat([]soundSpec{
		{id: 88, tones: tones, loopBegin: 1, loopEnd: 2},
	})

	// Compute the expected extent WITHOUT relying on parseTone: the wave payload
	// is tone (with filter) + loopBegin(2) + loopEnd(2). The wave instance has
	// tone[0] present (probe = tone's first byte, non-zero) followed by 9 absent
	// tone probes (each a single 0 byte), then loopBegin/loopEnd.
	// soundsDat layout: g2 id (2 bytes) | wave payload | g2 65535 terminator.
	// wave payload = tone (probe byte is tone[0]) + 9*0x00 + g2 loopBegin + g2 loopEnd.
	wantWaveLen := len(tone) + 9 /*absent tone probes*/ + 2 /*loopBegin*/ + 2 /*loopEnd*/
	// The .synth extent excludes the leading g2 id.
	wantBytes := soundsDat[2 : 2+wantWaveLen]

	cacheDir := t.TempDir()
	srcDir := t.TempDir()
	buildTestCache(t, cacheDir, soundsDat)

	if err := Unpack(Options{CacheDir: cacheDir, SrcDir: srcDir}); err != nil {
		t.Fatalf("Unpack: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(srcDir, "synth", "sound_88.synth"))
	if err != nil {
		t.Fatalf("read sound_88.synth: %v", err)
	}
	if !bytes.Equal(got, wantBytes) {
		t.Errorf("sound_88.synth bytes differ\n  want len=%d %x\n   got len=%d %x", len(wantBytes), wantBytes, len(got), got)
	}
}

// TestUnpack_ToneFilter_CountZero verifies the count=0 filter path: a single
// trailing count byte is consumed and nothing more.  This is the common case
// (most tones have no filter).
func TestUnpack_ToneFilter_CountZero(t *testing.T) {
	// pTone already appends count=0; build two tones in one wave and assert the
	// extent is exact (the second tone's bytes must not bleed into the first).
	tone0 := toneBytes(false, false, false, nil, 0, 0, 100, 0)
	tone1 := toneBytes(false, false, false, nil, 0, 0, 200, 5)

	var tones [10][]byte
	tones[0] = tone0
	tones[1] = tone1

	soundsDat := buildSoundsDat([]soundSpec{
		{id: 4, tones: tones, loopBegin: 7, loopEnd: 8},
	})

	// Independent extent: tone0 + tone1 + 8 absent probes + loopBegin(2) + loopEnd(2).
	wantWaveLen := len(tone0) + len(tone1) + 8 + 2 + 2
	wantBytes := soundsDat[2 : 2+wantWaveLen]

	cacheDir := t.TempDir()
	srcDir := t.TempDir()
	buildTestCache(t, cacheDir, soundsDat)

	if err := Unpack(Options{CacheDir: cacheDir, SrcDir: srcDir}); err != nil {
		t.Fatalf("Unpack: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(srcDir, "synth", "sound_4.synth"))
	if err != nil {
		t.Fatalf("read sound_4.synth: %v", err)
	}
	if !bytes.Equal(got, wantBytes) {
		t.Errorf("sound_4.synth bytes differ\n  want len=%d\n   got len=%d", len(wantBytes), len(got))
	}
}

// TestUnpack_ToneFilter_NoShape verifies the no-shape filter path: count != 0 but
// migration == 0 AND unities equal → no trailing Envelope.unpackShape.
func TestUnpack_ToneFilter_NoShape(t *testing.T) {
	fbuf := &packet.Packet{}
	// pairs0=1, pairs1=0, migration=0, unities equal (5,5) → withShape=false.
	pFilter(fbuf, 1, 0, 0, 5, 5, false)
	filterBytes := fbuf.Data

	// 1 (count) + 4 (unities) + 1 (migration) + phase1: 1*4 + phase2: 0 + no shape.
	wantFilterLen := 1 + 4 + 1 + 4
	if len(filterBytes) != wantFilterLen {
		t.Fatalf("encoder produced filter len=%d, want %d", len(filterBytes), wantFilterLen)
	}

	tone := toneBytesWithFilter(false, false, false, nil, 0, 0, 50, 0, filterBytes)
	var tones [10][]byte
	tones[0] = tone

	soundsDat := buildSoundsDat([]soundSpec{
		{id: 12, tones: tones},
	})

	wantWaveLen := len(tone) + 9 + 2 + 2
	wantBytes := soundsDat[2 : 2+wantWaveLen]

	cacheDir := t.TempDir()
	srcDir := t.TempDir()
	buildTestCache(t, cacheDir, soundsDat)

	if err := Unpack(Options{CacheDir: cacheDir, SrcDir: srcDir}); err != nil {
		t.Fatalf("Unpack: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(srcDir, "synth", "sound_12.synth"))
	if err != nil {
		t.Fatalf("read sound_12.synth: %v", err)
	}
	if !bytes.Equal(got, wantBytes) {
		t.Errorf("sound_12.synth bytes differ\n  want len=%d\n   got len=%d", len(wantBytes), len(got))
	}
}
