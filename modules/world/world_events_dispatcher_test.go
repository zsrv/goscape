package world

import (
	"log/slog"
	"strings"
	"testing"
)

func TestSlogWorldEventsDispatcher_LogsAtInfo(t *testing.T) {
	cases := []struct {
		name string
		want string // substring expected in log line
		call func(d WorldEventsDispatcher)
	}{
		{"Mute", "world event: mute", func(d WorldEventsDispatcher) { d.OnMute(123, 4567) }},
		{"Kick", "world event: kick", func(d WorldEventsDispatcher) { d.OnKick(123) }},
		{"Shutdown", "world event: shutdown", func(d WorldEventsDispatcher) { d.OnShutdown(100) }},
		{"Broadcast", "world event: broadcast", func(d WorldEventsDispatcher) { d.OnBroadcast("hi") }},
		{"Track", "world event: track", func(d WorldEventsDispatcher) { d.OnTrack(123, 1) }},
		{"Reload", "world event: reload", func(d WorldEventsDispatcher) { d.OnReload() }},
		{"ClearLogins", "world event: clear_logins", func(d WorldEventsDispatcher) { d.OnClearLogins() }},
		{"ClearLogouts", "world event: clear_logouts", func(d WorldEventsDispatcher) { d.OnClearLogouts() }},
		{"QueueScript", "world event: queue_script", func(d WorldEventsDispatcher) { d.OnQueueScript("dbg", 123) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			buf := &syncBuffer{}
			d := newSlogWorldEventsDispatcher(slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelInfo})))
			tc.call(d)
			if !strings.Contains(buf.String(), tc.want) {
				t.Fatalf("log missing %q; got: %s", tc.want, buf.String())
			}
		})
	}
}

// recordingWorldStateOps captures WorldStateOps calls for composition
// tests. Each method is single-call; tests assert via the captured
// fields. Unused fields stay at zero value.
type recordingWorldStateOps struct {
	muteU37, kickU37, trackU37 uint64
	muteMs                     int64
	trackState                 int32
	shutdownTicks              int32
	broadcastMsg               string
	reloadCalled               bool
	clearLoginsCalled          bool
	clearLogoutsCalled         bool
}

func (r *recordingWorldStateOps) SetPlayerMute(u37 uint64, ms int64) {
	r.muteU37, r.muteMs = u37, ms
}
func (r *recordingWorldStateOps) KickPlayer(u37 uint64)     { r.kickU37 = u37 }
func (r *recordingWorldStateOps) RelayShutdown(d int32)     { r.shutdownTicks = d }
func (r *recordingWorldStateOps) BroadcastMessage(m string) { r.broadcastMsg = m }
func (r *recordingWorldStateOps) SetPlayerInputTracking(u37 uint64, s int32) {
	r.trackU37, r.trackState = u37, s
}
func (r *recordingWorldStateOps) RelayReload()  { r.reloadCalled = true }
func (r *recordingWorldStateOps) ClearLogins()  { r.clearLoginsCalled = true }
func (r *recordingWorldStateOps) ClearLogouts() { r.clearLogoutsCalled = true }

// TestActionWorldEventsDispatcher_RoutesToOpsAndInner pins that each
// wired On* method calls (a) the WorldStateOps method with the right
// args, and (b) the inner WorldEventsDispatcher (composition).
func TestActionWorldEventsDispatcher_RoutesToOpsAndInner(t *testing.T) {
	buf := &syncBuffer{}
	innerLog := slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	inner := newSlogWorldEventsDispatcher(innerLog)
	ops := &recordingWorldStateOps{}
	d := newActionWorldEventsDispatcher(inner, ops, innerLog)

	d.OnMute(11, 22)
	if ops.muteU37 != 11 || ops.muteMs != 22 {
		t.Errorf("OnMute → ops: got (%d,%d), want (11,22)", ops.muteU37, ops.muteMs)
	}
	if !strings.Contains(buf.String(), "world event: mute") {
		t.Errorf("OnMute: inner slog did not fire")
	}

	d.OnKick(33)
	if ops.kickU37 != 33 {
		t.Errorf("OnKick → ops: got %d, want 33", ops.kickU37)
	}

	d.OnShutdown(44)
	if ops.shutdownTicks != 44 {
		t.Errorf("OnShutdown → ops: got %d, want 44", ops.shutdownTicks)
	}

	d.OnBroadcast("hello")
	if ops.broadcastMsg != "hello" {
		t.Errorf("OnBroadcast → ops: got %q, want %q", ops.broadcastMsg, "hello")
	}

	d.OnTrack(55, 1)
	if ops.trackU37 != 55 || ops.trackState != 1 {
		t.Errorf("OnTrack → ops: got (%d,%d), want (55,1)", ops.trackU37, ops.trackState)
	}

	d.OnReload()
	if !ops.reloadCalled {
		t.Errorf("OnReload → ops: not called")
	}

	d.OnClearLogins()
	if !ops.clearLoginsCalled {
		t.Errorf("OnClearLogins → ops: not called")
	}

	d.OnClearLogouts()
	if !ops.clearLogoutsCalled {
		t.Errorf("OnClearLogouts → ops: not called")
	}
}

// TestActionWorldEventsDispatcher_QueueScriptIsSlogWarnOnly pins that
// QUEUESCRIPT does NOT call any WorldStateOps method — the runescript
// runtime gap is documented at NAI-S5B-D-NO-RUNESCRIPT-RUNTIME.
// The inner slog dispatcher still fires; the action layer logs Warn.
func TestActionWorldEventsDispatcher_QueueScriptIsSlogWarnOnly(t *testing.T) {
	innerBuf := &syncBuffer{}
	innerLog := slog.New(slog.NewTextHandler(innerBuf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	inner := newSlogWorldEventsDispatcher(innerLog)

	actionBuf := &syncBuffer{}
	actionLog := slog.New(slog.NewTextHandler(actionBuf, &slog.HandlerOptions{Level: slog.LevelWarn}))

	ops := &recordingWorldStateOps{}
	d := newActionWorldEventsDispatcher(inner, ops, actionLog)

	d.OnQueueScript("dbg_dump", 99)

	// ops surface for QUEUESCRIPT must NOT exist — recording impl has
	// no method that would be set. (Compile-time guaranteed by the
	// interface lacking a QueueScript method.) The test below verifies
	// the slog-warn was emitted at the action layer.
	if !strings.Contains(actionBuf.String(), "RELAY_QUEUESCRIPT") {
		t.Fatalf("QUEUESCRIPT: action-layer Warn log missing; got: %s", actionBuf.String())
	}
	// Inner dispatcher still logs at Info — composition preserves slice-5a behavior.
	if !strings.Contains(innerBuf.String(), "world event: queue_script") {
		t.Fatalf("QUEUESCRIPT: inner Info log missing; got: %s", innerBuf.String())
	}
}
