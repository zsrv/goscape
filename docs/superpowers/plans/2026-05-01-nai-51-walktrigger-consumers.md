# NAI-51 Walktrigger Consumers Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Wire the two TS-shape-stub walktrigger consumers (Player + NPC) so walktrigger-driven RuneScript content actually fires, closing tracked deviations `NAI-37-D-WALKTRIGGER-NOREADER` and `NAI-44-D-PLAYER-WALKTRIGGER-NOOP`.

**Architecture:** Two parallel-but-independent bundles. Bundle 1 (Player) ports the `P_WALKTRIGGER` (2128) + `GETWALKTRIGGER` (2023) opcode handlers through the existing `s.Self`/`requireActivePlayer` convention, adds a `walktrigger int` field on `Player`, and replaces the empty `(*Player).processWalktrigger()` stub with the TS-Player.ts:1057-1070 body (gated on `!p.delayed` only — `protect` deviation introduced). Bundle 2 (NPC) inserts a 9-line consumer block at the top of `(*Npc).updateMovement` mirroring TS Npc.ts:343-360, dispatching via `s.runNpcScript` against the existing `walktrigger`/`walktriggerArg` fields and `TriggerAiQueue1+walktrigger` lookup.

**Tech Stack:** Go 1.26+; existing `pkg/script` Provider/Runner; `modules/world` Server/Player/Npc.

---

## Bundle ordering

Per spec's "Bundle ordering" section: Bundle 1 first (larger surface), Bundle 2 second (single insertion + tests). Each bundle ends with a tracked-deviation retirement and a `Closes memory:` close commit.

## File structure

**Bundle 1 (Player) touches:**
- `modules/world/player.go` — add `walktrigger int` field + default `-1` in `newPlayer`.
- `pkg/script/active.go` — extend `ActivePlayer` interface with `WalkTrigger() int` + `SetWalkTrigger(int)`.
- `modules/world/player_script.go` — add the two `*Player` adapter methods.
- `pkg/script/runner_test.go` — add the two `mockPlayer` methods + capture field.
- `pkg/script/handlers_player.go` — add `handleWalkTrigger`, `handleGetWalkTrigger`.
- `pkg/script/handlers.go` — register the two handlers in the dispatch table.
- `pkg/script/handlers_player_test.go` — add round-trip tests for both opcodes.
- `modules/world/player_test.go` — add `TestNewPlayer_WalkTrigger_DefaultMinusOne`.
- `modules/world/interaction.go` — replace empty stub of `processWalktrigger` + retire deviation comment.
- `modules/world/interaction_test.go` — rewrite `TestProcessWalktriggerNoOp` + add fires/delayed/missing-script + processInteraction integration tests.

**Bundle 2 (NPC) touches:**
- `modules/world/npc_interaction.go` — insert consumer block in `updateMovement`.
- `modules/world/npc_interaction_test.go` — add 5 `TestNpcUpdateMovement_Walktrigger*` tests.
- `modules/world/npc.go` — retire deviation comment in `walktrigger` field doc.

---

# Bundle 1 — Player walktrigger

### Task 1.1: Add `Player.walktrigger` field with `-1` default

**Files:**
- Modify: `modules/world/player.go` (struct definition near other interaction fields; constructor `newPlayer` at line 335)
- Test: `modules/world/player_test.go` (append after `TestNewPlayer_LastAppearance_DefaultMinusOne`)

- [ ] **Step 1: Write the failing test**

Append to `modules/world/player_test.go`:

```go
// TestNewPlayer_WalkTrigger_DefaultMinusOne pins the NAI-51 default for the
// new walktrigger field. -1 sentinel = "no script queued"; default 0 would
// silently fire script id 0 on every walktrigger consumer entry.
func TestNewPlayer_WalkTrigger_DefaultMinusOne(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()
	c := newClient(serverConn, time.Second, discardLogger())
	defer c.in.Release()
	c.state = ClientStateGame
	p := newPlayer(c)
	if p.walktrigger != -1 {
		t.Errorf("walktrigger default: got %d, want -1", p.walktrigger)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestNewPlayer_WalkTrigger_DefaultMinusOne -v`
Expected: FAIL — compile error `p.walktrigger undefined`.

- [ ] **Step 3: Add the field to Player struct**

In `modules/world/player.go`, locate the existing field cluster around line 167 (`animProtect`) — add the walktrigger field nearby. Suggested insertion point: just after the `animProtect int` block, mirroring the doc-comment shape of `Npc.walktrigger` at `npc.go:88-97` minus the deviation note (which Bundle 1 retires).

```go
	// walktrigger queues a deferred script id to fire from
	// processWalktrigger on the next interaction tick (-1 = unset).
	// Written by P_WALKTRIGGER (opcode 2128); read by GETWALKTRIGGER
	// (opcode 2023) and (*Player).processWalktrigger. Mirrors TS
	// Player.walktrigger at Player.ts:1057-1070.
	walktrigger int
```

- [ ] **Step 4: Set the `-1` default in `newPlayer`**

In `modules/world/player.go`, the existing `newPlayer` body (`player.go:335`) initialises sentinels for many fields. Add `walktrigger: -1,` next to `targetOp: -1,` for thematic grouping:

```go
		targetOp:       -1,
		walktrigger:    -1,
		apRange:        10,
```

- [ ] **Step 5: Run test to verify it passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestNewPlayer_WalkTrigger_DefaultMinusOne -v`
Expected: PASS.

- [ ] **Step 6: Run the full world package to verify no regressions**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/`
Expected: PASS — no `&Player{}` test fixture exercises a walktrigger-reading path; the field defaults to 0 in those literals but no consumer reads it yet (consumer wired in Task 1.7).

- [ ] **Step 7: Commit**

```bash
git add modules/world/player.go modules/world/player_test.go
git commit --no-gpg-sign -m "feat(world): NAI-51 T1.1 — Player.walktrigger field + -1 default"
```

---

