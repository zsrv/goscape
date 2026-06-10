package world

import (
	"os"
	"testing"
)

// ref225Cache is the rev-225 reference cache (Server225_2/engine at the
// Arc-26 byte-parity baseline). Tests that load game configs must use this
// constant rather than a realCacheDir helper: the git common-dir fallback
// that realCacheDir used can resolve a different revision's data/pack (e.g.
// a 245.2-format pack from the main checkout) which panics in rev-225's
// NPC/component decoders (config code 99 = unrecognized for this revision).
const ref225Cache = "/home/owner/Code/github.com/LostCityRS/Server225_2/engine/data/pack"

// ref225CacheDir returns ref225Cache, skipping the test when the reference
// checkout is not present on this machine.
func ref225CacheDir(t *testing.T) string {
	t.Helper()
	if _, err := os.Stat(ref225Cache); err != nil {
		t.Skipf("Server225_2 cache unavailable: %v", err)
	}
	return ref225Cache
}
