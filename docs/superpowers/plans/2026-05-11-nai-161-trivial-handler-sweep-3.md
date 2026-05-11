# NAI-161 Trivial-handler sweep #3 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port 3 unhandled script opcodes (`OpClearQueue` 2011, `OpGetQueue` 2021, `OpPOpHeld` 2076) by adding 2 small `(*Player)` queue-introspection methods + 3 handlers, narrowing the cascade-tail from 21→18 unhandled.

**Architecture:** New `(*Player).UnlinkQueuedScript(scriptID)` walks `p.queue` filtering by pointer-equality against `scriptProvider.GetByID(scriptID)` (TS-faithful default-NORMAL arm: walks queue+weakQueue, leaves engineQueue alone). New `(*Player).QueueCount(scriptID)` does the same filter but skips Weak-typed entries (TS GETQUEUE iterates `queue.all()` only). P_OPHELD ports as TS-faithful `'unimplemented'` error stub behind the `ProtectedActivePlayer` gate.

**Tech Stack:** Go 1.26+. Spec: `docs/superpowers/specs/2026-05-11-nai-161-trivial-handler-sweep-3-design.md`. TS source: `LostCityRS/Engine-TS/src/engine/script/handlers/PlayerOps.ts:381-383,903-912,1045-1048`; `entity/Player.ts:833-852`.

---

## File Structure

**Modify:**
- `modules/world/player_script.go` — append `UnlinkQueuedScript`, `QueueCount` near `clearWeakQueue` (~L95-103)
- `modules/world/player_script_test.go` — append unit tests for both new `*Player` methods
- `pkg/script/active.go` — widen `ActivePlayer` interface (insert before its closing `}` at L697)
- `pkg/script/runner_test.go` — add `mockPlayer` recorder fields + adapter methods
- `pkg/script/handlers_player.go` — append `handleClearQueue`, `handleGetQueue`, `handlePOpHeld` (after `handleHeadIconsSet` at ~L1679)
- `pkg/script/handlers_player_test.go` — append handler-level tests; extend `TestHandlersRequireActivePlayer` table
- `pkg/script/handlers.go` — register 3 new handlers in the `handlers` map (after the NAI-160 trailing block ~L539-545)

**No new files.**

---

## Conventions for every task

- All `go` commands prefixed: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go ...`
- All commits: `git commit --no-gpg-sign`
- Co-Authored-By trailer on every commit:
  ```
  Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
  ```

---

## Task 1: `(*Player).UnlinkQueuedScript`

**Files:**
- Modify: `modules/world/player_script.go` — append method after `clearWeakQueue` (~L103)
- Test: `modules/world/player_script_test.go` — append 5 unit tests

- [ ] **Step 1.1: Pre-flight grep — verify pointer-stability invariant**

Confirm `Provider.scripts` is built once (not rebuilt in place during normal operation), so pointer-equality of `req.Script == target` is stable across the enqueue→CLEARQUEUE window. Per Risk R1 in the spec.

Run:
```bash
rg -n "p\.scripts\s*=\s|p\.scripts\[" pkg/script/provider.go
```

Expected: only the `Register` append site (`p.scripts = append(p.scripts, f)` at ~L183) and the `GetByID` read site (`p.scripts[id]` at L176). No in-place reassignment.

If unexpected output appears, STOP and flag the controller before proceeding — the pointer-equality assumption needs revisiting.

- [ ] **Step 1.2: Write failing unit tests**

Append to `modules/world/player_script_test.go` (after the existing `TestEnqueueScriptFileNilIsNoop` block):

```go
// TestUnlinkQueuedScriptDropsMatchingEntries pins the basic filter
// behavior: enqueue 3 scripts at distinct IDs, unlink the middle one,
// assert the remaining two are preserved in original order. NAI-161 T1.
func TestUnlinkQueuedScriptDropsMatchingEntries(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	sf10 := &script.ScriptFile{Name: "[test_id10]"}
	sf20 := &script.ScriptFile{Name: "[test_id20]"}
	sf30 := &script.ScriptFile{Name: "[test_id30]"}
	s.scriptProvider.Register(sf10) // id=0
	s.scriptProvider.Register(sf20) // id=1
	s.scriptProvider.Register(sf30) // id=2

	p, _ := newTestPlayer(t)
	p.client.server = s
	p.EnqueueScriptFile(sf10, 0, nil, nil, script.QueueNormal)
	p.EnqueueScriptFile(sf20, 0, nil, nil, script.QueueNormal)
	p.EnqueueScriptFile(sf30, 0, nil, nil, script.QueueNormal)

	p.UnlinkQueuedScript(1) // id=1 → sf20

	if len(p.queue) != 2 {
		t.Fatalf("queue len: got %d, want 2", len(p.queue))
	}
	if p.queue[0].Script != sf10 {
		t.Errorf("queue[0].Script: got %v, want sf10", p.queue[0].Script)
	}
	if p.queue[1].Script != sf30 {
		t.Errorf("queue[1].Script: got %v, want sf30", p.queue[1].Script)
	}
}

