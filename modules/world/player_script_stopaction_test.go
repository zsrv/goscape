package world

import (
	"testing"

	io2 "github.com/zsrv/goscape/pkg/io/isaac"
	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
)

// TestStopAction_ClearsWaypointAndEmitsUnsetMapFlag pins player-script-1:
// TS Player.stopAction (Player.ts:944-947) calls clearPendingAction() AND
// unsetMapFlag(); goscape's StopAction omitted unsetMapFlag entirely, so
// the walk queue (waypointIndex) was preserved across StopAction AND no
// OpUnsetMapFlag packet was sent. Sibling-shaped to
// TestPlayer_UnsetMapFlag_ClearsWaypointAndEmitsPacket
// (player_post_decode_test.go:66) — same drain/encrypt mechanics.
func TestStopAction_ClearsWaypointAndEmitsUnsetMapFlag(t *testing.T) {
	p, cc := newTestPlayer(t)
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	p.waypointIndex = 5

	// Sibling decoder seeded from same key to compare the encrypted opcode.
	sibling := io2.New([4]uint32{1, 2, 3, 4})

	// Start drain BEFORE the action; drainConn requires this ordering.
	received := drainConn(t, cc)
	p.StopAction()
	p.client.flushWrite()
	emitted := <-received

	if p.waypointIndex != -1 {
		t.Errorf("waypointIndex: got %d, want -1 (TS Player.ts:2169 clearWaypoints arm of unsetMapFlag)", p.waypointIndex)
	}
	if len(emitted) == 0 {
		t.Fatalf("no bytes emitted; expected OpUnsetMapFlag (opcode %d) per TS Player.ts:2171", gameserver.OpUnsetMapFlag.Opcode)
	}
	wantEnc := byte((int(gameserver.OpUnsetMapFlag.Opcode) + int(sibling.GetNext())) & 0xff)
	if emitted[0] != wantEnc {
		t.Errorf("first emitted byte: got %d, want %d (encrypted OpUnsetMapFlag)", emitted[0], wantEnc)
	}
}
