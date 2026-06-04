# Go Packer Determinism Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `goscape-cli pack` produce byte-identical server-side output (`server/script.dat`, `server/script.idx`, and config `.dat`/`.idx`) across repeated runs over the same source tree. Once stable, re-measure remaining content diffs against the TS-packed cache.

**Architecture:** The non-determinism is concentrated in `pkg/pack/compiler/` (script bytecode emission) where several `for k, v := range map` iterations contribute to the on-the-wire byte stream — either directly (sequence emitted in iteration order) or indirectly (sequencing affects symbol-table state observed by later code). Fix is mechanical: replace each suspect `range map` with sorted-key iteration via `slices.Sorted(maps.Keys(m))` or an explicit `sort` over collected keys. Gate every change with a regression test that packs a fixture twice and `bytes.Equal`s the outputs.

**Tech Stack:** Go 1.26.3, `cmp` byte comparison, `t.TempDir()` for isolated fixtures, the existing `seedMinimalPackFixture` from `cmd/goscape-cli/cmd_pack_test.go` extended with a multi-script GOSUB-bearing variant.

---

## Background

Resume doc: `.claude/resume/2026-05-22-combat-engine-fixes-packer-determinism-resume.md`.

Three consecutive `goscape-cli pack --src-dir=/home/owner/Code/github.com/LostCityRS/Server225_2/content --out-dir=/home/owner/Code/github.com/zsrv/goscape/data/pack` invocations produce three different `server/script.dat` files (`cmp` diverges at byte 28207 / 129690 across pairs). Same idx checksum table layout, same 8032 file count, same ~4.37 MB total size. Decoding individual scripts: `[queue,reset_itest]` resolved to operand `18` in one run and `19` in another, meaning script-id assignment to script names is non-deterministic.

Explore-agent scoping pass + spot-verification confirmed these suspect sites in `pkg/pack/compiler/`:

| # | File:line | Map | Risk |
|---|-----------|-----|------|
| 1 | `symbol/table.go:128` | `st.symbols map[SymbolType]map[string]Symbol` (outer) | HIGH — `findAllInto` appends to slice in iteration order |
| 2 | `symbols.go:240` | `table.Types map[int][]ScriptVarType` | HIGH — `populateDbColumns` mutates output TypeInfo in iteration order |
| 3 | `runescript/server_script_compiler.go:196` | `c.CommandPointers map[string]*PointerHolder` | MEDIUM — `registerSecondaryCommands` inserts alias symbols in iteration order; affects `SymbolTable.Insert` sequencing |
| 4 | `run_server_compiler.go:108-125` | six `map[int]string` pointer-name maps | LOW (defensive) — in-place rewrites are order-independent for the resulting map, but a defensive sort costs little |
| 5 | `bridge.go:53-84` | nine int-keyed loaders → string-keyed maps | LOW (defensive) — output is a map, so insertion order doesn't change the resulting map; defensive sort is cheap |

Already-correct sites confirmed during scoping (do not modify):
- `runescript/load_special_symbols.go:24,82` — uses `sortedNumericKeys`
- `runescript/type_info_loader.go:43,73` — uses `sortedNumericKeys`
- `runescript/js5_pack_writer.go:64-67` — sorts keys before iteration
- `runescript/jag_file_writer.go:64-67` — sorts keys before iteration
- `symbols.go:124` — sorts entries after collection
- `pack/packfile.go:Save` — sorts ids ascending before write
- `pack/crawl.go::CrawlConfigNames` — uses ordered `filepath.WalkDir` underneath

## Validation strategy

The fix-and-verify loop uses two layers:

1. **Unit test** (`pkg/pack/compiler/run_server_compiler_determinism_test.go`) — packs the existing minimal fixture twice into separate temp dirs, byte-compares the produced `server/script.dat` + `server/script.idx`. Fast (~5s). May or may not reproduce the original bug because the minimal fixture has only 1 script and no cross-script references.

