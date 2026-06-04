# NAI-188 — `::speed` Cheat Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port the TS `::speed <ms>` dev-block cheat (`ClientCheatHandler.ts:154-167`) into goscape, promoting `tickRate` from a package-level `const` to a mutable `Server.tickRate` field so the tick loop honours runtime cadence changes.

**Architecture:** Promote the `tickRate` constant in `modules/world/tick.go` to a `Server` field (`s.tickRate time.Duration`) seeded from `defaultTickRate`. Rewrite `runTickLoopWithRate` to re-read `s.tickRate` on every iteration so cheat-induced mutations take effect on the next sleep computation. Add `case "speed":` to the existing dev-block switch in `modules/world/handlers_game.go`. No locking required: cheats dispatch on the tick goroutine (`processClientsIn → processIn`), so the writer/reader are the same goroutine.

**Tech Stack:** Go 1.26+

**Spec:** `docs/superpowers/specs/2026-05-13-nai-188-speed-cheat-design.md`
**HEAD at plan-write:** `42e5b7d`
**TS source:** `LostCityRS/Engine-TS/src/network/game/client/handler/ClientCheatHandler.ts:154-167`

---

## File Map

| File | Role | Action |
|---|---|---|
| `modules/world/tick.go` | tick-loop entry + per-tick dispatch | Modify: rename `const tickRate` → `defaultTickRate`; rewrite `runTickLoopWithRate` to re-read `s.tickRate`. |
| `modules/world/server.go` | `Server` struct + constructor | Modify: add `tickRate time.Duration` field; init in `New(...)`. |
| `modules/world/server_test.go` | shared test factory + tick-loop test | Modify: `newTestServer` inits `tickRate`; add `TestTickLoopHonoursFieldRate`. |
| `modules/world/handlers_game.go` | cheat dispatch + carryforward comment | Modify: add `case "speed":` to dev-block switch; rewrite `DEVIATION-NAI-187-D1-CARRYFORWARD` comment on close. |
| `modules/world/handlers_game_test.go` | cheat-handler tests | Modify: add 6 `TestHandleClientCheat_Speed_*` tests using the existing `teleTestPlayer` / `dispatchTeleCheat` / `drainAfterTele` helpers. |

No new files.

---

## Plan-author pre-flight (recorded at plan-write, HEAD `42e5b7d`)

Re-verified the spec's pre-flight claims against current `main`:

- `tickRate` in goscape:
  - `modules/world/tick.go:15` — `const tickRate = 600 * time.Millisecond` ✓
  - `modules/world/tick.go:23` — `s.runTickLoopWithRate(tickRate)` ✓
  - `modules/world/handlers_game.go:379` — carryforward doc only ✓
- `runTickLoopWithRate` callers:
  - `modules/world/tick.go:22-24` — production `runTickLoop` ✓
  - `modules/world/server_test.go:578` — `TestTickLoopIncrementsCurrentTick` passes `3 * time.Millisecond` ✓
- `&Server{}` direct literals: 17 sites total (`rg "&Server\{" modules/world/`). All of them are in test files; **none** call `runTickLoopWithRate` or `runTickLoop`. The single tick-loop test is `TestTickLoopIncrementsCurrentTick` (server_test.go:572), which uses `newTestServer`. Per memory `plan_helper_coverage`: updating `newTestServer` alone is sufficient — no `&Server{}` literals need to be touched for tickRate.
- `parseIntOr` exists at `handlers_game.go:1011` with TS-faithful tryParseInt semantics ✓
- Dev-block guard exists at `handlers_game.go:399` (`!s.cfg.NodeProduction && p.staffModLevel >= 4`) ✓
- Cheat-test infra: `teleTestPlayer` (handlers_game_test.go:366), `dispatchTeleCheat` (line 394), `drainAfterTele` (line 406) — all three reuseable as-is ✓
- The `Server` struct currently uses a flat-field literal style in `New(...)` at `server.go:188-204`; field insertion follows the existing alphabetical-ish grouping (matches `pmCount`, `shutdownTick`, etc.)

---

## Task 1: Promote `tickRate` from const to `Server` field

