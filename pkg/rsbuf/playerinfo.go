package rsbuf

import (
	"github.com/zsrv/goscape/pkg/buildarea"
	"github.com/zsrv/goscape/pkg/grid"
	"github.com/zsrv/goscape/pkg/io/packet"
)

const (
	PreferredPlayers  = 255
	MaxPacketBytes    = 4997
	ViewDistanceZones = 15
)

// EncodeLegacy is the NAI-29-and-earlier interface-based encoder.
// Retained during NAI-30 Bundle 2/3 only as a transition fallback
// while the new (pi *PlayerInfo).Encode method (receiver *PlayerInfo,
// taking *Buf as its first parameter) is being landed and validated.
// Callers swap to the new method in NAI-30 Bundle 4 Task 4.2; this
// function deletes in B4 Task 4.6.
func EncodeLegacy(self PlayerSource, all []PlayerSource, ba *buildarea.BuildArea, g *grid.Grid, r *Renderer) []byte {
	bySlot := make(map[int]PlayerSource, len(all))
	for _, p := range all {
		bySlot[p.Slot()] = p
	}

	main := packet.NewPacket(nil)
	updates := packet.NewPacket(nil)

	main.AccessBits()
	writeLocalPlayer(main, updates, self, r)
	writeOtherPlayers(main, updates, self, bySlot, ba, r)
	writeNewPlayers(main, updates, self, bySlot, ba, g, r)

	if len(updates.Data) > 0 {
		main.PBit(11, 2047) // sentinel before mask-updates section
	}
	main.AccessBytes()

	// Append the mask-updates buffer.
	for _, b := range updates.Data {
		main.P1(b)
	}
	return main.Data
}

