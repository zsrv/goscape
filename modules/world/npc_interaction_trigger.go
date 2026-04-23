package world

import (
	entitypkg "github.com/zsrv/goscape/pkg/entity"
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/script"
)

// fireAiOpTriggerLoc fires AI_OPLOC1..5 for a Loc target. Lifecycle
// gate via locStillValid (zone-membership + type match). Category
// resolved through the LocType registry (Loc carries only a packed
// Info bitfield, so category is a separate lookup).
func (n *Npc) fireAiOpTriggerLoc(s *Server, target *entitypkg.Loc) {
	tx, tz, tlevel := target.Coords()
	if !locStillValid(s, target, n.targetSubject.typ, tx, tz, tlevel) {
		n.clearInteraction()
		return
	}
	trigger := aiOpLocTriggerForOp(n.targetOp)
	if trigger == 0 {
		n.clearInteraction()
		return
	}
	locId := target.Type()
	category := locCategory(s, locId)
	sf := s.scriptProvider.GetByTrigger(trigger, locId, category)
	if sf == nil {
		n.clearInteraction()
		return
	}
	s.runNpcScript(sf, n, target, nil, nil)
}

// fireAiApTriggerLoc fires AI_APLOC1..5 for a Loc target — approach-
// range counterpart.
func (n *Npc) fireAiApTriggerLoc(s *Server, target *entitypkg.Loc) {
	tx, tz, tlevel := target.Coords()
	if !locStillValid(s, target, n.targetSubject.typ, tx, tz, tlevel) {
		n.clearInteraction()
		return
	}
	trigger := aiApLocTriggerForOp(n.targetOp)
	if trigger == 0 {
		n.clearInteraction()
		return
	}
	locId := target.Type()
	category := locCategory(s, locId)
	sf := s.scriptProvider.GetByTrigger(trigger, locId, category)
	if sf == nil {
		n.clearInteraction()
		return
	}
	s.runNpcScript(sf, n, target, nil, nil)
}

// locCategory resolves a loc's category through the server's LocType
// registry. Returns 0 if the registry is nil or the id is out of range
// — matches the defensive access pattern at handler_oploc.go:68-72.
func locCategory(s *Server, locId int) int {
	if s.locTypes == nil || locId < 0 || locId >= len(s.locTypes.Configs) {
		return 0
	}
	lt := s.locTypes.Configs[locId]
	if lt == nil {
		return 0
	}
	return lt.Category
}

// fireAiOpTriggerNpc fires AI_OPNPC1..5 for an NPC target. Lifecycle
// gate is target.dead (the "isActive" half handled by validateTarget).
// Category read from target.typ.Category (TS parity).
func (n *Npc) fireAiOpTriggerNpc(s *Server, target *Npc) {
	if target.dead {
		n.clearInteraction()
		return
	}
	trigger := aiOpNpcTriggerForOp(n.targetOp)
	if trigger == 0 {
		n.clearInteraction()
		return
	}
	category := 0
	if target.typ != nil {
		category = target.typ.Category
	}
	sf := s.scriptProvider.GetByTrigger(trigger, target.typeId, category)
	if sf == nil {
		n.clearInteraction()
		return
	}
	s.runNpcScript(sf, n, target, nil, nil)
}

// fireAiApTriggerNpc fires AI_APNPC1..5 for an NPC target — approach-
// range counterpart.
func (n *Npc) fireAiApTriggerNpc(s *Server, target *Npc) {
	if target.dead {
		n.clearInteraction()
		return
	}
	trigger := aiApNpcTriggerForOp(n.targetOp)
	if trigger == 0 {
		n.clearInteraction()
		return
	}
	category := 0
	if target.typ != nil {
		category = target.typ.Category
	}
	sf := s.scriptProvider.GetByTrigger(trigger, target.typeId, category)
	if sf == nil {
		n.clearInteraction()
		return
	}
	s.runNpcScript(sf, n, target, nil, nil)
}

// fireAiOpTriggerPlayer fires AI_OPPLAYER1..5 for a Player target.
// Called by tryInteract when targetOp is OPPLAYER + in operable range.
// Mirrors TS Npc.tryInteract → executeScript branch. Clears the
// interaction on lifecycle-invalid target, out-of-range targetOp, or
// no-script-found.
func (n *Npc) fireAiOpTriggerPlayer(s *Server, target *Player) {
	if !target.IsValid() {
		n.clearInteraction()
		return
	}
	trigger := aiOpPlayerTriggerForOp(n.targetOp)
	if trigger == 0 {
		n.clearInteraction()
		return
	}
	sf := s.scriptProvider.GetByTrigger(trigger, 0, 0)
	if sf == nil {
		n.clearInteraction()
		return
	}
	s.runNpcScript(sf, n, target, nil, nil)
}

