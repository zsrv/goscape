package world

import (
	"context"
	"errors"
	"log/slog"
	"strings"
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
	bridge := &grpcFriendsBridge{client: fake, worldID: testWorldID, dispatcher: newRunningFriendsDispatcher(t), log: discardLogger()}

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
	bridge := &grpcFriendsBridge{client: fake, worldID: testWorldID, dispatcher: newRunningFriendsDispatcher(t), log: discardLogger()}

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
	bridge := &grpcFriendsBridge{client: fake, worldID: testWorldID, dispatcher: newRunningFriendsDispatcher(t), log: discardLogger()}

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
	bridge := &grpcFriendsBridge{client: fake, worldID: testWorldID, dispatcher: newRunningFriendsDispatcher(t), log: discardLogger()}

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
	bridge := &grpcFriendsBridge{client: fake, worldID: testWorldID, dispatcher: newRunningFriendsDispatcher(t), log: discardLogger()}

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
	bridge := &grpcFriendsBridge{client: fake, worldID: testWorldID, dispatcher: newRunningFriendsDispatcher(t), log: discardLogger()}

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
	bridge := &grpcFriendsBridge{client: gated, worldID: testWorldID, dispatcher: newRunningFriendsDispatcher(t), log: discardLogger()}

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
	got := defaultFriendsBridge(newFakeFriendsClient(), testWorldID, newFriendsMutationDispatcher(discardLogger()), discardLogger())
	b, ok := got.(*grpcFriendsBridge)
	if !ok {
		t.Fatalf("defaultFriendsBridge: got %T, want *grpcFriendsBridge", got)
	}
	if b.worldID != testWorldID {
		t.Errorf("worldID: got %d, want %d", b.worldID, testWorldID)
	}
}

