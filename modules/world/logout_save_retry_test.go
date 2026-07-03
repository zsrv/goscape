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

func (f *flakyLoginClient) WorldStartup(ctx context.Context, nodeID int32, profile string) error {
	return nil
}

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

// blockingLoginClient is a test-only LoginClient whose PlayerLogout blocks
// until its ctx is done (or release is closed, as a belt-and-suspenders
// unblock), then returns ctx.Err(). Used to pin arch-29.11: cancelling
// bridgesCtx mid-attempt must abort sendPlayerLogoutWithRetry's loop
// promptly instead of leaving it to burn through logoutSaveAttempts on the
// configured retry delay. The other LoginClient methods are unused by this
// test and stubbed as no-ops, mirroring flakyLoginClient.
type blockingLoginClient struct {
	release chan struct{}
	calls   atomic.Int32
}

var _ LoginClient = (*blockingLoginClient)(nil)

func (b *blockingLoginClient) WorldStartup(ctx context.Context, nodeID int32, profile string) error {
	return nil
}

func (b *blockingLoginClient) PlayerLogin(ctx context.Context, req *loginpb.PlayerLoginRequest) (*loginpb.PlayerLoginResponse, error) {
	return nil, nil
}

func (b *blockingLoginClient) PlayerLogout(ctx context.Context, req *loginpb.PlayerLogoutRequest) (*loginpb.PlayerLogoutResponse, error) {
	b.calls.Add(1)
	select {
	case <-ctx.Done():
	case <-b.release:
	}
	return nil, ctx.Err()
}

func (b *blockingLoginClient) PlayerAutosave(ctx context.Context, req *loginpb.PlayerAutosaveRequest) {
}

func (b *blockingLoginClient) PlayerForceLogout(ctx context.Context, req *loginpb.PlayerForceLogoutRequest) {
}

func (b *blockingLoginClient) PlayerBan(ctx context.Context, req *loginpb.PlayerBanRequest) {}

func (b *blockingLoginClient) PlayerMute(ctx context.Context, req *loginpb.PlayerMuteRequest) {}

func (b *blockingLoginClient) Close() error { return nil }

// arch-29.11: cancelling bridgesCtx mid-retry must abort the loop
// promptly (shutdown's bridgesCancel path) instead of burning the
// remaining attempts.
func TestLogoutSaveAbortsOnBridgesCancel(t *testing.T) {
	block := make(chan struct{})
	fake := &blockingLoginClient{release: block} // PlayerLogout waits on ctx.Done, returns ctx.Err()
	s := newRetryTestServer(t, fake)
	s.logoutSaveRetryDelay = time.Hour // any retry sleep would hang the test — abort must preempt it
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.sendPlayerLogoutWithRetry("player_one", []byte{1})
	}()
	time.Sleep(20 * time.Millisecond)
	s.bridgesCancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("retry loop did not abort on bridgesCancel")
	}
	if got := fake.calls.Load(); got > 2 {
		t.Fatalf("calls after cancel: got %d, want <= 2", got)
	}
}