// fireAiApTriggerPlayer fires AI_APPLAYER1..5 for a Player target —
// approach-range counterpart of fireAiOpTriggerPlayer.
func (n *Npc) fireAiApTriggerPlayer(s *Server, target *Player) {
	if !target.IsValid() {
		n.clearInteraction()
		return
	}
	trigger := aiApPlayerTriggerForOp(n.targetOp)
	if trigger == 0 {
		n.clearInteraction()
		return
	}
	sf := s.scriptProvider.GetByTrigger(trigger, 0, 0)
	if sf == nil {
		n.clearInteraction()
		return
	}
	s.runNpcScript(sf, n, target, nil, nil)
}

// aiApPlayerTriggerForOp maps an APPLAYER targetOp (12..16) to the
// matching TriggerAiApPlayer{1..5}. Returns 0 for out-of-range.
func aiApPlayerTriggerForOp(op int) script.ServerTriggerType {
	if op >= objtype.NPCModeApPlayer1 && op <= objtype.NPCModeApPlayer5 {
		return script.TriggerAiApPlayer1 + script.ServerTriggerType(op-objtype.NPCModeApPlayer1)
	}
	return 0
}

// aiOpPlayerTriggerForOp: OPPLAYER targetOp (7..11) → TriggerAiOpPlayer{1..5}.
func aiOpPlayerTriggerForOp(op int) script.ServerTriggerType {
	if op >= objtype.NPCModeOpPlayer1 && op <= objtype.NPCModeOpPlayer5 {
		return script.TriggerAiOpPlayer1 + script.ServerTriggerType(op-objtype.NPCModeOpPlayer1)
	}
	return 0
}

// aiApLocTriggerForOp: APLOC targetOp (22..26) → TriggerAiApLoc{1..5}.
func aiApLocTriggerForOp(op int) script.ServerTriggerType {
	if op >= objtype.NPCModeApLoc1 && op <= objtype.NPCModeApLoc5 {
		return script.TriggerAiApLoc1 + script.ServerTriggerType(op-objtype.NPCModeApLoc1)
	}
	return 0
}

// aiOpLocTriggerForOp: OPLOC targetOp (17..21) → TriggerAiOpLoc{1..5}.
func aiOpLocTriggerForOp(op int) script.ServerTriggerType {
	if op >= objtype.NPCModeOpLoc1 && op <= objtype.NPCModeOpLoc5 {
		return script.TriggerAiOpLoc1 + script.ServerTriggerType(op-objtype.NPCModeOpLoc1)
	}
	return 0
}

// aiApObjTriggerForOp: APOBJ targetOp (32..36) → TriggerAiApObj{1..5}.
func aiApObjTriggerForOp(op int) script.ServerTriggerType {
	if op >= objtype.NPCModeApObj1 && op <= objtype.NPCModeApObj5 {
		return script.TriggerAiApObj1 + script.ServerTriggerType(op-objtype.NPCModeApObj1)
	}
	return 0
}

// aiOpObjTriggerForOp: OPOBJ targetOp (27..31) → TriggerAiOpObj{1..5}.
func aiOpObjTriggerForOp(op int) script.ServerTriggerType {
	if op >= objtype.NPCModeOpObj1 && op <= objtype.NPCModeOpObj5 {
		return script.TriggerAiOpObj1 + script.ServerTriggerType(op-objtype.NPCModeOpObj1)
	}
	return 0
}

// aiApNpcTriggerForOp: APNPC targetOp (42..46) → TriggerAiApNpc{1..5}.
func aiApNpcTriggerForOp(op int) script.ServerTriggerType {
	if op >= objtype.NPCModeApNpc1 && op <= objtype.NPCModeApNpc5 {
		return script.TriggerAiApNpc1 + script.ServerTriggerType(op-objtype.NPCModeApNpc1)
	}
	return 0
}

// aiOpNpcTriggerForOp: OPNPC targetOp (37..41) → TriggerAiOpNpc{1..5}.
func aiOpNpcTriggerForOp(op int) script.ServerTriggerType {
	if op >= objtype.NPCModeOpNpc1 && op <= objtype.NPCModeOpNpc5 {
		return script.TriggerAiOpNpc1 + script.ServerTriggerType(op-objtype.NPCModeOpNpc1)
	}
	return 0
}
