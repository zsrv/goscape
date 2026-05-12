package world

import (
	"github.com/zsrv/goscape/pkg/inventory"
)

// InvAdd mirrors TS Player.invAdd (Player.ts:1496-1504): a bare entity-
// level helper that bypasses the protect/scope/dummyitem gates which
// live in the INV_ADD opcode body (pkg/script.performInvAdd). Used by
// admin cheats (::give / ::givemany / ::givecrap / ::giveother) where
// gates are not desired.
//
// Returns the transaction.completed count (units actually inserted).
// Unlike performInvAdd, this method does NOT drop overflow at the
// player's tile — TS Player.invAdd is bare and the cheats silently
// discard whatever doesn't fit. DEVIATION-NAI-184-D2-INVADD-NIL-RETURN:
// TS throws on missing inv; goscape returns 0 (safer ergonomics for
// admin cheats — there's no exception-handling path to swallow it).
func (p *Player) InvAdd(invTypeID, obj, count int, assureFullInsertion bool) int {
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
	stockObj := false
	for _, id := range invType.StockObj {
		if int(id) == obj {
			stockObj = true
			break
		}
	}
	tx := inv.Add(obj, count, inventory.AddOpts{
		BeginSlot:           -1,
		AssureFullInsertion: assureFullInsertion,
		Stackable:           objType.Stackable,
		StockObj:            stockObj,
	})
	return tx.Completed
}
