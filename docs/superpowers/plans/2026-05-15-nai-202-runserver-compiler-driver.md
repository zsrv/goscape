# NAI-202: `runServerCompiler` driver port — implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port TS `runServerCompiler` (Compiler.ts:109-329) as `pkg/pack/compiler.BuildSymbols(srcDir, dataPackDir) (map[string]*TypeInfo, error)`, stopping short of the deferred `CompileServerScript` call. Close two NAI-201 carryforwards (PointerGroupFind hardening; reverse-coverage test) and fix two TS-source bugs with deviation tags.

**Architecture:** Single public function `BuildSymbols` orchestrates 22 `.pack` loads + 7 cache-config loader enrichments + 3 LoadMap + 2 LoadArray + 1 commandInfo build + 1 constants load + final 32-key map assembly. An internal `configLoaders` seam lets enrichment helpers be tested with in-memory `*objtype.FooTypeConfigs` instead of disk binary fixtures.

**Tech Stack:** Go 1.26+, stdlib only. Consumes `pkg/script` (`ScriptOpcodeMap`, `ScriptOpcodePointers`, `Opcode`), `pkg/objtype` (entity-type loaders + stat maps), `pkg/pack` (`LoadDirExt`), `pkg/pack/compiler` (NAI-200 `TypeInfo`, `Load`, `LoadMap`, `LoadArray`, `LoadRecords`).

---

## File structure

| File | Role | Status |
|---|---|---|
| `pkg/script/opcode_pointers.go` | Unexport `pointerGroupFind` array; add accessor | MODIFY |
| `pkg/script/opcode_pointers_test.go` | Adjust references through accessor | MODIFY |
| `pkg/script/opcode_map_test.go` | Add reverse-coverage test | MODIFY |
| `pkg/pack/compiler/symbols.go` | `BuildSymbols` + helpers (constants parser, commandInfo build, interface/overlay synth, dbcolumn synth, 5 enrichers, configLoaders seam) | NEW |
| `pkg/pack/compiler/symbols_test.go` | Unit tests for each helper + integration | NEW |
| `pkg/pack/compiler/nai202_deviation_pins_test.go` | Deviation-tag grep pin | NEW |

Tasks T1–T2 land the two NAI-201 carryforwards first (small, low-coupling) so subsequent tasks can build the driver on a clean foundation. T3–T9 build the driver bottom-up. T10 lands the deviation-tag grep pin once all four tags appear in source. T11 is the close commit.

---

## Task 1: Harden `PointerGroupFind`

**Files:**
- Modify: `pkg/script/opcode_pointers.go:1-12,39-60`
- Modify: `pkg/script/opcode_pointers_test.go` (existing references to `PointerGroupFind`)

- [ ] **Step 1: Read existing files to identify references**

Run: `grep -n "PointerGroupFind" pkg/script/*.go`

Expected output lists references in `opcode_pointers.go` (declaration + corruptExceptActive) and `opcode_pointers_test.go` (one or more test references). Note line numbers — they're inputs for step 3.

- [ ] **Step 2: Write the failing test (`pkg/script/opcode_pointers_test.go`, append)**

Append to the end of `pkg/script/opcode_pointers_test.go`:

```go
// TestPointerGroupFind_AccessorReturnsFreshCopy pins
// NAI-202-D-POINTER-GROUP-FIND-HARDENED: the public PointerGroupFind()
// accessor must return a fresh slice on each call so callers cannot
// mutate package-internal state.
func TestPointerGroupFind_AccessorReturnsFreshCopy(t *testing.T) {
	first := PointerGroupFind()
	want := []string{"find_player", "find_npc", "find_loc", "find_obj", "find_db"}

	if len(first) != len(want) {
		t.Fatalf("len(PointerGroupFind()) = %d, want %d", len(first), len(want))
	}
	for i, name := range want {
		if first[i] != name {
			t.Errorf("PointerGroupFind()[%d] = %q, want %q", i, first[i], name)
		}
	}

	// Mutate the returned slice — must not affect subsequent calls.
	first[0] = "MUTATED"
	second := PointerGroupFind()
	if second[0] != "find_player" {
		t.Errorf("after caller mutation of returned slice, second call returned %q at [0]; want %q (accessor must return fresh copies)", second[0], "find_player")
	}
}
```

- [ ] **Step 3: Run the test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... -run TestPointerGroupFind_AccessorReturnsFreshCopy -v`

Expected: FAIL with compile error along the lines of `cannot call non-function PointerGroupFind (variable of type []string)` — the symbol is currently a `var` of slice type, so `PointerGroupFind()` is a type error.

- [ ] **Step 4: Replace the exported var with unexported array + accessor**

Edit `pkg/script/opcode_pointers.go` lines 1-12 (the `PointerGroupFind` var block) to:

```go
package script

// pointerGroupFind is the 5-element find_* pointer-name list (unexported
// to prevent caller mutation). Mirrors TS POINTER_GROUP_FIND
// (ScriptOpcodePointers.ts:3). External callers reach it through the
// PointerGroupFind() accessor which returns a fresh copy.
//
// NAI-202-D-POINTER-GROUP-FIND-HARDENED: NAI-201 originally shipped this
// as `var PointerGroupFind = []string{...}` (exported slice). NAI-202
// closes a NAI-201 final-review follow-up by hiding the storage and
// returning copies, defending against accidental mutation of package
// state by callers that grow into existence in NAI-203+.
//
// Order matters: corrupt-slice content is concatenated in this exact
// order on all 6 TS spread sites.
var pointerGroupFind = [5]string{
	"find_player", "find_npc", "find_loc", "find_obj", "find_db",
}

// PointerGroupFind returns a fresh slice copy of the find_* pointer-name
// list. Returning a copy ensures callers cannot mutate package-internal
// state — see NAI-202-D-POINTER-GROUP-FIND-HARDENED.
func PointerGroupFind() []string {
	out := make([]string, len(pointerGroupFind))
	copy(out, pointerGroupFind[:])
	return out
}
```

Edit `pkg/script/opcode_pointers.go` line 55-60 (`corruptExceptActive` body) to use the unexported array:

```go
func corruptExceptActive(extras ...string) []string {
	out := make([]string, 0, len(pointerGroupFind)+len(extras))
	out = append(out, pointerGroupFind[:]...)
	out = append(out, extras...)
	return out
}
```

- [ ] **Step 5: Adjust existing test references**

For every match from step 1 in `pkg/script/opcode_pointers_test.go` that references the exported `PointerGroupFind` as a value (not a function call), update:

- If the test reads `PointerGroupFind` as a slice → change to `PointerGroupFind()`.
- If the test reads `len(PointerGroupFind)` → change to `len(PointerGroupFind())` or `len(pointerGroupFind)` (the unexported array is reachable within the same package).
- Direct indexing `PointerGroupFind[i]` → `PointerGroupFind()[i]` or `pointerGroupFind[i]`.

The NAI-201 helper-count pin test (`TestScriptOpcodePointers_CorruptExceptActiveCallSites`) should continue to pass without edits — it tests the helper's output, not the storage shape.

- [ ] **Step 6: Run the test to verify it passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... -v`

Expected: PASS. All NAI-201 pointers/opcode_map tests still green; new `TestPointerGroupFind_AccessorReturnsFreshCopy` passes.

- [ ] **Step 7: Commit**

```bash
git add pkg/script/opcode_pointers.go pkg/script/opcode_pointers_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-202 T1 — harden PointerGroupFind storage

Unexports the 5-element find_* pointer-name list to a [5]string array
and exposes a PointerGroupFind() accessor returning a fresh slice copy.
Closes NAI-201 final-review follow-up; defends against caller mutation
of package state. corruptExceptActive helper updated to slice the
unexported array directly.

Deviation: NAI-202-D-POINTER-GROUP-FIND-HARDENED.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: Add `TestScriptOpcodeMap_ReverseCoverage`

**Files:**
- Modify: `pkg/script/opcode_map_test.go` (append)

- [ ] **Step 1: Pre-flight grep — count Op* constants**

Run: `grep -c "^\s*Op[A-Z][A-Za-z]\+\s*Opcode\s*=\s*[0-9]" pkg/script/opcode.go`

Expected: ~398. Note the exact number — `wantNamedOpcodes` test arithmetic depends on it.

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... -run TestScriptOpcodeMap_LengthParity -v`

Expected: PASS (anchors `len(ScriptOpcodeMap) == 393`).

- [ ] **Step 2: Write the failing test (append to `pkg/script/opcode_map_test.go`)**

```go
// excludedOpcodes lists Op* constants intentionally absent from
// ScriptOpcodeMap (e.g., internal-only opcodes the script source can
// never reference by name). Empty at NAI-202 land. Entries get added
// only with a justifying comment.
var excludedOpcodes = map[Opcode]string{}

// TestScriptOpcodeMap_ReverseCoverage pins: every Op* constant declared
// in pkg/script/opcode.go either appears as a value in ScriptOpcodeMap
// or is explicitly listed in excludedOpcodes with rationale. Catches
// the failure mode where a new Op* constant is added (e.g., during
// NAI-203+ work) without the corresponding ScriptOpcodeMap entry.
//
// Detection strategy: walks the closed range [0, OpTimeSpent] and uses
// Opcode(i).String() — named opcodes return UPPER_SNAKE_CASE; unnamed
// values return "opcode_N". A named opcode missing from both
// ScriptOpcodeMap (values) and excludedOpcodes is a coverage gap.
func TestScriptOpcodeMap_ReverseCoverage(t *testing.T) {
	// Build the set of opcodes present in ScriptOpcodeMap as values.
	mapped := make(map[Opcode]struct{}, len(ScriptOpcodeMap))
	for _, op := range ScriptOpcodeMap {
		mapped[op] = struct{}{}
	}

	missing := []Opcode{}
	for i := 0; i <= int(OpTimeSpent); i++ {
		op := Opcode(i)
		name := op.String()
		if strings.HasPrefix(name, "opcode_") {
			continue // unnamed slot in the sparse enum
		}
		if _, ok := mapped[op]; ok {
			continue
		}
		if _, excluded := excludedOpcodes[op]; excluded {
			continue
		}
		missing = append(missing, op)
	}

	if len(missing) > 0 {
		// Format the missing list for the failure message.
		lines := make([]string, 0, len(missing))
		for _, op := range missing {
			lines = append(lines, fmt.Sprintf("\t%s (Opcode=%d)", op.String(), uint16(op)))
		}
		t.Fatalf("ReverseCoverage: %d named Op* constants are absent from ScriptOpcodeMap AND not listed in excludedOpcodes:\n%s\n\nFix: either add the entry to ScriptOpcodeMap (preferred — opcodes are reachable from script source) or add an excludedOpcodes entry with a justifying comment.", len(missing), strings.Join(lines, "\n"))
	}
}
```

- [ ] **Step 3: Verify imports**

`pkg/script/opcode_map_test.go` already imports `testing`. The new test needs `fmt` and `strings`. Confirm by running:

Run: `head -15 pkg/script/opcode_map_test.go`

If `fmt` or `strings` are missing, add them to the existing import block in the file.

- [ ] **Step 4: Run the test**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... -run TestScriptOpcodeMap_ReverseCoverage -v`

Expected: One of two outcomes:
- **PASS**: every named `Op*` constant is in `ScriptOpcodeMap`. `excludedOpcodes` stays empty. Proceed to step 6.
- **FAIL**: the failure message lists named `Op*` constants absent from `ScriptOpcodeMap`. For each such constant, the implementer reviews:
  - Is the constant actually unused by script source (e.g., `OpDefault`, `OpInvalid`, internal stack-machine markers)? → Add to `excludedOpcodes` with rationale: `OpFoo: "internal-only — never reachable from script source"`.
  - Is it a real gap? → Halt and report; this is a NAI-201 omission that needs investigation (the spec assumed gap = 0 per §9 R6). Do not silently bandage.

- [ ] **Step 5: If FAIL — populate `excludedOpcodes` and re-run**

Update `excludedOpcodes` with one entry per justified omission. Re-run step 4 until PASS.

- [ ] **Step 6: Commit**

```bash
git add pkg/script/opcode_map_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
test(script): NAI-202 T2 — ScriptOpcodeMap reverse-coverage gate

Closes NAI-201 final-review follow-up: every Op* constant in opcode.go
either appears as a value in ScriptOpcodeMap or sits on an explicit
excludedOpcodes allowlist with rationale. Catches the failure mode
where a new Op* constant lands (e.g., NAI-203+ compiler-arc work)
without the corresponding name→Opcode mapping.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: `loadCompilerConstants` parser

