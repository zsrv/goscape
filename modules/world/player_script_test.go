package world

import (
	"slices"
	"testing"

	"github.com/zsrv/goscape/pkg/cache"
	"github.com/zsrv/goscape/pkg/gamemap"
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
	key := script.LookupKeyForType(script.TriggerChangeStat, objtype.PlayerStatAttack)
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

	before := len(p.queue)
	p.AddXP(objtype.PlayerStatAttack, 1000) // → level 3

	if len(p.queue) != before+1 {
		t.Fatalf("queue len: got %d, want %d (+1 advancestat)", len(p.queue), before+1)
	}
	req := p.queue[before]
	if req.Script != sf {
		t.Errorf("queue[%d].Script: got %v, want [advancestat,attack] (%v)", before, req.Script, sf)
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

	before := len(p.queue)
	p.AddXP(objtype.PlayerStatAttack, 100) // → 200, still level 1

	if len(p.queue) != before {
		t.Errorf("queue len: got %d, want %d (no level-up = no advancestat fire)",
			len(p.queue), before)
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

	before := len(p.queue)
	p.AddXP(objtype.PlayerStatAttack, 1000) // level up

	if len(p.queue) != before {
		t.Errorf("queue len: got %d, want %d (global script must NOT fire — advancestat is type-specific only)",
			len(p.queue), before)
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

	before := len(p.queue)
	p.AddXP(objtype.PlayerStatAttack, 1000) // level up

	if len(p.queue) != before+2 {
		t.Fatalf("queue len: got %d, want %d (+2 — both changestat and advancestat)",
			len(p.queue), before+2)
	}
	// Order: changeStat before advanceStat (matches TS Player.ts:1772, 1804).
	if p.queue[before].Script != changeSF {
		t.Errorf("queue[%d].Script: got %v, want changestat first", before, p.queue[before].Script)
	}
	if p.queue[before+1].Script != advSF {
		t.Errorf("queue[%d].Script: got %v, want advancestat second", before+1, p.queue[before+1].Script)
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
