// Package wordenc ports the TS client-wordenc packer
// (tools/pack/chat/pack.ts@9aadcec4) to Go.
//
// Rev-244: the four-txt builder was removed in favour of a direct blob
// pass-through. packClientWordenc now reads the pre-built Jagfile from
// data/raw/wordenc and writes it to cache archive 0 file 7.
package wordenc

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/zsrv/goscape/pkg/pack"

	"github.com/zsrv/goscape/pkg/io/filestream"
)

// Pack ports TS tools/pack/chat/pack.ts:packClientWordenc at revision 244.
//
// Reads the pre-built wordenc Jagfile from <rawDir>/wordenc and writes it to
// cache archive 0 file 7. This mirrors the TS replacement of the four-txt
// builder with: `cache.write(0, 7, fs.readFileSync('data/raw/wordenc'))`.
//
// rawDir is the directory containing the engine-owned raw blobs. It mirrors
// the TS hardcoded 'data/raw' relative path; callers in pkg/packall pass
// "data/raw" (the project-root-relative location), which matches the TS
// convention. Tests may supply a custom rawDir pointing to a fixture.
//
// cache is an optional *filestream.FileStream. When nil the blob is read and
// validated (fail-fast on missing file) but the cache.Write is skipped.
// Real handle is wired in T15.
func Pack(rawDir string, cache *filestream.FileStream) error {
	blobPath := filepath.Join(rawDir, "wordenc")
	data, err := os.ReadFile(blobPath)
	if err != nil {
		return fmt.Errorf("wordenc.Pack: read %q: %w", blobPath, err)
	}
	if cache != nil {
		// TS chat/pack.ts:8-11 @1d25566c — new gate at 8139461a, over the raw
		// data/raw/wordenc blob.
		pack.VerifyArchive("packClientWordenc", data, pack.WordencCRCMagic)
		cache.Write(0, 7, data, 0)
	}
	return nil
}
