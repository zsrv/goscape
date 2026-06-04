# NAI-76 TUT_OPEN Handler Port Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port the `TUT_OPEN` script opcode (2122) to silence the per-login `[proc,tutorialstep_page]` runtime error and unblock the Tutorial Island chatnpc cascade (door interaction, click-away modal dismiss).

**Architecture:** Single-feature port mirroring the existing `IF_OPEN_*` opcode pattern. New wire opcode `OpTutOpen=185` in `pkg/io/protocol/game/server`, new `OpenTutorial(com int)` method on `*world.Player` that OR's a new `modalStateTut` bit and stores the component id without closing other modals (per TS `Player.openTutorial`), new diff-driven emit branch in `Player.encodeOut`, and new `handleTutOpen` in `pkg/script/handlers_interface.go` rejecting `-1` via the existing `checkNotNull` helper.

**Tech Stack:** Go 1.26+. Binary I/O via `pkg/io/packet` + `pkg/io/protocol`. Script handlers in `pkg/script` consuming the `ActivePlayer` interface from `pkg/script/active.go`. Implementer dispatch via `superpowers:subagent-driven-development`. Java-client smoke per `smoke_test_server_handoff.md`.

---

## Spec reference

`docs/superpowers/specs/2026-05-03-nai-76-tut-open-cascade-fix-design.md` (commit `31931c9`).

## Pre-flight verifications (controller-confirmed against HEAD `6081e17`)

| # | Premise | Verified at |
|---|---|---|
| P1 | `OpTutOpen Opcode = 2122` declared, no handler | `pkg/script/opcode.go:222`; absent from dispatch map at `pkg/script/handlers.go:281-285` (which only contains IF_OPEN_*) |
| P2 | `modalTutorial int` field exists, init `-1` in newPlayer | `modules/world/player.go:246`, `:431` |
| P3 | `modalStateNone/Main/Chat/Side` constants present, no `Tut` | `modules/world/player.go:35-38` |
| P4 | `lastModalMain/Chat/Side` present, no `lastModalTutorial` | `modules/world/player.go:244` |
| P5 | `OpenMain/Chat/Side/MainSide` exist on `*Player`, no `OpenTutorial` | `modules/world/player_script.go:738-776` |
| P6 | `ActivePlayer` interface declares `OpenMain/Chat/Side/MainSide`, no `OpenTutorial` | `pkg/script/active.go:148-162` |
| P7 | `mockPlayer` has `lastOpenMain/Chat/Side/MainSide` capture fields, no `lastOpenTutorial` | `pkg/script/runner_test.go:155-159, 420-425` |
| P8 | `checkNotNull` helper exists at `pkg/script/handlers_player.go:71-76`; error literal `"%s: input number was null(-1)"` | `handlers_player.go:71-76` |
| P9 | `handlers_interface.go` uses inline `s.Pointers&PtrActivePlayer == 0 \|\| s.Self == nil` gate (NOT the `requireActivePlayer` helper from `handlers_player.go:35`) | `handlers_interface.go:16, 26, 40, 54, 70` |
| P10 | `prot.go` `Op{}` literal shape: `Opcode byte`, `PayloadSize int` | `pkg/io/protocol/game/server/prot.go:4-7` |
| P11 | `encodeOut` emit path uses `p.writeOut(gameserver.OpX, payload)` with payload manually MSB-packed | `modules/world/player.go:347-363` |
| P12 | TS `TUT_OPEN` handler has `check(_, NumberNotNull)` | `LostCityRS/Engine-TS/src/engine/script/handlers/PlayerOps.ts:723-725` |
| P13 | TS `Player.openTutorial(com)` writes `TutOpen(com)` + `modalState |= ModalState.TUT` + `modalTutorial = com`; does NOT close other modals | `LostCityRS/Engine-TS/src/engine/entity/Player.ts:1999-2003` |
| P14 | TS `ServerGameProt.TUT_OPEN = (185, 2)` | `LostCityRS/Engine-TS/src/network/game/server/ServerGameProt.ts:25` |
| P15 | TS `TutOpenEncoder` writes `p2(message.component)` (2-byte big-endian) | `LostCityRS/Engine-TS/src/network/game/server/codec/TutOpenEncoder.ts:9-11` |

All 15 premises hold. Proceed to tasks.

---

## Task 1: Foundation (compressed cadence — no TDD; mechanical scaffolding)

Per `compressed_cadence.md`: ~25 LOC across 5 files; no behavior change reachable from production code paths until T2/T3 wire it up. Compile-only verification gate.

**Files:**
- Modify: `pkg/io/protocol/game/server/prot.go`
- Modify: `modules/world/player.go:35-38, 244`
- Modify: `modules/world/player.go::newPlayer (~line 431)`
- Modify: `pkg/script/active.go:160-162`
- Modify: `pkg/script/runner_test.go:159, 425`

- [ ] **Step 1.1: Add `OpTutOpen` to wire prot.**

In `pkg/io/protocol/game/server/prot.go`, insert immediately after `OpIfOpenMainSide` at line 16:

```go
	OpIfOpenMainSide = Op{Opcode: 28, PayloadSize: 4}
	OpTutOpen        = Op{Opcode: 185, PayloadSize: 2}
	OpLogout         = Op{Opcode: 142, PayloadSize: 0}
```

Source: `LostCityRS/Engine-TS/src/network/game/server/ServerGameProt.ts:25` — `TUT_OPEN = new ServerGameProt(185, 2)`.

- [ ] **Step 1.2: Add `modalStateTut` constant.**

In `modules/world/player.go` at lines 35-38, add:

```go
const (
	modalStateNone = 0x0
	modalStateMain = 0x1
	modalStateChat = 0x2
	modalStateSide = 0x4
	modalStateTut  = 0x8
)
```

(This may be a `var` block or `const` block at HEAD — match the existing form in the file.)

- [ ] **Step 1.3: Add `lastModalTutorial` field.**

In `modules/world/player.go`, modify line 244 from:

```go
	lastModalMain, lastModalChat, lastModalSide        int
```

to:

```go
	lastModalMain, lastModalChat, lastModalSide        int
	lastModalTutorial                                  int
```

- [ ] **Step 1.4: Add `lastModalTutorial: -1` init.**

In `modules/world/player.go::newPlayer` (~line 431), add `lastModalTutorial: -1,` immediately after the existing `modalTutorial: -1,` line:

```go
		modalTutorial:     -1,
		lastModalTutorial: -1,
		tabs:              [14]int{...},
```

(Adjust the trailing-comma alignment to match the existing struct-literal style at HEAD.)

- [ ] **Step 1.5: Add `OpenTutorial` to ActivePlayer interface.**

In `pkg/script/active.go`, immediately after the `OpenMainSide(mainCom, sideCom int)` line at ~162, add:

```go
	// OpenMainSide opens mainCom as the main modal and sideCom as the
	// side modal simultaneously.
	OpenMainSide(mainCom, sideCom int)

	// OpenTutorial opens com as the tutorial-overlay component. Per TS,
	// opening the tutorial does NOT close any other modal — the TUT bit
	// is OR'd into modalState. Mirrors LostCityRS/Engine-TS
	// Player.ts:1999-2003 (openTutorial).
	OpenTutorial(com int)
```

- [ ] **Step 1.6: Add mockPlayer field + impl.**

In `pkg/script/runner_test.go`:

(a) Insert `lastOpenTutorial int` after `lastOpenMainSide` at line 159:

```go
	lastOpenMain        int
	lastOpenChat        int
	lastOpenSide        int
	lastOpenMainSide    struct{ main, side int }
	lastOpenTutorial    int
```

(b) Insert the method body after the existing `OpenMainSide` method at ~line 425:

```go
func (m *mockPlayer) OpenMainSide(mainCom, sideCom int) {
	m.lastOpenMainSide = struct{ main, side int }{mainCom, sideCom}
}

func (m *mockPlayer) OpenTutorial(com int) { m.lastOpenTutorial = com }
```

- [ ] **Step 1.7: Verify compile.**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`
Expected: success, no output.

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...`
Expected: success, no output.

- [ ] **Step 1.8: Verify all existing tests still pass.**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... -count=1`
Expected: PASS (no behavior change at HEAD; the new interface method is implemented by mockPlayer and *world.Player will be implemented in T2 — but T1 doesn't introduce a new caller of OpenTutorial yet, so the *world.Player interface satisfaction is checked at T2 when go build pulls the dispatch in via t/handlers.go).

**WAIT — interface satisfaction caveat.** Adding `OpenTutorial(com int)` to the `ActivePlayer` interface in step 1.5 immediately requires *every* implementer to satisfy it, including `*world.Player`. If `*world.Player` doesn't have `OpenTutorial` yet, `go build` of `modules/world` will fail with `*Player does not implement script.ActivePlayer (missing OpenTutorial method)`.

**Resolution:** Step 1.5 must include a stub `OpenTutorial` on `*world.Player`. Insert into `modules/world/player_script.go` immediately after the existing `OpenMainSide` method (~line 776):

```go
// OpenTutorial — stubbed at T1 to satisfy the script.ActivePlayer
// interface; real implementation lands at T2.
func (p *Player) OpenTutorial(com int) {
	// implemented in T2
}
```

This stub MUST be replaced wholesale at T2 step 2.5. T1 commit body should call this out explicitly so T2 can re-grep `// implemented in T2` to find the replacement target.

- [ ] **Step 1.9: Commit.**

