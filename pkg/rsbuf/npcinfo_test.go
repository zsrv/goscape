package rsbuf

import (
	"bytes"
	"testing"
)

// setupNpc is a test helper that registers nid with ntype in b and calls
// ComputeNpc with sentinel defaults (same level=0, x=3200, z=3200 as the
// local player, active=true, RunDir=-1, WalkDir=-1, Masks=0). An optional
// modify callback may mutate the *Npc directly after ComputeNpc so that
// individual tests can set Tele, NID, Observers, etc.
func setupNpc(b *Buf, nid, ntype int32, modify func(n *Npc)) {
	b.AddNpc(nid, ntype)
	b.ComputeNpc(
		nid, ntype,
		3200, 0, 3200, // x, level, z — same zone as local player
		false,  // tele
		-1, -1, // runDir, walkDir
		true,       // active
		0,          // masks
		-1, -1, -1, // faceEntity, faceX, faceZ
		-1, -1, // orientationX, orientationZ
		-1, -1, // damageTaken, damageType
		-1, -1, // currentHitpoints, baseHitpoints
		-1, -1, // animID, animDelay
		nil,        // say
		-1, -1, -1, // graphicID, graphicHeight, graphicDelay
	)
	if modify != nil {
		modify(b.npcs[nid])
	}
}

func TestNpcInfo_Encode_Empty(t *testing.T) {
	b := New()
	ni := NewNpcInfo()
	b.AddPlayer(1)
	// Position self at (3200, 0, 3200) — no NPCs registered, so nearby/tracked
	// sets are both empty. 42-arg ComputePlayer signature (verify against
	// (*Buf).ComputePlayer in pkg/rsbuf/buf.go).
	b.ComputePlayer(
		1,             // pid
		3200, 0, 3200, // x, level, z
		3200, 3200, // originX, originZ
		false, false, // tele, jump
		-1, -1, // runDir, walkDir
		VisibilityDefault, // visibility
		0,                 // staffModLevel
		true,              // active
		0,                 // masks
		nil,               // appearance
		-1,                // lastAppearance
		-1,                // faceEntity
		-1, -1,            // faceX, faceZ
		-1, -1, // orientationX, orientationZ
		-1, -1, // damageTaken, damageType
		-1, -1, // currentHitpoints, baseHitpoints
		-1, -1, // animID, animDelay
		nil,          // say
		nil, 0, 0, 0, // message, color, effect, ignored
		-1, -1, -1, // graphicID, graphicHeight, graphicDelay
		-1, -1, // exactStartX, exactStartZ
		-1, -1, // exactEndX, exactEndZ
		-1, -1, -1, // exactMoveStart, exactMoveEnd, exactMoveDirection
	)

	r := NewRenderer()
	out := ni.Encode(b, 1, r)

	// Skeleton: writeNpcs emits PBit(8, 0) → 8 bits exactly → 1 byte after AccessBytes.
	// writeNewNpcs is a no-op; updates buffer empty; no terminator.
	if len(out) != 1 {
		t.Errorf("empty NpcInfo: got %d bytes, want 1; payload=% x", len(out), out)
	}
	if len(out) >= 1 && out[0] != 0 {
		t.Errorf("empty NpcInfo first byte: got 0x%02x, want 0x00 (8-bit zero count)", out[0])
	}
}

// ── Mode-branch tests (tracked NPC survives the reject gauntlet) ─────────────

// TestNpcInfo_TrackedNpc_Idle pins the idle branch: NPC at same level,
// in-distance, !Tele, Active, NID>=0; RunDir=-1, WalkDir=-1, Masks=0.
// writeNpcs emits: PBit(8,1) + PBit(1,0) = 9 bits → 2 bytes: 0x01 0x00.
func TestNpcInfo_TrackedNpc_Idle(t *testing.T) {
	b := New()
	setupLocalPlayer(b, 1, nil)
	setupNpc(b, 7, 100, nil)
	b.players[1].Build.Npcs.Insert(7)

	ni := NewNpcInfo()
	r := NewRenderer()
	out := ni.Encode(b, 1, r)

	// PBit(8,1)+PBit(1,0) = 9 bits → 2 bytes: 00000001 0_______
	// byte 0 = 0x01; byte 1 = 0x00
	if len(out) < 2 {
		t.Fatalf("tracked-idle: got %d bytes, want >= 2; bytes: % x", len(out), out)
	}
	if out[0] != 0x01 {
		t.Errorf("tracked-idle byte[0]: got 0x%02x, want 0x01 (count=1)", out[0])
	}
	if out[1] != 0x00 {
		t.Errorf("tracked-idle byte[1]: got 0x%02x, want 0x00 (idle bit=0)", out[1])
	}
}

