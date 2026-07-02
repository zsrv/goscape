package world

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	loginresp "github.com/zsrv/goscape/pkg/io/protocol/login/resp"
	"github.com/zsrv/goscape/pkg/loginpb"
)

// newClientWithFakeLoginServer constructs a *client whose c.server.loginClient
// is the supplied fake. The server has minimal config (NodeID/NodeProfile/
// NodeMembers) so PlayerLoginRequest fields are deterministic.
func newClientWithFakeLoginServer(t *testing.T, fake *fakeLoginClient) (*client, net.Conn) {
	t.Helper()
	c, conn := newTestClient(t)
	s := newTestServer(t)
	s.cfg.NodeID = 42
	s.cfg.NodeProfile = "main"
	s.cfg.NodeMembers = true
	s.loginClient = fake
	c.server = s
	return c, conn
}

// sampleLoginReq returns a deterministic PlayerLoginRequest matching what
// handleLogin would build for username "test" / password "pw".
func sampleLoginReq(t *testing.T, c *client) *loginpb.PlayerLoginRequest {
	t.Helper()
	return &loginpb.PlayerLoginRequest{
		NodeId:        int32(c.server.cfg.NodeID),
		Profile:       c.server.cfg.NodeProfile,
		NodeMembers:   c.server.cfg.NodeMembers,
		Username:      "test",
		Password:      "pw",
		Uid:           1234,
		RemoteAddress: c.conn.RemoteAddr().String(),
		Reconnecting:  false,
		HasSave:       false,
	}
}

func TestCallPlayerLoginRPC_CapturesRequest(t *testing.T) {
	fake := newFakeLoginClient()
	fake.playerLoginResp = &loginpb.PlayerLoginResponse{
		Result: loginpb.LoginResult_LOGIN_RESULT_OK,
	}
	c, _ := newClientWithFakeLoginServer(t, fake)
	req := sampleLoginReq(t, c)

	if _, err := c.callPlayerLoginRPC(req, "test"); err != nil {
		t.Fatalf("callPlayerLoginRPC: unexpected err %v", err)
	}

	got := fake.snapshotPlayerLoginReq()
	if got == nil {
		t.Fatal("no PlayerLoginRequest captured")
	}
	if got.NodeId != 42 || got.Profile != "main" || !got.NodeMembers {
		t.Errorf("server-cfg fields: got NodeId=%d Profile=%q Members=%v; want 42 main true",
			got.NodeId, got.Profile, got.NodeMembers)
	}
	if got.Username != "test" || got.Password != "pw" || got.Uid != 1234 {
		t.Errorf("user fields: got Username=%q Password=%q Uid=%d; want test pw 1234",
			got.Username, got.Password, got.Uid)
	}
	if got.Reconnecting || got.HasSave {
		t.Errorf("flags: Reconnecting=%v HasSave=%v; want false false", got.Reconnecting, got.HasSave)
	}
}

