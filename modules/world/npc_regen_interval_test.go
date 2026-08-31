package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
)

// --- A8 / dbfb82be "fix: NPC stat regen (#74)" @2e3bcf43 -------------------
//
// TS Npc.ts:511-531 @2e3bcf43:
//
//	private processRegen() {
//	    if (++this.regenClock >= this.regenInterval) {
//	        // Every time we regen, let's reload regen interval from NPC type
//	        // This seems to match NPC behavior for when they change type, the
//	        // regenrate doesn't update until a regen happens
//	        // See: Vorkath in OSRS
//	        const type = NpcType.get(this.type);
//	        this.regenInterval = type.regenrate;
//	        this.regenClock = 0;
//	        ...converge levels toward baseLevels...
//	    }
//	}

func newRegenNpc(t *testing.T, regenRate int) (*Server, *Npc) {
	t.Helper()
	s := newTestServer(t)
	typ := &objtype.NpcType{
		ID: 1, DebugName: "regen",
		RegenRate: regenRate,
	}
	n := newRegisteredNpc(t, s, typ, false)
	return s, n
}

// TestNpcRegenFirstTurnProcsAndCachesInterval pins the count-up contract:
// clock and interval both init 0 → first call procs (1 >= 0), caches
// interval = type.regenrate, resets clock to 0.
func TestNpcRegenFirstTurnProcsAndCachesInterval(t *testing.T) {
	s, n := newRegenNpc(t, 5)
	n.baseLevels[0] = 10
	n.levels[0] = 7

	s.processNpcRegen(n)

	if n.levels[0] != 8 {
		t.Errorf("levels[0]: got %d, want 8 (first-turn proc converges +1)", n.levels[0])
	}
	if n.regenInterval != 5 {
		t.Errorf("regenInterval: got %d, want 5 (cached from type at proc)", n.regenInterval)
	}
	if n.regenClock != 0 {
		t.Errorf("regenClock: got %d, want 0 (reset at proc)", n.regenClock)
	}

	// Next 4 calls (clock 1..4 < interval 5): no proc.
	for range 4 {
		s.processNpcRegen(n)
	}
	if n.levels[0] != 8 {
		t.Errorf("levels[0]: got %d, want 8 (no proc before clock reaches interval)", n.levels[0])
	}
	// 5th call: clock hits 5 >= 5 → proc.
	s.processNpcRegen(n)
	if n.levels[0] != 9 {
		t.Errorf("levels[0]: got %d, want 9 (proc at clock == interval)", n.levels[0])
	}
}

// TestNpcRegenIntervalRefreshOnlyAtExpiry pins the Vorkath quirk: a
// type change mid-interval does NOT shorten/lengthen the CURRENT period
// — the new regenrate is adopted only when the old interval expires.
func TestNpcRegenIntervalRefreshOnlyAtExpiry(t *testing.T) {
	s, n := newRegenNpc(t, 5)
	n.baseLevels[0] = 10
	n.levels[0] = 1

	s.processNpcRegen(n) // first-turn proc → interval 5, levels 2

	// Simulate changeType: live type now regens every 2 ticks.
	n.typ = &objtype.NpcType{
		ID: 2, DebugName: "regen-fast",
		RegenRate: 2,
	}

	// Old interval (5) still governs: calls 1..4 no proc.
	for i := range 4 {
		s.processNpcRegen(n)
		if n.levels[0] != 2 {
			t.Fatalf("levels[0]: got %d, want 2 (old interval must keep governing, call %d)", n.levels[0], i+1)
		}
	}
	// 5th call: proc fires AND adopts the new rate.
	s.processNpcRegen(n)
	if n.levels[0] != 3 {
		t.Errorf("levels[0]: got %d, want 3 (proc at old interval)", n.levels[0])
	}
	if n.regenInterval != 2 {
		t.Errorf("regenInterval: got %d, want 2 (new rate adopted AT the proc)", n.regenInterval)
	}
	// New cadence: 2 calls per proc now.
	s.processNpcRegen(n)
	s.processNpcRegen(n)
	if n.levels[0] != 4 {
		t.Errorf("levels[0]: got %d, want 4 (new 2-tick cadence)", n.levels[0])
	}
}

// TestNpcRegenZeroRateConvergesEveryTick pins the rev-254 contract change:
// regenrate 0 no longer disables regen (the 244 `type.regenrate !== 0`
// short-circuit is gone at the pin) — interval caches 0, so every call
// procs. NpcType's default regenrate is 100 (TS NpcType.ts:100 @2e3bcf43),
// so an explicit 0 is a pack-author opt-in.
func TestNpcRegenZeroRateConvergesEveryTick(t *testing.T) {
	s, n := newRegenNpc(t, 0)
	n.baseLevels[0] = 10
	n.levels[0] = 5

	for range 3 {
		s.processNpcRegen(n)
	}
	if n.levels[0] != 8 {
		t.Errorf("levels[0]: got %d, want 8 (regenrate 0 → proc every tick at the pin)", n.levels[0])
	}
}
