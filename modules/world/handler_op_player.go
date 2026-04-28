package world

import (
	"github.com/zsrv/goscape/pkg/io/packet"
)

// handleOpPlayer is the shared implementation for OPPLAYER1..OPPLAYER4
// (real client only sends ops 1..4 — no OPPLAYER5 wire packet).
//
// Op is 1..4. Payload = u2 PlayerSlot.
//
// Mirrors TS OpPlayerHandler.ts (45 lines): validate not-delayed,
// look up target by slot, validate visibility via rsbuf.HasPlayer,
// then anchor the engine interaction with op = msg.Op (1..4) and
// com = -1.
//
// The trigger arithmetic (TriggerApPlayer<N>, +7 → TriggerOpPlayer<N>)
// happens later in the trigger-fire path (player_interaction_trigger.go,
// landed in NAI-40 T5).
//
// DEVIATION NAI-40-D-OPCALLED-MISSING: TS sets player.opcalled = true
// at handler exit; goscape uses interactionFired (set by trigger fire)
// instead. Pre-existing S6a-era convention. Closure: NAI-40-SB1
// (cross-cutting opcalled-flag convergence).
//
// DEVIATION NAI-40-D-OPPLAYER3-FOLLOWOP-NOT-PORTED: TS Player.ts:1115
// special-cases targetOp == APPLAYER3 || OPPLAYER3 to keep the
// interaction anchored while chasing the target. Goscape fires-and-
// forgets. Tag-only; closure when player-script-lifecycle alignment
// sub-spec ports follow-op semantics.
func handleOpPlayer(p *Player, payload []byte, op int) error {
	if p.client == nil || p.client.server == nil {
		return nil
	}
	s := p.client.server

	if p.delayed && s.currentTick < p.delayedUntil {
		sendUnsetMapFlag(p)
		return nil
	}

	if len(payload) < 2 {
		sendUnsetMapFlag(p)
		return nil
	}

	r := packet.NewPacket(payload)
	slot := int(r.G2())

	other := s.LookupPlayerBySlot(slot)
	if other == nil {
		sendUnsetMapFlag(p)
		return nil
	}

	if !s.rsbuf.HasPlayer(int32(p.slot), int32(other.slot)) {
		sendUnsetMapFlag(p)
		return nil
	}

	p.ClearPendingAction()
	p.SetInteraction(InteractionEngine, other, op, -1)
	return nil
}

func handleOpPlayer1(p *Player, payload []byte) error { return handleOpPlayer(p, payload, 1) }
func handleOpPlayer2(p *Player, payload []byte) error { return handleOpPlayer(p, payload, 2) }
func handleOpPlayer3(p *Player, payload []byte) error { return handleOpPlayer(p, payload, 3) }
func handleOpPlayer4(p *Player, payload []byte) error { return handleOpPlayer(p, payload, 4) }
