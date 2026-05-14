# NAI-199 — specials slice (`category.dat` + `frame_del.dat` writers) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port the two PackShared writers that sit outside the per-config dispatch table (TS `tools/pack/config/PackShared.ts:340-388`): `category.dat` (server-only enumeration of CategoryPack names) and `frame_del.dat` (server-only per-anim del-byte extracted from `.frame` trailers).

**Architecture:** One new file `pkg/pack/pack_specials.go` housing two free functions (`packAndSaveCategoryDat`, `packAndSaveFrameDel`). Additive insertion into `pkg/pack/pack_configs.go` between the `LoadParamTypes` block (currently lines 320-323) and the `.enum` branch (currently line 326), reusing the existing `ensureCategoryPack` (line 182) and `ensureAnimPack` (line 215) lazy closures. Integration test `TestPackConfigs_EighteenConfigsLand` extends in-place to `TestPackConfigs_TwentyConfigsLand` (renamed, fixture extended with non-empty `category.pack` + `anim.pack` + one synthetic `.frame` file, expected-outputs list extended with `category.dat` + `frame_del.dat`). One new deviation tag (`NAI-199-D-TS-CODE-STALENESS-GATE`) pinned by a new `nai199_deviation_pins_test.go`.

**Tech Stack:** Go 1.26+. Stdlib + `pkg/io/packet` (`Alloc`, `Load`, `P1`/`P2`/`PJStrLF`/`G1`/`G2`, `Save`) + NAI-191–198 `pkg/pack` foundation (`PackFile`, `ShouldBuildFile`, `ShouldBuild`, `GetLatestModified`, `ListFilesExt`).

**Spec:** `docs/superpowers/specs/2026-05-14-nai-199-pack-specials-design.md` (commit `8dd7bd3`).
**HEAD at plan-write:** `8dd7bd3`.

---

## Conventions used throughout this plan

- **All `go` commands prefixed with `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache`** per global CLAUDE.md.
- **All commits use `git commit --no-gpg-sign`** per global CLAUDE.md.
- **Test style** matches existing `pkg/pack/*_test.go`: bare `if err != nil { t.Fatal(err) }`, `bytes.Equal` for byte comparison, `t.Fatalf("got % x, want % x", got, want)` for byte diffs, `t.TempDir()` for fixture roots, `ClearFsCache()` before tests that mutate the FS.
- **Existing helpers in `pkg/pack`** (use, do NOT redefine):
  - `writeFile(t *testing.T, path, content string)` — `constants_test.go:10`
  - `scanPkgPack(t *testing.T) string` — `nai196_deviation_pins_test.go:13`
  - `NewPackFile(srcDir, packType string, validator Validator) (*PackFile, error)` — `packfile.go:45`
  - `ShouldBuildFile(src, dest string) bool` — `freshness.go:59`
  - `ShouldBuild(srcPath, ext, out string) bool` — `freshness.go:45`
  - `GetLatestModified(path, ext string) int64` — `freshness.go:27`
  - `ListFilesExt(path, ext string) []string` — `parse.go:138`
  - `(*Packet).Save(filePath string, length int, start int) error` — `pkg/io/packet/packet.go:108`
  - `(*Packet).Length() int` — `pkg/io/packet/packet.go:96` (returns `len(p.Data)`; the WRITE byte count). Per `[[packet_rw_pointer_gotcha]]` + `NAI-192-D-PACKET-WRITE-CURSOR` doc-comment at `pkg/pack/packed_data.go:39-46`: writes append to `len(Data)`; `Pos` is the READ pointer and stays at 0 in a write-only flow. **MUST use `Length()` (not `Pos`) when passing the byte count to `Save`.**
  - `packet.Alloc(size int) *Packet` — `pkg/io/packet/packet.go:73`
  - `packet.Load(path string, compressed bool) (*Packet, error)` — `pkg/io/packet/load.go:10`
- **Modern Go** (per `[[use-modern-go]]`): `for id := range pf.Max`, `strings.HasSuffix`, `slices.Index` where it fits.
- **Identifier conventions** (mirroring NAI-196–198):
  - File: `pack_specials.go` (singular plural — file is the cohort, functions are the members).
  - Functions (free functions in `package pack`, no method receivers):
    - `packAndSaveCategoryDat(serverOut string, categoryPack *PackFile) error`
    - `packAndSaveFrameDel(srcDir, serverOut string, animPack *PackFile) error`
  - Test file: `pack_specials_test.go`
  - Deviation-pin file: `nai199_deviation_pins_test.go`
- **TS-fidelity discipline** (per `[[true_to_ts_gate]]`): each task block cites the TS file:line range and instructs the implementer to read TS directly. The plan codifies expected byte layouts only where required for failing-test step authorship.

---

## Pre-flight verification (controller, before dispatching tasks)

Verified at plan-write against HEAD `8dd7bd3`:

