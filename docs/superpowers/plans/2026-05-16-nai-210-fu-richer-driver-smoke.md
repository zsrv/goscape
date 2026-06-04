# NAI-210-FU Richer Driver Smoke Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the two header-only driver smokes in `pkg/pack/compiler/runescript/compile_test.go` with single-proc fixtures that exercise the full `Compile()` pipeline, and pin deterministic header bytes from the on-disk Jag and Js5 archive files.

**Architecture:** Test-only follow-up. Two existing test functions (`TestCompile_JagWriter_EndToEnd`, `TestCompile_Js5Writer_EndToEnd`) are renamed to `..._PinsScriptHeader` and rewritten in place. Each test writes `helper.rs2` to a temp scripts dir, calls `runescript.Compile()`, then reads the produced archive file and asserts header byte invariants. No production code changes.

**Tech Stack:** Go 1.26+, project test layout `pkg/pack/compiler/runescript/`, standard `testing` + `os` + `encoding/binary` packages. No new dependencies.

**Spec:** `docs/superpowers/specs/2026-05-16-nai-210-fu-richer-driver-smoke-design.md` (committed at `2dfa5db`).

**Pre-flight state (HEAD `2dfa5db`):**

- `compile_test.go` has 5 tests: `TestCompile_MissingCoreSymbols_ReturnsError`, `TestCompile_JagWriter_EndToEnd`, `TestCompile_Js5Writer_EndToEnd`, `TestCompile_HandlerInjectionUsedDuringRun`, `TestCompile_NilHandlerDefaultsToBase`. The first and last three are preserved unchanged.
- `JagFileScriptWriter` produces two files at `<Output>/script.dat` and `<Output>/script.idx`. `script.dat` layout: `BE32(lastID+1)` + `BE32(jagFileVersion=27)` + concatenated script blobs by id ascending. `script.idx` layout: `BE32(lastID+1)` + `BE32(blobLen)` per id (or `BE32(0)` if missing).
- `Js5PackScriptWriter` produces one file at the configured `Output` path. Layout: `packedIndex` (gzip-wrapped via `packGroup(.,gzip)`) + concatenated `packedGroups` (each via `packGroup(.,none)`) + `BE32(packedGroup-len)` per group. `packGroup(.,gzip)` layout: `P1(type=2)` + `P4(compressedLen)` + `P4(origLen)` + gzip stream (with byte 9 of the gzip stream zeroed for `NAI-210-D-GZIP-OS-BYTE-ZEROED`).
- Script blob header layout (pinned by `pkg/pack/compiler/codegen/smoke_test.go::TestPipeline_FullSliceWithWriter` for `"smoke.rs2"`): `fullName + \x00` + `sourceName + \x00` + `BE32(lookupKey)` + `byte(0)` (debugproc-zero for non-debugproc triggers). With `helper.rs2` (10 chars) and fullName `"[proc,helper]"` (13 chars): bytes `[0:13] == "[proc,helper]"`, `[13] == 0`, `[14:24] == "helper.rs2"`, `[24] == 0`, `[25:29]` = BE32 lookupKey, `[29] == 0`.
- `ServerTriggerProc.Pointers == NewPointerSet(All...)` (`pkg/pack/compiler/trigger/server_trigger_type.go:23`), so a `return` call with `Required: active_player` from the seeded command map is satisfied.

---

### Task 1: Replace `TestCompile_JagWriter_EndToEnd` with `TestCompile_JagWriter_PinsScriptHeader`

**Files:**
- Modify: `pkg/pack/compiler/runescript/compile_test.go` (the `TestCompile_JagWriter_EndToEnd` function, currently lines 33–63)

- [ ] **Step 1: Replace the test function**

In `pkg/pack/compiler/runescript/compile_test.go`, locate `TestCompile_JagWriter_EndToEnd` (currently the second test in the file, starting after `TestCompile_MissingCoreSymbols_ReturnsError`). Replace the entire function (including its doc-comment) with:

```go
// TestCompile_JagWriter_PinsScriptHeader runs the full Compile() pipeline
// against a single-proc fixture, then asserts deterministic header bytes
// in the produced script.dat + script.idx files. Replaces the prior
// header-only smoke; the [proc,helper] body exercises codegen
// (PushLocalVar + PushConstantInt + Multiply + Return) so writePhase
// emits a non-empty blob.
//
// Closes NAI-210 follow-up #1 (RICHER-DRIVER-SMOKE) for the Jag sink.
func TestCompile_JagWriter_PinsScriptHeader(t *testing.T) {
	tmp := t.TempDir()
	scriptsDir := filepath.Join(tmp, "scripts")
	if err := os.MkdirAll(scriptsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(scriptsDir, "helper.rs2"),
		[]byte("[proc,helper](int $n)(int)\nreturn(calc($n * 2));\n"), 0o644); err != nil {
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

	dat, err := os.ReadFile(filepath.Join(packDir, "script.dat"))
	if err != nil {
		t.Fatalf("read script.dat: %v", err)
	}
	idx, err := os.ReadFile(filepath.Join(packDir, "script.idx"))
	if err != nil {
		t.Fatalf("read script.idx: %v", err)
	}

	// script.dat: BE32(lastID+1) + BE32(jagFileVersion=27) + helper blob.
	if got := binary.BigEndian.Uint32(dat[0:4]); got != 1 {
		t.Errorf("script.dat[0:4] lastID+1 = %d, want 1", got)
	}
	if got := binary.BigEndian.Uint32(dat[4:8]); got != 27 {
		t.Errorf("script.dat[4:8] jagFileVersion = %d, want 27", got)
	}
	// Helper blob starts at offset 8.
	if got := string(dat[8:21]); got != "[proc,helper]" {
		t.Errorf("script.dat[8:21] fullName = %q, want %q", got, "[proc,helper]")
	}
	if dat[21] != 0 {
		t.Errorf("script.dat[21] fullName NUL = %#x, want 0", dat[21])
	}
	if got := string(dat[22:32]); got != "helper.rs2" {
		t.Errorf("script.dat[22:32] sourceName = %q, want %q", got, "helper.rs2")
	}
	if dat[32] != 0 {
		t.Errorf("script.dat[32] sourceName NUL = %#x, want 0", dat[32])
	}
	if got := int32(binary.BigEndian.Uint32(dat[33:37])); got != -1 {
		t.Errorf("script.dat[33:37] lookupKey = %d, want -1 (ModeName)", got)
	}
	if dat[37] != 0 {
		t.Errorf("script.dat[37] debugproc-zero = %#x, want 0", dat[37])
	}
	if len(dat) <= 50 {
		t.Errorf("script.dat len = %d, want > 50 (non-empty opcode stream)", len(dat))
	}

	// script.idx: BE32(lastID+1) + BE32(blobLen).
	if got := binary.BigEndian.Uint32(idx[0:4]); got != 1 {
		t.Errorf("script.idx[0:4] lastID+1 = %d, want 1", got)
	}
	if got := binary.BigEndian.Uint32(idx[4:8]); got <= 30 {
		t.Errorf("script.idx[4:8] blobLen = %d, want > 30", got)
	}
}
```

- [ ] **Step 2: Verify `encoding/binary` import is present**

Run:

```bash
grep -n 'encoding/binary' pkg/pack/compiler/runescript/compile_test.go
```

If the import is absent, add it to the import block (alphabetical order; goes after `"encoding"` if present, or as a fresh `"encoding/binary"` entry between any earlier-letter and later-letter imports).

Current imports (HEAD `2dfa5db`) are `"os"`, `"path/filepath"`, `"testing"`, plus internal `semantics` import. Add `"encoding/binary"` as the first import in the stdlib group.

- [ ] **Step 3: Run the test, expect PASS**

