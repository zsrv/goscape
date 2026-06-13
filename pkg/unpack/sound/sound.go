// Package sound — sound.go implements the synth-sound unpack entry point that
// mirrors TS tools/unpack/sound/Unpack.ts (230 lines, Engine-TS 9aadcec4).
//
// # Extent-parse mechanics
//
// Wave.unpack (static, TS ~18-95) reads sounds.dat from archive 0 / file 8 of
// the FileStream cache (no decompression — TS FileStream.read(0, …) skips the
// gunzip path).  It loops on g2():
//
//  1. Read id = g2().  id == 65535 → stop.
//  2. Record start = buf.pos (BEFORE the instance unpack).
//  3. Run Wave.unpack(buf) on a new Wave instance (advances buf.pos).
//  4. Record end = buf.pos.
//  5. Seek buf.pos back to start; copy buf[start:end] into a data slice via
//     gdata.
//
// The "instance unpack" (Wave.unpack, TS ~101-113) reads:
//   - 10 tones: g1 probe; if non-zero → pos-- then Tone.unpack; finally g2
//     loopBegin, g2 loopEnd.
//
// Tone.unpack (TS ~133-185):
//   - Always two Envelopes: frequencyBase, amplitudeBase.
//   - Three optional PAIRS gated by "if g1() != 0 → pos--": (freqModRate,
//     freqModRange), (ampModRate, ampModRange), (release, attack).
//   - Harmonics loop 0..9: gsmarts volume; 0 breaks; gsmart semitone (SIGNED);
//     gsmarts delay.
//   - reverbDelay=gsmarts, reverbVolume=gsmarts, length=g2, start=g2.
//
// Envelope.unpack (TS ~196-212):
//   - form=g1, start=g4s, end=g4s.
//   - unpackShape: length=g1; length×(g2 shapeDelta, g2 shapePeak).
//
// # CRC pre-scan gating
//
// The CRC pre-scan of existing .synth files (TS ~24-37) ONLY runs when
// keepNames=false.  The entry-point always calls Wave.unpack(soundsData) with
// no second argument (keepNames defaults to true), so the CRC scan never
// executes.  This port omits it accordingly.
//
// # Object structure
//
// The TS builds Wave / Tone / Envelope objects.  This port mirrors that object
// structure with lean Go structs; the parse methods advance buf.Pos identically
// to the TS instance.unpack calls so the extent bookkeeping (start/end) is
// bit-for-bit identical.
//
// TS source: tools/unpack/sound/Unpack.ts.
package sound

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/zsrv/goscape/pkg/io/filestream"
	"github.com/zsrv/goscape/pkg/io/jagfile"
	"github.com/zsrv/goscape/pkg/io/packet"
	"github.com/zsrv/goscape/pkg/pack"
)

// Options holds all inputs for a sound-family unpack run.
type Options struct {
	// CacheDir is the directory containing main_file_cache.dat/idx0-4.
	// TS: new FileStream('data/unpack') — default readOnly=false, createNew=false.
	CacheDir string

	// SrcDir is the content tree root (BUILD_SRC_DIR in TS).  Output files
	// are written to <SrcDir>/synth/ and <SrcDir>/pack/synth.order.
	SrcDir string

	// Out is the printWarning channel.  nil = discard.
	Out io.Writer
}

// Unpack is the top-level entry point for the sound-family unpack.
// It mirrors TS tools/unpack/sound/Unpack.ts.
//
// The function:
//  1. mkdir <srcDir>/synth.
//  2. Opens the FileStream cache (createNew=false, readOnly=false) and reads
//     archive 0 / file 8 (sounds, decompress=false).
//  3. Parses it as a Jagfile and reads the 'sounds.dat' member.
//  4. Calls waveUnpack(soundsData, synthPack, srcDir, Out) which:
//     a. Loops: read g2 id; 65535 → stop; record start; parse Wave instance;
//     record end; extract buf[start:end] as the .synth payload.
//     b. Resolves name: SynthPack.getById(id) or "sound_<id>"; registers.
//     c. Writes <srcDir>/synth/<name>.synth.
//     d. Calls SynthPack.save() and writes <srcDir>/pack/synth.order.
//
// TS source: tools/unpack/sound/Unpack.ts.
func Unpack(opts Options) error {
	printWarning := func(msg string) {
		if opts.Out != nil {
			fmt.Fprintf(opts.Out, "%s\n", msg)
		}
	}
	_ = printWarning // sound Unpack.ts has no printWarning calls in the keepNames=true path

	// TS lines 19-21: mkdir synth/ (also done inside Wave.unpack static).
	synthDir := filepath.Join(opts.SrcDir, "synth")
	if err := os.MkdirAll(synthDir, 0o755); err != nil {
		return fmt.Errorf("sound: mkdir synth: %w", err)
	}

	// TS line 215: new FileStream('data/unpack') — createNew=false, readOnly=false.
	cache := filestream.New(opts.CacheDir, false, false)
	defer cache.Close()

	// TS line 216: cache.read(0, 8) — archive 0, file 8, decompress=false.
	rawData := cache.Read(0, 8, false)
	if rawData == nil {
		return fmt.Errorf("sound: no archive 0/file 8 in cache")
	}

	sounds, err := jagfile.NewJagfile(packet.NewPacket(rawData))
	if err != nil {
		return fmt.Errorf("sound: parse sounds jagfile: %w", err)
	}

	// TS line 217: sounds.read('sounds.dat').
	soundsData, err := sounds.Read("sounds.dat")
	if err != nil {
		return fmt.Errorf("sound: missing sounds.dat: %w", err)
	}

	// Load SynthPack from srcDir.
	reg := &pack.Registry{SrcDir: opts.SrcDir}
	synthPack, err := reg.EnsureSynth()
	if err != nil {
		return fmt.Errorf("sound: ensure synth pack: %w", err)
	}

	// TS lines 41-94: Wave.unpack static body (keepNames=true path).
	if err := waveUnpack(soundsData, synthPack, opts.SrcDir, synthDir); err != nil {
		return err
	}

	return nil
}

