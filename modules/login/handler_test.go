package login

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
	"uuid"

	"golang.org/x/crypto/bcrypt"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/zsrv/goscape/pkg/loginpb"
	"github.com/zsrv/goscape/pkg/objtype"
)

// newTestHandler creates a handler with an in-memory DB and a temp save directory.
// Returns the handler and the save path.
func newTestHandler(t *testing.T) (*handler, string) {
	t.Helper()
	db := createTestDB(t)
	savePath := t.TempDir()
	h := &handler{
		db: db,
		cfg: Config{
			SavePath:             savePath,
			NodeProfile:          "main",
			AutoRegister:         true,
			AutoSubscribeMembers: true,
			BCryptCost:           4,
		},
		log: noopLogger(),
	}
	return h, savePath
}

func TestPlayerLogin_NewPlayer_AutoRegister(t *testing.T) {
	h, _ := newTestHandler(t)

	resp, err := h.PlayerLogin(t.Context(), &loginpb.PlayerLoginRequest{
		NodeId:        1,
		Profile:       "main",
		NodeMembers:   true,
		Username:      "newplayer",
		Password:      "hunter2",
		Uid:           42,
		RemoteAddress: "192.168.1.1:12345",
	})
	if err != nil {
		t.Fatalf("PlayerLogin: %v", err)
	}
	if resp.Result != loginpb.LoginResult_LOGIN_RESULT_NEW_PLAYER {
		t.Errorf("Result: got %v, want LOGIN_RESULT_NEW_PLAYER", resp.Result)
	}

	// Account should have been created
	acc, err := accountByUsername(t.Context(), h.db, "newplayer", "main")
	if err != nil {
		t.Fatalf("accountByUsername: %v", err)
	}
	if acc == nil {
		t.Fatal("expected account to be created")
	}
}

