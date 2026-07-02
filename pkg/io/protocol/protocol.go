package protocol

import (
	"errors"

	"github.com/zsrv/goscape/pkg/io/packet"
)

var (
	ErrIncorrectOpcode     = errors.New("incorrect opcode")
	ErrIncorrectDataLength = errors.New("incorrect data length")
	ErrPayloadTooSmall     = errors.New("payload too small")
	ErrPayloadTooLarge     = errors.New("payload too large")
)

type Operation struct {
	Name        string
	PayloadSize int
	Opcode      uint8
}

// CheckPacketLength reports whether p already buffers a complete packet for
// operation o, returning the packet's total size (header + payload) as the
// second value would suggest — but the first return's meaning is NOT
// uniform across the false-returning paths, and callers must branch on the
// bool rather than interpret the int the same way in every case:
//
//   - On the two "not enough header yet" paths (dynamic -2/-1 payload sizes
//     with fewer than 3/2 bytes buffered), the first return is p.Len() —
//     bytes currently AVAILABLE — because the declared packet size cannot be
//     computed until the length-prefix bytes themselves arrive.
//   - On every other path (the length prefix is known, or the size is
//     static), the first return is the declared packetSize — bytes NEEDED —
//     regardless of whether that size is fully buffered yet (the bool is what
//     tells you).
//
// Only when the bool is true is the first return guaranteed to equal the
// packet's full size. Do not compare the int across calls, or diff it
// against p.Len() again, expecting one consistent semantic.
func CheckPacketLength(p *packet.Packet, o Operation) (int, bool) {
	headerSize := 1 // opcode
	payloadSize := 0
	switch o.PayloadSize {
	case -2: // dynamic, two-byte payload size
		if p.Len() < 1+2 { // opcode + payload size
			return p.Len(), false
		}
		headerSize += 2 // payload size
		tl, err := p.Peek(3)
		if err != nil {
			return p.Len(), false
		}
		payloadSize = int(uint16(tl[1])<<8 | uint16(tl[2])) // Packet.G2(); skip opcode
	case -1: // dynamic, one-byte payload size
		if p.Len() < 1+1 { // opcode + payload size
			return p.Len(), false
		}
		headerSize += 1 // payload size
		tl, err := p.Peek(2)
		if err != nil {
			return p.Len(), false
		}
		payloadSize = int(tl[1]) // Packet.G1(); skip opcode
	default: // static payload size
		payloadSize = o.PayloadSize
	}

	packetSize := headerSize + payloadSize
	if p.Len() < packetSize {
		return packetSize, false
	}

	return packetSize, true
}
