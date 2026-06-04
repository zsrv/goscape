# rev-244 Bundle 1: io/cache/util primitives — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port the Bundle-1 slice of the Engine-TS 225→244 delta (`e1dea19f..9aadcec4`): the new `FileStream`/`GZip` io primitives, the PEM per-deployment token, and the cache-config decode deltas (SeqType/AnimFrame restructure, Component, NpcType, ObjType).

**Architecture:** Faithful TS→Go translation per `PORTING-LESSONS.md` (read it first — §3 gotchas, §4 comment conventions, §5 gates; it lives on the `main` branch: `git show main:PORTING-LESSONS.md`). Each task slices the upstream cross-pin diff to one file group; the TS diff is the contract. All work lands on branch `rev-244`. Reference checkout: `/home/owner/Code/github.com/LostCityRS/Engine-TS` (the local checkout is AT the 244 pin `9aadcec4` on branch `244-GOSCAPE`).

**Tech Stack:** Go (existing toolchain conventions: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache` prefix on every go command; `CGO_ENABLED=0 go build -trimpath ./...`; tests with `go test`, `-race` on touched packages).

**Spec:** `docs/superpowers/specs/2026-06-03-rev244-port-design.md`

**Bundle-1 scope notes (decisions already made — do not relitigate):**
- `src/cache/CrcTable.ts`, `src/cache/PreloadedPacks.ts` (deleted upstream), and `src/cache/DevThread.ts` changes are **deferred to B3/B6** — their rewiring is coupled to the new OnDemand engine and pack tooling. Do NOT touch `pkg/cache/crctable.go` / `pkg/cache/preloaded.go` in this bundle.
- `src/util/DoublyLinkList.ts` (new, 32 lines) has **zero consumers at the 244 pin** (verified via `git -C Engine-TS grep -l DoublyLinkList 9aadcec4 -- src tools`) — dead-at-pin. NOT ported; Task 8 records the exception.
- `src/io/Packet.ts` delta is import-path moves (TS `datastruct/`→`util/`, no Go analog) + `bitmask` made private — goscape's `pkg/io/packet/packetbit.go` bitmask is already unexported. NO-OP; Task 8 records it.
- **Format inconsistency window:** the config decoders move to the 244 cache format while `pkg/pack` still writes 225 format until B6. Unit tests are synthetic-packet based (existing convention in `pkg/objtype/*_test.go`); any test that loads the real `data/pack` cache and breaks on format must be gated with the existing skip-when-no-pack pattern and listed in the task report — do not "fix" decoders back to 225 shapes.

---

### Task 1: `pkg/io/filestream` — FileStream port

**Files:**
- Create: `pkg/io/filestream/filestream.go`
- Test: `pkg/io/filestream/filestream_test.go`

The TS source (`git -C /home/owner/Code/github.com/LostCityRS/Engine-TS show 9aadcec4:src/io/FileStream.ts`, 225 lines) is the contract. It implements the classic dat/idx client cache store: `main_file_cache.dat` + `main_file_cache.idx0..idx4`, 6-byte idx entries (`size:3, sector:3`), 520-byte dat sectors (8-byte header `file:2, part:2, nextSector:3, archiveIdx+1:1` + 512 data bytes), 2,000,000-byte size cap, in-memory `packed` cache of raw reads, gunzip on `decompress=true` for archives ≠ 0.

- [ ] **Step 1: Write the failing tests**

```go
package filestream

import (
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

// TS FileStream.ts:131-196 (write) + 44-123 (read): single-sector round-trip.
func TestWriteReadRoundTripSingleSector(t *testing.T) {
	fs := New(t.TempDir(), true, false)
	defer fs.Close()

	data := []byte("hello cache")
	if !fs.Write(1, 0, data, 0) {
		t.Fatal("Write returned false")
	}
	got := fs.Read(1, 0, false)
	if !bytes.Equal(got, data) {
		t.Fatalf("Read = %q, want %q", got, data)
	}
}

// TS FileStream.ts:78-104: multi-sector chaining for payloads > 512 bytes.
func TestWriteReadRoundTripMultiSector(t *testing.T) {
	fs := New(t.TempDir(), true, false)
	defer fs.Close()

	data := make([]byte, 1300) // 3 sectors
	for i := range data {
		data[i] = byte(i % 251)
	}
	if !fs.Write(2, 5, data, 0) {
		t.Fatal("Write returned false")
	}
	if got := fs.Read(2, 5, false); !bytes.Equal(got, data) {
		t.Fatalf("multi-sector read mismatch: len=%d want %d", len(got), len(data))
	}
}

// TS FileStream.ts:140-147: version != 0 appends a 2-byte big-endian trailer.
func TestWriteVersionTrailer(t *testing.T) {
	fs := New(t.TempDir(), true, false)
	defer fs.Close()

	if !fs.Write(1, 0, []byte{0xAA}, 0x1234) {
		t.Fatal("Write returned false")
	}
	got := fs.Read(1, 0, false)
	want := []byte{0xAA, 0x12, 0x34}
	if !bytes.Equal(got, want) {
		t.Fatalf("Read = %x, want %x", got, want)
	}
}

// TS FileStream.ts:35-41: count = idx length / 6.
func TestCount(t *testing.T) {
	fs := New(t.TempDir(), true, false)
	defer fs.Close()

	if n := fs.Count(1); n != 0 {
		t.Fatalf("Count(1) = %d, want 0", n)
	}
	fs.Write(1, 0, []byte{1}, 0)
	fs.Write(1, 1, []byte{2}, 0)
	if n := fs.Count(1); n != 2 {
		t.Fatalf("Count(1) = %d, want 2", n)
	}
	// TS: out-of-range archive returns 0.
	if n := fs.Count(9); n != 0 {
		t.Fatalf("Count(9) = %d, want 0", n)
	}
}

// TS FileStream.ts:198-225 (has) + 49-59 (read bounds): bounds and missing files.
func TestHasAndReadBounds(t *testing.T) {
	fs := New(t.TempDir(), true, false)
	defer fs.Close()

	fs.Write(0, 0, []byte{7}, 0)
	if !fs.Has(0, 0) {
		t.Fatal("Has(0,0) = false after write")
	}
	if fs.Has(0, 1) || fs.Has(-1, 0) || fs.Has(5, 0) || fs.Has(0, -1) {
		t.Fatal("Has returned true for an out-of-range entry")
	}
	if fs.Read(0, 1, false) != nil || fs.Read(-1, 0, false) != nil {
		t.Fatal("Read returned data for an out-of-range entry")
	}
}

// TS FileStream.ts:107-122: decompress=true gunzips for archive != 0,
// returns raw for archive 0.
func TestReadDecompress(t *testing.T) {
	fs := New(t.TempDir(), true, false)
	defer fs.Close()

	plain := []byte("the payload to be gzipped for archive one")
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	zw.Write(plain)
	zw.Close()

	fs.Write(1, 0, buf.Bytes(), 0)
	if got := fs.Read(1, 0, true); !bytes.Equal(got, plain) {
		t.Fatalf("decompressed read mismatch: %q", got)
	}

	fs.Write(0, 0, buf.Bytes(), 0)
	if got := fs.Read(0, 0, true); !bytes.Equal(got, buf.Bytes()) {
		t.Fatal("archive 0 must return raw bytes even with decompress=true")
	}
}

// TS FileStream.ts:14-32: createNew=false on an existing dir preserves content;
// the constructor creates dat + idx0..idx4 when missing.
func TestPersistenceAcrossOpens(t *testing.T) {
	dir := t.TempDir()
	fs1 := New(dir, true, false)
	fs1.Write(3, 2, []byte("persisted"), 0)
	fs1.Close()

	for i := 0; i <= 4; i++ {
		if _, err := os.Stat(filepath.Join(dir, "main_file_cache.idx"+string(rune('0'+i)))); err != nil {
			t.Fatalf("idx%d missing: %v", i, err)
		}
	}

	fs2 := New(dir, false, true) // read-only reopen
	defer fs2.Close()
	if got := fs2.Read(3, 2, false); !bytes.Equal(got, []byte("persisted")) {
		t.Fatalf("reopen read = %q", got)
	}
}

// TS FileStream.ts:56-58 + 106-112: packed[] caches raw reads unless
// DiscardPacked; a second Read returns the cached slice.
func TestPackedCache(t *testing.T) {
	dir := t.TempDir()
	fs := New(dir, true, false)
	defer fs.Close()

	fs.Write(1, 0, []byte("cache me"), 0)
	first := fs.Read(1, 0, false)
	// Overwrite the entry behind the cache's back; the cached copy must win.
	fs2 := New(dir, false, false)
	fs2.Write(1, 0, []byte("OVERWROTE"), 0)
	fs2.Close()
	second := fs.Read(1, 0, false)
	if !bytes.Equal(first, second) {
		t.Fatal("second Read did not come from the packed cache")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/io/filestream/...`
Expected: FAIL (package does not compile — `New` undefined).

- [ ] **Step 3: Implement `pkg/io/filestream/filestream.go`**

Translate `FileStream.ts` line-by-line. Skeleton with the TS-faithful semantics (fill bodies exactly per the TS; every method carries a `// TS FileStream.ts:<lines>` citation):

```go
// Package filestream ports the 244 engine's FileStream (src/io/FileStream.ts
// at Engine-TS 9aadcec4): the dat/idx client cache store used by OnDemand,
// pack, and unpack tooling.
package filestream

import (
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
)

const (
	sectorSize  = 520
	sectorData  = 512
	maxFileSize = 2_000_000
	numIdx      = 5 // idx0..idx4; TS loops i <= 4
)

// FileStream mirrors TS FileStream. Not goroutine-safe (TS is single-threaded);
// callers own synchronization.
type FileStream struct {
	dat *os.File
	idx [numIdx]*os.File

	DiscardPacked bool
	packed        [numIdx]map[int][]byte
}

// New mirrors the TS constructor (FileStream.ts:14-32): creates dir,
// truncates/creates dat+idx0..4 when createNew or dat missing, opens
// read-only when readOnly.
func New(dir string, createNew, readOnly bool) *FileStream { /* per TS */ }

