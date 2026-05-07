package world

// HeroPoints is the per-NPC ledger that tracks each player's damage
// contribution (or other hero-point credit) toward the NPC. The largest
// contributor at death gets the loot. Capped at 16 entries — TS uses
// `new HeroPoints(16)` for combat NPCs (Engine-TS/.../HeroPoints.ts).
//
// Lookups and writes are O(N) over the slice — N <= 16.
//
// TS uses a pre-allocated TypedArray of 16 sentinel entries
// ({hash64: -1n, points: 0}) and marks a slot as "empty" when hash64=-1n.
// goscape uses a simpler grow-from-empty slice with a cap field; this is
// observably equivalent under TS's drop-when-full semantics (no eviction).
//
// Read by World on NPC death to choose the loot recipient. The death-side
// reader is OUT OF SCOPE for NAI-120 — only the writer (NPC_HEROPOINTS
// opcode) lands here. NAI-120 Bundle 2D.
type HeroPoints struct {
	cap     int
	entries []heroEntry
}

type heroEntry struct {
	playerUID int
	amount    int
}

// NewHeroPoints constructs an empty HeroPoints with the given cap.
func NewHeroPoints(cap int) HeroPoints {
	return HeroPoints{cap: cap}
}

// AddHero credits `amount` to `playerUID`. Mirrors TS HeroPoints.addHero
// (Engine-TS/src/engine/entity/HeroPoints.ts):
//
//  1. Short-circuits when amount < 1 (TS: `if (points < 1) return`).
//  2. If the player already has an entry, increments it and returns.
//  3. If the ledger has room, inserts a new entry.
//  4. If the ledger is full, the incoming write is silently dropped —
//     TS performs NO eviction (plan originally suggested eviction;
//     corrected per TS source read on 2026-05-07, NAI-120 Bundle 2D).
func (h *HeroPoints) AddHero(playerUID, amount int) {
	if amount < 1 {
		return
	}
	for i := range h.entries {
		if h.entries[i].playerUID == playerUID {
			h.entries[i].amount += amount
			return
		}
	}
	if len(h.entries) < h.cap {
		h.entries = append(h.entries, heroEntry{playerUID, amount})
	}
	// Full: silently drop (TS-faithful — no eviction).
}

// TopContributor returns the playerUID with the highest contribution, or 0
// if the ledger is empty. (Stub for future loot-routing consumer.)
func (h *HeroPoints) TopContributor() int {
	if len(h.entries) == 0 {
		return 0
	}
	bestIdx := 0
	for i := 1; i < len(h.entries); i++ {
		if h.entries[i].amount > h.entries[bestIdx].amount {
			bestIdx = i
		}
	}
	return h.entries[bestIdx].playerUID
}