// listFilesExt returns all regular files under root with the given extension,
// walking recursively.  Mirrors TS listFilesExt.
func listFilesExt(root, ext string) ([]string, error) {
	var paths []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(d.Name(), ext) {
			paths = append(paths, path)
		}
		return nil
	})
	return paths, err
}

// waveUnpack is the Go equivalent of Wave.unpack (static, TS ~41-94) with
// keepNames=true (the only path the entry-point exercises).
//
// It populates order, extracts per-sound byte slices, writes .synth files,
// calls SynthPack.save(), and writes pack/synth.order.
//
// The existing-file lookup (TS lines 84-90): TS scans all .synth files under
// synth/ and for a given name, writes to the pre-existing path (e.g.
// synth/npc/bat/bat_death.synth) if found, else creates synth/<name>.synth.
// This preserves the deep directory layout of pre-populated content trees.
func waveUnpack(buf *packet.Packet, synthPack *pack.PackFile, srcDir, synthDir string) error {
	// Build name→path map from existing .synth files in synthDir.
	// TS: listFilesExt(synth/, '.synth') + find(x => x.endsWith('/${name}.synth')).
	existingFiles, err := listFilesExt(synthDir, ".synth")
	if err != nil {
		return fmt.Errorf("sound: list existing synth files: %w", err)
	}
	// Map: basename-without-ext → full path. FIRST entry wins on duplicate
	// basenames, mirroring TS existingFiles.find(endsWith) which returns the
	// first match in listing order (TS Unpack.ts:85). The reference tree has
	// no duplicate basenames today; this keeps the divergence theoretical.
	existingByName := make(map[string]string, len(existingFiles))
	for _, f := range existingFiles {
		base := strings.TrimSuffix(filepath.Base(f), ".synth")
		if _, ok := existingByName[base]; !ok {
			existingByName[base] = f
		}
	}

	var order []int

	// TS lines 41-91: main loop.
	for buf.Len() > 0 {
		id := int(buf.G2())
		if id == 65535 {
			break
		}

		order = append(order, id)

		// Record start position BEFORE the instance parse.
		start := buf.Pos

		// Parse the Wave instance to advance buf.Pos past this sound's bytes.
		if err := parseWave(buf); err != nil {
			return fmt.Errorf("sound: parse wave id=%d: %w", id, err)
		}

		end := buf.Pos

		// TS lines 54-56: extract buf[start:end].
		data := make([]byte, end-start)
		buf.Pos = start
		buf.GData(data, end-start)

		// TS lines 80-90: keepNames=true path — resolve name and write file.
		name := synthPack.GetByID(id)
		if name == "" {
			name = fmt.Sprintf("sound_%d", id)
		}
		if synthPack.GetByID(id) == "" {
			synthPack.Register(id, name)
		}

		// TS lines 84-90: write to existing path if found, else create in synthDir.
		var outPath string
		if existing, ok := existingByName[name]; ok {
			outPath = existing
		} else {
			outPath = filepath.Join(synthDir, name+".synth")
		}
		if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
			return fmt.Errorf("sound: mkdir for %s: %w", outPath, err)
		}
		if err := os.WriteFile(outPath, data, 0o644); err != nil {
			return fmt.Errorf("sound: write %s: %w", outPath, err)
		}
	}

	// TS line 93: SynthPack.save().
	if err := synthPack.Save(); err != nil {
		return fmt.Errorf("sound: save synth pack: %w", err)
	}

	// TS line 94: writeFileSync synth.order — order.join('\n') + '\n'.
	packDir := filepath.Join(srcDir, "pack")
	if err := os.MkdirAll(packDir, 0o755); err != nil {
		return fmt.Errorf("sound: mkdir pack: %w", err)
	}
	orderStrs := make([]string, len(order))
	for i, v := range order {
		orderStrs[i] = fmt.Sprintf("%d", v)
	}
	orderContent := strings.Join(orderStrs, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(packDir, "synth.order"), []byte(orderContent), 0o644); err != nil {
		return fmt.Errorf("sound: write synth.order: %w", err)
	}

	return nil
}

