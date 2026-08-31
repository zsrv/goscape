package world

import (
	"log/slog"
	"runtime/debug"

	"github.com/zsrv/goscape/pkg/script"
)

// crashSaveAll is the tick loop's last act before an unrecovered panic
// kills the process: log the panic with its stack, fire a per-player
// best-effort PlayerAutosave for every online player, and wait (bounded
// by playerSaveFlushTimeout) for the RPCs to flush. The caller re-panics
// afterwards so crash semantics for supervisors are unchanged.
//
// DEVIATION SEC1-D2: TS World.cycle's catch logs and process.exit(1)s
// without saving — up to NODE_AUTOSAVE_INTERVAL of progress was lost for
// every player. goscape saves first, per-player best-effort: unlike the
// normal-cadence autosavePlayers, crashSavePlayers isolates each player's
// Save behind its own recover, since the loop is already panicking and a
// second panic (e.g. p.Save itself panicking on the corrupt state that
// caused the original crash) must not abort the pass and strand every
// player behind the offender unsaved.
func (s *Server) crashSaveAll(r any) {
	s.log.Error("unrecovered panic in tick loop; autosaving all players before exit",
		"err", r,
		"stack", string(debug.Stack()))
	s.crashSavePlayers(func(p *Player) []byte { return p.Save(s.invTypes, s.varpTypes) })
	s.waitForSaveFlush()
}

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
		p.client.closeConn()
	}
}

// logWorldScriptPanic emits the structured error log for a world-script-queue
// panic. The recover() itself is performed by fireWorldScript (it must run in
// that deferred frame to set the panicked return); this helper only formats
// the log. r is the recovered panic value.
//
// ARCH-1: the panicking entry is intentionally LEFT in the queue by the caller
// so it retries on the next tick, mirroring TS World.ts:542-558 (unlink runs
// after execute; a throw skips it). The prior remove-before-fire behavior
// (swallow, no retry) is closed.
//
// Mirrors TS World.ts:534-559 catch logging.
func logWorldScriptPanic(state *script.ScriptState, r any, log *slog.Logger) {
	scriptName := ""
	if state != nil && state.Script != nil {
		scriptName = state.Script.Name
	}
	log.Error("panic in world script execution (retrying next tick)",
		"script", scriptName,
		"err", r,
		"stack", string(debug.Stack()))
}

// recoverNpc recovers from panics during a per-NPC tick step.
//
// Mirrors TS World.processNpcs (World.ts:667-672 @1d25566c) catch action:
// structured log + `removeNpc(npc, 0)`. Engine-TS 8139461a changed the
// duration from -1 to 0 here: -1 is the "no transition scheduled" sentinel,
// so a panicking respawn-lifecycle npc was being evicted with no respawn
// scheduled at all. 0 schedules the transition for the next cycle instead
// (and the clamp in Entity.setLifeCycle floors it at tick 1).
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
	s.removeNpc(n, 0)
}

// recoverObjDelayed recovers from panics during objDelayedQueue fire
// (NAI-134): structured log + swallow. The offending request was already
// removed before fire (per processObjDelayedQueue's remove-before-fire
// ordering), so recovery only logs — there is no retry.
//
// This remove-before-fire / no-retry is the TS-FAITHFUL behavior for
// objDelayed: TS World.ts:566-572 runs request.unlink() BEFORE addObj, so a
// throw drops the request (unlike the world-script queue at World.ts:542-558,
// where unlink runs AFTER execute and a throw retries — see fireWorldScript /
// ARCH-1). Do not "align" this with the world-queue retry; they differ in TS.
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
