package objtype

import (
	"reflect"
	"strconv"
	"testing"
)

// TestNpcModeMap_Parity pins spec §7.2: NpcModeMap mirrors TS
// src/engine/entity/NpcMode.ts:99-146 verbatim (48 active entries —
// NULL through APNPC5). Queue entries are absent (see deviation
// NAI-201-D-NPCMODE-QUEUE-TODO; tested separately below).
func TestNpcModeMap_Parity(t *testing.T) {
	expected := map[string]int{
		"NULL":            NPCModeNull,
		"NONE":            NPCModeNone,
		"WANDER":          NPCModeWander,
		"PATROL":          NPCModePatrol,
		"PLAYERESCAPE":    NPCModePlayerEscape,
		"PLAYERFOLLOW":    NPCModePlayerFollow,
		"PLAYERFACE":      NPCModePlayerFace,
		"PLAYERFACECLOSE": NPCModePlayerFaceClose,
		"OPPLAYER1":       NPCModeOpPlayer1,
		"OPPLAYER2":       NPCModeOpPlayer2,
		"OPPLAYER3":       NPCModeOpPlayer3,
		"OPPLAYER4":       NPCModeOpPlayer4,
		"OPPLAYER5":       NPCModeOpPlayer5,
		"APPLAYER1":       NPCModeApPlayer1,
		"APPLAYER2":       NPCModeApPlayer2,
		"APPLAYER3":       NPCModeApPlayer3,
		"APPLAYER4":       NPCModeApPlayer4,
		"APPLAYER5":       NPCModeApPlayer5,
		"OPLOC1":          NPCModeOpLoc1,
		"OPLOC2":          NPCModeOpLoc2,
		"OPLOC3":          NPCModeOpLoc3,
		"OPLOC4":          NPCModeOpLoc4,
		"OPLOC5":          NPCModeOpLoc5,
		"APLOC1":          NPCModeApLoc1,
		"APLOC2":          NPCModeApLoc2,
		"APLOC3":          NPCModeApLoc3,
		"APLOC4":          NPCModeApLoc4,
		"APLOC5":          NPCModeApLoc5,
		"OPOBJ1":          NPCModeOpObj1,
		"OPOBJ2":          NPCModeOpObj2,
		"OPOBJ3":          NPCModeOpObj3,
		"OPOBJ4":          NPCModeOpObj4,
		"OPOBJ5":          NPCModeOpObj5,
		"APOBJ1":          NPCModeApObj1,
		"APOBJ2":          NPCModeApObj2,
		"APOBJ3":          NPCModeApObj3,
		"APOBJ4":          NPCModeApObj4,
		"APOBJ5":          NPCModeApObj5,
		"OPNPC1":          NPCModeOpNpc1,
		"OPNPC2":          NPCModeOpNpc2,
		"OPNPC3":          NPCModeOpNpc3,
		"OPNPC4":          NPCModeOpNpc4,
		"OPNPC5":          NPCModeOpNpc5,
		"APNPC1":          NPCModeApNpc1,
		"APNPC2":          NPCModeApNpc2,
		"APNPC3":          NPCModeApNpc3,
		"APNPC4":          NPCModeApNpc4,
		"APNPC5":          NPCModeApNpc5,
	}
	if len(expected) != 48 {
		t.Fatalf("expected has %d entries; fixture broken (want 48)", len(expected))
	}
	if !reflect.DeepEqual(NpcModeMap, expected) {
		t.Fatalf("NpcModeMap mismatch\n got = %#v\nwant = %#v", NpcModeMap, expected)
	}
}

// TestNpcModeMap_QueueEntriesOmitted pins deviation NAI-201-D-NPCMODE-QUEUE-TODO:
// TS NpcMode.ts:147-167 has 20 QUEUE1..QUEUE20 entries commented out
// with "// TODO: these are not used?". Goscape's NpcModeMap omits them.
// The NPCMode* constants themselves exist (see npctype.go) for the NPC
// AI state machine; they just have no name-string mapping here.
func TestNpcModeMap_QueueEntriesOmitted(t *testing.T) {
	for i := 1; i <= 20; i++ {
		key := "QUEUE" + strconv.Itoa(i)
		if _, present := NpcModeMap[key]; present {
			t.Errorf("NpcModeMap[%q]: present, want absent (NAI-201-D-NPCMODE-QUEUE-TODO)", key)
		}
	}
}
