package script

import (
	"fmt"
	"math"

	"github.com/zsrv/goscape/pkg/objtype"
)

// handleNpcAdd (NPC_ADD, opcode 2500) pops [coord, id, duration] and
// spawns a despawn-lifecycle NPC of typeID `id` at the unpacked coord.
// Mirrors TS NpcOps.ts:42-53:
//
//	const [coord, id, duration] = state.popInts(3);
//	const position = check(coord, CoordValid);
//	const npcType  = check(id,    NpcTypeValid);
//	check(duration, DurationValid);
//	const npc = new Npc(level, x, z, size, size, DESPAWN, getNextNid(),
//	    id, moverestrict, blockwalk);
//	World.addNpc(npc, duration);
//	state.activeNpc = npc;
//	state.pointerAdd(ActiveNpc[state.intOperand]);
//
// Pop order (top first): duration, id, coord. NO push on success
// (TS handler does not push). Sets ActiveNpc + PtrActiveNpc via
// setActiveNpcSlot. Errors from AddNpcAt (registry full, etc.) bubble
// to the dispatch loop. NAI-163 B3.
func handleNpcAdd(s *ScriptState) error {
	duration := s.PopInt()
	id := s.PopInt()
	coord := s.PopInt()

	level, x, z, err := checkCoord(coord, "NPC_ADD")
	if err != nil {
		return err
	}
	if err := checkNpcType(s, id, "NPC_ADD"); err != nil {
		return err
	}
	if err := checkDuration(duration); err != nil {
		return err
	}

	npc, err := s.World.AddNpcAt(level, x, z, id, duration)
	if err != nil {
		return err
	}
	setActiveNpcSlot(s, npc)
	return nil
}

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

// checkHuntType mirrors TS HuntTypeValid (ScriptValidators.ts:115) —
// range + registry presence check, collapsed into a single
// Configs.HuntType(id) nil check per the checkNpcType pattern. Used by
// NPC_SETHUNTMODE; callers are responsible for handling the -1 ("clear")
// sentinel before invoking this validator (TS does the same).
func checkHuntType(s *ScriptState, id int, op string) error {
	if s.Configs == nil || s.Configs.HuntType(id) == nil {
		return fmt.Errorf("%s: no HuntType with value (%d) found", op, id)
	}
	return nil
}

// checkHitType validates a hit-type wire value against
// objtype.HitTypeCount. Mirrors TS HitTypeValid (ScriptValidators.ts:117)
// — ScriptInputRangeValidator(HitType.BLOCK, HitType.POISON), inclusive
// range [0, 2]. Accepts BLOCK / DAMAGE / POISON.
func checkHitType(v int, op string) error {
	if v < 0 || v >= objtype.HitTypeCount {
		return fmt.Errorf("%s: hit type out of range (%d)", op, v)
	}
	return nil
}