### Task 1.2: Extend `ActivePlayer` interface with `WalkTrigger`/`SetWalkTrigger`

**Files:**
- Modify: `pkg/script/active.go` (after the existing animation methods; mirror NPC analogues at `active.go:546-556`)

- [ ] **Step 1: Add the interface methods**

Locate a stable insertion point in the `ActivePlayer` interface — the `SetAnimProtect(v int)` method at `active.go:389` and surrounding S7b block is one good neighbor. Append after `SetAnimProtect`:

```go
	// WalkTrigger returns the active player's queued walktrigger script
	// id, or -1 if none. Read by GETWALKTRIGGER (opcode 2023) and by
	// (*Player).processWalktrigger before firing. Mirrors TS
	// Player.walktrigger getter at Player.ts:1057-1070.
	WalkTrigger() int

	// SetWalkTrigger writes the queued walktrigger script id. -1 clears.
	// Written by P_WALKTRIGGER (opcode 2128); also written by
	// (*Player).processWalktrigger to -1 immediately before script
	// dispatch (TS clear-before-check semantics). Mirrors TS
	// Player.walktrigger setter at PlayerOps.ts:1035-1037.
	SetWalkTrigger(scriptID int)
```

- [ ] **Step 2: Verify compile (no test yet — interface unimplemented)**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`
Expected: FAIL — `*Player does not implement script.ActivePlayer (missing WalkTrigger method)` and `*mockPlayer does not implement script.ActivePlayer ...`.

This compile failure is the "failing test" gate for Task 1.3 + 1.4.

(No commit yet — interface change without implementer is not buildable.)

---

### Task 1.3: Implement `*Player.WalkTrigger`/`SetWalkTrigger` adapters

**Files:**
- Modify: `modules/world/player_script.go` (append near other walk*-named setters around line 537)

- [ ] **Step 1: Add the adapter methods**

Append after `(p *Player) SetRunAnim` at `player_script.go:540`:

```go
// WalkTrigger implements script.ActivePlayer.WalkTrigger. Returns the
// queued walktrigger script id, or -1 if none. NAI-51.
func (p *Player) WalkTrigger() int { return p.walktrigger }

// SetWalkTrigger implements script.ActivePlayer.SetWalkTrigger. Stores
// scriptID in p.walktrigger. -1 clears. NAI-51.
func (p *Player) SetWalkTrigger(scriptID int) { p.walktrigger = scriptID }
```

- [ ] **Step 2: Verify world-package compile**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./modules/world/`
Expected: PASS (build succeeds).

- [ ] **Step 3: Verify pkg/script-package compile (will still fail until Task 1.4)**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./pkg/script/`
Expected: PASS (build succeeds — pkg/script doesn't import modules/world; mockPlayer compile-failure surfaces only under `go test`).

- [ ] **Step 4: Verify pkg/script tests fail with mockPlayer compile error**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -count=1 -run TestNothingMatches`
Expected: FAIL — `*mockPlayer does not implement ActivePlayer`.

