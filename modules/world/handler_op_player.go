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

// handleOpPlayerT is the handler for OPPLAYERT (opcode 177, 4-byte payload).
// Spell-on-Player: player drags a spell icon onto another player.
// Payload = (slot:G2, spellCom:G2).
//
// Validation gates (mirrors goscape's handleOpNpcT, NOT the full TS chain):
//  1. delayed player → UnsetMapFlag
//  2. payload too short → UnsetMapFlag
//  3. target not logged in (LookupPlayerBySlot returns nil) → UnsetMapFlag
//  4. target not visible (rsbuf.HasPlayer == false) → UnsetMapFlag
//
// DEVIATION NAI-40-D-COMPONENT-REGISTRY-VALIDATION-SKIPPED: TS validates
// spellCom references a component with ComActionTarget.PLAYER flag AND
// is visible in the player's interface stack. Skipped here for the same
// reason as S6o-D1 (NPC variant) — goscape has no component registry
// yet. Effective risk: client can forge spellCom values; scripts reading
// p.TargetSubjectCom() get raw wire values. Closure: bundle with S6o-D1
// when the component-registry sub-spec lands.
//
// DEVIATION NAI-40-D-OPCALLED-MISSING: see handleOpPlayer.
//
// On success: ClearPendingAction → SetInteraction(Engine, other,
// targetOpPlayerT, spellCom).
func handleOpPlayerT(p *Player, payload []byte) error {
	if p.client == nil || p.client.server == nil {
		return nil
	}
	s := p.client.server

	if p.delayed && s.currentTick < p.delayedUntil {
		sendUnsetMapFlag(p)
		return nil
	}

	if len(payload) < 4 {
		sendUnsetMapFlag(p)
		return nil
	}

	r := packet.NewPacket(payload)
	slot := int(r.G2())
	spellCom := int(r.G2())

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
	p.SetInteraction(InteractionEngine, other, targetOpPlayerT, spellCom)
	return nil
}
