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
//  3. coords outside player's build-area viewport (104 tiles each axis
//     from p.originX/originZ) → UnsetMapFlag
//  4. Server.GetLoc returns nil → UnsetMapFlag
//  5. LocType not registered → UnsetMapFlag
//
// DEVIATION (S6j-D1): TS gate 6 — `locType.op[op-1] != null && != "hidden"`
// (OpLocHandler.ts:38-42) — is skipped here because LocType.Op []string is
// not yet a field on LocType. Effective behavior: trigger registration
// absence becomes the gate (no trigger → silent no-op on next tick instead
// of TS's UnsetMapFlag at click time). Follow-up: "LocType.Op + loc_op
// script opcode" sub-spec.
//
// On success: ClearPendingAction → SetInteraction(Engine, loc, op) →
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

	// Viewport gate. Build area is ~13 zones × 8 tiles per side from
	// origin; 104 tiles is the half-extent. Mirrors TS OpLocHandler.ts:20-28
	// scene-bounds rejection.
	dx := x - p.originX
	if dx < 0 {
		dx = -dx
	}
	dz := z - p.originZ
	if dz < 0 {
		dz = -dz
	}
	if dx > 104 || dz > 104 {
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
	p.SetInteraction(InteractionEngine, loc, op)
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