**Files:**
- Create: `pkg/pack/compiler/symbols.go`
- Create: `pkg/pack/compiler/symbols_test.go`

- [ ] **Step 1: Write failing tests (`pkg/pack/compiler/symbols_test.go`)**

Create `pkg/pack/compiler/symbols_test.go` with these eight test functions:

```go
package compiler

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLoadCompilerConstants_StripsLeadingCaret pins TS Compiler.ts:162-164:
// names beginning with '^' have the '^' stripped before storage.
func TestLoadCompilerConstants_StripsLeadingCaret(t *testing.T) {
	dir := t.TempDir()
	writeConstantFile(t, dir, "a.constant", "^FOO=bar\n")

	m, err := loadCompilerConstants(filepath.Join(dir, "scripts"))
	if err != nil {
		t.Fatalf("loadCompilerConstants: %v", err)
	}
	if got, want := m["FOO"], "bar"; got != want {
		t.Errorf("m[\"FOO\"] = %q, want %q", got, want)
	}
	if _, present := m["^FOO"]; present {
		t.Errorf("m has both ^FOO and FOO; caret should have been stripped")
	}
}

// TestLoadCompilerConstants_StripsSurroundingQuotes pins TS Compiler.ts:166-169.
func TestLoadCompilerConstants_StripsSurroundingQuotes(t *testing.T) {
	dir := t.TempDir()
	writeConstantFile(t, dir, "a.constant",
		"A=\"quoted\"\nB=unquoted\nC=\"mismatch\nD=mismatch\"\nE=\"in\"middle\"\n",
	)

	m, err := loadCompilerConstants(filepath.Join(dir, "scripts"))
	if err != nil {
		t.Fatalf("loadCompilerConstants: %v", err)
	}
	cases := map[string]string{
		"A": "quoted",      // both-sided quotes stripped
		"B": "unquoted",    // no quotes, unchanged
		"C": "\"mismatch",  // open-only, unchanged
		"D": "mismatch\"",  // close-only, unchanged
		"E": "in\"middle",  // input "in"middle" — outer pair stripped, inner quote retained
	}
	for k, want := range cases {
		if got := m[k]; got != want {
			t.Errorf("m[%q] = %q, want %q", k, got, want)
		}
	}
}

// TestLoadCompilerConstants_LastWriterWins pins NAI-202-D-CONSTANT-LOOSE-PARSER:
// duplicate names within the same file resolve last-line-wins (no error,
// unlike pkg/pack.LoadConstants which errors).
func TestLoadCompilerConstants_LastWriterWins(t *testing.T) {
	dir := t.TempDir()
	writeConstantFile(t, dir, "a.constant", "K=a\nK=b\n")

	m, err := loadCompilerConstants(filepath.Join(dir, "scripts"))
	if err != nil {
		t.Fatalf("loadCompilerConstants: %v (NAI-202-D-CONSTANT-LOOSE-PARSER: dup must not error)", err)
	}
	if got, want := m["K"], "b"; got != want {
		t.Errorf("m[\"K\"] = %q, want %q (last-line-wins per loose parser)", got, want)
	}
}

// TestLoadCompilerConstants_SkipsComments pins TS Compiler.ts:155:
// lines beginning with '//' are skipped.
func TestLoadCompilerConstants_SkipsComments(t *testing.T) {
	dir := t.TempDir()
	writeConstantFile(t, dir, "a.constant", "// K=a\nK=b\n")

	m, err := loadCompilerConstants(filepath.Join(dir, "scripts"))
	if err != nil {
		t.Fatalf("loadCompilerConstants: %v", err)
	}
	if got, want := m["K"], "b"; got != want {
		t.Errorf("m[\"K\"] = %q, want %q (// line skipped)", got, want)
	}
	if len(m) != 1 {
		t.Errorf("len(m) = %d, want 1; map = %v", len(m), m)
	}
}

// TestLoadCompilerConstants_DiscardsPastSecondEquals pins TS-faithful
// destructure of unbounded split: parts[0]=name, parts[1]=value, parts[2:]
// dropped. K=v=extra → m["K"] = "v" (not "v=extra").
func TestLoadCompilerConstants_DiscardsPastSecondEquals(t *testing.T) {
	dir := t.TempDir()
	writeConstantFile(t, dir, "a.constant", "K=v=extra\n")

	m, err := loadCompilerConstants(filepath.Join(dir, "scripts"))
	if err != nil {
		t.Fatalf("loadCompilerConstants: %v", err)
	}
	if got, want := m["K"], "v"; got != want {
		t.Errorf("m[\"K\"] = %q, want %q (parts past second '=' discarded)", got, want)
	}
}

// TestLoadCompilerConstants_ErrorsOnMissingEquals pins TS-faithful
// behaviour: a line with no '=' triggers an undefined-index throw in TS.
// Goscape returns a wrapped error including the file path.
func TestLoadCompilerConstants_ErrorsOnMissingEquals(t *testing.T) {
	dir := t.TempDir()
	writeConstantFile(t, dir, "a.constant", "broken_line_no_equals\n")

	_, err := loadCompilerConstants(filepath.Join(dir, "scripts"))
	if err == nil {
		t.Fatal("expected error on line missing '=', got nil")
	}
	if !strings.Contains(err.Error(), "a.constant") {
		t.Errorf("error %q must mention the offending file path", err.Error())
	}
}

// TestLoadCompilerConstants_TrimsWhitespace pins TS Compiler.ts:161,166
// which call .trim() on both name and value.
func TestLoadCompilerConstants_TrimsWhitespace(t *testing.T) {
	dir := t.TempDir()
	writeConstantFile(t, dir, "a.constant", "  K  =  v  \n")

	m, err := loadCompilerConstants(filepath.Join(dir, "scripts"))
	if err != nil {
		t.Fatalf("loadCompilerConstants: %v", err)
	}
	if got, want := m["K"], "v"; got != want {
		t.Errorf("m[\"K\"] = %q, want %q (whitespace trimmed)", got, want)
	}
}

// TestLoadCompilerConstants_EmptyScriptsDir pins: missing scripts dir
// returns an empty map with nil error.
func TestLoadCompilerConstants_EmptyScriptsDir(t *testing.T) {
	dir := t.TempDir()
	// No scripts/ subdir created.

	m, err := loadCompilerConstants(filepath.Join(dir, "scripts"))
	if err != nil {
		t.Fatalf("loadCompilerConstants: %v (missing dir must not error)", err)
	}
	if len(m) != 0 {
		t.Errorf("len(m) = %d, want 0", len(m))
	}
}

// writeConstantFile writes content to <dir>/scripts/<rel>, creating the
// scripts subdir + any intermediate dirs of rel.
func writeConstantFile(t *testing.T, dir, rel, content string) {
	t.Helper()
	full := filepath.Join(dir, "scripts", rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/... -run TestLoadCompilerConstants -v`

Expected: All 8 tests FAIL with `undefined: loadCompilerConstants` (compile error).

- [ ] **Step 3: Write minimal implementation (`pkg/pack/compiler/symbols.go`)**

Create `pkg/pack/compiler/symbols.go`:

```go
// Package compiler — extended in NAI-202 to host BuildSymbols and its
// supporting helpers. NAI-200 introduced TypeInfo + Load family.
// NAI-202 introduces the runServerCompiler driver port that consumes
// them.
package compiler

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// loadCompilerConstants walks scriptsDir recursively for *.constant
// files and returns a flat map of constant name → raw textual value.
// Mirrors TS Compiler.ts:152-173.
//
// NAI-202-D-CONSTANT-LOOSE-PARSER: this is goscape's second .constant
// parser. TS has two semantically different parsers — PackShared.ts:262
// is strict (dedup-error, no quote-strip, split-rest-after-first '=');
// Compiler.ts:152 is loose (last-writer-wins, surrounding-quote strip,
// drop-past-second '='). Goscape mirrors that two-parser shape rather
// than collapsing them.
//
// Per-line rules (TS-faithful, in order):
//   - empty line                         → skip
//   - line starts with "//"              → skip
//   - split on "=", take first segment as name, second as value;
//     anything past the second "=" is discarded (mirrors TS
//     `const [name, value] = line.split('=')`)
//   - trim name and value (whitespace)
//   - if name starts with "^", strip the leading "^"
//   - if value starts AND ends with `"`, strip both
//   - assign m[name] = value (last writer wins; no dedup error)
//
// A line with no "=" returns an error wrapping the file path and
// offending line — TS would throw on the parts[1] undefined access.
//
// Missing scriptsDir → empty map, nil error.
func loadCompilerConstants(scriptsDir string) (map[string]string, error) {
	m := map[string]string{}

	info, err := os.Stat(scriptsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return m, nil
		}
		return nil, fmt.Errorf("stat %s: %w", scriptsDir, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s: not a directory", scriptsDir)
	}

	err = filepath.WalkDir(scriptsDir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || !strings.HasSuffix(path, ".constant") {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return fmt.Errorf("read %s: %w", path, readErr)
		}
		// TS uses split(/\r?\n/); strings.Split("\n") + per-line \r-strip mirrors that.
		for lineNo, raw := range strings.Split(string(data), "\n") {
			line := strings.TrimSuffix(raw, "\r")
			if len(line) == 0 {
				continue
			}
			if strings.HasPrefix(line, "//") {
				continue
			}
			// TS unbounded split + destructure of [name, value] drops parts past second '='.
			// SplitN with n=3 captures parts[0], parts[1]; parts[2] (if present) is discarded.
			parts := strings.SplitN(line, "=", 3)
			if len(parts) < 2 {
				return fmt.Errorf("%s:%d: line missing '=': %q", path, lineNo+1, line)
			}
			name := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			if strings.HasPrefix(name, "^") {
				name = strings.TrimPrefix(name, "^")
			}
			if len(value) >= 2 && strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"") {
				value = value[1 : len(value)-1]
			}
			m[name] = value
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return m, nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/... -run TestLoadCompilerConstants -v`

Expected: All 8 tests PASS.

Sanity-check on case `E`: the input line `E="in"middle"` has `value = "in"middle"` (12 chars). TS `if value.startsWith('"') && value.endsWith('"')` → both true, so strip outer pair → `in"middle` (10 chars). The test's expected `"in\"middle"` matches.

- [ ] **Step 5: Commit**

```bash
git add pkg/pack/compiler/symbols.go pkg/pack/compiler/symbols_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(compiler): NAI-202 T3 — loadCompilerConstants TS-faithful parser

Inline .constant parser mirroring TS Compiler.ts:152-173. Distinct from
pkg/pack.LoadConstants (strict, used for value substitution in
PackShared.ts:262). Two parsers in goscape mirror the two-parser reality
on the TS side.

Deviation: NAI-202-D-CONSTANT-LOOSE-PARSER (last-writer-wins on dups;
surrounding-quote strip on values; drops parts past second '=').

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: `scriptVarTypeName` helper

**Files:**
- Modify: `pkg/pack/compiler/symbols.go` (append)
- Modify: `pkg/pack/compiler/symbols_test.go` (append)

- [ ] **Step 1: Write failing tests (append to `pkg/pack/compiler/symbols_test.go`)**

Add to the import block: `"github.com/zsrv/goscape/pkg/objtype"`.

Append:

```go
// TestScriptVarTypeName_KnownCodes pins the name returned for each
// ScriptVarType constant, mirroring TS ScriptVarType.getType
// (ScriptVarType.ts:85-170) and goscape's existing
// objtype.ParamType.GetType() (paramtype.go:105).
func TestScriptVarTypeName_KnownCodes(t *testing.T) {
	cases := []struct {
		t    objtype.ScriptVarType
		want string
	}{
		{objtype.ScriptVarTypeInt, "int"},
		{objtype.ScriptVarTypeString, "string"},
		{objtype.ScriptVarTypeEnum, "enum"},
		{objtype.ScriptVarTypeObj, "obj"},
		{objtype.ScriptVarTypeLoc, "loc"},
		{objtype.ScriptVarTypeComponent, "component"},
		{objtype.ScriptVarTypeNamedObj, "namedobj"},
		{objtype.ScriptVarTypeStruct, "struct"},
		{objtype.ScriptVarTypeBoolean, "boolean"},
		{objtype.ScriptVarTypeCoord, "coord"},
		{objtype.ScriptVarTypeCategory, "category"},
		{objtype.ScriptVarTypeSpotanim, "spotanim"},
		{objtype.ScriptVarTypeNPC, "npc"},
		{objtype.ScriptVarTypeInv, "inv"},
		{objtype.ScriptVarTypeSynth, "synth"},
		{objtype.ScriptVarTypeSeq, "seq"},
		{objtype.ScriptVarTypeStat, "stat"},
		{objtype.ScriptVarTypeInterface, "interface"},
	}
	for _, c := range cases {
		if got := scriptVarTypeName(c.t); got != c.want {
			t.Errorf("scriptVarTypeName(%d) = %q, want %q", c.t, got, c.want)
		}
	}
}

// TestScriptVarTypeName_UnknownCode pins the "unknown" return for type
// codes not in the switch (matches ParamType.GetType()).
func TestScriptVarTypeName_UnknownCode(t *testing.T) {
	if got := scriptVarTypeName(objtype.ScriptVarType(0)); got != "unknown" {
		t.Errorf("scriptVarTypeName(0) = %q, want \"unknown\"", got)
	}
	if got := scriptVarTypeName(objtype.ScriptVarType(99)); got != "unknown" {
		t.Errorf("scriptVarTypeName(99) = %q, want \"unknown\"", got)
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/... -run TestScriptVarTypeName -v`

Expected: FAIL with `undefined: scriptVarTypeName`.

- [ ] **Step 3: Append implementation to `pkg/pack/compiler/symbols.go`**

Add to import block: `"github.com/zsrv/goscape/pkg/objtype"`.

Append at end of file:

```go
// scriptVarTypeName returns the TS-style name for a ScriptVarType code.
// Mirrors TS ScriptVarType.getType (ScriptVarType.ts:85-170). Unexported;
// used internally by the varp/varn/vars enrichment passes (NAI-202).
//
// Why not method on each VarPlayerType/VarNpcType/VarSharedType:
// goscape's existing objtype.ParamType.GetType() (paramtype.go:105) is
// the only entity that has this method. Adding three more parallel
// methods scatters identical switch statements across four files.
// Centralizing here is the cheaper maintenance posture.
func scriptVarTypeName(t objtype.ScriptVarType) string {
	switch t {
	case objtype.ScriptVarTypeInt:
		return "int"
	case objtype.ScriptVarTypeString:
		return "string"
	case objtype.ScriptVarTypeEnum:
		return "enum"
	case objtype.ScriptVarTypeObj:
		return "obj"
	case objtype.ScriptVarTypeLoc:
		return "loc"
	case objtype.ScriptVarTypeComponent:
		return "component"
	case objtype.ScriptVarTypeNamedObj:
		return "namedobj"
	case objtype.ScriptVarTypeStruct:
		return "struct"
	case objtype.ScriptVarTypeBoolean:
		return "boolean"
	case objtype.ScriptVarTypeCoord:
		return "coord"
	case objtype.ScriptVarTypeCategory:
		return "category"
	case objtype.ScriptVarTypeSpotanim:
		return "spotanim"
	case objtype.ScriptVarTypeNPC:
		return "npc"
	case objtype.ScriptVarTypeInv:
		return "inv"
	case objtype.ScriptVarTypeSynth:
		return "synth"
	case objtype.ScriptVarTypeSeq:
		return "seq"
	case objtype.ScriptVarTypeStat:
		return "stat"
	case objtype.ScriptVarTypeInterface:
		return "interface"
	default:
		return "unknown"
	}
}
```

- [ ] **Step 4: Run to verify pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/... -run TestScriptVarTypeName -v`

Expected: PASS (both tests).

- [ ] **Step 5: Commit**

```bash
git add pkg/pack/compiler/symbols.go pkg/pack/compiler/symbols_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(compiler): NAI-202 T4 — scriptVarTypeName helper

Internal helper returning the TS-style name for a ScriptVarType code.
Centralizes the switch consumed by the varp/varn/vars enrichment passes
that NAI-202 lands next. Mirrors objtype.ParamType.GetType() to keep
all four entity-type vartype lookups uniform.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: `populateCommandInfo` + CORRUPT2-FIELD pin

**Files:**
- Modify: `pkg/pack/compiler/symbols.go` (append)
- Modify: `pkg/pack/compiler/symbols_test.go` (append)

- [ ] **Step 1: Write failing tests (append to `symbols_test.go`)**

Add to import block: `"github.com/zsrv/goscape/pkg/script"`, `"sort"`.

Append:

```go
// TestPopulateCommandInfoFrom_AscendingIteration pins TS Compiler.ts:111
// — iteration is sorted ascending by opcode value. Tests via the seam
// populateCommandInfoFrom which accepts the opmap/pointers as args.
func TestPopulateCommandInfoFrom_AscendingIteration(t *testing.T) {
	opmap := map[string]script.Opcode{
		"ZETA": script.Opcode(100),
		"ALPHA": script.Opcode(1),
		"BETA":  script.Opcode(10),
	}
	pointers := map[script.Opcode]script.Pointers{}

	info := newTypeInfo()
	populateCommandInfoFrom(info, opmap, pointers)

	// Max should be 101 (highest opcode = 100, Max = id+1).
	if info.Max != 101 {
		t.Errorf("Max = %d, want 101 (highest opcode 100 → Max = 100+1)", info.Max)
	}
	// Names are lowercased.
	want := map[int]string{1: "alpha", 10: "beta", 100: "zeta"}
	for id, name := range want {
		if got := info.Map[id]; got != name {
			t.Errorf("Map[%d] = %q, want %q", id, got, name)
		}
	}
}

// TestPopulateCommandInfoFrom_RequireSetCorrupt pins TS Compiler.ts:123-149
// — for an opcode with Require/Set/Corrupt fields, the corresponding
// commandInfo maps are populated with comma-joined strings.
func TestPopulateCommandInfoFrom_RequireSetCorrupt(t *testing.T) {
	op := script.Opcode(42)
	opmap := map[string]script.Opcode{"FOO": op}
	pointers := map[script.Opcode]script.Pointers{
		op: {
			Require: []string{"active_player", "find_loc"},
			Set:     []string{"active_npc"},
			Corrupt: []string{"find_npc", "find_loc"},
		},
	}

	info := newTypeInfo()
	populateCommandInfoFrom(info, opmap, pointers)

	if got, want := info.Require[42], "active_player,find_loc"; got != want {
		t.Errorf("Require[42] = %q, want %q", got, want)
	}
	if got, want := info.Set[42], "active_npc"; got != want {
		t.Errorf("Set[42] = %q, want %q", got, want)
	}
	if got, want := info.Corrupt[42], "find_npc,find_loc"; got != want {
		t.Errorf("Corrupt[42] = %q, want %q", got, want)
	}
}

// TestPopulateCommandInfoFrom_Conditional pins TS Compiler.ts:132-134
// — Conditional is set in commandInfo.Conditional[op] only when both
// Set is non-empty AND ptrs.Conditional is true.
func TestPopulateCommandInfoFrom_Conditional(t *testing.T) {
	cases := []struct {
		name        string
		set         []string
		conditional bool
		wantPresent bool
		wantValue   bool
	}{
		{"set + conditional", []string{"x"}, true, true, true},
		{"set, not conditional", []string{"x"}, false, false, false},
		{"conditional, no set", nil, true, false, false}, // TS only writes inside `if pointers.set`
		{"neither", nil, false, false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			op := script.Opcode(1)
			opmap := map[string]script.Opcode{"OP": op}
			pointers := map[script.Opcode]script.Pointers{
				op: {Set: c.set, Conditional: c.conditional},
			}
			info := newTypeInfo()
			populateCommandInfoFrom(info, opmap, pointers)

			got, present := info.Conditional[1]
			if present != c.wantPresent {
				t.Errorf("Conditional[1] present=%v, want %v", present, c.wantPresent)
			}
			if got != c.wantValue {
				t.Errorf("Conditional[1] = %v, want %v", got, c.wantValue)
			}
		})
	}
}

// TestPopulateCommandInfoFrom_Corrupt2FieldFix pins NAI-202-D-CORRUPT2-FIELD:
// TS Compiler.ts:146-147 had a typo (corrupt2 arm assigned to .corrupt,
// overwriting). Goscape fixes by assigning corrupt2 to .Corrupt2.
func TestPopulateCommandInfoFrom_Corrupt2FieldFix(t *testing.T) {
	op := script.Opcode(7)
	opmap := map[string]script.Opcode{"OP": op}
	pointers := map[script.Opcode]script.Pointers{
		op: {
			Corrupt:  []string{"corrupt_a", "corrupt_b"},
			Corrupt2: []string{"corrupt2_x", "corrupt2_y"},
		},
	}

	info := newTypeInfo()
	populateCommandInfoFrom(info, opmap, pointers)

	if got, want := info.Corrupt[7], "corrupt_a,corrupt_b"; got != want {
		t.Errorf("Corrupt[7] = %q, want %q (must NOT be overwritten by corrupt2)", got, want)
	}
	if got, want := info.Corrupt2[7], "corrupt2_x,corrupt2_y"; got != want {
		t.Errorf("Corrupt2[7] = %q, want %q (NAI-202-D-CORRUPT2-FIELD: corrected destination)", got, want)
	}
}

// TestPopulateCommandInfoFrom_Require2Set2 covers the require2 + set2
// arms which are not bugs in TS but are still part of the enrichment.
func TestPopulateCommandInfoFrom_Require2Set2(t *testing.T) {
	op := script.Opcode(3)
	opmap := map[string]script.Opcode{"OP": op}
	pointers := map[script.Opcode]script.Pointers{
		op: {
			Require:  []string{"active_player"},
			Require2: []string{"active_player2"},
			Set:      []string{"active_npc"},
			Set2:     []string{"active_npc2"},
		},
	}

	info := newTypeInfo()
	populateCommandInfoFrom(info, opmap, pointers)

	if got, want := info.Require2[3], "active_player2"; got != want {
		t.Errorf("Require2[3] = %q, want %q", got, want)
	}
	if got, want := info.Set2[3], "active_npc2"; got != want {
		t.Errorf("Set2[3] = %q, want %q", got, want)
	}
}

// TestPopulateCommandInfo_RealData smoke-tests populateCommandInfo
// against the real ScriptOpcodeMap/ScriptOpcodePointers — the same
// data BuildSymbols feeds. Asserts the 393-entry parity and one or two
// spot-checks against NAI-201's known opcodes.
func TestPopulateCommandInfo_RealData(t *testing.T) {
	info := newTypeInfo()
	populateCommandInfo(info)

	if got, want := len(info.Map), len(script.ScriptOpcodeMap); got != want {
		t.Errorf("len(Map) = %d, want %d (one entry per ScriptOpcodeMap)", got, want)
	}
	// Spot-check: opcode 0 is PUSH_CONSTANT_INT.
	if got, want := info.Map[0], "push_constant_int"; got != want {
		t.Errorf("Map[0] = %q, want %q (lowercased)", got, want)
	}
	// Ordering sanity: iterate the entries we got and confirm none have
	// Map[op] populated for op > Max-1 — this would indicate a sort bug.
	maxOp := -1
	for op := range info.Map {
		if op > maxOp {
			maxOp = op
		}
	}
	if maxOp != info.Max-1 {
		t.Errorf("max(Map keys) = %d but Max-1 = %d; ascending-iteration invariant broken", maxOp, info.Max-1)
	}
	// Confirm sort.Slice is referenced (compile-only — prevent stale import).
	_ = sort.Slice
}
```

- [ ] **Step 2: Run to verify failure**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/... -run TestPopulateCommandInfo -v`

Expected: FAIL with `undefined: populateCommandInfo` and `undefined: populateCommandInfoFrom`.

- [ ] **Step 3: Implement (append to `pkg/pack/compiler/symbols.go`)**

Add to imports: `"sort"`, `"github.com/zsrv/goscape/pkg/script"`.

Append:

```go
// populateCommandInfo populates the command TypeInfo with one entry per
// ScriptOpcodeMap key plus pointer-flag enrichments from
// ScriptOpcodePointers. Wraps populateCommandInfoFrom with the package
// globals; the seam exists so tests can pass synthetic data.
func populateCommandInfo(info *TypeInfo) {
	populateCommandInfoFrom(info, script.ScriptOpcodeMap, script.ScriptOpcodePointers)
}

// populateCommandInfoFrom is the testable seam under populateCommandInfo.
// Mirrors TS Compiler.ts:110-150 (allCommands sort + commandInfo build).
//
// NAI-202-D-CORRUPT2-FIELD: TS Compiler.ts:146-147 has a typo — the
// corrupt2 arm assigns to commandInfo.corrupt[opcode] (overwriting
// commandInfo.corrupt[opcode] just-written one line above) instead of
// commandInfo.corrupt2[opcode]. Goscape writes to info.Corrupt2[op].
func populateCommandInfoFrom(
	info *TypeInfo,
	opmap map[string]script.Opcode,
	pointers map[script.Opcode]script.Pointers,
) {
	type entry struct {
		name   string
		opcode script.Opcode
	}
	entries := make([]entry, 0, len(opmap))
	for n, op := range opmap {
		entries = append(entries, entry{n, op})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].opcode < entries[j].opcode
	})

	for _, e := range entries {
		op := int(e.opcode)
		info.Add(op, strings.ToLower(e.name), true)

		ptrs, hasPtrs := pointers[e.opcode]
		if !hasPtrs {
			continue
		}

		if len(ptrs.Require) > 0 {
			info.Require[op] = strings.Join(ptrs.Require, ",")
			if len(ptrs.Require2) > 0 {
				info.Require2[op] = strings.Join(ptrs.Require2, ",")
			}
		}

		if len(ptrs.Set) > 0 {
			if ptrs.Conditional {
				info.Conditional[op] = true
			}
			info.Set[op] = strings.Join(ptrs.Set, ",")
			if len(ptrs.Set2) > 0 {
				info.Set2[op] = strings.Join(ptrs.Set2, ",")
			}
		}

		if len(ptrs.Corrupt) > 0 {
			info.Corrupt[op] = strings.Join(ptrs.Corrupt, ",")
			if len(ptrs.Corrupt2) > 0 {
				// NAI-202-D-CORRUPT2-FIELD: write to Corrupt2, not Corrupt.
				info.Corrupt2[op] = strings.Join(ptrs.Corrupt2, ",")
			}
		}
	}
}
```

- [ ] **Step 4: Run to verify pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/... -run TestPopulateCommandInfo -v`

