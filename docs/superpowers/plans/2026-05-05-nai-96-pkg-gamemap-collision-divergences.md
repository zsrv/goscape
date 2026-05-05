# NAI-96 — `pkg/gamemap/` collision-write divergences Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port four TS-fidelity divergences in `pkg/gamemap/` surfaced from NAI-95 user-launched smoke: BLOCK_MAP_SQUARE constant flip, LINK_BELOW bridge-tile handling, REMOVE_ROOFS roof-collision write, GroundDecor active=1 ChangeFloor write, and angle-aware width/length swap in LayerGround.

**Architecture:** Two parallel tasks under one sub-spec. Task 1 restructures `loadGround` into a two-pass parser (lands array → collision writes) and threads the lands array into `loadLocs` for shared LINK_BELOW handling, plus the REMOVE_ROOFS write. Task 2 adds an `active int` parameter to `ChangeLocCollision`, adds the LayerGroundDecor branch, fixes the LayerGround angle-swap, and threads `lt.Active` through all 7 caller sites in `modules/world/`. Tasks touch mostly-disjoint files; parallel-dispatchable.

**Tech Stack:** Go 1.26+.

**Spec:** `docs/superpowers/specs/2026-05-05-nai-96-pkg-gamemap-collision-divergences-design.md`.

**TS reference:** `LostCityRS/Engine-TS/src/engine/GameMap.ts:24-26, 182-217, 220-267, 326-341` (per `ts_source_canonical_path`).

**Pre-dispatch verification (controller, before each implementer):**
1. `pkg/gamemap/load.go:9` still has `gameMapBlockMapSquare = 0x2`.
2. `pkg/gamemap/gamemap.go::ChangeLocCollision` signature is `(shape, angle int, blocksRange bool, length, width, x, z, level int, add bool)`.
3. The 7 `ChangeLocCollision` call sites are at: `world_zone.go:20, 49, 56, 82`, `loc_turn.go:42, 49`, `server.go:328`. Re-grep with `rg "ChangeLocCollision\(" --type go` and confirm count and lines.
4. `loc.LayerGroundDecor`, `loc.AngleNorth`, `loc.AngleSouth` exist at `pkg/pathfinder/loc/{layer,angle}.go`.
5. `Pathfinder.ChangeRoof(x, z, level int, add bool)` signature matches `pkg/pathfinder/routefinder/api.go:127`.

---

## File Structure

**Task 1 — `loadGround` two-pass + LINK_BELOW + REMOVE_ROOFS:**
- Modify: `pkg/gamemap/load.go` (constants, `loadGround` rewrite, `loadLocs` lands consumption)
- Modify: `pkg/gamemap/gamemap.go:22-43` (add `landsByMapSquare` field + initializer in `New`; ~3 lines)
- Create: `pkg/gamemap/load_test.go` (~7 unit tests with hand-crafted byte streams)

**Task 2 — `ChangeLocCollision` active param + GroundDecor + angle-swap:**
- Modify: `pkg/gamemap/gamemap.go::ChangeLocCollision` (signature + body)
- Modify: `modules/world/world_zone.go` (4 call sites)
- Modify: `modules/world/loc_turn.go` (1 site, 2 calls)
- Modify: `modules/world/server.go` (1 call site in `populateStaticLocsIntoZones`)
- Modify: `modules/world/static_loc_collision_test.go` (extend with 5 new subtests)

**File-collision surface between tasks:** `pkg/gamemap/gamemap.go`. Task 1 edits the `GameMap` struct + `New` constructor (lines 22-43). Task 2 edits the `ChangeLocCollision` method (lines 50-61). Distinct line ranges; merge-conflict-free if auto-merged in either order. Parallel dispatch is safe.

---

## Task 1: `loadGround` two-pass + LINK_BELOW + REMOVE_ROOFS

**Files:**
- Modify: `pkg/gamemap/load.go`
- Modify: `pkg/gamemap/gamemap.go:22-43`
- Create: `pkg/gamemap/load_test.go`

### Task 1.1: Add `landsByMapSquare` field to `GameMap` struct and initializer

- [ ] **Step 1: Modify `pkg/gamemap/gamemap.go::GameMap` struct (lines 22-31) — add `landsByMapSquare` field**

Edit at line 28 (between `lData` and `staticLocs`):

```go
// GameMap holds collision data for the game world.
type GameMap struct {
	Pathfinder       *routefinder.PathFinderAPI
	multimap         map[int]bool      // packed zone coord -> multi combat
	freemap          map[int]bool      // packed zone coord -> F2P
	mData            map[uint16][]byte // (mapX<<8)|mapZ -> raw m{x}_{z} bytes (sub-spec 5b)
	lData            map[uint16][]byte // (mapX<<8)|mapZ -> raw l{x}_{z} bytes (sub-spec 5b)
	landsByMapSquare map[uint16][]int8 // (mapX<<8)|mapZ -> mapLevels*64*64 land bytes; populated by loadGround, consumed by loadLocs (NAI-96 LINK_BELOW)
	staticLocs       []*entity.Loc     // parsed static locs with absolute world coords
	npcSpawns        []NpcSpawn
	log              *slog.Logger
}
```

- [ ] **Step 2: Modify `pkg/gamemap/gamemap.go::New` (lines 33-43) — initialize `landsByMapSquare`**

```go
func New(log *slog.Logger) *GameMap {
	pf := routefinder.NewPathFinderAPI()
	return &GameMap{
		Pathfinder:       &pf,
		multimap:         make(map[int]bool),
		freemap:          make(map[int]bool),
		mData:            map[uint16][]byte{},
		lData:            map[uint16][]byte{},
		landsByMapSquare: map[uint16][]int8{},
		log:              log,
	}
}
```

- [ ] **Step 3: Verify compile**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./pkg/gamemap/...`
Expected: PASS (no functional change yet — field unused).

- [ ] **Step 4: Commit**

```bash
git add pkg/gamemap/gamemap.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
chore(gamemap): NAI-96 — add landsByMapSquare field for LINK_BELOW threading

Empty buffer until loadGround populates and loadLocs consumes it in the
follow-up commits. No behavior change.
EOF
)"
```

### Task 1.2: TDD — add land-byte fixture helper and BLOCK_MAP_SQUARE flip test

- [ ] **Step 1: Create `pkg/gamemap/load_test.go` with the helper and the first failing test**

```go
package gamemap

