package world

import (
	"github.com/zsrv/goscape/pkg/io/packet"
	"github.com/zsrv/goscape/pkg/objtype"
)

// handleOpPlayer is the shared implementation for OPPLAYER1..OPPLAYER4
// (real client only sends ops 1..4 — no OPPLAYER5 wire packet).
//
// Opcodes: OPPLAYER1=211, OPPLAYER2=219, OPPLAYER3=64, OPPLAYER4=43.
// Op is 1..4. Payload = u2 pid (goscape: slot — see identity-rule note below).
//
// Gates per TS OpPlayerHandler.ts (244):
//  1. delayed player → UnsetMapFlag (no clearPendingAction). TS:15-18.
//  2. payload too short → UnsetMapFlag (goscape-only guard; no TS analog).
//  3. target not found (LookupPlayerBySlot returns nil) → UnsetMapFlag +
//     clearPendingAction. TS:20-25.
//  4. target not visible (rsbuf.HasPlayer == false) → UnsetMapFlag +
//     clearPendingAction. TS:27-31.
//
// On success: clearPendingAction → SetInteraction(Engine, other, op, -1) →
// opcalled=true.
//
// Identity note: TS 244 decoder renames `playerSlot` to `pid`; goscape
// network layer identifies players by slot throughout. The field is the
// same wire u16; naming stays `slot` in Go per the established identity-rule
// convention (see T7 handler for precedent).
//
// The trigger arithmetic (TriggerApPlayer<N>, +7 → TriggerOpPlayer<N>)
// happens later in the trigger-fire path (player_interaction_trigger.go,
// landed in NAI-40 T5). TS 244 expanded the dispatch from arithmetic to
// explicit if/else (TS:33-42); the Go integer-op path is equivalent.
//
// NAI-40-D-OPPLAYER3-FOLLOWOP-NOT-PORTED closed by NAI-44 T5
// (processInteraction reshape with followOp + auto-clear).
func handleOpPlayer(p *Player, payload []byte, op int) error {
	if p.client == nil || p.client.server == nil {
		return nil
	}
	s := p.client.server

	// Gate 1: delayed player — no clearPendingAction. TS OpPlayerHandler.ts:15-18 (244).
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

	// Gate 3: target not found — clearPendingAction. TS OpPlayerHandler.ts:20-25 (244).
	other := s.LookupPlayerBySlot(slot)
	if other == nil {
		sendUnsetMapFlag(p)
		p.ClearPendingAction()
		return nil
	}

	// Gate 4: rsbuf visibility — clearPendingAction. TS OpPlayerHandler.ts:27-31 (244).
	if !s.rsbuf.HasPlayer(int32(p.slot), int32(other.slot)) {
		sendUnsetMapFlag(p)
		p.ClearPendingAction()
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

// handleOpPlayerT is the handler for OPPLAYERT (opcode 73, 4-byte payload).
// Spell-on-Player: player drags a spell icon onto another player.
// Payload = (pid:G2, spellComponent:G2).
//
// Gates per TS OpPlayerTHandler.ts (244):
//  1. delayed player → UnsetMapFlag (no clearPendingAction). TS:16-19.
//  2. payload too short → UnsetMapFlag (goscape-only guard).
//  3. spellCom: nil || !isVisible || (actionTarget&PLAYER)==0 → UnsetMapFlag +
//     clearPendingAction (combined check). TS:21-26.
//  4. target not found (LookupPlayerBySlot returns nil) → UnsetMapFlag +
//     clearPendingAction. TS:28-33.
//  5. target not visible (rsbuf.HasPlayer == false) → UnsetMapFlag +
//     clearPendingAction. TS:35-39.
//
// On success: clearPendingAction → SetInteraction(Engine, other,
// targetOpPlayerT, spellCom) → opcalled=true.
//
// Identity note: TS 244 renames `playerSlot` → `pid` in decoder; goscape
// keeps slot terminology (same wire value, established convention).
func handleOpPlayerT(p *Player, payload []byte) error {
	if p.client == nil || p.client.server == nil {
		return nil
	}
	s := p.client.server

	// Gate 1: delayed player — no clearPendingAction. TS OpPlayerTHandler.ts:16-19 (244).
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

	// Gate 3: combined component check — clearPendingAction. TS OpPlayerTHandler.ts:21-26 (244).
	// 244 combines nil/!visible/!actionTarget into a single if (was split at 225).
	com := s.lookupComponent(spellCom)
	if com == nil || !p.IsComponentVisible(com) || (com.ActionTarget&objtype.ComActionTargetPlayer) == 0 {
		sendUnsetMapFlag(p)
		p.ClearPendingAction()
		return nil
	}

	// Gate 4: target not found — clearPendingAction. TS OpPlayerTHandler.ts:28-33 (244).
	other := s.LookupPlayerBySlot(slot)
	if other == nil {
		sendUnsetMapFlag(p)
		p.ClearPendingAction()
		return nil
	}

	// Gate 5: rsbuf visibility — clearPendingAction. TS OpPlayerTHandler.ts:35-39 (244).
	if !s.rsbuf.HasPlayer(int32(p.slot), int32(other.slot)) {
		sendUnsetMapFlag(p)
		p.ClearPendingAction()
		return nil
	}

	p.ClearPendingAction()
	p.SetInteraction(InteractionEngine, other, targetOpPlayerT, spellCom)
	p.opcalled = true
	return nil
}

// handleOpPlayerU is the handler for OPPLAYERU (opcode 48, 8-byte payload).
// Item-on-Player: player drags an inventory item onto another player.
// Payload = (pid:G2, useObj:G2, useSlot:G2, useComponent:G2).
//
// Gates per TS OpPlayerUHandler.ts (244):
//  1. delayed player → UnsetMapFlag (no clearPendingAction). TS:18-21.
//  2. payload too short → UnsetMapFlag (goscape-only guard).
//  3. com: nil || !isVisible || !interactable → UnsetMapFlag + clearPendingAction
//     (combined check; was split at 225; usable renamed to interactable in 244). TS:23-27.
//  4. listener missing → UnsetMapFlag + clearPendingAction. TS:29-35.
//  5. inv nil or item-at-slot mismatch → UnsetMapFlag + clearPendingAction
//     (combined check). TS:37-42.
//  6. target not found → UnsetMapFlag + clearPendingAction. TS:44-48.
//  7. target not visible (rsbuf.HasPlayer == false) → UnsetMapFlag +
//     clearPendingAction. TS:50-54.
//  8. members-only item on free world → MessageGame + UnsetMapFlag (no
//     clearPendingAction here; clearPendingAction already called before). TS:58-62.
//
// On success: clearPendingAction → snapshot p.lastUseItem=useObj,
// p.lastUseSlot=useSlot → SetInteraction(Engine, other, targetOpPlayerU, useObj)
// (NAI-62: useObj threaded for trigger-lookup override per TS
// OpPlayerUHandler.ts:67 + Player.ts:993-995; useObj=0 canonicalised to
// com=-1 by SetInteraction per TS PathingEntity.ts:520) → opcalled=true.
//
// Identity note: TS 244 renames `playerSlot` → `pid` in decoder; goscape
// keeps slot terminology (same wire value, established convention).
func handleOpPlayerU(p *Player, payload []byte) error {
	if p.client == nil || p.client.server == nil {
		return nil
	}
	s := p.client.server

	// Gate 1: delayed player — no clearPendingAction. TS OpPlayerUHandler.ts:18-21 (244).
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

	// Gate 3: combined component check — clearPendingAction. TS OpPlayerUHandler.ts:23-27 (244).
	// 244 combines nil/!visible/!interactable into a single if (was split at 225;
	// field renamed from usable to interactable in 244).
	com := s.lookupComponent(useCom)
	if com == nil || !p.IsComponentVisible(com) || !com.Interactable {
		sendUnsetMapFlag(p)
		p.ClearPendingAction()
		return nil
	}

	// Gate 4: listener missing — clearPendingAction. TS OpPlayerUHandler.ts:29-35 (244).
	listener, ok := p.invListeners[useCom]
	if !ok {
		sendUnsetMapFlag(p)
		p.ClearPendingAction()
		return nil
	}

	// Gate 5: inv/slot/item combined check — clearPendingAction. TS OpPlayerUHandler.ts:37-42 (244).
	// TS adds explicit inv.validSlot(slot) to the combined check; goscape's HasAt
	// already covers slot-bounds via Inventory.Get (returns nil on out-of-bounds).
	inv := resolveListenerInv(s, listener)
	if inv == nil || !inv.HasAt(useSlot, useObj) {
		sendUnsetMapFlag(p)
		p.ClearPendingAction()
		return nil
	}

	// Gate 6: target not found — clearPendingAction. TS OpPlayerUHandler.ts:44-48 (244).
	other := s.LookupPlayerBySlot(slot)
	if other == nil {
		sendUnsetMapFlag(p)
		p.ClearPendingAction()
		return nil
	}

	// Gate 7: rsbuf visibility — clearPendingAction. TS OpPlayerUHandler.ts:50-54 (244).
	if !s.rsbuf.HasPlayer(int32(p.slot), int32(other.slot)) {
		sendUnsetMapFlag(p)
		p.ClearPendingAction()
		return nil
	}

	// clearPendingAction fires here, before the members check.
	// TS OpPlayerUHandler.ts:57 (244).
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