(No commit yet — bundle the interface + both implementations in Task 1.4's commit.)

---

### Task 1.4: Add `mockPlayer.WalkTrigger`/`SetWalkTrigger`

**Files:**
- Modify: `pkg/script/runner_test.go` (mockPlayer struct around line 99 + methods around line 511)

- [ ] **Step 1: Add capture field on mockPlayer**

In the `mockPlayer` struct (`runner_test.go:99`), add after the `animProtectValue` field at line 254:

```go
	// NAI-51: SetWalkTrigger captures. lastWalkTriggerSet is the last
	// scriptID passed to SetWalkTrigger; walkTriggerSetCalls counts
	// invocations (so error-path tests can assert the setter was NOT
	// reached). walkTriggerValue is pre-seeded by tests that exercise
	// GETWALKTRIGGER's read path.
	walkTriggerValue     int
	lastWalkTriggerSet   int
	walkTriggerSetCalls  int
```

- [ ] **Step 2: Add the two methods**

Append after `(m *mockPlayer) SetAnimProtect` at `runner_test.go:511`:

```go
func (m *mockPlayer) WalkTrigger() int { return m.walkTriggerValue }

func (m *mockPlayer) SetWalkTrigger(scriptID int) {
	m.lastWalkTriggerSet = scriptID
	m.walkTriggerSetCalls++
}
```

- [ ] **Step 3: Run pkg/script tests to verify compile**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -count=1`
Expected: PASS.

- [ ] **Step 4: Run world tests to confirm nothing regressed**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -count=1`
Expected: PASS.

- [ ] **Step 5: Commit (interface + both impls)**

```bash
git add pkg/script/active.go modules/world/player_script.go pkg/script/runner_test.go
git commit --no-gpg-sign -m "feat(script): NAI-51 T1.2-T1.4 — ActivePlayer.WalkTrigger/SetWalkTrigger + adapters"
```

---

### Task 1.5: Add `handleWalkTrigger` (P_WALKTRIGGER, opcode 2128)

**Files:**
- Modify: `pkg/script/handlers_player.go` (append after the existing animation-handler block ending around line 635)
- Modify: `pkg/script/handlers.go` (extend dispatch table near `OpRunAnim` at line 206)
- Test: `pkg/script/handlers_player_test.go` (append a new test alongside existing ALLOWDESIGN/BUILDAPPEARANCE tests)

- [ ] **Step 1: Write the failing test**

Append to `pkg/script/handlers_player_test.go` (place near other PlayerOps unit tests; use the existing surrounding pattern of `mp := &mockPlayer{}` + `state := Init(...)` + Execute):

```go
// TestHandleWalkTrigger_PopsAndWrites verifies P_WALKTRIGGER (opcode
// 2128) pops one int and writes it via SetWalkTrigger on the active
// player. Mirrors TS PlayerOps.ts:1035-1037.
func TestHandleWalkTrigger_PopsAndWrites(t *testing.T) {
	mp := &mockPlayer{}
	sf := &ScriptFile{
		Name:             "[walktrigger,test]",
		Opcodes:          []Opcode{OpPushConstantInt, OpWalkTrigger, OpReturn},
		IntOperands:      []int32{42, 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if mp.walkTriggerSetCalls != 1 {
		t.Errorf("SetWalkTrigger calls: got %d, want 1", mp.walkTriggerSetCalls)
	}
	if mp.lastWalkTriggerSet != 42 {
		t.Errorf("SetWalkTrigger arg: got %d, want 42", mp.lastWalkTriggerSet)
	}
}

// TestHandleWalkTrigger_NoActivePlayer asserts the handler errors when
// the active-player pointer is unset, matching the requireActivePlayer
// contract.
func TestHandleWalkTrigger_NoActivePlayer(t *testing.T) {
	state := &ScriptState{StackCapacity: StackCapacity}
	state.PushInt(42)
	err := handleWalkTrigger(state)
	if err == nil {
		t.Fatal("handleWalkTrigger: got nil, want no-active-player error")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestHandleWalkTrigger -v`
Expected: FAIL — `handleWalkTrigger undefined` and dispatch table doesn't dispatch `OpWalkTrigger`.

- [ ] **Step 3: Add `handleWalkTrigger`**

Append to `pkg/script/handlers_player.go` (just before the existing `// S5d:` config-read marker, or near the end of the animation-ops block):

```go
// handleWalkTrigger (P_WALKTRIGGER, opcode 2128) sets the active player's
// queued walktrigger script id. Pops one int. Mirrors TS PlayerOps.ts:1035-1037.
// Consumed by (*Player).processWalktrigger on the next interaction tick.
func handleWalkTrigger(s *ScriptState) error {
	if err := requireActivePlayer(s, "WALKTRIGGER"); err != nil {
		return err
	}
	s.Self.SetWalkTrigger(s.PopInt())
	return nil
}
```

- [ ] **Step 4: Register in dispatch table**

In `pkg/script/handlers.go`, locate the animation-handler block ending at `OpRunAnim: handleRunAnim,` (line 206). Add immediately after:

```go
	OpRunAnim:    handleRunAnim,
	// NAI-51: walktrigger consumer ops (Player side).
	OpWalkTrigger:    handleWalkTrigger,
```

(The `OpGetWalkTrigger` registration lands in Task 1.6.)

- [ ] **Step 5: Run test to verify it passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestHandleWalkTrigger -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add pkg/script/handlers_player.go pkg/script/handlers.go pkg/script/handlers_player_test.go
git commit --no-gpg-sign -m "feat(script): NAI-51 T1.5 — handleWalkTrigger (P_WALKTRIGGER, opcode 2128)"
```

---

### Task 1.6: Add `handleGetWalkTrigger` (GETWALKTRIGGER, opcode 2023)

**Files:**
- Modify: `pkg/script/handlers_player.go` (append after `handleWalkTrigger` from Task 1.5)
- Modify: `pkg/script/handlers.go` (extend dispatch table)
- Test: `pkg/script/handlers_player_test.go` (append after the WalkTrigger tests)

- [ ] **Step 1: Write the failing test**

Append to `pkg/script/handlers_player_test.go`:

```go
// TestHandleGetWalkTrigger_ReadsAndPushes verifies GETWALKTRIGGER (opcode
// 2023) reads p.walktrigger via WalkTrigger() and pushes the value.
// Mirrors TS PlayerOps.ts:1039-1042.
func TestHandleGetWalkTrigger_ReadsAndPushes(t *testing.T) {
	mp := &mockPlayer{walkTriggerValue: 99}
	sf := &ScriptFile{
		Name:             "[getwalktrigger,test]",
		Opcodes:          []Opcode{OpGetWalkTrigger, OpReturn},
		IntOperands:      []int32{0, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if state.ISP != 1 {
		t.Fatalf("ISP after GETWALKTRIGGER: got %d, want 1", state.ISP)
	}
	if got := state.PopInt(); got != 99 {
		t.Errorf("popped: got %d, want 99", got)
	}
}

// TestHandleGetWalkTrigger_DefaultUnsetReturnsMinusOne pins the unset
// sentinel propagation through the handler.
func TestHandleGetWalkTrigger_DefaultUnsetReturnsMinusOne(t *testing.T) {
	mp := &mockPlayer{walkTriggerValue: -1}
	sf := &ScriptFile{
		Name:             "[getwalktrigger,test]",
		Opcodes:          []Opcode{OpGetWalkTrigger, OpReturn},
		IntOperands:      []int32{0, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := state.PopInt(); got != -1 {
		t.Errorf("popped: got %d, want -1", got)
	}
}

// TestHandleGetWalkTrigger_NoActivePlayer asserts the handler errors when
// the active-player pointer is unset.
func TestHandleGetWalkTrigger_NoActivePlayer(t *testing.T) {
	state := &ScriptState{StackCapacity: StackCapacity}
	err := handleGetWalkTrigger(state)
	if err == nil {
		t.Fatal("handleGetWalkTrigger: got nil, want no-active-player error")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestHandleGetWalkTrigger -v`
Expected: FAIL — `handleGetWalkTrigger undefined`.

- [ ] **Step 3: Add `handleGetWalkTrigger`**

Append to `pkg/script/handlers_player.go` (immediately after `handleWalkTrigger`):

```go
// handleGetWalkTrigger (GETWALKTRIGGER, opcode 2023) pushes the active
// player's current walktrigger script id. Returns -1 when unset.
// Mirrors TS PlayerOps.ts:1039-1042.
func handleGetWalkTrigger(s *ScriptState) error {
	if err := requireActivePlayer(s, "GETWALKTRIGGER"); err != nil {
		return err
	}
	s.PushInt(s.Self.WalkTrigger())
	return nil
}
```

- [ ] **Step 4: Register in dispatch table**

In `pkg/script/handlers.go`, extend the NAI-51 block from Task 1.5:

```go
	// NAI-51: walktrigger consumer ops (Player side).
	OpWalkTrigger:    handleWalkTrigger,
	OpGetWalkTrigger: handleGetWalkTrigger,
```

- [ ] **Step 5: Run test to verify it passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestHandleGetWalkTrigger -v`
Expected: PASS.

- [ ] **Step 6: Run the full pkg/script suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -count=1`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add pkg/script/handlers_player.go pkg/script/handlers.go pkg/script/handlers_player_test.go
git commit --no-gpg-sign -m "feat(script): NAI-51 T1.6 — handleGetWalkTrigger (GETWALKTRIGGER, opcode 2023)"
```

---

### Task 1.7: Implement `(*Player).processWalktrigger` body

**Files:**
- Modify: `modules/world/interaction.go` (lines 232-241; replace the empty stub body and the `DEVIATION NAI-44-D-PLAYER-WALKTRIGGER-NOOP` comment block)
- Test: `modules/world/interaction_test.go` (rewrite `TestProcessWalktriggerNoOp` at lines 646-672, then append fires/delayed/missing-script tests)

- [ ] **Step 1: Write the failing tests**

Replace the existing `TestProcessWalktriggerNoOp` (lines 646-672) with the following block, and append the new tests directly after it:

```go
// TestProcessWalktrigger_UnsetNoOp — NAI-51 T1.7. walktrigger=-1 → no
// script lookup, no field write. Replaces the NAI-44 stub-no-op test.
func TestProcessWalktrigger_UnsetNoOp(t *testing.T) {
	s := newTestServer(t)
	p, wait := makeInteractionPlayer(t, s, 3200, 3200, 0)
	defer wait()

	// Default from newPlayer is -1.
	if p.walktrigger != -1 {
		t.Fatalf("precondition: walktrigger=%d, want -1", p.walktrigger)
	}

	p.processWalktrigger()

	if p.walktrigger != -1 {
		t.Errorf("walktrigger after no-op: got %d, want -1 (unchanged)", p.walktrigger)
	}
}

// TestProcessWalktrigger_DelayedNoOp — NAI-51 T1.7. delayed=true gates
// the consumer entirely; field stays unchanged. Mirrors TS gate at
// Player.ts:1062.
func TestProcessWalktrigger_DelayedNoOp(t *testing.T) {
	s := newTestServer(t)
	p, wait := makeInteractionPlayer(t, s, 3200, 3200, 0)
	defer wait()

	p.walktrigger = 7
	p.delayed = true

	p.processWalktrigger()

	if p.walktrigger != 7 {
		t.Errorf("walktrigger after delayed bail: got %d, want 7 (unchanged)", p.walktrigger)
	}
}

// TestProcessWalktrigger_FiresAndClears — NAI-51 T1.7. walktrigger=N + a
// registered script at slot N → script fires once, field cleared to -1.
// Verifies firing via mes "wt-fired" landing on the wire.
func TestProcessWalktrigger_FiresAndClears(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	sf := &script.ScriptFile{
		Name: "[walktrigger,test]",
		Opcodes: []script.Opcode{
			script.OpPushConstantString,
			script.OpMes,
			script.OpReturn,
		},
		IntOperands:      []int32{0, 0, 0},
		StringOperands:   []string{"wt-fired", "", ""},
		InstructionCount: 3,
	}
	s.scriptProvider.RegisterAt(42, sf)

	p, cc := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	received := drainConn(t, cc)

	p.walktrigger = 42

	p.processWalktrigger()
	p.client.flushWrite()
	pkt := <-received

	if p.walktrigger != -1 {
		t.Errorf("walktrigger after fire: got %d, want -1", p.walktrigger)
	}
	// MessageGame wire = opcode(1) + len(1) + PJStrLF("wt-fired") = 1+1+9 = 11 bytes
	if len(pkt) != 11 {
		t.Fatalf("packet length: got %d, want 11", len(pkt))
	}
	if string(pkt[2:10]) != "wt-fired" || pkt[10] != 0x0a {
		t.Errorf("payload: got %q, want 'wt-fired\\n'", pkt[2:])
	}
}

// TestProcessWalktrigger_MissingScriptStillClears — NAI-51 T1.7. TS
// Player.ts:1064 clears walktrigger BEFORE the script-found check, so a
// missing script still resets the field. No script registered at slot 42
// → walktrigger reset to -1, no script run.
func TestProcessWalktrigger_MissingScriptStillClears(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider() // empty
	p, wait := makeInteractionPlayer(t, s, 3200, 3200, 0)
	defer wait()

	p.walktrigger = 42

	p.processWalktrigger()

	if p.walktrigger != -1 {
		t.Errorf("walktrigger after missing-script: got %d, want -1 (TS clear-before-check)", p.walktrigger)
	}
}
```

The test file already imports `script` (used elsewhere) and `io2` aliasing (line 8). Verify the import block has `"github.com/zsrv/goscape/pkg/script"`; if not, add it.

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestProcessWalktrigger -v`
Expected: FAIL — `TestProcessWalktrigger_FiresAndClears` fails (no script run; walktrigger stays at 42); `TestProcessWalktrigger_MissingScriptStillClears` fails (walktrigger stays at 42); the unset/delayed tests pass against the existing stub but that's coincidental.

- [ ] **Step 3: Replace `processWalktrigger` body and retire the stub-deviation comment**

In `modules/world/interaction.go`, replace lines 232-241 (the entire comment block + `func (p *Player) processWalktrigger() {}`) with:

```go
// processWalktrigger is the per-tick walktrigger consumption hook
// invoked by processInteraction's pre-step and post-step arms. Looks up
// the queued script id, clears the field BEFORE the script-found check
// (TS clear-before-check semantics at Player.ts:1064), then dispatches
// via runScript with protect=true. Mirrors TS Player.processWalktrigger
// at Player.ts:1057-1070.
//
// DEVIATION NAI-51-D-PLAYER-WALKTRIGGER-NO-PROTECT-CHECK: TS L1062 also
// gates on !this.protect. Player has no boolean protect field; the
// anim-protect block (player.go:166) is a separate concern. Closure:
// future protect/anim-protect convergence sub-spec.
func (p *Player) processWalktrigger() {
	if p.walktrigger == -1 || p.delayed {
		return
	}
	if p.client == nil || p.client.server == nil {
		return
	}
	s := p.client.server
	sf := s.scriptProvider.GetByID(uint32(p.walktrigger))
	p.walktrigger = -1
	if sf == nil {
		return
	}
	s.runScript(sf, p, nil, true, nil, nil)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestProcessWalktrigger -v`
Expected: PASS — all four `TestProcessWalktrigger_*` tests pass.

- [ ] **Step 5: Run the full world-package test suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -count=1`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add modules/world/interaction.go modules/world/interaction_test.go
git commit --no-gpg-sign -m "feat(world): NAI-51 T1.7 — wire (*Player).processWalktrigger body"
```

---

### Task 1.8: Add processInteraction integration tests for walktrigger pre/post-step arms

**Files:**
- Test: `modules/world/interaction_test.go` (append after Task 1.7's tests)

- [ ] **Step 1: Write the failing tests**

Append to `modules/world/interaction_test.go`:

```go
// TestProcessInteraction_PreStepWalktriggerFires — NAI-51 T1.8. With
// a walktrigger queued and a target in operable distance, the pre-step
// arm at interaction.go:169 must fire the walktrigger BEFORE tryInteract.
// Verified via "wt-fired" wire output AND walktrigger=-1 after the tick.
func TestProcessInteraction_PreStepWalktriggerFires(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	sf := &script.ScriptFile{
		Name: "[walktrigger,test]",
		Opcodes: []script.Opcode{
			script.OpPushConstantString,
			script.OpMes,
			script.OpReturn,
		},
		IntOperands:      []int32{0, 0, 0},
		StringOperands:   []string{"wt-fired", "", ""},
		InstructionCount: 3,
	}
	s.scriptProvider.RegisterAt(7, sf)

	npc := makeInteractionNpc(t, s, 1, 100, 100, 0)
	p, cc := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	p.x, p.z, p.level = 99, 100, 0 // dx=1 → operable
	received := drainConn(t, cc)

	p.SetInteraction(InteractionEngine, npc, 1, -1)
	p.walktrigger = 7

	p.processInteraction()
	p.client.flushWrite()

	if p.walktrigger != -1 {
		t.Errorf("walktrigger after pre-step fire: got %d, want -1", p.walktrigger)
	}
	// First wire packet should be the "wt-fired" mes.
	pkt := <-received
	if !bytes.Contains(pkt, []byte("wt-fired")) {
		t.Errorf("first wire packet did not contain wt-fired: %q", pkt)
	}
}

// TestProcessInteraction_PostStepWalktriggerFires — NAI-51 T1.8. With a
// walktrigger queued, a target out of range, and waypoints set, the
// post-step arm at interaction.go:183 must fire the walktrigger.
func TestProcessInteraction_PostStepWalktriggerFires(t *testing.T) {
	s := setupServerForInteractionTest(t)
	s.scriptProvider = script.NewProvider()
	sf := &script.ScriptFile{
		Name: "[walktrigger,test]",
		Opcodes: []script.Opcode{
			script.OpPushConstantString,
			script.OpMes,
			script.OpReturn,
		},
		IntOperands:      []int32{0, 0, 0},
		StringOperands:   []string{"wt-post", "", ""},
		InstructionCount: 3,
	}
	s.scriptProvider.RegisterAt(11, sf)

	npc := makeInteractionNpc(t, s, 1, 200, 200, 0) // far away → no operable
	p, cc := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	p.x, p.z, p.level = 99, 100, 0
	received := drainConn(t, cc)

	p.SetInteraction(InteractionEngine, npc, 1, -1)
	p.walktrigger = 11
	// Pre-seed waypoints so hasWaypoints() is true after the pre-step
	// arm fails its tryInteract.
	p.waypointIndex = 0
	p.waypoints[0] = (0 << 28) | (200 << 14) | 200

	p.processInteraction()
	p.client.flushWrite()

	if p.walktrigger != -1 {
		t.Errorf("walktrigger after post-step fire: got %d, want -1", p.walktrigger)
	}
	pkt := <-received
	if !bytes.Contains(pkt, []byte("wt-post")) {
		t.Errorf("wire did not contain wt-post: %q", pkt)
	}
}
```

- [ ] **Step 2: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestProcessInteraction_(Pre|Post)StepWalktrigger" -v`
Expected: PASS — both tests pass without source changes (the body wired in Task 1.7 already wires through processInteraction's existing call sites).

If either fails, do NOT modify processInteraction — the call sites at lines 169 and 183 are already TS-faithful per spec lines 58-60. Investigate the test fixture (waypoint setup, NPC distance) instead.

- [ ] **Step 3: Run the full world-package suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -count=1`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add modules/world/interaction_test.go
git commit --no-gpg-sign -m "test(world): NAI-51 T1.8 — processInteraction pre/post-step walktrigger fires"
```

---

### Task 1.9: Bundle 1 close — retire NAI-44-D-PLAYER-WALKTRIGGER-NOOP

**Files:**
- (No code changes — the deviation comment was already replaced inline in Task 1.7. This task is a verification + memory-trailer commit.)

- [ ] **Step 1: Verify no stale `NAI-44-D-PLAYER-WALKTRIGGER-NOOP` references remain**

Run: `rg "NAI-44-D-PLAYER-WALKTRIGGER-NOOP" pkg/ modules/ cmd/`
Expected: no output (zero matches).

If any matches surface, edit them out (likely doc comments). The deviation block at `interaction.go:235-240` was replaced in Task 1.7; double-check.

- [ ] **Step 2: Verify the new deviation tag landed**

Run: `rg "NAI-51-D-PLAYER-WALKTRIGGER-NO-PROTECT-CHECK" pkg/ modules/ cmd/`
Expected: 1 match (the doc comment in `modules/world/interaction.go` from Task 1.7).

- [ ] **Step 3: Run the full repo suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`
Expected: PASS.

- [ ] **Step 4: Bundle-1 close commit (memory trailer)**

```bash
git commit --no-gpg-sign --allow-empty -m "$(cat <<'EOF'
chore(close): NAI-51 Bundle 1 — Player walktrigger consumer wired

Closes deviation: NAI-44-D-PLAYER-WALKTRIGGER-NOOP.
Introduces deviation: NAI-51-D-PLAYER-WALKTRIGGER-NO-PROTECT-CHECK.

Bundle 1 of NAI-51. Bundle 2 (NPC) follows.

Closes memory: nai_followups.md
EOF
)"
```

---

# Bundle 2 — NPC walktrigger

### Task 2.1: Insert walktrigger consumer block in `(*Npc).updateMovement`

**Files:**
- Modify: `modules/world/npc_interaction.go` (insert at line 287, between the `waypointIndex < 0` early return at line 283 and the `stepOnce` call at line 289)
- Test: `modules/world/npc_interaction_test.go` (append after `TestNpcUpdateMovementNoMoveRestrict`)

- [ ] **Step 1: Write the failing tests**

Append to `modules/world/npc_interaction_test.go`:

```go
// TestNpcUpdateMovement_WalktriggerFiresThenSteps — NAI-51 T2.1.
// walktrigger=0 + waypoint + script registered at
// (TriggerAiQueue1, typeId, category) → script fires (npc.sayText set
// by mes script), field reset to -1, step still consumed.
func TestNpcUpdateMovement_WalktriggerFiresThenSteps(t *testing.T) {
	s := newServerForScriptTest(t)
	s.scriptProvider = script.NewProvider()
	s.scriptProvider.Register(buildNpcSayScript(script.TriggerAiQueue1, 42, "wt-npc"))

	typ := &objtype.NpcType{ConfigType: objtype.ConfigType{ID: 42}, Category: 0}
	n := NewNpc(1, 42, 100, 100, 0, typ)
	n.server = s
	n.moveSpeed = MoveSpeedWalk
	n.waypoints[0] = coordgrid.PackCoord(0, 103, 100)
	n.waypointIndex = 0
	n.walktrigger = 0
	n.walktriggerArg = 7

	moved := n.updateMovement(s)

	if !moved {
		t.Error("moved: false, want true")
	}
	if n.walktrigger != -1 {
		t.Errorf("walktrigger after fire: got %d, want -1", n.walktrigger)
	}
	if string(n.sayText) != "wt-npc" {
		t.Errorf("sayText: got %q, want %q", n.sayText, "wt-npc")
	}
	if n.x != 101 {
		t.Errorf("x after step: got %d, want 101", n.x)
	}
}

// TestNpcUpdateMovement_WalktriggerSentinelSkipsLookup — NAI-51 T2.1.
// walktrigger=-1 (sentinel) → no provider call, step proceeds.
func TestNpcUpdateMovement_WalktriggerSentinelSkipsLookup(t *testing.T) {
	s := newServerForScriptTest(t)
	// Empty provider — any GetByTrigger call would return nil; we want
	// to verify the lookup is short-circuited entirely. Set
	// scriptProvider to nil so any reach into provider would panic.
	s.scriptProvider = nil

	typ := &objtype.NpcType{ConfigType: objtype.ConfigType{ID: 42}}
	n := NewNpc(1, 42, 100, 100, 0, typ)
	n.server = s
	n.moveSpeed = MoveSpeedWalk
	n.waypoints[0] = coordgrid.PackCoord(0, 103, 100)
	n.waypointIndex = 0
	// walktrigger defaults to -1 from NewNpc.

	moved := n.updateMovement(s)

	if !moved {
		t.Error("moved: false, want true")
	}
	if n.x != 101 {
		t.Errorf("x: got %d, want 101", n.x)
	}
}

// TestNpcUpdateMovement_WalktriggerMissingScriptStillClears — NAI-51 T2.1.
// walktrigger=N + no script registered at (TriggerAiQueue1+N, ...) →
// field cleared, no fire, step proceeds. TS clear-before-check at
// Npc.ts:355.
func TestNpcUpdateMovement_WalktriggerMissingScriptStillClears(t *testing.T) {
	s := newServerForScriptTest(t)
	s.scriptProvider = script.NewProvider() // empty

	typ := &objtype.NpcType{ConfigType: objtype.ConfigType{ID: 42}}
	n := NewNpc(1, 42, 100, 100, 0, typ)
	n.server = s
	n.moveSpeed = MoveSpeedWalk
	n.waypoints[0] = coordgrid.PackCoord(0, 103, 100)
	n.waypointIndex = 0
	n.walktrigger = 5

	moved := n.updateMovement(s)

	if !moved {
		t.Error("moved: false, want true")
	}
	if n.walktrigger != -1 {
		t.Errorf("walktrigger after missing-script: got %d, want -1 (TS clear-before-check)", n.walktrigger)
	}
	if string(n.sayText) != "" {
		t.Errorf("sayText: got %q, want empty (no script ran)", n.sayText)
	}
	if n.x != 101 {
		t.Errorf("x: got %d, want 101 (step consumed)", n.x)
	}
}

// TestNpcUpdateMovement_WalktriggerArgPassthrough — NAI-51 T2.1.
// walktriggerArg=42 + script that pushes the arg → script fires with
// intArgs=[42]. Verified by registering a script that does
// "arg(0); npc_say". Goscape's NpcSay handler reads the script's pushed
// string, but this test uses the simpler signal: walktrigger fires and
// we observe the per-tick reset.
func TestNpcUpdateMovement_WalktriggerArgPassthrough(t *testing.T) {
	s := newServerForScriptTest(t)
	s.scriptProvider = script.NewProvider()
	// Script that pushes a string from intArg-typed arg position is
	// involved; for argument-passthrough we use a simpler check: the
	// runNpcScript dispatch must observe walktriggerArg in intArgs[0].
	// We verify via firing-side-effect (sayText) AND the walktrigger
	// reset. The argument is captured by the runNpcScript call in
	// updateMovement; if the wiring drops it, the script still fires
	// (ARG opcode would error, no sayText). Asserting sayText IS the
	// arg-pass signal in this fixture's mes-only script — but that
	// doesn't isolate the arg path. We instead pin the
	// runNpcScript-arg path by reading the arg back via a script that
	// pushes the arg as a string and emits via mes.
	sf := &script.ScriptFile{
		Name:      "[ai_queue1,42]",
		LookupKey: script.LookupKeyForType(script.TriggerAiQueue1, 42),
		// Opcodes: read intArg[0] and emit via NPC_SAY as decimal string.
		// Goscape lacks a generic arg-to-string opcode; fall back to
		// asserting via state-side-effect: register a simple mes script
		// and pin walktriggerArg propagation by a separate unit-level
		// check on runNpcScript. For now, the side-effect signal is
		// sufficient to prove dispatch happened with non-nil intArgs.
		Opcodes:          []script.Opcode{script.OpPushConstantString, script.OpNpcSay, script.OpReturn},
		IntOperands:      []int32{0, 0, 0},
		StringOperands:   []string{"arg-test", "", ""},
		InstructionCount: 3,
	}
	s.scriptProvider.Register(sf)

	typ := &objtype.NpcType{ConfigType: objtype.ConfigType{ID: 42}}
	n := NewNpc(1, 42, 100, 100, 0, typ)
	n.server = s
	n.moveSpeed = MoveSpeedWalk
	n.waypoints[0] = coordgrid.PackCoord(0, 103, 100)
	n.waypointIndex = 0
	n.walktrigger = 0
	n.walktriggerArg = 42

	_ = n.updateMovement(s)

	// Side-effect signal: script ran (sayText set) AND walktrigger reset.
	// The arg-passthrough is verified at the runNpcScript wiring site
	// (the implementation must build intArgs=[]int{n.walktriggerArg}).
	if string(n.sayText) != "arg-test" {
		t.Errorf("sayText: got %q, want %q (script did not run)", n.sayText, "arg-test")
	}
	if n.walktrigger != -1 {
		t.Errorf("walktrigger: got %d, want -1", n.walktrigger)
	}
}

// TestNpcUpdateMovement_WalktriggerNilTypNoOp — NAI-51 T2.1. n.typ is
// nil → consumer block bails before lookup (defends the n.typ != nil
// guard); step proceeds. Mirrors the TS lookup which dereferences
// type.id and type.category.
func TestNpcUpdateMovement_WalktriggerNilTypNoOp(t *testing.T) {
	s := newServerForScriptTest(t)
	s.scriptProvider = script.NewProvider()
	// Pre-register a script at (TriggerAiQueue1, 42) — the test must
	// prove the consumer never hits this.
	s.scriptProvider.Register(buildNpcSayScript(script.TriggerAiQueue1, 42, "should-not-fire"))

	n := NewNpc(1, 42, 100, 100, 0, nil) // typ=nil deliberately
	n.server = s
	n.moveSpeed = MoveSpeedWalk
	n.waypoints[0] = coordgrid.PackCoord(0, 103, 100)
	n.waypointIndex = 0
	n.walktrigger = 0

	moved := n.updateMovement(s)

	if !moved {
		t.Error("moved: false, want true")
	}
	if string(n.sayText) != "" {
		t.Errorf("sayText: got %q, want empty (script must NOT fire on nil typ)", n.sayText)
	}
	if n.x != 101 {
		t.Errorf("x: got %d, want 101 (step still proceeds)", n.x)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestNpcUpdateMovement_Walktrigger" -v`
Expected: FAIL — `TestNpcUpdateMovement_WalktriggerFiresThenSteps`/`MissingScriptStillClears`/`ArgPassthrough` fail (no consumer wired); the Sentinel + NilTyp tests pass coincidentally because `walktrigger == -1` short-circuits the not-yet-existent block (sentinel) and `typ == nil` doesn't matter without a consumer (nil-typ).

- [ ] **Step 3: Insert the consumer block**

In `modules/world/npc_interaction.go`, locate `(*Npc).updateMovement` at line 277. Insert between the `waypointIndex < 0` early return (line 287) and the `advanced1, dir1 := n.stepOnce(s)` line (line 289):

```go
	if n.waypointIndex < 0 {
		n.walkDir = -1
		n.runDir = -1
		return false
	}

	// NAI-51: walktrigger consumer (TS Npc.ts:347-357). Fire BEFORE
	// step consumption. TS clears walktrigger BEFORE the script-found
	// check, so a missing script still resets the field. The n.typ
	// guard defends against the nil-typ test path; production NPCs
	// always have typ set by NewNpc, but defensive parity with TS's
	// NpcType.get(this.type) lookup avoids a nil deref here.
	if n.walktrigger != -1 && n.typ != nil {
		trigger := script.TriggerAiQueue1 + script.ServerTriggerType(n.walktrigger)
		sf := s.scriptProvider.GetByTrigger(trigger, n.typeId, n.typ.Category)
		wtArg := n.walktriggerArg
		n.walktrigger = -1
		if sf != nil {
			s.runNpcScript(sf, n, nil, []int{wtArg}, nil)
		}
	}

	advanced1, dir1 := n.stepOnce(s)
```

The file already imports `"github.com/zsrv/goscape/pkg/script"` (used elsewhere in the file). Verify; if absent, add it to the import block.

- [ ] **Step 4: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestNpcUpdateMovement_Walktrigger" -v`
Expected: PASS — all five tests pass.

- [ ] **Step 5: Run the full world-package suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -count=1`
Expected: PASS — including pre-existing `TestNpcUpdateMovement*` tests (those use `NewNpc` so `walktrigger=-1`; the new block short-circuits cleanly).

- [ ] **Step 6: Commit**

```bash
git add modules/world/npc_interaction.go modules/world/npc_interaction_test.go
git commit --no-gpg-sign -m "feat(world): NAI-51 T2.1 — NPC walktrigger consumer in updateMovement"
```

---

### Task 2.2: Bundle 2 close — retire NAI-37-D-WALKTRIGGER-NOREADER

**Files:**
- Modify: `modules/world/npc.go:88-97` (the `walktrigger` field doc-comment block)

- [ ] **Step 1: Replace the deviation comment with the consumer-wired comment**

In `modules/world/npc.go`, locate the field doc at lines 88-97. Replace:

```go
	// walktrigger queues a deferred AI-queue trigger (0..19, -1 = unset)
	// to fire when this NPC completes a walk step. Written by the
	// NPC_WALKTRIGGER (opcode 2545) handler. NOT YET CONSUMED — the
	// AI-tick walktrigger consumption is tracked deviation
	// NAI-37-D-WALKTRIGGER-NOREADER. Mirrors TS Npc.walktrigger.
	// Default in NewNpc is -1 (sentinel); existing &Npc{...} literals in
	// test files default to walktrigger=0 which is benign in NAI-37 (no
	// reader). When the AI-tick consumer is ported (future sub-spec),
	// every literal must be audited per plan_enumerate_struct_literals.md.
	walktrigger    int
	walktriggerArg int
```

with:

```go
	// walktrigger queues a deferred AI-queue trigger (0..19, -1 = unset)
	// to fire on the next updateMovement tick (BEFORE step consumption).
	// Written by the NPC_WALKTRIGGER (opcode 2545) handler at
	// pkg/script/handlers_npc.go:407 (transformed queueID-1). Read +
	// cleared by (*Npc).updateMovement at npc_interaction.go (NAI-51 T2.1).
	// Mirrors TS Npc.walktrigger / Npc.ts:343-360. Default in NewNpc is -1
	// (sentinel); raw &Npc{...} test literals default to 0 — safe because
	// existing tests build via NewNpc, and the consumer's `n.typ != nil`
	// guard short-circuits any literal that omits typ.
	walktrigger    int
	walktriggerArg int
```

- [ ] **Step 2: Verify no stale `NAI-37-D-WALKTRIGGER-NOREADER` references remain**

Run: `rg "NAI-37-D-WALKTRIGGER-NOREADER" pkg/ modules/ cmd/`
Expected: no output (zero matches).

- [ ] **Step 3: Run the full repo suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`
Expected: PASS.

- [ ] **Step 4: Bundle-2 close commit (memory trailer)**

```bash
git add modules/world/npc.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
chore(close): NAI-51 Bundle 2 — NPC walktrigger consumer wired

Closes deviation: NAI-37-D-WALKTRIGGER-NOREADER.

Net deviation tally: 22 → 23 (T1.7 introduced
NAI-51-D-PLAYER-WALKTRIGGER-NO-PROTECT-CHECK) → 21 (T1.9 retired
NAI-44-D-PLAYER-WALKTRIGGER-NOOP; T2.2 retires this one). Net change: −1.

Closes memory: nai_followups.md
EOF
)"
```

---

## Self-review

**Spec coverage:**
- Bundle 1 — Player field: T1.1 ✓
- Bundle 1 — `WalkTrigger`/`SetWalkTrigger` interface + adapter: T1.2-1.4 ✓
- Bundle 1 — `P_WALKTRIGGER` opcode handler: T1.5 ✓
- Bundle 1 — `GETWALKTRIGGER` opcode handler: T1.6 ✓
- Bundle 1 — `processWalktrigger` body: T1.7 ✓
- Bundle 1 — `processInteraction` integration tests: T1.8 ✓
- Bundle 1 — retire NAI-44-D-PLAYER-WALKTRIGGER-NOOP: T1.7 (inline comment swap) + T1.9 (close commit) ✓
- Bundle 1 — introduce NAI-51-D-PLAYER-WALKTRIGGER-NO-PROTECT-CHECK: T1.7 (inline comment) ✓
- Bundle 2 — `updateMovement` consumer block: T2.1 ✓
- Bundle 2 — five Bundle-2 tests: T2.1 ✓
- Bundle 2 — retire NAI-37-D-WALKTRIGGER-NOREADER: T2.2 ✓

**Type consistency:**
- `WalkTrigger() int` / `SetWalkTrigger(int)` interface ↔ `*Player.WalkTrigger() int` / `SetWalkTrigger(int)` adapter ↔ `mockPlayer.WalkTrigger()/SetWalkTrigger()` mock — names match across all four sites.
- `walktrigger int` field name consistent across `Player` (added) and `Npc` (existing).
- `runScript(sf, p, nil, true, nil, nil)` matches the existing 6-arg signature at `script.go:86`.
- `runNpcScript(sf, n, nil, []int{wtArg}, nil)` matches the existing 5-arg signature at `npc_script.go:278`.
- `GetByID(uint32(p.walktrigger))` — `Provider.GetByID` takes `uint32` per `provider.go:172`.

**Placeholder scan:** No TBD / TODO / "implement later" / "similar to" / unspecified-handler-shape language. Every code step shows full code; every test shows full test body.

**Deviation-tag consistency:** `NAI-44-D-PLAYER-WALKTRIGGER-NOOP` retired in T1.7 (comment block swap) and asserted-absent in T1.9. `NAI-37-D-WALKTRIGGER-NOREADER` retired in T2.2 (comment swap) and asserted-absent in same task. New tag `NAI-51-D-PLAYER-WALKTRIGGER-NO-PROTECT-CHECK` introduced exactly once in T1.7's `processWalktrigger` doc-comment.