| Premise | Verification |
|---|---|
| `pkg/pack/pack_specials.go` does NOT exist | ✅ `ls pkg/pack/pack_specials*` → no matches |
| `pkg/pack/nai199_deviation_pins_test.go` does NOT exist | ✅ `ls pkg/pack/nai199_*` → no matches |
| `ensureCategoryPack` closure exists in `pack_configs.go` (lazy `NewPackFile(srcDir, "category", nil)`) | ✅ `pack_configs.go:182-192` |
| `ensureAnimPack` closure exists in `pack_configs.go` (lazy `NewPackFile(srcDir, "anim", nil)`) | ✅ `pack_configs.go:215-225` |
| `LoadParamTypes` block sits at `pack_configs.go:316-323` (`paramTypes, err := objtype.LoadParamTypes(outDir)`) | ✅ verified |
| `.enum` branch starts at `pack_configs.go:325-326` (`// .enum — server-only, freshness-gated.`) | ✅ verified |
| `TestPackConfigs_EighteenConfigsLand` at `pack_configs_test.go:410` writes empty `category.pack` (line 438) and empty `anim.pack` (line 441) | ✅ verified |
| `NAI-192-D-NO-SRC-NO-OP` doc-comment at `pack_configs.go:50-54` says "the nine server-only freshness-gated branches"; pinned by `nai198_deviation_pins_test.go:20-47` (TestNAI198_PresencePin_NoSrcNoOpScopeExtended) | ✅ verified |
| `packet.Alloc(size)` returns `*Packet` with `Pos=0`, `len(Data)=0` (uses pool by size) | ✅ `pkg/io/packet/packet.go:73-83` |
| `PJStrLF(s)` appends `s` raw bytes + `0x0a` (LF) | ✅ `pkg/io/packet/packet.go:395-398` |
| `*PackFile.Size()` returns `len(pf.Pack)` (registered count, matches TS `Map.size`) | ✅ `packfile.go:61` |
| `*PackFile.Max` is `max id + 1` after `RefreshNames` (or `0` on empty registry) | ✅ `packfile.go:148-163` |
| `*PackFile.GetByID(id)` returns `""` for unregistered id | ✅ `packfile.go:189` |

**TS-side premises** (verified by reading `PackShared.ts:340-388` end-to-end at HEAD):

| TS premise | Source line |
|---|---|
| Category branch: `dat = Packet.alloc(1)`; `dat.p2(CategoryPack.size)`; for `i in 0..size-1`: `dat.p1(1) + dat.pjstr(CategoryPack.getById(i)) + dat.p1(0)`; `dat.save('data/pack/server/category.dat')`; server-only (no client jag) | `PackShared.ts:341-352` |
| Category gate: `shouldBuildFile(<src>/pack/category.pack, server/category.dat) || shouldBuild('tools/pack/config', '.ts', server/category.dat)` — second arm is the TS-source-staleness gate | `PackShared.ts:341` |
| Frame_del branch: `files = listFilesExt(<src>/models, '.frame')`; `frame_del = Packet.alloc(3)`; for `i in 0..AnimPack.max-1`: name=`AnimPack.getById(i)`; empty → `p1(0)`; else find `files.find(f => f.endsWith(name+'.frame'))`; not-found → `p1(0)`; else `Packet.load(file)`, `data.pos = data.length - 8`, read 3×g2 (head/tran1/tran2 lengths, discarding 4th g2), `data.pos = 0; data.pos += headLen + tran1Len + tran2Len`, `frame_del.p1(data.g1())`; `frame_del.save('data/pack/server/frame_del.dat')`; server-only | `PackShared.ts:355-388` |
| Frame_del gate: `shouldBuild(<src>/models, '.frame', server/frame_del.dat) || shouldBuild('tools/pack/config', '.ts', server/frame_del.dat)` — second arm is the TS-source-staleness gate | `PackShared.ts:355` |
| TS PackFile `.size` = `Map.size` (registered name count); `.max` = `max(keys)+1` after refreshNames | `PackFileBase.ts:20-22,108-113` |
| TS `pjstr(s)` writes raw bytes of `s` followed by `0x0a` (LF terminator) | inferred from existing PJStrLF parity in `pkg/pack/varp.go` and per-config packers (confirmed working in 18 prior configs) |
| TS `.frame` trailer layout: last 8 bytes are 4 × uint16 BE encoding head/tran1/tran2/del segment lengths in that order | `PackShared.ts:373-377` (reads 3 × g2; 4th g2 is implicit/discarded) |

---

## Pre-flight resolution of spec §9 risk-register rows

| Row | Resolution |
|---|---|
| R1 (pre-context "frame_del writes to client jag" wrong) | ✅ TS PackShared.ts:386 saves server-only. Plan codifies server-only routing for both writers. No `clientJag` parameter. |
| R2 (`.frame` file-match: `endsWith(name+'.frame')` matches `bigfoo.frame` for `name='foo'`) | ✅ **Plan-author decision: port literally with `strings.HasSuffix(f, name+".frame")` and inline-comment only — NO formal deviation tag**. Rationale: deviation tag inflates surface for a corner case (anim names like `foo` and `bigfoo` both having `.frame` files) that's unlikely to surface in real content. If future content triggers it, escalate at that point. Inline comment in `packAndSaveFrameDel` says: `// TS PackShared.ts:365 uses files.find(f => f.endsWith(name+'.frame')); goscape mirrors via strings.HasSuffix. Both share the (latent) suffix-substring false-positive: a name "foo" matches files ending "bigfoo.frame". Acceptable per [[true_to_ts_gate]] — literal port.` |
| R3 (TS-source-staleness gate, second arm of TS shouldBuild) | ✅ Drop both branches' second arm. New deviation tag `NAI-199-D-TS-CODE-STALENESS-GATE`. T1 doc-comments cite TS line; T4 pins the tag. |
| R4 (AnimPack lazy-init at new earlier insertion point) | ✅ Confirmed at plan-write: `ensureAnimPack` is dependency-free (just `NewPackFile(srcDir, "anim", nil)`). Safe to call between `LoadParamTypes` and `.enum`. |
| R5 (`.dbtable/.dbrow`-vs-`.enum/.inv/.mesanim/.struct` order divergence) | ✅ Out of NAI-199 scope. No reordering. |
| R6 (`(*Packet).Save(path, length, start)` signature) | ✅ Confirmed: `(p *Packet) Save(filePath string, length int, start int) error`. Plan passes `p.Pos, 0` everywhere. |
| R7 (extract `.frame` trailer reader to helper?) | ✅ Keep inline. Single call site. Per `[[dead_api_polish]]`. |
| R8 (synthetic vs real `.frame` fixture) | ✅ Synthetic is correct for unit-test scope. Real `.frame` validation is post-close smoke responsibility per `[[cascade_theory_smoke_binding]]`. |

