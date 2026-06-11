package world

import (
	"bytes"
	"io"
	"testing"

	io2 "github.com/zsrv/goscape/pkg/io/isaac"
	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
)

// TestSendUpdateRebootTimer_EmitsExactByteSequence pins the wire bytes of sendUpdateRebootTimer. NAI-182 B1.
func TestSendUpdateRebootTimer_EmitsExactByteSequence(t *testing.T) {
	p, cc := newTestPlayer(t)
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	want := []byte{
		byte((int(gameserver.OpUpdateRebootTimer.Opcode) + int(enc.GetNext())) & 0xff),
		0x00, 0x32,
	}

	received := drainConn(t, cc)
	sendUpdateRebootTimer(p, 50)
	p.client.flushWrite()

	got := <-received
	if !bytes.Equal(got, want) {
		t.Errorf("wire: got %v, want %v", got, want)
	}
}

// TestSendUpdateRebootTimer_ZeroTicks pins the wire bytes of sendUpdateRebootTimer with ticks=0. NAI-182 B1.
func TestSendUpdateRebootTimer_ZeroTicks(t *testing.T) {
	p, cc := newTestPlayer(t)
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	want := []byte{
		byte((int(gameserver.OpUpdateRebootTimer.Opcode) + int(enc.GetNext())) & 0xff),
		0x00, 0x00,
	}

	received := drainConn(t, cc)
	sendUpdateRebootTimer(p, 0)
	p.client.flushWrite()

	got := <-received
	if !bytes.Equal(got, want) {
		t.Errorf("wire: got %v, want %v", got, want)
	}
}

// TestNewServer_ShutdownTickDefaultsToMinusOne verifies that shutdownTick is -1 on construction. NAI-182 B2.
func TestNewServer_ShutdownTickDefaultsToMinusOne(t *testing.T) {
	s := newTestServer(t)
	if s.shutdownTick != -1 {
		t.Errorf("newServer: shutdownTick = %d, want -1", s.shutdownTick)
	}
}

// TestRebootTimer_SetsShutdownTickAndBroadcasts verifies that rebootTimer sets shutdownTick and sends wire bytes. NAI-182 B2.
func TestRebootTimer_SetsShutdownTickAndBroadcasts(t *testing.T) {
	p, cc := newTestPlayer(t)
	s := newTestServer(t)
	p.client.server = s
	p.slot = 1
	s.players.set(1, p)
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	startTick := s.currentTick
	received := drainConn(t, cc)
	s.rebootTimer(50)
	p.client.flushWrite()
	got := <-received

	if s.shutdownTick != startTick+50 {
		t.Errorf("shutdownTick after rebootTimer(50): got %d, want %d", s.shutdownTick, startTick+50)
	}

	want := []byte{
		byte((int(gameserver.OpUpdateRebootTimer.Opcode) + int(enc.GetNext())) & 0xff),
		0x00, 0x32,
	}
	if !bytes.Equal(got, want) {
		t.Errorf("broadcast bytes: got %#x, want %#x", got, want)
	}
}

// TestRebootTimer_DurationZero verifies that rebootTimer(0) sets shutdownTick=currentTick and sends 0x00 0x00. NAI-182 B2.
func TestRebootTimer_DurationZero(t *testing.T) {
	p, cc := newTestPlayer(t)
	s := newTestServer(t)
	p.client.server = s
	p.slot = 1
	s.players.set(1, p)
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	startTick := s.currentTick
	received := drainConn(t, cc)
	s.rebootTimer(0)
	p.client.flushWrite()
	got := <-received

	if s.shutdownTick != startTick {
		t.Errorf("shutdownTick after rebootTimer(0): got %d, want %d", s.shutdownTick, startTick)
	}

	want := []byte{
		byte((int(gameserver.OpUpdateRebootTimer.Opcode) + int(enc.GetNext())) & 0xff),
		0x00, 0x00,
	}
	if !bytes.Equal(got, want) {
		t.Errorf("broadcast bytes: got %#x, want %#x", got, want)
	}
}

