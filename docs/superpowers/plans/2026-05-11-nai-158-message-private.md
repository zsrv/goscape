# NAI-158: MessagePrivate handler (opcode 148) — implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port TS `MessagePrivateHandler` (client opcode 148, RuneScape /tell-style private chat) to goscape, including its `WordPack` codec dependency and a `FriendsBridge.PrivateMessage` bridge method.

**Architecture:** Three task split:
1. New package `pkg/wordenc/wordpack` ports the TS `WordPack` codec (95 LOC: unpack/pack/sentence-case) as a standalone library with no `modules/world` imports.
2. `FriendsBridge` interface gains a `PrivateMessage` method; `Server` gains a `pmCount uint32` counter and a `nextPmId() uint32` helper that mirrors the TS `World.pmCount` computation.
3. `modules/world/handler_message_private.go` mirrors `MessagePrivateHandler.ts:10-35` verbatim, dispatched from the existing `gameHandlers[256]` table in `handlers_game.go`.

No active deviations — every TS branch maps to existing goscape infrastructure (`LoginBridgeMod.NotifyPlayerBan`, `Player.mutedUntil`, `Server.cfg.NodeID`).

**Tech Stack:** Go 1.26+. All `go` commands prefixed with `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache`. Commits use `git commit --no-gpg-sign`.

**Reference:**
- Spec: `docs/superpowers/specs/2026-05-11-nai-158-message-private-design.md`
- TS handler: `LostCityRS/Engine-TS/src/network/game/client/handler/MessagePrivateHandler.ts`
- TS codec: `LostCityRS/Engine-TS/src/wordenc/WordPack.ts`
- TS routing: `LostCityRS/Engine-TS/src/engine/World.ts:1631-1643`

---

## File Structure

**Task 1 — WordPack codec:**
- Create: `pkg/wordenc/wordpack/wordpack.go` (~80 LOC) — `Pack`, `Unpack`, internal `charLookup`, internal `toSentenceCase`
- Create: `pkg/wordenc/wordpack/wordpack_test.go` (~150 LOC) — 7 test cases

**Task 2 — Bridge + Server counter:**
- Modify: `modules/world/bridges.go` — extend `FriendsBridge` interface (1 line); extend `noopBridges` (1 line)
- Modify: `modules/world/bridges_test.go` — add `recordedPrivateMessageCall` struct + `privateMsgs` field + `PrivateMessage` method on `recordingBridges` + capture test
- Modify: `modules/world/server.go` — add `pmCount uint32` field on `Server` struct
- Create: `modules/world/server_pmid.go` (~25 LOC) — `nextPmId` helper
- Create: `modules/world/server_pmid_test.go` (~80 LOC) — 3 tests (monotonicity, NodeID byte, rand-byte range)

**Task 3 — Handler + dispatcher:**
- Create: `modules/world/handler_message_private.go` (~45 LOC)
- Create: `modules/world/handler_message_private_test.go` (~200 LOC) — 5 cases
- Modify: `modules/world/handlers_game.go` — one entry in `init()` table

---

## Task 1: WordPack codec

**Files:**
- Create: `pkg/wordenc/wordpack/wordpack.go`
- Create: `pkg/wordenc/wordpack/wordpack_test.go`

Standalone unit: no imports from `modules/world`. Round-trip tests pin the table.

- [ ] **Step 1: Create the new package directory**

```bash
mkdir -p pkg/wordenc/wordpack
```

- [ ] **Step 2: Write the failing tests**

Create `pkg/wordenc/wordpack/wordpack_test.go`:

```go
package wordpack

import (
	"testing"

	"github.com/zsrv/goscape/pkg/io/packet"
)

// TestPackUnpackRoundTrip exercises the happy path: Pack a
// sentence-cased string, then Unpack the bytes — Unpack applies
// sentence-case so a sentence-cased input round-trips cleanly.
func TestPackUnpackRoundTrip(t *testing.T) {
	cases := []struct {
		name string
		in   string // already sentence-cased so round-trip equals identity
	}{
		{"simple ASCII", "Hello world"},
		{"mixed punctuation", "Hi! How are you?"},
		{"digits and letters", "Pick 3 swords"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pk := packet.NewPacket(nil)
			Pack(pk, c.in)
			pk2 := packet.NewPacket(pk.Data)
			got := Unpack(pk2, len(pk.Data))
			if got != c.in {
				t.Errorf("round-trip: got %q, want %q (packed: %x)", got, c.in, pk.Data)
			}
		})
	}
}

// TestUnpackAppliesSentenceCase pins the TS WordPack.toSentenceCase
// rules (WordPack.ts:80-94): capitalize after start, '.', or '!'.
func TestUnpackAppliesSentenceCase(t *testing.T) {
	pk := packet.NewPacket(nil)
	Pack(pk, "hello. world! foo")
	pk2 := packet.NewPacket(pk.Data)
	got := Unpack(pk2, len(pk.Data))
	want := "Hello. World! Foo"
	if got != want {
		t.Errorf("sentence-case: got %q, want %q", got, want)
	}
}

// TestUnpackLengthCap pins TS WordPack.ts:19 (`pos < 80`): Unpack
// stops emitting characters once the decoded output reaches 80 chars,
// even if more input bytes remain.
func TestUnpackLengthCap(t *testing.T) {
	// 50 'a' chars → 50 nibbles of value 3 (index of 'a' in charLookup).
	// Pack densely: 2 nibbles per byte → 25 bytes for 50 chars.
	src := ""
	for i := 0; i < 90; i++ {
		src += "a"
	}
	pk := packet.NewPacket(nil)
	Pack(pk, src) // Pack truncates input to 80 first (TS line 44-46).
	pk2 := packet.NewPacket(pk.Data)
	got := Unpack(pk2, len(pk.Data))
	if len(got) != 80 {
		t.Errorf("Unpack length cap: got %d chars, want 80", len(got))
	}
}

// TestPackLengthCap pins TS WordPack.ts:44-46: Pack truncates input
// to the first 80 characters.
func TestPackLengthCap(t *testing.T) {
	src := ""
	for i := 0; i < 90; i++ {
		src += "a"
	}
	pk := packet.NewPacket(nil)
	Pack(pk, src)
	// 80 'a' chars pack at 4 bits per char (index 3 < 13) → 40 bytes.
	if len(pk.Data) != 40 {
		t.Errorf("Pack truncate: got %d bytes, want 40 (80 chars * 4 bits)", len(pk.Data))
	}
}

// TestPackUnpackPoundSign pins multi-byte UTF-8 handling. The TS
// charLookup table includes '£' which is a single code unit in
// UTF-16 but 2 bytes in UTF-8; the Go port uses []string instead
// of []byte specifically to preserve this character.
func TestPackUnpackPoundSign(t *testing.T) {
	pk := packet.NewPacket(nil)
	Pack(pk, "Cost £5") // already sentence-cased
	pk2 := packet.NewPacket(pk.Data)
	got := Unpack(pk2, len(pk.Data))
	if got != "Cost £5" {
		t.Errorf("£ round-trip: got %q, want %q", got, "Cost £5")
	}
}

// TestUnpackEmpty pins zero-length decode behavior.
func TestUnpackEmpty(t *testing.T) {
	pk := packet.NewPacket(nil)
	got := Unpack(pk, 0)
	if got != "" {
		t.Errorf("empty Unpack: got %q, want \"\"", got)
	}
}

// TestPackEmpty pins zero-length encode behavior.
func TestPackEmpty(t *testing.T) {
	pk := packet.NewPacket(nil)
	Pack(pk, "")
	if len(pk.Data) != 0 {
		t.Errorf("empty Pack: got %d bytes, want 0", len(pk.Data))
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/wordenc/wordpack/... -run TestPack -v
```

Expected: build failure — `undefined: Pack`, `undefined: Unpack`. (No `wordpack.go` exists yet.)

- [ ] **Step 4: Write the implementation**

Create `pkg/wordenc/wordpack/wordpack.go`:

```go
// Package wordpack ports the TS WordPack codec (Engine-TS
// src/wordenc/WordPack.ts) used by the MessagePrivate handler to decode
// the word-packed chat payload. NAI-158.
//
// The codec uses a 60-entry character table indexed by 4-bit (indices
// 0-12) or 12-bit (indices 13-59, encoded as a carry nibble + 8 bits)
// nibble groups. Two nibbles fit in each byte.
package wordpack

import (
	"strings"

	"github.com/zsrv/goscape/pkg/io/packet"
)

// charLookup mirrors TS WordPack.CHAR_LOOKUP (WordPack.ts:5-12).
// Stored as []string instead of []byte because entry 56 ('£') is a
// multi-byte UTF-8 codepoint — preserving it as a length-1 substring
// keeps the TS semantics of "one table slot per character".
var charLookup = []string{
	" ",
	"e", "t", "a", "o", "i", "h", "n", "s", "r", "d", "l", "u", "m",
	"w", "c", "y", "f", "g", "p", "b", "v", "k", "x", "j", "q", "z",
	"0", "1", "2", "3", "4", "5", "6", "7", "8", "9",
	" ", "!", "?", ".", ",", ":", ";", "(", ")", "-",
	"&", "*", "\\", "'", "@", "#", "+", "=", "£", "$", "%", "\"", "[", "]",
}

// Unpack decodes length bytes of word-packed input from pk starting at
// pk.Pos, returning the sentence-cased plain text. Mirrors TS
// WordPack.unpack (WordPack.ts:14-41).
//
// Output is capped at 80 characters per TS line 19 (`pos < 80`).
func Unpack(pk *packet.Packet, length int) string {
	var parts []string
	pos := 0
	carry := -1
	for i := 0; i < length && pos < 80; i++ {
		data := int(pk.G1())
		nibble := (data >> 4) & 0xf
		if carry != -1 {
			parts = append(parts, charLookup[(carry<<4)+nibble-195])
			pos++
			carry = -1
		} else if nibble < 13 {
			parts = append(parts, charLookup[nibble])
			pos++
		} else {
			carry = nibble
		}
		nibble = data & 0xf
		if carry != -1 {
			parts = append(parts, charLookup[(carry<<4)+nibble-195])
			pos++
			carry = -1
		} else if nibble < 13 {
			parts = append(parts, charLookup[nibble])
			pos++
		} else {
			carry = nibble
		}
	}
	return toSentenceCase(strings.Join(parts, ""))
}

// Pack encodes input as word-packed bytes appended to pk. Input is
// lowercased and truncated to 80 characters first. Mirrors TS
// WordPack.pack (WordPack.ts:43-78).
func Pack(pk *packet.Packet, input string) {
	// Truncate to 80 runes (TS line 44-46 uses substring(0, 80) which
	// is UTF-16-code-unit-based; for the limited charLookup alphabet
	// all chars are single-rune so rune-count truncation matches).
	runes := []rune(strings.ToLower(input))
	if len(runes) > 80 {
		runes = runes[:80]
	}
	carry := -1
	for _, r := range runes {
		ch := string(r)
		index := 0
		for j := 0; j < len(charLookup); j++ {
			if ch == charLookup[j] {
				index = j
				break
			}
		}
		if index > 12 {
			index += 195
		}
		if carry == -1 {
			if index < 13 {
				carry = index
			} else {
				pk.P1(uint8(index))
			}
		} else if index < 13 {
			pk.P1(uint8((carry << 4) + index))
			carry = -1
		} else {
			pk.P1(uint8((carry << 4) + (index >> 4)))
			carry = index & 0xf
		}
	}
	if carry != -1 {
		pk.P1(uint8(carry << 4))
	}
}

// toSentenceCase mirrors TS WordPack.toSentenceCase (WordPack.ts:80-94):
// capitalize the first lowercase letter at the start of the string and
// after any '.' or '!'.
func toSentenceCase(input string) string {
	chars := []rune(strings.ToLower(input))
	punctuation := true
	for i, c := range chars {
		if punctuation && c >= 'a' && c <= 'z' {
			chars[i] = c - 'a' + 'A'
			punctuation = false
		}
		if c == '.' || c == '!' {
			punctuation = true
		}
	}
	return string(chars)
}
```

- [ ] **Step 5: Run tests to verify they pass**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/wordenc/wordpack/... -v
```

Expected: all 7 tests PASS (`TestPackUnpackRoundTrip` has 3 subtests).

- [ ] **Step 6: Run the full test suite to make sure nothing else broke**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
```

Expected: PASS. (Other packages cannot regress from a new isolated package.)

- [ ] **Step 7: Commit**

