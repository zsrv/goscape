// Package gziputil — stock zlib 1.3.1 deflate.c port.
//
// Bit-exact pure-Go port of stock zlib 1.3.1 (madler/zlib tag v1.3.1) at
// deflate level 6, one-shot gzip mode.  This is the rev-274 path: the r274
// cache was produced by stock zlib 1.3.1 level 6 (gzip OS byte zeroed), NOT
// the Cloudflare fork that cfdeflate.go ports.  python zlib 1.3.1 level-6
// reproduces the original r274 cache gzip members byte-for-byte.
//
// Differences from the Cloudflare port (cfdeflate.go), all confined to the
// match-finder half (deflate.c):
//   - Hash: stock uses the classic rolling multiply-shift UPDATE_HASH
//     (h = ((h << hash_shift) ^ c) & hash_mask) over MIN_MATCH(3)-byte windows,
//     inserting on window[str + MIN_MATCH-1].  Cloudflare uses a fresh CRC32C
//     of 4 bytes (ACTUAL_MIN_MATCH=4).
//   - MIN_MATCH = 3 (Cloudflare uses ACTUAL_MIN_MATCH = 4).
//   - longest_match: stock byte-at-a-time scan (deflate.c default variant).
//   - deflate_slow: the TOO_FAR (4096) check that discards length-3 matches at
//     distance > 4096 — stock HAS it; the Cloudflare port skipped it.
//   - fill_window: rolling-hash re-seed + insert-clamp after a window slide.
//
// The Huffman/trees half (cftrees.go) is byte-output-identical between stock
// zlib 1.3.1 trees.c and the Cloudflare fork — verified function-by-function
// (tr_static_init, build_tree, gen_bitlen, scan_tree, send_tree, compress_block,
// _tr_flush_block, send_bits/bi_windup, sym_buf 3-byte layout, lit_bufsize=16384,
// sym_end=(lit_bufsize-1)*3).  It is REUSED as-is.
//
// Scope: single deflate(Z_FINISH) call with all input available.  Only
// deflate_slow (level 6) is ported; streaming flush states beyond Z_FINISH
// are not needed.
package gziputil

import "hash/crc32"

// Stock-zlib constants — deflate.h / zutil.h (v1.3.1).
const (
	szMinMatch     = 3                                        // zutil.h:84
	szHashShift    = (hashBits + szMinMatch - 1) / szMinMatch // (15+2)/3 = 5; deflate.c:447
	szMinLookahead = maxMatch + szMinMatch + 1                // 258+3+1 = 262; deflate.h:290
	szMaxDist      = wSize - szMinLookahead                   // deflate.h:295
	szTooFar       = 4096                                     // deflate.c: TOO_FAR
)

// szUpdateHash applies the rolling multiply-shift hash for one byte.
// deflate.c:141  #define UPDATE_HASH(s,h,c) (h = (((h)<<s->hash_shift)^(c)) & s->hash_mask)
func szUpdateHash(h uint32, c uint8) uint32 {
	return ((h << szHashShift) ^ uint32(c)) & hashMask
}

// szInsertString inserts window[str .. str+MIN_MATCH-1] into the hash table
// using the rolling ins_h and returns the previous head of that chain.
// deflate.c:160 (non-FASTEST INSERT_STRING):
//
//	UPDATE_HASH(s, s->ins_h, s->window[(str)+(MIN_MATCH-1)]),
//	match_head = s->prev[(str)&s->w_mask] = s->head[s->ins_h],
//	s->head[s->ins_h] = (Pos)(str)
//
// IN assertion (deflate.c:150): all calls are made with consecutive input —
// i.e. ins_h already reflects window[str] and window[str+1].
func szInsertString(s *deflateState, str uint32) uint16 {
	s.insH = szUpdateHash(s.insH, s.window[str+szMinMatch-1])
	matchHead := s.head[s.insH]
	s.prev[str&wMask] = matchHead
	s.head[s.insH] = uint16(str)
	return matchHead
}