```bash
git add pkg/io/protocol/game/server/prot.go modules/world/player.go modules/world/player_script.go pkg/script/active.go pkg/script/runner_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-76 T1 — TUT_OPEN foundation scaffolding

Adds wire opcode OpTutOpen=185 (2-byte payload, mirrors TS
ServerGameProt.TUT_OPEN), modalStateTut=0x8 bit, lastModalTutorial
field on *Player (init -1 mirroring modalTutorial), OpenTutorial
declaration on script.ActivePlayer interface + mockPlayer recorder
(lastOpenTutorial), and a stub OpenTutorial on *world.Player to
satisfy interface (real impl lands at T2). Compile-only step;
no behavior change at HEAD.

Per controller pre-flight verifications P1-P15 against HEAD 6081e17.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: Player.OpenTutorial + encodeOut tutorial-emit branch

TDD task. Write tests first, run red, implement, run green, commit.

**Files:**
- Test: `modules/world/player_test.go` (append test functions)
- Test: `modules/world/player_script_test.go` (append test functions)
- Modify: `modules/world/player.go::encodeOut` (~line 327-364)
- Modify: `modules/world/player_script.go::OpenTutorial` (replace T1 stub)

- [ ] **Step 2.1: Write failing test — `TestOpenTutorial_SetsFieldsWithoutClosingOthers`.**

Append to `modules/world/player_script_test.go`:

```go
// TestOpenTutorial_SetsFieldsWithoutClosingOthers pins TS-fidelity:
// opening the tutorial overlay does NOT close any other modal.
// TS Player.ts:1999-2003 — `this.modalState |= ModalState.TUT;
// this.modalTutorial = com;`. No clear of modalMain/Chat/Side.
func TestOpenTutorial_SetsFieldsWithoutClosingOthers(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.modalMain = 5
	p.modalChat = 7
	p.modalSide = 9
	p.modalState = modalStateMain | modalStateChat | modalStateSide

	p.OpenTutorial(42)

	if p.modalTutorial != 42 {
		t.Errorf("modalTutorial: got %d, want 42", p.modalTutorial)
	}
	wantState := modalStateMain | modalStateChat | modalStateSide | modalStateTut
	if p.modalState != wantState {
		t.Errorf("modalState: got %#x, want %#x", p.modalState, wantState)
	}
	if p.modalMain != 5 {
		t.Errorf("modalMain: got %d, want 5 (must not be cleared)", p.modalMain)
	}
	if p.modalChat != 7 {
		t.Errorf("modalChat: got %d, want 7 (must not be cleared)", p.modalChat)
	}
	if p.modalSide != 9 {
		t.Errorf("modalSide: got %d, want 9 (must not be cleared)", p.modalSide)
	}
}
```

- [ ] **Step 2.2: Write failing test — `TestOpenTutorial_RefreshFlagsUntouched`.**

Append to `modules/world/player_script_test.go`:

```go
// TestOpenTutorial_RefreshFlagsUntouched pins that OpenTutorial uses
// the lastModalTutorial diff-check pattern, NOT the existing
// refreshModal/refreshModalClose flags. The existing flags are
// reserved for the main/chat/side switch in encodeOut.
func TestOpenTutorial_RefreshFlagsUntouched(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.refreshModal = false
	p.refreshModalClose = false

	p.OpenTutorial(42)

	if p.refreshModal {
		t.Error("refreshModal should remain false after OpenTutorial")
	}
	if p.refreshModalClose {
		t.Error("refreshModalClose should remain false after OpenTutorial")
	}
}
```

- [ ] **Step 2.3: Write failing test — `TestEncodeOutSendsTutOpen`.**

Append to `modules/world/player_test.go`. Use the same isaacPair / channel-read pattern as `TestEncodeOutSendsIfOpenMain` at lines 323-362.

```go
func TestEncodeOutSendsTutOpen(t *testing.T) {
	enc, _ := isaacPair([4]uint32{9, 10, 11, 12})
	wantEnc, _ := isaacPair([4]uint32{9, 10, 11, 12})

	p, clientConn := newTestPlayer(t)
	p.client.encryptor = enc
	p.OpenTutorial(42)

	received := make(chan []byte, 1)
	go func() {
		buf := make([]byte, 3) // 1 encrypted opcode + 2 payload bytes
		clientConn.SetReadDeadline(time.Now().Add(time.Second))
		if _, err := io.ReadFull(clientConn, buf); err == nil {
			received <- buf
		}
	}()

	p.encodeOut()
	p.client.flushWrite()

	expectedByte := byte((int(gameserver.OpTutOpen.Opcode) + int(wantEnc.GetNext())) & 0xff)

	select {
	case got := <-received:
		if got[0] != expectedByte {
			t.Errorf("TUT_OPEN encrypted opcode: got %d, want %d", got[0], expectedByte)
		}
		component := int(got[1])<<8 | int(got[2])
		if component != 42 {
			t.Errorf("TUT_OPEN component: got %d, want 42", component)
		}
		if p.lastModalTutorial != 42 {
			t.Errorf("lastModalTutorial: got %d, want 42", p.lastModalTutorial)
		}
	case <-time.After(time.Second):
		t.Error("timed out waiting for TUT_OPEN")
	}
}
```

- [ ] **Step 2.4: Write failing test — `TestEncodeOutTutorialNoChangeNoEmit`.**

Append to `modules/world/player_test.go`:

```go
func TestEncodeOutTutorialNoChangeNoEmit(t *testing.T) {
	enc, _ := isaacPair([4]uint32{13, 14, 15, 16})
	p, clientConn := newTestPlayer(t)
	p.client.encryptor = enc

	// First encodeOut after OpenTutorial(42) is consumed by a
	// separate goroutine; we only care about the second pass.
	p.OpenTutorial(42)
	p.encodeOut()
	p.client.flushWrite()
	clientConn.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
	drain := make([]byte, 16)
	clientConn.Read(drain) // best-effort drain of T_OPEN(42) emit

	// Second pass — no field change.
	p.encodeOut()
	p.client.flushWrite()
	clientConn.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
	buf := make([]byte, 16)
	n, _ := clientConn.Read(buf)
	if n != 0 {
		t.Errorf("expected 0 bytes from no-change encodeOut, got %d", n)
	}
}
```

- [ ] **Step 2.5: Write failing test — `TestEncodeOutTutorialResetEmitsMinusOne`.**

Append to `modules/world/player_test.go`:

```go
// TestEncodeOutTutorialResetEmitsMinusOne pins the wire shape for
// future TUT_CLOSE: setting modalTutorial back to -1 emits
// OpTutOpen with payload [0xFF, 0xFF] (signed -1 → uint16 0xFFFF).
// Mirrors TS Player.closeTutorial Player.ts:716-726 which writes
// `new TutOpen(-1)`. closeTutorial itself is deferred; this test
// pins the diff-check emit path independently.
func TestEncodeOutTutorialResetEmitsMinusOne(t *testing.T) {
	enc, _ := isaacPair([4]uint32{17, 18, 19, 20})
	wantEnc, _ := isaacPair([4]uint32{17, 18, 19, 20})

	p, clientConn := newTestPlayer(t)
	p.client.encryptor = enc

	// Establish lastModalTutorial = 42 via a first emit pass.
	p.OpenTutorial(42)
	p.encodeOut()
	p.client.flushWrite()
	clientConn.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
	drain := make([]byte, 16)
	clientConn.Read(drain)
	wantEnc.GetNext() // consume the encryptor step for the first emit

	// Now reset.
	p.modalTutorial = -1

	received := make(chan []byte, 1)
	go func() {
		buf := make([]byte, 3)
		clientConn.SetReadDeadline(time.Now().Add(time.Second))
		if _, err := io.ReadFull(clientConn, buf); err == nil {
			received <- buf
		}
	}()

	p.encodeOut()
	p.client.flushWrite()

	expectedByte := byte((int(gameserver.OpTutOpen.Opcode) + int(wantEnc.GetNext())) & 0xff)

	select {
	case got := <-received:
		if got[0] != expectedByte {
			t.Errorf("TUT_OPEN(reset) encrypted opcode: got %d, want %d", got[0], expectedByte)
		}
		if got[1] != 0xFF || got[2] != 0xFF {
			t.Errorf("TUT_OPEN(reset) payload: got [%#x %#x], want [0xFF 0xFF]", got[1], got[2])
		}
		if p.lastModalTutorial != -1 {
			t.Errorf("lastModalTutorial: got %d, want -1", p.lastModalTutorial)
		}
	case <-time.After(time.Second):
		t.Error("timed out waiting for TUT_OPEN(-1)")
	}
}
```

- [ ] **Step 2.6: Write failing test — `TestEncodeOutTutorialIndependentOfMain`.**

Append to `modules/world/player_test.go`. This test exercises the non-mutex property: `OpenMain` + `OpenTutorial` in the same tick should both emit.

```go
// TestEncodeOutTutorialIndependentOfMain pins that the tutorial emit
// branch is INDEPENDENT of the main/chat/side switch — opening
// main and tutorial in the same tick produces both packets.
func TestEncodeOutTutorialIndependentOfMain(t *testing.T) {
	enc, _ := isaacPair([4]uint32{21, 22, 23, 24})
	p, clientConn := newTestPlayer(t)
	p.client.encryptor = enc

	p.OpenMain(1234)
	p.OpenTutorial(42)

	// Read up to 6 bytes (3 for IF_OPENMAIN + 3 for TUT_OPEN).
	received := make(chan []byte, 1)
	go func() {
		buf := make([]byte, 6)
		clientConn.SetReadDeadline(time.Now().Add(time.Second))
		if n, err := io.ReadFull(clientConn, buf); err == nil && n == 6 {
			received <- buf
		}
	}()

	p.encodeOut()
	p.client.flushWrite()

	select {
	case got := <-received:
		if len(got) != 6 {
			t.Fatalf("expected 6 bytes (3+3), got %d: %#v", len(got), got)
		}
		// Per encodeOut order, IF_OPENMAIN is emitted first (within
		// the refreshModal switch), then the tutorial branch.
		mainComp := int(got[1])<<8 | int(got[2])
		tutComp := int(got[4])<<8 | int(got[5])
		if mainComp != 1234 {
			t.Errorf("main component: got %d, want 1234", mainComp)
		}
		if tutComp != 42 {
			t.Errorf("tut component: got %d, want 42", tutComp)
		}
	case <-time.After(time.Second):
		t.Error("timed out waiting for IF_OPENMAIN + TUT_OPEN")
	}
}
```

**NOTE for implementer:** if the goroutine read pattern proves flaky for 6-byte reads (e.g. partial reads), restructure to two sequential 3-byte reads — but verify ordering matches the encodeOut emit sequence. Per `plan_runnable_test_fixtures.md`, mentally-execute the fixture before dispatch: `OpenMain(1234)` sets `refreshModal=true, modalState=modalStateMain, modalMain=1234`; `OpenTutorial(42)` sets `modalTutorial=42, modalState |= Tut`; encodeOut then emits IF_OPENMAIN (refreshModal branch, switch case modalStateMain) THEN the new tutorial branch. ✓

- [ ] **Step 2.7: Run tests to verify all FAIL (red phase).**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run 'TestOpenTutorial_|TestEncodeOutSendsTutOpen|TestEncodeOutTutorialNoChangeNoEmit|TestEncodeOutTutorialResetEmitsMinusOne|TestEncodeOutTutorialIndependentOfMain' -v -count=1`

