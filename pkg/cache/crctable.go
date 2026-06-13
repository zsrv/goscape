package cache

import (
	"log/slog"
	"os"
	"sync/atomic"

	"github.com/zsrv/goscape/pkg/io/filestream"
	"github.com/zsrv/goscape/pkg/io/packet"
)

// DEVIATION-NAI-215-CACHE-ATOMIC-SWAP: package-level mutable cache state
// (CrcBytes, CrcTable) is read by concurrent ondemand HTTP + world TCP
// goroutines and is rebuilt live during the ::reload admin command.
// Original TypeScript single-threaded model reassigned package globals in
// place. Go port uses build-then-swap via atomic.Pointer[CRCSnapshot] —
// reader sees either old-complete or new-complete, never torn.
// See pkg/cache/crctable.go. (PreloadedPacks.ts deleted at 244; the
// former atomic.Pointer[PreloadSnapshot] in preloaded.go is gone.)

// CRCSnapshot is the immutable per-rebuild view of the 9-slot JAG
// archive-CRC table. Build a fresh value, atomically swap, drop the old.
//
// Bytes is the serialized 9-slot table + the 4-byte rolling-hash trailer
// (rev-274, 40 bytes total) — what ondemand /crc HTTP serves.
// Table is the 9 raw CRCs — what world login compares against.
// (The package-level CrcBuffer32 of the old API was unused in prod —
// the only reader at server.go:817 was commented out — so it is dropped.
// At rev-274 TS still computes CrcBuffer32 over CrcBuffer.data[:len-4]
// (excluding the trailer) for its single-hash login check; goscape's
// per-slot Table compare supersedes it, so CrcBuffer32 stays dropped —
// the 274 diff adds no new reader. If a future caller needs it, recompute
// from Bytes[:36] via packet.GetCRC.)
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
// "data/pack"). MakeCRCs opens a FileStream at cachePath and reads archive
// 0 to derive CRCs — matching TS CrcTable.ts:11-29 at dee467c8:
//
//	count = OnDemand.cache.count(0)
//	for i = 0; i < count; i++ {
//	    jag = cache.read(0, i)   // decompress=false (2-arg form, FileStream.ts:43)
//	    if jag { CrcTable[i] = getcrc; p4(CrcTable[i]) } else { CrcTable[i] = 0; p4(0) }
//	}
//	hash = 1234
//	for i = 0; i < 9; i++ { hash = (hash << 1) + CrcTable[i] }
//	p4(hash)
//
// Archive 0 file 0 is conventionally absent in the RS2 dat/idx cache; its
// idx entry is all zeros (sector=0 → Read returns nil → p4(0)), reproducing
// the leading-zero slot that the 225 shape wrote explicitly as p4(0).
//
// CrcBuffer wire shape: always 40 bytes at rev-274 (TS:
// new Packet(new Uint8Array(4*9+4))) — 36 CRC slots + a 4-byte big-endian
// rolling-hash trailer. When count < 9 the unused CRC tail stays zero
// (faithful to the pre-allocated fixed buffer); the hash loop still iterates
// all 9 slots, treating absent ones as 0.
//
// PORTING-EXCEPTION (rev244-b3-crc-compare, per-slot table vs TS
// CrcBuffer32): TS 244 leaves CrcTable EMPTY (makeCrcs resets it at
// CrcTable.ts:13 and never pushes) and the login check compares the single
// 32-bit hash CrcBuffer32 against getcrc of the client's 36 CRC bytes
// (World.ts:2170). goscape — since rev-225 — validates per-slot instead:
// Table here carries count entries and the login compare is
// slices.Equal(Table, client CRCs) (modules/world server handleLogin).
// Wire bytes are identical either way; the per-slot predicate is strictly
// stronger (a crc32-colliding forged blob passes TS, never goscape). An
// empty/absent cache yields an empty Table → every login rejected
// out-of-date until a real cache exists (B6-deferred posture).
// See PORTING.md.
//
// Module-init guard (CrcTable.ts:29-33): maps to the existing world-start
// and ::reload call sites (modules/world/world.go + reload.go). No new call
// sites are added.
//
// filestream.New creates missing cache files when cachePath does not exist
// yet; an empty cache yields count=0 → 36 zero CRC bytes + the rolling hash
// of an all-zero table (1234<<9) + empty table.
func MakeCRCs(cachePath string) {
	// Open FileStream read-only; New creates empty dat/idx if missing.
	// TS: OnDemand.cache is a FileStream opened once at server start;
	// Go opens a short-lived view here so MakeCRCs stays self-contained
	// and does not require a shared FileStream reference in the package.
	// readOnly=true avoids O_RDWR on files the packer may be writing.
	fs := filestream.New(cachePath, false, true)
	defer fs.Close() //nolint:errcheck // Close on read-only cache; errors are non-fatal.

	count := fs.Count(0)

	// Fixed 40-byte buffer at rev-274: TS CrcBuffer =
	// new Packet(new Uint8Array(4*9+4)) — 36 CRC slots + a 4-byte rolling
	// hash. Zero-initialised; the first count*4 bytes are filled by the CRC
	// loop, the next (9-count)*4 stay zero, then offset 36 holds the hash.
	var wire [4*9 + 4]byte

	table := make([]uint32, 0, count)

	for i := range count {
		data := fs.Read(0, i, false) // decompress=false, TS FileStream.ts:43
		var crc uint32
		if data != nil {
			crc = packet.GetCRC(data, 0, len(data))
		}
		// Big-endian p4 at offset i*4 in the fixed wire buffer.
		wire[i*4] = byte(crc >> 24)
		wire[i*4+1] = byte(crc >> 16)
		wire[i*4+2] = byte(crc >> 8)
		wire[i*4+3] = byte(crc)
		table = append(table, crc)
	}

	// Rolling-hash trailer (TS CrcTable.ts:25-29 at dee467c8):
	//   hash = 1234; for i in 0..9: hash = (hash << 1) + CrcTable[i]; p4(hash)
	// JS coerces to int32 at each `<<`; faithful Go is int32 arithmetic with
	// two's-complement wrap (each operand ≤ 2^32, intermediate sum ≤ 2^33 is
	// exact in double then re-coerced at the next shift — provably identical
	// to JS ToInt32 here). Missing slots (count < 9) contribute 0; the real
	// cache always has 9.
	hash := int32(1234)
	for i := range 9 {
		var c uint32
		if i < len(table) {
			c = table[i]
		}
		hash = (hash << 1) + int32(c)
	}
	h := uint32(hash)
	wire[36] = byte(h >> 24)
	wire[37] = byte(h >> 16)
	wire[38] = byte(h >> 8)
	wire[39] = byte(h)

	crcPtr.Store(&CRCSnapshot{
		Bytes: append([]byte(nil), wire[:]...),
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
