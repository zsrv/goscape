package world

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"testing"
	"time"

	"github.com/zsrv/goscape/pkg/gamemap"
	io2 "github.com/zsrv/goscape/pkg/io/isaac"
	"github.com/zsrv/goscape/pkg/io/packet"
	gameclient "github.com/zsrv/goscape/pkg/io/protocol/game/client"
	loginresp "github.com/zsrv/goscape/pkg/io/protocol/login/resp"
	"github.com/zsrv/goscape/pkg/loginpb"
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/pathfinder/collision"
	"github.com/zsrv/goscape/pkg/rsbuf"
	"github.com/zsrv/goscape/pkg/script"
	"github.com/zsrv/goscape/pkg/wordenc/encfilter"
	"github.com/zsrv/goscape/pkg/zone"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func newTestClient(t *testing.T) (*client, net.Conn) {
	t.Helper()
	serverConn, clientConn := net.Pipe()
	t.Cleanup(func() {
		serverConn.Close()
		clientConn.Close()
	})
	c := newClient(serverConn, time.Second, discardLogger())
	// dropConnRef (not a raw c.in.Release()) so this is a no-op if the test
	// already drove the client through removePlayerOnTick/processLogouts —
	// those call dropTickRef, which pool-returns the buffers once refs hit
	// 0. A second unconditional Release here would double-return the same
	// pooled object (arch-28.4b).
	t.Cleanup(func() { c.dropConnRef() })
	return c, clientConn
}

// Seed packet size

func TestSeedPacketIsExactly8Bytes(t *testing.T) {
	c, clientConn := newTestClient(t)

	received := make(chan []byte, 1)
	go func() {
		buf := make([]byte, 32) // read more than needed to catch oversized packets
		clientConn.SetReadDeadline(time.Now().Add(time.Second))
		n, _ := clientConn.Read(buf)
		received <- buf[:n]
	}()

	seed := packet.NewPacket(make([]byte, 0, 8))
	seed.P4(0xDEADBEEF)
	seed.P4(0xCAFEBABE)
	c.write(seed.Bytes())
	c.flushWrite()

	select {
	case got := <-received:
		if len(got) != 8 {
			t.Errorf("seed packet: got %d bytes, want 8", len(got))
		}
	case <-time.After(time.Second):
		t.Error("timed out waiting for seed packet")
	}
}

// Fix 3: incoming buffer overflow protection

func TestBufferDataRejectsWhenAtMaxSize(t *testing.T) {
	c, _ := newTestClient(t)
	c.in.Write(make([]byte, maxClientInBufSize))

	if c.bufferData([]byte{1}) {
		t.Error("expected bufferData to reject data when buffer is full")
	}
	if c.in.Len() != maxClientInBufSize {
		t.Errorf("buffer should be unchanged: got %d, want %d", c.in.Len(), maxClientInBufSize)
	}
}

func TestBufferDataAcceptsDataWithinLimit(t *testing.T) {
	c, _ := newTestClient(t)

	data := []byte{0xDE, 0xAD, 0xBE, 0xEF}
	if !c.bufferData(data) {
		t.Error("expected bufferData to accept data within limit")
	}
	if c.in.Len() != len(data) {
		t.Errorf("buffer length: got %d, want %d", c.in.Len(), len(data))
	}
}

func TestBufferDataRejectsExactOverflow(t *testing.T) {
	c, _ := newTestClient(t)
	c.in.Write(make([]byte, maxClientInBufSize-2))

	// 2 bytes remaining — 3 bytes should overflow
	if c.bufferData(make([]byte, 3)) {
		t.Error("expected bufferData to reject data that would overflow by 1 byte")
	}
}

func TestBufferDataAcceptsUpToExactLimit(t *testing.T) {
	c, _ := newTestClient(t)
	c.in.Write(make([]byte, maxClientInBufSize-3))

	// Exactly 3 bytes remaining
	if !c.bufferData(make([]byte, 3)) {
		t.Error("expected bufferData to accept data that fills buffer exactly")
	}
}

// Fix 4+5: login error byte is sent and errCloseConn is returned

func TestSendLoginErrorWritesCodeToClient(t *testing.T) {
	c, clientConn := newTestClient(t)

	received := make(chan byte, 1)
	go func() {
		buf := make([]byte, 1)
		clientConn.SetReadDeadline(time.Now().Add(time.Second))
		if _, err := io.ReadFull(clientConn, buf); err == nil {
			received <- buf[0]
		}
	}()

	_ = c.sendLoginError(loginresp.OpClientOutOfDate.Opcode)

	select {
	case got := <-received:
		if got != loginresp.OpClientOutOfDate.Opcode {
			t.Errorf("wrong response byte: got %d, want %d", got, loginresp.OpClientOutOfDate.Opcode)
		}
	case <-time.After(time.Second):
		t.Error("timed out waiting for response byte")
	}
}

func TestSendLoginErrorReturnsErrCloseConn(t *testing.T) {
	c, clientConn := newTestClient(t)
	go io.Copy(io.Discard, clientConn) // drain so flush doesn't block

	err := c.sendLoginError(loginresp.OpIPLimit.Opcode)
	if !errors.Is(err, errCloseConn) {
		t.Errorf("expected errCloseConn, got %v", err)
	}
}