Expected: all 6 tests FAIL. The OpenTutorial tests fail because the T1 stub does nothing (modalTutorial stays unchanged from default; modalState bits not set). The encodeOut tests fail because no tutorial-emit branch exists yet (timeout on read OR `0` bytes received).

- [ ] **Step 2.8: Replace T1 stub with real `OpenTutorial`.**

In `modules/world/player_script.go`, replace the T1 stub at the location grep-discoverable via `// implemented in T2` with:

```go
// OpenTutorial sets the player's tutorial-overlay component. Per TS,
// opening the tutorial does NOT close any other modal — the TUT bit
// is OR'd into modalState and the tutorial id is stored. The
// matching wire packet (OpTutOpen) is deferred to the next
// encodeOut pass which detects the modalTutorial != lastModalTutorial
// diff. Mirrors LostCityRS/Engine-TS Player.ts:1999-2003.
func (p *Player) OpenTutorial(com int) {
	p.modalTutorial = com
	p.modalState |= modalStateTut
}
```

- [ ] **Step 2.9: Add tutorial-emit branch to `encodeOut`.**

In `modules/world/player.go::encodeOut`, immediately after the closing `}` of the existing `if p.refreshModal { switch { ... } p.refreshModal = false }` block (~line 363, before the function-closing `}` at ~line 364), insert:

```go
	if p.modalTutorial != p.lastModalTutorial {
		payload := []byte{byte(p.modalTutorial >> 8), byte(p.modalTutorial)}
		p.writeOut(gameserver.OpTutOpen, payload)
		p.lastModalTutorial = p.modalTutorial
	}
```

The diff-check pattern mirrors the `lastModalMain/Chat/Side` change-detection at lines 328-345. No new refresh flag needed; the diff handles open (com=N) and reset (com=-1) symmetrically.

- [ ] **Step 2.10: Run tests to verify all PASS (green phase).**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run 'TestOpenTutorial_|TestEncodeOutSendsTutOpen|TestEncodeOutTutorialNoChangeNoEmit|TestEncodeOutTutorialResetEmitsMinusOne|TestEncodeOutTutorialIndependentOfMain' -v -count=1`

Expected: all 6 tests PASS.

- [ ] **Step 2.11: Run full module + race-detector.**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... -count=1`
Expected: PASS.

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/... -count=1`
Expected: PASS.

- [ ] **Step 2.12: Commit.**

```bash
git add modules/world/player.go modules/world/player_script.go modules/world/player_test.go modules/world/player_script_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-76 T2 — Player.OpenTutorial + encodeOut tutorial branch

Replaces T1 stub with real OpenTutorial(com): sets modalTutorial,
OR's modalStateTut into modalState, leaves other modals untouched
(TS-faithful per Player.ts:1999-2003). Adds diff-driven emit branch
to encodeOut: when modalTutorial != lastModalTutorial, writes
OpTutOpen with 2-byte big-endian component payload and updates
lastModalTutorial. Handles open (com=N) and future reset (com=-1)
symmetrically; the latter pinned by TestEncodeOutTutorialResetEmitsMinusOne.

Tests added (6): OpenTutorial state-mutation pin, refresh-flags
untouched pin, encodeOut emit pin, no-change no-emit pin, reset
emit pin, main+tutorial independent emit pin.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: handleTutOpen + dispatch registration

TDD task. Write tests first, run red, implement, run green, commit.

**Files:**
- Test: `pkg/script/handlers_interface_test.go` (append 3 tests)
- Modify: `pkg/script/handlers_interface.go` (add `handleTutOpen`)
- Modify: `pkg/script/handlers.go:281-285` (register dispatch entry)

- [ ] **Step 3.1: Write failing test — `TestTutOpen`.**

Append to `pkg/script/handlers_interface_test.go`:

```go
// TestTutOpen pins TUT_OPEN script-opcode dispatch:
// state.popInt() → ActivePlayer.OpenTutorial(com).
// Mirrors TS PlayerOps.ts:723-725.
func TestTutOpen(t *testing.T) {
	sf := &ScriptFile{
		Name:             "tut_open",
		Opcodes:          []Opcode{OpPushConstantInt, OpTutOpen, OpReturn},
		IntOperands:      []int32{42, 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	mp := &mockPlayer{}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if mp.lastOpenTutorial != 42 {
		t.Errorf("OpenTutorial: got %d, want 42", mp.lastOpenTutorial)
	}
}
```

