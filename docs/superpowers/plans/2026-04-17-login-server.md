# Login Server Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement the Go login server as a gRPC service backed by SQLite, wire it into the world server, and replace the stub `modules/login` with a real implementation.

**Architecture:** `modules/login` runs a gRPC server implementing `LoginService` (7 RPCs). World servers connect via a thin `LoginClient` in `modules/world`. Proto definitions live in `proto/login/login.proto`; generated code lands in `pkg/loginpb/`. The world's `handleLogin` is updated to call the login client instead of the current hardcoded stub.

**Tech Stack:** Go 1.26, `google.golang.org/grpc`, `google.golang.org/protobuf`, `modernc.org/sqlite`, `golang.org/x/crypto/bcrypt`, `buf` (codegen)

---

### Task 1: Add Go module dependencies

**Files:**
- Modify: `go.mod`, `go.sum`

- [ ] **Step 1: Add the four new direct dependencies**

```bash
cd /home/owner/Code/github.com/zsrv/goscape
go get google.golang.org/grpc@latest
go get google.golang.org/protobuf@latest
go get modernc.org/sqlite@latest
go get golang.org/x/crypto@latest
```

- [ ] **Step 2: Verify the project still compiles**

```bash
CGO_ENABLED=0 go build ./...
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "feat: add gRPC, SQLite, and crypto dependencies for login server"
```

---

### Task 2: Install proto tooling and configure buf

**Files:**
- Create: `buf.yaml`
- Create: `buf.gen.yaml`
- Modify: `Makefile`
- Create: `proto/login/` (directory)

- [ ] **Step 1: Install buf and the two protoc plugins via `go install`**

```bash
go install github.com/bufbuild/buf/cmd/buf@latest
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
```

Verify all three binaries are available:

```bash
buf --version && protoc-gen-go --version && protoc-gen-go-grpc --version
```

Expected: version strings for all three (buf will print something like `1.xx.x`).

- [ ] **Step 2: Create `buf.yaml` at the project root**

```yaml
version: v2
modules:
  - path: proto
```

- [ ] **Step 3: Create `buf.gen.yaml` at the project root**

```yaml
version: v2
inputs:
  - directory: proto
plugins:
  - local: protoc-gen-go
    out: .
    opt: module=github.com/zsrv/goscape
  - local: protoc-gen-go-grpc
    out: .
    opt: module=github.com/zsrv/goscape
```

- [ ] **Step 4: Add a `proto` target to `Makefile`**

Add after the existing `build-image` target:

```makefile

.PHONY: proto
proto:
	buf generate
```

- [ ] **Step 5: Create the proto source directory**

```bash
mkdir -p proto/login
```

- [ ] **Step 6: Commit**

```bash
git add buf.yaml buf.gen.yaml Makefile proto/
git commit -m "feat: add buf proto tooling configuration"
```

---

### Task 3: Write login.proto and generate Go code

**Files:**
- Create: `proto/login/login.proto`
- Create: `pkg/loginpb/login.pb.go` (generated — commit this)
- Create: `pkg/loginpb/login_grpc.pb.go` (generated — commit this)

- [ ] **Step 1: Write `proto/login/login.proto`**

```protobuf
syntax = "proto3";
package login.v1;

import "google/protobuf/empty.proto";
import "google/protobuf/timestamp.proto";

option go_package = "github.com/zsrv/goscape/pkg/loginpb";

service LoginService {
  rpc WorldStartup(WorldStartupRequest)           returns (google.protobuf.Empty);
  rpc PlayerLogin(PlayerLoginRequest)             returns (PlayerLoginResponse);
  rpc PlayerLogout(PlayerLogoutRequest)           returns (PlayerLogoutResponse);
  rpc PlayerAutosave(PlayerAutosaveRequest)       returns (google.protobuf.Empty);
  rpc PlayerForceLogout(PlayerForceLogoutRequest) returns (google.protobuf.Empty);
  rpc PlayerBan(PlayerBanRequest)                 returns (PlayerBanResponse);
  rpc PlayerMute(PlayerMuteRequest)               returns (PlayerMuteResponse);
}

enum LoginResult {
  LOGIN_RESULT_UNSPECIFIED         = 0;
  LOGIN_RESULT_OK                  = 1;
  LOGIN_RESULT_NEW_PLAYER          = 2;
  LOGIN_RESULT_RECONNECT_OK        = 3;
  LOGIN_RESULT_INVALID_CREDENTIALS = 4;
  LOGIN_RESULT_ALREADY_LOGGED_IN   = 5;
  LOGIN_RESULT_ACCOUNT_DISABLED    = 6;
  LOGIN_RESULT_NOT_A_MEMBER        = 7;
  LOGIN_RESULT_LOGIN_IN_PROGRESS   = 8;
  LOGIN_RESULT_TRY_AGAIN           = 9;
}

message WorldStartupRequest {
  int32  node_id  = 1;
  string profile  = 2;
}

message PlayerLoginRequest {
  int32  node_id        = 1;
  string profile        = 2;
  bool   node_members   = 3;
  string username       = 4;
  string password       = 5;
  int32  uid            = 6;
  string socket         = 7;
  string remote_address = 8;
  bool   reconnecting   = 9;
  bool   has_save       = 10;
}

message PlayerLoginResponse {
  LoginResult               result          = 1;
  int32                     account_id      = 2;
  int32                     staff_mod_level = 3;
  bytes                     save            = 4;
  google.protobuf.Timestamp muted_until     = 5;
  bool                      members         = 6;
  int32                     message_count   = 7;
}

message PlayerLogoutRequest {
  int32  node_id  = 1;
  string profile  = 2;
  string username = 3;
  bytes  save     = 4;
}

message PlayerLogoutResponse {
  bool success = 1;
}

message PlayerAutosaveRequest {
  string profile  = 1;
  string username = 2;
  bytes  save     = 3;
}

message PlayerForceLogoutRequest {
  int32  node_id  = 1;
  string profile  = 2;
  string username = 3;
}

message PlayerBanRequest {
  string                    staff    = 1;
  string                    username = 2;
  google.protobuf.Timestamp until    = 3;
}

message PlayerBanResponse {}

message PlayerMuteRequest {
  string                    staff    = 1;
  string                    username = 2;
  google.protobuf.Timestamp until    = 3;
}

message PlayerMuteResponse {}
```

- [ ] **Step 2: Lint the proto file**

```bash
buf lint
```

Expected: no output (no errors).

- [ ] **Step 3: Generate Go code**

```bash
make proto
```

Expected: two new files created:
- `pkg/loginpb/login.pb.go`
- `pkg/loginpb/login_grpc.pb.go`

- [ ] **Step 4: Verify the project compiles with the new package**

```bash
CGO_ENABLED=0 go build ./...
```

Expected: no errors.

- [ ] **Step 5: Commit**

```bash
git add proto/login/login.proto pkg/loginpb/
git commit -m "feat: add login service proto definition and generated Go code"
```

---

### Task 4: DB layer — test schema + all query functions (TDD)

**Files:**
- Create: `modules/login/db_test.go`
- Create: `modules/login/db.go`

The DB layer is independent of gRPC — test it directly against in-memory SQLite.

- [ ] **Step 1: Write `modules/login/db_test.go` with schema and helper functions**

