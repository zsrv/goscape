package rsbuf

import (
	"github.com/zsrv/goscape/pkg/buildarea"
	"github.com/zsrv/goscape/pkg/coordgrid"
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

// writeNpcs emits the per-tracked-NPC delta loop. Mirrors upstream
// NpcInfo::write_npcs at info.rs:466-509. T3.3 replaces T3.2's
// PBit(8, 0) skeleton with the full 5-remove-condition + 4-mode-branch
// loop. Observer-decrement on remove mirrors info.rs:480.
//
// Deliberate divergences from PlayerInfo's writePlayers (see
// playerinfo.go:451-530 for comparison):
//   - View distance is the package constant preferredViewDistance,
//     not the per-player self.Build.ViewDistance. NPC view distance
//     isn't dynamically resized until NAI-32; this matches the
//     EncodeNpcLegacy behavior at npcinfo.go:48 (NpcViewDistanceZones).
//   - No visibility gate. NPCs have no Visibility field, so
//     PlayerInfo's HARD/SOFT-with-staff-mod rejects are absent by
//     design, not oversight.
//   - Bounds-check and nil-slot check are combined into a single
//     defensive branch (lines 219-222), where PlayerInfo splits them
//     into two branches (playerinfo.go:457-465). Both shapes are
//     equivalent; the combined form is slightly cleaner.
func (ni *NpcInfo) writeNpcs(b *Buf, self *Player, renderer *Renderer) {
	tracked := self.Build.Npcs.Iter()
	ni.buf.PBit(8, len(tracked))
	selfPos := coordgrid.UnpackCoord(self.Coord)
	for _, nid := range tracked {
		if int(nid) >= len(b.npcs) || b.npcs[nid] == nil {
			ni.removeNpc(self, nid)
			ni.decObservers(b, nid)
			continue
		}
		other := b.npcs[nid]
		otherPos := coordgrid.UnpackCoord(other.Coord)
		if other.NID == -1 || other.Tele || otherPos.Level != selfPos.Level ||
			!withinDistanceSW(selfPos.X, selfPos.Z, otherPos.X, otherPos.Z, int(preferredViewDistance)) ||
			!other.Active {
			ni.removeNpc(self, nid)
			ni.decObservers(b, nid)
			continue
		}
		highDef := renderer.NpcHighDefOf(int(nid))
		hdLen := len(highDef)
		switch {
		case other.RunDir != -1:
			extend := 0
			if hdLen > 0 {
				extend = 1
			}
			ni.buf.PBit(1, 1)
			ni.buf.PBit(2, 2)
			ni.buf.PBit(3, int(other.WalkDir))
			ni.buf.PBit(3, int(other.RunDir))
			ni.buf.PBit(1, extend)
			if extend == 1 {
				for _, b2 := range highDef {
					ni.updates.P1(b2)
				}
			}
		case other.WalkDir != -1:
			extend := 0
			if hdLen > 0 {
				extend = 1
			}
			ni.buf.PBit(1, 1)
			ni.buf.PBit(2, 1)
			ni.buf.PBit(3, int(other.WalkDir))
			ni.buf.PBit(1, extend)
			if extend == 1 {
				for _, b2 := range highDef {
					ni.updates.P1(b2)
				}
			}
		case hdLen > 0:
			ni.buf.PBit(1, 1)
			ni.buf.PBit(2, 0)
			for _, b2 := range highDef {
				ni.updates.P1(b2)
			}
		default:
			ni.buf.PBit(1, 0)
		}
	}
}

// removeNpc emits the 3-bit remove leaf (PBit(1,1)+PBit(2,3) = "1 11")
// and removes nid from self's tracking set. Mirrors info.rs:478-480.
func (ni *NpcInfo) removeNpc(self *Player, nid int32) {
	ni.buf.PBit(1, 1)
	ni.buf.PBit(2, 3)
	self.Build.Npcs.Remove(nid)
}

// decObservers decrements b.npcs[nid].Observers, flooring at 0.
// Mirrors info.rs:480 `other.observers = (other.observers - 1).max(0)`.
func (ni *NpcInfo) decObservers(b *Buf, nid int32) {
	if int(nid) >= len(b.npcs) || b.npcs[nid] == nil {
		return
	}
	if b.npcs[nid].Observers > 0 {
		b.npcs[nid].Observers--
	}
}

// writeNewNpcs discovers nearby NPCs and emits add-leaves until the
// byte budget or preferredNpcs cap is hit. Mirrors upstream
// NpcInfo::write_new_npcs at info.rs:511-585. T3.4 replaces T3.2's
// no-op skeleton with the full discovery loop. Each successful add
// increments b.npcs[nid].Observers (mirrors info.rs:540).
//
// On byte-budget overflow, emits the 13-bit npcTerminator (8191)
// sentinel and returns — distinct from PlayerInfo's pre-AccessBytes
// 11-bit 2047 sentinel which fires at Encode level. NpcInfo's 8191
// terminator is purely a per-loop byte-budget cutoff signal.
func (ni *NpcInfo) writeNewNpcs(b *Buf, self *Player, renderer *Renderer) {
	selfPos := coordgrid.UnpackCoord(self.Coord)
	candidates := self.Build.GetNearbyNpcs(&b.npcs, b.zoneMap, selfPos.X, selfPos.Level, selfPos.Z)

	for _, nid := range candidates {
		if self.Build.Npcs.Contains(nid) {
			continue
		}
		if self.Build.Npcs.Len() >= int(preferredNpcs) {
			return
		}
		other := b.npcs[nid]
		if other == nil || !other.Active {
			continue
		}

		lowDef := renderer.NpcLowDefOf(int(nid))
		if !ni.fits(npcBitsAdd, len(lowDef)) {
			// Byte budget overflow — emit terminator and return.
			ni.buf.PBit(13, npcTerminator)
			return
		}

		otherPos := coordgrid.UnpackCoord(other.Coord)
		dx := clampInt(otherPos.X-selfPos.X, -15, 15)
		dz := clampInt(otherPos.Z-selfPos.Z, -15, 15)

		ni.buf.PBit(13, int(nid))
		ni.buf.PBit(11, int(other.NType))
		ni.buf.PBit(5, dx&0x1f)
		ni.buf.PBit(5, dz&0x1f)
		ni.buf.PBit(1, 1) // extend always set for add

		self.Build.Npcs.Insert(nid)
		other.Observers++

		for _, b2 := range lowDef {
			ni.updates.P1(b2)
		}
	}
}

// fits reports whether adding bitsToAdd + bytesToAdd will fit within
// maxNpcInfoBytes. Mirrors info.rs:577 (analogous to PlayerInfo's
// fits at playerinfo.go:604-608, identical formula).
func (ni *NpcInfo) fits(bitsToAdd, bytesToAdd int) bool {
	totalBits := ni.buf.BitPos + bitsToAdd + 7
	totalBytes := (totalBits >> 3) + len(ni.updates.Data) + bytesToAdd
	return totalBytes <= maxNpcInfoBytes
}