// TestNpcInfo_TrackedNpc_Walk pins the walk branch: WalkDir=2, RunDir=-1,
// no high-def payload.
// writeNpcs emits: PBit(8,1)+PBit(1,1)+PBit(2,1)+PBit(3,2)+PBit(1,0)
// = 15 bits → 2 bytes: 00000001 10101000 = 0x01 0xa8.
func TestNpcInfo_TrackedNpc_Walk(t *testing.T) {
	b := New()
	setupLocalPlayer(b, 1, nil)
	setupNpc(b, 7, 100, func(n *Npc) {
		n.WalkDir = 2
		// RunDir stays -1 (default from setupNpc)
	})
	b.players[1].Build.Npcs.Insert(7)

	ni := NewNpcInfo()
	r := NewRenderer()
	out := ni.Encode(b, 1, r)

	// PBit(8,1)+PBit(1,1)+PBit(2,1)+PBit(3,2)+PBit(1,0) = 15 bits → 2 bytes.
	// bits: 00000001 1 01 010 0 → 00000001 10101000 = 0x01 0xa8
	if len(out) < 2 {
		t.Fatalf("tracked-walk: got %d bytes, want >= 2; bytes: % x", len(out), out)
	}
	if out[0] != 0x01 || out[1] != 0xa8 {
		t.Errorf("tracked-walk: got % x, want 01 a8", out)
	}
	// NPC was NOT removed — still in tracking set.
	if !b.players[1].Build.Npcs.Contains(7) {
		t.Error("walk-mode NPC should remain in tracking set after Encode")
	}
}

// TestNpcInfo_TrackedNpc_Run pins the run branch: RunDir=4, WalkDir=2,
// no high-def payload.
// writeNpcs emits: PBit(8,1)+PBit(1,1)+PBit(2,2)+PBit(3,2)+PBit(3,4)+PBit(1,0)
// = 18 bits → 3 bytes: 0x01 0xca 0x00.
func TestNpcInfo_TrackedNpc_Run(t *testing.T) {
	b := New()
	setupLocalPlayer(b, 1, nil)
	setupNpc(b, 7, 100, func(n *Npc) {
		n.WalkDir = 2
		n.RunDir = 4
	})
	b.players[1].Build.Npcs.Insert(7)

	ni := NewNpcInfo()
	r := NewRenderer()
	out := ni.Encode(b, 1, r)

	// PBit(8,1)+PBit(1,1)+PBit(2,2)+PBit(3,2)+PBit(3,4)+PBit(1,0) = 18 bits → 3 bytes.
	// bits: 00000001 1 10 010 100 0 → 00000001 11001010 0_______
	// byte 0 = 0x01; byte 1 = 11001010 = 0xca; byte 2 = 0x00
	if len(out) < 3 {
		t.Fatalf("tracked-run: got %d bytes, want >= 3; bytes: % x", len(out), out)
	}
	if out[0] != 0x01 || out[1] != 0xca || out[2] != 0x00 {
		t.Errorf("tracked-run: got % x, want 01 ca 00", out)
	}
}

// TestNpcInfo_TrackedNpc_Extend pins the extend-only branch: RunDir=-1,
// WalkDir=-1, but renderer has non-empty high-def payload for this NPC.
// writeNpcs emits: PBit(8,1)+PBit(1,1)+PBit(2,0) = 11 bits, then
// (NAI-31 Bundle 3) Encode emits the 13-bit 8191 terminator because
// ni.updates.Data is non-empty, then after AccessBytes the update byte
// 0xab is appended → 4 bytes total.
func TestNpcInfo_TrackedNpc_Extend(t *testing.T) {
	b := New()
	setupLocalPlayer(b, 1, nil)
	setupNpc(b, 7, 100, nil)
	b.players[1].Build.Npcs.Insert(7)

	ni := NewNpcInfo()
	r := NewRenderer()
	// Seed high-def directly (renderer-internals reach-around, same pattern
	// as TestPlayerInfo_TrackedOther_Extend using r.highDef[2]).
	r.npcHighDef[7] = []byte{0xab}

	out := ni.Encode(b, 1, r)

	// Bit layout after NAI-31 Bundle 3:
	//   bits 0-7:   PBit(8,1)              → byte 0 = 0x01
	//   bits 8-10:  PBit(1,1)+PBit(2,0)    → "100"
	//   bits 11-23: PBit(13,8191)          → 13 ones (terminator)
	//   AccessBytes pads to bit 24 (byte 3 boundary)
	//   byte 3:     P1(0xab)               → 0xab
	// byte 1 = "100 11111" = 0x9F; byte 2 = 11111111 = 0xFF; byte 3 = 0xab.
	if len(out) != 4 {
		t.Fatalf("tracked-extend: got %d bytes, want 4; bytes: % x", len(out), out)
	}
	if out[0] != 0x01 || out[1] != 0x9F || out[2] != 0xFF || out[3] != 0xab {
		t.Errorf("tracked-extend: got % x, want 01 9f ff ab", out)
	}
}

// ── Remove-branch tests ────────────────────────────────────────────────────

