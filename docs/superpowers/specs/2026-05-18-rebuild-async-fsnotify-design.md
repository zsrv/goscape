# `::rebuild` async worker + fsnotify auto-rebuild

**Date:** 2026-05-18
**Tech stack:** Go 1.26+ per `[[go_version]]`. New dep: `github.com/fsnotify/fsnotify`.
**Cadence:** Standard — multi-file change with a new dep and new goroutines; warrants a full spec → plan → TDD execution cycle.
**TS canonical source:** `Engine-TS/src/cache/DevThread.ts` (the entire file) + `Engine-TS/src/engine/World.ts:1745-1810` (DevThread parent-port wiring) + `ClientCheatHandler.ts:151-153`.
**Predecessor spec:** `docs/superpowers/specs/2026-05-17-rebuild-cheat-design.md` (synchronous `::rebuild`, shipped at 7a6e6f2d). This spec ports the two follow-ups deferred there.

---

## §1. Scope

Wire two deferred follow-ups from `[[rebuild_cheat_close]]` in a single coherent slice:

1. **Async PackAll.** Replace the synchronous on-tick PackAll+Reload in `::rebuild`'s handler with a dispatch to a long-lived background `rebuildWorker` goroutine. The handler returns immediately; the tick continues normally during the ~7s pack. Reload still runs on the tick goroutine (preserves the single-tick-goroutine invariant for registry swaps), driven by a result-event drain at top-of-tick.
2. **fsnotify auto-rebuild.** New `contentWatcher` goroutine recursively watches the 12 TS-canonical subdirectories of ContentPath, debounces fs events for 1s (matching TS DevThread), and dispatches a rebuild request to the same worker. Opt-in via a new `--world.content-watch` flag.

Both share one coordination plane: a depth-1 `rebuildReq` chan with non-blocking-send coalescing, a depth-1 `rebuildResult` chan back to the tick goroutine, and a small `rebuildMu`-guarded busy/pending state machine on `Server` that mirrors TS DevThread's `active`/`processNextQueue` semantics.

**In scope:**
- New config field `ContentWatch` (CLI flag `--world.content-watch`, yaml `content_watch`). Default `false`.
- New `rebuildWorker` goroutine spawned in `NewServerService`'s startingFn when `ContentPath != ""`.
- New `contentWatcher` goroutine spawned when `ContentPath != "" && ContentWatch == true`.
- Coordination chans + state fields on `Server`. Test-injectable `packFn` seam.
- Replace the inline sync code in the `::rebuild` handler with an async dispatch.
- Top-of-tick non-blocking drain of `rebuildResult` → `Reload(true)` + broadcast.
- New broadcast helper: private message to manual invoker (if any) + all online staff (modlvl≥4).
- Add `github.com/fsnotify/fsnotify` dependency.
- Tests at three layers: worker (stubbed pack), watcher (real fsnotify on `t.TempDir()`), tick-drain (handler + recording Reload wrapper). All `-race` clean.

**Out of scope:**
- PackAll cancellation (no `context.Context` threading through pack stages). Worker waits for in-flight pack to finish before honoring `s.quit`. Mid-shutdown `::rebuild` is a non-goal.
- Reload latency reduction. Reload still runs synchronously on the tick goroutine; it already did under the sync spec. Separate concern.
- Watcher auto-restart after fatal fsnotify error. If `watcher.Errors` closes, the watcher goroutine exits; manual `::rebuild` continues to work. Restart is a deferred follow-up.
- Symlink-specific handling. fsnotify follows symlinks by default; we accept that.
- Friend-server RELAY_RELOAD integration (already out of scope for the sync spec; unchanged).
- New deviation tags. The design ports TS DevThread's async + watcher semantics directly; no permanent TS-divergent semantics.

---

## §2. Layout

