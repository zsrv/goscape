# NAI-200 — `CompilerTypeInfo` foundation — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port `CompilerTypeInfo` from TS `tools/pack/Compiler.ts:21-107` to goscape as a new sub-package `pkg/pack/compiler/`. Lands the symbol-table data type that NAI-201's `runServerCompiler` populator will write into.

**Architecture:** One new sub-package `pkg/pack/compiler/` with three files: `typeinfo.go` (struct + 5 constructors + `Add` method), `typeinfo_test.go` (per-loader unit tests), and `nai200_deviation_pins_test.go` (`NAI-200-D-DUAL-MAP` pin). No production wiring; no consumers outside the sub-package. The TS single `map: Record<string, string>` field is split into `Map map[int]string` + `NameMap map[string]string` (deviation `NAI-200-D-DUAL-MAP`); auxiliary maps (`VarType`, `Protect`, `Require`, `Set`, `Conditional`, `Corrupt`, `2`-suffix siblings) are exported but unwritten by NAI-200 — NAI-201 fills them.

**Tech Stack:** Go 1.26+. Stdlib only (`os`, `bufio`, `strings`, `strconv`, `path/filepath`, `testing`).

**Spec:** `docs/superpowers/specs/2026-05-14-nai-200-compilertypeinfo.md` (commit `604b3a7`).
**HEAD at plan-write:** `604b3a7`.

---

## Conventions used throughout this plan

- **All `go` commands prefixed with `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache`** per global CLAUDE.md.
- **All commits use `git commit --no-gpg-sign`** per global CLAUDE.md.
- **Test style** matches existing `pkg/pack/*_test.go`: bare `if err != nil { t.Fatal(err) }`, table-driven where convenient, `t.TempDir()` for fixture roots, `t.Fatalf("got %q, want %q", got, want)` for value diffs.
- **Modern Go** (per `[[use-modern-go]]`): `for i, s := range input` over indexed loops; `min`/`max` builtins where applicable; range-over-int (`for i := range n`) for fixed counts. No deprecated `ioutil.*` — use `os.*`.
- **Package** is `package compiler` (under `pkg/pack/compiler/`); import path `github.com/zsrv/goscape/pkg/pack/compiler`.

---

## Pre-flight verification (controller, before dispatching tasks)

Verified at plan-write against HEAD `604b3a7`:

| Premise | Verification |
|---|---|
| Module path is `github.com/zsrv/goscape` | ✅ `head -1 go.mod` |
| `pkg/pack/compiler/` does not exist | ✅ `test ! -e pkg/pack/compiler` |
| No collision with existing identifier `compiler` | ✅ `grep -rn "package compiler" pkg/` returns no hits |
| TS reference Compiler.ts:21-107 exists at canonical path | ✅ spec §3 |
| `scanPkgPack` test helper (in `pkg/pack/nai196_deviation_pins_test.go`) walks `..` rooted at `pkg/pack/` parent — the new sub-package cannot reuse it; needs its own `scanCompilerPkg(t)` walking the test binary's own directory | ✅ noted in T6 |

---

## File layout (created in this plan)

- Create: `pkg/pack/compiler/typeinfo.go`
- Create: `pkg/pack/compiler/typeinfo_test.go`
- Create: `pkg/pack/compiler/nai200_deviation_pins_test.go`

No modifications to existing files. NAI-200 ships a leaf sub-package.

---

## Task 1: Bootstrap `pkg/pack/compiler/` with `TypeInfo` struct + `Add` method

**Files:**
- Create: `pkg/pack/compiler/typeinfo.go`
- Create: `pkg/pack/compiler/typeinfo_test.go`

**TS source:** `LostCityRS/Engine-TS/tools/pack/Compiler.ts:21-36` (struct fields), `100-106` (`add` method).

### Step 1.1 — Write the failing tests for `Add` and `newTypeInfo`

Create `pkg/pack/compiler/typeinfo_test.go` with two TDD-friendly tests covering spec §7.11 (`Add` updateMax=false) and §7.12 (`Add` Max non-monotonic).

