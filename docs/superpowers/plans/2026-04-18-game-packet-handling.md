# Game Packet Handling Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Complete the login handshake (send OK byte, transition state) and add in-game packet reading with a full opcode table, ISAAC-decrypted dispatch, and handlers for keepalive and movement packets.

**Architecture:** A `[256]Op` table in `pkg/io/protocol/game/client/` defines all ~50 known game opcodes. `handleGame()` in `client_game.go` peeks one byte, ISAAC-decrypts it, looks up the opcode, resolves variable-length prefixes, and dispatches to a `[256]func(*client, []byte) error` handler table. The `c.opcode`/`c.waiting` fields on `client` act as a resume cursor so partial reads are safe across TCP recv calls.

**Tech Stack:** Go 1.26, `pkg/io/packet`, `pkg/io/isaac`, `pkg/io/protocol`

---

### Task 1: Opcode table + ClientStateGame

**Files:**
- Modify: `modules/world/client.go`
- Create: `pkg/io/protocol/game/client/prot.go`
- Test: `modules/world/server_test.go`

- [ ] **Step 1: Add ClientStateGame constant**

In `modules/world/client.go`, extend the const block:

```go
const (
	ClientStateClosed ClientState = -1
	ClientStateLogin  ClientState = 0
	ClientStateGame   ClientState = 1
)
```

- [ ] **Step 2: Write failing test for opcode table**

Add to `modules/world/server_test.go`. Add import `gameclient "github.com/zsrv/goscape/pkg/io/protocol/game/client"`:

```go
func TestGameProtTableHasExpectedOpcodes(t *testing.T) {
	cases := []struct {
		opcode      int
		name        string
		payloadSize int
	}{
		{108, "NO_TIMEOUT", 0},
		{70, "IDLE_TIMER", 0},
		{181, "MOVE_GAMECLICK", -1},
		{93, "MOVE_OPCLICK", -1},
		{165, "MOVE_MINIMAPCLICK", -1},
		{150, "REBUILD_GETMAPS", -1},
		{81, "EVENT_TRACKING", -2},
	}
	for _, tc := range cases {
		op := gameclient.Ops[tc.opcode]
		if op.Name != tc.name {
			t.Errorf("Ops[%d].Name = %q, want %q", tc.opcode, op.Name, tc.name)
		}
		if op.PayloadSize != tc.payloadSize {
			t.Errorf("Ops[%d].PayloadSize = %d, want %d", tc.opcode, op.PayloadSize, tc.payloadSize)
		}
	}
}
```

- [ ] **Step 3: Run test to confirm it fails**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestGameProtTableHasExpectedOpcodes -v
```

Expected: compile error — package `gameclient` not found.

- [ ] **Step 4: Create opcode table**

Create `pkg/io/protocol/game/client/prot.go`:

```go
package client

// Op describes a client game packet opcode.
type Op struct {
	Name        string
	PayloadSize int // 0=fixed-zero, N=fixed-N, -1=1-byte-len, -2=2-byte-len
}

// Ops is a 256-entry lookup table indexed by decrypted game opcode.
// A zero-value Op (empty Name) means the opcode is unknown.
var Ops [256]Op