// checkQueue validates an AI-queue identifier against the corrected
// closed range [1, 20] inclusive. Deliberate deviation from TS
// ScriptValidators.ts:114 (which uses [0, 19]) — TS's range combined
// with the call-site `+queueId-1` arithmetic at NPC_QUEUE /
// NPC_WALKTRIGGER admitted queueId=0 producing `TriggerAiQueue1 - 1`
// (a garbage trigger one below AI_QUEUE1). The corrected range
// matches actual LostCityRS/Content script usage (real first-args
// ∈ {1..7, 10, 11, 12} for npc_queue, {8} for npc_walktrigger; no
// script uses 0 or 20) and admits the previously-unreachable
// queueId=20 → TriggerAiQueue20.
//
// Per goscape convention (cf. Player.Damage negative-amount clamp at
// modules/world/player_masks.go), deliberate-deviation-for-correctness
// is documented inline; no formal NAI-XXX-D-* pin is opened.
//
// PORTING-EXCEPTION (M14, queue-range): KEEP [1,20]. The audit's ⚠verify is
// resolved — Content npc_queue first-args are {1..7,10,11,12} and
// npc_walktrigger {8} (re-confirmed 2026-05-24); neither 0 nor 20 is used. TS's
// [0,19] leaves TriggerAiQueue20 unreachable and admits a garbage AI_QUEUE0, so
// matching it would reintroduce a bug. Do NOT regress to [0,19]. See PORTING.md.
func checkQueue(v int, op string) error {
	if v < 1 || v > 20 {
		return fmt.Errorf("%s: queue id out of range (%d)", op, v)
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

// checkCategoryType mirrors TS CategoryTypeValid (ScriptValidators.ts:123)
// — ScriptInputConfigTypeValidator(CategoryType.get, 0 <= n < count),
// collapsed into a single Configs.CategoryType(id) nil check per the
// checkNpcType / checkHuntType pattern. Used by NPC_FINDCAT
// (NpcOps.ts:373) and INV_TOTALCAT (InvOps.ts:638).
func checkCategoryType(s *ScriptState, id int, op string) error {
	if s.Configs == nil || s.Configs.CategoryType(id) == nil {
		return fmt.Errorf("%s: no CategoryType with value (%d) found", op, id)
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
// operand-resolved active NPC slot is unbound. Mirrors TS
// `checkedHandler(ActiveNpc, ...)` → `pointerCheck(ActiveNpc[intOperand])`
// (ScriptPointer.ts:52): operand 0 checks the primary slot, operand 1 the
// secondary. All NPC_* read/mutate handlers gate on this, then read the same
// operand-resolved npc via s.activeNpc().
func requireActiveNpc(s *ScriptState, op string) error {
	if s.activeNpc() == nil {
		return fmt.Errorf("%s: %w", op, ErrNoActiveNpc)
	}
	return nil
}

// handleNpcType pushes the ActiveNpc's NpcType id. Mirrors TS
// NpcOps.ts:259-261: pushInt(check(activeNpc.type, NpcTypeValid).id).
func handleNpcType(s *ScriptState) error {
	if err := requireActiveNpc(s, "NPC_TYPE"); err != nil {
		return err
	}
	if err := requireConfigs(s, "NPC_TYPE"); err != nil {
		return err
	}
	id := s.activeNpc().NpcType()
	if err := checkNpcType(s, id, "NPC_TYPE"); err != nil {
		return err
	}
	s.PushInt(id)
	return nil
}

// handleNpcCoord pushes the packed RS2 coord of the ActiveNpc:
// (level<<28) | (x<<14) | z.
func handleNpcCoord(s *ScriptState) error {
	if err := requireActiveNpc(s, "NPC_COORD"); err != nil {
		return err
	}
	n := s.activeNpc()
	s.PushInt((n.NpcLevel() << 28) | (n.NpcX() << 14) | n.NpcZ())
	return nil
}

// handleNpcStat pops a stat id and pushes the NPC's current (boosted)
// level for that stat. Mirrors TS NpcOps.ts NPC_STAT —
// check(state.popInt(), NpcStatValid). Goscape mirrors via
// checkNpcStatID.
func handleNpcStat(s *ScriptState) error {
	if err := requireActiveNpc(s, "NPC_STAT"); err != nil {
		return err
	}
	stat := s.PopInt()
	if err := checkNpcStatID(stat, "NPC_STAT"); err != nil {
		return err
	}
	s.PushInt(s.activeNpc().NpcStat(stat))
	return nil
}

// handleNpcBaseStat pops a stat id and pushes the NPC's base level.
// Mirrors TS NpcOps.ts NPC_BASESTAT — check(state.popInt(),
// NpcStatValid). Goscape mirrors via checkNpcStatID.
func handleNpcBaseStat(s *ScriptState) error {
	if err := requireActiveNpc(s, "NPC_BASESTAT"); err != nil {
		return err
	}
	stat := s.PopInt()
	if err := checkNpcStatID(stat, "NPC_BASESTAT"); err != nil {
		return err
	}
	s.PushInt(s.activeNpc().NpcBaseStat(stat))
	return nil
}

// handleNpcName looks up the ActiveNpc's NpcType via Configs and pushes
// its Name, or "null" if empty (matching TS nullish-coalesce on
// NpcType.name).
// Mirrors TS NpcOps.ts:270-272 — check(activeNpc.type, NpcTypeValid).
func handleNpcName(s *ScriptState) error {
	if err := requireActiveNpc(s, "NPC_NAME"); err != nil {
		return err
	}
	if err := requireConfigs(s, "NPC_NAME"); err != nil {
		return err
	}
	typeID := s.activeNpc().NpcType()
	if err := checkNpcType(s, typeID, "NPC_NAME"); err != nil {
		return err
	}
	name := s.Configs.NpcType(typeID).Name
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
	cfg := s.Configs.NpcType(s.activeNpc().NpcType())
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
	s.PushInt(s.activeNpc().NpcUID())
	return nil
}

// handleNpcCategory looks up the ActiveNpc's NpcType via Configs and
// pushes its Category. Mirrors TS NpcOps.ts:68-70 —
// check(activeNpc.type, NpcTypeValid).category (no fallback).
func handleNpcCategory(s *ScriptState) error {
	if err := requireActiveNpc(s, "NPC_CATEGORY"); err != nil {
		return err
	}
	if err := requireConfigs(s, "NPC_CATEGORY"); err != nil {
		return err
	}
	typeID := s.activeNpc().NpcType()
	if err := checkNpcType(s, typeID, "NPC_CATEGORY"); err != nil {
		return err
	}
	s.PushInt(s.Configs.NpcType(typeID).Category)
	return nil
}

// handleNpcSay pops a string and sets it as the active NPC's speech
// bubble for this tick. Empty strings are legal (clears the bubble).
func handleNpcSay(s *ScriptState) error {
	if err := requireActiveNpc(s, "NPC_SAY"); err != nil {
		return err
	}
	text := s.PopString()
	s.activeNpc().Say([]byte(text))
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
	s.activeNpc().Animate(id, delay)
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
	s.activeNpc().FaceCoord(x, z)
	return nil
}

// handleNpcChangeType pops (newType, duration) in TS order (duration
// on top) and morphs the NPC. Matches TS NpcOps.ts:457-462 including
// the check(id, NpcTypeValid) registry-presence gate at :459.
// The full body (guard + typeId/uid/mask + stats-reset +
// lifecycleTick fast-path) lives in *Npc.changeTypeImpl.
func handleNpcChangeType(s *ScriptState) error {
	if err := requireActiveNpc(s, "NPC_CHANGETYPE"); err != nil {
		return err
	}
	duration := s.PopInt()
	newType := s.PopInt()
	if err := requireConfigs(s, "NPC_CHANGETYPE"); err != nil {
		return err
	}
	if err := checkNpcType(s, newType, "NPC_CHANGETYPE"); err != nil {
		return err
	}
	s.activeNpc().ChangeType(newType, duration)
	return nil
}

// handleNpcChangeTypeKeepAll pops (newType, duration) in TS order
// (duration on top) and morphs the NPC preserving all current stats.
// Matches TS NpcOps.ts:465-471 including the check(id, NpcTypeValid)
// registry-presence gate at :467.
func handleNpcChangeTypeKeepAll(s *ScriptState) error {
	if err := requireActiveNpc(s, "NPC_CHANGETYPE_KEEPALL"); err != nil {
		return err
	}
	duration := s.PopInt()
	newType := s.PopInt()
	if err := requireConfigs(s, "NPC_CHANGETYPE_KEEPALL"); err != nil {
		return err
	}
	if err := checkNpcType(s, newType, "NPC_CHANGETYPE_KEEPALL"); err != nil {
		return err
	}
	s.activeNpc().ChangeTypeKeepAll(newType, duration)
	return nil
}

// handleNpcDamage pops (type, amount) in TS order (amount on top) and
// applies damage. The concrete Npc impl manages HP; this handler stays thin.
// Mirrors TS NpcOps.ts NPC_DAMAGE: check(amount, NumberNotNull) +
// check(dmgType, HitTypeValid). Goscape mirrors via checkNotNull +
// checkHitType.
func handleNpcDamage(s *ScriptState) error {
	if err := requireActiveNpc(s, "NPC_DAMAGE"); err != nil {
		return err
	}
	amount := s.PopInt()
	if err := checkNotNull(amount, "NPC_DAMAGE"); err != nil {
		return err
	}
	dmgType := s.PopInt()
	if err := checkHitType(dmgType, "NPC_DAMAGE"); err != nil {
		return err
	}
	s.activeNpc().Damage(amount, dmgType)
	return nil
}

// handleNpcDel (NPC_DEL, opcode 2508) removes the active NPC. The
// duration passed to World.RemoveNpc is the active NPC type's
// respawnrate; Server.removeNpc scales it by player count and writes
// it to lifecycleTick (RESPAWN-lifecycle) or, on DESPAWN-lifecycle,
// releases the registry slot and runs Cleanup (NAI-19; see
// modules/world/npc_registry.go).
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
	s.World.RemoveNpc(s.activeNpc(), s.activeNpc().Respawnrate())
	return nil
}

// handleNpcDelay (NPC_DELAY, opcode 2509) suspends the active NPC's
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
	s.activeNpc().SetDelayed(ticks)
	s.Execution = NpcSuspended
	return nil
}

// handleNpcArriveDelay implements NPC_ARRIVEDELAY (opcode 2546): if the
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
		return fmt.Errorf("NPC_ARRIVEDELAY: %w", ErrNoWorld)
	}
	last := s.activeNpc().LastMovement()
	tick := s.World.CurrentTick()
	if last < tick-1 {
		return nil
	}
	if last == tick-1 {
		s.activeNpc().SetDelayed(0) // delayedUntil = T+1
	} else {
		s.activeNpc().SetDelayed(1) // delayedUntil = T+2
	}
	s.Execution = NpcSuspended
	return nil
}

