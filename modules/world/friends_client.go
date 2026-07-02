package world

import (
	"context"
	"fmt"
	"log/slog"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/zsrv/goscape/pkg/friendspb"
)

// FriendsClient is the world-side interface to the friends service.
// Production impl: grpcFriendsClient (this file). Test impl:
// fakeFriendsClient (friends_client_fake_test.go).
//
// All RPCs except Close and SubscribeUpdates are fire-and-forget: errors
// are logged via the embedded *slog.Logger and swallowed. The friends-
// server is best-effort by design — slice-1 and slice-2 deviation tags
// (e.g. NAI-S2-D-PLAYERLOGOUT-BOTH-PATHS) document the posture; the
// world does not depend on its responses.
type FriendsClient interface {
	WorldConnect(ctx context.Context, worldID int32, profile string)
	// PlayerLogin registers the player on the friends server. onResponse is
	// invoked once after the RPC completes: accepted=true on success,
	// accepted=false on cap-reached or RPC error. May be nil.
	PlayerLogin(ctx context.Context, req *friendspb.PlayerLoginRequest, onResponse func(accepted bool))
	PlayerLogout(ctx context.Context, req *friendspb.PlayerLogoutRequest)
	ChatSetMode(ctx context.Context, req *friendspb.ChatSetModeRequest)
	FriendlistAdd(ctx context.Context, req *friendspb.FriendlistAddRequest)
	FriendlistDel(ctx context.Context, req *friendspb.FriendlistDelRequest)
	IgnorelistAdd(ctx context.Context, req *friendspb.IgnorelistAddRequest)
	IgnorelistDel(ctx context.Context, req *friendspb.IgnorelistDelRequest)
	PrivateMessage(ctx context.Context, req *friendspb.PrivateMessageRequest)
	// PublicMessage audit-logs a public-chat utterance to the friends-
	// server. Fire-and-forget per the FriendsClient convention; the
	// grpc impl logs warn + swallows errors. Mirrors TS
	// FriendsClient.publicMessage (FriendServer.ts:669-694 inline).
	PublicMessage(ctx context.Context, req *friendspb.PublicMessageRequest)
	// SubscribeUpdates opens a server-streaming RPC. Returns the stream on
	// success; the caller drains stream.Recv(). Unlike the other RPCs,
	// this one is not fire-and-forget — the supervisor needs the error
	// to drive reconnect backoff.
	SubscribeUpdates(ctx context.Context, req *friendspb.SubscribeUpdatesRequest) (friendspb.FriendsService_SubscribeUpdatesClient, error)

	// --- slice 5a: RELAY_* admin outbound (all fire-and-forget; errors logged) ---
	RelayMute(ctx context.Context, req *friendspb.RelayMuteRequest)
	RelayKick(ctx context.Context, req *friendspb.RelayKickRequest)
	RelayShutdown(ctx context.Context, req *friendspb.RelayShutdownRequest)
	RelayBroadcast(ctx context.Context, req *friendspb.RelayBroadcastRequest)
	RelayTrack(ctx context.Context, req *friendspb.RelayTrackRequest)
	RelayReload(ctx context.Context, req *friendspb.RelayReloadRequest)
	RelayClearLogins(ctx context.Context, req *friendspb.RelayClearLoginsRequest)
	RelayClearLogouts(ctx context.Context, req *friendspb.RelayClearLogoutsRequest)
	RelayQueueScript(ctx context.Context, req *friendspb.RelayQueueScriptRequest)

	// SubscribeWorldEvents opens the per-world admin push stream. Like
	// SubscribeUpdates, this RPC returns the error so the supervisor can
	// drive reconnect backoff.
	SubscribeWorldEvents(ctx context.Context, req *friendspb.SubscribeWorldEventsRequest) (friendspb.FriendsService_SubscribeWorldEventsClient, error)

	Close() error
}

// grpcFriendsClient wraps the gRPC connection to the friends server.
type grpcFriendsClient struct {
	conn   *grpc.ClientConn
	client friendspb.FriendsServiceClient
	log    *slog.Logger
}

// NewFriendsClient creates a non-blocking gRPC client to the friends server.
// grpc.NewClient does not block — connection is established lazily with automatic retry.
func NewFriendsClient(addr string, log *slog.Logger) (FriendsClient, error) {
	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		worldClientKeepalive(),
	)
	if err != nil {
		return nil, fmt.Errorf("grpc dial friends server: %w", err)
	}
	return &grpcFriendsClient{
		conn:   conn,
		client: friendspb.NewFriendsServiceClient(conn),
		log:    log,
	}, nil
}

// Close releases the gRPC connection.
func (c *grpcFriendsClient) Close() error {
	return c.conn.Close()
}

