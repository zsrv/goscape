package world

import (
	"testing"
)

// TestSnapshotPlayersZeroAllocSteadyState pins the PERF-1 contract: the
// per-tick player-snapshot passes (13 sites in tick.go) must not allocate
// once the tick-owned scratch buffer has warmed up. Pre-fix each pass did
// make([]*Player)+copy — ~13 allocs/tick (~22/s at 600ms ticks, scaling
// with player count).
func TestSnapshotPlayersZeroAllocSteadyState(t *testing.T) {
	s := &Server{}
	for range 128 {
		s.playerLoop = append(s.playerLoop, &Player{})
	}

	// Warm the scratch to steady state.
	s.snapshotPlayers()

	allocs := testing.AllocsPerRun(100, func() {
		players := s.snapshotPlayers()
		if len(players) != 128 {
			t.Fatalf("snapshot length = %d, want 128", len(players))
		}
	})
	if allocs != 0 {
		t.Errorf("snapshotPlayers steady-state allocs/op = %v, want 0", allocs)
	}
}

// TestSnapshotPlayersCopiesAndDecouples pins the semantic the passes rely
// on: the snapshot reflects playerLoop at call time and stays stable while
// the live slice mutates mid-iteration (processLogouts splices players out
// of playerLoop while ranging the snapshot).
func TestSnapshotPlayersCopiesAndDecouples(t *testing.T) {
	a, b, c := &Player{}, &Player{}, &Player{}
	s := &Server{playerLoop: []*Player{a, b, c}}

	snap := s.snapshotPlayers()
	if len(snap) != 3 || snap[0] != a || snap[1] != b || snap[2] != c {
		t.Fatalf("snapshot does not mirror playerLoop order/content")
	}

	// Splice b out of the live slice (what removePlayerInternal does).
	s.playerLoop = append(s.playerLoop[:1], s.playerLoop[2:]...)
	if len(snap) != 3 || snap[0] != a || snap[1] != b || snap[2] != c {
		t.Errorf("snapshot mutated by live-slice splice; want stable copy")
	}

	// The next snapshot reflects the new state.
	snap2 := s.snapshotPlayers()
	if len(snap2) != 2 || snap2[0] != a || snap2[1] != c {
		t.Errorf("second snapshot = %v entries, want [a c]", len(snap2))
	}
}

// TestSnapshotPlayersClearsStaleTail pins the anti-pinning hygiene of the
// scratch buffer: when the player count shrinks, pointers beyond the new
// length must be nil'd so departed players aren't kept reachable by the
// scratch's spare capacity for the rest of the process lifetime.
func TestSnapshotPlayersClearsStaleTail(t *testing.T) {
	s := &Server{}
	for range 8 {
		s.playerLoop = append(s.playerLoop, &Player{})
	}
	s.snapshotPlayers() // scratch now holds 8 player pointers

	s.playerLoop = s.playerLoop[:2] // 6 players "log out"
	s.snapshotPlayers()

	scratch := s.playerScratch[:cap(s.playerScratch)]
	for i := 2; i < len(scratch); i++ {
		if scratch[i] != nil {
			t.Fatalf("scratch[%d] still pins a player pointer after shrink; want nil tail", i)
		}
	}
}

// BenchmarkSnapshotPlayers measures one tick pass's snapshot at typical
// (50) and stress (500) player counts. PORTING.md perf-row evidence.
func BenchmarkSnapshotPlayers(b *testing.B) {
	for _, n := range []int{50, 500} {
		b.Run(map[int]string{50: "players50", 500: "players500"}[n], func(b *testing.B) {
			s := &Server{}
			for range n {
				s.playerLoop = append(s.playerLoop, &Player{})
			}
			b.ReportAllocs()
			for b.Loop() {
				_ = s.snapshotPlayers()
			}
		})
	}
}
