package world

import (
	"log/slog"
	"runtime/debug"

	"github.com/zsrv/goscape/pkg/script"
)

// recoverPlayer recovers from panics during a per-player tick step.
//
// Mirrors TS World.processClients (World.ts:651-657) and World.processPlayers
// (World.ts:736-742) catch action: structured log + force-disconnect. Sets
// p.requestLogout so the existing processLogouts loop (tick.go:140) picks
// the player up next tick, and closes the TCP connection immediately so
// any further decode attempt fails fast.
//
// Must be called from inside a `defer recoverPlayer(...)` registered as
// the FIRST deferred call in a per-iteration closure — Go semantics
// require recover() to run inside the deferred frame.
//
// op identifies the tick step ("processIn", "processInteraction", etc.)
// for log readability; pass a constant string per call site.
func recoverPlayer(p *Player, op string, log *slog.Logger) {
	r := recover()
	if r == nil {
		return
	}
	username := ""
	if p != nil {
		username = p.username
	}
	log.Error("panic in tick step",
		"op", op,
		"player", username,
		"err", r,
		"stack", string(debug.Stack()))
	if p == nil {
		return
	}
	p.requestLogout = true
	if p.client != nil && p.client.conn != nil {
		_ = p.client.conn.Close()
	}
}

// recoverWorldScript recovers from panics during world-script-queue
// execution. The world queue has no Player to disconnect; the offending
// entry was already removed before fire (per processWorldQueue's
// remove-before-fire ordering at world_script_queue.go:75), so recovery
// only logs.
//
// Mirrors TS World.ts:534-559 catch action.
//
// PORTING-EXCEPTION (ARCH-1): the world-script panic is swallowed here (the
// offending entry was already removed before fire). TS retries via top-level
// catch. Risk: masks logic bugs that TS would propagate. Documented; deferred
// indefinitely. See PORTING.md.
func recoverWorldScript(state *script.ScriptState, log *slog.Logger) {
	r := recover()
	if r == nil {
		return
	}
	scriptName := ""
	if state != nil && state.Script != nil {
		scriptName = state.Script.Name
	}
	log.Error("panic in world script execution",
		"script", scriptName,
		"err", r,
		"stack", string(debug.Stack()))
}

// recoverNpc recovers from panics during a per-NPC tick step.
//
// Mirrors TS World.processNpcs (World.ts:681-690) catch action: structured
// log + `removeNpc(npc, -1)`. The TS catch evicts the offending NPC with
// the -1 duration sentinel so the despawn/respawn branch in removeNpc
// keys off the npc's existing lifecycle field (no caller-imposed scaling).
//
// Must be called from inside a `defer recoverNpc(...)` registered as the
// FIRST deferred call in a per-iteration closure — Go semantics require
// recover() to run inside the deferred frame.
//
// op identifies the tick step ("processNpcTurn", etc.) for log
// readability; pass a constant string per call site.
func recoverNpc(n *Npc, s *Server, op string, log *slog.Logger) {
	r := recover()
	if r == nil {
		return
	}
	nid := -1
	typeId := -1
	if n != nil {
		nid = n.nid
		typeId = n.typeId
	}
	log.Error("panic in tick step",
		"op", op,
		"nid", nid,
		"typeId", typeId,
		"err", r,
		"stack", string(debug.Stack()))
	if s == nil || n == nil {
		return
	}
	s.removeNpc(n, -1)
}

// recoverObjDelayed recovers from panics during objDelayedQueue fire
// (NAI-134). Mirrors recoverWorldScript: structured log + swallow. The
// offending request was already removed before fire (per
// processObjDelayedQueue's remove-before-fire ordering), so recovery
// only logs.
//
// Mirrors TS World.ts:566-572 catch action.
func recoverObjDelayed(req objDelayedRequest, log *slog.Logger) {
	r := recover()
	if r == nil {
		return
	}
	typeID := -1
	if req.obj != nil {
		typeID = req.obj.Type
	}
	log.Error("panic in objDelayedQueue fire",
		"typeID", typeID,
		"receiverID", req.receiverID,
		"duration", req.duration,
		"err", r,
		"stack", string(debug.Stack()))
}