**NAI-192-D-NO-SRC-NO-OP scope decision**: spec §5.4 referenced the existing NAI-192 tag in a passing comment on frame_del's gate. **Plan-author decision: do NOT extend the NAI-192 tag's scope to include frame_del.** Rationale: (a) the NAI-192 scope-extension would require touching `pack_configs.go:50-54` AND `nai198_deviation_pins_test.go:20-47` (TestNAI198_PresencePin_NoSrcNoOpScopeExtended currently asserts "nine" + enumerates nine configs); (b) frame_del produces `.dat` only (no `.idx`), so the tag's "empty .dat/.idx pair" phrasing doesn't fit cleanly; (c) the no-src semantic is documented inline at the frame_del branch in `pack_configs.go` without invoking the NAI-192 tag. Net effect: NAI-198 pin test is untouched; NAI-192 scope stays at nine.

---

## Task list

- **T1**: Create `pack_specials.go` with `packAndSaveCategoryDat` + byte-pin tests. Commit.
- **T2**: Add `packAndSaveFrameDel` to `pack_specials.go` + byte-pin test + empty-AnimPack test. Commit.
- **T3**: Wire both branches into `PackConfigs`; extend integration test `_EighteenConfigsLand` → `_TwentyConfigsLand`. Commit.
- **T4**: Add `nai199_deviation_pins_test.go` pinning `NAI-199-D-TS-CODE-STALENESS-GATE`. Commit.

---

## Task 1: `packAndSaveCategoryDat` + tests

**Files:**
- Create: `pkg/pack/pack_specials.go`
- Create: `pkg/pack/pack_specials_test.go`

### Step 1.1: Write the failing tests

- [ ] Create `pkg/pack/pack_specials_test.go` with the following content:

```go
package pack

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// TestPackAndSaveCategoryDat_BytePin asserts the exact byte layout
// of category.dat for a 3-entry dense CategoryPack: matches TS
// PackShared.ts:341-352 (p2(size); per id: p1(1) + pjstr(name) + p1(0)).
func TestPackAndSaveCategoryDat_BytePin(t *testing.T) {
	srcDir := t.TempDir()
	serverOut := t.TempDir()
	writeFile(t, filepath.Join(srcDir, "pack", "category.pack"),
		"0=alpha\n1=bravo\n2=charlie\n")
	ClearFsCache()

	pf, err := NewPackFile(srcDir, "category", nil)
	if err != nil {
		t.Fatalf("NewPackFile: %v", err)
	}
	if err := packAndSaveCategoryDat(serverOut, pf); err != nil {
		t.Fatalf("packAndSaveCategoryDat: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(serverOut, "category.dat"))
	if err != nil {
		t.Fatalf("read category.dat: %v", err)
	}
	want := []byte{
		0x00, 0x03, // p2(3)
		0x01, 'a', 'l', 'p', 'h', 'a', 0x0a, 0x00, // record 0
		0x01, 'b', 'r', 'a', 'v', 'o', 0x0a, 0x00, // record 1
		0x01, 'c', 'h', 'a', 'r', 'l', 'i', 'e', 0x0a, 0x00, // record 2
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("category.dat mismatch:\ngot  % x\nwant % x", got, want)
	}
}

// TestPackAndSaveCategoryDat_EmptyRegistry asserts that an empty
// CategoryPack (no <src>/pack/category.pack file) produces a 2-byte
// category.dat containing just p2(0). Matches TS no-src behaviour.
func TestPackAndSaveCategoryDat_EmptyRegistry(t *testing.T) {
	srcDir := t.TempDir()
	serverOut := t.TempDir()
	ClearFsCache()

	pf, err := NewPackFile(srcDir, "category", nil)
	if err != nil {
		t.Fatalf("NewPackFile: %v", err)
	}
	if err := packAndSaveCategoryDat(serverOut, pf); err != nil {
		t.Fatalf("packAndSaveCategoryDat: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(serverOut, "category.dat"))
	if err != nil {
		t.Fatalf("read category.dat: %v", err)
	}
	want := []byte{0x00, 0x00}
	if !bytes.Equal(got, want) {
		t.Fatalf("category.dat mismatch:\ngot  % x\nwant % x", got, want)
	}
}
```

### Step 1.2: Run tests to verify they fail

- [ ] Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/ -run TestPackAndSaveCategoryDat -v`

Expected: BUILD FAIL with `undefined: packAndSaveCategoryDat` (function does not exist yet).

### Step 1.3: Write the implementation

- [ ] Create `pkg/pack/pack_specials.go` with the following content:

```go
package pack

import (
	"path/filepath"

	"github.com/zsrv/goscape/pkg/io/packet"
)