Expected: All 6 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/pack/compiler/symbols.go pkg/pack/compiler/symbols_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(compiler): NAI-202 T5 — populateCommandInfo + CORRUPT2-FIELD fix

Ports TS Compiler.ts:110-150 commandInfo build. Iterates ScriptOpcodeMap
ascending by opcode value (sort.Slice), lowercases names, applies
ScriptOpcodePointers enrichments to Require/Require2/Set/Set2/Corrupt/
Corrupt2/Conditional maps. The populateCommandInfoFrom seam takes
opmap+pointers as args so unit tests can pin behaviour with synthetic
Pointers (the static ScriptOpcodePointers has no Corrupt2-populated
entries currently).

Deviation: NAI-202-D-CORRUPT2-FIELD — TS Compiler.ts:147 typo (overwrites
.corrupt instead of writing to .corrupt2) is fixed; pin test
TestPopulateCommandInfoFrom_Corrupt2FieldFix anchors the corrected
destination.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: `populateInterfaceOverlay`

**Files:**
- Modify: `pkg/pack/compiler/symbols.go` (append)
- Modify: `pkg/pack/compiler/symbols_test.go` (append)

- [ ] **Step 1: Write failing tests (append to `symbols_test.go`)**

```go
// TestPopulateInterfaceOverlay_PrefersComName pins TS Compiler.ts:225 —
// when com.comName is non-empty, that name takes precedence over the
// componentInfo.Map[id] fallback.
func TestPopulateInterfaceOverlay_PrefersComName(t *testing.T) {
	componentInfo := newTypeInfo()
	componentInfo.Add(5, "fallback_name", true)

	components := &objtype.ComponentTypeConfigs{
		Configs: make([]*objtype.ComponentType, 10),
	}
	components.Configs[5] = &objtype.ComponentType{
		ComName: "preferred_name",
		Overlay: false,
	}

	interfaceInfo := newTypeInfo()
	overlayInfo := newTypeInfo()
	populateInterfaceOverlay(componentInfo, interfaceInfo, overlayInfo, components)

	if got, want := interfaceInfo.Map[5], "preferred_name"; got != want {
		t.Errorf("interface.Map[5] = %q, want %q (com.comName must override fallback)", got, want)
	}
}

// TestPopulateInterfaceOverlay_FallsBackToComponentInfoMap pins the
// `com.comName || componentInfo.map[id]` fallback at TS Compiler.ts:225.
func TestPopulateInterfaceOverlay_FallsBackToComponentInfoMap(t *testing.T) {
	componentInfo := newTypeInfo()
	componentInfo.Add(3, "from_pack_file", true)

	components := &objtype.ComponentTypeConfigs{
		Configs: make([]*objtype.ComponentType, 10),
	}
	components.Configs[3] = &objtype.ComponentType{
		ComName: "",
		Overlay: false,
	}

	interfaceInfo := newTypeInfo()
	overlayInfo := newTypeInfo()
	populateInterfaceOverlay(componentInfo, interfaceInfo, overlayInfo, components)

	if got, want := interfaceInfo.Map[3], "from_pack_file"; got != want {
		t.Errorf("interface.Map[3] = %q, want %q (fallback to componentInfo)", got, want)
	}
}

// TestPopulateInterfaceOverlay_OverlayOnlyOnTrue pins TS Compiler.ts:229-231
// — overlayInfo gets the entry only when com.Overlay == true.
func TestPopulateInterfaceOverlay_OverlayOnlyOnTrue(t *testing.T) {
	componentInfo := newTypeInfo()
	componentInfo.Add(1, "a", true)
	componentInfo.Add(2, "b", true)

	components := &objtype.ComponentTypeConfigs{
		Configs: make([]*objtype.ComponentType, 10),
	}
	components.Configs[1] = &objtype.ComponentType{ComName: "a", Overlay: true}
	components.Configs[2] = &objtype.ComponentType{ComName: "b", Overlay: false}

	interfaceInfo := newTypeInfo()
	overlayInfo := newTypeInfo()
	populateInterfaceOverlay(componentInfo, interfaceInfo, overlayInfo, components)

	if _, ok := overlayInfo.Map[1]; !ok {
		t.Errorf("overlay.Map[1]: missing; want present (overlay=true)")
	}
	if _, ok := overlayInfo.Map[2]; ok {
		t.Errorf("overlay.Map[2]: present; want absent (overlay=false)")
	}
}

// TestPopulateInterfaceOverlay_SkipsNilConfig pins TS Compiler.ts:221-223
// — Configs[id] == nil triggers `continue`.
func TestPopulateInterfaceOverlay_SkipsNilConfig(t *testing.T) {
	componentInfo := newTypeInfo()
	componentInfo.Add(4, "x", true)

	components := &objtype.ComponentTypeConfigs{
		Configs: make([]*objtype.ComponentType, 10),
	}
	// Configs[4] left nil.

	interfaceInfo := newTypeInfo()
	overlayInfo := newTypeInfo()
	populateInterfaceOverlay(componentInfo, interfaceInfo, overlayInfo, components)

	if _, ok := interfaceInfo.Map[4]; ok {
		t.Errorf("interface.Map[4]: present; want absent (nil Configs[4] should skip)")
	}
	if _, ok := overlayInfo.Map[4]; ok {
		t.Errorf("overlay.Map[4]: present; want absent")
	}
}

// TestPopulateInterfaceOverlay_SkipsIdsAbsentFromComponentInfo pins TS
// Compiler.ts:216-218 — ids without a componentInfo.Map[id] entry get
// skipped (inclusive <= Max loop with map-presence guard).
func TestPopulateInterfaceOverlay_SkipsIdsAbsentFromComponentInfo(t *testing.T) {
	componentInfo := newTypeInfo()
	componentInfo.Add(0, "zero", true)
	componentInfo.Add(5, "five", true) // Max becomes 6; ids 1-4 absent

	components := &objtype.ComponentTypeConfigs{
		Configs: make([]*objtype.ComponentType, 10),
	}
	// Populate Configs[2] even though componentInfo skips id 2.
	components.Configs[2] = &objtype.ComponentType{ComName: "two", Overlay: true}

	interfaceInfo := newTypeInfo()
	overlayInfo := newTypeInfo()
	populateInterfaceOverlay(componentInfo, interfaceInfo, overlayInfo, components)

	if _, ok := interfaceInfo.Map[2]; ok {
		t.Errorf("interface.Map[2]: present; want absent (componentInfo.Map[2] missing → skip)")
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/... -run TestPopulateInterfaceOverlay -v`

Expected: FAIL with `undefined: populateInterfaceOverlay`.

- [ ] **Step 3: Append implementation**

```go
// populateInterfaceOverlay derives the `interface` and `overlayinterface`
// TypeInfos from componentInfo (loaded from interface.pack) enriched
// with Component.ComName / Component.Overlay from the cache loader.
// Mirrors TS Compiler.ts:214-232.
//
// `name` is com.ComName if non-empty, else componentInfo.Map[id]
// (TS `com.comName || componentInfo.map[id]`).
//
// Per TS Compiler.ts:215, the loop bound is `id <= componentInfo.Max`
// and the inner guards are the standard `Map[id]` presence check + a
// `Configs[id] != nil` check.
func populateInterfaceOverlay(
	componentInfo, interfaceInfo, overlayInfo *TypeInfo,
	components *objtype.ComponentTypeConfigs,
) {
	if components == nil {
		return
	}
	for id := 0; id <= componentInfo.Max; id++ {
		baseName, present := componentInfo.Map[id]
		if !present {
			continue
		}
		if id < 0 || id >= len(components.Configs) {
			continue
		}
		com := components.Configs[id]
		if com == nil {
			continue
		}
		name := com.ComName
		if name == "" {
			name = baseName
		}
		interfaceInfo.Add(id, name, true)
		if com.Overlay {
			overlayInfo.Add(id, name, true)
		}
	}
}
```

- [ ] **Step 4: Run to verify pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/... -run TestPopulateInterfaceOverlay -v`

Expected: All 5 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/pack/compiler/symbols.go pkg/pack/compiler/symbols_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(compiler): NAI-202 T6 — populateInterfaceOverlay synth

Ports TS Compiler.ts:214-232. Builds the `interface` and
`overlayinterface` TypeInfos from componentInfo (interface.pack contents)
enriched with Component.ComName / Component.Overlay from the cache
loader. Uses `com.ComName || componentInfo.Map[id]` fallback;
`overlayinterface` only populated when `com.Overlay == true`.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 7: `populateDbColumns`

**Files:**
- Modify: `pkg/pack/compiler/symbols.go` (append)
- Modify: `pkg/pack/compiler/symbols_test.go` (append)

- [ ] **Step 1: Write failing tests (append)**

```go
// TestPopulateDbColumns_SingleTypeColumn pins TS Compiler.ts:285-287 for
// a 1-type column: one primary entry, no tuple entries, Max unchanged.
func TestPopulateDbColumns_SingleTypeColumn(t *testing.T) {
	tables := &objtype.DbTableTypeConfigs{
		Configs: []*objtype.DbTableType{
			{
				ConfigType:  objtype.ConfigType{ID: 1, DebugName: "tbl1"},
				ColumnNames: []string{"col0"},
				Types:       [][]objtype.ScriptVarType{{objtype.ScriptVarTypeInt}},
			},
		},
	}

	info := newTypeInfo()
	populateDbColumns(info, tables)

	primaryID := (1 << 12) | (0 << 4) // = 4096
	if got, want := info.Map[primaryID], "tbl1:col0"; got != want {
		t.Errorf("Map[%d] = %q, want %q", primaryID, got, want)
	}
	if got, want := info.VarType[primaryID], "int"; got != want {
		t.Errorf("VarType[%d] = %q, want %q", primaryID, got, want)
	}
	if info.Max != -1 {
		t.Errorf("Max = %d, want -1 (updateMax=false)", info.Max)
	}
	// No tuple entries.
	for id := range info.Map {
		if id != primaryID {
			t.Errorf("unexpected Map entry at %d: %q (single-type column should produce no tuples)", id, info.Map[id])
		}
	}
}

// TestPopulateDbColumns_MultiTypeColumn pins TS Compiler.ts:289-294 for a
// 2-type column: primary entry with comma-joined vartypes + 2 tuple
// entries with single vartype each.
func TestPopulateDbColumns_MultiTypeColumn(t *testing.T) {
	tables := &objtype.DbTableTypeConfigs{
		Configs: []*objtype.DbTableType{
			{
				ConfigType:  objtype.ConfigType{ID: 1, DebugName: "tbl1"},
				ColumnNames: []string{"col0"},
				Types: [][]objtype.ScriptVarType{
					{objtype.ScriptVarTypeInt, objtype.ScriptVarTypeObj},
				},
			},
		},
	}

	info := newTypeInfo()
	populateDbColumns(info, tables)

	primary := (1 << 12) | (0 << 4) // 4096
	tup1 := primary | 1             // 4097
	tup2 := primary | 2             // 4098

	if got, want := info.Map[primary], "tbl1:col0"; got != want {
		t.Errorf("Map[primary=%d] = %q, want %q", primary, got, want)
	}
	if got, want := info.VarType[primary], "int,obj"; got != want {
		t.Errorf("VarType[primary] = %q, want %q", got, want)
	}
	if got, want := info.Map[tup1], "tbl1:col0:0"; got != want {
		t.Errorf("Map[tup1=%d] = %q, want %q", tup1, got, want)
	}
	if got, want := info.VarType[tup1], "int"; got != want {
		t.Errorf("VarType[tup1] = %q, want %q", got, want)
	}
	if got, want := info.Map[tup2], "tbl1:col0:1"; got != want {
		t.Errorf("Map[tup2=%d] = %q, want %q", tup2, got, want)
	}
	if got, want := info.VarType[tup2], "obj"; got != want {
		t.Errorf("VarType[tup2] = %q, want %q", got, want)
	}
}

