package world

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

// TestSlogLoggerBridgeNotifyPlayerReport pins that NotifyPlayerReport
// emits a structured slog record with the expected keys: type=report,
// session, offender, reason, coord.
func TestSlogLoggerBridgeNotifyPlayerReport(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	bridge := NewSlogLoggerBridge(logger)

	p := &Player{session: "test-session"}
	p.x = 3200
	p.z = 3200
	// p.level defaults to 0

	bridge.NotifyPlayerReport(p, "evilbob", "MACROING")

	out := buf.String()
	for _, want := range []string{
		"type=report",
		"session=test-session",
		"offender=evilbob",
		"reason=MACROING",
		"coord=", // packed value; exact value asserted separately
	} {
		if !strings.Contains(out, want) {
			t.Errorf("log output missing %q: %s", want, out)
		}
	}
}

// TestSlogLoggerBridgeSubmitInputTracking pins that SubmitInputTracking
// emits a record with type=input_track, session, blob_len, blob_b64.
func TestSlogLoggerBridgeSubmitInputTracking(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	bridge := NewSlogLoggerBridge(logger)

	p := &Player{session: "test-session"}
	bridge.SubmitInputTracking(p, []byte{0x00, 0x01, 0x02})

	out := buf.String()
	for _, want := range []string{
		"type=input_track",
		"session=test-session",
		"blob_len=3",
		"blob_b64=AAEC", // base64 of 0x000102
	} {
		if !strings.Contains(out, want) {
			t.Errorf("log output missing %q: %s", want, out)
		}
	}
}
