# RuneScript S6h: ChangeStat Trigger Fire Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port TS `Player.changeStat` (Player.ts:1816-1821) so `Player.AddXP`'s level-up event fires the `[changestat,<skill>]` trigger via the engine queue. Refactor `playerQueueRequest` to hold `*script.ScriptFile` directly (shift ID→file resolution from fire-time to enqueue-time). Add sibling `Player.EnqueueScriptFile` method. Closes the S6g final-review follow-up #1.

**Architecture:** Two tasks. Task 1 is a pure-infrastructure queue-API refactor — `playerQueueRequest` stores `*ScriptFile`; new `EnqueueScriptFile` method; existing `EnqueueScriptTyped` becomes a thin wrapper that does `GetByID` then delegates; `processPlayerQueue` simplified to use `req.Script` directly. Existing regression tests in `script_test.go` keep passing because the observable enqueue→fire contract is preserved. Task 2 adds the TS port: `Player.changeStat(stat int)` helper and an AddXP call site, plus 3 behavioral tests.

**Tech Stack:** Go; `modules/world` tick loop; `pkg/script` trigger lookup + ScriptFile type.

**Spec:** [`docs/superpowers/specs/2026-04-21-runescript-s6h-changestat-trigger-design.md`](../specs/2026-04-21-runescript-s6h-changestat-trigger-design.md) (commit `b1ba2f1`)

---

## Task 1: Queue API refactor — store `*ScriptFile` instead of `uint32` ID

**Files:**
- Modify: `modules/world/player_script.go` (`playerQueueRequest` struct + new `EnqueueScriptFile` + refactor `EnqueueScriptTyped`)
- Modify: `modules/world/tick.go` (`processPlayerQueue` simplification)
- Modify: `modules/world/player_script_test.go` (2 new tests)

Build stays green throughout — the observable contract of `EnqueueScriptTyped` is preserved. Existing regression tests (`script_test.go:239, 269, 336, 337`) pass because `RegisterAt(id, sf)` before enqueue means `GetByID(id)` returns the same sf that processPlayerQueue used to resolve at fire time.

- [ ] **Step 1: Write failing tests for the new `EnqueueScriptFile` method.** Open `modules/world/player_script_test.go`. Append at the end:

```go
func TestEnqueueScriptFileDirectPath(t *testing.T) {
	p, _ := newTestPlayer(t)
	sf := &script.ScriptFile{Name: "[test_direct]"}
	p.EnqueueScriptFile(sf, 3, 42, script.QueueNormal)
	if len(p.queue) != 1 {
		t.Fatalf("queue len: got %d, want 1", len(p.queue))
	}
	req := p.queue[0]
	if req.Script != sf {
		t.Errorf("queue[0].Script: got %v, want %v", req.Script, sf)
	}
	if req.Delay != 3 {
		t.Errorf("queue[0].Delay: got %d, want 3", req.Delay)
	}
	if req.IntArg != 42 {
		t.Errorf("queue[0].IntArg: got %d, want 42", req.IntArg)
	}
	if req.Type != script.QueueNormal {
		t.Errorf("queue[0].Type: got %v, want %v", req.Type, script.QueueNormal)
	}
}

func TestEnqueueScriptFileNilIsNoop(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.EnqueueScriptFile(nil, 0, 0, script.QueueNormal)
	if len(p.queue) != 0 {
		t.Errorf("queue len after nil enqueue: got %d, want 0", len(p.queue))
	}
}
```

If `script` is not already imported in `player_script_test.go`, add `"github.com/zsrv/goscape/pkg/script"` to the imports block (it likely is — the file already uses `objtype` plus the existing S6g tests may have added it).

