package world

import (
	"slices"

	"github.com/zsrv/goscape/pkg/io/packet"
	"github.com/zsrv/goscape/pkg/objtype"
)

// Save serializes p to a fresh SAV byte slice at version SavVersion.
// Inventories iterate over typeIds in ascending order (deviation
// NAI-PLAYERLOADING-D-INVS-SORTED-BY-TYPEID). Varps are written
// verbatim from p.varps (deviation
// NAI-PLAYERLOADING-D-SAVE-VARPS-VERBATIM — TS Player.save() zeros
// non-SCOPE_PERM slots; goscape trusts the in-memory slice and relies
// on the runtime not writing to temp-scope slots). Mirrors
// Player.save() at Engine-TS/.../Player.ts:190-270.
//
// invTypes is consulted to skip non-SCOPE_PERM inventories at encode
// time. Pass nil to encode every entry in p.invs unconditionally
// (test setups; production should always pass the real config).
//
// NAI-220 closes the previous NAI-PLAYERLOADING-D-SAVE-VARPS-VERBATIM
// deviation. TS Player.save() writes 0 for any varp slot whose
// VarpType.Scope != PERM; goscape now matches via the varpTypes
// scope check below. Pre-NAI-220 goscape wrote every value verbatim,
// which let combat-related temp varns like %lastcombat and
// %aggressive_npc persist across save/load and accumulate stale state
// that broke player_in_combat_check on existing accounts — new
// accounts (no save yet) worked because they started with zeroed
// varps, while existing accounts hit "I'm already under attack!" on
// every attempted retarget because the saved %lastcombat was within
// +8 ticks of the in-RAM map_clock after reload.
//
// Nil-tolerant: if varpTypes is nil (test fixtures that don't model
// scope), Save falls back to verbatim — preserves the pre-NAI-220
// shape for those tests.
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

	// Varps scope-filtered (NAI-220, TS-faithful). Non-PERM scope varps
	// are written as 0; PERM (and the nil-varpTypes fallback) write
	// verbatim. See the Save doc-comment for the rationale.
	pkt.P2(uint16(len(p.varps)))
	for i, v := range p.varps {
		out := v
		if varpTypes != nil && i < len(varpTypes.Configs) {
			if vt := varpTypes.Configs[i]; vt != nil && vt.Scope != objtype.VarpScopePerm {
				out = 0
			}
		}
		pkt.P4(uint32(out))
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
