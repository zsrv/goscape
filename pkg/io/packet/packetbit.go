package packet

// Bitmask[n] has the low n bits set (n in [0,32]). Mirrors TS
// Packet.bitmask at Packet.ts:17 @43e02957 — made public at revision 254
// because Player.getVarBit/setVarBit compute varbit masks from it
// (TS Player.ts:1757,1769).
var Bitmask = []uint32{
	0,
	0x1, 0x3, 0x7, 0xF,
	0x1F, 0x3F, 0x7F, 0xFF,
	0x1FF, 0x3FF, 0x7FF, 0xFFF,
	0x1FFF, 0x3FFF, 0x7FFF, 0xFFFF,
	0x1FFFF, 0x3FFFF, 0x7FFFF, 0xFFFFF,
	0x1FFFFF, 0x3FFFFF, 0x7FFFFF, 0xFFFFFF,
	0x1FFFFFF, 0x3FFFFFF, 0x7FFFFFF, 0xFFFFFFF,
	0x1FFFFFFF, 0x3FFFFFFF, 0x7FFFFFFF, 0xFFFFFFFF,
}

// AccessBits changes the stream position for bit access.
//
// Byte access functions must not be used again until [Packet.AccessBytes]
// is called.
func (p *Packet) AccessBits() {
	p.BitPos = p.Pos << 3
}

// AccessBytes changes the stream position for byte access.
//
// This only needs to be called after calling [Packet.AccessBits],
// before using byte access functions again.
func (p *Packet) AccessBytes() {
	p.Pos = (p.BitPos + 7) >> 3
}

// GBit returns the next n bits in the [Packet].
//
// Returns int to match TS Packet.gBit (Packet.ts:384, returns number): the
// accumulator must be wider than a byte or reads of more than 8 bits silently
// truncate. The bitmask table spans 32 bits, so n may be up to 32. L36.
func (p *Packet) GBit(n int) int {
	bytePos := p.BitPos >> 3
	bitsRemaining := 8 - (p.BitPos & 0x7)
	value := 0
	p.BitPos += n

	for ; n > bitsRemaining; bitsRemaining = 8 {
		value += int(p.Data[bytePos]&uint8(Bitmask[bitsRemaining])) << (n - bitsRemaining)
		bytePos++
		n -= bitsRemaining
	}

	if n == bitsRemaining {
		value += int(p.Data[bytePos] & uint8(Bitmask[bitsRemaining]))
	} else {
		value += int((p.Data[bytePos] >> (bitsRemaining - n)) & uint8(Bitmask[n]))
	}

	return value
}

func (p *Packet) PBit(n int, value int) {
	bytePos := p.BitPos >> 3
	remaining := 8 - (p.BitPos & 7)
	p.BitPos += n

	// grow if necessary
	if bytePos+1 > p.Len() {
		_, err := p.Write(make([]byte, (bytePos+1)-p.Len()))
		if err != nil {
			panic(err)
		}
	}

	for ; n > remaining; remaining = 8 {
		p.Data[bytePos] &= byte(^Bitmask[remaining])
		p.Data[bytePos] |= byte(uint32(value>>(n-remaining)) & Bitmask[remaining])
		bytePos += 1
		n -= remaining

		// grow if necessary
		if bytePos+1 > p.Len() {
			//b.Grow((bytePos + 1) - b.Len())
			p.Write(make([]byte, (bytePos+1)-p.Len()))
		}
	}

	if n == remaining {
		p.Data[bytePos] &= byte(^Bitmask[remaining])
		p.Data[bytePos] |= byte(value) & byte(Bitmask[remaining])
	} else {
		p.Data[bytePos] &= byte(int(^Bitmask[n]) << (remaining - n))
		p.Data[bytePos] |= byte((uint32(value) & Bitmask[n]) << (remaining - n))
	}
}
