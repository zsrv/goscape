# Friends-server follow-up — `public_chat` persistence implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Persist every public-chat utterance server-side to a new SQLite `public_chat` table on the friends-server, keyed by the per-login `Player.session` UUID introduced in slice 7. Retire `NAI-S6-D-PUBLIC-CHAT-DEFERRED`.

**Architecture:** Add migration `000003_public_chat.up.sql` and `Repository.LogPublicMessage(ctx, sessionUUID, coord, message) error`. New `handler.PublicMessage` gRPC method: blocking insert; insert error → `codes.Internal`. New proto RPC `PublicMessage(PublicMessageRequest) returns (google.protobuf.Empty)`. New `FriendsClient.PublicMessage` (void-returning, fire-and-forget) + new `FriendsBridge.PublicMessage` (clean signature: `sessionUUID, coord, message`). World-side hook in `handleMessagePublic` fires through the bridge after `p.Chat(...)`; the existing `grpcFriendsBridge` goroutine-wraps the underlying RPC so the tick never blocks. Skip the hook when `p.session == ""` or `p.session == "headless"`. The TS handler does **nothing but insert** — no delivery, no validation; we match.

**Tech Stack:** Go 1.x, `modernc.org/sqlite`, `golang-migrate/migrate/v4`, `embed.FS` migrations, `database/sql`, `google.golang.org/grpc`, `pkg/friendspb`, `pkg/wordenc/wordpack`, `pkg/coordgrid`.

**Predecessor:** slice 7 close commit `0c0709b8`; spec commit `f23ab975` at `docs/superpowers/specs/2026-05-20-friends-public-chat-followup-design.md`.

**Gate (must hold post-task-9):** `-race` clean across all 56 packages; smoke-pack 12 OK / 0 ERR / 0 SKIP.

---

## Discipline notes — read in every implementer subagent prompt

These carry forward from `[[friends-server-slice7-close]]` and the resume file. They are non-negotiable.

1. **NEVER `git checkout` / `git restore` tracked files.** Slice 5a T0 lost the user's in-flight Makefile edits this way; slice 6 spec-commit incident recovered with `git reset --mixed HEAD~1`. Escape hatch is BLOCKED-status reporting, not destructive git.
2. **Test helper files use `_test.go` suffix.**
3. **Pre-flight env (every shell session):**
   ```bash
   unset GOROOT
   export PATH="/home/owner/go/current/bin:$PATH"
   ```
4. **Go-test prefix (required by global CLAUDE.md):**
   ```bash
   GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/friends/... -count=1
   ```
   Replace the package selector per task.
5. **`git commit --no-gpg-sign`** — required by global CLAUDE.md.
6. **Before every commit:** run `git status` to confirm the staged set, then `git show --stat HEAD` after — concurrent shell edits from the user can sneak unrelated changes into the index. Recover with `git reset --mixed HEAD~1` (never amend). Per `[[git-pre-commit-status-check]]` feedback.
7. **Stale-IDE-LSP / `go list` "No packages found" diagnostics are environmental** — ignore them.
8. **`go test -race` always.** The world-side hook fires from the tick goroutine into a goroutine-wrapped RPC call inside `grpcFriendsBridge`. Verify no in-tick blocking and no data races on the fake's capture slice.
9. **`make protos` regenerates ALL `.pb.go`.** Stage only the friends-related `.pb.go` changes; revert any unrelated drift before commit.
10. **The world MUST NOT block on the friends-server RPC.** Goroutine wrap is in `grpcFriendsBridge.PublicMessage` (matches existing slice-4b `grpcFriendsBridge.PrivateMessage`). Do NOT add a second `go` wrap at the call site.

---

## File structure

| File | Action | Responsibility |
|---|---|---|
| `modules/friends/migrations/000003_public_chat.up.sql` | **create** | DDL for `public_chat` table + 2 composite indexes |
| `modules/friends/db_test.go` | **modify** | Extend `TestOpenDB_AppliesMigrations` to include `public_chat` |
| `modules/friends/repository.go` | **modify** | Add `LogPublicMessage(ctx, sessionUUID, coord, message) error` |
| `modules/friends/repository_test.go` | **modify** | 4 new `TestRepository_LogPublicMessage_*` tests |
| `proto/friends/friends.proto` | **modify** | Add `rpc PublicMessage(PublicMessageRequest) returns (...)` + `message PublicMessageRequest` |
| `pkg/friendspb/friends.pb.go` | **regenerate** | `make protos` |
| `pkg/friendspb/friends_grpc.pb.go` | **regenerate** | `make protos` |
| `modules/friends/handler.go` | **modify** | Add `PublicMessage(ctx, *friendspb.PublicMessageRequest) (*emptypb.Empty, error)` |
| `modules/friends/handler_test.go` | **modify** | 2 new tests (`PersistsRow`, `InsertErrorReturnsInternal`) |
| `modules/world/friends_client.go` | **modify** | Add `PublicMessage(ctx, req)` interface method + `grpcFriendsClient.PublicMessage` impl |
| `modules/world/friends_client_fake_test.go` | **modify** | Extend `fakeFriendsClient` with `publicMessageReqs` channel + `PublicMessage` method |
| `modules/world/bridges.go` | **modify** | Add `FriendsBridge.PublicMessage`, `grpcFriendsBridge.PublicMessage`, `noopBridges.PublicMessage` |
| `modules/world/bridges_test.go` | **modify** | Add `recordedPublicMessageCall` + `recordingBridges.PublicMessage` + `publicMsgs` slice |
| `modules/world/handlers_game.go` | **modify** | Extend `handleMessagePublic` with the audit hook |
| `modules/world/handler_message_public_test.go` | **create** | 3 new unit tests for the audit hook |
| `modules/world/friends_smoke_test.go` | **modify** | Add `TestFriendsClient_E2E_PublicMessagePersistsRow` |

LOC estimate: ~310 added, ~0 deleted.

---

## Task list

