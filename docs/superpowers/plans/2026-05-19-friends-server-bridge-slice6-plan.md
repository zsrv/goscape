# Friends-server bridge slice 6 — chat logging (`private_chat` persistence) implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Persist every `PrivateMessage` RPC to a new SQLite `private_chat` table before the registry routes the delivery to the recipient's stream, mirroring TS `FriendServer.ts:266-285` exactly. Retire `NAI-S1-D-PM-NO-PERSISTENCE`.

**Architecture:** Add migration `000002_private_chat.up.sql` and `Repository.LogPrivateMessage(ctx, from, to, coord, message) error` (slice-3-style `ExecContext`). The `handler.PrivateMessage` method gains an insert-before-send gate: insert error → `codes.Internal`, no delivery. No proto change, no world-side production change. One on-disk e2e test pins persistence end-to-end.

**Tech Stack:** Go 1.x, `modernc.org/sqlite`, `golang-migrate/migrate/v4`, `embed.FS` for migrations, `database/sql`, `google.golang.org/grpc`, `pkg/friendspb`.

**Predecessor:** slice 5b close commit `d6cbfe0e`; spec commit `ade5a007` at `docs/superpowers/specs/2026-05-19-friends-server-bridge-slice6-design.md`.

**Gate (must hold post-slice):** `-race` clean across all 30 packages; smoke-pack 12 OK / 0 ERR / 0 SKIP.

---

## Discipline notes — read every implementer subagent prompt

These carry forward from prior slices (per slice-5b close):

1. **NEVER `git checkout` / `git restore` tracked files.** Slice 5a T0 lost user's in-flight Makefile edits this way. Escape hatch is BLOCKED-status reporting, not destructive git.
2. **Test helper files use `_test.go` suffix.** Plan-writer must double-check; same for any helper introduced inside a task.
3. **Pre-flight env (every shell session):**
   ```bash
   unset GOROOT
   export PATH="/home/owner/go/current/bin:$PATH"
   ```
4. **Go-test prefix (required by global CLAUDE.md):**
   ```bash
   GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/friends/... -count=1
   ```
5. **`git commit --no-gpg-sign`** — required by global CLAUDE.md.
6. **Before every commit:** run `git status` to confirm the staged set, then `git show --stat HEAD` after — concurrent shell edits from the user can sneak unrelated changes into the index. Recover with `git reset --mixed HEAD~1` (never amend).
7. **Stale-IDE-LSP / `go list` "No packages found" diagnostics are environmental** — ignore them.
8. **`go test -race` always.** Slice 3 set the precedent: mixing in-memory presence + SQL writes is race-sensitive.

---

## File structure

| File | Action | Responsibility |
|---|---|---|
| `modules/friends/migrations/000002_private_chat.up.sql` | **create** | DDL for `private_chat` table + 2 composite indexes |
| `modules/friends/db_test.go` | **modify** | Extend `TestOpenDB_AppliesMigrations` to include `private_chat` |
| `modules/friends/repository.go` | **modify** | Add `LogPrivateMessage(ctx, from, to, coord, message) error` |
| `modules/friends/repository_test.go` | **modify** | 4 new `TestRepository_LogPrivateMessage_*` tests |
| `modules/friends/handler.go` | **modify** | `PrivateMessage`: insert-then-send; use `ctx`; retire tag in doc-comment |
| `modules/friends/handler_test.go` | **modify** | 2 new tests (`PersistsBeforeSending`, `InsertErrorBlocksSend`); existing tests stay green |
| `modules/world/friends_smoke_test.go` | **modify** | Add `TestFriendsClient_E2E_PrivateMessagePersistsRow` |

LOC estimate: ~150 added, ~10 deleted.

---

## Task list

- T1: Add `private_chat` migration + extend db migration test
- T2: Add `Repository.LogPrivateMessage` + 4 SQL-concern tests (TDD)
- T3: Update `handler.PrivateMessage` to insert-then-send + 2 handler tests
- T4: World-side e2e persistence test
- T5: Verification gate (`go test -race`, smoke-pack)

Each task ends in one commit.

---

## Task 1: `private_chat` migration + db test extension