**Files:**
- Modify: `modules/world/tick.go:15`
- Modify: `modules/world/server.go` — add field to `Server` struct; init in `New(...)`
- Modify: `modules/world/server_test.go:311-329` — init in `newTestServer`

This task is **compile-only**. No runtime behavior changes yet (the loop still uses its parameter; `s.tickRate` is set but unread). Existing tests must continue to pass green.

- [ ] **Step 1: Find the `Server` struct field block to insert into**

Run: `grep -n "shutdownTick\s\+int\|gracefulExit\s\+chan" modules/world/server.go | head -5`

Open `modules/world/server.go` and locate the `Server` struct declaration. Find a natural insertion point near the tick-loop-related fields (`shutdownTick`, `currentTick`, `gracefulExit`). Add the new field with a doc-comment.

- [ ] **Step 2: Add the `tickRate` field to `Server`**

Edit `modules/world/server.go` — inside the `Server` struct, near other tick-loop state. Add this line (preserving the file's existing formatting):

```go
// tickRate is the per-tick sleep interval. Initialised to
// defaultTickRate by New(...); mutated at runtime by the
// ::speed dev-block cheat (NAI-188; mirrors TS World.tickRate).
// Read/written exclusively on the tick goroutine.
tickRate time.Duration
```

`time` is already imported in `server.go` (line 16) — no import change needed.

- [ ] **Step 3: Rename `const tickRate` → `const defaultTickRate` in `tick.go`**

Edit `modules/world/tick.go`. Replace lines 15-23:

```go
// defaultTickRate is the canonical tick interval. Mirrors TS
// World.TICKRATE (Engine-TS World.ts:120) = 600ms. The ::speed
// dev-block cheat (NAI-188) writes Server.tickRate to a different
// value at runtime.
const defaultTickRate = 600 * time.Millisecond

const (
	timeoutNoResponse   = 100 // ticks = 60s at 600ms
	timeoutNoConnection = 50  // ticks = 30s at 600ms
)

func (s *Server) runTickLoop() {
	s.runTickLoopWithRate(s.tickRate)
}
```

Note: `runTickLoop` now reads `s.tickRate` (which was just set by `New(...)` in step 4) instead of the package-level const.

- [ ] **Step 4: Initialise `tickRate` in production `New(...)` constructor**

Edit `modules/world/server.go:188-204`. Inside the `s := &Server{ ... }` literal, add `tickRate: defaultTickRate,` near the other tick-loop state. The existing block:

```go
s := &Server{
	cfg:         cfg,
	handler:     handler,
	tcpListener: tcpListener,
	loginClient: loginClient,
	quit:        make(chan interface{}),

	log:           logger,
	invs:          make(map[int]*inventory.Inventory),
	zoneMap:       zone.NewZoneMap(),
	zonesTracking: map[*zone.Zone]struct{}{},
	locObjTracker: newLocObjTracker(),
	rsbuf:         rsbuf.New(),
	pmCount:       1,
	shutdownTick:  -1,
	gracefulExit:  make(chan struct{}),
}
```

becomes:

```go
s := &Server{
	cfg:         cfg,
	handler:     handler,
	tcpListener: tcpListener,
	loginClient: loginClient,
	quit:        make(chan interface{}),

	log:           logger,
	invs:          make(map[int]*inventory.Inventory),
	zoneMap:       zone.NewZoneMap(),
	zonesTracking: map[*zone.Zone]struct{}{},
	locObjTracker: newLocObjTracker(),
	rsbuf:         rsbuf.New(),
	pmCount:       1,
	shutdownTick:  -1,
	tickRate:      defaultTickRate,
	gracefulExit:  make(chan struct{}),
}
```

- [ ] **Step 5: Initialise `tickRate` in `newTestServer`**

Edit `modules/world/server_test.go:311-329`. Inside the `s := &Server{ ... }` literal, add `tickRate: defaultTickRate,`:

```go
s := &Server{
	quit:           make(chan interface{}),
	log:            discardLogger(),
	scriptProvider: defaultTestProvider(),
	zoneMap:        zone.NewZoneMap(),
	locObjTracker:  newLocObjTracker(),
	rsbuf:          rsbuf.New(),
	pmCount:        1,
	shutdownTick:   -1,
	tickRate:       defaultTickRate,
	gracefulExit:   make(chan struct{}),
}
```

- [ ] **Step 6: Verify build is green**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`
Expected: builds successfully, no compile errors.

- [ ] **Step 7: Verify existing tick-loop test still passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race -run TestTickLoopIncrementsCurrentTick ./modules/world/...`
Expected: PASS (this test still uses the parameter-only loop shape — no behavior has changed yet).

- [ ] **Step 8: Verify the rest of the test suite is unaffected**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...`
Expected: All tests pass. No new failures.

- [ ] **Step 9: Commit**

```bash
git add modules/world/tick.go modules/world/server.go modules/world/server_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
refactor(world): NAI-188 T1 — promote tickRate const to Server field

Renames the package-level `const tickRate = 600ms` to `defaultTickRate`
and adds `Server.tickRate time.Duration` initialised by New(...) /
newTestServer. No runtime behavior change yet: runTickLoopWithRate
still uses its parameter as a captured local. Sets up T2 to rewrite
the loop to re-read s.tickRate so the ::speed cheat (T3) can mutate it.
EOF
)"
```

---

## Task 2: Rewrite tick loop to re-read `s.tickRate` each iteration

**Files:**
- Test: `modules/world/server_test.go` — add `TestTickLoopHonoursFieldRate` (RED, then GREEN)
- Modify: `modules/world/tick.go:26-94` — `runTickLoopWithRate` body

- [ ] **Step 1: Write the failing integration test**

Open `modules/world/server_test.go`. Find `TestTickLoopIncrementsCurrentTick` (line ~572) and append the new test immediately after it:

```go
// TestTickLoopHonoursFieldRate pins NAI-188: mid-loop mutations to
// s.tickRate take effect on the next iteration. Starts at a 3ms
// cadence, runs ~30ms (expects ~10 ticks), mutates s.tickRate to 30ms,
// runs ~60ms (expects ~2 ticks). The post-mutation second-window tick
// delta must be strictly less than the pre-mutation first-window delta.
// Run with -race to validate the single-goroutine invariant documented
// in spec §6.
func TestTickLoopHonoursFieldRate(t *testing.T) {
	s := newTestServer(t)

	done := make(chan struct{})
	go func() {
		defer close(done)
		s.runTickLoopWithRate(3 * time.Millisecond)
	}()

	time.Sleep(30 * time.Millisecond)
	firstWindow := s.currentTick

	// Mutate the rate on this goroutine. Per spec §6 this is racy in a
	// strict sense but is the same single-writer/single-reader pattern
	// the ::speed cheat uses; the test is here precisely to catch a
	// future refactor that changes that invariant.
	s.tickRate = 30 * time.Millisecond

	time.Sleep(60 * time.Millisecond)
	secondWindow := s.currentTick - firstWindow

	close(s.quit)
	<-done

	if firstWindow < 5 {
		t.Errorf("first window (3ms rate, 30ms): currentTick = %d, want >= 5", firstWindow)
	}
	// Pre-mutation: ~10 ticks in 30ms. Post-mutation: ~2 ticks in 60ms.
	// Assert second-window delta is strictly smaller than first-window
	// count (loose comparison tolerates timer jitter).
	if secondWindow >= firstWindow {
		t.Errorf("rate mutation did not slow the loop: first window = %d ticks (3ms rate), second window = %d ticks (30ms rate after mutation). Loop is still reading the captured parameter, not s.tickRate.",
			firstWindow, secondWindow)
	}
}
```

- [ ] **Step 2: Run the failing test to confirm it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race -run TestTickLoopHonoursFieldRate ./modules/world/...`
Expected: FAIL. The loop is still using the captured `rate` parameter, so mutating `s.tickRate` has no effect. The failure message will show `firstWindow ≈ 10, secondWindow ≈ 20` (both running at 3ms).