// TestUnlinkQueuedScriptWalksAllNonEngineTypes pins the TS-faithful
// default-NORMAL arm: walks BOTH queue and weakQueue, regardless of
// Type discriminator. Engine entries live in p.engineQueue (separate
// slice) and are untouched. NAI-161 T1 — deviation
// NAI-161-D-QUEUE-TYPE-MAPPING.
func TestUnlinkQueuedScriptWalksAllNonEngineTypes(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	sf10 := &script.ScriptFile{Name: "[test_id10]"}
	s.scriptProvider.Register(sf10) // id=0

	p, _ := newTestPlayer(t)
	p.client.server = s
	p.EnqueueScriptFile(sf10, 0, nil, nil, script.QueueNormal)
	p.EnqueueScriptFile(sf10, 0, nil, nil, script.QueueWeak)
	p.EnqueueScriptFile(sf10, 0, nil, nil, script.QueueStrong)
	p.EnqueueScriptFile(sf10, 0, nil, nil, script.QueueLong)

	p.UnlinkQueuedScript(0)

	if len(p.queue) != 0 {
		t.Errorf("queue len after unlink: got %d, want 0 (all 4 types should match)", len(p.queue))
	}
}

// TestUnlinkQueuedScriptLeavesEngineQueueIntact pins that the
// engineQueue (separate slice) is NOT walked by the default-NORMAL
// arm of unlinkQueuedScript. NAI-161 T1.
func TestUnlinkQueuedScriptLeavesEngineQueueIntact(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	sf10 := &script.ScriptFile{Name: "[test_id10]"}
	s.scriptProvider.Register(sf10) // id=0

	p, _ := newTestPlayer(t)
	p.client.server = s
	p.EnqueueScriptFile(sf10, 0, nil, nil, script.QueueNormal)
	p.EnqueueScriptFile(sf10, 0, nil, nil, script.QueueEngine)

	p.UnlinkQueuedScript(0)

	if len(p.queue) != 0 {
		t.Errorf("queue len: got %d, want 0 (normal entry should be dropped)", len(p.queue))
	}
	if len(p.engineQueue) != 1 {
		t.Errorf("engineQueue len: got %d, want 1 (engine entry must be preserved)", len(p.engineQueue))
	}
}

// TestUnlinkQueuedScriptNilServerIsNoop pins the defensive guard:
// a Player with no client.server (or no scriptProvider) does not
// panic and is a no-op. Mirrors EnqueueScriptArgs defensive shape at
// player_script.go:127. NAI-161 T1 — deviation
// NAI-161-D-CLEARQUEUE-NIL-PROVIDER.
func TestUnlinkQueuedScriptNilServerIsNoop(t *testing.T) {
	p, _ := newTestPlayer(t)
	// p.client.server is nil by default — newTestPlayer doesn't wire a Server.
	p.UnlinkQueuedScript(99)
	if len(p.queue) != 0 {
		t.Errorf("queue len: got %d, want 0", len(p.queue))
	}
}

// TestUnlinkQueuedScriptUnknownIDIsNoop pins TS-equivalent "scriptId
// has no matches → zero iterations": when GetByID returns nil for an
// out-of-range scriptID, the queue is unchanged. NAI-161 T1.
func TestUnlinkQueuedScriptUnknownIDIsNoop(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	sf10 := &script.ScriptFile{Name: "[test_id10]"}
	s.scriptProvider.Register(sf10) // id=0

	p, _ := newTestPlayer(t)
	p.client.server = s
	p.EnqueueScriptFile(sf10, 0, nil, nil, script.QueueNormal)

	p.UnlinkQueuedScript(99) // id=99 is out of range

	if len(p.queue) != 1 {
		t.Errorf("queue len: got %d, want 1 (bogus scriptID is no-op)", len(p.queue))
	}
}
```

- [ ] **Step 1.3: Run tests to verify they fail**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestUnlinkQueuedScript' -v
```

Expected: FAIL with `p.UnlinkQueuedScript undefined` compile error.

- [ ] **Step 1.4: Write the minimal implementation**

Append to `modules/world/player_script.go` directly after `clearWeakQueue` (currently ends at L103):

```go
// UnlinkQueuedScript removes every p.queue entry whose Script resolves
// to the script at scriptID (default-NORMAL TS arm). Walks the entire
// p.queue regardless of Type discriminator — this matches TS
// unlinkQueuedScript's default branch which walks both `queue` and
// `weakQueue` (Player.ts:843-851). p.engineQueue is intentionally
// untouched: TS gates engineQueue iteration behind type=ENGINE, which
// CLEARQUEUE never passes (the only consumer at this time).
//
// No-op when scriptID does not resolve to a registered script (zero
// possible matches — TS iterates and finds nothing in the same
// scenario). Goscape matches by `req.Script == target` pointer-equality
// after a single provider lookup; pointer stability holds because
// Provider.scripts is append-only (provider.go).
//
// (goscape defensive; TS skips this check) The nil-server guard mirrors
// EnqueueScriptArgs at player_script.go:127 — load-bearing for test
// fixtures that don't wire a Server.
//
// Mirrors TS Player.unlinkQueuedScript(scriptId, type=NORMAL) at
// Engine-TS/src/engine/entity/Player.ts:833-852. NAI-161 T1.
func (p *Player) UnlinkQueuedScript(scriptID int) {
	if p.client == nil || p.client.server == nil || p.client.server.scriptProvider == nil {
		return
	}
	target := p.client.server.scriptProvider.GetByID(uint32(scriptID))
	if target == nil {
		return
	}
	out := p.queue[:0]
	for _, req := range p.queue {
		if req.Script != target {
			out = append(out, req)
		}
	}
	p.queue = out
}
```

