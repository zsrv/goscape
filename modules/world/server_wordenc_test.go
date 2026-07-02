package world

import (
	"path/filepath"
	"testing"
)

// TestNewServer_LoadsWordencFilter pins that NewServer populates s.wordenc from
// the raw wordenc jagfile. Rev-244: TS dropped the existence check and
// hardcoded "data/raw/wordenc" — NewServer now fails when the jag is absent.
// Test skips when the Server274-ref reference cache (needed for NewServer
// loads) is unavailable.
//
// Uses the Server274-ref pack (LoadComponentTypes expects the 274
// component layout — swappable/activeOverColour; ref274CacheDir is defined
// in testdata_path_test.go).
//
// encfilter.Load() reads data/raw/wordenc relative to the working directory;
// t.Chdir changes to the repo root so the committed data/raw/wordenc is reachable.
//
// TS ref: Engine-TS/src/cache/wordenc/WordEnc.ts:35-37 (static WordEnc.load).
func TestNewServer_LoadsWordencFilter(t *testing.T) {
	cachePath := ref274CacheDir(t)
	// encfilter.Load() resolves data/raw/wordenc relative to cwd. Switch to the
	// repo root so the committed data/raw/wordenc jagfile is reachable.
	repoRoot := filepath.Join("..", "..")
	absRoot, err := filepath.Abs(repoRoot)
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	t.Chdir(absRoot)
	cfg := Config{
		CachePath:        cachePath,
		TCPListenNetwork: "tcp",
		TCPListenAddress: "127.0.0.1",
		TCPListenPort:    0, // OS picks a free port
	}
	s, err := NewServer(cfg, nil, nil, discardLogger(), nil)
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}
	// arch-29.8: NewServer no longer binds the TCP listener (see
	// TestNewServerDoesNotBind in server_lifecycle_test.go) — nothing to
	// close here.
	if s.wordenc == nil {
		t.Fatal("NewServer must populate s.wordenc; got nil")
	}
	// Sanity-check: an unfiltered string passes through unchanged.
	if got := s.wordenc.Filter("plain text"); got != "plain text" {
		t.Errorf("wordenc.Filter(\"plain text\") = %q; want \"plain text\"", got)
	}
}

// TestNewTestServer_InjectsEmptyWordencFilter pins that the test scaffolding
// injects encfilter.Empty() so existing tests that call s.wordenc.Filter pass
// through chat text unchanged.
func TestNewTestServer_InjectsEmptyWordencFilter(t *testing.T) {
	s := newTestServer(t)
	if s.wordenc == nil {
		t.Fatal("newTestServer must inject a non-nil *encfilter.Filter")
	}
	// Empty filter is a passthrough — no substitution occurs.
	if got := s.wordenc.Filter("anal"); got != "anal" {
		t.Errorf("newTestServer injected filter should be Empty (passthrough); got %q", got)
	}
}