```go
package login

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
	_ "modernc.org/sqlite"
)

const testBcryptCost = 4

const testSchema = `
CREATE TABLE account (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    username          TEXT    NOT NULL UNIQUE,
    password          TEXT    NOT NULL,
    registration_ip   TEXT,
    registration_date TEXT    NOT NULL DEFAULT (datetime('now')),
    muted_until       TEXT,
    banned_until      TEXT,
    staffmodlevel     INTEGER NOT NULL DEFAULT 0,
    members           INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE account_login (
    account_id  INTEGER NOT NULL,
    profile     TEXT    NOT NULL DEFAULT 'main',
    logged_in   INTEGER NOT NULL DEFAULT 0,
    login_time  TEXT,
    logged_out  INTEGER NOT NULL DEFAULT 0,
    logout_time TEXT,
    PRIMARY KEY (account_id, profile)
);
CREATE TABLE ipban (
    ip TEXT PRIMARY KEY
);
CREATE TABLE session (
    uuid       TEXT    PRIMARY KEY,
    account_id INTEGER NOT NULL,
    profile    TEXT    NOT NULL,
    world      INTEGER NOT NULL,
    timestamp  TEXT    NOT NULL,
    uid        INTEGER NOT NULL,
    ip         TEXT
);
`

func createTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	_, err = db.Exec(testSchema)
	require.NoError(t, err)
	return db
}

func insertTestAccount(t *testing.T, db *sql.DB, username, password string) int {
	t.Helper()
	hashed, err := bcrypt.GenerateFromPassword([]byte(strings.ToLower(password)), testBcryptCost)
	require.NoError(t, err)
	res, err := db.Exec("INSERT INTO account (username, password) VALUES (?, ?)", username, string(hashed))
	require.NoError(t, err)
	id, err := res.LastInsertId()
	require.NoError(t, err)
	return int(id)
}

func noopLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestAccountByUsername_Exists(t *testing.T) {
	ctx := t.Context()
	db := createTestDB(t)
	insertTestAccount(t, db, "alice", "secret")

	row, err := accountByUsername(ctx, db, "alice", "main")
	require.NoError(t, err)
	require.NotNil(t, row)
	assert.Equal(t, "alice", row.Username)
	assert.False(t, row.HasLoginRow)
	assert.Equal(t, 0, row.LoggedIn)
}

func TestAccountByUsername_NotFound(t *testing.T) {
	ctx := t.Context()
	db := createTestDB(t)

	row, err := accountByUsername(ctx, db, "nobody", "main")
	require.NoError(t, err)
	assert.Nil(t, row)
}

func TestIpBanned(t *testing.T) {
	ctx := t.Context()
	db := createTestDB(t)
	_, err := db.Exec("INSERT INTO ipban (ip) VALUES (?)", "1.2.3.4")
	require.NoError(t, err)

	banned, err := ipBanned(ctx, db, "1.2.3.4")
	require.NoError(t, err)
	assert.True(t, banned)

	notBanned, err := ipBanned(ctx, db, "9.9.9.9")
	require.NoError(t, err)
	assert.False(t, notBanned)
}

func TestInsertAccount(t *testing.T) {
	ctx := t.Context()
	db := createTestDB(t)

	err := insertAccount(ctx, db, "bob", "hashedpw", "10.0.0.1")
	require.NoError(t, err)

	row, err := accountByUsername(ctx, db, "bob", "main")
	require.NoError(t, err)
	require.NotNil(t, row)
	assert.Equal(t, "bob", row.Username)
}

func TestUpsertAccountLogin_Insert(t *testing.T) {
	ctx := t.Context()
	db := createTestDB(t)
	id := insertTestAccount(t, db, "carol", "pw")

	require.NoError(t, upsertAccountLogin(ctx, db, id, "main", false, 10))

	row, err := accountByUsername(ctx, db, "carol", "main")
	require.NoError(t, err)
	assert.Equal(t, 10, row.LoggedIn)
	assert.True(t, row.HasLoginRow)
}

func TestUpsertAccountLogin_Update(t *testing.T) {
	ctx := t.Context()
	db := createTestDB(t)
	id := insertTestAccount(t, db, "dave", "pw")
	require.NoError(t, upsertAccountLogin(ctx, db, id, "main", false, 10))

	require.NoError(t, upsertAccountLogin(ctx, db, id, "main", true, 11))

	row, err := accountByUsername(ctx, db, "dave", "main")
	require.NoError(t, err)
	assert.Equal(t, 11, row.LoggedIn)
}

func TestClearWorldSessions(t *testing.T) {
	ctx := t.Context()
	db := createTestDB(t)
	id := insertTestAccount(t, db, "eve", "pw")
	require.NoError(t, upsertAccountLogin(ctx, db, id, "main", false, 10))

	require.NoError(t, clearWorldSessions(ctx, db, 10, "main"))

	row, err := accountByUsername(ctx, db, "eve", "main")
	require.NoError(t, err)
	assert.Equal(t, 0, row.LoggedIn)
}

func TestSetLoggedOut(t *testing.T) {
	ctx := t.Context()
	db := createTestDB(t)
	id := insertTestAccount(t, db, "frank", "pw")
	require.NoError(t, upsertAccountLogin(ctx, db, id, "main", false, 10))

	require.NoError(t, setLoggedOut(ctx, db, id, "main", 10))

	row, err := accountByUsername(ctx, db, "frank", "main")
	require.NoError(t, err)
	assert.Equal(t, 0, row.LoggedIn)
	assert.True(t, row.LogoutTime.Valid)
}

func TestSetAccountBanned(t *testing.T) {
	ctx := t.Context()
	db := createTestDB(t)
	insertTestAccount(t, db, "grace", "pw")

	require.NoError(t, setAccountBanned(ctx, db, "grace", time.Now().Add(24*time.Hour)))

	row, err := accountByUsername(ctx, db, "grace", "main")
	require.NoError(t, err)
	assert.True(t, row.BannedUntil.Valid)
}

func TestSetAccountMuted(t *testing.T) {
	ctx := t.Context()
	db := createTestDB(t)
	insertTestAccount(t, db, "henry", "pw")

	require.NoError(t, setAccountMuted(ctx, db, "henry", time.Now().Add(1*time.Hour)))

	row, err := accountByUsername(ctx, db, "henry", "main")
	require.NoError(t, err)
	assert.True(t, row.MutedUntil.Valid)
}
```

- [ ] **Step 2: Run the failing tests**

```bash
go test ./modules/login/... -v
```

Expected: FAIL — `accountByUsername`, `ipBanned`, etc. are not defined.

- [ ] **Step 3: Write `modules/login/db.go`**

```go
package login

import (
	"context"
	"database/sql"
	"errors"
	"time"

	_ "modernc.org/sqlite"
)

const dbTimeFormat = "2006-01-02 15:04:05"

func openDB(dsn string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec("PRAGMA journal_mode=WAL; PRAGMA foreign_keys=ON;"); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

type accountRow struct {
	ID            int
	Username      string
	Password      string
	StaffModLevel int
	Members       int
	MutedUntil    sql.NullString
	BannedUntil   sql.NullString
	HasLoginRow   bool
	LoggedIn      int
	LogoutTime    sql.NullString
}

func accountByUsername(ctx context.Context, db *sql.DB, username, profile string) (*accountRow, error) {
	const q = `
		SELECT a.id, a.username, a.password, a.staffmodlevel, a.members,
		       a.muted_until, a.banned_until,
		       al.account_id IS NOT NULL, COALESCE(al.logged_in, 0), al.logout_time
		FROM account a
		LEFT JOIN account_login al ON al.account_id = a.id AND al.profile = ?
		WHERE a.username = ?`
	var r accountRow
	err := db.QueryRowContext(ctx, q, profile, username).Scan(
		&r.ID, &r.Username, &r.Password, &r.StaffModLevel, &r.Members,
		&r.MutedUntil, &r.BannedUntil,
		&r.HasLoginRow, &r.LoggedIn, &r.LogoutTime,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func ipBanned(ctx context.Context, db *sql.DB, ip string) (bool, error) {
	var count int
	err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM ipban WHERE ip = ?", ip).Scan(&count)
	return count > 0, err
}

func insertAccount(ctx context.Context, db *sql.DB, username, hashedPassword, registrationIP string) error {
	_, err := db.ExecContext(ctx,
		"INSERT INTO account (username, password, registration_ip) VALUES (?, ?, ?)",
		username, hashedPassword, registrationIP,
	)
	return err
}

func setAccountMembers(ctx context.Context, db *sql.DB, accountID int) error {
	_, err := db.ExecContext(ctx, "UPDATE account SET members = 1 WHERE id = ?", accountID)
	return err
}

func upsertAccountLogin(ctx context.Context, db *sql.DB, accountID int, profile string, hasLoginRow bool, nodeID int) error {
	if hasLoginRow {
		_, err := db.ExecContext(ctx,
			"UPDATE account_login SET logged_in = ?, login_time = datetime('now') WHERE account_id = ? AND profile = ?",
			nodeID, accountID, profile,
		)
		return err
	}
	_, err := db.ExecContext(ctx,
		"INSERT INTO account_login (account_id, profile, logged_in, login_time) VALUES (?, ?, ?, datetime('now'))",
		accountID, profile, nodeID,
	)
	return err
}

func insertSession(ctx context.Context, db *sql.DB, sessionUUID string, accountID int, profile string, world, uid int, ip string) error {
	_, err := db.ExecContext(ctx,
		"INSERT INTO session (uuid, account_id, profile, world, timestamp, uid, ip) VALUES (?, ?, ?, ?, datetime('now'), ?, ?)",
		sessionUUID, accountID, profile, world, uid, ip,
	)
	return err
}

func clearWorldSessions(ctx context.Context, db *sql.DB, nodeID int, profile string) error {
	_, err := db.ExecContext(ctx,
		"UPDATE account_login SET logged_in = 0, login_time = NULL WHERE logged_in = ? AND profile = ?",
		nodeID, profile,
	)
	return err
}

func setLoggedOut(ctx context.Context, db *sql.DB, accountID int, profile string, nodeID int) error {
	_, err := db.ExecContext(ctx,
		"UPDATE account_login SET logged_in = 0, login_time = NULL, logged_out = ?, logout_time = datetime('now') WHERE account_id = ? AND profile = ?",
		nodeID, accountID, profile,
	)
	return err
}

func setAccountBanned(ctx context.Context, db *sql.DB, username string, until time.Time) error {
	_, err := db.ExecContext(ctx,
		"UPDATE account SET banned_until = ? WHERE username = ?",
		until.UTC().Format(dbTimeFormat), username,
	)
	return err
}

func setAccountMuted(ctx context.Context, db *sql.DB, username string, until time.Time) error {
	_, err := db.ExecContext(ctx,
		"UPDATE account SET muted_until = ? WHERE username = ?",
		until.UTC().Format(dbTimeFormat), username,
	)
	return err
}
```

- [ ] **Step 4: Run all DB tests**

```bash
go test ./modules/login/... -v
```

Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add modules/login/db.go modules/login/db_test.go
git commit -m "feat: add login DB query layer with tests"
```

---

### Task 5: Handler — WorldStartup + PlayerLogin (TDD)

**Files:**
- Replace: `modules/login/handler.go` (current file has HTTP asset handler — replace entirely with the gRPC handler below)
- Create: `modules/login/handler_test.go`

- [ ] **Step 1: Write `modules/login/handler_test.go`**

```go
package login

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/zsrv/goscape/pkg/loginpb"
)

