package world

// Tests for rev-254 A3: session-UUID logging identity — session logs +
// wealth events keyed by session_uuid ONLY. TS refs @2e3bcf43:
// Player.ts:311 (session: string = 'headless'), Player.ts:649-651
// (addSessionLog passes this.session), Player.ts:653-658
// (addWealthEvent stamps session_uuid: this.session),
// World.ts:2234-2243 (addSessionLog row shape, no account_id),
// WealthEvent.ts:7-21 (session_uuid + recipient_session?; account_id /
// account_session / recipient_id removed).
//
// Supersedes the rev-244 B3 account_id-threading pins (this file's
// previous content as account_id_threading_test.go): the NetworkPlayer
// addSessionLog/addWealthEvent overrides were deleted upstream — the
// parent Player impl passes `this.session` for every player, with no
// isClientConnected / 'disconnected' fork.

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/zsrv/goscape/pkg/coordgrid"
	"github.com/zsrv/goscape/pkg/script"
)

// --- SessionLog keyed by session_uuid only ---

// TestSessionLogCarriesSessionUUID pins World.ts:2234-2243 @2e3bcf43:
// every SessionLog record carries the originating player's session UUID
// (and nothing account-shaped — the struct has no account field at all,
// enforced at compile time).
func TestSessionLogCarriesSessionUUID(t *testing.T) {
	s := newTestServer(t)
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.accountID = 42 // present on the player, but NOT threaded into the log
	p.session = "sess-uuid"
	p.level, p.x, p.z = 0, 3200, 3200

	p.AddSessionLog(LoggerEventTypeModerator, "hello")

	if got := len(s.sessionLogs); got != 1 {
		t.Fatalf("sessionLogs: got %d, want 1", got)
	}
	if got := s.sessionLogs[0].SessionUUID; got != "sess-uuid" {
		t.Errorf("SessionUUID: got %q, want sess-uuid", got)
	}
	if got := s.sessionLogs[0].Coord; got != coordgrid.PackCoord(0, 3200, 3200) {
		t.Errorf("Coord: got %d, want %d", got, coordgrid.PackCoord(0, 3200, 3200))
	}
}

// TestSessionLogHeadlessDefault pins the TS field default
// (Player.ts:311 `session: string = 'headless'`): a player whose
// session was never assigned (login-bridge bypass paths leave the Go
// zero value "") logs as 'headless'.
func TestSessionLogHeadlessDefault(t *testing.T) {
	s := newTestServer(t)
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.session = ""

	p.AddSessionLog(LoggerEventTypeEngine, "msg")

	if got := s.sessionLogs[0].SessionUUID; got != "headless" {
		t.Errorf("SessionUUID: got %q, want headless (TS ctor default)", got)
	}
}

// TestSessionLogSessionString_NilClientNoEntry pins the existing
// goscape-defensive behaviour: nil client returns without panicking
// (no log entry). (TS has no equivalent gate — World is module-global.)
func TestSessionLogSessionString_NilClientNoEntry(t *testing.T) {
	p := &Player{}
	// Must not panic; no entry emitted.
	p.AddSessionLog(LoggerEventTypeEngine, "ignored")
}

// --- Coord-log entries carry the session UUID ---

// TestCoordLogCarriesSessionUUID pins that the periodic "Server check in"
// entries emitted by processSessionLogs carry the player's session UUID.
func TestCoordLogCarriesSessionUUID(t *testing.T) {
	s := newTestServer(t)
	rec := installRecordingBridges(s)
	p1, _ := newTestPlayer(t)
	p1.client.server = s
	p1.session = "p1-sess"
	p1.level, p1.x, p1.z = 0, 3200, 3200
	p1.slot = 1
	s.players.set(1, p1)
	s.currentTick = PlayerCoordLogRate

	s.processSessionLogs()

	if len(rec.submittedSessionLogs) != 1 {
		t.Fatalf("bridge calls: got %d, want 1", len(rec.submittedSessionLogs))
	}
	batch := rec.submittedSessionLogs[0]
	if len(batch) != 1 {
		t.Fatalf("batch size: got %d, want 1", len(batch))
	}
	if got := batch[0].SessionUUID; got != "p1-sess" {
		t.Errorf("SessionUUID on coord-log entry: got %q, want p1-sess", got)
	}
	if got := batch[0].Coord; got != coordgrid.PackCoord(0, 3200, 3200) {
		t.Errorf("Coord: got %d, want %d", got, coordgrid.PackCoord(0, 3200, 3200))
	}
}

// --- WealthEvent session_uuid threading ---

// TestWealthEvent_SessionUUIDThreaded pins Player.ts:653-658 +
// WealthEvent.ts:20 @2e3bcf43: AddWealthEvent stamps evt.SessionUUID
// from p.session.
func TestWealthEvent_SessionUUIDThreaded(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.session = "sess-uuid-wealth"

	p.AddWealthEvent(script.WealthEvent{
		EventType:    script.WealthEventTypeDrop,
		AccountValue: 1000,
	})

	if got := len(p.wealthLog); got != 1 {
		t.Fatalf("len(wealthLog): got %d, want 1", got)
	}
	if got := p.wealthLog[0].SessionUUID; got != "sess-uuid-wealth" {
		t.Errorf("SessionUUID: got %q, want sess-uuid-wealth", got)
	}
}

