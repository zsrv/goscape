package rsbuf

import (
	"bytes"
	"testing"

	"github.com/zsrv/goscape/pkg/coordgrid"
)

// setupLocalPlayer constructs a player at (3200, 0, 3200) with all sentinels.
// The modify callback receives the populated *Player and may override fields.
// 42-arg ComputePlayer call verified against (*Buf).ComputePlayer at pkg/rsbuf/buf.go:153-175.
func setupLocalPlayer(b *Buf, pid int32, modify func(p *Player)) {
	b.AddPlayer(pid)
	b.ComputePlayer(
		pid,
		3200, 0, 3200,
		3200, 3200,
		false, false,
		-1, -1,
		VisibilityDefault,
		0,
		true,
		0,
		nil,
		-1,
		-1,
		-1, -1,
		-1, -1,
		-1, -1,
		-1, -1,
		-1, -1,
		nil,
		nil, 0, 0, 0,
		-1, -1, -1,
		-1, -1,
		-1, -1,
		-1, -1, -1,
	)
	if modify != nil {
		modify(b.players[pid])
	}
}

func TestPlayerInfo_LocalPlayer_Walk(t *testing.T) {
	b := New()
	setupLocalPlayer(b, 1, func(p *Player) {
		p.WalkDir = 4 // arbitrary walk direction 0-7
	})
	pi := NewPlayerInfo()
	r := NewRenderer()
	out := pi.Encode(b, 1, r)

	// Walk-leaf: PBit(1,1) PBit(2,1) PBit(3,4) PBit(1,0) PBit(8,0)
	//   = 1 01 100 0 00000000 = 0xb0 0x00 (15 bits → 2 bytes).
	if len(out) != 2 {
		t.Errorf("walk: got %d bytes, want 2; bytes: %x", len(out), out)
	}
	if out[0] != 0xb0 {
		t.Errorf("walk leading byte: got 0x%02x, want 0xb0", out[0])
	}
}

func TestPlayerInfo_LocalPlayer_Run(t *testing.T) {
	b := New()
	setupLocalPlayer(b, 1, func(p *Player) {
		p.RunDir = 3
		p.WalkDir = 5
	})
	pi := NewPlayerInfo()
	r := NewRenderer()
	out := pi.Encode(b, 1, r)

	// Run-leaf: PBit(1,1) PBit(2,2) PBit(3,5) PBit(3,3) PBit(1,0) + PBit(8,0)
	//   = 1 10 101 011 0 00000000 = 0xd5 0x80 (18 bits → 3 bytes).
	if len(out) != 3 {
		t.Errorf("run: got %d bytes, want 3", len(out))
	}
	if out[0] != 0xd5 {
		t.Errorf("run byte[0]: got 0x%02x, want 0xd5", out[0])
	}
}

func TestPlayerInfo_LocalPlayer_Tele(t *testing.T) {
	b := New()
	setupLocalPlayer(b, 1, func(p *Player) {
		p.Tele = true
	})
	pi := NewPlayerInfo()
	r := NewRenderer()
	out := pi.Encode(b, 1, r)

	// Tele-leaf: PBit(1,1) PBit(2,3) PBit(2,level=0) PBit(7,localX=48) PBit(7,localZ=48) PBit(1,jump=0) PBit(1,extend=0)
	// = 1+2+2+7+7+1+1 = 21 bits + 8 count = 29 bits → 4 bytes.
	// localX = x - (((originX>>3) - 6) << 3) = 3200 - (((3200>>3) - 6) << 3) = 3200 - ((400 - 6)<<3) = 3200 - 3152 = 48
	// localZ = same logic = 48
	if len(out) != 4 {
		t.Errorf("tele: got %d bytes, want 4", len(out))
	}
	// Detailed byte-level pins: bit-stream math traces to upstream info.rs:79-89.
	// We assert length-only here; T2.9 round-trip parity test will catch byte-level
	// divergence vs EncodeLegacy.
}

