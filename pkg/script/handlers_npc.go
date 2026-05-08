package script

import (
	"errors"
	"fmt"

	"github.com/zsrv/goscape/pkg/objtype"
)

// checkCoord mirrors TS CoordValid (ScriptValidators.ts:109) — validates
// the packed int is in [0, 2147483647] and unpacks to (level, x, z).
// Uses the package-local unpackCoord helper at handlers_player.go:18.
func checkCoord(v int, op string) (level, x, z int, err error) {
	if v < 0 || v > 2147483647 {
		return 0, 0, 0, fmt.Errorf("%s: coord out of range (%d)", op, v)
	}
	level, x, z = unpackCoord(v)
	return
}

// checkNpcStatID validates a stat id against objtype.NpcStatCount. Mirrors
// TS NpcStatValid (ScriptValidators.ts) — range [0, NpcStatCount). NAI-120
// Bundle 2C.
func checkNpcStatID(id int, op string) error {
	if id < 0 || id >= objtype.NpcStatCount {
		return fmt.Errorf("%s: npc stat id out of range (%d)", op, id)
	}
	return nil
}

// checkNpcMode validates an NPC mode value against the full NPCMode* enum
// at pkg/objtype/npctype.go. Accepts every declared value (Null=-1 through
// ApNpc5=46). Mirrors TS NpcModeValid (ScriptValidators.ts:116) — same
// ScriptInputRangeValidator(NULL, APNPC5) range, no enum-table dispatch.
func checkNpcMode(mode int, op string) error {
	if mode < objtype.NPCModeNull || mode > objtype.NPCModeApNpc5 {
		return fmt.Errorf("%s: invalid npc mode (%d)", op, mode)
	}
	return nil
}

// checkNpcType mirrors TS NpcTypeValid (ScriptValidators.ts:111) — range
// + registry presence check, collapsed into a single Configs.NpcType(id)
// nil check per the S7c checkInvType pattern at handlers_player.go:75.
func checkNpcType(s *ScriptState, id int, op string) error {
	if s.Configs == nil || s.Configs.NpcType(id) == nil {
		return fmt.Errorf("%s: no NpcType with value (%d) found", op, id)
	}
	return nil
}

// checkHuntVis mirrors TS HuntVisValid (ScriptValidators.ts:125) — range
// [HuntVisOff=0, HuntVisLineOfWalk=2]. Constants live in
// pkg/objtype/hunttype.go:22-26 and match TS values.
func checkHuntVis(v int, op string) error {
	if v < 0 || v > 2 {
		return fmt.Errorf("%s: huntvis out of range (%d)", op, v)
	}
	return nil
}

// checkCategoryType partially mirrors TS CategoryTypeValid
// (ScriptValidators.ts:123). Goscape has no CategoryType config loader,
// so the count-bound check is absent — only null-sentinel rejection
// survives. Deviation S7f-D3. Follow-up: count-bound check when the
// CategoryType loader lands.
func checkCategoryType(v int, op string) error {
	if v == -1 {
		return fmt.Errorf("%s: category null(-1)", op)
	}
	return nil
}

// setActiveNpcSlot writes the found NPC to either ActiveNpc (primary) or
// OtherActiveNpc (secondary) based on the handler's IntOperand and sets
// the corresponding Pointer flag. Mirrors TS
// state.pointerAdd(ActiveNpc[state.intOperand]) at NpcOps.ts:365, 398, 105.
// IntOperand==0 → ActiveNpc/PtrActiveNpc (.npc syntax).
// IntOperand==1 → OtherActiveNpc/PtrActiveNpc2 (.npc2 syntax).
// Any other value panics (compiler invariant — bytecode only emits 0/1).
func setActiveNpcSlot(s *ScriptState, npc ActiveNpc) {
	operand := s.Script.IntOperands[s.PC]
	switch operand {
	case 0:
		s.ActiveNpc = npc
		s.Pointers |= PtrActiveNpc
	case 1:
		s.OtherActiveNpc = npc
		s.Pointers |= PtrActiveNpc2
	default:
		panic(fmt.Sprintf("setActiveNpcSlot: invalid IntOperand %d", operand))
	}
}

// requireActiveNpc returns an error tagged with the opcode name if the
// script has no ActiveNpc bound. All NPC_* read handlers start with this
// check to mirror TS `checkedHandler(ActiveNpc, ...)`.
func requireActiveNpc(s *ScriptState, op string) error {
	if s.ActiveNpc == nil {
		return fmt.Errorf("%s: no active npc", op)
	}
	return nil
}

// handleNpcType pushes the ActiveNpc's NpcType id.
func handleNpcType(s *ScriptState) error {
	if err := requireActiveNpc(s, "NPC_TYPE"); err != nil {
		return err
	}
	s.PushInt(s.ActiveNpc.NpcType())
	return nil
}

// handleNpcCoord pushes the packed RS2 coord of the ActiveNpc:
// (level<<28) | (x<<14) | z.
func handleNpcCoord(s *ScriptState) error {
	if err := requireActiveNpc(s, "NPC_COORD"); err != nil {
		return err
	}
	n := s.ActiveNpc
	s.PushInt((n.NpcLevel() << 28) | (n.NpcX() << 14) | n.NpcZ())
	return nil
}

// handleNpcStat pops a stat id and pushes the NPC's current (boosted)
// level for that stat.
func handleNpcStat(s *ScriptState) error {
	if err := requireActiveNpc(s, "NPC_STAT"); err != nil {
		return err
	}
	stat := s.PopInt()
	s.PushInt(s.ActiveNpc.NpcStat(stat))
	return nil
}

