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

// EncodeNpcLegacy is the NAI-29-and-earlier interface-based NpcInfo encoder.
// Retained during NAI-30 Bundle 3 only as a transition fallback while the
// new (ni *NpcInfo).Encode method (receiver *NpcInfo, taking *Buf as its
// first parameter) is being landed and validated. Callers swap to the new
// method in NAI-30 Bundle 4 Task 4.3; this function deletes in B4 Task 4.6.
func EncodeNpcLegacy(self PlayerSource, all []NpcSource, ba *buildarea.BuildArea, g *grid.Grid, r *Renderer) []byte {
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

// NpcInfo holds reusable scratch buffers for NpcInfo encoding.
// One instance per *Buf; reset and reused across all per-tick Encode
// calls (one per player). Mirrors upstream NpcInfo struct at
// info.rs:411-419 — the rsbuf-internal singleton (`NPC_INFO:
// Lazy<Mutex<NpcInfo>>` at lib.rs:37) collected onto the *Buf instance.
type NpcInfo struct {
	buf     *packet.Packet
	updates *packet.Packet
}

// NewNpcInfo allocates fresh scratch buffers sized for typical
// NpcInfo packets (~5000 bytes upstream Packet::new(5000)).
// Mirrors NpcInfo::new at info.rs:421-428.
func NewNpcInfo() *NpcInfo {
	return &NpcInfo{
		buf:     packet.NewPacket(make([]byte, 0, 5000)),
		updates: packet.NewPacket(make([]byte, 0, 5000)),
	}
}

// Bit-budget constants for fits() arithmetic. Mirror upstream
// NpcInfo::BITS_* at info.rs:413-417.
const (
	npcBitsAdd      = 13 + 11 + 5 + 5 + 1 // 35
	npcBitsRun      = 1 + 2 + 3 + 3 + 1   // 10
	npcBitsWalk     = 1 + 2 + 3 + 1       // 7
	npcBitsExtend   = 1 + 2               // 3
	npcTerminator   = 8191
	maxNpcInfoBytes = 4997
)

// Encode produces the NpcInfo payload for `pid` as a fresh []byte
// (no opcode/length prefix; caller wraps with OpNpcInfo).
// Mirrors upstream NpcInfo::encode at info.rs:430-464.
//
// Signature divergences from upstream: same shape as PlayerInfo.Encode
// (see playerinfo.go:299-308) — the players/grid/zoneMap/renderer args
// upstream collapse into `b *Buf` + `renderer *Renderer`.
//
// T3.2 SKELETON: writeNpcs emits a single 8-bit zero-count and
// writeNewNpcs is a no-op. T3.3 expands writeNpcs with the tracked-
// delta loop; T3.4 expands writeNewNpcs with discovery + observers
// + 8191 terminator.
//
// Returns nil if pid is out of range or slot is unpopulated.
func (ni *NpcInfo) Encode(b *Buf, pid int32, renderer *Renderer) []byte {
	if pid < 0 || int(pid) >= len(b.players) {
		return nil
	}
	self := b.players[pid]
	if self == nil {
		return nil
	}

	ni.buf.Reset()
	ni.updates.Reset()

	ni.buf.AccessBits()

	ni.writeNpcs(b, self, renderer)
	ni.writeNewNpcs(b, self, renderer)

	ni.buf.AccessBytes()

	// NpcInfo has no separate "before-updates" sentinel (unlike PlayerInfo's
	// 11-bit 2047). The 13-bit 8191 terminator emits inside writeNewNpcs on
	// byte-budget overflow only — see EncodeNpcLegacy lines 100-101 for the
	// reference pattern. T3.4 lands that emit site.
	for _, b2 := range ni.updates.Data {
		ni.buf.P1(b2)
	}

	// Return a copy — caller may write more, and ni.buf.Data is reused
	// next call. Mirrors PlayerInfo.Encode tail (playerinfo.go:350-352).
	out := make([]byte, len(ni.buf.Data))
	copy(out, ni.buf.Data)
	return out
}

// writeNpcs emits the per-tracked-NPC delta loop. T3.2 SKELETON:
// emits an 8-bit zero count and returns. T3.3 expands with the
// 5-remove-condition / 4-mode-branch tracked loop. Mirrors upstream
// NpcInfo::write_npcs at info.rs:466-509.
func (ni *NpcInfo) writeNpcs(b *Buf, self *Player, renderer *Renderer) {
	ni.buf.PBit(8, 0)
}

// writeNewNpcs discovers nearby NPCs and emits add-leaves until the
// byte budget or preferredNpcs cap is hit. T3.2 SKELETON: no-op.
// T3.4 expands with discovery + observers increment + 8191 terminator.
// Mirrors upstream NpcInfo::write_new_npcs at info.rs:511-585.
func (ni *NpcInfo) writeNewNpcs(b *Buf, self *Player, renderer *Renderer) {
}
