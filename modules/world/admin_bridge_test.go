package world

import (
	"testing"
)

// TestGRPCFriendsAdminBridge_Mute_IssuesRelayMute pins the bridge -> client
// mapping and that Profile is carried (rev-244 B5 multi-profile routing).
func TestGRPCFriendsAdminBridge_Mute_IssuesRelayMute(t *testing.T) {
	fake := newFakeFriendsClient()
	b := &grpcFriendsAdminBridge{client: fake, profile: "main", log: discardLogger()}
	b.Mute(2, 123, 4567)
	req := <-fake.relayMuteReqs
	if req.TargetWorldId != 2 || req.Username37 != 123 || req.MutedUntilMs != 4567 {
		t.Fatalf("unexpected RelayMute req: %+v", req)
	}
	if req.Profile != "main" {
		t.Errorf("Profile: got %q, want main (rev-244 B5 multi-profile carriage)", req.Profile)
	}
}

func TestGRPCFriendsAdminBridge_Kick_IssuesRelayKick(t *testing.T) {
	fake := newFakeFriendsClient()
	b := &grpcFriendsAdminBridge{client: fake, profile: "main", log: discardLogger()}
	b.Kick(2, 123)
	req := <-fake.relayKickReqs
	if req.TargetWorldId != 2 || req.Username37 != 123 {
		t.Fatalf("unexpected RelayKick req: %+v", req)
	}
	if req.Profile != "main" {
		t.Errorf("Profile: got %q, want main", req.Profile)
	}
}

func TestGRPCFriendsAdminBridge_Shutdown_IssuesRelayShutdown(t *testing.T) {
	fake := newFakeFriendsClient()
	b := &grpcFriendsAdminBridge{client: fake, profile: "main", log: discardLogger()}
	b.Shutdown(2, 100)
	req := <-fake.relayShutdownReqs
	if req.TargetWorldId != 2 || req.DurationTicks != 100 {
		t.Fatalf("unexpected RelayShutdown req: %+v", req)
	}
}

func TestGRPCFriendsAdminBridge_Broadcast_IssuesRelayBroadcast(t *testing.T) {
	fake := newFakeFriendsClient()
	b := &grpcFriendsAdminBridge{client: fake, profile: "main", log: discardLogger()}
	b.Broadcast(2, "hello")
	req := <-fake.relayBroadcastReqs
	if req.TargetWorldId != 2 || req.Message != "hello" {
		t.Fatalf("unexpected RelayBroadcast req: %+v", req)
	}
}

func TestGRPCFriendsAdminBridge_Track_IssuesRelayTrack(t *testing.T) {
	fake := newFakeFriendsClient()
	b := &grpcFriendsAdminBridge{client: fake, profile: "main", log: discardLogger()}
	b.Track(2, 123, 1)
	req := <-fake.relayTrackReqs
	if req.TargetWorldId != 2 || req.Username37 != 123 || req.State != 1 {
		t.Fatalf("unexpected RelayTrack req: %+v", req)
	}
}

func TestGRPCFriendsAdminBridge_Reload_IssuesRelayReload(t *testing.T) {
	fake := newFakeFriendsClient()
	b := &grpcFriendsAdminBridge{client: fake, profile: "main", log: discardLogger()}
	b.Reload(2)
	req := <-fake.relayReloadReqs
	if req.TargetWorldId != 2 {
		t.Fatalf("unexpected RelayReload req: %+v", req)
	}
}

func TestGRPCFriendsAdminBridge_ClearLogins_IssuesRelayClearLogins(t *testing.T) {
	fake := newFakeFriendsClient()
	b := &grpcFriendsAdminBridge{client: fake, profile: "main", log: discardLogger()}
	b.ClearLogins(2)
	req := <-fake.relayClearLoginsReqs
	if req.TargetWorldId != 2 {
		t.Fatalf("unexpected RelayClearLogins req: %+v", req)
	}
}

func TestGRPCFriendsAdminBridge_ClearLogouts_IssuesRelayClearLogouts(t *testing.T) {
	fake := newFakeFriendsClient()
	b := &grpcFriendsAdminBridge{client: fake, profile: "main", log: discardLogger()}
	b.ClearLogouts(2)
	req := <-fake.relayClearLogoutsReqs
	if req.TargetWorldId != 2 {
		t.Fatalf("unexpected RelayClearLogouts req: %+v", req)
	}
}

func TestGRPCFriendsAdminBridge_QueueScript_IssuesRelayQueueScript(t *testing.T) {
	fake := newFakeFriendsClient()
	b := &grpcFriendsAdminBridge{client: fake, profile: "main", log: discardLogger()}
	b.QueueScript(2, "debug:dump", 123)
	req := <-fake.relayQueueScriptReqs
	if req.TargetWorldId != 2 || req.ScriptName != "debug:dump" || req.Username37 != 123 {
		t.Fatalf("unexpected RelayQueueScript req: %+v", req)
	}
}

// TestDefaultFriendsAdminBridge_NilClient_NoopReturnsCleanly pins the
// nil-FriendsClient fallback to noopAdminBridge.
func TestDefaultFriendsAdminBridge_NilClient_NoopReturnsCleanly(t *testing.T) {
	b := defaultFriendsAdminBridge(nil, "main", discardLogger())
	// All methods must not panic.
	b.Mute(1, 1, 1)
	b.Kick(1, 1)
	b.Shutdown(1, 1)
	b.Broadcast(1, "x")
	b.Track(1, 1, 1)
	b.Reload(1)
	b.ClearLogins(1)
	b.ClearLogouts(1)
	b.QueueScript(1, "x", 1)
}
