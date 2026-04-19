# Tick Infrastructure Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement the 600ms world tick loop, thread-safe player registry, ISAAC-rate-limited client input pipeline, and skeleton output pipeline for the goscape RS2 game server.

**Architecture:** A `sync.Mutex` (`client.inMu`) separates the per-connection reader goroutine (which only appends raw bytes) from the single tick goroutine (which drains and dispatches packets at 600ms with per-category rate limits). `Server` holds a `[2048]*Player` registry; `sendLoginOK` registers, deferred connection cleanup deregisters.

**Tech Stack:** Go standard library — `net`, `sync`, `math/rand/v2`, `time`. No external dependencies added.

> All `go` commands must use the prefix: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache`

---

## File Map

| File | Action | Responsibility |
|------|--------|----------------|
| `pkg/io/protocol/game/client/prot.go` | Modify | Add `Category int` to `Op`, add category constants, populate all 50 opcodes |
| `pkg/io/protocol/game/client/prot_test.go` | Create | Tests for `Category` assignments |
| `pkg/io/protocol/game/server/prot.go` | **Create** | `Op` type + 6 modal opcodes + `LOGOUT` |
| `pkg/io/protocol/game/server/prot_test.go` | Create | Tests for server opcode values |
| `modules/world/client.go` | Modify | Add `inMu sync.Mutex`, `player *Player` to `client` struct |
| `modules/world/player.go` | **Create** | `Player` struct, `newPlayer`, `readPacket`, `processIn`, `writeOut`, `encodeOut`, `processOut`, 7 stub update methods |
| `modules/world/player_test.go` | **Create** | Tests for `readPacket`, `processIn` rate limits, `writeOut`, `encodeOut` |
| `modules/world/handlers_game.go` | Modify | Change handler type from `func(*client, …)` to `func(*Player, …)` |
| `modules/world/client_game.go` | **Delete** | `handleGame()` logic moves to `Player.readPacket()` |
| `modules/world/server.go` | Modify | Add player registry, `addPlayer`/`removePlayer`, update `handleTCPConn` and `sendLoginOK`, start tick loop in `Run()` |
| `modules/world/tick.go` | **Create** | `runTickLoop`, `processClientsIn`, `processClientsOut` |
| `modules/world/server_test.go` | Modify | Remove stale `TestHandleGameXxx` tests; add registry tests |

---

## Task 1: Add `Category` to `Op` and populate all 50 opcodes

**Files:**
- Modify: `pkg/io/protocol/game/client/prot.go`
- Create: `pkg/io/protocol/game/client/prot_test.go`

- [ ] **Step 1: Write the failing test**

Create `pkg/io/protocol/game/client/prot_test.go`:

```go
package client

import "testing"