// removeLeafInOutput returns true if the 3-bit "1 11" remove-leaf is present
// at the expected bit offset after PBit(8,1): bits 8,9,10 = "111".
// Bit 8 is the MSB of byte[1]; bits 9-10 are the next two bits of byte[1].
// So byte[1] high nibble should have bits `111_____` ≥ 0xe0.
func removeLeafInOutput(out []byte) bool {
	if len(out) < 2 {
		return false
	}
	// PBit(8,1) consumes byte 0 = 0x01.
	// The remove leaf "1 11" occupies bits [8..10] of byte[1]:
	// byte[1] & 0xe0 must be 0xe0.
	return out[0] == 0x01 && (out[1]&0xe0) == 0xe0
}

// TestNpcInfo_TrackedNpc_RemoveBecauseSlotEmpty: nid is in tracking set but
// b.npcs[nid] is nil (RemoveNpc was called). The bounds/nil-slot branch fires;
// decObservers no-ops (slot is nil). NPC is removed from tracking set.
func TestNpcInfo_TrackedNpc_RemoveBecauseSlotEmpty(t *testing.T) {
	b := New()
	setupLocalPlayer(b, 1, nil)
	// Insert nid into tracking set WITHOUT setting up an Npc slot.
	b.players[1].Build.Npcs.Insert(7)

	ni := NewNpcInfo()
	r := NewRenderer()
	out := ni.Encode(b, 1, r)

	// Remove-leaf must appear (no panic even with nil slot).
	if !removeLeafInOutput(out) {
		t.Errorf("slot-empty remove: expected remove-leaf in output % x", out)
	}
	// nid 7 must be gone from the tracking set.
	if b.players[1].Build.Npcs.Contains(7) {
		t.Error("slot-empty remove: nid 7 should be removed from tracking set after Encode")
	}
}

// TestNpcInfo_TrackedNpc_RemoveBecauseNidSentinel: b.npcs[nid].NID == -1.
// Observer counter must decrement from 5 to 4; nid removed from tracking set;
// remove-leaf emitted.
func TestNpcInfo_TrackedNpc_RemoveBecauseNidSentinel(t *testing.T) {
	b := New()
	setupLocalPlayer(b, 1, nil)
	setupNpc(b, 7, 100, func(n *Npc) {
		n.NID = -1 // sentinel
		n.Observers = 5
	})
	b.players[1].Build.Npcs.Insert(7)

	ni := NewNpcInfo()
	r := NewRenderer()
	out := ni.Encode(b, 1, r)

	if !removeLeafInOutput(out) {
		t.Errorf("NID=-1 remove: expected remove-leaf in output % x", out)
	}
	if b.players[1].Build.Npcs.Contains(7) {
		t.Error("NID=-1 remove: nid 7 should not be in tracking set after Encode")
	}
	if got := b.NpcForTest(7).Observers; got != 4 {
		t.Errorf("NID=-1 remove: Observers after remove: got %d, want 4", got)
	}
}

// TestNpcInfo_TrackedNpc_RemoveBecauseTele: NPC has Tele=true.
// writeNpcs removes the NPC (emits remove-leaf, decrements Observers 5→4).
// writeNewNpcs immediately re-discovers and re-adds it (filter_npc does not
// check Tele — mirrors Rust build.rs:324). End state: nid is back in
// tracking set, Observers returns to 5, remove-leaf still in output
// (before the add-leaf).
//
// This matches the Rust reference behavior (info.rs:478-480 removes;
// info.rs:511-528 re-adds in the same tick for a teleporting NPC that is
// still within range and active). The remove-leaf IS emitted (the
// remove-leaf clears the NPC from the client's tracked set so the
// add-leaf re-registers it with the new tile position).
func TestNpcInfo_TrackedNpc_RemoveBecauseTele(t *testing.T) {
	b := New()
	setupLocalPlayer(b, 1, nil)
	setupNpc(b, 7, 100, func(n *Npc) {
		n.Tele = true
		n.Observers = 5
	})
	b.players[1].Build.Npcs.Insert(7)

	ni := NewNpcInfo()
	r := NewRenderer()
	out := ni.Encode(b, 1, r)

	// The remove-leaf must appear in the output (writeNpcs emits it first).
	if !removeLeafInOutput(out) {
		t.Errorf("tele remove: expected remove-leaf in output % x", out)
	}
	// After writeNewNpcs re-adds the NPC, nid 7 is back in the tracking set.
	if !b.players[1].Build.Npcs.Contains(7) {
		t.Error("tele remove: nid 7 should be back in tracking set (re-added by writeNewNpcs)")
	}
	// Observers: decremented 5→4 in writeNpcs, incremented 4→5 in writeNewNpcs.
	if got := b.NpcForTest(7).Observers; got != 5 {
		t.Errorf("tele remove: Observers after remove+re-add: got %d, want 5", got)
	}
}