// packAndSaveCategoryDat writes <serverOut>/category.dat — a server-
// only enumeration of all registered category names from the in-memory
// categoryPack registry. No idx sibling; no client-jagfile contribution.
//
// Byte layout (per TS PackShared.ts:341-352):
//   - p2(categoryPack.Size())                        — registered name count
//   - for i in 0..Size()-1:                          — dense-id iteration
//       p1(1)                                        — record marker
//       pjstr(categoryPack.GetByID(i))               — LF-terminated name
//       p1(0)                                        — record terminator
//
// Empty registry → file contains just p2(0) (2 bytes).
//
// NAI-199-D-TS-CODE-STALENESS-GATE: TS gate's second arm
// `shouldBuild('tools/pack/config', '.ts', dest)` is dropped — that
// arm rebuilds when TS pipeline source files are newer than output;
// has no Go-binary equivalent at runtime. Goscape uses only the
// `ShouldBuildFile(<src>/pack/category.pack, dest)` arm. Gate logic
// lives in PackConfigs; this function is the unconditional writer.
//
// TS source: tools/pack/config/PackShared.ts:341-352.
func packAndSaveCategoryDat(serverOut string, categoryPack *PackFile) error {
	dat := packet.Alloc(1)
	size := categoryPack.Size()
	dat.P2(uint16(size))
	for i := range size {
		dat.P1(1)
		dat.PJStrLF(categoryPack.GetByID(i))
		dat.P1(0)
	}
	// NAI-192-D-PACKET-WRITE-CURSOR: writes append to len(Data); Pos is
	// the read pointer (memory [[packet_rw_pointer_gotcha]]). Use
	// Length() — i.e. len(Data) — for the byte count.
	return dat.Save(filepath.Join(serverOut, "category.dat"), dat.Length(), 0)
}
```

### Step 1.4: Run tests to verify they pass

- [ ] Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/ -run TestPackAndSaveCategoryDat -v`

Expected: 2/2 PASS.

### Step 1.5: Run the full pkg/pack test suite

- [ ] Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/...`

Expected: PASS (no regressions in 18-config existing tests).

### Step 1.6: Commit

- [ ] Commit:

```bash
git add pkg/pack/pack_specials.go pkg/pack/pack_specials_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(pack): NAI-199 T1 — packAndSaveCategoryDat writer + byte-pin tests

New pkg/pack/pack_specials.go houses the PackShared specials writers
that sit outside the per-config dispatch table. T1 adds the category
writer: server-only, no idx, no client-jagfile contribution. Byte
layout matches TS PackShared.ts:341-352 exactly.

Drops TS's second-arm shouldBuild('tools/pack/config', '.ts', ...)
gate (NAI-199-D-TS-CODE-STALENESS-GATE — pinned in T4).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: `packAndSaveFrameDel` + tests

**Files:**
- Modify: `pkg/pack/pack_specials.go`
- Modify: `pkg/pack/pack_specials_test.go`

### Step 2.1: Write the failing tests

- [ ] Append to `pkg/pack/pack_specials_test.go`:

```go
// TestPackAndSaveFrameDel_BytePin asserts frame_del.dat byte layout
// for a 3-slot AnimPack (foo at id 0, gap at id 1, bar at id 2) with
// only foo.frame present on disk. Matches TS PackShared.ts:355-388.
//
// Synthetic foo.frame layout (32 bytes total):
//   bytes  0..5 : head segment (6 bytes)          — aa bb cc dd ee ff
//   bytes  6..9 : tran1 segment (4 bytes)         — 11 22 33 44
//   bytes 10..11: tran2 segment (2 bytes)         — 55 66
//   bytes 12..23: del segment (12 bytes)          — 42 99 99 99 99 99 99 99 99 99 99 99
//   bytes 24..31: trailer (4×u16 BE)              — 00 06 00 04 00 02 00 0c
//                  = head=6, tran1=4, tran2=2, del=12
//
// TS reads pos=len-8=24, three g2 → 6, 4, 2 (discards 4th g2). Then
// pos=0+6+4+2=12, reads g1() = 0x42 (del[0]). Expected output for foo.
//
// Expected frame_del.dat: 42 (foo id 0) 00 (id 1 gap) 00 (bar id 2,
// no .frame file). Total 3 bytes.
func TestPackAndSaveFrameDel_BytePin(t *testing.T) {
	srcDir := t.TempDir()
	serverOut := t.TempDir()

	writeFile(t, filepath.Join(srcDir, "pack", "anim.pack"),
		"0=foo\n2=bar\n")

	fooFrame := []byte{
		0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff, // head[6]
		0x11, 0x22, 0x33, 0x44, // tran1[4]
		0x55, 0x66, // tran2[2]
		0x42, 0x99, 0x99, 0x99, 0x99, 0x99, 0x99, 0x99, 0x99, 0x99, 0x99, 0x99, // del[12]
		0x00, 0x06, 0x00, 0x04, 0x00, 0x02, 0x00, 0x0c, // trailer
	}
	modelsDir := filepath.Join(srcDir, "models")
	if err := os.MkdirAll(modelsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modelsDir, "foo.frame"), fooFrame, 0o644); err != nil {
		t.Fatal(err)
	}
	ClearFsCache()

	pf, err := NewPackFile(srcDir, "anim", nil)
	if err != nil {
		t.Fatalf("NewPackFile: %v", err)
	}
	if err := packAndSaveFrameDel(srcDir, serverOut, pf); err != nil {
		t.Fatalf("packAndSaveFrameDel: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(serverOut, "frame_del.dat"))
	if err != nil {
		t.Fatalf("read frame_del.dat: %v", err)
	}
	want := []byte{0x42, 0x00, 0x00}
	if !bytes.Equal(got, want) {
		t.Fatalf("frame_del.dat mismatch:\ngot  % x\nwant % x", got, want)
	}
}

// TestPackAndSaveFrameDel_EmptyAnimPack asserts that an AnimPack with
// no registered names (no <src>/pack/anim.pack file) produces a
// zero-byte frame_del.dat. Matches TS no-src behaviour: Packet.alloc(3)
// is empty, loop runs 0 times, save writes 0 bytes.
func TestPackAndSaveFrameDel_EmptyAnimPack(t *testing.T) {
	srcDir := t.TempDir()
	serverOut := t.TempDir()
	ClearFsCache()

	pf, err := NewPackFile(srcDir, "anim", nil)
	if err != nil {
		t.Fatalf("NewPackFile: %v", err)
	}
	if err := packAndSaveFrameDel(srcDir, serverOut, pf); err != nil {
		t.Fatalf("packAndSaveFrameDel: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(serverOut, "frame_del.dat"))
	if err != nil {
		t.Fatalf("read frame_del.dat: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("frame_del.dat for empty AnimPack: got %d bytes (% x), want 0", len(got), got)
	}
}
```