- [ ] **Step 1.5: Run tests to verify they pass**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestUnlinkQueuedScript' -v
```

Expected: PASS — all 5 subtests green.

- [ ] **Step 1.6: Run full modules/world test suite**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...
```

Expected: PASS — no regressions.

- [ ] **Step 1.7: Commit**

```bash
git add modules/world/player_script.go modules/world/player_script_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-161 T1 — (*Player).UnlinkQueuedScript

Walks p.queue dropping entries whose Script pointer matches
scriptProvider.GetByID(scriptID). Default-NORMAL TS arm covers
both queue and weakQueue; engineQueue is untouched per
NAI-161-D-QUEUE-TYPE-MAPPING.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: `(*Player).QueueCount`

**Files:**
- Modify: `modules/world/player_script.go` — append method after `UnlinkQueuedScript` (added in T1)
- Test: `modules/world/player_script_test.go` — append 4 unit tests

- [ ] **Step 2.1: Write failing unit tests**

Append to `modules/world/player_script_test.go` (after the T1 tests):

```go
// TestQueueCountExcludesWeak pins TS GETQUEUE semantics: walks
// queue.all() only (NOT weakQueue). Goscape filters p.queue to
// Type != QueueWeak. NAI-161 T2 — deviation
// NAI-161-D-QUEUE-TYPE-MAPPING.
func TestQueueCountExcludesWeak(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	sf10 := &script.ScriptFile{Name: "[test_id10]"}
	s.scriptProvider.Register(sf10) // id=0

	p, _ := newTestPlayer(t)
	p.client.server = s
	p.EnqueueScriptFile(sf10, 0, nil, nil, script.QueueNormal)
	p.EnqueueScriptFile(sf10, 0, nil, nil, script.QueueStrong)
	p.EnqueueScriptFile(sf10, 0, nil, nil, script.QueueLong)
	p.EnqueueScriptFile(sf10, 0, nil, nil, script.QueueWeak)

	got := p.QueueCount(0)
	if got != 3 {
		t.Errorf("QueueCount(0): got %d, want 3 (Normal+Strong+Long; Weak excluded)", got)
	}
}

// TestQueueCountExcludesEngineQueue pins that engineQueue is a
// separate slice and is never counted by QueueCount. NAI-161 T2.
func TestQueueCountExcludesEngineQueue(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	sf10 := &script.ScriptFile{Name: "[test_id10]"}
	s.scriptProvider.Register(sf10) // id=0

	p, _ := newTestPlayer(t)
	p.client.server = s
	p.EnqueueScriptFile(sf10, 0, nil, nil, script.QueueNormal)
	p.EnqueueScriptFile(sf10, 0, nil, nil, script.QueueEngine)

	got := p.QueueCount(0)
	if got != 1 {
		t.Errorf("QueueCount(0): got %d, want 1 (engine entry excluded)", got)
	}
}

// TestQueueCountUnknownIDReturnsZero pins that an out-of-range
// scriptID resolves to nil → returns 0. Mirrors TS finding zero
// matches in the same scenario. NAI-161 T2.
func TestQueueCountUnknownIDReturnsZero(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	sf10 := &script.ScriptFile{Name: "[test_id10]"}
	s.scriptProvider.Register(sf10) // id=0

	p, _ := newTestPlayer(t)
	p.client.server = s
	p.EnqueueScriptFile(sf10, 0, nil, nil, script.QueueNormal)

	got := p.QueueCount(99)
	if got != 0 {
		t.Errorf("QueueCount(99): got %d, want 0 (bogus scriptID)", got)
	}
}

// TestQueueCountNilServerReturnsZero pins the defensive guard.
// NAI-161 T2 — deviation NAI-161-D-CLEARQUEUE-NIL-PROVIDER.
func TestQueueCountNilServerReturnsZero(t *testing.T) {
	p, _ := newTestPlayer(t)
	// p.client.server is nil by default.
	got := p.QueueCount(99)
	if got != 0 {
		t.Errorf("QueueCount on nil-server player: got %d, want 0", got)
	}
}
```

- [ ] **Step 2.2: Run tests to verify they fail**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestQueueCount' -v
```

Expected: FAIL with `p.QueueCount undefined` compile error.

- [ ] **Step 2.3: Write the minimal implementation**

Append to `modules/world/player_script.go` directly after `UnlinkQueuedScript`:

