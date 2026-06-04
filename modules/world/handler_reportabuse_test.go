package world

import (
	"bytes"
	"testing"
	"time"

	io2 "github.com/zsrv/goscape/pkg/io/isaac"
	util "github.com/zsrv/goscape/pkg/util/jstring"
)

// reportAbusePayload builds the 10-byte REPORT_ABUSE payload:
//
//	g8 offender (big-endian uint64)
//	g1 reason
//	g1 moderatorMute (1=true, 0=false)
func reportAbusePayload(offender uint64, reason ReportAbuseReason, moderatorMute bool) []byte {
	mute := byte(0)
	if moderatorMute {
		mute = 1
	}
	return []byte{
		byte(offender >> 56), byte(offender >> 48), byte(offender >> 40), byte(offender >> 32),
		byte(offender >> 24), byte(offender >> 16), byte(offender >> 8), byte(offender),
		byte(reason),
		mute,
	}
}

// reportAbuseSetup wires a Player against a Server with recordingBridges and
// sets p.username = "alice". Installs an ISAAC encryptor so writeOut
// (called via MessageGame on the in-range path) does not panic.
// Returns p and the recorder.
func reportAbuseSetup(t *testing.T) (*Player, *recordingBridges) {
	t.Helper()
	s := newTestServer(t)
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.username = "alice"
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	rec := installRecordingBridges(s)
	return p, rec
}

// contains reports whether needle is a subslice of haystack.
func contains(haystack, needle []byte) bool {
	return bytes.Contains(haystack, needle)
}

// TestHandleReportAbuseInRangeReasonFiresLoggerAndMessageAndProtect verifies
// the happy-path: in-range reason fires the logger bridge, sends the ack
// MessageGame, and sets reportAbuseProtect.
func TestHandleReportAbuseInRangeReasonFiresLoggerAndMessageAndProtect(t *testing.T) {
	p, rec := reportAbuseSetup(t)
	offender := util.ToBase37("bob")
	payload := reportAbusePayload(offender, ReportAbuseItemScamming, false)

	if err := handleReportAbuse(p, payload); err != nil {
		t.Fatalf("handleReportAbuse: %v", err)
	}
	if len(rec.logger) != 1 {
		t.Fatalf("logger: got %d calls, want 1", len(rec.logger))
	}
	got := rec.logger[0]
	if got.offender != util.FromBase37(offender) {
		t.Errorf("logger offender: got %q, want %q", got.offender, util.FromBase37(offender))
	}
	if got.reason != reasonLabel(ReportAbuseItemScamming) {
		t.Errorf("logger reason: got %q, want %q", got.reason, reasonLabel(ReportAbuseItemScamming))
	}
	if !p.reportAbuseProtect {
		t.Error("reportAbuseProtect: must be true after successful report")
	}
}

// TestHandleReportAbuseOutOfRangeReasonFiresBan verifies that reason=12
// (out of range 0..11) fires NotifyPlayerBan with staff="automated" and the
// reporting player's username, does NOT call the logger, and does NOT set
// reportAbuseProtect (TS ReportAbuseHandler.ts:14-17 returns without protect-set).
func TestHandleReportAbuseOutOfRangeReasonFiresBan(t *testing.T) {
	p, rec := reportAbuseSetup(t)
	offender := util.ToBase37("bob")
	// reason=12 is out-of-range (> ReportAbuseRealWorldTrading=11).
	payload := reportAbusePayload(offender, ReportAbuseReason(12), false)

	if err := handleReportAbuse(p, payload); err != nil {
		t.Fatalf("handleReportAbuse: %v", err)
	}
	if len(rec.loginMod) != 1 {
		t.Fatalf("loginMod: got %d calls, want 1", len(rec.loginMod))
	}
	ban := rec.loginMod[0]
	if ban.method != "NotifyPlayerBan" {
		t.Errorf("loginMod[0].method: got %q, want NotifyPlayerBan", ban.method)
	}
	if ban.staff != "automated" {
		t.Errorf("ban.staff: got %q, want automated", ban.staff)
	}
	if ban.username != "alice" {
		t.Errorf("ban.username: got %q, want alice", ban.username)
	}
	// 48h from now; allow generous 5-second window for test timing.
	wantUntil := time.Now().Add(48 * time.Hour)
	diff := ban.until.Sub(wantUntil)
	if diff < -5*time.Second || diff > 5*time.Second {
		t.Errorf("ban.until off by %v; want within 5s of now+48h", diff)
	}
	// Logger must NOT fire.
	if len(rec.logger) != 0 {
		t.Errorf("logger: got %d calls, want 0 (out-of-range path skips logger)", len(rec.logger))
	}
	// No protect-set on out-of-range early-return.
	if p.reportAbuseProtect {
		t.Error("reportAbuseProtect: must remain false on out-of-range early-return (TS fidelity)")
	}
}