// WorldConnect notifies the friends server that this world is connecting.
// Validates the profile and initializes the world's player-count slot.
func (c *grpcFriendsClient) WorldConnect(ctx context.Context, worldID int32, profile string) {
	if _, err := c.client.WorldConnect(ctx, &friendspb.WorldConnectRequest{
		WorldId: worldID,
		Profile: profile,
	}); err != nil {
		c.log.Warn("WorldConnect RPC failed",
			slog.Int("world_id", int(worldID)),
			slog.String("profile", profile),
			slog.Any("err", err),
		)
	}
}

// PlayerLogin registers the player on the friends server. onResponse is
// invoked once after the RPC completes: accepted=true on success,
// accepted=false on cap-rejection or RPC error. May be nil. Errors are
// logged warn + swallowed before the callback fires (matches the
// fire-and-forget posture of every other void RPC on this client).
func (c *grpcFriendsClient) PlayerLogin(ctx context.Context, req *friendspb.PlayerLoginRequest, onResponse func(accepted bool)) {
	resp, err := c.client.PlayerLogin(ctx, req)
	if err != nil {
		c.log.Warn("PlayerLogin RPC failed",
			slog.Uint64("username37", req.Username37),
			slog.Any("err", err),
		)
		if onResponse != nil {
			onResponse(false)
		}
		return
	}
	if onResponse != nil {
		onResponse(resp.GetAccepted())
	}
}

// PlayerLogout removes the player from the friends server.
func (c *grpcFriendsClient) PlayerLogout(ctx context.Context, req *friendspb.PlayerLogoutRequest) {
	if _, err := c.client.PlayerLogout(ctx, req); err != nil {
		c.log.Warn("PlayerLogout RPC failed",
			slog.Uint64("username37", req.Username37),
			slog.Any("err", err),
		)
	}
}

// ChatSetMode updates the player's privateChat setting on the friends server.
func (c *grpcFriendsClient) ChatSetMode(ctx context.Context, req *friendspb.ChatSetModeRequest) {
	if _, err := c.client.ChatSetMode(ctx, req); err != nil {
		c.log.Warn("ChatSetMode RPC failed",
			slog.Uint64("username37", req.Username37),
			slog.Any("err", err),
		)
	}
}

// FriendlistAdd appends target to the player's friend set on the friends server.
func (c *grpcFriendsClient) FriendlistAdd(ctx context.Context, req *friendspb.FriendlistAddRequest) {
	if _, err := c.client.FriendlistAdd(ctx, req); err != nil {
		c.log.Warn("FriendlistAdd RPC failed",
			slog.Uint64("username37", req.Username37),
			slog.Any("err", err),
		)
	}
}

// FriendlistDel removes target from the player's friend set on the friends server.
func (c *grpcFriendsClient) FriendlistDel(ctx context.Context, req *friendspb.FriendlistDelRequest) {
	if _, err := c.client.FriendlistDel(ctx, req); err != nil {
		c.log.Warn("FriendlistDel RPC failed",
			slog.Uint64("username37", req.Username37),
			slog.Any("err", err),
		)
	}
}

// IgnorelistAdd appends target to the player's ignore set on the friends server.
func (c *grpcFriendsClient) IgnorelistAdd(ctx context.Context, req *friendspb.IgnorelistAddRequest) {
	if _, err := c.client.IgnorelistAdd(ctx, req); err != nil {
		c.log.Warn("IgnorelistAdd RPC failed",
			slog.Uint64("username37", req.Username37),
			slog.Any("err", err),
		)
	}
}

// IgnorelistDel removes target from the player's ignore set on the friends server.
func (c *grpcFriendsClient) IgnorelistDel(ctx context.Context, req *friendspb.IgnorelistDelRequest) {
	if _, err := c.client.IgnorelistDel(ctx, req); err != nil {
		c.log.Warn("IgnorelistDel RPC failed",
			slog.Uint64("username37", req.Username37),
			slog.Any("err", err),
		)
	}
}

// PrivateMessage posts a /tell-style chat message to the friends server.
// Slice 1 logs and returns; slice 4 will fan out to the target's world.
func (c *grpcFriendsClient) PrivateMessage(ctx context.Context, req *friendspb.PrivateMessageRequest) {
	if _, err := c.client.PrivateMessage(ctx, req); err != nil {
		c.log.Warn("PrivateMessage RPC failed",
			slog.Uint64("username37", req.Username37),
			slog.Uint64("target_username37", req.TargetUsername37),
			slog.Any("err", err),
		)
	}
}

// PublicMessage audit-logs a public-chat utterance to the friends server.
// Fire-and-forget — errors are logged at Warn and swallowed.
func (c *grpcFriendsClient) PublicMessage(ctx context.Context, req *friendspb.PublicMessageRequest) {
	if _, err := c.client.PublicMessage(ctx, req); err != nil {
		c.log.Warn("PublicMessage RPC failed",
			slog.String("session_uuid", req.SessionUuid),
			slog.Any("err", err),
		)
	}
}

