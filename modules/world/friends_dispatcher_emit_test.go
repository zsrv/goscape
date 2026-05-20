package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/friendspb"
	io2 "github.com/zsrv/goscape/pkg/io/isaac"
	"github.com/zsrv/goscape/pkg/io/packet"
	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
	"github.com/zsrv/goscape/pkg/wordenc/wordpack"
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

// TestEmitFriendsDispatcher_OnIgnorelistUpdate_EmitsSnapshot pins that
// the dispatcher emits one UPDATE_IGNORELIST packet carrying the full
// snapshot to the viewer's wire.
func TestEmitFriendsDispatcher_OnIgnorelistUpdate_EmitsSnapshot(t *testing.T) {
	p, cc := newTestPlayer(t)
	s := newTestServer(t)
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	const viewer uint64 = 0x1111
	p.username37 = viewer
	p.active = true
	s.playerLoop = append(s.playerLoop, p)

	d := newEmitFriendsDispatcher(s, s.log)
	received := drainConn(t, cc)

	d.OnIgnorelistUpdate(viewer, []uint64{0x0102030405060708, 0xAABBCCDDEEFF0011})
	s.drainRelayActions()
	p.client.flushWrite()

	got := <-received
	want := []byte{
		byte((int(gameserver.OpUpdateIgnoreList.Opcode) + int(enc.GetNext())) & 0xff),
		0x00, 0x10,
		0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
		0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF, 0x00, 0x11,
	}
	if string(got) != string(want) {
		t.Fatalf("wire bytes: got % x, want % x", got, want)
	}
}

// TestEmitFriendsDispatcher_OnPrivateMessage_EmitsPacket pins that the
// dispatcher emits one MESSAGE_PRIVATE packet to the recipient's wire
// matching the T2 encoder byte-pin.
func TestEmitFriendsDispatcher_OnPrivateMessage_EmitsPacket(t *testing.T) {
	p, cc := newTestPlayer(t)
	s := newTestServer(t)
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	const target uint64 = 0x4444
	p.username37 = target
	p.active = true
	s.playerLoop = append(s.playerLoop, p)

	// Compute the wordpacked bytes for "hi".
	wpBuf := packet.NewPacket(nil)
	wordpack.Pack(wpBuf, "hi")
	wpBytes := wpBuf.Bytes()

	d := newEmitFriendsDispatcher(s, s.log)
	received := drainConn(t, cc)

	d.OnPrivateMessage(target, 0x0102030405060708, 0, 0xDEADBEEF, "hi")
	s.drainRelayActions()
	p.client.flushWrite()

	got := <-received
	header := []byte{
		byte((int(gameserver.OpMessagePrivate.Opcode) + int(enc.GetNext())) & 0xff),
		byte(8 + 4 + 1 + len(wpBytes)),
		0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
		0xDE, 0xAD, 0xBE, 0xEF,
		0x00,
	}
	want := append(header, wpBytes...)
	if string(got) != string(want) {
		t.Fatalf("wire bytes: got % x, want % x", got, want)
	}
}

// TestEmitFriendsDispatcher_OnPrivateMessage_MissingTargetNoEmit pins
// that the dispatcher silently drops PMs for a target not in s.playerLoop
// (e.g., player logged out between sender's send and recipient's tick).
func TestEmitFriendsDispatcher_OnPrivateMessage_MissingTargetNoEmit(t *testing.T) {
	s := newTestServer(t)
	d := newEmitFriendsDispatcher(s, s.log)

	d.OnPrivateMessage(0xDEAD, 0xBEEF, 0, 0, "hi")
	s.drainRelayActions()

	select {
	case <-s.relayActionQueue:
		t.Fatal("relayActionQueue should be drained")
	default:
	}
}