// szLongestMatch finds the longest match for the string at s.strstart.
// deflate.c default longest_match (non-FASTEST, byte-at-a-time scan).
//
// The byte-at-a-time scan and the UNALIGNED_OK 2-byte scan are
// match-equivalent: both compute the same best_len / match_start, so the
// emitted deflate stream is identical.  We port the byte-at-a-time variant.
func szLongestMatch(s *deflateState, curMatch uint32) uint32 {
	chainLength := uint32(maxChain)
	scan := s.strstart
	bestLen := int(s.prevLength)
	niceMatch := niceLength
	limit := uint32(0)
	if s.strstart > szMaxDist {
		limit = s.strstart - szMaxDist
	}

	// strend = window + strstart + MAX_MATCH (deflate.c, non-UNALIGNED_OK).
	strend := scan + maxMatch
	scanEnd1 := s.window[scan+uint32(bestLen)-1]
	scanEnd := s.window[scan+uint32(bestLen)]

	// Do not waste too much time if we already have a good match.
	if s.prevLength >= goodMatch {
		chainLength >>= 2
	}
	// Do not look beyond the end of the input (determinism).
	if uint32(niceMatch) > s.lookahead {
		niceMatch = int(s.lookahead)
	}

	for {
		match := curMatch

		// Skip if the match cannot improve on best_len.
		// deflate.c:
		//   if (match[best_len]   != scan_end  ||
		//       match[best_len-1] != scan_end1 ||
		//       *match            != *scan     ||
		//       *++match          != scan[1])  continue;
		if s.window[match+uint32(bestLen)] != scanEnd ||
			s.window[match+uint32(bestLen)-1] != scanEnd1 ||
			s.window[match] != s.window[scan] ||
			s.window[match+1] != s.window[scan+1] {
			next := uint32(s.prev[curMatch&wMask])
			chainLength--
			if next > limit && chainLength != 0 {
				curMatch = next
				continue
			}
			break
		}

		// scan += 2, match++ (we already advanced match conceptually by 1
		// in the *++match check above, so the comparison cursor starts at
		// scan+2 vs match+2).  Scan forward 8 bytes at a time.
		sp := scan + 2
		mp := match + 2
		// do { } while (*++scan == *++match && ... ×8 && scan < strend);
		for {
			ok := true
			for range 8 {
				sp++
				mp++
				if s.window[sp] != s.window[mp] {
					ok = false
					break
				}
			}
			if !ok || sp >= strend {
				break
			}
		}

		// len = MAX_MATCH - (strend - scan)
		ln := int(maxMatch) - int(strend-sp)

		if ln > bestLen {
			s.matchStart = curMatch
			bestLen = ln
			if ln >= niceMatch {
				break
			}
			scanEnd1 = s.window[scan+uint32(bestLen)-1]
			scanEnd = s.window[scan+uint32(bestLen)]
		}

		next := uint32(s.prev[curMatch&wMask])
		chainLength--
		if next > limit && chainLength != 0 {
			curMatch = next
			continue
		}
		break
	}

	if uint32(bestLen) <= s.lookahead {
		return uint32(bestLen)
	}
	return s.lookahead
}

