package friends

import (
	"testing"

	"github.com/zsrv/goscape/pkg/friendspb"
	"github.com/zsrv/goscape/pkg/io/protocol/revision"
	"github.com/zsrv/goscape/pkg/telemetry"
)

// TestPrivateMessage_EnvelopeCarriesOrigin pins the two origin fields on the
// relayed-PM event. The request profile is deliberately NOT the "main"
// default, because the envelope describes the ORIGINATING world (its
// world_id is req.WorldId too), and the world sends its own
// world.node_profile on every PrivateMessageRequest. This friends server is
// multi-profile and holds no profile of its own — it keys its repository off
// the same req.Profile — so the request is the only source there is.
func TestPrivateMessage_EnvelopeCarriesOrigin(t *testing.T) {
	cap := &captureEmitter{}
	telemetry.Set(cap)
	t.Cleanup(telemetry.Reset)

	h := newTestHandler(t)
	seedAccount(t, h.repos.db, 0xAAAA)
	seedAccount(t, h.repos.db, 0xBBBB)

	if _, err := h.PrivateMessage(t.Context(), &friendspb.PrivateMessageRequest{
		WorldId:          7,
		Profile:          "beta",
		Username37:       0xAAAA,
		TargetUsername37: 0xBBBB,
		PmId:             1,
		Chat:             "hello",
	}); err != nil {
		t.Fatalf("PrivateMessage: %v", err)
	}

	envs := cap.snapshot()
	if len(envs) != 1 {
		t.Fatalf("emitted %d envelopes, want 1", len(envs))
	}
	env := envs[0]
	if env.Revision != uint32(revision.Expected) {
		t.Errorf("Revision = %d, want %d (revision.Expected)", env.Revision, uint32(revision.Expected))
	}
	if env.Profile != "beta" {
		t.Errorf("Profile = %q, want %q (the originating world's profile)", env.Profile, "beta")
	}
}