// handleNpcBaseStat pops a stat id and pushes the NPC's base level.
func handleNpcBaseStat(s *ScriptState) error {
	if err := requireActiveNpc(s, "NPC_BASESTAT"); err != nil {
		return err
	}
	stat := s.PopInt()
	s.PushInt(s.ActiveNpc.NpcBaseStat(stat))
	return nil
}

// handleNpcName looks up the ActiveNpc's NpcType via Configs and pushes
// its Name, falling back to DebugName, then "null" (matching TS
// nullish-coalesce on NpcType.name).
func handleNpcName(s *ScriptState) error {
	if err := requireActiveNpc(s, "NPC_NAME"); err != nil {
		return err
	}
	if s.Configs == nil {
		return errors.New("NPC_NAME: no configs")
	}
	cfg := s.Configs.NpcType(s.ActiveNpc.NpcType())
	if cfg == nil {
		s.PushString("null")
		return nil
	}
	name := cfg.Name
	if name == "" {
		name = cfg.DebugName
	}
	if name == "" {
		name = "null"
	}
	s.PushString(name)
	return nil
}

// handleNpcHasOp pops a 1-indexed op slot and pushes 1 if the NPC's
// NpcType has a non-empty op string at that slot, else 0.
// Mirrors TS NpcOps.ts NPC_HASOP: check(op, NumberNotNull) (NAI-23 Bundle 4a).
func handleNpcHasOp(s *ScriptState) error {
	if err := requireActiveNpc(s, "NPC_HASOP"); err != nil {
		return err
	}
	op := s.PopInt()
	if err := checkNotNull(op, "NPC_HASOP"); err != nil {
		return err
	}
	if s.Configs == nil {
		s.PushInt(0)
		return nil
	}
	cfg := s.Configs.NpcType(s.ActiveNpc.NpcType())
	if cfg == nil {
		s.PushInt(0)
		return nil
	}
	idx := op - 1
	if idx < 0 || idx >= len(cfg.Op) || cfg.Op[idx] == "" {
		s.PushInt(0)
	} else {
		s.PushInt(1)
	}
	return nil
}

// handleNpcUID pushes the NPC's packed UID: (typeId << 16) | nid.
func handleNpcUID(s *ScriptState) error {
	if err := requireActiveNpc(s, "NPC_UID"); err != nil {
		return err
	}
	s.PushInt(s.ActiveNpc.NpcUID())
	return nil
}

// handleNpcCategory looks up the ActiveNpc's NpcType via Configs and
// pushes its Category, or -1 if the type can't be resolved.
func handleNpcCategory(s *ScriptState) error {
	if err := requireActiveNpc(s, "NPC_CATEGORY"); err != nil {
		return err
	}
	if s.Configs == nil {
		s.PushInt(-1)
		return nil
	}
	cfg := s.Configs.NpcType(s.ActiveNpc.NpcType())
	if cfg == nil {
		s.PushInt(-1)
		return nil
	}
	s.PushInt(cfg.Category)
	return nil
}

// handleNpcSay pops a string and sets it as the active NPC's speech
// bubble for this tick. Empty strings are legal (clears the bubble).
func handleNpcSay(s *ScriptState) error {
	if err := requireActiveNpc(s, "NPC_SAY"); err != nil {
		return err
	}
	text := s.PopString()
	s.ActiveNpc.Say([]byte(text))
	return nil
}

// handleNpcAnim pops (seq, delay) in TS order (delay on top) and schedules
// the animation on the active NPC this tick.
// Mirrors TS NpcOps.ts NPC_ANIM: check(delay, NumberNotNull); seq is NOT
// wrapped per TS (NAI-23 Bundle 4a).
func handleNpcAnim(s *ScriptState) error {
	if err := requireActiveNpc(s, "NPC_ANIM"); err != nil {
		return err
	}
	delay := s.PopInt()
	if err := checkNotNull(delay, "NPC_ANIM"); err != nil {
		return err
	}
	id := s.PopInt()
	s.ActiveNpc.Animate(id, delay)
	return nil
}

// handleNpcFaceSquare pops a single packed coord (level<<28 | x<<14 | z)
// and rotates the NPC to face that absolute square. Level bits are unused
// here (the NPC's own level always matches its face target in practice).
func handleNpcFaceSquare(s *ScriptState) error {
	if err := requireActiveNpc(s, "NPC_FACESQUARE"); err != nil {
		return err
	}
	_, x, z := unpackCoord(s.PopInt())
	s.ActiveNpc.FaceCoord(x, z)
	return nil
}

// handleNpcChangeType pops (newType, duration) in TS order (duration
// on top) and morphs the NPC. Matches TS NpcOps.ts:457-462.
// The full body (guard + typeId/uid/mask + stats-reset +
// lifecycleTick fast-path) lives in *Npc.changeTypeImpl.
func handleNpcChangeType(s *ScriptState) error {
	if err := requireActiveNpc(s, "NPC_CHANGETYPE"); err != nil {
		return err
	}
	duration := s.PopInt()
	newType := s.PopInt()
	s.ActiveNpc.ChangeType(newType, duration)
	return nil
}

// handleNpcChangeTypeKeepAll pops (newType, duration) in TS order
// (duration on top) and morphs the NPC preserving all current stats.
// Matches TS NpcOps.ts:465-471.
func handleNpcChangeTypeKeepAll(s *ScriptState) error {
	if err := requireActiveNpc(s, "NPC_CHANGETYPE_KEEPALL"); err != nil {
		return err
	}
	duration := s.PopInt()
	newType := s.PopInt()
	s.ActiveNpc.ChangeTypeKeepAll(newType, duration)
	return nil
}

