package world

import (
	"errors"

	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/rsbuf"
	"github.com/zsrv/goscape/pkg/script"
)

var errNpcsFull = errors.New("npc registry full")

// spawnBootNpc is the single-spawn step for the boot-time NPC pass. It
// mirrors TS GameMap.loadNpcs at the 254 pin (GameMap.ts:132-136
// @2e3bcf43), where Npc construction (consuming World.getNextNid()) sits
// INSIDE the members gate — a non-members world consumes NO nid for a
// skipped members-only NPC, so F2P nid sequences stay compact. (The
// 244-era hoist that burned a nid per skipped NPC is gone.)
//
//	if ((npcType.members && this.members) || !npcType.members) {
//	    const size: number = npcType.size;
//	    const npc: Npc = new Npc(..., World.getNextNid(), npcType.id, npcType.blockwalk);
//	    World.addNpc(npc, -1);
//	}
//
// Contract:
//   - typ nil-check must be done by the caller BEFORE calling (TS
//     printFatalError+continue at :128-131 fires before the gate).
//   - Returns (nil, nil) when the members gate rejects the NPC (no nid
//     consumed); callers should continue.
//   - Returns (nil, errNpcsFull) when the registry is full; callers must
//     break the spawn loop on a non-nil error.
//   - Returns (n, nil) on success: NPC is registered and ready.
//
// [gamemap-2]
func (s *Server) spawnBootNpc(typ *objtype.NpcType, typeID, x, z, level int, worldMembers bool) (*Npc, error) {
	// 254 pin: members gate BEFORE nid allocation (TS GameMap.ts:132
	// @2e3bcf43 moved the Npc ctor inside the gate).
	if !shouldSpawnNpc(typ, worldMembers) {
		return nil, nil
	}
	nid := s.allocNpcSlot()
	if nid < 0 {
		return nil, errNpcsFull
	}
	n := NewNpc(nid, typeID, x, z, level, typ)
	// NewNpc(nid, ...) already set n.nid and n.uid correctly — unlike the
	// addNpc production callers which construct NewNpc(0, ...) and rely on
	// addNpc to overwrite nid/uid after slot allocation (NAI-150). No
	// overwrite needed here.
	n.server = s
	s.npcs[nid] = n
	s.npcLoop = append(s.npcLoop, n)
	if s.rsbuf != nil {
		s.rsbuf.AddNpc(int32(n.nid), int32(n.typeId))
	}
	// Delegate post-registration setup (position restore, zone enter,
	// collision, resetEntityForRespawn, AI_SPAWN trigger) to addNpc with
	// firstSpawn=false — which applies all shared tail logic without
	// re-allocating a slot.
	_ = s.addNpc(n, -1, false)
	return n, nil
}

// allocNpcSlot returns a free nid (1..16383). Returns -1 if full.
func (s *Server) allocNpcSlot() int {
	for offset := range len(s.npcs) - 1 {
		i := max(s.nextNpcSlot+offset, 1)
		if i >= len(s.npcs) {
			i = (i % (len(s.npcs) - 1)) + 1
		}
		if s.npcs[i] == nil {
			s.nextNpcSlot = max((i+1)%len(s.npcs), 1)
			return i
		}
	}
	return -1
}

