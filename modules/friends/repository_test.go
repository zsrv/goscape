package friends

import (
	"io"
	"log/slog"
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