func newTestHandler(t *testing.T) (*handler, string) {
	t.Helper()
	db := createTestDB(t)
	savePath := t.TempDir()
	h := &handler{
		db:  db,
		cfg: Config{BCryptCost: testBcryptCost, SavePath: savePath, AutoRegister: true, AutoSubscribeMembers: true},
		log: noopLogger(),
	}
	return h, savePath
}

func TestWorldStartup_ClearsLoggedInSessions(t *testing.T) {
	ctx := t.Context()
	h, _ := newTestHandler(t)
	id := insertTestAccount(t, h.db, "alice", "pw")
	require.NoError(t, upsertAccountLogin(ctx, h.db, id, "main", false, 10))

	_, err := h.WorldStartup(ctx, &loginpb.WorldStartupRequest{NodeId: 10, Profile: "main"})
	require.NoError(t, err)

	row, err := accountByUsername(ctx, h.db, "alice", "main")
	require.NoError(t, err)
	assert.Equal(t, 0, row.LoggedIn)
}

func TestPlayerLogin_InvalidCredentials(t *testing.T) {
	ctx := t.Context()
	h, _ := newTestHandler(t)
	insertTestAccount(t, h.db, "alice", "correct")

	resp, err := h.PlayerLogin(ctx, &loginpb.PlayerLoginRequest{
		Username: "alice", Password: "wrong", Profile: "main",
	})
	require.NoError(t, err)
	assert.Equal(t, loginpb.LoginResult_LOGIN_RESULT_INVALID_CREDENTIALS, resp.Result)
}

func TestPlayerLogin_NewPlayer_AutoRegister(t *testing.T) {
	ctx := t.Context()
	h, _ := newTestHandler(t)

	resp, err := h.PlayerLogin(ctx, &loginpb.PlayerLoginRequest{
		Username: "newplayer", Password: "pw", Profile: "main", Socket: "test-uuid",
	})
	require.NoError(t, err)
	assert.Equal(t, loginpb.LoginResult_LOGIN_RESULT_NEW_PLAYER, resp.Result)
	assert.NotZero(t, resp.AccountId)
}

func TestPlayerLogin_ExistingPlayer(t *testing.T) {
	ctx := t.Context()
	h, savePath := newTestHandler(t)
	insertTestAccount(t, h.db, "alice", "pw")

	saveFile := filepath.Join(savePath, "main", "alice.sav")
	require.NoError(t, os.MkdirAll(filepath.Dir(saveFile), 0755))
	require.NoError(t, os.WriteFile(saveFile, []byte("savebytes"), 0644))

	resp, err := h.PlayerLogin(ctx, &loginpb.PlayerLoginRequest{
		Username: "alice", Password: "pw", Profile: "main", Socket: "test-uuid",
	})
	require.NoError(t, err)
	assert.Equal(t, loginpb.LoginResult_LOGIN_RESULT_OK, resp.Result)
	assert.Equal(t, []byte("savebytes"), resp.Save)
}