// TestPopulateDbColumns_BitfieldEncoding pins the exact ID arithmetic for
// non-trivial (table, column) values. table=2, column=5, types=[STRING]
// → primary id = (2<<12) | (5<<4) = 8192 | 80 = 8272.
func TestPopulateDbColumns_BitfieldEncoding(t *testing.T) {
	tables := &objtype.DbTableTypeConfigs{
		Configs: []*objtype.DbTableType{
			nil, nil,
			{
				ConfigType:  objtype.ConfigType{ID: 2, DebugName: "tbl2"},
				ColumnNames: []string{"a", "b", "c", "d", "e", "col5"},
				Types: [][]objtype.ScriptVarType{
					nil, nil, nil, nil, nil,
					{objtype.ScriptVarTypeString},
				},
			},
		},
	}

	info := newTypeInfo()
	populateDbColumns(info, tables)

	want := (2 << 12) | (5 << 4)
	if want != 8272 {
		t.Fatalf("want arithmetic wrong: (2<<12)|(5<<4) = %d, expected 8272", want)
	}
	if got, ok := info.Map[want]; !ok || got != "tbl2:col5" {
		t.Errorf("Map[%d] = %q (present=%v), want \"tbl2:col5\"", want, got, ok)
	}
}

// TestPopulateDbColumns_NilColumnTypes pins: a column whose Types[col]
// is nil (no `code 1` block written) is skipped.
func TestPopulateDbColumns_NilColumnTypes(t *testing.T) {
	tables := &objtype.DbTableTypeConfigs{
		Configs: []*objtype.DbTableType{
			{
				ConfigType:  objtype.ConfigType{ID: 1, DebugName: "tbl1"},
				ColumnNames: []string{"present", "absent"},
				Types: [][]objtype.ScriptVarType{
					{objtype.ScriptVarTypeInt},
					nil,
				},
			},
		},
	}

	info := newTypeInfo()
	populateDbColumns(info, tables)

	presentID := (1 << 12) | (0 << 4)
	if _, ok := info.Map[presentID]; !ok {
		t.Errorf("Map[%d]: missing; want present (column 0 has types)", presentID)
	}
	absentID := (1 << 12) | (1 << 4)
	if _, ok := info.Map[absentID]; ok {
		t.Errorf("Map[%d]: present; want absent (column 1 has nil types)", absentID)
	}
}

