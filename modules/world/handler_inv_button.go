package world

import (
	"github.com/zsrv/goscape/pkg/io/packet"
	"github.com/zsrv/goscape/pkg/script"
)

// handleInvButton is the shared implementation for INV_BUTTON1..INV_BUTTON5.
// op is 1..5. Wire format: obj:G2 | slot:G2 | com:G2 (6 bytes).
//
// Validation gates (mirrors TS InvButtonHandler.ts):
//  1. delayed player → drop
//  2. payload < 6 bytes → drop
//  3. comId not in invListeners → drop
//  4. listener's inventory unresolved → drop
//  5. inv.HasAt(slot, obj) false → drop (covers TS validSlot + hasAt)
//
// On pass: set p.lastItem=obj, p.lastSlot=slot, look up
// [inv_button<op>,<comId>] via GetByTrigger and run with protect=true.
//
// DEVIATION NAI-48-D1: component lookup, com.iop[op-1] null-check,
// isComponentVisible, and root.overlay protect computation skipped —
// no component registry. protect=true always. Same cluster as NAI-45-D1/D2.
// Closure: component-registry sub-spec.
func (s *Server) handleInvButton(p *Player, payload []byte, op int) error {
	if p.delayed && s.currentTick < p.delayedUntil {
		return nil
	}
	if len(payload) < 6 {
		return nil
	}
	r := packet.NewPacket(payload)
	obj := int(r.G2())
	slot := int(r.G2())
	comId := int(r.G2())

	listener, ok := p.invListeners[comId]
	if !ok {
		return nil
	}
	inv := resolveListenerInv(s, listener)
	if inv == nil {
		return nil
	}
	if !inv.HasAt(slot, obj) {
		return nil
	}

	p.lastItem = obj
	p.lastSlot = slot

	trigger := script.TriggerInvButton1 + script.ServerTriggerType(op-1)
	sf := s.scriptProvider.GetByTrigger(trigger, comId, -1)
	s.runScript(sf, p, nil, true, nil, nil)
	return nil
}

// handleInvButtonD is the handler for INV_BUTTOND (opcode 159, 6-byte payload).
// Inventory drag-and-drop: player drags an item from slot to targetSlot within
// the same UI component. Wire format: com:G2 | slot:G2 | targetSlot:G2.
//
// Validation gates (mirrors TS InvButtonDHandler.ts — NOTE: delayed check
// is intentionally AFTER slot/item validation so the client visual can be
// reverted):
//  1. comId not in invListeners → drop
//  2. listener's inventory unresolved → drop
//  3. slot or targetSlot out of inv.Capacity bounds → drop
//  4. source slot empty (inv.Get(slot)==nil) → drop
//  5. player delayed → sendUpdateInvPartial to revert drag visual, then drop
//
// On pass: set p.lastSlot=slot, p.lastTargetSlot=targetSlot, look up
// [inv_buttond,<comId>] via GetByTrigger and run with protect=true.
//
// DEVIATION NAI-48-D1: component lookup, com.draggable, and
// isComponentVisible skipped — no component registry. protect=true always.
// Closure: component-registry sub-spec.
func (s *Server) handleInvButtonD(p *Player, payload []byte) error {
	if len(payload) < 6 {
		return nil
	}
	r := packet.NewPacket(payload)
	comId := int(r.G2())
	slot := int(r.G2())
	targetSlot := int(r.G2())

	listener, ok := p.invListeners[comId]
	if !ok {
		return nil
	}
	inv := resolveListenerInv(s, listener)
	if inv == nil {
		return nil
	}
	if slot < 0 || slot >= inv.Capacity || targetSlot < 0 || targetSlot >= inv.Capacity {
		return nil
	}
	if inv.Get(slot) == nil {
		return nil
	}

	if p.delayed && s.currentTick < p.delayedUntil {
		sendUpdateInvPartial(p, comId, inv, slot, targetSlot)
		return nil
	}

	p.lastSlot = slot
	p.lastTargetSlot = targetSlot

	sf := s.scriptProvider.GetByTrigger(script.TriggerInvButtonD, comId, -1)
	s.runScript(sf, p, nil, true, nil, nil)
	return nil
}
