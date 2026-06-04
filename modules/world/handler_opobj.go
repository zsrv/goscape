package world

import (
	"github.com/zsrv/goscape/pkg/io/packet"
	"github.com/zsrv/goscape/pkg/objtype"
)

// handleOpObj is the shared implementation for OPOBJ1..OPOBJ5.
// op is 1..5. Payload = 6 bytes: (x: G2, z: G2, objId: G2).
//
// Gate order per TS OpObjHandler.ts (244):
//  1. player.delayed → UnsetMapFlag (no clearPendingAction). TS:14-17.
//  2. payload < 6 bytes → UnsetMapFlag (goscape defensive).
//  3. viewport: outside ±52 of originX/Z → UnsetMapFlag + clearPendingAction. TS:23-27.
//  4. GetObj returns nil → moveClickRequest=false + clearPendingAction (no UnsetMapFlag). TS:29-34.
//  5. ObjType not registered → UnsetMapFlag + clearPendingAction.
//     (goscape defensive; TS skips this check)
//  6. Partial op gate: op==1 rejects if op[0] is absent/empty; op==4 rejects if
//     op[3] is absent/empty; ops 2/3/5 are not gated. TS:36-42 ("todo: validate
//     all options"). Note: 'hidden' check removed at 244 — "hidden" is a non-empty
//     string and is now truthy, passing the gate (matches OpNpc handler precedent).
//
// On success: clearPendingAction → SetInteraction(Engine, obj, op, -1) → opcalled=true
// → targetSubject snapshot. TS:57-60.
func handleOpObj(p *Player, payload []byte, op int) error {
	if p.client == nil || p.client.server == nil {
		return nil
	}
	s := p.client.server

	// Gate 1: delayed player — no clearPendingAction. TS OpObjHandler.ts:14-17 (244).
	if p.delayed && s.currentTick < p.delayedUntil {
		sendUnsetMapFlag(p)
		return nil
	}

	if len(payload) < 6 {
		sendUnsetMapFlag(p)
		return nil
	}

	r := packet.NewPacket(payload)
	x := int(r.G2())
	z := int(r.G2())
	objId := int(r.G2())

	// Gate 3: viewport — clearPendingAction. TS OpObjHandler.ts:19-27 (244).
	dx := x - p.originX
	if dx < 0 {
		dx = -dx
	}
	dz := z - p.originZ
	if dz < 0 {
		dz = -dz
	}
	if dx > 52 || dz > 52 {
		sendUnsetMapFlag(p)
		p.ClearPendingAction()
		return nil
	}

	// Gate 4: obj missing — no UnsetMapFlag, just moveClickRequest + clearPendingAction.
	// TS OpObjHandler.ts:29-34 (244).
	obj := s.GetObj(p.level, x, z, objId, p.uid)
	if obj == nil {
		p.moveClickRequest = false
		p.ClearPendingAction()
		return nil
	}

	// Gate 5: goscape-only ObjType registration check (TS skips this).
	if s.objTypes == nil || objId < 0 || objId >= len(s.objTypes.Configs) {
		sendUnsetMapFlag(p)
		p.ClearPendingAction()
		return nil
	}
	objType := s.objTypes.Configs[objId]
	if objType == nil {
		sendUnsetMapFlag(p)
		p.ClearPendingAction()
		return nil
	}

	// Gate 6: partial op validation — only op1 (index 0) and op4 (index 3) are
	// checked at 244; ops 2/3/5 pass unconditionally. TS OpObjHandler.ts:36-42 (244):
	// "todo: validate all options". The 'hidden' check is absent at 244: "hidden" is
	// a non-empty string (truthy) and passes.
	if op == 1 && (len(objType.Op) == 0 || objType.Op[0] == "") {
		sendUnsetMapFlag(p)
		p.ClearPendingAction()
		return nil
	}
	if op == 4 && (len(objType.Op) < 4 || objType.Op[3] == "") {
		sendUnsetMapFlag(p)
		p.ClearPendingAction()
		return nil
	}

	p.ClearPendingAction()
	p.SetInteraction(InteractionEngine, obj, op, -1)
	p.opcalled = true
	p.targetSubject.typ = obj.Type
	p.targetSubject.x = obj.X
	p.targetSubject.z = obj.Z
	p.targetSubject.level = obj.Level
	return nil
}

