package audio

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/zsrv/goscape/pkg/io/jagfile"
)

// PackMidi ports TS midi/pack.ts:packClientMidi.
//
// For each of jingles/ and songs/ under srcDir, copies new files
// into <outDir>/client/<subdir>/, bzip2-compressed (prefixLength=true,
// removeHeader=false, blockSize=1) matching TS BZip2.compress(data, true).
//
// NAI-213-D-PACKMIDI-MTIME-CHECK-MIRROR-TS-TODO: per-file gate is
// existence-only (TS comment: "TODO: mtime-based check"). Both TS
// and goscape have this TODO.
//
// NAI-213-D-PACKMIDI-PER-SUBDIR-GATE: TS calls shouldBuild() only on
// jingles/, and if the gate skips, ALSO skips songs/. goscape gates
// per-subdir independently (more correct: each subdir's freshness is
// independent). Behavioral divergence; no client-visible side effect
// since both eventually converge.
//
// NAI-192-D-NO-SRC-NO-OP mirror: missing src subdir => no-op.
func PackMidi(srcDir, outDir string) error {
	for _, sub := range []string{"jingles", "songs"} {
		if err := packMidiSubdir(srcDir, outDir, sub); err != nil {
			return err
		}
	}
	return nil
}

func packMidiSubdir(srcDir, outDir, sub string) error {
	srcSub := filepath.Join(srcDir, sub)
	outSub := filepath.Join(outDir, "client", sub)

	if _, err := os.Stat(srcSub); os.IsNotExist(err) {
		// NAI-192-D-NO-SRC-NO-OP mirror.
		return nil
	}

	if err := os.MkdirAll(outSub, 0o755); err != nil {
		return err
	}

	entries, err := os.ReadDir(srcSub)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		dest := filepath.Join(outSub, e.Name())
		// Existence-only skip - TS midi/pack.ts:15-17 + 27-29.
		if _, err := os.Stat(dest); err == nil {
			continue
		}
		data, err := os.ReadFile(filepath.Join(srcSub, e.Name()))
		if err != nil {
			return err
		}
		// TS BZip2.compress(data, true) =
		//   (prefixLength=true, removeHeader=false, blockSize=1, compressedLength=0)
		compressed, err := jagfile.BZip2Compress(data, true, false, 1, 0)
		if err != nil {
			return fmt.Errorf("PackMidi(%s/%s): %w", sub, e.Name(), err)
		}
		if err := os.WriteFile(dest, compressed, 0o644); err != nil {
			return err
		}
	}
	return nil
}
