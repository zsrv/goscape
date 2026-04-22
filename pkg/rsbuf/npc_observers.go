package rsbuf

// npcObservers counts, per-NPC, the number of players currently
// subscribed to that NPC via NpcInfo. Mirrors the @2004scape/rsbuf
// WASM's internal state backing the getNpcObservers(nid) public API
// (see Engine-TS/node_modules/@2004scape/rsbuf/dist/rsbuf.d.ts:13).
//
// Maintained at three sites in npcinfo.go's EncodeNpc:
//   - incNpcObserver on subscription-add (line ~108)
//   - decNpcObserver on subscription-remove (inactive-path, ~line 39)
//   - decNpcObserver on subscription-remove (out-of-range, ~line 46)
// And in bulk via RemovePlayer on player logout.
//
// Read by consumers via GetNpcObservers — currently called from
// modules/world/npc_hunt.go's PAUSEHUNT gate.
var npcObservers = map[int]int{}

// GetNpcObservers returns the number of players currently subscribed
// to this NPC via NpcInfo. Returns 0 for any nid never observed or
// whose count floored at zero. Public API; mirrors
// @2004scape/rsbuf's getNpcObservers(nid) at rsbuf.d.ts:13.
func GetNpcObservers(nid int) int { return npcObservers[nid] }

// RemovePlayer performs bulk-decrement of observer counts for every
// NPC in the player's subscription set. Mirrors @2004scape/rsbuf's
// removePlayer(pid) at rsbuf.d.ts:6, whose WASM internals iterate
// the player's build.npcs set and decrement each NPC's observer
// count.
//
// Caller supplies the subscription set because goscape's pkg/rsbuf
// doesn't own per-player BuildArea state (see deviation D5 in
// the NAI-9 design spec). pid is unused in the goscape
// implementation but kept for API-shape parity with TS.
//
// Safe to call with a nil or empty set.
func RemovePlayer(pid int, subscribedNpcs map[int]struct{}) {
	for nid := range subscribedNpcs {
		decNpcObserver(nid)
	}
}

// SetObserverForTest is a test-only helper that directly writes an
// observer count. Used by tests in modules/world that need to seed
// a specific count (e.g., PAUSEHUNT gate tests) without going
// through the full EncodeNpc pipeline.
//
// NOT for production use. Present on the public API surface only
// because cross-package tests in modules/world need to reach it.
func SetObserverForTest(nid, count int) {
	if count <= 0 {
		delete(npcObservers, nid)
		return
	}
	npcObservers[nid] = count
}

// incNpcObserver increments nid's observer count. Unexported;
// called only from EncodeNpc.
func incNpcObserver(nid int) { npcObservers[nid]++ }

// decNpcObserver decrements nid's observer count, flooring at 0.
// Matches @2004scape/rsbuf semantics (Math.max(x - 1, 0) observable
// from the TS shim). Unexported; called from EncodeNpc remove sites
// and RemovePlayer.
func decNpcObserver(nid int) {
	if v := npcObservers[nid]; v > 0 {
		npcObservers[nid] = v - 1
	}
}