// TestNpcInfo_TrackedNpc_RemoveBecauseLevelMismatch: self at level=0, NPC
// at level=1. Level comparison fires remove.
func TestNpcInfo_TrackedNpc_RemoveBecauseLevelMismatch(t *testing.T) {
	b := New()
	setupLocalPlayer(b, 1, nil) // level 0
	b.AddNpc(7, 100)
	b.ComputeNpc(
		7, 100,
		3200, 1, 3200, // level=1 — mismatch with player level=0
		false, -1, -1, true, 0,
		-1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, nil, -1, -1, -1,
	)
	b.npcs[7].Observers = 5
	b.players[1].Build.Npcs.Insert(7)

	ni := NewNpcInfo()
	r := NewRenderer()
	out := ni.Encode(b, 1, r)

	if !removeLeafInOutput(out) {
		t.Errorf("level-mismatch remove: expected remove-leaf in output % x", out)
	}
	if b.players[1].Build.Npcs.Contains(7) {
		t.Error("level-mismatch remove: nid 7 should not be in tracking set after Encode")
	}
	if got := b.NpcForTest(7).Observers; got != 4 {
		t.Errorf("level-mismatch remove: Observers after remove: got %d, want 4", got)
	}
}

// TestNpcInfo_TrackedNpc_RemoveBecauseOutOfDistance: self at (3200, 0, 3200);
// NPC at (3200+16*8, 0, 3200). Chebyshev distance in tiles = 16*8 = 128 >
// preferredViewDistance(15). Out-of-distance branch fires.
func TestNpcInfo_TrackedNpc_RemoveBecauseOutOfDistance(t *testing.T) {
	b := New()
	setupLocalPlayer(b, 1, nil) // x=3200, z=3200, level=0
	npcX := 3200 + 16*8         // 128 tiles away — distance > 15
	b.AddNpc(7, 100)
	b.ComputeNpc(
		7, 100,
		npcX, 0, 3200,
		false, -1, -1, true, 0,
		-1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, nil, -1, -1, -1,
	)
	b.npcs[7].Observers = 5
	b.players[1].Build.Npcs.Insert(7)

	ni := NewNpcInfo()
	r := NewRenderer()
	out := ni.Encode(b, 1, r)

	if !removeLeafInOutput(out) {
		t.Errorf("out-of-distance remove: expected remove-leaf in output % x", out)
	}
	if b.players[1].Build.Npcs.Contains(7) {
		t.Error("out-of-distance remove: nid 7 should not be in tracking set after Encode")
	}
	if got := b.NpcForTest(7).Observers; got != 4 {
		t.Errorf("out-of-distance remove: Observers after remove: got %d, want 4", got)
	}
}

// TestNpcInfo_TrackedNpc_RemoveBecauseInactive: NPC has Active=false.
// Observer counter decrements; nid removed; remove-leaf emitted.
func TestNpcInfo_TrackedNpc_RemoveBecauseInactive(t *testing.T) {
	b := New()
	setupLocalPlayer(b, 1, nil)
	b.AddNpc(7, 100)
	b.ComputeNpc(
		7, 100,
		3200, 0, 3200,
		false, -1, -1,
		false, // active=false
		0,
		-1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, nil, -1, -1, -1,
	)
	b.npcs[7].Observers = 5
	b.players[1].Build.Npcs.Insert(7)

	ni := NewNpcInfo()
	r := NewRenderer()
	out := ni.Encode(b, 1, r)

	if !removeLeafInOutput(out) {
		t.Errorf("inactive remove: expected remove-leaf in output % x", out)
	}
	if b.players[1].Build.Npcs.Contains(7) {
		t.Error("inactive remove: nid 7 should not be in tracking set after Encode")
	}
	if got := b.NpcForTest(7).Observers; got != 4 {
		t.Errorf("inactive remove: Observers after remove: got %d, want 4", got)
	}
}

// ── writeNewNpcs tests ────────────────────────────────────────────────────────

