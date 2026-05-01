package world

import (
	"github.com/zsrv/goscape/pkg/io/packet"
	"github.com/zsrv/goscape/pkg/objtype"
)

// handleOpLoc is the shared implementation for OPLOC1..OPLOC5.
// op is 1..5. Payload = 6 bytes: (x: G2, z: G2, locId: G2).
//
// Validation gates (mirrors TS OpLocHandler.ts:14-42):
//  1. p.delayed → UnsetMapFlag
//  2. payload too short → UnsetMapFlag
//  3. coords outside player's render viewport (52 tiles each axis
//     from p.originX/originZ) → UnsetMapFlag
//  4. Server.GetLoc returns nil → UnsetMapFlag
//  5. LocType not registered → UnsetMapFlag
//
// S6j-D1 closed in S6k: per-op validation gate (locType.Op[op-1])
// restored below, mirroring handler_opnpc.go:38-44 for consistency.
//
// On success: ClearPendingAction → SetInteraction(Engine, loc, op, -1) →
// snapshot loc identity into p.targetSubject for tryFireOpTrigger's
// lifecycle gate.
func handleOpLoc(p *Player, payload []byte, op int) error {
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
	locId := int(r.G2())

	// Viewport gate. 52 tiles is the player's render half-distance from
	// origin (TS OpLocHandler.ts:20-28). NOT to be confused with the
	// 104-tile build-area diameter — the player sits at scene center,
	// so the rendered radius is half the diameter.
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

	loc := s.GetLoc(p.level, x, z, locId)
	if loc == nil {
		sendUnsetMapFlag(p)
		return nil
	}

	if s.locTypes == nil || locId < 0 || locId >= len(s.locTypes.Configs) {
		sendUnsetMapFlag(p)
		return nil
	}
	locType := s.locTypes.Configs[locId]
	if locType == nil {
		sendUnsetMapFlag(p)
		return nil
	}
	// S6j-D1 closed in S6k: per-op validation gate. TS OpLocHandler.ts:38-42
	// rejects clicks where locType.op is nil, too short, or the slot
	// is empty. The decoder coerces "hidden" to "" at load time
	// (pkg/objtype/loctype.go cases 30-34), so the runtime check is
	// just `== ""`.
	if len(locType.Op) < op || locType.Op[op-1] == "" {
		sendUnsetMapFlag(p)
		return nil
	}

	p.ClearPendingAction()
	p.opcalled = true
	p.SetInteraction(InteractionEngine, loc, op, -1)
	p.targetSubject.typ = loc.Type()
	p.targetSubject.x = loc.X
	p.targetSubject.z = loc.Z
	p.targetSubject.level = loc.Level
	return nil
}

func handleOpLoc1(p *Player, payload []byte) error { return handleOpLoc(p, payload, 1) }
func handleOpLoc2(p *Player, payload []byte) error { return handleOpLoc(p, payload, 2) }
func handleOpLoc3(p *Player, payload []byte) error { return handleOpLoc(p, payload, 3) }
func handleOpLoc4(p *Player, payload []byte) error { return handleOpLoc(p, payload, 4) }
func handleOpLoc5(p *Player, payload []byte) error { return handleOpLoc(p, payload, 5) }

