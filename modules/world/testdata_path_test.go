package world

import (
	"os"
	"testing"
)

// ref244Cache is the rev-244 reference cache (Engine-TS 9aadcec4 + Content
// e5d0282e, bun-packed via Server244-ref worktrees). Tests that load game
// configs — including LoadComponentTypes which expects the rev-244 component
// layout — must use this constant rather than a realCacheDir helper: the
// git common-dir fallback that realCacheDir used can resolve a different
// revision's data/pack (e.g. a 245.2-format pack from the main checkout)
// which panics in rev-244's Component decoder.
const ref244Cache = "/home/owner/Code/github.com/LostCityRS/Server244-ref/engine/data/pack"

// ref244CacheDir returns ref244Cache, skipping the test when the reference
// checkout is not present on this machine.
func ref244CacheDir(t *testing.T) string {
	t.Helper()
	if _, err := os.Stat(ref244Cache); err != nil {
		t.Skipf("Server244-ref cache unavailable: %v", err)
	}
	return ref244Cache
}
