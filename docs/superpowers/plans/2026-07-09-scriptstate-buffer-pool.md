# ScriptState Buffer Pool Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Recycle the three fixed-capacity ScriptState buffers (IntStack/StringStack/Frames, ~26 KB per `script.Init`) through a `sync.Pool`, eliminating ~71% of the idle server's allocation churn with observably identical script behavior.

**Architecture:** A `scriptBuffers` bundle pooled in `pkg/script` (new `pool.go`), wired into `Init`; a new exported `script.Release(*ScriptState)` clears the reference-holding buffers and nils the state's slices; `Release` is called at exactly the three terminal dispatch arms in `modules/world`. Everything with read-before-write zero-init semantics (state struct, locals, arrays) stays freshly allocated. Spec: `docs/superpowers/specs/2026-07-09-scriptstate-buffer-pool-design.md`.

**Tech Stack:** Go 1.26 (use `clear()`, `b.Loop()`, range-over-int), `sync.Pool`, existing pkg/script + modules/world test suites.

## Global Constraints

- Repo: `~/Code/github.com/zsrv/goscape`, branch `rev-274` only. Verify with `git branch --show-current` before starting.
- `go` commands: prefix env `GOPATH="${TMPDIR:-/tmp}/go" GOCACHE="${TMPDIR:-/tmp}/go-cache"`; the repo builds with `CGO_ENABLED=0` except `-race` runs, which need `CGO_ENABLED=1`.
- Commits: `git commit --no-gpg-sign`; run `git status --short` first and stage only files this plan names. Pre-existing untracked entries (`.superpowers/`, `goscape`, `audit-274/` if present) are never staged. A sandboxed `git status` may show phantom `/dev/null` dotfiles (`.bashrc`, `.gitconfig`, …) — sandbox mask-mounts, not real files; never stage them.
- Behavior invariance is the hard requirement: observable script behavior must be byte-identical. `Release` may be called ONLY at the three terminal arms named in Task 2 — never in a suspend arm, never on the error-handling early-return paths (a missed release is safe; a premature one aliases a live script).
- Fidelity: TS allocates fresh arrays per init (`Engine-TS/src/engine/script/ScriptRunner.ts:66-119`, `ScriptState.ts:39-146`) and never reuses. This is a documented Go-only perf deviation following the PERF-1/PERF-2 precedent (`docs/PORTING-CLOSED.md` §Performance hotspots).
- Key invariants relied on (from the spec's safety audit — do not "improve" them away): int/string stack reads are SP-guarded pops only; locals and `Arrays` are read-before-write and MUST keep coming from fresh `make`; `*ScriptState` pointer identity is never recycled.

---

### Task 1: Pool + `Release` in `pkg/script`

**Files:**
- Create: `pkg/script/pool.go`
- Create: `pkg/script/pool_test.go`
- Modify: `pkg/script/runner.go:15-43` (`Init`)
- Modify: `pkg/script/state.go` (add unexported `buf` field to `ScriptState`)

**Interfaces:**
- Consumes: existing `StackCapacity` (=1024, `state.go:15`), `FrameCapacity` (`state.go:23-25`), `Frame` (`state.go:251-256`), `ScriptState` fields `IntStack []int` / `StringStack []string` / `Frames []Frame` / `ISP`/`SSP`/`FrameSP`, `PushInt`/`PushString`/`PopInt`/`PopString` (`state.go:483-527`).
- Produces (Task 2 relies on): `func Release(s *ScriptState)` — no-op on `nil` state, un-pooled state, or double release; after it returns, `s.IntStack`, `s.StringStack`, `s.Frames` are nil.

- [ ] **Step 1: Write the failing tests**

Create `pkg/script/pool_test.go`:

```go
package script

import "testing"

// newPoolTestScript builds a minimal one-instruction script (immediate
// RETURN) with a few locals declared, mirroring the fixture idiom of
// modules/world/world_script_queue_test.go.
func newPoolTestScript() *ScriptFile {
	return &ScriptFile{
		Name:             "pool_test",
		Opcodes:          []Opcode{OpReturn},
		IntOperands:      []int32{0},
		StringOperands:   []string{""},
		InstructionCount: 1,
		IntLocalCount:    4,
		StringLocalCount: 2,
	}
}

func frameIsZero(f Frame) bool {
	return f.Script == nil && f.PC == 0 && f.IntLocals == nil && f.StringLocals == nil
}

func TestReleaseClearsAndNils(t *testing.T) {
	s := Init(newPoolTestScript(), nil, false, []int{7}, []string{"a"})
	s.PushInt(42)
	s.PushString("secret")
	s.Frames[0] = Frame{Script: s.Script, PC: 9, IntLocals: []int{1}}
	b := s.buf
	if b == nil {
		t.Fatal("Init did not attach pooled buffers (s.buf == nil)")
	}

	Release(s)

	if s.IntStack != nil || s.StringStack != nil || s.Frames != nil || s.buf != nil {
		t.Fatal("Release must nil the state's slice fields and buf")
	}
	for i, v := range b.stringStack {
		if v != "" {
			t.Fatalf("stringStack[%d] not cleared: %q", i, v)
		}
	}
	for i, f := range b.frames {
		if !frameIsZero(f) {
			t.Fatalf("frames[%d] not cleared: %+v", i, f)
		}
	}

	Release(s)   // double release must be a no-op
	Release(nil) // nil must be a no-op
}

func TestReleaseNoopOnUnpooledState(t *testing.T) {
	// Literal &ScriptState{} fixtures exist throughout the test suites;
	// Release must tolerate them.
	s := &ScriptState{}
	Release(s) // must not panic
}

func TestInitReusesReleasedBuffers(t *testing.T) {
	// sync.Pool gives no hard reuse guarantee (a background GC empties
	// it), so accept reuse on ANY of 5 attempts; all-5 misses means the
	// pool wiring is broken.
	reused := false
	for range 5 {
		s1 := Init(newPoolTestScript(), nil, false, nil, nil)
		p := &s1.IntStack[0]
		Release(s1)
		s2 := Init(newPoolTestScript(), nil, false, nil, nil)
		if &s2.IntStack[0] == p {
			reused = true
		}
		Release(s2)
	}
	if !reused {
		t.Fatal("no buffer reuse observed across 5 Release→Init cycles")
	}
}

// The spec's stale-leak pin: nothing from a released run may be observable
// in a state built on the recycled buffers.
func TestRecycledStateLeaksNothing(t *testing.T) {
	s1 := Init(newPoolTestScript(), nil, false, []int{111, 222}, []string{"stale-local"})
	s1.PushInt(31337)
	s1.PushString("stale-stack")
	s1.Frames[0] = Frame{Script: s1.Script, PC: 77, IntLocals: []int{9}}
	Release(s1)

	s2 := Init(newPoolTestScript(), nil, false, nil, nil)
	defer Release(s2)

	if got := s2.PopInt(); got != 0 {
		t.Errorf("empty-stack PopInt on recycled state = %d, want 0 (underflow default)", got)
	}
	if got := s2.PopString(); got != "" {
		t.Errorf("empty-stack PopString on recycled state = %q, want \"\"", got)
	}
	for i, v := range s2.IntLocals {
		if v != 0 {
			t.Errorf("IntLocals[%d] = %d, want 0 (locals must be fresh allocations)", i, v)
		}
	}
	for i, v := range s2.StringLocals {
		if v != "" {
			t.Errorf("StringLocals[%d] = %q, want \"\" (locals must be fresh allocations)", i, v)
		}
	}
	if s2.FrameSP != 0 {
		t.Errorf("FrameSP = %d, want 0", s2.FrameSP)
	}
	if !frameIsZero(s2.Frames[0]) {
		t.Errorf("Frames[0] not zero on recycled state: %+v", s2.Frames[0])
	}
}

func BenchmarkInitRelease(b *testing.B) {
	sf := newPoolTestScript()
	b.ReportAllocs()
	for b.Loop() {
		Release(Init(sf, nil, false, nil, nil))
	}
}

// Regression gate on the win itself: pre-pool Init allocates ~26.5 KB/op
// (two 1024-cap stacks + 50 frames). Post-pool steady state must be under
// 4 KiB/op (struct + locals only). Generous threshold — not flaky.
func TestInitReleaseAllocBytes(t *testing.T) {
	if testing.Short() {
		t.Skip("benchmark-backed gate; skipped in -short")
	}
	res := testing.Benchmark(BenchmarkInitRelease)
	if bpo := res.AllocedBytesPerOp(); bpo > 4096 {
		t.Fatalf("Init+Release allocates %d B/op, want ≤4096 (buffer-pool regression; pre-pool ≈26500)", bpo)
	}
}
```

Field-name notes (verified against rev-274 source): `ScriptFile{Name, Opcodes, IntOperands, StringOperands, InstructionCount, IntLocalCount, StringLocalCount}` all exist (`pkg/script/file.go:14-26` + locals counts used at `runner.go:24-25`); `OpReturn` is the idiom used by `modules/world/world_script_queue_test.go:16-26`. `Frame` is NOT comparable (holds slices) — hence `frameIsZero`, not `f != (Frame{})`.

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd ~/Code/github.com/zsrv/goscape
GOPATH="${TMPDIR:-/tmp}/go" GOCACHE="${TMPDIR:-/tmp}/go-cache" go test ./pkg/script/ -run 'TestRelease|TestInitReuses|TestRecycled|TestInitRelease' -v
```

Expected: FAIL to build — `undefined: Release`, `s.buf undefined`.

- [ ] **Step 3: Create `pkg/script/pool.go`**

```go
package script

import "sync"

// scriptBuffers bundles the three fixed-capacity buffers every ScriptState
// carries, pooled as one unit (PERF-3). Profiling on the idle singleplayer
// server (2026-07-09) attributed 71% of allocation churn to Init allocating
// these fresh per run (~26 KB: 8 KB IntStack + 16 KB StringStack + Frames).
//
// TS allocates fresh arrays per init (ScriptRunner.ts:66-119,
// ScriptState.ts:39-146) and never reuses them; pooling is a Go-only
// optimization. It is behavior-invisible because stack reads are strictly
// SP-disciplined — PopInt/PopString (state.go) only read slots pushed
// earlier in the same run — so stale values in a recycled intStack are
// unobservable. Everything with read-before-write zero-init semantics
// (IntLocals, StringLocals, Arrays, the state struct itself) is NOT pooled
// and keeps coming from fresh make/new. See
// docs/superpowers/specs/2026-07-09-scriptstate-buffer-pool-design.md.
type scriptBuffers struct {
	intStack    []int
	stringStack []string
	frames      []Frame
}

var buffersPool = sync.Pool{
	New: func() any {
		return &scriptBuffers{
			intStack:    make([]int, StackCapacity),
			stringStack: make([]string, StackCapacity),
			frames:      make([]Frame, FrameCapacity),
		}
	},
}

// Release recycles s's stack buffers into the pool. Call ONLY when s is
// provably unreferenced: the Finished/Aborted dispatch arms in
// modules/world, after OnScriptFinishedOrAborted's pointer-identity guard
// has cleared any matching activeScript. Never call it on a suspended
// state — its buffers stay live (held via Player/Npc activeScript or the
// world script queue) until it terminates through those same arms. A missed
// Release is safe: the buffers just fall to the GC as before pooling.
//
// stringStack and frames are cleared before pooling so recycled bundles pin
// no string/*ScriptFile/locals references; intStack is intentionally left
// dirty (slots above SP are never read). The state's slice fields and buf
// are nil'd so use-after-release panics on a nil slice instead of silently
// aliasing a recycled buffer, and so a double Release is a no-op.
func Release(s *ScriptState) {
	if s == nil || s.buf == nil {
		return
	}
	b := s.buf
	clear(b.stringStack)
	clear(b.frames)
	s.IntStack = nil
	s.StringStack = nil
	s.Frames = nil
	s.buf = nil
	buffersPool.Put(b)
}
```

- [ ] **Step 4: Wire the pool into `Init` and add the `buf` field**

In `pkg/script/state.go`, add to the `ScriptState` struct (place it with the
other unexported fields at the bottom of the struct; if the struct has no
unexported fields yet, add it last):

```go
	// buf is the pooled backing bundle for IntStack/StringStack/Frames
	// (PERF-3, pool.go). Nil for states not built by Init (literal
	// &ScriptState{} test fixtures) and after Release.
	buf *scriptBuffers
```

In `pkg/script/runner.go`, `Init` currently reads (lines 15-30):

```go
func Init(script *ScriptFile, self ActivePlayer, protect bool, intArgs []int, stringArgs []string) *ScriptState {
	s := &ScriptState{
		Script:    script,
		PC:        0,
		Execution: Running,

		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),

		IntLocals:    make([]int, max(int(script.IntLocalCount), len(intArgs))),
		StringLocals: make([]string, max(int(script.StringLocalCount), len(stringArgs))),

		Frames: make([]Frame, FrameCapacity),

		Self: self,
	}
```

Change to (locals stay `make` — read-before-write zero-init lives there):

```go
func Init(script *ScriptFile, self ActivePlayer, protect bool, intArgs []int, stringArgs []string) *ScriptState {
	// PERF-3: the three fixed-capacity buffers come from the pool
	// (pool.go); a fresh-from-New bundle is zeroed by make, a recycled one
	// has stringStack/frames cleared by Release and an intentionally-dirty
	// intStack (unobservable — see pool.go). Locals stay freshly allocated:
	// RuneScript reads them before writing and relies on zero-init.
	b := buffersPool.Get().(*scriptBuffers)
	s := &ScriptState{
		Script:    script,
		PC:        0,
		Execution: Running,

		IntStack:    b.intStack,
		StringStack: b.stringStack,

		IntLocals:    make([]int, max(int(script.IntLocalCount), len(intArgs))),
		StringLocals: make([]string, max(int(script.StringLocalCount), len(stringArgs))),

		Frames: b.frames,

		Self: self,
		buf:  b,
	}
```

The rest of `Init` (arg copies, pointer flags, return) is unchanged.

- [ ] **Step 5: Run the new tests**

Same command as Step 2. Expected: all PASS (including `TestInitReleaseAllocBytes` — if it fails on bytes/op, something still allocates per call; investigate before proceeding, do not raise the threshold).

- [ ] **Step 6: Run the full script package + vet + race**

```bash
cd ~/Code/github.com/zsrv/goscape
GOPATH="${TMPDIR:-/tmp}/go" GOCACHE="${TMPDIR:-/tmp}/go-cache" go test ./pkg/script/... && \
GOPATH="${TMPDIR:-/tmp}/go" GOCACHE="${TMPDIR:-/tmp}/go-cache" go vet ./pkg/script/... && \
GOPATH="${TMPDIR:-/tmp}/go" GOCACHE="${TMPDIR:-/tmp}/go-cache" CGO_ENABLED=1 go test -race ./pkg/script/ -run 'TestRelease|TestInitReuses|TestRecycled' && \
gofmt -l pkg/script
```

Expected: PASS / clean / PASS / no output.

- [ ] **Step 7: Commit**

```bash
cd ~/Code/github.com/zsrv/goscape && git status --short
git add pkg/script/pool.go pkg/script/pool_test.go pkg/script/runner.go pkg/script/state.go
git commit --no-gpg-sign -m "perf(script): pool ScriptState stack buffers (PERF-3); Release seam"
```

---

### Task 2: Release at the three terminal dispatch arms

**Files:**
- Modify: `modules/world/script.go` (`resumeOrFinish` Finished/Aborted arm, ~line 181-185; `resumeOrFinishWorld` Finished/Aborted arm, ~line 301-302)
- Modify: `modules/world/npc_script.go` (`resumeOrFinishNpc` Finished/Aborted arm, ~line 461-464)
- Test: `modules/world/script_pool_release_test.go` (create)

**Interfaces:**
- Consumes: `script.Release(*ScriptState)` from Task 1; existing test helpers `newTestServer(t)` and the `newReturnImmediatelyScript(t)` fixture idiom (`modules/world/world_script_queue_test.go:16-26`).
- Produces: terminal states have nil'd stack buffers; suspended states keep theirs.

**Placement rule (the safety-critical part):** `Release(state)` goes as the LAST statement of the `case script.Finished, script.Aborted:` arm in each dispatcher — i.e. AFTER `OnScriptFinishedOrAborted(state)` where that call exists. Do NOT add Release to any suspend arm, any `default:` arm, or the error-handling early-return paths above the switches (those states may still be referenced; missed releases are safe by design).

- [ ] **Step 1: Write the failing test**

Create `modules/world/script_pool_release_test.go`:

```go
package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/script"
)

// PERF-3: the terminal arms of the three resumeOrFinish* dispatchers must
// recycle the state's pooled stack buffers — observable as Release having
// nil'd the state's slices. Suspend paths must leave them intact.

func TestProcessWorldQueueReleasesTerminalState(t *testing.T) {
	s := newTestServer(t)
	state := newReturnImmediatelyScript(t)
	s.EnqueueWorldScript(state, 0) // stored as delay=1 per TS World.enqueueScript
	s.processWorldQueue()          // tick 1: delay gate, no fire
	s.processWorldQueue()          // tick 2: fires; script returns → Finished
	if state.Execution != script.Finished {
		t.Fatalf("precondition: Execution = %v, want Finished", state.Execution)
	}
	if state.IntStack != nil || state.StringStack != nil || state.Frames != nil {
		t.Error("terminal world-queue state still holds pooled buffers (Release not called in resumeOrFinishWorld)")
	}
}

func TestEnqueuedWorldScriptRetainsBuffersWhileWaiting(t *testing.T) {
	s := newTestServer(t)
	state := newReturnImmediatelyScript(t)
	s.EnqueueWorldScript(state, 3)
	s.processWorldQueue() // still waiting
	if state.IntStack == nil {
		t.Error("waiting world-queue state lost its buffers (premature Release)")
	}
}
```

Then add coverage for the player and NPC dispatchers. These need an entity
fixture: read the existing tests that already drive `resumeOrFinish` /
`resumeOrFinishNpc` end-to-end (`modules/world/script_test.go` and
`modules/world/npc_script_test.go` — e.g. the tests around
`npc_script_test.go:388` construct NPC-anchored states, and
`world_script_queue_test.go` shows the server fixture). Model one test per
dispatcher on the file's existing helper idiom, with exactly these
postconditions:

- a state the dispatcher saw reach `Finished` (or `Aborted`) has
  `IntStack == nil && StringStack == nil && Frames == nil`;
- a state the dispatcher stored as suspended (e.g. `Execution` forced to
  `script.NpcSuspended` before dispatch, with the entity's
  `activeScript` asserted set afterward) still has `IntStack != nil`.

Name them `TestResumeOrFinishReleasesTerminalState` and
`TestResumeOrFinishNpcReleasesTerminalState` (+ `...RetainsSuspendedBuffers`
variants). Use whatever minimal player/NPC construction the neighboring
tests in the same file already use — do not invent new fixture machinery.

- [ ] **Step 2: Run to verify the new tests fail**

```bash
cd ~/Code/github.com/zsrv/goscape
GOPATH="${TMPDIR:-/tmp}/go" GOCACHE="${TMPDIR:-/tmp}/go-cache" go test ./modules/world/ -run 'PoolRelease|ReleasesTerminal|RetainsBuffers|RetainsSuspended' -v
```

Expected: the `ReleasesTerminalState` tests FAIL (buffers still non-nil); the retention tests PASS already (nothing releases yet — they are the premature-release guards and must be committed failing-proof, i.e. green before AND after).

- [ ] **Step 3: Add the three Release calls**

`modules/world/script.go`, `resumeOrFinish` (the arm currently reads):

```go
	case script.Finished, script.Aborted:
		// NAI-54: TS Player.ts:2143-2148 — only nulls activeScript when
		// state matches, and additionally fires CloseModal(false) on
		// no-MAIN-modal. Both behaviors live in OnScriptFinishedOrAborted.
		self.OnScriptFinishedOrAborted(state)
```

becomes:

```go
	case script.Finished, script.Aborted:
		// NAI-54: TS Player.ts:2143-2148 — only nulls activeScript when
		// state matches, and additionally fires CloseModal(false) on
		// no-MAIN-modal. Both behaviors live in OnScriptFinishedOrAborted.
		self.OnScriptFinishedOrAborted(state)
		// PERF-3: terminal — the identity guard above has already cleared
		// any matching activeScript, so the state is provably
		// unreferenced; recycle its stack buffers. Suspend arms must
		// never do this.
		script.Release(state)
```

`modules/world/npc_script.go`, `resumeOrFinishNpc`:

```go
	case script.Finished, script.Aborted:
		// NAI-54: TS Npc.ts:226-228 — only nulls activeScript when
		// state matches.
		npc.OnScriptFinishedOrAborted(state)
		// PERF-3: terminal — identity guard done; recycle stack buffers.
		script.Release(state)
```

`modules/world/script.go`, `resumeOrFinishWorld`:

```go
	case script.Finished, script.Aborted:
		// Clean exit; nothing to do (entry already removed by caller).
		// PERF-3: recycle the terminal state's stack buffers.
		script.Release(state)
```

- [ ] **Step 4: Run the new tests to verify they pass**

Same command as Step 2. Expected: ALL PASS.

- [ ] **Step 5: Behavior-invariance gate — full world + script suites, then race**

```bash
cd ~/Code/github.com/zsrv/goscape
GOPATH="${TMPDIR:-/tmp}/go" GOCACHE="${TMPDIR:-/tmp}/go-cache" go test ./pkg/script/... ./modules/world/... && \
GOPATH="${TMPDIR:-/tmp}/go" GOCACHE="${TMPDIR:-/tmp}/go-cache" CGO_ENABLED=1 go test -race ./modules/world/ && \
GOPATH="${TMPDIR:-/tmp}/go" GOCACHE="${TMPDIR:-/tmp}/go-cache" go build ./... && gofmt -l modules/world
```

Expected: PASS everywhere, no gofmt output. If ANY existing world test fails, that is real information — a test observed a released state's stacks. STOP and report (do not weaken the test or move the Release); the failing test names the code path that still references terminal states.

- [ ] **Step 6: Verify resume-path closure (report, no code)**

Confirm each resume site re-enters one of the three dispatchers, so suspended states eventually pass through a Release arm: player delay `tick.go:733-737`, NPC delay `npc_ai.go:20-24`, button `handler_interface.go:56-58`, count-dialog `resume_dialog.go:38-56`, world queue `world_script_queue.go:80-82`. Grep/read each and record file:line of the dispatcher call in your report. Also note (as accepted, by design) the paths that terminate WITHOUT release: the error-handling early returns above the dispatch switches, and logout/despawn drops (`ClearActiveScript`, `Npc.Cleanup`) — these degrade to GC, never to aliasing.

- [ ] **Step 7: Commit**

```bash
cd ~/Code/github.com/zsrv/goscape && git status --short
git add modules/world/script.go modules/world/npc_script.go modules/world/script_pool_release_test.go
git commit --no-gpg-sign -m "perf(world): release ScriptState buffers at the three terminal dispatch arms (PERF-3)"
```

---

### Task 3: Soak evidence + PORTING documentation

**Files:**
- Modify: `docs/PORTING-CLOSED.md` (§Performance hotspots — add PERF-3 row)
- Modify: `docs/PORTING.md` (§Performance hotspots pointer line, currently ~line 41)
- Temporary (create, run, DELETE — never commit): `~/Code/github.com/zsrv/goscape-singleplayer/internal/server/soak_diag_test.go`

**Interfaces:**
- Consumes: Tasks 1-2 committed on goscape rev-274; the goscape-singleplayer repo's `internal/server` harness (sibling checkout, `replace ../goscape` picks up the pool automatically).

- [ ] **Step 1: Re-run the 4-minute server-only soak against the pooled build**

Recreate the diagnostic harness (verbatim; it existed during the 2026-07-09 memory investigation) at `~/Code/github.com/zsrv/goscape-singleplayer/internal/server/soak_diag_test.go`:

```go
package server

// Temporary diagnostic soak — NOT for commit (delete after recording the
// PERF-3 numbers). Boots the full stack with no client attached and samples
// process/Go-heap memory over ~4 minutes.

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"strings"
	"testing"
	"time"
)

