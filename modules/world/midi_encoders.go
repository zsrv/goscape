package world

import (
	"os"
	"strconv"
	"strings"

	"github.com/zsrv/goscape/pkg/io/packet"
)

// loadMidiPack reads "id=name" lines from path and returns a name→id map.
// Mirrors TS PackFileBase.ts:50-71 (load): lines not matching "^\d+=" are
// skipped; absent file returns an empty map (degrade posture). Names are
// stored VERBATIM (TS register(id, parts[1]) — no normalization). NOTE the
// faithful consequence: the real Content midi.pack keys multi-word songs
// with SPACES ("0=scape main") while playSong normalizes spaces→underscores
// (Player.ts:1922), so multi-word songs miss the registry and silently
// no-op — in TS 244 too (upstream's own `// todo: make compiler do this at
// pack time` at Player.ts:1918 acknowledges it). Reproduced as-is; revisit
// only if upstream fixes it. Micro-divergence: TS line.split('=') truncates
// a name at a second '=' — Go keeps the full remainder (no pack name
// contains '='; recorded, not coded around).
//
// A tiny local parser is used instead of pkg/pack's to avoid coupling
// modules/world to the pack-pipeline package.
func loadMidiPack(path string) map[string]int {
	data, err := os.ReadFile(path)
	if err != nil {
		return map[string]int{}
	}
	out := make(map[string]int)
	for _, line := range strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n") {
		eq := strings.IndexByte(line, '=')
		if eq < 1 {
			continue
		}
		id, err := strconv.Atoi(line[:eq])
		if err != nil {
			continue
		}
		name := line[eq+1:]
		if name == "" {
			continue
		}
		out[name] = id
	}
	return out
}

// encodeMidiSong writes a MidiSong payload per TS MidiSongEncoder.ts (244):
//
//	buf.p2(message.id);
//
// Fixed 2-byte payload. Caller wraps in:
//
//	p.writeOut(gameserver.OpMidiSong, buf.Bytes())
func encodeMidiSong(buf *packet.Packet, id int) {
	buf.P2(uint16(id))
}

// encodeMidiJingle writes a MidiJingle payload per TS MidiJingleEncoder.ts (244):
//
//	buf.p2(message.id);
//	buf.p2(message.delay);
//
// Fixed 4-byte payload. Caller wraps in:
//
//	p.writeOut(gameserver.OpMidiJingle, buf.Bytes())
func encodeMidiJingle(buf *packet.Packet, id int, delay int) {
	buf.P2(uint16(id))
	buf.P2(uint16(delay))
}

// midiIDByName returns the pack id for the given MIDI name from the server's
// registry, or -1 if not found. Mirrors PackFileBase.ts:129-131 (getByName)
// called by Player.ts:1919-1933 (playSong / playJingle).
//
// Registry is populated at world start from <ContentPath>/pack/midi.pack;
// absent file or nil registry degrades to -1 for every name (TS unknown-name
// posture). Callers guard on id != -1 before writing the wire packet.
func (s *Server) midiIDByName(name string) int {
	if id, ok := s.midiPack[name]; ok {
		return id
	}
	return -1
}
