package rsbuf

import (
	"testing"

	"github.com/zsrv/goscape/pkg/buildarea"
	"github.com/zsrv/goscape/pkg/grid"
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
		false,         // tele
		-1, -1,        // runDir, walkDir
		true,          // active
		0,             // masks
		-1, -1, -1,    // faceEntity, faceX, faceZ
		-1, -1,        // orientationX, orientationZ
		-1, -1,        // damageTaken, damageType
		-1, -1,        // currentHitpoints, baseHitpoints
		-1, -1,        // animID, animDelay
		nil,           // say
		-1, -1, -1,    // graphicID, graphicHeight, graphicDelay
	)
	if modify != nil {
		modify(b.npcs[nid])
	}
}

func TestEncodeNpcLegacyEmpty(t *testing.T) {
	self := &fakeSource{slot: 1, x: 3094, z: 3106, level: 0, originX: 3094, originZ: 3106}
	ba := buildarea.New()
	g := grid.New()
	r := NewRenderer()

	payload := EncodeNpcLegacy(self, nil, ba, g, r)
	if len(payload) == 0 {
		t.Fatal("EncodeNpcLegacy should produce at least the pbit(8, 0) byte")
	}
	// First 8 bits = pbit(8, 0) = 0x00.
	if payload[0] != 0 {
		t.Errorf("first byte: got %#x, want 0x00 (count=0)", payload[0])
	}
}

func TestEncodeNpcLegacyAddsNew(t *testing.T) {
	self := &fakeSource{slot: 1, x: 3094, z: 3106, level: 0, originX: 3094, originZ: 3106}
	npc := &fakeNpcSource{nid: 7, typeID: 100, x: 3095, z: 3106, level: 0, active: true}

	ba := buildarea.New()
	g := grid.New()
	g.AddNpc(npc.nid, npc.x, npc.z, npc.level)
	r := NewRenderer()
	r.ComputeNpcs([]NpcSource{npc})

	payload := EncodeNpcLegacy(self, []NpcSource{npc}, ba, g, r)
	if len(payload) == 0 {
		t.Fatal("EncodeNpcLegacy should produce non-empty payload")
	}
	if _, ok := ba.Npcs[7]; !ok {
		t.Errorf("ba.Npcs should contain 7 after EncodeNpcLegacy; got %v", ba.Npcs)
	}
}

func TestEncodeNpcLegacyAddIncrementsObservers(t *testing.T) {
	resetObserversForTest()
	// Build a minimal scene: one player, one nearby active NPC,
	// empty subscription. EncodeNpcLegacy should emit an add and the
	// observer counter for that nid should tick from 0 to 1.
	self := &fakeSource{slot: 1, x: 3094, z: 3106, level: 0, originX: 3094, originZ: 3106}
	npc := &fakeNpcSource{nid: 100, typeID: 1, x: 3094 + 2, z: 3106 + 2, level: 0, active: true}
	g := grid.New()
	g.AddNpc(npc.nid, npc.x, npc.z, npc.level)
	ba := buildarea.New()
	r := NewRenderer()
	r.ComputeNpcs([]NpcSource{npc})

	EncodeNpcLegacy(self, []NpcSource{npc}, ba, g, r)

	if got := GetNpcObservers(npc.nid); got != 1 {
		t.Errorf("GetNpcObservers(%d) after add: got %d, want 1", npc.nid, got)
	}
}

func TestEncodeNpcLegacyRemoveOnInactiveDecrementsObservers(t *testing.T) {
	resetObserversForTest()
	// Pre-seed: NPC already subscribed + observer count = 1.
	// EncodeNpcLegacy sees Active() == false and removes it — counter
	// must decrement to 0.
	self := &fakeSource{slot: 1, x: 3094, z: 3106, level: 0, originX: 3094, originZ: 3106}
	npc := &fakeNpcSource{nid: 200, typeID: 1, x: 3094 + 2, z: 3106 + 2, level: 0, active: false}
	g := grid.New()
	g.AddNpc(npc.nid, npc.x, npc.z, npc.level)
	ba := buildarea.New()
	ba.Npcs[npc.nid] = struct{}{}
	SetObserverForTest(npc.nid, 1)
	r := NewRenderer()

	EncodeNpcLegacy(self, []NpcSource{npc}, ba, g, r)

	if got := GetNpcObservers(npc.nid); got != 0 {
		t.Errorf("GetNpcObservers(%d) after inactive-remove: got %d, want 0", npc.nid, got)
	}
}

func TestEncodeNpcLegacyRemoveOnOutOfRangeDecrementsObservers(t *testing.T) {
	resetObserversForTest()
	// Pre-seed: NPC subscribed + count = 1. This tick, move the
	// NPC far enough that zoneDist > NpcViewDistanceZones. EncodeNpcLegacy
	// should remove + decrement.
	self := &fakeSource{slot: 1, x: 3094, z: 3106, level: 0, originX: 3094, originZ: 3106}
	// NpcViewDistanceZones = 15; put NPC at (3094 + 16*8, ...) so
	// zone-distance is 16 > 15.
	npc := &fakeNpcSource{nid: 300, typeID: 1, x: 3094 + 16*8, z: 3106, level: 0, active: true}
	g := grid.New()
	g.AddNpc(npc.nid, npc.x, npc.z, npc.level)
	ba := buildarea.New()
	ba.Npcs[npc.nid] = struct{}{}
	SetObserverForTest(npc.nid, 1)
	r := NewRenderer()

	EncodeNpcLegacy(self, []NpcSource{npc}, ba, g, r)

	if got := GetNpcObservers(npc.nid); got != 0 {
		t.Errorf("GetNpcObservers(%d) after out-of-range-remove: got %d, want 0", npc.nid, got)
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
// writeNpcs emits: PBit(8,1)+PBit(1,1)+PBit(2,0) = 11 bits → 2 bytes,
// then after AccessBytes the update byte 0xab is appended → 3 bytes total.
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

	// PBit(8,1)+PBit(1,1)+PBit(2,0) = 11 bits → 2 bytes: 00000001 100_____
	// byte 0 = 0x01; byte 1 = 10000000 = 0x80; then P1(0xab) → byte 2 = 0xab.
	if len(out) != 3 {
		t.Fatalf("tracked-extend: got %d bytes, want 3; bytes: % x", len(out), out)
	}
	if out[0] != 0x01 || out[1] != 0x80 || out[2] != 0xab {
		t.Errorf("tracked-extend: got % x, want 01 80 ab", out)
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
// Observer counter decrements; nid removed; remove-leaf emitted.
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

	if !removeLeafInOutput(out) {
		t.Errorf("tele remove: expected remove-leaf in output % x", out)
	}
	if b.players[1].Build.Npcs.Contains(7) {
		t.Error("tele remove: nid 7 should not be in tracking set after Encode")
	}
	if got := b.NpcForTest(7).Observers; got != 4 {
		t.Errorf("tele remove: Observers after remove: got %d, want 4", got)
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
	npcX := 3200 + 16*8       // 128 tiles away — distance > 15
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

