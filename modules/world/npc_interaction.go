package world

import (
	"github.com/zsrv/goscape/pkg/coordgrid"
	entitypkg "github.com/zsrv/goscape/pkg/entity"
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

// SetInteraction anchors the NPC's interaction on target. Mirrors TS
// PathingEntity.setInteraction at Engine-TS/.../PathingEntity.ts:510-548.
// Closes the seven NAI-10 deferred setInteraction fields:
//  1. apRange = 10
//  2. apRangeCalled = false
//  3. targetSubject.com/typ snapshot
//  4. focus() → faceAngleX/Z
//  5. faceEntity + masks|=entitymask (Player/Npc targets)
//  6. targetX/targetZ (Loc/Obj targets)
//  7. target.IsValid() pre-check
//
// TS quirk preserved: `com ? com : -1` coerces 0 → -1 on subject.com.
//
// DEVIATION: n.entitymask is currently always 0 (the mask-plumbing
// sub-spec will wire it), so `n.masks |= n.entitymask` is a harmless
// no-op. The statement is kept for structural parity with TS so the
// mask-plumbing port is a one-line change rather than a body rewrite.
func (n *Npc) SetInteraction(kind InteractionKind, target entity, op, com int) bool {
	if !target.IsValid() {
		return false
	}

	n.target = target
	n.targetOp = op
	n.apRange = 10
	n.apRangeCalled = false

	// TS "com ? com : -1": 0 coerces to -1.
	if com == 0 {
		n.targetSubject.com = -1
	} else {
		n.targetSubject.com = com
	}

	// targetSubject.typ snapshot for changetype-detection in validateTarget.
	switch t := target.(type) {
	case *Npc:
		n.targetSubject.typ = t.typeId
	case *entitypkg.Loc:
		n.targetSubject.typ = t.Type()
	case *entitypkg.Obj:
		n.targetSubject.typ = t.Type
	default:
		n.targetSubject.typ = -1
	}

	// focus — fine-grained face-angle coord. Non-pathing targets
	// (Loc/Obj) use the engine-face path when the kind is engine;
	// pathing targets (Player/Npc) never set instant.
	tx, tz, _ := target.Coords()
	tw, tl := targetWidthLength(target)
	fx := coordgrid.Fine(tx, tw)
	fz := coordgrid.Fine(tz, tl)
	isNonPathing := false
	switch target.(type) {
	case *entitypkg.Loc, *entitypkg.Obj:
		isNonPathing = true
	}
	n.focus(fx, fz, isNonPathing && kind == InteractionEngine)

	// faceEntity (Player/Npc) or targetX/Z (Loc/Obj) dispatch.
	switch t := target.(type) {
	case *Player:
		slot := t.slot + 32768
		if n.faceEntity != slot {
			n.faceEntity = slot
			n.masks |= n.entitymask
		}
	case *Npc:
		if n.faceEntity != t.nid {
			n.faceEntity = t.nid
			n.masks |= n.entitymask
		}
	default:
		n.targetX = fx
		n.targetZ = fz
	}

	return true
}

// targetWidthLength returns the target's (width, length) for fine-grained
// coord math. 1x1 for PathingEntity; real dimensions for Loc; 1x1 for Obj.
func targetWidthLength(target entity) (width, length int) {
	if l, ok := target.(*entitypkg.Loc); ok {
		return l.Width, l.Length
	}
	return 1, 1
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