// handleNpcDamage pops (type, amount) in TS order (amount on top) and
// applies damage. The concrete Npc impl manages HP; this handler stays thin.
// Mirrors TS NpcOps.ts NPC_DAMAGE: check(amount, NumberNotNull); dmgType is
// wrapped with HitTypeValid (not NumberNotNull) and stays raw (NAI-23 Bundle 4a).
func handleNpcDamage(s *ScriptState) error {
	if err := requireActiveNpc(s, "NPC_DAMAGE"); err != nil {
		return err
	}
	amount := s.PopInt()
	if err := checkNotNull(amount, "NPC_DAMAGE"); err != nil {
		return err
	}
	dmgType := s.PopInt()
	s.ActiveNpc.Damage(amount, dmgType)
	return nil
}

// handleNpcDel (NPC_DEL, opcode 2510) removes the active NPC. The
// duration passed to World.RemoveNpc is the active NPC type's
// respawnrate; Server.removeNpc scales it by player count and writes
// it to lifecycleTick (RESPAWN-lifecycle) or schedules registry
// cleanup (DESPAWN-lifecycle, currently dead-bool model — see
// modules/world/npc_registry.go:181 and TODO(NAI-19)).
//
// Mirrors TS NpcOps.ts:78-80:
//
//	[ScriptOpcode.NPC_DEL]: checkedHandler(ActiveNpc, state => {
//	    World.removeNpc(state.activeNpc, check(state.activeNpc.type, NpcTypeValid).respawnrate);
//	}),
//
// DEVIATION-NAI-126-D1: nil-World defensive guard (goscape defensive;
// TS skips this check — World is always present in a running engine).
// Mirrors handleObjDel at handlers_obj.go:122-124. Retire when an
// upstream invariant proves s.World is non-nil for any executing
// script.
func handleNpcDel(s *ScriptState) error {
	if err := requireActiveNpc(s, "NPC_DEL"); err != nil {
		return err
	}
	if s.World == nil {
		return fmt.Errorf("NPC_DEL: no world surface")
	}
	s.World.RemoveNpc(s.ActiveNpc, s.ActiveNpc.Respawnrate())
	return nil
}

// handleNpcDelay (NPC_DELAY, opcode 2511) suspends the active NPC's
// script for N ticks. Transitions the script to NpcSuspended and
// records the wake tick on the NPC via SetDelayed. The tick loop
// resumes the script from Npc.turn() when delayedUntil expires.
// Mirrors TS NpcOps.ts:82-84, including the NumberNotNull check
// (closed in NAI-20).
func handleNpcDelay(s *ScriptState) error {
	if err := requireActiveNpc(s, "NPC_DELAY"); err != nil {
		return err
	}
	ticks := s.PopInt()
	if err := checkNotNull(ticks, "NPC_DELAY"); err != nil {
		return err
	}
	s.ActiveNpc.SetDelayed(ticks)
	s.Execution = NpcSuspended
	return nil
}

// handleNpcArriveDelay implements NPC_ARRIVEDELAY (opcode 2502): if the
// active NPC has moved within the past 3 ticks (this tick, last tick, or
// 2 ticks ago), suspend the script with a delay computed from the
// movement recency; otherwise no-op. TS NpcOps.ts:542-555.
//
// The 3-tick window arises from the TS lastMovement contract (written
// to currentTick + 1 after a moving tick): the gate accepts moves from
// this tick (lastMovement = T+1), last tick (lastMovement = T), and
// 2 ticks ago (lastMovement = T-1) but rejects moves from 3+ ticks ago
// (lastMovement <= T-2; T-2 < T-1 ⇒ return).
//
// Inner branch: if NPC moved 2 ticks ago (lastMovement = T-1), suspend
// for 1 tick (TS delayedUntil = T+1 ⇒ goscape SetDelayed(0)). Otherwise
// (this tick or last tick), suspend for 2 ticks (TS delayedUntil = T+2
// ⇒ goscape SetDelayed(1)). The +1 offset comes from goscape's
// SetDelayed(ticks) writing delayedUntil = currentTick + 1 + ticks
// (npc.go:323-326).
//
// Vs P_ARRIVEDELAY (handlers.go:739): NPC variant has a 3-tick window
// (vs 2) and a recency-dependent suspend duration (vs always 1 tick),
// per TS NpcOps.ts asymmetry vs PlayerOps.ts:357-366.
//
// DEVIATION-NAI-125-D1: s.World == nil defensive guard (goscape
// defensive; TS skips this check). Mirrors handlePArriveDelay /
// handleMapClock / handlePlayerCount sibling-handler convention.
func handleNpcArriveDelay(s *ScriptState) error {
	if err := requireActiveNpc(s, "NPC_ARRIVEDELAY"); err != nil {
		return err
	}
	if s.World == nil {
		return errors.New("NPC_ARRIVEDELAY: no world")
	}
	last := s.ActiveNpc.LastMovement()
	tick := s.World.CurrentTick()
	if last < tick-1 {
		return nil
	}
	if last == tick-1 {
		s.ActiveNpc.SetDelayed(0) // delayedUntil = T+1
	} else {
		s.ActiveNpc.SetDelayed(1) // delayedUntil = T+2
	}
	s.Execution = NpcSuspended
	return nil
}

