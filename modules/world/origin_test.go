package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/eventspb"
	"github.com/zsrv/goscape/pkg/gamemap"
	gameclient "github.com/zsrv/goscape/pkg/io/protocol/game/client"
	"github.com/zsrv/goscape/pkg/io/protocol/revision"
	"github.com/zsrv/goscape/pkg/rsbuf"
	"github.com/zsrv/goscape/pkg/telemetry"
)

// originProfile is deliberately NOT the "main" default, so a site that
// hard-codes the default instead of reading world.node_profile fails here.
const originProfile = "beta"

// assertOrigin checks the two origin fields every envelope emitted from this
// module must carry: the binary's wire revision and the world's configured
// deployment profile.
func assertOrigin(t *testing.T, revisionGot uint32, profileGot string) {
	t.Helper()
	if revisionGot != uint32(revision.Expected) {
		t.Errorf("Revision = %d, want %d (revision.Expected)", revisionGot, uint32(revision.Expected))
	}
	if profileGot != originProfile {
		t.Errorf("Profile = %q, want %q (world.node_profile)", profileGot, originProfile)
	}
}

func (c *captureEmitter) snapshotWorld() []*eventspb.WorldEnvelope {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]*eventspb.WorldEnvelope(nil), c.worldEnvs...)
}

func (c *captureEmitter) snapshotPlayerInput() []*eventspb.PlayerInputEnvelope {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]*eventspb.PlayerInputEnvelope(nil), c.playerInputEnvs...)
}

// newOriginServer is newTestServer with a non-default node profile and a
// node id, plus a recording emitter installed on the telemetry seam.
func newOriginServer(t *testing.T) (*Server, *captureEmitter) {
	t.Helper()
	s := newTestServer(t)
	s.cfg.NodeID = 7
	s.cfg.NodeProfile = originProfile
	cap := &captureEmitter{}
	telemetry.Set(cap)
	t.Cleanup(telemetry.Reset)
	return s, cap
}

// TestPublicChatEnvelopeCarriesOrigin covers handlers_game.go.
func TestPublicChatEnvelopeCarriesOrigin(t *testing.T) {
	s, cap := newOriginServer(t)
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.username = "alice"
	p.session = "uuid-sess-1"
	p.level, p.x, p.z = 0, 3210, 3210

	if err := handleMessagePublic(p, packPublicChatPayload(0, 0, "hi")); err != nil {
		t.Fatalf("handleMessagePublic: %v", err)
	}
	envs := cap.publicChats()
	if len(envs) != 1 {
		t.Fatalf("PublicChatEvent envelopes: got %d, want 1", len(envs))
	}
	assertOrigin(t, envs[0].Revision, envs[0].Profile)
}

// TestPacketReceivedEnvelopeCarriesOrigin covers player.go.
func TestPacketReceivedEnvelopeCarriesOrigin(t *testing.T) {
	s, cap := newOriginServer(t)
	enc, dec := isaacPair([4]uint32{11, 22, 33, 44})
	p, _ := newTestPlayer(t)
	p.client.decryptor = dec
	p.client.encryptor = enc
	p.client.server = s

	// MOVE_GAMECLICK, 1-byte length prefix. Payload is
	// ctrlHeld(1) + startX G2(2) + startZ G2(2) = 5 bytes.
	payload := []byte{0, 0x0C, 0xA4, 0x0C, 0x8B}
	buf := []byte{encryptOpcode(enc, gameclient.OpcMoveGameClick), byte(len(payload))}
	buf = append(buf, payload...)
	p.client.in.Write(buf)

	if _, _, _, err := p.readPacket(); err != nil {
		t.Fatalf("readPacket: %v", err)
	}

	var got *eventspb.WorldEnvelope
	for _, e := range cap.snapshotWorld() {
		if e.GetPacketReceived() != nil {
			got = e
		}
	}
	if got == nil {
		t.Fatal("no PacketReceivedEvent envelope emitted")
	}
	assertOrigin(t, got.Revision, got.Profile)
}

// TestTilePositionEnvelopeCarriesOrigin covers player_info.go.
func TestTilePositionEnvelopeCarriesOrigin(t *testing.T) {
	s, cap := newOriginServer(t)
	s.gamemap = gamemap.New(discardLogger())
	if err := s.gamemap.Init(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	s.renderer = rsbuf.NewRenderer()

	a := setupInfoPlayer(t, s, 1, 3094, 3106, 0)
	s.processInfo()
	a.updatePlayers()

	var got *eventspb.WorldEnvelope
	for _, e := range cap.snapshotWorld() {
		if e.GetTilePosition() != nil {
			got = e
		}
	}
	if got == nil {
		t.Fatal("no TilePositionEvent envelope emitted")
	}
	assertOrigin(t, got.Revision, got.Profile)
}

// TestMouseMoveEnvelopeCarriesOrigin covers handler_event_tracking.go, the
// module's only PlayerInputEnvelope site: the windowed EVENT_TRACKING handler
// emits the MouseMoveEvent once the blob clears every gate (size, active
// window, ShouldSubmitTrackingDetails, per-session byte cap).
func TestMouseMoveEnvelopeCarriesOrigin(t *testing.T) {
	s, cap := newOriginServer(t)
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.input = &InputTracking{player: p}
	p.input.startTrackingAt = 0
	p.input.endTrackingAt = 1000
	p.submitInput = true
	s.cfg.NodeLimitBytesPerTrackingSession = 50000

	if err := handleEventTracking(p, []byte{0xAA, 0xBB, 0xCC}); err != nil {
		t.Fatalf("handleEventTracking: %v", err)
	}
	envs := cap.snapshotPlayerInput()
	if len(envs) != 1 {
		t.Fatalf("PlayerInput envelopes: got %d, want 1", len(envs))
	}
	assertOrigin(t, envs[0].Revision, envs[0].Profile)
}
