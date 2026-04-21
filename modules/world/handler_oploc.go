package world

import (
	"github.com/zsrv/goscape/pkg/io/packet"
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
// Validation gates (mirrors TS OpLocTHandler.ts:~49):
//  1. delayed player → UnsetMapFlag
//  2. payload too short → UnsetMapFlag
//  3. coords outside viewport (52-tile half-extent) → UnsetMapFlag
//  4. Server.GetLoc returns nil → UnsetMapFlag
//  5. LocType not registered → UnsetMapFlag
//
// DEVIATION (S6m-D1): TS also validates spellCom references a component
// with ComActionTarget.LOC flag AND that the component is visible in the
// player's interface stack (OpLocTHandler.ts:~25-35). Skipped here
// because goscape has no component registry yet. Effective risk: client
// can forge spellCom values; scripts reading p.TargetSubjectCom() get
// raw wire values. Follow-up: "component registry + ComActionTarget
// validation" sub-spec.
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
//
// DEVIATION (S6m-D2): TS validates useCom references a usable, visible
// interface component (OpLocUHandler.ts:~25-35). Skipped — no component
// registry yet.
//
// DEVIATION (S6m-D3): TS does an inventory-listener lookup by useCom to
// verify the player has an inv listening at that interface, plus
// slot-bounds + item-at-slot-matches-useObj validation
// (OpLocUHandler.ts:~45-70). Goscape's invListeners is a slice, not a
// keyed map, so this lookup shape doesn't translate directly. Skip;
// scripts reading p.LastUseItem()/p.LastUseSlot() get raw wire values.
// Security risk: client can claim any item/slot. Real scripts
// defensively re-check via inv_getobj-style opcodes. Follow-up:
// "InvListener keyed-map refactor + OpLocU item validation" sub-spec.
//
// DEVIATION (S6m-D4): TS checks members-only items against NODE_MEMBERS
// server config (OpLocUHandler.ts:~71-77). Skipped because goscape has
// no members-config surface yet. Follow-up: "members-config + item-
// gating" sub-spec.
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
	_ = int(r.G2()) // useCom — deliberately discarded (S6m-D2/D3)

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

	p.lastUseItem = useObj
	p.lastUseSlot = useSlot

	p.ClearPendingAction()
	p.SetInteraction(InteractionEngine, loc, targetOpLocU, -1)
	p.targetSubject.typ = loc.Type()
	p.targetSubject.x = loc.X
	p.targetSubject.z = loc.Z
	p.targetSubject.level = loc.Level
	return nil
}
