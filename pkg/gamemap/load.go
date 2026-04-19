package gamemap

import (
	"github.com/zsrv/goscape/pkg/io/packet"
)

const (
	gameMapBlockMapSquare = 0x2 // BLOCK_MAP_SQUARE — marks a tile as blocked floor

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

// loadLocs parses a mapsquare's l{X}_{Z} file.
//
// Sub-spec 2 scope: does not invoke loc collision because LocType config
// isn't ported yet. The function simply advances through the stream so future
// sub-specs can hook in. Ground-floor collision from loadGround already covers
// the bulk of static obstacles (water, map edges, solid terrain).
func (gm *GameMap) loadLocs(data []byte, mapSquareX, mapSquareZ int) {
	// Intentionally unused: sub-spec 2 doesn't have LocType yet.
	_ = data
	_ = mapSquareX
	_ = mapSquareZ
}

// loadNPCs records NPC spawn positions from the n{X}_{Z} file.
// Sub-spec 2 does not instantiate NPCs; later sub-specs will populate a
// spawn list here.
func (gm *GameMap) loadNPCs(data []byte, mapSquareX, mapSquareZ int) {
	_ = data
	_ = mapSquareX
	_ = mapSquareZ
}

// loadObjs records ground-object positions from the o{X}_{Z} file.
// Sub-spec 2 discards these (no Obj entity type yet).
func (gm *GameMap) loadObjs(data []byte, mapSquareX, mapSquareZ int) {
	_ = data
	_ = mapSquareX
	_ = mapSquareZ
}