If the test does not fail, STOP — the loop is reading `s.tickRate` already; verify the failing run is targeting the un-rewritten loop.

- [ ] **Step 3: Rewrite `runTickLoopWithRate` to re-read `s.tickRate` per iteration**

Edit `modules/world/tick.go:26-94`. The current shape:

```go
func (s *Server) runTickLoopWithRate(rate time.Duration) {
	nextTick := time.Now()
	for {
		// ... shutdown / processX dispatch (unchanged) ...

		nextTick = nextTick.Add(rate)
		delay := rate - time.Since(start) - drift
		if delay < 0 {
			delay = 0
		}

		select {
		case <-s.quit:
			return
		case <-time.After(delay):
		}
	}
}
```

Replace the function body, keeping all shutdown / processX dispatch identical. Two changes:

1. Seed `s.tickRate` from the `rate` parameter at function entry so test-injected rates remain observable. Existing test `TestTickLoopIncrementsCurrentTick` relies on the parameter form.
2. Inside the loop body, re-read `s.tickRate` into a local `currentRate` (NOT named `rate` — that would shadow the parameter; per memory `plan_var_name_collision`).

Final shape:

```go
func (s *Server) runTickLoopWithRate(rate time.Duration) {
	// Seed s.tickRate from the parameter so test-injected rates are
	// observable via the field. The loop body below re-reads s.tickRate
	// each iteration so ::speed mutations (NAI-188) take effect on the
	// next sleep computation. Per spec §6: writer (cheat dispatch) and
	// reader (this loop) both run on the tick goroutine, so no lock.
	s.tickRate = rate
	nextTick := time.Now()
	for {
		// NAI-182 — shutdown consumer must run BEFORE any per-tick work
		// so a doomed conn doesn't receive one more tick of activity.
		// Mirrors TS World.cycle (World.ts:419-420 `if (this.shutdown)
		// this.processShutdown();`).
		if s.shutdownTick != -1 && s.currentTick >= s.shutdownTick {
			s.processShutdown()
			if s.shutdownGraceful {
				return // tick loop terminates; Server.Run() returns nil via s.gracefulExit
			}
		}

		start := time.Now()
		drift := start.Sub(nextTick)
		if drift < 0 {
			drift = 0
		}

		s.processClientsIn()
		s.processWorldQueue() // NAI-37: matches TS World.processWorld start-of-cycle ordering
		// NAI-122: processNpcEventQueue moved up to mirror TS World.ts:356
		// (drains BEFORE processPlayers at TS line 376). Closes the
		// V-PARTIAL where AI_SPAWN-populated npc varns
		// (%npc_combat_xp_multiplier and friends) were read as zero by
		// same-tick combat dispatch because the queue drained AFTER
		// processInteractions. DEVIATION-NAI-122-D3 declared in Bundle 0
		// findings: NAI-121 audit's "TS sync-inline" claim was a misread
		// — TS uses a unified queue identical to goscape's, just drained
		// earlier in the tick.
		s.processNpcEventQueue()
		s.processActiveScripts()
		// NAI-134: drain the obj-delayed-spawn queue. Mirrors TS
		// World.cycle ordering at World.ts:563 — runs after script-firing
		// (so same-tick INV_DROPITEM_DELAYED with delay=0 spawns the obj
		// before processNpcs / processInfo reads zone state).
		s.processObjDelayedQueue()
		s.processPlayerTimers()
		// NAI-144: TS World.ts:725 — engineQueue drains between timers and
		// movement. processPlayerEngineQueues mirrors TS
		// Player.processEngineQueue per-player drain semantics.
		s.processPlayerEngineQueues()
		s.processPathing()
		s.processInteractions()
		s.processEnergy() // NAI-135: TS World.ts:731 per-player updateEnergy
		s.processNpcs()
		s.processLogouts()
		s.processLogins()
		s.processInfo()
		s.processZones() // compute ComputeShared before delivery
		s.processClientsOut()
		s.processCleanup()
		s.processSessionLogs() // NAI-74: TS World.cycle session-log block (W.ts:428-442)
		s.currentTick++

		// NAI-188: re-read s.tickRate every iteration so ::speed
		// mutations take effect on the next sleep. Named currentRate
		// (not rate) to avoid shadowing the parameter; per memory
		// plan_var_name_collision.
		currentRate := s.tickRate
		nextTick = nextTick.Add(currentRate)
		delay := currentRate - time.Since(start) - drift
		if delay < 0 {
			delay = 0
		}

		select {
		case <-s.quit:
			return
		case <-time.After(delay):
		}
	}
}
```

The body between `s.processClientsIn()` and `s.currentTick++` is unchanged from the existing implementation — copy verbatim. Only the entry-point `s.tickRate = rate` seed and the post-tick `currentRate := s.tickRate` re-read are new.

- [ ] **Step 4: Run the new test to verify it passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race -run TestTickLoopHonoursFieldRate ./modules/world/...`
Expected: PASS.

