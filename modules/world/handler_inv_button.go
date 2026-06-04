package world

import (
	"github.com/zsrv/goscape/pkg/io/packet"
	"github.com/zsrv/goscape/pkg/script"
)

// handleInvButton is the shared implementation for INV_BUTTON1..INV_BUTTON5.
// op is 1..5. Wire format: obj:G2 | slot:G2 | component:G2 (6 bytes).
//
// Gates per TS InvButtonHandler.ts (244) — note delayed is checked AFTER all
// validation guards (changed from 225 where it was first):
//  1. payload < 6 bytes → drop
//  2. nil component, or !InventoryOptions, or !IsComponentVisible → drop
//  3. com.InventoryOptions[op-1] == "" → drop
//  4. comId not in invListeners → drop
//  5. listener's inventory unresolved, or !validSlot, or !hasAt → drop
//  6. delayed player → drop
//
// On pass: set p.lastItem=item, p.lastSlot=slot; dispatch trigger via
// explicit op→TriggerInvButtonN switch (TS if/else chain); run with
// protect = !rootLayer.Overlay (rootLayer nil → protect=true).
func (s *Server) handleInvButton(p *Player, payload []byte, op int) error {
	if len(payload) < 6 {
		return nil
	}
	r := packet.NewPacket(payload)
	item := int(r.G2()) // TS InvButtonHandler.ts (244): obj renamed to item locally
	slot := int(r.G2())
	comId := int(r.G2()) // TS InvButtonDecoder.ts (244): com renamed to component

	// TS InvButtonHandler.ts (244):
	// if (typeof com === 'undefined' || !com.inventoryOptions ||
	//     !com.inventoryOptions.length || !player.isComponentVisible(com))
	com := s.lookupComponent(comId)
	if com == nil || com.InventoryOptions == nil || len(com.InventoryOptions) == 0 {
		return nil
	}
	if !p.IsComponentVisible(com) {
		return nil
	}

	// TS InvButtonHandler.ts (244): if (!com.inventoryOptions[op - 1])
	if op-1 < 0 || op-1 >= len(com.InventoryOptions) || com.InventoryOptions[op-1] == "" {
		return nil
	}

	// TS InvButtonHandler.ts (244): listener check before inv resolution
	listener, ok := p.invListeners[comId]
	if !ok {
		return nil
	}

	// TS InvButtonHandler.ts (244): !inv || !inv.validSlot(slot) || !inv.hasAt(slot, item)
	inv := resolveListenerInv(s, listener)
	if inv == nil || !inv.HasAt(slot, item) {
		return nil
	}

	// TS InvButtonHandler.ts (244): delayed check is AFTER all validation gates
	if p.delayed && s.currentTick < p.delayedUntil {
		return nil
	}

	p.lastItem = item
	p.lastSlot = slot

	// TS InvButtonHandler.ts (244): explicit if/else chain (not arithmetic add)
	var trigger script.ServerTriggerType
	if op == 1 {
		trigger = script.TriggerInvButton1
	} else if op == 2 {
		trigger = script.TriggerInvButton2
	} else if op == 3 {
		trigger = script.TriggerInvButton3
	} else if op == 4 {
		trigger = script.TriggerInvButton4
	} else {
		trigger = script.TriggerInvButton5
	}

	sf := s.scriptProvider.GetByTrigger(trigger, comId, -1)
	root := s.lookupComponent(com.RootLayer)
	protect := root == nil || !root.Overlay
	s.runScript(sf, p, nil, trigger, protect, nil, nil)
	return nil
}

// handleInvButtonD is the handler for INV_BUTTOND (opcode 81, 7-byte payload).
// Inventory drag-and-drop. Wire format: component:G2 | slot:G2 | targetSlot:G2 | mode:G1.
// TS InvButtonDDecoder.ts (244): mode g1 added as 7th byte (was 6 bytes in 225).
// The mode value is decoded but not yet forwarded to the script
// (TS InvButtonDHandler.ts (244): "// todo: pass message.mode to script").
//
// Gates per TS InvButtonDHandler.ts (244):
//  1. payload < 7 bytes → drop
//  2. nil component, or !IsComponentVisible, or !Draggable → drop
//  3. comId not in invListeners → drop
//  4. inv unresolved, or !validSlot(slot), or !validSlot(targetSlot), or src empty → drop
//  5. player delayed → sendUpdateInvPartial to revert visual, then drop
//
// On pass: set p.lastSlot, p.lastTargetSlot, look up [inv_buttond,<comId>]
// and run with protect = !rootLayer.Overlay.
func (s *Server) handleInvButtonD(p *Player, payload []byte) error {
	if len(payload) < 7 {
		return nil
	}
	r := packet.NewPacket(payload)
	comId := int(r.G2())
	slot := int(r.G2())
	targetSlot := int(r.G2())
	_ = int(r.G1()) // mode — TS InvButtonDDecoder.ts (244); todo: pass to script

	// TS InvButtonDHandler.ts (244): consolidated into single guard
	// if (typeof com === 'undefined' || !player.isComponentVisible(com) || !com.draggable)
	com := s.lookupComponent(comId)
	if com == nil || !p.IsComponentVisible(com) || !com.Draggable {
		return nil
	}

	// TS InvButtonDHandler.ts (244): listener check before inv resolution
	listener, ok := p.invListeners[comId]
	if !ok {
		return nil
	}

	// TS InvButtonDHandler.ts (244): consolidated inv+slot+source checks
	// if (!inv || !inv.validSlot(slot) || !inv.validSlot(targetSlot) || !inv.get(slot))
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
		// do nothing; revert the client visual — TS InvButtonDHandler.ts (244)
		sendUpdateInvPartial(p, comId, inv, slot, targetSlot)
		return nil
	}

	p.lastSlot = slot
	p.lastTargetSlot = targetSlot

	// TS InvButtonDHandler.ts (244): renamed script→dragTrigger locally
	dragTrigger := s.scriptProvider.GetByTrigger(script.TriggerInvButtonD, comId, -1)
	root := s.lookupComponent(com.RootLayer)
	protect := root == nil || !root.Overlay
	s.runScript(dragTrigger, p, nil, script.TriggerInvButtonD, protect, nil, nil)
	return nil
}
