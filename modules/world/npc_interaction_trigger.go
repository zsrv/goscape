package world

import (
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/script"
)

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
