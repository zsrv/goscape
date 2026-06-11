package world

import (
	"slices"

	"github.com/zsrv/goscape/pkg/io/packet"
	"github.com/zsrv/goscape/pkg/objtype"
)

// Save serializes p to a fresh SAV byte slice at version SavVersion
// (v7 since rev-254 A6). Inventories iterate over typeIds in ascending
// order (deviation NAI-PLAYERLOADING-D-INVS-SORTED-BY-TYPEID). Mirrors
// Player.save() at Engine-TS Player.ts:192-277 @2e3bcf43.
//
// invTypes is consulted to skip non-SCOPE_PERM inventories at encode
// time. Pass nil to encode every entry in p.invs unconditionally
// (test setups; production should always pass the real config).
//
// Varps (rev-254 A6, SAV v7): sparse encoding — only non-zero
// SCOPE_PERM varps are saved, as p2(id) + pVarInt(value) pairs behind
// a p2 count (TS Player.ts:215-229 @2e3bcf43). This subsumes NAI-220
// (the v6-era zero-out of non-PERM slots, which had fixed the
// %lastcombat / %aggressive_npc stale-temp-varp combat bug): temp
// scope varps are now simply absent from the save. Nil-tolerant: if
// varpTypes is nil (test fixtures that don't model scope), every
// non-zero varp is saved.
func (p *Player) Save(invTypes *objtype.InvTypeConfigs, varpTypes *objtype.VarpTypeConfigs) []byte {
	pkt := packet.NewPacket(make([]byte, 0, 1500))
	pkt.P2(SavMagic)
	pkt.P2(SavVersion)
	pkt.P2(uint16(p.x))
	pkt.P2(uint16(p.z))
	pkt.P1(uint8(p.level))
	for i := range 7 {
		pkt.P1(uint8(p.body[i])) // -1 → 0xFF via two's-complement
	}
	for i := range 5 {
		pkt.P1(uint8(p.colors[i]))
	}
	pkt.P1(uint8(p.gender))
	pkt.P2(uint16(p.runenergy))
	pkt.P4(uint32(p.playtime))

	for i := range objtype.PlayerStatCount {
		pkt.P4(uint32(p.stats[i]))
		pkt.P1(p.levels[i])
	}

	// Varps: rev-254 A6 sparse encoding (SAV v7; TS Player.save
	// @2e3bcf43 Player.ts:215-229): p2 count of non-zero SCOPE_PERM
	// varps, then per saved varp p2(id) + pVarInt(value). Non-PERM and
	// zero-valued varps are never written — the v6 dense
	// zero-out-non-PERM shape (NAI-220) is subsumed by simply skipping
	// them. Nil-tolerance keeps the pre-A6 convention: a nil varpTypes,
	// an out-of-range id, or a nil config slot is treated as PERM (test
	// fixtures that don't model scope save non-zero values verbatim).
	saved := 0
	for id, v := range p.varps {
		if v != 0 && varpScopeIsPerm(varpTypes, id) {
			saved++
		}
	}
	pkt.P2(uint16(saved))
	for id, v := range p.varps {
		if v != 0 && varpScopeIsPerm(varpTypes, id) {
			pkt.P2(uint16(id))
			pkt.PVarInt(v)
		}
	}

	// Inventories. Placeholder count, then per-inv body, then backfill
	// the count once we know how many we wrote. Iterate typeIds in
	// ascending order (NAI-PLAYERLOADING-D-INVS-SORTED-BY-TYPEID).
	invCountPos := len(pkt.Data)
	pkt.P1(0) // placeholder
	typeIDs := make([]int, 0, len(p.invs))
	for tid := range p.invs {
		typeIDs = append(typeIDs, tid)
	}
	slices.Sort(typeIDs)
	invCount := 0
	for _, tid := range typeIDs {
		if invTypes != nil && tid < len(invTypes.Configs) {
			if cfg := invTypes.Configs[tid]; cfg != nil && cfg.Scope != objtype.InvTypeScopePerm {
				continue
			}
		}
		inv := p.invs[tid]
		pkt.P2(uint16(tid))
		pkt.P2(uint16(inv.Capacity)) // v5+ per-inv size
		for slot := range inv.Capacity {
			item := inv.Get(slot)
			if item == nil {
				pkt.P2(0)
				continue
			}
			pkt.P2(uint16(item.Id + 1))
			if item.Count >= 255 {
				pkt.P1(255)
				pkt.P4(uint32(item.Count))
			} else {
				pkt.P1(uint8(item.Count))
			}
		}
		invCount++
	}
	pkt.Data[invCountPos] = byte(invCount)

	// v3+ afk zones — current SavVersion=6 always writes this section.
	pkt.P1(uint8(len(p.afkZones)))
	for _, v := range p.afkZones {
		pkt.P4(uint32(v))
	}
	pkt.P2(uint16(p.lastAfkZone))

	// v4+ packed chat modes.
	packed := uint8((p.publicChat&0b11)<<4 | (p.privateChat&0b11)<<2 | (p.tradeDuel & 0b11))
	pkt.P1(packed)

	// v6+ lastLoginTime.
	pkt.P8(uint64(p.lastLoginTime))

	// Trailing CRC over [0, len).
	crc := packet.GetCRC(pkt.Data, 0, len(pkt.Data))
	pkt.P4(crc)
	return pkt.Data
}

// varpScopeIsPerm reports whether varp id should be treated as
// SCOPE_PERM for save purposes. TS Player.save consults
// VarPlayerType.get(id).scope (Player.ts:217-228 @2e3bcf43); goscape's
// nil-tolerance convention (nil varpTypes / out-of-range id / nil
// config slot → PERM) is kept from the pre-A6 dense writer so test
// fixtures that don't model scope still round-trip values.
func varpScopeIsPerm(varpTypes *objtype.VarpTypeConfigs, id int) bool {
	if varpTypes == nil || id >= len(varpTypes.Configs) {
		return true
	}
	vt := varpTypes.Configs[id]
	return vt == nil || vt.Scope == objtype.VarpScopePerm
}
