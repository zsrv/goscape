// Package audio ports the client-stage audio packers from
// tools/pack/sound/pack.ts and tools/pack/midi/pack.ts.
package audio

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/zsrv/goscape/pkg/io/filestream"
	"github.com/zsrv/goscape/pkg/io/gziputil"
	"github.com/zsrv/goscape/pkg/pack"
)

// PackMidi ports TS tools/pack/midi/pack.ts:packClientMidi at rev-244.
//
// For each .mid file in <srcDir>/jingles and <srcDir>/songs (lexical order,
// jingles first matching TS spread `[...jingles, ...songs]`):
//   - id = MidiPack.getByName(basename sans ".mid")
//   - data = file bytes; if len > 0: cache.Write(3, id, CompressGz(data), 1)
//
// cache is required for the write path. When nil the stage is a no-op (the
// cache is the only output at 244; the 225 bzip2 per-file subdirs are deleted
// upstream and not produced here). Callers in pkg/packall pass nil until T15
// wires the real FileStream handle.
//
// Rev-244: the 225 shouldBuild guard and bzip2 client/jingles + client/songs
// dirs are deleted upstream. This port follows suit — no file outputs, no
// existence-check skip, no bzip2.
//
// NAI-192-D-NO-SRC-NO-OP mirror: missing jingles/songs dirs → no .mid files
// returned by ListFilesExt → no-op (not an error).
//
// TS source: tools/pack/midi/pack.ts:10-19 @ 9aadcec4 (rev-244).
func PackMidi(reg *pack.Registry, srcDir string, cache *filestream.FileStream) error {
	if cache == nil {
		// T15 comment: nil cache means real FileStream not yet wired; no-op.
		return nil
	}

	midiPack, err := reg.EnsureMidi()
	if err != nil {
		return err
	}

	// TS: [...listFilesExt(jingles, '.mid'), ...listFilesExt(songs, '.mid')]
	var midis []string
	midis = append(midis, pack.ListFilesExt(filepath.Join(srcDir, "jingles"), ".mid")...)
	midis = append(midis, pack.ListFilesExt(filepath.Join(srcDir, "songs"), ".mid")...)

	for _, file := range midis {
		base := filepath.Base(file)
		name := strings.TrimSuffix(base, ".mid")
		id := midiPack.GetByName(name)
		data, err := os.ReadFile(file)
		if err != nil {
			return fmt.Errorf("audio.PackMidi: read %q: %w", file, err)
		}
		if len(data) > 0 {
			compressed := gziputil.CompressGz(data, 0, len(data))
			if compressed != nil {
				cache.Write(3, id, compressed, 1)
			}
		}
	}

	return nil
}
