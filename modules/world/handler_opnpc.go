package world

import (
	"github.com/zsrv/goscape/pkg/io/packet"
)

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

	// NpcType.Op[op-1] must be a non-empty, non-"hidden" string.
	// RuneScript will later replace this with trigger-existence lookup.
	if npc.typ == nil || len(npc.typ.Op) < op ||
		npc.typ.Op[op-1] == "" || npc.typ.Op[op-1] == "hidden" {
		sendUnsetMapFlag(p)
		return nil
	}

	p.ClearPendingAction()
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
// Validation gates (mirrors TS OpNpcTHandler.ts):
//  1. delayed player → UnsetMapFlag
//  2. payload too short → UnsetMapFlag
//  3. slot out of range → UnsetMapFlag
//  4. NPC nil or dead → UnsetMapFlag
//  5. NpcType nil → UnsetMapFlag
//
// DEVIATION S6o-D1: TS validates spellCom references a component
// with ComActionTarget.NPC flag AND that the component is visible in
// the player's interface stack. Skipped here because goscape has no
// component registry yet. Effective risk: client can forge spellCom
// values; scripts reading p.TargetSubjectCom() get raw wire values.
// Follow-up: "component registry + ComActionTarget validation"
// sub-spec (bundle with S6m-D1).
//
// Unlike handleOpNpc (handler_opnpc.go:40-44), there is NO per-op
// validation gate — T/U variants don't index into NpcType.Op.
//
// No targetSubject.{typ,x,z,level} snapshot — NPCs have no in-place
// mutation risk (unlike Loc's packed Info bitfield). npc.dead is the
// lifecycle gate, checked at fire time (fireApTriggerNpc/fireOpTriggerNpc).
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

	if slot < 0 || slot >= len(s.npcs) {
		sendUnsetMapFlag(p)
		return nil
	}
	npc := s.npcs[slot]
	if npc == nil || npc.dead {
		sendUnsetMapFlag(p)
		return nil
	}
	if npc.typ == nil {
		sendUnsetMapFlag(p)
		return nil
	}

	p.ClearPendingAction()
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
//
// DEVIATION S6o-D2: TS validates useCom references a usable, visible
// interface component. Skipped — no component registry. (Mirrors S6m-D2.)
//
// DEVIATION S6o-D3: TS does an inventory-listener lookup by useCom +
// slot-bounds + item-at-slot-matches-useObj validation. Goscape's
// invListeners is a slice not keyed map, so this lookup shape doesn't
// translate. Skip; scripts reading p.LastUseItem()/p.LastUseSlot() get
// raw wire values. (Mirrors S6m-D3.)
//
// DEVIATION S6o-D4: TS checks members-only items against NODE_MEMBERS
// config. Skipped — no members-config surface. (Mirrors S6m-D4.)
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
	_ = int(r.G2()) // useCom — deliberately discarded (S6o-D2/D3)

	if slot < 0 || slot >= len(s.npcs) {
		sendUnsetMapFlag(p)
		return nil
	}
	npc := s.npcs[slot]
	if npc == nil || npc.dead {
		sendUnsetMapFlag(p)
		return nil
	}
	if npc.typ == nil {
		sendUnsetMapFlag(p)
		return nil
	}

	p.lastUseItem = useObj
	p.lastUseSlot = useSlot

	p.ClearPendingAction()
	p.SetInteraction(InteractionEngine, npc, targetOpNpcU, -1)
	return nil
}
