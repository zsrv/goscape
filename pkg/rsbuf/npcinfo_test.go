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
