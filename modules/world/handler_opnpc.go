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
// Gate order per TS OpNpcHandler.ts (244):
//  1. player.delayed → UnsetMapFlag (no clearPendingAction). TS:16-19.
//  2. payload too short → UnsetMapFlag (goscape defensive).
//  3. !npc || npc.delayed → UnsetMapFlag + clearPendingAction. TS:21-26.
//  4. !hasNpc(player.pid, npc.nid) → UnsetMapFlag + clearPendingAction. TS:28-32.
//  5. !npcType.op || !npcType.op[op-1] → UnsetMapFlag + clearPendingAction. TS:34-39.
//     Note: at 244 the explicit `=== 'hidden'` check was removed; only falsy
//     (null/empty-string) rejects. In goscape, "hidden" is stored verbatim as a
//     non-empty string, so it is truthy and passes — matching 244 TS semantics.
//
// On success: clearPendingAction → setInteraction(ENGINE, npc, op, -1) → opcalled=true.
func handleOpNpc(p *Player, payload []byte, op int) error {
	if p.client == nil || p.client.server == nil {
		return nil
	}
	s := p.client.server

	// Gate 1: delayed player — no clearPendingAction. TS OpNpcHandler.ts:16-19 (244).
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
		p.ClearPendingAction()
		return nil
	}
	// Gate 3: merged !npc || npc.dead || npc.delayed — clearPendingAction on any branch.
	// TS OpNpcHandler.ts:21-26 (244).
	// npc.dead = goscape analog of TS getNpc returning undefined for despawned npcs
	// (dead npcs linger in s.npcs until cleanup); npc.dead predates 244 B2.
	npc := s.npcs[nid]
	if npc == nil || npc.dead || (npc.delayed && s.currentTick < npc.delayedUntil) {
		sendUnsetMapFlag(p)
		p.ClearPendingAction()
		return nil
	}
	// Gate 4: rsbuf visibility — clearPendingAction. TS OpNpcHandler.ts:28-32 (244).
	if !s.rsbuf.HasNpc(int32(p.pid), int32(npc.nid)) {
		sendUnsetMapFlag(p)
		p.ClearPendingAction()
		return nil
	}

	// Gate 5: op check — only empty string (falsy) rejects; "hidden" is truthy.
	// TS OpNpcHandler.ts:34-39 (244): `!npcType.op || !npcType.op[op-1]`.
	if npc.typ == nil || len(npc.typ.Op) < op || npc.typ.Op[op-1] == "" {
		sendUnsetMapFlag(p)
		p.ClearPendingAction()
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
// Gate order per TS OpNpcTHandler.ts (244):
//  1. player.delayed → UnsetMapFlag (no clearPendingAction). TS:16-19.
//  2. payload too short → UnsetMapFlag (goscape defensive).
//  3. com undefined || !isVisible || (actionTarget&NPC)==0 → UnsetMapFlag
//     + clearPendingAction. TS:21-26 (244 combined check).
//  4. !npc || npc.delayed → UnsetMapFlag + clearPendingAction. TS:28-33.
//  5. !hasNpc(player.pid, npc.nid) → UnsetMapFlag + clearPendingAction. TS:35-39.
//
// On success: clearPendingAction → setInteraction(ENGINE, npc,
// targetOpNpcT, spellCom) → opcalled=true. TS:41-44.
func handleOpNpcT(p *Player, payload []byte) error {
	if p.client == nil || p.client.server == nil {
		return nil
	}
	s := p.client.server

	// Gate 1: delayed player — no clearPendingAction. TS OpNpcTHandler.ts:16-19 (244).
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

	// Gate 3: combined component check — clearPendingAction on any failure.
	// TS OpNpcTHandler.ts:21-26 (244): undefined || !isVisible || !actionTarget.
	com := s.lookupComponent(spellCom)
	if com == nil || !p.IsComponentVisible(com) || (com.ActionTarget&objtype.ComActionTargetNpc) == 0 {
		sendUnsetMapFlag(p)
		p.ClearPendingAction()
		return nil
	}

	if nid < 0 || nid >= len(s.npcs) {
		sendUnsetMapFlag(p)
		p.ClearPendingAction()
		return nil
	}
	// Gate 4: merged !npc || npc.dead || npc.delayed — clearPendingAction on any branch.
	// TS OpNpcTHandler.ts:28-33 (244).
	// npc.dead = goscape analog of TS getNpc returning undefined for despawned npcs
	// (dead npcs linger in s.npcs until cleanup); npc.dead predates 244 B2.
	npc := s.npcs[nid]
	if npc == nil || npc.dead || (npc.delayed && s.currentTick < npc.delayedUntil) {
		sendUnsetMapFlag(p)
		p.ClearPendingAction()
		return nil
	}
	// Gate 5: rsbuf visibility — clearPendingAction. TS OpNpcTHandler.ts:35-39 (244).
	if !s.rsbuf.HasNpc(int32(p.pid), int32(npc.nid)) {
		sendUnsetMapFlag(p)
		p.ClearPendingAction()
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
// Gate order per TS OpNpcUHandler.ts (244):
//  1. player.delayed → UnsetMapFlag (no clearPendingAction). TS:18-21.
//  2. payload too short → UnsetMapFlag (goscape defensive).
//  3. com undefined || !isVisible || !interactable → UnsetMapFlag
//     + clearPendingAction. TS:23-28 (244: usable→interactable, combined check).
//  4. listener not found → UnsetMapFlag + clearPendingAction. TS:30-35.
//  5. inv unresolved || !validSlot || !hasAt → UnsetMapFlag + clearPendingAction. TS:37-42.
//  6. !npc || npc.delayed → UnsetMapFlag + clearPendingAction. TS:44-49.
//  7. !hasNpc(player.pid, npc.nid) → UnsetMapFlag + clearPendingAction. TS:51-55.
//  8. members-only item on free world → MessageGame + UnsetMapFlag. TS:58-61.
//
// On success: clearPendingAction → (gate 8 members check) → lastUseItem/lastUseSlot →
// setInteraction(ENGINE, npc, targetOpNpcU, -1) → opcalled=true. TS:57-69.
func handleOpNpcU(p *Player, payload []byte) error {
	if p.client == nil || p.client.server == nil {
		return nil
	}
	s := p.client.server

	// Gate 1: delayed player — no clearPendingAction. TS OpNpcUHandler.ts:18-21 (244).
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

	// Gate 3: combined component check — clearPendingAction on any failure.
	// 244: checks com.interactable (was com.usable at 225).
	// TS OpNpcUHandler.ts:23-28 (244).
	com := s.lookupComponent(useCom)
	if com == nil || !p.IsComponentVisible(com) || !com.Interactable {
		sendUnsetMapFlag(p)
		p.ClearPendingAction()
		return nil
	}

	// Gate 4: listener check — clearPendingAction. TS OpNpcUHandler.ts:30-35 (244).
	listener, ok := p.invListeners[useCom]
	if !ok {
		sendUnsetMapFlag(p)
		p.ClearPendingAction()
		return nil
	}
	// Gate 5: inv + slot + item check — clearPendingAction. TS OpNpcUHandler.ts:37-42 (244).
	inv := resolveListenerInv(s, listener)
	if inv == nil || !inv.HasAt(useSlot, useObj) {
		sendUnsetMapFlag(p)
		p.ClearPendingAction()
		return nil
	}

	if nid < 0 || nid >= len(s.npcs) {
		sendUnsetMapFlag(p)
		p.ClearPendingAction()
		return nil
	}
	// Gate 6: merged !npc || npc.dead || npc.delayed — clearPendingAction on any branch.
	// TS OpNpcUHandler.ts:44-49 (244).
	// npc.dead = goscape analog of TS getNpc returning undefined for despawned npcs
	// (dead npcs linger in s.npcs until cleanup); npc.dead predates 244 B2.
	npc := s.npcs[nid]
	if npc == nil || npc.dead || (npc.delayed && s.currentTick < npc.delayedUntil) {
		sendUnsetMapFlag(p)
		p.ClearPendingAction()
		return nil
	}
	// Gate 7: rsbuf visibility — clearPendingAction. TS OpNpcUHandler.ts:51-55 (244).
	if !s.rsbuf.HasNpc(int32(p.pid), int32(npc.nid)) {
		sendUnsetMapFlag(p)
		p.ClearPendingAction()
		return nil
	}

	p.ClearPendingAction()

	// Gate 8: members-only item on free world. TS OpNpcUHandler.ts:58-61 (244).
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