import (
	"bytes"
	"io"
	"log/slog"
	"testing"

	"github.com/zsrv/goscape/pkg/pathfinder/collision"
)

// mFileWithLand returns the bytes of an m-file where exactly one tile
// (level, localX, localZ) within mapsquare (mapX, mapZ) carries the given
// land value. All other tiles terminate with opcode 0 (no land).
//
// Encoding per loadGround's parser (load.go):
//   - opcode 0: terminate tile (no land set; lands[idx] stays 0)
//   - opcode 50+land: terminate tile with land = opcode - 49
//
// Iteration order is level outer, x middle, z inner — matches loadGround.
// The packCoord index is the same as the parser's packCoord helper.
func mFileWithLand(targetLevel, targetX, targetZ int, land byte) []byte {
	var buf bytes.Buffer
	for level := 0; level < mapLevels; level++ {
		for x := 0; x < mapSquareSize; x++ {
			for z := 0; z < mapSquareSize; z++ {
				if level == targetLevel && x == targetX && z == targetZ {
					buf.WriteByte(49 + land) // opcode encoding land
				} else {
					buf.WriteByte(0) // empty tile
				}
			}
		}
	}
	return buf.Bytes()
}

// mFileWithLands writes multiple (level,x,z,land) entries; all other tiles empty.
func mFileWithLands(entries map[[3]int]byte) []byte {
	var buf bytes.Buffer
	for level := 0; level < mapLevels; level++ {
		for x := 0; x < mapSquareSize; x++ {
			for z := 0; z < mapSquareSize; z++ {
				if land, ok := entries[[3]int{level, x, z}]; ok {
					buf.WriteByte(49 + land)
				} else {
					buf.WriteByte(0)
				}
			}
		}
	}
	return buf.Bytes()
}

func newTestGameMap() *GameMap {
	return New(slog.New(slog.NewTextHandler(io.Discard, nil)))
}

// TestLoadGround_BlockMapSquare_WritesFloorBlock pins the BLOCK_MAP_SQUARE constant
// flip (NAI-96): a tile with land=0x1 must mark FlagBlockWalk via ChangeFloor.
// Pre-fix (gameMapBlockMapSquare=0x2): land=0x1 is silently ignored.
func TestLoadGround_BlockMapSquare_WritesFloorBlock(t *testing.T) {
	const mapX, mapZ = 50, 50 // arbitrary mapsquare; absolute = (mapX*64+x, mapZ*64+z)
	const localX, localZ = 1, 1
	const targetLevel = 0
	absX := mapX*mapSquareSize + localX
	absZ := mapZ*mapSquareSize + localZ

	gm := newTestGameMap()
	// Pre-allocate the touched zone so flag reads return real values, not FlagNull.
	gm.Pathfinder.Flags.AllocateIfAbsent(absX, absZ, targetLevel)

	gm.loadGround(mFileWithLand(targetLevel, localX, localZ, 0x1), mapX, mapZ)

	flag := gm.Pathfinder.Flags.Get(absX, absZ, targetLevel)
	if flag&collision.FlagBlockWalk == 0 {
		t.Errorf("tile (%d, %d, %d) land=0x1: flag=0x%x missing FlagBlockWalk (0x%x)",
			absX, absZ, targetLevel, flag, collision.FlagBlockWalk)
	}
}
```

- [ ] **Step 2: Run test to verify it FAILS at HEAD**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/gamemap/ -run TestLoadGround_BlockMapSquare_WritesFloorBlock -v`
Expected: FAIL — `flag=0x0 missing FlagBlockWalk` (because at HEAD, `gameMapBlockMapSquare = 0x2` so land=0x1 is ignored).

- [ ] **Step 3: Apply minimal fix — flip the constant**

Edit `pkg/gamemap/load.go` lines 8-13:

```go
const (
	gameMapBlockMapSquare = 0x1 // BLOCK_MAP_SQUARE — marks a tile as blocked floor (TS GameMap.ts:24)
	gameMapLinkBelow      = 0x2 // LINK_BELOW — bridge tile; collision drops to level-1 (TS GameMap.ts:25)
	gameMapRemoveRoofs    = 0x4 // REMOVE_ROOFS — write roof collision (TS GameMap.ts:26)

	mapSquareSize = 64
	mapLevels     = 4
)
```

- [ ] **Step 4: Run test to verify it PASSES**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/gamemap/ -run TestLoadGround_BlockMapSquare_WritesFloorBlock -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/gamemap/load.go pkg/gamemap/load_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
fix(gamemap): NAI-96 — flip BLOCK_MAP_SQUARE constant 0x2 → 0x1

TS Engine-TS/src/engine/GameMap.ts:24 defines BLOCK_MAP_SQUARE=0x1; goscape
inverted with LINK_BELOW=0x2. NAI-95 user-launched smoke surfaced this as
tile (3219, 3223) on Hans straight-line path getting spurious FlagBlockWalk,
forcing a 3-waypoint detour.

Adds gameMapLinkBelow=0x2 and gameMapRemoveRoofs=0x4 constants for the
follow-up two-pass restructure.
EOF
)"
```

### Task 1.3: TDD — REMOVE_ROOFS write (forces two-pass restructure)

- [ ] **Step 1: Add the failing test**

Append to `pkg/gamemap/load_test.go`:

```go
// TestLoadGround_RemoveRoofs_WritesRoof pins the REMOVE_ROOFS=0x4 →
// Pathfinder.ChangeRoof write per TS GameMap.ts:200-202.
func TestLoadGround_RemoveRoofs_WritesRoof(t *testing.T) {
	const mapX, mapZ = 50, 50
	const localX, localZ = 1, 1
	const targetLevel = 0
	absX := mapX*mapSquareSize + localX
	absZ := mapZ*mapSquareSize + localZ

	gm := newTestGameMap()
	gm.Pathfinder.Flags.AllocateIfAbsent(absX, absZ, targetLevel)

	gm.loadGround(mFileWithLand(targetLevel, localX, localZ, 0x4), mapX, mapZ)

	flag := gm.Pathfinder.Flags.Get(absX, absZ, targetLevel)
	if flag&collision.FlagRoof == 0 {
		t.Errorf("tile (%d, %d, %d) land=0x4: flag=0x%x missing FlagRoof (0x%x)",
			absX, absZ, targetLevel, flag, collision.FlagRoof)
	}
	if flag&collision.FlagBlockWalk != 0 {
		t.Errorf("tile (%d, %d, %d) land=0x4: flag=0x%x unexpectedly has FlagBlockWalk",
			absX, absZ, targetLevel, flag)
	}
}

