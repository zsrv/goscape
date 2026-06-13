package world

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	packmaps "github.com/zsrv/goscape/pkg/pack/maps"
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

// ref274Content is the rev-274 Content worktree (the source maps/ tree the
// reference toolchain packs). The 274 reference engine cache stores its
// server-side maps in .cache/maps-server.zip (an upstream incremental-build
// layout goscape neither replicates nor reads), so it has NO loose
// data/pack/server/maps/ directory. Map-dependent collision/pathing tests
// therefore source LOOSE maps from this Content tree via goscape's own
// pkg/pack/maps packer (see packTestServerMapsLoose). The non-map cache data
// (loc.dat, script.dat, …) still resolves from ref274Cache, which has it.
const ref274Content = "/home/owner/Code/github.com/LostCityRS/Server274-ref/content"

// ref274CacheDir returns ref274Cache, skipping the test when the reference
// checkout is not present on this machine.
func ref274CacheDir(t *testing.T) string {
	t.Helper()
	if _, err := os.Stat(ref274Cache); err != nil {
		t.Skipf("Server274-ref cache unavailable: %v", err)
	}
	return ref274Cache
}

var (
	looseMapsOnce sync.Once
	looseMapsDir  string
	looseMapsErr  error
)

// packTestServerMapsLoose packs the rev-274 Content's maps into a temporary
// cache directory with the loose server/maps/ layout (m/l/n/o + CSVs) that
// gamemap.Init reads, using goscape's own pkg/pack/maps packer — the exact
// production path, byte-parity-verified at plan Task T20.
//
// The 274 reference engine cache stores maps in .cache/maps-server.zip, so it
// has no loose server/maps/. This helper restores that loose layout for the
// map-dependent collision/pathing tests (NAI-95, NAI-101) without reading the
// upstream build-infra zip. The result is cached per test process (~1.3s to
// pack all 483 mapsquares) and shared via t.TempDir-independent storage
// (os.MkdirTemp) so it survives across the subtests of every caller.
//
// Skips the test when the Content worktree is absent.
func packTestServerMapsLoose(t *testing.T) string {
	t.Helper()
	if _, err := os.Stat(filepath.Join(ref274Content, "maps")); err != nil {
		t.Skipf("Server274-ref content maps unavailable: %v", err)
	}
	looseMapsOnce.Do(func() {
		out, err := os.MkdirTemp("", "goscape-loosemaps-")
		if err != nil {
			looseMapsErr = err
			return
		}
		// PackServerMaps writes only the loose server m/l/n/o streams + CSVs
		// that gamemap.Init reads — the same byte-parity-verified encoders the
		// production Pack uses, without the worldmap rebuild or NPC validation
		// (which would need config artifacts these collision tests don't load).
		if err := packmaps.PackServerMaps(ref274Content, out); err != nil {
			looseMapsErr = err
			return
		}
		looseMapsDir = out
	})
	if looseMapsErr != nil {
		t.Fatalf("packTestServerMapsLoose: %v", looseMapsErr)
	}
	return looseMapsDir
}