// SubscribeUpdates opens the server-streaming SubscribeUpdates RPC.
func (c *grpcFriendsClient) SubscribeUpdates(ctx context.Context, req *friendspb.SubscribeUpdatesRequest) (friendspb.FriendsService_SubscribeUpdatesClient, error) {
	return c.client.SubscribeUpdates(ctx, req)
}

// --- slice 5a: RELAY_* admin outbound shims ---
//
// Each method is fire-and-forget per the FriendsClient convention; the
// RPC error is logged at Warn and swallowed. The friends-server is
// best-effort by design — see the file-level FriendsClient doc-comment.

func (c *grpcFriendsClient) RelayMute(ctx context.Context, req *friendspb.RelayMuteRequest) {
	if _, err := c.client.RelayMute(ctx, req); err != nil {
		c.log.Warn("RelayMute RPC failed",
			slog.Int("target_world_id", int(req.TargetWorldId)),
			slog.Uint64("username37", req.Username37),
			slog.Any("err", err),
		)
	}
}

func (c *grpcFriendsClient) RelayKick(ctx context.Context, req *friendspb.RelayKickRequest) {
	if _, err := c.client.RelayKick(ctx, req); err != nil {
		c.log.Warn("RelayKick RPC failed",
			slog.Int("target_world_id", int(req.TargetWorldId)),
			slog.Uint64("username37", req.Username37),
			slog.Any("err", err),
		)
	}
}

func (c *grpcFriendsClient) RelayShutdown(ctx context.Context, req *friendspb.RelayShutdownRequest) {
	if _, err := c.client.RelayShutdown(ctx, req); err != nil {
		c.log.Warn("RelayShutdown RPC failed",
			slog.Int("target_world_id", int(req.TargetWorldId)),
			slog.Any("err", err),
		)
	}
}

func (c *grpcFriendsClient) RelayBroadcast(ctx context.Context, req *friendspb.RelayBroadcastRequest) {
	if _, err := c.client.RelayBroadcast(ctx, req); err != nil {
		c.log.Warn("RelayBroadcast RPC failed",
			slog.Int("target_world_id", int(req.TargetWorldId)),
			slog.Any("err", err),
		)
	}
}

func (c *grpcFriendsClient) RelayTrack(ctx context.Context, req *friendspb.RelayTrackRequest) {
	if _, err := c.client.RelayTrack(ctx, req); err != nil {
		c.log.Warn("RelayTrack RPC failed",
			slog.Int("target_world_id", int(req.TargetWorldId)),
			slog.Uint64("username37", req.Username37),
			slog.Any("err", err),
		)
	}
}

func (c *grpcFriendsClient) RelayReload(ctx context.Context, req *friendspb.RelayReloadRequest) {
	if _, err := c.client.RelayReload(ctx, req); err != nil {
		c.log.Warn("RelayReload RPC failed",
			slog.Int("target_world_id", int(req.TargetWorldId)),
			slog.Any("err", err),
		)
	}
}

func (c *grpcFriendsClient) RelayClearLogins(ctx context.Context, req *friendspb.RelayClearLoginsRequest) {
	if _, err := c.client.RelayClearLogins(ctx, req); err != nil {
		c.log.Warn("RelayClearLogins RPC failed",
			slog.Int("target_world_id", int(req.TargetWorldId)),
			slog.Any("err", err),
		)
	}
}

func (c *grpcFriendsClient) RelayClearLogouts(ctx context.Context, req *friendspb.RelayClearLogoutsRequest) {
	if _, err := c.client.RelayClearLogouts(ctx, req); err != nil {
		c.log.Warn("RelayClearLogouts RPC failed",
			slog.Int("target_world_id", int(req.TargetWorldId)),
			slog.Any("err", err),
		)
	}
}

func (c *grpcFriendsClient) RelayQueueScript(ctx context.Context, req *friendspb.RelayQueueScriptRequest) {
	if _, err := c.client.RelayQueueScript(ctx, req); err != nil {
		c.log.Warn("RelayQueueScript RPC failed",
			slog.Int("target_world_id", int(req.TargetWorldId)),
			slog.String("script_name", req.ScriptName),
			slog.Uint64("username37", req.Username37),
			slog.Any("err", err),
		)
	}
}

// SubscribeWorldEvents opens the server-streaming SubscribeWorldEvents
// RPC. Like SubscribeUpdates, it returns the error so the supervisor
// can drive reconnect backoff (NOT fire-and-forget).
func (c *grpcFriendsClient) SubscribeWorldEvents(ctx context.Context, req *friendspb.SubscribeWorldEventsRequest) (friendspb.FriendsService_SubscribeWorldEventsClient, error) {
	return c.client.SubscribeWorldEvents(ctx, req)
}