// TestLoadGround_BlockAndRemoveRoofs_BothWritten pins that land=0x5
// (BLOCK_MAP_SQUARE | REMOVE_ROOFS) writes both flags.
func TestLoadGround_BlockAndRemoveRoofs_BothWritten(t *testing.T) {
	const mapX, mapZ = 50, 50
	const localX, localZ = 1, 1
	const targetLevel = 0
	absX := mapX*mapSquareSize + localX
	absZ := mapZ*mapSquareSize + localZ

	gm := newTestGameMap()
	gm.Pathfinder.Flags.AllocateIfAbsent(absX, absZ, targetLevel)

	gm.loadGround(mFileWithLand(targetLevel, localX, localZ, 0x5), mapX, mapZ)

	flag := gm.Pathfinder.Flags.Get(absX, absZ, targetLevel)
	if flag&collision.FlagRoof == 0 {
		t.Errorf("flag=0x%x missing FlagRoof", flag)
	}
	if flag&collision.FlagBlockWalk == 0 {
		t.Errorf("flag=0x%x missing FlagBlockWalk", flag)
	}
}
```

- [ ] **Step 2: Run tests to verify they FAIL**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/gamemap/ -run "TestLoadGround_RemoveRoofs|TestLoadGround_BlockAndRemoveRoofs" -v`
Expected: FAIL — current `loadGround` only writes BLOCK_MAP_SQUARE, no REMOVE_ROOFS branch.

- [ ] **Step 3: Restructure `loadGround` to two-pass with REMOVE_ROOFS write**

Replace `pkg/gamemap/load.go::loadGround` (currently lines 24-63) and add the `packCoord` helper. Final shape:

```go
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
//	opcode 0:     end of tile (lands[idx] stays 0)
//	opcode 1:     1-byte height follows; ends tile
//	opcode 2..49: overlay data (3 bytes skipped); continues
//	opcode 50+:   direct land = opcode - 49; ends tile
func (gm *GameMap) loadGround(data []byte, mapSquareX, mapSquareZ int) {
	p := packet.NewPacket(data)
	lands := make([]int8, mapLevels*mapSquareSize*mapSquareSize)

	// Pass 1 — parse opcodes into lands.
parseLoop:
	for level := 0; level < mapLevels; level++ {
		for x := 0; x < mapSquareSize; x++ {
			for z := 0; z < mapSquareSize; z++ {
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
						if p.Len() >= 3 {
							_ = p.Next(3) // overlay (id, rot, underlay)
						}
						continue
					}
					lands[packCoord(x, z, level)] = int8(op) - 49
					break
				}
			}
		}
	}
	gm.landsByMapSquare[uint16((mapSquareX<<8)|mapSquareZ)] = lands

	// Pass 2 — write collision flags.
	for level := 0; level < mapLevels; level++ {
		for x := 0; x < mapSquareSize; x++ {
			absX := mapSquareX*mapSquareSize + x
			for z := 0; z < mapSquareSize; z++ {
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
```

