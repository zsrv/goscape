// Package gziputil — cf-zlib trees.c port.
// Huffman tree construction, bit emission, and block flushing logic
// faithful to cf-zlib trees.c (commit 886098f3).
package gziputil

// cf-zlib trees.c — constant tables ported verbatim from trees.h / trees.c.

// cf-zlib trees.c:62-63
var extraLBits = [lengthCodes]int{
	0, 0, 0, 0, 0, 0, 0, 0, 1, 1, 1, 1, 2, 2, 2, 2,
	3, 3, 3, 3, 4, 4, 4, 4, 5, 5, 5, 5, 0,
}

// cf-zlib trees.c:65-66
var extraDBits = [dCodes]int{
	0, 0, 0, 0, 1, 1, 2, 2, 3, 3, 4, 4, 5, 5, 6, 6,
	7, 7, 8, 8, 9, 9, 10, 10, 11, 11, 12, 12, 13, 13,
}

// cf-zlib trees.c:68-69
var extraBLBits = [blCodes]int{
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2, 3, 7,
}

// cf-zlib trees.c:71-72
var blOrder = [blCodes]uint8{
	16, 17, 18, 0, 8, 7, 9, 6, 10, 5, 11, 4, 12, 3, 13, 2, 14, 1, 15,
}

// static_ltree — cf-zlib trees.h (generated from GEN_TREES_H).
// Format: [code, len] pairs for indices 0..L_CODES+1.
// cf-zlib trees.h:3-62
var staticLTree [lCodes + 2]ctData

// static_dtree — cf-zlib trees.h:64-71
var staticDTree [dCodes]ctData

// _dist_code — cf-zlib trees.h:73-100
var distCode [distCodeLen]uint8

// _length_code — cf-zlib trees.h:102-116
var lengthCode [maxMatch - minMatch + 1]uint8

// base_length — cf-zlib trees.h:118-120
var baseLength [lengthCodes]int

// base_dist — cf-zlib trees.h:123-127
var baseDist [dCodes]int

func init() {
	trStaticInit()
}

// trStaticInit initialises the static tables.
// cf-zlib trees.c:201-280
func trStaticInit() {
	// Initialize length_code / base_length
	length := 0
	for code := 0; code < lengthCodes-1; code++ {
		baseLength[code] = length
		for n := 0; n < 1<<extraLBits[code]; n++ {
			lengthCode[length] = uint8(code)
			length++
		}
	}
	// length 255 (match length 258) override — cf-zlib trees.c:236
	lengthCode[length-1] = uint8(lengthCodes - 1)

	// Initialize dist_code / base_dist
	dist := 0
	for code := 0; code < 16; code++ {
		baseDist[code] = dist
		for n := 0; n < 1<<extraDBits[code]; n++ {
			distCode[dist] = uint8(code)
			dist++
		}
	}
	// dist 256..511 — cf-zlib trees.c:247-253
	dist >>= 7
	for code := 16; code < dCodes; code++ {
		baseDist[code] = dist << 7
		for n := 0; n < 1<<(extraDBits[code]-7); n++ {
			distCode[256+dist] = uint8(code)
			dist++
		}
	}

	// Build static literal tree bit-lengths — cf-zlib trees.c:257-267
	var blCount [maxBits + 1]uint16
	n := 0
	for n <= 143 {
		staticLTree[n].Len = 8
		blCount[8]++
		n++
	}
	for n <= 255 {
		staticLTree[n].Len = 9
		blCount[9]++
		n++
	}
	for n <= 279 {
		staticLTree[n].Len = 7
		blCount[7]++
		n++
	}
	for n <= 287 {
		staticLTree[n].Len = 8
		blCount[8]++
		n++
	}
	genCodes(staticLTree[:], lCodes+1, blCount[:])

	// Static distance tree — cf-zlib trees.c:269-273
	for n := 0; n < dCodes; n++ {
		staticDTree[n].Len = 5
		staticDTree[n].Code = uint16(biReverse(uint(n), 5))
	}
}

// ctData mirrors cf-zlib deflate.h ct_data_s.
// cf-zlib deflate.h:57-66
type ctData struct {
	Code uint16 // fc.code (also fc.freq)
	Len  uint16 // dl.len  (also dl.dad)
}