// TestHandleReportAbuseProtectGate pins the early-return when
// reportAbuseProtect is already true: no bridge calls at all.
func TestHandleReportAbuseProtectGate(t *testing.T) {
	p, rec := reportAbuseSetup(t)
	p.reportAbuseProtect = true
	offender := util.ToBase37("bob")
	payload := reportAbusePayload(offender, ReportAbuseItemScamming, false)

	if err := handleReportAbuse(p, payload); err != nil {
		t.Fatalf("handleReportAbuse: %v", err)
	}
	if len(rec.loginMod) != 0 {
		t.Errorf("loginMod: got %d calls, want 0 (gated by protect)", len(rec.loginMod))
	}
	if len(rec.logger) != 0 {
		t.Errorf("logger: got %d calls, want 0 (gated by protect)", len(rec.logger))
	}
}

// TestHandleReportAbuseModeratorMuteAllConditionsTrue verifies that when
// moderatorMute=true, staffModLevel > 0, and NodeProduction=true,
// NotifyPlayerMute fires with the correct staff/username AND the logger
// bridge also fires (mute is additive, not short-circuiting).
func TestHandleReportAbuseModeratorMuteAllConditionsTrue(t *testing.T) {
	p, rec := reportAbuseSetup(t)
	p.staffModLevel = 1
	p.client.server.cfg.NodeProduction = true
	offender := util.ToBase37("bob")
	payload := reportAbusePayload(offender, ReportAbuseOffensiveLanguage, true)

	if err := handleReportAbuse(p, payload); err != nil {
		t.Fatalf("handleReportAbuse: %v", err)
	}
	if len(rec.loginMod) != 1 {
		t.Fatalf("loginMod: got %d calls, want 1", len(rec.loginMod))
	}
	mute := rec.loginMod[0]
	if mute.method != "NotifyPlayerMute" {
		t.Errorf("loginMod[0].method: got %q, want NotifyPlayerMute", mute.method)
	}
	if mute.staff != "alice" {
		t.Errorf("mute.staff: got %q, want alice", mute.staff)
	}
	if mute.username != util.FromBase37(offender) {
		t.Errorf("mute.username: got %q, want %q", mute.username, util.FromBase37(offender))
	}
	// Logger must also fire (additive, not exclusive).
	if len(rec.logger) != 1 {
		t.Errorf("logger: got %d calls, want 1 (mute is additive — logger still fires)", len(rec.logger))
	}
	if !p.reportAbuseProtect {
		t.Error("reportAbuseProtect: must be true after full-path completion")
	}
}

// TestHandleReportAbuseModeratorMuteFlagFalseDoesNotFire verifies that when
// moderatorMute=false (even with staffModLevel > 0 and production), no mute fires.
func TestHandleReportAbuseModeratorMuteFlagFalseDoesNotFire(t *testing.T) {
	p, rec := reportAbuseSetup(t)
	p.staffModLevel = 1
	p.client.server.cfg.NodeProduction = true
	offender := util.ToBase37("bob")
	payload := reportAbusePayload(offender, ReportAbuseOffensiveLanguage, false)

	if err := handleReportAbuse(p, payload); err != nil {
		t.Fatalf("handleReportAbuse: %v", err)
	}
	if len(rec.loginMod) != 0 {
		t.Errorf("loginMod: got %d calls, want 0 (moderatorMute=false)", len(rec.loginMod))
	}
	if len(rec.logger) != 1 {
		t.Errorf("logger: got %d calls, want 1", len(rec.logger))
	}
}