func TestPlayerLogin_IPBanned(t *testing.T) {
	ctx := t.Context()
	h, _ := newTestHandler(t)
	_, err := h.db.Exec("INSERT INTO ipban (ip) VALUES (?)", "1.2.3.4")
	require.NoError(t, err)

	resp, err := h.PlayerLogin(ctx, &loginpb.PlayerLoginRequest{
		Username: "alice", Password: "pw", Profile: "main", RemoteAddress: "1.2.3.4",
	})
	require.NoError(t, err)
	assert.Equal(t, loginpb.LoginResult_LOGIN_RESULT_TRY_AGAIN, resp.Result)
}

func TestPlayerLogin_AccountDisabled(t *testing.T) {
	ctx := t.Context()
	h, _ := newTestHandler(t)
	insertTestAccount(t, h.db, "alice", "pw")
	require.NoError(t, setAccountBanned(ctx, h.db, "alice", time.Now().Add(24*time.Hour)))

	resp, err := h.PlayerLogin(ctx, &loginpb.PlayerLoginRequest{
		Username: "alice", Password: "pw", Profile: "main",
	})
	require.NoError(t, err)
	assert.Equal(t, loginpb.LoginResult_LOGIN_RESULT_ACCOUNT_DISABLED, resp.Result)
}

func TestPlayerLogin_AlreadyLoggedIn(t *testing.T) {
	ctx := t.Context()
	h, _ := newTestHandler(t)
	id := insertTestAccount(t, h.db, "alice", "pw")
	require.NoError(t, upsertAccountLogin(ctx, h.db, id, "main", false, 99))

	resp, err := h.PlayerLogin(ctx, &loginpb.PlayerLoginRequest{
		Username: "alice", Password: "pw", Profile: "main", NodeId: 10,
	})
	require.NoError(t, err)
	assert.Equal(t, loginpb.LoginResult_LOGIN_RESULT_ALREADY_LOGGED_IN, resp.Result)
}

func TestPlayerLogin_DuplicateInFlight(t *testing.T) {
	ctx := t.Context()
	h, _ := newTestHandler(t)
	h.loginRequests.Store("alice", struct{}{})

	resp, err := h.PlayerLogin(ctx, &loginpb.PlayerLoginRequest{
		Username: "alice", Password: "pw", Profile: "main",
	})
	require.NoError(t, err)
	assert.Equal(t, loginpb.LoginResult_LOGIN_RESULT_LOGIN_IN_PROGRESS, resp.Result)
}

func TestPlayerLogin_Reconnect(t *testing.T) {
	ctx := t.Context()
	h, savePath := newTestHandler(t)
	id := insertTestAccount(t, h.db, "alice", "pw")
	require.NoError(t, upsertAccountLogin(ctx, h.db, id, "main", false, 10))

	saveFile := filepath.Join(savePath, "main", "alice.sav")
	require.NoError(t, os.MkdirAll(filepath.Dir(saveFile), 0755))
	require.NoError(t, os.WriteFile(saveFile, []byte("savedata"), 0644))

	resp, err := h.PlayerLogin(ctx, &loginpb.PlayerLoginRequest{
		Username: "alice", Password: "pw", Profile: "main",
		NodeId: 10, Socket: "test-uuid", Reconnecting: true, HasSave: false,
	})
	require.NoError(t, err)
	assert.Equal(t, loginpb.LoginResult_LOGIN_RESULT_RECONNECT_OK, resp.Result)
	assert.Equal(t, []byte("savedata"), resp.Save)
}

func TestPlayerBan_SetsBannedUntil(t *testing.T) {
	ctx := t.Context()
	h, _ := newTestHandler(t)
	insertTestAccount(t, h.db, "alice", "pw")

	_, err := h.PlayerBan(ctx, &loginpb.PlayerBanRequest{
		Staff: "admin", Username: "alice", Until: timestamppb.New(time.Now().Add(48 * time.Hour)),
	})
	require.NoError(t, err)

	row, err := accountByUsername(ctx, h.db, "alice", "main")
	require.NoError(t, err)
	assert.True(t, row.BannedUntil.Valid)
}

func TestPlayerMute_SetsMutedUntil(t *testing.T) {
	ctx := t.Context()
	h, _ := newTestHandler(t)
	insertTestAccount(t, h.db, "alice", "pw")

	_, err := h.PlayerMute(ctx, &loginpb.PlayerMuteRequest{
		Staff: "admin", Username: "alice", Until: timestamppb.New(time.Now().Add(2 * time.Hour)),
	})
	require.NoError(t, err)

	row, err := accountByUsername(ctx, h.db, "alice", "main")
	require.NoError(t, err)
	assert.True(t, row.MutedUntil.Valid)
}

func TestPlayerLogout_WritesAndClearsSession(t *testing.T) {
	ctx := t.Context()
	h, savePath := newTestHandler(t)
	id := insertTestAccount(t, h.db, "alice", "pw")
	require.NoError(t, upsertAccountLogin(ctx, h.db, id, "main", false, 10))

	resp, err := h.PlayerLogout(ctx, &loginpb.PlayerLogoutRequest{
		NodeId: 10, Profile: "main", Username: "alice", Save: []byte("playerdata"),
	})
	require.NoError(t, err)
	assert.True(t, resp.Success)

	row, err := accountByUsername(ctx, h.db, "alice", "main")
	require.NoError(t, err)
	assert.Equal(t, 0, row.LoggedIn)
	assert.True(t, row.LogoutTime.Valid)

	written, err := os.ReadFile(filepath.Join(savePath, "main", "alice.sav"))
	require.NoError(t, err)
	assert.Equal(t, []byte("playerdata"), written)
}

func TestPlayerAutosave_WritesSaveFile(t *testing.T) {
	ctx := t.Context()
	h, savePath := newTestHandler(t)

	_, err := h.PlayerAutosave(ctx, &loginpb.PlayerAutosaveRequest{
		Profile: "main", Username: "alice", Save: []byte("autosavedata"),
	})
	require.NoError(t, err)

	written, err := os.ReadFile(filepath.Join(savePath, "main", "alice.sav"))
	require.NoError(t, err)
	assert.Equal(t, []byte("autosavedata"), written)
}

func TestPlayerForceLogout_ClearsLoggedIn(t *testing.T) {
	ctx := t.Context()
	h, _ := newTestHandler(t)
	id := insertTestAccount(t, h.db, "alice", "pw")
	require.NoError(t, upsertAccountLogin(ctx, h.db, id, "main", false, 10))

	_, err := h.PlayerForceLogout(ctx, &loginpb.PlayerForceLogoutRequest{
		NodeId: 10, Profile: "main", Username: "alice",
	})
	require.NoError(t, err)

	row, err := accountByUsername(ctx, h.db, "alice", "main")
	require.NoError(t, err)
	assert.Equal(t, 0, row.LoggedIn)
}
```

- [ ] **Step 2: Run failing tests**

```bash
go test ./modules/login/... -run "Test" -v
```

Expected: compile error — `handler` type not defined.

- [ ] **Step 3: Replace `modules/login/handler.go` entirely with the gRPC handler**

This replaces the current HTTP asset handler stub. Write this as the complete new file:

```go
package login

import (
	"context"
	"database/sql"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	util "github.com/zsrv/goscape/pkg/util/jstring"
	"github.com/zsrv/goscape/pkg/loginpb"
)

