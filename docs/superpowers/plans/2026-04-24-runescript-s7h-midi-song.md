# S7h — MIDI_SONG + MIDI_JINGLE Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port the `MIDI_SONG` (2064) and `MIDI_JINGLE` (2063) RuneScript opcodes to goscape, plumb the RS2 `lowMemory` login flag through to the `Player` so the handlers' low-memory gate actually gates, and keep the client-packet writes deferred (single deviation **S7h-D1**) until PRELOADED music / CRC infrastructure lands under NAI-16.

**Architecture:** Handlers follow the established CAM_RESET template — registered in `pkg/script/handlers.go` → method on `ActivePlayer` interface → implementation on `*Player` (modules/world) + test-mock implementation (pkg/script/runner_test.go). Name-normalization is extracted into two unexported helpers (`normalizeSongName`, `normalizeJingleName`) so positive direction-pin tests have a concrete observation point. Player methods perform TS-fidelity normalization and early-return-on-empty but never call `p.writeOut` — absence-pin tests escalate when NAI-16 wires the encoder.

**Tech Stack:** Go 1.26+. Existing packages: `pkg/script/`, `modules/world/`. No new dependencies.

**Source of truth for the port:** `/home/owner/Code/github.com/LostCityRS/Engine-TS` — NEVER any sibling LostCityRS repo. TS references cited with file:line. If you cannot confirm a cited line, open the file at the commit reachable from that repo's `main` and verify before coding.

**Verification discipline:** Every task ends with a `go test ./...` run from project root with `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache` prefix. Do not trust prior-task green status — rerun. Do not report completion without running step-by-step checkboxes in order.

---

## File map (exact paths)

**Modified production files (7):**

- `pkg/script/handlers_player.go` — `checkStringNotNull` validator; `handleMidiSong`; `handleMidiJingle`.
- `pkg/script/handlers.go` — two new dispatch rows (`OpMidiSong`, `OpMidiJingle`).
- `pkg/script/active.go` — `LowMemory()`, `PlaySong(name string)`, `PlayJingle(delay int, name string)` on `ActivePlayer`.
- `modules/world/player_script.go` — `(*Player).LowMemory`; `normalizeSongName` + `(*Player).PlaySong`; `normalizeJingleName` + `(*Player).PlayJingle`.
- `modules/world/client.go` — `lowMemory bool` field on `client` struct.
- `modules/world/server.go` — single-line `c.lowMemory = req.LowMemory` after `req.UnmarshalBinary`.
- `modules/world/player.go` — single-line `lowMemory: c.lowMemory,` in `newPlayer(c)` struct literal.

**Modified test files (4):**

- `pkg/script/handlers_player_test.go` — validator unit tests, MIDI_SONG handler tests, MIDI_JINGLE handler tests.
- `pkg/script/runner_test.go` — `mockPlayer` struct gains `lowMemory` field, `playSongCalls` + `playJingleCalls` slices; impls `LowMemory`, `PlaySong`, `PlayJingle`.
- `modules/world/player_script_test.go` — `normalizeSongName` / `normalizeJingleName` unit tests; `PlaySong` / `PlayJingle` empty-return tests; absence-pin tests; `LowMemory` getter test.
- `modules/world/player_test.go` — `TestNewPlayerCopiesLowMemoryFromClient`.

**Untouched (verify at acceptance gate):**

- `pkg/io/protocol/game/server/prot.go` — MUST NOT gain `OpMidiSong` or `OpMidiJingle`. S7h-D1 depends on this.

## Task ordering rationale

Tasks are ordered so each one leaves the tree in a compile-green, test-green state:

1. **Task 1 — `checkStringNotNull`** — pure function, no cross-file coupling. Independent.
2. **Task 2 — `lowMemory` plumbing + `LowMemory()` getter + interface + mock** — bundled because Go's interface-satisfaction check would fail on any ordering where one piece lands without the others. Includes plumbing assignment, getter, interface decl, and mock impl as a single atomic unit.
3. **Task 3 — `MIDI_SONG` full stack** — handler + interface `PlaySong` + mock `PlaySong` + `normalizeSongName` + `(*Player).PlaySong` + dispatch registration. All MIDI_SONG bits together so the interface never has a dangling unimplemented method.
4. **Task 4 — `MIDI_JINGLE` full stack** — symmetric to Task 3.
5. **Task 5 — Close commit** — final `go test ./...` + `go vet ./...` from project root + dead-code audit + close-commit with `Closes memory:` trailer.

---

## Task 1: `checkStringNotNull` validator

**Files:**
- Modify: `pkg/script/handlers_player.go` (insert helper)
- Test: `pkg/script/handlers_player_test.go` (append tests)

### Step 1.1: Write the failing test

- [ ] Add this test to the end of `pkg/script/handlers_player_test.go`. Placement: anywhere after existing tests; alphabetical by test name is fine.

```go
func TestCheckStringNotNullEmpty(t *testing.T) {
	err := checkStringNotNull("", "MIDI_SONG")
	if err == nil {
		t.Fatal("empty string: want error, got nil")
	}
	if !strings.Contains(err.Error(), "MIDI_SONG: input string was null") {
		t.Errorf("error message %q does not contain %q", err.Error(), "MIDI_SONG: input string was null")
	}
}

func TestCheckStringNotNullNonEmpty(t *testing.T) {
	if err := checkStringNotNull("harmony1", "MIDI_SONG"); err != nil {
		t.Errorf("non-empty string: want nil, got %v", err)
	}
}
```

### Step 1.2: Run tests to verify failure

- [ ] Run:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestCheckStringNotNull -v
```

Expected: compile error `undefined: checkStringNotNull` (not a test failure — the symbol does not exist yet).

### Step 1.3: Add the validator

- [ ] Open `pkg/script/handlers_player.go`. Locate `checkNotNull` (currently at lines 58-66, identified by the comment `// checkNotNull mirrors TS NumberNotNull`). Insert the following helper immediately **after** the closing `}` of `checkNotNull` (and before the `checkInvType` comment that follows):

```go
// checkStringNotNull mirrors TS StringNotNull
// (ScriptInputStringNotNullValidator at ScriptValidators.ts:50-55) —
// rejects empty strings, accepts any non-empty string. Used by handlers
// wrapping a popString result with TS check(..., StringNotNull). TS
// error literal: "An input string was null(-1)." — goscape drops the
// "(-1)" suffix since strings have no -1 sentinel (the sentinel is "").
func checkStringNotNull(v, op string) error {
	if v == "" {
		return fmt.Errorf("%s: input string was null", op)
	}
	return nil
}
```

