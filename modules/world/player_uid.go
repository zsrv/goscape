package world

// composeUID derives a Player.uid from username37 + slot.
//
// Mirrors TS Engine-TS World.ts:937:
//
//	player.uid = ((Number(player.username37 & 0x1fffffn) << 11) | player.slot) >>> 0;
//
// The lower 21 bits of username37 are shifted up 11 bits; the 11-bit
// slot occupies the low bits. Stable per (account, slot) for the
// session. Goscape masks slot to 11 bits defensively (TS slot is ≤2047
// by construction); username37 is masked to 21 bits matching TS.
//
// Single source of truth for the formula. Production callers:
// Server.addPlayer. Test callers: newInvListenerTestPlayer (and any
// future test fixture that needs a deterministic per-player uid).
func composeUID(username37 uint64, slot int) int {
	return int(((username37 & 0x1FFFFF) << 11) | uint64(slot&0x7FF))
}