// TestNpcInfo_NewNpcs_DiscoversAndAdds: 1 NPC near self, Build.Npcs empty
// initially. After Encode: nid is in tracking set, Observers incremented to 1,
// and the output bit stream contains the 35-bit add-leaf.
//
// Add-leaf bit layout: PBit(8,0) [count] + PBit(13,nid) + PBit(11,ntype) +
// PBit(5,dx&0x1f) + PBit(5,dz&0x1f) + PBit(1,1) [extend always 1 for new add].
// With nid=7, ntype=100, self=(3200,0,3200), npc=(3200,0,3200) → dx=0,dz=0:
//
//	bits 0-7:  0x00 (count=0)
//	bits 8-20: 0b0000000000111 = 7  → nid
//	bits 21-31: 0b00001100100 = 100 → ntype
//	bits 32-36: 0b00000 → dx=0
//	bits 37-41: 0b00000 → dz=0
//	bit 42:    1 → extend
//
// Total: 43 bits → 6 bytes (rounded up). First byte 0x00 (count=0).
func TestNpcInfo_NewNpcs_DiscoversAndAdds(t *testing.T) {
	b := New()
	setupLocalPlayer(b, 1, nil)
	setupNpc(b, 7, 100, nil)

	ni := NewNpcInfo()
	r := NewRenderer()
	out := ni.Encode(b, 1, r)

	// nid 7 must be in tracking set after Encode.
	if !b.PlayerForTest(1).Build.Npcs.Contains(7) {
		t.Error("DiscoversAndAdds: nid 7 should be in tracking set after Encode")
	}
	// Observer counter must have been incremented from 0 to 1.
	if got := b.NpcForTest(7).Observers; got != 1 {
		t.Errorf("DiscoversAndAdds: Observers after add: got %d, want 1", got)
	}
	// Output must be longer than the 1-byte empty-case (count byte only).
	if len(out) <= 1 {
		t.Errorf("DiscoversAndAdds: got %d bytes, want > 1 (add-leaf present); bytes: % x", len(out), out)
	}
	// First byte = PBit(8, 0) = 0x00 (tracked-count = 0).
	if out[0] != 0x00 {
		t.Errorf("DiscoversAndAdds: byte[0] = 0x%02x, want 0x00 (count=0)", out[0])
	}
}

// TestNpcInfo_NewNpcs_RespectsPreferredCap: 256 NPCs (nids 1-256) all at
// self's coord. After Encode, exactly 255 are in tracking (preferredNpcs cap).
//
// The cap mechanism is GetNearbyNpcs's internal
//
//	remaining := int(preferredNpcs) - count
//
// guard: when Build.Npcs starts empty (count=0), GetNearbyNpcs returns at
// most 255 candidates regardless of how many NPCs are registered in the zone.
// The 256th NPC is silently excluded by GetNearbyNpcs, not by writeNewNpcs's
// own Len() >= preferredNpcs check. The test asserts the observable end-state (255 added),
// not which internal check fired.
func TestNpcInfo_NewNpcs_RespectsPreferredCap(t *testing.T) {
	b := New()
	setupLocalPlayer(b, 1, nil)
	// Register 256 NPCs (nids 1-256), all at self's coord (3200, 0, 3200).
	for nid := int32(1); nid <= 256; nid++ {
		setupNpc(b, nid, 100, nil)
	}

	ni := NewNpcInfo()
	r := NewRenderer()
	ni.Encode(b, 1, r)

	got := b.PlayerForTest(1).Build.Npcs.Len()
	// GetNearbyNpcs caps at preferredNpcs (255) regardless of how many are
	// registered; writeNewNpcs can add at most what GetNearbyNpcs returns.
	if got != 255 {
		t.Errorf("RespectsPreferredCap: Build.Npcs.Len() = %d, want 255", got)
	}
}

// TestNpcInfo_NewNpcs_ByteBudgetOverflow_EmitsTerminator: 1 NPC near self
// with a huge npcLowDef payload (5000 bytes). fits(35, 5000) returns false
// because (8+35+7)/8 + 0 + 5000 = 5006 > maxNpcInfoBytes (4997). The loop
// must emit the 13-bit 8191 terminator and return without adding nid.
//
// Output layout after Encode:
//
//	byte 0:      0x00       — PBit(8,0), count=0 (no tracked NPCs)
//	bits 8-20:   8191 = 0x1FFF — 13-bit terminator
//	bit-pad to byte boundary (3 bits of zero)
//
// 8191 in 13 bits MSB-first: 1111111111111.
// Packed into bytes 1-2: bit8..bit15 = 11111111 = 0xFF,
//
//	bit16..bit20 = 11111 and 3 padding bits = 11111_000 = 0xF8.
func TestNpcInfo_NewNpcs_ByteBudgetOverflow_EmitsTerminator(t *testing.T) {
	const nid = int32(7)
	b := New()
	setupLocalPlayer(b, 1, nil)
	setupNpc(b, nid, 100, nil)

	ni := NewNpcInfo()
	r := NewRenderer()
	// Inject a huge low-def payload directly (same-package reach-around,
	// matching the T3.3 Extend test pattern with r.npcHighDef[7]).
	r.npcLowDef[nid] = make([]byte, 5000)

	out := ni.Encode(b, 1, r)

	// nid must NOT be in tracking set — terminator fired before insert.
	if b.PlayerForTest(1).Build.Npcs.Contains(nid) {
		t.Error("ByteBudgetOverflow: nid 7 should NOT be in tracking set (terminator fired)")
	}
	// Observers must NOT be incremented — terminator fired before increment.
	if got := b.NpcForTest(nid).Observers; got != 0 {
		t.Errorf("ByteBudgetOverflow: Observers = %d, want 0 (not incremented)", got)
	}
	// Output must be at least 3 bytes: byte[0]=0x00, byte[1]=0xFF, byte[2]=0xF8.
	if len(out) < 3 {
		t.Fatalf("ByteBudgetOverflow: got %d bytes, want >= 3; bytes: % x", len(out), out)
	}
	if out[0] != 0x00 {
		t.Errorf("ByteBudgetOverflow: byte[0] = 0x%02x, want 0x00", out[0])
	}
	if out[1] != 0xFF {
		t.Errorf("ByteBudgetOverflow: byte[1] = 0x%02x, want 0xFF (terminator bits 8-15)", out[1])
	}
	if out[2] != 0xF8 {
		t.Errorf("ByteBudgetOverflow: byte[2] = 0x%02x, want 0xF8 (terminator bits 16-20 + 3-bit pad)", out[2])
	}
}

