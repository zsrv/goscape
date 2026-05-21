package world

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/zsrv/goscape/pkg/friendspb"
)

// recordingWorldEventsDispatcher captures all received events.
type recordingWorldEventsDispatcher struct {
	mute chan struct {
		U uint64
		M int64
	}
	kick      chan uint64
	shutdown  chan int32
	broadcast chan string
	track     chan struct {
		U uint64
		S int32
	}
	reload       chan struct{}
	clearLogins  chan struct{}
	clearLogouts chan struct{}
	queueScript  chan struct {
		Name string
		U    uint64
	}
}

func newRecordingWorldEventsDispatcher() *recordingWorldEventsDispatcher {
	return &recordingWorldEventsDispatcher{
		mute: make(chan struct {
			U uint64
			M int64
		}, 8),
		kick:      make(chan uint64, 8),
		shutdown:  make(chan int32, 8),
		broadcast: make(chan string, 8),
		track: make(chan struct {
			U uint64
			S int32
		}, 8),
		reload:       make(chan struct{}, 8),
		clearLogins:  make(chan struct{}, 8),
		clearLogouts: make(chan struct{}, 8),
		queueScript: make(chan struct {
			Name string
			U    uint64
		}, 8),
	}
}

func (r *recordingWorldEventsDispatcher) OnMute(u uint64, m int64) {
	r.mute <- struct {
		U uint64
		M int64
	}{u, m}
}
func (r *recordingWorldEventsDispatcher) OnKick(u uint64)      { r.kick <- u }
func (r *recordingWorldEventsDispatcher) OnShutdown(d int32)   { r.shutdown <- d }
func (r *recordingWorldEventsDispatcher) OnBroadcast(m string) { r.broadcast <- m }
func (r *recordingWorldEventsDispatcher) OnTrack(u uint64, s int32) {
	r.track <- struct {
		U uint64
		S int32
	}{u, s}
}
func (r *recordingWorldEventsDispatcher) OnReload()       { r.reload <- struct{}{} }
func (r *recordingWorldEventsDispatcher) OnClearLogins()  { r.clearLogins <- struct{}{} }
func (r *recordingWorldEventsDispatcher) OnClearLogouts() { r.clearLogouts <- struct{}{} }
func (r *recordingWorldEventsDispatcher) OnQueueScript(n string, u uint64) {
	r.queueScript <- struct {
		Name string
		U    uint64
	}{n, u}
}

func TestWorldEventsSubscriber_DispatchRouting(t *testing.T) {
	fake := newFakeFriendsClient()
	disp := newRecordingWorldEventsDispatcher()
	sub := newWorldEventsSubscriber(fake, 7, disp, discardLogger())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		sub.run(ctx)
		close(done)
	}()

	// Wait for SubscribeWorldEvents to be called and stream installed.
	waitForWorldStream(t, fake)
	stream := fake.lastWorldStream

	stream.recv <- &friendspb.WorldEvent{Event: &friendspb.WorldEvent_Mute{Mute: &friendspb.MuteEvent{Username37: 1, MutedUntilMs: 9}}}
	got := <-disp.mute
	if got.U != 1 || got.M != 9 {
		t.Fatalf("got %+v", got)
	}

	stream.recv <- &friendspb.WorldEvent{Event: &friendspb.WorldEvent_Kick{Kick: &friendspb.KickEvent{Username37: 2}}}
	if u := <-disp.kick; u != 2 {
		t.Fatalf("kick = %d", u)
	}

	stream.recv <- &friendspb.WorldEvent{Event: &friendspb.WorldEvent_Shutdown{Shutdown: &friendspb.ShutdownEvent{DurationTicks: 10}}}
	if d := <-disp.shutdown; d != 10 {
		t.Fatalf("shutdown = %d", d)
	}

	stream.recv <- &friendspb.WorldEvent{Event: &friendspb.WorldEvent_Broadcast{Broadcast: &friendspb.BroadcastEvent{Message: "hi"}}}
	if m := <-disp.broadcast; m != "hi" {
		t.Fatalf("broadcast = %q", m)
	}

	stream.recv <- &friendspb.WorldEvent{Event: &friendspb.WorldEvent_Track{Track: &friendspb.TrackEvent{Username37: 3, State: 1}}}
	if g := <-disp.track; g.U != 3 || g.S != 1 {
		t.Fatalf("track = %+v", g)
	}

	stream.recv <- &friendspb.WorldEvent{Event: &friendspb.WorldEvent_Reload{Reload: &friendspb.ReloadEvent{}}}
	<-disp.reload

	stream.recv <- &friendspb.WorldEvent{Event: &friendspb.WorldEvent_ClearLogins{ClearLogins: &friendspb.ClearLoginsEvent{}}}
	<-disp.clearLogins

	stream.recv <- &friendspb.WorldEvent{Event: &friendspb.WorldEvent_ClearLogouts{ClearLogouts: &friendspb.ClearLogoutsEvent{}}}
	<-disp.clearLogouts

	stream.recv <- &friendspb.WorldEvent{Event: &friendspb.WorldEvent_QueueScript{QueueScript: &friendspb.QueueScriptEvent{ScriptName: "dbg", Username37: 4}}}
	if g := <-disp.queueScript; g.Name != "dbg" || g.U != 4 {
		t.Fatalf("queueScript = %+v", g)
	}

	cancel()
	<-done
}

func TestWorldEventsSubscriber_EOFLogsAtInfo(t *testing.T) {
	fake := newFakeFriendsClient()
	disp := newRecordingWorldEventsDispatcher()
	buf := &syncBuffer{}
	log := slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	sub := newWorldEventsSubscriber(fake, 7, disp, log)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { sub.run(ctx); close(done) }()

	waitForWorldStream(t, fake)
	// Close stream channel → Recv returns io.EOF.
	close(fake.lastWorldStream.recv)

	// Wait until "EOF; reconnecting" appears (supervisor logs Info).
	waitForLog(t, buf, "world events subscriber EOF; reconnecting")

	cancel()
	<-done
	if strings.Contains(buf.String(), "level=WARN") {
		t.Fatalf("EOF should log at Info, not Warn; got: %s", buf.String())
	}
}

func TestWorldEventsSubscriber_ReconnectBackoff(t *testing.T) {
	fake := newFakeFriendsClient()
	disp := newRecordingWorldEventsDispatcher()
	fake.mu.Lock()
	fake.worldSubscribeErr = errors.New("dial fail")
	fake.mu.Unlock()

	buf := &syncBuffer{}
	log := slog.New(slog.NewTextHandler(buf, nil))
	sub := newWorldEventsSubscriber(fake, 7, disp, log)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { sub.run(ctx); close(done) }()

	// First run returns immediately with the dial error; supervisor sleeps
	// 1s before retrying. Wait for the first "disconnected; reconnecting"
	// log line, then cancel.
	waitForLog(t, buf, "world events subscriber disconnected; reconnecting")

	cancel()
	<-done
}

// waitForWorldStream polls fake.lastWorldStream up to 2s.
func waitForWorldStream(t *testing.T, fake *fakeFriendsClient) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		fake.mu.Lock()
		s := fake.lastWorldStream
		fake.mu.Unlock()
		if s != nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timeout waiting for world events subscriber to open stream")
}

// waitForLog polls buf for a substring up to 2s.
func waitForLog(t *testing.T, buf *syncBuffer, want string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(buf.String(), want) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for log %q; got: %s", want, buf.String())
}
