package world

import (
	"testing"
	"time"
)

// TestHealthSnapshotBeforeFirstTick pins the pre-tick zero state
// HealthSnapshot reports (consumed by modules/ondemand's boot-grace
// handling, arch-29.6): LastTick is the Unix epoch (the atomic's zero
// value run through time.Unix), not "now".
func TestHealthSnapshotBeforeFirstTick(t *testing.T) {
	s := newTestServer(t)
	snap := s.HealthSnapshot()
	if snap.LastTick.Unix() != 0 {
		t.Fatalf("LastTick before first tick: got %v, want the Unix epoch", snap.LastTick)
	}
	if snap.CurrentTick != 0 {
		t.Fatalf("CurrentTick before first tick: got %d, want 0", snap.CurrentTick)
	}
}

func TestHealthSnapshotTracksTickAndPlayers(t *testing.T) {
	s := newTestServer(t)

	s.stampTick(0)
	snap := s.HealthSnapshot()
	if time.Since(snap.LastTick) > time.Second {
		t.Fatalf("LastTick not stamped: got %v", snap.LastTick)
	}
	if snap.PlayersOnline != 0 {
		t.Fatalf("PlayersOnline: got %d, want 0 before any player joins", snap.PlayersOnline)
	}

	c, _ := newTestClient(t)
	p := newPlayer(c)
	if err := s.addPlayer(p); err != nil {
		t.Fatalf("addPlayer: %v", err)
	}
	if got := s.HealthSnapshot().PlayersOnline; got != 1 {
		t.Fatalf("PlayersOnline after addPlayer: got %d, want 1", got)
	}

	s.removePlayerInternal(p)
	if got := s.HealthSnapshot().PlayersOnline; got != 0 {
		t.Fatalf("PlayersOnline after removePlayerInternal: got %d, want 0", got)
	}

	// removePlayerInternal's slot-identity guard must make this idempotent
	// — a double-removal (e.g. the tick's idle-logout racing an
	// already-enqueued disconnect removal) must not drift the counter
	// negative.
	s.removePlayerInternal(p)
	if got := s.HealthSnapshot().PlayersOnline; got != 0 {
		t.Fatalf("PlayersOnline after duplicate removePlayerInternal: got %d, want 0 (must not go negative)", got)
	}
}

// TestHealthSnapshotCurrentTickIsTickOwnedCopy pins the mid-brief
// correction: CurrentTick must come from currentTickAtomic (stamped by
// stampTick), never from a direct cross-goroutine read of the
// tick-owned s.currentTick int.
func TestHealthSnapshotCurrentTickIsTickOwnedCopy(t *testing.T) {
	s := newTestServer(t)
	s.currentTick = 41
	s.currentTick++ // simulate tick.go's s.currentTick++
	s.stampTick(0)
	if got := s.HealthSnapshot().CurrentTick; got != 42 {
		t.Fatalf("CurrentTick: got %d, want 42", got)
	}
}

// TestHealthSnapshotLastCycleMillis pins that stampTick's cycleMillis
// argument (the tick loop's time.Since(start)) surfaces as
// HealthSnapshot.LastCycleMillis. rev-225 has no lastCycleStats array, so
// the value is passed to stampTick directly rather than read from a stat
// index.
func TestHealthSnapshotLastCycleMillis(t *testing.T) {
	s := newTestServer(t)
	s.stampTick(12)
	if got := s.HealthSnapshot().LastCycleMillis; got != 12 {
		t.Fatalf("LastCycleMillis: got %d, want 12", got)
	}
}