- [ ] **Step 5: Run the existing tick-loop test to verify it still passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race -run TestTickLoopIncrementsCurrentTick ./modules/world/...`
Expected: PASS.

- [ ] **Step 6: Run the full world test suite (race-clean)**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/...`
Expected: All tests pass.

- [ ] **Step 7: Commit**

```bash
git add modules/world/tick.go modules/world/server_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-188 T2 — tick loop re-reads s.tickRate each iteration

Seeds s.tickRate from the runTickLoopWithRate parameter at function
entry, then re-reads s.tickRate into a per-iteration local
(currentRate, not rate — avoids shadowing the parameter) for the
nextTick advance and the sleep computation. Mid-loop mutations now
take effect on the next sleep, matching TS World.cycle's per-iteration
reads at World.ts:502+506.

TestTickLoopHonoursFieldRate pins the behavior: 3ms rate runs ~10
ticks in 30ms; switching to 30ms mid-run produces ~2 ticks in 60ms.
Run with -race to validate the single-goroutine invariant (spec §6).
EOF
)"
```

---

## Task 3: Add `case "speed":` to the dev-block cheat switch

**Files:**
- Test: `modules/world/handlers_game_test.go` — six RED tests
- Modify: `modules/world/handlers_game.go:401-429` — append `case "speed":` to the dev-block switch