// handleNpcQueue (NPC_QUEUE, opcode 2524) enqueues an ai_queueN
// dispatch on the active NPC. Pop order: delay (top), arg, queueId
// (bottom). queueId ∈ [1, 20] (goscape deviation from TS-literal —
// see checkQueue doc) maps to TriggerAiQueue1..20 via arithmetic:
// trigger = TriggerAiQueue1 + queueId - 1. Mirrors TS NpcOps.ts:144-150,
// including the NumberNotNull check on delay (closed in NAI-20). The
// arg pop is unwrapped per TS.
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
	if err := checkQueue(queueID, "NPC_QUEUE"); err != nil {
		return err
	}
	trigger := TriggerAiQueue1 + ServerTriggerType(queueID-1)
	s.activeNpc().EnqueueScriptForTrigger(trigger, delay, lastIntArg)
	return nil
}

// handleNpcSetTimer (NPC_SETTIMER, opcode 2534) sets the active
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
	s.activeNpc().SetTimer(interval)
	return nil
}

// handleNpcTele (NPC_TELE, opcode 2539) teleports the active NPC to
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
	s.activeNpc().Teleport(x, z, level)
	return nil
}

// handleNpcWalk (NPC_WALK, opcode 2543) queues a single waypoint for the
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
	s.activeNpc().QueueWaypoint(x, z)
	return nil
}