```go
// QueueCount returns the number of non-Weak p.queue entries whose
// Script resolves to the script at scriptID. Mirrors TS GETQUEUE at
// PlayerOps.ts:903-912 which walks `state.activePlayer.queue.all()`
// only — NOT weakQueue and NOT engineQueue. Goscape's unified p.queue
// holds Normal/Strong/Long/Weak entries; the Type != QueueWeak filter
// reproduces TS's `queue` vs `weakQueue` partition. p.engineQueue is
// a separate slice and is intentionally excluded.
//
// (goscape defensive; TS skips this check) See UnlinkQueuedScript.
//
// NAI-161 T2.
func (p *Player) QueueCount(scriptID int) int {
	if p.client == nil || p.client.server == nil || p.client.server.scriptProvider == nil {
		return 0
	}
	target := p.client.server.scriptProvider.GetByID(uint32(scriptID))
	if target == nil {
		return 0
	}
	n := 0
	for _, req := range p.queue {
		if req.Type == script.QueueWeak {
			continue
		}
		if req.Script == target {
			n++
		}
	}
	return n
}
```

- [ ] **Step 2.4: Run tests to verify they pass**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestQueueCount' -v
```

Expected: PASS — all 4 subtests green.

- [ ] **Step 2.5: Run full modules/world test suite**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...
```

Expected: PASS.

- [ ] **Step 2.6: Commit**

```bash
git add modules/world/player_script.go modules/world/player_script_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-161 T2 — (*Player).QueueCount

Counts non-Weak p.queue entries whose Script matches
scriptProvider.GetByID(scriptID). Mirrors TS GETQUEUE's
queue.all() iteration; weakQueue and engineQueue intentionally
excluded per NAI-161-D-QUEUE-TYPE-MAPPING.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: Widen `ActivePlayer` interface + add `mockPlayer` adapters

**Files:**
- Modify: `pkg/script/active.go` — insert 2 method declarations before the closing `}` of `type ActivePlayer interface` (~L697)
- Modify: `pkg/script/runner_test.go` — add 2 recorder fields to `mockPlayer` + 2 adapter methods

- [ ] **Step 3.1: Pre-flight grep — enumerate ActivePlayer impls**

Confirm only `mockPlayer` and `*Player` implement `ActivePlayer` (NAI-160 spec §4 R7 — re-verify at HEAD).

Run:
```bash
rg -ln "MessageGame\(msg string\)" pkg/script/ modules/world/
```

Expected: exactly two file matches — `pkg/script/runner_test.go` (mockPlayer) and `modules/world/player_script.go` or `modules/world/player.go` (*Player). If a third impl appears (e.g., another mock in NPC tests), STOP and flag.

- [ ] **Step 3.2: Verify *Player satisfies the soon-to-be-widened interface**

T1+T2 added both methods to `*Player`. Verify compile cleanliness across the cross-package boundary:

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
```