```go
package compiler

import (
	"testing"
)

// TestNewTypeInfo_ZeroValues pins spec §5.8: constructors return *TypeInfo
// with Max=-1 and all maps non-nil so callers can write immediately.
func TestNewTypeInfo_ZeroValues(t *testing.T) {
	p := newTypeInfo()
	if p.Max != -1 {
		t.Fatalf("Max: got %d, want -1", p.Max)
	}
	if p.Map == nil {
		t.Fatal("Map: got nil, want non-nil empty map")
	}
	if len(p.Map) != 0 {
		t.Fatalf("Map: got len %d, want 0", len(p.Map))
	}
	if p.NameMap == nil {
		t.Fatal("NameMap: got nil, want non-nil empty map")
	}
	if p.VarType == nil || p.Protect == nil || p.Require == nil || p.Require2 == nil ||
		p.Conditional == nil || p.Set == nil || p.Set2 == nil ||
		p.Corrupt == nil || p.Corrupt2 == nil {
		t.Fatal("auxiliary maps must be non-nil so NAI-201 populator can write without re-init")
	}
}

// TestAdd_UpdateMaxFalse pins spec §7.11: updateMax=false skips Max bump
// even when id > Max.
func TestAdd_UpdateMaxFalse(t *testing.T) {
	p := newTypeInfo()
	p.Add(0, "a", true)
	p.Add(5, "b", false)
	p.Add(2, "c", true)

	if p.Max != 3 {
		t.Fatalf("Max: got %d, want 3 (-1→1 via id=0; skip via updateMax=false; 1→3 via id=2)", p.Max)
	}
	if p.Map[0] != "a" || p.Map[5] != "b" || p.Map[2] != "c" {
		t.Fatalf("Map: got %v, want {0:a, 5:b, 2:c}", p.Map)
	}
}

// TestAdd_MaxMonotonic pins spec §7.12: Max is monotonic non-decreasing —
// a smaller id following a larger id does NOT shrink Max.
func TestAdd_MaxMonotonic(t *testing.T) {
	p := newTypeInfo()
	p.Add(0, "a", true)
	p.Add(5, "b", true)
	p.Add(2, "c", true)

	if p.Max != 6 {
		t.Fatalf("Max: got %d, want 6 (id=5 bumps to 6; id=2 does NOT re-bump since 6<2 is false)", p.Max)
	}
}
```

- [ ] **Step 1.2 — Run tests to verify they fail**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/...
```

Expected: build error (no `typeinfo.go` yet — `undefined: newTypeInfo` / `undefined: TypeInfo`).

### Step 1.3 — Write `typeinfo.go` with struct + `newTypeInfo` + `Add`

Create `pkg/pack/compiler/typeinfo.go`:

```go
// Package compiler ports the symbol-table data type that the bytecode
// compiler consumes. TS source: tools/pack/Compiler.ts:21-107
// (CompilerTypeInfo class). NAI-201 will port the runServerCompiler
// driver (TS Compiler.ts:109-367) that populates one TypeInfo per symbol
// category and hands them to the lexer/parser/typechecker (NAI-202+
// arc — the external @lostcityrs/runescript package).
package compiler

// TypeInfo holds the name/id/type-info maps for ONE symbol category
// (commands, varp, obj, …) consumed by the bytecode compiler.
//
// Per TS Compiler.ts:21-36 (class CompilerTypeInfo):
//
//	max: number = -1;
//	map: Record<string, string> = {};
//	vartype: Record<string, string> = {};
//	protect: Record<string, boolean> = {};
//	require, require2, set, set2, corrupt, corrupt2: Record<string, string>;
//	conditional: Record<string, boolean>;
//
// NAI-200-D-DUAL-MAP: TS `map: Record<string, string>` accepts BOTH
// numeric-IDs-coerced-to-strings (from add(id, name)) AND genuine
// string keys (from loadRecords/loadMap). Go cannot mix int and string
// keys in one map. The single TS field is split here into:
//
//   - Map     map[int]string    — int-keyed; Load + LoadArray write.
//   - NameMap map[string]string — string-keyed; LoadRecords + LoadMap write.
//
// Auxiliary maps (VarType, Protect, Require, …) are all map[int]<T>
// because every TS write to them uses a numeric ID — falls out of the
// dual-map split.
//
// NAI-200 ships zero writers for the auxiliary maps. NAI-201 will be
// the first slice that populates them (see spec §9 R2).
type TypeInfo struct {
	Max int // upper-exclusive bound; -1 = empty (TS-faithful)

	Map     map[int]string
	NameMap map[string]string

	VarType     map[int]string
	Protect     map[int]bool
	Require     map[int]string
	Require2    map[int]string
	Conditional map[int]bool
	Set         map[int]string
	Set2        map[int]string
	Corrupt     map[int]string
	Corrupt2    map[int]string
}

// newTypeInfo returns a zero-valued *TypeInfo with all maps initialized
// and Max=-1. All five public constructors call this so callers may
// invoke Add immediately without nil-map panic. Mirrors TS class field
// defaults at Compiler.ts:22-36.
func newTypeInfo() *TypeInfo {
	return &TypeInfo{
		Max:         -1,
		Map:         map[int]string{},
		NameMap:     map[string]string{},
		VarType:     map[int]string{},
		Protect:     map[int]bool{},
		Require:     map[int]string{},
		Require2:    map[int]string{},
		Conditional: map[int]bool{},
		Set:         map[int]string{},
		Set2:        map[int]string{},
		Corrupt:     map[int]string{},
		Corrupt2:    map[int]string{},
	}
}

