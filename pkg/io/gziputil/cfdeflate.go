// Package gziputil — cf-zlib deflate.c port.
// Bit-exact pure-Go port of Cloudflare's zlib fork (commit 886098f3) at
// deflate level 6, one-shot gzip mode.  The hash function mirrors
// _mm_crc32_u32(0, *(uint32_t*)str) & hash_mask (x86-64 SSE4.2 CRC32C path),
// implemented via hash/crc32 Castagnoli table — same polynomial, same result.
//
// Scope: single deflate(Z_FINISH) call with all input available.  Only
// deflate_slow (level 6) is ported; streaming flush states beyond Z_FINISH
// are not needed.
package gziputil

import (
	"encoding/binary"
	"hash/crc32"
	"math/bits"
)

// Constants — cf-zlib deflate.h / trees.c
const (
	maxMatch  = 258
	minMatch  = 3 // cf-zlib zutil.h:74: #define MIN_MATCH 3 (used for sym_buf length offset)
	actualMin = 4 // cf-zlib deflate.c:96 ACTUAL_MIN_MATCH

	wBits    = 15         // windowBits for level 6 / memLevel 8
	wSize    = 1 << wBits // 32768
	wMask    = wSize - 1
	hashBits = 8 + 7 // memLevel+7 = 15; cf-zlib deflate.c:250
	hashSize = 1 << hashBits
	hashMask = hashSize - 1

	litBufSize = 1 << (8 + 6) // memLevel+6 = 14 → 16384; cf-zlib deflate.c:260
	// sym_buf starts at pending_buf+lit_bufsize; sym_end = (litBufSize-1)*3
	symEnd = (litBufSize - 1) * 3

	// Huffman tree sizes — cf-zlib deflate.h
	lengthCodes = 29
	literals    = 256
	lCodes      = literals + 1 + lengthCodes // 286
	dCodes      = 30
	blCodes     = 19
	heapSize    = 2*lCodes + 1
	maxBits     = 15
	maxBLBits   = 7
	distCodeLen = 512

	// Block type bits — cf-zlib trees.c
	storedBlock = 0
	staticTrees = 1
	dynTrees    = 2

	endBlock  = 256
	rep36     = 16
	repZ310   = 17
	repZ11138 = 18

	// Data types — cf-zlib zlib.h
	zBinary  = 0
	zText    = 1
	zUnknown = 2

	// Compression parameters for level 6 — cf-zlib deflate.c:119
	goodMatch  = 8
	maxLazy    = 16
	niceLength = 128
	maxChain   = 128

	minLookahead = maxMatch + actualMin + 1
	maxDist      = wSize - minLookahead
)

// staticTreeDesc describes a static tree for a symbol class.
// cf-zlib trees.c:117-123
type staticTreeDesc struct {
	staticTree []ctData
	extraBits  []int
	extraBase  int
	elems      int
	maxLength  int
}

// treeDesc describes a dynamic tree plus its static counterpart.
// cf-zlib deflate.h:75-79
type treeDesc struct {
	dynTree  []ctData
	maxCode  int
	statDesc *staticTreeDesc
}

// deflateState holds all compression state.
// cf-zlib deflate.h:88-258 (adapted for one-shot Go use)
type deflateState struct {
	// sliding window
	// +8: longestMatch's 8-byte XOR loop may read past window_size near the slide boundary;
	// C zlib allocates MAX_MATCH slack beyond window_size (zutil/deflate WIN_INIT) —
	// 8 bytes covers the widest Go read. Latent-only in one-shot scope; see B6 quality review.
	window [wSize*2 + 8]uint8
	prev   [wSize]uint16
	head   [hashSize]uint16

	// match-finding state
	insH        uint32
	strstart    uint32
	matchStart  uint32
	prevMatch   uint32 // cf-zlib deflate.h:149 prev_match
	lookahead   uint32
	prevLength  uint32
	matchLength uint32
	matchAvail  bool
	blockStart  int64
	insert      uint32
	highWater   uint64

	// Huffman trees
	dynLTree [heapSize]ctData
	dynDTree [2*dCodes + 1]ctData
	blTree   [2*blCodes + 1]ctData

	lDesc  treeDesc
	dDesc  treeDesc
	blDesc treeDesc

	blCount [maxBits + 1]uint16
	heap    [2*lCodes + 1]int
	heapLen int
	heapMax int
	depth   [2*lCodes + 1]uint8

	// pending output buffer (lit_bufsize * 4 bytes).
	// sym_buf overlays pending_buf[litBufSize:] — cf-zlib deflate.c:311.
	// After each block is flushed, flushPending copies pending bytes to out
	// and resets pending to 0.
	pendingBuf [litBufSize * 4]uint8
	pending    int // bytes used in pendingBuf[0:pending]

	// symBuf is pendingBuf[litBufSize:] — the symbol accumulation area.
	symNext uint // running byte index into symBuf
	symBuf  []uint8

	// out collects all compressed bytes (grows across blocks)
	out []byte

	optLen    uint64
	staticLen uint64
	matches   uint32

	// bit buffer
	biBuf   uint64
	biValid int

	// compression params
	level    int
	strategy int

	// data type (Z_TEXT/Z_BINARY)
	dataType int

	// window_size
	windowSize uint64
}