- [ ] **Step 2: Run tests to verify they fail.**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestEnqueueScriptFile" -v
```

Expected: FAIL at build with `p.EnqueueScriptFile undefined` and, after the struct change lands, `req.Script` (field name) undefined if the struct still has `ScriptID`.

- [ ] **Step 3: Change the `playerQueueRequest` struct shape.** Open `modules/world/player_script.go`. Find the struct at lines 10-21:

```go
// playerQueueRequest is one queued fresh-run script request with a
// single int arg. Queue entries are processed in processActiveScripts;
// when Delay reaches zero (or below) the target script runs as a brand-
// new ScriptState. Type selects the queue variant (NORMAL/WEAK/LONG/
// STRONG); STRONG fires even when the player is delayed, the others
// wait for idle.
type playerQueueRequest struct {
	ScriptID uint32
	Delay    int
	IntArg   int
	Type     script.PlayerQueueType
}
```

Replace with:

```go
// playerQueueRequest is one queued fresh-run script request with a
// single int arg. Queue entries are processed in processPlayerQueue;
// when Delay reaches zero (or below) the target script runs as a brand-
// new ScriptState. Type selects the queue variant (NORMAL/WEAK/LONG/
// STRONG); STRONG fires even when the player is delayed, the others
// wait for idle.
//
// As of S6h, Script holds the pre-resolved *ScriptFile directly. ID →
// ScriptFile resolution happens at enqueue time via Player.EnqueueScriptTyped;
// engine-dispatch paths (e.g. changeStat) use Player.EnqueueScriptFile.
type playerQueueRequest struct {
	Script *script.ScriptFile
	Delay  int
	IntArg int
	Type   script.PlayerQueueType
}
```

- [ ] **Step 4: Add `Player.EnqueueScriptFile` and refactor `Player.EnqueueScriptTyped`.** In the same file, find the existing `EnqueueScriptTyped` at lines 37-48 and replace the entire function + doc comment with:

```go
// EnqueueScriptFile appends a queued fresh-run request for a specific
// ScriptFile. Delay=0 fires on the next processPlayerQueue pass (subject
// to the STRONG/NORMAL gate). Nil sf is a silent no-op — engine
// dispatchers (e.g. changeStat) call GetByTrigger and may legitimately
// pass nil when no cache script is registered for the event.
func (p *Player) EnqueueScriptFile(sf *script.ScriptFile, delay, intArg int, qtype script.PlayerQueueType) {
	if sf == nil {
		return
	}
	p.queue = append(p.queue, playerQueueRequest{
		Script: sf,
		Delay:  delay,
		IntArg: intArg,
		Type:   qtype,
	})
}

// EnqueueScriptTyped implements script.ActivePlayer.EnqueueScriptTyped by
// resolving scriptID → *ScriptFile via scriptProvider.GetByID and
// delegating to EnqueueScriptFile. Silent no-op on missing script or
// unwired server — same observable contract as the pre-S6h impl, where
// processPlayerQueue's GetByID check served the same role.
//
// Resolution shifts from fire-time (pre-S6h) to enqueue-time (S6h).
// Same tick boundary in practice; simpler codepath.
func (p *Player) EnqueueScriptTyped(scriptID uint32, delay, intArg int, qtype script.PlayerQueueType) {
	if p.client == nil || p.client.server == nil || p.client.server.scriptProvider == nil {
		return
	}
	p.EnqueueScriptFile(p.client.server.scriptProvider.GetByID(scriptID), delay, intArg, qtype)
}
```

- [ ] **Step 5: Simplify `processPlayerQueue` in `modules/world/tick.go`.** Find the function around lines 219-245:

```go
func (s *Server) processPlayerQueue(p *Player) {
	i := 0
	for i < len(p.queue) {
		req := &p.queue[i]
		req.Delay--
		if req.Delay > 0 {
			i++
			continue
		}
		// STRONG queue fires even when delayed; others wait for idle.
		if p.delayed && req.Type != script.QueueStrong {
			i++
			continue
		}
		scriptID := req.ScriptID
		intArg := req.IntArg
		p.queue = append(p.queue[:i], p.queue[i+1:]...)

		if s.scriptProvider != nil {
			if sf := s.scriptProvider.GetByID(scriptID); sf != nil {
				s.runScript(sf, p, false, []int{intArg}, nil)
			}
		}
		// Don't advance i: we just removed the current element, so i
		// now points to what was the next element (or past end).
	}
}
```

Replace with:

```go
func (s *Server) processPlayerQueue(p *Player) {
	i := 0
	for i < len(p.queue) {
		req := &p.queue[i]
		req.Delay--
		if req.Delay > 0 {
			i++
			continue
		}
		// STRONG queue fires even when delayed; others wait for idle.
		if p.delayed && req.Type != script.QueueStrong {
			i++
			continue
		}
		sf := req.Script
		intArg := req.IntArg
		p.queue = append(p.queue[:i], p.queue[i+1:]...)
		if sf != nil {
			s.runScript(sf, p, false, []int{intArg}, nil)
		}
		// Don't advance i: we just removed the current element, so i
		// now points to what was the next element (or past end).
	}
}
```

The change: `scriptID := req.ScriptID` + `GetByID` lookup becomes `sf := req.Script`. The nil check remains defensive (engine dispatchers may enqueue nil-gated no-ops, though EnqueueScriptFile also short-circuits).

- [ ] **Step 6: Run the new `EnqueueScriptFile` tests to verify they pass.**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestEnqueueScriptFile" -v
```

