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
// NAI-40-D-OPPLAYER3-FOLLOWOP-NOT-PORTED closed by NAI-44 T5
// (processInteraction reshape with followOp + auto-clear).
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

// handleOpPlayerU is the handler for OPPLAYERU (opcode 248, 8-byte payload).
// Item-on-Player: player drags an inventory item onto another player.
// Payload = (slot:G2, useObj:G2, useSlot:G2, useCom:G2).
//
// Validation gates (mirrors goscape's handleOpNpcU, NOT the full TS chain):
//  1. delayed player → UnsetMapFlag
//  2. payload too short → UnsetMapFlag
//  3. useCom not in invListeners → UnsetMapFlag
//  4. listener's inventory unresolved or item-at-slot mismatch → UnsetMapFlag
//  5. target not logged in → UnsetMapFlag
//  6. target not visible (rsbuf.HasPlayer == false) → UnsetMapFlag
//  7. members-only item on free world → MessageGame + UnsetMapFlag
//
// DEVIATION NAI-40-D-COMPONENT-REGISTRY-VALIDATION-SKIPPED: TS validates
// useCom references a usable, visible component. Skipped — same reason
// as S6o-D2 (NPC variant): no component registry yet. Closure: bundle
// with S6o-D2.
//
// DEVIATION NAI-40-D-OPCALLED-MISSING: see handleOpPlayer.
//
// On success: snapshot p.lastUseItem=useObj, p.lastUseSlot=useSlot →
// ClearPendingAction → SetInteraction(Engine, other, targetOpPlayerU, -1).
func handleOpPlayerU(p *Player, payload []byte) error {
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
	slot := int(r.G2())
	useObj := int(r.G2())
	useSlot := int(r.G2())
	useCom := int(r.G2())

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

	other := s.LookupPlayerBySlot(slot)
	if other == nil {
		sendUnsetMapFlag(p)
		return nil
	}

	if !s.rsbuf.HasPlayer(int32(p.slot), int32(other.slot)) {
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
	p.SetInteraction(InteractionEngine, other, targetOpPlayerU, -1)
	return nil
}
