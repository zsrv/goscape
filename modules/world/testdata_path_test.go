package world

import (
	"os"
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
const ref254Cache = "/home/owner/Code/github.com/LostCityRS/Server254-ref/engine/data/pack"

// ref254CacheDir returns ref254Cache, skipping the test when the reference
// checkout is not present on this machine.
func ref254CacheDir(t *testing.T) string {
	t.Helper()
	if _, err := os.Stat(ref254Cache); err != nil {
		t.Skipf("Server254-ref cache unavailable: %v", err)
	}
	return ref254Cache
}