func TestOpCategories(t *testing.T) {
	cases := []struct {
		opcode   int
		name     string
		category int
	}{
		// CLIENT_EVENT
		{108, "NO_TIMEOUT", CategoryClientEvent},
		{70, "IDLE_TIMER", CategoryClientEvent},
		{189, "EVENT_CAMERA_POSITION", CategoryClientEvent},
		{7, "ANTICHEAT_OPLOGIC1", CategoryClientEvent},
		{233, "ANTICHEAT_CYCLELOGIC1", CategoryClientEvent},
		// RESTRICTED_EVENT
		{81, "EVENT_TRACKING", CategoryRestrictedEvent},
		{150, "REBUILD_GETMAPS", CategoryRestrictedEvent},
		// USER_EVENT
		{181, "MOVE_GAMECLICK", CategoryUserEvent},
		{93, "MOVE_OPCLICK", CategoryUserEvent},
		{165, "MOVE_MINIMAPCLICK", CategoryUserEvent},
		{4, "CLIENT_CHEAT", CategoryUserEvent},
		{140, "OPOBJ1", CategoryUserEvent},
		{194, "OPNPC1", CategoryUserEvent},
		{245, "OPLOC1", CategoryUserEvent},
		{164, "OPPLAYER1", CategoryUserEvent},
		{195, "OPHELD1", CategoryUserEvent},
		{31, "INV_BUTTON1", CategoryUserEvent},
		{155, "IF_BUTTON", CategoryUserEvent},
		{231, "CLOSE_MODAL", CategoryUserEvent},
		{158, "MESSAGE_PUBLIC", CategoryUserEvent},
	}
	for _, tc := range cases {
		op := Ops[tc.opcode]
		if op.Name != tc.name {
			t.Errorf("Ops[%d].Name = %q, want %q", tc.opcode, op.Name, tc.name)
		}
		if op.Category != tc.category {
			t.Errorf("Ops[%d] (%s): Category = %d, want %d", tc.opcode, tc.name, op.Category, tc.category)
		}
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/io/protocol/game/client/... -run TestOpCategories -v
```

Expected: compile error — `CategoryClientEvent`, `CategoryRestrictedEvent`, `CategoryUserEvent` undefined; `Op` has no field `Category`.

- [ ] **Step 3: Add `Category` field, constants, and update `init()` in `prot.go`**

Replace the entire `prot.go` with:

```go
package client

// Op describes a client game packet opcode.
type Op struct {
	Name        string
	PayloadSize int // 0=fixed-zero, N=fixed-N, -1=1-byte-len, -2=2-byte-len
	Category    int // CategoryClientEvent | CategoryUserEvent | CategoryRestrictedEvent
}

const (
	CategoryClientEvent     = 0 // limit 20/tick
	CategoryUserEvent       = 1 // limit 5/tick
	CategoryRestrictedEvent = 2 // limit 2/tick
)

// Ops is a 256-entry lookup table indexed by decrypted game opcode.
// A zero-value Op (empty Name) means the opcode is unknown.
var Ops [256]Op

func init() {
	u := CategoryUserEvent
	c := CategoryClientEvent
	r := CategoryRestrictedEvent

	set := func(opcode uint8, name string, payloadSize int, category int) {
		Ops[opcode] = Op{Name: name, PayloadSize: payloadSize, Category: category}
	}

	set(150, "REBUILD_GETMAPS", -1, r)
	set(108, "NO_TIMEOUT", 0, c)
	set(70, "IDLE_TIMER", 0, c)
	set(81, "EVENT_TRACKING", -2, r)
	set(189, "EVENT_CAMERA_POSITION", 6, c)

	set(7, "ANTICHEAT_OPLOGIC1", 4, c)
	set(88, "ANTICHEAT_OPLOGIC2", 4, c)
	set(30, "ANTICHEAT_OPLOGIC3", 3, c)
	set(176, "ANTICHEAT_OPLOGIC4", 2, c)
	set(220, "ANTICHEAT_OPLOGIC5", 0, c)
	set(66, "ANTICHEAT_OPLOGIC6", 4, c)
	set(17, "ANTICHEAT_OPLOGIC7", 4, c)
	set(2, "ANTICHEAT_OPLOGIC8", 2, c)
	set(238, "ANTICHEAT_OPLOGIC9", 1, c)

	set(233, "ANTICHEAT_CYCLELOGIC1", 1, c)
	set(146, "ANTICHEAT_CYCLELOGIC2", -1, c)
	set(215, "ANTICHEAT_CYCLELOGIC3", 3, c)
	set(236, "ANTICHEAT_CYCLELOGIC4", 4, c)
	set(85, "ANTICHEAT_CYCLELOGIC5", 0, c)
	set(219, "ANTICHEAT_CYCLELOGIC6", -1, c)

	set(140, "OPOBJ1", 6, u)
	set(40, "OPOBJ2", 6, u)
	set(200, "OPOBJ3", 6, u)
	set(178, "OPOBJ4", 6, u)
	set(247, "OPOBJ5", 6, u)
	set(138, "OPOBJT", 8, u)
	set(239, "OPOBJU", 12, u)

	set(194, "OPNPC1", 2, u)
	set(8, "OPNPC2", 2, u)
	set(27, "OPNPC3", 2, u)
	set(113, "OPNPC4", 2, u)
	set(100, "OPNPC5", 2, u)
	set(134, "OPNPCT", 4, u)
	set(202, "OPNPCU", 8, u)

	set(245, "OPLOC1", 6, u)
	set(172, "OPLOC2", 6, u)
	set(96, "OPLOC3", 6, u)
	set(97, "OPLOC4", 6, u)
	set(116, "OPLOC5", 6, u)
	set(9, "OPLOCT", 8, u)
	set(75, "OPLOCU", 12, u)

	set(164, "OPPLAYER1", 2, u)
	set(53, "OPPLAYER2", 2, u)
	set(185, "OPPLAYER3", 2, u)
	set(206, "OPPLAYER4", 2, u)
	set(177, "OPPLAYERT", 4, u)
	set(248, "OPPLAYERU", 8, u)

	set(195, "OPHELD1", 6, u)
	set(71, "OPHELD2", 6, u)
	set(133, "OPHELD3", 6, u)
	set(157, "OPHELD4", 6, u)
	set(211, "OPHELD5", 6, u)
	set(48, "OPHELDT", 8, u)
	set(130, "OPHELDU", 12, u)

	set(31, "INV_BUTTON1", 6, u)
	set(59, "INV_BUTTON2", 6, u)
	set(212, "INV_BUTTON3", 6, u)
	set(38, "INV_BUTTON4", 6, u)
	set(6, "INV_BUTTON5", 6, u)

	set(155, "IF_BUTTON", 2, u)
	set(235, "RESUME_PAUSEBUTTON", 2, u)
	set(231, "CLOSE_MODAL", 0, u)
	set(237, "RESUME_P_COUNTDIALOG", 4, u)
	set(175, "TUT_CLICKSIDE", 1, u)

	set(93, "MOVE_OPCLICK", -1, u)
	set(190, "REPORT_ABUSE", 10, u)
	set(165, "MOVE_MINIMAPCLICK", -1, u)
	set(159, "INV_BUTTOND", 6, u)
	set(171, "IGNORELIST_DEL", 8, u)
	set(79, "IGNORELIST_ADD", 8, u)
	set(52, "IDK_SAVEDESIGN", 13, u)
	set(244, "CHAT_SETMODE", 3, u)
	set(148, "MESSAGE_PRIVATE", -1, u)
	set(11, "FRIENDLIST_DEL", 8, u)
	set(118, "FRIENDLIST_ADD", 8, u)
	set(4, "CLIENT_CHEAT", -1, u)
	set(158, "MESSAGE_PUBLIC", -1, u)
	set(181, "MOVE_GAMECLICK", -1, u)
}
```

- [ ] **Step 4: Run the test to verify it passes**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/io/protocol/game/client/... -v
```

Expected:
```
ok  	github.com/zsrv/goscape/pkg/io/protocol/game/client	0.001s
```

- [ ] **Step 5: Commit**

```bash
git add pkg/io/protocol/game/client/prot.go pkg/io/protocol/game/client/prot_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(prot): add Category field to Op and populate all 50 client opcodes

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: Create `ServerOp` type and sub-spec 1 server opcodes

**Files:**
- Create: `pkg/io/protocol/game/server/prot.go`
- Create: `pkg/io/protocol/game/server/prot_test.go`

- [ ] **Step 1: Write the failing test**

Create `pkg/io/protocol/game/server/prot_test.go`:

```go
package server

import "testing"

func TestServerOpValues(t *testing.T) {
	cases := []struct {
		op     Op
		opcode byte
		size   int
	}{
		{OpIfClose, 129, 0},
		{OpIfOpenMain, 168, 2},
		{OpIfOpenChat, 14, 2},
		{OpIfOpenSide, 195, 2},
		{OpIfOpenMainSide, 28, 4},
		{OpLogout, 142, 0},
	}
	for _, tc := range cases {
		if tc.op.Opcode != tc.opcode {
			t.Errorf("%v: Opcode = %d, want %d", tc.op, tc.op.Opcode, tc.opcode)
		}
		if tc.op.PayloadSize != tc.size {
			t.Errorf("%v: PayloadSize = %d, want %d", tc.op, tc.op.PayloadSize, tc.size)
		}
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/io/protocol/game/server/... -run TestServerOpValues -v
```

Expected: compile error — package `server` does not exist.

- [ ] **Step 3: Create `pkg/io/protocol/game/server/prot.go`**

```go
package server

// Op describes a server→client game packet opcode.
type Op struct {
	Opcode      byte
	PayloadSize int // 0=fixed-zero, 2=fixed-2, 4=fixed-4, -1=1-byte-len, -2=2-byte-len
}

// Modal interface opcodes and logout — sub-spec 1 only.
// Remaining ~40 server opcodes added in sub-specs 2–4.
var (
	OpIfClose       = Op{Opcode: 129, PayloadSize: 0}
	OpIfOpenMain    = Op{Opcode: 168, PayloadSize: 2}
	OpIfOpenChat    = Op{Opcode: 14, PayloadSize: 2}
	OpIfOpenSide    = Op{Opcode: 195, PayloadSize: 2}
	OpIfOpenMainSide = Op{Opcode: 28, PayloadSize: 4}
	OpLogout        = Op{Opcode: 142, PayloadSize: 0}
)
```

- [ ] **Step 4: Run the test to verify it passes**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/io/protocol/game/server/... -v
```

Expected:
```
ok  	github.com/zsrv/goscape/pkg/io/protocol/game/server	0.001s
```

- [ ] **Step 5: Commit**

```bash
git add pkg/io/protocol/game/server/prot.go pkg/io/protocol/game/server/prot_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(prot): add ServerOp type with sub-spec 1 modal and logout opcodes

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: Add `inMu`/`player` to `client`; create `Player` struct

**Files:**
- Modify: `modules/world/client.go`
- Create: `modules/world/player.go`

- [ ] **Step 1: Add `inMu sync.Mutex` and `player *Player` to the `client` struct in `client.go`**

In `client.go`, update the `client` struct (around line 35):

```go
type client struct {
	conn          net.Conn
	log           *slog.Logger
	bufr          *bufio.Reader
	bufw          *bufio.Writer
	in            *packet.Packet
	inMu          sync.Mutex  // guards in, opcode, waiting between reader goroutine and tick goroutine
	player        *Player     // nil until sendLoginOK; set by tick goroutine exclusively after login
	encryptor     *io2.Isaac
	decryptor     *io2.Isaac
	server        *Server
	writeTimeout  time.Duration
	state         ClientState
	opcode        int
	waiting       int
	staffModLevel int32
	members       bool
}
```

(`sync` is already imported via the pool declarations at the bottom of client.go.)

- [ ] **Step 2: Create `modules/world/player.go`**

```go
package world

const (
	userEventLimit       = 5
	clientEventLimit     = 20
	restrictedEventLimit = 2
	afkEventRate         = 500

	modalStateNone = 0x0
	modalStateMain = 0x1
	modalStateChat = 0x2
	modalStateSide = 0x4
)

// Player is the game-side representation of a connected player.
// All fields except client and slot are owned exclusively by the tick goroutine.
type Player struct {
	slot   int     // RS2 player slot 1–2047; assigned by addPlayer
	client *client // network handle; never nil while the player is registered

	// per-tick tracking
	playtime      int
	afkEventReady bool
	lastConnected int
	lastResponse  int

	// per-tick rate-limit counters (reset at start of each processIn call)
	userLimit       int
	clientLimit     int
	restrictedLimit int

	// modal state — drives encodeOut
	modalMain         int
	modalChat         int
	modalSide         int
	lastModalMain     int
	lastModalChat     int
	lastModalSide     int
	modalState        int
	refreshModal      bool
	refreshModalClose bool
}

func newPlayer(c *client) *Player {
	return &Player{client: c}
}
```

- [ ] **Step 3: Verify it compiles**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./modules/world/...
```

Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add modules/world/client.go modules/world/player.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): add Player struct and inMu/player fields to client

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: Implement `Player.readPacket`; migrate `handleGame` tests; delete `client_game.go`

This task moves the inner loop of `handleGame()` into `Player.readPacket()`, changes the handler dispatch signature from `*client` to `*Player`, and removes the now-superseded `handleGame()`.

**Files:**
- Create: `modules/world/player_test.go`
- Modify: `modules/world/player.go`
- Modify: `modules/world/handlers_game.go`
- Delete: `modules/world/client_game.go`
- Modify: `modules/world/server.go` (update `handleData` game-state branch)
- Modify: `modules/world/server_test.go` (remove stale `TestHandleGameXxx` tests)

- [ ] **Step 1: Create `modules/world/player_test.go` with `newTestPlayer` and `readPacket` tests**

```go
package world

import (
	"errors"
	"io"
	"net"
	"testing"
	"time"
)

func newTestPlayer(t *testing.T) (*Player, net.Conn) {
	t.Helper()
	serverConn, clientConn := net.Pipe()
	t.Cleanup(func() {
		serverConn.Close()
		clientConn.Close()
	})
	c := newClient(serverConn, time.Second, discardLogger())
	t.Cleanup(func() { c.in.Release() })
	c.state = ClientStateGame
	p := newPlayer(c)
	c.player = p
	return p, clientConn
}

func TestReadPacketEmptyBufferReturnsFalse(t *testing.T) {
	_, dec := isaacPair([4]uint32{1, 2, 3, 4})
	p, _ := newTestPlayer(t)
	p.client.decryptor = dec

	opcode, ok, err := p.readPacket()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Error("expected ok=false for empty buffer")
	}
	if opcode != -1 {
		t.Errorf("opcode: got %d, want -1", opcode)
	}
}

func TestReadPacketUnknownOpcodeReturnsErrCloseConn(t *testing.T) {
	enc, dec := isaacPair([4]uint32{5, 6, 7, 8})
	p, _ := newTestPlayer(t)
	p.client.decryptor = dec

	// Opcode 0 is not registered in Ops.
	p.client.in.Write([]byte{encryptOpcode(enc, 0)})

	_, _, err := p.readPacket()
	if !errors.Is(err, errCloseConn) {
		t.Errorf("unknown opcode: got %v, want errCloseConn", err)
	}
}

func TestReadPacketNoTimeoutConsumesAndResetsOpcode(t *testing.T) {
	enc, dec := isaacPair([4]uint32{10, 20, 30, 40})
	p, _ := newTestPlayer(t)
	p.client.decryptor = dec

	// NO_TIMEOUT: opcode 108, payload size 0
	p.client.in.Write([]byte{encryptOpcode(enc, 108)})

	opcode, ok, err := p.readPacket()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Error("expected ok=true")
	}
	if opcode != 108 {
		t.Errorf("opcode: got %d, want 108", opcode)
	}
	if p.client.opcode != -1 {
		t.Errorf("client.opcode after dispatch: got %d, want -1", p.client.opcode)
	}
}

func TestReadPacketMoveGameClickFullPacket(t *testing.T) {
	enc, dec := isaacPair([4]uint32{11, 22, 33, 44})
	p, _ := newTestPlayer(t)
	p.client.decryptor = dec

	// MOVE_GAMECLICK: opcode 181, 1-byte length prefix
	// Payload: ctrlHeld(1) + startX G2(2) + startZ G2(2) = 5 bytes
	payload := []byte{0, 0x0C, 0xA4, 0x0C, 0x8B}
	var buf []byte
	buf = append(buf, encryptOpcode(enc, 181))
	buf = append(buf, byte(len(payload)))
	buf = append(buf, payload...)
	p.client.in.Write(buf)

	opcode, ok, err := p.readPacket()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Error("expected ok=true")
	}
	if opcode != 181 {
		t.Errorf("opcode: got %d, want 181", opcode)
	}
	if p.client.opcode != -1 {
		t.Errorf("client.opcode after dispatch: got %d, want -1", p.client.opcode)
	}
}

func TestReadPacketPartialPayloadReturnsFalse(t *testing.T) {
	enc, dec := isaacPair([4]uint32{55, 66, 77, 88})
	p, _ := newTestPlayer(t)
	p.client.decryptor = dec

	// MOVE_GAMECLICK claiming 10 payload bytes, only 3 arrive
	buf := []byte{encryptOpcode(enc, 181), 10, 0x01, 0x02, 0x03}
	p.client.in.Write(buf)

	_, ok, err := p.readPacket()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Error("expected ok=false for partial payload")
	}
	// cursor must be preserved
	if p.client.opcode != 181 {
		t.Errorf("client.opcode preserved: got %d, want 181", p.client.opcode)
	}
	if p.client.waiting != 10 {
		t.Errorf("client.waiting preserved: got %d, want 10", p.client.waiting)
	}
}

func TestReadPacketEventTrackingTwoByteLenPrefix(t *testing.T) {
	enc, dec := isaacPair([4]uint32{99, 88, 77, 66})
	p, _ := newTestPlayer(t)
	p.client.decryptor = dec

	// EVENT_TRACKING: opcode 81, -2 (2-byte length prefix)
	payload := []byte{0x01, 0x02, 0x03, 0x04}
	var buf []byte
	buf = append(buf, encryptOpcode(enc, 81))
	buf = append(buf, 0x00, byte(len(payload))) // 2-byte big-endian length
	buf = append(buf, payload...)
	p.client.in.Write(buf)

	opcode, ok, err := p.readPacket()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Error("expected ok=true")
	}
	if opcode != 81 {
		t.Errorf("opcode: got %d, want 81", opcode)
	}
}

func TestReadPacketOversizedTwoByteLenClosesConn(t *testing.T) {
	enc, dec := isaacPair([4]uint32{1, 1, 1, 1})
	p, _ := newTestPlayer(t)
	p.client.decryptor = dec

	// EVENT_TRACKING with 2-byte length > 1600
	var buf []byte
	buf = append(buf, encryptOpcode(enc, 81))
	buf = append(buf, 0x07, 0x00) // 0x0700 = 1792 > 1600
	p.client.in.Write(buf)

	_, _, err := p.readPacket()
	if !errors.Is(err, errCloseConn) {
		t.Errorf("oversized packet: got %v, want errCloseConn", err)
	}
}

```

- [ ] **Step 2: Run the test to verify it fails**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestReadPacket -v
```

Expected: compile error — `p.readPacket` undefined.

- [ ] **Step 3: Change handler type in `handlers_game.go` from `*client` to `*Player`**

Replace the entire `handlers_game.go`:

```go
package world

import (
	"github.com/zsrv/goscape/pkg/io/packet"
)

// gameHandlers is indexed by decrypted game opcode. Nil means no handler
// registered for that opcode; readPacket() silently discards such packets
// (they still must be in Ops[] to be accepted at all).
var gameHandlers [256]func(*Player, []byte) error

func init() {
	gameHandlers[108] = handleNoTimeout  // NO_TIMEOUT
	gameHandlers[70] = handleNoTimeout   // IDLE_TIMER

	gameHandlers[181] = handleMoveClick        // MOVE_GAMECLICK
	gameHandlers[93] = handleMoveClick         // MOVE_OPCLICK
	gameHandlers[165] = handleMoveMinimapClick // MOVE_MINIMAPCLICK
}

func handleNoTimeout(_ *Player, _ []byte) error {
	return nil
}

func handleMoveClick(p *Player, payload []byte) error {
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

	p.client.log.Info("move click", "ctrl_held", ctrlHeld, "path", path)
	return nil
}

func handleMoveMinimapClick(p *Player, payload []byte) error {
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

	p.client.log.Info("minimap click", "ctrl_held", ctrlHeld, "path", path)
	return nil
}
```

- [ ] **Step 4: Add `readPacket` to `player.go`**

Add these imports and the method to `modules/world/player.go`:

```go
import (
	gameclient "github.com/zsrv/goscape/pkg/io/protocol/game/client"
)
```

```go
// readPacket reads, ISAAC-decrypts, and dispatches one complete packet from c.in.
// Returns (opcode, true, nil) on success, (-1, false, nil) if the buffer is empty
// or the payload is incomplete, and (-1, false, errCloseConn) on a fatal error.
// Must be called with c.inMu held.
func (p *Player) readPacket() (int, bool, error) {
	c := p.client

	if c.opcode == -1 {
		raw, err := c.in.Peek(1)
		if err != nil {
			return -1, false, nil
		}
		decrypted := (int(raw[0]) - int(c.decryptor.GetNext())) & 0xff
		op := gameclient.Ops[decrypted]
		if op.Name == "" {
			c.log.Warn("unknown game opcode", "opcode", decrypted)
			c.conn.Close()
			return -1, false, errCloseConn
		}
		c.in.Next(1)
		c.opcode = decrypted
		c.waiting = op.PayloadSize
	}

	if c.waiting == -1 {
		if c.in.Len() < 1 {
			return -1, false, nil
		}
		c.waiting = int(c.in.Next(1)[0])
	} else if c.waiting == -2 {
		if c.in.Len() < 2 {
			return -1, false, nil
		}
		b := c.in.Next(2)
		c.waiting = int(uint16(b[0])<<8 | uint16(b[1]))
		if c.waiting > 1600 {
			c.log.Warn("oversized game packet, closing", "opcode", c.opcode, "size", c.waiting)
			c.conn.Close()
			return -1, false, errCloseConn
		}
	}

	if c.in.Len() < c.waiting {
		return -1, false, nil
	}

	payload := c.in.Next(c.waiting)
	opcode := c.opcode
	c.opcode = -1

	c.log.Debug("game packet", "opcode", opcode, "name", gameclient.Ops[opcode].Name, "len", len(payload))

	if handler := gameHandlers[opcode]; handler != nil {
		if err := handler(p, payload); err != nil {
			return -1, false, err
		}
	}

	return opcode, true, nil
}
```

- [ ] **Step 5: Run the `readPacket` tests to verify they pass**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestReadPacket -v
```

Expected: all `TestReadPacket*` pass.

- [ ] **Step 6: Delete `client_game.go`**

```bash
rm modules/world/client_game.go
```

- [ ] **Step 7: Update `handleData()` in `server.go` to return nil for game state**

In `server.go`, find the `handleData()` function (around line 247) and replace:

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

(Remove the `ClientStateGame` case — the tick loop owns game state dispatch now.)

- [ ] **Step 8: Remove stale `TestHandleGameXxx` tests from `server_test.go`**

Delete the following test functions from `modules/world/server_test.go` (they are now covered by `player_test.go`):
- `TestHandleGameEmptyBufferReturnsErrPayloadTooSmall`
- `TestHandleGameUnknownOpcodeReturnsErrCloseConn`
- `TestHandleGameNoTimeoutCompletesAndResetsOpcode`
- `TestHandleGameMoveGameClickFullPacket`
- `TestHandleGamePartialPayloadPreservesOpcode`

Also remove the helper functions that are no longer used in `server_test.go` if they are now only used in `player_test.go`. **Keep** `isaacPair`, `encryptOpcode`, and `discardLogger` — they are shared across both test files in the same package.

- [ ] **Step 9: Run all world tests to verify no regressions**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -v
```

Expected: all tests pass, no compile errors.

- [ ] **Step 10: Commit**

```bash
git add modules/world/player.go modules/world/player_test.go modules/world/handlers_game.go modules/world/server.go modules/world/server_test.go
git rm modules/world/client_game.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): implement Player.readPacket; migrate handleGame tests; remove client_game.go

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: Implement `Player.processIn` with rate limits

**Files:**
- Modify: `modules/world/player.go`
- Modify: `modules/world/player_test.go`

- [ ] **Step 1: Write the failing tests**

Add to `modules/world/player_test.go`:

```go
func TestProcessInIncrementsPlaytime(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.playtime = 0
	p.processIn(0)
	if p.playtime != 1 {
		t.Errorf("playtime: got %d, want 1", p.playtime)
	}
}

func TestProcessInUserEventRateLimit(t *testing.T) {
	enc, dec := isaacPair([4]uint32{10, 20, 30, 40})
	p, _ := newTestPlayer(t)
	p.client.decryptor = dec

	// CLOSE_MODAL: opcode 231, USER_EVENT, 0-byte payload — just the opcode byte
	var buf []byte
	for range 6 {
		buf = append(buf, encryptOpcode(enc, 231))
	}
	p.client.in.Write(buf)

	p.processIn(0)

	if p.userLimit != 5 {
		t.Errorf("userLimit: got %d, want 5", p.userLimit)
	}
	// 6th packet remains in the buffer
	if p.client.in.Len() != 1 {
		t.Errorf("remaining bytes: got %d, want 1", p.client.in.Len())
	}
}

func TestProcessInClientEventRateLimit(t *testing.T) {
	enc, dec := isaacPair([4]uint32{11, 22, 33, 44})
	p, _ := newTestPlayer(t)
	p.client.decryptor = dec

	// NO_TIMEOUT: opcode 108, CLIENT_EVENT, 0-byte payload
	var buf []byte
	for range 21 {
		buf = append(buf, encryptOpcode(enc, 108))
	}
	p.client.in.Write(buf)

	p.processIn(0)

	if p.clientLimit != 20 {
		t.Errorf("clientLimit: got %d, want 20", p.clientLimit)
	}
	if p.client.in.Len() != 1 {
		t.Errorf("remaining bytes: got %d, want 1", p.client.in.Len())
	}
}

func TestProcessInRestrictedEventRateLimit(t *testing.T) {
	enc, dec := isaacPair([4]uint32{55, 44, 33, 22})
	p, _ := newTestPlayer(t)
	p.client.decryptor = dec

	// EVENT_TRACKING: opcode 81, RESTRICTED_EVENT, -2 (2-byte length prefix), 0 payload bytes
	// Wire format: opcode_byte + 0x00 + 0x00 (length=0)
	var buf []byte
	for range 3 {
		buf = append(buf, encryptOpcode(enc, 81))
		buf = append(buf, 0x00, 0x00) // 2-byte length = 0
	}
	p.client.in.Write(buf)

	p.processIn(0)

	if p.restrictedLimit != 2 {
		t.Errorf("restrictedLimit: got %d, want 2", p.restrictedLimit)
	}
	// 3rd packet (3 bytes) remains
	if p.client.in.Len() != 3 {
		t.Errorf("remaining bytes: got %d, want 3", p.client.in.Len())
	}
}

func TestProcessInSkipsDisconnectedClient(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.client.state = ClientStateClosed

	p.processIn(0)

	// playtime still increments even for disconnected clients
	if p.playtime != 1 {
		t.Errorf("playtime: got %d, want 1", p.playtime)
	}
	// no packet processing
	if p.userLimit != 0 {
		t.Errorf("userLimit: got %d, want 0", p.userLimit)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestProcessIn -v
```

Expected: compile error — `p.processIn` undefined.

- [ ] **Step 3: Add `processIn` to `player.go`**

Add this import to `player.go`:

```go
import (
	"math/rand/v2"

	gameclient "github.com/zsrv/goscape/pkg/io/protocol/game/client"
)
```

Add this method:

```go
func (p *Player) processIn(currentTick int) {
	p.playtime++

	if currentTick%afkEventRate == 0 {
		p.afkEventReady = rand.Float64() < 0.0167 // AFK_CHANCE1 from TS
	}

	c := p.client
	if c.state != ClientStateGame {
		return
	}

	p.userLimit = 0
	p.clientLimit = 0
	p.restrictedLimit = 0

	c.inMu.Lock()
	defer c.inMu.Unlock()

	for p.userLimit < userEventLimit &&
		p.clientLimit < clientEventLimit &&
		p.restrictedLimit < restrictedEventLimit {

		opcode, ok, err := p.readPacket()
		if err != nil {
			return
		}
		if !ok {
			break
		}
		switch gameclient.Ops[opcode].Category {
		case gameclient.CategoryUserEvent:
			p.userLimit++
		case gameclient.CategoryRestrictedEvent:
			p.restrictedLimit++
		default:
			p.clientLimit++
		}
	}
}
```

- [ ] **Step 4: Run the tests to verify they pass**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestProcessIn -v
```

Expected: all `TestProcessIn*` pass.

- [ ] **Step 5: Commit**

```bash
git add modules/world/player.go modules/world/player_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): implement Player.processIn with TS-faithful per-category rate limits

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: Implement `Player.writeOut`

**Files:**
- Modify: `modules/world/player.go`
- Modify: `modules/world/player_test.go`

- [ ] **Step 1: Write the failing tests**

Add to `modules/world/player_test.go`:

```go
import (
	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
)
```

```go
func TestWriteOutFixedSize(t *testing.T) {
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	wantEnc, _ := isaacPair([4]uint32{1, 2, 3, 4}) // mirror to derive expected encrypted byte

	p, clientConn := newTestPlayer(t)
	p.client.encryptor = enc

	op := gameserver.Op{Opcode: 42, PayloadSize: 2}
	payload := []byte{0xAB, 0xCD}

	received := make(chan []byte, 1)
	go func() {
		buf := make([]byte, 3)
		clientConn.SetReadDeadline(time.Now().Add(time.Second))
		if _, err := io.ReadFull(clientConn, buf); err == nil {
			received <- buf
		}
	}()

	p.writeOut(op, payload)
	p.client.flushWrite()

	expectedOpByte := byte((int(op.Opcode) + int(wantEnc.GetNext())) & 0xff)

	select {
	case got := <-received:
		if got[0] != expectedOpByte {
			t.Errorf("encrypted opcode: got %d, want %d", got[0], expectedOpByte)
		}
		if got[1] != 0xAB || got[2] != 0xCD {
			t.Errorf("payload: got [%d %d], want [0xAB 0xCD]", got[1], got[2])
		}
	case <-time.After(time.Second):
		t.Error("timed out waiting for writeOut bytes")
	}
}

func TestWriteOutOneByteLenPrefix(t *testing.T) {
	enc, _ := isaacPair([4]uint32{2, 3, 4, 5})
	wantEnc, _ := isaacPair([4]uint32{2, 3, 4, 5})

	p, clientConn := newTestPlayer(t)
	p.client.encryptor = enc

	op := gameserver.Op{Opcode: 50, PayloadSize: -1}
	payload := []byte{0x01, 0x02, 0x03}

	received := make(chan []byte, 1)
	go func() {
		// 1 opcode + 1 len-prefix + 3 payload = 5 bytes
		buf := make([]byte, 5)
		clientConn.SetReadDeadline(time.Now().Add(time.Second))
		if _, err := io.ReadFull(clientConn, buf); err == nil {
			received <- buf
		}
	}()

	p.writeOut(op, payload)
	p.client.flushWrite()

	expectedOpByte := byte((int(op.Opcode) + int(wantEnc.GetNext())) & 0xff)

	select {
	case got := <-received:
		if got[0] != expectedOpByte {
			t.Errorf("encrypted opcode: got %d, want %d", got[0], expectedOpByte)
		}
		if got[1] != byte(len(payload)) {
			t.Errorf("length prefix: got %d, want %d", got[1], len(payload))
		}
		if got[2] != 0x01 || got[3] != 0x02 || got[4] != 0x03 {
			t.Errorf("payload: got %v, want [1 2 3]", got[2:])
		}
	case <-time.After(time.Second):
		t.Error("timed out")
	}
}

func TestWriteOutTwoByteLenPrefix(t *testing.T) {
	enc, _ := isaacPair([4]uint32{3, 4, 5, 6})
	wantEnc, _ := isaacPair([4]uint32{3, 4, 5, 6})

	p, clientConn := newTestPlayer(t)
	p.client.encryptor = enc

	op := gameserver.Op{Opcode: 60, PayloadSize: -2}
	payload := []byte{0xDE, 0xAD}

	received := make(chan []byte, 1)
	go func() {
		// 1 opcode + 2 len-prefix + 2 payload = 5 bytes
		buf := make([]byte, 5)
		clientConn.SetReadDeadline(time.Now().Add(time.Second))
		if _, err := io.ReadFull(clientConn, buf); err == nil {
			received <- buf
		}
	}()

	p.writeOut(op, payload)
	p.client.flushWrite()

	expectedOpByte := byte((int(op.Opcode) + int(wantEnc.GetNext())) & 0xff)

	select {
	case got := <-received:
		if got[0] != expectedOpByte {
			t.Errorf("encrypted opcode: got %d, want %d", got[0], expectedOpByte)
		}
		// big-endian 2-byte length = 2
		if got[1] != 0x00 || got[2] != 0x02 {
			t.Errorf("length prefix: got [%d %d], want [0 2]", got[1], got[2])
		}
		if got[3] != 0xDE || got[4] != 0xAD {
			t.Errorf("payload: got [%d %d], want [0xDE 0xAD]", got[3], got[4])
		}
	case <-time.After(time.Second):
		t.Error("timed out")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestWriteOut -v
```

Expected: compile error — `p.writeOut` undefined.

- [ ] **Step 3: Add `writeOut` to `player.go`**

Add import:

```go
import (
	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
)
```

Add method:

```go
// writeOut ISAAC-encrypts op.Opcode, writes any length prefix, then writes
// payload to c.bufw. Does NOT flush — processOut calls flushWrite() once per tick.
func (p *Player) writeOut(op gameserver.Op, payload []byte) {
	c := p.client
	encrypted := byte((int(op.Opcode) + int(c.encryptor.GetNext())) & 0xff)
	c.bufw.WriteByte(encrypted)

	switch op.PayloadSize {
	case -1:
		c.bufw.WriteByte(byte(len(payload)))
	case -2:
		n := len(payload)
		c.bufw.WriteByte(byte(n >> 8))
		c.bufw.WriteByte(byte(n))
	}

	c.bufw.Write(payload)
}
```

- [ ] **Step 4: Run the tests to verify they pass**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestWriteOut -v
```

Expected: all `TestWriteOut*` pass.

- [ ] **Step 5: Commit**

```bash
git add modules/world/player.go modules/world/player_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): implement Player.writeOut with ISAAC encryption and length prefix

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 7: Implement `Player.encodeOut` with modal state

