package world

import (
	"context"
	"io"
	"sync"

	"google.golang.org/grpc"

	"github.com/zsrv/goscape/pkg/friendspb"
)

// fakeFriendsClient is a test-only implementation of FriendsClient that
// records each request. WorldConnect appends under a mutex; every other
// RPC pushes into a cap-16 buffered channel via non-blocking select so
// tests assert via channel reads with timeouts.
type fakeFriendsClient struct {
	mu sync.Mutex

	worldConnectCalls []worldConnectCall

	playerLoginReqs    chan *friendspb.PlayerLoginRequest
	playerLogoutReqs   chan *friendspb.PlayerLogoutRequest
	chatSetModeReqs    chan *friendspb.ChatSetModeRequest
	friendlistAddReqs  chan *friendspb.FriendlistAddRequest
	friendlistDelReqs  chan *friendspb.FriendlistDelRequest
	ignorelistAddReqs  chan *friendspb.IgnorelistAddRequest
	ignorelistDelReqs  chan *friendspb.IgnorelistDelRequest
	privateMessageReqs chan *friendspb.PrivateMessageRequest

	// SubscribeUpdates state.
	subscribeReqs []*friendspb.SubscribeUpdatesRequest
	lastStream    *fakeSubscribeStream
	subscribeErr  error // one-shot error returned on next call; tests set to simulate dial failures

	// playerLoginAccepted is the value passed to PlayerLogin's onResponse
	// callback. Defaults to true; set false to simulate cap-rejection.
	// Read under mu.
	playerLoginAccepted bool

	closed bool
}

type worldConnectCall struct {
	WorldID int32
	Profile string
}

// newFakeFriendsClient constructs a fake with buffered channels (capacity 16
// each — large enough that tests don't have to drain in lockstep).
func newFakeFriendsClient() *fakeFriendsClient {
	return &fakeFriendsClient{
		playerLoginReqs:     make(chan *friendspb.PlayerLoginRequest, 16),
		playerLogoutReqs:    make(chan *friendspb.PlayerLogoutRequest, 16),
		chatSetModeReqs:     make(chan *friendspb.ChatSetModeRequest, 16),
		friendlistAddReqs:   make(chan *friendspb.FriendlistAddRequest, 16),
		friendlistDelReqs:   make(chan *friendspb.FriendlistDelRequest, 16),
		ignorelistAddReqs:   make(chan *friendspb.IgnorelistAddRequest, 16),
		ignorelistDelReqs:   make(chan *friendspb.IgnorelistDelRequest, 16),
		privateMessageReqs:  make(chan *friendspb.PrivateMessageRequest, 16),
		playerLoginAccepted: true,
	}
}

// Compile-time assertion that fakeFriendsClient implements FriendsClient.
var _ FriendsClient = (*fakeFriendsClient)(nil)

func (f *fakeFriendsClient) WorldConnect(ctx context.Context, worldID int32, profile string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.worldConnectCalls = append(f.worldConnectCalls, worldConnectCall{WorldID: worldID, Profile: profile})
}

func (f *fakeFriendsClient) PlayerLogin(ctx context.Context, req *friendspb.PlayerLoginRequest, onResponse func(accepted bool)) {
	select {
	case f.playerLoginReqs <- req:
	default:
	}
	f.mu.Lock()
	accepted := f.playerLoginAccepted
	f.mu.Unlock()
	if onResponse != nil {
		onResponse(accepted)
	}
}

func (f *fakeFriendsClient) PlayerLogout(ctx context.Context, req *friendspb.PlayerLogoutRequest) {
	select {
	case f.playerLogoutReqs <- req:
	default:
	}
}

func (f *fakeFriendsClient) ChatSetMode(ctx context.Context, req *friendspb.ChatSetModeRequest) {
	select {
	case f.chatSetModeReqs <- req:
	default:
	}
}

func (f *fakeFriendsClient) FriendlistAdd(ctx context.Context, req *friendspb.FriendlistAddRequest) {
	select {
	case f.friendlistAddReqs <- req:
	default:
	}
}

func (f *fakeFriendsClient) FriendlistDel(ctx context.Context, req *friendspb.FriendlistDelRequest) {
	select {
	case f.friendlistDelReqs <- req:
	default:
	}
}

func (f *fakeFriendsClient) IgnorelistAdd(ctx context.Context, req *friendspb.IgnorelistAddRequest) {
	select {
	case f.ignorelistAddReqs <- req:
	default:
	}
}

func (f *fakeFriendsClient) IgnorelistDel(ctx context.Context, req *friendspb.IgnorelistDelRequest) {
	select {
	case f.ignorelistDelReqs <- req:
	default:
	}
}

func (f *fakeFriendsClient) PrivateMessage(ctx context.Context, req *friendspb.PrivateMessageRequest) {
	select {
	case f.privateMessageReqs <- req:
	default:
	}
}

func (f *fakeFriendsClient) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
	return nil
}

// snapshotWorldConnectCalls returns a copy of the recorded WorldConnect
// invocations under mu.
func (f *fakeFriendsClient) snapshotWorldConnectCalls() []worldConnectCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]worldConnectCall, len(f.worldConnectCalls))
	copy(out, f.worldConnectCalls)
	return out
}

// fakeSubscribeStream is a controllable test impl of
// friendspb.FriendsService_SubscribeUpdatesClient. Tests push updates
// onto recv; Recv drains and returns them. Close ctx (passed to
// SubscribeUpdates) to terminate the stream.
type fakeSubscribeStream struct {
	grpc.ClientStream
	ctx  context.Context
	recv chan *friendspb.FriendsUpdate
}

func newFakeSubscribeStream(ctx context.Context) *fakeSubscribeStream {
	return &fakeSubscribeStream{ctx: ctx, recv: make(chan *friendspb.FriendsUpdate, 16)}
}

func (s *fakeSubscribeStream) Recv() (*friendspb.FriendsUpdate, error) {
	select {
	case u, ok := <-s.recv:
		if !ok {
			return nil, io.EOF
		}
		return u, nil
	case <-s.ctx.Done():
		return nil, s.ctx.Err()
	}
}
func (s *fakeSubscribeStream) Context() context.Context { return s.ctx }

// SubscribeUpdates returns a fakeSubscribeStream the test can push to
// via the field exposed below.
func (f *fakeFriendsClient) SubscribeUpdates(ctx context.Context, req *friendspb.SubscribeUpdatesRequest) (friendspb.FriendsService_SubscribeUpdatesClient, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.subscribeErr != nil {
		err := f.subscribeErr
		f.subscribeErr = nil // one-shot
		return nil, err
	}
	s := newFakeSubscribeStream(ctx)
	f.lastStream = s
	f.subscribeReqs = append(f.subscribeReqs, req)
	return s, nil
}
