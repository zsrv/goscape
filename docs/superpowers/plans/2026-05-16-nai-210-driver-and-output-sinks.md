# NAI-210 Driver + File-Output Sinks Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port TS `BytePacket` + three `BinaryOutput` sinks (`BinaryFileScriptWriter` / `JagFileScriptWriter` / `Js5PackScriptWriter`) + `ServerScriptCompiler` driver (`Setup` + `Run`) + `Compile(cfg)` facade + supporting prerequisites (`CompilerTypeInfo`, three `SymbolLoader` impls, `ScriptVarType` enum, `ServerTriggerType` enum, `PrimitiveCategory`); wire feature-gating in `command.RegisterAllDynCommands`; retire 4 deviation tags.

**Architecture:** Single Go struct `runescript.ServerScriptCompiler` flattens TS `ScriptCompiler` + `ServerScriptCompiler` inheritance. The driver wires existing parser / semantics / codegen / pointer-checker / writer packages into a parse → analyze → codegen → check-pointers → write pipeline. Three file-output sinks consume `BinaryScriptWriter` (NAI-209) via the `BinaryOutput` interface. A `runescript.Compile(cfg) error` facade ports the inner logic of TS `ServerScriptCompilerApplication.CompileServerScript`.

**Tech Stack:** Go 1.26+, `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache`, `git commit --no-gpg-sign`. TS pin: `LostCityRS/RuneScriptTS @ b8c338801fbb72d294ff9576a58925a8d3f6de47`.

**Authoritative task numbering:** T1, T2, T3, T4, T5, T6, T7, T8, T9, T10, T11, T12, T13, T14, T15, T16. Per `[[plan_code_block_t_number_drift]]`, all in-file doc comments and commit subjects must use this numbering.

**Spec:** `docs/superpowers/specs/2026-05-16-nai-210-driver-and-output-sinks-design.md` (commit `88286f4`).

**Predecessor:** NAI-209 (`f184492`) — binary writer pipeline.

---

## File Structure

### Created

- `pkg/pack/compiler/runescript/bytepacket.go` — `Crc32` + `ByteWriter` (T1)
- `pkg/pack/compiler/runescript/bytepacket_test.go` (T1)
- `pkg/pack/compiler/runescript/binary_file_writer.go` — per-script-id file sink (T2)
- `pkg/pack/compiler/runescript/binary_file_writer_test.go` (T2)
- `pkg/pack/compiler/runescript/jag_file_writer.go` — `script.dat` + `script.idx` sink (T3)
- `pkg/pack/compiler/runescript/jag_file_writer_test.go` (T3)
- `pkg/pack/compiler/runescript/js5_pack_writer.go` — JS5 archive sink (T4)
- `pkg/pack/compiler/runescript/js5_pack_writer_test.go` (T4)
- `pkg/pack/compiler/type/scriptvar.go` — 41 `ScriptVar*` singletons + `ScriptVarTypeAll` slice (T7)
- `pkg/pack/compiler/type/scriptvar_test.go` (T7)
- `pkg/pack/compiler/trigger/server_trigger_type.go` — ~50 `ServerTrigger*` singletons + `ServerTriggerTypeAll` slice (T8)
- `pkg/pack/compiler/trigger/server_trigger_type_test.go` (T8)
- `pkg/pack/compiler/runescript/compiler_type_info.go` — `CompilerTypeInfo` data struct (T9)
- `pkg/pack/compiler/symbol/loader.go` — `SymbolLoader` interface + `AddConstant`/`AddBasic` helpers + `CompilerContext` interface (T9)
- `pkg/pack/compiler/symbol/loader_test.go` (T9)
- `pkg/pack/compiler/runescript/type_info_loader.go` — three `CompilerTypeInfo*Loader` structs (T10)
- `pkg/pack/compiler/runescript/type_info_loader_test.go` (T10)
- `pkg/pack/compiler/runescript/load_special_symbols.go` — `LoadSpecialSymbols` + `parsePointerList` (T11)
- `pkg/pack/compiler/runescript/load_special_symbols_test.go` (T11)
- `pkg/pack/compiler/runescript/default_type_checkers.go` — 7-checker registration helper (T12)
- `pkg/pack/compiler/runescript/default_type_checkers_test.go` (T12)
- `pkg/pack/compiler/runescript/server_script_compiler.go` — driver struct + `Run` (T12, T14)
- `pkg/pack/compiler/runescript/server_script_compiler_test.go` (T12, T14)
- `pkg/pack/compiler/runescript/setup.go` — `Setup` body + `registerScriptVarTypes` + sym-loader helpers (T13)
- `pkg/pack/compiler/runescript/setup_test.go` (T13)
- `pkg/pack/compiler/runescript/compile.go` — `runescript.Compile(cfg)` facade (T15)
- `pkg/pack/compiler/runescript/compile_test.go` — driver smoke (Jag + Js5) (T15)
- `pkg/pack/compiler/runescript/nai210_deviation_pins_test.go` — pin NAI-210-introduced tags (T16)

### Modified

- `pkg/pack/compiler/type/primitive.go` — add `PrimitiveCategory` + extend `PrimitiveAll` (T5)
- `pkg/pack/compiler/type/primitive_test.go` — assert new entry (T5)
- `pkg/pack/compiler/runescript/binary_writer.go` — `generateLookupKey`: per-subject category check (T5)
- `pkg/pack/compiler/runescript/binary_writer_lookup_test.go` — TYPEMARKER-CATEGORY case lands real (T5)
- `pkg/pack/compiler/runescript/nai209_deviation_pins_test.go` — drop `NAI-209-D-BYTEPACKET-DEFER` + `NAI-209-D-TYPEMARKER-CATEGORY-DISCRIMINATOR` (T16)
- `pkg/pack/compiler/command/register.go` — wire `features.DisableX` gates; drop `_ = features` (T6)
- `pkg/pack/compiler/command/cohort_a_test.go` — add feature-gating positive/negative cases (T6)
- `pkg/pack/compiler/codegen/nai207_deviation_pins_test.go` — drop `NAI-207-D-REGISTERALL-NO-FEATURES` (T16)

### Deviation tags (set in this slice)

| Tag | Origin |
|---|---|
| `NAI-210-D-GZIP-OS-BYTE-ZEROED` | T4 — Go `compress/gzip` writes host OS byte; zero post-write to match TS reproducibility |
| `NAI-210-D-LOADER-SORTED-ITERATION` | T10/T11 — Go map iteration randomized; sort by numeric id for byte-identical `SymbolMapper` |
| `NAI-210-D-EMPTYPOINTERS-RETURNS-FALSE` | T14 — TS-faithful early-return false on empty `commandPointers` |

Retired in T16: `NAI-209-D-BYTEPACKET-DEFER`, `NAI-209-D-TYPEMARKER-CATEGORY-DISCRIMINATOR`, `NAI-208-D-COMMAND-POINTERS-DEFERRED` (if pinned anywhere), `NAI-207-D-REGISTERALL-NO-FEATURES`.

---

## Task T1: BytePacket — `Crc32` + `ByteWriter`

**Files:**
- Create: `pkg/pack/compiler/runescript/bytepacket.go`
- Test: `pkg/pack/compiler/runescript/bytepacket_test.go`

**TS source:** `src/runescript/writer/BytePacket.ts` (87 lines).

- [ ] **Step 1: Write the failing test**

Create `pkg/pack/compiler/runescript/bytepacket_test.go`:

```go
// pkg/pack/compiler/runescript/bytepacket_test.go
package runescript

import (
	"bytes"
	"testing"
)

// TestCrc32_GoldenVectors pins crc32 against known values.
// Verified against TS BytePacket.crc32 output for identical inputs.
func TestCrc32_GoldenVectors(t *testing.T) {
	cases := []struct {
		name string
		in   []byte
		want int32
	}{
		{"empty", []byte{}, 0},
		{"single zero byte", []byte{0x00}, int32(-771559539)},      // 0xD202EF8D as signed
		{"abc", []byte("abc"), int32(0x352441C2)},
		{"binary", []byte{0xDE, 0xAD, 0xBE, 0xEF}, int32(0x7C9CA35A)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Crc32(c.in)
			if got != c.want {
				t.Errorf("Crc32(%q) = 0x%08x, want 0x%08x", c.in, uint32(got), uint32(c.want))
			}
		})
	}
}

func TestByteWriter_P1(t *testing.T) {
	w := NewByteWriter(8)
	w.P1(0x12)
	w.P1(0xFF)
	w.P1(0x00)
	got := w.Bytes()
	want := []byte{0x12, 0xFF, 0x00}
	if !bytes.Equal(got, want) {
		t.Errorf("ByteWriter.P1: got %x, want %x", got, want)
	}
}

func TestByteWriter_P2(t *testing.T) {
	w := NewByteWriter(8)
	w.P2(0x1234)
	w.P2(0xFFFF)
	got := w.Bytes()
	want := []byte{0x12, 0x34, 0xFF, 0xFF}
	if !bytes.Equal(got, want) {
		t.Errorf("ByteWriter.P2: got %x, want %x", got, want)
	}
}

func TestByteWriter_P4(t *testing.T) {
	w := NewByteWriter(8)
	w.P4(0x12345678)
	w.P4(-1)
	got := w.Bytes()
	want := []byte{0x12, 0x34, 0x56, 0x78, 0xFF, 0xFF, 0xFF, 0xFF}
	if !bytes.Equal(got, want) {
		t.Errorf("ByteWriter.P4: got %x, want %x", got, want)
	}
}

// TestByteWriter_PSmart2or4 pins the 32768 boundary.
// TS BytePacket.ts L51-57: <32768 → 2-byte BE; else → 4-byte (value | 0x80000000) BE.
func TestByteWriter_PSmart2or4(t *testing.T) {
	cases := []struct {
		name string
		v    int
		want []byte
	}{
		{"zero", 0, []byte{0x00, 0x00}},
		{"32767 last 2-byte", 32767, []byte{0x7F, 0xFF}},
		{"32768 first 4-byte", 32768, []byte{0x80, 0x00, 0x80, 0x00}},
		{"65536", 65536, []byte{0x80, 0x01, 0x00, 0x00}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			w := NewByteWriter(8)
			w.PSmart2or4(c.v)
			got := w.Bytes()
			if !bytes.Equal(got, c.want) {
				t.Errorf("PSmart2or4(%d): got %x, want %x", c.v, got, c.want)
			}
		})
	}
}

func TestByteWriter_PData(t *testing.T) {
	w := NewByteWriter(4)
	w.PData([]byte{0xAA, 0xBB})
	w.PData([]byte{0xCC, 0xDD, 0xEE, 0xFF})
	got := w.Bytes()
	want := []byte{0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF}
	if !bytes.Equal(got, want) {
		t.Errorf("ByteWriter.PData: got %x, want %x", got, want)
	}
}

// TestByteWriter_GrowsBuffer pins TS ensure() doubling behavior.
func TestByteWriter_GrowsBuffer(t *testing.T) {
	w := NewByteWriter(2)
	for i := 0; i < 100; i++ {
		w.P1(byte(i & 0xff))
	}
	if w.Len() != 100 {
		t.Errorf("Len after 100 P1s: got %d, want 100", w.Len())
	}
}

// TestByteWriter_InitialSizeFloor pins TS L33: max(64, initialSize).
func TestByteWriter_InitialSizeFloor(t *testing.T) {
	w := NewByteWriter(4)
	// Writes up to 64 bytes should not require any growth visible to caller.
	for i := 0; i < 64; i++ {
		w.P1(0xAA)
	}
	if w.Len() != 64 {
		t.Errorf("Len after 64 P1s with initialSize=4: got %d, want 64", w.Len())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/runescript/ -run 'TestCrc32_GoldenVectors|TestByteWriter' -v`
Expected: FAIL — `Crc32` undefined, `NewByteWriter` undefined.

- [ ] **Step 3: Write minimal implementation**

Create `pkg/pack/compiler/runescript/bytepacket.go`:

```go
// pkg/pack/compiler/runescript/bytepacket.go
package runescript

// crcTable is the IEEE 802.3 polynomial table used by Crc32.
// Mirrors TS BytePacket.ts L1-19.
var crcTable = func() [256]uint32 {
	var t [256]uint32
	for b := 0; b < 256; b++ {
		r := uint32(b)
		for bit := 0; bit < 8; bit++ {
			if r&1 == 1 {
				r = (r >> 1) ^ 0xedb88320
			} else {
				r >>= 1
			}
		}
		t[b] = r
	}
	return t
}()

// Crc32 returns the signed-int32 form of ~crc, matching TS BytePacket.crc32
// (BytePacket.ts L21-27).
func Crc32(data []byte) int32 {
	crc := uint32(0xffffffff)
	for _, b := range data {
		crc = (crc >> 8) ^ crcTable[(crc^uint32(b))&0xff]
	}
	return int32(^crc)
}

// ByteWriter is an append-only big-endian byte buffer with a doubling growth
// policy. Mirrors TS BytePacket.ByteWriter (BytePacket.ts L29-87).
type ByteWriter struct {
	buf    []byte
	offset int
}

// NewByteWriter allocates a ByteWriter with an initial capacity of
// max(64, initialSize). Mirrors TS constructor L33.
func NewByteWriter(initialSize int) *ByteWriter {
	if initialSize < 64 {
		initialSize = 64
	}
	return &ByteWriter{buf: make([]byte, initialSize)}
}

// P1 writes one byte. Mirrors TS p1 L37-41.
func (w *ByteWriter) P1(v int) {
	w.ensure(1)
	w.buf[w.offset] = byte(v & 0xff)
	w.offset++
}

// P2 writes a 16-bit big-endian value. Mirrors TS p2 L43-47.
func (w *ByteWriter) P2(v int) {
	w.ensure(2)
	w.buf[w.offset] = byte((v >> 8) & 0xff)
	w.buf[w.offset+1] = byte(v & 0xff)
	w.offset += 2
}

// P4 writes a 32-bit big-endian signed value. Mirrors TS p4 L49-53.
func (w *ByteWriter) P4(v int32) {
	w.ensure(4)
	u := uint32(v)
	w.buf[w.offset] = byte((u >> 24) & 0xff)
	w.buf[w.offset+1] = byte((u >> 16) & 0xff)
	w.buf[w.offset+2] = byte((u >> 8) & 0xff)
	w.buf[w.offset+3] = byte(u & 0xff)
	w.offset += 4
}

// PSmart2or4 writes a value as 2 bytes if <32768, else as 4 bytes with the
// high bit set. Mirrors TS pSmart2or4 L55-61.
func (w *ByteWriter) PSmart2or4(v int) {
	if v < 32768 {
		w.P2(v)
	} else {
		w.P4(int32(uint32(v) | 0x80000000))
	}
}

// PData appends raw bytes. Mirrors TS pdata L63-67.
func (w *ByteWriter) PData(data []byte) {
	w.ensure(len(data))
	copy(w.buf[w.offset:], data)
	w.offset += len(data)
}

// Bytes returns the active prefix of the buffer (no copy). Mirrors TS
// toBuffer L69-71 (which uses Buffer.subarray, also a view).
func (w *ByteWriter) Bytes() []byte {
	return w.buf[:w.offset]
}

// Len returns the current write offset. Test helper; TS has no equivalent.
func (w *ByteWriter) Len() int {
	return w.offset
}

// ensure doubles the underlying buffer until offset+extra fits.
// Mirrors TS ensure L73-86.
func (w *ByteWriter) ensure(extra int) {
	if w.offset+extra <= len(w.buf) {
		return
	}
	nextSize := len(w.buf) * 2
	for w.offset+extra > nextSize {
		nextSize *= 2
	}
	next := make([]byte, nextSize)
	copy(next, w.buf[:w.offset])
	w.buf = next
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/runescript/ -run 'TestCrc32_GoldenVectors|TestByteWriter' -v`
Expected: PASS — all bytepacket tests.

- [ ] **Step 5: Run full suite (regression check)**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`
Expected: PASS (or only pre-existing failures unchanged).

- [ ] **Step 6: Commit**

```bash
git add pkg/pack/compiler/runescript/bytepacket.go pkg/pack/compiler/runescript/bytepacket_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(compiler/runescript): NAI-210 T1 — BytePacket

Ports TS src/runescript/writer/BytePacket.ts: Crc32 + ByteWriter
(P1/P2/P4/PSmart2or4/PData) with doubling-buffer growth.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task T2: BinaryFileScriptWriter

**Files:**
- Create: `pkg/pack/compiler/runescript/binary_file_writer.go`
- Test: `pkg/pack/compiler/runescript/binary_file_writer_test.go`

**TS source:** `src/runescript/writer/BinaryFileScriptWriter.ts` (33 lines).

Verify before writing: read `pkg/pack/compiler/runescript/binary_writer.go` to confirm the `BinaryScriptWriter` struct field name for its `BinaryOutput`. The new sink **embeds** `*BinaryScriptWriter` and **implements** `BinaryOutput`. Constructor wires `bsw.SetOutput(w)` or assigns the field directly per existing NAI-209 surface (grep `OutputScript|BinaryOutput|SetOutput` in `binary_writer.go` to determine).

- [ ] **Step 1: Write the failing test**

Create `pkg/pack/compiler/runescript/binary_file_writer_test.go`:

```go
// pkg/pack/compiler/runescript/binary_file_writer_test.go
package runescript

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zsrv/goscape/pkg/pack/compiler/codegen"
	"github.com/zsrv/goscape/pkg/pack/compiler/symbol"
	"github.com/zsrv/goscape/pkg/pack/compiler/trigger"
	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

// TestBinaryFileScriptWriter_OutputScript_WritesFile pins TS L26-30:
// each script is written to <output>/<id> via fs.writeFileSync.
func TestBinaryFileScriptWriter_OutputScript_WritesFile(t *testing.T) {
	tmp := t.TempDir()
	mapper := NewSymbolMapper(nil)

	// Pre-register a script symbol so SymbolMapper.Get returns a known id.
	procTrig := &trigger.TriggerType{ID: 0, Identifier: "proc", SubjectMode: trigger.ModeName}
	scriptSym := &symbol.ScriptSymbol{
		BaseTriggerType: procTrig,
		BaseName:        "hello",
	}
	mapper.PutScript(42, "[proc,hello]")

	w, err := NewBinaryFileScriptWriter(tmp, mapper, nil)
	if err != nil {
		t.Fatalf("NewBinaryFileScriptWriter: %v", err)
	}
	rs := &codegen.RuneScript{Symbol: scriptSym, Trigger: procTrig}
	data := []byte{0x01, 0x02, 0x03}
	w.OutputScript(rs, data)

	got, err := os.ReadFile(filepath.Join(tmp, "42"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != string(data) {
		t.Errorf("file contents: got %x, want %x", got, data)
	}
}

// TestBinaryFileScriptWriter_RejectsNonDirectory pins TS L21-23 (Go-idiomatic
// error vs TS throw).
func TestBinaryFileScriptWriter_RejectsNonDirectory(t *testing.T) {
	tmp := t.TempDir()
	regularFile := filepath.Join(tmp, "regular")
	if err := os.WriteFile(regularFile, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	mapper := NewSymbolMapper(nil)
	_, err := NewBinaryFileScriptWriter(regularFile, mapper, nil)
	if err == nil {
		t.Fatal("NewBinaryFileScriptWriter on regular file: want error, got nil")
	}
}

// TestBinaryFileScriptWriter_MkdirAll pins TS L17-19: missing dir is created.
func TestBinaryFileScriptWriter_MkdirAll(t *testing.T) {
	tmp := t.TempDir()
	deep := filepath.Join(tmp, "a", "b", "c")
	mapper := NewSymbolMapper(nil)
	_, err := NewBinaryFileScriptWriter(deep, mapper, nil)
	if err != nil {
		t.Fatalf("NewBinaryFileScriptWriter: %v", err)
	}
	info, err := os.Stat(deep)
	if err != nil {
		t.Fatalf("Stat created dir: %v", err)
	}
	if !info.IsDir() {
		t.Error("created path is not a directory")
	}
}
```

The exact `ScriptSymbol` field names and constructor must be re-verified by reading `pkg/pack/compiler/symbol/`. If `ScriptSymbol` isn't constructed that way, adapt the fixture — the test's intent is "any script symbol that SymbolMapper.Get resolves to 42".

- [ ] **Step 2: Run test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/runescript/ -run TestBinaryFileScriptWriter -v`
Expected: FAIL — `NewBinaryFileScriptWriter` undefined.

- [ ] **Step 3: Write minimal implementation**

Create `pkg/pack/compiler/runescript/binary_file_writer.go`:

```go
// pkg/pack/compiler/runescript/binary_file_writer.go
package runescript

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/zsrv/goscape/pkg/pack/compiler/codegen"
	"github.com/zsrv/goscape/pkg/pack/compiler/diagnostics"
	"github.com/zsrv/goscape/pkg/pack/compiler/writer"
)