// TestPlayerInfo_LocalPlayer_Idle pins the writeLocalPlayer default branch
// after the dispatch is wired in. T2.2's TestPlayerInfo_Encode_LocalIdleNoOthers
// covers the same path against the stub; this regression-locks the
// post-writeLocalPlayer behavior. Real extend-only branch coverage (the
// `case hdLen > 0:` arm) defers to T2.6, where mask-payload pinning makes
// seeded high-def state natural; reproducing it in T2.3 would force a
// renderer-internals reach-around (`r.highDef[1] = []byte{...}`).
func TestPlayerInfo_LocalPlayer_Idle(t *testing.T) {
	b := New()
	setupLocalPlayer(b, 1, func(p *Player) {
		// No movement; no masks. Renderer returns empty payload. Idle path taken.
	})
	pi := NewPlayerInfo()
	r := NewRenderer()
	out := pi.Encode(b, 1, r)

	// Idle: PBit(1,0) + PBit(8,0) = 9 bits = 2 bytes, both zero.
	if len(out) != 2 || out[0] != 0 || out[1] != 0 {
		t.Errorf("idle: got %x, want 00 00", out)
	}
}

func setupOtherPlayer(b *Buf, pid int32, modify func(p *Player)) {
	b.AddPlayer(pid)
	b.ComputePlayer(
		pid,
		3200, 0, 3200,
		3200, 3200,
		false, false,
		-1, -1,
		VisibilityDefault,
		0,
		true,
		0,
		nil,
		-1,
		-1,
		-1, -1,
		-1, -1,
		-1, -1,
		-1, -1,
		-1, -1,
		nil,
		nil, 0, 0, 0,
		-1, -1, -1,
		-1, -1,
		-1, -1,
		-1, -1, -1,
	)
	if modify != nil {
		modify(b.players[pid])
	}
}

func TestPlayerInfo_TrackedOther_Idle(t *testing.T) {
	b := New()
	setupLocalPlayer(b, 1, nil)
	setupOtherPlayer(b, 2, nil)
	b.players[1].Build.Players.Insert(2)

	pi := NewPlayerInfo()
	r := NewRenderer()
	out := pi.Encode(b, 1, r)

	// Local idle (1 bit) + count=1 (8 bits) + other-idle (1 bit) = 10 bits → 2 bytes.
	// Bytes: 0_0000000 1_0XXXXXX = 0x00, 0x80
	if len(out) != 2 {
		t.Errorf("tracked idle: got %d bytes, want 2", len(out))
	}
	if out[0] != 0x00 || out[1] != 0x80 {
		t.Errorf("tracked idle bytes: got %x, want 00 80", out)
	}
}

func TestPlayerInfo_TrackedOther_RemoveBecauseSlotEmpty(t *testing.T) {
	b := New()
	setupLocalPlayer(b, 1, nil)
	// Mark slot 2 as observed but NEVER add it (slot stays nil).
	b.players[1].Build.Players.Insert(2)

	pi := NewPlayerInfo()
	r := NewRenderer()
	out := pi.Encode(b, 1, r)

	// remove leaf: PBit(1,1) PBit(2,3) = 3 bits.
	// Total: 1 + 8 + 3 = 12 bits → 2 bytes.
	// 0_0000000 1_111_XXXX = 0x00, 0xf0
	if out[0] != 0x00 || out[1] != 0xf0 {
		t.Errorf("remove (slot empty): got %x, want 00 f0", out)
	}
	if b.players[1].Build.Players.Contains(2) {
		t.Error("slot 2 should be removed from build.Players after remove")
	}
}

func TestPlayerInfo_TrackedOther_RemoveBecauseTele(t *testing.T) {
	b := New()
	setupLocalPlayer(b, 1, nil)
	setupOtherPlayer(b, 2, func(p *Player) { p.Tele = true })
	b.players[1].Build.Players.Insert(2)

	pi := NewPlayerInfo()
	r := NewRenderer()
	pi.Encode(b, 1, r)

	// writePlayers removes pid 2 (Tele=true triggers remove-leaf) but
	// writeNewPlayers immediately re-discovers and re-adds it since the
	// player is still within view distance and active. This is the correct
	// upstream behavior (info.rs:136-166 does not filter on tele).
	// The net state after a full Encode: pid 2 IS in the build set.
	if !b.players[1].Build.Players.Contains(2) {
		t.Error("slot 2 should be re-added by writeNewPlayers after tele-remove")
	}
}