// Count mirrors FileStream.ts:35-41 (idx file length / 6; 0 on bad index).
func (f *FileStream) Count(index int) int { /* per TS */ }

// Read mirrors FileStream.ts:44-123. Returns nil on any validation failure
// (size cap, sector bounds, sector-header mismatch), caches raw payloads in
// packed unless DiscardPacked, gunzips when decompress && archive != 0.
func (f *FileStream) Read(archive, file int, decompress bool) []byte { /* per TS */ }

// Write mirrors FileStream.ts:131-196. version != 0 appends a 2-byte
// big-endian version trailer before writing.
func (f *FileStream) Write(archive, file int, data []byte, version int) bool { /* per TS */ }

// Has mirrors FileStream.ts:198-225.
func (f *FileStream) Has(archive, file int) bool { /* per TS */ }

// Close releases the underlying files (Go addition — TS relies on process
// exit; goscape needs deterministic cleanup in tests and reloads).
func (f *FileStream) Close() error { /* close dat + idx, first error wins */ }
```

Translation notes (PORTING-LESSONS §3 applied):
- TS `idx.length / 6` is float division then implicit floor via array use → Go integer division is already the right semantics; TS `count` bound check `archive >= this.idx.length` ports to `archive >= numIdx` (and `Count` uses `index > numIdx` faithfully? **No** — TS `count()` checks `index < 0 || index > this.idx.length` (a latent off-by-one allowing index 5) but `this.idx[5]` is undefined → `!this.idx[index]` catches it. In Go, mirror the *observable* behavior: out-of-range → 0. Cite the TS quirk in a comment.)
- TS `read` writes `data.pdata(header.data, header.pos, header.data.length)` — appends the *remaining* bytes of the sector packet after the 8-byte header. Port as a plain byte copy.
- TS sector loop `for part := 0; data.pos < size; part++` with `if sector === 0 break`.
- `zlib.gunzipSync` → `compress/gzip` reader over a `bytes.Reader` + `io.ReadAll`; on error return nil? **No** — TS `gunzipSync` THROWS on bad data and `read` does not catch → the TS process would throw. goscape's convention for impossible-corruption paths in ported code is to return nil and log nothing only when TS returns null; where TS throws, Go may return nil with a comment noting the TS throw (the OnDemand consumer in B3 will define the real contract). Keep it: error → return nil with `// TS throws here (gunzipSync); nil is goscape's panic-free analog.`
- `packed` uses maps keyed by file id (TS sparse arrays).

