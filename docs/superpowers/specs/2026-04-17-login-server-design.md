# Login Server Design

**Date:** 2026-04-17
**Reference:** `Engine-TS/src/server/login/LoginServer.ts`

## Summary

A Go rewrite of the TypeScript `LoginServer`. The login server is a standalone `modules/login` dskit service that authenticates players via gRPC, backed by SQLite. World servers connect to it over gRPC and retry automatically on disconnect. The login server is always required; the world does not have a standalone fallback mode.

## Decisions

| Decision | Choice | Rationale |
|---|---|---|
| World↔login IPC | gRPC | Typed RPCs, built-in retry/backoff, no custom framing |
| Database | SQLite via `modernc.org/sqlite` | Parity with TS reference, works with `CGO_ENABLED=0` |
| DB access | `database/sql` directly | Schema is small and stable; no ORM needed |
| Password hashing | `golang.org/x/crypto/bcrypt` | Direct equivalent of TS `bcrypt` |
| Save file validation | Stubbed (`// TODO: PlayerLoading.Verify`) | Not yet ported to Go |
| Hiscore update on logout | Stubbed (`// TODO: updateHiscores`) | Depends on unported `PlayerLoading` |

## File Layout

```
modules/login/
  config.go          — Config struct (gRPC listen, SQLite DSN, save path, bcrypt cost)
  login.go           — Login struct, New(), NewLoginService() via dskit BasicService
  server.go          — gRPC server setup, service registration, graceful shutdown
  db.go              — DB open/close, ~10 query helper functions
  handler.go         — LoginServiceServer implementation (7 RPC handlers)
                       replaces the current HTTP asset handler entirely

modules/world/
  login_client.go    — LoginClient wrapping grpc.ClientConn + generated stub

proto/login/
  login.proto        — LoginService definition

pkg/loginpb/         — Go code generated from login.proto (via buf or protoc)
```

**Deleted:** current `modules/login/handler.go` (HTTP asset handler — belongs in the asset module).

## Protobuf Service

```protobuf
syntax = "proto3";
package login.v1;

import "google/protobuf/empty.proto";
import "google/protobuf/timestamp.proto";

option go_package = "github.com/zsrv/goscape/pkg/loginpb";

service LoginService {
  rpc WorldStartup(WorldStartupRequest)       returns (google.protobuf.Empty);
  rpc PlayerLogin(PlayerLoginRequest)         returns (PlayerLoginResponse);
  rpc PlayerLogout(PlayerLogoutRequest)       returns (PlayerLogoutResponse);
  rpc PlayerAutosave(PlayerAutosaveRequest)   returns (google.protobuf.Empty);
  rpc PlayerForceLogout(PlayerForceLogoutRequest) returns (google.protobuf.Empty);
  rpc PlayerBan(PlayerBanRequest)             returns (PlayerBanResponse);
  rpc PlayerMute(PlayerMuteRequest)           returns (PlayerMuteResponse);
}

enum LoginResult {
  LOGIN_RESULT_UNSPECIFIED         = 0;
  LOGIN_RESULT_OK                  = 1;  // existing player, save included
  LOGIN_RESULT_NEW_PLAYER          = 2;  // first login, no save file on disk
  LOGIN_RESULT_RECONNECT_OK        = 3;  // reconnecting to same world
  LOGIN_RESULT_INVALID_CREDENTIALS = 4;
  LOGIN_RESULT_ALREADY_LOGGED_IN   = 5;
  LOGIN_RESULT_ACCOUNT_DISABLED    = 6;
  LOGIN_RESULT_NOT_A_MEMBER        = 7;
  LOGIN_RESULT_LOGIN_IN_PROGRESS   = 8;
  LOGIN_RESULT_TRY_AGAIN           = 9;  // transient safety reject
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
  string socket         = 7;   // session UUID assigned by world
  string remote_address = 8;
  bool   reconnecting   = 9;
  bool   has_save       = 10;  // world already has the save in memory
}

message PlayerLoginResponse {
  LoginResult               result        = 1;
  int32                     account_id    = 2;
  int32                     staffmodlevel = 3;
  optional bytes            save          = 4;  // absent for new player or reconnect+has_save
  optional google.protobuf.Timestamp muted_until = 5;
  bool                      members       = 6;
  int32                     message_count = 7;
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
  string staff    = 1;
  string username = 2;
  google.protobuf.Timestamp until = 3;
}

message PlayerBanResponse {}

message PlayerMuteRequest {
  string staff    = 1;
  string username = 2;
  google.protobuf.Timestamp until = 3;
}

message PlayerMuteResponse {}
```

The world maps `LoginResult` enum values back to RS2 wire response codes when replying to the game client, keeping the gRPC layer decoupled from the RS2 protocol numbers.

## DB Layer (`db.go`)

Opens a single `*sql.DB` with `modernc.org/sqlite`. WAL mode and foreign keys are enabled at open time:

```sql
PRAGMA journal_mode=WAL;
PRAGMA foreign_keys=ON;
```

Schema management (migrations) is out of scope — the login server assumes the schema already exists.

Query functions:

| Function | Tables |
|---|---|
| `accountByUsername(ctx, db, username, profile)` | `account` LEFT JOIN `account_login` |
| `ipBanned(ctx, db, ip)` | `ipban` |
| `insertAccount(ctx, db, username, hashedPassword, ip)` | `account` |
| `upsertAccountLogin(ctx, db, accountID, profile, nodeID)` | `account_login` |
| `insertSession(ctx, db, req)` | `session` |
| `clearWorldSessions(ctx, db, nodeID, profile)` | `account_login` |
| `setLoggedOut(ctx, db, accountID, profile, nodeID)` | `account_login` |
| `setAccountBanned(ctx, db, username, until)` | `account` |
| `setAccountMuted(ctx, db, username, until)` | `account` |

All queries accept and propagate `context.Context` so RPC deadlines cancel in-flight DB operations.

## Handler Logic (`handler.go`)

Implements `loginpb.LoginServiceServer`.

**WorldStartup** — calls `clearWorldSessions` to mark all players on that world as logged out (handles ungraceful world restarts).

**PlayerLogin** — the main authentication path:
1. Check `loginRequests` (`sync.Map`) for duplicate in-flight request on this username → return `LOGIN_RESULT_LOGIN_IN_PROGRESS`
2. Add to `loginRequests`; defer removal
3. Check `ipBanned` → return `LOGIN_RESULT_TRY_AGAIN`
4. `accountByUsername`; if no account and auto-registration enabled → `insertAccount`
5. `bcrypt.CompareHashAndPassword` → `LOGIN_RESULT_INVALID_CREDENTIALS`
6. Check `banned_until` → `LOGIN_RESULT_ACCOUNT_DISABLED`
7. Check members mismatch → `LOGIN_RESULT_NOT_A_MEMBER` (or auto-upgrade if configured)
8. Check `logged_in != 0 && != nodeID` → `LOGIN_RESULT_ALREADY_LOGGED_IN`
9. `insertSession`
10. Read `.sav` from disk; validation stubbed (`// TODO: PlayerLoading.Verify`)
11. `upsertAccountLogin` to mark player logged in on this node
12. Return `LOGIN_RESULT_OK` / `LOGIN_RESULT_NEW_PLAYER` / `LOGIN_RESULT_RECONNECT_OK`

**PlayerLogout:**
1. Validate save bytes (stubbed)
2. Write `.sav` to `data/players/{profile}/{username}.sav`
3. `setLoggedOut` in `account_login`
4. Hiscore update stubbed (`// TODO: updateHiscores`)
5. Return `success: true`

**PlayerAutosave** — write `.sav` to disk, no DB update. Validation stubbed.

**PlayerForceLogout** — `setLoggedOut` in `account_login` without a save write.

**PlayerBan / PlayerMute** — `setAccountBanned` / `setAccountMuted`.

## World LoginClient (`modules/world/login_client.go`)

```go
type LoginClient struct {
    conn   *grpc.ClientConn
    client loginpb.LoginServiceClient
}

func NewLoginClient(addr string) (*LoginClient, error) {
    conn, err := grpc.NewClient(addr,
        grpc.WithTransportCredentials(insecure.NewCredentials()),
    )
    // ...
}
```

`grpc.NewClient` does not block — the connection is established lazily with automatic exponential backoff. World startup calls `WorldStartup` RPC; if the login server is unreachable, the RPC fails with a gRPC status error. The world logs the error and continues — player logins will fail with a safe error response until connectivity is established.

## Configuration

**Login module (`modules/login/config.go`):**

| Flag | Default | Description |
|---|---|---|
| `login.grpc-listen-address` | `127.0.0.1` | gRPC listen address |
| `login.grpc-listen-port` | `2004` | gRPC listen port |
| `login.sqlite-dsn` | `data/login.db` | SQLite database path |
| `login.save-path` | `data/players` | Player save file root directory |
| `login.bcrypt-cost` | `10` | bcrypt work factor |
| `login.node-profile` | `main` | Profile name for DB queries |
| `login.auto-register` | `true` | Automatically create accounts on first login |
| `login.enable` | `false` | Whether to run this module |

**World module additions (`modules/world/config.go`):**

| Flag | Default | Description |
|---|---|---|
| `world.login-server-address` | `127.0.0.1:2004` | Login server gRPC address |
| `world.login-server-enabled` | `true` | Whether to connect to login server |

## Error Handling

| Situation | gRPC status | World behaviour |
|---|---|---|
| DB error | `codes.Internal` | Map to `LOGIN_RESULT_TRY_AGAIN` |
| Save I/O error on logout | `codes.Internal` | Log error; surface to caller |
| Login server unreachable | `codes.Unavailable` | Log; continue world startup |
| Business logic rejection | `codes.OK` with result enum | Map enum to RS2 wire code |

## Testing

Handler tests use an in-memory SQLite DB (`file::memory:?cache=shared`) opened with the same `PRAGMA` setup as production. Tests cover:

- `PlayerLogin`: happy path (new player, existing player, reconnect), invalid credentials, IP ban, already logged in, duplicate in-flight
- `PlayerLogout`: happy path, save write failure
- `WorldStartup`: clears stale sessions for the given node
- `PlayerBan` / `PlayerMute`: DB field updated correctly

No mocking of `*sql.DB` — real SQLite, millisecond-fast.