func TestPlayerInfo_TrackedOther_RemoveBecauseLevelMismatch(t *testing.T) {
	b := New()
	setupLocalPlayer(b, 1, nil) // level 0
	setupOtherPlayer(b, 2, func(p *Player) {
		p.Coord = coordgrid.PackCoord(1, 3200, 3200) // level 1
	})
	b.players[1].Build.Players.Insert(2)

	pi := NewPlayerInfo()
	r := NewRenderer()
	out := pi.Encode(b, 1, r)
	if len(out) < 2 {
		t.Fatalf("remove (level mismatch): got %d bytes, want >= 2; bytes: %x", len(out), out)
	}
	if out[0] != 0x00 || out[1] != 0xf0 {
		t.Errorf("remove (level mismatch): got %x, want 00 f0", out)
	}
}

func TestPlayerInfo_TrackedOther_RemoveBecauseOutOfDistance(t *testing.T) {
	b := New()
	setupLocalPlayer(b, 1, nil)
	setupOtherPlayer(b, 2, func(p *Player) {
		p.Coord = coordgrid.PackCoord(0, 5000, 5000) // far away
	})
	b.players[1].Build.Players.Insert(2)

	pi := NewPlayerInfo()
	r := NewRenderer()
	out := pi.Encode(b, 1, r)
	if len(out) < 2 {
		t.Fatalf("remove (out of distance): got %d bytes, want >= 2; bytes: %x", len(out), out)
	}
	if out[0] != 0x00 || out[1] != 0xf0 {
		t.Errorf("remove (out of distance): got %x, want 00 f0", out)
	}
}

func TestPlayerInfo_TrackedOther_RemoveBecauseInactive(t *testing.T) {
	b := New()
	setupLocalPlayer(b, 1, nil)
	setupOtherPlayer(b, 2, func(p *Player) { p.Active = false })
	b.players[1].Build.Players.Insert(2)

	pi := NewPlayerInfo()
	r := NewRenderer()
	out := pi.Encode(b, 1, r)
	if len(out) < 2 {
		t.Fatalf("remove (inactive): got %d bytes, want >= 2; bytes: %x", len(out), out)
	}
	if out[0] != 0x00 || out[1] != 0xf0 {
		t.Errorf("remove (inactive): got %x, want 00 f0", out)
	}
}

func TestPlayerInfo_TrackedOther_RemoveBecauseHardVisibility(t *testing.T) {
	b := New()
	setupLocalPlayer(b, 1, nil)
	setupOtherPlayer(b, 2, func(p *Player) { p.Visibility = VisibilityHard })
	b.players[1].Build.Players.Insert(2)

	pi := NewPlayerInfo()
	r := NewRenderer()
	out := pi.Encode(b, 1, r)
	if len(out) < 2 {
		t.Fatalf("remove (hard visibility): got %d bytes, want >= 2; bytes: %x", len(out), out)
	}
	if out[0] != 0x00 || out[1] != 0xf0 {
		t.Errorf("remove (hard visibility): got %x, want 00 f0", out)
	}
}

func TestPlayerInfo_TrackedOther_Walk(t *testing.T) {
	b := New()
	setupLocalPlayer(b, 1, nil)
	setupOtherPlayer(b, 2, func(p *Player) { p.WalkDir = 3 })
	b.players[1].Build.Players.Insert(2)

	pi := NewPlayerInfo()
	r := NewRenderer()
	out := pi.Encode(b, 1, r)

	// Bit-stream: PBit(1,0) PBit(8,1) PBit(1,1) PBit(2,1) PBit(3,3) PBit(1,0)
	//   = 0_00000001_1_01_011_0 = 16 bits → 2 bytes.
	// byte 0 = b0..b7 = 00000000 = 0x00
	// byte 1 = b8..b15 = 11010110 = 0xd6
	if len(out) != 2 {
		t.Fatalf("tracked-walk: got %d bytes, want 2; bytes: %x", len(out), out)
	}
	if out[0] != 0x00 || out[1] != 0xd6 {
		t.Errorf("tracked-walk: got %x, want 00 d6", out)
	}
}

