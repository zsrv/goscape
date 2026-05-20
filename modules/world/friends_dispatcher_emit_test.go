package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/friendspb"
	io2 "github.com/zsrv/goscape/pkg/io/isaac"
	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
)

// TestEmitFriendsDispatcher_OnFriendlistUpdate_EnqueuesOnePacketPerEntry
// pins that the dispatcher closes over a single closure that emits one
// UPDATE_FRIENDLIST packet per FriendEntry — matching TS World.ts:1964-1966
// (for-loop over data.friends).
func TestEmitFriendsDispatcher_OnFriendlistUpdate_EnqueuesOnePacketPerEntry(t *testing.T) {
	p, cc := newTestPlayer(t)
	s := newTestServer(t)
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	const viewer uint64 = 0x1111222233334444
	p.username37 = viewer
	p.active = true
	s.playerLoop = append(s.playerLoop, p)

	d := newEmitFriendsDispatcher(s, s.log)
	received := drainConn(t, cc)

	d.OnFriendlistUpdate(viewer, []*friendspb.FriendEntry{
		{Username37: 0x0102030405060708, WorldId: 1},
		{Username37: 0xAABBCCDDEEFF0011, WorldId: 0},
	})
	s.drainRelayActions()
	p.client.flushWrite()

	got := <-received
	// Expect two UPDATE_FRIENDLIST packets back-to-back: each is
	// 1 opcode byte + 8 username37 + 1 worldId = 10 bytes.
	want := []byte{
		byte((int(gameserver.OpUpdateFriendList.Opcode) + int(enc.GetNext())) & 0xff),
		0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x01,
		byte((int(gameserver.OpUpdateFriendList.Opcode) + int(enc.GetNext())) & 0xff),
		0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF, 0x00, 0x11, 0x00,
	}
	if string(got) != string(want) {
		t.Fatalf("wire bytes: got % x, want % x", got, want)
	}
}

// TestEmitFriendsDispatcher_OnFriendlistUpdate_MissingPlayerNoEmit pins
// that the dispatcher silently drops events for a viewer not in s.playerLoop.
func TestEmitFriendsDispatcher_OnFriendlistUpdate_MissingPlayerNoEmit(t *testing.T) {
	s := newTestServer(t)
	d := newEmitFriendsDispatcher(s, s.log)

	// No players registered.
	d.OnFriendlistUpdate(0xDEADBEEF, []*friendspb.FriendEntry{{Username37: 1, WorldId: 1}})
	s.drainRelayActions() // closure runs, lookup returns nil, no-op.

	// No panic, no error — just verify the queue is now empty.
	select {
	case <-s.relayActionQueue:
		t.Fatal("relayActionQueue should be drained")
	default:
	}
}

// TestEmitFriendsDispatcher_OnFriendlistUpdate_LogoutBetweenEnqueueAndDrain
// pins that a player who logs out between event arrival and drain is
// silently skipped (no panic, no orphan write). Models logout by clearing
// p.active (lookupPlayerByUsername37 skips !p.active entries).
func TestEmitFriendsDispatcher_OnFriendlistUpdate_LogoutBetweenEnqueueAndDrain(t *testing.T) {
	p, _ := newTestPlayer(t)
	s := newTestServer(t)

	const viewer uint64 = 0xCAFE
	p.username37 = viewer
	p.active = true
	s.playerLoop = append(s.playerLoop, p)

	d := newEmitFriendsDispatcher(s, s.log)
	d.OnFriendlistUpdate(viewer, []*friendspb.FriendEntry{{Username37: 1, WorldId: 1}})

	// Player logs out before drain.
	p.active = false

	// Drain — closure runs, lookup returns nil, no-op.
	s.drainRelayActions()
}
