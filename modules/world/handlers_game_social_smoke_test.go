package world

import (
	"testing"

	io2 "github.com/zsrv/goscape/pkg/io/isaac"
	util "github.com/zsrv/goscape/pkg/util/jstring"
)

// TestNAI72AllSixOpcodesDispatch drives all 6 NAI-72 opcodes through one
// recordingBridges fixture and asserts the expected bridge effect for
// each, plus the per-tick reset between bursts. Mirrors a smoke pass of
// the entire bundle.
func TestNAI72AllSixOpcodesDispatch(t *testing.T) {
	s := newTestServer(t)
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.username = "alice"
	// REPORT_ABUSE in-range path calls MessageGame → writeOut → ISAAC.
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	rec := installRecordingBridges(s)
	p.slot = 1
	s.playersMu.Lock()
	s.players.set(1, p)
	s.playersMu.Unlock()

	target := util.ToBase37("bob")

	// Burst 1: all 4 social-list opcodes — second through fourth must be
	// gated by socialProtect (set by the first).
	if err := handleFriendListAdd(p, payloadG8(target)); err != nil {
		t.Fatalf("FriendListAdd: %v", err)
	}
	if err := handleFriendListDel(p, payloadG8(target)); err != nil {
		t.Fatalf("FriendListDel: %v", err)
	}
	if err := handleIgnoreListAdd(p, payloadG8(target)); err != nil {
		t.Fatalf("IgnoreListAdd: %v", err)
	}
	if err := handleIgnoreListDel(p, payloadG8(target)); err != nil {
		t.Fatalf("IgnoreListDel: %v", err)
	}
	if len(rec.friends) != 1 {
		t.Errorf("burst 1: friends bridge calls = %d, want 1 (last 3 gated by socialProtect)", len(rec.friends))
	}

	// CHAT_SETMODE always fires (no socialProtect gate).
	if err := handleChatSetMode(p, []byte{0, 1, 0}); err != nil {
		t.Fatalf("ChatSetMode: %v", err)
	}
	if len(rec.friends) != 2 {
		t.Errorf("after ChatSetMode: friends bridge = %d, want 2", len(rec.friends))
	}

	// REPORT_ABUSE in-range fires logger and sets reportAbuseProtect.
	if err := handleReportAbuse(p, reportAbusePayload(target, ReportAbuseOffensiveLanguage, false)); err != nil {
		t.Fatalf("ReportAbuse: %v", err)
	}
	if len(rec.logger) != 1 {
		t.Errorf("logger: got %d, want 1", len(rec.logger))
	}
	if !p.reportAbuseProtect {
		t.Error("reportAbuseProtect not set")
	}
	// Second ReportAbuse this tick is gated.
	rec.logger = nil
	if err := handleReportAbuse(p, reportAbusePayload(target, ReportAbuseOffensiveLanguage, false)); err != nil {
		t.Fatalf("ReportAbuse #2: %v", err)
	}
	if len(rec.logger) != 0 {
		t.Errorf("second ReportAbuse same tick: logger = %d, want 0 (gated)", len(rec.logger))
	}

	// Per-tick reset clears both protect flags.
	s.processCleanup()
	if p.socialProtect {
		t.Error("socialProtect not reset by processCleanup")
	}
	if p.reportAbuseProtect {
		t.Error("reportAbuseProtect not reset by processCleanup")
	}

	// Burst 2: post-reset, social handlers fire again.
	rec.friends = nil
	if err := handleFriendListAdd(p, payloadG8(target)); err != nil {
		t.Fatalf("FriendListAdd post-reset: %v", err)
	}
	if len(rec.friends) != 1 {
		t.Errorf("post-reset FriendListAdd: friends = %d, want 1", len(rec.friends))
	}
}
