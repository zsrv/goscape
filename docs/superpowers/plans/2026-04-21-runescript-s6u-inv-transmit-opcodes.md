# S6u — inv_transmit / inv_stoptransmit Opcodes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development or superpowers:executing-plans. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Wire S6p-2's `Player.invListenOnCom` / `invStopListenOnCom` as two RuneScript opcodes (`inv_transmit`, `inv_stoptransmit`).

**Architecture:** Extend `ActivePlayer` interface (2 methods) → implement on `*Player` (2 wrappers) → add 2 handlers in `handlers_inv.go` → register in `handlers.go` → test with `mockPlayer` extension.

**Tech Stack:** Go 1.26, standard `testing`.

**Spec:** `docs/superpowers/specs/2026-04-21-runescript-s6u-inv-transmit-opcodes-design.md` (commit `050c034`).

---

## File Structure

### Production
- **Modify:** `pkg/script/active.go` — add 2 interface methods
- **Modify:** `pkg/script/handlers_inv.go` — add 2 handler functions (alongside the existing INV_* handlers, not in handlers_player.go — `handlers_inv.go` is the correct file per codebase convention)
- **Modify:** `pkg/script/handlers.go` — register both handlers in the opcode→handler map
- **Modify:** `modules/world/player_script.go` — implement `(*Player).InvListenOnCom` + `.InvStopListenOnCom` (thin wrappers)

### Tests
- **Modify:** `pkg/script/runner_test.go` — extend `mockPlayer` with 2 new methods that record calls
- **Modify:** `pkg/script/handlers_inv_test.go` — add 4 new tests

---

## Single Task

**Files:**
- `pkg/script/active.go`
- `pkg/script/handlers_inv.go`
- `pkg/script/handlers.go`
- `pkg/script/runner_test.go`
- `pkg/script/handlers_inv_test.go`
- `modules/world/player_script.go`

### TDD context

Red-green cycle: extend mockPlayer first (so tests compile) → write 4 failing tests (interfaces/handlers not yet added) → add interface methods + handlers + registration + Player wrapper → tests pass.

