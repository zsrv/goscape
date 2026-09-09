package signals

import (
	"io"
	"log/slog"
	"testing"
)

// TestHandler_StopIdempotent guards the arch-29.8 backfill: calling Stop
// twice must not panic with "close of closed channel". A real OS signal
// unblocking Loop and a caller also invoking Stop is exactly the race this
// protects against.
func TestHandler_StopIdempotent(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := NewHandler(logger)

	h.Stop()
	h.Stop()

	select {
	case <-h.quit:
		// quit is closed, as expected.
	default:
		t.Fatal("expected h.quit to be closed after Stop")
	}
}
