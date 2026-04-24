package world

import "github.com/zsrv/goscape/pkg/io/packet"

// encodeMidiSong writes a MidiSong payload per TS MidiSongEncoder.ts:
//
//	buf.pjstr(message.name);
//	buf.p4(message.crc);
//	buf.p4(message.length);
//
// Byte-aligned. Caller wraps in:
//
//	p.writeOut(gameserver.OpMidiSong, buf.Bytes())
//
// The string terminator is 0x0A (LF) per TS Packet.pjstr at
// io/Packet.ts:330-337 (universal goscape PJStrLF precedent).
func encodeMidiSong(buf *packet.Packet, name string, crc uint32, length uint32) {
	buf.PJStrLF(name)
	buf.P4(crc)
	buf.P4(length)
}

// encodeMidiJingle writes a MidiJingle payload per TS MidiJingleEncoder.ts:
//
//	buf.p2(message.delay);
//	buf.pdata(message.data, 0, message.data.length);
//
// Byte-aligned. Caller wraps in:
//
//	p.writeOut(gameserver.OpMidiJingle, buf.Bytes())
//
// goscape's PData(src) takes no offset/length and writes the whole
// slice; TS's pdata(src, 0, src.length) reduces to the same output.
func encodeMidiJingle(buf *packet.Packet, delay uint16, data []byte) {
	buf.P2(delay)
	buf.PData(data)
}
