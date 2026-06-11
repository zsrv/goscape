package midi

import (
	"encoding/binary"
	"testing"
)

// buildMidi assembles a single-track format-0 SMF blob with the given
// division and raw track-event bytes.
func buildMidi(division int, events []byte) []byte {
	var out []byte
	out = append(out, 'M', 'T', 'h', 'd')
	out = binary.BigEndian.AppendUint32(out, 6)
	out = binary.BigEndian.AppendUint16(out, 0) // format
	out = binary.BigEndian.AppendUint16(out, 1) // tracks
	out = binary.BigEndian.AppendUint16(out, uint16(division))
	out = append(out, 'M', 'T', 'r', 'k')
	out = binary.BigEndian.AppendUint32(out, uint32(len(events)))
	out = append(out, events...)
	return out
}

var endOfTrack = []byte{0x00, 0xff, 0x2f, 0x00}

// TestParseLengthDefaultTempo: no Set Tempo event → default 500000 µs per
// quarter note (TS Midi.ts:203 `let currentTempo = 500000`). PPQ=96,
// maxTick=96 → exactly one quarter note → 500ms.
func TestParseLengthDefaultTempo(t *testing.T) {
	events := []byte{
		0x00, 0x90, 0x3c, 0x40, // delta 0, NoteOn
		0x60, 0x80, 0x3c, 0x40, // delta 96, NoteOff
	}
	events = append(events, endOfTrack...)
	ms, ok := ParseLength(buildMidi(96, events))
	if !ok {
		t.Fatal("ParseLength: ok=false, want true")
	}
	if ms != 500 {
		t.Errorf("ms: got %d, want 500 (96 ticks @ PPQ 96, default 500000µs/qn)", ms)
	}
}

// TestParseLengthTempoEvent: a Set Tempo (FF 51 03) at tick 0 governs the
// whole track (TS Midi.ts:142-145 + 207-220). 250000 µs/qn halves the
// default → 250ms for one quarter note.
func TestParseLengthTempoEvent(t *testing.T) {
	events := []byte{
		0x00, 0xff, 0x51, 0x03, 0x03, 0xd0, 0x90, // delta 0, SetTempo 250000
		0x00, 0x90, 0x3c, 0x40, // delta 0, NoteOn (running status seed)
		0x60, 0x80, 0x3c, 0x40, // delta 96, NoteOff
	}
	events = append(events, endOfTrack...)
	ms, ok := ParseLength(buildMidi(96, events))
	if !ok {
		t.Fatal("ParseLength: ok=false, want true")
	}
	if ms != 250 {
		t.Errorf("ms: got %d, want 250 (tempo 250000µs/qn)", ms)
	}
}

// TestParseLengthSMPTEDivision pins the SMPTE branch (TS Midi.ts:191-198):
// division bit 15 set → frames/second = two's complement of the high
// byte, ticks/frame = low byte. fps 25 (high byte 0x100-25 = 0xE7) ×
// 40 ticks/frame = 1000 ticks/second; maxTick 500 → 500ms. Tempo events
// are IGNORED on this path (wall-clock division).
func TestParseLengthSMPTEDivision(t *testing.T) {
	events := []byte{
		0x00, 0x90, 0x3c, 0x40, // delta 0, NoteOn
		0x83, 0x74, 0x80, 0x3c, 0x40, // delta 500 (varlen 83 74), NoteOff
	}
	events = append(events, endOfTrack...)
	division := 0xE7<<8 | 40 // SMPTE 25fps, 40 ticks/frame
	ms, ok := ParseLength(buildMidi(division, events))
	if !ok {
		t.Fatal("ParseLength: ok=false, want true")
	}
	if ms != 500 {
		t.Errorf("ms: got %d, want 500 (500 ticks @ 1000 ticks/sec SMPTE)", ms)
	}
}

// TestParseLengthRunningStatus: a data byte (<0x80) reuses the previous
// channel status (TS Midi.ts:110-122).
func TestParseLengthRunningStatus(t *testing.T) {
	events := []byte{
		0x00, 0x90, 0x3c, 0x40, // delta 0, NoteOn — sets running status
		0x60, 0x3c, 0x00, // delta 96, running-status NoteOn vel 0
	}
	events = append(events, endOfTrack...)
	ms, ok := ParseLength(buildMidi(96, events))
	if !ok {
		t.Fatal("ParseLength: ok=false, want true")
	}
	if ms != 500 {
		t.Errorf("ms: got %d, want 500", ms)
	}
}