**Files:**
- Modify: `modules/world/player.go`
- Modify: `modules/world/player_test.go`

- [ ] **Step 1: Write the failing tests**

Add to `modules/world/player_test.go`:

```go
func TestEncodeOutNoopWhenModalUnchanged(t *testing.T) {
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	p, clientConn := newTestPlayer(t)
	p.client.encryptor = enc

	p.encodeOut()
	p.client.flushWrite()

	// No bytes should be written — use a short deadline to confirm
	clientConn.SetReadDeadline(time.Now().Add(30 * time.Millisecond))
	buf := make([]byte, 16)
	n, _ := clientConn.Read(buf)
	if n != 0 {
		t.Errorf("expected 0 bytes from encodeOut no-op, got %d", n)
	}
}

func TestEncodeOutSendsIfCloseOnRefreshModalClose(t *testing.T) {
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	wantEnc, _ := isaacPair([4]uint32{1, 2, 3, 4})

	p, clientConn := newTestPlayer(t)
	p.client.encryptor = enc
	p.refreshModalClose = true

	received := make(chan []byte, 1)
	go func() {
		buf := make([]byte, 1) // IF_CLOSE has no payload
		clientConn.SetReadDeadline(time.Now().Add(time.Second))
		if _, err := io.ReadFull(clientConn, buf); err == nil {
			received <- buf
		}
	}()

	p.encodeOut()
	p.client.flushWrite()

	expectedByte := byte((int(gameserver.OpIfClose.Opcode) + int(wantEnc.GetNext())) & 0xff)

	select {
	case got := <-received:
		if got[0] != expectedByte {
			t.Errorf("IF_CLOSE encrypted opcode: got %d, want %d", got[0], expectedByte)
		}
		if p.refreshModalClose {
			t.Error("refreshModalClose should be false after encodeOut")
		}
	case <-time.After(time.Second):
		t.Error("timed out waiting for IF_CLOSE")
	}
}

func TestEncodeOutSendsIfOpenMain(t *testing.T) {
	enc, _ := isaacPair([4]uint32{5, 6, 7, 8})
	wantEnc, _ := isaacPair([4]uint32{5, 6, 7, 8})

	p, clientConn := newTestPlayer(t)
	p.client.encryptor = enc
	p.refreshModal = true
	p.modalState = modalStateMain
	p.modalMain = 1234

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

	expectedByte := byte((int(gameserver.OpIfOpenMain.Opcode) + int(wantEnc.GetNext())) & 0xff)

	select {
	case got := <-received:
		if got[0] != expectedByte {
			t.Errorf("IF_OPENMAIN encrypted opcode: got %d, want %d", got[0], expectedByte)
		}
		component := int(got[1])<<8 | int(got[2])
		if component != 1234 {
			t.Errorf("IF_OPENMAIN component: got %d, want 1234", component)
		}
		if p.refreshModal {
			t.Error("refreshModal should be false after encodeOut")
		}
	case <-time.After(time.Second):
		t.Error("timed out waiting for IF_OPENMAIN")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestEncodeOut -v
```

