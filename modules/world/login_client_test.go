package world

import (
	"errors"
	"net"
	"testing"

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