// BinaryFileScriptWriter writes each script as a single file named after its
// numeric id under output. Mirrors TS
// src/runescript/writer/BinaryFileScriptWriter.ts.
type BinaryFileScriptWriter struct {
	*BinaryScriptWriter
	output string
}

// NewBinaryFileScriptWriter prepares output as a directory and returns a sink
// that satisfies BinaryOutput. Returns error if output exists and is not a
// directory.
func NewBinaryFileScriptWriter(output string, ids writer.IdProvider, diag *diagnostics.Diagnostics) (*BinaryFileScriptWriter, error) {
	if err := os.MkdirAll(output, 0o755); err != nil {
		return nil, fmt.Errorf("BinaryFileScriptWriter: mkdir %s: %w", output, err)
	}
	info, err := os.Stat(output)
	if err != nil {
		return nil, fmt.Errorf("BinaryFileScriptWriter: stat %s: %w", output, err)
	}
	if !info.IsDir() {
		abs, _ := filepath.Abs(output)
		return nil, fmt.Errorf("BinaryFileScriptWriter: %s is not a directory", abs)
	}

	w := &BinaryFileScriptWriter{output: output}
	w.BinaryScriptWriter = NewBinaryScriptWriter(ids, diag)
	w.BinaryScriptWriter.Output = w   // self-route OutputScript through this struct.
	return w, nil
}

// OutputScript writes data to <output>/<id>. Mirrors TS L26-30.
func (w *BinaryFileScriptWriter) OutputScript(script *codegen.RuneScript, data []byte) {
	id := w.IdProvider.Get(script.Symbol)
	path := filepath.Join(w.output, strconv.Itoa(id))
	if err := os.WriteFile(path, data, 0o644); err != nil {
		panic(fmt.Sprintf("BinaryFileScriptWriter: write %s: %v", path, err))
	}
}
```

The `w.BinaryScriptWriter.Output = w` line wires the embedded writer's `BinaryOutput` field to the outer struct. If NAI-209 named that field differently (`Sink` / `Out` / etc.), use the actual field name — read `pkg/pack/compiler/runescript/binary_writer.go` first.

- [ ] **Step 4: Run test to verify it passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/runescript/ -run TestBinaryFileScriptWriter -v`
Expected: PASS.

- [ ] **Step 5: Run full suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add pkg/pack/compiler/runescript/binary_file_writer.go pkg/pack/compiler/runescript/binary_file_writer_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(compiler/runescript): NAI-210 T2 — BinaryFileScriptWriter

Ports TS src/runescript/writer/BinaryFileScriptWriter.ts: per-script-id
file sink with directory-prep and non-directory rejection.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task T3: JagFileScriptWriter

**Files:**
- Create: `pkg/pack/compiler/runescript/jag_file_writer.go`
- Test: `pkg/pack/compiler/runescript/jag_file_writer_test.go`

**TS source:** `src/runescript/writer/JagFileScriptWriter.ts` (85 lines).

- [ ] **Step 1: Write the failing test**

Create `pkg/pack/compiler/runescript/jag_file_writer_test.go`:

```go
// pkg/pack/compiler/runescript/jag_file_writer_test.go
package runescript

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/zsrv/goscape/pkg/pack/compiler/codegen"
	"github.com/zsrv/goscape/pkg/pack/compiler/symbol"
	"github.com/zsrv/goscape/pkg/pack/compiler/trigger"
)

// TestJagFileScriptWriter_HappyPath pins close-time output for ids {0, 2}:
//   - script.idx: P4(3) + P4(len[0]) + P4(0 gap) + P4(len[2])
//   - script.dat: P4(3) + P4(27 version) + data[0] + data[2]
// Per TS L42-72.
func TestJagFileScriptWriter_HappyPath(t *testing.T) {
	tmp := t.TempDir()
	mapper := NewSymbolMapper(nil)

	procTrig := &trigger.TriggerType{ID: 0, Identifier: "proc", SubjectMode: trigger.ModeName}
	for _, id := range []int{0, 2} {
		mapper.PutScript(id, "[proc,s"+itoaTest(id)+"]")
	}

	w, err := NewJagFileScriptWriter(tmp, mapper, nil)
	if err != nil {
		t.Fatalf("NewJagFileScriptWriter: %v", err)
	}
	w.OutputScript(&codegen.RuneScript{
		Symbol:  &symbol.ScriptSymbol{BaseTriggerType: procTrig, BaseName: "s0"},
		Trigger: procTrig,
	}, []byte{0xAA, 0xBB})
	w.OutputScript(&codegen.RuneScript{
		Symbol:  &symbol.ScriptSymbol{BaseTriggerType: procTrig, BaseName: "s2"},
		Trigger: procTrig,
	}, []byte{0xCC, 0xDD, 0xEE})

	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	idx, err := os.ReadFile(filepath.Join(tmp, "script.idx"))
	if err != nil {
		t.Fatal(err)
	}
	wantIdx := []byte{
		0x00, 0x00, 0x00, 0x03, // count = lastID+1 = 3
		0x00, 0x00, 0x00, 0x02, // len[0] = 2
		0x00, 0x00, 0x00, 0x00, // gap at id=1
		0x00, 0x00, 0x00, 0x03, // len[2] = 3
	}
	if !bytes.Equal(idx, wantIdx) {
		t.Errorf("script.idx: got %x, want %x", idx, wantIdx)
	}

	dat, err := os.ReadFile(filepath.Join(tmp, "script.dat"))
	if err != nil {
		t.Fatal(err)
	}
	wantDat := []byte{
		0x00, 0x00, 0x00, 0x03, // count
		0x00, 0x00, 0x00, 0x1B, // version = 27
		0xAA, 0xBB,             // data[0]
		0xCC, 0xDD, 0xEE,       // data[2]
	}
	if !bytes.Equal(dat, wantDat) {
		t.Errorf("script.dat: got %x, want %x", dat, wantDat)
	}
}

// TestJagFileScriptWriter_EmptyClose pins behavior when no scripts written.
// TS L51: lastID = 0 when keys empty → write P4(1) header in each file.
func TestJagFileScriptWriter_EmptyClose(t *testing.T) {
	tmp := t.TempDir()
	mapper := NewSymbolMapper(nil)
	w, err := NewJagFileScriptWriter(tmp, mapper, nil)
	if err != nil {
		t.Fatalf("NewJagFileScriptWriter: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	idx, _ := os.ReadFile(filepath.Join(tmp, "script.idx"))
	dat, _ := os.ReadFile(filepath.Join(tmp, "script.dat"))
	wantIdx := []byte{0x00, 0x00, 0x00, 0x01}                                                      // count = 1 (lastID 0 + 1)
	wantDat := []byte{0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x1B, 0x00, 0x00, 0x00, 0x00, 0x00} // count + version + gap[0]
	if !bytes.Equal(idx, wantIdx) {
		t.Errorf("empty idx: got %x, want %x", idx, wantIdx)
	}
	// The empty-close exact dat layout depends on whether TS writes a gap for id=0.
	// TS L60-66: when buffer is undefined for index i, only idx receives P4(0), dat receives nothing.
	// So wantDat for empty: P4(1) + P4(27) only (8 bytes).
	wantDatActual := []byte{0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x1B}
	if !bytes.Equal(dat, wantDatActual) {
		t.Errorf("empty dat: got %x, want %x", dat, wantDatActual)
	}
	_ = wantDat // silence unused
}

func itoaTest(i int) string {
	if i == 0 { return "0" }
	if i == 2 { return "2" }
	return "x"
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/runescript/ -run TestJagFileScriptWriter -v`
Expected: FAIL — `NewJagFileScriptWriter` undefined.

- [ ] **Step 3: Write minimal implementation**

Create `pkg/pack/compiler/runescript/jag_file_writer.go`:

```go
// pkg/pack/compiler/runescript/jag_file_writer.go
package runescript

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/zsrv/goscape/pkg/pack/compiler/codegen"
	"github.com/zsrv/goscape/pkg/pack/compiler/diagnostics"
	"github.com/zsrv/goscape/pkg/pack/compiler/writer"
)

// jagFileVersion is the .dat header version constant. Mirrors TS L18.
const jagFileVersion = 27

// JagFileScriptWriter buffers scripts in-memory and writes script.dat +
// script.idx at Close. Mirrors TS src/runescript/writer/JagFileScriptWriter.ts.
type JagFileScriptWriter struct {
	*BinaryScriptWriter
	output  string
	buffers map[int][]byte
}

// NewJagFileScriptWriter prepares output as a directory.
func NewJagFileScriptWriter(output string, ids writer.IdProvider, diag *diagnostics.Diagnostics) (*JagFileScriptWriter, error) {
	if err := os.MkdirAll(output, 0o755); err != nil {
		return nil, fmt.Errorf("JagFileScriptWriter: mkdir %s: %w", output, err)
	}
	info, err := os.Stat(output)
	if err != nil {
		return nil, fmt.Errorf("JagFileScriptWriter: stat %s: %w", output, err)
	}
	if !info.IsDir() {
		abs, _ := filepath.Abs(output)
		return nil, fmt.Errorf("JagFileScriptWriter: %s is not a directory", abs)
	}

	w := &JagFileScriptWriter{
		output:  output,
		buffers: map[int][]byte{},
	}
	w.BinaryScriptWriter = NewBinaryScriptWriter(ids, diag)
	w.BinaryScriptWriter.Output = w
	return w, nil
}

// OutputScript stores a copy of data keyed by script id. Mirrors TS L37-41
// (Buffer.from(data) clones the byte slice).
func (w *JagFileScriptWriter) OutputScript(script *codegen.RuneScript, data []byte) {
	id := w.IdProvider.Get(script.Symbol)
	w.buffers[id] = bytes.Clone(data)
}

// Close emits script.dat + script.idx. Mirrors TS L43-72.
func (w *JagFileScriptWriter) Close() error {
	datPath := filepath.Join(w.output, "script.dat")
	idxPath := filepath.Join(w.output, "script.idx")

	dat, err := os.Create(datPath)
	if err != nil {
		return fmt.Errorf("JagFileScriptWriter: create dat: %w", err)
	}
	defer dat.Close()
	idx, err := os.Create(idxPath)
	if err != nil {
		return fmt.Errorf("JagFileScriptWriter: create idx: %w", err)
	}
	defer idx.Close()

	keys := make([]int, 0, len(w.buffers))
	for k := range w.buffers {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	lastID := 0
	if len(keys) > 0 {
		lastID = keys[len(keys)-1]
	}

	// P4(lastID+1) header in both files.
	if err := writeBE4(dat, int32(lastID+1)); err != nil {
		return err
	}
	if err := writeBE4(idx, int32(lastID+1)); err != nil {
		return err
	}
	// dat-only version header.
	if err := writeBE4(dat, int32(jagFileVersion)); err != nil {
		return err
	}

	for i := 0; i <= lastID; i++ {
		buf, ok := w.buffers[i]
		if !ok {
			// gap
			if err := writeBE4(idx, 0); err != nil {
				return err
			}
			continue
		}
		if err := writeBE4(idx, int32(len(buf))); err != nil {
			return err
		}
		if _, err := dat.Write(buf); err != nil {
			return err
		}
	}
	return nil
}

// writeBE4 writes a 32-bit big-endian value to f.
func writeBE4(f *os.File, v int32) error {
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], uint32(v))
	_, err := f.Write(b[:])
	return err
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/runescript/ -run TestJagFileScriptWriter -v`
Expected: PASS.

- [ ] **Step 5: Run full suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add pkg/pack/compiler/runescript/jag_file_writer.go pkg/pack/compiler/runescript/jag_file_writer_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(compiler/runescript): NAI-210 T3 — JagFileScriptWriter

Ports TS src/runescript/writer/JagFileScriptWriter.ts: buffers scripts by
id and emits script.dat + script.idx at Close with version=27.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task T4: Js5PackScriptWriter (with GZIP-OS-byte-zero deviation pin)

**Files:**
- Create: `pkg/pack/compiler/runescript/js5_pack_writer.go`
- Test: `pkg/pack/compiler/runescript/js5_pack_writer_test.go`

**TS source:** `src/runescript/writer/Js5PackScriptWriter.ts` (153 lines).

- [ ] **Step 1: Write the failing test**

Create `pkg/pack/compiler/runescript/js5_pack_writer_test.go`:

```go
// pkg/pack/compiler/runescript/js5_pack_writer_test.go
package runescript

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/zsrv/goscape/pkg/pack/compiler/codegen"
	"github.com/zsrv/goscape/pkg/pack/compiler/symbol"
	"github.com/zsrv/goscape/pkg/pack/compiler/trigger"
)

// TestJs5PackScriptWriter_HappyPath pins file existence + GZIP byte-9 zero.
func TestJs5PackScriptWriter_HappyPath(t *testing.T) {
	tmp := t.TempDir()
	out := filepath.Join(tmp, "pack.js5")
	mapper := NewSymbolMapper(nil)
	procTrig := &trigger.TriggerType{ID: 0, Identifier: "proc", SubjectMode: trigger.ModeName}
	mapper.PutScript(0, "[proc,s0]")
	mapper.PutScript(1, "[proc,s1]")

	w, err := NewJs5PackScriptWriter(out, mapper, nil)
	if err != nil {
		t.Fatalf("NewJs5PackScriptWriter: %v", err)
	}
	w.OutputScript(&codegen.RuneScript{
		Symbol: &symbol.ScriptSymbol{BaseTriggerType: procTrig, BaseName: "s0"}, Trigger: procTrig,
	}, []byte{0xAA, 0xBB})
	w.OutputScript(&codegen.RuneScript{
		Symbol: &symbol.ScriptSymbol{BaseTriggerType: procTrig, BaseName: "s1"}, Trigger: procTrig,
	}, []byte{0xCC, 0xDD, 0xEE})
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	body, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(body) < 20 {
		t.Fatalf(".js5 too short: %d bytes", len(body))
	}

	// First byte: compression type = 2 (GZIP) for the packed-index group.
	if body[0] != 0x02 {
		t.Errorf("packed index group compression byte: got %d, want 2", body[0])
	}

	// NAI-210-D-GZIP-OS-BYTE-ZEROED: GZIP magic 0x1F 0x8B starts at offset 9
	// (after compression=1 + compressedLen=4 + uncompressedLen=4 = 9 bytes
	// of header). OS byte sits at offset 9 + 9 = 18.
	// TS sets `compressed[9] = 0` BEFORE the prefix is written, so the
	// raw-gzip OS byte at gzip-offset 9 (file offset 18) must be zero.
	if body[18] != 0x00 {
		t.Errorf("GZIP OS byte at offset 18: got 0x%02x, want 0x00 (NAI-210-D-GZIP-OS-BYTE-ZEROED)", body[18])
	}
}

// TestJs5PackScriptWriter_MissingParentDirIsCreated pins TS L33-37.
func TestJs5PackScriptWriter_MissingParentDirIsCreated(t *testing.T) {
	tmp := t.TempDir()
	deep := filepath.Join(tmp, "a", "b", "pack.js5")
	mapper := NewSymbolMapper(nil)
	w, err := NewJs5PackScriptWriter(deep, mapper, nil)
	if err != nil {
		t.Fatalf("NewJs5PackScriptWriter: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := os.Stat(deep); err != nil {
		t.Errorf("output not created: %v", err)
	}
}

// TestJs5PackScriptWriter_OutputScript_ClonesData pins retain-semantics.
// TS L46-49: Buffer.from(data) is a copy; mutating the input afterward must
// not affect what gets written.
func TestJs5PackScriptWriter_OutputScript_ClonesData(t *testing.T) {
	tmp := t.TempDir()
	out := filepath.Join(tmp, "pack.js5")
	mapper := NewSymbolMapper(nil)
	procTrig := &trigger.TriggerType{ID: 0, Identifier: "proc", SubjectMode: trigger.ModeName}
	mapper.PutScript(0, "[proc,s0]")
	w, err := NewJs5PackScriptWriter(out, mapper, nil)
	if err != nil {
		t.Fatalf("NewJs5PackScriptWriter: %v", err)
	}
	data := []byte{0x11, 0x22, 0x33}
	w.OutputScript(&codegen.RuneScript{
		Symbol: &symbol.ScriptSymbol{BaseTriggerType: procTrig, BaseName: "s0"}, Trigger: procTrig,
	}, data)
	data[0] = 0xFF // mutate after handoff
	stored := w.buffers[0]
	if stored[0] != 0x11 {
		t.Errorf("OutputScript did not clone: stored[0]=0x%02x, want 0x11", stored[0])
	}
	_ = bytes.Equal // keep import
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/runescript/ -run TestJs5PackScriptWriter -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Create `pkg/pack/compiler/runescript/js5_pack_writer.go`:

```go
// pkg/pack/compiler/runescript/js5_pack_writer.go
package runescript

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/zsrv/goscape/pkg/pack/compiler/codegen"
	"github.com/zsrv/goscape/pkg/pack/compiler/diagnostics"
	"github.com/zsrv/goscape/pkg/pack/compiler/writer"
)

// JS5 constants. Mirror TS L25-27.
const (
	js5IndexFormat  = 7
	js5IndexVersion = 1
	js5GroupVersion = 1
)

type js5CompressionType int

const (
	js5CompressionNone  js5CompressionType = 0
	js5CompressionBzip2 js5CompressionType = 1
	js5CompressionGzip  js5CompressionType = 2
)

// Js5PackScriptWriter packs scripts into a complete sequential .js5 archive.
// Mirrors TS src/runescript/writer/Js5PackScriptWriter.ts.
type Js5PackScriptWriter struct {
	*BinaryScriptWriter
	output  string
	buffers map[int][]byte
}

// NewJs5PackScriptWriter prepares filepath.Dir(output) as a directory.
func NewJs5PackScriptWriter(output string, ids writer.IdProvider, diag *diagnostics.Diagnostics) (*Js5PackScriptWriter, error) {
	dir := filepath.Dir(output)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("Js5PackScriptWriter: mkdir %s: %w", dir, err)
	}
	info, err := os.Stat(dir)
	if err != nil {
		return nil, fmt.Errorf("Js5PackScriptWriter: stat %s: %w", dir, err)
	}
	if !info.IsDir() {
		abs, _ := filepath.Abs(dir)
		return nil, fmt.Errorf("Js5PackScriptWriter: %s is not a directory", abs)
	}

	w := &Js5PackScriptWriter{
		output:  output,
		buffers: map[int][]byte{},
	}
	w.BinaryScriptWriter = NewBinaryScriptWriter(ids, diag)
	w.BinaryScriptWriter.Output = w
	return w, nil
}

// OutputScript stores a copy of data keyed by script id. Mirrors TS L45-49.
func (w *Js5PackScriptWriter) OutputScript(script *codegen.RuneScript, data []byte) {
	id := w.IdProvider.Get(script.Symbol)
	w.buffers[id] = bytes.Clone(data)
}

type js5Group struct {
	groupID     int
	packedGroup []byte
	checksum    int32
	version     int32
}

// Close emits the JS5 archive. Mirrors TS L51-78.
func (w *Js5PackScriptWriter) Close() error {
	keys := make([]int, 0, len(w.buffers))
	for k := range w.buffers {
		keys = append(keys, k)
	}
	sort.Ints(keys)

	groups := make([]js5Group, 0, len(keys))
	for _, id := range keys {
		scriptData := w.buffers[id]
		packed, err := w.packGroup(scriptData, js5CompressionNone)
		if err != nil {
			return err
		}
		groups = append(groups, js5Group{
			groupID:     id,
			packedGroup: packed,
			checksum:    Crc32(packed),
			version:     js5GroupVersion,
		})
	}

	indexData := w.encodeIndex(groups)
	packedIndex, err := w.packGroup(indexData, js5CompressionGzip)
	if err != nil {
		return err
	}

	f, err := os.Create(w.output)
	if err != nil {
		return fmt.Errorf("Js5PackScriptWriter: create %s: %w", w.output, err)
	}
	defer f.Close()

	if _, err := f.Write(packedIndex); err != nil {
		return err
	}
	for _, g := range groups {
		if _, err := f.Write(g.packedGroup); err != nil {
			return err
		}
	}
	for _, g := range groups {
		if err := writeBE4(f, int32(len(g.packedGroup))); err != nil {
			return err
		}
	}
	return nil
}