// Freq and Dad are aliases that use the same fields as Code/Len.
// We alias as methods to match naming used in the port without extra structs.
func (c *ctData) Freq() *uint16 { return &c.Code }
func (c *ctData) Dad() *uint16  { return &c.Len }

// dCode maps distance to distance code.
// cf-zlib deflate.h:299-303
func dCode(dist uint) uint {
	if dist < 256 {
		return uint(distCode[dist])
	}
	return uint(distCode[256+(dist>>7)])
}

// biReverse reverses the first len bits of a code.
// cf-zlib trees.c:1150-1159
func biReverse(code uint, length int) uint {
	res := uint(0)
	for {
		res |= code & 1
		code >>= 1
		res <<= 1
		length--
		if length == 0 {
			break
		}
	}
	return res >> 1
}

// genCodes generates Huffman codes from bit lengths.
// cf-zlib trees.c:530-558
func genCodes(tree []ctData, maxCode int, blCount []uint16) {
	var nextCode [maxBits + 1]uint16
	code := uint16(0)
	for bits := 1; bits <= maxBits; bits++ {
		code = (code + blCount[bits-1]) << 1
		nextCode[bits] = code
	}
	for n := 0; n <= maxCode; n++ {
		ln := int(tree[n].Len)
		if ln == 0 {
			continue
		}
		tree[n].Code = uint16(biReverse(uint(nextCode[ln]), ln))
		nextCode[ln]++
	}
}

// trInit initialises the tree structures for a new stream.
// cf-zlib trees.c:346-367
func trInit(s *deflateState) {
	s.lDesc = treeDesc{dynTree: s.dynLTree[:], statDesc: &staticLDesc}
	s.dDesc = treeDesc{dynTree: s.dynDTree[:], statDesc: &staticDDesc}
	s.blDesc = treeDesc{dynTree: s.blTree[:], statDesc: &staticBLDesc}
	s.biBuf = 0
	s.biValid = 0
	initBlock(s)
}

// tree descriptor static instances
// cf-zlib trees.c:125-132
var staticLDesc = staticTreeDesc{
	staticTree: staticLTree[:],
	extraBits:  extraLBits[:],
	extraBase:  literals + 1,
	elems:      lCodes,
	maxLength:  maxBits,
}

var staticDDesc = staticTreeDesc{
	staticTree: staticDTree[:],
	extraBits:  extraDBits[:],
	extraBase:  0,
	elems:      dCodes,
	maxLength:  maxBits,
}

var staticBLDesc = staticTreeDesc{
	staticTree: nil,
	extraBits:  extraBLBits[:],
	extraBase:  0,
	elems:      blCodes,
	maxLength:  maxBLBits,
}

// initBlock initialises a new block.
// cf-zlib trees.c:372-384
func initBlock(s *deflateState) {
	for n := range lCodes {
		s.dynLTree[n].Code = 0
	}
	for n := range dCodes {
		s.dynDTree[n].Code = 0
	}
	for n := range blCodes {
		s.blTree[n].Code = 0
	}
	s.dynLTree[endBlock].Code = 1 // freq = 1
	s.optLen = 0
	s.staticLen = 0
	s.symNext = 0
	s.matches = 0
}

// pqDownHeap restores heap property from node k downward.
// cf-zlib trees.c:415-434
func pqDownHeap(s *deflateState, tree []ctData, k int) {
	v := s.heap[k]
	j := k << 1
	for j <= s.heapLen {
		if j < s.heapLen &&
			smaller(tree, s.heap[j+1], s.heap[j], s.depth[:]) {
			j++
		}
		if smaller(tree, v, s.heap[j], s.depth[:]) {
			break
		}
		s.heap[k] = s.heap[j]
		k = j
		j <<= 1
	}
	s.heap[k] = v
}

// smaller compares two tree nodes.
// cf-zlib trees.c:405-407
func smaller(tree []ctData, n, m int, depth []uint8) bool {
	return tree[n].Code < tree[m].Code ||
		(tree[n].Code == tree[m].Code && depth[n] <= depth[m])
}

