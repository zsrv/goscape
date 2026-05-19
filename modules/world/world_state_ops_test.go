package world

import (
	"io"
	"log/slog"
	"strings"
	"sync/atomic"
	"testing"

	io2 "github.com/zsrv/goscape/pkg/io/isaac"
	jstring "github.com/zsrv/goscape/pkg/util/jstring"
)

// Compile-time assertion that *Server implements WorldStateOps.
// Until the WorldStateOps methods are implemented in T3-T4, this line
// must compile against the interface declared in T2 — *Server has all
// methods → compiles. The runtime test below ensures *Server can be
// constructed and bound through the interface.
var _ WorldStateOps = (*Server)(nil)

// TestWorldStateOps_InterfaceBindsToServer pins that *Server satisfies
// WorldStateOps at construction time. Method behavior is verified in
// the per-method tests below.
func TestWorldStateOps_InterfaceBindsToServer(t *testing.T) {
	s := newTestServer(t)
	var ops WorldStateOps = s
	if ops == nil {
		t.Fatal("WorldStateOps binding returned nil")
	}
}

// TestRelayActionQueue_DrainExecutesOnTick pins that an action enqueued
// via enqueueRelayAction runs exactly once when drainRelayActions is
// invoked, and runs on the caller's goroutine (tick semantics).
func TestRelayActionQueue_DrainExecutesOnTick(t *testing.T) {
	s := newTestServer(t)

	var ran atomic.Int32
	s.enqueueRelayAction(func() { ran.Add(1) })

	if ran.Load() != 0 {
		t.Fatalf("action ran before drain: count=%d", ran.Load())
	}

	s.drainRelayActions()

	if got := ran.Load(); got != 1 {
		t.Fatalf("action did not run on drain: count=%d, want 1", got)
	}

	// Second drain with empty queue must be a no-op (no blocking).
	s.drainRelayActions()
	if got := ran.Load(); got != 1 {
		t.Fatalf("second drain re-ran action: count=%d, want 1", got)
	}
}

// TestRelayActionQueue_DropsOnFull pins that enqueueRelayAction is
// non-blocking and drops the action when the queue is at capacity.
// Mirrors slice-4a NAI-S4A-D-DROP-ON-FULL posture (drop-newest).
func TestRelayActionQueue_DropsOnFull(t *testing.T) {
	s := newTestServer(t)

	// Fill the queue to capacity with no-op closures.
	for i := 0; i < cap(s.relayActionQueue); i++ {
		s.enqueueRelayAction(func() {})
	}

	// The next enqueue must NOT block. If the implementation blocks,
	// the test will hang and fail on test timeout.
	var dropped atomic.Bool
	dropped.Store(true) // assume dropped; flipped to false if executed.
	s.enqueueRelayAction(func() { dropped.Store(false) })

	// Drain everything; only the first cap(queue) closures should run.
	// The over-cap closure was dropped, so dropped stays true.
	s.drainRelayActions()

	if dropped.Load() {
		// Got dropped — correct behavior.
		return
	}
	t.Fatal("over-cap enqueue was NOT dropped — drainRelayActions executed the over-cap closure")
}

// registerActivePlayer wires a Player into s.players + s.playerLoop
// with active=true, mirroring teleTestPlayer/addOtherTestPlayer minus
// the gamemap/zoneMap setup that WorldStateOps tests don't need. The
// returned *Player has username, username37, and a working encryptor
// so writeOut paths don't NPE. The test conn is drained to /dev/null
// so packet writes don't block in the bufio.Writer.
//
// Plan deviation: the plan called `s.addPlayer(p)`, which requires
// p.username37 to be set up correctly AND drives zoneMap.EnterPlayer.
// We bypass it to keep tests focused — the registration shape matches
// the slice-5b T3/T4 contract: lookupPlayerByUsername37 iterates
// playerLoop and matches on jstring.ToBase37(p.username).
func registerActivePlayer(t *testing.T, s *Server, username string, slot int) *Player {
	t.Helper()
	c, conn := newTestClient(t)
	go io.Copy(io.Discard, conn)
	c.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	p := newPlayer(c)
	p.client.server = s
	p.username = username
	p.username37 = jstring.ToBase37(username)
	p.slot = slot
	s.players[slot] = p
	s.playerLoop = append(s.playerLoop, p)
	p.active = true
	return p
}

// TestWorldStateOps_Shutdown_AdvancesShutdownTick pins that
// RelayShutdown(d) enqueues a closure that, when drained, calls
// rebootTimer(d) — which sets shutdownTick = currentTick + d.
func TestWorldStateOps_Shutdown_AdvancesShutdownTick(t *testing.T) {
	s := newTestServer(t)
	s.currentTick = 100

	var ops WorldStateOps = s
	ops.RelayShutdown(50)
	s.drainRelayActions()

	if s.shutdownTick != 150 {
		t.Fatalf("shutdownTick: got %d, want 150 (currentTick=100 + duration=50)", s.shutdownTick)
	}
}

// TestWorldStateOps_Reload_EnqueuesRebuildReq pins that
// RelayReload() enqueues a closure that, when drained, posts on
// rebuildReq via dispatchRebuildRequest (existing helper).
func TestWorldStateOps_Reload_EnqueuesRebuildReq(t *testing.T) {
	s := newTestServer(t)

	var ops WorldStateOps = s
	ops.RelayReload()
	s.drainRelayActions()

	select {
	case <-s.rebuildReq:
		// expected
	default:
		t.Fatal("rebuildReq did not receive after RelayReload + drain")
	}
}

