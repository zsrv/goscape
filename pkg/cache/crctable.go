package cache

import (
	"log/slog"
	"os"
	"path/filepath"
	"sync/atomic"

	"github.com/zsrv/goscape/pkg/io/packet"
)

// DEVIATION-NAI-215-CACHE-ATOMIC-SWAP: package-level mutable cache state
// (CrcBytes, CrcTable, Preloaded, PreloadedCRC) is read by concurrent
// asset HTTP + world TCP goroutines and is rebuilt live during the
// ::reload admin command. Original TypeScript single-threaded model
// reassigned package globals in place. Go port uses build-then-swap
// via atomic.Pointer[CRCSnapshot] / atomic.Pointer[PreloadSnapshot] —
// reader sees either old-complete or new-complete, never torn.
// See pkg/cache/crctable.go + pkg/cache/preloaded.go.

// CRCSnapshot is the immutable per-rebuild view of the 9-slot JAG
// archive-CRC table. Build a fresh value, atomically swap, drop the old.
//
// Bytes is the serialized 9-slot table — what asset /crc HTTP serves.
// Table is the 9 raw CRCs — what world login compares against.
// (The package-level CrcBuffer32 of the old API was unused in prod —
// the only reader at server.go:817 was commented out — so it is dropped.
// If a future caller needs it, recompute from Bytes via packet.GetCRC.)
type CRCSnapshot struct {
	Bytes []byte
	Table []uint32
}

var crcPtr atomic.Pointer[CRCSnapshot]

// CRC returns the current snapshot. Never nil. Pre-MakeCRCs callers
// get a zero-value snapshot (zero-length Bytes/Table) — preserves the
// prior pre-init contract where the package-level CrcBytes/CrcTable
// were nil.
func CRC() *CRCSnapshot {
	s := crcPtr.Load()
	if s == nil {
		return &CRCSnapshot{}
	}
	return s
}

// MakeCRCs builds a fresh snapshot off to the side and atomically swaps
// it in. Safe to call concurrently with CRC() readers — readers see
// either the old-complete or new-complete snapshot, never a torn write.
//
// cachePath is the cache root (mirrors world.Config.CachePath, e.g.
// "data/pack"); per-archive paths are joined as <cachePath>/client/<name>.
// Completes the data-path-resolution work from Arc 13 V (ae6d6aa1) which
// shipped realCacheDir(t) for PreloadClient but missed this function;
// the prior hardcoded "data/pack/client/" relative path emitted
// `WARN cache: loadCrc Stat failed` noise under git-worktree test runs.
func MakeCRCs(cachePath string) {
	buf := packet.NewPacket(make([]byte, 0, 4*9))
	table := make([]uint32, 0, 9)

	// slot 0: header (always 0)
	buf.P4(0)
	table = append(table, 0)

	clientDir := filepath.Join(cachePath, "client")
	for _, name := range []string{"title", "config", "interface", "media", "models", "textures", "wordenc", "sounds"} {
		crc := loadCrc(filepath.Join(clientDir, name))
		buf.P4(crc)
		table = append(table, crc)
	}

	crcPtr.Store(&CRCSnapshot{
		Bytes: append([]byte(nil), buf.Bytes()...),
		Table: table,
	})
}

// loadCrc returns the CRC for a packed-client archive file, or 0 if the
// file is missing or unreadable (same posture as the old in-place
// makeCrc helper — log a warning and continue, so missing archives
// surface as zero-CRC mismatches at login rather than startup panics).
func loadCrc(path string) uint32 {
	if _, err := os.Stat(path); err != nil {
		slog.Warn("cache: loadCrc Stat failed",
			"path", path, "err", err)
		return 0
	}
	p, err := packet.Load(path, false)
	if err != nil {
		slog.Warn("cache: loadCrc Load failed",
			"path", path, "err", err)
		return 0
	}
	return packet.GetCRC(p.Bytes(), 0, len(p.Bytes()))
}

// ResetCRCForTest clears the snapshot. Test-only.
func ResetCRCForTest() {
	crcPtr.Store(nil)
}

// SetCRCForTest installs a custom snapshot. Test-only.
func SetCRCForTest(s *CRCSnapshot) {
	crcPtr.Store(s)
}