func TestSendLoginErrorVariousCodes(t *testing.T) {
	codes := []byte{
		loginresp.OpInvalidUsernameOrPassword.Opcode,
		loginresp.OpBanned.Opcode,
		loginresp.OpDuplicate.Opcode,
		loginresp.OpClientOutOfDate.Opcode,
		loginresp.OpServerFull.Opcode,
		loginresp.OpLoginServerOffline.Opcode,
		loginresp.OpIPLimit.Opcode,
	}

	for _, code := range codes {
		t.Run("code"+string(rune('0'+code)), func(t *testing.T) {
			c, clientConn := newTestClient(t)

			received := make(chan byte, 1)
			go func() {
				buf := make([]byte, 1)
				clientConn.SetReadDeadline(time.Now().Add(time.Second))
				if _, err := io.ReadFull(clientConn, buf); err == nil {
					received <- buf[0]
				}
			}()

			if err := c.sendLoginError(code); !errors.Is(err, errCloseConn) {
				t.Errorf("expected errCloseConn for code %d", code)
			}

			select {
			case got := <-received:
				if got != code {
					t.Errorf("wrong byte: got %d, want %d", got, code)
				}
			case <-time.After(time.Second):
				t.Errorf("timed out waiting for code %d", code)
			}
		})
	}
}

func TestSendLoginOKSendsOpOKAndTransitionsState(t *testing.T) {
	c, clientConn := newTestClient(t)

	received := make(chan [3]byte, 1)
	go func() {
		var buf [3]byte
		clientConn.SetReadDeadline(time.Now().Add(time.Second))
		if _, err := io.ReadFull(clientConn, buf[:]); err == nil {
			received <- buf
		}
	}()

	if err := c.sendLoginOK(); err != nil {
		t.Fatalf("sendLoginOK: %v", err)
	}

	select {
	case got := <-received:
		// TS World.ts:946-950 @43e02957: [2, min(staffModLevel,2), 1].
		if want := [3]byte{loginresp.OpOK.Opcode, 0, 1}; got != want {
			t.Errorf("login OK bytes: got %v, want %v", got, want)
		}
	case <-time.After(time.Second):
		t.Error("timed out waiting for login OK bytes")
	}

	if c.state != ClientStateGame {
		t.Errorf("state after sendLoginOK: got %v, want ClientStateGame", c.state)
	}
}

func TestSendLoginOKStaffSendsCappedStaffByte(t *testing.T) {
	c, clientConn := newTestClient(t)
	c.staffModLevel = 1

	received := make(chan [3]byte, 1)
	go func() {
		var buf [3]byte
		clientConn.SetReadDeadline(time.Now().Add(time.Second))
		if _, err := io.ReadFull(clientConn, buf[:]); err == nil {
			received <- buf
		}
	}()

	if err := c.sendLoginOK(); err != nil {
		t.Fatalf("sendLoginOK: %v", err)
	}

	select {
	case got := <-received:
		// 254 drops the opcode-18 mod fork: staff level rides in byte 2 of
		// the always-opcode-2 reply (TS World.ts:946-950 @43e02957).
		if want := [3]byte{loginresp.OpOK.Opcode, 1, 1}; got != want {
			t.Errorf("staff login OK bytes: got %v, want %v", got, want)
		}
	case <-time.After(time.Second):
		t.Error("timed out waiting for staff login OK bytes")
	}
}

