# NAI-160 — Trivial-handler sweep #2 (7 ops) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port 7 single-statement RuneScript opcode handlers — `SAY`, `HEADICONS_GET`, `HEADICONS_SET`, `P_EXACTMOVE`, `INV_ALLSTOCK`, `NPC_ATTACKRANGE`, `NPC_INRANGE` — closing the top user-hot entry in the 28-opcode unhandled-handler tail (`say` @ 126 content callers) and bundling 6 sibling 1-3-line ports.

**Architecture:** Each handler is a 1-3 line port from `LostCityRS/Engine-TS` per the NAI-149 trivial-sweep template. Surface additions: 5 new `ActivePlayer` methods (`Say`, `HeadIcons`, `SetHeadIcons`, `ExactMove`, `UnsetMapFlag`), 1 new `ActiveNpc` method (`TargetWithinMaxRange`), 3 new `*Player` adapter methods, 1 new `*Npc` adapter method, and corresponding test-mock recorders. Every handler dispatches through the existing `handlers` map in `pkg/script/handlers.go`. Each task is one opcode, TDD-shaped: failing test → minimal impl → pass → commit.

**Tech Stack:** Go 1.26+ (`go_version.md`). All `go` commands prefix `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache` per global CLAUDE.md.

**Spec:** `docs/superpowers/specs/2026-05-11-nai-160-trivial-handler-sweep-2-design.md` @ `355363d`.

**Cadence pointers:**
- `execution_mode_default.md` — subagent-driven-development, one subagent per task, two-stage Sonnet review at end-of-impl.
- `superpowers_clear_between_spec_and_impl.md` — `/clear` between this plan and impl (resume prompt at bottom).
- `superpowers_code_reviewer_model.md` — single `superpowers:code-reviewer` agent on Sonnet at end-of-impl, NOT per task.
- `close_commit_memory_trailer.md` — final close commit cites memory hits per spec §9.

---

## File Structure

**Created:** none — every new method/handler lives in an existing file.

**Modified:**

| File | What changes |
|---|---|
| `pkg/script/active.go` | +6 method declarations on `ActivePlayer` / `ActiveNpc` interfaces |
| `pkg/script/handlers.go` | +7 lines registering new handlers in the `handlers` map |
| `pkg/script/handlers_player.go` | +4 new handler funcs (`handleSay`, `handleHeadIconsGet`, `handleHeadIconsSet`, `handlePExactMove`) |
| `pkg/script/handlers_inv.go` | +1 new handler func (`handleInvAllStock`) |
| `pkg/script/handlers_npc.go` | +2 new handler funcs (`handleNpcAttackRange`, `handleNpcInRange`) |
| `pkg/script/runner_test.go` | +5 `mockPlayer` recorder fields + 5 recorder methods (Say, HeadIcons, SetHeadIcons, ExactMove, UnsetMapFlag) |
| `pkg/script/handlers_npc_test.go` | +1 `mockNpc` recorder field + 1 method (TargetWithinMaxRange) + new positive/negative tests for NPC_ATTACKRANGE / NPC_INRANGE |
| `pkg/script/handlers_player_test.go` | new positive/negative tests for SAY / HEADICONS_GET / HEADICONS_SET / P_EXACTMOVE; extend `TestHandlersRequireActivePlayer` cases table |
| `pkg/script/handlers_inv_test.go` | new positive/negative tests for INV_ALLSTOCK |
| `modules/world/player_masks.go` | +3 new `*Player` methods (`HeadIcons`, `SetHeadIcons`, `UnsetMapFlag`) |
| `modules/world/npc_interaction.go` | +1 new exported `*Npc` method (`TargetWithinMaxRange`) wrapping the existing unexported `targetWithinMaxRange` |

**Why these locations:**
- `player_masks.go` already contains `Say`, `ExactMove`, `FaceCoord` (mask-flagging player ops). The new `HeadIcons`/`SetHeadIcons` are not mask ops but read/write the same `Player` struct; placing them adjacent keeps player-state surfaces co-located. `UnsetMapFlag` is co-located with the other one-shot player intents.
- `npc_interaction.go` already houses `targetWithinMaxRange` and the AP/maxrange logic; the export wrapper goes immediately after the existing method.

---

## Pre-impl preflight (controller, before T1)

Before dispatching T1, controller agent verifies the spec premises against HEAD (per `controller_preflight.md`):

- [ ] `git rev-parse HEAD` matches the SHA in the spec header (`b0d576e`) — if a newer commit landed, re-run `missing_handler_audit.md` one-liner to confirm the 7 target opcodes are still in the unhandled set.
- [ ] `grep -n "OpSay\|OpHeadIconsGet\|OpHeadIconsSet\|OpPExactMove\|OpInvAllStock\|OpNpcAttackRange\|OpNpcInRange" pkg/script/handlers.go` returns zero matches (none are registered yet).
- [ ] `grep -n "func (p \*Player) Say\b\|func (p \*Player) ExactMove\b" modules/world/player_masks.go` confirms the `*Player.Say` and `*Player.ExactMove` methods exist at the cited line ranges (player_masks.go:8 / :28).
- [ ] `grep -n "func (n \*Npc) targetWithinMaxRange\b" modules/world/npc_interaction.go` confirms the unexported method exists at the cited line (:591).
- [ ] `grep -n "headicons\s*int\b" modules/world/player.go` confirms the `headicons int` field at :209.
- [ ] `grep -n "sendUnsetMapFlag\b" modules/world/handler_oploc.go` confirms the package-local helper is callable from `(*Player).UnsetMapFlag`.

If any premise has drifted, controller halts and reports to user before dispatching T1.

---

## Task 1: `SAY` (OpSay, opcode 2097)

**Files:**
- Modify: `pkg/script/active.go` (add `Say(text []byte)` to `ActivePlayer` interface)
- Modify: `pkg/script/handlers_player.go` (add `handleSay`)
- Modify: `pkg/script/handlers.go` (register `OpSay`)
- Modify: `pkg/script/runner_test.go` (add `sayCalls [][]byte` field + `Say([]byte)` method on `mockPlayer`)
- Test: `pkg/script/handlers_player_test.go` (add `TestSay`, `TestSayEmptyString`, and extend `TestHandlersRequireActivePlayer` cases table to include `{"SAY", OpSay}`)

**TS source:** `LostCityRS/Engine-TS/src/engine/script/handlers/PlayerOps.ts:462-464`:
```ts
[ScriptOpcode.SAY]: checkedHandler(ActivePlayer, state => {
    state.activePlayer.say(state.popString());
}),
```

**Preflight grep (subagent runs first):**
- `grep -n "type ActivePlayer interface\|^}" pkg/script/active.go | head -10` → confirms ActivePlayer is at the top of the file (~line 6).
- `grep -n "sayCalls\|type mockPlayer struct" pkg/script/runner_test.go | head -5` → confirms `mockPlayer` shape; verify `sayCalls` field does NOT already exist on `mockPlayer` (it does exist on `mockNpc` at handlers_npc_test.go:218; the two structs are independent).
- `grep -n "TestHandlersRequireActivePlayer\b" pkg/script/handlers_player_test.go` → confirms the existing table-test at :831 to extend.

---

- [ ] **Step 1: Write the failing positive test**

Append to `pkg/script/handlers_player_test.go` (after the existing P_TELEPORT / P_TELEJUMP cluster, alphabetical-by-handler-name slot):

```go
// TestSay pins OpSay's body: pop the top-of-stack string and pass it to
// ActivePlayer.Say as a []byte. Mirrors TS PlayerOps.ts:462-464.
// NAI-160 T1.
func TestSay(t *testing.T) {
	mp := &mockPlayer{}
	sf := &ScriptFile{
		Name:             "[say,test]",
		Opcodes:          []Opcode{OpPushConstantString, OpSay, OpReturn},
		IntOperands:      []int32{0, 0, 0},
		StringOperands:   []string{"hello world", "", ""},
		InstructionCount: 3,
	}
	state := Init(sf, mp, false, nil, nil)
	state.Pointers |= PtrActivePlayer
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := len(mp.sayCalls); got != 1 {
		t.Fatalf("sayCalls: got %d, want 1", got)
	}
	if got, want := string(mp.sayCalls[0]), "hello world"; got != want {
		t.Errorf("sayCalls[0]: got %q, want %q", got, want)
	}
}

// TestSayEmptyString pins TS semantics that an empty bubble is legal —
// matches the doc-comment at modules/world/player_masks.go:8-11 and
// the parallel TestNpcSay convention. NAI-160 T1.
func TestSayEmptyString(t *testing.T) {
	mp := &mockPlayer{}
	sf := &ScriptFile{
		Name:             "[say_empty,test]",
		Opcodes:          []Opcode{OpPushConstantString, OpSay, OpReturn},
		IntOperands:      []int32{0, 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	state := Init(sf, mp, false, nil, nil)
	state.Pointers |= PtrActivePlayer
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := len(mp.sayCalls); got != 1 {
		t.Fatalf("sayCalls: got %d, want 1", got)
	}
	if got := len(mp.sayCalls[0]); got != 0 {
		t.Errorf("sayCalls[0]: got len=%d, want 0", got)
	}
}
```