| File | State | Purpose |
|---|---|---|
| `modules/world/config.go` | MODIFIED | Add `ContentWatch bool` field. Register flag `--world.content-watch`; yaml `content_watch:`. Default `false`. No validation interaction with `ContentPath` (watcher just no-ops if ContentPath is empty). |
| `modules/world/server.go` | MODIFIED | Add `Server` fields: `rebuildReq chan struct{}`, `rebuildResult chan rebuildResult`, `rebuildMu sync.Mutex`, `rebuildBusy bool`, `rebuildPending bool`, `rebuildManualInvoker *Player`, `packFn func(string, string, string) error`. Initialize chans + default `packFn = packall.PackAll` in `New(...)`. Spawn worker + (optionally) watcher in startingFn. |
| `modules/world/rebuild_worker.go` | NEW | `runRebuildWorker` goroutine body, `handleRebuildResult` tick-side helper, `dispatchRebuildRequest` non-blocking-send helper, `rebuildResult` struct. |
| `modules/world/content_watcher.go` | NEW | `runContentWatcher` goroutine body, `addWatchesRecursive` helper, the canonical-subdir list. |
| `modules/world/tick.go` | MODIFIED | At the very top of the for-body (before the shutdown gate), add non-blocking `select`-with-default drain on `s.rebuildResult` → `s.handleRebuildResult(r)`. |
| `modules/world/handlers_game.go` | MODIFIED | Replace the inline PackAll/Reload block in `case "rebuild":` with: ContentPath check (private error to invoker only), staff-broadcast "Rebuilding scripts...", capture `rebuildManualInvoker` under mu, `dispatchRebuildRequest`. |
| `modules/world/rebuild_worker_test.go` | NEW | 4 worker tests with stub `packFn`. |
| `modules/world/content_watcher_test.go` | NEW | 5 watcher tests against `t.TempDir()`. |
| `modules/world/handler_cheat_rebuild_test.go` | REWRITTEN | Replace the 3 existing sync-path tests with 6 async-path tests covering dispatch, drain, error path, broadcast targeting. |
| `go.mod` / `go.sum` | MODIFIED | Add `github.com/fsnotify/fsnotify v1.x` (latest stable). |

No changes outside `modules/world/`. `pkg/packall`, `pkg/pack/*`, `modules/asset` untouched.

---

## §3. Invocation surface

- Existing flag: `--world.content-path=<dir>` (unchanged from `[[rebuild_cheat_close]]`). Default `""`. When empty, `::rebuild` returns a private error to the invoker and the watcher is not started even if `ContentWatch` is true.
- New flag: `--world.content-watch` (yaml `content_watch:`). Default `false`. When `true` AND `ContentPath != ""`, spawn the watcher.
- Cheat: `::rebuild` (unchanged). Inside the existing dev-block (`!NodeProduction && staffModLevel >= 4`).

Combinations:
- `content-path="" content-watch=false`: no worker, no watcher. `::rebuild` returns "Rebuild failed: --world.content-path is not configured."
- `content-path="" content-watch=true`: no worker, no watcher (watcher requires ContentPath). Operator-config drift; warn-log at startup, treat as `content-watch=false`.
- `content-path=<dir> content-watch=false`: worker runs; `::rebuild` works; no auto-rebuild.
- `content-path=<dir> content-watch=true`: worker + watcher both run.

---

## §4. Architecture

### §4.1 Coordination plane

```
                                        Server (lifecycle owner)
                                        │
┌─ ::rebuild handler ────────┐          │  rebuildReq    chan struct{}   buf 1
│  non-blocking-send         │ ───────► │  rebuildResult chan rebuildResult buf 1
└────────────────────────────┘          │  rebuildMu     sync.Mutex
                                        │  rebuildBusy   bool
┌─ contentWatcher goroutine ─┐          │  rebuildPending bool
│  fsnotify events ─► 1s     │ ───────► │  rebuildManualInvoker *Player
│  debounce ─► non-blk-send  │          │  packFn        func (test seam)
└────────────────────────────┘          │
                                        ├── rebuildWorker goroutine
                                        │   select on rebuildReq / s.quit
                                        │   runs packFn; posts rebuildResult
                                        │   coalesces via rebuildPending
                                        │
                                        └── tick loop (existing goroutine)
                                            top-of-body: non-blocking drain
                                            of rebuildResult → handleRebuildResult
                                            (calls Reload(true) on success)
```