- [ ] **Step 3.2: Write failing test — `TestHandleTutOpenNullRejected`.**

Append to `pkg/script/handlers_interface_test.go`:

```go
// TestHandleTutOpenNullRejected pins TUT_OPEN: TS wraps com with
// NumberNotNull (PlayerOps.ts:723-724). A com value of -1 must be
// rejected before any side-effect occurs.
func TestHandleTutOpenNullRejected(t *testing.T) {
	mp := &mockPlayer{}
	sf := &ScriptFile{
		Name: "tut_open_null_com",
		Opcodes: []Opcode{
			OpPushConstantInt, // com = -1
			OpTutOpen,
			OpReturn,
		},
		IntOperands: []int32{-1, 0, 0},
	}
	state := Init(sf, mp, false, nil, nil)

	err := Execute(state)
	if err == nil {
		t.Fatalf("Execute: want error for com=-1, got nil")
	}
	want := "TUT_OPEN: input number was null(-1)"
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error: got %q, want substring %q", err.Error(), want)
	}
	if mp.lastOpenTutorial != 0 {
		t.Errorf("OpenTutorial: should not have been called, got %d", mp.lastOpenTutorial)
	}
}
```

- [ ] **Step 3.3: Write failing test — `TestTutOpenNoActivePlayer`.**

Append to `pkg/script/handlers_interface_test.go`:

```go
func TestTutOpenNoActivePlayer(t *testing.T) {
	sf := &ScriptFile{
		Name:             "tut_open_nap",
		Opcodes:          []Opcode{OpPushConstantInt, OpTutOpen, OpReturn},
		IntOperands:      []int32{1, 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	state := Init(sf, nil, false, nil, nil)
	if err := Execute(state); err == nil {
		t.Fatal("expected error from TUT_OPEN with no active player, got nil")
	}
	if state.Execution != Aborted {
		t.Errorf("Execution: got %v, want Aborted", state.Execution)
	}
}
```

- [ ] **Step 3.4: Run tests to verify all FAIL (red phase).**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... -run 'TestTutOpen|TestHandleTutOpenNullRejected|TestTutOpenNoActivePlayer' -v -count=1`

Expected: all 3 tests FAIL. The most likely failure mode is a runtime error from the dispatch table — `Execute` returns `"unknown opcode 2122"` or similar — because `OpTutOpen` is not registered in the handler map yet.

- [ ] **Step 3.5: Add `handleTutOpen` to `handlers_interface.go`.**

In `pkg/script/handlers_interface.go`, immediately after the `handleIfOpenMainSide` function (~line 83), insert:

```go
// handleTutOpen implements TUT_OPEN.
// TS PlayerOps.ts:723-725 — pops a single int (com); check(com,
// NumberNotNull). TS reserves com=-1 for the closeTutorial path
// (Player.ts:716-726 writes TutOpen(-1) directly via Player.write,
// not through this opcode) — closeTutorial is deferred per
// stub_deferred_comment_marker.md.
func handleTutOpen(s *ScriptState) error {
	if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
		return errors.New("TUT_OPEN: no active player")
	}
	com := s.PopInt()
	if err := checkNotNull(com, "TUT_OPEN"); err != nil {
		return err
	}
	s.Self.OpenTutorial(com)
	return nil
}
```

(The `errors` import is already present at the file head, line 3.)

- [ ] **Step 3.6: Register dispatch entry in `handlers.go`.**

In `pkg/script/handlers.go` modal-management block at lines 281-285, append the new entry:

```go
	// S5f: interface / modal.
	// Modal management (5).
	OpIfClose:        handleIfClose,
	OpIfOpenMain:     handleIfOpenMain,
	OpIfOpenChat:     handleIfOpenChat,
	OpIfOpenSide:     handleIfOpenSide,
	OpIfOpenMainSide: handleIfOpenMainSide,
	OpTutOpen:        handleTutOpen,
```

(Update the `// Modal management (5).` comment to `// Modal management (6).` to match the new count.)

Adjacent to this entry, insert a `stub_deferred_comment_marker.md` annotation for the missing TUT_CLOSE port:

```go
	OpTutOpen:        handleTutOpen,
	// OpTutClose: deferred to later sub-spec — TUT_CLOSE handler port
	// + Player.closeTutorial() method. See NAI-76 spec §5 R4.
```

- [ ] **Step 3.7: Run tests to verify all PASS (green phase).**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... -run 'TestTutOpen|TestHandleTutOpenNullRejected|TestTutOpenNoActivePlayer' -v -count=1`

Expected: all 3 tests PASS.

- [ ] **Step 3.8: Run full repo test + race.**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... -count=1`
Expected: PASS.

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./... -count=1`
Expected: PASS.

- [ ] **Step 3.9: Commit.**

```bash
git add pkg/script/handlers_interface.go pkg/script/handlers.go pkg/script/handlers_interface_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-76 T3 — handleTutOpen + dispatch registration