func handleOpObj1(p *Player, payload []byte) error { return handleOpObj(p, payload, 1) }
func handleOpObj2(p *Player, payload []byte) error { return handleOpObj(p, payload, 2) }
func handleOpObj3(p *Player, payload []byte) error { return handleOpObj(p, payload, 3) }
func handleOpObj4(p *Player, payload []byte) error { return handleOpObj(p, payload, 4) }
func handleOpObj5(p *Player, payload []byte) error { return handleOpObj(p, payload, 5) }

// handleOpObjT is the handler for OPOBJT (opcode 138, 8-byte payload).
// Spell-on-obj: player casts a spell onto a ground item.
// Payload = (x:G2, z:G2, objId:G2, spellComponent:G2).
//
// Gate order per TS OpObjTHandler.ts (244):
//  1. player.delayed → UnsetMapFlag (no clearPendingAction). TS:14-17.
//  2. payload too short → UnsetMapFlag (goscape defensive).
//  3. com undefined || !isVisible || (actionTarget&OBJ)==0 → UnsetMapFlag +
//     clearPendingAction (combined check at 244). TS:19-24.
//  4. viewport: outside ±52 of originX/Z → UnsetMapFlag + clearPendingAction. TS:29-34.
//  5. GetObj returns nil → UnsetMapFlag + clearPendingAction. TS:36-41.
//  6. ObjType not registered → UnsetMapFlag + clearPendingAction.
//     (goscape defensive; TS skips this check)
//
// On success: clearPendingAction → SetInteraction(Engine, obj, targetOpObjT,
// spellCom) → opcalled=true → targetSubject snapshot. TS:43-46.
func handleOpObjT(p *Player, payload []byte) error {
	if p.client == nil || p.client.server == nil {
		return nil
	}
	s := p.client.server

	// Gate 1: delayed player — no clearPendingAction. TS OpObjTHandler.ts:14-17 (244).
	if p.delayed && s.currentTick < p.delayedUntil {
		sendUnsetMapFlag(p)
		return nil
	}

	if len(payload) < 8 {
		sendUnsetMapFlag(p)
		return nil
	}

	r := packet.NewPacket(payload)
	x := int(r.G2())
	z := int(r.G2())
	objId := int(r.G2())
	spellCom := int(r.G2())

	// Gate 3: combined component check — clearPendingAction on any failure.
	// TS OpObjTHandler.ts:19-24 (244): undefined || !isVisible || !actionTarget.
	com := s.lookupComponent(spellCom)
	if com == nil || !p.IsComponentVisible(com) || (com.ActionTarget&objtype.ComActionTargetObj) == 0 {
		sendUnsetMapFlag(p)
		p.ClearPendingAction()
		return nil
	}

	// Gate 4: viewport — clearPendingAction. TS OpObjTHandler.ts:29-34 (244).
	dx := x - p.originX
	if dx < 0 {
		dx = -dx
	}
	dz := z - p.originZ
	if dz < 0 {
		dz = -dz
	}
	if dx > 52 || dz > 52 {
		sendUnsetMapFlag(p)
		p.ClearPendingAction()
		return nil
	}

	// Gate 5: obj missing — clearPendingAction. TS OpObjTHandler.ts:36-41 (244).
	obj := s.GetObj(p.level, x, z, objId, p.uid)
	if obj == nil {
		sendUnsetMapFlag(p)
		p.ClearPendingAction()
		return nil
	}

	// Gate 6: goscape-only ObjType registration check (TS skips this).
	if s.objTypes == nil || objId < 0 || objId >= len(s.objTypes.Configs) || s.objTypes.Configs[objId] == nil {
		sendUnsetMapFlag(p)
		p.ClearPendingAction()
		return nil
	}

	p.ClearPendingAction()
	p.SetInteraction(InteractionEngine, obj, targetOpObjT, spellCom)
	p.opcalled = true
	p.targetSubject.typ = obj.Type
	p.targetSubject.x = obj.X
	p.targetSubject.z = obj.Z
	p.targetSubject.level = obj.Level
	return nil
}

