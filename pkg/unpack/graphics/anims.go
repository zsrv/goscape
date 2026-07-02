// anims.go implements the anim-set unpack entry point that mirrors
// TS tools/unpack/graphics/UnpackAnims.ts (117 lines, Engine-TS 9aadcec4).
//
// # Decompress flag
//
// TS line 21: cache.read(2, baseId, true) — the third argument sets
// decompress=true, so Go passes Read(2, baseId, true).
//
// # Output path
//
// TS line 29: fs.writeFileSync(`${BUILD_SRC_DIR}/models/${setName}.anim`, set)
// — the raw decompressed bytes are written directly to <srcDir>/models/
// (not _unpack), named anim_<baseId>.anim.
//
// # Offset table (TS lines 31-53)
//
// The last 8 bytes of the raw set buffer hold four g2 values that encode
// section lengths:
//
//	offsets.pos = set.length - 8   (seek to last 8 bytes)
//	head.pos   = 0;             offset += offsets.g2() + 2
//	tran1.pos  = offset;        offset += offsets.g2()
//	tran2.pos  = offset;        offset += offsets.g2()
//	del.pos    = offset;        offset += offsets.g2()
//	base.pos   = offset
//
// The +2 on the first section accounts for the frameCount g2 that
// immediately precedes the per-frame records in head.
//
// # Base-skeleton walk (TS lines 55-66)
//
// base.g1() → length (joint count)
// skip length joint-type bytes (base.g1() per joint)
// skip per-joint labelCount bytes (base.g1() labelCount; base.g1() × labelCount)
//
// # Frame-table walk (TS lines 78-112)
//
// frameCount = head.g2()
// per frame:
//   frameId   = head.g2()
//   del.g1()  (delay — consumed but not used)
//   if !FramePack.getById(frameId) → register(frameId, `anim_${frameId}`)
//   frameName = FramePack.getById(frameId)
//   existingFrame = find(endsWith `/${frameName}.frame`)
//   if existingFrame → fs.unlinkSync(existingFrame)  (delete stale file)
//   labelCount = head.g1()
//   per label j:
//     flags = tran1.g1()
//     if flags == 0 → continue
//     if flags & 0x1 → tran2.gsmart()
//     if flags & 0x2 → tran2.gsmart()
//     if flags & 0x4 → tran2.gsmart()
//
// # Base-file stale-delete (TS lines 68-76)
//
// After the base walk (before frameCount), if BasePack has no entry for
// baseId, register(baseId, `base_${baseId}`); resolve baseName; find and
// delete the existing .base file if present.
//
// # Save order (TS lines 115-117)
//
// AnimSetPack.save(); BasePack.save(); FramePack.save() — in that order.
//
// TS source: tools/unpack/graphics/UnpackAnims.ts.

package graphics

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/zsrv/goscape/pkg/io/filestream"
	"github.com/zsrv/goscape/pkg/io/packet"
	"github.com/zsrv/goscape/pkg/pack"
)

