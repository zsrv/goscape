package world

import (
	"github.com/zsrv/goscape/pkg/io/packet"
	"github.com/zsrv/goscape/pkg/objtype"
)

// handleOpPlayer is the shared implementation for OPPLAYER1..OPPLAYER5
// (254 adds the OPPLAYER5 wire packet for SET_PLAYER_OP slot 5).
//
// Opcodes (254): OPPLAYER1=192, OPPLAYER2=17, OPPLAYER3=18, OPPLAYER4=72,
// OPPLAYER5=230. Op is 1..5. Payload = u2 playerSlot (TS OpPlayer.playerSlot @2e3bcf43).
//
// Gates per TS OpPlayerHandler.ts @2e3bcf43 (clearPendingAction only on
// the success path at the 254 pin):
//  1. delayed player → UnsetMapFlag. TS:15-19.
//  2. payload too short → UnsetMapFlag (goscape-only guard; no TS analog).
//  3. target not found (LookupPlayerBySlot returns nil) → UnsetMapFlag. TS:21-26.
//  4. target not visible (rsbuf.HasPlayer == false) → UnsetMapFlag. TS:28-32.
//
// On success: clearPendingAction → SetInteraction(Engine, other, op, -1) →
// opcalled=true.
//
// The trigger arithmetic (TriggerApPlayer<N>, +7 → TriggerOpPlayer<N>)
// happens later in the trigger-fire path (player_interaction_trigger.go,
// landed in NAI-40 T5). TS 254 dispatches via explicit if/else with the
// final else mapping to APPLAYER5 (TS OpPlayerHandler.ts:35-46
// @43e02957); the Go integer-op path is equivalent for ops 1..5.
//
// NAI-40-D-OPPLAYER3-FOLLOWOP-NOT-PORTED closed by NAI-44 T5
// (processInteraction reshape with followOp + auto-clear).
func handleOpPlayer(p *Player, payload []byte, op int) error {
	if p.client == nil || p.client.server == nil {
		return nil
	}
	s := p.client.server

	// Gate 1: delayed player. TS OpPlayerHandler.ts:15-19 @2e3bcf43.
	if p.delayed && s.currentTick < p.delayedUntil {
		sendUnsetMapFlag(p)
		return nil
	}

	if len(payload) < 2 {
		sendUnsetMapFlag(p)
		return nil
	}

	r := packet.NewPacket(payload)
	playerSlot := int(r.G2())

	// Gate 3: target not found. TS OpPlayerHandler.ts:21-26 @2e3bcf43.
	other := s.LookupPlayerBySlot(playerSlot)
	if other == nil {
		sendUnsetMapFlag(p)
		return nil
	}

	// Gate 4: rsbuf visibility. TS OpPlayerHandler.ts:28-32 @2e3bcf43.
	if !s.rsbuf.HasPlayer(int32(p.slot), int32(other.slot)) {
		sendUnsetMapFlag(p)
		return nil
	}

	p.ClearPendingAction()
	p.SetInteraction(InteractionEngine, other, op, -1)
	p.opcalled = true
	return nil
}

func handleOpPlayer1(p *Player, payload []byte) error { return handleOpPlayer(p, payload, 1) }
func handleOpPlayer2(p *Player, payload []byte) error { return handleOpPlayer(p, payload, 2) }
func handleOpPlayer3(p *Player, payload []byte) error { return handleOpPlayer(p, payload, 3) }
func handleOpPlayer4(p *Player, payload []byte) error { return handleOpPlayer(p, payload, 4) }
func handleOpPlayer5(p *Player, payload []byte) error { return handleOpPlayer(p, payload, 5) }

// handleOpPlayerT is the handler for OPPLAYERT (opcode 73, 4-byte payload).
// Spell-on-Player: player drags a spell icon onto another player.
// Payload = (playerSlot:G2, spellComponent:G2).
//
// Gates per TS OpPlayerTHandler.ts @2e3bcf43 (clearPendingAction only on
// the success path at the 254 pin):
//  1. delayed player → UnsetMapFlag. TS:16-20.
//  2. payload too short → UnsetMapFlag (goscape-only guard).
//  3. spellCom: nil || (actionTarget&PLAYER)==0, then !isVisible →
//     UnsetMapFlag. TS:22-31 (two branches; Go combines — same accept set).
//  4. target not found (LookupPlayerBySlot returns nil) → UnsetMapFlag. TS:33-38.
//  5. target not visible (rsbuf.HasPlayer == false) → UnsetMapFlag. TS:40-44.
//
// On success: clearPendingAction → SetInteraction(Engine, other,
// targetOpPlayerT, spellCom) → opcalled=true.
func handleOpPlayerT(p *Player, payload []byte) error {
	if p.client == nil || p.client.server == nil {
		return nil
	}
	s := p.client.server

	// Gate 1: delayed player. TS OpPlayerTHandler.ts:16-20 @2e3bcf43.
	if p.delayed && s.currentTick < p.delayedUntil {
		sendUnsetMapFlag(p)
		return nil
	}

	if len(payload) < 4 {
		sendUnsetMapFlag(p)
		return nil
	}

	r := packet.NewPacket(payload)
	playerSlot := int(r.G2())
	spellCom := int(r.G2())

	// Gate 3: component check. TS OpPlayerTHandler.ts:22-31 @2e3bcf43.
	com := s.lookupComponent(spellCom)
	if com == nil || !p.IsComponentVisible(com) || (com.ActionTarget&objtype.ComActionTargetPlayer) == 0 {
		sendUnsetMapFlag(p)
		return nil
	}

	// Gate 4: target not found. TS OpPlayerTHandler.ts:33-38 @2e3bcf43.
	other := s.LookupPlayerBySlot(playerSlot)
	if other == nil {
		sendUnsetMapFlag(p)
		return nil
	}

	// Gate 5: rsbuf visibility. TS OpPlayerTHandler.ts:40-44 @2e3bcf43.
	if !s.rsbuf.HasPlayer(int32(p.slot), int32(other.slot)) {
		sendUnsetMapFlag(p)
		return nil
	}

	p.ClearPendingAction()
	p.SetInteraction(InteractionEngine, other, targetOpPlayerT, spellCom)
	p.opcalled = true
	return nil
}

