// Package midi — midi.go implements the midi-family unpack entry point that
// mirrors TS tools/unpack/midi/Unpack.ts (43 lines, Engine-TS 9aadcec4).
//
// The unpack reads archive 0 / file 5 (versionlist) from the cache, parses
// the midi_index jagfile member (1 byte per entry: jingle flag), and for each
// midi in archive 3 writes raw data to <srcDir>/songs/<name>.mid or
// <srcDir>/jingles/<name>.mid based on the jingle flag.  Missing midi data
// emits a printWarning.  Calls MidiPack.save() after the loop.
//
// TS source: tools/unpack/midi/Unpack.ts.
package midi

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/zsrv/goscape/pkg/io/filestream"
	"github.com/zsrv/goscape/pkg/io/jagfile"
	"github.com/zsrv/goscape/pkg/io/packet"
	"github.com/zsrv/goscape/pkg/pack"
)

// Options holds all inputs for a midi-family unpack run.
type Options struct {
	// CacheDir is the directory containing main_file_cache.dat/idx0-4.
	// TS: new FileStream('data/unpack', false, true) — readOnly=true.
	CacheDir string

	// SrcDir is the content tree root (BUILD_SRC_DIR in TS).  Output files
	// are written to <SrcDir>/songs/ and <SrcDir>/jingles/.
	SrcDir string

	// Out is the printWarning channel.  nil = discard.
	Out io.Writer
}

// Unpack is the top-level entry point for the midi-family unpack.
// It mirrors TS tools/unpack/midi/Unpack.ts.
//
// The function:
//  1. mkdir <srcDir>/songs and <srcDir>/jingles.
//  2. Opens the FileStream cache (readOnly=true) and reads archive 0 / file 5
//     (versionlist); fatal error if absent.
//  3. Parses the versionlist as a Jagfile and reads its midi_index entry.
//  4. Iterates cache.count(3) midis: reads data from archive 3; resolves
//     name from MidiPack or falls back to "midi_<i>"; registers (i, name);
//     reads g1() jingle flag (BEFORE the data-nil check — TS line order);
//     writes raw data to songs/ or jingles/; warns on missing data.
//  5. Calls MidiPack.save().
//
// TS source: tools/unpack/midi/Unpack.ts.
//
// Note: console.time/timeEnd diagnostics are not ported (timing only).
func Unpack(opts Options) error {
	printWarning := func(msg string) {
		if opts.Out != nil {
			fmt.Fprintf(opts.Out, "%s\n", msg)
		}
	}

	// TS lines 10-16: mkdir songs/ and jingles/.
	songsDir := filepath.Join(opts.SrcDir, "songs")
	if err := os.MkdirAll(songsDir, 0o755); err != nil {
		return fmt.Errorf("midi: mkdir songs: %w", err)
	}
	jinglesDir := filepath.Join(opts.SrcDir, "jingles")
	if err := os.MkdirAll(jinglesDir, 0o755); err != nil {
		return fmt.Errorf("midi: mkdir jingles: %w", err)
	}

	// TS line 18: new FileStream('data/unpack', false, true) — readOnly=true.
	cache := filestream.New(opts.CacheDir, false, true)
	defer cache.Close()

	// TS line 19: cache.read(0, 5) → versionlist Jagfile.
	data := cache.Read(0, 5, false)
	if data == nil {
		return fmt.Errorf("midi: no versionlist in cache")
	}

	versionlist, err := jagfile.NewJagfile(packet.NewPacket(data))
	if err != nil {
		return fmt.Errorf("midi: parse versionlist jagfile: %w", err)
	}

	// TS line 20: versionlist.read('midi_index').
	index, err := versionlist.Read("midi_index")
	if err != nil {
		return fmt.Errorf("midi: read midi_index: %w", err)
	}

	// MidiPack is imported from the shared #tools/pack/PackFile.js, whose
	// singletons are constructed empty under suspendAutoReload (NEW at 274,
	// TS PackFile.ts:276 @dee467c8). So getById misses for every id and the
	// loop falls back to midi_<i>. SuspendAutoReload mirrors that.
	reg := &pack.Registry{SrcDir: opts.SrcDir, SuspendAutoReload: true}
	midiPack, err := reg.EnsureMidi()
	if err != nil {
		return fmt.Errorf("midi: ensure midi pack: %w", err)
	}

	// TS line 24: cache.count(3).
	midiCount := cache.Count(3)

	// TS lines 24-40: main loop.
	for i := range midiCount {
		fileData := cache.Read(3, i, true)

		// TS lines 27-30: name from MidiPack or fallback.
		name := midiPack.GetByID(i)
		if name == "" {
			name = fmt.Sprintf("midi_%d", i)
		}
		midiPack.Register(i, name)

		// TS line 33: const jingle = index.g1() — consumed BEFORE data check.
		jingle := index.G1()

		if fileData != nil {
			// TS line 36: writeFileSync to songs/ or jingles/ based on jingle flag.
			var dir string
			if jingle != 0 {
				dir = jinglesDir
			} else {
				dir = songsDir
			}
			outPath := filepath.Join(dir, name+".mid")
			if err := os.WriteFile(outPath, fileData, 0o644); err != nil {
				return fmt.Errorf("midi: write %s: %w", outPath, err)
			}
		} else {
			// TS line 38: printWarning(`Missing midi id=${i}`).
			printWarning(fmt.Sprintf("Missing midi id=%d", i))
		}
	}

	// TS line 43: MidiPack.save().
	if err := midiPack.Save(); err != nil {
		return fmt.Errorf("midi: save midi pack: %w", err)
	}

	return nil
}
