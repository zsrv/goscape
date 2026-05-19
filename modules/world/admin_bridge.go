package world

import (
	"context"
	"log/slog"

	"github.com/zsrv/goscape/pkg/friendspb"
)

// grpcFriendsAdminBridge is the production FriendsAdminBridge impl. Each
// method fans out to the FriendsClient's corresponding Relay* RPC with
// context.Background() — admin commands are fire-and-forget, errors are
// logged inside the FriendsClient layer.
type grpcFriendsAdminBridge struct {
	client FriendsClient
	log    *slog.Logger
}

var _ FriendsAdminBridge = (*grpcFriendsAdminBridge)(nil)

func (b *grpcFriendsAdminBridge) Mute(targetWorldID int32, username37 uint64, mutedUntilMs int64) {
	b.client.RelayMute(context.Background(), &friendspb.RelayMuteRequest{
		TargetWorldId: targetWorldID, Username37: username37, MutedUntilMs: mutedUntilMs,
	})
}

func (b *grpcFriendsAdminBridge) Kick(targetWorldID int32, username37 uint64) {
	b.client.RelayKick(context.Background(), &friendspb.RelayKickRequest{
		TargetWorldId: targetWorldID, Username37: username37,
	})
}

func (b *grpcFriendsAdminBridge) Shutdown(targetWorldID int32, durationTicks int32) {
	b.client.RelayShutdown(context.Background(), &friendspb.RelayShutdownRequest{
		TargetWorldId: targetWorldID, DurationTicks: durationTicks,
	})
}

func (b *grpcFriendsAdminBridge) Broadcast(targetWorldID int32, message string) {
	b.client.RelayBroadcast(context.Background(), &friendspb.RelayBroadcastRequest{
		TargetWorldId: targetWorldID, Message: message,
	})
}

func (b *grpcFriendsAdminBridge) Track(targetWorldID int32, username37 uint64, state int32) {
	b.client.RelayTrack(context.Background(), &friendspb.RelayTrackRequest{
		TargetWorldId: targetWorldID, Username37: username37, State: state,
	})
}

func (b *grpcFriendsAdminBridge) Reload(targetWorldID int32) {
	b.client.RelayReload(context.Background(), &friendspb.RelayReloadRequest{TargetWorldId: targetWorldID})
}

func (b *grpcFriendsAdminBridge) ClearLogins(targetWorldID int32) {
	b.client.RelayClearLogins(context.Background(), &friendspb.RelayClearLoginsRequest{TargetWorldId: targetWorldID})
}

func (b *grpcFriendsAdminBridge) ClearLogouts(targetWorldID int32) {
	b.client.RelayClearLogouts(context.Background(), &friendspb.RelayClearLogoutsRequest{TargetWorldId: targetWorldID})
}

func (b *grpcFriendsAdminBridge) QueueScript(targetWorldID int32, scriptName string, username37 uint64) {
	b.client.RelayQueueScript(context.Background(), &friendspb.RelayQueueScriptRequest{
		TargetWorldId: targetWorldID, ScriptName: scriptName, Username37: username37,
	})
}

// noopAdminBridge is the fallback when FriendsClient is nil
// (FriendsServerEnabled=false). Mirrors the noopBridges{} pattern used
// by defaultFriendsBridge for the social-list bridge.
type noopAdminBridge struct{}

var _ FriendsAdminBridge = noopAdminBridge{}

func (noopAdminBridge) Mute(int32, uint64, int64)         {}
func (noopAdminBridge) Kick(int32, uint64)                {}
func (noopAdminBridge) Shutdown(int32, int32)             {}
func (noopAdminBridge) Broadcast(int32, string)           {}
func (noopAdminBridge) Track(int32, uint64, int32)        {}
func (noopAdminBridge) Reload(int32)                      {}
func (noopAdminBridge) ClearLogins(int32)                 {}
func (noopAdminBridge) ClearLogouts(int32)                {}
func (noopAdminBridge) QueueScript(int32, string, uint64) {}

// defaultFriendsAdminBridge returns grpcFriendsAdminBridge when client
// is non-nil; otherwise noopAdminBridge{}.
func defaultFriendsAdminBridge(client FriendsClient, log *slog.Logger) FriendsAdminBridge {
	if client == nil {
		return noopAdminBridge{}
	}
	return &grpcFriendsAdminBridge{client: client, log: log}
}