Run:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/runescript/ -run TestCompile_JagWriter_PinsScriptHeader -v
```

Expected: PASS. The fixture should reach writePhase with one script (`[proc,helper]`), JagFileScriptWriter emits `script.dat` + `script.idx`, and the header byte pins match.

If FAIL: diagnose. Most likely causes:
- Pointer-checker error (would indicate proc trigger pointer assumption wrong — re-check `pkg/pack/compiler/trigger/server_trigger_type.go:23`).
- Codegen error (calc/return semantics — re-check `pkg/pack/compiler/codegen/smoke_test.go::TestPipeline_FullSlice` which uses identical body shape).
- Offset off-by-one (re-check against `pkg/pack/compiler/runescript/binary_writer_test.go:65`).

Do NOT modify production code to make this pass. If the test fails for a production reason, stop and report — that would indicate a NAI-210 / NAI-211 regression, not test-fixture drift.

- [ ] **Step 4: Verify no other tests regressed**

Run:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/runescript/ -v
```

Expected: All tests in the package pass. The other 4 tests in `compile_test.go` (MissingCoreSymbols, Js5WriterEndToEnd, HandlerInjection, NilHandlerDefaults) are unchanged and must still pass. Other test files (binary_writer_test.go, jag_file_writer_test.go, etc.) are untouched.

- [ ] **Step 5: Commit**

```bash
git add pkg/pack/compiler/runescript/compile_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
test(compiler/runescript): NAI-210-FU T1 — Jag driver smoke pins header

Replace TestCompile_JagWriter_EndToEnd with TestCompile_JagWriter_
PinsScriptHeader: drive a single-proc fixture through Compile() and pin
script.dat header bytes (fullName, sourceName, lookupKey, debugproc-
zero) plus script.idx blob length.
EOF
)"
```

---

### Task 2: Replace `TestCompile_Js5Writer_EndToEnd` with `TestCompile_Js5Writer_PinsScriptHeader`

**Files:**
- Modify: `pkg/pack/compiler/runescript/compile_test.go` (the `TestCompile_Js5Writer_EndToEnd` function)

- [ ] **Step 1: Replace the test function**

In `pkg/pack/compiler/runescript/compile_test.go`, locate `TestCompile_Js5Writer_EndToEnd`. Replace the entire function (including its doc-comment) with:

```go
// TestCompile_Js5Writer_PinsScriptHeader runs the full Compile() pipeline
// against a single-proc fixture, then asserts deterministic envelope bytes
// in the produced .js5 file. Replaces the prior header-only smoke. The
// gzip OS byte pin re-asserts NAI-210-D-GZIP-OS-BYTE-ZEROED at the driver
// level (the per-writer pin lives in js5_pack_writer_test.go).
//
// Closes NAI-210 follow-up #1 (RICHER-DRIVER-SMOKE) for the Js5 sink.
func TestCompile_Js5Writer_PinsScriptHeader(t *testing.T) {
	tmp := t.TempDir()
	scriptsDir := filepath.Join(tmp, "scripts")
	if err := os.MkdirAll(scriptsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(scriptsDir, "helper.rs2"),
		[]byte("[proc,helper](int $n)(int)\nreturn(calc($n * 2));\n"), 0o644); err != nil {
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

	bytes, err := os.ReadFile(js5Out)
	if err != nil {
		t.Fatalf("read %s: %v", js5Out, err)
	}

	// packedIndex envelope: P1(type=2 gzip) + P4(compLen) + P4(origLen) +
	// gzip stream. Gzip stream starts at file offset 9.
	if bytes[0] != 0x02 {
		t.Errorf("bytes[0] compressionType = %#x, want 0x02 (gzip)", bytes[0])
	}
	if got := binary.BigEndian.Uint32(bytes[1:5]); got == 0 {
		t.Errorf("bytes[1:5] compressedLen = 0, want > 0")
	}
	if got := binary.BigEndian.Uint32(bytes[5:9]); got == 0 {
		t.Errorf("bytes[5:9] origLen = 0, want > 0")
	}
	if bytes[9] != 0x1f || bytes[10] != 0x8b {
		t.Errorf("bytes[9:11] gzip magic = %#x %#x, want 0x1f 0x8b", bytes[9], bytes[10])
	}
	// NAI-210-D-GZIP-OS-BYTE-ZEROED: byte 9 of the gzip stream (file
	// offset 9+9 = 18) must be zero.
	if bytes[18] != 0 {
		t.Errorf("bytes[18] gzip OS byte = %#x, want 0 (NAI-210-D-GZIP-OS-BYTE-ZEROED)", bytes[18])
	}
	if len(bytes) <= 50 {
		t.Errorf("js5 file len = %d, want > 50 (envelope + packedGroup + lengths)", len(bytes))
	}
}
```