// TestHandleReportAbuseModeratorMuteStaffZeroDoesNotFire verifies that when
// staffModLevel == 0, the mute branch is skipped even if moderatorMute=true.
func TestHandleReportAbuseModeratorMuteStaffZeroDoesNotFire(t *testing.T) {
	p, rec := reportAbuseSetup(t)
	p.staffModLevel = 0
	p.client.server.cfg.NodeProduction = true
	offender := util.ToBase37("bob")
	payload := reportAbusePayload(offender, ReportAbuseOffensiveLanguage, true)

	if err := handleReportAbuse(p, payload); err != nil {
		t.Fatalf("handleReportAbuse: %v", err)
	}
	if len(rec.loginMod) != 0 {
		t.Errorf("loginMod: got %d calls, want 0 (staffModLevel=0)", len(rec.loginMod))
	}
	if len(rec.logger) != 1 {
		t.Errorf("logger: got %d calls, want 1", len(rec.logger))
	}
}

// TestHandleReportAbuseModeratorMuteNonProductionDoesNotFire verifies that
// when NodeProduction=false, the mute branch is skipped even if all other
// mute conditions are met.
func TestHandleReportAbuseModeratorMuteNonProductionDoesNotFire(t *testing.T) {
	p, rec := reportAbuseSetup(t)
	p.staffModLevel = 1
	p.client.server.cfg.NodeProduction = false // non-production
	offender := util.ToBase37("bob")
	payload := reportAbusePayload(offender, ReportAbuseOffensiveLanguage, true)

	if err := handleReportAbuse(p, payload); err != nil {
		t.Fatalf("handleReportAbuse: %v", err)
	}
	if len(rec.loginMod) != 0 {
		t.Errorf("loginMod: got %d calls, want 0 (non-production)", len(rec.loginMod))
	}
	if len(rec.logger) != 1 {
		t.Errorf("logger: got %d calls, want 1", len(rec.logger))
	}
}

// TestHandleReportAbuseMessageGameAck pins the exact ack text by reading
// the raw bytes from the conn. The text is embedded in the JagString payload.
func TestHandleReportAbuseMessageGameAck(t *testing.T) {
	// Build the player+server manually so we can capture the clientConn
	// (the other end of the net.Pipe that newTestPlayer creates internally).
	s := newTestServer(t)
	p, clientConn := newTestPlayer(t)
	p.client.server = s
	p.username = "alice"
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	installRecordingBridges(s)

	offender := util.ToBase37("bob")
	payload := reportAbusePayload(offender, ReportAbuseOffensiveLanguage, false)

	received := drainConn(t, clientConn)
	if err := handleReportAbuse(p, payload); err != nil {
		t.Fatalf("handleReportAbuse: %v", err)
	}
	p.client.flushWrite()

	select {
	case got := <-received:
		want := []byte("Thank-you, your abuse report has been received")
		if !contains(got, want) {
			t.Errorf("ack text not found in wire bytes\ngot:  %v\nwant (substring): %q", got, want)
		}
	case <-time.After(200 * time.Millisecond):
		t.Error("timed out waiting for MessageGame ack packet")
	}
}

// TestHandleReportAbuseNilServerNoOp pins the defensive nil-server guard:
// no panic, no bridge calls, no protect-set.
func TestHandleReportAbuseNilServerNoOp(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.client.server = nil
	offender := util.ToBase37("bob")
	payload := reportAbusePayload(offender, ReportAbuseOffensiveLanguage, false)

	if err := handleReportAbuse(p, payload); err != nil {
		t.Errorf("handleReportAbuse nil-server: got err %v", err)
	}
	if p.reportAbuseProtect {
		t.Error("reportAbuseProtect: must remain false on nil-server early-return")
	}
}

// TestHandleReportAbuseRangeBoundaryEdges verifies that reason=0
// (ReportAbuseOffensiveLanguage) and reason=11 (ReportAbuseRealWorldTrading)
// are treated as in-range: no ban fires, logger fires, protect is set.
func TestHandleReportAbuseRangeBoundaryEdges(t *testing.T) {
	offender := util.ToBase37("bob")

	for _, reason := range []ReportAbuseReason{
		ReportAbuseOffensiveLanguage, // 0
		ReportAbuseRealWorldTrading,  // 11
	} {
		t.Run(reasonLabel(reason), func(t *testing.T) {
			p, rec := reportAbuseSetup(t)
			payload := reportAbusePayload(offender, reason, false)

			if err := handleReportAbuse(p, payload); err != nil {
				t.Fatalf("handleReportAbuse: %v", err)
			}
			if len(rec.loginMod) != 0 {
				t.Errorf("loginMod: got %d calls, want 0 (in-range should not ban)", len(rec.loginMod))
			}
			if len(rec.logger) != 1 {
				t.Errorf("logger: got %d calls, want 1", len(rec.logger))
			}
			if !p.reportAbuseProtect {
				t.Error("reportAbuseProtect: must be true for in-range boundary")
			}
		})
	}
}

