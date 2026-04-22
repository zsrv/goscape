package script

import (
	"errors"
	"fmt"
)

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
func handleNpcHasOp(s *ScriptState) error {
	if err := requireActiveNpc(s, "NPC_HASOP"); err != nil {
		return err
	}
	op := s.PopInt()
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
func handleNpcAnim(s *ScriptState) error {
	if err := requireActiveNpc(s, "NPC_ANIM"); err != nil {
		return err
	}
	delay := s.PopInt()
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

// handleNpcChangeType pops (newType, duration) in TS order (duration on
// top) and morphs the NPC. S6c discards duration — timed revert is
// deferred to a future AI sub-spec.
func handleNpcChangeType(s *ScriptState) error {
	if err := requireActiveNpc(s, "NPC_CHANGETYPE"); err != nil {
		return err
	}
	_ = s.PopInt() // duration; see spec S6c Gotchas
	newType := s.PopInt()
	s.ActiveNpc.ChangeType(newType)
	return nil
}

// handleNpcDamage pops (type, amount) in TS order (amount on top) and
// applies damage. The concrete Npc impl manages HP; this handler stays thin.
func handleNpcDamage(s *ScriptState) error {
	if err := requireActiveNpc(s, "NPC_DAMAGE"); err != nil {
		return err
	}
	amount := s.PopInt()
	dmgType := s.PopInt()
	s.ActiveNpc.Damage(amount, dmgType)
	return nil
}

// handleNpcDelay (NPC_DELAY, opcode 2511) suspends the active NPC's
// script for N ticks. Transitions the script to NpcSuspended and
// records the wake tick on the NPC via SetDelayed. The tick loop
// resumes the script from Npc.turn() when delayedUntil expires.
// Mirrors TS NpcOps.ts NPC_DELAY.
func handleNpcDelay(s *ScriptState) error {
	if err := requireActiveNpc(s, "NPC_DELAY"); err != nil {
		return err
	}
	ticks := s.PopInt()
	s.ActiveNpc.SetDelayed(ticks)
	s.Execution = NpcSuspended
	return nil
}

// handleNpcQueue (NPC_QUEUE, opcode 2530) enqueues an ai_queueN
// dispatch on the active NPC. Pop order: delay (top), arg, queueId
// (bottom). queueId ∈ [1, 20] maps to TriggerAiQueue1..20 via
// arithmetic: trigger = TriggerAiQueue1 + queueId - 1. Mirrors TS
// NpcOps.ts:144-150.
func handleNpcQueue(s *ScriptState) error {
	if err := requireActiveNpc(s, "NPC_QUEUE"); err != nil {
		return err
	}
	delay := s.PopInt()
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
// NpcOps.ts:278-280. No NumberNotNull check — tracked as future
// fidelity-audit item in nai_followups memory.
func handleNpcSetTimer(s *ScriptState) error {
	if err := requireActiveNpc(s, "NPC_SETTIMER"); err != nil {
		return err
	}
	interval := s.PopInt()
	s.ActiveNpc.SetTimer(interval)
	return nil
}

// handleNpcSetHunt (NPC_SETHUNT, opcode 2533) sets the NPC's hunt
// search range. Despite the opcode name, this sets RANGE only —
// hunt mode is set via the separate NPC_SETHUNTMODE opcode.
// Mirrors TS NpcOps.ts:174-176.
func handleNpcSetHunt(s *ScriptState) error {
	if err := requireActiveNpc(s, "NPC_SETHUNT"); err != nil {
		return err
	}
	s.ActiveNpc.SetHuntRange(s.PopInt())
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