func writeLocalPlayer(main, updates *packet.Packet, self PlayerSource, r *Renderer) {
	x, z, level := self.Coords()
	masks := self.Masks()
	extend := 0
	payload := r.HighDefOf(self.Slot())
	if len(payload) > 0 && fits(main, updates, len(payload)) {
		extend = 1
	}

	switch {
	case self.Tele():
		originX := self.OriginX()
		originZ := self.OriginZ()
		localX := x - (((originX >> 3) - 6) << 3)
		localZ := z - (((originZ >> 3) - 6) << 3)
		main.PBit(1, 1)
		main.PBit(2, 3)
		main.PBit(2, level)
		main.PBit(7, localX)
		main.PBit(7, localZ)
		main.PBit(1, boolToInt(self.Jump()))
		main.PBit(1, extend)
	case self.RunDir() != -1:
		main.PBit(1, 1)
		main.PBit(2, 2)
		main.PBit(3, self.WalkDir())
		main.PBit(3, self.RunDir())
		main.PBit(1, extend)
	case self.WalkDir() != -1:
		main.PBit(1, 1)
		main.PBit(2, 1)
		main.PBit(3, self.WalkDir())
		main.PBit(1, extend)
	case masks != 0:
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

func writeOtherPlayers(main, updates *packet.Packet, self PlayerSource, bySlot map[int]PlayerSource, ba *buildarea.BuildArea, r *Renderer) {
	main.PBit(8, len(ba.Players))

	// Iterate a snapshot so we can delete during loop.
	slots := make([]int, 0, len(ba.Players))
	for slot := range ba.Players {
		slots = append(slots, slot)
	}

	for _, slot := range slots {
		other, ok := bySlot[slot]
		selfX, selfZ, selfLevel := self.Coords()

		if !ok || !other.Active() || other.Tele() {
			main.PBit(1, 1)
			main.PBit(2, 3)
			delete(ba.Players, slot)
			continue
		}
		ox, oz, ol := other.Coords()
		if ol != selfLevel || zoneDist(selfX, selfZ, ox, oz) > ViewDistanceZones {
			main.PBit(1, 1)
			main.PBit(2, 3)
			delete(ba.Players, slot)
			continue
		}
		if other.Visibility() == VisibilityHard {
			main.PBit(1, 1)
			main.PBit(2, 3)
			delete(ba.Players, slot)
			continue
		}
		if other.Visibility() == VisibilitySoft && self.StaffModLevel() < 1 {
			main.PBit(1, 1)
			main.PBit(2, 3)
			delete(ba.Players, slot)
			continue
		}

		extend := 0
		payload := r.HighDefOf(slot)
		if len(payload) > 0 && fits(main, updates, len(payload)) {
			extend = 1
		}
		switch {
		case other.RunDir() != -1:
			main.PBit(1, 1)
			main.PBit(2, 2)
			main.PBit(3, other.WalkDir())
			main.PBit(3, other.RunDir())
			main.PBit(1, extend)
		case other.WalkDir() != -1:
			main.PBit(1, 1)
			main.PBit(2, 1)
			main.PBit(3, other.WalkDir())
			main.PBit(1, extend)
		case other.Masks() != 0:
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
}

func writeNewPlayers(main, updates *packet.Packet, self PlayerSource, bySlot map[int]PlayerSource, ba *buildarea.BuildArea, g *grid.Grid, r *Renderer) {
	selfX, selfZ, selfLevel := self.Coords()
	candidates := g.NearbyPlayers(selfX, selfZ, selfLevel, ViewDistanceZones)

	for _, slot := range candidates {
		if slot == self.Slot() {
			continue
		}
		if _, already := ba.Players[slot]; already {
			continue
		}
		if len(ba.Players) >= PreferredPlayers {
			break
		}
		other, ok := bySlot[slot]
		if !ok || !other.Active() || other.Visibility() == VisibilityHard {
			continue
		}
		if other.Visibility() == VisibilitySoft && self.StaffModLevel() < 1 {
			continue
		}

		ox, oz, _ := other.Coords()
		dx := clamp(ox-selfX, -15, 15)
		dz := clamp(oz-selfZ, -15, 15)

		main.PBit(11, slot)
		main.PBit(5, dx&0x1f)
		main.PBit(5, dz&0x1f)
		main.PBit(1, boolToInt(other.Jump()))
		main.PBit(1, 1)

		ba.Players[slot] = struct{}{}

		hash := other.AppearanceHash()
		if ba.HasAppearance(slot, hash) {
			if payload := r.LowDefNoAppOf(slot); len(payload) > 0 {
				for _, b := range payload {
					updates.P1(b)
				}
			}
		} else {
			if payload := r.LowDefFullOf(slot); len(payload) > 0 {
				for _, b := range payload {
					updates.P1(b)
				}
			}
			ba.RecordAppearance(slot, hash)
		}
	}
}

// fits reports whether adding nBytes of mask updates keeps the packet within budget.
// In bit mode, main's written byte count is len(main.Data) (possibly with a
// partially-filled byte tracked internally via BitPos; treating len(Data) as
// the best-available approximation is fine for the budget check).
func fits(main, updates *packet.Packet, nBytes int) bool {
	total := len(main.Data) + len(updates.Data) + nBytes
	return total <= MaxPacketBytes
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func zoneDist(x1, z1, x2, z2 int) int {
	dx := abs((x1 >> 3) - (x2 >> 3))
	dz := abs((z1 >> 3) - (z2 >> 3))
	if dx > dz {
		return dx
	}
	return dz
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// PlayerInfo holds reusable scratch buffers for PlayerInfo encoding.
// One instance per *Buf; reset and reused across all per-tick Encode
// calls (one per player). Mirrors upstream PlayerInfo struct at
// info.rs:13-16 — the rsbuf-internal singleton (`PLAYER_INFO:
// Lazy<Mutex<PlayerInfo>>` at lib.rs:36) collected onto the *Buf
// instance.
type PlayerInfo struct {
	buf     *packet.Packet
	updates *packet.Packet
}

// NewPlayerInfo allocates fresh scratch buffers sized for typical
// PlayerInfo packets (~5000 bytes upstream Packet::new(5000)).
// Mirrors PlayerInfo::new at info.rs:24-30.
func NewPlayerInfo() *PlayerInfo {
	return &PlayerInfo{
		buf:     packet.NewPacket(make([]byte, 0, 5000)),
		updates: packet.NewPacket(make([]byte, 0, 5000)),
	}
}

// Bit-budget constants for fits() arithmetic. Mirror upstream
// PlayerInfo::BITS_* at info.rs:19-22.
const (
	playerBitsAdd    = 11 + 5 + 5 + 1 + 1 // 23
	playerBitsRun    = 1 + 2 + 3 + 3 + 1  // 10
	playerBitsWalk   = 1 + 2 + 3 + 1      // 7
	playerBitsExtend = 1 + 2              // 3

	// Per-packet byte budget. Mirrors upstream literal at info.rs:407.
	maxPlayerInfoBytes = 4997
)

// Encode produces the PlayerInfo payload for `pid` as a fresh []byte
// (no opcode/length prefix; caller wraps with OpPlayerInfo).
// Mirrors upstream PlayerInfo::encode at info.rs:32-70.
//
// Signature divergences from upstream:
//   - `pos` upstream param dropped: NAI-30 always starts at byte 0
//     (each Encode call wraps standalone).
//   - `dx`, `dz`, `rebuild` upstream params dropped: NAI-30 doesn't
//     run BuildArea.rebuild_players (view-distance resize is NAI-32).
//   - `players: &[Option<Player>]`, `grid: &HashMap<...>`,
//     `map: &mut ZoneMap` collapse into `b *Buf`.
//   - `player: &mut Player` collapses into `b.players[pid]`.
//
// Returns nil if pid is out of range or slot is unpopulated.
func (pi *PlayerInfo) Encode(b *Buf, pid int32, renderer *Renderer) []byte {
	if pid < 0 || int(pid) >= len(b.players) {
		return nil
	}
	self := b.players[pid]
	if self == nil {
		return nil
	}

	// Reset scratch buffers (mirrors info.rs:53-56 zeroing).
	pi.buf.Reset()
	pi.updates.Reset()

	pi.buf.AccessBits()

	// Bundle 2 Task 2.3 will fill writeLocalPlayer here.
	// For Task 2.2 skeleton, write idle bit only.
	pi.buf.PBit(1, 0) // idle

	// Bundle 2 Task 2.4 will fill writePlayers here.
	// For T2.2 skeleton, emit zero-count.
	pi.buf.PBit(8, 0)

	// Bundle 2 Task 2.5 will fill writeNewPlayers.

	// Mirrors info.rs:62-68: append updates buffer if non-empty,
	// preceded by the 11-bit `2047` sentinel. NB: detect "non-empty"
	// via `len(pi.updates.Data) > 0`, not `pi.updates.Pos`. In the
	// project's packet.Packet shape (pkg/io/packet/buffer.go:20),
	// Pos is the READ pointer; writes append to len(Data). Mirrors
	// the EncodeLegacy pattern at playerinfo.go:31,37-39.
	if len(pi.updates.Data) > 0 {
		pi.buf.PBit(11, 2047)
		pi.buf.AccessBytes()
		for _, b2 := range pi.updates.Data {
			pi.buf.P1(b2)
		}
	} else {
		pi.buf.AccessBytes()
	}

	// Return a copy — the caller may write more to its OpPlayerInfo
	// wrapper, and pi.buf.Data is reused next call.
	out := make([]byte, len(pi.buf.Data))
	copy(out, pi.buf.Data)
	return out
}