Note: the local variable name `bytes` shadows the `bytes` package, which is fine because `compile_test.go` does not import the `bytes` package.

- [ ] **Step 2: Run the test, expect PASS**

Run:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/runescript/ -run TestCompile_Js5Writer_PinsScriptHeader -v
```

Expected: PASS. The fixture reaches writePhase, Js5PackScriptWriter emits the .js5 file, and the envelope byte pins match.

If FAIL: diagnose. Most likely:
- Js5 file path / parent dir creation issue (verify `filepath.Dir(js5Out)` is created — `Js5PackScriptWriter.NewJs5PackScriptWriter` `MkdirAll`s the parent, so this should not fail).
- compressionType byte not 0x02 — re-check `pkg/pack/compiler/runescript/js5_pack_writer.go`'s `packGroup` for the index. If it changed to `none`, the spec is wrong.
- Gzip magic offset wrong — verify ByteWriter emits P1 at +1, P4 at +4 each.

Do NOT modify production code. Stop and report if the test fails for a production reason.

- [ ] **Step 3: Run full package test suite**

Run:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/runescript/ -v
```

Expected: All tests pass. Confirm the 3 preserved tests (`TestCompile_MissingCoreSymbols_ReturnsError`, `TestCompile_HandlerInjectionUsedDuringRun`, `TestCompile_NilHandlerDefaultsToBase`) still pass alongside the two new pin tests.

- [ ] **Step 4: Run with race detector**

Run:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./pkg/pack/compiler/runescript/
```

Expected: PASS. No concurrency added in this change; the race run is a regression-net check.

- [ ] **Step 5: Commit**

```bash
git add pkg/pack/compiler/runescript/compile_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
test(compiler/runescript): NAI-210-FU T2 — Js5 driver smoke pins envelope

Replace TestCompile_Js5Writer_EndToEnd with TestCompile_Js5Writer_
PinsScriptHeader: drive the same single-proc fixture through Compile()
with the Js5 sink and pin envelope bytes (compressionType, lengths,
gzip magic). Byte 18 of the file (offset 9 of the gzip stream) is
pinned to zero, re-asserting NAI-210-D-GZIP-OS-BYTE-ZEROED at the
driver level.
EOF
)"
```

---

### Task 3: Full-suite verification + close

**Files:** none modified.

- [ ] **Step 1: Run the full repo test suite**

Run:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
```

Expected: all packages pass. The richer driver smoke is test-only and should not perturb any other package.

If FAIL: investigate. Pre-existing working-tree mods (Dockerfile, Makefile, build.sh, config.yaml, go.mod) at HEAD are NOT part of this change — failures there are pre-existing. If a Go package test fails that isn't `pkg/pack/compiler/runescript`, verify against `git stash && go test ./<failed-package>/...` to confirm pre-existing.

- [ ] **Step 2: Verify the two replaced tests are gone**

Run:

```bash
grep -n 'TestCompile_JagWriter_EndToEnd\|TestCompile_Js5Writer_EndToEnd' pkg/pack/compiler/runescript/compile_test.go
```

Expected: no output. Both old names are gone.

- [ ] **Step 3: Verify the two new tests are present**

Run:

```bash
grep -n 'TestCompile_JagWriter_PinsScriptHeader\|TestCompile_Js5Writer_PinsScriptHeader' pkg/pack/compiler/runescript/compile_test.go
```

