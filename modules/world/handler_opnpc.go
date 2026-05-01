package world

import (
	"github.com/zsrv/goscape/pkg/inventory"
	"github.com/zsrv/goscape/pkg/io/packet"
	"github.com/zsrv/goscape/pkg/objtype"
)

// resolveListenerInv returns the inventory the given listener observes,
// or nil if it can't be resolved. Source = -1 → world-shared inventory
// (Server.invs[Type]); otherwise the source is another player's slot,
// and the inventory is that player's local invs[Type]. Mirrors TS
// getInventoryFromListener in Player.ts.
func resolveListenerInv(s *Server, listener InventoryListener) *inventory.Inventory {
	if listener.Source == -1 {
		return s.invs[listener.Type]
	}
	if listener.Source < 0 || listener.Source >= len(s.players) {
		return nil
	}
	other := s.players[listener.Source]
	if other == nil {
		return nil
	}
	return other.invs[listener.Type]
}

// handleOpNpc is the shared implementation for OPNPC1..OPNPC5.
// op is 1..5. Payload = p2(slot).
func handleOpNpc(p *Player, payload []byte, op int) error {
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

	if slot < 0 || slot >= len(s.npcs) {
		sendUnsetMapFlag(p)
		return nil
	}
	npc := s.npcs[slot]
	if npc == nil || npc.dead {
		sendUnsetMapFlag(p)
		return nil
	}
	if npc.delayed && s.currentTick < npc.delayedUntil {
		sendUnsetMapFlag(p)
		return nil
	}
	if !s.rsbuf.HasNpc(int32(p.slot), int32(npc.nid)) {
		sendUnsetMapFlag(p)
		return nil
	}

	// NpcType.Op[op-1] must be a non-empty, non-"hidden" string.
	// RuneScript will later replace this with trigger-existence lookup.
	if npc.typ == nil || len(npc.typ.Op) < op ||
		npc.typ.Op[op-1] == "" || npc.typ.Op[op-1] == "hidden" {
		sendUnsetMapFlag(p)
		return nil
	}

	p.ClearPendingAction()
	p.opcalled = true
	p.SetInteraction(InteractionEngine, npc, op, -1)
	return nil
}

func handleOpNpc1(p *Player, payload []byte) error { return handleOpNpc(p, payload, 1) }
func handleOpNpc2(p *Player, payload []byte) error { return handleOpNpc(p, payload, 2) }
func handleOpNpc3(p *Player, payload []byte) error { return handleOpNpc(p, payload, 3) }
func handleOpNpc4(p *Player, payload []byte) error { return handleOpNpc(p, payload, 4) }
func handleOpNpc5(p *Player, payload []byte) error { return handleOpNpc(p, payload, 5) }

// handleOpNpcT is the handler for OPNPCT (opcode 134, 4-byte payload).
// Spell-on-NPC: player drags a spell icon onto an NPC.
// Payload = (slot:G2, spellCom:G2).
//
// Gates per TS OpNpcTHandler.ts:
//  1. delayed player → UnsetMapFlag
//  2. payload too short → UnsetMapFlag
//  3. spellCom: nil component or ActionTarget&NPC == 0 → UnsetMapFlag
//  4. spellCom: !IsComponentVisible → UnsetMapFlag
//  5. slot out of range → UnsetMapFlag
//  6. NPC nil or dead → UnsetMapFlag
//  7. NPC delayed → UnsetMapFlag
//  8. NPC not rsbuf-visible → UnsetMapFlag
//  9. NpcType nil → UnsetMapFlag  (goscape defensive; TS skips — type always valid when npc exists)
//
// On success: ClearPendingAction → SetInteraction(Engine, npc,
// targetOpNpcT, spellCom).
func handleOpNpcT(p *Player, payload []byte) error {
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

	com := s.lookupComponent(spellCom)
	if com == nil || (com.ActionTarget&objtype.ComActionTargetNpc) == 0 {
		sendUnsetMapFlag(p)
		return nil
	}
	if !p.IsComponentVisible(com) {
		sendUnsetMapFlag(p)
		return nil
	}

	if slot < 0 || slot >= len(s.npcs) {
		sendUnsetMapFlag(p)
		return nil
	}
	npc := s.npcs[slot]
	if npc == nil || npc.dead {
		sendUnsetMapFlag(p)
		return nil
	}
	if npc.delayed && s.currentTick < npc.delayedUntil {
		sendUnsetMapFlag(p)
		return nil
	}
	if !s.rsbuf.HasNpc(int32(p.slot), int32(npc.nid)) {
		sendUnsetMapFlag(p)
		return nil
	}
	if npc.typ == nil {
		sendUnsetMapFlag(p)
		return nil
	}

	p.ClearPendingAction()
	p.opcalled = true
	p.SetInteraction(InteractionEngine, npc, targetOpNpcT, spellCom)
	return nil
}

// handleOpNpcU is the handler for OPNPCU (opcode 202, 8-byte payload).
// Item-on-NPC: player drags an inventory item onto an NPC (e.g., feed
// pet, give gift, sacrifice item).
// Payload = (slot:G2, useObj:G2, useSlot:G2, useCom:G2).
//
// Validation gates (subset of TS OpNpcUHandler.ts):
//  1. delayed player → UnsetMapFlag
//  2. payload too short → UnsetMapFlag
//  3. slot out of range → UnsetMapFlag
//  4. NPC nil or dead → UnsetMapFlag
//  5. NpcType nil → UnsetMapFlag
//  6. useCom not in invListeners → UnsetMapFlag (S6p)
//  7. listener's inventory unresolved or slot/item mismatch → UnsetMapFlag (S6p)
//  8. members-only item on free world → MessageGame + UnsetMapFlag (S6z)
//
// DEVIATION S6o-D2: TS validates useCom references a usable, visible
// interface component. Skipped — no component registry. (Mirrors S6m-D2.)
//
// S6o-D3 closed in S6p: per-op useCom listener lookup + slot/item
// validation gates added below, mirroring TS OpNpcUHandler.ts:35-50.
//
// S6o-D4 closed in S6z: members-only items on free-to-play worlds
// are now rejected via the gate below, matching TS
// OpNpcUHandler.ts:72-75.
//
// On success: set p.lastUseItem=useObj, p.lastUseSlot=useSlot →
// ClearPendingAction → SetInteraction(Engine, npc, targetOpNpcU, -1).
func handleOpNpcU(p *Player, payload []byte) error {
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

	if slot < 0 || slot >= len(s.npcs) {
		sendUnsetMapFlag(p)
		return nil
	}
	npc := s.npcs[slot]
	if npc == nil || npc.dead {
		sendUnsetMapFlag(p)
		return nil
	}
	if npc.delayed && s.currentTick < npc.delayedUntil {
		sendUnsetMapFlag(p)
		return nil
	}
	if !s.rsbuf.HasNpc(int32(p.slot), int32(npc.nid)) {
		sendUnsetMapFlag(p)
		return nil
	}
	if npc.typ == nil {
		sendUnsetMapFlag(p)
		return nil
	}

	// S6o-D3 closed: verify the player has an inv listener at useCom
	// and that the claimed item lives at the claimed slot (TS
	// OpNpcUHandler.ts:35-50).
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

	// S6o-D4 closed in S6z: reject members-only items on
	// free-to-play worlds. Matches TS OpNpcUHandler.ts:72-75.
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
	p.opcalled = true
	p.SetInteraction(InteractionEngine, npc, targetOpNpcU, -1)
	return nil
}