**Files:**
- Create: `modules/friends/migrations/000002_private_chat.up.sql`
- Modify: `modules/friends/db_test.go` (extend `wantTables` in `TestOpenDB_AppliesMigrations`)

**Outcome:** Migration applies on every `openDB`; `TestOpenDB_AppliesMigrations` asserts the new table exists.

### Steps

- [ ] **Step 1.1: Write the failing test update.**

Edit `modules/friends/db_test.go` line 26 — extend the slice:

```go
	wantTables := []string{"friendlist", "ignorelist", "private_chat"}
```

- [ ] **Step 1.2: Run the test to verify it fails.**

```bash
unset GOROOT && export PATH="/home/owner/go/current/bin:$PATH"
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/friends/ -run TestOpenDB_AppliesMigrations -count=1 -v
```

Expected: FAIL with `table "private_chat" missing: sql: no rows in result set`.

- [ ] **Step 1.3: Create the migration file.**

Create `modules/friends/migrations/000002_private_chat.up.sql`:

```sql
CREATE TABLE private_chat (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    profile TEXT NOT NULL,
    from_username37 INTEGER NOT NULL,
    to_username37 INTEGER NOT NULL,
    coord INTEGER NOT NULL,
    message TEXT NOT NULL,
    created TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_private_chat_to
    ON private_chat (profile, to_username37, created);

CREATE INDEX idx_private_chat_from
    ON private_chat (profile, from_username37, created);
```

- [ ] **Step 1.4: Run the test to verify it passes.**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/friends/ -run TestOpenDB_AppliesMigrations -count=1 -v
```

Expected: PASS.

- [ ] **Step 1.5: Run the full friends package to confirm no regressions.**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/friends/... -count=1
```

Expected: all tests PASS.

- [ ] **Step 1.6: Commit.**

Run `git status` first to confirm only these two files are staged:

```bash
git status
git add modules/friends/migrations/000002_private_chat.up.sql modules/friends/db_test.go
git status # confirm staged set
git commit --no-gpg-sign -m "$(cat <<'EOF'
friends: slice 6 T1 — private_chat migration

New SQLite table for slice 6 chat-logging. Schema mirrors TS
private_chat (Engine-TS prisma/singleworld/.../migration.sql:120-129)
with the established account_id → username37 divergence
(NAI-S3-D-USERNAME37-NOT-ACCOUNTID) and the slice-3 column-shape
convention (profile, created TEXT DEFAULT CURRENT_TIMESTAMP).

Two composite indexes anticipate read-side admin tooling
(profile, to_username37, created) and (profile, from_username37,
created). Append-only; no retention pruning (NAI-S6-D-NO-RETENTION
will be opened in T3 alongside the handler change).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
git show --stat HEAD  # confirm exactly the 2 expected files landed
```

---

## Task 2: `Repository.LogPrivateMessage` + 4 SQL-concern tests (TDD)

**Files:**
- Modify: `modules/friends/repository.go` (append method after the existing `Delete*` methods)
- Modify: `modules/friends/repository_test.go` (append 4 tests)

**Outcome:** `LogPrivateMessage` writes one row per call under `r.profile`; tests cover the four behaviors enumerated in spec §6.

### Steps

- [ ] **Step 2.1: Write the four failing tests.**

Append to `modules/friends/repository_test.go`:

```go
func TestRepository_LogPrivateMessage_PersistsRow(t *testing.T) {
	r, db := newTestRepo(t)
	ctx := t.Context()
	if err := r.LogPrivateMessage(ctx, 1111, 2222, 12345, "hi"); err != nil {
		t.Fatalf("LogPrivateMessage: %v", err)
	}
	var n int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM private_chat WHERE profile = 'test'`).Scan(&n); err != nil {
		t.Fatalf("COUNT query: %v", err)
	}
	if n != 1 {
		t.Fatalf("row count = %d, want 1", n)
	}
	var from, to int64
	var coord int32
	var msg string
	if err := db.QueryRowContext(ctx,
		`SELECT from_username37, to_username37, coord, message FROM private_chat`).
		Scan(&from, &to, &coord, &msg); err != nil {
		t.Fatalf("SELECT row: %v", err)
	}
	if from != 1111 {
		t.Errorf("from_username37 = %d, want 1111", from)
	}
	if to != 2222 {
		t.Errorf("to_username37 = %d, want 2222", to)
	}
	if coord != 12345 {
		t.Errorf("coord = %d, want 12345", coord)
	}
	if msg != "hi" {
		t.Errorf("message = %q, want %q", msg, "hi")
	}
}

