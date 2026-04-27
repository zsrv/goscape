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
	// huntvis is stored at construction (validated upstream by handlers
	// via checkHuntVis) but NOT consumed by passesFilter today — see
	// NAI-33-D1 deviation. Field kept (rather than dropped) for
	// retirement readiness: when LoS/LoW filtering lands, passesFilter
	// only needs to start reading huntvis; no constructor surface change.
	huntvis int
	// lineValidator is the LoS/LoW validator used by HuntAll-mode
	// passesFilter when huntvis ∈ {LineOfSight, LineOfWalk}. Nil = no
	// validator wired (test stub or pre-wiring); production sets via
	// the constructor. NAI-35-T3.
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
// HuntAll mode (NAI-35-T3) activates the huntvis branch — ZONE mode
// remains unfiltered (matches TS line 329-335). Distance mode keeps
// the pre-NAI-35 deferred behavior (huntvis validated but not consumed;
// tracked as NAI-33-D1 / S7f-D1). TS DOES filter Distance mode by
// huntvis (ScriptIterators.ts:348-352); goscape's deferred posture is
// intentional pending FINDALL-family consumer audit.
// Accessor names match pkg/script/active.go:400-408 ActiveNpc interface
// (NpcX/NpcZ/NpcType, NOT X/Z/Type).
func (it *NpcIterator) passesFilter(npc ActiveNpc) bool {
	if it.mode == NpcIteratorZone {
		return true // ZONE mode: no per-NPC filtering per TS line 329-335
	}
	if coordgrid.DistanceToSW(it.x, it.z, npc.NpcX(), npc.NpcZ()) > it.distance {
		return false
	}
	if it.mode == NpcIteratorHuntAll {
		// NAI-35-T3-D1 deviation: TS NpcHuntAllCommandIterator
		// (ScriptIterators.ts:274-280) ALSO rejects NPCs whose
		// NpcType.Op[1] is empty (operability gate). Goscape skips this
		// filter pending plumbing Configs onto NpcIterator. Content-script
		// audit will decide port-vs-keep; tracked in nai_followups.md.
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
	}
	if it.typeID >= 0 && npc.NpcType() != it.typeID {
		return false
	}
	return true
}

// npcVisibleViaLineOfSight returns true when the iterator's lineValidator
// passes a LoS check from the iterator's center coord to the NPC. Nil
// validator = pessimistically allow. NAI-35-T3.
// (srcSize=1, destWidth=destLength=0, extraFlag=0) — single-tile src
// against a zero-size NPC dest; mirrors TS isLineOfSight wrapper at
// ScriptIterators.ts:359-361.
func (it *NpcIterator) npcVisibleViaLineOfSight(npc ActiveNpc) bool {
	if it.lineValidator == nil {
		return true
	}
	return it.lineValidator.HasLineOfSight(it.level, it.x, it.z, npc.NpcX(), npc.NpcZ(), 1, 0, 0, 0)
}

// npcVisibleViaLineOfWalk returns true when the iterator's lineValidator
// passes a LoW check. Nil validator = pessimistically allow. NAI-35-T3.
// (srcSize=1, destWidth=destLength=0, extraFlag=0) — single-tile src
// against a zero-size NPC dest; mirrors TS isLineOfWalk wrapper at
// ScriptIterators.ts:359-361.
func (it *NpcIterator) npcVisibleViaLineOfWalk(npc ActiveNpc) bool {
	if it.lineValidator == nil {
		return true
	}
	return it.lineValidator.HasLineOfWalk(it.level, it.x, it.z, npc.NpcX(), npc.NpcZ(), 1, 0, 0, 0)
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

// NewHuntAllNpcIterator constructs an iterator that walks NPCs in zones
// within `distance` of (level, x, z), filtered by huntvis (now ACTIVE
// per NAI-35-T3 — closes NAI-33-D1) and no typeID filter (-1). Mirrors
// TS NpcHuntAllCommandIterator at ScriptIterators.ts:234-295. Bounds math
// identical to NewDistanceNpcIterator. HuntAll mode is distinguished
// only by passesFilter activating huntvis-based LoS/LoW filtering.
func NewHuntAllNpcIterator(lookup NpcLookup, lv LineValidator, tick, level, x, z, distance, huntvis int) *NpcIterator {
	centerX := x >> 3
	centerZ := z >> 3
	radius := 1 + distance/8
	return &NpcIterator{
		mode:          NpcIteratorHuntAll,
		creationTick:  tick,
		lookup:        lookup,
		lineValidator: lv,
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
