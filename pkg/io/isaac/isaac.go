package io

const size = 256
const goldenRatio uint32 = 0x9E3779B9

type Isaac struct {
	count int32
	rsl   [size]uint32
	mem   [size]uint32
	a     uint32
	b     uint32
	c     uint32
}

func New(seed [4]uint32) *Isaac {
	isaac := new(Isaac)

	for i := range seed {
		isaac.rsl[i] = seed[i]
	}

	isaac.init()

	return isaac
}

func (is *Isaac) GetNext() uint32 {
	c := is.count
	is.count--
	if c == 0 {
		is.isaac()
		is.count = size - 1
	}
	return is.rsl[is.count]
}

func (is *Isaac) init() {
	a := goldenRatio
	b := goldenRatio
	c := goldenRatio
	d := goldenRatio
	e := goldenRatio
	f := goldenRatio
	g := goldenRatio
	h := goldenRatio

	for i := 0; i < 4; i++ {
		a ^= b << 11
		d += a
		b += c

		b ^= c >> 2
		e += b
		c += d

		c ^= d << 8
		f += c
		d += e

		d ^= e >> 16
		g += d
		e += f

		e ^= f << 10
		h += e
		f += g

		f ^= g >> 4
		a += f
		g += h

		g ^= h << 8
		b += g
		h += a

		h ^= a >> 9
		c += h
		a += b
	}

	for i := 0; i < size; i += 8 {
		a += is.rsl[i]
		b += is.rsl[i+1]
		c += is.rsl[i+2]
		d += is.rsl[i+3]
		e += is.rsl[i+4]
		f += is.rsl[i+5]
		g += is.rsl[i+6]
		h += is.rsl[i+7]

		a ^= b << 11
		d += a
		b += c

		b ^= c >> 2
		e += b
		c += d

		c ^= d << 8
		f += c
		d += e

		d ^= e >> 16
		g += d
		e += f

		e ^= f << 10
		h += e
		f += g

		f ^= g >> 4
		a += f
		g += h

		g ^= h << 8
		b += g
		h += a

		h ^= a >> 9
		c += h
		a += b

		is.mem[i] = a
		is.mem[i+1] = b
		is.mem[i+2] = c
		is.mem[i+3] = d
		is.mem[i+4] = e
		is.mem[i+5] = f
		is.mem[i+6] = g
		is.mem[i+7] = h
	}

	for i := 0; i < size; i += 8 {
		a += is.mem[i]
		b += is.mem[i+1]
		c += is.mem[i+2]
		d += is.mem[i+3]
		e += is.mem[i+4]
		f += is.mem[i+5]
		g += is.mem[i+6]
		h += is.mem[i+7]

		a ^= b << 11
		d += a
		b += c

		b ^= c >> 2
		e += b
		c += d

		c ^= d << 8
		f += c
		d += e

		d ^= e >> 16
		g += d
		e += f

		e ^= f << 10
		h += e
		f += g

		f ^= g >> 4
		a += f
		g += h

		g ^= h << 8
		b += g
		h += a

		h ^= a >> 9
		c += h
		a += b

		is.mem[i] = a
		is.mem[i+1] = b
		is.mem[i+2] = c
		is.mem[i+3] = d
		is.mem[i+4] = e
		is.mem[i+5] = f
		is.mem[i+6] = g
		is.mem[i+7] = h
	}

	is.isaac()
	is.count = size
}

func (is *Isaac) isaac() {
	is.c++
	is.b += is.c

	for i := 0; i < size; i++ {
		x := is.mem[i]

		switch i & 3 {
		case 0:
			is.a ^= is.a << 13
		case 1:
			is.a ^= is.a >> 6
		case 2:
			is.a ^= is.a << 2
		case 3:
			is.a ^= is.a >> 16
		}

		is.a += is.mem[(i+128)&0xFF]

		y := is.mem[(x>>2)&0xFF] + is.a + is.b
		is.mem[i] = y
		is.b = is.mem[(y>>10)&0xFF] + x
		is.rsl[i] = is.b
	}
}
