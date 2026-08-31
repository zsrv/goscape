# SEC1 Hardening (M-1, M-2, M-7, M-8, M-12) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close five Medium findings from the 2026-08-20 goscape security review: unrecovered login/logout script panics (M-1), tick goroutine blocked by slow socket writes (M-2), plaintext password at Debug (M-7), portal CSRF/headers/timeouts/verify-GET (M-8), Helm + `/debug/status` hardening (M-12).

**Architecture:** Each finding is an isolated change in one module. M-1 reuses the existing `recoverPlayer` helper and the `autosavePlayers` + `waitForSaveFlush` pair. M-2 inserts an `outboundWriter` (bounded queue + goroutine) between `bufio.Writer` and `net.Conn` so the existing single-writer `bufw` ownership model is untouched; every `conn.Close()` on a `*client` becomes `c.closeConn()` which drains the queue then closes. M-7 is a `slog.LogValuer`. M-8 extends the portal middleware. M-12 is Helm values/templates plus one new ondemand flag.

**Tech Stack:** Go 1.26 (`GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go ...` for every go command; `CGO_ENABLED=1` only when adding `-race`), `log/slog`, `net/http`, Helm 3 (`helm lint`/`helm template`), `html/template`.

**Spec:** `docs/superpowers/specs/2026-08-22-sec1-hardening-design.md` (deviation IDs SEC1-D1..D3 live there; code comments must cite them).

## Global Constraints

- Every `go` invocation: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go ...`. Commits: `git commit --no-gpg-sign`.
- Work on branch `sec1-hardening` created from `rev-274` (`git checkout -b sec1-hardening rev-274`). Do not touch `main`.
- Every behavioural divergence from Engine-TS carries a `DEVIATION SEC1-Dn:` comment citing the spec table. Tasks below say exactly where.
- Compile-all gate after each task: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run '^$' ./...` must pass (memory: cross-rev port methodology).
- Do not change wire formats or packet ordering. M-2 must deliver the same bytes in the same order; only *who* calls `net.Conn.Write` changes.
- Modern Go (1.26): `for i := range n`, `min`/`max` builtins, `slices`/`maps`, `sync.WaitGroup.Go` where already used in the package, `t.Context()` in tests (never inside `t.Cleanup`).
- `gofmt` clean; `go vet ./modules/... ./pkg/io/...` clean.

---

### Task 1: M-7 — redact the login request in logs

**Files:**
- Modify: `pkg/io/protocol/login/req/req.go` (add `LogValue` after the `GameLogin` struct, ~line 30)
- Test: `pkg/io/protocol/login/req/req_logvalue_test.go` (create)

**Interfaces:**
- Produces: `func (q GameLogin) LogValue() slog.Value` — value receiver so both `req` (a value at `modules/world/server_login.go:113`) and `&req` log redacted.

- [ ] **Step 1: Write the failing test**

```go
package req

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

// Pins SEC1 M-7: logging a GameLogin at any level must never emit the
// password, ISAAC seed or CRC table. Username/revision/uid stay visible
// because they are the operationally useful fields.
func TestGameLogin_LogValueRedacts(t *testing.T) {
	q := GameLogin{
		Username:         "alice",
		Password:         "s3cretpw",
		ArchiveChecksums: [9]uint32{0xdeadbeef},
		ISAACSeed:        [4]uint32{0x11223344},
		UID:              42,
		Revision:         274,
		LowMemory:        true,
	}
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, &slog.Options{Level: slog.LevelDebug}))
	log.Debug("unmarshalled", "req", q)
	log.Debug("unmarshalled-ptr", "req", &q)
	out := buf.String()
	for _, forbidden := range []string{"s3cretpw", "deadbeef", "3735928559", "11223344", "287454020"} {
		if strings.Contains(out, forbidden) {
			t.Fatalf("log leaked %q:\n%s", forbidden, out)
		}
	}
	for _, want := range []string{"username=alice", "revision=274", "uid=42", "low_memory=true", "password=[redacted]"} {
		if !strings.Contains(out, want) {
			t.Fatalf("log missing %q:\n%s", want, out)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/io/protocol/login/req/ -run TestGameLogin_LogValueRedacts -v`
Expected: FAIL — `log leaked "s3cretpw"`.

- [ ] **Step 3: Implement `LogValue`**

Add `"log/slog"` to the imports of `req.go` and insert directly after the `GameLogin` struct:

```go
// LogValue implements slog.LogValuer so that logging a GameLogin (by value
// or pointer) never emits the cleartext password, the ISAAC seed or the
// CRC table. SEC1 M-7: server_login.go logs the whole request at Debug;
// without this a `log_level: debug` world wrote every player's password
// to its log. Value receiver so `*GameLogin` redacts too (Go promotes the
// method to the pointer type).
func (q GameLogin) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("username", q.Username),
		slog.String("password", "[redacted]"),
		slog.Uint64("uid", uint64(q.UID)),
		slog.Int("revision", int(q.Revision)),
		slog.Bool("low_memory", q.LowMemory),
	)
}
```

- [ ] **Step 4: Run the test and the package**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/io/protocol/login/req/ -v`
Expected: PASS (all, including existing marshal round-trip tests).

- [ ] **Step 5: Confirm the call site needs no change**

Run: `sed -n 164,168p modules/world/server_login.go` — the line `c.log.Debug("unmarshalled OpReqInitGameConnection", "req", req)` stays as is; slog picks up `LogValue` automatically. Append to the existing `// LOG-1:` comment block above it one line: `// SEC1 M-7: GameLogin.LogValue redacts password/seed/CRC table.`

- [ ] **Step 6: Commit**

```bash
git add pkg/io/protocol/login/req/req.go pkg/io/protocol/login/req/req_logvalue_test.go modules/world/server_login.go
git commit --no-gpg-sign -m "fix(world): redact password/seed from login request debug log (SEC1 M-7)"
```

---

### Task 2: M-1a — contain `[login]`/`[logout]` trigger panics per player

**Files:**
- Modify: `modules/world/tick.go:546-550` (login trigger) and `modules/world/tick.go:664-669` (logout trigger)
- Test: `modules/world/tick_trigger_recover_test.go` (create)

**Interfaces:**
- Consumes: `recoverPlayer(p *Player, op string, log *slog.Logger)` from `modules/world/tick_recovery.go`; `s.runScriptFn` seam (`modules/world/server.go:441`); `s.scriptProvider`.
- Produces: nothing new; behaviour only.

- [ ] **Step 1: Read how tests seed logins**