// TestNpcInfo_ObserverCountFloorsAtZero: NPC with Observers=0, forced into a
// remove condition via level mismatch (NPC at level=1, player at level=0).
// Level mismatch both triggers remove in writeNpcs AND prevents re-discovery
// by filterNpc (which also rejects level mismatches). Verifies that
// decObservers floors at 0 and does NOT underflow to -1.
//
// Note: Tele=true alone cannot be used here — filter_npc does not check Tele
// (matches Rust build.rs:324), so a teleporting NPC that is still in range
// and active would be re-added by writeNewNpcs in the same tick.
func TestNpcInfo_ObserverCountFloorsAtZero(t *testing.T) {
	b := New()
	setupLocalPlayer(b, 1, nil) // player at level=0
	b.AddNpc(7, 100)
	b.ComputeNpc(
		7, 100,
		3200, 1, 3200, // level=1 — mismatch with player level=0; prevents re-add
		false, -1, -1, true, 0,
		-1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, nil, -1, -1, -1,
	)
	b.npcs[7].Observers = 0
	b.players[1].Build.Npcs.Insert(7)

	ni := NewNpcInfo()
	r := NewRenderer()
	ni.Encode(b, 1, r)

	// decObservers must floor at 0 — not decrement to -1.
	if got := b.NpcForTest(7).Observers; got != 0 {
		t.Errorf("ObserverCountFloorsAtZero: Observers = %d, want 0 (floored)", got)
	}
}

// TestNpcInfo_NewNpcs_SkipsAlreadyTracked: pre-insert nid into Build.Npcs
// BEFORE Encode. filterNpc (buildarea.go:218) excludes already-tracked
// candidates from GetNearbyNpcs, so the NPC never appears in the candidates
// slice. The in-loop Contains check in writeNewNpcs is a defensive fallback
// for any hypothetical duplicate.
//
// Asserts: no double-add (Observers stays 0), Build.Npcs still contains nid,
// and no add-leaf appears in the output (output is only the 1-byte count).
func TestNpcInfo_NewNpcs_SkipsAlreadyTracked(t *testing.T) {
	b := New()
	setupLocalPlayer(b, 1, nil)
	setupNpc(b, 7, 100, nil)
	// Pre-insert nid=7 into Build.Npcs BEFORE Encode.
	b.players[1].Build.Npcs.Insert(7)

	ni := NewNpcInfo()
	r := NewRenderer()
	out := ni.Encode(b, 1, r)

	// Build.Npcs must still contain nid=7 (not double-removed).
	if !b.PlayerForTest(1).Build.Npcs.Contains(7) {
		t.Error("SkipsAlreadyTracked: nid 7 should still be in tracking set after Encode")
	}
	// Observers must be 0 — no add (already tracked before Encode).
	if got := b.NpcForTest(7).Observers; got != 0 {
		t.Errorf("SkipsAlreadyTracked: Observers = %d, want 0 (no double-increment)", got)
	}
	// No add-leaf: output is just the tracked-delta loop result for 1 NPC.
	// PBit(8,1) + PBit(1,0) = 9 bits → 2 bytes (idle branch: no walk/run/extend).
	if len(out) != 2 {
		t.Errorf("SkipsAlreadyTracked: got %d bytes, want 2 (tracked idle, no add-leaf); bytes: % x", len(out), out)
	}
}

// TestNpcInfo_Encode_OutputBytesAreCopy regression-locks the
// make+copy pattern at the end of (ni *NpcInfo).Encode (T3.2).
// Each Encode call returns an independent byte slice; the next
// Encode call mutates ni.buf.Data (truncates + rewrites) but must
// not corrupt the previously-returned slice.
func TestNpcInfo_Encode_OutputBytesAreCopy(t *testing.T) {
	b := New()
	setupLocalPlayer(b, 1, nil)
	setupNpc(b, 7, 100, nil) // NPC near self at default coords
	ni := NewNpcInfo()
	r := NewRenderer()

	out1 := ni.Encode(b, 1, r)
	out1Saved := append([]byte(nil), out1...)

	// Mutate the NPC to force a different branch in the second Encode.
	// out1 added the NPC via writeNewNpcs (Build.Npcs now contains 7);
	// out2 will see it in writeNpcs's tracked-loop and emit the walk-mode
	// branch instead of the add-leaf, producing different bytes.
	b.npcs[7].WalkDir = 2
	out2 := ni.Encode(b, 1, r)

	if !bytes.Equal(out1, out1Saved) {
		t.Errorf("out1 mutated after second Encode: got %x, want %x", out1, out1Saved)
	}
	if bytes.Equal(out1, out2) {
		t.Errorf("out2 identical to out1 (expected different branch); both = %x", out1)
	}
}