Expected: compile error — `p.encodeOut` undefined.

- [ ] **Step 3: Add `encodeOut` to `player.go`**

```go
// encodeOut mirrors TS NetworkPlayer.encodeOut(). It sends modal open/close
// packets for any state changes since the last tick. All modal fields are zero
// on a new Player, so this is a no-op until sub-spec 2 populates them.
func (p *Player) encodeOut() {
	modalChanged := p.modalMain != p.lastModalMain ||
		p.modalChat != p.lastModalChat ||
		p.modalSide != p.lastModalSide ||
		p.refreshModalClose

	if modalChanged {
		if p.refreshModalClose {
			p.writeOut(gameserver.OpIfClose, nil)
		}
		p.refreshModalClose = false
		p.lastModalMain = p.modalMain
		p.lastModalChat = p.modalChat
		p.lastModalSide = p.modalSide
	}

	if p.refreshModal {
		switch {
		case p.modalState&modalStateMain != modalStateNone && p.modalState&modalStateSide != modalStateNone:
			payload := []byte{byte(p.modalMain >> 8), byte(p.modalMain), byte(p.modalSide >> 8), byte(p.modalSide)}
			p.writeOut(gameserver.OpIfOpenMainSide, payload)
		case p.modalState&modalStateMain != modalStateNone:
			payload := []byte{byte(p.modalMain >> 8), byte(p.modalMain)}
			p.writeOut(gameserver.OpIfOpenMain, payload)
		case p.modalState&modalStateChat != modalStateNone:
			payload := []byte{byte(p.modalChat >> 8), byte(p.modalChat)}
			p.writeOut(gameserver.OpIfOpenChat, payload)
		case p.modalState&modalStateSide != modalStateNone:
			payload := []byte{byte(p.modalSide >> 8), byte(p.modalSide)}
			p.writeOut(gameserver.OpIfOpenSide, payload)
		}
		p.refreshModal = false
	}
}
```