// Anims is the top-level entry point for the anim-set unpack.
// It mirrors TS tools/unpack/graphics/UnpackAnims.ts.
//
// The function:
//  1. Opens the FileStream cache and iterates cache.count(2) sets.
//  2. For each set: reads+decompresses the raw bytes; registers in AnimSetPack;
//     writes <srcDir>/models/anim_<baseId>.anim.
//  3. Parses the offset table from the last 8 bytes; walks the base skeleton
//     and registers/deletes stale .base files; walks the frame table and
//     registers/deletes stale .frame files (consuming tran2 smarts per flags).
//  4. Saves AnimSetPack, BasePack, FramePack in that order.
//
// TS source: tools/unpack/graphics/UnpackAnims.ts.
func Anims(opts Options) error {
	printWarning := func(msg string) {
		if opts.Out != nil {
			fmt.Fprintf(opts.Out, "%s\n", msg)
		}
	}

	// TS line 14: new FileStream('data/unpack').
	cache, err := filestream.New(opts.CacheDir, false, true)
	if err != nil {
		return fmt.Errorf("graphics/anims: open cache: %w", err)
	}
	defer cache.Close()

	modelsDir := filepath.Join(opts.SrcDir, "models")
	if err := os.MkdirAll(modelsDir, 0o755); err != nil {
		return fmt.Errorf("graphics/anims: mkdir models: %w", err)
	}

	// Collect existing .base and .frame files; first-wins on duplicate basenames.
	existingBases, err := listFilesExtFirstWins(modelsDir, ".base")
	if err != nil {
		return fmt.Errorf("graphics/anims: list existing .base: %w", err)
	}
	existingFrames, err := listFilesExtFirstWins(modelsDir, ".frame")
	if err != nil {
		return fmt.Errorf("graphics/anims: list existing .frame: %w", err)
	}

	// Load pack registries.
	reg := &pack.Registry{SrcDir: opts.SrcDir}
	animSetPack, err := reg.EnsureAnimSet()
	if err != nil {
		return fmt.Errorf("graphics/anims: ensure animset pack: %w", err)
	}
	basePack, err := reg.EnsureBase()
	if err != nil {
		return fmt.Errorf("graphics/anims: ensure base pack: %w", err)
	}
	// TS: FramePack = new PackFile('anim', ...) — pack type is "anim".
	framePack, err := reg.EnsureAnim()
	if err != nil {
		return fmt.Errorf("graphics/anims: ensure anim (frame) pack: %w", err)
	}

	// TS line 19: baseCount = cache.count(2).
	baseCount := cache.Count(2)

	for baseId := range baseCount {
		// TS line 21: cache.read(2, baseId, true) — decompress=true.
		set := cache.Read(2, baseId, true)
		if set == nil {
			// TS line 23-25: printWarning + continue.
			printWarning(fmt.Sprintf("Missing anim set %d", baseId))
			continue
		}

		// Go-side guard with no TS counterpart: a decompressed set shorter
		// than the 8-byte offset table would panic the section walker below
		// (TS NaN-reads through it instead). Real 244 sets are never this
		// short; bail with a warning rather than crash on corrupt data.
		if len(set) < 8 {
			printWarning(fmt.Sprintf("Truncated anim set %d (len=%d)", baseId, len(set)))
			continue
		}

		// TS line 27-29: setName + AnimSetPack.register + write .anim file.
		setName := fmt.Sprintf("anim_%d", baseId)
		animSetPack.Register(baseId, setName)

		animPath := filepath.Join(modelsDir, setName+".anim")
		if err := os.WriteFile(animPath, set, 0o644); err != nil {
			return fmt.Errorf("graphics/anims: write %s: %w", animPath, err)
		}

		// TS lines 31-53: build section packets from the offset table at
		// the last 8 bytes of the raw buffer.
		//
		// All five Packet instances share the same underlying byte slice.
		// Pos is set independently on each to point at its section start.
		//
		// offsets.pos = set.length - 8
		offsets := packet.NewPacket(set)
		offsets.Pos = len(set) - 8

		head := packet.NewPacket(set)
		tran1 := packet.NewPacket(set)
		tran2 := packet.NewPacket(set)
		del := packet.NewPacket(set)
		base := packet.NewPacket(set)

		var offset int

		// head section starts at 0; length = g2() + 2 (the +2 accounts for
		// the frameCount g2 at the very start of the head section).
		head.Pos = offset
		offset += int(offsets.G2()) + 2

		// tran1 section.
		tran1.Pos = offset
		offset += int(offsets.G2())

		// tran2 section.
		tran2.Pos = offset
		offset += int(offsets.G2())

		// del section.
		del.Pos = offset
		offset += int(offsets.G2())

		// base section starts at the end of del.
		base.Pos = offset

		// TS lines 55-66: walk base skeleton (advances base.Pos).
		length := int(base.G1()) // joint count
		for range length {
			base.G1() // joint type — skip
		}
		for range length {
			labelCount := int(base.G1())
			for range labelCount {
				base.G1() // label — skip
			}
		}

		// TS lines 68-76: BasePack registration + stale .base deletion.
		if basePack.GetByID(baseId) == "" {
			basePack.Register(baseId, fmt.Sprintf("base_%d", baseId))
		}
		baseName := basePack.GetByID(baseId)

		if existingBase, ok := existingBases[baseName]; ok {
			if err := os.Remove(existingBase); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("graphics/anims: remove stale base %s: %w", existingBase, err)
			}
			delete(existingBases, baseName) // deleted (or gone) — drop the map entry
		}

		// TS lines 78-112: frame-table walk.
		frameCount := int(head.G2())
		for range frameCount {
			frameId := int(head.G2())
			del.G1() // delay — consume but discard

			// TS lines 83-86: FramePack registration.
			if framePack.GetByID(frameId) == "" {
				framePack.Register(frameId, fmt.Sprintf("anim_%d", frameId))
			}
			frameName := framePack.GetByID(frameId)

			// TS lines 88-91: stale .frame deletion.
			if existingFrame, ok := existingFrames[frameName]; ok {
				if err := os.Remove(existingFrame); err != nil && !os.IsNotExist(err) {
					return fmt.Errorf("graphics/anims: remove stale frame %s: %w", existingFrame, err)
				}
				delete(existingFrames, frameName) // deleted (or gone) — drop the map entry
			}

			// TS lines 93-111: per-label tran1/tran2 consumption.
			labelCount := int(head.G1())
			for range labelCount {
				flags := tran1.G1()
				if flags == 0 {
					continue
				}
				if flags&0x1 != 0 {
					tran2.GSmart()
				}
				if flags&0x2 != 0 {
					tran2.GSmart()
				}
				if flags&0x4 != 0 {
					tran2.GSmart()
				}
			}
		}
	}

	// TS lines 115-117: save in order AnimSetPack, BasePack, FramePack.
	if err := animSetPack.Save(); err != nil {
		return fmt.Errorf("graphics/anims: save animset pack: %w", err)
	}
	if err := basePack.Save(); err != nil {
		return fmt.Errorf("graphics/anims: save base pack: %w", err)
	}
	if err := framePack.Save(); err != nil {
		return fmt.Errorf("graphics/anims: save anim (frame) pack: %w", err)
	}

	return nil
}