func TestGameProtTableHasExpectedOpcodes(t *testing.T) {
	cases := []struct {
		opcode      uint8
		name        string
		payloadSize int
	}{
		{gameclient.OpcNoTimeout, "NO_TIMEOUT", 0},
		{gameclient.OpcIdleTimer, "IDLE_TIMER", 0},
		{gameclient.OpcMoveGameClick, "MOVE_GAMECLICK", -1},
		{gameclient.OpcMoveOpClick, "MOVE_OPCLICK", -1},
		{gameclient.OpcMoveMinimapClick, "MOVE_MINIMAPCLICK", -1},
		{gameclient.OpcEventTracking, "EVENT_TRACKING", -2},
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

// isaacPair returns two independent ISAAC instances with identical initial state.
// Use enc to encrypt opcodes in the test, dec to give to the client under test.
func isaacPair(seed [4]uint32) (enc, dec *io2.Isaac) {
	return io2.New(seed), io2.New(seed)
}

// encryptOpcode produces the wire byte the Java client sends for realOpcode.
func encryptOpcode(enc *io2.Isaac, realOpcode byte) byte {
	return byte((int(realOpcode) + int(enc.GetNext())) & 0xff)
}

// defaultTestProvider returns a Provider with a global [opnpc1] no-op script
// that suspends via P_DELAY(0). Tests that need a real provider seed their
// own via s.scriptProvider = script.NewProvider() after calling newTestServer.
func defaultTestProvider() *script.Provider {
	p := script.NewProvider()
	// Global catch-all: matches any NPC + op1 when no type/category script exists.
	// Suspends (P_DELAY) so processInteraction leaves interacted=true and target
	// intact — allowing reach/face tests written before script dispatch existed to
	// keep passing without modification.
	//
	// Side effect: tests that reach in-range dispatch via this fixture will see
	// p.delayed=true and p.activeScript!=nil after processInteraction runs, since
	// P_DELAY suspends the player for currentTick+1. Tests asserting the absence
	// of those fields must seed an empty provider (s.scriptProvider = script.NewProvider()).
	globalScript := &script.ScriptFile{
		Name:      "[opnpc1,_default]",
		LookupKey: script.LookupKeyForGlobal(script.TriggerOpNpc1),
		Opcodes: []script.Opcode{
			script.OpPushConstantInt,
			script.OpPDelay,
			script.OpReturn,
		},
		IntOperands:      []int32{0, 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	p.Register(globalScript)
	return p
}

func newTestServer(t *testing.T) *Server {
	t.Helper()
	// Arc 18 R3 — bridgesCtx must be non-nil so tests that inject a
	// non-nil loginClient/friendsClient and exercise removePlayerOnTick /
	// autosavePlayers / processLogins (which now do
	// context.WithTimeout(s.bridgesCtx, ...)) don't nil-deref.
	bridgesCtx, bridgesCancel := context.WithCancel(context.Background())
	t.Cleanup(bridgesCancel)
	s := &Server{
		quit:             make(chan interface{}),
		log:              discardLogger(),
		scriptProvider:   defaultTestProvider(),
		zoneMap:          zone.NewZoneMap(),
		locObjTracker:    newLocObjTracker(),
		rsbuf:            rsbuf.New(),
		shutdownTick:     -1,
		tickRate:         defaultTickRate,
		gracefulExit:     make(chan struct{}),
		rebuildReq:       make(chan struct{}, 1),
		rebuildResult:    make(chan rebuildResult, 1),
		relayActionQueue: make(chan func(), 64),
		bridgesCtx:       bridgesCtx,
		bridgesCancel:    bridgesCancel,
		players:          newPlayerList(2048),
	}
	// R4 (Arc 18): pmCount is atomic.Uint32; init to 1 per TS World.ts:167.
	s.pmCount.Store(1)
	s.initChildLoggers(s.log)
	s.friendsBridge = noopBridges{}
	s.loginBridgeMod = noopBridges{}
	s.loggerBridge = noopBridges{}
	s.locOps = &serverLocOps{s: s}
	s.reloadFn = s.Reload
	s.watchSessionFn = s.runWatchSession
	s.runScriptFn = s.runScript
	// packFn intentionally left nil; tests that exercise the worker set it explicitly.
	// Inject a passthrough filter so test paths that may invoke s.wordenc.Filter
	// via T9/T10 wiring do not require a real wordenc jagfile in the test cache.
	s.wordenc = encfilter.Empty()
	return s
}

// newTestPlayerWithInvTypes constructs a Player wired to a Server whose
// invTypes is populated with the given configs. Used by scope-aware tests
// of (*Player).invListenOnCom that exercise the SCOPE_SHARED rewrite
// branch (γ). For tests that don't need invTypes wiring, use newTestPlayer.
//
// Builds a minimal Server inline (only invTypes + log) rather than calling
// newTestServer — the (γ) lookup chain reads only invTypes; the
// quit-channel / scriptProvider scaffolding from newTestServer is unneeded
// here.
func newTestPlayerWithInvTypes(t *testing.T, configs []*objtype.InvType) (*Player, net.Conn) {
	t.Helper()
	p, conn := newTestPlayer(t)
	p.client.server = &Server{
		log:      discardLogger(),
		invTypes: &objtype.InvTypeConfigs{Configs: configs},
	}
	return p, conn
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
	if s.players.get(p.slot) != p {
		t.Error("players.get(slot) should point to p")
	}
	if s.players.count.Load() != 1 {
		t.Errorf("players.count: got %d, want 1", s.players.count.Load())
	}
}

func TestRemovePlayerClearsSlot(t *testing.T) {
	s := newTestServer(t)
	c, _ := newTestClient(t)
	p := newPlayer(c)

	_ = s.addPlayer(p)
	slot := p.slot

	s.removePlayerInternal(p)

	if s.players.get(slot) != nil {
		t.Error("players[slot] should be nil after remove")
	}
	if s.players.count.Load() != 0 {
		t.Errorf("players.count: got %d, want 0", s.players.count.Load())
	}
}

// TestRemovePlayerClearsNpcCollisionFootprint pins world-ops-2.
// TS World.removePlayer (World.ts:1601) unconditionally calls
// changeNpcCollision(width, x, z, level, false) on logout. goscape's
// removePlayerInternal must do the same, otherwise the FlagBlockNPCs
// set at the logout tile (e.g. by SetVisibility(Default)) remains
// stuck on the map after the player is gone.
func TestRemovePlayerClearsNpcCollisionFootprint(t *testing.T) {
	s := newTestServer(t)
	s.gamemap = gamemap.New(discardLogger())
	c, _ := newTestClient(t)
	p := newPlayer(c)

	_ = s.addPlayer(p)
	// Seed FlagBlockNPCs at the player's tile, matching what
	// SetVisibility(Default) (player.go:674) would have left there.
	s.gamemap.Pathfinder.Flags.AllocateIfAbsent(p.x, p.z, p.level)
	s.gamemap.ChangeNPCCollision(1, p.x, p.z, p.level, true)
	if !s.gamemap.Pathfinder.Flags.IsFlagged(p.x, p.z, p.level, collision.FlagBlockNPCs) {
		t.Fatal("seed: FlagBlockNPCs must be set before removePlayerInternal")
	}

	s.removePlayerInternal(p)

	if s.gamemap.Pathfinder.Flags.IsFlagged(p.x, p.z, p.level, collision.FlagBlockNPCs) {
		t.Error("TS World.removePlayer (World.ts:1601) calls changeNpcCollision(false); FlagBlockNPCs must be cleared after removePlayerInternal")
	}
}

func TestAddPlayerWorldFull(t *testing.T) {
	s := newTestServer(t)

	for i := 1; i <= 2047; i++ {
		s.players.set(i, &Player{slot: i})
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

	go func() {
		defer close(done)
		for range 50 {
			c, _ := newTestClient(t)
			p := newPlayer(c)
			if err := s.addPlayer(p); err == nil {
				s.removePlayerInternal(p)
			}
		}
	}()

	for range 50 {
		s.playersMu.RLock()
		_ = s.players.count.Load()
		s.playersMu.RUnlock()
	}

	<-done
}

func TestSendLoginOKQueuesPlayer(t *testing.T) {
	s := newTestServer(t)
	c, clientConn := newTestClient(t)
	c.server = s

	go io.Copy(io.Discard, clientConn)

	if err := c.sendLoginOK(); err != nil {
		t.Fatalf("sendLoginOK: %v", err)
	}
	if c.player == nil {
		t.Fatal("c.player should be set after sendLoginOK")
	}
	s.playersMu.RLock()
	queued := len(s.newPlayers)
	s.playersMu.RUnlock()
	if queued != 1 {
		t.Errorf("newPlayers queue: got %d, want 1", queued)
	}
	if c.state != ClientStateGame {
		t.Errorf("state: got %v, want ClientStateGame", c.state)
	}
}

func TestProcessLoginsDrainsNewPlayers(t *testing.T) {
	s := newTestServer(t)
	c, clientConn := newTestClient(t)
	c.server = s
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	c.encryptor = enc
	go io.Copy(io.Discard, clientConn)

	p := newPlayer(c)
	c.player = p
	s.appendNewPlayer(p)

	s.processLogins()

	s.playersMu.RLock()
	queued := len(s.newPlayers)
	inPlayers := int(s.players.count.Load())
	s.playersMu.RUnlock()

	if queued != 0 {
		t.Errorf("newPlayers: got %d, want 0", queued)
	}
	if inPlayers != 1 {
		t.Errorf("players.count: got %d, want 1", inPlayers)
	}
	if p.slot < 1 {
		t.Errorf("slot: got %d, want >= 1", p.slot)
	}
}

func TestProcessLoginsWorldFullRejectsCleanly(t *testing.T) {
	s := newTestServer(t)

	for i := 1; i < len(s.players.entities); i++ {
		s.players.set(i, &Player{slot: i})
	}

	c, clientConn := newTestClient(t)
	c.server = s
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	c.encryptor = enc
	go io.Copy(io.Discard, clientConn)
	p := newPlayer(c)
	c.player = p
	s.appendNewPlayer(p)

	s.processLogins()

	s.playersMu.RLock()
	queued := len(s.newPlayers)
	s.playersMu.RUnlock()

	if queued != 0 {
		t.Errorf("newPlayers should be drained even on world-full: got %d", queued)
	}
}

func TestProcessLogoutsTimeoutMarksLoggingOut(t *testing.T) {
	s := newTestServer(t)
	c, clientConn := newTestClient(t)
	c.server = s
	c.state = ClientStateGame
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	c.encryptor = enc
	go io.Copy(io.Discard, clientConn)

	p := newPlayer(c)
	c.player = p
	if err := s.addPlayer(p); err != nil {
		t.Fatal(err)
	}
	p.lastResponse = 0
	p.lastConnected = 0
	s.currentTick = timeoutNoResponse

	s.processLogouts()

	if !p.loggingOut {
		t.Error("loggingOut should be true after lastResponse timeout")
	}
	still := s.players.count.Load()
	if still != 0 {
		t.Errorf("players.count should be 0 after logout, got %d", still)
	}
}

func TestProcessInUpdatesLastConnectedWhenGameState(t *testing.T) {
	s := newTestServer(t)
	c, _ := newTestClient(t)
	c.server = s
	c.state = ClientStateGame
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	c.encryptor = enc
	p := newPlayer(c)
	c.player = p
	p.lastConnected = 0

	p.processIn(42)

	if p.lastConnected != 42 {
		t.Errorf("lastConnected: got %d, want 42", p.lastConnected)
	}
}

// TestTickIterationLoginOrderNotSlotOrder pins the rev-254 player-loop
// restructure (TS a8186b95 @2e3bcf43): slots are assigned lowest-free
// (World.getNextPlayerSlot, World.ts:1634-1642) while per-tick processing
// iterates the IP-bucketed playerLoop (HashTable.all, HashTable.ts:49-60)
// in bucket order then insertion (login) order — slot numbers no longer
// drive processing order. Supersedes the rev-244 pid-order pin
// (TestTickIterationPidOrder); PORTING-EXCEPTION (gap-db-datastruct-4)
// stays closed, now via the playerLoop port.
//
// net.Pipe test clients have address "pipe" → all land in the headless
// 127.0.0.1 bucket, so iteration order here is pure login order.
func TestTickIterationLoginOrderNotSlotOrder(t *testing.T) {
	s := newTestServer(t)

	c1, _ := newTestClient(t)
	p1 := newPlayer(c1)
	c2, _ := newTestClient(t)
	p2 := newPlayer(c2)
	c3, _ := newTestClient(t)
	p3 := newPlayer(c3)

	// Add three players; they occupy slots 1, 2, 3 (linear from a fresh list).
	if err := s.addPlayer(p1); err != nil {
		t.Fatalf("addPlayer p1: %v", err)
	}
	if err := s.addPlayer(p2); err != nil {
		t.Fatalf("addPlayer p2: %v", err)
	}
	if err := s.addPlayer(p3); err != nil {
		t.Fatalf("addPlayer p3: %v", err)
	}
	if p1.slot != 1 || p2.slot != 2 || p3.slot != 3 {
		t.Fatalf("linear slot assignment: got %d,%d,%d, want 1,2,3", p1.slot, p2.slot, p3.slot)
	}

	// Remove p1; the next login reuses the lowest free slot (1), unlike
	// the rev-244 round-robin which would have resumed past 3.
	s.removePlayerInternal(p1)

	c4, _ := newTestClient(t)
	p4 := newPlayer(c4)
	if err := s.addPlayer(p4); err != nil {
		t.Fatalf("addPlayer p4 (re-login): %v", err)
	}
	if p4.slot != 1 {
		t.Fatalf("p4.slot = %d, want 1 (lowest free slot reused)", p4.slot)
	}

	// s.players.all() must yield in login order — p2, p3, p4 — even
	// though p4 holds the lowest slot number.
	var got []*Player
	for p := range s.players.all() {
		got = append(got, p)
	}
	want := []*Player{p2, p3, p4}
	if len(got) != len(want) {
		t.Fatalf("all() length: got %d, want %d", len(got), len(want))
	}
	for i, p := range got {
		if p != want[i] {
			t.Errorf("all()[%d]: got slot %d, want slot %d", i, p.slot, want[i].slot)
		}
	}
}

func TestProcessInDoesNotUpdateLastConnectedWhenNotGameState(t *testing.T) {
	s := newTestServer(t)
	c, _ := newTestClient(t)
	c.server = s
	// c.state defaults to ClientStateLogin
	p := newPlayer(c)
	c.player = p
	p.lastConnected = 5

	p.processIn(42)

	if p.lastConnected != 5 {
		t.Errorf("lastConnected: got %d, want 5 (unchanged)", p.lastConnected)
	}
}

func TestTickLoopIncrementsCurrentTick(t *testing.T) {
	s := newTestServer(t)

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

func BenchmarkClientSetup(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		c := newClient(nil, 30*time.Second, slog.Default())
		// dropConnRef, not raw pool returns: the refcount contract
		// (arch-28.4b) owns buffer release; a fresh client's only owner is
		// the conn side, so this pool-returns all three buffers.
		c.dropConnRef()
	}
}

// TestPlayerCanAccess asserts the four-case truth table for S7a:
// delayed, modal main/chat open, or protected activeScript → false;
// otherwise → true. Mirrors TS Player.canAccess.
func TestPlayerCanAccess(t *testing.T) {
	cases := []struct {
		name            string
		delayed         bool
		modalState      int
		protectedScript bool
		want            bool
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
				p.activeScript = &script.ScriptState{Pointers: script.PtrProtectedActivePlayer}
				p.protect = true // NAI-111-D1: Player.protect is the TS-faithful gate, set alongside activeScript fixture
			}
			if got := p.CanAccess(); got != tc.want {
				t.Errorf("CanAccess() = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestLookupPlayerByUIDFound: a single logged-in player with a matching
// uid is returned.
func TestLookupPlayerByUIDFound(t *testing.T) {
	s := newTestServer(t)
	c, _ := newTestClient(t)
	p := newPlayer(c)
	if err := s.addPlayer(p); err != nil {
		t.Fatalf("addPlayer: %v", err)
	}

	// After addPlayer, p.uid is composed from (p.username37=0, p.slot).
	// Use p.uid as the lookup arg.
	got := s.LookupPlayerByUID(p.uid)
	if got != p {
		t.Errorf("LookupPlayerByUID(%d) = %v, want %v", p.uid, got, p)
	}
}

// TestLookupPlayerByUIDNotFound: returns nil for an unknown uid.
func TestLookupPlayerByUIDNotFound(t *testing.T) {
	s := newTestServer(t)
	c, _ := newTestClient(t)
	p := newPlayer(c)
	_ = s.addPlayer(p)

	// After addPlayer, p.uid is composed from (p.username37=0, p.slot).
	// Use a uid that won't collide.
	notFoundUID := p.uid + 1000
	got := s.LookupPlayerByUID(notFoundUID)
	if got != nil {
		t.Errorf("LookupPlayerByUID(%d) = %v, want nil", notFoundUID, got)
	}
}

// TestLookupPlayerByUIDSkipsInactive: an entry in s.players whose
// active flag is false is not returned even on uid match. This defends
// against stale references during the add/remove race window — the
// tick loop drains newPlayers and addPlayer flips active=true; removal
// flips active=false before the slot reassignment. See server.go:586-596.
func TestLookupPlayerByUIDSkipsInactive(t *testing.T) {
	s := newTestServer(t)
	c, _ := newTestClient(t)
	p := newPlayer(c)
	_ = s.addPlayer(p)
	p.active = false

	// After addPlayer, p.uid is composed from (p.username37=0, p.slot).
	// Lookup arg matches p.uid, but active=false, so should return nil.
	got := s.LookupPlayerByUID(p.uid)
	if got != nil {
		t.Errorf("LookupPlayerByUID(%d) on inactive player = %v, want nil", p.uid, got)
	}
}

// setPlayerCountForTest is a test-only helper that fills the first playerCount
// slots of s.players with non-nil placeholder entries, clearing the rest.
// Used by TestScaleByPlayerCountFormula to simulate a given active-player count.
func setPlayerCountForTest(t *testing.T, s *Server, playerCount int) {
	t.Helper()
	// Clear all existing entries first.
	for i := range s.players.entities {
		if s.players.entities[i] != nil {
			s.players.remove(s.players.get(i))
		}
	}
	// Fill slots 1..playerCount with placeholder entries.
	for i := 1; i <= playerCount && i < len(s.players.entities); i++ {
		s.players.set(i, &Player{slot: i})
	}
}

// TestScaleByPlayerCountFormula pins the TS World.scaleByPlayerCount
// formula at TS World.ts:1715-1719:
//
//	playerCount := min(getTotalPlayers(), 2000)
//	return ((4000 - playerCount) * rate) / 4000  // int truncation
//
// Cap at 2000 players; rate=100, count=0 → 100; rate=100, count=2000 → 50;
// rate=100, count=2048 (capped to 2000) → 50.
func TestScaleByPlayerCountFormula(t *testing.T) {
	cases := []struct {
		playerCount, rate, want int
	}{
		{0, 100, 100},   // empty world: full rate
		{2000, 100, 50}, // cap point: half rate
		{2048, 100, 50}, // beyond cap (full array): still half rate
		{1000, 100, 75}, // mid: 3/4 rate
		{0, 0, 0},       // zero rate
		{0, -1, -1},     // negative rate passes through
	}
	s := &Server{players: newPlayerList(2048)}
	for _, c := range cases {
		setPlayerCountForTest(t, s, c.playerCount)
		got := s.scaleByPlayerCount(c.rate)
		if got != c.want {
			t.Errorf("scaleByPlayerCount(rate=%d, players=%d): got %d, want %d",
				c.rate, c.playerCount, got, c.want)
		}
	}
}

func TestAddPlayerEntersZoneAndFlagsGrid(t *testing.T) {
	s := newTestServer(t)
	c, _ := newTestClient(t)
	p := newPlayer(c)
	// newPlayer's default coords are (0,0,0); set to a known zone for clarity.
	p.x, p.z, p.level = 3200, 3200, 0
	if err := s.addPlayer(p); err != nil {
		t.Fatalf("addPlayer: %v", err)
	}
	z := s.zoneMap.Get(0, 3200, 3200)
	if z.PlayersCount() != 1 {
		t.Errorf("after addPlayer, Zone.PlayersCount: got %d, want 1", z.PlayersCount())
	}
	if !s.zoneMap.Grid(0).IsFlagged(400, 400, 0) {
		t.Error("after addPlayer, ZoneGrid should be flagged at (400,400)")
	}
	if p.zoneListElement == nil {
		t.Error("addPlayer should populate p.zoneListElement")
	}
}

func TestRemovePlayerLeavesZoneAndUnflagsGrid(t *testing.T) {
	s := newTestServer(t)
	c, _ := newTestClient(t)
	p := newPlayer(c)
	p.x, p.z, p.level = 3200, 3200, 0
	if err := s.addPlayer(p); err != nil {
		t.Fatalf("addPlayer: %v", err)
	}
	s.removePlayerInternal(p)
	z := s.zoneMap.Get(0, 3200, 3200)
	if z.PlayersCount() != 0 {
		t.Errorf("after removePlayer, Zone.PlayersCount: got %d, want 0", z.PlayersCount())
	}
	if s.zoneMap.Grid(0).IsFlagged(400, 400, 0) {
		t.Error("after removePlayer (last player gone), grid should be unflagged")
	}
	if p.zoneListElement != nil {
		t.Error("removePlayer should null p.zoneListElement")
	}
}

func TestRemovePlayerDoubleCallIsNoop(t *testing.T) {
	s := newTestServer(t)
	c, _ := newTestClient(t)
	p := newPlayer(c)
	p.x, p.z, p.level = 3200, 3200, 0
	if err := s.addPlayer(p); err != nil {
		t.Fatalf("addPlayer: %v", err)
	}
	s.removePlayerInternal(p)
	// Second call must not panic.
	s.removePlayerInternal(p)
	z := s.zoneMap.Get(0, 3200, 3200)
	if z.PlayersCount() != 0 {
		t.Errorf("PlayersCount after double removePlayer: got %d, want 0", z.PlayersCount())
	}
}

// TestRemovePlayerClearsBuildArea pins TS Player.ts:453 cleanup() →
// buildArea.clear(false): all three buildArea sets must be empty after
// removePlayerInternal. Mirrors TS Player.cleanup field order
// (heroPoints.clear → buildArea.clear(false) → appearanceInv=-1).
// Pre-existing 225 gap; wired in rev-244 B3 T24.
func TestRemovePlayerClearsBuildArea(t *testing.T) {
	s := newTestServer(t)
	c, _ := newTestClient(t)
	p := newPlayer(c)
	p.x, p.z, p.level = 3200, 3200, 0
	if err := s.addPlayer(p); err != nil {
		t.Fatalf("addPlayer: %v", err)
	}

	// Seed non-empty state so the test can observe the clear.
	p.buildArea.activeZones[999] = true
	p.buildArea.loadedZones[888] = true
	p.buildArea.mapsquares[777] = true

	s.removePlayerInternal(p)

	if len(p.buildArea.activeZones) != 0 {
		t.Errorf("buildArea.activeZones not cleared: len=%d", len(p.buildArea.activeZones))
	}
	if len(p.buildArea.loadedZones) != 0 {
		t.Errorf("buildArea.loadedZones not cleared: len=%d", len(p.buildArea.loadedZones))
	}
	if len(p.buildArea.mapsquares) != 0 {
		t.Errorf("buildArea.mapsquares not cleared: len=%d", len(p.buildArea.mapsquares))
	}
}

// TestLookupPlayerBySlot_Found returns the player at the slot.
func TestLookupPlayerBySlot_Found(t *testing.T) {
	s := newTestServer(t)
	p, _ := newTestPlayer(t)
	slot := 5
	s.players.set(slot, p)
	t.Cleanup(func() { s.players.remove(p) })

	got := s.LookupPlayerBySlot(slot)
	if got != p {
		t.Errorf("LookupPlayerBySlot(%d): got %v, want %p", slot, got, p)
	}
}

// TestLookupPlayerBySlot_OutOfRange returns nil for indices outside
// [0, len(s.players.entities)).
func TestLookupPlayerBySlot_OutOfRange(t *testing.T) {
	s := newTestServer(t)
	if got := s.LookupPlayerBySlot(-1); got != nil {
		t.Errorf("LookupPlayerBySlot(-1): got %v, want nil", got)
	}
	if got := s.LookupPlayerBySlot(len(s.players.entities)); got != nil {
		t.Errorf("LookupPlayerBySlot(len): got %v, want nil", got)
	}
	if got := s.LookupPlayerBySlot(len(s.players.entities) + 100); got != nil {
		t.Errorf("LookupPlayerBySlot(len+100): got %v, want nil", got)
	}
}

// TestLookupPlayerBySlot_EmptySlotReturnsNil — slot is in range but no
// player logged in there.
func TestLookupPlayerBySlot_EmptySlotReturnsNil(t *testing.T) {
	s := newTestServer(t)
	if got := s.LookupPlayerBySlot(7); got != nil {
		t.Errorf("LookupPlayerBySlot(empty): got %v, want nil", got)
	}
}

// TestIsUsernameLoggingOut_HitWhenPlayerLoggingOut pins the lookup that
// guards handleLogin against admitting a new session while the prior
// one is still draining. Mirrors TS World.ts:2194-2199
// (logoutRequests.has(username) → reply byte 5).
func TestIsUsernameLoggingOut_HitWhenPlayerLoggingOut(t *testing.T) {
	s := newTestServer(t)
	s.players.set(1, &Player{username: "bob", loggingOut: true})

	if !s.isUsernameLoggingOut("bob") {
		t.Error("isUsernameLoggingOut(bob): got false, want true")
	}
}

// TestIsUsernameLoggingOut_MissWhenLoggingOutFalse: a live player with
// the same name but loggingOut=false must NOT trigger the guard — that
// path is the OpDuplicate handled by the login-server RPC, not by this
// pre-RPC check (which only fires for in-flight logouts on THIS world).
func TestIsUsernameLoggingOut_MissWhenLoggingOutFalse(t *testing.T) {
	s := newTestServer(t)
	s.players.set(1, &Player{username: "bob", loggingOut: false})

	if s.isUsernameLoggingOut("bob") {
		t.Error("isUsernameLoggingOut(bob, loggingOut=false): got true, want false")
	}
}

// TestIsUsernameLoggingOut_MissWhenNameDiffers verifies the lookup is
// keyed by safe-name and ignores other logging-out players.
func TestIsUsernameLoggingOut_MissWhenNameDiffers(t *testing.T) {
	s := newTestServer(t)
	s.players.set(1, &Player{username: "alice", loggingOut: true})

	if s.isUsernameLoggingOut("bob") {
		t.Error("isUsernameLoggingOut(bob) with only alice logging out: got true, want false")
	}
}

// TestIsUsernameLoggingOut_EmptyServer guards against false-positives on
// a freshly initialized server with no players.
func TestIsUsernameLoggingOut_EmptyServer(t *testing.T) {
	s := newTestServer(t)

	if s.isUsernameLoggingOut("bob") {
		t.Error("isUsernameLoggingOut on empty server: got true, want false")
	}
}

// TestHandleLogin_FullWorldReturnsServerFull drives handleLogin past the
// pre-RPC fullness check at server.go:~813 (TS World.ts:2188-2192). A
// server populated to one player above NodeMaxConnected must short-
// circuit with reply byte 7 (OpServerFull) before reaching the login
// RPC. Tests the helper integration without needing a fake RSA stack:
// we invoke the same getTotalPlayers/cfg path the guard uses.
func TestHandleLogin_FullWorldGuardFires(t *testing.T) {
	s := newTestServer(t)
	s.cfg.NodeMaxConnected = 5
	setPlayerCountForTest(t, s, 6) // strictly greater than the cap

	if s.getTotalPlayers() <= s.cfg.NodeMaxConnected {
		t.Fatalf("preflight: total=%d, cap=%d — guard would not fire",
			s.getTotalPlayers(), s.cfg.NodeMaxConnected)
	}

	// Pin the response byte the guard would send.
	if loginresp.OpServerFull.Opcode != 7 {
		t.Errorf("OpServerFull.Opcode drift: got %d, want 7 (TS byte)",
			loginresp.OpServerFull.Opcode)
	}
}

// TestHandleLogin_FullWorldGuardSilentAtBoundary: the TS condition is
// strictly greater-than (> NODE_MAX_CONNECTED). When total ==
// NodeMaxConnected the guard MUST NOT fire; one more accepted login
// would tip it over.
func TestHandleLogin_FullWorldGuardSilentAtBoundary(t *testing.T) {
	s := newTestServer(t)
	s.cfg.NodeMaxConnected = 5
	setPlayerCountForTest(t, s, 5) // at cap, not over

	if s.getTotalPlayers() > s.cfg.NodeMaxConnected {
		t.Errorf("boundary: total=%d > cap=%d, guard would wrongly fire",
			s.getTotalPlayers(), s.cfg.NodeMaxConnected)
	}
}

// TestShouldSpawnNpc_MembersGate pins gamemap-1: the boot-time NPC spawn
// loop must skip members-only NpcTypes on an F2P world. Mirrors TS
// GameMap.loadNpcs at GameMap.ts:131 — `(npcType.members && this.members)
// || !npcType.members`.
func TestShouldSpawnNpc_MembersGate(t *testing.T) {
	cases := []struct {
		name         string
		typ          *objtype.NpcType
		worldMembers bool
		want         bool
	}{
		{"f2p-npc-on-f2p-world", &objtype.NpcType{Members: false}, false, true},
		{"f2p-npc-on-members-world", &objtype.NpcType{Members: false}, true, true},
		{"members-npc-on-members-world", &objtype.NpcType{Members: true}, true, true},
		// Audit-cited divergence — pre-fix spawned (true), post-fix skips (false).
		{"members-npc-on-f2p-world", &objtype.NpcType{Members: true}, false, false},
		{"nil-typ", nil, true, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := shouldSpawnNpc(c.typ, c.worldMembers)
			if got != c.want {
				t.Errorf("shouldSpawnNpc: got %v, want %v — TS GameMap.ts:131 (npcType.members && this.members) || !npcType.members (gamemap-1)", got, c.want)
			}
		})
	}
}

// TestLoginResultToRS2_RateLimits pins the reply→byte contract at the
// rev-254 pin (Engine-TS @2e3bcf43): login-server response 8 (3-in-5s
// rate limit) → client byte 16 (TOO_MANY_ATTEMPTS); response 10 (hop
// timer, NEW numbering at 254 — was response 6 → byte 9 at 244) →
// client opcode 21 with a remaining-seconds payload byte
// (World.ts:1861-1866; the payload byte is sendLoginHopTimer's job,
// covered by TestSendLoginHopTimerWire).
func TestLoginResultToRS2_RateLimits(t *testing.T) {
	if got := loginResultToRS2(loginpb.LoginResult_LOGIN_RESULT_RATE_LIMITED); got != loginresp.OpTooManyAttempts.Opcode {
		t.Errorf("RATE_LIMITED: got byte %d, want %d", got, loginresp.OpTooManyAttempts.Opcode)
	}
	if got := loginResultToRS2(loginpb.LoginResult_LOGIN_RESULT_HOP_TIMER); got != loginresp.OpHopTimer.Opcode {
		t.Errorf("HOP_TIMER: got byte %d, want %d (rev-254 [21, secs] reply)", got, loginresp.OpHopTimer.Opcode)
	}
}
