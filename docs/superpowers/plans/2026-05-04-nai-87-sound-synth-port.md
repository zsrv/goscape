# NAI-87 SOUND_SYNTH Port Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port the `SOUND_SYNTH` script opcode (2104) end-to-end so `[proc,open_and_close_door]` runs past pc=68. Cascade-blocker from NAI-86.

**Architecture:** Single-bundle template-mirror port of NAI-16 MIDI_SONG/MIDI_JINGLE retire. Wire opcode + encoder + Player wire-out + ActivePlayer interface method + script handler + dispatch registration + mockPlayer capture, all TDD-ordered so each layer compiles and tests green before the next.

**Tech Stack:** Go 1.26+; `pkg/io/packet` (RS2 binary buffer); test runner `go test`. Spec: `docs/superpowers/specs/2026-05-04-nai-87-sound-synth-port-design.md`.

---

## Pre-flight (controller)

Before dispatching the implementer, the controller MUST verify (per `controller_preflight.md`):

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... -count=1
```

Expected: clean (HEAD `d979ddd` post-spec). Re-grep these claims at HEAD:

| Claim | Verify |
|---|---|
| `OpSoundSynth Opcode = 2104` declared | `grep -n "OpSoundSynth" pkg/script/opcode.go` (line 204) |
| `SOUND_SYNTH` in opcode-name switch | `grep -n "SOUND_SYNTH" pkg/script/opcode.go` (line 815-816) |
| No existing `OpSynthSound` wire-opcode constant | `grep -n "OpSynthSound\|SynthSound" pkg/io/protocol/game/server/prot.go` returns empty |
| No existing `encodeSynthSound` / `PlaySynth` | `grep -rn "encodeSynthSound\|PlaySynth" --include="*.go"` returns empty |
| `ActivePlayer.LowMemory()` and `PlaySong/PlayJingle` exist | `grep -n "LowMemory()\|PlaySong\|PlayJingle" pkg/script/active.go` returns 467, 475, 483 |
| `mockPlayer.lowMemoryValue` + `LowMemory()` method exist | `grep -n "lowMemoryValue\|func (m \*mockPlayer) LowMemory" pkg/script/runner_test.go` returns 286, 566 |
| `requireActivePlayer` helper exists at `pkg/script/handlers_player.go:35` | `grep -n "func requireActivePlayer" pkg/script/handlers_player.go` |
| MIDI dispatch block in handlers.go | `grep -n "OpMidiSong\|OpMidiJingle" pkg/script/handlers.go` returns 423-424 |

If any check fails, STOP — investigate before dispatching.

---

## Task 1: Wire opcode constant `OpSynthSound`

**Files:**
- Modify: `pkg/io/protocol/game/server/prot.go` (insert after line 100, before line 101's `// Input-tracking signals` block)

This task introduces no test of its own — `OpSynthSound` is a single `Op` constant used by Task 3. Verification is `go build ./...`. The wire-format correctness lands in Task 2's bytes-exact test (which pins payload size) and Task 3's `TestPlaySynthWritesOut` (which pins the writeOut opcode wiring).

- [ ] **Step 1: Add `OpSynthSound` constant**

Add the following block immediately after the existing `OpMidiJingle` line (currently line 100), before the `// Input-tracking signals` comment block:

```go
	// Sound-effect packet (verified against TS ServerGameProt.ts:80).
	// SYNTH_SOUND plays a short synthesized sound effect; payload is
	// fixed 5 bytes: p2(synth) p1(loops) p2(delay) per
	// SynthSoundEncoder.ts:9-13. Wired from the SOUND_SYNTH (2104)
	// script opcode via (*Player).PlaySynth.
	OpSynthSound = Op{Opcode: 12, PayloadSize: 5}
```

- [ ] **Step 2: Verify build**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`
Expected: clean (no compile errors).

- [ ] **Step 3: Commit**

```bash
git add pkg/io/protocol/game/server/prot.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(protocol): NAI-87 T1 — OpSynthSound wire opcode (12, fixed 5 bytes)

