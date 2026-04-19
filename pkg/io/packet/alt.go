package packet

// Alt byte writers — Jagex's scrambled protocol variants.
// Reference: github.com/2004scape/rsbuf branch 225, src/packet.rs.

func (p *Packet) P1Alt1(v uint8) { p.P1(v + 128) }
func (p *Packet) P1Alt2(v uint8) { p.P1(128 - v) }
func (p *Packet) P1Alt3(v uint8) { p.P1(uint8(-int8(v))) }

// P2Alt2 writes a u16 in little-endian order.
// Equivalent to the existing IP2 method on Packet.
func (p *Packet) P2Alt2(v uint16) { p.IP2(v) }

// P4Alt2 writes a u32 in middle-endian order: bytes 2, 3, 0, 1.
func (p *Packet) P4Alt2(v uint32) {
	p.P1(uint8(v >> 16))
	p.P1(uint8(v >> 24))
	p.P1(uint8(v))
	p.P1(uint8(v >> 8))
}

// PDataAlt1 writes data with each byte offset by +128.
func (p *Packet) PDataAlt1(b []byte) {
	for _, x := range b {
		p.P1(x + 128)
	}
}

// PDataAlt2 writes data with each byte transformed as (128 - b).
func (p *Packet) PDataAlt2(b []byte) {
	for _, x := range b {
		p.P1(128 - x)
	}
}
