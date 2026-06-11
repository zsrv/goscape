package world

import (
	"github.com/zsrv/goscape/pkg/io/packet"
)

// A10 @2e3bcf43: the 244-era loadMidiPack/midiIDByName name→id registry
// that lived here is RETIRED — TS deleted the runtime PackFile lookup;
// names resolve to ids at compile time (tools/pack/Compiler.ts:199 loads
// midi.pack as a `midi`-type symbol table) and Player.playSong/playJingle
// are id-based (Player.ts:1985-1991). Track lengths come from the
// pkg/midi cache (Server.midi).

// encodeMidiSong writes a MidiSong payload per TS MidiSongEncoder.ts
// @2e3bcf43 (unchanged from 244):
//
//	buf.p2(message.id);
//
// Fixed 2-byte payload. Caller wraps in:
//
//	p.writeOut(gameserver.OpMidiSong, buf.Bytes())
func encodeMidiSong(buf *packet.Packet, id int) {
	buf.P2(uint16(id))
}

// encodeMidiJingle writes a MidiJingle payload per TS MidiJingleEncoder.ts
// @2e3bcf43 (unchanged from 244):
//
//	buf.p2(message.id);
//	buf.p2(message.delay);
//
// Fixed 4-byte payload; delay is the track length in MILLISECONDS
// (Midi.getLength — see (*Player).PlayJingle). Caller wraps in:
//
//	p.writeOut(gameserver.OpMidiJingle, buf.Bytes())
func encodeMidiJingle(buf *packet.Packet, id int, delay int) {
	buf.P2(uint16(id))
	buf.P2(uint16(delay))
}
