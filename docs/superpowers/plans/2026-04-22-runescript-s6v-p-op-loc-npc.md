# S6v — p_op_loc / p_op_npc Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:subagent-driven-development. Steps use checkbox syntax.

**Goal:** Wire `P_OPLOC` (opcode 2077) and `P_OPNPC` (opcode 2078) as RuneScript handlers calling `ActivePlayer.SetInteractionScriptLoc/Npc`.

**Architecture:** 2 new ActivePlayer methods → 2 handlers in handlers_player.go → registration in handlers.go → 2 Player wrappers in player_script.go (type-assert narrow interface back to concrete types) → mockPlayer extension + 6 tests.

**Tech Stack:** Go 1.26, standard `testing`.

**Spec:** `docs/superpowers/specs/2026-04-22-runescript-s6v-p-op-loc-npc-design.md` (commit `566147d`).

---

## Single Task

**Files:**
- Modify: `pkg/script/active.go` — add 2 interface methods
- Modify: `pkg/script/handlers_player.go` — add 2 handlers
- Modify: `pkg/script/handlers.go` — register 2 handlers
- Modify: `modules/world/player_script.go` — add 2 Player methods
- Modify: `pkg/script/runner_test.go` — mockPlayer extension
- Modify: `pkg/script/handlers_player_test.go` — 6 new tests

### TDD context

Extend mockPlayer first (so tests compile), write 6 failing tests, then implement interface + handlers + registration + Player wrappers. Green at the end.

- [ ] **Step 1: Baseline test count.**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... -count=1 2>&1 | tail -5`
Record baseline.

- [ ] **Step 2: Extend `mockPlayer` in `pkg/script/runner_test.go`.**

Find `type mockPlayer struct {` (around line 79). Just above that, add helper types:

```go
type mockLocOp struct {
	Loc ActiveLoc
	Op  int
}

type mockNpcOp struct {
	Npc ActiveNpc
	Op  int
}
```

Inside the `mockPlayer` struct, near the end (before closing `}`), add fields:

```go
	lastSetInteractionScriptLoc []mockLocOp
	lastSetInteractionScriptNpc []mockNpcOp
```

At the bottom of the file (after the last `func (m *mockPlayer)` method), add:

```go
func (m *mockPlayer) SetInteractionScriptLoc(loc ActiveLoc, op int) {
	m.lastSetInteractionScriptLoc = append(m.lastSetInteractionScriptLoc, mockLocOp{Loc: loc, Op: op})
}

func (m *mockPlayer) SetInteractionScriptNpc(npc ActiveNpc, op int) {
	m.lastSetInteractionScriptNpc = append(m.lastSetInteractionScriptNpc, mockNpcOp{Npc: npc, Op: op})
}
```

- [ ] **Step 3: Add a mock ActiveLoc/ActiveNpc for tests in `pkg/script/handlers_player_test.go`.**

Check if mock implementations exist for `ActiveLoc`/`ActiveNpc`. If not, add minimal ones at the top of `handlers_player_test.go`:

```go
type mockActiveLoc struct {
	locType int
}

