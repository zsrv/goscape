package world

import (
	"slices"

	"github.com/zsrv/goscape/pkg/coordgrid"
)

// buildArea mirrors TS BuildArea (Engine-TS BuildArea.ts:7-94) — the
// per-player rebuild state. Holds the 13×13-zone window the client has
// loaded, the 7×7 active-zone subset currently being delivered, the
// mapsquare set, and the tick of the most recent REBUILD_NORMAL emit.
//
// Owned by *Player; constructed in newPlayer via newBuildArea(p). The
// player backref mirrors TS's `readonly player: Player` field
// (BuildArea.ts:9) and lets methods read p.x, p.z, p.originX,
// p.originZ, p.level, p.reconnecting, p.rebuiltOnce, and
// p.client.server.currentTick without threading those args through
// every call.
//
// PORTING-EXCEPTION (NAI-93 + rebuiltOnce): the rebuiltOnce gate lives
// on *Player (not on buildArea) because it tracks a goscape-only
// divergence forced by NAI-93 tick-order: TS's BuildArea.rebuildNormal
// uses originX=-1 as the implicit "first build" sentinel (the bounds-
// check `if (player.x < reloadLeftX || ...)` naturally fails because
// reloadLeftX = (-1-4)<<3 = -40 when originX = -1). Goscape's tick
// order at tick.go:166-169 has processLogins set originX/Z to a real
// coord BEFORE rebuildNormal runs in processInfo (per NAI-93, moved
// from processOut so PlayerInfo's zone-relative encoding reads a fresh
// origin). The TS sentinel-via-origin pattern cannot work in goscape;
// a separate rebuiltOnce bool gates the first build. The buildArea
// struct otherwise matches TS one-to-one (4 fields + backref + 3
// methods). See PORTING-CLOSED.md NAI-30 closure row.
type buildArea struct {
	player      *Player
	loadedZones map[int]bool
	activeZones map[int]bool
	mapsquares  map[uint16]bool
	lastBuild   int
}

// newBuildArea constructs a fresh buildArea bound to the given player.
// Mirrors TS `new BuildArea(this)` at Player.ts:320; initialises the
// three Sets and leaves lastBuild at zero (the goscape pre-flatten
// initial value — the first-build gate is rebuiltOnce, not lastBuild).
func newBuildArea(p *Player) *buildArea {
	return &buildArea{
		player:      p,
		loadedZones: map[int]bool{},
		activeZones: map[int]bool{},
		mapsquares:  map[uint16]bool{},
		lastBuild:   0,
	}
}

// clear mirrors TS BuildArea.clear (BuildArea.ts:23-29). When
// !reconnecting, drops every tracked zone + mapsquare so the next tick
// rebuilds from scratch. No-op on a reconnect (the client retains its
// loaded state across resume).
func (b *buildArea) clear(reconnecting bool) {
	if !reconnecting {
		b.activeZones = map[int]bool{}
		b.loadedZones = map[int]bool{}
		b.mapsquares = map[uint16]bool{}
	}
}

// rebuildZones mirrors TS BuildArea.rebuildZones (BuildArea.ts:31-55).
// Refreshes activeZones to a 7×7-zone window centered on the player's
// current zone, intersected with the 13×13-zone build-area window
// centered on origin.
//
// Called from two sites:
//
//  1. handleRebuildGetMaps (data_map.go:153) — after client confirms
//     maps loaded post-REBUILD_NORMAL.
//  2. updateBuildArea (player.go, top of processOut) — per-tick zone
//     transition (NAI-142, mirroring TS NetworkPlayer.ts:269-271).
//
// Both sites fire on the same tick on a REBUILD path; rebuildZones
// resets activeZones at its top, so the duplication is idempotent.
// Matches TS ordering (TS World.ts:1097 → NetworkPlayer.updateMap also
// calls rebuildZones unconditionally on lastZone change).
func (b *buildArea) rebuildZones() {
	p := b.player
	b.activeZones = map[int]bool{}
	centerX := p.x >> 3
	centerZ := p.z >> 3
	originZoneX := p.originX >> 3
	originZoneZ := p.originZ >> 3
	leftX := originZoneX - 6
	rightX := originZoneX + 6
	bottomZ := originZoneZ - 6
	topZ := originZoneZ + 6
	for x := centerX - 3; x <= centerX+3; x++ {
		for z := centerZ - 3; z <= centerZ+3; z++ {
			if x < leftX || x > rightX || z < bottomZ || z > topZ {
				continue
			}
			if x < 0 || z < 0 { // (goscape defensive; TS skips this check)
				continue
			}
			b.activeZones[coordgrid.ZoneIndex(x<<3, z<<3, p.level)] = true
		}
	}
}

