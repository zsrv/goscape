package world

import (
	"context"
	"testing"
	"time"

	"github.com/zsrv/goscape/pkg/friendspb"
	jstring "github.com/zsrv/goscape/pkg/util/jstring"
)

const testWorldID = 42

func TestGRPCFriendsBridge_AddFriend_FiresRPC(t *testing.T) {
	fake := newFakeFriendsClient()
	bridge := &grpcFriendsBridge{client: fake, worldID: testWorldID, log: discardLogger()}

	bridge.AddFriend("alice", 1234)

	select {
	case got := <-fake.friendlistAddReqs:
		if got.WorldId != testWorldID {
			t.Errorf("WorldId: got %d, want %d", got.WorldId, testWorldID)
		}
		if got.Username37 != jstring.ToBase37("alice") {
			t.Errorf("Username37: got %d, want ToBase37(alice)", got.Username37)
		}
		if got.TargetUsername37 != 1234 {
			t.Errorf("TargetUsername37: got %d, want 1234", got.TargetUsername37)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for FriendlistAdd RPC")
	}
}

func TestGRPCFriendsBridge_RemoveFriend_FiresRPC(t *testing.T) {
	fake := newFakeFriendsClient()
	bridge := &grpcFriendsBridge{client: fake, worldID: testWorldID, log: discardLogger()}

	bridge.RemoveFriend("alice", 1234)

	select {
	case got := <-fake.friendlistDelReqs:
		if got.WorldId != testWorldID || got.Username37 != jstring.ToBase37("alice") || got.TargetUsername37 != 1234 {
			t.Errorf("FriendlistDel record: %+v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for FriendlistDel RPC")
	}
}

func TestGRPCFriendsBridge_AddIgnore_FiresRPC(t *testing.T) {
	fake := newFakeFriendsClient()
	bridge := &grpcFriendsBridge{client: fake, worldID: testWorldID, log: discardLogger()}

	bridge.AddIgnore("alice", 1234)

	select {
	case got := <-fake.ignorelistAddReqs:
		if got.WorldId != testWorldID || got.Username37 != jstring.ToBase37("alice") || got.TargetUsername37 != 1234 {
			t.Errorf("IgnorelistAdd record: %+v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for IgnorelistAdd RPC")
	}
}

func TestGRPCFriendsBridge_RemoveIgnore_FiresRPC(t *testing.T) {
	fake := newFakeFriendsClient()
	bridge := &grpcFriendsBridge{client: fake, worldID: testWorldID, log: discardLogger()}

	bridge.RemoveIgnore("alice", 1234)

	select {
	case got := <-fake.ignorelistDelReqs:
		if got.WorldId != testWorldID || got.Username37 != jstring.ToBase37("alice") || got.TargetUsername37 != 1234 {
			t.Errorf("IgnorelistDel record: %+v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for IgnorelistDel RPC")
	}
}

func TestGRPCFriendsBridge_SetChatMode_FiresRPC(t *testing.T) {
	fake := newFakeFriendsClient()
	bridge := &grpcFriendsBridge{client: fake, worldID: testWorldID, log: discardLogger()}

	bridge.SetChatMode("alice", 2)

	select {
	case got := <-fake.chatSetModeReqs:
		if got.WorldId != testWorldID {
			t.Errorf("WorldId: got %d, want %d", got.WorldId, testWorldID)
		}
		if got.Username37 != jstring.ToBase37("alice") {
			t.Errorf("Username37: got %d, want ToBase37(alice)", got.Username37)
		}
		if got.PrivateChat != 2 {
			t.Errorf("PrivateChat: got %d, want 2", got.PrivateChat)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for ChatSetMode RPC")
	}
}

func TestGRPCFriendsBridge_PrivateMessage_FiresRPC(t *testing.T) {
	fake := newFakeFriendsClient()
	bridge := &grpcFriendsBridge{client: fake, worldID: testWorldID, log: discardLogger()}

	bridge.PrivateMessage("alice", 2, 0xDEADBEEF, 1234, "hi bob", 0xC0DE)

	select {
	case got := <-fake.privateMessageReqs:
		if got.WorldId != testWorldID {
			t.Errorf("WorldId: got %d, want %d", got.WorldId, testWorldID)
		}
		if got.Username37 != jstring.ToBase37("alice") {
			t.Errorf("Username37: got %d, want ToBase37(alice)", got.Username37)
		}
		if got.TargetUsername37 != 1234 {
			t.Errorf("TargetUsername37: got %d, want 1234", got.TargetUsername37)
		}
		if got.StaffLvl != 2 {
			t.Errorf("StaffLvl: got %d, want 2", got.StaffLvl)
		}
		if got.PmId != 0xDEADBEEF {
			t.Errorf("PmId: got %d, want 0xDEADBEEF", got.PmId)
		}
		if got.Chat != "hi bob" {
			t.Errorf("Chat: got %q, want %q", got.Chat, "hi bob")
		}
		if got.Coord != 0xC0DE {
			t.Errorf("Coord: got %d, want 0xC0DE", got.Coord)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for PrivateMessage RPC")
	}
}

// gatedFriendsClient embeds fakeFriendsClient but blocks FriendlistAdd
// on <-gate. Used to verify grpcFriendsBridge's goroutine fan-out: the
// synchronous bridge call must return before the underlying RPC
// completes.
type gatedFriendsClient struct {
	*fakeFriendsClient
	gate chan struct{}
	hit  chan struct{}
}

func (g *gatedFriendsClient) FriendlistAdd(ctx context.Context, req *friendspb.FriendlistAddRequest) {
	<-g.gate
	g.fakeFriendsClient.FriendlistAdd(ctx, req)
	close(g.hit)
}

func TestGRPCFriendsBridge_FireAndForget_DoesNotBlock(t *testing.T) {
	gate := make(chan struct{})
	gated := &gatedFriendsClient{
		fakeFriendsClient: newFakeFriendsClient(),
		gate:              gate,
		hit:               make(chan struct{}),
	}
	bridge := &grpcFriendsBridge{client: gated, worldID: testWorldID, log: discardLogger()}

	done := make(chan struct{})
	go func() {
		bridge.AddFriend("alice", 1234)
		close(done)
	}()

	select {
	case <-done:
		// expected: synchronous call returned before gate opened
	case <-time.After(100 * time.Millisecond):
		t.Fatal("AddFriend blocked on RPC despite go-fan-out")
	}

	close(gate)

	select {
	case <-gated.hit:
		// expected: after gate, underlying FriendlistAdd completed
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for gated FriendlistAdd to fire")
	}
}

func TestDefaultFriendsBridge_NonNilClient_ReturnsGRPCBridge(t *testing.T) {
	got := defaultFriendsBridge(newFakeFriendsClient(), testWorldID, discardLogger())
	b, ok := got.(*grpcFriendsBridge)
	if !ok {
		t.Fatalf("defaultFriendsBridge: got %T, want *grpcFriendsBridge", got)
	}
	if b.worldID != testWorldID {
		t.Errorf("worldID: got %d, want %d", b.worldID, testWorldID)
	}
}

func TestDefaultFriendsBridge_NilClient_ReturnsNoop(t *testing.T) {
	got := defaultFriendsBridge(nil, testWorldID, discardLogger())
	if _, ok := got.(noopBridges); !ok {
		t.Fatalf("defaultFriendsBridge: got %T, want noopBridges", got)
	}
}