// TestIsPendingShutdown_AndTicksRemaining verifies isPendingShutdown and shutdownTicksRemaining getters. NAI-182 B2.
func TestIsPendingShutdown_AndTicksRemaining(t *testing.T) {
	s := newTestServer(t)
	if s.isPendingShutdown() {
		t.Error("isPendingShutdown: pre-rebootTimer: got true, want false")
	}

	startTick := s.currentTick
	s.rebootTimer(50)

	if !s.isPendingShutdown() {
		t.Error("isPendingShutdown: post-rebootTimer(50): got false, want true")
	}
	if got := s.shutdownTicksRemaining(); got != 50 {
		t.Errorf("shutdownTicksRemaining: got %d, want 50", got)
	}

	s.currentTick = startTick + 10
	if got := s.shutdownTicksRemaining(); got != 40 {
		t.Errorf("shutdownTicksRemaining after +10 ticks: got %d, want 40", got)
	}
}

// TestProcessShutdown_MarksAllConnectedPlayersForLogout verifies that
// processShutdown sets loggingOut on every player with a live client. NAI-182 B5.
func TestProcessShutdown_MarksAllConnectedPlayersForLogout(t *testing.T) {
	p, cc := newTestPlayer(t)
	s := newTestServer(t)
	p.client.server = s
	p.slot = 1
	s.players.set(1, p)
	go io.Copy(io.Discard, cc)

	s.shutdownTick = s.currentTick
	s.processShutdown()

	if !p.loggingOut {
		t.Errorf("player slot=%d: loggingOut not set after processShutdown", p.slot)
	}
}

// TestProcessShutdown_ForceRemoveAfter1024Ticks verifies that processShutdown
// directly removes players that are still present after 1024 ticks. Mirrors TS
// World.processShutdown (World.ts:1207-1213), which calls removePlayer inline.
// NAI-182 B5 (re-pointed to the direct-removal contract per world-tick-1).
func TestProcessShutdown_ForceRemoveAfter1024Ticks(t *testing.T) {
	p, cc := newTestPlayer(t)
	s := newTestServer(t)
	p.client.server = s
	if err := s.addPlayer(p); err != nil {
		t.Fatalf("addPlayer: %v", err)
	}
	go io.Copy(io.Discard, cc)

	slot := p.slot
	p.loggingOut = true
	s.shutdownTick = s.currentTick - 1024

	s.processShutdown()

	if s.players.get(slot) != nil {
		t.Errorf("after 1024-tick processShutdown: s.players[%d] still set, want nil (player not removed)", slot)
	}
}

// TestProcessShutdown_ForceRemoveNotSetBeforeDuration verifies that the direct
// force-removal does NOT fire when fewer than 1024 ticks have elapsed: the
// player remains registered. NAI-182 B5 (re-pointed per world-tick-1).
func TestProcessShutdown_ForceRemoveNotSetBeforeDuration(t *testing.T) {
	p, cc := newTestPlayer(t)
	s := newTestServer(t)
	p.client.server = s
	if err := s.addPlayer(p); err != nil {
		t.Fatalf("addPlayer: %v", err)
	}
	go io.Copy(io.Discard, cc)

	slot := p.slot
	p.loggingOut = true
	s.shutdownTick = s.currentTick - 1023

	s.processShutdown()

	if s.players.get(slot) != p {
		t.Errorf("after 1023-tick processShutdown: s.players[%d] = %v, want player still present", slot, s.players.get(slot))
	}
}

// TestProcessShutdown_ZeroPlayersTriggersGracefulExit verifies that processShutdown
// sets shutdownGraceful and closes gracefulExit when no players are connected. NAI-182 B5.
func TestProcessShutdown_ZeroPlayersTriggersGracefulExit(t *testing.T) {
	s := newTestServer(t)
	s.shutdownTick = s.currentTick

	s.processShutdown()

	if !s.shutdownGraceful {
		t.Error("shutdownGraceful: got false, want true after zero-player processShutdown")
	}
	select {
	case <-s.gracefulExit:
		// pass
	default:
		t.Error("gracefulExit channel: not closed after zero-player processShutdown")
	}
}