// parseWave mirrors Wave.unpack instance method (TS ~101-113).
// It advances buf.Pos past the entire wave payload.
func parseWave(buf *packet.Packet) error {
	// TS lines 102-109: 10 tones.
	for range 10 {
		probe := buf.G1()
		if probe != 0 {
			buf.Pos-- // TS: buf.pos--
			if err := parseTone(buf); err != nil {
				return err
			}
		}
	}

	// TS lines 111-112: loopBegin=g2, loopEnd=g2.
	buf.G2()
	buf.G2()
	return nil
}

// parseTone mirrors Tone.unpack (TS ~133-185).
// It advances buf.Pos past the entire tone payload.
func parseTone(buf *packet.Packet) error {
	// TS lines 134-138: frequencyBase + amplitudeBase (always present).
	parseEnvelope(buf)
	parseEnvelope(buf)

	// TS lines 140-148: optional pair (frequencyModRate, frequencyModRange).
	probe := buf.G1()
	if probe != 0 {
		buf.Pos--
		parseEnvelope(buf)
		parseEnvelope(buf)
	}

	// TS lines 150-158: optional pair (amplitudeModRate, amplitudeModRange).
	probe = buf.G1()
	if probe != 0 {
		buf.Pos--
		parseEnvelope(buf)
		parseEnvelope(buf)
	}

	// TS lines 160-168: optional pair (release, attack).
	probe = buf.G1()
	if probe != 0 {
		buf.Pos--
		parseEnvelope(buf)
		parseEnvelope(buf)
	}

	// TS lines 170-179: harmonics loop 0..9.
	for range 10 {
		volume := buf.GSmartS() // gsmarts → unsigned smart
		if volume == 0 {
			break
		}
		buf.GSmart()  // harmonicSemitone — signed smart
		buf.GSmartS() // harmonicDelay — unsigned smart
	}

	// TS lines 181-184: reverbDelay, reverbVolume, length, start.
	buf.GSmartS() // reverbDelay
	buf.GSmartS() // reverbVolume
	buf.G2()      // length
	buf.G2()      // start

	// rev-274: Tone.unpack unconditionally appends a Filter (TS Unpack.ts
	// @dee467c8: this.filter.unpack(buf, this.filterRange)).
	parseFilter(buf)

	return nil
}

// parseFilter mirrors Filter.unpack (TS Unpack.ts @dee467c8, the rev-274
// addition to Tone.unpack).  It only advances buf.Pos — the goscape sound
// unpacker extracts byte extents, so it never materialises the filter values.
//
// Byte layout:
//   - count = g1.  pairs[0] = count>>4, pairs[1] = count&0xf.
//   - if count == 0: done (no further reads).
//   - else:
//   - unities[0] = g2, unities[1] = g2.
//   - migration = g1.
//   - phase 1: for direction 0,1: pairs[direction] × (g2 freq, g2 range).
//   - phase 2: for direction 0,1, pair 0..pairs[direction]-1: if
//     (migration & ((1<<(direction*4))<<pair)) != 0 → g2 freq + g2 range.
//   - if migration != 0 || unities[1] != unities[0] → unpackShape (g1 length
//     + length × (g2 shapeDelta, g2 shapePeak)).
func parseFilter(buf *packet.Packet) {
	count := int(buf.G1())
	if count == 0 {
		return
	}

	pairs := [2]int{count >> 4, count & 0xf}

	unity0 := buf.G2() // unities[0]
	unity1 := buf.G2() // unities[1]

	migration := int(buf.G1())

	// Phase 1: base frequency/range pairs.
	for direction := range 2 {
		for range pairs[direction] {
			buf.G2() // frequencies[direction][0][pair]
			buf.G2() // ranges[direction][0][pair]
		}
	}

	// Phase 2: migrated frequency/range pairs (only when the migration bit is set).
	for direction := range 2 {
		for pair := range pairs[direction] {
			if migration&((1<<(direction*4))<<pair) != 0 {
				buf.G2() // frequencies[direction][1][pair]
				buf.G2() // ranges[direction][1][pair]
			}
		}
	}

	// Trailing shape: TS emits envelope.unpackShape when the filter migrated or
	// the unity bounds differ.
	if migration != 0 || unity1 != unity0 {
		unpackShape(buf)
	}
}

// unpackShape mirrors Envelope.unpackShape (TS Unpack.ts @dee467c8): a g1 length
// followed by length × (g2 shapeDelta, g2 shapePeak).  It only advances buf.Pos.
func unpackShape(buf *packet.Packet) {
	length := buf.G1()
	for range length {
		buf.G2() // shapeDelta[i]
		buf.G2() // shapePeak[i]
	}
}

// parseEnvelope mirrors Envelope.unpack (TS ~196-212).
// It advances buf.Pos past the entire envelope payload.
func parseEnvelope(buf *packet.Packet) {
	buf.G1() // form
	buf.G4() // start (g4s — signed, but advances 4 bytes same as G4)
	buf.G4() // end   (g4s — signed, but advances 4 bytes same as G4)
	unpackShape(buf)
}
