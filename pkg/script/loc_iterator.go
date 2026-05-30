package script

// LocIterator is the script-VM iterator state for the LOC_FIND iterator
// family (currently LOC_FINDALLZONE only — the LOC iterator family is
// single-mode, unlike NpcIterator's DISTANCE/ZONE/HuntAll). Mirrors TS
// LocIterator at ScriptIterators.ts:365-385.
//
// Lifetime: single-tick. Created by LOC_FINDALLZONE; consumed by
// LOC_FINDNEXT. Stale() check at FINDNEXT compares creationTick to
// World.CurrentTick(); on mismatch the handler returns an error
// mirroring the NPC family pattern (npc_script.go log-warn +
// ClearActiveScript path runs).
//
// Snapshot strategy: lazy on first Next() call via
// LocOps.AllLocsSafe(level, x, z, reverse=true). Subsequent calls drain
// the snapshot. Mirrors TS `getZone(...).getAllLocsSafe(true)` exactly:
// iter visits IsActive locs only, in reverse zone order. The snapshot
// is point-in-time and is drained independent of subsequent zone
// mutation.
//
// Ownership: held by ScriptState.locIterator. Nil = no active iterator.
type LocIterator struct {
	creationTick int
	ops          LocOps
	level, x, z  int
	locs         []ActiveLoc
	idx          int
	started      bool
}

// NewZoneLocIterator constructs a single-zone iterator for the zone
// containing (level, x, z). Mirrors TS LocIterator constructor at
// ScriptIterators.ts:370-374. The snapshot is deferred to first Next();
// the constructor only stores center coords and tick.
func NewZoneLocIterator(ops LocOps, tick, level, x, z int) *LocIterator {
	return &LocIterator{
		creationTick: tick,
		ops:          ops,
		level:        level,
		x:            x,
		z:            z,
	}
}

// Stale reports whether the iterator was created in a prior tick. The
// FINDNEXT handler MUST check this before calling Next when single-tick
// lifetime matters. Mirrors TS strict-greater-than at
// ScriptIterators.ts:379 (World.currentTick > this.tick).
func (it *LocIterator) Stale(currentTick int) bool {
	return currentTick > it.creationTick
}

// Next returns the next loc in the zone snapshot, or (nil, false) on
// exhaustion. Lazy-initializes the snapshot on first call.
//
// Nil-ops degrades to immediate exhaustion (test stub or pre-wiring) —
// mirrors NpcIterator.Next nil-lookup handling at npc_iterator.go:238-240.
func (it *LocIterator) Next() (ActiveLoc, bool) {
	if !it.started {
		it.started = true
		if it.ops != nil {
			// reverse=true mirrors TS ScriptIterators.ts:378
			// (getAllLocsSafe(true)); filtering on IsActive happens
			// inside AllLocsSafe (Zone.ts:459-465).
			it.locs = it.ops.AllLocsSafe(it.level, it.x, it.z, true)
		}
	}
	if it.idx >= len(it.locs) {
		return nil, false
	}
	loc := it.locs[it.idx]
	it.idx++
	return loc, true
}