Run: `sed -n 100,125p modules/world/login_resync_test.go` and `grep -n 'func newTestClient' -A15 modules/world/*_test.go`. Note: `newTestClient(t)` returns `(*client, net.Conn)`; `newPlayer(c)` builds a `*Player`; `s.newPlayers = append(s.newPlayers, p)` then `s.processLogins()` drives the login path. Also run `grep -n 'scriptProvider' modules/world/server.go | head` to see the field type; `GetByTrigger` must return non-nil for the trigger to fire, so the test injects a provider. Run `grep -rn 'type stubScriptProvider\|type fakeScriptProvider\|ScriptProvider interface' modules/world/*.go | head` and reuse whatever stub exists (if none exists, build one in the test that satisfies the interface's method set with `GetByTrigger`/`GetByTriggerSpecific` returning a non-nil `&script.ScriptFile{Name: "login"}` and every other method returning zero values).

- [ ] **Step 2: Write the failing tests**

```go
package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/script"
)

// SEC1 M-1 / DEVIATION SEC1-D1: a panicking [login] trigger must not
// escape processLogins. The offending player is force-disconnected
// (recoverPlayer semantics); every other player is untouched.
func TestProcessLogins_LoginTriggerPanicIsContained(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = newPanicTriggerProvider() // see Step 1 for the stub to reuse/build
	s.runScriptFn = func(sf *script.ScriptFile, self script.ActivePlayer, target any, trigger script.ServerTriggerType, protect bool, intArgs []int, stringArgs []string) {
		if trigger == script.TriggerLogin {
			panic("boom: login trigger")
		}
	}
	c, _ := newTestClient(t)
	p := newPlayer(c)
	p.username = "alice"
	s.newPlayers = append(s.newPlayers, p)

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panic escaped processLogins: %v", r)
		}
	}()
	s.processLogins()

	if !p.requestLogout {
		t.Fatal("panicking player must be flagged for logout")
	}
}

// SEC1 M-1 / DEVIATION SEC1-D1: same for [logout]; the player is still
// removed from the world after the trigger panics.
func TestProcessLogouts_LogoutTriggerPanicIsContained(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = newPanicTriggerProvider()
	c, _ := newTestClient(t)
	p := newPlayer(c)
	p.username = "bob"
	if err := s.addPlayer(p); err != nil {
		t.Fatal(err)
	}
	slot := p.slot
	s.runScriptFn = func(sf *script.ScriptFile, self script.ActivePlayer, target any, trigger script.ServerTriggerType, protect bool, intArgs []int, stringArgs []string) {
		if trigger == script.TriggerLogout {
			panic("boom: logout trigger")
		}
	}
	p.requestLogout = true

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panic escaped processLogouts: %v", r)
		}
	}()
	s.processLogouts()

	if s.players.get(slot) != nil {
		t.Fatal("player must be removed even when [logout] panics")
	}
}
```

Note for the logout test: `tick.go:667` calls `s.runScript` directly, not `s.runScriptFn`. Step 4 switches it to `s.runScriptFn` so the seam is honoured (server.go:437 says every tick.go fire site should use the seam). If `processLogouts` has a different name, use the real one (`grep -n 'func (s \*Server) processLogouts' modules/world/tick.go`).

- [ ] **Step 3: Run the tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestProcessLogins_LoginTriggerPanicIsContained|TestProcessLogouts_LogoutTriggerPanicIsContained' -v`
Expected: FAIL with `panic escaped ...` (the panic propagates through the test's recover).

- [ ] **Step 4: Wrap both trigger calls**

Replace `tick.go:546-550`:

```go
		// Fire the LOGIN trigger if the cache has one. Sub-spec RuneScript S3.
		// DEVIATION SEC1-D1: TS has no per-player catch here (a throw
		// reaches cycle()'s catch and the process exits). goscape contains
		// the panic to this player: recoverPlayer logs, flags requestLogout
		// and closes the socket, so one corrupt save/script cannot take the
		// world down. Closure so the deferred recover runs per player.
		if s.scriptProvider != nil {
			func() {
				defer recoverPlayer(p, "loginTrigger", s.logTick)
				sf := s.scriptProvider.GetByTrigger(script.TriggerLogin, -1, -1)
				s.runScriptFn(sf, p, nil, script.TriggerLogin, true, nil, nil)
			}()
		}
```

Replace the `if logoutScript != nil { s.runScript(...) }` arm at `tick.go:666-668`:

```go
			if logoutScript != nil {
				// DEVIATION SEC1-D1 (see login trigger above): contain a
				// [logout] panic to this player. Removal continues below
				// regardless — recoverPlayer's requestLogout/close are
				// no-ops for a player already being torn down.
				func() {
					defer recoverPlayer(p, "logoutTrigger", s.logTick)
					s.runScriptFn(logoutScript, p, nil, script.TriggerLogout, true, nil, nil)
				}()
			} else {
```

If `s.logTick` is nil in `newTestServer`, use `s.log` instead — check with `grep -n 'logTick' modules/world/server_test.go modules/world/server.go | head`.

- [ ] **Step 5: Run the new tests, then the package**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestProcessLogins_LoginTriggerPanicIsContained|TestProcessLogouts_LogoutTriggerPanicIsContained' -v` → PASS.
Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/` → PASS.

- [ ] **Step 6: Commit**

```bash
git add modules/world/tick.go modules/world/tick_trigger_recover_test.go
git commit --no-gpg-sign -m "fix(world): contain [login]/[logout] trigger panics per player (SEC1 M-1, DEVIATION SEC1-D1)"
```

---

### Task 3: M-1b — autosave every player before the tick loop dies

**Files:**
- Modify: `modules/world/tick.go:33-35` (`runTickLoop`)
- Modify: `modules/world/tick_recovery.go` (add `crashSaveAll`)
- Test: `modules/world/tick_crash_save_test.go` (create)

**Interfaces:**
- Consumes: `s.autosavePlayers()` and `s.waitForSaveFlush()` (`modules/world/server_players.go`), `fakeLoginClient.autosaveReqs` (`modules/world/login_client_fake_test.go`).
- Produces: `func (s *Server) crashSaveAll(r any)` — logs the panic, fires autosaves, waits, returns. Caller re-panics.

- [ ] **Step 1: Write the failing test**

```go
package world

import (
	"testing"
	"time"
)

// SEC1 M-1 / DEVIATION SEC1-D2: an unrecovered tick-loop panic fires one
// PlayerAutosave per online player before the process dies. TS cycle()
// exits without saving.
func TestCrashSaveAll_AutosavesEveryPlayer(t *testing.T) {
	s := newTestServer(t)
	fake := newFakeLoginClient()
	s.loginClient = fake
	for _, name := range []string{"alice", "bob"} {
		c, _ := newTestClient(t)
		p := newPlayer(c)
		p.username = name
		if err := s.addPlayer(p); err != nil {
			t.Fatal(err)
		}
	}

	s.crashSaveAll("boom")

	got := map[string]bool{}
	for range 2 {
		select {
		case req := <-fake.autosaveReqs:
			got[req.Username] = true
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for autosaves; got %v", got)
		}
	}
	if !got["alice"] || !got["bob"] {
		t.Fatalf("expected autosave for alice and bob, got %v", got)
	}
}

// runTickLoop must still die (re-panic) after saving — supervisors rely
// on the crash.
func TestRunTickLoop_RepanicsAfterCrashSave(t *testing.T) {
	s := newTestServer(t)
	s.loginClient = newFakeLoginClient()
	s.tickBodyFn = func() { panic("tick boom") }
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("runTickLoop must re-panic after crash save")
		}
	}()
	s.runTickLoopWithRate(time.Millisecond)
}
```

`s.tickBodyFn` does not exist yet; Step 3 adds it as a test seam defaulting to the real per-tick body. If `runTickLoopWithRate`'s body is not already a single function, extract the for-body into `func (s *Server) tickOnce()` and make `tickBodyFn` default to `s.tickOnce` in `NewServer` next to `s.runScriptFn = s.runScript` (server.go:524). Keep the sleep/`nextTick` bookkeeping in the loop. If `newTestServer` does not go through `NewServer`, set the default in `newTestServerWithDispatcher` too (grep `runScriptFn` there to see where seams are defaulted for tests).

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestCrashSaveAll_AutosavesEveryPlayer|TestRunTickLoop_RepanicsAfterCrashSave' -v`
Expected: compile error (`crashSaveAll`, `tickBodyFn` undefined).

- [ ] **Step 3: Implement**

In `tick_recovery.go` add:

```go
// crashSaveAll is the tick loop's last act before an unrecovered panic
// kills the process: log the panic with its stack, fire a best-effort
// PlayerAutosave for every online player, and wait (bounded by
// playerSaveFlushTimeout) for the RPCs to flush. The caller re-panics
// afterwards so crash semantics for supervisors are unchanged.
//
// DEVIATION SEC1-D2: TS World.cycle's catch logs and process.exit(1)s
// without saving — up to NODE_AUTOSAVE_INTERVAL of progress was lost for
// every player. goscape saves first.
func (s *Server) crashSaveAll(r any) {
	s.log.Error("unrecovered panic in tick loop; autosaving all players before exit",
		"err", r,
		"stack", string(debug.Stack()))
	s.autosavePlayers()
	s.waitForSaveFlush()
}
```

In `tick.go`:

```go
func (s *Server) runTickLoop() {
	// DEVIATION SEC1-D2: save everyone, then let the panic continue.
	defer func() {
		if r := recover(); r != nil {
			s.crashSaveAll(r)
			panic(r)
		}
	}()
	s.runTickLoopWithRate(s.tickRate)
}
```

and give `runTickLoopWithRate` the seam: where the loop body runs the per-tick steps, call `s.tickBodyFn()`; add `tickBodyFn func()` to `Server` next to `runScriptFn` (server.go:441) with a doc comment `// tickBodyFn is the per-tick body seam (SEC1 test hook); defaults to s.tickOnce.` and default it beside `s.runScriptFn = s.runScript`. The recover lives in `runTickLoop` (not `runTickLoopWithRate`), so `TestRunTickLoop_RepanicsAfterCrashSave` must drive `runTickLoop`: change its last two lines to

```go
	s.tickRate = time.Millisecond
	s.runTickLoop()
```

- [ ] **Step 4: Run tests, then package with race**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestCrashSaveAll_AutosavesEveryPlayer|TestRunTickLoop_RepanicsAfterCrashSave' -v` → PASS.
Run: `CGO_ENABLED=1 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race -short ./modules/world/` → PASS.

- [ ] **Step 5: Commit**

```bash
git add modules/world/tick.go modules/world/tick_recovery.go modules/world/server.go modules/world/tick_crash_save_test.go
git commit --no-gpg-sign -m "fix(world): autosave all players before a tick-loop panic exits (SEC1 M-1, DEVIATION SEC1-D2)"
```