// handleNpcWalkTrigger (NPC_WALKTRIGGER, opcode 2533) sets a deferred
// AI-queue trigger and arg on the active NPC; the trigger fires when
// the NPC completes a walk step. Pop order: arg (top), queueID
// (bottom). queueId ∈ [1, 20] (goscape deviation from TS-literal —
// see checkQueue doc); then queueId-1 mirrors TS NpcOps.ts:488 storage
// (walktrigger = queueId - 1, so the stored field is 0-indexed
// 0..19). Mirrors TS NpcOps.ts:483-490. The walktrigger consumer
// fires from (*Npc).updateMovement (modules/world/npc_interaction.go,
// NAI-51 T2.1).
func handleNpcWalkTrigger(s *ScriptState) error {
	if err := requireActiveNpc(s, "NPC_WALKTRIGGER"); err != nil {
		return err
	}
	arg := s.PopInt()
	queueID := s.PopInt()
	if err := checkQueue(queueID, "NPC_WALKTRIGGER"); err != nil {
		return err
	}
	s.activeNpc().SetWalkTrigger(queueID - 1)
	s.activeNpc().SetWalkTriggerArg(arg)
	return nil
}

// handleNpcGetMode (NPC_GETMODE, opcode 2520) pushes the active NPC's
// targetOp value (the mode set by NPC_SETMODE / interaction binding).
// Mirrors TS NpcOps.ts:473-475 — checkedHandler(ActiveNpc) + pushInt.
func handleNpcGetMode(s *ScriptState) error {
	if err := requireActiveNpc(s, "NPC_GETMODE"); err != nil {
		return err
	}
	s.PushInt(s.activeNpc().TargetOp())
	return nil
}

// handleNpcSetMode (NPC_SETMODE, opcode 2532) sets the active NPC's mode
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
		s.activeNpc().ClearInteraction()
		s.activeNpc().SetTargetOp(mode)
		if mode == objtype.NPCModePatrol {
			s.activeNpc().ClearPatrol()
		}
		return nil
	}

	// Branch 2: NULL → resetDefaults.
	if mode == objtype.NPCModeNull {
		s.activeNpc().ResetDefaults()
		return nil
	}

	// Branch 3: target-binding modes.
	s.activeNpc().SetTargetOp(mode)

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
		s.activeNpc().ResetDefaults()
		return nil
	}
	s.activeNpc().SetInteractionScript(target, mode)
	return nil
}

