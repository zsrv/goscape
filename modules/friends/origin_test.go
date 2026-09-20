package friends

import (
	"testing"

	"github.com/zsrv/goscape/pkg/friendspb"
	"github.com/zsrv/goscape/pkg/io/protocol/revision"
	"github.com/zsrv/goscape/pkg/telemetry"
)

// TestPrivateMessage_EnvelopeCarriesOrigin pins the two origin fields on the
// relayed-PM event. The configured profile is deliberately NOT the "main"
// default, so a site that hard-codes the default instead of reading
// friends.node_profile fails here. PrivateMessageRequest carries no profile
// of its own on this revision, and this friends server serves exactly one
// profile (WorldConnect rejects a world whose profile differs), so the
// originating world's profile — what the envelope describes — is
// h.cfg.NodeProfile.
func TestPrivateMessage_EnvelopeCarriesOrigin(t *testing.T) {
	cap := &captureEmitter{}
	telemetry.Set(cap)
	t.Cleanup(telemetry.Reset)

	h := newTestHandler(t)
	h.cfg.NodeProfile = "beta"
	seedAccount(t, h.repo.db, 0xAAAA)
	seedAccount(t, h.repo.db, 0xBBBB)

	if _, err := h.PrivateMessage(t.Context(), &friendspb.PrivateMessageRequest{
		WorldId:          7,
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
		t.Errorf("Profile = %q, want %q (friends.node_profile)", env.Profile, "beta")
	}
}
