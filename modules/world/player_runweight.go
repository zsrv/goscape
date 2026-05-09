package world

// calculateRunWeight recomputes p.runweight from all RunWeight=true
// invs that the player owns, summing ObjType.Weight * Count over
// non-stackable items only. Stackable items contribute 0 (TS Player.ts:620-622).
//
// Mirrors TS Engine-TS/src/engine/entity/Player.ts:598-627
// line-for-line. Called from (*Player).updateInvs when a
// runweight-flagged listener fires; also safe to call directly for
// tests / forced recompute.
//
// Defensive nil-server guard (goscape defensive; TS uses static
// InvType/ObjType imports).
//
// NAI-136.
func (p *Player) calculateRunWeight() {
	p.runweight = 0
	if p.client == nil || p.client.server == nil {
		return
	}
	srv := p.client.server
	if srv.invTypes == nil || srv.objTypes == nil {
		return
	}
	invConfigs := srv.invTypes.Configs
	objConfigs := srv.objTypes.Configs
	for _, inv := range p.invs {
		if inv == nil {
			continue
		}
		if inv.Type < 0 || inv.Type >= len(invConfigs) {
			continue
		}
		invType := invConfigs[inv.Type]
		if invType == nil || !invType.RunWeight {
			continue
		}
		for slot := range inv.Capacity {
			item := inv.Get(slot)
			if item == nil {
				continue
			}
			if item.Id < 0 || item.Id >= len(objConfigs) {
				continue
			}
			objType := objConfigs[item.Id]
			if objType == nil || objType.Stackable {
				continue
			}
			p.runweight += objType.Weight * item.Count
		}
	}
}