// handleNpcSetHunt (NPC_SETHUNT, opcode 2530) sets the NPC's hunt
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
	s.activeNpc().SetHuntRange(huntRange)
	return nil
}

// handleNpcSetHuntMode (NPC_SETHUNTMODE, opcode 2531) sets the NPC's
// HuntType id. -1 clears the hunt mode (valid input, bypasses the
// registry check). Any other id is validated against Configs.HuntType
// before being assigned — an unknown id aborts the script. Mirrors TS
// NpcOps.ts:178-186 (check(huntTypeId, HuntTypeValid)).
func handleNpcSetHuntMode(s *ScriptState) error {
	if err := requireActiveNpc(s, "NPC_SETHUNTMODE"); err != nil {
		return err
	}
	hid := s.PopInt()
	if hid != -1 {
		if err := checkHuntType(s, hid, "NPC_SETHUNTMODE"); err != nil {
			return err
		}
	}
	s.activeNpc().SetHuntMode(hid)
	return nil
}

// handleNpcFind (NPC_FIND, opcode 2511) pops (coord, npc, distance,
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

// handleNpcFindCat (NPC_FINDCAT, opcode 2512) pops (coord, category,
// distance, huntvis). Same spine as handleNpcFind but filter is by
// NpcType.Category == category (handled in the world-side impl).
// Mirrors TS NpcOps.ts:369-400.
func handleNpcFindCat(s *ScriptState) error {
	checkVis := s.PopInt()
	distance := s.PopInt()
	category := s.PopInt()
	coord := s.PopInt()

	level, x, z, err := checkCoord(coord, "NPC_FINDCAT")
	if err != nil {
		return err
	}
	if err := checkCategoryType(s, category, "NPC_FINDCAT"); err != nil {
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

// handleNpcFindExact (NPC_FINDEXACT, opcode 2515) pops (coord, npcType).
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

// handleNpcFindAllAny (NPC_FINDALLANY, opcode 2513) pops (coord, distance,
// huntvis), validates, and stores a DISTANCE-mode NpcIterator on
// state.npcIterator with no type filter. Mirrors TS NpcOps.ts:403-411.
// Pointer-set is `set ['find_npc']` (ScriptOpcodePointers.ts:586-588);
// goscape encodes the find_npc pointer as state.npcIterator != nil.
// No push (TS doesn't push either). huntvis filtering is active per TS
// ScriptIterators.ts:348-352 (Distance-mode LoS/LoW consumed by
// passesFilter); s.LineValidator is plumbed into the iterator.
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
		s.Npcs, s.LineValidator, s.World.CurrentTick(),
		level, x, z, distance, checkVis, -1,
	)
	return nil
}

// handleNpcFindAll (NPC_FINDALL, opcode 2514) pops (coord, npc, distance,
// huntvis), validates, and stores a DISTANCE-mode NpcIterator with
// typeID set to filter by NPC type. Mirrors TS NpcOps.ts:413-422.
// Pop order matches TS popInts(4): top → bottom = checkVis, distance,
// npcTypeID, coord. huntvis filtering is active per TS
// ScriptIterators.ts:348-352; s.LineValidator is plumbed into the
// iterator.
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
		s.Npcs, s.LineValidator, s.World.CurrentTick(),
		level, x, z, distance, checkVis, npcTypeID,
	)
	return nil
}

// handleNpcFindAllZone (NPC_FINDALLZONE, opcode 2517) pops a coord,
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

// handleNpcHuntAll (NPC_HUNTALL, opcode 2528) pops [coord, distance,
// huntvis] and stores a HuntAll-mode NpcIterator in s.huntIterator
// (consumed by NPC_HUNTNEXT 2529). Mirrors TS ServerOps.ts:114-122
// at pin 9aadcec4. NPC_FINDNEXT (which reads npcIterator) no longer
// sees these results — the split is intentional (rev-244 B4).
//
// Pop order (top-of-stack first): huntvis, distance, coord.
// Validation: checkCoord, checkNotNull(distance), checkHuntVis.
// Nil-Npcs degrades silently (matches NPC_FINDALL convention).
//
// NAI-35-T3: HuntAll-mode iterator activated LoS/LoW filtering at the
// passesFilter HuntAll branch.
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
	s.huntIterator = NewHuntAllNpcIterator(
		s.Npcs, s.LineValidator, s.Configs, s.World.CurrentTick(),
		level, x, z, distance, checkVis,
	)
	return nil
}

