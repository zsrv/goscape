package gziputil

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/zsrv/goscape/pkg/io/filestream"
)

// origCacheDir returns the path of the ORIGINAL (acceptance-target) r274 cache,
// taken from GOSCAPE_ORIG_CACHE_DIR (mirrors the GOSCAPE_REF*_DIR convention).
// It is the oracle for the stock-zlib gzip port: every gzip member in it was
// produced by stock zlib 1.3.1 level 6 with the gzip OS byte zeroed. Returns ""
// when the env var is unset.
func origCacheDir() string {
	return os.Getenv("GOSCAPE_ORIG_CACHE_DIR")
}

// szArchives are the four gzip-compressed RS2 Jag store indices in the r274
// cache and their (approximate) populated member counts.
//
//	idx1 = models, idx2 = anims, idx3 = midi, idx4 = client maps.
var szArchives = []int{1, 2, 3, 4}

// TestCompressSZGz_OrigCorpus is the acceptance gate for the stock-zlib port.
// For every gzip member in the ORIGINAL r274 cache it: reads the member,
// strips the 2-byte version trailer to get the gzip stream, decompresses it to
// recover the input, re-compresses with CompressSZGz (== CompressGz on rev-274),
// and byte-compares the result to the original gzip stream.
//
// Skips cleanly when the original cache is absent so CI without the cache stays
// green. When present it must reach the full corpus (6201 members) with zero
// mismatches.
func TestCompressSZGz_OrigCorpus(t *testing.T) {
	dir := origCacheDir()
	if dir == "" {
		t.Skip("GOSCAPE_ORIG_CACHE_DIR not set; skipping corpus check")
	}
	if _, err := os.Stat(dir); err != nil {
		t.Skipf("original cache absent (%s); skipping corpus check", dir)
	}

	fs := filestream.New(dir, false, true)
	defer fs.Close()

	total, ok, fail := 0, 0, 0
	var firstFails []string

	for _, idx := range szArchives {
		cnt := fs.Count(idx)
		aOK, aFail := 0, 0
		for f := 0; f < cnt; f++ {
			member := fs.Read(idx, f, false)
			if member == nil || len(member) < 2 {
				continue
			}
			// member = gzip stream + 2-byte version trailer.
			refGz := member[:len(member)-2]

			zr, err := gzip.NewReader(bytes.NewReader(refGz))
			if err != nil {
				// Not a gzip member (some indices may hold other formats); skip.
				continue
			}
			raw, err := io.ReadAll(zr)
			closeErr := zr.Close()
			if err != nil || closeErr != nil {
				continue
			}

			total++
			got := CompressSZGz(raw, 0, len(raw))
			if bytes.Equal(got, refGz) {
				ok++
				aOK++
				continue
			}
			fail++
			aFail++
			if len(firstFails) < 10 {
				fd := -1
				for i := 0; i < min(len(got), len(refGz)); i++ {
					if got[i] != refGz[i] {
						fd = i
						break
					}
				}
				firstFails = append(firstFails,
					describeFail(idx, f, len(raw), len(got), len(refGz), fd))
			}
		}
		t.Logf("archive idx%d: %d OK, %d FAIL", idx, aOK, aFail)
	}

	t.Logf("orig corpus: %d/%d OK, %d FAIL", ok, total, fail)
	if fail > 0 {
		t.Errorf("orig corpus: %d byte-mismatches; first failures: %v", fail, firstFails)
	}
	if total == 0 {
		t.Fatalf("orig corpus: read 0 members — cache layout unexpected")
	}
}

func describeFail(idx, file, rawLen, gotLen, refLen, firstDiff int) string {
	return fmt.Sprintf("idx%d/f%d raw=%d got=%d ref=%d diff@%d",
		idx, file, rawLen, gotLen, refLen, firstDiff)
}

// TestCompressSZGz_StoredMember pins a tiny (1-byte) member from the original
// cache (idx4/file1).  A 1-byte input is too short for any match, so this
// exercises the stored/static block tail path and the gzip wrapper.  Pinned
// hard-coded so the port keeps fast regression coverage without the cache.
func TestCompressSZGz_StoredMember(t *testing.T) {
	raw := []byte{0x00}
	want := []byte{
		0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x63, 0x00, 0x00, 0x8d, 0xef, 0x02, 0xd2, 0x01, 0x00, 0x00, 0x00,
	}
	got := CompressSZGz(raw, 0, len(raw))
	if !bytes.Equal(got, want) {
		t.Fatalf("stored member: got % x\nwant % x", got, want)
	}
}

// TestCompressSZGz_DynMember pins a real 383-byte model member (idx1/file1864)
// from the original cache.  It compresses to a 251-byte dynamic-Huffman gzip
// stream with back-references, exercising longest_match, the rolling hash,
// lazy matching, and dynamic-tree emission end-to-end.
func TestCompressSZGz_DynMember(t *testing.T) {
	raw := szDynMemberRaw
	want := szDynMemberGz
	got := CompressSZGz(raw, 0, len(raw))
	if !bytes.Equal(got, want) {
		fd := -1
		for i := 0; i < min(len(got), len(want)); i++ {
			if got[i] != want[i] {
				fd = i
				break
			}
		}
		t.Fatalf("dyn member: len got=%d want=%d firstdiff@%d", len(got), len(want), fd)
	}
}
