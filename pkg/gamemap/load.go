package gamemap

import (
	"github.com/zsrv/goscape/pkg/entity"
	"github.com/zsrv/goscape/pkg/io/packet"
)

const (
	gameMapBlockMapSquare = 0x1 // BLOCK_MAP_SQUARE — marks a tile as blocked floor (TS GameMap.ts:24)
	gameMapLinkBelow      = 0x2 // LINK_BELOW — bridge tile; collision drops to level-1 (TS GameMap.ts:25)
	gameMapRemoveRoofs    = 0x4 // REMOVE_ROOFS — write roof collision (TS GameMap.ts:26)

	mapSquareSize = 64
	mapLevels     = 4
)

// loadGround parses a mapsquare's m{X}_{Z} file and marks blocked floor tiles.
//
// The byte stream iterates per-level, per-tile, reading opcodes until a
// tile-terminator is seen:
//
//	opcode 0:     end of tile
//	opcode 1:     1-byte height follows, ends tile
//	opcode 2..49: overlay data (3 bytes skipped)
//	opcode 50+:   direct land value = opcode - 49; ends tile
func (gm *GameMap) loadGround(data []byte, mapSquareX, mapSquareZ int) {
	p := packet.NewPacket(data)

	for level := 0; level < mapLevels; level++ {
		for x := 0; x < mapSquareSize; x++ {
			for z := 0; z < mapSquareSize; z++ {
				absX := (mapSquareX * mapSquareSize) + x
				absZ := (mapSquareZ * mapSquareSize) + z
				for {
					if p.Len() == 0 {
						return
					}
					op := p.G1()
					if op == 0 {
						break
					}
					if op == 1 {
						if p.Len() >= 1 {
							_ = p.G1() // height
						}
						break
					}
					if op <= 49 {
						// overlay: 3 trailing bytes (id, rot, underlay) — not used for collision
						if p.Len() >= 3 {
							_ = p.Next(3)
						}
						continue
					}
					// direct land value in range 50..255
					land := int(op) - 49
					if land&gameMapBlockMapSquare != 0 && level == 0 {
						gm.Pathfinder.ChangeFloor(absX, absZ, level, true)
					}
					break
				}
			}
		}
	}
}

// loadLocs parses a mapsquare's l{X}_{Z} file into LifecycleRespawn Loc
// entities accumulated in gm.staticLocs.
//
// Stream format (from TS GameMap.ts::loadLocations):
//
//	locID = -1
//	loop:
//	  delta = gsmart(); if delta == 0: end.
//	  locID += delta
//	  coord = 0
//	  loop:
//	    coordDelta = gsmart(); if coordDelta == 0: next locID.
//	    coord += coordDelta - 1
//	    level  = (coord >> 12) & 0x3
//	    localX = (coord >>  6) & 0x3F
//	    localZ =  coord         & 0x3F
//	    info   = g1()
//	    shape  = info >> 2
//	    angle  = info & 0x3
//	    instantiate LifecycleRespawn loc at absolute (mapX*64+localX, mapZ*64+localZ)
//
// Footprint is hardcoded to 1x1 until LocType config loading lands. Multi-tile
// locs (trees, large buildings) render correctly client-side because the client
// has its own LocType cache; server-side positional queries (pathing, aggro)
// will be wrong for those until LocType arrives.
// TODO(loctype): use LocType.Width/Length.
// TODO(bridged-levels): honour LINK_BELOW for bridge tiles (see TS reference).
func (gm *GameMap) loadLocs(data []byte, mapSquareX, mapSquareZ int) {
	p := packet.NewPacket(data)
	locID := -1
	for {
		if p.Len() == 0 {
			return
		}
		delta := int(p.GSmart())
		if delta == 0 {
			return
		}
		locID += delta
		coord := 0
		for {
			if p.Len() == 0 {
				return
			}
			coordDelta := int(p.GSmart())
			if coordDelta == 0 {
				break
			}
			coord += coordDelta - 1
			localZ := coord & 0x3F
			localX := (coord >> 6) & 0x3F
			level := (coord >> 12) & 0x3

			if p.Len() == 0 {
				return
			}
			info := p.G1()
			shape := int(info >> 2)
			angle := int(info & 0x3)

			absX := mapSquareX*mapSquareSize + localX
			absZ := mapSquareZ*mapSquareSize + localZ

			loc := entity.NewLoc(level, absX, absZ, 1, 1,
				entity.LifecycleRespawn,
				locID, shape, angle)
			gm.staticLocs = append(gm.staticLocs, loc)
		}
	}
}

// loadNPCs records NPC spawn positions from the n{X}_{Z} file.
//
// Layout (mirrors LostCityRS/Engine-TS at src/engine/GameMap.ts:114-137): each record is a 2-byte
// packed position (top 2 bits level, next 6 bits local X, low 6 bits local Z),
// followed by a 1-byte count and that many 2-byte NPC type IDs.
func (gm *GameMap) loadNPCs(data []byte, mapSquareX, mapSquareZ int) {
	p := packet.NewPacket(data)
	for p.Len() >= 3 {
		packed := int(p.G2())
		count := int(p.G1())
		level := (packed >> 12) & 0x3
		localX := (packed >> 6) & 0x3F
		localZ := packed & 0x3F
		absX := mapSquareX*mapSquareSize + localX
		absZ := mapSquareZ*mapSquareSize + localZ
		for i := 0; i < count && p.Len() >= 2; i++ {
			typeID := int(p.G2())
			gm.npcSpawns = append(gm.npcSpawns, NpcSpawn{
				TypeID: typeID, X: absX, Z: absZ, Level: level,
			})
		}
	}
}

// loadObjs records ground-object positions from the o{X}_{Z} file.
// Sub-spec 2 discards these (no Obj entity type yet).
func (gm *GameMap) loadObjs(data []byte, mapSquareX, mapSquareZ int) {
	_ = data
	_ = mapSquareX
	_ = mapSquareZ
}