type handler struct {
	loginpb.UnimplementedLoginServiceServer

	db            *sql.DB
	cfg           Config
	log           *slog.Logger
	loginRequests sync.Map
}

func (h *handler) WorldStartup(ctx context.Context, req *loginpb.WorldStartupRequest) (*emptypb.Empty, error) {
	if err := clearWorldSessions(ctx, h.db, int(req.NodeId), req.Profile); err != nil {
		return nil, status.Errorf(codes.Internal, "db error: %v", err)
	}
	return &emptypb.Empty{}, nil
}

func (h *handler) PlayerLogin(ctx context.Context, req *loginpb.PlayerLoginRequest) (*loginpb.PlayerLoginResponse, error) {
	safeName := util.ToSafeName(req.Username)

	if _, loaded := h.loginRequests.LoadOrStore(safeName, struct{}{}); loaded {
		return &loginpb.PlayerLoginResponse{Result: loginpb.LoginResult_LOGIN_RESULT_LOGIN_IN_PROGRESS}, nil
	}
	defer h.loginRequests.Delete(safeName)

	banned, err := ipBanned(ctx, h.db, req.RemoteAddress)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "db error: %v", err)
	}
	if banned {
		return &loginpb.PlayerLoginResponse{Result: loginpb.LoginResult_LOGIN_RESULT_TRY_AGAIN}, nil
	}

	account, err := accountByUsername(ctx, h.db, req.Username, req.Profile)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "db error: %v", err)
	}

	if account == nil && h.cfg.AutoRegister {
		hashed, err := bcrypt.GenerateFromPassword([]byte(strings.ToLower(req.Password)), h.cfg.BCryptCost)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "bcrypt: %v", err)
		}
		if err := insertAccount(ctx, h.db, req.Username, string(hashed), req.RemoteAddress); err != nil {
			return nil, status.Errorf(codes.Internal, "insert account: %v", err)
		}
		account, err = accountByUsername(ctx, h.db, req.Username, req.Profile)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "db error: %v", err)
		}
	}

	if account == nil {
		return &loginpb.PlayerLoginResponse{Result: loginpb.LoginResult_LOGIN_RESULT_INVALID_CREDENTIALS}, nil
	}

	if err := bcrypt.CompareHashAndPassword([]byte(account.Password), []byte(strings.ToLower(req.Password))); err != nil {
		return &loginpb.PlayerLoginResponse{Result: loginpb.LoginResult_LOGIN_RESULT_INVALID_CREDENTIALS}, nil
	}

	if account.BannedUntil.Valid {
		if t, err := time.Parse(dbTimeFormat, account.BannedUntil.String); err == nil && time.Now().UTC().Before(t) {
			return &loginpb.PlayerLoginResponse{Result: loginpb.LoginResult_LOGIN_RESULT_ACCOUNT_DISABLED}, nil
		}
	}

	if req.NodeMembers && account.Members == 0 {
		if h.cfg.AutoSubscribeMembers {
			if err := setAccountMembers(ctx, h.db, account.ID); err != nil {
				return nil, status.Errorf(codes.Internal, "db error: %v", err)
			}
			account.Members = 1
		} else {
			return &loginpb.PlayerLoginResponse{Result: loginpb.LoginResult_LOGIN_RESULT_NOT_A_MEMBER}, nil
		}
	}

	if req.Reconnecting && account.LoggedIn == int(req.NodeId) {
		if err := insertSession(ctx, h.db, req.Socket, account.ID, req.Profile, int(req.NodeId), int(req.Uid), req.RemoteAddress); err != nil {
			return nil, status.Errorf(codes.Internal, "db error: %v", err)
		}
		resp := h.buildLoginResponse(loginpb.LoginResult_LOGIN_RESULT_RECONNECT_OK, account)
		if !req.HasSave {
			save, err := os.ReadFile(filepath.Join(h.cfg.SavePath, req.Profile, req.Username+".sav"))
			if err != nil {
				h.log.Error("failed to read save on reconnect", "username", req.Username, "error", err)
				return &loginpb.PlayerLoginResponse{Result: loginpb.LoginResult_LOGIN_RESULT_TRY_AGAIN}, nil
			}
			// TODO: PlayerLoading.Verify(save)
			resp.Save = save
		}
		return resp, nil
	}

	if account.LoggedIn != 0 {
		return &loginpb.PlayerLoginResponse{Result: loginpb.LoginResult_LOGIN_RESULT_ALREADY_LOGGED_IN}, nil
	}

	if err := insertSession(ctx, h.db, req.Socket, account.ID, req.Profile, int(req.NodeId), int(req.Uid), req.RemoteAddress); err != nil {
		return nil, status.Errorf(codes.Internal, "db error: %v", err)
	}

	savePath := filepath.Join(h.cfg.SavePath, req.Profile, req.Username+".sav")
	if _, statErr := os.Stat(savePath); os.IsNotExist(statErr) {
		if account.LogoutTime.Valid {
			h.log.Error("account has logout_time but no save file on disk", "username", req.Username, "account_id", account.ID)
			return &loginpb.PlayerLoginResponse{Result: loginpb.LoginResult_LOGIN_RESULT_TRY_AGAIN}, nil
		}
		if err := upsertAccountLogin(ctx, h.db, account.ID, req.Profile, account.HasLoginRow, int(req.NodeId)); err != nil {
			return nil, status.Errorf(codes.Internal, "db error: %v", err)
		}
		return h.buildLoginResponse(loginpb.LoginResult_LOGIN_RESULT_NEW_PLAYER, account), nil
	}

	save, err := os.ReadFile(savePath)
	if err != nil {
		h.log.Error("failed to read save on login", "username", req.Username, "error", err)
		return &loginpb.PlayerLoginResponse{Result: loginpb.LoginResult_LOGIN_RESULT_TRY_AGAIN}, nil
	}
	// TODO: PlayerLoading.Verify(save)

	if err := upsertAccountLogin(ctx, h.db, account.ID, req.Profile, account.HasLoginRow, int(req.NodeId)); err != nil {
		return nil, status.Errorf(codes.Internal, "db error: %v", err)
	}

	resp := h.buildLoginResponse(loginpb.LoginResult_LOGIN_RESULT_OK, account)
	resp.Save = save
	return resp, nil
}

func (h *handler) PlayerLogout(ctx context.Context, req *loginpb.PlayerLogoutRequest) (*loginpb.PlayerLogoutResponse, error) {
	// TODO: PlayerLoading.Verify(req.Save)
	if len(req.Save) > 0 {
		savePath := filepath.Join(h.cfg.SavePath, req.Profile, req.Username+".sav")
		if err := os.MkdirAll(filepath.Dir(savePath), 0755); err != nil {
			return nil, status.Errorf(codes.Internal, "mkdir: %v", err)
		}
		if err := os.WriteFile(savePath, req.Save, 0644); err != nil {
			return nil, status.Errorf(codes.Internal, "write save: %v", err)
		}
	}
	account, err := accountByUsername(ctx, h.db, req.Username, req.Profile)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "db error: %v", err)
	}
	if account != nil && account.HasLoginRow {
		if err := setLoggedOut(ctx, h.db, account.ID, req.Profile, int(req.NodeId)); err != nil {
			return nil, status.Errorf(codes.Internal, "db error: %v", err)
		}
	}
	// TODO: updateHiscores
	return &loginpb.PlayerLogoutResponse{Success: true}, nil
}

