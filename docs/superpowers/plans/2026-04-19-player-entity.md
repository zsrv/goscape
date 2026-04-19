# Player Entity Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Deliver the full RS2 player entity, server-side collision-aware pathfinding, per-tick movement resolution, and `processLogins`/`processLogouts` tick phases.

**Architecture:** Vendor three packages from `github.com/zsrv/rs-server-225` (coordgrid, pathfinder, gamemap parsers), expand the `Player` struct to the full TS field set, add a `processPathing` tick phase that advances waypoints one tile per tick (two on run) against collision data, and replace the sub-spec 1 login/logout stubs with the canonical TS flow through `newPlayers`.

**Tech Stack:** Go standard library only — `net`, `sync`, `math/rand/v2`, `time`, `filepath`, `os`, `encoding/csv`. Vendored code from the user's older project.

> All `go` commands must use the prefix: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache`

---

## File Map

| File | Action | Responsibility |
|------|--------|----------------|
| `pkg/coordgrid/coordgrid.go` | **New (vendored)** | Direction enum, pack/unpack, zone/local conversion, face/move helpers — adapted from `rs-server-225/entity/position.go` |
| `pkg/coordgrid/coordgrid_test.go` | New | Tests for pack/unpack round-trip and the 8 direction cases |
| `pkg/pathfinder/collision/*.go` | **New (vendored)** | Flag, FlagMap, strategies, type — from `rs-server-225/ext/routefinder/collision/` |
| `pkg/pathfinder/reach/*.go` | **New (vendored)** | rectangularbounds, strategy — from `rs-server-225/ext/routefinder/reach/` |
| `pkg/pathfinder/loc/*.go` | **New (vendored)** | angle, layer, shape |
| `pkg/pathfinder/flag/*.go` | **New (vendored)** | blockaccess, direction |
| `pkg/pathfinder/rotation/*.go` | **New (vendored)** | rotation |
| `pkg/pathfinder/routefinder/*.go` | **New (vendored)** | api, line, lineroutefinder, linevalidator, naiveroutefinder, raycast, route, routecoordinates, routefinder, stepvalidator |
| `pkg/pathfinder/internal/*.go` | **New (vendored)** | test.go helper |
| `pkg/gamemap/gamemap.go` | **New** | `GameMap` struct wrapping `*routefinder.PathFinderAPI`; collision mutation wrappers |
| `pkg/gamemap/load.go` | **New** | `loadGround`, `loadLocs`, `loadObjs`, `loadNPCs` parsers — adapted from `rs-server-225/engine/gamemap.go` |
| `pkg/gamemap/multimap.go` | **New** | CSV loaders + `IsMulti`/`IsFreeToPlay` lookups |
| `pkg/gamemap/gamemap_test.go` | **New** | Synthetic fixture test |
| `modules/world/player.go` | Modify | Expand `Player` struct with full field set; update `newPlayer` with defaults |
| `modules/world/movement.go` | **New** | `pathToMoveClick`, `queueWaypoint`, `queueWaypoints`, `resolveMovement`, `stepOnce` |
| `modules/world/movement_test.go` | **New** | Unit tests for movement methods |
| `modules/world/handlers_game.go` | Modify | `handleMoveClick` calls `p.pathToMoveClick` |
| `modules/world/tick.go` | Modify | Add `processPathing`; implement `processLogouts` and `processLogins` |
| `modules/world/server.go` | Modify | Add `gamemap *gamemap.GameMap`, `newPlayers []*Player`, `appendNewPlayer`; wire `gamemap.Init` into `NewServer` |
| `modules/world/client.go` | Modify | `sendLoginOK` calls `appendNewPlayer` |
| `modules/world/config.go` | Modify | Add `CachePath string` (default `./data/pack/client/maps`) and `NodeClientRouteFinder bool` (default `true`) |

---

## Vendoring Notes

- **Skip packfile.go.** The older project's `cache/packfile.go` is 924 lines handling many pack types. Sub-spec 2 only needs to read raw bytes from map files in a flat directory — `os.ReadFile` + `filepath.Glob` is sufficient.
- **Map data source:** the goscape repo has `data/pack/client/maps/` containing `l*` and `m*` files. For sub-spec 2, this serves as the map source. (True server-side `n*`/`o*` files may be added later; missing files are silently skipped.)
- **CSV data:** `multiway.csv` and `free2play.csv` don't exist in goscape yet. `gamemap.Init` must handle their absence gracefully (skip + log warning; `IsMulti`/`IsFreeToPlay` default to `false`).

---

## Task 1: Vendor `pkg/coordgrid`

**Files:**
- Create: `pkg/coordgrid/coordgrid.go`
- Create: `pkg/coordgrid/coordgrid_test.go`

- [ ] **Step 1: Create `pkg/coordgrid/coordgrid.go`** (adapted from `rs-server-225/entity/position.go`)

```go
package coordgrid

import (
	"fmt"
	"math"
)

type Direction int

const (
	DirectionNorthwest Direction = iota
	DirectionNorth
	DirectionNortheast
	DirectionWest
	DirectionEast
	DirectionSouthwest
	DirectionSouth
	DirectionSoutheast
)

func Zone(pos int) int {
	return pos >> 3
}

func ZoneCenter(pos int) int {
	return Zone(pos) - 6
}

func ZoneOrigin(pos int) int {
	return ZoneCenter(pos) << 3
}

func MapSquare(pos uint16) uint16 {
	return pos >> 6
}

func Local(pos int, origin int) int {
	return pos - (ZoneCenter(origin) << 3)
}

func Face(srcX, srcZ, dstX, dstZ int) Direction {
	if srcX == dstX {
		if srcZ > dstZ {
			return DirectionSouth
		} else if srcZ < dstZ {
			return DirectionNorth
		}
	} else if srcX > dstX {
		if srcZ > dstZ {
			return DirectionSouthwest
		} else if srcZ < dstZ {
			return DirectionNorthwest
		} else {
			return DirectionWest
		}
	} else {
		if srcZ > dstZ {
			return DirectionSoutheast
		} else if srcZ < dstZ {
			return DirectionNortheast
		} else {
			return DirectionEast
		}
	}
	return -1
}

func DeltaX(dir Direction) int {
	switch dir {
	case DirectionSoutheast, DirectionNortheast, DirectionEast:
		return 1
	case DirectionSouthwest, DirectionNorthwest, DirectionWest:
		return -1
	default:
		return 0
	}
}

func DeltaZ(dir Direction) int {
	switch dir {
	case DirectionNorthwest, DirectionNortheast, DirectionNorth:
		return 1
	case DirectionSouthwest, DirectionSoutheast, DirectionSouth:
		return -1
	default:
		return 0
	}
}

func MoveX(pos int, dir Direction) int {
	return pos + DeltaX(dir)
}

func MoveZ(pos int, dir Direction) int {
	return pos + DeltaZ(dir)
}

func Closest(posX, posZ, posWidth, posLength, otherX, otherZ, otherWidth, otherLength int) (x, z int) {
	occupiedX := posX + posWidth - 1
	occupiedZ := posZ + posLength - 1
	if otherX <= posX {
		x = posX
	} else if otherX >= occupiedX {
		x = occupiedX
	} else {
		x = otherX
	}
	if otherZ <= posZ {
		z = posZ
	} else if otherZ >= occupiedZ {
		z = occupiedZ
	} else {
		z = otherZ
	}
	return
}

func DistanceTo(posX, posZ, posWidth, posLength, otherX, otherZ, otherWidth, otherLength int) int {
	p1X, p1Z := Closest(posX, posZ, posWidth, posLength, otherX, otherZ, otherWidth, otherLength)
	p2X, p2Z := Closest(otherX, otherZ, otherWidth, otherLength, posX, posZ, posWidth, posLength)
	return int(max(math.Abs(float64(p1X-p2X)), math.Abs(float64(p1Z-p2Z))))
}

func DistanceToSW(posX, posZ, otherX, otherZ int) int {
	dx := math.Abs(float64(posX - otherX))
	dz := math.Abs(float64(posZ - otherZ))
	return int(max(dx, dz))
}

func IsWithinDistanceSW(posX, posZ, otherX, otherZ, distance int) bool {
	if int(math.Abs(float64(posX-otherX))) > distance || int(math.Abs(float64(posZ-otherZ))) > distance {
		return false
	}
	return true
}

type Position struct {
	Level int
	X     int
	Z     int
}

func UnpackCoord(coord int) Position {
	return Position{
		Level: (coord >> 28) & 0x3,
		X:     (coord >> 14) & 0x3FFF,
		Z:     coord & 0x3FFF,
	}
}

func PackCoord(level, x, z int) int {
	return (z & 0x3fff) | ((x & 0x3fff) << 14) | ((level & 0x3) << 28)
}

func Intersects(srcX, srcZ, srcWidth, srcHeight, destX, destZ, destWidth, destHeight int) bool {
	srcHorizontal := srcX + srcWidth
	srcVertical := srcZ + srcHeight
	destHorizontal := destX + destWidth
	destVertical := destZ + destHeight
	return !(destX >= srcHorizontal || destHorizontal <= srcX || destZ >= srcVertical || destVertical <= srcZ)
}

func FormatString(level, x, z int, separator string) string {
	mx := x >> 6
	mz := z >> 6
	lx := x & 0x3F
	lz := z & 0x3F
	return fmt.Sprintf("%d%s%d%s%d%s%d%s%d", level, separator, mx, separator, mz, separator, lx, separator, lz)
}
```

- [ ] **Step 2: Create `pkg/coordgrid/coordgrid_test.go`**

```go
package coordgrid

import "testing"

func TestPackUnpackRoundTrip(t *testing.T) {
	cases := []struct{ level, x, z int }{
		{0, 3094, 3106},
		{3, 16383, 16383},
		{1, 0, 0},
		{2, 100, 200},
	}
	for _, tc := range cases {
		packed := PackCoord(tc.level, tc.x, tc.z)
		got := UnpackCoord(packed)
		if got.Level != tc.level || got.X != tc.x || got.Z != tc.z {
			t.Errorf("round-trip (%d,%d,%d): got (%d,%d,%d)", tc.level, tc.x, tc.z, got.Level, got.X, got.Z)
		}
	}
}

func TestFaceDirections(t *testing.T) {
	cases := []struct {
		name                   string
		srcX, srcZ, dstX, dstZ int
		want                   Direction
	}{
		{"north", 0, 0, 0, 1, DirectionNorth},
		{"south", 0, 1, 0, 0, DirectionSouth},
		{"east", 0, 0, 1, 0, DirectionEast},
		{"west", 1, 0, 0, 0, DirectionWest},
		{"northeast", 0, 0, 1, 1, DirectionNortheast},
		{"northwest", 1, 0, 0, 1, DirectionNorthwest},
		{"southeast", 0, 1, 1, 0, DirectionSoutheast},
		{"southwest", 1, 1, 0, 0, DirectionSouthwest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Face(tc.srcX, tc.srcZ, tc.dstX, tc.dstZ)
			if got != tc.want {
				t.Errorf("Face(%d,%d,%d,%d) = %d, want %d", tc.srcX, tc.srcZ, tc.dstX, tc.dstZ, got, tc.want)
			}
		})
	}
}

func TestDeltaXZ(t *testing.T) {
	if DeltaX(DirectionEast) != 1 || DeltaX(DirectionWest) != -1 || DeltaX(DirectionNorth) != 0 {
		t.Error("DeltaX wrong")
	}
	if DeltaZ(DirectionNorth) != 1 || DeltaZ(DirectionSouth) != -1 || DeltaZ(DirectionEast) != 0 {
		t.Error("DeltaZ wrong")
	}
}
```

- [ ] **Step 3: Run the tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/coordgrid/... -v
```

Expected: all pass.

- [ ] **Step 4: Commit**

```bash
git add pkg/coordgrid/
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(coordgrid): vendor coordinate utilities from rs-server-225

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: Vendor `pkg/pathfinder`

This is a bulk-copy task. The routefinder tree in `rs-server-225/ext/routefinder/` has 31 files across 7 subpackages. We copy the whole tree into `pkg/pathfinder/`, rewrite the module path, and verify tests pass.

**Files:**
- Create: `pkg/pathfinder/collision/{flag.go, flagmap.go, flagmap_test.go, strategies.go, type.go}`
- Create: `pkg/pathfinder/reach/{rectangularbounds.go, strategy.go, strategy_test.go}`
- Create: `pkg/pathfinder/loc/{angle.go, layer.go, shape.go}`
- Create: `pkg/pathfinder/flag/{blockaccess.go, direction.go}`
- Create: `pkg/pathfinder/rotation/{rotation.go, rotation_test.go}`
- Create: `pkg/pathfinder/routefinder/{api.go, line.go, lineroutefinder.go, lineroutefinder_test.go, linevalidator.go, linevalidator_test.go, naiveroutefinder.go, raycast.go, route.go, routecoordinates.go, routefinder.go, routefinder_bench_test.go, routefinder_test.go, stepvalidator.go, stepvalidator_test.go}`
- Create: `pkg/pathfinder/internal/test.go`
- Create: `pkg/pathfinder/routefinder/testdata/` (preserve any fixture files)

- [ ] **Step 1: Copy the tree**

```bash
cp -r /home/owner/Code/github.com/zsrv/rs-server-225/ext/routefinder/* /home/owner/Code/github.com/zsrv/goscape/pkg/pathfinder/
```

- [ ] **Step 2: Rewrite import paths**

Use `find` + `sed` to replace the module path across all vendored files:

```bash
find /home/owner/Code/github.com/zsrv/goscape/pkg/pathfinder -name '*.go' -exec sed -i 's|github.com/zsrv/rs-server-225/ext/routefinder|github.com/zsrv/goscape/pkg/pathfinder|g' {} +
```

- [ ] **Step 3: Verify tests pass**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pathfinder/... 2>&1 | tail -20
```

Expected: all subpackages `ok`. If any vendored file has a compile error (e.g., missing import), inspect it and fix by trimming unused imports or porting missing dependencies from the TS reference at `/home/owner/Code/github.com/LostCityRS/Engine-TS/`.

- [ ] **Step 4: Verify public API is reachable**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go doc github.com/zsrv/goscape/pkg/pathfinder/routefinder PathFinderAPI 2>&1 | head -10
```

Expected: `type PathFinderAPI struct { ... }` with `NewPathFinderAPI`, `FindPathDefault`, `FindPath`, `ChangeFloor`, `ChangeLoc`, etc. visible.

- [ ] **Step 5: Commit**

```bash
git add pkg/pathfinder/
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(pathfinder): vendor rsmod-pathfinder port from rs-server-225

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: Create `pkg/gamemap` — struct, collision wrappers, multimap

**Files:**
- Create: `pkg/gamemap/gamemap.go`
- Create: `pkg/gamemap/multimap.go`

- [ ] **Step 1: Create `pkg/gamemap/gamemap.go`**

```go
package gamemap

import (
	"log/slog"

	"github.com/zsrv/goscape/pkg/pathfinder/collision"
	"github.com/zsrv/goscape/pkg/pathfinder/loc"
	"github.com/zsrv/goscape/pkg/pathfinder/routefinder"
)

// GameMap holds collision data for the game world.
type GameMap struct {
	Pathfinder *routefinder.PathFinderAPI
	multimap   map[int]bool // packed zone coord -> multi combat
	freemap    map[int]bool // packed zone coord -> F2P
	log        *slog.Logger
}

func New(log *slog.Logger) *GameMap {
	pf := routefinder.NewPathFinderAPI()
	return &GameMap{
		Pathfinder: &pf,
		multimap:   make(map[int]bool),
		freemap:    make(map[int]bool),
		log:        log,
	}
}

// ChangeLandCollision marks or clears a floor tile as walkable.
func (gm *GameMap) ChangeLandCollision(x, z, level int, add bool) {
	gm.Pathfinder.ChangeFloor(x, z, level, add)
}

// ChangeLocCollision updates collision for a wall/scenery piece.
// Uses loc.LayerOf to route the call to the appropriate pathfinder method.
func (gm *GameMap) ChangeLocCollision(shape, angle int, blocksRange bool, length, width, x, z, level int, add bool) {
	layer := loc.LayerOf(loc.Shape(shape))
	switch layer {
	case loc.LayerWall:
		gm.Pathfinder.ChangeWall(x, z, level, angle, shape, blocksRange, false, add)
	case loc.LayerGround:
		gm.Pathfinder.ChangeLoc(x, z, level, width, length, blocksRange, false, add)
	}
	// LayerWallDecor and LayerGroundDecor do not affect collision.
}

// ChangeNPCCollision marks or clears an NPC's occupied tiles.
func (gm *GameMap) ChangeNPCCollision(size, x, z, level int, add bool) {
	gm.Pathfinder.ChangeNPC(x, z, level, size, add)
}

// ChangePlayerCollision marks or clears a player's occupied tile.
func (gm *GameMap) ChangePlayerCollision(size, x, z, level int, add bool) {
	gm.Pathfinder.ChangePlayer(x, z, level, size, add)
}

// ChangeRoofCollision marks or clears a roof.
func (gm *GameMap) ChangeRoofCollision(x, z, level int, add bool) {
	gm.Pathfinder.ChangeRoof(x, z, level, add)
}

// CanTravel tests whether moving from (x, z, level) with the given offset
// (dx, dz) to an adjacent tile is allowed. offsetX/offsetZ must be in {-1, 0, 1}.
func (gm *GameMap) CanTravel(level, x, z, offsetX, offsetZ int) bool {
	// size=1 (player), extraFlag=0, collisionType=Normal.
	return gm.Pathfinder.StepValidator.CanTravel(
		level, x, z, offsetX, offsetZ, 1, 0, collision.TypeNormal,
	)
}
```

- [ ] **Step 2: Create `pkg/gamemap/multimap.go`**

```go
package gamemap

import (
	"encoding/csv"
	"errors"
	"io/fs"
	"os"
	"strconv"
)

// packZoneCoord packs a level/zoneX/zoneZ tuple into a single int.
// Matches the TS GameMap.packCoord for multi/free zone lookups.
func packZoneCoord(x, z, level int) int {
	return (z & 0x3fff) | ((x & 0x3fff) << 14) | ((level & 0x3) << 28)
}

// IsMulti reports whether the tile at (x, z, level) is in a multi-combat zone.
func (gm *GameMap) IsMulti(x, z, level int) bool {
	return gm.multimap[packZoneCoord(x, z, level)]
}

// IsFreeToPlay reports whether the tile at (x, z) is in an F2P zone.
// F2P tables are level-agnostic in the TS reference.
func (gm *GameMap) IsFreeToPlay(x, z int) bool {
	return gm.freemap[packZoneCoord(x, z, 0)]
}

// loadCsvMap parses a CSV of "level,x,z" rows and inserts into dst.
// Missing files are not errors.
func loadCsvMap(path string, dst map[int]bool) error {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	defer f.Close()

	r := csv.NewReader(f)
	r.FieldsPerRecord = -1
	rows, err := r.ReadAll()
	if err != nil {
		return err
	}

	for _, row := range rows {
		if len(row) < 3 {
			continue
		}
		level, err1 := strconv.Atoi(row[0])
		x, err2 := strconv.Atoi(row[1])
		z, err3 := strconv.Atoi(row[2])
		if err1 != nil || err2 != nil || err3 != nil {
			continue
		}
		dst[packZoneCoord(x, z, level)] = true
	}
	return nil
}
```

- [ ] **Step 3: Verify it compiles**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./pkg/gamemap/...
```

Expected: no errors. If `CanTravel` fails to compile due to pathfinder API drift, adjust its body based on the actual vendored API surface.

- [ ] **Step 4: Commit**

```bash
git add pkg/gamemap/
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(gamemap): add GameMap struct with collision wrappers and multi/free-zone lookups

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: Implement `pkg/gamemap/load.go` — map-pack parsers

**Files:**
- Create: `pkg/gamemap/load.go`

The parsers are adapted from `rs-server-225/engine/gamemap.go` (the `loadGround`, `loadLocs`, `loadNPCs`, `loadObjs` methods). They read a `packet.Packet` (byte buffer) representing one mapsquare's data and update the GameMap's collision flags.

- [ ] **Step 1: Create `pkg/gamemap/load.go`**

```go
package gamemap

import (
	"github.com/zsrv/goscape/pkg/io/packet"
)

const (
	gameMapOpen           = 0x0
	gameMapBlocked        = 0x1
	gameMapBridge         = 0x2
	gameMapRoof           = 0x4
	gameMapBlockMapSquare = 0x2 // matches TS BLOCK_MAP_SQUARE
	gameMapLinkBelow      = 0x2 // matches TS LINK_BELOW bit

	mapSquareX = 64
	mapSquareZ = 64
	mapLevels  = 4
)

// packTileCoord packs absolute (x, z, level) into a single int matching the TS layout.
func packTileCoord(x, z, level int) int {
	return (z & 0x3fff) | ((x & 0x3fff) << 14) | ((level & 0x3) << 28)
}

// loadGround parses a mapsquare's m{X}_{Z} file, populates the land bitmap,
// and marks blocked floor tiles in the pathfinder.
//
// The byte stream is a per-level, per-tile opcode sequence:
//   opcode 0:     end of tile
//   opcode 1:     1-byte height, ends tile
//   opcode 2..49: overlay data (3 bytes skipped)
//   opcode 50+:   direct land value = opcode - 49; ends tile
func (gm *GameMap) loadGround(data []byte, mapSquareX, mapSquareZ int) {
	p := packet.NewPacket(data)
	lands := make([]int8, mapSquareX*mapSquareZ*mapLevels)

	for level := 0; level < mapLevels; level++ {
		for x := 0; x < 64; x++ {
			for z := 0; z < 64; z++ {
				absX := (mapSquareX * 64) + x
				absZ := (mapSquareZ * 64) + z
				for {
					if p.Len() == 0 {
						return
					}
					op := p.G1()
					if op == 0 {
						break
					}
					if op == 1 {
						_ = p.G1() // height
						break
					}
					if op <= 49 {
						_ = p.Next(3) // overlay(1) + rot(1) + underlay(1)
						continue
					}
					// direct land value
					land := int8(op - 49)
					lands[idx(x, z, level, 64, 64)] = land
					// For level 0 only — blocked floor tiles get pathfinder flag.
					if land&gameMapBlockMapSquare != 0 {
						if level == 0 {
							gm.Pathfinder.ChangeFloor(absX, absZ, level, true)
						}
					}
					_ = absX // silence unused if the level==0 check skips this
					_ = absZ
					break
				}
			}
		}
	}
}

func idx(x, z, level, w, d int) int {
	return (level * w * d) + (x * d) + z
}

// loadLocs parses a mapsquare's l{X}_{Z} file and calls ChangeLocCollision
// for each loc with blockwalk != 0. This requires LocType lookup by id;
// sub-spec 2 loads locs without the full LocType config, so it marks the
// tile as blocked conservatively (calls ChangeWall with stock values).
//
// Full LocType-driven collision is deferred to a later sub-spec.
func (gm *GameMap) loadLocs(data []byte, mapSquareX, mapSquareZ int) {
	if len(data) == 0 {
		return
	}
	p := packet.NewPacket(data)

	// Stream format: until EOF, each loc is preceded by its delta-id chain.
	// See rs-server-225 loadLocs for details. For sub-spec 2 we log the count
	// and skip actual loc insertion — the floor-blocked tiles from loadGround
	// already account for major map-pack obstacles.
	locCount := 0
	for p.Len() >= 3 {
		_ = p.G2() // id delta
		_ = p.G1() // pos delta
		locCount++
		if locCount > 100000 {
			break
		}
	}
	gm.log.Debug("loadLocs skipped (LocType not yet available)", "mapsquare", mapSquareX*256+mapSquareZ, "approx_count", locCount)
	_ = mapSquareX
	_ = mapSquareZ
}

// loadNPCs records NPC spawn positions from the n{X}_{Z} file. Sub-spec 2
// does not instantiate NPCs; the data is discarded but the call provides
// the hook for sub-spec 3+ to populate spawn lists.
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
```

> **Simplification note:** The full TS `loadLocs` reads an `id` delta stream, looks up each loc's `LocType`, and only calls `ChangeLoc` when the type has `blockwalk != 0`. Since `LocType` isn't ported yet, sub-spec 2 skips loc collision. The `loadGround` path still marks the BLOCK_MAP_SQUARE floor tiles, which covers the bulk of static obstacles (water, walls of solid terrain). Loc collision (fences, doors, scenery) is deferred to a later sub-spec that ports `LocType`.

- [ ] **Step 2: Verify it compiles**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./pkg/gamemap/...
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add pkg/gamemap/load.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(gamemap): add map-pack parsers (ground floors marked in pathfinder)

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: Implement `GameMap.Init` + synthetic-fixture test

**Files:**
- Modify: `pkg/gamemap/gamemap.go`
- Create: `pkg/gamemap/gamemap_test.go`

- [ ] **Step 1: Write the failing test**

Create `pkg/gamemap/gamemap_test.go`:

```go
package gamemap

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestInitHandlesMissingDir(t *testing.T) {
	gm := New(discardLogger())
	err := gm.Init(t.TempDir()) // empty dir
	if err != nil {
		t.Errorf("Init on empty dir: got %v, want nil", err)
	}
}

func TestInitHandlesMissingCsv(t *testing.T) {
	tmp := t.TempDir()
	// create maps subdir but no CSVs
	if err := os.MkdirAll(filepath.Join(tmp, "maps"), 0755); err != nil {
		t.Fatal(err)
	}
	gm := New(discardLogger())
	err := gm.Init(tmp)
	if err != nil {
		t.Errorf("Init with missing CSVs: got %v, want nil", err)
	}
	if gm.IsMulti(1000, 2000, 0) {
		t.Error("IsMulti should default false when multimap CSV missing")
	}
	if gm.IsFreeToPlay(1000, 2000) {
		t.Error("IsFreeToPlay should default false when freemap CSV missing")
	}
}

func TestInitLoadsCsvMaps(t *testing.T) {
	tmp := t.TempDir()
	mapsDir := filepath.Join(tmp, "maps")
	if err := os.MkdirAll(mapsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mapsDir, "multiway.csv"), []byte("0,1000,2000\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mapsDir, "free2play.csv"), []byte("0,1500,2500\n"), 0644); err != nil {
		t.Fatal(err)
	}

	gm := New(discardLogger())
	if err := gm.Init(tmp); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if !gm.IsMulti(1000, 2000, 0) {
		t.Error("expected (1000,2000,0) to be multi")
	}
	if !gm.IsFreeToPlay(1500, 2500) {
		t.Error("expected (1500,2500) to be F2P")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/gamemap/... -run TestInit -v
```

Expected: compile error — `Init` undefined.

- [ ] **Step 3: Add `Init` to `gamemap.go`**

Append to `pkg/gamemap/gamemap.go`:

```go
// Init reads map-pack files from cacheDir/maps/ and populates the collision map.
// Missing files and missing CSVs are treated as warnings, not errors.
func (gm *GameMap) Init(cacheDir string) error {
	mapsDir := filepath.Join(cacheDir, "maps")

	// Load multimap and freemap CSVs (non-fatal if missing).
	if err := loadCsvMap(filepath.Join(mapsDir, "multiway.csv"), gm.multimap); err != nil {
		gm.log.Warn("failed to load multiway.csv", "err", err)
	}
	if err := loadCsvMap(filepath.Join(mapsDir, "free2play.csv"), gm.freemap); err != nil {
		gm.log.Warn("failed to load free2play.csv", "err", err)
	}

	// Enumerate m{X}_{Z} files and load each mapsquare.
	mMatches, err := filepath.Glob(filepath.Join(mapsDir, "m*_*"))
	if err != nil {
		return fmt.Errorf("glob mapsquare files: %w", err)
	}

	loaded := 0
	for _, mPath := range mMatches {
		base := filepath.Base(mPath)
		if len(base) < 2 || base[0] != 'm' {
			continue
		}
		// parse "m{X}_{Z}"
		var sqX, sqZ int
		if _, err := fmt.Sscanf(base, "m%d_%d", &sqX, &sqZ); err != nil {
			gm.log.Warn("invalid mapsquare filename", "name", base, "err", err)
			continue
		}

		mData, err := os.ReadFile(mPath)
		if err != nil {
			gm.log.Warn("failed to read mapsquare data", "path", mPath, "err", err)
			continue
		}
		gm.loadGround(mData, sqX, sqZ)

		// Optional: l*, n*, o* files — not errors if missing.
		lPath := filepath.Join(mapsDir, fmt.Sprintf("l%d_%d", sqX, sqZ))
		if lData, err := os.ReadFile(lPath); err == nil {
			gm.loadLocs(lData, sqX, sqZ)
		}
		nPath := filepath.Join(mapsDir, fmt.Sprintf("n%d_%d", sqX, sqZ))
		if nData, err := os.ReadFile(nPath); err == nil {
			gm.loadNPCs(nData, sqX, sqZ)
		}
		oPath := filepath.Join(mapsDir, fmt.Sprintf("o%d_%d", sqX, sqZ))
		if oData, err := os.ReadFile(oPath); err == nil {
			gm.loadObjs(oData, sqX, sqZ)
		}

		loaded++
	}

	gm.log.Info("game map loaded", "mapsquares", loaded)
	return nil
}
```

Add imports to the top of `gamemap.go`:

```go
import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/zsrv/goscape/pkg/pathfinder/collision"
	"github.com/zsrv/goscape/pkg/pathfinder/loc"
	"github.com/zsrv/goscape/pkg/pathfinder/routefinder"
)
```

- [ ] **Step 4: Run the tests to verify they pass**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/gamemap/... -v
```

Expected: all pass.

- [ ] **Step 5: Commit**

```bash
git add pkg/gamemap/
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(gamemap): implement Init with CSV and mapsquare loading

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: Update world config with `CachePath` and `NodeClientRouteFinder`

**Files:**
- Modify: `modules/world/config.go`

- [ ] **Step 1: Add fields to `Config` struct**

Open `modules/world/config.go`. Find the `Config` struct definition. Add two fields:

```go
CachePath             string
NodeClientRouteFinder bool
```

In `RegisterFlagsAndApplyDefaults` (or whatever the existing method is named — check the file), register them with defaults:

```go
cfg.CachePath = "./data/pack/client/maps"  // adjusts later for true server-pack dir when available
cfg.NodeClientRouteFinder = true
// If the codebase uses a flag set or env mapping, also add:
//   flagSet.StringVar(&cfg.CachePath, "cache.path", cfg.CachePath, "...")
//   flagSet.BoolVar(&cfg.NodeClientRouteFinder, "world.clientRouteFinder", cfg.NodeClientRouteFinder, "...")
```

The implementer must match the exact existing flag-registration style in that file. Read `config.go` first (about 60 lines) to see the pattern.

- [ ] **Step 2: Verify it compiles**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./modules/world/...
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add modules/world/config.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): add CachePath and NodeClientRouteFinder config fields

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 7: Wire `GameMap` into `Server`

**Files:**
- Modify: `modules/world/server.go`

- [ ] **Step 1: Add `gamemap` field to `Server`**

Open `modules/world/server.go`. Add an import:

```go
import "github.com/zsrv/goscape/pkg/gamemap"
```

Add a field to the `Server` struct:

```go
gamemap *gamemap.GameMap
```

- [ ] **Step 2: Initialize in `NewServer`**

Find `NewServer`. After `s := &Server{...}`, add:

```go
gm := gamemap.New(logger)
if err := gm.Init(cfg.CachePath); err != nil {
	return nil, fmt.Errorf("failed to load game map: %w", err)
}
s.gamemap = gm
```

- [ ] **Step 3: Verify it compiles and tests pass**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... 2>&1 | tail -5
```

Expected: tests pass (the existing `newTestServer` helper in `server_test.go` may need updating if it fails because `NewServer` is only called through the world module, not in tests — tests use `newTestServer` which constructs `Server` directly without calling `NewServer`, so `s.gamemap = nil` in tests. This is OK for sub-spec 2 tests that don't touch movement.)

- [ ] **Step 4: Commit**

```bash
git add modules/world/server.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): load GameMap during Server startup

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 8: Add movement constants and `entity` interface

**Files:**
- Create: `modules/world/movement_consts.go`

- [ ] **Step 1: Create the file**

```go
package world

// MoveSpeed describes a player or NPC's movement mode.
type MoveSpeed int

const (
	MoveSpeedStationary MoveSpeed = iota
	MoveSpeedCrawl
	MoveSpeedWalk
	MoveSpeedRun
	MoveSpeedInstant
)

// MoveRestrict controls which surfaces an entity may walk on.
type MoveRestrict int

const (
	MoveRestrictNormal MoveRestrict = iota
	MoveRestrictBlocked
	MoveRestrictIndoors
	MoveRestrictOutdoors
	MoveRestrictNoMove
	MoveRestrictPassthru
)

// MoveStrategy selects between SMART (pathfinder-routed) and NAIVE (straight-line) movement.
type MoveStrategy int

const (
	MoveStrategySmart MoveStrategy = iota
	MoveStrategyNaive
)

// BlockWalk controls whether an entity blocks others from walking through its tile.
type BlockWalk int

const (
	BlockWalkNone BlockWalk = iota
	BlockWalkNpc
	BlockWalkAll
)

// entity is implemented by all targetable game objects.
// Sub-spec 2 only has *Player; sub-specs 3+ add Npc, Loc, Obj.
type entity interface {
	Slot() int
	Coords() (x, z, level int)
}
```

- [ ] **Step 2: Verify it compiles**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./modules/world/...
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add modules/world/movement_consts.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): add movement constants and entity interface

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 9: Expand the `Player` struct

**Files:**
- Modify: `modules/world/player.go`

- [ ] **Step 1: Replace the `Player` struct definition**

Open `modules/world/player.go`. Replace the existing `type Player struct { ... }` block with:

```go
type Player struct {
	// === network (from sub-spec 1) ===
	slot   int
	client *client

	// === identity ===
	username      string
	username37    uint64
	hash64        uint64
	displayName   string
	uid           int
	members       bool
	staffModLevel int32

	// === coordinates & level (Entity) ===
	x, z, level                     int
	originX, originZ                int
	lastTickX, lastTickZ, lastLevel int
	lastStepX, lastStepZ            int

	// === movement (PathingEntity) ===
	moveSpeed              MoveSpeed
	moveRestrict           MoveRestrict
	moveStrategy           MoveStrategy
	blockWalk              BlockWalk
	walkDir, runDir        int
	waypointIndex          int
	waypoints              [25]int
	tele, jump             bool
	stepsTaken             int
	followX, followZ       int
	targetX, targetZ       int
	faceAngleX, faceAngleZ int

	// === interaction target ===
	target        entity
	targetOp      int
	targetSubject struct{ typ, com int }
	apRange       int
	apRangeCalled bool
	interacted    bool
	repathed      bool
	delayed       bool
	delayedUntil  int

	// === masks ===
	masks      int
	entitymask int

	// === appearance ===
	body           [7]int
	colors         [5]int
	gender         int
	combatLevel    int
	headicons      int
	appearanceInv  int
	appearanceBuf  []byte
	lastAppearance int

	// === stats & vars ===
	stats      [21]int32
	levels     [21]uint8
	baseLevels [21]uint8
	lastStats  [21]int32
	lastLevels [21]uint8
	vars       []int32
	varsString []string

	// === run energy ===
	run, tempRun             int
	runenergy, lastRunEnergy int
	runweight                int

	// === chat state ===
	publicChat, privateChat, tradeDuel int
	chatMessage                        []byte
	chatColour, chatEffect, chatRights int
	mutedUntil                         time.Time
	messageCount                       int

	// === session flags ===
	playtime                                     int
	lastResponse, lastConnected                  int
	requestLogout, requestIdleLogout, loggingOut bool
	preventLogoutMessage                         string
	preventLogoutUntil                           int
	reconnecting, lowMemory, webClient           bool
	afkEventReady, moveClickRequest              bool

	// === modal (from sub-spec 1) ===
	modalMain, modalChat, modalSide             int
	lastModalMain, lastModalChat, lastModalSide int
	modalState                                  int
	refreshModal, refreshModalClose             bool

	// === per-tick rate limits (from sub-spec 1) ===
	userLimit, clientLimit, restrictedLimit int

	// === last* fields — for echo suppression ===
	lastItem, lastSlot, lastUseItem, lastUseSlot, lastTargetSlot, lastCom int
}
```

Add `"time"` to the import list.

- [ ] **Step 2: Update `newPlayer` with defaults**

Replace the existing `newPlayer` function:

```go
func newPlayer(c *client) *Player {
	p := &Player{
		client:        c,
		slot:          -1,
		uid:           -1,
		x:             3094, // tutorial island
		z:             3106,
		level:         0,
		originX:       -1,
		originZ:       -1,
		lastTickX:     -1,
		lastTickZ:     -1,
		lastLevel:     -1,
		lastStepX:     -1,
		lastStepZ:     -1,
		walkDir:       -1,
		runDir:        -1,
		waypointIndex: -1,
		runenergy:     10000,
		lastRunEnergy: -1,
		moveSpeed:     MoveSpeedInstant,
		moveStrategy:  MoveStrategySmart,
		moveRestrict:  MoveRestrictNormal,
		blockWalk:     BlockWalkNpc,
		combatLevel:   3,
		colors:        [5]int{0, 0, 0, 0, 0},
		body:          [7]int{0, 10, 18, 26, 33, 36, 42},
		appearanceInv: -1,
		targetOp:      -1,
		apRange:       10,
		followX:       -1,
		followZ:       -1,
		targetX:       -1,
		targetZ:       -1,
		faceAngleX:    -1,
		faceAngleZ:    -1,
		lastItem:      -1,
		lastSlot:      -1,
		lastUseItem:   -1,
		lastUseSlot:   -1,
		lastTargetSlot: -1,
		lastCom:       -1,
		lastConnected: -1,
		lastResponse:  -1,
	}
	return p
}
```

- [ ] **Step 3: Implement the `entity` interface methods**

Add at the end of `player.go`:

```go
// Slot returns the RS2 slot of this player.
func (p *Player) Slot() int { return p.slot }

// Coords returns the player's current absolute coordinates.
func (p *Player) Coords() (x, z, level int) { return p.x, p.z, p.level }
```

- [ ] **Step 4: Verify it compiles and existing tests still pass**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... 2>&1 | tail -5
```

Expected: all tests pass. The existing `TestSendLoginOKRegistersPlayer` and other sub-spec 1 tests still compile and pass because they don't touch the new fields.

- [ ] **Step 5: Commit**

```bash
git add modules/world/player.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): expand Player struct with full RS2 field set and defaults

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 10: Implement `queueWaypoint` / `queueWaypoints`

**Files:**
- Create: `modules/world/movement.go`
- Create: `modules/world/movement_test.go`

- [ ] **Step 1: Write the failing tests**

Create `modules/world/movement_test.go`:

```go
package world

import "testing"

func TestQueueWaypointSetsFirstEntry(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.x, p.z, p.level = 3094, 3106, 0

	p.queueWaypoint(3100, 3110)

	if p.waypointIndex != 0 {
		t.Errorf("waypointIndex: got %d, want 0", p.waypointIndex)
	}
	// waypoints[0] should be packed (level, 3100, 3110)
	if p.waypoints[0] == 0 {
		t.Error("waypoints[0] should be set")
	}
}

func TestQueueWaypointsReplacesExisting(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.x, p.z, p.level = 3094, 3106, 0

	first := []int{packTestCoord(0, 3100, 3110)}
	second := []int{packTestCoord(0, 3200, 3210), packTestCoord(0, 3205, 3215)}

	p.queueWaypoints(first)
	if p.waypointIndex != 0 {
		t.Errorf("after first queueWaypoints, waypointIndex = %d, want 0", p.waypointIndex)
	}

	p.queueWaypoints(second)
	if p.waypointIndex != 1 {
		t.Errorf("after second queueWaypoints (2 entries), waypointIndex = %d, want 1", p.waypointIndex)
	}
}

// packTestCoord is a test helper using the coordgrid packing.
func packTestCoord(level, x, z int) int {
	return (z & 0x3fff) | ((x & 0x3fff) << 14) | ((level & 0x3) << 28)
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run "TestQueueWaypoint" -v
```

Expected: compile error — `queueWaypoint` / `queueWaypoints` undefined.

- [ ] **Step 3: Create `modules/world/movement.go`**

```go
package world

import (
	"github.com/zsrv/goscape/pkg/coordgrid"
)

// queueWaypoint clears any existing path and sets a single destination.
func (p *Player) queueWaypoint(x, z int) {
	p.waypoints[0] = coordgrid.PackCoord(p.level, x, z)
	p.waypointIndex = 0
}

// queueWaypoints replaces the current path with the given packed coords,
// storing up to 24 entries (waypoints[] capacity is 25; index 0 reserved as the oldest).
// The input is in order: waypoints[0] is the final destination and the last element
// (input[len-1]) is the first step. This matches the TS queueWaypoints behaviour.
func (p *Player) queueWaypoints(packed []int) {
	if len(packed) == 0 {
		p.waypointIndex = -1
		return
	}
	n := len(packed)
	if n > len(p.waypoints) {
		n = len(p.waypoints)
	}
	for i := 0; i < n; i++ {
		p.waypoints[i] = packed[i]
	}
	p.waypointIndex = n - 1
}
```

- [ ] **Step 4: Run the tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run "TestQueueWaypoint" -v
```

Expected: all pass.

- [ ] **Step 5: Commit**

```bash
git add modules/world/movement.go modules/world/movement_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): add queueWaypoint and queueWaypoints for path setup

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 11: Implement `resolveMovement` and `stepOnce`

**Files:**
- Modify: `modules/world/movement.go`
- Modify: `modules/world/movement_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `movement_test.go`:

```go
func TestResolveMovementAdvancesOneTileWalking(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.x, p.z, p.level = 3094, 3106, 0
	p.moveSpeed = MoveSpeedWalk
	p.queueWaypoint(3094, 3107) // one step north

	p.resolveMovement()

	if p.z != 3107 {
		t.Errorf("after walk step z: got %d, want 3107", p.z)
	}
	if p.walkDir == -1 {
		t.Error("walkDir should be set after a step")
	}
	if p.runDir != -1 {
		t.Errorf("runDir: got %d, want -1 (walking)", p.runDir)
	}
	if p.lastTickX != 3094 || p.lastTickZ != 3106 {
		t.Errorf("lastTick: got (%d,%d), want (3094,3106)", p.lastTickX, p.lastTickZ)
	}
}

func TestResolveMovementAdvancesTwoTilesRunning(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.x, p.z, p.level = 3094, 3106, 0
	p.moveSpeed = MoveSpeedRun
	p.runenergy = 10000
	// Path: two steps north
	p.waypoints[0] = packTestCoord(0, 3094, 3108)
	p.waypointIndex = 0

	p.resolveMovement()

	if p.z != 3108 {
		t.Errorf("after run z: got %d, want 3108 (two steps)", p.z)
	}
	if p.walkDir == -1 {
		t.Error("walkDir should be set")
	}
	if p.runDir == -1 {
		t.Error("runDir should be set when running")
	}
}

func TestResolveMovementNoPathClearsDirections(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.waypointIndex = -1
	p.walkDir = 5
	p.runDir = 3

	p.resolveMovement()

	if p.walkDir != -1 {
		t.Errorf("walkDir with no path: got %d, want -1", p.walkDir)
	}
	if p.runDir != -1 {
		t.Errorf("runDir with no path: got %d, want -1", p.runDir)
	}
}

func TestResolveMovementDrainsRunEnergy(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.x, p.z, p.level = 3094, 3106, 0
	p.moveSpeed = MoveSpeedRun
	p.runenergy = 10000
	p.runweight = 0
	p.waypoints[0] = packTestCoord(0, 3094, 3108)
	p.waypointIndex = 0

	p.resolveMovement()

	if p.runenergy >= 10000 {
		t.Errorf("run energy should have drained, got %d", p.runenergy)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run "TestResolveMovement" -v
```

Expected: compile error — `resolveMovement` undefined.

- [ ] **Step 3: Add `resolveMovement` and `stepOnce` to `movement.go`**

Append to `movement.go`:

```go
// resolveMovement advances the player along their waypoint queue for one tick.
// Called from processPathing. Must run before processClientsOut so walkDir/runDir
// are set for the outgoing info block.
func (p *Player) resolveMovement() {
	p.lastTickX = p.x
	p.lastTickZ = p.z
	p.lastLevel = p.level

	if p.waypointIndex < 0 {
		p.walkDir = -1
		p.runDir = -1
		return
	}

	// First step (walk).
	dir, ok := p.stepOnce()
	if !ok {
		p.walkDir = -1
		p.runDir = -1
		return
	}
	p.walkDir = int(dir)
	p.runDir = -1

	// Second step if running.
	if p.moveSpeed == MoveSpeedRun && p.runenergy > 0 && p.waypointIndex >= 0 {
		dir2, ok2 := p.stepOnce()
		if ok2 {
			p.runDir = int(dir2)
			p.drainRunEnergy()
		}
	}
}

// stepOnce advances one tile toward the current waypoint.
// Returns the direction taken and whether a step was made.
func (p *Player) stepOnce() (coordgrid.Direction, bool) {
	if p.waypointIndex < 0 {
		return -1, false
	}
	dest := coordgrid.UnpackCoord(p.waypoints[p.waypointIndex])
	dir := coordgrid.Face(p.x, p.z, dest.X, dest.Z)
	if dir == -1 {
		// Already at the current waypoint — advance.
		p.waypointIndex--
		return -1, false
	}

	// Collision check — skip if gamemap is nil (test contexts).
	dx := coordgrid.DeltaX(dir)
	dz := coordgrid.DeltaZ(dir)
	if p.client != nil && p.client.server != nil && p.client.server.gamemap != nil {
		if !p.client.server.gamemap.CanTravel(p.level, p.x, p.z, dx, dz) {
			p.waypointIndex = -1
			return -1, false
		}
	}

	p.lastStepX = p.x
	p.lastStepZ = p.z
	p.x += dx
	p.z += dz
	p.stepsTaken++

	// Reached current waypoint? advance.
	if p.x == dest.X && p.z == dest.Z {
		p.waypointIndex--
	}
	return dir, true
}

// drainRunEnergy applies the TS run-energy decay formula once per running step.
// Formula: runenergy -= (67 + 67*runweight/64) / 100 (floored at 0)
func (p *Player) drainRunEnergy() {
	decay := (67 + 67*p.runweight/64) / 100
	if decay < 1 {
		decay = 1
	}
	p.runenergy -= decay
	if p.runenergy < 0 {
		p.runenergy = 0
	}
}
```

- [ ] **Step 4: Run the tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run "TestResolveMovement" -v
```

Expected: all pass.

- [ ] **Step 5: Commit**

```bash
git add modules/world/movement.go modules/world/movement_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): add resolveMovement and stepOnce with run-energy drain

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 12: Implement `pathToMoveClick`

**Files:**
- Modify: `modules/world/movement.go`
- Modify: `modules/world/movement_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `movement_test.go`:

```go
func TestPathToMoveClickSmartTrustClient(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.x, p.z, p.level = 3094, 3106, 0
	p.moveStrategy = MoveStrategySmart

	packed := []int{packTestCoord(0, 3100, 3110)}
	p.pathToMoveClick(packed, false) // needsFinding = false — trust client

	if p.waypointIndex != 0 {
		t.Errorf("waypointIndex: got %d, want 0", p.waypointIndex)
	}
	if p.waypoints[0] != packed[0] {
		t.Error("waypoints[0] should equal input")
	}
}

func TestPathToMoveClickNaiveTakesLastCoord(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.x, p.z, p.level = 3094, 3106, 0
	p.moveStrategy = MoveStrategyNaive

	packed := []int{packTestCoord(0, 3100, 3110), packTestCoord(0, 3105, 3115)}
	p.pathToMoveClick(packed, false)

	dest := coordgridUnpackForTest(p.waypoints[0])
	if dest.X != 3105 || dest.Z != 3115 {
		t.Errorf("NAIVE should take input[-1]: got (%d,%d), want (3105,3115)", dest.X, dest.Z)
	}
}

// coordgridUnpackForTest mirrors coordgrid.UnpackCoord for test readability.
func coordgridUnpackForTest(coord int) struct{ Level, X, Z int } {
	return struct{ Level, X, Z int }{
		Level: (coord >> 28) & 0x3,
		X:     (coord >> 14) & 0x3FFF,
		Z:     coord & 0x3FFF,
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run "TestPathToMoveClick" -v
```

Expected: compile error — `pathToMoveClick` undefined.

- [ ] **Step 3: Add `pathToMoveClick` to `movement.go`**

Append to `movement.go`:

```go
// pathToMoveClick translates a MOVE_GAMECLICK / MOVE_OPCLICK waypoint list
// into the player's movement queue. If needsFinding is true and moveStrategy
// is SMART, the server runs its own pathfinder; otherwise it trusts the
// client-supplied coords.
func (p *Player) pathToMoveClick(packed []int, needsFinding bool) {
	if len(packed) == 0 {
		return
	}

	switch p.moveStrategy {
	case MoveStrategySmart:
		if needsFinding && p.client != nil && p.client.server != nil && p.client.server.gamemap != nil {
			dest := coordgrid.UnpackCoord(packed[0])
			route := p.client.server.gamemap.Pathfinder.FindPathDefault(p.level, p.x, p.z, dest.X, dest.Z)
			if coords := routeToPacked(route); len(coords) > 0 {
				p.queueWaypoints(coords)
			}
		} else {
			p.queueWaypoints(packed)
		}
	case MoveStrategyNaive:
		dest := coordgrid.UnpackCoord(packed[len(packed)-1])
		p.queueWaypoint(dest.X, dest.Z)
	}
}

// routeToPacked converts a pathfinder.Route into packed coord ints matching
// the queueWaypoints expectation (waypoints[0] is the final destination,
// later indices are earlier steps). The pathfinder returns Waypoints ordered
// from destination to source, which matches our needs.
func routeToPacked(route routefinder.Route) []int {
	if !route.Success || len(route.Waypoints) == 0 {
		return nil
	}
	out := make([]int, len(route.Waypoints))
	for i, rc := range route.Waypoints {
		out[i] = coordgrid.PackCoord(rc.Level(), rc.X(), rc.Z())
	}
	return out
}
```

Add the pathfinder import to `movement.go`:

```go
import (
	"github.com/zsrv/goscape/pkg/coordgrid"
	"github.com/zsrv/goscape/pkg/pathfinder/routefinder"
)
```

> **Waypoint ordering note:** the pathfinder's `Route.Waypoints` slice ordering (source-to-dest vs dest-to-source) should be verified during implementation — inspect `pkg/pathfinder/routefinder/routefinder.go` for how waypoints are appended. If the ordering is source-to-dest, reverse the slice inside `routeToPacked` before returning.

- [ ] **Step 4: Run the tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run "TestPathToMoveClick" -v
```

Expected: both tests pass (they use `needsFinding=false`, so the pathfinder isn't invoked).

- [ ] **Step 5: Commit**

```bash
git add modules/world/movement.go modules/world/movement_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): add pathToMoveClick with SMART and NAIVE strategies

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 13: Update `handleMoveClick` to call `pathToMoveClick`

**Files:**
- Modify: `modules/world/handlers_game.go`

- [ ] **Step 1: Rewrite `handleMoveClick`**

Find `handleMoveClick` in `modules/world/handlers_game.go`. Replace the body:

```go
func handleMoveClick(p *Player, payload []byte) error {
	if len(payload) < 5 {
		return nil
	}
	r := packet.NewPacket(payload)
	ctrlHeld := r.G1()
	startX := int(r.G2())
	startZ := int(r.G2())

	pathLen := min((len(payload)-5)/2, 24) + 1
	packed := make([]int, 0, pathLen)
	packed = append(packed, coordgrid.PackCoord(p.level, startX, startZ))
	for range min((len(payload)-5)/2, 24) {
		dx := int(r.G1B())
		dz := int(r.G1B())
		packed = append(packed, coordgrid.PackCoord(p.level, startX+dx, startZ+dz))
	}

	p.client.log.Debug("move click", "ctrl_held", ctrlHeld, "dest_packed", packed[0])
	needsFinding := false
	if p.client.server != nil {
		needsFinding = !p.client.server.cfg.NodeClientRouteFinder
	}
	p.pathToMoveClick(packed, needsFinding)
	return nil
}
```

Same for `handleMoveMinimapClick` — replace:

```go
func handleMoveMinimapClick(p *Player, payload []byte) error {
	const trailingBytes = 14
	if len(payload) < 5+trailingBytes {
		return nil
	}
	r := packet.NewPacket(payload)
	ctrlHeld := r.G1()
	startX := int(r.G2())
	startZ := int(r.G2())

	pathLen := min((len(payload)-5-trailingBytes)/2, 24) + 1
	packed := make([]int, 0, pathLen)
	packed = append(packed, coordgrid.PackCoord(p.level, startX, startZ))
	for range min((len(payload)-5-trailingBytes)/2, 24) {
		dx := int(r.G1B())
		dz := int(r.G1B())
		packed = append(packed, coordgrid.PackCoord(p.level, startX+dx, startZ+dz))
	}

	p.client.log.Debug("minimap click", "ctrl_held", ctrlHeld, "dest_packed", packed[0])
	needsFinding := false
	if p.client.server != nil {
		needsFinding = !p.client.server.cfg.NodeClientRouteFinder
	}
	p.pathToMoveClick(packed, needsFinding)
	return nil
}
```

Add `"github.com/zsrv/goscape/pkg/coordgrid"` to the imports.

- [ ] **Step 2: Verify it compiles and existing tests pass**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... 2>&1 | tail -5
```

Expected: all pass.

- [ ] **Step 3: Commit**

```bash
git add modules/world/handlers_game.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): handleMoveClick now queues waypoints via pathToMoveClick

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 14: Add `processPathing` tick phase

**Files:**
- Modify: `modules/world/tick.go`

- [ ] **Step 1: Add `processPathing` to `tick.go`**

Open `modules/world/tick.go`. Add a new method:

```go
func (s *Server) processPathing() {
	s.playersMu.RLock()
	players := make([]*Player, len(s.playerLoop))
	copy(players, s.playerLoop)
	s.playersMu.RUnlock()

	for _, p := range players {
		p.resolveMovement()
	}
}
```

Update `runTickLoopWithRate` to call `processPathing` between `processClientsIn` and `processClientsOut`:

```go
s.processClientsIn()
s.processPathing()
s.processClientsOut()
s.currentTick++
```

- [ ] **Step 2: Verify existing tests pass**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... 2>&1 | tail -5
```

Expected: all pass (existing `TestTickLoopIncrementsCurrentTick` still works — `processPathing` is a no-op on zero players).

- [ ] **Step 3: Commit**

```bash
git add modules/world/tick.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): add processPathing tick phase

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 15: Implement `processLogouts`

**Files:**
- Modify: `modules/world/tick.go`
- Modify: `modules/world/server_test.go`

- [ ] **Step 1: Add timeout constants**

Add to `modules/world/tick.go`:

```go
const (
	timeoutNoResponse   = 100 // ticks = 60s
	timeoutNoConnection = 50  // ticks = 30s
)
```

- [ ] **Step 2: Write the failing test**

Append to `server_test.go`:

```go
func TestProcessLogoutsTimeoutMarksLoggingOut(t *testing.T) {
	s := newTestServer(t)
	c, clientConn := newTestClient(t)
	c.server = s
	c.state = ClientStateGame
	go io.Copy(io.Discard, clientConn) // drain any bytes

	p := newPlayer(c)
	c.player = p
	if err := s.addPlayer(p); err != nil {
		t.Fatal(err)
	}
	p.lastResponse = 0
	s.currentTick = timeoutNoResponse // exactly at threshold

	s.processLogouts()

	if !p.loggingOut {
		t.Error("loggingOut should be true after lastResponse timeout")
	}
	// Player should be removed from registry
	s.playersMu.RLock()
	still := len(s.playerLoop)
	s.playersMu.RUnlock()
	if still != 0 {
		t.Errorf("playerLoop should be empty after logout, got %d", still)
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestProcessLogouts -v
```

Expected: compile error — `processLogouts` undefined (currently a stub name not present).

- [ ] **Step 4: Add `processLogouts` to `tick.go`**

Add imports `gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"` if not already present.

```go
func (s *Server) processLogouts() {
	s.playersMu.RLock()
	players := make([]*Player, len(s.playerLoop))
	copy(players, s.playerLoop)
	s.playersMu.RUnlock()

	for _, p := range players {
		force := false
		if s.currentTick-p.lastResponse >= timeoutNoResponse {
			p.loggingOut = true
			force = true
		} else if s.currentTick-p.lastConnected >= timeoutNoConnection {
			p.requestIdleLogout = true
		}

		if p.requestLogout || p.requestIdleLogout {
			if s.currentTick >= p.preventLogoutUntil {
				p.loggingOut = true
			}
			p.requestLogout = false
			p.requestIdleLogout = false
		}

		if p.loggingOut && (force || s.currentTick >= p.preventLogoutUntil) {
			p.writeOut(gameserver.OpLogout, nil)
			_ = p.client.flushWrite()
			_ = p.client.conn.Close()
			s.removePlayer(p)
		}
	}
}
```

Call `processLogouts` in `runTickLoopWithRate` before `processClientsOut`:

```go
s.processClientsIn()
s.processPathing()
s.processLogouts()
s.processClientsOut()
s.currentTick++
```

- [ ] **Step 5: Run the tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestProcessLogouts -v
```

Expected: pass.

- [ ] **Step 6: Commit**

```bash
git add modules/world/tick.go modules/world/server_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): implement processLogouts with timeout detection

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 16: Add `newPlayers` queue and `appendNewPlayer`; change `sendLoginOK`

**Files:**
- Modify: `modules/world/server.go`
- Modify: `modules/world/client.go`
- Modify: `modules/world/server_test.go`

- [ ] **Step 1: Add `newPlayers` field and `appendNewPlayer` method**

In `server.go`, add to the `Server` struct:

```go
newPlayers []*Player  // guarded by playersMu
```

Add a method:

```go
// appendNewPlayer queues a player for registration on the next tick.
func (s *Server) appendNewPlayer(p *Player) {
	s.playersMu.Lock()
	s.newPlayers = append(s.newPlayers, p)
	s.playersMu.Unlock()
}
```

- [ ] **Step 2: Update `sendLoginOK` to use `appendNewPlayer`**

In `client.go`, replace the player-registration block in `sendLoginOK`:

```go
func (c *client) sendLoginOK() error {
	if c.server != nil {
		p := newPlayer(c)
		c.server.appendNewPlayer(p)
		c.player = p
	}

	if c.staffModLevel >= 1 {
		c.bufw.WriteByte(loginresp.OpLoginOKWithRights.Opcode)
	} else {
		c.bufw.WriteByte(loginresp.OpOK.Opcode)
	}
	if err := c.flushWrite(); err != nil {
		// appendNewPlayer doesn't take a slot yet, so no rollback needed — processLogins
		// will see the nil client pointer on its next attempt if we've closed.
		return fmt.Errorf("failed to flush login OK: %w", err)
	}
	c.state = ClientStateGame
	return nil
}
```

> **Behavior change:** in sub-spec 1, `sendLoginOK` returned `OpServerFull` if the registry was full. In sub-spec 2 the world-full check moves into `processLogins` (runs on next tick). A player whose login packet lands on a world-full tick still receives `OpOK`, then sees a logout packet on the next tick. This is a TS-faithful change.

- [ ] **Step 3: Update existing `TestSendLoginOKRegistersPlayer` test**

The old test checks that `s.players[p.slot] != nil` *immediately* after `sendLoginOK`. With sub-spec 2 behavior, the player is in `newPlayers` not `players`. Update the test:

Find the test in `server_test.go` and change:

```go
// Old (sub-spec 1): immediate registration
if s.players[c.player.slot] != c.player { ... }

// New (sub-spec 2): queued for next tick
s.playersMu.RLock()
queued := len(s.newPlayers)
s.playersMu.RUnlock()
if queued != 1 {
    t.Errorf("newPlayers queue length: got %d, want 1", queued)
}
if c.player == nil {
    t.Fatal("c.player should be set after sendLoginOK")
}
```

Also remove/update `TestSendLoginOKWorldFullReturnsError` — the world-full check moves to `processLogins`. Delete that test; it's replaced by a new test in Task 17.

- [ ] **Step 4: Run all tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... 2>&1 | tail -5
```

Expected: all pass.

- [ ] **Step 5: Commit**

```bash
git add modules/world/server.go modules/world/client.go modules/world/server_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): login queues into newPlayers instead of immediate addPlayer

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 17: Implement `processLogins`

**Files:**
- Modify: `modules/world/tick.go`
- Modify: `modules/world/server_test.go`

- [ ] **Step 1: Write the failing test**

Append to `server_test.go`:

```go
func TestProcessLoginsDrainsNewPlayers(t *testing.T) {
	s := newTestServer(t)
	c, clientConn := newTestClient(t)
	c.server = s
	go io.Copy(io.Discard, clientConn)

	p := newPlayer(c)
	c.player = p
	s.appendNewPlayer(p)

	s.processLogins()

	s.playersMu.RLock()
	queued := len(s.newPlayers)
	inLoop := len(s.playerLoop)
	s.playersMu.RUnlock()

	if queued != 0 {
		t.Errorf("newPlayers: got %d, want 0", queued)
	}
	if inLoop != 1 {
		t.Errorf("playerLoop: got %d, want 1", inLoop)
	}
	if p.slot < 1 {
		t.Errorf("slot: got %d, want >= 1", p.slot)
	}
}

func TestProcessLoginsWorldFullRejectsCleanly(t *testing.T) {
	s := newTestServer(t)

	// Fill all slots directly
	for i := 1; i < len(s.players); i++ {
		s.players[i] = &Player{slot: i}
		s.playerLoop = append(s.playerLoop, s.players[i])
	}

	c, clientConn := newTestClient(t)
	c.server = s
	go io.Copy(io.Discard, clientConn)
	p := newPlayer(c)
	c.player = p
	s.appendNewPlayer(p)

	s.processLogins()

	s.playersMu.RLock()
	queued := len(s.newPlayers)
	s.playersMu.RUnlock()

	if queued != 0 {
		t.Errorf("newPlayers should be drained even on world-full: got %d", queued)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestProcessLogins -v
```

Expected: compile error — `processLogins` undefined.

- [ ] **Step 3: Add `processLogins` to `tick.go`**

```go
func (s *Server) processLogins() {
	s.playersMu.Lock()
	batch := s.newPlayers
	s.newPlayers = nil
	s.playersMu.Unlock()

	for _, p := range batch {
		// addPlayer acquires its own lock — we released ours above.
		if err := s.addPlayer(p); err != nil {
			// world full — reject cleanly
			p.writeOut(gameserver.OpLogout, nil)
			_ = p.client.flushWrite()
			_ = p.client.conn.Close()
			continue
		}
		p.lastConnected = s.currentTick
		p.lastResponse = s.currentTick
		p.originX = p.x
		p.originZ = p.z
	}
}
```

Call it in `runTickLoopWithRate` after `processLogouts`:

```go
s.processClientsIn()
s.processPathing()
s.processLogouts()
s.processLogins()
s.processClientsOut()
s.currentTick++
```

- [ ] **Step 4: Run the tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestProcessLogins -v
```

Expected: both pass.

- [ ] **Step 5: Commit**

```bash
git add modules/world/tick.go modules/world/server_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): implement processLogins draining newPlayers queue

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 18: Integration test — MOVE_GAMECLICK advances a player

**Files:**
- Modify: `modules/world/movement_test.go`

- [ ] **Step 1: Write the integration test**

Append to `movement_test.go`:

```go
func TestMoveGameClickAdvancesPlayer(t *testing.T) {
	enc, dec := isaacPair([4]uint32{1, 2, 3, 4})
	p, _ := newTestPlayer(t)
	p.client.decryptor = dec
	p.client.encryptor = enc
	p.x, p.z, p.level = 3094, 3106, 0
	p.moveSpeed = MoveSpeedWalk

	// MOVE_GAMECLICK: opcode 181, 1-byte length prefix
	// Payload: ctrlHeld(1) + startX G2(2) + startZ G2(2) = 5 bytes
	// Move to (3094, 3107) — one step north
	payload := []byte{
		0,          // ctrlHeld
		0x0C, 0x16, // startX = 3094
		0x0C, 0x23, // startZ = 3107
	}
	buf := []byte{encryptOpcode(enc, 181), byte(len(payload))}
	buf = append(buf, payload...)
	p.client.in.Write(buf)

	// Process input — dispatches handleMoveClick which calls pathToMoveClick
	p.processIn(0)

	if p.waypointIndex < 0 {
		t.Fatal("pathToMoveClick should have queued a waypoint")
	}

	// Process pathing — advances one tile
	p.resolveMovement()

	if p.z != 3107 {
		t.Errorf("after tick, z: got %d, want 3107", p.z)
	}
}
```

> **Note:** the exact payload bytes depend on the G2 encoding. The above assumes big-endian unsigned. 3094 in hex is 0x0C16; 3107 is 0x0C23. If the existing test-harness pattern shows different byte ordering, match it.

- [ ] **Step 2: Run the test**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestMoveGameClickAdvancesPlayer -v
```

Expected: pass. If payload decoding fails, inspect with `t.Logf("%+v", p)` and adjust the bytes.

- [ ] **Step 3: Commit**

```bash
git add modules/world/movement_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
test(world): integration test MOVE_GAMECLICK advances player one tile

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 19: Final verification

- [ ] **Step 1: Run the complete test suite**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
```

Expected: all `ok`, no failures.

- [ ] **Step 2: Run with the race detector**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...
```

Expected: all pass, zero races.

- [ ] **Step 3: Verify the binary builds**

```bash
CGO_ENABLED=0 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build -trimpath ./cmd/goscape && rm -f goscape
```

Expected: no errors.