- [ ] **Step 4: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/io/filestream/... -v`
Expected: all 8 tests PASS.

- [ ] **Step 5: Race + vet, then commit**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache CGO_ENABLED=1 go test -race ./pkg/io/filestream/...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./pkg/io/filestream/...
git add pkg/io/filestream
git commit --no-gpg-sign -m "feat(io): port 244 FileStream dat/idx cache store [rev-244 B1]

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

### Task 2: `pkg/io/gziputil` — GZip helpers

**Files:**
- Create: `pkg/io/gziputil/gzip.go`
- Test: `pkg/io/gziputil/gzip_test.go`

TS contract (`src/io/GZip.ts`, 33 lines, full source in Task-1 style): `compressGz(src, off, len)` gzips a subarray and **zeroes byte 9** (the gzip OS byte — makes output deterministic across platforms; byte-parity-relevant for B6 pack output); `decompressGz` gunzips a subarray; both return null on error.

- [ ] **Step 1: Write the failing tests**

```go
package gziputil

import (
	"bytes"
	"testing"
)

// TS GZip.ts:3-18: round-trip + the data[9]=0 OS-byte pin.
func TestCompressGzZeroesOSByte(t *testing.T) {
	src := []byte("determinism matters for pack byte-parity")
	gz := CompressGz(src, 0, len(src))
	if gz == nil {
		t.Fatal("CompressGz returned nil")
	}
	if gz[9] != 0 {
		t.Fatalf("gz[9] = %d, want 0 (OS byte must be zeroed)", gz[9])
	}
	back := DecompressGz(gz, 0, len(gz))
	if !bytes.Equal(back, src) {
		t.Fatalf("round-trip = %q, want %q", back, src)
	}
}

