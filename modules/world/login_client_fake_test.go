package world

import (
	"context"
	"sync"

	"github.com/zsrv/goscape/pkg/loginpb"
)

// fakeLoginClient is a test-only implementation of LoginClient that records
// each request and serves canned responses. Sync RPCs (PlayerLogin /
// PlayerLogout) use last-call-wins recording; async/fire-and-forget RPCs
// (PlayerAutosave / PlayerForceLogout) push to buffered channels so tests
// can select-with-timeout.
type fakeLoginClient struct {
	mu sync.Mutex

	worldStartupCalls []worldStartupCall

	lastPlayerLoginReq  *loginpb.PlayerLoginRequest
	lastPlayerLogoutReq *loginpb.PlayerLogoutRequest

	autosaveReqs    chan *loginpb.PlayerAutosaveRequest
	forceLogoutReqs chan *loginpb.PlayerForceLogoutRequest
	playerBanReqs   chan *loginpb.PlayerBanRequest
	playerMuteReqs  chan *loginpb.PlayerMuteRequest

	playerLoginResp  *loginpb.PlayerLoginResponse
	playerLoginErr   error
	playerLogoutResp *loginpb.PlayerLogoutResponse
	playerLogoutErr  error

	// playerLogoutFired is sent on after lastPlayerLogoutReq is recorded.
	// removePlayerOnTick spawns its own goroutine that calls PlayerLogout,
	// so tests need a synchronisation point that doesn't depend on the
	// goroutine winning the race.
	playerLogoutFired chan struct{}

	closed bool
}

type worldStartupCall struct {
	NodeID  int32
	Profile string
}

// newFakeLoginClient constructs a fake with buffered channels (capacity 16
// each — large enough that tests don't have to drain in lockstep).
func newFakeLoginClient() *fakeLoginClient {
	return &fakeLoginClient{
		autosaveReqs:      make(chan *loginpb.PlayerAutosaveRequest, 16),
		forceLogoutReqs:   make(chan *loginpb.PlayerForceLogoutRequest, 16),
		playerBanReqs:     make(chan *loginpb.PlayerBanRequest, 16),
		playerMuteReqs:    make(chan *loginpb.PlayerMuteRequest, 16),
		playerLogoutFired: make(chan struct{}, 16),
	}
}

// Compile-time assertion that fakeLoginClient implements LoginClient.
var _ LoginClient = (*fakeLoginClient)(nil)

func (f *fakeLoginClient) WorldStartup(ctx context.Context, nodeID int32, profile string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.worldStartupCalls = append(f.worldStartupCalls, worldStartupCall{NodeID: nodeID, Profile: profile})
}

func (f *fakeLoginClient) PlayerLogin(ctx context.Context, req *loginpb.PlayerLoginRequest) (*loginpb.PlayerLoginResponse, error) {
	f.mu.Lock()
	f.lastPlayerLoginReq = req
	resp := f.playerLoginResp
	err := f.playerLoginErr
	f.mu.Unlock()
	return resp, err
}

func (f *fakeLoginClient) PlayerLogout(ctx context.Context, req *loginpb.PlayerLogoutRequest) (*loginpb.PlayerLogoutResponse, error) {
	f.mu.Lock()
	f.lastPlayerLogoutReq = req
	resp := f.playerLogoutResp
	err := f.playerLogoutErr
	f.mu.Unlock()
	select {
	case f.playerLogoutFired <- struct{}{}:
	default:
	}
	return resp, err
}

func (f *fakeLoginClient) PlayerAutosave(ctx context.Context, req *loginpb.PlayerAutosaveRequest) {
	select {
	case f.autosaveReqs <- req:
	default:
		// Channel full — drop. Tests should assert via channel reads, so a
		// full channel means the test isn't keeping pace; surface by leaving
		// the unread requests visible.
	}
}

func (f *fakeLoginClient) PlayerForceLogout(ctx context.Context, req *loginpb.PlayerForceLogoutRequest) {
	select {
	case f.forceLogoutReqs <- req:
	default:
	}
}

func (f *fakeLoginClient) PlayerBan(ctx context.Context, req *loginpb.PlayerBanRequest) {
	select {
	case f.playerBanReqs <- req:
	default:
	}
}

func (f *fakeLoginClient) PlayerMute(ctx context.Context, req *loginpb.PlayerMuteRequest) {
	select {
	case f.playerMuteReqs <- req:
	default:
	}
}

func (f *fakeLoginClient) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
	return nil
}

// snapshotPlayerLoginReq returns a copy of the last captured PlayerLoginRequest
// without holding the mutex across the test's assertions.
func (f *fakeLoginClient) snapshotPlayerLoginReq() *loginpb.PlayerLoginRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.lastPlayerLoginReq
}

// snapshotPlayerLogoutReq is the PlayerLogout equivalent of
// snapshotPlayerLoginReq.
func (f *fakeLoginClient) snapshotPlayerLogoutReq() *loginpb.PlayerLogoutRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.lastPlayerLogoutReq
}
