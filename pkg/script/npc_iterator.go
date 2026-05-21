package script

import (
	"github.com/zsrv/goscape/pkg/coordgrid"
	"github.com/zsrv/goscape/pkg/objtype"
)

// NpcIteratorMode selects between DISTANCE (square radius around center
// coord, walks zones outer-X-desc/inner-Z-desc) and ZONE (single zone
// at center coord). Mirrors TS NpcIteratorType enum.
type NpcIteratorMode int

const (
	NpcIteratorDistance NpcIteratorMode = iota
	NpcIteratorZone
	NpcIteratorHuntAll // NAI-35-T3: distance-bounded with active huntvis filter
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
	// huntvis is the LoS/LoW gate level (HuntVisOff/LineOfSight/
	// LineOfWalk). Consumed by passesFilter in Distance + HuntAll modes
	// (TS ScriptIterators.ts:348-352 for DISTANCE, :284-287 for HuntAll
	// — identical arg shape). Zone mode unfiltered per TS line 329-335
	// (NpcIterator ZONE branch yields without huntvis checks; the
	// npc_findallzone command takes no huntvis arg either, per
	// engine.rs2:605).
	huntvis int
	// lineValidator is the LoS/LoW validator used by passesFilter in
	// Distance + HuntAll modes when huntvis ∈ {LineOfSight, LineOfWalk}.
	// Nil = no validator wired (test stub or pre-wiring) → pessimistic
	// allow; production sets via the constructor. NAI-35-T3 (HuntAll),
	// extended to Distance per TS ScriptIterators.ts:348-352.
	lineValidator LineValidator
	typeID        int // -1 = no filter (FINDALLANY, FINDALLZONE); else exact match

	// Zone-cursor (DISTANCE mode)
	minZoneX, maxZoneX int
	minZoneZ, maxZoneZ int
	curZoneX, curZoneZ int
	started            bool

	// Intra-zone snapshot (lazy: filled on zone-entry)
	zoneNpcs []ActiveNpc
	zoneIdx  int

	// configs is the cache-loaded NpcType/LocType/etc. provider used by
	// passesFilter's HuntAll-mode op[1] gate (TS ScriptIterators.ts:274-280).
	// Nil = test fixture without Configs wired; pessimistic-allow per
	// the lineValidator==nil convention. Production sets this from
	// s.Configs at NewHuntAllNpcIterator. NAI-180.
	configs Configs
}

// Stale reports whether the iterator was created in a prior tick.
// FINDNEXT handler MUST check this before calling Next. Mirrors TS
// strict-greater-than at ScriptIterators.ts:332,343 (World.currentTick
// > this.tick). Single-tick lifetime: any forward tick = stale; past
// ticks are impossible in practice (caller is the script VM, which
// can't go backwards) but per TS we don't flag them as stale.
func (it *NpcIterator) Stale(currentTick int) bool {
	return currentTick > it.creationTick
}

// passesFilter applies the per-NPC filter chain in TS line 345-356 order.
// Both Distance and HuntAll modes consume huntvis (TS
// ScriptIterators.ts:348-352 / :284-287); ZONE mode early-returns
// unfiltered (TS line 329-335). HuntAll-mode op[1] reject (NAI-180)
// runs before the distance check; otherwise the chain is shared:
// distance → huntvis → typeID. Accessor names match
// pkg/script/active.go:400-408 ActiveNpc interface (NpcX/NpcZ/NpcType,
// NOT X/Z/Type).
func (it *NpcIterator) passesFilter(npc ActiveNpc) bool {
	if it.mode == NpcIteratorZone {
		return true // ZONE mode: no per-NPC filtering per TS ScriptIterators.ts:329-335
	}
	// HuntAll-mode op[1] reject runs BEFORE distance check per TS order
	// at ScriptIterators.ts:274-282. NAI-180 closes NAI-35-T3-D1.
	if it.mode == NpcIteratorHuntAll && it.configs != nil {
		// (goscape defensive; TS throws on missing NpcType) — when the
		// configs lookup returns nil (unknown NPC type), pessimistically
		// allow to match the lineValidator==nil convention at
		// npcVisibleViaLineOfSight. Production NPCs always have a type.
		npcType := it.configs.NpcType(npc.NpcType())
		if npcType != nil && (len(npcType.Op) <= 1 || npcType.Op[1] == "") {
			return false
		}
	}
	if coordgrid.DistanceToSW(it.x, it.z, npc.NpcX(), npc.NpcZ()) > it.distance {
		return false
	}
	// Huntvis filter applies in Distance + HuntAll modes (TS
	// ScriptIterators.ts:348-352 for DISTANCE, :284-287 for HuntAll —
	// identical arg shape). Zone mode early-returns above per TS line
	// 329-335 (unfiltered yield).
	switch it.huntvis {
	case objtype.HuntVisOff:
		// no LoS/LoW gate
	case objtype.HuntVisLineOfSight:
		if !it.npcVisibleViaLineOfSight(npc) {
			return false
		}
	case objtype.HuntVisLineOfWalk:
		if !it.npcVisibleViaLineOfWalk(npc) {
			return false
		}
	}
	if it.typeID >= 0 && npc.NpcType() != it.typeID {
		return false
	}
	return true
}