// handleNpcHuntNext (NPC_HUNTNEXT, opcode 2529) advances the unified hunt
// iterator and binds the next NPC to the operand-selected active slot.
// Mirrors TS ServerOps.ts:124-138 at pin 9aadcec4.
//
// TS drives next() BEFORE checking instanceof Npc (ServerOps.ts:125-135):
// an exhausted iterator's done-branch pushes 0 regardless of the iterator
// type; only a YIELDED non-Npc value trips the instanceof throw.
// Stale-before-Next stays per iterator_state_pattern.md element 3.
//
// Exhaustion does NOT clear s.huntIterator (mirrors NPC_FINDNEXT semantics
// and the HUNTNEXT convention at iterator_state_pattern.md element 7).
func handleNpcHuntNext(s *ScriptState) error {
	switch it := s.huntIterator.(type) {
	case nil:
		// TS ServerOps.ts:125-129 — nil iterator → !result → push 0.
		s.PushInt(0)
		return nil
	case *NpcIterator:
		if it.Stale(s.World.CurrentTick()) {
			return fmt.Errorf("NPC_HUNTNEXT: tried to use an old iterator. Create a new iterator instead.")
		}
		npc, ok := it.Next()
		if !ok {
			s.PushInt(0)
			return nil
		}
		setActiveNpcSlot(s, npc)
		s.PushInt(1)
		return nil
	case *PlayerIterator:
		// TS drives next() BEFORE the instanceof guard (ServerOps.ts:125-135):
		// an exhausted iterator pushes 0 (done-branch short-circuits before
		// instanceof); only a YIELDED wrong-type value trips the throw.
		if it.Stale(s.World.CurrentTick()) {
			return fmt.Errorf("NPC_HUNTNEXT: tried to use an old iterator. Create a new iterator instead.")
		}
		if _, ok := it.Next(); !ok {
			s.PushInt(0)
			return nil
		}
		return fmt.Errorf("NPC_HUNTNEXT: command must result instance of Npc") // TS ServerOps.ts:132
	default:
		return fmt.Errorf("NPC_HUNTNEXT: unknown hunt iterator type %T", it)
	}
}

// handleNpcHunt (NPC_HUNT, opcode 2527) pops [coord, distance, huntvis] and
// selects the closest NPC by euclidean² distance from a HuntAll-mode
// iterator over zone-sweep candidates, then sets ActiveNpc + pushes 1. On
// empty iterator (no candidates), nil-Npcs, or no in-range NPCs, pushes 0.
// Mirrors TS ServerOps.ts:79-110 at pin 9aadcec4.
//
// Pop order (top first): huntvis, distance, coord.
// Validation: checkCoord, checkNotNull(distance), checkHuntVis.
// Tie-break: TS uses `<=` (NpcOps.ts:307), so later iterator yields win
// equidistant comparisons; pinned by TestHandleNpcHunt_TieBreak_*.
//
// Iterator lifetime: LOCAL to this handler — not stored in s.npcIterator
// (unlike NPC_HUNTALL which exposes its iterator to a subsequent
// NPC_FINDNEXT). Stale-check matches handleNpcFindNext convention.
//
// NAI-163 B2.
func handleNpcHunt(s *ScriptState) error {
	huntvis := s.PopInt()
	distance := s.PopInt()
	coord := s.PopInt()

	level, x, z, err := checkCoord(coord, "NPC_HUNT")
	if err != nil {
		return err
	}
	if err := checkNotNull(distance, "NPC_HUNT"); err != nil {
		return err
	}
	if err := checkHuntVis(huntvis, "NPC_HUNT"); err != nil {
		return err
	}

	if s.Npcs == nil {
		s.PushInt(0)
		return nil
	}

	tick := s.World.CurrentTick()
	it := NewHuntAllNpcIterator(s.Npcs, s.LineValidator, s.Configs, tick, level, x, z, distance, huntvis)

	var closest ActiveNpc
	closestDist := math.MaxInt
	for {
		if it.Stale(s.World.CurrentTick()) {
			return fmt.Errorf("NPC_HUNT: tried to use an old iterator. Create a new iterator instead.")
		}
		npc, ok := it.Next()
		if !ok {
			break
		}
		dx := npc.NpcX() - x
		dz := npc.NpcZ() - z
		d := dx*dx + dz*dz
		if d <= closestDist {
			closest = npc
			closestDist = d
		}
	}

	if closest == nil {
		s.PushInt(0)
		return nil
	}
	setActiveNpcSlot(s, closest)
	s.PushInt(1)
	return nil
}

