package world

import (
	"github.com/zsrv/goscape/pkg/io/packet"
	"github.com/zsrv/goscape/pkg/objtype"
)

// handleOpLoc is the shared implementation for OPLOC1..OPLOC5.
// op is 1..5. Payload = 6 bytes: (x: G2, z: G2, locId: G2).
//
// Gate order per TS OpLocHandler.ts @2e3bcf43 (the 254 pin removes
// clearPendingAction from EVERY rejection branch — it now runs only on
// the success path — and restores the explicit 'hidden' op rejection):
//  1. player.delayed → UnsetMapFlag. TS:15-19.
//  2. payload < 6 bytes → UnsetMapFlag (goscape defensive).
//  3. viewport: outside ±52 of originX/Z → UnsetMapFlag. TS:21-29.
//  4. Server.GetLoc nil → UnsetMapFlag. TS:31-36.
//  5. LocType not registered → UnsetMapFlag (goscape defensive; TS
//     LocType.get(locId) cannot miss for a loc that exists).
//  6. !locType.op || op[op-1] === null || op[op-1] === 'hidden' →
//     UnsetMapFlag. TS:38-43 ("" is the Go encoding of TS null).
//
// On success: clearPendingAction → SetInteraction(Engine, loc, op, -1) →
// opcalled=true → targetSubject snapshot. TS:45-49.
func handleOpLoc(p *Player, payload []byte, op int) error {
	if p.client == nil || p.client.server == nil {
		return nil
	}
	s := p.client.server

	// Gate 1: delayed. TS OpLocHandler.ts:15-19 @2e3bcf43.
	if p.delayed && s.currentTick < p.delayedUntil {
		emitOpLocGate(s, p, "delayed", op, -1, -1, -1)
		sendUnsetMapFlag(p)
		return nil
	}

	if len(payload) < 6 {
		emitOpLocGate(s, p, "payload_short", op, -1, -1, -1)
		sendUnsetMapFlag(p)
		return nil
	}

	r := packet.NewPacket(payload)
	x := int(r.G2())
	z := int(r.G2())
	locId := int(r.G2())

	// Gate 3: viewport. TS OpLocHandler.ts:21-29 @2e3bcf43.
	// 52 tiles is the player's render half-distance from origin.
	dx := x - p.originX
	if dx < 0 {
		dx = -dx
	}
	dz := z - p.originZ
	if dz < 0 {
		dz = -dz
	}
	if dx > 52 || dz > 52 {
		emitOpLocGate(s, p, "viewport", op, x, z, locId)
		sendUnsetMapFlag(p)
		return nil
	}

	// Gate 4: loc missing. TS OpLocHandler.ts:31-36 @2e3bcf43.
	loc := s.GetLoc(p.level, x, z, locId)
	if loc == nil {
		emitOpLocGate(s, p, "getloc_nil", op, x, z, locId)
		sendUnsetMapFlag(p)
		return nil
	}

	// Gate 5: goscape-only LocType registration check (TS skips this).
	// Uses loc.Type() (≡ locId once GetLoc matched).
	locTypeId := loc.Type()
	var locType *objtype.LocType
	if s.locTypes != nil && locTypeId >= 0 && locTypeId < len(s.locTypes.Configs) {
		locType = s.locTypes.Configs[locTypeId]
	}
	if locType == nil {
		emitOpLocGate(s, p, "loctype_nil", op, x, z, locId)
		sendUnsetMapFlag(p)
		return nil
	}

	// Gate 6: op slot check. TS OpLocHandler.ts:38-43 @2e3bcf43:
	// `!locType.op || locType.op[message.op - 1] === null ||
	//  locType.op[message.op - 1] === 'hidden'` — the explicit 'hidden'
	// rejection is BACK (244 accepted it as truthy).
	if len(locType.Op) < op || locType.Op[op-1] == "" || locType.Op[op-1] == "hidden" {
		emitOpLocGate(s, p, "op_slot_empty", op, x, z, locId)
		sendUnsetMapFlag(p)
		return nil
	}

	p.ClearPendingAction()
	p.SetInteraction(InteractionEngine, loc, op, -1)
	p.opcalled = true
	p.targetSubject.typ = loc.Type()
	p.targetSubject.x = loc.X
	p.targetSubject.z = loc.Z
	p.targetSubject.level = loc.Level

	// NAI-79 Stage 1 — handler frame for H1/H2 evidence channel.
	if s.cfg.NodeDebug && s.log != nil {
		s.log.Debug("oploc handler",
			"tick", s.currentTick,
			"player_uid", p.uid,
			"op", op,
			"click_x", x,
			"click_z", z,
			"loc_id", locId,
			"loc_name", locType.DebugName,
			"loc_shape", loc.Shape(),
			"loc_angle", loc.Angle(),
			"lt_width", locType.Width,
			"lt_length", locType.Length,
			"op_slot", locType.Op[op-1],
		)
	}
	return nil
}

func handleOpLoc1(p *Player, payload []byte) error { return handleOpLoc(p, payload, 1) }
func handleOpLoc2(p *Player, payload []byte) error { return handleOpLoc(p, payload, 2) }
func handleOpLoc3(p *Player, payload []byte) error { return handleOpLoc(p, payload, 3) }
func handleOpLoc4(p *Player, payload []byte) error { return handleOpLoc(p, payload, 4) }
func handleOpLoc5(p *Player, payload []byte) error { return handleOpLoc(p, payload, 5) }

