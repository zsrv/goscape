# NAI-212 — Server-side `PackAll` + `runServerCompiler` glue — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Wire goscape's existing `BuildSymbols` (NAI-202) into `runescript.Compile` (NAI-210) via a type bridge, then add `pack.PackAll` server-side orchestration on top.

**Architecture:** Three concentric layers, each independently testable. `compiler.ToCompilerTypeInfo` (pure conversion). `compiler.RunServerCompiler` (chains BuildSymbols → bridge → Compile). `pack.PackAll` (ClearFsCache → PackConfigs → RunServerCompiler).

**Tech Stack:** Go 1.26+ per `[[go_version]]`. All `go` invocations prefix with `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache` per CLAUDE.md.

**Spec:** `docs/superpowers/specs/2026-05-16-nai-212-server-pack-all-design.md`.

**TS source:** `tools/pack/Compiler.ts:329-365` + `tools/pack/PackAll.ts:17-52` in `/home/owner/Code/github.com/LostCityRS/Engine-TS`.

---

## File structure (created in this plan)

| Path | Responsibility | Created in |
|---|---|---|
| `pkg/pack/compiler/bridge.go` | `ToCompilerTypeInfo(src) *runescript.CompilerTypeInfo` | T1 |
| `pkg/pack/compiler/bridge_test.go` | 5 bridge unit tests | T1 |
| `pkg/pack/compiler/run_server_compiler.go` | `RunServerCompiler(srcDir, outDir, dataPackDir)` | T2 |
| `pkg/pack/compiler/run_server_compiler_test.go` | 3 driver tests (uses test seam) | T2 |
| `pkg/pack/pack_all.go` | `PackAll(srcDir, outDir, dataPackDir)` | T3 |
| `pkg/pack/pack_all_test.go` | 2 integration smoke tests | T3 |
| `pkg/pack/nai212_deviation_pins_test.go` | 3 deviation pins | T4 |

No existing files are modified.

---

## Task 1: Type bridge `compiler.ToCompilerTypeInfo`

**Goal:** Convert `*compiler.TypeInfo` (NAI-200 dual-map shape) → `*runescript.CompilerTypeInfo` (NAI-210 single-map shape). Pure conversion, no IO.

**Files:**
- Create: `pkg/pack/compiler/bridge.go`
- Create: `pkg/pack/compiler/bridge_test.go`

### Step 1.1: Write the 5 failing tests

- [ ] Create `pkg/pack/compiler/bridge_test.go` with the following content:

```go
package compiler

import (
	"reflect"
	"testing"

	"github.com/zsrv/goscape/pkg/pack/compiler/runescript"
)

// TestToCompilerTypeInfo_Empty pins NAI-212 spec §4 edge case: empty
// source (Max=-1, all maps empty) → dst with all maps initialized,
// Max=-1.
func TestToCompilerTypeInfo_Empty(t *testing.T) {
	src := newTypeInfo()
	dst := ToCompilerTypeInfo(src)
	if dst == nil {
		t.Fatal("dst is nil, want non-nil")
	}
	if dst.Max != -1 {
		t.Errorf("Max=%d, want -1", dst.Max)
	}
	checkMap(t, "Map", dst.Map, map[string]string{})
	checkBoolMap(t, "Protect", dst.Protect, map[string]bool{})
	checkBoolMap(t, "Conditional", dst.Conditional, map[string]bool{})
}

// TestToCompilerTypeInfo_NumericIDs pins NAI-212 spec §4 rule 2: int
// keys are stringified into dst.Map.
func TestToCompilerTypeInfo_NumericIDs(t *testing.T) {
	src := newTypeInfo()
	src.Add(0, "alpha", true)
	src.Add(42, "beta", true)
	dst := ToCompilerTypeInfo(src)
	if dst.Max != 43 { // Add updates Max to id+1
		t.Errorf("Max=%d, want 43", dst.Max)
	}
	checkMap(t, "Map", dst.Map, map[string]string{"0": "alpha", "42": "beta"})
}

// TestToCompilerTypeInfo_NameMap pins NAI-212 spec §4 rule 3: NameMap
// entries (constantInfo shape) merge into dst.Map.
func TestToCompilerTypeInfo_NameMap(t *testing.T) {
	src := LoadRecords(map[string]string{"FOO": "100", "BAR": "hello"}, false)
	dst := ToCompilerTypeInfo(src)
	// LoadRecords lowercases values; keys preserved as-is.
	checkMap(t, "Map", dst.Map, map[string]string{"FOO": "100", "BAR": "hello"})
}

// TestToCompilerTypeInfo_AuxiliaryMaps pins NAI-212 spec §4 table: all
// auxiliary fields (VarType/Protect/Require/Set/Corrupt/Conditional)
// carry over with stringified keys.
func TestToCompilerTypeInfo_AuxiliaryMaps(t *testing.T) {
	src := newTypeInfo()
	src.Add(5, "v5", true)
	src.VarType[5] = "int"
	src.Protect[5] = true
	src.Add(7, "v7", true)
	src.Require[7] = "active_player"
	src.Require2[7] = "active_npc"
	src.Set[7] = "x"
	src.Set2[7] = "y"
	src.Corrupt[7] = "c1"
	src.Corrupt2[7] = "c2"
	src.Conditional[7] = true
	dst := ToCompilerTypeInfo(src)

	checkMap(t, "Vartype", dst.Vartype, map[string]string{"5": "int"})
	checkBoolMap(t, "Protect", dst.Protect, map[string]bool{"5": true})
	checkMap(t, "Require", dst.Require, map[string]string{"7": "active_player"})
	checkMap(t, "Require2", dst.Require2, map[string]string{"7": "active_npc"})
	checkMap(t, "Set", dst.Set, map[string]string{"7": "x"})
	checkMap(t, "Set2", dst.Set2, map[string]string{"7": "y"})
	checkMap(t, "Corrupt", dst.Corrupt, map[string]string{"7": "c1"})
	checkMap(t, "Corrupt2", dst.Corrupt2, map[string]string{"7": "c2"})
	checkBoolMap(t, "Conditional", dst.Conditional, map[string]bool{"7": true})
}

// TestToCompilerTypeInfo_CollisionNumericWins pins NAI-212 spec §4
// rule 3 collision precedence: when an int key (stringified) collides
// with a NameMap key, the int-keyed value wins. Empirically impossible
// in TS-canonical call sites (loadRecords is only used for constants);
// defensive rule.
func TestToCompilerTypeInfo_CollisionNumericWins(t *testing.T) {
	src := newTypeInfo()
	src.Add(3, "from-int", true)
	src.NameMap["3"] = "from-str"
	dst := ToCompilerTypeInfo(src)
	if got := dst.Map["3"]; got != "from-int" {
		t.Errorf("Map[\"3\"]=%q, want \"from-int\" (numeric precedence)", got)
	}
}

// --- helpers ---

func checkMap(t *testing.T, name string, got, want map[string]string) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s = %#v, want %#v", name, got, want)
	}
}

func checkBoolMap(t *testing.T, name string, got, want map[string]bool) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s = %#v, want %#v", name, got, want)
	}
}

// runescript import sanity: the package must be reachable from the
// bridge tests without cycle. Confirmed pre-flight; this var keeps the
// import even if some test refactor temporarily removes the only use.
var _ = runescript.CompilerTypeInfo{}
```

### Step 1.2: Run tests to confirm RED

- [ ] Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/ -run TestToCompilerTypeInfo -v`

Expected: build failure ("undefined: ToCompilerTypeInfo"). This is the RED phase.

### Step 1.3: Implement the bridge

- [ ] Create `pkg/pack/compiler/bridge.go` with the following content:

```go
// Package compiler — NAI-212 bridge from compiler.TypeInfo (NAI-200
// dual-map shape) to runescript.CompilerTypeInfo (NAI-210 single-map
// shape). Pure conversion, no IO.
package compiler

import (
	"strconv"

	"github.com/zsrv/goscape/pkg/pack/compiler/runescript"
)