func TestPlayerInfo_TrackedOther_Run(t *testing.T) {
	b := New()
	setupLocalPlayer(b, 1, nil)
	setupOtherPlayer(b, 2, func(p *Player) {
		p.WalkDir = 5
		p.RunDir = 3
	})
	b.players[1].Build.Players.Insert(2)

	pi := NewPlayerInfo()
	r := NewRenderer()
	out := pi.Encode(b, 1, r)

	// Bit-stream: PBit(1,0) PBit(8,1) PBit(1,1) PBit(2,2) PBit(3,5) PBit(3,3) PBit(1,0)
	//   = 0_00000001_1_10_101_011_0 = 19 bits → 3 bytes (after AccessBytes round-up).
	// byte 0 = 0x00; byte 1 = 11101010 = 0xea; byte 2 = 11000000 = 0xc0
	if len(out) != 3 {
		t.Fatalf("tracked-run: got %d bytes, want 3; bytes: %x", len(out), out)
	}
	if out[0] != 0x00 || out[1] != 0xea || out[2] != 0xc0 {
		t.Errorf("tracked-run: got %x, want 00 ea c0", out)
	}
}

// TestPlayerInfo_TrackedOther_Extend pins the per-other extend-only branch
// (`case hdLen > 0:` inside writePlayers). Seeds renderer high-def directly
// to avoid coupling the test to ComputePlayers; T2.6 will exercise this
// branch end-to-end via real mask payloads.
func TestPlayerInfo_TrackedOther_Extend(t *testing.T) {
	b := New()
	setupLocalPlayer(b, 1, nil)
	setupOtherPlayer(b, 2, nil) // WalkDir=-1, RunDir=-1, so `case hdLen > 0:` arm fires
	b.players[1].Build.Players.Insert(2)

	pi := NewPlayerInfo()
	r := NewRenderer()
	r.highDef[2] = []byte{0xab} // 1 byte; renderer-internals reach-around for branch isolation.

	out := pi.Encode(b, 1, r)

	// Bit-stream: PBit(1,0) PBit(8,1) PBit(1,1) PBit(2,0) [updates non-empty]
	//   PBit(11,2047) AccessBytes → P1(0xab)
	// = 0_00000001_1_00_11111111111 = 23 bits + 1-byte append = 4 bytes total.
	// byte 0 = 0x00; byte 1 = 11001111 = 0xcf; byte 2 = 11111110 = 0xfe; byte 3 = 0xab
	if len(out) != 4 {
		t.Fatalf("tracked-extend: got %d bytes, want 4; bytes: %x", len(out), out)
	}
	if out[0] != 0x00 || out[1] != 0xcf || out[2] != 0xfe || out[3] != 0xab {
		t.Errorf("tracked-extend: got %x, want 00 cf fe ab", out)
	}
}

func TestPlayerInfo_NewPlayers_DiscoversAndAdds(t *testing.T) {
	b := New()
	setupLocalPlayer(b, 1, nil)
	setupOtherPlayer(b, 2, func(p *Player) {
		// New player at adjacent zone — passes filterPlayer.
	})

	pi := NewPlayerInfo()
	r := NewRenderer()
	out := pi.Encode(b, 1, r)

	// Verify add happened: build set now contains pid 2.
	if !b.players[1].Build.Players.Contains(2) {
		t.Errorf("after Encode, build.Players should contain 2; bytes %x", out)
	}
}

func TestPlayerInfo_NewPlayers_RespectsPreferredCap(t *testing.T) {
	b := New()
	setupLocalPlayer(b, 1, nil)
	// Set up preferredPlayers (250) real active players so writePlayers
	// keeps them in the set rather than removing them (nil slots get removed).
	for i := int32(2); i < int32(2+preferredPlayers); i++ {
		setupOtherPlayer(b, i, nil)
		b.players[1].Build.Players.Insert(i)
	}
	// Add one more nearby player that would otherwise discover.
	setupOtherPlayer(b, 1000, nil)

	pi := NewPlayerInfo()
	r := NewRenderer()
	out := pi.Encode(b, 1, r)
	_ = out

	// Pid 1000 should NOT be added (cap blocks).
	if b.players[1].Build.Players.Contains(1000) {
		t.Error("preferred cap exceeded; pid 1000 should not have been added")
	}
}

func TestPlayerInfo_NewPlayers_SkipsHardVisibility(t *testing.T) {
	b := New()
	setupLocalPlayer(b, 1, nil)
	setupOtherPlayer(b, 2, func(p *Player) { p.Visibility = VisibilityHard })

	pi := NewPlayerInfo()
	r := NewRenderer()
	_ = pi.Encode(b, 1, r)

	if b.players[1].Build.Players.Contains(2) {
		t.Error("HARD visibility excluded; pid 2 should not have been added")
	}
}