// handleOpLocT is the handler for OPLOCT (opcode 9, 8-byte payload).
// Spell-on-loc: player drags a spell icon from the magic-book interface
// onto a loc. Payload = (x:G2, z:G2, locId:G2, spellCom:G2).
//
// Gates per TS OpLocTHandler.ts:
//  1. delayed player → UnsetMapFlag
//  2. payload too short → UnsetMapFlag
//  3. spellCom: nil or ActionTarget&LOC == 0 → UnsetMapFlag
//  4. spellCom: !IsComponentVisible → UnsetMapFlag
//  5. coords outside viewport (52-tile half-extent) → UnsetMapFlag
//  6. Server.GetLoc returns nil → UnsetMapFlag
//  7. LocType not registered → UnsetMapFlag
//
// On success: ClearPendingAction → SetInteraction(Engine, loc,
// targetOpLocT, spellCom) → targetSubject snapshot.
func handleOpLocT(p *Player, payload []byte) error {
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
	locId := int(r.G2())
	spellCom := int(r.G2())

	com := s.lookupComponent(spellCom)
	if com == nil || (com.ActionTarget&objtype.ComActionTargetLoc) == 0 {
		sendUnsetMapFlag(p)
		return nil
	}
	if !p.IsComponentVisible(com) {
		sendUnsetMapFlag(p)
		return nil
	}

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

	loc := s.GetLoc(p.level, x, z, locId)
	if loc == nil {
		sendUnsetMapFlag(p)
		return nil
	}

	if s.locTypes == nil || locId < 0 || locId >= len(s.locTypes.Configs) || s.locTypes.Configs[locId] == nil {
		sendUnsetMapFlag(p)
		return nil
	}

	p.ClearPendingAction()
	p.opcalled = true
	p.SetInteraction(InteractionEngine, loc, targetOpLocT, spellCom)
	p.targetSubject.typ = loc.Type()
	p.targetSubject.x = loc.X
	p.targetSubject.z = loc.Z
	p.targetSubject.level = loc.Level
	return nil
}

// handleOpLocU is the handler for OPLOCU (opcode 75, 12-byte payload).
// Item-on-loc: player drags an inventory item onto a loc (e.g., axe on
// tree, tinderbox on logs, seed on patch).
// Payload = (x:G2, z:G2, locId:G2, useObj:G2, useSlot:G2, useCom:G2).
//
// Validation gates (subset of TS OpLocUHandler.ts:~79):
//  1. delayed player → UnsetMapFlag
//  2. payload too short → UnsetMapFlag
//  3. coords outside viewport → UnsetMapFlag
//  4. Server.GetLoc returns nil → UnsetMapFlag
//  5. LocType not registered → UnsetMapFlag
//  6. useCom not in invListeners → UnsetMapFlag (S6p)
//  7. listener's inventory unresolved or slot/item mismatch → UnsetMapFlag (S6p)
//  8. members-only item on free world → MessageGame + UnsetMapFlag (S6z)
//
// DEVIATION (S6m-D2): TS validates useCom references a usable, visible
// interface component (OpLocUHandler.ts:~25-35). Skipped — no component
// registry yet.
//
// S6m-D3 closed in S6p: per-op useCom listener lookup + slot/item
// validation gates added below, mirroring TS OpLocUHandler.ts:50-66.
//
// S6m-D4 closed in S6z: members-only items on free-to-play worlds
// are now rejected via the gate below, matching TS
// OpLocUHandler.ts:70-73.
//
// On success: set p.lastUseItem = useObj, p.lastUseSlot = useSlot →
// ClearPendingAction → SetInteraction(Engine, loc, targetOpLocU, -1) →
// targetSubject snapshot.
func handleOpLocU(p *Player, payload []byte) error {
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
	locId := int(r.G2())
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

	loc := s.GetLoc(p.level, x, z, locId)
	if loc == nil {
		sendUnsetMapFlag(p)
		return nil
	}

	if s.locTypes == nil || locId < 0 || locId >= len(s.locTypes.Configs) || s.locTypes.Configs[locId] == nil {
		sendUnsetMapFlag(p)
		return nil
	}

	// S6m-D3 closed: verify the player has an inv listener at useCom
	// and that the claimed item lives at the claimed slot (TS
	// OpLocUHandler.ts:50-66).
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

	// S6m-D4 closed in S6z: reject members-only items on
	// free-to-play worlds. Matches TS OpLocUHandler.ts:70-73.
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
	p.SetInteraction(InteractionEngine, loc, targetOpLocU, -1)
	p.targetSubject.typ = loc.Type()
	p.targetSubject.x = loc.X
	p.targetSubject.z = loc.Z
	p.targetSubject.level = loc.Level
	return nil
}