// Add records id→name in Map. If updateMax is true and id exceeds the
// current Max, Max is set to id+1 (TS-faithful off-by-one: Max is the
// upper-exclusive bound, so iteration uses `for i := 0; i <= Max; i++`
// in callers like runServerCompiler).
//
// TS Compiler.ts:100-106:
//
//	add(id: number, name: string, updateMax: boolean = true) {
//	    this.map[id] = name;
//	    if (updateMax && this.max < id) {
//	        this.max = id + 1;
//	    }
//	}
func (p *TypeInfo) Add(id int, name string, updateMax bool) {
	p.Map[id] = name
	if updateMax && p.Max < id {
		p.Max = id + 1
	}
}
```

- [ ] **Step 1.4 — Run tests to verify they pass**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/...
```

Expected: `ok   github.com/zsrv/goscape/pkg/pack/compiler` with `TestNewTypeInfo_ZeroValues`, `TestAdd_UpdateMaxFalse`, `TestAdd_MaxMonotonic` all PASS.

- [ ] **Step 1.5 — Commit**

```bash
git add pkg/pack/compiler/typeinfo.go pkg/pack/compiler/typeinfo_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(pack): NAI-200 T1 — TypeInfo struct + Add method (compiler sub-package)

Ports tools/pack/Compiler.ts:21-36 (struct fields) and :100-106 (add).
Splits TS single `map: Record<string, string>` into Map (int-keyed) +
NameMap (string-keyed) per NAI-200-D-DUAL-MAP. Auxiliary maps shipped
empty for NAI-201 population.
EOF
)"
```

---

## Task 2: `Load` constructor (from `.pack`-style file)

**Files:**
- Modify: `pkg/pack/compiler/typeinfo.go` (add `Load` function)
- Modify: `pkg/pack/compiler/typeinfo_test.go` (add five `Load` tests)

**TS source:** `LostCityRS/Engine-TS/tools/pack/Compiler.ts:38-60` (`static load(file: string)`).

### Step 2.1 — Write the failing tests

Append to `pkg/pack/compiler/typeinfo_test.go`:

```go
import (
	"os"
	"path/filepath"
	"testing"
)
// (merge imports with the existing import block — keep the file using
// one consolidated import group)

// TestLoad_HappyPath pins spec §7.1: three dense entries land as
// Map[i]=name, Max=3.
func TestLoad_HappyPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "category.pack")
	if err := os.WriteFile(path, []byte("0=alpha\n1=bravo\n2=charlie\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	p, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	if p.Max != 3 {
		t.Fatalf("Max: got %d, want 3", p.Max)
	}
	if p.Map[0] != "alpha" || p.Map[1] != "bravo" || p.Map[2] != "charlie" {
		t.Fatalf("Map: got %v, want {0:alpha,1:bravo,2:charlie}", p.Map)
	}
	if len(p.NameMap) != 0 {
		t.Fatalf("NameMap: got %v, want empty (Load writes only Map)", p.NameMap)
	}
}

// TestLoad_MissingFile pins spec §7.2: nonexistent path returns
// empty *TypeInfo, nil error (TS !fs.existsSync early-return).
func TestLoad_MissingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "does_not_exist.pack")

	p, err := Load(path)
	if err != nil {
		t.Fatalf("err: got %v, want nil", err)
	}
	if p == nil {
		t.Fatal("p: got nil, want empty *TypeInfo")
	}
	if p.Max != -1 {
		t.Fatalf("Max: got %d, want -1", p.Max)
	}
	if len(p.Map) != 0 {
		t.Fatalf("Map: got %v, want empty", p.Map)
	}
}

// TestLoad_FilterCases pins spec §7.3: blank lines, no-=, name=="null",
// name=="null:null" are skipped; name=="" (line "1=") is RETAINED.
func TestLoad_FilterCases(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mixed.pack")
	content := "0=valid_alpha\n\n1=\n2=null\n3=null:null\nnot_an_equals_line\n4=valid_beta\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	p, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	// Expected: id 0 → "valid_alpha", id 1 → "" (empty name retained per TS),
	// ids 2 and 3 skipped (null/null:null sentinels), id 4 → "valid_beta".
	// "not_an_equals_line" skipped (no '=').
	wantMap := map[int]string{0: "valid_alpha", 1: "", 4: "valid_beta"}
	if len(p.Map) != len(wantMap) {
		t.Fatalf("Map len: got %d (%v), want %d (%v)", len(p.Map), p.Map, len(wantMap), wantMap)
	}
	for k, v := range wantMap {
		if p.Map[k] != v {
			t.Fatalf("Map[%d]: got %q, want %q", k, p.Map[k], v)
		}
	}
	if p.Max != 5 {
		t.Fatalf("Max: got %d, want 5 (last Add(4, _, true) bumps 1→5)", p.Max)
	}
}

// TestLoad_IOError pins spec §7.4: passing a directory path triggers a
// genuine IO error (not IsNotExist) and returns (nil, err).
func TestLoad_IOError(t *testing.T) {
	dir := t.TempDir() // dir IS a directory; os.ReadFile on it returns a non-nil error

	p, err := Load(dir)
	if err == nil {
		t.Fatal("err: got nil, want non-nil (read of directory)")
	}
	if p != nil {
		t.Fatalf("p: got %+v, want nil on IO error", p)
	}
}

// TestLoad_DuplicateID pins spec §7.5: TS-faithful no-validation —
// later `0=second` silently overwrites earlier `0=first`. Max bumps
// only once (on the first Add(0,_,true): -1→1) because the second
// Add(0,_,true) sees Max=1 and 1<0 is false.
func TestLoad_DuplicateID(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dup.pack")
	if err := os.WriteFile(path, []byte("0=first\n0=second\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	p, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	if p.Map[0] != "second" {
		t.Fatalf("Map[0]: got %q, want %q (second write overwrites silently)", p.Map[0], "second")
	}
	if p.Max != 1 {
		t.Fatalf("Max: got %d, want 1", p.Max)
	}
}
```

