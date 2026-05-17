// Copyright 2015, Joe Tsai. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE.md file.

package bzip2

import (
	"io"
	"math"

	"github.com/zsrv/goscape/pkg/io/bzip2/internal/dscommon"
	"github.com/zsrv/goscape/pkg/io/bzip2/internal/dserrors"
	"github.com/zsrv/goscape/pkg/io/bzip2/internal/prefix"
)

type Writer struct {
	InputOffset  int64 // Total number of bytes issued to Write
	OutputOffset int64 // Total number of bytes written to underlying io.Writer

	wr     prefixWriter
	err    error
	level  int    // The current compression level
	wrHdr  bool   // Have we written the stream header?
	blkCRC uint32 // CRC-32 IEEE of each block
	endCRC uint32 // Checksum of all blocks using bzip2's custom method

	crc crc
	rle runLengthEncoding
	bwt burrowsWheelerTransform
	mtf moveToFront

	// These fields are allocated with Writer and re-used later.
	buf         []byte
	treeSels    []uint8
	treeSelsMTF []uint8
	codes2D     [maxNumTrees][maxNumSyms]prefix.PrefixCode
	codes1D     [maxNumTrees]prefix.PrefixCodes
	trees1D     [maxNumTrees]prefix.Encoder
}

type WriterConfig struct {
	Level int

	_ struct{} // Blank field to prevent unkeyed struct literals
}

func NewWriter(w io.Writer, conf *WriterConfig) (*Writer, error) {
	var lvl int
	if conf != nil {
		lvl = conf.Level
	}
	if lvl == 0 {
		lvl = DefaultCompression
	}
	if lvl < BestSpeed || lvl > BestCompression {
		return nil, errorf(dserrors.Invalid, "compression level: %d", lvl)
	}
	zw := new(Writer)
	zw.level = lvl
	zw.Reset(w)
	return zw, nil
}

func (zw *Writer) Reset(w io.Writer) error {
	*zw = Writer{
		wr:    zw.wr,
		level: zw.level,

		rle: zw.rle,
		bwt: zw.bwt,
		mtf: zw.mtf,

		buf:         zw.buf,
		treeSels:    zw.treeSels,
		treeSelsMTF: zw.treeSelsMTF,
		trees1D:     zw.trees1D,
	}
	zw.wr.Init(w)
	// Block sizing mirrors libbzip2 bzlib.c:194 — `nblockMAX = 100000 *
	// blockSize100k - 19`. The soft cap (maxIdx) stops feeding chars when
	// idx ≥ nblockMAX; the physical buf is sized to nblockMAX + 5 so a
	// final pair-flush (up to 5 bytes) past the cap always fits, matching
	// libbzip2's behavior where nblock may grow to nblockMAX+4 (see
	// ADD_PAIR_TO_BLOCK and bzlib.c:298,314 break-after-overflow).
	nblockMAX := zw.level*blockSize - 19
	// Physical = nblockMAX + 10 covers worst-case Write end-state
	// (idx ≤ nblockMAX+4 after a final pair-flush past the cap) plus one
	// more 5-byte pair-flush from Close's flush_RL-equivalent.
	physical := nblockMAX + 10
	if len(zw.buf) != physical {
		zw.buf = make([]byte, physical)
	}
	zw.rle.Init(zw.buf, nblockMAX)
	return nil
}

func (zw *Writer) Write(buf []byte) (int, error) {
	if zw.err != nil {
		return 0, zw.err
	}

	cnt := len(buf)
	for {
		wrCnt, err := zw.rle.Write(buf, &zw.crc)
		if err != rleDone && zw.err == nil {
			zw.err = err
		}
		buf = buf[wrCnt:]
		if len(buf) == 0 {
			zw.InputOffset += int64(cnt)
			return cnt, nil
		}
		if zw.err = zw.flush(); zw.err != nil {
			return 0, zw.err
		}
	}
}

func (zw *Writer) flush() error {
	vals := zw.rle.Bytes()
	if len(vals) == 0 {
		return nil
	}
	zw.wr.Offset = zw.OutputOffset
	func() {
		defer dserrors.Recover(&zw.err)
		if !zw.wrHdr {
			// Write stream header.
			zw.wr.WriteBitsBE64(hdrMagic, 16)
			zw.wr.WriteBitsBE64('h', 8)
			zw.wr.WriteBitsBE64(uint64('0'+zw.level), 8)
			zw.wrHdr = true
		}
		zw.encodeBlock(vals)
	}()
	var err error
	if zw.OutputOffset, err = zw.wr.Flush(); zw.err == nil {
		zw.err = err
	}
	if zw.err != nil {
		zw.err = errWrap(zw.err, dserrors.Internal)
		return zw.err
	}
	zw.endCRC = (zw.endCRC<<1 | zw.endCRC>>31) ^ zw.blkCRC
	zw.blkCRC = 0
	// Preserve pending RLE state across block boundaries to match libbzip2.
	zw.rle.ResetBuf()
	return nil
}

