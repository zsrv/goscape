package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/script"
)

func TestAddXPNormalGainNoLevelUp(t *testing.T) {
	p, _ := newTestPlayer(t)
	// Level 2 threshold = GetExpByLevel(2) = 820.
	// Start at 820 (exactly level 2); adding 100 → 920, still below level-3
	// threshold (GetExpByLevel(3) = 1740), so baseLevels stays 2.
	p.stats[objtype.PlayerStatAttack] = int32(objtype.GetExpByLevel(2))
	p.baseLevels[objtype.PlayerStatAttack] = 2
	p.levels[objtype.PlayerStatAttack] = 2
	p.AddXP(objtype.PlayerStatAttack, 100)
	want := int32(objtype.GetExpByLevel(2)) + 100
	if p.stats[objtype.PlayerStatAttack] != want {
		t.Errorf("stats: got %d, want %d", p.stats[objtype.PlayerStatAttack], want)
	}
	// Still level 2 (level 3 threshold is GetExpByLevel(3) = 1740).
	if p.baseLevels[objtype.PlayerStatAttack] != 2 {
		t.Errorf("baseLevels: got %d, want 2", p.baseLevels[objtype.PlayerStatAttack])
	}
	if p.levels[objtype.PlayerStatAttack] != 2 {
		t.Errorf("levels: got %d, want 2 (no replenish)", p.levels[objtype.PlayerStatAttack])
	}
}

func TestAddXPLevelUpUnbuffedAdvancesLevels(t *testing.T) {
	// Un-buffed (levels == baseLevels) player levels up: TS advances BOTH
	// levels and baseLevels in lockstep so the stat display updates.
	// Matches TS Player.ts:1760-1763.
	p, _ := newTestPlayer(t)
	p.stats[objtype.PlayerStatAttack] = 800
	p.baseLevels[objtype.PlayerStatAttack] = 2
	p.levels[objtype.PlayerStatAttack] = 2  // un-buffed
	p.AddXP(objtype.PlayerStatAttack, 1000) // → 1800, crosses 1740 = level 3
	if p.baseLevels[objtype.PlayerStatAttack] != 3 {
		t.Errorf("baseLevels: got %d, want 3", p.baseLevels[objtype.PlayerStatAttack])
	}
	if p.levels[objtype.PlayerStatAttack] != 3 {
		t.Errorf("levels: got %d, want 3 (un-buffed, advanced with baseLevels)",
			p.levels[objtype.PlayerStatAttack])
	}
}

func TestAddXPLevelUpWhileDrained(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.stats[objtype.PlayerStatAttack] = 800
	p.baseLevels[objtype.PlayerStatAttack] = 2
	p.levels[objtype.PlayerStatAttack] = 1  // drained below base
	p.AddXP(objtype.PlayerStatAttack, 1000) // → level 3
	if p.baseLevels[objtype.PlayerStatAttack] != 3 {
		t.Errorf("baseLevels: got %d, want 3", p.baseLevels[objtype.PlayerStatAttack])
	}
	// Replenish: levels + (afterBase - beforeBase) = 1 + (3 - 2) = 2.
	if p.levels[objtype.PlayerStatAttack] != 2 {
		t.Errorf("levels: got %d, want 2 (drained, replenished by 1)", p.levels[objtype.PlayerStatAttack])
	}
}

func TestAddXPMultiLevelUpUnbuffed(t *testing.T) {
	// Un-buffed player jumps 9 levels in one call: both stats advance in
	// lockstep to the new level. Matches TS Player.ts:1760-1763.
	p, _ := newTestPlayer(t)
	p.stats[objtype.PlayerStatAttack] = 0
	p.baseLevels[objtype.PlayerStatAttack] = 1
	p.levels[objtype.PlayerStatAttack] = 1   // un-buffed
	p.AddXP(objtype.PlayerStatAttack, 11540) // GetExpByLevel(10)
	if p.baseLevels[objtype.PlayerStatAttack] != 10 {
		t.Errorf("baseLevels: got %d, want 10", p.baseLevels[objtype.PlayerStatAttack])
	}
	if p.levels[objtype.PlayerStatAttack] != 10 {
		t.Errorf("levels: got %d, want 10 (un-buffed, advanced with baseLevels)",
			p.levels[objtype.PlayerStatAttack])
	}
}