Note on imports: the existing `typeinfo_test.go` (from T1) uses only `"testing"`. T2 adds `"os"` and `"path/filepath"`. Consolidate into one grouped import block at the top of the file.

- [ ] **Step 2.2 — Run tests to verify they fail**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/...
```

Expected: build error `undefined: Load`.

### Step 2.3 — Implement `Load`

Append to `pkg/pack/compiler/typeinfo.go`:

```go
import (
	"errors"
	"os"
	"strconv"
	"strings"
)
// (merge with existing imports — none yet in typeinfo.go from T1)

// Load reads a .pack-style file (one `id=name` entry per line) and
// returns a *TypeInfo populated via Add. Mirrors TS Compiler.ts:38-60
// (CompilerTypeInfo.load).
//
// Skip rules (TS-faithful, in order):
//   - empty line                → skip
//   - line contains no `=`      → skip
//   - name == "null"            → skip
//   - name == "null:null"       → skip
//   - id parse error            → skip (TS parseInt returns NaN;
//                                  see spec §9 R1 for unreachable-in-
//                                  practice rationale. .pack files are
//                                  mechanically generated by
//                                  pkg/pack/PackFile.Save → always
//                                  well-formed.)
//
// Missing file → empty *TypeInfo (Max=-1), nil error. Genuine IO
// errors (read of a directory, permission failure, etc.) → (nil, err).
func Load(path string) (*TypeInfo, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return newTypeInfo(), nil
		}
		return nil, err
	}

	p := newTypeInfo()
	// TS splits on /\r?\n/. strings.Split with "\n" handles \n; we strip
	// trailing \r per-line to match the regex.
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSuffix(raw, "\r")
		if len(line) == 0 {
			continue
		}
		eq := strings.IndexByte(line, '=')
		if eq < 0 {
			continue
		}
		idStr := line[:eq]
		name := line[eq+1:]
		if name == "null" || name == "null:null" {
			continue
		}
		id, parseErr := strconv.Atoi(idStr)
		if parseErr != nil {
			// TS parseInt would produce NaN here; goscape skips silently.
			// Unreachable in practice — .pack files are mechanically
			// generated. See spec §9 R1.
			continue
		}
		p.Add(id, name, true)
	}
	return p, nil
}
```

- [ ] **Step 2.4 — Run tests to verify they pass**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/...
```

Expected: all 5 `Load*` tests + the 3 T1 tests = 8 PASS.

- [ ] **Step 2.5 — Commit**

```bash
git add pkg/pack/compiler/typeinfo.go pkg/pack/compiler/typeinfo_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(pack): NAI-200 T2 — Load constructor (file-based)

Ports tools/pack/Compiler.ts:38-60 (CompilerTypeInfo.load). Skip rules:
empty / no-= / "null" / "null:null" / parse-error. Missing file →
empty TypeInfo, nil err; IO error → nil, err.
EOF
)"
```

---

## Task 3: `LoadArray` constructor (from `[]string`)