func TestRepository_LogPrivateMessage_AppendOnly(t *testing.T) {
	r, db := newTestRepo(t)
	ctx := t.Context()
	if err := r.LogPrivateMessage(ctx, 1111, 2222, 0, "first"); err != nil {
		t.Fatalf("first: %v", err)
	}
	if err := r.LogPrivateMessage(ctx, 1111, 2222, 0, "second"); err != nil {
		t.Fatalf("second: %v", err)
	}
	var n int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM private_chat`).Scan(&n); err != nil {
		t.Fatalf("COUNT: %v", err)
	}
	if n != 2 {
		t.Errorf("row count = %d, want 2 (append-only, no dedupe)", n)
	}
}

func TestRepository_LogPrivateMessage_RespectsProfile(t *testing.T) {
	r, db := newTestRepo(t) // profile = "test"
	r2 := NewRepository(db, "other")
	ctx := t.Context()
	if err := r.LogPrivateMessage(ctx, 1, 2, 0, "from default"); err != nil {
		t.Fatalf("r: %v", err)
	}
	if err := r2.LogPrivateMessage(ctx, 1, 2, 0, "from other"); err != nil {
		t.Fatalf("r2: %v", err)
	}
	rows, err := db.QueryContext(ctx,
		`SELECT profile, message FROM private_chat ORDER BY id`)
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

func TestRepository_LogPrivateMessage_EmptyMessageAllowed(t *testing.T) {
	r, db := newTestRepo(t)
	ctx := t.Context()
	if err := r.LogPrivateMessage(ctx, 1, 2, 0, ""); err != nil {
		t.Fatalf("LogPrivateMessage(empty): %v", err)
	}
	var n int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM private_chat WHERE message = ''`).Scan(&n); err != nil {
		t.Fatalf("COUNT: %v", err)
	}
	if n != 1 {
		t.Errorf("row count = %d, want 1 (empty message allowed, no server-side validation)", n)
	}
}
```

Note: `newTestRepo(t)` already returns `(*Repository, *sql.DB)` per `repository_test.go:20`. The existing import list at the top of `repository_test.go` already includes `slices` and `sync` and `testing` — no new imports needed.

The `slices` import (`repository_test.go:7`) and `sync` import (`repository_test.go:8`) may flag as unused if the rest of the file changes. Verify they're still used by `TestRepository_GetFriends_Sorted` and any other extant tests before deleting — only delete imports if `go vet` or `go build` flags them.

- [ ] **Step 2.2: Run the tests to verify they fail with the expected error.**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/friends/ -run 'TestRepository_LogPrivateMessage' -count=1 -v
```

Expected: FAIL with `r.LogPrivateMessage undefined (type *Repository has no field or method LogPrivateMessage)`.

- [ ] **Step 2.3: Implement `Repository.LogPrivateMessage`.**

Append to `modules/friends/repository.go` (after the existing `DeleteIgnore` method, before `GetFollowers` / `IsVisibleTo` / `IsVisibleToMany` if those come after; verify position by reading the file). Suggested position: as the last method in the file, after `IsVisibleToMany`.

```go
// LogPrivateMessage appends one row to private_chat under r.profile.
// Mirrors TS FriendServer.ts:273-283 — append-only, no dedupe, no
// validation. Insert is the synchronous gate for PrivateMessage
// delivery: a failure here returns an error to the handler which
// surfaces codes.Internal to the caller, matching the TS thrown-
// await pattern.
func (r *Repository) LogPrivateMessage(ctx context.Context, from, to uint64, coord int32, message string) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO private_chat (profile, from_username37, to_username37, coord, message)
		 VALUES (?, ?, ?, ?, ?)`,
		r.profile, int64(from), int64(to), coord, message,
	)
	if err != nil {
		return fmt.Errorf("LogPrivateMessage: %w", err)
	}
	return nil
}
```

The existing imports at the top of `repository.go` already cover `context` (line 8) and `fmt` (line 10) — no new imports.

- [ ] **Step 2.4: Run the four tests to verify they pass.**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/friends/ -run 'TestRepository_LogPrivateMessage' -count=1 -v
```

