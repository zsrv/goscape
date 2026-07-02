# Architecture Fix Arc (Arc 28) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix the seven verified HIGH findings from the 2026-07-02 architecture review (session 38a058c4): SQLite write-contention, exit-0-on-module-failure, WS-bridge panic gap, the connection-teardown/shutdown cluster, logout-save loss, and missing CI.

**Architecture:** Each finding is an independent fix branch off `rev-274`, merged back after its own RED→GREEN cycle (Arc-27 cadence). The connection-teardown cluster (Task 4) is one branch with four commits because its sub-fixes share the client-lifecycle design: guaranteed removal queue → refcounted buffer release → shutdown conn-close + serverDone restructure → write-timeout/flush-error teardown. All fixes are Go-side infrastructure; none change wire behavior or gameplay (fidelity gate untouched).

**Tech Stack:** Go 1.26, modernc.org/sqlite v1.50.1, dskit-port services/modules, GitHub Actions.

## Global Constraints

- Base branch: `rev-274`. Branch per task: `fix/arch28-<slug>`, merge back with `git merge --no-ff`, delete branch. Do NOT touch other rev branches or `main`.
- Every `go` invocation: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go ...`.
- Every commit: `git commit --no-gpg-sign`, message ends with `Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>`.
- Go 1.26 idioms: `wg.Go()`, `t.Context()`, `atomic.Int32`, `min()`, `errors.Join`. Match surrounding comment density/naming.
- Race detector: run `CGO_ENABLED=1 go test -race` on every touched package before declaring a task green.
- Fidelity gate: no change to bytes on the wire, tick semantics, or gameplay behavior. Config-default changes (Task 4d) are Go-side operational defaults, documented in `examples/full-config-reference.yaml`.
- After each task's merge: `go build ./...` and `go test ./...` must pass on `rev-274`.

---

### Task 1: SQLite write-contention hardening (login + friends)

**Files:**
- Modify: `modules/login/db.go` (openDB, ~line 40)
- Modify: `modules/friends/db.go` (openDB, ~line 37)
- Test: `modules/login/db_test.go`, `modules/friends/db_test.go`

**Interfaces:**
- Consumes: existing `openDB(dsn string) (*sql.DB, error)` in both modules.
- Produces: same signature; adds unexported `dsnWithPragmas(dsn string) string` in each module (mirrored sibling files, per existing login/friends mirroring convention).

**Why:** busy_timeout is 0 and the pool is unbounded, so two concurrent write transactions → immediate SQLITE_BUSY (mass logout at shutdown fires one write tx per player). Also `PRAGMA foreign_keys=ON` via `db.Exec` lands on one pooled connection only — migration 000003's cascades are inert on the rest. `SetMaxOpenConns(1)` matches TS's better-sqlite3 single-connection posture.

- [ ] **Step 1: Write the failing tests (login)**

Add to `modules/login/db_test.go` (reuse the existing test-DB helpers/file conventions in that file; use a real temp-file DSN, not in-memory, so pool behavior is real):

```go
func TestOpenDB_PragmasApplied(t *testing.T) {
	dsn := filepath.Join(t.TempDir(), "login.db")
	db, err := openDB(dsn)
	if err != nil {
		t.Fatalf("openDB: %v", err)
	}
	defer db.Close()

	var busy int
	if err := db.QueryRow(`PRAGMA busy_timeout`).Scan(&busy); err != nil {
		t.Fatalf("query busy_timeout: %v", err)
	}
	if busy != 5000 {
		t.Errorf("busy_timeout: got %d, want 5000", busy)
	}
	var fk int
	if err := db.QueryRow(`PRAGMA foreign_keys`).Scan(&fk); err != nil {
		t.Fatalf("query foreign_keys: %v", err)
	}
	if fk != 1 {
		t.Errorf("foreign_keys: got %d, want 1", fk)
	}
}

