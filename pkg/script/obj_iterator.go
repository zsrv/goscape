package script

// ObjIterator is the script-VM iterator state for the OBJ_FIND iterator
// family (currently OBJ_FINDALLZONE only — single-mode like LocIterator,
// unlike NpcIterator's DISTANCE/ZONE/HuntAll). Mirrors TS ObjIterator at
// ScriptIterators.ts:387-407.
//
// Lifetime: single-tick. Created by OBJ_FINDALLZONE; consumed by
// OBJ_FINDNEXT. Stale() check at FINDNEXT compares creationTick to
// World.CurrentTick(); on mismatch, the handler returns an error
// mirroring the LOC family pattern.
//
// Snapshot strategy: lazy on first Next() call via
// WorldVars.ZoneObjs(level, x, z). TS uses a generator over
// `getZone(...).getAllObjsSafe(true)` — equivalent because both produce
// a single point-in-time slice that the iterator drains independent of
// subsequent zone mutation.
//
// Ownership: held by ScriptState.objIterator. Nil = no active iterator.
type ObjIterator struct {
	creationTick int
	world        WorldVars
	level, x, z  int
	objs         []ActiveObj
	idx          int
	started      bool
}

// NewZoneObjIterator constructs a single-zone iterator for the zone
// containing (level, x, z). Mirrors TS ObjIterator constructor at
// ScriptIterators.ts:392-396. The snapshot is deferred to first Next();
// the constructor only stores center coords and tick.
func NewZoneObjIterator(world WorldVars, tick, level, x, z int) *ObjIterator {
	return &ObjIterator{
		creationTick: tick,
		world:        world,
		level:        level,
		x:            x,
		z:            z,
	}
}

// Stale reports whether the iterator was created in a prior tick. The
// FINDNEXT handler MUST check this before calling Next when single-tick
// lifetime matters. Mirrors TS strict-greater-than at
// ScriptIterators.ts:401 (World.currentTick > this.tick).
func (it *ObjIterator) Stale(currentTick int) bool {
	return currentTick > it.creationTick
}

// Next returns the next obj in the zone snapshot, or (nil, false) on
// exhaustion. Lazy-initializes the snapshot on first call.
//
// Nil-world degrades to immediate exhaustion (test stub or pre-wiring) —
// mirrors LocIterator.Next nil-ops handling (loc_iterator.go).
func (it *ObjIterator) Next() (ActiveObj, bool) {
	if !it.started {
		it.started = true
		if it.world != nil {
			it.objs = it.world.ZoneObjs(it.level, it.x, it.z)
		}
	}
	if it.idx >= len(it.objs) {
		return nil, false
	}
	obj := it.objs[it.idx]
	it.idx++
	return obj, true
}