**Files:**
- Modify: `pkg/pack/compiler/typeinfo.go`
- Modify: `pkg/pack/compiler/typeinfo_test.go`

**TS source:** `LostCityRS/Engine-TS/tools/pack/Compiler.ts:62-70` (`static loadArray`).

### Step 3.1 — Write the failing tests

Append to `pkg/pack/compiler/typeinfo_test.go`:

```go
// TestLoadArray_HappyPath pins spec §7.6: each index → Add(i,
// lowercase(s), true); Max = len(input).
func TestLoadArray_HappyPath(t *testing.T) {
	p := LoadArray([]string{"Alpha", "BRAVO", "Charlie"})

	if p.Map[0] != "alpha" || p.Map[1] != "bravo" || p.Map[2] != "charlie" {
		t.Fatalf("Map: got %v, want {0:alpha,1:bravo,2:charlie}", p.Map)
	}
	if p.Max != 3 {
		t.Fatalf("Max: got %d, want 3 (len-1 + 1)", p.Max)
	}
}

// TestLoadArray_Empty pins spec §7.7: empty input → no Add calls →
// Max remains -1.
func TestLoadArray_Empty(t *testing.T) {
	p := LoadArray([]string{})

	if p.Max != -1 {
		t.Fatalf("Max: got %d, want -1", p.Max)
	}
	if len(p.Map) != 0 {
		t.Fatalf("Map: got %v, want empty", p.Map)
	}
}
```

- [ ] **Step 3.2 — Run tests to verify they fail**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/...
```

Expected: build error `undefined: LoadArray`.

### Step 3.3 — Implement `LoadArray`

Append to `pkg/pack/compiler/typeinfo.go`:

```go
// LoadArray builds a *TypeInfo by treating each slice index as an ID
// and lowercasing the value. Mirrors TS Compiler.ts:62-70
// (CompilerTypeInfo.loadArray).
//
// Used by runServerCompiler (NAI-201) for static enum-like sources:
// fontmetrics (['p11','p12','b12','q8']) and locshape (23 entries).
func LoadArray(input []string) *TypeInfo {
	p := newTypeInfo()
	for i, s := range input {
		p.Add(i, strings.ToLower(s), true)
	}
	return p
}
```

`strings` is already imported from T2.

- [ ] **Step 3.4 — Run tests to verify they pass**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/...
```

Expected: 10 tests PASS.

- [ ] **Step 3.5 — Commit**

```bash
git add pkg/pack/compiler/typeinfo.go pkg/pack/compiler/typeinfo_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(pack): NAI-200 T3 — LoadArray constructor

Ports tools/pack/Compiler.ts:62-70 (CompilerTypeInfo.loadArray). Each
index → Add(i, lowercase(s), true). Used by NAI-201 for fontmetrics
and locshape static enums.
EOF
)"
```

---

## Task 4: `LoadRecords` constructor (from `map[string]string`)

**Files:**
- Modify: `pkg/pack/compiler/typeinfo.go`
- Modify: `pkg/pack/compiler/typeinfo_test.go`

**TS source:** `LostCityRS/Engine-TS/tools/pack/Compiler.ts:72-84` (`static loadRecords`).

### Step 4.1 — Write the failing tests

Append to `pkg/pack/compiler/typeinfo_test.go`:

```go
// TestLoadRecords_ValueAsKeyFalse pins spec §7.8: with valueAsKey=false,
// NameMap[key] = lowercase(value); key UNCHANGED.
func TestLoadRecords_ValueAsKeyFalse(t *testing.T) {
	p := LoadRecords(map[string]string{"foo": "BAR", "baz": "QUX"}, false)

	if p.NameMap["foo"] != "bar" || p.NameMap["baz"] != "qux" {
		t.Fatalf("NameMap: got %v, want {foo:bar,baz:qux}", p.NameMap)
	}
	if len(p.Map) != 0 {
		t.Fatalf("Map: got %v, want empty (LoadRecords writes only NameMap)", p.Map)
	}
	if p.Max != -1 {
		t.Fatalf("Max: got %d, want -1 (LoadRecords doesn't call Add)", p.Max)
	}
}

// TestLoadRecords_ValueAsKeyTrue pins spec §7.9: with valueAsKey=true,
// NameMap[value] = lowercase(key). The new map KEY (the TS `value`)
// is UNCHANGED — only the new map VALUE (the TS `key`) is lowercased.
// This pins the TS asymmetry at Compiler.ts:77 (`pack.map[value] =
// key.toLowerCase()`).
func TestLoadRecords_ValueAsKeyTrue(t *testing.T) {
	p := LoadRecords(map[string]string{"FOO": "BAR", "BAZ": "QUX"}, true)

	// KEYS of NameMap should be original (uppercase) values "BAR"/"QUX";
	// VALUES should be lowercased original keys "foo"/"baz".
	if p.NameMap["BAR"] != "foo" || p.NameMap["QUX"] != "baz" {
		t.Fatalf("NameMap: got %v, want {BAR:foo,QUX:baz} — key=value-preserved, value=lowercase(key)", p.NameMap)
	}
}
```

- [ ] **Step 4.2 — Run tests to verify they fail**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/...
```

