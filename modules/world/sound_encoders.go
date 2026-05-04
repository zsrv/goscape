package world

import "github.com/zsrv/goscape/pkg/io/packet"

// encodeSynthSound writes a SynthSound payload per TS SynthSoundEncoder.ts:
//
//	buf.p2(message.synth);
//	buf.p1(message.loops);
//	buf.p2(message.delay);
//
// Fixed 5-byte payload. Caller wraps in:
//
//	p.writeOut(gameserver.OpSynthSound, buf.Bytes())
//
// Out-of-range script values silently truncate at the cast boundary
// (TS encoder behavior: JS implicit narrowing in p1/p2). Caller is
// responsible for the cast (see (*Player).PlaySynth).
func encodeSynthSound(buf *packet.Packet, synth uint16, loops uint8, delay uint16) {
	buf.P2(synth)
	buf.P1(loops)
	buf.P2(delay)
}