TS ServerGameProt.ts:80 declares SYNTH_SOUND = (12, 5). Fixed
5-byte payload (p2/p1/p2 per SynthSoundEncoder.ts:9-13). No
handler wiring yet — wired in NAI-87 T3 via (*Player).PlaySynth.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: `encodeSynthSound` (new file `modules/world/sound_encoders.go`)

**Files:**
- Create: `modules/world/sound_encoders.go`
- Create: `modules/world/sound_encoders_test.go`

TDD: tests first, then encoder. Test shape mirrors `modules/world/midi_encoders_test.go` verbatim.

- [ ] **Step 1: Write the failing tests**

Create `modules/world/sound_encoders_test.go`:

```go
package world

import (
	"bytes"
	"testing"

	"github.com/zsrv/goscape/pkg/io/packet"
)

// TestEncodeSynthSoundFieldsDecodeInClientOrder pins client-order
// field decode of an encodeSynthSound payload. Mirrors TS
// SynthSoundEncoder.ts:9-13 (p2 synth, p1 loops, p2 delay).
func TestEncodeSynthSoundFieldsDecodeInClientOrder(t *testing.T) {
	buf := packet.NewPacket(make([]byte, 0, 8))
	encodeSynthSound(buf, 0x1234, 0x56, 0x789A)

	r := packet.NewPacket(buf.Bytes())
	r.Pos = 0
	if got := r.G2(); got != 0x1234 {
		t.Errorf("G2 (synth) = 0x%04x, want 0x1234", got)
	}
	if got := r.G1(); got != 0x56 {
		t.Errorf("G1 (loops) = 0x%02x, want 0x56", got)
	}
	if got := r.G2(); got != 0x789A {
		t.Errorf("G2 (delay) = 0x%04x, want 0x789A", got)
	}
	if r.Pos != len(buf.Bytes()) {
		t.Errorf("not all bytes consumed: pos=%d, len=%d", r.Pos, len(buf.Bytes()))
	}
}

// TestEncodeSynthSoundBytesExact pins the exact 5-byte big-endian
// payload (synth=0x0102, loops=0x03, delay=0x0405).
func TestEncodeSynthSoundBytesExact(t *testing.T) {
	buf := packet.NewPacket(make([]byte, 0, 8))
	encodeSynthSound(buf, 0x0102, 0x03, 0x0405)
	want := []byte{0x01, 0x02, 0x03, 0x04, 0x05}
	if !bytes.Equal(buf.Bytes(), want) {
		t.Errorf("bytes mismatch:\n got: % 02x\nwant: % 02x", buf.Bytes(), want)
	}
}

// TestEncodeSynthSoundZeroValuesValid pins the all-zeros payload.
func TestEncodeSynthSoundZeroValuesValid(t *testing.T) {
	buf := packet.NewPacket(make([]byte, 0, 8))
	encodeSynthSound(buf, 0, 0, 0)
	want := []byte{0, 0, 0, 0, 0}
	if !bytes.Equal(buf.Bytes(), want) {
		t.Errorf("bytes mismatch:\n got: % 02x\nwant: % 02x", buf.Bytes(), want)
	}
}

// TestEncodeSynthSoundMaxValuesValid pins boundary values
// (uint16 max for synth/delay, uint8 max for loops).
func TestEncodeSynthSoundMaxValuesValid(t *testing.T) {
	buf := packet.NewPacket(make([]byte, 0, 8))
	encodeSynthSound(buf, 0xFFFF, 0xFF, 0xFFFF)
	want := []byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF}
	if !bytes.Equal(buf.Bytes(), want) {
		t.Errorf("bytes mismatch:\n got: % 02x\nwant: % 02x", buf.Bytes(), want)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestEncodeSynthSound -count=1 -v`
Expected: FAIL — "undefined: encodeSynthSound" (compilation error).

- [ ] **Step 3: Write minimal implementation**

Create `modules/world/sound_encoders.go`:

```go
package world

import "github.com/zsrv/goscape/pkg/io/packet"

// encodeSynthSound writes a SynthSound payload per TS SynthSoundEncoder.ts:
//
//	buf.p2(message.synth);
//	buf.p1(message.loops);
//	buf.p2(message.delay);
//
// Fixed 5-byte payload. Caller wraps in:
//
//	p.writeOut(gameserver.OpSynthSound, buf.Bytes())
//
// Out-of-range script values silently truncate at the cast boundary
// (TS encoder behavior: JS implicit narrowing in p1/p2). Caller is
// responsible for the cast (see (*Player).PlaySynth).
func encodeSynthSound(buf *packet.Packet, synth uint16, loops uint8, delay uint16) {
	buf.P2(synth)
	buf.P1(loops)
	buf.P2(delay)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestEncodeSynthSound -count=1 -v`
Expected: PASS (4 tests).

- [ ] **Step 5: Commit**

```bash
git add modules/world/sound_encoders.go modules/world/sound_encoders_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-87 T2 — encodeSynthSound + bytes-exact tests

Mirrors TS SynthSoundEncoder.ts:9-13 (p2/p1/p2 fixed 5-byte
payload). Tests pin client-order decode + bytes-exact + zero/max
boundaries per rsbuf_roundtrip_tests.md. New sound_encoders.go
file (vs extending midi_encoders.go) per spec §"Out of scope".

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: `(*Player).PlaySynth` wire-out

**Files:**
- Modify: `modules/world/player_script.go` (append at end of file)
- Modify: `modules/world/player_script_test.go` (append at end of file)

`PlaySynth` has no name normalization, no PRELOADED lookup, no zero-arg early exit — pure encode + writeOut. Single positive-pin test.

- [ ] **Step 1: Write the failing test**

Append to `modules/world/player_script_test.go`:

```go
// TestPlaySynthWritesOut pins NAI-87 T3: (*Player).PlaySynth issues
// a writeOut to the client for the OpSynthSound opcode. Failure
// signal = "wire-out broken or encoder mis-wired."
func TestPlaySynthWritesOut(t *testing.T) {
	p, _ := newTestPlayer(t)
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	p.client.encryptor = enc
	p.PlaySynth(123, 1, 0)
	if n := p.client.bufw.Buffered(); n == 0 {
		t.Errorf("PlaySynth wrote 0 bytes to c.bufw; want >0 (NAI-87 positive pin)")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestPlaySynthWritesOut -count=1 -v`
Expected: FAIL — "p.PlaySynth undefined" (compilation error).

- [ ] **Step 3: Write minimal implementation**

Append to `modules/world/player_script.go` (after the existing `PlayJingle` function ending at line 973):

```go
// PlaySynth sends a synthesized sound effect to the client. Called by
// the SOUND_SYNTH script opcode (PlayerOps.ts:466-474). Encodes
// synth/loops/delay via encodeSynthSound and writes OpSynthSound.
//
// No name normalization, no PRELOADED lookup, no validation — TS
// handler has none. Out-of-range int values truncate at the
// uint16/uint8/uint16 cast boundary (matches TS p1/p2 narrowing).
func (p *Player) PlaySynth(synth, loops, delay int) {
	buf := packet.NewPacket(make([]byte, 0, 5))
	encodeSynthSound(buf, uint16(synth), uint8(loops), uint16(delay))
	p.writeOut(gameserver.OpSynthSound, buf.Bytes())
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestPlaySynthWritesOut -count=1 -v`
Expected: PASS.

- [ ] **Step 5: Verify the full world-package test suite is still green**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -count=1`
Expected: ok (whole package, no regressions from earlier MIDI tests).

- [ ] **Step 6: Commit**

```bash
git add modules/world/player_script.go modules/world/player_script_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-87 T3 — (*Player).PlaySynth wire-out

Encodes synth/loops/delay via encodeSynthSound and writes
OpSynthSound to the client. Mirrors TS PlayerOps.ts:466-474
write path (no name normalization, no PRELOADED lookup, no
validation — TS handler has none). Positive-pin test mirrors
TestPlaySongWritesOut shape.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: `ActivePlayer.PlaySynth` interface + `mockPlayer` capture

**Files:**
- Modify: `pkg/script/active.go` (add method to `ActivePlayer` interface, after `PlayJingle` declaration ending at line 483)
- Modify: `pkg/script/runner_test.go` (add `playSynthCalls` field to `mockPlayer` struct + `PlaySynth` method)

No new tests in this task — the interface and mock additions are consumed by Task 5's handler tests. Verification is `go build ./...` + existing tests still green.

- [ ] **Step 1: Add `PlaySynth` to the `ActivePlayer` interface**

In `pkg/script/active.go`, immediately after the `PlayJingle(delay int, name string)` line (currently line 483) and before the `// NAI-47: SETIDKIT appearance mutation.` comment:

```go
	// PlaySynth sends a synthesized sound effect to the client. Called
	// by the SOUND_SYNTH script opcode (PlayerOps.ts:466-474). No name
	// normalization, no PRELOADED lookup, no validation — TS handler
	// gates only on lowMemory; the script-handler layer applies that
	// gate. Implementation encodes p2(synth) p1(loops) p2(delay) and
	// writes OpSynthSound.
	PlaySynth(synth, loops, delay int)
```

- [ ] **Step 2: Add `playSynthCalls` field to `mockPlayer`**

In `pkg/script/runner_test.go`, locate the existing block in the `mockPlayer` struct (around line 295-303):

```go
	// S7h: captured MIDI_JINGLE plays. Each entry records the delay and
	// the normalized-name argument as seen by the mock.
	playJingleCalls []struct {
		delay int
		name  string
	}
```

Immediately after that block (before the next `// NAI-74: ...` block on line 305), add:

```go
	// NAI-87: captured SOUND_SYNTH plays. Each entry records the three
	// int arguments passed to PlaySynth in TS argument order
	// (synth, loops, delay).
	playSynthCalls []struct {
		synth, loops, delay int
	}
```

- [ ] **Step 3: Add `PlaySynth` method to `mockPlayer`**

Locate the existing `PlayJingle` method on `mockPlayer` (around line 578-580). Immediately after `PlayJingle`'s closing brace, add:

```go
// NAI-87: PlaySynth captures the SOUND_SYNTH (synth, loops, delay)
// triple for handler tests. The mock does not encode anything;
// wire-format coverage lives in modules/world/sound_encoders_test.go.
func (m *mockPlayer) PlaySynth(synth, loops, delay int) {
	m.playSynthCalls = append(m.playSynthCalls, struct {
		synth, loops, delay int
	}{synth, loops, delay})
}
```

- [ ] **Step 4: Verify build + existing tests pass**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ ./modules/world/ -count=1
```

Expected: build clean, both packages green. (Modules/world's `(*Player)` already has a real `PlaySynth` from Task 3, so it satisfies the new interface method.)

- [ ] **Step 5: Commit**

```bash
git add pkg/script/active.go pkg/script/runner_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-87 T4 — ActivePlayer.PlaySynth + mockPlayer capture

Extends the script-engine's ActivePlayer interface with PlaySynth
(consumed by handleSoundSynth in T5). mockPlayer gains a
playSynthCalls slice + PlaySynth method that records the
(synth, loops, delay) triple for handler tests. (*Player) already
satisfies the new method via NAI-87 T3.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: `handleSoundSynth` + dispatch wiring

**Files:**
- Modify: `pkg/script/handlers_player.go` (append after `handleMidiJingle`, currently ending at line 909)
- Modify: `pkg/script/handlers.go` (add to S7h audio block at lines 422-424)
- Modify: `pkg/script/handlers_player_test.go` (append at end)

TDD: tests first, then handler, then dispatch wiring. The dispatch wiring step is intentionally separated so the TDD cycle stays clean — tests call `handleSoundSynth` directly.

- [ ] **Step 1: Write the failing tests**

Append to `pkg/script/handlers_player_test.go`:

```go
// TestSoundSynthHappyPath pins NAI-87: SOUND_SYNTH dispatches to
// (*ActivePlayer).PlaySynth with the popped (synth, loops, delay)
// triple in TS argument order. Push order left-to-right matches
// TS popInts(3) at ScriptState.ts:325-331 (top-of-stack popped
// first, written into result[amount-1]).
func TestSoundSynthHappyPath(t *testing.T) {
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Self:        &mockPlayer{},
		Pointers:    PtrActivePlayer,
	}
	s.PushInt(42)  // synth
	s.PushInt(2)   // loops
	s.PushInt(100) // delay
	mp := s.Self.(*mockPlayer)

	if err := handleSoundSynth(s); err != nil {
		t.Fatalf("handleSoundSynth: %v", err)
	}
	if len(mp.playSynthCalls) != 1 {
		t.Fatalf("playSynthCalls: got %d, want 1", len(mp.playSynthCalls))
	}
	got := mp.playSynthCalls[0]
	if got.synth != 42 || got.loops != 2 || got.delay != 100 {
		t.Errorf("playSynthCalls[0] = %+v, want {synth:42, loops:2, delay:100}", got)
	}
}

// TestSoundSynthLowMemoryBails pins TS PlayerOps.ts:470-472 silent
// no-op gate. lowMemory=true → handler returns nil and PlaySynth is
// NOT called.
func TestSoundSynthLowMemoryBails(t *testing.T) {
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Self:        &mockPlayer{lowMemoryValue: true},
		Pointers:    PtrActivePlayer,
	}
	s.PushInt(42)
	s.PushInt(2)
	s.PushInt(100)
	mp := s.Self.(*mockPlayer)

	if err := handleSoundSynth(s); err != nil {
		t.Fatalf("handleSoundSynth: %v", err)
	}
	if len(mp.playSynthCalls) != 0 {
		t.Errorf("lowMemory=true: playSynthCalls=%d, want 0", len(mp.playSynthCalls))
	}
}

// TestSoundSynthNoActivePlayerRejects pins the requireActivePlayer
// gate. Self=nil + Pointers=0 → error containing "SOUND_SYNTH: no
// active player".
func TestSoundSynthNoActivePlayerRejects(t *testing.T) {
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Self:        nil,
		Pointers:    0,
	}
	s.PushInt(42)
	s.PushInt(2)
	s.PushInt(100)

	err := handleSoundSynth(s)
	if err == nil {
		t.Fatal("no active player: want error, got nil")
	}
	if !strings.Contains(err.Error(), "SOUND_SYNTH: no active player") {
		t.Errorf("error %q does not contain %q", err.Error(), "SOUND_SYNTH: no active player")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestSoundSynth -count=1 -v`
Expected: FAIL — "undefined: handleSoundSynth" (compilation error).

- [ ] **Step 3: Write the handler**

Append to `pkg/script/handlers_player.go`, immediately after `handleMidiJingle`'s closing brace (currently line 909):

```go
// handleSoundSynth (SOUND_SYNTH, opcode 2104) plays a synthesized
// sound effect to the active player. Silent no-op if the player has
// lowMemory set. Mirrors TS PlayerOps.ts:466-474.
//
// Pointer gate: require active_player (TS ScriptOpcodePointers.ts:434
// require: ['active_player']).
//
// Pop order (top-of-stack first per ScriptState.ts:325-331):
// delay, loops, synth. TS uses popInts(3) which fills the result
// slice from index amount-1 down to 0, so the destructured
// `[synth, loops, delay]` gets `synth = bottom-most pop`,
// `delay = first pop`. No check() validation — TS has none.
func handleSoundSynth(s *ScriptState) error {
	delay := s.PopInt()
	loops := s.PopInt()
	synth := s.PopInt()
	if err := requireActivePlayer(s, "SOUND_SYNTH"); err != nil {
		return err
	}
	if s.Self.LowMemory() {
		return nil
	}
	s.Self.PlaySynth(synth, loops, delay)
	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestSoundSynth -count=1 -v`
Expected: PASS (3 tests).

- [ ] **Step 5: Wire dispatch in `handlers.go`**

In `pkg/script/handlers.go`, locate the existing audio block at lines 422-424:

```go
	// S7h: audio — MIDI_SONG + MIDI_JINGLE.
	OpMidiJingle: handleMidiJingle,
	OpMidiSong:   handleMidiSong,
```

Replace that block with:

```go
	// S7h + NAI-87: audio — MIDI_SONG + MIDI_JINGLE + SOUND_SYNTH.
	OpMidiJingle: handleMidiJingle,
	OpMidiSong:   handleMidiSong,
	OpSoundSynth: handleSoundSynth,
```

- [ ] **Step 6: Verify the full pkg/script suite is green**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -count=1`
Expected: ok.

- [ ] **Step 7: Verify the full repo build + test suite is green**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... -count=1`
Expected: ok (no regressions across any package).

- [ ] **Step 8: Commit**

```bash
git add pkg/script/handlers_player.go pkg/script/handlers.go pkg/script/handlers_player_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
close: NAI-87 — SOUND_SYNTH (opcode 2104) handler + dispatch wiring

Implements handleSoundSynth + register in S7h/NAI-87 audio block.
Pop order delay/loops/synth (top-of-stack first per
ScriptState.ts:325-331). TS-mirror: requireActivePlayer gate +
lowMemory silent-bail; no check() validation. Tests pin all three
gates + happy-path argument routing.

Cascade-blocker resolved: [proc,open_and_close_door] should now
run past pc=68. NAI-86 carry-forward items 3+4 (door walkability
post-LOC_CHANGE→inviswall + auto-revert via duration=3 lifecycle
path) re-smoke at user-driven door-click; divergence routes to
NAI-88.

Closes memory: nai_followups.md NAI-86 carry-forward 1 (NAI-87
candidate SOUND_SYNTH).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Final controller verification

After Task 5 commits, the controller MUST:

- [ ] **A. Re-grep at HEAD to confirm wiring**

```bash
grep -n "OpSoundSynth" pkg/script/handlers.go pkg/script/opcode.go
grep -n "handleSoundSynth" pkg/script/handlers.go pkg/script/handlers_player.go
grep -n "OpSynthSound" pkg/io/protocol/game/server/prot.go modules/world/player_script.go
grep -n "PlaySynth" pkg/script/active.go pkg/script/runner_test.go modules/world/player_script.go
```

Each grep MUST return at least one hit; the `OpSoundSynth: handleSoundSynth` dispatch line is the critical one (most recent missed-wiring cause).

- [ ] **B. Cross-package test re-run with race detector**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./... -count=1
```

Expected: ok across all packages.

- [ ] **C. Hand off door-click smoke to user**

Per `smoke_test_server_handoff.md`: ask the user to (1) start the server (`CGO_ENABLED=0 go run -trimpath ./cmd/goscape --config.file config.yaml`), (2) login Tutorial Island, (3) click the closed door, (4) report observed behavior on items 3 (walkability) and 4 (auto-revert).

Routing of the smoke result follows `cascade_theory_smoke_binding.md`:
- Both verify → already closed in T5 commit's `Closes memory:` trailer.
- Either diverges → file new tracker entry; brainstorm NAI-88.

---

## Self-review

**Spec coverage:**
- ✅ Wire opcode constant — Task 1.
- ✅ Encoder + tests — Task 2 (4 tests covering field decode, bytes-exact, zero, max).
- ✅ Player.PlaySynth + test — Task 3.
- ✅ ActivePlayer interface extension — Task 4.
- ✅ Handler + dispatch + tests — Task 5 (3 tests covering happy-path, lowMemory, no-active-player).
- ✅ mockPlayer capture — Task 4.
- ✅ Re-smoke handoff — Final controller verification step C.

**Placeholder scan:** No "TBD", "TODO", or unresolved references. All code blocks complete. All test names spelled `SoundSynth` (not `SoundSynch`).

**Type consistency:** `PlaySynth(synth, loops, delay int)` signature is consistent across `ActivePlayer` interface (Task 4 Step 1), `(*Player)` impl (Task 3 Step 3), `mockPlayer` impl (Task 4 Step 3), and handler call site (Task 5 Step 3). `encodeSynthSound(buf, synth uint16, loops uint8, delay uint16)` signature is consistent between Task 2 Step 3 (impl) and Task 3 Step 3 (caller, with explicit casts).

**Ordering rationale:** Bottom-up dependency order — opcode constant → encoder → Player wire → interface → handler+dispatch. Each task compiles and tests green before the next; controller can verify each layer independently before approving the next dispatch.
