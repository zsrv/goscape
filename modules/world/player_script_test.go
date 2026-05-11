package world

import (
	"bytes"
	"fmt"
	"slices"
	"testing"

	"github.com/zsrv/goscape/pkg/cache"
	"github.com/zsrv/goscape/pkg/gamemap"
	io2 "github.com/zsrv/goscape/pkg/io/isaac"
	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/rsbuf"
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
	p.EnqueueScriptFile(sf, 3, []int{42}, nil, script.QueueNormal)
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
	if !slices.Equal(req.IntArgs, []int{42}) {
		t.Errorf("queue[0].IntArgs: got %v, want [42]", req.IntArgs)
	}
	if req.StringArgs != nil {
		t.Errorf("queue[0].StringArgs: got %v, want nil", req.StringArgs)
	}
	if req.Type != script.QueueNormal {
		t.Errorf("queue[0].Type: got %v, want %v", req.Type, script.QueueNormal)
	}
}

func TestEnqueueScriptFileNilIsNoop(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.EnqueueScriptFile(nil, 0, nil, nil, script.QueueNormal)
	if len(p.queue) != 0 {
		t.Errorf("queue len after nil enqueue: got %d, want 0", len(p.queue))
	}
}

// TestUnlinkQueuedScriptDropsMatchingEntries pins the basic filter
// behavior: enqueue 3 scripts at distinct IDs, unlink the middle one,
// assert the remaining two are preserved in original order. NAI-161 T1.
func TestUnlinkQueuedScriptDropsMatchingEntries(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	sf10 := &script.ScriptFile{Name: "[test_id10]"}
	sf20 := &script.ScriptFile{Name: "[test_id20]"}
	sf30 := &script.ScriptFile{Name: "[test_id30]"}
	s.scriptProvider.Register(sf10) // id=0
	s.scriptProvider.Register(sf20) // id=1
	s.scriptProvider.Register(sf30) // id=2

	p, _ := newTestPlayer(t)
	p.client.server = s
	p.EnqueueScriptFile(sf10, 0, nil, nil, script.QueueNormal)
	p.EnqueueScriptFile(sf20, 0, nil, nil, script.QueueNormal)
	p.EnqueueScriptFile(sf30, 0, nil, nil, script.QueueNormal)

	p.UnlinkQueuedScript(1) // id=1 → sf20

	if len(p.queue) != 2 {
		t.Fatalf("queue len: got %d, want 2", len(p.queue))
	}
	if p.queue[0].Script != sf10 {
		t.Errorf("queue[0].Script: got %v, want sf10", p.queue[0].Script)
	}
	if p.queue[1].Script != sf30 {
		t.Errorf("queue[1].Script: got %v, want sf30", p.queue[1].Script)
	}
}

// TestUnlinkQueuedScriptWalksAllNonEngineTypes pins the TS-faithful
// default-NORMAL arm: walks BOTH queue and weakQueue, regardless of
// Type discriminator. Engine entries live in p.engineQueue (separate
// slice) and are untouched. NAI-161 T1 — deviation
// NAI-161-D-QUEUE-TYPE-MAPPING.
func TestUnlinkQueuedScriptWalksAllNonEngineTypes(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	sf10 := &script.ScriptFile{Name: "[test_id10]"}
	s.scriptProvider.Register(sf10) // id=0

	p, _ := newTestPlayer(t)
	p.client.server = s
	p.EnqueueScriptFile(sf10, 0, nil, nil, script.QueueNormal)
	p.EnqueueScriptFile(sf10, 0, nil, nil, script.QueueWeak)
	p.EnqueueScriptFile(sf10, 0, nil, nil, script.QueueStrong)
	p.EnqueueScriptFile(sf10, 0, nil, nil, script.QueueLong)

	p.UnlinkQueuedScript(0)

	if len(p.queue) != 0 {
		t.Errorf("queue len after unlink: got %d, want 0 (all 4 types should match)", len(p.queue))
	}
}

// TestUnlinkQueuedScriptLeavesEngineQueueIntact pins that the
// engineQueue (separate slice) is NOT walked by the default-NORMAL
// arm of unlinkQueuedScript. NAI-161 T1.
func TestUnlinkQueuedScriptLeavesEngineQueueIntact(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	sf10 := &script.ScriptFile{Name: "[test_id10]"}
	s.scriptProvider.Register(sf10) // id=0

	p, _ := newTestPlayer(t)
	p.client.server = s
	p.EnqueueScriptFile(sf10, 0, nil, nil, script.QueueNormal)
	p.EnqueueScriptFile(sf10, 0, nil, nil, script.QueueEngine)

	p.UnlinkQueuedScript(0)

	if len(p.queue) != 0 {
		t.Errorf("queue len: got %d, want 0 (normal entry should be dropped)", len(p.queue))
	}
	if len(p.engineQueue) != 1 {
		t.Errorf("engineQueue len: got %d, want 1 (engine entry must be preserved)", len(p.engineQueue))
	}
}

// TestUnlinkQueuedScriptNilServerIsNoop pins the defensive guard:
// a Player with no client.server (or no scriptProvider) does not
// panic and is a no-op. Mirrors EnqueueScriptArgs defensive shape at
// player_script.go:127. NAI-161 T1 — deviation
// NAI-161-D-CLEARQUEUE-NIL-PROVIDER.
func TestUnlinkQueuedScriptNilServerIsNoop(t *testing.T) {
	p, _ := newTestPlayer(t)
	// p.client.server is nil by default — newTestPlayer doesn't wire a Server.
	p.UnlinkQueuedScript(99)
	if len(p.queue) != 0 {
		t.Errorf("queue len: got %d, want 0", len(p.queue))
	}
}

// TestUnlinkQueuedScriptUnknownIDIsNoop pins TS-equivalent "scriptId
// has no matches → zero iterations": when GetByID returns nil for an
// out-of-range scriptID, the queue is unchanged. NAI-161 T1.
func TestUnlinkQueuedScriptUnknownIDIsNoop(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	sf10 := &script.ScriptFile{Name: "[test_id10]"}
	s.scriptProvider.Register(sf10) // id=0

	p, _ := newTestPlayer(t)
	p.client.server = s
	p.EnqueueScriptFile(sf10, 0, nil, nil, script.QueueNormal)

	p.UnlinkQueuedScript(99) // id=99 is out of range

	if len(p.queue) != 1 {
		t.Errorf("queue len: got %d, want 1 (bogus scriptID is no-op)", len(p.queue))
	}
}

// TestQueueCountIncludesAllQueueTypes pins TS GETQUEUE semantics: walks
// BOTH queue.all() AND weakQueue.all() (PlayerOps.ts:903-920). Goscape's
// unified p.queue holds all four types; the loop counts all matches
// regardless of Type. NAI-161 T2.
func TestQueueCountIncludesAllQueueTypes(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	sf10 := &script.ScriptFile{Name: "[test_id10]"}
	s.scriptProvider.Register(sf10) // id=0

	p, _ := newTestPlayer(t)
	p.client.server = s
	p.EnqueueScriptFile(sf10, 0, nil, nil, script.QueueNormal)
	p.EnqueueScriptFile(sf10, 0, nil, nil, script.QueueStrong)
	p.EnqueueScriptFile(sf10, 0, nil, nil, script.QueueLong)
	p.EnqueueScriptFile(sf10, 0, nil, nil, script.QueueWeak)

	got := p.QueueCount(0)
	if got != 4 {
		t.Errorf("QueueCount(0): got %d, want 4 (Normal+Strong+Long+Weak all counted)", got)
	}
}

// TestQueueCountExcludesEngineQueue pins that engineQueue is a
// separate slice and is never counted by QueueCount. NAI-161 T2.
func TestQueueCountExcludesEngineQueue(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	sf10 := &script.ScriptFile{Name: "[test_id10]"}
	s.scriptProvider.Register(sf10) // id=0

	p, _ := newTestPlayer(t)
	p.client.server = s
	p.EnqueueScriptFile(sf10, 0, nil, nil, script.QueueNormal)
	p.EnqueueScriptFile(sf10, 0, nil, nil, script.QueueEngine)

	got := p.QueueCount(0)
	if got != 1 {
		t.Errorf("QueueCount(0): got %d, want 1 (engine entry excluded)", got)
	}
}

