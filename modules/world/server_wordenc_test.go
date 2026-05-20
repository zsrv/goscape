package world

import (
	"os"
	"testing"
)

// TestNewServer_LoadsWordencFilter pins that NewServer populates s.wordenc from
// the cache. Uses the canonical Engine-TS cache path; skips when the pack is
// absent (mirrors world_test.go / nai101_fountain_test.go skip convention).
//
// TS ref: Engine-TS/src/cache/wordenc/WordEnc.ts:37-44 (static WordEnc.load).
func TestNewServer_LoadsWordencFilter(t *testing.T) {
	const tsCache = "/home/owner/Code/github.com/LostCityRS/Engine-TS/data/pack"
	if _, err := os.Stat(tsCache); err != nil {
		t.Skipf("Engine-TS cache unavailable: %v", err)
	}
	cfg := Config{CachePath: tsCache}
	s, err := NewServer(cfg, nil, nil, discardLogger())
	if err != nil {
		t.Skipf("NewServer failed (expected when data/ not staged): %v", err)
	}
	if s.wordenc == nil {
		t.Fatal("NewServer must populate s.wordenc; got nil")
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
