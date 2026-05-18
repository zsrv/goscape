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
		Socket:        c.conn.RemoteAddr().String(),
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
				if c.staffModLevel != 2 || !c.members || c.username != "test" || string(c.savePayload) != "SAVE-BYTES" {
					t.Errorf("expected session cached: got staffModLevel=%d members=%v username=%q savePayload=%q",
						c.staffModLevel, c.members, c.username, c.savePayload)
				}
			} else {
				if c.staffModLevel != 0 || c.members || c.username != "" || c.savePayload != nil {
					t.Errorf("expected session NOT cached: got staffModLevel=%d members=%v username=%q savePayload=%v",
						c.staffModLevel, c.members, c.username, c.savePayload)
				}
			}
		})
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
	if c.savePayload != nil || c.username != "" {
		t.Errorf("session must NOT be cached on RPC error: savePayload=%v username=%q", c.savePayload, c.username)
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