func TestAddXPClampsAtMaxXP(t *testing.T) {
	// XP accumulation cap is MaxXP (200m real = 2B ×10), NOT MaxSkillXP
	// (the level-99 threshold, 13m real). Matches TS Player.ts:1754-1757.
	p, _ := newTestPlayer(t)
	p.stats[objtype.PlayerStatAttack] = int32(objtype.MaxXP - 10)
	p.baseLevels[objtype.PlayerStatAttack] = 99
	p.levels[objtype.PlayerStatAttack] = 99
	p.AddXP(objtype.PlayerStatAttack, 1000)
	if int(p.stats[objtype.PlayerStatAttack]) != objtype.MaxXP {
		t.Errorf("stats: got %d, want MaxXP %d",
			p.stats[objtype.PlayerStatAttack], objtype.MaxXP)
	}
	if p.baseLevels[objtype.PlayerStatAttack] != 99 {
		t.Errorf("baseLevels: got %d, want 99 (capped)", p.baseLevels[objtype.PlayerStatAttack])
	}
}

func TestAddXPAccumulatesPastLevel99ThresholdUpToMaxXP(t *testing.T) {
	// A level-99 player keeps accumulating XP past MaxSkillXP up to MaxXP.
	// Prestige / XP-chase gameplay depends on this.
	p, _ := newTestPlayer(t)
	p.stats[objtype.PlayerStatAttack] = int32(objtype.MaxSkillXP) // at level-99 threshold
	p.baseLevels[objtype.PlayerStatAttack] = 99
	p.levels[objtype.PlayerStatAttack] = 99
	p.AddXP(objtype.PlayerStatAttack, 1000000) // 100k real XP past level 99
	want := int32(objtype.MaxSkillXP + 1000000)
	if p.stats[objtype.PlayerStatAttack] != want {
		t.Errorf("stats: got %d, want %d (accumulation past level-99 threshold)",
			p.stats[objtype.PlayerStatAttack], want)
	}
	// Level stays at 99.
	if p.baseLevels[objtype.PlayerStatAttack] != 99 {
		t.Errorf("baseLevels: got %d, want 99", p.baseLevels[objtype.PlayerStatAttack])
	}
}

func TestAddXPBuffedLevelUpPreservesBuff(t *testing.T) {
	// Buffed player (levels > baseLevels, e.g. super-strength) levels up:
	// TS only advances baseLevels; levels stays (buff preserved).
	p, _ := newTestPlayer(t)
	p.stats[objtype.PlayerStatAttack] = 800
	p.baseLevels[objtype.PlayerStatAttack] = 2
	p.levels[objtype.PlayerStatAttack] = 5  // buffed by +3
	p.AddXP(objtype.PlayerStatAttack, 1000) // → level 3
	if p.baseLevels[objtype.PlayerStatAttack] != 3 {
		t.Errorf("baseLevels: got %d, want 3", p.baseLevels[objtype.PlayerStatAttack])
	}
	if p.levels[objtype.PlayerStatAttack] != 5 {
		t.Errorf("levels: got %d, want 5 (buff preserved across level-up)",
			p.levels[objtype.PlayerStatAttack])
	}
}

func TestAddXPNegativeClampsAtZero(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.stats[objtype.PlayerStatAttack] = 50
	p.baseLevels[objtype.PlayerStatAttack] = 1
	p.levels[objtype.PlayerStatAttack] = 1
	p.AddXP(objtype.PlayerStatAttack, -100) // would go negative
	if p.stats[objtype.PlayerStatAttack] != 0 {
		t.Errorf("stats: got %d, want 0 (negative clamped)", p.stats[objtype.PlayerStatAttack])
	}
	if p.baseLevels[objtype.PlayerStatAttack] != 1 {
		t.Errorf("baseLevels: got %d, want 1 (from 0 XP)", p.baseLevels[objtype.PlayerStatAttack])
	}
}

func TestAddXPOOBIsNoop(t *testing.T) {
	p, _ := newTestPlayer(t)
	var before [21]int32
	copy(before[:], p.stats[:])
	p.AddXP(-1, 100)
	p.AddXP(21, 100)
	p.AddXP(100, 100)
	for i := range 21 {
		if p.stats[i] != before[i] {
			t.Errorf("OOB AddXP mutated stats[%d]: got %d, want %d", i, p.stats[i], before[i])
		}
	}
}

