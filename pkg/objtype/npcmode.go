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
//
// NAI-201-D-NPCMODE-QUEUE-TODO — FORMAL CLOSURE (Arc 23 #176 pattern,
// "TS doesn't either, so we don't"). Investigated this slice and
// confirmed:
//   - TS NpcMode.ts:147-167 last touched 2025-04-09 (commit 626736a9);
//     the QUEUE1..20 NpcModeMap entries are commented out with the
//     literal comment "// TODO: these are not used?". TS has not shipped
//     impl since.
//   - Go DOES ship the underlying QUEUE machinery: NPCModeQueue1..20
//     constants (npctype.go:111-130), TriggerAiQueue1..20 dispatch via
//     Server.consumeHuntTarget (modules/world/npc_hunt.go:334-344
//     mirrors TS Npc.ts:896-903), and `findnewmode=queueN` is accepted
//     by the .hunt config parser (pkg/pack/hunt.go:174-212). The only
//     thing OMITTED is the string→mode entry in this map (which
//     pkg/pack/compiler's symbol resolver would otherwise expose to
//     bytecode), matching TS's commented-out posture exactly.
//   - checkNpcMode (pkg/script/handlers_npc.go:77-82) rejects QUEUE
//     modes from NPC_SETMODE, matching TS NpcModeValid range NULL..APNPC5.
//
// Conclusion: deviation is a TS-parity exception, not a missing impl.
// Pinned by TestNpcModeMap_QueueEntriesOmitted in npcmode_test.go and
// TestNAI201Deviations_Pinned in pkg/script/. Do NOT re-investigate
// unless TS uncomments the QUEUE entries in NpcModeMap upstream.
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