// handleNpcQueue (NPC_QUEUE, opcode 2530) enqueues an ai_queueN
// dispatch on the active NPC. Pop order: delay (top), arg, queueId
// (bottom). queueId ∈ [1, 20] maps to TriggerAiQueue1..20 via
// arithmetic: trigger = TriggerAiQueue1 + queueId - 1. Mirrors TS
// NpcOps.ts:144-150, including the NumberNotNull check on delay
// (closed in NAI-20). The Go-side queueId 1..20 range check
// corresponds to TS QueueValid; the arg pop is unwrapped per TS.
func handleNpcQueue(s *ScriptState) error {
	if err := requireActiveNpc(s, "NPC_QUEUE"); err != nil {
		return err
	}
	delay := s.PopInt()
	if err := checkNotNull(delay, "NPC_QUEUE"); err != nil {
		return err
	}
	lastIntArg := s.PopInt()
	queueID := s.PopInt()
	if queueID < 1 || queueID > 20 {
		return fmt.Errorf("NPC_QUEUE: invalid queueId %d (want 1..20)", queueID)
	}
	trigger := TriggerAiQueue1 + ServerTriggerType(queueID-1)
	s.ActiveNpc.EnqueueScriptForTrigger(trigger, delay, lastIntArg)
	return nil
}

// handleNpcSetTimer (NPC_SETTIMER, opcode 2536) sets the active
// NPC's ai_timer tick interval. Pop order: interval. Mirrors TS
// NpcOps.ts:278-280, including the NumberNotNull check (closed in S7b).
func handleNpcSetTimer(s *ScriptState) error {
	if err := requireActiveNpc(s, "NPC_SETTIMER"); err != nil {
		return err
	}
	interval := s.PopInt()
	if err := checkNotNull(interval, "NPC_SETTIMER"); err != nil {
		return err
	}
	s.ActiveNpc.SetTimer(interval)
	return nil
}

// handleNpcTele (NPC_TELE, opcode 2541) teleports the active NPC to
// the packed coord. Pop order: coord (single int). Mirrors TS
// NpcOps.ts:443 — checkedHandler(ActiveNpc) + CoordValid +
// activeNpc.teleport(x, z, level).
func handleNpcTele(s *ScriptState) error {
	if err := requireActiveNpc(s, "NPC_TELE"); err != nil {
		return err
	}
	coord := s.PopInt()
	level, x, z, err := checkCoord(coord, "NPC_TELE")
	if err != nil {
		return err
	}
	s.ActiveNpc.Teleport(x, z, level)
	return nil
}

// handleNpcWalk (NPC_WALK, opcode 2544) queues a single waypoint for the
// active NPC at the unpacked coord. Pop order: coord (single int). Mirrors
// TS NpcOps.ts:451-455 — checkedHandler(ActiveNpc) + CoordValid +
// activeNpc.queueWaypoint(x, z). NOTE: level is dropped TS-faithfully; the
// waypoint uses the NPC's current level by convention.
func handleNpcWalk(s *ScriptState) error {
	if err := requireActiveNpc(s, "NPC_WALK"); err != nil {
		return err
	}
	coord := s.PopInt()
	_, x, z, err := checkCoord(coord, "NPC_WALK")
	if err != nil {
		return err
	}
	s.ActiveNpc.QueueWaypoint(x, z)
	return nil
}

// handleNpcWalkTrigger (NPC_WALKTRIGGER, opcode 2545) sets a deferred
// AI-queue trigger and arg on the active NPC; the trigger fires when
// the NPC completes a walk step. Pop order: arg (top), queueID
// (bottom). queueID ∈ [1, 20] mirrors TS QueueValid range, transformed
// to [0, 19] via queueID-1 to match TS NpcOps.ts:488 storage. Mirrors
// TS NpcOps.ts:483-490. The walktrigger consumer fires from
// (*Npc).updateMovement (modules/world/npc_interaction.go, NAI-51 T2.1).
func handleNpcWalkTrigger(s *ScriptState) error {
	if err := requireActiveNpc(s, "NPC_WALKTRIGGER"); err != nil {
		return err
	}
	arg := s.PopInt()
	queueID := s.PopInt()
	if queueID < 1 || queueID > 20 {
		return fmt.Errorf("NPC_WALKTRIGGER: invalid queueId %d (want 1..20)", queueID)
	}
	s.ActiveNpc.SetWalkTrigger(queueID - 1)
	s.ActiveNpc.SetWalkTriggerArg(arg)
	return nil
}

// handleNpcGetMode (NPC_GETMODE, opcode 2522) pushes the active NPC's
// targetOp value (the mode set by NPC_SETMODE / interaction binding).
// Mirrors TS NpcOps.ts:473-475 — checkedHandler(ActiveNpc) + pushInt.
func handleNpcGetMode(s *ScriptState) error {
	if err := requireActiveNpc(s, "NPC_GETMODE"); err != nil {
		return err
	}
	s.PushInt(s.ActiveNpc.TargetOp())
	return nil
}