func (m *mockActiveLoc) LocType() int { return m.locType }
```

For `ActiveNpc`, it has more methods (`NpcType`, `NpcX`, `NpcZ`, `NpcLevel`, `NpcStat`). Check `pkg/script/active.go` around line 294 for the full method set. Add a mock struct implementing all methods, returning 0 or stored values for each. If a `mockActiveNpc` already exists elsewhere in the test package, reuse.

- [ ] **Step 4: Append 6 failing tests to `pkg/script/handlers_player_test.go`.**

```go
// TestPOpLocAnchorsOnActiveLoc runs a script pushing op=3 then OpPOpLoc;
// asserts mockPlayer recorded SetInteractionScriptLoc(loc, 3).
func TestPOpLocAnchorsOnActiveLoc(t *testing.T) {
	mp := &mockPlayer{}
	loc := &mockActiveLoc{locType: 42}

	sf := &ScriptFile{
		Name:             "p_op_loc",
		Opcodes:          []Opcode{OpPushConstantInt, OpPOpLoc, OpReturn},
		IntOperands:      []int32{3, 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	state := Init(sf, mp, false, nil, nil)
	state.ActiveLoc = loc
	state.Pointers |= PtrActiveLoc
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(mp.lastSetInteractionScriptLoc) != 1 {
		t.Fatalf("expected 1 SetInteractionScriptLoc call, got %d", len(mp.lastSetInteractionScriptLoc))
	}
	got := mp.lastSetInteractionScriptLoc[0]
	if got.Loc != loc || got.Op != 3 {
		t.Errorf("args: got %+v, want {Loc:%p, Op:3}", got, loc)
	}
}

// TestPOpLocNoActivePlayerErrors verifies P_OPLOC errors when PtrActivePlayer
// is clear.
func TestPOpLocNoActivePlayerErrors(t *testing.T) {
	sf := newSingleOp("p_op_loc_no_player", OpPOpLoc)
	state := Init(sf, nil, false, nil, nil)
	state.PushInt(3)

	err := Execute(state)
	if err == nil || err.Error() != "P_OPLOC: no active player" {
		t.Errorf("expected 'P_OPLOC: no active player', got %v", err)
	}
}

// TestPOpLocNoActiveLocErrors verifies P_OPLOC errors when ActiveLoc is nil.
func TestPOpLocNoActiveLocErrors(t *testing.T) {
	mp := &mockPlayer{}

	sf := newSingleOp("p_op_loc_no_loc", OpPOpLoc)
	state := Init(sf, mp, false, nil, nil)
	state.PushInt(3)
	// Deliberately leave state.ActiveLoc = nil.

	err := Execute(state)
	if err == nil || err.Error() != "P_OPLOC: no active loc" {
		t.Errorf("expected 'P_OPLOC: no active loc', got %v", err)
	}
}

// TestPOpLocInvalidOpErrors verifies op=0 and op=6 both return an error.
func TestPOpLocInvalidOpErrors(t *testing.T) {
	for _, op := range []int32{0, 6, -1, 100} {
		mp := &mockPlayer{}
		loc := &mockActiveLoc{locType: 42}

		sf := &ScriptFile{
			Name:             "p_op_loc_invalid",
			Opcodes:          []Opcode{OpPushConstantInt, OpPOpLoc, OpReturn},
			IntOperands:      []int32{op, 0, 0},
			StringOperands:   []string{"", "", ""},
			InstructionCount: 3,
		}
		state := Init(sf, mp, false, nil, nil)
		state.ActiveLoc = loc
		state.Pointers |= PtrActiveLoc

		err := Execute(state)
		if err == nil {
			t.Errorf("op=%d: expected error, got nil", op)
			continue
		}
		wantPrefix := "P_OPLOC: invalid op"
		if len(err.Error()) < len(wantPrefix) || err.Error()[:len(wantPrefix)] != wantPrefix {
			t.Errorf("op=%d: expected error starting with %q, got %v", op, wantPrefix, err)
		}
	}
}

// TestPOpNpcAnchorsOnActiveNpc — symmetric happy path.
func TestPOpNpcAnchorsOnActiveNpc(t *testing.T) {
	mp := &mockPlayer{}
	npc := &mockActiveNpc{typeId: 7}

	sf := &ScriptFile{
		Name:             "p_op_npc",
		Opcodes:          []Opcode{OpPushConstantInt, OpPOpNpc, OpReturn},
		IntOperands:      []int32{2, 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	state := Init(sf, mp, false, nil, nil)
	state.ActiveNpc = npc
	state.Pointers |= PtrActiveNpc
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(mp.lastSetInteractionScriptNpc) != 1 {
		t.Fatalf("expected 1 SetInteractionScriptNpc call, got %d", len(mp.lastSetInteractionScriptNpc))
	}
	got := mp.lastSetInteractionScriptNpc[0]
	if got.Npc != npc || got.Op != 2 {
		t.Errorf("args: got %+v, want {Npc:%p, Op:2}", got, npc)
	}
}

// TestPOpNpcInvalidOpErrors — symmetric op-range check.
func TestPOpNpcInvalidOpErrors(t *testing.T) {
	for _, op := range []int32{0, 6} {
		mp := &mockPlayer{}
		npc := &mockActiveNpc{typeId: 7}

		sf := &ScriptFile{
			Name:             "p_op_npc_invalid",
			Opcodes:          []Opcode{OpPushConstantInt, OpPOpNpc, OpReturn},
			IntOperands:      []int32{op, 0, 0},
			StringOperands:   []string{"", "", ""},
			InstructionCount: 3,
		}
		state := Init(sf, mp, false, nil, nil)
		state.ActiveNpc = npc
		state.Pointers |= PtrActiveNpc

		err := Execute(state)
		if err == nil {
			t.Errorf("op=%d: expected error, got nil", op)
		}
	}
}
```

**Important:** `mockActiveNpc` must exist in the test package. If it doesn't, add it to `handlers_player_test.go` alongside `mockActiveLoc`:

```go
type mockActiveNpc struct {
	typeId, x, z, level int
	stats               [8]int
}

func (m *mockActiveNpc) NpcType() int                    { return m.typeId }
func (m *mockActiveNpc) NpcX() int                       { return m.x }
func (m *mockActiveNpc) NpcZ() int                       { return m.z }
func (m *mockActiveNpc) NpcLevel() int                   { return m.level }
func (m *mockActiveNpc) NpcStat(stat int) int            { return m.stats[stat] }
// ... any remaining methods from ActiveNpc interface — read pkg/script/active.go
```

Read `pkg/script/active.go` from line 294 onward to enumerate all methods on `ActiveNpc` and implement stubs for any missing.

- [ ] **Step 5: Build + run tests to confirm red phase.**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run 'TestPOp' -v`
Expected: FAIL across all 6 — some build-level (handlers/interface methods undefined) and some runtime (unimplemented opcode).

- [ ] **Step 6: Extend `ActivePlayer` interface in `pkg/script/active.go`.**

Find a natural spot (end of interface or near StopAction/ClearPendingAction). Add:

```go
// S6v: p_op* script-queued interaction methods.

// SetInteractionScriptLoc anchors the player on `loc` with trigger
// ApLoc<op> as a script-queued interaction (TS Interaction.SCRIPT).
// op is 1-indexed (1..5). Matches TS PlayerOps.ts:386-402 terminal
// setInteraction call.
//
// Implementations must type-assert the narrow ActiveLoc interface to
// their concrete loc type. Caller pre-validates op ∈ [1,5].
SetInteractionScriptLoc(loc ActiveLoc, op int)

// SetInteractionScriptNpc anchors the player on `npc` with trigger
// ApNpc<op> as a script-queued interaction. Matches TS
// PlayerOps.ts:404-415.
SetInteractionScriptNpc(npc ActiveNpc, op int)
```

- [ ] **Step 7: Add handlers in `pkg/script/handlers_player.go`.**

Append at EOF (or after the last existing handler; handlers_player.go hosts the player-mutating ops):

```go
// -- p_op* script-queued interaction anchoring (S6v) --------------------

// handleP_OpLoc (P_OPLOC, opcode 2077) re-anchors the active player on
// the active loc with AP trigger APLOC<op>. Matches TS
// PlayerOps.ts:386-402.
//
// DEVIATION S6v-D1: TS wraps this in checkedHandler(ProtectedActivePlayer);
// goscape uses requireActivePlayer until a ProtectedActivePlayer gate
// sub-spec lands.
func handleP_OpLoc(s *ScriptState) error {
	if err := requireActivePlayer(s, "P_OPLOC"); err != nil {
		return err
	}
	if s.ActiveLoc == nil {
		return errors.New("P_OPLOC: no active loc")
	}
	op := s.PopInt()
	if op < 1 || op > 5 {
		return fmt.Errorf("P_OPLOC: invalid op %d (must be 1..5)", op)
	}
	s.Self.StopAction()
	s.Self.SetInteractionScriptLoc(s.ActiveLoc, op)
	return nil
}

// handleP_OpNpc (P_OPNPC, opcode 2078) re-anchors on the active npc.
// Matches TS PlayerOps.ts:404-415. DEVIATION S6v-D1 applies (see above).
func handleP_OpNpc(s *ScriptState) error {
	if err := requireActivePlayer(s, "P_OPNPC"); err != nil {
		return err
	}
	if s.ActiveNpc == nil {
		return errors.New("P_OPNPC: no active npc")
	}
	op := s.PopInt()
	if op < 1 || op > 5 {
		return fmt.Errorf("P_OPNPC: invalid op %d (must be 1..5)", op)
	}
	s.Self.StopAction()
	s.Self.SetInteractionScriptNpc(s.ActiveNpc, op)
	return nil
}
```

**Imports:** `handlers_player.go` already imports `errors`. Verify `fmt` is imported; if not, add it.

- [ ] **Step 8: Register handlers in `pkg/script/handlers.go`.**

Find an existing `OpP*:` entry (e.g., `OpPDelay`) and add near it:

```go
	OpPOpLoc: handleP_OpLoc,
	OpPOpNpc: handleP_OpNpc,
```

Match surrounding conventions for grouping (comment section headers etc.).

- [ ] **Step 9: Implement Player wrappers in `modules/world/player_script.go`.**

Append at EOF:

```go
// SetInteractionScriptLoc implements script.ActivePlayer. Type-asserts
// the narrow script.ActiveLoc back to *entity.Loc and anchors the
// player with trigger ApLoc<op> + InteractionScript. Matches TS
// PlayerOps.ts P_OPLOC. Silently no-ops if the loc isn't a real
// *entity.Loc (defensive).
func (p *Player) SetInteractionScriptLoc(loc script.ActiveLoc, op int) {
	realLoc, ok := loc.(*entitypkg.Loc)
	if !ok {
		return
	}
	p.SetInteraction(InteractionScript, realLoc, op, -1)
}

// SetInteractionScriptNpc implements script.ActivePlayer.
func (p *Player) SetInteractionScriptNpc(npc script.ActiveNpc, op int) {
	realNpc, ok := npc.(*Npc)
	if !ok {
		return
	}
	p.SetInteraction(InteractionScript, realNpc, op, -1)
}
```

Check existing imports in `player_script.go` — `entitypkg` is the existing alias for `github.com/zsrv/goscape/pkg/entity`. If the alias is different, use the one already in the file.

- [ ] **Step 10: Build.**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`
Expected: PASS. Both mockPlayer (via step 2) and Player (via step 9) implement the extended ActivePlayer.

- [ ] **Step 11: Run the 6 new tests.**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run 'TestPOp' -v`
Expected: PASS × 6.

- [ ] **Step 12: Full repo tests + vet + race.**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`
Expected: baseline + 6 PASS.

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...`
Expected: clean.

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/...`
Expected: PASS.

- [ ] **Step 13: Commit.**

```bash
git add pkg/script/active.go \
        pkg/script/handlers_player.go \
        pkg/script/handlers.go \
        pkg/script/runner_test.go \
        pkg/script/handlers_player_test.go \
        modules/world/player_script.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script,world): p_op_loc / p_op_npc opcodes — partial closure of S6l-D5 (S6v)

Wire two p_op* re-anchor opcodes from TS PlayerOps.ts:386-415:
  P_OPLOC (2077) — p_op_loc(op) → anchors on active_loc with
                   AP trigger APLOC<op>
  P_OPNPC (2078) — p_op_npc(op) → anchors on active_npc with
                   AP trigger APNPC<op>

Scripts can now initiate engine-driven interactions programmatically
(e.g., a dialog-end handler firing p_op_loc(3) to auto-start a
chop-tree sequence).

Two new ActivePlayer methods:
  SetInteractionScriptLoc(loc ActiveLoc, op int)
  SetInteractionScriptNpc(npc ActiveNpc, op int)

Player wrappers in modules/world/player_script.go type-assert the
narrow script interface back to *entity.Loc / *world.Npc and call
SetInteraction(InteractionScript, ...). Silent no-op on type-assert
failure (defensive guard).

DEVIATION S6v-D1 (new): ProtectedActivePlayer gate deferred. TS wraps
p_op* in checkedHandler(ProtectedActivePlayer) which refuses dispatch
when the script wasn't started with protection. Goscape uses the
simpler requireActivePlayer as interim gate. Follow-up: dedicated
sub-spec adding PtrProtectedActivePlayer bit (was S6l-D3).

Scope boundaries (not deviations):
  - Queue-waypoint step omitted from P_OPLOC (goscape's
    processInteraction paths on next tick; observable behavior
    equivalent)
  - P_OPHELD unwired (TS throws unimplemented)
  - P_OPNPCT / P_OPOBJ / P_OPPLAYER(T) deferred pending spell / obj /
    active_player2 infrastructure

Tests: 6 new (happy-path + no-active-player + no-active-entity + invalid-op
range per opcode). mockPlayer extended with call-recording methods.
mockActiveLoc and mockActiveNpc added for ScriptState pointer tests.

Partial closure of S6l-D5. 1 new deviation (S6v-D1). Zero regressions.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

## Self-review checklist

- All 6 tests present and passing?
- `requireActivePlayer` error strings match `"P_OPLOC: no active player"` / `"P_OPNPC: no active player"` exactly?
- Both Player wrappers use `InteractionScript` (not `InteractionEngine`)?
- No drive-by changes to unrelated files?
- mockActiveLoc/mockActiveNpc implementations satisfy the full interface (no "X does not implement Y" errors)?
