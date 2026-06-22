package world

import (
	"os"
	"path/filepath"
	"testing"
)

// ref245Cache is the 245.2 reference cache (Engine-TS 3c16994c + Content
// cbcfe670, bun-packed via Server245.2-ref worktrees). ALL modules/world
// tests that read a packed cache resolve it here.
//
// History: tests used to resolve the repo's own data/pack with a
// git-common-dir fallback (realCacheDir) that, from a linked worktree,
// landed on the MAIN checkout's cache. data/pack is revision-specific
// generated output, so the moment the main checkout moves to a different
// revision branch every other branch's cache tests silently read a
// wrong-format cache (found on rev-244/rev-225 during the post-rev-245.2
// backports: 245.2 packs carry component swappable/activeOverColour and
// npc config code 99 that older parsers reject). Pinning each branch to
// its own reference cache removes the cross-revision hazard; the pinned
// worktree is the same byte-parity baseline the pack gates use.
//
// Resolved from GOSCAPE_REF245_DIR (pointing at .../engine, pack derived as
// data/pack); returns "" when the env var is unset.
func ref245Cache() string {
	if ref := os.Getenv("GOSCAPE_REF245_DIR"); ref != "" {
		return filepath.Join(ref, "data", "pack")
	}
	return ""
}

// ref245CacheDir returns the reference cache-pack dir, skipping the test when
// GOSCAPE_REF245_DIR is unset or the reference checkout is not present.
func ref245CacheDir(t *testing.T) string {
	t.Helper()
	cache := ref245Cache()
	if cache == "" {
		t.Skip("Server245.2-ref cache unavailable: GOSCAPE_REF245_DIR not set")
	}
	if _, err := os.Stat(cache); err != nil {
		t.Skipf("Server245.2-ref cache unavailable: %v", err)
	}
	return cache
}