func (zw *Writer) Close() error {
	if zw.err == errClosed {
		return nil
	}

	// Drain any pending RLE state into buf — mirrors libbzip2 flush_RL
	// (bzlib.c:252-256) called from BZ2_bzCompress at end-of-stream.
	// Physical buf is sized maxIdx+10 (≥ worst-case Write end-state of
	// maxIdx+4 plus a final 5-byte pair-flush), so this can't overflow.
	zw.rle.EmitPending(&zw.crc)
	// Flush RLE buffer if there is left-over data.
	if zw.err = zw.flush(); zw.err != nil {
		return zw.err
	}

	// Write stream footer.
	zw.wr.Offset = zw.OutputOffset
	func() {
		defer dserrors.Recover(&zw.err)
		if !zw.wrHdr {
			// Write stream header.
			zw.wr.WriteBitsBE64(hdrMagic, 16)
			zw.wr.WriteBitsBE64('h', 8)
			zw.wr.WriteBitsBE64(uint64('0'+zw.level), 8)
			zw.wrHdr = true
		}
		zw.wr.WriteBitsBE64(endMagic, 48)
		zw.wr.WriteBitsBE64(uint64(zw.endCRC), 32)
		zw.wr.WritePads(0)
	}()
	var err error
	if zw.OutputOffset, err = zw.wr.Flush(); zw.err == nil {
		zw.err = err
	}
	if zw.err != nil {
		zw.err = errWrap(zw.err, dserrors.Internal)
		return zw.err
	}

	zw.err = errClosed
	return nil
}

func (zw *Writer) encodeBlock(buf []byte) {
	zw.blkCRC = zw.crc.val
	zw.wr.WriteBitsBE64(blkMagic, 48)
	zw.wr.WriteBitsBE64(uint64(zw.blkCRC), 32)
	zw.wr.WriteBitsBE64(0, 1)
	zw.crc.val = 0

	// Step 1: Burrows-Wheeler transformation.
	ptr := zw.bwt.Encode(buf)
	zw.wr.WriteBitsBE64(uint64(ptr), 24)

	// Step 2: Move-to-front transform and run-length encoding.
	var dictMap [256]bool
	for _, c := range buf {
		dictMap[c] = true
	}

	var dictArr [256]uint8
	var bmapLo [16]uint16
	dict := dictArr[:0]
	bmapHi := uint16(0)
	for i, b := range dictMap {
		if b {
			c := uint8(i)
			dict = append(dict, c)
			bmapHi |= 1 << (c >> 4)
			bmapLo[c>>4] |= 1 << (c & 0xf)
		}
	}

	zw.wr.WriteBits(uint(bmapHi), 16)
	for _, m := range bmapLo {
		if m > 0 {
			zw.wr.WriteBits(uint(m), 16)
		}
	}

	zw.mtf.Init(dict, len(buf))
	syms := zw.mtf.Encode(buf)

	// Step 3: Prefix encoding.
	zw.encodePrefix(syms, len(dict))
}