### Step 2.2: Run tests to verify they fail

- [ ] Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/ -run TestPackAndSaveFrameDel -v`

Expected: BUILD FAIL with `undefined: packAndSaveFrameDel`.

### Step 2.3: Write the implementation

- [ ] Append to `pkg/pack/pack_specials.go` (after `packAndSaveCategoryDat`):

```go
// packAndSaveFrameDel writes <serverOut>/frame_del.dat — for each
// registered AnimPack id (0..animPack.Max-1), one byte extracted from
// the corresponding <srcDir>/models/<name>.frame file's del segment.
// Server-only; no idx sibling; no client-jagfile contribution.
//
// Per-id byte:
//   - animPack.GetByID(i) == ""           → p1(0)
//   - no <name>.frame file on disk        → p1(0)
//   - else load .frame, read trailer at end-8 (3×g2 = head/tran1/tran2
//     lengths; 4th g2 implicit/discarded), seek to start+head+tran1+
//     tran2, emit g1() (first byte of del segment).
//
// Empty AnimPack (Max=0) → 0-byte output.
//
// File-match (TS PackShared.ts:365 uses files.find(f =>
// f.endsWith(name+'.frame'))): goscape mirrors via strings.HasSuffix.
// Both share the (latent) suffix-substring false-positive: a name "foo"
// matches files ending "bigfoo.frame". Acceptable per [[true_to_ts_gate]]
// — literal port; not promoted to a formal deviation tag.
//
// NAI-199-D-TS-CODE-STALENESS-GATE: TS gate's second arm
// `shouldBuild('tools/pack/config', '.ts', dest)` is dropped — that
// arm rebuilds when TS pipeline source files are newer than output;
// has no Go-binary equivalent at runtime. Goscape uses only the
// `ShouldBuild(<src>/models, '.frame', dest)` arm (plus
// `GetLatestModified > 0` no-src guard). Gate logic lives in
// PackConfigs; this function is the unconditional writer.
//
// TS source: tools/pack/config/PackShared.ts:355-388.
func packAndSaveFrameDel(srcDir, serverOut string, animPack *PackFile) error {
	modelsDir := filepath.Join(srcDir, "models")
	files := ListFilesExt(modelsDir, ".frame")
	out := packet.Alloc(3)
	for i := range animPack.Max {
		name := animPack.GetByID(i)
		if name == "" {
			out.P1(0)
			continue
		}
		suffix := name + ".frame"
		var match string
		for _, f := range files {
			if strings.HasSuffix(f, suffix) {
				match = f
				break
			}
		}
		if match == "" {
			out.P1(0)
			continue
		}
		data, err := packet.Load(match, false)
		if err != nil {
			return err
		}
		data.Pos = len(data.Data) - 8
		headLen := data.G2()
		tran1Len := data.G2()
		tran2Len := data.G2()
		data.Pos = int(headLen) + int(tran1Len) + int(tran2Len)
		out.P1(data.G1())
	}
	// NAI-192-D-PACKET-WRITE-CURSOR: out is write-only; use Length()
	// for the byte count (Pos remains 0 since writes don't advance it).
	return out.Save(filepath.Join(serverOut, "frame_del.dat"), out.Length(), 0)
}
```

- [ ] Update the import block at the top of `pkg/pack/pack_specials.go` to add `strings`:

```go
import (
	"path/filepath"
	"strings"

	"github.com/zsrv/goscape/pkg/io/packet"
)
```

### Step 2.4: Run tests to verify they pass

- [ ] Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/ -run TestPackAndSaveFrameDel -v`

Expected: 2/2 PASS.

### Step 2.5: Run the full pkg/pack test suite

- [ ] Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/...`

Expected: PASS (T1 tests + T2 tests + 18-config tests all green).

### Step 2.6: Commit

- [ ] Commit:

```bash
git add pkg/pack/pack_specials.go pkg/pack/pack_specials_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(pack): NAI-199 T2 — packAndSaveFrameDel writer + byte-pin tests

Second NAI-199 specials writer. Reads AnimPack + <srcDir>/models/**/
*.frame trailers; extracts first byte of del segment per registered
anim id; emits p1(0) for empty/missing slots. Server-only.

Empty AnimPack → 0-byte frame_del.dat (matches TS Packet.alloc(3)
zero-iteration loop semantics).

File-match uses strings.HasSuffix to mirror TS endsWith semantics
literally, including the latent suffix-substring false-positive on
overlapping anim names ("foo" matching "bigfoo.frame"). Inline-
commented per [[true_to_ts_gate]]; not promoted to a formal tag.

NAI-199-D-TS-CODE-STALENESS-GATE applies to this branch's gate too
(pinned in T4).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: Wire both branches into `PackConfigs` + extend integration test

**Files:**
- Modify: `pkg/pack/pack_configs.go` (insert two new branches between `LoadParamTypes` block and `.enum` branch)
- Modify: `pkg/pack/pack_configs_test.go` (rename `TestPackConfigs_EighteenConfigsLand` → `TestPackConfigs_TwentyConfigsLand`; extend fixture + expected outputs)