Expected: clean build (no `*Player does not implement script.ActivePlayer` error yet — the interface hasn't widened).

- [ ] **Step 3.3: Widen the ActivePlayer interface**

Edit `pkg/script/active.go`. Insert immediately BEFORE the closing brace of the `ActivePlayer` interface at L697 (right after the existing `AddSessionLog` method declaration at L696):

```go

	// UnlinkQueuedScript drops queued fresh-run requests whose script
	// resolves to scriptID. Mirrors TS Player.unlinkQueuedScript with
	// the default NORMAL arm (walks queue + weakQueue; engineQueue
	// untouched). Backing impl at modules/world/player_script.go.
	// NAI-161 T3 — wired by CLEARQUEUE (OpClearQueue, PlayerOps.ts:1045-1048).
	UnlinkQueuedScript(scriptID int)

	// QueueCount returns the count of non-Weak queued requests whose
	// script resolves to scriptID. Mirrors TS GETQUEUE iteration over
	// queue.all() (PlayerOps.ts:907-911). Backing impl at
	// modules/world/player_script.go. NAI-161 T3 — wired by GETQUEUE
	// (OpGetQueue).
	QueueCount(scriptID int) int
```

- [ ] **Step 3.4: Run `go build` to surface mockPlayer compile gap**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
```

Expected: clean build (the `mockPlayer` interface check fires only during test compile, not regular `go build`).

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./pkg/script/
```

Expected: clean (vet doesn't run the test build).

Now force the test compile:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./pkg/script/ -run xxxxxNeverMatchesxxxxx
```

Expected: FAIL with `*mockPlayer does not implement ActivePlayer (missing UnlinkQueuedScript method)` (or similar — the missing-method error from the test compile).

- [ ] **Step 3.5: Add mockPlayer recorder fields**

Edit `pkg/script/runner_test.go`. Insert the following inside the `mockPlayer` struct definition (after the NAI-160 trailing fields at ~L390-395, just before the closing `}`):

```go

	// NAI-161 T1/T2: queue-introspection recorders.
	unlinkScriptCalls  []int       // every UnlinkQueuedScript call's scriptID
	queueCountByScript map[int]int // scriptID → return value; unset entries return 0
```

- [ ] **Step 3.6: Add mockPlayer adapter methods**

Edit `pkg/script/runner_test.go`. Append to the existing block of `func (m *mockPlayer) ...` adapters (after the existing NAI-160 adapter block, near where the other recorder methods live; pick any consistent insertion point that matches the file's grouping):

```go

// NAI-161 T3: queue-introspection adapters.
func (m *mockPlayer) UnlinkQueuedScript(scriptID int) {
	m.unlinkScriptCalls = append(m.unlinkScriptCalls, scriptID)
}

func (m *mockPlayer) QueueCount(scriptID int) int {
	return m.queueCountByScript[scriptID]
}
```

Note: a nil `map[int]int` returns 0 on read in Go — no nil-guard required.

- [ ] **Step 3.7: Run test compile to verify the interface is satisfied**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./pkg/script/ -run xxxxxNeverMatchesxxxxx
```

Expected: PASS (0 tests run, but the test binary compiles, proving `mockPlayer` satisfies the widened interface).

- [ ] **Step 3.8: Run full pkg/script test suite**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/...
```

Expected: PASS — no regressions. (Existing tests don't exercise the new methods yet; T4-T6 add those.)

- [ ] **Step 3.9: Commit**

```bash
git add pkg/script/active.go pkg/script/runner_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-161 T3 — widen ActivePlayer with queue-introspection

Adds UnlinkQueuedScript(int) and QueueCount(int) int to the
ActivePlayer interface. mockPlayer gets matching recorder fields
(unlinkScriptCalls, queueCountByScript) and adapter methods.
*Player already satisfies via NAI-161 T1/T2.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: `handleClearQueue` (OpClearQueue, 2011)

**Files:**
- Modify: `pkg/script/handlers_player.go` — append `handleClearQueue` after the existing NAI-160 trailing handlers (~L1679)
- Modify: `pkg/script/handlers.go` — add 1 registration line in the `handlers` map (after the NAI-160 trailing block at ~L545)
- Modify: `pkg/script/handlers_player_test.go` — add positive test + extend `TestHandlersRequireActivePlayer` table

- [ ] **Step 4.1: Pre-flight grep — confirm `requireActivePlayer` signature**

Run:
```bash
rg -n "^func requireActivePlayer\b" pkg/script/handlers_player.go
```

Expected: `pkg/script/handlers_player.go:35:func requireActivePlayer(s *ScriptState, op string) error {` — confirms two-arg signature used by all existing handlers.

- [ ] **Step 4.2: Write failing handler test**

Append to `pkg/script/handlers_player_test.go` (near the existing handler tests; pick an insertion point grouped with the other NAI-160 player ops, e.g., after `TestPExactMoveInvalidCoord` at ~L5193):

```go
// TestClearQueueDispatch pins OpClearQueue: pop the scriptID arg,
// delegate to ActivePlayer.UnlinkQueuedScript. Mirrors TS
// PlayerOps.ts:1045-1048. NAI-161 T4.
func TestClearQueueDispatch(t *testing.T) {
	mp := &mockPlayer{}
	sf := &ScriptFile{
		Name: "[clearqueue,test]",
		Opcodes: []Opcode{
			OpPushConstantInt, // push scriptID=42
			OpClearQueue,
			OpReturn,
		},
		IntOperands:      []int32{42, 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	state := Init(sf, mp, false, nil, nil)
	state.Pointers |= PtrActivePlayer
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := mp.unlinkScriptCalls; len(got) != 1 || got[0] != 42 {
		t.Errorf("unlinkScriptCalls: got %v, want [42]", got)
	}
}
```

- [ ] **Step 4.3: Extend the no-active-player table**

Edit `pkg/script/handlers_player_test.go`. In `TestHandlersRequireActivePlayer` (~L962-1002), append a row to the `cases` slice after the NAI-160 trailing entry (`{"P_EXACTMOVE", OpPExactMove},`):

```go
		// NAI-161 T4.
		{"CLEARQUEUE", OpClearQueue},
```

- [ ] **Step 4.4: Run tests to verify they fail**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run 'TestClearQueueDispatch|TestHandlersRequireActivePlayer/CLEARQUEUE' -v
```

Expected: FAIL with `no handler for CLEARQUEUE (opcode 2011)` or similar — handler not registered yet.

- [ ] **Step 4.5: Write the handler**

Append to `pkg/script/handlers_player.go` (after `handleHeadIconsSet` at ~L1679):

```go

// handleClearQueue implements OpClearQueue (TS CLEARQUEUE at
// PlayerOps.ts:1045-1048). Pops a scriptID, delegates to
// ActivePlayer.UnlinkQueuedScript — which (per NAI-161 T1) walks the
// player's p.queue and drops every entry whose Script resolves to that
// scriptID. NAI-161 T4.
func handleClearQueue(s *ScriptState) error {
	if err := requireActivePlayer(s, "CLEARQUEUE"); err != nil {
		return err
	}
	s.Self.UnlinkQueuedScript(s.PopInt())
	return nil
}
```

- [ ] **Step 4.6: Register the handler**

Edit `pkg/script/handlers.go`. Append 1 line to the `handlers` map after the NAI-160 trailing entries (after `OpHeadIconsSet: handleHeadIconsSet,` at ~L545, but before the closing `}` of the map):

```go
	// NAI-161 T4: CLEARQUEUE.
	OpClearQueue: handleClearQueue,
