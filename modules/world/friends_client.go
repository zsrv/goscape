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
// All RPCs except Close are fire-and-forget: errors are logged via
// the embedded *slog.Logger and swallowed. The friends-server is
// best-effort by design (slice 1's NAI-S1-D-PM-NO-DELIVERY etc.);
// the world does not depend on its responses through slice 3.
//
// SubscribeUpdates is intentionally absent — slice 4 adds it.
type FriendsClient interface {
	WorldConnect(ctx context.Context, worldID int32, profile string)
	PlayerLogin(ctx context.Context, req *friendspb.PlayerLoginRequest)
	PlayerLogout(ctx context.Context, req *friendspb.PlayerLogoutRequest)
	ChatSetMode(ctx context.Context, req *friendspb.ChatSetModeRequest)
	FriendlistAdd(ctx context.Context, req *friendspb.FriendlistAddRequest)
	FriendlistDel(ctx context.Context, req *friendspb.FriendlistDelRequest)
	IgnorelistAdd(ctx context.Context, req *friendspb.IgnorelistAddRequest)
	IgnorelistDel(ctx context.Context, req *friendspb.IgnorelistDelRequest)
	PrivateMessage(ctx context.Context, req *friendspb.PrivateMessageRequest)
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

// PlayerLogin registers the player on the friends server.
// PlayerLoginResponse.Accepted is ignored slice-2 (NAI-S2-D-PLAYERLOGIN-IGNORES-ACCEPTED).
func (c *grpcFriendsClient) PlayerLogin(ctx context.Context, req *friendspb.PlayerLoginRequest) {
	if _, err := c.client.PlayerLogin(ctx, req); err != nil {
		c.log.Warn("PlayerLogin RPC failed",
			slog.Uint64("username37", req.Username37),
			slog.Any("err", err),
		)
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
