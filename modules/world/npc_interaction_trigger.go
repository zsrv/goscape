package world

import (
	entitypkg "github.com/zsrv/goscape/pkg/entity"
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/script"
)

// fireAiOpTrigger dispatches to the per-target-kind OP fire helper
// based on n.target's concrete type (selecting the OPNPC/OPLOC/OPOBJ/
// OPPLAYER trigger family). Called by tryInteract when
// targetOp is in an OP band and target is in operable distance.
// Unknown target types silently no-op.
func (n *Npc) fireAiOpTrigger(s *Server) {
	switch t := n.target.(type) {
	case *Player:
		n.fireAiOpTriggerPlayer(s, t)
	case *Npc:
		n.fireAiOpTriggerNpc(s, t)
	case *entitypkg.Loc:
		n.fireAiOpTriggerLoc(s, t)
	case *entitypkg.Obj:
		n.fireAiOpTriggerObj(s, t)
	}
}

// fireAiApTrigger — same shape as fireAiOpTrigger for the AP band.
func (n *Npc) fireAiApTrigger(s *Server) {
	switch t := n.target.(type) {
	case *Player:
		n.fireAiApTriggerPlayer(s, t)
	case *Npc:
		n.fireAiApTriggerNpc(s, t)
	case *entitypkg.Loc:
		n.fireAiApTriggerLoc(s, t)
	case *entitypkg.Obj:
		n.fireAiApTriggerObj(s, t)
	}
}

// aiTriggerCategory returns the ACTING npc's own category for AI trigger
// resolution. TS Npc.tryInteract resolves the trigger via getTrigger(type)
// where type = NpcType.get(this.type), so getByTrigger is keyed on the
// acting npc's type+category for every target kind, never the target's
// (src/engine/entity/Npc.ts:865-866,989-995). Returns 0 when typ is unset.
func (n *Npc) aiTriggerCategory() int {
	if n.typ == nil {
		return 0
	}
	return n.typ.Category
}

// fireAiOpTriggerObj fires AI_OPOBJ1..5 for an Obj target. Lifecycle
// gate via objStillValid (zone-membership). The trigger is keyed on the
// acting npc's type+category (TS Npc.ts:992), not the obj's.
func (n *Npc) fireAiOpTriggerObj(s *Server, target *entitypkg.Obj) {
	tx, tz, tlevel := target.Coords()
	if !objStillValid(s, target, tx, tz, tlevel) {
		n.clearInteraction()
		return
	}
	trigger := aiOpObjTriggerForOp(n.targetOp)
	if trigger == 0 {
		n.clearInteraction()
		return
	}
	sf := s.scriptProvider.GetByTrigger(trigger, n.typeId, n.aiTriggerCategory())
	if sf == nil {
		n.clearInteraction()
		return
	}
	s.runNpcScript(sf, n, target, trigger, nil, nil)
}

// fireAiApTriggerObj fires AI_APOBJ1..5 for an Obj target — approach-
// range counterpart.
func (n *Npc) fireAiApTriggerObj(s *Server, target *entitypkg.Obj) {
	tx, tz, tlevel := target.Coords()
	if !objStillValid(s, target, tx, tz, tlevel) {
		n.clearInteraction()
		return
	}
	trigger := aiApObjTriggerForOp(n.targetOp)
	if trigger == 0 {
		n.clearInteraction()
		return
	}
	sf := s.scriptProvider.GetByTrigger(trigger, n.typeId, n.aiTriggerCategory())
	if sf == nil {
		n.clearInteraction()
		return
	}
	s.runNpcScript(sf, n, target, trigger, nil, nil)
}

// fireAiOpTriggerLoc fires AI_OPLOC1..5 for a Loc target. Lifecycle
// gate via locStillValid (zone-membership + type match). The trigger is
// keyed on the acting npc's type+category (TS Npc.ts:992), not the loc's.
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
	sf := s.scriptProvider.GetByTrigger(trigger, n.typeId, n.aiTriggerCategory())
	if sf == nil {
		n.clearInteraction()
		return
	}
	s.runNpcScript(sf, n, target, trigger, nil, nil)
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
	sf := s.scriptProvider.GetByTrigger(trigger, n.typeId, n.aiTriggerCategory())
	if sf == nil {
		n.clearInteraction()
		return
	}
	s.runNpcScript(sf, n, target, trigger, nil, nil)
}

// fireAiOpTriggerNpc fires AI_OPNPC1..5 for an NPC target. Lifecycle
// gate is target.dead (the "isActive" half handled by validateTarget).
// The trigger is keyed on the acting npc's type+category (TS Npc.ts:992),
// not the target npc's.
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
	sf := s.scriptProvider.GetByTrigger(trigger, n.typeId, n.aiTriggerCategory())
	if sf == nil {
		n.clearInteraction()
		return
	}
	s.runNpcScript(sf, n, target, trigger, nil, nil)
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
	sf := s.scriptProvider.GetByTrigger(trigger, n.typeId, n.aiTriggerCategory())
	if sf == nil {
		n.clearInteraction()
		return
	}
	s.runNpcScript(sf, n, target, trigger, nil, nil)
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
	sf := s.scriptProvider.GetByTrigger(trigger, n.typeId, n.aiTriggerCategory())
	if sf == nil {
		n.clearInteraction()
		return
	}
	s.runNpcScript(sf, n, target, trigger, nil, nil)
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
	sf := s.scriptProvider.GetByTrigger(trigger, n.typeId, n.aiTriggerCategory())
	if sf == nil {
		n.clearInteraction()
		return
	}
	s.runNpcScript(sf, n, target, trigger, nil, nil)
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