---

### Task 4: M-2 — async outbound writer so the tick never blocks on a socket

**Files:**
- Create: `modules/world/outbound.go`
- Modify: `modules/world/client.go` (struct field, `newClient`, `flushWrite`, `flushWriteOrClose`, `clientODAdapter.close`, add `closeConn`)
- Modify call sites that close a client's conn: `modules/world/tick.go:397,678`, `modules/world/tick_recovery.go:43`, `modules/world/handlers_game.go:804`, `modules/world/player.go:1327,1348`, `modules/world/server_accept.go:214`
- Modify: `modules/world/config.go:89` (flag help text)
- Test: `modules/world/outbound_test.go` (create)

**Interfaces:**
- Produces:
  - `type outboundWriter struct` with `newOutboundWriter(conn net.Conn, writeTimeout time.Duration, log *slog.Logger) *outboundWriter`
  - `func (o *outboundWriter) Write(p []byte) (int, error)` — non-blocking enqueue (copies `p`); returns `errOutboundFull` and closes `conn` when either cap is exceeded; returns `net.ErrClosed` after `Close`.
  - `func (o *outboundWriter) Close()` — idempotent; stops accepting, drains what is queued under one `writeTimeout` deadline, then closes `conn`.
  - `func (c *client) closeConn()` — calls `c.out.Close()`. **All** `*client` teardown paths use this; raw `conn.Close()` remains only for pre-client conns (`server_accept.go:63,115`, `conn_handler.go:25`) and `Server.closeLiveConns` (shutdown hard-close, leave as is).
  - Constants `maxOutboundQueueSlots = 64`, `maxOutboundQueueBytes = 256 << 10`.

Design notes the implementer must honour:
- `bufio.Writer.Flush` hands its *internal* buffer to `Write`; the bytes **must be copied** before enqueueing.
- The goroutine starts lazily on the first `Write` (`sync.Once`) so tests that build a `client` without writing leak nothing.
- Per-write deadline: `conn.SetWriteDeadline(time.Now().Add(writeTimeout))` before each `conn.Write` inside the goroutine; on any error → `conn.Close()`, mark failed, drain+discard the rest.
- `Close()`: `closeOnce` → close a `done` channel. The goroutine, on `done`, sets one absolute deadline `now+writeTimeout`, writes whatever is left in the channel (non-blocking receive loop), then `conn.Close()`. If the goroutine was never started (no writes), `Close()` closes `conn` directly.
- `flushWrite` no longer sets a deadline (the goroutine owns deadlines); it is just `c.bufw.Flush()`.

- [ ] **Step 1: Write the failing tests**

```go
package world

import (
	"bytes"
	"errors"
	"io"
	"net"
	"testing"
	"time"
)

// SEC1 M-2 / DEVIATION SEC1-D3: a peer that never reads must not block
// the writer's caller (the tick goroutine). The queue absorbs writes
// instantly and the connection is closed once a cap is exceeded.
func TestOutboundWriter_NeverBlocksCaller(t *testing.T) {
	client, server := net.Pipe() // client side never reads
	t.Cleanup(func() { client.Close(); server.Close() })
	o := newOutboundWriter(server, 50*time.Millisecond, discardLogger())

	frame := bytes.Repeat([]byte{0xAB}, 8<<10) // 8 KiB
	start := time.Now()
	var lastErr error
	for range 200 { // 1.6 MiB ≫ maxOutboundQueueBytes
		if _, err := o.Write(frame); err != nil {
			lastErr = err
			break
		}
	}
	if d := time.Since(start); d > 500*time.Millisecond {
		t.Fatalf("Write blocked for %v", d)
	}
	if lastErr == nil {
		t.Fatal("expected an overflow error once the queue cap was exceeded")
	}
	// Peer observes the close: the read end errors out promptly.
	_ = client.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 1)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := client.Read(buf); err != nil && !errors.Is(err, io.EOF) && err.Error() != "EOF" {
			return // closed pipe
		} else if errors.Is(err, io.EOF) {
			return
		}
	}
	t.Fatal("peer never saw the connection close")
}

// Bytes queued before Close are delivered in order, then the peer sees EOF.
func TestOutboundWriter_CloseDrainsInOrder(t *testing.T) {
	client, server := net.Pipe()
	t.Cleanup(func() { client.Close() })
	o := newOutboundWriter(server, time.Second, discardLogger())

	got := make(chan []byte, 1)
	go func() {
		b, _ := io.ReadAll(client) // returns on EOF after server closes
		got <- b
	}()
	for _, chunk := range [][]byte{[]byte("one"), []byte("two"), []byte("three")} {
		if _, err := o.Write(chunk); err != nil {
			t.Fatal(err)
		}
	}
	o.Close()
	o.Close() // idempotent

	select {
	case b := <-got:
		if string(b) != "onetwothree" {
			t.Fatalf("got %q", b)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("peer never received EOF")
	}
	if _, err := o.Write([]byte("late")); !errors.Is(err, net.ErrClosed) {
		t.Fatalf("write after close: got %v, want net.ErrClosed", err)
	}
}

// A write that the peer stalls past writeTimeout closes the conn instead
// of hanging the goroutine forever.
func TestOutboundWriter_WriteTimeoutCloses(t *testing.T) {
	client, server := net.Pipe()
	t.Cleanup(func() { client.Close(); server.Close() })
	o := newOutboundWriter(server, 30*time.Millisecond, discardLogger())
	if _, err := o.Write([]byte("x")); err != nil {
		t.Fatal(err)
	}
	// Nobody reads `client`; the goroutine's conn.Write must time out and close.
	_ = client.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 4)
	_, err := client.Read(buf) // unblocks with EOF/closed once server side closes
	if err == nil {
		// first Read may deliver "x" if the pipe handed it over before timeout; read again
		_, err = client.Read(buf)
	}
	if err == nil {
		t.Fatal("expected the server side to close after write timeout")
	}
}

// End-to-end through client: closeConn after writeOut+flush delivers the
// frame, and a second closeConn is harmless.
func TestClientCloseConn_DeliversPendingBytes(t *testing.T) {
	c, peer := newTestClient(t)
	got := make(chan []byte, 1)
	go func() { b, _ := io.ReadAll(peer); got <- b }()
	c.bufw.Write([]byte{7, 8, 9})
	if err := c.flushWrite(); err != nil {
		t.Fatal(err)
	}
	c.closeConn()
	c.closeConn()
	select {
	case b := <-got:
		if !bytes.Equal(b, []byte{7, 8, 9}) {
			t.Fatalf("got %v", b)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("pending bytes not delivered before close")
	}
}
```

Check `newTestClient`'s peer conn: run `grep -n 'func newTestClient' -A15 modules/world/*_test.go`. If it returns the *server* side rather than the peer, adapt the last test to read from the correct end (the test needs the end that `c.conn` writes *to*).

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestOutboundWriter|TestClientCloseConn' -v`
Expected: compile error (`newOutboundWriter`, `closeConn` undefined).

- [ ] **Step 3: Create `modules/world/outbound.go`**