2. **Multi-script unit test** (same file, separate test function) — fixture with 3+ scripts, GOSUB between them, exercises script-id resolution. Designed to fail until script-id assignment is deterministic.

3. **Manual real-world check** (script provided, run by user) — `goscape-cli pack` over Server225_2 twice, `cmp` server outputs, document. Final acceptance gate.

## File Structure

**Files to create:**
- `pkg/pack/compiler/run_server_compiler_determinism_test.go` — packer determinism regression tests.

**Files to modify (one fix per task):**
- `pkg/pack/compiler/symbol/table.go` — `findAllInto` sorted iteration
- `pkg/pack/compiler/symbols.go` — `populateDbColumns` sorted iteration
- `pkg/pack/compiler/runescript/server_script_compiler.go` — `registerSecondaryCommands` sorted iteration
- `pkg/pack/compiler/run_server_compiler.go` — `translateCommandPointerNames` defensive sorted iteration
- `pkg/pack/compiler/bridge.go` — `ToCompilerTypeInfo` defensive sorted iteration

---

### Task 1: Reproduce the bug end-to-end and snapshot a baseline

**Files:**
- None (shell only).

- [ ] **Step 1: Pack three times into separate output dirs**

User-shell command (sandbox does not let agents touch `data/pack/` outputs but the user can run this and paste results):

```bash
cd /home/owner/Code/github.com/zsrv/goscape

mkdir -p /tmp/claude/det-run1 /tmp/claude/det-run2 /tmp/claude/det-run3

CGO_ENABLED=0 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache \
  /home/owner/go/go1.26.3/bin/go run -trimpath ./cmd/goscape-cli pack \
  --src-dir=/home/owner/Code/github.com/LostCityRS/Server225_2/content \
  --out-dir=/tmp/claude/det-run1

# Repeat for run2 / run3 (or copy via rsync if rerun cost too high).
```

- [ ] **Step 2: Confirm script.dat differs across runs**

```bash
cmp /tmp/claude/det-run1/server/script.dat /tmp/claude/det-run2/server/script.dat
cmp /tmp/claude/det-run1/server/script.dat /tmp/claude/det-run3/server/script.dat
cmp /tmp/claude/det-run2/server/script.dat /tmp/claude/det-run3/server/script.dat
```

Expected: each `cmp` reports a differing byte offset (e.g. `differ: char 28207, line 1`).

- [ ] **Step 3: Confirm script.idx is byte-identical across runs**

```bash
cmp /tmp/claude/det-run1/server/script.idx /tmp/claude/det-run2/server/script.idx
```