// newDeflateState allocates and initialises a deflate_state for level 6 gzip.
// cf-zlib deflate.c:238-322 (deflateInit2_ + lm_init)
func newDeflateState(initialOutCap int) *deflateState {
	s := &deflateState{
		level:      6,
		strategy:   0, // Z_DEFAULT_STRATEGY
		dataType:   zUnknown,
		windowSize: uint64(wSize) * 2,
		blockStart: 0,
		out:        make([]byte, 0, initialOutCap),
	}
	s.symBuf = s.pendingBuf[litBufSize:]
	// Arrays zero-initialised by Go → CLEAR_HASH already done.

	// lm_init — cf-zlib deflate.c:1049-1068
	s.prevLength = actualMin - 1
	s.matchLength = actualMin - 1
	trInit(s)
	return s
}

// flushPending moves pendingBuf[0:pending] to s.out and resets pending.
// Mirrors flush_pending in cf-zlib deflate.c:622-640.
func (s *deflateState) flushPending() {
	biFlush(s)
	if s.pending > 0 {
		s.out = append(s.out, s.pendingBuf[:s.pending]...)
		s.pending = 0
	}
}

// castagnoli is the CRC32C table, used by hashFunc.
var castagnoli = crc32.MakeTable(crc32.Castagnoli)

// hashFunc computes the hash of 4 bytes at window[pos].
// Mirrors cf-zlib deflate.c:143-145 (x86-64 path):
//
//	return _mm_crc32_u32(0, *(uint32_t*)str) & hash_mask;
//
// _mm_crc32_u32(0, val) equals standard CRC32C (Castagnoli) of the 4 bytes
// of val in little-endian order — same as crc32.Checksum/ChecksumCastagnoli.
// Verified: crc32.Update(^uint32(0), castagnoli, bytes) ^ ^uint32(0) matches
// the hardware instruction output.
func hashFunc(window []uint8, pos uint32) uint32 {
	data := window[pos : pos+4]
	return (crc32.Update(^uint32(0), castagnoli, data) ^ ^uint32(0)) & hashMask
}

// insertString inserts window[str..str+3] into the hash table and returns the
// previous head of that chain (i.e. the match_head before str was inserted).
// cf-zlib deflate.c:159-165:
//
//	match_head = s->prev[str & wmask] = s->head[ins_h];
//	s->head[ins_h] = str;
//	return match_head;
func insertString(s *deflateState, str uint32) uint16 {
	h := hashFunc(s.window[:], str)
	s.insH = h
	matchHead := s.head[h] // old head = previous occurrence in this hash chain
	s.prev[str&wMask] = matchHead
	s.head[h] = uint16(str)
	return matchHead
}

// bulkInsertStr inserts count strings starting at startpos.
// cf-zlib deflate.c:167-174
func bulkInsertStr(s *deflateState, startpos uint32, count uint32) {
	for idx := range count {
		h := hashFunc(s.window[:], startpos+idx)
		s.prev[(startpos+idx)&wMask] = s.head[h]
		s.head[h] = uint16(startpos + idx)
	}
}

