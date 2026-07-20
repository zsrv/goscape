package account

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"testing"

	"github.com/zsrv/goscape/pkg/gamedb"
	"github.com/zsrv/goscape/pkg/gamedb/gamedbtest"
)

// noopLogger returns a *slog.Logger that discards all output. Mirrors
// modules/login/db_test.go's noopLogger.
func noopLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// openTestStore returns a Store over a fresh migrated DB: in-memory
// SQLite by default, or the env-configured Postgres (unique schema per
// test, dropped on cleanup via gamedbtest.OpenTestSchema) when
// GOSCAPE_TEST_POSTGRES_DSN is set — the whole module suite then runs
// against the real backend. Mirrors modules/login/db_test.go's
// createTestDB / modules/friends/repository_test.go's createTestDB
// exactly, so one setting drives all three suites. Every Store query
// goes through db.Rebind, and this dual-backend helper is what proves
// it (spec: "SQLite and Postgres both").
func openTestStore(t *testing.T) *Store {
	t.Helper()
	if dsn := os.Getenv("GOSCAPE_TEST_POSTGRES_DSN"); dsn != "" {
		return NewStore(gamedbtest.OpenTestSchema(t, dsn, t.Name(), noopLogger()))
	}

	var cfg gamedb.Config
	fs := flag.NewFlagSet("", flag.PanicOnError)
	cfg.RegisterFlagsAndApplyDefaults(fs)
	cfg.SQLite.DSN = fmt.Sprintf("file:%s?mode=memory&cache=shared", url.PathEscape(t.Name()))
	db, err := gamedb.Open(cfg, noopLogger())
	if err != nil {
		t.Fatalf("openTestStore: open: %v", err)
	}
	if err := db.Migrate(t.Context()); err != nil {
		db.Close()
		t.Fatalf("openTestStore: migrate: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return NewStore(db)
}

func TestStore_CreateAndFetchAccount(t *testing.T) {
	s := openTestStore(t)
	ctx := t.Context()

	id, err := s.CreateAccount(ctx, "  Player@Example.COM ", "$argon2id$fake")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	acct, err := s.AccountByEmail(ctx, "player@example.com")
	if err != nil {
		t.Fatalf("by email: %v", err)
	}
	if acct.ID != id || acct.Email != "player@example.com" || acct.EmailVerified ||
		acct.Status != StatusActive || acct.PasswordHash != "$argon2id$fake" {
		t.Fatalf("bad row: %+v", acct)
	}

	if _, err := s.CreateAccount(ctx, "PLAYER@example.com", "x"); !errors.Is(err, ErrEmailTaken) {
		t.Fatalf("duplicate email: got %v, want ErrEmailTaken", err)
	}
	if _, err := s.AccountByEmail(ctx, "ghost@example.com"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing account: got %v, want ErrNotFound", err)
	}

	if err := s.SetEmailVerified(ctx, id); err != nil {
		t.Fatalf("verify: %v", err)
	}
	if err := s.SetAccountStatus(ctx, id, StatusDisabled); err != nil {
		t.Fatalf("disable: %v", err)
	}
	if err := s.SetPasswordHash(ctx, id, "$argon2id$new"); err != nil {
		t.Fatalf("set hash: %v", err)
	}
	acct, err = s.AccountByID(ctx, id)
	if err != nil {
		t.Fatalf("by id: %v", err)
	}
	if !acct.EmailVerified || acct.Status != StatusDisabled || acct.PasswordHash != "$argon2id$new" {
		t.Fatalf("updates not applied: %+v", acct)
	}
}

func TestStore_Groups(t *testing.T) {
	s := openTestStore(t)
	ctx := t.Context()
	id, err := s.CreateAccount(ctx, "a@example.com", "x")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	ok, err := s.IsGroupMember(ctx, GroupManuallyApproved, id)
	if err != nil || ok {
		t.Fatalf("fresh account must not be approved: ok=%v err=%v", ok, err)
	}
	if err := s.AddGroupMember(ctx, GroupManuallyApproved, id, 0); err != nil {
		t.Fatalf("add: %v", err)
	}
	// Idempotent re-add.
	if err := s.AddGroupMember(ctx, GroupManuallyApproved, id, 0); err != nil {
		t.Fatalf("re-add must be idempotent: %v", err)
	}
	ok, err = s.IsGroupMember(ctx, GroupManuallyApproved, id)
	if err != nil || !ok {
		t.Fatalf("membership: ok=%v err=%v", ok, err)
	}
	if err := s.RemoveGroupMember(ctx, GroupManuallyApproved, id); err != nil {
		t.Fatalf("remove: %v", err)
	}
	ok, _ = s.IsGroupMember(ctx, GroupManuallyApproved, id)
	if ok {
		t.Fatal("membership must be gone after remove")
	}
	if err := s.AddGroupMember(ctx, "no_such_group", id, 0); err == nil {
		t.Fatal("unknown group must error")
	}
}

func TestStore_Audit(t *testing.T) {
	s := openTestStore(t)
	ctx := t.Context()
	id, _ := s.CreateAccount(ctx, "a@example.com", "x")

	if err := s.AppendAudit(ctx, 0, "account.register", "account:1", "self-service"); err != nil {
		t.Fatalf("append (system actor): %v", err)
	}
	if err := s.AppendAudit(ctx, id, "group.add", "account:1", "manually_approved"); err != nil {
		t.Fatalf("append: %v", err)
	}
	entries, err := s.RecentAudit(ctx, 10, "")
	if err != nil || len(entries) != 2 {
		t.Fatalf("recent: n=%d err=%v", len(entries), err)
	}
	// Newest first.
	if entries[0].Action != "group.add" || !entries[0].Actor.Valid || entries[0].Actor.Int64 != id {
		t.Fatalf("bad newest entry: %+v", entries[0])
	}
	if entries[1].Actor.Valid {
		t.Fatalf("system entry must have NULL actor: %+v", entries[1])
	}
	only, err := s.RecentAudit(ctx, 10, "account:1")
	if err != nil || len(only) != 2 {
		t.Fatalf("target filter: n=%d err=%v", len(only), err)
	}
}
