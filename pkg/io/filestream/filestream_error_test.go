package filestream

import (
	"path/filepath"
	"testing"
)

// TestNewErrorsOnUnreadableDir pins arch-29.7: New must return an error, not
// panic, when the underlying directory can't be created/opened. A NUL byte
// in a path component is rejected by the OS (invalid argument) regardless of
// permissions, giving a deterministic, platform-independent-enough failure
// for the initial os.MkdirAll call.
func TestNewErrorsOnUnreadableDir(t *testing.T) {
	_, err := New(filepath.Join(t.TempDir(), "no", "such", "deep", "\x00bad"), false, true)
	if err == nil {
		t.Fatal("want error, got nil")
	}
}
