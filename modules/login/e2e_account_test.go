package login_test

import (
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"path/filepath"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/zsrv/goscape/modules/account"
	"github.com/zsrv/goscape/modules/login"
	"github.com/zsrv/goscape/pkg/dskit/services"
	"github.com/zsrv/goscape/pkg/gamedb"
	"github.com/zsrv/goscape/pkg/loginpb"
)

func freePort(t *testing.T) int {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer lis.Close()
	return lis.Addr().(*net.TCPAddr).Port
}

// TestE2E_PortalToGameLogin runs the whole story on real listeners:
// register → verify (link from log-mailer is unavailable, so verify via
// DB) → approve → create character in the portal → PlayerLogin through
// the login module in account mode → NEW_PLAYER.
func TestE2E_PortalToGameLogin(t *testing.T) {
	dir := t.TempDir()
	var dbCfg gamedb.Config
	dbCfg.Backend = gamedb.BackendSQLite
	dbCfg.SQLite.DSN = filepath.Join(dir, "e2e.db")
	logger := slog.Default()

	// Migrate (the app's database module normally does this).
	db, err := gamedb.Open(dbCfg, logger)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Migrate(t.Context()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	// Account module on real ports.
	acctHTTP, acctGRPC := freePort(t), freePort(t)
	acctCfg := account.NewTestConfig()
	acctCfg.Enable = true
	acctCfg.HTTPListenPort = acctHTTP
	acctCfg.GRPCListenPort = acctGRPC
	acctCfg.PublicURL = fmt.Sprintf("http://127.0.0.1:%d", acctHTTP)
	acctMod, err := account.New(acctCfg, dbCfg, logger)
	if err != nil {
		t.Fatal(err)
	}
	if err := services.StartAndAwaitRunning(t.Context(), acctMod); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = services.StopAndAwaitTerminated(t.Context(), acctMod) })

	// Login module in account mode on a real port.
	loginPort := freePort(t)
	loginCfg := login.NewTestConfig()
	loginCfg.Enable = true
	loginCfg.GRPCListenPort = loginPort
	loginCfg.SavePath = filepath.Join(dir, "players")
	loginCfg.AuthMode = login.AuthModeAccount
	loginCfg.AccountGRPCAddress = fmt.Sprintf("127.0.0.1:%d", acctGRPC)
	loginCfg.AutoRegister = false
	loginMod, err := login.New(loginCfg, dbCfg, logger)
	if err != nil {
		t.Fatal(err)
	}
	if err := services.StartAndAwaitRunning(t.Context(), loginMod); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = services.StopAndAwaitTerminated(t.Context(), loginMod) })

	// --- Portal: register + create character ---
	base := fmt.Sprintf("http://127.0.0.1:%d", acctHTTP)
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar, Timeout: 10 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

	// SEC1 M-8: /register and /login are anonymous, double-submit-CSRF
	// protected. Seed the cookie with a GET of a public form page, then
	// thread its value back as the "csrf" form field on both POSTs.
	if resp, err := client.Get(base + "/login"); err != nil {
		t.Fatal(err)
	} else {
		resp.Body.Close()
	}
	anonCSRF := account.AnonymousCSRFTokenForTest(jar, base)
	if anonCSRF == "" {
		t.Fatal("no anonymous csrf cookie after GET /login")
	}

	if _, err := client.PostForm(base+"/register", url.Values{
		"email": {"e2e@example.com"}, "password": {"hunter22!"}, "password2": {"hunter22!"}, "csrf": {anonCSRF},
	}); err != nil {
		t.Fatal(err)
	}
	// Verify + approve directly in the DB (mail went to the log mailer).
	if _, err := db.ExecContext(t.Context(),
		`UPDATE portal_account SET email_verified = 1 WHERE email = 'e2e@example.com'`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(t.Context(), `
		INSERT INTO portal_group_member (group_id, account_id, added_by, added_at)
		SELECT g.id, a.id, NULL, '2026-07-19 00:00:00'
		FROM portal_group g, portal_account a
		WHERE g.name = 'manually_approved' AND a.email = 'e2e@example.com'`); err != nil {
		t.Fatal(err)
	}
	// Log in and create the character through the real portal.
	if _, err := client.PostForm(base+"/login", url.Values{
		"email": {"e2e@example.com"}, "password": {"hunter22!"}, "csrf": {anonCSRF},
	}); err != nil {
		t.Fatal(err)
	}
	var raw string
	u, _ := url.Parse(base)
	for _, c := range jar.Cookies(u) {
		if c.Name == "goscape_session" {
			raw = c.Value
		}
	}
	if raw == "" {
		t.Fatal("no portal session")
	}
	resp, err := client.PostForm(base+"/characters/new", url.Values{
		"name": {"e2ehero"}, "csrf": {account.CSRFTokenForTest(raw)},
	})
	if err != nil || resp.StatusCode != http.StatusFound {
		t.Fatalf("character create: %v %d", err, resp.StatusCode)
	}

	// --- Game side: PlayerLogin over the login module's real gRPC ---
	conn, err := grpc.NewClient(fmt.Sprintf("127.0.0.1:%d", loginPort),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	lc := loginpb.NewLoginServiceClient(conn)

	lresp, err := lc.PlayerLogin(t.Context(), &loginpb.PlayerLoginRequest{
		NodeId: 10, Profile: "main", Username: "e2ehero",
		Password: "hunter22!", RemoteAddress: "127.0.0.1:9", Uid: 1,
	})
	if err != nil {
		t.Fatalf("PlayerLogin: %v", err)
	}
	if lresp.Result != loginpb.LoginResult_LOGIN_RESULT_NEW_PLAYER {
		t.Fatalf("result = %v, want NEW_PLAYER", lresp.Result)
	}

	// Wrong password fails; wrong case fails (case-sensitive in account mode).
	for _, pw := range []string{"wrong", "HUNTER22!"} {
		lresp, err = lc.PlayerLogin(t.Context(), &loginpb.PlayerLoginRequest{
			NodeId: 10, Profile: "main", Username: "e2ehero",
			Password: pw, RemoteAddress: "127.0.0.1:9", Uid: 1,
		})
		if err != nil || lresp.Result != loginpb.LoginResult_LOGIN_RESULT_INVALID_CREDENTIALS {
			t.Fatalf("pw %q: %v %v", pw, lresp.Result, err)
		}
	}
}
