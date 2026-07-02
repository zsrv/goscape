package signals

import (
	"io"
	"log/slog"
	"testing"
)

// TestHandlerStopIsIdempotent pins arch-29.8: a second Stop() call must not
// panic (close of a closed channel). Production hits this when a real OS
// signal already unblocked Loop and a caller (e.g. App.Run's deferred
// cleanup, or an explicit App.Stop) also calls Stop.
func TestHandlerStopIsIdempotent(t *testing.T) {
	h := NewHandler(slog.New(slog.NewTextHandler(io.Discard, nil)))
	h.Stop()
	h.Stop() // must not panic
}
