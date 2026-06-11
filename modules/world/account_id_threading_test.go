package world

// Tests for Task 13: account_id threading — session logs + wealth events
// re-keyed. TS refs: Player.ts:306, Player.ts:633-635,
// NetworkPlayer.ts:252-254, SessionLog.ts:1-2, WealthEvent.ts:10-22,
// World.ts:2250-2261, World.ts:2276-2284.

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/zsrv/goscape/pkg/coordgrid"
	"github.com/zsrv/goscape/pkg/script"
)

// --- SessionLog.AccountID ---

// TestSessionLogCarriesAccountID pins SessionLog.ts:1-2 + World.ts:2252:
// every SessionLog record must carry the account_id of the originating player.
func TestSessionLogCarriesAccountID(t *testing.T) {
	s := newTestServer(t)
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.accountID = 42
	p.session = "sess-acct"
	p.level, p.x, p.z = 0, 3200, 3200

	p.AddSessionLog(LoggerEventTypeModerator, "hello")

	if got := len(s.sessionLogs); got != 1 {
		t.Fatalf("sessionLogs: got %d, want 1", got)
	}
	if got := s.sessionLogs[0].AccountID; got != 42 {
		t.Errorf("AccountID: got %d, want 42", got)
	}
}

// TestSessionLogAccountIDFlowsFromPlayer pins that AddSessionLog threads
// whatever accountID is on the player into the SessionLog record — the
// identity value flows through unchanged. (TS Player.ts:306 initialises
// account_id to -1 as the "DB not yet resolved" sentinel; Go's zero-value
// is 0 on login-bypass paths — documented at client.go:89-91. The
// meaningful invariant is pass-through fidelity, not the sentinel value.)
func TestSessionLogAccountIDFlowsFromPlayer(t *testing.T) {
	s := newTestServer(t)
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.accountID = 999
	p.session = "s"

	p.AddSessionLog(LoggerEventTypeEngine, "msg")

	if got := s.sessionLogs[0].AccountID; got != 999 {
		t.Errorf("AccountID: got %d, want 999", got)
	}
}

// --- Session string fork semantics ---

// TestSessionLogSessionString_WithClient pins NetworkPlayer.ts:252-254:
// when the player has a live client, AddSessionLog uses the session UUID
// (the per-login correlation key, mapped to p.session in goscape).
func TestSessionLogSessionString_WithClient(t *testing.T) {
	s := newTestServer(t)
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.session = "live-uuid"

	p.AddSessionLog(LoggerEventTypeEngine, "x")

	if got := s.sessionLogs[0].SessionUUID; got != "live-uuid" {
		t.Errorf("SessionUUID: got %q, want live-uuid", got)
	}
}

// TestSessionLogSessionString_NilClient pins the existing behaviour:
// nil client returns without panicking (no log entry).
// (Already covered by TestPlayerAddSessionLogNilClient; kept for
// completeness of the fork contract.)
func TestSessionLogSessionString_NilClientNoEntry(t *testing.T) {
	p := &Player{accountID: 1}
	// Must not panic; no entry emitted.
	p.AddSessionLog(LoggerEventTypeEngine, "ignored")
}

// --- Coord-log entries carry AccountID ---

// TestCoordLogCarriesAccountID pins that the periodic "Server check in"
// entries emitted by processSessionLogs also carry the player's accountID.
func TestCoordLogCarriesAccountID(t *testing.T) {
	s := newTestServer(t)
	rec := installRecordingBridges(s)
	p1, _ := newTestPlayer(t)
	p1.client.server = s
	p1.accountID = 77
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
	if got := batch[0].AccountID; got != 77 {
		t.Errorf("AccountID on coord-log entry: got %d, want 77", got)
	}
	if got := batch[0].Coord; got != coordgrid.PackCoord(0, 3200, 3200) {
		t.Errorf("Coord: got %d, want %d", got, coordgrid.PackCoord(0, 3200, 3200))
	}
}

// --- WealthEvent AccountID / AccountSession threading ---

