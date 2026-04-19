package rsbuf

import (
	"testing"

	"github.com/zsrv/goscape/pkg/buildarea"
	"github.com/zsrv/goscape/pkg/grid"
)

func TestEncodeIdlePlayer(t *testing.T) {
	self := &fakeSource{
		slot:    1,
		x:       3094, z: 3106, level: 0,
		originX: 3094, originZ: 3106,
		masks: 0, entityMask: 0,
	}
	all := []PlayerSource{self}
	ba := buildarea.New()
	g := grid.New()
	r := NewRenderer()
	r.ComputePlayers(all)

	payload := Encode(self, all, ba, g, r)
	if len(payload) == 0 {
		t.Fatal("Encode should produce non-empty payload")
	}
	// Idle player, no other players, no masks.
	// First phase: pbit(1, 0) for idle.
	// This first bit is the MSB of byte[0], so top bit should be 0.
	if payload[0]&0x80 != 0 {
		t.Errorf("first bit (idle flag): got 1, want 0; payload=%v", payload)
	}
}

func TestEncodeTwoPlayersAddsOther(t *testing.T) {
	a := &fakeSource{
		slot:    1,
		x:       3094, z: 3106, level: 0,
		originX: 3094, originZ: 3106,
		appearance: []byte{1, 2, 3},
	}
	b := &fakeSource{
		slot:    2,
		x:       3095, z: 3106, level: 0,
		originX: 3095, originZ: 3106,
		appearance: []byte{4, 5, 6},
	}
	all := []PlayerSource{a, b}

	baA := buildarea.New()
	g := grid.New()
	g.Add(a.Slot(), a.x, a.z, a.level)
	g.Add(b.Slot(), b.x, b.z, b.level)

	r := NewRenderer()
	r.ComputePlayers(all)

	payload := Encode(a, all, baA, g, r)
	if len(payload) == 0 {
		t.Fatal("Encode should produce non-empty payload")
	}
	// After encoding, a's BuildArea.Players should contain b's slot.
	if _, ok := baA.Players[2]; !ok {
		t.Errorf("after encode, a's tracked players should include slot 2; got %v", baA.Players)
	}
}
