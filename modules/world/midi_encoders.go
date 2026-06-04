package world

import "github.com/zsrv/goscape/pkg/io/packet"

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

// midiIDByName returns the pack id for the given MIDI name, or -1 if not found.
//
// PORTING-EXCEPTION (rev244-b2-midi-window, silent until B3 MidiPack): the 244
// MIDI_SONG/MIDI_JINGLE wire carries pack ids; the MidiPack name→id registry
// lands with B3's cache rework. Returning -1 makes PlaySong/PlayJingle silent
// no-ops, mirroring TS's id!==-1 guard (Player.ts:1921-1929). See PORTING.md.
func midiIDByName(_ string) int {
	return -1
}