Expected: 4 PASS.

- [ ] **Step 2.5: Run the full friends package to confirm no regressions.**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/friends/... -count=1
```

Expected: all tests PASS.

- [ ] **Step 2.6: Commit.**

```bash
git status
git add modules/friends/repository.go modules/friends/repository_test.go
git status
git commit --no-gpg-sign -m "$(cat <<'EOF'
friends: slice 6 T2 — Repository.LogPrivateMessage

Add the single SQL-concern write method for private_chat. Mirrors
slice-3 method shape: ctx-aware, returns error, scopes by r.profile.
Four tests cover persistence, append-only behavior, profile scoping
across two Repository instances on the same *sql.DB, and TS-faithful
empty-message tolerance.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
git show --stat HEAD
```

---

## Task 3: `handler.PrivateMessage` insert-then-send + 2 handler tests

**Files:**
- Modify: `modules/friends/handler.go` (`PrivateMessage`, lines 146-173 today)
- Modify: `modules/friends/handler_test.go` (append 2 new tests; existing tests stay)

**Outcome:** Insert succeeds → delivery; insert fails → `codes.Internal`, no delivery. Tag `NAI-S1-D-PM-NO-PERSISTENCE` retired from handler.go doc-comment.

### Steps

- [ ] **Step 3.1: Write the two failing tests.**

Append to `modules/friends/handler_test.go` (after `TestPrivateMessage_CrossWorld` at line 677):

```go
// TestHandler_PrivateMessage_PersistsBeforeSending pins slice 6's
// insert-then-send ordering: the handler writes to private_chat
// before pushing PrivateMessageDelivery to the recipient's stream.
// Mirrors TS FriendServer.ts:273-285.
func TestHandler_PrivateMessage_PersistsBeforeSending(t *testing.T) {
	r, db := newTestRepo(t)
	log := noopLogger()
	cfg := Config{NodeProfile: "main", WorldPlayerLimit: 100}
	h := &handler{repo: r, subs: newSubscriptions(log), cfg: cfg, log: log}
	r.InitializeWorld(1, 100)
	r.Register(1, 200, 0, 0) // recipient online

	stream := newTestStream(t)
	errc := make(chan error, 1)
	go func() {
		errc <- h.SubscribeUpdates(&friendspb.SubscribeUpdatesRequest{WorldId: 1, Username37: 200}, stream)
	}()
	t.Cleanup(func() {
		stream.cancel()
		<-errc
	})
	stream.recvWithin(t, 2*time.Second) // empty friendlist snapshot
	stream.recvWithin(t, 2*time.Second) // empty ignorelist snapshot

	if _, err := h.PrivateMessage(t.Context(), &friendspb.PrivateMessageRequest{
		WorldId:          1,
		Username37:       100,
		TargetUsername37: 200,
		StaffLvl:         0,
		PmId:             0xCAFEBABE,
		Chat:             "hi",
		Coord:            12345,
	}); err != nil {
		t.Fatalf("PrivateMessage: %v", err)
	}

	// Persistence
	var from, to int64
	var coord int32
	var msg string
	if err := db.QueryRowContext(t.Context(),
		`SELECT from_username37, to_username37, coord, message FROM private_chat`).
		Scan(&from, &to, &coord, &msg); err != nil {
		t.Fatalf("SELECT private_chat: %v", err)
	}
	if from != 100 || to != 200 || coord != 12345 || msg != "hi" {
		t.Errorf("row = (%d, %d, %d, %q), want (100, 200, 12345, %q)",
			from, to, coord, msg, "hi")
	}

	// Delivery
	u := stream.recvWithin(t, 2*time.Second)
	pm, ok := u.Update.(*friendspb.FriendsUpdate_PrivateMessage)
	if !ok {
		t.Fatalf("update = %T, want FriendsUpdate_PrivateMessage", u.Update)
	}
	if pm.PrivateMessage.PmId != 0xCAFEBABE {
		t.Errorf("PmId = %#x, want 0xCAFEBABE", pm.PrivateMessage.PmId)
	}
}