// longestMatch finds the longest match for the string at s.strstart.
// cf-zlib deflate.c:1134-1234 (x86_64 variant with 64-bit XOR match loop)
func longestMatch(s *deflateState, curMatch uint32) uint32 {
	chainLength := uint32(maxChain)
	scan := s.strstart
	bestLen := int(s.prevLength)
	niceMatch := niceLength
	limit := uint32(0)
	if s.strstart > maxDist {
		limit = s.strstart - maxDist
	}

	if uint32(niceMatch) > s.lookahead {
		niceMatch = int(s.lookahead)
	}

	strend := scan + maxMatch
	// We optimise for ACTUAL_MIN_MATCH=4: use uint32 for initial check
	scanStart := binary.LittleEndian.Uint32(s.window[scan:])
	// scan_end: uint32 at scan+best_len-3  — cf-zlib deflate.c:1153
	scanEnd := binary.LittleEndian.Uint32(s.window[scan+uint32(bestLen)-3:])

	if s.prevLength >= goodMatch {
		chainLength >>= 2
	}

	for {
		// Inner loop: find candidate with matching head 4 bytes
		// cf-zlib deflate.c:1183-1194
		cont := true
		for {
			match := curMatch
			// check scan_end and scan_start simultaneously — cf-zlib deflate.c:1186
			mEnd := binary.LittleEndian.Uint32(s.window[match+uint32(bestLen)-3:])
			mStart := binary.LittleEndian.Uint32(s.window[match:])
			if mEnd != scanEnd || mStart != scanStart {
				var ok bool
				curMatch, ok = nextChain(s, curMatch, limit, &chainLength)
				if !ok {
					cont = false
					break
				}
				continue
			}
			break
		}
		if !cont {
			break
		}

		// Compare remaining bytes using 8-byte XOR loop — cf-zlib deflate.c:1199-1213
		scanOff := scan + 4
		matchOff := curMatch + 4

		for scanOff < strend {
			sv := binary.LittleEndian.Uint64(s.window[scanOff:])
			mv := binary.LittleEndian.Uint64(s.window[matchOff:])
			x := sv ^ mv
			if x != 0 {
				// count trailing zero bytes — cf-zlib deflate.c:1205
				matchByte := bits.TrailingZeros64(x) / 8
				scanOff += uint32(matchByte)
				matchOff += uint32(matchByte)
				break
			}
			scanOff += 8
			matchOff += 8
		}
		if scanOff > strend {
			scanOff = strend
		}

		ln := int(maxMatch) - int(strend-scanOff)
		if ln > bestLen {
			s.matchStart = curMatch
			bestLen = ln
			if ln >= niceMatch {
				break
			}
			scanEnd = binary.LittleEndian.Uint32(s.window[scan+uint32(bestLen)-3:])
		}

		// Advance chain — cf-zlib deflate.c:1229
		var ok bool
		curMatch, ok = nextChain(s, curMatch, limit, &chainLength)
		if !ok {
			break
		}
	}

	if uint32(bestLen) <= s.lookahead {
		return uint32(bestLen)
	}
	return s.lookahead
}

// nextChain advances cur_match along the prev chain.
func nextChain(s *deflateState, curMatch, limit uint32, chainLength *uint32) (uint32, bool) {
	next := uint32(s.prev[curMatch&wMask])
	*chainLength--
	if next > limit && *chainLength != 0 {
		return next, true
	}
	return 0, false
}

// fillWindow fills the lookahead window from src.
// cf-zlib deflate.c:1274-1430 — adapted for one-shot: src provides the full input,
// tracked by srcOff/srcLen which are attached to the state.
func fillWindow(s *deflateState, src []byte, srcOff *int) {
	wsize := uint32(wSize)

	for {
		more := uint32(s.windowSize) - s.lookahead - s.strstart

		// Slide window if needed — cf-zlib deflate.c:1303-1354
		if s.strstart >= wsize+maxDist {
			copy(s.window[:], s.window[wsize:wsize+wsize])
			if s.matchStart >= wsize {
				s.matchStart -= wsize
			} else {
				s.matchStart = 0
			}
			s.strstart -= wsize
			s.blockStart -= int64(wsize)

			// Slide head — cf-zlib deflate.c x86_64 _mm_subs_epu16 path
			// (saturating subtract by wsize, equivalent to: if v >= wsize { v -= wsize } else { v = 0 })
			for i := range hashSize {
				v := s.head[i]
				if v >= uint16(wsize) {
					s.head[i] = v - uint16(wsize)
				} else {
					s.head[i] = 0
				}
			}
			for i := range wSize {
				v := s.prev[i]
				if v >= uint16(wsize) {
					s.prev[i] = v - uint16(wsize)
				} else {
					s.prev[i] = 0
				}
			}
			more += wsize
		}

		if *srcOff >= len(src) {
			break
		}

		// Read input — cf-zlib deflate.c:1370-1371
		avail := min(len(src)-*srcOff, int(more))
		copy(s.window[s.strstart+s.lookahead:], src[*srcOff:*srcOff+avail])
		*srcOff += avail
		s.lookahead += uint32(avail)

		// Update hash for pending inserts — cf-zlib deflate.c:1374-1387
		if s.lookahead+s.insert >= actualMin {
			str := s.strstart - s.insert
			insH := uint32(s.window[str]) // cf-zlib: ins_h = s->window[str]
			for s.insert > 0 {
				insH = hashFunc(s.window[:], str)
				s.prev[str&wMask] = s.head[insH]
				s.head[insH] = uint16(str)
				str++
				s.insert--
				if s.lookahead+s.insert < actualMin {
					break
				}
			}
			s.insH = insH
		}

		if s.lookahead >= minLookahead {
			break
		}
	}

	// Zero WIN_INIT bytes past end for valgrind-clean reads — cf-zlib deflate.c:1401-1426
	curr := uint64(s.strstart) + uint64(s.lookahead)
	if s.highWater < curr {
		initN := min(s.windowSize-curr, maxMatch)
		for i := uint64(0); i < initN; i++ {
			s.window[curr+i] = 0
		}
		s.highWater = curr + initN
	} else if s.highWater < curr+maxMatch {
		initN := min(curr+maxMatch-s.highWater, s.windowSize-s.highWater)
		for i := uint64(0); i < initN; i++ {
			s.window[s.highWater+i] = 0
		}
		s.highWater += initN
	}
}