- [ ] **Step 4: Run the tests to verify they pass**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestEncodeOut -v
```

Expected: all `TestEncodeOut*` pass.

- [ ] **Step 5: Commit**

```bash
git add modules/world/player.go modules/world/player_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): implement Player.encodeOut with modal state management

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 8: Add stub update methods and `Player.processOut`

**Files:**
- Modify: `modules/world/player.go`

No meaningful behavior to test here — just compile and build verification.

- [ ] **Step 1: Add stub update methods and `processOut` to `player.go`**

```go
func (p *Player) updateMap()       {}
func (p *Player) updatePlayers()   {}
func (p *Player) updateNpcs()      {}
func (p *Player) updateZones()     {}
func (p *Player) updateInvs()      {}
func (p *Player) updateStats()     {}
func (p *Player) updateAfkZones()  {}

func (p *Player) processOut() {
	p.updateMap()
	p.updatePlayers()
	p.updateNpcs()
	p.updateZones()
	p.updateInvs()
	p.updateStats()
	p.updateAfkZones()
	p.encodeOut()
	p.client.flushWrite()
}
```

- [ ] **Step 2: Verify it compiles**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./modules/world/...
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add modules/world/player.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): add stub update methods and Player.processOut skeleton

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 9: Add player registry to `Server` with `addPlayer`/`removePlayer`

