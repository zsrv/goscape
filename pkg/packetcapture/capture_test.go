package packetcapture

import (
	"testing"
	"time"

	"go.opentelemetry.io/otel/metric/noop"

	"github.com/zsrv/goscape/pkg/tapper"
)

// TestRealCaptureSatisfiesTapper is a compile-time assertion that the concrete
// *Capture satisfies the tapper.Tapper seam the world module taps. It lives
// here rather than in pkg/tapper because that package must keep compiling
// without any concrete implementation.
func TestRealCaptureSatisfiesTapper(t *testing.T) {
	var _ tapper.Tapper = (*Capture)(nil)
}

func newTestCapture(enabled bool, capacity int) *Capture {
	m, _ := NewMetrics(noop.NewMeterProvider().Meter("test"))
	return &Capture{
		enabled:  enabled,
		ring:     NewRingBuffer(capacity),
		metrics:  m,
		worldID:  1,
		revision: 225,
	}
}

func TestCapture_TapPushesToRing(t *testing.T) {
	c := newTestCapture(true, 8)
	c.SessionStarted(42, "11111111-2222-3333-4444-555555555555", time.Now())
	c.Tap(42, "11111111-2222-3333-4444-555555555555", tapper.DirOut, 181, []byte{1, 2, 3}, time.Now())

	if got := c.ring.Len(); got != 2 { // STARTED + 1 packet
		t.Fatalf("ring.Len=%d, want 2", got)
	}
}

func TestCapture_TapDisabledIsShortCircuit(t *testing.T) {
	c := newTestCapture(false, 8)
	c.SessionStarted(42, "11111111-2222-3333-4444-555555555555", time.Now())
	c.Tap(42, "11111111-2222-3333-4444-555555555555", tapper.DirOut, 181, []byte{1, 2, 3}, time.Now())
	c.SessionEnded(42, "11111111-2222-3333-4444-555555555555", time.Now(), tapper.CloseReasonLogout)

	if got := c.ring.Len(); got != 0 {
		t.Fatalf("ring.Len=%d, want 0 (disabled path must not enqueue)", got)
	}
}

func TestCapture_SessionEndedCarriesCounters(t *testing.T) {
	c := newTestCapture(true, 8)
	sid := "11111111-2222-3333-4444-555555555555"
	c.SessionStarted(42, sid, time.Now())
	c.Tap(42, sid, tapper.DirIn, 181, []byte{1, 2, 3, 4}, time.Now())
	c.Tap(42, sid, tapper.DirIn, 181, []byte{5, 6}, time.Now())
	c.Tap(42, sid, tapper.DirOut, 90, []byte{7}, time.Now())
	c.SessionEnded(42, sid, time.Now(), tapper.CloseReasonLogout)

	stats, ok := c.lastSessionStats(sid)
	if !ok {
		t.Fatal("session stats unavailable after End")
	}
	if stats.PacketCountIn != 2 || stats.PacketCountOut != 1 ||
		stats.ByteCountIn != 6 || stats.ByteCountOut != 1 {
		t.Fatalf("counters wrong: %+v", stats)
	}
}
