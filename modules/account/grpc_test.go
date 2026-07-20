package account

import (
	"context"
	"net"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	"github.com/zsrv/goscape/pkg/accountpb"
)

// startBufconnServer runs the account gRPC server over an in-memory
// listener and returns a connected client. Reused by admin + e2e tests.
func startBufconnServer(t *testing.T, cfg Config, store *Store) accountpb.AccountServiceClient {
	t.Helper()
	lis := bufconn.Listen(1 << 20)
	srv := newGRPCServer(cfg, store, testLogger(t))
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)

	conn, err := grpc.NewClient("passthrough:///bufconn",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return accountpb.NewAccountServiceClient(conn)
}

// seedVerifiedAccountWithCharacter registers an account (argon2
// password "hunter22!"), verifies email, and creates character `name`.
func seedVerifiedAccountWithCharacter(t *testing.T, s *Store, email, name string) int64 {
	t.Helper()
	ctx := t.Context()
	phc, err := HashPassword("hunter22!", testArgon2())
	if err != nil {
		t.Fatal(err)
	}
	id, err := s.CreateAccount(ctx, email, phc)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetEmailVerified(ctx, id); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateCharacter(ctx, id, name, 5); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestVerifyGameLogin(t *testing.T) {
	s := openTestStore(t)
	cfg := defaultConfig(t)
	client := startBufconnServer(t, cfg, s)
	ctx := t.Context()

	portalID := seedVerifiedAccountWithCharacter(t, s, "a@example.com", "zezima")

	verify := func(name, pw string) *accountpb.VerifyGameLoginResponse {
		t.Helper()
		resp, err := client.VerifyGameLogin(ctx, &accountpb.VerifyGameLoginRequest{
			CharacterName: name, Password: pw, RemoteAddress: "1.2.3.4:5",
		})
		if err != nil {
			t.Fatalf("rpc: %v", err)
		}
		return resp
	}

	// OK path.
	resp := verify("zezima", "hunter22!")
	if resp.Result != accountpb.VerifyResult_VERIFY_RESULT_OK ||
		resp.PortalAccountId != portalID || resp.GameAccountId == 0 {
		t.Fatalf("ok path: %+v", resp)
	}
	// Unknown character.
	if r := verify("ghost", "hunter22!"); r.Result != accountpb.VerifyResult_VERIFY_RESULT_INVALID_CREDENTIALS {
		t.Fatalf("unknown char: %v", r.Result)
	}
	// Wrong password (case-sensitive).
	if r := verify("zezima", "HUNTER22!"); r.Result != accountpb.VerifyResult_VERIFY_RESULT_INVALID_CREDENTIALS {
		t.Fatalf("wrong pw: %v", r.Result)
	}
	// Disabled account (correct password).
	if err := s.SetAccountStatus(ctx, portalID, StatusDisabled); err != nil {
		t.Fatal(err)
	}
	if r := verify("zezima", "hunter22!"); r.Result != accountpb.VerifyResult_VERIFY_RESULT_ACCOUNT_DISABLED {
		t.Fatalf("disabled: %v", r.Result)
	}
	// Unverified email (correct password, re-enabled).
	if err := s.SetAccountStatus(ctx, portalID, StatusActive); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, s.db.Rebind(
		`UPDATE portal_account SET email_verified = 0 WHERE id = ?`), portalID); err != nil {
		t.Fatal(err)
	}
	if r := verify("zezima", "hunter22!"); r.Result != accountpb.VerifyResult_VERIFY_RESULT_EMAIL_UNVERIFIED {
		t.Fatalf("unverified: %v", r.Result)
	}
}

func TestAdminRPCsRequireToken(t *testing.T) {
	s := openTestStore(t)

	// No token configured: admin surface is disabled outright.
	cfgNoToken := defaultConfig(t)
	client := startBufconnServer(t, cfgNoToken, s)
	_, err := client.SearchAccounts(t.Context(), &accountpb.SearchAccountsRequest{Query: "x"})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("no-token: got %v, want PermissionDenied", err)
	}

	// Token configured: wrong/missing bearer is Unauthenticated; right one passes.
	cfg := defaultConfig(t)
	cfg.AdminToken = "sekrit"
	client = startBufconnServer(t, cfg, s)

	_, err = client.SearchAccounts(t.Context(), &accountpb.SearchAccountsRequest{Query: "x"})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("missing bearer: got %v, want Unauthenticated", err)
	}
	bad := metadata.AppendToOutgoingContext(t.Context(), "authorization", "Bearer wrong")
	_, err = client.SearchAccounts(bad, &accountpb.SearchAccountsRequest{Query: "x"})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("bad bearer: got %v, want Unauthenticated", err)
	}
	good := metadata.AppendToOutgoingContext(t.Context(), "authorization", "Bearer sekrit")
	if _, err = client.SearchAccounts(good, &accountpb.SearchAccountsRequest{Query: "x"}); err != nil {
		if c := status.Code(err); c != codes.OK && c != codes.Unimplemented {
			t.Fatalf("good bearer: %v", err)
		}
	}

	// VerifyGameLogin never needs the token.
	if _, err = client.VerifyGameLogin(t.Context(), &accountpb.VerifyGameLoginRequest{
		CharacterName: "nobody", Password: "x",
	}); err != nil {
		t.Fatalf("VerifyGameLogin must bypass admin auth: %v", err)
	}
}