**Files:**
- Modify: `modules/world/server.go`
- Modify: `modules/world/server_test.go`

- [ ] **Step 1: Write the failing tests**

Add to `modules/world/server_test.go`:

```go
func newTestServer(t *testing.T) *Server {
	t.Helper()
	serverConn, _ := net.Pipe()
	t.Cleanup(func() { serverConn.Close() })
	s := &Server{
		quit: make(chan interface{}),
		log:  discardLogger(),
	}
	return s
}

func TestAddPlayerAssignsSlot(t *testing.T) {
	s := newTestServer(t)
	c, _ := newTestClient(t)
	p := newPlayer(c)

	if err := s.addPlayer(p); err != nil {
		t.Fatalf("addPlayer: %v", err)
	}
	if p.slot < 1 || p.slot > 2047 {
		t.Errorf("slot out of range: %d", p.slot)
	}
	if s.players[p.slot] != p {
		t.Error("players[slot] should point to p")
	}
	if len(s.playerLoop) != 1 {
		t.Errorf("playerLoop len: got %d, want 1", len(s.playerLoop))
	}
}

func TestRemovePlayerClearsSlot(t *testing.T) {
	s := newTestServer(t)
	c, _ := newTestClient(t)
	p := newPlayer(c)

	_ = s.addPlayer(p)
	slot := p.slot

	s.removePlayer(p)

	if s.players[slot] != nil {
		t.Error("players[slot] should be nil after remove")
	}
	if len(s.playerLoop) != 0 {
		t.Errorf("playerLoop len: got %d, want 0", len(s.playerLoop))
	}
}

func TestAddPlayerWorldFull(t *testing.T) {
	s := newTestServer(t)

	// Fill all 2047 slots
	for i := 1; i <= 2047; i++ {
		s.players[i] = &Player{slot: i}
	}

	c, _ := newTestClient(t)
	p := newPlayer(c)
	if err := s.addPlayer(p); err == nil {
		t.Error("expected error when world is full")
	}
}

func TestAddPlayerConcurrentSafety(t *testing.T) {
	s := newTestServer(t)
	done := make(chan struct{})

	// Concurrently add and remove players
	go func() {
		defer close(done)
		for i := range 50 {
			c, conn := newTestClient(t)
			_ = conn
			p := newPlayer(c)
			if err := s.addPlayer(p); err == nil {
				s.removePlayer(p)
			}
			_ = i
		}
	}()

	// Concurrently read the playerLoop
	for range 50 {
		s.playersMu.RLock()
		_ = len(s.playerLoop)
		s.playersMu.RUnlock()
	}

	<-done
}
```