func init() {
	set := func(opcode uint8, name string, payloadSize int) {
		Ops[opcode] = Op{Name: name, PayloadSize: payloadSize}
	}

	set(150, "REBUILD_GETMAPS", -1)
	set(108, "NO_TIMEOUT", 0)
	set(70, "IDLE_TIMER", 0)
	set(81, "EVENT_TRACKING", -2)
	set(189, "EVENT_CAMERA_POSITION", 6)

	set(7, "ANTICHEAT_OPLOGIC1", 4)
	set(88, "ANTICHEAT_OPLOGIC2", 4)
	set(30, "ANTICHEAT_OPLOGIC3", 3)
	set(176, "ANTICHEAT_OPLOGIC4", 2)
	set(220, "ANTICHEAT_OPLOGIC5", 0)
	set(66, "ANTICHEAT_OPLOGIC6", 4)
	set(17, "ANTICHEAT_OPLOGIC7", 4)
	set(2, "ANTICHEAT_OPLOGIC8", 2)
	set(238, "ANTICHEAT_OPLOGIC9", 1)

	set(233, "ANTICHEAT_CYCLELOGIC1", 1)
	set(146, "ANTICHEAT_CYCLELOGIC2", -1)
	set(215, "ANTICHEAT_CYCLELOGIC3", 3)
	set(236, "ANTICHEAT_CYCLELOGIC4", 4)
	set(85, "ANTICHEAT_CYCLELOGIC5", 0)
	set(219, "ANTICHEAT_CYCLELOGIC6", -1)

	set(140, "OPOBJ1", 6)
	set(40, "OPOBJ2", 6)
	set(200, "OPOBJ3", 6)
	set(178, "OPOBJ4", 6)
	set(247, "OPOBJ5", 6)
	set(138, "OPOBJT", 8)
	set(239, "OPOBJU", 12)

	set(194, "OPNPC1", 2)
	set(8, "OPNPC2", 2)
	set(27, "OPNPC3", 2)
	set(113, "OPNPC4", 2)
	set(100, "OPNPC5", 2)
	set(134, "OPNPCT", 4)
	set(202, "OPNPCU", 8)

	set(245, "OPLOC1", 6)
	set(172, "OPLOC2", 6)
	set(96, "OPLOC3", 6)
	set(97, "OPLOC4", 6)
	set(116, "OPLOC5", 6)
	set(9, "OPLOCT", 8)
	set(75, "OPLOCU", 12)

	set(164, "OPPLAYER1", 2)
	set(53, "OPPLAYER2", 2)
	set(185, "OPPLAYER3", 2)
	set(206, "OPPLAYER4", 2)
	set(177, "OPPLAYERT", 4)
	set(248, "OPPLAYERU", 8)

	set(195, "OPHELD1", 6)
	set(71, "OPHELD2", 6)
	set(133, "OPHELD3", 6)
	set(157, "OPHELD4", 6)
	set(211, "OPHELD5", 6)
	set(48, "OPHELDT", 8)
	set(130, "OPHELDU", 12)

	set(31, "INV_BUTTON1", 6)
	set(59, "INV_BUTTON2", 6)
	set(212, "INV_BUTTON3", 6)
	set(38, "INV_BUTTON4", 6)
	set(6, "INV_BUTTON5", 6)

	set(155, "IF_BUTTON", 2)
	set(235, "RESUME_PAUSEBUTTON", 2)
	set(231, "CLOSE_MODAL", 0)
	set(237, "RESUME_P_COUNTDIALOG", 4)
	set(175, "TUT_CLICKSIDE", 1)

	set(93, "MOVE_OPCLICK", -1)
	set(190, "REPORT_ABUSE", 10)
	set(165, "MOVE_MINIMAPCLICK", -1)
	set(159, "INV_BUTTOND", 6)
	set(171, "IGNORELIST_DEL", 8)
	set(79, "IGNORELIST_ADD", 8)
	set(52, "IDK_SAVEDESIGN", 13)
	set(244, "CHAT_SETMODE", 3)
	set(148, "MESSAGE_PRIVATE", -1)
	set(11, "FRIENDLIST_DEL", 8)
	set(118, "FRIENDLIST_ADD", 8)
	set(4, "CLIENT_CHEAT", -1)
	set(158, "MESSAGE_PUBLIC", -1)
	set(181, "MOVE_GAMECLICK", -1)
}
```

- [ ] **Step 5: Run test**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestGameProtTableHasExpectedOpcodes -v
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add modules/world/client.go pkg/io/protocol/game/client/prot.go modules/world/server_test.go
git commit --no-gpg-sign -m "feat: add ClientStateGame and game opcode table"
```

---

### Task 2: Complete login handshake — send OK byte, transition state

**Files:**
- Modify: `modules/world/client.go` (add `sendLoginOK`)
- Modify: `modules/world/server.go` (wire `sendLoginOK` into `handleLogin`)
- Test: `modules/world/server_test.go`

- [ ] **Step 1: Write failing tests**

Add to `modules/world/server_test.go`. Existing import `loginresp` is already present:

```go
func TestSendLoginOKSendsOpOKAndTransitionsState(t *testing.T) {
	c, clientConn := newTestClient(t)

	received := make(chan byte, 1)
	go func() {
		buf := make([]byte, 1)
		clientConn.SetReadDeadline(time.Now().Add(time.Second))
		if _, err := io.ReadFull(clientConn, buf); err == nil {
			received <- buf[0]
		}
	}()

	if err := c.sendLoginOK(); err != nil {
		t.Fatalf("sendLoginOK: %v", err)
	}

	select {
	case got := <-received:
		if got != loginresp.OpOK.Opcode {
			t.Errorf("login OK byte: got %d, want %d", got, loginresp.OpOK.Opcode)
		}
	case <-time.After(time.Second):
		t.Error("timed out waiting for login OK byte")
	}

	if c.state != ClientStateGame {
		t.Errorf("state after sendLoginOK: got %v, want ClientStateGame", c.state)
	}
}

func TestSendLoginOKStaffSendsRightsByte(t *testing.T) {
	c, clientConn := newTestClient(t)
	c.staffModLevel = 1

	received := make(chan byte, 1)
	go func() {
		buf := make([]byte, 1)
		clientConn.SetReadDeadline(time.Now().Add(time.Second))
		if _, err := io.ReadFull(clientConn, buf); err == nil {
			received <- buf[0]
		}
	}()

	if err := c.sendLoginOK(); err != nil {
		t.Fatalf("sendLoginOK: %v", err)
	}

	select {
	case got := <-received:
		if got != loginresp.OpLoginOKWithRights.Opcode {
			t.Errorf("staff login OK byte: got %d, want %d", got, loginresp.OpLoginOKWithRights.Opcode)
		}
	case <-time.After(time.Second):
		t.Error("timed out waiting for staff login OK byte")
	}
}
```

- [ ] **Step 2: Run tests to confirm they fail**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run "TestSendLoginOK" -v
```

Expected: compile error — `sendLoginOK` undefined.

- [ ] **Step 3: Add `sendLoginOK` to `client.go`**

Add imports `"fmt"` and `loginresp "github.com/zsrv/goscape/pkg/io/protocol/login/resp"` to `modules/world/client.go`.

Add method:

```go
// sendLoginOK sends the appropriate login-accepted byte based on staff level,
// flushes the write buffer, and transitions the client to ClientStateGame.
func (c *client) sendLoginOK() error {
	if c.staffModLevel >= 1 {
		c.bufw.WriteByte(loginresp.OpLoginOKWithRights.Opcode)
	} else {
		c.bufw.WriteByte(loginresp.OpOK.Opcode)
	}
	if err := c.flushWrite(); err != nil {
		return fmt.Errorf("failed to flush login OK: %w", err)
	}
	c.state = ClientStateGame
	return nil
}
```

- [ ] **Step 4: Wire into `handleLogin()` in `server.go`**

Replace the line:

```go
c.log.Info("END OF LOGIN", "safename", safeName, "reply", reply, "reconnecting", reconnecting)
```

with:

```go
c.log.Info("login accepted", "safename", safeName, "reply", reply, "reconnecting", reconnecting)
return c.sendLoginOK()
```

- [ ] **Step 5: Run tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run "TestSendLoginOK" -v
```

Expected: PASS for both tests.

- [ ] **Step 6: Run all tests to confirm no regressions**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... 2>&1 | tail -10
```

Expected: all PASS.

- [ ] **Step 7: Commit**

```bash
git add modules/world/client.go modules/world/server.go modules/world/server_test.go
git commit --no-gpg-sign -m "feat: send login OK byte and transition to game state"
```

---

### Task 3: Implement `handleGame()` dispatcher

**Files:**
- Create: `modules/world/client_game.go`
- Create: `modules/world/handlers_game.go` (stub — populated in Task 4)
- Modify: `modules/world/server.go` (add `ClientStateGame` case to `handleData`)
- Test: `modules/world/server_test.go`

- [ ] **Step 1: Write failing tests**

Add to `modules/world/server_test.go`. Add import `io2 "github.com/zsrv/goscape/pkg/io/isaac"`:

```go
// isaacPair returns two independent ISAAC instances with identical initial state.
// Use enc to encrypt opcodes in the test, dec to give to the client under test.
func isaacPair(seed [4]uint32) (enc, dec *io2.Isaac) {
	return io2.New(seed), io2.New(seed)
}