// TestNpcInfo_FaceCoord_PreservedAcrossEncode pins that Encode does not
// mutate b.npcs[nid].FaceX or FaceZ. The encoder reads NpcHighDefOf /
// NpcLowDefOf (eager Renderer) for the face-coord wire bits but never
// writes back to the Npc struct.
//
// NAI-31 will port the upstream Rust lowdefinition fallback ladder
// (info.rs:642-664) where face_x falls back to orientation_x when
// face_x == -1. At that point this test expands to pin the fallback
// fires when face_x is -1; for NAI-30, the placeholder pins
// non-mutation only.
func TestNpcInfo_FaceCoord_PreservedAcrossEncode(t *testing.T) {
	b := New()
	setupLocalPlayer(b, 1, nil)
	setupNpc(b, 7, 100, func(n *Npc) {
		n.FaceX = 12345
		n.FaceZ = 67890
	})
	// Pre-track the NPC so writeNpcs's tracked loop iterates it
	// (rather than writeNewNpcs's add path, which is also non-mutating
	// but exercises a different code path).
	b.players[1].Build.Npcs.Insert(7)

	ni := NewNpcInfo()
	r := NewRenderer()
	ni.Encode(b, 1, r)

	if got := b.npcs[7].FaceX; got != 12345 {
		t.Errorf("FaceX mutated by Encode: got %d, want 12345", got)
	}
	if got := b.npcs[7].FaceZ; got != 67890 {
		t.Errorf("FaceZ mutated by Encode: got %d, want 67890", got)
	}
}

// TestNpcInfo_Orientation_PreservedAcrossEncode pins that Encode does
// not mutate b.npcs[nid].OrientationX or OrientationZ. Per Rust
// upstream npc.rs:68-71, orientation values persist across cleanup
// (and across Encode). The encoder's fallback ladder uses orientation
// as the face_x fallback when face_x == -1 (info.rs:328-340); the
// orientation producer (set_orient script + npc-config initial
// orientation) is deferred to the engine-port series per NAI-30-D1.
// This test pins non-mutation.
func TestNpcInfo_Orientation_PreservedAcrossEncode(t *testing.T) {
	b := New()
	setupLocalPlayer(b, 1, nil)
	setupNpc(b, 7, 100, func(n *Npc) {
		n.OrientationX = 256
		n.OrientationZ = 512
	})
	b.players[1].Build.Npcs.Insert(7)

	ni := NewNpcInfo()
	r := NewRenderer()
	ni.Encode(b, 1, r)

	if got := b.npcs[7].OrientationX; got != 256 {
		t.Errorf("OrientationX mutated by Encode: got %d, want 256", got)
	}
	if got := b.npcs[7].OrientationZ; got != 512 {
		t.Errorf("OrientationZ mutated by Encode: got %d, want 512", got)
	}
}

// TestNpcInfo_FaceEntity_PreservedAcrossEncode pins that Encode does
// not mutate b.npcs[nid].FaceEntity. Per Rust upstream npc.rs:68-71,
// FaceEntity persists across cleanup. NAI-31 may add fallback semantics
// for FaceEntity when no producer set it; for NAI-30, the placeholder
// pins non-mutation.
func TestNpcInfo_FaceEntity_PreservedAcrossEncode(t *testing.T) {
	b := New()
	setupLocalPlayer(b, 1, nil)
	setupNpc(b, 7, 100, func(n *Npc) {
		n.FaceEntity = 42
	})
	b.players[1].Build.Npcs.Insert(7)

	ni := NewNpcInfo()
	r := NewRenderer()
	ni.Encode(b, 1, r)

	if got := b.npcs[7].FaceEntity; got != 42 {
		t.Errorf("FaceEntity mutated by Encode: got %d, want 42", got)
	}
}

// readBitsRange extracts a range of bits [start, start+n) from a byte
// slice MSB-first within each byte. Used for terminator-position
// assertions where GBit's uint8 return doesn't fit a 13-bit value.
func readBitsRange(data []byte, start, n int) int {
	v := 0
	for i := 0; i < n; i++ {
		bitIdx := start + i
		byteIdx := bitIdx >> 3
		bitInByte := 7 - (bitIdx & 7)
		bit := (int(data[byteIdx]) >> bitInByte) & 1
		v = (v << 1) | bit
	}
	return v
}

