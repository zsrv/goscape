package world

import (
	"os"
	"testing"
)

// ref274Cache is the rev-274 reference cache (Engine-TS + the pinned Content,
// bun-packed via the Server274-ref worktrees, byte-parity-verified at plan
// Task T20). ALL modules/world tests that read a packed cache resolve it here.
//
// Re-pinned from the rev-254 reference at plan Task T21: the rev-274
// ScriptOpcode enum inserts 4 ops mid-enum (MAP_LOC, MINIMAP_TOGGLE,
// SET_SKILL_LEVEL, NPC_DESTINATION) which shifts every implicitly-numbered
// opcode after each insertion point, so the rev-254 script.dat bytecode is
// reinterpreted by the rev-274 dispatch table. The rev-274 cache is packed
// with the rev-274 opcode numbering, so its script.dat is semantically
// compatible with the rev-274 opcode table. This repin also un-skips the
// NAI-128 cascade test that was deferred at Task T6 for exactly this reason.
//
// History: tests used to resolve the repo's own data/pack with a
// git-common-dir fallback (realCacheDir) that, from a linked worktree,
// landed on the MAIN checkout's cache. data/pack is revision-specific
// generated output, so the moment the main checkout moves to a different
// revision branch every other branch's cache tests silently read a
// wrong-format cache. Pinning each branch to its own reference cache
// removes the cross-revision hazard; the pinned worktree is the same
// byte-parity baseline the pack gates use.
const ref274Cache = "/home/owner/Code/github.com/LostCityRS/Server274-ref/engine/data/pack"

// ref274CacheDir returns ref274Cache, skipping the test when the reference
// checkout is not present on this machine.
func ref274CacheDir(t *testing.T) string {
	t.Helper()
	if _, err := os.Stat(ref274Cache); err != nil {
		t.Skipf("Server274-ref cache unavailable: %v", err)
	}
	return ref274Cache
}