Expected: exactly two lines, one per new test.

- [ ] **Step 4: Verify the three preserved tests are still present**

Run:

```bash
grep -n 'TestCompile_MissingCoreSymbols_ReturnsError\|TestCompile_HandlerInjectionUsedDuringRun\|TestCompile_NilHandlerDefaultsToBase' pkg/pack/compiler/runescript/compile_test.go
```

Expected: exactly three lines.

- [ ] **Step 5: Write close commit**

Run:

```bash
git commit --no-gpg-sign --allow-empty -m "$(cat <<'EOF'
close(compiler/runescript): NAI-210-FU richer driver smoke

Closes NAI-210 follow-up #1 (RICHER-DRIVER-SMOKE). Driver smoke now
pins header bytes from both Jag and Js5 sinks via a real single-proc
fixture that exercises parse → analyze → codegen → checkPointers →
write end-to-end.

Re-pins NAI-210-D-GZIP-OS-BYTE-ZEROED at the driver level (writer-unit
pin remains in js5_pack_writer_test.go).

Tests:
- TestCompile_JagWriter_PinsScriptHeader (T1)
- TestCompile_Js5Writer_PinsScriptHeader (T2)

NAI-210 open follow-ups remaining: none from #1; NAI-211-FU-CODEGEN-
ERROR-DISPATCH-PIN remains as low-priority defensive pin (separate).

Closes memory: nai210_close (follow-up #1).
EOF
)"
```

---

## Self-Review

**Spec coverage:**

| Spec section | Task |
|---|---|
| §4 Fixture (single proc, [proc,helper]) | T1 Step 1 + T2 Step 1 |
| §5.1 Jag pin (script.dat + script.idx) | T1 Step 1 (all assertions) |
| §5.2 Js5 pin (envelope + gzip OS byte) | T2 Step 1 (all assertions) |
| §6 Preserved tests | T3 Steps 2–4 (verification) |
| §7 Files touched (compile_test.go only) | T1 + T2 |
| §8 Risks (none) | Confirmed via T1 Step 3 + T2 Step 2 diagnostic guidance |
| §9 Test strategy (TDD, run, race) | T1 Step 3, T2 Steps 2 + 4 |
| §10 Deviations (none) | No tag work in any task |

All sections covered.

**Placeholder scan:** No TODOs, no "add appropriate X", no "similar to Task N", no "TBD". Every code step contains the full code.

**Type consistency:** `Config`, `CompilerTypeInfo`, `WriterConfig`, `JagWriterConfig`, `Js5WriterConfig`, `semantics.StrictFeatureLevel{}` all match the API at HEAD `2dfa5db` (verified by reading `pkg/pack/compiler/runescript/compile.go`). Test function names: T1 uses `TestCompile_JagWriter_PinsScriptHeader` consistently; T2 uses `TestCompile_Js5Writer_PinsScriptHeader` consistently. Byte offset numbers `[8:21]`, `[14:24]`, `[33:37]` are arithmetic-checked against `len("[proc,helper]")==13` and `len("helper.rs2")==10`.

**Byte offset math check:**
- script.dat: `BE32(lastID+1)` [0:4] + `BE32(version)` [4:8] = 8 bytes header. Blob starts at 8.
- Blob `[0:13]` = fullName → script.dat `[8:21]`. ✓
- Blob `[13]` = NUL → script.dat `[21]`. ✓
- Blob `[14:24]` = "helper.rs2" → script.dat `[22:32]`. ✓
- Blob `[24]` = NUL → script.dat `[32]`. ✓
- Blob `[25:29]` = lookupKey → script.dat `[33:37]`. ✓
- Blob `[29]` = debugproc-zero → script.dat `[37]`. ✓
- Js5: P1 + P4 + P4 = 9 bytes envelope. Gzip stream `[0:2]` = magic → file `[9:11]`. ✓
- Gzip stream `[9]` = OS byte → file `[18]`. ✓

All offsets check out.
