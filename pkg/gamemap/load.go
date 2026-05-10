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

// packCoord packs (x, z, level) into a single index per TS GameMap.ts:284-286.
// x and z are local-to-mapsquare (0..63); level is 0..3.
func packCoord(x, z, level int) int {
	return (z & 0x3F) | ((x & 0x3F) << 6) | ((level & 0x3) << 12)
}

// loadGround parses a mapsquare's m{X}_{Z} file in two passes:
//
//	pass 1: opcodes → lands[level*64*64 + x*64 + z] (per packCoord)
//	pass 2: for each tile, write FlagRoof when REMOVE_ROOFS set, then
//	        write FlagBlockWalk when BLOCK_MAP_SQUARE set, dropping the
//	        write level by 1 when the tile is bridged (LINK_BELOW).
//
// Mirrors TS Engine-TS/src/engine/GameMap.ts:182-217.
//
// The opcode stream per tile (loop until terminator):
//
//	opcode 0:      end of tile (lands[idx] stays 0)
//	opcode 1:      1-byte height follows; ends tile
//	opcode 2..49:  overlay tag (1 byte skipped); continues
//	opcode 50..81: direct land = opcode - 49; continues
//	opcode 82..255: no-op; continues
func (gm *GameMap) loadGround(data []byte, mapSquareX, mapSquareZ int) {
	p := packet.NewPacket(data)
	lands := make([]int8, mapLevels*mapSquareSize*mapSquareSize)

	// Pass 1 — parse opcodes into lands.
parseLoop:
	for level := range mapLevels {
		for x := range mapSquareSize {
			for z := range mapSquareSize {
				for {
					if p.Len() == 0 {
						break parseLoop
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
						if p.Len() >= 1 {
							_ = p.G1() // overlay tag (TS GameMap.ts:174)
						}
						continue
					}
					if op <= 81 {
						lands[packCoord(x, z, level)] = int8(op) - 49
					}
					// op >= 82: no-op (TS implicitly skips; no else branch)
				}
			}
		}
	}
	gm.landsByMapSquare[uint16((mapSquareX<<8)|mapSquareZ)] = lands

	// Pass 2 — write collision flags.
	for level := range mapLevels {
		for x := range mapSquareSize {
			absX := mapSquareX*mapSquareSize + x
			for z := range mapSquareSize {
				absZ := mapSquareZ*mapSquareSize + z
				land := int(lands[packCoord(x, z, level)])

				if land&gameMapRemoveRoofs != 0 {
					gm.Pathfinder.ChangeRoof(absX, absZ, level, true)
				}
				if land&gameMapBlockMapSquare == 0 {
					continue
				}

				var bridgeLand int
				if level == 1 {
					bridgeLand = land
				} else {
					bridgeLand = int(lands[packCoord(x, z, 1)])
				}
				actualLevel := level
				if bridgeLand&gameMapLinkBelow != 0 {
					actualLevel = level - 1
				}
				if actualLevel < 0 {
					continue
				}
				gm.Pathfinder.ChangeFloor(absX, absZ, actualLevel, true)
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
//	    bridged: if level==1 use lands[coord], else lands[packCoord(x,z,1)];
//	             actualLevel = bridged ? level-1 : level; skip if <0.
//	    instantiate LifecycleRespawn loc at actualLevel.
//
// Per-instance footprint (Width, Length) is read from the LocType registry
// when SetLocTypes was called before Init. When the registry is unset
// (e.g., empty-cache test fixtures) or the locID is missing/out-of-range
// (goscape defensive; TS calls printFatalError — see GameMap.ts:249-252),
// falls back to 1×1 with a log-warn for the missing-LocType branches.
// Mirrors TS GameMap.ts:248-263.
func (gm *GameMap) loadLocs(data []byte, mapSquareX, mapSquareZ int) {
	p := packet.NewPacket(data)
	lands := gm.landsByMapSquare[uint16((mapSquareX<<8)|mapSquareZ)]
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

			actualLevel := level
			if lands != nil { // goscape defensive; TS skips this check (lands is always populated by caller)
				var bridgeLand int
				if level == 1 {
					bridgeLand = int(lands[coord])
				} else {
					bridgeLand = int(lands[packCoord(localX, localZ, 1)])
				}
				if bridgeLand&gameMapLinkBelow != 0 {
					actualLevel = level - 1
				}
			}
			if actualLevel < 0 {
				continue
			}

			width, length := 1, 1
			if gm.locTypes != nil {
				if locID >= 0 && locID < len(gm.locTypes.Configs) {
					if lt := gm.locTypes.Configs[locID]; lt != nil {
						width, length = lt.Width, lt.Length
					} else {
						// (goscape defensive; TS calls printFatalError on missing LocType — see GameMap.ts:249-252)
						gm.log.Warn("loadLocs: nil LocType for locID; using 1x1 fallback",
							"locID", locID, "mapSquareX", mapSquareX, "mapSquareZ", mapSquareZ)
					}
				} else {
					// (goscape defensive; TS calls printFatalError on missing LocType — see GameMap.ts:249-252)
					gm.log.Warn("loadLocs: locID out of range; using 1x1 fallback",
						"locID", locID, "mapSquareX", mapSquareX, "mapSquareZ", mapSquareZ)
				}
			}

			loc := entity.NewLoc(actualLevel, absX, absZ, width, length,
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
// Mirrors LostCityRS/Engine-TS/src/engine/GameMap.ts:139-159.
//
// Wire layout per record: position(G2, packed level<<12|localX<<6|localZ)
// + tile-count(G1) + tile-count × (typeID(G2) + count(G1)).
//
// Two gates mirror TS:
//   - tile gate: skip when !members && !isFreeToPlay(absX, absZ)
//   - objtype gate: include when (objType.Members && members) || !objType.Members
//
// nil objTypes (test fixtures without registered configs) → skip all
// records silently. NAI-151.
func (gm *GameMap) loadObjs(data []byte, mapSquareX, mapSquareZ int) {
	p := packet.NewPacket(data)
	for p.Len() >= 3 {
		packed := int(p.G2())
		tileCount := int(p.G1())
		level := (packed >> 12) & 0x3
		localX := (packed >> 6) & 0x3F
		localZ := packed & 0x3F
		absX := mapSquareX*mapSquareSize + localX
		absZ := mapSquareZ*mapSquareSize + localZ
		for i := 0; i < tileCount && p.Len() >= 3; i++ {
			typeID := int(p.G2())
			count := int(p.G1())
			// Tile gate: skip members-only tile in F2P-only server.
			if !gm.members && !gm.IsFreeToPlay(absX, absZ) {
				continue
			}
			// nil-objTypes guard preserves test fixtures with empty caches.
			if gm.objTypes == nil {
				continue
			}
			if typeID < 0 || typeID >= len(gm.objTypes.Configs) {
				continue
			}
			ot := gm.objTypes.Configs[typeID]
			if ot == nil {
				continue
			}
			// ObjType gate: TS expression `(Members && members) || !Members`.
			if !((ot.Members && gm.members) || !ot.Members) {
				continue
			}
			gm.objSpawns = append(gm.objSpawns, ObjSpawn{
				TypeID: typeID, Count: count, X: absX, Z: absZ, Level: level,
			})
		}
	}
}
