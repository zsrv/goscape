package objtype

// NpcStatMap maps uppercase NPC-stat name → stat index. Mirrors TS
// NpcStatMap (NpcStat.ts:10-17). Consumed by the bytecode compiler's
// LoadMap call in pkg/pack/compiler (NAI-202 runServerCompiler) and by
// any future ::npc cheat handlers that parse stat names.
//
// NpcStat* constants are declared in npctype.go (pre-existing; not
// redeclared here).
var NpcStatMap = map[string]int{
	"ATTACK":    NpcStatAttack,
	"DEFENCE":   NpcStatDefence,
	"STRENGTH":  NpcStatStrength,
	"HITPOINTS": NpcStatHitpoints,
	"RANGED":    NpcStatRanged,
	"MAGIC":     NpcStatMagic,
}
