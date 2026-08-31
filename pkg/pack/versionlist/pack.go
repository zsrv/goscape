// Package versionlist ports tools/pack/versionlist/pack.ts at Engine-TS 9aadcec4.
// It is a NEW stage introduced in rev-244 (not present in 225).
package versionlist

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/zsrv/goscape/pkg/io/filestream"
	"github.com/zsrv/goscape/pkg/io/jagfile"
	"github.com/zsrv/goscape/pkg/io/packet"
	"github.com/zsrv/goscape/pkg/pack"
)

// Pack ports TS packClientVersionList(cache, modelFlags) at 9aadcec4 (rev-244).
//
// It builds a Jagfile with 12 named members (model_version/crc/index,
// anim_version/crc/index, midi_version/crc/index, map_version/crc/index),
// saves it to <outDir>/client/versionlist, and writes the same bytes into
// cache(0, 5) — mirroring TS lines 130-132.
//
// CRC parity note: packet.GetCRC (pkg/io/packet/packet.go) uses crc32.IEEE
// (CRC-32/ISO-HDLC, poly 0xEDB88320, init 0xFFFFFFFF, final XOR 0xFFFFFFFF).
// TS Packet.getcrc (Packet.ts:54-60) loops `for (i=offset; i<length; i++)` over
// the same CRC-32 IEEE table with the same init/final — semantically identical
// for the offset=0 case used here. Both exclude the trailing 2-byte version
// field (data.length-2), so CRC is over the raw payload only.
//
// cache is REQUIRED for both the read path (archives 1-4) and the write path
// (cache(0,5)). When nil, the whole stage no-ops (consistent with graphics and
// midi; callers in pkg/packall pass nil until T15 wires the real FileStream).
//
// modelFlags is nil-safe and bounds-safe: missing entries are treated as 0,
// matching TS `modelFlags[id] ?? 0`.
//
// TS source: tools/pack/versionlist/pack.ts @ 9aadcec4 (rev-244).
func Pack(reg *pack.Registry, srcDir, outDir string, modelFlags []int, cache *filestream.FileStream) error {
	if cache == nil {
		// T15 comment: nil cache means real FileStream not yet wired; no-op.
		return nil
	}

	modelPack, err := reg.EnsureModel()
	if err != nil {
		return fmt.Errorf("versionlist.Pack: EnsureModel: %w", err)
	}
	animSetPack, err := reg.EnsureAnimSet()
	if err != nil {
		return fmt.Errorf("versionlist.Pack: EnsureAnimSet: %w", err)
	}
	animPack, err := reg.EnsureAnim()
	if err != nil {
		return fmt.Errorf("versionlist.Pack: EnsureAnim: %w", err)
	}
	midiPack, err := reg.EnsureMidi()
	if err != nil {
		return fmt.Errorf("versionlist.Pack: EnsureMidi: %w", err)
	}
	mapPack, err := reg.EnsureMap()
	if err != nil {
		return fmt.Errorf("versionlist.Pack: EnsureMap: %w", err)
	}

	// TS: Jagfile.new(true) — whole-blob bzip2, matching clientinterface flavor.
	versionlist := jagfile.NewEmptyJagfile(true)

	// ---- model_version / model_crc / model_index ----
	// TS lines 13-38: for id in [0, ModelPack.max).
	//
	// model flag meanings (TS comment block, lines 23-32):
	//   0x80 — player chatheads
	//   0x40 — item inventory models
	//   0x20 — item inventory models (f2p)
	//   0x10 — item worn models
	//   0x08 — item worn models (f2p)
	//   0x04 — npc models/chatheads + scenery of anything that is mapped down
	//   0x02 — anything that spawns dynamically (non-mapped-down npcs/scenery, spotanims, interfaces)
	//   0x01 — used on tutorial island
	modelVersion := packet.Alloc(3)
	modelCrc := packet.Alloc(4)
	modelIndex := packet.Alloc(3)
	for id := range modelPack.Max {
		data := cache.Read(1, id, false)
		if data != nil {
			modelVersion.P2(1)
			// CRC excludes the trailing 2-byte version field (TS: data.length-2).
			modelCrc.P4(packet.GetCRC(data, 0, len(data)-2))
			// nil-safe + bounds-safe: modelFlags[id] ?? 0
			flag := 0
			if modelFlags != nil && id < len(modelFlags) {
				flag = modelFlags[id]
			}
			modelIndex.P1(uint8(flag))
		} else {
			modelVersion.P2(0)
			modelCrc.P4(0)
			modelIndex.P1(0)
		}
	}
	versionlist.Write("model_version", modelVersion)
	versionlist.Write("model_crc", modelCrc)
	versionlist.Write("model_index", modelIndex)

	// ---- anim_version / anim_crc / anim_index ----
	// TS lines 43-62: AnimSetPack loop for version/crc; AnimPack loop for index.
	//
	// anim_index: for each frame id, the 1-based animset file that contains
	// it (0 = not referenced by any packed animset). Engine-TS 8139461a
	// resolved the old `// todo: i think this is each frame's animset file`
	// posture — the field really is the frame's owning animset, and it is now
	// computed by parsing each .anim source (versionlist/pack.ts:101-121
	// @1d25566c):
	//
	//	const anim = new Packet(fs.readFileSync(animFiles.get(name)!));
	//	const frameCount = anim.g2();
	//	for (let frame = 0; frame < frameCount; frame++) {
	//	    frameBase[anim.g2()] = id + 1;
	//	    anim.pos++; // group count
	//	}
	//
	// The scan runs only for animsets actually present in the cache, so a
	// frame belonging to an absent animset keeps its 0.
	animVersion := packet.Alloc(3)
	animCrc := packet.Alloc(4)
	animIndex := packet.Alloc(3)

	animFiles := map[string]string{}
	for _, f := range pack.ListFilesExt(filepath.Join(srcDir, "models"), ".anim") {
		animFiles[strings.TrimSuffix(filepath.Base(f), ".anim")] = f
	}
	frameBase := make([]uint16, animPack.Max)

	for id := range animSetPack.Max {
		data := cache.Read(2, id, false)
		if data != nil {
			animVersion.P2(1)
			animCrc.P4(packet.GetCRC(data, 0, len(data)-2))

			name := animSetPack.GetByID(id)
			path, ok := animFiles[name]
			if !ok {
				return fmt.Errorf("versionlist: animset %q (id %d) has no .anim source", name, id)
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				return fmt.Errorf("versionlist: read %s: %w", path, err)
			}
			anim := packet.NewPacket(raw)
			frameCount := int(anim.G2())
			for range frameCount {
				frame := int(anim.G2())
				if frame < 0 || frame >= len(frameBase) {
					return fmt.Errorf("versionlist: animset %q frame id %d out of range [0,%d)", name, frame, len(frameBase))
				}
				frameBase[frame] = uint16(id + 1)
				anim.Pos++ // group count
			}
		} else {
			animVersion.P2(0)
			animCrc.P4(0)
		}
	}
	for id := range animPack.Max {
		animIndex.P2(frameBase[id])
	}
	versionlist.Write("anim_version", animVersion)
	versionlist.Write("anim_crc", animCrc)
	versionlist.Write("anim_index", animIndex)

	// ---- midi_version / midi_crc / midi_index ----
	// TS lines 64-82: MidiPack.max loop.
	//
	// midi_index per present id: pbool(fileExists(<srcDir>/jingles/<name>.mid))
	// Used for prefetching jingles (TS line 72 comment).
	midiVersion := packet.Alloc(3)
	midiCrc := packet.Alloc(4)
	midiIndex := packet.Alloc(3)
	for id := range midiPack.Max {
		data := cache.Read(3, id, false)
		if data != nil {
			midiVersion.P2(1)
			midiCrc.P4(packet.GetCRC(data, 0, len(data)-2))
			// used for prefetching jingles (TS line 72)
			name := midiPack.GetByID(id)
			jinglePath := filepath.Join(srcDir, "jingles", name+".mid")
			midiIndex.PBool(pack.FileExists(jinglePath))
		} else {
			midiVersion.P2(0)
			midiCrc.P4(0)
			midiIndex.P1(0)
		}
	}
	versionlist.Write("midi_version", midiVersion)
	versionlist.Write("midi_crc", midiCrc)
	versionlist.Write("midi_index", midiIndex)

	// ---- map_version / map_crc ----
	// TS lines 84-96: MapPack.max loop.
	mapVersion := packet.Alloc(3)
	mapCrc := packet.Alloc(4)
	for id := range mapPack.Max {
		data := cache.Read(4, id, false)
		if data != nil {
			mapVersion.P2(1)
			mapCrc.P4(packet.GetCRC(data, 0, len(data)-2))
		} else {
			mapVersion.P2(0)
			mapCrc.P4(0)
		}
	}

	// ---- map_index ----
	// TS lines 98-125: parse free2play.csv, build prefetch set, emit region+pair+prefetch.
	//
	// CSV format: lines are "_y_mx_mz_lx_lz" (split by '_', map to Number).
	// Skip lines starting with "//" or empty. Build prefetch set as (mx<<8)|mz.
	// Then scan mapX 0..99, mapZ 0..254: find m<x>_<z> / l<x>_<z> in MapPack;
	// emit p2(region) p2(mapId) p2(locMapId) pbool(prefetch.has(region)).
	prefetch := map[int]bool{}
	free2playPath := filepath.Join(srcDir, "maps", "free2play.csv")
	f2pData, err := os.ReadFile(free2playPath)
	if err != nil {
		return fmt.Errorf("versionlist.Pack: read free2play.csv: %w", err)
	}
	scanner := bufio.NewScanner(strings.NewReader(strings.ReplaceAll(string(f2pData), "\r", "")))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "//") || len(line) == 0 {
			continue
		}
		parts := strings.Split(line, "_")
		if len(parts) < 3 {
			continue
		}
		mx, errMx := strconv.Atoi(parts[1])
		mz, errMz := strconv.Atoi(parts[2])
		if errMx != nil || errMz != nil {
			continue
		}
		prefetch[(mx<<8)|mz] = true
	}

	mapIndex := packet.Alloc(4)
	for mapX := range 100 {
		for mapZ := range 255 {
			mapId := mapPack.GetByName(fmt.Sprintf("m%d_%d", mapX, mapZ))
			if mapId == -1 {
				continue
			}
			locMapId := mapPack.GetByName(fmt.Sprintf("l%d_%d", mapX, mapZ))
			region := (mapX << 8) | mapZ
			mapIndex.P2(uint16(region))
			mapIndex.P2(uint16(mapId))
			mapIndex.P2(uint16(locMapId))
			mapIndex.PBool(prefetch[region])
		}
	}
	versionlist.Write("map_version", mapVersion)
	versionlist.Write("map_crc", mapCrc)
	versionlist.Write("map_index", mapIndex)

	// TS line 130: versionlist.save('data/pack/client/versionlist')
	clientOut := filepath.Join(outDir, "client", "versionlist")
	if err := os.MkdirAll(filepath.Dir(clientOut), 0o755); err != nil {
		return fmt.Errorf("versionlist.Pack: mkdir: %w", err)
	}
	if err := versionlist.Save(clientOut); err != nil {
		return fmt.Errorf("versionlist.Pack: save jag: %w", err)
	}

	// TS line 132: cache.write(0, 5, fs.readFileSync('data/pack/client/versionlist'))
	jagBytes, err := os.ReadFile(clientOut)
	if err != nil {
		return fmt.Errorf("versionlist.Pack: read jag for cache: %w", err)
	}
	cache.Write(0, 5, jagBytes, 0)

	return nil
}
