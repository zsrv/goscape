package world

import (
	"github.com/zsrv/goscape/pkg/objtype"
)

// checkOpTrigger reports whether targetOp falls in any OP-trigger band
// (OPPLAYER / OPLOC / OPOBJ / OPNPC — 5 values each, four bands).
// Matches TS Npc.checkOpTrigger at Engine-TS/src/engine/entity/Npc.ts:1073-1080.
func checkOpTrigger(op int) bool {
	return (op >= objtype.NPCModeOpPlayer1 && op <= objtype.NPCModeOpPlayer5) ||
		(op >= objtype.NPCModeOpLoc1 && op <= objtype.NPCModeOpLoc5) ||
		(op >= objtype.NPCModeOpObj1 && op <= objtype.NPCModeOpObj5) ||
		(op >= objtype.NPCModeOpNpc1 && op <= objtype.NPCModeOpNpc5)
}

// checkApTrigger reports whether targetOp falls in any AP-trigger band
// (APPLAYER / APLOC / APOBJ / APNPC — 5 values each, four bands).
// Matches TS Npc.checkApTrigger at Engine-TS/src/engine/entity/Npc.ts:1064-1071.
func checkApTrigger(op int) bool {
	return (op >= objtype.NPCModeApPlayer1 && op <= objtype.NPCModeApPlayer5) ||
		(op >= objtype.NPCModeApLoc1 && op <= objtype.NPCModeApLoc5) ||
		(op >= objtype.NPCModeApObj1 && op <= objtype.NPCModeApObj5) ||
		(op >= objtype.NPCModeApNpc1 && op <= objtype.NPCModeApNpc5)
}

// resetDefaults clears target/targetOp to defaultMode baseline. Matches
// TS Npc.resetDefaults — INTENTIONALLY does NOT clear apRange,
// apRangeCalled, faceEntity, or masks. Those are overwritten only by
// the next SetInteraction call.
func (n *Npc) resetDefaults() {
	n.target = nil
	n.targetOp = n.defaultMode()
}

// clearInteraction resets interaction state to idle, including apRange
// fields. Matches TS PathingEntity.clearInteraction. Does NOT touch
// faceEntity/masks — those are cleared by the masks frame-pass, not here.
func (n *Npc) clearInteraction() {
	n.target = nil
	n.targetOp = -1
	n.apRange = 10
	n.apRangeCalled = false
	n.targetSubject = npcTargetSubject{com: -1, typ: -1}
}

// focus records the fine-grained face-angle target. Called from
// SetInteraction with CoordGrid.fine of the target's width/length.
// Matches TS PathingEntity.focus.
//
// DEVIATION: TS takes an `instant` flag distinguishing engine-face
// from script-face, which selects between two wire-protocol paths.
// Go's current protocol doesn't branch on it, so the flag is accepted
// for signature parity but currently stored write-only. Follow-up:
// "face-instant wire protocol" sub-spec.
func (n *Npc) focus(fx, fz int, instant bool) {
	n.faceAngleX = fx
	n.faceAngleZ = fz
	_ = instant
}

// defaultMode returns the NPC's baseline mode based on its NpcType
// config. Patrol if PatrolCoord is set; else Wander if WanderRange>0;
// else None. Single source of truth used by NewNpc (initial targetOp)
// and resetDefaults (revert targetOp). Matches TS NpcType.defaultmode.
func (n *Npc) defaultMode() int {
	if n.typ == nil {
		return objtype.NPCModeNone
	}
	if len(n.typ.PatrolCoord) > 0 {
		return objtype.NPCModePatrol
	}
	if n.typ.WanderRange > 0 {
		return objtype.NPCModeWander
	}
	return objtype.NPCModeNone
}
