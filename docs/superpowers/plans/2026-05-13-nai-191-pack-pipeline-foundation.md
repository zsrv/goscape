# NAI-191 — Pack-pipeline source-side foundation — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Open the multi-slice arc that eventually closes `::rebuild`. NAI-191 ships a new `pkg/pack/` package — format-agnostic source-side foundation. No production callsites; library-only.

**Architecture:** Six self-contained files in a new `pkg/pack/` package, each porting one TS source file or one cohort from `tools/pack/PackFile.ts`. Dependency order: `fscache` → `parse`/`namemap`/`freshness` → `packfile`/`crawl`. Each task is one file + tests + commit. Testdata fixtures live in `pkg/pack/testdata/`.

**Tech Stack:** Go 1.26+, stdlib only (`os`, `path/filepath`, `bufio`, `sort`, `strconv`, `strings`, `sync`, `time`). No new external deps.

**Spec:** `docs/superpowers/specs/2026-05-13-nai-191-pack-pipeline-foundation-design.md`
**Predecessors:** NAI-190 (`e7b2950`) — `World.reload()` port + `::reload` cheat.
**HEAD at plan-write:** `2ea986d` (spec commit).

---

## §0 Pre-flight verifications (controller, not implementer)

Performed at plan-write against HEAD `2ea986d`. Re-verify only on contradiction.

1. **No existing `pkg/pack/`** — confirmed via `find pkg -name "pack*"` returns only `pkg/io/packet/` (RS2 wire packet buffer, unrelated).
2. **Module path** — `github.com/zsrv/goscape` per `go.mod:1`; package import path `github.com/zsrv/goscape/pkg/pack`.
3. **Go version** — `go.mod:3` declares `go 1.26`; modern Go idioms (`min`/`max` builtins, `for-range` over int, etc.) are permitted.
4. **TS source quirks pinned in plan:**
   - `loadFileFull`'s `/* /* */ */`-on-one-line case strips ONLY the first `/* */`, leaving literal ` */` in the output (re-traced verbatim from `Parse.ts:55-73`). Task 2's test (g) pins this.
   - `PackFile.refreshNames` does NOT rebuild `nameToId` — only `names` and `max` (re-traced from `PackFileBase.ts:106-111`). Task 5's test (e) pins this.
   - `PackFile.save` sorts entries by id ascending, writes `id=name\n` lines with trailing `\n`, creates `<srcDir>/pack/` recursively if absent (re-traced from `PackFileBase.ts:113-124`).
   - `ListDir` returns `<path>/<entry>` paths and includes subdir entries (suffixed `/`) AND their recursive contents (re-traced from `FsCache.ts:32-66`).
5. **mtime granularity** — TS `fs.Stats.mtimeMs` is float64 ms; Go's `os.FileInfo.ModTime().UnixMilli()` is int64 ms. Plan uses `int64` throughout (TS `> latest` comparison stays correct: integer ordering matches).
6. **Concurrency posture** — `FsCache`'s three caches are guarded by a single `sync.RWMutex` (NAI-191-D-CONCURRENCY). No callers exist yet; lock contention is not measured.
7. **No new external deps** — confirmed via the §1 import set.

---

## §1 File map

| Path | Action | Responsibility |
|---|---|---|
| `pkg/pack/fscache.go` | Create | Module-level dir/exists/stat caches (mutex-guarded); `ListDir`, `ListFiles`, `FileExists`, `FileStat`, `ClearFsCache`. |
| `pkg/pack/fscache_test.go` | Create | Cache hit/invalidate; recursion; missing-path returns nil. |
| `pkg/pack/parse.go` | Create | `LoadFile`, `LoadFileFull`, `LoadDir*`, `ListFilesExt`, `LoadDirExt*`, `ReadConfigs`. |
| `pkg/pack/parse_test.go` | Create | Comment-stripping cases incl. TS quirk pins; `ReadConfigs` duplicate error. |
| `pkg/pack/namemap.go` | Create | `LoadOrder`, `LoadPack`, `LoadDir`, `LoadDirExact`. |
| `pkg/pack/namemap_test.go` | Create | Sparse array preservation; `LoadDirExact` does NOT filter empties. |
| `pkg/pack/freshness.go` | Create | `GetModified`, `GetLatestModified`, `ShouldBuild`, `ShouldBuildFile`, `ShouldBuildFileAny`. |
| `pkg/pack/freshness_test.go` | Create | mtime ms; missing-out triggers rebuild; recursive `Any`. |
| `pkg/pack/packfile.go` | Create | `PackFile` struct + `NewPackFile`, `Reload`, `Load`, `Save`, `Register`, `Delete`, `DeleteByName`, `RefreshNames`, `GetByID`, `GetByName`, `Clear`, `Size`. |
| `pkg/pack/packfile_test.go` | Create | Load → Save → reload byte-equality; `refreshNames` asymmetry pin; validator path. |
| `pkg/pack/crawl.go` | Create | Generic `CrawlConfigNames`. |
| `pkg/pack/crawl_test.go` | Create | `engine.rs2` skip; dedup; bracket-include toggle. |
| `pkg/pack/testdata/...` | Create | Small fixtures created lazily inside tests (`t.TempDir()`); no committed binary or large text fixtures. |

**Decision: fixtures are written in-test via `t.TempDir()` + `os.WriteFile`.** This keeps the diff diff-readable and avoids `testdata/` becoming a maintenance burden. Each test that needs fixture files writes them itself.

---

## §2 Task overview

Six tasks, one per production file, in dependency order:

| # | File | Depends on |
|---|---|---|
| T1 | `fscache.go` | — |
| T2 | `parse.go` | T1 |
| T3 | `namemap.go` | T1 |
| T4 | `freshness.go` | T1 |
| T5 | `packfile.go` | T2 |
| T6 | `crawl.go` | T2 |

Each task follows TDD: write failing test → run to confirm fail → write impl → run to confirm pass → commit.

**Test invocation prefix** (per global CLAUDE.md): `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/...`.

---

## Task 1: `fscache.go` — directory/exists/stat caches

**Files:**
- Create: `pkg/pack/fscache.go`
- Test: `pkg/pack/fscache_test.go`

- [ ] **Step 1.1: Write the failing test**

Create `pkg/pack/fscache_test.go`:

```go
package pack

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestFileExistsMissing(t *testing.T) {
	ClearFsCache()
	if FileExists("/nonexistent/path/should/not/be/here") {
		t.Fatal("expected false for missing path")
	}
}

func TestFileExistsCachesResult(t *testing.T) {
	ClearFsCache()
	dir := t.TempDir()
	p := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(p, []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !FileExists(p) {
		t.Fatal("expected true")
	}
	// Remove file; cached "true" should persist until ClearFsCache.
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
	if !FileExists(p) {
		t.Fatal("expected cached true after removal")
	}
	ClearFsCache()
	if FileExists(p) {
		t.Fatal("expected false after ClearFsCache")
	}
}

func TestListDirMissingReturnsNil(t *testing.T) {
	ClearFsCache()
	got := ListDir("/nonexistent/somewhere")
	if got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
}

func TestListDirRecursesWithSubdirSuffix(t *testing.T) {
	ClearFsCache()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(root, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "b.txt"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	got := ListDir(root)
	want := []string{
		root + "/a.txt",
		root + "/sub/",
		root + "/sub/b.txt",
	}
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestListDirTrailingSlashNormalized(t *testing.T) {
	ClearFsCache()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "x"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	got := ListDir(root + "/")
	want := []string{root + "/x"}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestFileStatCaches(t *testing.T) {
	ClearFsCache()
	root := t.TempDir()
	p := filepath.Join(root, "f")
	if err := os.WriteFile(p, []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	info, err := FileStat(p)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() != 1 {
		t.Fatalf("size=%d", info.Size())
	}
	// Modify file; cached size should persist.
	if err := os.WriteFile(p, []byte("longer"), 0o644); err != nil {
		t.Fatal(err)
	}
	info2, err := FileStat(p)
	if err != nil {
		t.Fatal(err)
	}
	if info2.Size() != 1 {
		t.Fatalf("expected cached size=1, got %d", info2.Size())
	}
}
```