// TS GZip.ts: offset/length subarray semantics.
func TestSubarrayOffsets(t *testing.T) {
	padded := append([]byte{0xFF, 0xFF}, []byte("payload")...)
	gz := CompressGz(padded, 2, 7)
	if back := DecompressGz(gz, 0, len(gz)); !bytes.Equal(back, []byte("payload")) {
		t.Fatalf("round-trip = %q", back)
	}
}

// TS GZip.ts:24-31: decompress error -> null.
func TestDecompressGzBadData(t *testing.T) {
	if DecompressGz([]byte{1, 2, 3}, 0, 3) != nil {
		t.Fatal("DecompressGz on garbage must return nil")
	}
}
```

- [ ] **Step 2: Run to verify FAIL** (`go test ./pkg/io/gziputil/...` — compile error)

- [ ] **Step 3: Implement**

```go
// Package gziputil ports src/io/GZip.ts (Engine-TS 9aadcec4): gzip helpers
// with the byte-9 (OS field) zeroing that keeps pack output deterministic.
package gziputil

import (
	"bytes"
	"compress/gzip"
	"io"
)

// CompressGz mirrors TS compressGz (GZip.ts:3-18). Byte 9 of the gzip
// header (OS) is zeroed for deterministic output. Returns nil on error.
func CompressGz(src []byte, off, length int) []byte {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write(src[off : off+length]); err != nil {
		return nil
	}
	if err := zw.Close(); err != nil {
		return nil
	}
	data := buf.Bytes()
	data[9] = 0
	return data
}