// TestHandler_PrivateMessage_InsertErrorBlocksSend pins that a SQL
// failure on private_chat insert returns codes.Internal AND does not
// deliver the PM. Forces the failure by closing the *sql.DB after
// the initial-snapshot reads complete. Mirrors the TS thrown-await
// pattern.
func TestHandler_PrivateMessage_InsertErrorBlocksSend(t *testing.T) {
	r, db := newTestRepo(t)
	log := noopLogger()
	cfg := Config{NodeProfile: "main", WorldPlayerLimit: 100}
	h := &handler{repo: r, subs: newSubscriptions(log), cfg: cfg, log: log}
	r.InitializeWorld(1, 100)
	r.Register(1, 200, 0, 0)

	stream := newTestStream(t)
	errc := make(chan error, 1)
	go func() {
		errc <- h.SubscribeUpdates(&friendspb.SubscribeUpdatesRequest{WorldId: 1, Username37: 200}, stream)
	}()
	t.Cleanup(func() {
		stream.cancel()
		<-errc
	})
	// Drain initial snapshots BEFORE closing db — those reads need the
	// DB to be open.
	stream.recvWithin(t, 2*time.Second)
	stream.recvWithin(t, 2*time.Second)

	// Force INSERT failure: close the underlying *sql.DB. The subscriber
	// goroutine is now in select{} waiting for new updates; it doesn't
	// query the DB until something arrives on its channel.
	if err := db.Close(); err != nil {
		t.Fatalf("db.Close: %v", err)
	}

	_, err := h.PrivateMessage(t.Context(), &friendspb.PrivateMessageRequest{
		WorldId:          1,
		Username37:       100,
		TargetUsername37: 200,
		PmId:             0xDEADBEEF,
		Chat:             "should not arrive",
	})
	if err == nil {
		t.Fatalf("PrivateMessage on closed DB: got nil error, want Internal")
	}
	if status.Code(err) != codes.Internal {
		t.Fatalf("PrivateMessage err code = %v, want %v", status.Code(err), codes.Internal)
	}

	// No delivery should land on the recipient stream. Short non-fatal
	// poll — recvWithin would t.Fatal on timeout.
	select {
	case u := <-stream.out:
		t.Fatalf("unexpected delivery after insert error: %T", u.Update)
	case <-time.After(200 * time.Millisecond):
		// expected: nothing arrives
	}
}
```

Both tests use the in-test handler construction pattern (`r, db := newTestRepo(t)` + inline struct), matching the existing `TestPrivateMessage_DeliveredToRecipient` / `TestPrivateMessage_CrossWorld` shape. Imports already cover `time`, `codes`, `status`, `friendspb`.

- [ ] **Step 3.2: Run the new tests to verify they fail.**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/friends/ -run 'TestHandler_PrivateMessage_(PersistsBeforeSending|InsertErrorBlocksSend)' -count=1 -v
```

Expected: both FAIL.
- `PersistsBeforeSending`: `no such table: private_chat` (if T1 wasn't run) OR `no rows in result set` (handler doesn't write yet).
- `InsertErrorBlocksSend`: `got nil error, want Internal` (current handler returns OK regardless).

- [ ] **Step 3.3: Modify `handler.PrivateMessage`.**

Replace `modules/friends/handler.go` lines 146-173 (the entire `PrivateMessage` method including its doc-comment) with:

```go
// PrivateMessage persists the PM to private_chat under r.profile,
// then routes a PrivateMessageDelivery to the target's open stream
// (if any). Mirrors TS FriendServer.ts:266-285 — insert-then-send,
// fail the RPC on insert error (matches the established
// codes.Internal posture of FriendlistAdd/Del/IgnorelistAdd/Del).
//
// req.Coord is server-side-persisted (and otherwise unused for
// routing). req.WorldId is unused for routing because the registry
// is keyed solely by username37; cross-world routing therefore falls
// out for free.
func (h *handler) PrivateMessage(ctx context.Context, req *friendspb.PrivateMessageRequest) (*emptypb.Empty, error) {
	h.ensureWorld(req.WorldId)
	if err := h.repo.LogPrivateMessage(ctx, req.Username37, req.TargetUsername37, req.Coord, req.Chat); err != nil {
		return nil, status.Errorf(codes.Internal, "LogPrivateMessage: %v", err)
	}
	h.subs.send(req.TargetUsername37, &friendspb.FriendsUpdate{
		Update: &friendspb.FriendsUpdate_PrivateMessage{
			PrivateMessage: &friendspb.PrivateMessageDelivery{
				FromUsername37: req.Username37,
				StaffLvl:       req.StaffLvl,
				PmId:           req.PmId,
				Chat:           req.Chat,
			},
		},
	})
	return &emptypb.Empty{}, nil
}
```