- [ ] **Step 2: Run the tests to verify they fail**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run "TestAddPlayer|TestRemovePlayer" -v
```

Expected: compile error — `s.addPlayer`, `s.players`, `s.playerLoop`, `s.playersMu` undefined.

- [ ] **Step 3: Add registry fields and methods to `server.go`**

In `server.go`, add these fields to the `Server` struct:

```go
type Server struct {
	handler     SignalHandler
	tcpListener net.Listener
	quit        chan interface{}
	log         *slog.Logger
	loginClient *LoginClient
	cfg         Config
	tcpWg       sync.WaitGroup

	players     [2048]*Player
	playerLoop  []*Player
	playersMu   sync.RWMutex
	currentTick int
}
```

Add these methods to `server.go`:

```go
var errWorldFull = errors.New("world full")

func (s *Server) addPlayer(p *Player) error {
	s.playersMu.Lock()
	defer s.playersMu.Unlock()

	for i := 1; i < len(s.players); i++ {
		if s.players[i] == nil {
			p.slot = i
			s.players[i] = p
			s.playerLoop = append(s.playerLoop, p)
			return nil
		}
	}
	return errWorldFull
}

func (s *Server) removePlayer(p *Player) {
	s.playersMu.Lock()
	defer s.playersMu.Unlock()

	if p.slot < 1 || p.slot >= len(s.players) || s.players[p.slot] != p {
		return
	}
	s.players[p.slot] = nil

	for i, lp := range s.playerLoop {
		if lp == p {
			s.playerLoop = append(s.playerLoop[:i], s.playerLoop[i+1:]...)
			break
		}
	}
}
```

- [ ] **Step 4: Run the tests to verify they pass**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run "TestAddPlayer|TestRemovePlayer" -v
```

Expected: all pass.

- [ ] **Step 5: Run with race detector**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/... -run "TestAddPlayer|TestRemovePlayer" -v
```

Expected: all pass with no race conditions.

- [ ] **Step 6: Commit**

```bash
git add modules/world/server.go modules/world/server_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): add thread-safe player registry with addPlayer/removePlayer

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 10: Login/logout integration and reader goroutine change

**Files:**
- Modify: `modules/world/server.go`
- Modify: `modules/world/server_test.go`

- [ ] **Step 1: Write the failing tests**

Add to `modules/world/server_test.go`:

```go
func TestSendLoginOKRegistersPlayer(t *testing.T) {
	s := newTestServer(t)
	c, clientConn := newTestClient(t)
	c.server = s

	go io.Copy(io.Discard, clientConn) // drain login OK byte

	if err := c.sendLoginOK(); err != nil {
		t.Fatalf("sendLoginOK: %v", err)
	}
	if c.player == nil {
		t.Fatal("c.player should be set after sendLoginOK")
	}
	if c.player.slot < 1 {
		t.Errorf("player slot: got %d, want >= 1", c.player.slot)
	}
	if s.players[c.player.slot] != c.player {
		t.Error("player not found in server registry")
	}
}

func TestSendLoginOKWorldFullReturnsError(t *testing.T) {
	s := newTestServer(t)
	c, clientConn := newTestClient(t)
	c.server = s

	// Fill all slots
	for i := 1; i < len(s.players); i++ {
		s.players[i] = &Player{slot: i}
	}

	go io.Copy(io.Discard, clientConn)
	err := c.sendLoginOK()
	if err == nil {
		t.Error("expected error when world is full")
	}
	if c.state == ClientStateGame {
		t.Error("state should not be ClientStateGame when world is full")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run "TestSendLoginOKRegisters|TestSendLoginOKWorld" -v
```

Expected: tests compile but fail — `c.player` is nil after `sendLoginOK`.

- [ ] **Step 3: Update `sendLoginOK` in `client.go` to register the player**

