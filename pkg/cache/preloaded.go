package cache

import (
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"

	"github.com/zsrv/goscape/pkg/io/packet"
)

// PreloadSnapshot is the immutable per-rebuild view of the
// PRELOADED/PRELOADED_CRC maps. See DEVIATION-NAI-215-CACHE-ATOMIC-SWAP
// marker at the top of crctable.go.
//
// Data maps bare filenames (e.g. "m30_72", "adventure.mid",
// "advance agility.mid") to their raw bytes. Mirrors TS
// PreloadedPacks.ts's `PRELOADED` Map<string, Uint8Array>.
//
// CRC pairs with Data: bare-filename → CRC32/IEEE of the raw bytes.
// Mirrors TS PRELOADED_CRC.
//
// Distinct from CRCSnapshot in crctable.go — that is the 9-slot JAG
// archive-CRC table served by the /crc HTTP endpoint; this is
// per-individual-file state for MIDI playback + map/loc streaming.
type PreloadSnapshot struct {
	Data map[string][]byte
	CRC  map[string]uint32
}

var preloadPtr atomic.Pointer[PreloadSnapshot]

// Preload returns the current snapshot. Never nil — empty snapshot if
// PreloadClient hasn't been called yet.
func Preload() *PreloadSnapshot {
	s := preloadPtr.Load()
	if s == nil {
		return &PreloadSnapshot{Data: map[string][]byte{}, CRC: map[string]uint32{}}
	}
	return s
}

// PreloadClient walks baseDir/{maps,songs,jingles}, builds a fresh
// snapshot, and atomically swaps it in. Mirrors TS preloadClient() at
// PreloadedPacks.ts:8-41.
//
// Error-returning (vs TS's throw-on-failure) so the caller can fail the
// world startingFn cleanly. Eager: all three dirs read synchronously
// before the swap.
//
// On error the prior snapshot is preserved (snapshot is built into a
// fresh map then swapped in atomically — no partial-success leak into
// the live state, unlike the prior in-place map-mutation
// implementation).
func PreloadClient(baseDir string) error {
	data := map[string][]byte{}
	crc := map[string]uint32{}
	for _, sub := range []string{"maps", "songs", "jingles"} {
		dir := filepath.Join(baseDir, sub)
		entries, err := os.ReadDir(dir)
		if err != nil {
			return fmt.Errorf("preload %s: %w", sub, err)
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			path := filepath.Join(dir, name)
			b, err := os.ReadFile(path)
			if err != nil {
				return fmt.Errorf("preload read %s: %w", path, err)
			}
			data[name] = b
			crc[name] = packet.GetCRC(b, 0, len(b))
		}
	}
	preloadPtr.Store(&PreloadSnapshot{Data: data, CRC: crc})
	return nil
}

// SetPreloadForTest installs a custom snapshot. Test-only.
func SetPreloadForTest(s *PreloadSnapshot) { preloadPtr.Store(s) }

// ResetPreloadForTest clears the snapshot. Test-only.
func ResetPreloadForTest() { preloadPtr.Store(nil) }