// flushBlock flushes the current block and moves output to s.out.
// Mirrors FLUSH_BLOCK_ONLY + flush_pending — cf-zlib deflate.c:1436-1445
func flushBlock(s *deflateState, last bool) {
	var buf []byte
	if s.blockStart >= 0 {
		buf = s.window[s.blockStart:s.strstart]
	}
	storedLen := int(int64(s.strstart) - s.blockStart)
	trFlushBlock(s, buf, storedLen, last)
	s.blockStart = int64(s.strstart)
	s.flushPending()
}

// trTallyLit records a literal symbol.
// cf-zlib deflate.h:309-316 (macro _tr_tally_lit)
func trTallyLit(s *deflateState, c uint8) bool {
	s.symBuf[s.symNext] = 0
	s.symBuf[s.symNext+1] = 0
	s.symBuf[s.symNext+2] = c
	s.symNext += 3
	s.dynLTree[c].Code++ // freq++
	return s.symNext == symEnd
}

// trTallyDist records a distance/length match.
// cf-zlib deflate.h:317-327 (macro _tr_tally_dist)
func trTallyDist(s *deflateState, distance, length uint32) bool {
	ln := uint8(length)
	dist := uint16(distance)
	s.symBuf[s.symNext] = uint8(dist)
	s.symBuf[s.symNext+1] = uint8(dist >> 8)
	s.symBuf[s.symNext+2] = ln
	s.symNext += 3
	dist--
	s.dynLTree[int(lengthCode[ln])+literals+1].Code++ // freq++
	s.dynDTree[dCode(uint(dist))].Code++              // freq++
	return s.symNext == symEnd
}