The file already imports `"fmt"`; no import adjustment needed.

### Step 1.4: Run tests to verify pass

- [ ] Run:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestCheckStringNotNull -v
```

Expected: both `TestCheckStringNotNullEmpty` and `TestCheckStringNotNullNonEmpty` PASS.

### Step 1.5: Run the full package

- [ ] Run:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/...
```

Expected: all green. Unused `checkStringNotNull` is acceptable at this point — no `ineffassign` lint ships by default; Tasks 3 and 4 will consume it.

### Step 1.6: Commit

- [ ] Run:

```bash
git add pkg/script/handlers_player.go pkg/script/handlers_player_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): S7h Task 1 — checkStringNotNull validator

Sibling to checkNotNull, for handlers wrapping a popString result with
TS check(..., StringNotNull). First consumer lands in Task 3
(handleMidiSong) and Task 4 (handleMidiJingle).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: `lowMemory` plumbing + `LowMemory()` getter + interface + mock

**Files:**
- Modify: `modules/world/client.go` (add field)
- Modify: `modules/world/server.go` (assign from req)
- Modify: `modules/world/player.go` (copy to Player)
- Modify: `modules/world/player_script.go` (add `LowMemory()` method)
- Modify: `pkg/script/active.go` (add `LowMemory()` to `ActivePlayer` interface)
- Modify: `pkg/script/runner_test.go` (add `lowMemory` field + `LowMemory()` impl to `mockPlayer`)
- Test: `modules/world/player_test.go` (plumbing test)
- Test: `modules/world/player_script_test.go` (getter test)

### Step 2.1: Write the failing plumbing test

- [ ] Append to `modules/world/player_test.go`:

```go
func TestNewPlayerCopiesLowMemoryFromClient(t *testing.T) {
	// lowMemory=true case
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()
	c := newClient(serverConn, time.Second, discardLogger())
	defer c.in.Release()
	c.state = ClientStateGame
	c.lowMemory = true
	p := newPlayer(c)
	if !p.lowMemory {
		t.Errorf("lowMemory=true on client: want p.lowMemory=true, got false")
	}

	// lowMemory=false (default) case
	serverConn2, clientConn2 := net.Pipe()
	defer serverConn2.Close()
	defer clientConn2.Close()
	c2 := newClient(serverConn2, time.Second, discardLogger())
	defer c2.in.Release()
	c2.state = ClientStateGame
	// c2.lowMemory defaults to false
	p2 := newPlayer(c2)
	if p2.lowMemory {
		t.Errorf("lowMemory=false on client: want p.lowMemory=false, got true")
	}
}
```

Check the existing file header — confirm it imports `"net"`, `"time"`, `"testing"`. If `"net"` or `"time"` is missing, add them. `discardLogger()` is already used by `newTestPlayer`, so it's in scope.

### Step 2.2: Write the failing getter test

- [ ] Append to `modules/world/player_script_test.go`:

```go
func TestPlayerLowMemoryGetter(t *testing.T) {
	p, _ := newTestPlayer(t)
	if p.LowMemory() {
		t.Errorf("default: want LowMemory()=false, got true")
	}
	p.lowMemory = true
	if !p.LowMemory() {
		t.Errorf("after set: want LowMemory()=true, got false")
	}
}
```

### Step 2.3: Run tests to verify failure

- [ ] Run:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestNewPlayerCopiesLowMemoryFromClient|TestPlayerLowMemoryGetter" -v
```

Expected:
- `TestNewPlayerCopiesLowMemoryFromClient`: compile error `c.lowMemory undefined (type *client has no field or method lowMemory)`.
- `TestPlayerLowMemoryGetter`: compile error `p.LowMemory undefined`.

### Step 2.4: Add the `lowMemory` field to `client`

- [ ] Open `modules/world/client.go`. Locate the `client` struct (declared at line 35, identified by `type client struct {`). Find the `staffModLevel` / `members` fields around lines 50-51. Insert the new field immediately after `members`:

```go
	// lowMemory carries the client's low-memory capability bit from the
	// RS2 login packet (LoginRequest.LowMemory, parsed at server.go's
	// req.UnmarshalBinary). Copied onto Player at newPlayer(). Read by
	// script opcodes that trigger client audio loads (MIDI_SONG, MIDI_JINGLE).
	lowMemory bool
```

### Step 2.5: Wire the server.go login → client assignment

- [ ] Open `modules/world/server.go`. Locate the `req.UnmarshalBinary(b)` call at line 470 (identified by the `if err := req.UnmarshalBinary(b); err != nil {` guard preceding it). Immediately **after** the closing `}` of that error-guard block, insert:

```go
		c.lowMemory = req.LowMemory
```

Indentation matches the surrounding scope (two tab-indent levels inside the handler closure). Placement rationale: `lowMemory` is a client-capability flag present on every login attempt regardless of accept/reject; set it alongside the earliest successful-parse step rather than the conditional `c.staffModLevel` / `c.members` assignments at the RPC-response branch.

### Step 2.6: Copy the flag into `newPlayer`

- [ ] Open `modules/world/player.go`. Locate `func newPlayer(c *client) *Player` at line 293. The function body is a single `p := &Player{ ... }` struct literal. Locate the first bool-valued field in the literal — which is `client: c,` (at line 294) followed by int/enum fields. The canonical location for session-flag copies is near the top of the literal with other `c.*` copies.

Since the existing literal does not yet copy any `c.*` bool, add a new line directly after the `client: c,` line:

```go
		client:    c,
		lowMemory: c.lowMemory,
```

Preserve the vertical-alignment style of the surrounding literal (gofmt will handle spacing once `go build` is run; manual alignment is not required).

### Step 2.7: Add `LowMemory()` to the `ActivePlayer` interface

- [ ] Open `pkg/script/active.go`. Locate the `ActivePlayer` interface — it is declared as a single `interface {` block and contains dozens of methods. Append the following to the interface body (trailing location, just before the closing `}`):

```go
	// LowMemory reports whether the player's client requested low-memory
	// mode at login (carried on the RS2 login request's LowMemory bit).
	// Script opcodes that trigger client audio loads gate on this flag —
	// see handleMidiSong / handleMidiJingle in handlers_player.go.
	LowMemory() bool
```

