package telemetry

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"
)

// TestStopping_ClampsZeroStopTimeout pins the shutdown budget for a config
// built as a struct literal (tests, and any embedder that skips the flag
// defaults): StopTimeout of 0 must fall back to the 5s default rather than
// handing otelShutdown an already-expired context, which made every clean stop
// log "OTel provider shutdown: context deadline exceeded".
func TestStopping_ClampsZeroStopTimeout(t *testing.T) {
	for _, tc := range []struct {
		name    string
		cfg     time.Duration
		wantMin time.Duration
	}{
		{"zero falls back to the default", 0, 4 * time.Second},
		{"negative falls back to the default", -time.Second, 4 * time.Second},
		{"configured value is honoured", 2 * time.Second, time.Second},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var gotDeadline time.Duration
			var hadDeadline bool
			tel := &Telemetry{
				cfg: Config{StopTimeout: tc.cfg},
				log: slog.New(slog.NewTextHandler(io.Discard, nil)),
				otelShutdown: func(ctx context.Context) error {
					dl, ok := ctx.Deadline()
					hadDeadline = ok
					if ok {
						gotDeadline = time.Until(dl)
					}
					return ctx.Err()
				},
			}
			if err := tel.stopping(nil); err != nil {
				t.Fatalf("stopping: %v", err)
			}
			if !hadDeadline {
				t.Fatal("otelShutdown was called without a deadline")
			}
			if gotDeadline < tc.wantMin {
				t.Fatalf("otelShutdown deadline = %v, want at least %v", gotDeadline, tc.wantMin)
			}
		})
	}
}