Both PASS.

- [ ] **Step 7: Run the full regression suite to verify the queue refactor preserves behavior.**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/
```

All tests green — especially the existing queue-path tests (`TestProcessPlayerQueue*`, `TestEnqueueScriptTyped*`, etc. at script_test.go:239/269/336/337) which exercise the full enqueue→fire flow.

- [ ] **Step 8: Run race + vet + gofmt.**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache gofmt -l modules/world/
```

All clean. `gofmt -l` empty for `modules/world/player_script.go`, `modules/world/tick.go`, `modules/world/player_script_test.go`.

- [ ] **Step 9: Commit.**

```bash
git add modules/world/player_script.go modules/world/tick.go modules/world/player_script_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
refactor(world): playerQueueRequest holds *ScriptFile + EnqueueScriptFile

playerQueueRequest.ScriptID uint32 becomes Script *script.ScriptFile.
New Player.EnqueueScriptFile method stores the file pointer directly;
existing Player.EnqueueScriptTyped resolves the ID via
scriptProvider.GetByID and delegates (same public signature, same
observable contract). processPlayerQueue simplified — fires via
req.Script directly instead of GetByID(req.ScriptID).

ID to ScriptFile resolution shifts from fire-time to enqueue-time.
Same tick boundary in practice; simpler codepath, removes one
lookup per queue fire.

Pure infrastructure for S6h Task 2, which will use EnqueueScriptFile
for the changeStat trigger dispatch from AddXP's level-up branch.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

**Report:** commit SHA + full-repo green + race + vet + gofmt clean + confirmation that existing queue-path tests still pass.

---

## Task 2: `Player.changeStat` helper + `Player.AddXP` fire + 3 behavioral tests

**Files:**
- Modify: `modules/world/player_script.go` (new `Player.changeStat(stat int)` helper + `Player.AddXP` calls it inside the level-up branch)
- Modify: `modules/world/player_script_test.go` (3 new ChangeStat-fire tests)

- [ ] **Step 1: Write the failing tests FIRST.** Open `modules/world/player_script_test.go`. Append at the end:

```go
func TestAddXPFiresChangeStatOnLevelUp(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	// Register [changestat,attack=0] — keyed by trigger(165) | (0x2<<8) | (0<<10).
	key := uint32(script.TriggerChangeStat) | (0x2 << 8) | (uint32(objtype.PlayerStatAttack) << 10)
	sf := &script.ScriptFile{
		Name:      "[changestat,attack]",
		LookupKey: key,
	}
	s.scriptProvider.Register(sf)

	p, _ := newTestPlayer(t)
	p.client.server = s
	p.stats[objtype.PlayerStatAttack] = 800
	p.baseLevels[objtype.PlayerStatAttack] = 2
	p.levels[objtype.PlayerStatAttack] = 2

	before := len(p.queue)
	p.AddXP(objtype.PlayerStatAttack, 1000) // → level 3

	if len(p.queue) != before+1 {
		t.Fatalf("queue len: got %d, want %d (+1 changestat)", len(p.queue), before+1)
	}
	req := p.queue[before]
	if req.Script != sf {
		t.Errorf("queue[%d].Script: got %v, want [changestat,attack] (%v)", before, req.Script, sf)
	}
	if req.Type != script.QueueNormal {
		t.Errorf("queue[%d].Type: got %v, want QueueNormal (TS ENGINE equivalent)", before, req.Type)
	}
}

