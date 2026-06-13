package world

// InvAdd mirrors TS Player.invAdd (Player.ts:1543-1550): a bare entity-
// level helper that bypasses the protect/scope/dummyitem gates which
// live in the INV_ADD opcode body (pkg/script.performInvAdd). Used by
// admin cheats (::give / ::givemany / ::givecrap / ::giveother) where
// gates are not desired.
//
// Returns the completed count (units actually inserted). 274: the bare
// invAdd is a partial-fill add — the all-or-nothing assureFullInsertion
// contract was deleted at rev-274 (Inventory.ts @dee467c8) — and, like the
// TS cheats (ClientCheatHandler.ts:354/374/389/404), does NOT drop overflow
// at the player's tile; whatever doesn't fit is silently discarded.
// DEVIATION-NAI-184-D2-INVADD-NIL-RETURN: TS throws on missing inv; goscape
// returns 0 (safer ergonomics for admin cheats — there's no exception-handling
// path to swallow it).
func (p *Player) InvAdd(invTypeID, obj, count int) int {
	srv := p.client.server
	if srv == nil || srv.invTypes == nil || srv.objTypes == nil {
		return 0
	}
	if invTypeID < 0 || invTypeID >= len(srv.invTypes.Configs) {
		return 0
	}
	invType := srv.invTypes.Configs[invTypeID]
	if invType == nil {
		return 0
	}
	if obj < 0 || obj >= len(srv.objTypes.Configs) {
		return 0
	}
	objType := srv.objTypes.Configs[obj]
	if objType == nil {
		return 0
	}
	inv := srv.invLookup.Get(p, invTypeID)
	if inv == nil {
		return 0
	}
	// 274: partial-fill add via the bare 3-arg form (beginSlot=-1). Stockobj
	// retention is derived inside inventory.Remove from the inv's own InvType
	// (matching TS); Add just needs the pre-resolved Stackable predicate.
	return inv.Add(obj, count, -1, objType.Stackable)
}