- **T1** — `public_chat` migration + extend `TestOpenDB_AppliesMigrations`
- **T2** — `Repository.LogPublicMessage` + 4 SQL-concern tests (TDD)
- **T3** — Proto `PublicMessage` RPC + regenerate
- **T4** — `handler.PublicMessage` + 2 handler tests (TDD)
- **T5** — `FriendsClient.PublicMessage` interface method + grpc impl + fake extension
- **T6** — `FriendsBridge.PublicMessage` + grpc/noop/recording impls
- **T7** — `handleMessagePublic` hook + 3 unit tests (TDD)
- **T8** — World-side e2e persistence test
- **T9** — Full `-race` suite + smoke-pack gate

---

### Task 1 — `public_chat` migration + DB migration test

**Files:**
- Create: `modules/friends/migrations/000003_public_chat.up.sql`
- Modify: `modules/friends/db_test.go`

- [ ] **Step 1: Extend `TestOpenDB_AppliesMigrations` to assert `public_chat`**

Edit `modules/friends/db_test.go`, change the `wantTables` slice from `[]string{"friendlist", "ignorelist", "private_chat"}` to:

```go
	wantTables := []string{"friendlist", "ignorelist", "private_chat", "public_chat"}
```

- [ ] **Step 2: Run the test — expect FAIL**

```bash
unset GOROOT; export PATH="/home/owner/go/current/bin:$PATH"
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/friends/ -run TestOpenDB_AppliesMigrations -count=1
```

Expected: FAIL with `table "public_chat" missing` (or sqlite-scanner equivalent).

- [ ] **Step 3: Create the migration file**

Create `modules/friends/migrations/000003_public_chat.up.sql` with:

```sql
CREATE TABLE public_chat (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    profile TEXT NOT NULL,
    session_uuid TEXT NOT NULL,
    coord INTEGER NOT NULL,
    message TEXT NOT NULL,
    created TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_public_chat_session
    ON public_chat (profile, session_uuid, created);

CREATE INDEX idx_public_chat_recent
    ON public_chat (profile, created);
```

- [ ] **Step 4: Run the test — expect PASS**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/friends/ -run TestOpenDB_AppliesMigrations -count=1
```

Expected: PASS.

- [ ] **Step 5: Run the full friends-pkg test suite — expect PASS**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/friends/... -count=1
```

Expected: ok (no regression in slice 1-6 tests).

- [ ] **Step 6: Commit**

```bash
git status
git add modules/friends/migrations/000003_public_chat.up.sql modules/friends/db_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
friends: public_chat T1 — add public_chat migration + DB test

Adds modules/friends/migrations/000003_public_chat.up.sql with a single
public_chat table keyed by (profile, session_uuid) plus two composite
indexes for per-session and recent-activity reads. Extends the existing
TestOpenDB_AppliesMigrations to assert the table exists post-migration.

Mirrors slice 6's private_chat schema posture; session_uuid replaces the
two username37 columns (TS-faithful — admin tooling joins to the
login-server session table downstream).
EOF
)"
git show --stat HEAD
```

---

### Task 2 — `Repository.LogPublicMessage` + 4 SQL-concern tests (TDD)

**Files:**
- Modify: `modules/friends/repository.go`
- Modify: `modules/friends/repository_test.go`

- [ ] **Step 1: Add 4 failing tests at the bottom of `modules/friends/repository_test.go`**

Append:

```go
// --- public_chat persistence (follow-up post-slice-7) ---

func TestRepository_LogPublicMessage_PersistsRow(t *testing.T) {
	r, db := newTestRepo(t)
	ctx := t.Context()
	if err := r.LogPublicMessage(ctx, "uuid-aaa", 54321, "hello"); err != nil {
		t.Fatalf("LogPublicMessage: %v", err)
	}
	var n int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM public_chat WHERE profile = 'test'`).Scan(&n); err != nil {
		t.Fatalf("COUNT query: %v", err)
	}
	if n != 1 {
		t.Fatalf("row count = %d, want 1", n)
	}
	var sess, msg string
	var coord int32
	if err := db.QueryRowContext(ctx,
		`SELECT session_uuid, coord, message FROM public_chat`).
		Scan(&sess, &coord, &msg); err != nil {
		t.Fatalf("SELECT row: %v", err)
	}
	if sess != "uuid-aaa" {
		t.Errorf("session_uuid = %q, want %q", sess, "uuid-aaa")
	}
	if coord != 54321 {
		t.Errorf("coord = %d, want 54321", coord)
	}
	if msg != "hello" {
		t.Errorf("message = %q, want %q", msg, "hello")
	}
}

func TestRepository_LogPublicMessage_AppendOnly(t *testing.T) {
	r, db := newTestRepo(t)
	ctx := t.Context()
	if err := r.LogPublicMessage(ctx, "uuid-bbb", 0, "first"); err != nil {
		t.Fatalf("first: %v", err)
	}
	if err := r.LogPublicMessage(ctx, "uuid-bbb", 0, "second"); err != nil {
		t.Fatalf("second: %v", err)
	}
	var n int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM public_chat`).Scan(&n); err != nil {
		t.Fatalf("COUNT: %v", err)
	}
	if n != 2 {
		t.Errorf("row count = %d, want 2 (append-only, no dedupe)", n)
	}
}

func TestRepository_LogPublicMessage_RespectsProfile(t *testing.T) {
	r, db := newTestRepo(t) // profile = "test"
	r2 := NewRepository(db, "other")
	ctx := t.Context()
	if err := r.LogPublicMessage(ctx, "uuid-x", 0, "from default"); err != nil {
		t.Fatalf("r: %v", err)
	}
	if err := r2.LogPublicMessage(ctx, "uuid-x", 0, "from other"); err != nil {
		t.Fatalf("r2: %v", err)
	}
	rows, err := db.QueryContext(ctx,
		`SELECT profile, message FROM public_chat ORDER BY id`)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer rows.Close()
	type pair struct {
		profile string
		message string
	}
	var got []pair
	for rows.Next() {
		var p pair
		if err := rows.Scan(&p.profile, &p.message); err != nil {
			t.Fatalf("scan: %v", err)
		}
		got = append(got, p)
	}
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2", len(got))
	}
	if got[0] != (pair{"test", "from default"}) {
		t.Errorf("got[0] = %+v, want {test, from default}", got[0])
	}
	if got[1] != (pair{"other", "from other"}) {
		t.Errorf("got[1] = %+v, want {other, from other}", got[1])
	}
}

func TestRepository_LogPublicMessage_EmptyMessageAllowed(t *testing.T) {
	r, db := newTestRepo(t)
	ctx := t.Context()
	if err := r.LogPublicMessage(ctx, "uuid-empty", 0, ""); err != nil {
		t.Fatalf("LogPublicMessage(empty): %v", err)
	}
	var n int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM public_chat WHERE message = ''`).Scan(&n); err != nil {
		t.Fatalf("COUNT: %v", err)
	}
	if n != 1 {
		t.Errorf("row count = %d, want 1 (empty message allowed, no server-side validation)", n)
	}
}
```

- [ ] **Step 2: Run the new tests — expect FAIL**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/friends/ -run TestRepository_LogPublicMessage -count=1
```