Also extend the `TestHandlersRequireActivePlayer` cases table at line ~836 to add:

```go
		{"SAY", OpSay},
```

(Alphabetical order; insert between RUNANIM and STAT or wherever it fits the current ordering — read the table first.)

- [ ] **Step 2: Run test to verify it fails**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run 'TestSay$|TestSayEmptyString|TestHandlersRequireActivePlayer$' -v
```

Expected: compile errors — `mp.sayCalls undefined`, `Self.Say undefined`, `OpSay handler not registered` (the dispatch path emits `script execute error … no handler for SAY (opcode 2097)`).

- [ ] **Step 3: Add `Say(text []byte)` to ActivePlayer interface**

Edit `pkg/script/active.go`. Find the `ActivePlayer` interface block (starts at line ~6, `type ActivePlayer interface {`). Add — group near other player-output methods (e.g. near existing `MessageGame` or `PlayAnim`):

```go
	// Say buffers `text` as the player's speech bubble for the current
	// tick, flagging MaskSay so the player-info encoder emits it. Empty
	// text is allowed (produces an empty bubble that clears itself next
	// tick via ResetMasks). Mirrors TS Player.say at Player.ts:1893-1896
	// (this.sayMessage = message; this.masks |= PlayerInfoProt.SAY).
	// NAI-160 T1.
	Say(text []byte)
```

- [ ] **Step 4: Add `sayCalls` recorder to mockPlayer**

Edit `pkg/script/runner_test.go`. Find the `type mockPlayer struct` block (line 99). Add field (group with other recorder-call slices; "NAI-160" comment per convention seen at neighbors):

```go
	// NAI-160 T1: SAY recorder. Defensive-copies the byte slice on Say()
	// to immunize from caller-mutates-buffer after the call.
	sayCalls [][]byte
```

Add method (anywhere after the struct; group near other recorder methods like `MessageGame`):

```go
// Say records the byte slice passed by handleSay. Mirrors the mockNpc.Say
// recorder at handlers_npc_test.go:328-330. NAI-160 T1.
func (m *mockPlayer) Say(text []byte) {
	m.sayCalls = append(m.sayCalls, append([]byte(nil), text...))
}
```

- [ ] **Step 5: Add `handleSay`**

Edit `pkg/script/handlers_player.go`. Append (group near other ActivePlayer-only-output handlers; alphabetical by handler name in the existing file works):

```go
// handleSay implements OpSay (TS SAY at PlayerOps.ts:462-464).
// Mirrors `state.activePlayer.say(state.popString())` —
// checkedHandler(ActivePlayer, ...). NAI-160 T1.
func handleSay(s *ScriptState) error {
	if err := requireActivePlayer(s, "SAY"); err != nil {
		return err
	}
	text := s.PopString()
	s.Self.Say([]byte(text))
	return nil
}
```

- [ ] **Step 6: Register the handler**

Edit `pkg/script/handlers.go`. Add to the `handlers` map (group with other player-output ops; a clear slot is alongside the NAI-149 `OpPlayerMember` cluster at line ~137):

```go
	// NAI-160 T1: SAY.
	OpSay: handleSay,
```

- [ ] **Step 7: Run tests to verify they pass**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run 'TestSay$|TestSayEmptyString|TestHandlersRequireActivePlayer$' -v
```

Expected: all 3 PASS. Then run the full pkg/script test suite to catch any unintended regressions from the interface addition:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/...
```

Expected: PASS.

Also confirm the world module still compiles (the `var _ script.ActivePlayer = (*Player)(nil)` assertion at `modules/world/message_game.go:11` must still hold — `(*Player).Say([]byte)` already exists at `player_masks.go:8`):

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./modules/world/...
```

Expected: clean build, no errors.

- [ ] **Step 8: Commit**

```bash
git add pkg/script/active.go pkg/script/handlers.go pkg/script/handlers_player.go pkg/script/runner_test.go pkg/script/handlers_player_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-160 T1 — SAY handler (opcode 2097)

Ports TS PlayerOps.ts:462-464 `state.activePlayer.say(state.popString())`.
Adds ActivePlayer.Say(text []byte); (*Player).Say already exists at
modules/world/player_masks.go:8-11 (mirrors TS Player.ts:1893-1896
two-field write + MaskSay).

Closes top user-hot entry in the 28-opcode unhandled tail (126 content
callers in LostCityRS/Content/scripts).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: `HEADICONS_GET` (OpHeadIconsGet, opcode 2024)

**Files:**
- Modify: `pkg/script/active.go` (add `HeadIcons() int`)
- Modify: `pkg/script/handlers_player.go` (add `handleHeadIconsGet`)
- Modify: `pkg/script/handlers.go` (register `OpHeadIconsGet`)
- Modify: `pkg/script/runner_test.go` (add `headiconsValue int` field + `HeadIcons() int` method on `mockPlayer`)
- Modify: `modules/world/player_masks.go` (add `(*Player).HeadIcons() int`)
- Test: `pkg/script/handlers_player_test.go` (add `TestHeadIconsGet` and extend require-active table)

**TS source:** `PlayerOps.ts:980-982`:
```ts
[ScriptOpcode.HEADICONS_GET]: state => {
    state.pushInt(state.activePlayer.headicons);
},
```

(TS does NOT wrap this in `checkedHandler(ActivePlayer, …)` — but it dereferences `state.activePlayer`, so reaching the body without a player would throw at the read. Goscape ports this with a `requireActivePlayer` guard so the failure mode is the consistent "no active player" error instead of a nil deref panic. Per `defensive_gate_doc_comment_label.md`, this is a goscape-side guard — doc-comment labels it `(goscape defensive; TS deref-panics)`.)

---

- [ ] **Step 1: Write the failing positive test**

Append to `pkg/script/handlers_player_test.go`:

```go
// TestHeadIconsGet pins OpHeadIconsGet's body: read the player's headicons
// field and push it as an int. Mirrors TS PlayerOps.ts:980-982. NAI-160 T2.
func TestHeadIconsGet(t *testing.T) {
	mp := &mockPlayer{headiconsValue: 7}
	sf := &ScriptFile{
		Name:             "[headicons_get,test]",
		Opcodes:          []Opcode{OpHeadIconsGet, OpReturn},
		IntOperands:      []int32{0, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	state := Init(sf, mp, false, nil, nil)
	state.Pointers |= PtrActivePlayer
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := state.PopInt(); got != 7 {
		t.Errorf("HEADICONS_GET: got %d, want 7", got)
	}
}
```

Extend `TestHandlersRequireActivePlayer` cases:

```go
		{"HEADICONS_GET", OpHeadIconsGet},
```

- [ ] **Step 2: Run test to verify it fails**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run 'TestHeadIconsGet$|TestHandlersRequireActivePlayer$' -v
```

Expected: compile errors (`headiconsValue` undefined, `Self.HeadIcons` undefined, handler not registered).

- [ ] **Step 3: Add `HeadIcons() int` to ActivePlayer interface**

Edit `pkg/script/active.go`. Append in the `ActivePlayer` block (near other readable player-config getters):

```go
	// HeadIcons returns the player's current head-icon bitmask. Mirrors
	// TS Player.headicons (default 0) at Player.ts:314 / PlayerOps.ts:981.
	// NAI-160 T2.
	HeadIcons() int

	// SetHeadIcons writes `v` into the head-icon bitmask. Caller is
	// responsible for NumberNotNull validation (handler calls checkNotNull
	// before invoking). Mirrors TS direct assignment at PlayerOps.ts:985.
	// NAI-160 T3.
	SetHeadIcons(v int)
```

(Declare both at once for cohesion; only HeadIcons is exercised in T2's test, SetHeadIcons is wired in T3.)

- [ ] **Step 4: Add `headiconsValue` + `HeadIcons` recorder to mockPlayer**

Edit `pkg/script/runner_test.go`. Add field to `mockPlayer` struct:

```go
	// NAI-160 T2/T3: HEADICONS_GET / HEADICONS_SET recorders.
	headiconsValue       int
	setHeadIconsCalls    []int
```

Add methods:

```go
// HeadIcons returns the seeded headiconsValue. NAI-160 T2.
func (m *mockPlayer) HeadIcons() int { return m.headiconsValue }

// SetHeadIcons records the write AND updates headiconsValue so a
// subsequent HeadIcons() read returns the new value (mirrors TS direct
// field assignment). NAI-160 T3.
func (m *mockPlayer) SetHeadIcons(v int) {
	m.setHeadIconsCalls = append(m.setHeadIconsCalls, v)
	m.headiconsValue = v
}
```

- [ ] **Step 5: Add `(*Player).HeadIcons` and `(*Player).SetHeadIcons`**

Edit `modules/world/player_masks.go`. Append (these are not mask ops; group at the bottom of the file with a section comment):

```go
// HeadIcons / SetHeadIcons expose the headicons field for the
// HEADICONS_GET / HEADICONS_SET RuneScript handlers. Mirrors TS direct
// read/write at PlayerOps.ts:980-986. The encoder at
// modules/world/appearance.go:65 (`buf.P1(uint8(p.headicons))`) does
// byte-truncation downstream, matching TS Player.ts:1314
// `stream.p1(this.headicons)`. NAI-160 T2/T3.
func (p *Player) HeadIcons() int { return p.headicons }

// SetHeadIcons writes the validated head-icon bitmask. NumberNotNull
// gating is the handler's responsibility (handleHeadIconsSet at
// pkg/script/handlers_player.go).
func (p *Player) SetHeadIcons(v int) { p.headicons = v }
```

- [ ] **Step 6: Add `handleHeadIconsGet`**

Edit `pkg/script/handlers_player.go`. Append:

```go
// handleHeadIconsGet implements OpHeadIconsGet (TS HEADICONS_GET at
// PlayerOps.ts:980-982). Pushes the player's headicons bitmask.
//
// goscape defensive: requireActivePlayer guard fronts the dereference;
// TS deref-panics on a nil activePlayer (no checkedHandler wrap).
// NAI-160 T2.
func handleHeadIconsGet(s *ScriptState) error {
	if err := requireActivePlayer(s, "HEADICONS_GET"); err != nil {
		return err
	}
	s.PushInt(s.Self.HeadIcons())
	return nil
}
```

- [ ] **Step 7: Register the handler**

Edit `pkg/script/handlers.go`. Add to the `handlers` map:

```go
	// NAI-160 T2: HEADICONS_GET.
	OpHeadIconsGet: handleHeadIconsGet,
```

- [ ] **Step 8: Run tests to verify they pass**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run 'TestHeadIconsGet$|TestHandlersRequireActivePlayer$' -v
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
```

Expected: PASS + clean build (the world module must compile because `*Player` now has the new methods even though T3's handler hasn't landed yet).

- [ ] **Step 9: Commit**

```bash
git add pkg/script/active.go pkg/script/handlers.go pkg/script/handlers_player.go pkg/script/runner_test.go pkg/script/handlers_player_test.go modules/world/player_masks.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-160 T2 — HEADICONS_GET handler (opcode 2024)

Ports TS PlayerOps.ts:980-982 `state.pushInt(state.activePlayer.headicons)`.
Adds ActivePlayer.HeadIcons/SetHeadIcons (SetHeadIcons wired in T3) and
backing (*Player) readers/writers at modules/world/player_masks.go.

requireActivePlayer guard fronts the dereference per
defensive_gate_doc_comment_label.md (TS deref-panics; goscape returns
an "OPNAME: no active player" error).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: `HEADICONS_SET` (OpHeadIconsSet, opcode 2025)

**Files:**
- Modify: `pkg/script/handlers_player.go` (add `handleHeadIconsSet`)
- Modify: `pkg/script/handlers.go` (register `OpHeadIconsSet`)
- Test: `pkg/script/handlers_player_test.go` (add `TestHeadIconsSet`, `TestHeadIconsSetRejectsNull`, extend require-active table)

(No surface changes — `SetHeadIcons` already added in T2.)

**TS source:** `PlayerOps.ts:984-986`:
```ts
[ScriptOpcode.HEADICONS_SET]: state => {
    state.activePlayer.headicons = check(state.popInt(), NumberNotNull);
},
```

---

- [ ] **Step 1: Write the failing tests**

Append to `pkg/script/handlers_player_test.go`:

```go
// TestHeadIconsSet pins OpHeadIconsSet's body: pop an int, NumberNotNull
// check, write into headicons. Mirrors TS PlayerOps.ts:984-986. NAI-160 T3.
func TestHeadIconsSet(t *testing.T) {
	mp := &mockPlayer{}
	sf := &ScriptFile{
		Name:             "[headicons_set,test]",
		Opcodes:          []Opcode{OpPushConstantInt, OpHeadIconsSet, OpReturn},
		IntOperands:      []int32{42, 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	state := Init(sf, mp, false, nil, nil)
	state.Pointers |= PtrActivePlayer
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := len(mp.setHeadIconsCalls); got != 1 {
		t.Fatalf("setHeadIconsCalls: got %d, want 1", got)
	}
	if got := mp.setHeadIconsCalls[0]; got != 42 {
		t.Errorf("setHeadIconsCalls[0]: got %d, want 42", got)
	}
	if got := mp.headiconsValue; got != 42 {
		t.Errorf("headiconsValue post-set: got %d, want 42", got)
	}
}

// TestHeadIconsSetRejectsNull pins the NumberNotNull check (goscape
// checkNotNull rejects -1; matches TS NumberNotNull). NAI-160 T3.
func TestHeadIconsSetRejectsNull(t *testing.T) {
	mp := &mockPlayer{}
	sf := &ScriptFile{
		Name:             "[headicons_set_null,test]",
		Opcodes:          []Opcode{OpPushConstantInt, OpHeadIconsSet, OpReturn},
		IntOperands:      []int32{-1, 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	state := Init(sf, mp, false, nil, nil)
	state.Pointers |= PtrActivePlayer
	err := Execute(state)
	if err == nil {
		t.Fatalf("Execute: got nil err, want HEADICONS_SET: input number was null(-1)")
	}
	if got := err.Error(); !strings.Contains(got, "HEADICONS_SET: input number was null(-1)") {
		t.Errorf("err: got %q, want substring 'HEADICONS_SET: input number was null(-1)'", got)
	}
	if got := len(mp.setHeadIconsCalls); got != 0 {
		t.Errorf("setHeadIconsCalls: got %d, want 0 (write must NOT happen on validation failure)", got)
	}
}
```

Extend `TestHandlersRequireActivePlayer`:

```go
		{"HEADICONS_SET", OpHeadIconsSet},
```

(Add `strings` import if not already present; check the existing imports in `handlers_player_test.go`.)

- [ ] **Step 2: Run tests to verify they fail**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run 'TestHeadIconsSet|TestHandlersRequireActivePlayer$' -v
```

Expected: handler-not-registered errors.

- [ ] **Step 3: Add `handleHeadIconsSet`**

Edit `pkg/script/handlers_player.go`. Append:

```go
// handleHeadIconsSet implements OpHeadIconsSet (TS HEADICONS_SET at
// PlayerOps.ts:984-986). Pops an int, checks NumberNotNull, writes into
// the player's headicons bitmask.
//
// goscape defensive: requireActivePlayer guard (TS deref-panics).
// Order is pop → check → set so a failed gate leaves headicons untouched
// (covered by TestHeadIconsSetRejectsNull). NAI-160 T3.
func handleHeadIconsSet(s *ScriptState) error {
	if err := requireActivePlayer(s, "HEADICONS_SET"); err != nil {
		return err
	}
	v := s.PopInt()
	if err := checkNotNull(v, "HEADICONS_SET"); err != nil {
		return err
	}
	s.Self.SetHeadIcons(v)
	return nil
}
```

- [ ] **Step 4: Register the handler**

Edit `pkg/script/handlers.go`:

```go
	// NAI-160 T3: HEADICONS_SET.
	OpHeadIconsSet: handleHeadIconsSet,
```

- [ ] **Step 5: Run tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run 'TestHeadIconsSet|TestHandlersRequireActivePlayer$' -v
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/...
```

Expected: all PASS.

- [ ] **Step 6: Commit**

```bash
git add pkg/script/handlers.go pkg/script/handlers_player.go pkg/script/handlers_player_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-160 T3 — HEADICONS_SET handler (opcode 2025)

Ports TS PlayerOps.ts:984-986 — pop int, checkNotNull, write headicons.
Backing surface (ActivePlayer.SetHeadIcons, (*Player).SetHeadIcons,
mockPlayer.SetHeadIcons) landed in T2.

Validation order: pop → check → set, so a failed NumberNotNull gate
leaves headicons untouched (pinned by TestHeadIconsSetRejectsNull).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: `P_EXACTMOVE` (OpPExactMove, opcode 2072)

**Files:**
- Modify: `pkg/script/active.go` (add `ExactMove(sX, sZ, eX, eZ, begin, finish, dir int)` + `UnsetMapFlag()`)
- Modify: `pkg/script/handlers_player.go` (add `handlePExactMove`)
- Modify: `pkg/script/handlers.go` (register `OpPExactMove`)
- Modify: `pkg/script/runner_test.go` (add `exactMoveCalls` + `unsetMapFlagCalls` recorders + methods)
- Modify: `modules/world/player_masks.go` (add `(*Player).UnsetMapFlag`)
- Test: `pkg/script/handlers_player_test.go` (add `TestPExactMove`, `TestPExactMoveRequiresProtected`, `TestPExactMoveInvalidCoord`; extend require-active table)

**TS source:** `PlayerOps.ts:881-890`:
```ts
[ScriptOpcode.P_EXACTMOVE]: checkedHandler(ProtectedActivePlayer, state => {
    const [start, end, startCycle, endCycle, direction] = state.popInts(5);

    const startPos: CoordGrid = check(start, CoordValid);
    const endPos: CoordGrid = check(end, CoordValid);

    state.activePlayer.unsetMapFlag();
    state.activePlayer.exactMove(startPos.x, startPos.z, endPos.x, endPos.z, startCycle, endCycle, direction);
}),
```

**Pop order (critical, per `handler_pop_order_test_masking.md`):** TS `popInts(5)` returns the array in push order — `direction` is top-of-stack, `start` was pushed first. Goscape pops top-first:

```
1st PopInt → direction
2nd PopInt → endCycle
3rd PopInt → startCycle
4th PopInt → endPacked
5th PopInt → startPacked
```

---

- [ ] **Step 1: Write the failing tests**

Append to `pkg/script/handlers_player_test.go` (uses `coordgrid.PackCoord` for valid packed coords — confirm via grep that the import path is `github.com/zsrv/goscape/pkg/coordgrid`):

```go
// TestPExactMove pins OpPExactMove's body: pop 5 ints (top-down: dir,
// endCycle, startCycle, end, start), unpack two coords via CoordValid,
// call UnsetMapFlag(), then ExactMove(sX, sZ, eX, eZ, begin, finish, dir).
// Mirrors TS PlayerOps.ts:881-890. NAI-160 T4.
//
// Per handler_pop_order_test_masking.md, the 5 push values are all
// distinct so a pop-order regression mis-binds at least one slot.
func TestPExactMove(t *testing.T) {
	mp := &mockPlayer{}
	startPacked := coordgrid.PackCoord(0, 3200, 3300)
	endPacked := coordgrid.PackCoord(0, 3205, 3308)
	sf := &ScriptFile{
		Name: "[p_exactmove,test]",
		Opcodes: []Opcode{
			OpPushConstantInt, OpPushConstantInt, OpPushConstantInt,
			OpPushConstantInt, OpPushConstantInt,
			OpPExactMove, OpReturn,
		},
		// Push order matches TS popInts(5) source order:
		// [start, end, startCycle, endCycle, direction]
		IntOperands:      []int32{int32(startPacked), int32(endPacked), 11, 22, 3, 0, 0},
		StringOperands:   []string{"", "", "", "", "", "", ""},
		InstructionCount: 7,
	}
	state := Init(sf, mp, false, nil, nil)
	state.Pointers |= PtrActivePlayer | PtrProtectedActivePlayer
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := mp.unsetMapFlagCalls; got != 1 {
		t.Errorf("unsetMapFlagCalls: got %d, want 1", got)
	}
	if got := len(mp.exactMoveCalls); got != 1 {
		t.Fatalf("exactMoveCalls: got %d, want 1", got)
	}
	c := mp.exactMoveCalls[0]
	if c.sX != 3200 || c.sZ != 3300 {
		t.Errorf("start coord: got (sX=%d, sZ=%d), want (3200, 3300)", c.sX, c.sZ)
	}
	if c.eX != 3205 || c.eZ != 3308 {
		t.Errorf("end coord: got (eX=%d, eZ=%d), want (3205, 3308)", c.eX, c.eZ)
	}
	if c.begin != 11 || c.finish != 22 || c.dir != 3 {
		t.Errorf("cycle/dir: got (begin=%d, finish=%d, dir=%d), want (11, 22, 3)",
			c.begin, c.finish, c.dir)
	}
}

// TestPExactMoveRequiresProtected pins the ProtectedActivePlayer gate.
// Mirrors TestPTeleportRequiresProtected at handlers_player_test.go (existing
// requireProtectedActivePlayer pattern). NAI-160 T4.
func TestPExactMoveRequiresProtected(t *testing.T) {
	mp := &mockPlayer{}
	startPacked := coordgrid.PackCoord(0, 3200, 3300)
	endPacked := coordgrid.PackCoord(0, 3205, 3308)
	sf := &ScriptFile{
		Name: "[p_exactmove_unprotected,test]",
		Opcodes: []Opcode{
			OpPushConstantInt, OpPushConstantInt, OpPushConstantInt,
			OpPushConstantInt, OpPushConstantInt,
			OpPExactMove, OpReturn,
		},
		IntOperands:      []int32{int32(startPacked), int32(endPacked), 11, 22, 3, 0, 0},
		StringOperands:   []string{"", "", "", "", "", "", ""},
		InstructionCount: 7,
	}
	state := Init(sf, mp, false, nil, nil)
	state.Pointers |= PtrActivePlayer // protect flag intentionally unset
	err := Execute(state)
	if err == nil {
		t.Fatalf("Execute: got nil err, want P_EXACTMOVE: script not protected")
	}
	if got := err.Error(); !strings.Contains(got, "P_EXACTMOVE: script not protected") {
		t.Errorf("err: got %q, want substring 'P_EXACTMOVE: script not protected'", got)
	}
	if got := mp.unsetMapFlagCalls; got != 0 {
		t.Errorf("unsetMapFlagCalls: got %d, want 0 (gate must fire before side effects)", got)
	}
}

// TestPExactMoveInvalidCoord pins checkCoord's reject of negative packed
// coords. NAI-160 T4.
func TestPExactMoveInvalidCoord(t *testing.T) {
	mp := &mockPlayer{}
	endPacked := coordgrid.PackCoord(0, 3205, 3308)
	sf := &ScriptFile{
		Name: "[p_exactmove_badcoord,test]",
		Opcodes: []Opcode{
			OpPushConstantInt, OpPushConstantInt, OpPushConstantInt,
			OpPushConstantInt, OpPushConstantInt,
			OpPExactMove, OpReturn,
		},
		// start = -1 (invalid packed coord)
		IntOperands:      []int32{-1, int32(endPacked), 11, 22, 3, 0, 0},
		StringOperands:   []string{"", "", "", "", "", "", ""},
		InstructionCount: 7,
	}
	state := Init(sf, mp, false, nil, nil)
	state.Pointers |= PtrActivePlayer | PtrProtectedActivePlayer
	err := Execute(state)
	if err == nil {
		t.Fatalf("Execute: got nil err, want P_EXACTMOVE: coord out of range (-1)")
	}
	if got := err.Error(); !strings.Contains(got, "P_EXACTMOVE: coord out of range (-1)") {
		t.Errorf("err: got %q, want substring 'P_EXACTMOVE: coord out of range (-1)'", got)
	}
	if got := mp.unsetMapFlagCalls; got != 0 {
		t.Errorf("unsetMapFlagCalls: got %d, want 0 (validation must precede side effects)", got)
	}
}
```

Extend `TestHandlersRequireActivePlayer`:

```go
		{"P_EXACTMOVE", OpPExactMove},
```

(Note: the require-active test has no pushed ints, so `PopInt` may zero-fill or panic — confirm the existing P_TELEPORT entry in the same table is OK with that. If the table-test pre-arms a stack, replicate for P_EXACTMOVE.)

Add `coordgrid` import to `handlers_player_test.go` if not already present.

- [ ] **Step 2: Run tests to verify they fail**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run 'TestPExactMove|TestHandlersRequireActivePlayer$' -v
```

Expected: compile errors (`exactMoveCalls`/`unsetMapFlagCalls` undefined, `Self.ExactMove`/`Self.UnsetMapFlag` undefined, handler not registered).

- [ ] **Step 3: Add `ExactMove` and `UnsetMapFlag` to ActivePlayer interface**

Edit `pkg/script/active.go`. Append in `ActivePlayer` block:

```go
	// ExactMove schedules an exact-movement animation: the player follows
	// a straight line from (sX, sZ) to (eX, eZ) between client ticks
	// `begin` and `finish`, facing direction `dir`. Mirrors TS
	// Player.exactMove (sets 7 fields + MaskExactMove). Backing impl at
	// modules/world/player_masks.go:28-37. NAI-160 T4.
	ExactMove(sX, sZ, eX, eZ, begin, finish, dir int)

	// UnsetMapFlag clears the player's map-click destination by sending
	// the matching client packet. Mirrors TS Player.unsetMapFlag — called
	// by P_EXACTMOVE (PlayerOps.ts:888) and adjacent server-script paths
	// that override a queued waypoint. NAI-160 T4.
	UnsetMapFlag()
```

- [ ] **Step 4: Add `(*Player).UnsetMapFlag`**

Edit `modules/world/player_masks.go`. Append (group with other one-shot intent methods near `Damage`):

```go
// UnsetMapFlag clears the player's map-click destination by sending the
// matching client packet. Mirrors TS Player.unsetMapFlag (called by
// P_EXACTMOVE at PlayerOps.ts:888 and by adjacent server-script paths).
// Thin wrapper over the package-local helper. NAI-160 T4.
func (p *Player) UnsetMapFlag() {
	sendUnsetMapFlag(p)
}
```

(Note: `(*Player).ExactMove` already exists at line 28-37 — no change needed.)

- [ ] **Step 5: Add `exactMoveCalls` + `unsetMapFlagCalls` to mockPlayer**

Edit `pkg/script/runner_test.go`. Add to the `mockPlayer` struct:

```go
	// NAI-160 T4: P_EXACTMOVE / UnsetMapFlag recorders.
	exactMoveCalls    []struct{ sX, sZ, eX, eZ, begin, finish, dir int }
	unsetMapFlagCalls int
```

Add methods:

```go
// ExactMove records the 7-arg call. NAI-160 T4.
func (m *mockPlayer) ExactMove(sX, sZ, eX, eZ, begin, finish, dir int) {
	m.exactMoveCalls = append(m.exactMoveCalls,
		struct{ sX, sZ, eX, eZ, begin, finish, dir int }{sX, sZ, eX, eZ, begin, finish, dir})
}

// UnsetMapFlag counts invocations. NAI-160 T4.
func (m *mockPlayer) UnsetMapFlag() { m.unsetMapFlagCalls++ }
```

- [ ] **Step 6: Add `handlePExactMove`**

Edit `pkg/script/handlers_player.go`. Append:

```go
// handlePExactMove implements OpPExactMove (TS P_EXACTMOVE at
// PlayerOps.ts:881-890). Pops 5 ints, validates two packed coords,
// clears the map-flag, then calls ExactMove with horizontal coords only
// (TS-faithful: the unpacked `level` component is discarded — deviation
// NAI-160-D-EXACTMOVE-COORDLEVEL-IGNORE per spec §3).
//
// Pop order: TS `state.popInts(5)` destructures
// [start, end, startCycle, endCycle, direction] from push order;
// direction is top-of-stack. Goscape pops top-first: dir → endCycle →
// startCycle → endPacked → startPacked. Critical per
// handler_pop_order_test_masking.md. NAI-160 T4.
func handlePExactMove(s *ScriptState) error {
	if err := requireProtectedActivePlayer(s, "P_EXACTMOVE"); err != nil {
		return err
	}
	direction := s.PopInt()
	endCycle := s.PopInt()
	startCycle := s.PopInt()
	endPacked := s.PopInt()
	startPacked := s.PopInt()
	_, sX, sZ, err := checkCoord(startPacked, "P_EXACTMOVE")
	if err != nil {
		return err
	}
	_, eX, eZ, err := checkCoord(endPacked, "P_EXACTMOVE")
	if err != nil {
		return err
	}
	s.Self.UnsetMapFlag()
	s.Self.ExactMove(sX, sZ, eX, eZ, startCycle, endCycle, direction)
	return nil
}
```

- [ ] **Step 7: Register the handler**

Edit `pkg/script/handlers.go`:

```go
	// NAI-160 T4: P_EXACTMOVE.
	OpPExactMove: handlePExactMove,
```

- [ ] **Step 8: Run tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run 'TestPExactMove|TestHandlersRequireActivePlayer$' -v
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
```

Expected: all PASS + clean build (world module compiles via new `(*Player).UnsetMapFlag`).

- [ ] **Step 9: Commit**

```bash
git add pkg/script/active.go pkg/script/handlers.go pkg/script/handlers_player.go pkg/script/runner_test.go pkg/script/handlers_player_test.go modules/world/player_masks.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-160 T4 — P_EXACTMOVE handler (opcode 2072)

Ports TS PlayerOps.ts:881-890 — pop 5 ints, validate two packed coords,
clear map-flag, call ExactMove(sX, sZ, eX, eZ, begin, finish, dir).

Adds ActivePlayer.ExactMove (delegating to existing
(*Player).ExactMove at modules/world/player_masks.go:28-37) and
ActivePlayer.UnsetMapFlag (new wrapper over the package-local
sendUnsetMapFlag helper). Pop-order pinned by 5-distinct-int recorded-
args test per handler_pop_order_test_masking.md.

NAI-160-D-EXACTMOVE-COORDLEVEL-IGNORE: TS discards the unpacked level
component (only x/z reach exactMove); goscape mirrors via _-bound level
return from checkCoord.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: `INV_ALLSTOCK` (OpInvAllStock, opcode 4303)

**Files:**
- Modify: `pkg/script/handlers_inv.go` (add `handleInvAllStock`)
- Modify: `pkg/script/handlers.go` (register `OpInvAllStock`)
- Test: `pkg/script/handlers_inv_test.go` (add `TestInvAllStock`, `TestInvAllStockFalseDefault`, `TestInvAllStockInvalidType`)

(No surface changes — `s.Configs.InvType` already exists, `InvType.AllStock` already parsed at `pkg/objtype/invtype.go:54`.)

**TS source:** `InvOps.ts:20-24`:
```ts
[ScriptOpcode.INV_ALLSTOCK]: state => {
    const invType: InvType = check(state.popInt(), InvTypeValid);

    state.pushInt(invType.allstock ? 1 : 0);
},
```

---

- [ ] **Step 1: Write the failing tests**

Append to `pkg/script/handlers_inv_test.go`:

```go
// TestInvAllStock pins OpInvAllStock's body: pop typeID, checkInvType,
// push 1 if InvType.AllStock else 0. Mirrors TS InvOps.ts:20-24.
// NAI-160 T5.
func TestInvAllStock(t *testing.T) {
	mp := &mockPlayer{}
	mc := &mockConfigs{invs: map[int]*objtype.InvType{42: {AllStock: true}}}
	sf := &ScriptFile{
		Name:             "[inv_allstock_true,test]",
		Opcodes:          []Opcode{OpPushConstantInt, OpInvAllStock, OpReturn},
		IntOperands:      []int32{42, 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	state := Init(sf, mp, false, nil, nil)
	state.Configs = mc
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := state.PopInt(); got != 1 {
		t.Errorf("INV_ALLSTOCK(AllStock=true): got %d, want 1", got)
	}
}

// TestInvAllStockFalseDefault pins the AllStock=false path. NAI-160 T5.
func TestInvAllStockFalseDefault(t *testing.T) {
	mp := &mockPlayer{}
	mc := &mockConfigs{invs: map[int]*objtype.InvType{42: {AllStock: false}}}
	sf := &ScriptFile{
		Name:             "[inv_allstock_false,test]",
		Opcodes:          []Opcode{OpPushConstantInt, OpInvAllStock, OpReturn},
		IntOperands:      []int32{42, 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	state := Init(sf, mp, false, nil, nil)
	state.Configs = mc
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := state.PopInt(); got != 0 {
		t.Errorf("INV_ALLSTOCK(AllStock=false): got %d, want 0", got)
	}
}

// TestInvAllStockInvalidType pins checkInvType rejection. NAI-160 T5.
func TestInvAllStockInvalidType(t *testing.T) {
	mp := &mockPlayer{}
	mc := &mockConfigs{invs: map[int]*objtype.InvType{}}
	sf := &ScriptFile{
		Name:             "[inv_allstock_invalid,test]",
		Opcodes:          []Opcode{OpPushConstantInt, OpInvAllStock, OpReturn},
		IntOperands:      []int32{99, 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	state := Init(sf, mp, false, nil, nil)
	state.Configs = mc
	err := Execute(state)
	if err == nil {
		t.Fatalf("Execute: got nil err, want INV_ALLSTOCK: no InvType with value (99) found")
	}
	if got := err.Error(); !strings.Contains(got, "INV_ALLSTOCK: no InvType with value (99) found") {
		t.Errorf("err: got %q, want substring 'INV_ALLSTOCK: no InvType with value (99) found'", got)
	}
}
```

(Confirm `objtype` and `strings` imports are present at the top of `handlers_inv_test.go`. They almost certainly are — existing tests reference both.)

- [ ] **Step 2: Run tests to verify they fail**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run 'TestInvAllStock' -v
```

Expected: handler-not-registered errors.

- [ ] **Step 3: Add `handleInvAllStock`**

Edit `pkg/script/handlers_inv.go`. Append:

```go
// handleInvAllStock implements OpInvAllStock (TS INV_ALLSTOCK at
// InvOps.ts:20-24). Pops a typeID, validates via checkInvType, pushes 1
// if InvType.AllStock else 0. NAI-160 T5.
func handleInvAllStock(s *ScriptState) error {
	typeID := s.PopInt()
	if err := checkInvType(s, typeID, "INV_ALLSTOCK"); err != nil {
		return err
	}
	if s.Configs.InvType(typeID).AllStock {
		s.PushInt(1)
	} else {
		s.PushInt(0)
	}
	return nil
}
```

- [ ] **Step 4: Register the handler**

Edit `pkg/script/handlers.go`:

```go
	// NAI-160 T5: INV_ALLSTOCK.
	OpInvAllStock: handleInvAllStock,
```

- [ ] **Step 5: Run tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run 'TestInvAllStock' -v
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/...
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add pkg/script/handlers.go pkg/script/handlers_inv.go pkg/script/handlers_inv_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-160 T5 — INV_ALLSTOCK handler (opcode 4303)

Ports TS InvOps.ts:20-24 — pop typeID, checkInvType, push
InvType.AllStock as 0/1. No new surface required; InvType.AllStock
already parsed at pkg/objtype/invtype.go:54.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: `NPC_ATTACKRANGE` (OpNpcAttackRange, opcode 2503)

**Files:**
- Modify: `pkg/script/handlers_npc.go` (add `handleNpcAttackRange`)
- Modify: `pkg/script/handlers.go` (register `OpNpcAttackRange`)
- Test: `pkg/script/handlers_npc_test.go` (add `TestNpcAttackRange`, `TestNpcAttackRangeRequiresActiveNpc`, `TestNpcAttackRangeInvalidType`)

(No surface changes — `s.ActiveNpc.NpcType()` already returns the type id, `s.Configs.NpcType` already exists, `NpcType.AttackRange` already parsed at `pkg/objtype/npctype.go:273`.)

**TS source:** `NpcOps.ts:521-523`:
```ts
[ScriptOpcode.NPC_ATTACKRANGE]: checkedHandler(ActiveNpc, state => {
    state.pushInt(check(state.activeNpc.type, NpcTypeValid).attackrange);
}),
```

---

- [ ] **Step 1: Write the failing tests**

Append to `pkg/script/handlers_npc_test.go`:

```go
// TestNpcAttackRange pins OpNpcAttackRange's body: read activeNpc.type,
// checkNpcType, push NpcType.AttackRange (widened to int). Mirrors TS
// NpcOps.ts:521-523. NAI-160 T6.
func TestNpcAttackRange(t *testing.T) {
	npc := &mockNpc{typeID: 7}
	mc := &mockConfigs{npcs: map[int]*objtype.NpcType{7: {AttackRange: 5}}}
	sf := &ScriptFile{
		Name:             "[npc_attackrange,test]",
		Opcodes:          []Opcode{OpNpcAttackRange, OpReturn},
		IntOperands:      []int32{0, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	state := Init(sf, nil, false, nil, nil)
	state.ActiveNpc = npc
	state.Pointers |= PtrActiveNpc
	state.Configs = mc
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := state.PopInt(); got != 5 {
		t.Errorf("NPC_ATTACKRANGE: got %d, want 5", got)
	}
}

// TestNpcAttackRangeRequiresActiveNpc pins the ActiveNpc gate.
// NAI-160 T6.
func TestNpcAttackRangeRequiresActiveNpc(t *testing.T) {
	sf := &ScriptFile{
		Name:             "[npc_attackrange_noactive,test]",
		Opcodes:          []Opcode{OpNpcAttackRange, OpReturn},
		IntOperands:      []int32{0, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	state := Init(sf, nil, false, nil, nil)
	// ActiveNpc intentionally nil.
	err := Execute(state)
	if err == nil {
		t.Fatalf("Execute: got nil err, want NPC_ATTACKRANGE: no active npc")
	}
	if got := err.Error(); !strings.Contains(got, "NPC_ATTACKRANGE: no active npc") {
		t.Errorf("err: got %q, want substring 'NPC_ATTACKRANGE: no active npc'", got)
	}
}

// TestNpcAttackRangeInvalidType pins checkNpcType rejection (e.g. an
// NPC whose type was decached or never loaded). NAI-160 T6.
func TestNpcAttackRangeInvalidType(t *testing.T) {
	npc := &mockNpc{typeID: 99}
	mc := &mockConfigs{npcs: map[int]*objtype.NpcType{}}
	sf := &ScriptFile{
		Name:             "[npc_attackrange_badtype,test]",
		Opcodes:          []Opcode{OpNpcAttackRange, OpReturn},
		IntOperands:      []int32{0, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	state := Init(sf, nil, false, nil, nil)
	state.ActiveNpc = npc
	state.Pointers |= PtrActiveNpc
	state.Configs = mc
	err := Execute(state)
	if err == nil {
		t.Fatalf("Execute: got nil err, want NPC_ATTACKRANGE: no NpcType with value (99) found")
	}
	if got := err.Error(); !strings.Contains(got, "NPC_ATTACKRANGE: no NpcType with value (99) found") {
		t.Errorf("err: got %q, want substring 'NPC_ATTACKRANGE: no NpcType with value (99) found'", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run 'TestNpcAttackRange' -v
```

Expected: handler-not-registered errors.

- [ ] **Step 3: Add `handleNpcAttackRange`**

Edit `pkg/script/handlers_npc.go`. Append (group near `handleNpcHasOp` / `handleNpcType` — neighbors NAI-120 introspection cohort):

```go
// handleNpcAttackRange implements OpNpcAttackRange (TS NPC_ATTACKRANGE at
// NpcOps.ts:521-523). Reads the active NPC's type, validates via
// checkNpcType, pushes NpcType.AttackRange widened from uint16 to int
// (deviation NAI-160-D-NPC-ATTACKRANGE-WIDEN — value-faithful, width is
// Go-side artifact). NAI-160 T6.
func handleNpcAttackRange(s *ScriptState) error {
	if err := requireActiveNpc(s, "NPC_ATTACKRANGE"); err != nil {
		return err
	}
	typeID := s.ActiveNpc.NpcType()
	if err := checkNpcType(s, typeID, "NPC_ATTACKRANGE"); err != nil {
		return err
	}
	s.PushInt(int(s.Configs.NpcType(typeID).AttackRange))
	return nil
}
```

- [ ] **Step 4: Register the handler**

Edit `pkg/script/handlers.go`:

```go
	// NAI-160 T6: NPC_ATTACKRANGE.
	OpNpcAttackRange: handleNpcAttackRange,
```

- [ ] **Step 5: Run tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run 'TestNpcAttackRange' -v
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/...
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add pkg/script/handlers.go pkg/script/handlers_npc.go pkg/script/handlers_npc_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-160 T6 — NPC_ATTACKRANGE handler (opcode 2503)

Ports TS NpcOps.ts:521-523 — read activeNpc.type, checkNpcType, push
NpcType.AttackRange. uint16 → int widening at push site (deviation
NAI-160-D-NPC-ATTACKRANGE-WIDEN). No new surface; backing config
field parsed at pkg/objtype/npctype.go:273.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 7: `NPC_INRANGE` (OpNpcInRange, opcode 2527)

**Files:**
- Modify: `pkg/script/active.go` (add `TargetWithinMaxRange() bool` to `ActiveNpc` interface)
- Modify: `pkg/script/handlers_npc.go` (add `handleNpcInRange`)
- Modify: `pkg/script/handlers.go` (register `OpNpcInRange`)
- Modify: `pkg/script/handlers_npc_test.go` (add `targetWithinMaxRangeValue bool` field + `TargetWithinMaxRange()` method on `mockNpc`)
- Modify: `modules/world/npc_interaction.go` (add exported `(*Npc).TargetWithinMaxRange` wrapper)
- Test: `pkg/script/handlers_npc_test.go` (add `TestNpcInRangeTrue`, `TestNpcInRangeFalse`, `TestNpcInRangeRequiresActiveNpc`)

**TS source:** `NpcOps.ts:556-558`:
```ts
[ScriptOpcode.NPC_INRANGE]: checkedHandler(ActiveNpc, state => {
    state.pushInt(state.activeNpc.targetWithinMaxRange() ? 1 : 0);
})
```

---

- [ ] **Step 1: Write the failing tests**

Append to `pkg/script/handlers_npc_test.go`:

```go
// TestNpcInRangeTrue pins OpNpcInRange's body when the NPC's target is
// within max range: push 1. Mirrors TS NpcOps.ts:556-558. NAI-160 T7.
func TestNpcInRangeTrue(t *testing.T) {
	npc := &mockNpc{targetWithinMaxRangeValue: true}
	sf := &ScriptFile{
		Name:             "[npc_inrange_true,test]",
		Opcodes:          []Opcode{OpNpcInRange, OpReturn},
		IntOperands:      []int32{0, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	state := Init(sf, nil, false, nil, nil)
	state.ActiveNpc = npc
	state.Pointers |= PtrActiveNpc
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := state.PopInt(); got != 1 {
		t.Errorf("NPC_INRANGE(true): got %d, want 1", got)
	}
}

// TestNpcInRangeFalse pins the false path (default zero-value). NAI-160 T7.
func TestNpcInRangeFalse(t *testing.T) {
	npc := &mockNpc{targetWithinMaxRangeValue: false}
	sf := &ScriptFile{
		Name:             "[npc_inrange_false,test]",
		Opcodes:          []Opcode{OpNpcInRange, OpReturn},
		IntOperands:      []int32{0, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	state := Init(sf, nil, false, nil, nil)
	state.ActiveNpc = npc
	state.Pointers |= PtrActiveNpc
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := state.PopInt(); got != 0 {
		t.Errorf("NPC_INRANGE(false): got %d, want 0", got)
	}
}

// TestNpcInRangeRequiresActiveNpc pins the ActiveNpc gate. NAI-160 T7.
func TestNpcInRangeRequiresActiveNpc(t *testing.T) {
	sf := &ScriptFile{
		Name:             "[npc_inrange_noactive,test]",
		Opcodes:          []Opcode{OpNpcInRange, OpReturn},
		IntOperands:      []int32{0, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	state := Init(sf, nil, false, nil, nil)
	err := Execute(state)
	if err == nil {
		t.Fatalf("Execute: got nil err, want NPC_INRANGE: no active npc")
	}
	if got := err.Error(); !strings.Contains(got, "NPC_INRANGE: no active npc") {
		t.Errorf("err: got %q, want substring 'NPC_INRANGE: no active npc'", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run 'TestNpcInRange' -v
```

Expected: compile errors (`targetWithinMaxRangeValue` undefined, `ActiveNpc.TargetWithinMaxRange` undefined, handler not registered).

- [ ] **Step 3: Add `TargetWithinMaxRange()` to ActiveNpc interface**

Edit `pkg/script/active.go`. Append in the `ActiveNpc` interface block (near other NPC-state readers like `LastMovement`):

```go
	// TargetWithinMaxRange returns true if the NPC's current target is
	// inside the per-mode maxrange envelope (HUNT-distance + corner-quirk
	// adjustments). Mirrors TS Npc.targetWithinMaxRange — read by
	// NPC_INRANGE (NpcOps.ts:556-558). Backing impl at
	// modules/world/npc_interaction.go:591. Returns false defensively when
	// the NPC has no target (TS-equivalent). NAI-160 T7.
	TargetWithinMaxRange() bool
```

- [ ] **Step 4: Add `targetWithinMaxRangeValue` + recorder to mockNpc**

Edit `pkg/script/handlers_npc_test.go`. Add to the `mockNpc` struct (group with other state-readback fields near `lastMovement`):

```go
	// NAI-160 T7: NPC_INRANGE seeded value.
	targetWithinMaxRangeValue bool
```

Add method (group with other ActiveNpc impls):

```go
// TargetWithinMaxRange returns the seeded value. NAI-160 T7.
func (m *mockNpc) TargetWithinMaxRange() bool { return m.targetWithinMaxRangeValue }
```

- [ ] **Step 5: Add `(*Npc).TargetWithinMaxRange` exported wrapper**

Edit `modules/world/npc_interaction.go`. Add IMMEDIATELY AFTER the existing `func (n *Npc) targetWithinMaxRange() bool {` definition (currently around line 591):

```go
// TargetWithinMaxRange exports the unexported targetWithinMaxRange for
// the ActiveNpc.TargetWithinMaxRange surface consumed by NPC_INRANGE
// (TS NpcOps.ts:556-558). Thin wrapper; no logic. NAI-160 T7.
func (n *Npc) TargetWithinMaxRange() bool {
	return n.targetWithinMaxRange()
}
```

- [ ] **Step 6: Add `handleNpcInRange`**

Edit `pkg/script/handlers_npc.go`. Append:

```go
// handleNpcInRange implements OpNpcInRange (TS NPC_INRANGE at
// NpcOps.ts:556-558). Calls ActiveNpc.TargetWithinMaxRange() and pushes
// 0/1. NAI-160 T7.
func handleNpcInRange(s *ScriptState) error {
	if err := requireActiveNpc(s, "NPC_INRANGE"); err != nil {
		return err
	}
	if s.ActiveNpc.TargetWithinMaxRange() {
		s.PushInt(1)
	} else {
		s.PushInt(0)
	}
	return nil
}
```

- [ ] **Step 7: Register the handler**

Edit `pkg/script/handlers.go`:

```go
	// NAI-160 T7: NPC_INRANGE.
	OpNpcInRange: handleNpcInRange,
```

- [ ] **Step 8: Run tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run 'TestNpcInRange' -v
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
```

Expected: all PASS + clean build.

Also verify no `*Npc` ActiveNpc satisfaction breaks: `var _ script.ActiveNpc = (*Npc)(nil)` must still compile. Grep for the assertion:

```bash
grep -rn "var _ script.ActiveNpc = (\*Npc)(nil)" modules/world/
```

If found, run `go build ./modules/world/...` — non-zero exit means the assertion failed and a method is missing. Expected: clean.

- [ ] **Step 9: Commit**

```bash
git add pkg/script/active.go pkg/script/handlers.go pkg/script/handlers_npc.go pkg/script/handlers_npc_test.go modules/world/npc_interaction.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-160 T7 — NPC_INRANGE handler (opcode 2527)

Ports TS NpcOps.ts:556-558 — call activeNpc.targetWithinMaxRange(),
push 0/1.

Adds ActiveNpc.TargetWithinMaxRange and exported (*Npc) wrapper over
the existing unexported targetWithinMaxRange method at
modules/world/npc_interaction.go:591.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## End-of-impl review + smoke handoff

After T7's commit, before close:

- [ ] **Run full test suite** (regression check):

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...
```

Expected: all PASS, no race warnings.

- [ ] **Two-stage review** — single `superpowers:code-reviewer` agent on Sonnet, scope = `git log --oneline b0d576e..HEAD` (the 7 NAI-160 commits). Per `superpowers_code_reviewer_model.md`, NOT Opus.

Reviewer's prompt should explicitly cover:
- TS-fidelity per handler against the cited TS line ranges in commit bodies
- Pop-order pin on P_EXACTMOVE (5-distinct-int test pattern)
- Deviation register entries (4 from spec §3) match the implementation
- No spec premise has drifted (re-verify the §10 no-deviations audit assertions at the impl SHAs)
- Mock recorder fields and methods don't break existing test fixtures in other suites

- [ ] **Apply any reviewer fix-ups** as `feat(script): NAI-160 T<N>-fixup — <description>` commits.

- [ ] **Smoke handoff to user** (per `smoke_test_server_handoff.md`):

User prompt to drop:

> NAI-160 implementation complete locally on `main`. Please run goscape + Java client per the standard handoff convention. Smoke targets (per spec §6):
> 1. **SAY-bubble visibility** (binding signal, 126 content callers): trigger any NPC dialog (e.g., talk to any NPC with a `chat_npc` content script). Expect the speech bubble text to appear above the NPC; server logs should show no `no handler for SAY (opcode 2097)` WARN.
> 2. **(Optional, non-binding)** Trigger any content path that touches `headicons_set` or `p_exactmove` — most are quest-gated and may not be reachable from a fresh tutorial; skip if unreachable. INV_ALLSTOCK / NPC_ATTACKRANGE / NPC_INRANGE are unit-pinned only.
>
> Report any WARN classes or unexpected behavior.

- [ ] **After smoke confirms, write close commit:**

```bash
git commit --allow-empty --no-gpg-sign -m "$(cat <<'EOF'
chore(close): NAI-160 — trivial-handler sweep #2 landed

7 opcode handlers ported: SAY (T1), HEADICONS_GET (T2), HEADICONS_SET (T3),
P_EXACTMOVE (T4), INV_ALLSTOCK (T5), NPC_ATTACKRANGE (T6),
NPC_INRANGE (T7). Closes top user-hot entry in 28-opcode unhandled
tail (`say` @ 126 content callers — smoke-confirmed via NPC-dialog
speech-bubble round-trip).

Closes memory:
- runescript_cadence.md
- controller_preflight.md
- missing_handler_audit.md
- audit_full_method_against_ts.md
- handler_pop_order_test_masking.md
- plan_grep_helper_patterns.md
- plan_sibling_site_guard_audit.md
- mock_recorder_field_naming_check.md
- defensive_gate_doc_comment_label.md
- true_to_ts_gate.md
- spec_ts_source_read.md
- smoke_test_server_handoff.md
- cascade_theory_smoke_binding.md
- enumerate_all_sites.md
- superpowers_clear_between_spec_and_impl.md
- superpowers_code_reviewer_model.md
- execution_mode_default.md

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

- [ ] **Final memory sweep:** any non-derivable lesson learned during impl (per spec §10 + `nai_followups.md`)? Write a single new memory entry if so (e.g., a pop-order test fixture trap, a TS-deref-vs-goscape-guard pattern). If none, skip.

---

## Plan self-review

**Spec coverage:**

| Spec section | Plan task(s) |
|---|---|
| §2.1 ActivePlayer additions (Say, HeadIcons, SetHeadIcons, ExactMove, UnsetMapFlag) | T1, T2 (decl), T3 (use), T4 |
| §2.2 ActiveNpc additions (TargetWithinMaxRange) | T7 |
| §2.3 *Player adapters (HeadIcons, SetHeadIcons, UnsetMapFlag) | T2, T4 |
| §2.4 *Npc adapter (TargetWithinMaxRange) | T7 |
| §2.5 7 handler bodies | T1–T7 (one task each) |
| §2.6 Handler registration (7 lines) | T1–T7 final Step before commit |
| §2.7 Mock recorder additions | T1, T2, T4, T7 |
| §2.8 InvType/NpcType mock backing | T5, T6 (use existing `mockConfigs`) |
| §3 Deviation register (4 entries) | Codified in T4 doc-comment (EXACTMOVE-COORDLEVEL-IGNORE), T2 doc-comment (HEADICONS-INT-WIDTH absorbed into appearance.go cite), T6 doc-comment (NPC-ATTACKRANGE-WIDEN). INV-ALLSTOCK-NIL-DEFENSIVE is "no comment" per spec — verified at T5 by NOT adding a comment. |
| §4 Risk register | R1 → T4 recorded-args test; R2 → T4 sendUnsetMapFlag wrapper in `*Player`; R3 → resolved at brainstorm (checkNpcType exists at handlers_npc.go:45); R4 → T1 defensive byte-copy in mockPlayer.Say; R5 → T3 pop→check→set order; R6 → T7 false-default test; R7 → all 7 tasks include `go build ./...` step. |
| §5 Test strategy (positive + require-active + protected gate + coord validation + null gate + invalid-type + empty-string + recorded-args) | Distributed: T1 (positive, empty, require-active), T2 (positive, require-active), T3 (positive, null-gate, require-active), T4 (positive, protected, invalid-coord, require-active, recorded-args), T5 (positive true, positive false, invalid-type), T6 (positive, require-active, invalid-type), T7 (positive true, positive false, require-active). |
| §6 Smoke binding | End-of-impl handoff section |
| §7 Cadence routing | Plan header pointers |

**Placeholder scan:** None. All TBD/TODO/XXX are TS-source quotes (in §3 deviation register and TS line cites). No "implement later" — every step has full code.

**Type consistency:**
- `Say(text []byte)` — declared T1 active.go, recorder T1 mockPlayer, handler T1 calls `s.Self.Say([]byte(text))`. Consistent.
- `HeadIcons() int` / `SetHeadIcons(v int)` — declared T2 active.go (both), T2 implements HeadIcons / T3 implements SetHeadIcons; mockPlayer adds both in T2 to avoid interface-unsatisfied compile errors. Consistent.
- `ExactMove(sX, sZ, eX, eZ, begin, finish, dir int)` — 7-int signature matches `(*Player).ExactMove` at modules/world/player_masks.go:28. Consistent.
- `UnsetMapFlag()` — 0-arg, declared T4, implemented T4. Consistent.
- `TargetWithinMaxRange() bool` — declared T7, implemented T7. Consistent.
- Recorder field name `sayCalls` — T1 mockPlayer adds `sayCalls [][]byte` (NOT `[]string` — defensive byte slice). Tests reference `mp.sayCalls[0]` as `[]byte`. Consistent.
- `setHeadIconsCalls` (T2/T3): plural-Calls slice of int recording each write. Tests reference `mp.setHeadIconsCalls[0]` as int. Consistent.
- `exactMoveCalls` (T4): slice of anonymous struct with 7 named fields matching `ExactMove` arg names exactly. Test accesses fields by name (`c.sX`, `c.begin`, etc.). Consistent.

**Order sanity:** T1–T7 are independent (each handler self-contained); no inter-task imports. T2 declares both `HeadIcons` and `SetHeadIcons` on the ActivePlayer interface (and mockPlayer satisfies both) so the package compiles after T2 even though T3 hasn't landed yet. T4 introduces UnsetMapFlag on the interface and adds the (*Player) impl in the same task — no intermediate broken state.

---

## Resume prompt (for fresh session after `/clear`)

Per `superpowers_clear_between_spec_and_impl.md`:

> NAI-160 — trivial-handler sweep #2 (7 opcodes: SAY, HEADICONS_GET, HEADICONS_SET, P_EXACTMOVE, INV_ALLSTOCK, NPC_ATTACKRANGE, NPC_INRANGE). Spec at `docs/superpowers/specs/2026-05-11-nai-160-trivial-handler-sweep-2-design.md`. Plan at `docs/superpowers/plans/2026-05-11-nai-160-trivial-handler-sweep-2.md`. Execute via subagent-driven-development (per `execution_mode_default.md`): controller dispatches one subagent per task T1–T7 in order, TDD-shaped per the plan, two-stage review at end via single Sonnet `superpowers:code-reviewer` (per `superpowers_code_reviewer_model.md`). User-driven smoke after T7 binds on SAY-bubble visibility.