### Step 2.8: Implement `(*Player).LowMemory`

- [ ] Open `modules/world/player_script.go`. Append to the file (trailing location):

```go
// LowMemory returns the player's low-memory flag as plumbed from the
// RS2 login request (req.LowMemory) through client.lowMemory and
// copied onto the Player at newPlayer().
func (p *Player) LowMemory() bool { return p.lowMemory }
```

### Step 2.9: Extend `mockPlayer` with the matching field + method

- [ ] Open `pkg/script/runner_test.go`. Locate the end of the `mockPlayer` struct body (currently terminates around line 224 after `allowDesignValue` / `allowDesignCalls`). Append the new field block:

```go
	// S7h: lowMemory pre-seed for MIDI_SONG / MIDI_JINGLE lowMemory-gate tests.
	lowMemoryValue bool
```

- [ ] Below the existing `func (m *mockPlayer) SetAllowDesign(...)` method (or wherever the trailing group of method implementations ends before `mockNpcLookup`), append the impl:

```go
// S7h: LowMemory returns the seeded value for MIDI_SONG / MIDI_JINGLE
// handler tests that exercise the lowMemory bail path.
func (m *mockPlayer) LowMemory() bool { return m.lowMemoryValue }
```

Note the `lowMemoryValue` name pattern matches the existing mock convention (`staffModLevelValue`, `uidValue`, `canAccessValue`, `targetSubjectComValue`, etc. — all use a `Value` suffix for pre-seed slots).

### Step 2.10: Run tests to verify pass

- [ ] Run:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ ./pkg/script/ -run "TestNewPlayerCopiesLowMemoryFromClient|TestPlayerLowMemoryGetter" -v
```

Expected: both tests PASS.

### Step 2.11: Run the full test suite

- [ ] Run:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
```

Expected: build clean, all packages green. If a local fake-ActivePlayer stub exists in some `_test.go` that doesn't implement `LowMemory()`, the failure will surface here — add the method to any such stub, returning `false` unless the test explicitly needs a different value. To enumerate candidates, run:

```bash
rg -n "ActivePlayer\b" pkg/script/ modules/world/ --type go | grep -v "^Binary" | head -40
```

Typical stubs are `mockPlayer` (just touched) and occasionally inline struct-literals in per-test helpers.

### Step 2.12: Commit

- [ ] Run:

```bash
git add modules/world/client.go modules/world/server.go modules/world/player.go modules/world/player_script.go modules/world/player_test.go modules/world/player_script_test.go pkg/script/active.go pkg/script/runner_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): S7h Task 2 — lowMemory plumbing + LowMemory() getter

Threads req.LowMemory from the RS2 login packet through client.lowMemory
into the Player struct literal at newPlayer. Adds LowMemory() on the
ActivePlayer interface and (*Player), plus the matching mockPlayer
field and impl. Tasks 3 and 4 consume LowMemory() in the MIDI_SONG /
MIDI_JINGLE handlers' low-memory gate.

Placement of c.lowMemory = req.LowMemory directly after
req.UnmarshalBinary (not beside c.staffModLevel / c.members) reflects
that lowMemory is a client-capability flag on every login, not an
RPC-response attribute conditional on accept.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: `MIDI_SONG` full stack

**Files:**
- Modify: `pkg/script/handlers_player.go` (add `handleMidiSong`)
- Modify: `pkg/script/handlers.go` (register `OpMidiSong`)
- Modify: `pkg/script/active.go` (add `PlaySong` to `ActivePlayer`)
- Modify: `pkg/script/runner_test.go` (add `playSongCalls` + `PlaySong` impl to mock)
- Modify: `modules/world/player_script.go` (add `normalizeSongName` + `(*Player).PlaySong`)
- Test: `pkg/script/handlers_player_test.go` (handler dispatch tests)
- Test: `modules/world/player_script_test.go` (normalizer unit tests + absence-pin)

### Step 3.1: Write failing `normalizeSongName` unit tests

- [ ] Append to `modules/world/player_script_test.go`:

```go
func TestNormalizeSongNameLowercaseAndSpacesToUnderscores(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"Harmony 1", "harmony_1"},
		{"already_lower", "already_lower"},
		{"ALLCAPS", "allcaps"},
		{"Mixed CASE With Spaces", "mixed_case_with_spaces"},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			if got := normalizeSongName(tc.in); got != tc.want {
				t.Errorf("normalizeSongName(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestNormalizeSongNameEmptyReturnsEmpty(t *testing.T) {
	if got := normalizeSongName(""); got != "" {
		t.Errorf("normalizeSongName(\"\") = %q, want \"\"", got)
	}
}
```

### Step 3.2: Write failing `PlaySong` absence-pin + empty-return tests

- [ ] Append to `modules/world/player_script_test.go`:

```go
// TestPlaySongNoWriteOut pins S7h-D1: (*Player).PlaySong must NOT
// issue a writeOut until PRELOADED music / CRC infra lands (tracked as
// NAI-16-midi-encoders). When the encoder ports, this test fails —
// the failure is the escalation signal to retire S7h-D1.
func TestPlaySongNoWriteOut(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.PlaySong("harmony1")
	if n := p.client.bufw.Buffered(); n != 0 {
		t.Errorf("PlaySong wrote %d bytes to c.bufw; want 0 (S7h-D1 absence-pin)", n)
	}
}

func TestPlaySongEmptyNameReturnsSilently(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.PlaySong("")
	if n := p.client.bufw.Buffered(); n != 0 {
		t.Errorf("empty name: PlaySong wrote %d bytes; want 0", n)
	}
}
```

### Step 3.3: Write failing handler tests

- [ ] Append to `pkg/script/handlers_player_test.go`:

```go
func TestMidiSongHappyPath(t *testing.T) {
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Self:        &mockPlayer{},
		Pointers:    PtrActivePlayer,
	}
	s.PushString("harmony1")
	mp := s.Self.(*mockPlayer)

	if err := handleMidiSong(s); err != nil {
		t.Fatalf("handleMidiSong: %v", err)
	}
	if len(mp.playSongCalls) != 1 {
		t.Fatalf("playSongCalls: got %d, want 1", len(mp.playSongCalls))
	}
	if mp.playSongCalls[0].name != "harmony1" {
		t.Errorf("playSongCalls[0].name: got %q, want %q", mp.playSongCalls[0].name, "harmony1")
	}
}