// TestQueueCountUnknownIDReturnsZero pins that an out-of-range
// scriptID resolves to nil → returns 0. Mirrors TS finding zero
// matches in the same scenario. NAI-161 T2.
func TestQueueCountUnknownIDReturnsZero(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	sf10 := &script.ScriptFile{Name: "[test_id10]"}
	s.scriptProvider.Register(sf10) // id=0

	p, _ := newTestPlayer(t)
	p.client.server = s
	p.EnqueueScriptFile(sf10, 0, nil, nil, script.QueueNormal)

	got := p.QueueCount(99)
	if got != 0 {
		t.Errorf("QueueCount(99): got %d, want 0 (bogus scriptID)", got)
	}
}

// TestQueueCountNilServerReturnsZero pins the defensive guard.
// NAI-161 T2 — deviation NAI-161-D-CLEARQUEUE-NIL-PROVIDER.
func TestQueueCountNilServerReturnsZero(t *testing.T) {
	p, _ := newTestPlayer(t)
	// p.client.server is nil by default.
	got := p.QueueCount(99)
	if got != 0 {
		t.Errorf("QueueCount on nil-server player: got %d, want 0", got)
	}
}

func TestAddXPFiresChangeStatOnLevelUp(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	// Register [changestat,attack=0] — keyed by trigger(165) | (0x2<<8) | (0<<10).
	key := script.LookupKeyForType(script.TriggerChangeStat, objtype.PlayerStatAttack)
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

	// NAI-144: changeStat now uses QueueEngine, not QueueNormal.
	beforeQueue := len(p.queue)
	beforeEngineQueue := len(p.engineQueue)
	p.AddXP(objtype.PlayerStatAttack, 1000) // → level 3

	if len(p.queue) != beforeQueue {
		t.Errorf("p.queue len: got %d, want %d (changestat must NOT land in primary queue post-NAI-144)",
			len(p.queue), beforeQueue)
	}
	if len(p.engineQueue) != beforeEngineQueue+1 {
		t.Fatalf("p.engineQueue len: got %d, want %d (+1 changestat via QueueEngine)",
			len(p.engineQueue), beforeEngineQueue+1)
	}
	req := p.engineQueue[beforeEngineQueue]
	if req.Script != sf {
		t.Errorf("p.engineQueue[%d].Script: got %v, want [changestat,attack] (%v)", beforeEngineQueue, req.Script, sf)
	}
	if req.Type != script.QueueEngine {
		t.Errorf("p.engineQueue[%d].Type: got %v, want QueueEngine (NAI-144 closes S6h deviation)", beforeEngineQueue, req.Type)
	}
}