// addNpc registers n in the world. Mirrors TS World.addNpc at
// World.ts:1258-1294.
//
// firstSpawn=true: allocate a slot, register in s.npcs + s.npcLoop
// (the original goscape behavior). firstSpawn=false: skip those —
// used by revertType's respawn cycle (NPC keeps its slot).
//
// Always: tele to (startX, startZ), clear dead flag, toggle collision
// flags on per n.typ.BlockWalk, run resetEntityForRespawn (stats
// reseed + waypoint/queue clear + tele/mask flag), reset animation,
// and (if duration > -1) write n.lifecycleTick.
//
// Caller responsible for synchronisation (called during NewServer or
// under playersMu).
func (s *Server) addNpc(n *Npc, duration int, firstSpawn bool) error {
	if firstSpawn {
		nid := s.allocNpcSlot()
		if nid < 0 {
			return errNpcsFull
		}
		n.nid = nid
		// NAI-150 stretch: refresh uid against the freshly-allocated nid.
		// Production spawn site (server.go:312) constructs NewNpc(0, ...);
		// without this line every spawned NPC retains slot=0 in uid,
		// breaking PROJANIM_NPC and FindNpcByUID for the entire NPC set.
		n.uid = (n.typeId << 16) | n.nid
		n.server = s
		s.npcs[nid] = n
		s.npcLoop = append(s.npcLoop, n)
		// TS World.addNpc at World.ts:1259-1262 calls rsbuf.addNpc
		// ONLY inside the firstSpawn branch (paired with this.npcs.set).
		// A RESPAWN NPC keeps its rsbuf registration across death/respawn
		// — the despawn-only rsbuf.removeNpc symmetry is preserved in
		// removeNpc below. Pre-fix goscape called AddNpc on every addNpc
		// call (including revertType respawns), re-registering an already-
		// registered slot per tick. Closes world-ops-3 (2026-05-28
		// fresh-audit MED) — paired with the removeNpc DESPAWN-gating
		// edit below for the symmetric lifecycle contract.
		if s.rsbuf != nil {
			s.rsbuf.AddNpc(int32(n.nid), int32(n.typeId))
		}
	}
	n.x = n.startX
	n.z = n.startZ
	n.dead = false
	// Zone enter — mirrors TS World.addNpc at World.ts:1268-1269.
	if s.zoneMap != nil {
		z := s.zoneMap.Get(n.level, n.x, n.z)
		n.zoneListElement = z.EnterNpc(n)
	}
	if s.gamemap != nil {
		switch n.blockWalk {
		case objtype.BlockWalkNPC:
			s.gamemap.ChangeNPCCollision(n.size, n.x, n.z, n.level, true)
		case objtype.BlockWalkAll:
			s.gamemap.ChangeNPCCollision(n.size, n.x, n.z, n.level, true)
			s.gamemap.ChangePlayerCollision(n.size, n.x, n.z, n.level, true)
		}
	}
	s.resetEntityForRespawn(n)
	n.animID = -1
	n.animDelay = 0
	// AI_SPAWN trigger queue (matches TS World.ts:1284-1289). Fires
	// unconditionally — for both firstSpawn=true (server boot) and
	// firstSpawn=false (revertType respawn). NPCs without a registered
	// AI_SPAWN script never enter the queue (the script != nil guard).
	// processNpcEventQueue dispatches early in the tick — before
	// processInteractions — per NAI-122 (TS World.ts:356 vs 376).
	// Mirrors the existing AI_DESPAWN producer pattern at npc_ai.go:47-58.
	if s.scriptProvider != nil && n.typ != nil {
		sf := s.scriptProvider.GetByTrigger(
			script.TriggerAiSpawn, n.typeId, n.typ.Category)
		if sf != nil {
			s.npcEventQueue = append(s.npcEventQueue,
				NpcEventRequest{
					Type:   NpcEventSpawn,
					Script: sf,
					Npc:    n,
				})
		}
	}
	if duration > -1 {
		n.lifecycleTick = duration
	}
	return nil
}

// resetEntityForRespawn applies the TS Npc.resetEntity(true) reseed
// (TS Npc.ts:280-317, respawn=true branch) factored out so addNpc and
// revertType (NAI-19 Task 5e) share one definition.
//
// Resets typeId/uid to baseType (with fresh n.typ lookup), reseeds
// all 6 stats from n.typ.Stats, clears heroPoints (TS Npc.ts:292) +
// queue/waypoints, sets tele + CHANGE_TYPE mask, resets hunt fields.
// Does NOT touch n.x/n.z (the caller handles position) or collision
// flags (the caller handles those via gamemap).
func (s *Server) resetEntityForRespawn(n *Npc) {
	// TS Npc.resetEntity(true) at Npc.ts:284 — restore default-south
	// face-angle. Reads n.x, n.z, n.size; none are mutated by the
	// rest of this function so the call order is safe at the top.
	n.unfocus()

	if n.typeId != n.baseType {
		n.typeId = n.baseType
		n.uid = (n.typeId << 16) | n.nid
		if newTyp := n.lookupType(n.baseType); newTyp != nil {
			n.typ = newTyp
		}
		n.masks |= rsbuf.NpcMaskChangeType
	}
	if n.typ != nil {
		for i := range min(objtype.NpcStatCount, len(n.typ.Stats)) {
			v := int(n.typ.Stats[i])
			n.levels[i] = v
			n.baseLevels[i] = v
		}
	}
	// TS Npc.ts:292 — clear heroPoints contributor ledger on respawn.
	// The Npc struct is reused across respawn cycles; old contributors
	// would otherwise linger into the next life. NAI-120 Bundle 2D
	// follow-up.
	n.heroPoints.Clear()
	n.queue = nil
	n.waypointIndex = -1
	n.tele = true
	n.huntClock = 0
	n.huntTarget = nil
	if n.typ != nil {
		n.huntRange = int(n.typ.HuntRange)
		n.huntMode = n.typ.HuntMode
	}

	// TS Npc.resetEntity(true) varn re-seed loop (Npc.ts:296-306).
	// Per-type defaults: STRING → "" (TS uses undefined; goscape uses
	// zero-value string per DEVIATION-NAI-121-D2); INT → 0; everything
	// else → -1. Defensive (DEVIATION-NAI-121-D3): if s.varnTypes is nil
	// (test path) the loop is a no-op and reads fall back to slice
	// defaults. (goscape defensive; TS skips this check.)
	if s.varnTypes != nil {
		if len(n.varns) != len(s.varnTypes.Configs) {
			n.varns = make([]int32, len(s.varnTypes.Configs))
			n.varnsString = make([]string, len(s.varnTypes.Configs))
		}
		for i, vt := range s.varnTypes.Configs {
			switch vt.Type {
			case objtype.ScriptVarTypeString:
				n.varnsString[i] = ""
			case objtype.ScriptVarTypeInt:
				n.varns[i] = 0
			default:
				n.varns[i] = -1
			}
		}
	}

	// TS Npc.resetEntity(true) at Npc.ts:309 @2e3bcf43 calls resetDefaults()
	// after the varsString fill. TS resetDefaults (Npc.ts:412-422 @2e3bcf43)
	// clears interaction state and re-seeds targetOp/timerInterval from the
	// type (the faceEntity/mask tail was removed by ee28c1aa — facing is
	// derived per-turn by setFaceEntity()). goscape's (n *Npc).resetDefaults()
	// is the NAI-11-stripped subset (target/targetOp only); the
	// apRange/apRangeCalled/targetSubject/timerInterval resets the stripped
	// subset omits are re-applied inline here so the respawn surface reaches
	// full TS-fidelity. 2026-05-28 fresh-audit row npc-core-1.
	n.resetDefaults()
	n.apRange = 10
	n.apRangeCalled = false
	n.targetSubject = npcTargetSubject{com: -1, typ: -1}
	if n.typ != nil {
		n.timerInterval = int(n.typ.Timer)
	}
}