Adds handleTutOpen at pkg/script/handlers_interface.go: pops com,
inline-rejects nil pointer + nil Self, runs checkNotNull (mirrors
TS check(_, NumberNotNull) at PlayerOps.ts:723-724), calls
Self.OpenTutorial. Registers OpTutOpen → handleTutOpen in the
dispatch map. Closes the runtime gap that emitted
"no handler for TUT_OPEN (opcode 2122) at pc=112" on every login
when [proc,tutorialstep_page] iterated post-NAI-75 SPLIT_*.

Tests added (3): popInt+OpenTutorial dispatch, NumberNotNull
rejection, no-active-player rejection. Mirrors IF_OPEN_* test
trio at handlers_interface_test.go:34-50, 546-570, 500-515.

stub_deferred_comment_marker.md annotation added adjacent to the
new dispatch entry for the missing TUT_CLOSE port (NAI-76 spec §5 R4).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: Smoke handoff + close

User-mediated Java-client smoke per `smoke_test_server_handoff.md`. Controller decides close vs route per the spec §4 decision tree.

- [ ] **Step 4.1: Hand off server launch.**

Reply to user:

> NAI-76 T3 committed. Ready for smoke. Please run:
> ```
> CGO_ENABLED=0 go run -trimpath ./cmd/goscape --config.file config.yaml
> ```
> Then drive the Java client through the smoke matrix (see spec §4):
>
> 1. **TUT_OPEN log silence** — log in fresh; grep server stderr for `TUT_OPEN`. Pass = no error line.
> 2. **Door interaction** — walk to RS Guide, advance dialog, click wooden door. Pass = door visually opens, player walks through, "Moving around" tutorial chatbox appears.
> 3. **Click-away modal dismiss** — with a chatnpc dialog open, click ground tiles. Pass = dialog closes on movement.
>
> Report each item pass/fail/partial + relevant log lines.

- [ ] **Step 4.2: Apply smoke decision tree.**

Per spec §4:

| Outcome | Action |
|---|---|
| 1+2+3 all pass | Proceed to step 4.3 (close commit). |
| 1+2 pass, 3 fail | If click-away fix is ≤30 LOC, in-scope-stretch (write tests first, fix, commit as T4-stretch); else route to NAI-77. |
| 1 passes, 2 partial | Investigate post-tut_open wiring (likely encodeOut diff-check or OpenTutorial wiring). Fix in-scope. |
| 1 passes, 2 fail | Route door to NAI-77 with characterization (TUT_OPEN no longer noise). Proceed to step 4.3. |
| 1 fails | Port has a defect — re-investigate before close. Likely root: dispatch entry missing, popInt arity wrong, or encodeOut not emitting. |

- [ ] **Step 4.3: Memory updates.**

Save any new lessons surfaced during T1-T3 to memory (per `post_task_handoff.md`). Likely candidates:

- If the implementer caught a plan defect (e.g. flaky 6-byte read in step 2.6) → memory entry + cross-reference.
- If the smoke decision tree path was taken (residuals routed) → entry in `nai_followups.md` for NAI-77.

Then update `~/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/MEMORY.md` index entries.

- [ ] **Step 4.4: Close commit.**

Mirror NAI-75's close-commit shape (commit `6081e17`). Required body sections:

- **Scope** — what landed.
- **Cadence** — investigation+fix variant; T1 compressed; T2-T3 full TDD; smoke handoff.
- **Spec / Plan** — file paths + commit shas (spec `31931c9`; plan = the commit landing this plan doc, see step ?).
- **Commits (chronological)** — T1, T2, T3 SHAs.
- **Follow-ups closed** — any if applicable (cascade-resolved residuals).
- **Deviations opened** — none expected per spec §6.
- **Net deviation tally** — 14 → 14 (or adjusted if smoke route opened deviations).
- **Wire-behaviour delta at HEAD** — TUT_OPEN now handled; OpenTutorial method functional; encodeOut emits OpTutOpen on diff.
- **Lessons confirmed / surfaced** — per `runescript_cadence.md`, `compressed_cadence.md`, `controller_preflight.md`, `verify_implementer_claims.md`, `smoke_test_server_handoff.md`, `defensive_gate_doc_comment_label.md`, `stub_deferred_comment_marker.md`, `execution_mode_default.md`, `plan_grep_helper_patterns.md`.
- **Carry-forwards** — TUT_CLOSE deferred (per spec §6); FONT-WRAP-NAIVE + MESANIM-NOT-PORTED carry from NAI-75; other long-running deferrals.
- **Smoke result** — N of 3 items pass.
- **Closes memory:** trailer per `close_commit_memory_trailer.md`.