Expected: exit 0 (identical). The idx file lists script *sizes* — sizes are stable; only operand bytes inside `script.dat` shuffle. (This was the user's observation in the resume.)

- [ ] **Step 4: Take a snapshot of the baseline for later regression comparison**

```bash
cp /tmp/claude/det-run1/server/script.{dat,idx} /tmp/claude/baseline-pre-fix.{dat,idx}
```

(No commit — investigation only.)

---

### Task 2: Add a determinism regression test using the minimal fixture

**Files:**
- Create: `pkg/pack/compiler/run_server_compiler_determinism_test.go`

- [ ] **Step 1: Write the failing test**

```go
// pkg/pack/compiler/run_server_compiler_determinism_test.go
package compiler

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// TestRunServerCompiler_DeterministicMinimal packs the minimal fixture twice
// into separate temp output dirs and asserts byte-identical server outputs.
// Pre-fix this may pass (minimal fixture has 1 script — possibly insufficient
// to trigger map-iteration shuffling) — it's the cheap gate. The multi-script
// variant below is the real reproducer.
func TestRunServerCompiler_DeterministicMinimal(t *testing.T) {
	src := t.TempDir()
	seedMinimalScriptFixture(t, src)

	out1 := t.TempDir()
	if err := RunServerCompiler(src, out1, ""); err != nil {
		t.Fatalf("run 1: %v", err)
	}
	out2 := t.TempDir()
	if err := RunServerCompiler(src, out2, ""); err != nil {
		t.Fatalf("run 2: %v", err)
	}

	assertServerOutputsEqual(t, out1, out2)
}

// TestRunServerCompiler_DeterministicCrossRefs uses a multi-script fixture
// with GOSUB cross-references; this is what catches script-id shuffling.
func TestRunServerCompiler_DeterministicCrossRefs(t *testing.T) {
	src := t.TempDir()
	seedCrossRefScriptFixture(t, src)

	out1 := t.TempDir()
	if err := RunServerCompiler(src, out1, ""); err != nil {
		t.Fatalf("run 1: %v", err)
	}
	out2 := t.TempDir()
	if err := RunServerCompiler(src, out2, ""); err != nil {
		t.Fatalf("run 2: %v", err)
	}

	assertServerOutputsEqual(t, out1, out2)
}

// assertServerOutputsEqual byte-compares server/script.dat and server/script.idx
// across two pack output dirs and fails the test with offset of the first
// divergence if any.
func assertServerOutputsEqual(t *testing.T, out1, out2 string) {
	t.Helper()
	for _, name := range []string{"server/script.dat", "server/script.idx"} {
		a, err := os.ReadFile(filepath.Join(out1, name))
		if err != nil {
			t.Fatalf("read %s from out1: %v", name, err)
		}
		b, err := os.ReadFile(filepath.Join(out2, name))
		if err != nil {
			t.Fatalf("read %s from out2: %v", name, err)
		}
		if !bytes.Equal(a, b) {
			off := firstDifferenceOffset(a, b)
			t.Errorf("%s differs between runs at byte offset %d (len out1=%d, len out2=%d)",
				name, off, len(a), len(b))
		}
	}
}

func firstDifferenceOffset(a, b []byte) int {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		if a[i] != b[i] {
			return i
		}
	}
	if len(a) != len(b) {
		return n
	}
	return -1
}

// seedMinimalScriptFixture writes the smallest srcDir layout that lets
// RunServerCompiler succeed end-to-end: 1 proc script + script.pack registering it.
// Mirrors the minimal half of cmd/goscape-cli/cmd_pack_test.go::seedMinimalPackFixture
// (subset that the script compiler needs).
func seedMinimalScriptFixture(t *testing.T, dir string) {
	t.Helper()
	mustWriteFile(t, filepath.Join(dir, "scripts", "helper.rs2"),
		"[proc,helper]\nreturn;\n")
	mustWriteFile(t, filepath.Join(dir, "pack", "script.pack"),
		"0=[proc,helper]\n")
}

// seedCrossRefScriptFixture writes three procs with two GOSUB cross-references.
// Targets the script-id-shuffle bug: alpha and beta each gosub gamma; if the
// script-id assignment for gamma differs between runs, the PUSH_CONSTANT_INT
// operand bytes preceding the GOSUB opcode in the alpha/beta bytecode shift.
func seedCrossRefScriptFixture(t *testing.T, dir string) {
	t.Helper()
	mustWriteFile(t, filepath.Join(dir, "scripts", "a.rs2"),
		"[proc,alpha]\n~gamma;\nreturn;\n")
	mustWriteFile(t, filepath.Join(dir, "scripts", "b.rs2"),
		"[proc,beta]\n~gamma;\nreturn;\n")
	mustWriteFile(t, filepath.Join(dir, "scripts", "c.rs2"),
		"[proc,gamma]\nreturn;\n")
	mustWriteFile(t, filepath.Join(dir, "pack", "script.pack"),
		"0=[proc,alpha]\n1=[proc,beta]\n2=[proc,gamma]\n")
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
```

- [ ] **Step 2: Run the test to see what state we're in**

```bash
cd /home/owner/Code/github.com/zsrv/goscape
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache \
  /home/owner/go/go1.26.3/bin/go test -run TestRunServerCompiler_Deterministic ./pkg/pack/compiler/ -count=1 -v
```

Expected on the first run: at least `TestRunServerCompiler_DeterministicCrossRefs` should FAIL (this is the script-id-shuffle reproducer). `TestRunServerCompiler_DeterministicMinimal` may pass (insufficient fixture) or fail (sufficient).

If both pass at this stage, the minimal-fixture isn't large enough to trigger the bug — extend `seedCrossRefScriptFixture` with more scripts and a `dbtable` / `interface` / `varp` / `varn` mix until at least one test reproduces, or skip ahead to Task 8's real-world validation.

If at least one fails: this is our gate. Proceed.

- [ ] **Step 3: Commit the failing test**

```bash
git add pkg/pack/compiler/run_server_compiler_determinism_test.go
git commit --no-gpg-sign -m "test(pack/compiler): pin packer output determinism (failing)

Two packer runs over the same source dir must produce byte-identical
server/script.dat and server/script.idx. Minimal fixture (1 proc) and
multi-script fixture with cross-references (3 procs, 2 GOSUBs) each
pack-twice and bytes.Equal the outputs.

Cross-ref variant fails on HEAD because map iteration during symbol-
table assembly and dbcolumn population is non-deterministic.
"
```

---

### Task 3: Fix `findAllInto` to iterate sorted outer keys

**Files:**
- Modify: `pkg/pack/compiler/symbol/table.go` (function `findAllInto` near line 128)

- [ ] **Step 1: Read the current code**

```bash
grep -n "findAllInto\|st\.symbols" pkg/pack/compiler/symbol/table.go | head -20
```

Identify the iteration site (currently `for outerKey, inner := range st.symbols` around line 128).

- [ ] **Step 2: Replace with sorted-key iteration**

The outer map is keyed by `SymbolType`. `SymbolType.Key()` returns a string identifier. Iterate via collected sorted keys. Read the existing types to find the right access pattern, then:

```go
// At the top of table.go ensure import "slices" is present
// (it likely already is for sort.Strings or similar helpers).

// Inside findAllInto, replace
//     for outerKey, inner := range st.symbols {
//         ...
//     }
// with:
keys := make([]symbolTypeKey, 0, len(st.symbols)) // adjust key type to actual map-key type
for k := range st.symbols {
    keys = append(keys, k)
}
slices.SortFunc(keys, func(a, b symbolTypeKey) int {
    return strings.Compare(a.String(), b.String()) // or whatever comparator matches the key type
})
for _, outerKey := range keys {
    inner := st.symbols[outerKey]
    // ... existing inner-loop body unchanged ...
}
```

Inner-map iteration (the nested `for ... := range inner` if present) should also be sorted by name. Inner keys are normalized symbol names (strings) — use `slices.Sort` on collected `[]string` keys.

When you read table.go, adapt the sort comparator to whatever `SymbolType`'s natural ordering should be. If it doesn't have a natural ordering, fall back to `fmt.Sprint(a) < fmt.Sprint(b)` or its `Key()` method.

- [ ] **Step 3: Run the determinism tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache \
  /home/owner/go/go1.26.3/bin/go test -run TestRunServerCompiler_Deterministic ./pkg/pack/compiler/ -count=1 -v
```

Expected: same failures (this is the first of several fixes), OR improvement (test now passes for one variant but not the other). Note whether the failing byte offset shifts or shrinks — that's signal that this fix helped.

- [ ] **Step 4: Run the package's own tests to make sure no regression**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache \
  /home/owner/go/go1.26.3/bin/go test ./pkg/pack/compiler/symbol/... -count=1
```

Expected: all green. If a symbol-table test asserts a specific iteration order, update the assertion to match the now-sorted order.

- [ ] **Step 5: Commit**

```bash
git add pkg/pack/compiler/symbol/table.go
git commit --no-gpg-sign -m "fix(pack/compiler): sort findAllInto iteration for determinism

st.symbols outer keys are now collected and sorted before iteration
so Symbol output ordering does not depend on Go's randomized map
iteration. Eliminates one source of byte-shifting between consecutive
goscape-cli pack runs over the same source tree.
"
```

---

### Task 4: Fix `populateDbColumns` to iterate sorted column ids

**Files:**
- Modify: `pkg/pack/compiler/symbols.go` (function `populateDbColumns` around line 234-260)

- [ ] **Step 1: Read the current code**

```bash
sed -n '230,265p' pkg/pack/compiler/symbols.go
```

Identify the `for column, types := range table.Types` loop (currently around line 240). `table.Types` is `map[int][]ScriptVarType`.

- [ ] **Step 2: Replace with sorted-key iteration**

```go
// Inside populateDbColumns, the existing inner section:
//     for _, table := range tables.Configs {
//         ...
//         for column, types := range table.Types {
//             ...
//         }
//     }
// Becomes:

for _, table := range tables.Configs {
    // ...
    cols := make([]int, 0, len(table.Types))
    for c := range table.Types {
        cols = append(cols, c)
    }
    slices.Sort(cols)
    for _, column := range cols {
        types := table.Types[column]
        // ... existing loop body unchanged ...
    }
}
```

Ensure `slices` is imported (the file likely already imports it; check).

- [ ] **Step 3: Run the determinism tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache \
  /home/owner/go/go1.26.3/bin/go test -run TestRunServerCompiler_Deterministic ./pkg/pack/compiler/ -count=1 -v
```

Expected: progress (failing byte offset moves) or pass.

- [ ] **Step 4: Run the surrounding test suite**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache \
  /home/owner/go/go1.26.3/bin/go test ./pkg/pack/compiler/... -count=1
```

- [ ] **Step 5: Commit**

```bash
git add pkg/pack/compiler/symbols.go
git commit --no-gpg-sign -m "fix(pack/compiler): sort dbcolumn iteration in populateDbColumns

table.Types is a map[int][]ScriptVarType; iterating in random order
caused dbcolumn TypeInfo entries to be populated in non-deterministic
sequence between pack runs.
"
```

---

### Task 5: Fix `registerSecondaryCommands` to iterate sorted CommandPointer names

**Files:**
- Modify: `pkg/pack/compiler/runescript/server_script_compiler.go` (function `registerSecondaryCommands` around line 191-225)

- [ ] **Step 1: Read the current code (already inspected; for reference):**

```go
func (c *ServerScriptCompiler) registerSecondaryCommands() {
    if len(c.CommandPointers) < 1 {
        return
    }
    commandType := symbol.SymbolTypeServerScript(trigger.CommandTrigger)
    for name := range c.CommandPointers {
        if !strings.HasPrefix(name, ".") {
            continue
        }
        // ... insert alias into c.RootTable ...
    }
}
```

- [ ] **Step 2: Replace with sorted-name iteration**

```go
func (c *ServerScriptCompiler) registerSecondaryCommands() {
    if len(c.CommandPointers) < 1 {
        return
    }
    commandType := symbol.SymbolTypeServerScript(trigger.CommandTrigger)
    names := make([]string, 0, len(c.CommandPointers))
    for name := range c.CommandPointers {
        names = append(names, name)
    }
    slices.Sort(names)
    for _, name := range names {
        if !strings.HasPrefix(name, ".") {
            continue
        }
        // ... existing body unchanged ...
    }
}
```

Add `"slices"` to imports if missing.

- [ ] **Step 3: Run the determinism tests + package tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache \
  /home/owner/go/go1.26.3/bin/go test ./pkg/pack/compiler/runescript/... -count=1
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache \
  /home/owner/go/go1.26.3/bin/go test -run TestRunServerCompiler_Deterministic ./pkg/pack/compiler/ -count=1 -v
```

- [ ] **Step 4: Commit**

```bash
git add pkg/pack/compiler/runescript/server_script_compiler.go
git commit --no-gpg-sign -m "fix(pack/compiler): sort registerSecondaryCommands iteration

c.CommandPointers is a map[string]*PointerHolder; iterating it in
randomized order caused alias ServerScriptSymbols to be inserted into
RootTable in non-deterministic sequence, contributing to packer output
byte drift between runs.
"
```

---

### Task 6: Defensive sort on `translateCommandPointerNames`

**Files:**
- Modify: `pkg/pack/compiler/run_server_compiler.go` (function `translateCommandPointerNames` around lines 100-125)

- [ ] **Step 1: Read the current code**

```bash
sed -n '95,130p' pkg/pack/compiler/run_server_compiler.go
```

Currently has six `for k, v := range cmd.Require { cmd.Require[k] = ... }` style in-place mutation loops. Each loop independently rewrites values for the same map. **In-place value rewrites are order-independent for the final map**, so this is defensive (rules out future regressions if downstream iteration becomes order-sensitive).

- [ ] **Step 2: Replace each with sorted-key iteration**

```go
// Before:
//     for k, v := range cmd.Require {
//         cmd.Require[k] = transform(v)
//     }
// After (do this for each of Require, Require2, Set, Set2, Corrupt, Corrupt2):
keys := make([]int, 0, len(cmd.Require))
for k := range cmd.Require {
    keys = append(keys, k)
}
slices.Sort(keys)
for _, k := range keys {
    cmd.Require[k] = transform(cmd.Require[k])
}
```

Or factor a tiny helper:

```go
func sortedKeysInt[V any](m map[int]V) []int {
    keys := make([]int, 0, len(m))
    for k := range m {
        keys = append(keys, k)
    }
    slices.Sort(keys)
    return keys
}
```

and use it for all six.

- [ ] **Step 3: Tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache \
  /home/owner/go/go1.26.3/bin/go test ./pkg/pack/compiler/... -count=1
```

- [ ] **Step 4: Commit**

```bash
git add pkg/pack/compiler/run_server_compiler.go
git commit --no-gpg-sign -m "refactor(pack/compiler): sort translateCommandPointerNames loops (defensive)

In-place value rewrites under map iteration are order-independent for
the resulting map, but a defensive sort guards against future code that
might iterate these maps unsorted and contribute to packer non-
determinism. No expected behavioural change.
"
```

---

### Task 7: Defensive sort on `bridge.go::ToCompilerTypeInfo`

**Files:**
- Modify: `pkg/pack/compiler/bridge.go` (function `ToCompilerTypeInfo` lines 53-84)

- [ ] **Step 1: Read the current code**

```bash
sed -n '40,95p' pkg/pack/compiler/bridge.go
```

Nine loops of the shape:

```go
for k, v := range src.Map {
    dst.Map[strconv.Itoa(k)] = v
}
```

across `src.Map`, `src.VarType`, `src.Protect`, `src.Require`, `src.Require2`, `src.Set`, `src.Set2`, `src.Corrupt`, `src.Corrupt2`, `src.Conditional`.

Output is a map; insertion order doesn't affect the final map. So this is also defensive. But because downstream loaders that consume the bridged maps DO use `sortedNumericKeys` (verified in `type_info_loader.go` and `load_special_symbols.go`), this is genuinely safe today — keeping it sorted is just future-proofing.

- [ ] **Step 2: Replace each with sorted-key iteration**

Factor a helper at top of bridge.go:

```go
func sortedIntKeys[V any](m map[int]V) []int {
    keys := make([]int, 0, len(m))
    for k := range m {
        keys = append(keys, k)
    }
    slices.Sort(keys)
    return keys
}
```

Then each loop becomes:

```go
for _, k := range sortedIntKeys(src.Map) {
    v := src.Map[k]
    dst.Map[strconv.Itoa(k)] = v
}
```

Repeat for all 9 source maps.

- [ ] **Step 3: Tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache \
  /home/owner/go/go1.26.3/bin/go test ./pkg/pack/compiler/... -count=1
```

- [ ] **Step 4: Commit**

```bash
git add pkg/pack/compiler/bridge.go
git commit --no-gpg-sign -m "refactor(pack/compiler): sort ToCompilerTypeInfo loops (defensive)

Nine int-keyed -> string-keyed copies under randomized map iteration.
Output is a map (insertion order doesn't affect the result) and all
known downstream consumers use sortedNumericKeys, so this is purely
future-proofing.
"
```

---

### Task 8: Real-world validation against Server225_2 content

**Files:**
- None (shell only — user runs).

- [ ] **Step 1: Pack twice over real content and cmp**

User-shell:

```bash
cd /home/owner/Code/github.com/zsrv/goscape

rm -rf /tmp/claude/det-post1 /tmp/claude/det-post2
mkdir -p /tmp/claude/det-post1 /tmp/claude/det-post2

CGO_ENABLED=0 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache \
  /home/owner/go/go1.26.3/bin/go run -trimpath ./cmd/goscape-cli pack \
  --src-dir=/home/owner/Code/github.com/LostCityRS/Server225_2/content \
  --out-dir=/tmp/claude/det-post1

CGO_ENABLED=0 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache \
  /home/owner/go/go1.26.3/bin/go run -trimpath ./cmd/goscape-cli pack \
  --src-dir=/home/owner/Code/github.com/LostCityRS/Server225_2/content \
  --out-dir=/tmp/claude/det-post2

cmp /tmp/claude/det-post1/server/script.dat /tmp/claude/det-post2/server/script.dat
echo "script.dat exit: $?"
cmp /tmp/claude/det-post1/server/script.idx /tmp/claude/det-post2/server/script.idx
echo "script.idx exit: $?"
```

Expected: both `cmp` commands exit 0 (no diff). If either still differs, jump to Task 9.

- [ ] **Step 2: Also cmp every other server output file (defensive)**

```bash
for f in /tmp/claude/det-post1/server/*; do
  base=$(basename "$f")
  cmp "$f" "/tmp/claude/det-post2/server/$base" || echo "DIFF: $base"
done
```

Expected: no `DIFF:` lines printed.

- [ ] **Step 3: No commit (validation only).** If everything matches, proceed to Task 10. If anything differs, proceed to Task 9 to enumerate remaining sites.

---

### Task 9 (conditional): Investigate remaining non-determinism

Trigger condition: after Tasks 3-7, Task 8 still shows `cmp` divergence.

**Files:**
- None initially — investigation only.

- [ ] **Step 1: Snapshot the differing byte offset**

```bash
cmp /tmp/claude/det-post1/server/script.dat /tmp/claude/det-post2/server/script.dat
# e.g. differ: char 28207
```

Note: if the offset is now in a different region than the pre-fix offset (28207, 129690), that confirms earlier fixes worked partially.

- [ ] **Step 2: Use the `/tmp/claude/` decode helpers to identify the script-id and operand**

The resume doc lists helpers at `/tmp/claude/decode_script_n.go` (decodes script id N from a .dat) and `/tmp/claude/diff_set_a_vs_b.go` (diffs the set of differing scripts). If those scripts have been purged, recreate from the resume's commit `02615ab2` reference.

```bash
# Identify which script contains the offset.
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache \
  /home/owner/go/go1.26.3/bin/go run /tmp/claude/diff_all_scripts.go \
  /tmp/claude/det-post1/server/script.dat \
  /tmp/claude/det-post2/server/script.dat | head
```

- [ ] **Step 3: Search for additional `range map` sites**

```bash
cd /home/owner/Code/github.com/zsrv/goscape
grep -rn "for .* := range " pkg/pack/compiler/ | grep -v _test.go | grep -v 'sortedNumericKeys\|sortedStringKeys\|sortedIntKeys\|sortedKeysInt' > /tmp/claude/remaining-ranges.txt
wc -l /tmp/claude/remaining-ranges.txt
```

Inspect each entry — for any iteration over a `map[K]V`, audit whether the iteration order is observed (writes to output, conditional logic that branches on order, slice-appends). Promote any newly-identified suspect to a Task-3-style fix.

- [ ] **Step 4: Iterate Task 8 until clean**

After each new fix: commit, then re-run Task 8.

---

### Task 10: Compare deterministic Go output to TS-packed cache

**Files:**
- None (validation only).

- [ ] **Step 1: Identify TS-packed baseline location**

The user maintains a TS-packed snapshot at `/home/owner/Code/github.com/zsrv/goscape/data/pack/server/script.dat` (per the resume's reference). Confirm timestamp / cocktail-guide content:

```bash
ls -la data/pack/server/script.{dat,idx}
```

If this has drifted, re-pack with TS (`/home/owner/Code/github.com/LostCityRS/Engine-TS`) into a known temp dir.

- [ ] **Step 2: Diff Go vs TS**

```bash
cmp /tmp/claude/det-post1/server/script.dat data/pack/server/script.dat
cmp /tmp/claude/det-post1/server/script.idx data/pack/server/script.idx
```

If `script.idx` is byte-identical and `script.dat` differs: remaining divergence is real bytecode shape, not non-determinism.

- [ ] **Step 3: Count differing scripts**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache \
  /home/owner/go/go1.26.3/bin/go run /tmp/claude/diff_all_scripts.go \
  /tmp/claude/det-post1/server/script.dat \
  data/pack/server/script.dat | tee /tmp/claude/remaining-diffs.txt
wc -l /tmp/claude/remaining-diffs.txt
```

Pre-fix this was ~163 scripts. Determinism alone shouldn't change this number much (different runs still all diverge from TS at the same scripts), but the *set* of differing scripts will now be stable.

- [ ] **Step 4: Document remaining diffs in a new PORTING.md row OR a follow-up plan**

If `<10` remaining diffs: list each in a follow-up arc memo.
If `≥10`: open a new plan doc `docs/superpowers/plans/YYYY-MM-DD-pack-ts-parity.md` for the bytecode-shape work.

(No commit yet — this is the analysis step.)

---

### Task 11: Final validation gate + arc commit summary

**Files:**
- None (verification + optional doc).

- [ ] **Step 1: Run the full repo test suite with race**

```bash
cd /home/owner/Code/github.com/zsrv/goscape
GOROOT=/home/owner/go/go1.26.3 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache \
  /home/owner/go/go1.26.3/bin/go test -race ./... -count=1
```

Expected: all green except the three pre-existing compiler-version-mismatch failures noted in the resume (`TestNAI128_RatLootCascade`, `TestReload_ScriptCount_NodeDebug_SuccessBroadcast`, `TestHandleClientCheat_Reload_Dispatches`). The new `TestRunServerCompiler_Deterministic*` tests must pass.

- [ ] **Step 2: gofmt clean**

```bash
GOROOT=/home/owner/go/go1.26.3 /home/owner/go/go1.26.3/bin/gofmt -l modules pkg cmd internal
```

Expected: no output.

- [ ] **Step 3: git log review**

```bash
git log --oneline ^02615ab2 HEAD
```

Expected: 5-7 commits (the test + 3-5 fixes + optional defensive sorts).

- [ ] **Step 4: Optional — add a PORTING.md row if remaining TS diffs need follow-up**

Per the resume backlog snapshot, PORTING.md is effectively empty. If Task 10 surfaced a follow-up class of bytecode-shape diffs, add a single row referencing the new plan doc.

---

## Self-review checklist

- [x] **Spec coverage:** Each suspect site from the Explore agent's scoping (table.go:128, symbols.go:240, server_script_compiler.go:196, run_server_compiler.go:108-125, bridge.go:53-84) has a dedicated task.
- [x] **Reproducer first:** Task 1 + Task 2 establish the bug exists before any fix.
- [x] **Real-world gate:** Task 8 is the acceptance test on full Server225_2 content.
- [x] **Backstop:** Task 9 has a conditional path for further investigation if Tasks 3-7 don't suffice.
- [x] **TS parity follow-up:** Task 10 separates determinism (fixed here) from bytecode-shape divergence (followed up elsewhere).
- [x] **Test isolation:** All unit tests use `t.TempDir()` — no shared state.
- [x] **Sandbox awareness:** Tasks 1, 8, and 10 are flagged as user-shell because they pack against a path the agent sandbox cannot reach without authorization.