func TestAddXPDoesNotFireChangeStatWithoutLevelUp(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	key := uint32(script.TriggerChangeStat) | (0x2 << 8) | (uint32(objtype.PlayerStatAttack) << 10)
	s.scriptProvider.Register(&script.ScriptFile{Name: "[changestat,attack]", LookupKey: key})

	p, _ := newTestPlayer(t)
	p.client.server = s
	p.stats[objtype.PlayerStatAttack] = 100 // below level-2 threshold (830)
	p.baseLevels[objtype.PlayerStatAttack] = 1
	p.levels[objtype.PlayerStatAttack] = 1

	before := len(p.queue)
	p.AddXP(objtype.PlayerStatAttack, 100) // → 200, still level 1 (< 830)

	if len(p.queue) != before {
		t.Errorf("queue len: got %d, want %d (no level-up = no changestat fire)",
			len(p.queue), before)
	}
}

func TestAddXPChangeStatNoScriptIsNoop(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider() // empty — no changestat script registered

	p, _ := newTestPlayer(t)
	p.client.server = s
	p.stats[objtype.PlayerStatAttack] = 800
	p.baseLevels[objtype.PlayerStatAttack] = 2
	p.levels[objtype.PlayerStatAttack] = 2

	before := len(p.queue)
	p.AddXP(objtype.PlayerStatAttack, 1000) // level up, but no script registered

	if len(p.queue) != before {
		t.Errorf("queue len: got %d, want %d (no registered script = silent no-op)",
			len(p.queue), before)
	}
	// Verify the level-up math still happened.
	if p.baseLevels[objtype.PlayerStatAttack] != 3 {
		t.Errorf("baseLevels: got %d, want 3 (level-up math independent of changeStat)",
			p.baseLevels[objtype.PlayerStatAttack])
	}
}
```

- [ ] **Step 2: Run tests to verify they fail.**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestAddXP(FiresChangeStat|DoesNotFireChangeStat|ChangeStatNoScript)" -v
```

Expected: FAIL. The level-up and no-script tests assert on queue length changes that don't happen yet (AddXP doesn't call changeStat). The no-level-up test may coincidentally pass (nothing gets enqueued without the changeStat call), but the other two will fail.

- [ ] **Step 3: Add the `Player.changeStat` helper.** Open `modules/world/player_script.go`. Find `Player.AddXP` (search for `func (p *Player) AddXP`). Immediately BEFORE `AddXP`, insert:

```go
// changeStat fires the [changestat,<skill>] trigger for the given stat
// slot when a cache script is registered. Enqueued as QueueNormal so it
// runs asynchronously through processPlayerQueue, not inline with the
// triggering action. Matches TS Player.changeStat (Player.ts:1816-1821)
// which uses PlayerQueueType.ENGINE — goscape's closest match is
// QueueNormal (same tick-later semantics, same delayed-player gating).
//
// Silent no-op if no script is registered (GetByTrigger returns nil →
// EnqueueScriptFile's nil-check short-circuits). Called from AddXP's
// level-up branch.
func (p *Player) changeStat(stat int) {
	if p.client == nil || p.client.server == nil || p.client.server.scriptProvider == nil {
		return
	}
	sf := p.client.server.scriptProvider.GetByTrigger(script.TriggerChangeStat, stat, -1)
	p.EnqueueScriptFile(sf, 0, 0, script.QueueNormal)
}
```

- [ ] **Step 4: Wire the call into `Player.AddXP`.** Find the existing level-up block inside `Player.AddXP`:

```go
	if afterBase > beforeBase && int(p.levels[id]) < beforeBase {
		// Drained + level-up: replenish levels by the level delta.
		// Matches TS Player.ts:1767-1770.
		p.levels[id] = uint8(min(int(p.levels[id])+(afterBase-beforeBase), 255))
	}
}
```