// ToCompilerTypeInfo bridges *compiler.TypeInfo → *runescript.CompilerTypeInfo.
//
// Shape divergence summary (per NAI-212 spec §4):
//   - compiler.TypeInfo splits TS `map: Record<string, string>` into
//     Map (map[int]string) + NameMap (map[string]string) because Go
//     forbids mixed-type keys in one map.
//   - runescript.CompilerTypeInfo carries Map as map[string]string,
//     mirroring the TS single-map shape used by the compiler driver.
//
// Conversion rules:
//   1. Int-keyed Map entries → dst.Map with strconv.Itoa(k) → v.
//   2. NameMap entries → dst.Map under their string keys.
//   3. On key collision (impossible in TS-canonical call sites since
//      loadRecords is only used for constantInfo, but defensively
//      enforced): numeric-id entries win.
//   4. Auxiliary int-keyed maps (VarType/Protect/Require/Require2/
//      Set/Set2/Corrupt/Corrupt2/Conditional) → dst with stringified
//      keys, values preserved.
//   5. Max copies as-is.
func ToCompilerTypeInfo(src *TypeInfo) *runescript.CompilerTypeInfo {
	if src == nil {
		return nil
	}
	dst := &runescript.CompilerTypeInfo{
		Max:         src.Max,
		Map:         make(map[string]string, len(src.Map)+len(src.NameMap)),
		Vartype:     make(map[string]string, len(src.VarType)),
		Protect:     make(map[string]bool, len(src.Protect)),
		Require:     make(map[string]string, len(src.Require)),
		Require2:    make(map[string]string, len(src.Require2)),
		Set:         make(map[string]string, len(src.Set)),
		Set2:        make(map[string]string, len(src.Set2)),
		Corrupt:     make(map[string]string, len(src.Corrupt)),
		Corrupt2:    make(map[string]string, len(src.Corrupt2)),
		Conditional: make(map[string]bool, len(src.Conditional)),
	}

	// Rule 2: NameMap first, so numeric-id entries overwrite on collision.
	for k, v := range src.NameMap {
		dst.Map[k] = v
	}
	// Rule 1 + 3: numeric-id entries (precedence).
	for k, v := range src.Map {
		dst.Map[strconv.Itoa(k)] = v
	}

	// Rule 4: auxiliary maps.
	for k, v := range src.VarType {
		dst.Vartype[strconv.Itoa(k)] = v
	}
	for k, v := range src.Protect {
		dst.Protect[strconv.Itoa(k)] = v
	}
	for k, v := range src.Require {
		dst.Require[strconv.Itoa(k)] = v
	}
	for k, v := range src.Require2 {
		dst.Require2[strconv.Itoa(k)] = v
	}
	for k, v := range src.Set {
		dst.Set[strconv.Itoa(k)] = v
	}
	for k, v := range src.Set2 {
		dst.Set2[strconv.Itoa(k)] = v
	}
	for k, v := range src.Corrupt {
		dst.Corrupt[strconv.Itoa(k)] = v
	}
	for k, v := range src.Corrupt2 {
		dst.Corrupt2[strconv.Itoa(k)] = v
	}
	for k, v := range src.Conditional {
		dst.Conditional[strconv.Itoa(k)] = v
	}

	return dst
}
```

### Step 1.4: Run tests to confirm GREEN

- [ ] Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/ -run TestToCompilerTypeInfo -v`

Expected: 5 tests PASS.

- [ ] Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./pkg/pack/compiler/...`

Expected: no output (clean).

### Step 1.5: Commit

- [ ] Run:

```bash
git add pkg/pack/compiler/bridge.go pkg/pack/compiler/bridge_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(compiler): NAI-212 T1 — ToCompilerTypeInfo bridge

Converts compiler.TypeInfo (NAI-200 dual-map int+string keys) into
runescript.CompilerTypeInfo (NAI-210 single-map string keys) so the
compiler driver can consume what BuildSymbols produces. 5 unit tests
cover empty / numeric-ids / NameMap / auxiliary maps / collision
precedence.
EOF
)"
```

---

## Task 2: `compiler.RunServerCompiler` + test seam

**Goal:** Wrap `BuildSymbols` + bridge + `runescript.Compile` into the single function `RunServerCompiler(srcDir, outDir, dataPackDir) error`. Expose `runServerCompilerCore(srcDir, outDir string, loaders *configLoaders) error` test seam (lowercase / unexported) mirroring the `buildSymbolsCore` precedent.

**Files:**
- Create: `pkg/pack/compiler/run_server_compiler.go`
- Create: `pkg/pack/compiler/run_server_compiler_test.go`

### Step 2.1: Write the 3 failing tests

- [ ] Create `pkg/pack/compiler/run_server_compiler_test.go` with the following content:

```go
package compiler

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
)

// writeFile is a local test helper mirroring pkg/pack's writeFile.
// We can't import pkg/pack from pkg/pack/compiler (cycle), so it's
// duplicated here.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// emptyConfigLoaders returns a *configLoaders with all 7 entity-type
// configs allocated but empty. RunServerCompiler's enrichment passes
// tolerate nil/empty Configs slices (verified at symbols.go:274-291,
// 295-313, 319-336, 340-357, 363-379).
func emptyConfigLoaders() *configLoaders {
	return &configLoaders{
		inv:     &objtype.InvTypeConfigs{},
		comp:    &objtype.ComponentTypeConfigs{},
		varp:    &objtype.VarpTypeConfigs{},
		varn:    &objtype.VarnTypeConfigs{},
		varsCfg: &objtype.VarsTypeConfigs{},
		param:   &objtype.ParamTypeConfigs{},
		dbtable: &objtype.DbTableTypeConfigs{},
	}
}