Register the player *before* sending the OK byte so that a world-full error can be sent cleanly without having already committed to the OK response.

Replace `sendLoginOK` in `client.go`:

```go
func (c *client) sendLoginOK() error {
	if c.server != nil {
		p := newPlayer(c)
		if err := c.server.addPlayer(p); err != nil {
			return c.sendLoginError(loginresp.OpServerFull.Opcode)
		}
		c.player = p
	}

	if c.staffModLevel >= 1 {
		c.bufw.WriteByte(loginresp.OpLoginOKWithRights.Opcode)
	} else {
		c.bufw.WriteByte(loginresp.OpOK.Opcode)
	}
	if err := c.flushWrite(); err != nil {
		if c.server != nil && c.player != nil {
			c.server.removePlayer(c.player)
			c.player = nil
		}
		return fmt.Errorf("failed to flush login OK: %w", err)
	}
	c.state = ClientStateGame
	return nil
}
```

- [ ] **Step 4: Update `handleTCPConn` in `server.go`**

Replace the deferred cleanup block with one that also deregisters the player:

```go
defer func() {
	if c.player != nil {
		s.removePlayer(c.player)
		c.player = nil
	}
	if err := c.flushWrite(); err != nil {
		s.log.Warn("failed to flush on connection close", "error", err, "remote_addr", conn.RemoteAddr())
	}
	c.in.Release()
	putBufioReader64k(c.bufr)
	putBufioWriter64k(c.bufw)
	conn.Close()
	s.log.Info("connection closed", "remote_addr", conn.RemoteAddr())
}()
```

Replace the entire body of the inner read loop (from after `n, err := c.bufr.Read(buf)` through the end of the loop) with the following. The key change: remove the unconditional `bufferData` + `handleData` calls and replace them with a state-dispatch that uses `inMu` for game state:

```go
		n, err := c.bufr.Read(buf)
		if err != nil {
			if err != io.EOF {
				s.log.Error("connection read error", "error", err)
			}
			return
		}

		msg := buf[:n]
		c.log.Info("received data", "num_bytes", len(msg), "data", fmt.Sprintf("%v", msg))

		switch c.state {
		case ClientStateLogin:
			if !c.bufferData(msg) {
				c.log.Warn("incoming buffer overflow, closing connection", "remote_addr", conn.RemoteAddr())
				return
			}
			err = c.handleData()
			if err != nil {
				if errors.Is(err, protocol.ErrPayloadTooSmall) {
					continue
				}
				if errors.Is(err, errCloseConn) {
					return
				}
				c.log.Error("handleData error, closing connection", "error", err)
				return
			}
		case ClientStateGame:
			c.inMu.Lock()
			ok := c.bufferData(msg)
			c.inMu.Unlock()
			if !ok {
				c.log.Warn("incoming buffer overflow, closing connection", "remote_addr", conn.RemoteAddr())
				return
			}
		}
```

- [ ] **Step 5: Run all tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -v
```

Expected: all tests pass.

- [ ] **Step 6: Run with race detector**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/... -v
```

Expected: all pass, no data races.

- [ ] **Step 7: Commit**

```bash
git add modules/world/server.go modules/world/client.go modules/world/server_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): login registers player, disconnect deregisters; reader goroutine uses inMu for game state

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 11: Create `tick.go` and start the tick loop

**Files:**
- Create: `modules/world/tick.go`
- Modify: `modules/world/server.go`

- [ ] **Step 1: Write the failing test**

Add to `modules/world/server_test.go`:

```go
func TestTickLoopIncrementsCurrentTick(t *testing.T) {
	s := newTestServer(t)

	// Run the tick loop for a short duration and verify currentTick increments.
	// Use a very short tick rate to avoid slow tests.
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.runTickLoopWithRate(3 * time.Millisecond)
	}()

	time.Sleep(50 * time.Millisecond)
	close(s.quit)
	<-done

	if s.currentTick < 5 {
		t.Errorf("currentTick: got %d, want >= 5 after 50ms with 3ms tick rate", s.currentTick)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestTickLoop -v
```

Expected: compile error — `s.runTickLoopWithRate` undefined.

- [ ] **Step 3: Create `modules/world/tick.go`**

```go
package world

import "time"

const tickRate = 600 * time.Millisecond

func (s *Server) runTickLoop() {
	s.runTickLoopWithRate(tickRate)
}

func (s *Server) runTickLoopWithRate(rate time.Duration) {
	nextTick := time.Now()
	for {
		start := time.Now()
		drift := start.Sub(nextTick)
		if drift < 0 {
			drift = 0
		}

		s.processClientsIn()
		s.processClientsOut()
		s.currentTick++

		nextTick = nextTick.Add(rate)
		delay := rate - time.Since(start) - drift
		if delay < 0 {
			delay = 0
		}

		select {
		case <-s.quit:
			return
		case <-time.After(delay):
		}
	}
}

func (s *Server) processClientsIn() {
	s.playersMu.RLock()
	players := make([]*Player, len(s.playerLoop))
	copy(players, s.playerLoop)
	s.playersMu.RUnlock()

	for _, p := range players {
		p.processIn(s.currentTick)
	}
}

func (s *Server) processClientsOut() {
	s.playersMu.RLock()
	players := make([]*Player, len(s.playerLoop))
	copy(players, s.playerLoop)
	s.playersMu.RUnlock()

	for _, p := range players {
		p.processOut()
	}
}
```

- [ ] **Step 4: Run the test to verify it passes**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestTickLoop -v
```

Expected:
```
--- PASS: TestTickLoopIncrementsCurrentTick (0.05s)
ok  github.com/zsrv/goscape/modules/world
```

- [ ] **Step 5: Start the tick loop goroutine in `Server.Run()`**

In `server.go`, update `Run()` to start the tick loop alongside `serveTCP`:

```go
func (s *Server) Run() error {
	errChan := make(chan error, 1)

	go func() {
		s.handler.Loop()
		select {
		case errChan <- nil:
		default:
		}
	}()

	go func() {
		err := s.serveTCP()
		if errors.Is(err, net.ErrClosed) {
			err = nil
		}
		select {
		case errChan <- err:
		default:
		}
	}()

	go s.runTickLoop()

	return <-errChan
}
```

- [ ] **Step 6: Run the full test suite**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... -v
```

Expected: all tests pass.

- [ ] **Step 7: Run with race detector**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...
```

Expected: all pass, no data races.

- [ ] **Step 8: Commit**

```bash
git add modules/world/tick.go modules/world/server.go modules/world/server_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): add 600ms tick loop with processClientsIn/processClientsOut

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 12: Final verification

- [ ] **Step 1: Run the complete test suite**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
```

Expected:
```
ok  	github.com/zsrv/goscape/pkg/io/protocol/game/client	0.001s
ok  	github.com/zsrv/goscape/pkg/io/protocol/game/server	0.001s
ok  	github.com/zsrv/goscape/modules/world	0.XXs
ok  	...  (all other packages)
```

- [ ] **Step 2: Run with the race detector**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...
```

Expected: all pass, zero race conditions detected.

- [ ] **Step 3: Verify the binary builds**

```bash
CGO_ENABLED=0 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build -trimpath ./cmd/goscape
```

Expected: no errors.

- [ ] **Step 4: Clean up the build artifact**

```bash
rm -f goscape
```