func TestCallPlayerLoginRPC_ReplyByteMapping(t *testing.T) {
	cases := []struct {
		name      string
		result    loginpb.LoginResult
		wantReply byte
		caches    bool // whether session fields should be cached
	}{
		{"OK", loginpb.LoginResult_LOGIN_RESULT_OK, loginresp.OpOK.Opcode, true},
		{"NEW_PLAYER", loginpb.LoginResult_LOGIN_RESULT_NEW_PLAYER, loginresp.OpOK.Opcode, true},
		{"RECONNECT_OK", loginpb.LoginResult_LOGIN_RESULT_RECONNECT_OK, loginresp.OpReconnectOK.Opcode, true},
		{"INVALID_CREDENTIALS", loginpb.LoginResult_LOGIN_RESULT_INVALID_CREDENTIALS, loginresp.OpInvalidUsernameOrPassword.Opcode, false},
		{"ALREADY_LOGGED_IN", loginpb.LoginResult_LOGIN_RESULT_ALREADY_LOGGED_IN, loginresp.OpDuplicate.Opcode, false},
		{"ACCOUNT_DISABLED", loginpb.LoginResult_LOGIN_RESULT_ACCOUNT_DISABLED, loginresp.OpBanned.Opcode, false},
		{"NOT_A_MEMBER", loginpb.LoginResult_LOGIN_RESULT_NOT_A_MEMBER, loginresp.OpNeedMembersAccount.Opcode, false},
		{"LOGIN_IN_PROGRESS", loginpb.LoginResult_LOGIN_RESULT_LOGIN_IN_PROGRESS, loginresp.OpTooManyAttempts.Opcode, false},
		{"IP_BANNED", loginpb.LoginResult_LOGIN_RESULT_IP_BANNED, loginresp.OpLoginServerRejected.Opcode, false},
		// rev-254 A4: hop timer is response 10 → wire opcode 21 (TS
		// World.ts:1861-1866 @2e3bcf43); pre-254 it rode response 6 →
		// byte 9 (OpIPLimit).
		{"HOP_TIMER", loginpb.LoginResult_LOGIN_RESULT_HOP_TIMER, loginresp.OpHopTimer.Opcode, false},
		{"UNSPECIFIED_default", loginpb.LoginResult_LOGIN_RESULT_UNSPECIFIED, loginresp.OpIPLimit.Opcode, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := newFakeLoginClient()
			fake.playerLoginResp = &loginpb.PlayerLoginResponse{
				Result:        tc.result,
				StaffModLevel: 2,
				Members:       true,
				Save:          []byte("SAVE-BYTES"),
				SessionUuid:   "test-uuid-123",
			}
			c, _ := newClientWithFakeLoginServer(t, fake)
			req := sampleLoginReq(t, c)

			reply, err := c.callPlayerLoginRPC(req, "test")
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if reply != tc.wantReply {
				t.Errorf("reply byte: got %d, want %d", reply, tc.wantReply)
			}

			if tc.caches {
				if c.staffModLevel != 2 || !c.members || c.username != "test" || string(c.savePayload) != "SAVE-BYTES" || c.sessionUUID != "test-uuid-123" {
					t.Errorf("expected session cached: got staffModLevel=%d members=%v username=%q savePayload=%q sessionUUID=%q",
						c.staffModLevel, c.members, c.username, c.savePayload, c.sessionUUID)
				}
			} else {
				if c.staffModLevel != 0 || c.members || c.username != "" || c.savePayload != nil || c.sessionUUID != "" {
					t.Errorf("expected session NOT cached: got staffModLevel=%d members=%v username=%q savePayload=%v sessionUUID=%q",
						c.staffModLevel, c.members, c.username, c.savePayload, c.sessionUUID)
				}
			}
		})
	}
}

// TestCallPlayerLoginRPC_HopTimerCachesRemaining pins the rev-254 A4
// remaining_ms plumbing: a HOP_TIMER response caches the millisecond
// remainder on c.hopRemainingMs (consumed by sendLoginHopTimer to build
// the [21, min(255, remaining/1000)] reply) and caches NO session fields.
func TestCallPlayerLoginRPC_HopTimerCachesRemaining(t *testing.T) {
	fake := newFakeLoginClient()
	fake.playerLoginResp = &loginpb.PlayerLoginResponse{
		Result:      loginpb.LoginResult_LOGIN_RESULT_HOP_TIMER,
		RemainingMs: 32_000,
	}
	c, _ := newClientWithFakeLoginServer(t, fake)
	req := sampleLoginReq(t, c)

	reply, err := c.callPlayerLoginRPC(req, "test")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if reply != loginresp.OpHopTimer.Opcode {
		t.Errorf("reply byte: got %d, want OpHopTimer (%d)", reply, loginresp.OpHopTimer.Opcode)
	}
	if c.hopRemainingMs != 32_000 {
		t.Errorf("hopRemainingMs: got %d, want 32000", c.hopRemainingMs)
	}
	if c.savePayload != nil || c.username != "" || c.sessionUUID != "" {
		t.Errorf("session must NOT be cached on hop-timer reject: savePayload=%v username=%q sessionUUID=%q",
			c.savePayload, c.username, c.sessionUUID)
	}
}

