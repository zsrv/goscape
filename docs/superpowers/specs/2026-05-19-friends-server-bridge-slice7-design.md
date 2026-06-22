# Friends-server bridge — slice 7 design: `Player.session` per-login UUID assignment

**Date:** 2026-05-19
**Slice:** 7 of 7 (friends-server bridge arc — final slice)
**Predecessor:** slice 6 (close commit `969f5cfd`, retired `NAI-S1-D-PM-NO-PERSISTENCE`; see `[[friends-server-slice6-close]]`)
**Closes:** UUID-half carry-forward of the original `NAI-72-D-LOGIN-SERVER-BRIDGE-MOD` deferral (ban/mute half already retired by NAI-214).
**Opens:** none.

## 1. Scope

Make `Player.session` a real per-login UUID instead of the constant string `"headless"`.

The login server already maintains a `session` SQLite table (`modules/login/migrations/000001_init.up.sql:23`) with a `session_uuid TEXT NOT NULL` column, but the handler stubs the value: `insertSession(ctx, h.db, req.Socket, …)` at `modules/login/handler.go:158` passes the world-side socket string as the UUID. The `PlayerLoginResponse` proto has no `session_uuid` field, so the world has no way to learn it; `modules/world/tick.go:293-294` falls back to `"headless"` for every player.

Slice 7 fixes the entire chain:

1. **Proto** — add `string session_uuid = 8;` to `PlayerLoginResponse`.
2. **Login server** — generate a fresh UUID v4 at the top of `PlayerLogin` (after the duplicate-in-flight gate), use it for the `session` table insert, and return it in `PlayerLoginResponse.SessionUuid` on every positive result (OK / NEW_PLAYER / RECONNECT_OK / ACCOUNT_DISABLED / NOT_A_MEMBER — i.e. everything that goes through `buildLoginResponse`; only the OK / NEW_PLAYER paths actually `insertSession`).
3. **World** — cache `resp.SessionUuid` on the `client` struct in the accepted-paths block of `callPlayerLoginRPC`; copy it into `Player.session` in `newPlayer(c)`. The `tick.go:293-294` `"headless"` fallback is retained for paths where the world runs without a login bridge (standalone-world; test fixtures that bypass login).

That's the entire wire change. No friends-server change; no schema migration; no config change.

## 2. Forward map (what changes)

| File | New / changed | Notes |
|---|---|---|
| `proto/login/login.proto` | **changed** | Add `string session_uuid = 8;` to `PlayerLoginResponse`. |
| `pkg/loginpb/login.pb.go` | **regenerated** | `make protos`. |
| `go.mod` | **changed** | Promote `github.com/google/uuid v1.6.0` from `// indirect` to direct. |
| `modules/login/handler.go` | **changed** | Generate UUID via `uuid.NewString()`; pass to `insertSession`; thread through `buildLoginResponse` so every positive response carries it. |
| `modules/login/handler_test.go` | **changed** | New `TestPlayerLogin_SessionUUID_*` (returned uuid is a valid v4; matches `session` row on OK; freshness across calls). Existing tests get a `_ = resp.SessionUuid` no-op or trivial non-empty assertion where applicable. |
| `modules/world/client.go` | **changed** | Add `sessionUUID string` field to `client` struct (alongside `staffModLevel`, `members`, `username`, `savePayload`). |
| `modules/world/server.go` | **changed** | In `callPlayerLoginRPC` accepted-paths block, `c.sessionUUID = resp.GetSessionUuid()`. |
| `modules/world/player.go` | **changed** | `newPlayer(c)` initialiser sets `session: c.sessionUUID`; update doc-comment at lines 291-296 to reflect that real UUID is now wired through the login bridge. |
| `modules/world/login_client_test.go` | **changed** | Extend `TestCallPlayerLoginRPC_ReplyByteMapping` to assert `c.sessionUUID` is cached on accepted paths and zero-valued on rejected paths. |
| `modules/world/tick.go` | **unchanged** | The `if p.session == "" { p.session = "headless" }` fallback at lines 293-294 stays — handles standalone-world / unbridged-test paths. Update the slice-comment to point at slice 7 instead of `LOGIN-SERVER-BRIDGE-MOD`. |
| `modules/world/friends_smoke_test.go` | **changed** | New `TestFriendsClient_E2E_PlayerSessionIsUUID` — full login through the real login server, then assert the player's `session` is a non-empty UUID (not `"headless"`). |

**LOC estimate:** ~120 added, ~10 deleted.