- [ ] **Step 1.2: Run test, confirm fail**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/...
```
Expected: build failure (no `pkg/pack/fscache.go` defines the referenced symbols).

- [ ] **Step 1.3: Write the implementation**

Create `pkg/pack/fscache.go`:

```go
// Package pack ports the source-side packing-pipeline foundation from
// LostCityRS/Engine-TS/tools/pack/*.ts. It provides format-agnostic
// primitives — filesystem caches, source-file parsers, the PackFile
// name↔id registry struct, file-freshness helpers, and a generic
// [header]-block crawler — that downstream per-config packer slices
// (NAI-192+) build on. No production callsites in this slice.
package pack

import (
	"os"
	"strings"
	"sync"
)

// NAI-191-D-CONCURRENCY: TS tools/pack/FsCache.ts uses unguarded
// module-level Maps (single-threaded). Goscape guards the equivalent
// caches with a single RWMutex so the eventual ::rebuild worker
// goroutine can call freely from a non-tick context.
var (
	fsCacheMu   sync.RWMutex
	dirCache    = map[string][]string{}
	existsCache = map[string]bool{}
	statsCache  = map[string]os.FileInfo{}
)

// ClearFsCache resets the memoized dir/exists/stat lookups.
func ClearFsCache() {
	fsCacheMu.Lock()
	defer fsCacheMu.Unlock()
	dirCache = map[string][]string{}
	existsCache = map[string]bool{}
	statsCache = map[string]os.FileInfo{}
}

// FileExists reports whether path exists on disk. Result is memoized
// until ClearFsCache.
func FileExists(path string) bool {
	fsCacheMu.RLock()
	if v, ok := existsCache[path]; ok {
		fsCacheMu.RUnlock()
		return v
	}
	fsCacheMu.RUnlock()

	_, err := os.Stat(path)
	exists := err == nil

	fsCacheMu.Lock()
	existsCache[path] = exists
	fsCacheMu.Unlock()
	return exists
}

// FileStat returns os.FileInfo for path, memoized.
func FileStat(path string) (os.FileInfo, error) {
	fsCacheMu.RLock()
	if v, ok := statsCache[path]; ok {
		fsCacheMu.RUnlock()
		return v, nil
	}
	fsCacheMu.RUnlock()

	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	fsCacheMu.Lock()
	statsCache[path] = info
	fsCacheMu.Unlock()
	return info, nil
}

// ListDir returns all entries (recursive) under path. Subdirectory
// entries are suffixed "/" to match TS tools/pack/FsCache.ts. Returns
// nil for missing paths. Cached entries are the bare directory contents
// (e.g. "a.txt", "sub/"); the returned paths prepend the input path
// (e.g. "<path>/a.txt").
func ListDir(path string) []string {
	path = strings.TrimSuffix(path, "/")

	fsCacheMu.RLock()
	cached, ok := dirCache[path]
	fsCacheMu.RUnlock()

	var files []string
	if ok {
		files = cached
	} else {
		if _, err := os.Stat(path); err != nil {
			return nil
		}
		entries, err := os.ReadDir(path)
		if err != nil {
			return nil
		}
		files = make([]string, 0, len(entries))
		for _, e := range entries {
			if e.IsDir() {
				files = append(files, e.Name()+"/")
			} else {
				files = append(files, e.Name())
			}
		}
		fsCacheMu.Lock()
		dirCache[path] = files
		fsCacheMu.Unlock()
	}

	out := make([]string, 0, len(files))
	for _, f := range files {
		out = append(out, path+"/"+f)
		if strings.HasSuffix(f, "/") {
			out = append(out, ListDir(path+"/"+f)...)
		}
	}
	return out
}

// ListFiles is TS parity for listFiles(path, out=[]). Equivalent to
// ListDir; subdirectory-suffixed entries are included in the output.
func ListFiles(path string) []string {
	return ListDir(path)
}
```

- [ ] **Step 1.4: Run test, confirm pass**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/... -count=1 -race
```
Expected: PASS (6 tests).

- [ ] **Step 1.5: Commit**

```bash
git add pkg/pack/fscache.go pkg/pack/fscache_test.go
git commit --no-gpg-sign -m "feat(pack): NAI-191 T1 — FsCache port (dir/exists/stat caches)"
```

---

## Task 2: `parse.go` — source-file loaders + ReadConfigs

**Files:**
- Create: `pkg/pack/parse.go`
- Test: `pkg/pack/parse_test.go`

- [ ] **Step 2.1: Write the failing test**

Create `pkg/pack/parse_test.go`:

```go
package pack

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestLoadFileMissing(t *testing.T) {
	got := LoadFile("/nonexistent/file.txt")
	if got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
}

func TestLoadFileFullStripsSingleLineComment(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.rs2")
	if err := os.WriteFile(p, []byte("foo  // trailing\nbar"), 0o644); err != nil {
		t.Fatal(err)
	}
	ClearFsCache()
	lines, err := LoadFileFull(p)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"foo", "bar"}
	if !slices.Equal(lines, want) {
		t.Fatalf("got %q, want %q", lines, want)
	}
}

func TestLoadFileFullStripsSameLineMultiComment(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.rs2")
	if err := os.WriteFile(p, []byte("a/* in */b"), 0o644); err != nil {
		t.Fatal(err)
	}
	ClearFsCache()
	lines, err := LoadFileFull(p)
	if err != nil {
		t.Fatal(err)
	}
	// TS substring(0,1)+substring(idx+2) = "a" + "b" = "ab"
	want := []string{"ab"}
	if !slices.Equal(lines, want) {
		t.Fatalf("got %q, want %q", lines, want)
	}
}

func TestLoadFileFullStripsMultiLineComment(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.rs2")
	if err := os.WriteFile(p, []byte("first\n/* multi\nline */\nlast"), 0o644); err != nil {
		t.Fatal(err)
	}
	ClearFsCache()
	lines, err := LoadFileFull(p)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"first", "last"}
	if !slices.Equal(lines, want) {
		t.Fatalf("got %q, want %q", lines, want)
	}
}

func TestLoadFileFullTSQuirkDoubleStarOnOneLine(t *testing.T) {
	// TS-parity pin per spec §3.6: the outer block strips ONLY the
	// first /* */ pair; a trailing */ literal survives in the output
	// WITH its leading space (TS does no post-substring trim).
	//
	// Input:  "/* /* */ */"  (11 chars; first "*/" at idx 6)
	// TS:     line.substring(0,0) + line.substring(8) = "" + " */"
	// Result pushed verbatim: " */"
	dir := t.TempDir()
	p := filepath.Join(dir, "f.rs2")
	if err := os.WriteFile(p, []byte("/* /* */ */"), 0o644); err != nil {
		t.Fatal(err)
	}
	ClearFsCache()
	lines, err := LoadFileFull(p)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{" */"}
	if !slices.Equal(lines, want) {
		t.Fatalf("got %q, want %q", lines, want)
	}
}

func TestLoadFileFullUnclosedCommentErrors(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.rs2")
	if err := os.WriteFile(p, []byte("ok\n/* never closes\nstill open"), 0o644); err != nil {
		t.Fatal(err)
	}
	ClearFsCache()
	_, err := LoadFileFull(p)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "unclosed multi-line comment") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(err.Error(), "line 2") {
		t.Fatalf("expected error to name line 2, got: %v", err)
	}
}

func TestReadConfigsAggregatesAcrossFiles(t *testing.T) {
	dir := t.TempDir()
	scripts := filepath.Join(dir, "scripts")
	if err := os.MkdirAll(scripts, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(scripts, "a.obj"),
		[]byte("[coins]\nmodel=coins_obj\n[bronze_dagger]\nmodel=bd"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(scripts, "b.obj"),
		[]byte("[oak_log]\nmodel=ol"), 0o644); err != nil {
		t.Fatal(err)
	}
	ClearFsCache()
	cfg, err := ReadConfigs(dir, ".obj")
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg) != 3 {
		t.Fatalf("got %d configs, want 3: %v", len(cfg), cfg)
	}
	if !slices.Equal(cfg["coins"], []string{"model=coins_obj"}) {
		t.Fatalf("coins=%v", cfg["coins"])
	}
}

func TestReadConfigsDuplicateErrors(t *testing.T) {
	dir := t.TempDir()
	scripts := filepath.Join(dir, "scripts")
	if err := os.MkdirAll(scripts, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(scripts, "dup.obj"),
		[]byte("[coins]\nmodel=a\n[coins]\nmodel=b"), 0o644); err != nil {
		t.Fatal(err)
	}
	ClearFsCache()
	_, err := ReadConfigs(dir, ".obj")
	if err == nil {
		t.Fatal("expected duplicate error")
	}
	if !strings.Contains(err.Error(), "duplicate config") || !strings.Contains(err.Error(), "coins") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestListFilesExtFiltersByExtension(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a.obj", "b.npc", "c.obj", "d.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	ClearFsCache()
	got := ListFilesExt(dir, ".obj")
	want := []string{dir + "/a.obj", dir + "/c.obj"}
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}
```

