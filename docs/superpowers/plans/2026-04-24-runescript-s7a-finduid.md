# S7a — FINDUID + P_FINDUID Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement the two RuneScript player-lookup-by-UID opcodes (`FINDUID` = 2019, `P_FINDUID` = 2073) so `[proc,update_all]` and other scripts that pop a UID and rebind the active player no longer abort with "no handler for ...".

**Architecture:** Layered introduction — (1) script-package infrastructure: `PlayerLookup` host interface, `ScriptState.PlayerLookup` field, `CanAccess() bool` on the `ActivePlayer` interface; (2) `handleFindUID` handler (unprotected, no access check); (3) `handlePFindUID` handler (protected, with self-reacquire fast-path and `CanAccess` gate); (4) world-module wiring: `Player.CanAccess`, `Server.LookupPlayerByUID`, and `state.PlayerLookup = s` in `runScript`. Goscape collapses TS's `ActivePlayer` / `ProtectedActivePlayer` pointer pair into `PtrActivePlayer` + `ScriptState.Protect bool` (established by S6w); P_FINDUID's "set both pointers" reduces to "set pointer + set Protect = true".

**Tech Stack:** Go 1.26+. No new packages. Touches `pkg/script/state.go`, `pkg/script/active.go`, `pkg/script/handlers.go`, `pkg/script/handlers_player.go`, `pkg/script/handlers_player_test.go`, `pkg/script/runner_test.go`, `modules/world/player.go`, `modules/world/server.go`, `modules/world/server_test.go`, `modules/world/script.go`. Spec: `docs/superpowers/specs/2026-04-24-runescript-s7a-finduid-design.md`.

---

## Task 1: Script-package foundation (interface + state field + CanAccess)

**Files:**
- Modify: `pkg/script/state.go` — add `PlayerLookup` interface and `ScriptState.PlayerLookup` field
- Modify: `pkg/script/active.go:6-305` — add `CanAccess() bool` to the `ActivePlayer` interface
- Modify: `pkg/script/runner_test.go:95-206` — add `canAccessValue bool` field and `CanAccess()` method to `mockPlayer` so existing tests still compile

Lands first because both handlers in Tasks 2 and 3 depend on these symbols. No handler dispatch wired yet — just plumbing.

- [ ] **Step 1: Add the PlayerLookup interface and ScriptState field**

Modify `pkg/script/state.go`. Add the interface just above `WorldVars` (around line 22, before the existing `WorldVars` declaration) — it belongs with the other host-surface interfaces:

```go
// PlayerLookup resolves a UID to an ActivePlayer if a player with that UID
// is currently logged in. Handlers decide whether the result is usable:
// FINDUID accepts any match; P_FINDUID additionally gates on CanAccess.
// Returns nil if no logged-in player has that UID.
type PlayerLookup interface {
	LookupPlayerByUID(uid int) ActivePlayer
}
```

Then in the `ScriptState` struct (around line 70, immediately after the `Inv InvLookup` field), add:

```go
	// PlayerLookup is the player-resolution surface for FINDUID / P_FINDUID.
	// Callers set this after Init if the script uses UID-keyed player
	// ops. Nil disables the lookup (handlers degrade to "not found").
	PlayerLookup PlayerLookup
```

- [ ] **Step 2: Add CanAccess to the ActivePlayer interface**

Modify `pkg/script/active.go`. Append inside the `ActivePlayer` interface (right before the closing `}` at line 305), grouped at the end so it's clearly S7a:

```go

	// S7a: protected-binding gate.

	// CanAccess reports whether the player can be bound as the active
	// player by P_FINDUID. Returns false when delayed, when a modal
	// main/chat is open, or when a suspended protected script is
	// stored on the player. Mirrors TS Player.canAccess
	// (Engine-TS/src/engine/entity/Player.ts:805-812). FINDUID does
	// NOT consult this — only P_FINDUID does.
	CanAccess() bool
```

