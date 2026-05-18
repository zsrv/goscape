package worldmap

import (
	"errors"

	"github.com/zsrv/goscape/pkg/coordgrid"
	packet2 "github.com/zsrv/goscape/pkg/io/packet"
	"github.com/zsrv/goscape/pkg/objtype"
	pf "github.com/zsrv/goscape/pkg/pathfinder/loc"
)

// Pack is the worldmap packer entry point. Implementation lands in
// the Pack-entry-point task; this stub exists so callers can compile
// against the public surface while the per-map loop is being ported.
func Pack(srcDir, outDir string) error {
	_ = srcDir
	_ = outDir
	return errors.New("worldmap.Pack: not implemented")
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
