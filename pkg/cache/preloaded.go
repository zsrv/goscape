package cache

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/zsrv/goscape/pkg/io/packet"
)

// Preloaded maps bare filenames (e.g. "m30_72", "adventure.mid",
// "advance agility.mid") to their raw bytes. Mirrors TS
// PreloadedPacks.ts's `PRELOADED` Map<string, Uint8Array>.
//
// Write-once at world-module startup via PreloadClient; read-many at
// runtime by (*Player).PlaySong / PlayJingle (and future Rebuild*
// consumers — see TS RebuildNormalEncoder.ts:18-19,
// RebuildGetMapsHandler.ts:44,54).
//
// Distinct from CrcTable / CrcBuffer in crctable.go — those are the
// 9-slot JAG archive-CRC table served by the /crc HTTP endpoint; this
// is per-individual-file state for MIDI playback + map/loc streaming.
var Preloaded = map[string][]byte{}

// PreloadedCRC pairs with Preloaded: bare-filename → CRC32/IEEE of the
// raw bytes. Mirrors TS PRELOADED_CRC. Same write/read posture as
// Preloaded above.
var PreloadedCRC = map[string]uint32{}

// PreloadClient walks baseDir/{maps,songs,jingles} and populates
// Preloaded + PreloadedCRC. Mirrors TS preloadClient() at
// PreloadedPacks.ts:8-41.
//
// Error-returning (vs TS's throw-on-failure) so the caller can fail
// the world startingFn cleanly. Eager: all three dirs read
// synchronously before return.
//
// Partial-success leak: if maps/ loads but songs/ fails, Preloaded
// already contains map entries when the error returns. Not retried
// (the services.BasicService lifecycle treats startingFn as one-shot;
// failure halts the service). Documented; acceptable.
func PreloadClient(baseDir string) error {
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
			data, err := os.ReadFile(path)
			if err != nil {
				return fmt.Errorf("preload read %s: %w", path, err)
			}
			Preloaded[name] = data
			PreloadedCRC[name] = packet.GetCRC(data, 0, len(data))
		}
	}
	return nil
}