// TestRunServerCompilerCore_HappyPath_WritesJagOutput pins NAI-212 spec
// §7 RunServerCompiler test 1 (Mitigation A: test seam).
//
// Sets up a minimal scripts dir with one [proc,helper] body, seeds
// script.pack with the matching id, then invokes runServerCompilerCore.
// Asserts the Jag sink wrote both server-side outputs.
func TestRunServerCompilerCore_HappyPath_WritesJagOutput(t *testing.T) {
	tmp := t.TempDir()

	// scripts/helper.rs2 — a proc body that compiles cleanly.
	writeFile(t, filepath.Join(tmp, "scripts", "helper.rs2"),
		"[proc,helper]\nreturn;\n")

	// pack/script.pack — registers id 0 → [proc,helper] so the
	// SymbolMapper resolves the compiled script to a non-negative id.
	writeFile(t, filepath.Join(tmp, "pack", "script.pack"),
		"0=[proc,helper]\n")

	outDir := filepath.Join(tmp, "out")

	if err := runServerCompilerCore(tmp, outDir, emptyConfigLoaders()); err != nil {
		t.Fatalf("runServerCompilerCore: %v", err)
	}

	for _, name := range []string{"script.dat", "script.idx"} {
		p := filepath.Join(outDir, "server", name)
		fi, err := os.Stat(p)
		if err != nil {
			t.Fatalf("missing %s: %v", p, err)
		}
		if fi.Size() == 0 {
			t.Errorf("%s is empty (0 bytes)", p)
		}
	}
}

// TestRunServerCompilerCore_MissingScriptsDir pins NAI-212 spec §7
// RunServerCompiler test 2: BuildSymbols error path. Passing an srcDir
// with no scripts/ subdir produces a constants-walk error from
// buildSymbolsCore; the error wraps with "RunServerCompiler:".
func TestRunServerCompilerCore_MissingScriptsDir(t *testing.T) {
	tmp := t.TempDir()
	// Intentionally no scripts/ or pack/ subdir.
	outDir := filepath.Join(tmp, "out")

	err := runServerCompilerCore(tmp, outDir, emptyConfigLoaders())
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !strings.Contains(err.Error(), "RunServerCompiler") {
		t.Errorf("error %q does not wrap with \"RunServerCompiler\"", err.Error())
	}
}

// TestRunServerCompilerCore_EmptyScriptPack_HeaderOnly pins the
// degenerate path: no pack/script.pack on disk. Per [[plan_compile_facade_runescript_map_seeding]],
// Load returns an empty *TypeInfo, bridge produces an empty Map,
// SymbolMapper.Get returns -1 for the lone compiled proc, and the
// Jag writer emits an 8-byte zero-header file with no blob. Compile
// returns nil. Asserts header-only output shape.
func TestRunServerCompilerCore_EmptyScriptPack_HeaderOnly(t *testing.T) {
	tmp := t.TempDir()
	// Scripts dir present, one parseable proc, but no pack/script.pack.
	writeFile(t, filepath.Join(tmp, "scripts", "helper.rs2"),
		"[proc,helper]\nreturn;\n")

	outDir := filepath.Join(tmp, "out")
	if err := runServerCompilerCore(tmp, outDir, emptyConfigLoaders()); err != nil {
		t.Fatalf("runServerCompilerCore: %v", err)
	}

	dat := filepath.Join(outDir, "server", "script.dat")
	fi, err := os.Stat(dat)
	if err != nil {
		t.Fatalf("missing %s: %v", dat, err)
	}
	if fi.Size() != 8 {
		t.Errorf("script.dat size = %d, want 8 (header-only when SymbolMapper has no mapping)", fi.Size())
	}
}
```

### Step 2.2: Run tests to confirm RED

- [ ] Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/ -run TestRunServerCompiler -v`

Expected: build failure ("undefined: runServerCompilerCore"). This is the RED phase.

### Step 2.3: Implement `RunServerCompiler` + test seam

- [ ] Create `pkg/pack/compiler/run_server_compiler.go` with the following content:

```go
// pkg/pack/compiler/run_server_compiler.go
package compiler

import (
	"fmt"
	"path/filepath"

	"github.com/zsrv/goscape/pkg/pack/compiler/runescript"
)

// RunServerCompiler is the goscape equivalent of TS runServerCompiler's
// final CompileServerScript({symbols}) call (Compiler.ts:330-365).
//
// It chains:
//   1. BuildSymbols(srcDir, dataPackDir) — assembles the 32-key
//      symbol-category dict (NAI-202).
//   2. ToCompilerTypeInfo per entry — bridges NAI-200 dual-map shape
//      into NAI-210 single-map shape (NAI-212 T1).
//   3. runescript.Compile(cfg) — drives parse → analyze → codegen →
//      pointer-check → write (NAI-210).
//
// srcDir: directory containing scripts/ and pack/ subdirs.
// outDir: directory under which <outDir>/server/script.{dat,idx} land.
// dataPackDir: cache directory with the 7 .dat/.idx pairs BuildSymbols
// reads (InvType, Component, VarP, VarN, VarS, Param, DbTableType).
// In practice callers pass outDir for dataPackDir (i.e. read back the
// cache PackConfigs just wrote).
//
// NAI-212-D-EXPLICIT-SOURCEPATHS: TS CompileServerScript defaults
// sourcePaths to "../content/scripts". goscape parameterizes srcDir
// so it cannot rely on a CWD-relative default; SourcePaths is passed
// explicitly. Permanent deviation.
func RunServerCompiler(srcDir, outDir, dataPackDir string) error {
	loaders, err := loadConfigs(dataPackDir)
	if err != nil {
		return fmt.Errorf("RunServerCompiler: %w", err)
	}
	return runServerCompilerCore(srcDir, outDir, loaders)
}

// runServerCompilerCore is the testable seam under RunServerCompiler,
// mirroring buildSymbolsCore precedent. Takes pre-loaded *configLoaders
// so unit tests can pass synthetic in-memory configs without writing
// binary cache fixtures.
func runServerCompilerCore(srcDir, outDir string, loaders *configLoaders) error {
	symbols, err := buildSymbolsCore(srcDir, loaders)
	if err != nil {
		return fmt.Errorf("RunServerCompiler: %w", err)
	}

	bridged := make(map[string]*runescript.CompilerTypeInfo, len(symbols))
	for k, v := range symbols {
		bridged[k] = ToCompilerTypeInfo(v)
	}

	serverOut := filepath.Join(outDir, "server")
	cfg := runescript.Config{
		SourcePaths: []string{filepath.Join(srcDir, "scripts")},
		Symbols:     bridged,
		Writer: runescript.WriterConfig{
			Jag: &runescript.JagWriterConfig{Output: serverOut},
		},
	}
	if err := runescript.Compile(cfg); err != nil {
		return fmt.Errorf("RunServerCompiler: %w", err)
	}
	return nil
}
```

### Step 2.4: Run tests to confirm GREEN

- [ ] Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/compiler/ -run TestRunServerCompiler -v`

Expected: 3 tests PASS.

If `TestRunServerCompilerCore_EmptyScriptPack_HeaderOnly` fails because `script.dat` is a different size than 8 bytes, do NOT relax the assertion. Investigate: read `pkg/pack/compiler/runescript/jag_file_writer.go` and `binary_context.go` to confirm the header-only shape, and update the expected size to match the actual writer contract. Document the discrepancy in the commit body.

- [ ] Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./pkg/pack/compiler/...`

Expected: no output.

### Step 2.5: Commit

- [ ] Run:

```bash
git add pkg/pack/compiler/run_server_compiler.go pkg/pack/compiler/run_server_compiler_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(compiler): NAI-212 T2 — RunServerCompiler driver + test seam

Wires BuildSymbols (NAI-202) → ToCompilerTypeInfo bridge (T1) →
runescript.Compile (NAI-210) under a single RunServerCompiler entry
point. runServerCompilerCore test seam mirrors buildSymbolsCore so
tests pass synthetic *configLoaders. 3 tests pin happy path / missing
scripts dir / empty script.pack. NAI-212-D-EXPLICIT-SOURCEPATHS tag
documents the CWD-relative default the goscape entry cannot use.
EOF
)"
```

---

## Task 3: `pack.PackAll` orchestrator

**Goal:** `PackAll(srcDir, outDir, dataPackDir)` runs `ClearFsCache` → `PackConfigs` → `compiler.RunServerCompiler` in sequence, returning the first error wrapped with the stage name.

**Files:**
- Create: `pkg/pack/pack_all.go`
- Create: `pkg/pack/pack_all_test.go`

### Step 3.1: Write the 2 failing tests

- [ ] Create `pkg/pack/pack_all_test.go` with the following content:

```go
package pack

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestPackAll_ThreeStageSmoke pins NAI-212 spec §7 PackAll test 1.
//
// Drives a fixture with one .obj source + one .rs2 script through the
// full three-stage pipeline. Asserts each stage produced its expected
// artifact:
//   - Stage A (PackConfigs server): <outDir>/server/obj.dat exists.
//   - Stage B (PackConfigs client): <outDir>/client/config jagfile exists.
//   - Stage C (RunServerCompiler): <outDir>/server/script.dat exists.
//
// dataPackDir is passed as outDir so RunServerCompiler reads back the
// cache PackConfigs just wrote.
func TestPackAll_ThreeStageSmoke(t *testing.T) {
	dir := t.TempDir()

	// Minimal .obj fixture mirrors pkg/pack/obj_test.go shape.
	writeFile(t, filepath.Join(dir, "scripts", "o.obj"),
		"[bronze_sword]\nname=Bronze sword\n")
	writeFile(t, filepath.Join(dir, "pack", "obj.pack"),
		"0=bronze_sword\n")

	// Minimal .rs2 script: a single empty proc.
	writeFile(t, filepath.Join(dir, "scripts", "helper.rs2"),
		"[proc,helper]\nreturn;\n")
	writeFile(t, filepath.Join(dir, "pack", "script.pack"),
		"0=[proc,helper]\n")

	outDir := filepath.Join(dir, "out")
	if err := PackAll(dir, outDir, outDir); err != nil {
		t.Fatalf("PackAll: %v", err)
	}

	for _, p := range []string{
		filepath.Join(outDir, "server", "obj.dat"),
		filepath.Join(outDir, "server", "script.dat"),
		filepath.Join(outDir, "client", "config"),
	} {
		if fi, err := os.Stat(p); err != nil {
			t.Errorf("missing %s: %v", p, err)
		} else if fi.Size() == 0 {
			// Header-only files (8 bytes) are acceptable; truly-empty
			// (0 bytes) is not.
			t.Errorf("%s is 0 bytes", p)
		}
	}
}

// TestPackAll_PackConfigsErrorPropagates pins NAI-212 spec §7 PackAll
// test 2: error from a stage is wrapped with the stage name.
//
// We trigger a PackConfigs failure by writing a .varn / .vars name
// collision (cross-domain uniqueness check at pack_configs.go:85
// returns an error). The PackAll wrapper must prefix "PackAll:" or
// "PackConfigs:" so the caller can identify which stage failed.
func TestPackAll_PackConfigsErrorPropagates(t *testing.T) {
	dir := t.TempDir()

	// Cross-domain collision: same name "duplicate_name" registered as
	// both varn and vars triggers checkVarNameUniqueness.
	writeFile(t, filepath.Join(dir, "scripts", "n.varn"),
		"[duplicate_name]\ntype=int\n")
	writeFile(t, filepath.Join(dir, "scripts", "s.vars"),
		"[duplicate_name]\ntype=int\n")
	writeFile(t, filepath.Join(dir, "pack", "varn.pack"),
		"0=duplicate_name\n")
	writeFile(t, filepath.Join(dir, "pack", "vars.pack"),
		"0=duplicate_name\n")

	outDir := filepath.Join(dir, "out")
	err := PackAll(dir, outDir, outDir)
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !strings.Contains(err.Error(), "PackConfigs") {
		t.Errorf("error %q does not name the failing stage (\"PackConfigs\")",
			err.Error())
	}
}
```

### Step 3.2: Run tests to confirm RED

- [ ] Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/ -run TestPackAll -v`

Expected: build failure ("undefined: PackAll"). This is the RED phase.

### Step 3.3: Implement `PackAll`

- [ ] Create `pkg/pack/pack_all.go` with the following content:

```go
// pkg/pack/pack_all.go
package pack

import (
	"fmt"

	"github.com/zsrv/goscape/pkg/pack/compiler"
)

// PackAll is the goscape equivalent of TS packAll (PackAll.ts:17-52),
// restricted to its server-applicable steps.
//
// Pipeline:
//   1. ClearFsCache — drop cached file stats so the per-file
//      freshness gates re-stat from disk.
//   2. PackConfigs(srcDir, outDir) — pack all 18 server-side config
//      types into <outDir>/server/<type>.{dat,idx} + the
//      <outDir>/client/config jagfile.
//   3. compiler.RunServerCompiler(srcDir, outDir, dataPackDir) —
//      compile all .rs2 sources into <outDir>/server/script.{dat,idx}
//      using the symbol tables freshly written by stage 2.
//
// dataPackDir is the cache directory RunServerCompiler reads (the 7
// entity-type loaders: InvType, Component, VarP, VarN, VarS, Param,
// DbTableType). Most callers pass outDir (read back what PackConfigs
// just wrote); the spec leaves it explicit so callers can point at a
// pre-built cache without re-packing.
//
// Errors from any stage are wrapped with the stage name and returned
// immediately. Subsequent stages do not execute.
//
// NAI-212-D-CLIENT-PACKERS-DEFERRED: TS packAll calls 9 additional
// stages with no goscape implementation: packClientInterface,
// packClientTitle, packClientMedia, packClientTexture,
// packClientWordenc, packClientSound, packClientGraphics,
// packClientMidi, packMaps. Retires when the client-pack arc lands.
//
// NAI-212-D-REVALIDATEPACK-INSIDE-PACKCONFIGS: TS packAll calls
// revalidatePack() before packConfigs(). PackConfigs constructs+saves
// every PackFile it touches internally, making a standalone revalidate
// a no-op in goscape. Permanent.
func PackAll(srcDir, outDir, dataPackDir string) error {
	ClearFsCache()
	if err := PackConfigs(srcDir, outDir); err != nil {
		return fmt.Errorf("PackAll: PackConfigs: %w", err)
	}
	if err := compiler.RunServerCompiler(srcDir, outDir, dataPackDir); err != nil {
		return fmt.Errorf("PackAll: %w", err)
	}
	return nil
}
```

### Step 3.4: Run tests to confirm GREEN

- [ ] Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/ -run TestPackAll -v`