// TestParseLengthRiffWrapped: a RIFF/RMID container is unwrapped to its
// 'data' chunk before parsing (TS unwrapRiffMidi, Midi.ts:226-258 —
// "attack1.mid has a RIFF header").
func TestParseLengthRiffWrapped(t *testing.T) {
	events := []byte{0x60, 0x90, 0x3c, 0x40} // delta 96, NoteOn
	events = append(events, endOfTrack...)
	smf := buildMidi(96, events)

	var riff []byte
	riff = append(riff, 'R', 'I', 'F', 'F')
	riff = binary.LittleEndian.AppendUint32(riff, uint32(4+8+len(smf)))
	riff = append(riff, 'R', 'M', 'I', 'D')
	riff = append(riff, 'd', 'a', 't', 'a')
	riff = binary.LittleEndian.AppendUint32(riff, uint32(len(smf)))
	riff = append(riff, smf...)

	ms, ok := ParseLength(riff)
	if !ok {
		t.Fatal("ParseLength(RIFF): ok=false, want true")
	}
	if ms != 500 {
		t.Errorf("ms: got %d, want 500", ms)
	}
}

// TestParseLengthMalformedRejects mirrors the TS null returns.
func TestParseLengthMalformedRejects(t *testing.T) {
	cases := map[string][]byte{
		"empty-after-min-len": make([]byte, 20),
		"bad-chunk-id":        append([]byte("XXXX"), make([]byte, 16)...),
	}
	for name, blob := range cases {
		if _, ok := ParseLength(blob); ok {
			t.Errorf("%s: ok=true, want false", name)
		}
	}
	// Sub-minimum data: TS returns null at `data.length < 14`.
	if _, ok := ParseLength([]byte{1, 2, 3}); ok {
		t.Error("short blob: ok=true, want false")
	}
}

// fakeStore drives Load without a real FileStream.
type fakeStore struct {
	blobs map[int][]byte
	count int
}

func (f *fakeStore) Count(archive int) int {
	if archive != 3 {
		return 0
	}
	return f.count
}

func (f *fakeStore) Read(archive, file int, decompress bool) []byte {
	if archive != 3 {
		return nil
	}
	return f.blobs[file]
}

// TestLoadAndLengths pins the Load → GetLength/GetTickLength chain:
//   - id 0: valid 500ms track → GetLength 500, GetTickLength ceil(500/600)+1 = 2.
//   - id 1: missing blob → 0ms (warned), tick length 1.
//   - id 2: malformed blob → 0ms (warned), tick length 1.
//   - OOB / negative ids → 0ms / 1 tick (TS `this.lengths[id] ?? 0`).
func TestLoadAndLengths(t *testing.T) {
	events := []byte{0x60, 0x90, 0x3c, 0x40}
	events = append(events, endOfTrack...)
	good := buildMidi(96, events)

	var warns int
	c := Load(&fakeStore{
		count: 3,
		blobs: map[int][]byte{0: good, 2: {0xde, 0xad}},
	}, func(string, ...any) { warns++ })

	if got := c.GetLength(0); got != 500 {
		t.Errorf("GetLength(0): got %d, want 500 (ms)", got)
	}
	if got := c.GetTickLength(0); got != 2 {
		t.Errorf("GetTickLength(0): got %d, want 2 (ceil(500/600)+1)", got)
	}
	if got := c.GetLength(1); got != 0 {
		t.Errorf("GetLength(1): got %d, want 0 (missing blob)", got)
	}
	if got := c.GetTickLength(1); got != 1 {
		t.Errorf("GetTickLength(1): got %d, want 1 (0ms → +1)", got)
	}
	if got := c.GetLength(2); got != 0 {
		t.Errorf("GetLength(2): got %d, want 0 (malformed blob)", got)
	}
	if got := c.GetLength(99); got != 0 {
		t.Errorf("GetLength(99): got %d, want 0 (OOB)", got)
	}
	if got := c.GetLength(-1); got != 0 {
		t.Errorf("GetLength(-1): got %d, want 0", got)
	}
	if warns != 2 {
		t.Errorf("warnings: got %d, want 2 (missing id=1 + failed id=2)", warns)
	}

	// Empty cache posture.
	empty := Load(&fakeStore{count: 0}, nil)
	if got := empty.GetTickLength(0); got != 1 {
		t.Errorf("empty cache GetTickLength: got %d, want 1", got)
	}
	// Nil store / nil cache postures.
	if got := Load(nil, nil).GetLength(0); got != 0 {
		t.Errorf("nil store GetLength: got %d, want 0", got)
	}
	var nilCache *Cache
	if got := nilCache.GetTickLength(5); got != 1 {
		t.Errorf("nil cache GetTickLength: got %d, want 1", got)
	}
}