// encryptOpcode produces the wire byte the Java client sends for realOpcode.
func encryptOpcode(enc *io2.Isaac, realOpcode byte) byte {
	return byte((int(realOpcode) + int(enc.GetNext())) & 0xff)
}

func TestHandleGameEmptyBufferReturnsErrPayloadTooSmall(t *testing.T) {
	_, dec := isaacPair([4]uint32{1, 2, 3, 4})
	c, _ := newTestClient(t)
	c.state = ClientStateGame
	c.decryptor = dec

	err := c.handleGame()
	if !errors.Is(err, protocol.ErrPayloadTooSmall) {
		t.Errorf("empty buffer: got %v, want ErrPayloadTooSmall", err)
	}
}

func TestHandleGameUnknownOpcodeReturnsErrCloseConn(t *testing.T) {
	// Opcode 0 is not registered in the Ops table.
	enc, dec := isaacPair([4]uint32{5, 6, 7, 8})
	c, _ := newTestClient(t)
	c.state = ClientStateGame
	c.decryptor = dec

	c.in.Write([]byte{encryptOpcode(enc, 0)})

	err := c.handleGame()
	if !errors.Is(err, errCloseConn) {
		t.Errorf("unknown opcode: got %v, want errCloseConn", err)
	}
}
```

Add import `protocol "github.com/zsrv/goscape/pkg/io/protocol"` to `server_test.go`.

- [ ] **Step 2: Run tests to confirm they fail**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run "TestHandleGame" -v
```

Expected: compile error — `handleGame` undefined.

- [ ] **Step 3: Create stub `handlers_game.go`**

Create `modules/world/handlers_game.go`:

```go
package world

// gameHandlers is indexed by decrypted game opcode. Nil means no handler
// registered for that opcode; handleGame() silently discards such packets
// (they still must be in Ops[] to be accepted at all).
var gameHandlers [256]func(*client, []byte) error
```

- [ ] **Step 4: Create `client_game.go`**

Create `modules/world/client_game.go`:

```go
package world

import (
	"github.com/zsrv/goscape/pkg/io/protocol"
	gameclient "github.com/zsrv/goscape/pkg/io/protocol/game/client"
)

// handleGame reads and dispatches ISAAC-encrypted game packets from c.in.
//
// It drains all fully-buffered packets in a loop and returns ErrPayloadTooSmall
// when no complete packet is available. The caller (handleTCPConn) will then
// wait for more TCP data before calling handleData() again.
//
// c.opcode and c.waiting act as a resume cursor: they are set as soon as bytes
// are consumed from c.in, so a partial read on any TCP recv is safe.
func (c *client) handleGame() error {
	if c.decryptor == nil {
		c.log.Error("decryptor nil in game state")
		return errCloseConn
	}

	for {
		// Read and ISAAC-decrypt the next opcode if we don't have one pending.
		if c.opcode == -1 {
			raw, err := c.in.Peek(1)
			if err != nil {
				return protocol.ErrPayloadTooSmall
			}
			decrypted := (int(raw[0]) - int(c.decryptor.GetNext())) & 0xff
			op := gameclient.Ops[decrypted]
			if op.Name == "" {
				c.log.Warn("unknown game opcode", "opcode", decrypted)
				return errCloseConn
			}
			c.in.Next(1) // consume opcode byte — ISAAC has already advanced
			c.opcode = decrypted
			c.waiting = op.PayloadSize
		}

		// Resolve 1-byte or 2-byte dynamic length prefix.
		if c.waiting == -1 {
			if c.in.Len() < 1 {
				return protocol.ErrPayloadTooSmall
			}
			c.waiting = int(c.in.Next(1)[0])
		} else if c.waiting == -2 {
			if c.in.Len() < 2 {
				return protocol.ErrPayloadTooSmall
			}
			b := c.in.Next(2)
			c.waiting = int(uint16(b[0])<<8 | uint16(b[1]))
			if c.waiting > 1600 {
				c.log.Warn("oversized game packet, closing", "opcode", c.opcode, "size", c.waiting)
				return errCloseConn
			}
		}

		// Wait for the full payload.
		if c.in.Len() < c.waiting {
			return protocol.ErrPayloadTooSmall
		}

		// Consume payload and dispatch. Reset c.opcode before calling the
		// handler so the cursor is clean for the next packet.
		payload := c.in.Next(c.waiting)
		opcode := c.opcode
		c.opcode = -1

		c.log.Debug("game packet", "opcode", opcode, "name", gameclient.Ops[opcode].Name, "len", len(payload))

		if handler := gameHandlers[opcode]; handler != nil {
			if err := handler(c, payload); err != nil {
				return err
			}
		}
	}
}
```