```bash
git add pkg/wordenc/wordpack/
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(wordenc): NAI-158 T1 — port WordPack codec from TS

Ports Engine-TS src/wordenc/WordPack.ts (95 LOC) to
pkg/wordenc/wordpack as a standalone package. Includes Unpack +
Pack + internal toSentenceCase, with round-trip tests pinning the
60-entry character table (including the multi-byte '£' entry).

Required by NAI-158 handler_message_private wire-up (T3).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: FriendsBridge.PrivateMessage + Server.pmCount + nextPmId

**Files:**
- Modify: `modules/world/bridges.go` (extend interface + noopBridges)
- Modify: `modules/world/bridges_test.go` (extend recorder + capture test)
- Modify: `modules/world/server.go` (add `pmCount uint32` field)
- Create: `modules/world/server_pmid.go` (nextPmId helper)
- Create: `modules/world/server_pmid_test.go` (3 tests)

Per-task TDD: extend the interface and capture test first (drives the recorder shape), then the pmCount + helper.

- [ ] **Step 1: Extend the FriendsBridge interface and noopBridges**

Edit `modules/world/bridges.go`:

Add the new method to the `FriendsBridge` interface (after `SetChatMode`):

```go
type FriendsBridge interface {
	AddFriend(playerUsername string, target uint64)
	RemoveFriend(playerUsername string, target uint64)
	AddIgnore(playerUsername string, target uint64)
	RemoveIgnore(playerUsername string, target uint64)
	SetChatMode(playerUsername string, privateChat int)
	// PrivateMessage posts a /tell-style private chat message to the
	// friends-server. Mirrors TS World.sendPrivateMessage payload
	// (World.ts:1631-1643): {username, staffLvl, pmId, target, message,
	// coord}. coord is the packed coordgrid.PackCoord value.
	// Real impl deferred via NAI-72-D-FRIENDS-SERVER-BRIDGE.
	PrivateMessage(playerUsername string, staffLvl int32, pmId uint32, target uint64, message string, coord int)
}
```

Add a no-op impl to `noopBridges` (alongside the other no-op methods):

```go
func (noopBridges) PrivateMessage(string, int32, uint32, uint64, string, int) {}
```

- [ ] **Step 2: Extend recordingBridges to capture PrivateMessage**

Edit `modules/world/bridges_test.go`:

Add a new record struct after `recordedInputTrackingCall`:

```go
type recordedPrivateMessageCall struct {
	method         string // "PrivateMessage"
	playerUsername string
	staffLvl       int32
	pmId           uint32
	target         uint64
	message        string
	coord          int
}
```

Add a `privateMsgs` field to `recordingBridges`:

```go
type recordingBridges struct {
	friends              []recordedFriendsCall
	loginMod             []recordedLoginModCall
	logger               []recordedLoggerCall
	inputTracks          []recordedInputTrackingCall // NAI-73
	submittedSessionLogs [][]SessionLog              // NAI-74 — one element per tick flush
	privateMsgs          []recordedPrivateMessageCall // NAI-158
}
```

Add the recorder method (after `SetChatMode`):

```go
func (r *recordingBridges) PrivateMessage(p string, staffLvl int32, pmId uint32, target uint64, message string, coord int) {
	r.privateMsgs = append(r.privateMsgs, recordedPrivateMessageCall{
		method: "PrivateMessage", playerUsername: p, staffLvl: staffLvl,
		pmId: pmId, target: target, message: message, coord: coord,
	})
}
```

Add a capture test (insert after `TestRecordingBridgesCapturesAllCalls`):

```go
// TestRecordingBridgesCapturesPrivateMessage pins the NAI-158
// PrivateMessage capture: every arg is recorded verbatim.
func TestRecordingBridgesCapturesPrivateMessage(t *testing.T) {
	rec := &recordingBridges{}
	rec.PrivateMessage("alice", 2, 0xDEADBEEF, 1234, "hi bob", 0xC0DE)
	if len(rec.privateMsgs) != 1 {
		t.Fatalf("privateMsgs: got %d, want 1", len(rec.privateMsgs))
	}
	got := rec.privateMsgs[0]
	if got.method != "PrivateMessage" || got.playerUsername != "alice" ||
		got.staffLvl != 2 || got.pmId != 0xDEADBEEF || got.target != 1234 ||
		got.message != "hi bob" || got.coord != 0xC0DE {
		t.Errorf("PrivateMessage record: %+v", got)
	}
}
```

Also extend the existing `TestNoopBridgesAllMethods` to exercise the new method (insert one line after `b.SetChatMode("u", 0)`):

```go
	b.PrivateMessage("u", 0, 0, 1, "x", 0)
```

- [ ] **Step 3: Run tests to verify they fail**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestRecordingBridgesCapturesPrivateMessage|TestNoopBridgesAllMethods" -v
```

Expected: build error — `noopBridges` does not implement `FriendsBridge` (missing `PrivateMessage`); or the new test fails. (If Step 1 was completed cleanly, the build passes and only the recorder test exercises new code; the compile-time `_ FriendsBridge = (*recordingBridges)(nil)` assertion at bridges_test.go:94 would already enforce method presence.)

If the build error blocks: confirm Step 1's edits to `bridges.go` are complete.