func (h *handler) PlayerAutosave(ctx context.Context, req *loginpb.PlayerAutosaveRequest) (*emptypb.Empty, error) {
	// TODO: PlayerLoading.Verify(req.Save)
	if len(req.Save) > 0 {
		savePath := filepath.Join(h.cfg.SavePath, req.Profile, req.Username+".sav")
		if err := os.MkdirAll(filepath.Dir(savePath), 0755); err != nil {
			return nil, status.Errorf(codes.Internal, "mkdir: %v", err)
		}
		if err := os.WriteFile(savePath, req.Save, 0644); err != nil {
			return nil, status.Errorf(codes.Internal, "write save: %v", err)
		}
	}
	return &emptypb.Empty{}, nil
}

func (h *handler) PlayerForceLogout(ctx context.Context, req *loginpb.PlayerForceLogoutRequest) (*emptypb.Empty, error) {
	account, err := accountByUsername(ctx, h.db, req.Username, req.Profile)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "db error: %v", err)
	}
	if account != nil && account.HasLoginRow {
		if err := setLoggedOut(ctx, h.db, account.ID, req.Profile, int(req.NodeId)); err != nil {
			return nil, status.Errorf(codes.Internal, "db error: %v", err)
		}
	}
	return &emptypb.Empty{}, nil
}

func (h *handler) PlayerBan(ctx context.Context, req *loginpb.PlayerBanRequest) (*loginpb.PlayerBanResponse, error) {
	if err := setAccountBanned(ctx, h.db, req.Username, req.Until.AsTime()); err != nil {
		return nil, status.Errorf(codes.Internal, "db error: %v", err)
	}
	return &loginpb.PlayerBanResponse{}, nil
}

func (h *handler) PlayerMute(ctx context.Context, req *loginpb.PlayerMuteRequest) (*loginpb.PlayerMuteResponse, error) {
	if err := setAccountMuted(ctx, h.db, req.Username, req.Until.AsTime()); err != nil {
		return nil, status.Errorf(codes.Internal, "db error: %v", err)
	}
	return &loginpb.PlayerMuteResponse{}, nil
}

func (h *handler) buildLoginResponse(result loginpb.LoginResult, account *accountRow) *loginpb.PlayerLoginResponse {
	resp := &loginpb.PlayerLoginResponse{
		Result:        result,
		AccountId:     int32(account.ID),
		StaffModLevel: int32(account.StaffModLevel),
		Members:       account.Members > 0,
	}
	if account.MutedUntil.Valid {
		if t, err := time.Parse(dbTimeFormat, account.MutedUntil.String); err == nil {
			resp.MutedUntil = timestamppb.New(t)
		}
	}
	return resp
}
```

- [ ] **Step 4: Run all handler tests**

```bash
go test ./modules/login/... -v
```

Expected: all PASS.

- [ ] **Step 5: Run with race detector**

```bash
go test -race ./modules/login/...
```

Expected: PASS with no data race warnings.

- [ ] **Step 6: Commit**

```bash
git add modules/login/handler.go modules/login/handler_test.go
git commit -m "feat: implement all 7 login gRPC RPC handlers with tests"
```

---

### Task 6: Login module wiring — config, server, login

**Files:**
- Replace: `modules/login/config.go`
- Replace: `modules/login/login.go`
- Create: `modules/login/server.go`

- [ ] **Step 1: Replace `modules/login/config.go`**

```go
package login

import (
	"flag"
)

type Config struct {
	GRPCListenAddress    string `yaml:"grpc_listen_address"`
	SQLiteDSN            string `yaml:"sqlite_dsn"`
	SavePath             string `yaml:"save_path"`
	NodeProfile          string `yaml:"node_profile"`
	GRPCListenPort       int    `yaml:"grpc_listen_port"`
	BCryptCost           int    `yaml:"bcrypt_cost"`
	AutoRegister         bool   `yaml:"auto_register"`
	AutoSubscribeMembers bool   `yaml:"auto_subscribe_members"`
	Enable               bool   `yaml:"enable"`
}

func (c *Config) RegisterFlagsAndApplyDefaults(f *flag.FlagSet) {
	f.StringVar(&c.GRPCListenAddress, "login.grpc-listen-address", "127.0.0.1", "Login gRPC server listen address")
	f.IntVar(&c.GRPCListenPort, "login.grpc-listen-port", 2004, "Login gRPC server listen port")
	f.StringVar(&c.SQLiteDSN, "login.sqlite-dsn", "data/login.db", "Login SQLite database path")
	f.StringVar(&c.SavePath, "login.save-path", "data/players", "Player save file root directory")
	f.IntVar(&c.BCryptCost, "login.bcrypt-cost", 10, "bcrypt work factor")
	f.StringVar(&c.NodeProfile, "login.node-profile", "main", "Profile name for DB queries")
	f.BoolVar(&c.AutoRegister, "login.auto-register", true, "Automatically create accounts on first login")
	f.BoolVar(&c.AutoSubscribeMembers, "login.auto-subscribe-members", true, "Automatically upgrade non-member accounts when logging into a members world")
	f.BoolVar(&c.Enable, "login.enable", false, "Whether to run the login module")
}

func (c *Config) Validate() error {
	return nil
}
```

- [ ] **Step 2: Create `modules/login/server.go`**

```go
package login

import (
	"database/sql"
	"fmt"
	"log/slog"
	"net"
	"strconv"

	"google.golang.org/grpc"

	"github.com/zsrv/goscape/pkg/loginpb"
)

type grpcServer struct {
	server *grpc.Server
	log    *slog.Logger
}

func newGRPCServer(cfg Config, db *sql.DB, log *slog.Logger) *grpcServer {
	h := &handler{db: db, cfg: cfg, log: log}
	s := grpc.NewServer()
	loginpb.RegisterLoginServiceServer(s, h)
	return &grpcServer{server: s, log: log}
}

func (s *grpcServer) run(cfg Config) error {
	addr := net.JoinHostPort(cfg.GRPCListenAddress, strconv.Itoa(cfg.GRPCListenPort))
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	s.log.Info("login gRPC server listening", "addr", addr)
	return s.server.Serve(lis)
}

func (s *grpcServer) shutdown() {
	s.server.GracefulStop()
}
```

- [ ] **Step 3: Replace `modules/login/login.go`**

```go
package login

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"

	"github.com/zsrv/goscape/internal/dskit/services"
)

type Login struct {
	cfg    Config
	log    *slog.Logger
	db     *sql.DB
	server *grpcServer
}

func New(cfg Config, log *slog.Logger) (*Login, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &Login{cfg: cfg, log: log}, nil
}