- [ ] **Step 5: Add game case to `handleData()` in `server.go`**

Replace:

```go
func (c *client) handleData() error {
	switch c.state {
	case ClientStateLogin:
		return c.handleLogin()
	default:
		c.log.Info("unhandled client state", "state", c.state)
		return errors.New("unhandled client state")
	}
}
```

With:

```go
func (c *client) handleData() error {
	switch c.state {
	case ClientStateLogin:
		return c.handleLogin()
	case ClientStateGame:
		return c.handleGame()
	default:
		c.log.Info("unhandled client state", "state", c.state)
		return errors.New("unhandled client state")
	}
}
```

- [ ] **Step 6: Run tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run "TestHandleGame" -v
```

Expected: PASS for both tests.

- [ ] **Step 7: Commit**

```bash
git add modules/world/client_game.go modules/world/handlers_game.go modules/world/server.go modules/world/server_test.go
git commit --no-gpg-sign -m "feat: implement handleGame() ISAAC dispatcher"
```

---

### Task 4: Register keepalive and movement handlers

**Files:**
- Modify: `modules/world/handlers_game.go` (add `init` + handler funcs)
- Test: `modules/world/server_test.go`

- [ ] **Step 1: Write failing tests**

Add to `modules/world/server_test.go`:

```go
func TestHandleGameNoTimeoutCompletesAndResetsOpcode(t *testing.T) {
	enc, dec := isaacPair([4]uint32{10, 20, 30, 40})
	c, _ := newTestClient(t)
	c.state = ClientStateGame
	c.decryptor = dec

	// NO_TIMEOUT: opcode 108, payload size 0 — only the encrypted opcode byte
	c.in.Write([]byte{encryptOpcode(enc, 108)})

	err := c.handleGame()
	if !errors.Is(err, protocol.ErrPayloadTooSmall) {
		t.Errorf("after NO_TIMEOUT: got %v, want ErrPayloadTooSmall", err)
	}
	if c.opcode != -1 {
		t.Errorf("opcode after NO_TIMEOUT: got %d, want -1", c.opcode)
	}
}

func TestHandleGameMoveGameClickFullPacket(t *testing.T) {
	enc, dec := isaacPair([4]uint32{11, 22, 33, 44})
	c, _ := newTestClient(t)
	c.state = ClientStateGame
	c.decryptor = dec

	// MOVE_GAMECLICK: opcode 181, 1-byte length prefix
	// Payload: ctrlHeld(1) + startX G2(2) + startZ G2(2) = 5 bytes, no waypoints
	payload := []byte{
		0,          // ctrlHeld = 0
		0x0C, 0xA4, // startX = 3236
		0x0C, 0x8B, // startZ = 3211
	}
	var buf []byte
	buf = append(buf, encryptOpcode(enc, 181))
	buf = append(buf, byte(len(payload)))
	buf = append(buf, payload...)
	c.in.Write(buf)

	err := c.handleGame()
	if !errors.Is(err, protocol.ErrPayloadTooSmall) {
		t.Errorf("after MOVE_GAMECLICK: got %v, want ErrPayloadTooSmall", err)
	}
	if c.opcode != -1 {
		t.Errorf("opcode after MOVE_GAMECLICK: got %d, want -1", c.opcode)
	}
}

func TestHandleGamePartialPayloadPreservesOpcode(t *testing.T) {
	enc, dec := isaacPair([4]uint32{55, 66, 77, 88})
	c, _ := newTestClient(t)
	c.state = ClientStateGame
	c.decryptor = dec

	// Send opcode 181 + length byte claiming 10 payload bytes, but only 3 arrive
	buf := []byte{encryptOpcode(enc, 181), 10, 0x01, 0x02, 0x03}
	c.in.Write(buf)

	err := c.handleGame()
	if !errors.Is(err, protocol.ErrPayloadTooSmall) {
		t.Errorf("partial payload: got %v, want ErrPayloadTooSmall", err)
	}
	// c.opcode must be preserved so the next handleData() call resumes correctly
	if c.opcode != 181 {
		t.Errorf("opcode after partial read: got %d, want 181", c.opcode)
	}
}
```

- [ ] **Step 2: Run tests to confirm partial failure**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run "TestHandleGame(NoTimeout|MoveGame|Partial)" -v
```