func TestMidiSongLowMemoryBails(t *testing.T) {
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Self:        &mockPlayer{lowMemoryValue: true},
		Pointers:    PtrActivePlayer,
	}
	s.PushString("harmony1")
	mp := s.Self.(*mockPlayer)

	if err := handleMidiSong(s); err != nil {
		t.Fatalf("handleMidiSong: %v", err)
	}
	if len(mp.playSongCalls) != 0 {
		t.Errorf("lowMemory=true: playSongCalls=%d, want 0", len(mp.playSongCalls))
	}
}

func TestMidiSongNullStringRejects(t *testing.T) {
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Self:        &mockPlayer{},
		Pointers:    PtrActivePlayer,
	}
	s.PushString("")

	err := handleMidiSong(s)
	if err == nil {
		t.Fatal("empty name: want error, got nil")
	}
	if !strings.Contains(err.Error(), "MIDI_SONG: input string was null") {
		t.Errorf("error %q does not contain %q", err.Error(), "MIDI_SONG: input string was null")
	}
}

func TestMidiSongNoActivePlayerRejects(t *testing.T) {
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Self:        nil,
		Pointers:    0, // PtrActivePlayer unset
	}
	s.PushString("harmony1")

	err := handleMidiSong(s)
	if err == nil {
		t.Fatal("no active player: want error, got nil")
	}
	if !strings.Contains(err.Error(), "MIDI_SONG: no active player") {
		t.Errorf("error %q does not contain %q", err.Error(), "MIDI_SONG: no active player")
	}
}
```

### Step 3.4: Run tests to verify failure

- [ ] Run:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ ./pkg/script/ -run "MidiSong|NormalizeSongName|PlaySong" -v
```

Expected: compile errors for undefined `normalizeSongName`, `p.PlaySong`, `handleMidiSong`, and `mp.playSongCalls`.

### Step 3.5: Add `PlaySong` to `ActivePlayer` interface

- [ ] Open `pkg/script/active.go`. Append to the `ActivePlayer` interface body, immediately after the `LowMemory()` declaration added in Task 2:

```go
	// PlaySong sends a MIDI song by name to the client. Called by the
	// MIDI_SONG script opcode (PlayerOps.ts:796-804).
	//
	// S7h-D1: actual MidiSong client packet is deferred pending PRELOADED
	// music + CRC infrastructure; current impl performs TS name
	// normalization (lowercase + spaces→underscores) and early-returns
	// on empty without writing.
	PlaySong(name string)
```

### Step 3.6: Extend `mockPlayer` with `playSongCalls` field + `PlaySong` method

- [ ] Open `pkg/script/runner_test.go`. In the `mockPlayer` struct body, append (right after the `lowMemoryValue bool` field added in Task 2):

```go
	// S7h: captured MIDI_SONG plays. Each entry records the normalized-name
	// argument as seen by the mock; the mock does not perform TS
	// normalization (that's (*Player).PlaySong's responsibility).
	playSongCalls []struct{ name string }
```

- [ ] After the `func (m *mockPlayer) LowMemory()` impl added in Task 2, append:

```go
// S7h: PlaySong captures the MIDI_SONG name for handler tests.
func (m *mockPlayer) PlaySong(name string) {
	m.playSongCalls = append(m.playSongCalls, struct{ name string }{name})
}
```

### Step 3.7: Implement `normalizeSongName` + `(*Player).PlaySong`

- [ ] Open `modules/world/player_script.go`. At the top of the file, check the import block — ensure `"strings"` is imported. If not, add it.
- [ ] Append to the file (trailing, after `(*Player).LowMemory` from Task 2):

```go
// normalizeSongName mirrors TS Player.playSong's normalization step
// (Engine-TS/src/engine/entity/Player.ts:1903) — lowercase + spaces
// replaced by underscores. Extracted for direct testability given
// PlaySong's current no-op write body (S7h-D1). Asymmetric with
// normalizeJingleName (spaces→underscores vs. underscores→spaces);
// the asymmetry is TS-intentional — songs key into disk with
// underscore filenames; jingles key into a space-separated title map.
func normalizeSongName(name string) string {
	return strings.ReplaceAll(strings.ToLower(name), " ", "_")
}

// PlaySong normalizes the song name per TS Player.playSong
// (Engine-TS/src/engine/entity/Player.ts:1902-1914) and early-returns
// on empty.
//
// S7h-D1: the subsequent TS PRELOADED + PRELOADED_CRC lookup and
// MidiSong(name, crc, length) write from TS is not yet ported.
// goscape lacks the PRELOADED music registry (zero rg hits at
// HEAD=25bef29). No client packet is sent. The TestPlaySongNoWriteOut
// absence-pin (player_script_test.go) escalates this deviation when
// the write path is wired; retirement tracked as NAI-16-midi-encoders.
func (p *Player) PlaySong(name string) {
	name = normalizeSongName(name)
	if name == "" {
		return
	}
	// deferred (S7h-D1): PRELOADED lookup + p.writeOut(gameserver.OpMidiSong, ...)
}
```

### Step 3.8: Implement `handleMidiSong`

- [ ] Open `pkg/script/handlers_player.go`. Check imports — ensure `"errors"` is present (it is already, used by sibling handlers). Append the following to the file (trailing location):

```go
// handleMidiSong (MIDI_SONG, opcode 2064) plays a MIDI song by name to
// the active player. Silent no-op if the player has lowMemory set.
// Mirrors TS PlayerOps.ts:796-804.
//
// Pointer gate: require active_player (TS ScriptOpcodePointers.ts:272
// require: ['active_player']).
//
// S7h-D1: downstream (*Player).PlaySong currently performs TS name
// normalization + early-return only; no MidiSong client packet is sent.
func handleMidiSong(s *ScriptState) error {
	name := s.PopString()
	if err := checkStringNotNull(name, "MIDI_SONG"); err != nil {
		return err
	}
	if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
		return errors.New("MIDI_SONG: no active player")
	}
	if s.Self.LowMemory() {
		return nil
	}
	s.Self.PlaySong(name)
	return nil
}
```

### Step 3.9: Register `OpMidiSong` in the dispatch map

- [ ] Open `pkg/script/handlers.go`. Locate the handler dispatch map — it is the long `var handlers = map[Opcode]Handler{ ... }` block (or similar map declaration; the opcode dispatch). Find a grouping that makes sense for audio-related opcodes. The most natural spot is near the `CamReset` / `StaffModLevel` / `UID` cluster (those are the "account / session / device" group). Insert a new two-line section:

```go
	// S7h: audio — MIDI_SONG (MIDI_JINGLE lands in Task 4).
	OpMidiSong: handleMidiSong,
```

Placement note: keep the trailing comma. Exact line position is not strict — what matters is that the map literal still compiles and the new entry is in a sensible cluster (audio / session). If uncertain, place it immediately after the `OpCamReset: handleCamReset,` entry.

### Step 3.10: Run all new tests

- [ ] Run:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ ./pkg/script/ -run "MidiSong|NormalizeSongName|PlaySong" -v
```

Expected: all nine tests PASS:
- `TestNormalizeSongNameLowercaseAndSpacesToUnderscores` (4 sub-cases)
- `TestNormalizeSongNameEmptyReturnsEmpty`
- `TestPlaySongNoWriteOut`
- `TestPlaySongEmptyNameReturnsSilently`
- `TestMidiSongHappyPath`
- `TestMidiSongLowMemoryBails`
- `TestMidiSongNullStringRejects`
- `TestMidiSongNoActivePlayerRejects`

### Step 3.11: Run full test suite

- [ ] Run:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
```

Expected: build clean, all packages green. No unused-import complaints on `strings` in `player_script.go`.

### Step 3.12: Commit

- [ ] Run:

```bash
git add pkg/script/handlers_player.go pkg/script/handlers_player_test.go pkg/script/handlers.go pkg/script/active.go pkg/script/runner_test.go modules/world/player_script.go modules/world/player_script_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): S7h Task 3 — MIDI_SONG (2064) handler + PlaySong stub

Ports the MIDI_SONG script opcode faithfully (pop+validate+active-player
gate+lowMemory bail+method invocation) and adds (*Player).PlaySong which
performs TS name normalization (lowercase + spaces→underscores) and
early-returns on empty. Per S7h-D1, no client packet is sent pending
PRELOADED music / CRC infrastructure; TestPlaySongNoWriteOut absence-pin
escalates when the encoder path is wired in NAI-16-midi-encoders.

Extracted normalizeSongName helper gives the TS-fidelity normalization
step a directly testable observation point while PlaySong itself has
no observable output in S7h.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: `MIDI_JINGLE` full stack

**Files:**
- Modify: `pkg/script/handlers_player.go` (add `handleMidiJingle`)
- Modify: `pkg/script/handlers.go` (register `OpMidiJingle`)
- Modify: `pkg/script/active.go` (add `PlayJingle` to `ActivePlayer`)
- Modify: `pkg/script/runner_test.go` (add `playJingleCalls` + `PlayJingle` impl to mock)
- Modify: `modules/world/player_script.go` (add `normalizeJingleName` + `(*Player).PlayJingle`)
- Test: `pkg/script/handlers_player_test.go` (handler dispatch tests)
- Test: `modules/world/player_script_test.go` (normalizer unit tests + absence-pin)

### Step 4.1: Write failing `normalizeJingleName` unit tests

- [ ] Append to `modules/world/player_script_test.go`:

```go
func TestNormalizeJingleNameLowercaseAndUnderscoresToSpaces(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"a_quick_jingle", "a quick jingle"},
		{"Space Already", "space already"},
		{"ALLCAPS", "allcaps"},
		{"Mixed_CASE_With_Underscores", "mixed case with underscores"},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			if got := normalizeJingleName(tc.in); got != tc.want {
				t.Errorf("normalizeJingleName(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestNormalizeJingleNameEmptyReturnsEmpty(t *testing.T) {
	if got := normalizeJingleName(""); got != "" {
		t.Errorf("normalizeJingleName(\"\") = %q, want \"\"", got)
	}
}
```

### Step 4.2: Write failing `PlayJingle` absence-pin + empty-return tests

- [ ] Append to `modules/world/player_script_test.go`:

```go
// TestPlayJingleNoWriteOut pins S7h-D1: (*Player).PlayJingle must NOT
// issue a writeOut until PRELOADED music infra lands (NAI-16). When
// the encoder ports, this test fails — signal to retire S7h-D1.
func TestPlayJingleNoWriteOut(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.PlayJingle(3, "fanfare")
	if n := p.client.bufw.Buffered(); n != 0 {
		t.Errorf("PlayJingle wrote %d bytes to c.bufw; want 0 (S7h-D1 absence-pin)", n)
	}
}

func TestPlayJingleEmptyNameReturnsSilently(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.PlayJingle(3, "")
	if n := p.client.bufw.Buffered(); n != 0 {
		t.Errorf("empty name: PlayJingle wrote %d bytes; want 0", n)
	}
}
```

### Step 4.3: Write failing handler tests

- [ ] Append to `pkg/script/handlers_player_test.go`:

```go
func TestMidiJingleHappyPath(t *testing.T) {
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Self:        &mockPlayer{},
		Pointers:    PtrActivePlayer,
	}
	// Pop order in handler: delay first (top-of-stack), then name.
	// Push order: name (deepest), delay (topmost).
	s.PushString("fanfare")
	s.PushInt(3)
	mp := s.Self.(*mockPlayer)

	if err := handleMidiJingle(s); err != nil {
		t.Fatalf("handleMidiJingle: %v", err)
	}
	if len(mp.playJingleCalls) != 1 {
		t.Fatalf("playJingleCalls: got %d, want 1", len(mp.playJingleCalls))
	}
	if mp.playJingleCalls[0].delay != 3 || mp.playJingleCalls[0].name != "fanfare" {
		t.Errorf("playJingleCalls[0]: got {delay:%d, name:%q}, want {delay:3, name:\"fanfare\"}",
			mp.playJingleCalls[0].delay, mp.playJingleCalls[0].name)
	}
}

func TestMidiJingleLowMemoryBails(t *testing.T) {
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Self:        &mockPlayer{lowMemoryValue: true},
		Pointers:    PtrActivePlayer,
	}
	s.PushString("fanfare")
	s.PushInt(3)
	mp := s.Self.(*mockPlayer)

	if err := handleMidiJingle(s); err != nil {
		t.Fatalf("handleMidiJingle: %v", err)
	}
	if len(mp.playJingleCalls) != 0 {
		t.Errorf("lowMemory=true: playJingleCalls=%d, want 0", len(mp.playJingleCalls))
	}
}

