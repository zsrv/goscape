// Package maps ports tools/pack/map/Pack.js's packMaps entrypoint.
package maps

import (
	"cmp"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/zsrv/goscape/pkg/io/jagfile"
	"github.com/zsrv/goscape/pkg/io/packet"
	"github.com/zsrv/goscape/pkg/pack"
)

// Pack ports TS map/Pack.js:packMaps.
//
// Walks <srcDir>/maps/*.jm2; for each, parses the four sections,
// encodes the land/loc/npc/obj streams, writes:
//
//	<outDir>/client/maps/m<XZ>  — bzip2 land
//	<outDir>/client/maps/l<XZ>  — bzip2 loc
//	<outDir>/server/maps/m<XZ>  — raw land
//	<outDir>/server/maps/l<XZ>  — raw loc
//	<outDir>/server/maps/n<XZ>  — raw npc
//	<outDir>/server/maps/o<XZ>  — raw obj
//
// NAI-213-D-PACKMAPS-PRINTWARN-LOG: TS uses printWarning for malformed
// lines; goscape silently skips them (parser-side guards). Permanent.
//
// NAI-213-D-PACKMAPS-DETERMINISTIC-ORDER: TS JS Map preserves insertion
// order so NPC/OBJ writers emit entries in readMap line order. Go's
// map iteration order is randomized — sort keys ascending instead for
// deterministic byte output. Permanent.
func Pack(srcDir, outDir string) error {
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

		if !pack.ShouldBuildFile(file, clientMap) {
			continue
		}

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

		if err := writeLand(m, clientMap, serverMap); err != nil {
			return err
		}
		if err := writeLocs(m, clientLoc, serverLoc); err != nil {
			return err
		}
		if err := writeNpcs(m, serverNpc); err != nil {
			return err
		}
		if err := writeObjs(m, serverObj); err != nil {
			return err
		}
	}
	return nil
}

const tileStride = 4 * 64 * 64

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
	compressed, err := jagfile.BZip2Compress(raw, true, false, 1, 0)
	if err != nil {
		return err
	}
	if err := os.WriteFile(clientPath, compressed, 0o644); err != nil {
		return err
	}
	return os.WriteFile(serverPath, raw, 0o644)
}

type locRecord struct {
	ID, Level, X, Z, Shape, Angle int
}

func writeLocs(m mapData, clientPath, serverPath string) error {
	list := []locRecord{}
	for key, entries := range m.Loc {
		level := (key >> 12) & 0x3
		x := (key >> 6) & 0x3f
		z := key & 0x3f
		for _, e := range entries {
			list = append(list, locRecord{ID: e.ID, Level: level, X: x, Z: z, Shape: e.Shape, Angle: e.Angle})
		}
	}
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
	compressed, err := jagfile.BZip2Compress(raw, true, false, 1, 0)
	if err != nil {
		return err
	}
	if err := os.WriteFile(clientPath, compressed, 0o644); err != nil {
		return err
	}
	return os.WriteFile(serverPath, raw, 0o644)
}

func writeNpcs(m mapData, path string) error {
	out := packet.Alloc(1)
	defer out.Release()
	keys := make([]int, 0, len(m.Npc))
	for k := range m.Npc {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, key := range keys {
		ids := m.Npc[key]
		out.P2(uint16(key))
		out.P1(uint8(len(ids)))
		for _, id := range ids {
			out.P2(uint16(id))
		}
	}
	return os.WriteFile(path, out.Data, 0o644)
}

func writeObjs(m mapData, path string) error {
	out := packet.Alloc(1)
	defer out.Release()
	keys := make([]int, 0, len(m.Obj))
	for k := range m.Obj {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, key := range keys {
		objs := m.Obj[key]
		out.P2(uint16(key))
		out.P1(uint8(len(objs)))
		for _, o := range objs {
			out.P2(uint16(o.ID))
			out.P1(uint8(o.Count))
		}
	}
	return os.WriteFile(path, out.Data, 0o644)
}