func TestDefaultFriendsBridge_NilClient_ReturnsNoop(t *testing.T) {
	got := defaultFriendsBridge(nil, testWorldID, nil, discardLogger())
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
	worldConnectFn     func(ctx context.Context, in *friendspb.WorldConnectRequest, opts ...grpc.CallOption) (*emptypb.Empty, error)
	playerLoginFn      func(ctx context.Context, in *friendspb.PlayerLoginRequest, opts ...grpc.CallOption) (*friendspb.PlayerLoginResponse, error)
	playerLogoutFn     func(ctx context.Context, in *friendspb.PlayerLogoutRequest, opts ...grpc.CallOption) (*emptypb.Empty, error)
	chatSetModeFn      func(ctx context.Context, in *friendspb.ChatSetModeRequest, opts ...grpc.CallOption) (*emptypb.Empty, error)
	friendlistAddFn    func(ctx context.Context, in *friendspb.FriendlistAddRequest, opts ...grpc.CallOption) (*emptypb.Empty, error)
	friendlistDelFn    func(ctx context.Context, in *friendspb.FriendlistDelRequest, opts ...grpc.CallOption) (*emptypb.Empty, error)
	ignorelistAddFn    func(ctx context.Context, in *friendspb.IgnorelistAddRequest, opts ...grpc.CallOption) (*emptypb.Empty, error)
	ignorelistDelFn    func(ctx context.Context, in *friendspb.IgnorelistDelRequest, opts ...grpc.CallOption) (*emptypb.Empty, error)
	privateMessageFn   func(ctx context.Context, in *friendspb.PrivateMessageRequest, opts ...grpc.CallOption) (*emptypb.Empty, error)
	subscribeUpdatesFn func(ctx context.Context, in *friendspb.SubscribeUpdatesRequest, opts ...grpc.CallOption) (friendspb.FriendsService_SubscribeUpdatesClient, error)

	// slice 5a Relay* outbound + SubscribeWorldEvents stream
	relayMuteFn            func(ctx context.Context, in *friendspb.RelayMuteRequest, opts ...grpc.CallOption) (*emptypb.Empty, error)
	relayKickFn            func(ctx context.Context, in *friendspb.RelayKickRequest, opts ...grpc.CallOption) (*emptypb.Empty, error)
	relayShutdownFn        func(ctx context.Context, in *friendspb.RelayShutdownRequest, opts ...grpc.CallOption) (*emptypb.Empty, error)
	relayBroadcastFn       func(ctx context.Context, in *friendspb.RelayBroadcastRequest, opts ...grpc.CallOption) (*emptypb.Empty, error)
	relayTrackFn           func(ctx context.Context, in *friendspb.RelayTrackRequest, opts ...grpc.CallOption) (*emptypb.Empty, error)
	relayReloadFn          func(ctx context.Context, in *friendspb.RelayReloadRequest, opts ...grpc.CallOption) (*emptypb.Empty, error)
	relayClearLoginsFn     func(ctx context.Context, in *friendspb.RelayClearLoginsRequest, opts ...grpc.CallOption) (*emptypb.Empty, error)
	relayClearLogoutsFn    func(ctx context.Context, in *friendspb.RelayClearLogoutsRequest, opts ...grpc.CallOption) (*emptypb.Empty, error)
	relayQueueScriptFn     func(ctx context.Context, in *friendspb.RelayQueueScriptRequest, opts ...grpc.CallOption) (*emptypb.Empty, error)
	subscribeWorldEventsFn func(ctx context.Context, in *friendspb.SubscribeWorldEventsRequest, opts ...grpc.CallOption) (friendspb.FriendsService_SubscribeWorldEventsClient, error)
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

func (m *mockFriendsPBClient) RelayMute(ctx context.Context, in *friendspb.RelayMuteRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	return m.relayMuteFn(ctx, in, opts...)
}
func (m *mockFriendsPBClient) RelayKick(ctx context.Context, in *friendspb.RelayKickRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	return m.relayKickFn(ctx, in, opts...)
}
func (m *mockFriendsPBClient) RelayShutdown(ctx context.Context, in *friendspb.RelayShutdownRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	return m.relayShutdownFn(ctx, in, opts...)
}
func (m *mockFriendsPBClient) RelayBroadcast(ctx context.Context, in *friendspb.RelayBroadcastRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	return m.relayBroadcastFn(ctx, in, opts...)
}
func (m *mockFriendsPBClient) RelayTrack(ctx context.Context, in *friendspb.RelayTrackRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	return m.relayTrackFn(ctx, in, opts...)
}
func (m *mockFriendsPBClient) RelayReload(ctx context.Context, in *friendspb.RelayReloadRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	return m.relayReloadFn(ctx, in, opts...)
}
func (m *mockFriendsPBClient) RelayClearLogins(ctx context.Context, in *friendspb.RelayClearLoginsRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	return m.relayClearLoginsFn(ctx, in, opts...)
}
func (m *mockFriendsPBClient) RelayClearLogouts(ctx context.Context, in *friendspb.RelayClearLogoutsRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	return m.relayClearLogoutsFn(ctx, in, opts...)
}
func (m *mockFriendsPBClient) RelayQueueScript(ctx context.Context, in *friendspb.RelayQueueScriptRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	return m.relayQueueScriptFn(ctx, in, opts...)
}
func (m *mockFriendsPBClient) SubscribeWorldEvents(ctx context.Context, in *friendspb.SubscribeWorldEventsRequest, opts ...grpc.CallOption) (friendspb.FriendsService_SubscribeWorldEventsClient, error) {
	if m.subscribeWorldEventsFn != nil {
		return m.subscribeWorldEventsFn(ctx, in, opts...)
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

// TestGRPCFriendsClient_PlayerLogin_InvokesCallback pins slice 4c's
// callback contract: onResponse fires with accepted=true on
// PlayerLoginResponse{Accepted: true}, accepted=false on Accepted: false,
// and accepted=false on RPC error. Replaces the slice-2 posture that
// discarded the response (NAI-S2-D-PLAYERLOGIN-IGNORES-ACCEPTED, retired
// by slice 4c).
func TestGRPCFriendsClient_PlayerLogin_InvokesCallback(t *testing.T) {
	cases := []struct {
		name    string
		resp    *friendspb.PlayerLoginResponse
		err     error
		wantAcc bool
	}{
		{"AcceptedTrue", &friendspb.PlayerLoginResponse{Accepted: true}, nil, true},
		{"AcceptedFalse", &friendspb.PlayerLoginResponse{Accepted: false}, nil, false},
		{"RPCError", nil, errors.New("simulated"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mock := &mockFriendsPBClient{
				playerLoginFn: func(ctx context.Context, in *friendspb.PlayerLoginRequest, opts ...grpc.CallOption) (*friendspb.PlayerLoginResponse, error) {
					return tc.resp, tc.err
				},
			}
			c := &grpcFriendsClient{client: mock, log: discardLogger()}
			ch := make(chan bool, 1)
			c.PlayerLogin(context.Background(), &friendspb.PlayerLoginRequest{Username37: 1}, func(accepted bool) {
				ch <- accepted
			})
			select {
			case got := <-ch:
				if got != tc.wantAcc {
					t.Errorf("callback accepted: got %v, want %v", got, tc.wantAcc)
				}
			case <-time.After(time.Second):
				t.Fatal("timeout waiting for callback")
			}
		})
	}
}

// erroringFriendsPBClient returns codes.Unavailable for every slice-5a
// Relay* RPC. Used to exercise the production grpcFriendsClient's
// error-logging branches across all 9 fire-and-forget admin RPCs.
type erroringFriendsPBClient struct {
	friendspb.FriendsServiceClient
}

func (erroringFriendsPBClient) RelayMute(context.Context, *friendspb.RelayMuteRequest, ...grpc.CallOption) (*emptypb.Empty, error) {
	return nil, status.Error(codes.Unavailable, "test")
}
func (erroringFriendsPBClient) RelayKick(context.Context, *friendspb.RelayKickRequest, ...grpc.CallOption) (*emptypb.Empty, error) {
	return nil, status.Error(codes.Unavailable, "test")
}
func (erroringFriendsPBClient) RelayShutdown(context.Context, *friendspb.RelayShutdownRequest, ...grpc.CallOption) (*emptypb.Empty, error) {
	return nil, status.Error(codes.Unavailable, "test")
}
func (erroringFriendsPBClient) RelayBroadcast(context.Context, *friendspb.RelayBroadcastRequest, ...grpc.CallOption) (*emptypb.Empty, error) {
	return nil, status.Error(codes.Unavailable, "test")
}
func (erroringFriendsPBClient) RelayTrack(context.Context, *friendspb.RelayTrackRequest, ...grpc.CallOption) (*emptypb.Empty, error) {
	return nil, status.Error(codes.Unavailable, "test")
}
func (erroringFriendsPBClient) RelayReload(context.Context, *friendspb.RelayReloadRequest, ...grpc.CallOption) (*emptypb.Empty, error) {
	return nil, status.Error(codes.Unavailable, "test")
}
func (erroringFriendsPBClient) RelayClearLogins(context.Context, *friendspb.RelayClearLoginsRequest, ...grpc.CallOption) (*emptypb.Empty, error) {
	return nil, status.Error(codes.Unavailable, "test")
}
func (erroringFriendsPBClient) RelayClearLogouts(context.Context, *friendspb.RelayClearLogoutsRequest, ...grpc.CallOption) (*emptypb.Empty, error) {
	return nil, status.Error(codes.Unavailable, "test")
}
func (erroringFriendsPBClient) RelayQueueScript(context.Context, *friendspb.RelayQueueScriptRequest, ...grpc.CallOption) (*emptypb.Empty, error) {
	return nil, status.Error(codes.Unavailable, "test")
}

// TestGRPCFriendsClient_Relay_LogsErrorOnFailure exercises each Relay*
// RPC path under a forced-error gRPC client and asserts the production
// fire-and-forget impl logs warn + swallows the error. Table-driven to
// keep the 9 cases concise.
func TestGRPCFriendsClient_Relay_LogsErrorOnFailure(t *testing.T) {
	cases := []struct {
		name string
		op   string // substring expected in the warn log
		call func(c *grpcFriendsClient, ctx context.Context)
	}{
		{"RelayMute", "RelayMute", func(c *grpcFriendsClient, ctx context.Context) {
			c.RelayMute(ctx, &friendspb.RelayMuteRequest{TargetWorldId: 2, Username37: 1, MutedUntilMs: 0})
		}},
		{"RelayKick", "RelayKick", func(c *grpcFriendsClient, ctx context.Context) {
			c.RelayKick(ctx, &friendspb.RelayKickRequest{TargetWorldId: 2, Username37: 1})
		}},
		{"RelayShutdown", "RelayShutdown", func(c *grpcFriendsClient, ctx context.Context) {
			c.RelayShutdown(ctx, &friendspb.RelayShutdownRequest{TargetWorldId: 2, DurationTicks: 0})
		}},
		{"RelayBroadcast", "RelayBroadcast", func(c *grpcFriendsClient, ctx context.Context) {
			c.RelayBroadcast(ctx, &friendspb.RelayBroadcastRequest{TargetWorldId: 2, Message: "x"})
		}},
		{"RelayTrack", "RelayTrack", func(c *grpcFriendsClient, ctx context.Context) {
			c.RelayTrack(ctx, &friendspb.RelayTrackRequest{TargetWorldId: 2, Username37: 1, State: 0})
		}},
		{"RelayReload", "RelayReload", func(c *grpcFriendsClient, ctx context.Context) {
			c.RelayReload(ctx, &friendspb.RelayReloadRequest{TargetWorldId: 2})
		}},
		{"RelayClearLogins", "RelayClearLogins", func(c *grpcFriendsClient, ctx context.Context) {
			c.RelayClearLogins(ctx, &friendspb.RelayClearLoginsRequest{TargetWorldId: 2})
		}},
		{"RelayClearLogouts", "RelayClearLogouts", func(c *grpcFriendsClient, ctx context.Context) {
			c.RelayClearLogouts(ctx, &friendspb.RelayClearLogoutsRequest{TargetWorldId: 2})
		}},
		{"RelayQueueScript", "RelayQueueScript", func(c *grpcFriendsClient, ctx context.Context) {
			c.RelayQueueScript(ctx, &friendspb.RelayQueueScriptRequest{TargetWorldId: 2, ScriptName: "s", Username37: 1})
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			buf := &syncBuffer{}
			log := slog.New(slog.NewTextHandler(buf, nil))
			c := &grpcFriendsClient{
				client: &erroringFriendsPBClient{},
				log:    log,
			}
			tc.call(c, context.Background())
			if !strings.Contains(buf.String(), tc.op+" RPC failed") {
				t.Fatalf("log missing %q; got: %s", tc.op+" RPC failed", buf.String())
			}
		})
	}
}