- [ ] **Step 1: Capture baseline test count.**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... 2>&1 | tail -5`
Expected: PASS. Record the count.

- [ ] **Step 2: Extend `mockPlayer` in `pkg/script/runner_test.go`.**

Find `type mockPlayer struct {` (around line 79). Add fields to capture call history:

```go
// Inside the mockPlayer struct, add (near existing inv-related fields if any,
// or at end of struct body before the closing `}`):
lastInvListenOnCom     []mockInvListen
lastInvStopListenOnCom []int // com values
```

Just above the `type mockPlayer struct` definition, add a helper type:

```go
type mockInvListen struct {
	InvType int
	Com     int
	Source  int
}
```

At the bottom of the existing method list (after the last `func (m *mockPlayer)` entry), add:

```go
func (m *mockPlayer) InvListenOnCom(invType, com, source int) {
	m.lastInvListenOnCom = append(m.lastInvListenOnCom, mockInvListen{InvType: invType, Com: com, Source: source})
}

func (m *mockPlayer) InvStopListenOnCom(com int) {
	m.lastInvStopListenOnCom = append(m.lastInvStopListenOnCom, com)
}
```

- [ ] **Step 3: Attempt to build — expect interface mismatch.**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./pkg/script/...`
Expected: FAIL with "mockPlayer does not implement ActivePlayer (missing method InvListenOnCom)" — because mockPlayer implements ActivePlayer, and ActivePlayer will need the new methods. Wait — this only fails AFTER we add the methods to the interface. Skip this check; proceed to Step 4.

- [ ] **Step 4: Add 4 failing tests to `pkg/script/handlers_inv_test.go`.**

Append:

```go
// TestInvTransmitRegistersListener runs a script pushing (com, inv) then
// OpInvTransmit; asserts the mock player recorded
// InvListenOnCom(invType, com, -1). Matches TS InvOps.ts INV_TRANSMIT.
func TestInvTransmitRegistersListener(t *testing.T) {
	mp := &mockPlayer{}

	sf := &ScriptFile{
		Name: "inv_transmit",
		Opcodes: []Opcode{
			OpPushConstantInt, // com
			OpPushConstantInt, // inv (top)
			OpInvTransmit,
			OpReturn,
		},
		IntOperands:      []int32{149, 93, 0, 0},
		StringOperands:   []string{"", "", "", ""},
		InstructionCount: 4,
	}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(mp.lastInvListenOnCom) != 1 {
		t.Fatalf("expected 1 call to InvListenOnCom, got %d", len(mp.lastInvListenOnCom))
	}
	got := mp.lastInvListenOnCom[0]
	if got.InvType != 93 || got.Com != 149 || got.Source != -1 {
		t.Errorf("InvListenOnCom args: got %+v, want {InvType:93, Com:149, Source:-1}", got)
	}
}

// TestInvTransmitNoActivePlayerErrors verifies INV_TRANSMIT returns
// an error when PtrActivePlayer is not set.
func TestInvTransmitNoActivePlayerErrors(t *testing.T) {
	sf := newSingleOp("inv_transmit_no_player", OpInvTransmit)
	state := Init(sf, nil, false, nil, nil)
	// Push dummy operands so the handler's PopInt doesn't panic before
	// the active-player guard fires.
	state.PushInt(93)
	state.PushInt(149)

	err := Execute(state)
	if err == nil || err.Error() != "INV_TRANSMIT: no active player" {
		t.Errorf("expected 'INV_TRANSMIT: no active player' error, got %v", err)
	}
}

// TestInvStopTransmitUnregistersListener runs a script pushing com then
// OpInvStopTransmit; asserts mockPlayer recorded InvStopListenOnCom(com).
func TestInvStopTransmitUnregistersListener(t *testing.T) {
	mp := &mockPlayer{}

	sf := &ScriptFile{
		Name: "inv_stoptransmit",
		Opcodes: []Opcode{
			OpPushConstantInt, // com
			OpInvStopTransmit,
			OpReturn,
		},
		IntOperands:      []int32{149, 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(mp.lastInvStopListenOnCom) != 1 || mp.lastInvStopListenOnCom[0] != 149 {
		t.Errorf("InvStopListenOnCom: got %v, want [149]", mp.lastInvStopListenOnCom)
	}
}

// TestInvStopTransmitNoActivePlayerErrors verifies INV_STOPTRANSMIT
// returns an error when PtrActivePlayer is not set.
func TestInvStopTransmitNoActivePlayerErrors(t *testing.T) {
	sf := newSingleOp("inv_stoptransmit_no_player", OpInvStopTransmit)
	state := Init(sf, nil, false, nil, nil)
	state.PushInt(149) // com

	err := Execute(state)
	if err == nil || err.Error() != "INV_STOPTRANSMIT: no active player" {
		t.Errorf("expected 'INV_STOPTRANSMIT: no active player' error, got %v", err)
	}
}
```

If `handlers_inv_test.go` doesn't have `newSingleOp` or it's package-private to `handlers_player_test.go`, use a full `ScriptFile` literal like `TestInvTransmitRegistersListener` does. Actually `newSingleOp` lives at `handlers_player_test.go:8` and is package-private to the `script` test package — reachable from handlers_inv_test.go since both are in `package script`. Use it.

- [ ] **Step 5: Build — expect "undefined" errors for the new interface methods + handlers + opcodes.**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./pkg/script/...`
Expected: FAIL — specifically because:
- `mockPlayer.InvListenOnCom` / `InvStopListenOnCom` were added (Step 2), so mockPlayer now has EXTRA methods that aren't on the interface — this compiles fine (extra methods are allowed).
- But `s.Self.InvListenOnCom(...)` inside handleInvTransmit doesn't exist yet — so handlers_inv.go won't compile once we add it.
- The tests reference `OpInvTransmit` / `OpInvStopTransmit` which ARE defined in opcode.go, so those refs compile.
- Test execution: `Execute(state)` with an unregistered opcode will return an error like "unregistered opcode", failing the positive tests.

So the failure mode is: tests compile but fail at runtime because the opcodes aren't registered with handlers. That's fine — that's the red phase.

If the build fails for unrelated reasons (e.g., missing type), STOP and report.

- [ ] **Step 6: Run tests to verify red phase.**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run 'TestInvTransmitRegistersListener|TestInvTransmitNoActivePlayerErrors|TestInvStopTransmitUnregistersListener|TestInvStopTransmitNoActivePlayerErrors' -v`
Expected:
- `TestInvTransmitRegistersListener` — FAIL (opcode not registered; Execute returns error)
- `TestInvTransmitNoActivePlayerErrors` — FAIL (opcode not registered — wrong error string)
- `TestInvStopTransmitUnregistersListener` — FAIL (same)
- `TestInvStopTransmitNoActivePlayerErrors` — FAIL (same)

- [ ] **Step 7: Extend `ActivePlayer` interface.**

In `pkg/script/active.go`, add 2 methods. Find the spot between existing `ActivePlayer` methods — the natural grouping is near the end of the interface (after inv-adjacent methods if any; otherwise just before the closing `}`). Add:

```go
// S6u: inventory listener registration opcodes (inv_transmit /
// inv_stoptransmit).

// InvListenOnCom registers an inventory listener at UI component id
// `com` tracking inv type `invType`. `source == -1` means the
// world-shared inventory; `source >= 0` means the player at that server
// slot. Replaces any existing listener at com; resets FirstSeen=true
// on replace. Safe when the implementation's listener map is still nil
// — it must lazy-init.
InvListenOnCom(invType, com, source int)

// InvStopListenOnCom unregisters the listener at UI component id com.
// No-op when no listener exists there. Must be safe when the listener
// map is nil.
InvStopListenOnCom(com int)
```

- [ ] **Step 8: Add handlers in `pkg/script/handlers_inv.go`.**

Append to the file (after the last existing `func handleInv*`):

```go
// -- Listener registration (S6u) -----------------------------------------

// handleInvTransmit implements INV_TRANSMIT. Registers a listener on
// the active player for UI component `com` tracking world-shared
// inventory type `invType` (source=-1).
//
// TS: InvOps.ts INV_TRANSMIT — popInt(inv), popInt(com),
// activePlayer.invListenOnCom(inv, com, -1).
func handleInvTransmit(s *ScriptState) error {
	if err := requireActivePlayer(s, "INV_TRANSMIT"); err != nil {
		return err
	}
	invType := s.PopInt()
	com := s.PopInt()
	s.Self.InvListenOnCom(invType, com, -1)
	return nil
}

// handleInvStopTransmit implements INV_STOPTRANSMIT. Unregisters the
// listener at UI component `com`. Safe when no listener exists there.
//
// TS: InvOps.ts INV_STOPTRANSMIT — popInt(com),
// activePlayer.invStopListenOnCom(com).
func handleInvStopTransmit(s *ScriptState) error {
	if err := requireActivePlayer(s, "INV_STOPTRANSMIT"); err != nil {
		return err
	}
	com := s.PopInt()
	s.Self.InvStopListenOnCom(com)
	return nil
}
```

If `requireActivePlayer` isn't defined in handlers_inv.go (it's in handlers_player.go), it's accessible because both are `package script`. Confirm and use bare name.