// arch-29.3 fix wave (reviewer Critical): logins must be rejected until
// the WorldStartup registration has succeeded. WorldStartup's blanket
// UPDATE clears logged_in for the whole node+profile; if a login were
// admitted while the registration retry loop was still pending, the
// eventually-successful retry would wipe the LIVE session's logged_in
// flag and falsify the duplicate-login guard. The reject is the same
// wire behavior as the login-server-unreachable path (opcode 8) — a
// not-yet-registered world and a down login server are operationally
// the same.
func TestLoginGateBlocksUntilWorldStartup(t *testing.T) {
	fake := newFakeLoginClient()
	fake.playerLoginResp = &loginpb.PlayerLoginResponse{
		Result: loginpb.LoginResult_LOGIN_RESULT_OK,
	}
	c, _ := newClientWithFakeLoginServer(t, fake)
	c.server.worldStartupDone.Store(false) // registration still retrying
	req := sampleLoginReq(t, c)

	reply, err := c.callPlayerLoginRPC(req, "test")
	if err == nil {
		t.Fatal("want error while WorldStartup registration is pending, got nil")
	}
	if reply != loginresp.OpLoginServerOffline.Opcode {
		t.Errorf("reply: got %d, want OpLoginServerOffline (%d) — gate must reuse the unreachable-login-server reject",
			reply, loginresp.OpLoginServerOffline.Opcode)
	}
	if got := fake.snapshotPlayerLoginReq(); got != nil {
		t.Fatalf("PlayerLogin must NOT be dispatched while the gate is closed; captured %v", got)
	}

	c.server.worldStartupDone.Store(true) // registration succeeded
	reply, err = c.callPlayerLoginRPC(req, "test")
	if err != nil {
		t.Fatalf("callPlayerLoginRPC after gate open: unexpected err %v", err)
	}
	if reply != loginresp.OpOK.Opcode {
		t.Errorf("reply after gate open: got %d, want OpOK (%d)", reply, loginresp.OpOK.Opcode)
	}
	if fake.snapshotPlayerLoginReq() == nil {
		t.Fatal("PlayerLogin should be dispatched once the gate is open")
	}
}

// arch-29.3 fix wave: a standalone world (no login client configured) has
// no WorldStartup registration to wait for, so its login gate starts open;
// a world WITH a login client starts gated until the registration succeeds.
func TestLoginGateOpenWhenStandalone(t *testing.T) {
	s := newTestServer(t)
	s.worldStartupDone.Store(false)
	s.initLoginGate(nil)
	if !s.worldStartupDone.Load() {
		t.Fatal("standalone world (nil login client) must not gate logins")
	}

	s2 := newTestServer(t)
	s2.worldStartupDone.Store(false)
	s2.initLoginGate(newFakeLoginClient())
	if s2.worldStartupDone.Load() {
		t.Fatal("gate must start closed when a login client is configured")
	}
}

func TestCallPlayerLoginRPC_RPCErrorReturnsServerOffline(t *testing.T) {
	rpcErr := errors.New("simulated rpc failure")
	fake := newFakeLoginClient()
	fake.playerLoginErr = rpcErr
	c, _ := newClientWithFakeLoginServer(t, fake)
	req := sampleLoginReq(t, c)

	reply, err := c.callPlayerLoginRPC(req, "test")
	if !errors.Is(err, rpcErr) {
		t.Errorf("err: got %v, want %v (or wrapped)", err, rpcErr)
	}
	if reply != loginresp.OpLoginServerOffline.Opcode {
		t.Errorf("reply: got %d, want OpLoginServerOffline (%d)", reply, loginresp.OpLoginServerOffline.Opcode)
	}
	if c.savePayload != nil || c.username != "" || c.sessionUUID != "" {
		t.Errorf("session must NOT be cached on RPC error: savePayload=%v username=%q sessionUUID=%q", c.savePayload, c.username, c.sessionUUID)
	}
}