// handleOpLocT is the handler for OPLOCT (8-byte payload).
// Spell-on-loc: player drags a spell icon from the magic-book interface
// onto a loc. Payload = (x:G2, z:G2, locId:G2, spellComponent:G2).
//
// Gate order per TS OpLocTHandler.ts @2e3bcf43 (clearPendingAction only
// on the success path at the 254 pin):
//  1. player.delayed → UnsetMapFlag. TS:15-19.
//  2. payload < 8 bytes → UnsetMapFlag (goscape defensive).
//  3. spellCom: nil || (actionTarget&LOC)==0, then !isVisible →
//     UnsetMapFlag. TS:21-30 (two branches; Go combines — same accept set).
//  4. viewport: outside ±52 → UnsetMapFlag. TS:32-40.
//  5. Server.GetLoc nil → UnsetMapFlag. TS:42-47 (the 244-era
//     moveClickRequest=false/no-UnsetMapFlag asymmetry is GONE — the
//     pin writes UnsetMapFlag like the rest of the family).
//
// On success: clearPendingAction → SetInteraction(Engine, loc,
// targetOpLocT, spellCom) → opcalled=true → targetSubject snapshot.
// TS:49-52.
func handleOpLocT(p *Player, payload []byte) error {
	if p.client == nil || p.client.server == nil {
		return nil
	}
	s := p.client.server

	// Gate 1: delayed. TS OpLocTHandler.ts:15-19 @2e3bcf43.
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

	// Gate 3: component check. TS OpLocTHandler.ts:21-30 @2e3bcf43.
	com := s.lookupComponent(spellCom)
	if com == nil || !p.IsComponentVisible(com) || (com.ActionTarget&objtype.ComActionTargetLoc) == 0 {
		sendUnsetMapFlag(p)
		return nil
	}

	// Gate 4: viewport. TS OpLocTHandler.ts:32-40 @2e3bcf43.
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

	// Gate 5: loc missing → UnsetMapFlag. TS OpLocTHandler.ts:42-47
	// @2e3bcf43 (244's moveClickRequest=false branch is gone).
	loc := s.GetLoc(p.level, x, z, locId)
	if loc == nil {
		sendUnsetMapFlag(p)
		return nil
	}

	p.ClearPendingAction()
	p.SetInteraction(InteractionEngine, loc, targetOpLocT, spellCom)
	p.opcalled = true
	p.targetSubject.typ = loc.Type()
	p.targetSubject.x = loc.X
	p.targetSubject.z = loc.Z
	p.targetSubject.level = loc.Level
	return nil
}

// handleOpLocU is the handler for OPLOCU (12-byte payload).
// Item-on-loc: player drags an inventory item onto a loc (e.g., axe on
// tree, tinderbox on logs, seed on patch).
// Payload = (x:G2, z:G2, locId:G2, useObj:G2, useSlot:G2, useComponent:G2).
//
// Gate order per TS OpLocUHandler.ts @2e3bcf43 (clearPendingAction only
// on the success path; the 254 pin REORDERS the gates — viewport and
// loc lookup now precede the component/inv checks — and the component
// gate reverts to com.usable):
//  1. player.delayed → UnsetMapFlag. TS:17-21.
//  2. payload < 12 bytes → UnsetMapFlag (goscape defensive).
//  3. viewport: outside ±52 → UnsetMapFlag. TS:23-31.
//  4. Server.GetLoc nil → UnsetMapFlag. TS:33-38.
//  5. useCom: nil || !usable, then !isVisible → UnsetMapFlag. TS:40-49.
//  6. listener/inv unresolved → UnsetMapFlag. TS:51-57.
//  7. !validSlot || !hasAt → UnsetMapFlag. TS:59-67.
//  8. members-only item on free world → MessageGame + UnsetMapFlag
//     (after clearPendingAction). TS:69-75.
//
// On success: clearPendingAction (before gate 8) → (gate 8 members
// check) → lastUseItem/lastUseSlot → SetInteraction(Engine, loc,
// targetOpLocU, -1) → opcalled=true → targetSubject snapshot. TS:69-82.
func handleOpLocU(p *Player, payload []byte) error {
	if p.client == nil || p.client.server == nil {
		return nil
	}
	s := p.client.server

	// Gate 1: delayed. TS OpLocUHandler.ts:17-21 @2e3bcf43.
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

	// Gate 3: viewport — moved BEFORE the component check at the pin.
	// TS OpLocUHandler.ts:23-31 @2e3bcf43.
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

	// Gate 4: loc missing — moved before the component check at the pin.
	// TS OpLocUHandler.ts:33-38 @2e3bcf43.
	loc := s.GetLoc(p.level, x, z, locId)
	if loc == nil {
		sendUnsetMapFlag(p)
		return nil
	}

	// Gate 5: component check — 254 reverts to com.usable (TS
	// OpLocUHandler.ts:40-49 @2e3bcf43 `!useCom.usable`; 244 had
	// interactable here).
	com := s.lookupComponent(useCom)
	if com == nil || !p.IsComponentVisible(com) || !com.Usable {
		sendUnsetMapFlag(p)
		return nil
	}

	// Gates 6+7: listener → inv → slot → item. TS OpLocUHandler.ts:51-67
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

	// Gate 8: members-only item on free world. TS OpLocUHandler.ts:71-75 @2e3bcf43.
	if s.objTypes != nil && useObj >= 0 && useObj < len(s.objTypes.Configs) {
		if useObjType := s.objTypes.Configs[useObj]; useObjType != nil && useObjType.Members && !s.cfg.NodeMembers {
			p.MessageGame("To use this item please login to a members' server.")
			sendUnsetMapFlag(p)
			return nil
		}
	}

	p.lastUseItem = useObj
	p.lastUseSlot = useSlot

	p.SetInteraction(InteractionEngine, loc, targetOpLocU, -1)
	p.opcalled = true
	p.targetSubject.typ = loc.Type()
	p.targetSubject.x = loc.X
	p.targetSubject.z = loc.Z
	p.targetSubject.level = loc.Level
	return nil
}