- [ ] **Step 9: Register handlers in `pkg/script/handlers.go`.**

Find the opcode→handler map (the big `var handlers = map[Opcode]func(*ScriptState) error{` block, around line 100+). Scan for existing `OpInv*: handleInv*,` entries (there are many — e.g., `OpInvAdd: handleInvAdd,`). Add in the alphabetical position that matches the ordering convention:

```go
OpInvStopTransmit: handleInvStopTransmit,
OpInvTransmit:     handleInvTransmit,
```

Confirm the existing ordering (alphabetical by Opcode name or by enum value — check surrounding entries before inserting).

- [ ] **Step 10: Add Player wrapper methods in `modules/world/player_script.go`.**

Append to the file:

```go
// InvListenOnCom implements script.ActivePlayer. Thin wrapper
// delegating to the internal unexported method landed in S6p-2.
func (p *Player) InvListenOnCom(invType, com, source int) {
	p.invListenOnCom(invType, com, source)
}

// InvStopListenOnCom implements script.ActivePlayer. Thin wrapper
// delegating to the internal unexported method landed in S6p-2.
func (p *Player) InvStopListenOnCom(com int) {
	p.invStopListenOnCom(com)
}
```

- [ ] **Step 11: Build the full repo.**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`
Expected: PASS (mockPlayer + Player both implement the extended ActivePlayer).

- [ ] **Step 12: Run the 4 new tests.**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run 'TestInvTransmitRegistersListener|TestInvTransmitNoActivePlayerErrors|TestInvStopTransmitUnregistersListener|TestInvStopTransmitNoActivePlayerErrors' -v`
Expected: PASS × 4.

- [ ] **Step 13: Run full repo tests + vet.**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`
Expected: baseline + 4 PASS. No regressions.

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...`
Expected: clean.

- [ ] **Step 14: Commit.**

```bash
git add pkg/script/active.go \
        pkg/script/handlers_inv.go \
        pkg/script/handlers.go \
        pkg/script/runner_test.go \
        pkg/script/handlers_inv_test.go \
        modules/world/player_script.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script,world): inv_transmit / inv_stoptransmit opcodes — wire S6p-2 API (S6u)

Implement two RuneScript opcodes that let scripts register/unregister
UI inventory listeners on the active player:

  INV_TRANSMIT       (4331) — inv_transmit(com, inv) → listener at
                              com tracking world-shared inv (source=-1)
  INV_STOPTRANSMIT   (4326) — inv_stoptransmit(com) → delete listener

Both delegate to S6p-2's Player.invListenOnCom / invStopListenOnCom
via two new ActivePlayer interface methods:
  InvListenOnCom(invType, com, source int)
  InvStopListenOnCom(com int)

Matches TS InvOps.ts INV_TRANSMIT / INV_STOPTRANSMIT exactly: popInt
order is (inv, com) for transmit, (com) for stoptransmit. On the
no-active-player path returns the opcode-name-prefixed error string
("INV_TRANSMIT: no active player", etc.) matching the stat-op family.

Scope boundary: OpInvOtherTransmit (4332) deferred because goscape
has no $active_player2 infrastructure. Opcode stays in the enum +
disasm; no handler registered. Filed as S6u-SB1.

Tests: 4 new (2 per opcode: formula + no-active-player error). mockPlayer
extended with InvListenOnCom / InvStopListenOnCom methods that record
call history.

Closes no deviations. Zero new deviations.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```
