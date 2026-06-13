// Package versionlist — versionlist.go implements the three versionlist index
// unpack tools that mirror TS tools/unpack/versionlist/{anim_index,midi_index,
// model_index}.ts (Engine-TS 9aadcec4).
//
// All three tools read archive 0 / file 5 (versionlist) from the cache and
// emit their respective index member as console output or cache files.
//
// TS sources:
//   - tools/unpack/versionlist/anim_index.ts (13 lines)
//   - tools/unpack/versionlist/midi_index.ts (14 lines)
//   - tools/unpack/versionlist/model_index.ts (62 lines)
package versionlist

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/zsrv/goscape/pkg/io/filestream"
	"github.com/zsrv/goscape/pkg/io/jagfile"
	"github.com/zsrv/goscape/pkg/io/packet"
	"github.com/zsrv/goscape/pkg/pack"
)

// openVersionlist opens the cache (readOnly=false, matching TS default FileStream
// constructor), reads archive 0 / file 5, and returns the parsed Jagfile.
// TS: new FileStream('data/unpack') — createNew=false, readOnly=false.
func openVersionlist(cacheDir string) (*filestream.FileStream, *jagfile.Jagfile, error) {
	cache := filestream.New(cacheDir, false, false)
	data := cache.Read(0, 5, false)
	if data == nil {
		cache.Close()
		return nil, nil, fmt.Errorf("no versionlist in cache")
	}
	vl, err := jagfile.NewJagfile(packet.NewPacket(data))
	if err != nil {
		cache.Close()
		return nil, nil, fmt.Errorf("parse versionlist jagfile: %w", err)
	}
	return cache, vl, nil
}

// AnimIndex prints each anim_index entry as:
//
//	<i> <flags-binary-padStart8> <flags-decimal>
//
// Mirrors TS tools/unpack/versionlist/anim_index.ts.
//
// TS console.log format (line 12):
//
//	console.log(i, flags.toString(2).padStart(8, '0'), flags)
//
// JS padStart(8,'0') on a number that is already ≥8 binary digits leaves it
// as-is (padStart only pads when the string is shorter than the target width).
// For g2 values 0..65535 binary length is at most 16 digits; padStart(8,'0')
// pads only values 0..255 (0-digit binary = "0", up to "11111111").
// Go mirrors this with fmt.Sprintf("%08b", …) — the %0Nb verb left-pads with
// zeros only when the formatted value is shorter than N digits; wider values
// are rendered without truncation.
//
// Output is written to out (stdout equivalent); nil = discard.
//
// TS source: tools/unpack/versionlist/anim_index.ts.
func AnimIndex(cacheDir string, out io.Writer) error {
	if out == nil {
		out = io.Discard
	}

	cache, vl, err := openVersionlist(cacheDir)
	if err != nil {
		return fmt.Errorf("AnimIndex: %w", err)
	}
	defer cache.Close()

	// TS line 7: versionlist.read('anim_index').
	index, err := vl.Read("anim_index")
	if err != nil {
		return fmt.Errorf("AnimIndex: read anim_index: %w", err)
	}

	// TS line 9: const size = index.length / 2.
	size := index.Length() / 2

	// TS lines 10-12: for i < size; g2(); console.log(i, flags.toString(2).padStart(8,'0'), flags).
	for i := range size {
		flags := int(index.G2())
		// JS padStart(8,'0'): pad to at least 8 binary digits with leading zeros.
		// %08b mirrors this: pads when <8 digits, leaves wider values unchanged.
		fmt.Fprintf(out, "%d %08b %d\n", i, flags, flags)
	}

	return nil
}

// MidiIndex prints each midi_index entry as:
//
//	<i> <name> <prefetch>
//
// where name is the midi name from MidiPack (or "" if absent) and prefetch is
// the g1() byte value.
//
// Mirrors TS tools/unpack/versionlist/midi_index.ts.
//
// TS console.log format (line 13):
//
//	console.log(i, MidiPack.getById(i), prefetch)
//
// JS console.log with three args joins them with a space: "<i> <name> <prefetch>".
// When MidiPack.getById(i) returns undefined (absent), JS prints "undefined".
// Go mirrors "absent" as the empty PackFile sentinel ""; we render "" as "undefined"
// to match TS output for unregistered ids.
//
// srcDir is the content tree root (BUILD_SRC_DIR in TS); the MidiPack
// registry is loaded from <srcDir>/pack/midi.pack.
//
// Output is written to out (stdout equivalent); nil = discard.
//
// TS source: tools/unpack/versionlist/midi_index.ts.
func MidiIndex(cacheDir, srcDir string, out io.Writer) error {
	if out == nil {
		out = io.Discard
	}

	cache, vl, err := openVersionlist(cacheDir)
	if err != nil {
		return fmt.Errorf("MidiIndex: %w", err)
	}
	defer cache.Close()

	// TS line 8: versionlist.read('midi_index').
	index, err := vl.Read("midi_index")
	if err != nil {
		return fmt.Errorf("MidiIndex: read midi_index: %w", err)
	}

	// MidiPack is the shared #tools/pack/PackFile.js singleton, constructed
	// empty under 274 suspendAutoReload (TS PackFile.ts:276 @dee467c8), so
	// getById returns "" for every id — the index prints an empty name field.
	reg := &pack.Registry{SrcDir: srcDir, SuspendAutoReload: true}
	midiPack, err := reg.EnsureMidi()
	if err != nil {
		return fmt.Errorf("MidiIndex: ensure midi pack: %w", err)
	}

	// TS line 10: const size = index.length.
	size := index.Length()

	// TS lines 11-13: for i < size; g1(); console.log(i, MidiPack.getById(i), prefetch).
	// getById returns '' (PackFileBase.ts:131 `?? ''`), never undefined, so
	// console.log joins `i`, the (possibly empty) name and prefetch with single
	// spaces: an empty name leaves a double space (e.g. "0  5"). Under 274
	// suspendAutoReload the registry is empty, so every line carries the empty
	// name field.
	for i := range size {
		prefetch := int(index.G1())
		name := midiPack.GetByID(i)
		fmt.Fprintf(out, "%d %s %d\n", i, name, prefetch)
	}

	return nil
}

