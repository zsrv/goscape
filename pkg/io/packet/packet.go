package packet

import (
	"bufio"
	"hash/crc32"
	"io"
	"log"
	"math/big"
	"os"
	"path/filepath"
	"sync"
)

// GetCRC returns the checksum of length bytes in src beginning at offset.
func GetCRC(src []uint8, offset int, length int) uint32 {
	return crc32.ChecksumIEEE(src[offset : offset+length])
}

// CheckCRC returns true if the checksum of length bytes in src
// beginning at offset matches expected.
func CheckCRC(src []uint8, offset int, length int, expected uint32) bool {
	return GetCRC(src, offset, length) == expected
}

var (
	packet100Pool = sync.Pool{
		New: func() any { return NewPacket(make([]byte, 0, 100)) },
	}

	packet5000Pool = sync.Pool{
		New: func() any { return NewPacket(make([]byte, 0, 5_000)) },
	}

	packet30000Pool = sync.Pool{
		New: func() any { return NewPacket(make([]byte, 0, 30_000)) },
	}

	packet100000Pool = sync.Pool{
		New: func() any { return NewPacket(make([]byte, 0, 100_000)) },
	}

	packet500000Pool = sync.Pool{
		New: func() any { return NewPacket(make([]byte, 0, 500_000)) },
	}

	packet2000000Pool = sync.Pool{
		New: func() any { return NewPacket(make([]byte, 0, 2_000_000)) },
	}
)

// poolForCapacity returns the pool whose capacity tier covers size,
// or nil if size exceeds all tiers.
func poolForCapacity(size int) *sync.Pool {
	switch {
	case size <= 100:
		return &packet100Pool
	case size <= 5_000:
		return &packet5000Pool
	case size <= 30_000:
		return &packet30000Pool
	case size <= 100_000:
		return &packet100000Pool
	case size <= 500_000:
		return &packet500000Pool
	case size <= 2_000_000:
		return &packet2000000Pool
	}
	return nil
}

// Alloc returns a reset Packet from the pool tier that covers size.
// If the pool is empty or size exceeds all tiers, a new Packet is allocated.
func Alloc(size int) *Packet {
	pool := poolForCapacity(size)
	if pool != nil {
		if v := pool.Get(); v != nil {
			p := v.(*Packet)
			p.Reset()
			return p
		}
	}
	return NewPacket(make([]byte, 0, size))
}

func (p *Packet) Peek(n int) ([]byte, error) {
	if p.Len() < n {
		return nil, io.EOF
	}
	return p.Data[p.Pos : p.Pos+n], nil
}

func (p *Packet) Unused() int {
	return cap(p.Data) - p.Pos
}

func (p *Packet) Length() int {
	return len(p.Data)
}

// Release resets the Packet and returns it to the appropriate pool tier.
func (p *Packet) Release() {
	p.Reset()
	if pool := poolForCapacity(cap(p.Data)); pool != nil {
		pool.Put(p)
	}
}

