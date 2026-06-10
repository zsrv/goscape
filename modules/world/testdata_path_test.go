package world

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// realCacheDir resolves the project-root data/pack directory.
//
// Tests in modules/world historically used a hard-coded "../../data/pack"
// relative path. That works for a normal `go test ./...` invocation rooted
// in the main checkout, but breaks under git worktrees (e.g. when
// .claude/worktrees/<id>/modules/world/ has no sibling data/ tree).
//
// Resolution strategy:
//  1. Try "../../data/pack" relative to the test's CWD (main checkout case).
//  2. Fall back to `git rev-parse --git-common-dir`. In a worktree, this
//     returns the *main* repo's .git path, whose parent dir is the main
//     repo root that owns data/pack. In a regular checkout it returns
//     ".git" (resolved against the CWD), so the fallback is consistent.
//  3. If neither resolves, return the original relative path so callers
//     emit their usual t.Skipf("data/pack ... unavailable: %v", err) message.
func realCacheDir(t *testing.T) string {
	t.Helper()

	const rel = "../../data/pack"
	if _, err := os.Stat(rel); err == nil {
		return rel
	}

	dir, ok := mainRepoDataPack()
	if !ok {
		// The packed cache is generated output (gitignored): a fresh clone
		// doesn't have it. The tests that resolve a cache dir here exercise
		// Reload against the real cache, so skip rather than fail when it
		// hasn't been built yet.
		t.Skipf("packed game cache not present at %s — build it with `goscape-cli pack` to run these tests", rel)
	}
	return dir
}

// ref245Cache is the 245.2 reference cache (Engine-TS 3c16994c + Content
// cbcfe670, bun-packed via Server245.2-ref worktrees). Tests that decode
// COMPONENT configs (e.g. the Reload suite via LoadComponentTypes) must use
// this cache: the repo's own data/pack may be a stale 244-format pack that
// predates the swappable/activeOverColour component layout.
const ref245Cache = "/home/owner/Code/github.com/LostCityRS/Server245.2-ref/engine/data/pack"

// ref245CacheDir returns ref245Cache, skipping the test when the reference
// checkout is not present on this machine.
func ref245CacheDir(t *testing.T) string {
	t.Helper()
	if _, err := os.Stat(ref245Cache); err != nil {
		t.Skipf("Server245.2-ref cache unavailable: %v", err)
	}
	return ref245Cache
}

// mainRepoDataPack returns the main checkout's data/pack directory by
// asking git for the common-dir (the .git dir shared between the primary
// worktree and any linked worktrees). Result is memoised because the path
// is stable for the lifetime of the test binary and `git rev-parse` is a
// non-trivial fork+exec on the hot path of every reload test.
func mainRepoDataPack() (string, bool) {
	mainRepoOnce.Do(func() {
		out, err := exec.Command("git", "rev-parse", "--git-common-dir").Output()
		if err != nil {
			return
		}
		commonDir := strings.TrimSpace(string(out))
		if commonDir == "" {
			return
		}
		// `git rev-parse --git-common-dir` may return either an absolute
		// path (typical in worktrees) or ".git" (typical in the primary
		// checkout). Make it absolute so the result is independent of the
		// test's CWD.
		if !filepath.IsAbs(commonDir) {
			abs, err := filepath.Abs(commonDir)
			if err != nil {
				return
			}
			commonDir = abs
		}
		// Parent of the .git dir is the main repo root.
		candidate := filepath.Join(filepath.Dir(commonDir), "data", "pack")
		if _, err := os.Stat(candidate); err != nil {
			return
		}
		mainRepoCache = candidate
	})
	return mainRepoCache, mainRepoCache != ""
}

var (
	mainRepoOnce  sync.Once
	mainRepoCache string
)
