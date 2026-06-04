package world

import (
	"github.com/zsrv/goscape/pkg/io/packet"
	"github.com/zsrv/goscape/pkg/script"
)

// handleInvButton is the shared implementation for INV_BUTTON1..INV_BUTTON5.
// op is 1..5. Wire format: obj:G2 | slot:G2 | com:G2 (6 bytes).
//
// Gates per TS InvButtonHandler.ts:
//  1. delayed player → drop
//  2. payload < 6 bytes → drop
//  3. nil component or !IsComponentVisible → drop
//  4. com.InventoryOptions nil or InventoryOptions[op-1]=="" → drop
//  5. comId not in invListeners → drop
//  6. listener's inventory unresolved → drop
//  7. inv.HasAt(slot, obj) false → drop
//
// On pass: set p.lastItem=obj, p.lastSlot=slot, look up
// [inv_button<op>,<comId>] via GetByTrigger and run with
// protect = !rootLayer.Overlay (rootLayer nil → protect=true).
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

	com := s.lookupComponent(comId)
	if com == nil {
		return nil
	}
	if !p.IsComponentVisible(com) {
		return nil
	}
	if com.InventoryOptions == nil || op-1 < 0 || op-1 >= len(com.InventoryOptions) || com.InventoryOptions[op-1] == "" {
		return nil
	}

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
	root := s.lookupComponent(com.RootLayer)
	protect := root == nil || !root.Overlay
	s.runScript(sf, p, nil, trigger, protect, nil, nil)
	return nil
}

// handleInvButtonD is the handler for INV_BUTTOND (opcode 159, 6-byte payload).
// Inventory drag-and-drop. Wire format: com:G2 | slot:G2 | targetSlot:G2.
//
// Gates per TS InvButtonDHandler.ts (note: visual-revert delayed-gate is
// AFTER inv-listener gates, matching TS):
//  1. payload < 6 bytes → drop
//  2. nil component or !Draggable → drop
//  3. !IsComponentVisible → drop
//  4. comId not in invListeners → drop
//  5. listener's inventory unresolved → drop
//  6. slot or targetSlot out of inv.Capacity bounds → drop
//  7. source slot empty (inv.Get(slot)==nil) → drop
//  8. player delayed → sendUpdateInvPartial to revert visual, then drop
//
// On pass: set p.lastSlot, p.lastTargetSlot, look up [inv_buttond,<comId>]
// and run with protect = !rootLayer.Overlay.
func (s *Server) handleInvButtonD(p *Player, payload []byte) error {
	if len(payload) < 6 {
		return nil
	}
	r := packet.NewPacket(payload)
	comId := int(r.G2())
	slot := int(r.G2())
	targetSlot := int(r.G2())

	com := s.lookupComponent(comId)
	if com == nil || !com.Draggable {
		return nil
	}
	if !p.IsComponentVisible(com) {
		return nil
	}

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
	root := s.lookupComponent(com.RootLayer)
	protect := root == nil || !root.Overlay
	s.runScript(sf, p, nil, script.TriggerInvButtonD, protect, nil, nil)
	return nil
}