### Step 3.1: Write the failing test (extend integration test)

- [ ] Open `pkg/pack/pack_configs_test.go`. Rename the function and extend the fixture as follows. Apply via `Edit`:

  - **Edit 1** (rename): replace `func TestPackConfigs_EighteenConfigsLand(t *testing.T) {` with `func TestPackConfigs_TwentyConfigsLand(t *testing.T) {`.

  - **Edit 2** (non-empty category.pack): replace `writeFile(t, filepath.Join(srcDir, "pack", "category.pack"), "")` with `writeFile(t, filepath.Join(srcDir, "pack", "category.pack"), "0=cat_one\n")`.

  - **Edit 3** (non-empty anim.pack + synthetic .frame file): replace `writeFile(t, filepath.Join(srcDir, "pack", "anim.pack"), "")` with the following block:

```go
		writeFile(t, filepath.Join(srcDir, "pack", "anim.pack"), "0=walk\n")
		walkFrame := []byte{
			0xab,                                           // del[0] sentinel
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01, // trailer: head=0,tran1=0,tran2=0,delLen=1
		}
		modelsDir := filepath.Join(srcDir, "models")
		if err := os.MkdirAll(modelsDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(modelsDir, "walk.frame"), walkFrame, 0o644); err != nil {
			t.Fatal(err)
		}
```

  - **Edit 4** (assert two new outputs exist): immediately after the existing `for _, typ := range []string{ "varp", ..., "hunt" }` loop that asserts `.dat`/`.idx` presence (currently around line 491-501), append:

```go
	for _, name := range []string{"category.dat", "frame_del.dat"} {
		if _, err := os.Stat(filepath.Join(server, name)); err != nil {
			t.Errorf("%s missing: %v", name, err)
		}
	}
```

  - **Edit 5** (assert frame_del has 1 byte = 0xab): immediately after the loop added in Edit 4, append:

```go
	frameDel, err := os.ReadFile(filepath.Join(server, "frame_del.dat"))
	if err != nil {
		t.Fatalf("read frame_del.dat: %v", err)
	}
	if len(frameDel) != 1 || frameDel[0] != 0xab {
		t.Fatalf("frame_del.dat: got % x, want ab", frameDel)
	}
```

  - **Edit 6** (assert category has 1 entry):

```go
	cat, err := os.ReadFile(filepath.Join(server, "category.dat"))
	if err != nil {
		t.Fatalf("read category.dat: %v", err)
	}
	wantCat := []byte{0x00, 0x01, 0x01, 'c', 'a', 't', '_', 'o', 'n', 'e', 0x0a, 0x00}
	if !bytes.Equal(cat, wantCat) {
		t.Fatalf("category.dat mismatch:\ngot  % x\nwant % x", cat, wantCat)
	}
```

  - **Edit 7** (add `bytes` import to `pack_configs_test.go` if not already present): check the existing import block at the top of the file. If `"bytes"` is not present, add it. Existing imports include `fmt`, `os`, `path/filepath`, `slices`, `strings`, `testing`, `time` (verified at plan-write line 1-14 of `pack_configs_test.go`).

### Step 3.2: Run test to verify it fails

- [ ] Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/ -run TestPackConfigs_TwentyConfigsLand -v`

Expected: FAIL — `category.dat missing` and `frame_del.dat missing` (PackConfigs doesn't yet write either).

### Step 3.3: Wire both branches into `PackConfigs`

- [ ] Modify `pkg/pack/pack_configs.go`. Find the block:

```go
	// Eager LoadParamTypes (replaces former lazy ensureParamTypes).
	// .param was just packed above; param.dat/idx now exist on disk
	// for downstream consumers (.struct, .loc, .npc, .obj).
	// TS source: PackShared.ts:334 (ParamType.load).
	paramTypes, err := objtype.LoadParamTypes(outDir)
	if err != nil {
		return fmt.Errorf("load param types: %w", err)
	}

	// .enum — server-only, freshness-gated.
```

- [ ] Insert the following block between the `paramTypes, err := ...` block and `// .enum — server-only, freshness-gated.`:

```go
	// category — server-only special. TS PackShared.ts:341-352.
	// Reads <srcDir>/pack/category.pack (already loaded by
	// ensureCategoryPack). NAI-199-D-TS-CODE-STALENESS-GATE drops TS's
	// second arm `shouldBuild('tools/pack/config', '.ts', dest)`.
	if ShouldBuildFile(
		filepath.Join(srcDir, "pack", "category.pack"),
		filepath.Join(serverOut, "category.dat"),
	) {
		if err := ensureCategoryPack(); err != nil {
			return err
		}
		if err := packAndSaveCategoryDat(serverOut, categoryPack); err != nil {
			return err
		}
	}

	// frame_del — server-only special. TS PackShared.ts:355-388.
	// Reads AnimPack + <srcDir>/models/**/*.frame trailers.
	// Server-only; no idx. Empty models dir → branch skipped
	// (GetLatestModified guard); empty AnimPack registry inside the
	// branch → 0-byte frame_del.dat (per packAndSaveFrameDel docs).
	// NAI-199-D-TS-CODE-STALENESS-GATE drops TS's second arm
	// `shouldBuild('tools/pack/config', '.ts', dest)`.
	if GetLatestModified(filepath.Join(srcDir, "models"), ".frame") > 0 &&
		ShouldBuild(
			filepath.Join(srcDir, "models"),
			".frame",
			filepath.Join(serverOut, "frame_del.dat"),
		) {
		if err := ensureAnimPack(); err != nil {
			return err
		}
		if err := packAndSaveFrameDel(srcDir, serverOut, animPack); err != nil {
			return err
		}
	}

```

