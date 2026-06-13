package rsbuf

import (
	"github.com/zsrv/goscape/pkg/coordgrid"
	"github.com/zsrv/goscape/pkg/io/packet"
)

const (
	NpcViewDistanceZones = 15
	PreferredNpcs        = 255
	NpcTerminator        = 16383
)

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
// NpcInfo's BITS_ADD/RUN/WALK/EXTEND (info.rs:413,418-420) — each names the
// full leaf size so fits() accounts for the bits about to be written when it
// gates the high-def 'extend' block. (npcBitsAdd counts the rev-274 jump bit
// as an exact leaf size; upstream's BITS_ADD is a fits() budget heuristic that
// upstream left at 36 — goscape keeps the exact 37 per the project's
// constant-names-the-leaf convention, sibling to playerBitsAdd=23.)
// The Run/Walk/Extend siblings were retired at NAI-30 B4 T4.6 (mirroring the
// PlayerInfo retirement) and reintroduced at rsbuf-npc-1, where writeNpcs
// gained the per-tracked-NPC byte-budget gate (sibling of rsbuf-player-1's
// playerBitsRun/Walk/Extend).
const (
	npcBitsAdd      = 14 + 11 + 5 + 5 + 1 + 1 // 37 (rev-274: +jump bit, crate 66911610; 254: nid widened 13→14 bits, crate 304955d5)
	npcBitsRun      = 1 + 2 + 3 + 3 + 1       // 10
	npcBitsWalk     = 1 + 2 + 3 + 1           // 7
	npcBitsExtend   = 1 + 2                   // 3
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

	// Mirrors info.rs:456-462: emit the 14-bit 16383 terminator before
	// AccessBytes when there are pending mask-payload updates, then append
	// updates after byte alignment. Without the terminator, the Java
	// client's getNpcPosNewVis (Client-Java client.java:5787-5821) reads
	// bits past the new-NPCs section into the mask-payload bytes (no
	// bit-budget exit at first-tick counts) and crashes parsing garbage.
	// PlayerInfo's analogous pattern at playerinfo.go:89-97 uses 11-bit 2047.
	if len(ni.updates.Data) > 0 {
		ni.buf.PBit(14, NpcTerminator)
		ni.buf.AccessBytes()
		for _, b2 := range ni.updates.Data {
			ni.buf.P1(b2)
		}
	} else {
		ni.buf.AccessBytes()
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
//     legacy encoder behavior carried over (NpcViewDistanceZones).
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
			// rsbuf-npc-1: emit the extend bit only when the high-def block
			// is non-empty AND it still fits the byte budget, mirroring Rust
			// write_npcs `len>0 && self.fits(...)` (info.rs:484). The movement
			// leaf is always emitted; only the high-def block is conditional.
			// Sibling of writePlayers' run arm (playerinfo.go).
			extend := 0
			if hdLen > 0 && ni.fits(npcBitsRun, hdLen) {
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
			if hdLen > 0 && ni.fits(npcBitsWalk, hdLen) {
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
		case hdLen > 0 && ni.fits(npcBitsExtend, hdLen):
			// Idle-with-extend arm: emit high-def only when it fits the
			// budget (info.rs:487 `else if len>0 && self.fits(...)`).
			ni.buf.PBit(1, 1)
			ni.buf.PBit(2, 0)
			for _, b2 := range highDef {
				ni.updates.P1(b2)
			}
		default:
			// idle: no movement, and either no high-def or it no longer fits
			// the budget (info.rs:489 `else { self.idle(); }`).
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
// On byte-budget overflow, emits the 14-bit NpcTerminator (16383)
// sentinel and returns — distinct from PlayerInfo's pre-AccessBytes
// 11-bit 2047 sentinel which fires at Encode level. NpcInfo's 16383
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
			ni.buf.PBit(14, NpcTerminator)
			return
		}

		otherPos := coordgrid.UnpackCoord(other.Coord)
		dx := clampInt(otherPos.X-selfPos.X, -15, 15)
		dz := clampInt(otherPos.Z-selfPos.Z, -15, 15)
		jump := 0
		if other.Jump {
			jump = 1
		}

		ni.buf.PBit(14, int(nid))
		ni.buf.PBit(11, int(other.NType))
		ni.buf.PBit(5, dx&0x1f)
		ni.buf.PBit(5, dz&0x1f)
		// rev-274 (crate 66911610 info.rs:251, TS rsbuf/info.ts:365 @dee467c8):
		// jump bit between dz and the extend bit; mirrors the player add-leaf.
		ni.buf.PBit(1, jump)
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
