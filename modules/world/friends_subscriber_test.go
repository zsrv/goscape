package world

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/zsrv/goscape/pkg/friendspb"
)

// recordingFriendsDispatcher captures dispatch calls under mu.
type recordingFriendsDispatcher struct {
	mu      sync.Mutex
	friend  []friendlistCall
	ignore  []ignorelistCall
	private []privateCall
}

type friendlistCall struct {
	Viewer  uint64
	Entries []*friendspb.FriendEntry
}
type ignorelistCall struct {
	Viewer  uint64
	Ignored []uint64
}
type privateCall struct {
	Target, From uint64
	StaffLvl     int32
	PmId         uint32
	Chat         string
}

func (d *recordingFriendsDispatcher) OnFriendlistUpdate(v uint64, e []*friendspb.FriendEntry) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.friend = append(d.friend, friendlistCall{Viewer: v, Entries: e})
}
func (d *recordingFriendsDispatcher) OnIgnorelistUpdate(v uint64, ig []uint64) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.ignore = append(d.ignore, ignorelistCall{Viewer: v, Ignored: ig})
}
func (d *recordingFriendsDispatcher) OnPrivateMessage(target, from uint64, staffLvl int32, pmId uint32, chat string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.private = append(d.private, privateCall{Target: target, From: from, StaffLvl: staffLvl, PmId: pmId, Chat: chat})
}

func (d *recordingFriendsDispatcher) friendCalls() []friendlistCall {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]friendlistCall(nil), d.friend...)
}

// waitForStream polls fc.lastStream under fc.mu until it is non-nil or
// the deadline is exceeded, returning the stream (or nil on timeout).
func waitForStream(t *testing.T, fc *fakeFriendsClient, deadline time.Time) *fakeSubscribeStream {
	t.Helper()
	for {
		fc.mu.Lock()
		s := fc.lastStream
		fc.mu.Unlock()
		if s != nil {
			return s
		}
		if time.Now().After(deadline) {
			return nil
		}
		time.Sleep(time.Millisecond)
	}
}

func TestFriendsSubscriber_DispatchesFriendlist(t *testing.T) {
	fc := newFakeFriendsClient()
	disp := &recordingFriendsDispatcher{}
	sub := newFriendsSubscriber(fc, 1, 42, disp, discardLogger())

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() { defer close(done); sub.run(ctx) }()

	// Wait until the fake client has produced a stream.
	stream := waitForStream(t, fc, time.Now().Add(2*time.Second))
	if stream == nil {
		t.Fatalf("fake stream never created")
	}

	// Push a FriendlistUpdate.
	stream.recv <- &friendspb.FriendsUpdate{
		Update: &friendspb.FriendsUpdate_Friendlist{
			Friendlist: &friendspb.FriendlistUpdate{
				Entries: []*friendspb.FriendEntry{{WorldId: 7, Username37: 99}},
			},
		},
	}

	// Wait for dispatch.
	deadline := time.Now().Add(2 * time.Second)
	for len(disp.friendCalls()) == 0 {
		if time.Now().After(deadline) {
			t.Fatalf("dispatcher never invoked")
		}
		time.Sleep(time.Millisecond)
	}

	calls := disp.friendCalls()
	if calls[0].Viewer != 42 {
		t.Fatalf("viewer = %d, want 42", calls[0].Viewer)
	}
	if len(calls[0].Entries) != 1 || calls[0].Entries[0].WorldId != 7 || calls[0].Entries[0].Username37 != 99 {
		t.Fatalf("entries = %v, want [(7, 99)]", calls[0].Entries)
	}

	cancel()
	<-done
}

func TestFriendsSubscriber_CtxCancelStopsCleanly(t *testing.T) {
	fc := newFakeFriendsClient()
	disp := &recordingFriendsDispatcher{}
	sub := newFriendsSubscriber(fc, 1, 42, disp, discardLogger())

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() { defer close(done); sub.run(ctx) }()

	if waitForStream(t, fc, time.Now().Add(2*time.Second)) == nil {
		t.Fatalf("fake stream never created")
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("sub.run did not return on ctx cancel")
	}
}

func TestFriendsSubscriber_EOFTriggersReconnect(t *testing.T) {
	fc := newFakeFriendsClient()
	disp := &recordingFriendsDispatcher{}
	sub := newFriendsSubscriber(fc, 1, 42, disp, discardLogger())

	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	done := make(chan struct{})
	go func() { defer close(done); sub.run(ctx) }()

	// First stream appears.
	firstStream := waitForStream(t, fc, time.Now().Add(2*time.Second))
	if firstStream == nil {
		t.Fatalf("first stream never created")
	}

	// Simulate clean EOF.
	close(firstStream.recv)

	// Wait for a new stream to appear (note: backoff is 1s first time;
	// test waits longer).
	deadline := time.Now().Add(3 * time.Second)
	for {
		fc.mu.Lock()
		cur := fc.lastStream
		fc.mu.Unlock()
		if cur != firstStream {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("supervisor did not reconnect after EOF")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestNextBackoff_DoublesAndCaps(t *testing.T) {
	got := nextBackoff(time.Second)
	if got != 2*time.Second {
		t.Errorf("nextBackoff(1s) = %v, want 2s", got)
	}
	got = nextBackoff(16 * time.Second)
	if got != friendsSubscriberBackoffMax {
		t.Errorf("nextBackoff(16s) = %v, want %v", got, friendsSubscriberBackoffMax)
	}
	got = nextBackoff(friendsSubscriberBackoffMax)
	if got != friendsSubscriberBackoffMax {
		t.Errorf("nextBackoff(max) = %v, want %v (cap)", got, friendsSubscriberBackoffMax)
	}
}

func TestFriendsSubscriber_DispatchesIgnorelist(t *testing.T) {
	fc := newFakeFriendsClient()
	disp := &recordingFriendsDispatcher{}
	sub := newFriendsSubscriber(fc, 1, 42, disp, discardLogger())

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() { defer close(done); sub.run(ctx) }()
	t.Cleanup(func() { cancel(); <-done })

	stream := waitForStream(t, fc, time.Now().Add(2*time.Second))
	if stream == nil {
		t.Fatalf("fake stream never created")
	}

	stream.recv <- &friendspb.FriendsUpdate{
		Update: &friendspb.FriendsUpdate_Ignorelist{
			Ignorelist: &friendspb.IgnorelistUpdate{Username37: []uint64{100, 200}},
		},
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		disp.mu.Lock()
		n := len(disp.ignore)
		disp.mu.Unlock()
		if n > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("dispatcher OnIgnorelistUpdate never called")
		}
		time.Sleep(time.Millisecond)
	}

	disp.mu.Lock()
	defer disp.mu.Unlock()
	if disp.ignore[0].Viewer != 42 {
		t.Errorf("viewer = %d, want 42", disp.ignore[0].Viewer)
	}
	if len(disp.ignore[0].Ignored) != 2 {
		t.Errorf("ignored len = %d, want 2", len(disp.ignore[0].Ignored))
	}
}

// Smoke test that runOnce returns the error from SubscribeUpdates when
// the client fails to dial. (Exercises the supervisor's error path.)
func TestFriendsSubscriber_RunOnce_DialErrorPropagates(t *testing.T) {
	fc := newFakeFriendsClient()
	fc.subscribeErr = io.ErrUnexpectedEOF
	sub := newFriendsSubscriber(fc, 1, 42, &recordingFriendsDispatcher{}, discardLogger())

	err := sub.runOnce(t.Context())
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("err = %v, want io.ErrUnexpectedEOF", err)
	}
}