- [ ] **Step 1: Write the six failing cheat-handler tests**

Open `modules/world/handlers_game_test.go`. Append after `TestHandleClientCheat_Random_SetsAfkEventReady` (around line 587, before the NAI-183 reboot/slowreboot tests):

```go
// --- NAI-188: ::speed dev-block cheat ---
//
// TS ClientCheatHandler.ts:154-167 ports to the dev block at
// modules/world/handlers_game.go. Branch matrix:
//   args == ""       → "Usage: ::speed <ms>"; no state change.
//   parsed < 20      → "::speed input was too low."; no state change.
//   parsed >= 20     → "World speed was changed to {ms}ms"; s.tickRate update.
// Per spec §7.1, non-numeric arg traces to default 20 via parseIntOr
// (TS tryParseInt fallback), which is >= floor → success at 20ms.
// Negative numeric arg (-5) parses to -5, which is < 20 → "too low".

func TestHandleClientCheat_Speed_EmptyArgs_EmitsUsageMessage(t *testing.T) {
	p, cc, s := teleTestPlayer(t)
	p.staffModLevel = 4
	priorRate := s.tickRate

	emitted1 := drainAfterTele(t, p, cc)
	dispatchTeleCheat(t, p, "speed")
	emitted2 := drainAfterTele(t, p, cc)
	all := append(emitted1, emitted2...)

	if !bytes.Contains(all, []byte("Usage: ::speed <ms>")) {
		t.Errorf("missing 'Usage: ::speed <ms>' in emitted bytes")
	}
	if s.tickRate != priorRate {
		t.Errorf("s.tickRate: got %v, want %v (unchanged on empty args)", s.tickRate, priorRate)
	}
}

func TestHandleClientCheat_Speed_BelowFloor_EmitsTooLow(t *testing.T) {
	p, cc, s := teleTestPlayer(t)
	p.staffModLevel = 4
	priorRate := s.tickRate

	emitted1 := drainAfterTele(t, p, cc)
	dispatchTeleCheat(t, p, "speed 19")
	emitted2 := drainAfterTele(t, p, cc)
	all := append(emitted1, emitted2...)

	if !bytes.Contains(all, []byte("::speed input was too low.")) {
		t.Errorf("missing '::speed input was too low.' in emitted bytes")
	}
	if s.tickRate != priorRate {
		t.Errorf("s.tickRate: got %v, want %v (unchanged on too-low input)", s.tickRate, priorRate)
	}
}

func TestHandleClientCheat_Speed_AtFloor_SetsTickRate(t *testing.T) {
	p, cc, s := teleTestPlayer(t)
	p.staffModLevel = 4

	emitted1 := drainAfterTele(t, p, cc)
	dispatchTeleCheat(t, p, "speed 20")
	emitted2 := drainAfterTele(t, p, cc)
	all := append(emitted1, emitted2...)

	if !bytes.Contains(all, []byte("World speed was changed to 20ms")) {
		t.Errorf("missing 'World speed was changed to 20ms' in emitted bytes")
	}
	want := 20 * time.Millisecond
	if s.tickRate != want {
		t.Errorf("s.tickRate after ::speed 20: got %v, want %v", s.tickRate, want)
	}
}

func TestHandleClientCheat_Speed_AboveFloor_SetsTickRate(t *testing.T) {
	p, cc, s := teleTestPlayer(t)
	p.staffModLevel = 4

	emitted1 := drainAfterTele(t, p, cc)
	dispatchTeleCheat(t, p, "speed 100")
	emitted2 := drainAfterTele(t, p, cc)
	all := append(emitted1, emitted2...)

	if !bytes.Contains(all, []byte("World speed was changed to 100ms")) {
		t.Errorf("missing 'World speed was changed to 100ms' in emitted bytes")
	}
	want := 100 * time.Millisecond
	if s.tickRate != want {
		t.Errorf("s.tickRate after ::speed 100: got %v, want %v", s.tickRate, want)
	}
}

func TestHandleClientCheat_Speed_NonNumeric_DefaultsTo20ms(t *testing.T) {
	// Per spec §7.1: TS tryParseInt("banana", 20) returns 20 (the
	// default), and 20 < 20 is false → success branch at 20ms.
	// parseIntOr mirrors this exactly.
	p, cc, s := teleTestPlayer(t)
	p.staffModLevel = 4

	emitted1 := drainAfterTele(t, p, cc)
	dispatchTeleCheat(t, p, "speed banana")
	emitted2 := drainAfterTele(t, p, cc)
	all := append(emitted1, emitted2...)

	if !bytes.Contains(all, []byte("World speed was changed to 20ms")) {
		t.Errorf("missing 'World speed was changed to 20ms' in emitted bytes (non-numeric traces to default 20)")
	}
	want := 20 * time.Millisecond
	if s.tickRate != want {
		t.Errorf("s.tickRate after ::speed banana: got %v, want %v", s.tickRate, want)
	}
}

func TestHandleClientCheat_Speed_Negative_EmitsTooLow(t *testing.T) {
	// Per spec §7.1: parseIntOr("-5", 20) == -5; -5 < 20 → "too low".
	p, cc, s := teleTestPlayer(t)
	p.staffModLevel = 4
	priorRate := s.tickRate

	emitted1 := drainAfterTele(t, p, cc)
	dispatchTeleCheat(t, p, "speed -5")
	emitted2 := drainAfterTele(t, p, cc)
	all := append(emitted1, emitted2...)

	if !bytes.Contains(all, []byte("::speed input was too low.")) {
		t.Errorf("missing '::speed input was too low.' in emitted bytes for negative input")
	}
	if s.tickRate != priorRate {
		t.Errorf("s.tickRate: got %v, want %v (unchanged on negative input)", s.tickRate, priorRate)
	}
}
```