// DecompressGz mirrors TS decompressGz (GZip.ts:20-31). Returns nil on error.
func DecompressGz(src []byte, off, length int) []byte {
	zr, err := gzip.NewReader(bytes.NewReader(src[off : off+length]))
	if err != nil {
		return nil
	}
	defer zr.Close()
	out, err := io.ReadAll(zr)
	if err != nil {
		return nil
	}
	return out
}
```

Note: Go's gzip header also differs from Node zlib in the MTIME field (Go writes zeros by default when `Header.ModTime` is unset — verify in the test run; if MTIME bytes 4-7 are nonzero, set `zw.Header.ModTime = time.Time{}`). Node's gzipSync writes MTIME=0. The B6 byte-parity loop is the final arbiter; this task pins gz[9]=0 and adds a `TestHeaderMatchesNodeZlib` if discrepancies appear.

- [ ] **Step 4: PASS + `-race` + vet**
- [ ] **Step 5: Commit** (`feat(io): port 244 GZip helpers with OS-byte zeroing [rev-244 B1]`, same trailer convention as Task 1)

### Task 3: per-deployment PEM token

**Files:**
- Create: `pkg/util/pemtoken/pemtoken.go`
- Test: `pkg/util/pemtoken/pemtoken_test.go`

TS contract (`src/io/PemUtil.ts`, 29 lines): load `data/config/public.pem` RSA public key; token = hex(sha1(n_hex + e_hex + hostname)) where n_hex/e_hex are lowercase hex (forge `BigInteger.toString(16)`); consumed by `src/web.ts` (B3/B5 wiring). Port as a pure function — file loading and wiring land with the consumer.

- [ ] **Step 1: Failing test**

```go
package pemtoken

import "testing"

// TS PemUtil.ts:10-21: sha1 over n-hex + e-hex + hostname, hex-encoded.
// Fixture key: 512-bit RSA generated for this test (committed, test-only).
func TestTokenDeterministic(t *testing.T) {
	const pubPEM = `-----BEGIN PUBLIC KEY-----
MFwwDQYJKoZIhvcNAQEBBQADSwAwSAJBAMSrpdYCNAVhc24z+9oBT2c5lWi42cyk
MAfgrhDIRqMSrzs5Ll1sQE05cAIDk9eMpTHkRYDvJ1WsmYC8M+OqzWMCAwEAAQ==
-----END PUBLIC KEY-----`
	tok1, err := Token([]byte(pubPEM), "hosta")
	if err != nil {
		t.Fatal(err)
	}
	tok2, _ := Token([]byte(pubPEM), "hosta")
	if tok1 != tok2 {
		t.Fatal("token not deterministic")
	}
	if len(tok1) != 40 {
		t.Fatalf("token length = %d, want 40 (sha1 hex)", len(tok1))
	}
	tok3, _ := Token([]byte(pubPEM), "hostb")
	if tok1 == tok3 {
		t.Fatal("token must vary by hostname")
	}
}

func TestTokenBadPEM(t *testing.T) {
	if _, err := Token([]byte("not a pem"), "h"); err == nil {
		t.Fatal("want error for invalid PEM")
	}
}
```

(The fixture PEM above is a placeholder-shaped example — the implementer MUST regenerate a real one: `openssl genrsa 512 | openssl rsa -pubout` and paste the real output; the test asserts determinism/shape, not a magic value, so any valid key works.)

- [ ] **Step 2: FAIL run** (compile error)

- [ ] **Step 3: Implement**

```go
// Package pemtoken ports src/io/PemUtil.ts (Engine-TS 9aadcec4):
// the per-deployment public token = sha1(rsa.N hex + rsa.E hex + hostname).
package pemtoken

import (
	"crypto/rsa"
	"crypto/sha1"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"strconv"
)

