package friends

import (
	"database/sql"
	"io"
	"log/slog"
	"slices"
	"sync"
	"testing"
)

// noopLogger returns a *slog.Logger that discards all output.
// Mirrors modules/login/db_test.go:42-44.
func noopLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// newTestRepo returns a Repository backed by a fresh in-memory SQLite
// database. The DB is closed via t.Cleanup. profile defaults to "test".
func newTestRepo(t *testing.T) (*Repository, *sql.DB) {
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

func TestRepository_InitializeWorld_OverwritesExisting(t *testing.T) {
	r, _ := newTestRepo(t)
	r.InitializeWorld(7, 5)
	if !r.Register(7, 0xAAAA, 0, 0) {
		t.Fatalf("first Register: got false, want true")
	}
	r.InitializeWorld(7, 5)
	for i := uint64(1); i <= 5; i++ {
		if !r.Register(7, i, 0, 0) {
			t.Errorf("Register #%d after re-init: got false, want true", i)
		}
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

func TestRepository_AddFriend_Idempotent(t *testing.T) {
	r, _ := newTestRepo(t)
	ctx := t.Context()
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

func TestRepository_AddIgnore_Idempotent(t *testing.T) {
	r, _ := newTestRepo(t)
	ctx := t.Context()
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
	r, _ := newTestRepo(t)
	ctx := t.Context()
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
	r, _ := newTestRepo(t)
	ctx := t.Context()
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
	r, _ := newTestRepo(t)
	ctx := t.Context()
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
	r, _ := newTestRepo(t)
	ctx := t.Context()
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

// TestRepository_IsVisibleTo_IgnoredViewerHidden pins H15: if other has
// ignored viewer, other is hidden even with ON. TS FriendServerRepository.ts:340-342.
func TestRepository_IsVisibleTo_IgnoredViewerHidden(t *testing.T) {
	r, _ := newTestRepo(t)
	ctx := t.Context()
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
	r, _ := newTestRepo(t)
	ctx := t.Context()
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
	r, _ := newTestRepo(t)
	ctx := t.Context()
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

func TestRepository_AddFriend_Idempotent_SQL(t *testing.T) {
	r, db := newTestRepo(t)
	const owner = uint64(0xAAAA)
	const target = uint64(0xBBBB)

	for i := 0; i < 3; i++ {
		if err := r.AddFriend(t.Context(), owner, target); err != nil {
			t.Fatalf("AddFriend iter %d: %v", i, err)
		}
	}

	var count int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM friendlist WHERE profile=? AND owner_username37=?`,
		"test", int64(owner),
	).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Errorf("rows after 3 AddFriend calls: got %d, want 1", count)
	}
}

func TestRepository_AddFriend_RespectsProfileBoundary(t *testing.T) {
	db := createTestDB(t)
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
	r, _ := newTestRepo(t)
	const target = uint64(0xBBBB)
	owners := []uint64{0xA1, 0xA2, 0xA3, 0xA4}

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
	r, _ := newTestRepo(t)
	const owner = uint64(0xAAAA)
	targets := []uint64{0xB1, 0xB2, 0xB3}
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
	r, _ := newTestRepo(t)
	r.InitializeWorld(1, 100)
	r.Register(1, 100, 1, 0) // privateChat FRIENDS
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
	r, _ := newTestRepo(t)
	r.InitializeWorld(1, 100)
	r.Register(1, 100, 1, 0)
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
