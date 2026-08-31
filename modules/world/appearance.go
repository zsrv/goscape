package world

import (
	"github.com/zsrv/goscape/pkg/inventory"
	"github.com/zsrv/goscape/pkg/io/packet"
	"github.com/zsrv/goscape/pkg/objtype"
)

// slotToBodyTable maps worn-inventory slot -> body-part index in Player.body[].
var slotToBodyTable = map[int]int{
	8:  0, // head
	11: 1, // jaw
	4:  2, // torso
	6:  3, // arms
	9:  4, // hands
	7:  5, // legs
	10: 6, // feet
}

// generateAppearance writes p.appearanceBuf using the worn inventory + body/colors.
// Mirrors LostCityRS/Engine-TS Player.generateAppearance().
func (p *Player) generateAppearance(objs *objtype.ObjTypeConfigs, invs *objtype.InvTypeConfigs, currentTick int) {
	buf := packet.NewPacket(nil)

	// Production flow: client.go's sendLoginOK calls
	// p.SetAppearanceInv(invs.Worn) immediately after newPlayer, so by the
	// time any tick runs, p.appearanceInv is already bound. The -1 sentinel
	// default in newPlayer is retained as test-only safety: tests that build
	// a Player via newPlayer(c) without going through sendLoginOK have
	// appearanceInv == -1, and the fallback below maps that to invs.Worn for
	// byte-equivalent behavior. Mirrors TS Player.ts:1318:
	// `let worn = this.getInventory(this.appearanceInv);`.
	var worn *inventory.Inventory
	if p.invs != nil {
		inventoryId := p.appearanceInv
		if inventoryId < 0 {
			inventoryId = invs.Worn
		}
		worn = p.invs[inventoryId]
	}

	skipped := map[int]bool{}
	if worn != nil {
		for _, it := range worn.Items {
			if it == nil {
				continue
			}
			if it.Id < 0 || it.Id >= len(objs.Configs) {
				continue
			}
			ot := objs.Configs[it.Id]
			if ot == nil {
				continue
			}
			if ot.WearPos2 >= 0 {
				skipped[ot.WearPos2] = true
			}
			if ot.WearPos3 >= 0 {
				skipped[ot.WearPos3] = true
			}
		}
	}

	buf.P1(uint8(p.gender))
	buf.P1(uint8(p.headicons))

	for slot := range 12 {
		// Transmogrification: when npcId is set the whole 12-slot equipment
		// region is replaced by a single -1 sentinel followed by the npc id,
		// and the loop stops. TS Player.ts:1390-1395 @1d25566c:
		//
		//	if (this.npcId != -1) {
		//	    stream.p2(-1);
		//	    stream.p2(this.npcId);
		//	    break;
		//	}
		//
		// Engine-TS 8139461a implemented this in place of the long-standing
		// `// todo: transmog support` comment. The write sits INSIDE the loop
		// and breaks immediately, so exactly one pair is emitted — not one
		// per slot.
		if p.npcId != -1 {
			buf.P2(0xffff)
			buf.P2(uint16(p.npcId))
			break
		}
		if skipped[slot] {
			buf.P1(0)
			continue
		}
		var equipped *inventory.Item
		if worn != nil && slot < len(worn.Items) {
			equipped = worn.Items[slot]
		}
		if equipped != nil {
			buf.P2(uint16(0x200 + equipped.Id))
			continue
		}
		bodyIdx, ok := slotToBodyTable[slot]
		if !ok || p.body[bodyIdx] == -1 {
			buf.P1(0)
			continue
		}
		buf.P2(uint16(0x100 + p.body[bodyIdx]))
	}

	for i := range 5 {
		buf.P1(uint8(p.colors[i]))
	}

	buf.P2(uint16(p.readyanim))
	buf.P2(uint16(p.turnanim))
	buf.P2(uint16(p.walkanim))
	buf.P2(uint16(p.walkanim_b))
	buf.P2(uint16(p.walkanim_l))
	buf.P2(uint16(p.walkanim_r))
	buf.P2(uint16(p.runanim))

	buf.P8(p.username37)
	buf.P1(uint8(p.combatLevel))
	// rev-274: skillLevel follows combatLevel as a 2-byte big-endian field
	// (TS Player.ts:1422-1423 @dee467c8: stream.p1(this.combatLevel);
	// stream.p2(this.skillLevel)). New at 274; the 254 appearance block
	// stopped after combatLevel. skillLevel is not persisted (defaults 0).
	buf.P2(uint16(p.skillLevel))

	p.appearanceBuf = append([]byte(nil), buf.Bytes()...)
	p.lastAppearance = currentTick
}
