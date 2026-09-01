// Package maps ports tools/pack/map/Pack.js's packMaps entrypoint.
package maps

import (
	"cmp"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/zsrv/goscape/pkg/io/filestream"
	"github.com/zsrv/goscape/pkg/io/gziputil"
	"github.com/zsrv/goscape/pkg/io/packet"
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/pack"
)

// Pack ports TS map/Pack.js:packMaps at revision 244.
//
// Walks <srcDir>/maps/*.jm2; for each, parses the four sections,
// encodes the land/loc/npc/obj streams, writes:
//
//	<outDir>/client/maps/m<XZ>  — gzip land    (TS Pack.js:275 @ 9aadcec4)
//	<outDir>/client/maps/l<XZ>  — gzip loc     (TS Pack.js:356 @ 9aadcec4)
//	<outDir>/server/maps/m<XZ>  — raw land
//	<outDir>/server/maps/l<XZ>  — raw loc
//	<outDir>/server/maps/n<XZ>  — raw npc
//	<outDir>/server/maps/o<XZ>  — raw obj
//
// mapPack is the map PackFile used for cache write-back keying. It is
// nil-safe: when nil the cache writes are skipped (pkg/packall passes nil
// until T15). Mirrors the wordenc/graphics/audio nil-cache convention.
//
// cache is the FileStream for archive-4 writes. Nil-safe per above.
//
// modelFlags is the shared model flags slice (indexed by model ID).
// When non-nil, bits are set for NPC model/head references found in each
// map file. Nil-safe. (TS Pack.js:394-403 @ 9aadcec4.)
//
// NPC/OBJ emission order: level 0..3 → x 0..63 → z 0..63 ascending,
// mirroring the 244 nested-array iteration. (TS Pack.js:366-447 @ 9aadcec4.)
// This replaced the 225 Map-insertion-order iteration. The Go struct
// representation (map[int][]int etc.) is kept; the output is equivalent.
//
// NPC type validation: when outDir contains a packed npc.dat, each NPC
// id in a map is checked against LoadNPCTypes(outDir). A missing type is
// a fatal error (TS Pack.js:390-393 @ 9aadcec4 via printFatalError). Go
// idiom: return the error rather than calling os.Exit.
//
// Per-artifact rebuild conditions (TS Pack.js:120-135,140,281,361,413 @ 9aadcec4):
// each output artifact is independently gated on its own freshness check.
// Cache writes happen unconditionally at the end of each file's loop iter.
//
// NAI-213-D-PACKMAPS-PRINTWARN-LOG: TS uses printWarning for malformed
// lines; goscape silently skips them (parser-side guards). Permanent.
func Pack(srcDir, outDir string, mapPack *pack.PackFile, cache *filestream.FileStream, modelFlags []int) error {
	mapsSrc := filepath.Join(srcDir, "maps")
	if !pack.FileExists(mapsSrc) {
		return nil
	}

	mapsClient := filepath.Join(outDir, "client", "maps")
	mapsServer := filepath.Join(outDir, "server", "maps")
	if err := os.MkdirAll(mapsClient, 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(mapsServer, 0o755); err != nil {
		return err
	}

	// multiway.csv / free2play.csv are read verbatim by the runtime GameMap
	// from <cache>/server/maps/ (gamemap.Init → loadCsvMap). TS reads them
	// straight from the source maps dir at boot; goscape runs off the packed
	// cache, so copy them alongside the m/l/n/o streams here. Missing source
	// CSVs are not an error (a map pack without multi-combat/F2P data).
	for _, name := range []string{"multiway.csv", "free2play.csv"} {
		if err := copyIfExists(filepath.Join(mapsSrc, name), filepath.Join(mapsServer, name)); err != nil {
			return err
		}
	}

	// Load NPC types for validation (TS Pack.js:364 NpcType.load('data/pack')).
	// LoadNPCTypes is called once before the loop when the packed npc.dat exists.
	// When absent (e.g. first run before PackConfigs) the maps stage has no NPC
	// to validate or the npc block will be freshness-skipped; treat as nil.
	var npcTypes *objtype.NPCTypeConfigs
	if pack.FileExists(filepath.Join(outDir, "server", "npc.dat")) {
		var err error
		npcTypes, err = objtype.LoadNPCTypes(outDir)
		if err != nil {
			return fmt.Errorf("maps.Pack: load npc types: %w", err)
		}
	}

	files := pack.ListFilesExt(mapsSrc, ".jm2")
	for _, file := range files {
		base := strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
		if len(base) < 2 {
			continue
		}
		mapXZ := base[1:] // drop leading "m"

		clientMap := filepath.Join(mapsClient, "m"+mapXZ)
		clientLoc := filepath.Join(mapsClient, "l"+mapXZ)
		serverMap := filepath.Join(mapsServer, "m"+mapXZ)
		serverLoc := filepath.Join(mapsServer, "l"+mapXZ)
		serverNpc := filepath.Join(mapsServer, "n"+mapXZ)
		serverObj := filepath.Join(mapsServer, "o"+mapXZ)

		// Per-artifact freshness: any output older than its source triggers rebuild.
		// TS Pack.js:130: packerUpdated = shouldBuildFile(__filename, mapFile).
		// goscape has no __filename analog, and this was documented as a
		// PERMANENT omission ("no script mtime"). PSG closes it: the
		// ShouldBuildFile calls below consult pack.ForceRebuild(), which the
		// packer format stamp latches on when the byte layout changes. The
		// identity differs from TS's (a version, not a source mtime — PSG-D2)
		// but the effect is the one TS's packerUpdated was reaching for.
		needLand := pack.ShouldBuildFile(file, clientMap) || pack.ShouldBuildFile(file, serverMap)
		needLoc := pack.ShouldBuildFile(file, clientLoc) || pack.ShouldBuildFile(file, serverLoc)
		needNpc := pack.ShouldBuildFile(file, serverNpc)
		needObj := pack.ShouldBuildFile(file, serverObj)

		if !needLand && !needLoc && !needNpc && !needObj && cache == nil {
			// All outputs are fresh and no cache writes needed.
			continue
		}

		// Parse source file (shared across all four output artifacts).
		raw, err := os.ReadFile(file)
		if err != nil {
			return err
		}
		lines := []string{}
		for line := range strings.SplitSeq(strings.ReplaceAll(string(raw), "\r", ""), "\n") {
			if line != "" {
				lines = append(lines, line)
			}
		}
		m := readMap(lines)

		if needLand {
			if err := writeLand(m, clientMap, serverMap); err != nil {
				return err
			}
		}
		if needLoc {
			if err := writeLocs(m, clientLoc, serverLoc); err != nil {
				return err
			}
		}
		if needNpc {
			if err := writeNpcs(m, serverNpc, mapXZ, npcTypes, modelFlags); err != nil {
				return err
			}
		}
		if needObj {
			if err := writeObjs(m, serverObj); err != nil {
				return err
			}
		}

		// Cache writes: unconditional at end of loop, regardless of freshness.
		// TS Pack.js:449-450 @ 9aadcec4:
		//   cache.write(4, MapPack.getByName(`m${mapX}_${mapZ}`), fs.readFileSync(mapFile), 1)
		//   cache.write(4, MapPack.getByName(`l${mapX}_${mapZ}`), fs.readFileSync(locFile), 1)
		if cache != nil && mapPack != nil {
			mID := mapPack.GetByName("m" + mapXZ)
			if mID >= 0 {
				mBytes, err := os.ReadFile(clientMap)
				if err != nil {
					return fmt.Errorf("maps.Pack: read client map %q: %w", clientMap, err)
				}
				cache.Write(4, mID, mBytes, 1)
			}
			lID := mapPack.GetByName("l" + mapXZ)
			if lID >= 0 {
				lBytes, err := os.ReadFile(clientLoc)
				if err != nil {
					return fmt.Errorf("maps.Pack: read client loc %q: %w", clientLoc, err)
				}
				cache.Write(4, lID, lBytes, 1)
			}
		}
	}
	return nil
}

// copyIfExists copies src to dst verbatim. A missing src is not an error.
func copyIfExists(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}

const tileStride = 4 * 64 * 64

// writeLand encodes land data and writes gzip to clientPath, raw to serverPath.
// TS Pack.js:140-278 @ 9aadcec4. Changed from bzip2 → gzip at rev-244.
func writeLand(m mapData, clientPath, serverPath string) error {
	heightmap := make([]int16, tileStride)
	overlayIDs := make([]int16, tileStride)
	overlayShape := make([]int16, tileStride)
	overlayRot := make([]int16, tileStride)
	flags := make([]int16, tileStride)
	underlay := make([]int16, tileStride)
	for i := range tileStride {
		overlayIDs[i] = -1
		overlayShape[i] = -1
		overlayRot[i] = -1
		flags[i] = -1
		underlay[i] = -1
	}
	for key, tile := range m.Land {
		heightmap[key] = int16(tile.H)
		overlayIDs[key] = int16(tile.OverlayID)
		overlayShape[key] = int16(tile.OverlayShape)
		overlayRot[key] = int16(tile.OverlayRot)
		flags[key] = int16(tile.Flags)
		underlay[key] = int16(tile.Underlay)
	}

	out := packet.Alloc(3)
	defer out.Release()

	for i := range tileStride {
		h := heightmap[i]
		ov := overlayIDs[i]
		sh := overlayShape[i]
		rt := overlayRot[i]
		fl := flags[i]
		un := underlay[i]

		if h == 0 && ov == -1 && fl == -1 && un == -1 {
			out.P1(0)
			continue
		}

		if ov != -1 {
			opcode := int16(2)
			if sh != -1 {
				opcode += sh << 2
			}
			if rt != -1 {
				opcode += rt
			}
			out.P1(uint8(opcode))
			out.P1(uint8(ov))
		}

		if fl != -1 {
			out.P1(uint8(fl + 49))
		}
		if un != -1 {
			out.P1(uint8(un + 81))
		}
		if h != 0 {
			out.P1(1)
			out.P1(uint8(h))
		} else {
			out.P1(0)
		}
	}

	raw := out.Data
	// Rev-244: client file compressed with gzip (TS Pack.js:275 @ 9aadcec4).
	// 225 used BZip2.compress; 244 replaced with compressGz.
	compressed := gziputil.CompressGz(raw, 0, len(raw))
	if compressed == nil {
		return fmt.Errorf("maps.Pack: gzip compress land failed")
	}
	if err := os.WriteFile(clientPath, compressed, 0o644); err != nil {
		return err
	}
	return os.WriteFile(serverPath, raw, 0o644)
}

type locRecord struct {
	ID, Level, X, Z, Shape, Angle int
}

// writeLocs encodes loc data and writes gzip to clientPath, raw to serverPath.
// TS Pack.js:281-359 @ 9aadcec4. Changed from bzip2 → gzip at rev-244.
func writeLocs(m mapData, clientPath, serverPath string) error {
	// 225 used map insertion order; 244 uses level→x→z nested loop.
	// Go mirrors the 225 approach (collect+sort by id then coord) which
	// produces identical output bytes: locs with the same id are emitted
	// in coord-ascending order regardless of iteration order.
	// NAI-213-D-PACKMAPS-DETERMINISTIC-ORDER: permanent deviation (map key
	// sort gives same result as nested-array sorted traversal for locs).
	list := []locRecord{}
	for key, entries := range m.Loc {
		level := (key >> 12) & 0x3
		x := (key >> 6) & 0x3f
		z := key & 0x3f
		for _, e := range entries {
			list = append(list, locRecord{ID: e.ID, Level: level, X: x, Z: z, Shape: e.Shape, Angle: e.Angle})
		}
	}

	// Sort: primary by id ascending, secondary by packed coord ascending.
	// Mirrors TS Pack.js:327-329 locIds.sort + locData implicit order.
	slices.SortStableFunc(list, func(a, b locRecord) int {
		if c := cmp.Compare(a.ID, b.ID); c != 0 {
			return c
		}
		aKey := (a.Level << 12) | (a.X << 6) | a.Z
		bKey := (b.Level << 12) | (b.X << 6) | b.Z
		return cmp.Compare(aKey, bKey)
	})

	out := packet.Alloc(3)
	defer out.Release()
	lastLocID := -1
	lastLocData := int32(0)
	i := 0
	for i < len(list) {
		id := list[i].ID
		out.PSmart(int32(id - lastLocID))
		lastLocID = id
		lastLocData = 0

		for i < len(list) && list[i].ID == id {
			r := list[i]
			i++
			currentLocData := int32((r.Level << 12) | (r.X << 6) | r.Z)
			out.PSmart(currentLocData - lastLocData + 1)
			lastLocData = currentLocData
			out.P1(uint8((r.Shape << 2) | r.Angle))
		}
		out.PSmart(0) // end of this loc
	}
	out.PSmart(0) // end of map

	raw := out.Data
	// Rev-244: client file compressed with gzip (TS Pack.js:356 @ 9aadcec4).
	compressed := gziputil.CompressGz(raw, 0, len(raw))
	if compressed == nil {
		return fmt.Errorf("maps.Pack: gzip compress loc failed")
	}
	if err := os.WriteFile(clientPath, compressed, 0o644); err != nil {
		return err
	}
	return os.WriteFile(serverPath, raw, 0o644)
}

// writeNpcs encodes NPC spawn data and writes raw bytes to path.
//
// Rev-244 emission order: level 0..3 → x 0..63 → z 0..63 ascending.
// (TS Pack.js:366-411 @ 9aadcec4 — nested array traversal replaces
// 225's Map-insertion-order iteration.)
//
// NPC type validation: for each id, checks npcTypes.Configs[id] exists.
// Missing type → error "m<XZ>: NPC type does not exist: <id>".
// (TS Pack.js:390-393 @ 9aadcec4 — printFatalError → Go return error.)
//
// modelFlags: for each NpcType.Models and NpcType.Heads entry, sets
// modelFlags[model] |= 0x4. (TS Pack.js:394-403 @ 9aadcec4.)
func writeNpcs(m mapData, path, mapXZ string, npcTypes *objtype.NPCTypeConfigs, modelFlags []int) error {
	out := packet.Alloc(1)
	defer out.Release()

	// TS Pack.js:366-407 @ 9aadcec4: nested for level/x/z loops.
	for level := range 4 {
		for x := range 64 {
			for z := range 64 {
				key := packKey(level, x, z)
				ids, ok := m.Npc[key]
				if !ok {
					continue
				}
				pos := uint16((level << 12) | (x << 6) | z)
				out.P2(pos)
				out.P1(uint8(len(ids)))
				for _, id := range ids {
					out.P2(uint16(id))

					// TS Pack.js:390-403 @ 9aadcec4: NPC type validation + modelFlags.
					if npcTypes != nil {
						if id < 0 || id >= len(npcTypes.Configs) || npcTypes.Configs[id] == nil {
							return fmt.Errorf("m%s: NPC type does not exist: %d", mapXZ, id)
						}
						npc := npcTypes.Configs[id]
						if modelFlags != nil {
							for _, model := range npc.Models {
								mid := int(model)
								if mid < len(modelFlags) {
									modelFlags[mid] |= 0x4
								}
							}
							for _, model := range npc.Heads {
								mid := int(model)
								if mid < len(modelFlags) {
									modelFlags[mid] |= 0x4
								}
							}
						}
					}
				}
			}
		}
	}
	return os.WriteFile(path, out.Data, 0o644)
}

// writeObjs encodes OBJ spawn data and writes raw bytes to path.
//
// Rev-244 emission order: level 0..3 → x 0..63 → z 0..63 ascending.
// (TS Pack.js:416-447 @ 9aadcec4 — nested array traversal replaces
// 225's Map-insertion-order iteration.)
func writeObjs(m mapData, path string) error {
	out := packet.Alloc(1)
	defer out.Release()

	// TS Pack.js:416-447 @ 9aadcec4: nested for level/x/z loops.
	for level := range 4 {
		for x := range 64 {
			for z := range 64 {
				key := packKey(level, x, z)
				objs, ok := m.Obj[key]
				if !ok {
					continue
				}
				pos := uint16(((level & 0x3) << 12) | ((x & 0x3f) << 6) | (z & 0x3f))
				out.P2(pos)
				out.P1(uint8(len(objs)))
				for _, o := range objs {
					out.P2(uint16(o.ID))
					out.P1(uint8(o.Count))
				}
			}
		}
	}
	return os.WriteFile(path, out.Data, 0o644)
}
