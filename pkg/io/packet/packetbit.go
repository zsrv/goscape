package packet

var bitmask = []uint32{
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
func (p *Packet) GBit(n int) uint8 {
	bytePos := p.BitPos >> 3
	bitsRemaining := 8 - (p.BitPos & 0x7)
	value := uint8(0)
	p.BitPos += n

	for ; n > bitsRemaining; bitsRemaining = 8 {
		value += (p.Data[bytePos] & uint8(bitmask[bitsRemaining])) << (n - bitsRemaining)
		bytePos++
		n -= bitsRemaining
	}

	if n == bitsRemaining {
		value += p.Data[bytePos] & uint8(bitmask[bitsRemaining])
	} else {
		value += (p.Data[bytePos] >> (bitsRemaining - n)) & uint8(bitmask[n])
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
		p.Data[bytePos] &= byte(^bitmask[remaining])
		p.Data[bytePos] |= byte(uint32(value>>(n-remaining)) & bitmask[remaining])
		bytePos += 1
		n -= remaining

		// grow if necessary
		if bytePos+1 > p.Len() {
			//b.Grow((bytePos + 1) - b.Len())
			p.Write(make([]byte, (bytePos+1)-p.Len()))
		}
	}

	if n == remaining {
		p.Data[bytePos] &= byte(^bitmask[remaining])
		p.Data[bytePos] |= byte(value) & byte(bitmask[remaining])
	} else {
		p.Data[bytePos] &= byte(int(^bitmask[n]) << (remaining - n))
		p.Data[bytePos] |= byte((uint32(value) & bitmask[n]) << (remaining - n))
	}
}
