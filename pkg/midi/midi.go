// Package midi ports TS src/cache/midi/Midi.ts @2e3bcf43 (new at the
// rev-254 pin-advance): a boot-time scan of the client cache's MIDI
// store (archive index 3) that parses each track's MThd/MTrk chunks to
// derive its playback length in milliseconds. The lengths feed
// Player.playJingle's delay field and the MIDI_LENGTH script op.
package midi

import (
	"math"
	"slices"
)

// Store is the slice of the client-cache FileStream surface Load needs —
// TS Midi.load reads OnDemand.cache (OnDemand.ts: new FileStream('data/pack')):
//
//	const count = OnDemand.cache.count(3);
//	const data = OnDemand.cache.read(3, i, true);
//
// *filestream.FileStream satisfies it.
type Store interface {
	Count(archive int) int
	Read(archive, file int, decompress bool) []byte
}

// midiArchive is the MIDI store's archive index in the client cache
// (TS Midi.ts:264 `OnDemand.cache.count(3)`).
const midiArchive = 3

// Cache holds the parsed per-track lengths. Mirrors the TS
// `static lengths: number[]` on the Midi class.
type Cache struct {
	lengths []int
}

// NewCache builds a Cache from pre-parsed lengths (milliseconds, indexed
// by track id). For tests / injection; production paths use Load.
func NewCache(lengths []int) *Cache {
	return &Cache{lengths: lengths}
}

// Load scans the MIDI archive and parses every track's length. Mirrors
// TS Midi.load (Midi.ts:263-291 @2e3bcf43):
//
//	static load(): void {
//	    const count = OnDemand.cache.count(3);
//	    if (!count) { printWarning('No MIDI data in cache.'); return; }
//	    this.lengths = new Array(count).fill(0);
//	    for (let i = 0; i < count; i++) {
//	        const data = OnDemand.cache.read(3, i, true);
//	        if (!data) { printWarning(`Missing midi id=${i}`); continue; }
//	        const length = parseMidiLength(data);
//	        if (!length) { printWarning(`Failed to parse midi id=${i}`); continue; }
//	        this.lengths[i] = length;
//	    }
//	}
//
// warnf receives the TS printWarning lines (nil → discard). A nil store
// degrades like an empty cache. Always returns a non-nil *Cache.
func Load(store Store, warnf func(format string, args ...any)) *Cache {
	if warnf == nil {
		warnf = func(string, ...any) {}
	}
	c := &Cache{}
	if store == nil {
		warnf("No MIDI data in cache.")
		return c
	}
	count := store.Count(midiArchive)
	if count == 0 {
		warnf("No MIDI data in cache.")
		return c
	}
	c.lengths = make([]int, count)
	for i := range count {
		data := store.Read(midiArchive, i, true)
		if data == nil {
			warnf("Missing midi id=%d", i)
			continue
		}
		length, ok := ParseLength(data)
		// TS `if (!length)` — both the parse-failure null AND a parsed
		// 0ms length are falsy, leaving the slot at 0 with a warning.
		if !ok || length == 0 {
			warnf("Failed to parse midi id=%d", i)
			continue
		}
		c.lengths[i] = length
	}
	return c
}

// GetLength returns the track's length in MILLISECONDS, or 0 for an
// unknown/out-of-range id. Mirrors TS Midi.getLength (Midi.ts:293-295):
//
//	static getLength(id: number): number {
//	    return this.lengths[id] ?? 0;
//	}
func (c *Cache) GetLength(id int) int {
	if c == nil || id < 0 || id >= len(c.lengths) {
		return 0
	}
	return c.lengths[id]
}

// GetTickLength returns the track's length in 600ms GAME TICKS, rounded
// up, plus one. Mirrors TS Midi.getTickLength (Midi.ts:297-299):
//
//	static getTickLength(id: number): number {
//	    return Math.ceil(this.getLength(id) / 600) + 1;
//	}
//
// Note the units: getLength is milliseconds; getTickLength is ticks
// (ceil(ms/600) + 1) — an unknown id yields 1, not 0.
func (c *Cache) GetTickLength(id int) int {
	return (c.GetLength(id)+599)/600 + 1
}

// tempoEvent mirrors the TS MidiTempoEvent {tick, tempo, order} triple.
type tempoEvent struct {
	tick  int
	tempo int
	order int
}

func readU16BE(data []byte, offset int) int {
	return int(data[offset])<<8 | int(data[offset+1])
}

func readU32BE(data []byte, offset int) int {
	return int(data[offset])<<24 | int(data[offset+1])<<16 | int(data[offset+2])<<8 | int(data[offset+3])
}

func readU32LE(data []byte, offset int) int {
	return int(data[offset]) | int(data[offset+1])<<8 | int(data[offset+2])<<16 | int(data[offset+3])<<24
}

func readChunkID(data []byte, offset int) string {
	return string(data[offset : offset+4])
}

// readVarLen ports TS readVarLen (Midi.ts:27-42): up to 4 bytes of MIDI
// variable-length quantity. Returns (value, nextOffset, ok).
func readVarLen(data []byte, offset, limit int) (int, int, bool) {
	value := 0
	for range 4 {
		if offset >= limit {
			return 0, 0, false
		}
		b := data[offset]
		offset++
		value = value<<7 | int(b&0x7f)
		if b&0x80 == 0 {
			return value, offset, true
		}
	}
	return 0, 0, false
}

