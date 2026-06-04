package world

import (
	"github.com/zsrv/goscape/pkg/io/packet"
	"github.com/zsrv/goscape/pkg/objtype"
)

// handleOpLoc is the shared implementation for OPLOC1..OPLOC5.
// op is 1..5. Payload = 6 bytes: (x: G2, z: G2, locId: G2).
// Wire opcodes at 244: OPLOC1=238, OPLOC2=38, OPLOC3=19, OPLOC4=55, OPLOC5=243.
//
// Gate order per TS OpLocHandler.ts (244):
//  1. player.delayed → UnsetMapFlag (no clearPendingAction). TS:14-17.
//  2. payload < 6 bytes → UnsetMapFlag (goscape defensive).
//  3. viewport: outside ±52 of originX/Z → UnsetMapFlag + clearPendingAction. TS:23-27.
//  4. Server.GetLoc nil → UnsetMapFlag + clearPendingAction. TS:29-33.
//  5. LocType not registered → UnsetMapFlag + clearPendingAction.
//     (goscape defensive; TS skips this check; uses loc.type for the lookup — TS:36)
//  6. op slot absent or empty → UnsetMapFlag + clearPendingAction. TS:37-40.
//     Note: 'hidden' check removed at 244 — "hidden" is a non-empty string and is
//     truthy, passing the gate (TS: `!locType.op[message.op - 1]`).
//
// On success: clearPendingAction → SetInteraction(Engine, loc, op, -1) →
// opcalled=true → targetSubject snapshot.
func handleOpLoc(p *Player, payload []byte, op int) error {
	if p.client == nil || p.client.server == nil {
		return nil
	}
	s := p.client.server

	// Gate 1: delayed — no clearPendingAction. TS OpLocHandler.ts:14-17 (244).
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

	// Gate 3: viewport — clearPendingAction. TS OpLocHandler.ts:23-27 (244).
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
		p.ClearPendingAction()
		return nil
	}

	// Gate 4: loc missing — clearPendingAction. TS OpLocHandler.ts:29-33 (244).
	loc := s.GetLoc(p.level, x, z, locId)
	if loc == nil {
		emitOpLocGate(s, p, "getloc_nil", op, x, z, locId)
		sendUnsetMapFlag(p)
		p.ClearPendingAction()
		return nil
	}

	// Gate 5: goscape-only LocType registration check (TS skips this).
	// Uses loc.Type() per TS OpLocHandler.ts:36 (`LocType.get(loc.type)`).
	locTypeId := loc.Type()
	var locType *objtype.LocType
	if s.locTypes != nil && locTypeId >= 0 && locTypeId < len(s.locTypes.Configs) {
		locType = s.locTypes.Configs[locTypeId]
	}
	if locType == nil {
		emitOpLocGate(s, p, "loctype_nil", op, x, z, locId)
		sendUnsetMapFlag(p)
		p.ClearPendingAction()
		return nil
	}

	// Gate 6: op slot check. TS OpLocHandler.ts:37-40 (244).
	// Rejects if the op array is absent or the slot is empty/nil.
	// "hidden" is truthy at 244 and is NOT rejected here (225 drop).
	// Note: gates ALL ops — contrast with handleOpObj, which gates only
	// op1/op4 (TS OpObjHandler.ts:36-42 "todo: validate all options").
	if len(locType.Op) < op || locType.Op[op-1] == "" {
		emitOpLocGate(s, p, "op_slot_empty", op, x, z, locId)
		sendUnsetMapFlag(p)
		p.ClearPendingAction()
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

// handleOpLocT is the handler for OPLOCT (wire opcode 182 at 244, 8-byte payload).
// Spell-on-loc: player drags a spell icon from the magic-book interface
// onto a loc. Payload = (x:G2, z:G2, locId:G2, spellComponent:G2).
//
// Gate order per TS OpLocTHandler.ts (244):
//  1. player.delayed → UnsetMapFlag (no clearPendingAction). TS:14-17.
//  2. payload < 8 bytes → UnsetMapFlag (goscape defensive).
//  3. spellCom: nil || !isVisible || (actionTarget&LOC)==0 → UnsetMapFlag +
//     clearPendingAction (combined check at 244). TS:19-24.
//  4. viewport: outside ±52 → UnsetMapFlag + clearPendingAction. TS:29-34.
//  5. Server.GetLoc nil → moveClickRequest=false + clearPendingAction
//     (no UnsetMapFlag at 244). TS:36-40.
//
// Note: goscape's defensive locType check is removed at 244 — TS drops it.
//
// On success: clearPendingAction → SetInteraction(Engine, loc,
// targetOpLocT, spellCom) → opcalled=true → targetSubject snapshot.
func handleOpLocT(p *Player, payload []byte) error {
	if p.client == nil || p.client.server == nil {
		return nil
	}
	s := p.client.server

	// Gate 1: delayed — no clearPendingAction. TS OpLocTHandler.ts:14-17 (244).
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

	// Gate 3: combined component check — clearPendingAction on any failure.
	// TS OpLocTHandler.ts:19-24 (244): undefined || !isVisible || !actionTarget.
	com := s.lookupComponent(spellCom)
	if com == nil || !p.IsComponentVisible(com) || (com.ActionTarget&objtype.ComActionTargetLoc) == 0 {
		sendUnsetMapFlag(p)
		p.ClearPendingAction()
		return nil
	}

	// Gate 4: viewport — clearPendingAction. TS OpLocTHandler.ts:29-34 (244).
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

	// Gate 5: loc missing — moveClickRequest=false + clearPendingAction (no UnsetMapFlag).
	// TS OpLocTHandler.ts:36-40 (244). Cross-family asymmetry: OpObjTHandler.ts:37-41
	// sends UnsetMapFlag at the equivalent gate — OpLocT does NOT. Both TS-verified;
	// do not harmonize.
	loc := s.GetLoc(p.level, x, z, locId)
	if loc == nil {
		p.moveClickRequest = false
		p.ClearPendingAction()
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

// handleOpLocU is the handler for OPLOCU (wire opcode 106 at 244, 12-byte payload).
// Item-on-loc: player drags an inventory item onto a loc (e.g., axe on
// tree, tinderbox on logs, seed on patch).
// Payload = (x:G2, z:G2, locId:G2, useObj:G2, useSlot:G2, useComponent:G2).
//
// Gate order per TS OpLocUHandler.ts (244):
//  1. player.delayed → UnsetMapFlag (no clearPendingAction). TS:16-19.
//  2. payload < 12 bytes → UnsetMapFlag (goscape defensive).
//  3. com: nil || !isVisible || !interactable → UnsetMapFlag + clearPendingAction.
//     (244: uses interactable not usable; combined check; fires before viewport).
//     TS:21-26.
//  4. viewport: outside ±52 → UnsetMapFlag + clearPendingAction. TS:31-36.
//  5. listener not found → UnsetMapFlag + clearPendingAction. TS:38-43.
//  6. inv unresolved || !hasAt(slot, item) → UnsetMapFlag + clearPendingAction. TS:45-50.
//  7. Server.GetLoc nil → UnsetMapFlag + clearPendingAction. TS:52-57.
//     (loc lookup moved after inv check at 244)
//  8. members-only item on free world → MessageGame + UnsetMapFlag. TS:60-63.
//     (no clearPendingAction here — clearPendingAction already ran at success path)
//
// Note: goscape's defensive locType check is removed at 244 — TS drops it.
//
// On success: clearPendingAction (before gate 8) → (gate 8 members check) →
// lastUseItem/lastUseSlot → SetInteraction(Engine, loc, targetOpLocU, -1) →
// opcalled=true → targetSubject snapshot.
func handleOpLocU(p *Player, payload []byte) error {
	if p.client == nil || p.client.server == nil {
		return nil
	}
	s := p.client.server

	// Gate 1: delayed — no clearPendingAction. TS OpLocUHandler.ts:16-19 (244).
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

	// Gate 3: combined component check — clearPendingAction on any failure.
	// 244: checks com.Interactable (was com.Usable at 225); fires before viewport.
	// TS OpLocUHandler.ts:21-26 (244).
	com := s.lookupComponent(useCom)
	if com == nil || !p.IsComponentVisible(com) || !com.Interactable {
		sendUnsetMapFlag(p)
		p.ClearPendingAction()
		return nil
	}

	// Gate 4: viewport — clearPendingAction. TS OpLocUHandler.ts:31-36 (244).
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

	// Gate 5: listener check — clearPendingAction. TS OpLocUHandler.ts:38-43 (244).
	listener, ok := p.invListeners[useCom]
	if !ok {
		sendUnsetMapFlag(p)
		p.ClearPendingAction()
		return nil
	}

	// Gate 6: inv + item check — clearPendingAction. TS OpLocUHandler.ts:45-50 (244).
	// HasAt covers both validSlot (OOB slot → false) and item identity.
	inv := resolveListenerInv(s, listener)
	if inv == nil || !inv.HasAt(useSlot, useObj) {
		sendUnsetMapFlag(p)
		p.ClearPendingAction()
		return nil
	}

	// Gate 7: loc missing — clearPendingAction. TS OpLocUHandler.ts:52-57 (244).
	// Loc lookup moved after inv check at 244.
	loc := s.GetLoc(p.level, x, z, locId)
	if loc == nil {
		sendUnsetMapFlag(p)
		p.ClearPendingAction()
		return nil
	}

	p.ClearPendingAction()

	// Gate 8: members-only item on free world. TS OpLocUHandler.ts:60-63 (244).
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