// genBitlen computes optimal bit lengths for a tree.
// In our ctData, both the frequency (fc.freq) and the code (fc.code) share the
// Code field; the Dad index (dl.dad) and bit length (dl.len) share the Len field.
// After build_tree: internal nodes have Len = Dad index (parent node index).
// gen_bitlen then overwrites Len with the actual bit length.
// cf-zlib trees.c:446-519
func genBitlen(s *deflateState, desc *treeDesc) {
	tree := desc.dynTree
	maxCode := desc.maxCode
	stree := desc.statDesc.staticTree
	extra := desc.statDesc.extraBits
	base := desc.statDesc.extraBase
	maxLength := desc.statDesc.maxLength

	for bits := range maxBits + 1 {
		s.blCount[bits] = 0
	}

	// Root: tree[heap[heapMax]].Len (Dad field) = 0 means depth = 0
	tree[s.heap[s.heapMax]].Len = 0

	overflow := 0
	for h := s.heapMax + 1; h < heapSize; h++ {
		n := s.heap[h]
		// tree[n].Len currently holds the parent (Dad) node index
		// cf-zlib: bits = tree[tree[n].Dad].Len + 1
		parentIdx := int(tree[n].Len)
		bits := int(tree[parentIdx].Len) + 1
		if bits > maxLength {
			bits = maxLength
			overflow++
		}
		tree[n].Len = uint16(bits) // overwrite Dad with actual length

		if n > maxCode {
			continue // internal node, not a leaf
		}
		s.blCount[bits]++
		xbits := 0
		if n >= base {
			xbits = extra[n-base]
		}
		f := tree[n].Code // freq stored in Code field
		s.optLen += uint64(f) * uint64(bits+xbits)
		if stree != nil {
			s.staticLen += uint64(f) * uint64(int(stree[n].Len)+xbits)
		}
	}
	if overflow == 0 {
		return
	}

	// Fix overflow — cf-zlib trees.c:488-499
	for {
		bits := maxLength - 1
		for s.blCount[bits] == 0 {
			bits--
		}
		s.blCount[bits]--
		s.blCount[bits+1] += 2
		s.blCount[maxLength]--
		overflow -= 2
		if overflow <= 0 {
			break
		}
	}

	// Recompute lengths — cf-zlib trees.c:505-519
	h := heapSize
	for bits := maxLength; bits != 0; bits-- {
		n := int(s.blCount[bits])
		for n != 0 {
			h--
			m := s.heap[h]
			if m > maxCode {
				continue
			}
			if int(tree[m].Len) != bits {
				// cf-zlib trees.c:513: opt_len += ((long)bits - (long)tree[m].Len) * (long)tree[m].Freq
				diff := int64(bits) - int64(tree[m].Len)
				s.optLen = uint64(int64(s.optLen) + diff*int64(tree[m].Code))
				tree[m].Len = uint16(bits)
			}
			n--
		}
	}
}

// buildTree constructs a Huffman tree.
// cf-zlib trees.c:569-648
func buildTree(s *deflateState, desc *treeDesc) {
	tree := desc.dynTree
	stree := desc.statDesc.staticTree
	elems := desc.statDesc.elems

	s.heapLen = 0
	s.heapMax = heapSize

	maxCode := -1
	for n := 0; n < elems; n++ {
		if tree[n].Code != 0 { // freq != 0
			s.heapLen++
			s.heap[s.heapLen] = n
			maxCode = n
			s.depth[n] = 0
		} else {
			tree[n].Len = 0
		}
	}

	// Ensure at least two codes — cf-zlib trees.c:597-603
	for s.heapLen < 2 {
		node := 0
		if maxCode < 2 {
			maxCode++
			node = maxCode
		}
		s.heapLen++
		s.heap[s.heapLen] = node
		tree[node].Code = 1 // freq = 1
		s.depth[node] = 0
		s.optLen--
		if stree != nil {
			s.staticLen -= uint64(stree[node].Len)
		}
	}
	desc.maxCode = maxCode

	// Establish sub-heaps — cf-zlib trees.c:609
	for n := s.heapLen / 2; n >= 1; n-- {
		pqDownHeap(s, tree, n)
	}

	// Build Huffman tree by combining least-frequent nodes — cf-zlib trees.c:614-637
	node := elems
	for {
		// pqremove
		n := s.heap[1]
		s.heap[1] = s.heap[s.heapLen]
		s.heapLen--
		pqDownHeap(s, tree, 1)

		m := s.heap[1] // next least frequent

		s.heapMax--
		s.heap[s.heapMax] = n
		s.heapMax--
		s.heap[s.heapMax] = m

		tree[node].Code = tree[n].Code + tree[m].Code // freq sum
		d1 := s.depth[n]
		d2 := s.depth[m]
		if d1 >= d2 {
			s.depth[node] = d1 + 1
		} else {
			s.depth[node] = d2 + 1
		}
		tree[n].Len = uint16(node) // Dad = node (stored in Len field)
		tree[m].Len = uint16(node)

		s.heap[1] = node
		node++
		pqDownHeap(s, tree, 1)

		if s.heapLen < 2 {
			break
		}
	}

	s.heapMax--
	s.heap[s.heapMax] = s.heap[1]

	genBitlen(s, desc)
	genCodes(tree, maxCode, s.blCount[:])
}

