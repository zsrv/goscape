package world

import (
	"os"
	"path/filepath"
	"testing"
)

// ref225Cache is the rev-225 reference cache (Server225_2/engine at the
// Arc-26 byte-parity baseline). Tests that load game configs must use this
// helper rather than a realCacheDir helper: the git common-dir fallback
// that realCacheDir used can resolve a different revision's data/pack (e.g.
// a 245.2-format pack from the main checkout) which panics in rev-225's
// NPC/component decoders (config code 99 = unrecognized for this revision).
//
// Resolved from GOSCAPE_REF225_DIR (pointing at .../engine, pack derived as
// data/pack); returns "" when the env var is unset.
func ref225Cache() string {
	if ref := os.Getenv("GOSCAPE_REF225_DIR"); ref != "" {
		return filepath.Join(ref, "data", "pack")
	}
	return ""
}

// ref225CacheDir returns the reference cache-pack dir, skipping the test when
// GOSCAPE_REF225_DIR is unset or the reference checkout is not present.
func ref225CacheDir(t *testing.T) string {
	t.Helper()
	cache := ref225Cache()
	if cache == "" {
		t.Skip("Server225_2 cache unavailable: GOSCAPE_REF225_DIR not set")
	}
	if _, err := os.Stat(cache); err != nil {
		t.Skipf("Server225_2 cache unavailable: %v", err)
	}
	return cache
}
