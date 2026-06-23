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
	s := &Server{players: newPlayerList(2048)}
	for i := range 128 {
		s.players.set(i+1, &Player{pid: i + 1})
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
// on: the snapshot reflects players at call time and stays stable while
// the live registry mutates mid-iteration (processLogouts removes players
// from the live playerList while ranging the snapshot).
func TestSnapshotPlayersCopiesAndDecouples(t *testing.T) {
	a, b, c := &Player{pid: 1}, &Player{pid: 2}, &Player{pid: 3}
	s := &Server{players: newPlayerList(2048)}
	s.players.set(1, a)
	s.players.set(2, b)
	s.players.set(3, c)

	snap := s.snapshotPlayers()
	if len(snap) != 3 || snap[0] != a || snap[1] != b || snap[2] != c {
		t.Fatalf("snapshot does not mirror players order/content")
	}

	// Remove b from the live registry (what removePlayerInternal does).
	s.players.remove(2)
	if len(snap) != 3 || snap[0] != a || snap[1] != b || snap[2] != c {
		t.Errorf("snapshot mutated by live-registry remove; want stable copy")
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
	s := &Server{players: newPlayerList(2048)}
	for i := range 8 {
		s.players.set(i+1, &Player{pid: i + 1})
	}
	s.snapshotPlayers() // scratch now holds 8 player pointers

	// "Log out" 6 players.
	for i := 3; i <= 8; i++ {
		s.players.remove(i)
	}
	s.snapshotPlayers()

	scratch := s.playerScratch[:cap(s.playerScratch)]
	for i := 2; i < len(scratch); i++ {
		if scratch[i] != nil {
			t.Fatalf("scratch[%d] still pins a player pointer after shrink; want nil tail", i)
		}
	}
}

// BenchmarkSnapshotPlayers measures one tick pass's snapshot at typical
// (50) and stress (500) player counts. docs/PORTING.md perf-row evidence.
func BenchmarkSnapshotPlayers(b *testing.B) {
	for _, n := range []int{50, 500} {
		b.Run(map[int]string{50: "players50", 500: "players500"}[n], func(b *testing.B) {
			s := &Server{players: newPlayerList(2048)}
			for i := range n {
				s.players.set(i+1, &Player{pid: i + 1})
			}
			b.ReportAllocs()
			for b.Loop() {
				_ = s.snapshotPlayers()
			}
		})
	}
}