Expected: 2 tests PASS.

If `TestPackAll_PackConfigsErrorPropagates` passes silently because `checkVarNameUniqueness` doesn't actually surface (e.g., loader fixture too sparse), **immediately** investigate: read `pkg/pack/pack_configs.go:625` to confirm the function signature and trigger condition, adjust the fixture to actually trip the check, and re-run. Do NOT relax the assertion.

- [ ] Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./pkg/pack/...`

Expected: no output.

### Step 3.5: Commit

- [ ] Run:

```bash
git add pkg/pack/pack_all.go pkg/pack/pack_all_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(pack): NAI-212 T3 — PackAll server-side orchestrator

Top-level entry: ClearFsCache → PackConfigs → compiler.RunServerCompiler.
Wraps each stage error with the stage name. Two integration tests pin
the three-stage smoke and PackConfigs error propagation. Two live
deviation tags documented inline: CLIENT-PACKERS-DEFERRED and
REVALIDATEPACK-INSIDE-PACKCONFIGS.
EOF
)"
```

---

## Task 4: Deviation pins

**Goal:** Pin the three live deviation tags in a single test file under `pkg/pack/`. One test per tag; each greps the production source for the literal tag string. Pattern mirrors `pkg/pack/compiler/runescript/nai210_deviation_pins_test.go` and siblings.

**Files:**
- Create: `pkg/pack/nai212_deviation_pins_test.go`

### Step 4.1: Write the 3 pin tests

- [ ] Create `pkg/pack/nai212_deviation_pins_test.go` with the following content:

```go
// pkg/pack/nai212_deviation_pins_test.go
package pack

import (
	"os"
	"strings"
	"testing"
)

// TestNAI212DeviationPin_ClientPackersDeferred ensures the
// CLIENT-PACKERS-DEFERRED tag remains grep-discoverable in pack_all.go.
// Retires when the client-pack arc lands and the tag's doc-paragraph
// is removed.
func TestNAI212DeviationPin_ClientPackersDeferred(t *testing.T) {
	requireTagInFile(t, "pack_all.go", "NAI-212-D-CLIENT-PACKERS-DEFERRED")
}

// TestNAI212DeviationPin_RevalidatePackInsidePackConfigs ensures the
// REVALIDATEPACK-INSIDE-PACKCONFIGS tag remains grep-discoverable in
// pack_all.go. Permanent (no retirement plan unless PackConfigs is
// refactored to decouple namemap refresh from packing).
func TestNAI212DeviationPin_RevalidatePackInsidePackConfigs(t *testing.T) {
	requireTagInFile(t, "pack_all.go", "NAI-212-D-REVALIDATEPACK-INSIDE-PACKCONFIGS")
}

// TestNAI212DeviationPin_ExplicitSourcePaths ensures the
// EXPLICIT-SOURCEPATHS tag remains grep-discoverable in
// compiler/run_server_compiler.go. Permanent.
func TestNAI212DeviationPin_ExplicitSourcePaths(t *testing.T) {
	requireTagInFile(t, "compiler/run_server_compiler.go", "NAI-212-D-EXPLICIT-SOURCEPATHS")
}

func requireTagInFile(t *testing.T, relPath, tag string) {
	t.Helper()
	data, err := os.ReadFile(relPath)
	if err != nil {
		t.Fatalf("read %s: %v", relPath, err)
	}
	if !strings.Contains(string(data), tag) {
		t.Errorf("%s missing tag %q", relPath, tag)
	}
}
```

### Step 4.2: Run tests

- [ ] Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/ -run TestNAI212DeviationPin -v`

Expected: 3 tests PASS (the tags were written into the doc-comments at T2 + T3).

### Step 4.3: Commit

- [ ] Run:

```bash
git add pkg/pack/nai212_deviation_pins_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
test(pack): NAI-212 T4 — deviation pins

Pins the three live NAI-212 deviation tags so they stay grep-
discoverable across refactors: CLIENT-PACKERS-DEFERRED,
REVALIDATEPACK-INSIDE-PACKCONFIGS (both in pack_all.go), and
EXPLICIT-SOURCEPATHS (in compiler/run_server_compiler.go).
EOF
)"
```

---

## Task 5: Full-suite verification + close