// handleNpcSetMode (NPC_SETMODE, opcode 2535) sets the active NPC's mode
// (targetOp). 3-branch dispatch:
//
//  1. clear-target modes (NONE/WANDER/PATROL): clearInteraction +
//     targetOp = mode; PATROL additionally clearPatrol.
//  2. NULL: resetDefaults.
//  3. target-binding modes (OPPLAYER*/OPLOC*/OPOBJ*/OPNPC* + AP* + the
//     four PlayerEscape/Follow/Face/FaceClose modes): targetOp = mode,
//     then resolve target by mode-range and bind via setInteraction(SCRIPT,
//     target, mode); a nil target falls through to resetDefaults.
//
// Mirrors TS NpcOps.ts:188-249. Branch order in step 3 (Npc/Obj/Loc/Player)
// matches TS line 207-219; the OpNpc branch additionally consults
// state.IntOperands[PC] (TS state.intOperand): operand==0 selects
// OtherActiveNpc (".npc2" syntax), nonzero selects ActiveNpc (self).
func handleNpcSetMode(s *ScriptState) error {
	if err := requireActiveNpc(s, "NPC_SETMODE"); err != nil {
		return err
	}
	mode := s.PopInt()
	if err := checkNpcMode(mode, "NPC_SETMODE"); err != nil {
		return err
	}

	// Branch 1: clear-target modes.
	if mode == objtype.NPCModeNone || mode == objtype.NPCModeWander || mode == objtype.NPCModePatrol {
		s.ActiveNpc.ClearInteraction()
		s.ActiveNpc.SetTargetOp(mode)
		if mode == objtype.NPCModePatrol {
			s.ActiveNpc.ClearPatrol()
		}
		return nil
	}

	// Branch 2: NULL → resetDefaults.
	if mode == objtype.NPCModeNull {
		s.ActiveNpc.ResetDefaults()
		return nil
	}

	// Branch 3: target-binding modes.
	s.ActiveNpc.SetTargetOp(mode)

	var target any
	switch {
	case mode >= objtype.NPCModeOpNpc1: // OPNPC1..APNPC5
		operand := s.Script.IntOperands[s.PC]
		if operand == 0 {
			target = s.OtherActiveNpc
		} else {
			target = s.ActiveNpc
		}
	case mode >= objtype.NPCModeOpObj1: // OPOBJ1..APOBJ5
		target = s.ActiveObj
	case mode >= objtype.NPCModeOpLoc1: // OPLOC1..APLOC5
		target = s.ActiveLoc
	default: // PlayerEscape/Follow/Face/FaceClose + OPPLAYER1..APPLAYER5
		target = s.Self
	}

	if target == nil {
		s.ActiveNpc.ResetDefaults()
		return nil
	}
	s.ActiveNpc.SetInteractionScript(target, mode)
	return nil
}

// handleNpcSetHunt (NPC_SETHUNT, opcode 2533) sets the NPC's hunt
// search range. Despite the opcode name, this sets RANGE only —
// hunt mode is set via the separate NPC_SETHUNTMODE opcode.
// Mirrors TS NpcOps.ts:174-176, including the NumberNotNull check
// (closed in NAI-20).
func handleNpcSetHunt(s *ScriptState) error {
	if err := requireActiveNpc(s, "NPC_SETHUNT"); err != nil {
		return err
	}
	huntRange := s.PopInt()
	if err := checkNotNull(huntRange, "NPC_SETHUNT"); err != nil {
		return err
	}
	s.ActiveNpc.SetHuntRange(huntRange)
	return nil
}

// handleNpcSetHuntMode (NPC_SETHUNTMODE, opcode 2534) sets the NPC's
// HuntType id. -1 clears the hunt mode (valid input). Mirrors TS
// NpcOps.ts:178-185.
func handleNpcSetHuntMode(s *ScriptState) error {
	if err := requireActiveNpc(s, "NPC_SETHUNTMODE"); err != nil {
		return err
	}
	s.ActiveNpc.SetHuntMode(s.PopInt())
	return nil
}

// handleNpcFind (NPC_FIND, opcode 2513) pops (coord, npc, distance,
// huntvis), validates each, asks NpcLookup for the closest NPC of that
// type within square-bounded distance, and either sets the active NPC
// slot + pushes 1 or pushes 0. Mirrors TS NpcOps.ts:336-367. Gate:
// none (ActivePlayer-agnostic — the opcode only depends on the world).
// Pointer-set is conditional on hit (TS ScriptOpcodePointers.ts:579).
func handleNpcFind(s *ScriptState) error {
	checkVis := s.PopInt()
	distance := s.PopInt()
	npcTypeID := s.PopInt()
	coord := s.PopInt()

	level, x, z, err := checkCoord(coord, "NPC_FIND")
	if err != nil {
		return err
	}
	if err := checkNpcType(s, npcTypeID, "NPC_FIND"); err != nil {
		return err
	}
	if err := checkNotNull(distance, "NPC_FIND"); err != nil {
		return err
	}
	if err := checkHuntVis(checkVis, "NPC_FIND"); err != nil {
		return err
	}

	var npc ActiveNpc
	if s.Npcs != nil {
		npc = s.Npcs.FindClosestNpcByType(level, x, z, distance, npcTypeID, checkVis)
	}
	if npc == nil {
		s.PushInt(0)
		return nil
	}
	setActiveNpcSlot(s, npc)
	s.PushInt(1)
	return nil
}

// handleNpcFindCat (NPC_FINDCAT, opcode 2517) pops (coord, category,
// distance, huntvis). Same spine as handleNpcFind but filter is by
// NpcType.Category == category (handled in the world-side impl).
// checkCategoryType is partial (S7f-D3). Mirrors TS NpcOps.ts:369-400.
func handleNpcFindCat(s *ScriptState) error {
	checkVis := s.PopInt()
	distance := s.PopInt()
	category := s.PopInt()
	coord := s.PopInt()

	level, x, z, err := checkCoord(coord, "NPC_FINDCAT")
	if err != nil {
		return err
	}
	if err := checkCategoryType(category, "NPC_FINDCAT"); err != nil {
		return err
	}
	if err := checkNotNull(distance, "NPC_FINDCAT"); err != nil {
		return err
	}
	if err := checkHuntVis(checkVis, "NPC_FINDCAT"); err != nil {
		return err
	}

	var npc ActiveNpc
	if s.Npcs != nil {
		npc = s.Npcs.FindClosestNpcByCategory(level, x, z, distance, category, checkVis)
	}
	if npc == nil {
		s.PushInt(0)
		return nil
	}
	setActiveNpcSlot(s, npc)
	s.PushInt(1)
	return nil
}

