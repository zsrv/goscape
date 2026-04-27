package script

import (
	"github.com/zsrv/goscape/pkg/coordgrid"
)

// NpcIteratorMode selects between DISTANCE (square radius around center
// coord, walks zones outer-X-desc/inner-Z-desc) and ZONE (single zone
// at center coord). Mirrors TS NpcIteratorType enum.
type NpcIteratorMode int

const (
	NpcIteratorDistance NpcIteratorMode = iota
	NpcIteratorZone
)

// NpcIterator is the script-VM iterator state for the NPC_FIND iterator
// family (NPC_FINDALL / NPC_FINDALLANY / NPC_FINDALLZONE). Mirrors TS
// NpcIterator at ScriptIterators.ts:297-363.
//
// Lifetime: single-tick. Created by FINDALL*; consumed by FINDNEXT.
// Stale() check at FINDNEXT compares creationTick to World.CurrentTick();
// on mismatch, handler returns error → existing npc_script.go:167-172
// log-warn + ClearActiveScript path runs (mirrors TS throw-on-stale at
// ScriptIterators.ts:332,343).
//
// Ownership: held by ScriptState.npcIterator. Nil = no active iterator.
// No termination-path cleanup needed: Aborted/Finished drops state;
// NpcSuspended carries iterator, but Stale() on resume catches stale use.
type NpcIterator struct {
	mode         NpcIteratorMode
	creationTick int
	lookup       NpcLookup

	// Center + filter config
	level    int
	x, z     int
	distance int // DISTANCE mode only; 0 for ZONE
	huntvis  int // validated at handler; not used as filter (NAI-33-D1)
	typeID   int // -1 = no filter (FINDALLANY, FINDALLZONE); else exact match

	// Zone-cursor (DISTANCE mode)
	minZoneX, maxZoneX int
	minZoneZ, maxZoneZ int
	curZoneX, curZoneZ int
	started            bool

	// Intra-zone snapshot (lazy: filled on zone-entry)
	zoneNpcs []ActiveNpc
	zoneIdx  int
}

// Stale reports whether currentTick differs from the iterator's
// creationTick. FINDNEXT handler MUST check this before calling Next.
// Single-tick lifetime: any drift = stale.
func (it *NpcIterator) Stale(currentTick int) bool {
	return currentTick != it.creationTick
}

// passesFilter applies the per-NPC filter chain in TS line 345-356 order.
// huntvis filtering is intentionally omitted (NAI-33-D1 / S7f-D1 carryover —
// see deviation registry). Accessor names match pkg/script/active.go:400-408
// ActiveNpc interface (NpcX/NpcZ/NpcType, NOT X/Z/Type).
func (it *NpcIterator) passesFilter(npc ActiveNpc) bool {
	if it.mode == NpcIteratorZone {
		return true // ZONE mode: no per-NPC filtering per TS line 329-335
	}
	if coordgrid.DistanceToSW(it.x, it.z, npc.NpcX(), npc.NpcZ()) > it.distance {
		return false
	}
	// huntvis filter intentionally omitted — NAI-33-D1 carryover
	if it.typeID >= 0 && npc.NpcType() != it.typeID {
		return false
	}
	return true
}

// NewDistanceNpcIterator constructs an iterator that walks NPCs in zones
// within `distance` of (level, x, z), filtered by huntvis (validated only —
// NAI-33-D1) and typeID (-1 = no filter). Mirrors TS NpcIterator
// constructor at ScriptIterators.ts:310-326 with type=DISTANCE.
//
// Bounds math (per TS line 312-321):
//
//	centerX = x >> 3
//	radius  = 1 + distance/8       // integer division
//	zone bounds = [center - radius, center + radius]
//
// Cursor starts at (maxZoneX, maxZoneZ) per TS line 337-340; advances
// outer X descending, inner Z descending in advanceZone (Task 6).
func NewDistanceNpcIterator(lookup NpcLookup, tick, level, x, z, distance, huntvis, typeID int) *NpcIterator {
	centerX := x >> 3
	centerZ := z >> 3
	radius := 1 + distance/8
	return &NpcIterator{
		mode:         NpcIteratorDistance,
		creationTick: tick,
		lookup:       lookup,
		level:        level,
		x:            x,
		z:            z,
		distance:     distance,
		huntvis:      huntvis,
		typeID:       typeID,
		minZoneX:     centerX - radius,
		maxZoneX:     centerX + radius,
		minZoneZ:     centerZ - radius,
		maxZoneZ:     centerZ + radius,
		curZoneX:     centerX + radius,
		curZoneZ:     centerZ + radius,
	}
}

// NewZoneNpcIterator constructs an iterator that yields all NPCs in the
// single zone containing (level, x, z) — no distance/type filtering.
// Mirrors TS NpcIterator constructor at ScriptIterators.ts:310-326 with
// type=ZONE (no npcType arg). Cursor (curZoneX/Z) is set on first Next
// call by advanceZone (Task 6).
func NewZoneNpcIterator(lookup NpcLookup, tick, level, x, z int) *NpcIterator {
	return &NpcIterator{
		mode:         NpcIteratorZone,
		creationTick: tick,
		lookup:       lookup,
		level:        level,
		x:            x,
		z:            z,
		typeID:       -1, // not used in ZONE mode
	}
}

// Next advances the iterator and returns the next matching NPC. Returns
// (nil, false) on exhaustion. Caller must check Stale(currentTick) before
// invoking Next when the single-tick lifetime invariant matters; FINDNEXT
// handler does this. Mirrors TS NpcIterator.generator at
// ScriptIterators.ts:328-362 (the for-of consumption shape).
func (it *NpcIterator) Next() (ActiveNpc, bool) {
	if it.lookup == nil {
		return nil, false
	}
	for {
		// Drain current intra-zone snapshot
		for it.zoneIdx < len(it.zoneNpcs) {
			npc := it.zoneNpcs[it.zoneIdx]
			it.zoneIdx++
			if it.passesFilter(npc) {
				return npc, true
			}
		}
		// Snapshot exhausted; advance zone cursor (or terminate)
		if !it.advanceZone() {
			return nil, false
		}
		it.zoneNpcs = it.lookup.ZoneNpcs(it.level, it.curZoneX*8, it.curZoneZ*8)
		it.zoneIdx = 0
	}
}

// advanceZone moves the (curZoneX, curZoneZ) cursor and returns true if
// a new zone is now selected, false if iteration has exhausted the
// bounding region. Walks outer-X-desc / inner-Z-desc per TS line 337-340.
//
// ZONE mode: returns true exactly once (the single-zone visit), false
// thereafter. ZONE-mode cursor is initialized HERE on first call (the
// constructor leaves curZoneX/Z at zero so the lazy initialization
// sits with the rest of the cursor logic).
func (it *NpcIterator) advanceZone() bool {
	if it.mode == NpcIteratorZone {
		if it.started {
			return false
		}
		it.started = true
		// Initialize cursor for the single zone at (level, x, z)
		it.curZoneX = it.x >> 3
		it.curZoneZ = it.z >> 3
		return true
	}
	// DISTANCE mode
	if !it.started {
		it.started = true
		// Cursor already at (maxX, maxZ) from constructor
		return true
	}
	// Inner Z descends; on underflow, reset to maxZ and outer X descends;
	// on outer-X underflow, exhausted.
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
