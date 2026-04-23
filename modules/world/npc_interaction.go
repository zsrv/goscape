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