func TestMidiJingleNullStringRejects(t *testing.T) {
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Self:        &mockPlayer{},
		Pointers:    PtrActivePlayer,
	}
	s.PushString("")
	s.PushInt(3)

	err := handleMidiJingle(s)
	if err == nil {
		t.Fatal("empty name: want error, got nil")
	}
	if !strings.Contains(err.Error(), "MIDI_JINGLE: input string was null") {
		t.Errorf("error %q does not contain %q", err.Error(), "MIDI_JINGLE: input string was null")
	}
}

func TestMidiJingleNullDelayRejects(t *testing.T) {
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Self:        &mockPlayer{},
		Pointers:    PtrActivePlayer,
	}
	s.PushString("fanfare")
	s.PushInt(-1)

	err := handleMidiJingle(s)
	if err == nil {
		t.Fatal("delay=-1: want error, got nil")
	}
	if !strings.Contains(err.Error(), "MIDI_JINGLE: input number was null(-1)") {
		t.Errorf("error %q does not contain %q", err.Error(), "MIDI_JINGLE: input number was null(-1)")
	}
}

func TestMidiJingleNoActivePlayerRejects(t *testing.T) {
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Self:        nil,
		Pointers:    0,
	}
	s.PushString("fanfare")
	s.PushInt(3)

	err := handleMidiJingle(s)
	if err == nil {
		t.Fatal("no active player: want error, got nil")
	}
	if !strings.Contains(err.Error(), "MIDI_JINGLE: no active player") {
		t.Errorf("error %q does not contain %q", err.Error(), "MIDI_JINGLE: no active player")
	}
}
```

### Step 4.4: Run tests to verify failure

- [ ] Run:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ ./pkg/script/ -run "MidiJingle|NormalizeJingleName|PlayJingle" -v
```

Expected: compile errors for `normalizeJingleName`, `p.PlayJingle`, `handleMidiJingle`, `mp.playJingleCalls`.

### Step 4.5: Add `PlayJingle` to `ActivePlayer` interface

- [ ] Open `pkg/script/active.go`. Append to the `ActivePlayer` interface body, immediately after the `PlaySong` declaration added in Task 3:

```go
	// PlayJingle sends a short MIDI jingle by name to the client. Called
	// by the MIDI_JINGLE script opcode (PlayerOps.ts:806-816).
	//
	// S7h-D1: actual MidiJingle client packet is deferred pending
	// PRELOADED music infrastructure; current impl performs TS name
	// normalization (lowercase + underscores→spaces) and early-returns
	// on empty without writing.
	PlayJingle(delay int, name string)
```

### Step 4.6: Extend `mockPlayer` with `playJingleCalls` field + `PlayJingle` method

- [ ] Open `pkg/script/runner_test.go`. In the `mockPlayer` struct body, append (right after the `playSongCalls` field added in Task 3):

```go
	// S7h: captured MIDI_JINGLE plays. Each entry records the delay and
	// the normalized-name argument as seen by the mock.
	playJingleCalls []struct {
		delay int
		name  string
	}
```

- [ ] After the `func (m *mockPlayer) PlaySong(...)` impl added in Task 3, append:

```go
// S7h: PlayJingle captures the MIDI_JINGLE delay + name for handler tests.
func (m *mockPlayer) PlayJingle(delay int, name string) {
	m.playJingleCalls = append(m.playJingleCalls, struct {
		delay int
		name  string
	}{delay, name})
}
```

### Step 4.7: Implement `normalizeJingleName` + `(*Player).PlayJingle`

- [ ] Open `modules/world/player_script.go`. Append (trailing, after `(*Player).PlaySong` from Task 3):

```go
// normalizeJingleName mirrors TS Player.playJingle's normalization step
// (Engine-TS/src/engine/entity/Player.ts:1917) — lowercase + underscores
// replaced by spaces. Extracted for direct testability given
// PlayJingle's current no-op write body (S7h-D1). Asymmetric with
// normalizeSongName (underscores→spaces vs. spaces→underscores);
// the asymmetry is TS-intentional — jingles key into a space-separated
// title map; songs key into underscore-filename disk paths.
func normalizeJingleName(name string) string {
	return strings.ReplaceAll(strings.ToLower(name), "_", " ")
}

// PlayJingle normalizes the jingle name per TS Player.playJingle
// (Engine-TS/src/engine/entity/Player.ts:1916-1926) and early-returns
// on empty.
//
// S7h-D1: the subsequent TS PRELOADED lookup and MidiJingle(delay, data)
// write from TS is not yet ported. No client packet is sent. The
// TestPlayJingleNoWriteOut absence-pin (player_script_test.go)
// escalates this deviation when the write path is wired; retirement
// tracked as NAI-16-midi-encoders.
func (p *Player) PlayJingle(delay int, name string) {
	_ = delay // preserved for future MidiJingle encoder wiring
	name = normalizeJingleName(name)
	if name == "" {
		return
	}
	// deferred (S7h-D1): PRELOADED lookup + p.writeOut(gameserver.OpMidiJingle, ...)
}
```

### Step 4.8: Implement `handleMidiJingle`

- [ ] Open `pkg/script/handlers_player.go`. Append (trailing, after `handleMidiSong` from Task 3):

```go
// handleMidiJingle (MIDI_JINGLE, opcode 2063) plays a short MIDI jingle
// by name and delay to the active player. Silent no-op if the player
// has lowMemory set. Mirrors TS PlayerOps.ts:806-816.
//
// Pointer gate: require active_player (TS ScriptOpcodePointers.ts:269
// require: ['active_player']).
//
// Pop order (top-of-stack first): delay (NumberNotNull), then name
// (StringNotNull). Matches TS `check(state.popInt(), NumberNotNull)` /
// `check(state.popString(), StringNotNull)` evaluation order.
//
// S7h-D1: downstream (*Player).PlayJingle currently performs TS name
// normalization + early-return only; no MidiJingle client packet is sent.
func handleMidiJingle(s *ScriptState) error {
	delay := s.PopInt()
	if err := checkNotNull(delay, "MIDI_JINGLE"); err != nil {
		return err
	}
	name := s.PopString()
	if err := checkStringNotNull(name, "MIDI_JINGLE"); err != nil {
		return err
	}
	if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
		return errors.New("MIDI_JINGLE: no active player")
	}
	if s.Self.LowMemory() {
		return nil
	}
	s.Self.PlayJingle(delay, name)
	return nil
}
```

### Step 4.9: Register `OpMidiJingle` in the dispatch map

