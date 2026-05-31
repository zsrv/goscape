package world

import "testing"

// TestHeroPoints_AmountZeroNoOp verifies AddHero is a no-op when amount == 0.
// Mirrors TS HeroPoints.addHero `if (points < 1) return` — amount 0 is
// treated as non-contribution. NAI-120 Bundle 2D.
func TestHeroPoints_AmountZeroNoOp(t *testing.T) {
	h := NewHeroPoints(16)
	h.AddHero(1, 0)
	if len(h.entries) != 0 {
		t.Errorf("AddHero(uid=1, amount=0): expected no entry, got %d", len(h.entries))
	}
}

// TestHeroPoints_AmountNegativeNoOp verifies AddHero is a no-op when amount < 0.
// TS: `if (points < 1) return` covers all negative values. NAI-120 Bundle 2D.
func TestHeroPoints_AmountNegativeNoOp(t *testing.T) {
	h := NewHeroPoints(16)
	h.AddHero(1, -5)
	if len(h.entries) != 0 {
		t.Errorf("AddHero(uid=1, amount=-5): expected no entry, got %d", len(h.entries))
	}
}

// TestHeroPoints_DuplicateUIDCumulative verifies that a second AddHero for the
// same player accumulates rather than inserting a new entry.
func TestHeroPoints_DuplicateUIDCumulative(t *testing.T) {
	h := NewHeroPoints(16)
	h.AddHero(42, 10)
	h.AddHero(42, 30)
	if len(h.entries) != 1 {
		t.Fatalf("duplicate uid: expected 1 entry, got %d", len(h.entries))
	}
	if got := h.entries[0].amount; got != 40 {
		t.Errorf("duplicate uid: cumulative amount = %d, want 40", got)
	}
}

// TestHeroPoints_FullDropsNewEntry verifies that when the ledger is at capacity
// a new (distinct) player's write is silently dropped. TS performs NO eviction.
// NAI-120 Bundle 2D.
func TestHeroPoints_FullDropsNewEntry(t *testing.T) {
	h := NewHeroPoints(2)
	h.AddHero(1, 100)
	h.AddHero(2, 200)
	// Ledger is now full (cap=2).
	h.AddHero(3, 9999) // should be dropped, even though 9999 > any existing entry
	if len(h.entries) != 2 {
		t.Fatalf("full ledger: expected 2 entries, got %d", len(h.entries))
	}
	// Existing entries must be unmodified.
	for _, e := range h.entries {
		if e.playerUID == 3 {
			t.Error("full ledger: entry for uid=3 must not exist (no eviction)")
		}
	}
}

// TestHeroPoints_Clear pins (*HeroPoints).Clear() — resets the
// contributor ledger to zero entries. Mirrors TS HeroPoints.clear()
// called from NPC_STATHEAL HP-full branch (NpcOps.ts:255).
func TestHeroPoints_Clear(t *testing.T) {
	hp := NewHeroPoints(10)
	hp.AddHero(101, 50)
	hp.AddHero(202, 30)
	if got := len(hp.entries); got != 2 {
		t.Fatalf("setup: want 2 entries, got %d", got)
	}

	hp.Clear()

	if got := len(hp.entries); got != 0 {
		t.Errorf("after Clear: want 0 entries, got %d", got)
	}
}

// TestHeroPoints_Clear_Empty pins that Clear() on an empty ledger
// is a safe no-op (no panic).
func TestHeroPoints_Clear_Empty(t *testing.T) {
	hp := NewHeroPoints(10)
	hp.Clear()
	if got := len(hp.entries); got != 0 {
		t.Errorf("want 0 entries, got %d", got)
	}
}

// TestHeroPoints_TopContributor verifies the TopContributor helper returns the
// uid with the highest total contribution.
func TestHeroPoints_TopContributor(t *testing.T) {
	h := NewHeroPoints(16)
	if tc := h.TopContributor(); tc != 0 {
		t.Errorf("empty ledger: TopContributor = %d, want 0", tc)
	}
	h.AddHero(10, 50)
	h.AddHero(20, 100)
	h.AddHero(30, 75)
	if tc := h.TopContributor(); tc != 20 {
		t.Errorf("TopContributor = %d, want 20", tc)
	}
}

// TestHeroPoints_TopContributor_TiedMaxUsesRSQuicksortTiebreak pins util-1:
// when multiple ledger entries are tied at the max amount, the loot
// recipient is selected by the RuneScape-authentic quicksort (TS
// HeroPoints.findHero @ HeroPoints.ts:37-47 + QuickSort.ts:9-36), NOT
// by linear first-max order.
//
// Fixture: insertion order uids [1,2,3,4] with amounts [100,50,100,50],
// cap=16 sentinel-padded clone. The two tied-max entries (uids 1 and 3)
// are at non-adjacent insertion positions, so the sentinel-padded
// quicksort's parity tiebreak resolves the tie differently from linear
// first-max — hand-traced reference:
//
//   • Linear first-max: picks uid 1 (first amount=100 in insertion order).
//   • TS RS-quicksort (sentinel-padded): picks uid 3.
//
// The non-adjacent placement of the tied-max entries is the
// load-bearing fixture property — adjacent tied-max entries
// (e.g. [{1:100},{2:100},{3:50}]) happen to round-trip back to
// insertion order through the parity-driven partition and so
// coincidentally agree with linear first-max.
//
// Pre-fix RED — linear returns uid 1. Post-fix GREEN — RS-quicksort
// returns uid 3.
func TestHeroPoints_TopContributor_TiedMaxUsesRSQuicksortTiebreak(t *testing.T) {
	h := NewHeroPoints(16)
	h.AddHero(1, 100)
	h.AddHero(2, 50)
	h.AddHero(3, 100)
	h.AddHero(4, 50)

	got := h.TopContributor()
	if got == 2 || got == 4 {
		t.Fatalf("TopContributor = %d, want one of {1,3} — sort failed to put max-amount entries first (regression)", got)
	}
	if got != 3 {
		t.Errorf("TopContributor (insertion [1,2,3,4]@[100,50,100,50], cap=16) = %d, want 3 (TS HeroPoints.findHero / QuickSort.ts parity tiebreak; pre-fix linear-first-max returned 1)", got)
	}
}

// TestPlayerHeroPointsClear pins the real (*Player).HeroPointsClear()
// method: clears the player's heroPoints ledger so TopContributor()
// returns 0 after the call. Implements script.ActivePlayer.HeroPointsClear.
// NAI-120 Bundle 2D follow-up.
func TestPlayerHeroPointsClear(t *testing.T) {
	p := &Player{heroPoints: NewHeroPoints(16)}
	p.heroPoints.AddHero(1, 5)
	p.heroPoints.AddHero(2, 3)
	if got := p.heroPoints.TopContributor(); got != 1 {
		t.Fatalf("setup: TopContributor() = %d, want 1", got)
	}
	p.HeroPointsClear()
	if got := p.heroPoints.TopContributor(); got != 0 {
		t.Errorf("after HeroPointsClear: TopContributor() = %d, want 0", got)
	}
}