Changes vs slice-4b version:
- Signature: `_ context.Context` → `ctx context.Context`.
- New line: `if err := h.repo.LogPrivateMessage(...)` before the `subs.send` block.
- Doc-comment rewritten; the line `// NAI-S1-D-PM-NO-PERSISTENCE — slice 6 retires.` is removed.
- Imports already cover `context`, `codes`, `status`, `friendspb`, `emptypb` per `handler.go:3-12`.

- [ ] **Step 3.4: Run all friends tests including the new ones.**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/friends/... -count=1
```

Expected: ALL tests PASS, including:
- The 4 slice-6 `TestRepository_LogPrivateMessage_*` from T2.
- The 2 new `TestHandler_PrivateMessage_(PersistsBeforeSending|InsertErrorBlocksSend)`.
- The 3 slice-4b PM tests (`DeliveredToRecipient`, `NoSubscription`, `CrossWorld`) — these stay green because `newTestRepo(t)` gives them a working DB; INSERT succeeds invisibly.

If the slice-4b tests fail with a SQL error, that means the handler is somehow being called against a closed/unbuilt DB; verify T1's migration ran by re-checking `TestOpenDB_AppliesMigrations`.

- [ ] **Step 3.5: Commit.**

```bash
git status
git add modules/friends/handler.go modules/friends/handler_test.go
git status
git commit --no-gpg-sign -m "$(cat <<'EOF'
friends: slice 6 T3 — handler.PrivateMessage insert-then-send

Mirrors TS FriendServer.ts:266-285: persist to private_chat first,
then route PrivateMessageDelivery via subs.send. Insert failure
returns codes.Internal and blocks delivery, matching the established
posture of FriendlistAdd/Del/IgnorelistAdd/Del.

Two new tests pin both halves: TestHandler_PrivateMessage_Persists
BeforeSending asserts row + delivery; TestHandler_PrivateMessage_
InsertErrorBlocksSend forces a closed *sql.DB and asserts
codes.Internal + no delivery on the recipient stream.

Existing slice-4b PM tests (DeliveredToRecipient, NoSubscription,
CrossWorld) stay green — the new INSERT succeeds invisibly against
newTestRepo's working DB.

