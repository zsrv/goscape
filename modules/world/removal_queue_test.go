package world

import "testing"

// arch-28.4a: player removals must never be dropped, even when the lossy
// relay queue is saturated by RELAY_* traffic.
func TestRemovalSurvivesFullRelayQueue(t *testing.T) {
	s := &Server{relayActionQueue: make(chan func(), 64)}
	for range 64 {
		s.enqueueRelayAction(func() {}) // saturate the lossy queue
	}
	ran := false
	s.enqueueRemoval(func() { ran = true })
	s.drainRemovals()
	if !ran {
		t.Fatal("removal action was dropped")
	}
}

func TestDrainRemovalsFIFO(t *testing.T) {
	s := &Server{}
	var order []int
	for i := range 3 {
		s.enqueueRemoval(func() { order = append(order, i) })
	}
	s.drainRemovals()
	if len(order) != 3 || order[0] != 0 || order[2] != 2 {
		t.Fatalf("want FIFO [0 1 2], got %v", order)
	}
}