- [ ] **Step 2.2: Run test, confirm fail**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/... -run TestLoadFile -count=1
```
Expected: build failure.

- [ ] **Step 2.3: Write the implementation**

Create `pkg/pack/parse.go`:

```go
package pack

import (
	"fmt"
	"os"
	"strings"
)

// LoadDirCallback is the per-file callback for LoadDir / LoadDirFull /
// LoadDirExt / LoadDirExtFull.
type LoadDirCallback func(lines []string, file string)

// LoadFile returns the file's lines (split on \r?\n), or nil if the
// path is missing.
func LoadFile(path string) []string {
	if !FileExists(path) {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return splitLinesCRLF(string(data))
}

// LoadFileFull returns LoadFile's output with single-line ("//") and
// multi-line ("/* */") comments stripped using TS Parse.ts:loadFileFull
// counter semantics. Returns an error if a /* is never closed.
//
// TS-parity quirk pinned in tests: the outer block (counter==0) strips
// ONLY the first /* */ pair on a line; trailing */ tokens survive into
// the output. See spec §3.6.
func LoadFileFull(path string) ([]string, error) {
	text := LoadFile(path)
	lines := make([]string, 0, len(text))
	multiCommentStart := 0
	multiLineComments := 0

	for i := 0; i < len(text); i++ {
		line := strings.TrimSpace(text[i])

		if multiLineComments > 0 {
			// Already inside multi-line comment: walk all /* and */ on
			// this line, incrementing / decrementing the counter.
			for {
				idx := strings.Index(line, "/*")
				if idx == -1 {
					break
				}
				line = strings.TrimLeft(line[idx+2:], " \t")
				multiLineComments++
			}
			for multiLineComments > 0 {
				idx := strings.Index(line, "*/")
				if idx == -1 {
					break
				}
				line = strings.TrimLeft(line[idx+2:], " \t")
				multiLineComments--
			}
			if multiLineComments > 0 {
				continue
			}
		}

		if len(line) == 0 {
			continue
		}

		// Single-line // comment.
		if c := strings.Index(line, "//"); c != -1 {
			line = strings.TrimRight(line[:c], " \t")
			if len(line) == 0 {
				continue
			}
		}

		// Multi-line /* */ first-entry handling.
		commentStart := strings.Index(line, "/*")
		commentEnd := strings.Index(line, "*/")
		if commentStart != -1 {
			if commentEnd != -1 {
				line = line[:commentStart] + line[commentEnd+2:]
			} else {
				line = line[:commentStart]
				multiLineComments++
				if multiCommentStart == 0 {
					multiCommentStart = i + 1
				}
			}
			if len(line) == 0 {
				continue
			}
		}

		lines = append(lines, line)
	}

	if multiLineComments > 0 {
		return nil, fmt.Errorf("%s has an unclosed multi-line comment starting at line %d", path, multiCommentStart)
	}
	return lines, nil
}

// LoadDir invokes cb(lines, basename) for every file under path
// (recursive). Subdirectory entries (suffixed "/") are skipped.
func LoadDir(path string, cb LoadDirCallback) {
	for _, f := range ListFiles(path) {
		if strings.HasSuffix(f, "/") {
			continue
		}
		base := f[strings.LastIndex(f, "/")+1:]
		cb(LoadFile(f), base)
	}
}

// LoadDirFull is LoadDir but with comment stripping. Returns the first
// LoadFileFull error and halts the walk (TS throws synchronously).
func LoadDirFull(path string, cb LoadDirCallback) error {
	for _, f := range ListFiles(path) {
		if strings.HasSuffix(f, "/") {
			continue
		}
		lines, err := LoadFileFull(f)
		if err != nil {
			return err
		}
		base := f[strings.LastIndex(f, "/")+1:]
		cb(lines, base)
	}
	return nil
}

// ListFilesExt returns all files (recursive) under path with the given
// extension. Returns nil for missing paths.
func ListFilesExt(path, ext string) []string {
	if !FileExists(path) {
		return nil
	}
	all := ListDir(path)
	out := make([]string, 0, len(all))
	for _, f := range all {
		if strings.HasSuffix(f, ext) {
			out = append(out, f)
		}
	}
	return out
}

// LoadDirExt is LoadDir filtered by extension. The callback receives
// the FULL path as the file argument (TS parity for this overload).
func LoadDirExt(path, ext string, cb LoadDirCallback) {
	for _, f := range ListFilesExt(path, ext) {
		cb(LoadFile(f), f)
	}
}

// LoadDirExtFull is LoadDirExt with comment stripping. Halts on first
// LoadFileFull error.
func LoadDirExtFull(path, ext string, cb LoadDirCallback) error {
	for _, f := range ListFilesExt(path, ext) {
		lines, err := LoadFileFull(f)
		if err != nil {
			return err
		}
		cb(lines, f)
	}
	return nil
}

// ReadConfigs walks <srcDir>/scripts/*.<ext>, splits each file into
// [header]-delimited config blocks, and returns the aggregated
// map[header] = lines. Returns an error on duplicate header keys or on
// any unclosed multi-line comment.
func ReadConfigs(srcDir, ext string) (map[string][]string, error) {
	configs := map[string][]string{}
	var outerErr error

	err := LoadDirExtFull(srcDir+"/scripts", ext, func(lines []string, file string) {
		if outerErr != nil {
			return
		}
		current := ""
		var block []string
		for _, line := range lines {
			if strings.HasPrefix(line, "[") {
				if current != "" {
					if _, dup := configs[current]; dup {
						outerErr = fmt.Errorf("duplicate config found in %s: %s", file, current)
						return
					}
					configs[current] = block
				}
				current = line[1 : len(line)-1]
				block = nil
				continue
			}
			block = append(block, line)
		}
		if current != "" {
			if _, dup := configs[current]; dup {
				outerErr = fmt.Errorf("duplicate config found in %s: %s", file, current)
				return
			}
			configs[current] = block
		}
	})
	if err != nil {
		return nil, err
	}
	if outerErr != nil {
		return nil, outerErr
	}
	return configs, nil
}

func splitLinesCRLF(s string) []string {
	// TS splits on /\r?\n/. strings.Split on "\n" then trim trailing \r
	// matches.
	raw := strings.Split(s, "\n")
	for i, line := range raw {
		raw[i] = strings.TrimSuffix(line, "\r")
	}
	return raw
}
```

- [ ] **Step 2.4: Run test, confirm pass**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/... -count=1 -race
```
Expected: PASS (all parse tests + T1's fscache tests).

- [ ] **Step 2.5: Commit**

```bash
git add pkg/pack/parse.go pkg/pack/parse_test.go
git commit --no-gpg-sign -m "feat(pack): NAI-191 T2 — Parse port (loaders + ReadConfigs)"
```

---

## Task 3: `namemap.go` — sparse pack/order readers

**Files:**
- Create: `pkg/pack/namemap.go`
- Test: `pkg/pack/namemap_test.go`

- [ ] **Step 3.1: Write the failing test**

Create `pkg/pack/namemap_test.go`:

```go
package pack

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestLoadOrderNumericLines(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "x.order")
	if err := os.WriteFile(p, []byte("0\n5\n\n12\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := LoadOrder(p)
	want := []int{0, 5, 12}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestLoadOrderMissing(t *testing.T) {
	if got := LoadOrder("/nonexistent.order"); got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
}

func TestLoadPackSparseArray(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "x.pack")
	if err := os.WriteFile(p, []byte("0=alpha\n3=delta\n5=epsilon"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := LoadPack(p)
	if len(got) < 6 {
		t.Fatalf("len=%d, want ≥6 (sparse)", len(got))
	}
	if got[0] != "alpha" || got[3] != "delta" || got[5] != "epsilon" {
		t.Fatalf("indexed values wrong: %v", got)
	}
	if got[1] != "" || got[2] != "" || got[4] != "" {
		t.Fatalf("gaps not preserved as empty: %v", got)
	}
}

func TestLoadDirExactDoesNotFilterEmpties(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "x.txt"), []byte("a\n\nb\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ClearFsCache()
	var got [][]string
	LoadDirExact(dir, ".txt", func(src []string, _, _ string) {
		got = append(got, src)
	})
	if len(got) != 1 {
		t.Fatalf("expected 1 file, got %d", len(got))
	}
	// TS LoadDirExact does NOT filter empties; should see ["a","","b",""] (trailing newline → empty).
	if len(got[0]) != 4 {
		t.Fatalf("expected 4 lines (incl empties), got %d: %v", len(got[0]), got[0])
	}
}

func TestNameMapLoadDirFiltersEmpties(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "x.txt"), []byte("a\n\nb\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ClearFsCache()
	var got [][]string
	NameMapLoadDir(dir, ".txt", func(src []string, _, _ string) {
		got = append(got, src)
	})
	if len(got) != 1 || len(got[0]) != 2 {
		t.Fatalf("expected 1 file with 2 non-empty lines, got %v", got)
	}
}
```

- [ ] **Step 3.2: Run test, confirm fail**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/... -run TestLoad -count=1
```
Expected: build failure for `LoadOrder`, `LoadPack`, `LoadDirExact`, `NameMapLoadDir`.

- [ ] **Step 3.3: Write the implementation**

Create `pkg/pack/namemap.go`:

```go
package pack

import (
	"os"
	"strconv"
	"strings"
)

// NameMapCallback is the per-file callback for LoadDirExact /
// NameMapLoadDir. It receives the raw line slice, the file basename,
// and the parent directory path.
type NameMapCallback func(src []string, file, path string)

// LoadOrder reads numeric-only lines from path (one int per line,
// blank lines filtered). Returns nil for missing path.
//
// TS source: tools/pack/NameMap.ts:loadOrder.
func LoadOrder(path string) []int {
	if !FileExists(path) {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	out := []int{}
	for _, line := range splitLinesCRLF(string(data)) {
		if line == "" {
			continue
		}
		n, err := strconv.Atoi(line)
		if err != nil {
			continue
		}
		out = append(out, n)
	}
	return out
}

// LoadPack reads "id=name" lines into a sparse string slice indexed by
// id. Gaps in id space are represented by empty strings. Returns nil
// for missing path.
//
// TS source: tools/pack/NameMap.ts:loadPack.
func LoadPack(path string) []string {
	if !FileExists(path) {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	out := []string{}
	for _, line := range splitLinesCRLF(string(data)) {
		if line == "" {
			continue
		}
		eq := strings.IndexByte(line, '=')
		if eq < 0 {
			continue
		}
		id, err := strconv.Atoi(line[:eq])
		if err != nil {
			continue
		}
		for len(out) <= id {
			out = append(out, "")
		}
		out[id] = line[eq+1:]
	}
	return out
}

// NameMapLoadDir invokes cb for each file under path with the given
// extension. Empty lines are filtered out before the callback.
//
// TS source: tools/pack/NameMap.ts:loadDir. Renamed in goscape to
// disambiguate from Parse.LoadDir (which has different callback shape).
func NameMapLoadDir(path, ext string, cb NameMapCallback) {
	for _, f := range ListDir(path) {
		if strings.HasSuffix(f, "/") {
			continue
		}
		if !strings.HasSuffix(f, ext) {
			continue
		}
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		lines := splitLinesCRLF(string(data))
		filtered := make([]string, 0, len(lines))
		for _, l := range lines {
			if l != "" {
				filtered = append(filtered, l)
			}
		}
		slash := strings.LastIndexByte(f, '/')
		cb(filtered, f[slash+1:], f[:slash])
	}
}

// LoadDirExact is NameMapLoadDir but does NOT filter empty lines
// (TS-parity per spec §8.1 namemap_test (d)).
//
// TS source: tools/pack/NameMap.ts:loadDirExact.
func LoadDirExact(path, ext string, cb NameMapCallback) {
	for _, f := range ListDir(path) {
		if strings.HasSuffix(f, "/") {
			continue
		}
		if !strings.HasSuffix(f, ext) {
			continue
		}
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		slash := strings.LastIndexByte(f, '/')
		cb(splitLinesCRLF(string(data)), f[slash+1:], f[:slash])
	}
}
```

- [ ] **Step 3.4: Run test, confirm pass**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/... -count=1 -race
```
Expected: PASS.

- [ ] **Step 3.5: Commit**

```bash
git add pkg/pack/namemap.go pkg/pack/namemap_test.go
git commit --no-gpg-sign -m "feat(pack): NAI-191 T3 — NameMap port (LoadOrder/LoadPack/LoadDirExact)"
```

---

## Task 4: `freshness.go` — mtime helpers

**Files:**
- Create: `pkg/pack/freshness.go`
- Test: `pkg/pack/freshness_test.go`

- [ ] **Step 4.1: Write the failing test**

Create `pkg/pack/freshness_test.go`:

```go
package pack

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestGetModifiedMissingReturnsZero(t *testing.T) {
	ClearFsCache()
	if got := GetModified("/does/not/exist"); got != 0 {
		t.Fatalf("got %d, want 0", got)
	}
}

func TestGetModifiedReturnsMs(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f")
	if err := os.WriteFile(p, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	ClearFsCache()
	got := GetModified(p)
	info, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	want := info.ModTime().UnixMilli()
	if got != want {
		t.Fatalf("got %d, want %d", got, want)
	}
}

func TestGetLatestModifiedMaxAcrossExtension(t *testing.T) {
	dir := t.TempDir()
	older := filepath.Join(dir, "a.obj")
	newer := filepath.Join(dir, "b.obj")
	if err := os.WriteFile(older, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-1 * time.Hour)
	if err := os.Chtimes(older, old, old); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(newer, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	// Sibling file with a different extension that's newer than both;
	// must be excluded by the extension filter.
	other := filepath.Join(dir, "ignored.txt")
	if err := os.WriteFile(other, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(1 * time.Hour)
	if err := os.Chtimes(other, future, future); err != nil {
		t.Fatal(err)
	}

	ClearFsCache()
	got := GetLatestModified(dir, ".obj")
	newerInfo, _ := os.Stat(newer)
	want := newerInfo.ModTime().UnixMilli()
	if got != want {
		t.Fatalf("got %d, want %d (ignored.txt should be excluded)", got, want)
	}
}

func TestShouldBuildMissingOut(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "in.obj"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	ClearFsCache()
	if !ShouldBuild(dir, ".obj", "/nope") {
		t.Fatal("expected true when out missing")
	}
}

func TestShouldBuildOutNewer(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "x.obj")
	if err := os.WriteFile(in, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-1 * time.Hour)
	if err := os.Chtimes(in, old, old); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "o")
	if err := os.WriteFile(out, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	ClearFsCache()
	if ShouldBuild(dir, ".obj", out) {
		t.Fatal("expected false when out newer than all inputs")
	}
}

func TestShouldBuildFileAnyRecurses(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "a", "b"), 0o755); err != nil {
		t.Fatal(err)
	}
	deep := filepath.Join(dir, "a", "b", "deep.txt")
	if err := os.WriteFile(deep, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "out")
	if err := os.WriteFile(out, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-1 * time.Hour)
	if err := os.Chtimes(out, old, old); err != nil {
		t.Fatal(err)
	}
	ClearFsCache()
	if !ShouldBuildFileAny(dir, out) {
		t.Fatal("expected true: deep file newer than out")
	}
}
```

- [ ] **Step 4.2: Run test, confirm fail**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/... -run TestShouldBuild -count=1
```
Expected: build failure.

- [ ] **Step 4.3: Write the implementation**

Create `pkg/pack/freshness.go`:

```go
package pack

import (
	"os"
	"path/filepath"
)

// GetModified returns the mtime of path in milliseconds since epoch,
// or 0 if the path is missing.
//
// TS source: tools/pack/PackFile.ts:getModified.
func GetModified(path string) int64 {
	if !FileExists(path) {
		return 0
	}
	info, err := FileStat(path)
	if err != nil {
		return 0
	}
	return info.ModTime().UnixMilli()
}

// GetLatestModified returns the most-recent mtime (ms) across all
// files under path with the given extension.
//
// TS source: tools/pack/PackFile.ts:getLatestModified.
func GetLatestModified(path, ext string) int64 {
	var latest int64
	for _, f := range ListFilesExt(path, ext) {
		info, err := FileStat(f)
		if err != nil {
			continue
		}
		if m := info.ModTime().UnixMilli(); m > latest {
			latest = m
		}
	}
	return latest
}

// ShouldBuild returns true if out is missing or older than the
// most-recent matching-extension file under srcPath.
//
// TS source: tools/pack/PackFile.ts:shouldBuild.
func ShouldBuild(srcPath, ext, out string) bool {
	if !FileExists(out) {
		return true
	}
	info, err := FileStat(out)
	if err != nil {
		return true
	}
	return info.ModTime().UnixMilli() < GetLatestModified(srcPath, ext)
}

// ShouldBuildFile returns true if dest is missing or older than src.
//
// TS source: tools/pack/PackFile.ts:shouldBuildFile.
func ShouldBuildFile(src, dest string) bool {
	if !FileExists(dest) {
		return true
	}
	destInfo, err := FileStat(dest)
	if err != nil {
		return true
	}
	srcInfo, err := FileStat(src)
	if err != nil {
		return true
	}
	return destInfo.ModTime().UnixMilli() < srcInfo.ModTime().UnixMilli()
}

// ShouldBuildFileAny returns true if dest is missing or older than ANY
// file (recursive) under path.
//
// TS source: tools/pack/PackFile.ts:shouldBuildFileAny.
func ShouldBuildFileAny(path, dest string) bool {
	if !FileExists(dest) {
		return true
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return false
	}
	for _, e := range entries {
		target := filepath.Join(path, e.Name())
		if e.IsDir() {
			if ShouldBuildFileAny(target, dest) {
				return true
			}
		} else {
			if ShouldBuildFile(target, dest) {
				return true
			}
		}
	}
	return false
}
```

- [ ] **Step 4.4: Run test, confirm pass**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/... -count=1 -race
```
Expected: PASS.

- [ ] **Step 4.5: Commit**

```bash
git add pkg/pack/freshness.go pkg/pack/freshness_test.go
git commit --no-gpg-sign -m "feat(pack): NAI-191 T4 — Freshness port (GetModified/ShouldBuild*)"
```

---

## Task 5: `packfile.go` — PackFile struct

**Files:**
- Create: `pkg/pack/packfile.go`
- Test: `pkg/pack/packfile_test.go`

- [ ] **Step 5.1: Write the failing test**

Create `pkg/pack/packfile_test.go`:

```go
package pack

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestNewPackFileMissingNoError(t *testing.T) {
	dir := t.TempDir()
	ClearFsCache()
	pf, err := NewPackFile(dir, "ghost", nil)
	if err != nil {
		t.Fatal(err)
	}
	if pf.Size() != 0 {
		t.Fatalf("expected empty, size=%d", pf.Size())
	}
}

func TestPackFileLoadValid(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "pack"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pack", "obj.pack"),
		[]byte("0=coins\n1=bronze_dagger\n2=oak_log\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ClearFsCache()
	pf, err := NewPackFile(dir, "obj", nil)
	if err != nil {
		t.Fatal(err)
	}
	if pf.Size() != 3 {
		t.Fatalf("size=%d, want 3", pf.Size())
	}
	if pf.GetByID(1) != "bronze_dagger" {
		t.Fatalf("GetByID(1)=%q", pf.GetByID(1))
	}
	if pf.GetByName("coins") != 0 {
		t.Fatalf("GetByName(coins)=%d", pf.GetByName("coins"))
	}
	if pf.Max != 3 {
		t.Fatalf("Max=%d, want 3", pf.Max)
	}
}

func TestPackFileLoadGapMax(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "pack"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pack", "x.pack"),
		[]byte("0=a\n5=b\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ClearFsCache()
	pf, err := NewPackFile(dir, "x", nil)
	if err != nil {
		t.Fatal(err)
	}
	if pf.Max != 6 {
		t.Fatalf("Max=%d, want 6 (max id + 1)", pf.Max)
	}
}

func TestPackFileLoadEmptyNameErrors(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "pack"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pack", "x.pack"),
		[]byte("0=ok\n1=\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ClearFsCache()
	_, err := NewPackFile(dir, "x", nil)
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "empty name") {
		t.Fatalf("expected error to mention 'empty name', got: %v", err)
	}
	if !strings.Contains(msg, "x.pack:2") {
		t.Fatalf("expected error to name x.pack:2, got: %v", err)
	}
}

func TestPackFileRefreshNamesAsymmetry(t *testing.T) {
	// Pin TS PackFileBase.ts:refreshNames behavior (spec §3.7):
	// RefreshNames rebuilds Names + Max from Pack but does NOT rebuild
	// NameToID. NameToID is maintained incrementally by Register/Delete.
	pf := &PackFile{
		Pack:     map[int]string{},
		Names:    map[string]struct{}{},
		NameToID: map[string]int{},
	}
	pf.Register(0, "alpha")
	pf.Register(1, "beta")
	// Manually corrupt NameToID to verify RefreshNames does NOT fix it.
	pf.NameToID["stale"] = 99
	pf.RefreshNames()
	if _, ok := pf.NameToID["stale"]; !ok {
		t.Fatal("RefreshNames must NOT touch NameToID (TS parity)")
	}
	if _, ok := pf.Names["alpha"]; !ok {
		t.Fatal("Names should contain alpha after RefreshNames")
	}
	if pf.Max != 2 {
		t.Fatalf("Max=%d, want 2", pf.Max)
	}
}

func TestPackFileClearEmpties(t *testing.T) {
	pf := &PackFile{
		Pack:     map[int]string{},
		Names:    map[string]struct{}{},
		NameToID: map[string]int{},
	}
	pf.Register(0, "alpha")
	pf.RefreshNames()
	pf.Clear()
	if len(pf.Pack) != 0 || len(pf.Names) != 0 || len(pf.NameToID) != 0 || pf.Max != 0 {
		t.Fatalf("Clear left state: pack=%v names=%v nameToId=%v max=%d",
			pf.Pack, pf.Names, pf.NameToID, pf.Max)
	}
}

func TestPackFileSaveRoundTrip(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "pack"), 0o755); err != nil {
		t.Fatal(err)
	}
	original := []byte("0=alpha\n2=gamma\n5=epsilon\n")
	if err := os.WriteFile(filepath.Join(dir, "pack", "y.pack"), original, 0o644); err != nil {
		t.Fatal(err)
	}
	ClearFsCache()
	pf, err := NewPackFile(dir, "y", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := pf.Save(); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "pack", "y.pack"))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, original) {
		t.Fatalf("round-trip mismatch:\nwant %q\ngot  %q", original, got)
	}
}

func TestPackFileSaveCreatesPackDir(t *testing.T) {
	dir := t.TempDir()
	ClearFsCache()
	pf, err := NewPackFile(dir, "z", nil)
	if err != nil {
		t.Fatal(err)
	}
	pf.Register(0, "hello")
	pf.RefreshNames()
	if err := pf.Save(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "pack", "z.pack")); err != nil {
		t.Fatalf("expected pack dir created: %v", err)
	}
}

func TestPackFileValidatorRunsOnReload(t *testing.T) {
	called := 0
	validator := func(pf *PackFile) error {
		called++
		pf.Register(7, "from_validator")
		pf.RefreshNames()
		return nil
	}
	pf, err := NewPackFile(t.TempDir(), "v", validator)
	if err != nil {
		t.Fatal(err)
	}
	if called != 1 {
		t.Fatalf("validator called %d times, want 1", called)
	}
	if pf.GetByID(7) != "from_validator" {
		t.Fatalf("validator did not populate: %v", pf.Pack)
	}
}

func TestPackFileValidatorErrorSurfaces(t *testing.T) {
	wantErr := errors.New("boom")
	_, err := NewPackFile(t.TempDir(), "v", func(*PackFile) error { return wantErr })
	if !errors.Is(err, wantErr) {
		t.Fatalf("got %v, want %v", err, wantErr)
	}
}

func TestPackFileDeleteRefreshesNames(t *testing.T) {
	pf := &PackFile{
		Pack:     map[int]string{},
		Names:    map[string]struct{}{},
		NameToID: map[string]int{},
	}
	pf.Register(0, "alpha")
	pf.Register(1, "beta")
	pf.RefreshNames()
	pf.Delete(0)
	if pf.GetByID(0) != "" {
		t.Fatal("Delete(0) did not remove from Pack")
	}
	if _, ok := pf.NameToID["alpha"]; ok {
		t.Fatal("Delete(0) did not remove alpha from NameToID")
	}
	if _, ok := pf.Names["alpha"]; ok {
		t.Fatal("Delete(0) did not run RefreshNames (alpha still in Names)")
	}
}

func TestPackFileDeleteByName(t *testing.T) {
	pf := &PackFile{
		Pack:     map[int]string{},
		Names:    map[string]struct{}{},
		NameToID: map[string]int{},
	}
	pf.Register(0, "alpha")
	pf.RefreshNames()
	pf.DeleteByName("alpha")
	if pf.Size() != 0 {
		t.Fatalf("expected empty after DeleteByName, size=%d", pf.Size())
	}
}

func TestPackFileGetByNameMissingReturnsMinusOne(t *testing.T) {
	pf := &PackFile{
		Pack:     map[int]string{},
		Names:    map[string]struct{}{},
		NameToID: map[string]int{},
	}
	if id := pf.GetByName("nope"); id != -1 {
		t.Fatalf("got %d, want -1", id)
	}
}

func TestPackFileGetByIDMissingReturnsEmpty(t *testing.T) {
	pf := &PackFile{
		Pack:     map[int]string{},
		Names:    map[string]struct{}{},
		NameToID: map[string]int{},
	}
	if s := pf.GetByID(42); s != "" {
		t.Fatalf("got %q, want empty", s)
	}
}
```

- [ ] **Step 5.2: Run test, confirm fail**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/... -run TestPackFile -count=1
```
Expected: build failure (PackFile type undefined).

- [ ] **Step 5.3: Write the implementation**

Create `pkg/pack/packfile.go`:

```go
package pack

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Validator is the optional pre-load hook on PackFile. When non-nil,
// PackFile.Reload calls Validator(pf) instead of the default Load
// path. Per-config packer slices (NAI-192+) supply validators.
type Validator func(pf *PackFile) error

// PackFile is a name↔id registry loaded from a .pack source file.
//
// NOT safe for concurrent use; callers must serialize Reload, Load,
// Save, Register, Delete*, RefreshNames.
//
// TS source: tools/pack/PackFileBase.ts.
type PackFile struct {
	Type      string
	SrcDir    string
	Validator Validator
	Pack      map[int]string
	Names     map[string]struct{}
	NameToID  map[string]int
	Max       int
}

var packLineRE = regexp.MustCompile(`^\d+=`)

// NewPackFile constructs a PackFile and immediately Reloads. Errors
// from Reload are surfaced (TS parity is a printError/printFatalError
// mode-switch on parentPort — NAI-191-D-NO-PARENT-PORT).
func NewPackFile(srcDir, packType string, validator Validator) (*PackFile, error) {
	pf := &PackFile{
		Type:      packType,
		SrcDir:    srcDir,
		Validator: validator,
		Pack:      map[int]string{},
		Names:     map[string]struct{}{},
		NameToID:  map[string]int{},
	}
	if err := pf.Reload(); err != nil {
		return nil, err
	}
	return pf, nil
}

// Size returns the number of registered (id, name) entries.
func (pf *PackFile) Size() int { return len(pf.Pack) }

// Reload runs the Validator if non-nil, else Loads the default file
// at <SrcDir>/pack/<Type>.pack.
func (pf *PackFile) Reload() error {
	if pf.Validator != nil {
		return pf.Validator(pf)
	}
	return pf.Load(filepath.Join(pf.SrcDir, "pack", pf.Type+".pack"))
}

// Load reads an "id=name" file. Missing paths are not errors (empty
// registry). Lines that fail the `^\d+=` regex are skipped (TS parity:
// preserves comment/blank tolerance). Lines with empty names return
// an error.
func (pf *PackFile) Load(path string) error {
	pf.Pack = map[int]string{}
	if !FileExists(path) {
		pf.RefreshNames()
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := splitLinesCRLF(string(data))
	for i, line := range lines {
		if len(line) == 0 || !packLineRE.MatchString(line) {
			continue
		}
		eq := strings.IndexByte(line, '=')
		name := line[eq+1:]
		if len(name) == 0 {
			return fmt.Errorf("pack file has an empty name %s:%d", path, i+1)
		}
		id, err := strconv.Atoi(line[:eq])
		if err != nil {
			continue
		}
		pf.Register(id, name)
	}
	pf.RefreshNames()
	return nil
}

// Clear empties all four maps and resets Max.
func (pf *PackFile) Clear() {
	pf.Pack = map[int]string{}
	pf.Names = map[string]struct{}{}
	pf.NameToID = map[string]int{}
	pf.Max = 0
}

// Register inserts (id, name) into Pack and NameToID. Does NOT touch
// Names or Max — call RefreshNames if needed.
func (pf *PackFile) Register(id int, name string) {
	pf.Pack[id] = name
	pf.NameToID[name] = id
}

// Delete removes the entry at id and calls RefreshNames. No-op if id
// is absent.
func (pf *PackFile) Delete(id int) {
	name, ok := pf.Pack[id]
	if !ok {
		return
	}
	delete(pf.Pack, id)
	delete(pf.NameToID, name)
	pf.RefreshNames()
}

// DeleteByName removes the entry with the given name and calls
// RefreshNames. No-op if name is absent.
func (pf *PackFile) DeleteByName(name string) {
	id, ok := pf.NameToID[name]
	if !ok {
		return
	}
	delete(pf.NameToID, name)
	delete(pf.Pack, id)
	pf.RefreshNames()
}

// RefreshNames rebuilds Names from Pack values and recomputes Max as
// (max id + 1) when Names is non-empty. Does NOT rebuild NameToID
// (TS parity per spec §3.7).
func (pf *PackFile) RefreshNames() {
	pf.Names = make(map[string]struct{}, len(pf.Pack))
	for _, name := range pf.Pack {
		pf.Names[name] = struct{}{}
	}
	if len(pf.Names) == 0 {
		return
	}
	maxID := 0
	for id := range pf.Pack {
		if id > maxID {
			maxID = id
		}
	}
	pf.Max = maxID + 1
}

// Save writes the registry to <SrcDir>/pack/<Type>.pack, sorted by id
// ascending, "id=name\n" form with trailing newline. Creates the pack
// directory recursively if absent.
func (pf *PackFile) Save() error {
	packDir := filepath.Join(pf.SrcDir, "pack")
	if err := os.MkdirAll(packDir, 0o755); err != nil {
		return err
	}
	ids := make([]int, 0, len(pf.Pack))
	for id := range pf.Pack {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	var buf strings.Builder
	for _, id := range ids {
		buf.WriteString(strconv.Itoa(id))
		buf.WriteByte('=')
		buf.WriteString(pf.Pack[id])
		buf.WriteByte('\n')
	}
	return os.WriteFile(filepath.Join(packDir, pf.Type+".pack"), []byte(buf.String()), 0o644)
}

// GetByID returns the name at id, or "" if absent.
func (pf *PackFile) GetByID(id int) string { return pf.Pack[id] }

// GetByName returns the id for name, or -1 if absent.
func (pf *PackFile) GetByName(name string) int {
	if id, ok := pf.NameToID[name]; ok {
		return id
	}
	return -1
}
```

- [ ] **Step 5.4: Run test, confirm pass**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/... -count=1 -race
```
Expected: PASS.

- [ ] **Step 5.5: Commit**

```bash
git add pkg/pack/packfile.go pkg/pack/packfile_test.go
git commit --no-gpg-sign -m "feat(pack): NAI-191 T5 — PackFile struct port"
```

---

## Task 6: `crawl.go` — generic CrawlConfigNames

**Files:**
- Create: `pkg/pack/crawl.go`
- Test: `pkg/pack/crawl_test.go`

- [ ] **Step 6.1: Write the failing test**

Create `pkg/pack/crawl_test.go`:

```go
package pack

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func writeScript(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCrawlConfigNamesReturnsHeadersInOrder(t *testing.T) {
	srcDir := t.TempDir()
	scripts := filepath.Join(srcDir, "scripts")
	writeScript(t, scripts, "a.obj", "[coins]\nmodel=c\n[bronze_dagger]\nmodel=bd")
	ClearFsCache()
	got, err := CrawlConfigNames(srcDir, ".obj", false)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"coins", "bronze_dagger"}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestCrawlConfigNamesDedups(t *testing.T) {
	srcDir := t.TempDir()
	scripts := filepath.Join(srcDir, "scripts")
	writeScript(t, scripts, "a.obj", "[coins]\nmodel=a")
	writeScript(t, scripts, "b.obj", "[coins]\nmodel=b\n[oak_log]\nmodel=o")
	ClearFsCache()
	got, err := CrawlConfigNames(srcDir, ".obj", false)
	if err != nil {
		t.Fatal(err)
	}
	// "coins" appears once; "oak_log" follows.
	if len(got) != 2 {
		t.Fatalf("got %d names (want 2): %v", len(got), got)
	}
	if !slices.Contains(got, "coins") || !slices.Contains(got, "oak_log") {
		t.Fatalf("missing names: %v", got)
	}
}

func TestCrawlConfigNamesSkipsEngineRs2(t *testing.T) {
	srcDir := t.TempDir()
	scripts := filepath.Join(srcDir, "scripts")
	writeScript(t, scripts, "engine.rs2", "[command_signature]\nfoo=bar")
	writeScript(t, scripts, "real.rs2", "[real_proc]\nfoo=bar")
	ClearFsCache()
	got, err := CrawlConfigNames(srcDir, ".rs2", false)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"real_proc"}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v (engine.rs2 must be skipped)", got, want)
	}
}

func TestCrawlConfigNamesIncludeBrackets(t *testing.T) {
	srcDir := t.TempDir()
	scripts := filepath.Join(srcDir, "scripts")
	writeScript(t, scripts, "a.obj", "[coins]\nmodel=c")
	ClearFsCache()
	got, err := CrawlConfigNames(srcDir, ".obj", true)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"[coins]"}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}
```

- [ ] **Step 6.2: Run test, confirm fail**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/... -run TestCrawl -count=1
```
Expected: build failure.

- [ ] **Step 6.3: Write the implementation**

Create `pkg/pack/crawl.go`:

```go
package pack

import (
	"path/filepath"
	"slices"
	"strings"
)

// CrawlConfigNames walks <srcDir>/scripts recursively, parses every
// file with the given extension, and returns the unique set of
// [header] tokens found at the start of any line.
//
// If includeBrackets is true, returned names include the surrounding
// square brackets ("[name]"); otherwise just "name".
//
// The single TS-specific exclusion is <srcDir>/scripts/engine.rs2 —
// this file holds compiler type signatures for built-in commands and
// must NOT be treated as a packable config.
//
// NAI-191-D-VALIDATE-FLAGS-DEFERRED: TS BUILD_VERIFY_FOLDER also
// enforces directory-structure rules (configs must live under
// configs/, scripts under scripts/). This validator-side check defers
// to NAI-192+ alongside the env-flag plumbing.
//
// TS source: tools/pack/PackFile.ts:crawlConfigNames.
func CrawlConfigNames(srcDir, ext string, includeBrackets bool) ([]string, error) {
	enginePath := filepath.Join(srcDir, "scripts", "engine.rs2")
	var names []string
	err := LoadDirExtFull(filepath.Join(srcDir, "scripts"), ext, func(lines []string, file string) {
		if file == enginePath {
			return
		}
		for _, line := range lines {
			if !strings.HasPrefix(line, "[") {
				continue
			}
			end := strings.Index(line, "]")
			if end < 0 {
				continue
			}
			name := line[:end+1]
			if !includeBrackets {
				name = name[1 : len(name)-1]
			}
			if !slices.Contains(names, name) {
				names = append(names, name)
			}
		}
	})
	if err != nil {
		return nil, err
	}
	return names, nil
}
```

- [ ] **Step 6.4: Run test, confirm pass**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/... -count=1 -race
```
Expected: PASS (all six tasks' tests).

- [ ] **Step 6.5: Final build check**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
```
Expected: success.

- [ ] **Step 6.6: Verify no production callers**

```
rg "github.com/zsrv/goscape/pkg/pack" modules/ cmd/
```
Expected: zero matches. (Foundation slice ships library-only.)

- [ ] **Step 6.7: Commit**

```bash
git add pkg/pack/crawl.go pkg/pack/crawl_test.go
git commit --no-gpg-sign -m "feat(pack): NAI-191 T6 — Crawl port (generic CrawlConfigNames)"
```

---

## §3 Self-review checklist (controller, post-implementation)

Run after T6 commits to confirm the slice meets §12 acceptance:

1. [ ] `pkg/pack/` contains six production files and six test files per §1 file map.
2. [ ] `go test ./pkg/pack/... -count=1 -race` passes.
3. [ ] `go build ./...` succeeds.
4. [ ] `rg "github.com/zsrv/goscape/pkg/pack" modules/ cmd/` returns zero matches.
5. [ ] All five §7 deviations are documented inline:
   - `NAI-191-D-CONCURRENCY` in `fscache.go`
   - `NAI-191-D-NO-PARENT-PORT` in `packfile.go` (`NewPackFile` doc-comment)
   - `NAI-191-D-NO-GLOBAL-SRCDIR` — covered by struct field on `PackFile` + function args; mention in `packfile.go` package-or-struct doc
   - `NAI-191-D-VALIDATE-FLAGS-DEFERRED` in `crawl.go`
   - `NAI-191-D-LOADFILEFULL-ERRORS-PROPAGATE` in `parse.go` (`LoadDirFull` doc)
6. [ ] No `// TODO`, no `// FIXME`, no panic paths unless TS panics in the same shape.
7. [ ] All TS source-line citations in doc-comments reference the correct `tools/pack/*.ts` file.

---

## §4 Test-coverage cross-check (per memory `plan_test_coverage_crosscheck`)

Spec §8.1 enumerates required test cases. Plan task ↔ test case mapping:

| Spec §8.1 case | Plan task | Test function |
|---|---|---|
| fscache_test (a) `ListDir` missing → nil | T1 | `TestListDirMissingReturnsNil` |
| fscache_test (b) recursion + "/" suffix | T1 | `TestListDirRecursesWithSubdirSuffix` |
| fscache_test (c) cache hit | T1 | `TestFileExistsCachesResult`, `TestFileStatCaches` |
| fscache_test (d) `ClearFsCache` invalidation | T1 | `TestFileExistsCachesResult` (last assertion) |
| parse_test (a) `LoadFile` missing → nil | T2 | `TestLoadFileMissing` |
| parse_test (b) `//` single-line strip | T2 | `TestLoadFileFullStripsSingleLineComment` |
| parse_test (c) `/* */` same-line | T2 | `TestLoadFileFullStripsSameLineMultiComment` |
| parse_test (d) `/* */` multi-line | T2 | `TestLoadFileFullStripsMultiLineComment` |
| parse_test (e) TS quirk pin | T2 | `TestLoadFileFullTSQuirkDoubleStarOnOneLine` |
| parse_test (f) unclosed `/*` error | T2 | `TestLoadFileFullUnclosedCommentErrors` |
| parse_test (g) `ReadConfigs` aggregate | T2 | `TestReadConfigsAggregatesAcrossFiles` |
| parse_test (h) `ReadConfigs` dup error | T2 | `TestReadConfigsDuplicateErrors` |
| parse_test (i) `LoadDirExtFull` propagates error | T2 | Covered by `ReadConfigs` test which calls `LoadDirExtFull` |
| namemap_test (a) numeric lines + empties filtered | T3 | `TestLoadOrderNumericLines` |
| namemap_test (b) sparse `[]string` | T3 | `TestLoadPackSparseArray` |
| namemap_test (c) `LoadDir` ext filter | T3 | `TestNameMapLoadDirFiltersEmpties` |
| namemap_test (d) `LoadDirExact` no empty filter | T3 | `TestLoadDirExactDoesNotFilterEmpties` |
| packfile_test (a) missing → empty | T5 | `TestNewPackFileMissingNoError` |
| packfile_test (b) load 3-line valid | T5 | `TestPackFileLoadValid` |
| packfile_test (c) gap → Max | T5 | `TestPackFileLoadGapMax` |
| packfile_test (d) empty-name error | T5 | `TestPackFileLoadEmptyNameErrors` |
| packfile_test (e) refreshNames asymmetry pin | T5 | `TestPackFileRefreshNamesAsymmetry` |
| packfile_test (f) Clear empties | T5 | `TestPackFileClearEmpties` |
| packfile_test (g) round-trip | T5 | `TestPackFileSaveRoundTrip` |
| packfile_test (h) validator path | T5 | `TestPackFileValidatorRunsOnReload` + `TestPackFileValidatorErrorSurfaces` |
| packfile_test (i) `Delete` flow | T5 | `TestPackFileDeleteRefreshesNames` |
| packfile_test (j) `DeleteByName` | T5 | `TestPackFileDeleteByName` |
| packfile_test (k) `GetByID` missing → "" | T5 | `TestPackFileGetByIDMissingReturnsEmpty` |
| packfile_test (l) `GetByName` missing → -1 | T5 | `TestPackFileGetByNameMissingReturnsMinusOne` |
| packfile_test (m) `Save` creates dir | T5 | `TestPackFileSaveCreatesPackDir` |
| freshness_test (a) `GetModified` missing → 0 | T4 | `TestGetModifiedMissingReturnsZero` |
| freshness_test (b) `GetLatestModified` max | T4 | `TestGetLatestModifiedMaxAcrossExtension` |
| freshness_test (c) `ShouldBuild` cases | T4 | `TestShouldBuildMissingOut`, `TestShouldBuildOutNewer` |
| freshness_test (d) `ShouldBuildFileAny` recursion | T4 | `TestShouldBuildFileAnyRecurses` |
| crawl_test (a) order | T6 | `TestCrawlConfigNamesReturnsHeadersInOrder` |
| crawl_test (b) dedup | T6 | `TestCrawlConfigNamesDedups` |
| crawl_test (c) engine.rs2 skip | T6 | `TestCrawlConfigNamesSkipsEngineRs2` |
| crawl_test (d) bracket toggle | T6 | `TestCrawlConfigNamesIncludeBrackets` |

All spec §8.1 cases mapped to a task. No gaps.

---

## §5 Resume prompt (post-/clear)

> Implementing NAI-191 via subagent-driven-development. Spec: `docs/superpowers/specs/2026-05-13-nai-191-pack-pipeline-foundation-design.md`. Plan: `docs/superpowers/plans/2026-05-13-nai-191-pack-pipeline-foundation.md`. Execute T1 through T6 in order. After T6, verify the §3 self-review checklist and run the §4 cross-check.
