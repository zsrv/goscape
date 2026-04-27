package script

import (
	"testing"
)

func TestNpcIterator_StaleCheck(t *testing.T) {
	it := &NpcIterator{creationTick: 100}
	if it.Stale(100) {
		t.Error("Stale(creationTick) should be false")
	}
	if !it.Stale(101) {
		t.Error("Stale(creationTick+1) should be true")
	}
	if !it.Stale(99) {
		t.Error("Stale(creationTick-1) should be true (any !=)")
	}
}
