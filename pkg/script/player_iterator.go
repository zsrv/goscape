package script

import (
	"github.com/zsrv/goscape/pkg/coordgrid"
	"github.com/zsrv/goscape/pkg/objtype"
)

// PlayerIteratorMode mirrors NpcIteratorMode but for the player iterator
// family. Currently only HuntAll mode has a script-VM consumer
// (HUNTNEXT, NAI-35-T5). Distance and Zone modes are deferred per
// NAI-35-D2 to avoid speculative dead-API.
type PlayerIteratorMode int

const (
	PlayerIteratorHuntAll PlayerIteratorMode = iota
)

// PlayerIterator is the script-VM iterator state for the player
// iterator family (HUNTALL only at NAI-35). Lifetime: single-tick.
// Created by HUNTALL; consumed by HUNTNEXT.
//
// Mirrors NpcIterator template (pkg/script/npc_iterator.go) closely:
// same lazy zone-walking shape, same Stale check, same exhaustion
// semantics. PlayerLookup.ZonePlayers provides the per-zone snapshot.
type PlayerIterator struct {
	mode          PlayerIteratorMode
	creationTick  int
	lookup        PlayerLookup
	lineValidator LineValidator

	level    int
	x, z     int
	distance int
	huntvis  int

	minZoneX, maxZoneX int
	minZoneZ, maxZoneZ int
	curZoneX, curZoneZ int
	started            bool

	zonePlayers []ActivePlayer
	zoneIdx     int
}

// Stale reports whether the iterator was created in a prior tick.
// HUNTNEXT MUST check this before calling Next. Strict greater-than
// per `iterator_state_pattern.md` element 3 (TS-faithful).
func (it *PlayerIterator) Stale(currentTick int) bool {
	return currentTick > it.creationTick
}

// passesFilter applies the per-player filter chain. HuntAll mode only:
// distance + huntvis (LoS/LoW). Distance and Zone modes are not yet
// implemented (NAI-35-D2). NAI-35-T3 NpcIterator analogue: per-mode
// branching is unnecessary here because HuntAll is the only mode.
func (it *PlayerIterator) passesFilter(p ActivePlayer) bool {
	if coordgrid.DistanceToSW(it.x, it.z, p.X(), p.Z()) > it.distance {
		return false
	}
	switch it.huntvis {
	case objtype.HuntVisOff:
		return true
	case objtype.HuntVisLineOfSight:
		if it.lineValidator == nil {
			return true
		}
		// TS-faithful: PlayerHuntAllCommandIterator passes player-as-src,
		// iterator-as-dest (ScriptIterators.ts:216) — REVERSE of NPC variant
		// at line 284. The TS asymmetry is intentional; RayCast direction
		// sensitivity (linevalidator.go:42-160) makes the swap observable.
		return it.lineValidator.HasLineOfSight(it.level, p.X(), p.Z(), it.x, it.z, 1, 0, 0, 0)
	case objtype.HuntVisLineOfWalk:
		if it.lineValidator == nil {
			return true
		}
		// TS-faithful: see HuntVisLineOfSight comment.
		return it.lineValidator.HasLineOfWalk(it.level, p.X(), p.Z(), it.x, it.z, 1, 0, 0, 0)
	}
	return true
}

// NewHuntAllPlayerIterator constructs a HuntAll-mode iterator. Mirrors
// NewHuntAllNpcIterator bounds-math (centerX = x>>3, radius = 1+distance/8,
// cursor at maxZone). NAI-35-T4.
func NewHuntAllPlayerIterator(lookup PlayerLookup, lv LineValidator, tick, level, x, z, distance, huntvis int) *PlayerIterator {
	centerX := x >> 3
	centerZ := z >> 3
	radius := 1 + distance/8
	return &PlayerIterator{
		mode:          PlayerIteratorHuntAll,
		creationTick:  tick,
		lookup:        lookup,
		lineValidator: lv,
		level:         level,
		x:             x,
		z:             z,
		distance:      distance,
		huntvis:       huntvis,
		minZoneX:      centerX - radius,
		maxZoneX:      centerX + radius,
		minZoneZ:      centerZ - radius,
		maxZoneZ:      centerZ + radius,
		curZoneX:      centerX + radius,
		curZoneZ:      centerZ + radius,
	}
}

// Next advances and returns the next matching player. Returns
// (nil, false) on exhaustion. Caller MUST check Stale first when the
// single-tick lifetime invariant matters; HUNTNEXT does this.
func (it *PlayerIterator) Next() (ActivePlayer, bool) {
	if it.lookup == nil {
		return nil, false
	}
	for {
		for it.zoneIdx < len(it.zonePlayers) {
			p := it.zonePlayers[it.zoneIdx]
			it.zoneIdx++
			if it.passesFilter(p) {
				return p, true
			}
		}
		if !it.advanceZone() {
			return nil, false
		}
		it.zonePlayers = it.lookup.ZonePlayers(it.level, it.curZoneX*8, it.curZoneZ*8)
		it.zoneIdx = 0
	}
}

// advanceZone walks outer-X-desc / inner-Z-desc per the NpcIterator
// reference impl. Returns false on exhaustion.
func (it *PlayerIterator) advanceZone() bool {
	if !it.started {
		it.started = true
		return true
	}
	it.curZoneZ--
	if it.curZoneZ < it.minZoneZ {
		it.curZoneZ = it.maxZoneZ
		it.curZoneX--
		if it.curZoneX < it.minZoneX {
			return false
		}
	}
	return true
}
