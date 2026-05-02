package world

import (
	"github.com/zsrv/goscape/pkg/io/packet"
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/script"
)

// handleOpHeld is the shared implementation for OPHELD1..OPHELD5.
// op is 1..5. Wire format: obj:G2 | slot:G2 | com:G2 (6 bytes).
//
// Gates per TS OpHeldHandler.ts:
//  1. p.delayed → drop
//  2. payload < 6 → drop
//  3. nil component or !Operable → drop
//  4. !IsComponentVisible → drop
//  5. comId not in invListeners → drop
//  6. listener's inventory unresolved → drop
//  7. inv.HasAt(slot, obj) false → drop
//  8. ObjType not registered (goscape defensive; TS throws here) → drop
//  9. objType.IOp[op-1] == "" → drop
//
// On pass: p.lastItem/lastSlot snapshot → ClearPendingAction iff
// com.RootLayer != p.modalMain → moveClickRequest=false →
// faceEntity=-1 + emit entitymask (unconditional, matches TS) →
// fire [opheld<op>,<objId>] via GetByTrigger keyed on
// (objType.id, objType.Category) and runScript with protect=true.
//
// DEVIATION NAI-71-D-OPHELD-NO-SESSION-LOG: TS OpHeldHandler.ts:62-65
// calls addSessionLog(MODERATOR, ...) for op != 5. Skipped — no
// session-log subsystem in goscape. Closure path: future moderator-
// logging sub-spec ports LoggerEventType + session-log buffer.
func handleOpHeld(p *Player, payload []byte, op int) error {
	if p.client == nil || p.client.server == nil {
		return nil
	}
	s := p.client.server

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
	if com == nil || !com.Operable {
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
	if !inv.HasAt(slot, obj) {
		return nil
	}

	if s.objTypes == nil || obj < 0 || obj >= len(s.objTypes.Configs) {
		return nil
	}
	objType := s.objTypes.Configs[obj]
	if objType == nil { // goscape defensive; TS throws here
		return nil
	}
	if len(objType.IOp) < op || objType.IOp[op-1] == "" {
		return nil
	}

	p.lastItem = obj
	p.lastSlot = slot

	if com.RootLayer != p.modalMain {
		p.ClearPendingAction()
	}

	p.moveClickRequest = false
	if p.faceEntity != -1 {
		p.faceEntity = -1
	}
	p.masks |= p.entitymask

	trigger := script.TriggerOpHeld1 + script.ServerTriggerType(op-1)
	sf := s.scriptProvider.GetByTrigger(trigger, obj, objType.Category)
	s.runScript(sf, p, nil, true, nil, nil)
	return nil
}

func handleOpHeld1(p *Player, payload []byte) error { return handleOpHeld(p, payload, 1) }
func handleOpHeld2(p *Player, payload []byte) error { return handleOpHeld(p, payload, 2) }
func handleOpHeld3(p *Player, payload []byte) error { return handleOpHeld(p, payload, 3) }
func handleOpHeld4(p *Player, payload []byte) error { return handleOpHeld(p, payload, 4) }
func handleOpHeld5(p *Player, payload []byte) error { return handleOpHeld(p, payload, 5) }

// handleOpHeldT is the handler for OPHELDT (opcode 48, 8-byte payload).
// Spell-on-held-item: player drags a spell from the magic-book interface
// onto an inventory item.
// Wire format: obj:G2 | slot:G2 | com:G2 | spellCom:G2.
//
// Gates per TS OpHeldTHandler.ts:
//  1. p.delayed → drop
//  2. payload < 8 → drop
//  3. spellCom: nil or (ActionTarget & HELD) == 0 → drop
//  4. spellCom: !IsComponentVisible → drop
//  5. com: nil or !Usable → drop
//  6. com: !IsComponentVisible → drop
//  7. comId not in invListeners → drop
//  8. listener's inventory unresolved → drop
//  9. inv.HasAt(slot, obj) false → drop
//
// On pass: lastItem/lastSlot snapshot → ClearPendingAction
// (unconditional, contrast OPHELD1-5 conditional) → faceEntity=-1 +
// emit entitymask → fire [opheldt,<spellComId>] via
// GetByTrigger(typeID=spellComId, cat=-1). On no-script: emit
// "Nothing interesting happens.".
//
// DEVIATION NAI-71-D-OPHELD-NO-SESSION-LOG: TS OpHeldTHandler.ts:61
// addSessionLog skipped — no session-log subsystem in goscape. Closure
// path: future moderator-logging sub-spec ports LoggerEventType +
// session-log buffer.
func handleOpHeldT(p *Player, payload []byte) error {
	if p.client == nil || p.client.server == nil {
		return nil
	}
	s := p.client.server

	if p.delayed && s.currentTick < p.delayedUntil {
		return nil
	}
	if len(payload) < 8 {
		return nil
	}

	r := packet.NewPacket(payload)
	obj := int(r.G2())
	slot := int(r.G2())
	comId := int(r.G2())
	spellComId := int(r.G2())

	spellCom := s.lookupComponent(spellComId)
	if spellCom == nil || (spellCom.ActionTarget&objtype.ComActionTargetHeld) == 0 {
		return nil
	}
	if !p.IsComponentVisible(spellCom) {
		return nil
	}

	com := s.lookupComponent(comId)
	if com == nil || !com.Usable {
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
	if !inv.HasAt(slot, obj) {
		return nil
	}

	p.lastItem = obj
	p.lastSlot = slot

	p.ClearPendingAction()
	if p.faceEntity != -1 {
		p.faceEntity = -1
	}
	p.masks |= p.entitymask

	sf := s.scriptProvider.GetByTrigger(script.TriggerOpHeldT, spellComId, -1)
	if sf == nil {
		p.MessageGame("Nothing interesting happens.")
		return nil
	}
	s.runScript(sf, p, nil, true, nil, nil)
	return nil
}