// ParseLength derives a MIDI blob's playback length in milliseconds.
// Full port of TS parseMidiLength (Midi.ts:44-224 @2e3bcf43): walks every
// MTrk's delta-time stream collecting Set Tempo (FF 51) meta events and
// the maximum tick, then integrates tick→µs over the tempo map (default
// 500000 µs/quarter; PPQ from the MThd division) or, for SMPTE division,
// converts ticks/second directly. Returns ok=false where TS returns null
// (malformed header/track data).
func ParseLength(src []byte) (int, bool) {
	data, ok := unwrapRiffMidi(src)
	if !ok {
		return 0, false
	}
	if len(data) < 14 {
		return 0, false
	}

	offset := 0
	if readChunkID(data, offset) != "MThd" {
		return 0, false
	}

	headerLength := readU32BE(data, offset+4)
	offset += 8
	if headerLength < 6 || offset+headerLength > len(data) {
		return 0, false
	}

	format := readU16BE(data, offset)
	trackCount := readU16BE(data, offset+2)
	division := readU16BE(data, offset+4)
	offset += headerLength

	if format > 2 || trackCount <= 0 {
		return 0, false
	}

	maxTick := 0
	var tempos []tempoEvent
	tempoOrder := 0

	for range trackCount {
		if offset+8 > len(data) {
			return 0, false
		}
		if readChunkID(data, offset) != "MTrk" {
			return 0, false
		}

		trackLength := readU32BE(data, offset+4)
		offset += 8
		trackEnd := offset + trackLength
		if trackEnd > len(data) {
			return 0, false
		}

		tick := 0
		runningStatus := 0

		for offset < trackEnd {
			delta, next, ok := readVarLen(data, offset, trackEnd)
			if !ok {
				return 0, false
			}
			tick += delta
			offset = next

			if offset >= trackEnd {
				break
			}

			status := int(data[offset])
			if status < 0x80 {
				if runningStatus == 0 {
					return 0, false
				}
				status = runningStatus
			} else {
				offset++
				if status < 0xf0 {
					runningStatus = status
				}
			}

			switch {
			case status == 0xff:
				if offset >= trackEnd {
					return 0, false
				}
				metaType := int(data[offset])
				offset++
				metaLength, next, ok := readVarLen(data, offset, trackEnd)
				if !ok {
					return 0, false
				}
				offset = next
				if offset+metaLength > trackEnd {
					return 0, false
				}
				if metaType == 0x51 && metaLength == 3 {
					tempo := int(data[offset])<<16 | int(data[offset+1])<<8 | int(data[offset+2])
					tempos = append(tempos, tempoEvent{tick: tick, tempo: tempo, order: tempoOrder})
					tempoOrder++
				}
				offset += metaLength
				if metaType == 0x2f { // End of Track
					offset = trackEnd
				}
			case status == 0xf0 || status == 0xf7:
				sysexLength, next, ok := readVarLen(data, offset, trackEnd)
				if !ok {
					return 0, false
				}
				offset = next + sysexLength
				if offset > trackEnd {
					return 0, false
				}
			case status >= 0xf0:
				dataBytes := 0
				if status == 0xf1 || status == 0xf3 {
					dataBytes = 1
				} else if status == 0xf2 {
					dataBytes = 2
				}
				offset += dataBytes
				if offset > trackEnd {
					return 0, false
				}
			default:
				typ := status & 0xf0
				dataBytes := 2
				if typ == 0xc0 || typ == 0xd0 {
					dataBytes = 1
				}
				offset += dataBytes
				if offset > trackEnd {
					return 0, false
				}
			}
		}

		if tick > maxTick {
			maxTick = tick
		}
	}

	if division&0x8000 != 0 {
		// SMPTE division: frames/second is the two's-complement of the
		// high byte; ticks/frame is the low byte (TS Midi.ts:191-198).
		smpte := (division >> 8) & 0xff
		framesPerSecond := 0x100 - smpte
		ticksPerFrame := division & 0xff
		ticksPerSecond := framesPerSecond * ticksPerFrame
		if ticksPerSecond <= 0 {
			return 0, true
		}
		return int(math.Round(float64(maxTick) / float64(ticksPerSecond) * 1000)), true
	}

	ppq := division
	if ppq == 0 {
		ppq = 1
	}
	// Stable (tick, order) sort — TS Midi.ts:201
	// `tempos.sort((a, b) => (a.tick - b.tick) || (a.order - b.order))`.
	slices.SortFunc(tempos, func(a, b tempoEvent) int {
		if a.tick != b.tick {
			return a.tick - b.tick
		}
		return a.order - b.order
	})

	currentTempo := 500000
	lastTick := 0
	totalUs := 0.0

	for _, tempo := range tempos {
		if tempo.tick < lastTick {
			continue
		}
		deltaTicks := tempo.tick - lastTick
		totalUs += float64(deltaTicks) * float64(currentTempo) / float64(ppq)
		currentTempo = tempo.tempo
		lastTick = tempo.tick
	}

	if maxTick > lastTick {
		totalUs += float64(maxTick-lastTick) * float64(currentTempo) / float64(ppq)
	}

	return int(math.Round(totalUs / 1000)), true
}

// unwrapRiffMidi strips the RIFF/RMID container some tracks ship in
// ("attack1.mid has a RIFF header" — TS Midi.ts:226-258). Non-RIFF data
// passes through unchanged; a RIFF without an RMID form-type or a 'data'
// chunk fails.
func unwrapRiffMidi(data []byte) ([]byte, bool) {
	if len(data) < 12 {
		return data, true
	}
	if readChunkID(data, 0) != "RIFF" {
		return data, true
	}
	if readChunkID(data, 8) != "RMID" {
		return nil, false
	}

	offset := 12
	for offset+8 <= len(data) {
		id := readChunkID(data, offset)
		size := readU32LE(data, offset+4)
		offset += 8
		if size < 0 || offset+size > len(data) {
			return nil, false
		}
		if id == "data" {
			return data[offset : offset+size], true
		}
		offset += size + size%2
	}

	return nil, false
}