Replace with (add a sibling `if` block after the drain-replenish check):

```go
	if afterBase > beforeBase && int(p.levels[id]) < beforeBase {
		// Drained + level-up: replenish levels by the level delta.
		// Matches TS Player.ts:1767-1770.
		p.levels[id] = uint8(min(int(p.levels[id])+(afterBase-beforeBase), 255))
	}
	if afterBase > beforeBase {
		// Level-up: fire the [changestat,<skill>] trigger if registered.
		// Matches TS Player.ts:1772.
		p.changeStat(id)
	}
}
```

Both `if` blocks gate on `afterBase > beforeBase` but are independent. A non-drained level-up still fires changeStat. TS has them similarly separate inside its outer `if` at Player.ts:1766-1772.

- [ ] **Step 5: Run the new ChangeStat tests to verify they pass.**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestAddXP(FiresChangeStat|DoesNotFireChangeStat|ChangeStatNoScript)" -v
```

All 3 PASS.

- [ ] **Step 6: Verify the full AddXP test suite still passes (S6g + S6h together).**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestAddXP" -v
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestStatAdvanceViaScript -v
```

All pass. `TestStatAdvanceViaScript` in particular asserts `p.stats[3] == 150` after `AddXP(3, 50)` starting from 100 XP. 150 XP is below the level-2 threshold (830), so `afterBase > beforeBase` is false and changeStat doesn't fire. No interference with the assertion.

- [ ] **Step 7: Run full repo + quality checks.**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache gofmt -l modules/world/
```

All PASS / clean. `gofmt -l` empty for files you touched.

- [ ] **Step 8: Commit.**

```bash
git add modules/world/player_script.go modules/world/player_script_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): S6h Player.AddXP fires [changestat,<skill>] on level-up

Ports TS Player.changeStat (Player.ts:1816-1821). New unexported
Player.changeStat(stat int) helper does GetByTrigger(TriggerChangeStat,
stat, -1) and enqueues via EnqueueScriptFile as QueueNormal (TS
ENGINE-equivalent). Called from Player.AddXP inside the
`afterBase > beforeBase` block, as a sibling to the drain-replenish
branch — both gate on level-up but are independent.

Three new tests pin the fire semantics:
- fires on level-up with script registered
- doesn't fire without level-up (even if script registered)
- no-op when no script is registered (level-up math still runs)

Closes the S6g final-review follow-up #1 (ChangeStat trigger fire).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

**Report:** commit SHA + all 3 new tests pass + full repo green + race + vet + gofmt clean.

---

## Self-Review Checklist

After both tasks complete:

- [ ] `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...` clean
- [ ] `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...` green
- [ ] `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...` clean
- [ ] `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...` clean
- [ ] `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache gofmt -l modules/world/` empty (or only pre-existing drift)
- [ ] Two commits on main: T1 `refactor(world): playerQueueRequest holds *ScriptFile + EnqueueScriptFile`, T2 `feat(world): S6h Player.AddXP fires [changestat,<skill>] on level-up`
- [ ] `grep -n 'ScriptID' modules/world/` shows no references to `req.ScriptID` or `playerQueueRequest.ScriptID` (the field is gone)
- [ ] Spec coverage:
  - [ ] `playerQueueRequest.Script *script.ScriptFile` field shape → T1
  - [ ] `Player.EnqueueScriptFile(sf, delay, intArg, qtype)` new method → T1
  - [ ] `Player.EnqueueScriptTyped` refactored to resolve ID→file via GetByID → T1
  - [ ] `processPlayerQueue` uses `req.Script` directly → T1
  - [ ] 2 EnqueueScriptFile tests (direct path, nil no-op) → T1
  - [ ] `Player.changeStat(stat int)` helper → T2
  - [ ] `Player.AddXP` calls `changeStat(id)` inside level-up block → T2
  - [ ] 3 ChangeStat behavior tests (fires on level-up, doesn't fire without, no-op without script) → T2
  - [ ] Existing regression tests still pass (TestEnqueueScriptTyped*, TestStatAdvanceViaScript) → both tasks
