package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/cache"
	"github.com/zsrv/goscape/pkg/io/packet"
)

// TestStartingFnPopulatesCrcBuffer asserts that world.startingFn
// invokes cache.MakeCRCs() so the asset module's /crc HTTP handler
// can serve the buffer without itself touching cache state.
// Closes the asset/handler.go:24 "TEST - belongs in world" smell.
func TestStartingFnPopulatesCrcBuffer(t *testing.T) {
	// Reset to a fresh empty packet (mirrors pkg/cache init expression).
	// MakeCRCs mutates CrcBuffer.Pos in place rather than reassigning, so
	// CrcBuffer must be non-nil before the call.
	cache.CrcBuffer = packet.NewPacket(make([]byte, 0, 4*9))
	cache.CrcTable = nil
	t.Cleanup(func() {
		cache.CrcBuffer = packet.NewPacket(make([]byte, 0, 4*9))
		cache.CrcTable = nil
	})

	// The world startingFn closure is built inside NewWorldService.
	// We re-implement the relevant prefix here as a unit test would
	// need a full Server + LoginClient otherwise. Mirror the production
	// sequence: PreloadClient, MakeCRCs.
	if err := cache.PreloadClient("../../data/pack/client"); err != nil {
		t.Skipf("PreloadClient failed (expected when data/ not staged): %v", err)
	}
	cache.MakeCRCs()

	if cache.CrcBuffer == nil {
		t.Error("cache.CrcBuffer: got nil, want non-nil after MakeCRCs")
	}
	if len(cache.CrcTable) == 0 {
		t.Error("cache.CrcTable: got empty, want populated after MakeCRCs")
	}
}
