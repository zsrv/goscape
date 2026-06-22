package world

import (
	"os"
	"path/filepath"
	"testing"
)

// ref254Cache is the rev-254 reference cache (Engine-TS 2e3bcf43 + the
// pinned Content, bun-packed via the Server254-ref worktrees at the
// pin-advance, addendum Task T17). ALL modules/world tests that read a
// packed cache resolve it here.
//
// Re-pinned from the 245.2 reference at addendum Task A1: the 2e3bcf43
// script-op enum regen (418→396, 231 values moved) makes the 245.2
// script.dat semantically incompatible with the rev-254 opcode table, and
// the new cache's script.dat header reads compiler version 27
// (pkg/script.CompilerVersion). Task A17 additionally renames the
// pack-parity env gate to GOSCAPE_REF254_DIR.
//
// History: tests used to resolve the repo's own data/pack with a
// git-common-dir fallback (realCacheDir) that, from a linked worktree,
// landed on the MAIN checkout's cache. data/pack is revision-specific
// generated output, so the moment the main checkout moves to a different
// revision branch every other branch's cache tests silently read a
// wrong-format cache. Pinning each branch to its own reference cache
// removes the cross-revision hazard; the pinned worktree is the same
// byte-parity baseline the pack gates use.
//
// Resolved from GOSCAPE_REF254_DIR (pointing at .../engine, pack derived as
// data/pack); returns "" when the env var is unset.
func ref254Cache() string {
	if ref := os.Getenv("GOSCAPE_REF254_DIR"); ref != "" {
		return filepath.Join(ref, "data", "pack")
	}
	return ""
}

// ref254CacheDir returns the reference cache-pack dir, skipping the test when
// GOSCAPE_REF254_DIR is unset or the reference checkout is not present.
func ref254CacheDir(t *testing.T) string {
	t.Helper()
	cache := ref254Cache()
	if cache == "" {
		t.Skip("Server254-ref cache unavailable: GOSCAPE_REF254_DIR not set")
	}
	if _, err := os.Stat(cache); err != nil {
		t.Skipf("Server254-ref cache unavailable: %v", err)
	}
	return cache
}
