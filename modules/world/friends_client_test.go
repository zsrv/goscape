package world

import (
	"context"
	"errors"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

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

// mockFriendsPBClient embeds friendspb.FriendsServiceClient so we can override
// individual methods. The embedded nil-interface gives no default implementation
// for non-overridden methods — calling them would nil-deref. The table-driven
// test below routes each test case through exactly one overridden method.
type mockFriendsPBClient struct {
	friendspb.FriendsServiceClient
	worldConnectFn   func(ctx context.Context, in *friendspb.WorldConnectRequest, opts ...grpc.CallOption) (*emptypb.Empty, error)
	playerLoginFn    func(ctx context.Context, in *friendspb.PlayerLoginRequest, opts ...grpc.CallOption) (*friendspb.PlayerLoginResponse, error)
	playerLogoutFn   func(ctx context.Context, in *friendspb.PlayerLogoutRequest, opts ...grpc.CallOption) (*emptypb.Empty, error)
	chatSetModeFn    func(ctx context.Context, in *friendspb.ChatSetModeRequest, opts ...grpc.CallOption) (*emptypb.Empty, error)
	friendlistAddFn  func(ctx context.Context, in *friendspb.FriendlistAddRequest, opts ...grpc.CallOption) (*emptypb.Empty, error)
	friendlistDelFn  func(ctx context.Context, in *friendspb.FriendlistDelRequest, opts ...grpc.CallOption) (*emptypb.Empty, error)
	ignorelistAddFn  func(ctx context.Context, in *friendspb.IgnorelistAddRequest, opts ...grpc.CallOption) (*emptypb.Empty, error)
	ignorelistDelFn  func(ctx context.Context, in *friendspb.IgnorelistDelRequest, opts ...grpc.CallOption) (*emptypb.Empty, error)
	privateMessageFn   func(ctx context.Context, in *friendspb.PrivateMessageRequest, opts ...grpc.CallOption) (*emptypb.Empty, error)
	subscribeUpdatesFn func(ctx context.Context, in *friendspb.SubscribeUpdatesRequest, opts ...grpc.CallOption) (friendspb.FriendsService_SubscribeUpdatesClient, error)
}

func (m *mockFriendsPBClient) WorldConnect(ctx context.Context, in *friendspb.WorldConnectRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	return m.worldConnectFn(ctx, in, opts...)
}
func (m *mockFriendsPBClient) PlayerLogin(ctx context.Context, in *friendspb.PlayerLoginRequest, opts ...grpc.CallOption) (*friendspb.PlayerLoginResponse, error) {
	return m.playerLoginFn(ctx, in, opts...)
}
func (m *mockFriendsPBClient) PlayerLogout(ctx context.Context, in *friendspb.PlayerLogoutRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	return m.playerLogoutFn(ctx, in, opts...)
}
func (m *mockFriendsPBClient) ChatSetMode(ctx context.Context, in *friendspb.ChatSetModeRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	return m.chatSetModeFn(ctx, in, opts...)
}
func (m *mockFriendsPBClient) FriendlistAdd(ctx context.Context, in *friendspb.FriendlistAddRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	return m.friendlistAddFn(ctx, in, opts...)
}
func (m *mockFriendsPBClient) FriendlistDel(ctx context.Context, in *friendspb.FriendlistDelRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	return m.friendlistDelFn(ctx, in, opts...)
}
func (m *mockFriendsPBClient) IgnorelistAdd(ctx context.Context, in *friendspb.IgnorelistAddRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	return m.ignorelistAddFn(ctx, in, opts...)
}
func (m *mockFriendsPBClient) IgnorelistDel(ctx context.Context, in *friendspb.IgnorelistDelRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	return m.ignorelistDelFn(ctx, in, opts...)
}
func (m *mockFriendsPBClient) PrivateMessage(ctx context.Context, in *friendspb.PrivateMessageRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	return m.privateMessageFn(ctx, in, opts...)
}
func (m *mockFriendsPBClient) SubscribeUpdates(ctx context.Context, in *friendspb.SubscribeUpdatesRequest, opts ...grpc.CallOption) (friendspb.FriendsService_SubscribeUpdatesClient, error) {
	if m.subscribeUpdatesFn != nil {
		return m.subscribeUpdatesFn(ctx, in, opts...)
	}
	return nil, nil
}

// TestGRPCFriendsClient_LogsErrorOnFailure verifies that every fire-and-forget
// RPC method on grpcFriendsClient logs warn + swallows the error when the
// underlying gRPC client returns an error. Uses a discard logger so the
// test asserts on the swallow contract: the method returns normally
// (no panic, no propagation) even though the RPC errored.
func TestGRPCFriendsClient_LogsErrorOnFailure(t *testing.T) {
	rpcErr := errors.New("simulated RPC failure")

	cases := []struct {
		name string
		call func(c *grpcFriendsClient)
	}{
		{"WorldConnect", func(c *grpcFriendsClient) {
			c.WorldConnect(context.Background(), 10, "main")
		}},
		{"PlayerLogin", func(c *grpcFriendsClient) {
			c.PlayerLogin(context.Background(), &friendspb.PlayerLoginRequest{Username37: 1}, nil)
		}},
		{"PlayerLogout", func(c *grpcFriendsClient) {
			c.PlayerLogout(context.Background(), &friendspb.PlayerLogoutRequest{Username37: 1})
		}},
		{"ChatSetMode", func(c *grpcFriendsClient) {
			c.ChatSetMode(context.Background(), &friendspb.ChatSetModeRequest{Username37: 1})
		}},
		{"FriendlistAdd", func(c *grpcFriendsClient) {
			c.FriendlistAdd(context.Background(), &friendspb.FriendlistAddRequest{Username37: 1})
		}},
		{"FriendlistDel", func(c *grpcFriendsClient) {
			c.FriendlistDel(context.Background(), &friendspb.FriendlistDelRequest{Username37: 1})
		}},
		{"IgnorelistAdd", func(c *grpcFriendsClient) {
			c.IgnorelistAdd(context.Background(), &friendspb.IgnorelistAddRequest{Username37: 1})
		}},
		{"IgnorelistDel", func(c *grpcFriendsClient) {
			c.IgnorelistDel(context.Background(), &friendspb.IgnorelistDelRequest{Username37: 1})
		}},
		{"PrivateMessage", func(c *grpcFriendsClient) {
			c.PrivateMessage(context.Background(), &friendspb.PrivateMessageRequest{Username37: 1, TargetUsername37: 2})
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mock := &mockFriendsPBClient{
				worldConnectFn: func(ctx context.Context, in *friendspb.WorldConnectRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
					return nil, rpcErr
				},
				playerLoginFn: func(ctx context.Context, in *friendspb.PlayerLoginRequest, opts ...grpc.CallOption) (*friendspb.PlayerLoginResponse, error) {
					return nil, rpcErr
				},
				playerLogoutFn: func(ctx context.Context, in *friendspb.PlayerLogoutRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
					return nil, rpcErr
				},
				chatSetModeFn: func(ctx context.Context, in *friendspb.ChatSetModeRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
					return nil, rpcErr
				},
				friendlistAddFn: func(ctx context.Context, in *friendspb.FriendlistAddRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
					return nil, rpcErr
				},
				friendlistDelFn: func(ctx context.Context, in *friendspb.FriendlistDelRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
					return nil, rpcErr
				},
				ignorelistAddFn: func(ctx context.Context, in *friendspb.IgnorelistAddRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
					return nil, rpcErr
				},
				ignorelistDelFn: func(ctx context.Context, in *friendspb.IgnorelistDelRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
					return nil, rpcErr
				},
				privateMessageFn: func(ctx context.Context, in *friendspb.PrivateMessageRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
					return nil, rpcErr
				},
			}
			c := &grpcFriendsClient{client: mock, log: discardLogger()}
			// Must not panic; must return normally.
			tc.call(c)
		})
	}
}

func TestGRPCFriendsClient_SubscribeUpdates_Delegates(t *testing.T) {
	called := make(chan *friendspb.SubscribeUpdatesRequest, 1)
	mock := &mockFriendsPBClient{
		subscribeUpdatesFn: func(ctx context.Context, in *friendspb.SubscribeUpdatesRequest, opts ...grpc.CallOption) (friendspb.FriendsService_SubscribeUpdatesClient, error) {
			called <- in
			return nil, status.Error(codes.Unavailable, "test")
		},
	}
	c := &grpcFriendsClient{client: mock, log: discardLogger()}
	_, err := c.SubscribeUpdates(context.Background(), &friendspb.SubscribeUpdatesRequest{WorldId: 5, Username37: 42})
	if err == nil {
		t.Fatalf("expected error from mock")
	}
	select {
	case got := <-called:
		if got.WorldId != 5 || got.Username37 != 42 {
			t.Fatalf("got = %v, want WorldId=5 Username37=42", got)
		}
	case <-time.After(time.Second):
		t.Fatalf("mock not called")
	}
}