Expected: build error `undefined: LoadRecords`.

### Step 4.3 — Implement `LoadRecords`

Append to `pkg/pack/compiler/typeinfo.go`:

```go
// LoadRecords builds a *TypeInfo from a string-keyed map. The
// valueAsKey flag flips which side of the input becomes the NameMap
// key. Mirrors TS Compiler.ts:72-84 (CompilerTypeInfo.loadRecords).
//
//	false: NameMap[k] = lowercase(v)
//	true:  NameMap[v] = lowercase(k)
//
// Used by runServerCompiler (NAI-201) for the constant table loaded
// from data/src/scripts/**/*.constant files.
func LoadRecords(input map[string]string, valueAsKey bool) *TypeInfo {
	p := newTypeInfo()
	for k, v := range input {
		if valueAsKey {
			p.NameMap[v] = strings.ToLower(k)
		} else {
			p.NameMap[k] = strings.ToLower(v)
		}
	}
	return p
}
```

- [ ] **Step 4.4 — Run tests to verify they pass**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/...
```

Expected: 12 tests PASS.

- [ ] **Step 4.5 — Commit**

```bash
git add pkg/pack/compiler/typeinfo.go pkg/pack/compiler/typeinfo_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(pack): NAI-200 T4 — LoadRecords constructor

Ports tools/pack/Compiler.ts:72-84 (CompilerTypeInfo.loadRecords). Two
flag values cover both directional flips. Pins TS lowercase asymmetry
(value-as-key path preserves new-key case, only lowercases new-value).
EOF
)"
```

---

## Task 5: `LoadMap` constructor (from `map[string]int`)

**Files:**
- Modify: `pkg/pack/compiler/typeinfo.go`
- Modify: `pkg/pack/compiler/typeinfo_test.go`

**TS source:** `LostCityRS/Engine-TS/tools/pack/Compiler.ts:86-98` (`static loadMap`).

### Step 5.1 — Write the failing tests

Append to `pkg/pack/compiler/typeinfo_test.go`:

```go
// TestLoadMap_ValueAsKeyFalse pins spec §7.10: with valueAsKey=false,
// NameMap[lowercase(k)] = Itoa(v).
func TestLoadMap_ValueAsKeyFalse(t *testing.T) {
	p := LoadMap(map[string]int{"FOO": 7, "BAR": 9}, false)

	if p.NameMap["foo"] != "7" || p.NameMap["bar"] != "9" {
		t.Fatalf("NameMap: got %v, want {foo:7,bar:9}", p.NameMap)
	}
}

// TestLoadMap_ValueAsKeyTrue pins spec §7.10: with valueAsKey=true,
// NameMap[Itoa(v)] = lowercase(k). Both sides lowercased on the
// string-key side per TS.
func TestLoadMap_ValueAsKeyTrue(t *testing.T) {
	p := LoadMap(map[string]int{"FOO": 7, "BAR": 9}, true)

	if p.NameMap["7"] != "foo" || p.NameMap["9"] != "bar" {
		t.Fatalf("NameMap: got %v, want {7:foo,9:bar}", p.NameMap)
	}
}
```

- [ ] **Step 5.2 — Run tests to verify they fail**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/...
```

Expected: build error `undefined: LoadMap`.

### Step 5.3 — Implement `LoadMap`

Append to `pkg/pack/compiler/typeinfo.go`:

```go
// LoadMap builds a *TypeInfo from a map[string]int (mirroring TS
// Map<string, number>). The valueAsKey flag flips the direction.
// Mirrors TS Compiler.ts:86-98 (CompilerTypeInfo.loadMap).
//
//	false: NameMap[lowercase(k)] = Itoa(v)
//	true:  NameMap[Itoa(v)]      = lowercase(k)
//
// Iteration-order note: TS Map<string,number> is insertion-ordered;
// Go map[string]int iteration is randomized. Order only matters when
// two distinct keys collide on the same value (valueAsKey=true case).
// In every Compiler.ts call site the input is a static enum
// (PlayerStatMap, NpcStatMap, NpcModeMap) with unique-value-per-key —
// collisions don't occur. See spec §5.6 / §9 R6.
//
// Used by runServerCompiler (NAI-201) for stat / npc_stat / npc_mode.
func LoadMap(input map[string]int, valueAsKey bool) *TypeInfo {
	p := newTypeInfo()
	for k, v := range input {
		if valueAsKey {
			p.NameMap[strconv.Itoa(v)] = strings.ToLower(k)
		} else {
			p.NameMap[strings.ToLower(k)] = strconv.Itoa(v)
		}
	}
	return p
}
```

