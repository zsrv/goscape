package world

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

// TestSlogLoggerBridgeNotifyPlayerReport pins the rev-244 report shape
// (TS LoggerClient.ts:48-67): type=report, world, profile, username,
// timestamp, coord, offender, reason — the 225-era session uuid key is
// GONE (re-keyed to username at 244).
func TestSlogLoggerBridgeNotifyPlayerReport(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	bridge := NewSlogLoggerBridge(logger, 10, "main")

	p := &Player{username: "alice", session: "test-session"}
	p.x = 3200
	p.z = 3200
	// p.level defaults to 0

	bridge.NotifyPlayerReport(p, "evilbob", "MACROING")

	out := buf.String()
	for _, want := range []string{
		"type=report",
		"world=10",
		"profile=main",
		"username=alice",
		"timestamp_ms=",
		"offender=evilbob",
		"reason=MACROING",
		"coord=", // packed value; exact value asserted separately
	} {
		if !strings.Contains(out, want) {
			t.Errorf("log output missing %q: %s", want, out)
		}
	}
	if strings.Contains(out, "session=") {
		t.Errorf("report still carries the 225 session key (re-keyed to username at 244): %s", out)
	}
}

// TestSlogLoggerBridgeSubmitInputTracking pins that SubmitInputTracking
// emits a record with type=input_track, username, session_uuid, blob_count.
// 244 re-shape: username + session_uuid + ALL blobs (InputTracking.ts:147,
// World.ts:2343-2351). The old session/blob_len/blob_b64 fields are gone.
func TestSlogLoggerBridgeSubmitInputTracking(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	bridge := NewSlogLoggerBridge(logger, 10, "main")

	blobs := []InputTrackingBlob{
		NewInputTrackingBlob([]byte{0x00, 0x01, 0x02}, 1, 0xC0DE),
		NewInputTrackingBlob([]byte{0xFF}, 2, 0xBEEF),
	}
	bridge.SubmitInputTracking("alice", "test-session-uuid", blobs)

	out := buf.String()
	for _, want := range []string{
		"type=input_track",
		"world=10",
		"profile=main",
		"username=alice",
		"session_uuid=test-session-uuid",
		"blob_count=2",
		"blobs=", // the payload attribute itself must be present
		"seq:1",  // first blob rendered inside the blobs attribute
		"seq:2",  // second blob present → ALL blobs emitted, not just [0]
	} {
		if !strings.Contains(out, want) {
			t.Errorf("log output missing %q: %s", want, out)
		}
	}
}