`time` is already imported in `handlers_game_test.go` (line 11) — no import change needed.

- [ ] **Step 2: Run the six new tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race -run 'TestHandleClientCheat_Speed_' ./modules/world/...`
Expected: All six FAIL. The dev-block switch has no `case "speed":` — the cheat falls through with no message emission and no state change.

Failure signatures will look like: `missing 'Usage: ::speed <ms>' in emitted bytes`, etc.

- [ ] **Step 3: Add the `case "speed":` body to the dev block**

Edit `modules/world/handlers_game.go`. Locate the dev-block switch at line 400 (`case "fly":`). Find `case "random":` at line 426-428 and append the new case immediately after it, BEFORE the closing brace of the switch at line 429.

The existing tail of the switch:

```go
case "random":
	// TS L184-186 — primes the AFK event for the next tick.
	p.afkEventReady = true
}
```

becomes:

```go
case "random":
	// TS L184-186 — primes the AFK event for the next tick.
	p.afkEventReady = true
case "speed":
	// TS ClientCheatHandler.ts:154-167. NAI-188.
	// Args layout: single positional integer (ms). Branches:
	//   empty args  → "Usage: ::speed <ms>"; no state change.
	//   parsed < 20 → "::speed input was too low."; no state change.
	//   else        → "World speed was changed to {ms}ms"; mutate s.tickRate.
	// Non-numeric arg: parseIntOr defaults to 20 → success at 20ms
	// (mirrors TS tryParseInt fallback). Per spec §6, no lock — this
	// runs on the tick goroutine, same as the loop that reads s.tickRate.
	if args == "" {
		p.MessageGame("Usage: ::speed <ms>")
		return nil
	}
	// args.shift() in TS takes the first whitespace-delimited token;
	// goscape's `args` is the post-first-space tail. Slice the first
	// whitespace token to match.
	first := args
	if i := strings.IndexAny(args, " \t"); i >= 0 {
		first = args[:i]
	}
	speed := parseIntOr(first, 20)
	if speed < 20 {
		p.MessageGame("::speed input was too low.")
		return nil
	}
	p.MessageGame(fmt.Sprintf("World speed was changed to %dms", speed))
	p.client.server.tickRate = time.Duration(speed) * time.Millisecond
}
```

- [ ] **Step 4: Run the six new tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race -run 'TestHandleClientCheat_Speed_' ./modules/world/...`
Expected: All six PASS.