Retires NAI-S1-D-PM-NO-PERSISTENCE.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
git show --stat HEAD
```

---

## Task 4: World-side e2e persistence test

**Files:**
- Modify: `modules/world/friends_smoke_test.go` (append `TestFriendsClient_E2E_PrivateMessagePersistsRow`)

**Outcome:** End-to-end pin: a `client.PrivateMessage` call against a real in-process `friends.Friends` produces a row queryable via a second `*sql.DB` open against the same on-disk file.

### Steps

- [ ] **Step 4.1: Locate the existing slice-4b e2e and append after it.**

The existing test is `TestFriendsClient_E2E_PrivateMessageDelivery` starting at `modules/world/friends_smoke_test.go:261`. Append the new test after that function closes.

- [ ] **Step 4.2: Write the failing e2e test.**

Append to `modules/world/friends_smoke_test.go`:

```go
// TestFriendsClient_E2E_PrivateMessagePersistsRow pins slice 6:
// a client.PrivateMessage call against a real in-process
// friends.Friends produces a row in private_chat under r.profile,
// queryable via a second *sql.DB open against the same on-disk file.
//
// This is the persistence half of the slice-4b-and-slice-6 chain;
// delivery is pinned by TestFriendsClient_E2E_PrivateMessageDelivery.
func TestFriendsClient_E2E_PrivateMessagePersistsRow(t *testing.T) {
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
	client.PlayerLogin(ctx, &friendspb.PlayerLoginRequest{
		WorldId: 10, Username37: 2222, PrivateChat: 0, StaffLvl: 0,
	}, nil)
	client.PlayerLogin(ctx, &friendspb.PlayerLoginRequest{
		WorldId: 10, Username37: 1111, PrivateChat: 0, StaffLvl: 0,
	}, nil)

	client.PrivateMessage(ctx, &friendspb.PrivateMessageRequest{
		WorldId:          10,
		Username37:       1111,
		TargetUsername37: 2222,
		StaffLvl:         0,
		PmId:             0x1234,
		Chat:             "persisted",
		Coord:            42,
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
	var from, to int64
	var coord int32
	var msg string
	for time.Now().Before(deadline) {
		err := rdb.QueryRowContext(t.Context(),
			`SELECT from_username37, to_username37, coord, message
			 FROM private_chat
			 WHERE profile = 'main'
			 ORDER BY id DESC
			 LIMIT 1`).Scan(&from, &to, &coord, &msg)
		if err == nil {
			break
		}
		if !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("query private_chat: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}

	if from != 1111 || to != 2222 || coord != 42 || msg != "persisted" {
		t.Errorf("private_chat row = (%d, %d, %d, %q), want (1111, 2222, 42, %q)",
			from, to, coord, msg, "persisted")
	}
}
```

- [ ] **Step 4.3: Verify imports in `modules/world/friends_smoke_test.go`.**

The new test uses `sql` (`database/sql`), `errors`, `filepath`, `time`, `context`, `strconv`, `friends`, `friendspb`. The existing slice-4b e2e already imports `filepath`, `time`, `context`, `strconv`, `friends`, `friendspb`. Likely-missing imports: `database/sql`, `errors`, and the SQLite driver registration import.

The SQLite driver registration is `_ "modernc.org/sqlite"`. Add it (with the underscore for side-effect-only import) to the import block of `friends_smoke_test.go` if not already present.

Read the existing import block before editing:

```bash
sed -n '1,30p' modules/world/friends_smoke_test.go
```

If `_ "modernc.org/sqlite"` is missing, add it. If `database/sql` is missing, add it. If `errors` is missing, add it.

**Note:** the `friends` package already imports `_ "modernc.org/sqlite"` (per `modules/friends/db.go:12`), and the test binary will link against it via the `friends.New` call — but **explicitly importing the driver in the test file is required** for `sql.Open("sqlite", ...)` to find a registered driver in the test process. (Driver registration is `init()`-based and is reachable via any transitive import; the friends-server's init makes it available process-wide. So in practice the import may already be reachable; if `sql.Open` returns `sql: unknown driver "sqlite"`, add the explicit `_ "modernc.org/sqlite"` import.)

- [ ] **Step 4.4: Run the new test to verify it fails.**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/ -run TestFriendsClient_E2E_PrivateMessagePersistsRow -count=1 -v -timeout 60s
```

If T3 hasn't been committed yet, the test will fail on the assertion (row = `(0, 0, 0, "")`, want `(1111, 2222, 42, "persisted")`). Since T3 is committed by this point, expected: PASS.

If it fails on import (`sql: unknown driver "sqlite"`), add the import in Step 4.3 and re-run.

- [ ] **Step 4.5: Run the full world package.**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/ -count=1 -timeout 300s
```

Expected: all tests PASS.

- [ ] **Step 4.6: Commit.**

```bash
git status
git add modules/world/friends_smoke_test.go
git status
git commit --no-gpg-sign -m "$(cat <<'EOF'
friends: slice 6 T4 — world-side e2e PrivateMessage persistence

Boots in-process friends.Friends with an on-disk SQLite at
t.TempDir()/friends.db, sends one PrivateMessage via a real
FriendsClient, then opens a second *sql.DB against the same file
and confirms the row landed with the expected (from, to, coord,
message) tuple. Persistence half of the slice-4b/slice-6 chain;
delivery is already pinned by TestFriendsClient_E2E_
PrivateMessageDelivery.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
git show --stat HEAD
```

---

## Task 5: Verification gate

**Files:** none modified.

**Outcome:** Full `-race` suite + smoke-pack both green. Slice 6 is ready to close.

### Steps

- [ ] **Step 5.1: Full -race suite across all 30 packages.**

```bash
unset GOROOT && export PATH="/home/owner/go/current/bin:$PATH"
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./... -count=1 -timeout 600s
```

Expected: all 30 packages PASS. Record wall time for the slice-close memory.

- [ ] **Step 5.2: smoke-pack 12-stage byte-diff against real content.**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go run -trimpath ./cmd/goscape-cli smoke-pack --content-dir /home/owner/Code/github.com/LostCityRS/content
```

Expected: `12 OK / 0 ERR / 0 SKIP`. Record wall time.

- [ ] **Step 5.3: Confirm `NAI-S1-D-PM-NO-PERSISTENCE` no longer appears in any `.go` file.**

```bash
grep -rn "NAI-S1-D-PM-NO-PERSISTENCE" modules/ pkg/ cmd/ 2>/dev/null
```

Expected: zero results (it stays in `docs/superpowers/specs/*` and `docs/superpowers/plans/*` for historical reference, but should have been removed from `modules/friends/handler.go:159` in T3).

If T3 missed a `.go` site, add a follow-up commit that strips it.

- [ ] **Step 5.4: Record the gate results.**

These numbers go into the slice-6 close memory. No commit at this step — the close memory is written outside the repo (under `.claude/projects/`).

---

## Self-review (writing-plans skill)

**1. Spec coverage check:**

| Spec section | Plan task |
|---|---|
| §3 Schema (new table + 2 indexes) | T1 |
| §4 Repository surface (`LogPrivateMessage`) | T2 (impl + 4 tests) |
| §5 Handler change (insert-then-send, `_` → `ctx`, doc-comment) | T3 |
| §6 Repository tests (4 cases) | T2 |
| §7 db_test extension | T1 |
| §8 Handler tests (PersistsBeforeSending, InsertErrorBlocksSend) | T3 |
| §9 World-side e2e | T4 |
| §12 Tag retirement (`NAI-S1-D-PM-NO-PERSISTENCE`) | T3 + T5.3 audit |
| §12 New tags (`NAI-S6-D-NO-READ-RPC`, `NAI-S6-D-NO-RETENTION`, `NAI-S6-D-PUBLIC-CHAT-DEFERRED`) | These are slice-close memory entries; no code site mentions them unless explicitly added. T1's commit message mentions `NAI-S6-D-NO-RETENTION`. The other two are documented only in the spec and close memory. |
| §10 No ChatPersister abstraction | followed (direct repo call) |
| §11 Insert-then-send rationale | followed (T3) |
| §13 Risks R1-R5 | R1 mitigated in T3 step 3.1 test sequencing; R2 mitigated in T4 with 2s poll; R3 N/A; R4 covered by T3's two tests; R5 N/A (no new races). |

All covered.

**2. Placeholder scan:** No TBDs, no "implement later", no "similar to Task N". Code blocks are complete.

**3. Type consistency:**

- `LogPrivateMessage(ctx context.Context, from, to uint64, coord int32, message string) error` — used identically in T2 implementation, T3 handler call, T3 test, T4 test. ✓
- `private_chat` columns `from_username37 INTEGER`, `to_username37 INTEGER`, `coord INTEGER`, `message TEXT`, `profile TEXT`, `created TEXT` — referenced consistently in T1 (DDL), T2 tests (SELECT), T3 tests (SELECT), T4 test (SELECT). ✓
- `newTestRepo(t)` returns `(*Repository, *sql.DB)` — used consistently. ✓
- Handler test pattern (`r, db := newTestRepo(t); h := &handler{...}`) matches existing `TestPrivateMessage_DeliveredToRecipient` (line 575). ✓

---

## Slice close

After T5 passes, write the slice-close memory at `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/friends_server_slice6_close.md` mirroring the slice-5b close format (commit range, retired tags, opened tags, plan-execution deviations, test counts, gate results, links to predecessor slice memory). Add a one-line entry at the top of `MEMORY.md`.

After slice 6: slice 7 (`Player.session` per-login UUID) is the final slice. Post-slice-7, the `NAI-S6-D-PUBLIC-CHAT-DEFERRED` follow-up adds `public_chat` table + RPC + world-side hook.