**Coalescing semantics (mirrors TS DevThread's `active` + `processNextQueue` + `processNextTimeout` machinery):**

- `rebuildReq` is `chan struct{}` cap 1. Both sender sites use non-blocking send via `select { case s.rebuildReq <- struct{}{}: default: }`. If the chan is full, a request is already queued — drop silently.
- Worker reads from `rebuildReq`, sets `rebuildBusy = true` and clears `rebuildPending` under `rebuildMu`, runs `packFn` **without holding the mutex**, posts `rebuildResult{...}` (blocking until tick drains), then under `rebuildMu` clears `rebuildBusy` and — if `rebuildPending` was set by a coalesced sender — re-queues itself via non-blocking send on `rebuildReq`.
- Senders set `rebuildPending = true` under `rebuildMu` before attempting the non-blocking send. This ensures that any request which arrived *while* worker was busy is honored on the next iteration, even if the chan-send dropped because chan was already full.

The dual mechanism (chan + pending-flag) is necessary because the chan alone can drop a request that lands between "worker drained chan" and "worker cleared busy". The pending-flag closes that race.

### §4.2 rebuildWorker (`rebuild_worker.go`)

```go
func (s *Server) runRebuildWorker() {
    for {
        select {
        case <-s.quit:
            return
        case <-s.rebuildReq:
        }

        s.rebuildMu.Lock()
        s.rebuildBusy = true
        s.rebuildPending = false
        invoker := s.rebuildManualInvoker
        s.rebuildMu.Unlock()

        start := time.Now()
        err := s.packFn(s.cfg.ContentPath, s.cfg.CachePath, s.cfg.CachePath)
        elapsed := time.Since(start)

        // Blocking send: back-pressure ensures tick drains and runs Reload
        // before worker accepts the next request. Worst-case wait ≈ 1 tick.
        select {
        case s.rebuildResult <- rebuildResult{err: err, duration: elapsed, invoker: invoker}:
        case <-s.quit:
            return
        }

        s.rebuildMu.Lock()
        s.rebuildBusy = false
        s.rebuildManualInvoker = nil
        pending := s.rebuildPending
        s.rebuildPending = false
        s.rebuildMu.Unlock()

        if pending {
            // Re-queue self. Non-blocking send is safe because we just
            // drained from rebuildReq above (and nothing else writes
            // unless a new request arrived after our drain — in which
            // case the chan is full and the pending flag was redundant).
            select {
            case s.rebuildReq <- struct{}{}:
            default:
            }
        }
    }
}
```

`packFn` defaults to `packall.PackAll`. Tests override.

### §4.3 dispatchRebuildRequest (`rebuild_worker.go`)

```go
// Called from both the ::rebuild handler and the content watcher.
// Sets rebuildPending under mu, then non-blocking sends on rebuildReq.
// Caller is responsible for setting rebuildManualInvoker (manual path only).
func (s *Server) dispatchRebuildRequest() {
    s.rebuildMu.Lock()
    s.rebuildPending = true
    s.rebuildMu.Unlock()

    select {
    case s.rebuildReq <- struct{}{}:
    default:
    }
}
```

### §4.4 handleRebuildResult (`rebuild_worker.go`, tick-side)

```go
// Runs on the tick goroutine. Called from the top-of-body drain in tick.go.
func (s *Server) handleRebuildResult(r rebuildResult) {
    if r.err != nil {
        s.log.Error("rebuild: PackAll failed", "err", r.err)
        s.broadcastRebuildStaff(r.invoker, "Rebuild failed: "+r.err.Error())
        return
    }
    if err := s.Reload(true); err != nil {
        s.log.Error("rebuild: Reload failed", "err", err)
        s.broadcastRebuildStaff(r.invoker, "Rebuild failed: reload returned error (see server log).")
        return
    }
    msg := fmt.Sprintf("Rebuilt: %s.", r.duration.Round(time.Millisecond))
    s.broadcastRebuildStaff(r.invoker, msg)
}
```

### §4.5 broadcastRebuildStaff (helper on Server)

```go
// Sends msg privately to invoker (if non-nil) AND to every online player
// with staffModLevel >= 4. Deduplicates: if invoker is staff≥4, they only
// receive once.
func (s *Server) broadcastRebuildStaff(invoker *Player, msg string) {
    s.playersMu.RLock()
    players := make([]*Player, len(s.playerLoop))
    copy(players, s.playerLoop)
    s.playersMu.RUnlock()

    delivered := false
    for _, p := range players {
        if p.staffModLevel < 4 && p != invoker {
            continue
        }
        p.MessageGame(msg)
        if p == invoker {
            delivered = true
        }
    }
    if invoker != nil && !delivered {
        // invoker not in playerLoop (test scaffolding) — best effort
        invoker.MessageGame(msg)
    }
}
```

(The `staffModLevel` accessor name is illustrative; the plan task will confirm against `Player`'s actual field/method — likely a method that pulls from the player's record/account.)

### §4.6 contentWatcher (`content_watcher.go`)

```go
// canonicalContentSubdirs is the TS DevThread.ts:100-111 list verbatim.
var canonicalContentSubdirs = []string{
    "maps", "songs", "jingles", "binary", "fonts", "title",
    "scripts", "sprites", "models", "textures", "synth", "wordenc",
}

func (s *Server) runContentWatcher() {
    w, err := fsnotify.NewWatcher()
    if err != nil {
        s.log.Error("contentWatcher: fsnotify init failed", "err", err)
        return
    }
    defer w.Close()

    for _, sub := range canonicalContentSubdirs {
        root := filepath.Join(s.cfg.ContentPath, sub)
        if err := addWatchesRecursive(w, root); err != nil {
            s.log.Warn("contentWatcher: add-watches failed", "root", root, "err", err)
            // continue with whatever we did add
        }
    }

    var debounceC <-chan time.Time
    const debounce = 1 * time.Second

    for {
        select {
        case <-s.quit:
            return

        case ev, ok := <-w.Events:
            if !ok {
                s.log.Warn("contentWatcher: Events chan closed")
                return
            }
            // Dynamic add-on-CREATE-dir (subdir created mid-session).
            if ev.Op&fsnotify.Create != 0 {
                if info, err := os.Stat(ev.Name); err == nil && info.IsDir() {
                    _ = w.Add(ev.Name) // best-effort
                }
            }
            // Reset debounce timer.
            debounceC = time.After(debounce)

        case err, ok := <-w.Errors:
            if !ok {
                s.log.Warn("contentWatcher: Errors chan closed")
                return
            }
            s.log.Warn("contentWatcher: fsnotify error", "err", err)

        case <-debounceC:
            debounceC = nil
            s.dispatchRebuildRequest()
        }
    }
}

func addWatchesRecursive(w *fsnotify.Watcher, root string) error {
    return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
        if err != nil {
            // Missing root is fine (content subset not present); skip.
            if errors.Is(err, fs.ErrNotExist) {
                return fs.SkipDir
            }
            return err
        }
        if !d.IsDir() {
            return nil
        }
        return w.Add(path)
    })
}
```

`time.After`-based debounce reset is correct here because `debounceC` is a single per-burst timer; if a new event arrives during the 1s window, we overwrite `debounceC` with a fresh `time.After(...)` and the old timer fires into a never-read variable (GC reclaims). No timer leak risk in practice — even a million events over the session is dwarfed by per-tick allocations.

### §4.7 `::rebuild` handler (`handlers_game.go`)

```go
case "rebuild":
    // NAI-REBUILD-ASYNC — async dispatch via rebuildWorker. Mirrors TS
    // ClientCheatHandler.ts:151-153 → World.rebuild() →
    // DevThread.processChangedFiles. Handler returns immediately; result
    // arrives on the next tick.
    if p.client.server.cfg.ContentPath == "" {
        p.MessageGame("Rebuild failed: --world.content-path is not configured.")
        return nil
    }
    p.client.server.rebuildMu.Lock()
    if p.client.server.rebuildManualInvoker == nil {
        p.client.server.rebuildManualInvoker = p
    }
    p.client.server.rebuildMu.Unlock()
    p.client.server.broadcastRebuildStaff(p, "Rebuilding scripts...")
    p.client.server.dispatchRebuildRequest()
    return nil
```

### §4.8 Tick drain (`tick.go`)

Inserted as the **first** statement inside the `for {` body in `runTickLoopWithRate`, before the existing shutdown gate at L44:

```go
for {
    // NAI-REBUILD-ASYNC — drain at top-of-body so Reload runs before any
    // per-tick work observes mid-swap state. Mirrors processShutdown
    // placement at the top of the body.
    select {
    case r := <-s.rebuildResult:
        s.handleRebuildResult(r)
    default:
    }

    // NAI-182 — shutdown consumer must run BEFORE any per-tick work …
    if s.shutdownTick != -1 && s.currentTick >= s.shutdownTick {
        s.processShutdown()
        …
```

### §4.9 Server lifecycle wiring

In `New(...)`:
- Initialize `s.rebuildReq = make(chan struct{}, 1)`.
- Initialize `s.rebuildResult = make(chan rebuildResult, 1)`.
- Initialize `s.packFn = packall.PackAll`.

In `NewServerService`'s `startingFn`:
- If `cfg.ContentPath != ""`: `go s.runRebuildWorker()`.
- If `cfg.ContentPath != "" && cfg.ContentWatch`: `go s.runContentWatcher()`.
- If `cfg.ContentPath == "" && cfg.ContentWatch`: log Warn "content-watch is set but content-path is empty; auto-rebuild disabled". No goroutine.

Chans are always created (in `New`), so the tick drain is safe regardless of worker presence — draining an empty chan via `select`-default just falls through.

When `s.quit` closes:
- Worker exits on the outer select. If currently mid-pack, finishes the pack and exits on the result-send select.
- Watcher exits on the outer select. `defer w.Close()` drains fsnotify cleanly.

### §4.10 Retire predecessor's "Open follow-ups" entry

`[[rebuild_cheat_close]]`'s "Open follow-ups" lists "Async-via-worker (deferred)" and "fsnotify auto-rebuild (deferred)". After this spec ships, both bullets retire. The memory entry gets updated post-merge with a back-link to this spec's close memory.

The third bullet (PackAll's RunServerCompiler emitting pointer-check errors on real Content) is unrelated and remains open.

---

## §5. Error Handling & Failure Semantics

| Failure | Behavior |
|---|---|
| PackAll error | Worker posts `rebuildResult{err}`. Tick drain logs Error, broadcasts "Rebuild failed: <err>." to invoker+staff. Reload NOT called. CachePath left partial (preserves NAI-190-D2-HALF-SWAP). |
| Reload error after successful PackAll | Tick drain runs Reload; on error logs Error, broadcasts "Rebuild failed: reload returned error (see server log)." Server state may be partially swapped (existing NAI-190-D2-HALF-SWAP). |
| `::rebuild` with `ContentPath==""` | Handler short-circuits with private message to invoker only. No staff broadcast. |
| `::rebuild` while in-flight | Pending-flag set; non-blocking send drops if chan full. Worker re-queues on completion. New invoker NOT recorded (original holds slot); coalesced staff invoker still gets messages via staff broadcast. |
| fsnotify `watcher.Add` partial failure | Log Warn with path; continue with partial coverage. |
| fsnotify `Events` or `Errors` chan closed | Log Warn; watcher exits. Manual `::rebuild` continues. No auto-restart in v1. |
| `s.quit` closed mid-pack | Worker's result-send selects on `s.quit`; if quit wins, worker returns immediately, result is discarded. Tick may not run another iteration anyway. |
| `s.quit` closed mid-watcher | Watcher selects on `s.quit` first; `defer w.Close()` cleans up. |
| `--world.content-watch=true` with `ContentPath==""` | Log Warn at startup; watcher not started. No runtime impact. |

---

## §6. Test Strategy

All new tests run under `-race`. No `time.Sleep` for synchronization; use channel rendezvous. Bounded `time.After` waits only in "did NOT receive" negative assertions.

### §6.1 Worker tests (`rebuild_worker_test.go`)

Inject stub `packFn` directly: `s := newTestServer(t); s.packFn = ...`. Spawn worker via `go s.runRebuildWorker()`; close `s.quit` in `t.Cleanup`.

| Test | Pins |
|---|---|
| `TestRebuildWorker_Request_RunsPackAndPostsResult` | Send on `rebuildReq` via `dispatchRebuildRequest` → assert stub called once with `(ContentPath, CachePath, CachePath)` → assert result on `rebuildResult` with `err==nil`, duration > 0. |
| `TestRebuildWorker_PackError_PostsErrorResult` | Stub returns `errors.New("boom")` → result.err.Error() == "boom". |
| `TestRebuildWorker_BusyCoalesce_PendingTriggersSecondRun` | Stub blocks on `<-release` chan. Dispatch req1; wait for stub-entered signal. Dispatch req2 and req3 (rapidly). Release. Drain result1; assert stub called a second time (pending). Drain result2. Verify stub called exactly twice (req3 coalesced into req2's pending state). |
| `TestRebuildWorker_QuitMidIdle_ExitsCleanly` | Close `s.quit`; assert goroutine returns within 1s via a done-chan. |

### §6.2 Watcher tests (`content_watcher_test.go`)

Real fsnotify on `t.TempDir()`. Pre-create canonical subdirs as needed. Replace `s.rebuildReq` with a test-owned cap-1 chan and read it directly (or drain via `dispatchRebuildRequest`'s sender path).

| Test | Pins |
|---|---|
| `TestContentWatcher_FileWrite_TriggersRebuildAfterDebounce` | Pre-create `scripts/`; spawn watcher; write `scripts/foo.rs2`; assert receive on `rebuildReq` within ~2s. |
| `TestContentWatcher_BurstCoalesces` | Write 10 files in rapid succession; assert exactly ONE receive on `rebuildReq` within debounce + slack window. |
| `TestContentWatcher_NewSubdir_AddedToWatch` | Pre-create `scripts/`; spawn; create `scripts/newdir/`; wait for create-event to be processed; write `scripts/newdir/x.rs2`; assert receive. |
| `TestContentWatcher_NonWatchedDirIgnored` | Pre-create canonical dirs + `node_modules/`; write to `node_modules/foo`; assert NO receive within 2s. |
| `TestContentWatcher_QuitClosesCleanly` | Spawn; close `s.quit`; assert goroutine returns within 1s. |

### §6.3 Handler + tick-drain tests (rewritten `handler_cheat_rebuild_test.go`)

| Test | Pins |
|---|---|
| `TestHandleClientCheat_Rebuild_NoContentPath_PrivateError` | Unchanged semantics from sync spec. Private message to invoker only; no broadcast; worker not engaged. |
| `TestHandleClientCheat_Rebuild_DispatchesAndBroadcastsInProgress` | Stub `packFn` blocks. Dispatch `::rebuild`. Drain invoker conn: "Rebuilding scripts..." received. Worker still busy (assert `rebuildBusy` true under mu, or via test-only accessor). |
| `TestHandleClientCheat_Rebuild_Async_HappyPath_DrainsAndReloads` | Stub returns nil immediately. Dispatch. Manually drive ONE tick-drain iteration via a test-exposed helper or by running the worker + calling `handleRebuildResult` directly via a recording Reload wrapper. Assert "Rebuilt: <duration>." on invoker conn. Assert Reload invoked. |
| `TestHandleClientCheat_Rebuild_Async_PackError_BroadcastsFailure_SkipsReload` | Stub returns error. After drain: "Rebuild failed: <err>." on invoker conn. Reload NOT called. |
| `TestRebuildResult_BroadcastsToInvokerAndStaff` | Two players in `playerLoop`: staff (modlvl≥4) + non-staff. Invoker is a third staff player (also in loop). Drive a result with success. Invoker + first staff see "Rebuilt: ..."; non-staff sees nothing. |
| `TestRebuildResult_FsnotifyTriggered_BroadcastsToStaffOnly` | Result with `invoker=nil`. Staff player receives message; non-staff doesn't. |

### §6.4 Smoke-pack invariant

Re-run `goscape-cli smoke-pack --content-dir /home/owner/Code/github.com/LostCityRS/content`; expect 12 OK / 0 ERR baseline unchanged. The new code touches no pack stage.

### §6.5 Test-only seams summary

- `Server.packFn` field — direct assignment in test setup.
- A possible `Server.reloadFn` field or recording wrapper for asserting Reload-invoked. Tasks will decide between a `reloadFn` field on Server (smallest, matches packFn shape) vs. introducing a `Reloader` interface (heavier). Default: `reloadFn` field with default `s.Reload`.
- `t.Cleanup(func() { close(s.quit) })` is the canonical worker/watcher teardown.

---

## §7. Open Questions / Deferrals

- **Watcher auto-restart.** If fsnotify's chans close (e.g., inotify watch limit), watcher exits and auto-rebuild stops; manual `::rebuild` continues. No auto-restart. Trigger to add: reports of silently-dropped auto-rebuild.
- **PackAll cancellation.** PackAll has no `context.Context` today. Worker can't interrupt an in-flight pack on shutdown. Threading ctx through every pack stage is a much larger separate slice. Acceptable: shutdown-during-rebuild is a non-goal.
- **Coalesced manual-invoker visibility.** Two staff hit `::rebuild` in quick succession → only first is captured in `rebuildManualInvoker`; second's per-invoker message arrives via staff broadcast (they're staff). Documented.
- **Reload latency on the tick.** Reload still runs synchronously on the tick goroutine (~1–2 ticks on real Content). This was already true under the sync spec. Separate concern if it becomes operational pain.
- **Watcher symlinks.** fsnotify follows symlinks; we don't add special handling.
- **No invoker timeout for stale `rebuildManualInvoker`.** If invoker disconnects between dispatch and result-drain (~7s), the `*Player` may point at a logged-off player. `MessageGame` on a disconnected player is a no-op in the existing codebase (verify during plan execution). If not, plan task to add a "player still in loop" check before delivering invoker-side message.

---

## §8. Acceptance

- `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...` passes.
- `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/...` passes.
- `goscape --help` shows `--world.content-watch` flag.
- Smoke-pack baseline unchanged (12 OK / 0 ERR).
- Manual smoke A: launch server with `--world.content-path=<path>`; type `::rebuild` as staff; "Rebuilding scripts..." appears immediately; `::ping` or movement works during the ~7s pack (tick is not blocked); "Rebuilt: Ns." appears when pack completes. New scripts are live.
- Manual smoke B: launch server with `--world.content-path=<path> --world.content-watch`; edit a file under `${ContentPath}/scripts/`; within ~1–2s, "Rebuilding scripts..." broadcasts to staff (no invoker); "Rebuilt: Ns." follows.

---

## §9. Memory updates (post-merge)

- Update `[[rebuild_cheat_close]]`: retire the "Async-via-worker" and "fsnotify auto-rebuild" open follow-up bullets; add back-link to this spec's close memory.
- New close memory `rebuild_async_fsnotify_close.md` summarizing the slice shape, any deviations, and the final test count.