- [ ] **Step 4: Run tests to verify they PASS**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/gamemap/ -run "TestLoadGround" -v`
Expected: PASS for `TestLoadGround_BlockMapSquare_WritesFloorBlock`, `TestLoadGround_RemoveRoofs_WritesRoof`, `TestLoadGround_BlockAndRemoveRoofs_BothWritten`.

- [ ] **Step 5: Run the full pkg/gamemap suite to catch regressions**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/gamemap/...`
Expected: PASS. The two-pass restructure is functionally equivalent for the existing pre-NAI-96 test surface (which doesn't probe lands beyond opcode-0 minimal fixtures).

- [ ] **Step 6: Commit**

```bash
git add pkg/gamemap/load.go pkg/gamemap/load_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(gamemap): NAI-96 — two-pass loadGround with REMOVE_ROOFS write

Restructures loadGround into TS-faithful two-pass parse:
  - Pass 1: opcode stream → per-mapsquare lands[] buffer (4*64*64 int8)
  - Pass 2: per-tile FlagRoof write (REMOVE_ROOFS=0x4) and FlagBlockWalk
    write (BLOCK_MAP_SQUARE=0x1) with bridged-level adjustment.

Persists lands buffer on GameMap.landsByMapSquare for loadLocs to consume
in the follow-up commit.

Mirrors TS Engine-TS/src/engine/GameMap.ts:182-217.
EOF
)"
```

### Task 1.4: TDD — LINK_BELOW bridged-level adjustment

- [ ] **Step 1: Add three failing tests for bridged-level handling**

Append to `pkg/gamemap/load_test.go`:

```go
// TestLoadGround_BridgedLevel0_DropsToLevelMinus1_Skipped pins that a level-0
// BLOCK_MAP_SQUARE with the level-1 land carrying LINK_BELOW becomes
// actualLevel=-1 and is silently skipped (TS GameMap.ts:208-212).
func TestLoadGround_BridgedLevel0_DropsToLevelMinus1_Skipped(t *testing.T) {
	const mapX, mapZ = 50, 50
	const localX, localZ = 1, 1
	absX := mapX*mapSquareSize + localX
	absZ := mapZ*mapSquareSize + localZ

	gm := newTestGameMap()
	gm.Pathfinder.Flags.AllocateIfAbsent(absX, absZ, 0)

	// Level 0: BLOCK_MAP_SQUARE only; level 1 same (x,z): LINK_BELOW only.
	data := mFileWithLands(map[[3]int]byte{
		{0, localX, localZ}: 0x1,
		{1, localX, localZ}: 0x2,
	})
	gm.loadGround(data, mapX, mapZ)

	flag := gm.Pathfinder.Flags.Get(absX, absZ, 0)
	if flag&collision.FlagBlockWalk != 0 {
		t.Errorf("level 0 bridged tile: flag=0x%x unexpectedly has FlagBlockWalk (actualLevel=-1 should skip)", flag)
	}
}

// TestLoadGround_BridgedLevel1_WritesAtLevel0 pins TS GameMap.ts:208 — when
// level=1 land has both BLOCK_MAP_SQUARE and LINK_BELOW, write at level 0.
func TestLoadGround_BridgedLevel1_WritesAtLevel0(t *testing.T) {
	const mapX, mapZ = 50, 50
	const localX, localZ = 1, 1
	absX := mapX*mapSquareSize + localX
	absZ := mapZ*mapSquareSize + localZ

	gm := newTestGameMap()
	gm.Pathfinder.Flags.AllocateIfAbsent(absX, absZ, 0)
	gm.Pathfinder.Flags.AllocateIfAbsent(absX, absZ, 1)

	// Level 1: BLOCK_MAP_SQUARE | LINK_BELOW.
	data := mFileWithLand(1, localX, localZ, 0x3)
	gm.loadGround(data, mapX, mapZ)

	flag0 := gm.Pathfinder.Flags.Get(absX, absZ, 0)
	flag1 := gm.Pathfinder.Flags.Get(absX, absZ, 1)
	if flag0&collision.FlagBlockWalk == 0 {
		t.Errorf("level 0 (post-bridge target): flag=0x%x missing FlagBlockWalk", flag0)
	}
	if flag1&collision.FlagBlockWalk != 0 {
		t.Errorf("level 1 (bridged origin): flag=0x%x unexpectedly has FlagBlockWalk", flag1)
	}
}

// TestLoadGround_NonBridgedLevel1_WritesAtLevel1 pins the inverse: level=1
// BLOCK_MAP_SQUARE without LINK_BELOW writes at level 1 (no bridge drop).
func TestLoadGround_NonBridgedLevel1_WritesAtLevel1(t *testing.T) {
	const mapX, mapZ = 50, 50
	const localX, localZ = 1, 1
	absX := mapX*mapSquareSize + localX
	absZ := mapZ*mapSquareSize + localZ

	gm := newTestGameMap()
	gm.Pathfinder.Flags.AllocateIfAbsent(absX, absZ, 0)
	gm.Pathfinder.Flags.AllocateIfAbsent(absX, absZ, 1)

	data := mFileWithLand(1, localX, localZ, 0x1) // BLOCK only, no LINK_BELOW
	gm.loadGround(data, mapX, mapZ)

	flag1 := gm.Pathfinder.Flags.Get(absX, absZ, 1)
	flag0 := gm.Pathfinder.Flags.Get(absX, absZ, 0)
	if flag1&collision.FlagBlockWalk == 0 {
		t.Errorf("level 1 (non-bridged): flag=0x%x missing FlagBlockWalk", flag1)
	}
	if flag0&collision.FlagBlockWalk != 0 {
		t.Errorf("level 0 (no bridge): flag=0x%x unexpectedly has FlagBlockWalk", flag0)
	}
}

// TestLoadGround_LinkBelowOnly_DoesNotBlock pins that LINK_BELOW alone (no
// BLOCK_MAP_SQUARE) does not write FlagBlockWalk. Confirms the constant flip
// distinguishes the two bits per TS GameMap.ts:204.
func TestLoadGround_LinkBelowOnly_DoesNotBlock(t *testing.T) {
	const mapX, mapZ = 50, 50
	const localX, localZ = 1, 1
	absX := mapX*mapSquareSize + localX
	absZ := mapZ*mapSquareSize + localZ

	gm := newTestGameMap()
	gm.Pathfinder.Flags.AllocateIfAbsent(absX, absZ, 0)

	gm.loadGround(mFileWithLand(0, localX, localZ, 0x2), mapX, mapZ)

	flag := gm.Pathfinder.Flags.Get(absX, absZ, 0)
	if flag&collision.FlagBlockWalk != 0 {
		t.Errorf("LINK_BELOW-only tile: flag=0x%x unexpectedly has FlagBlockWalk", flag)
	}
}
```

- [ ] **Step 2: Run tests to verify they PASS**

The Task 1.3 implementation already includes the LINK_BELOW logic (`bridgeLand`, `actualLevel`). These tests are regression guards.

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/gamemap/ -run "TestLoadGround_Bridged|TestLoadGround_NonBridged|TestLoadGround_LinkBelowOnly" -v`
Expected: PASS for all four tests.

- [ ] **Step 3: Commit**

```bash
git add pkg/gamemap/load_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
test(gamemap): NAI-96 — pin LINK_BELOW bridged-level adjustment

Four regression guards for the two-pass loadGround LINK_BELOW logic:
  - level 0 bridged → actualLevel=-1 skipped
  - level 1 bridged → write at level 0
  - level 1 non-bridged → write at level 1
  - LINK_BELOW alone → no write (constant flip pin)
EOF
)"
```

### Task 1.5: TDD — `loadLocs` LINK_BELOW level adjustment

- [ ] **Step 1: Add failing test for bridged loc placement**

Append to `pkg/gamemap/load_test.go`:

```go
// lFileWithOneLoc returns the bytes of an l-file placing one loc.
//   - locId is the absolute loc id (delta = locId+1, since prevId starts at -1)
//   - level/localX/localZ encode coord
//   - shape, angle pack into info = (shape<<2)|angle
//
// Stream shape (per loadLocs):
//   gsmart(locDelta=locId+1)
//   gsmart(coordDelta = packedCoord+1)
//   g1(info)
//   gsmart(0)        // end of coords for this loc
//   gsmart(0)        // end of locs
func lFileWithOneLoc(locId, level, localX, localZ, shape, angle int) []byte {
	pw := packet.NewPacket(nil)
	pw.PSmart(int32(locId + 1))
	packed := (localZ & 0x3F) | ((localX & 0x3F) << 6) | ((level & 0x3) << 12)
	pw.PSmart(int32(packed + 1))
	pw.P1(uint8((shape << 2) | (angle & 0x3)))
	pw.PSmart(0) // end coords
	pw.PSmart(0) // end locs
	return pw.Data
}

