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

	var worn *inventory.Inventory
	if p.invs != nil {
		worn = p.invs[invs.Worn]
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

	for slot := 0; slot < 12; slot++ {
		if skipped[slot] {
			buf.P1(0)
			continue
		}
		var equipped *inventory.Item
		if worn != nil && slot < len(worn.Items) {
			equipped = worn.Items[slot]
		}
		if equipped != nil {
			buf.P2(uint16(0x200 | (equipped.Id & 0x1FF)))
			continue
		}
		bodyIdx, ok := slotToBodyTable[slot]
		if !ok || p.body[bodyIdx] == -1 {
			buf.P1(0)
			continue
		}
		buf.P2(uint16(0x100 | (p.body[bodyIdx] & 0xFF)))
	}

	for i := 0; i < 5; i++ {
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

	p.appearanceBuf = append([]byte(nil), buf.Bytes()...)
	p.lastAppearance = currentTick
}