// handleNpcFindExact (NPC_FINDEXACT, opcode 2518) pops (coord, npcType).
// Iterates NPCs at exactly (level, x, z) of the popped coord whose type
// matches. Mirrors TS NpcOps.ts:94-112. Pointer-set conditional on hit.
func handleNpcFindExact(s *ScriptState) error {
	npcTypeID := s.PopInt()
	coord := s.PopInt()

	level, x, z, err := checkCoord(coord, "NPC_FINDEXACT")
	if err != nil {
		return err
	}
	if err := checkNpcType(s, npcTypeID, "NPC_FINDEXACT"); err != nil {
		return err
	}

	var npc ActiveNpc
	if s.Npcs != nil {
		npc = s.Npcs.FindNpcAtExactCoord(level, x, z, npcTypeID)
	}
	if npc == nil {
		s.PushInt(0)
		return nil
	}
	setActiveNpcSlot(s, npc)
	s.PushInt(1)
	return nil
}

// handleNpcFindAllAny (NPC_FINDALLANY, opcode 2515) pops (coord, distance,
// huntvis), validates, and stores a DISTANCE-mode NpcIterator on
// state.npcIterator with no type filter. Mirrors TS NpcOps.ts:403-411.
// Pointer-set is `set ['find_npc']` (ScriptOpcodePointers.ts:586-588);
// goscape encodes the find_npc pointer as state.npcIterator != nil.
// No push (TS doesn't push either). NAI-33-D1: huntvis validated but
// not consumed by passesFilter (Distance mode preserves the
// deferred-not-consumed posture; HuntAll mode at NAI-35-T3 is the only
// mode that activates LoS/LoW filtering).
func handleNpcFindAllAny(s *ScriptState) error {
	checkVis := s.PopInt()
	distance := s.PopInt()
	coord := s.PopInt()

	level, x, z, err := checkCoord(coord, "NPC_FINDALLANY")
	if err != nil {
		return err
	}
	if err := checkNotNull(distance, "NPC_FINDALLANY"); err != nil {
		return err
	}
	if err := checkHuntVis(checkVis, "NPC_FINDALLANY"); err != nil {
		return err
	}

	// Mirror existing FIND handler nil-Npcs degradation pattern: skip
	// iterator creation; FINDNEXT's nil-iterator branch pushes 0.
	if s.Npcs == nil {
		return nil
	}
	s.npcIterator = NewDistanceNpcIterator(
		s.Npcs, s.World.CurrentTick(),
		level, x, z, distance, checkVis, -1,
	)
	return nil
}

// handleNpcFindAll (NPC_FINDALL, opcode 2514) pops (coord, npc, distance,
// huntvis), validates, and stores a DISTANCE-mode NpcIterator with
// typeID set to filter by NPC type. Mirrors TS NpcOps.ts:413-422.
// Pop order matches TS popInts(4): top → bottom = checkVis, distance,
// npcTypeID, coord. NAI-33-D1: huntvis validated but not consumed by
// passesFilter (Distance mode preserves the deferred-not-consumed
// posture; HuntAll mode at NAI-35-T3 is the only mode that activates
// LoS/LoW filtering).
func handleNpcFindAll(s *ScriptState) error {
	checkVis := s.PopInt()
	distance := s.PopInt()
	npcTypeID := s.PopInt()
	coord := s.PopInt()

	level, x, z, err := checkCoord(coord, "NPC_FINDALL")
	if err != nil {
		return err
	}
	if err := checkNotNull(distance, "NPC_FINDALL"); err != nil {
		return err
	}
	if err := checkNpcType(s, npcTypeID, "NPC_FINDALL"); err != nil {
		return err
	}
	if err := checkHuntVis(checkVis, "NPC_FINDALL"); err != nil {
		return err
	}

	if s.Npcs == nil {
		return nil
	}
	s.npcIterator = NewDistanceNpcIterator(
		s.Npcs, s.World.CurrentTick(),
		level, x, z, distance, checkVis, npcTypeID,
	)
	return nil
}

// handleNpcFindAllZone (NPC_FINDALLZONE, opcode 2516) pops a coord,
// validates, and stores a ZONE-mode NpcIterator targeting the single
// zone containing that coord. Mirrors TS NpcOps.ts:424-428. No
// distance/huntvis/type validation (TS doesn't do them either).
func handleNpcFindAllZone(s *ScriptState) error {
	coord := s.PopInt()
	level, x, z, err := checkCoord(coord, "NPC_FINDALLZONE")
	if err != nil {
		return err
	}
	if s.Npcs == nil {
		return nil
	}
	s.npcIterator = NewZoneNpcIterator(s.Npcs, s.World.CurrentTick(), level, x, z)
	return nil
}

