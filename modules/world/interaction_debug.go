package world

import (
	entitypkg "github.com/zsrv/goscape/pkg/entity"
	applog "github.com/zsrv/goscape/pkg/util/log"
)

// targetTypeID extracts the type/config ID from an interaction target for
// logging. Uses a type-switch since the entity interface does not expose
// a Type() method. Returns -1 for unknown/nil targets.
func targetTypeID(t entity) int {
	switch v := t.(type) {
	case *entitypkg.Loc:
		return v.Type()
	case *entitypkg.Obj:
		return v.ObjType()
	case *Npc:
		return v.typeId
	case *Player:
		// Player trigger lookup uses typeId=-1 (no config-layer Player
		// type). Return -1 rather than v.slot so log readers don't
		// confuse a session-local slot with a config ID; matches H2
		// routing rule's "target_type_id == loc_id" semantic.
		_ = v
		return -1
	default:
		return -1
	}
}

// emitInteractionTickFrame writes the NAI-79 Stage 1 "interaction tick"
// Frame B record. Caller (processInteraction) gates on hadTarget; this
// helper additionally gates on s.cfg.NodeDebug. All target-coord fields
// refer to the INITIAL target (snapshotted by the caller at function
// entry); target_still_set separately signals whether p.target was
// nulled during the tick.
func emitInteractionTickFrame(
	s *Server,
	p *Player,
	hadTarget bool,
	initialTarget entity,
	initialTargetX, initialTargetZ int,
	opTriggerPresent, apTriggerPresent bool,
	interactedFinal bool,
) {
	if !hadTarget || !s.cfg.NodeDebug || s.log == nil {
		return
	}
	applog.Trace(s.log, "interaction tick",
		"tick", s.currentTick,
		"player_uid", p.uid,
		"target_kind", targetKindString(initialTarget),
		"target_type_id", targetTypeID(initialTarget),
		"target_x", initialTargetX,
		"target_z", initialTargetZ,
		"player_x", p.x,
		"player_z", p.z,
		"cheb_dist", chebDist(p.x, p.z, initialTargetX, initialTargetZ),
		"op_trigger", opTriggerPresent,
		"ap_trigger", apTriggerPresent,
		"ap_range", p.apRange,
		"waypoint_idx", p.waypointIndex,
		"branch_pre", p.lastInteractBranchPre,
		"branch_post", p.lastInteractBranchPost,
		"interacted", interactedFinal,
		"steps_taken", p.stepsTaken,
		"repathed", p.repathed,
		"target_still_set", p.target != nil,
		"modal_state", p.modalState,
		"active_script_exec", activeScriptExec(p),
		"protected_active", p.protectedScriptActive(),
	)
}

// activeScriptExec returns -1 if p.activeScript is nil, otherwise int(Execution).
func activeScriptExec(p *Player) int {
	if p.activeScript == nil {
		return -1
	}
	return int(p.activeScript.Execution)
}

// chebDist returns the Chebyshev distance between two tile coords.
// Used by NAI-79 Stage 1 instrumentation (interaction tick frame).
func chebDist(ax, az, bx, bz int) int {
	dx := ax - bx
	if dx < 0 {
		dx = -dx
	}
	dz := az - bz
	if dz < 0 {
		dz = -dz
	}
	if dx > dz {
		return dx
	}
	return dz
}

// emitOpLocGate emits a gate-name slog.Debug record for one of
// handleOpLoc's six gate names (seven early-return branches; the
// out-of-range and registered-as-nil locType checks share gate name
// "loctype_nil"). NodeDebug-gated; nil-log safe.
//
// Field schema (NAI-79 Bundle H4):
//
//	tick / player_uid / gate / op / click_x / click_z / loc_id
//
// Pre-decode gates ("delayed", "payload_short") pass (-1, -1, -1) for
// (x, z, locId) since the 6-byte payload has not been parsed yet. The
// -1 sentinel matches the project's apRange=-1 convention and keeps
// the field set uniform across all gate emits.
func emitOpLocGate(s *Server, p *Player, gate string, op, x, z, locId int) {
	if !s.cfg.NodeDebug || s.log == nil {
		return
	}
	applog.Trace(s.log, "oploc gate",
		"tick", s.currentTick,
		"player_uid", p.uid,
		"gate", gate,
		"op", op,
		"click_x", x,
		"click_z", z,
		"loc_id", locId,
	)
}

// recordTryInteractBranch is the side-channel writer used by
// (*Player).tryInteract to surface which of its 4 branches (or
// fallthrough = 0) returned. processInteraction sets p.interactCallSlot
// to 0 before its pre-step call and 1 before its post-step call; this
// helper picks the right Player field based on the slot value.
func recordTryInteractBranch(p *Player, branch int) {
	if p.interactCallSlot == 1 {
		p.lastInteractBranchPost = branch
	} else {
		p.lastInteractBranchPre = branch
	}
}

// targetKindString returns a stable string label for an interaction
// target so the NAI-79 interaction tick frame can name the target type
// without relying on slog's reflection-based formatting. Returns
// "unknown" for nil or unrecognized types.
func targetKindString(t entity) string {
	switch t.(type) {
	case *entitypkg.Loc:
		return "Loc"
	case *entitypkg.Obj:
		return "Obj"
	case *Npc:
		return "Npc"
	case *Player:
		return "Player"
	default:
		return "unknown"
	}
}
