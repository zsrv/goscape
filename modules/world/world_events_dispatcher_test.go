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
