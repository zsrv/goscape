package world

import (
	"iter"

	entitypkg "github.com/zsrv/goscape/pkg/entity"
	"github.com/zsrv/goscape/pkg/zone"
)

// locObjTracker is the per-Server registry of NonPathing entities with
// pending lifecycle transitions. Iterated each tick by Server.processZones.
// Mirrors TS World.locObjTracker (Engine-TS/.../World.ts:154,964-973).
//
// Backed by pkg/zone.DoublyLinkList for O(1) Add/Unlink and an auxiliary
// map *NonPathing → *Element for O(1) Unregister-by-pointer.
type locObjTracker struct {
	list  *zone.DoublyLinkList[*entitypkg.NonPathing]
	nodes map[*entitypkg.NonPathing]*zone.Element[*entitypkg.NonPathing]
}

// newLocObjTracker constructs an empty tracker. Server.New calls this
// once at server startup.
func newLocObjTracker() *locObjTracker {
	return &locObjTracker{
		list:  &zone.DoublyLinkList[*entitypkg.NonPathing]{},
		nodes: map[*entitypkg.NonPathing]*zone.Element[*entitypkg.NonPathing]{},
	}
}

// Register adds np to the tracker. Idempotent — re-registering an
// already-tracked np unlinks the old node first to keep the list
// duplicate-free, matching TS behavior where setLifeCycle always
// unlinks the previous eventTracker before re-adding.
func (t *locObjTracker) Register(np *entitypkg.NonPathing) {
	if existing, ok := t.nodes[np]; ok {
		existing.Unlink()
		delete(t.nodes, np)
	}
	t.nodes[np] = t.list.AddTail(np)
}

// Unregister removes np from the tracker. No-op if np is not tracked.
func (t *locObjTracker) Unregister(np *entitypkg.NonPathing) {
	if e, ok := t.nodes[np]; ok {
		e.Unlink()
		delete(t.nodes, np)
	}
}

// All returns an iterator over the tracked entries in insertion order.
// Safe under mid-iteration removal of the JUST-YIELDED entry via
// Unregister (datastruct-db-1, 2026-05-28 audit): DoublyLinkList.All
// captures the next pointer before yielding so the iterator survives
// the Unlink. Removing OTHER entries during a yield body (specifically
// the saved next pointer) remains unsafe — Server.processZones already
// snapshots to side-step that case, and the snapshot pattern is the
// recommended approach for callers that touch more than the current
// node.
func (t *locObjTracker) All() iter.Seq[*entitypkg.NonPathing] {
	return t.list.All(false)
}