// szFillWindow fills the lookahead window from src (one-shot variant).
// deflate.c fill_window — with the stock rolling-hash re-seed and the
// `if (insert > strstart) insert = strstart` clamp after a window slide.
func szFillWindow(s *deflateState, src []byte, srcOff *int) {
	wsize := uint32(wSize)

	for {
		more := uint32(s.windowSize) - s.lookahead - s.strstart

		// Slide window if almost full and lookahead insufficient.
		if s.strstart >= wsize+szMaxDist {
			copy(s.window[:], s.window[wsize:wsize+wsize])
			s.matchStart -= wsize
			s.strstart -= wsize
			s.blockStart -= int64(wsize)
			if s.insert > s.strstart {
				s.insert = s.strstart
			}
			// slide_hash: plain subtract, m >= wsize ? m-wsize : 0.
			for i := range hashSize {
				m := s.head[i]
				if m >= uint16(wsize) {
					s.head[i] = m - uint16(wsize)
				} else {
					s.head[i] = 0
				}
			}
			for i := range wSize {
				m := s.prev[i]
				if m >= uint16(wsize) {
					s.prev[i] = m - uint16(wsize)
				} else {
					s.prev[i] = 0
				}
			}
			more += wsize
		}

		if *srcOff >= len(src) {
			break
		}

		// read_buf
		avail := len(src) - *srcOff
		if avail > int(more) {
			avail = int(more)
		}
		copy(s.window[s.strstart+s.lookahead:], src[*srcOff:*srcOff+avail])
		*srcOff += avail
		n := uint32(avail)
		s.lookahead += n

		// Initialize/extend the rolling hash now that we have input.
		// deflate.c:307-322
		if s.lookahead+s.insert >= szMinMatch {
			str := s.strstart - s.insert
			s.insH = uint32(s.window[str])
			s.insH = szUpdateHash(s.insH, s.window[str+1])
			// MIN_MATCH == 3, so no extra UPDATE_HASH calls here.
			for s.insert > 0 {
				s.insH = szUpdateHash(s.insH, s.window[str+szMinMatch-1])
				s.prev[str&wMask] = s.head[s.insH]
				s.head[s.insH] = uint16(str)
				str++
				s.insert--
				if s.lookahead+s.insert < szMinMatch {
					break
				}
			}
		}

		if s.lookahead >= szMinLookahead || *srcOff >= len(src) {
			break
		}
	}

	// Zero WIN_INIT (=MAX_MATCH) bytes past end so longest_match never reads
	// uninitialised window memory.  deflate.c:339-365
	if s.highWater < s.windowSize {
		curr := uint64(s.strstart) + uint64(s.lookahead)
		if s.highWater < curr {
			initN := s.windowSize - curr
			if initN > maxMatch {
				initN = maxMatch
			}
			for i := uint64(0); i < initN; i++ {
				s.window[curr+i] = 0
			}
			s.highWater = curr + initN
		} else if s.highWater < curr+maxMatch {
			initN := curr + maxMatch - s.highWater
			if initN > s.windowSize-s.highWater {
				initN = s.windowSize - s.highWater
			}
			for i := uint64(0); i < initN; i++ {
				s.window[s.highWater+i] = 0
			}
			s.highWater += initN
		}
	}
}