```go
package world

import (
	"errors"
	"log/slog"
	"net"
	"sync"
	"time"
)

// Outbound queue caps. A healthy client drains a few KiB per tick; a
// client that stops reading fills its kernel window within seconds and
// then hits these. DEVIATION SEC1-D3: TS socket.write buffers without
// bound and never disconnects; goscape bounds memory and closes.
const (
	maxOutboundQueueSlots = 64
	maxOutboundQueueBytes = 256 << 10
)

var errOutboundFull = errors.New("outbound queue overflow")

// outboundWriter sits between a client's bufio.Writer and its net.Conn.
// Write never blocks: it copies the bytes onto a bounded queue that a
// per-client goroutine drains to the socket under a per-write deadline.
// This is what keeps a stalled or slow-reading client from holding the
// tick goroutine inside net.Conn.Write (SEC1 M-2).
//
// Single producer (whoever owns bufw: the conn goroutine pre-login, the
// tick goroutine post-login, the OnDemand pump in state 2 — each already
// serialised by the existing ownership rules), single consumer (the
// goroutine). Close is safe from any goroutine.
type outboundWriter struct {
	conn         net.Conn
	writeTimeout time.Duration
	log          *slog.Logger

	queue chan []byte
	done  chan struct{}

	mu     sync.Mutex // guards queuedBytes + closed + failed
	queued int
	closed bool
	failed bool

	startOnce sync.Once
	closeOnce sync.Once
	exited    chan struct{}
}

func newOutboundWriter(conn net.Conn, writeTimeout time.Duration, log *slog.Logger) *outboundWriter {
	return &outboundWriter{
		conn:         conn,
		writeTimeout: writeTimeout,
		log:          log,
		queue:        make(chan []byte, maxOutboundQueueSlots),
		done:         make(chan struct{}),
		exited:       make(chan struct{}),
	}
}

// Write enqueues a copy of p. It returns net.ErrClosed after Close (or
// after a socket failure) and errOutboundFull — having closed the
// connection — when either cap would be exceeded.
func (o *outboundWriter) Write(p []byte) (int, error) {
	o.mu.Lock()
	if o.closed || o.failed {
		o.mu.Unlock()
		return 0, net.ErrClosed
	}
	if o.queued+len(p) > maxOutboundQueueBytes || len(o.queue) >= maxOutboundQueueSlots {
		o.failed = true
		o.mu.Unlock()
		o.log.Warn("outbound queue overflow; closing connection",
			"remote_addr", o.conn.RemoteAddr(), "queued_bytes", o.queued, "frame_bytes", len(p))
		_ = o.conn.Close()
		return 0, errOutboundFull
	}
	o.queued += len(p)
	o.mu.Unlock()

	buf := make([]byte, len(p))
	copy(buf, p)
	o.startOnce.Do(func() { go o.run() })
	o.queue <- buf // cannot block: slot check above holds under single producer
	return len(p), nil
}

// Close stops accepting writes, lets the goroutine drain what is queued
// under one writeTimeout, then closes the socket. Idempotent.
func (o *outboundWriter) Close() {
	o.closeOnce.Do(func() {
		o.mu.Lock()
		o.closed = true
		started := false
		o.mu.Unlock()
		o.startOnce.Do(func() {
			// Never written: nothing to drain, close directly and mark the
			// goroutine as never-started so we don't wait on it.
			close(o.exited)
		})
		select {
		case <-o.exited:
			// Either the goroutine already exited (socket failure) or it was
			// never started (branch above).
		default:
			started = true
		}
		close(o.done)
		if !started {
			_ = o.conn.Close()
		}
	})
}

func (o *outboundWriter) run() {
	defer close(o.exited)
	defer func() { _ = o.conn.Close() }()
	for {
		select {
		case buf := <-o.queue:
			if !o.writeOne(buf) {
				o.discardAll()
				return
			}
		case <-o.done:
			// Drain under one absolute deadline.
			_ = o.conn.SetWriteDeadline(time.Now().Add(o.writeTimeout))
			for {
				select {
				case buf := <-o.queue:
					if _, err := o.conn.Write(buf); err != nil {
						o.discardAll()
						return
					}
				default:
					return
				}
			}
		}
	}
}

// writeOne writes buf under a fresh deadline; false on failure.
func (o *outboundWriter) writeOne(buf []byte) bool {
	if o.writeTimeout > 0 {
		_ = o.conn.SetWriteDeadline(time.Now().Add(o.writeTimeout))
	}
	_, err := o.conn.Write(buf)
	o.mu.Lock()
	o.queued -= len(buf)
	if err != nil {
		o.failed = true
	}
	o.mu.Unlock()
	if err != nil {
		o.log.Debug("outbound write failed; closing connection", "remote_addr", o.conn.RemoteAddr(), "err", err)
		return false
	}
	return true
}

func (o *outboundWriter) discardAll() {
	for {
		select {
		case <-o.queue:
		default:
			return
		}
	}
}
```

**Careful:** the `Close()` start/exited logic above is subtle. Simplify if you can prove equivalence; the required behaviours are exactly the four tests. One known-simpler alternative: always start the goroutine in `newOutboundWriter` but have it exit when `done` closes — this leaks one goroutine per `client` built in tests that never call `closeConn`. Prefer the lazy start; if `go test -race` flags the `started` dance, restructure with a single `state` int under `mu` (0=idle,1=running,2=closed) and keep the tests green.

- [ ] **Step 4: Wire into `client`**

In `client.go`:
- Add field `out *outboundWriter` to `client`.
- In `newClient`: build `out := newOutboundWriter(conn, writeTimeout, logger)` and use `bufw: getBufioWriter64k(out)` instead of `getBufioWriter64k(conn)`; set `out: out`. Check `getBufioWriter64k` takes an `io.Writer` (it does: `bufio.Writer.Reset(w)`); if it is typed `net.Conn`, widen it to `io.Writer`.
- Replace `flushWrite`:

```go
// flushWrite hands bufw's bytes to the outbound writer. Never blocks on
// the socket (SEC1 M-2): the writer goroutine owns write deadlines.
func (c *client) flushWrite() error {
	return c.bufw.Flush()
}
```

- Replace `flushWriteOrClose`'s `_ = c.conn.Close()` with `c.closeConn()`.
- Add:

```go
// closeConn is the one way to close a client's connection: it stops new
// writes, drains already-flushed frames (bounded by writeTimeout) and
// then closes the socket, so a logout byte flushed just before close is
// still delivered. Idempotent. The reader goroutine unblocks when the
// socket finally closes and runs the normal teardown.
func (c *client) closeConn() {
	if c.out != nil {
		c.out.Close()
		return
	}
	_ = c.conn.Close()
}
```

- `clientODAdapter.close()` → `a.c.closeConn()`.
- Update the doc comment above `clientODAdapter` ("close calls conn.Close()...") to say it calls `closeConn` (drain-then-close).

- [ ] **Step 5: Replace the call sites**

Apply exactly these edits (verify each line with `sed -n` first; line numbers are from rev-274 @ 60db51ba and may have shifted by Tasks 2–3):
- `tick.go:397` `_ = p.client.conn.Close()` → `p.client.closeConn()`
- `tick.go:678` `_ = p.client.conn.Close()` → `p.client.closeConn()`
- `tick_recovery.go:43` `_ = p.client.conn.Close()` → `p.client.closeConn()` (keep the `p.client != nil && p.client.conn != nil` guard)
- `handlers_game.go:804` `_ = p.client.conn.Close()` → `p.client.closeConn()`
- `player.go:1327` and `:1348` `c.conn.Close()` → `c.closeConn()`
- `server_accept.go:214` `conn.Close()` → `c.closeConn()` (the pre-login flush at :210 stays before it; the drain makes the login reply reach the wire)

Run `grep -n 'conn.Close()' modules/world/*.go | grep -v _test` afterwards; the only remaining hits must be `conn_handler.go`, `server_accept.go:63,115`, `friends_client.go`, `login_client.go`, `closeLiveConns` in `server.go` (if present), and `outbound.go` itself.

- [ ] **Step 6: Config help text**

`config.go:89`: change the help string to `"Per-write deadline for the client's outbound socket writer and the drain budget on close. Socket writes never run on the tick goroutine (SEC1 M-2)."`. Update the matching comment in `examples/full-config-reference.yaml` (grep `tcp_server_write_timeout`).