// handleNpcHuntAll (NPC_HUNTALL, opcode 2526) pops [coord, distance,
// huntvis] and stores a HuntAll-mode NpcIterator in s.npcIterator
// (consumed by NPC_FINDNEXT 2520). Mirrors TS NpcOps.ts:325-333.
//
// Pop order (top-of-stack first): huntvis, distance, coord.
// Validation: checkCoord, checkNotNull(distance), checkHuntVis.
// Nil-Npcs degrades silently (matches NPC_FINDALL convention).
//
// NAI-35-T3: partially closes NAI-33-D1 (huntvis becomes a live
// consumer of LoS/LoW filtering via passesFilter HuntAll branch);
// Distance mode + FindClosestNpc* still residual.
func handleNpcHuntAll(s *ScriptState) error {
	checkVis := s.PopInt()
	distance := s.PopInt()
	coord := s.PopInt()

	level, x, z, err := checkCoord(coord, "NPC_HUNTALL")
	if err != nil {
		return err
	}
	if err := checkNotNull(distance, "NPC_HUNTALL"); err != nil {
		return err
	}
	if err := checkHuntVis(checkVis, "NPC_HUNTALL"); err != nil {
		return err
	}

	if s.Npcs == nil {
		return nil
	}
	s.npcIterator = NewHuntAllNpcIterator(
		s.Npcs, s.LineValidator, s.World.CurrentTick(),
		level, x, z, distance, checkVis,
	)
	return nil
}

// handleNpcFindNext (NPC_FINDNEXT, opcode 2520) advances the active
// NpcIterator and either sets active_npc + pushes 1 on hit, or pushes 0
// on miss / nil-iterator. Mirrors TS NpcOps.ts:430-441. Pointer-set is
// `require ['find_npc']`, `set ['active_npc']`, conditional
// (ScriptOpcodePointers.ts:595-600). Goscape encodes the require as a
// nil-check on s.npcIterator.
//
// Stale-iterator semantics: TS throws on stale (ScriptIterators.ts:332,343);
// goscape returns error → existing npc_script.go:169 log-warn +
// ClearActiveScript path runs. Single-tick lifetime preserved.
//
// Exhaustion does NOT clear s.npcIterator (matches TS
// state.npcIterator?.next() returning {done:true} without nulling).
// Subsequent FINDNEXT calls continue to return push-0.
//
// Stale-check timing: TS checks staleness inside the generator on
// each yield (ScriptIterators.ts:331,342); goscape checks once at
// handler entry. Equivalent in the single-tick lifetime model since
// the iterator is consumed entirely within a single FINDNEXT call —
// no opportunity for tick drift mid-iteration.
func handleNpcFindNext(s *ScriptState) error {
	it := s.npcIterator
	if it == nil {
		s.PushInt(0)
		return nil
	}
	if it.Stale(s.World.CurrentTick()) {
		return fmt.Errorf("NPC_FINDNEXT: tried to use an old iterator. Create a new iterator instead.")
	}
	npc, ok := it.Next()
	if !ok {
		s.PushInt(0)
		return nil
	}
	setActiveNpcSlot(s, npc)
	s.PushInt(1)
	return nil
}

// handleNpcFindUID (NPC_FINDUID, opcode 2521) pops a packed NPC UID and
// binds the matching live NPC to the active slot dictated by the bytecode
// IntOperand (.npc → primary, .npc2 → secondary). Pushes 1 on hit, 0 on
// miss. Does NOT set the Protect bit. Mirrors TS NpcOps.ts:26-40:
//
//	const slot = npcUid & 0xffff;
//	const expectedType = (npcUid >> 16) & 0xffff;
//	const npc = World.getNpc(slot);
//	if (!npc || npc.type !== expectedType) {
//	    state.pushInt(0);
//	    return;
//	}
//	state.activeNpc = npc;
//	state.pointerAdd(ActiveNpc[state.intOperand]);
//	state.pushInt(1);
//
// goscape's NpcLookup.FindNpcByUID encapsulates the slot-lookup +
// type-match check, returning nil on miss. NAI-120 Bundle 2A.
func handleNpcFindUID(s *ScriptState) error {
	uid := s.PopInt()
	if s.Npcs == nil {
		s.PushInt(0)
		return nil
	}
	npc := s.Npcs.FindNpcByUID(uid)
	if npc == nil {
		s.PushInt(0)
		return nil
	}
	setActiveNpcSlot(s, npc)
	s.PushInt(1)
	return nil
}

// handleNpcRange (NPC_RANGE, opcode 2531) pops a packed coord and pushes the
// Chebyshev distance from the active NPC to that 1x1 tile. Returns -1 when
// the coord's level differs from the NPC's level (TS sentinel). Mirrors TS
// NpcOps.ts:152-168:
//
//	const coord: CoordGrid = check(state.popInt(), CoordValid);
//	const npc = state.activeNpc;
//	if (coord.level !== npc.level) {
//	    state.pushInt(-1);
//	} else {
//	    state.pushInt(CoordGrid.distanceTo(npc, {x, z, width:1, length:1}));
//	}
//
// `CoordGrid.distanceTo` for a 1x1 target reduces to Chebyshev:
// max(|npcX - x|, |npcZ - z|) -- width=1/length=1 contributes 0 to the
// per-axis subtractions in the TS formula. NAI-120 Bundle 2A.
//
// Multi-tile NPCs (size > 1): the inner-ring call sites in
// player_combat.rs2 do not require size-aware distance -- sites pass
// `coord` (the player's own coord) and the active NPC is the combat
// target. This handler treats the NPC as a 1x1 source (matches TS
// behaviour for size=1 NPCs; size>1 audit deferred to a future sub-spec
// per NAI-120 Bundle 1 audit section 6 dependency note).
func handleNpcRange(s *ScriptState) error {
	if err := requireActiveNpc(s, "NPC_RANGE"); err != nil {
		return err
	}
	coord := s.PopInt()
	level, x, z, err := checkCoord(coord, "NPC_RANGE")
	if err != nil {
		return err
	}
	n := s.ActiveNpc
	if level != n.NpcLevel() {
		s.PushInt(-1)
		return nil
	}
	dx := n.NpcX() - x
	if dx < 0 {
		dx = -dx
	}
	dz := n.NpcZ() - z
	if dz < 0 {
		dz = -dz
	}
	s.PushInt(max(dx, dz))
	return nil
}

