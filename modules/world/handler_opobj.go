package world

import (
	"github.com/zsrv/goscape/pkg/io/packet"
	"github.com/zsrv/goscape/pkg/objtype"
)

// handleOpObj is the shared implementation for OPOBJ1..OPOBJ5.
// op is 1..5. Payload = 6 bytes: (x: G2, z: G2, objId: G2).
//
// Gate order per TS OpObjHandler.ts @2e3bcf43 (the 254 pin removes
// clearPendingAction from EVERY rejection branch — it now runs only on
// the success path — gates ALL ops via the type.op array, and the
// obj-missing branch writes UnsetMapFlag like the rest of the family):
//  1. player.delayed → UnsetMapFlag. TS:14-18.
//  2. payload < 6 bytes → UnsetMapFlag (goscape defensive).
//  3. viewport: outside ±52 of originX/Z → UnsetMapFlag. TS:20-28.
//  4. GetObj returns nil → UnsetMapFlag. TS:30-35 (the 244-era
//     moveClickRequest=false/no-UnsetMapFlag branch is GONE).
//  5. ObjType not registered → UnsetMapFlag (goscape defensive; TS
//     ObjType.get always returns a config).
//  6. type.op[op-1] === null || === 'hidden' → UnsetMapFlag. TS:37-41
//     (ALL five ops gated — the 244 op1/op4-only partial gate is gone;
//     "" is the Go encoding of TS null; note the Take default at index
//     2 means OPOBJ3 passes for objs without explicit ops).
//
// On success: clearPendingAction → SetInteraction(Engine, obj, op, -1)
// → opcalled=true → targetSubject snapshot. TS:43-47.
func handleOpObj(p *Player, payload []byte, op int) error {
	if p.client == nil || p.client.server == nil {
		return nil
	}
	s := p.client.server

	// Gate 1: delayed player. TS OpObjHandler.ts:14-18 @2e3bcf43.
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

	// Gate 3: viewport. TS OpObjHandler.ts:20-28 @2e3bcf43.
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
		return nil
	}

	// Gate 4: obj missing → UnsetMapFlag. TS OpObjHandler.ts:30-35
	// @2e3bcf43 (244's moveClickRequest=false branch is gone).
	obj := s.GetObj(p.level, x, z, objId, p.uid)
	if obj == nil {
		sendUnsetMapFlag(p)
		return nil
	}

	// Gate 5: goscape-only ObjType registration check (TS skips this).
	if s.objTypes == nil || objId < 0 || objId >= len(s.objTypes.Configs) {
		sendUnsetMapFlag(p)
		return nil
	}
	objType := s.objTypes.Configs[objId]
	if objType == nil {
		sendUnsetMapFlag(p)
		return nil
	}

	// Gate 6: full op validation. TS OpObjHandler.ts:37-41 @2e3bcf43:
	// `type.op[message.op - 1] === null || type.op[message.op - 1] === 'hidden'`
	// — every op is gated (244 only gated op1/op4) and 'hidden' rejects.
	// type.op is always non-null at the pin (Take/Drop class defaults).
	if len(objType.Op) < op || objType.Op[op-1] == "" || objType.Op[op-1] == "hidden" {
		sendUnsetMapFlag(p)
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

// handleOpObjT is the handler for OPOBJT (8-byte payload).
// Spell-on-obj: player casts a spell onto a ground item.
// Payload = (x:G2, z:G2, objId:G2, spellComponent:G2).
//
// Gate order per TS OpObjTHandler.ts @2e3bcf43 (clearPendingAction only
// on the success path at the 254 pin):
//  1. player.delayed → UnsetMapFlag. TS:14-18.
//  2. payload too short → UnsetMapFlag (goscape defensive).
//  3. com undefined || (actionTarget&OBJ)==0, then !isVisible →
//     UnsetMapFlag. TS:20-29 (two branches; Go combines — same accept set).
//  4. viewport: outside ±52 of originX/Z → UnsetMapFlag. TS:31-39.
//  5. GetObj returns nil → UnsetMapFlag. TS:41-46.
//  6. ObjType not registered → UnsetMapFlag (goscape defensive; TS
//     skips this check).
//
// On success: clearPendingAction → SetInteraction(Engine, obj,
// targetOpObjT, spellCom) → opcalled=true → targetSubject snapshot.
// TS:48-51.
func handleOpObjT(p *Player, payload []byte) error {
	if p.client == nil || p.client.server == nil {
		return nil
	}
	s := p.client.server

	// Gate 1: delayed player. TS OpObjTHandler.ts:14-18 @2e3bcf43.
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

	// Gate 3: component check. TS OpObjTHandler.ts:20-29 @2e3bcf43.
	com := s.lookupComponent(spellCom)
	if com == nil || !p.IsComponentVisible(com) || (com.ActionTarget&objtype.ComActionTargetObj) == 0 {
		sendUnsetMapFlag(p)
		return nil
	}

	// Gate 4: viewport. TS OpObjTHandler.ts:31-39 @2e3bcf43.
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
		return nil
	}

	// Gate 5: obj missing. TS OpObjTHandler.ts:41-46 @2e3bcf43.
	obj := s.GetObj(p.level, x, z, objId, p.uid)
	if obj == nil {
		sendUnsetMapFlag(p)
		return nil
	}

	// Gate 6: goscape-only ObjType registration check (TS skips this).
	if s.objTypes == nil || objId < 0 || objId >= len(s.objTypes.Configs) || s.objTypes.Configs[objId] == nil {
		sendUnsetMapFlag(p)
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

// handleOpObjU is the handler for OPOBJU (12-byte payload).
// Item-on-obj: player drags an inventory item onto a ground item.
// Payload = (x:G2, z:G2, objId:G2, useObj:G2, useSlot:G2, useComponent:G2).
//
// Gate order per TS OpObjUHandler.ts @2e3bcf43 (clearPendingAction only
// on the success path; the 254 pin REORDERS the gates — viewport and
// obj lookup now precede the component/inv checks — and the component
// gate reverts to com.usable):
//  1. player.delayed → UnsetMapFlag. TS:17-21.
//  2. payload too short → UnsetMapFlag (goscape defensive).
//  3. viewport: outside ±52 of originX/Z → UnsetMapFlag. TS:23-31.
//  4. GetObj returns nil → UnsetMapFlag. TS:33-38.
//  5. useCom: nil || !usable, then !isVisible → UnsetMapFlag. TS:40-49.
//  6. listener/inv unresolved → UnsetMapFlag. TS:51-57.
//  7. !validSlot || !hasAt → UnsetMapFlag. TS:59-67.
//  8. members-only item on free world → MessageGame + UnsetMapFlag
//     (after clearPendingAction). TS:69-75.
//
// On success: clearPendingAction (before gate 8) → (gate 8 members
// check) → lastUseItem/lastUseSlot → SetInteraction(Engine, obj,
// targetOpObjU, -1) → opcalled=true → targetSubject snapshot. TS:69-82.
func handleOpObjU(p *Player, payload []byte) error {
	if p.client == nil || p.client.server == nil {
		return nil
	}
	s := p.client.server

	// Gate 1: delayed player. TS OpObjUHandler.ts:17-21 @2e3bcf43.
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

	// Gate 3: viewport — moved BEFORE the component check at the pin.
	// TS OpObjUHandler.ts:23-31 @2e3bcf43.
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
		return nil
	}

	// Gate 4: obj missing — moved before the component check at the pin.
	// TS OpObjUHandler.ts:33-38 @2e3bcf43.
	obj := s.GetObj(p.level, x, z, objId, p.uid)
	if obj == nil {
		sendUnsetMapFlag(p)
		return nil
	}

	// Gate 5: component check — 254 reverts to com.usable (TS
	// OpObjUHandler.ts:40-49 @2e3bcf43 `!useCom.usable`; 244 had
	// interactable here).
	com := s.lookupComponent(useCom)
	if com == nil || !p.IsComponentVisible(com) || !com.Usable {
		sendUnsetMapFlag(p)
		return nil
	}

	// Gates 6+7: listener → inv → slot → item. TS OpObjUHandler.ts:51-67
	// @2e3bcf43. HasAt covers both validSlot (OOB slot → false) and item
	// identity.
	listener, ok := p.invListeners[useCom]
	if !ok {
		sendUnsetMapFlag(p)
		return nil
	}
	inv := resolveListenerInv(s, listener)
	if inv == nil || !inv.HasAt(useSlot, useObj) {
		sendUnsetMapFlag(p)
		return nil
	}

	p.ClearPendingAction()

	// Gate 8: members-only item on free world. TS OpObjUHandler.ts:71-75 @2e3bcf43.
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
