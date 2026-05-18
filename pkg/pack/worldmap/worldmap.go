package worldmap

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/zsrv/goscape/pkg/coordgrid"
	"github.com/zsrv/goscape/pkg/io/jagfile"
	packet2 "github.com/zsrv/goscape/pkg/io/packet"
	"github.com/zsrv/goscape/pkg/objtype"
	pf "github.com/zsrv/goscape/pkg/pathfinder/loc"
	"github.com/zsrv/goscape/pkg/pixpack"
)

// Pack builds outDir/mapview/worldmap.jag from server-side map
// outputs (outDir/server/maps/{m,l,o,n}*) plus fonts/sprites/CSVs
// in srcDir. Returns nil if outDir/server/maps is missing (TS
// parity with Worldmap.ts:31-33).
//
// Tag NAI-WORLDMAP-D-READDIR-SORTED: os.ReadDir returns lexically
// sorted entries; TS fs.readdirSync is filesystem-order. The per-
// (mx,mz) data blocks in the seven mapsquare-output entries
// (underlay/overlay/loc/obj/npc/multi/free) are concatenated in
// iteration order, so reordering changes bzip2 output bytes even
// though the total uncompressed size and decoded data are unchanged.
// Goscape uses sorted order for cross-machine determinism. Sample
// canonical RS jags (openrs2 #33025, #33160, #33021) and Engine-TS's
// own pack output are each in different ad-hoc orders (historical
// build add-order or filesystem-baked), with no portable algorithmic
// rule, so byte-pin against any external reference is intrinsically
// fragile. Sorted iteration is the defensible permanent choice.
func Pack(srcDir, outDir string) error {
	mapsDir := filepath.Join(outDir, "server", "maps")
	if _, err := os.Stat(mapsDir); errors.Is(err, fs.ErrNotExist) {
		return nil
	} else if err != nil {
		return fmt.Errorf("stat %s: %w", mapsDir, err)
	}

	lg := slog.Default().With("pack", "worldmap")

	flo, err := objtype.LoadFloTypes(outDir)
	if err != nil {
		return fmt.Errorf("LoadFloTypes: %w", err)
	}
	locTypes, err := objtype.LoadLocTypes(outDir)
	if err != nil {
		return fmt.Errorf("LoadLocTypes: %w", err)
	}
	npcTypes, err := objtype.LoadNPCTypes(outDir)
	if err != nil {
		return fmt.Errorf("LoadNPCTypes: %w", err)
	}

	readCsv := func(name string) ([]string, error) {
		path := filepath.Join(srcDir, "maps", name)
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", path, err)
		}
		return strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n"), nil
	}

	multilines, err := readCsv("multiway.csv")
	if err != nil {
		return err
	}
	multimap := processCsv(multilines, "multiway", lg)

	freeLines, err := readCsv("free2play.csv")
	if err != nil {
		return err
	}
	freemap := processCsv(freeLines, "free", lg)

	ignoreLines, err := readCsv("ignore.csv")
	if err != nil {
		return err
	}
	ignoremap := processCsv(ignoreLines, "ignore", lg)

	ctx := mapCtx{
		flo:      flo,
		locTypes: locTypes,
		npcTypes: npcTypes,
		multimap: multimap,
		freemap:  freemap,
	}
	out := newMapPackets()
	defer out.release()

	entries, err := os.ReadDir(mapsDir)
	if err != nil {
		return fmt.Errorf("readdir %s: %w", mapsDir, err)
	}
	for _, ent := range entries {
		name := ent.Name()
		if !strings.HasPrefix(name, "m") {
			continue
		}
		parts := strings.Split(name[1:], "_")
		if len(parts) != 2 {
			continue
		}
		mx, err1 := strconv.Atoi(parts[0])
		mz, err2 := strconv.Atoi(parts[1])
		if err1 != nil || err2 != nil {
			continue
		}
		if _, skip := ignoremap[coordgrid.PackCoord(0, mx<<6, mz<<6)]; skip {
			continue
		}
		if err := func() error {
			land, err := packet2.Load(filepath.Join(mapsDir, name), false)
			if err != nil {
				return fmt.Errorf("load %s: %w", name, err)
			}
			defer land.Release()
			loc, err := packet2.Load(filepath.Join(mapsDir, fmt.Sprintf("l%d_%d", mx, mz)), false)
			if err != nil {
				return fmt.Errorf("load l%d_%d: %w", mx, mz, err)
			}
			defer loc.Release()
			obj, err := packet2.Load(filepath.Join(mapsDir, fmt.Sprintf("o%d_%d", mx, mz)), false)
			if err != nil {
				return fmt.Errorf("load o%d_%d: %w", mx, mz, err)
			}
			defer obj.Release()
			npc, err := packet2.Load(filepath.Join(mapsDir, fmt.Sprintf("n%d_%d", mx, mz)), false)
			if err != nil {
				return fmt.Errorf("load n%d_%d: %w", mx, mz, err)
			}
			defer npc.Release()
			if err := processMap(ctx, out, mx, mz, land, loc, obj, npc); err != nil {
				return fmt.Errorf("processMap %d,%d: %w", mx, mz, err)
			}
			return nil
		}(); err != nil {
			return err
		}
	}

	// Hardcoded water tiles (TS:513-528).
	for _, mxmz := range [][2]int{
		{39, 56}, {40, 56},
		{42, 44}, {42, 45}, {42, 46}, {42, 47}, {42, 48},
		{43, 44}, {44, 44}, {45, 44}, {46, 44}, {47, 44},
		{47, 45}, {47, 46}, {48, 45}, {48, 46},
	} {
		packWater(flo, out.underlay, out.overlay, mxmz[0], mxmz[1])
	}

	// floorcol
	if len(flo.Configs) > len(refColors) {
		return fmt.Errorf("floorcol: flo.Configs has %d entries but refColors only covers %d; update refcolors.go to add the new rows in TS Worldmap.ts:534-612 order", len(flo.Configs), len(refColors))
	}
	floorcol := packet2.Alloc(1)
	defer floorcol.Release()
	floorcol.P2(uint16(len(flo.Configs)))
	for i := range len(flo.Configs) {
		floorcol.P4(refColors[i][0])
		floorcol.P4(refColors[i][1])
	}

	// Sprites + fonts.
	spriteDir := filepath.Join(srcDir, "sprites")
	fontDir := filepath.Join(srcDir, "fonts")
	index := packet2.Alloc(1)
	defer index.Release()

	convert := func(dir, name string) (*packet2.Packet, error) {
		p, err := pixpack.ConvertImage(index, dir, name)
		if err != nil {
			return nil, fmt.Errorf("convertImage %s/%s: %w", dir, name, err)
		}
		return p, nil
	}

	mapscene, err := convert(spriteDir, "mapscene")
	if err != nil {
		return err
	}
	defer mapscene.Release()
	mapfunction, err := convert(spriteDir, "mapfunction")
	if err != nil {
		return err
	}
	defer mapfunction.Release()
	b12, err := convert(fontDir, "b12")
	if err != nil {
		return err
	}
	defer b12.Release()
	mapdots, err := convert(spriteDir, "mapdots")
	if err != nil {
		return err
	}
	defer mapdots.Release()

	loadFM := func(name string) (*packet2.Packet, error) {
		p, err := packet2.Load(filepath.Join(fontDir, name), false)
		if err != nil {
			return nil, fmt.Errorf("load font %s: %w", name, err)
		}
		return p, nil
	}
	fontNames := []string{"f11.fm", "f12.fm", "f14.fm", "f17.fm", "f19.fm", "f22.fm", "f26.fm", "f30.fm"}
	fonts := make(map[string]*packet2.Packet, len(fontNames))
	for _, n := range fontNames {
		p, err := loadFM(n)
		if err != nil {
			return err
		}
		fonts[n] = p
		// Deferred Release; accumulates until function return, which is
		// safe since fonts are referenced by the jagfile until Save().
		defer p.Release()
	}

	// labels
	labelsRaw, err := os.ReadFile(filepath.Join(srcDir, "maps", "labels.txt"))
	if err != nil {
		return fmt.Errorf("read labels.txt: %w", err)
	}
	labels := parseLabels(string(labelsRaw))
	labelsPkt := packet2.Alloc(1)
	defer labelsPkt.Release()
	labelsPkt.P2(uint16(len(labels)))
	for _, lab := range labels {
		labelsPkt.PJStrLF(lab.Text)
		labelsPkt.P2(uint16(lab.X))
		labelsPkt.P2(uint16(lab.Z))
		labelsPkt.P1(uint8(lab.Type))
	}

	// Assemble jagfile (22 entries, TS:657-678 order).
	jag := jagfile.NewEmptyJagfile(false)
	jag.Write("underlay.dat", out.underlay)
	jag.Write("overlay.dat", out.overlay)
	jag.Write("loc.dat", out.loc)
	jag.Write("obj.dat", out.obj)
	jag.Write("npc.dat", out.npc)
	jag.Write("multi.dat", out.multi)
	jag.Write("free.dat", out.free)
	jag.Write("floorcol.dat", floorcol)
	jag.Write("mapscene.dat", mapscene)
	jag.Write("mapfunction.dat", mapfunction)
	jag.Write("b12.dat", b12)
	jag.Write("f11.dat", fonts["f11.fm"])
	jag.Write("f12.dat", fonts["f12.fm"])
	jag.Write("f14.dat", fonts["f14.fm"])
	jag.Write("f17.dat", fonts["f17.fm"])
	jag.Write("f19.dat", fonts["f19.fm"])
	jag.Write("f22.dat", fonts["f22.fm"])
	jag.Write("f26.dat", fonts["f26.fm"])
	jag.Write("f30.dat", fonts["f30.fm"])
	jag.Write("mapdots.dat", mapdots)
	jag.Write("index.dat", index)
	jag.Write("labels.dat", labelsPkt)

	outPath := filepath.Join(outDir, "mapview", "worldmap.jag")
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(outPath), err)
	}
	if err := jag.Save(outPath); err != nil {
		return fmt.Errorf("save jagfile %s: %w", outPath, err)
	}
	return nil
}