- [ ] **Step 5: Run the full world test suite (race-clean)**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/...`
Expected: All tests pass.

- [ ] **Step 6: Verify nothing in the broader codebase regressed**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`
Expected: All packages pass.

- [ ] **Step 7: Commit**

```bash
git add modules/world/handlers_game.go modules/world/handlers_game_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-188 T3 — port ::speed dev-block cheat

TS ClientCheatHandler.ts:154-167. Adds `case "speed":` to the existing
dev-block switch (gated on !NodeProduction && staffModLevel >= 4).
Six tests pin the branch matrix:
  - empty args → Usage message, s.tickRate unchanged
  - parsed < 20 → "too low" message, s.tickRate unchanged
  - speed=20 (at floor) → "World speed was changed to 20ms" + s.tickRate update
  - speed=100 → ditto with 100ms
  - non-numeric (banana) → parseIntOr default 20 → success at 20ms
  - negative (-5) → "too low" (negative parses, then < 20 floor rejects)
EOF
)"
```

---

## Task 4: Rewrite carryforward doc-comment + close commit

**Files:**
- Modify: `modules/world/handlers_game.go:370-386` — drop the `speed:` row, update tally

- [ ] **Step 1: Read the current carryforward comment**

Open `modules/world/handlers_game.go` and view lines 370-386. The current text is:

```go
// DEVIATION-NAI-187-D1-CARRYFORWARD — supersedes
// DEVIATION-NAI-186-D2-CARRYFORWARD. 3 TS ClientCheatHandler
// cheats remain unported, all in the dev block (!NP && >=4):
//   reload:  TS L149-150. Calls World.reload() — full cache
//            hot-reload pipeline. No goscape equivalent;
//            substantial new subsystem.
//   rebuild: TS L151-153. Calls World.rebuild() — script-provider
//            hot-reload. Same infra gap as reload.
//   speed:   TS L154-167. Trivial code (~10 LOC) but mutates
//            World.tickRate, currently a package-level const at
//            modules/world/tick.go:15. Right size for its own
//            one-shot follow-up sub-spec.
// NAI-187 retired the admin spawn/interface cluster (locadd /
// npcadd / openmain). Per memory tracker_entry_framing_can_be_
// incomplete: the prior "blocked on dynamic Loc/Npc spawn +
// interface routing" framing was stale at HEAD — all primitives
// existed; sole gap was three ByName helpers in pkg/objtype.
```