// TestPopulateDbColumns_SkipsNilTable pins: a nil entry in tables.Configs
// is skipped (TS Compiler.ts:277 inclusive-loop with `Map[id]` guard;
// goscape mirrors by guarding on Configs[id] != nil).
func TestPopulateDbColumns_SkipsNilTable(t *testing.T) {
	tables := &objtype.DbTableTypeConfigs{
		Configs: []*objtype.DbTableType{
			nil,
			{
				ConfigType:  objtype.ConfigType{ID: 1, DebugName: "tbl1"},
				ColumnNames: []string{"col0"},
				Types:       [][]objtype.ScriptVarType{{objtype.ScriptVarTypeInt}},
			},
		},
	}

	info := newTypeInfo()
	populateDbColumns(info, tables)

	// Table 0 is nil → no entries.
	id0 := (0 << 12) | (0 << 4) // = 0
	if _, ok := info.Map[id0]; ok {
		t.Errorf("Map[0]: present; want absent (Configs[0] nil)")
	}
	// Table 1 → present.
	id1 := (1 << 12) | (0 << 4)
	if _, ok := info.Map[id1]; !ok {
		t.Errorf("Map[%d]: missing; want present", id1)
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/... -run TestPopulateDbColumns -v`

Expected: FAIL with `undefined: populateDbColumns`.

- [ ] **Step 3: Implement**

Append to `pkg/pack/compiler/symbols.go`:

```go
// populateDbColumns synthesizes the dbcolumn TypeInfo from DbTableType
// column metadata. Mirrors TS Compiler.ts:275-297.
//
// Bitfield-encoded column ids:
//   - primary id  = (table.ID & 0xffff) << 12 | (column & 0x7f) << 4
//   - tuple id    = primary | ((tuple + 1) & 0xf)     // only if len(types) > 1
//
// .Add is called with updateMax=false on all entries — dbcolumn.Max
// stays at -1 (matching TS Compiler.ts:286,292 third arg).
//
// .VarType[primary] is the comma-joined list of all type names.
// .VarType[tuple_n] is the single tuple type name.
//
// Skips: nil table entries; columns whose Types[col] is nil.
func populateDbColumns(info *TypeInfo, tables *objtype.DbTableTypeConfigs) {
	if tables == nil {
		return
	}
	for _, table := range tables.Configs {
		if table == nil {
			continue
		}
		for column, types := range table.Types {
			if types == nil {
				continue
			}
			primary := int(((table.ID & 0xffff) << 12) | ((column & 0x7f) << 4))

			typeNames := make([]string, len(types))
			for i, t := range types {
				typeNames[i] = scriptVarTypeName(t)
			}
			columnName := ""
			if column < len(table.ColumnNames) {
				columnName = table.ColumnNames[column]
			}
			primaryLabel := fmt.Sprintf("%s:%s", table.DebugName, columnName)
			info.Add(primary, primaryLabel, false)
			info.VarType[primary] = strings.Join(typeNames, ",")

			if len(types) > 1 {
				for tuple := range types {
					tupleID := primary | ((tuple + 1) & 0xf)
					tupleLabel := fmt.Sprintf("%s:%s:%d", table.DebugName, columnName, tuple)
					info.Add(tupleID, tupleLabel, false)
					info.VarType[tupleID] = typeNames[tuple]
				}
			}
		}
	}
}
```

- [ ] **Step 4: Run to verify pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/... -run TestPopulateDbColumns -v`

Expected: All 5 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/pack/compiler/symbols.go pkg/pack/compiler/symbols_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(compiler): NAI-202 T7 — populateDbColumns bitfield synth

Ports TS Compiler.ts:275-297. Synthesizes dbcolumn TypeInfo from
DbTableType column metadata with bitfield-encoded ids:
  primary  = (table.ID & 0xffff) << 12 | (column & 0x7f) << 4
  tuple    = primary | ((tuple+1) & 0xf)   // multi-type columns only

VarType strings comma-joined for primary entries, single-typed for
tuple entries. dbcolumn.Max stays at -1 (updateMax=false on all adds).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 8: Five enrichment helpers + VARN-LOOP-GUARD pin

**Files:**
- Modify: `pkg/pack/compiler/symbols.go` (append)
- Modify: `pkg/pack/compiler/symbols_test.go` (append)

- [ ] **Step 1: Write failing tests (append to `symbols_test.go`)**

```go
// TestEnrichWriteinvInfo pins TS Compiler.ts:203-212 — writeinv.Protect[id]
// reflects the loaded InvType.Protect (which defaults to true, falsified
// by op-code-7 in the .dat).
func TestEnrichWriteinvInfo(t *testing.T) {
	writeinv := newTypeInfo()
	writeinv.Add(0, "main_inv", true)
	writeinv.Add(1, "worn_inv", true)

	invs := &objtype.InvTypeConfigs{
		Configs: []*objtype.InvType{
			{ConfigType: objtype.ConfigType{ID: 0}, Protect: true},
			{ConfigType: objtype.ConfigType{ID: 1}, Protect: false},
		},
	}

	enrichWriteinvInfo(writeinv, invs)

	if got, want := writeinv.Protect[0], true; got != want {
		t.Errorf("Protect[0] = %v, want %v", got, want)
	}
	if got, want := writeinv.Protect[1], false; got != want {
		t.Errorf("Protect[1] = %v, want %v", got, want)
	}
}

// TestEnrichVarpInfo pins TS Compiler.ts:234-243 — varp.VarType[id] and
// varp.Protect[id] reflect VarPlayerType.Type / VarPlayerType.Protect.
func TestEnrichVarpInfo(t *testing.T) {
	varp := newTypeInfo()
	varp.Add(0, "v0", true)

	configs := &objtype.VarpTypeConfigs{
		Configs: []*objtype.VarPlayerType{
			{
				ConfigType: objtype.ConfigType{ID: 0},
				Type:       objtype.ScriptVarTypeInt,
				Protect:    true,
			},
		},
	}

	enrichVarpInfo(varp, configs)

	if got, want := varp.VarType[0], "int"; got != want {
		t.Errorf("VarType[0] = %q, want %q", got, want)
	}
	if got, want := varp.Protect[0], true; got != want {
		t.Errorf("Protect[0] = %v, want %v", got, want)
	}
}

// TestEnrichVarnInfo_HappyPath pins TS Compiler.ts:245-253 (corrected
// per NAI-202-D-VARN-LOOP-GUARD).
func TestEnrichVarnInfo_HappyPath(t *testing.T) {
	varn := newTypeInfo()
	varn.Add(0, "n0", true)

	configs := &objtype.VarnTypeConfigs{
		Configs: []*objtype.VarNpcType{
			{ConfigType: objtype.ConfigType{ID: 0}, Type: objtype.ScriptVarTypeString},
		},
	}

	enrichVarnInfo(varn, configs)

	if got, want := varn.VarType[0], "string"; got != want {
		t.Errorf("VarType[0] = %q, want %q", got, want)
	}
}

// TestEnrichVarnInfo_VarnLoopGuardFix pins NAI-202-D-VARN-LOOP-GUARD: a
// varn id that has no corresponding varp at the same id MUST still get
// a vartype emitted. TS Compiler.ts:247 reads `varpInfo.map[id]` (typo);
// goscape reads varn's own .Map.
func TestEnrichVarnInfo_VarnLoopGuardFix(t *testing.T) {
	varn := newTypeInfo()
	varn.Add(7, "lonely_varn", true) // id=7 — no varp at this id

	configs := &objtype.VarnTypeConfigs{
		Configs: make([]*objtype.VarNpcType, 10),
	}
	configs.Configs[7] = &objtype.VarNpcType{
		ConfigType: objtype.ConfigType{ID: 7},
		Type:       objtype.ScriptVarTypeBoolean,
	}

	enrichVarnInfo(varn, configs)

	if got, want := varn.VarType[7], "boolean"; got != want {
		t.Errorf("VarType[7] = %q, want %q (NAI-202-D-VARN-LOOP-GUARD: varn-only id must enrich)", got, want)
	}
}

// TestEnrichVarsInfo pins TS Compiler.ts:255-263.
func TestEnrichVarsInfo(t *testing.T) {
	vars := newTypeInfo()
	vars.Add(0, "s0", true)

	configs := &objtype.VarsTypeConfigs{
		Configs: []*objtype.VarSharedType{
			{ConfigType: objtype.ConfigType{ID: 0}, Type: objtype.ScriptVarTypeCoord},
		},
	}

	enrichVarsInfo(vars, configs)

	if got, want := vars.VarType[0], "coord"; got != want {
		t.Errorf("VarType[0] = %q, want %q", got, want)
	}
}

// TestEnrichParamInfo pins TS Compiler.ts:265-273 — uses ParamType.GetType()
// directly (rather than scriptVarTypeName) to honour the existing
// instance method.
func TestEnrichParamInfo(t *testing.T) {
	param := newTypeInfo()
	param.Add(0, "p0", true)

	configs := &objtype.ParamTypeConfigs{
		Configs: []*objtype.ParamType{
			{
				ConfigType: objtype.ConfigType{ID: 0},
				Type:       objtype.ScriptVarTypeNamedObj,
			},
		},
	}

	enrichParamInfo(param, configs)

	if got, want := param.VarType[0], "namedobj"; got != want {
		t.Errorf("VarType[0] = %q, want %q", got, want)
	}
}

// TestEnrich_SkipsIdsAbsentFromMap pins the inclusive-<=Max + Map-presence
// guard on every enricher (TS Compiler.ts:206-208 etc.).
func TestEnrich_SkipsIdsAbsentFromMap(t *testing.T) {
	// One enricher exercises the pattern; the others share the same
	// loop shape.
	writeinv := newTypeInfo()
	writeinv.Add(0, "present", true)
	writeinv.Add(5, "present", true) // Max=6; ids 1..4 absent

	invs := &objtype.InvTypeConfigs{
		Configs: make([]*objtype.InvType, 10),
	}
	for i := range invs.Configs {
		invs.Configs[i] = &objtype.InvType{
			ConfigType: objtype.ConfigType{ID: i},
			Protect:    true,
		}
	}

	enrichWriteinvInfo(writeinv, invs)

	for i := 1; i <= 4; i++ {
		if _, ok := writeinv.Protect[i]; ok {
			t.Errorf("Protect[%d]: present; want absent (id missing from Map)", i)
		}
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/... -run TestEnrich -v`

Expected: FAIL with `undefined: enrichWriteinvInfo` and the other four.

- [ ] **Step 3: Implement (append to `symbols.go`)**

```go
// enrichWriteinvInfo populates writeinv.Protect[id] from
// InvType.Protect for every id present in writeinv.Map. Mirrors TS
// Compiler.ts:203-212.
func enrichWriteinvInfo(info *TypeInfo, configs *objtype.InvTypeConfigs) {
	if configs == nil {
		return
	}
	for id := 0; id <= info.Max; id++ {
		if _, ok := info.Map[id]; !ok {
			continue
		}
		if id < 0 || id >= len(configs.Configs) {
			continue
		}
		inv := configs.Configs[id]
		if inv == nil {
			continue
		}
		info.Protect[id] = inv.Protect
	}
}

// enrichVarpInfo populates varp.VarType and varp.Protect from
// VarPlayerType fields. Mirrors TS Compiler.ts:234-243.
func enrichVarpInfo(info *TypeInfo, configs *objtype.VarpTypeConfigs) {
	if configs == nil {
		return
	}
	for id := 0; id <= info.Max; id++ {
		if _, ok := info.Map[id]; !ok {
			continue
		}
		if id < 0 || id >= len(configs.Configs) {
			continue
		}
		varp := configs.Configs[id]
		if varp == nil {
			continue
		}
		info.VarType[id] = scriptVarTypeName(varp.Type)
		info.Protect[id] = varp.Protect
	}
}

// enrichVarnInfo populates varn.VarType from VarNpcType.Type. Mirrors TS
// Compiler.ts:245-253 with NAI-202-D-VARN-LOOP-GUARD fix: the loop guard
// reads info.Map[id] (this TypeInfo's own Map), not varpInfo.Map[id]
// (the TS typo).
func enrichVarnInfo(info *TypeInfo, configs *objtype.VarnTypeConfigs) {
	if configs == nil {
		return
	}
	for id := 0; id <= info.Max; id++ {
		if _, ok := info.Map[id]; !ok {
			continue
		}
		if id < 0 || id >= len(configs.Configs) {
			continue
		}
		varn := configs.Configs[id]
		if varn == nil {
			continue
		}
		info.VarType[id] = scriptVarTypeName(varn.Type)
	}
}

// enrichVarsInfo populates vars.VarType from VarSharedType.Type. Mirrors
// TS Compiler.ts:255-263.
func enrichVarsInfo(info *TypeInfo, configs *objtype.VarsTypeConfigs) {
	if configs == nil {
		return
	}
	for id := 0; id <= info.Max; id++ {
		if _, ok := info.Map[id]; !ok {
			continue
		}
		if id < 0 || id >= len(configs.Configs) {
			continue
		}
		vars := configs.Configs[id]
		if vars == nil {
			continue
		}
		info.VarType[id] = scriptVarTypeName(vars.Type)
	}
}

// enrichParamInfo populates param.VarType from ParamType.GetType(). Mirrors
// TS Compiler.ts:265-273. Uses ParamType's existing instance method
// rather than scriptVarTypeName — they share the same switch but the
// method is already exported.
func enrichParamInfo(info *TypeInfo, configs *objtype.ParamTypeConfigs) {
	if configs == nil {
		return
	}
	for id := 0; id <= info.Max; id++ {
		if _, ok := info.Map[id]; !ok {
			continue
		}
		if id < 0 || id >= len(configs.Configs) {
			continue
		}
		param := configs.Configs[id]
		if param == nil {
			continue
		}
		info.VarType[id] = param.GetType()
	}
}
```

- [ ] **Step 4: Run to verify pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/... -run TestEnrich -v`

Expected: All 7 enrich tests PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/pack/compiler/symbols.go pkg/pack/compiler/symbols_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(compiler): NAI-202 T8 — five enrich helpers + VARN-LOOP-GUARD fix

Ports TS Compiler.ts:203-273 enrichment loops as five focused helpers:
enrichWriteinvInfo, enrichVarpInfo, enrichVarnInfo, enrichVarsInfo,
enrichParamInfo. Each shares the inclusive-<=Max iteration with a
Map-presence guard and a Configs[id] != nil guard. Param uses the
existing ParamType.GetType() method; the other three go through
scriptVarTypeName (NAI-202 T4).

Deviation: NAI-202-D-VARN-LOOP-GUARD — TS Compiler.ts:247 typo
(`varpInfo.map[id]` inside a varn loop) is fixed; pin test
TestEnrichVarnInfo_VarnLoopGuardFix anchors the corrected behaviour for
varn ids absent from varp.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 9: `BuildSymbols` integration

**Files:**
- Modify: `pkg/pack/compiler/symbols.go` (append)
- Modify: `pkg/pack/compiler/symbols_test.go` (append)

- [ ] **Step 1: Write failing tests (append)**

```go
// TestBuildSymbolsCore_AllCategoriesPresent pins §5.8: the returned map
// has exactly 32 keys covering the TS symbols dict at Compiler.ts:330-365.
func TestBuildSymbolsCore_AllCategoriesPresent(t *testing.T) {
	dir := t.TempDir()
	// Seed a few .pack files so Load returns non-nil TypeInfo; absent ones
	// also return non-nil (Load is silent-on-missing).
	writePackFile(t, dir, "npc.pack", "0=goblin\n")
	writePackFile(t, dir, "obj.pack", "0=bronze_sword\n")

	loaders := emptyConfigLoaders()

	symbols, err := buildSymbolsCore(dir, loaders)
	if err != nil {
		t.Fatalf("buildSymbolsCore: %v", err)
	}

	wantKeys := []string{
		"command", "constant", "npc", "obj", "inv", "writeinv", "seq",
		"idk", "spotanim", "loc", "component", "interface",
		"overlayinterface", "varp", "varn", "vars", "param", "struct",
		"enum", "hunt", "mesanim", "synth", "category", "runescript",
		"dbtable", "dbcolumn", "dbrow", "stat", "npc_stat", "npc_mode",
		"fontmetrics", "locshape",
	}
	if got, want := len(symbols), len(wantKeys); got != want {
		t.Errorf("len(symbols) = %d, want %d", got, want)
	}
	for _, k := range wantKeys {
		if _, ok := symbols[k]; !ok {
			t.Errorf("symbols[%q]: missing", k)
		}
		if symbols[k] == nil {
			t.Errorf("symbols[%q]: nil *TypeInfo (every category must be non-nil)", k)
		}
	}
}

// TestBuildSymbolsCore_StaticArraysPopulated pins the LoadArray inputs
// (fontmetrics, locshape) against TS Compiler.ts:303-328.
func TestBuildSymbolsCore_StaticArraysPopulated(t *testing.T) {
	dir := t.TempDir()
	loaders := emptyConfigLoaders()
	symbols, err := buildSymbolsCore(dir, loaders)
	if err != nil {
		t.Fatalf("buildSymbolsCore: %v", err)
	}

	// fontmetrics — 4 entries.
	fm := symbols["fontmetrics"]
	wantFm := []string{"p11", "p12", "b12", "q8"}
	if got, want := len(fm.Map), len(wantFm); got != want {
		t.Errorf("len(fontmetrics.Map) = %d, want %d", got, want)
	}
	for i, name := range wantFm {
		if got := fm.Map[i]; got != name {
			t.Errorf("fontmetrics.Map[%d] = %q, want %q", i, got, name)
		}
	}

	// locshape — 23 entries.
	ls := symbols["locshape"]
	if got, want := len(ls.Map), 23; got != want {
		t.Errorf("len(locshape.Map) = %d, want %d", got, want)
	}
	if got, want := ls.Map[0], "wall_straight"; got != want {
		t.Errorf("locshape.Map[0] = %q, want %q", got, want)
	}
	if got, want := ls.Map[22], "grounddecor"; got != want {
		t.Errorf("locshape.Map[22] = %q, want %q (final entry)", got, want)
	}
}

// TestBuildSymbolsCore_MetaMaps pins TS Compiler.ts:300-302 — the three
// LoadMap(valueAsKey=true) outputs (stat, npc_stat, npc_mode).
func TestBuildSymbolsCore_MetaMaps(t *testing.T) {
	dir := t.TempDir()
	loaders := emptyConfigLoaders()
	symbols, err := buildSymbolsCore(dir, loaders)
	if err != nil {
		t.Fatalf("buildSymbolsCore: %v", err)
	}

	// stat: PlayerStatMap with valueAsKey=true → NameMap["0"] = "attack" etc.
	stat := symbols["stat"]
	if got, want := stat.NameMap["0"], "attack"; got != want {
		t.Errorf("stat.NameMap[\"0\"] = %q, want %q", got, want)
	}

	// npc_stat: NpcStatMap with valueAsKey=true → NameMap["0"] = "attack" etc.
	npcStat := symbols["npc_stat"]
	if got, want := npcStat.NameMap["0"], "attack"; got != want {
		t.Errorf("npc_stat.NameMap[\"0\"] = %q, want %q", got, want)
	}

	// npc_mode: NpcModeMap with valueAsKey=true → NameMap["-1"] = "null" etc.
	npcMode := symbols["npc_mode"]
	if got, want := npcMode.NameMap["-1"], "null"; got != want {
		t.Errorf("npc_mode.NameMap[\"-1\"] = %q, want %q", got, want)
	}
}

// TestBuildSymbolsCore_ConstantsLoaded pins TS Compiler.ts:152-174 ->
// LoadRecords flow.
func TestBuildSymbolsCore_ConstantsLoaded(t *testing.T) {
	dir := t.TempDir()
	writeConstantFile(t, dir, "a.constant", "^FOO=bar\nBAZ=\"quoted\"\n")

	loaders := emptyConfigLoaders()
	symbols, err := buildSymbolsCore(dir, loaders)
	if err != nil {
		t.Fatalf("buildSymbolsCore: %v", err)
	}

	c := symbols["constant"]
	if got, want := c.NameMap["FOO"], "bar"; got != want {
		t.Errorf("constant.NameMap[FOO] = %q, want %q", got, want)
	}
	if got, want := c.NameMap["BAZ"], "quoted"; got != want {
		t.Errorf("constant.NameMap[BAZ] = %q, want %q (quote-stripped)", got, want)
	}
}

// TestBuildSymbolsCore_PackFilesConsumed pins TS Compiler.ts:177-200 — each
// of the 22 .pack files lands as the named TypeInfo.
func TestBuildSymbolsCore_PackFilesConsumed(t *testing.T) {
	dir := t.TempDir()
	writePackFile(t, dir, "inv.pack", "5=secret_stash\n")
	writePackFile(t, dir, "script.pack", "99=some_script\n")

	loaders := emptyConfigLoaders()
	symbols, err := buildSymbolsCore(dir, loaders)
	if err != nil {
		t.Fatalf("buildSymbolsCore: %v", err)
	}

	if got, want := symbols["inv"].Map[5], "secret_stash"; got != want {
		t.Errorf("inv.Map[5] = %q, want %q", got, want)
	}
	// script.pack → "runescript" symbol key.
	if got, want := symbols["runescript"].Map[99], "some_script"; got != want {
		t.Errorf("runescript.Map[99] = %q, want %q (script.pack → \"runescript\" key)", got, want)
	}
}

// TestBuildSymbolsCore_CommandPopulated pins the commandInfo step at
// integration level (T5 covers the unit-level behaviour).
func TestBuildSymbolsCore_CommandPopulated(t *testing.T) {
	dir := t.TempDir()
	loaders := emptyConfigLoaders()
	symbols, err := buildSymbolsCore(dir, loaders)
	if err != nil {
		t.Fatalf("buildSymbolsCore: %v", err)
	}

	cmd := symbols["command"]
	if got, want := len(cmd.Map), len(script.ScriptOpcodeMap); got != want {
		t.Errorf("len(command.Map) = %d, want %d", got, want)
	}
}

// emptyConfigLoaders returns a *configLoaders with every field set to an
// empty (but non-nil) Configs slice — exercises the
// inclusive-<=Max-with-presence-guard pattern for the case where no
// entity-type data is available.
func emptyConfigLoaders() *configLoaders {
	return &configLoaders{
		inv:     &objtype.InvTypeConfigs{Configs: []*objtype.InvType{}},
		comp:    &objtype.ComponentTypeConfigs{Configs: []*objtype.ComponentType{}},
		varp:    &objtype.VarpTypeConfigs{Configs: []*objtype.VarPlayerType{}},
		varn:    &objtype.VarnTypeConfigs{Configs: []*objtype.VarNpcType{}},
		varsCfg: &objtype.VarsTypeConfigs{Configs: []*objtype.VarSharedType{}},
		param:   &objtype.ParamTypeConfigs{Configs: []*objtype.ParamType{}},
		dbtable: &objtype.DbTableTypeConfigs{Configs: []*objtype.DbTableType{}},
	}
}

// writePackFile writes content to <dir>/pack/<name> creating the pack/
// subdir.
func writePackFile(t *testing.T, dir, name, content string) {
	t.Helper()
	packDir := filepath.Join(dir, "pack")
	if err := os.MkdirAll(packDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(packDir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
```

Note the field name `varsCfg` rather than `vars` — Go reserves no keyword there but a struct field named `vars` would later shadow a `vars` local in `buildSymbolsCore` (per `[[plan_var_name_collision]]`). Same defensive pattern: keep struct field names that won't collide with locals.

- [ ] **Step 2: Run to verify failure**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/... -run TestBuildSymbolsCore -v`

Expected: FAIL with `undefined: configLoaders`, `undefined: buildSymbolsCore`.

- [ ] **Step 3: Implement (append to `symbols.go`)**

```go
// configLoaders bundles the entity-type configurations consumed by the
// enrichment passes. Unexported — internal seam for testability so
// buildSymbolsCore can be exercised with synthetic in-memory configs
// instead of binary .dat/.idx fixtures.
type configLoaders struct {
	inv     *objtype.InvTypeConfigs
	comp    *objtype.ComponentTypeConfigs
	varp    *objtype.VarpTypeConfigs
	varn    *objtype.VarnTypeConfigs
	varsCfg *objtype.VarsTypeConfigs
	param   *objtype.ParamTypeConfigs
	dbtable *objtype.DbTableTypeConfigs
}

// loadConfigs reads the 7 entity-type configurations from dataPackDir.
// Mirrors the cluster of TS .load() calls at Compiler.ts:203, 214, 234,
// 245, 255, 265, 275.
func loadConfigs(dataPackDir string) (*configLoaders, error) {
	inv, err := objtype.LoadInvTypes(dataPackDir)
	if err != nil {
		return nil, fmt.Errorf("LoadInvTypes: %w", err)
	}
	comp, err := objtype.LoadComponentTypes(dataPackDir)
	if err != nil {
		return nil, fmt.Errorf("LoadComponentTypes: %w", err)
	}
	varp, err := objtype.LoadVarpTypes(dataPackDir)
	if err != nil {
		return nil, fmt.Errorf("LoadVarpTypes: %w", err)
	}
	varn, err := objtype.LoadVarnTypes(dataPackDir)
	if err != nil {
		return nil, fmt.Errorf("LoadVarnTypes: %w", err)
	}
	varsCfg, err := objtype.LoadVarsTypes(dataPackDir)
	if err != nil {
		return nil, fmt.Errorf("LoadVarsTypes: %w", err)
	}
	param, err := objtype.LoadParamTypes(dataPackDir)
	if err != nil {
		return nil, fmt.Errorf("LoadParamTypes: %w", err)
	}
	dbtable, err := objtype.LoadDbTableTypes(dataPackDir)
	if err != nil {
		return nil, fmt.Errorf("LoadDbTableTypes: %w", err)
	}
	return &configLoaders{
		inv: inv, comp: comp, varp: varp, varn: varn,
		varsCfg: varsCfg, param: param, dbtable: dbtable,
	}, nil
}

// BuildSymbols ports TS runServerCompiler (Compiler.ts:109-329) up to —
// but not including — the final CompileServerScript({symbols}) call,
// which is deferred to NAI-203+.
//
// srcDir: path containing scripts/ and pack/ subdirs.
// dataPackDir: path containing client/ and server/ subdirs with cache
// .dat/.idx for InvType, Component, VarP, VarN, VarS, Param, DbTableType.
//
// Returns the 32-key symbol-category dict the bytecode compiler's
// typechecker consumes. Categories match TS Compiler.ts:330-365 exactly.
func BuildSymbols(srcDir, dataPackDir string) (map[string]*TypeInfo, error) {
	loaders, err := loadConfigs(dataPackDir)
	if err != nil {
		return nil, fmt.Errorf("BuildSymbols: %w", err)
	}
	return buildSymbolsCore(srcDir, loaders)
}

// buildSymbolsCore is the testable seam under BuildSymbols. Takes
// pre-loaded *configLoaders so unit tests can construct synthetic
// configs without writing binary cache fixtures.
func buildSymbolsCore(srcDir string, loaders *configLoaders) (map[string]*TypeInfo, error) {
	packDir := filepath.Join(srcDir, "pack")
	scriptsDir := filepath.Join(srcDir, "scripts")

	// 1. commandInfo from ScriptOpcodeMap + ScriptOpcodePointers.
	commandInfo := newTypeInfo()
	populateCommandInfo(commandInfo)

	// 2. constantInfo from <srcDir>/scripts/**/*.constant.
	constants, err := loadCompilerConstants(scriptsDir)
	if err != nil {
		return nil, fmt.Errorf("BuildSymbols: constants: %w", err)
	}
	constantInfo := LoadRecords(constants, false)

	// 3. 22 .pack file Loads.
	load := func(packType string) (*TypeInfo, error) {
		p := filepath.Join(packDir, packType+".pack")
		info, err := Load(p)
		if err != nil {
			return nil, fmt.Errorf("Load(%s): %w", packType, err)
		}
		return info, nil
	}
	loadOrFail := func(packType string) *TypeInfo {
		info, lerr := load(packType)
		if lerr != nil && err == nil {
			err = lerr
		}
		return info
	}
	npcInfo := loadOrFail("npc")
	objInfo := loadOrFail("obj")
	invInfo := loadOrFail("inv")
	seqInfo := loadOrFail("seq")
	idkInfo := loadOrFail("idk")
	spotanimInfo := loadOrFail("spotanim")
	locInfo := loadOrFail("loc")
	componentInfo := loadOrFail("interface")
	interfaceInfo := newTypeInfo() // synthesized below
	overlayInfo := newTypeInfo()   // synthesized below
	varpInfo := loadOrFail("varp")
	varnInfo := loadOrFail("varn")
	varsInfo := loadOrFail("vars")
	paramInfo := loadOrFail("param")
	structInfo := loadOrFail("struct")
	enumInfo := loadOrFail("enum")
	huntInfo := loadOrFail("hunt")
	mesanimInfo := loadOrFail("mesanim")
	synthInfo := loadOrFail("synth")
	categoryInfo := loadOrFail("category")
	runescriptInfo := loadOrFail("script") // script.pack → "runescript" symbol key
	dbtableInfo := loadOrFail("dbtable")
	dbcolumnInfo := newTypeInfo() // synthesized below
	dbrowInfo := loadOrFail("dbrow")
	// TS Compiler.ts:204 re-loads inv.pack for writeinv (separate
	// TypeInfo with its own .Protect map enriched below).
	writeinvInfo := loadOrFail("inv")
	if err != nil {
		return nil, fmt.Errorf("BuildSymbols: %w", err)
	}

	// 4. writeinv (InvType.Protect).
	enrichWriteinvInfo(writeinvInfo, loaders.inv)

	// 5. interface / overlayinterface (Component.ComName + Component.Overlay).
	populateInterfaceOverlay(componentInfo, interfaceInfo, overlayInfo, loaders.comp)

	// 6. varp/varn/vars/param vartype + protect enrichments.
	enrichVarpInfo(varpInfo, loaders.varp)
	enrichVarnInfo(varnInfo, loaders.varn)
	enrichVarsInfo(varsInfo, loaders.varsCfg)
	enrichParamInfo(paramInfo, loaders.param)

	// 7. dbcolumn synth.
	populateDbColumns(dbcolumnInfo, loaders.dbtable)

	// 8. stat / npc_stat / npc_mode via LoadMap valueAsKey=true.
	statInfo := LoadMap(objtype.PlayerStatMap, true)
	npcStatInfo := LoadMap(objtype.NpcStatMap, true)
	npcModeInfo := LoadMap(objtype.NPCModeMap, true)

	// 9. fontmetrics / locshape (static LoadArray).
	fontmetricsInfo := LoadArray([]string{"p11", "p12", "b12", "q8"})
	locshapeInfo := LoadArray([]string{
		"wall_straight", "wall_diagonalcorner", "wall_l", "wall_squarecorner",
		"walldecor_straight_nooffset", "walldecor_straight_offset",
		"walldecor_diagonal_offset", "walldecor_diagonal_nooffset",
		"walldecor_diagonal_both", "wall_diagonal",
		"centrepiece_straight", "centrepiece_diagonal",
		"roof_straight", "roof_diagonal_with_roofedge", "roof_diagonal",
		"roof_l_concave", "roof_l_convex", "roof_flat",
		"roofedge_straight", "roofedge_diagonalcorner", "roofedge_l",
		"roofedge_squarecorner",
		"grounddecor",
	})

	// 10. Assemble the 32-key dict, mirroring TS Compiler.ts:330-365 order.
	symbols := map[string]*TypeInfo{
		"command":          commandInfo,
		"constant":         constantInfo,
		"npc":              npcInfo,
		"obj":              objInfo,
		"inv":              invInfo,
		"writeinv":         writeinvInfo,
		"seq":              seqInfo,
		"idk":              idkInfo,
		"spotanim":         spotanimInfo,
		"loc":              locInfo,
		"component":        componentInfo,
		"interface":        interfaceInfo,
		"overlayinterface": overlayInfo,
		"varp":             varpInfo,
		"varn":             varnInfo,
		"vars":             varsInfo,
		"param":            paramInfo,
		"struct":           structInfo,
		"enum":             enumInfo,
		"hunt":             huntInfo,
		"mesanim":          mesanimInfo,
		"synth":            synthInfo,
		"category":         categoryInfo,
		"runescript":       runescriptInfo,
		"dbtable":          dbtableInfo,
		"dbcolumn":         dbcolumnInfo,
		"dbrow":            dbrowInfo,
		"stat":             statInfo,
		"npc_stat":         npcStatInfo,
		"npc_mode":         npcModeInfo,
		"fontmetrics":      fontmetricsInfo,
		"locshape":         locshapeInfo,
	}

	return symbols, nil
}
```

**Verification note**: `objtype.NPCModeMap` is the existing exported name (all-caps `NPC` per the NAI-201 close commit body). If the symbol is `NpcModeMap` instead, adjust the line:

```go
npcModeInfo := LoadMap(objtype.NPCModeMap, true)
```

Run `grep -n "^var .*Map\|^var NpcModeMap\|^var NPCModeMap" pkg/objtype/npcmode.go` before step 4 to confirm the exact name.

- [ ] **Step 4: Run to verify pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/... -run TestBuildSymbolsCore -v`

Expected: All 6 tests PASS. If `npc_mode` test fails with `NPCModeMap` not found, fix the import per the verification note.

- [ ] **Step 5: Run the full package suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/... -v -race`

Expected: Every test green, including NAI-200's existing TypeInfo tests.

- [ ] **Step 6: Commit**

```bash
git add pkg/pack/compiler/symbols.go pkg/pack/compiler/symbols_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(compiler): NAI-202 T9 — BuildSymbols driver + configLoaders seam

Ports TS runServerCompiler (Compiler.ts:109-329) as the 32-key symbol-
table driver. configLoaders is an unexported seam: loadConfigs reads
seven entity-type configs from dataPackDir; buildSymbolsCore takes the
bundle as an arg so unit tests use synthetic in-memory configs instead
of binary .dat fixtures.

BuildSymbols(srcDir, dataPackDir) is the sole public entry point.
Categories match TS Compiler.ts:330-365 exactly: command, constant, npc,
obj, inv, writeinv, seq, idk, spotanim, loc, component, interface,
overlayinterface, varp, varn, vars, param, struct, enum, hunt, mesanim,
synth, category, runescript, dbtable, dbcolumn, dbrow, stat, npc_stat,
npc_mode, fontmetrics, locshape.

CompileServerScript step (TS lines 330-367) deferred to NAI-203+.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 10: Deviation-tag grep pin

**Files:**
- Create: `pkg/pack/compiler/nai202_deviation_pins_test.go`

- [ ] **Step 1: Write the test (`pkg/pack/compiler/nai202_deviation_pins_test.go`)**

```go
package compiler

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestNAI202Deviations_Pinned pins NAI-202's deviation tags — each must
// have at least one in-source reference across the touched packages.
// Mirrors the NAI-201 pin-test shape at pkg/script/nai201_deviation_pins_test.go.
//
// Search roots:
//   - pkg/pack/compiler/   — host of the driver + parser
//   - pkg/script/          — PointerGroupFind hardening
func TestNAI202Deviations_Pinned(t *testing.T) {
	wantTags := []string{
		"NAI-202-D-CORRUPT2-FIELD",
		"NAI-202-D-VARN-LOOP-GUARD",
		"NAI-202-D-CONSTANT-LOOSE-PARSER",
		"NAI-202-D-POINTER-GROUP-FIND-HARDENED",
	}

	roots := []string{".", "../../script"}

	counts := make(map[string]int, len(wantTags))
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if d.IsDir() {
				return nil
			}
			if !strings.HasSuffix(path, ".go") {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			s := string(data)
			for _, tag := range wantTags {
				counts[tag] += strings.Count(s, tag)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walking %s: %v", root, err)
		}
	}

	for _, tag := range wantTags {
		if counts[tag] < 1 {
			t.Errorf("deviation tag %q: 0 references found; want >=1 (search roots: pkg/pack/compiler/, pkg/script/)", tag)
		}
	}
}
```

- [ ] **Step 2: Run the test**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/... -run TestNAI202Deviations_Pinned -v`

Expected: PASS. Every tag from prior tasks (T1, T3, T5, T8) has multiple in-source references by now.

If any tag fails, locate its absence: re-grep `rg "NAI-202-D-<TAG>" pkg/` and confirm at least one source-code reference exists (NOT just in this test file — pin tests should fail when the tag is removed from production code, so the implementer reviews scope before silently retiring).

- [ ] **Step 3: Commit**

```bash
git add pkg/pack/compiler/nai202_deviation_pins_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
test(compiler): NAI-202 T10 — deviation-tag grep pin

Anchors that every NAI-202 deviation tag (CORRUPT2-FIELD,
VARN-LOOP-GUARD, CONSTANT-LOOSE-PARSER, POINTER-GROUP-FIND-HARDENED) has
at least one in-source reference across pkg/pack/compiler/ and
pkg/script/. Following [[retire_deviation_grep_all_comments]]: a future
retire-this-deviation refactor that misses an in-source reference will
fire this test.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 11: Full-suite verification + close commit

**Files:** none

- [ ] **Step 1: Run the full test suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... -race`

Expected: every test green. No regressions in modules/, cmd/, or any other package. If a regression appears, investigate per `[[verify_implementer_claims]]` — confirm via fresh independent run before claiming "pre-existing failure".

- [ ] **Step 2: Run `go vet` and `gofmt -d`**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./pkg/pack/compiler/... ./pkg/script/...`

Expected: no output.

Run: `gofmt -d pkg/pack/compiler/ pkg/script/opcode_pointers.go pkg/script/opcode_map_test.go`

Expected: no output (no formatting drift).

- [ ] **Step 3: Confirm deviation tags reach all expected sites**

Run: `grep -rn "NAI-202-D-" pkg/pack/compiler/ pkg/script/ | wc -l`

Expected: >= 8 references (each of the four tags should appear in at least one production-code location AND at least one test or pin-test reference).

Run: `grep -rn "NAI-202-D-" pkg/pack/compiler/ pkg/script/`

Inspect the output. For each tag, confirm:
- `CORRUPT2-FIELD`: appears in `symbols.go` (production) + `symbols_test.go` (pin) + `nai202_deviation_pins_test.go` (grep pin).
- `VARN-LOOP-GUARD`: appears in `symbols.go` (production) + `symbols_test.go` (pin) + `nai202_deviation_pins_test.go` (grep pin).
- `CONSTANT-LOOSE-PARSER`: appears in `symbols.go` (production) + `symbols_test.go` (pin) + `nai202_deviation_pins_test.go` (grep pin).
- `POINTER-GROUP-FIND-HARDENED`: appears in `opcode_pointers.go` (production) + `opcode_pointers_test.go` (pin) + `nai202_deviation_pins_test.go` (grep pin).

If any tag is missing from production code, that's a coverage gap — the deviation should be visible at the divergence site, not only in tests.

- [ ] **Step 4: Compose the close commit**

```bash
git commit --no-gpg-sign --allow-empty -m "$(cat <<'EOF'
chore(close): NAI-202 — runServerCompiler driver port (slice 2 of 3)

Lands pkg/pack/compiler.BuildSymbols(srcDir, dataPackDir) — the
32-key symbol-table driver ported from TS runServerCompiler
(Compiler.ts:109-329). Stops short of the CompileServerScript call
(deferred to NAI-203+ multi-sub-spec arc covering lexer/parser/
typechecker/bytecode-emitter for the external @lostcityrs/runescript
package).

Slice scope:
  - 22 .pack file Load calls (NAI-200 surface).
  - 7 entity-type loader enrichments: Inv.Protect → writeinv;
    Component.ComName/Overlay → interface/overlayinterface;
    VarP.Type/Protect, VarN.Type, VarS.Type, Param.GetType() → vartype maps;
    DbTableType → dbcolumn with bitfield-encoded ids.
  - 3 LoadMap (PlayerStatMap/NpcStatMap/NpcModeMap valueAsKey=true).
  - 2 LoadArray (fontmetrics 4-entry, locshape 23-entry).
  - 1 inline TS-faithful loose .constant parser
    (loadCompilerConstants — distinct from pkg/pack.LoadConstants).
  - 1 commandInfo build from ScriptOpcodeMap + ScriptOpcodePointers
    (ascending-by-opcode iteration via sort.Slice).
  - configLoaders unexported seam → unit tests use synthetic in-memory
    *FooTypeConfigs instead of binary .dat fixtures.

NAI-201 carryforward closes:
  - PointerGroupFind hardened: unexported [5]string + PointerGroupFind()
    accessor returning fresh copy.
  - TestScriptOpcodeMap_ReverseCoverage: every Op* constant either in
    ScriptOpcodeMap or on excludedOpcodes allowlist with rationale.

Deviations:
  - NAI-202-D-CORRUPT2-FIELD — TS Compiler.ts:147 typo fixed
    (corrupt2 string now writes to .Corrupt2, not .Corrupt).
  - NAI-202-D-VARN-LOOP-GUARD — TS Compiler.ts:247 typo fixed
    (varn loop guard reads varnInfo.Map, not varpInfo.Map).
  - NAI-202-D-CONSTANT-LOOSE-PARSER — second .constant parser
    mirroring TS Compiler.ts:152-173 (last-writer-wins on dups;
    surrounding-quote strip on values; drops parts past second '=').
  - NAI-202-D-POINTER-GROUP-FIND-HARDENED — exported var → unexported
    array + accessor.

Pin tests anchor each deviation; grep-pin
(nai202_deviation_pins_test.go) catches retire-without-grep drift.

Arc next step: NAI-203+ ports @lostcityrs/runescript (lexer →
parser → typechecker → bytecode emitter → CompileServerScript driver
that consumes BuildSymbols' output and writes script.dat/script.idx).

Spec: docs/superpowers/specs/2026-05-15-nai-202-runserver-compiler-driver-design.md
Plan: docs/superpowers/plans/2026-05-15-nai-202-runserver-compiler-driver.md

Closes memory: scope_gate_prerequisite_chain plan_var_name_collision

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

- [ ] **Step 5: Confirm final state**

Run: `git log --oneline -15`

Expected: ~12 NAI-202 commits visible (T1..T10 implementer commits + T11 close + the earlier spec/plan commits).

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... -race -count=1`

Expected: all green.

NAI-202 closes.

---

## Self-review

**Spec coverage check** (spec §-by-§ vs plan tasks):

- §2 In: `symbols.go` (T3-T9), `symbols_test.go` (T3-T9), `nai202_deviation_pins_test.go` (T10), `opcode_pointers.go` (T1), `opcode_pointers_test.go` (T1), `opcode_map_test.go` (T2). ✓
- §5.2 BuildSymbols orchestration: T9. ✓
- §5.3 commandInfo build: T5. ✓
- §5.4 constants parser: T3. ✓
- §5.5 enrichment loops: T6 (interface/overlay), T8 (writeinv/varp/varn/vars/param). ✓
- §5.6 dbcolumn synth: T7. ✓
- §5.7 meta + static-array symbols: T9 (covered inside buildSymbolsCore). ✓
- §5.8 final map assembly: T9. ✓
- §5.9 PointerGroupFind hardening: T1. ✓
- §5.10 reverse-coverage test: T2. ✓
- §7.1 end-to-end smoke: T9 (`TestBuildSymbolsCore_AllCategoriesPresent` + sibling integration tests). Note: cache-config disk fixtures replaced with the configLoaders seam approach per spec §5.2 ("internal seam for testability"). No binary fixtures required.
- §7.2 populateCommandInfo unit: T5. ✓
- §7.3 constant parser tests: T3 (8 cases). ✓
- §7.4 interface/overlay synth: T6 (5 cases). ✓
- §7.5 dbcolumn synth: T7 (5 cases). ✓
- §7.6 enrichment cover-all: T8 (single shared test exercises pattern; per-enricher tests also present). ✓
- §7.7 deviation pin tests: T1 (POINTER-GROUP-FIND-HARDENED), T3 (CONSTANT-LOOSE-PARSER via _LastWriterWins), T5 (CORRUPT2-FIELD), T8 (VARN-LOOP-GUARD). Each test name contains its tag string. ✓
- §7.8 reverse-coverage: T2. ✓
- §7.9 existing-test adjustments: T1 step 5. ✓
- §10 deviations enumerated: all four tracked in T1, T3, T5, T8 + grep pin in T10. ✓
- §13 acceptance criteria: T11 verification steps. ✓

No spec gaps.

**Placeholder scan**: no TBD/TODO/FIXME in plan content. Step 5 of T2 ("populate excludedOpcodes if FAIL") is conditional with explicit guidance, not a placeholder.

**Type/name consistency**:
- `configLoaders` field `varsCfg` (not `vars`) — used consistently in T9 test helper `emptyConfigLoaders` and `loadConfigs` and `buildSymbolsCore`. ✓
- `PointerGroupFind` (T1) referenced as function (with `()`) everywhere post-T1. ✓
- `populateCommandInfo` / `populateCommandInfoFrom` — both called by T5 test, public/seam pattern. ✓
- `enrichWriteinvInfo` / `enrichVarpInfo` / `enrichVarnInfo` / `enrichVarsInfo` / `enrichParamInfo` — same naming convention; referenced in T9. ✓
- `populateInterfaceOverlay` (T6), `populateDbColumns` (T7) — referenced in T9. ✓
- `loadCompilerConstants` (T3), `scriptVarTypeName` (T4), `buildSymbolsCore` (T9), `BuildSymbols` (T9), `loadConfigs` (T9) — consistent across tasks.

**Runnable fixture check** (per `[[plan_runnable_test_fixtures]]`):
- `writeConstantFile` (T3) and `writePackFile` (T9) helpers create their target subdirs via `MkdirAll` before `WriteFile` — won't fail on missing parent.
- `emptyConfigLoaders` (T9) returns non-nil `*FooTypeConfigs` with empty (non-nil) `Configs` slices — the inclusive-`<=Max` loops in T8 enrichers iterate `0` times for a freshly-allocated `*TypeInfo` (Max=-1), so the slice-bounds guard `id < len(configs.Configs)` is never exercised. But when `Map[id]` is non-empty (T9 integration tests seed via `.pack` files), the enricher iterates and the slice-bounds guard kicks in.
- T7's `TestPopulateDbColumns_BitfieldEncoding` constructs `Configs` with `nil, nil, {tbl2}` — iteration via `for _, table := range tables.Configs` honours the nil-check on the first two entries. ✓

No runnable-fixture failures.

**Helper-coverage cross-check** (per `[[plan_helper_coverage]]`):
- `enrichXxxInfo` helpers share the same loop+guard shape across all 5 sites. The `TestEnrich_SkipsIdsAbsentFromMap` test only exercises `enrichWriteinvInfo` to demonstrate the pattern; the others rely on the implementation parallel for correctness. Acceptable: implementation is mechanical, and the integration test in T9 exercises all five via real .pack files.
