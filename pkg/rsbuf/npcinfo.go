package rsbuf

import (
	"github.com/zsrv/goscape/pkg/buildarea"
	"github.com/zsrv/goscape/pkg/grid"
	"github.com/zsrv/goscape/pkg/io/packet"
)

const (
	NpcViewDistanceZones = 15
	PreferredNpcs        = 255
	NpcTerminator        = 8191
)

// EncodeNpc produces the NpcInfo payload for `self` (no opcode/length prefix).
func EncodeNpc(self PlayerSource, all []NpcSource, ba *buildarea.BuildArea, g *grid.Grid, r *Renderer) []byte {
	byNid := make(map[int]NpcSource, len(all))
	for _, n := range all {
		byNid[n.Nid()] = n
	}

	main := packet.NewPacket(nil)
	updates := packet.NewPacket(nil)

	main.AccessBits()

	// Phase 1: tracked-npcs delta loop.
	main.PBit(8, len(ba.Npcs))
	slots := make([]int, 0, len(ba.Npcs))
	for nid := range ba.Npcs {
		slots = append(slots, nid)
	}
	selfX, selfZ, selfLevel := self.Coords()
	for _, nid := range slots {
		n, ok := byNid[nid]
		if !ok || !n.Active() {
			main.PBit(1, 1)
			main.PBit(2, 3) // remove
			decNpcObserver(nid)
			delete(ba.Npcs, nid)
			continue
		}
		nx, nz, nl := n.Coords()
		if nl != selfLevel || zoneDist(selfX, selfZ, nx, nz) > NpcViewDistanceZones {
			main.PBit(1, 1)
			main.PBit(2, 3)
			decNpcObserver(nid)
			delete(ba.Npcs, nid)
			continue
		}
		extend := 0
		payload := r.NpcHighDefOf(nid)
		if len(payload) > 0 && fits(main, updates, len(payload)) {
			extend = 1
		}
		switch {
		case n.RunDir() != -1:
			main.PBit(1, 1)
			main.PBit(2, 2)
			main.PBit(3, n.WalkDir())
			main.PBit(3, n.RunDir())
			main.PBit(1, extend)
		case n.WalkDir() != -1:
			main.PBit(1, 1)
			main.PBit(2, 1)
			main.PBit(3, n.WalkDir())
			main.PBit(1, extend)
		case n.Masks() != 0:
			main.PBit(1, 1)
			main.PBit(2, 0)
			extend = 1
		default:
			main.PBit(1, 0)
		}
		if extend == 1 && len(payload) > 0 {
			for _, b := range payload {
				updates.P1(b)
			}
		}
	}

	// Phase 2: new-npcs loop.
	candidates := g.NearbyNpcs(selfX, selfZ, selfLevel, NpcViewDistanceZones)
	for _, nid := range candidates {
		if _, already := ba.Npcs[nid]; already {
			continue
		}
		if len(ba.Npcs) >= PreferredNpcs {
			break
		}
		n, ok := byNid[nid]
		if !ok || !n.Active() {
			continue
		}
		payload := r.NpcLowDefOf(nid)
		if !fits(main, updates, len(payload)+5) { // ~5 bytes for the 35-bit add header
			main.PBit(13, NpcTerminator)
			break
		}
		nx, nz, _ := n.Coords()
		dx := clamp(nx-selfX, -15, 15)
		dz := clamp(nz-selfZ, -15, 15)

		main.PBit(13, nid)
		main.PBit(11, n.TypeID())
		main.PBit(5, dx&0x1f)
		main.PBit(5, dz&0x1f)
		main.PBit(1, boolToInt(len(payload) > 0))

		ba.Npcs[nid] = struct{}{}
		incNpcObserver(nid)
		if len(payload) > 0 {
			for _, b := range payload {
				updates.P1(b)
			}
		}
	}

	main.AccessBytes()
	for _, b := range updates.Data {
		main.P1(b)
	}
	return main.Data
}