// TestProcessShutdown_AcceleratesTickRateAfterDuration2 pins world-tick-4.
// TS World.processShutdown (World.ts:1222-1225) sets `this.tickRate = 0`
// once `duration > 2`, so the remaining shutdown ticks drain at the
// fastest possible rate (reach the 1024-tick force-removal deadline in
// seconds instead of ~10 minutes). goscape never mutated s.tickRate in
// processShutdown — shutdown drain ran at the normal 600ms rate.
func TestProcessShutdown_AcceleratesTickRateAfterDuration2(t *testing.T) {
	s := newTestServer(t)
	// duration = currentTick - shutdownTick. currentTick defaults to 0;
	// set shutdownTick=-3 so duration=3 (> 2). Keeps the player force-
	// removal branch (duration >= 1024) off the path.
	s.shutdownTick = -3
	// Seed one player so the graceful-exit branch (getTotalPlayers()==0)
	// doesn't fire before the tickRate=0 line — TS sets tickRate AFTER
	// the online==0 check, so a zero-player world graceful-exits without
	// touching tickRate.
	s.players.set(1, &Player{slot: 1})

	if s.tickRate != defaultTickRate {
		t.Fatalf("precondition: tickRate=%v, want defaultTickRate (%v)", s.tickRate, defaultTickRate)
	}

	s.processShutdown()

	if s.tickRate != 0 {
		t.Errorf("TS World.processShutdown (World.ts:1222-1225) sets tickRate=0 once duration>2; got %v, want 0", s.tickRate)
	}
}

// TestProcessShutdown_LeavesTickRateAloneWithinDuration2 verifies the
// inverse of world-tick-4: when `duration <= 2`, tickRate must NOT be
// mutated (TS gates the acceleration on `duration > 2`).
func TestProcessShutdown_LeavesTickRateAloneWithinDuration2(t *testing.T) {
	s := newTestServer(t)
	// duration = 2 — boundary; TS uses `>` so this stays at normal rate.
	s.shutdownTick = -2
	s.players.set(1, &Player{slot: 1})

	s.processShutdown()

	if s.tickRate != defaultTickRate {
		t.Errorf("TS World.processShutdown gates tickRate=0 on duration>2; duration=2 must leave tickRate at %v, got %v", defaultTickRate, s.tickRate)
	}
}

// TestProcessShutdown_ForceRemovesStuckPlayerAfter1024 verifies that once the
// shutdown duration reaches 1024 ticks, processShutdown force-removes EVERY
// remaining player directly — even one that fails the normal logout gate
// (here !CanAccess() via p.delayed). Mirrors TS World.processShutdown
// (World.ts:1207-1213), which calls this.removePlayer(player) unconditionally.
// Pins finding world-tick-1.
func TestProcessShutdown_ForceRemovesStuckPlayerAfter1024(t *testing.T) {
	p, cc := newTestPlayer(t)
	s := newTestServer(t)
	p.client.server = s
	// Register the player in a real slot so removal is observable
	// (newPlayer leaves slot=-1, which removePlayerInternal's slot guard
	// would skip). addPlayer assigns a slot via nextSlot and
	// calls s.players.add(slot, key, p).
	if err := s.addPlayer(p); err != nil {
		t.Fatalf("addPlayer: %v", err)
	}
	go io.Copy(io.Discard, cc)

	slot := p.slot

	// Make the player fail processLogouts' inner gate: !CanAccess(). This is
	// exactly the stuck-player case that the buggy forceRemove path never
	// evicted, hanging shutdown forever.
	p.delayed = true
	if p.CanAccess() {
		t.Fatal("precondition: player should not have access (delayed=true)")
	}

	p.loggingOut = true
	s.shutdownTick = s.currentTick - 1024

	s.processShutdown()

	if s.players.get(slot) != nil {
		t.Errorf("after 1024-tick processShutdown: s.players[%d] still set, want nil (player not force removed)", slot)
	}
	for lp := range s.players.all() {
		if lp == p {
			t.Errorf("after 1024-tick processShutdown: player still in players, want removed")
		}
	}
}
