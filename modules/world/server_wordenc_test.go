package world

import (
	"path/filepath"
	"testing"
)

// TestNewServer_LoadsWordencFilter pins that NewServer populates s.wordenc from
// the raw wordenc jagfile. Rev-244: TS dropped the existence check and
// hardcoded "data/raw/wordenc" — NewServer now fails when the jag is absent.
// Test skips when the Server244-ref reference cache (needed for NewServer loads)
// is unavailable.
//
// Uses the rev-244 reference cache via ref244CacheDir (defined in
// testdata_path_test.go); the repo's own data/pack is not used because
// the git common-dir fallback can resolve a different revision's pack
// (e.g. 245.2-format) that LoadComponentTypes can no longer decode.
//
// encfilter.Load() reads data/raw/wordenc relative to the working directory;
// t.Chdir changes to the repo root so the committed data/raw/wordenc is reachable.
//
// TS ref: Engine-TS/src/cache/wordenc/WordEnc.ts:35-37 (static WordEnc.load).
func TestNewServer_LoadsWordencFilter(t *testing.T) {
	cachePath := ref244CacheDir(t)
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
	t.Cleanup(func() { s.tcpListener.Close() })
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
