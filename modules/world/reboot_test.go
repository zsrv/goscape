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
	s.playerLoop = append(s.playerLoop, p)
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
	s.playerLoop = append(s.playerLoop, p)
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
	s.playerLoop = append(s.playerLoop, p)
	go io.Copy(io.Discard, cc)

	s.shutdownTick = s.currentTick
	s.processShutdown()

	if !p.loggingOut {
		t.Errorf("player slot=%d: loggingOut not set after processShutdown", p.slot)
	}
}

// TestProcessShutdown_ForceRemoveAfter1024Ticks verifies that processShutdown
// sets forceRemove on players that are still present after 1024 ticks. NAI-182 B5.
func TestProcessShutdown_ForceRemoveAfter1024Ticks(t *testing.T) {
	p, cc := newTestPlayer(t)
	s := newTestServer(t)
	p.client.server = s
	s.playerLoop = append(s.playerLoop, p)
	go io.Copy(io.Discard, cc)

	p.loggingOut = true
	s.shutdownTick = s.currentTick - 1024

	s.processShutdown()

	if !p.forceRemove {
		t.Errorf("p.forceRemove after 1024-tick processShutdown: got false, want true")
	}
}

// TestProcessShutdown_ForceRemoveNotSetBeforeDuration verifies that forceRemove
// is NOT set when fewer than 1024 ticks have elapsed. NAI-182 B5.
func TestProcessShutdown_ForceRemoveNotSetBeforeDuration(t *testing.T) {
	p, cc := newTestPlayer(t)
	s := newTestServer(t)
	p.client.server = s
	s.playerLoop = append(s.playerLoop, p)
	go io.Copy(io.Discard, cc)

	p.loggingOut = true
	s.shutdownTick = s.currentTick - 1023

	s.processShutdown()

	if p.forceRemove {
		t.Errorf("p.forceRemove after 1023-tick processShutdown: got true, want false")
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