// scanTree scans a literal/distance tree to count bit-length code frequencies.
// cf-zlib trees.c:654-688
func scanTree(s *deflateState, tree []ctData, maxCode int) {
	prevLen := -1
	nextLen := int(tree[0].Len)
	count := 0
	maxCount := 7
	minCount := 4

	if nextLen == 0 {
		maxCount = 138
		minCount = 3
	}
	tree[maxCode+1].Len = 0xffff // guard

	for n := 0; n <= maxCode; n++ {
		curLen := nextLen
		nextLen = int(tree[n+1].Len)
		count++
		if count < maxCount && curLen == nextLen {
			continue
		} else if count < minCount {
			s.blTree[curLen].Code += uint16(count)
		} else if curLen != 0 {
			if curLen != prevLen {
				s.blTree[curLen].Code++
			}
			s.blTree[rep36].Code++
		} else if count <= 10 {
			s.blTree[repZ310].Code++
		} else {
			s.blTree[repZ11138].Code++
		}
		count = 0
		prevLen = curLen
		if nextLen == 0 {
			maxCount = 138
			minCount = 3
		} else if curLen == nextLen {
			maxCount = 6
			minCount = 3
		} else {
			maxCount = 7
			minCount = 4
		}
	}
}

// sendTree emits a literal/distance tree using bit-length codes.
// cf-zlib trees.c:695-736
func sendTree(s *deflateState, tree []ctData, maxCode int) {
	prevLen := -1
	nextLen := int(tree[0].Len)
	count := 0
	maxCount := 7
	minCount := 4

	if nextLen == 0 {
		maxCount = 138
		minCount = 3
	}

	for n := 0; n <= maxCode; n++ {
		curLen := nextLen
		nextLen = int(tree[n+1].Len)
		count++
		if count < maxCount && curLen == nextLen {
			continue
		} else if count < minCount {
			for i := count; i > 0; i-- {
				sendCode(s, curLen, s.blTree[:])
			}
		} else if curLen != 0 {
			if curLen != prevLen {
				sendCode(s, curLen, s.blTree[:])
				count--
			}
			sendCode(s, rep36, s.blTree[:])
			sendBits(s, uint64(count-3), 2)
		} else if count <= 10 {
			sendCode(s, repZ310, s.blTree[:])
			sendBits(s, uint64(count-3), 3)
		} else {
			sendCode(s, repZ11138, s.blTree[:])
			sendBits(s, uint64(count-11), 7)
		}
		count = 0
		prevLen = curLen
		if nextLen == 0 {
			maxCount = 138
			minCount = 3
		} else if curLen == nextLen {
			maxCount = 6
			minCount = 3
		} else {
			maxCount = 7
			minCount = 4
		}
	}
}

// buildBLTree builds the bit-length tree and returns max_blindex.
// cf-zlib trees.c:742-768
func buildBLTree(s *deflateState) int {
	scanTree(s, s.dynLTree[:], s.lDesc.maxCode)
	scanTree(s, s.dynDTree[:], s.dDesc.maxCode)
	buildTree(s, &s.blDesc)

	maxBlIndex := blCodes - 1
	for maxBlIndex >= 3 {
		if s.blTree[blOrder[maxBlIndex]].Len != 0 {
			break
		}
		maxBlIndex--
	}
	s.optLen += uint64(3*(maxBlIndex+1)) + 5 + 5 + 4
	return maxBlIndex
}

// sendAllTrees emits the dynamic block header.
// cf-zlib trees.c:775-797
func sendAllTrees(s *deflateState, lcodes, dcodes, blcodes int) {
	sendBits(s, uint64(lcodes-257), 5)
	sendBits(s, uint64(dcodes-1), 5)
	sendBits(s, uint64(blcodes-4), 4)
	for rank := 0; rank < blcodes; rank++ {
		sendBits(s, uint64(s.blTree[blOrder[rank]].Len), 3)
	}
	sendTree(s, s.dynLTree[:], lcodes-1)
	sendTree(s, s.dynDTree[:], dcodes-1)
}

