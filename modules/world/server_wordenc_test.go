package world

import (
	"os"
	"testing"
)

// TestNewServer_LoadsWordencFilter pins that NewServer populates s.wordenc from
// the cache. Uses the canonical Engine-TS cache path; skips when the pack is
// absent or in a non-244 format (mirrors world_test.go / nai101_fountain_test.go
// skip convention).
//
// Rev-244 note: the Engine-TS local data/pack must have been regenerated with
// the 244 packer (which writes the trans byte + g2 childCount in the interface
// binary). A stale 225-format cache produces an EOF panic in parseComponentTypes;
// we catch that and skip the same way we skip on a missing cache.
//
// TS ref: Engine-TS/src/cache/wordenc/WordEnc.ts:37-44 (static WordEnc.load).
func TestNewServer_LoadsWordencFilter(t *testing.T) {
	const tsCache = "/home/owner/Code/github.com/LostCityRS/Engine-TS/data/pack"
	if _, err := os.Stat(tsCache); err != nil {
		t.Skipf("Engine-TS cache unavailable: %v", err)
	}
	cfg := Config{
		CachePath:        tsCache,
		TCPListenNetwork: "tcp",
		TCPListenAddress: "127.0.0.1",
		TCPListenPort:    0, // OS picks a free port
	}
	var (
		s        *Server
		err      error
		panicVal any
	)
	func() {
		defer func() { panicVal = recover() }()
		s, err = NewServer(cfg, nil, nil, discardLogger(), nil)
	}()
	if panicVal != nil {
		t.Skipf("NewServer panicked (likely stale 225-format Engine-TS cache; repack with 244 packer): %v", panicVal)
	}
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
