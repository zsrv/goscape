package world

import (
	"github.com/zsrv/goscape/pkg/inventory"
	"github.com/zsrv/goscape/pkg/io/packet"
	"github.com/zsrv/goscape/pkg/objtype"
)

// resolveListenerInv returns the inventory the given listener observes,
// or nil if it can't be resolved. Source = -1 → world-shared inventory
// (Server.invs[Type]); otherwise Source is another player's UID, and
// the inventory is that player's local invs[Type]. Mirrors TS
// Player.getInventoryFromListener (Player.ts:getInventoryFromListener).
//
// NAI-114 Stage 5: prior to this fix Source was indexed directly into
// s.players[], which silently failed for any UID >= len(s.players)
// (always, in practice). Sister consumer Player.updateInvs already used
// LookupPlayerByUID; this function now matches.
func resolveListenerInv(s *Server, listener InventoryListener) *inventory.Inventory {
	if listener.Source == -1 {
		return s.invs[listener.Type]
	}
	otherActive := s.LookupPlayerByUID(listener.Source)
	if otherActive == nil {
		return nil
	}
	other, ok := otherActive.(*Player)
	if !ok || other == nil {
		return nil
	}
	return other.invs[listener.Type]
}

// handleOpNpc is the shared implementation for OPNPC1..OPNPC5.
// op is 1..5. Payload = p2(nid).
//
// Gate order per TS OpNpcHandler.ts @2e3bcf43 (the 254 pin removes
// clearPendingAction from EVERY rejection branch — it now runs only on
// the success path — and restores the explicit 'hidden' op rejection):
//  1. player.delayed → UnsetMapFlag. TS:16-20.
//  2. payload too short → UnsetMapFlag (goscape defensive).
//  3. !npc || npc.delayed → UnsetMapFlag. TS:22-31.
//  4. !hasNpc(player.slot, npc.nid) → UnsetMapFlag. TS:33-37.
//  5. !npcType.op || op[op-1] === null || op[op-1] === 'hidden' →
//     UnsetMapFlag. TS:39-44 ("" is the Go encoding of TS null).
//
// On success: clearPendingAction → setInteraction(ENGINE, npc, op, -1)
// → opcalled=true. TS:46-50.
func handleOpNpc(p *Player, payload []byte, op int) error {
	if p.client == nil || p.client.server == nil {
		return nil
	}
	s := p.client.server

	// Gate 1: delayed player. TS OpNpcHandler.ts:16-20 @2e3bcf43.
	if p.delayed && s.currentTick < p.delayedUntil {
		sendUnsetMapFlag(p)
		return nil
	}

	if len(payload) < 2 {
		sendUnsetMapFlag(p)
		return nil
	}

	r := packet.NewPacket(payload)
	nid := int(r.G2())

	if nid < 0 || nid >= len(s.npcs) {
		sendUnsetMapFlag(p)
		return nil
	}
	// Gate 3: merged !npc || npc.dead || npc.delayed. TS OpNpcHandler.ts:22-31.
	// npc.dead = goscape analog of TS getNpc returning undefined for despawned npcs
	// (dead npcs linger in s.npcs until cleanup); npc.dead predates 244 B2.
	npc := s.npcs[nid]
	if npc == nil || npc.dead || (npc.delayed && s.currentTick < npc.delayedUntil) {
		sendUnsetMapFlag(p)
		return nil
	}
	// Gate 4: rsbuf visibility. TS OpNpcHandler.ts:33-37.
	if !s.rsbuf.HasNpc(int32(p.slot), int32(npc.nid)) {
		sendUnsetMapFlag(p)
		return nil
	}

	// Gate 5: op check. TS OpNpcHandler.ts:39-44 @2e3bcf43:
	// `!npcType.op || npcType.op[message.op - 1] === null ||
	//  npcType.op[message.op - 1] === 'hidden'` — the explicit 'hidden'
	// rejection is BACK (244 accepted it as truthy).
	if npc.typ == nil || len(npc.typ.Op) < op || npc.typ.Op[op-1] == "" || npc.typ.Op[op-1] == "hidden" {
		sendUnsetMapFlag(p)
		return nil
	}

	p.ClearPendingAction()
	p.SetInteraction(InteractionEngine, npc, op, -1)
	p.opcalled = true
	return nil
}