// handleOpPlayerU is the handler for OPPLAYERU (opcode 48, 8-byte payload).
// Item-on-Player: player drags an inventory item onto another player.
// Payload = (playerSlot:G2, useObj:G2, useSlot:G2, useComponent:G2).
//
// Gates per TS OpPlayerUHandler.ts @2e3bcf43 (clearPendingAction only on
// the success path at the 254 pin; the component gate reverts to
// com.usable):
//  1. delayed player → UnsetMapFlag. TS:19-23.
//  2. payload too short → UnsetMapFlag (goscape-only guard).
//  3. useCom: nil || !usable, then !isVisible → UnsetMapFlag. TS:25-34
//     (two branches; Go combines — same accept set).
//     4+5. listener/inv unresolved || !validSlot || !hasAt → UnsetMapFlag.
//     TS:36-52.
//  6. target not found → UnsetMapFlag. TS:54-59.
//  7. target not visible (rsbuf.HasPlayer == false) → UnsetMapFlag. TS:61-65.
//  8. members-only item on free world → MessageGame + UnsetMapFlag
//     (after clearPendingAction). TS:67-73.
//
// On success: clearPendingAction → snapshot p.lastUseItem=useObj,
// p.lastUseSlot=useSlot → SetInteraction(Engine, other, targetOpPlayerU, useObj)
// (NAI-62: useObj threaded for trigger-lookup override per TS
// OpPlayerUHandler.ts:67 + Player.ts:993-995; useObj=0 canonicalised to
// com=-1 by SetInteraction per TS PathingEntity.ts:520) → opcalled=true.
func handleOpPlayerU(p *Player, payload []byte) error {
	if p.client == nil || p.client.server == nil {
		return nil
	}
	s := p.client.server

	// Gate 1: delayed player. TS OpPlayerUHandler.ts:19-23 @2e3bcf43.
	if p.delayed && s.currentTick < p.delayedUntil {
		sendUnsetMapFlag(p)
		return nil
	}

	if len(payload) < 8 {
		sendUnsetMapFlag(p)
		return nil
	}

	r := packet.NewPacket(payload)
	playerSlot := int(r.G2())
	useObj := int(r.G2())
	useSlot := int(r.G2())
	useCom := int(r.G2())

	// Gate 3: component check — 254 reverts to com.usable (TS
	// OpPlayerUHandler.ts:25-34 @2e3bcf43 `!useCom.usable`; 244 had
	// interactable here).
	com := s.lookupComponent(useCom)
	if com == nil || !p.IsComponentVisible(com) || !com.Usable {
		sendUnsetMapFlag(p)
		return nil
	}

	// Gates 4+5: listener → inv → slot → item. TS OpPlayerUHandler.ts:36-52 @2e3bcf43.
	listener, ok := p.invListeners[useCom]
	if !ok {
		sendUnsetMapFlag(p)
		return nil
	}

	// HasAt covers both validSlot (OOB slot → false) and item identity.
	inv := resolveListenerInv(s, listener)
	if inv == nil || !inv.HasAt(useSlot, useObj) {
		sendUnsetMapFlag(p)
		return nil
	}

	// Gate 6: target not found. TS OpPlayerUHandler.ts:54-59 @2e3bcf43.
	other := s.LookupPlayerBySlot(playerSlot)
	if other == nil {
		sendUnsetMapFlag(p)
		return nil
	}

	// Gate 7: rsbuf visibility. TS OpPlayerUHandler.ts:61-65 @2e3bcf43.
	if !s.rsbuf.HasPlayer(int32(p.slot), int32(other.slot)) {
		sendUnsetMapFlag(p)
		return nil
	}

	// clearPendingAction fires here, before the members check.
	// TS OpPlayerUHandler.ts:67 @2e3bcf43.
	p.ClearPendingAction()

	if s.objTypes != nil && useObj >= 0 && useObj < len(s.objTypes.Configs) {
		if useObjType := s.objTypes.Configs[useObj]; useObjType != nil && useObjType.Members && !s.cfg.NodeMembers {
			p.MessageGame("To use this item please login to a members' server.")
			sendUnsetMapFlag(p)
			return nil
		}
	}

	p.lastUseItem = useObj
	p.lastUseSlot = useSlot

	p.SetInteraction(InteractionEngine, other, targetOpPlayerU, useObj)
	p.opcalled = true
	return nil
}
