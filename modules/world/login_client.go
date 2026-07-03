package world

import (
	"context"
	"fmt"
	"log/slog"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/zsrv/goscape/pkg/loginpb"
)

// LoginClient is the world-side interface to the login service.
// Production impl: grpcLoginClient (this file). Test impl:
// fakeLoginClient (login_client_fake_test.go).
type LoginClient interface {
	WorldStartup(ctx context.Context, nodeID int32, profile string) error
	PlayerLogin(ctx context.Context, req *loginpb.PlayerLoginRequest) (*loginpb.PlayerLoginResponse, error)
	PlayerLogout(ctx context.Context, req *loginpb.PlayerLogoutRequest) (*loginpb.PlayerLogoutResponse, error)
	PlayerAutosave(ctx context.Context, req *loginpb.PlayerAutosaveRequest)
	PlayerForceLogout(ctx context.Context, req *loginpb.PlayerForceLogoutRequest)
	PlayerBan(ctx context.Context, req *loginpb.PlayerBanRequest)
	PlayerMute(ctx context.Context, req *loginpb.PlayerMuteRequest)
	Close() error
}

// grpcLoginClient wraps the gRPC connection to the login server.
type grpcLoginClient struct {
	conn   *grpc.ClientConn
	client loginpb.LoginServiceClient
	log    *slog.Logger
}

// NewLoginClient creates a non-blocking gRPC client to the login server.
// grpc.NewClient does not block — connection is established lazily with automatic retry.
func NewLoginClient(addr string, log *slog.Logger) (LoginClient, error) {
	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		worldClientKeepalive(),
	)
	if err != nil {
		return nil, fmt.Errorf("grpc dial login server: %w", err)
	}
	return &grpcLoginClient{
		conn:   conn,
		client: loginpb.NewLoginServiceClient(conn),
		log:    log,
	}, nil
}

// Close releases the gRPC connection.
func (c *grpcLoginClient) Close() error {
	return c.conn.Close()
}

// WorldStartup notifies the login server that this world is starting.
// Clears any stale sessions from a previous (ungraceful) shutdown. Returns
// the RPC error (in addition to logging it) so the caller can retry —
// arch-29.3: this is the only mechanism that clears stale
// account_login.logged_in rows, so a swallowed failure here used to strand
// every crashed-out player at ALREADY_LOGGED_IN until an operator
// intervened.
func (c *grpcLoginClient) WorldStartup(ctx context.Context, nodeID int32, profile string) error {
	_, err := c.client.WorldStartup(ctx, &loginpb.WorldStartupRequest{
		NodeId:  nodeID,
		Profile: profile,
	})
	if err != nil {
		c.log.Warn("WorldStartup RPC failed", slog.Any("err", err))
	}
	return err
}

// PlayerLogin runs the full auth flow on the login server and returns the response.
func (c *grpcLoginClient) PlayerLogin(ctx context.Context, req *loginpb.PlayerLoginRequest) (*loginpb.PlayerLoginResponse, error) {
	return c.client.PlayerLogin(ctx, req)
}

// PlayerLogout marks the player as logged out and persists their save file.
func (c *grpcLoginClient) PlayerLogout(ctx context.Context, req *loginpb.PlayerLogoutRequest) (*loginpb.PlayerLogoutResponse, error) {
	return c.client.PlayerLogout(ctx, req)
}

// PlayerAutosave persists a player save without logging out (best-effort; called periodically).
func (c *grpcLoginClient) PlayerAutosave(ctx context.Context, req *loginpb.PlayerAutosaveRequest) {
	if _, err := c.client.PlayerAutosave(ctx, req); err != nil {
		c.log.Warn("PlayerAutosave RPC failed",
			slog.String("username", req.Username),
			slog.Any("err", err),
		)
	}
}

// PlayerForceLogout clears the logged-in flag without writing a save (used on disconnect without save data).
func (c *grpcLoginClient) PlayerForceLogout(ctx context.Context, req *loginpb.PlayerForceLogoutRequest) {
	if _, err := c.client.PlayerForceLogout(ctx, req); err != nil {
		c.log.Warn("PlayerForceLogout RPC failed",
			slog.String("username", req.Username),
			slog.Any("err", err),
		)
	}
}

// PlayerBan sets the banned_until timestamp on the login server (best-effort; errors are logged and swallowed).
func (c *grpcLoginClient) PlayerBan(ctx context.Context, req *loginpb.PlayerBanRequest) {
	if _, err := c.client.PlayerBan(ctx, req); err != nil {
		c.log.Warn("PlayerBan RPC failed",
			slog.String("username", req.Username),
			slog.Any("err", err),
		)
	}
}

// PlayerMute sets the muted_until timestamp on the login server (best-effort; errors are logged and swallowed).
func (c *grpcLoginClient) PlayerMute(ctx context.Context, req *loginpb.PlayerMuteRequest) {
	if _, err := c.client.PlayerMute(ctx, req); err != nil {
		c.log.Warn("PlayerMute RPC failed",
			slog.String("username", req.Username),
			slog.Any("err", err),
		)
	}
}
