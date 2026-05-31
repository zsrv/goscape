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

// Clear zeroes the contributor ledger. Mirrors TS HeroPoints.clear()
// invoked from NPC_STATHEAL HP-full branch at NpcOps.ts:255.
func (h *HeroPoints) Clear() {
	h.entries = h.entries[:0]
}

// TopContributor returns the playerUID with the highest contribution, or 0
// if the ledger is empty. Consumed by FINDHERO at
// pkg/script/handlers_npc.go:1356 (NPC_FINDHERO) and handlers_player.go:1670
// (FINDHERO from a player script).
//
// Mirrors TS HeroPoints.findHero (HeroPoints.ts:37-47): clones the ledger
// padded to cap with sentinel-equivalent zero-value entries, runs the
// RuneScape-authentic quicksort (TS QuickSort.ts:9-36) with descending-by-
// amount + parity tiebreak, then returns the playerUID at clone[0]. The
// sentinel padding is load-bearing for the RS-authentic tiebreak: TS sorts
// the full 16-slot array including empty slots, and the parity-driven
// partition swaps real entries into different positions depending on
// whether sentinels are present. Without padding, ties among real entries
// can resolve differently from TS.
//
// Pre-fix this was a linear first-max scan, which on ties returned the
// FIRST-inserted max — not the RS-quicksort tiebreak (util-1).
//
// The 0-empty-sentinel deviates from TS findHero's `-1n` return on a
// fully-empty ledger; the goscape caller checks for 0, so the cross-
// language sentinel translation is preserved here. Tracked separately
// as entity-base-8.
func (h *HeroPoints) TopContributor() int {
	if len(h.entries) == 0 {
		return 0
	}
	// Clone padded to cap mirrors TS `clone = [...this]` over the 16-slot
	// Array<Hero>. Empty slots become zero-value heroEntry
	// (playerUID=0, amount=0) — the goscape sentinel equivalent of TS's
	// {hash64:-1n, points:0}. Since AddHero rejects amount<1, real entries
	// always have amount≥1, so a sentinel cannot win the descending-by-
	// amount sort once at least one real entry is present.
	clone := make([]heroEntry, h.cap)
	copy(clone, h.entries)
	runescapeQuicksortByPointsDesc(0, len(clone)-1, clone)
	return clone[0].playerUID
}

// runescapeQuicksortByPointsDesc is the RuneScape-authentic quicksort
// algorithm used by TS HeroPoints.findHero. Mirrors TS QuickSort.ts:9-36
// with the comparator inlined as descending-by-amount and the parity
// tiebreak `compare(arr[loop_index], pivot) < (loop_index & 1)`. The
// parity rule is the load-bearing detail — it distributes equal-amount
// elements based on their index parity rather than producing a stable
// sort, so the result on ties differs from a plain stable sort and
// matches the original RS-authentic ranking.
func runescapeQuicksortByPointsDesc(low, high int, arr []heroEntry) {
	if low >= high {
		return
	}
	pivotIndex := (low + high) / 2 // matches TS `~~((low+high)/2)` floor div
	pivotValue := arr[pivotIndex]
	arr[pivotIndex] = arr[high]
	arr[high] = pivotValue
	counter := low
	for loopIndex := low; loopIndex < high; loopIndex++ {
		// TS comparator (b,a) => b.points - a.points applied as
		// compare(arr[loop_index], pivot) = pivot.amount - arr[loop_index].amount.
		cmp := pivotValue.amount - arr[loopIndex].amount
		if cmp < (loopIndex & 1) {
			arr[loopIndex], arr[counter] = arr[counter], arr[loopIndex]
			counter++
		}
	}
	arr[high] = arr[counter]
	arr[counter] = pivotValue

	if low < counter-1 {
		runescapeQuicksortByPointsDesc(low, counter-1, arr)
	}
	if counter+1 < high {
		runescapeQuicksortByPointsDesc(counter+1, high, arr)
	}
}
