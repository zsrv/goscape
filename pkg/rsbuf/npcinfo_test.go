package rsbuf

import (
	"testing"

	"github.com/zsrv/goscape/pkg/buildarea"
	"github.com/zsrv/goscape/pkg/grid"
)

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
