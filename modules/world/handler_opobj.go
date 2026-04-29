package world

import (
	"github.com/zsrv/goscape/pkg/io/packet"
)

// handleOpObj is the shared implementation for OPOBJ1..OPOBJ5.
// op is 1..5. Payload = 6 bytes: (x: G2, z: G2, objId: G2).
//
// Validation gates (mirrors TS OpObjHandler.ts:14-42):
//  1. nil client/server guard
//  2. p.delayed → UnsetMapFlag
//  3. payload < 6 bytes → UnsetMapFlag
//  4. viewport gate: |x-originX| > 52 || |z-originZ| > 52 → UnsetMapFlag
//  5. Server.GetObj returns nil → UnsetMapFlag
//  6. ObjType not registered → UnsetMapFlag
//  7. per-op gate: Op[op-1] == "" → UnsetMapFlag
//
// On success: ClearPendingAction → opcalled=true →
// SetInteraction(Engine, obj, op, -1) → targetSubject snapshot.
func handleOpObj(p *Player, payload []byte, op int) error {
	if p.client == nil || p.client.server == nil {
		return nil
	}
	s := p.client.server

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

	obj := s.GetObj(p.level, x, z, objId, p.slot)
	if obj == nil {
		sendUnsetMapFlag(p)
		return nil
	}

	if s.objTypes == nil || objId < 0 || objId >= len(s.objTypes.Configs) {
		sendUnsetMapFlag(p)
		return nil
	}
	objType := s.objTypes.Configs[objId]
	if objType == nil {
		sendUnsetMapFlag(p)
		return nil
	}
	if len(objType.Op) < op || objType.Op[op-1] == "" {
		sendUnsetMapFlag(p)
		return nil
	}

	p.ClearPendingAction()
	p.opcalled = true
	p.SetInteraction(InteractionEngine, obj, op, -1)
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
// Payload = (x:G2, z:G2, objId:G2, spellCom:G2).
//
// DEVIATION NAI-50-D1: TS OpObjTHandler.ts:20-29 validates spellCom
// references a component with ComActionTarget.OBJ AND that the component
// is visible. Skipped — no component registry. Same cluster as S6m-D1,
// NAI-45-D1, NAI-48-D1. Closure: component-registry sub-spec.
func handleOpObjT(p *Player, payload []byte) error {
	if p.client == nil || p.client.server == nil {
		return nil
	}
	s := p.client.server

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

	obj := s.GetObj(p.level, x, z, objId, p.slot)
	if obj == nil {
		sendUnsetMapFlag(p)
		return nil
	}

	if s.objTypes == nil || objId < 0 || objId >= len(s.objTypes.Configs) || s.objTypes.Configs[objId] == nil {
		sendUnsetMapFlag(p)
		return nil
	}

	p.ClearPendingAction()
	p.opcalled = true
	p.SetInteraction(InteractionEngine, obj, targetOpObjT, spellCom)
	p.targetSubject.typ = obj.Type
	p.targetSubject.x = obj.X
	p.targetSubject.z = obj.Z
	p.targetSubject.level = obj.Level
	return nil
}

// handleOpObjU is the handler for OPOBJU (opcode 239, 12-byte payload).
// Item-on-obj: player drags an inventory item onto a ground item.
// Payload = (x:G2, z:G2, objId:G2, useObj:G2, useSlot:G2, useCom:G2).
//
// DEVIATION NAI-50-D2: TS OpObjUHandler.ts:39-48 validates useCom
// references a usable, visible component. Skipped — no component registry.
// Same cluster as S6m-D2, NAI-45-D2, NAI-48-D1. Closure: component-registry sub-spec.
func handleOpObjU(p *Player, payload []byte) error {
	if p.client == nil || p.client.server == nil {
		return nil
	}
	s := p.client.server

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

	obj := s.GetObj(p.level, x, z, objId, p.slot)
	if obj == nil {
		sendUnsetMapFlag(p)
		return nil
	}

	if s.objTypes == nil || objId < 0 || objId >= len(s.objTypes.Configs) || s.objTypes.Configs[objId] == nil {
		sendUnsetMapFlag(p)
		return nil
	}

	listener, ok := p.invListeners[useCom]
	if !ok {
		sendUnsetMapFlag(p)
		return nil
	}
	inv := resolveListenerInv(s, listener)
	if inv == nil {
		sendUnsetMapFlag(p)
		return nil
	}
	if !inv.HasAt(useSlot, useObj) {
		sendUnsetMapFlag(p)
		return nil
	}

	if s.objTypes != nil && useObj >= 0 && useObj < len(s.objTypes.Configs) {
		if useObjType := s.objTypes.Configs[useObj]; useObjType != nil && useObjType.Members && !s.cfg.NodeMembers {
			p.MessageGame("To use this item please login to a members' server.")
			sendUnsetMapFlag(p)
			return nil
		}
	}

	p.lastUseItem = useObj
	p.lastUseSlot = useSlot

	p.ClearPendingAction()
	p.opcalled = true
	p.SetInteraction(InteractionEngine, obj, targetOpObjU, -1)
	p.targetSubject.typ = obj.Type
	p.targetSubject.x = obj.X
	p.targetSubject.z = obj.Z
	p.targetSubject.level = obj.Level
	return nil
}
