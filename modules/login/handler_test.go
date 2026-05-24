package login

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/zsrv/goscape/pkg/loginpb"
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

func TestPlayerLogin_IPBanned(t *testing.T) {
	h, _ := newTestHandler(t)
	_, err := h.db.ExecContext(t.Context(),
		`INSERT INTO ipban (ip, added_by, added_on) VALUES (?, ?, ?)`,
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
	until := time.Now().Add(24 * time.Hour).UTC().Format(dbTimeFormat)
	_, err := h.db.ExecContext(t.Context(),
		`UPDATE account SET banned_until = ? WHERE username = ?`,
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
		`INSERT INTO account_login (account_id, profile, node_id, logged_in) VALUES (?, ?, ?, ?)`,
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
		`INSERT INTO account_login (account_id, profile, node_id, logged_in) VALUES (?, ?, ?, ?)`,
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
		`SELECT logged_in FROM account_login WHERE account_id = ? AND profile = ?`,
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
	expected := until.Format(dbTimeFormat)
	if acc.BannedUntil.String != expected {
		t.Errorf("BannedUntil: got %q, want %q", acc.BannedUntil.String, expected)
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
	expected := until.Format(dbTimeFormat)
	if acc.MutedUntil.String != expected {
		t.Errorf("MutedUntil: got %q, want %q", acc.MutedUntil.String, expected)
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
		`SELECT logged_in FROM account_login WHERE account_id = ? AND profile = ?`,
		id, "main",
	).Scan(&loggedIn)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if loggedIn != 0 {
		t.Errorf("logged_in: got %d, want 0", loggedIn)
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
	if u.Version() != 4 {
		t.Errorf("uuid version: got %d, want 4", u.Version())
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
		`SELECT session_uuid FROM session WHERE account_id = ?`,
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
			`INSERT INTO ipban (ip, added_by, added_on) VALUES (?, ?, ?)`,
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
		`INSERT INTO account_login (account_id, profile, node_id, logged_in) VALUES (?, ?, ?, ?)`,
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
		`INSERT INTO account_login (account_id, profile, node_id, logged_in) VALUES (?, ?, ?, ?)`,
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
		t.Fatalf("precondition: logout_time should be NULL before logout, got %q", acc.LogoutTime.String)
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
