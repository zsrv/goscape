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