## 3. Proto change

`proto/login/login.proto`:

```diff
 message PlayerLoginResponse {
   LoginResult                result        = 1;
   int32                      account_id    = 2;
   int32                      staff_mod_level = 3;
   optional bytes             save          = 4;
   optional google.protobuf.Timestamp muted_until = 5;
   bool                       members       = 6;
   int32                      message_count = 7;
+  string                     session_uuid  = 8;
 }
```

`session_uuid` is a plain (non-optional) `string`. The TS authority (Node's `crypto.randomUUID()` returning RFC 4122 v4) always produces a value, so the empty string is the documented "absent" sentinel — used by the world only as a fallback signal that the login bridge didn't populate it (which, after slice 7, can only happen on rejected paths). The world treats `""` as "leave `session` blank so the tick fallback assigns `"headless"`."

**Tag selection (`= 8`):** current max tag is `message_count = 7`. New non-optional string field is backwards-compat-safe — older clients see the zero value, newer clients populate it. No `reserved` block needed (no removed tags).

## 4. Login-server change

`modules/login/handler.go`:

### 4.1 UUID generation — placement

Generate the UUID once at the top of `PlayerLogin`, immediately after the duplicate-in-flight check (so a duplicate-in-flight rejection still gets `""`, which is fine — the world doesn't admit the player). All later paths use the same value.

```go
import "github.com/google/uuid"
…

func (h *handler) PlayerLogin(ctx context.Context, req *loginpb.PlayerLoginRequest) (*loginpb.PlayerLoginResponse, error) {
    // 1. Duplicate in-flight check.
    if _, loaded := h.loginRequests.LoadOrStore(req.Username, struct{}{}); loaded {
        return &loginpb.PlayerLoginResponse{
            Result: loginpb.LoginResult_LOGIN_RESULT_LOGIN_IN_PROGRESS,
        }, nil
    }
    defer h.loginRequests.Delete(req.Username)

    // Per-login UUID. Generated once; used for the `session` row insert
    // (when applicable) and stamped on every positive response so the
    // world can assign Player.session = <uuid>. Mirrors TS
    // FriendServer.ts equivalent of crypto.randomUUID(). Retires the
    // UUID half of the original NAI-72-D-LOGIN-SERVER-BRIDGE-MOD
    // deferral (ban/mute half retired by NAI-214).
    sessionUUID := uuid.NewString()

    // 2. IP ban check.
    …
}
```

### 4.2 Insert call site

Replace the `req.Socket` stub at line 158:

```diff
-    if err := insertSession(ctx, h.db, req.Socket, account.ID, req.Profile, int(req.NodeId), int(req.Uid), ip); err != nil {
+    if err := insertSession(ctx, h.db, sessionUUID, account.ID, req.Profile, int(req.NodeId), int(req.Uid), ip); err != nil {
         return nil, status.Errorf(codes.Internal, "insertSession: %v", err)
     }
```

`insertSession`'s signature is unchanged. `req.Socket` is no longer used in this file (and the field is not removed from the proto — it's still part of the request envelope; the login server simply doesn't index by it).

### 4.3 `buildLoginResponse` plumbing

Thread `sessionUUID` through `buildLoginResponse` so every positive response stamps it:

```diff
-func buildLoginResponse(result loginpb.LoginResult, account *accountRow, save []byte) *loginpb.PlayerLoginResponse {
+func buildLoginResponse(result loginpb.LoginResult, account *accountRow, save []byte, sessionUUID string) *loginpb.PlayerLoginResponse {
     resp := &loginpb.PlayerLoginResponse{
         Result:        result,
         AccountId:     int32(account.ID),
         StaffModLevel: int32(account.StaffModLevel),
         Members:       account.Members == 1,
+        SessionUuid:   sessionUUID,
     }
     …
 }
```

Call sites (five `buildLoginResponse(...)` invocations in `PlayerLogin`: `ACCOUNT_DISABLED` at line 123, `NOT_A_MEMBER` at line 136, `ALREADY_LOGGED_IN` at line 144, `RECONNECT_OK` at line 154, final `OK`/`NEW_PLAYER` at line 180) all gain a `, sessionUUID` argument. Non-`buildLoginResponse` branches (the early `LOGIN_RESULT_INVALID_CREDENTIALS` returns at lines 88-90 and 111-114, the `LOGIN_RESULT_IP_BANNED` return at lines 76-78, the `LOGIN_RESULT_LOGIN_IN_PROGRESS` short-circuit) are intentionally NOT plumbed — the world doesn't admit those players and the field stays `""`.

This is the only signature change.

### 4.4 UUID library

`go.mod` already lists `github.com/google/uuid v1.6.0 // indirect` (pulled in by the migrate dependency tree). Slice 7 promotes it to a direct dependency by importing it. `go mod tidy` will adjust the `// indirect` comment.

`uuid.NewString()` (returns canonical 36-char hyphenated v4 form, e.g. `xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx`) is the right entry point — equivalent to `uuid.New().String()`. RFC 4122 v4 is what Node's `crypto.randomUUID()` produces, matching TS.

## 5. World-side change

### 5.1 `client` struct — add `sessionUUID`

`modules/world/client.go`:

```diff
     staffModLevel int32
     members       bool
+    // sessionUUID is the per-login session correlation key returned
+    // by the login server's PlayerLogin RPC. Copied onto Player.session
+    // at newPlayer(). Empty for paths that bypass the login bridge
+    // (standalone world; tests) — tick.go's "headless" fallback then
+    // applies. Slice 7 of friends-server bridge arc.
+    sessionUUID string
     // username is the safe-form ("snake_case") account name …
     username string
```

### 5.2 `callPlayerLoginRPC` — cache UUID alongside other session fields

`modules/world/server.go:845-852`:

```diff
     if result == loginpb.LoginResult_LOGIN_RESULT_OK ||
         result == loginpb.LoginResult_LOGIN_RESULT_NEW_PLAYER ||
         result == loginpb.LoginResult_LOGIN_RESULT_RECONNECT_OK {
         c.staffModLevel = resp.GetStaffModLevel()
         c.members = resp.GetMembers()
         c.username = safeName
         c.savePayload = resp.GetSave()
+        c.sessionUUID = resp.GetSessionUuid()
     }
```

Caching only on accepted paths matches the existing pattern; rejected paths leave `c.sessionUUID == ""` and the player never reaches `newPlayer`.

### 5.3 `newPlayer` — copy onto Player

`modules/world/player.go:504-512`:

```diff
 func newPlayer(c *client) *Player {
     p := &Player{
         client:         c,
         reconnecting:   c.reconnecting,
         lowMemory:      c.lowMemory,
         username:       c.username,
         displayName:    util.ToDisplayName(c.username),
         username37:     util.ToBase37(c.username),
         staffModLevel:  c.staffModLevel,
+        session:        c.sessionUUID,
         slot:           -1,
         …
```

When `c.sessionUUID == ""` (test fixtures, standalone-world), `p.session` starts as `""` and the tick fallback at line 293-294 replaces it with `"headless"`. When `c.sessionUUID` is a real UUID, `p.session` is non-empty and the fallback skips it.

### 5.4 Player doc-comment retirement

`modules/world/player.go:291-296`:

```diff
-    // session is the per-player session correlation key for the logger
-    // bridge. Defaults to "headless" (TS Player.session = 'headless',
-    // Player.ts:304). Real per-login UUID assignment is a carry-forward
-    // from the original NAI-72 social-subsystem deferral — not wired by
-    // NAI-214, which only ported the ban/mute half of that umbrella.
+    // session is the per-player session correlation key for the logger
+    // bridge. Assigned from the login-server PlayerLoginResponse's
+    // session_uuid field via newPlayer(c). Falls back to "headless" in
+    // processLogins for paths that bypass the login bridge (standalone
+    // world, unit tests). Mirrors TS Player.session (Player.ts:304),
+    // which is also a per-login UUID. Slice 7 of friends-server bridge
+    // arc; ban/mute half of the umbrella was retired by NAI-214.
     session string
```

### 5.5 `tick.go` fallback comment update

`modules/world/tick.go:288-295`:

```diff
-        // NAI-73: allocate the InputTracking state machine. Defaults
-        // session to "headless" until LOGIN-SERVER-BRIDGE-MOD ships a
-        // real UUID assignment.
+        // NAI-73: allocate the InputTracking state machine.
+        // session is normally assigned in newPlayer() from the
+        // PlayerLoginResponse.session_uuid; the "headless" fallback
+        // covers standalone-world and unit-test paths that bypass
+        // the login bridge.
         p.input = NewInputTracking(p, s.currentTick)
         if p.session == "" {
             p.session = "headless"
         }
```

No behavior change at this site — purely a comment refresh now that the fallback is no longer the only writer.

## 6. Test plan

### 6.1 Login-server unit tests (`modules/login/handler_test.go`)

| Test | Purpose |
|---|---|
| `TestPlayerLogin_SessionUUID_FormatOnAccept` | `Result == OK` (or `NEW_PLAYER` via auto-register) ⇒ `resp.SessionUuid` parses via `uuid.Parse` and `uuid.MustParse(...).Version() == 4`. |
| `TestPlayerLogin_SessionUUID_PersistedInDB` | After `OK`, query `SELECT session_uuid FROM session WHERE account_id = ?` and assert the value matches `resp.SessionUuid`. |
| `TestPlayerLogin_SessionUUID_FreshPerLogin` | Two consecutive `PlayerLogin` calls (different users, same handler) produce distinct UUIDs. |
| `TestPlayerLogin_SessionUUID_EmptyOnReject` | `INVALID_CREDENTIALS` / `IP_BANNED` paths return `resp.SessionUuid == ""` (early-return paths that bypass `buildLoginResponse`). `ALREADY_LOGGED_IN` / `ACCOUNT_DISABLED` / `NOT_A_MEMBER` go through `buildLoginResponse` so they get the UUID; tests assert these paths' UUID is well-formed but no `session` row was inserted. |

The existing `db_test.go` `TestInsertSession_*` continues to work — `insertSession`'s signature didn't change; the literal `"uuid-abc-123"` test string is still legal input.

### 6.2 World-side unit tests (`modules/world/login_client_test.go`)

Extend `TestCallPlayerLoginRPC_ReplyByteMapping`:

```diff
             fake := newFakeLoginClient()
             fake.playerLoginResp = &loginpb.PlayerLoginResponse{
                 Result:        tc.result,
                 StaffModLevel: 2,
                 Members:       true,
                 Save:          []byte("SAVE-BYTES"),
+                SessionUuid:   "test-uuid-123",
             }
             …
             if tc.caches {
-                if c.staffModLevel != 2 || !c.members || c.username != "test" || string(c.savePayload) != "SAVE-BYTES" {
+                if c.staffModLevel != 2 || !c.members || c.username != "test" || string(c.savePayload) != "SAVE-BYTES" || c.sessionUUID != "test-uuid-123" {
                     t.Errorf("expected session cached: …")
                 }
             } else {
-                if c.staffModLevel != 0 || c.members || c.username != "" || c.savePayload != nil {
+                if c.staffModLevel != 0 || c.members || c.username != "" || c.savePayload != nil || c.sessionUUID != "" {
                     t.Errorf("expected session NOT cached: …")
                 }
             }
```

New `TestCallPlayerLoginRPC_RPCError_SessionUUIDNotCached` — pins that `c.sessionUUID == ""` after an RPC error.

### 6.3 World-side unit test — `newPlayer` copies sessionUUID

New file or extension to existing `player_test.go` (whichever the repo uses):

```
TestNewPlayer_SessionFromClientUUID
  c.sessionUUID = "abc-def" → newPlayer(c).session == "abc-def"

TestNewPlayer_SessionEmptyWhenClientEmpty
  c.sessionUUID = "" → newPlayer(c).session == ""
  (tick fallback applies later — covered separately)
```

### 6.4 World-side e2e (`modules/world/friends_smoke_test.go`)

`TestFriendsClient_E2E_PlayerSessionIsUUID` — full login flow through the real login server (mirrors the existing `TestFriendsClient_E2E_PrivateMessagePersistsRow` shape):

1. Start a real `modules/login` server on a buffered listener.
2. Drive a world `client` through `handleLogin` to login as "smoke".
3. Inspect the `*Player` admitted to the world (via `processLogins`) and assert:
   - `p.session != "headless"` and `p.session != ""`.
   - `uuid.Parse(p.session)` succeeds and the parsed UUID's `Version() == 4`.
4. Cross-check against the `session` table row: `SELECT session_uuid FROM session WHERE account_id = ?` returns the same value.

This is the gate test for the slice — proves login UUID → `Player.session` end-to-end on real DB + real proto + real gRPC.

### 6.5 Existing-tests audit (regression sweep)

- `modules/login/handler_test.go` existing tests (`TestPlayerLogin_NewPlayer_AutoRegister`, `TestPlayerLogin_ExistingPlayer`, etc.) — no breaking change; they don't assert on `resp.SessionUuid`. They keep passing because the new field is a non-required addition.
- `modules/world/login_client_test.go` — extension as above; existing assertions unchanged in shape.
- `modules/world/friends_smoke_test.go` slice-6 PM-persistence e2e — unaffected; it doesn't read `p.session`.
- `modules/world/session_log.go:24` and `modules/world/logger_bridge.go:30,44` — already read `p.session`. The behaviour change (`"headless"` → real UUID) is observed transparently; no test change needed at those sites unless a test explicitly pins `session=headless`. Audit at execute time and update any such tests to either accept either value or use the unbridged path.

## 7. Tag retirement

The umbrella tag `NAI-72-D-LOGIN-SERVER-BRIDGE-MOD` was fully removed from code by NAI-214 (`bridges.go:23-26` comment block deleted). The UUID-half carry-forward exists only as a doc-comment on `Player.session` (lines 291-296) and as historical references in older specs (NAI-72 / NAI-73 / NAI-74). Slice 7:

- Rewrites the `Player.session` doc-comment to remove the "carry-forward from NAI-72" sentence (see §5.4).
- Updates the `tick.go:288-295` slice-comment.
- Leaves older spec docs untouched (they're frozen history).

No new tag is opened. Slice 7 is the closing slice of the arc and ships clean.

## 8. Non-goals

- **Public chat** — `NAI-S6-D-PUBLIC-CHAT-DEFERRED` retires by a separate follow-up slice (post-7) that adds the `public_chat` table + `PublicMessageRequest` RPC + world-side hook. That follow-up depends on slice 7's `Player.session` UUID.
- **Session-table reads** — no RPC for "give me my session row by uuid." Admin tooling lives elsewhere.
- **Session expiry** — TS doesn't expire session rows; goscape doesn't either. Permanent.
- **Reconnect UUID reuse** — every `PlayerLogin` call gets a fresh UUID, including `RECONNECT_OK`. TS does the same (`crypto.randomUUID()` per login attempt; reconnects are new logical sessions even when the account is the same).
- **Session_uuid in `PlayerLoginRequest`** — the world does NOT send a UUID up; the login server is authoritative for session identity. Out of scope.

## 9. Execution order (preview for plan)

The plan will split this into four tasks roughly:

- **T1 — Proto + regen.** Update `proto/login/login.proto`; `make protos`; commit.
- **T2 — Login-server handler.** `uuid.NewString()` plumbing; `buildLoginResponse` signature; `go.mod` promotion. Tests in 6.1.
- **T3 — World plumbing.** `client.sessionUUID` field; `callPlayerLoginRPC` cache; `newPlayer` copy; doc-comment + tick-comment retire. Tests in 6.2 + 6.3.
- **T4 — World-side e2e.** Test in 6.4. Final regression sweep per 6.5.

Each task is independently verifiable: T1 ships a proto-only change that compiles and re-runs `go build`; T2 makes the login server populate the field but no consumer reads it yet (still works); T3 starts consuming it but the e2e is in T4; T4 is the gate.

## 10. Verification gate

```bash
unset GOROOT; export PATH="$HOME/go/current/bin:$PATH"
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./... -count=1 -timeout 600s
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go run -trimpath ./cmd/goscape-cli smoke-pack \
  --content-dir $HOME/Code/github.com/LostCityRS/content
```

Expected: 30 packages PASS; smoke-pack 12 OK / 0 ERR / 0 SKIP.

## 11. Risks

- **Proto regen drift.** If `make protos` produces unexpected diff in unrelated `.pb.go` (e.g. friendspb), the executor must NOT commit those changes as part of T1. Slice 5a saw this pattern; the discipline is to commit only the affected file.
- **Existing test pins on `session=headless`.** A grep for `"headless"` across `modules/world/` is part of T3's preflight; any test that pins `session=headless` either gets switched to the unbridged path or relaxed to "non-empty."
- **Logger-bridge log output change.** Slog lines that previously included `session=headless` will now include `session=<uuid>` for bridged logins. Any test that pins the literal `session=headless` in log output must be updated. Audit `modules/world/logger_bridge*_test.go` at T3.
- **Pre-commit `git status`.** `[[git-pre-commit-status-check]]` — concurrent shell can stage files; always `git status` immediately before `git commit` and `git show --stat HEAD` after.

## 12. After slice 7

The friends-server bridge arc is fully closed. The single remaining follow-up is the `public_chat` slice (retires `NAI-S6-D-PUBLIC-CHAT-DEFERRED`); it consumes the now-real `Player.session` UUID. The follow-up's spec/plan will live separately, not as a slice of this arc.
