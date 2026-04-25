package script

import (
	"errors"
	"fmt"
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
	arg := s.PopInt()
	queueID := s.PopInt()
	if queueID < 1 || queueID > 20 {
		return fmt.Errorf("NPC_QUEUE: invalid queueId %d (want 1..20)", queueID)
	}
	trigger := TriggerAiQueue1 + ServerTriggerType(queueID-1)
	s.ActiveNpc.EnqueueScriptForTrigger(trigger, delay, arg)
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