// shouldRebuild reports whether the player has crossed the 13×13 zone
// window centered on (originX, originZ), or whether reconnect is true,
// or whether the player has never had a build emitted yet. TS inlines
// this bounds-check inside BuildArea.rebuildNormal (BuildArea.ts:67);
// goscape extracts it so tests can pin the regression at
// TestShouldRebuild_FiresOnFirstBuildEvenWithOriginSet. The function-
// vs-inline split is code-org only; semantics match TS exactly modulo
// the rebuiltOnce gate (see buildArea doc-comment for the NAI-93
// rationale).
func (b *buildArea) shouldRebuild() bool {
	p := b.player
	if !p.rebuiltOnce {
		return true
	}
	if p.reconnecting {
		return true
	}
	originZoneX := p.originX >> 3
	originZoneZ := p.originZ >> 3
	reloadLeftX := (originZoneX - 4) << 3
	reloadRightX := (originZoneX + 5) << 3
	reloadTopZ := (originZoneZ + 5) << 3
	reloadBottomZ := (originZoneZ - 4) << 3
	if p.x < reloadLeftX || p.z < reloadBottomZ ||
		p.x > reloadRightX-1 || p.z > reloadTopZ-1 {
		return true
	}
	return false
}

// rebuildScenery is the state-compute body of TS rebuildNormal
// (BuildArea.ts:67-92) extracted as a helper so the test fixture at
// newZoneTestPlayer (player_zone_test.go) can drive the build state
// without invoking sendRebuildNormal (which requires a wired
// encryptor + server). Resets loadedZones + mapsquares, computes the
// 13×13 mapsquare window centered on the player's current position,
// commits the new origin, stamps lastBuild, sets rebuiltOnce, and
// returns the mapsquares as a sorted []uint16 ready for
// sendRebuildNormal.
//
// Goscape-only divergence: TS clears mapsquares and loadedZones here
// but does NOT touch activeZones in rebuildNormal (activeZones is
// cleared at the top of rebuildZones). Goscape historically cleared
// all three here. Aligned to TS in this refactor: activeZones is no
// longer reset by rebuildScenery (rebuildZones still resets it on the
// next call, idempotent).
func (b *buildArea) rebuildScenery(currentTick int) []uint16 {
	p := b.player
	b.loadedZones = map[int]bool{}
	b.mapsquares = map[uint16]bool{}

	zoneX := p.x >> 3
	zoneZ := p.z >> 3
	for dx := -6; dx <= 6; dx++ {
		for dz := -6; dz <= 6; dz++ {
			zx := zoneX + dx
			zz := zoneZ + dz
			if zx < 0 || zz < 0 {
				continue
			}
			mapX := zx >> 3
			mapZ := zz >> 3
			if mapX > 0xff || mapZ > 0xff {
				continue
			}
			b.mapsquares[uint16((mapX<<8)|mapZ)] = true
		}
	}

	p.originX = p.x
	p.originZ = p.z
	b.lastBuild = currentTick
	p.rebuiltOnce = true

	out := make([]uint16, 0, len(b.mapsquares))
	for m := range b.mapsquares {
		out = append(out, m)
	}
	slices.Sort(out)
	return out
}

// rebuildNormal mirrors TS BuildArea.rebuildNormal (BuildArea.ts:57-93).
// Per-tick gate via shouldRebuild; on trigger, recomputes the build
// area state via rebuildScenery and emits REBUILD_NORMAL.
//
// NAI-93 cross-reference: TS World.ts:996 dispatches this from the
// processInfo phase. Goscape's tick.go:858-867 invokes it inside
// Server.processInfo per player, BEFORE updatePlayers reads
// p.originX/Z for zone-relative encoding. Pre-NAI-93 the call lived in
// processOut, which left ComputePlayer reading a STALE origin and
// produced out-of-bounds localX values on cross-window tele,
// crashing the Java client's getHeightmapY and getTopLevel.
func (b *buildArea) rebuildNormal() {
	p := b.player
	if p.client == nil || p.client.server == nil {
		return
	}
	if !b.shouldRebuild() {
		return
	}
	ms := b.rebuildScenery(p.client.server.currentTick)
	p.reconnecting = false
	sendRebuildNormal(p, ms)
}
