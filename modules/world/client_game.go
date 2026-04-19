package world

import (
	"github.com/zsrv/goscape/pkg/io/protocol"
	gameclient "github.com/zsrv/goscape/pkg/io/protocol/game/client"
)

// handleGame reads and dispatches ISAAC-encrypted game packets from c.in.
//
// It drains all fully-buffered packets in a loop and returns ErrPayloadTooSmall
// when no complete packet is available. The caller (handleTCPConn) will then
// wait for more TCP data before calling handleData() again.
//
// c.opcode and c.waiting act as a resume cursor: they are set as soon as bytes
// are consumed from c.in, so a partial read on any TCP recv is safe.
func (c *client) handleGame() error {
	if c.decryptor == nil {
		c.log.Error("decryptor nil in game state")
		return errCloseConn
	}

	for {
		// Read and ISAAC-decrypt the next opcode if we don't have one pending.
		if c.opcode == -1 {
			raw, err := c.in.Peek(1)
			if err != nil {
				return protocol.ErrPayloadTooSmall
			}
			decrypted := (int(raw[0]) - int(c.decryptor.GetNext())) & 0xff
			op := gameclient.Ops[decrypted]
			if op.Name == "" {
				c.log.Warn("unknown game opcode", "opcode", decrypted)
				return errCloseConn
			}
			c.in.Next(1) // consume opcode byte — ISAAC has already advanced
			c.opcode = decrypted
			c.waiting = op.PayloadSize
		}

		// Resolve 1-byte or 2-byte dynamic length prefix.
		if c.waiting == -1 {
			if c.in.Len() < 1 {
				return protocol.ErrPayloadTooSmall
			}
			c.waiting = int(c.in.Next(1)[0])
		} else if c.waiting == -2 {
			if c.in.Len() < 2 {
				return protocol.ErrPayloadTooSmall
			}
			b := c.in.Next(2)
			c.waiting = int(uint16(b[0])<<8 | uint16(b[1]))
			if c.waiting > 1600 {
				c.log.Warn("oversized game packet, closing", "opcode", c.opcode, "size", c.waiting)
				return errCloseConn
			}
		}

		// Wait for the full payload.
		if c.in.Len() < c.waiting {
			return protocol.ErrPayloadTooSmall
		}

		// Consume payload and dispatch. Reset c.opcode before calling the
		// handler so the cursor is clean for the next packet.
		payload := c.in.Next(c.waiting)
		opcode := c.opcode
		c.opcode = -1

		c.log.Debug("game packet", "opcode", opcode, "name", gameclient.Ops[opcode].Name, "len", len(payload))

		if handler := gameHandlers[opcode]; handler != nil {
			if err := handler(c, payload); err != nil {
				return err
			}
		}
	}
}