// npcVisibleViaLineOfSight returns true when the iterator's lineValidator
// passes a LoS check from the iterator's center coord to the NPC. Nil
// validator = pessimistically allow. NAI-35-T3.
// Arg tuple (1, 1, 1, 0) mirrors TS isLineOfSight wrapper at
// GameMap.ts:429-431, invoked from ScriptIterators.ts:284.
// NAI-166-D-LOW-ARG-SHAPE-SWEEP closes the prior (1, 0, 0, 0) shape.
func (it *NpcIterator) npcVisibleViaLineOfSight(npc ActiveNpc) bool {
	if it.lineValidator == nil {
		return true
	}
	return it.lineValidator.HasLineOfSight(it.level, it.x, it.z, npc.NpcX(), npc.NpcZ(), 1, 1, 1, 0)
}

// npcVisibleViaLineOfWalk returns true when the iterator's lineValidator
// passes a LoW check. Nil validator = pessimistically allow. NAI-35-T3.
// Arg tuple (1, 1, 1, 0) mirrors TS isLineOfWalk wrapper at
// GameMap.ts:425-427, invoked from ScriptIterators.ts:287.
// NAI-166-D-LOW-ARG-SHAPE-SWEEP closes the prior (1, 0, 0, 0) shape.
func (it *NpcIterator) npcVisibleViaLineOfWalk(npc ActiveNpc) bool {
	if it.lineValidator == nil {
		return true
	}
	return it.lineValidator.HasLineOfWalk(it.level, it.x, it.z, npc.NpcX(), npc.NpcZ(), 1, 1, 1, 0)
}

// NewDistanceNpcIterator constructs an iterator that walks NPCs in zones
// within `distance` of (level, x, z), filtered by typeID (-1 = no filter).
// Mirrors TS NpcIterator constructor at ScriptIterators.ts:310-326 with
// type=DISTANCE.
//
// huntvis is consumed by passesFilter per TS ScriptIterators.ts:348-352
// (Distance mode filters by LoS/LoW like HuntAll). lv may be nil
// (pessimistic-allow per the lineValidator==nil convention at
// npcVisibleViaLineOfSight); production passes s.LineValidator from the
// handler call site.
//
// Bounds math (per TS line 312-321):
//
//	centerX = x >> 3
//	radius  = 1 + distance/8       // integer division
//	zone bounds = [center - radius, center + radius]
//
// Cursor starts at (maxZoneX, maxZoneZ) per TS line 337-340; advances
// outer X descending, inner Z descending in advanceZone.
func NewDistanceNpcIterator(lookup NpcLookup, lv LineValidator, tick, level, x, z, distance, huntvis, typeID int) *NpcIterator {
	centerX := x >> 3
	centerZ := z >> 3
	radius := 1 + distance/8
	return &NpcIterator{
		mode:          NpcIteratorDistance,
		creationTick:  tick,
		lookup:        lookup,
		lineValidator: lv,
		level:         level,
		x:             x,
		z:             z,
		distance:      distance,
		huntvis:       huntvis,
		typeID:        typeID,
		minZoneX:      centerX - radius,
		maxZoneX:      centerX + radius,
		minZoneZ:      centerZ - radius,
		maxZoneZ:      centerZ + radius,
		curZoneX:      centerX + radius,
		curZoneZ:      centerZ + radius,
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

// NewHuntAllNpcIterator constructs an iterator that walks NPCs in zones
// within `distance` of (level, x, z), filtered by huntvis (TS
// ScriptIterators.ts:284-287) and the NpcType.Op[1] operability gate
// (NAI-180 closes NAI-35-T3-D1; TS ScriptIterators.ts:274-280).
// No typeID filter (-1). Mirrors TS NpcHuntAllCommandIterator at
// ScriptIterators.ts:234-295. Bounds math identical to
// NewDistanceNpcIterator.
//
// configs is the cache-loaded NpcType provider; production passes
// s.Configs. Nil-Configs path is goscape defensive (TS throws on
// missing NpcType) — test fixtures pessimistically allow.
func NewHuntAllNpcIterator(lookup NpcLookup, lv LineValidator, configs Configs, tick, level, x, z, distance, huntvis int) *NpcIterator {
	centerX := x >> 3
	centerZ := z >> 3
	radius := 1 + distance/8
	return &NpcIterator{
		mode:          NpcIteratorHuntAll,
		creationTick:  tick,
		lookup:        lookup,
		lineValidator: lv,
		configs:       configs,
		level:         level,
		x:             x,
		z:             z,
		distance:      distance,
		huntvis:       huntvis,
		typeID:        -1,
		minZoneX:      centerX - radius,
		maxZoneX:      centerX + radius,
		minZoneZ:      centerZ - radius,
		maxZoneZ:      centerZ + radius,
		curZoneX:      centerX + radius,
		curZoneZ:      centerZ + radius,
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