func TestPlayerInfo_LastAppearance_FreshGuardSkipsAppearance(t *testing.T) {
	b := New()
	setupLocalPlayer(b, 1, nil)
	setupOtherPlayer(b, 2, func(p *Player) {
		p.LastAppearance = -1 // never generated — encoder should skip APPEARANCE
	})

	pi := NewPlayerInfo()
	r := NewRenderer()
	_ = pi.Encode(b, 1, r)

	// pid 2 was added to build set, but no SaveAppearance call
	// should have triggered for non-zero tick. Negative pin.
	for tick := uint32(1); tick <= 100; tick++ {
		if b.players[1].Build.HasAppearance(2, tick) {
			t.Errorf("lastAppearance=-1: build should not have saved tick %d", tick)
		}
	}
}

func TestPlayerInfo_LastAppearance_BuildSavesOnFirstSend(t *testing.T) {
	b := New()
	setupLocalPlayer(b, 1, nil)
	setupOtherPlayer(b, 2, func(p *Player) {
		p.LastAppearance = 42
	})

	pi := NewPlayerInfo()
	r := NewRenderer()
	_ = pi.Encode(b, 1, r)

	// First send — build stores tick 42.
	if !b.players[1].Build.HasAppearance(2, 42) {
		t.Error("after first encode with lastAppearance=42, build should have saved tick 42")
	}

	// Second send same tick — no resend.
	_ = pi.Encode(b, 1, r)
	if !b.players[1].Build.HasAppearance(2, 42) {
		t.Error("after second encode same tick, build should still equal 42")
	}
}

func TestPlayerInfo_LastAppearance_BuildResendsOnTickChange(t *testing.T) {
	b := New()
	setupLocalPlayer(b, 1, nil)
	setupOtherPlayer(b, 2, func(p *Player) {
		p.LastAppearance = 42
	})

	pi := NewPlayerInfo()
	r := NewRenderer()
	_ = pi.Encode(b, 1, r) // saves tick 42

	// Bump lastAppearance — equipment changed.
	b.players[2].LastAppearance = 43

	// To re-trigger discovery, we need pid 2 NOT to be in the tracked set.
	// Remove and re-add.
	b.players[1].Build.Players.Remove(2)
	_ = pi.Encode(b, 1, r)
	if !b.players[1].Build.HasAppearance(2, 43) {
		t.Error("after re-discovery with lastAppearance=43, build should have saved tick 43")
	}
}

func TestPlayerInfo_Encode_LocalIdleNoOthers(t *testing.T) {
	b := New()
	pi := NewPlayerInfo()
	b.AddPlayer(1)
	// ComputePlayer with all sentinels — local stationary at (3200, 0, 3200), no masks,
	// no exact move. 42-arg signature; verify against (*Buf).ComputePlayer in pkg/rsbuf/buf.go.
	b.ComputePlayer(
		1,           // pid
		3200, 0, 3200, // x, level, z
		3200, 3200,    // originX, originZ
		false, false,  // tele, jump
		-1, -1,        // runDir, walkDir
		VisibilityDefault, // visibility
		0,                 // staffModLevel
		true,              // active
		0,                 // masks
		nil,               // appearance
		-1,                // lastAppearance
		-1,                // faceEntity
		-1, -1,            // faceX, faceZ
		-1, -1,            // orientationX, orientationZ
		-1, -1,            // damageTaken, damageType
		-1, -1,            // currentHitpoints, baseHitpoints
		-1, -1,            // animID, animDelay
		nil,               // say
		nil, 0, 0, 0,      // message, color, effect, ignored
		-1, -1, -1,        // graphicID, graphicHeight, graphicDelay
		-1, -1,            // exactStartX, exactStartZ
		-1, -1,            // exactEndX, exactEndZ
		-1, -1, -1,        // exactMoveStart, exactMoveEnd, exactMoveDirection
	)

	r := NewRenderer()
	out := pi.Encode(b, 1, r)

	// Local idle: 1 leading bit `0` (not-update flag), then 8-bit "0 other players tracked".
	// First byte: 0_0000000 (idle local, then top 7 bits of count) — verify the leading byte
	// is 0 in bit-MSB order. Total 2 bytes when no updates buffer follows.
	if len(out) < 1 {
		t.Fatalf("encode produced empty output")
	}
	if out[0] != 0 {
		t.Errorf("local-idle leading byte: got 0x%02x, want 0x00 (idle bit + count high)", out[0])
	}
	// No updates buffer payload appended (no extends, no appearance triggers).
	// Total length is bit-aligned to 9 bits (1 idle + 8 count) → ceil(9/8) = 2 bytes.
	if len(out) != 2 {
		t.Errorf("local-idle, no others: total bytes got %d, want 2", len(out))
	}
}