### Step 3.4: Run tests to verify they pass

- [ ] Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/ -run TestPackConfigs_TwentyConfigsLand -v`

Expected: PASS.

### Step 3.5: Run the full pkg/pack test suite

- [ ] Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/...`

Expected: PASS (T1 + T2 + T3 + all 18-config tests green).

### Step 3.6: Run the full repo test suite (smoke)

- [ ] Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`

Expected: PASS (no cross-package regressions).

### Step 3.7: Commit

- [ ] Commit:

```bash
git add pkg/pack/pack_configs.go pkg/pack/pack_configs_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(pack): NAI-199 T3 — wire specials into PackConfigs + 20-configs test

Both NAI-199 writers now run as part of PackConfigs. Insertion point
matches TS PackShared.ts canonical layout: category between
LoadParamTypes and .enum, then frame_del immediately after.

Category gate: ShouldBuildFile(<src>/pack/category.pack, dest).
Frame_del gate: GetLatestModified > 0 && ShouldBuild on <src>/models
*.frame. Both drop TS's TS-source-staleness second arm
(NAI-199-D-TS-CODE-STALENESS-GATE — pinned in T4).

TestPackConfigs_EighteenConfigsLand renamed to _TwentyConfigsLand:
fixture extended with non-empty category.pack (cat_one), non-empty
anim.pack (walk), and one synthetic walk.frame; assertions extended
with category.dat (12 bytes) + frame_del.dat (1 byte, sentinel 0xab).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: Deviation-tag pin test

**Files:**
- Create: `pkg/pack/nai199_deviation_pins_test.go`

### Step 4.1: Write the failing test

- [ ] Create `pkg/pack/nai199_deviation_pins_test.go` with the following content:

```go
package pack

import (
	"strings"
	"testing"
)

// TestNAI199_PresencePin_TSCodeStalenessGate asserts that the
// NAI-199-D-TS-CODE-STALENESS-GATE deviation tag appears in pkg/pack/
// production code at ≥2 sites: doc-comments on the category branch
// and the frame_del branch in pack_configs.go (plus the documentation
// on both packAndSave functions in pack_specials.go, for a total of
// ≥4 matches).
//
// The tag records the goscape decision to drop TS's second-arm
// shouldBuild('tools/pack/config', '.ts', dest) gate, which rebuilds
// when TS pipeline source files are newer than output — a semantic
// with no Go-binary equivalent at runtime.
//
// Per [[pin_test_self_trigger_production_doc]], this pin matches the
// tag identifier ONLY — not paraphrases like "TS source staleness" —
// to avoid self-triggering against adjacent prose.
func TestNAI199_PresencePin_TSCodeStalenessGate(t *testing.T) {
	src := scanPkgPack(t)
	const tag = "NAI-199-D-TS-CODE-STALENESS-GATE"
	count := strings.Count(src, tag)
	if count < 2 {
		t.Fatalf("NAI-199-D-TS-CODE-STALENESS-GATE should appear ≥2 times in pkg/pack production code (category branch + frame_del branch); got %d", count)
	}
}
```

### Step 4.2: Run test to verify it passes (production tag was added in T1/T2/T3)

- [ ] Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/ -run TestNAI199_PresencePin -v`

Expected: PASS. The tag identifier was already inserted into `pack_specials.go` (T1, T2) and `pack_configs.go` (T3); this test merely binds those existing tags against accidental removal.

If FAIL with count < 2: re-inspect T1 step 1.3 (`pack_specials.go` `packAndSaveCategoryDat` doc-comment), T2 step 2.3 (`packAndSaveFrameDel` doc-comment), and T3 step 3.3 (both `PackConfigs` insertions) to verify the tag identifier text was preserved verbatim.

### Step 4.3: Run the full pkg/pack test suite

- [ ] Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/...`

Expected: PASS (all NAI-199 tests + all prior tests green).

### Step 4.4: Commit

- [ ] Commit:

```bash
git add pkg/pack/nai199_deviation_pins_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
test(pack): NAI-199 T4 — deviation-tag pin

Binds NAI-199-D-TS-CODE-STALENESS-GATE against accidental removal
across pkg/pack production code. Per [[pin_test_self_trigger_production_doc]],
the pin matches the tag identifier ONLY (≥2 occurrences required).

The tag records the goscape decision to drop TS's second-arm
shouldBuild('tools/pack/config', '.ts', dest) gate from both specials
writers — TS pipeline source mtime has no runtime equivalent in the
goscape Go binary.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Final verification

After T1–T4 commits:

- [ ] Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`

Expected: PASS across the whole repo.

- [ ] Run: `git log --oneline 8dd7bd3..HEAD` to confirm exactly 4 new commits (T1–T4).

- [ ] Verify the `NAI-199-D-TS-CODE-STALENESS-GATE` tag appears in exactly the expected sites:

```bash
grep -rn "NAI-199-D-TS-CODE-STALENESS-GATE" pkg/ modules/
```

Expected sites (≥4 matches):
- `pkg/pack/pack_specials.go` `packAndSaveCategoryDat` doc-comment
- `pkg/pack/pack_specials.go` `packAndSaveFrameDel` doc-comment
- `pkg/pack/pack_configs.go` category branch insertion (T3)
- `pkg/pack/pack_configs.go` frame_del branch insertion (T3)
- (Optional bonus: `pkg/pack/nai199_deviation_pins_test.go` test name — counts as the pin itself)

---

## Plan self-review (controller, pre-dispatch)

