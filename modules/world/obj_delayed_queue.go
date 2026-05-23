package world

import (
	entitypkg "github.com/zsrv/goscape/pkg/entity"
)

// objDelayedRequest is one INV_DROPITEM_DELAYED request awaiting drain.
// Mirrors TS ObjDelayedRequest (Engine-TS/src/engine/entity/ObjDelayedRequest.ts).
//
// DEVIATION-NAI-134-D1: TS uses LinkList<ObjDelayedRequest> (Linkable mixin).
// Goscape uses a slice on Server, mirroring worldScriptQueue. Behavior identical.
type objDelayedRequest struct {
	obj              *entitypkg.Obj
	receiverID       int
	duration         int
	delay            int   // ticks remaining; post-decrement per TS World.ts:564
	dropperAccountID int64 // persistent account_id of the dropper; 0 for system/NPC spawns
}

// enqueueObjDelayed appends a request to s.objDelayedQueue. Called by
// worldVarsView.EnqueueObjDelayed (server_varp.go) which is in turn
// driven by handleInvDropItemDelayed (pkg/script/handlers_inv.go).
//
// Mirrors TS World.objDelayedQueue.addTail at InvOps.ts:208. No `+1`
// offset — TS stores delay verbatim (unlike worldScriptQueue which
// stores delay+1 per TS World.ts:1239).
func (s *Server) enqueueObjDelayed(obj *entitypkg.Obj, receiverID, duration, delay int, dropperAccountID int64) {
	s.objDelayedQueue = append(s.objDelayedQueue, objDelayedRequest{
		obj:              obj,
		receiverID:       receiverID,
		duration:         duration,
		delay:            delay,
		dropperAccountID: dropperAccountID,
	})
}

// processObjDelayedQueue drains ready entries from s.objDelayedQueue,
// firing each by calling s.AddObj (zone routing).
//
// Index-based slice walk with mid-pass append visibility (re-reads
// len(s.objDelayedQueue) each iteration), mirroring processWorldQueue
// (world_script_queue.go:59). Removal happens BEFORE fire so a panicking
// fire path doesn't leave a dead entry in the queue (recoverObjDelayed
// in tick_recovery.go).
//
// Mirrors TS World.cycle objDelayedQueue iteration at World.ts:563-573,
// including the per-iteration try/catch (mirrors NAI-42 pattern).
func (s *Server) processObjDelayedQueue() {
	i := 0
	for i < len(s.objDelayedQueue) {
		e := &s.objDelayedQueue[i]
		// POST-decrement: capture current, then decrement. Mirrors TS
		// World.ts:564 (`const delay = request.delay--;`).
		delay := e.delay
		e.delay--
		if delay > 0 {
			i++
			continue
		}
		req := *e
		s.objDelayedQueue = append(s.objDelayedQueue[:i], s.objDelayedQueue[i+1:]...)
		func(req objDelayedRequest) {
			defer recoverObjDelayed(req, s.log)
			s.AddObj(req.obj, req.receiverID, req.duration, req.dropperAccountID)
		}(req)
		// Don't advance i — slice contracted under us (mirrors processWorldQueue).
	}
}