// TestWealthEvent_SessionUUIDHeadless pins the TS Player.session field
// default (Player.ts:311): a player with no assigned session stamps
// 'headless' — even with no client attached (the 244-era
// isClientConnected fork is gone; the parent impl handles all players).
func TestWealthEvent_SessionUUIDHeadless(t *testing.T) {
	p := &Player{session: ""}
	p.AddWealthEvent(script.WealthEvent{EventType: script.WealthEventTypeDrop})

	if got := len(p.wealthLog); got != 1 {
		t.Fatalf("len(wealthLog): got %d, want 1", got)
	}
	if got := p.wealthLog[0].SessionUUID; got != "headless" {
		t.Errorf("SessionUUID headless: got %q, want headless", got)
	}
}

// TestWealthEvent_RecipientSessionPresent pins WealthEvent.ts:13
// @2e3bcf43 (recipient_session?: string — the SOLE recipient key after
// recipient_id's removal). The RecipientSession field must be preserved
// when the caller explicitly sets it.
func TestWealthEvent_RecipientSessionPresent(t *testing.T) {
	p := &Player{}
	p.AddWealthEvent(script.WealthEvent{
		EventType:        script.WealthEventTypeTrade,
		RecipientSession: "to-sess",
	})

	if got := p.wealthLog[0].RecipientSession; got != "to-sess" {
		t.Errorf("RecipientSession: got %q, want to-sess", got)
	}
}

// TestRecipientSession_UsesSessionField pins TS InvOps.ts:454/489/707
// @2e3bcf43 `recipient_session: toPlayer.session`: the counterparty
// seam returns the session field directly ('headless' default) — the
// 244-era 'disconnected' fork is gone.
func TestRecipientSession_UsesSessionField(t *testing.T) {
	p := &Player{session: "live-uuid"}
	if got := p.RecipientSession(); got != "live-uuid" {
		t.Errorf("RecipientSession: got %q, want live-uuid", got)
	}
	p2 := &Player{}
	if got := p2.RecipientSession(); got != "headless" {
		t.Errorf("RecipientSession (unassigned): got %q, want headless (TS ctor default, not 'disconnected')", got)
	}
}

// --- slog bridge keyed by session only ---

// TestSlogBridgeSessionLog_SessionKeyedNoAccountID pins the rev-254 A3
// slog-seam adaptation: SubmitSessionLogs emits "session" per record
// and NO account_id attribute (World.addSessionLog @2e3bcf43 dropped
// the account_id column).
func TestSlogBridgeSessionLog_SessionKeyedNoAccountID(t *testing.T) {
	var buf bytes.Buffer
	parent := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	b := NewSlogLoggerBridge(parent, 10, "main")

	b.SubmitSessionLogs([]SessionLog{
		{SessionUUID: "s", Event: "e", EventType: LoggerEventTypeModerator},
	})

	out := buf.String()
	if !strings.Contains(out, `"session":"s"`) {
		t.Errorf("slog output missing session=s: %s", out)
	}
	if strings.Contains(out, "account_id") {
		t.Errorf("slog output must not carry account_id at 254: %s", out)
	}
}

// TestSlogBridgeReport_SessionKeyed pins the rev-254 A3 'report'
// channel re-key (World.ts:2309-2324 @2e3bcf43 posts
// `session_uuid: player.session`; LoggerThread.ts:45-51 destructures
// session_uuid — username is gone).
func TestSlogBridgeReport_SessionKeyed(t *testing.T) {
	var buf bytes.Buffer
	parent := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	b := NewSlogLoggerBridge(parent, 10, "main")

	p := &Player{session: "rep-sess", username: "alice"}
	b.NotifyPlayerReport(p, "offender", "MACROING")

	out := buf.String()
	if !strings.Contains(out, `"session_uuid":"rep-sess"`) {
		t.Errorf("report record missing session_uuid=rep-sess: %s", out)
	}
	if strings.Contains(out, `"username"`) {
		t.Errorf("report record must not carry username at 254: %s", out)
	}
}

// TestWealthEvent_AppendOrderSessionUUID pins that adding multiple
// events preserves insertion order, session UUID threaded into each.
func TestWealthEvent_AppendOrderSessionUUID(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.session = "s"

	p.AddWealthEvent(script.WealthEvent{EventType: script.WealthEventTypeDrop, AccountValue: 10})
	p.AddWealthEvent(script.WealthEvent{EventType: script.WealthEventTypePVP, AccountValue: 20})

	if got := len(p.wealthLog); got != 2 {
		t.Fatalf("len: got %d, want 2", got)
	}
	for i, e := range p.wealthLog {
		if e.SessionUUID != "s" {
			t.Errorf("wealthLog[%d].SessionUUID: got %q, want s", i, e.SessionUUID)
		}
	}
	if p.wealthLog[0].AccountValue != 10 || p.wealthLog[1].AccountValue != 20 {
		t.Errorf("order preserved: %v", p.wealthLog)
	}
}