```bash
git commit --allow-empty --no-gpg-sign -m "$(cat <<'EOF'
chore(close): NAI-76 — TUT_OPEN handler port closes login-error cascade
            (opens 0; tally 14 → 14)

[full body per the structure above]

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Self-review

Per `writing-plans` skill checklist:

**1. Spec coverage:**

| Spec section | Plan coverage |
|---|---|
| §A wire protocol (`OpTutOpen`) | T1 step 1.1 |
| §B `modalStateTut` bit | T1 step 1.2 |
| §C `lastModalTutorial` field + init | T1 steps 1.3, 1.4 |
| §D `encodeOut` tutorial-emit branch | T2 step 2.9 |
| §E `Player.OpenTutorial` method | T2 step 2.8 (replacing T1 stub from step 1.5/1.8 fixup) |
| §F `ActivePlayer` interface extension | T1 step 1.5 |
| §G `handleTutOpen` + dispatch registration | T3 steps 3.5, 3.6 |
| §6 Out of scope (TUT_CLOSE deferred annotation) | T3 step 3.6 (annotation block) |
| §3 `TestTutOpen` (pop+dispatch) | T3 step 3.1 |
| §3 `TestHandleTutOpenNullRejected` | T3 step 3.2 |
| §3 `TestTutOpenNoActivePlayer` | T3 step 3.3 |
| §3 `TestOpenTutorial_SetsFieldsWithoutClosingOthers` | T2 step 2.1 |
| §3 `TestOpenTutorial_RefreshFlagsUntouched` | T2 step 2.2 |
| §3 `TestEncodeOutSendsTutOpen` | T2 step 2.3 |
| §3 `TestEncodeOutTutorialNoChangeNoEmit` | T2 step 2.4 |
| §3 `TestEncodeOutTutorialResetEmitsMinusOne` | T2 step 2.5 |
| §3 `TestEncodeOutTutorialIndependentOfMain` | T2 step 2.6 |
| §3 mock recorder field + impl | T1 step 1.6 |
| §4 smoke matrix + decision tree | T4 |
| §5 risk register | Mitigations baked into T1-T3 (R1 non-touch; R2 init -1; R3 checkNotNull; R4 stub annotation; R5 smoke route; R6 smoke route) |

No gaps.

**2. Placeholder scan:** No TBD / TODO / "implement later". One `[full body per the structure above]` placeholder in step 4.4 — intentional, expanded inline by the controller at close time per the bulleted structure listed.

**3. Type consistency:** `OpenTutorial(com int)` consistent across §F interface, §E method, §G handler call site, T1 stub, T2 implementation, T3 mock impl, all tests. `lastModalTutorial int` consistent across §C declaration, T1 step 1.3, encodeOut diff-check, T2 tests. `OpTutOpen` consistent across prot.go declaration, dispatch registration, all tests.

**4. Test fixture runnability** (per `plan_runnable_test_fixtures.md`): mentally executed each fixture:

- T2.1: pre-state set; OpenTutorial(42) called; mutations asserted. ✓
- T2.3: OpenTutorial(42); encodeOut emits 3 bytes (1 opcode + 2 payload); read 3 bytes; assert. Aligns with `TestEncodeOutSendsIfOpenMain` template. ✓
- T2.4: drain-then-no-emit pattern matches the existing `TestEncodeOutNoopWhenModalUnchanged`. ✓
- T2.5: lastModalTutorial established at 42; reset to -1; emit `[0xFF, 0xFF]` payload (Go's signed-int `-1` shifts arithmetically, `byte(-1) == 0xFF`). ✓
- T2.6: OpenMain(1234) + OpenTutorial(42); 6-byte read; assert ordering matches encodeOut emit sequence (IF_OPENMAIN first, then TUT_OPEN). ✓
- T3.1: `[OpPushConstantInt, OpTutOpen, OpReturn]` with `[42, 0, 0]` mirrors `TestIfOpenMain` exactly. InstructionCount=3 matches array length. ✓
- T3.2: -1 push, expect error `"TUT_OPEN: input number was null(-1)"` matches `checkNotNull` error literal at `handlers_player.go:73`. ✓
- T3.3: nil Self → no PtrActivePlayer set → handler errors. Matches `TestIfOpenMainNoActivePlayer`. ✓

**5. Variable name collision check** (per `plan_var_name_collision.md`): each function body inspected. No `:=` redeclarations of parameters or sibling-scoped vars. `payload := []byte{...}` in encodeOut tutorial branch is in its own `if` block scope, distinct from the switch-case `payload`s above. ✓

**6. Mock recorder field naming** (per `mock_recorder_field_naming_check.md`): `lastOpenTutorial int` matches the existing `lastOpenMain/Chat/Side` pattern at `runner_test.go:156-158`. ✓

**7. Helper-pattern grep** (per `plan_grep_helper_patterns.md`): `handlers_interface.go` uses inline `s.Pointers&PtrActivePlayer == 0 || s.Self == nil` (not the helper from `handlers_player.go`); plan T3.5 mirrors the local convention. `checkNotNull` from `handlers_player.go:71-76` is reused. ✓

**8. Sibling-site guard audit** (per `plan_sibling_site_guard_audit.md`): all IF_OPEN_* handlers in `handlers_interface.go` use the same inline guard + checkNotNull pattern; T3.5 mirrors. ✓

Plan complete.

---

## Execution mode

Per `execution_mode_default.md`: dispatch via `superpowers:subagent-driven-development`.