func handleOpNpc1(p *Player, payload []byte) error { return handleOpNpc(p, payload, 1) }
func handleOpNpc2(p *Player, payload []byte) error { return handleOpNpc(p, payload, 2) }
func handleOpNpc3(p *Player, payload []byte) error { return handleOpNpc(p, payload, 3) }
func handleOpNpc4(p *Player, payload []byte) error { return handleOpNpc(p, payload, 4) }
func handleOpNpc5(p *Player, payload []byte) error { return handleOpNpc(p, payload, 5) }

// handleOpNpcT is the handler for OPNPCT (opcode 101, 4-byte payload).
// Spell-on-NPC: player drags a spell icon onto an NPC.
// Payload = (nid:G2, spellCom:G2).
//
// Gate order per TS OpNpcTHandler.ts @2e3bcf43 (clearPendingAction only
// on the success path at the 254 pin):
//  1. player.delayed → UnsetMapFlag. TS:16-20.
//  2. payload too short → UnsetMapFlag (goscape defensive).
//  3. com undefined || (actionTarget&NPC)==0, then !isVisible →
//     UnsetMapFlag. TS:22-31 (two branches; Go combines — same accept set).
//  4. !npc || npc.delayed → UnsetMapFlag. TS:33-42.
//  5. !hasNpc(player.slot, npc.nid) → UnsetMapFlag. TS:44-48.
//
// On success: clearPendingAction → setInteraction(ENGINE, npc,
// targetOpNpcT, spellCom) → opcalled=true. TS:50-53.
func handleOpNpcT(p *Player, payload []byte) error {
	if p.client == nil || p.client.server == nil {
		return nil
	}
	s := p.client.server

	// Gate 1: delayed player. TS OpNpcTHandler.ts:16-20 @2e3bcf43.
	if p.delayed && s.currentTick < p.delayedUntil {
		sendUnsetMapFlag(p)
		return nil
	}

	if len(payload) < 4 {
		sendUnsetMapFlag(p)
		return nil
	}

	r := packet.NewPacket(payload)
	nid := int(r.G2())
	spellCom := int(r.G2())

	// Gate 3: component check. TS OpNpcTHandler.ts:22-31 @2e3bcf43.
	com := s.lookupComponent(spellCom)
	if com == nil || !p.IsComponentVisible(com) || (com.ActionTarget&objtype.ComActionTargetNpc) == 0 {
		sendUnsetMapFlag(p)
		return nil
	}

	if nid < 0 || nid >= len(s.npcs) {
		sendUnsetMapFlag(p)
		return nil
	}
	// Gate 4: merged !npc || npc.dead || npc.delayed. TS OpNpcTHandler.ts:33-42.
	// npc.dead = goscape analog of TS getNpc returning undefined for despawned npcs
	// (dead npcs linger in s.npcs until cleanup); npc.dead predates 244 B2.
	npc := s.npcs[nid]
	if npc == nil || npc.dead || (npc.delayed && s.currentTick < npc.delayedUntil) {
		sendUnsetMapFlag(p)
		return nil
	}
	// Gate 5: rsbuf visibility. TS OpNpcTHandler.ts:44-48.
	if !s.rsbuf.HasNpc(int32(p.slot), int32(npc.nid)) {
		sendUnsetMapFlag(p)
		return nil
	}

	p.ClearPendingAction()
	p.SetInteraction(InteractionEngine, npc, targetOpNpcT, spellCom)
	p.opcalled = true
	return nil
}