// reportAbuseSetupWithOnlineOffender extends reportAbuseSetup by also
// adding an offender Player to the server's players with the given
// username. Returns reporter, offender, and recorder.
func reportAbuseSetupWithOnlineOffender(t *testing.T, offenderName string) (*Player, *Player, *recordingBridges) {
	t.Helper()
	reporter, rec := reportAbuseSetup(t)
	s := reporter.client.server
	offender, _ := newTestPlayer(t)
	offender.client.server = s
	offender.username = offenderName
	offender.active = true
	slot := s.players.next()
	if slot == -1 {
		t.Fatal("reportAbuseSetupWithOnlineOffender: world full")
	}
	offender.pid = slot
	s.players.set(slot, offender)
	return reporter, offender, rec
}

// TestHandleReportAbuseMacroingFlipsSubmitInput pins that reason=
// MACROING(6) on an online offender flips offender.submitInput=true.
// Mirrors TS World.notifyPlayerReport (World.ts:2298-2304).
func TestHandleReportAbuseMacroingFlipsSubmitInput(t *testing.T) {
	reporter, offender, _ := reportAbuseSetupWithOnlineOffender(t, "evilbob")
	if offender.submitInput {
		t.Fatal("preflight: offender.submitInput should start false")
	}
	payload := reportAbusePayload(util.ToBase37("evilbob"), ReportAbuseMacroing, false)

	if err := handleReportAbuse(reporter, payload); err != nil {
		t.Fatalf("handleReportAbuse: %v", err)
	}
	if !offender.submitInput {
		t.Error("offender.submitInput: must be true after MACROING report")
	}
}

// TestHandleReportAbuseBugAbuseFlipsSubmitInput pins the same for BUG_ABUSE.
func TestHandleReportAbuseBugAbuseFlipsSubmitInput(t *testing.T) {
	reporter, offender, _ := reportAbuseSetupWithOnlineOffender(t, "evilbob")
	payload := reportAbusePayload(util.ToBase37("evilbob"), ReportAbuseBugAbuse, false)

	if err := handleReportAbuse(reporter, payload); err != nil {
		t.Fatalf("handleReportAbuse: %v", err)
	}
	if !offender.submitInput {
		t.Error("offender.submitInput: must be true after BUG_ABUSE report")
	}
}

// TestHandleReportAbuseNonMacroingDoesNotFlipSubmitInput pins that
// other reasons (e.g. OffensiveLanguage=0) do NOT flip submitInput.
func TestHandleReportAbuseNonMacroingDoesNotFlipSubmitInput(t *testing.T) {
	reporter, offender, _ := reportAbuseSetupWithOnlineOffender(t, "evilbob")
	payload := reportAbusePayload(util.ToBase37("evilbob"), ReportAbuseOffensiveLanguage, false)

	if err := handleReportAbuse(reporter, payload); err != nil {
		t.Fatalf("handleReportAbuse: %v", err)
	}
	if offender.submitInput {
		t.Error("offender.submitInput: must remain false for non-MACROING/BUG_ABUSE reasons")
	}
}

// TestHandleReportAbuseMacroingOfflineOffenderNoOp pins that MACROING
// against an offline offender does not panic and does not affect any
// other state. (TS getPlayerByUsername returns undefined; the handler
// silently skips the submitInput flip.)
func TestHandleReportAbuseMacroingOfflineOffenderNoOp(t *testing.T) {
	reporter, _ := reportAbuseSetup(t) // no offender added
	payload := reportAbusePayload(util.ToBase37("ghost"), ReportAbuseMacroing, false)

	if err := handleReportAbuse(reporter, payload); err != nil {
		t.Fatalf("handleReportAbuse: %v", err)
	}
	// No panic — done.
}
