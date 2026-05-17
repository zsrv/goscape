package bzip2

// hbMakeCodeLengths ports libbzip2 BZ2_hbMakeCodeLengths (huffman.c:63-148) to
// Go. The vendored dsnet/compress prefix.GenerateLengths uses a different
// Huffman algorithm (two-queue O(n) with no tie-breaker on equal-frequency
// nodes), producing slightly different code-length assignments than libbzip2's
// heap-based algorithm. Mirroring libbzip2 here is required for byte-identical
// output against `bzip2 -1` from system libbzip2 and Engine-TS's bzip2-wasm.
//
// Key invariants from libbzip2 huffman.c:
//   - zero-frequency symbols are treated as frequency 1 (line 80).
//   - weight is packed as (freq<<8) | depth; ADDWEIGHTS sums freqs and bumps
//     depth by `1 + max(left_depth, right_depth)`, biasing the tree toward
//     balance and giving deterministic tie-breaks.
//   - on length-overflow (any code > maxLen), all weights are scaled by
//     `j = 1 + (j/2)` (line 142-146) and the whole tree is rebuilt.
//
// Inputs:
//
//	freq      per-symbol counts (length must be >= alphaSize).
//	alphaSize number of symbols.
//	maxLen    maximum bit-length (libbzip2 1.0.3+ uses 17).
//
// Returns a fresh []uint8 of length alphaSize with per-symbol lengths.
func hbMakeCodeLengths(freq []int32, alphaSize, maxLen int) []uint8 {
	const maxAlpha = 258 // BZ_MAX_ALPHA_SIZE; bzip2 caps at 258 (256 syms + RUNA/RUNB)
	var heap [maxAlpha + 2]int32
	var weight [maxAlpha * 2]int32
	var parent [maxAlpha * 2]int32

	for i := 0; i < alphaSize; i++ {
		w := freq[i]
		if w == 0 {
			w = 1
		}
		weight[i+1] = w << 8
	}

	out := make([]uint8, alphaSize)

	for {
		nNodes := int32(alphaSize)
		nHeap := int32(0)

		heap[0] = 0
		weight[0] = 0
		parent[0] = -2

		for i := 1; i <= alphaSize; i++ {
			parent[i] = -1
			nHeap++
			heap[nHeap] = int32(i)
			upHeap(&heap, &weight, nHeap)
		}

		for nHeap > 1 {
			n1 := heap[1]
			heap[1] = heap[nHeap]
			nHeap--
			downHeap(&heap, &weight, 1, nHeap)
			n2 := heap[1]
			heap[1] = heap[nHeap]
			nHeap--
			downHeap(&heap, &weight, 1, nHeap)
			nNodes++
			parent[n1] = nNodes
			parent[n2] = nNodes
			weight[nNodes] = addWeights(weight[n1], weight[n2])
			parent[nNodes] = -1
			nHeap++
			heap[nHeap] = nNodes
			upHeap(&heap, &weight, nHeap)
		}

		tooLong := false
		for i := 1; i <= alphaSize; i++ {
			j := 0
			k := int32(i)
			for parent[k] >= 0 {
				k = parent[k]
				j++
			}
			out[i-1] = uint8(j)
			if j > maxLen {
				tooLong = true
			}
		}

		if !tooLong {
			return out
		}

		for i := 1; i <= alphaSize; i++ {
			j := weight[i] >> 8
			j = 1 + (j / 2)
			weight[i] = j << 8
		}
	}
}

// upHeap mirrors libbzip2 huffman.c:33-42 UPHEAP macro.
func upHeap(heap *[260]int32, weight *[516]int32, z int32) {
	tmp := heap[z]
	for weight[tmp] < weight[heap[z>>1]] {
		heap[z] = heap[z>>1]
		z >>= 1
	}
	heap[z] = tmp
}

// downHeap mirrors libbzip2 huffman.c:44-59 DOWNHEAP macro.
func downHeap(heap *[260]int32, weight *[516]int32, z, nHeap int32) {
	tmp := heap[z]
	for {
		yy := z << 1
		if yy > nHeap {
			break
		}
		if yy < nHeap && weight[heap[yy+1]] < weight[heap[yy]] {
			yy++
		}
		if weight[tmp] < weight[heap[yy]] {
			break
		}
		heap[z] = heap[yy]
		z = yy
	}
	heap[z] = tmp
}

// addWeights mirrors libbzip2 huffman.c:29-31 ADDWEIGHTS macro: high 24 bits
// hold the summed frequency; low 8 bits hold 1+max(left_depth,right_depth) to
// break ties in favor of balanced subtrees.
func addWeights(w1, w2 int32) int32 {
	d1 := w1 & 0xff
	d2 := w2 & 0xff
	d := d1
	if d2 > d {
		d = d2
	}
	return (w1 & ^int32(0xff)) + (w2 & ^int32(0xff)) | (1 + d)
}