- [ ] **Step 3: Add canAccessValue + CanAccess to mockPlayer**

Modify `pkg/script/runner_test.go`. In the `mockPlayer` struct (around line 204, just before the closing `}`), add:

```go
	// S7a: canAccess return value. Defaults to false; tests that exercise
	// P_FINDUID positive paths set this to true explicitly.
	canAccessValue bool
```

Then add the method below the existing `UID() int` method (around line 391):

```go
// CanAccess returns the seeded accessibility flag for P_FINDUID tests.
func (m *mockPlayer) CanAccess() bool { return m.canAccessValue }
```

- [ ] **Step 4: Verify script package compiles and all existing tests still pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./pkg/script/`
Expected: `ok github.com/zsrv/goscape/pkg/script`

If any other ActivePlayer implementation exists in the codebase that hasn't been updated, the build fails. Resolve by grepping: `grep -rn "func.*ActivePlayer\b" --include="*.go"` — the only production impl is `*Player` in `modules/world`, which Task 4 handles. If any test file has a second mock implementation, add a `CanAccess() bool { return false }` stub there.

- [ ] **Step 5: Verify the world module still compiles (it won't run yet — Player.CanAccess not added until Task 4)**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./modules/world/`
Expected: FAIL with `*Player does not implement script.ActivePlayer (missing method CanAccess)`.

This failure is expected and confirms the interface change propagates. Do NOT fix it yet — Task 4 adds `Player.CanAccess`.

- [ ] **Step 6: Commit**

```bash
git add pkg/script/state.go pkg/script/active.go pkg/script/runner_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): S7a Task 1 — PlayerLookup interface + CanAccess on ActivePlayer

Lays the foundation for FINDUID / P_FINDUID (Tasks 2-4). No dispatch
wired yet. modules/world does not build until Task 4 adds
Player.CanAccess — intentional staging.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: handleFindUID (unprotected lookup)

**Files:**
- Modify: `pkg/script/handlers_player.go` — append `handleFindUID`
- Modify: `pkg/script/handlers.go` — register `OpFindUID`
- Create: `pkg/script/handlers_player_test.go` — add 3 tests (or append to existing file if present)

FINDUID is simpler than P_FINDUID (no self-reacquire, no CanAccess check). Landing it first establishes the mock-lookup fixture that Task 3 reuses.

- [ ] **Step 1: Write TestFindUIDFound (failing test)**

Append to `pkg/script/handlers_player_test.go` (create the file if it doesn't exist; the file was referenced in the grep output so it does). The test uses a new `mockPlayerLookup` helper that Task 2 also introduces:

```go
// mockPlayerLookup resolves UIDs via a pre-seeded map. Introduced in S7a.
type mockPlayerLookup struct {
	byUID map[int]ActivePlayer
	calls int
}

func (m *mockPlayerLookup) LookupPlayerByUID(uid int) ActivePlayer {
	m.calls++
	return m.byUID[uid]
}