```

- [ ] **Step 4.7: Run tests to verify they pass**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run 'TestClearQueueDispatch|TestHandlersRequireActivePlayer/CLEARQUEUE' -v
```

Expected: PASS.

- [ ] **Step 4.8: Run full pkg/script test suite**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/...
```

Expected: PASS — no regressions.

- [ ] **Step 4.9: Commit**

```bash
git add pkg/script/handlers_player.go pkg/script/handlers.go pkg/script/handlers_player_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-161 T4 — CLEARQUEUE handler (opcode 2011)

Pops a scriptID, delegates to ActivePlayer.UnlinkQueuedScript.
Mirrors TS PlayerOps.ts:1045-1048. Hottest unhandled op at HEAD
(42 Content callers — minigame teardowns + engine.rs2 command).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: `handleGetQueue` (OpGetQueue, 2021)

**Files:**
- Modify: `pkg/script/handlers_player.go` — append after T4's `handleClearQueue`
- Modify: `pkg/script/handlers.go` — 1 registration line after T4's entry
- Modify: `pkg/script/handlers_player_test.go` — positive test + extend require-active table

- [ ] **Step 5.1: Write failing handler tests**

Append to `pkg/script/handlers_player_test.go` (after T4's `TestClearQueueDispatch`):

```go
// TestGetQueueReturnsSeededCount pins OpGetQueue: pop a scriptID,
// push ActivePlayer.QueueCount(scriptID). Mirrors TS
// PlayerOps.ts:903-912. NAI-161 T5.
func TestGetQueueReturnsSeededCount(t *testing.T) {
	mp := &mockPlayer{
		queueCountByScript: map[int]int{7: 3},
	}
	sf := &ScriptFile{
		Name: "[getqueue,test]",
		Opcodes: []Opcode{
			OpPushConstantInt, // push scriptID=7
			OpGetQueue,
			OpReturn,
		},
		IntOperands:      []int32{7, 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	state := Init(sf, mp, false, nil, nil)
	state.Pointers |= PtrActivePlayer
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := state.PopInt(); got != 3 {
		t.Errorf("GETQUEUE: got %d, want 3", got)
	}
}

// TestGetQueueNoMatchReturnsZero pins zero-result behavior: an
// unmapped scriptID returns the Go zero-value of int via the mock's
// nil-map read. Mirrors TS finding zero loop iterations. NAI-161 T5.
func TestGetQueueNoMatchReturnsZero(t *testing.T) {
	mp := &mockPlayer{} // queueCountByScript is nil
	sf := &ScriptFile{
		Name: "[getqueue_zero,test]",
		Opcodes: []Opcode{
			OpPushConstantInt, // push scriptID=99
			OpGetQueue,
			OpReturn,
		},
		IntOperands:      []int32{99, 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	state := Init(sf, mp, false, nil, nil)
	state.Pointers |= PtrActivePlayer
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := state.PopInt(); got != 0 {
		t.Errorf("GETQUEUE no-match: got %d, want 0", got)
	}
}
```

- [ ] **Step 5.2: Extend the no-active-player table**

Edit `pkg/script/handlers_player_test.go`. In `TestHandlersRequireActivePlayer`, append:

```go
		// NAI-161 T5.
		{"GETQUEUE", OpGetQueue},
```

- [ ] **Step 5.3: Run tests to verify they fail**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run 'TestGetQueue|TestHandlersRequireActivePlayer/GETQUEUE' -v
```

Expected: FAIL — `no handler for GETQUEUE (opcode 2021)`.

- [ ] **Step 5.4: Write the handler**

Append to `pkg/script/handlers_player.go` (after T4's `handleClearQueue`):

```go

// handleGetQueue implements OpGetQueue (TS GETQUEUE at
// PlayerOps.ts:903-912). Pops a scriptID, pushes
// ActivePlayer.QueueCount(scriptID) — the count of non-Weak queue
// entries whose Script matches. The for-loop in the TS body lives
// inside QueueCount per NAI-161 T2. NAI-161 T5.
func handleGetQueue(s *ScriptState) error {
	if err := requireActivePlayer(s, "GETQUEUE"); err != nil {
		return err
	}
	s.PushInt(s.Self.QueueCount(s.PopInt()))
	return nil
}
```

- [ ] **Step 5.5: Register the handler**

Edit `pkg/script/handlers.go`. Append 1 line after T4's entry:

```go
	// NAI-161 T5: GETQUEUE.
	OpGetQueue: handleGetQueue,
```

- [ ] **Step 5.6: Run tests to verify they pass**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run 'TestGetQueue|TestHandlersRequireActivePlayer/GETQUEUE' -v
```

Expected: PASS — all subtests green.

- [ ] **Step 5.7: Run full pkg/script test suite**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/...
```

Expected: PASS.

- [ ] **Step 5.8: Commit**

```bash
git add pkg/script/handlers_player.go pkg/script/handlers.go pkg/script/handlers_player_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-161 T5 — GETQUEUE handler (opcode 2021)

Pops a scriptID, pushes ActivePlayer.QueueCount(scriptID).
Mirrors TS PlayerOps.ts:903-912 — the queue.all() iteration
lives inside QueueCount per NAI-161 T2.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: `handlePOpHeld` (OpPOpHeld, 2076)

**Files:**
- Modify: `pkg/script/handlers_player.go` — append after T5's `handleGetQueue`
- Modify: `pkg/script/handlers.go` — 1 registration line after T5's entry
- Modify: `pkg/script/handlers_player_test.go` — 2 tests (unimplemented + protected-gate)

- [ ] **Step 6.1: Pre-flight grep — confirm protected-gate error format**

Run:
```bash
rg -n "script not protected" pkg/script/handlers_player.go
```

Expected: `pkg/script/handlers_player.go:62:		return errors.New(op + ": script not protected")` — confirms the `<OPNAME>: script not protected` error string used by the unprotected-gate test below.

- [ ] **Step 6.2: Write failing handler tests**

Append to `pkg/script/handlers_player_test.go` (after T5's tests):

```go
// TestPOpHeldUnimplemented pins OpPOpHeld's TS-faithful
// 'unimplemented' error stub. Protected gate passes (both pointer
// flags set), then handler returns the unimplemented error.
// Mirrors TS PlayerOps.ts:381-383
// (`checkedHandler(ProtectedActivePlayer, () => { throw new Error('unimplemented'); })`).
// NAI-161 T6 — deviation NAI-161-D-POPHELD-STUB.
func TestPOpHeldUnimplemented(t *testing.T) {
	mp := &mockPlayer{}
	sf := newSingleOp("[p_opheld,test]", OpPOpHeld)
	state := Init(sf, mp, false, nil, nil)
	state.Pointers |= PtrActivePlayer | PtrProtectedActivePlayer
	err := Execute(state)
	if err == nil {
		t.Fatalf("Execute: got nil err, want P_OPHELD: unimplemented")
	}
	if got := err.Error(); !strings.Contains(got, "P_OPHELD") || !strings.Contains(got, "unimplemented") {
		t.Errorf("err: got %q, want substrings 'P_OPHELD' and 'unimplemented'", got)
	}
}

// TestPOpHeldRequiresProtected pins gate-ordering: the
// ProtectedActivePlayer check fires BEFORE the unimplemented stub.
// Without the protect flag, the error is "script not protected",
// not "unimplemented". NAI-161 T6.
func TestPOpHeldRequiresProtected(t *testing.T) {
	mp := &mockPlayer{}
	sf := newSingleOp("[p_opheld_unprotected,test]", OpPOpHeld)
	state := Init(sf, mp, false, nil, nil)
	state.Pointers |= PtrActivePlayer // protect flag intentionally unset
	err := Execute(state)
	if err == nil {
		t.Fatalf("Execute: got nil err, want P_OPHELD: script not protected")
	}
	if got := err.Error(); !strings.Contains(got, "P_OPHELD") || !strings.Contains(got, "script not protected") {
		t.Errorf("err: got %q, want substrings 'P_OPHELD' and 'script not protected'", got)
	}
	if got := err.Error(); strings.Contains(got, "unimplemented") {
		t.Errorf("err: got %q, must NOT contain 'unimplemented' — protected gate must fire first", got)
	}
}
```

Note: P_OPHELD is NOT added to `TestHandlersRequireActivePlayer` — that table tests the `ActivePlayer` (non-protected) gate, but P_OPHELD requires `ProtectedActivePlayer`. The `TestPOpHeldRequiresProtected` test above covers the protected-gate negative case explicitly. (The `requireActivePlayer` no-active-player branch is also covered: `requireProtectedActivePlayer` chains through `requireActivePlayer` first per `handlers_player.go:57-65`, so without `PtrActivePlayer` the error would be `P_OPHELD: no active player` — but that's the unprotected handler's failure mode and not worth a separate table entry.)

- [ ] **Step 6.3: Run tests to verify they fail**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run 'TestPOpHeld' -v
```

Expected: FAIL — `no handler for P_OPHELD (opcode 2076)`.

- [ ] **Step 6.4: Write the handler**

Append to `pkg/script/handlers_player.go` (after T5's `handleGetQueue`):

```go

// handlePOpHeld implements OpPOpHeld (TS P_OPHELD at PlayerOps.ts:381-383).
// TS-faithful 'unimplemented' error stub behind the ProtectedActivePlayer
// gate. Stub remains until OPHELD trigger plumbing is ported (separate
// cohort with OcOp/LcOp/OcIop). NAI-161 T6 — deviation
// NAI-161-D-POPHELD-STUB.
func handlePOpHeld(s *ScriptState) error {
	if err := requireProtectedActivePlayer(s, "P_OPHELD"); err != nil {
		return err
	}
	return fmt.Errorf("P_OPHELD: unimplemented")
}
```

- [ ] **Step 6.5: Register the handler**

Edit `pkg/script/handlers.go`. Append 1 line after T5's entry:

```go
	// NAI-161 T6: P_OPHELD (TS-faithful unimplemented stub).
	OpPOpHeld: handlePOpHeld,
```

- [ ] **Step 6.6: Run tests to verify they pass**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run 'TestPOpHeld' -v
```

Expected: PASS — both subtests green.

- [ ] **Step 6.7: Run full pkg/script test suite**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/...
```

Expected: PASS — no regressions.

- [ ] **Step 6.8: Run full repository test suite**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
```

Expected: PASS.

- [ ] **Step 6.9: Re-run the missing-handler audit to confirm 21→18**

Run:
```bash
mkdir -p /tmp/claude && \
awk '/^var handlers = map\[Opcode\]/,/^}/' pkg/script/handlers.go | grep -oE 'Op[A-Za-z]+' | sort -u > /tmp/claude/handled.txt && \
awk '/^const \(/,/^\)/' pkg/script/opcode.go | grep -oE 'Op[A-Za-z]+\b' | sort -u > /tmp/claude/declared.txt && \
echo "=== count ===" && comm -23 /tmp/claude/declared.txt /tmp/claude/handled.txt | wc -l && \
echo "=== full list ===" && comm -23 /tmp/claude/declared.txt /tmp/claude/handled.txt
```

Expected count: `18`. Expected list: the 21-op list minus `OpClearQueue`, `OpGetQueue`, `OpPOpHeld`.

- [ ] **Step 6.10: Commit**

```bash
git add pkg/script/handlers_player.go pkg/script/handlers.go pkg/script/handlers_player_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-161 T6 — P_OPHELD handler (opcode 2076)

TS-faithful 'unimplemented' error stub behind the
ProtectedActivePlayer gate. Mirrors TS PlayerOps.ts:381-383
which literally throws new Error('unimplemented'). Stub remains
until OPHELD trigger plumbing is ported (separate cohort with
OcOp/LcOp/OcIop).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Post-implementation handoff

After Task 6 commits, the cohort is ready for:

1. **Single combined Sonnet `superpowers:code-reviewer`** at end-of-impl per `superpowers_code_reviewer_model.md`. NOT per-task reviews. Run against the 6-commit T1..T6 range.
2. **Smoke handoff to user** per `smoke_test_server_handoff.md` — target: Tutorial Island bury-bone step (`Content/scripts/tutorial/scripts/skills/tut_bury_bone.rs2`) for the GETQUEUE-binding signal. Fallback: NPC shop interaction (tea_seller / spice_seller / silver_merchant).
3. **Close commit** with `Closes memory:` trailer per `close_commit_memory_trailer.md`, citing the §9 memory list from the spec.

---

## Self-review (plan-author)

**Spec coverage check** (against `docs/superpowers/specs/2026-05-11-nai-161-trivial-handler-sweep-3-design.md`):

- §1 Cohort (3 ops) — T4, T5, T6 ✓
- §2.1 New (*Player) methods — T1, T2 ✓
- §2.2 ActivePlayer interface widening — T3 ✓
- §2.3 Handlers — T4, T5, T6 ✓
- §2.4 Handler registration — embedded in T4/T5/T6 ✓
- §2.5 Mock recorder additions — T3 ✓
- §3 Deviations — pinned by tests: T1 deviation `NAI-161-D-QUEUE-TYPE-MAPPING` by `TestUnlinkQueuedScriptWalksAllNonEngineTypes` + T2 `TestQueueCountExcludesWeak`; `NAI-161-D-CLEARQUEUE-NIL-PROVIDER` by `TestUnlinkQueuedScriptNilServerIsNoop` + `TestQueueCountNilServerReturnsZero`; `NAI-161-D-CLEARQUEUE-RESOLVE-FIRST` by `TestUnlinkQueuedScriptUnknownIDIsNoop`; `NAI-161-D-POPHELD-STUB` by `TestPOpHeldUnimplemented` ✓
- §4 Risks — R1 (pointer-stability) by T1.1 grep ✓; R2 (requireProtectedActivePlayer signature) by T6.1 grep ✓; R3 (queue[:0] aliasing) by mirroring `clearWeakQueue` ✓; R4 (mockPlayer interface gap) by T3.4 test compile ✓; R5 (P_OPHELD gate ordering) by `TestPOpHeldRequiresProtected` ✓; R6 (ActivePlayer impl enumeration) by T3.1 grep ✓; R7 (slice aliasing in tests) — tests read `p.queue` post-mutation, no pre-snapshot ✓
- §5 Test strategy — every listed test is present in T1, T2, T4, T5, T6 ✓
- §6 Smoke binding — captured in post-implementation handoff ✓
- §10 No-deviations audit — captured by per-test TS line citations ✓

**Placeholder scan:** No "TBD", "TODO", "implement later", or "similar to Task N" patterns present. Every code block is complete.

**Type-consistency check:** `unlinkScriptCalls []int`, `queueCountByScript map[int]int`, `UnlinkQueuedScript(scriptID int)`, `QueueCount(scriptID int) int`, `OpClearQueue`, `OpGetQueue`, `OpPOpHeld` — same names used in every reference across T1-T6 ✓