// TestWorldStateOps_ClearLogins_EmptiesNewPlayers pins that
// ClearLogins() enqueues a closure that clears s.newPlayers.
func TestWorldStateOps_ClearLogins_EmptiesNewPlayers(t *testing.T) {
	s := newTestServer(t)
	// Seed two pending logins. Bypass addPlayer (which requires a wired
	// client) — directly populate the slice under playersMu.
	s.playersMu.Lock()
	s.newPlayers = append(s.newPlayers, &Player{}, &Player{})
	s.playersMu.Unlock()

	var ops WorldStateOps = s
	ops.ClearLogins()
	s.drainRelayActions()

	s.playersMu.RLock()
	got := len(s.newPlayers)
	s.playersMu.RUnlock()
	if got != 0 {
		t.Fatalf("newPlayers len after ClearLogins + drain: got %d, want 0", got)
	}
}

// TestWorldStateOps_ClearLogouts_IsTaggedNoop pins that
// ClearLogouts() runs without panic and emits a single Info log line
// referencing the no-op. NAI-S5B-D-CLEARLOGOUTS-NO-GOSCAPE-QUEUE.
func TestWorldStateOps_ClearLogouts_IsTaggedNoop(t *testing.T) {
	s := newTestServer(t)
	buf := &syncBuffer{}
	s.log = slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelInfo}))

	var ops WorldStateOps = s
	ops.ClearLogouts()
	s.drainRelayActions()

	if !strings.Contains(buf.String(), "RELAY_CLEARLOGOUTS") {
		t.Fatalf("expected ClearLogouts Info log; got: %s", buf.String())
	}
}

// TestWorldStateOps_BroadcastMessage_FansOutToPlayers pins that
// BroadcastMessage(m) enqueues a closure that calls BroadcastMes(m).
// Verified by counting bytes written to the player's bufw — every
// connected player gets a MESSAGE_GAME frame.
func TestWorldStateOps_BroadcastMessage_FansOutToPlayers(t *testing.T) {
	s := newTestServer(t)
	p := registerActivePlayer(t, s, "alice", 1)

	var ops WorldStateOps = s
	ops.BroadcastMessage("hello world")
	s.drainRelayActions()

	// MessageGame appends a MESSAGE_GAME opcode + payload to the
	// player's 64k bufio.Writer. writeOut does NOT flush, and an
	// 11-byte payload cannot overflow a 64k buffer, so Buffered() is
	// guaranteed to be non-zero. A zero reading would mean BroadcastMes
	// did not reach our player — a contract violation, not a skip.
	if p.client.bufw.Buffered() == 0 {
		t.Fatal("bufw unexpectedly empty after BroadcastMessage+drain")
	}
}

// TestWorldStateOps_SetPlayerMute_SetsMutedUntil pins that
// SetPlayerMute(u37, ms) flips p.mutedUntil on the looked-up player.
func TestWorldStateOps_SetPlayerMute_SetsMutedUntil(t *testing.T) {
	s := newTestServer(t)
	p := registerActivePlayer(t, s, "alice", 1)

	u37 := jstring.ToBase37("alice")
	wantMs := int64(1700000000000)

	var ops WorldStateOps = s
	ops.SetPlayerMute(u37, wantMs)
	s.drainRelayActions()

	if got := p.mutedUntil.UnixMilli(); got != wantMs {
		t.Fatalf("mutedUntil: got %d ms, want %d ms", got, wantMs)
	}
}

// TestWorldStateOps_SetPlayerMute_LookupMissIsHarmless pins that
// SetPlayerMute against an offline player is a no-op (Debug log only,
// no panic).
func TestWorldStateOps_SetPlayerMute_LookupMissIsHarmless(t *testing.T) {
	s := newTestServer(t)
	u37 := jstring.ToBase37("ghost")

	var ops WorldStateOps = s
	ops.SetPlayerMute(u37, 999)
	s.drainRelayActions()
	// No panic = pass.
}

// TestWorldStateOps_KickPlayer_FlipsLoggingOut mirrors the existing
// ::kick cheat assertion (handler_cheats_supermod_test.go:401). After
// dispatch + drain, p.loggingOut must be true. Teardown deferred to
// processLogouts (NAI-186-D1).
func TestWorldStateOps_KickPlayer_FlipsLoggingOut(t *testing.T) {
	s := newTestServer(t)
	p := registerActivePlayer(t, s, "bob", 1)

	if p.loggingOut {
		t.Fatal("preflight: p.loggingOut should be false before kick")
	}

	u37 := jstring.ToBase37("bob")
	var ops WorldStateOps = s
	ops.KickPlayer(u37)
	s.drainRelayActions()

	if !p.loggingOut {
		t.Fatal("p.loggingOut: must be true after KickPlayer + drain")
	}
}

// TestWorldStateOps_SetPlayerInputTracking_FlipsSubmitInput pins that
// SetPlayerInputTracking(u37, 1) flips p.submitInput=true and
// SetPlayerInputTracking(u37, 0) flips it back to false. Mirrors TS
// Player.submitInput = state at World.ts:2033.
func TestWorldStateOps_SetPlayerInputTracking_FlipsSubmitInput(t *testing.T) {
	s := newTestServer(t)
	p := registerActivePlayer(t, s, "carol", 1)

	u37 := jstring.ToBase37("carol")
	var ops WorldStateOps = s

	ops.SetPlayerInputTracking(u37, 1)
	s.drainRelayActions()
	if !p.submitInput {
		t.Fatal("submitInput: must be true after SetPlayerInputTracking(1)")
	}

	ops.SetPlayerInputTracking(u37, 0)
	s.drainRelayActions()
	if p.submitInput {
		t.Fatal("submitInput: must be false after SetPlayerInputTracking(0)")
	}
}