func TestEnqueueScriptFileDirectPath(t *testing.T) {
	p, _ := newTestPlayer(t)
	sf := &script.ScriptFile{Name: "[test_direct]"}
	p.EnqueueScriptFile(sf, 3, 42, script.QueueNormal)
	if len(p.queue) != 1 {
		t.Fatalf("queue len: got %d, want 1", len(p.queue))
	}
	req := p.queue[0]
	if req.Script != sf {
		t.Errorf("queue[0].Script: got %v, want %v", req.Script, sf)
	}
	if req.Delay != 3 {
		t.Errorf("queue[0].Delay: got %d, want 3", req.Delay)
	}
	if req.IntArg != 42 {
		t.Errorf("queue[0].IntArg: got %d, want 42", req.IntArg)
	}
	if req.Type != script.QueueNormal {
		t.Errorf("queue[0].Type: got %v, want %v", req.Type, script.QueueNormal)
	}
}

func TestEnqueueScriptFileNilIsNoop(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.EnqueueScriptFile(nil, 0, 0, script.QueueNormal)
	if len(p.queue) != 0 {
		t.Errorf("queue len after nil enqueue: got %d, want 0", len(p.queue))
	}
}

func TestAddXPFiresChangeStatOnLevelUp(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	// Register [changestat,attack=0] — keyed by trigger(165) | (0x2<<8) | (0<<10).
	key := uint32(script.TriggerChangeStat) | (0x2 << 8) | (uint32(objtype.PlayerStatAttack) << 10)
	sf := &script.ScriptFile{
		Name:      "[changestat,attack]",
		LookupKey: key,
	}
	s.scriptProvider.Register(sf)

	p, _ := newTestPlayer(t)
	p.client.server = s
	p.stats[objtype.PlayerStatAttack] = 800
	p.baseLevels[objtype.PlayerStatAttack] = 2
	p.levels[objtype.PlayerStatAttack] = 2

	before := len(p.queue)
	p.AddXP(objtype.PlayerStatAttack, 1000) // → level 3

	if len(p.queue) != before+1 {
		t.Fatalf("queue len: got %d, want %d (+1 changestat)", len(p.queue), before+1)
	}
	req := p.queue[before]
	if req.Script != sf {
		t.Errorf("queue[%d].Script: got %v, want [changestat,attack] (%v)", before, req.Script, sf)
	}
	if req.Type != script.QueueNormal {
		t.Errorf("queue[%d].Type: got %v, want QueueNormal (TS ENGINE equivalent)", before, req.Type)
	}
}

func TestAddXPDoesNotFireChangeStatWithoutLevelUp(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	key := uint32(script.TriggerChangeStat) | (0x2 << 8) | (uint32(objtype.PlayerStatAttack) << 10)
	s.scriptProvider.Register(&script.ScriptFile{Name: "[changestat,attack]", LookupKey: key})

	p, _ := newTestPlayer(t)
	p.client.server = s
	p.stats[objtype.PlayerStatAttack] = 100 // below level-2 threshold (830)
	p.baseLevels[objtype.PlayerStatAttack] = 1
	p.levels[objtype.PlayerStatAttack] = 1

	before := len(p.queue)
	p.AddXP(objtype.PlayerStatAttack, 100) // → 200, still level 1 (< 830)

	if len(p.queue) != before {
		t.Errorf("queue len: got %d, want %d (no level-up = no changestat fire)",
			len(p.queue), before)
	}
}

func TestAddXPChangeStatNoScriptIsNoop(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider() // empty — no changestat script registered

	p, _ := newTestPlayer(t)
	p.client.server = s
	p.stats[objtype.PlayerStatAttack] = 800
	p.baseLevels[objtype.PlayerStatAttack] = 2
	p.levels[objtype.PlayerStatAttack] = 2

	before := len(p.queue)
	p.AddXP(objtype.PlayerStatAttack, 1000) // level up, but no script registered

	if len(p.queue) != before {
		t.Errorf("queue len: got %d, want %d (no registered script = silent no-op)",
			len(p.queue), before)
	}
	// Verify the level-up math still happened.
	if p.baseLevels[objtype.PlayerStatAttack] != 3 {
		t.Errorf("baseLevels: got %d, want 3 (level-up math independent of changeStat)",
			p.baseLevels[objtype.PlayerStatAttack])
	}
}