- [ ] **Step 7: Run tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestOutboundWriter|TestClientCloseConn' -v` → PASS.
Run: `CGO_ENABLED=1 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race -short ./modules/world/ ./modules/ondemand/` → PASS. Expect some existing tests that read from a `net.Pipe` peer synchronously after a flush to still pass (delivery is asynchronous but ordered; if a test asserted that bytes were on the wire *before* `flushWrite` returned, it must now read with a deadline — fix such tests by reading in a goroutine/with `SetReadDeadline`, never by sleeping).
Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run '^$' ./...` → compiles.

- [ ] **Step 8: Commit**

```bash
git add modules/world/outbound.go modules/world/outbound_test.go modules/world/client.go modules/world/tick.go modules/world/tick_recovery.go modules/world/handlers_game.go modules/world/player.go modules/world/server_accept.go modules/world/config.go examples/full-config-reference.yaml
git commit --no-gpg-sign -m "fix(world): move socket writes off the tick goroutine behind a bounded outbound queue (SEC1 M-2, DEVIATION SEC1-D3)"
```

---

### Task 5: M-8a — CSRF on anonymous forms + session rotation on login

**Files:**
- Modify: `modules/account/middleware.go` (`requireCSRF`, `public`, new cookie helpers)
- Modify: `modules/account/portal.go:74-80` (`render` seeds the anonymous CSRF cookie)
- Modify: `modules/account/handlers_auth.go:107-150` (`handleLogin` rotation)
- Modify: `modules/account/handlers_auth_test.go:26-33` (`postForm` auto-attaches csrf)
- Test: `modules/account/csrf_public_test.go` (create)

**Interfaces:**
- Produces: `const csrfCookieName = "goscape_csrf"`; `func (p *portal) ensureCSRFCookie(w http.ResponseWriter, r *http.Request) string` returns the anonymous token (existing cookie value or a fresh one it just set).
- `requireCSRF(w, r)` semantics: GET/HEAD pass. Otherwise, if a session cookie exists → compare against `csrfToken(session)` (unchanged). Else → compare against the `goscape_csrf` cookie value. Missing both → 403.
- `public()` now enforces `requireCSRF` on non-GET/HEAD. (`authed`/`admin` keep their own call; double-checking is harmless.)

- [ ] **Step 1: Update the test helper first so existing tests keep passing**

Replace `postForm` in `handlers_auth_test.go`:

```go
// postForm posts a form, attaching the right CSRF token automatically
// (session-derived when the jar holds a session cookie, otherwise the
// anonymous double-submit cookie, seeding it with a GET when absent).
// Tests that deliberately omit/forge the token build their own request.
func postForm(t *testing.T, c *http.Client, u string, form url.Values) *http.Response {
	t.Helper()
	if form.Get("csrf") == "" {
		form = cloneValues(form)
		form.Set("csrf", csrfFor(t, c, u))
	}
	resp, err := c.PostForm(u, form)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func cloneValues(v url.Values) url.Values {
	out := make(url.Values, len(v))
	for k, vs := range v {
		out[k] = append([]string(nil), vs...)
	}
	return out
}

func csrfFor(t *testing.T, c *http.Client, u string) string {
	t.Helper()
	parsed, err := url.Parse(u)
	if err != nil {
		t.Fatal(err)
	}
	lookup := func() (string, bool) {
		var anon string
		for _, ck := range c.Jar.Cookies(parsed) {
			switch ck.Name {
			case sessionCookieName:
				return csrfToken(ck.Value), true
			case csrfCookieName:
				anon = ck.Value
			}
		}
		return anon, anon != ""
	}
	if tok, ok := lookup(); ok {
		return tok
	}
	resp, err := c.Get(u) // any rendered page seeds the anonymous cookie
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	tok, ok := lookup()
	if !ok {
		t.Fatalf("no csrf cookie after GET %s", u)
	}
	return tok
}
```

Some tests build clients without a jar or post with a hand-built request; run `grep -n 'PostForm\|http.NewRequest(http.MethodPost' modules/account/*_test.go` and route each through `postForm`/`csrfFor` unless the test is *about* CSRF rejection.

- [ ] **Step 2: Write the failing tests**

```go
package account

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// SEC1 M-8: anonymous POSTs (/login, /register, /forgot-password,
// /reset-password) require the double-submit CSRF token; a cross-site
// form that cannot read the cookie is rejected with 403.
func TestPublicPOST_RejectsMissingOrWrongCSRF(t *testing.T) {
	p, _ := newTestPortal(t)
	srv, client := portalClient(t, p)

	for _, path := range []string{"/login", "/register", "/forgot-password", "/reset-password"} {
		resp, err := client.PostForm(srv.URL+path, url.Values{"email": {"x@example.com"}})
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("%s without csrf: got %d, want 403", path, resp.StatusCode)
		}
		resp, err = client.PostForm(srv.URL+path, url.Values{"email": {"x@example.com"}, "csrf": {"forged"}})
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("%s forged csrf: got %d, want 403", path, resp.StatusCode)
		}
	}
}

// The rendered login form carries the anonymous token and the cookie
// that backs it, and a POST using both is accepted.
func TestPublicPOST_AcceptsDoubleSubmitToken(t *testing.T) {
	p, _ := newTestPortal(t)
	srv, client := portalClient(t, p)
	resp, _ := client.Get(srv.URL + "/login")
	body := readBody(t, resp)
	if !strings.Contains(body, `name="csrf"`) {
		t.Fatal("login form must embed csrf")
	}
	tok := csrfFor(t, client, srv.URL+"/login")
	resp = postForm(t, client, srv.URL+"/login", url.Values{
		"email": {"nobody@example.com"}, "password": {"wrong"}, "csrf": {tok},
	})
	if resp.StatusCode != http.StatusOK { // wrong password renders the form again; not 403
		t.Fatalf("valid csrf: got %d", resp.StatusCode)
	}
}

// Logging in invalidates whatever session the browser already had
// (fixation / cross-site login defence) and issues a fresh one.
func TestLogin_RotatesExistingSession(t *testing.T) {
	p, s := newTestPortal(t)
	srv, client := portalClient(t, p)
	phc, _ := HashPassword("hunter22!", testArgon2())
	_, _ = s.CreateAccount(t.Context(), "a@example.com", phc)

	login := func() string {
		t.Helper()
		resp := postForm(t, client, srv.URL+"/login", url.Values{"email": {"a@example.com"}, "password": {"hunter22!"}})
		if resp.StatusCode != http.StatusFound {
			t.Fatalf("login: %d", resp.StatusCode)
		}
		u, _ := url.Parse(srv.URL)
		for _, c := range client.Jar.Cookies(u) {
			if c.Name == sessionCookieName {
				return c.Value
			}
		}
		t.Fatal("no session cookie")
		return ""
	}
	first := login()
	second := login()
	if first == second {
		t.Fatal("second login must issue a new session token")
	}
	if _, err := s.SessionAccount(t.Context(), HashToken(first), p.cfg.Session); err == nil {
		t.Fatal("first session must be deleted on re-login")
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/account/ -run 'TestPublicPOST|TestLogin_RotatesExistingSession' -v`
Expected: compile error (`csrfCookieName` undefined) — then after adding only the constant, `got 200, want 403` failures.

- [ ] **Step 4: Implement in `middleware.go`**

```go
const csrfCookieName = "goscape_csrf"

// ensureCSRFCookie returns the anonymous double-submit token for this
// browser, minting and setting the cookie when absent. Used by render()
// for anonymous pages so every public form carries a token the server
// can check against a cookie a cross-site attacker cannot read.
func (p *portal) ensureCSRFCookie(w http.ResponseWriter, r *http.Request) string {
	if c, err := r.Cookie(csrfCookieName); err == nil && c.Value != "" {
		return c.Value
	}
	raw, err := NewRawToken()
	if err != nil {
		p.log.Error("csrf token mint failed", slog.Any("err", err))
		return ""
	}
	http.SetCookie(w, &http.Cookie{
		Name: csrfCookieName, Value: raw, Path: "/", MaxAge: 24 * 60 * 60,
		HttpOnly: true, Secure: strings.HasPrefix(p.cfg.PublicURL, "https://"), SameSite: http.SameSiteLaxMode,
	})
	return raw
}
```

Replace `requireCSRF`:

```go
// requireCSRF checks the CSRF token on state-changing methods, writing a
// 403 and returning false if it is missing or wrong. GET/HEAD are exempt.
// Logged-in browsers prove the token is derived from their HttpOnly
// session cookie; anonymous browsers (SEC1 M-8: /login, /register,
// /forgot-password, /reset-password) prove it matches the HttpOnly
// double-submit cookie render() seeded on the form page.
func requireCSRF(w http.ResponseWriter, r *http.Request) bool {
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		return true
	}
	var want string
	if c, err := r.Cookie(sessionCookieName); err == nil {
		want = csrfToken(c.Value)
	} else if c, err := r.Cookie(csrfCookieName); err == nil {
		want = c.Value
	}
	if want == "" || subtle.ConstantTimeCompare([]byte(r.FormValue("csrf")), []byte(want)) != 1 {
		http.Error(w, "invalid csrf token", http.StatusForbidden)
		return false
	}
	return true
}
```

In `public()`, after the session-loading block and before `h(w, r)`:

```go
		if !requireCSRF(w, r) {
			return
		}
```

In `portal.go` `render`, replace the CSRF lines:

```go
	pd := pageData{Account: ctxAccount(r), Msg: r.URL.Query().Get("msg"), Data: data}
	if c, err := r.Cookie(sessionCookieName); err == nil && pd.Account != nil {
		pd.CSRF = csrfToken(c.Value)
	} else {
		pd.CSRF = p.ensureCSRFCookie(w, r)
	}
```

(`ensureCSRFCookie` must run before the buffer is written to `w`; it does — headers are set first.)

Templates: add `<input type="hidden" name="csrf" value="{{.CSRF}}">` as the first child of the `<form>` in `templates/pages/login.html`, `register.html`, `forgot.html`, and `reset.html` (open `reset.html` to find its form; it posts to `/reset-password`).

`handleLogin`, right before `raw, err := NewRawToken()`:

```go
	// SEC1 M-8: rotate — any session the browser already holds is
	// dropped so a login cannot be fixated onto a pre-set cookie.
	if c, err := r.Cookie(sessionCookieName); err == nil {
		_ = p.store.DeleteSession(r.Context(), HashToken(c.Value))
	}
```

- [ ] **Step 5: Run the package**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/account/ -v 2>&1 | tail -40` → all PASS. Fix any test that posts without going through `postForm` (Step 1 grep).

- [ ] **Step 6: Commit**

```bash
git add modules/account/middleware.go modules/account/portal.go modules/account/handlers_auth.go modules/account/handlers_auth_test.go modules/account/csrf_public_test.go modules/account/templates/pages/login.html modules/account/templates/pages/register.html modules/account/templates/pages/forgot.html modules/account/templates/pages/reset.html
git commit --no-gpg-sign -m "fix(account): CSRF on anonymous portal forms + session rotation on login (SEC1 M-8)"
```

---

### Task 6: M-8b — security headers, server timeouts, body cap

**Files:**
- Modify: `modules/account/portal.go` (`routes()` returns `http.Handler` wrapped in `secureHeaders`)
- Modify: `modules/account/account.go:89` (`http.Server` timeouts)
- Test: `modules/account/headers_test.go` (create)

**Interfaces:**
- Produces: `func (p *portal) secureHeaders(next http.Handler) http.Handler`; `func (p *portal) routes() http.Handler` (was `*http.ServeMux` — `httptest.NewServer` and `http.Server{Handler:}` accept `http.Handler`, so callers compile unchanged).

- [ ] **Step 1: Write the failing test**

```go
package account

import (
	"net/http"
	"strings"
	"testing"
)

// SEC1 M-8: every response carries the defensive headers; HSTS only when
// the public URL is https (cookies are Secure under the same rule).
func TestSecureHeaders(t *testing.T) {
	p, _ := newTestPortal(t) // PublicURL is http://portal.test
	srv, client := portalClient(t, p)
	resp, err := client.Get(srv.URL + "/login")
	if err != nil {
		t.Fatal(err)
	}
	h := resp.Header
	want := map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"Referrer-Policy":        "no-referrer",
	}
	for k, v := range want {
		if got := h.Get(k); got != v {
			t.Errorf("%s: got %q, want %q", k, got, v)
		}
	}
	csp := h.Get("Content-Security-Policy")
	for _, directive := range []string{"default-src 'self'", "form-action 'self'", "frame-ancestors 'none'", "base-uri 'self'"} {
		if !strings.Contains(csp, directive) {
			t.Errorf("CSP missing %q: %q", directive, csp)
		}
	}
	if h.Get("Strict-Transport-Security") != "" {
		t.Error("HSTS must not be sent for an http public_url")
	}

	p.cfg.PublicURL = "https://portal.test"
	resp, _ = client.Get(srv.URL + "/login")
	if !strings.HasPrefix(resp.Header.Get("Strict-Transport-Security"), "max-age=") {
		t.Error("HSTS must be sent for an https public_url")
	}
}

// Oversized form bodies are refused instead of being buffered.
func TestBodyLimit(t *testing.T) {
	p, _ := newTestPortal(t)
	srv, client := portalClient(t, p)
	big := strings.Repeat("a", 70<<10)
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/login", strings.NewReader("email="+big))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusRequestEntityTooLarge && resp.StatusCode != http.StatusForbidden && resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("oversized body: got %d", resp.StatusCode)
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/account/ -run 'TestSecureHeaders|TestBodyLimit' -v` → FAIL (`X-Content-Type-Options: got ""`).

- [ ] **Step 3: Implement**

First check the templates for inline styles/scripts: `grep -rn 'style=\|<script' modules/account/templates` returned nothing at plan time, so the strict CSP below is safe. In `portal.go`:

```go
// maxFormBody bounds request bodies: the largest legitimate form is a
// few hundred bytes; 64 KiB leaves room without letting a client park a
// multi-megabyte body in memory (SEC1 M-8).
const maxFormBody = 64 << 10

// secureHeaders adds the defensive response headers to every reply and
// caps the request body. HSTS is sent only when public_url is https —
// the same rule that makes the cookies Secure — so an http-only dev
// deployment does not pin browsers to TLS it cannot serve.
func (p *portal) secureHeaders(next http.Handler) http.Handler {
	https := strings.HasPrefix(p.cfg.PublicURL, "https://")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", "default-src 'self'; form-action 'self'; frame-ancestors 'none'; base-uri 'self'; object-src 'none'")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		if https {
			h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxFormBody)
		next.ServeHTTP(w, r)
	})
}
```

`TestSecureHeaders` mutates `p.cfg.PublicURL` after construction, so read `p.cfg.PublicURL` **inside** the handler func rather than capturing `https` once — change accordingly (`https := strings.HasPrefix(p.cfg.PublicURL, "https://")` as the first line inside the closure).

Change `routes()` signature to `func (p *portal) routes() http.Handler` and its last line to `return p.secureHeaders(mux)`. `strings` is already imported in `portal.go`? Check; add if needed.

In `account.go:89`:

```go
	a.httpSrv = &http.Server{
		Handler:           p.routes(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    64 << 10,
	}
```

- [ ] **Step 4: Run the package**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/account/` → PASS. (`requireCSRF` on an over-limit body: `r.FormValue` triggers `ParseForm`, which fails with the MaxBytes error and yields an empty token → 403. Either 403 or 413 satisfies `TestBodyLimit`.)

- [ ] **Step 5: Commit**

```bash
git add modules/account/portal.go modules/account/account.go modules/account/headers_test.go
git commit --no-gpg-sign -m "fix(account): security headers, server timeouts and body cap on the portal (SEC1 M-8)"
```

---

### Task 7: M-8c — two-step email verification (no state change on GET)

**Files:**
- Modify: `modules/account/handlers_auth.go:160-182` (`handleVerifyEmail` split)
- Modify: `modules/account/portal.go` routes (add `POST /verify-email`)
- Create: `modules/account/templates/pages/verify.html`
- Modify: `modules/account/handlers_verify_test.go:23-55`

**Interfaces:**
- `GET /verify-email?token=` → renders `verify.html` with a confirm form (hidden `token`, hidden `csrf`). No store writes.
- `POST /verify-email` (form `token`) → consumes the token, marks verified, renders the success message. Wrong/used token → "invalid or expired" message page.

- [ ] **Step 1: Update the existing flow test (it is the spec)**

In `TestVerifyEmailFlow`, replace the "Following the link verifies the account." block:

```go
	// Following the link only shows a confirm button (SEC1 M-8: no state
	// change on GET — mail scanners prefetch links).
	resp, err := client.Get(local)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("verify GET: %v %d", err, resp.StatusCode)
	}
	if !strings.Contains(readBody(t, resp), `name="token"`) {
		t.Fatal("verify page must offer a confirm form")
	}
	if acct, _ := s.AccountByEmail(t.Context(), "v@example.com"); acct.EmailVerified {
		t.Fatal("GET must not verify")
	}

	// Confirming verifies the account.
	tokenVal := strings.TrimPrefix(local, srv.URL+"/verify-email?token=")
	resp = postForm(t, client, srv.URL+"/verify-email", url.Values{"token": {tokenVal}})
	if resp.StatusCode != http.StatusOK || !strings.Contains(readBody(t, resp), "verified") {
		t.Fatal("verify POST must confirm")
	}
	acct, _ := s.AccountByEmail(t.Context(), "v@example.com")
	if !acct.EmailVerified {
		t.Fatal("account must be verified")
	}

	// Token is single-use.
	resp = postForm(t, client, srv.URL+"/verify-email", url.Values{"token": {tokenVal}})
	if !strings.Contains(readBody(t, resp), "invalid or expired") {
		t.Fatal("second use must fail")
	}
```

Apply the same GET-then-POST change to the resend branch later in that test (it currently `client2.Get(local2)` then checks verified).

- [ ] **Step 2: Run to verify failure**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/account/ -run TestVerifyEmailFlow -v` → FAIL (`verify page must offer a confirm form`).

- [ ] **Step 3: Implement**

`templates/pages/verify.html`:

```html
{{define "title"}}Verify email — goscape{{end}}
{{define "content"}}
<h1>Verify your email</h1>
<p>Click the button below to confirm this email address for your goscape account.</p>
<form class="stack" method="post" action="/verify-email">
  <input type="hidden" name="csrf" value="{{.CSRF}}">
  <input type="hidden" name="token" value="{{.Data}}">
  <button type="submit">Confirm my email</button>
</form>
{{end}}
```

Check how page templates are discovered (`grep -n 'pages' modules/account/portal.go | head`, likely `embed` + glob) so the new file is picked up automatically.

`handlers_auth.go`:

```go
// handleVerifyEmailForm shows the confirm button. It deliberately
// changes nothing (SEC1 M-8): mail scanners and link previews GET the
// link before the user does, which used to burn the single-use token.
func (p *portal) handleVerifyEmailForm(w http.ResponseWriter, r *http.Request) {
	raw := r.URL.Query().Get("token")
	if raw == "" {
		p.render(w, r, "message.html", "That verification link is invalid or expired.")
		return
	}
	p.render(w, r, "verify.html", raw)
}

func (p *portal) handleVerifyEmail(w http.ResponseWriter, r *http.Request) {
	raw := r.FormValue("token")
	if raw == "" {
		p.render(w, r, "message.html", "That verification link is invalid or expired.")
		return
	}
	// ... existing body from ConsumeToken onward, unchanged ...
}
```

Routes: `mux.HandleFunc("GET /verify-email", p.public(p.handleVerifyEmailForm))` and add `mux.HandleFunc("POST /verify-email", p.public(p.handleVerifyEmail))`.

- [ ] **Step 4: Run the package**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/account/` → PASS.

- [ ] **Step 5: Commit**

```bash
git add modules/account/handlers_auth.go modules/account/portal.go modules/account/templates/pages/verify.html modules/account/handlers_verify_test.go
git commit --no-gpg-sign -m "fix(account): make email verification a confirmed POST, not a state-changing GET (SEC1 M-8)"
```

---

### Task 8: M-12a — gate `/debug/status` behind a config flag (default off)

**Files:**
- Modify: `modules/ondemand/config.go` (field + flag)
- Modify: `modules/ondemand/health.go:78-100` (`RegisterHealthRoutes` signature)
- Modify: `cmd/goscape/app/modules.go:121`
- Modify: `modules/ondemand/health_test.go` (all `RegisterHealthRoutes` calls + new test)
- Modify: `examples/full-config-reference.yaml` (ondemand block)
- Modify: `docs/PORTING.md` (one line in the arch-29.6 mention is enough: note the flag)

**Interfaces:**
- `Config.DebugStatusEnabled bool \`yaml:"debug_status_enabled"\``, flag `ondemand.debug-status-enabled`, default `false`.
- `func RegisterHealthRoutes(mux *http.ServeMux, snap func() (HealthSnapshot, bool), debugStatus bool)`.

- [ ] **Step 1: Write the failing test**

Append to `health_test.go`:

```go
// SEC1 M-12: /debug/status is off unless explicitly enabled; /healthz
// is unaffected.
func TestDebugStatusDisabledByDefault(t *testing.T) {
	mux := http.NewServeMux()
	RegisterHealthRoutes(mux, func() (HealthSnapshot, bool) {
		return HealthSnapshot{LastTick: time.Now()}, true
	}, false)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest("GET", "/debug/status", nil))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("/debug/status when disabled: got %d, want 404", rr.Code)
	}
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest("GET", "/healthz", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("/healthz: got %d, want 200", rr.Code)
	}
}
```

and add `, true` as the third argument to every existing `RegisterHealthRoutes(` call in `health_test.go`.

- [ ] **Step 2: Run to verify failure**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/ondemand/ -run 'TestDebugStatus|TestHealthz' -v` → compile error (too many arguments).

- [ ] **Step 3: Implement**

`config.go`: next to `Debug bool \`yaml:"node_debug"\`` add `DebugStatusEnabled bool \`yaml:"debug_status_enabled"\`` and in `RegisterFlagsAndApplyDefaults` next to the `node-debug` flag:

```go
	f.BoolVar(&c.DebugStatusEnabled, "ondemand.debug-status-enabled", false, "Serve GET /debug/status (players online, tick age) on the public ondemand listener. Off by default: it is an unauthenticated load/presence oracle (SEC1 M-12).")
```

`health.go`: signature `RegisterHealthRoutes(mux *http.ServeMux, snap func() (HealthSnapshot, bool), debugStatus bool)`; wrap the `/debug/status` registration in `if debugStatus { ... }`; extend the doc comment: `debugStatus gates GET /debug/status (SEC1 M-12 — default off, see ondemand.debug_status_enabled).`

`modules.go:121`: pass `g.cfg.OnDemand.DebugStatusEnabled` as the third argument (confirm the config path with `grep -n 'cfg.OnDemand' cmd/goscape/app/modules.go | head -3`).

`examples/full-config-reference.yaml`, in the `ondemand:` block after `node_debug: true`:

```yaml
  # Serve GET /debug/status (players online, current tick, tick age) on the
  # public ondemand listener. It is unauthenticated, so it doubles as a load /
  # presence oracle for attackers — leave off unless you front it with auth.
  # CLI: --ondemand.debug-status-enabled (default: false)
  debug_status_enabled: false
```

- [ ] **Step 4: Run tests and the config verify gate**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/ondemand/ ./cmd/...` → PASS.
Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go run ./cmd/goscape --config.file examples/full-config-reference.yaml --config.verify=true` → exits 0 (strict YAML accepts the new key). If a config-reference test exists (`grep -rn 'full-config-reference' --include=*_test.go .`), run it.

- [ ] **Step 5: Commit**

```bash
git add modules/ondemand/config.go modules/ondemand/health.go modules/ondemand/health_test.go cmd/goscape/app/modules.go examples/full-config-reference.yaml docs/PORTING.md
git commit --no-gpg-sign -m "fix(ondemand): gate /debug/status behind ondemand.debug_status_enabled, default off (SEC1 M-12)"
```

---

### Task 9: M-12b — Helm pod hardening, SA token, expand-env, hiscore ingress, Dockerfile USER

**Files:**
- Modify: `production/helm/goscape/values.yaml`
- Modify: `production/helm/goscape/templates/_helpers.tpl` (`goscape.image`, `goscape.podTemplate`)
- Modify: `production/helm/goscape/templates/networkpolicy.yaml`
- Modify: `production/helm/goscape/README.md` (notes on the new values; find the values table)
- Modify: `cmd/goscape/Dockerfile:40` (uncomment `USER 65532:65532`; image stays `debug-nonroot` — user decision)
- Test: `production/helm/goscape/Makefile` `lint` + `helm template` assertions run by hand (no unittest plugin installed)

**Interfaces (values):**
- `image.digest: ""` — when set, image ref is `repo@digest` (tag ignored).
- `serviceAccount.automountServiceAccountToken: false` (was true).
- Per workload (`singleBinary`, `management`, `world`): `podSecurityContext` and `containerSecurityContext` defaults filled in; `resources` default requests/limits; new `livenessProbe: {enabled: true, initialDelaySeconds: 60, periodSeconds: 30, failureThreshold: 3}`.
- `hiscoreGateway.proxyNamespace: kong`, `hiscoreGateway.proxyPodSelector: {app.kubernetes.io/name: kong}` — used by the NetworkPolicy when `createGatewayConfig` is true.
- Always render `--config.expand-env=true`.

- [ ] **Step 1: Capture the baseline render to diff against**

Run from `production/helm/goscape`: `helm template goscape . -f single-binary-values.yaml > $TMPDIR/before-sb.yaml; helm template goscape . -f management-values.yaml > $TMPDIR/before-mgmt.yaml; helm template goscape . -f world-values.yaml --set goscape.loginServerAddress=mgmt:2004 --set goscape.friendsServerAddress=mgmt:2005 > $TMPDIR/before-world.yaml` and `make lint` (must already pass).

- [ ] **Step 2: values.yaml**

Apply:
- `image.digest: ""` with comment `# -- Image digest (sha256:...). When set, the image is pinned by digest and tag is ignored.`
- `serviceAccount.automountServiceAccountToken: false` with comment `# -- goscape never talks to the Kubernetes API; keep the token unmounted.`
- For each of `singleBinary`, `management`, `world` replace the four empty knobs with:

```yaml
  resources:
    requests:
      cpu: 500m
      memory: 1Gi
    limits:
      memory: 2Gi
  podSecurityContext:
    runAsNonRoot: true
    runAsUser: 65532
    runAsGroup: 65532
    fsGroup: 65532
    seccompProfile:
      type: RuntimeDefault
  containerSecurityContext:
    allowPrivilegeEscalation: false
    readOnlyRootFilesystem: true
    capabilities:
      drop: ["ALL"]
  # -- Liveness probe: tcpSocket on the primary port (world-tcp, or login-grpc
  #    in Management). /healthz is deliberately NOT used for liveness — it
  #    returns 503 during a slow cold-cache boot and must not restart the pod.
  livenessProbe:
    enabled: true
    initialDelaySeconds: 60
    periodSeconds: 30
    failureThreshold: 3
```

  (Memory limit rationale, as a comment: the world fills to its GC ceiling at ~1.1–1.3 GiB under load; 2Gi leaves headroom.)
- Under `hiscoreGateway` add:

```yaml
  # When createGatewayConfig is true and networkPolicy.enabled is true, the
  # hiscore port is reachable only from the Kong proxy pods matched below, so
  # in-cluster callers cannot bypass key-auth and rate limits.
  proxyNamespace: kong
  proxyPodSelector:
    app.kubernetes.io/name: kong
```

- [ ] **Step 3: `_helpers.tpl`**

`goscape.image`:

```
{{- define "goscape.image" -}}
{{- $repo := .Values.image.repository -}}
{{- if .Values.image.registry -}}{{- $repo = printf "%s/%s" .Values.image.registry $repo -}}{{- end -}}
{{- if .Values.image.digest -}}
{{- printf "%s@%s" $repo .Values.image.digest -}}
{{- else -}}
{{- printf "%s:%s" $repo (.Values.image.tag | default .Chart.AppVersion) -}}
{{- end -}}
{{- end -}}
```

`goscape.podTemplate`:
- `args`: make `- "--config.expand-env=true"` unconditional (delete the `{{- if $pgActive }}` guard around it; keep the guard on the env var).
- After `readinessProbe` block add:

```
      {{- if $w.livenessProbe.enabled }}
      livenessProbe:
        tcpSocket:
          {{- if eq $mode "Management" }}
          port: login-grpc
          {{- else }}
          port: world-tcp
          {{- end }}
        initialDelaySeconds: {{ $w.livenessProbe.initialDelaySeconds }}
        periodSeconds: {{ $w.livenessProbe.periodSeconds }}
        failureThreshold: {{ $w.livenessProbe.failureThreshold }}
      {{- end }}
```

- `volumeMounts`: add `- name: tmp` / `mountPath: /tmp` (readOnlyRootFilesystem needs a writable `/tmp` for `os.CreateTemp`/heap-profile debug paths); `volumes`: add `- name: tmp` / `emptyDir: {}`.
- `securityContext` blocks already render from the values via `with`; nothing else to change.

- [ ] **Step 4: networkpolicy.yaml**

Replace the hiscore rule:

```
    {{- if or (eq .Values.deploymentMode "SingleBinary") (eq .Values.deploymentMode "Management") }}
    # Hiscore API. When Kong config is rendered, only the Kong proxy pods may
    # reach the port directly (SEC1 M-12: otherwise any in-cluster client can
    # skip key-auth + rate limits). Without Kong it stays open like the other
    # client-facing ports.
    - {{- if .Values.hiscoreGateway.createGatewayConfig }}
      from:
        - namespaceSelector:
            matchLabels:
              kubernetes.io/metadata.name: {{ .Values.hiscoreGateway.proxyNamespace }}
          podSelector:
            matchLabels:
              {{- toYaml .Values.hiscoreGateway.proxyPodSelector | nindent 14 }}
      {{- end }}
      ports:
        - port: {{ .Values.goscape.ports.hiscoreHTTP }}
          protocol: TCP
    {{- end }}
```

(Watch the YAML: the `- ` list item must keep `from:` and `ports:` at the same indentation. Verify with `helm template ... --set networkPolicy.enabled=true --set hiscoreGateway.createGatewayConfig=true --set hiscoreGateway.host=h.example` and eyeball the NetworkPolicy.)

- [ ] **Step 5: Dockerfile**

`cmd/goscape/Dockerfile:40`: change `#USER 65532:65532` to `USER 65532:65532`. Leave the `FROM gcr.io/distroless/static-debian13:debug-nonroot` line as is (user decision: keep the debug image).

- [ ] **Step 6: README**

In `production/helm/goscape/README.md`, add a short "Security defaults" section: pods run as uid 65532 with a read-only root filesystem and all capabilities dropped; SA token not mounted; memory limit 2Gi; liveness via tcpSocket; `--config.expand-env=true` is always on, so `${VAR}` in `extraConfig` resolves from `extraEnv` (use `secretKeyRef` for secrets such as `account.admin_token`, SMTP and Discord credentials) and a literal `$` must be escaped per `drone/envsubst` (`$$`); `image.digest` pins by digest; `hiscoreGateway.proxyNamespace/proxyPodSelector` scope the hiscore NetworkPolicy rule.

- [ ] **Step 7: Verify**

Run from `production/helm/goscape`:
- `make lint` → 3× "1 chart(s) linted, 0 chart(s) failed".
- Render all three modes as in Step 1 to `$TMPDIR/after-*.yaml`; `diff $TMPDIR/before-sb.yaml $TMPDIR/after-sb.yaml` must show ONLY: `automountServiceAccountToken: false`, the `securityContext` blocks, `resources`, `livenessProbe`, the `tmp` mount/volume, and `--config.expand-env=true`. Anything else in the diff is a regression.
- `helm template goscape . -f single-binary-values.yaml --set networkPolicy.enabled=true --set hiscoreGateway.createGatewayConfig=true --set hiscoreGateway.host=h.example | sed -n '/kind: NetworkPolicy/,/^---/p'` → the hiscore rule carries the `from:` selector; without `createGatewayConfig` it does not.
- `helm template goscape . -f single-binary-values.yaml --set image.digest=sha256:0000000000000000000000000000000000000000000000000000000000000000 | grep 'image:'` → `docker.io/goscape/goscape@sha256:...`.
- `docker build -f cmd/goscape/Dockerfile` is NOT required (no network); `grep -n '^USER' cmd/goscape/Dockerfile` → the line is active.

- [ ] **Step 8: Commit**

```bash
git add production/helm/goscape/values.yaml production/helm/goscape/templates/_helpers.tpl production/helm/goscape/templates/networkpolicy.yaml production/helm/goscape/README.md cmd/goscape/Dockerfile
git commit --no-gpg-sign -m "chore(helm): harden pod defaults, always expand env, scope hiscore ingress to Kong, explicit USER (SEC1 M-12)"
```

---

### Task 10: Close-out — PORTING.md entry and full gate

**Files:**
- Modify: `docs/PORTING.md` (append an entry in the same style as the arch-31 entry: what changed, deviation IDs, backport note)

- [ ] **Step 1: Write the entry**

Append a `### SEC1 — security hardening batch 1 (2026-08-22)` entry listing M-1/M-2/M-7/M-8/M-12 with one line each, the three deviation IDs with a pointer to `docs/superpowers/specs/2026-08-22-sec1-hardening-design.md`, and a `[BACKPORT-FIDELITY]` note: M-1/M-2 are goscape-own process hardening (off the backport list per `no_forward_port_deviations`), M-7/M-8/M-12 touch goscape-only surfaces and may be backported to any rev branch that carries the account portal / Helm chart.

- [ ] **Step 2: Full gate**

Run:
- `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache gofmt -l ./modules ./pkg ./cmd` → empty.
- `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./modules/... ./pkg/io/... ./cmd/...` → clean.
- `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...` → PASS (note `TestNAI128_RatLootCascade` is a known pre-existing failure per memory; report it if it fails, do not "fix" it).
- `CGO_ENABLED=1 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race -short ./modules/world/ ./modules/account/ ./modules/ondemand/` → PASS.
- `cd production/helm/goscape && make lint` → PASS.

- [ ] **Step 3: Commit**

```bash
git add docs/PORTING.md
git commit --no-gpg-sign -m "docs(porting): record SEC1 hardening batch and deviations SEC1-D1..D3"
```

---

## Self-review

- **Spec coverage:** M-1 → Tasks 2, 3 (D1, D2). M-2 → Task 4 (D3). M-7 → Task 1. M-8 → Tasks 5, 6, 7 (CSRF, rotation, headers, timeouts, body cap, verify POST). M-12 → Tasks 8, 9 (debug/status flag, securityContext, resources, liveness, SA token, digest, expand-env, hiscore NP scope, USER; image stays debug-nonroot). Docs/deviation registry → Task 10 + spec doc.
- **Placeholder scan:** none of TBD/TODO/"similar to". Every code step has code. Two places instruct the implementer to *check* a name before use (`newTestClient` return order, `logTick` nil-ness, script-provider stub) — with the exact grep to run.
- **Type consistency:** `RegisterHealthRoutes(mux, snap, debugStatus bool)` used identically in Task 8 test, impl and `modules.go`. `closeConn()` name consistent across Task 4. `csrfCookieName`/`csrfFor`/`postForm` consistent across Tasks 5–7. `routes() http.Handler` consistent with `httptest.NewServer`/`http.Server` callers.