func (zw *Writer) encodePrefix(syms []uint16, numSyms int) {
	numSyms += 2 // Remove 0 symbol, add RUNA, RUNB, and EOB symbols
	if numSyms < 3 {
		panicf(dserrors.Internal, "unable to encode EOB marker")
	}
	syms = append(syms, uint16(numSyms-1)) // EOB marker

	// Compute number of prefix trees needed.
	numTrees := maxNumTrees
	for i, lim := range []int{200, 600, 1200, 2400} {
		if len(syms) < lim {
			numTrees = minNumTrees + i
			break
		}
	}

	// Compute number of block selectors.
	numSels := (len(syms) + numBlockSyms - 1) / numBlockSyms
	if cap(zw.treeSels) < numSels {
		zw.treeSels = make([]uint8, numSels)
	}
	treeSels := zw.treeSels[:numSels]

	// Initialize prefix codes (sym-indexed; .Cnt and .Len start at 0).
	for i := range zw.codes2D[:numTrees] {
		pc := zw.codes2D[i][:numSyms]
		for j := range pc {
			pc[j] = prefix.PrefixCode{Sym: uint32(j)}
		}
		zw.codes1D[i] = pc
	}

	// K-means tree-selection refinement, ported from libbzip2 sendMTFValues.
	// libbzip2 1.0.8 compress.c:239-452. Without this, dsnet/compress falls
	// back to round-robin tree assignment, producing ~10% larger output than
	// libbzip2 on the same input.
	const (
		bzNIters       = 4  // BZ_N_ITERS: K-means refinement passes
		bzLesserICost  = 0  // BZ_LESSER_ICOST: seed length for in-partition symbols
		bzGreaterICost = 15 // BZ_GREATER_ICOST: seed length for out-of-partition symbols
		bzMaxCodeLen   = 17 // libbzip2 BZ2_hbMakeCodeLengths max; bzip2 1.0.3+ lowered from 20
	)

	// Seed: partition symbols by cumulative frequency, give each tree a
	// "cheap zone" of contiguous symbols.
	mtfFreq := make([]int, numSyms)
	for _, s := range syms {
		mtfFreq[s]++
	}
	for t := 0; t < numTrees; t++ {
		for v := 0; v < numSyms; v++ {
			zw.codes2D[t][v].Len = bzGreaterICost
		}
	}
	{
		remF := len(syms)
		nPart := numTrees
		gs := 0
		for nPart > 0 {
			tFreq := remF / nPart
			ge := gs - 1
			aFreq := 0
			for aFreq < tFreq && ge < numSyms-1 {
				ge++
				aFreq += mtfFreq[ge]
			}
			// Parity tweak from libbzip2: nudge the boundary back one symbol
			// for odd partition indices (skipping the first and last partitions).
			if ge > gs && nPart != numTrees && nPart != 1 && (numTrees-nPart)%2 == 1 {
				aFreq -= mtfFreq[ge]
				ge--
			}
			for v := gs; v <= ge; v++ {
				zw.codes2D[nPart-1][v].Len = bzLesserICost
			}
			nPart--
			gs = ge + 1
			remF -= aFreq
		}
	}

	// Iterate: each pass assigns each 50-symbol group to the tree with the
	// shortest current encoding, accumulates per-tree symbol frequencies,
	// then recomputes Huffman lengths for the next pass via libbzip2's
	// heap-with-depth-tie-breaker algorithm (see hbMakeCodeLengths).
	freqBuf := make([]int32, numSyms)
	for iter := 0; iter < bzNIters; iter++ {
		for t := 0; t < numTrees; t++ {
			for v := 0; v < numSyms; v++ {
				zw.codes2D[t][v].Cnt = 0
			}
		}

		selIdx := 0
		for gs := 0; gs < len(syms); gs += numBlockSyms {
			ge := gs + numBlockSyms
			if ge > len(syms) {
				ge = len(syms)
			}
			var bestCost uint32 = math.MaxUint32
			var bestTree uint8
			for t := 0; t < numTrees; t++ {
				var cost uint32
				for i := gs; i < ge; i++ {
					cost += zw.codes2D[t][syms[i]].Len
				}
				if cost < bestCost {
					bestCost = cost
					bestTree = uint8(t)
				}
			}
			treeSels[selIdx] = bestTree
			selIdx++
			for i := gs; i < ge; i++ {
				zw.codes2D[bestTree][syms[i]].Cnt++
			}
		}

		for t := 0; t < numTrees; t++ {
			for v := 0; v < numSyms; v++ {
				freqBuf[v] = int32(zw.codes2D[t][v].Cnt)
			}
			lens := hbMakeCodeLengths(freqBuf, numSyms, bzMaxCodeLen)
			for v := 0; v < numSyms; v++ {
				zw.codes2D[t][v].Len = uint32(lens[v])
			}
		}
	}

	// Write out information about the trees and tree selectors.
	var mtf dscommon.MoveToFront
	zw.wr.WriteBitsBE64(uint64(numTrees), 3)
	zw.wr.WriteBitsBE64(uint64(numSels), 15)
	zw.treeSelsMTF = append(zw.treeSelsMTF[:0], treeSels...)
	mtf.Encode(zw.treeSelsMTF)
	for _, sym := range zw.treeSelsMTF {
		zw.wr.WriteSymbol(uint(sym), &encSel)
	}
	zw.wr.WritePrefixCodes(zw.codes1D[:numTrees], zw.trees1D[:numTrees])

	// Write out prefix encoded symbols of compressed data.
	var tree *prefix.Encoder
	var blkLen, selIdx int
	for _, sym := range syms {
		if blkLen == 0 {
			blkLen = numBlockSyms
			tree = &zw.trees1D[treeSels[selIdx]]
			selIdx++
		}
		blkLen--
		ok := zw.wr.TryWriteSymbol(uint(sym), tree)
		if !ok {
			zw.wr.WriteSymbol(uint(sym), tree)
		}
	}
}