// deflateSlow implements the lazy-evaluation compressor for level 6.
// One-shot variant: flush is always Z_FINISH so we never return need_more.
// cf-zlib deflate.c:1614-1734
func deflateSlow(s *deflateState, src []byte, srcOff *int) {
	for {
		if s.lookahead < minLookahead {
			fillWindow(s, src, srcOff)
			// With Z_FINISH: return need_more only when lookahead==0.
			// cf-zlib deflate.c:1626-1630
			if s.lookahead == 0 {
				break // flush the current block
			}
		}

		// Insert current string and get hash head — cf-zlib deflate.c:1636-1638
		hashHead := uint32(0)
		if s.lookahead >= actualMin {
			hashHead = uint32(insertString(s, s.strstart))
		}

		s.prevLength = s.matchLength
		s.prevMatch = s.matchStart // prevMatch = matchStart before this step
		s.matchLength = actualMin - 1

		// Try to find a longer match — cf-zlib deflate.c:1646-1662
		if hashHead != 0 && s.prevLength < maxLazy &&
			s.strstart-hashHead <= maxDist {
			s.matchLength = longestMatch(s, hashHead)

			// Z_FILTERED shortening — cf-zlib deflate.c:1655-1661 (strategy != Z_FILTERED here, skip)
		}

		// Accept previous match if current is not better — cf-zlib deflate.c:1666-1696
		if s.prevLength >= actualMin && s.matchLength <= s.prevLength {
			maxInsert := s.strstart + s.lookahead - actualMin

			var bflush bool
			// cf-zlib deflate.c:1675-1676: _tr_tally_dist(s, dist, prev_length - MIN_MATCH, bflush)
			// sym_buf stores length - MIN_MATCH (not the actual length).
			bflush = trTallyDist(s, s.strstart-1-s.prevMatch, s.prevLength-minMatch)

			s.lookahead -= s.prevLength - 1
			movFwd := s.prevLength - 2
			insertCnt := movFwd
			if insertCnt > maxInsert-s.strstart {
				if maxInsert > s.strstart {
					insertCnt = maxInsert - s.strstart
				} else {
					insertCnt = 0
				}
			}
			bulkInsertStr(s, s.strstart+1, insertCnt)
			s.prevLength = 0
			s.matchAvail = false
			s.matchLength = actualMin - 1
			s.strstart += movFwd + 1

			if bflush {
				flushBlock(s, false)
			}
		} else if s.matchAvail {
			// Emit literal for previous position — cf-zlib deflate.c:1698-1710
			bflush := trTallyLit(s, s.window[s.strstart-1])
			if bflush {
				flushBlock(s, false)
			}
			s.strstart++
			s.lookahead--
		} else {
			// No previous match — wait for next step — cf-zlib deflate.c:1711-1718
			s.matchAvail = true
			s.strstart++
			s.lookahead--
		}
	}

	// Flush remaining — cf-zlib deflate.c:1721-1733
	if s.matchAvail {
		trTallyLit(s, s.window[s.strstart-1])
		s.matchAvail = false
	}
	s.insert = min(s.strstart, actualMin-1)
	flushBlock(s, true)
}

// CompressCFGz compresses src[off:off+length] using the cf-zlib deflate engine
// at level 6 and wraps with a gzip header/trailer (OS byte = 0).
// This is the bit-exact replacement for the stdlib-based CompressGz.
//
// Gzip header format matches cf-zlib deflate.c:667-681 + TS GZip.ts OS zeroing.
func CompressCFGz(src []byte, off, length int) []byte {
	input := src[off : off+length]

	// Conservative output capacity (gzip bound)
	outCap := 18 + length + (length >> 12) + (length >> 14) + (length >> 25) + 13 + 100
	s := newDeflateState(outCap)

	// --- gzip header (10 bytes) --- cf-zlib deflate.c:667-681
	// ID1=0x1f ID2=0x8b CM=8 FLG=0 MTIME=0 XFL=0 OS=0
	s.out = append(s.out,
		0x1f, 0x8b, // magic
		8,          // CM = Z_DEFLATED
		0,          // FLG = 0 (no extra, name, comment, hcrc)
		0, 0, 0, 0, // MTIME = 0
		0, // XFL = 0 (level 6; cf-zlib deflate.c:676: level9→2, level<2→4, else 0)
		0, // OS = 0  (cf-zlib writes OS_CODE=3 on Linux, zeroed per TS GZip.ts)
	)

	// Run compression.  fillWindow does the initial read from input.
	srcOff := 0
	fillWindow(s, input, &srcOff)

	// Detect data type — cf-zlib deflate.c:846-847
	if s.dataType == zUnknown {
		s.dataType = detectDataType(s)
	}

	// deflateSlow processes all input and calls flushBlock (which calls flushPending)
	// after each block, including the final block with last=true.
	deflateSlow(s, input, &srcOff)

	if debugAfterDeflate != nil {
		debugAfterDeflate(s.symNext, s.optLen, s.staticLen)
	}

	// Any remaining bits in biBuf were already flushed by biWindup inside
	// trFlushBlock → last=true path.  But biFlush in flushPending may have
	// missed the last few bits if biWindup wrote them.  flushPending is
	// already called at end of deflateSlow via flushBlock(last=true).

	// --- gzip trailer: CRC32 (IEEE/zlib) + ISIZE ---
	// cf-zlib deflate.c:924-933: wrap==2 writes crc32 LE + total_in LE
	crcVal := crc32.ChecksumIEEE(input)
	s.out = append(s.out,
		byte(crcVal),
		byte(crcVal>>8),
		byte(crcVal>>16),
		byte(crcVal>>24),
		byte(length),
		byte(length>>8),
		byte(length>>16),
		byte(length>>24),
	)
	return s.out
}

// debugAfterDeflate is set by tests to inspect state after deflation.
var debugAfterDeflate func(symNext uint, optLen, staticLen uint64)
