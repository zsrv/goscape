package objtype

// NpcModeMap maps uppercase NPC-mode name → mode index. Mirrors TS
// NpcModeMap (src/engine/entity/NpcMode.ts:98-146 active entries; lines
// 147-167 are commented-out TODOs that are NOT ported per
// NAI-201-D-NPCMODE-QUEUE-TODO).
//
// 48 entries: NULL plus the 47 explicit names through APNPC5. Consumed
// by the bytecode compiler's LoadMap call in pkg/pack/compiler (NAI-202
// runServerCompiler).
//
// NPCMode* constants are declared in npctype.go (pre-existing; not
// redeclared here).
var NpcModeMap = map[string]int{
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