// TestWealthEvent_AccountIDThreaded pins WealthEvent.ts:21 + Player.ts:640:
// AddWealthEvent must stamp evt.AccountID from p.accountID.
func TestWealthEvent_AccountIDThreaded(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.accountID = 55

	p.AddWealthEvent(script.WealthEvent{
		EventType:    script.WealthEventTypeDrop,
		AccountValue: 1000,
	})

	if got := len(p.wealthLog); got != 1 {
		t.Fatalf("len(wealthLog): got %d, want 1", got)
	}
	if got := p.wealthLog[0].AccountID; got != 55 {
		t.Errorf("AccountID: got %d, want 55 (from p.accountID)", got)
	}
}

// TestWealthEvent_AccountSessionWithClient pins WealthEvent.ts:22 +
// NetworkPlayer.ts:260: when the player has a live client, AccountSession
// must be the session UUID string.
func TestWealthEvent_AccountSessionWithClient(t *testing.T) {
	p, c := newTestPlayer(t)
	_ = c
	p.session = "sess-uuid-wealth"

	p.AddWealthEvent(script.WealthEvent{EventType: script.WealthEventTypePVP})

	if got := p.wealthLog[0].AccountSession; got != "sess-uuid-wealth" {
		t.Errorf("AccountSession with client: got %q, want sess-uuid-wealth", got)
	}
}

// TestWealthEvent_AccountSessionHeadless pins Player.ts:641: when the
// player has no client (headless/test context), AccountSession is "headless".
func TestWealthEvent_AccountSessionHeadless(t *testing.T) {
	p := &Player{accountID: 10, session: ""}
	p.AddWealthEvent(script.WealthEvent{EventType: script.WealthEventTypeDrop})

	if got := len(p.wealthLog); got != 1 {
		t.Fatalf("len(wealthLog): got %d, want 1", got)
	}
	if got := p.wealthLog[0].AccountSession; got != "headless" {
		t.Errorf("AccountSession headless: got %q, want headless", got)
	}
}

// TestWealthEvent_RecipientIDPresent pins WealthEvent.ts:13
// (recipient_id?: number). The RecipientID field must be preserved when
// the caller explicitly sets it (non-zero value passes through).
func TestWealthEvent_RecipientIDPresent(t *testing.T) {
	p := &Player{accountID: 10}
	p.AddWealthEvent(script.WealthEvent{
		EventType:   script.WealthEventTypeTrade,
		RecipientID: 99,
	})

	if got := p.wealthLog[0].RecipientID; got != 99 {
		t.Errorf("RecipientID: got %d, want 99", got)
	}
}

// --- slog bridge emits account_id ---

// TestSlogBridgeSessionLog_EmitsAccountID pins rev-244 B3 adaptation to
// the slog seam: SubmitSessionLogs must emit "account_id" in each record.
func TestSlogBridgeSessionLog_EmitsAccountID(t *testing.T) {
	var buf bytes.Buffer
	parent := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	b := NewSlogLoggerBridge(parent, 10, "main")

	b.SubmitSessionLogs([]SessionLog{
		{AccountID: 42, SessionUUID: "s", Event: "e", EventType: LoggerEventTypeModerator},
	})

	out := buf.String()
	if !strings.Contains(out, `"account_id":42`) {
		t.Errorf("slog output missing account_id=42: %s", out)
	}
	if !strings.Contains(out, `"session":"s"`) {
		t.Errorf("slog output missing session=s: %s", out)
	}
}

// TestWealthEvent_AppendOrderPreserved pins that adding multiple events
// still preserves insertion order, account_id threaded into each.
func TestWealthEvent_AppendOrderAccountID(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.accountID = 100
	p.session = "s"

	p.AddWealthEvent(script.WealthEvent{EventType: script.WealthEventTypeDrop, AccountValue: 10})
	p.AddWealthEvent(script.WealthEvent{EventType: script.WealthEventTypePVP, AccountValue: 20})

	if got := len(p.wealthLog); got != 2 {
		t.Fatalf("len: got %d, want 2", got)
	}
	for i, e := range p.wealthLog {
		if e.AccountID != 100 {
			t.Errorf("wealthLog[%d].AccountID: got %d, want 100", i, e.AccountID)
		}
	}
	if p.wealthLog[0].AccountValue != 10 || p.wealthLog[1].AccountValue != 20 {
		t.Errorf("order preserved: %v", p.wealthLog)
	}
}