func (p *Packet) Save(filePath string, length int, start int) error {
	// TODO: test
	// default length = p.Pos
	// default start = 0
	dir := filepath.Dir(filePath)
	_, err := os.Stat(dir)
	if os.IsNotExist(err) {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	f, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer f.Close()

	w := bufio.NewWriter(f)
	_, err = w.Write(p.Data[start : start+length])
	if err != nil {
		return err
	}
	err = w.Flush()
	if err != nil {
		return err
	}

	return nil
}

// Readers

// G1 gets 1 unsigned byte.
// TODO: error isn't returned if there are no bytes to read sometimes. handle this for all getters somehow
func (p *Packet) G1() uint8 {
	if p.Pos >= len(p.Data) {
		panic(io.EOF)
	}
	b := p.Data[p.Pos]
	p.Pos++
	return b
}

// G1B gets 1 signed byte.
func (p *Packet) G1B() int8 {
	return int8(p.G1())
}

// G2 gets 2 unsigned bytes.
func (p *Packet) G2() uint16 {
	if p.Pos+2 > len(p.Data) {
		panic(io.EOF)
	}
	v := uint16(p.Data[p.Pos])<<8 | uint16(p.Data[p.Pos+1])
	p.Pos += 2
	return v
}

// G2S gets 2 signed bytes.
func (p *Packet) G2S() int16 {
	return int16(p.G2())
}

// IG2 gets 2 unsigned bytes represented in little-endian byte order.
func (p *Packet) IG2() uint16 {
	if p.Pos+2 > len(p.Data) {
		panic(io.EOF)
	}
	v := uint16(p.Data[p.Pos]) | uint16(p.Data[p.Pos+1])<<8
	p.Pos += 2
	return v
}

// G3 gets 3 unsigned bytes.
func (p *Packet) G3() uint32 {
	if p.Pos+3 > len(p.Data) {
		panic(io.EOF)
	}
	v := uint32(p.Data[p.Pos])<<16 | uint32(p.Data[p.Pos+1])<<8 | uint32(p.Data[p.Pos+2])
	p.Pos += 3
	return v
}

// G4 gets 4 unsigned bytes.
func (p *Packet) G4() uint32 {
	if p.Pos+4 > len(p.Data) {
		panic(io.EOF)
	}
	v := uint32(p.Data[p.Pos])<<24 | uint32(p.Data[p.Pos+1])<<16 |
		uint32(p.Data[p.Pos+2])<<8 | uint32(p.Data[p.Pos+3])
	p.Pos += 4
	return v
}

// IG4 gets 4 unsigned bytes represented in little-endian byte order.
func (p *Packet) IG4() uint32 {
	if p.Pos+4 > len(p.Data) {
		panic(io.EOF)
	}
	v := uint32(p.Data[p.Pos+3])<<24 | uint32(p.Data[p.Pos+2])<<16 |
		uint32(p.Data[p.Pos+1])<<8 | uint32(p.Data[p.Pos])
	p.Pos += 4
	return v
}

// G8 gets 8 unsigned bytes.
func (p *Packet) G8() uint64 {
	return (uint64(p.G4()) << 32) + uint64(p.G4())
}

// GBool gets one byte and returns true if the value is 1,
// or false if the value is anything else.
func (p *Packet) GBool() bool {
	return p.G1() == 1
}

// GJStr gets a JagString, reading from the Packet
// until terminator is reached.
func (p *Packet) GJStr(terminator byte) string {
	// TODO: optimize this
	if p.Len() == 0 {
		log.Println("NO BYTES AVAILABLE IN GJSTR")
		return ""
	}
	// TODO: review the Packet.java version for charset
	start := p.Pos
	for p.Pos < len(p.Data) && p.Data[p.Pos] != terminator {
		p.Pos++
	}
	if p.Pos >= len(p.Data) {
		// terminator not found; consume remaining bytes and return what we have
		length := p.Pos - start
		return string(p.Data[start : start+length])
	}
	p.Pos++
	length := p.Pos - start - 1
	return string(p.Data[start : start+length])
}

// GJStrLF gets a newline-terminated JagString.
func (p *Packet) GJStrLF() string {
	return p.GJStr(10)
}

// GJStrNUL gets a NUL-terminated JagString.
func (p *Packet) GJStrNUL() string {
	return p.GJStr(0)
}

// GData gets data.
func (p *Packet) GData(dest []byte, length int) {
	copy(dest, p.Data[p.Pos:p.Pos+length])
	p.Pos += length
}

// GSmart gets a Smart value (range 0 to 32767).
func (p *Packet) GSmart() uint16 {
	if p.Data[p.Pos] >= 128 {
		return p.G2() - 32768
	} else {
		return uint16(p.G1())
	}
}

// GSmartS gets a signed Smart value (range -16384 to 16383).
func (p *Packet) GSmartS() int32 {
	// TODO: 2004scape server has this as uint.. maybe? maybe not
	if p.Data[p.Pos] >= 128 {
		return int32(p.G2() - 49152)
	} else {
		return int32(p.G1() - 64)
	}
}

// Writers

// P1 puts 1 unsigned byte.
func (p *Packet) P1(value uint8) {
	if err := p.WriteByte(value); err != nil {
		panic(err)
	}
}

// P2 puts 2 unsigned bytes.
func (p *Packet) P2(value uint16) {
	p.lastRead = opInvalid
	i, ok := p.tryGrowByReslice(2)
	if !ok {
		i = p.grow(2)
	}
	p.Data[i] = uint8(value >> 8)
	p.Data[i+1] = uint8(value)
}

// IP2 puts 2 unsigned bytes represented in little-endian byte order.
func (p *Packet) IP2(value uint16) {
	p.lastRead = opInvalid
	i, ok := p.tryGrowByReslice(2)
	if !ok {
		i = p.grow(2)
	}
	p.Data[i] = uint8(value)
	p.Data[i+1] = uint8(value >> 8)
}

// P3 puts 3 unsigned bytes.
func (p *Packet) P3(value uint32) {
	p.lastRead = opInvalid
	i, ok := p.tryGrowByReslice(3)
	if !ok {
		i = p.grow(3)
	}
	p.Data[i] = uint8(value >> 16)
	p.Data[i+1] = uint8(value >> 8)
	p.Data[i+2] = uint8(value)
}

// P4 puts 4 unsigned bytes.
// TODO: 2004scape has this as int32
func (p *Packet) P4(value uint32) {
	p.lastRead = opInvalid
	i, ok := p.tryGrowByReslice(4)
	if !ok {
		i = p.grow(4)
	}
	p.Data[i] = uint8(value >> 24)
	p.Data[i+1] = uint8(value >> 16)
	p.Data[i+2] = uint8(value >> 8)
	p.Data[i+3] = uint8(value)
}

// IP4 puts 4 unsigned bytes represented in little-endian byte order.
// TODO: 2004scape has this as int32
func (p *Packet) IP4(value uint32) {
	p.lastRead = opInvalid
	i, ok := p.tryGrowByReslice(4)
	if !ok {
		i = p.grow(4)
	}
	p.Data[i] = uint8(value)
	p.Data[i+1] = uint8(value >> 8)
	p.Data[i+2] = uint8(value >> 16)
	p.Data[i+3] = uint8(value >> 24)
}

// P8 puts 8 unsigned bytes.
// TODO: 2004scape has this as int64
func (p *Packet) P8(value uint64) {
	p.lastRead = opInvalid
	i, ok := p.tryGrowByReslice(8)
	if !ok {
		i = p.grow(8)
	}
	p.Data[i] = uint8(value >> 56)
	p.Data[i+1] = uint8(value >> 48)
	p.Data[i+2] = uint8(value >> 40)
	p.Data[i+3] = uint8(value >> 32)
	p.Data[i+4] = uint8(value >> 24)
	p.Data[i+5] = uint8(value >> 16)
	p.Data[i+6] = uint8(value >> 8)
	p.Data[i+7] = uint8(value)
}

// PBool puts 1 if the value is true, or 0 if the value is false.
func (p *Packet) PBool(value bool) {
	v := uint8(0)
	if value {
		v = 1
	}
	err := p.WriteByte(v)
	if err != nil {
		panic(err)
	}
}

// PJStr puts a JagString, terminated by terminator.
func (p *Packet) PJStr(str string, terminator byte) {
	p.WriteString(str)
	if err := p.WriteByte(terminator); err != nil {
		panic(err)
	}
}

// PJStrLF puts a newline-terminated JagString.
func (p *Packet) PJStrLF(str string) {
	p.PJStr(str, 10)
}

// PJStrNUL puts a NUL-terminated JagString.
func (p *Packet) PJStrNUL(str string) {
	p.PJStr(str, 0)
}

// PData puts data.
// TODO: might have to add offset arg
// TODO: keep offset and length out, and just pass in the correct slice each time?
func (p *Packet) PData(src []byte) {
	_, err := p.Write(src)
	if err != nil {
		panic(err)
	}
}

// PSize1 puts a 1 byte size?
func (p *Packet) PSize1(length int) {
	p.Data[len(p.Data)-length-1] = uint8(length)
}

// PSize2 puts a size of 2 bytes?
func (p *Packet) PSize2(length int) {
	p.Data[len(p.Data)-length-2] = uint8(length >> 8)
	p.Data[len(p.Data)-length-1] = uint8(length)
}

// PSize4 puts the size of a byte sequence in the buffer
// as 4 bytes preceding the sequence.
func (p *Packet) PSize4(length int) {
	p.Data[len(p.Data)-length-4] = uint8(length >> 24)
	p.Data[len(p.Data)-length-3] = uint8(length >> 16)
	p.Data[len(p.Data)-length-2] = uint8(length >> 8)
	p.Data[len(p.Data)-length-1] = uint8(length)
}

// PSmart puts a Smart value.
// TODO: does it make sense to convert to unsigned?
func (p *Packet) PSmart(value int32) {
	if value >= 0 && value < 128 {
		p.P1(uint8(value))
	} else if value >= 0 && value < 32768 {
		p.P2(uint16(value + 32768))
	} else {
		panic("value out of range")
	}
}

// PSmartS puts a Smart value (signed?).
// TODO: does it make sense to convert to unsigned?
func (p *Packet) PSmartS(value int32) {
	if value < 64 && value >= -64 {
		p.P1(uint8(value + 64))
	} else if value < 16384 && value >= -16384 {
		p.P2(uint16(value + 49152))
	} else {
		panic("value out of range")
	}
}

// RSA

// RSAEnc RSA-encrypts the buffer contents.
func (p *Packet) RSAEnc(modulus *big.Int, exponent *big.Int) {
	//length := p.Pos
	length := p.Len()
	//p.Pos = 0

	plaintextBytes := make([]byte, length)
	p.GData(plaintextBytes, length)

	plaintext := new(big.Int).SetBytes(plaintextBytes)
	ciphertext := plaintext.Exp(plaintext, exponent, modulus)
	ciphertextBytes := ciphertext.Bytes()

	//p.Pos = 0
	p.Reset()
	p.P1(uint8(len(ciphertextBytes)))
	p.PData(ciphertextBytes)
}

// RSADec RSA-decrypts the next block in the buffer using the provided modulus and
// private exponent, mirroring the RSAEnc signature.
func (p *Packet) RSADec(modulus *big.Int, exponent *big.Int) (*Packet, error) {
	// TODO: add a test for this
	// TODO: make two funcs: one that can use raw key components (for the original key)
	// and one that can use a PEM/DER key from disk or something (normal keys for later)

	// TODO: when reading a cert, if stdlib fails because of exponent, extract values manually

	// we aren't using BigInteger, so we have to do this manually
	numBytes := p.G1()
	rsax := make([]byte, numBytes)
	p.GData(rsax, int(numBytes))
	if len(rsax) == 65 && rsax[0] == 0 {
		// Java BigInteger adds a 0 to indicate it's unsigned
		rsax = rsax[1:]
	} else if len(rsax) == 63 {
		// Java BigInteger didn't pad to 64
		temp := make([]byte, 64)
		copy(temp[1:], rsax)
		rsax = temp
	}

	// RSA raw decryption (no padding)
	// better: take decrypt() from crypto/rsa/rsa.go
	c := new(big.Int).SetBytes(rsax)
	decrypted := c.Exp(c, exponent, modulus).Bytes()
	decryptedBuf := NewPacket(decrypted)

	// BigInteger would also remove all the preceding 0s, so we seek past them
	for decryptedBuf.Pos < len(decryptedBuf.Data) && decryptedBuf.Data[decryptedBuf.Pos] == 0 {
		decryptedBuf.G1()
	}

	return decryptedBuf, nil
}
