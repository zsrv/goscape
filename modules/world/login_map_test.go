package world

import (
	"io"
	"testing"
	"time"

	"github.com/zsrv/goscape/pkg/gamemap"
	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
)

// TestLoginSendsRebuildNormal verifies that rebuildNormal() sends a RebuildNormal
// packet on first call (when p.originX is -1, the initial newPlayer sentinel).
func TestLoginSendsRebuildNormal(t *testing.T) {
	s := newTestServer(t)
	s.gamemap = gamemap.New(discardLogger())
	if err := s.gamemap.Init(t.TempDir()); err != nil {
		t.Fatal(err)
	}

	p, clientConn := newTestPlayer(t)
	p.client.server = s
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	wantEnc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	p.client.encryptor = enc

	received := make(chan byte, 1)
	go func() {
		buf := make([]byte, 1)
		clientConn.SetReadDeadline(time.Now().Add(time.Second))
		if _, err := io.ReadFull(clientConn, buf); err == nil {
			received <- buf[0]
		}
	}()

	p.buildArea.rebuildNormal()
	p.client.flushWrite()

	wantOp := byte((int(gameserver.OpRebuildNormal.Opcode) + int(wantEnc.GetNext())) & 0xff)

	select {
	case got := <-received:
		if got != wantOp {
			t.Errorf("first byte: got %d, want %d (encrypted RebuildNormal opcode)", got, wantOp)
		}
	case <-time.After(time.Second):
		t.Error("timed out waiting for RebuildNormal")
	}
}

// TestRebuildNormalAnchorsOriginToPlayer verifies that rebuildNormal()'s rebuild
// path refreshes p.originX/Z to match the player's current position.
// Without this, a subsequent teleport-triggered PlayerInfo tele block
// would compute localX relative to the stale origin and overflow the
// 7-bit PBit(7, localX) encoding — visible on the Java client as the
// player landing at the wrong local-scene position after far teleport.
func TestRebuildNormalAnchorsOriginToPlayer(t *testing.T) {
	s := newTestServer(t)
	s.gamemap = gamemap.New(discardLogger())
	if err := s.gamemap.Init(t.TempDir()); err != nil {
		t.Fatal(err)
	}

	p, _ := newTestPlayer(t)
	p.client.server = s
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	p.client.encryptor = enc

	// Seed: player at login position, origin matches (as processLogins sets).
	p.x, p.z, p.level = 3094, 3106, 0
	p.originX, p.originZ = 3094, 3106

	// First rebuild is a no-op on origin (already matches).
	p.buildArea.rebuildNormal()
	p.client.flushWrite()
	if p.originX != 3094 || p.originZ != 3106 {
		t.Errorf("initial rebuild: originX/Z = (%d, %d), want (3094, 3106)",
			p.originX, p.originZ)
	}

	// Simulate a far teleport — player's coord jumps but p.originX/Z is
	// NOT pre-updated by the teleport handler (TeleJump only writes
	// p.x/z/level and the tele/jump flags). ShouldRebuild will trigger
	// because the new position is outside the 13x13 reload window.
	p.x, p.z = 5000, 5000
	p.reconnecting = false

	p.buildArea.rebuildNormal()
	p.client.flushWrite()

	if p.originX != 5000 || p.originZ != 5000 {
		t.Errorf("after teleport rebuild: originX/Z = (%d, %d), want (5000, 5000)",
			p.originX, p.originZ)
	}
}
