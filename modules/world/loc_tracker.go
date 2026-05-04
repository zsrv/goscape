package world

import (
	"fmt"
	"iter"
	"log/slog"
	"runtime"

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

	// log + nodeDebug back the NAI-88 Stage 1 probes (P5/P6) at
	// Register/Unregister. Both are nil-safe; production passes
	// (s.log, s.cfg.NodeDebug) at Server.New, test fixtures pass
	// (nil, false) and the probe emit-helper short-circuits.
	// NAI-88 probe; remove at Stage 2 close.
	log       *slog.Logger
	nodeDebug bool
}

// newLocObjTracker constructs an empty tracker. Server.New calls this
// once at server startup. log+nodeDebug back NAI-88 Stage 1 probes;
// pass (nil, false) in tests to no-op them.
func newLocObjTracker(log *slog.Logger, nodeDebug bool) *locObjTracker {
	return &locObjTracker{
		list:      &zone.DoublyLinkList[*entitypkg.NonPathing]{},
		nodes:     map[*entitypkg.NonPathing]*zone.Element[*entitypkg.NonPathing]{},
		log:       log,
		nodeDebug: nodeDebug,
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
	// NAI-88 probe; remove at Stage 2 close.
	if t.nodeDebug && t.log != nil {
		t.log.Debug("nai88 tracker register",
			"event_id", "P5",
			"np_addr", fmt.Sprintf("%p", np),
			"tracker_size_after", t.list.Size(),
		)
	}
}

// Unregister removes np from the tracker. No-op if np is not tracked.
func (t *locObjTracker) Unregister(np *entitypkg.NonPathing) {
	hit := false
	if e, ok := t.nodes[np]; ok {
		e.Unlink()
		delete(t.nodes, np)
		hit = true
	}
	// NAI-88 probe; remove at Stage 2 close.
	if t.nodeDebug && t.log != nil {
		caller := "unknown"
		if _, file, line, ok := runtime.Caller(1); ok {
			caller = fmt.Sprintf("%s:%d", file, line)
		}
		t.log.Debug("nai88 tracker unregister",
			"event_id", "P6",
			"np_addr", fmt.Sprintf("%p", np),
			"hit", hit,
			"tracker_size_after", t.list.Size(),
			"caller", caller,
		)
	}
}

// All returns an iterator over the tracked entries in insertion order.
// Callers that mutate the tracker mid-iteration MUST snapshot first
// (Server.processZones does this).
func (t *locObjTracker) All() iter.Seq[*entitypkg.NonPathing] {
	return t.list.All(false)
}