**Goal:** Run the entire test suite (including `-race`) to confirm no regressions from the bridge / driver / orchestrator additions, then write the close commit with a `Closes memory:` trailer per `[[close_commit_memory_trailer]]`.

**Files:** None (verification + close commit only).

### Step 5.1: Run the full suite

- [ ] Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`

Expected: no output.

- [ ] Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`

Expected: ALL PASS. If any pre-existing failures surface, attribute them per `[[verify_implementer_claims]]` — re-run the failing package at `HEAD~5` (before T1) and confirm same failure to attribute as pre-existing.

- [ ] Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./pkg/pack/... ./pkg/pack/compiler/...`

Expected: PASS (the bridge has no concurrent paths; orchestrator is sequential; included for parity with the NAI-211 close).

- [ ] Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...`

Expected: no output.

### Step 5.2: Combined-review handoff (Sonnet reviewer)

- [ ] Dispatch a Sonnet code-reviewer subagent with the diff `HEAD~4..HEAD` (T1..T4) and the spec path. Address findings via a single fixup commit if needed before close.

### Step 5.3: Write the close commit

- [ ] Run:

```bash
git commit --allow-empty --no-gpg-sign -m "$(cat <<'EOF'
close(compiler/runescript): NAI-212 — server-side PackAll + runServerCompiler glue

Wires BuildSymbols (NAI-202) → ToCompilerTypeInfo bridge → runescript.Compile
(NAI-210) under compiler.RunServerCompiler; adds pack.PackAll top-level
orchestrator (ClearFsCache → PackConfigs → RunServerCompiler). Three
live deviation tags pinned: CLIENT-PACKERS-DEFERRED (9 TS packAll
stages with no goscape analog), REVALIDATEPACK-INSIDE-PACKCONFIGS
(PackConfigs already refreshes namemaps internally), and
EXPLICIT-SOURCEPATHS (parameterized srcDir vs TS CWD-relative default).

Closes memory: nai212_close
EOF
)"
```

### Step 5.4: Save close memory entry

- [ ] Write the close memory per `[[close_commit_memory_trailer]]` convention. Create `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai212_close.md` with the standard layout:
  - YAML frontmatter (name, description, type=project).
  - Scope delivered (3 layers + files).
  - Live deviation tags (3).
  - Open follow-ups (CLI wiring, client-pack arc, ::rebuild cheat, macros).
  - Closes memory line.

- [ ] Update `MEMORY.md` index with one-line entry per `[[session_context_management]]`:

```
- [NAI-212 close](nai212_close.md) — server-side PackAll + RunServerCompiler glue shipped 2026-05-16; bridge + driver + orchestrator across pkg/pack/{compiler/bridge,compiler/run_server_compiler,pack_all}.go; 3 live tags
```

---

## Self-review notes

**Spec coverage:**
- §4 type bridge → T1 (5 tests cover empty / numeric / NameMap / aux maps / collision precedence).
- §5 RunServerCompiler → T2 (3 tests + test seam).
- §6 PackAll → T3 (2 integration tests).
- §7 testing strategy → T1 + T2 + T3 + T4 collectively.
- §8 deviation tags (3) → T4 pins all three.
- §9 file inventory matches T1–T4 file structure.
- §10 sequencing T1–T5 matches plan tasks T1–T5.
- §11 risks: cycle risk verified pre-plan (pkg/pack/compiler does not import pkg/pack; pkg/pack/compiler/runescript does not import pkg/pack/compiler root). Plan does not need a fallback subpackage.
- §12 follow-ups explicitly out-of-scope (CLI, client-pack, ::rebuild, macros).

**Placeholder scan:** no "TBD" / "TODO" / "implement later". The one runtime `t.Skip` in T2 step 2.1 is bounded — step 2.4 mandates immediate rewrite per the inline branch.

**Type consistency:**
- `compiler.TypeInfo` (with `Map: map[int]string`, `NameMap: map[string]string`) — T1 src, T1 tests, T2 production via `BuildSymbols`.
- `runescript.CompilerTypeInfo` (with `Map: map[string]string`, `Vartype: map[string]string`) — T1 dst, T2 production via bridge.
- `*configLoaders` (unexported, in `pkg/pack/compiler`) — T2 production + tests.
- `runescript.Config` / `runescript.WriterConfig` / `runescript.JagWriterConfig` — T2 production only.
- `ClearFsCache` (no args), `PackConfigs(srcDir, outDir)`, `compiler.RunServerCompiler(srcDir, outDir, dataPackDir)` — T3 calls.

All signatures match across tasks.

**Cadence note:** standard cadence per `[[runescript_cadence]]`. T1–T4 dispatched as fresh subagents per `[[execution_mode_default]]`. T5 verification + close in the controller session.