- [ ] Open `pkg/script/handlers.go`. Find the `OpMidiSong: handleMidiSong,` row added in Task 3. Update the preceding comment and add a sibling row:

```go
	// S7h: audio — MIDI_SONG + MIDI_JINGLE.
	OpMidiJingle: handleMidiJingle,
	OpMidiSong:   handleMidiSong,
```

Numeric order inside the cluster is ascending (2063 before 2064). Vertical alignment of the arrows is gofmt-managed — on save/build it will align; don't spend effort aligning by hand.

### Step 4.10: Run all new tests

- [ ] Run:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ ./pkg/script/ -run "MidiJingle|NormalizeJingleName|PlayJingle" -v
```

Expected: all tests PASS:
- `TestNormalizeJingleNameLowercaseAndUnderscoresToSpaces` (4 sub-cases)
- `TestNormalizeJingleNameEmptyReturnsEmpty`
- `TestPlayJingleNoWriteOut`
- `TestPlayJingleEmptyNameReturnsSilently`
- `TestMidiJingleHappyPath`
- `TestMidiJingleLowMemoryBails`
- `TestMidiJingleNullStringRejects`
- `TestMidiJingleNullDelayRejects`
- `TestMidiJingleNoActivePlayerRejects`

### Step 4.11: Run full test suite

- [ ] Run:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...
```

Expected: build + test + vet all clean.

### Step 4.12: Commit

- [ ] Run:

```bash
git add pkg/script/handlers_player.go pkg/script/handlers_player_test.go pkg/script/handlers.go pkg/script/active.go pkg/script/runner_test.go modules/world/player_script.go modules/world/player_script_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): S7h Task 4 — MIDI_JINGLE (2063) handler + PlayJingle stub

Bundled with the MIDI_SONG cluster (S7h Task 3) per S7f/S7g cluster
pattern. Handler mirrors TS PlayerOps.ts:806-816 exactly:
delay (NumberNotNull) pops first, then name (StringNotNull), then
active-player + lowMemory gate, then method invocation.

(*Player).PlayJingle applies TS name normalization (lowercase +
underscores→spaces — asymmetric with song normalization by TS design)
and early-returns on empty. Per S7h-D1, no client packet is sent
pending PRELOADED music infrastructure (NAI-16-midi-encoders).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: Close — final acceptance gates + close commit

**Files:** None modified — this task is verification + close-commit.

### Step 5.1: Run full verification suite

- [ ] Run each command and confirm expected output (per spec acceptance gates 1-8):

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...
```

Expected: all clean. If any gate fails, diagnose and fix — do not proceed to the close commit.

### Step 5.2: Confirm dispatch registrations

- [ ] Run:

```bash
rg -n "OpMidiSong|OpMidiJingle" pkg/script/handlers.go
```

Expected output: two lines, one per opcode, inside the dispatch map. If there are more or fewer, inspect and correct.

### Step 5.3: Confirm lowMemory plumbing touch-sites

- [ ] Run:

```bash
rg -n "lowMemory" modules/world/ pkg/script/
```

Expected sites (exactly this set):
- `modules/world/client.go` — field declaration
- `modules/world/server.go` — `c.lowMemory = req.LowMemory`
- `modules/world/player.go` — `lowMemory: c.lowMemory,` in `newPlayer` literal, plus the existing `reconnecting, lowMemory, webClient bool` declaration on the Player struct
- `modules/world/player_script.go` — `(*Player).LowMemory()` method
- `modules/world/player_test.go` — `TestNewPlayerCopiesLowMemoryFromClient`
- `modules/world/player_script_test.go` — `TestPlayerLowMemoryGetter`
- `pkg/script/active.go` — interface `LowMemory()` decl
- `pkg/script/runner_test.go` — `lowMemoryValue` field + `LowMemory()` impl
- `pkg/script/handlers_player.go` — `s.Self.LowMemory()` in `handleMidiSong` and `handleMidiJingle`
- `pkg/script/handlers_player_test.go` — `lowMemoryValue` in `TestMidiSongLowMemoryBails` and `TestMidiJingleLowMemoryBails`

No stragglers. If something unexpected surfaces (e.g. in a file not on this list), investigate.

### Step 5.4: Confirm S7h-D1 absence (no writeOut call for MIDI ops)

- [ ] Visually inspect `modules/world/player_script.go`:

```bash
awk '/^func \(p \*Player\) (PlaySong|PlayJingle)/,/^}/' modules/world/player_script.go
```

Neither body should contain a `p.writeOut(` call. Both bodies should show only: normalize → if empty, return → trailing `// deferred (S7h-D1): ...` comment.

- [ ] Cross-check:

```bash
rg -n "writeOut.*OpMidiSong|writeOut.*OpMidiJingle" modules/
```

Expected output: **zero lines**.

### Step 5.5: Confirm `prot.go` untouched

- [ ] Run:

```bash
git diff main...HEAD -- pkg/io/protocol/game/server/prot.go
```

Expected output: **empty**. If anything shows up here, S7h-D1's "no prot.go touch" invariant is violated — revert.

### Step 5.6: Struct-literal enumeration per `plan_enumerate_struct_literals`

- [ ] Run:

```bash
rg -n "Player\{" modules/world/ --type go | grep -v "_test.go:.*newPlayer"
```

For each non-`newPlayer` `Player{...}` literal, confirm the literal intent does not need `lowMemory`. Known sites from HEAD=25bef29:
- `modules/world/server_test.go:359` — `&Player{slot: i}` — slot-only test, `lowMemory=false` default is fine.
- `modules/world/server_test.go:449` — `&Player{slot: i}` — same.
- `modules/world/server_test.go:359` and `:449` appear twice each in the probe; both are bare `{slot: i}` literals.

If rg surfaces any **new** literal that sets bool fields like `reconnecting:` / `webClient:` / similar, inspect it — the implementer may need to add `lowMemory:` for test intent.

### Step 5.7: Confirm no stale carry-forward deviation list drift

- [ ] Read the "Pre-existing deviations carried forward" line in `docs/superpowers/specs/2026-04-24-runescript-s7h-midi-song-design.md` (line 448):

Expected: `S7a-D1, S7a-D2, S7b-D1, S7c-D1, S7d-D1, S7d-D2, S7d-D3, S7d-D4, S7e-D1, S7f-D1, S7f-D2, S7f-D3, S7g-D1, S7g-D2, S7g-D3.`