func NewLoginService(l *Login) services.Service {
	serverDone := make(chan error, 1)

	startingFn := func(ctx context.Context) error {
		db, err := openDB(l.cfg.SQLiteDSN)
		if err != nil {
			return fmt.Errorf("open db: %w", err)
		}
		l.db = db
		l.server = newGRPCServer(l.cfg, db, l.log)
		return nil
	}

	runFn := func(ctx context.Context) error {
		go func() {
			defer close(serverDone)
			serverDone <- l.server.run(l.cfg)
		}()
		select {
		case <-ctx.Done():
			return nil
		case err := <-serverDone:
			if err != nil {
				return err
			}
			return fmt.Errorf("gRPC server stopped unexpectedly")
		}
	}

	stoppingFn := func(_ error) error {
		l.server.shutdown()
		<-serverDone
		if l.db != nil {
			l.db.Close()
		}
		return nil
	}

	return services.NewBasicService(startingFn, runFn, stoppingFn)
}
```

- [ ] **Step 4: Verify the project compiles**

```bash
CGO_ENABLED=0 go build ./...
```

Expected: no errors.

- [ ] **Step 5: Run all tests**

```bash
go test ./modules/login/...
```

Expected: all PASS.

- [ ] **Step 6: Commit**

```bash
git add modules/login/config.go modules/login/login.go modules/login/server.go
git commit -m "feat: add login gRPC server wiring and dskit service lifecycle"
```

---

### Task 7: App config and module registration

**Files:**
- Modify: `cmd/goscape/app/config.go`
- Modify: `cmd/goscape/app/modules.go`

- [ ] **Step 1: Add `Login` config to `cmd/goscape/app/config.go`**

Add to the `Config` struct:
```go
Login login.Config `yaml:"login,omitempty"`
```

Add the import:
```go
"github.com/zsrv/goscape/modules/login"
```

Add to `RegisterFlagsAndApplyDefaults`:
```go
c.Login.RegisterFlagsAndApplyDefaults(f)
```

The file after modification:

```go
package app

import (
	"flag"
	"log/slog"

	"github.com/zsrv/goscape/modules/asset"
	"github.com/zsrv/goscape/modules/login"
	"github.com/zsrv/goscape/modules/world"
)

type Config struct {
	Target    string     `yaml:"target,omitempty"`
	LogFormat string     `yaml:"log_format,omitempty"`
	LogLevel  slog.Level `yaml:"log_level,omitempty"`

	Asset asset.Config `yaml:"asset,omitempty"`
	Login login.Config `yaml:"login,omitempty"`
	World world.Config `yaml:"world,omitempty"`
}

func NewDefaultConfig() *Config {
	defaultConfig := &Config{}
	defaultFS := flag.NewFlagSet("", flag.PanicOnError)
	defaultConfig.RegisterFlagsAndApplyDefaults(defaultFS)
	return defaultConfig
}

func (c *Config) RegisterFlagsAndApplyDefaults(f *flag.FlagSet) {
	c.Target = SingleBinary

	f.StringVar(&c.Target, "target", SingleBinary, "Target module")
	f.TextVar(&c.LogLevel, "log.level", slog.LevelInfo, "Only log messages with the given severity or above. Valid levels: [debug, info, warn, error]")
	f.StringVar(&c.LogFormat, "log.format", "text", "Output log messages in the given format. Valid formats: [text, json]")

	c.Asset.RegisterFlagsAndApplyDefaults(f)
	c.Login.RegisterFlagsAndApplyDefaults(f)
	c.World.RegisterFlagsAndApplyDefaults(f)
}

func (c *Config) CheckConfig() []ConfigWarning {
	var warnings []ConfigWarning
	return warnings
}

type ConfigWarning struct {
	Message string
	Explain string
}
```

- [ ] **Step 2: Register the login module in `cmd/goscape/app/modules.go`**

Add the `Login` target constant and `initLogin` function, and register it in `setupModuleManager`.

The `App` struct (in `app.go` or `modules.go`) needs a `login *login.Login` field. Add it to wherever the other module fields (`asset`, `world`) live.

Add at the top of modules.go constants block:
```go
Login string = "login"
```

Add `initLogin` function (same file as `initAsset` and `initWorld`):

```go
func (g *App) initLogin() (services.Service, error) {
	if !g.cfg.Login.Enable {
		return services.NewIdleService(nil, nil), nil
	}

	logger, err := log.NewLogger(g.cfg.LogLevel, g.cfg.LogFormat)
	if err != nil {
		g.logger.Error("failed to create logger", "module", "login", "err", err)
		os.Exit(1)
	}

	l, err := login.New(g.cfg.Login, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create login: %w", err)
	}
	g.login = l

	return login.NewLoginService(g.login), nil
}
```

Add `login *login.Login` to the `App` struct — it lives in the same file as the `asset *asset.Asset` and `world *world.World` fields (search for `asset *asset.Asset` to find the struct).

Update `setupModuleManager` to include login:

```go
mm.RegisterModule(Login, g.initLogin)

mm.RegisterModule(SingleBinary, nil)

deps := map[string][]string{
    Common: {},
    Asset:        {Common},
    Login:        {Common},
    World:        {Common},
    SingleBinary: {Asset, Login, World},
}
```

Add import `"github.com/zsrv/goscape/modules/login"` and add `login *login.Login` field to the `App` struct.

- [ ] **Step 3: Verify the project compiles**

```bash
CGO_ENABLED=0 go build ./...
```

Expected: no errors.

- [ ] **Step 4: Run all tests**

```bash
go test ./...
```

Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/goscape/app/config.go cmd/goscape/app/modules.go cmd/goscape/app/
git commit -m "feat: register login module in app module manager"
```

---

### Task 8: World login client + handleLogin wiring

**Files:**
- Modify: `modules/world/config.go`
- Create: `modules/world/login_client.go`
- Modify: `modules/world/world.go`
- Modify: `modules/world/server.go`
- Modify: `modules/world/client.go`

- [ ] **Step 1: Add login server config fields to `modules/world/config.go`**

Add these two fields to the `Config` struct (keep all existing fields unchanged):

```go
LoginServerAddress string `yaml:"login_server_address"`
LoginServerEnabled bool   `yaml:"login_server_enabled"`
```

Add these two lines to `RegisterFlagsAndApplyDefaults`:

```go
f.StringVar(&c.LoginServerAddress, "world.login-server-address", "127.0.0.1:2004", "Login server gRPC address")
f.BoolVar(&c.LoginServerEnabled, "world.login-server-enabled", true, "Whether to connect to the login server")
```

- [ ] **Step 2: Create `modules/world/login_client.go`**

