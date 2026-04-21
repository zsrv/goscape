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