// Token mirrors PemUtil.ts:10-21. n and e are lowercase hex with no
// leading zeros (forge BigInteger.toString(16) semantics — big.Int.Text(16)
// matches for positive values).
func Token(pubPEM []byte, hostname string) (string, error) {
	block, _ := pem.Decode(pubPEM)
	if block == nil {
		return "", errors.New("pemtoken: no PEM block")
	}
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return "", fmt.Errorf("pemtoken: %w", err)
	}
	rsaPub, ok := pub.(*rsa.PublicKey)
	if !ok {
		return "", errors.New("pemtoken: not an RSA public key")
	}
	h := sha1.New()
	h.Write([]byte(rsaPub.N.Text(16)))
	h.Write([]byte(strconv.FormatInt(int64(rsaPub.E), 16)))
	h.Write([]byte(hostname))
	return hex.EncodeToString(h.Sum(nil)), nil
}
```

- [ ] **Step 4: PASS + vet**
- [ ] **Step 5: Commit** (`feat(util): port 244 per-deployment PEM token [rev-244 B1]`)

### Task 4: SeqType/AnimFrame restructure

**Files:**
- Modify: `pkg/objtype/seqtype.go` (+ its test)
- Modify/replace: `pkg/objtype/seqframe.go` (+ its test)

Work list = `git -C /home/owner/Code/github.com/LostCityRS/Engine-TS diff e1dea19f..9aadcec4 -- src/cache/config/SeqType.ts src/cache/config/SeqFrame.ts src/cache/graphics/AnimFrame.ts src/cache/graphics/AnimBase.ts` (read it in full before editing). Verified key changes:
- `SeqFrame.ts` **deleted**; delay lookups go through slimmed `AnimFrame.instances` (AnimFrame drops 212 lines of transform parsing, gains a `load()` that reads delays).
- `SeqType` gains `frameCount` field (code 1 reads count into it), new decode codes for `preanim_move`, `postanim_move`, `duplicatebehavior` (exact code numbers and read widths are in the diff past the `replaceheldright` hunk — translate them verbatim), and a `postDecode()` that precomputes `duration`.
- Map goscape names: `pkg/objtype/seqframe.go` is the 225 SeqFrame port — restructure it to mirror the new AnimFrame shape (keep the file if the type keeps living there; match the upstream *semantics*, follow the 244 naming per the adopt-new-names policy in `main:REFERENCES.md`).

- [ ] **Step 1: Read the full TS diff for these four files; write the failing synthetic-packet tests** mirroring the existing `pkg/objtype/seqtype_test.go` style: a test for code-1 `frameCount` population, one per new code (preanim_move/postanim_move/duplicatebehavior) asserting field + read width, one for `postDecode` duration precalc, one for the delay-fallback-to-AnimFrame path.
- [ ] **Step 2: FAIL run** (`go test ./pkg/objtype/ -run 'TestSeq'`)
- [ ] **Step 3: Implement the decode/postDecode/AnimFrame changes** per the diff, citing `// TS SeqType.ts:<lines>` (244 pin).
- [ ] **Step 4: PASS + full-package run** (`go test ./pkg/objtype/...`) — if any pre-existing test pinned the 225 contract (e.g. SeqFrame loading), update it to the 244 contract (verify against TS first — PORTING-LESSONS §3 "a test can pin a bug" applies to revision deltas too: the OLD contract is now the wrong contract on this branch).
- [ ] **Step 5: Commit** (`feat(objtype): 244 SeqType/AnimFrame restructure — frameCount, move anims, duration precalc [rev-244 B1]`)

### Task 5: Component decode delta

**Files:**
- Modify: `pkg/objtype/componenttype.go` (+ test)

Work list = the `src/cache/config/Component.ts` hunk of the cross-pin diff. Verified changes: new `trans` byte after width/height; layer `childCount` g1→**g2**; `operable`→`interactable` and `iop`→`inventoryOptions` renames (adopt the 244 names; goscape exported equivalents rename with them — fix all call sites, this includes the world-handler reads (`modules/world`) which B2 will re-touch); TYPE_RECT/`colour`/`margin` block changes per the diff.

- [ ] **Step 1: Failing tests** (synthetic packets in the existing `componenttype_test.go` style: `trans` byte read, g2 childCount, renamed-field decode for inventory + rect blocks)
- [ ] **Step 2: FAIL run**
- [ ] **Step 3: Implement** per diff with TS citations; `gofmt`; chase the rename compile-cascade across `modules/world` (mechanical — signatures/semantics unchanged this bundle)
- [ ] **Step 4: PASS + `go build ./...`** (whole tree must compile — the renames cascade)
- [ ] **Step 5: Commit** (`feat(objtype): 244 Component decode — trans byte, g2 children, 244 field names [rev-244 B1]`)

### Task 6: NpcType + ObjType decode delta

**Files:**
- Modify: `pkg/objtype/npctype.go`, `pkg/objtype/objtype.go` (+ tests)