// handleOpObjU is the handler for OPOBJU (opcode 239, 12-byte payload).
// Item-on-obj: player drags an inventory item onto a ground item.
// Payload = (x:G2, z:G2, objId:G2, useObj:G2, useSlot:G2, useComponent:G2).
//
// Gate order per TS OpObjUHandler.ts (244):
//  1. player.delayed → UnsetMapFlag (no clearPendingAction). TS:16-19.
//  2. payload too short → UnsetMapFlag (goscape defensive).
//  3. com undefined || !isVisible || !interactable → UnsetMapFlag + clearPendingAction.
//     (244: uses interactable, was usable at 225; combined check; fires before viewport).
//     TS:21-26.
//  4. viewport: outside ±52 of originX/Z → UnsetMapFlag + clearPendingAction. TS:31-36.
//  5. listener not found → UnsetMapFlag + clearPendingAction. TS:38-43.
//  6. inv unresolved || !hasAt(slot, item) → UnsetMapFlag + clearPendingAction. TS:45-50.
//  7. GetObj returns nil → UnsetMapFlag + clearPendingAction. TS:52-57.
//  8. members-only item on free world → MessageGame + UnsetMapFlag. TS:60-63.
//     (no clearPendingAction here — clearPendingAction already ran before this gate)
//
// On success: clearPendingAction (before gate 8) → (gate 8 members check) →
// lastUseItem/lastUseSlot → SetInteraction(Engine, obj, targetOpObjU, -1) →
// opcalled=true → targetSubject snapshot. TS:59-71.
func handleOpObjU(p *Player, payload []byte) error {
	if p.client == nil || p.client.server == nil {
		return nil
	}
	s := p.client.server

	// Gate 1: delayed player — no clearPendingAction. TS OpObjUHandler.ts:16-19 (244).
	if p.delayed && s.currentTick < p.delayedUntil {
		sendUnsetMapFlag(p)
		return nil
	}

	if len(payload) < 12 {
		sendUnsetMapFlag(p)
		return nil
	}

	r := packet.NewPacket(payload)
	x := int(r.G2())
	z := int(r.G2())
	objId := int(r.G2())
	useObj := int(r.G2())
	useSlot := int(r.G2())
	useCom := int(r.G2())

	// Gate 3: combined component check — clearPendingAction on any failure.
	// 244: checks com.Interactable (was com.Usable at 225); fires before viewport.
	// TS OpObjUHandler.ts:21-26 (244).
	com := s.lookupComponent(useCom)
	if com == nil || !p.IsComponentVisible(com) || !com.Interactable {
		sendUnsetMapFlag(p)
		p.ClearPendingAction()
		return nil
	}

	// Gate 4: viewport — clearPendingAction. TS OpObjUHandler.ts:31-36 (244).
	dx := x - p.originX
	if dx < 0 {
		dx = -dx
	}
	dz := z - p.originZ
	if dz < 0 {
		dz = -dz
	}
	if dx > 52 || dz > 52 {
		sendUnsetMapFlag(p)
		p.ClearPendingAction()
		return nil
	}

	// Gate 5: listener check — clearPendingAction. TS OpObjUHandler.ts:38-43 (244).
	listener, ok := p.invListeners[useCom]
	if !ok {
		sendUnsetMapFlag(p)
		p.ClearPendingAction()
		return nil
	}

	// Gate 6: inv + item check — clearPendingAction. TS OpObjUHandler.ts:45-50 (244).
	// HasAt covers both validSlot (slot out-of-bounds → Get returns nil) and hasAt.
	inv := resolveListenerInv(s, listener)
	if inv == nil || !inv.HasAt(useSlot, useObj) {
		sendUnsetMapFlag(p)
		p.ClearPendingAction()
		return nil
	}

	// Gate 7: obj missing — clearPendingAction. TS OpObjUHandler.ts:52-57 (244).
	obj := s.GetObj(p.level, x, z, objId, p.uid)
	if obj == nil {
		sendUnsetMapFlag(p)
		p.ClearPendingAction()
		return nil
	}

	p.ClearPendingAction()

	// Gate 8: members-only item on free world. TS OpObjUHandler.ts:60-63 (244).
	if s.objTypes != nil && useObj >= 0 && useObj < len(s.objTypes.Configs) {
		if useObjType := s.objTypes.Configs[useObj]; useObjType != nil && useObjType.Members && !s.cfg.NodeMembers {
			p.MessageGame("To use this item please login to a members' server.")
			sendUnsetMapFlag(p)
			return nil
		}
	}

	p.lastUseItem = useObj
	p.lastUseSlot = useSlot

	p.SetInteraction(InteractionEngine, obj, targetOpObjU, -1)
	p.opcalled = true
	p.targetSubject.typ = obj.Type
	p.targetSubject.x = obj.X
	p.targetSubject.z = obj.Z
	p.targetSubject.level = obj.Level
	return nil
}