// TestLoadLocs_BridgedLoc_PlacedAtActualLevel pins that a loc with the
// LINK_BELOW bit set on its corresponding lands tile is downshifted by one
// level on the staticLocs entity (TS GameMap.ts:242-246).
func TestLoadLocs_BridgedLoc_PlacedAtActualLevel(t *testing.T) {
	const mapX, mapZ = 50, 50
	const localX, localZ = 1, 1
	const locId = 0
	const shape = int(loc.ShapeCentrepieceStraight) // LayerGround
	const angle = int(loc.AngleNorth)

	gm := newTestGameMap()

	// loadGround populates landsByMapSquare with level 1 LINK_BELOW set at (1,1,1).
	mData := mFileWithLand(1, localX, localZ, 0x2)
	gm.loadGround(mData, mapX, mapZ)

	// loadLocs places the loc at level 1 (request level), but lands[(1,1,1)]
	// has LINK_BELOW set, so actualLevel = 0.
	lData := lFileWithOneLoc(locId, 1, localX, localZ, shape, angle)
	gm.loadLocs(lData, mapX, mapZ)

	if len(gm.staticLocs) != 1 {
		t.Fatalf("expected 1 static loc; got %d", len(gm.staticLocs))
	}
	got := gm.staticLocs[0]
	if got.Level != 0 {
		t.Errorf("bridged loc: level=%d, want 0 (actualLevel = level-1)", got.Level)
	}
}
```

This test imports `pkg/pathfinder/loc` for `ShapeCentrepieceStraight` and `AngleNorth`, plus `pkg/io/packet` for `PSmart`/`P1`. Add to `pkg/gamemap/load_test.go` imports:

```go
import (
	"bytes"
	"io"
	"log/slog"
	"testing"

	"github.com/zsrv/goscape/pkg/io/packet"
	"github.com/zsrv/goscape/pkg/pathfinder/collision"
	"github.com/zsrv/goscape/pkg/pathfinder/loc"
)
```

- [ ] **Step 2: Run test to verify it FAILS**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/gamemap/ -run TestLoadLocs_BridgedLoc -v`
Expected: FAIL — current `loadLocs` writes the loc at the requested level (1), not actualLevel (0).

- [ ] **Step 3: Modify `loadLocs` to apply LINK_BELOW**

Edit `pkg/gamemap/load.go::loadLocs`. The change is to look up `lands := gm.landsByMapSquare[key]` once at the top and apply LINK_BELOW per coord. Final shape:

```go
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
// Footprint hardcoded to 1x1 until LocType config loading lands.
// TODO(loctype): use LocType.Width/Length.
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
			if lands != nil {
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

			loc := entity.NewLoc(actualLevel, absX, absZ, 1, 1,
				entity.LifecycleRespawn,
				locID, shape, angle)
			gm.staticLocs = append(gm.staticLocs, loc)
		}
	}
}
```

Also remove the `TODO(bridged-levels)` line at the existing location (above the function).

- [ ] **Step 4: Run test to verify PASS**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/gamemap/ -run TestLoadLocs_BridgedLoc -v`
Expected: PASS.

- [ ] **Step 5: Run full pkg/gamemap suite to confirm no regression**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/gamemap/...`
Expected: PASS — existing `TestLoadLocsParsesKnownFixture`, `TestLoadLocsMultipleLocIDs`, `TestLoadLocsEmptyFile` operate on synthetic data with no LINK_BELOW set, so `lands` is nil or has no LINK_BELOW; `actualLevel = level` preserves prior behavior.

- [ ] **Step 6: Run the broader suite to catch cross-package regressions**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`
Expected: PASS. NAI-95 `TestNAI95_StaticLocCollision_HansArea` may now produce a tighter Hans path (waypoint count drops); the existing test only requires `len(Waypoints) >= 1` so it remains green.

- [ ] **Step 7: Commit**

```bash
git add pkg/gamemap/load.go pkg/gamemap/load_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(gamemap): NAI-96 — LINK_BELOW level adjustment in loadLocs

loadLocs now consults the lands buffer populated by loadGround. When the
loc's lands tile carries LINK_BELOW (level==1: same coord; otherwise level-1
of the same x,z), the loc is downshifted by one level — matching TS
Engine-TS/src/engine/GameMap.ts:242-246. actualLevel<0 silently skips.

Removes the TODO(bridged-levels) marker.
EOF
)"
```

### Task 1.6: Verify Task 1 close

- [ ] **Step 1: Run all gamemap tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/gamemap/... -v`
Expected: all 7 new `TestLoadGround_*` / `TestLoadLocs_BridgedLoc_*` tests PASS, plus all pre-existing tests PASS.

- [ ] **Step 2: Run race detector on the package**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./pkg/gamemap/...`
Expected: PASS.

- [ ] **Step 3: Build the full module**

Run: `CGO_ENABLED=0 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build -trimpath ./...`
Expected: PASS.

---

## Task 2: `ChangeLocCollision` active param + GroundDecor + angle-swap

**Files:**
- Modify: `pkg/gamemap/gamemap.go::ChangeLocCollision` (lines 50-61)
- Modify: `modules/world/world_zone.go` (lines 20-21, 49-50, 56-57, 82-83)
- Modify: `modules/world/loc_turn.go` (lines 42-43, 49-50)
- Modify: `modules/world/server.go` (line 328-329)
- Modify: `modules/world/static_loc_collision_test.go` (extend)

### Task 2.1: TDD — GroundDecor active=1 ChangeFloor write

- [ ] **Step 1: Add the failing test to `modules/world/static_loc_collision_test.go`**

Append a new top-level test (don't nest under existing `TestNAI95_*`):

```go
// TestNAI96_GroundDecor_Active1_WritesFloor pins TS GameMap.ts:336-340 —
// LocLayer.GROUND_DECOR + active==1 writes ChangeFloor (FlagBlockWalk).
func TestNAI96_GroundDecor_Active1_WritesFloor(t *testing.T) {
	s := newTestServer(t)
	s.gamemap = gamemap.New(discardLogger())

	// Build a synthetic LocType: BlockWalk=true, Active=1, BlockRange=false.
	// LocType.Configs is indexed by typeId; index 0 reserved by convention.
	lt := &objtype.LocType{BlockWalk: true, Active: 1}
	s.locTypes = &objtype.LocTypes{Configs: []*objtype.LocType{nil, lt}}

	// Static loc with GroundDecor shape (ShapeGroundDecor=22) at (3220, 3220, 0).
	// Width/Length 1x1 (matches load.go convention).
	const absX, absZ, level = 3220, 3220, 0
	staticLoc := entity.NewLoc(level, absX, absZ, 1, 1,
		entity.LifecycleRespawn,
		1, /*locId*/
		int(loc.ShapeGroundDecor),
		int(loc.AngleWest))
	s.gamemap.AddStaticLoc(staticLoc)

	// Pre-allocate the touched zone so flag reads return real values.
	s.gamemap.Pathfinder.Flags.AllocateIfAbsent(absX, absZ, level)

	s.populateStaticLocsIntoZones()

	flag := s.gamemap.Pathfinder.Flags.Get(absX, absZ, level)
	if flag&collision.FlagBlockWalk == 0 {
		t.Errorf("GroundDecor active=1 at (%d, %d, %d): flag=0x%x missing FlagBlockWalk (0x%x)",
			absX, absZ, level, flag, collision.FlagBlockWalk)
	}
}
```

Imports needed at top of `static_loc_collision_test.go` (some already present):

```go
import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zsrv/goscape/pkg/entity"
	"github.com/zsrv/goscape/pkg/gamemap"
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/pathfinder/collision"
	"github.com/zsrv/goscape/pkg/pathfinder/loc"
)
```

- [ ] **Step 2: Run test to verify FAILS**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestNAI96_GroundDecor_Active1 -v`
Expected: FAIL — current `ChangeLocCollision` switch has no `LayerGroundDecor` case; flag stays 0.