// TestFindUIDFound: lookup returns a target → push 1, Self rebinds,
// PtrActivePlayer set, Protect stays false (FINDUID is unprotected).
func TestFindUIDFound(t *testing.T) {
	target := &mockPlayer{username: "Target", uidValue: 99}
	origSelf := &mockPlayer{username: "Orig", uidValue: 1}
	lookup := &mockPlayerLookup{byUID: map[int]ActivePlayer{99: target}}

	sf := newSingleOp("finduid_found", OpFindUID)
	state := Init(sf, origSelf, false, nil, nil)
	state.PlayerLookup = lookup
	state.PushInt(99)

	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if state.ISP != 1 || state.IntStack[0] != 1 {
		t.Errorf("stack: got [%v], want [1]", state.IntStack[:state.ISP])
	}
	if state.Self != target {
		t.Errorf("Self: got %v, want target", state.Self)
	}
	if state.Pointers&PtrActivePlayer == 0 {
		t.Errorf("PtrActivePlayer should be set, pointers=%b", state.Pointers)
	}
	if state.Protect {
		t.Errorf("Protect should remain false for FINDUID")
	}
}
```

Note: `newSingleOp` is an existing helper in the test package (used by S6w tests). Verify its signature before running — run `grep -n "func newSingleOp" pkg/script/*_test.go` to confirm shape. If absent under that exact name, substitute the equivalent helper from the existing tests (e.g., `buildScript` or inline construction as seen in other S6 tests).

- [ ] **Step 2: Run test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./pkg/script/ -run TestFindUIDFound -v`
Expected: FAIL with an error message like `no handler for FINDUID (opcode 2019)`.

- [ ] **Step 3: Write TestFindUIDNotFound and TestFindUIDNoLookupConfigured (both failing)**

Append to `pkg/script/handlers_player_test.go`:

```go
// TestFindUIDNotFound: lookup returns nil → push 0, Self unchanged.
func TestFindUIDNotFound(t *testing.T) {
	origSelf := &mockPlayer{username: "Orig", uidValue: 1}
	lookup := &mockPlayerLookup{byUID: map[int]ActivePlayer{}}

	sf := newSingleOp("finduid_notfound", OpFindUID)
	state := Init(sf, origSelf, false, nil, nil)
	state.PlayerLookup = lookup
	state.PushInt(999)

	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if state.ISP != 1 || state.IntStack[0] != 0 {
		t.Errorf("stack: got [%v], want [0]", state.IntStack[:state.ISP])
	}
	if state.Self != origSelf {
		t.Errorf("Self should be unchanged, got %v", state.Self)
	}
}

// TestFindUIDNoLookupConfigured: PlayerLookup nil → push 0.
// Host configurations that don't wire a lookup degrade to "not found"
// rather than erroring, matching the LAST_INT / LAST_COM precedent.
func TestFindUIDNoLookupConfigured(t *testing.T) {
	origSelf := &mockPlayer{username: "Orig"}

	sf := newSingleOp("finduid_nolookup", OpFindUID)
	state := Init(sf, origSelf, false, nil, nil)
	// state.PlayerLookup left nil
	state.PushInt(1)

	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if state.ISP != 1 || state.IntStack[0] != 0 {
		t.Errorf("stack: got [%v], want [0]", state.IntStack[:state.ISP])
	}
	if state.Self != origSelf {
		t.Errorf("Self should be unchanged")
	}
}
```

- [ ] **Step 4: Run the three tests — all fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./pkg/script/ -run "TestFindUID" -v`
Expected: 3 FAIL results, same "no handler" error for each.

- [ ] **Step 5: Implement handleFindUID**

Append to `pkg/script/handlers_player.go` (after the last existing handler):

```go
// handleFindUID (opcode 2019) pops a uid, looks up the logged-in player
// with that uid, and rebinds Self on success. Pushes 1 if found, 0 if
// the lookup returned nil or no PlayerLookup is configured.
//
// Does NOT check CanAccess — that's P_FINDUID's job. Does NOT set
// Protect. Mirrors TS PlayerOps.ts:60-72 with goscape's collapsed
// pointer model (single PtrActivePlayer).
func handleFindUID(s *ScriptState) error {
	uid := s.PopInt()
	if s.PlayerLookup == nil {
		s.PushInt(0)
		return nil
	}
	target := s.PlayerLookup.LookupPlayerByUID(uid)
	if target == nil {
		s.PushInt(0)
		return nil
	}
	s.Self = target
	s.Pointers |= PtrActivePlayer
	s.PushInt(1)
	return nil
}
```

- [ ] **Step 6: Register OpFindUID in the handlers map**

Modify `pkg/script/handlers.go`. The handlers map currently ends with S6-era registrations (there are comment-delimited sections per sub-spec). Find the last registration in the map (just before the closing `}`) and add, preceded by a section comment:

```go

	// S7a: player UID lookup.
	OpFindUID: handleFindUID,
```

The exact line depends on current file state — grep for `OpFindUID` to confirm it's not already registered, then pattern-match on how S6w added `OpPOpLoc` / `OpPOpNpc` as a guide.

- [ ] **Step 7: Run the three tests — all pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./pkg/script/ -run "TestFindUID" -v`
Expected: 3 PASS.

- [ ] **Step 8: Run the full script-package test suite — no regressions**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./pkg/script/`
Expected: `ok`.

- [ ] **Step 9: Commit**

```bash
git add pkg/script/handlers_player.go pkg/script/handlers.go pkg/script/handlers_player_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): S7a Task 2 — handleFindUID (opcode 2019)

Unprotected UID-to-player lookup. Pops uid, pushes 1 on successful
rebind or 0 on miss. No CanAccess check; no Protect mutation.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: handlePFindUID (protected lookup)

**Files:**
- Modify: `pkg/script/handlers_player.go` — append `handlePFindUID`
- Modify: `pkg/script/handlers.go` — register `OpPFindUID`
- Modify: `pkg/script/handlers_player_test.go` — append 4 tests

Adds the self-reacquire fast-path, the CanAccess gate, and `Protect = true` mutation. Reuses `mockPlayerLookup` from Task 2.

- [ ] **Step 1: Write TestPFindUIDSelfReacquire (failing test)**

Append to `pkg/script/handlers_player_test.go`:

```go
// TestPFindUIDSelfReacquire: script already runs protected on the target
// uid → push 1 with no state mutation, no lookup call (fast-path).
// Mirrors TS PlayerOps.ts:79-83.
func TestPFindUIDSelfReacquire(t *testing.T) {
	self := &mockPlayer{username: "Self", uidValue: 42}
	lookup := &mockPlayerLookup{byUID: map[int]ActivePlayer{}}

	sf := newSingleOp("pfinduid_self", OpPFindUID)
	state := Init(sf, self, true, nil, nil) // protect=true
	state.PlayerLookup = lookup
	state.PushInt(42)

	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if state.ISP != 1 || state.IntStack[0] != 1 {
		t.Errorf("stack: got [%v], want [1]", state.IntStack[:state.ISP])
	}
	if state.Self != self {
		t.Errorf("Self should be unchanged on self-reacquire")
	}
	if lookup.calls != 0 {
		t.Errorf("fast-path should skip lookup, calls=%d", lookup.calls)
	}
	if !state.Protect {
		t.Errorf("Protect should remain true")
	}
}
```

- [ ] **Step 2: Write the three remaining tests**

Continue appending to `pkg/script/handlers_player_test.go`:

```go
// TestPFindUIDFoundCanAccess: target is reachable and CanAccess=true →
// push 1, Self rebinds, Protect=true, PtrActivePlayer set.
func TestPFindUIDFoundCanAccess(t *testing.T) {
	target := &mockPlayer{username: "Target", uidValue: 99, canAccessValue: true}
	origSelf := &mockPlayer{username: "Orig", uidValue: 1}
	lookup := &mockPlayerLookup{byUID: map[int]ActivePlayer{99: target}}

	sf := newSingleOp("pfinduid_ok", OpPFindUID)
	state := Init(sf, origSelf, false, nil, nil) // protect=false initially
	state.PlayerLookup = lookup
	state.PushInt(99)

	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if state.ISP != 1 || state.IntStack[0] != 1 {
		t.Errorf("stack: got [%v], want [1]", state.IntStack[:state.ISP])
	}
	if state.Self != target {
		t.Errorf("Self: got %v, want target", state.Self)
	}
	if state.Pointers&PtrActivePlayer == 0 {
		t.Errorf("PtrActivePlayer should be set")
	}
	if !state.Protect {
		t.Errorf("Protect should be true after successful P_FINDUID")
	}
}

// TestPFindUIDFoundCannotAccess: target exists but CanAccess=false →
// push 0, Self unchanged, Protect unchanged.
func TestPFindUIDFoundCannotAccess(t *testing.T) {
	target := &mockPlayer{username: "Target", uidValue: 99, canAccessValue: false}
	origSelf := &mockPlayer{username: "Orig", uidValue: 1}
	lookup := &mockPlayerLookup{byUID: map[int]ActivePlayer{99: target}}

	sf := newSingleOp("pfinduid_busy", OpPFindUID)
	state := Init(sf, origSelf, false, nil, nil)
	state.PlayerLookup = lookup
	state.PushInt(99)

	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if state.ISP != 1 || state.IntStack[0] != 0 {
		t.Errorf("stack: got [%v], want [0]", state.IntStack[:state.ISP])
	}
	if state.Self != origSelf {
		t.Errorf("Self should be unchanged when CanAccess=false")
	}
	if state.Protect {
		t.Errorf("Protect should remain false")
	}
}

// TestPFindUIDNotFound: lookup returns nil → push 0, Self unchanged.
func TestPFindUIDNotFound(t *testing.T) {
	origSelf := &mockPlayer{username: "Orig", uidValue: 1}
	lookup := &mockPlayerLookup{byUID: map[int]ActivePlayer{}}

	sf := newSingleOp("pfinduid_notfound", OpPFindUID)
	state := Init(sf, origSelf, false, nil, nil)
	state.PlayerLookup = lookup
	state.PushInt(999)

	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if state.ISP != 1 || state.IntStack[0] != 0 {
		t.Errorf("stack: got [%v], want [0]", state.IntStack[:state.ISP])
	}
	if state.Self != origSelf {
		t.Errorf("Self should be unchanged")
	}
}
```

- [ ] **Step 3: Run the four tests — all fail with "no handler"**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./pkg/script/ -run "TestPFindUID" -v`
Expected: 4 FAIL with `no handler for P_FINDUID (opcode 2073)`.

- [ ] **Step 4: Implement handlePFindUID**

Append to `pkg/script/handlers_player.go` (after `handleFindUID`):

```go
// handlePFindUID (opcode 2073) is P_FINDUID — the protected variant of
// FINDUID. Pops a uid, tries to rebind Self with protected access.
// Three outcomes:
//   - Self-reacquire fast-path: script already runs protected on a
//     player whose UID matches → push 1, no state change, no lookup.
//   - Lookup miss OR target.CanAccess()==false → push 0.
//   - Success → Self rebinds, PtrActivePlayer set, Protect=true, push 1.
//
// Mirrors TS PlayerOps.ts:75-94 with goscape's collapsed pointer model
// (single PtrActivePlayer + ScriptState.Protect bool).
func handlePFindUID(s *ScriptState) error {
	uid := s.PopInt()
	// Self-reacquire fast-path: already protected on this player.
	if s.Protect && s.Self != nil && s.Self.UID() == uid {
		s.PushInt(1)
		return nil
	}
	if s.PlayerLookup == nil {
		s.PushInt(0)
		return nil
	}
	target := s.PlayerLookup.LookupPlayerByUID(uid)
	if target == nil || !target.CanAccess() {
		s.PushInt(0)
		return nil
	}
	s.Self = target
	s.Pointers |= PtrActivePlayer
	s.Protect = true
	s.PushInt(1)
	return nil
}
```

- [ ] **Step 5: Register OpPFindUID**

Modify `pkg/script/handlers.go`. Find the `OpFindUID: handleFindUID,` line added in Task 2 and append immediately after it (inside the same "S7a: player UID lookup." section):

```go
	OpPFindUID: handlePFindUID,
```

- [ ] **Step 6: Run all S7a tests — all pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./pkg/script/ -run "TestFindUID|TestPFindUID" -v`
Expected: 7 PASS (3 from Task 2 + 4 from Task 3).

- [ ] **Step 7: Run the full script-package test suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./pkg/script/`
Expected: `ok`.

- [ ] **Step 8: Commit**

```bash
git add pkg/script/handlers_player.go pkg/script/handlers.go pkg/script/handlers_player_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): S7a Task 3 — handlePFindUID (opcode 2073)

Protected UID lookup with self-reacquire fast-path and CanAccess
gate. Sets ScriptState.Protect=true on successful rebind (goscape's
collapsed equivalent of TS's ProtectedActivePlayer pointer add).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: World-module wiring (Player.CanAccess, Server.LookupPlayerByUID, runScript wiring)

**Files:**
- Modify: `modules/world/player.go` — add `CanAccess()` method
- Modify: `modules/world/server.go` — add `LookupPlayerByUID` method
- Modify: `modules/world/script.go:14-24` — wire `state.PlayerLookup = s`
- Modify: `modules/world/server_test.go` — append lookup tests

After this task the world module compiles again and `[proc,update_all]`'s P_FINDUID dispatch reaches a working handler.

- [ ] **Step 1: Add Player.CanAccess**

Modify `modules/world/player.go`. Add the method below the existing `Active() bool` accessor (around line 625). Find a natural location — grep for `func (p \*Player) Active` to locate it:

```go
// CanAccess reports whether this player can be bound as the active
// player by P_FINDUID. False when delayed, when a modal main/chat is
// open, or when a suspended protected script is stored. Mirrors TS
// Player.canAccess at Engine-TS/src/engine/entity/Player.ts:805-812.
//
// The World-shutdown early-return from TS is omitted — goscape has
// no global shutdown flag to consult and rejects lookups uniformly.
func (p *Player) CanAccess() bool {
	if p.delayed {
		return false
	}
	if p.modalState&(modalStateMain|modalStateChat) != 0 {
		return false
	}
	if p.activeScript != nil && p.activeScript.Protect {
		return false
	}
	return true
}
```

- [ ] **Step 2: Verify the world module builds**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./modules/world/`
Expected: `(no output)` — build succeeds now that `*Player` satisfies the expanded `ActivePlayer` interface.

- [ ] **Step 3: Write TestPlayerCanAccess (table-driven)**

Append to `modules/world/server_test.go`:

```go
// TestPlayerCanAccess asserts the four-case truth table for S7a:
// delayed, modal main/chat open, or protected activeScript → false;
// otherwise → true. Mirrors TS Player.canAccess.
func TestPlayerCanAccess(t *testing.T) {
	cases := []struct {
		name         string
		delayed      bool
		modalState   int
		protectedScript bool
		want         bool
	}{
		{"idle_no_modal_no_script", false, modalStateNone, false, true},
		{"delayed", true, modalStateNone, false, false},
		{"modal_main_open", false, modalStateMain, false, false},
		{"modal_chat_open", false, modalStateChat, false, false},
		{"modal_side_only_ok", false, modalStateSide, false, true},
		{"protected_script_stored", false, modalStateNone, true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := newTestClient(t)
			p := newPlayer(c)
			p.delayed = tc.delayed
			p.modalState = tc.modalState
			if tc.protectedScript {
				p.activeScript = &script.ScriptState{Protect: true}
			}
			if got := p.CanAccess(); got != tc.want {
				t.Errorf("CanAccess() = %v, want %v", got, tc.want)
			}
		})
	}
}
```

Note: this test requires the `script` import to already be present in `server_test.go`. If it isn't, add `"github.com/zsrv/goscape/pkg/script"` to the imports block. Verify first with `grep -n "\"github.com/zsrv/goscape/pkg/script\"" modules/world/server_test.go`.

- [ ] **Step 4: Run it — passes (no server code yet)**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./modules/world/ -run TestPlayerCanAccess -v`
Expected: 6 sub-tests PASS.

- [ ] **Step 5: Write TestLookupPlayerByUID cases (failing)**

Append to `modules/world/server_test.go`:

```go
// TestLookupPlayerByUIDFound: a single logged-in player with a matching
// uid is returned.
func TestLookupPlayerByUIDFound(t *testing.T) {
	s := newTestServer(t)
	c, _ := newTestClient(t)
	p := newPlayer(c)
	p.uid = 12345
	if err := s.addPlayer(p); err != nil {
		t.Fatalf("addPlayer: %v", err)
	}

	got := s.LookupPlayerByUID(12345)
	if got != p {
		t.Errorf("LookupPlayerByUID(12345) = %v, want %v", got, p)
	}
}

// TestLookupPlayerByUIDNotFound: returns nil for an unknown uid.
func TestLookupPlayerByUIDNotFound(t *testing.T) {
	s := newTestServer(t)
	c, _ := newTestClient(t)
	p := newPlayer(c)
	p.uid = 1
	_ = s.addPlayer(p)

	got := s.LookupPlayerByUID(999)
	if got != nil {
		t.Errorf("LookupPlayerByUID(999) = %v, want nil", got)
	}
}

// TestLookupPlayerByUIDSkipsInactive: an entry in playerLoop whose
// active flag is false is not returned even on uid match. This defends
// against stale references during the add/remove race window — the
// tick loop drains newPlayers and addPlayer flips active=true; removal
// flips active=false before the slot reassignment. See server.go:586-596.
func TestLookupPlayerByUIDSkipsInactive(t *testing.T) {
	s := newTestServer(t)
	c, _ := newTestClient(t)
	p := newPlayer(c)
	p.uid = 7
	_ = s.addPlayer(p)
	p.active = false

	got := s.LookupPlayerByUID(7)
	if got != nil {
		t.Errorf("LookupPlayerByUID(7) on inactive player = %v, want nil", got)
	}
}
```

- [ ] **Step 6: Run them — all fail with undefined method**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./modules/world/ -run TestLookupPlayerByUID -v`
Expected: 3 FAIL. Build error: `s.LookupPlayerByUID undefined`.

- [ ] **Step 7: Implement Server.LookupPlayerByUID**

Modify `modules/world/server.go`. Add the method near `addPlayer` / `removePlayer` (around line 620, after `TrackZone`):

```go
// LookupPlayerByUID returns the logged-in player whose uid field matches
// the argument, or nil if no such player is active. Intended to be
// called from the tick goroutine (playerLoop is unguarded there).
// Implements the script.PlayerLookup interface consumed by
// FINDUID / P_FINDUID (S7a).
//
// Does NOT filter on CanAccess — callers that need the protected
// variant consult the returned player's CanAccess() separately. Mirrors
// TS World.getPlayerByUid which is a pure lookup.
func (s *Server) LookupPlayerByUID(uid int) script.ActivePlayer {
	for _, p := range s.playerLoop {
		if p == nil || !p.active {
			continue
		}
		if p.uid == uid {
			return p
		}
	}
	return nil
}
```

Verify `"github.com/zsrv/goscape/pkg/script"` is already imported in `server.go` — grep with `grep -n "pkg/script" modules/world/server.go`. If not, add it to the imports block.

- [ ] **Step 8: Run the server tests — pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./modules/world/ -run "TestLookupPlayerByUID|TestPlayerCanAccess" -v`
Expected: All sub-tests PASS.

- [ ] **Step 9: Wire state.PlayerLookup in runScript**

Modify `modules/world/script.go`. Current `runScript` body (lines 14-24):

```go
func (s *Server) runScript(sf *script.ScriptFile, self script.ActivePlayer, protect bool, intArgs []int, stringArgs []string) {
	if sf == nil {
		return
	}
	state := script.Init(sf, self, protect, intArgs, stringArgs)
	state.Provider = s.scriptProvider
	state.World = s.worldVars
	state.Configs = s.configsView
	state.Inv = s.invLookup
	s.resumeOrFinish(state, self)
}
```

Add one line between `state.Inv = s.invLookup` and `s.resumeOrFinish(...)`:

```go
	state.PlayerLookup = s
```

- [ ] **Step 10: Run full world + script test suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./modules/world/ ./pkg/script/`
Expected: both packages `ok`.

- [ ] **Step 11: Run the full project test suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`
Expected: all packages `ok`.

- [ ] **Step 12: Commit**

```bash
git add modules/world/player.go modules/world/server.go modules/world/script.go modules/world/server_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): S7a Task 4 — Player.CanAccess + Server.LookupPlayerByUID

Implements the PlayerLookup / ActivePlayer-CanAccess host surfaces
that FINDUID and P_FINDUID consume. Wires state.PlayerLookup = s in
runScript so scripts reach the real lookup.

[proc,update_all]'s P_FINDUID dispatch at pc=61 now executes cleanly;
with Player.uid still -1 per S7a-D2, non-self UIDs resolve to 0
(graceful "target logged out" fallback that existing scripts handle).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: Series close

- [ ] **Step 1: Smoke-test with the live Java client**

Start the server and connect with the Java client. In the server log, confirm the absence of the `[proc,update_all] err="... no handler for P_FINDUID ..."` WARN line. The log should show `[proc,update_all]` completing without warning (it may still fail on other unimplemented opcodes downstream — those are separate sub-specs).

Record the observed outcome in the close commit.

- [ ] **Step 2: Close commit**

```bash
git commit --no-gpg-sign --allow-empty -m "$(cat <<'EOF'
chore(nai): S7a closed — FINDUID + P_FINDUID handlers

[proc,update_all] at pc=61 now dispatches cleanly. With Player.uid
still -1 (deviation S7a-D2), non-self lookups push 0 — scripts handle
this as the "target logged out" case already. Follow-up sub-spec
needed to pick a canonical uid source (likely username hash per TS
getUid()).

Closes memory: rsbuf_roundtrip_tests.md (no change; S7a is script
layer, unrelated to rsbuf encoders)

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Self-review notes

- **Spec coverage:** §3.1 PlayerLookup → Task 1. §3.2 CanAccess → Task 1 (interface) + Task 4 (Player impl). §3.3 handlers → Tasks 2 + 3. §3.4 server lookup → Task 4. §3.5 Player.CanAccess → Task 4. All tests from §5 mapped: 1-3 → Task 2, 4-7 → Task 3, 8-11 → Task 4.
- **Type consistency:** `PlayerLookup.LookupPlayerByUID(uid int) ActivePlayer` used identically in Tasks 1, 2, 3, 4. `CanAccess() bool` used identically. `state.PlayerLookup` field name consistent throughout. `OpFindUID` = 2019, `OpPFindUID` = 2073 per existing opcode.go — no invented constants.
- **Deliberately staged build failure:** Task 1 intentionally leaves `modules/world` uncompilable (the interface gained a method that `*Player` doesn't implement yet). Task 4 restores the build. This is called out in Task 1 Step 5 so the implementer doesn't panic.
- **Mock fixture discipline:** `mockPlayerLookup` is introduced inline in Task 2 Step 1 and reused in Task 3 — no duplicate struct declaration. `mockPlayer.canAccessValue` defaults to `false` so existing tests (which never call `CanAccess`) are unaffected; Task 3 positive paths seed it to `true` explicitly.
- **Newly visible risk:** Task 4 Step 3's `TestPlayerCanAccess` references `p.activeScript = &script.ScriptState{Protect: true}`. If `activeScript` is unexported and this test lives in the same package, it works. If it's in a `_test` package, the test would need to use an exported setter. Grep `grep -n "activeScript" modules/world/player.go | head -3` to confirm it's a lowercase field in the same package; `server_test.go` is `package world` (same package) per the existing `newPlayer(c)` direct call in current tests, so this is fine.