func TestPlayerLogin_ExistingPlayer(t *testing.T) {
	h, savePath := newTestHandler(t)
	insertTestAccount(t, h.db, "testuser", "pw")

	// Create save file
	saveDir := filepath.Join(savePath, "main")
	if err := os.MkdirAll(saveDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	saveBytes := makeValidSave(500) // must pass verifySave to be served
	saveFile := filepath.Join(saveDir, "testuser.sav")
	if err := os.WriteFile(saveFile, saveBytes, 0o644); err != nil {
		t.Fatalf("write save: %v", err)
	}

	resp, err := h.PlayerLogin(t.Context(), &loginpb.PlayerLoginRequest{
		NodeId:        1,
		Profile:       "main",
		NodeMembers:   true,
		Username:      "testuser",
		Password:      "pw",
		Uid:           42,
		RemoteAddress: "192.168.1.1:12345",
	})
	if err != nil {
		t.Fatalf("PlayerLogin: %v", err)
	}
	if resp.Result != loginpb.LoginResult_LOGIN_RESULT_OK {
		t.Errorf("Result: got %v, want LOGIN_RESULT_OK", resp.Result)
	}
	if string(resp.Save) != string(saveBytes) {
		t.Errorf("Save: got %q, want %q", resp.Save, saveBytes)
	}
}

func TestPlayerLogin_InvalidCredentials(t *testing.T) {
	h, _ := newTestHandler(t)
	insertTestAccount(t, h.db, "creduser", "rightpw")

	resp, err := h.PlayerLogin(t.Context(), &loginpb.PlayerLoginRequest{
		NodeId:        1,
		Profile:       "main",
		NodeMembers:   true,
		Username:      "creduser",
		Password:      "wrongpw",
		Uid:           42,
		RemoteAddress: "192.168.1.1:12345",
	})
	if err != nil {
		t.Fatalf("PlayerLogin: %v", err)
	}
	if resp.Result != loginpb.LoginResult_LOGIN_RESULT_INVALID_CREDENTIALS {
		t.Errorf("Result: got %v, want LOGIN_RESULT_INVALID_CREDENTIALS", resp.Result)
	}
}

// TestPlayerLogin_PortalSentinelPassword_LocalMode pins the
// portal-sentinel-vs-local-mode fix: a character created via the portal
// (account-mode game row password = "!portal-managed!", 16 bytes, never
// a bcrypt hash) that attempts to log in after a rollback to
// auth_mode=local must get a clean INVALID_CREDENTIALS response, not an
// Internal error from bcrypt choking on a too-short hash.
func TestPlayerLogin_PortalSentinelPassword_LocalMode(t *testing.T) {
	h, _ := newTestHandler(t)
	const sentinel = "!portal-managed!" // mirrors account.SentinelGamePassword
	if _, err := insertAccount(t.Context(), h.db, "portaluser", sentinel, "127.0.0.1"); err != nil {
		t.Fatalf("insertAccount: %v", err)
	}

	resp, err := h.PlayerLogin(t.Context(), &loginpb.PlayerLoginRequest{
		NodeId:        1,
		Profile:       "main",
		NodeMembers:   true,
		Username:      "portaluser",
		Password:      "whatever",
		Uid:           42,
		RemoteAddress: "192.168.1.1:12345",
	})
	if err != nil {
		t.Fatalf("PlayerLogin: %v", err)
	}
	if resp.Result != loginpb.LoginResult_LOGIN_RESULT_INVALID_CREDENTIALS {
		t.Errorf("Result: got %v, want LOGIN_RESULT_INVALID_CREDENTIALS", resp.Result)
	}
}

// TestPlayerLogin_ExistingPlayer_CaseInsensitivePassword pins login-server-4:
// TS LoginServer.ts:233 calls `bcrypt.compare(password.toLowerCase(), …)`,
// so a user who registered with "lowerpw" (lowercase hash) can log in with
// "LOWERPW" or "LowerPw". Pre-fix RED: bcrypt would compare verbatim
// "LOWERPW" against hash("lowerpw") → mismatch → INVALID_CREDENTIALS.
// Post-fix GREEN: handler lowercases the input before bcrypt.Compare →
// match → OK.
func TestPlayerLogin_ExistingPlayer_CaseInsensitivePassword(t *testing.T) {
	h, savePath := newTestHandler(t)
	// Account stored with the lowercased hash, matching TS auto-register
	// shape (LoginServer.ts:213 `bcrypt.hashSync(password.toLowerCase(), 10)`).
	insertTestAccount(t, h.db, "caseuser", "lowerpw")

	// Need a valid save so the success path reaches LOGIN_RESULT_OK.
	saveDir := filepath.Join(savePath, "main")
	if err := os.MkdirAll(saveDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	saveBytes := makeValidSave(500)
	saveFile := filepath.Join(saveDir, "caseuser.sav")
	if err := os.WriteFile(saveFile, saveBytes, 0o644); err != nil {
		t.Fatalf("write save: %v", err)
	}

	// Login with uppercased password — the load-bearing case.
	resp, err := h.PlayerLogin(t.Context(), &loginpb.PlayerLoginRequest{
		NodeId:        1,
		Profile:       "main",
		NodeMembers:   true,
		Username:      "caseuser",
		Password:      "LOWERPW",
		Uid:           42,
		RemoteAddress: "192.168.1.1:12345",
	})
	if err != nil {
		t.Fatalf("PlayerLogin: %v", err)
	}
	if resp.Result != loginpb.LoginResult_LOGIN_RESULT_OK {
		t.Errorf("Result: got %v, want LOGIN_RESULT_OK (TS LoginServer.ts:233 must lowercase password before bcrypt.Compare)", resp.Result)
	}
}

// TestPlayerLogin_AutoRegister_StoresLowercaseHash pins the symmetric half
// of login-server-4: TS LoginServer.ts:213 stores the hash of the LOWERCASED
// password on auto-register. Verify directly by submitting "Hunter2" through
// the auto-register path, then reading the stored hash from the DB and
// asserting it matches "hunter2" (lowercased input). Pre-fix the stored
// hash matches the verbatim "Hunter2"; post-fix it matches "hunter2".
func TestPlayerLogin_AutoRegister_StoresLowercaseHash(t *testing.T) {
	h, _ := newTestHandler(t)

	resp, err := h.PlayerLogin(t.Context(), &loginpb.PlayerLoginRequest{
		NodeId:        1,
		Profile:       "main",
		NodeMembers:   true,
		Username:      "casenewbie",
		Password:      "Hunter2",
		Uid:           42,
		RemoteAddress: "192.168.1.1:12345",
	})
	if err != nil {
		t.Fatalf("PlayerLogin: %v", err)
	}
	if resp.Result != loginpb.LoginResult_LOGIN_RESULT_NEW_PLAYER {
		t.Fatalf("Result: got %v, want LOGIN_RESULT_NEW_PLAYER", resp.Result)
	}

	acc, err := accountByUsername(t.Context(), h.db, "casenewbie", "main")
	if err != nil {
		t.Fatalf("accountByUsername: %v", err)
	}
	if acc == nil {
		t.Fatal("expected account to be created")
	}

	// Stored hash must verify against "hunter2" (the lowercased input)
	// per TS LoginServer.ts:213 `bcrypt.hashSync(password.toLowerCase(),10)`.
	if err := bcrypt.CompareHashAndPassword([]byte(acc.Password), []byte("hunter2")); err != nil {
		t.Errorf("stored hash should verify against lowercased password %q; got: %v (TS LoginServer.ts:213 must hash the lowercased password)", "hunter2", err)
	}
}

func TestPlayerLogin_IPBanned(t *testing.T) {
	h, _ := newTestHandler(t)
	_, err := h.db.ExecContext(t.Context(),
		h.db.Rebind(`INSERT INTO ipban (ip, added_by, added_on) VALUES (?, ?, ?)`),
		"10.0.0.7", "admin", "2026-01-01 00:00:00",
	)
	if err != nil {
		t.Fatalf("insert ipban: %v", err)
	}

	resp, err := h.PlayerLogin(t.Context(), &loginpb.PlayerLoginRequest{
		NodeId:        1,
		Profile:       "main",
		NodeMembers:   true,
		Username:      "someuser",
		Password:      "pw",
		Uid:           42,
		RemoteAddress: "10.0.0.7:12345",
	})
	if err != nil {
		t.Fatalf("PlayerLogin: %v", err)
	}
	if resp.Result != loginpb.LoginResult_LOGIN_RESULT_IP_BANNED {
		t.Errorf("Result: got %v, want LOGIN_RESULT_IP_BANNED", resp.Result)
	}
}

func TestPlayerLogin_AccountDisabled(t *testing.T) {
	h, _ := newTestHandler(t)
	insertTestAccount(t, h.db, "banneduser", "pw")
	// Set banned_until in the future
	until := time.Now().Add(24 * time.Hour).UTC()
	_, err := h.db.ExecContext(t.Context(),
		h.db.Rebind(`UPDATE account SET banned_until = ? WHERE username = ?`),
		until, "banneduser",
	)
	if err != nil {
		t.Fatalf("set banned_until: %v", err)
	}

	resp, err := h.PlayerLogin(t.Context(), &loginpb.PlayerLoginRequest{
		NodeId:        1,
		Profile:       "main",
		NodeMembers:   true,
		Username:      "banneduser",
		Password:      "pw",
		Uid:           42,
		RemoteAddress: "192.168.1.1:12345",
	})
	if err != nil {
		t.Fatalf("PlayerLogin: %v", err)
	}
	if resp.Result != loginpb.LoginResult_LOGIN_RESULT_ACCOUNT_DISABLED {
		t.Errorf("Result: got %v, want LOGIN_RESULT_ACCOUNT_DISABLED", resp.Result)
	}
}

func TestPlayerLogin_AlreadyLoggedIn(t *testing.T) {
	h, _ := newTestHandler(t)
	id := insertTestAccount(t, h.db, "loggeduser", "pw")
	// Insert login row on a *different* node
	_, err := h.db.ExecContext(t.Context(),
		h.db.Rebind(`INSERT INTO account_login (account_id, profile, node_id, logged_in) VALUES (?, ?, ?, ?)`),
		id, "main", 2, 1,
	)
	if err != nil {
		t.Fatalf("insert account_login: %v", err)
	}

	resp, err := h.PlayerLogin(t.Context(), &loginpb.PlayerLoginRequest{
		NodeId:        1, // different from the 2 stored above
		Profile:       "main",
		NodeMembers:   true,
		Username:      "loggeduser",
		Password:      "pw",
		Uid:           42,
		RemoteAddress: "192.168.1.1:12345",
	})
	if err != nil {
		t.Fatalf("PlayerLogin: %v", err)
	}
	if resp.Result != loginpb.LoginResult_LOGIN_RESULT_ALREADY_LOGGED_IN {
		t.Errorf("Result: got %v, want LOGIN_RESULT_ALREADY_LOGGED_IN", resp.Result)
	}
}

func TestPlayerLogin_DuplicateInFlight(t *testing.T) {
	h, _ := newTestHandler(t)
	insertTestAccount(t, h.db, "dupuser", "pw")

	// Pre-store username in loginRequests to simulate an in-flight login
	h.loginRequests.Store("dupuser", struct{}{})
	defer h.loginRequests.Delete("dupuser")

	resp, err := h.PlayerLogin(t.Context(), &loginpb.PlayerLoginRequest{
		NodeId:        1,
		Profile:       "main",
		NodeMembers:   true,
		Username:      "dupuser",
		Password:      "pw",
		Uid:           42,
		RemoteAddress: "192.168.1.1:12345",
	})
	if err != nil {
		t.Fatalf("PlayerLogin: %v", err)
	}
	if resp.Result != loginpb.LoginResult_LOGIN_RESULT_LOGIN_IN_PROGRESS {
		t.Errorf("Result: got %v, want LOGIN_RESULT_LOGIN_IN_PROGRESS", resp.Result)
	}
}

func TestPlayerLogin_Reconnect(t *testing.T) {
	h, _ := newTestHandler(t)
	id := insertTestAccount(t, h.db, "reconuser", "pw")
	// Insert login row on the SAME node we'll log in from
	_, err := h.db.ExecContext(t.Context(),
		h.db.Rebind(`INSERT INTO account_login (account_id, profile, node_id, logged_in) VALUES (?, ?, ?, ?)`),
		id, "main", 1, 1,
	)
	if err != nil {
		t.Fatalf("insert account_login: %v", err)
	}

	resp, err := h.PlayerLogin(t.Context(), &loginpb.PlayerLoginRequest{
		NodeId:        1,
		Profile:       "main",
		NodeMembers:   true,
		Username:      "reconuser",
		Password:      "pw",
		Uid:           42,
		RemoteAddress: "192.168.1.1:12345",
		Reconnecting:  true,
		HasSave:       true,
	})
	if err != nil {
		t.Fatalf("PlayerLogin: %v", err)
	}
	if resp.Result != loginpb.LoginResult_LOGIN_RESULT_RECONNECT_OK {
		t.Errorf("Result: got %v, want LOGIN_RESULT_RECONNECT_OK", resp.Result)
	}
}

// TestPlayerLogin_SameNodeNotReconnecting_AlreadyLoggedIn pins the
// login-server-1 fix: an account already logged in on the SAME node that
// attempts a fresh (non-reconnect) login must be rejected with
// ALREADY_LOGGED_IN, not admitted into a second full-login session. TS
// (LoginServer.ts:271,318) only treats `reconnecting && logged_in===nodeId`
// as a reconnect; every other already-logged-in case (including same node,
// reconnecting=false) falls to the `else if (logged_in!==null && !==0)`
// branch → response 3.
func TestPlayerLogin_SameNodeNotReconnecting_AlreadyLoggedIn(t *testing.T) {
	h, _ := newTestHandler(t)
	id := insertTestAccount(t, h.db, "samenodeuser", "pw")
	// Insert login row on the SAME node we'll log in from.
	_, err := h.db.ExecContext(t.Context(),
		h.db.Rebind(`INSERT INTO account_login (account_id, profile, node_id, logged_in) VALUES (?, ?, ?, ?)`),
		id, "main", 1, 1,
	)
	if err != nil {
		t.Fatalf("insert account_login: %v", err)
	}

	resp, err := h.PlayerLogin(t.Context(), &loginpb.PlayerLoginRequest{
		NodeId:        1, // SAME node as the stored login row
		Profile:       "main",
		NodeMembers:   true,
		Username:      "samenodeuser",
		Password:      "pw",
		Uid:           42,
		RemoteAddress: "192.168.1.1:12345",
		Reconnecting:  false, // fresh login, not a reconnect
	})
	if err != nil {
		t.Fatalf("PlayerLogin: %v", err)
	}
	if resp.Result != loginpb.LoginResult_LOGIN_RESULT_ALREADY_LOGGED_IN {
		t.Errorf("Result: got %v, want LOGIN_RESULT_ALREADY_LOGGED_IN", resp.Result)
	}
}

func TestPlayerLogout_HappyPath(t *testing.T) {
	h, savePath := newTestHandler(t)
	insertTestAccount(t, h.db, "logoutuser", "pw")

	// Log in first so account exists and login row is set
	_, err := h.PlayerLogin(t.Context(), &loginpb.PlayerLoginRequest{
		NodeId:        1,
		Profile:       "main",
		NodeMembers:   true,
		Username:      "logoutuser",
		Password:      "pw",
		Uid:           42,
		RemoteAddress: "192.168.1.1:12345",
	})
	if err != nil {
		t.Fatalf("PlayerLogin: %v", err)
	}

	saveBytes := makeValidSave(500) // must pass verifySave to be persisted
	resp, err := h.PlayerLogout(t.Context(), &loginpb.PlayerLogoutRequest{
		NodeId:   1,
		Profile:  "main",
		Username: "logoutuser",
		Save:     saveBytes,
	})
	if err != nil {
		t.Fatalf("PlayerLogout: %v", err)
	}
	if !resp.Success {
		t.Error("expected Success=true")
	}

	// Verify save file was written
	saveFile := filepath.Join(savePath, "main", "logoutuser.sav")
	got, err := os.ReadFile(saveFile)
	if err != nil {
		t.Fatalf("read save file: %v", err)
	}
	if string(got) != string(saveBytes) {
		t.Errorf("save file: got %q, want %q", got, saveBytes)
	}
}

func TestPlayerLogout_SaveWriteFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("read-only dir semantics differ on Windows")
	}
	// On Linux, chmod 0 on the savePath root won't stop root users.
	// Instead, make the savePath a file so MkdirAll fails.
	tmp := t.TempDir()
	savePathFile := filepath.Join(tmp, "savepath-as-file")
	if err := os.WriteFile(savePathFile, []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	db := createTestDB(t)
	insertTestAccount(t, db, "logoutfail", "pw")
	h := &handler{
		db: db,
		cfg: Config{
			SavePath:     savePathFile, // file, not dir
			NodeProfile:  "main",
			AutoRegister: true,
			BCryptCost:   4,
		},
		log: noopLogger(),
	}

	_, err := h.PlayerLogout(t.Context(), &loginpb.PlayerLogoutRequest{
		NodeId:   1,
		Profile:  "main",
		Username: "logoutfail",
		Save:     makeValidSave(1), // valid so it passes the gate and reaches the failing write
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("error is not a gRPC status: %v", err)
	}
	if st.Code() != codes.Internal {
		t.Errorf("code: got %v, want Internal", st.Code())
	}
}

func TestWorldStartup(t *testing.T) {
	h, _ := newTestHandler(t)
	id := insertTestAccount(t, h.db, "wsuser", "pw")
	// Pre-insert login row as logged_in=1 on node 7
	err := upsertAccountLogin(t.Context(), h.db, int(id), "main", 7)
	if err != nil {
		t.Fatalf("upsertAccountLogin: %v", err)
	}

	_, err = h.WorldStartup(t.Context(), &loginpb.WorldStartupRequest{
		NodeId:  7,
		Profile: "main",
	})
	if err != nil {
		t.Fatalf("WorldStartup: %v", err)
	}

	var loggedIn int
	err = h.db.QueryRowContext(t.Context(),
		h.db.Rebind(`SELECT logged_in FROM account_login WHERE account_id = ? AND profile = ?`),
		id, "main",
	).Scan(&loggedIn)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if loggedIn != 0 {
		t.Errorf("logged_in: got %d, want 0", loggedIn)
	}
}

func TestPlayerBan(t *testing.T) {
	h, _ := newTestHandler(t)
	insertTestAccount(t, h.db, "banme", "pw")

	until := time.Date(2030, 1, 15, 12, 0, 0, 0, time.UTC)
	_, err := h.PlayerBan(t.Context(), &loginpb.PlayerBanRequest{
		Staff:    "mod1",
		Username: "banme",
		Until:    timestamppb.New(until),
	})
	if err != nil {
		t.Fatalf("PlayerBan: %v", err)
	}

	acc, err := accountByUsername(t.Context(), h.db, "banme", "main")
	if err != nil {
		t.Fatalf("accountByUsername: %v", err)
	}
	if !acc.BannedUntil.Valid {
		t.Fatal("BannedUntil should be set")
	}
	if !acc.BannedUntil.Time.UTC().Equal(until) {
		t.Errorf("BannedUntil: got %v, want %v", acc.BannedUntil.Time, until)
	}
}

func TestPlayerMute(t *testing.T) {
	h, _ := newTestHandler(t)
	insertTestAccount(t, h.db, "muteme", "pw")

	until := time.Date(2030, 6, 1, 0, 0, 0, 0, time.UTC)
	_, err := h.PlayerMute(t.Context(), &loginpb.PlayerMuteRequest{
		Staff:    "mod1",
		Username: "muteme",
		Until:    timestamppb.New(until),
	})
	if err != nil {
		t.Fatalf("PlayerMute: %v", err)
	}

	acc, err := accountByUsername(t.Context(), h.db, "muteme", "main")
	if err != nil {
		t.Fatalf("accountByUsername: %v", err)
	}
	if !acc.MutedUntil.Valid {
		t.Fatal("MutedUntil should be set")
	}
	if !acc.MutedUntil.Time.UTC().Equal(until) {
		t.Errorf("MutedUntil: got %v, want %v", acc.MutedUntil.Time, until)
	}
}

func TestPlayerForceLogout(t *testing.T) {
	h, _ := newTestHandler(t)
	id := insertTestAccount(t, h.db, "forceout", "pw")
	err := upsertAccountLogin(t.Context(), h.db, int(id), "main", 1)
	if err != nil {
		t.Fatalf("upsertAccountLogin: %v", err)
	}

	_, err = h.PlayerForceLogout(t.Context(), &loginpb.PlayerForceLogoutRequest{
		NodeId:   1,
		Profile:  "main",
		Username: "forceout",
	})
	if err != nil {
		t.Fatalf("PlayerForceLogout: %v", err)
	}

	var loggedIn int
	err = h.db.QueryRowContext(t.Context(),
		h.db.Rebind(`SELECT logged_in FROM account_login WHERE account_id = ? AND profile = ?`),
		id, "main",
	).Scan(&loggedIn)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if loggedIn != 0 {
		t.Errorf("logged_in: got %d, want 0", loggedIn)
	}
}

// TestPlayerForceLogout_DoesNotStampLogoutTime pins login-server-2: TS
// force-logout (LoginServer.ts:477-487) writes ONLY `logged_in:0,
// login_time:null` — never logout_time. Stamping logout_time would arm
// the M25 "save missing but logout_time set" reject on the next login.
// The graceful PlayerLogout path keeps stamping logout_time
// (TestSetLoggedOutStampsLogoutTime continues to pin that contract).
func TestPlayerForceLogout_DoesNotStampLogoutTime(t *testing.T) {
	h, _ := newTestHandler(t)
	id := insertTestAccount(t, h.db, "noStamper", "pw")
	if err := upsertAccountLogin(t.Context(), h.db, int(id), "main", 1); err != nil {
		t.Fatalf("upsertAccountLogin: %v", err)
	}

	pre, err := accountByUsername(t.Context(), h.db, "noStamper", "main")
	if err != nil {
		t.Fatalf("accountByUsername (pre): %v", err)
	}
	if pre.LogoutTime.Valid {
		t.Fatalf("precondition: logout_time should be NULL before force-logout, got %v", pre.LogoutTime.Time)
	}

	if _, err := h.PlayerForceLogout(t.Context(), &loginpb.PlayerForceLogoutRequest{
		NodeId: 1, Profile: "main", Username: "noStamper",
	}); err != nil {
		t.Fatalf("PlayerForceLogout: %v", err)
	}

	post, err := accountByUsername(t.Context(), h.db, "noStamper", "main")
	if err != nil {
		t.Fatalf("accountByUsername (post): %v", err)
	}
	if post.LogoutTime.Valid {
		t.Errorf("logout_time: got %v, want NULL — TS LoginServer.ts:477-487 force-logout does not stamp logout_time (login-server-2)", post.LogoutTime.Time)
	}
}

func TestPlayerAutosave(t *testing.T) {
	h, savePath := newTestHandler(t)
	insertTestAccount(t, h.db, "autosaveuser", "pw")

	saveBytes := makeValidSave(500) // must pass verifySave to be persisted
	_, err := h.PlayerAutosave(t.Context(), &loginpb.PlayerAutosaveRequest{
		Profile:  "main",
		Username: "autosaveuser",
		Save:     saveBytes,
	})
	if err != nil {
		t.Fatalf("PlayerAutosave: %v", err)
	}

	saveFile := filepath.Join(savePath, "main", "autosaveuser.sav")
	got, err := os.ReadFile(saveFile)
	if err != nil {
		t.Fatalf("read save file: %v", err)
	}
	if string(got) != string(saveBytes) {
		t.Errorf("save: got %q, want %q", got, saveBytes)
	}
}

// TestPlayerLogin_SessionUUID_FormatOnAccept pins that every positive
// PlayerLogin response carries a valid UUID v4 in SessionUuid. Slice 7.
func TestPlayerLogin_SessionUUID_FormatOnAccept(t *testing.T) {
	h, _ := newTestHandler(t)

	resp, err := h.PlayerLogin(t.Context(), &loginpb.PlayerLoginRequest{
		NodeId:        1,
		Profile:       "main",
		NodeMembers:   true,
		Username:      "uuidplayer",
		Password:      "pw",
		Uid:           42,
		RemoteAddress: "192.168.1.1:12345",
	})
	if err != nil {
		t.Fatalf("PlayerLogin: %v", err)
	}
	if resp.SessionUuid == "" {
		t.Fatal("SessionUuid: got empty, want a UUID v4")
	}
	u, err := uuid.Parse(resp.SessionUuid)
	if err != nil {
		t.Fatalf("uuid.Parse(%q): %v", resp.SessionUuid, err)
	}
	// RFC 9562 4.2: the version is the high nibble of octet 6. The stdlib
	// uuid.UUID is a [16]byte with no Version accessor.
	if v := u[6] >> 4; v != 4 {
		t.Errorf("uuid version: got %d, want 4", v)
	}
}

// TestPlayerLogin_SessionUUID_PersistedInDB pins that the SessionUuid
// returned in the PlayerLoginResponse is the same value that lands in
// the `session` table's session_uuid column. Slice 7.
func TestPlayerLogin_SessionUUID_PersistedInDB(t *testing.T) {
	h, _ := newTestHandler(t)

	resp, err := h.PlayerLogin(t.Context(), &loginpb.PlayerLoginRequest{
		NodeId:        1,
		Profile:       "main",
		NodeMembers:   true,
		Username:      "persistuser",
		Password:      "pw",
		Uid:           42,
		RemoteAddress: "192.168.1.1:12345",
	})
	if err != nil {
		t.Fatalf("PlayerLogin: %v", err)
	}
	if resp.SessionUuid == "" {
		t.Fatal("SessionUuid empty: handler did not generate one")
	}

	var stored string
	if err := h.db.QueryRowContext(t.Context(),
		h.db.Rebind(`SELECT session_uuid FROM session WHERE account_id = ?`),
		resp.AccountId,
	).Scan(&stored); err != nil {
		t.Fatalf("SELECT session_uuid: %v", err)
	}
	if stored != resp.SessionUuid {
		t.Errorf("session table session_uuid = %q, response SessionUuid = %q; want equal", stored, resp.SessionUuid)
	}
}

// TestPlayerLogin_SessionUUID_FreshPerLogin pins that two distinct
// PlayerLogin calls produce distinct UUIDs. Slice 7.
func TestPlayerLogin_SessionUUID_FreshPerLogin(t *testing.T) {
	h, _ := newTestHandler(t)

	resp1, err := h.PlayerLogin(t.Context(), &loginpb.PlayerLoginRequest{
		NodeId: 1, Profile: "main", NodeMembers: true,
		Username: "u1", Password: "pw", Uid: 1,
		RemoteAddress: "192.168.1.1:12345",
	})
	if err != nil {
		t.Fatalf("PlayerLogin(u1): %v", err)
	}
	resp2, err := h.PlayerLogin(t.Context(), &loginpb.PlayerLoginRequest{
		NodeId: 1, Profile: "main", NodeMembers: true,
		Username: "u2", Password: "pw", Uid: 2,
		RemoteAddress: "192.168.1.1:12345",
	})
	if err != nil {
		t.Fatalf("PlayerLogin(u2): %v", err)
	}
	if resp1.SessionUuid == "" || resp2.SessionUuid == "" {
		t.Fatalf("empty UUIDs: u1=%q u2=%q", resp1.SessionUuid, resp2.SessionUuid)
	}
	if resp1.SessionUuid == resp2.SessionUuid {
		t.Errorf("UUIDs collided: u1=%s u2=%s", resp1.SessionUuid, resp2.SessionUuid)
	}
}

// TestPlayerLogin_SessionUUID_EmptyOnEarlyReject pins that the four
// early-return paths that bypass buildLoginResponse return an empty
// SessionUuid:
//   - LOGIN_IN_PROGRESS (duplicate in-flight)
//   - IP_BANNED
//   - INVALID_CREDENTIALS (auto-register disabled, account absent)
//   - INVALID_CREDENTIALS (account present, wrong password)
//
// Paths that route through buildLoginResponse (ALREADY_LOGGED_IN,
// ACCOUNT_DISABLED, NOT_A_MEMBER, OK, NEW_PLAYER, RECONNECT_OK) do
// populate the field — covered by the format/persisted tests above.
func TestPlayerLogin_SessionUUID_EmptyOnEarlyReject(t *testing.T) {
	t.Run("LOGIN_IN_PROGRESS", func(t *testing.T) {
		h, _ := newTestHandler(t)
		insertTestAccount(t, h.db, "dupuser", "pw")
		h.loginRequests.Store("dupuser", struct{}{})
		defer h.loginRequests.Delete("dupuser")

		resp, err := h.PlayerLogin(t.Context(), &loginpb.PlayerLoginRequest{
			NodeId: 1, Profile: "main", NodeMembers: true,
			Username: "dupuser", Password: "pw", Uid: 42,
			RemoteAddress: "192.168.1.1:12345",
		})
		if err != nil {
			t.Fatalf("PlayerLogin: %v", err)
		}
		if resp.Result != loginpb.LoginResult_LOGIN_RESULT_LOGIN_IN_PROGRESS {
			t.Fatalf("Result: got %v, want LOGIN_IN_PROGRESS", resp.Result)
		}
		if resp.SessionUuid != "" {
			t.Errorf("SessionUuid: got %q, want empty (early-return path)", resp.SessionUuid)
		}
	})

	t.Run("IP_BANNED", func(t *testing.T) {
		h, _ := newTestHandler(t)
		_, err := h.db.ExecContext(t.Context(),
			h.db.Rebind(`INSERT INTO ipban (ip, added_by, added_on) VALUES (?, ?, ?)`),
			"10.0.0.7", "admin", "2026-01-01 00:00:00",
		)
		if err != nil {
			t.Fatalf("insert ipban: %v", err)
		}
		resp, err := h.PlayerLogin(t.Context(), &loginpb.PlayerLoginRequest{
			NodeId: 1, Profile: "main", NodeMembers: true,
			Username: "some", Password: "pw", Uid: 42,
			RemoteAddress: "10.0.0.7:12345",
		})
		if err != nil {
			t.Fatalf("PlayerLogin: %v", err)
		}
		if resp.Result != loginpb.LoginResult_LOGIN_RESULT_IP_BANNED {
			t.Fatalf("Result: got %v, want IP_BANNED", resp.Result)
		}
		if resp.SessionUuid != "" {
			t.Errorf("SessionUuid: got %q, want empty", resp.SessionUuid)
		}
	})

	t.Run("INVALID_CREDENTIALS_AutoRegisterOff", func(t *testing.T) {
		h, _ := newTestHandler(t)
		h.cfg.AutoRegister = false
		resp, err := h.PlayerLogin(t.Context(), &loginpb.PlayerLoginRequest{
			NodeId: 1, Profile: "main", NodeMembers: true,
			Username: "ghost", Password: "pw", Uid: 42,
			RemoteAddress: "192.168.1.1:12345",
		})
		if err != nil {
			t.Fatalf("PlayerLogin: %v", err)
		}
		if resp.Result != loginpb.LoginResult_LOGIN_RESULT_INVALID_CREDENTIALS {
			t.Fatalf("Result: got %v, want INVALID_CREDENTIALS", resp.Result)
		}
		if resp.SessionUuid != "" {
			t.Errorf("SessionUuid: got %q, want empty", resp.SessionUuid)
		}
	})

	t.Run("INVALID_CREDENTIALS_WrongPassword", func(t *testing.T) {
		h, _ := newTestHandler(t)
		insertTestAccount(t, h.db, "creduser", "rightpw")
		resp, err := h.PlayerLogin(t.Context(), &loginpb.PlayerLoginRequest{
			NodeId: 1, Profile: "main", NodeMembers: true,
			Username: "creduser", Password: "wrongpw", Uid: 42,
			RemoteAddress: "192.168.1.1:12345",
		})
		if err != nil {
			t.Fatalf("PlayerLogin: %v", err)
		}
		if resp.Result != loginpb.LoginResult_LOGIN_RESULT_INVALID_CREDENTIALS {
			t.Fatalf("Result: got %v, want INVALID_CREDENTIALS", resp.Result)
		}
		if resp.SessionUuid != "" {
			t.Errorf("SessionUuid: got %q, want empty", resp.SessionUuid)
		}
	})
}

// TestPlayerLogout_WritesHiscores pins login-server-9: a graceful logout exports
// the player's enabled-stat XP into hiscore_large (type 0) end-to-end through the
// handler, mirroring TS LoginServer.ts:450.
func TestPlayerLogout_WritesHiscores(t *testing.T) {
	h, _ := newTestHandler(t)
	id := int(insertTestAccount(t, h.db, "logouths", "pw"))

	var levels [objtype.PlayerStatCount]int
	for i := range levels {
		levels[i] = 1
	}
	levels[objtype.PlayerStatAttack] = 25
	save := makeSaveWithStats(0, statsForLevels(levels))

	resp, err := h.PlayerLogout(t.Context(), &loginpb.PlayerLogoutRequest{
		NodeId: 1, Profile: "main", Username: "logouths", Save: save,
	})
	if err != nil {
		t.Fatalf("PlayerLogout: %v", err)
	}
	if !resp.Success {
		t.Error("PlayerLogout returned Success=false")
	}

	if _, _, _, found := queryHiscoreRow(t, h.db, "hiscore_large", id, 0); !found {
		t.Error("PlayerLogout did not export hiscores (hiscore_large type 0 missing)")
	}
	if _, _, _, found := queryHiscoreRow(t, h.db, "hiscore", id, objtype.PlayerStatAttack+1); !found {
		t.Error("PlayerLogout did not export the attack hiscore row (level 25 >= 15)")
	}
}

// requireCode fails the test unless err is a gRPC status with the given code.
func requireCode(t *testing.T, err error, want codes.Code) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error with code %v, got nil", want)
	}
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got %T: %v", err, err)
	}
	if st.Code() != want {
		t.Fatalf("error code: got %v, want %v (%v)", st.Code(), want, err)
	}
}

