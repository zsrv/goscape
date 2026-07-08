package friends

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"

	"github.com/zsrv/goscape/pkg/gamedb"
	"github.com/zsrv/goscape/pkg/gamedb/gamedbtest"
	jstring "github.com/zsrv/goscape/pkg/util/jstring"
)

// noopLogger returns a *slog.Logger that discards all output. Mirrors
// modules/login/db_test.go's noopLogger.
func noopLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// createTestDB opens an isolated central test DB: in-memory sqlite by
// default; the env-configured Postgres (unique schema per test, dropped
// on cleanup) when GOSCAPE_TEST_POSTGRES_DSN is set — the whole module
// suite then runs against the real backend. Mirrors
// modules/login/db_test.go's createTestDB.
func createTestDB(t *testing.T) *gamedb.DB {
	t.Helper()
	if dsn := os.Getenv("GOSCAPE_TEST_POSTGRES_DSN"); dsn != "" {
		return gamedbtest.OpenTestSchema(t, dsn, t.Name(), noopLogger())
	}

	var cfg gamedb.Config
	fs := flag.NewFlagSet("", flag.PanicOnError)
	cfg.RegisterFlagsAndApplyDefaults(fs)
	cfg.SQLite.DSN = fmt.Sprintf("file:%s?mode=memory&cache=shared", url.PathEscape(t.Name()))
	db, err := gamedb.Open(cfg, noopLogger())
	if err != nil {
		t.Fatalf("createTestDB: open: %v", err)
	}
	if err := db.Migrate(t.Context()); err != nil {
		db.Close()
		t.Fatalf("createTestDB: migrate: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// testGamedbConfig returns a gamedb.Config with all flag defaults
// applied and the SQLite DSN pointed at a fresh on-disk file under
// t.TempDir(). Used where a test needs to gamedb.Open the SAME
// database twice (e.g. pre-migrating before handing the config to
// Friends.New, which opens its own pool in starting()) — an in-memory
// shared-cache DSN would also work, but a real file matches production
// shape more closely for shutdown/lifecycle tests.
func testGamedbConfig(t *testing.T) gamedb.Config {
	t.Helper()
	var cfg gamedb.Config
	fs := flag.NewFlagSet("", flag.PanicOnError)
	cfg.RegisterFlagsAndApplyDefaults(fs)
	cfg.SQLite.DSN = filepath.Join(t.TempDir(), "goscape.db")
	return cfg
}

// newTestRepo returns a Repository backed by a fresh in-memory central
// database. The DB is closed via t.Cleanup. profile defaults to "test".
func newTestRepo(t *testing.T) (*Repository, *gamedb.DB) {
	t.Helper()
	db := createTestDB(t)
	return NewRepository(db, "test"), db
}

func TestRepository_NewRepository_Empty(t *testing.T) {
	r, _ := newTestRepo(t)
	if got := r.GetWorld(0xDEADBEEF); got != 0 {
		t.Errorf("GetWorld on empty repo: got %d, want 0", got)
	}
}

func TestRepository_InitializeWorld_PreservesExistingCount(t *testing.T) {
	// TS FriendServerRepository.initializeWorld at FriendServerRepository.ts:48-54
	// early-returns `if (this.playersByWorld[world]) return;`. A duplicate
	// WORLD_CONNECT must NOT zero the player count — the audit row
	// friend-server-1 (2026-05-28 fresh-audit) closes the prior unconditional
	// reset behaviour. Limit=5, one prior register → count=1; after re-init
	// only 4 more (1..4) must fit, the 5th must be rejected.
	r, _ := newTestRepo(t)
	r.InitializeWorld(7, 5)
	if !r.Register(7, 0xAAAA, 0, 0) {
		t.Fatalf("first Register: got false, want true")
	}
	r.InitializeWorld(7, 5)
	for i := uint64(1); i <= 4; i++ {
		if !r.Register(7, i, 0, 0) {
			t.Errorf("Register #%d after re-init: got false, want true (count must preserve to 1, leaving 4 slots free)", i)
		}
	}
	if r.Register(7, 5, 0, 0) {
		t.Error("Register #5 after re-init: got true, want false (TS initializeWorld is a no-op on existing world; count must NOT reset, FriendServerRepository.ts:48-54)")
	}
}

func TestRepository_Register_RespectsPlayerLimit(t *testing.T) {
	r, _ := newTestRepo(t)
	r.InitializeWorld(1, 3)
	for i := uint64(1); i <= 3; i++ {
		if !r.Register(1, i, 0, 0) {
			t.Fatalf("Register %d: got false, want true", i)
		}
	}
	if r.Register(1, 99, 0, 0) {
		t.Errorf("Register beyond limit: got true, want false")
	}
}

func TestRepository_Register_DedupesAcrossWorlds(t *testing.T) {
	r, _ := newTestRepo(t)
	r.InitializeWorld(1, 10)
	r.InitializeWorld(2, 10)
	if !r.Register(1, 0xAAAA, 0, 0) {
		t.Fatalf("Register on world 1: got false, want true")
	}
	r.Unregister(0xAAAA)
	if !r.Register(2, 0xAAAA, 0, 0) {
		t.Fatalf("Register on world 2: got false, want true")
	}
	if got := r.GetWorld(0xAAAA); got != 2 {
		t.Errorf("GetWorld after move: got %d, want 2", got)
	}
}

func TestRepository_Register_UninitializedWorld_ReturnsFalse(t *testing.T) {
	r, _ := newTestRepo(t)
	if r.Register(42, 0xAAAA, 0, 0) {
		t.Errorf("Register on uninitialized world: got true, want false")
	}
}

func TestRepository_Unregister_UnknownPlayer_NoOp(t *testing.T) {
	r, _ := newTestRepo(t)
	r.InitializeWorld(1, 10)
	r.Unregister(0xDEADBEEF) // must not panic
}

func TestRepository_SetChatMode_Updates(t *testing.T) {
	r, _ := newTestRepo(t)
	r.InitializeWorld(1, 10)
	r.Register(1, 0xAAAA, 0, 0)
	r.SetChatMode(0xAAAA, 2)
	if got := r.GetChatMode(0xAAAA); got != 2 {
		t.Errorf("GetChatMode after Set: got %d, want 2", got)
	}
}

func TestRepository_GetChatMode_UnknownPlayer_ReturnsZero(t *testing.T) {
	r, _ := newTestRepo(t)
	if got := r.GetChatMode(0xDEADBEEF); got != 0 {
		t.Errorf("GetChatMode on unknown: got %d, want 0", got)
	}
}

func TestRepository_SetChatMode_UnknownPlayer_NoOp(t *testing.T) {
	r, _ := newTestRepo(t)
	r.SetChatMode(0xDEADBEEF, 2) // must not panic
}

// TestRepository_AddFriend_Idempotent pins that a duplicate AddFriend is
// a no-op (unique PK on (profile, account_id, friend_account_id)). Both
// endpoints must exist under the re-keyed TS 9aadcec4 schema (AddFriend
// now resolves owner and target against the central database).
func TestRepository_AddFriend_Idempotent(t *testing.T) {
	r, db := newTestRepo(t)
	ctx := t.Context()
	seedAccount(t, db, 0xAAAA)
	seedAccount(t, db, 0xBBBB)
	if err := r.AddFriend(ctx, 0xAAAA, 0xBBBB); err != nil {
		t.Fatalf("AddFriend: %v", err)
	}
	if err := r.AddFriend(ctx, 0xAAAA, 0xBBBB); err != nil {
		t.Fatalf("AddFriend (idempotent): %v", err)
	}
	got, err := r.GetFriends(ctx, 0xAAAA)
	if err != nil {
		t.Fatalf("GetFriends: %v", err)
	}
	if len(got) != 1 || got[0] != 0xBBBB {
		t.Errorf("GetFriends after double-add: got %v, want [0xBBBB]", got)
	}
}

// TestRepository_GetFriends_OrderedByCreated pins L44: GetFriends and
// GetIgnores must return entries in created-ascending order, matching TS
// FriendServerRepository.loadFriends/loadIgnores (orderBy('created', 'asc')).
// Rows are inserted with explicit created timestamps whose order differs from
// insertion order, so a missing ORDER BY (which would surface insertion/rowid
// order) is caught. Re-keyed for the TS 9aadcec4 account-id schema:
// friendlist rows reference friend_account_id (an account id), ignorelist
// rows still store the raw username string in `value`.
func TestRepository_GetFriends_OrderedByCreated(t *testing.T) {
	r, db := newTestRepo(t)
	ctx := t.Context()
	ownerID := seedAccount(t, db, 0xAAAA)
	id1111 := seedAccount(t, db, 0x1111)
	id2222 := seedAccount(t, db, 0x2222)
	id3333 := seedAccount(t, db, 0x3333)

	insertFriend := func(friendID int64, created string) {
		t.Helper()
		_, err := db.Exec(
			db.Rebind(`INSERT INTO friendlist (profile, account_id, friend_account_id, created) VALUES (?, ?, ?, ?)`),
			"test", ownerID, friendID, created,
		)
		if err != nil {
			t.Fatalf("insert friendlist: %v", err)
		}
	}
	insertIgnore := func(value, created string) {
		t.Helper()
		_, err := db.Exec(
			db.Rebind(`INSERT INTO ignorelist (profile, account_id, value, created) VALUES (?, ?, ?, ?)`),
			"test", ownerID, value, created,
		)
		if err != nil {
			t.Fatalf("insert ignorelist: %v", err)
		}
	}

	// Insertion order: 1111, 2222, 3333. created order: 2222, 3333, 1111.
	insertFriend(id1111, "2020-01-03 00:00:00")
	insertFriend(id2222, "2020-01-01 00:00:00")
	insertFriend(id3333, "2020-01-02 00:00:00")
	insertIgnore(jstring.FromBase37(0x1111), "2020-01-03 00:00:00")
	insertIgnore(jstring.FromBase37(0x2222), "2020-01-01 00:00:00")
	insertIgnore(jstring.FromBase37(0x3333), "2020-01-02 00:00:00")

	want := []uint64{0x2222, 0x3333, 0x1111}

	gotFriends, err := r.GetFriends(ctx, 0xAAAA)
	if err != nil {
		t.Fatalf("GetFriends: %v", err)
	}
	if !slices.Equal(gotFriends, want) {
		t.Errorf("GetFriends order: got %v, want %v", gotFriends, want)
	}

	gotIgnores, err := r.GetIgnores(ctx, 0xAAAA)
	if err != nil {
		t.Fatalf("GetIgnores: %v", err)
	}
	if !slices.Equal(gotIgnores, want) {
		t.Errorf("GetIgnores order: got %v, want %v", gotIgnores, want)
	}
}

func TestRepository_DeleteFriend_AbsentNoOp(t *testing.T) {
	r, _ := newTestRepo(t)
	ctx := t.Context()
	if err := r.DeleteFriend(ctx, 0xAAAA, 0xBBBB); err != nil {
		t.Fatalf("DeleteFriend: %v", err)
	}
	got, err := r.GetFriends(ctx, 0xAAAA)
	if err != nil {
		t.Fatalf("GetFriends: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("GetFriends after delete-missing: got %v, want empty", got)
	}
}

// TestRepository_AddIgnore_Idempotent pins that a duplicate AddIgnore is a
// no-op (ON CONFLICT DO NOTHING on the (profile, account_id, value) PK).
// Only the owner needs an account under TS 9aadcec4 semantics — AddIgnore
// never resolves the target.
func TestRepository_AddIgnore_Idempotent(t *testing.T) {
	r, db := newTestRepo(t)
	ctx := t.Context()
	seedAccount(t, db, 0xAAAA)
	if err := r.AddIgnore(ctx, 0xAAAA, 0xBBBB); err != nil {
		t.Fatalf("AddIgnore: %v", err)
	}
	if err := r.AddIgnore(ctx, 0xAAAA, 0xBBBB); err != nil {
		t.Fatalf("AddIgnore (idempotent): %v", err)
	}
	got, err := r.GetIgnores(ctx, 0xAAAA)
	if err != nil {
		t.Fatalf("GetIgnores: %v", err)
	}
	if len(got) != 1 || got[0] != 0xBBBB {
		t.Errorf("GetIgnores after double-add: got %v, want [0xBBBB]", got)
	}
}

func TestRepository_DeleteIgnore_Removes(t *testing.T) {
	r, db := newTestRepo(t)
	ctx := t.Context()
	seedAccount(t, db, 0xAAAA)
	if err := r.AddIgnore(ctx, 0xAAAA, 0xBBBB); err != nil {
		t.Fatalf("AddIgnore: %v", err)
	}
	if err := r.DeleteIgnore(ctx, 0xAAAA, 0xBBBB); err != nil {
		t.Fatalf("DeleteIgnore: %v", err)
	}
	got, err := r.GetIgnores(ctx, 0xAAAA)
	if err != nil {
		t.Fatalf("GetIgnores: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("GetIgnores after delete: got %v, want empty", got)
	}
}

func TestRepository_GetFollowers_TraversesCorrectly(t *testing.T) {
	r, db := newTestRepo(t)
	ctx := t.Context()
	for _, u := range []uint64{0xAAAA, 0xBBBB, 0xCCCC, 0xDDDD, 0xEEEE} {
		seedAccount(t, db, u)
	}
	if err := r.AddFriend(ctx, 0xAAAA, 0xBBBB); err != nil {
		t.Fatalf("AddFriend: %v", err)
	}
	if err := r.AddFriend(ctx, 0xCCCC, 0xBBBB); err != nil {
		t.Fatalf("AddFriend: %v", err)
	}
	if err := r.AddFriend(ctx, 0xDDDD, 0xEEEE); err != nil {
		t.Fatalf("AddFriend: %v", err)
	}
	got, err := r.GetFollowers(ctx, 0xBBBB)
	if err != nil {
		t.Fatalf("GetFollowers: %v", err)
	}
	slices.Sort(got)
	want := []uint64{0xAAAA, 0xCCCC}
	if !slices.Equal(got, want) {
		t.Errorf("GetFollowers(B): got %v, want %v", got, want)
	}
}

func TestRepository_GetFollowers_NoFollowers_Nil(t *testing.T) {
	r, _ := newTestRepo(t)
	got, err := r.GetFollowers(t.Context(), 0xBBBB)
	if err != nil {
		t.Fatalf("GetFollowers: %v", err)
	}
	if got != nil {
		t.Errorf("GetFollowers on no-followers: got %v, want nil", got)
	}
}

func TestRepository_IsVisibleTo_ChatModeOn_Always(t *testing.T) {
	r, _ := newTestRepo(t)
	r.InitializeWorld(1, 10)
	r.Register(1, 0xAAAA, 0, 0) // privateChat=ON
	visible, err := r.IsVisibleTo(t.Context(), 0xBBBB, 0xAAAA)
	if err != nil {
		t.Fatalf("IsVisibleTo: %v", err)
	}
	if !visible {
		t.Errorf("IsVisibleTo with ON: got false, want true")
	}
}

func TestRepository_IsVisibleTo_ChatModeOff_Never(t *testing.T) {
	r, db := newTestRepo(t)
	ctx := t.Context()
	seedAccount(t, db, 0xAAAA)
	seedAccount(t, db, 0xBBBB)
	r.InitializeWorld(1, 10)
	r.Register(1, 0xAAAA, 2, 0) // privateChat=OFF
	if err := r.AddFriend(ctx, 0xAAAA, 0xBBBB); err != nil {
		t.Fatalf("AddFriend: %v", err)
	}
	visible, err := r.IsVisibleTo(ctx, 0xBBBB, 0xAAAA)
	if err != nil {
		t.Fatalf("IsVisibleTo: %v", err)
	}
	if visible {
		t.Errorf("IsVisibleTo with OFF: got true, want false")
	}
}

func TestRepository_IsVisibleTo_ChatModeFriends_OnlyFriends(t *testing.T) {
	r, db := newTestRepo(t)
	ctx := t.Context()
	seedAccount(t, db, 0xAAAA)
	seedAccount(t, db, 0xBBBB)
	r.InitializeWorld(1, 10)
	r.Register(1, 0xAAAA, 1, 0) // privateChat=FRIENDS
	if err := r.AddFriend(ctx, 0xAAAA, 0xBBBB); err != nil {
		t.Fatalf("AddFriend: %v", err)
	}
	visible, err := r.IsVisibleTo(ctx, 0xBBBB, 0xAAAA)
	if err != nil {
		t.Fatalf("IsVisibleTo: %v", err)
	}
	if !visible {
		t.Errorf("IsVisibleTo for friend with FRIENDS: got false, want true")
	}
	visible, err = r.IsVisibleTo(ctx, 0xCCCC, 0xAAAA)
	if err != nil {
		t.Fatalf("IsVisibleTo: %v", err)
	}
	if visible {
		t.Errorf("IsVisibleTo for non-friend with FRIENDS: got true, want false")
	}
}

// TestRepository_IsVisibleTo_StaffBypassesOff pins H14: a staff viewer
// (staffLvl > 1) sees an OFF player. TS FriendServerRepository.ts:336-338.
func TestRepository_IsVisibleTo_StaffBypassesOff(t *testing.T) {
	r, _ := newTestRepo(t)
	r.InitializeWorld(1, 10)
	r.Register(1, 0xAAAA, 2, 0) // other: privateChat OFF
	r.Register(1, 0xBBBB, 0, 2) // viewer: staffLvl 2 (>1)
	visible, err := r.IsVisibleTo(t.Context(), 0xBBBB, 0xAAAA)
	if err != nil {
		t.Fatalf("IsVisibleTo: %v", err)
	}
	if !visible {
		t.Error("staff viewer must see an OFF player (H14)")
	}
	// staffLvl 1 is NOT staff (threshold is > 1).
	r.Register(1, 0xCCCC, 0, 1)
	visible, err = r.IsVisibleTo(t.Context(), 0xCCCC, 0xAAAA)
	if err != nil {
		t.Fatalf("IsVisibleTo: %v", err)
	}
	if visible {
		t.Error("staffLvl 1 viewer must not bypass OFF (threshold is staffLvl > 1)")
	}
}

// TestIsVisibleTo_StaffGateThreshold dual-pins the staff-gate threshold
// at rev-244's own TS pin: register() gates playerStaff membership with
// `staffLvl > 1 && !this.playerStaff.has(username37)`
// (FriendServerRepository.ts:83 @9aadcec4 — the audit's ONE in-memory
// delta vs rev-245.2's `staffLvl > 0` @3c16994c). A viewer registered
// with staffLvl 1 is NOT staff and must not bypass an OFF player;
// staffLvl 2 IS staff and bypasses. Regressing the goscape threshold to
// > 0 would flip the first assertion; regressing to > 2 would flip the
// second — both sides of the boundary are pinned on the SAME viewer via
// re-registration.
func TestIsVisibleTo_StaffGateThreshold(t *testing.T) {
	r, _ := newTestRepo(t)
	r.InitializeWorld(1, 10)
	r.Register(1, 0xAAAA, 2, 0) // other: privateChat OFF
	r.Register(1, 0xBBBB, 0, 1) // viewer: staffLvl 1 — NOT staff at 9aadcec4
	visible, err := r.IsVisibleTo(t.Context(), 0xBBBB, 0xAAAA)
	if err != nil {
		t.Fatalf("IsVisibleTo: %v", err)
	}
	if visible {
		t.Error("staffLvl 1 viewer must NOT bypass OFF (TS gates playerStaff at staffLvl > 1)")
	}

	r.Unregister(0xBBBB)
	r.Register(1, 0xBBBB, 0, 2) // re-register viewer: staffLvl 2 — staff
	visible, err = r.IsVisibleTo(t.Context(), 0xBBBB, 0xAAAA)
	if err != nil {
		t.Fatalf("IsVisibleTo: %v", err)
	}
	if !visible {
		t.Error("staffLvl 2 viewer must bypass OFF (TS gates playerStaff at staffLvl > 1)")
	}
}

// TestRepository_IsVisibleTo_IgnoredViewerHidden pins H15: if other has
// ignored viewer, other is hidden even with ON. TS FriendServerRepository.ts:340-342.
func TestRepository_IsVisibleTo_IgnoredViewerHidden(t *testing.T) {
	r, db := newTestRepo(t)
	ctx := t.Context()
	seedAccount(t, db, 0xAAAA)
	r.InitializeWorld(1, 10)
	r.Register(1, 0xAAAA, 0, 0) // other: privateChat ON
	r.Register(1, 0xBBBB, 0, 0) // viewer: normal
	if err := r.AddIgnore(ctx, 0xAAAA, 0xBBBB); err != nil {
		t.Fatalf("AddIgnore: %v", err)
	}
	visible, err := r.IsVisibleTo(ctx, 0xBBBB, 0xAAAA)
	if err != nil {
		t.Fatalf("IsVisibleTo: %v", err)
	}
	if visible {
		t.Error("ignored viewer must not see other even with ON (H15)")
	}
	// A non-ignored viewer still sees the ON player.
	r.Register(1, 0xCCCC, 0, 0)
	visible, err = r.IsVisibleTo(ctx, 0xCCCC, 0xAAAA)
	if err != nil {
		t.Fatalf("IsVisibleTo: %v", err)
	}
	if !visible {
		t.Error("non-ignored viewer must see ON player")
	}
}

// TestRepository_IsVisibleTo_StaffBypassesIgnore pins the TS ordering: the
// staff check (step 1) precedes the ignore check (step 2), so a staff viewer
// sees a player who has ignored them.
func TestRepository_IsVisibleTo_StaffBypassesIgnore(t *testing.T) {
	r, db := newTestRepo(t)
	ctx := t.Context()
	seedAccount(t, db, 0xAAAA)
	r.InitializeWorld(1, 10)
	r.Register(1, 0xAAAA, 0, 0) // other ON
	r.Register(1, 0xBBBB, 0, 2) // viewer staff
	if err := r.AddIgnore(ctx, 0xAAAA, 0xBBBB); err != nil {
		t.Fatalf("AddIgnore: %v", err)
	}
	visible, err := r.IsVisibleTo(ctx, 0xBBBB, 0xAAAA)
	if err != nil {
		t.Fatalf("IsVisibleTo: %v", err)
	}
	if !visible {
		t.Error("staff viewer must bypass ignore (TS checks staff before ignore)")
	}
}

// TestIsVisibleToMany_StaffAndIgnore pins the batched analogue of H14/H15.
func TestIsVisibleToMany_StaffAndIgnore(t *testing.T) {
	r, db := newTestRepo(t)
	ctx := t.Context()
	seedAccount(t, db, 100)
	r.InitializeWorld(1, 10)
	r.Register(1, 100, 0, 0) // other: ON
	r.Register(1, 200, 0, 2) // staff viewer
	r.Register(1, 300, 0, 0) // ignored viewer
	r.Register(1, 400, 0, 0) // normal viewer
	if err := r.AddIgnore(ctx, 100, 300); err != nil {
		t.Fatalf("AddIgnore: %v", err)
	}
	got, err := r.IsVisibleToMany(ctx, []uint64{200, 300, 400}, 100)
	if err != nil {
		t.Fatalf("IsVisibleToMany: %v", err)
	}
	if !got[200] {
		t.Error("staff viewer 200 must be visible-true")
	}
	if got[300] {
		t.Error("ignored viewer 300 must be visible-false")
	}
	if !got[400] {
		t.Error("normal viewer 400 must be visible-true (ON)")
	}
}

func TestRepository_IsVisibleTo_UnknownPlayer_NotVisible(t *testing.T) {
	r, _ := newTestRepo(t)
	visible, err := r.IsVisibleTo(t.Context(), 0xBBBB, 0xDEADBEEF)
	if err != nil {
		t.Fatalf("IsVisibleTo: %v", err)
	}
	if visible {
		t.Errorf("IsVisibleTo on unknown other: got true, want false")
	}
}

// TestRepository_AddFriend_Idempotent_SQL pins the raw-row idempotency
// invariant directly against the account-id-keyed friendlist table.
func TestRepository_AddFriend_Idempotent_SQL(t *testing.T) {
	r, db := newTestRepo(t)
	const owner = uint64(0xAAAA)
	const target = uint64(0xBBBB)
	ownerID := seedAccount(t, db, owner)
	seedAccount(t, db, target)

	for i := 0; i < 3; i++ {
		if err := r.AddFriend(t.Context(), owner, target); err != nil {
			t.Fatalf("AddFriend iter %d: %v", i, err)
		}
	}

	var count int
	if err := db.QueryRow(
		db.Rebind(`SELECT COUNT(*) FROM friendlist WHERE profile=? AND account_id=?`),
		"test", ownerID,
	).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Errorf("rows after 3 AddFriend calls: got %d, want 1", count)
	}
}

func TestRepository_AddFriend_RespectsProfileBoundary(t *testing.T) {
	db := createTestDB(t)
	seedAccount(t, db, 0xAAAA)
	seedAccount(t, db, 0xBBBB)
	rMain := NewRepository(db, "main")
	rAlt := NewRepository(db, "alt")

	const owner = uint64(0xAAAA)
	const target = uint64(0xBBBB)

	if err := rMain.AddFriend(t.Context(), owner, target); err != nil {
		t.Fatalf("rMain AddFriend: %v", err)
	}

	gotAlt, err := rAlt.GetFriends(t.Context(), owner)
	if err != nil {
		t.Fatalf("rAlt GetFriends: %v", err)
	}
	if len(gotAlt) != 0 {
		t.Errorf("rAlt GetFriends: got %v, want empty (profile boundary)", gotAlt)
	}

	gotMain, err := rMain.GetFriends(t.Context(), owner)
	if err != nil {
		t.Fatalf("rMain GetFriends: %v", err)
	}
	if len(gotMain) != 1 || gotMain[0] != target {
		t.Errorf("rMain GetFriends: got %v, want [%#x]", gotMain, target)
	}
}

func TestRepository_GetFollowers_FindsAllOwners(t *testing.T) {
	r, db := newTestRepo(t)
	const target = uint64(0xBBBB)
	owners := []uint64{0xA1, 0xA2, 0xA3, 0xA4}
	seedAccount(t, db, target)
	for _, o := range owners {
		seedAccount(t, db, o)
	}

	for _, o := range owners {
		if err := r.AddFriend(t.Context(), o, target); err != nil {
			t.Fatalf("AddFriend %#x: %v", o, err)
		}
	}

	got, err := r.GetFollowers(t.Context(), target)
	if err != nil {
		t.Fatalf("GetFollowers: %v", err)
	}
	if len(got) != len(owners) {
		t.Errorf("GetFollowers len: got %d (%v), want %d", len(got), got, len(owners))
	}
	gotSet := make(map[uint64]bool, len(got))
	for _, o := range got {
		gotSet[o] = true
	}
	for _, o := range owners {
		if !gotSet[o] {
			t.Errorf("GetFollowers missing owner %#x", o)
		}
	}
}

func TestRepository_GetFriends_OrderIsStable(t *testing.T) {
	r, db := newTestRepo(t)
	const owner = uint64(0xAAAA)
	targets := []uint64{0xB1, 0xB2, 0xB3}
	seedAccount(t, db, owner)
	for _, t37 := range targets {
		seedAccount(t, db, t37)
	}
	for _, t37 := range targets {
		if err := r.AddFriend(t.Context(), owner, t37); err != nil {
			t.Fatalf("AddFriend %#x: %v", t37, err)
		}
	}

	first, err := r.GetFriends(t.Context(), owner)
	if err != nil {
		t.Fatalf("GetFriends 1: %v", err)
	}
	for i := 0; i < 5; i++ {
		again, err := r.GetFriends(t.Context(), owner)
		if err != nil {
			t.Fatalf("GetFriends %d: %v", i, err)
		}
		if !slices.Equal(first, again) {
			t.Errorf("GetFriends iter %d: got %v, want %v (PK ordering)", i, again, first)
		}
	}
}

func TestRepository_Concurrent_RaceClean(t *testing.T) {
	r, _ := newTestRepo(t)
	r.InitializeWorld(1, 10000)
	ctx := t.Context()

	const goroutines = 16
	const iterations = 200

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := range goroutines {
		go func(id int) {
			defer wg.Done()
			base := uint64(id) * 1000
			for i := range iterations {
				u := base + uint64(i)
				r.Register(1, u, int32(i%3), 0)
				_ = r.AddFriend(ctx, u, u+1)
				r.SetChatMode(u, int32((i+1)%3))
				_, _ = r.GetFriends(ctx, u)
				_, _ = r.IsVisibleTo(ctx, u+1, u)
				_ = r.DeleteFriend(ctx, u, u+1)
				r.Unregister(u)
			}
		}(g)
	}
	wg.Wait()
}

func TestIsVisibleToMany_EmptyViewers(t *testing.T) {
	r, _ := newTestRepo(t)
	r.InitializeWorld(1, 100)
	r.Register(1, 100, 0, 0)
	got, err := r.IsVisibleToMany(t.Context(), nil, 100)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %v, want empty map", got)
	}
}

func TestIsVisibleToMany_OtherNotRegistered(t *testing.T) {
	r, _ := newTestRepo(t)
	got, err := r.IsVisibleToMany(t.Context(), []uint64{1, 2, 3}, 100)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	for _, v := range []uint64{1, 2, 3} {
		if got[v] {
			t.Errorf("viewer %d: got true, want false (other not registered)", v)
		}
	}
}

func TestIsVisibleToMany_ChatModeOnAllVisible(t *testing.T) {
	r, _ := newTestRepo(t)
	r.InitializeWorld(1, 100)
	r.Register(1, 100, 0, 0) // privateChat ON
	got, err := r.IsVisibleToMany(t.Context(), []uint64{1, 2, 3}, 100)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	for _, v := range []uint64{1, 2, 3} {
		if !got[v] {
			t.Errorf("viewer %d: got false, want true (mode ON)", v)
		}
	}
}

func TestIsVisibleToMany_ChatModeOffNoneVisible(t *testing.T) {
	r, _ := newTestRepo(t)
	r.InitializeWorld(1, 100)
	r.Register(1, 100, 2, 0) // privateChat OFF
	got, err := r.IsVisibleToMany(t.Context(), []uint64{1, 2, 3}, 100)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	for _, v := range []uint64{1, 2, 3} {
		if got[v] {
			t.Errorf("viewer %d: got true, want false (mode OFF)", v)
		}
	}
}

func TestIsVisibleToMany_ChatModeFriendsOnlyFriendsVisible(t *testing.T) {
	r, db := newTestRepo(t)
	r.InitializeWorld(1, 100)
	r.Register(1, 100, 1, 0) // privateChat FRIENDS
	seedAccount(t, db, 100)
	seedAccount(t, db, 2)
	seedAccount(t, db, 3)
	// 100 added 2 and 3 as friends; 1 is not a friend.
	if err := r.AddFriend(t.Context(), 100, 2); err != nil {
		t.Fatalf("AddFriend: %v", err)
	}
	if err := r.AddFriend(t.Context(), 100, 3); err != nil {
		t.Fatalf("AddFriend: %v", err)
	}
	got, err := r.IsVisibleToMany(t.Context(), []uint64{1, 2, 3}, 100)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got[1] {
		t.Errorf("viewer 1: got true, want false (not a friend)")
	}
	if !got[2] {
		t.Errorf("viewer 2: got false, want true (friend)")
	}
	if !got[3] {
		t.Errorf("viewer 3: got false, want true (friend)")
	}
}

func TestIsVisibleToMany_MatchesScalarIsVisibleTo(t *testing.T) {
	r, db := newTestRepo(t)
	r.InitializeWorld(1, 100)
	r.Register(1, 100, 1, 0)
	seedAccount(t, db, 100)
	seedAccount(t, db, 2)
	if err := r.AddFriend(t.Context(), 100, 2); err != nil {
		t.Fatalf("AddFriend: %v", err)
	}

	viewers := []uint64{1, 2, 3, 4, 5}
	batch, err := r.IsVisibleToMany(t.Context(), viewers, 100)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	for _, v := range viewers {
		want, err := r.IsVisibleTo(t.Context(), v, 100)
		if err != nil {
			t.Fatalf("IsVisibleTo: %v", err)
		}
		if batch[v] != want {
			t.Errorf("viewer %d: batch=%v scalar=%v", v, batch[v], want)
		}
	}
}

// (LogPrivateMessage/LogPublicMessage persistence tests retired — chat
// is Kafka-only, spec docs/superpowers/specs/2026-07-07-chat-kafka-only-design.md;
// both tables and the LogPublicMessage/LogPrivateMessage insert paths no
// longer exist. Resolution is covered by
// TestResolvePrivateMessageEndpoints_* below.)

// distinctUsername37s returns n values >= start, skipping any multiple of
// 37. jstring.FromBase37 treats every multiple of 37 (37, 74, 111, ...) as
// the single sentinel string "invalid_name" (JString.ts:42-44), so a naive
// contiguous range used as a stand-in for n distinct usernames silently
// collapses two or more loop iterations onto the same stored username —
// exactly the kind of test-data footgun that must be avoided once
// usernames round-trip through the real jstring codec (unlike the
// pre-port schema, which stored raw username37 ints with no encoding).
func distinctUsername37s(start uint64, n int) []uint64 {
	out := make([]uint64, 0, n)
	for v := start; len(out) < n; v++ {
		if v%37 != 0 {
			out = append(out, v)
		}
	}
	return out
}

// TestRepository_AddIgnore_EnforcesCap pins the 100-entry cap on the ignore
// list (TS addIgnore, FriendServerRepository.ts:266/268 @9aadcec4), counted
// across ALL profiles with no profile filter (TS quirk).
func TestRepository_AddIgnore_EnforcesCap(t *testing.T) {
	r, db := newTestRepo(t)
	ctx := t.Context()
	const owner = uint64(0x2222)
	seedAccount(t, db, owner)

	targets := distinctUsername37s(1, ignoreListLimit+1) // 100 + 1 over-cap probe
	for _, target := range targets[:ignoreListLimit] {
		if err := r.AddIgnore(ctx, owner, target); err != nil {
			t.Fatalf("AddIgnore #%d: %v", target, err)
		}
	}
	if err := r.AddIgnore(ctx, owner, targets[ignoreListLimit]); err != nil {
		t.Fatalf("AddIgnore at cap: %v", err)
	}
	ignores, err := r.GetIgnores(ctx, owner)
	if err != nil {
		t.Fatalf("GetIgnores: %v", err)
	}
	if len(ignores) != ignoreListLimit {
		t.Errorf("over-cap ignore add: got %d, want %d", len(ignores), ignoreListLimit)
	}
}

// --- TS 9aadcec4 re-key: new-behavior tests (account resolution, flat
// cap incl. members dual-pin, TS-exact private/public chat row shapes) ---

// seedAccount inserts an account whose username is the canonical
// FromBase37 form of username37, mirroring how the login module and TS
// both key accounts by username. Returns the account id.
func seedAccount(t *testing.T, db *gamedb.DB, username37 uint64) int64 {
	t.Helper()
	var id int64
	err := db.QueryRowContext(t.Context(),
		db.Rebind(`INSERT INTO account (username, password) VALUES (?, '') RETURNING id`),
		jstring.FromBase37(username37),
	).Scan(&id)
	if err != nil {
		t.Fatalf("seedAccount(%d): %v", username37, err)
	}
	return id
}

// seedMemberAccount is seedAccount with members=1. Used to dual-pin
// TestAddFriend_MemberAccount_StillCapped100: unlike rev-274's own LATER
// TS pin (account.members ? 200 : 100, FriendServerRepository.ts:229),
// 9aadcec4 has NOT landed the members-aware split yet — a member
// account gets no cap boost.
func seedMemberAccount(t *testing.T, db *gamedb.DB, username37 uint64) int64 {
	t.Helper()
	var id int64
	err := db.QueryRowContext(t.Context(),
		db.Rebind(`INSERT INTO account (username, password, members) VALUES (?, '', 1) RETURNING id`),
		jstring.FromBase37(username37),
	).Scan(&id)
	if err != nil {
		t.Fatalf("seedMemberAccount(%d): %v", username37, err)
	}
	return id
}

func TestAddFriend_MissingTarget_NoInsert(t *testing.T) {
	// TS FriendServerRepository.ts:226-228 @9aadcec4: `if (!account ||
	// !friendAccount) return`.
	r, db := newTestRepo(t)
	seedAccount(t, db, 1)
	if err := r.AddFriend(t.Context(), 1, 2); err != nil { // 2 has no account
		t.Fatalf("AddFriend: %v", err)
	}
	friends, err := r.GetFriends(t.Context(), 1)
	if err != nil {
		t.Fatalf("GetFriends: %v", err)
	}
	if len(friends) != 0 {
		t.Errorf("friend of missing account persisted: %v", friends)
	}
}

func TestAddFriend_MissingOwner_NoInsert(t *testing.T) {
	r, db := newTestRepo(t)
	seedAccount(t, db, 2)
	if err := r.AddFriend(t.Context(), 1, 2); err != nil {
		t.Fatalf("AddFriend: %v", err)
	}
	if friends, _ := r.GetFriends(t.Context(), 1); len(friends) != 0 {
		t.Errorf("friend row for missing owner persisted: %v", friends)
	}
}

func TestAddFriend_BothExist_Persists(t *testing.T) {
	// Dual-pin: presence AND absence (ts_asymmetry posture).
	r, db := newTestRepo(t)
	seedAccount(t, db, 1)
	seedAccount(t, db, 2)
	if err := r.AddFriend(t.Context(), 1, 2); err != nil {
		t.Fatalf("AddFriend: %v", err)
	}
	friends, err := r.GetFriends(t.Context(), 1)
	if err != nil {
		t.Fatalf("GetFriends: %v", err)
	}
	if len(friends) != 1 || friends[0] != 2 {
		t.Errorf("GetFriends: got %v, want [2]", friends)
	}
}

// TestAddFriend_EnforcesCap100 pins the audit's sanity gate (a): the flat
// literal 100 (FriendServerRepository.ts:233 @9aadcec4) applies to a
// non-member owner. See TestAddFriend_MemberAccount_StillCapped100 for
// the dual pin that a members account gets no boost at this pin.
func TestAddFriend_EnforcesCap100(t *testing.T) {
	r, db := newTestRepo(t)
	seedAccount(t, db, 1)
	targets := distinctUsername37s(2, friendListLimit+1) // 100 friends + 1 over-cap probe
	for _, target := range targets {
		seedAccount(t, db, target)
	}
	for _, target := range targets[:friendListLimit] {
		if err := r.AddFriend(t.Context(), 1, target); err != nil {
			t.Fatalf("AddFriend #%d: %v", target, err)
		}
	}
	if err := r.AddFriend(t.Context(), 1, targets[friendListLimit]); err != nil {
		t.Fatalf("AddFriend #101: %v", err)
	}
	friends, _ := r.GetFriends(t.Context(), 1)
	if len(friends) != friendListLimit {
		t.Errorf("cap: got %d friends, want %d", len(friends), friendListLimit)
	}
}

// TestAddFriend_MemberAccount_StillCapped100 dual-pins sanity gate (a):
// at TS 9aadcec4 there is NO members-aware cap split yet (unlike
// rev-274's own LATER pin, which reads account.members and grants a 200
// cap) — a members=1 owner is capped at the SAME flat 100 as a
// non-member. Regressing this to a 200 cap would silently reintroduce
// rev-274's later behavior at the wrong pin.
func TestAddFriend_MemberAccount_StillCapped100(t *testing.T) {
	r, db := newTestRepo(t)
	seedMemberAccount(t, db, 1)
	targets := distinctUsername37s(2, friendListLimit+1) // 100 + 1 over-cap probe
	for _, target := range targets {
		seedAccount(t, db, target)
	}
	for _, target := range targets[:friendListLimit] {
		if err := r.AddFriend(t.Context(), 1, target); err != nil {
			t.Fatalf("AddFriend #%d: %v", target, err)
		}
	}
	if err := r.AddFriend(t.Context(), 1, targets[friendListLimit]); err != nil {
		t.Fatalf("AddFriend at cap: %v", err)
	}
	friends, _ := r.GetFriends(t.Context(), 1)
	if len(friends) != friendListLimit {
		t.Errorf("members account cap: got %d friends, want %d (NOT 200 — no members-aware split at 9aadcec4)", len(friends), friendListLimit)
	}
}

func TestAddIgnore_NonexistentTarget_Succeeds(t *testing.T) {
	// TS never resolves the ignore target (FriendServerRepository.ts:249-294
	// @9aadcec4): ignoring a player who doesn't exist works. 1000 (not 999 —
	// a multiple of 37, which jstring.FromBase37 maps to the sentinel
	// "invalid_name") is used so the round-trip through the ignorelist.value
	// column is exact.
	r, db := newTestRepo(t)
	seedAccount(t, db, 1)
	if err := r.AddIgnore(t.Context(), 1, 1000); err != nil {
		t.Fatalf("AddIgnore(nonexistent target): %v", err)
	}
	ignores, err := r.GetIgnores(t.Context(), 1)
	if err != nil {
		t.Fatalf("GetIgnores: %v", err)
	}
	if len(ignores) != 1 || ignores[0] != 1000 {
		t.Errorf("GetIgnores: got %v, want [1000]", ignores)
	}
}

func TestAddIgnore_MissingOwner_NoOp(t *testing.T) {
	// TS resolves the OWNER and returns on miss (FriendServerRepository.ts:260-262
	// @9aadcec4).
	r, _ := newTestRepo(t)
	if err := r.AddIgnore(t.Context(), 1, 2); err != nil {
		t.Fatalf("AddIgnore: %v", err)
	}
	if ignores, _ := r.GetIgnores(t.Context(), 1); len(ignores) != 0 {
		t.Errorf("ignore row for missing owner persisted: %v", ignores)
	}
}

// TestResolvePrivateMessageEndpoints_MissingEndpoint pins the TS
// silent-drop precondition (FriendServer.ts:275-276 @9aadcec4): either
// endpoint unresolvable → errAccountMissing.
func TestResolvePrivateMessageEndpoints_MissingEndpoint(t *testing.T) {
	r, db := newTestRepo(t)

	seedAccount(t, db, 1) // only 'from' exists
	if _, _, err := r.ResolvePrivateMessageEndpoints(t.Context(), 1, 2); !errors.Is(err, errAccountMissing) {
		t.Fatalf("missing to: got %v, want errAccountMissing", err)
	}
	if _, _, err := r.ResolvePrivateMessageEndpoints(t.Context(), 3, 1); !errors.Is(err, errAccountMissing) {
		t.Fatalf("missing from: got %v, want errAccountMissing", err)
	}
}

// TestResolvePrivateMessageEndpoints_BothExist pins that both ids come
// back resolved (the ids TS used to key the private_chat row by).
func TestResolvePrivateMessageEndpoints_BothExist(t *testing.T) {
	r, db := newTestRepo(t)

	fromWant := seedAccount(t, db, 1)
	toWant := seedAccount(t, db, 2)
	from, to, err := r.ResolvePrivateMessageEndpoints(t.Context(), 1, 2)
	if err != nil {
		t.Fatalf("ResolvePrivateMessageEndpoints: %v", err)
	}
	if from != fromWant || to != toWant {
		t.Errorf("resolved = (%d, %d), want (%d, %d)", from, to, fromWant, toWant)
	}
}