// removeNpc marks n as logically absent from the world. Mirrors TS
// World.removeNpc at World.ts:1296-1319.
//
// Per TS: scales `duration` by player count, runs zone.leave (now wired),
// flips isActive=false (n.dead=true in goscape), toggles
// collision flags off per n.typ.BlockWalk, and branches on lifecycle:
//   - DESPAWN: TS World.ts:1312-1315 — rsbuf.RemoveNpc, release the
//     registry slot, run n.Cleanup(). The s.npcLoop splice is deferred
//     to end-of-tick compactNpcLoop per NAI-19-D-DEFERRED-COMPACT-VS-
//     IMMEDIATE-SPLICE (mid-iteration splice unsafe on append-only slice).
//   - RESPAWN+duration>-1: writes n.lifecycleTick = scaledDuration.
func (s *Server) removeNpc(n *Npc, duration int) {
	// Zone leave — mirrors TS World.removeNpc at World.ts:1297-1299.
	if s.zoneMap != nil && n.zoneListElement != nil {
		z := s.zoneMap.Get(n.level, n.x, n.z)
		z.LeaveNpc(n, n.zoneListElement)
		n.zoneListElement = nil
	}
	n.dead = true
	if s.gamemap != nil {
		switch n.blockWalk {
		case objtype.BlockWalkNPC:
			s.gamemap.ChangeNPCCollision(n.size, n.x, n.z, n.level, false)
		case objtype.BlockWalkAll:
			s.gamemap.ChangeNPCCollision(n.size, n.x, n.z, n.level, false)
			s.gamemap.ChangePlayerCollision(n.size, n.x, n.z, n.level, false)
		}
	}
	if n.lifecycle == NpcLifecycleDespawn {
		// NAI-19: TS World.ts:1312-1315 — rsbuf.removeNpc fires ONLY in
		// the DESPAWN branch (paired with this.npcs.remove +
		// npc.cleanup()). A RESPAWN NPC keeps its rsbuf registration
		// across death/respawn; pre-fix goscape called RemoveNpc
		// unconditionally before this branch, so a respawning NPC was
		// unregistered then re-registered every cycle. world-ops-3
		// (2026-05-28 fresh-audit MED) — paired with the addNpc
		// firstSpawn-gating edit above. Release the registry slot and
		// run Cleanup. The s.npcLoop splice is deferred to
		// compactNpcLoop (end-of-tick) per
		// NAI-19-D-DEFERRED-COMPACT-VS-IMMEDIATE-SPLICE to keep
		// mid-tick iteration safe. Order matters: nil the slot BEFORE
		// Cleanup, because Cleanup sets n.nid = -1.
		if s.rsbuf != nil {
			s.rsbuf.RemoveNpc(int32(n.nid))
		}
		s.npcs[n.nid] = nil
		n.Cleanup()
	} else if n.lifecycle == NpcLifecycleRespawn && duration > -1 {
		n.lifecycleTick = s.scaleByPlayerCount(duration)
	}
}
