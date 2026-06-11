package world

import (
	"github.com/zsrv/goscape/pkg/io/packet"
	"github.com/zsrv/goscape/pkg/script"
)

// handleInvButton is the shared implementation for INV_BUTTON1..INV_BUTTON5.
// op is 1..5. Wire format: obj:G2 | slot:G2 | component:G2 (6 bytes).
//
// Gates per TS InvButtonHandler.ts @2e3bcf43 (the 254 pin moves the
// delayed check FIRST — 244 had it last — and reorders the iop gate to
// sit between the visibility and listener checks):
//  1. delayed player → drop. TS:15-18.
//  2. payload < 6 bytes → drop (goscape defensive).
//  3. com undefined, then !isComponentVisible → drop. TS:20-27.
//  4. !com.iop || com.iop[op-1] === null → drop ("" encodes TS null;
//     goscape's InventoryOptions field is the TS iop). TS:29-32.
//  5. listener/inv unresolved → drop. TS:34-39.
//  6. !validSlot(slot) || !hasAt(slot, obj) → drop. TS:41-47.
//
// On pass: set p.lastItem=obj, p.lastSlot=slot; dispatch trigger via
// INV_BUTTON1 + (op-1); run with protect = !rootLayer.Overlay
// (rootLayer nil → protect=true). TS:49-60.
func (s *Server) handleInvButton(p *Player, payload []byte, op int) error {
	// Gate 1: delayed — FIRST at the 254 pin. TS InvButtonHandler.ts:15-18.
	if p.delayed && s.currentTick < p.delayedUntil {
		return nil
	}

	if len(payload) < 6 {
		return nil
	}
	r := packet.NewPacket(payload)
	item := int(r.G2())
	slot := int(r.G2())
	comId := int(r.G2())

	// Gate 3: component + visibility. TS InvButtonHandler.ts:20-27 @2e3bcf43.
	com := s.lookupComponent(comId)
	if com == nil || !p.IsComponentVisible(com) {
		return nil
	}

	// Gate 4: iop slot. TS InvButtonHandler.ts:29-32 @2e3bcf43:
	// `!com.iop || com.iop[message.op - 1] === null`.
	if com.InventoryOptions == nil || op-1 < 0 || op-1 >= len(com.InventoryOptions) || com.InventoryOptions[op-1] == "" {
		return nil
	}

	// Gates 5+6: listener → inv → slot → item. TS InvButtonHandler.ts:34-47.
	listener, ok := p.invListeners[comId]
	if !ok {
		return nil
	}
	inv := resolveListenerInv(s, listener)
	if inv == nil || !inv.HasAt(slot, item) {
		return nil
	}

	p.lastItem = item
	p.lastSlot = slot

	// TS InvButtonHandler.ts:52 @2e3bcf43: INV_BUTTON1 + (message.op - 1).
	var trigger script.ServerTriggerType
	switch op {
	case 1:
		trigger = script.TriggerInvButton1
	case 2:
		trigger = script.TriggerInvButton2
	case 3:
		trigger = script.TriggerInvButton3
	case 4:
		trigger = script.TriggerInvButton4
	default:
		trigger = script.TriggerInvButton5
	}

	sf := s.scriptProvider.GetByTrigger(trigger, comId, -1)
	root := s.lookupComponent(com.RootLayer)
	protect := root == nil || !root.Overlay
	s.runScript(sf, p, nil, trigger, protect, nil, nil)
	return nil
}

// handleInvButtonD is the handler for INV_BUTTOND (7-byte payload).
// Inventory drag-and-drop. Wire format: component:G2 | slot:G2 | targetSlot:G2 | mode:G1.
// The mode value is decoded but not yet forwarded to the script
// (TS InvButtonDHandler.ts @2e3bcf43: "// todo: is it necessary to pass
// message.mode to script? is it just verification?").
//
// Gates per TS InvButtonDHandler.ts @2e3bcf43 (the component gate
// becomes `!draggable && !swappable` — 244 required draggable alone):
//  1. payload < 7 bytes → drop (goscape defensive).
//  2. com undefined || (!draggable && !swappable), then !isVisible →
//     drop. TS:16-23.
//  3. listener/inv unresolved → drop. TS:25-30.
//  4. !validSlot(slot) || !validSlot(targetSlot) || src empty → drop. TS:32-38.
//  5. player delayed → sendUpdateInvPartial to revert visual, then drop. TS:40-44.
//
// On pass: set p.lastSlot, p.lastTargetSlot, look up [inv_buttond,<comId>]
// and run with protect = !rootLayer.Overlay. TS:46-56.
func (s *Server) handleInvButtonD(p *Player, payload []byte) error {
	if len(payload) < 7 {
		return nil
	}
	r := packet.NewPacket(payload)
	comId := int(r.G2())
	slot := int(r.G2())
	targetSlot := int(r.G2())
	_ = int(r.G1()) // mode — decoded but not forwarded (TS todo)

	// Gate 2: TS InvButtonDHandler.ts:16-23 @2e3bcf43:
	// `typeof com === 'undefined' || (!com.draggable && !com.swappable)`
	// — swappable components qualify at the 254 pin (244 required
	// draggable alone).
	com := s.lookupComponent(comId)
	if com == nil || (!com.Draggable && !com.Swappable) || !p.IsComponentVisible(com) {
		return nil
	}

	// Gates 3+4: listener → inv → slots → source. TS InvButtonDHandler.ts:25-38.
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

	// Gate 5: delayed → revert the client visual. TS InvButtonDHandler.ts:40-44.
	if p.delayed && s.currentTick < p.delayedUntil {
		sendUpdateInvPartial(p, comId, inv, slot, targetSlot)
		return nil
	}

	p.lastSlot = slot
	p.lastTargetSlot = targetSlot

	sf := s.scriptProvider.GetByTrigger(script.TriggerInvButtonD, comId, -1)
	root := s.lookupComponent(com.RootLayer)
	protect := root == nil || !root.Overlay
	s.runScript(sf, p, nil, script.TriggerInvButtonD, protect, nil, nil)
	return nil
}
