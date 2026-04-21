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