// handleNpcFindNext (NPC_FINDNEXT, opcode 2518) advances the active
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

// handleNpcFindUID (NPC_FINDUID, opcode 2519) pops a packed NPC UID and
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

// handleNpcRange (NPC_RANGE, opcode 2525) pops a packed coord and pushes the
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
// Multi-tile NPCs (size > 1): goscape ports the full TS
// CoordGrid.distanceTo + CoordGrid.closest semantics (CoordGrid.ts:60-72)
// — clamp the target cell into the NPC's occupied footprint
// [(npc.x, npc.z) .. (npc.x + npc.width - 1, npc.z + npc.length - 1)]
// and take the max-absolute-axis delta from the clamped point. For
// size=1 NPCs (width = length = 1), occupiedX/Z collapse to npc.x/z
// and the formula reduces to origin-based Chebyshev (byte-identical
// to the size=1 prior behavior). Closes the NAI-120 Bundle 1 audit
// section 6 deferral per docs/superpowers/specs/2026-05-21-npc-range-
// size-aware-distance-design.md.
func handleNpcRange(s *ScriptState) error {
	if err := requireActiveNpc(s, "NPC_RANGE"); err != nil {
		return err
	}
	coord := s.PopInt()
	level, x, z, err := checkCoord(coord, "NPC_RANGE")
	if err != nil {
		return err
	}
	n := s.activeNpc()
	if level != n.NpcLevel() {
		s.PushInt(-1)
		return nil
	}
	// Closest-edge Chebyshev per TS CoordGrid.distanceTo + closest
	// (CoordGrid.ts:60-72): clamp the target cell into the NPC's
	// occupied footprint, then take the max-absolute-axis delta. For
	// size=1 NPCs (width=length=1), occupiedX = n.NpcX() and the
	// formula collapses to the prior origin-Chebyshev form
	// (byte-identical).
	nx := n.NpcX()
	nz := n.NpcZ()
	occupiedX := nx + n.NpcWidth() - 1
	occupiedZ := nz + n.NpcLength() - 1

	clampedX := x
	if x < nx {
		clampedX = nx
	} else if x > occupiedX {
		clampedX = occupiedX
	}
	clampedZ := z
	if z < nz {
		clampedZ = nz
	} else if z > occupiedZ {
		clampedZ = occupiedZ
	}

	dx := clampedX - x
	if dx < 0 {
		dx = -dx
	}
	dz := clampedZ - z
	if dz < 0 {
		dz = -dz
	}
	s.PushInt(max(dx, dz))
	return nil
}

// handleNpcStatAdd (NPC_STATADD, opcode 2536) boosts the active NPC's stat.
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
	base := s.activeNpc().NpcBaseStat(stat)
	cur := s.activeNpc().NpcStat(stat)
	added := cur + (constant + (base*percent)/100)
	added = min(added, 255)
	s.activeNpc().SetNpcStat(stat, added)
	return nil
}

// handleNpcStatSub (NPC_STATSUB, opcode 2538) drains the active NPC's stat.
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
	base := s.activeNpc().NpcBaseStat(stat)
	cur := s.activeNpc().NpcStat(stat)
	subbed := cur - (constant + (base*percent)/100)
	subbed = max(subbed, 0)
	s.activeNpc().SetNpcStat(stat, subbed)
	return nil
}

// handleSpotAnimNpc (SPOTANIM_NPC, opcode 2542) queues a spotanim on the
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
	s.activeNpc().PlaySpotAnim(id, height, delay)
	return nil
}

// handleNpcHeroPoints (NPC_HEROPOINTS, opcode 2521) credits the active
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
	s.activeNpc().AddHeroPoints(s.activePlayer().UID(), amount)
	return nil
}

