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
