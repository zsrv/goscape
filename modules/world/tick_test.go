package world

import (
	"testing"
)

// TestProcessLoginsAllocatesInputTracking pins that newly logged-in
// players have a non-nil InputTracking with a future-scheduled window.
// NAI-73 T6.
func TestProcessLoginsAllocatesInputTracking(t *testing.T) {
	s := newTestServer(t)
	p, _ := newTestPlayer(t)
	p.client.server = s
	s.newPlayers = []*Player{p}
	s.currentTick = 1000

	s.processLogins()

	if p.input == nil {
		t.Fatal("p.input: must be non-nil after processLogins")
	}
	if p.input.player != p {
		t.Error("p.input.player back-pointer must equal p")
	}
	wantMin := 1000 + inputTrackingRate - inputTrackingJitterRange
	wantMax := 1000 + inputTrackingRate + inputTrackingJitterRange
	if p.input.startTrackingAt < wantMin || p.input.startTrackingAt > wantMax {
		t.Errorf("startTrackingAt: got %d, want in [%d, %d]", p.input.startTrackingAt, wantMin, wantMax)
	}
	if got, want := p.input.endTrackingAt, p.input.startTrackingAt+inputTrackingTime; got != want {
		t.Errorf("endTrackingAt: got %d, want %d (startTrackingAt + inputTrackingTime)", got, want)
	}
	if got, want := p.session, "headless"; got != want {
		t.Errorf("p.session default: got %q, want %q", got, want)
	}
}