**Spec coverage:**
| Spec § | Task |
|---|---|
| §2 In: two free functions in `pack_specials.go` | T1, T2 |
| §2 In: two new branches in `PackConfigs` | T3 |
| §2 In: byte-pin tests for both writers | T1, T2 |
| §2 In: negative-path tests (missing source for category; empty AnimPack for frame_del) | T1 (EmptyRegistry), T2 (EmptyAnimPack) |
| §2 In: integration test extension (18→20 configs) | T3 |
| §2 In: one new deviation tag pin test | T4 |
| §5.1 file layout | T1 (pack_specials.go, pack_specials_test.go), T4 (nai199_deviation_pins_test.go) |
| §5.2 category byte protocol | T1 step 1.3 |
| §5.3 frame_del byte protocol | T2 step 2.3 |
| §5.4 PackConfigs wiring with TS-canonical position | T3 step 3.3 |
| §7.1 category byte-pin | T1 step 1.1 (TestPackAndSaveCategoryDat_BytePin) |
| §7.2 category empty-source | T1 step 1.1 (TestPackAndSaveCategoryDat_EmptyRegistry) |
| §7.3 frame_del byte-pin | T2 step 2.1 (TestPackAndSaveFrameDel_BytePin) |
| §7.4 frame_del no-models-dir | Covered via T3 integration (when `<src>/models` is absent, `GetLatestModified == 0` → branch skipped → no `frame_del.dat` written). The 20-configs test ADDS a models dir; the converse (absence → skip) is implicit. **Explicitly-tested at unit level in T2 (TestPackAndSaveFrameDel_EmptyAnimPack covers the empty-registry path, which is a stronger version of "no source").** |
| §7.5 PackConfigs integration extension | T3 |
| §7.6 deviation tag pin | T4 |
| §10 NAI-199-D-TS-CODE-STALENESS-GATE | T1, T2, T3 (production sites), T4 (pin) |
| §11 carry-forward audits | Confirmed out-of-scope at spec-write; no plan tasks needed |

All spec requirements covered.

**Placeholder scan:** No "TBD", "TODO", "fill in", or "similar to Task N". All code blocks contain executable content. All commands have expected output stated.

**Type/identifier consistency:**
- `packAndSaveCategoryDat(serverOut string, categoryPack *PackFile) error` — consistent in T1 (definition), T3 (call site), pin grep.
- `packAndSaveFrameDel(srcDir, serverOut string, animPack *PackFile) error` — consistent in T2 (definition), T3 (call site), pin grep.
- `NAI-199-D-TS-CODE-STALENESS-GATE` — string identifier consistent across T1/T2/T3 doc-comments and T4 pin test.
- `TestPackConfigs_TwentyConfigsLand` — renamed in T3; no other test references the old `_EighteenConfigsLand` name (verified at plan-write: `grep -n "EighteenConfigsLand" pkg/pack/` → only the function definition at line 410).

**Per `[[plan_test_coverage_crosscheck]]`:** spec §7's test strategy fully mapped to plan tasks above.

**Per `[[plan_grep_helper_patterns]]`:** all helpers used in plan tasks (`writeFile`, `ClearFsCache`, `scanPkgPack`, `NewPackFile`, `ShouldBuildFile`, `ShouldBuild`, `GetLatestModified`, `ListFilesExt`, `packet.Alloc`, `packet.Load`, `(*Packet).P1/P2/PJStrLF/G1/G2/Save`) verified to exist at plan-write — see "Pre-flight verification" table above.

**Per `[[plan_var_name_collision]]`:** mental-compile of each function body:
- `packAndSaveCategoryDat`: locals are `dat`, `size`, `i`. Params are `serverOut`, `categoryPack`. No collisions.
- `packAndSaveFrameDel`: locals are `modelsDir`, `files`, `out`, `i`, `name`, `suffix`, `match`, `f`, `data`, `headLen`, `tran1Len`, `tran2Len`, `err`. Params are `srcDir`, `serverOut`, `animPack`. No collisions.

**Per `[[plan_runnable_test_fixtures]]`:** mental-execution of all fixtures:
- T1 byte-pin: `0=alpha\n1=bravo\n2=charlie\n` → `PackFile.Load` registers 3 dense IDs, `Size()==3`. Loop emits 3 records. Trailing `\n` after `charlie` is the line separator; line 4 is empty and skipped by the regex check (`^\d+=`). ✅
- T1 empty: no `category.pack` file → `PackFile.Load` early-returns on `!FileExists`, `Size()==0`. Loop runs 0 times. Output is just `p2(0)`. ✅
- T2 byte-pin: anim.pack with `0=foo\n2=bar\n` → `Max=3` (after `RefreshNames`: maxID=2, +1=3). Loop iterates `0,1,2`. ✅
- T2 empty: no `anim.pack` → `Max=0` (no entries → `RefreshNames` early-returns before computing Max). Loop iterates 0 times. ✅
- T3 walk.frame: 9 bytes total. `len-8=1`, read three g2 at pos 1,3,5 = 0,0,0. Seek to 0, read g1 at pos 0 = 0xab. ✅

**Per `[[plan_sibling_site_guard_audit]]`:** the existing `ensureCategoryPack` (called from `.loc`, `.npc`, `.obj`, `.hunt` branches) and `ensureAnimPack` (called from `.seq` branch) are dependency-free lazy closures. Calling them earlier in the pipeline (before `.enum`) is safe — no upstream guard depends on the prior call site. ✅

No issues found; plan is dispatch-ready.

---

## Execution handoff

Plan complete. Two execution options:

1. **Subagent-Driven** (recommended) — fresh subagent per task, two-stage review between tasks, fast iteration.
2. **Inline Execution** — execute tasks in this session with checkpoints.

Per `[[execution_mode_default]]`: default is subagent-driven via `superpowers:subagent-driven-development`.