// packWater appends one "ocean" map square (mx, mz) to underlay
// and overlay. Mirrors TS Worldmap.ts:15-28.
//
// underlay grows by 2 + 4096 = 4098 bytes.
// overlay  grows by 2 + 4096*2 = 8194 bytes.
func packWater(flo *objtype.FloTypeConfigs, underlay, overlay *packet2.Packet, mx, mz int) {
	muddyId := uint8(1 + flo.GetId("muddygrass"))
	waterId := uint8(1 + flo.GetId("water"))

	underlay.P1(uint8(mx))
	underlay.P1(uint8(mz))
	overlay.P1(uint8(mx))
	overlay.P1(uint8(mz))

	for range 4096 {
		underlay.P1(muddyId)
		overlay.P1(waterId)
		overlay.P1(0)
	}
}

// unpackCoord extracts (level, x, z) from a packed local-coord
// int. x and z are LOCAL mapsquare coords (0..63). Mirrors TS
// Worldmap.ts:53-58.
func unpackCoord(packed int) (level, x, z int) {
	z = packed & 0x3f
	x = (packed >> 6) & 0x3f
	level = (packed >> 12) & 0x3
	return
}

// mapPackets bundles the seven per-stage output packets that the
// per-map loop appends to. Decoupled into a struct so the per-map
// processor can be unit-tested without dragging in the full Pack
// orchestration.
type mapPackets struct {
	underlay *packet2.Packet
	overlay  *packet2.Packet
	loc      *packet2.Packet
	obj      *packet2.Packet
	npc      *packet2.Packet
	multi    *packet2.Packet
	free     *packet2.Packet
}

