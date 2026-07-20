package login

import (
	"context"
	"errors"
	"flag"
	"net"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	"github.com/zsrv/goscape/pkg/accountpb"
	"github.com/zsrv/goscape/pkg/loginpb"
)

// stubAccountService returns canned VerifyGameLogin responses and
// records the request the login handler sent.
type stubAccountService struct {
	accountpb.UnimplementedAccountServiceServer
	resp   *accountpb.VerifyGameLoginResponse
	err    error
	gotReq *accountpb.VerifyGameLoginRequest
}

func (s *stubAccountService) VerifyGameLogin(_ context.Context, req *accountpb.VerifyGameLoginRequest) (*accountpb.VerifyGameLoginResponse, error) {
	s.gotReq = req
	return s.resp, s.err
}

func stubAccountClient(t *testing.T, stub *stubAccountService) accountpb.AccountServiceClient {
	t.Helper()
	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer()
	accountpb.RegisterAccountServiceServer(srv, stub)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)
	conn, err := grpc.NewClient("passthrough:///bufconn",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	return accountpb.NewAccountServiceClient(conn)
}

func TestConfig_AuthModeValidation(t *testing.T) {
	base := func() Config {
		var c Config
		fs := flag.NewFlagSet("", flag.PanicOnError)
		c.RegisterFlagsAndApplyDefaults(fs)
		c.Enable = true
		return c
	}
	c := base()
	c.AuthMode = "bogus"
	if err := c.Validate(); err == nil {
		t.Fatal("bogus auth_mode must fail validation")
	}
	c = base()
	c.AuthMode = AuthModeAccount
	c.AccountGRPCAddress = ""
	c.AutoRegister = false
	if err := c.Validate(); err == nil {
		t.Fatal("account mode without address must fail validation")
	}
	c = base()
	c.AuthMode = AuthModeAccount
	c.AccountGRPCAddress = "127.0.0.1:2005"
	c.AutoRegister = true
	if err := c.Validate(); err == nil {
		t.Fatal("account mode + auto_register must be a config conflict")
	}
	c = base()
	c.AuthMode = AuthModeAccount
	c.AccountGRPCAddress = "127.0.0.1:2005"
	c.AutoRegister = false
	if err := c.Validate(); err != nil {
		t.Fatalf("valid account mode rejected: %v", err)
	}
}

func TestPlayerLogin_AccountMode(t *testing.T) {
	h, _ := newTestHandler(t)
	db := h.db
	h.cfg.AuthMode = AuthModeAccount
	h.cfg.AutoRegister = false
	ctx := t.Context()

	// Seed the game account row the way portal character creation does.
	if _, err := db.ExecContext(ctx, db.Rebind(
		`INSERT INTO account (username, password, registration_ip) VALUES ('zezima', '!portal-managed!', 'portal')`)); err != nil {
		t.Fatal(err)
	}
	var gameID int64
	if err := db.QueryRowContext(ctx, db.Rebind(`SELECT id FROM account WHERE username = 'zezima'`)).Scan(&gameID); err != nil {
		t.Fatal(err)
	}

	req := func() *loginpb.PlayerLoginRequest {
		return &loginpb.PlayerLoginRequest{
			NodeId: 10, Profile: "main", Username: "zezima",
			Password: "MixedCase!", RemoteAddress: "9.9.9.9:1", Uid: 7,
		}
	}

	// OK path: delegated verify succeeds → NEW_PLAYER (no save on disk),
	// and the password reaches the account service VERBATIM (no
	// lowercasing — that quirk is local-mode only).
	stub := &stubAccountService{resp: &accountpb.VerifyGameLoginResponse{
		Result: accountpb.VerifyResult_VERIFY_RESULT_OK, GameAccountId: gameID, PortalAccountId: 1}}
	h.acct = stubAccountClient(t, stub)
	resp, err := h.PlayerLogin(ctx, req())
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if resp.Result != loginpb.LoginResult_LOGIN_RESULT_NEW_PLAYER {
		t.Fatalf("result = %v, want NEW_PLAYER", resp.Result)
	}
	if stub.gotReq.Password != "MixedCase!" || stub.gotReq.CharacterName != "zezima" {
		t.Fatalf("delegated request: %+v", stub.gotReq)
	}

	// Result mapping table.
	cases := []struct {
		verify accountpb.VerifyResult
		want   loginpb.LoginResult
	}{
		{accountpb.VerifyResult_VERIFY_RESULT_INVALID_CREDENTIALS, loginpb.LoginResult_LOGIN_RESULT_INVALID_CREDENTIALS},
		{accountpb.VerifyResult_VERIFY_RESULT_ACCOUNT_DISABLED, loginpb.LoginResult_LOGIN_RESULT_ACCOUNT_DISABLED},
		{accountpb.VerifyResult_VERIFY_RESULT_EMAIL_UNVERIFIED, loginpb.LoginResult_LOGIN_RESULT_ACCOUNT_DISABLED},
	}
	for _, tc := range cases {
		stub.resp = &accountpb.VerifyGameLoginResponse{Result: tc.verify}
		resp, err := h.PlayerLogin(ctx, req())
		if err != nil {
			t.Fatalf("%v: %v", tc.verify, err)
		}
		if resp.Result != tc.want {
			t.Fatalf("%v → %v, want %v", tc.verify, resp.Result, tc.want)
		}
	}

	// Transport failure → gRPC error (world maps to login-server-offline).
	stub.resp = nil
	stub.err = status.Error(codes.Unavailable, "down")
	if _, err := h.PlayerLogin(ctx, req()); status.Code(err) != codes.Unavailable {
		t.Fatalf("transport failure: got %v, want Unavailable", err)
	}

	// IP ban still runs BEFORE delegation.
	stub.err = errors.New("must not be called")
	if _, err := db.ExecContext(ctx, db.Rebind(`INSERT INTO ipban (ip) VALUES ('9.9.9.9')`)); err != nil {
		t.Fatal(err)
	}
	stub.gotReq = nil
	resp, err = h.PlayerLogin(ctx, req())
	if err != nil || resp.Result != loginpb.LoginResult_LOGIN_RESULT_IP_BANNED {
		t.Fatalf("ip ban: %v %v", resp, err)
	}
	if stub.gotReq != nil {
		t.Fatal("account service must not be consulted for IP-banned logins")
	}
}
