package world

import (
	"errors"
	"io"
	"log/slog"
	"net"
	"testing"
	"time"

	io2 "github.com/zsrv/goscape/pkg/io/isaac"
	"github.com/zsrv/goscape/pkg/io/packet"
	gameclient "github.com/zsrv/goscape/pkg/io/protocol/game/client"
	loginresp "github.com/zsrv/goscape/pkg/io/protocol/login/resp"
	"github.com/zsrv/goscape/pkg/objtype"
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
	t.Cleanup(func() { c.in.Release() })
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
	s := &Server{
		quit:           make(chan interface{}),
		log:            discardLogger(),
		scriptProvider: defaultTestProvider(),
		zoneMap:        zone.NewZoneMap(),
		locObjTracker:  newLocObjTracker(),
		rsbuf:          rsbuf.New(),
		pmCount:        1,
		shutdownTick:   -1,
		tickRate:       defaultTickRate,
		gracefulExit:   make(chan struct{}),
		rebuildReq:    make(chan struct{}, 1),
		rebuildResult: make(chan rebuildResult, 1),
		relayActionQueue: make(chan func(), 64),
	}
	s.friendsBridge = noopBridges{}
	s.loginBridgeMod = noopBridges{}
	s.loggerBridge = noopBridges{}
	s.locOps = &serverLocOps{s: s}
	s.reloadFn = s.Reload
	s.watchSessionFn = s.runWatchSession
	// packFn intentionally left nil; tests that exercise the worker set it explicitly.
	// Inject a passthrough filter so test paths that call s.wordenc.Filter do
	// not require a real wordenc jagfile in the test cache.
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

	s.removePlayerInternal(p)

	if s.players[slot] != nil {
		t.Error("players[slot] should be nil after remove")
	}
	if len(s.playerLoop) != 0 {
		t.Errorf("playerLoop len: got %d, want 0", len(s.playerLoop))
	}
}

func TestAddPlayerWorldFull(t *testing.T) {
	s := newTestServer(t)

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
		_ = len(s.playerLoop)
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
	inLoop := len(s.playerLoop)
	s.playersMu.RUnlock()

	if queued != 0 {
		t.Errorf("newPlayers: got %d, want 0", queued)
	}
	if inLoop != 1 {
		t.Errorf("playerLoop: got %d, want 1", inLoop)
	}
	if p.slot < 1 {
		t.Errorf("slot: got %d, want >= 1", p.slot)
	}
}

func TestProcessLoginsWorldFullRejectsCleanly(t *testing.T) {
	s := newTestServer(t)

	for i := 1; i < len(s.players); i++ {
		s.players[i] = &Player{slot: i}
		s.playerLoop = append(s.playerLoop, s.players[i])
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
	s.playersMu.RLock()
	still := len(s.playerLoop)
	s.playersMu.RUnlock()
	if still != 0 {
		t.Errorf("playerLoop should be empty after logout, got %d", still)
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
		c.in.Release()
		putBufioReader64k(c.bufr)
		putBufioWriter64k(c.bufw)
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

// TestLookupPlayerByUIDSkipsInactive: an entry in playerLoop whose
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
// slots of s.players with non-nil placeholder entries, zeroing the rest.
// s.players is a fixed-size [2048]*Player array, so we iterate-and-assign
// rather than reassigning the field.
func setPlayerCountForTest(t *testing.T, s *Server, playerCount int) {
	t.Helper()
	for i := range s.players {
		if i < playerCount {
			s.players[i] = &Player{}
		} else {
			s.players[i] = nil
		}
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
	s := &Server{}
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

// TestLookupPlayerBySlot_Found returns the player at the slot.
func TestLookupPlayerBySlot_Found(t *testing.T) {
	s := newTestServer(t)
	p, _ := newTestPlayer(t)
	slot := 5
	s.players[slot] = p
	t.Cleanup(func() { s.players[slot] = nil })

	got := s.LookupPlayerBySlot(slot)
	if got != p {
		t.Errorf("LookupPlayerBySlot(%d): got %v, want %p", slot, got, p)
	}
}

// TestLookupPlayerBySlot_OutOfRange returns nil for indices outside
// [0, len(s.players)).
func TestLookupPlayerBySlot_OutOfRange(t *testing.T) {
	s := newTestServer(t)
	if got := s.LookupPlayerBySlot(-1); got != nil {
		t.Errorf("LookupPlayerBySlot(-1): got %v, want nil", got)
	}
	if got := s.LookupPlayerBySlot(len(s.players)); got != nil {
		t.Errorf("LookupPlayerBySlot(len): got %v, want nil", got)
	}
	if got := s.LookupPlayerBySlot(len(s.players) + 100); got != nil {
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