// TestPlayerLogin_ReconnectReservesLostSave pins M27: a reconnecting client that
// lost its save (HasSave=false, which the world always sends) must have its save
// read back from disk and re-served with RECONNECT_OK. Mirrors TS
// LoginServer.ts:284-300 (!hasSave -> readFile + send).
func TestPlayerLogin_ReconnectReservesLostSave(t *testing.T) {
	h, savePath := newTestHandler(t)
	id := insertTestAccount(t, h.db, "reconlost", "pw")
	if _, err := h.db.ExecContext(t.Context(),
		h.db.Rebind(`INSERT INTO account_login (account_id, profile, node_id, logged_in) VALUES (?, ?, ?, ?)`),
		id, "main", 1, 1,
	); err != nil {
		t.Fatalf("insert account_login: %v", err)
	}

	want := makeValidSave(1234)
	if err := os.MkdirAll(filepath.Join(savePath, "main"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(savePath, "main", "reconlost.sav"), want, 0o644); err != nil {
		t.Fatal(err)
	}

	resp, err := h.PlayerLogin(t.Context(), &loginpb.PlayerLoginRequest{
		NodeId: 1, Profile: "main", NodeMembers: true,
		Username: "reconlost", Password: "pw", Uid: 42,
		RemoteAddress: "192.168.1.1:1", Reconnecting: true, HasSave: false,
	})
	if err != nil {
		t.Fatalf("PlayerLogin: %v", err)
	}
	if resp.Result != loginpb.LoginResult_LOGIN_RESULT_RECONNECT_OK {
		t.Fatalf("Result: got %v, want RECONNECT_OK", resp.Result)
	}
	if string(resp.GetSave()) != string(want) {
		t.Errorf("re-served save: got %d bytes, want %d (the on-disk save)", len(resp.GetSave()), len(want))
	}
}

// TestPlayerLogin_ReconnectRejectsUnreadableSave pins the M27 safety branch: a
// reconnecting client with no readable save on disk is rejected (DataLoss),
// matching TS rejectLoginForSafety (LoginServer.ts:288-290), not resumed blank.
func TestPlayerLogin_ReconnectRejectsUnreadableSave(t *testing.T) {
	h, _ := newTestHandler(t)
	id := insertTestAccount(t, h.db, "reconnosave", "pw")
	if _, err := h.db.ExecContext(t.Context(),
		h.db.Rebind(`INSERT INTO account_login (account_id, profile, node_id, logged_in) VALUES (?, ?, ?, ?)`),
		id, "main", 1, 1,
	); err != nil {
		t.Fatalf("insert account_login: %v", err)
	}

	_, err := h.PlayerLogin(t.Context(), &loginpb.PlayerLoginRequest{
		NodeId: 1, Profile: "main", NodeMembers: true,
		Username: "reconnosave", Password: "pw", Uid: 42,
		RemoteAddress: "192.168.1.1:1", Reconnecting: true, HasSave: false,
	})
	requireCode(t, err, codes.DataLoss)
}

// TestSetLoggedOutStampsLogoutTime pins M26: PlayerLogout must stamp
// account.logout_time (TS LoginServer.ts:429-440), which arms the M25 safety
// reject. It starts NULL and must be non-NULL after logout.
func TestSetLoggedOutStampsLogoutTime(t *testing.T) {
	h, _ := newTestHandler(t)
	insertTestAccount(t, h.db, "stamper", "pw")
	if _, err := h.PlayerLogin(t.Context(), &loginpb.PlayerLoginRequest{
		NodeId: 1, Profile: "main", NodeMembers: true,
		Username: "stamper", Password: "pw", Uid: 1, RemoteAddress: "1.2.3.4:5",
	}); err != nil {
		t.Fatalf("PlayerLogin: %v", err)
	}

	acc, err := accountByUsername(t.Context(), h.db, "stamper", "main")
	if err != nil {
		t.Fatalf("accountByUsername: %v", err)
	}
	if acc.LogoutTime.Valid {
		t.Fatalf("precondition: logout_time should be NULL before logout, got %v", acc.LogoutTime.Time)
	}

	if _, err := h.PlayerLogout(t.Context(), &loginpb.PlayerLogoutRequest{
		NodeId: 1, Profile: "main", Username: "stamper", Save: makeValidSave(10),
	}); err != nil {
		t.Fatalf("PlayerLogout: %v", err)
	}

	acc, err = accountByUsername(t.Context(), h.db, "stamper", "main")
	if err != nil {
		t.Fatalf("accountByUsername after logout: %v", err)
	}
	if !acc.LogoutTime.Valid {
		t.Error("logout_time: still NULL after logout; setLoggedOut did not stamp it")
	}
}

// TestPlayerLogin_SaveMissingWithLogoutTimeRejected pins the M25+M26 chain
// end-to-end: a player logs in, logs out (writing a save + stamping logout_time),
// then the save vanishes from disk. The next login must be rejected for safety
// (DataLoss) rather than silently resetting the character to fresh. TS
// LoginServer.ts:342-348.
func TestPlayerLogin_SaveMissingWithLogoutTimeRejected(t *testing.T) {
	h, savePath := newTestHandler(t)
	insertTestAccount(t, h.db, "vanished", "pw")
	if _, err := h.PlayerLogin(t.Context(), &loginpb.PlayerLoginRequest{
		NodeId: 1, Profile: "main", NodeMembers: true,
		Username: "vanished", Password: "pw", Uid: 1, RemoteAddress: "1.2.3.4:5",
	}); err != nil {
		t.Fatalf("first PlayerLogin: %v", err)
	}
	if _, err := h.PlayerLogout(t.Context(), &loginpb.PlayerLogoutRequest{
		NodeId: 1, Profile: "main", Username: "vanished", Save: makeValidSave(99),
	}); err != nil {
		t.Fatalf("PlayerLogout: %v", err)
	}

	// Simulate data loss: the save file disappears.
	if err := os.Remove(filepath.Join(savePath, "main", "vanished.sav")); err != nil {
		t.Fatalf("remove save: %v", err)
	}

	_, err := h.PlayerLogin(t.Context(), &loginpb.PlayerLoginRequest{
		NodeId: 1, Profile: "main", NodeMembers: true,
		Username: "vanished", Password: "pw", Uid: 1, RemoteAddress: "1.2.3.4:5",
	})
	requireCode(t, err, codes.DataLoss)
}

// TestPlayerLogin_SaveMissingNoLogoutTimeIsNewPlayer pins the M25 complement:
// an existing account that never logged out (logout_time NULL) with no save is
// still a legitimate NEW_PLAYER, not a safety reject.
func TestPlayerLogin_SaveMissingNoLogoutTimeIsNewPlayer(t *testing.T) {
	h, _ := newTestHandler(t)
	insertTestAccount(t, h.db, "fresh", "pw") // logout_time NULL, no save on disk

	resp, err := h.PlayerLogin(t.Context(), &loginpb.PlayerLoginRequest{
		NodeId: 1, Profile: "main", NodeMembers: true,
		Username: "fresh", Password: "pw", Uid: 1, RemoteAddress: "1.2.3.4:5",
	})
	if err != nil {
		t.Fatalf("PlayerLogin: %v", err)
	}
	if resp.Result != loginpb.LoginResult_LOGIN_RESULT_NEW_PLAYER {
		t.Errorf("Result: got %v, want NEW_PLAYER (NULL logout_time must not reject)", resp.Result)
	}
}

// TestPlayerLogin_RateLimit pins TS LoginServer.ts:234-268: per-attempt
// `login` rows keyed (account, ip); 3 rows inside 5s → response 8
// (LOGIN_RESULT_RATE_LIMITED) BEFORE the password compare; a rejected
// attempt does NOT insert a row.
func TestPlayerLogin_RateLimit(t *testing.T) {
	h, _ := newTestHandler(t)
	req := func(pw string) *loginpb.PlayerLoginRequest {
		return &loginpb.PlayerLoginRequest{
			NodeId: 10, Profile: "main", Username: "bob", Password: pw,
			RemoteAddress: "1.2.3.4:5", Uid: 1,
		}
	}
	// Attempt 1 registers the account (NEW_PLAYER) and logs row 1.
	resp, err := h.PlayerLogin(t.Context(), req("pw"))
	if err != nil || resp.Result != loginpb.LoginResult_LOGIN_RESULT_NEW_PLAYER {
		t.Fatalf("attempt 1: %v / %v", resp, err)
	}
	// Logout so attempts 2-3 are not ALREADY_LOGGED_IN. Force-logout does
	// NOT stamp logout_time, so M25 stays unarmed.
	if _, err := h.PlayerForceLogout(t.Context(), &loginpb.PlayerForceLogoutRequest{
		NodeId: 10, Profile: "main", Username: "bob",
	}); err != nil {
		t.Fatal(err)
	}
	// Attempts 2-3: wrong password — still insert attempt rows (the TS
	// insert precedes the bcrypt compare).
	for i := 2; i <= 3; i++ {
		resp, err = h.PlayerLogin(t.Context(), req("wrong"))
		if err != nil || resp.Result != loginpb.LoginResult_LOGIN_RESULT_INVALID_CREDENTIALS {
			t.Fatalf("attempt %d: %v / %v", i, resp, err)
		}
	}
	// Attempt 4 inside the window: rate limited, even with the right password.
	resp, err = h.PlayerLogin(t.Context(), req("pw"))
	if err != nil || resp.Result != loginpb.LoginResult_LOGIN_RESULT_RATE_LIMITED {
		t.Fatalf("attempt 4: got %v / %v, want RATE_LIMITED", resp, err)
	}
	// Exactly 3 rows (the rejected attempt did not insert).
	var n int
	if err := h.db.QueryRow(`SELECT COUNT(*) FROM login`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Errorf("login rows: got %d, want 3", n)
	}
}

// TestPlayerLogin_RateLimit_ScopedToAccountAndIP pins the window key:
// a different IP for the same account is NOT limited (TS keys the
// window by account_id AND ip, LoginServer.ts:238-239).
func TestPlayerLogin_RateLimit_ScopedToAccountAndIP(t *testing.T) {
	h, _ := newTestHandler(t)
	mk := func(addr string) *loginpb.PlayerLoginRequest {
		return &loginpb.PlayerLoginRequest{
			NodeId: 10, Profile: "main", Username: "bob", Password: "wrong",
			RemoteAddress: addr, Uid: 1,
		}
	}
	if _, err := h.PlayerLogin(t.Context(), &loginpb.PlayerLoginRequest{
		NodeId: 10, Profile: "main", Username: "bob", Password: "pw",
		RemoteAddress: "1.2.3.4:5", Uid: 1,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.PlayerForceLogout(t.Context(), &loginpb.PlayerForceLogoutRequest{
		NodeId: 10, Profile: "main", Username: "bob",
	}); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if _, err := h.PlayerLogin(t.Context(), mk("1.2.3.4:5")); err != nil {
			t.Fatal(err)
		}
	}
	// Attempt from IP B: not limited (INVALID_CREDENTIALS, not RATE_LIMITED).
	resp, err := h.PlayerLogin(t.Context(), mk("9.9.9.9:5"))
	if err != nil || resp.Result != loginpb.LoginResult_LOGIN_RESULT_INVALID_CREDENTIALS {
		t.Errorf("other-IP attempt: got %v / %v, want INVALID_CREDENTIALS", resp, err)
	}
	// And the same-IP side of the scope still limits: IP A has 3 rows.
	resp, err = h.PlayerLogin(t.Context(), mk("1.2.3.4:5"))
	if err != nil || resp.Result != loginpb.LoginResult_LOGIN_RESULT_RATE_LIMITED {
		t.Errorf("same-IP attempt: got %v / %v, want RATE_LIMITED", resp, err)
	}
}

// TestPlayerLogin_RateLimit_WindowExpiry pins the 5s window edge: rows
// older than 5s do not count (TS timestamp >= now-5000,
// LoginServer.ts:240). Backdates the rows directly rather than sleeping.
func TestPlayerLogin_RateLimit_WindowExpiry(t *testing.T) {
	h, _ := newTestHandler(t)
	seed := &loginpb.PlayerLoginRequest{
		NodeId: 10, Profile: "main", Username: "bob", Password: "pw",
		RemoteAddress: "1.2.3.4:5", Uid: 1,
	}
	if _, err := h.PlayerLogin(t.Context(), seed); err != nil {
		t.Fatal(err)
	}
	if _, err := h.PlayerForceLogout(t.Context(), &loginpb.PlayerForceLogoutRequest{
		NodeId: 10, Profile: "main", Username: "bob",
	}); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if _, err := h.PlayerLogin(t.Context(), &loginpb.PlayerLoginRequest{
			NodeId: 10, Profile: "main", Username: "bob", Password: "wrong",
			RemoteAddress: "1.2.3.4:5", Uid: 1,
		}); err != nil {
			t.Fatal(err)
		}
	}
	// Backdate all 3 rows past the window.
	if _, err := h.db.Exec(h.db.Rebind(`UPDATE login SET timestamp = ?`),
		time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	resp, err := h.PlayerLogin(t.Context(), seed)
	if err != nil || (resp.Result != loginpb.LoginResult_LOGIN_RESULT_OK &&
		resp.Result != loginpb.LoginResult_LOGIN_RESULT_NEW_PLAYER) {
		t.Errorf("post-window attempt: got %v / %v, want OK/NEW_PLAYER", resp, err)
	}
}

// hopTimerFixture registers `bob` (password "pw"), force-logs-out, then
// directly seeds account_login.logged_out/logout_time to simulate a
// graceful logout from another node. Returns the handler.
func hopTimerFixture(t *testing.T, loggedOut int, logoutAge time.Duration, staffLvl int) *handler {
	t.Helper()
	h, savePath := newTestHandler(t)
	if _, err := h.PlayerLogin(t.Context(), &loginpb.PlayerLoginRequest{
		NodeId: 11, Profile: "main", Username: "bob", Password: "pw",
		RemoteAddress: "1.2.3.4:5", Uid: 1,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.PlayerForceLogout(t.Context(), &loginpb.PlayerForceLogoutRequest{
		NodeId: 11, Profile: "main", Username: "bob",
	}); err != nil {
		t.Fatal(err)
	}
	// Resolve bob's real id rather than assuming AUTOINCREMENT id 1 —
	// keeps the fixture correct if seeding order ever changes.
	acc, err := accountByUsername(t.Context(), h.db, "bob", "main")
	if err != nil || acc == nil {
		t.Fatalf("hopTimerFixture account lookup: %v / %v", acc, err)
	}
	lt := time.Now().UTC().Add(-logoutAge)
	if _, err := h.db.Exec(h.db.Rebind(`UPDATE account_login SET logged_out = ?, logout_time = ?
	                        WHERE account_id = ? AND profile = 'main'`), loggedOut, lt, acc.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := h.db.Exec(h.db.Rebind(`UPDATE account SET staff_mod_level = ? WHERE id = ?`), staffLvl, acc.ID); err != nil {
		t.Fatal(err)
	}
	// A valid save must exist or the M25 missing-save reject fires first
	// (logout_time is set by this fixture).
	saveDir := filepath.Join(savePath, "main")
	if err := os.MkdirAll(saveDir, 0o755); err != nil {
		t.Fatalf("hopTimerFixture mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(saveDir, "bob.sav"), makeValidSave(100), 0o644); err != nil {
		t.Fatalf("hopTimerFixture write save: %v", err)
	}
	return h
}

// TestPlayerLogin_HopTimer pins TS LoginServer.ts:366-379: a non-staff
// account that gracefully logged out of ANOTHER world < 45s ago is
// rejected with response 6 (LOGIN_RESULT_HOP_TIMER).
// Each sub-test runs in its own t.Run so createTestDB picks a unique
// in-memory DSN (keyed by t.Name()); without sub-tests all calls to
// hopTimerFixture would share the same DB and accumulate rate-limit rows.
func TestPlayerLogin_HopTimer(t *testing.T) {
	attempt := func(t *testing.T, h *handler) loginpb.LoginResult {
		t.Helper()
		resp, err := h.PlayerLogin(t.Context(), &loginpb.PlayerLoginRequest{
			NodeId: 10, Profile: "main", Username: "bob", Password: "pw",
			RemoteAddress: "5.6.7.8:5", Uid: 1, // distinct IP — stays under the rate limit
		})
		if err != nil {
			t.Fatalf("PlayerLogin: %v", err)
		}
		return resp.Result
	}
	// Fires: other node (11 != 10), 10s ago, staff 0.
	t.Run("fires", func(t *testing.T) {
		if got := attempt(t, hopTimerFixture(t, 11, 10*time.Second, 0)); got != loginpb.LoginResult_LOGIN_RESULT_HOP_TIMER {
			t.Errorf("hop case: got %v, want HOP_TIMER", got)
		}
	})
	// Bypass: same node (logged_out == nodeId 10).
	t.Run("same_node", func(t *testing.T) {
		if got := attempt(t, hopTimerFixture(t, 10, 10*time.Second, 0)); got != loginpb.LoginResult_LOGIN_RESULT_OK {
			t.Errorf("same-node case: got %v, want OK", got)
		}
	})
	// Bypass: logged_out == 0 (no recorded origin; backfill posture).
	t.Run("logged_out_zero", func(t *testing.T) {
		if got := attempt(t, hopTimerFixture(t, 0, 10*time.Second, 0)); got != loginpb.LoginResult_LOGIN_RESULT_OK {
			t.Errorf("logged_out=0 case: got %v, want OK", got)
		}
	})
	// Bypass: outside the 45s window.
	t.Run("window_expired", func(t *testing.T) {
		if got := attempt(t, hopTimerFixture(t, 11, 46*time.Second, 0)); got != loginpb.LoginResult_LOGIN_RESULT_OK {
			t.Errorf(">45s case: got %v, want OK", got)
		}
	})
	// Bypass: staffmodlevel >= 2 (supermod tier, B3 T18).
	t.Run("staff_bypass", func(t *testing.T) {
		if got := attempt(t, hopTimerFixture(t, 11, 10*time.Second, 2)); got != loginpb.LoginResult_LOGIN_RESULT_OK {
			t.Errorf("staff case: got %v, want OK", got)
		}
	})
}

// TestPlayerLogin_MessageCountWired pins that the unread count reaches
// PlayerLoginResponse.message_count on the full-login path (TS
// LoginServer.ts:395 + :433) — previously a stub 0.
func TestPlayerLogin_MessageCountWired(t *testing.T) {
	h, _ := newTestHandler(t)
	if _, err := h.PlayerLogin(t.Context(), &loginpb.PlayerLoginRequest{
		NodeId: 10, Profile: "main", Username: "bob", Password: "pw",
		RemoteAddress: "1.2.3.4:5", Uid: 1,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.PlayerForceLogout(t.Context(), &loginpb.PlayerForceLogoutRequest{
		NodeId: 10, Profile: "main", Username: "bob",
	}); err != nil {
		t.Fatal(err)
	}
	// One unread thread to account 1 from account 99.
	if _, err := h.db.Exec(`INSERT INTO message_thread (to_account_id, from_account_id, last_message_from, subject)
	                        VALUES (1, 99, 99, 's')`); err != nil {
		t.Fatal(err)
	}
	if _, err := h.db.Exec(`INSERT INTO message (thread_id, sender_id, sender_ip, content)
	                        VALUES (1, 99, '', 'm')`); err != nil {
		t.Fatal(err)
	}
	resp, err := h.PlayerLogin(t.Context(), &loginpb.PlayerLoginRequest{
		NodeId: 10, Profile: "main", Username: "bob", Password: "pw",
		RemoteAddress: "5.6.7.8:5", Uid: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.MessageCount != 1 {
		t.Errorf("MessageCount: got %d, want 1", resp.MessageCount)
	}
}

// TestPlayerLogin_M25_PerProfileLogoutTime pins the re-pointed M25
// safety reject (login-server-7 closure step iv): a missing save with a
// PER-PROFILE logout_time set rejects, while a different profile with
// NULL logout_time (legitimate first login) is admitted. This was the
// login-server-7 latent failure mode — now fixed.
func TestPlayerLogin_M25_PerProfileLogoutTime(t *testing.T) {
	h, _ := newTestHandler(t)
	resp, err := h.PlayerLogin(t.Context(), &loginpb.PlayerLoginRequest{
		NodeId: 10, Profile: "main", Username: "bob", Password: "pw",
		RemoteAddress: "1.2.3.4:5", Uid: 1,
	})
	if err != nil || resp.Result != loginpb.LoginResult_LOGIN_RESULT_NEW_PLAYER {
		t.Fatalf("first login: resp=%v err=%v", resp, err)
	}
	// Graceful logout on main with NO save written → logout_time stamped
	// per-profile on main only. (PlayerLogout's persistSaveIfValid skips
	// an invalid save without error.)
	if _, err := h.PlayerLogout(t.Context(), &loginpb.PlayerLogoutRequest{
		NodeId: 10, Profile: "main", Username: "bob", Save: []byte("bad"),
	}); err != nil {
		t.Fatalf("logout: %v", err)
	}
	// main: save missing + logout_time set → DataLoss reject (M25).
	if _, err := h.PlayerLogin(t.Context(), &loginpb.PlayerLoginRequest{
		NodeId: 10, Profile: "main", Username: "bob", Password: "pw",
		RemoteAddress: "1.2.3.4:5", Uid: 1,
	}); status.Code(err) != codes.DataLoss {
		t.Errorf("main relogin: got err %v, want codes.DataLoss", err)
	}
	// beta: same account, no beta logout_time → NEW_PLAYER admitted.
	resp, err = h.PlayerLogin(t.Context(), &loginpb.PlayerLoginRequest{
		NodeId: 10, Profile: "beta", Username: "bob", Password: "pw",
		RemoteAddress: "1.2.3.4:5", Uid: 1,
	})
	if err != nil || resp.Result != loginpb.LoginResult_LOGIN_RESULT_NEW_PLAYER {
		t.Errorf("beta first login: resp=%v err=%v; want NEW_PLAYER", resp, err)
	}
}

// TestPlayerLogin_MessageCountWired_Reconnect pins the reconnect-path
// half of the messageCount wiring (TS LoginServer.ts:322): a
// RECONNECT_OK reply carries the unread count too.
func TestPlayerLogin_MessageCountWired_Reconnect(t *testing.T) {
	h, _ := newTestHandler(t)
	id := insertTestAccount(t, h.db, "reconuser", "pw")
	// Login row on the SAME node we'll reconnect from.
	if _, err := h.db.ExecContext(t.Context(),
		h.db.Rebind(`INSERT INTO account_login (account_id, profile, node_id, logged_in) VALUES (?, ?, ?, ?)`),
		id, "main", 1, 1,
	); err != nil {
		t.Fatalf("insert account_login: %v", err)
	}
	// One unread thread to the account from account 99.
	if _, err := h.db.Exec(h.db.Rebind(`INSERT INTO message_thread (to_account_id, from_account_id, last_message_from, subject)
	                        VALUES (?, 99, 99, 's')`), id); err != nil {
		t.Fatal(err)
	}
	if _, err := h.db.Exec(`INSERT INTO message (thread_id, sender_id, sender_ip, content)
	                        VALUES (1, 99, '', 'm')`); err != nil {
		t.Fatal(err)
	}

	resp, err := h.PlayerLogin(t.Context(), &loginpb.PlayerLoginRequest{
		NodeId:        1,
		Profile:       "main",
		NodeMembers:   true,
		Username:      "reconuser",
		Password:      "pw",
		Uid:           42,
		RemoteAddress: "192.168.1.1:12345",
		Reconnecting:  true,
		HasSave:       true,
	})
	if err != nil {
		t.Fatalf("PlayerLogin: %v", err)
	}
	if resp.Result != loginpb.LoginResult_LOGIN_RESULT_RECONNECT_OK {
		t.Fatalf("Result: got %v, want LOGIN_RESULT_RECONNECT_OK", resp.Result)
	}
	if resp.MessageCount != 1 {
		t.Errorf("MessageCount on reconnect: got %d, want 1", resp.MessageCount)
	}
}