- [ ] **Step 3: Add the `active` parameter to `ChangeLocCollision` and the GroundDecor branch**

Edit `pkg/gamemap/gamemap.go`. Replace the function (currently lines 50-61):

```go
// ChangeLocCollision updates collision for a loc based on its layer.
// Mirrors TS Engine-TS/src/engine/GameMap.ts:326-341 changeLocCollision.
//
// LayerWall:        ChangeWall (writes FlagWall* by angle).
// LayerGround:      ChangeLoc with angle-aware (length,width) swap.
// LayerGroundDecor: ChangeFloor when active==1 (writes FlagBlockWalk).
// LayerWallDecor:   no-op (TS skips: GameMap.ts:326-340 has no WALL_DECOR branch).
//
// `active` is the LocType.Active field (0 or 1 after PostDecode).
func (gm *GameMap) ChangeLocCollision(shape, angle int, blocksRange bool, length, width, active, x, z, level int, add bool) {
	layer := loc.LayerOf(loc.Shape(shape))
	switch layer {
	case loc.LayerWall:
		gm.Pathfinder.ChangeWall(x, z, level, angle, shape, blocksRange, false, add)
	case loc.LayerGround:
		if angle == loc.AngleNorth || angle == loc.AngleSouth {
			gm.Pathfinder.ChangeLoc(x, z, level, length, width, blocksRange, false, add)
		} else {
			gm.Pathfinder.ChangeLoc(x, z, level, width, length, blocksRange, false, add)
		}
	case loc.LayerGroundDecor:
		if active == 1 {
			gm.Pathfinder.ChangeFloor(x, z, level, add)
		}
	}
	// LayerWallDecor: TS skips (GameMap.ts:326-340 has no WALL_DECOR branch).
}
```

Note: `loc.AngleNorth` and `loc.AngleSouth` are untyped `int` constants (verified at `pkg/pathfinder/loc/angle.go`), so the `==` comparison is direct.

- [ ] **Step 4: Update all 7 caller sites to thread `lt.Active`**

The callers will fail to compile until updated. Update them in this order so each commit is buildable. The threaded value goes between `loc.Width` (or `l.Width`) and `loc.X` (or `l.X`).

Edit `modules/world/world_zone.go:20-21` (`AddLoc`):

```go
		s.gamemap.ChangeLocCollision(loc.Shape(), loc.Angle(), lt.BlockRange,
			loc.Length, loc.Width, lt.Active, loc.X, loc.Z, loc.Level, true)
```

Edit `modules/world/world_zone.go:49-50` (`ChangeLoc` remove old):

```go
			s.gamemap.ChangeLocCollision(loc.Shape(), loc.Angle(), oldLt.BlockRange,
				loc.Length, loc.Width, oldLt.Active, loc.X, loc.Z, loc.Level, false)
```

Edit `modules/world/world_zone.go:56-57` (`ChangeLoc` add new):

```go
			s.gamemap.ChangeLocCollision(loc.Shape(), loc.Angle(), newLt.BlockRange,
				loc.Length, loc.Width, newLt.Active, loc.X, loc.Z, loc.Level, true)
```

Edit `modules/world/world_zone.go:82-83` (`RemoveLoc`):

```go
			s.gamemap.ChangeLocCollision(loc.Shape(), loc.Angle(), lt.BlockRange,
				loc.Length, loc.Width, lt.Active, loc.X, loc.Z, loc.Level, false)
```

Edit `modules/world/loc_turn.go:42-43` (`RevertLoc` remove old):

```go
			s.gamemap.ChangeLocCollision(l.Shape(), l.Angle(), oldLt.BlockRange,
				l.Length, l.Width, oldLt.Active, l.X, l.Z, l.Level, false)
```

Edit `modules/world/loc_turn.go:49-50` (`RevertLoc` add new):

```go
			s.gamemap.ChangeLocCollision(l.Shape(), l.Angle(), newLt.BlockRange,
				l.Length, l.Width, newLt.Active, l.X, l.Z, l.Level, true)
```

Edit `modules/world/server.go:328-329` (`populateStaticLocsIntoZones`):

```go
				s.gamemap.ChangeLocCollision(loc.Shape(), loc.Angle(), lt.BlockRange,
					loc.Length, loc.Width, lt.Active, loc.X, loc.Z, loc.Level, true)
```

- [ ] **Step 5: Run test to verify PASSES**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestNAI96_GroundDecor_Active1 -v`
Expected: PASS.

- [ ] **Step 6: Run the full module test suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...`
Expected: PASS. Existing `TestNAI95_StaticLocCollision_HansArea` remains green (its `WallTileBlocked` subtest probes `FlagLoc` which is unaffected by GroundDecor changes).

- [ ] **Step 7: Commit**

```bash
git add pkg/gamemap/gamemap.go modules/world/world_zone.go modules/world/loc_turn.go modules/world/server.go modules/world/static_loc_collision_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(gamemap): NAI-96 — ChangeLocCollision active param + GroundDecor branch

Adds 'active int' to ChangeLocCollision signature (TS arg-order parity per
GameMap.ts:326). New LayerGroundDecor branch writes ChangeFloor when
active==1 (TS GameMap.ts:336-340). LayerWallDecor remains a no-op
(TS-faithful: TS skips it).

Threads lt.Active through 7 caller sites: world_zone.go AddLoc/ChangeLoc/
RemoveLoc, loc_turn.go RevertLoc, server.go populateStaticLocsIntoZones.

Surfaced from NAI-95 user-launched smoke (Lumbridge fountain walk-through:
GroundDecor active=1 BlockWalk=true loc was not writing collision).
EOF
)"
```

### Task 2.2: TDD — GroundDecor active=0 negative pin

- [ ] **Step 1: Add negative test**

Append to `modules/world/static_loc_collision_test.go`:

```go
// TestNAI96_GroundDecor_Active0_NoWrite pins that GroundDecor with active=0
// does not write collision (TS GameMap.ts:337 — only active===1 calls changeFloor).
func TestNAI96_GroundDecor_Active0_NoWrite(t *testing.T) {
	s := newTestServer(t)
	s.gamemap = gamemap.New(discardLogger())

	lt := &objtype.LocType{BlockWalk: true, Active: 0}
	s.locTypes = &objtype.LocTypes{Configs: []*objtype.LocType{nil, lt}}

	const absX, absZ, level = 3220, 3220, 0
	staticLoc := entity.NewLoc(level, absX, absZ, 1, 1,
		entity.LifecycleRespawn,
		1,
		int(loc.ShapeGroundDecor),
		int(loc.AngleWest))
	s.gamemap.AddStaticLoc(staticLoc)
	s.gamemap.Pathfinder.Flags.AllocateIfAbsent(absX, absZ, level)

	s.populateStaticLocsIntoZones()

	flag := s.gamemap.Pathfinder.Flags.Get(absX, absZ, level)
	if flag&collision.FlagBlockWalk != 0 {
		t.Errorf("GroundDecor active=0 at (%d, %d, %d): flag=0x%x unexpectedly has FlagBlockWalk",
			absX, absZ, level, flag)
	}
}

// TestNAI96_WallDecor_NoWrite pins that WallDecor never writes collision
// regardless of active (TS GameMap.ts:326-340 has no WALL_DECOR branch).
func TestNAI96_WallDecor_NoWrite(t *testing.T) {
	s := newTestServer(t)
	s.gamemap = gamemap.New(discardLogger())

	lt := &objtype.LocType{BlockWalk: true, Active: 1}
	s.locTypes = &objtype.LocTypes{Configs: []*objtype.LocType{nil, lt}}

	const absX, absZ, level = 3220, 3220, 0
	// ShapeWallDecorStraightNoOffset = 4 (LayerWallDecor per LayerOf).
	staticLoc := entity.NewLoc(level, absX, absZ, 1, 1,
		entity.LifecycleRespawn,
		1,
		int(loc.ShapeWallDecorStraightNoOffset),
		int(loc.AngleWest))
	s.gamemap.AddStaticLoc(staticLoc)
	s.gamemap.Pathfinder.Flags.AllocateIfAbsent(absX, absZ, level)

	s.populateStaticLocsIntoZones()

	flag := s.gamemap.Pathfinder.Flags.Get(absX, absZ, level)
	if flag != collision.FlagOpen {
		t.Errorf("WallDecor active=1 at (%d, %d, %d): flag=0x%x, want FlagOpen (0x0)",
			absX, absZ, level, flag)
	}
}
```

- [ ] **Step 2: Run tests — both should PASS immediately**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestNAI96_GroundDecor_Active0|TestNAI96_WallDecor" -v`
Expected: PASS for both. The Task 2.1 implementation already correctly skips active=0 and WallDecor; these are regression guards.

- [ ] **Step 3: Commit**

```bash
git add modules/world/static_loc_collision_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
test(gamemap): NAI-96 — pin GroundDecor active=0 and WallDecor as no-write

Two regression guards for TS GameMap.ts:326-340 layer dispatch:
  - GroundDecor active=0 → no flag write (only active==1 writes)
  - WallDecor regardless of active → no flag write (TS skips)
EOF
)"
```

### Task 2.3: TDD — angle-aware (length, width) swap

- [ ] **Step 1: Add the failing test pair**

Append to `modules/world/static_loc_collision_test.go`:

```go
// TestNAI96_AngleSwap_North_2x3 pins TS GameMap.ts:331-332 — N/S angles call
// ChangeLoc with (length, width) order, producing a length-along-X,
// width-along-Z footprint.
//
// Goscape Pathfinder.ChangeLoc(x, z, level, w, l, ...) iterates w*l tiles at
// offsets (index%w, index/w). With width=length=swapped to (length=3,
// width=2), the footprint covers X∈[x..x+2], Z∈[z..z+1] — 3 tiles wide, 2 tiles deep.
func TestNAI96_AngleSwap_North_2x3(t *testing.T) {
	s := newTestServer(t)
	s.gamemap = gamemap.New(discardLogger())

	// LocType: BlockWalk=true, Active=0 (LayerGround doesn't gate on active).
	lt := &objtype.LocType{BlockWalk: true, Active: 0}
	s.locTypes = &objtype.LocTypes{Configs: []*objtype.LocType{nil, lt}}

	// Loc: width=2, length=3, angle=North, LayerGround shape (Centrepiece).
	const absX, absZ, level = 3220, 3220, 0
	dynamicLoc := entity.NewLoc(level, absX, absZ, 2 /*width*/, 3 /*length*/,
		entity.LifecycleRespawn,
		1,
		int(loc.ShapeCentrepieceStraight),
		int(loc.AngleNorth))

	// Pre-allocate all tiles in the maximum possible footprint.
	for dx := 0; dx < 3; dx++ {
		for dz := 0; dz < 3; dz++ {
			s.gamemap.Pathfinder.Flags.AllocateIfAbsent(absX+dx, absZ+dz, level)
		}
	}

	// Use AddLoc to exercise the runtime path through ChangeLocCollision.
	s.AddLoc(dynamicLoc, -1)

	// Expected N/S footprint: 3 wide along X, 2 along Z.
	expected := map[[2]int]bool{
		{0, 0}: true, {1, 0}: true, {2, 0}: true,
		{0, 1}: true, {1, 1}: true, {2, 1}: true,
	}
	for dx := 0; dx < 3; dx++ {
		for dz := 0; dz < 3; dz++ {
			flag := s.gamemap.Pathfinder.Flags.Get(absX+dx, absZ+dz, level)
			has := flag&collision.FlagLoc != 0
			want := expected[[2]int{dx, dz}]
			if has != want {
				t.Errorf("N-angled 2x3 loc at (%d, %d, %d) offset (%d, %d): FlagLoc=%v, want %v (flag=0x%x)",
					absX, absZ, level, dx, dz, has, want, flag)
			}
		}
	}
}

// TestNAI96_AngleSwap_East_2x3 pins TS GameMap.ts:333-334 — non-N/S angles
// call ChangeLoc with (width, length) order, producing a width-along-X,
// length-along-Z footprint (2 wide along X, 3 along Z).
func TestNAI96_AngleSwap_East_2x3(t *testing.T) {
	s := newTestServer(t)
	s.gamemap = gamemap.New(discardLogger())

	lt := &objtype.LocType{BlockWalk: true, Active: 0}
	s.locTypes = &objtype.LocTypes{Configs: []*objtype.LocType{nil, lt}}

	const absX, absZ, level = 3220, 3220, 0
	dynamicLoc := entity.NewLoc(level, absX, absZ, 2 /*width*/, 3 /*length*/,
		entity.LifecycleRespawn,
		1,
		int(loc.ShapeCentrepieceStraight),
		int(loc.AngleEast))

	for dx := 0; dx < 3; dx++ {
		for dz := 0; dz < 3; dz++ {
			s.gamemap.Pathfinder.Flags.AllocateIfAbsent(absX+dx, absZ+dz, level)
		}
	}

	s.AddLoc(dynamicLoc, -1)

	// Expected E/W footprint: 2 wide along X, 3 along Z.
	expected := map[[2]int]bool{
		{0, 0}: true, {1, 0}: true,
		{0, 1}: true, {1, 1}: true,
		{0, 2}: true, {1, 2}: true,
	}
	for dx := 0; dx < 3; dx++ {
		for dz := 0; dz < 3; dz++ {
			flag := s.gamemap.Pathfinder.Flags.Get(absX+dx, absZ+dz, level)
			has := flag&collision.FlagLoc != 0
			want := expected[[2]int{dx, dz}]
			if has != want {
				t.Errorf("E-angled 2x3 loc at (%d, %d, %d) offset (%d, %d): FlagLoc=%v, want %v (flag=0x%x)",
					absX, absZ, level, dx, dz, has, want, flag)
			}
		}
	}
}
```