Expected: compilation failure (`r.LogPublicMessage undefined`).

- [ ] **Step 3: Add `LogPublicMessage` to `modules/friends/repository.go`**

Append to the file (after the existing `LogPrivateMessage` method):

```go
// LogPublicMessage appends one row to public_chat under r.profile.
// Mirrors TS FriendServer.ts:286-297 — append-only, no dedupe, no
// validation, no session_uuid existence check. Insert is the
// synchronous gate for the PublicMessage RPC: a failure here returns
// an error to the handler which surfaces codes.Internal to the caller,
// matching the TS thrown-await pattern and slice 6's posture.
func (r *Repository) LogPublicMessage(ctx context.Context, sessionUUID string, coord int32, message string) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO public_chat (profile, session_uuid, coord, message)
		 VALUES (?, ?, ?, ?)`,
		r.profile, sessionUUID, coord, message,
	)
	if err != nil {
		return fmt.Errorf("LogPublicMessage: %w", err)
	}
	return nil
}
```

- [ ] **Step 4: Run the 4 tests — expect PASS**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/friends/ -run TestRepository_LogPublicMessage -count=1
```

Expected: ok (4 PASS).

- [ ] **Step 5: Run the full friends-pkg test suite — expect PASS**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/friends/... -count=1
```

Expected: ok.

- [ ] **Step 6: Commit**

```bash
git status
git add modules/friends/repository.go modules/friends/repository_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
friends: public_chat T2 — Repository.LogPublicMessage + SQL tests

Adds Repository.LogPublicMessage(ctx, sessionUUID, coord, message) error
as a thin INSERT into public_chat under r.profile. Mirrors slice 6's
LogPrivateMessage signature shape — ctx + error return, single
ExecContext, no validation. Adds 4 SQL-concern tests covering
persistence, append-only semantics, profile scoping, and
empty-message tolerance.
EOF
)"
git show --stat HEAD
```

---

### Task 3 — Proto `PublicMessage` RPC + regenerate

**Files:**
- Modify: `proto/friends/friends.proto`
- Regenerate: `pkg/friendspb/friends.pb.go`, `pkg/friendspb/friends_grpc.pb.go`

- [ ] **Step 1: Add the new RPC to the `FriendsService` definition in `proto/friends/friends.proto`**

Inside the `service FriendsService { ... }` block, immediately after the existing `rpc PrivateMessage(...)` line:

```proto
  // Public-chat audit log. Mirrors TS FriendServer.ts:286-297 — append-
  // only persistence keyed by per-login session UUID (Player.session,
  // populated by slice 7). No delivery half; the world handles in-world
  // chat propagation itself. Insert error → codes.Internal.
  rpc PublicMessage(PublicMessageRequest) returns (google.protobuf.Empty);
