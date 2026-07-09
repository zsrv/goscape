# ScriptState buffer pool — Design (rev-274)

**Date:** 2026-07-09
**Status:** Approved
**Scope:** rev-274 only (standing policy: perf improvements land on the newest
branch; earlier branches stay faithful-translation-first).

## Motivation

The goscape-singleplayer memory investigation (2026-07-09) measured the idle
server allocating ~3.3 MB/s with zero players connected; heap profiling
attributed 71% of that churn to `script.Init`
(`pkg/script/runner.go:15`), reached every 600ms tick via
`(*Server).processNpcTimer` → `buildNpcScriptState` → `(*Npc).turn`. Each
`Init` allocates ~26.5 KB, of which ~26 KB is three fixed-capacity buffers:
`IntStack` (`make([]int, StackCapacity)`, 8 KB), `StringStack`
(`make([]string, StackCapacity)`, 16 KB), and `Frames`
(`make([]Frame, FrameCapacity)`, ~2 KB). The churn is not a leak — live heap
is flat — but it drives the GC heap ceiling and makes the process fill its
RSS high-water mark within minutes.

## Decision

Pool exactly those three buffers. Do NOT pool the `ScriptState` struct,
`IntLocals`, `StringLocals`, or `Arrays` — those carry RuneScript's
read-before-write zero-init semantics (`handlePushIntLocal`,
`pkg/script/handlers.go:626-635`, reads locals with no written-this-run
guard; `handlePushArrayInt`, `handlers_array.go:24-34`, likewise) and stay
freshly allocated per `Init`, byte-for-byte TS-equivalent. Whole-state
pooling was considered and rejected: it requires a ~25-field reset table that
must track every future field, and a premature release can alias a live
`activeScript` through the pointer-identity guards
(`player_script.go:248`, `npc.go:381`).

## Mechanism

In `pkg/script`:

- `type scriptBuffers struct { intStack []int; stringStack []string; frames []Frame }`
  with one package-level `sync.Pool` whose `New` allocates all three at full
  capacity. One `Get`/`Put` moves the bundle atomically (pooling a pointer —
  no interface boxing).
- `ScriptState` gains an unexported field `buf *scriptBuffers`.
- `Init` (signature unchanged) takes a bundle from the pool instead of three
  `make` calls, wires the slices into the state, and records `buf`. States
  built by tests that never release simply drop the bundle to the GC —
  no contract change for existing callers.

### Release seam

New exported `func Release(s *ScriptState)`:

- No-op when `s == nil` or `s.buf == nil` (double-release is therefore also
  a no-op: the first release nils `buf`).
- Clears every slot of `stringStack` and `frames` before returning the
  bundle — releases pinned `string`, `*ScriptFile`, and locals-slice
  references. `intStack` is intentionally NOT cleared: values above SP are
  provably never read (see Safety argument).
- Returns the bundle to the pool and **nils `s.IntStack`, `s.StringStack`,
  `s.Frames`, `s.buf`** so any use-after-release fails loudly (nil-slice
  index panic) instead of silently aliasing a recycled buffer.

### Release call sites — exactly three

The provably-dead point is the `Finished`/`Aborted` arm of each dispatcher,
placed AFTER `OnScriptFinishedOrAborted` has run (its pointer-identity guard
is what makes this point safe):

1. `resumeOrFinish` — `modules/world/script.go:181-185`
2. `resumeOrFinishNpc` — `modules/world/npc_script.go:461-464`
3. `resumeOrFinishWorld` — `modules/world/script.go:301-302`

Suspend arms (`Suspended`, `PauseButton`, `CountDialog`, `NpcSuspended`,
`WorldSuspended`) release nothing — a suspended state keeps its buffers
across ticks (stored in `Player.activeScript`, `Npc.activeScript`, or the
world script queue, the only three cross-tick retention points in the
codebase) until it terminates through one of the same three arms.

The implementation plan must verify each resume path (player delay
`tick.go:733-737`, NPC delay `npc_ai.go:20-24`, button
`handler_interface.go:56-58`, count-dialog `resume_dialog.go:54-56`, world
queue `world_script_queue.go:80-82`) re-enters one of the three dispatchers,
so no terminal state bypasses release. A missed release is safe regardless —
it degrades to today's GC behavior for that one state.

## Safety argument (why reused buffers cannot change behavior)

- **Int/string stacks are SP-disciplined:** `PushInt`/`PushString` write
  `[SP]` then increment; `PopInt`/`PopString` (`pkg/script/state.go:483-527`)
  are `SP > 0`-guarded and only ever read slots written earlier in the same
  run. Slots above SP are unreachable. Stale ints in a reused `intStack` are
  therefore unobservable; `stringStack` is cleared at release anyway (for GC,
  not correctness).
- **Frames:** written at `[FrameSP]` before increment, read only below
  `FrameSP` (`GosubCall`/`JumpCall`, `state.go:551-573`; `Backtrace`,
  `runner.go:212` is `FrameSP`-bounded). Cleared at release for GC.
- **Everything with read-before-write semantics stays freshly allocated**
  (struct, locals, arrays) — the zero-init the interpreter relies on keeps
  coming from `make`, exactly as today.
- **Pointer identity of `*ScriptState` is never recycled**, so the
  `activeScript != state` guards and all `activeScript.Execution` reads are
  untouched.
- `sync.Pool` is concurrency-safe; script execution is confined to the world
  tick goroutine today, and nothing in this design depends on that staying
  true.

## Fidelity

TS allocates fresh arrays per init and never reuses them
(`ScriptRunner.init`, `Engine-TS/src/engine/script/ScriptRunner.ts:66-119`;
`ScriptState` field initializers, `ScriptState.ts:39-146`). This is a
documented Go-only optimization with observably identical behavior, recorded
as a deviation row in `docs/PORTING.md` citing the stack-discipline audit
above.

## Testing

1. **Unit (white-box, `pkg/script`):** `Release` clears every `stringStack`
   and `frames` slot; returns buffers for reuse (assert backing-array
   identity across `Release` → `Init` via `&slice[0]` comparison, tolerating
   pool non-determinism by draining/priming the pool in the test); nils the
   state's slice fields; double-`Release` and `Release(nil)` are no-ops.
2. **Stale-leak pin:** execute a script that pushes distinctive sentinel
   values onto both stacks and gosub frames, `Release` it, `Init` a second
   state on the recycled bundle, and prove nothing from the first run is
   observable (locals zero, pops on the empty stack return 0/"" via the
   underflow path, frames empty).
3. **Behavior invariance:** the full existing script/world suite green —
   `go test ./pkg/script/... ./modules/world/...` plus `-race` on those
   packages (race detector is available on this box; CGO_ENABLED=1).
4. **Benchmark:** `BenchmarkInitExecuteRelease` with `b.ReportAllocs()`
   pinning the per-iteration allocation drop (~26.5 KB → ~1 KB; assert the
   improvement loosely — bytes/op below a threshold — so the benchmark
   doubles as a regression gate without being flaky).
5. **End-to-end:** re-run the 4-minute singleplayer server-only soak
   (the diagnostic harness from the memory investigation); expect idle churn
   well under 1 MB/s (from ~3.3 MB/s) and correspondingly rarer GC cycles.

## Out of scope

- Pooling the `ScriptState` struct, locals, or script arrays (rejected above).
- Other rev branches (backport only on concrete need, per policy).
- The singleplayer `--memory-limit` flag (separate, complementary knob —
  may be picked up independently).
- Client-side allocation work (`pix8`/scene churn — no evidence it matters).