// TestNpcInfo_Encode_EmitsTerminatorBeforeMaskPayloads is the NAI-31
// Bundle 3 regression: NpcInfo.Encode must emit the 13-bit 8191
// terminator after the bit-packed new-NPCs section and before the
// AccessBytes-aligned mask-payload section.
//
// Without the terminator, the Java client's getNpcPosNewVis
// (Client-Java client.java:5787-5821) loops `bitPos + 21 < packetSize*8`
// and exits only on `gBit(13) == 8191` OR on bit-budget exhaustion;
// it reads bits past the new-NPCs section into the mask-payload bytes,
// parses garbage NTypes, and crashes ("Error: T2").
//
// Setup: 1 player at default coord, 1 NPC nid=7 ntype=100 with a small
// non-nil low-def payload so writeNewNpcs both adds the NPC AND
// populates ni.updates.Data (forces the terminator branch to fire).
//
// Bit layout after Encode:
//
//	bits  0-7:   PBit(8,0)        — count
//	bits  8-20:  PBit(13,7)       — nid
//	bits 21-31:  PBit(11,100)     — ntype
//	bits 32-36:  PBit(5,0)        — dx
//	bits 37-41:  PBit(5,0)        — dz
//	bit  42:     PBit(1,1)        — extend
//	bits 43-55:  PBit(13,8191)    — terminator (the bug fix)
//	bytes 7+:    mask payload bytes (after AccessBytes)
//
// Assertion: bits 43-55 must equal 8191.
func TestNpcInfo_Encode_EmitsTerminatorBeforeMaskPayloads(t *testing.T) {
	const nid = int32(7)
	b := New()
	setupLocalPlayer(b, 1, nil)
	setupNpc(b, nid, 100, nil)

	ni := NewNpcInfo()
	r := NewRenderer()
	// Inject a small non-nil low-def payload so writeNewNpcs's add path
	// populates ni.updates.Data, forcing Encode through the
	// terminator-emitting branch (mirrors info.rs:456-462). A nil/empty
	// low-def would skip the terminator branch entirely.
	r.npcLowDef[nid] = []byte{0x80, 0xFF, 0xFF, 0xFF, 0xFF}

	out := ni.Encode(b, 1, r)

	// Output must be at least 7 bytes (8 + 35 + 13 = 56 bits packed) +
	// 5 mask-payload bytes = 12 bytes total.
	if len(out) < 7 {
		t.Fatalf("EmitsTerminator: got %d bytes, want >= 7; bytes: % x", len(out), out)
	}

	// Extract bits 43-55 (13 bits) MSB-first; must equal 8191.
	got := readBitsRange(out, 43, 13)
	if got != 8191 {
		t.Errorf("EmitsTerminator: bits 43-55 = %d (0x%x), want 8191 (0x1FFF); bytes: % x",
			got, got, out)
	}
}

// TestNpcInfo_Encode_NoOrphanByteOnPersistentFaceEntity pins the NAI-116
// wire-output: with the renderer's masks==0 gate in place, a tracked NPC
// in the Idle branch produces a 2-byte NpcInfo payload [0x01, 0x00]:
// PBit(8,1) [count=1] + PBit(1,0) [Idle leaf] = 9 bits → AccessBytes pads
// to 2 bytes. No Extend bit, no terminator (updates stays empty), no
// orphan 0x00 mask-header byte.
//
// Pre-NAI-116, the same setup produced 4 bytes [0x01, 0x9F, 0xFF, 0x00]:
// count + Extend leaf "1 00" + 13-bit terminator 8191 + orphan mask byte
// — the exact bytes the Java client crashed decoding (T2 - 1,184,162 -
// 4,3072,3090 - 1,-97,-1,0).
func TestNpcInfo_Encode_NoOrphanByteOnPersistentFaceEntity(t *testing.T) {
	b := New()
	setupLocalPlayer(b, 1, nil)
	setupNpc(b, 7, 100, nil) // masks=0, faceEntity=-1 in *pkg/rsbuf/Npc
	// Pre-track nid=7 so writeNpcs handles it (skip writeNewNpcs path).
	b.players[1].Build.Npcs.Insert(7)

	ni := NewNpcInfo()
	r := NewRenderer()
	// Source for ComputeNpcs: masks=0, entityMask!=0 (FaceEntity carrier).
	// Pre-fix this populated r.npcHighDef[7] = [0x00].
	// Post-fix r.npcHighDef[7] = nil.
	n := &fakeNpcSource{
		nid: 7, masks: 0, entityMask: NpcMaskFaceEntity,
		faceEntity: 12345, active: true,
	}
	r.ComputeNpcs([]NpcSource{n})

	out := ni.Encode(b, 1, r)
	want := []byte{0x01, 0x00}
	if !bytes.Equal(out, want) {
		t.Errorf("Encode: got % x, want % x (NAI-116: orphan 0x00 mask byte must not leak)", out, want)
	}
}
