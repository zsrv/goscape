package packetcapture

import (
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/zsrv/goscape/pkg/io/protocol/revision"
)

func TestNew_DisabledOK(t *testing.T) {
	var c Config
	c.Enabled = false
	m, err := New(c, slog.New(slog.NewTextHandler(os.Stderr, nil)))
	if err != nil {
		t.Fatalf("New disabled: %v", err)
	}
	if m == nil {
		t.Fatal("New returned nil module")
	}
	// Disabled mode: Capture() returns the noop, so Tap is a safe no-op.
	m.Capture().Tap(0, "11111111-2222-3333-4444-555555555555", 0, 0, nil, time.Time{})
}

func TestNew_EnabledRejectsMissingBrokers(t *testing.T) {
	var c Config
	c.Enabled = true
	c.Revision = revision.Expected
	c.RingBufferSize = 1024
	c.DrainBatchMax = 64
	if _, err := New(c, slog.New(slog.NewTextHandler(os.Stderr, nil))); err == nil {
		t.Fatal("expected error from missing brokers, got nil")
	}
}