// sendBits emits `len` bits of `val` (LSB first).
// cf-zlib trees.c:178-194 (64-bit XOR accumulator version)
func sendBits(s *deflateState, val uint64, length int) {
	s.biBuf ^= val << s.biValid
	s.biValid += length
	if s.biValid >= 64 {
		// flush 8 bytes
		_ = s.pendingBuf[s.pending+7] // bounds check hint
		putU64LE(s.pendingBuf[s.pending:], s.biBuf)
		s.pending += 8
		s.biValid -= 64
		if length > s.biValid {
			s.biBuf = val >> (length - s.biValid)
		} else {
			s.biBuf = 0
		}
	}
}

// sendCode emits the code for symbol c from tree.
// cf-zlib trees.c:163
func sendCode(s *deflateState, c int, tree []ctData) {
	sendBits(s, uint64(tree[c].Code), int(tree[c].Len))
}

// biFlush flushes the bit buffer keeping at most 7 bits.
// cf-zlib trees.c:1165-1177
func biFlush(s *deflateState) {
	for s.biValid >= 16 {
		putU16LE(s.pendingBuf[s.pending:], uint16(s.biBuf))
		s.pending += 2
		s.biBuf >>= 16
		s.biValid -= 16
	}
	if s.biValid >= 8 {
		s.pendingBuf[s.pending] = uint8(s.biBuf)
		s.pending++
		s.biBuf >>= 8
		s.biValid -= 8
	}
}

// biWindup aligns the bit buffer to a byte boundary.
// cf-zlib trees.c:1182-1199
func biWindup(s *deflateState) {
	for s.biValid >= 16 {
		putU16LE(s.pendingBuf[s.pending:], uint16(s.biBuf))
		s.pending += 2
		s.biBuf >>= 16
		s.biValid -= 16
	}
	if s.biValid > 8 {
		putU16LE(s.pendingBuf[s.pending:], uint16(s.biBuf))
		s.pending += 2
	} else if s.biValid > 0 {
		s.pendingBuf[s.pending] = uint8(s.biBuf)
		s.pending++
	}
	s.biBuf = 0
	s.biValid = 0
}

// copyBlock copies a stored block with optional 2×16-bit header.
// cf-zlib trees.c:1205-1222
func copyBlock(s *deflateState, buf []byte, ln int, header bool) {
	biWindup(s)
	if header {
		putU16LE(s.pendingBuf[s.pending:], uint16(ln))
		s.pending += 2
		putU16LE(s.pendingBuf[s.pending:], uint16(^ln))
		s.pending += 2
	}
	copy(s.pendingBuf[s.pending:], buf[:ln])
	s.pending += ln
}

// trStoredBlock emits a stored (uncompressed) block.
// cf-zlib trees.c:802-810
func trStoredBlock(s *deflateState, buf []byte, storedLen int, last bool) {
	lastBit := uint64(0)
	if last {
		lastBit = 1
	}
	sendBits(s, storedBlock<<1|lastBit, 3)
	copyBlock(s, buf, storedLen, true)
}

// trFlushBlockDebug is set during tests to capture block choice info.
var trFlushBlockDebug func(optLenB, staticLenB, storedLen uint64, last bool, symNext uint, rawOptLen, rawStaticLen uint64)

// trFlushBlockSymHook is set during tests to capture the sym buffer.
var trFlushBlockSymHook func(symBuf []byte, symNext uint)