// ModelIndex saves the raw model_index member bytes and writes model_index.txt.
//
// Two files are written into cacheDir:
//   - <cacheDir>/model_index — raw bytes of the model_index jagfile member
//     (Packet.save semantics: writes Data[0:length], length = index.Length()).
//   - <cacheDir>/model_index.txt — one line per model id:
//     "<name>=<readable>, 0x<hex2> (0b<binary8>)\n"
//
// <readable> is "none" when flags == 0, otherwise a trimmed space-separated
// list of flag names in order: tutorial dynamic static wornf2p worn invf2p inv player.
// The id column uses the model name from ModelPack when available; otherwise i.
//
// ModelIndex also prints nothing to stdout (STDOUT-NORM = sha256 of empty string).
//
// srcDir is the content tree root used to load ModelPack.
//
// Mirrors TS tools/unpack/versionlist/model_index.ts.
//
// TS source: tools/unpack/versionlist/model_index.ts.
func ModelIndex(cacheDir, srcDir string, out io.Writer) error {
	cache, vl, err := openVersionlist(cacheDir)
	if err != nil {
		return fmt.Errorf("ModelIndex: %w", err)
	}
	defer cache.Close()

	// TS line 11: versionlist.read('model_index').
	modelIndex, err := vl.Read("model_index")
	if err != nil {
		return fmt.Errorf("ModelIndex: read model_index: %w", err)
	}

	// TS line 14: modelIndex.save('data/unpack/model_index', modelIndex.length)
	// Packet.save(path, length, start=0): writes Data[0:length] to path.
	indexPath := filepath.Join(cacheDir, "model_index")
	if err := modelIndex.Save(indexPath, modelIndex.Length(), 0); err != nil {
		return fmt.Errorf("ModelIndex: save model_index: %w", err)
	}

	// TS line 15: fs.writeFileSync('data/unpack/model_index.txt', '')
	// — creates or truncates the file before appending.
	txtPath := filepath.Join(cacheDir, "model_index.txt")
	if err := os.WriteFile(txtPath, nil, 0o644); err != nil {
		return fmt.Errorf("ModelIndex: create model_index.txt: %w", err)
	}

	// ModelPack is the shared #tools/pack/PackFile.js singleton, constructed
	// empty under 274 suspendAutoReload (TS PackFile.ts:276 @dee467c8), so
	// getById misses for every id and the index falls back to the numeric id
	// (TS `ModelPack.getById(i) || i`).
	reg := &pack.Registry{SrcDir: srcDir, SuspendAutoReload: true}
	modelPack, err := reg.EnsureModel()
	if err != nil {
		return fmt.Errorf("ModelIndex: ensure model pack: %w", err)
	}

	// TS lines 16-61: main loop.
	// Reset the read position; Save above did not advance Pos (it reads Data[0:length]).
	modelIndex.Pos = 0

	// Open model_index.txt for append (matches TS appendFileSync pattern).
	f, err := os.OpenFile(txtPath, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("ModelIndex: open model_index.txt for append: %w", err)
	}
	defer f.Close()

	for i := range modelIndex.Length() {
		flags := int(modelIndex.G1())

		// TS lines 19-57: decode flags to readable string.
		var readable string
		if flags == 0 {
			readable = "none"
		} else {
			var parts []string
			if flags&0x1 != 0 {
				parts = append(parts, "tutorial")
			}
			if flags&0x2 != 0 {
				parts = append(parts, "dynamic")
			}
			if flags&0x4 != 0 {
				parts = append(parts, "static")
			}
			if flags&0x8 != 0 {
				parts = append(parts, "wornf2p")
			}
			if flags&0x10 != 0 {
				parts = append(parts, "worn")
			}
			if flags&0x20 != 0 {
				parts = append(parts, "invf2p")
			}
			if flags&0x40 != 0 {
				parts = append(parts, "inv")
			}
			if flags&0x80 != 0 {
				parts = append(parts, "player")
			}
			// TS: readable = readable.trimEnd() — strips trailing space.
			// strings.Join produces no trailing space, so trimEnd is a no-op.
			readable = strings.Join(parts, " ")
		}

		// TS line 59: const id = ModelPack.getById(i) || i
		// When absent, getById returns "" (falsy in JS), so fall back to i.
		idStr := modelPack.GetByID(i)
		var id string
		if idStr != "" {
			id = idStr
		} else {
			id = fmt.Sprintf("%d", i)
		}

		// TS line 60: fs.appendFileSync('data/unpack/model_index.txt',
		//   `${id}=${readable}, 0x${modelFlags[i].toString(16).padStart(2,'00')} (0b${modelFlags[i].toString(2).padStart(8,'0')})\n`)
		// hex: padStart(2,'0') — lowercase hex, minimum 2 digits.
		// binary: padStart(8,'0') — minimum 8 binary digits.
		line := fmt.Sprintf("%s=%s, 0x%02x (0b%08b)\n", id, readable, flags, flags)
		if _, err := f.WriteString(line); err != nil {
			return fmt.Errorf("ModelIndex: write model_index.txt: %w", err)
		}
	}

	// Explicit close: a flush-on-close failure must not be reported as success
	// (the deferred close above still guards the error returns).
	if err := f.Close(); err != nil {
		return fmt.Errorf("ModelIndex: close model_index.txt: %w", err)
	}
	return nil
}