// handleOpNpcU is the handler for OPNPCU (opcode 52, 8-byte payload).
// Item-on-NPC: player drags an inventory item onto an NPC (e.g., feed
// pet, give gift, sacrifice item).
// Payload = (nid:G2, useObj:G2, useSlot:G2, useCom:G2).
//
// Gate order per TS OpNpcUHandler.ts @2e3bcf43 (clearPendingAction only
// on the success path at the 254 pin; the component gate reverts to
// com.usable):
//  1. player.delayed → UnsetMapFlag. TS:18-22.
//  2. payload too short → UnsetMapFlag (goscape defensive).
//  3. com undefined || !usable, then !isVisible → UnsetMapFlag.
//     TS:24-33 (two branches; Go combines — same accept set).
//     4+5. listener/inv unresolved || !validSlot || !hasAt → UnsetMapFlag.
//     TS:35-51.
//  6. !npc || npc.delayed → UnsetMapFlag. TS:53-62.
//  7. !hasNpc(player.slot, npc.nid) → UnsetMapFlag. TS:64-68.
//  8. members-only item on free world → MessageGame + UnsetMapFlag
//     (after clearPendingAction). TS:70-76.
//
// On success: clearPendingAction → (gate 8 members check) → lastUseItem/lastUseSlot →
// setInteraction(ENGINE, npc, targetOpNpcU, -1) → opcalled=true. TS:70-83.
func handleOpNpcU(p *Player, payload []byte) error {
	if p.client == nil || p.client.server == nil {
		return nil
	}
	s := p.client.server

	// Gate 1: delayed player. TS OpNpcUHandler.ts:18-22 @2e3bcf43.
	if p.delayed && s.currentTick < p.delayedUntil {
		sendUnsetMapFlag(p)
		return nil
	}

	if len(payload) < 8 {
		sendUnsetMapFlag(p)
		return nil
	}

	r := packet.NewPacket(payload)
	nid := int(r.G2())
	useObj := int(r.G2())
	useSlot := int(r.G2())
	useCom := int(r.G2())

	// Gate 3: component check — 254 reverts to com.usable (TS
	// OpNpcUHandler.ts:24-33 @2e3bcf43 `!useCom.usable`; 244 had
	// interactable here).
	com := s.lookupComponent(useCom)
	if com == nil || !p.IsComponentVisible(com) || !com.Usable {
		sendUnsetMapFlag(p)
		return nil
	}

	// Gates 4+5: listener → inv → slot → item. TS OpNpcUHandler.ts:35-51
	// @2e3bcf43 (getInventoryFromListener takes the possibly-undefined
	// find result and returns null — one unified "inventory is not
	// transmitted" reject; validSlot/hasAt follow).
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

	if nid < 0 || nid >= len(s.npcs) {
		sendUnsetMapFlag(p)
		return nil
	}
	// Gate 6: merged !npc || npc.dead || npc.delayed. TS OpNpcUHandler.ts:53-62.
	// npc.dead = goscape analog of TS getNpc returning undefined for despawned npcs
	// (dead npcs linger in s.npcs until cleanup); npc.dead predates 244 B2.
	npc := s.npcs[nid]
	if npc == nil || npc.dead || (npc.delayed && s.currentTick < npc.delayedUntil) {
		sendUnsetMapFlag(p)
		return nil
	}
	// Gate 7: rsbuf visibility. TS OpNpcUHandler.ts:64-68.
	if !s.rsbuf.HasNpc(int32(p.slot), int32(npc.nid)) {
		sendUnsetMapFlag(p)
		return nil
	}

	p.ClearPendingAction()

	// Gate 8: members-only item on free world. TS OpNpcUHandler.ts:72-76 @2e3bcf43.
	if s.objTypes != nil && useObj >= 0 && useObj < len(s.objTypes.Configs) {
		if useObjType := s.objTypes.Configs[useObj]; useObjType != nil && useObjType.Members && !s.cfg.NodeMembers {
			p.MessageGame("To use this item please login to a members' server.")
			sendUnsetMapFlag(p)
			return nil
		}
	}

	p.lastUseItem = useObj
	p.lastUseSlot = useSlot

	p.SetInteraction(InteractionEngine, npc, targetOpNpcU, -1)
	p.opcalled = true
	return nil
}