// encodeIndex serialises the JS5 index group. Mirrors TS L80-112.
func (w *Js5PackScriptWriter) encodeIndex(groups []js5Group) []byte {
	bw := NewByteWriter(128)
	bw.P1(js5IndexFormat)
	bw.P4(js5IndexVersion)
	bw.P1(0) // flags: no names / digests / lengths / uncompressed checksums.
	bw.PSmart2or4(len(groups))

	previousGroupID := 0
	for _, g := range groups {
		bw.PSmart2or4(g.groupID - previousGroupID)
		previousGroupID = g.groupID
	}
	for _, g := range groups {
		bw.P4(g.checksum)
	}
	for _, g := range groups {
		bw.P4(g.version)
	}
	for range groups {
		bw.PSmart2or4(1) // one file per group
	}
	for range groups {
		bw.PSmart2or4(0) // single file id (0), delta-encoded
	}
	return bw.Bytes()
}

// packGroup wraps src with the JS5 compression prefix. Mirrors TS L114-138.
// Returns error on unsupported compression type.
func (w *Js5PackScriptWriter) packGroup(src []byte, compression js5CompressionType) ([]byte, error) {
	bw := NewByteWriter(len(src) + 16)
	bw.P1(int(compression))

	switch compression {
	case js5CompressionNone:
		bw.P4(int32(len(src)))
		bw.PData(src)
	case js5CompressionGzip:
		var buf bytes.Buffer
		gz := gzip.NewWriter(&buf)
		if _, err := gz.Write(src); err != nil {
			return nil, err
		}
		if err := gz.Close(); err != nil {
			return nil, err
		}
		compressed := buf.Bytes()
		// NAI-210-D-GZIP-OS-BYTE-ZEROED: TS BytePacket comment in
		// Js5PackScriptWriter.ts L125: `compressed[9] = 0;`. Go
		// compress/gzip writes the host OS byte at offset 9 of the gzip
		// stream; zero it for byte-identical reproducibility with TS.
		if len(compressed) > 9 {
			compressed[9] = 0
		}
		bw.P4(int32(len(compressed)))
		bw.P4(int32(len(src)))
		bw.PData(compressed)
	default:
		return nil, fmt.Errorf("Js5PackScriptWriter: unsupported compression type %d", compression)
	}
	return bw.Bytes(), nil
}
```

The standard library `compress/gzip.NewWriter` writes a 10-byte fixed header where byte 9 is the OS byte. Linux gzip writes `0x03`, macOS `0x07`, etc. Zeroing matches TS reproducibility.

- [ ] **Step 4: Run test to verify it passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/runescript/ -run TestJs5PackScriptWriter -v`
Expected: PASS — including the byte-18 zero pin.

- [ ] **Step 5: Run full suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add pkg/pack/compiler/runescript/js5_pack_writer.go pkg/pack/compiler/runescript/js5_pack_writer_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(compiler/runescript): NAI-210 T4 — Js5PackScriptWriter

Ports TS src/runescript/writer/Js5PackScriptWriter.ts: packed JS5 archive
sink with delta-encoded group index, CRC32 + version trailers, and
NAI-210-D-GZIP-OS-BYTE-ZEROED for reproducible GZIP output.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task T5: `PrimitiveCategory` + lookup-key per-subject category check

**Files:**
- Modify: `pkg/pack/compiler/type/primitive.go`
- Modify: `pkg/pack/compiler/type/primitive_test.go`
- Modify: `pkg/pack/compiler/runescript/binary_writer.go`
- Modify: `pkg/pack/compiler/runescript/binary_writer_lookup_test.go`

**TS source:** `src/runescript/type/ScriptVarType.ts` L36 (`CATEGORY = new ScriptVarType('y', BaseVarType.INTEGER, -1, 'category')`).

Note: This anticipates T7's `ScriptVarType` port. T5 ports JUST `PrimitiveCategory` as a primitive singleton so the lookup-key fix can land without waiting for the full enum. The TS ScriptVarType.CATEGORY entry has identical semantics to a primitive (code `'y'`, base `INTEGER`, default `-1`, name `category`); we ship it as `PrimitiveCategory`. The remaining ScriptVarType entries land in T7 as `*ScriptVarType` instances.

- [ ] **Step 1: Write the failing test (extend `primitive_test.go`)**

Read current `pkg/pack/compiler/type/primitive_test.go::TestPrimitiveAll_Order`. Add a new test case AND extend `want` to include `category`:

```go
// pkg/pack/compiler/type/primitive_test.go — extend TestPrimitiveAll_Order

// (current 'want' slice contains 7 entries; extend to 8 with "category".)
// Test will fail until PrimitiveCategory is added to PrimitiveAll.
```

Replace the existing `want` literal in `TestPrimitiveAll_Order` to be:

```go
want := []string{
    "int", "boolean", "coord", "string", "char", "long", "mapzone", "category",
}
```

Then add:

```go
// TestPrimitiveCategory_Code pins the TS ScriptVarType.CATEGORY code.
func TestPrimitiveCategory_Code(t *testing.T) {
	code, ok := PrimitiveCategory.Code()
	if !ok || code != "y" {
		t.Errorf("PrimitiveCategory.Code() = (%q, %v), want (\"y\", true)", code, ok)
	}
	if got := PrimitiveCategory.Representation(); got != "category" {
		t.Errorf("PrimitiveCategory.Representation() = %q, want \"category\"", got)
	}
}
```

For `binary_writer_lookup_test.go::TestLookupKey_TypeMode_Category` — read the file, then replace the existing fixture (which currently uses `tm.Category`) with one that uses `subject.Type = typ.PrimitiveCategory`:

```go
// TestLookupKey_TypeMode_Category pins the per-subject category check.
// Replaces NAI-209-D-TYPEMARKER-CATEGORY-DISCRIMINATOR's per-trigger gate.
func TestLookupKey_TypeMode_Category(t *testing.T) {
	tm := trigger.NewTypeMode(true /* AllowedTypes irrelevant for this test */)
	tr := &trigger.TriggerType{ID: 5, Identifier: "opheld1", SubjectMode: tm}
	categorySubject := &symbol.BasicSymbol{Name: "wooden_bowls", Type: typ.PrimitiveCategory}
	bsw := NewBinaryScriptWriter(stubIdProvider{42}, nil)
	rs := &codegen.RuneScript{
		Trigger:          tr,
		SubjectReference: categorySubject,
	}
	got := bsw.generateLookupKey(rs)

	// Expected: typeMarker = 1 (category arm fires for THIS subject), subjectId = 42 (via IdProvider.Get).
	// key = trigger.ID(5) + (1<<8) + (42<<10) = 5 + 256 + 43008 = 43269.
	want := int32(5 + (1 << 8) + (42 << 10))
	if got != want {
		t.Errorf("generateLookupKey for category subject: got %d, want %d", got, want)
	}
}
```

The exact `trigger.NewTypeMode` constructor signature must be verified by reading `pkg/pack/compiler/trigger/subjectmode.go`. The point of the test is "subject's type IS PrimitiveCategory regardless of what the TypeMode says".

- [ ] **Step 2: Run test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/type/ ./pkg/pack/compiler/runescript/ -run 'TestPrimitive|TestLookupKey_TypeMode' -v`
Expected: FAIL — `PrimitiveCategory` undefined, lookup test asserts new semantics.

- [ ] **Step 3a: Add `PrimitiveCategory` to `primitive.go`**

In `pkg/pack/compiler/type/primitive.go`, edit the `var (...)` singleton block to add:

```go
	PrimitiveCategory = newPrimitiveType("CATEGORY", "y", BaseVarInteger, -1)
```

Then extend the `PrimitiveAll` slice:

```go
var PrimitiveAll = []*PrimitiveType{
	PrimitiveInt, PrimitiveBoolean, PrimitiveCoord, PrimitiveString,
	PrimitiveChar, PrimitiveLong, PrimitiveMapzone, PrimitiveCategory,
}
```

- [ ] **Step 3b: Update `generateLookupKey` in `binary_writer.go`**

In `pkg/pack/compiler/runescript/binary_writer.go`, replace the `var typeMarker int32 = 2 / if tm.Category { typeMarker = 1 }` block with a per-subject check:

```go
	subject, ok := script.SubjectReference.(symbol.Symbol)
	if !ok {
		panic(fmt.Sprintf("BinaryScriptWriter: SubjectReference %T is not a symbol.Symbol", script.SubjectReference))
	}
	subjectId := b.resolveSubjectId(subject)
	var typeMarker int32 = 2
	// Per-subject category check; mirrors TS L80
	// `subjectType === ScriptVarType.CATEGORY`.
	if bs, ok := subject.(*symbol.BasicSymbol); ok && bs.Type == typ.PrimitiveCategory {
		typeMarker = 1
	}
	key += (typeMarker << 8) + (subjectId << 10)
	return key
```

Delete the `tm` variable from the surrounding code if it becomes unused after the change. If `tm.Category` was the only reason `tm` was obtained, the `tm, ok := trigger.IsTypeMode(...)` line should be replaced with a discard form: `if _, ok := trigger.IsTypeMode(...); !ok || script.SubjectReference == nil { return key }`.

Update the doc-comment block to delete the `NAI-209-D-TYPEMARKER-CATEGORY-DISCRIMINATOR` paragraph and replace with:

```go
// Per-subject category check: TS L80 reads
// `subjectType === ScriptVarType.CATEGORY` (now goscape's typ.PrimitiveCategory).
// typeMarker = 1 when the subject's runtime type is the category primitive;
// otherwise 2.
```

- [ ] **Step 4: Run test to verify it passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/type/ ./pkg/pack/compiler/runescript/ -v`
Expected: PASS — `TestPrimitiveAll_Order`, `TestPrimitiveCategory_Code`, `TestLookupKey_TypeMode_Category`.

- [ ] **Step 5: Run full suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`
Expected: PASS (or only pre-existing failures unchanged).

- [ ] **Step 6: Commit**

```bash
git add pkg/pack/compiler/type/primitive.go pkg/pack/compiler/type/primitive_test.go pkg/pack/compiler/runescript/binary_writer.go pkg/pack/compiler/runescript/binary_writer_lookup_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(compiler): NAI-210 T5 — PrimitiveCategory + per-subject lookup-key check

Adds PrimitiveCategory primitive singleton (TS ScriptVarType.CATEGORY,
code 'y'). generateLookupKey now reads subject.Type ==
typ.PrimitiveCategory rather than the per-trigger TypeMode.Category flag,
matching TS BinaryScriptWriter L80.

Retires NAI-209-D-TYPEMARKER-CATEGORY-DISCRIMINATOR (pin updated in T16).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task T6: Feature-gating in `command/register.go`

**Files:**
- Modify: `pkg/pack/compiler/command/register.go`
- Modify: `pkg/pack/compiler/command/cohort_a_test.go`

**TS source:** `src/runescript/ServerScriptCompiler.ts` L84-212 (the per-feature gates inside `setup()`).

- [ ] **Step 1: Write failing tests (extend `cohort_a_test.go`)**

Read `cohort_a_test.go` to identify the test helper that exercises `RegisterAllDynCommands`. Append:

```go
// TestRegisterAll_QueueTypedDisabled pins TS L95-102 + L108-110 + L137-145 +
// L154-160 + L162-166 + L168-172 — gated on features.queueTyped.
func TestRegisterAll_QueueTypedDisabled(t *testing.T) {
	tm := newTestTypeManager(t)
	handlers := map[string]semantics.DynamicCommandHandler{}
	register := func(name string, h semantics.DynamicCommandHandler) { handlers[name] = h }
	RegisterAllDynCommands(tm, semantics.StrictFeatureLevel{DisableQueueTyped: true}, register)

	disallowed := []string{
		"queue*", ".queue*", "weakqueue*", ".weakqueue*",
		"strongqueue*", ".strongqueue*", "longqueue*", ".longqueue*",
	}
	for _, name := range disallowed {
		if _, present := handlers[name]; present {
			t.Errorf("DisableQueueTyped: handler %q should NOT be registered", name)
		}
	}
	// Positive: non-typed variants still present.
	for _, name := range []string{"queue", ".queue", "longqueue", "settimer"} {
		if _, present := handlers[name]; !present {
			t.Errorf("DisableQueueTyped: handler %q should still be registered", name)
		}
	}
}

func TestRegisterAll_EnumsDisabled(t *testing.T) {
	tm := newTestTypeManager(t)
	handlers := map[string]semantics.DynamicCommandHandler{}
	register := func(name string, h semantics.DynamicCommandHandler) { handlers[name] = h }
	RegisterAllDynCommands(tm, semantics.StrictFeatureLevel{DisableEnums: true}, register)
	if _, present := handlers["enum"]; present {
		t.Error("DisableEnums: 'enum' handler should NOT be registered")
	}
}

func TestRegisterAll_StructsDisabled(t *testing.T) {
	tm := newTestTypeManager(t)
	handlers := map[string]semantics.DynamicCommandHandler{}
	register := func(name string, h semantics.DynamicCommandHandler) { handlers[name] = h }
	RegisterAllDynCommands(tm, semantics.StrictFeatureLevel{DisableStructs: true}, register)
	if _, present := handlers["struct_param"]; present {
		t.Error("DisableStructs: 'struct_param' handler should NOT be registered")
	}
}

func TestRegisterAll_DBTablesDisabled(t *testing.T) {
	tm := newTestTypeManager(t)
	handlers := map[string]semantics.DynamicCommandHandler{}
	register := func(name string, h semantics.DynamicCommandHandler) { handlers[name] = h }
	RegisterAllDynCommands(tm, semantics.StrictFeatureLevel{DisableDBTables: true}, register)
	for _, name := range []string{"db_find", "db_find_refine", "db_find_with_count", "db_find_refine_with_count", "db_getfield"} {
		if _, present := handlers[name]; present {
			t.Errorf("DisableDBTables: %q handler should NOT be registered", name)
		}
	}
}

func TestRegisterAll_AllEnabled_GatesAllRegister(t *testing.T) {
	tm := newTestTypeManager(t)
	handlers := map[string]semantics.DynamicCommandHandler{}
	register := func(name string, h semantics.DynamicCommandHandler) { handlers[name] = h }
	RegisterAllDynCommands(tm, semantics.StrictFeatureLevel{}, register)
	for _, name := range []string{
		"queue*", ".queue*", "longqueue*", "enum", "struct_param", "db_find", "db_getfield",
	} {
		if _, present := handlers[name]; !present {
			t.Errorf("default features: %q handler should be registered", name)
		}
	}
}
```

`newTestTypeManager` is a helper that pre-registers `queue` / `timer` / `softtimer` MetaScript types so `RegisterAllDynCommands` can resolve them via `FindOrNil`. Read `cohort_a_test.go` for the existing helper; if absent, add at the top:

```go
func newTestTypeManager(t *testing.T) *typ.TypeManager {
	t.Helper()
	tm := typ.NewTypeManager()
	if err := tm.Register("queue", typ.NewMetaScript("queue", typ.MetaAny, typ.MetaNothing)); err != nil {
		t.Fatal(err)
	}
	if err := tm.Register("timer", typ.NewMetaScript("timer", typ.MetaAny, typ.MetaNothing)); err != nil {
		t.Fatal(err)
	}
	if err := tm.Register("softtimer", typ.NewMetaScript("softtimer", typ.MetaAny, typ.MetaNothing)); err != nil {
		t.Fatal(err)
	}
	return tm
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/command/ -run 'TestRegisterAll_(QueueTypedDisabled|EnumsDisabled|StructsDisabled|DBTablesDisabled|AllEnabled)' -v`
Expected: FAIL — disabled gates currently register handlers unconditionally.

- [ ] **Step 3: Update `register.go` to honor feature flags**

In `pkg/pack/compiler/command/register.go`:

1. Delete the `_ = features // consumed above` line at the end of the function.
2. Delete the doc-comment paragraphs containing `NAI-207-D-REGISTERALL-NO-FEATURES`.
3. Wrap typed-queue registrations in `if !features.DisableQueueTyped { ... }`:

```go
	// queue* vararg variants. Mirrors TS L95-L102.
	if !features.DisableQueueTyped {
		for _, name := range []string{"queue*", ".queue*", "weakqueue*", ".weakqueue*", "strongqueue*", ".strongqueue*"} {
			register(name, NewQueueVarArgCommandHandler(queueType))
		}
	}
```

4. Wrap longqueue\* in `if !features.DisableQueueTyped { ... }`:

```go
	if !features.DisableQueueTyped {
		for _, name := range []string{"longqueue*", ".longqueue*"} {
			register(name, NewLongQueueVarArgCommandHandler(queueType))
		}
	}
```

5. Wrap `enum` registration in `if !features.DisableEnums { ... }`:

```go
	if !features.DisableEnums {
		register("enum", &EnumCommandHandler{})
	}
```

6. Wrap `struct_param` in `if !features.DisableStructs { ... }`:

```go
	if !features.DisableStructs {
		register("struct_param", NewParamCommandHandler(nil))
	}
```

7. Wrap all `db_*` registrations in `if !features.DisableDBTables { ... }`:

```go
	if !features.DisableDBTables {
		register("db_find", NewDbFindCommandHandler(false))
		register("db_find_refine", NewDbFindCommandHandler(false))
		register("db_find_with_count", NewDbFindCommandHandler(true))
		register("db_find_refine_with_count", NewDbFindCommandHandler(true))
		register("db_getfield", &DbGetFieldCommandHandler{})
	}
```

Keep the doc-comments preceding each gated block updated to remove the NAI-207 deviation reference and instead say "gated on features.DisableX per TS L<line>".

- [ ] **Step 4: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/command/ -v`
Expected: PASS — new + existing tests.

- [ ] **Step 5: Run full suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add pkg/pack/compiler/command/register.go pkg/pack/compiler/command/cohort_a_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(compiler/command): NAI-210 T6 — wire StrictFeatureLevel gates in RegisterAllDynCommands

Honors features.DisableQueueTyped / DisableEnums / DisableStructs /
DisableDBTables in dynamic-command registration, matching TS
ServerScriptCompiler.setup() L80-212.

Retires NAI-207-D-REGISTERALL-NO-FEATURES (pin updated in T16).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task T7: `ScriptVarType` enum

**Files:**
- Create: `pkg/pack/compiler/type/scriptvar.go`
- Create: `pkg/pack/compiler/type/scriptvar_test.go`

**TS source:** `src/runescript/type/ScriptVarType.ts` (84 lines). 41 named singletons (40 in addition to `CATEGORY` which is `PrimitiveCategory` from T5).

These behave identically to `*PrimitiveType` from the compiler's perspective: name, code, base type, default value, options. We model them as `*ScriptVarType` (new struct in `type/scriptvar.go`) sharing the same `Type` interface implementation as `*PrimitiveType`. Rationale: keeping a distinct type lets future ScriptVarType-specific behavior (e.g., type-vs-primitive predicates) attach. They differ from primitives only in being registered at compiler-setup time vs at-startup.

- [ ] **Step 1: Write the failing test**

Create `pkg/pack/compiler/type/scriptvar_test.go`:

```go
// pkg/pack/compiler/type/scriptvar_test.go
package typ

import "testing"

// TestScriptVarTypeAll_Order pins TS L25-83 declaration order.
// Each entry: (representation, code).
func TestScriptVarTypeAll_Order(t *testing.T) {
	want := []struct {
		rep, code string
	}{
		{"seq", "A"},
		{"locshape", "H"},
		{"component", "I"},
		{"idkit", "K"},
		{"midi", "M"},
		{"npc_mode", "N"},
		{"namedobj", "O"},
		{"synth", "P"},
		{"area", "R"},
		{"stat", "S"},
		{"npc_stat", "T"},
		{"writeinv", "V"},
		{"wma", "`"},
		{"graphic", "d"},
		{"fontmetrics", "f"},
		{"enum", "g"},
		{"hunt", "h"},
		{"jingle", "j"},
		{"loc", "l"},
		{"model", "m"},
		{"npc", "n"},
		{"obj", "o"},
		{"player_uid", "p"},
		{"spotanim", "t"},
		{"npc_uid", "u"},
		{"inv", "v"},
		{"texture", "x"},
		// CATEGORY ('y') lives in PrimitiveCategory (T5); not in ScriptVarTypeAll.
		{"mapelement", "µ"},
		{"hitmark", "×"},
		{"struct", "J"},
		{"dbrow", "Ð"},
		{"interface", "a"},
		{"toplevelinterface", "F"},
		{"overlayinterface", "L"},
		{"movespeed", "Ý"},
		{"entityoverlay", "-"},
		{"dbtable", "Ø"},
		{"stringvector", "¸"},
		{"mesanim", "Á"},
		{"verifyobj", "®"},
	}
	if got := len(ScriptVarTypeAll); got != len(want) {
		t.Fatalf("ScriptVarTypeAll length = %d, want %d", got, len(want))
	}
	for i, w := range want {
		entry := ScriptVarTypeAll[i]
		if got := entry.Representation(); got != w.rep {
			t.Errorf("ScriptVarTypeAll[%d].Representation = %q, want %q", i, got, w.rep)
		}
		code, ok := entry.Code()
		if !ok || code != w.code {
			t.Errorf("ScriptVarTypeAll[%d].Code = (%q, %v), want (%q, true)", i, code, ok, w.code)
		}
	}
}