// TestOpenDB_ConcurrentWriteTxs pins the arch-28.1 fix contract: concurrent
// write transactions must serialize (SetMaxOpenConns(1) + busy_timeout)
// instead of failing SQLITE_BUSY. Pre-fix this fails with "database is
// locked" almost every run (unbounded pool, busy_timeout=0).
func TestOpenDB_ConcurrentWriteTxs(t *testing.T) {
	dsn := filepath.Join(t.TempDir(), "login.db")
	db, err := openDB(dsn)
	if err != nil {
		t.Fatalf("openDB: %v", err)
	}
	defer db.Close()

	var wg sync.WaitGroup
	errs := make(chan error, 20)
	for i := range 20 {
		wg.Go(func() {
			tx, err := db.BeginTx(t.Context(), nil)
			if err != nil {
				errs <- err
				return
			}
			_, err = tx.Exec(`INSERT INTO ipban (ip, added_by) VALUES (?, 'test')`,
				fmt.Sprintf("10.0.0.%d", i))
			if err != nil {
				tx.Rollback()
				errs <- err
				return
			}
			time.Sleep(5 * time.Millisecond) // widen the write-lock hold
			errs <- tx.Commit()
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent write tx: %v", err)
		}
	}
}
```

Also add `TestDSNWithPragmas` covering both DSN shapes:

```go
func TestDSNWithPragmas(t *testing.T) {
	got := dsnWithPragmas("data/login.db")
	want := "data/login.db?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)"
	if got != want {
		t.Errorf("plain dsn: got %q, want %q", got, want)
	}
	got = dsnWithPragmas("file:x?mode=memory&cache=shared")
	want = "file:x?mode=memory&cache=shared&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)"
	if got != want {
		t.Errorf("param dsn: got %q, want %q", got, want)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/login/ -run 'TestOpenDB_PragmasApplied|TestOpenDB_ConcurrentWriteTxs|TestDSNWithPragmas' -v`
Expected: `TestDSNWithPragmas` FAILS to compile or "undefined: dsnWithPragmas"; after stubbing, `PragmasApplied` fails (busy_timeout=0), `ConcurrentWriteTxs` fails with "database is locked".

- [ ] **Step 3: Implement (login)**

In `modules/login/db.go`:

```go
// dsnWithPragmas appends the per-connection pragmas every pooled
// connection must carry. busy_timeout and foreign_keys are
// per-connection settings (unlike journal_mode, which is persistent
// in the file), so they must ride the DSN — a db.Exec PRAGMA would
// only reach whichever single connection the pool hands out.
func dsnWithPragmas(dsn string) string {
	sep := "?"
	if strings.Contains(dsn, "?") {
		sep = "&"
	}
	return dsn + sep + "_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)"
}
```

In `openDB`: change `sql.Open("sqlite", dsn)` to `sql.Open("sqlite", dsnWithPragmas(dsn))`; immediately after a successful Open add:

```go
	// One writer at a time is SQLite's own model; serializing the pool
	// removes SQLITE_BUSY between our own transactions and matches the
	// TS engine's better-sqlite3 single-connection posture.
	db.SetMaxOpenConns(1)
```

Delete the `PRAGMA foreign_keys=ON` Exec block (now redundant — DSN applies it to every connection). Keep the `journal_mode=WAL` Exec (persistent pragma, one-time is correct).

- [ ] **Step 4: Run login tests to verify pass, then mirror to friends**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/login/ -v -run 'TestOpenDB|TestDSN'` → PASS, then full `go test ./modules/login/`.
Mirror the identical change + tests into `modules/friends/db.go` / `db_test.go` (friends has no `ipban`; use `INSERT INTO friendlist (profile, owner_username37, target_username37) VALUES ('main', ?, 1)` with `i` as owner for the concurrency test). Run `go test ./modules/friends/`.

- [ ] **Step 5: Race check + commit**

Run: `CGO_ENABLED=1 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/login/ ./modules/friends/`
Expected: PASS.

```bash
git add modules/login/db.go modules/login/db_test.go modules/friends/db.go modules/friends/db_test.go
git commit --no-gpg-sign -m "fix(db): serialize sqlite pool + per-connection busy_timeout/foreign_keys pragmas

arch-28.1: busy_timeout was 0 with an unbounded pool, so concurrent write
transactions (mass logout at shutdown) failed SQLITE_BUSY immediately, and
the foreign_keys pragma only reached one pooled connection, leaving the
000003 cascades inert. SetMaxOpenConns(1) matches TS better-sqlite3.

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 2: Non-zero exit when a module fails

**Files:**
- Modify: `cmd/goscape/app/app.go` (Run, ~line 147)
- Test: `cmd/goscape/app/app_failure_test.go` (new)

**Interfaces:**
- Produces: unexported `failedServicesError(sm *services.Manager, serviceMap map[string]services.Service) error` in package `app`.

**Why:** `App.Run` returns `sm.AwaitStopped(...)`, which is nil however services ended; a world crash exits status 0, breaking `Restart=on-failure`. Upstream Loki checks `ServicesByState()[services.Failed]` after stopping; the port dropped that.

- [ ] **Step 1: Write the failing test**

Create `cmd/goscape/app/app_failure_test.go`:

```go
package app

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/zsrv/goscape/pkg/dskit/modules"
	"github.com/zsrv/goscape/pkg/dskit/services"
)

func runManagerToStopped(t *testing.T, svc services.Service) *services.Manager {
	t.Helper()
	sm, err := services.NewManager(svc)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	if err := sm.StartAsync(t.Context()); err != nil {
		t.Fatalf("StartAsync: %v", err)
	}
	if err := sm.AwaitStopped(t.Context()); err != nil {
		t.Fatalf("AwaitStopped: %v", err)
	}
	return sm
}

func TestFailedServicesError_ReportsFailedModule(t *testing.T) {
	boom := errors.New("boom")
	svc := services.NewBasicService(nil, func(_ context.Context) error { return boom }, nil)
	sm := runManagerToStopped(t, svc)

	err := failedServicesError(sm, map[string]services.Service{"world": svc})
	if err == nil {
		t.Fatal("want error for failed module, got nil")
	}
	if !strings.Contains(err.Error(), "world") || !errors.Is(err, boom) {
		t.Errorf("error should name the module and wrap the cause: %v", err)
	}
}

func TestFailedServicesError_NilOnCleanStop(t *testing.T) {
	svc := services.NewIdleService(nil, nil)
	sm, err := services.NewManager(svc)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	if err := sm.StartAsync(t.Context()); err != nil {
		t.Fatalf("StartAsync: %v", err)
	}
	sm.StopAsync()
	if err := sm.AwaitStopped(t.Context()); err != nil {
		t.Fatalf("AwaitStopped: %v", err)
	}
	if err := failedServicesError(sm, map[string]services.Service{"login": svc}); err != nil {
		t.Errorf("clean stop should yield nil, got %v", err)
	}
}

func TestFailedServicesError_IgnoresStopProcessAndCanceled(t *testing.T) {
	svc := services.NewBasicService(nil,
		func(_ context.Context) error { return modules.ErrStopProcess }, nil)
	sm := runManagerToStopped(t, svc)
	if err := failedServicesError(sm, map[string]services.Service{"world": svc}); err != nil {
		t.Errorf("ErrStopProcess is a requested stop, want nil, got %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./cmd/goscape/app/ -run TestFailedServicesError -v`
Expected: FAIL "undefined: failedServicesError".

- [ ] **Step 3: Implement**

In `cmd/goscape/app/app.go`, add:

```go
// failedServicesError maps any Failed services back to their module
// names and returns a joined error, or nil if everything stopped
// cleanly. ErrStopProcess (a module requesting shutdown) and
// context.Canceled (normal stop signal) are not failures. Without
// this check App.Run returned AwaitStopped's nil regardless of how
// services ended, so a crashed module exited the process with status
// 0 — invisible to systemd Restart=on-failure and orchestrators.
// (Upstream Loki/Tempo perform the same post-stop inspection.)
func failedServicesError(sm *services.Manager, serviceMap map[string]services.Service) error {
	var errs []error
	for _, s := range sm.ServicesByState()[services.Failed] {
		fc := s.FailureCase()
		if errors.Is(fc, modules.ErrStopProcess) || errors.Is(fc, context.Canceled) {
			continue
		}
		name := "unknown"
		for m, svc := range serviceMap {
			if svc == s {
				name = m
				break
			}
		}
		errs = append(errs, fmt.Errorf("module %s failed: %w", name, fc))
	}
	return errors.Join(errs...)
}
```

Change the end of `Run` from `return sm.AwaitStopped(context.Background())` to:

```go
	if err := sm.AwaitStopped(context.Background()); err != nil {
		return err
	}
	return failedServicesError(sm, serviceMap)
```

- [ ] **Step 4: Run tests to verify pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./cmd/goscape/app/ -v` then `CGO_ENABLED=1 ... go test -race ./cmd/goscape/app/`
Expected: PASS (including the pre-existing App tests).

- [ ] **Step 5: Commit**

```bash
git add cmd/goscape/app/app.go cmd/goscape/app/app_failure_test.go
git commit --no-gpg-sign -m "fix(app): exit non-zero when a module fails

arch-28.2: App.Run returned AwaitStopped's unconditional nil, so a module
crash exited status 0 and orchestrator restart policies never fired.
Restore upstream Loki/Tempo's post-stop ServicesByState[Failed] check.

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 3: WS-bridge panic containment + quit gate

**Files:**
- Modify: `modules/world/conn_handler.go`
- Test: `modules/world/conn_handler_test.go` (new)

**Interfaces:**
- Consumes: `Server.serveConn(conn net.Conn)` (already defers `tcpWg.Done()` and recovers panics — see the gap-login-wire-1 comment at `server.go:~927`).

**Why:** `HandleConn` calls `handleTCPConn` directly, bypassing `serveConn`'s recover — a malformed WS-framed login panics the whole process (the exact bug gap-login-wire-1 fixed for TCP). It also does `tcpWg.Add(1)` with no quit gate, racing WaitGroup reuse after `Shutdown`'s `Wait`.

- [ ] **Step 1: Write the failing test**

Create `modules/world/conn_handler_test.go`. First locate the existing gap-login-wire-1 regression test (`grep -rn "gap-login-wire-1" modules/world/*_test.go`) and reuse its Server construction harness; the test below assumes a helper that yields a usable `*Server` (adapt the constructor call to match that harness exactly):

```go
package world

import (
	"net"
	"testing"
	"time"
)

// panicReadConn panics on first Read — a stand-in for any connection
// whose bytes drive the RS2 packet readers into their documented
// panic-on-underflow behavior (gap-login-wire-1).
type panicReadConn struct{ net.Conn }

func (panicReadConn) Read([]byte) (int, error) { panic("malformed login packet") }

func TestHandleConn_ContainsPanic(t *testing.T) {
	s := newHandleConnTestServer(t) // adapt: same harness as the gap-login-wire-1 test
	client, server := net.Pipe()
	defer client.Close()

	done := make(chan struct{})
	go func() {
		defer close(done)
		s.HandleConn(panicReadConn{server}) // pre-fix: panics the test binary
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("HandleConn did not return")
	}
}

func TestHandleConn_QuitGate(t *testing.T) {
	s := newHandleConnTestServer(t)
	close(s.quit)
	client, server := net.Pipe()
	defer client.Close()

	done := make(chan struct{})
	go func() { defer close(done); s.HandleConn(server) }()
	select {
	case <-done: // returned without touching tcpWg
	case <-time.After(2 * time.Second):
		t.Fatal("HandleConn did not honor closed quit channel")
	}
	// server side must be closed: a read on the peer should error out.
	client.SetReadDeadline(time.Now().Add(time.Second))
	buf := make([]byte, 1)
	if _, err := client.Read(buf); err == nil {
		t.Error("conn should be closed when quit is already closed")
	}
}
```

If no reusable harness exists, construct the minimal Server literal the gap-login-wire-1 test uses (quit channel, logNet/log loggers, zero-value cfg) as `newHandleConnTestServer`.

- [ ] **Step 2: Run test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestHandleConn -v`
Expected: `TestHandleConn_ContainsPanic` FAILS — the panic escapes and crashes the test process (or the test reports the panic). `TestHandleConn_QuitGate` fails (connection not closed / Add after Wait).

- [ ] **Step 3: Implement**

Replace the body of `HandleConn` in `modules/world/conn_handler.go`:

```go
func (s *Server) HandleConn(conn net.Conn) {
	// Shutdown already began (or completed): don't Add to tcpWg after
	// Shutdown's Wait may have started — that's WaitGroup reuse — and
	// don't start a login flow on a dying server.
	select {
	case <-s.quit:
		_ = conn.Close()
		return
	default:
	}
	s.tcpWg.Add(1)
	// serveConn (not handleTCPConn directly): it owns the tcpWg.Done and
	// the per-connection recover — without it a malformed WS-framed login
	// panics the whole process (gap-login-wire-1, see server.go).
	s.serveConn(conn)
}
```

Update the doc comment above it accordingly (keep the ownership/blocking sentences).

- [ ] **Step 4: Run tests to verify pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestHandleConn -v` then `CGO_ENABLED=1 ... go test -race ./modules/world/ -run TestHandleConn`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add modules/world/conn_handler.go modules/world/conn_handler_test.go
git commit --no-gpg-sign -m "fix(world): route WS-bridge connections through serveConn's panic recover

arch-28.3: HandleConn bypassed the gap-login-wire-1 per-connection recover,
so a malformed WS-framed login crashed the process; it also Add'd to tcpWg
with no quit gate, racing WaitGroup reuse during Shutdown.

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 4: Connection teardown + shutdown cluster (one branch, four commits)

**Files:**
- Modify: `modules/world/world_state_ops.go`, `modules/world/tick.go` (~line 58), `modules/world/server.go` (defer ~975-998, serveTCP/serveConn ~900-955, Shutdown ~851-896, removePlayerOnTick ~1634-1689, removePlayerOnDisconnect ~1703), `modules/world/client.go` (struct + newClient + flush), `modules/world/player.go` (processOut ~1160), `modules/world/world.go` (NewWorldService ~94-165), `modules/world/config.go` (~line 84), `examples/full-config-reference.yaml`
- Test: `modules/world/removal_queue_test.go`, `modules/world/client_teardown_test.go`, `modules/world/world_service_test.go`, `modules/world/server_shutdown_test.go` (new files; fold into existing test files if a natural home exists)

**Interfaces (produced, used across sub-tasks):**
- `Server.enqueueRemoval(action func())` / `Server.drainRemovals()` — guaranteed (unbounded, mutex+slice) tick-side execution.
- `client.dropConnRef()` / `client.dropTickRef()` — refcounted pooled-buffer release; last owner out releases.
- `client.flushWriteOrClose()` — flush; on error close conn so the read loop tears down.
- `worldServiceFns(...)` — extracted starting/run/stopping closures for `NewWorldService`, unit-testable.
- `Server.trackConn/untrackConn/closeLiveConns` — live-connection registry for Shutdown.

#### Task 4a: Guaranteed removal queue

**Why:** `removePlayerOnDisconnect` rides the lossy 64-slot `relayActionQueue` (drop-newest). A dropped removal ghosts the player for 60s while the tick keeps writing into a dead connection's buffers.

- [ ] **Step 1: Write the failing test** (`modules/world/removal_queue_test.go`)

```go
package world

import "testing"

// arch-28.4a: player removals must never be dropped, even when the lossy
// relay queue is saturated by RELAY_* traffic.
func TestRemovalSurvivesFullRelayQueue(t *testing.T) {
	s := &Server{relayActionQueue: make(chan func(), 64)}
	for range 64 {
		s.enqueueRelayAction(func() {}) // saturate the lossy queue
	}
	ran := false
	s.enqueueRemoval(func() { ran = true })
	s.drainRemovals()
	if !ran {
		t.Fatal("removal action was dropped")
	}
}

func TestDrainRemovalsFIFO(t *testing.T) {
	s := &Server{}
	var order []int
	for i := range 3 {
		s.enqueueRemoval(func() { order = append(order, i) })
	}
	s.drainRemovals()
	if len(order) != 3 || order[0] != 0 || order[2] != 2 {
		t.Fatalf("want FIFO [0 1 2], got %v", order)
	}
}
```

(If the `Server` literal needs a logger for `enqueueRelayAction`'s Warn, set `s.log` the way neighboring unit tests do.)

- [ ] **Step 2: Run to verify FAIL** — `go test ./modules/world/ -run 'TestRemoval|TestDrainRemovals' -v` → "undefined: enqueueRemoval".

- [ ] **Step 3: Implement**

In `modules/world/world_state_ops.go` add (and add `sync` import if missing; fields `removalQueue []func()` + `removalMu sync.Mutex` go on `Server` in server.go next to `relayActionQueue`):

```go
// enqueueRemoval posts a lifecycle-critical closure (player removal on
// disconnect) for execution on the next tick. Unlike enqueueRelayAction
// this NEVER drops: a dropped removal ghosts the player in-world for the
// 100-tick no-response timeout while the tick keeps writing into a dead
// connection's buffers. Unbounded by design — growth is bounded by the
// number of concurrently disconnecting players.
func (s *Server) enqueueRemoval(action func()) {
	s.removalMu.Lock()
	s.removalQueue = append(s.removalQueue, action)
	s.removalMu.Unlock()
}

// drainRemovals runs every pending removal in FIFO order. Must be invoked
// from the tick goroutine, before drainRelayActions, so a disconnect
// enqueued last tick is processed before any relay traffic this tick.
func (s *Server) drainRemovals() {
	s.removalMu.Lock()
	actions := s.removalQueue
	s.removalQueue = nil
	s.removalMu.Unlock()
	for _, action := range actions {
		action()
	}
}
```

In `removePlayerOnDisconnect` (server.go ~1703) change `s.enqueueRelayAction(...)` to `s.enqueueRemoval(...)` and update its comment ("relay queue" → "removal queue; guaranteed, non-lossy"). In `tick.go` insert `s.drainRemovals()` immediately BEFORE the existing `s.drainRelayActions()` call (~line 58), with a one-line comment.

- [ ] **Step 4: Verify** — `go test ./modules/world/ -run 'TestRemoval|TestDrainRemovals' -v` PASS; full `go test ./modules/world/` PASS.

- [ ] **Step 5: Commit** — `fix(world): guaranteed non-lossy queue for disconnect removals` + arch-28.4a body + trailer.

#### Task 4b: Refcounted pooled-buffer release

**Why:** the `handleTCPConn` defer returns `c.in`/`c.bufr`/`c.bufw` to `sync.Pool` on the conn goroutine while the not-yet-removed player's tick still writes into them (`processOut → flushWrite`) — a recycled writer can reach a new connection while the tick writes the old player's frames into it. The defer's `flushWrite` also races the tick's flush.

Ownership model: `bufr` is only ever touched by the conn goroutine; `bufw` and `c.in` are shared with the tick after login. Release all three only when the LAST owner exits: conn goroutine (always an owner) and tick (owner from successful login until `removePlayerOnTick`).

- [ ] **Step 1: Write the failing test** (`modules/world/client_teardown_test.go`)

```go
package world

import (
	"log/slog"
	"net"
	"sync"
	"testing"
	"time"
)

// arch-28.4b: buffers may be pool-returned only after BOTH the conn
// goroutine and the tick have dropped their refs; each side's drop is
// idempotent. Run with -race: pre-fix the conn-side release races the
// tick-side flush.
func TestClientTeardownRefcount(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()
	c := newClient(c2, time.Second, slog.Default())

	if got := c.teardownRefs.Load(); got != 1 {
		t.Fatalf("fresh client refs: got %d, want 1", got)
	}
	c.teardownRefs.Add(1) // simulate successful login (tick becomes co-owner)

	var wg sync.WaitGroup
	wg.Go(func() { c.dropConnRef() })
	wg.Go(func() {
		c.bufw.WriteByte(0) // tick-side write while conn side is tearing down
		c.dropTickRef()
	})
	wg.Wait()

	if got := c.teardownRefs.Load(); got != 0 {
		t.Fatalf("refs after both drops: got %d, want 0", got)
	}
	c.dropTickRef() // double-drop must be a no-op (idle logout + disconnect)
	c.dropConnRef()
	if got := c.teardownRefs.Load(); got != 0 {
		t.Fatalf("refs after redundant drops: got %d, want 0 (no double release)", got)
	}
}
```

- [ ] **Step 2: Run to verify FAIL** — "c.teardownRefs undefined".

- [ ] **Step 3: Implement**

`modules/world/client.go` — add fields to `client` (imports: `sync/atomic`):

```go
	// teardownRefs counts the goroutines that may still touch this
	// client's pooled buffers: the conn goroutine (always, from
	// newClient) and the tick goroutine (from successful login in
	// sendLoginOK until removePlayerOnTick). The last owner out
	// returns the buffers to their pools — releasing on the conn
	// goroutine alone recycled bufw into a NEW connection while the
	// tick was still flushing the old player's frames into it
	// (arch-28.4b).
	teardownRefs atomic.Int32
	connRefOnce  sync.Once
	tickRefOnce  sync.Once
```

In `newClient` (before return): `c := &client{...}` form — set `c.teardownRefs.Store(1)` (restructure the literal into a variable if needed). Add methods:

```go
// dropRef releases one buffer owner; the last one returns the pooled
// buffers. Callers use dropConnRef/dropTickRef (idempotent per side).
func (c *client) dropRef() {
	if c.teardownRefs.Add(-1) == 0 {
		c.in.Release()
		putBufioReader64k(c.bufr)
		putBufioWriter64k(c.bufw)
	}
}

func (c *client) dropConnRef() { c.connRefOnce.Do(c.dropRef) }
func (c *client) dropTickRef() { c.tickRefOnce.Do(c.dropRef) }
```

In `sendLoginOK`, immediately after `c.player = p` (the `appendNewPlayer` block): `c.teardownRefs.Add(1)` with a one-line comment ("tick co-owns the buffers until removePlayerOnTick").

`modules/world/server.go` — rewrite the `handleTCPConn` defer (~975-998): keep the tap/SessionEnded block and the onDemand `clientClosed` block unchanged; then:

```go
		if c.player != nil {
			// Post-login: the tick co-owns bufw/c.in until it processes
			// the removal — no flush here (it would race the tick's own
			// flushWrite) and no pool return (dropConnRef defers that to
			// whichever owner exits last).
			s.removePlayerOnDisconnect(c.player)
			c.player = nil
		} else if err := c.flushWrite(); err != nil {
			// Pre-login: this goroutine is the only writer; flush any
			// pending login reply before closing.
			s.logNet.Warn("failed to flush on connection close", "error", err, "remote_addr", conn.RemoteAddr())
		}
		conn.Close()
		c.dropConnRef()
		s.logNet.Debug("connection closed", "remote_addr", conn.RemoteAddr())
```

(The old unconditional `c.in.Release()` / `putBufioReader64k` / `putBufioWriter64k` lines are deleted — `dropRef` owns them now.)

At the END of `removePlayerOnTick` (server.go, after `s.removePlayerInternal(p)`):

```go
	// Last tick-side touch of this connection's buffers: drop the tick's
	// ref (idempotent — the idle-logout and disconnect paths can both
	// land here for the same player).
	if p.client != nil {
		p.client.dropTickRef()
	}
```

- [ ] **Step 4: Verify** — `CGO_ENABLED=1 go test -race ./modules/world/ -run TestClientTeardownRefcount -v` PASS; full `-race` package run PASS.

- [ ] **Step 5: Commit** — `fix(world): refcount pooled connection buffers across conn/tick teardown` + arch-28.4b body + trailer.

#### Task 4c: Shutdown closes live conns + serverDone restructure

**Why:** (i) `Shutdown` closes only the listener then blocks on `tcpWg.Wait()`; a client that keeps sending re-arms its read deadline forever → shutdown hangs (the dead TODO block admits this). (ii) `NewWorldService.stoppingFn` blocks on `<-serverDone`, which only `runFn`'s goroutine writes; `BasicService` legally runs stoppingFn WITHOUT runFn when the service context is canceled after a successful (slow — CRC + RPCs) startingFn → permanent deadlock.

- [ ] **Step 1: Write the failing tests**

`modules/world/world_service_test.go`:

```go
package world

import (
	"errors"
	"testing"
	"time"
)

// arch-28.4c: BasicService runs stoppingFn without runFn when the service
// context is canceled between Starting and Running. stoppingFn blocks on
// <-serverDone, so Run must be spawned by startingFn (which always runs),
// not by runFn (which may not).
func TestWorldServiceStoppingWithoutRun(t *testing.T) {
	runCalled := make(chan struct{})
	fns := worldServiceFns(
		func() error { <-runCalled; return nil },  // run: blocks until shutdown
		func() { close(runCalled) },               // shutdown: unblocks run
		func() bool { return false },              // gracefulExit
		nil,                                       // lc close
		nil,                                       // fc close
		func() error { return nil },               // starting body (CRC/RPCs stand-in)
		func() []interface{ AwaitTerminated(ctx context.Context) error } { return nil },
	)
	if err := fns.starting(t.Context()); err != nil {
		t.Fatalf("starting: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- fns.stopping(nil) }() // NOTE: run fn deliberately never invoked
	select {
	case err := <-done:
		if err != nil && !errors.Is(err, nil) {
			t.Fatalf("stopping: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("stoppingFn deadlocked without runFn (pre-fix behavior)")
	}
}
```

**Note to implementer:** the exact `worldServiceFns` seam signature above is a sketch — shape it minimally so that (a) `NewWorldService`'s public signature and observable behavior are unchanged, (b) the three closures are constructable in a test without a real `*Server` (inject `run func() error`, `shutdown func()`, `graceful func() bool`, the starting-body work, `servicesToWaitFor`, and the two client Close funcs), and (c) the test above compiles against your shape. Keep the seam unexported.

`modules/world/server_shutdown_test.go` — integration-style: reuse whichever existing harness boots a real `Server` on `127.0.0.1:0` (grep for `net.Listen` / `Run()` in `modules/world/*_test.go`; the drainConn tests from commit 85876fa7 exercise real conns). Test contract:

```go
// arch-28.4c: a client that keeps sending must not wedge Shutdown — the
// live-conn registry closes it. Pre-fix: read deadline re-arms per read
// and tcpWg.Wait blocks until the test times out.
func TestShutdownClosesChattyConn(t *testing.T) {
	s := startTestServer(t) // adapt to the existing harness
	conn, err := net.Dial("tcp", s.tcpListener.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	stop := make(chan struct{})
	go func() { // chatty client: 1 byte / 50ms keeps the read deadline fresh
		for {
			select {
			case <-stop:
				return
			case <-time.After(50 * time.Millisecond):
				if _, err := conn.Write([]byte{0}); err != nil {
					return
				}
			}
		}
	}()
	defer close(stop)

	done := make(chan struct{})
	go func() { defer close(done); s.Shutdown() }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Shutdown wedged on a chatty connection")
	}
}
```

- [ ] **Step 2: Run to verify FAIL** — worldServiceFns undefined; after stubbing pre-fix wiring, `TestWorldServiceStoppingWithoutRun` times out at the 5s guard and `TestShutdownClosesChattyConn` times out at 10s.

- [ ] **Step 3: Implement**

`modules/world/server.go` — add to `Server`: `liveConns map[net.Conn]struct{}` + `liveConnsMu sync.Mutex`. Add:

```go
// trackConn registers an accepted connection so Shutdown can close it.
// The read loop re-arms its deadline on every read, so without an
// explicit close a chatty client keeps its goroutine alive and
// tcpWg.Wait never returns.
func (s *Server) trackConn(conn net.Conn) {
	s.liveConnsMu.Lock()
	if s.liveConns == nil {
		s.liveConns = make(map[net.Conn]struct{})
	}
	s.liveConns[conn] = struct{}{}
	s.liveConnsMu.Unlock()
}

func (s *Server) untrackConn(conn net.Conn) {
	s.liveConnsMu.Lock()
	delete(s.liveConns, conn)
	s.liveConnsMu.Unlock()
}

func (s *Server) closeLiveConns() {
	s.liveConnsMu.Lock()
	defer s.liveConnsMu.Unlock()
	for conn := range s.liveConns {
		_ = conn.Close()
	}
}
```

In `serveConn`: `s.trackConn(conn)` as the first statement of the function body (before the deferred recover runs the handler) and `defer s.untrackConn(conn)` right after `defer s.tcpWg.Done()`. (Both HandleConn and serveTCP flow through serveConn after Task 3.)

In `Shutdown`, insert after `s.tcpListener.Close()`:

```go
	// Close every accepted connection: read loops re-arm their deadlines
	// per read, so without this a connected client blocks tcpWg.Wait
	// indefinitely. Closing is safe concurrently with in-flight reads and
	// writes; each conn goroutine exits through its normal error path
	// (enqueueing its player's removal for the tick's final save-all).
	s.closeLiveConns()
```

Delete the commented-out dead block + TODOs at the bottom of `Shutdown` (lines ~890-895).

`modules/world/world.go` — extract the three closures into the `worldServiceFns` seam per the test, and move the `go serv.Run()` spawn to the END of the starting fn:

```go
	// Spawn Run here, not in runFn: BasicService legally runs stoppingFn
	// WITHOUT runFn (service context canceled between Starting and
	// Running — reachable because this startingFn does slow work: CRC
	// compute + WorldStartup/WorldConnect RPCs). stoppingFn blocks on
	// <-serverDone, so the goroutine that feeds serverDone must be alive
	// by the time startingFn returns nil (arch-28.4c).
	go func() {
		defer close(serverDone)
		serverDone <- run()
	}()
	return nil
```

runFn keeps only the `select { case <-ctx.Done(): return nil; case err := <-serverDone: ... }` block (unchanged semantics: error → failure, gracefulExit → nil, otherwise "server stopped unexpectedly"). stoppingFn unchanged. `NewWorldService` becomes a thin wrapper wiring `serv.Run`, `serv.Shutdown`, `func() bool { return serv.shutdownGraceful }`, the CRC/RPC/content-watch starting body, `servicesToWaitFor`, and the lc/fc closes into `worldServiceFns`.

- [ ] **Step 4: Verify** — targeted tests PASS, then `CGO_ENABLED=1 go test -race ./modules/world/` PASS.

- [ ] **Step 5: Commit** — `fix(world): close live conns on Shutdown; spawn Run from startingFn to unhang stop-during-Starting` + arch-28.4c body + trailer.

#### Task 4d: Write-timeout default 2s + flush-error teardown

**Why:** `processOut` flushes each player's socket ON the tick goroutine with a 30s default write deadline — one stalled client freezes every player's tick for 30s; the flush error is then ignored so the dead conn lingers. A tick-based protocol has no use for a 30s socket flush budget.

- [ ] **Step 1: Write the failing test** (append to `modules/world/client_teardown_test.go`)

```go
type errWriteConn struct {
	net.Conn
	closed atomic.Bool
}

func (c *errWriteConn) Write([]byte) (int, error) { return 0, errors.New("stalled peer") }
func (c *errWriteConn) Close() error              { c.closed.Store(true); return c.Conn.Close() }
func (*errWriteConn) SetWriteDeadline(time.Time) error { return nil }

// arch-28.4d: a write failure means the socket is dead or the client
// stopped reading — close the conn so the read loop tears the connection
// down through the normal disconnect path instead of ignoring the sticky
// bufio error forever.
func TestFlushWriteOrCloseClosesOnError(t *testing.T) {
	p1, p2 := net.Pipe()
	defer p1.Close()
	ec := &errWriteConn{Conn: p2}
	c := newClient(ec, time.Second, slog.Default())
	c.bufw.WriteByte(0xFF) // something to flush
	c.flushWriteOrClose()
	if !ec.closed.Load() {
		t.Fatal("conn not closed after flush error")
	}
}

func TestWriteTimeoutDefault(t *testing.T) {
	var cfg Config
	fs := flag.NewFlagSet("t", flag.PanicOnError)
	cfg.RegisterFlagsAndApplyDefaults("world", fs) // adapt name/signature to config.go
	if cfg.TCPServerWriteTimeout != 2*time.Second {
		t.Fatalf("default write timeout: got %v, want 2s", cfg.TCPServerWriteTimeout)
	}
}
```

(Adapt the default-registration call to the exact `RegisterFlagsAndApplyDefaults` signature in `modules/world/config.go`; mirror how existing config default tests in the repo invoke it, if any exist — `grep -rn "RegisterFlagsAndApplyDefaults" modules/world/*_test.go cmd/`.)

- [ ] **Step 2: Run to verify FAIL** — `flushWriteOrClose` undefined; default is 30s.

- [ ] **Step 3: Implement**

`modules/world/client.go`:

```go
// flushWriteOrClose flushes the buffered writer; on failure it closes the
// conn so the reader goroutine exits and tears the connection down through
// the normal disconnect path (TS: socket error event → close). bufio's
// error is sticky, so without the close a dead connection lingered,
// silently receiving nothing, until the read-side timeout.
func (c *client) flushWriteOrClose() {
	if err := c.flushWrite(); err != nil {
		_ = c.conn.Close()
	}
}
```

`modules/world/player.go` `processOut` (~line 1160): `p.client.flushWrite()` → `p.client.flushWriteOrClose()`.

`modules/world/config.go` (~line 84): default `30*time.Second` → `2*time.Second`, and extend the flag help string: `"Write timeout for TCP server (per-flush budget on the tick goroutine — a stalled client blocks the world tick for at most this long)"`.

`examples/full-config-reference.yaml`: update the `tcp_server_write_timeout` value and comment to `2s` (grep the key; also `grep -rn "tcp-write-timeout\|tcp_server_write_timeout" production/helm examples/` and update any other occurrence).

- [ ] **Step 4: Verify** — targeted tests PASS; `go test ./modules/world/ ./cmd/...`; `make validate-example-configs` (needs the binary: `make goscape` first) PASS.

- [ ] **Step 5: Commit** — `fix(world): 2s tick-flush write budget; close conn on flush error` + arch-28.4d body (mention one-stalled-client-freezes-world) + trailer.

#### Task 4 wrap-up

- [ ] Run the full suite on the branch: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...` and `CGO_ENABLED=1 ... go test -race ./modules/world/`. Expected: all PASS.

---

### Task 5: Logout-save bounded retry

**Files:**
- Modify: `modules/world/server.go` (removePlayerOnTick's login goroutine ~1642-1658, `NewServer` for the new field)
- Test: `modules/world/logout_save_retry_test.go` (new)

**Interfaces:**
- Produces: `Server.sendPlayerLogoutWithRetry(username string, save []byte)` (blocking; called inside the existing saveWg goroutine) and `Server.logoutSaveRetryDelay time.Duration` field (default `2*time.Second`, set in `NewServer`; tests override).

**Why:** a graceful-logout save gets exactly one 5s `PlayerLogout` attempt; a login-service restart in that window silently loses up to ~15 minutes of progress (autosave cadence), then resurfaces as an M25 `DataLoss` login reject. TS's in-process worker queue had no such window.

- [ ] **Step 1: Write the failing test**

Find the existing `LoginClient` fake used by world tests (`grep -rn "PlayerLogout" modules/world/*_test.go` — reuse its type or add a minimal local fake implementing the `LoginClient` interface from `modules/world/login_client.go`):

```go
package world

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

type flakyLoginClient struct {
	failFirst int32 // number of leading calls that fail
	calls     atomic.Int32
	// embed or stub the remaining LoginClient methods per the existing fake
}

func (f *flakyLoginClient) PlayerLogout(ctx context.Context, req *loginpb.PlayerLogoutRequest, _ ...grpc.CallOption) (*loginpb.PlayerLogoutResponse, error) {
	if f.calls.Add(1) <= f.failFirst {
		return nil, errors.New("login server restarting")
	}
	return &loginpb.PlayerLogoutResponse{}, nil
}

// arch-28.5: a transient login-service outage at logout must not lose the
// save — retry up to 3 attempts total.
func TestLogoutSaveRetriesTransientFailure(t *testing.T) {
	fake := &flakyLoginClient{failFirst: 2}
	s := newRetryTestServer(t, fake) // minimal Server: loginClient, bridgesCtx, log, cfg
	s.logoutSaveRetryDelay = time.Millisecond

	s.sendPlayerLogoutWithRetry("player_one", []byte{1, 2, 3})
	if got := fake.calls.Load(); got != 3 {
		t.Fatalf("calls: got %d, want 3 (2 failures + 1 success)", got)
	}
}

func TestLogoutSaveGivesUpAfterMaxAttempts(t *testing.T) {
	fake := &flakyLoginClient{failFirst: 99}
	s := newRetryTestServer(t, fake)
	s.logoutSaveRetryDelay = time.Millisecond

	s.sendPlayerLogoutWithRetry("player_one", []byte{1})
	if got := fake.calls.Load(); got != 3 {
		t.Fatalf("calls: got %d, want exactly 3 attempts", got)
	}
}
```

(`newRetryTestServer` constructs the minimal `Server` literal — `loginClient`, `bridgesCtx: t.Context()` equivalent via `context.WithCancel`, `log: slog.Default()`, zero-value cfg — mirroring how other server unit tests construct partial Servers.)

- [ ] **Step 2: Run to verify FAIL** — `sendPlayerLogoutWithRetry` undefined.

- [ ] **Step 3: Implement**

`modules/world/server.go`:

```go
// Logout saves get a bounded retry (arch-28.5): TS's login "server" is an
// in-process worker whose message queue survives with the process, so a
// momentary outage never lost a save; the gRPC split introduced a loss
// window (last-autosave rollback, up to ~15 min) that a couple of retries
// close for restart-blip outages. Retries abort early once bridgesCtx is
// cancelled (shutdown's waitForSaveFlush stays bounded).
const logoutSaveAttempts = 3

func (s *Server) sendPlayerLogoutWithRetry(username string, save []byte) {
	req := &loginpb.PlayerLogoutRequest{
		NodeId:   int32(s.cfg.NodeID),
		Profile:  s.cfg.NodeProfile,
		Username: username,
		Save:     save,
	}
	for attempt := 1; ; attempt++ {
		ctx, cancel := context.WithTimeout(s.bridgesCtx, bridgeCallTimeout)
		_, err := s.loginClient.PlayerLogout(ctx, req)
		cancel()
		if err == nil {
			return
		}
		if attempt >= logoutSaveAttempts || s.bridgesCtx.Err() != nil {
			s.log.Error("PlayerLogout RPC failed; save lost until next login",
				slog.String("username", username), slog.Int("attempts", attempt), slog.Any("err", err))
			return
		}
		s.log.Warn("PlayerLogout RPC failed; retrying",
			slog.String("username", username), slog.Int("attempt", attempt), slog.Any("err", err))
		select {
		case <-time.After(s.logoutSaveRetryDelay):
		case <-s.bridgesCtx.Done():
		}
	}
}
```

Add `logoutSaveRetryDelay time.Duration` to `Server`, set to `2 * time.Second` in `NewServer` next to the other bridge wiring. Replace the body of the existing login goroutine in `removePlayerOnTick` (keep `saveWg.Add(1)`/`defer Done` and the username/save capture) with a call to `s.sendPlayerLogoutWithRetry(username, save)`, updating the surrounding comment (single-attempt note → bounded-retry note). Leave the friends `PlayerLogout` fan-out untouched (best-effort presence; self-heals via snapshot resync).

- [ ] **Step 4: Verify** — targeted tests PASS; `CGO_ENABLED=1 go test -race ./modules/world/ -run TestLogoutSave` PASS; full package PASS. Note: total worst-case retry time (3×5s + 2 delays) exceeds `playerSaveFlushTimeout` (7s) — that is intentional; shutdown's `bridgesCancel` aborts residual retries via `bridgesCtx.Err()`. State this in the commit body.

- [ ] **Step 5: Commit** — `fix(world): bounded retry for logout saves` + arch-28.5 body + trailer.

---

### Task 6: CI workflow

**Files:**
- Create: `.github/workflows/go.yml`

**Why:** the only workflow is proto linting; the ~580-file test suite and the documented `-race` convention are enforced by discipline alone.

- [ ] **Step 1: Write the workflow**

```yaml
name: go

on:
  push:
    branches: ['rev-*']
  pull_request:
    branches: ['rev-*']

jobs:
  build-test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
      - name: Build
        run: CGO_ENABLED=0 go build -trimpath ./...
      - name: Test
        run: go test ./...

  race:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
      - name: Race-detector tests (concurrent packages)
        run: CGO_ENABLED=1 go test -race ./modules/... ./pkg/dskit/... ./pkg/io/...
```

- [ ] **Step 2: Validate structurally**

Run: `python3 -c "import yaml,sys; yaml.safe_load(open('.github/workflows/go.yml'))"` (or `yq . .github/workflows/go.yml` if available). Then run both job commands locally to confirm they pass:
`CGO_ENABLED=0 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build -trimpath ./...`
`GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`
`CGO_ENABLED=1 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/... ./pkg/dskit/... ./pkg/io/...`
Expected: all PASS.

- [ ] **Step 3: Commit** — `ci: build, test, and race-detector workflow for rev-* branches` + arch-28.6 body + trailer.

---

### Task 7: Documentation — PORTING.md arc row

**Files:**
- Modify: `docs/PORTING.md`

- [ ] **Step 1:** Read the tail of the arcs section (`grep -n "Arc 2[0-9]" docs/PORTING.md | tail`) to confirm the next arc number (expected: Arc 28).
- [ ] **Step 2:** Append an "Arc 28 — Architecture fix arc (2026-07-02 five-agent review)" entry in the established Arc-27 row style: one line per fix (arch-28.1 … 28.6) with finding → fix → branch → merge commit SHA (fill SHAs after merges), plus a pointer to the plan doc `docs/superpowers/plans/2026-07-02-arch-fix-arc.md` and a note that the MED/LOW findings remain open (tracked in auto-memory `arch_review_2026_07_02_findings.md`).
- [ ] **Step 3:** Commit — `docs(porting): record Arc 28 architecture fix arc` + trailer.

---

## Execution notes

- Task order is fixed: 1 → 2 → 3 → 4(a-d) → 5 → 6 → 7. Task 4 depends on Task 3 (HandleConn must route through serveConn before the conn registry lives in serveConn). Tasks 1, 2, 6 are independent of the rest but keep the order anyway — it front-loads the smallest risk.
- Merge cadence: finish a task's branch (tests green, `-race` green on touched packages) → merge `--no-ff` into `rev-274` → next task branches from updated `rev-274`.
- The pre-existing untracked `goscape` binary and `.superpowers/` in the main checkout are not part of this arc; never `git add` them.
