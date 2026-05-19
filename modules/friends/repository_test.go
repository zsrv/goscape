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
