package world

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zsrv/goscape/pkg/loginpb"
)

// flakyLoginClient is a test-only LoginClient whose PlayerLogout fails the
// first failFirst calls, then succeeds. Used to pin the arch-28.5 bounded
// retry in sendPlayerLogoutWithRetry. The other LoginClient methods are
// unused by these tests and stubbed as no-ops.
type flakyLoginClient struct {
	failFirst int32 // number of leading calls that fail
	calls     atomic.Int32
}

var _ LoginClient = (*flakyLoginClient)(nil)

func (f *flakyLoginClient) WorldStartup(ctx context.Context, nodeID int32, profile string) {}

func (f *flakyLoginClient) PlayerLogin(ctx context.Context, req *loginpb.PlayerLoginRequest) (*loginpb.PlayerLoginResponse, error) {
	return nil, nil
}

func (f *flakyLoginClient) PlayerLogout(ctx context.Context, req *loginpb.PlayerLogoutRequest) (*loginpb.PlayerLogoutResponse, error) {
	if f.calls.Add(1) <= f.failFirst {
		return nil, errors.New("login server restarting")
	}
	return &loginpb.PlayerLogoutResponse{}, nil
}

func (f *flakyLoginClient) PlayerAutosave(ctx context.Context, req *loginpb.PlayerAutosaveRequest) {}

func (f *flakyLoginClient) PlayerForceLogout(ctx context.Context, req *loginpb.PlayerForceLogoutRequest) {
}

func (f *flakyLoginClient) PlayerBan(ctx context.Context, req *loginpb.PlayerBanRequest) {}

func (f *flakyLoginClient) PlayerMute(ctx context.Context, req *loginpb.PlayerMuteRequest) {}

func (f *flakyLoginClient) Close() error { return nil }

// newRetryTestServer builds the minimal partial Server these tests need
// (loginClient, bridgesCtx, log, zero-value cfg) via the shared newTestServer
// fixture, then wires in the given fake LoginClient.
func newRetryTestServer(t *testing.T, loginClient LoginClient) *Server {
	t.Helper()
	s := newTestServer(t)
	s.loginClient = loginClient
	return s
}

// arch-28.5: a transient login-service outage at logout must not lose the
// save — retry up to 3 attempts total.
func TestLogoutSaveRetriesTransientFailure(t *testing.T) {
	fake := &flakyLoginClient{failFirst: 2}
	s := newRetryTestServer(t, fake)
	s.logoutSaveRetryDelay = time.Millisecond

	s.sendPlayerLogoutWithRetry("player_one", []byte{1, 2, 3})
	if got := fake.calls.Load(); got != 3 {
		t.Fatalf("calls: got %d, want 3 (2 failures + 1 success)", got)
	}
}

func TestLogoutSaveGivesUpAfterMaxAttempts(t *testing.T) {
	fake := &flakyLoginClient{failFirst: 99}
	s := newRetryTestServer(t, fake)
	s.logoutSaveRetryDelay = time.Millisecond

	s.sendPlayerLogoutWithRetry("player_one", []byte{1})
	if got := fake.calls.Load(); got != 3 {
		t.Fatalf("calls: got %d, want exactly 3 attempts", got)
	}
}