- [ ] **Step 2: Run tests to verify state**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestNAI96_AngleSwap -v`
Expected at this point: BOTH PASS (because Task 2.1 already added the angle-swap logic). These act as regression guards.

If `TestNAI96_AngleSwap_North_2x3` fails with the footprint inverted (3-along-Z instead of 3-along-X), Task 2.1's edit was incomplete; re-verify the `if angle == loc.AngleNorth || angle == loc.AngleSouth` branch.

- [ ] **Step 3: Commit**

```bash
git add modules/world/static_loc_collision_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
test(gamemap): NAI-96 — pin LayerGround angle-aware width/length swap

Two regression guards for TS GameMap.ts:331-334:
  - N-angled 2x3 loc → footprint 3-along-X, 2-along-Z (length, width swap)
  - E-angled 2x3 loc → footprint 2-along-X, 3-along-Z (width, length)

Currently masked for static locs (1x1 hardcode in load.go) but live for
script-driven AddLoc with multi-tile non-square locs.
EOF
)"
```

### Task 2.4: Verify Task 2 close

- [ ] **Step 1: Run all NAI-96 tests in modules/world**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestNAI96_|TestNAI95_" -v`
Expected: all 5 new `TestNAI96_*` tests PASS, NAI-95 `TestNAI95_StaticLocCollision_HansArea` PASSes.

- [ ] **Step 2: Run full repo test suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`
Expected: PASS.

- [ ] **Step 3: Run race detector on touched packages**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./pkg/gamemap/... ./modules/world/...`
Expected: PASS.

- [ ] **Step 4: Build the binary**

Run: `CGO_ENABLED=0 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build -trimpath ./cmd/goscape`
Expected: PASS.

---

## Smoke handoff (close commit, after both tasks merge)

- [ ] **Controller close commit:**

```bash
git commit --no-gpg-sign --allow-empty -m "$(cat <<'EOF'
chore(close): NAI-96 — pkg/gamemap/ collision-write divergences

Bundled close for NAI-95 smoke residuals in pkg/gamemap/:
  - BLOCK_MAP_SQUARE constant flip 0x2 → 0x1 (TS GameMap.ts:24)
  - LINK_BELOW bridged-level handling in loadGround + loadLocs
    (TS GameMap.ts:208, 242)
  - REMOVE_ROOFS roof-collision write (TS GameMap.ts:200-202)
  - GroundDecor active=1 → ChangeFloor (TS GameMap.ts:336-340)
  - LayerGround angle-aware (length,width) swap (TS GameMap.ts:331-334)

Awaits user-launched smoke per smoke_test_server_handoff.

Closes memory: smoke_surfaces_adjacent_divergences (NAI-95→NAI-96 routing)
EOF
)"
```

- [ ] **Smoke probe (user-launched):** Hans straight-line path test (3219, 3230 → 3219, 3222) — expect ≤2 waypoints (no detour through (3219, 3223)). Lumbridge fountain walk-into — expect blocked.

---

## Self-Review

**Spec coverage:**
- §2 in-scope Task 1: BLOCK_MAP_SQUARE flip ✓ Task 1.2; LINK_BELOW two-pass ✓ Task 1.3-1.4; REMOVE_ROOFS ✓ Task 1.3; loadLocs lands consumption ✓ Task 1.5.
- §2 in-scope Task 2: active param ✓ Task 2.1; LayerGroundDecor branch ✓ Task 2.1; angle-swap ✓ Task 2.1 (impl), Task 2.3 (test); 7 caller updates ✓ Task 2.1.
- §3.2 tests: 7 listed scenarios, all covered (Tasks 1.2, 1.3, 1.4 [3 subtests], 1.5).
- §4.3 tests: 5 listed scenarios, all covered (Tasks 2.1, 2.2 [2 subtests], 2.3 [2 subtests]).
- §6 risks: `controller_preflight` ✓ pre-dispatch checklist; `enumerate_all_sites` ✓ 7 sites listed; `plan_runnable_test_fixtures` ✓ helper functions inlined.

**Placeholder scan:** No "TBD" / "TODO in plan" / "implement later". The `TODO(loctype)` line in `loadLocs` is preserved code (existing TODO in production tracked in spec §8 out-of-scope).

**Type consistency:**
- `gameMapBlockMapSquare`, `gameMapLinkBelow`, `gameMapRemoveRoofs` — all referenced consistently.
- `landsByMapSquare map[uint16][]int8` — same name in struct, `New`, `loadGround` write, `loadLocs` read.
- `packCoord(x, z, level)` — defined once in load.go, used both in loadGround and loadLocs.
- `ChangeLocCollision` new signature `(shape, angle int, blocksRange bool, length, width, active, x, z, level int, add bool)` — callers all pass identifier-named values matching this order.
- Test name prefixes consistent: `TestLoadGround_*` / `TestLoadLocs_*` (Task 1) and `TestNAI96_*` (Task 2).