Expected active count after S7h close: **16** (15 carried + 1 new).

No spec edit is needed unless the list drifted — the spec is already committed as `8048d91`.

### Step 5.8: Close commit

- [ ] Run:

```bash
git commit --allow-empty --no-gpg-sign -m "$(cat <<'EOF'
chore(script): S7h closed — MIDI_SONG + MIDI_JINGLE + lowMemory plumbing

Handlers for opcodes 2063 (MIDI_JINGLE) and 2064 (MIDI_SONG) land with
full TS-fidelity dispatch: StringNotNull validation, active-player
pointer gate, lowMemory bail, and Play{Song,Jingle} method invocation.
lowMemory is plumbed from req.LowMemory through client.lowMemory into
the Player struct literal at newPlayer, making the gate actually gate.

Single deviation S7h-D1: (*Player).PlaySong / .PlayJingle perform TS
name normalization and early-return-on-empty but make no writeOut call,
pending PRELOADED music / CRC infrastructure. normalizeSongName /
normalizeJingleName extracted as unexported helpers for direct
testability. OpMidiSong / OpMidiJingle intentionally NOT registered
in pkg/io/protocol/game/server/prot.go — avoids dead-API wire ops.

Absence-pin tests (TestPlaySongNoWriteOut, TestPlayJingleNoWriteOut)
escalate loudly when NAI-16 ports the encoder.

Unblocks [label,music_playbyregion] past pc=74 (prior stall after
S7g closed pc=21).

Active deviations: 16 (15 carried + 1 new).

Closes memory: nai_followups

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

This is an intentional empty commit (no file diff) marking the sub-spec close — all code landed in Tasks 1-4. If you prefer a non-empty close commit, let the reviewer polish land here instead.

### Step 5.9: Final sanity check

- [ ] Run:

```bash
git log --oneline main..HEAD
```

Expected: five commits (Tasks 1-4 + close), newest first:
- `<hash>` `chore(script): S7h closed — ...`
- `<hash>` `feat(script): S7h Task 4 — MIDI_JINGLE (2063) ...`
- `<hash>` `feat(script): S7h Task 3 — MIDI_SONG (2064) ...`
- `<hash>` `feat(script): S7h Task 2 — lowMemory plumbing ...`
- `<hash>` `feat(script): S7h Task 1 — checkStringNotNull ...`

Plus the earlier spec + plan `docs(spec):` / `docs(plan):` commits.

---

## Post-implementation smoke test (user-driven)

Per `smoke_test_server_handoff`, after Task 5 closes, hand off to the user:

> User: run `CGO_ENABLED=0 go run -trimpath ./cmd/goscape --config.file config.yaml` and exercise `[label,music_playbyregion]`. Expected: script advances past pc=74 (no `no handler for MIDI_SONG` / `no handler for MIDI_JINGLE`). Report the next stall (if any) as S7i. Secondary watch: whether `combat_get_damagetype` finally exercises `DB_GETFIELD` cleanly.

---

## Spec-to-plan coverage cross-check

Per `plan_test_coverage_crosscheck` memory, each spec-listed test maps to exactly one task step:

| Spec test | Plan location |
|---|---|
| `TestCheckStringNotNullEmpty` / `_NonEmpty` | Task 1 step 1.1 |
| `TestNewPlayerCopiesLowMemoryFromClient` | Task 2 step 2.1 |
| `TestPlayerLowMemoryGetter` | Task 2 step 2.2 |
| `TestNormalizeSongName_*` (LowercaseAndSpacesToUnderscores, EmptyReturnsEmpty) | Task 3 step 3.1 |
| `TestPlaySongNoWriteOut`, `TestPlaySongEmptyNameReturnsSilently` | Task 3 step 3.2 |
| `TestMidiSongHappyPath`, `TestMidiSongLowMemoryBails`, `TestMidiSongNullStringRejects`, `TestMidiSongNoActivePlayerRejects` | Task 3 step 3.3 |
| `TestNormalizeJingleName_*` | Task 4 step 4.1 |
| `TestPlayJingleNoWriteOut`, `TestPlayJingleEmptyNameReturnsSilently` | Task 4 step 4.2 |
| `TestMidiJingleHappyPath`, `TestMidiJingleLowMemoryBails`, `TestMidiJingleNullStringRejects`, `TestMidiJingleNullDelayRejects`, `TestMidiJingleNoActivePlayerRejects` | Task 4 step 4.3 |

Per `plan_helper_coverage` memory: no shared test helpers are introduced in this plan (the only common helper is `newTestPlayer` which already exists and is reused), so no flag-set enumeration is required.

---

## Spec-to-plan requirement cross-check

| Spec requirement | Implemented by |
|---|---|
| `checkStringNotNull` validator | Task 1 steps 1.3 |
| `handleMidiSong` handler | Task 3 step 3.8 |
| `handleMidiJingle` handler | Task 4 step 4.8 |
| `OpMidiSong` / `OpMidiJingle` dispatch registration | Task 3 step 3.9 + Task 4 step 4.9 |
| `ActivePlayer.LowMemory() bool` | Task 2 step 2.7 |
| `ActivePlayer.PlaySong(name string)` | Task 3 step 3.5 |
| `ActivePlayer.PlayJingle(delay int, name string)` | Task 4 step 4.5 |
| `(*Player).LowMemory()` | Task 2 step 2.8 |
| `(*Player).PlaySong` + `normalizeSongName` | Task 3 step 3.7 |
| `(*Player).PlayJingle` + `normalizeJingleName` | Task 4 step 4.7 |
| `client.lowMemory` field | Task 2 step 2.4 |
| `c.lowMemory = req.LowMemory` in server.go | Task 2 step 2.5 |
| `lowMemory: c.lowMemory` in newPlayer | Task 2 step 2.6 |
| `mockPlayer.lowMemoryValue` + `LowMemory()` impl | Task 2 step 2.9 |
| `mockPlayer.playSongCalls` + `PlaySong()` impl | Task 3 step 3.6 |
| `mockPlayer.playJingleCalls` + `PlayJingle()` impl | Task 4 step 4.6 |
| Acceptance gates 1-8 | Task 5 steps 5.1-5.7 |
| `S7h-D1` retained single | Task 5 step 5.8 close commit body |
| `Closes memory: nai_followups` trailer | Task 5 step 5.8 |
| `prot.go` untouched | Task 5 step 5.5 |

No spec requirement lacks a plan step.
