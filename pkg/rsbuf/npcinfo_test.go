package rsbuf

import (
	"testing"

	"github.com/zsrv/goscape/pkg/buildarea"
	"github.com/zsrv/goscape/pkg/grid"
)

func TestEncodeNpcEmpty(t *testing.T) {
	self := &fakeSource{slot: 1, x: 3094, z: 3106, level: 0, originX: 3094, originZ: 3106}
	ba := buildarea.New()
	g := grid.New()
	r := NewRenderer()

	payload := EncodeNpc(self, nil, ba, g, r)
	if len(payload) == 0 {
		t.Fatal("EncodeNpc should produce at least the pbit(8, 0) byte")
	}
	// First 8 bits = pbit(8, 0) = 0x00.
	if payload[0] != 0 {
		t.Errorf("first byte: got %#x, want 0x00 (count=0)", payload[0])
	}
}

func TestEncodeNpcAddsNew(t *testing.T) {
	self := &fakeSource{slot: 1, x: 3094, z: 3106, level: 0, originX: 3094, originZ: 3106}
	npc := &fakeNpcSource{nid: 7, typeID: 100, x: 3095, z: 3106, level: 0, active: true}

	ba := buildarea.New()
	g := grid.New()
	g.AddNpc(npc.nid, npc.x, npc.z, npc.level)
	r := NewRenderer()
	r.ComputeNpcs([]NpcSource{npc})

	payload := EncodeNpc(self, []NpcSource{npc}, ba, g, r)
	if len(payload) == 0 {
		t.Fatal("EncodeNpc should produce non-empty payload")
	}
	if _, ok := ba.Npcs[7]; !ok {
		t.Errorf("ba.Npcs should contain 7 after EncodeNpc; got %v", ba.Npcs)
	}
}

func TestEncodeNpcAddIncrementsObservers(t *testing.T) {
	resetObserversForTest()
	// Build a minimal scene: one player, one nearby active NPC,
	// empty subscription. EncodeNpc should emit an add and the
	// observer counter for that nid should tick from 0 to 1.
	self := &fakeSource{slot: 1, x: 3094, z: 3106, level: 0, originX: 3094, originZ: 3106}
	npc := &fakeNpcSource{nid: 100, typeID: 1, x: 3094 + 2, z: 3106 + 2, level: 0, active: true}
	g := grid.New()
	g.AddNpc(npc.nid, npc.x, npc.z, npc.level)
	ba := buildarea.New()
	r := NewRenderer()
	r.ComputeNpcs([]NpcSource{npc})

	EncodeNpc(self, []NpcSource{npc}, ba, g, r)

	if got := GetNpcObservers(npc.nid); got != 1 {
		t.Errorf("GetNpcObservers(%d) after add: got %d, want 1", npc.nid, got)
	}
}

func TestEncodeNpcRemoveOnInactiveDecrementsObservers(t *testing.T) {
	resetObserversForTest()
	// Pre-seed: NPC already subscribed + observer count = 1.
	// EncodeNpc sees Active() == false and removes it — counter
	// must decrement to 0.
	self := &fakeSource{slot: 1, x: 3094, z: 3106, level: 0, originX: 3094, originZ: 3106}
	npc := &fakeNpcSource{nid: 200, typeID: 1, x: 3094 + 2, z: 3106 + 2, level: 0, active: false}
	g := grid.New()
	g.AddNpc(npc.nid, npc.x, npc.z, npc.level)
	ba := buildarea.New()
	ba.Npcs[npc.nid] = struct{}{}
	SetObserverForTest(npc.nid, 1)
	r := NewRenderer()

	EncodeNpc(self, []NpcSource{npc}, ba, g, r)

	if got := GetNpcObservers(npc.nid); got != 0 {
		t.Errorf("GetNpcObservers(%d) after inactive-remove: got %d, want 0", npc.nid, got)
	}
}

func TestEncodeNpcRemoveOnOutOfRangeDecrementsObservers(t *testing.T) {
	resetObserversForTest()
	// Pre-seed: NPC subscribed + count = 1. This tick, move the
	// NPC far enough that zoneDist > NpcViewDistanceZones. EncodeNpc
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

	EncodeNpc(self, []NpcSource{npc}, ba, g, r)

	if got := GetNpcObservers(npc.nid); got != 0 {
		t.Errorf("GetNpcObservers(%d) after out-of-range-remove: got %d, want 0", npc.nid, got)
	}
}