// handleNpcStatAdd (NPC_STATADD, opcode 2538) boosts the active NPC's stat.
// Pop order: percent (top), constant, stat (bottom). All three are popped
// before any validation runs so that validation order matches TS exactly:
// stat → constant → percent (TS NpcOps.ts:495-497, popInts(3) destructured).
// Formula clamped at 255:
//
//	added = current + trunc(constant + (base*percent)/100)
//	npc.levels[stat] = min(added, 255)
//
// Mirrors TS NpcOps.ts:492-504. NAI-120 Bundle 2C.
func handleNpcStatAdd(s *ScriptState) error {
	if err := requireActiveNpc(s, "NPC_STATADD"); err != nil {
		return err
	}
	percent := s.PopInt()
	constant := s.PopInt()
	stat := s.PopInt()
	if err := checkNpcStatID(stat, "NPC_STATADD"); err != nil {
		return err
	}
	if err := checkNotNull(constant, "NPC_STATADD"); err != nil {
		return err
	}
	if err := checkNotNull(percent, "NPC_STATADD"); err != nil {
		return err
	}
	base := s.ActiveNpc.NpcBaseStat(stat)
	cur := s.ActiveNpc.NpcStat(stat)
	added := cur + (constant + (base*percent)/100)
	added = min(added, 255)
	s.ActiveNpc.SetNpcStat(stat, added)
	return nil
}

// handleNpcStatSub (NPC_STATSUB, opcode 2540) drains the active NPC's stat.
// Pop order matches NPC_STATADD; validation order is stat → constant →
// percent to match TS NpcOps.ts:509-511 exactly. Formula clamped at 0:
//
//	subbed = current - trunc(constant + (base*percent)/100)
//	npc.levels[stat] = max(subbed, 0)
//
// Mirrors TS NpcOps.ts:506-518. NAI-120 Bundle 2C.
func handleNpcStatSub(s *ScriptState) error {
	if err := requireActiveNpc(s, "NPC_STATSUB"); err != nil {
		return err
	}
	percent := s.PopInt()
	constant := s.PopInt()
	stat := s.PopInt()
	if err := checkNpcStatID(stat, "NPC_STATSUB"); err != nil {
		return err
	}
	if err := checkNotNull(constant, "NPC_STATSUB"); err != nil {
		return err
	}
	if err := checkNotNull(percent, "NPC_STATSUB"); err != nil {
		return err
	}
	base := s.ActiveNpc.NpcBaseStat(stat)
	cur := s.ActiveNpc.NpcStat(stat)
	subbed := cur - (constant + (base*percent)/100)
	subbed = max(subbed, 0)
	s.ActiveNpc.SetNpcStat(stat, subbed)
	return nil
}

// handleSpotAnimNpc (SPOTANIM_NPC, opcode 2547) queues a spotanim on the
// active NPC. Pop order: delay (top), height, spotanim id (bottom). Mirrors
// TS NpcOps.ts:282-288:
//
//	const delay = check(state.popInt(), NumberNotNull);
//	const height = check(state.popInt(), NumberNotNull);
//	const spotanimType = check(state.popInt(), SpotAnimTypeValid);
//	state.activeNpc.spotanim(spotanimType.id, height, delay);
//
// NAI-120 Bundle 2C.
func handleSpotAnimNpc(s *ScriptState) error {
	if err := requireActiveNpc(s, "SPOTANIM_NPC"); err != nil {
		return err
	}
	delay := s.PopInt()
	if err := checkNotNull(delay, "SPOTANIM_NPC"); err != nil {
		return err
	}
	height := s.PopInt()
	if err := checkNotNull(height, "SPOTANIM_NPC"); err != nil {
		return err
	}
	id := s.PopInt()
	if err := checkSpotAnimType(s, id, "SPOTANIM_NPC"); err != nil {
		return err
	}
	s.ActiveNpc.PlaySpotAnim(id, height, delay)
	return nil
}

// handleNpcHeroPoints (NPC_HEROPOINTS, opcode 2524) credits the active
// player's UID with `amount` hero points on the active NPC's ledger. Used
// for damage-contribution loot routing on NPC death. Mirrors TS
// NpcOps.ts:477-480 (https://x.com/JagexAsh/status/1704492467226091853):
//
//	state.activeNpc.heroPoints.addHero(state.activePlayer.hash64,
//	    check(state.popInt(), NumberNotNull));
//
// Gate: ProtectedActivePlayer NOT required. TS uses
// checkedHandler([ActivePlayer, ...ActiveNpc]) which selects exactly ONE
// pointer to validate via state.intOperand (ScriptPointer.ts:52); for the
// compiled value used in combat scripts (intOperand=0) only ActivePlayer
// is enforced, with ActiveNpc relied on implicitly. Goscape additionally
// gates on requireActiveNpc (goscape defensive; TS skips this check —
// would surface as a runtime NPE in TS). goscape uses player UID instead
// of TS hash64 (player UID is the goscape analog of the hash64 player
// identity token). NAI-120 Bundle 2D.
func handleNpcHeroPoints(s *ScriptState) error {
	if err := requireActivePlayer(s, "NPC_HEROPOINTS"); err != nil {
		return err
	}
	if err := requireActiveNpc(s, "NPC_HEROPOINTS"); err != nil {
		return err
	}
	amount := s.PopInt()
	if err := checkNotNull(amount, "NPC_HEROPOINTS"); err != nil {
		return err
	}
	s.ActiveNpc.AddHeroPoints(s.Self.UID(), amount)
	return nil
}
