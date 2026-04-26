package rsbuf

import (
	"testing"
)

func TestIdBitSet_InsertContains(t *testing.T) {
	s := newIdBitSet(2048, 250)
	if s.Contains(5) {
		t.Error("empty set contains 5")
	}
	s.Insert(5)
	if !s.Contains(5) {
		t.Error("after Insert(5), Contains(5) is false")
	}
	if s.Contains(6) {
		t.Error("after Insert(5), Contains(6) is true")
	}
}

func TestIdBitSet_InsertIdempotent(t *testing.T) {
	s := newIdBitSet(2048, 250)
	s.Insert(10)
	s.Insert(10)
	s.Insert(10)
	if s.Len() != 1 {
		t.Errorf("after triple Insert(10), Len: got %d, want 1", s.Len())
	}
	if got := s.Iter(); len(got) != 1 || got[0] != 10 {
		t.Errorf("Iter: got %v, want [10]", got)
	}
}

func TestIdBitSet_RemoveExisting(t *testing.T) {
	s := newIdBitSet(2048, 250)
	s.Insert(7)
	s.Insert(9)
	s.Insert(11)
	s.Remove(9)
	if s.Contains(9) {
		t.Error("after Remove(9), Contains(9) is true")
	}
	if !s.Contains(7) || !s.Contains(11) {
		t.Error("Remove(9) affected unrelated ids")
	}
	if s.Len() != 2 {
		t.Errorf("Len: got %d, want 2", s.Len())
	}
}

func TestIdBitSet_RemoveAbsentIsNoop(t *testing.T) {
	s := newIdBitSet(2048, 250)
	s.Insert(7)
	s.Remove(42) // not present — no-op
	if s.Len() != 1 || !s.Contains(7) {
		t.Errorf("Remove(absent) altered set: Len=%d, Contains(7)=%v", s.Len(), s.Contains(7))
	}
}

func TestIdBitSet_IterPreservesInsertionOrder(t *testing.T) {
	s := newIdBitSet(2048, 250)
	s.Insert(20)
	s.Insert(5)
	s.Insert(15)
	s.Insert(10)
	got := s.Iter()
	want := []int32{20, 5, 15, 10}
	if len(got) != len(want) {
		t.Fatalf("Iter len: got %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Iter[%d]: got %d, want %d", i, got[i], want[i])
		}
	}
}

func TestIdBitSet_IterAfterRemove(t *testing.T) {
	s := newIdBitSet(2048, 250)
	s.Insert(1)
	s.Insert(2)
	s.Insert(3)
	s.Remove(2)
	got := s.Iter()
	want := []int32{1, 3}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("Iter after Remove(2): got %v, want %v", got, want)
	}
}

func TestIdBitSet_IterIsCopy(t *testing.T) {
	s := newIdBitSet(2048, 250)
	s.Insert(7)
	got := s.Iter()
	got[0] = 99 // mutate caller-owned slice
	if !s.Contains(7) {
		t.Error("Iter return mutation leaked into bit-array")
	}
	if s.Iter()[0] != 7 {
		t.Error("Iter return mutation leaked into ids slice")
	}
}

func TestIdBitSet_Clear(t *testing.T) {
	s := newIdBitSet(2048, 250)
	s.Insert(1)
	s.Insert(31)
	s.Insert(32) // crosses bit-word boundary
	s.Insert(2047)
	s.Clear()
	if s.Len() != 0 {
		t.Errorf("after Clear, Len: got %d, want 0", s.Len())
	}
	for _, id := range []int32{1, 31, 32, 2047} {
		if s.Contains(id) {
			t.Errorf("after Clear, Contains(%d) is true", id)
		}
	}
}

func TestIdBitSet_BoundsBitWordCrossing(t *testing.T) {
	s := newIdBitSet(2048, 250)
	for _, id := range []int32{0, 31, 32, 33, 63, 64, 2047} {
		s.Insert(id)
	}
	for _, id := range []int32{0, 31, 32, 33, 63, 64, 2047} {
		if !s.Contains(id) {
			t.Errorf("Contains(%d) = false after Insert", id)
		}
	}
	if s.Len() != 7 {
		t.Errorf("Len: got %d, want 7", s.Len())
	}
}
