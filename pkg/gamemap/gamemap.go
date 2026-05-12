package gamemap

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/zsrv/goscape/pkg/entity"
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/pathfinder/collision"
	"github.com/zsrv/goscape/pkg/pathfinder/loc"
	"github.com/zsrv/goscape/pkg/pathfinder/routefinder"
)

// NpcSpawn records an NPC spawn position from a mapsquare's n-file.
type NpcSpawn struct {
	TypeID      int
	X, Z, Level int
}

// ObjSpawn records a static ground-obj spawn position from a mapsquare's
// o-file. Mirrors NpcSpawn (above). NAI-151.
type ObjSpawn struct {
	TypeID, Count int
	X, Z, Level   int
}

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
	objSpawns        []ObjSpawn
	members          bool                    // NodeMembers flag — set via SetMembers before Init. Mirrors TS GameMap.members. NAI-151.
	objTypes         *objtype.ObjTypeConfigs // optional; consumed by loadObjs gate. nil-OK preserves t.TempDir() test fixtures with empty caches. NAI-151.
	locTypes         *objtype.LocTypeConfigs // optional; when set before Init, loadLocs uses lt.Width/lt.Length per LocType (NAI-100). nil-OK preserves t.TempDir() test fixtures.
	log              *slog.Logger
}

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

// SetLocTypes registers the LocType configs used by loadLocs to thread
// per-instance Width/Length into static *entity.Loc construction. Must be
// called BEFORE Init for static-loc footprint correctness; calling later
// has no effect on already-loaded static locs. nil-OK: when unset,
// loadLocs falls back to 1×1 (preserves test fixtures with empty caches).
// Mirrors TS GameMap.ts:248-263 where loadLocations consults LocType.get().
func (gm *GameMap) SetLocTypes(cfgs *objtype.LocTypeConfigs) {
	gm.locTypes = cfgs
}

// SetMembers registers the world's NodeMembers flag for use by loadObjs.
// Must be called BEFORE Init; calling later has no effect on already-
// loaded static objs. Default false. Mirrors TS GameMap.members.
// NAI-151.
func (gm *GameMap) SetMembers(m bool) {
	gm.members = m
}

// SetObjTypes registers the ObjType configs used by loadObjs to gate
// per-obj members visibility. Must be called BEFORE Init. nil-OK:
// when unset, loadObjs records no objs (preserves test fixtures with
// empty caches). Mirrors TS ObjType.get() inside GameMap.loadObjs.
// NAI-151.
func (gm *GameMap) SetObjTypes(cfgs *objtype.ObjTypeConfigs) {
	gm.objTypes = cfgs
}

// ChangeLandCollision marks or clears a floor tile as walkable.
func (gm *GameMap) ChangeLandCollision(x, z, level int, add bool) {
	gm.Pathfinder.ChangeFloor(x, z, level, add)
}

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
// (offsetX, offsetZ) to an adjacent tile is allowed under the given
// per-entity collision strategy. offsetX/offsetZ must be in {-1, 0, 1}.
// size is the entity's tile footprint width (1 for players and 1-tile NPCs).
// extraFlag is the entity's blockWalkFlag() (e.g. FlagBlockPlayers for
// players, FlagBlockNPCs for normal NPCs, FlagOpen for blocked NPCs).
// collisionType is the entity's getCollisionStrategy() (TypeNormal,
// TypeBlocked, TypeIndoors, TypeOutdoors).
func (gm *GameMap) CanTravel(level, x, z, offsetX, offsetZ, size, extraFlag int, collisionType collision.Type) bool {
	return gm.Pathfinder.StepValidator.CanTravel(
		level, x, z, offsetX, offsetZ, size, extraFlag, collisionType,
	)
}

// Init reads map-pack files from cacheDir/server/maps/ and populates the collision map.
// Missing files and missing CSVs are treated as warnings, not errors.
func (gm *GameMap) Init(cacheDir string) error {
	mapsDir := filepath.Join(cacheDir, "server", "maps")

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
		key := uint16((sqX << 8) | sqZ)
		gm.mData[key] = mData
		gm.loadGround(mData, sqX, sqZ)

		lPath := filepath.Join(mapsDir, fmt.Sprintf("l%d_%d", sqX, sqZ))
		if lData, err := os.ReadFile(lPath); err == nil {
			gm.lData[key] = lData
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

// NpcSpawns returns the list of NPC spawn records collected during Init.
func (gm *GameMap) NpcSpawns() []NpcSpawn { return gm.npcSpawns }

// ObjSpawns returns the list of static-obj spawn records collected
// during Init. Returned slice is internal — do not mutate. NAI-151.
func (gm *GameMap) ObjSpawns() []ObjSpawn { return gm.objSpawns }

// StaticLocs returns the parsed static locs accumulated during Init.
// Pointers are stable for the lifetime of GameMap.
func (gm *GameMap) StaticLocs() []*entity.Loc { return gm.staticLocs }

// AddStaticLoc appends a pre-built loc to the static-loc list. Used by
// the Server at startup (indirectly via StaticLocs) and by tests that
// need to seed a synthetic map.
func (gm *GameMap) AddStaticLoc(loc *entity.Loc) {
	gm.staticLocs = append(gm.staticLocs, loc)
}

// LandBytes returns the raw (on-disk) bytes of m{X}_{Z}, or nil if unloaded.
func (gm *GameMap) LandBytes(mapX, mapZ int) []byte {
	return gm.mData[uint16((mapX<<8)|mapZ)]
}

// LocBytes returns the raw (on-disk) bytes of l{X}_{Z}, or nil if unloaded.
func (gm *GameMap) LocBytes(mapX, mapZ int) []byte {
	return gm.lData[uint16((mapX<<8)|mapZ)]
}

// SetLandBytesForTest seeds raw m{mapX}_{mapZ} bytes for tests that want
// to exercise serving without real cache files.
func (gm *GameMap) SetLandBytesForTest(mapX, mapZ int, b []byte) {
	gm.mData[uint16((mapX<<8)|mapZ)] = b
}

// SetLocBytesForTest seeds raw l{mapX}_{mapZ} bytes for tests.
func (gm *GameMap) SetLocBytesForTest(mapX, mapZ int, b []byte) {
	gm.lData[uint16((mapX<<8)|mapZ)] = b
}

// LoadObjsForTest exposes the unexported loadObjs parser for tests in
// downstream packages (modules/world). NAI-151.
func (gm *GameMap) LoadObjsForTest(data []byte, mapSquareX, mapSquareZ int) {
	gm.loadObjs(data, mapSquareX, mapSquareZ)
}

// SetFreeMapForTest flags the zone containing (x, z) as F2P. Mirrors
// the encoding used by gm.IsFreeToPlay → packZoneCoord(x, z, 0). NAI-151.
func (gm *GameMap) SetFreeMapForTest(x, z int) {
	gm.freemap[packZoneCoord(x, z, 0)] = true
}
