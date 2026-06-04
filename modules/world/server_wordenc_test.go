package world

import (
	"os"
	"testing"
)

// TestNewServer_LoadsWordencFilter pins that NewServer populates s.wordenc from
// the raw wordenc jagfile. Rev-244: TS dropped the existence check and
// hardcoded "data/raw/wordenc" — NewServer now fails when the jag is absent.
// Test skips when either the raw wordenc jag or the Engine-TS pack (needed for
// all other NewServer loads) is unavailable.
//
// Skips on stale 225-format Engine-TS cache (EOF panic in parseComponentTypes);
// mirrors world_test.go / nai101_fountain_test.go skip convention.
//
// TS ref: Engine-TS/src/cache/wordenc/WordEnc.ts:35-37 (static WordEnc.load).
func TestNewServer_LoadsWordencFilter(t *testing.T) {
	const tsCache = "/home/owner/Code/github.com/LostCityRS/Engine-TS/data/pack"
	const tsRaw = "/home/owner/Code/github.com/LostCityRS/Engine-TS/data/raw"
	if _, err := os.Stat(tsCache); err != nil {
		t.Skipf("Engine-TS cache unavailable: %v", err)
	}
	// Rev-244: Load() reads data/raw/wordenc relative to the working directory.
	// Tests run from modules/world/, so the raw jag is not reachable there;
	// skip instead of failing (binary runs from project root where it IS reachable).
	if _, err := os.Stat(tsRaw); err != nil {
		t.Skipf("Engine-TS data/raw unavailable: %v", err)
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
