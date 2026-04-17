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
