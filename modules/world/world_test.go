package world

import (
	"path/filepath"
	"testing"

	"github.com/zsrv/goscape/pkg/cache"
)

// TestStartingFnPopulatesCRCSnapshot asserts that the production sequence
// (cache.PreloadClient → cache.MakeCRCs, as invoked by world.startingFn)
// publishes a non-empty CRC snapshot. The test mirrors startingFn's
// prefix rather than invoking NewWorldService directly, because the
// latter requires a full Server + LoginClient fixture.
// Pairs with cmd/goscape/app/modules_test.go's TestAssetDependsOnWorld
// which pins the asset→world dep-edge that makes the sequence visible
// to /crc requests at request time.
func TestStartingFnPopulatesCRCSnapshot(t *testing.T) {
	cache.ResetCRCForTest()
	t.Cleanup(cache.ResetCRCForTest)

	// The world startingFn closure is built inside NewWorldService.
	// We re-implement the relevant prefix here as a unit test would
	// need a full Server + LoginClient otherwise. Mirror the production
	// sequence: PreloadClient, MakeCRCs.
	cacheDir := realCacheDir(t)
	if err := cache.PreloadClient(filepath.Join(cacheDir, "client")); err != nil {
		t.Skipf("PreloadClient failed (expected when data/ not staged): %v", err)
	}
	cache.MakeCRCs(cacheDir)

	snap := cache.CRC()
	if len(snap.Bytes) == 0 {
		t.Error("cache.CRC().Bytes: got empty, want non-empty after MakeCRCs")
	}
	if len(snap.Table) == 0 {
		t.Error("cache.CRC().Table: got empty, want populated after MakeCRCs")
	}
}
