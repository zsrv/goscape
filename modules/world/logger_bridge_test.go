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
// emits a record with type=input_track, username, session_uuid, blob_count.
// 244 re-shape: username + session_uuid + ALL blobs (InputTracking.ts:147,
// World.ts:2343-2351). The old session/blob_len/blob_b64 fields are gone.
func TestSlogLoggerBridgeSubmitInputTracking(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	bridge := NewSlogLoggerBridge(logger)

	blobs := []InputTrackingBlob{
		NewInputTrackingBlob([]byte{0x00, 0x01, 0x02}, 1, 0xC0DE),
		NewInputTrackingBlob([]byte{0xFF}, 2, 0xBEEF),
	}
	bridge.SubmitInputTracking("alice", "test-session-uuid", blobs)

	out := buf.String()
	for _, want := range []string{
		"type=input_track",
		"username=alice",
		"session_uuid=test-session-uuid",
		"blob_count=2",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("log output missing %q: %s", want, out)
		}
	}
}