`strconv` and `strings` already imported from T2/T3.

- [ ] **Step 5.4 — Run tests to verify they pass**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/...
```

Expected: 14 tests PASS.

Also verify race-clean:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./pkg/pack/compiler/...
```

Expected: 14 tests PASS (no data-race detector warnings — the package has no goroutines).

- [ ] **Step 5.5 — Commit**

```bash
git add pkg/pack/compiler/typeinfo.go pkg/pack/compiler/typeinfo_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(pack): NAI-200 T5 — LoadMap constructor

Ports tools/pack/Compiler.ts:86-98 (CompilerTypeInfo.loadMap). Stringifies
int values via strconv.Itoa to mirror TS value.toString(). Two flag
values cover both directional flips.
EOF
)"
```

---

## Task 6: `NAI-200-D-DUAL-MAP` deviation pin test

**Files:**
- Create: `pkg/pack/compiler/nai200_deviation_pins_test.go`

**Source:** spec §7.13 + `[[pin_test_self_trigger_production_doc]]`.

### Step 6.1 — Write the pin test (FAILS first if tag absent)

Create `pkg/pack/compiler/nai200_deviation_pins_test.go`:

```go
package compiler

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// scanCompilerPkg reads every .go file in pkg/pack/compiler/ (the
// current package directory, since tests run with cwd = package dir)
// excluding _test.go files, and returns concatenated content. Used by
// the NAI-200 deviation-tag pin.
//
// Distinct from sibling pkg/pack/scanPkgPack (in nai196_deviation_pins_test.go)
// because that one walks `..` rooted at pkg/pack/'s parent — wrong root
// for a sub-package pin.
func scanCompilerPkg(t *testing.T) string {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read pkg/pack/compiler: %v", err)
	}
	var sb strings.Builder
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(".", name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		sb.Write(data)
		sb.WriteString("\n")
	}
	return sb.String()
}

// TestNAI200_PresencePin_DualMap asserts the NAI-200-D-DUAL-MAP tag
// appears ≥1 times in pkg/pack/compiler/ production code (the
// doc-comment on TypeInfo at typeinfo.go).
//
// The tag records the goscape decision to split TS's
// `map: Record<string, string>` (mixed numeric/string keys) into two
// statically-typed maps: `Map map[int]string` (from Add) and
// `NameMap map[string]string` (from LoadRecords/LoadMap).
//
// Per [[pin_test_self_trigger_production_doc]], this pin matches the
// tag identifier ONLY — not paraphrases like "dual map" — to avoid
// self-triggering against adjacent prose.
func TestNAI200_PresencePin_DualMap(t *testing.T) {
	src := scanCompilerPkg(t)
	const tag = "NAI-200-D-DUAL-MAP"
	count := strings.Count(src, tag)
	if count < 1 {
		t.Fatalf("%s should appear ≥1 times in pkg/pack/compiler/ production code (TypeInfo doc-comment); got %d", tag, count)
	}
}
```

- [ ] **Step 6.2 — Run test to verify it PASSES on first run**

The tag `NAI-200-D-DUAL-MAP` was already written into `typeinfo.go` in T1 step 1.3 (in the doc-comment on `TypeInfo`). The pin test should pass immediately.

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/... -run NAI200 -v
```

Expected: `TestNAI200_PresencePin_DualMap` PASS.

If it FAILS (count == 0), the implementer must add the tag to the `TypeInfo` doc-comment in `typeinfo.go` — but T1 already placed it. Re-grep:

```bash
grep -c "NAI-200-D-DUAL-MAP" pkg/pack/compiler/typeinfo.go
```

Expected: ≥1.

- [ ] **Step 6.3 — Confirm self-trigger guard**

Verify the pin test file itself does NOT contain a bare literal `NAI-200-D-DUAL-MAP` outside the `const tag = "..."` declaration that would trigger `scanCompilerPkg`'s grep — except that `scanCompilerPkg` EXCLUDES `_test.go`, so the pin file is invisible to itself. No action required.

- [ ] **Step 6.4 — Run full package tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/...
```

Expected: 15 tests PASS (14 from T1-T5 + 1 from T6).

- [ ] **Step 6.5 — Commit**