- [ ] **Step 2: Rewrite to drop the `speed:` row and update the tally**

Replace with:

```go
// DEVIATION-NAI-188-D1-CARRYFORWARD — supersedes
// DEVIATION-NAI-187-D1-CARRYFORWARD. 2 TS ClientCheatHandler
// cheats remain unported, both in the dev block (!NP && >=4) and
// both blocked on the same infra gap (cache / script hot-reload):
//   reload:  TS L149-150. Calls World.reload() — full cache
//            hot-reload pipeline. No goscape equivalent;
//            substantial new subsystem.
//   rebuild: TS L151-153. Calls World.rebuild() — script-provider
//            hot-reload. Same infra gap as reload.
// NAI-188 retired ::speed (TS L154-167). The tickRate package-level
// const at modules/world/tick.go:15 was promoted to Server.tickRate
// (default initialised to defaultTickRate); the tick loop re-reads
// the field each iteration so the cheat-induced mutation takes
// effect on the next sleep. See spec §6 for the single-goroutine
// concurrency argument.
// NAI-187 retired the admin spawn/interface cluster (locadd /
// npcadd / openmain). Per memory tracker_entry_framing_can_be_
// incomplete: the prior "blocked on dynamic Loc/Npc spawn +
// interface routing" framing was stale at HEAD — all primitives
// existed; sole gap was three ByName helpers in pkg/objtype.
```

- [ ] **Step 3: Verify build remains green**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`
Expected: builds successfully.

- [ ] **Step 4: Verify no test broke due to the comment edit**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...`
Expected: all green.

- [ ] **Step 5: Confirm no stale references to `const tickRate` remain**

Run: `grep -rn "const tickRate\b\|tickRate\s*=\s*600\s*\*\s*time" modules/ pkg/`
Expected: NO matches (only `defaultTickRate` should remain as a constant; `s.tickRate` and `tickRate time.Duration` as field references are fine).

- [ ] **Step 6: Commit the close**

```bash
git add modules/world/handlers_game.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
chore(close): NAI-188 — ::speed cheat ported; carryforward updated

Rewrites DEVIATION-NAI-187-D1-CARRYFORWARD as
DEVIATION-NAI-188-D1-CARRYFORWARD: removes the `speed:` row, updates
the tally from "3 TS cheats remain" to "2", and adds a one-paragraph
NAI-188 retirement note describing the tickRate const→field migration.
The two remaining cheats (reload, rebuild) are both blocked on the
same hot-reload infra gap, which the rewritten comment now states
more plainly.
EOF
)"
```

---

## Self-review (run after completing all four tasks)

After landing T4, run this final-check pass:

- [ ] **A. Branch test sweep**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...`
Expected: all green.

- [ ] **B. Verify all six branch matrices fire**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race -run 'TestHandleClientCheat_Speed_|TestTickLoopHonoursFieldRate|TestTickLoopIncrementsCurrentTick' ./modules/world/... -v`
Expected: 8 tests pass (6 cheat-handler + 1 mid-loop mutation + 1 existing baseline).

- [ ] **C. Spec coverage cross-check**

For each spec §, verify a task implements it:
  - §4.1 (tickRate const→field): T1 + T2
  - §4.2 (::speed case): T3
  - §4.3 (carryforward rewrite): T4
  - §6 (concurrency claim): asserted via -race in T2/T3
  - §7 (six handler tests): T3 step 1
  - §7.2 (tick-loop integration test): T2 step 1
  - §10 (close criteria): T4 + this self-review

- [ ] **D. Memory-entry triage**

This is a straight TS port with established patterns. No new tracker entries expected. If any task surfaced a non-derivable lesson (e.g. a surprising goroutine-boundary discovery), add a one-line entry to `MEMORY.md` per the project's memory protocol BEFORE the close commit and append `Closes memory:` to the close commit trailer per memory `close_commit_memory_trailer`.