func readVmRSSKB(t *testing.T) int {
	b, err := os.ReadFile("/proc/self/status")
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(line, "VmRSS:") {
			var kb int
			fmt.Sscanf(strings.TrimPrefix(line, "VmRSS:"), "%d", &kb)
			return kb
		}
	}
	return -1
}

func writeHeapProfileGC(t *testing.T, path string) {
	runtime.GC()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := pprof.Lookup("heap").WriteTo(f, 0); err != nil {
		t.Fatal(err)
	}
}

func TestServerSoakMemory(t *testing.T) {
	outDir := os.Getenv("GOSCAPE_SP_SOAK_OUT")
	if outDir == "" {
		t.Skip("set GOSCAPE_SP_SOAK_OUT to run the diagnostic soak")
	}
	cacheDir := "../../../goscape/data/pack"
	if err := CheckCache(cacheDir); err != nil {
		t.Skip(err)
	}

	cfg, err := NewConfig(Options{
		DataDir:      t.TempDir(),
		CacheDir:     cacheDir,
		WorldPort:    43894,
		OndemandPort: 48090,
		LoginPort:    42204,
		FriendsPort:  42205,
	})
	if err != nil {
		t.Fatal(err)
	}
	srv, err := Start(slog.New(slog.DiscardHandler), cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := srv.WaitReady(ctx, 48090); err != nil {
		t.Fatal(err)
	}

	csv, err := os.Create(filepath.Join(outDir, "soak.csv"))
	if err != nil {
		t.Fatal(err)
	}
	defer csv.Close()
	fmt.Fprintln(csv, "sec,rss_kb,heap_alloc,heap_sys,heap_inuse,total_alloc,num_gc,goroutines")

	writeHeapProfileGC(t, filepath.Join(outDir, "heap_start.prof"))
	start := time.Now()
	var m runtime.MemStats
	for time.Since(start) < 240*time.Second {
		runtime.ReadMemStats(&m)
		fmt.Fprintf(csv, "%d,%d,%d,%d,%d,%d,%d,%d\n",
			int(time.Since(start).Seconds()), readVmRSSKB(t),
			m.HeapAlloc, m.HeapSys, m.HeapInuse, m.TotalAlloc, m.NumGC, runtime.NumGoroutine())
		time.Sleep(10 * time.Second)
	}
	writeHeapProfileGC(t, filepath.Join(outDir, "heap_end.prof"))

	if err := srv.Stop(30 * time.Second); err != nil {
		t.Fatal(err)
	}
}
```

Run it (sandbox note: writes to the goscape-singleplayer repo and binding loopback ports may need the sandbox disabled):

```bash
mkdir -p "${TMPDIR:-/tmp}/perf3-soak"
cd ~/Code/github.com/zsrv/goscape-singleplayer
GOPATH="${TMPDIR:-/tmp}/go" GOCACHE="${TMPDIR:-/tmp}/go-cache" \
GOSCAPE_SP_SOAK_OUT="${TMPDIR:-/tmp}/perf3-soak" \
go test ./internal/server/ -run TestServerSoakMemory -v -count=1 -timeout 420s
```

Compute the idle churn rate from the CSV: `(total_alloc[last] − total_alloc[first]) / (sec[last] − sec[first])`. **Baseline (pre-pool, measured 2026-07-09): ~3.3 MB/s, one GC cycle in 4 minutes.** Expected now: well under 1 MB/s. Also note GC-cycle count and the RSS trend. If churn is NOT substantially reduced, STOP — diff `heap_start.prof`/`heap_end.prof` with `-sample_index=alloc_space` and report what still allocates; do not proceed to docs.

- [ ] **Step 2: Delete the temporary harness**

```bash
rm ~/Code/github.com/zsrv/goscape-singleplayer/internal/server/soak_diag_test.go
cd ~/Code/github.com/zsrv/goscape-singleplayer && git status --short
```

Expected: goscape-singleplayer tree clean (no staged/unstaged changes).

- [ ] **Step 3: Add the PERF-3 closure row**

In `~/Code/github.com/zsrv/goscape/docs/PORTING-CLOSED.md`, §Performance hotspots (line ~176), append after the PERF-2 row, matching the existing column format (`| ✅ FIXED | location | issue | Size | note |`):

```markdown
| ✅ FIXED | `pkg/script/runner.go` `Init` + `pkg/script/pool.go` | Fresh `make` of IntStack (8 KB) + StringStack (16 KB) + Frames per Init, ~26.5 KB/run; 71% of idle-server alloc churn (~3.3 MB/s with zero players — singleplayer memory investigation 2026-07-09). | S | PERF-3, `<TASK1_COMMIT>`+`<TASK2_COMMIT>` (2026-07-09). `sync.Pool`'d `scriptBuffers` bundle recycled via `script.Release` at exactly the three terminal dispatch arms (`resumeOrFinish` / `resumeOrFinishNpc` / `resumeOrFinishWorld`), after `OnScriptFinishedOrAborted`'s identity guard; suspend/error paths never release (missed release = GC, as before). `stringStack`+`frames` cleared at release; `intStack` left dirty — SP-discipline makes stale slots unobservable (pops only read slots pushed in the same run). Locals/Arrays/state struct stay freshly allocated (read-before-write zero-init preserved). **TS allocates fresh per init (`ScriptRunner.ts:66-119`, `ScriptState.ts:39-146`) — Go-only deviation**, spec `docs/superpowers/specs/2026-07-09-scriptstate-buffer-pool-design.md`. BenchmarkInitRelease: ~26500→`<MEASURED>` B/op. Idle soak churn: 3.3 MB/s→`<MEASURED>` MB/s (4-min server-only soak). Pins: `TestRecycledStateLeaksNothing` / `TestReleaseClearsAndNils` / `TestInitReleaseAllocBytes` (pkg/script/pool_test.go), `TestProcessWorldQueueReleasesTerminalState` + player/NPC dispatcher tests (modules/world/script_pool_release_test.go). `-race` pkg/script + modules/world clean. |
```

Replace `<TASK1_COMMIT>`/`<TASK2_COMMIT>` with the real short SHAs (`git log --oneline -3`) and both `<MEASURED>` values with the numbers from Step 1 and Task 1's benchmark output.

In `~/Code/github.com/zsrv/goscape/docs/PORTING.md`, the §Performance hotspots table body (~line 41) currently reads:

```markdown
| _(none — both LOW rows closed 2026-06-03: PERF-1 tick player-snapshot scratch + PERF-2 hunt zone-iteration scratch/iterator; benchmarks + closure rows in [`docs/PORTING-CLOSED.md`](docs/PORTING-CLOSED.md) §Performance hotspots)_ |
```

Replace with:

```markdown
| _(none — all rows closed: PERF-1 tick player-snapshot scratch + PERF-2 hunt zone-iteration scratch/iterator (2026-06-03), PERF-3 ScriptState buffer pool (2026-07-09); benchmarks + closure rows in [`docs/PORTING-CLOSED.md`](docs/PORTING-CLOSED.md) §Performance hotspots)_ |
```

- [ ] **Step 4: Commit the docs**

```bash
cd ~/Code/github.com/zsrv/goscape && git status --short
git add docs/PORTING-CLOSED.md docs/PORTING.md
git commit --no-gpg-sign -m "docs(porting): PERF-3 closure row — ScriptState buffer pool, soak + benchmark evidence"
```