```

- [ ] **Step 2: Add the new message type at the bottom of the messages section**

Place after the existing `message PrivateMessageRequest { ... }` block:

```proto
message PublicMessageRequest {
  int32  world_id     = 1;
  string session_uuid = 2;
  int32  coord        = 3;
  string chat         = 4;
}
```

- [ ] **Step 3: Regenerate friends `.pb.go` files**

```bash
unset GOROOT; export PATH="/home/owner/go/current/bin:$PATH"
make protos
```

Expected: protoc invocations succeed; `pkg/friendspb/friends.pb.go` and `pkg/friendspb/friends_grpc.pb.go` are updated.

- [ ] **Step 4: Verify only friends `.pb.go` files changed**

```bash
git status
```

Expected: `modified: proto/friends/friends.proto`, `modified: pkg/friendspb/friends.pb.go`, `modified: pkg/friendspb/friends_grpc.pb.go`.

If other `.pb.go` files show as modified (e.g., `pkg/loginpb/*`), revert them:

```bash
git checkout HEAD -- pkg/loginpb/  # ONLY if they show modified AND are unrelated
```

**Caveat:** before invoking `git checkout`, re-read discipline lesson #1. If any of those changes appear to be legitimate user WIP, STOP and report BLOCKED — do not destructively revert.

- [ ] **Step 5: Verify build**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
```

Expected: ok (the new `PublicMessage` server interface method on `FriendsServiceServer` is satisfied by `UnimplementedFriendsServiceServer` embedded in `handler`, so the build passes even before T4 adds the real handler).

- [ ] **Step 6: Run a quick smoke test to confirm friends-pkg still compiles & tests pass**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/friends/... -count=1
```

Expected: ok.

- [ ] **Step 7: Commit**

```bash
git status
git add proto/friends/friends.proto pkg/friendspb/friends.pb.go pkg/friendspb/friends_grpc.pb.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
friends: public_chat T3 — proto PublicMessage RPC + regenerate

Adds rpc PublicMessage(PublicMessageRequest) returns (google.protobuf
.Empty) to FriendsService, plus PublicMessageRequest with envelope-first
field order matching PrivateMessageRequest: world_id, session_uuid,
coord, chat. Regenerates pkg/friendspb/friends{,_grpc}.pb.go via
make protos.

UnimplementedFriendsServiceServer satisfies the new method on the
server side; T4 replaces it with the real handler.
EOF
)"
git show --stat HEAD
```

---

### Task 4 — `handler.PublicMessage` + 2 handler tests (TDD)

**Files:**
- Modify: `modules/friends/handler.go`
- Modify: `modules/friends/handler_test.go`

- [ ] **Step 1: Add 2 failing tests at the bottom of `modules/friends/handler_test.go`**

Append:

```go
// --- public_chat audit (follow-up post-slice-7) ---

// TestHandler_PublicMessage_PersistsRow pins the happy path: a valid
// PublicMessageRequest returns (Empty, nil) AND the row is visible in
// public_chat under r.profile. No delivery, no subscription, no
// validation. Mirrors TS FriendServer.ts:286-297.
func TestHandler_PublicMessage_PersistsRow(t *testing.T) {
	r, db := newTestRepo(t)
	log := noopLogger()
	cfg := Config{NodeProfile: "main", WorldPlayerLimit: 100}
	h := &handler{repo: r, subs: newSubscriptions(log), cfg: cfg, log: log}

	resp, err := h.PublicMessage(t.Context(), &friendspb.PublicMessageRequest{
		WorldId:     10,
		SessionUuid: "uuid-pub-1",
		Coord:       9876,
		Chat:        "audit me",
	})
	if err != nil {
		t.Fatalf("PublicMessage: %v", err)
	}
	if resp == nil {
		t.Fatalf("PublicMessage: nil response, want non-nil Empty")
	}

	var sess, msg string
	var coord int32
	if err := db.QueryRowContext(t.Context(),
		`SELECT session_uuid, coord, message FROM public_chat`).
		Scan(&sess, &coord, &msg); err != nil {
		t.Fatalf("SELECT public_chat: %v", err)
	}
	if sess != "uuid-pub-1" || coord != 9876 || msg != "audit me" {
		t.Errorf("row = (%q, %d, %q), want (uuid-pub-1, 9876, audit me)", sess, coord, msg)
	}
}

// TestHandler_PublicMessage_InsertErrorReturnsInternal pins that a SQL
// failure on public_chat insert returns codes.Internal. Forces the
// failure by closing the *sql.DB before the call. Mirrors the slice 6
// TestHandler_PrivateMessage_InsertErrorBlocksSend pattern (minus the
// delivery half, which doesn't exist for public_chat).
func TestHandler_PublicMessage_InsertErrorReturnsInternal(t *testing.T) {
	r, db := newTestRepo(t)
	log := noopLogger()
	cfg := Config{NodeProfile: "main", WorldPlayerLimit: 100}
	h := &handler{repo: r, subs: newSubscriptions(log), cfg: cfg, log: log}

	if err := db.Close(); err != nil {
		t.Fatalf("db.Close: %v", err)
	}

	resp, err := h.PublicMessage(t.Context(), &friendspb.PublicMessageRequest{
		WorldId:     10,
		SessionUuid: "uuid-pub-err",
		Coord:       0,
		Chat:        "should not persist",
	})
	if err == nil {
		t.Fatalf("PublicMessage on closed DB: got nil error, want Internal")
	}
	if resp != nil {
		t.Errorf("PublicMessage err path: resp = %+v, want nil", resp)
	}
	if status.Code(err) != codes.Internal {
		t.Fatalf("PublicMessage err code = %v, want %v", status.Code(err), codes.Internal)
	}
}
```

- [ ] **Step 2: Run the new tests — expect FAIL**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/friends/ -run TestHandler_PublicMessage -count=1
```

Expected: compilation failure or runtime panic (`h.PublicMessage` does not exist; only `UnimplementedFriendsServiceServer.PublicMessage` returns `codes.Unimplemented`).

- [ ] **Step 3: Add `PublicMessage` method to `modules/friends/handler.go`**

Append at the bottom of the file (after `SubscribeWorldEvents`):

```go
// PublicMessage persists one row to public_chat. Mirrors TS
// FriendServer.ts:286-297 — append-only, no delivery, no validation,
// no session_uuid existence check. Insert error → codes.Internal
// (matches slice 6 PrivateMessage posture and FRIENDLIST/IGNORELIST
// mutation handlers).
//
// Retires NAI-S6-D-PUBLIC-CHAT-DEFERRED.
func (h *handler) PublicMessage(ctx context.Context, req *friendspb.PublicMessageRequest) (*emptypb.Empty, error) {
	if err := h.repo.LogPublicMessage(ctx, req.SessionUuid, req.Coord, req.Chat); err != nil {
		return nil, status.Errorf(codes.Internal, "LogPublicMessage: %v", err)
	}
	return &emptypb.Empty{}, nil
}
```

- [ ] **Step 4: Run the 2 tests — expect PASS**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/friends/ -run TestHandler_PublicMessage -count=1
```

Expected: ok (2 PASS).

- [ ] **Step 5: Run the full friends-pkg test suite — expect PASS**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/friends/... -count=1
```

Expected: ok.

- [ ] **Step 6: Commit**

```bash
git status
git add modules/friends/handler.go modules/friends/handler_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
friends: public_chat T4 — handler.PublicMessage + 2 tests

Adds gRPC handler.PublicMessage that mirrors TS FriendServer.ts:286-297
— blocking insert into public_chat under r.profile, no delivery, no
validation. Insert error → codes.Internal (matches slice 6 PM posture).
Two tests pin the happy path and the insert-error → Internal contract.

Retires NAI-S6-D-PUBLIC-CHAT-DEFERRED on the friends-server side; the
world-side hook lands in T7.
EOF
)"
git show --stat HEAD
```

---

### Task 5 — `FriendsClient.PublicMessage` + grpc + fake

**Files:**
- Modify: `modules/world/friends_client.go`
- Modify: `modules/world/friends_client_fake_test.go`

- [ ] **Step 1: Extend `FriendsClient` interface in `modules/world/friends_client.go`**

Locate the interface block (lines ~13-58 in the existing file). Add a new method after the existing `PrivateMessage` line:

```go
	// PublicMessage audit-logs a public-chat utterance to the friends-
	// server. Fire-and-forget per the FriendsClient convention; the
	// grpc impl logs warn + swallows errors. Mirrors TS
	// FriendsClient.publicMessage (FriendServer.ts:669-694 inline).
	PublicMessage(ctx context.Context, req *friendspb.PublicMessageRequest)
```

- [ ] **Step 2: Add `grpcFriendsClient.PublicMessage` impl after `PrivateMessage`**

Locate the existing `grpcFriendsClient.PrivateMessage` method (search for `func (c *grpcFriendsClient) PrivateMessage`). Insert the following immediately after it, before the `SubscribeUpdates` impl:

```go
// PublicMessage audit-logs a public-chat utterance to the friends server.
// Fire-and-forget — errors are logged at Warn and swallowed.
func (c *grpcFriendsClient) PublicMessage(ctx context.Context, req *friendspb.PublicMessageRequest) {
	if _, err := c.client.PublicMessage(ctx, req); err != nil {
		c.log.Warn("PublicMessage RPC failed",
			slog.String("session_uuid", req.SessionUuid),
			slog.Any("err", err),
		)
	}
}
```

- [ ] **Step 3: Extend `fakeFriendsClient` in `modules/world/friends_client_fake_test.go`**

Three edits in this file:

(a) Add a new channel field to the `fakeFriendsClient` struct. Locate the line:
```go
	privateMessageReqs chan *friendspb.PrivateMessageRequest
```
Add directly below:
```go
	publicMessageReqs  chan *friendspb.PublicMessageRequest
```

(b) Add the channel to `newFakeFriendsClient()`. Locate the line:
```go
		privateMessageReqs:  make(chan *friendspb.PrivateMessageRequest, 16),
```
Add directly below:
```go
		publicMessageReqs:   make(chan *friendspb.PublicMessageRequest, 16),
```

(c) Add the `PublicMessage` method after `PrivateMessage`:

```go
func (f *fakeFriendsClient) PublicMessage(ctx context.Context, req *friendspb.PublicMessageRequest) {
	select {
	case f.publicMessageReqs <- req:
	default:
	}
}
```

- [ ] **Step 4: Verify build**

```bash
unset GOROOT; export PATH="/home/owner/go/current/bin:$PATH"
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./modules/world/...
```

Expected: ok. The compile-time assertion `var _ FriendsClient = (*fakeFriendsClient)(nil)` (line 92 in fake_test.go) catches incomplete impls.

- [ ] **Step 5: Run modules/world tests — expect PASS**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/... -count=1 -timeout 300s
```

Expected: ok (no regression — no callers exercise the new method yet).

- [ ] **Step 6: Commit**

```bash
git status
git add modules/world/friends_client.go modules/world/friends_client_fake_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
friends: public_chat T5 — FriendsClient.PublicMessage shim + fake

Extends the world-side FriendsClient interface with a void-returning
PublicMessage(ctx, req) method (fire-and-forget per the existing
convention — see file-level doc-comment on FriendsClient). Adds the
production grpcFriendsClient.PublicMessage impl (warn-log + swallow on
RPC error) and extends fakeFriendsClient with a cap-16 buffered
publicMessageReqs capture channel.

No callers yet; T6 adds the FriendsBridge layer, T7 wires the hook.
EOF
)"
git show --stat HEAD
```

---

### Task 6 — `FriendsBridge.PublicMessage` + grpc / noop / recording

**Files:**
- Modify: `modules/world/bridges.go`
- Modify: `modules/world/bridges_test.go`

- [ ] **Step 1: Extend `FriendsBridge` interface in `modules/world/bridges.go`**

Locate the `FriendsBridge` interface (search for `type FriendsBridge interface`). Add a new method after the existing `PrivateMessage(...)` declaration:

```go
	// PublicMessage audit-logs a public-chat utterance to the friends
	// server. sessionUUID is the per-login UUID (Player.session,
	// populated by slice 7). coord is the packed coordgrid.PackCoord
	// value at utterance. message is the WordPack-decoded text (not
	// the raw word-packed bytes — see handleMessagePublic for the
	// decode site). Real impl: grpcFriendsBridge.PublicMessage (this
	// file) fans a friendspb.PublicMessageRequest out to the friends
	// server. Mirrors TS World.sendPublicMessageLog inline payload
	// (FriendServer.ts:670-694 publicMessage emitter).
	PublicMessage(sessionUUID string, coord int, message string)
```

- [ ] **Step 2: Add `grpcFriendsBridge.PublicMessage` impl**

Locate the `grpcFriendsBridge.PrivateMessage` method (search for `func (b *grpcFriendsBridge) PrivateMessage`). Insert directly after it, before the compile-time `var _ FriendsBridge = (*grpcFriendsBridge)(nil)` assertion:

```go
func (b *grpcFriendsBridge) PublicMessage(sessionUUID string, coord int, message string) {
	go b.client.PublicMessage(context.Background(), &friendspb.PublicMessageRequest{
		WorldId:     b.worldID,
		SessionUuid: sessionUUID,
		Coord:       int32(coord),
		Chat:        message,
	})
}
```

- [ ] **Step 3: Add `noopBridges.PublicMessage` impl**

Locate the existing `noopBridges` method block (search for `func (noopBridges) PrivateMessage`). Add directly after it:

```go
func (noopBridges) PublicMessage(string, int, string) {}
```

- [ ] **Step 4: Extend `recordingBridges` in `modules/world/bridges_test.go`**

Three edits:

(a) Add a new struct type at the top, alongside the existing `recordedPrivateMessageCall`:

```go
type recordedPublicMessageCall struct {
	method      string // "PublicMessage"
	sessionUUID string
	coord       int
	message     string
}
```

(b) Add a new slice field to the `recordingBridges` struct. Locate:
```go
	privateMsgs          []recordedPrivateMessageCall // NAI-158
```
Add directly below:
```go
	publicMsgs           []recordedPublicMessageCall  // public_chat follow-up
```

(c) Add the recorder method after `PrivateMessage`:

```go
func (r *recordingBridges) PublicMessage(sessionUUID string, coord int, message string) {
	r.publicMsgs = append(r.publicMsgs, recordedPublicMessageCall{
		method: "PublicMessage", sessionUUID: sessionUUID, coord: coord, message: message,
	})
}
```

- [ ] **Step 5: Verify build**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./modules/world/...
```

Expected: ok. The compile-time assertions in `bridges_test.go` (`_ FriendsBridge = noopBridges{}` and `_ FriendsBridge = (*recordingBridges)(nil)`) catch incomplete impls.

- [ ] **Step 6: Run modules/world tests — expect PASS**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/... -count=1 -timeout 300s
```

Expected: ok (no regression; `TestNoopBridgesAllMethods` and `TestRecordingBridgesCapturesAllCalls` should pass — they exercise every method but only assert what they touch). If any existing test broke because it iterates over expected method coverage, append a `PublicMessage(...)` call to it to match the slice-6 pattern.

- [ ] **Step 7: Commit**

```bash
git status
git add modules/world/bridges.go modules/world/bridges_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
friends: public_chat T6 — FriendsBridge.PublicMessage

Adds FriendsBridge.PublicMessage(sessionUUID, coord, message) — the
clean-signature world-side surface for public-chat audit. Production
grpcFriendsBridge.PublicMessage builds a friendspb.PublicMessageRequest
and fires the underlying FriendsClient.PublicMessage in a goroutine so
the tick never blocks on network I/O (mirrors the existing
PrivateMessage shape).

noopBridges and recordingBridges both gain a PublicMessage method to
satisfy the interface; recordingBridges captures into a new publicMsgs
slice for per-handler assertion in T7.
EOF
)"
git show --stat HEAD
```

---

### Task 7 — `handleMessagePublic` hook + 3 unit tests (TDD)

**Files:**
- Create: `modules/world/handler_message_public_test.go`
- Modify: `modules/world/handlers_game.go`

- [ ] **Step 1: Create the test file with 3 failing tests**

Create `modules/world/handler_message_public_test.go` with:

```go
package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/coordgrid"
	"github.com/zsrv/goscape/pkg/io/packet"
	"github.com/zsrv/goscape/pkg/wordenc/wordpack"
)

// commonMessagePublicSetup wires a player against a server with
// recording bridges and a known username + session. Mirrors
// commonMessagePrivateSetup in handler_message_private_test.go.
func commonMessagePublicSetup(t *testing.T) (*Player, *recordingBridges) {
	t.Helper()
	s := newTestServer(t)
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.username = "alice"
	p.session = "uuid-sess-1"
	rec := installRecordingBridges(s)
	return p, rec
}

// packPublicChatPayload returns an opcode-158 MESSAGE_PUBLIC payload:
// [color, effect, word-packed(message)].
func packPublicChatPayload(color, effect byte, message string) []byte {
	out := []byte{color, effect}
	pk := packet.NewPacket(nil)
	wordpack.Pack(pk, message)
	return append(out, pk.Data...)
}

// TestHandleMessagePublic_FiresFriendsBridge pins that a valid
// public-chat utterance triggers FriendsBridge.PublicMessage with the
// expected (sessionUUID, coord, decoded message) tuple. Coord is the
// packed coordgrid.PackCoord(level, x, z) value at utterance.
func TestHandleMessagePublic_FiresFriendsBridge(t *testing.T) {
	p, rec := commonMessagePublicSetup(t)
	// Move the player to a known coord so PackCoord output is deterministic.
	p.level, p.x, p.z = 0, 3210, 3210

	payload := packPublicChatPayload(0, 0, "hi")
	if err := handleMessagePublic(p, payload); err != nil {
		t.Fatalf("handleMessagePublic: %v", err)
	}

	if len(rec.publicMsgs) != 1 {
		t.Fatalf("publicMsgs: got %d, want 1", len(rec.publicMsgs))
	}
	got := rec.publicMsgs[0]
	if got.sessionUUID != "uuid-sess-1" {
		t.Errorf("sessionUUID: got %q, want uuid-sess-1", got.sessionUUID)
	}
	wantCoord := coordgrid.PackCoord(0, 3210, 3210)
	if got.coord != wantCoord {
		t.Errorf("coord: got %d, want %d", got.coord, wantCoord)
	}
	if got.message != "Hi" { // wordpack.Unpack applies sentence-case to "hi"
		t.Errorf("message: got %q, want %q (sentence-cased)", got.message, "Hi")
	}
}

// TestHandleMessagePublic_SkipsWhenSessionHeadless pins that the audit
// hook is skipped when p.session == "headless" (unbridged path; tick
// fallback from slice 7 — audit row would be meaningless).
func TestHandleMessagePublic_SkipsWhenSessionHeadless(t *testing.T) {
	p, rec := commonMessagePublicSetup(t)
	p.session = "headless"

	payload := packPublicChatPayload(0, 0, "hi")
	if err := handleMessagePublic(p, payload); err != nil {
		t.Fatalf("handleMessagePublic: %v", err)
	}
	if len(rec.publicMsgs) != 0 {
		t.Errorf("publicMsgs: got %d, want 0 (skipped due to headless session)", len(rec.publicMsgs))
	}
	// In-world propagation must still fire.
	if p.chatBytes == nil {
		t.Errorf("p.chatBytes: got nil, want non-nil (Chat must fire regardless of session)")
	}
}

// TestHandleMessagePublic_SkipsWhenSessionEmpty pins the defensive skip
// when p.session == "". Slice 7 stamps p.session at newPlayer(c) so
// this should never happen in production, but the guard is defensive
// against test paths and future regressions.
func TestHandleMessagePublic_SkipsWhenSessionEmpty(t *testing.T) {
	p, rec := commonMessagePublicSetup(t)
	p.session = ""

	payload := packPublicChatPayload(0, 0, "hi")
	if err := handleMessagePublic(p, payload); err != nil {
		t.Fatalf("handleMessagePublic: %v", err)
	}
	if len(rec.publicMsgs) != 0 {
		t.Errorf("publicMsgs: got %d, want 0 (skipped due to empty session)", len(rec.publicMsgs))
	}
}
```

- [ ] **Step 2: Run the new tests — expect FAIL**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestHandleMessagePublic_ -count=1 -timeout 60s
```

Expected: the existing `TestHandleMessagePublic_SetsMaskChat/RightsFromStaffModLevel/ShortPayloadIsNoop` still PASS; the 3 new tests FAIL with "publicMsgs: got 0, want 1" / similar (no hook wired yet).

- [ ] **Step 3: Wire the hook in `handleMessagePublic`**

Edit `modules/world/handlers_game.go` (the `handleMessagePublic` function near line 332). Replace the existing function body:

```go
func handleMessagePublic(p *Player, payload []byte) error {
	if len(payload) < 2 {
		return nil
	}
	color := int(payload[0])
	effect := int(payload[1])
	// Copy the message bytes — the underlying packet buffer may be reused.
	msg := bytes.Clone(payload[2:])
	p.Chat(color, effect, int(p.staffModLevel), msg)
	return nil
}
```

with:

```go
func handleMessagePublic(p *Player, payload []byte) error {
	if len(payload) < 2 {
		return nil
	}
	color := int(payload[0])
	effect := int(payload[1])
	// Copy the message bytes — the underlying packet buffer may be reused.
	msg := bytes.Clone(payload[2:])
	p.Chat(color, effect, int(p.staffModLevel), msg)

	// Audit-log to friends-server. WordPack-decode the message so the
	// row contains human-readable text (matches TS player.logMessage at
	// MessagePublicHandler.ts:18). Skip when p.session is empty or the
	// unbridged "headless" sentinel — audit logging is meaningless
	// without a real per-login UUID. The bridge goroutine-wraps the
	// underlying RPC so the tick never blocks.
	if p.client != nil && p.client.server != nil && p.session != "" && p.session != "headless" {
		s := p.client.server
		pk := packet.NewPacket(msg)
		decoded := wordpack.Unpack(pk, len(msg))
		coord := coordgrid.PackCoord(p.level, p.x, p.z)
		s.friendsBridge.PublicMessage(p.session, coord, decoded)
	}
	return nil
}
```

Verify the file's import block already contains `"github.com/zsrv/goscape/pkg/coordgrid"`, `"github.com/zsrv/goscape/pkg/io/packet"`, and `"github.com/zsrv/goscape/pkg/wordenc/wordpack"`. The `bytes` import is already present. If any of these three are missing, add them — `coordgrid` is already imported (see the existing `parseDebugprocCoord` function in this file); `packet` is imported (used elsewhere); `wordenc/wordpack` may not be — check and add if absent.

- [ ] **Step 4: Run the 3 new tests — expect PASS**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestHandleMessagePublic_ -count=1 -timeout 60s
```

Expected: 6 PASS total (3 existing + 3 new).

- [ ] **Step 5: Run -race over modules/world — expect PASS**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/... -count=1 -timeout 600s
```

Expected: ok.

- [ ] **Step 6: Commit**

```bash
git status
git add modules/world/handlers_game.go modules/world/handler_message_public_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
friends: public_chat T7 — handleMessagePublic audit hook

Extends handleMessagePublic with an audit-log call to
FriendsBridge.PublicMessage after the in-world Chat propagation.
WordPack-decodes the message bytes so the row contains human-readable
text (matches TS player.logMessage at MessagePublicHandler.ts:18).

Skips the hook when p.session is empty or the unbridged "headless"
sentinel — audit logging is meaningless without a real per-login UUID
(slice 7 introduces p.session). The grpcFriendsBridge goroutine-wraps
the underlying RPC; the tick never blocks.

Three new unit tests pin the fires-with-correct-fields contract plus
both skip conditions.
EOF
)"
git show --stat HEAD
```

---

### Task 8 — World-side e2e persistence test

**Files:**
- Modify: `modules/world/friends_smoke_test.go`

- [ ] **Step 1: Add the e2e test at the bottom of `modules/world/friends_smoke_test.go`**

Append:

```go
// TestFriendsClient_E2E_PublicMessagePersistsRow pins the public_chat
// follow-up end-to-end: a client.PublicMessage call against a real
// in-process friends.Friends produces a row in public_chat under
// r.profile, queryable via a second *sql.DB open against the same
// on-disk file. Mirrors slice 6's TestFriendsClient_E2E_
// PrivateMessagePersistsRow.
func TestFriendsClient_E2E_PublicMessagePersistsRow(t *testing.T) {
	port := freePort(t)
	dbPath := filepath.Join(t.TempDir(), "friends.db")
	cfg := friends.Config{
		GRPCListenAddress:       "127.0.0.1",
		GRPCListenPort:          port,
		NodeProfile:             "main",
		WorldPlayerLimit:        100,
		Enable:                  true,
		GracefulShutdownTimeout: 5 * time.Second,
		SQLiteDSN:               dbPath,
	}
	log := discardLogger()
	svc, err := friends.New(cfg, log)
	if err != nil {
		t.Fatalf("friends.New: %v", err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	if err := svc.StartAsync(ctx); err != nil {
		t.Fatalf("StartAsync: %v", err)
	}
	if err := svc.AwaitRunning(ctx); err != nil {
		t.Fatalf("AwaitRunning: %v", err)
	}
	t.Cleanup(func() {
		svc.StopAsync()
		_ = svc.AwaitTerminated(context.Background())
	})

	addr := "127.0.0.1:" + strconv.Itoa(port)
	client, err := NewFriendsClient(addr, log)
	if err != nil {
		t.Fatalf("NewFriendsClient: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	client.WorldConnect(ctx, 10, "main")

	client.PublicMessage(ctx, &friendspb.PublicMessageRequest{
		WorldId:     10,
		SessionUuid: "uuid-e2e-1",
		Coord:       42,
		Chat:        "persisted publicly",
	})

	// Open a second *sql.DB against the same file. Poll up to 2s for
	// the row — synchronous RPC completion should mean the row is
	// already committed, but WAL settling on a fresh file under -race
	// can take a few ms.
	rdb, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { _ = rdb.Close() })

	deadline := time.Now().Add(2 * time.Second)
	var sess, msg string
	var coord int32
	for time.Now().Before(deadline) {
		err := rdb.QueryRowContext(t.Context(),
			`SELECT session_uuid, coord, message
			 FROM public_chat
			 WHERE profile = 'main'
			 ORDER BY id DESC
			 LIMIT 1`).Scan(&sess, &coord, &msg)
		if err == nil {
			break
		}
		if !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("query public_chat: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}

	if sess != "uuid-e2e-1" || coord != 42 || msg != "persisted publicly" {
		t.Errorf("public_chat row = (%q, %d, %q), want (uuid-e2e-1, 42, %q)",
			sess, coord, msg, "persisted publicly")
	}
}
```

- [ ] **Step 2: Run the new test — expect PASS**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/ -run TestFriendsClient_E2E_PublicMessagePersistsRow -count=1 -timeout 60s
```

Expected: PASS within ~1-2 seconds.

- [ ] **Step 3: Run all friends_smoke tests under -race — expect PASS**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/ -run TestFriendsClient_E2E -count=1 -timeout 120s
```

Expected: ok (slice-4a/4b/4c/5a/5b/6/7 e2e tests still green).

- [ ] **Step 4: Commit**

```bash
git status
git add modules/world/friends_smoke_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
friends: public_chat T8 — world-side e2e PublicMessage persistence

Adds TestFriendsClient_E2E_PublicMessagePersistsRow: boots an
in-process friends.Friends service against an on-disk SQLite file,
opens a second *sql.DB on the same file, fires PublicMessage end-to-end
via the world-side FriendsClient, polls briefly for WAL settling, then
selects the row and asserts (session_uuid, coord, message). Mirrors
slice 6's TestFriendsClient_E2E_PrivateMessagePersistsRow.
EOF
)"
git show --stat HEAD
```

---

### Task 9 — Full `-race` suite + smoke-pack gate

**Files:** none modified (verification only).

- [ ] **Step 1: Run the full `-race` suite**

```bash
unset GOROOT; export PATH="/home/owner/go/current/bin:$PATH"
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./... -count=1 -timeout 600s
```

Expected: ok across all 56 packages (matches slice 7 baseline; new tests in `modules/friends/` and `modules/world/` should add ~7-9 assertions). If any package fails, STOP and report BLOCKED with the failure output — do not attempt fix-up commits without per-package diagnosis.

- [ ] **Step 2: Run smoke-pack against real Content**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go run -trimpath ./cmd/goscape-cli smoke-pack --content-dir /home/owner/Code/github.com/LostCityRS/content
```

Expected: 12 OK / 0 ERR / 0 SKIP in ~8-10s. Matches slice 7 baseline. If any stage flips to ERR, STOP and report BLOCKED — public_chat persistence should not affect pack output at all, so an ERR here likely indicates an unrelated regression that crept in via concurrent shell edits.

- [ ] **Step 3: Final `git log` sanity check**

```bash
git log --oneline 0c0709b8..HEAD
```

Expected: 8 commits (T1..T8). T9 makes no commit.

- [ ] **Step 4: Final `git status` — expect clean tracked state**

```bash
git status
```

Expected: working tree clean (untracked files like `.claude/` and `RUNESCRIPT.md` may remain — they are user WIP, never touched by this plan).

---

## Self-review checklist (run after writing the plan)

- [x] **Spec coverage:** every section of `2026-05-20-friends-public-chat-followup-design.md` has a corresponding task. Schema §3 → T1. Repository §4 → T2. Proto §6 → T3. Handler §5 → T4. World FriendsClient §7.1 → T5. (FriendsBridge layer added at plan time per discipline #10; not in spec but required by code conventions.) World hook §7.2 → T6 + T7. Tests §8 (Repository / Handler / World unit / e2e) → T2 / T4 / T7 / T8.
- [x] **Placeholder scan:** no TBD / TODO / "implement later"; every code block is complete.
- [x] **Type consistency:** `Repository.LogPublicMessage(ctx, sessionUUID string, coord int32, message string) error` consistent across T2/T4. `FriendsBridge.PublicMessage(sessionUUID string, coord int, message string)` consistent across T6/T7. `FriendsClient.PublicMessage(ctx, *friendspb.PublicMessageRequest)` consistent across T5/T6. Proto field names `SessionUuid`/`Coord`/`Chat` consistent across T3/T4/T5/T6/T8.
- [x] **Spec deviations at plan time:** §7.2 of the spec proposed a `publishPublicChatAudit` helper on `*Server` with a 5-second context timeout and a call-site `go` wrap. **Plan deviation:** dropped that helper in favor of the existing `FriendsBridge` layer (which already goroutine-wraps internally per slice 4b precedent). Rationale: existing slice-4b `grpcFriendsBridge.PrivateMessage` pattern is exactly what we need; adding a parallel helper would duplicate the wrap. The 5-second context-timeout claim in the spec wasn't backed by precedent — slice 4b PM uses `context.Background()` with no deadline, and we mirror.
- [x] **Self-contained tasks:** each task lists exact file paths, complete code blocks, exact commands with expected output. No "see Task N" cross-references in code blocks.

---

## Closing the follow-up (post-T9)

After all 8 commits ship and the gate holds (T9 step 1+2 both PASS):

1. Write `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/friends_server_public_chat_followup_close.md`, mirroring `[[friends-server-slice7-close]]` format. Include: HEAD before/after; commit list `T1..T8` with one-line summaries; tags retired (`NAI-S6-D-PUBLIC-CHAT-DEFERRED`); zero tags opened; race-clean + smoke-pack timings; any plan deviations encountered; any reviewer fix-ups.
2. Add a one-line entry to `MEMORY.md`.
3. **Resulting state:** all friends-server bridge work is at a stable rest state. All deviation tags are either retired or permanent; no further conditional retirements remain.
