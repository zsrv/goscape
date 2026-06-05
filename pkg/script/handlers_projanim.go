package script

import "fmt"

// handleProjAnimMap (PROJANIM_MAP, opcode 1018) queues a tile→tile
// projectile event broadcast to all players in the source zone.
// Mirrors TS Engine-TS/src/engine/script/handlers/ServerOps.ts:202-210.
//
// TS validation order is spotanim → srcCoord → dstCoord (different
// from PROJANIM_NPC/PL which validate srcCoord first). Pinned by
// TestProjAnimMap_ValidationOrder.
func handleProjAnimMap(s *ScriptState) error {
	arc := s.PopInt()
	peak := s.PopInt()
	duration := s.PopInt()
	delay := s.PopInt()
	dstHeight := s.PopInt()
	srcHeight := s.PopInt()
	spotanim := s.PopInt()
	dstCoord := s.PopInt()
	srcCoord := s.PopInt()

	if err := checkSpotAnimType(s, spotanim, "PROJANIM_MAP"); err != nil {
		return err
	}
	srcLevel, srcX, srcZ, err := checkCoord(srcCoord, "PROJANIM_MAP")
	if err != nil {
		return err
	}
	_, dstX, dstZ, err := checkCoord(dstCoord, "PROJANIM_MAP")
	if err != nil {
		return err
	}

	s.World.MapProjAnim(srcLevel, srcX, srcZ, dstX, dstZ, 0,
		spotanim, srcHeight*4, dstHeight*4, delay, duration, peak, arc)
	return nil
}

// handleProjAnimNpc (PROJANIM_NPC, opcode 1019) queues a tile→NPC
// projectile event with the NPC encoded as receiver via npc.Nid()+1.
// Slot-only NPC lookup — does NOT verify the high-16 expectedType
// bits (mirrors TS comment-out at ServerOps.ts:192). Mirrors TS
// ServerOps.ts:185-200.
func handleProjAnimNpc(s *ScriptState) error {
	arc := s.PopInt()
	peak := s.PopInt()
	duration := s.PopInt()
	delay := s.PopInt()
	dstHeight := s.PopInt()
	srcHeight := s.PopInt()
	spotanim := s.PopInt()
	npcUid := s.PopInt()
	srcCoord := s.PopInt()

	srcLevel, srcX, srcZ, err := checkCoord(srcCoord, "PROJANIM_NPC")
	if err != nil {
		return err
	}
	if err := checkSpotAnimType(s, spotanim, "PROJANIM_NPC"); err != nil {
		return err
	}

	slot := npcUid & 0xffff
	npc := s.World.LookupNpcBySlot(slot)
	if npc == nil {
		return fmt.Errorf("PROJANIM_NPC: invalid npc uid: %d", npcUid)
	}

	s.World.MapProjAnim(srcLevel, srcX, srcZ, npc.NpcX(), npc.NpcZ(),
		npc.Nid()+1, spotanim, srcHeight*4, dstHeight*4,
		delay, duration, peak, arc)
	return nil
}

// handleProjAnimPl (PROJANIM_PL, opcode 1020) queues a tile→player
// projectile event with the player encoded as receiver via
// -player.Slot()-1. Mirrors TS ServerOps.ts:171-183.
func handleProjAnimPl(s *ScriptState) error {
	arc := s.PopInt()
	peak := s.PopInt()
	duration := s.PopInt()
	delay := s.PopInt()
	dstHeight := s.PopInt()
	srcHeight := s.PopInt()
	spotanim := s.PopInt()
	uid := s.PopInt()
	srcCoord := s.PopInt()

	srcLevel, srcX, srcZ, err := checkCoord(srcCoord, "PROJANIM_PL")
	if err != nil {
		return err
	}
	if err := checkSpotAnimType(s, spotanim, "PROJANIM_PL"); err != nil {
		return err
	}

	pl := s.World.LookupPlayerByUID(uid)
	if pl == nil {
		return fmt.Errorf("PROJANIM_PL: invalid player uid: %d", uid)
	}

	s.World.MapProjAnim(srcLevel, srcX, srcZ, pl.X(), pl.Z(),
		-pl.Slot()-1, spotanim, srcHeight*4, dstHeight*4,
		delay, duration, peak, arc)
	return nil
}