func TestAddXPDoesNotFireChangeStatWithoutLevelUp(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	key := script.LookupKeyForType(script.TriggerChangeStat, objtype.PlayerStatAttack)
	s.scriptProvider.Register(&script.ScriptFile{Name: "[changestat,attack]", LookupKey: key})

	p, _ := newTestPlayer(t)
	p.client.server = s
	p.stats[objtype.PlayerStatAttack] = 100 // below level-2 threshold (830)
	p.baseLevels[objtype.PlayerStatAttack] = 1
	p.levels[objtype.PlayerStatAttack] = 1

	beforeQueue := len(p.queue)
	beforeEngineQueue := len(p.engineQueue)
	p.AddXP(objtype.PlayerStatAttack, 100) // → 200, still level 1 (< 830)

	if len(p.queue) != beforeQueue {
		t.Errorf("queue len: got %d, want %d (no level-up = no changestat fire)",
			len(p.queue), beforeQueue)
	}
	if len(p.engineQueue) != beforeEngineQueue {
		t.Errorf("engineQueue len: got %d, want %d (no level-up = no changestat fire)",
			len(p.engineQueue), beforeEngineQueue)
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

	beforeQueue := len(p.queue)
	beforeEngineQueue := len(p.engineQueue)
	p.AddXP(objtype.PlayerStatAttack, 1000) // level up, but no script registered

	if len(p.queue) != beforeQueue {
		t.Errorf("queue len: got %d, want %d (no registered script = silent no-op)",
			len(p.queue), beforeQueue)
	}
	if len(p.engineQueue) != beforeEngineQueue {
		t.Errorf("engineQueue len: got %d, want %d (no registered script = silent no-op)",
			len(p.engineQueue), beforeEngineQueue)
	}
	// Verify the level-up math still happened.
	if p.baseLevels[objtype.PlayerStatAttack] != 3 {
		t.Errorf("baseLevels: got %d, want 3 (level-up math independent of changeStat)",
			p.baseLevels[objtype.PlayerStatAttack])
	}
}

func TestAddXPFiresAdvanceStatOnLevelUp(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	// Register [advancestat,attack=0] at the type-specific lookup key.
	key := script.LookupKeyForType(script.TriggerAdvanceStat, objtype.PlayerStatAttack)
	sf := &script.ScriptFile{
		Name:      "[advancestat,attack]",
		LookupKey: key,
	}
	s.scriptProvider.Register(sf)

	p, _ := newTestPlayer(t)
	p.client.server = s
	p.stats[objtype.PlayerStatAttack] = 800
	p.baseLevels[objtype.PlayerStatAttack] = 2
	p.levels[objtype.PlayerStatAttack] = 2

	// NAI-144: advanceStat now uses QueueEngine, not QueueNormal.
	beforeQueue := len(p.queue)
	beforeEngineQueue := len(p.engineQueue)
	p.AddXP(objtype.PlayerStatAttack, 1000) // → level 3

	if len(p.queue) != beforeQueue {
		t.Errorf("p.queue len: got %d, want %d (advancestat must NOT land in primary queue post-NAI-144)",
			len(p.queue), beforeQueue)
	}
	if len(p.engineQueue) != beforeEngineQueue+1 {
		t.Fatalf("p.engineQueue len: got %d, want %d (+1 advancestat via QueueEngine)",
			len(p.engineQueue), beforeEngineQueue+1)
	}
	req := p.engineQueue[beforeEngineQueue]
	if req.Script != sf {
		t.Errorf("p.engineQueue[%d].Script: got %v, want [advancestat,attack] (%v)", beforeEngineQueue, req.Script, sf)
	}
	if req.Type != script.QueueEngine {
		t.Errorf("p.engineQueue[%d].Type: got %v, want QueueEngine (NAI-144 closes S6h deviation)", beforeEngineQueue, req.Type)
	}
}

func TestAddXPDoesNotFireAdvanceStatWithoutLevelUp(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	key := script.LookupKeyForType(script.TriggerAdvanceStat, objtype.PlayerStatAttack)
	s.scriptProvider.Register(&script.ScriptFile{Name: "[advancestat,attack]", LookupKey: key})

	p, _ := newTestPlayer(t)
	p.client.server = s
	p.stats[objtype.PlayerStatAttack] = 100 // below level-2 threshold
	p.baseLevels[objtype.PlayerStatAttack] = 1
	p.levels[objtype.PlayerStatAttack] = 1

	beforeQueue := len(p.queue)
	beforeEngineQueue := len(p.engineQueue)
	p.AddXP(objtype.PlayerStatAttack, 100) // → 200, still level 1

	if len(p.queue) != beforeQueue {
		t.Errorf("queue len: got %d, want %d (no level-up = no advancestat fire)",
			len(p.queue), beforeQueue)
	}
	if len(p.engineQueue) != beforeEngineQueue {
		t.Errorf("engineQueue len: got %d, want %d (no level-up = no advancestat fire)",
			len(p.engineQueue), beforeEngineQueue)
	}
}

func TestAddXPAdvanceStatNoFallbackToGlobal(t *testing.T) {
	// Register a GLOBAL [advancestat,_] script. AdvanceStat uses
	// GetByTriggerSpecific which does NOT fall back, so the global script
	// should NOT fire on a per-skill level-up.
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	globalKey := script.LookupKeyForGlobal(script.TriggerAdvanceStat)
	s.scriptProvider.Register(&script.ScriptFile{Name: "[advancestat,_]", LookupKey: globalKey})

	p, _ := newTestPlayer(t)
	p.client.server = s
	p.stats[objtype.PlayerStatAttack] = 800
	p.baseLevels[objtype.PlayerStatAttack] = 2
	p.levels[objtype.PlayerStatAttack] = 2

	beforeQueue := len(p.queue)
	beforeEngineQueue := len(p.engineQueue)
	p.AddXP(objtype.PlayerStatAttack, 1000) // level up

	if len(p.queue) != beforeQueue {
		t.Errorf("queue len: got %d, want %d (global script must NOT fire — advancestat is type-specific only)",
			len(p.queue), beforeQueue)
	}
	if len(p.engineQueue) != beforeEngineQueue {
		t.Errorf("engineQueue len: got %d, want %d (global script must NOT fire — advancestat is type-specific only)",
			len(p.engineQueue), beforeEngineQueue)
	}
	// Verify level-up math still happened.
	if p.baseLevels[objtype.PlayerStatAttack] != 3 {
		t.Errorf("baseLevels: got %d, want 3 (level-up math independent of advancestat fire)",
			p.baseLevels[objtype.PlayerStatAttack])
	}
}

func TestAddXPFiresBothChangeAndAdvanceStatOnLevelUp(t *testing.T) {
	// Both triggers should enqueue when both scripts are registered.
	// Validates that S6h's changeStat and S6i's advanceStat coexist
	// AND that they fire in TS order (changeStat first, advanceStat second).
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()

	changeKey := script.LookupKeyForType(script.TriggerChangeStat, objtype.PlayerStatAttack)
	advKey := script.LookupKeyForType(script.TriggerAdvanceStat, objtype.PlayerStatAttack)
	changeSF := &script.ScriptFile{Name: "[changestat,attack]", LookupKey: changeKey}
	advSF := &script.ScriptFile{Name: "[advancestat,attack]", LookupKey: advKey}
	s.scriptProvider.Register(changeSF)
	s.scriptProvider.Register(advSF)

	p, _ := newTestPlayer(t)
	p.client.server = s
	p.stats[objtype.PlayerStatAttack] = 800
	p.baseLevels[objtype.PlayerStatAttack] = 2
	p.levels[objtype.PlayerStatAttack] = 2

	// NAI-144: both changeStat and advanceStat now use QueueEngine.
	beforeQueue := len(p.queue)
	beforeEngineQueue := len(p.engineQueue)
	p.AddXP(objtype.PlayerStatAttack, 1000) // level up

	if len(p.queue) != beforeQueue {
		t.Errorf("p.queue len: got %d, want %d (neither changestat nor advancestat must land in primary queue post-NAI-144)",
			len(p.queue), beforeQueue)
	}
	if len(p.engineQueue) != beforeEngineQueue+2 {
		t.Fatalf("p.engineQueue len: got %d, want %d (+2 — both changestat and advancestat via QueueEngine)",
			len(p.engineQueue), beforeEngineQueue+2)
	}
	// Order: changeStat before advanceStat (matches TS Player.ts:1772, 1804).
	if p.engineQueue[beforeEngineQueue].Script != changeSF {
		t.Errorf("p.engineQueue[%d].Script: got %v, want changestat first", beforeEngineQueue, p.engineQueue[beforeEngineQueue].Script)
	}
	if p.engineQueue[beforeEngineQueue+1].Script != advSF {
		t.Errorf("p.engineQueue[%d].Script: got %v, want advancestat second", beforeEngineQueue+1, p.engineQueue[beforeEngineQueue+1].Script)
	}
}

func TestPlayerLowMemoryGetter(t *testing.T) {
	p, _ := newTestPlayer(t)
	if p.LowMemory() {
		t.Errorf("default: want LowMemory()=false, got true")
	}
	p.lowMemory = true
	if !p.LowMemory() {
		t.Errorf("after set: want LowMemory()=true, got false")
	}
}

func TestNormalizeSongNameLowercaseAndSpacesToUnderscores(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"Harmony 1", "harmony_1"},
		{"already_lower", "already_lower"},
		{"ALLCAPS", "allcaps"},
		{"Mixed CASE With Spaces", "mixed_case_with_spaces"},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			if got := normalizeSongName(tc.in); got != tc.want {
				t.Errorf("normalizeSongName(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestNormalizeSongNameEmptyReturnsEmpty(t *testing.T) {
	if got := normalizeSongName(""); got != "" {
		t.Errorf("normalizeSongName(\"\") = %q, want \"\"", got)
	}
}

// seedCachedMidi seeds both cache.Preloaded and cache.PreloadedCRC under
// `name` and registers a t.Cleanup to remove both entries after the test.
// Mirrors the production PreloadClient write shape without touching the
// filesystem. Usable for both song and jingle test paths (PlayJingle
// ignores the CRC entry; the wasted write is harmless).
func seedCachedMidi(t *testing.T, name string, data []byte, crc uint32) {
	t.Helper()
	cache.Preloaded[name] = data
	cache.PreloadedCRC[name] = crc
	t.Cleanup(func() {
		delete(cache.Preloaded, name)
		delete(cache.PreloadedCRC, name)
	})
}

// TestPlaySongWritesOut pins NAI-16's retirement of S7h-D1:
// (*Player).PlaySong now issues a writeOut after the PRELOADED lookup.
// Failure signal = "write-path broken or PRELOADED seeding broken."
// Replaces the prior absence-pin (which was the S7h-D1 escalation
// signal — now satisfied by NAI-16).
func TestPlaySongWritesOut(t *testing.T) {
	seedCachedMidi(t, "adventure.mid", []byte{0x01, 0x02, 0x03}, 0xDEADBEEF)
	p, _ := newTestPlayer(t)
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	p.client.encryptor = enc
	p.PlaySong("adventure")
	if n := p.client.bufw.Buffered(); n == 0 {
		t.Errorf("PlaySong wrote 0 bytes to c.bufw; want >0 (NAI-16 positive pin)")
	}
}

// TestPlaySongMissingFromPreloadedReturnsSilently pins TS's
// `if (song && crc)` guard at Player.ts:1910. PlaySong with a name that
// is not in PRELOADED must be a silent no-op.
func TestPlaySongMissingFromPreloadedReturnsSilently(t *testing.T) {
	// Do NOT seed the cache for "missing.mid".
	p, _ := newTestPlayer(t)
	p.PlaySong("missing")
	if n := p.client.bufw.Buffered(); n != 0 {
		t.Errorf("PlaySong with missing PRELOADED key wrote %d bytes; want 0 (silent no-op)", n)
	}
}

// TestPlaySongSongSeededButCRCMissingReturnsSilently pins the `||`
// conjunction in the (*Player).PlaySong guard: both Preloaded AND
// PreloadedCRC must be populated for the write to fire. Defensive
// guard against future test seeding that populates only one map.
func TestPlaySongSongSeededButCRCMissingReturnsSilently(t *testing.T) {
	// Seed Preloaded but not PreloadedCRC.
	cache.Preloaded["orphan.mid"] = []byte{0xAA}
	t.Cleanup(func() {
		delete(cache.Preloaded, "orphan.mid")
	})
	p, _ := newTestPlayer(t)
	p.PlaySong("orphan")
	if n := p.client.bufw.Buffered(); n != 0 {
		t.Errorf("PlaySong with PRELOADED-only seed wrote %d bytes; want 0", n)
	}
}

func TestPlaySongEmptyNameReturnsSilently(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.PlaySong("")
	if n := p.client.bufw.Buffered(); n != 0 {
		t.Errorf("empty name: PlaySong wrote %d bytes; want 0", n)
	}
}

func TestNormalizeJingleNameLowercaseAndUnderscoresToSpaces(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"a_quick_jingle", "a quick jingle"},
		{"Space Already", "space already"},
		{"ALLCAPS", "allcaps"},
		{"Mixed_CASE_With_Underscores", "mixed case with underscores"},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			if got := normalizeJingleName(tc.in); got != tc.want {
				t.Errorf("normalizeJingleName(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestNormalizeJingleNameEmptyReturnsEmpty(t *testing.T) {
	if got := normalizeJingleName(""); got != "" {
		t.Errorf("normalizeJingleName(\"\") = %q, want \"\"", got)
	}
}

// TestPlayJingleWritesOut pins NAI-16's retirement of S7h-D1 (jingle side):
// (*Player).PlayJingle now issues a writeOut after the PRELOADED lookup.
func TestPlayJingleWritesOut(t *testing.T) {
	seedCachedMidi(t, "fanfare.mid", []byte{0xAB, 0xCD}, 0)
	p, _ := newTestPlayer(t)
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	p.client.encryptor = enc
	p.PlayJingle(3, "fanfare")
	if n := p.client.bufw.Buffered(); n == 0 {
		t.Errorf("PlayJingle wrote 0 bytes to c.bufw; want >0 (NAI-16 positive pin)")
	}
}

// TestPlayJingleMissingFromPreloadedReturnsSilently pins TS's
// `if (jingle)` guard at Player.ts:1923.
func TestPlayJingleMissingFromPreloadedReturnsSilently(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.PlayJingle(3, "missing")
	if n := p.client.bufw.Buffered(); n != 0 {
		t.Errorf("PlayJingle with missing PRELOADED key wrote %d bytes; want 0 (silent no-op)", n)
	}
}

func TestPlayJingleEmptyNameReturnsSilently(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.PlayJingle(3, "")
	if n := p.client.bufw.Buffered(); n != 0 {
		t.Errorf("empty name: PlayJingle wrote %d bytes; want 0", n)
	}
}

// TestClearWeakQueueRemovesOnlyWeakEntries pins (*Player).clearWeakQueue:
// drops every QueueWeak entry from p.queue, preserves relative order
// of remaining entries. Mirrors TS Player.weakQueue.clear() (Player.ts:743).
func TestClearWeakQueueRemovesOnlyWeakEntries(t *testing.T) {
	p, _ := newTestPlayer(t)
	sf := &script.ScriptFile{Name: "stub"}
	p.queue = []playerQueueRequest{
		{Script: sf, Type: script.QueueStrong},
		{Script: sf, Type: script.QueueWeak},
		{Script: sf, Type: script.QueueNormal},
		{Script: sf, Type: script.QueueWeak},
		{Script: sf, Type: script.QueueLong},
	}

	p.clearWeakQueue()

	if got, want := len(p.queue), 3; got != want {
		t.Fatalf("queue len after clearWeakQueue: got %d, want %d", got, want)
	}
	wantTypes := []script.PlayerQueueType{
		script.QueueStrong, script.QueueNormal, script.QueueLong,
	}
	for i, want := range wantTypes {
		if got := p.queue[i].Type; got != want {
			t.Errorf("queue[%d].Type: got %v, want %v (order must be preserved)", i, got, want)
		}
	}
}

// TestClearWeakQueueEmptyQueueNoOp pins clearWeakQueue is safe on empty queue.
func TestClearWeakQueueEmptyQueueNoOp(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.queue = nil

	p.clearWeakQueue()

	if len(p.queue) != 0 {
		t.Errorf("queue len: got %d, want 0", len(p.queue))
	}
}

// TestClearWeakQueueAllWeakEntries pins clearWeakQueue empties a queue
// of all-weak entries.
func TestClearWeakQueueAllWeakEntries(t *testing.T) {
	p, _ := newTestPlayer(t)
	sf := &script.ScriptFile{Name: "stub"}
	p.queue = []playerQueueRequest{
		{Script: sf, Type: script.QueueWeak},
		{Script: sf, Type: script.QueueWeak},
	}

	p.clearWeakQueue()

	if len(p.queue) != 0 {
		t.Errorf("queue len: got %d, want 0 (all weak entries should be removed)", len(p.queue))
	}
}

// TestClearWeakQueueIdempotent pins repeated clearWeakQueue is a no-op
// after the first call.
func TestClearWeakQueueIdempotent(t *testing.T) {
	p, _ := newTestPlayer(t)
	sf := &script.ScriptFile{Name: "stub"}
	p.queue = []playerQueueRequest{
		{Script: sf, Type: script.QueueStrong},
		{Script: sf, Type: script.QueueWeak},
	}

	p.clearWeakQueue()
	p.clearWeakQueue()

	if got, want := len(p.queue), 1; got != want {
		t.Errorf("queue len after 2× clearWeakQueue: got %d, want %d", got, want)
	}
	if p.queue[0].Type != script.QueueStrong {
		t.Errorf("queue[0].Type: got %v, want QueueStrong", p.queue[0].Type)
	}
}

func TestPlayerTeleportCrossZoneRefreshSubscription(t *testing.T) {
	s := newTestServer(t)
	c, _ := newTestClient(t)
	p := newPlayer(c)
	p.client.server = s
	p.x, p.z, p.level = 3200, 3200, 0
	if err := s.addPlayer(p); err != nil {
		t.Fatalf("addPlayer: %v", err)
	}
	prevZone := s.zoneMap.Get(0, 3200, 3200)
	p.Teleport(4000, 4000, 0)
	newZone := s.zoneMap.Get(0, 4000, 4000)
	if prevZone.PlayersCount() != 0 {
		t.Errorf("prev zone PlayersCount after Teleport: got %d, want 0", prevZone.PlayersCount())
	}
	if newZone.PlayersCount() != 1 {
		t.Errorf("new zone PlayersCount after Teleport: got %d, want 1", newZone.PlayersCount())
	}
}

func TestPlayerTeleJumpCrossLevelRefreshSubscription(t *testing.T) {
	s := newTestServer(t)
	c, _ := newTestClient(t)
	p := newPlayer(c)
	p.client.server = s
	p.x, p.z, p.level = 3200, 3200, 0
	if err := s.addPlayer(p); err != nil {
		t.Fatalf("addPlayer: %v", err)
	}
	prevZone := s.zoneMap.Get(0, 3200, 3200)
	p.TeleJump(3200, 3200, 1) // same xy, level=0→1
	newZone := s.zoneMap.Get(1, 3200, 3200)
	if prevZone.PlayersCount() != 0 {
		t.Errorf("prev zone PlayersCount after cross-level TeleJump: got %d, want 0", prevZone.PlayersCount())
	}
	if newZone.PlayersCount() != 1 {
		t.Errorf("new zone PlayersCount after cross-level TeleJump: got %d, want 1", newZone.PlayersCount())
	}
}

func TestPlayerTeleportSameZoneNoRefresh(t *testing.T) {
	s := newTestServer(t)
	c, _ := newTestClient(t)
	p := newPlayer(c)
	p.client.server = s
	p.x, p.z, p.level = 3200, 3200, 0
	if err := s.addPlayer(p); err != nil {
		t.Fatalf("addPlayer: %v", err)
	}
	z := s.zoneMap.Get(0, 3200, 3200)
	prevElement := p.zoneListElement
	p.Teleport(3201, 3201, 0) // same zone (400, 400)
	if z.PlayersCount() != 1 {
		t.Errorf("same-zone Teleport PlayersCount: got %d, want 1", z.PlayersCount())
	}
	// Same-zone teleport should NOT re-subscribe (no leave/enter dance).
	if p.zoneListElement != prevElement {
		t.Error("same-zone Teleport should preserve zoneListElement (no leave/enter)")
	}
}

// --- NAI-36 Task 7: Player.Teleport partial parity ----------------------
//
// Closes NAI-34-D1 (level clamp), NAI-34-D2 (unallocated-zone reject),
// body-order alignment to refresh-then-tele, and NAI-34-D5 (level-change
// INSTANT + jump branch) for Player. Mirrors TS PathingEntity.teleport
// at PathingEntity.ts:267-298.

// TestPlayerTeleport_LevelClampNegative pins D1: level=-1 clamps to 0
// per TS PathingEntity.ts:268-271 (TS uses Math.max(0, ...)).
func TestPlayerTeleport_LevelClampNegative(t *testing.T) {
	s := newTestServer(t)
	c, _ := newTestClient(t)
	p := newPlayer(c)
	p.client.server = s
	p.x, p.z, p.level = 3200, 3300, 0
	if err := s.addPlayer(p); err != nil {
		t.Fatalf("addPlayer: %v", err)
	}

	p.Teleport(3210, 3310, -1)

	if p.level != 0 {
		t.Errorf("level after Teleport(level=-1): got %d, want 0 (clamp)", p.level)
	}
	if p.x != 3210 || p.z != 3310 {
		t.Errorf("x/z after Teleport: got (%d, %d), want (3210, 3310)", p.x, p.z)
	}
}

// TestPlayerTeleport_LevelClampHigh pins D1 upper bound: level=4 clamps
// to 3 per TS PathingEntity.ts:271 (TS uses Math.min(level, 3)).
func TestPlayerTeleport_LevelClampHigh(t *testing.T) {
	s := newTestServer(t)
	c, _ := newTestClient(t)
	p := newPlayer(c)
	p.client.server = s
	p.x, p.z, p.level = 3200, 3300, 0
	if err := s.addPlayer(p); err != nil {
		t.Fatalf("addPlayer: %v", err)
	}

	p.Teleport(3210, 3310, 4)

	if p.level != 3 {
		t.Errorf("level after Teleport(level=4): got %d, want 3 (clamp)", p.level)
	}
}

// TestPlayerTeleport_UnallocatedZoneRejects pins D2: a teleport to a zone
// where IsZoneAllocated returns false is silently ignored — no coord
// mutation, no tele flag write. Per TS PathingEntity.ts:273-278.
func TestPlayerTeleport_UnallocatedZoneRejects(t *testing.T) {
	s := newTestServer(t)
	// Wire a real gamemap so IsZoneAllocated returns false for any
	// un-allocated zone (test default: all zones unallocated).
	s.gamemap = gamemap.New(discardLogger())
	// Allocate the starting zone so the player can be placed there.
	s.gamemap.Pathfinder.Flags.AllocateIfAbsent(3200, 3300, 0)

	c, _ := newTestClient(t)
	p := newPlayer(c)
	p.client.server = s
	p.x, p.z, p.level = 3200, 3300, 0
	if err := s.addPlayer(p); err != nil {
		t.Fatalf("addPlayer: %v", err)
	}
	prevX, prevZ, prevLevel := p.x, p.z, p.level
	p.tele = false

	// Target zone (3210, 3310) is NOT allocated → reject.
	p.Teleport(3210, 3310, 0)

	if p.x != prevX || p.z != prevZ || p.level != prevLevel {
		t.Errorf("Teleport to unallocated zone: state changed (%d,%d,%d) → (%d,%d,%d), want unchanged",
			prevX, prevZ, prevLevel, p.x, p.z, p.level)
	}
	if p.tele {
		t.Errorf("tele flag: got true, want false (rejected teleport must not set flag)")
	}
}

// TestPlayerTeleport_AllocatedZoneAccepts pins the D2 positive case: a
// teleport to a zone where IsZoneAllocated returns true completes
// normally. Pairs with TestPlayerTeleport_UnallocatedZoneRejects to
// guard against a degenerate "always-reject" implementation.
func TestPlayerTeleport_AllocatedZoneAccepts(t *testing.T) {
	s := newTestServer(t)
	s.gamemap = gamemap.New(discardLogger())
	s.gamemap.Pathfinder.Flags.AllocateIfAbsent(3200, 3300, 0)
	s.gamemap.Pathfinder.Flags.AllocateIfAbsent(3210, 3310, 0)

	c, _ := newTestClient(t)
	p := newPlayer(c)
	p.client.server = s
	p.x, p.z, p.level = 3200, 3300, 0
	if err := s.addPlayer(p); err != nil {
		t.Fatalf("addPlayer: %v", err)
	}

	p.Teleport(3210, 3310, 0)

	if p.x != 3210 || p.z != 3310 || p.level != 0 {
		t.Errorf("Teleport to allocated zone: got (%d,%d,%d), want (3210,3310,0)",
			p.x, p.z, p.level)
	}
	if !p.tele {
		t.Error("tele flag: got false, want true (accepted teleport must set flag)")
	}
}

// TestPlayerTeleport_SameLevelNoMoveSpeedChange pins the D5 negative
// case: a same-level teleport leaves moveSpeed/jump untouched per TS
// PathingEntity.ts:295 (the `previousLevel != level` guard).
func TestPlayerTeleport_SameLevelNoMoveSpeedChange(t *testing.T) {
	s := newTestServer(t)
	c, _ := newTestClient(t)
	p := newPlayer(c)
	p.client.server = s
	p.x, p.z, p.level = 3200, 3300, 0
	if err := s.addPlayer(p); err != nil {
		t.Fatalf("addPlayer: %v", err)
	}
	p.moveSpeed = MoveSpeedWalk
	p.jump = false

	p.Teleport(3210, 3310, 0) // same level (0 → 0)

	if p.moveSpeed != MoveSpeedWalk {
		t.Errorf("same-level moveSpeed: got %v, want MoveSpeedWalk (unchanged)", p.moveSpeed)
	}
	if p.jump {
		t.Errorf("same-level jump: got true, want false (unchanged)")
	}
}

// TestPlayerTeleport_LevelChangeSetsInstantAndJump pins the D5 positive
// case: a level-change teleport sets moveSpeed=Instant + jump=true per
// TS PathingEntity.ts:295-298.
func TestPlayerTeleport_LevelChangeSetsInstantAndJump(t *testing.T) {
	s := newTestServer(t)
	c, _ := newTestClient(t)
	p := newPlayer(c)
	p.client.server = s
	p.x, p.z, p.level = 3200, 3300, 0
	if err := s.addPlayer(p); err != nil {
		t.Fatalf("addPlayer: %v", err)
	}
	p.moveSpeed = MoveSpeedWalk
	p.jump = false

	p.Teleport(3210, 3310, 1) // level changed 0 → 1

	if p.moveSpeed != MoveSpeedInstant {
		t.Errorf("level-change moveSpeed: got %v, want MoveSpeedInstant", p.moveSpeed)
	}
	if !p.jump {
		t.Errorf("level-change jump: got false, want true")
	}
}

// TestPlayerTeleport_OrderRefreshThenFlag pins the body-order alignment.
// Note: refreshPlayerZone reads only previous + current coords, never
// p.tele; and p.tele = true never reads zone state. The two writes are
// runtime-commutative — order is purely structural and invisible at the
// observable-state layer. This test is a behavior witness rather than a
// strict order-pin: it asserts that BOTH effects are applied (refresh
// happened AND tele=true) so a regression that drops one would surface.
// Source-level order is enforced by the doc comment on Teleport plus
// code review; the runtime-equivalent claim is documented at
// modules/world/player_script.go in the Teleport doc-block. NAI-36-T7.
func TestPlayerTeleport_OrderRefreshThenFlag(t *testing.T) {
	s := newTestServer(t)
	c, _ := newTestClient(t)
	p := newPlayer(c)
	p.client.server = s
	p.x, p.z, p.level = 3200, 3200, 0
	if err := s.addPlayer(p); err != nil {
		t.Fatalf("addPlayer: %v", err)
	}
	prevZone := s.zoneMap.Get(0, 3200, 3200)
	p.tele = false

	p.Teleport(4000, 4000, 0) // cross-zone so refresh actually runs

	newZone := s.zoneMap.Get(0, 4000, 4000)
	if prevZone.PlayersCount() != 0 {
		t.Errorf("refresh effect missing: prevZone PlayersCount=%d, want 0",
			prevZone.PlayersCount())
	}
	if newZone.PlayersCount() != 1 {
		t.Errorf("refresh effect missing: newZone PlayersCount=%d, want 1",
			newZone.PlayersCount())
	}
	if !p.tele {
		t.Errorf("tele flag write missing: got false, want true")
	}
}

// --- NAI-37 Task 5: HintNpc payload byte-pin test --------------------------

// TestHintNpcPayloadBytes pins the type=1 HintArrow encoder branch
// byte-for-byte. nid=0x1234 chosen so each byte position is
// distinguishable from the zero-fill (catches type-byte regression,
// nid endianness, and field misordering — see rsbuf_roundtrip_tests.md).
// OpHintArrow has PayloadSize=6 (fixed), so the wire is
// [encrypted_opcode, p1(type=1), p2(nid_hi), p2(nid_lo), p2(0)_hi, p2(0)_lo, p1(0)] — 7 bytes total.
func TestHintNpcPayloadBytes(t *testing.T) {
	p, cc := newTestPlayer(t)
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	want := []byte{
		byte((int(gameserver.OpHintArrow.Opcode) + int(enc.GetNext())) & 0xff),
		0x01,       // p1: type = 1 (NPC hint)
		0x12, 0x34, // p2: nid (big-endian)
		0x00, 0x00, // p2: 0 (unused playerSlot for type=1)
		0x00, // p1: 0 (unused y for type=1)
	}

	received := drainConn(t, cc)
	p.HintNpc(0x1234)
	p.client.flushWrite()
	got := <-received
	if !bytes.Equal(got, want) {
		t.Errorf("HintNpc(0x1234) wire: got %#x, want %#x", got, want)
	}
}

// --- NAI-39 Task 2: HintCoord / HintPlayer / HintStop byte-pin tests -------

// TestHintCoordPayloadBytes pins the type=2..6 (TILE) HintArrow encoder
// branch byte-for-byte. Per HintArrowEncoder.ts:17-27 the wire shape is
// p1(type=offset), p2(x), p2(z), p1(height). The encoder name "y" is
// the script-author-facing "height".
func TestHintCoordPayloadBytes(t *testing.T) {
	p, cc := newTestPlayer(t)
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	want := []byte{
		byte((int(gameserver.OpHintArrow.Opcode) + int(enc.GetNext())) & 0xff),
		0x03,       // p1: type=offset=3
		0x12, 0x34, // p2: x = 0x1234
		0x56, 0x78, // p2: z = 0x5678
		0x42, // p1: height=0x42
	}

	received := drainConn(t, cc)
	p.HintCoord(3, 0x1234, 0x5678, 0x42)
	p.client.flushWrite()
	got := <-received
	if !bytes.Equal(got, want) {
		t.Errorf("HintCoord(3, 0x1234, 0x5678, 0x42) wire: got %#x, want %#x", got, want)
	}
}

// TestHintCoordOffsetBoundaries pins both ends of the TILE-branch range
// (offset=2 = far-left, offset=6 = top-left). Both must produce well-formed
// 6-byte payloads with the offset in byte[0] post-encryption.
func TestHintCoordOffsetBoundaries(t *testing.T) {
	for _, offset := range []int{2, 6} {
		t.Run(fmt.Sprintf("offset=%d", offset), func(t *testing.T) {
			p, cc := newTestPlayer(t)
			enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
			p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

			want := []byte{
				byte((int(gameserver.OpHintArrow.Opcode) + int(enc.GetNext())) & 0xff),
				byte(offset), // p1: type=offset
				0x00, 0x01,   // p2: x=1
				0x00, 0x02, // p2: z=2
				0x00, // p1: height=0
			}

			received := drainConn(t, cc)
			p.HintCoord(offset, 1, 2, 0)
			p.client.flushWrite()
			got := <-received
			if !bytes.Equal(got, want) {
				t.Errorf("HintCoord(%d,1,2,0) wire: got %#x, want %#x", offset, got, want)
			}
		})
	}
}

// TestHintPlayerPayloadBytes pins the type=10 (PL) HintArrow encoder
// branch byte-for-byte. Per HintArrowEncoder.ts:28-32 the wire shape is
// p1(0x0A), p2(playerSlot), p2(0), p1(0). slot=0xABCD chosen so each
// byte position is distinguishable from the zero-fill.
func TestHintPlayerPayloadBytes(t *testing.T) {
	p, cc := newTestPlayer(t)
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	want := []byte{
		byte((int(gameserver.OpHintArrow.Opcode) + int(enc.GetNext())) & 0xff),
		0x0A,       // p1: type = 10 (player hint)
		0xAB, 0xCD, // p2: slot=0xABCD (big-endian)
		0x00, 0x00, // p2: 0
		0x00, // p1: 0
	}

	received := drainConn(t, cc)
	p.HintPlayer(0xABCD)
	p.client.flushWrite()
	got := <-received
	if !bytes.Equal(got, want) {
		t.Errorf("HintPlayer(0xABCD) wire: got %#x, want %#x", got, want)
	}
}

// TestHintStopPayloadBytes pins the type=-1 (STOP) HintArrow encoder
// branch byte-for-byte. Per HintArrowEncoder.ts:33-38 the wire shape is
// p1(-1), p2(0), p2(0), p1(0). p1(-1) is 0xFF on the wire (low byte of
// two's-complement). The 0xFF asymmetry is the conspicuous-pin per
// ts_asymmetry_dual_pin.md.
func TestHintStopPayloadBytes(t *testing.T) {
	p, cc := newTestPlayer(t)
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	want := []byte{
		byte((int(gameserver.OpHintArrow.Opcode) + int(enc.GetNext())) & 0xff),
		0xFF,       // p1: type = -1 sentinel (two's-complement low byte)
		0x00, 0x00, // p2: 0
		0x00, 0x00, // p2: 0
		0x00, // p1: 0
	}

	received := drainConn(t, cc)
	p.HintStop()
	p.client.flushWrite()
	got := <-received
	if !bytes.Equal(got, want) {
		t.Errorf("HintStop() wire: got %#x, want %#x", got, want)
	}
}

// TestPlayerOnScriptFinishedOrAborted_MatchNoMain pins the player-path
// Finished/Aborted tail where state matches activeScript and no MAIN
// modal is open: activeScript is nulled and CloseModal(false) fires.
// Mirrors TS Player.ts:2143-2148. NAI-54 T1.
func TestPlayerOnScriptFinishedOrAborted_MatchNoMain(t *testing.T) {
	p, _ := newTestPlayer(t)
	state := &script.ScriptState{Script: &script.ScriptFile{Name: "match-no-main"}}
	p.activeScript = state
	p.modalState = modalStateChat
	p.modalChat = 100
	p.refreshModalClose = false

	p.OnScriptFinishedOrAborted(state)

	if p.activeScript != nil {
		t.Errorf("activeScript: got non-nil, want nil (state matched and was cleared)")
	}
	if p.modalState != modalStateNone {
		t.Errorf("modalState: got %#x, want %#x (CloseModal must reset)", p.modalState, modalStateNone)
	}
	if !p.refreshModalClose {
		t.Errorf("refreshModalClose: got false, want true (CloseModal fired)")
	}
	if p.modalChat != -1 {
		t.Errorf("modalChat: got %d, want -1 (CloseModal must reset slot)", p.modalChat)
	}
}

// TestPlayerOnScriptFinishedOrAborted_MatchWithMain pins the
// MAIN-modal-preserving branch: activeScript clears but CloseModal does
// NOT fire because (modalState & MAIN) != NONE. Mirrors TS Player.ts:2146.
// NAI-54 T1.
func TestPlayerOnScriptFinishedOrAborted_MatchWithMain(t *testing.T) {
	p, _ := newTestPlayer(t)
	state := &script.ScriptState{Script: &script.ScriptFile{Name: "match-with-main"}}
	p.activeScript = state
	p.modalState = modalStateMain | modalStateChat
	p.modalMain = 200
	p.modalChat = 100
	p.refreshModalClose = false

	p.OnScriptFinishedOrAborted(state)

	if p.activeScript != nil {
		t.Errorf("activeScript: got non-nil, want nil (state matched and was cleared)")
	}
	if p.modalState != modalStateMain|modalStateChat {
		t.Errorf("modalState: got %#x, want %#x (MAIN bit set must skip CloseModal)",
			p.modalState, modalStateMain|modalStateChat)
	}
	if p.refreshModalClose {
		t.Errorf("refreshModalClose: got true, want false (CloseModal must not fire with MAIN open)")
	}
	if p.modalMain != 200 {
		t.Errorf("modalMain: got %d, want 200 (slot must be preserved)", p.modalMain)
	}
	if p.modalChat != 100 {
		t.Errorf("modalChat: got %d, want 100 (slot must be preserved)", p.modalChat)
	}
}

// TestPlayerOnScriptFinishedOrAborted_Mismatch pins the guard: when the
// supplied state is NOT p.activeScript, activeScript is preserved and
// CloseModal does not fire. Closes the silent Suspended-clobber bug.
// NAI-54 T1.
func TestPlayerOnScriptFinishedOrAborted_Mismatch(t *testing.T) {
	p, _ := newTestPlayer(t)
	stored := &script.ScriptState{Script: &script.ScriptFile{Name: "stored"}}
	other := &script.ScriptState{Script: &script.ScriptFile{Name: "other"}}
	p.activeScript = stored
	p.modalState = modalStateChat
	p.modalChat = 100
	p.refreshModalClose = false

	p.OnScriptFinishedOrAborted(other)

	if p.activeScript != stored {
		t.Errorf("activeScript: got %p, want preserved %p", p.activeScript, stored)
	}
	if p.modalState != modalStateChat {
		t.Errorf("modalState: got %#x, want %#x (mismatch must not fire CloseModal)",
			p.modalState, modalStateChat)
	}
	if p.refreshModalClose {
		t.Errorf("refreshModalClose: got true, want false (mismatch must not fire CloseModal)")
	}
}

// TestPlayerOnScriptFinishedOrAborted_NilActive pins the nil-active
// guard: p.activeScript == nil + non-nil arg → no-op (no panic, no
// state change). NAI-54 T1.
func TestPlayerOnScriptFinishedOrAborted_NilActive(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.activeScript = nil
	other := &script.ScriptState{Script: &script.ScriptFile{Name: "other"}}
	p.modalState = modalStateChat
	p.modalChat = 100
	p.refreshModalClose = false

	p.OnScriptFinishedOrAborted(other) // must not panic

	if p.activeScript != nil {
		t.Errorf("activeScript: got non-nil, want nil")
	}
	if p.modalState != modalStateChat {
		t.Errorf("modalState: got %#x, want %#x (no-op)", p.modalState, modalStateChat)
	}
	if p.refreshModalClose {
		t.Errorf("refreshModalClose: got true, want false (no-op)")
	}
}

// TestPlayerFocusWritesFaceAngleAlwaysAndFaceSquareOnInstant pins TS
// PathingEntity.focus (PathingEntity.ts:321-333). instant=false sets
// faceAngleX/Z only — does NOT touch faceSquareX/Z or masks.
// instant=true ALSO writes faceSquareX = fineX, faceSquareZ = fineZ,
// and ORs MaskFaceCoord into masks.
//
// Per ts_asymmetry_dual_pin.md: dual-pin both branches. The
// instant=false absence-pin escalates if upstream changes the focus()
// shape; the instant=true presence-pin escalates if the wire writes
// regress.
func TestPlayerFocusWritesFaceAngleAlwaysAndFaceSquareOnInstant(t *testing.T) {
	c, _ := newTestClient(t)
	p := newPlayer(c)
	p.faceAngleX = -1
	p.faceAngleZ = -1
	p.faceSquareX = -1
	p.faceSquareZ = -1
	p.masks = 0

	// instant=false — faceAngle written; faceSquare/mask untouched.
	p.focus(123, 456, false)
	if p.faceAngleX != 123 || p.faceAngleZ != 456 {
		t.Errorf("instant=false faceAngle: got (%d, %d), want (123, 456)", p.faceAngleX, p.faceAngleZ)
	}
	if p.faceSquareX != -1 || p.faceSquareZ != -1 {
		t.Errorf("instant=false faceSquare: got (%d, %d), want (-1, -1) unchanged", p.faceSquareX, p.faceSquareZ)
	}
	if p.masks != 0 {
		t.Errorf("instant=false masks: got %d, want 0 unchanged", p.masks)
	}

	// instant=true — faceAngle written; faceSquare = (fx, fz);
	// MaskFaceCoord ORed in.
	p.focus(789, 1011, true)
	if p.faceAngleX != 789 || p.faceAngleZ != 1011 {
		t.Errorf("instant=true faceAngle: got (%d, %d), want (789, 1011)", p.faceAngleX, p.faceAngleZ)
	}
	if p.faceSquareX != 789 || p.faceSquareZ != 1011 {
		t.Errorf("instant=true faceSquare: got (%d, %d), want (789, 1011)", p.faceSquareX, p.faceSquareZ)
	}
	if p.masks&rsbuf.MaskFaceCoord == 0 {
		t.Errorf("instant=true masks: MaskFaceCoord bit not set (masks=%d)", p.masks)
	}
}

// TestPlayerTeleport_FocusFromDirection pins NAI-65 D3-Player. Teleport
// from (3200, 3200, 0) to (3300, 3300, 0): direction is NE, so MoveX/MoveZ
// each return prevDest+1. faceAngleX = Fine(3301, 1) = 3301*64 + (1*64-1)/2
// = 211264 + 31 = 211295. Mirrors TS PathingEntity.ts:286-289.
func TestPlayerTeleport_FocusFromDirection(t *testing.T) {
	s := newTestServer(t)
	c, _ := newTestClient(t)
	p := newPlayer(c)
	p.client.server = s
	p.x, p.z, p.level = 3200, 3200, 0
	if err := s.addPlayer(p); err != nil {
		t.Fatalf("addPlayer: %v", err)
	}
	p.faceAngleX = -1
	p.faceAngleZ = -1

	p.Teleport(3300, 3300, 0)

	wantX := 3301*64 + 31
	wantZ := 3301*64 + 31
	if p.faceAngleX != wantX {
		t.Errorf("faceAngleX after Teleport(NE): got %d, want %d (Fine(3301, 1))", p.faceAngleX, wantX)
	}
	if p.faceAngleZ != wantZ {
		t.Errorf("faceAngleZ after Teleport(NE): got %d, want %d (Fine(3301, 1))", p.faceAngleZ, wantZ)
	}
}

// TestPlayerTeleport_LastStepAdjust pins NAI-65 D4-Player. After Teleport,
// p.lastStepX = p.x - 1 and p.lastStepZ = p.z per TS PathingEntity.ts:291-292.
func TestPlayerTeleport_LastStepAdjust(t *testing.T) {
	s := newTestServer(t)
	c, _ := newTestClient(t)
	p := newPlayer(c)
	p.client.server = s
	p.x, p.z, p.level = 3200, 3200, 0
	if err := s.addPlayer(p); err != nil {
		t.Fatalf("addPlayer: %v", err)
	}
	p.lastStepX = -999
	p.lastStepZ = -999

	p.Teleport(3300, 3300, 0)

	if p.lastStepX != 3299 {
		t.Errorf("lastStepX after Teleport: got %d, want 3299 (x - 1)", p.lastStepX)
	}
	if p.lastStepZ != 3300 {
		t.Errorf("lastStepZ after Teleport: got %d, want 3300 (z)", p.lastStepZ)
	}
}

// TestPlayerTeleport_InPlaceFocusUsesSelfCenter pins the in-place edge case.
// When prev == new, coordgrid.Face returns -1; coordgrid.MoveX/MoveZ no-op
// (DeltaX/Z default-case = 0). focus uses self-center coords:
// Fine(p.x, 1), Fine(p.z, 1). lastStep adjust still applies (x-1, z).
func TestPlayerTeleport_InPlaceFocusUsesSelfCenter(t *testing.T) {
	s := newTestServer(t)
	c, _ := newTestClient(t)
	p := newPlayer(c)
	p.client.server = s
	p.x, p.z, p.level = 3200, 3200, 0
	if err := s.addPlayer(p); err != nil {
		t.Fatalf("addPlayer: %v", err)
	}
	p.faceAngleX = -1
	p.faceAngleZ = -1

	p.Teleport(3200, 3200, 0)

	wantSelf := 3200*64 + 31
	if p.faceAngleX != wantSelf {
		t.Errorf("in-place faceAngleX: got %d, want %d (Fine(3200, 1) self-center)", p.faceAngleX, wantSelf)
	}
	if p.faceAngleZ != wantSelf {
		t.Errorf("in-place faceAngleZ: got %d, want %d (Fine(3200, 1) self-center)", p.faceAngleZ, wantSelf)
	}
	if p.lastStepX != 3199 {
		t.Errorf("in-place lastStepX: got %d, want 3199 (x - 1 still applies)", p.lastStepX)
	}
	if p.lastStepZ != 3200 {
		t.Errorf("in-place lastStepZ: got %d, want 3200", p.lastStepZ)
	}
	if !p.tele {
		t.Error("in-place tele flag: got false, want true")
	}
}

// TestOpenTutorial_SetsFieldsWithoutClosingOthers pins TS-fidelity:
// opening the tutorial overlay does NOT close any other modal.
// TS Player.ts:1999-2003 — `this.modalState |= ModalState.TUT;
// this.modalTutorial = com;`. No clear of modalMain/Chat/Side.
func TestOpenTutorial_SetsFieldsWithoutClosingOthers(t *testing.T) {
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	p, _ := newTestPlayer(t)
	p.client.encryptor = enc
	p.modalMain = 5
	p.modalChat = 7
	p.modalSide = 9
	p.modalState = modalStateMain | modalStateChat | modalStateSide

	p.OpenTutorial(42)

	if p.modalTutorial != 42 {
		t.Errorf("modalTutorial: got %d, want 42", p.modalTutorial)
	}
	wantState := modalStateMain | modalStateChat | modalStateSide | modalStateTut
	if p.modalState != wantState {
		t.Errorf("modalState: got %#x, want %#x", p.modalState, wantState)
	}
	if p.modalMain != 5 {
		t.Errorf("modalMain: got %d, want 5 (must not be cleared)", p.modalMain)
	}
	if p.modalChat != 7 {
		t.Errorf("modalChat: got %d, want 7 (must not be cleared)", p.modalChat)
	}
	if p.modalSide != 9 {
		t.Errorf("modalSide: got %d, want 9 (must not be cleared)", p.modalSide)
	}
}

// TestOpenTutorial_RefreshFlagsUntouched pins that OpenTutorial does
// NOT touch the refreshModal/refreshModalClose flags; those remain
// reserved for the main/chat/side modal switch in encodeOut.
// (Pre-NAI-112 OpenTutorial deferred via the lastModalTutorial diff;
// post-NAI-112 OpenTutorial writes the wire packet directly per TS
// Player.ts:1999-2003 — neither shape involved the refresh flags.)
func TestOpenTutorial_RefreshFlagsUntouched(t *testing.T) {
	enc, _ := isaacPair([4]uint32{5, 6, 7, 8})
	p, _ := newTestPlayer(t)
	p.client.encryptor = enc
	p.refreshModal = false
	p.refreshModalClose = false

	p.OpenTutorial(42)

	if p.refreshModal {
		t.Error("refreshModal should remain false after OpenTutorial")
	}
	if p.refreshModalClose {
		t.Error("refreshModalClose should remain false after OpenTutorial")
	}
}

// TestPlaySynthWritesOut pins NAI-87 T3: (*Player).PlaySynth issues
// a writeOut to the client for the OpSynthSound opcode. Failure
// signal = "wire-out broken or encoder mis-wired."
func TestPlaySynthWritesOut(t *testing.T) {
	p, _ := newTestPlayer(t)
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	p.client.encryptor = enc
	p.PlaySynth(123, 1, 0)
	if n := p.client.bufw.Buffered(); n == 0 {
		t.Errorf("PlaySynth wrote 0 bytes to c.bufw; want >0 (NAI-87 positive pin)")
	}
}

// TestHasInteraction_NoTarget pins (*Player).HasInteraction → false when
// there is no interaction target. NAI-120 Bundle 2B; mirrors TS
// Player.hasInteraction Player.ts:955-957.
func TestHasInteraction_NoTarget(t *testing.T) {
	p, _ := newTestPlayer(t)
	if p.HasInteraction() {
		t.Error("HasInteraction with nil target: got true, want false")
	}
}

// TestHasInteraction_NormalInteraction pins HasInteraction → true for
// a non-follow interaction (e.g., targetOp=1 / OPPLAYER1).
func TestHasInteraction_NormalInteraction(t *testing.T) {
	p, _ := newTestPlayer(t)
	other, _ := newTestPlayer(t)
	p.target = other
	p.targetOp = 1
	if !p.HasInteraction() {
		t.Error("HasInteraction with normal player interaction: got false, want true")
	}
}

// TestHasInteraction_FollowOp_Excluded pins TS-faithful exclusion of the
// follow interaction (targetOp=3 with player target = APPLAYER3/OPPLAYER3).
// Mirrors TS Player.ts:959-962 — "the follow interaction doesn't do
// anything" so HasInteraction reports false. NAI-120 Bundle 2B.
func TestHasInteraction_FollowOp_Excluded(t *testing.T) {
	p, _ := newTestPlayer(t)
	other, _ := newTestPlayer(t)
	p.target = other
	p.targetOp = 3
	if p.HasInteraction() {
		t.Error("HasInteraction with follow-op (targetOp=3, player target): got true, want false (TS Player.ts:959-962)")
	}
}

// TestHasInteraction_Op3_NonPlayerTarget pins that the follow-op exclusion
// only applies to player targets. targetOp=3 against a non-player (e.g., NPC)
// is a regular interaction; reports true. The isFollowOp helper at
// interaction.go:145-151 narrows to *Player targets.
func TestHasInteraction_Op3_NonPlayerTarget(t *testing.T) {
	p, _ := newTestPlayer(t)
	npc := &Npc{}
	p.target = npc
	p.targetOp = 3
	if !p.HasInteraction() {
		t.Error("HasInteraction with op=3 against non-player target: got false, want true (follow-op narrows to *Player)")
	}
}

// TestChangeStatUsesQueueEngine is a direct regression fence for the
// NAI-144 migration: changeStat must enqueue to p.engineQueue with
// Type=QueueEngine, not the previous S6h QueueNormal-as-ENGINE shape.
func TestChangeStatUsesQueueEngine(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	key := script.LookupKeyForType(script.TriggerChangeStat, objtype.PlayerStatAttack)
	sf := &script.ScriptFile{Name: "[changestat,attack]", LookupKey: key}
	s.scriptProvider.Register(sf)

	p, _ := newTestPlayer(t)
	p.client.server = s

	p.changeStat(objtype.PlayerStatAttack)

	if len(p.queue) != 0 {
		t.Errorf("p.queue len: got %d, want 0 (changeStat must NOT land in primary queue)", len(p.queue))
	}
	if len(p.engineQueue) != 1 {
		t.Fatalf("p.engineQueue len: got %d, want 1 (changeStat uses QueueEngine)", len(p.engineQueue))
	}
	if p.engineQueue[0].Type != script.QueueEngine {
		t.Errorf("Type: got %v, want QueueEngine", p.engineQueue[0].Type)
	}
	if p.engineQueue[0].Script != sf {
		t.Errorf("Script: got %v, want %v", p.engineQueue[0].Script, sf)
	}
}

// TestAdvanceStatUsesQueueEngine pins NAI-144 migration: advanceStat
// uses QueueEngine to match TS Player.ts:1804-1807.
func TestAdvanceStatUsesQueueEngine(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	key := script.LookupKeyForType(script.TriggerAdvanceStat, objtype.PlayerStatAttack)
	sf := &script.ScriptFile{Name: "[advancestat,attack]", LookupKey: key}
	s.scriptProvider.Register(sf)

	p, _ := newTestPlayer(t)
	p.client.server = s

	p.advanceStat(objtype.PlayerStatAttack)

	if len(p.queue) != 0 {
		t.Errorf("p.queue len: got %d, want 0 (advanceStat must NOT land in primary queue)", len(p.queue))
	}
	if len(p.engineQueue) != 1 {
		t.Fatalf("p.engineQueue len: got %d, want 1 (advanceStat uses QueueEngine)", len(p.engineQueue))
	}
	if p.engineQueue[0].Type != script.QueueEngine {
		t.Errorf("Type: got %v, want QueueEngine", p.engineQueue[0].Type)
	}
	if p.engineQueue[0].Script != sf {
		t.Errorf("Script: got %v, want %v", p.engineQueue[0].Script, sf)
	}
}
