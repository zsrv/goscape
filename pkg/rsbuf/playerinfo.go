package rsbuf

import (
	"github.com/zsrv/goscape/pkg/coordgrid"
	"github.com/zsrv/goscape/pkg/io/packet"
)

const (
	PreferredPlayers  = 255
	MaxPacketBytes    = 4997
	ViewDistanceZones = 15
)

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

// Bit-budget constant for fits() arithmetic. Mirrors upstream
// PlayerInfo::BITS_ADD at info.rs:19. The Run/Walk/Extend siblings
// upstream are unused by goscape's encoder shape (the per-other delta
// loop measures via len(buf.Data) directly) and were retired at
// NAI-30 B4 T4.6.
const (
	playerBitsAdd = 11 + 5 + 5 + 1 + 1 // 23

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

	// Local player section (info.rs:72-100).
	pi.writeLocalPlayer(self, renderer)

	// Tracked-others delta loop (info.rs:102-134).
	pi.writePlayers(b, self, renderer)

	pi.writeNewPlayers(b, self, renderer)

	// Mirrors info.rs:62-68: append updates buffer if non-empty,
	// preceded by the 11-bit `2047` sentinel. NB: detect "non-empty"
	// via `len(pi.updates.Data) > 0`, not `pi.updates.Pos`. In the
	// project's packet.Packet shape (pkg/io/packet/buffer.go:20),
	// Pos is the READ pointer; writes append to len(Data).
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

// writeLocalPlayer emits the local player's per-tick movement bits
// (or idle/extend), branching on tele/run/walk/masks. Mirrors upstream
// PlayerInfo::write_local_player at info.rs:72-100.
//
// Returns the high-def payload length for the local player (consumed
// by the new-players byte-budget math at info.rs:60).
//
// NAI-30-D2 (deferred to NAI-31): upstream PlayerInfo::highdefinition
// at info.rs:289-291 strips the CHAT mask bit for self (no chat
// self-echo). Goscape's existing eager Renderer doesn't expose
// per-mask suppression, so the local player's own chat may echo back
// to its own client by one chat block per say. Fix lands when NAI-31
// ports the renderer cache and adds suppress-chat-for-self plumbing.
// Test pinned via TestPlayerInfo_LocalPlayer_ChatMaskStripped (t.Skip).
func (pi *PlayerInfo) writeLocalPlayer(self *Player, renderer *Renderer) int {
	pos := coordgrid.UnpackCoord(self.Coord)
	originPos := coordgrid.UnpackCoord(self.Origin)
	highDef := renderer.HighDefOf(int(self.PID))
	hdLen := len(highDef)

	switch {
	case self.Tele:
		// Mirrors info.rs:80-89: teleport leaf with local-window coords.
		localX := pos.X - (((originPos.X >> 3) - 6) << 3)
		localZ := pos.Z - (((originPos.Z >> 3) - 6) << 3)
		jump := 0
		if self.Jump {
			jump = 1
		}
		extend := 0
		if hdLen > 0 {
			extend = 1
		}
		pi.buf.PBit(1, 1)
		pi.buf.PBit(2, 3)
		// pos.Level (player.coord.y in upstream) — NOT originPos.Level.
		// info.rs:84. Tele test fixture has both equal to 0, so a swap
		// would not be caught by the test alone; T2.9 round-trip parity
		// against the legacy encoder (retired at NAI-30 B4 T4.6) locked
		// the choice.
		pi.buf.PBit(2, pos.Level)
		pi.buf.PBit(7, localX)
		pi.buf.PBit(7, localZ)
		pi.buf.PBit(1, jump)
		pi.buf.PBit(1, extend)
		if extend == 1 {
			for _, b := range highDef {
				pi.updates.P1(b)
			}
		}
	case self.RunDir != -1:
		// Mirrors info.rs:91 + run() at info.rs:226-243.
		extend := 0
		if hdLen > 0 {
			extend = 1
		}
		pi.buf.PBit(1, 1)
		pi.buf.PBit(2, 2)
		pi.buf.PBit(3, int(self.WalkDir))
		pi.buf.PBit(3, int(self.RunDir))
		pi.buf.PBit(1, extend)
		if extend == 1 {
			for _, b := range highDef {
				pi.updates.P1(b)
			}
		}
	case self.WalkDir != -1:
		// Mirrors info.rs:93 + walk() at info.rs:246-262.
		extend := 0
		if hdLen > 0 {
			extend = 1
		}
		pi.buf.PBit(1, 1)
		pi.buf.PBit(2, 1)
		pi.buf.PBit(3, int(self.WalkDir))
		pi.buf.PBit(1, extend)
		if extend == 1 {
			for _, b := range highDef {
				pi.updates.P1(b)
			}
		}
	case hdLen > 0:
		// Mirrors info.rs:94-95 + extend() at info.rs:265-274.
		pi.buf.PBit(1, 1)
		pi.buf.PBit(2, 0)
		for _, b := range highDef {
			pi.updates.P1(b)
		}
	default:
		// idle (info.rs:97).
		pi.buf.PBit(1, 0)
	}
	return hdLen
}

// writePlayers emits the per-tracked-other delta loop. Mirrors upstream
// PlayerInfo::write_players at info.rs:102-134.
func (pi *PlayerInfo) writePlayers(b *Buf, self *Player, renderer *Renderer) {
	tracked := self.Build.Players.Iter()
	pi.buf.PBit(8, len(tracked))

	selfPos := coordgrid.UnpackCoord(self.Coord)
	for _, otherPid := range tracked {
		if int(otherPid) >= len(b.players) {
			pi.removeOther(self, otherPid)
			continue
		}
		other := b.players[otherPid]
		if other == nil {
			pi.removeOther(self, otherPid)
			continue
		}

		otherPos := coordgrid.UnpackCoord(other.Coord)
		// Six remove conditions (mirrors info.rs:114). A 7th goscape-specific
		// condition (Visibility==VisibilitySoft && self.StaffModLevel<1)
		// preserves NAI-9 visibility behavior carried over from the legacy
		// encoder (retired at NAI-30 B4 T4.6); absent from upstream Rust by
		// design. T2.9 round-trip parity validates the union remains correct.
		if other.PID == -1 ||
			other.Tele ||
			otherPos.Level != selfPos.Level ||
			!withinDistanceSW(selfPos.X, selfPos.Z, otherPos.X, otherPos.Z, int(self.Build.ViewDistance)) ||
			!other.Active ||
			other.Visibility == VisibilityHard {
			pi.removeOther(self, otherPid)
			continue
		}
		// 7th reject: SOFT visibility + insufficient staff mod level
		// (matches goscape's NAI-9 behavior; upstream info.rs only checks HARD).
		if other.Visibility == VisibilitySoft && self.StaffModLevel < 1 {
			pi.removeOther(self, otherPid)
			continue
		}

		highDef := renderer.HighDefOf(int(otherPid))
		hdLen := len(highDef)
		switch {
		case other.RunDir != -1:
			extend := 0
			if hdLen > 0 {
				extend = 1
			}
			pi.buf.PBit(1, 1)
			pi.buf.PBit(2, 2)
			pi.buf.PBit(3, int(other.WalkDir))
			pi.buf.PBit(3, int(other.RunDir))
			pi.buf.PBit(1, extend)
			if extend == 1 {
				for _, b2 := range highDef {
					pi.updates.P1(b2)
				}
			}
		case other.WalkDir != -1:
			extend := 0
			if hdLen > 0 {
				extend = 1
			}
			pi.buf.PBit(1, 1)
			pi.buf.PBit(2, 1)
			pi.buf.PBit(3, int(other.WalkDir))
			pi.buf.PBit(1, extend)
			if extend == 1 {
				for _, b2 := range highDef {
					pi.updates.P1(b2)
				}
			}
		case hdLen > 0:
			pi.buf.PBit(1, 1)
			pi.buf.PBit(2, 0)
			for _, b2 := range highDef {
				pi.updates.P1(b2)
			}
		default:
			pi.buf.PBit(1, 0)
		}
	}
}

// removeOther emits the 3-bit remove leaf and updates the build set.
// Mirrors PlayerInfo::remove at info.rs:189-197.
func (pi *PlayerInfo) removeOther(self *Player, otherPid int32) {
	pi.buf.PBit(1, 1)
	pi.buf.PBit(2, 3)
	self.Build.Players.Remove(otherPid)
}

// writeNewPlayers discovers nearby players and emits add-leaves until
// the byte budget or preferredPlayers cap is hit. Mirrors upstream
// PlayerInfo::write_new_players at info.rs:136-166.
func (pi *PlayerInfo) writeNewPlayers(b *Buf, self *Player, renderer *Renderer) {
	selfPos := coordgrid.UnpackCoord(self.Coord)
	candidates := self.Build.GetNearbyPlayers(&b.players, b.zoneMap, self.PID, selfPos.X, selfPos.Level, selfPos.Z)

	for _, otherPid := range candidates {
		if self.Build.Players.Contains(otherPid) {
			continue
		}
		if self.Build.Players.Len() >= int(preferredPlayers) {
			return
		}
		other := b.players[otherPid]
		if other == nil || other.Visibility == VisibilityHard {
			continue
		}
		if other.Visibility == VisibilitySoft && self.StaffModLevel < 1 {
			continue
		}

		// Byte budget: BITS_ADD + low-def payload size.
		lowDef := renderer.LowDefFullOf(int(otherPid))
		if !pi.fits(playerBitsAdd, len(lowDef)) {
			return
		}

		otherPos := coordgrid.UnpackCoord(other.Coord)
		dx := clampInt(otherPos.X-selfPos.X, -15, 15)
		dz := clampInt(otherPos.Z-selfPos.Z, -15, 15)
		jump := 0
		if other.Jump {
			jump = 1
		}

		pi.buf.PBit(11, int(otherPid))
		pi.buf.PBit(5, dx&0x1f)
		pi.buf.PBit(5, dz&0x1f)
		pi.buf.PBit(1, jump)
		pi.buf.PBit(1, 1) // extend bit always set for add

		self.Build.Players.Insert(otherPid)

		// Choose low-def variant per appearance dedup.
		// Mirrors info.rs:296-310: if other.lastAppearance != -1 AND
		// build's stored tick != lastAppearance, send LowDefFullOf
		// (includes APPEARANCE block) and save tick.
		if other.LastAppearance != -1 && !self.Build.HasAppearance(otherPid, uint32(other.LastAppearance)) {
			self.Build.SaveAppearance(otherPid, uint32(other.LastAppearance))
			for _, b2 := range lowDef {
				pi.updates.P1(b2)
			}
		} else {
			noApp := renderer.LowDefNoAppOf(int(otherPid))
			for _, b2 := range noApp {
				pi.updates.P1(b2)
			}
		}
	}
}

// fits reports whether adding bitsToAdd + bytesToAdd will fit within
// maxPlayerInfoBytes. Mirrors info.rs:404-408.
func (pi *PlayerInfo) fits(bitsToAdd, bytesToAdd int) bool {
	totalBits := pi.buf.BitPos + bitsToAdd + 7
	totalBytes := (totalBits >> 3) + len(pi.updates.Data) + bytesToAdd
	return totalBytes <= maxPlayerInfoBytes
}

// clampInt clamps v to [lo, hi]. Canonical helper for the (pi *PlayerInfo)
// and (ni *NpcInfo) method blocks. Replaced the legacy free clamp function
// at NAI-30 B4 T4.6 (rename, not delete + add) when EncodeLegacy retired.
func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
