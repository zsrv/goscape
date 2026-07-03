package filestream

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// countOpenFDs returns the number of open file descriptors for this
// process via /proc/self/fd. Linux-only (this project targets Linux only —
// see TestNewClosesEarlierHandlesOnPartialOpenFailure's skip guard); there
// is no portable stdlib equivalent.
func countOpenFDs(t *testing.T) int {
	t.Helper()
	entries, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		t.Fatalf("read /proc/self/fd: %v", err)
	}
	return len(entries)
}

// TestNewClosesEarlierHandlesOnPartialOpenFailure pins New's partial-open
// cleanup: when the dat file and some idx files have already been opened
// successfully but a LATER idx file fails to open, New must close every
// handle it had already opened before returning the error — not leak them.
//
// Forces the failure deterministically by pre-creating main_file_cache.idx2
// as a DIRECTORY: os.OpenFile(path, os.O_RDWR, ...) on a directory fails
// with EISDIR on Linux (verified: opening a directory O_RDWR errors "is a
// directory", unlike O_RDONLY which would succeed). dat, idx0, and idx1 are
// pre-seeded as ordinary empty files so New's OpenFile loop (not its
// os.WriteFile "create if missing" step — datPath already exists, so that
// step is skipped entirely) opens them successfully before reaching idx2,
// giving New's error branch real handles to close: dat, idx[0], idx[1].
// idx3/idx4 are never attempted because the loop returns on the first
// error.
//
// Linux-only: relies on /proc/self/fd to count descriptors and on Linux's
// EISDIR-on-O_RDWR-open behavior for directories.
func TestNewClosesEarlierHandlesOnPartialOpenFailure(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("relies on /proc/self/fd and Linux directory-open semantics")
	}

	dir := t.TempDir()
	datPath := filepath.Join(dir, "main_file_cache.dat")
	if err := os.WriteFile(datPath, nil, 0o666); err != nil {
		t.Fatalf("seed dat file: %v", err)
	}
	for i := 0; i <= 1; i++ {
		p := filepath.Join(dir, "main_file_cache.idx"+string(rune('0'+i)))
		if err := os.WriteFile(p, nil, 0o666); err != nil {
			t.Fatalf("seed idx%d file: %v", i, err)
		}
	}
	idx2Path := filepath.Join(dir, "main_file_cache.idx2")
	if err := os.Mkdir(idx2Path, 0o755); err != nil {
		t.Fatalf("seed idx2 directory: %v", err)
	}
	// idx3/idx4 deliberately left absent — the loop must never reach them.

	before := countOpenFDs(t)

	fs, err := New(dir, false, false)
	if err == nil {
		t.Fatal("want error opening idx2 (a directory as O_RDWR), got nil")
	}
	if fs != nil {
		t.Fatalf("want nil *FileStream on error, got %+v", fs)
	}

	after := countOpenFDs(t)
	if after != before {
		t.Errorf("open fd count leaked across failed New: before=%d after=%d "+
			"(dat/idx0/idx1 handles opened before idx2's failure were not all closed)",
			before, after)
	}
}