// TestCallPlayerLoginRPC_DataLossErrorReturnsRejected pins login-server-5:
// the login handler emits codes.DataLoss for every rejectLoginForSafety
// path (TS LoginServer.ts:115-124 / 287-290 / 346-347 / 364-367). The
// world must map that specific gRPC status to OpLoginServerRejected
// (wire opcode 11 — "Login server rejected session. Please try again.")
// — matching TS World.ts:1857-1861 where reply=7 → opcode 11. The
// previous code lumped DataLoss in with every other gRPC error and
// returned OpLoginServerOffline (opcode 8), surfacing a safety-reject
// to the user as "Login server offline".
func TestCallPlayerLoginRPC_DataLossErrorReturnsRejected(t *testing.T) {
	dataLossErr := status.Error(codes.DataLoss, "save verify failed")
	fake := newFakeLoginClient()
	fake.playerLoginErr = dataLossErr
	c, _ := newClientWithFakeLoginServer(t, fake)
	req := sampleLoginReq(t, c)

	reply, err := c.callPlayerLoginRPC(req, "test")
	if !errors.Is(err, dataLossErr) {
		t.Errorf("err: got %v, want %v (or wrapped)", err, dataLossErr)
	}
	if reply != loginresp.OpLoginServerRejected.Opcode {
		t.Errorf("reply: got %d, want OpLoginServerRejected (%d) — TS LoginServer.ts:115-124 rejectLoginForSafety → World.ts:1859 opcode 11 (login-server-5)",
			reply, loginresp.OpLoginServerRejected.Opcode)
	}
	if c.savePayload != nil || c.username != "" || c.sessionUUID != "" {
		t.Errorf("session must NOT be cached on rejected login: savePayload=%v username=%q sessionUUID=%q",
			c.savePayload, c.username, c.sessionUUID)
	}
}

// TestCallPlayerLoginRPC_NonDataLossErrorStillReturnsOffline guards against
// a regression where the login-server-5 dispatch widens to catch every gRPC
// error: a real transport failure (codes.Unavailable) or codes.Internal must
// continue to surface as OpLoginServerOffline (opcode 8 — "Login server
// offline"). Only the DataLoss-coded safety-reject path translates to opcode 11.
func TestCallPlayerLoginRPC_NonDataLossErrorStillReturnsOffline(t *testing.T) {
	for _, code := range []codes.Code{codes.Unavailable, codes.Internal, codes.Unknown} {
		t.Run(code.String(), func(t *testing.T) {
			rpcErr := status.Error(code, "transport down")
			fake := newFakeLoginClient()
			fake.playerLoginErr = rpcErr
			c, _ := newClientWithFakeLoginServer(t, fake)
			req := sampleLoginReq(t, c)

			reply, err := c.callPlayerLoginRPC(req, "test")
			if !errors.Is(err, rpcErr) {
				t.Errorf("err: got %v, want %v (or wrapped)", err, rpcErr)
			}
			if reply != loginresp.OpLoginServerOffline.Opcode {
				t.Errorf("reply for %s: got %d, want OpLoginServerOffline (%d)",
					code, reply, loginresp.OpLoginServerOffline.Opcode)
			}
		})
	}
}

// mockLoginPBClient is an in-package stub of loginpb.LoginServiceClient
// used by grpcLoginClient unit tests. Only the methods exercised by tests
// are overridden; the embedded loginpb.LoginServiceClient panics on any
// unstubbed call (intentional — unexpected calls should surface loudly).
type mockLoginPBClient struct {
	loginpb.LoginServiceClient

	mu               sync.Mutex
	gotPlayerBanReq  *loginpb.PlayerBanRequest
	gotPlayerMuteReq *loginpb.PlayerMuteRequest
	playerBanErr     error
	playerMuteErr    error
}

func (m *mockLoginPBClient) PlayerBan(ctx context.Context, in *loginpb.PlayerBanRequest, opts ...grpc.CallOption) (*loginpb.PlayerBanResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.gotPlayerBanReq = in
	if m.playerBanErr != nil {
		return nil, m.playerBanErr
	}
	return &loginpb.PlayerBanResponse{}, nil
}

