package world

// setFaceEntity derives the wire faceEntity value from the CURRENT target
// every time it is called, emitting the entitymask bit only on change.
// Port of TS PathingEntity.setFaceEntity (Engine-TS
// src/engine/entity/PathingEntity.ts:506-524 @2e3bcf43, extracted by
// ee28c1aa "improve Npc and Player facing to match RS2 behavior (#82)"):
//
//	setFaceEntity(): void {
//	    const oldEntity = this.faceEntity;
//	    if (this.target instanceof Player) {
//	        const playerSlot: number = this.target.slot + 32768;
//	        if (this.faceEntity !== playerSlot) {
//	            this.faceEntity = playerSlot;
//	        }
//	    } else if (this.target instanceof Npc) {
//	        const nid: number = this.target.nid;
//	        if (this.faceEntity !== nid) {
//	            this.faceEntity = nid;
//	        }
//	    } else {
//	        this.faceEntity = -1;
//	    }
//	    if (this.faceEntity !== oldEntity) {
//	        this.masks |= this.entitymask;
//	    }
//	}
//
// ee28c1aa centralised ALL faceEntity writes into this method and removed
// the per-site manual `faceEntity = -1; masks |= entitymask` writes
// (setInteraction's Player/Npc arms, Npc.clearInteraction/resetDefaults,
// OpHeld/OpHeldT/OpHeldU handlers, and the World.ts post-decode reset
// block). Call sites at the pin:
//   - World.ts:708 — per-player, after processEngineQueue, before
//     processInteraction (goscape: tick.go processPlayerFacing pass).
//   - Npc.ts:184 — per-NPC turn, after processMovementInteraction
//     (goscape: npc_ai.go turn()).
//   - PathingEntity.ts:626 — tail of resetPathingEntity (goscape:
//     ResetMasks tail in player_masks.go / npc_masks.go).
//
// Note the else branch: a Loc/Obj target now CLEARS faceEntity (-1) —
// pre-ee28c1aa the reset only fired for nil targets at tick end (the
// Loc/Obj clear lived in the World.ts post-decode block instead).
//
// Arc-30 lesson #202 check: the wire FACE_ENTITY payload is serialized
// from the renderer accessors FaceEntity() (player_source.go:21,
// npc_source.go:19) and the direct tick.go ComputePlayers/ComputeNpcs
// reads (tick.go:1076/:1123) — all read the same p.faceEntity/n.faceEntity
// field written here. There is no parallel compute path for FACE_ENTITY
// (unlike FACE_COORD's effectiveFaceCoord).
//
// Go fork note (lesson "TS-unified-handler / Go-fork drift"): TS has ONE
// method on PathingEntity; goscape's Player/Npc forks below must stay
// byte-identical in shape — parameterized only by the receiver's
// faceEntity/target/masks/entitymask fields.
func (p *Player) setFaceEntity() {
	oldEntity := p.faceEntity
	switch t := p.target.(type) {
	case *Player:
		p.faceEntity = t.slot + 32768
	case *Npc:
		p.faceEntity = t.nid
	default: // nil, *Loc, *Obj
		p.faceEntity = -1
	}
	if p.faceEntity != oldEntity {
		p.masks |= p.entitymask
	}
}

// setFaceEntity is the Npc fork of (*Player).setFaceEntity — see the
// doc-comment above for the TS source quote and call-site map.
func (n *Npc) setFaceEntity() {
	oldEntity := n.faceEntity
	switch t := n.target.(type) {
	case *Player:
		n.faceEntity = t.slot + 32768
	case *Npc:
		n.faceEntity = t.nid
	default: // nil, *Loc, *Obj
		n.faceEntity = -1
	}
	if n.faceEntity != oldEntity {
		n.masks |= n.entitymask
	}
}