func newMapPackets() *mapPackets {
	return &mapPackets{
		underlay: packet2.Alloc(1),
		overlay:  packet2.Alloc(1),
		loc:      packet2.Alloc(1),
		obj:      packet2.Alloc(1),
		npc:      packet2.Alloc(1),
		multi:    packet2.Alloc(1),
		free:     packet2.Alloc(1),
	}
}

func (m *mapPackets) release() {
	m.underlay.Release()
	m.overlay.Release()
	m.loc.Release()
	m.obj.Release()
	m.npc.Release()
	m.multi.Release()
	m.free.Release()
}

// mapCtx is the immutable per-Pack context passed to processMap.
type mapCtx struct {
	flo      *objtype.FloTypeConfigs
	locTypes *objtype.LocTypeConfigs
	npcTypes *objtype.NPCTypeConfigs
	multimap map[int]struct{}
	freemap  map[int]struct{}
}

// processMap appends one (mx, mz) mapsquare's worth of bytes to
// the seven output packets. Mirrors the body of the TS for-loop at
// Worldmap.ts:114-510.
//
// land/loc/obj/npc are the binary mapsquare files. obj and npc may
// be empty (Length()==0); in that case no bytes are emitted for
// those stages.
//
// Goscape deviation: actualLevel is bounds-checked (0..3) before
// indexing the fixed-size arrays. TS treats out-of-range as
// "undefined → falsy" and silently emits 0; Go would panic on a
// negative or >3 index. We make the same observable choice (emit 0,
// continue) via an explicit guard.
//
// Goscape note: obj/npc termination loops use Packet.Len()
// (= len(Data) - Pos), the TS `view.byteLength - pos` equivalent.
// Packet.Unused() returns cap(Data)-Pos which is unsafe for files
// loaded via os.ReadFile (cap can exceed len by 1, producing a
// one-byte off-by-one).
//
// Goscape note: loc-file locIdOffset/coordOffset use GSmart() (the
// goscape API name is reversed from TS — goscape GSmart matches TS
// gsmarts: byte path is plain g1(), two-byte path is g2()-0x8000).
// The plan draft said GSmartS() in error.
func processMap(ctx mapCtx, out *mapPackets, mx, mz int, land, loc, obj, npc *packet2.Packet) error {
	level := 0
	if mx == 33 && mz >= 71 && mz <= 73 {
		level = 1 // exception for underground pass
	}

	// --- land file decode ---
	var (
		flags        [4][64][64]int
		overlayIds   [4][64][64]int
		overlayShape [4][64][64]int
		overlayRot   [4][64][64]int
		underlayIds  [4][64][64]int
	)
	for l := range 4 {
		for x := range 64 {
			for z := range 64 {
				overlayIds[l][x][z] = -1
				underlayIds[l][x][z] = -1
			}
		}
	}

	for l := range 4 {
		for x := range 64 {
			for z := range 64 {
				for {
					op := int(land.G1())
					if op == 0 {
						break
					}
					if op == 1 {
						_ = land.G1()
						break
					}
					switch {
					case op <= 49:
						overlayIds[l][x][z] = int(land.G1())
						overlayShape[l][x][z] = (op - 2) / 4
						overlayRot[l][x][z] = (op - 2) & 0x3
					case op <= 81:
						flags[l][x][z] = op - 49
					default:
						underlayIds[l][x][z] = op - 81
					}
				}
			}
		}
	}

	out.overlay.P1(uint8(mx))
	out.overlay.P1(uint8(mz))
	out.underlay.P1(uint8(mx))
	out.underlay.P1(uint8(mz))
	for x := range 64 {
		for z := range 64 {
			bridged := (flags[1][x][z] & 0x2) == 2
			actualLevel := level
			if bridged {
				actualLevel = 1 + level
			}
			if actualLevel < 0 || actualLevel > 3 {
				out.overlay.P1(0)
				out.underlay.P1(0)
				continue
			}
			if overlayIds[actualLevel][x][z] != -1 {
				out.overlay.P1(uint8(overlayIds[actualLevel][x][z]))
				out.overlay.P1(uint8(overlayRot[actualLevel][x][z] + (overlayShape[actualLevel][x][z] << 2)))
			} else {
				out.overlay.P1(0)
			}
			if underlayIds[actualLevel][x][z] != -1 {
				out.underlay.P1(uint8(underlayIds[actualLevel][x][z]))
			} else {
				out.underlay.P1(0)
			}
		}
	}

	// --- loc file decode ---
	var (
		walls        [4][64][64]int
		mapscenes    [4][64][64]int
		mapfunctions [4][64][64]int
	)
	for l := range 4 {
		for x := range 64 {
			for z := range 64 {
				walls[l][x][z] = -1
				mapscenes[l][x][z] = -1
				mapfunctions[l][x][z] = -1
			}
		}
	}

	locId := -1
	locIdOffset := int(loc.GSmart())
	for locIdOffset != 0 {
		locId += locIdOffset

		coord := 0
		coordOffset := int(loc.GSmart())
		for coordOffset != 0 {
			coord += coordOffset - 1
			locLevel, x, z := unpackCoord(coord)
			info := int(loc.G1())
			coordOffset = int(loc.GSmart())

			var bridgedFlag int
			if locLevel == 1 {
				bridgedFlag = flags[locLevel][x][z] & 0x2
			} else {
				bridgedFlag = flags[1][x][z] & 0x2
			}
			actualLevel := locLevel
			if bridgedFlag == 2 {
				actualLevel = locLevel - 1
			}
			if actualLevel < 0 {
				continue
			}

			var locType *objtype.LocType
			if locId >= 0 && locId < len(ctx.locTypes.Configs) {
				locType = ctx.locTypes.Configs[locId]
			}
			if locType == nil {
				continue
			}
			shape := info >> 2
			angle := info & 0x3

			if locType.MapScene == 22 {
				continue
			}

			if walls[actualLevel][x][z] == -1 {
				switch pf.Shape(shape) {
				case pf.ShapeWallStraight:
					w := 1 + angle
					if locType.Active == 1 {
						w += 4
					}
					walls[actualLevel][x][z] = w
				case pf.ShapeWallL:
					w := 9 + angle
					if locType.Active == 1 {
						w += 4
					}
					walls[actualLevel][x][z] = w
				case pf.ShapeWallDecorStraightNoOffset:
					w := 17 + angle
					if locType.Active == 1 {
						w += 4
					}
					walls[actualLevel][x][z] = w
				case pf.ShapeWallDiagonal:
					w := 25 + (angle % 2)
					if locType.Active == 1 {
						w += 2
					}
					walls[actualLevel][x][z] = w
				}
			}
			if locType.MapScene != -1 {
				mapscenes[actualLevel][x][z] = locType.MapScene
			}
			if locType.MapFunction != -1 {
				mapfunctions[actualLevel][x][z] = locType.MapFunction
			}
		}
		locIdOffset = int(loc.GSmart())
	}

	out.loc.P1(uint8(mx))
	out.loc.P1(uint8(mz))
	for x := range 64 {
		for z := range 64 {
			if walls[level][x][z] != -1 {
				out.loc.P1(uint8(walls[level][x][z]))
			}
			if mapscenes[level][x][z] != -1 {
				out.loc.P1(uint8(29 + mapscenes[level][x][z]))
			}
			if mapfunctions[level][x][z] != -1 {
				out.loc.P1(uint8(160 + mapfunctions[level][x][z]))
			}
			out.loc.P1(0)
		}
	}

	// --- obj file ---
	if obj.Length() > 0 {
		var objs [4][64][64]int
		for l := range 4 {
			for x := range 64 {
				for z := range 64 {
					objs[l][x][z] = -1
				}
			}
		}
		for obj.Len() > 0 {
			pos := int(obj.G2())
			lvl := (pos >> 12) & 0x3
			lx := (pos >> 6) & 0x3f
			lz := pos & 0x3f
			count := int(obj.G1())
			for range count {
				id := int(obj.G2())
				_ = obj.G1() // count, discarded
				objs[lvl][lx][lz] = id
			}
		}
		out.obj.P1(uint8(mx))
		out.obj.P1(uint8(mz))
		for x := range 64 {
			for z := range 64 {
				out.obj.PBool(objs[level][x][z] != -1)
			}
		}
	}

	// --- npc file ---
	if npc.Length() > 0 {
		var npcs [4][64][64]int
		for l := range 4 {
			for x := range 64 {
				for z := range 64 {
					npcs[l][x][z] = -1
				}
			}
		}
		for npc.Len() > 0 {
			pos := int(npc.G2())
			lvl := (pos >> 12) & 0x3
			lx := (pos >> 6) & 0x3f
			lz := pos & 0x3f
			count := int(npc.G1())
			for range count {
				id := int(npc.G2())
				if id >= 0 && id < len(ctx.npcTypes.Configs) && ctx.npcTypes.Configs[id] != nil && ctx.npcTypes.Configs[id].Minimap {
					npcs[lvl][lx][lz] = id
				}
			}
		}
		out.npc.P1(uint8(mx))
		out.npc.P1(uint8(mz))
		for x := range 64 {
			for z := range 64 {
				out.npc.PBool(npcs[level][x][z] != -1)
			}
		}
	}

	// --- multi / free tile masks ---
	hasMulti := false
	hasFree := false
	var multiTiles [4][64][64]bool
	var freeTiles [4][64][64]bool
	for l := range 4 {
		for x := range 64 {
			for z := range 64 {
				worldX := (mx << 6) + x
				worldZ := (mz << 6) + z
				packed := coordgrid.PackCoord(l, worldX, worldZ)
				if _, ok := ctx.multimap[packed]; ok {
					multiTiles[l][x][z] = true
					hasMulti = true
				}
				if _, ok := ctx.freemap[packed]; ok {
					freeTiles[l][x][z] = true
					hasFree = true
				}
			}
		}
	}
	if hasMulti {
		out.multi.P1(uint8(mx))
		out.multi.P1(uint8(mz))
		for x := range 64 {
			for z := range 64 {
				out.multi.PBool(multiTiles[0][x][z])
			}
		}
	}
	if hasFree {
		out.free.P1(uint8(mx))
		out.free.P1(uint8(mz))
		for x := range 64 {
			for z := range 64 {
				out.free.PBool(freeTiles[0][x][z])
			}
		}
	}
	return nil
}
