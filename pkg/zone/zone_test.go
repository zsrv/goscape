package zone

import (
	"bytes"
	"testing"
)

func TestNewZoneFields(t *testing.T) {
	z := New(42, 1, 100, 200)
	if z.Index != 42 {
		t.Errorf("Index: got %d, want 42", z.Index)
	}
	if z.Level != 1 || z.X != 100 || z.Z != 200 {
		t.Errorf("coords: got (L=%d, X=%d, Z=%d), want (1,100,200)", z.Level, z.X, z.Z)
	}
	if z.entityEvents == nil {
		t.Error("entityEvents map should be initialised")
	}
	if z.Shared() != nil {
		t.Error("fresh zone should have nil Shared()")
	}
	if len(z.Events()) != 0 {
		t.Error("fresh zone should have no events")
	}
}

func TestComputeSharedEmptyIsNil(t *testing.T) {
	z := New(0, 0, 0, 0)
	z.ComputeShared()
	if z.Shared() != nil {
		t.Errorf("Shared after empty ComputeShared: got %v, want nil", z.Shared())
	}
}

func TestComputeSharedConcatsEnclosed(t *testing.T) {
	z := New(0, 0, 0, 0)
	z.events = []ZoneEvent{
		{Type: ZoneEventEnclosed, ReceiverID: PublicReceiver, Bytes: []byte{0x01, 0x02}},
		{Type: ZoneEventEnclosed, ReceiverID: PublicReceiver, Bytes: []byte{0x03, 0x04, 0x05}},
	}
	z.ComputeShared()
	want := []byte{0x01, 0x02, 0x03, 0x04, 0x05}
	if !bytes.Equal(z.Shared(), want) {
		t.Errorf("Shared: got %v, want %v", z.Shared(), want)
	}
}

func TestComputeSharedSkipsFollows(t *testing.T) {
	z := New(0, 0, 0, 0)
	z.events = []ZoneEvent{
		{Type: ZoneEventEnclosed, Bytes: []byte{0xEE}},
		{Type: ZoneEventFollows, ReceiverID: 5, Bytes: []byte{0xFF}},
	}
	z.ComputeShared()
	if !bytes.Equal(z.Shared(), []byte{0xEE}) {
		t.Errorf("Shared: got %v, want [0xEE]", z.Shared())
	}
}

func TestComputeSharedSkipsTombstones(t *testing.T) {
	z := New(0, 0, 0, 0)
	z.events = []ZoneEvent{
		{Type: ZoneEventEnclosed, Bytes: []byte{0x11}},
		{Type: ZoneEventEnclosed, Bytes: nil}, // tombstone
		{Type: ZoneEventEnclosed, Bytes: []byte{0x22}},
	}
	z.ComputeShared()
	if !bytes.Equal(z.Shared(), []byte{0x11, 0x22}) {
		t.Errorf("Shared: got %v, want [0x11 0x22]", z.Shared())
	}
}

func TestResetClearsEverything(t *testing.T) {
	z := New(0, 0, 0, 0)
	z.events = []ZoneEvent{{Type: ZoneEventEnclosed, Bytes: []byte{1}}}
	z.ComputeShared()
	z.Reset()
	if z.Shared() != nil {
		t.Error("Shared should be nil after Reset")
	}
	if len(z.Events()) != 0 {
		t.Error("events should be empty after Reset")
	}
	if len(z.entityEvents) != 0 {
		t.Error("entityEvents should be empty after Reset")
	}
}
