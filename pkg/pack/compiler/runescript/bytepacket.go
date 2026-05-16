// pkg/pack/compiler/runescript/bytepacket.go
package runescript

// crcTable is the IEEE 802.3 polynomial table used by Crc32.
// Mirrors TS BytePacket.ts L1-19.
var crcTable = func() [256]uint32 {
	var t [256]uint32
	for b := range 256 {
		r := uint32(b)
		for range 8 {
			if r&1 == 1 {
				r = (r >> 1) ^ 0xedb88320
			} else {
				r >>= 1
			}
		}
		t[b] = r
	}
	return t
}()

// Crc32 returns the signed-int32 form of ~crc, matching TS BytePacket.crc32
// (BytePacket.ts L21-27).
func Crc32(data []byte) int32 {
	crc := uint32(0xffffffff)
	for _, b := range data {
		crc = (crc >> 8) ^ crcTable[(crc^uint32(b))&0xff]
	}
	return int32(^crc)
}

// ByteWriter is an append-only big-endian byte buffer with a doubling growth
// policy. Mirrors TS BytePacket.ByteWriter (BytePacket.ts L29-87).
type ByteWriter struct {
	buf    []byte
	offset int
}

// NewByteWriter allocates a ByteWriter with an initial buffer size of
// max(64, initialSize). Mirrors TS constructor L33.
func NewByteWriter(initialSize int) *ByteWriter {
	if initialSize < 64 {
		initialSize = 64
	}
	return &ByteWriter{buf: make([]byte, initialSize)}
}

// P1 writes one byte. Mirrors TS p1 L37-41.
func (w *ByteWriter) P1(v int) {
	w.ensure(1)
	w.buf[w.offset] = byte(v & 0xff)
	w.offset++
}

// P2 writes a 16-bit big-endian value. Mirrors TS p2 L43-47.
func (w *ByteWriter) P2(v int) {
	w.ensure(2)
	w.buf[w.offset] = byte((v >> 8) & 0xff)
	w.buf[w.offset+1] = byte(v & 0xff)
	w.offset += 2
}

// P4 writes a 32-bit big-endian signed value. Mirrors TS p4 L49-53.
func (w *ByteWriter) P4(v int32) {
	w.ensure(4)
	u := uint32(v)
	w.buf[w.offset] = byte((u >> 24) & 0xff)
	w.buf[w.offset+1] = byte((u >> 16) & 0xff)
	w.buf[w.offset+2] = byte((u >> 8) & 0xff)
	w.buf[w.offset+3] = byte(u & 0xff)
	w.offset += 4
}

// PSmart2or4 writes a value as 2 bytes if <32768, else as 4 bytes with the
// high bit set. Mirrors TS pSmart2or4 L55-61.
func (w *ByteWriter) PSmart2or4(v int) {
	if v < 32768 {
		w.P2(v)
	} else {
		w.P4(int32(uint32(v) | 0x80000000))
	}
}

// PData appends raw bytes. Mirrors TS pdata L63-67.
func (w *ByteWriter) PData(data []byte) {
	w.ensure(len(data))
	copy(w.buf[w.offset:], data)
	w.offset += len(data)
}

// Bytes returns the active prefix of the buffer (no copy). Mirrors TS
// toBuffer L69-71 (which uses Buffer.subarray, also a view).
func (w *ByteWriter) Bytes() []byte {
	return w.buf[:w.offset]
}

// Len returns the current write offset. Test helper; TS has no equivalent.
func (w *ByteWriter) Len() int {
	return w.offset
}

// ensure doubles the underlying buffer until offset+extra fits.
// Mirrors TS ensure L73-86.
func (w *ByteWriter) ensure(extra int) {
	if w.offset+extra <= len(w.buf) {
		return
	}
	nextSize := len(w.buf) * 2
	for w.offset+extra > nextSize {
		nextSize *= 2
	}
	next := make([]byte, nextSize)
	copy(next, w.buf[:w.offset])
	w.buf = next
}