```go
package world

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/zsrv/goscape/pkg/loginpb"
)

type LoginClient struct {
	conn   *grpc.ClientConn
	client loginpb.LoginServiceClient
	log    *slog.Logger
}

func NewLoginClient(addr string, log *slog.Logger) (*LoginClient, error) {
	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("create login client: %w", err)
	}
	return &LoginClient{
		conn:   conn,
		client: loginpb.NewLoginServiceClient(conn),
		log:    log,
	}, nil
}

func (c *LoginClient) Close() {
	c.conn.Close()
}

func (c *LoginClient) WorldStartup(ctx context.Context, nodeID int32, profile string) {
	if _, err := c.client.WorldStartup(ctx, &loginpb.WorldStartupRequest{
		NodeId:  nodeID,
		Profile: profile,
	}); err != nil {
		c.log.Warn("world startup notification to login server failed", "error", err)
	}
}

func (c *LoginClient) PlayerLogin(ctx context.Context, req *loginpb.PlayerLoginRequest) (*loginpb.PlayerLoginResponse, error) {
	return c.client.PlayerLogin(ctx, req)
}

func (c *LoginClient) PlayerLogout(ctx context.Context, req *loginpb.PlayerLogoutRequest) (bool, error) {
	resp, err := c.client.PlayerLogout(ctx, req)
	if err != nil {
		return false, err
	}
	return resp.Success, nil
}

func (c *LoginClient) PlayerAutosave(username, profile string, save []byte, log *slog.Logger) {
	go func() {
		if _, err := c.client.PlayerAutosave(context.Background(), &loginpb.PlayerAutosaveRequest{
			Profile:  profile,
			Username: username,
			Save:     save,
		}); err != nil {
			log.Warn("autosave failed", "username", username, "error", err)
		}
	}()
}

func (c *LoginClient) PlayerForceLogout(ctx context.Context, nodeID int32, profile, username string) {
	if _, err := c.client.PlayerForceLogout(ctx, &loginpb.PlayerForceLogoutRequest{
		NodeId:   nodeID,
		Profile:  profile,
		Username: username,
	}); err != nil {
		c.log.Warn("force logout failed", "username", username, "error", err)
	}
}

func (c *LoginClient) PlayerBan(ctx context.Context, staff, username string, until time.Time) {
	if _, err := c.client.PlayerBan(ctx, &loginpb.PlayerBanRequest{
		Staff:    staff,
		Username: username,
		Until:    timestamppb.New(until),
	}); err != nil {
		c.log.Warn("ban failed", "username", username, "error", err)
	}
}

func (c *LoginClient) PlayerMute(ctx context.Context, staff, username string, until time.Time) {
	if _, err := c.client.PlayerMute(ctx, &loginpb.PlayerMuteRequest{
		Staff:    staff,
		Username: username,
		Until:    timestamppb.New(until),
	}); err != nil {
		c.log.Warn("mute failed", "username", username, "error", err)
	}
}
```

- [ ] **Step 3: Update `modules/world/world.go` to create LoginClient in `New()`**

In the `World` struct, add:
```go
loginClient *LoginClient
```

In `New()`, create the LoginClient before calling `NewServer`, then pass it in. Replace the existing `server, err := NewServer(cfg, logger)` line with:

```go
var loginClient *LoginClient
if cfg.LoginServerEnabled {
    lc, err := NewLoginClient(cfg.LoginServerAddress, logger)
    if err != nil {
        // grpc.NewClient rarely errors — log and continue; RPCs will fail at call time
        logger.Warn("failed to create login client", "error", err)
    } else {
        loginClient = lc
    }
}
w.loginClient = loginClient

server, err := NewServer(cfg, logger, loginClient)
```

`NewServer` signature changes in the next step.

- [ ] **Step 4: Update `modules/world/server.go` — Server struct, NewServer, newClient**

In `Server` struct, add:
```go
loginClient *LoginClient
```

Update `NewServer` signature to accept loginClient:

```go
func NewServer(cfg Config, logger *slog.Logger, loginClient *LoginClient) (*Server, error) {
```

Add in `NewServer` body when building the struct:
```go
loginClient: loginClient,
```

Update `handleTCPConn` call to `newClient`:
```go
c := newClient(conn, s.cfg, s.loginClient, s.log)
```

- [ ] **Step 5: Update `modules/world/client.go` — add loginClient and cfg to client struct**

In the `client` struct, add:
```go
loginClient *LoginClient
serverCfg   Config
```

Update `newClient` signature:
```go
func newClient(conn net.Conn, cfg Config, loginClient *LoginClient, logger *slog.Logger) *client {
```

In `newClient` return statement, add:
```go
loginClient: loginClient,
serverCfg:   cfg,
```

Keep the existing `writeTimeout: cfg.TCPServerWriteTimeout` (update if it was previously passed directly).

- [ ] **Step 6: Replace the login stub in `handleLogin` in `modules/world/server.go`**

Find the block starting with `safeName := util.ToSafeName(req.Username)` through the end of the `case` block. Replace the entire `// TODO` section (the `reply := 6` and the switch on reply) with:

```go
safeName := util.ToSafeName(req.Username)

if c.loginClient == nil {
    return c.sendLoginError(loginresp.OpLoginServerOffline.Opcode)
}

reconnecting := opcode[0] == loginreq.OpReqGameReconnect.Opcode

loginResp, err := c.loginClient.PlayerLogin(context.Background(), &loginpb.PlayerLoginRequest{
    NodeId:        int32(c.serverCfg.NodeID),
    Profile:       c.serverCfg.NodeProfile,
    NodeMembers:   c.serverCfg.NodeMembers,
    Username:      safeName,
    Password:      req.Password,
    Uid:           int32(req.UID),
    Socket:        uuid.NewString(),
    RemoteAddress: c.conn.RemoteAddr().String(),
    Reconnecting:  reconnecting,
    HasSave:       false,
})
if err != nil {
    c.log.Error("login server RPC failed", "error", err)
    return c.sendLoginError(loginresp.OpLoginServerOffline.Opcode)
}

switch loginResp.Result {
case loginpb.LoginResult_LOGIN_RESULT_INVALID_CREDENTIALS:
    return c.sendLoginError(loginresp.OpInvalidUsernameOrPassword.Opcode)
case loginpb.LoginResult_LOGIN_RESULT_ALREADY_LOGGED_IN:
    return c.sendLoginError(loginresp.OpDuplicate.Opcode)
case loginpb.LoginResult_LOGIN_RESULT_ACCOUNT_DISABLED:
    return c.sendLoginError(loginresp.OpBanned.Opcode)
case loginpb.LoginResult_LOGIN_RESULT_TRY_AGAIN:
    return c.sendLoginError(loginresp.OpLoginServerRejected.Opcode)
case loginpb.LoginResult_LOGIN_RESULT_LOGIN_IN_PROGRESS:
    return c.sendLoginError(loginresp.OpTooManyAttempts.Opcode)
case loginpb.LoginResult_LOGIN_RESULT_NOT_A_MEMBER:
    return c.sendLoginError(loginresp.OpNeedMembersAccount.Opcode)
case loginpb.LoginResult_LOGIN_RESULT_OK,
    loginpb.LoginResult_LOGIN_RESULT_NEW_PLAYER,
    loginpb.LoginResult_LOGIN_RESULT_RECONNECT_OK:
    // TODO: use loginResp.Save, loginResp.AccountId, loginResp.StaffModLevel, loginResp.Members
    c.log.Info("login success",
        "username", safeName,
        "result", loginResp.Result,
        "account_id", loginResp.AccountId,
        "members", loginResp.Members,
    )
default:
    return c.sendLoginError(loginresp.OpLoginServerOffline.Opcode)
}
```

Also remove the now-unused `LoginResponse` struct at the bottom of `server.go`.

Add imports to `server.go`:
```go
"context"

"github.com/google/uuid"
"github.com/zsrv/goscape/pkg/loginpb"
```

- [ ] **Step 7: Verify the project compiles**

```bash
CGO_ENABLED=0 go build ./...
```

Expected: no errors. If `github.com/google/uuid` is only indirect in go.mod, promote it:
```bash
go get github.com/google/uuid
```

- [ ] **Step 8: Run all tests**

```bash
go test ./...
```

Expected: all PASS.

- [ ] **Step 9: Commit**

```bash
git add modules/world/config.go modules/world/login_client.go modules/world/world.go modules/world/server.go modules/world/client.go go.mod go.sum
git commit -m "feat: wire login client into world server handleLogin"
```