// trFlushBlock chooses and emits the best encoding for the current block.
// cf-zlib trees.c:837-926
func trFlushBlock(s *deflateState, buf []byte, storedLen int, last bool) {
	var optLenB, staticLenB uint64
	maxBlIndex := 0

	if s.level > 0 {
		buildTree(s, &s.lDesc)
		buildTree(s, &s.dDesc)
		maxBlIndex = buildBLTree(s)

		optLenB = (s.optLen + 3 + 7) >> 3
		staticLenB = (s.staticLen + 3 + 7) >> 3

		// Use static if not worse — cf-zlib trees.c:875-877
		if staticLenB <= optLenB {
			optLenB = staticLenB
		}
	} else {
		optLenB = uint64(storedLen) + 5
		staticLenB = optLenB
	}

	if trFlushBlockDebug != nil {
		trFlushBlockDebug(optLenB, staticLenB, uint64(storedLen), last, s.symNext, s.optLen, s.staticLen)
	}
	if trFlushBlockSymHook != nil {
		trFlushBlockSymHook(s.symBuf[:s.symNext], s.symNext)
	}

	lastBit := uint64(0)
	if last {
		lastBit = 1
	}

	// Stored block if it saves bytes — cf-zlib trees.c:887-896
	if buf != nil && uint64(storedLen)+4 <= optLenB {
		trStoredBlock(s, buf, storedLen, last)
	} else if staticLenB == optLenB {
		sendBits(s, staticTrees<<1|lastBit, 3)
		compressBlock(s, staticLTree[:], staticDTree[:])
	} else {
		sendBits(s, dynTrees<<1|lastBit, 3)
		sendAllTrees(s, s.lDesc.maxCode+1, s.dDesc.maxCode+1, maxBlIndex+1)
		compressBlock(s, s.dynLTree[:], s.dynDTree[:])
	}
	initBlock(s)
	if last {
		biWindup(s)
	}
}

// compressBlock encodes the sym_buf using given Huffman trees.
// cf-zlib trees.c:957-1101 (optimised 64-bit accumulator version)
func compressBlock(s *deflateState, ltree, dtree []ctData) {
	bitBuf := s.biBuf
	filled := s.biValid

	// Inline send helper — mirrors the cf-zlib inline block in compress_block
	sendInline := func(val uint64, length int) {
		bitBuf ^= val << filled
		filled += length
		if filled >= 64 {
			putU64LE(s.pendingBuf[s.pending:], bitBuf)
			s.pending += 8
			filled -= 64
			if length > filled {
				bitBuf = val >> (length - filled)
			} else {
				bitBuf = 0
			}
		}
	}

	sx := uint(0)
	for sx < s.symNext {
		dist := uint(s.symBuf[sx]) | uint(s.symBuf[sx+1])<<8
		lc := int(s.symBuf[sx+2])
		sx += 3

		if dist == 0 {
			// Literal — cf-zlib trees.c:975-993
			sendInline(uint64(ltree[lc].Code), int(ltree[lc].Len))
		} else {
			// Match — cf-zlib trees.c:994-1074
			code := uint(lengthCode[lc])
			sendInline(uint64(ltree[code+literals+1].Code), int(ltree[code+literals+1].Len))

			extra := extraLBits[code]
			if extra != 0 {
				lc -= baseLength[code]
				sendInline(uint64(lc), extra)
			}

			dist-- // dist = match distance - 1
			code = dCode(dist)
			sendInline(uint64(dtree[code].Code), int(dtree[code].Len))

			extra = extraDBits[code]
			if extra != 0 {
				dist -= uint(baseDist[code])
				sendInline(uint64(dist), extra)
			}
		}
	}

	// END_BLOCK — cf-zlib trees.c:1082-1098
	sendInline(uint64(ltree[endBlock].Code), int(ltree[endBlock].Len))

	s.biBuf = bitBuf
	s.biValid = filled
}

// detectDataType determines if stream is text or binary.
// cf-zlib trees.c:1117-1143
func detectDataType(s *deflateState) int {
	blockMask := uint32(0xf3ffc07f)
	for n := 0; n <= 31; n++ {
		if blockMask&1 != 0 && s.dynLTree[n].Code != 0 {
			return zBinary
		}
		blockMask >>= 1
	}
	if s.dynLTree[9].Code != 0 || s.dynLTree[10].Code != 0 || s.dynLTree[13].Code != 0 {
		return zText
	}
	for n := 32; n < literals; n++ {
		if s.dynLTree[n].Code != 0 {
			return zText
		}
	}
	return zBinary
}

// Little-endian write helpers.
func putU16LE(b []byte, v uint16) {
	b[0] = byte(v)
	b[1] = byte(v >> 8)
}

func putU64LE(b []byte, v uint64) {
	b[0] = byte(v)
	b[1] = byte(v >> 8)
	b[2] = byte(v >> 16)
	b[3] = byte(v >> 24)
	b[4] = byte(v >> 32)
	b[5] = byte(v >> 40)
	b[6] = byte(v >> 48)
	b[7] = byte(v >> 56)
}
