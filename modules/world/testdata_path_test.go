package world

import (
	"os"
	"path/filepath"
	"testing"
)

// ref244Cache is the rev-244 reference cache (Engine-TS 9aadcec4 + Content
// e5d0282e, bun-packed via Server244-ref worktrees). Tests that load game
// configs — including LoadComponentTypes which expects the rev-244 component
// layout — must use this helper rather than a realCacheDir helper: the
// git common-dir fallback that realCacheDir used can resolve a different
// revision's data/pack (e.g. a 245.2-format pack from the main checkout)
// which panics in rev-244's Component decoder.
//
// Resolved from GOSCAPE_REF244_DIR (pointing at .../engine, pack derived as
// data/pack); returns "" when the env var is unset.
func ref244Cache() string {
	if ref := os.Getenv("GOSCAPE_REF244_DIR"); ref != "" {
		return filepath.Join(ref, "data", "pack")
	}
	return ""
}

// ref244CacheDir returns the reference cache-pack dir, skipping the test when
// GOSCAPE_REF244_DIR is unset or the reference checkout is not present.
func ref244CacheDir(t *testing.T) string {
	t.Helper()
	cache := ref244Cache()
	if cache == "" {
		t.Skip("Server244-ref cache unavailable: GOSCAPE_REF244_DIR not set")
	}
	if _, err := os.Stat(cache); err != nil {
		t.Skipf("Server244-ref cache unavailable: %v", err)
	}
	return cache
}