// handleNpcFindHero (NPC_FINDHERO, opcode 2516) returns the player
// with the largest HeroPoints credit on this NPC's ledger and binds
// them to the primary or secondary active-player slot per IntOperand.
// Pushes 1 on success, 0 if the ledger is empty, the resolved player
// has logged out, or s.World is nil. Mirrors TS NpcOps.ts:114-130 —
// state.activePlayer setter behavior at ScriptState.ts:235-241
// routes to Self (primary) or Self2 (secondary) based on intOperand.
//
// DEVIATION-NAI-127-D1: defensive nil-s.World guard (goscape defensive;
// TS skips this check). Mirrors handleNpcDel from NAI-126. Retire when
// an upstream invariant proves s.World non-nil for any executing
// script.
func handleNpcFindHero(s *ScriptState) error {
	if err := requireActiveNpc(s, "NPC_FINDHERO"); err != nil {
		return err
	}
	pushed := 0
	var topUID int
	lookupNonNil := false
	defer func() {
		if s.NodeDebug && s.Log != nil {
			s.Log.Info("nai128.npc.findhero",
				"topUID", topUID,
				"lookupNonNil", lookupNonNil,
				"pushed", pushed,
			)
		}
	}()
	if s.World == nil {
		s.PushInt(0)
		return nil
	}
	topUID = s.activeNpc().TopContributor()
	if topUID == 0 {
		s.PushInt(0)
		return nil
	}
	player := s.World.LookupPlayerByUID(topUID)
	if player == nil {
		s.PushInt(0)
		return nil
	}
	lookupNonNil = true
	if s.Script.IntOperands[s.PC] == 0 {
		s.Self = player
		s.Pointers |= PtrActivePlayer
	} else {
		s.Self2 = player
		s.Pointers |= PtrActivePlayer2
	}
	s.PushInt(1)
	pushed = 1
	return nil
}

// handleNpcInRange implements OpNpcInRange (TS NPC_INRANGE at
// NpcOps.ts:556-558). Calls ActiveNpc.TargetWithinMaxRange() and pushes
// 0/1. NAI-160 T7.
func handleNpcInRange(s *ScriptState) error {
	if err := requireActiveNpc(s, "NPC_INRANGE"); err != nil {
		return err
	}
	if s.activeNpc().TargetWithinMaxRange() {
		s.PushInt(1)
	} else {
		s.PushInt(0)
	}
	return nil
}

// handleNpcAttackRange implements OpNpcAttackRange (TS NPC_ATTACKRANGE at
// NpcOps.ts:521-523). Reads the active NPC's type, validates via
// checkNpcType, pushes NpcType.AttackRange widened from uint16 to int
// (deviation NAI-160-D-NPC-ATTACKRANGE-WIDEN — value-faithful, width is
// Go-side artifact). NAI-160 T6.
func handleNpcAttackRange(s *ScriptState) error {
	if err := requireActiveNpc(s, "NPC_ATTACKRANGE"); err != nil {
		return err
	}
	typeID := s.activeNpc().NpcType()
	if err := checkNpcType(s, typeID, "NPC_ATTACKRANGE"); err != nil {
		return err
	}
	s.PushInt(int(s.Configs.NpcType(typeID).AttackRange))
	return nil
}

// handleNpcStatHeal (NPC_STATHEAL, opcode 2537) heals the active NPC's
// stat by `constant + (base*percent/100)`, capped at base. When the
// healed value reaches base and the stat is HITPOINTS, the NPC's HeroPoints
// ledger is cleared. Mirrors TS NpcOps.ts:241-257.
//
// Pop order (LIFO): percent (top), constant, stat (bottom) — TS popInts(3)
// returns [stat, constant, percent].
func handleNpcStatHeal(s *ScriptState) error {
	if err := requireActiveNpc(s, "NPC_STATHEAL"); err != nil {
		return err
	}
	percent := s.PopInt()
	constant := s.PopInt()
	stat := s.PopInt()
	if err := checkNpcStatID(stat, "NPC_STATHEAL"); err != nil {
		return err
	}
	if err := checkNotNull(constant, "NPC_STATHEAL"); err != nil {
		return err
	}
	if err := checkNotNull(percent, "NPC_STATHEAL"); err != nil {
		return err
	}
	base := s.activeNpc().NpcBaseStat(stat)
	cur := s.activeNpc().NpcStat(stat)
	healed := cur + (constant + (base*percent)/100) // TS `| 0` ≡ Go int truncation
	if healed > base {
		healed = base
	}
	s.activeNpc().SetNpcStat(stat, healed)
	if stat == objtype.NpcStatHitpoints && healed >= base {
		s.activeNpc().HeroPointsClear()
	}
	return nil
}