Expected: `TestHandleGameNoTimeoutCompletesAndResetsOpcode` and `TestHandleGameMoveGameClickFullPacket` pass (nil handler = silently discard). `TestHandleGamePartialPayloadPreservesOpcode` should also pass — verify all three PASS before continuing.

- [ ] **Step 3: Populate `handlers_game.go` with `init` and handler funcs**

Replace the contents of `modules/world/handlers_game.go` with:

```go
package world

import (
	"github.com/zsrv/goscape/pkg/io/packet"
)

// gameHandlers is indexed by decrypted game opcode. Nil means no handler
// registered for that opcode; handleGame() silently discards such packets
// (they still must be in Ops[] to be accepted at all).
var gameHandlers [256]func(*client, []byte) error

func init() {
	// Keepalive — discard silently
	gameHandlers[108] = handleNoTimeout  // NO_TIMEOUT
	gameHandlers[70] = handleNoTimeout   // IDLE_TIMER

	// Movement
	gameHandlers[181] = handleMoveClick        // MOVE_GAMECLICK
	gameHandlers[93] = handleMoveClick         // MOVE_OPCLICK
	gameHandlers[165] = handleMoveMinimapClick // MOVE_MINIMAPCLICK
}

func handleNoTimeout(_ *client, _ []byte) error {
	return nil
}

// handleMoveClick decodes MOVE_GAMECLICK and MOVE_OPCLICK.
//
// Payload layout (from MoveClickDecoder.ts):
//   - 1 byte:  ctrlHeld
//   - 2 bytes: startX (G2, unsigned)
//   - 2 bytes: startZ (G2, unsigned)
//   - N pairs: signed-byte deltaX + signed-byte deltaZ (up to 24 waypoints)
func handleMoveClick(c *client, payload []byte) error {
	if len(payload) < 5 {
		return nil
	}
	r := packet.NewPacket(payload)
	ctrlHeld := r.G1()
	startX := r.G2()
	startZ := r.G2()

	type point struct{ x, z int }
	path := make([]point, 0, min((len(payload)-5)/2, 24)+1)
	path = append(path, point{int(startX), int(startZ)})
	for range min((len(payload)-5)/2, 24) {
		dx := r.G1B()
		dz := r.G1B()
		path = append(path, point{int(startX) + int(dx), int(startZ) + int(dz)})
	}

	c.log.Info("move click", "ctrl_held", ctrlHeld, "path", path)
	return nil
}

// handleMoveMinimapClick decodes MOVE_MINIMAPCLICK.
//
// Same layout as MOVE_GAMECLICK but with 14 trailing bytes (camera/anticheat
// data) that must be excluded from the waypoint count.
func handleMoveMinimapClick(c *client, payload []byte) error {
	const trailingBytes = 14
	if len(payload) < 5+trailingBytes {
		return nil
	}
	r := packet.NewPacket(payload)
	ctrlHeld := r.G1()
	startX := r.G2()
	startZ := r.G2()

	type point struct{ x, z int }
	path := make([]point, 0, min((len(payload)-5-trailingBytes)/2, 24)+1)
	path = append(path, point{int(startX), int(startZ)})
	for range min((len(payload)-5-trailingBytes)/2, 24) {
		dx := r.G1B()
		dz := r.G1B()
		path = append(path, point{int(startX) + int(dx), int(startZ) + int(dz)})
	}

	c.log.Info("minimap click", "ctrl_held", ctrlHeld, "path", path)
	return nil
}
```

- [ ] **Step 4: Run all tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... -v 2>&1 | grep -E "^(ok|FAIL|---)"
```

Expected: all `ok`, no `FAIL`.

- [ ] **Step 5: Commit**

```bash
git add modules/world/handlers_game.go modules/world/server_test.go
git commit --no-gpg-sign -m "feat: register keepalive and movement game packet handlers"
```