Work list = the `NpcType.ts` + `ObjType.ts` hunks. Verified: NpcType new codes 99 (`alwaysontop` flag), 100 (`ambient` g1b), 101 (`contrast` g1b), 102 (`headicon` g2) + 4 new fields; ObjType has a members-gating rework around `NODE_MEMBERS` plus the rest of its 25/8 hunk — read it in full and translate verbatim.

- [ ] **Step 1: Failing tests** (one per new NpcType code asserting field + signedness — 100/101 are g1**b** signed; ObjType per its hunk)
- [ ] **Step 2: FAIL run**
- [ ] **Step 3: Implement** per diff with TS citations
- [ ] **Step 4: PASS + `-race ./pkg/objtype/...`**
- [ ] **Step 5: Commit** (`feat(objtype): 244 NpcType codes 99-102 + ObjType members gating [rev-244 B1]`)

### Task 7: Suite-wide gate + format-window triage

**Files:** none new (possible skip-gating edits to realcache-coupled tests)

- [ ] **Step 1:** `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... 2>&1 | tail -30` — capture the REAL exit code.
- [ ] **Step 2:** Triage failures: legitimate 244-contract updates were handled in Tasks 4-6; anything failing because it loads the 225-format `data/pack` real cache gets the existing skip-when-no-pack gating extended (skip with message `"244 decoder vs 225 cache — until B6 repack"`), each with a `// PORTING-EXCEPTION (rev244-b1-format-window, …)` one-liner. List every gated test in the task report.
- [ ] **Step 3:** `CGO_ENABLED=0 go build -trimpath ./...` + `go vet ./...` green.
- [ ] **Step 4: Commit** (if gating edits were needed): `test: gate 225-cache-coupled tests during the 244 format window [rev-244 B1]`

### Task 8: Tracker rows + B1 correspondence audit

**Files:**
- Modify: `PORTING.md` (on rev-244)

- [ ] **Step 1:** Add a `## rev-244 Bundle audit trail` section to `PORTING.md` with B1 rows:
  - `DoublyLinkList.ts` — NOT-PORTED (dead-at-pin; zero consumers at `9aadcec4`; revisit if a later bundle's TS imports it)
  - `Packet.ts` delta — NO-OP (import moves + bitmask visibility already-unexported in Go)
  - `CrcTable/PreloadedPacks/DevThread` — DEFERRED to B3/B6 (consumer-coupled)
  - format-window gated tests (from Task 7), pointing at B6 for closure
- [ ] **Step 2: Correspondence audit:** for each B1 file, diff-walk the TS hunks vs the Go commits (`git log --oneline rev-225..HEAD`) and confirm every hunk has a Go counterpart or a tracker row. Record the audit line in the same PORTING.md section.
- [ ] **Step 3: Commit** (`docs(porting): rev-244 B1 audit trail — filestream/gzip/pemtoken/config deltas [rev-244 B1]`)

---

## Execution addendum (2026-06-04)

The Task-7 gate surfaced one plan gap: the `src/cache/wordenc/WordEnc.ts` hunk
(load from the raw jag `data/raw/wordenc`, unconditional) was in the bundle's
file inventory but had no task. Closed during execution as Task 6b, commit
`e4eaec54`; recorded in PORTING.md's B1 correspondence table. Also executed
beyond the letter of the plan: the `pkg/pack/clientinterface` writer hunks
(PackShared.ts:267-274,428-431) were pulled forward from B6 in `e4e881d8` to
keep the component round-trip test coherent — B6 must not double-apply them.

## Self-review notes

- Spec coverage: B1 = "new FileStream/GZip/PemUtil, DoublyLinkList, cache-loading rework" — FileStream T1, GZip T2, PemUtil T3, DoublyLinkList resolved as dead-at-pin (T8), cache config deltas T4-T6; the `src/cache` CrcTable/PreloadedPacks/DevThread slice is explicitly deferred with tracker rows (T8) per the consumer-coupling analysis. Packet delta covered (T8 NO-OP row).
- The Task 3 fixture PEM is explicitly marked for regeneration by the implementer (the test asserts shape/determinism, not a magic constant).
- Tasks 4-6 quote the verified hunk semantics and bind the full TS diff as the contract via exact extraction commands — implementers MUST read the diff before editing; reviewers check TS-parity against the cited 244 sources.