```bash
git add pkg/pack/compiler/nai200_deviation_pins_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
test(pack): NAI-200 T6 — NAI-200-D-DUAL-MAP deviation pin

Adds presence pin matching the tag identifier in pkg/pack/compiler/
production code. Introduces scanCompilerPkg helper (sub-package
equivalent of pkg/pack/scanPkgPack).
EOF
)"
```

---

## Task 7: Whole-tree regression check + NAI-200 close commit

**Files:** none (verification + close commit only).

### Step 7.1 — Run whole-tree tests

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
```

Expected: all packages PASS. NAI-200's sub-package has no consumers, so no regressions are possible — this is the verification-before-completion sanity check per `[[verification_before_completion]]`.

Also `-race`:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...
```

Expected: same PASS, no race warnings.

### Step 7.2 — Sanity-check the deviation tag count

```bash
git grep -c "NAI-200-D-DUAL-MAP" -- 'pkg/pack/compiler/*.go' ':(exclude)pkg/pack/compiler/*_test.go'
```

Expected: ≥1 (specifically: in `typeinfo.go`).

### Step 7.3 — File the close commit with memory trailer

Per `[[close_commit_memory_trailer]]`:

```bash
git commit --allow-empty --no-gpg-sign -m "$(cat <<'EOF'
chore(close): NAI-200 — CompilerTypeInfo foundation (bytecode arc opener)

Per spec docs/superpowers/specs/2026-05-14-nai-200-compilertypeinfo.md
(604b3a7):

- TypeInfo struct + Add method (Compiler.ts:21-36, :100-106)
- 5 constructors: Load, LoadArray, LoadRecords, LoadMap (Compiler.ts:38-98)
- NAI-200-D-DUAL-MAP: TS Record<string,string> mixed-key split into
  Map map[int]string + NameMap map[string]string

NAI-200 lands the symbol-table data type with zero consumers. NAI-201
will port the runServerCompiler driver (Compiler.ts:109-367) that
populates one TypeInfo per symbol category, with prerequisites:
NpcModeMap, NpcStatMap, ScriptOpcodeMap, ScriptOpcodePointers.

NAI-202+ then opens the external @lostcityrs/runescript lexer/parser/
typechecker port arc.

Closes memory:
EOF
)"
```

(Note: no specific memory entries to retire at NAI-200 close — the spec did not invalidate any prior memory. The `Closes memory:` trailer is empty but present, preserving provenance grep-discoverability per the memory convention.)

---

## Plan summary

| Task | Files | Tests added | Cumulative tests |
|------|-------|-------------|------------------|
| T1 | `typeinfo.go`, `typeinfo_test.go` | 3 (`NewTypeInfo_ZeroValues`, `Add_UpdateMaxFalse`, `Add_MaxMonotonic`) | 3 |
| T2 | + `Load` | 5 (`HappyPath`, `MissingFile`, `FilterCases`, `IOError`, `DuplicateID`) | 8 |
| T3 | + `LoadArray` | 2 (`HappyPath`, `Empty`) | 10 |
| T4 | + `LoadRecords` | 2 (`ValueAsKeyFalse`, `ValueAsKeyTrue`) | 12 |
| T5 | + `LoadMap` | 2 (`ValueAsKeyFalse`, `ValueAsKeyTrue`) | 14 |
| T6 | `nai200_deviation_pins_test.go` | 1 (`PresencePin_DualMap`) | 15 |
| T7 | (verification + close commit) | — | 15 |

Total commits: 7 (six feat/test + one close).
Total LOC (estimated): ~250 production, ~250 test.

---

## Spec → plan coverage check

| Spec section | Plan task |
|---|---|
| §1 Goal — `pkg/pack/compiler/` exists with type + 5 constructors | T1–T5 |
| §2 In — sub-package + 5 loaders + Add + deviation pin | T1–T6 |
| §5.2 TypeInfo struct | T1 |
| §5.3 Load | T2 |
| §5.4 LoadArray | T3 |
| §5.5 LoadRecords | T4 |
| §5.6 LoadMap | T5 |
| §5.7 Add | T1 |
| §5.8 Zero-value init via newTypeInfo | T1 |
| §6 Error handling | T2 (`Load` IO error + missing-file paths) |
| §7.1–7.5 Load tests | T2 |
| §7.6–7.7 LoadArray tests | T3 |
| §7.8–7.9 LoadRecords tests | T4 |
| §7.10 LoadMap tests | T5 |
| §7.11–7.12 Add tests | T1 |
| §7.13 Deviation pin | T6 |
| §10 NAI-200-D-DUAL-MAP deviation | T1 (doc-comment) + T6 (pin) |
| §13 Acceptance criteria | T7 (whole-tree test + deviation grep) |

No spec sections left uncovered.
