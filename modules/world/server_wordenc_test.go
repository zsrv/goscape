package world

import (
	"testing"
)

// TestNewServer_LoadsWordencFilter pins that NewServer populates s.wordenc from
// the cache. Uses the rev-225 reference cache via ref225CacheDir (defined in
// testdata_path_test.go); the repo's own data/pack is not used because the
// git common-dir fallback can resolve a different revision's pack (e.g.
// 245.2-format) that rev-225's decoders can no longer read.
//
// TS ref: Engine-TS/src/cache/wordenc/WordEnc.ts:37-44 (static WordEnc.load).
func TestNewServer_LoadsWordencFilter(t *testing.T) {
	cfg := Config{
		CachePath:        ref225CacheDir(t),
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
