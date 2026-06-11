package world

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

// TestSlogLoggerBridgeNotifyPlayerReport pins the rev-254 A3 report
// shape (TS World.ts:2309-2324 @2e3bcf43 posts session_uuid;
// LoggerThread.ts:45-51 destructures it): type=report, world, profile,
// session_uuid, timestamp, coord, offender, reason — the 244-era
// username key is GONE (re-keyed back to the session UUID).
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
		"session_uuid=test-session",
		"timestamp_ms=",
		"offender=evilbob",
		"reason=MACROING",
		"coord=", // packed value; exact value asserted separately
	} {
		if !strings.Contains(out, want) {
			t.Errorf("log output missing %q: %s", want, out)
		}
	}
	if strings.Contains(out, "username=") {
		t.Errorf("report still carries the 244 username key (re-keyed to session_uuid at 254): %s", out)
	}
}

// TestSlogLoggerBridgeSubmitInputTracking pins the rev-254 A5 record
// shape (TS World.submitInputTracking @2e3bcf43 World.ts:2326-2333 posts
// {type:'input_track', session_uuid: player.session, timestamp, buf:
// base64}): the blob wrapper (username/seq/coord/blob_count) is GONE,
// and — unlike report/session_log/wealth_event — the LoggerClient
// inputTrack envelope does NOT stamp world/profile
// (LoggerClient.ts:64-79 @2e3bcf43).
func TestSlogLoggerBridgeSubmitInputTracking(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	bridge := NewSlogLoggerBridge(logger, 10, "main")

	p := &Player{username: "alice", session: "test-session-uuid"}
	bridge.SubmitInputTracking(p, []byte{0x00, 0x01, 0x02})

	out := buf.String()
	for _, want := range []string{
		"type=input_track",
		"session_uuid=test-session-uuid",
		"timestamp_ms=",
		"buf=AAEC", // base64 of 00 01 02 — receiver-side encode
	} {
		if !strings.Contains(out, want) {
			t.Errorf("log output missing %q: %s", want, out)
		}
	}
	for _, gone := range []string{
		"username=", "blob_count=", "blobs=", "seq:", "coord=",
		"world=", "profile=",
	} {
		if strings.Contains(out, gone) {
			t.Errorf("log output still carries 43e02957-era key %q: %s", gone, out)
		}
	}
}

// TestSlogLoggerBridgeSubmitInputTrackingHeadless pins the empty-session
// → "headless" fallback at the receiver: TS Player.session defaults to
// 'headless' (Player.ts:311 @2e3bcf43); goscape's zero value is "" and
// sessionOrHeadless maps it.
func TestSlogLoggerBridgeSubmitInputTrackingHeadless(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	bridge := NewSlogLoggerBridge(logger, 10, "main")

	bridge.SubmitInputTracking(&Player{username: "headlessbot"}, []byte{0x01})

	if out := buf.String(); !strings.Contains(out, "session_uuid=headless") {
		t.Errorf("log output missing session_uuid=headless: %s", out)
	}
}