func TestPlayerInfo_TrackedOther_RemoveBecauseSoftVisAndLowStaff(t *testing.T) {
	b := New()
	setupLocalPlayer(b, 1, nil) // StaffModLevel default 0
	setupOtherPlayer(b, 2, func(p *Player) { p.Visibility = VisibilitySoft })
	b.players[1].Build.Players.Insert(2)

	pi := NewPlayerInfo()
	r := NewRenderer()
	out := pi.Encode(b, 1, r)
	if len(out) < 2 {
		t.Fatalf("remove (soft + low staff): got %d bytes, want >= 2; bytes %x", len(out), out)
	}
	if out[0] != 0x00 || out[1] != 0xf0 {
		t.Errorf("remove (soft + low staff): got %x, want 00 f0", out)
	}
}

func TestPlayerInfo_TrackedOther_KeepsSoftVisWithStaffMod(t *testing.T) {
	b := New()
	setupLocalPlayer(b, 1, func(p *Player) { p.StaffModLevel = 1 })
	setupOtherPlayer(b, 2, func(p *Player) { p.Visibility = VisibilitySoft })
	b.players[1].Build.Players.Insert(2)

	pi := NewPlayerInfo()
	r := NewRenderer()
	out := pi.Encode(b, 1, r)
	if len(out) < 2 {
		t.Fatalf("keeps (soft + staff>=1): got %d bytes, want >= 2; bytes %x", len(out), out)
	}
	// Should NOT be remove (0xf0 prefix on byte 1) — should be idle (0x80).
	if out[1] == 0xf0 {
		t.Errorf("remove triggered when staff mod >= 1: got %x", out)
	}
}

// TestPlayerInfo_LocalPlayer_ChatMaskStripped pins the upstream
// PlayerInfo::highdefinition at info.rs:289-291 behavior: local
// player's own CHAT mask bit is stripped from the high-def payload
// (no self-echo). Goscape's eager Renderer doesn't currently expose
// per-mask suppression, so this is deferred to NAI-31 when the
// renderer cache is ported. Tagged NAI-30-D2 (see writeLocalPlayer
// doc-comment in playerinfo.go).
func TestPlayerInfo_LocalPlayer_ChatMaskStripped(t *testing.T) {
	t.Skip("NAI-30-D2: requires NAI-31 renderer cache port for per-mask suppression")
}

// TestPlayerInfo_Encode_OutputBytesAreCopy regression-locks the
// make+copy pattern at the end of (pi *PlayerInfo).Encode (T2.2).
// Each Encode call returns an independent byte slice; the next
// Encode call mutates pi.buf.Data (truncates + rewrites) but must
// not corrupt the previously-returned slice.
func TestPlayerInfo_Encode_OutputBytesAreCopy(t *testing.T) {
	b := New()
	setupLocalPlayer(b, 1, nil)
	pi := NewPlayerInfo()
	r := NewRenderer()

	out1 := pi.Encode(b, 1, r)
	out1Saved := append([]byte(nil), out1...)

	// Mutate state and re-encode (different branch in writeLocalPlayer →
	// different bytes in pi.buf.Data).
	b.players[1].WalkDir = 3
	out2 := pi.Encode(b, 1, r)

	// out1 must be unchanged after the second Encode rewrote pi.buf.Data.
	if !bytes.Equal(out1, out1Saved) {
		t.Errorf("out1 mutated after second Encode: got %x, want %x", out1, out1Saved)
	}
	// out2 should differ from out1 (different bytes — sanity that we
	// actually exercised a re-encode rather than a no-op).
	if bytes.Equal(out1, out2) {
		t.Errorf("out2 identical to out1 (expected different branch); both = %x", out1)
	}
}
