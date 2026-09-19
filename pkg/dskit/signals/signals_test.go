package signals

import (
	"io"
	"log/slog"
	"testing"
)

// TestHandlerStopIsIdempotent pins arch-29.8: a second Stop() call must not
// panic (close of a closed channel), and Stop must genuinely close quit
// rather than silently do nothing. Production hits the double call when a
// real OS signal already unblocked Loop and a caller (e.g. App.Run's
// deferred cleanup, or an explicit App.Stop) also calls Stop.
func TestHandlerStopIsIdempotent(t *testing.T) {
	h := NewHandler(slog.New(slog.NewTextHandler(io.Discard, nil)))
	h.Stop()
	h.Stop() // must not panic

	select {
	case <-h.quit:
		// quit is closed, as expected.
	default:
		t.Fatal("expected h.quit to be closed after Stop")
	}
}
