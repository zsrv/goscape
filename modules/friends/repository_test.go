package friends

import (
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

func TestRepository_NewRepository_Empty(t *testing.T) {
	r := NewRepository()
	if got := r.GetWorld(0xDEADBEEF); got != 0 {
		t.Errorf("GetWorld on empty repo: got %d, want 0", got)
	}
}

func TestRepository_InitializeWorld_OverwritesExisting(t *testing.T) {
	r := NewRepository()
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
	r := NewRepository()
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
	r := NewRepository()
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
	r := NewRepository()
	if r.Register(42, 0xAAAA, 0, 0) {
		t.Errorf("Register on uninitialized world: got true, want false")
	}
}

func TestRepository_Unregister_UnknownPlayer_NoOp(t *testing.T) {
	r := NewRepository()
	r.InitializeWorld(1, 10)
	r.Unregister(0xDEADBEEF) // must not panic
}

func TestRepository_SetChatMode_Updates(t *testing.T) {
	r := NewRepository()
	r.InitializeWorld(1, 10)
	r.Register(1, 0xAAAA, 0, 0)
	r.SetChatMode(0xAAAA, 2)
	if got := r.GetChatMode(0xAAAA); got != 2 {
		t.Errorf("GetChatMode after Set: got %d, want 2", got)
	}
}

func TestRepository_GetChatMode_UnknownPlayer_ReturnsZero(t *testing.T) {
	r := NewRepository()
	if got := r.GetChatMode(0xDEADBEEF); got != 0 {
		t.Errorf("GetChatMode on unknown: got %d, want 0", got)
	}
}

func TestRepository_SetChatMode_UnknownPlayer_NoOp(t *testing.T) {
	r := NewRepository()
	r.SetChatMode(0xDEADBEEF, 2) // must not panic
}

func TestRepository_AddFriend_Idempotent(t *testing.T) {
	r := NewRepository()
	r.AddFriend(0xAAAA, 0xBBBB)
	r.AddFriend(0xAAAA, 0xBBBB)
	got := r.GetFriends(0xAAAA)
	if len(got) != 1 || got[0] != 0xBBBB {
		t.Errorf("GetFriends after double-add: got %v, want [0xBBBB]", got)
	}
}

func TestRepository_DeleteFriend_AbsentNoOp(t *testing.T) {
	r := NewRepository()
	r.DeleteFriend(0xAAAA, 0xBBBB) // must not panic
	if got := r.GetFriends(0xAAAA); len(got) != 0 {
		t.Errorf("GetFriends after delete-missing: got %v, want empty", got)
	}
}

func TestRepository_AddIgnore_Idempotent(t *testing.T) {
	r := NewRepository()
	r.AddIgnore(0xAAAA, 0xBBBB)
	r.AddIgnore(0xAAAA, 0xBBBB)
	got := r.GetIgnores(0xAAAA)
	if len(got) != 1 || got[0] != 0xBBBB {
		t.Errorf("GetIgnores after double-add: got %v, want [0xBBBB]", got)
	}
}

func TestRepository_DeleteIgnore_Removes(t *testing.T) {
	r := NewRepository()
	r.AddIgnore(0xAAAA, 0xBBBB)
	r.DeleteIgnore(0xAAAA, 0xBBBB)
	if got := r.GetIgnores(0xAAAA); len(got) != 0 {
		t.Errorf("GetIgnores after delete: got %v, want empty", got)
	}
}

func TestRepository_GetFollowers_TraversesCorrectly(t *testing.T) {
	r := NewRepository()
	r.AddFriend(0xAAAA, 0xBBBB)
	r.AddFriend(0xCCCC, 0xBBBB)
	r.AddFriend(0xDDDD, 0xEEEE)
	got := r.GetFollowers(0xBBBB)
	slices.Sort(got)
	want := []uint64{0xAAAA, 0xCCCC}
	if !slices.Equal(got, want) {
		t.Errorf("GetFollowers(B): got %v, want %v", got, want)
	}
}

func TestRepository_GetFollowers_NoFollowers_Nil(t *testing.T) {
	r := NewRepository()
	if got := r.GetFollowers(0xBBBB); got != nil {
		t.Errorf("GetFollowers on no-followers: got %v, want nil", got)
	}
}

func TestRepository_IsVisibleTo_ChatModeOn_Always(t *testing.T) {
	r := NewRepository()
	r.InitializeWorld(1, 10)
	r.Register(1, 0xAAAA, 0, 0) // privateChat=ON
	if !r.IsVisibleTo(0xBBBB, 0xAAAA) {
		t.Errorf("IsVisibleTo with ON: got false, want true")
	}
}

func TestRepository_IsVisibleTo_ChatModeOff_Never(t *testing.T) {
	r := NewRepository()
	r.InitializeWorld(1, 10)
	r.Register(1, 0xAAAA, 2, 0) // privateChat=OFF
	r.AddFriend(0xAAAA, 0xBBBB) // even friends don't see
	if r.IsVisibleTo(0xBBBB, 0xAAAA) {
		t.Errorf("IsVisibleTo with OFF: got true, want false")
	}
}

func TestRepository_IsVisibleTo_ChatModeFriends_OnlyFriends(t *testing.T) {
	r := NewRepository()
	r.InitializeWorld(1, 10)
	r.Register(1, 0xAAAA, 1, 0)  // privateChat=FRIENDS
	r.AddFriend(0xAAAA, 0xBBBB) // A friends B
	if !r.IsVisibleTo(0xBBBB, 0xAAAA) {
		t.Errorf("IsVisibleTo for friend with FRIENDS: got false, want true")
	}
	if r.IsVisibleTo(0xCCCC, 0xAAAA) {
		t.Errorf("IsVisibleTo for non-friend with FRIENDS: got true, want false")
	}
}

func TestRepository_IsVisibleTo_UnknownPlayer_NotVisible(t *testing.T) {
	r := NewRepository()
	if r.IsVisibleTo(0xBBBB, 0xDEADBEEF) {
		t.Errorf("IsVisibleTo on unknown other: got true, want false")
	}
}

func TestRepository_Concurrent_RaceClean(t *testing.T) {
	r := NewRepository()
	r.InitializeWorld(1, 10000)

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
				r.AddFriend(u, u+1)
				r.SetChatMode(u, int32((i+1)%3))
				_ = r.GetFriends(u)
				_ = r.IsVisibleTo(u+1, u)
				r.DeleteFriend(u, u+1)
				r.Unregister(u)
			}
		}(g)
	}
	wg.Wait()
}
