// Copyright 2015, Joe Tsai. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE.md file.

package bzip2

import "github.com/zsrv/goscape/pkg/io/bzip2/internal/dserrors"

// rleDone is a special "error" to indicate that the RLE stage is done.
var rleDone = errorf(dserrors.Unknown, "RLE1 stage is completed")

// runLengthEncoding implements the first RLE stage of bzip2. Every sequence
// of 4..255 duplicated bytes is replaced by only the first 4 bytes, and a
// single byte representing the repeat length. Similar to the C bzip2
// implementation, the encoder will always terminate repeat sequences with a
// count (even if it is the end of the buffer), and it will also never produce
// run lengths of 256..259. The decoder can handle the latter case.
//
// For example, if the input was:
//	input:  "AAAAAAABBBBCCCD"
//
// Then the output will be:
//	output: "AAAA\x03BBBB\x00CCCD"
//
// goscape: rewritten 2026-05-17 to mirror libbzip2's ADD_CHAR_TO_BLOCK +
// ADD_PAIR_TO_BLOCK semantics — the pending run lives in (lastVal, lastCnt)
// and is emitted to buf only when it ends (different char arrives, count
// hits 255, or EmitPending is called for end-of-stream). Critically the
// state PERSISTS across block boundaries so a run that straddles two blocks
// is recorded as a single record in the second block (matching libbzip2's
// state_in_ch / state_in_len fields, bzlib.c:130-148, 260-283). The earlier
// dsnet impl emitted bytes eagerly and reset state at every block flush,
// producing different boundary content and ~6-20 B drift on multi-block
// inputs versus libbzip2 -1.
type runLengthEncoding struct {
	buf     []byte // physical buffer, sized with overhead for one pair-flush past maxIdx
	maxIdx  int    // soft cap; analog of libbzip2 nblockMAX (bzlib.c:194)
	idx     int
	lastVal byte
	lastCnt int // 0 ⇒ no pending run; otherwise lastVal × lastCnt is held in state
}

func (rle *runLengthEncoding) Init(buf []byte, maxIdx int) {
	*rle = runLengthEncoding{buf: buf, maxIdx: maxIdx}
}

// ResetBuf clears the output buffer but preserves the pending RLE state
// (lastVal, lastCnt). Used at block-flush boundaries so a pending run
// carries over to the next block — see libbzip2 bzlib.c:386-389 where
// BZ2_compressBlock is called without flush_RL when only nblockMAX is hit.
func (rle *runLengthEncoding) ResetBuf() {
	rle.idx = 0
}

// EmitPending writes the pending run (lastVal × lastCnt) to buf and updates
// c with the lastCnt pre-RLE bytes that just became part of the current
// block — mirroring libbzip2 add_pair_to_block's BZ_UPDATE_CRC loop
// (bzlib.c:219-222). buf is sized so a single pair-flush (≤5 bytes) past
// maxIdx always fits; physical-overflow is therefore unreachable when
// Write enforces the maxIdx soft cap.
func (rle *runLengthEncoding) EmitPending(c *crc) {
	if rle.lastCnt == 0 {
		return
	}
	n := rle.lastCnt
	if c != nil {
		var tmp [256]byte
		for i := 0; i < n; i++ {
			tmp[i] = rle.lastVal
		}
		c.update(tmp[:n])
	}
	if n < 4 {
		for k := 0; k < n; k++ {
			rle.buf[rle.idx] = rle.lastVal
			rle.idx++
		}
	} else {
		for k := 0; k < 4; k++ {
			rle.buf[rle.idx] = rle.lastVal
			rle.idx++
		}
		rle.buf[rle.idx] = byte(n - 4)
		rle.idx++
	}
	rle.lastCnt = 0
}

// Write feeds input bytes through the RLE encoder. Completed runs are
// flushed to buf via EmitPending; the in-progress run is held in state.
// c receives CRC updates indirectly via EmitPending so that per-block
// CRCs match libbzip2 byte-for-byte. Returns (consumed, rleDone) when
// the buf has filled past maxIdx — matching libbzip2's "nblock >= nblockMAX"
// check (bzlib.c:298,314) which lets the final pair-flush of a block push
// nblock up to +4 past nblockMAX.
func (rle *runLengthEncoding) Write(buf []byte, c *crc) (int, error) {
	for i, b := range buf {
		if rle.idx >= rle.maxIdx {
			return i, rleDone
		}
		if rle.lastCnt > 0 && b == rle.lastVal && rle.lastCnt < 255 {
			rle.lastCnt++
			continue
		}
		rle.EmitPending(c)
		rle.lastVal = b
		rle.lastCnt = 1
	}
	return len(buf), nil
}

func (rle *runLengthEncoding) Read(buf []byte) (int, error) {
	for i := range buf {
		switch {
		case rle.lastCnt == -4:
			if rle.idx >= len(rle.buf) {
				return i, errorf(dserrors.Corrupted, "missing terminating run-length repeater")
			}
			rle.lastCnt = int(rle.buf[rle.idx])
			rle.idx++
			if rle.lastCnt > 0 {
				break // Break the switch
			}
			fallthrough // Count was zero, continue the work
		case rle.lastCnt <= 0:
			if rle.idx >= len(rle.buf) {
				return i, rleDone
			}
			b := rle.buf[rle.idx]
			rle.idx++
			if b != rle.lastVal {
				rle.lastCnt = 0
				rle.lastVal = b
			}
		}
		buf[i] = rle.lastVal
		rle.lastCnt--
	}
	return len(buf), nil
}

func (rle *runLengthEncoding) Bytes() []byte { return rle.buf[:rle.idx] }
