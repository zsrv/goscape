package rsbuf

import (
	"testing"

	"github.com/zsrv/goscape/pkg/buildarea"
	"github.com/zsrv/goscape/pkg/grid"
	"github.com/zsrv/goscape/pkg/io/packet"
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

// TestEncodeLocalPlayerTeleBitLayout pins the bit order of the local-player
// tele block against what the Java client (rev 225) decodes in getPlayerLocal.
// Client expects: has_update(1) | type=3(2) | level(2) | localX(7) | localZ(7)
// | jump(1) | extend(1), followed by 8 bits of oldVis count. Any other order
// garbles the client's localPlayer coords and crashes it with "packet size
// mismatch in getplayer" downstream.
func TestEncodeLocalPlayerTeleBitLayout(t *testing.T) {
	self := &fakeSource{
		slot:    1,
		x:       3094, z: 3106, level: 0,
		originX: 3094, originZ: 3106,
		tele: true, jump: true,
	}
	// Scene base: ((3094>>3)-6)<<3 = 3040, ((3106>>3)-6)<<3 = 3056.
	// So localX = 3094-3040 = 54 (0b0110110), localZ = 3106-3056 = 50 (0b0110010).
	all := []PlayerSource{self}
	ba := buildarea.New()
	g := grid.New()
	g.Add(self.Slot(), self.x, self.z, self.level)
	r := NewRenderer()
	r.ComputePlayers(all)

	got := Encode(self, all, ba, g, r)

	// Bit stream (MSB-first, 21 local + 8 oldVis count = 29 bits, padded to 4):
	//   1 | 11 | 00 | 0110110 | 0110010 | 1 | 0 | 00000000 | 000
	// = 11100011 01100110 01010000 00000000
	// = 0xE3     0x66     0x50     0x00
	want := []byte{0xE3, 0x66, 0x50, 0x00}
	if len(got) != len(want) {
		t.Fatalf("payload length: got %d, want %d; payload=% x", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("payload[%d]: got %#02x, want %#02x (full got=% x, want=% x)",
				i, got[i], want[i], got, want)
		}
	}
}

// TestEncodeNewPlayerAddBitLayout pins the bit order of the new-player add
// block against getPlayerNewVis in the Java client (rev 225):
//   slot(11) | dx(5) | dz(5) | jump(1) | extend=1(1)
// Client reads var6 as the X delta (line 3945) and var7 as the Z delta; jump
// precedes the always-1 extend bit. Any other order causes the client to
// teleport newly-visible players to garbage coordinates.
func TestEncodeNewPlayerAddBitLayout(t *testing.T) {
	// Self at (3094, 3106); other at (3097, 3104) → dx=+3, dz=-2.
	self := &fakeSource{
		slot:    1,
		x:       3094, z: 3106, level: 0,
		originX: 3094, originZ: 3106,
	}
	other := &fakeSource{
		slot:    2,
		x:       3097, z: 3104, level: 0,
		originX: 3097, originZ: 3104,
		jump: true,
	}
	all := []PlayerSource{self, other}
	ba := buildarea.New()
	g := grid.New()
	g.Add(self.Slot(), self.x, self.z, self.level)
	g.Add(other.Slot(), other.x, other.z, other.level)
	r := NewRenderer()
	r.ComputePlayers(all)

	payload := Encode(self, all, ba, g, r)

	// Decode the bit stream in the order the Java client reads it and verify
	// each field. This is a round-trip assertion: if fields come out right,
	// encoding matches the client's expectations.
	p := packet.NewPacket(payload)
	p.AccessBits()
	// Skip local player "idle" block (1 bit, value 0 — no masks, no movement).
	if got := p.GBit(1); got != 0 {
		t.Fatalf("local-player idle flag: got %d, want 0 (payload=% x)", got, payload)
	}
	// oldVis count for tracked-players list; self tracks nobody on first encode.
	if got := p.GBit(8); got != 0 {
		t.Fatalf("oldVis player count: got %d, want 0", got)
	}
	// Now the new-player add block for `other`.
	slot := int(p.GBit(11))
	if slot != other.slot {
		t.Errorf("add slot: got %d, want %d", slot, other.slot)
	}
	dx := int(p.GBit(5))
	if dx >= 16 {
		dx -= 32
	}
	if dx != 3 {
		t.Errorf("add dx: got %d, want 3", dx)
	}
	dz := int(p.GBit(5))
	if dz >= 16 {
		dz -= 32
	}
	if dz != -2 {
		t.Errorf("add dz: got %d, want -2", dz)
	}
	if got := p.GBit(1); got != 1 {
		t.Errorf("add jump: got %d, want 1", got)
	}
	if got := p.GBit(1); got != 1 {
		t.Errorf("add extend (constant 1): got %d, want 1", got)
	}
}

// TestEncodeIdleWithCachedFaceEntityNoOrphanMaskByte pins the bug where an
// idle player (masks=0) but with a non-zero EntityMask field caused the
// renderer to cache a 1-byte mask header and the encoder to append it after
// the 2047 sentinel — producing a 4-byte packet whose bit-stream content
// only filled 3 bytes. The Java client's getPlayer ended at pos:3 with
// psize:4 and crashed with "Error packet size mismatch in getplayer".
//
// Expected idle payload: 1 bit (idle flag) + 8 bits (oldVis count=0)
// = 9 bits, byte-aligned to 2 bytes [0x00, 0x00]. No 2047 sentinel
// (no extends), no mask-update bytes.
func TestEncodeIdleWithCachedFaceEntityNoOrphanMaskByte(t *testing.T) {
	self := &fakeSourceWithEntityMask{
		fakeSource: fakeSource{
			slot:    1,
			x:       3094, z: 3106, level: 0,
			originX: 3094, originZ: 3106,
			masks: 0, // idle: no movement, no fresh masks
		},
		// EntityMask non-zero mirrors Player.entitymask = MaskFaceEntity in
		// newPlayer — the original trigger for the orphan-byte bug.
		entityMaskOverride: MaskFaceEntity,
	}
	all := []PlayerSource{self}
	ba := buildarea.New()
	g := grid.New()
	g.Add(self.Slot(), self.x, self.z, self.level)
	r := NewRenderer()
	r.ComputePlayers(all)

	got := Encode(self, all, ba, g, r)

	want := []byte{0x00, 0x00}
	if len(got) != len(want) {
		t.Fatalf("idle payload length: got %d, want %d (got=% x)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("byte[%d]: got %#02x, want %#02x (full=% x)", i, got[i], want[i], got)
		}
	}
}

// fakeSourceWithEntityMask overrides EntityMask() so we can test the
// Player.entitymask = MaskFaceEntity scenario without altering the shared
// fakeSource type.
type fakeSourceWithEntityMask struct {
	fakeSource
	entityMaskOverride int
}

func (f *fakeSourceWithEntityMask) EntityMask() int { return f.entityMaskOverride }

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