- [ ] **Step 4: Run tests to verify they pass**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestRecordingBridgesCapturesPrivateMessage|TestNoopBridgesAllMethods" -v
```

Expected: both PASS.

- [ ] **Step 5: Add pmCount field on Server struct**

Edit `modules/world/server.go` at the `type Server struct {` block (line 45). Add a new field. Place it near the other engine-counter fields if any exist; otherwise at the end of the struct before the closing brace. Add this exact line:

```go
	// pmCount is the monotonic counter feeding the low 16 bits of the
	// pmId stamped on each FriendThread private_message payload.
	// Mirrors TS World.pmCount. Used only by nextPmId (NAI-158).
	pmCount uint32
```

- [ ] **Step 6: Write the failing pmId tests**

Create `modules/world/server_pmid_test.go`:

```go
package world

import "testing"

// TestNextPmIdCounterMonotone pins the low 16 bits of pmId as a
// monotonically increasing counter, and confirms pmCount advances by 1
// per call. Random byte (bits 16-23) is masked before assertion per
// memory:no_rng_seam_cascade_probe_bypass.md — pkg/script and friends
// use math/rand/v2 globally with no test seam.
func TestNextPmIdCounterMonotone(t *testing.T) {
	s := newTestServer(t)
	s.cfg.NodeID = 0
	got := []uint32{s.nextPmId(), s.nextPmId(), s.nextPmId()}
	// Mask out the random byte (bits 16-23); compare NodeID byte + counter.
	const randMask = uint32(0xff00ffff)
	want := []uint32{0x00000000, 0x00000001, 0x00000002}
	for i, g := range got {
		if g&randMask != want[i] {
			t.Errorf("nextPmId[%d]: got %08x masked %08x, want %08x",
				i, g, g&randMask, want[i])
		}
	}
	if s.pmCount != 3 {
		t.Errorf("pmCount: got %d, want 3", s.pmCount)
	}
}

// TestNextPmIdNodeIDByte pins the high 8 bits to cfg.NodeID & 0xff.
// Mirrors TS World.sendPrivateMessage (World.ts:1641) where
// Environment.NODE_ID populates bits 24-31.
func TestNextPmIdNodeIDByte(t *testing.T) {
	s := newTestServer(t)
	s.cfg.NodeID = 0x42
	pm := s.nextPmId()
	if (pm>>24)&0xff != 0x42 {
		t.Errorf("NodeID byte: got %02x, want 0x42 (pm=%08x)", (pm>>24)&0xff, pm)
	}
}

// TestNextPmIdRandByteInRange pins the TS off-by-one from
// Math.random()*0xff producing values in [0, 254]. The Go port uses
// rand.IntN(0xff) which yields [0, 254], NOT rand.IntN(256).
func TestNextPmIdRandByteInRange(t *testing.T) {
	s := newTestServer(t)
	s.cfg.NodeID = 0
	for i := 0; i < 64; i++ {
		// Reset pmCount each iteration so it doesn't pollute bits 0-15
		// when the counter wraps into bit 16+ (would take 65536 calls
		// in practice; defensive reset keeps the test independent).
		s.pmCount = 0
		pm := s.nextPmId()
		randByte := (pm >> 16) & 0xff
		if randByte > 0xfe {
			t.Errorf("iter %d: rand byte %d > 254 (pm=%08x)", i, randByte, pm)
		}
	}
}
```

- [ ] **Step 7: Run tests to verify they fail**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestNextPmId -v
```

Expected: build error — `s.nextPmId` undefined.

- [ ] **Step 8: Implement nextPmId**

Create `modules/world/server_pmid.go`:

```go
package world

import "math/rand/v2"

// nextPmId mirrors the pmId computation inside TS
// World.sendPrivateMessage (World.ts:1641):
//
//	(Environment.NODE_ID << 24) + ((Math.random() * 0xff) << 16)
//	  + this.pmCount++
//
// Bit layout (MSB to LSB):
//
//	bits 24-31: cfg.NodeID & 0xff
//	bits 16-23: random byte in [0, 254]
//	bits 0-15:  pmCount (post-increment)
//
// Uses rand.IntN(0xff) (range [0, 254]) — NOT rand.IntN(256) — to match
// the TS off-by-one from Math.random()*0xff yielding strictly less than
// 0xff. Test seam absent per memory:no_rng_seam_cascade_probe_bypass.md;
// tests mask bits 16-23 to assert deterministic parts.
func (s *Server) nextPmId() uint32 {
	randByte := uint32(rand.IntN(0xff))
	pm := uint32(s.cfg.NodeID&0xff)<<24 | randByte<<16 | s.pmCount
	s.pmCount++
	return pm
}
```

- [ ] **Step 9: Run tests to verify they pass**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestNextPmId|TestRecordingBridgesCapturesPrivateMessage|TestNoopBridgesAllMethods" -v
```

Expected: all PASS.

- [ ] **Step 10: Run the full test suite**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
```

Expected: PASS. The new method is non-disruptive — every existing `FriendsBridge` consumer continues to compile (noopBridges + recordingBridges both implement it).

- [ ] **Step 11: Commit**

```bash
git add modules/world/bridges.go modules/world/bridges_test.go \
        modules/world/server.go modules/world/server_pmid.go \
        modules/world/server_pmid_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-158 T2 — FriendsBridge.PrivateMessage + Server.pmCount

Extends FriendsBridge with a PrivateMessage(playerUsername, staffLvl,
pmId, target, message, coord) method mirroring the TS World.friendThread
'private_message' payload (World.ts:1631-1643). Adds Server.pmCount
uint32 and Server.nextPmId() helper computing the TS bit layout:
NodeID<<24 | rand<<16 | pmCount++. Real friends-server impl deferred
via NAI-72-D-FRIENDS-SERVER-BRIDGE.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: Handler + dispatcher

**Files:**
- Create: `modules/world/handler_message_private.go`
- Create: `modules/world/handler_message_private_test.go`
- Modify: `modules/world/handlers_game.go` (one line + import if needed)

- [ ] **Step 1: Write the failing handler tests**

Create `modules/world/handler_message_private_test.go`:

```go
package world

import (
	"testing"
	"time"

	"github.com/zsrv/goscape/pkg/io/packet"
	util "github.com/zsrv/goscape/pkg/util/jstring"
	"github.com/zsrv/goscape/pkg/wordenc/wordpack"
)

// commonMessagePrivateSetup wires a player against a server with
// recording bridges and a known username. Mirrors commonSocialListSetup
// in handler_social_list_test.go.
func commonMessagePrivateSetup(t *testing.T) (*Player, *recordingBridges) {
	t.Helper()
	s := newTestServer(t)
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.username = "alice"
	rec := installRecordingBridges(s)
	return p, rec
}

// packMessagePayload returns an opcode-148 payload: 8 bytes target
// (big-endian, matching the encoding read by Packet.G8 — see
// payloadG8 in handler_social_list_test.go) followed by word-packed
// message bytes.
func packMessagePayload(target uint64, message string) []byte {
	out := payloadG8(target)
	pk := packet.NewPacket(nil)
	wordpack.Pack(pk, message)
	return append(out, pk.Data...)
}

// TestHandleMessagePrivateHappyPath: bridge receives the message,
// socialProtect flips true, pmCount advances.
func TestHandleMessagePrivateHappyPath(t *testing.T) {
	p, rec := commonMessagePrivateSetup(t)
	target := util.ToBase37("bob")

	if err := handleMessagePrivate(p, packMessagePayload(target, "hi")); err != nil {
		t.Fatalf("handleMessagePrivate: %v", err)
	}
	if len(rec.privateMsgs) != 1 {
		t.Fatalf("privateMsgs: got %d, want 1", len(rec.privateMsgs))
	}
	got := rec.privateMsgs[0]
	if got.playerUsername != "alice" {
		t.Errorf("playerUsername: got %q, want alice", got.playerUsername)
	}
	if got.target != target {
		t.Errorf("target: got %d, want %d", got.target, target)
	}
	if got.message != "Hi" { // Unpack applies sentence-case to "hi"
		t.Errorf("message: got %q, want %q", got.message, "Hi")
	}
	if got.staffLvl != p.staffModLevel {
		t.Errorf("staffLvl: got %d, want %d", got.staffLvl, p.staffModLevel)
	}
	// NodeID byte of pmId. cfg.NodeID is 0 (default from newTestServer);
	// counter is 0 for the first call. Random byte masked out.
	if got.pmId&0xff00ffff != 0 {
		t.Errorf("pmId structure: got %08x masked %08x, want 0 (NodeID=0, counter=0)",
			got.pmId, got.pmId&0xff00ffff)
	}
	if !p.socialProtect {
		t.Error("socialProtect: must be true after successful PrivateMessage")
	}
	if p.client.server.pmCount != 1 {
		t.Errorf("pmCount: got %d, want 1", p.client.server.pmCount)
	}
}

// TestHandleMessagePrivateGatedBySocialProtect: early-return when
// p.socialProtect is already set; no bridge call, no protect-set.
func TestHandleMessagePrivateGatedBySocialProtect(t *testing.T) {
	p, rec := commonMessagePrivateSetup(t)
	p.socialProtect = true
	target := util.ToBase37("bob")

	if err := handleMessagePrivate(p, packMessagePayload(target, "hi")); err != nil {
		t.Fatalf("handleMessagePrivate: %v", err)
	}
	if len(rec.privateMsgs) != 0 {
		t.Errorf("privateMsgs: got %d, want 0 (gated)", len(rec.privateMsgs))
	}
	if len(rec.loginMod) != 0 {
		t.Errorf("loginMod: got %d, want 0 (no ban expected)", len(rec.loginMod))
	}
	if p.client.server.pmCount != 0 {
		t.Errorf("pmCount: got %d, want 0 (gate fires before nextPmId)", p.client.server.pmCount)
	}
}

// TestHandleMessagePrivateGatedByLengthCap: payload with > 100 bytes of
// word-packed input is dropped.
func TestHandleMessagePrivateGatedByLengthCap(t *testing.T) {
	p, rec := commonMessagePrivateSetup(t)
	target := util.ToBase37("bob")

	// 101 bytes of word-packed tail after the 8-byte target.
	payload := payloadG8(target)
	tail := make([]byte, 101)
	payload = append(payload, tail...)

	if err := handleMessagePrivate(p, payload); err != nil {
		t.Fatalf("handleMessagePrivate: %v", err)
	}
	if len(rec.privateMsgs) != 0 {
		t.Errorf("privateMsgs: got %d, want 0 (length>100 gated)", len(rec.privateMsgs))
	}
	if p.socialProtect {
		t.Error("socialProtect: must remain false on length-gated branch")
	}
	if p.client.server.pmCount != 0 {
		t.Errorf("pmCount: got %d, want 0", p.client.server.pmCount)
	}
}

// TestHandleMessagePrivateGatedByMutedUntil: mute window active → drop.
func TestHandleMessagePrivateGatedByMutedUntil(t *testing.T) {
	p, rec := commonMessagePrivateSetup(t)
	p.mutedUntil = time.Now().Add(time.Hour)
	target := util.ToBase37("bob")

	if err := handleMessagePrivate(p, packMessagePayload(target, "hi")); err != nil {
		t.Fatalf("handleMessagePrivate: %v", err)
	}
	if len(rec.privateMsgs) != 0 {
		t.Errorf("privateMsgs: got %d, want 0 (muted)", len(rec.privateMsgs))
	}
	if len(rec.loginMod) != 0 {
		t.Errorf("loginMod: got %d, want 0 (no ban on mute branch)", len(rec.loginMod))
	}
	if p.socialProtect {
		t.Error("socialProtect: must remain false on mute branch")
	}
}

// TestHandleMessagePrivateInvalidNameTriggersBan: invalid_name base37
// decode → 48h automated ban; no friends bridge call; no protect-set.
// The sentinel value comes from pkg/util/jstring/jstring_test.go line 7.
func TestHandleMessagePrivateInvalidNameTriggersBan(t *testing.T) {
	p, rec := commonMessagePrivateSetup(t)
	const invalidNameSentinel uint64 = 6582952005840035281

	before := time.Now()
	if err := handleMessagePrivate(p, packMessagePayload(invalidNameSentinel, "hi")); err != nil {
		t.Fatalf("handleMessagePrivate: %v", err)
	}
	after := time.Now()

	if len(rec.privateMsgs) != 0 {
		t.Errorf("privateMsgs: got %d, want 0 (banned)", len(rec.privateMsgs))
	}
	if len(rec.loginMod) != 1 {
		t.Fatalf("loginMod: got %d, want 1", len(rec.loginMod))
	}
	mod := rec.loginMod[0]
	if mod.method != "NotifyPlayerBan" {
		t.Errorf("method: got %q, want NotifyPlayerBan", mod.method)
	}
	if mod.staff != "automated" {
		t.Errorf("staff: got %q, want automated", mod.staff)
	}
	if mod.username != "alice" {
		t.Errorf("username: got %q, want alice", mod.username)
	}
	// Until must be ~48h from now (allow ±5s window for test latency).
	lo := before.Add(48*time.Hour - 5*time.Second)
	hi := after.Add(48*time.Hour + 5*time.Second)
	if mod.until.Before(lo) || mod.until.After(hi) {
		t.Errorf("until: got %v, want within %v..%v", mod.until, lo, hi)
	}
	if p.socialProtect {
		t.Error("socialProtect: must remain false on ban branch")
	}
	if p.client.server.pmCount != 0 {
		t.Errorf("pmCount: got %d, want 0", p.client.server.pmCount)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestHandleMessagePrivate -v
```

Expected: build error — `undefined: handleMessagePrivate`.

- [ ] **Step 3: Implement the handler**

Create `modules/world/handler_message_private.go`:

```go
package world

import (
	"time"

	"github.com/zsrv/goscape/pkg/coordgrid"
	"github.com/zsrv/goscape/pkg/io/packet"
	util "github.com/zsrv/goscape/pkg/util/jstring"
	"github.com/zsrv/goscape/pkg/wordenc/wordpack"
)

// handleMessagePrivate handles client opcode 148 (MESSAGE_PRIVATE),
// dynamic 1-byte length. Wire: G8 target(base37) + word-packed input
// bytes (variable length, payload tail). NAI-158.
//
// Mirrors TS MessagePrivateHandler.ts:10-35. Gate order (no protect-set
// on any early return):
//  1. socialProtect || len(input) > 100 → return.
//  2. mutedUntil active → return.
//  3. invalid_name base37 → automated 48h ban; return.
//  4. WordPack.Unpack; friendsBridge.PrivateMessage; socialProtect=true.
//
// Friends-server propagation deferred via NAI-72-D-FRIENDS-SERVER-BRIDGE
// (PrivateMessage bridge is a stub). LoginBridgeMod.NotifyPlayerBan is
// the same stub pattern used by handler_reportabuse.go:50.
func handleMessagePrivate(p *Player, payload []byte) error {
	if p.client == nil || p.client.server == nil {
		// goscape defensive; TS reaches via static accessor.
		return nil
	}
	pk := packet.NewPacket(payload)
	target := pk.G8()
	inputLen := len(payload) - 8
	if p.socialProtect || inputLen > 100 {
		return nil
	}
	if !p.mutedUntil.IsZero() && time.Now().Before(p.mutedUntil) {
		return nil
	}
	s := p.client.server
	if util.FromBase37(target) == "invalid_name" {
		s.loginBridgeMod.NotifyPlayerBan("automated", p.username, time.Now().Add(48*time.Hour))
		return nil
	}
	msg := wordpack.Unpack(pk, inputLen)
	coord := coordgrid.PackCoord(p.level, p.x, p.z)
	s.friendsBridge.PrivateMessage(p.username, p.staffModLevel, s.nextPmId(), target, msg, coord)
	p.socialProtect = true
	return nil
}
```

- [ ] **Step 4: Wire the dispatcher**

Edit `modules/world/handlers_game.go`. Locate the line:

```go
	gameHandlers[158] = handleMessagePublic // MESSAGE_PUBLIC
```

Add a new entry on the line above it:

```go
	gameHandlers[148] = handleMessagePrivate // MESSAGE_PRIVATE
	gameHandlers[158] = handleMessagePublic  // MESSAGE_PUBLIC
```

- [ ] **Step 5: Run handler tests to verify they pass**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestHandleMessagePrivate -v
```

Expected: all 5 tests PASS.

- [ ] **Step 6: Run the full test suite**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
```

Expected: PASS across all packages.

- [ ] **Step 7: Run go vet for static checks**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...
```

Expected: clean.

- [ ] **Step 8: Commit**

```bash
git add modules/world/handler_message_private.go \
        modules/world/handler_message_private_test.go \
        modules/world/handlers_game.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-158 T3 — MessagePrivate handler (opcode 148)

Ports Engine-TS MessagePrivateHandler.ts to
modules/world/handler_message_private.go and wires opcode 148 into
the gameHandlers dispatch table. Five test cases pin the gate order:
happy path, socialProtect gate, length>100 gate, mutedUntil gate,
and invalid_name → automated 48h ban via LoginBridgeMod (same stub
pattern as handler_reportabuse.go:50). Activates the previously-
dormant Player.mutedUntil field set by the login bridge.

Closes NAI-158.

Closes memory: runescript_cadence.md controller_preflight.md
true_to_ts_gate.md dead_api_polish.md
no_rng_seam_cascade_probe_bypass.md audit_full_method_against_ts.md
defensive_gate_doc_comment_label.md
helper_as_oracle_test_anti_pattern.md spec_ts_source_read.md

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Self-Review Notes

Plan covers every section of the spec:

- §5 Architecture → T3 handler + dispatcher
- §6.1 `pkg/wordenc/wordpack` → T1
- §6.2 `FriendsBridge.PrivateMessage` → T2 Step 1
- §6.3 `Server.pmCount` → T2 Step 5
- §6.4 `nextPmId` helper → T2 Steps 6-9
- §6.5 handler → T3 Steps 1-5
- §6.6 dispatcher → T3 Step 4
- §7 data flow → encoded in T3 handler body
- §8 error handling → defensive nil-check + gate returns matching sibling-handler precedent
- §9.1 wordpack tests → T1 Step 2 (7 cases)
- §9.2 bridges tests → T2 Step 2 (PrivateMessage capture + noopBridges extension)
- §9.3 server_pmid tests → T2 Step 6 (3 cases)
- §9.4 handler tests → T3 Step 1 (5 cases)
- §10 No active deviations → confirmed by ban path using existing LoginBridgeMod
- §11 task split → T1/T2/T3 commits
- §12 memory hits → cited in T3 commit trailer (close commit)

Type consistency check: `coord int` is used uniformly across §6.2 spec, T2 Step 1 bridge interface, T2 Step 2 recorder struct, T2 Step 8 noop sig, T3 Step 3 handler body (matches `coordgrid.PackCoord(level, x, z int) int`). `pmId uint32`, `staffLvl int32`, `target uint64` consistent across all sites. `pmCount uint32` matches both the bit layout (16-bit room before random byte) and the Server field.

No placeholders. Every code block contains the actual content the implementer needs.