func (m *mockLoginPBClient) PlayerMute(ctx context.Context, in *loginpb.PlayerMuteRequest, opts ...grpc.CallOption) (*loginpb.PlayerMuteResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.gotPlayerMuteReq = in
	if m.playerMuteErr != nil {
		return nil, m.playerMuteErr
	}
	return &loginpb.PlayerMuteResponse{}, nil
}

func TestGRPCLoginClient_PlayerBan_PassesRequest(t *testing.T) {
	mock := &mockLoginPBClient{}
	c := &grpcLoginClient{client: mock, log: discardLogger()}

	req := &loginpb.PlayerBanRequest{
		Staff:    "alice",
		Username: "evilbob",
		Until:    timestamppb.New(time.Unix(1747569600, 0)),
	}
	c.PlayerBan(context.Background(), req)

	mock.mu.Lock()
	defer mock.mu.Unlock()
	if mock.gotPlayerBanReq == nil {
		t.Fatal("PlayerBan was not invoked on underlying client")
	}
	if mock.gotPlayerBanReq.Staff != "alice" || mock.gotPlayerBanReq.Username != "evilbob" {
		t.Errorf("req fields: got Staff=%q Username=%q; want alice evilbob",
			mock.gotPlayerBanReq.Staff, mock.gotPlayerBanReq.Username)
	}
}

func TestGRPCLoginClient_PlayerBan_LogsErrorOnFailure(t *testing.T) {
	var logBuf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelWarn}))

	mock := &mockLoginPBClient{playerBanErr: errors.New("rpc down")}
	c := &grpcLoginClient{client: mock, log: log}

	c.PlayerBan(context.Background(), &loginpb.PlayerBanRequest{Username: "evilbob"})

	got := logBuf.String()
	if !strings.Contains(got, "PlayerBan RPC failed") {
		t.Errorf("log output missing message; got: %s", got)
	}
	if !strings.Contains(got, "evilbob") {
		t.Errorf("log output missing username; got: %s", got)
	}
	if !strings.Contains(got, "rpc down") {
		t.Errorf("log output missing error; got: %s", got)
	}
}

func TestGRPCLoginClient_PlayerMute_PassesRequest(t *testing.T) {
	mock := &mockLoginPBClient{}
	c := &grpcLoginClient{client: mock, log: discardLogger()}

	req := &loginpb.PlayerMuteRequest{
		Staff:    "alice",
		Username: "evilbob",
		Until:    timestamppb.New(time.Unix(1747569600, 0)),
	}
	c.PlayerMute(context.Background(), req)

	mock.mu.Lock()
	defer mock.mu.Unlock()
	if mock.gotPlayerMuteReq == nil {
		t.Fatal("PlayerMute was not invoked on underlying client")
	}
	if mock.gotPlayerMuteReq.Staff != "alice" || mock.gotPlayerMuteReq.Username != "evilbob" {
		t.Errorf("req fields: got Staff=%q Username=%q; want alice evilbob",
			mock.gotPlayerMuteReq.Staff, mock.gotPlayerMuteReq.Username)
	}
}

func TestGRPCLoginClient_PlayerMute_LogsErrorOnFailure(t *testing.T) {
	var logBuf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelWarn}))

	mock := &mockLoginPBClient{playerMuteErr: errors.New("rpc down")}
	c := &grpcLoginClient{client: mock, log: log}

	c.PlayerMute(context.Background(), &loginpb.PlayerMuteRequest{Username: "evilbob"})

	got := logBuf.String()
	if !strings.Contains(got, "PlayerMute RPC failed") {
		t.Errorf("log output missing message; got: %s", got)
	}
	if !strings.Contains(got, "evilbob") {
		t.Errorf("log output missing username; got: %s", got)
	}
	if !strings.Contains(got, "rpc down") {
		t.Errorf("log output missing error; got: %s", got)
	}
}