// TestScriptVarType_BaseType pins TS L21: all entries are INTEGER.
func TestScriptVarType_BaseType(t *testing.T) {
	for i, sv := range ScriptVarTypeAll {
		bt, ok := sv.BaseType()
		if !ok || bt != BaseVarInteger {
			t.Errorf("ScriptVarTypeAll[%d].BaseType = (%v, %v), want (Integer, true)", i, bt, ok)
		}
	}
}

// TestScriptVarType_AsType verifies *ScriptVarType implements Type.
func TestScriptVarType_AsType(t *testing.T) {
	var _ Type = ScriptVarLoc
}

// TestScriptVarLoc_Singleton spot-checks a named singleton.
func TestScriptVarLoc_Singleton(t *testing.T) {
	if got := ScriptVarLoc.Representation(); got != "loc" {
		t.Errorf("ScriptVarLoc.Representation = %q, want \"loc\"", got)
	}
	code, _ := ScriptVarLoc.Code()
	if code != "l" {
		t.Errorf("ScriptVarLoc.Code = %q, want \"l\"", code)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/type/ -run 'TestScriptVarType|TestScriptVarLoc' -v`
Expected: FAIL — `ScriptVarType` / `ScriptVarTypeAll` undefined.

- [ ] **Step 3: Write minimal implementation**

Create `pkg/pack/compiler/type/scriptvar.go`:

```go
// pkg/pack/compiler/type/scriptvar.go
package typ

// ScriptVarType represents a runtime-loaded named type
// (loc/npc/obj/enum/struct/etc.) registered at compiler-setup time. Distinct
// from PrimitiveType because ScriptVarType entries always have
// BaseVarType.Integer + defaultValue -1 + mutable options, and are not
// part of the bootstrap primitive set.
//
// Mirrors TS src/runescript/type/ScriptVarType.ts. Type-wise indistinguishable
// from a PrimitiveType registered with the same parameters.
type ScriptVarType struct {
	rep      string
	code     string
	codeOK   bool
	options  TypeOptions
}

func newScriptVarType(name, code string) *ScriptVarType {
	return &ScriptVarType{
		rep:     name,
		code:    code,
		codeOK:  code != "",
		options: NewTypeOptions(),
	}
}

func (s *ScriptVarType) Representation() string        { return s.rep }
func (s *ScriptVarType) Code() (string, bool)          { return s.code, s.codeOK }
func (s *ScriptVarType) BaseType() (BaseVarType, bool) { return BaseVarInteger, true }
func (s *ScriptVarType) DefaultValue() any             { return -1 }
func (s *ScriptVarType) Options() TypeOptions          { return s.options }
func (s *ScriptVarType) AsTypeRef()                    {}

// Singletons. Names + codes match TS ScriptVarType.ts L25-83.
// (CATEGORY is PrimitiveCategory, ported in T5 — not in this slice.)
var (
	ScriptVarSeq               = newScriptVarType("seq", "A")
	ScriptVarLocShape          = newScriptVarType("locshape", "H")
	ScriptVarComponent         = newScriptVarType("component", "I")
	ScriptVarIdKit             = newScriptVarType("idkit", "K")
	ScriptVarMidi              = newScriptVarType("midi", "M")
	ScriptVarNpcMode           = newScriptVarType("npc_mode", "N")
	ScriptVarNamedObj          = newScriptVarType("namedobj", "O")
	ScriptVarSynth             = newScriptVarType("synth", "P")
	ScriptVarArea              = newScriptVarType("area", "R")
	ScriptVarStat              = newScriptVarType("stat", "S")
	ScriptVarNpcStat           = newScriptVarType("npc_stat", "T")
	ScriptVarWriteInv          = newScriptVarType("writeinv", "V")
	ScriptVarMapArea           = newScriptVarType("wma", "`")
	ScriptVarGraphic           = newScriptVarType("graphic", "d")
	ScriptVarFontMetrics       = newScriptVarType("fontmetrics", "f")
	ScriptVarEnum              = newScriptVarType("enum", "g")
	ScriptVarHunt              = newScriptVarType("hunt", "h")
	ScriptVarJingle            = newScriptVarType("jingle", "j")
	ScriptVarLoc               = newScriptVarType("loc", "l")
	ScriptVarModel             = newScriptVarType("model", "m")
	ScriptVarNpc               = newScriptVarType("npc", "n")
	ScriptVarObj               = newScriptVarType("obj", "o")
	ScriptVarPlayerUID         = newScriptVarType("player_uid", "p")
	ScriptVarSpotAnim          = newScriptVarType("spotanim", "t")
	ScriptVarNpcUID            = newScriptVarType("npc_uid", "u")
	ScriptVarInv               = newScriptVarType("inv", "v")
	ScriptVarTexture           = newScriptVarType("texture", "x")
	ScriptVarMapElement        = newScriptVarType("mapelement", "µ")
	ScriptVarHitmark           = newScriptVarType("hitmark", "×")
	ScriptVarStruct            = newScriptVarType("struct", "J")
	ScriptVarDbRow             = newScriptVarType("dbrow", "Ð")
	ScriptVarInterface         = newScriptVarType("interface", "a")
	ScriptVarTopLevelInterface = newScriptVarType("toplevelinterface", "F")
	ScriptVarOverlayInterface  = newScriptVarType("overlayinterface", "L")
	ScriptVarMoveSpeed         = newScriptVarType("movespeed", "Ý")
	ScriptVarEntityOverlay     = newScriptVarType("entityoverlay", "-")
	ScriptVarDbTable           = newScriptVarType("dbtable", "Ø")
	ScriptVarStringVector      = newScriptVarType("stringvector", "¸")
	ScriptVarMesAnim           = newScriptVarType("mesanim", "Á")
	ScriptVarVerifyObject      = newScriptVarType("verifyobj", "®")
)

// ScriptVarTypeAll preserves TS ALL push order (declaration order at L25-83).
var ScriptVarTypeAll = []*ScriptVarType{
	ScriptVarSeq, ScriptVarLocShape, ScriptVarComponent, ScriptVarIdKit,
	ScriptVarMidi, ScriptVarNpcMode, ScriptVarNamedObj, ScriptVarSynth,
	ScriptVarArea, ScriptVarStat, ScriptVarNpcStat, ScriptVarWriteInv,
	ScriptVarMapArea, ScriptVarGraphic, ScriptVarFontMetrics, ScriptVarEnum,
	ScriptVarHunt, ScriptVarJingle, ScriptVarLoc, ScriptVarModel,
	ScriptVarNpc, ScriptVarObj, ScriptVarPlayerUID, ScriptVarSpotAnim,
	ScriptVarNpcUID, ScriptVarInv, ScriptVarTexture,
	ScriptVarMapElement, ScriptVarHitmark, ScriptVarStruct, ScriptVarDbRow,
	ScriptVarInterface, ScriptVarTopLevelInterface, ScriptVarOverlayInterface,
	ScriptVarMoveSpeed, ScriptVarEntityOverlay, ScriptVarDbTable,
	ScriptVarStringVector, ScriptVarMesAnim, ScriptVarVerifyObject,
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/type/ -run 'TestScriptVarType|TestScriptVarLoc' -v`
Expected: PASS.

- [ ] **Step 5: Run full suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add pkg/pack/compiler/type/scriptvar.go pkg/pack/compiler/type/scriptvar_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(compiler/type): NAI-210 T7 — ScriptVarType enum (40 singletons)

Ports TS src/runescript/type/ScriptVarType.ts as 40 *ScriptVarType
singletons + ScriptVarTypeAll slice preserving declaration order. CATEGORY
lives in PrimitiveCategory (T5).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task T8: `ServerTriggerType` enum

**Files:**
- Create: `pkg/pack/compiler/trigger/server_trigger_type.go`
- Create: `pkg/pack/compiler/trigger/server_trigger_type_test.go`

**TS source:** `src/runescript/trigger/ServerTriggerType.ts`. **Plan-task instruction:** read the TS file first (`cat /home/owner/Code/github.com/LostCityRS/RuneScriptTS/src/runescript/trigger/ServerTriggerType.ts`) to enumerate every trigger declaration. Each declaration becomes a `var ServerTrigger<Name> = &TriggerType{...}` in goscape. Preserve declaration order in the `ServerTriggerTypeAll` slice.

Each TS `ServerTriggerType.X = new ServerTriggerType(id, identifier, subjectMode, allowParameters, parameters, allowReturns, returns, pointers)` maps to:

```go
ServerTriggerX = &TriggerType{
    ID:              <id>,
    Identifier:      "<identifier>",
    SubjectMode:     <SubjectMode singleton or constructor call>,
    AllowParameters: <bool>,
    Parameters:      <typ.Type or nil>,
    AllowReturns:    <bool>,
    Returns:         <typ.Type or nil>,
    Pointers:        <pointer.PointerSet or nil>,
}
```

**Caution:** TS uses `ScriptVarType.X` and `MetaType.X` as Parameters/Returns. Translate to goscape's equivalent (`typ.ScriptVarLoc` / `typ.PrimitiveInt` / `typ.MetaUnit` / `typ.MetaAny` / `typ.MetaNothing` etc.). Pointers translate via `pointer.NewPointerSet(pointer.ActivePlayer, pointer.ActiveNpc)` etc. (see `pkg/pack/compiler/pointer/type.go` for the singletons).

**SubjectMode translation:** TS `SubjectMode.NAME` → `trigger.ModeName`; TS `SubjectMode.Type(...)` → `trigger.NewTypeMode(...)`; TS `SubjectMode.NONE` → `trigger.ModeNone`. Verify exact API by reading `pkg/pack/compiler/trigger/subjectmode.go`.

- [ ] **Step 1: Read the TS file**

Run: `cat /home/owner/Code/github.com/LostCityRS/RuneScriptTS/src/runescript/trigger/ServerTriggerType.ts | head -300`

Enumerate every static declaration (`static readonly X = new ServerTriggerType(...)`). Build a mental (or scratchpad) table of `(name, id, identifier, subjectMode, allowParameters, parameters, allowReturns, returns, pointers)` for each.

- [ ] **Step 2: Write the failing test**

Create `pkg/pack/compiler/trigger/server_trigger_type_test.go`:

```go
// pkg/pack/compiler/trigger/server_trigger_type_test.go
package trigger

import "testing"

// TestServerTriggerTypeAll_NonEmpty pins existence + minimum count.
// Exact count = number of TS ServerTriggerType.ts static declarations.
// Subagent: replace `wantMin` with the actual TS count after reading
// the file. Acceptable range: 40-60.
func TestServerTriggerTypeAll_NonEmpty(t *testing.T) {
	if len(ServerTriggerTypeAll) < 40 {
		t.Errorf("ServerTriggerTypeAll length = %d, want >=40", len(ServerTriggerTypeAll))
	}
}

// TestServerTriggerProc pins TS .PROC identifier + id.
func TestServerTriggerProc(t *testing.T) {
	if ServerTriggerProc.Identifier != "proc" {
		t.Errorf("ServerTriggerProc.Identifier = %q, want \"proc\"", ServerTriggerProc.Identifier)
	}
}

// TestServerTriggerLabel pins TS .LABEL identifier.
func TestServerTriggerLabel(t *testing.T) {
	if ServerTriggerLabel.Identifier != "label" {
		t.Errorf("ServerTriggerLabel.Identifier = %q, want \"label\"", ServerTriggerLabel.Identifier)
	}
}

// TestServerTriggerQueue pins TS .QUEUE identifier.
func TestServerTriggerQueue(t *testing.T) {
	if ServerTriggerQueue.Identifier != "queue" {
		t.Errorf("ServerTriggerQueue.Identifier = %q, want \"queue\"", ServerTriggerQueue.Identifier)
	}
}

// TestServerTriggerTypeAll_AllRegisterable verifies every entry has a
// non-empty Identifier (TriggerManager.Register would reject otherwise).
func TestServerTriggerTypeAll_AllRegisterable(t *testing.T) {
	for i, tr := range ServerTriggerTypeAll {
		if tr.Identifier == "" {
			t.Errorf("ServerTriggerTypeAll[%d].Identifier empty", i)
		}
	}
}

// TestServerTriggerIfButton_ID pins NAI-208 cross-referenced ID 147.
// This is the only existing cross-package ID assumption that must remain
// stable.
func TestServerTriggerIfButton_ID(t *testing.T) {
	if ServerTriggerIfButton.ID != 147 {
		t.Errorf("ServerTriggerIfButton.ID = %d, want 147", ServerTriggerIfButton.ID)
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/trigger/ -run TestServerTrigger -v`
Expected: FAIL — `ServerTriggerProc` etc. undefined.

- [ ] **Step 4: Write minimal implementation**

Create `pkg/pack/compiler/trigger/server_trigger_type.go`. Header + declaration template:

```go
// pkg/pack/compiler/trigger/server_trigger_type.go
package trigger

import (
	"github.com/zsrv/goscape/pkg/pack/compiler/pointer"
	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

// Server triggers — ports TS src/runescript/trigger/ServerTriggerType.ts at
// SHA b8c338801fbb72d294ff9576a58925a8d3f6de47.
//
// Each singleton mirrors a TS `static readonly X = new ServerTriggerType(...)`
// declaration. Declaration order = TS file order; ServerTriggerTypeAll
// preserves that order so triggers.RegisterAll(...) consumes triggers in
// the TS-deterministic sequence.

// (Singletons go here.)
var (
	ServerTriggerProc = &TriggerType{
		ID:              0,
		Identifier:      "proc",
		SubjectMode:     ModeName,
		AllowParameters: true,
		Parameters:      typ.MetaAny,
		AllowReturns:    true,
		Returns:         typ.MetaAny,
		Pointers:        nil,
	}

	ServerTriggerLabel = &TriggerType{
		ID:              1,
		Identifier:      "label",
		SubjectMode:     ModeName,
		AllowParameters: true,
		Parameters:      typ.MetaAny,
		AllowReturns:    false,
		Returns:         typ.MetaNothing,
		Pointers:        nil,
	}

	// ... port every other ServerTriggerType.X declaration here, preserving:
	//   - declaration order (drives ServerTriggerTypeAll order),
	//   - ID,
	//   - identifier (lowercase),
	//   - SubjectMode (ModeName / ModeNone / NewTypeMode(...)),
	//   - Parameters / Returns (typ.MetaUnit / typ.MetaAny / typ.MetaNothing
	//     or one of the ScriptVar* / Primitive* singletons),
	//   - Pointers (pointer.NewPointerSet(pointer.X, pointer.Y) or nil if
	//     empty in TS).

	// Cross-referenced IDs (also pinned in runescript/server_pointer_checker.go):
	//   ServerTriggerIfButton.ID   = 147
	//   ServerTriggerInvButton1.ID = 149
	//   ServerTriggerInvButton2.ID = 150
	//   ServerTriggerInvButton3.ID = 151
	//   ServerTriggerInvButton4.ID = 152
	//   ServerTriggerInvButton5.ID = 153
	//   ServerTriggerInvButtonD.ID = 154
	ServerTriggerQueue = &TriggerType{
		ID:              2,
		Identifier:      "queue",
		SubjectMode:     ModeName,
		AllowParameters: true,
		Parameters:      typ.MetaAny,
		AllowReturns:    false,
		Returns:         typ.MetaNothing,
		Pointers:        nil,
	}
	// ... remaining triggers ...
	ServerTriggerIfButton = &TriggerType{
		ID:              147,
		Identifier:      "if_button",
		// ... fields per TS ...
	}
	// ... etc through the end of ServerTriggerType.ts ...
)

// ServerTriggerTypeAll preserves TS ALL push order.
var ServerTriggerTypeAll = []*TriggerType{
	ServerTriggerProc, ServerTriggerLabel, ServerTriggerQueue,
	// ... rest, in declaration order ...
	ServerTriggerIfButton,
	// ... etc ...
}
```

**Subagent execution note:** This task requires reading the entire TS file (~50 triggers) and translating each. The translation rules are mechanical:

1. `new ServerTriggerType(N, 'foo_bar', ...)` → ID=`N`, Identifier=`"foo_bar"`.
2. `SubjectMode.NAME` → `ModeName`. `SubjectMode.Type(allowedTypes)` → `NewTypeMode(...)`. `SubjectMode.NONE` → `ModeNone`.
3. `allowParameters: true|false` → `AllowParameters` literal.
4. Parameters: TS `MetaType.Any` → `typ.MetaAny`; `MetaType.Unit` → `typ.MetaUnit`; `MetaType.Nothing` → `typ.MetaNothing`. `ScriptVarType.X` → `typ.ScriptVarX` (note: TS `OBJ` → goscape `ScriptVarObj`, etc.). `PrimitiveType.INT` → `typ.PrimitiveInt`. Tuples translate via `typ.NewTupleType([typ.A, typ.B])` if such a constructor exists, else hand-build (read `pkg/pack/compiler/type/tuple.go`).
5. Pointers: TS `new Set([PointerType.X, PointerType.Y])` → `pointer.NewPointerSet(pointer.X, pointer.Y)`. Empty set → `nil`.

Cross-check the cross-referenced IDs after writing — they must match `pkg/pack/compiler/runescript/server_pointer_checker.go::IDIfButton` (147) and the other six `IDInvButton*` constants.

- [ ] **Step 5: Run test to verify it passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/trigger/ -v`
Expected: PASS — `TestServerTriggerTypeAll_*`, `TestServerTriggerProc`, `TestServerTriggerLabel`, `TestServerTriggerQueue`, `TestServerTriggerIfButton_ID`, plus any existing trigger tests.

- [ ] **Step 6: Run full suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add pkg/pack/compiler/trigger/server_trigger_type.go pkg/pack/compiler/trigger/server_trigger_type_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(compiler/trigger): NAI-210 T8 — ServerTriggerType enum

Ports TS src/runescript/trigger/ServerTriggerType.ts as *TriggerType
singletons + ServerTriggerTypeAll slice in declaration order. Pins the
cross-referenced IDs already used by runescript/server_pointer_checker.go
(IF_BUTTON=147, INV_BUTTON1..D = 149..154).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task T9: `CompilerTypeInfo` + `SymbolLoader` interface

**Files:**
- Create: `pkg/pack/compiler/runescript/compiler_type_info.go`
- Create: `pkg/pack/compiler/symbol/loader.go`
- Create: `pkg/pack/compiler/symbol/loader_test.go`

**TS source:** `src/runescript/CompilerTypeInfo.ts` (17 lines) + `src/compiler/configuration/SymbolLoader.ts` (54 lines).

- [ ] **Step 1: Write the failing test**

Create `pkg/pack/compiler/symbol/loader_test.go`:

```go
// pkg/pack/compiler/symbol/loader_test.go
package symbol

import (
	"testing"

	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

// TestAddConstant_HappyPath inserts a constant and verifies it can be looked up.
func TestAddConstant_HappyPath(t *testing.T) {
	tab := NewSymbolTable(nil)
	sym, err := AddConstant(tab, "MAX_LEVEL", "99")
	if err != nil {
		t.Fatalf("AddConstant: %v", err)
	}
	if sym.Name != "MAX_LEVEL" || sym.Value != "99" {
		t.Errorf("inserted ConstantSymbol: got %+v", sym)
	}
	got := tab.Find(SymbolTypeConstant(), "MAX_LEVEL")
	if got != sym {
		t.Errorf("Find after AddConstant: got %v, want %v", got, sym)
	}
}

// TestAddConstant_DuplicateReturnsError pins TS L26-28 (Go: returns error).
func TestAddConstant_DuplicateReturnsError(t *testing.T) {
	tab := NewSymbolTable(nil)
	if _, err := AddConstant(tab, "X", "1"); err != nil {
		t.Fatal(err)
	}
	if _, err := AddConstant(tab, "X", "2"); err == nil {
		t.Error("AddConstant duplicate: want error, got nil")
	}
}

// TestAddBasic_HappyPath inserts a basic symbol with type.
func TestAddBasic_HappyPath(t *testing.T) {
	tab := NewSymbolTable(nil)
	sym, err := AddBasic(tab, typ.PrimitiveInt, "foo", false)
	if err != nil {
		t.Fatalf("AddBasic: %v", err)
	}
	if sym.Name != "foo" || sym.Type != typ.PrimitiveInt || sym.IsProtected {
		t.Errorf("inserted BasicSymbol: got %+v", sym)
	}
}

// TestAddBasic_Protected pins isProtected propagation.
func TestAddBasic_Protected(t *testing.T) {
	tab := NewSymbolTable(nil)
	sym, err := AddBasic(tab, typ.PrimitiveInt, "secret", true)
	if err != nil {
		t.Fatal(err)
	}
	if !sym.IsProtected {
		t.Error("AddBasic isProtected=true: want IsProtected=true, got false")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/symbol/ -run 'TestAddConstant|TestAddBasic' -v`
Expected: FAIL — `AddConstant`, `AddBasic` undefined.

- [ ] **Step 3: Write minimal implementation**

Create `pkg/pack/compiler/symbol/loader.go`:

```go
// pkg/pack/compiler/symbol/loader.go
package symbol

import (
	"fmt"

	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

// CompilerContext is the narrow interface SymbolLoader implementations need.
// Defined here (not in runescript/) to break the import cycle that storing
// a *runescript.ServerScriptCompiler in SymbolLoader.Load would create.
type CompilerContext interface {
	FindType(name string) typ.Type
}

// SymbolLoader is the contract for pre-compilation external-symbol loading.
// Mirrors TS abstract class SymbolLoader at
// src/compiler/configuration/SymbolLoader.ts.
type SymbolLoader interface {
	Load(table *SymbolTable, compiler CompilerContext) error
}

// AddConstant inserts a ConstantSymbol with the given name + value. Returns
// the inserted symbol or an error if Insert returned false (TS throws).
// Mirrors TS SymbolLoader.addConstant L19-26.
func AddConstant(table *SymbolTable, name, value string) (*ConstantSymbol, error) {
	s := &ConstantSymbol{Name: name, Value: value}
	if !table.Insert(SymbolTypeConstant(), s) {
		return nil, fmt.Errorf("unable to add constant: name=%s, value=%s", name, value)
	}
	return s, nil
}

// AddBasic inserts a BasicSymbol with the given type, name, and protected
// flag. Returns the inserted symbol or an error if Insert returned false.
// Mirrors TS SymbolLoader.addBasic L33-44.
func AddBasic(table *SymbolTable, t typ.Type, name string, isProtected bool) (*BasicSymbol, error) {
	s := &BasicSymbol{Name: name, Type: t, IsProtected: isProtected}
	if !table.Insert(SymbolTypeBasic(t), s) {
		return nil, fmt.Errorf("unable to add basic: type=%v, name=%s", t, name)
	}
	return s, nil
}
```

Create `pkg/pack/compiler/runescript/compiler_type_info.go`:

```go
// pkg/pack/compiler/runescript/compiler_type_info.go
package runescript

// CompilerTypeInfo carries the per-config / per-command metadata read from
// the external symbol files (one per config kind: command, runescript, loc,
// npc, obj, etc.). Mirrors TS src/runescript/CompilerTypeInfo.ts.
//
// All map fields are keyed by stringified numeric id ("0", "1", ...).
// The "vartype" and "protect" fields are populated only for some configs.
// The remaining fields are populated only for command symbols.
type CompilerTypeInfo struct {
	Max int

	// Map: id (as string) → symbol name.
	Map map[string]string

	// Vartype: id (as string) → comma-separated type list (for typed vars like varp/varn/dbcolumn).
	Vartype map[string]string

	// Protect: id (as string) → whether the symbol is write-protected (used by varp/varbit).
	Protect map[string]bool

	// Require / Set / Corrupt: id (as string) → comma-separated pointer-name list (commands only).
	Require  map[string]string
	Require2 map[string]string
	Set      map[string]string
	Set2     map[string]string
	Corrupt  map[string]string
	Corrupt2 map[string]string

	// Conditional: id (as string) → conditional-set marker (commands only).
	Conditional map[string]bool
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/symbol/ -v`
Expected: PASS.

- [ ] **Step 5: Run full suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add pkg/pack/compiler/symbol/loader.go pkg/pack/compiler/symbol/loader_test.go pkg/pack/compiler/runescript/compiler_type_info.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(compiler): NAI-210 T9 — CompilerTypeInfo + SymbolLoader prerequisites

Ports TS src/runescript/CompilerTypeInfo.ts (data struct) +
src/compiler/configuration/SymbolLoader.ts (interface + AddConstant /
AddBasic helpers + CompilerContext interface in symbol/ to avoid cycle).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task T10: Three `CompilerTypeInfo*Loader` implementations

**Files:**
- Create: `pkg/pack/compiler/runescript/type_info_loader.go`
- Create: `pkg/pack/compiler/runescript/type_info_loader_test.go`

**TS source:** `src/runescript/CompilerTypeInfoConstantLoader.ts` (18 lines) + `src/runescript/CompilerTypeInfoLoader.ts` (38 lines) + `src/runescript/CompilerTypeInfoProtectedLoader.ts` (45 lines).

- [ ] **Step 1: Write the failing test**

Create `pkg/pack/compiler/runescript/type_info_loader_test.go`:

```go
// pkg/pack/compiler/runescript/type_info_loader_test.go
package runescript

import (
	"testing"

	"github.com/zsrv/goscape/pkg/pack/compiler/symbol"
	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

type stubCompilerContext struct {
	types map[string]typ.Type
}

func (s *stubCompilerContext) FindType(name string) typ.Type {
	return s.types[name]
}

// TestCompilerTypeInfoConstantLoader_InsertsAll pins TS L13-15.
func TestCompilerTypeInfoConstantLoader_InsertsAll(t *testing.T) {
	tab := symbol.NewSymbolTable(nil)
	info := &CompilerTypeInfo{
		Map: map[string]string{
			"MAX_LEVEL":  "99",
			"MIN_LEVEL":  "1",
		},
	}
	loader := &CompilerTypeInfoConstantLoader{Symbols: info}
	if err := loader.Load(tab, &stubCompilerContext{}); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := tab.Find(symbol.SymbolTypeConstant(), "MAX_LEVEL"); got == nil {
		t.Error("MAX_LEVEL not inserted")
	}
	if got := tab.Find(symbol.SymbolTypeConstant(), "MIN_LEVEL"); got == nil {
		t.Error("MIN_LEVEL not inserted")
	}
}

// TestCompilerTypeInfoLoader_Vartype_TupleFromList pins TS L21-24 — comma-
// separated type list becomes a TupleType.
func TestCompilerTypeInfoLoader_Vartype_TupleFromList(t *testing.T) {
	mapper := NewSymbolMapper(nil)
	info := &CompilerTypeInfo{
		Map:     map[string]string{"0": "foo"},
		Vartype: map[string]string{"0": "int,string"},
	}
	ctx := &stubCompilerContext{types: map[string]typ.Type{
		"int":    typ.PrimitiveInt,
		"string": typ.PrimitiveString,
	}}
	loader := &CompilerTypeInfoLoader{
		Mapper:       mapper,
		Symbols:      info,
		TypeSupplier: func(sub typ.Type) typ.Type { return sub },
	}
	tab := symbol.NewSymbolTable(nil)
	if err := loader.Load(tab, ctx); err != nil {
		t.Fatalf("Load: %v", err)
	}
	// Verify the symbol was inserted with the right (Tuple) type.
	// Exact tuple lookup depends on SymbolTable API; spot-check it exists.
	syms := tab.FindAll("foo")
	if len(syms) == 0 {
		t.Fatal("symbol 'foo' not found")
	}
}

// TestCompilerTypeInfoLoader_NoVartype_DefaultsToUnit pins TS L19 — when
// vartype is missing, subTypes defaults to MetaType.Unit.
func TestCompilerTypeInfoLoader_NoVartype_DefaultsToUnit(t *testing.T) {
	mapper := NewSymbolMapper(nil)
	info := &CompilerTypeInfo{Map: map[string]string{"5": "bar"}}
	var capturedSub typ.Type
	loader := &CompilerTypeInfoLoader{
		Mapper:  mapper,
		Symbols: info,
		TypeSupplier: func(sub typ.Type) typ.Type {
			capturedSub = sub
			return typ.PrimitiveInt
		},
	}
	if err := loader.Load(symbol.NewSymbolTable(nil), &stubCompilerContext{}); err != nil {
		t.Fatal(err)
	}
	if capturedSub != typ.MetaUnit {
		t.Errorf("TypeSupplier subtype: got %v, want MetaUnit", capturedSub)
	}
}

// TestCompilerTypeInfoLoader_UnknownTypeNameMapsToError pins TS L23 —
// type lookup that fails resolves to MetaType.Error.
func TestCompilerTypeInfoLoader_UnknownTypeNameMapsToError(t *testing.T) {
	mapper := NewSymbolMapper(nil)
	info := &CompilerTypeInfo{
		Map:     map[string]string{"0": "baz"},
		Vartype: map[string]string{"0": "no_such_type"},
	}
	var capturedSub typ.Type
	loader := &CompilerTypeInfoLoader{
		Mapper:  mapper,
		Symbols: info,
		TypeSupplier: func(sub typ.Type) typ.Type {
			capturedSub = sub
			return typ.PrimitiveInt
		},
	}
	if err := loader.Load(symbol.NewSymbolTable(nil), &stubCompilerContext{}); err != nil {
		t.Fatal(err)
	}
	// For a single-entry vartype, the tuple "from list" returns the bare
	// element (TS TupleType.fromList unwraps single-element lists). The
	// captured sub should be MetaError.
	if capturedSub != typ.MetaError {
		t.Errorf("unknown type: got %v, want MetaError", capturedSub)
	}
}

// TestCompilerTypeInfoProtectedLoader_PropagatesProtect pins TS L24-29.
func TestCompilerTypeInfoProtectedLoader_PropagatesProtect(t *testing.T) {
	mapper := NewSymbolMapper(nil)
	info := &CompilerTypeInfo{
		Map:     map[string]string{"0": "secret"},
		Protect: map[string]bool{"0": true},
	}
	loader := &CompilerTypeInfoProtectedLoader{
		Mapper:       mapper,
		Symbols:      info,
		TypeSupplier: func(sub typ.Type) typ.Type { return typ.PrimitiveInt },
	}
	tab := symbol.NewSymbolTable(nil)
	if err := loader.Load(tab, &stubCompilerContext{}); err != nil {
		t.Fatal(err)
	}
	syms := tab.FindAll("secret")
	if len(syms) == 0 {
		t.Fatal("'secret' not inserted")
	}
	bs, ok := syms[0].(*symbol.BasicSymbol)
	if !ok {
		t.Fatalf("'secret' not a *BasicSymbol: %T", syms[0])
	}
	if !bs.IsProtected {
		t.Error("'secret' IsProtected = false, want true")
	}
}

// TestCompilerTypeInfoLoader_SortedIteration_ByID pins NAI-210-D-LOADER-SORTED-ITERATION.
func TestCompilerTypeInfoLoader_SortedIteration_ByID(t *testing.T) {
	mapper := NewSymbolMapper(nil)
	info := &CompilerTypeInfo{
		Map: map[string]string{
			"10": "ten",
			"2":  "two",
			"5":  "five",
		},
	}
	var order []int
	loader := &CompilerTypeInfoLoader{
		Mapper:  mapper,
		Symbols: info,
		TypeSupplier: func(sub typ.Type) typ.Type {
			return typ.PrimitiveInt
		},
	}
	// Hijack: peek at the order via PutSymbol observation. SymbolMapper.PutSymbol
	// inserts in the order called. We assume PutSymbol is what the loader uses.
	if err := loader.Load(symbol.NewSymbolTable(nil), &stubCompilerContext{}); err != nil {
		t.Fatal(err)
	}
	// Verify symbols were inserted in id order (2, 5, 10) by reading back
	// via Mapper.Get on inserted BasicSymbols. The exact API depends on
	// SymbolMapper.Symbols (or equivalent). The subagent may adapt this
	// test to assert the iteration is sorted by id via an exposed test helper.
	_ = order // placeholder; assertion adapted to actual SymbolMapper introspection API
}
```

The last test (sorted iteration) may be hard to verify without exposing SymbolMapper internals. If unable to assert directly, the subagent may instead instrument the `TypeSupplier` callback to record (id-implied-by-call-order, name) tuples and assert ordering. Adjust as needed.

- [ ] **Step 2: Run test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/runescript/ -run TestCompilerTypeInfo -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

Create `pkg/pack/compiler/runescript/type_info_loader.go`:

```go
// pkg/pack/compiler/runescript/type_info_loader.go
package runescript

import (
	"sort"
	"strconv"
	"strings"

	"github.com/zsrv/goscape/pkg/pack/compiler/symbol"
	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

// CompilerTypeInfoConstantLoader inserts every Map entry as a constant symbol.
// Mirrors TS src/runescript/CompilerTypeInfoConstantLoader.ts.
type CompilerTypeInfoConstantLoader struct {
	Symbols *CompilerTypeInfo
}

// Load iterates Symbols.Map (sorted by key for byte-identical reproducibility;
// see NAI-210-D-LOADER-SORTED-ITERATION) and calls symbol.AddConstant per entry.
func (l *CompilerTypeInfoConstantLoader) Load(table *symbol.SymbolTable, c symbol.CompilerContext) error {
	keys := sortedStringKeys(l.Symbols.Map)
	for _, key := range keys {
		value := l.Symbols.Map[key]
		if _, err := symbol.AddConstant(table, key, value); err != nil {
			return err
		}
	}
	return nil
}

// CompilerTypeInfoLoader registers BasicSymbols with the supplied type and
// updates SymbolMapper. Mirrors TS src/runescript/CompilerTypeInfoLoader.ts.
type CompilerTypeInfoLoader struct {
	Mapper       *SymbolMapper
	Symbols      *CompilerTypeInfo
	TypeSupplier func(subType typ.Type) typ.Type
}

func (l *CompilerTypeInfoLoader) Load(table *symbol.SymbolTable, c symbol.CompilerContext) error {
	keys := sortedNumericKeys(l.Symbols.Map)
	for _, key := range keys {
		name := l.Symbols.Map[key]
		id, err := strconv.Atoi(key)
		if err != nil {
			return err
		}

		subTypes := resolveVartype(l.Symbols.Vartype[key], c)
		t := l.TypeSupplier(subTypes)
		sym, err := symbol.AddBasic(table, t, name, false)
		if err != nil {
			return err
		}
		l.Mapper.PutSymbol(id, sym)
	}
	return nil
}

// CompilerTypeInfoProtectedLoader is identical to CompilerTypeInfoLoader except
// it propagates the Protect[key] flag to the inserted BasicSymbol.
// Mirrors TS src/runescript/CompilerTypeInfoProtectedLoader.ts.
type CompilerTypeInfoProtectedLoader struct {
	Mapper       *SymbolMapper
	Symbols      *CompilerTypeInfo
	TypeSupplier func(subType typ.Type) typ.Type
}

func (l *CompilerTypeInfoProtectedLoader) Load(table *symbol.SymbolTable, c symbol.CompilerContext) error {
	keys := sortedNumericKeys(l.Symbols.Map)
	for _, key := range keys {
		name := l.Symbols.Map[key]
		id, err := strconv.Atoi(key)
		if err != nil {
			return err
		}

		subTypes := resolveVartype(l.Symbols.Vartype[key], c)
		isProtected := false
		if p, ok := l.Symbols.Protect[key]; ok {
			isProtected = p
		}
		t := l.TypeSupplier(subTypes)
		sym, err := symbol.AddBasic(table, t, name, isProtected)
		if err != nil {
			return err
		}
		l.Mapper.PutSymbol(id, sym)
	}
	return nil
}

// resolveVartype handles TS L19-24: comma-separated type names become a
// TupleType, or default to MetaType.Unit if missing.
func resolveVartype(vartype string, c symbol.CompilerContext) typ.Type {
	if vartype == "" {
		return typ.MetaUnit
	}
	parts := strings.Split(vartype, ",")
	children := make([]typ.Type, len(parts))
	for i, tn := range parts {
		t := c.FindType(tn)
		if t == nil {
			t = typ.MetaError
		}
		children[i] = t
	}
	// TupleFromList unwraps single-element lists. Use the goscape equivalent.
	return tupleFromList(children)
}

// tupleFromList ports TS TupleType.fromList: a single-element list returns
// the element directly; otherwise wrap in a TupleType. Plan-task: verify
// pkg/pack/compiler/type/tuple.go for the exact constructor name; this
// helper may simplify to `typ.NewTupleType(children)` if that handles the
// unwrap internally.
func tupleFromList(children []typ.Type) typ.Type {
	if len(children) == 1 {
		return children[0]
	}
	return typ.NewTupleType(children)
}

// sortedStringKeys returns keys in lex order. For constants, key order is
// the symbol's own ordering (TS uses Object.entries — insertion order, not
// numeric).
func sortedStringKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// sortedNumericKeys parses each key as int and sorts ascending. For
// non-numeric keys, leaves them at the end in lex order. NAI-210-D-LOADER-SORTED-ITERATION.
func sortedNumericKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		ai, errA := strconv.Atoi(keys[i])
		bi, errB := strconv.Atoi(keys[j])
		if errA == nil && errB == nil {
			return ai < bi
		}
		if errA == nil {
			return true
		}
		if errB == nil {
			return false
		}
		return keys[i] < keys[j]
	})
	return keys
}
```

`typ.NewTupleType` may or may not exist with that exact name — verify by reading `pkg/pack/compiler/type/tuple.go`. If the existing API is `typ.NewTuple(children)` or similar, use that.

- [ ] **Step 4: Run test to verify it passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/runescript/ -run TestCompilerTypeInfo -v`
Expected: PASS.

- [ ] **Step 5: Run full suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add pkg/pack/compiler/runescript/type_info_loader.go pkg/pack/compiler/runescript/type_info_loader_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(compiler/runescript): NAI-210 T10 — CompilerTypeInfo loaders

Ports TS Constant/Protected/Regular CompilerTypeInfoLoader as three
SymbolLoader implementations. Iteration is sorted by numeric id for
byte-identical SymbolMapper reproducibility
(NAI-210-D-LOADER-SORTED-ITERATION).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task T11: `LoadSpecialSymbols` + pointer-list parsing

**Files:**
- Create: `pkg/pack/compiler/runescript/load_special_symbols.go`
- Create: `pkg/pack/compiler/runescript/load_special_symbols_test.go`

**TS source:** `src/runescript/ServerScriptCompilerApplication.ts` L93-127 (`loadSpecialSymbols` + `parsePointerList`).

- [ ] **Step 1: Write the failing test**

Create `pkg/pack/compiler/runescript/load_special_symbols_test.go`:

```go
// pkg/pack/compiler/runescript/load_special_symbols_test.go
package runescript

import (
	"testing"

	"github.com/zsrv/goscape/pkg/pack/compiler/pointer"
)

// TestLoadSpecialSymbols_BasicMapPopulation populates SymbolMapper entries
// for command + script CompilerTypeInfo.Map.
func TestLoadSpecialSymbols_BasicMapPopulation(t *testing.T) {
	commandInfo := &CompilerTypeInfo{
		Map: map[string]string{"0": "cmd_a", "1": "cmd_b"},
	}
	scriptInfo := &CompilerTypeInfo{
		Map: map[string]string{"100": "[proc,hello]", "200": "[proc,world]"},
	}
	mapper := NewSymbolMapper(nil)
	commandPointers := map[string]*pointer.Holder{}

	if err := LoadSpecialSymbols(commandInfo, scriptInfo, mapper, commandPointers, true); err != nil {
		t.Fatalf("LoadSpecialSymbols: %v", err)
	}
	// Command mapper entries
	// (SymbolMapper API exposes Get via symbol.Symbol — to peek without a Symbol,
	// rely on the resulting state through e.g. an exported "commands by name" helper.
	// If no such helper exists, this test reduces to "no error".)
}

// TestLoadSpecialSymbols_RequireOnly_InsertsHolder pins TS L98-106.
func TestLoadSpecialSymbols_RequireOnly_InsertsHolder(t *testing.T) {
	commandInfo := &CompilerTypeInfo{
		Map:     map[string]string{"0": "foo"},
		Require: map[string]string{"0": "active_player,active_npc"},
	}
	scriptInfo := &CompilerTypeInfo{Map: map[string]string{}}
	mapper := NewSymbolMapper(nil)
	commandPointers := map[string]*pointer.Holder{}

	if err := LoadSpecialSymbols(commandInfo, scriptInfo, mapper, commandPointers, true); err != nil {
		t.Fatalf("LoadSpecialSymbols: %v", err)
	}
	h, ok := commandPointers["foo"]
	if !ok {
		t.Fatal("foo holder not inserted")
	}
	if h.Required == nil || h.Required.Len() != 2 {
		t.Errorf("foo.Required size: got %d, want 2", h.Required.Len())
	}
	if !h.Required.Has(pointer.ActivePlayer) || !h.Required.Has(pointer.ActiveNpc) {
		t.Errorf("foo.Required missing ActivePlayer or ActiveNpc")
	}
}

// TestLoadSpecialSymbols_None_EmptySet pins TS L120-122: 'none' or empty
// resolves to empty set.
func TestLoadSpecialSymbols_None_EmptySet(t *testing.T) {
	commandInfo := &CompilerTypeInfo{
		Map:     map[string]string{"0": "foo"},
		Require: map[string]string{"0": "none"},
		Set:     map[string]string{"0": ""},
		Corrupt: map[string]string{"0": "active_player"},
	}
	scriptInfo := &CompilerTypeInfo{Map: map[string]string{}}
	mapper := NewSymbolMapper(nil)
	commandPointers := map[string]*pointer.Holder{}

	if err := LoadSpecialSymbols(commandInfo, scriptInfo, mapper, commandPointers, true); err != nil {
		t.Fatalf("LoadSpecialSymbols: %v", err)
	}
	h := commandPointers["foo"]
	if h == nil {
		t.Fatal("foo holder not inserted")
	}
	if h.Required != nil && h.Required.Len() != 0 {
		t.Errorf("'none' Require: got len %d, want empty", h.Required.Len())
	}
}

// TestLoadSpecialSymbols_Require2_InsertsAliasHolder pins TS L106-111.
func TestLoadSpecialSymbols_Require2_InsertsAliasHolder(t *testing.T) {
	commandInfo := &CompilerTypeInfo{
		Map:      map[string]string{"0": "foo"},
		Require:  map[string]string{"0": "active_player"},
		Require2: map[string]string{"0": "active_loc"},
	}
	scriptInfo := &CompilerTypeInfo{Map: map[string]string{}}
	mapper := NewSymbolMapper(nil)
	commandPointers := map[string]*pointer.Holder{}

	if err := LoadSpecialSymbols(commandInfo, scriptInfo, mapper, commandPointers, true); err != nil {
		t.Fatalf("LoadSpecialSymbols: %v", err)
	}
	if _, ok := commandPointers["foo"]; !ok {
		t.Error("foo holder not inserted")
	}
	if _, ok := commandPointers[".foo"]; !ok {
		t.Error(".foo alias holder not inserted")
	}
}

// TestLoadSpecialSymbols_NoPointerInfo_NoHolderInserted pins TS L98.
func TestLoadSpecialSymbols_NoPointerInfo_NoHolderInserted(t *testing.T) {
	commandInfo := &CompilerTypeInfo{
		Map: map[string]string{"0": "no_pointers_cmd"},
	}
	scriptInfo := &CompilerTypeInfo{Map: map[string]string{}}
	mapper := NewSymbolMapper(nil)
	commandPointers := map[string]*pointer.Holder{}

	if err := LoadSpecialSymbols(commandInfo, scriptInfo, mapper, commandPointers, true); err != nil {
		t.Fatalf("LoadSpecialSymbols: %v", err)
	}
	if _, ok := commandPointers["no_pointers_cmd"]; ok {
		t.Error("no_pointers_cmd holder should not be inserted")
	}
}

// TestLoadSpecialSymbols_CheckPointersFalse_SkipsHolders pins TS L98 (the
// `if (checkPointers && ...)` gate).
func TestLoadSpecialSymbols_CheckPointersFalse_SkipsHolders(t *testing.T) {
	commandInfo := &CompilerTypeInfo{
		Map:     map[string]string{"0": "foo"},
		Require: map[string]string{"0": "active_player"},
	}
	scriptInfo := &CompilerTypeInfo{Map: map[string]string{}}
	mapper := NewSymbolMapper(nil)
	commandPointers := map[string]*pointer.Holder{}

	if err := LoadSpecialSymbols(commandInfo, scriptInfo, mapper, commandPointers, false); err != nil {
		t.Fatalf("LoadSpecialSymbols: %v", err)
	}
	if _, ok := commandPointers["foo"]; ok {
		t.Error("checkPointers=false: foo holder should not be inserted")
	}
}

// TestLoadSpecialSymbols_UnknownPointer_ReturnsError pins TS L131 (Go: error
// vs throw).
func TestLoadSpecialSymbols_UnknownPointer_ReturnsError(t *testing.T) {
	commandInfo := &CompilerTypeInfo{
		Map:     map[string]string{"0": "foo"},
		Require: map[string]string{"0": "bogus_pointer_name"},
	}
	scriptInfo := &CompilerTypeInfo{Map: map[string]string{}}
	mapper := NewSymbolMapper(nil)
	commandPointers := map[string]*pointer.Holder{}

	err := LoadSpecialSymbols(commandInfo, scriptInfo, mapper, commandPointers, true)
	if err == nil {
		t.Error("unknown pointer: want error, got nil")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/runescript/ -run TestLoadSpecialSymbols -v`
Expected: FAIL — `LoadSpecialSymbols` undefined.

- [ ] **Step 3: Write minimal implementation**

Create `pkg/pack/compiler/runescript/load_special_symbols.go`:

```go
// pkg/pack/compiler/runescript/load_special_symbols.go
package runescript

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/zsrv/goscape/pkg/pack/compiler/pointer"
)

// LoadSpecialSymbols populates commandPointers from commandInfo and seeds
// SymbolMapper command + script maps. Mirrors TS
// src/runescript/ServerScriptCompilerApplication.ts L93-127.
//
// Iteration is sorted by numeric id (NAI-210-D-LOADER-SORTED-ITERATION) for
// reproducibility.
func LoadSpecialSymbols(
	commandInfo, scriptInfo *CompilerTypeInfo,
	mapper *SymbolMapper,
	commandPointers map[string]*pointer.Holder,
	checkPointers bool,
) error {
	commandKeys := sortedNumericKeys(commandInfo.Map)
	for _, key := range commandKeys {
		name := commandInfo.Map[key]
		id, err := strconv.Atoi(key)
		if err != nil {
			return fmt.Errorf("LoadSpecialSymbols: invalid command id %q: %w", key, err)
		}

		hasPtrInfo := commandInfo.Require[key] != "" ||
			commandInfo.Set[key] != "" ||
			commandInfo.Corrupt[key] != ""

		if checkPointers && hasPtrInfo {
			required, err := parsePointerList(commandInfo.Require[key])
			if err != nil {
				return fmt.Errorf("command %q Require: %w", name, err)
			}
			required2, err := parsePointerList(commandInfo.Require2[key])
			if err != nil {
				return fmt.Errorf("command %q Require2: %w", name, err)
			}
			setter, err := parsePointerList(commandInfo.Set[key])
			if err != nil {
				return fmt.Errorf("command %q Set: %w", name, err)
			}
			setter2, err := parsePointerList(commandInfo.Set2[key])
			if err != nil {
				return fmt.Errorf("command %q Set2: %w", name, err)
			}
			corrupted, err := parsePointerList(commandInfo.Corrupt[key])
			if err != nil {
				return fmt.Errorf("command %q Corrupt: %w", name, err)
			}
			corrupted2, err := parsePointerList(commandInfo.Corrupt2[key])
			if err != nil {
				return fmt.Errorf("command %q Corrupt2: %w", name, err)
			}
			conditionalSet := commandInfo.Conditional[key]

			commandPointers[name] = &pointer.Holder{
				Required:       required,
				Set:            setter,
				ConditionalSet: conditionalSet,
				Corrupted:      corrupted,
			}
			if required2.Len() > 0 || setter2.Len() > 0 || corrupted2.Len() > 0 {
				commandPointers["."+name] = &pointer.Holder{
					Required:       required2,
					Set:            setter2,
					ConditionalSet: conditionalSet,
					Corrupted:      corrupted2,
				}
			}
		}

		mapper.PutCommand(id, name)
	}

	scriptKeys := sortedNumericKeys(scriptInfo.Map)
	for _, key := range scriptKeys {
		name := scriptInfo.Map[key]
		id, err := strconv.Atoi(key)
		if err != nil {
			return fmt.Errorf("LoadSpecialSymbols: invalid script id %q: %w", key, err)
		}
		mapper.PutScript(id, name)
	}
	return nil
}

// parsePointerList resolves a comma-separated pointer name list. Empty
// strings and the literal "none" produce an empty set (TS L121-122).
// Unknown names return error (TS L131 throws).
func parsePointerList(s string) (*pointer.PointerSet, error) {
	if s == "" || s == "none" {
		return pointer.NewPointerSet(), nil
	}
	ps := pointer.NewPointerSet()
	for _, name := range strings.Split(s, ",") {
		p := pointer.ForName(strings.TrimSpace(name))
		if p == nil {
			return nil, fmt.Errorf("invalid pointer name: %s", name)
		}
		ps.Add(p)
	}
	return ps, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/runescript/ -run TestLoadSpecialSymbols -v`
Expected: PASS.

- [ ] **Step 5: Run full suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add pkg/pack/compiler/runescript/load_special_symbols.go pkg/pack/compiler/runescript/load_special_symbols_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(compiler/runescript): NAI-210 T11 — LoadSpecialSymbols + parsePointerList

Ports TS ServerScriptCompilerApplication.loadSpecialSymbols /
parsePointerList: command-pointer holder population + SymbolMapper command
and script seeding. Retires NAI-208-D-COMMAND-POINTERS-DEFERRED
(pin updated in T16).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task T12: `ServerScriptCompiler` struct + `setupDefaultTypeCheckers`

**Files:**
- Create: `pkg/pack/compiler/runescript/server_script_compiler.go`
- Create: `pkg/pack/compiler/runescript/server_script_compiler_test.go`
- Create: `pkg/pack/compiler/runescript/default_type_checkers.go`
- Create: `pkg/pack/compiler/runescript/default_type_checkers_test.go`

**TS source:** `src/compiler/ScriptCompiler.ts` L60-115 (constructor + `setupDefaultTypeCheckers`) + L96-180 (the struct layout's parent-class fields).

This task creates the empty struct shell + the default-type-checker helper. `Setup()` and `Run()` come in T13/T14.

- [ ] **Step 1: Write failing tests for `setupDefaultTypeCheckers`**

Create `pkg/pack/compiler/runescript/default_type_checkers_test.go`:

```go
// pkg/pack/compiler/runescript/default_type_checkers_test.go
package runescript

import (
	"testing"

	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

func TestDefaultTypeCheckers_AnyAcceptsAnything(t *testing.T) {
	tm := typ.NewTypeManager()
	registerDefaultTypeCheckers(tm)
	if !tm.Check(typ.MetaAny, typ.PrimitiveInt) {
		t.Error("MetaAny ← PrimitiveInt: want accept")
	}
	if !tm.Check(typ.MetaAny, typ.PrimitiveString) {
		t.Error("MetaAny ← PrimitiveString: want accept")
	}
}

func TestDefaultTypeCheckers_ErrorPropagation(t *testing.T) {
	tm := typ.NewTypeManager()
	registerDefaultTypeCheckers(tm)
	if !tm.Check(typ.MetaError, typ.PrimitiveInt) {
		t.Error("MetaError ← PrimitiveInt: want accept (error propagation)")
	}
	if !tm.Check(typ.PrimitiveInt, typ.MetaError) {
		t.Error("PrimitiveInt ← MetaError: want accept (error propagation)")
	}
}

func TestDefaultTypeCheckers_ReflexiveEquality(t *testing.T) {
	tm := typ.NewTypeManager()
	registerDefaultTypeCheckers(tm)
	if !tm.Check(typ.PrimitiveInt, typ.PrimitiveInt) {
		t.Error("Int ← Int: want accept")
	}
	if tm.Check(typ.PrimitiveInt, typ.PrimitiveString) {
		t.Error("Int ← String: want reject")
	}
}

func TestDefaultTypeCheckers_MetaScriptMatchesTriggerAndParams(t *testing.T) {
	tm := typ.NewTypeManager()
	registerDefaultTypeCheckers(tm)
	a := typ.NewMetaScript("proc", typ.PrimitiveInt, typ.PrimitiveString)
	b := typ.NewMetaScript("proc", typ.PrimitiveInt, typ.PrimitiveString)
	if !tm.Check(a, b) {
		t.Error("matching MetaScript pair: want accept")
	}
	c := typ.NewMetaScript("label", typ.PrimitiveInt, typ.PrimitiveString)
	if tm.Check(a, c) {
		t.Error("different trigger: want reject")
	}
}

// (Additional checker tests for MetaHook, WrappedType, TupleType,
// representation fallback may be added. The above is the minimum that
// covers the "happy path" semantics.)
```

Create `pkg/pack/compiler/runescript/server_script_compiler_test.go`:

```go
// pkg/pack/compiler/runescript/server_script_compiler_test.go
package runescript

import "testing"

// TestServerScriptCompiler_NewIsZeroValueSafe pins that constructing a fresh
// driver via the New* constructor (or struct literal with required fields)
// doesn't panic when its methods aren't yet called.
func TestServerScriptCompiler_StructIsConstructable(t *testing.T) {
	c := &ServerScriptCompiler{}
	if c == nil {
		t.Fatal("ServerScriptCompiler literal nil")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/runescript/ -run 'TestDefaultTypeCheckers|TestServerScriptCompiler_Struct' -v`
Expected: FAIL — `registerDefaultTypeCheckers` / `ServerScriptCompiler` undefined.

- [ ] **Step 3: Write `default_type_checkers.go`**

```go
// pkg/pack/compiler/runescript/default_type_checkers.go
package runescript

import (
	"reflect"

	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

// registerDefaultTypeCheckers installs the 7 default checkers on tm.
// Mirrors TS ScriptCompiler.setupDefaultTypeCheckers L121-204.
//
// The TS "allow Nothing on the right" checker is commented out in source
// (TS L130-131) — port the comment, not the code.
func registerDefaultTypeCheckers(tm *typ.TypeManager) {
	// 1) Anything → MetaAny.
	tm.AddTypeChecker(func(left, _ typ.Type) bool {
		return left == typ.MetaAny
	})

	// 2) Allow nothing → any (BOTTOM). TS L131-132 has this commented out.
	//   tm.AddTypeChecker(func(_, right typ.Type) bool { return right == typ.MetaNothing })

	// 3) Anything ↔ MetaError (propagation).
	tm.AddTypeChecker(func(left, right typ.Type) bool {
		return left == typ.MetaError || right == typ.MetaError
	})

	// 4) Reflexive equality.
	tm.AddTypeChecker(func(left, right typ.Type) bool {
		return left == right
	})

	// 5) MetaScript: matching trigger ident + recursive params/returns.
	tm.AddTypeChecker(func(left, right typ.Type) bool {
		lp, lr, lOK := typ.IsMetaScript(left)
		rp, rr, rOK := typ.IsMetaScript(right)
		if !lOK || !rOK {
			return false
		}
		// Recursive check on params and returns.
		return tm.Check(lp, rp) && tm.Check(lr, rr)
		// NOTE: TS additionally compares `left.trigger === right.trigger`.
		// goscape's NewMetaScript bakes the trigger identifier into the
		// representation string, so reflexive equality (checker 4) catches
		// identical triggers; this checker accepts param/return mismatches
		// when triggers match. If goscape adds trigger-storing MetaScript
		// later, extend here.
	})

	// 6) MetaHook: recursive on transmit list type.
	tm.AddTypeChecker(func(left, right typ.Type) bool {
		lh, lOK := typ.IsMetaHook(left)
		rh, rOK := typ.IsMetaHook(right)
		if !lOK || !rOK {
			return false
		}
		return tm.Check(lh, rh)
	})

	// 7) WrappedType: same Go type + recursive on Inner.
	tm.AddTypeChecker(func(left, right typ.Type) bool {
		lw, lOK := left.(typ.WrappedType)
		rw, rOK := right.(typ.WrappedType)
		if !lOK || !rOK {
			return false
		}
		if reflect.TypeOf(left) != reflect.TypeOf(right) {
			return false
		}
		return tm.Check(lw.Inner(), rw.Inner())
	})

	// 8) TupleType: equal child counts + recursive on every child.
	tm.AddTypeChecker(func(left, right typ.Type) bool {
		lc, lOK := typ.IsTupleType(left)
		rc, rOK := typ.IsTupleType(right)
		if !lOK || !rOK {
			return false
		}
		if len(lc) != len(rc) {
			return false
		}
		for i := range lc {
			if !tm.Check(lc[i], rc[i]) {
				return false
			}
		}
		return true
	})

	// 9) Representation-string fallback.
	tm.AddTypeChecker(func(left, right typ.Type) bool {
		return left.Representation() == right.Representation()
	})
}
```

The exact names of helper predicates (`typ.IsMetaScript`, `typ.IsMetaHook`, `typ.IsTupleType`) must be verified against actual goscape API. If `IsMetaHook` does not exist, add it inline:

```go
// In pkg/pack/compiler/type/meta.go (existing file):
func IsMetaHook(t Type) (transmitList Type, ok bool) {
	if h, ok := t.(*metaHook); ok {
		return h.transmitListType, true
	}
	return nil, false
}
```

Same for `IsTupleType`:

```go
// In pkg/pack/compiler/type/tuple.go (existing file):
func IsTupleType(t Type) (children []Type, ok bool) {
	if tt, ok := t.(*TupleType); ok {
		return tt.children, true
	}
	return nil, false
}
```

Create `pkg/pack/compiler/runescript/server_script_compiler.go`:

```go
// pkg/pack/compiler/runescript/server_script_compiler.go
package runescript

import (
	"github.com/zsrv/goscape/pkg/pack/compiler/diagnostics"
	"github.com/zsrv/goscape/pkg/pack/compiler/pointer"
	"github.com/zsrv/goscape/pkg/pack/compiler/semantics"
	"github.com/zsrv/goscape/pkg/pack/compiler/symbol"
	"github.com/zsrv/goscape/pkg/pack/compiler/trigger"
	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

// ServerScriptCompiler is the goscape port of TS ScriptCompiler +
// ServerScriptCompiler (single struct, no inheritance). Mirrors TS
// src/runescript/ServerScriptCompiler.ts.
//
// Setup() installs default type checkers + server triggers + script var
// types + dynamic command handlers + sym loaders; Run(ext) orchestrates
// parse → analyze → codegen → check-pointers → write. Both are defined
// in this file (Setup body in setup.go; Run in this file or a sibling).
type ServerScriptCompiler struct {
	SourcePaths  []string
	ExcludePaths []string

	Types           *typ.TypeManager
	Triggers        *trigger.TriggerManager
	RootTable       *symbol.SymbolTable
	DynHandlers     map[string]semantics.DynamicCommandHandler
	SymbolLoaders   []symbol.SymbolLoader
	CompilerSymbols map[string]*CompilerTypeInfo
	Mapper          *SymbolMapper
	CommandPointers map[string]*pointer.Holder
	Features        semantics.StrictFeatureLevel

	DiagHandler *diagnostics.Diagnostics

	BinaryWriter *BinaryScriptWriter
	Writer       BinaryOutput
}

// FindType satisfies symbol.CompilerContext.
func (c *ServerScriptCompiler) FindType(name string) typ.Type {
	return c.Types.FindOrNil(name, false)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/runescript/ -run 'TestDefaultTypeCheckers|TestServerScriptCompiler_Struct' -v`
Expected: PASS.

- [ ] **Step 5: Run full suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add pkg/pack/compiler/runescript/default_type_checkers.go pkg/pack/compiler/runescript/default_type_checkers_test.go pkg/pack/compiler/runescript/server_script_compiler.go pkg/pack/compiler/runescript/server_script_compiler_test.go
# Also stage any new IsMetaHook / IsTupleType helpers added to type/ as needed.
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(compiler/runescript): NAI-210 T12 — ServerScriptCompiler struct + default type checkers

Adds the ServerScriptCompiler struct shell (TS-flattened ScriptCompiler +
ServerScriptCompiler) and the registerDefaultTypeCheckers helper (7
checkers: any-top, error-propagation, reflexive, MetaScript, MetaHook,
WrappedType, TupleType, representation-string fallback).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task T13: `Setup()` body + sym-loader plumbing

**Files:**
- Create: `pkg/pack/compiler/runescript/setup.go`
- Create: `pkg/pack/compiler/runescript/setup_test.go`

**TS source:** `src/runescript/ServerScriptCompiler.ts` L60-212 (full `setup()` body).

This is the longest registration body in the slice. Read TS carefully and port line-by-line.

- [ ] **Step 1: Write the failing test**

Create `pkg/pack/compiler/runescript/setup_test.go`:

```go
// pkg/pack/compiler/runescript/setup_test.go
package runescript

import (
	"testing"

	"github.com/zsrv/goscape/pkg/pack/compiler/semantics"
	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

func newSetupTestCompiler(features semantics.StrictFeatureLevel) *ServerScriptCompiler {
	return &ServerScriptCompiler{
		Types:           typ.NewTypeManager(),
		Triggers:        nil, // populated in Setup
		CompilerSymbols: map[string]*CompilerTypeInfo{},
		Features:        features,
		DynHandlers:     map[string]semantics.DynamicCommandHandler{},
	}
}

// TestSetup_RegistersCorePrimitives pins that PrimitiveAll is registered.
func TestSetup_RegistersCorePrimitives(t *testing.T) {
	c := newSetupTestCompiler(semantics.StrictFeatureLevel{})
	c.Setup()
	if c.Types.FindOrNil("int", false) == nil {
		t.Error("'int' not registered after Setup")
	}
	if c.Types.FindOrNil("string", false) == nil {
		t.Error("'string' not registered after Setup")
	}
	if c.Types.FindOrNil("category", false) == nil {
		t.Error("'category' not registered after Setup")
	}
}

// TestSetup_RegistersAnyAndTypeAlias pins TS L107-108.
func TestSetup_RegistersAnyAndTypeAlias(t *testing.T) {
	c := newSetupTestCompiler(semantics.StrictFeatureLevel{})
	c.Setup()
	if c.Types.FindOrNil("any", false) == nil {
		t.Error("'any' not registered")
	}
	if c.Types.FindOrNil("type", false) == nil {
		t.Error("'type' alias not registered")
	}
}

// TestSetup_ProcsGate_DisableSkipsProc pins TS L80.
func TestSetup_ProcsGate_DisableSkipsProc(t *testing.T) {
	c := newSetupTestCompiler(semantics.StrictFeatureLevel{DisableProcs: true})
	c.Setup()
	if c.Types.FindOrNil("proc", false) != nil {
		t.Error("DisableProcs=true: 'proc' type should NOT be registered")
	}
}

// TestSetup_ProcsGate_DefaultRegistersProc pins TS L80 default.
func TestSetup_ProcsGate_DefaultRegistersProc(t *testing.T) {
	c := newSetupTestCompiler(semantics.StrictFeatureLevel{})
	c.Setup()
	if c.Types.FindOrNil("proc", false) == nil {
		t.Error("default: 'proc' type SHOULD be registered")
	}
}

// TestSetup_AddsSymLoadersForKnownCompilerSymbols pins addSymLoader.
func TestSetup_AddsSymLoadersForKnownCompilerSymbols(t *testing.T) {
	c := newSetupTestCompiler(semantics.StrictFeatureLevel{})
	c.CompilerSymbols["loc"] = &CompilerTypeInfo{Map: map[string]string{}}
	c.Setup()
	hasLoc := false
	for _, l := range c.SymbolLoaders {
		if _, ok := l.(*CompilerTypeInfoLoader); ok {
			hasLoc = true
			break
		}
	}
	if !hasLoc {
		t.Error("CompilerSymbols['loc'] present: expected at least one CompilerTypeInfoLoader")
	}
}

// TestSetup_AddsSymConstantLoader pins TS L97 (addSymConstantLoaders).
func TestSetup_AddsSymConstantLoader(t *testing.T) {
	c := newSetupTestCompiler(semantics.StrictFeatureLevel{})
	c.CompilerSymbols["constant"] = &CompilerTypeInfo{
		Map: map[string]string{"MAX": "99"},
	}
	c.Setup()
	hasConstantLoader := false
	for _, l := range c.SymbolLoaders {
		if _, ok := l.(*CompilerTypeInfoConstantLoader); ok {
			hasConstantLoader = true
			break
		}
	}
	if !hasConstantLoader {
		t.Error("CompilerSymbols['constant'] present: expected CompilerTypeInfoConstantLoader")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/runescript/ -run TestSetup -v`
Expected: FAIL.

- [ ] **Step 3: Write `setup.go`**

```go
// pkg/pack/compiler/runescript/setup.go
package runescript

import (
	"github.com/zsrv/goscape/pkg/pack/compiler/command"
	"github.com/zsrv/goscape/pkg/pack/compiler/semantics"
	"github.com/zsrv/goscape/pkg/pack/compiler/symbol"
	"github.com/zsrv/goscape/pkg/pack/compiler/trigger"
	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

// Setup installs the compiler's types, triggers, sym-loaders, and dynamic
// command handlers. Mirrors TS ScriptCompiler constructor body + ServerScriptCompiler.setup().
func (c *ServerScriptCompiler) Setup() {
	if c.Triggers == nil {
		c.Triggers = trigger.NewTriggerManager()
	}
	if c.RootTable == nil {
		c.RootTable = symbol.NewSymbolTable(nil)
	}
	if c.DynHandlers == nil {
		c.DynHandlers = map[string]semantics.DynamicCommandHandler{}
	}
	if c.CommandPointers == nil {
		c.CommandPointers = map[string]*pointer.Holder{}
	}

	// From TS ScriptCompiler constructor L96-115.
	for _, p := range typ.PrimitiveAll {
		_ = c.Types.RegisterByRepresentation(p)
	}
	_ = c.Types.Register("any", typ.MetaAny)
	_ = c.Types.Register("type", typ.MetaAny)
	registerDefaultTypeCheckers(c.Types)
	_ = c.Triggers.RegisterTrigger(trigger.CommandTrigger)

	// From TS ServerScriptCompiler.setup L60-212.
	_ = c.Triggers.RegisterAll(trigger.ServerTriggerTypeAll)

	c.registerScriptVarTypes()

	// TS L70: changeOptions('long', allowDeclaration=false, allowParameter=true).
	// Goscape PrimitiveLong is already configured AllowParameter=true, AllowSwitch=false,
	// AllowArray=false. The flip is AllowDeclaration=false. Apply via ChangeOptions.
	_ = c.Types.ChangeOptions("long", func(o *typ.TypeOptions) {
		o.AllowDeclaration = false
		o.AllowParameter = true
	})

	// TS L78-80: proc gated on DisableProcs.
	if !c.Features.DisableProcs {
		_ = c.Types.Register("proc", typ.NewMetaScript("proc", typ.MetaUnit, typ.MetaUnit))
	}
	_ = c.Types.Register("label", typ.NewMetaScript("label", typ.MetaUnit, typ.MetaNothing))

	// TS L83-84: namedobj → obj.
	c.Types.AddTypeChecker(func(left, right typ.Type) bool {
		return left == typ.ScriptVarObj && right == typ.ScriptVarNamedObj
	})

	c.addSymConstantLoaders()

	// TS L88-93: walktrigger, queue, timer.
	_ = c.Types.Register("walktrigger", typ.NewMetaScript("walktrigger", typ.MetaAny, typ.MetaNothing))
	_ = c.Types.Register("queue", typ.NewMetaScript("queue", typ.MetaAny, typ.MetaNothing))
	_ = c.Types.Register("timer", typ.NewMetaScript("timer", typ.MetaAny, typ.MetaNothing))

	// TS L110-176 — sym-loaders (~25 entries). Each gated on CompilerSymbols[name] presence.
	c.addSymLoader("loc", typ.ScriptVarLoc)
	c.addSymLoader("npc", typ.ScriptVarNpc)
	c.addSymLoader("obj", typ.ScriptVarNamedObj) // TS uses NAMEDOBJ for 'obj' sym-loader.
	c.addSymLoader("component", typ.ScriptVarComponent)
	c.addSymLoader("interface", typ.ScriptVarInterface)
	c.addSymLoader("overlayinterface", typ.ScriptVarOverlayInterface)
	c.addSymLoader("fontmetrics", typ.ScriptVarFontMetrics)
	c.addSymLoader("category", typ.PrimitiveCategory)
	c.addSymLoader("hunt", typ.ScriptVarHunt)
	c.addSymLoader("inv", typ.ScriptVarInv)
	c.addSymLoader("idk", typ.ScriptVarIdKit)
	c.addSymLoader("mesanim", typ.ScriptVarMesAnim)
	// param + intparam — TS L138-140 registers ParamType wrappers.
	_ = c.Types.Register("param", typ.NewParamType(typ.MetaAny))
	_ = c.Types.Register("intparam", typ.NewParamType(typ.PrimitiveInt))
	c.addSymLoaderWithSupplier("param", func(sub typ.Type) typ.Type {
		return typ.NewParamType(sub)
	})
	c.addSymLoader("seq", typ.ScriptVarSeq)
	c.addSymLoader("spotanim", typ.ScriptVarSpotAnim)

	// TS L147-149: varp + protected loader.
	_ = c.Types.Register("varp", typ.NewVarPlayerType(typ.MetaAny))
	c.addProtectedSymLoaderWithSupplier("varp", func(sub typ.Type) typ.Type {
		return typ.NewVarPlayerType(sub)
	})
	c.addSymLoaderWithSupplier("varn", func(sub typ.Type) typ.Type {
		return typ.NewVarNpcType(sub)
	})
	c.addSymLoaderWithSupplier("vars", func(sub typ.Type) typ.Type {
		return typ.NewVarSharedType(sub)
	})

	c.addSymLoader("stat", typ.ScriptVarStat)
	c.addSymLoader("locshape", typ.ScriptVarLocShape)
	c.addSymLoader("movespeed", typ.ScriptVarMoveSpeed)
	c.addSymLoader("npc_mode", typ.ScriptVarNpcMode)
	c.addSymLoader("npc_stat", typ.ScriptVarNpcStat)
	c.addSymLoader("model", typ.ScriptVarModel)
	c.addSymLoader("synth", typ.ScriptVarSynth)
	c.addSymLoader("midi", typ.ScriptVarMidi)
	c.addSymLoader("jingle", typ.ScriptVarJingle)

	// TS L168-170: varbit + protected loader.
	_ = c.Types.Register("varbit", typ.NewVarBitType(typ.MetaAny))
	c.addProtectedSymLoaderWithSupplier("varbit", func(sub typ.Type) typ.Type {
		return typ.NewVarBitType(sub)
	})

	// TS L182-189: enum gated on DisableEnums.
	if !c.Features.DisableEnums {
		c.addSymLoader("enum", typ.ScriptVarEnum)
	}

	// TS L190-194: structs gated on DisableStructs.
	if !c.Features.DisableStructs {
		c.addSymLoader("struct", typ.ScriptVarStruct)
	}

	_ = c.Types.Register("softtimer", typ.NewMetaScript("softtimer", typ.MetaAny, typ.MetaNothing))

	// TS L199-212: dbtables gated on DisableDBTables.
	if !c.Features.DisableDBTables {
		_ = c.Types.Register("dbcolumn", typ.NewDbColumnType(typ.MetaAny))
		c.addSymLoaderWithSupplier("dbcolumn", func(sub typ.Type) typ.Type {
			return typ.NewDbColumnType(sub)
		})
		c.addSymLoader("dbrow", typ.ScriptVarDbRow)
		c.addSymLoader("dbtable", typ.ScriptVarDbTable)
	}

	// Dynamic command handlers — inner half, retired NAI-207-D-REGISTERALL-NO-FEATURES.
	command.RegisterAllDynCommands(c.Types, c.Features, func(name string, h semantics.DynamicCommandHandler) {
		c.DynHandlers[name] = h
	})
}

// registerScriptVarTypes iterates typ.ScriptVarTypeAll and registers each
// entry, skipping enum/struct/dbrow/dbtable per features. Mirrors TS
// ServerScriptCompiler.registerScriptVarTypes L218-225.
func (c *ServerScriptCompiler) registerScriptVarTypes() {
	for _, t := range typ.ScriptVarTypeAll {
		if c.Features.DisableEnums && t == typ.ScriptVarEnum {
			continue
		}
		if c.Features.DisableStructs && t == typ.ScriptVarStruct {
			continue
		}
		if c.Features.DisableDBTables && (t == typ.ScriptVarDbRow || t == typ.ScriptVarDbTable) {
			continue
		}
		_ = c.Types.RegisterByRepresentation(t)
	}
}

// addSymLoader gates on presence of CompilerSymbols[name] and appends a
// CompilerTypeInfoLoader. Mirrors TS addSymLoader L232.
func (c *ServerScriptCompiler) addSymLoader(name string, t typ.Type) {
	c.addSymLoaderWithSupplier(name, func(_ typ.Type) typ.Type { return t })
}

func (c *ServerScriptCompiler) addSymLoaderWithSupplier(name string, ts func(typ.Type) typ.Type) {
	info, ok := c.CompilerSymbols[name]
	if !ok {
		return
	}
	c.SymbolLoaders = append(c.SymbolLoaders, &CompilerTypeInfoLoader{
		Mapper:       c.Mapper,
		Symbols:      info,
		TypeSupplier: ts,
	})
}

func (c *ServerScriptCompiler) addProtectedSymLoaderWithSupplier(name string, ts func(typ.Type) typ.Type) {
	info, ok := c.CompilerSymbols[name]
	if !ok {
		return
	}
	c.SymbolLoaders = append(c.SymbolLoaders, &CompilerTypeInfoProtectedLoader{
		Mapper:       c.Mapper,
		Symbols:      info,
		TypeSupplier: ts,
	})
}

func (c *ServerScriptCompiler) addSymConstantLoaders() {
	info, ok := c.CompilerSymbols["constant"]
	if !ok {
		return
	}
	c.SymbolLoaders = append(c.SymbolLoaders, &CompilerTypeInfoConstantLoader{Symbols: info})
}
```

Imports must include `"github.com/zsrv/goscape/pkg/pack/compiler/pointer"` (in the `c.CommandPointers = map[...]{}` initializer). Add to the import block.

- [ ] **Step 4: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/runescript/ -run TestSetup -v`
Expected: PASS.

- [ ] **Step 5: Run full suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add pkg/pack/compiler/runescript/setup.go pkg/pack/compiler/runescript/setup_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(compiler/runescript): NAI-210 T13 — ServerScriptCompiler.Setup body

Ports TS ScriptCompiler constructor L96-115 + ServerScriptCompiler.setup
L60-212: registers primitives, defaults, server triggers, ScriptVarType
enum, sym-loaders (gated on CompilerSymbols presence), proc/label/queue/
timer/softtimer MetaScript types, varp/varbit/varn/vars/dbcolumn wrappers,
and dynamic command handlers via command.RegisterAllDynCommands. All
feature gates honored.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task T14: `Run(ext)` pipeline

**Files:**
- Modify: `pkg/pack/compiler/runescript/server_script_compiler.go`
- Modify: `pkg/pack/compiler/runescript/server_script_compiler_test.go`

**TS source:** `src/compiler/ScriptCompiler.ts` L220-260 (`run` + `compile`) + L268-302 (`parse`) + L307-355 (`analyze`) + L361-379 (`codegen`) + L388-406 (`checkPointers`) + L411-430 (`write`).

The pipeline reuses existing packages — `parser`, `semantics.ScriptRegistration`, `semantics.TypeChecking`, `codegen.NewCodeGenerator`, `runescript.ServerPointerChecker`, `BinaryScriptWriter.WriteScript`. The driver glues them.

- [ ] **Step 1: Write failing tests**

Append to `server_script_compiler_test.go`:

```go
// TestServerScriptCompiler_Run_EmptySourcePathReturnsNoError pins that the
// driver returns nil (success) when SourcePaths is empty.
func TestServerScriptCompiler_Run_EmptySourcePathReturnsNoError(t *testing.T) {
	mapper := NewSymbolMapper(nil)
	c := &ServerScriptCompiler{
		SourcePaths:     []string{},
		Types:           typ.NewTypeManager(),
		Mapper:          mapper,
		CommandPointers: map[string]*pointer.Holder{"foo": {Required: pointer.NewPointerSet()}},
		Writer:          &noopBinaryOutput{},
	}
	c.Setup()
	c.BinaryWriter = NewBinaryScriptWriter(mapper, nil)
	c.BinaryWriter.Output = c.Writer

	if err := c.Run("rs2"); err != nil {
		t.Errorf("Run on empty source paths: got error %v, want nil", err)
	}
}

// TestServerScriptCompiler_Run_EmptyCommandPointers_HaltsBeforeWrite pins
// NAI-210-D-EMPTYPOINTERS-RETURNS-FALSE: TS-faithful early-return false.
func TestServerScriptCompiler_Run_EmptyCommandPointers_HaltsBeforeWrite(t *testing.T) {
	mapper := NewSymbolMapper(nil)
	sink := &noopBinaryOutput{}
	c := &ServerScriptCompiler{
		SourcePaths:     []string{},
		Types:           typ.NewTypeManager(),
		Mapper:          mapper,
		CommandPointers: map[string]*pointer.Holder{}, // EMPTY
		Writer:          sink,
	}
	c.Setup()
	c.BinaryWriter = NewBinaryScriptWriter(mapper, nil)
	c.BinaryWriter.Output = c.Writer

	_ = c.Run("rs2")
	// sink should have received zero writes.
	if sink.writeCount != 0 {
		t.Errorf("empty commandPointers: sink got %d writes, want 0", sink.writeCount)
	}
}

type noopBinaryOutput struct {
	writeCount int
}

func (n *noopBinaryOutput) OutputScript(s *codegen.RuneScript, data []byte) { n.writeCount++ }
```

The imports for the test file need to include `pointer`, `codegen`. Add as needed.

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/runescript/ -run TestServerScriptCompiler_Run -v`
Expected: FAIL — `Run` undefined.

- [ ] **Step 3: Add `Run(ext)` to `server_script_compiler.go`**

Append to `pkg/pack/compiler/runescript/server_script_compiler.go`:

```go
import (
	// ... existing imports ...
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/zsrv/goscape/pkg/pack/compiler/ast"
	"github.com/zsrv/goscape/pkg/pack/compiler/codegen"
	"github.com/zsrv/goscape/pkg/pack/compiler/parser"
	// existing imports stay
)

// Run executes the compile pipeline. Mirrors TS ScriptCompiler.run +
// compile L220-260.
func (c *ServerScriptCompiler) Run(ext string) error {
	if err := c.loadSymbols(); err != nil {
		return err
	}

	files, err := c.parsePhase(ext)
	if err != nil {
		return err
	}

	if err := c.analyzePhase(files); err != nil {
		return err
	}

	scripts, err := c.codegenPhase(files)
	if err != nil {
		return err
	}

	if c.checkPointersPhase(scripts) {
		return nil // NAI-210-D-EMPTYPOINTERS-RETURNS-FALSE early-return
	}

	c.writePhase(scripts)

	if closer, ok := c.Writer.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}

func (c *ServerScriptCompiler) loadSymbols() error {
	for _, l := range c.SymbolLoaders {
		if err := l.Load(c.RootTable, c); err != nil {
			return err
		}
	}
	return nil
}

func (c *ServerScriptCompiler) parsePhase(ext string) ([]*ast.ScriptFile, error) {
	var files []*ast.ScriptFile
	for _, sourcePath := range c.SourcePaths {
		err := filepath.Walk(sourcePath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			if !strings.HasSuffix(path, "."+ext) {
				return nil
			}
			node, perr := parser.ParseFile(path)
			if perr != nil {
				return perr
			}
			if node != nil {
				files = append(files, node)
			}
			return nil
		})
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}
	return files, nil
}

func (c *ServerScriptCompiler) analyzePhase(files []*ast.ScriptFile) error {
	// Phase 1: ScriptRegistration. Each file is visited.
	reg := semantics.NewScriptRegistration(c.Types, c.Triggers, c.RootTable, c.Features)
	for _, f := range files {
		f.Accept(reg)
	}

	c.registerSecondaryCommands()

	// Phase 2: TypeChecking.
	tc := semantics.NewTypeChecking(c.Types, c.Triggers, c.RootTable, c.DynHandlers, c.Features)
	for _, f := range files {
		f.Accept(tc)
	}

	if reg.HasErrors() || tc.HasErrors() {
		return fmt.Errorf("analyze: diagnostics reported errors")
	}
	return nil
}

func (c *ServerScriptCompiler) registerSecondaryCommands() {
	if len(c.CommandPointers) < 1 {
		return
	}
	commandType := symbol.SymbolTypeServerScript(trigger.CommandTrigger)
	for name := range c.CommandPointers {
		if !strings.HasPrefix(name, ".") {
			continue
		}
		baseName := name[1:]
		if baseName == "" {
			continue
		}
		baseSym := c.RootTable.Find(commandType, baseName)
		if baseSym == nil {
			continue
		}
		base, ok := baseSym.(*symbol.ScriptSymbol)
		if !ok {
			continue
		}
		if c.RootTable.Find(commandType, name) != nil {
			continue
		}
		alias := &symbol.ScriptSymbol{
			BaseTriggerType: trigger.CommandTrigger,
			BaseName:        name,
			BaseParameters:  base.BaseParameters,
			BaseReturns:     base.BaseReturns,
		}
		c.RootTable.Insert(commandType, alias)
	}
}

func (c *ServerScriptCompiler) codegenPhase(files []*ast.ScriptFile) ([]*codegen.RuneScript, error) {
	var scripts []*codegen.RuneScript
	for _, f := range files {
		gen := codegen.NewCodeGenerator(c.RootTable, c.DynHandlers)
		f.Accept(gen)
		if gen.HasErrors() {
			return nil, fmt.Errorf("codegen: diagnostics reported errors")
		}
		scripts = append(scripts, gen.Scripts...)
	}
	return scripts, nil
}

// checkPointersPhase returns true iff the pipeline should halt before write.
// Mirrors TS ScriptCompiler.checkPointers L388-406 — empty commandPointers
// returns false (TS), which here means "halt".
// NAI-210-D-EMPTYPOINTERS-RETURNS-FALSE.
func (c *ServerScriptCompiler) checkPointersPhase(scripts []*codegen.RuneScript) (halt bool) {
	if len(c.CommandPointers) < 1 {
		return true
	}
	checker := NewServerPointerChecker(c.RootTable, scripts, c.CommandPointers, c.Features)
	if err := checker.Run(); err != nil {
		return true
	}
	return false
}

func (c *ServerScriptCompiler) writePhase(scripts []*codegen.RuneScript) {
	for _, s := range scripts {
		if c.isExcluded(s.SourceName) {
			continue
		}
		c.BinaryWriter.WriteScript(s)
	}
}

func (c *ServerScriptCompiler) isExcluded(sourceName string) bool {
	abs, err := filepath.Abs(sourceName)
	if err != nil {
		return false
	}
	for _, excluded := range c.ExcludePaths {
		excludedClean := filepath.Clean(excluded)
		if abs == excludedClean {
			return true
		}
		if strings.HasPrefix(abs, excludedClean+string(filepath.Separator)) {
			return true
		}
	}
	return false
}
```

Several names here are best-effort guesses:
- `parser.ParseFile(path)` — exact name verified at plan-write by reading `pkg/pack/compiler/parser/`. If it's `parser.Parse(...)` or `ScriptParser.Parse(...)`, adapt.
- `semantics.NewScriptRegistration(...)` / `semantics.NewTypeChecking(...)` — verify by reading `pkg/pack/compiler/semantics/`.
- `gen.HasErrors() / gen.Scripts` — depend on existing codegen API.
- `ast.ScriptFile.Accept(visitor)` — depend on existing AST visitor interface.

**This is the longest single task in the slice.** Subagent must read each consumed package's surface API before writing the implementation. Adapt names as required; do NOT invent.

- [ ] **Step 4: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/runescript/ -run TestServerScriptCompiler_Run -v`
Expected: PASS.

- [ ] **Step 5: Run full suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add pkg/pack/compiler/runescript/server_script_compiler.go pkg/pack/compiler/runescript/server_script_compiler_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(compiler/runescript): NAI-210 T14 — ServerScriptCompiler.Run pipeline

Ports TS ScriptCompiler.run + compile + parse + analyze (incl.
registerSecondaryCommands) + codegen + checkPointers + write. Empty
commandPointers triggers TS-faithful halt-before-write
(NAI-210-D-EMPTYPOINTERS-RETURNS-FALSE).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task T15: `Compile(cfg)` facade + driver smoke (Jag + Js5)

**Files:**
- Create: `pkg/pack/compiler/runescript/compile.go`
- Create: `pkg/pack/compiler/runescript/compile_test.go`

**TS source:** `src/runescript/ServerScriptCompilerApplication.ts` L13-91 (`CompileServerScript`).

- [ ] **Step 1: Write the failing test**

Create `pkg/pack/compiler/runescript/compile_test.go`:

```go
// pkg/pack/compiler/runescript/compile_test.go
package runescript

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zsrv/goscape/pkg/pack/compiler/semantics"
)

// TestCompile_MissingCoreSymbols_ReturnsError pins TS L16-18: error when
// command or runescript symbol info is missing.
func TestCompile_MissingCoreSymbols_ReturnsError(t *testing.T) {
	cases := []struct {
		name    string
		symbols map[string]*CompilerTypeInfo
	}{
		{"nil map", nil},
		{"command only", map[string]*CompilerTypeInfo{"command": {Map: map[string]string{}}}},
		{"runescript only", map[string]*CompilerTypeInfo{"runescript": {Map: map[string]string{}}}},
		{"empty map", map[string]*CompilerTypeInfo{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := Compile(Config{Symbols: c.symbols})
			if err == nil {
				t.Errorf("Compile %s: want error, got nil", c.name)
			}
		})
	}
}

// TestCompile_JagWriter_EndToEnd compiles a trivial fixture and verifies
// script.dat + script.idx exist.
//
// CRITICAL: At least one commandInfo entry must have non-empty
// Require/Set/Corrupt to populate commandPointers — otherwise checkPointers
// halts before write (NAI-210-D-EMPTYPOINTERS-RETURNS-FALSE).
func TestCompile_JagWriter_EndToEnd(t *testing.T) {
	tmp := t.TempDir()
	scriptsDir := filepath.Join(tmp, "scripts")
	if err := os.MkdirAll(scriptsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(scriptsDir, "hello.rs2"),
		[]byte("[proc,hello]\nreturn;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	packDir := filepath.Join(tmp, "pack")

	cfg := Config{
		SourcePaths: []string{scriptsDir},
		Symbols: map[string]*CompilerTypeInfo{
			"command":    {Map: map[string]string{"0": "return"}, Require: map[string]string{"0": "active_player"}},
			"runescript": {Map: map[string]string{}},
		},
		Features: semantics.StrictFeatureLevel{},
		Writer:   WriterConfig{Jag: &JagWriterConfig{Output: packDir}},
	}
	if err := Compile(cfg); err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if _, err := os.Stat(filepath.Join(packDir, "script.dat")); err != nil {
		t.Errorf("script.dat not produced: %v", err)
	}
	if _, err := os.Stat(filepath.Join(packDir, "script.idx")); err != nil {
		t.Errorf("script.idx not produced: %v", err)
	}
}

// TestCompile_Js5Writer_EndToEnd ports the same scenario for Js5 writer.
func TestCompile_Js5Writer_EndToEnd(t *testing.T) {
	tmp := t.TempDir()
	scriptsDir := filepath.Join(tmp, "scripts")
	if err := os.MkdirAll(scriptsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(scriptsDir, "hello.rs2"),
		[]byte("[proc,hello]\nreturn;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	js5Out := filepath.Join(tmp, "pack", "scripts.js5")

	cfg := Config{
		SourcePaths: []string{scriptsDir},
		Symbols: map[string]*CompilerTypeInfo{
			"command":    {Map: map[string]string{"0": "return"}, Require: map[string]string{"0": "active_player"}},
			"runescript": {Map: map[string]string{}},
		},
		Features: semantics.StrictFeatureLevel{},
		Writer:   WriterConfig{Js5: &Js5WriterConfig{Output: js5Out}},
	}
	if err := Compile(cfg); err != nil {
		t.Fatalf("Compile: %v", err)
	}
	body, err := os.ReadFile(js5Out)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if len(body) < 20 {
		t.Errorf(".js5 produced but too short: %d bytes", len(body))
	}
}
```

The fixture script `[proc,hello]\nreturn;` must parse successfully. If the parser rejects this exact form (e.g., requires a different return statement syntax), simplify until it parses. Alternative: register `return` as a recognised command in the symbol table.

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/runescript/ -run TestCompile -v`
Expected: FAIL — `Compile` undefined.

- [ ] **Step 3: Write `compile.go`**

```go
// pkg/pack/compiler/runescript/compile.go
package runescript

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/zsrv/goscape/pkg/pack/compiler/pointer"
	"github.com/zsrv/goscape/pkg/pack/compiler/semantics"
	"github.com/zsrv/goscape/pkg/pack/compiler/symbol"
	"github.com/zsrv/goscape/pkg/pack/compiler/trigger"
	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

// Config drives Compile.
type Config struct {
	SourcePaths   []string
	ExcludePaths  []string
	Symbols       map[string]*CompilerTypeInfo
	CheckPointers *bool // nil → default true
	Features      semantics.StrictFeatureLevel
	Writer        WriterConfig
}

type WriterConfig struct {
	Jag *JagWriterConfig
	Js5 *Js5WriterConfig
}

type JagWriterConfig struct{ Output string }
type Js5WriterConfig struct{ Output string }

// Compile drives the full ServerScriptCompiler pipeline. Mirrors TS
// CompileServerScript (ServerScriptCompilerApplication.ts L13-91).
func Compile(cfg Config) error {
	if cfg.Symbols == nil || cfg.Symbols["command"] == nil || cfg.Symbols["runescript"] == nil {
		return errors.New("core symbols missing from compiler: provide command and runescript symbols")
	}

	sourcePaths := cfg.SourcePaths
	if len(sourcePaths) == 0 {
		sourcePaths = []string{"../content/scripts"}
	}
	excludePaths := cfg.ExcludePaths
	checkPointers := true
	if cfg.CheckPointers != nil {
		checkPointers = *cfg.CheckPointers
	}

	jag := cfg.Writer.Jag
	js5 := cfg.Writer.Js5
	if jag != nil && js5 != nil {
		return errors.New("only one of writer.jag / writer.js5 may be set")
	}
	if jag == nil && js5 == nil {
		jag = &JagWriterConfig{Output: "./data/pack/server"}
	}

	absSources, err := absAll(sourcePaths)
	if err != nil {
		return err
	}
	absExcludes, err := absAll(excludePaths)
	if err != nil {
		return err
	}

	mapper := NewSymbolMapper(nil)
	var writer BinaryOutput

	if jag != nil {
		absOut, err := filepath.Abs(jag.Output)
		if err != nil {
			return err
		}
		w, err := NewJagFileScriptWriter(absOut, mapper, nil)
		if err != nil {
			return err
		}
		writer = w
	} else {
		absOut, err := filepath.Abs(js5.Output)
		if err != nil {
			return err
		}
		w, err := NewJs5PackScriptWriter(absOut, mapper, nil)
		if err != nil {
			return err
		}
		writer = w
	}

	commandPointers := map[string]*pointer.Holder{}
	if err := LoadSpecialSymbols(cfg.Symbols["command"], cfg.Symbols["runescript"], mapper, commandPointers, checkPointers); err != nil {
		return fmt.Errorf("LoadSpecialSymbols: %w", err)
	}

	c := &ServerScriptCompiler{
		SourcePaths:     absSources,
		ExcludePaths:    absExcludes,
		Types:           typ.NewTypeManager(),
		Triggers:        trigger.NewTriggerManager(),
		RootTable:       symbol.NewSymbolTable(nil),
		DynHandlers:     map[string]semantics.DynamicCommandHandler{},
		CompilerSymbols: cfg.Symbols,
		Mapper:          mapper,
		CommandPointers: commandPointers,
		Features:        cfg.Features,
		Writer:          writer,
	}
	c.Setup()
	c.BinaryWriter = NewBinaryScriptWriter(mapper, nil)
	c.BinaryWriter.Output = c.Writer

	return c.Run("rs2")
}

func absAll(paths []string) ([]string, error) {
	out := make([]string, len(paths))
	for i, p := range paths {
		abs, err := filepath.Abs(p)
		if err != nil {
			return nil, err
		}
		out[i] = abs
	}
	return out, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/runescript/ -run TestCompile -v`
Expected: PASS.

- [ ] **Step 5: Run full suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`
Expected: PASS.

- [ ] **Step 6: Run with -race**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...`
Expected: PASS (or only pre-existing failures unchanged).

- [ ] **Step 7: Commit**

```bash
git add pkg/pack/compiler/runescript/compile.go pkg/pack/compiler/runescript/compile_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(compiler/runescript): NAI-210 T15 — runescript.Compile(cfg) facade + driver smoke

Ports TS CompileServerScript inner logic: validates core symbols, defaults
CheckPointers and writer selection, resolves paths, constructs
SymbolMapper + writer + pointer-populator, runs Setup + Run end-to-end.
Driver smoke compiles a trivial [proc,hello] fixture and pins production
of script.dat + script.idx (Jag) and .js5 (Js5).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task T16: Deviation-pin updates + close

**Files:**
- Create: `pkg/pack/compiler/runescript/nai210_deviation_pins_test.go`
- Modify: `pkg/pack/compiler/runescript/nai209_deviation_pins_test.go`
- Modify: `pkg/pack/compiler/codegen/nai207_deviation_pins_test.go`
- Modify: `pkg/pack/compiler/cfg/nai208_deviation_pins_test.go` (only if NAI-208-D-COMMAND-POINTERS-DEFERRED is pinned there; verify by grep first)

- [ ] **Step 1: Write the NAI-210 pin file**

Create `pkg/pack/compiler/runescript/nai210_deviation_pins_test.go`:

```go
// pkg/pack/compiler/runescript/nai210_deviation_pins_test.go
package runescript

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// TestDeviationPinsLive_NAI210 grep-pins every NAI-210-introduced deviation
// tag to a production touch point. Each tag must appear at least once in
// pkg/ + modules/ + cmd/. Mirrors the NAI-209 / NAI-208 pin pattern.
func TestDeviationPinsLive_NAI210(t *testing.T) {
	tags := []struct{ Tag, Why string }{
		{"NAI-210-D-GZIP-OS-BYTE-ZEROED", "Go compress/gzip writes host OS byte; zeroed for TS-equivalent reproducibility"},
		{"NAI-210-D-LOADER-SORTED-ITERATION", "Go map iteration randomized; sort by id for byte-identical SymbolMapper"},
		{"NAI-210-D-EMPTYPOINTERS-RETURNS-FALSE", "TS-faithful early-return false on empty commandPointers"},
	}

	cwd, _ := os.Getwd()
	// Walk up to repo root.
	root := cwd
	for {
		parent := strings.TrimSuffix(root, "/pkg/pack/compiler/runescript")
		if parent != root {
			root = parent
			break
		}
		break
	}

	for _, tag := range tags {
		t.Run(tag.Tag, func(t *testing.T) {
			cmd := exec.Command("rg", "-l", tag.Tag, "pkg", "modules", "cmd")
			cmd.Dir = root
			out, _ := cmd.Output()
			files := strings.Fields(string(out))
			productionHit := false
			for _, f := range files {
				if !strings.HasSuffix(f, "_test.go") && !strings.Contains(f, "nai210_deviation_pins_test.go") {
					productionHit = true
					break
				}
			}
			if !productionHit {
				t.Errorf("tag %s has no production touch point: %v", tag.Tag, files)
			}
		})
	}
}
```

- [ ] **Step 2: Edit `nai209_deviation_pins_test.go` — drop 2 retired tags**

Read the file; locate the `tags := []struct{...}{ ... }` literal. Delete the two entries:

```go
{"NAI-209-D-BYTEPACKET-DEFER", "BytePacket deferred to NAI-210"},
{"NAI-209-D-TYPEMARKER-CATEGORY-DISCRIMINATOR", "TS per-subject vs goscape per-trigger semantic"},
```

The remaining 9 NAI-209 tags stay in the table.

- [ ] **Step 3: Edit `codegen/nai207_deviation_pins_test.go` — drop 1 retired tag**

Locate the entry `"NAI-207-D-REGISTERALL-NO-FEATURES"` and remove it.

- [ ] **Step 4: Edit `cfg/nai208_deviation_pins_test.go` (only if NAI-208-D-COMMAND-POINTERS-DEFERRED is present there)**

Grep first: `rg "NAI-208-D-COMMAND-POINTERS-DEFERRED" pkg/pack/compiler/`.
If found in a pin test, drop the entry. If found only in doc comments / source code, the comments still need to be updated (delete the `NAI-208-D-COMMAND-POINTERS-DEFERRED` paragraphs and tag references — these were noted as live in spec section 13).

- [ ] **Step 5: Audit doc-comments for retired-tag references**

Run: `rg "NAI-209-D-BYTEPACKET-DEFER|NAI-209-D-TYPEMARKER-CATEGORY-DISCRIMINATOR|NAI-208-D-COMMAND-POINTERS-DEFERRED|NAI-207-D-REGISTERALL-NO-FEATURES" pkg/ modules/ cmd/`
For each remaining hit in a production `.go` file (non-`_test.go`), delete the doc-comment paragraph that references the now-retired tag.

- [ ] **Step 6: Run full suite + race**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`
Expected: PASS.

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...`
Expected: PASS (or only pre-existing race failures unchanged).

- [ ] **Step 7: Commit close**

```bash
git add pkg/pack/compiler/runescript/nai210_deviation_pins_test.go \
        pkg/pack/compiler/runescript/nai209_deviation_pins_test.go \
        pkg/pack/compiler/codegen/nai207_deviation_pins_test.go \
        pkg/pack/compiler/cfg/nai208_deviation_pins_test.go \
        pkg/pack/compiler/runescript/binary_writer.go \
        pkg/pack/compiler/runescript/binary_context.go \
        pkg/pack/compiler/command/register.go \
        pkg/pack/compiler/runescript/server_pointer_checker.go
# Stage any other files where retired-tag doc-comments were trimmed.
git commit --no-gpg-sign -m "$(cat <<'EOF'
close(compiler/runescript): NAI-210 T16 — deviation pins + close

Closes NAI-210 (compiler slice 6c of 6, FINAL). Retires four deviation tags:

  - NAI-207-D-REGISTERALL-NO-FEATURES — feature gates wired in T6
  - NAI-208-D-COMMAND-POINTERS-DEFERRED — LoadSpecialSymbols lands in T11
  - NAI-209-D-BYTEPACKET-DEFER — ByteWriter lands in T1
  - NAI-209-D-TYPEMARKER-CATEGORY-DISCRIMINATOR — per-subject category check in T5

Adds three new deviation tags pinned by nai210_deviation_pins_test.go:

  - NAI-210-D-GZIP-OS-BYTE-ZEROED — compress/gzip OS-byte zeroed
  - NAI-210-D-LOADER-SORTED-ITERATION — sorted-by-id loader iteration
  - NAI-210-D-EMPTYPOINTERS-RETURNS-FALSE — TS-faithful empty-commandPointers halt

Closes memory: nai210_close.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Post-Close

After T16 lands, save a `[[nai210_close]]` memory entry summarising:
- Final commit SHA
- Live deviation-tag count across the compiler
- Open follow-ups (e.g., if `SubjectMode.Type` arrays don't roundtrip in ServerTriggerType, or any other half-ported surface caught during T14)
- Note any test fixtures that ended up exercising slimmer-than-TS parser/codegen paths (e.g., the smoke fixture's `[proc,hello]\nreturn;` if the smoke had to use a minimal form due to grammar limitations).

## Spec Coverage Self-Review

| Spec section | Implementing task | Coverage |
|---|---|---|
| §4 BytePacket | T1 | Full |
| §5.1 BinaryFileScriptWriter | T2 | Full |
| §5.2 JagFileScriptWriter | T3 | Full |
| §5.3 Js5PackScriptWriter + GZIP byte-9 zero | T4 | Full (deviation tag pinned in T16) |
| §6.1 CompilerTypeInfo | T9 | Full |
| §6.2 SymbolLoader interface | T9 | Full |
| §6.3 Three loaders | T10 | Full |
| §7 LoadSpecialSymbols | T11 | Full |
| §8.1 ServerScriptCompiler struct | T12 | Full |
| §8.2 Setup() | T13 | Full |
| §8.3 setupDefaultTypeCheckers | T12 | Full |
| §8.4 Run(ext) pipeline + EMPTYPOINTERS deviation | T14 | Full |
| §9 Compile(cfg) facade | T15 | Full |
| §10 Feature-gating | T6 | Full |
| §11 PrimitiveCategory + lookup-key fix | T5 | Full |
| §12 Testing strategy | T1-T15 (tests embedded per task) + T16 (pins) | Full |
| §13 Deviation accounting | T16 | Full |
| **Extras beyond spec:** ScriptVarType (T7) + ServerTriggerType (T8) | T7, T8 | Added to plan as prerequisites surfaced at plan-write |

The plan adds two tasks (T7, T8) not in the spec's task decomposition but explicitly anticipated by the spec's §15 "Risks / open items" section. Plan-author judgment per [[controller_preflight]]: these are pure data-table ports and the slice cannot otherwise be self-contained.