// szDeflateSlow is the lazy-evaluation compressor for level 6 (stock zlib).
// One-shot variant: flush is always Z_FINISH so we never return need_more.
// deflate.c deflate_slow.
func szDeflateSlow(s *deflateState, src []byte, srcOff *int) {
	for {
		if s.lookahead < szMinLookahead {
			szFillWindow(s, src, srcOff)
			if s.lookahead == 0 {
				break // flush the current block (Z_FINISH)
			}
		}

		// Insert the current string and get the head of its hash chain.
		hashHead := uint32(0)
		if s.lookahead >= szMinMatch {
			hashHead = uint32(szInsertString(s, s.strstart))
		}

		s.prevLength = s.matchLength
		s.prevMatch = s.matchStart
		s.matchLength = szMinMatch - 1

		if hashHead != 0 && s.prevLength < maxLazy &&
			s.strstart-hashHead <= szMaxDist {
			s.matchLength = szLongestMatch(s, hashHead)

			// TOO_FAR: discard a length-3 match at distance > 4096.
			// deflate.c (Z_DEFAULT_STRATEGY, TOO_FAR <= 32767):
			//   if (match_length <= 5 &&
			//       (match_length == MIN_MATCH &&
			//        strstart - match_start > TOO_FAR))
			//       match_length = MIN_MATCH-1;
			if s.matchLength <= 5 &&
				s.matchLength == szMinMatch &&
				s.strstart-s.matchStart > szTooFar {
				s.matchLength = szMinMatch - 1
			}
		}

		// Output the previous match if the current one is not better.
		if s.prevLength >= szMinMatch && s.matchLength <= s.prevLength {
			maxInsert := s.strstart + s.lookahead - szMinMatch

			bflush := trTallyDist(s, s.strstart-1-s.prevMatch, s.prevLength-szMinMatch)

			// Insert all strings up to the end of the match.
			//   lookahead -= prev_length - 1;
			//   prev_length -= 2;
			//   do { if (++strstart <= max_insert) INSERT_STRING(strstart); }
			//   while (--prev_length != 0);
			//   strstart++;
			s.lookahead -= s.prevLength - 1
			cnt := s.prevLength - 2
			for {
				s.strstart++
				if s.strstart <= maxInsert {
					szInsertString(s, s.strstart)
				}
				cnt--
				if cnt == 0 {
					break
				}
			}
			s.matchAvail = false
			s.matchLength = szMinMatch - 1
			s.strstart++

			if bflush {
				flushBlock(s, false)
			}
		} else if s.matchAvail {
			// Output a single literal for the previous position.
			bflush := trTallyLit(s, s.window[s.strstart-1])
			if bflush {
				flushBlock(s, false)
			}
			s.strstart++
			s.lookahead--
		} else {
			// No previous match; wait one step.
			s.matchAvail = true
			s.strstart++
			s.lookahead--
		}
	}

	// Flush remaining.  deflate.c:
	if s.matchAvail {
		trTallyLit(s, s.window[s.strstart-1])
		s.matchAvail = false
	}
	if s.strstart < szMinMatch-1 {
		s.insert = s.strstart
	} else {
		s.insert = szMinMatch - 1
	}
	flushBlock(s, true)
}

// newDeflateStateSZ allocates a deflate_state for stock-zlib level 6 gzip.
// deflate.c deflateInit2_ + lm_init (level 6).
func newDeflateStateSZ(initialOutCap int) *deflateState {
	s := &deflateState{
		level:      6,
		strategy:   0, // Z_DEFAULT_STRATEGY
		dataType:   zUnknown,
		windowSize: uint64(wSize) * 2,
		blockStart: 0,
		out:        make([]byte, 0, initialOutCap),
	}
	s.symBuf = s.pendingBuf[litBufSize:]
	// Arrays zero-initialised by Go → CLEAR_HASH already done; ins_h = 0.

	// lm_init: match_length = prev_length = MIN_MATCH-1.
	s.prevLength = szMinMatch - 1
	s.matchLength = szMinMatch - 1
	trInit(s)
	return s
}

// CompressSZGz compresses src[off:off+length] using the stock zlib 1.3.1
// deflate engine at level 6 and wraps it with a gzip header/trailer (OS=0).
// Output is byte-identical to python/C zlib 1.3.1 gzip at level 6 with the
// OS byte zeroed (the r274 cache convention).
func CompressSZGz(src []byte, off, length int) []byte {
	input := src[off : off+length]

	outCap := 18 + length + (length >> 12) + (length >> 14) + (length >> 25) + 13 + 100
	s := newDeflateStateSZ(outCap)

	// gzip header (10 bytes): OS zeroed per GZip.ts convention.
	s.out = append(s.out,
		0x1f, 0x8b, // magic
		8,          // CM = Z_DEFLATED
		0,          // FLG = 0
		0, 0, 0, 0, // MTIME = 0
		0, // XFL = 0 (level 6)
		0, // OS = 0
	)

	srcOff := 0
	szFillWindow(s, input, &srcOff)

	if s.dataType == zUnknown {
		s.dataType = detectDataType(s)
	}

	szDeflateSlow(s, input, &srcOff)

	if debugAfterDeflate != nil {
		debugAfterDeflate(s.symNext, s.optLen, s.staticLen)
	}

	// gzip trailer: CRC32 (IEEE) LE + ISIZE LE.
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
