package login

import (
	"testing"

	"github.com/zsrv/goscape/pkg/io/protocol/revision"
	"github.com/zsrv/goscape/pkg/loginpb"
	"github.com/zsrv/goscape/pkg/telemetry"
)

// TestPlayerLogin_AuthEnvelopeCarriesOrigin pins the two origin fields on the
// login auth event. The request profile is deliberately NOT the "main"
// default (and differs from h.cfg.NodeProfile, which stays "main"), because
// one login server serves every profile: account.id is global while
// account_login is keyed by (account_id, profile), so the event's profile is
// the one the request — and every DB query in the handler — is scoped by.
func TestPlayerLogin_AuthEnvelopeCarriesOrigin(t *testing.T) {
	cap := &captureEmitter{}
	telemetry.Set(cap)
	t.Cleanup(telemetry.Reset)

	h, _ := newTestHandler(t)
	if h.cfg.NodeProfile == "beta" {
		t.Fatal("fixture profile must differ from the request profile for this test to mean anything")
	}
	if _, err := h.PlayerLogin(t.Context(), &loginpb.PlayerLoginRequest{
		NodeId:        7,
		Profile:       "beta",
		NodeMembers:   true,
		Username:      "originuser",
		Password:      "hunter2",
		Uid:           42,
		RemoteAddress: "192.168.1.1:12345",
	}); err != nil {
		t.Fatalf("PlayerLogin: %v", err)
	}

	if len(cap.envelopes) != 1 {
		t.Fatalf("emitted %d envelopes, want 1", len(cap.envelopes))
	}
	env := cap.envelopes[0]
	if env.Revision != uint32(revision.Expected) {
		t.Errorf("Revision = %d, want %d (revision.Expected)", env.Revision, uint32(revision.Expected))
	}
	if env.Profile != "beta" {
		t.Errorf("Profile = %q, want %q (the request's profile)", env.Profile, "beta")
	}
}

// TestPlayerLogin_AuthEnvelopeTakesRevisionFromRequest pins the other half of
// the origin: the revision is the CALLING world's, not this login binary's.
// The login service and its central DB are revision-agnostic, so a single
// login server may serve worlds of several revisions; stamping
// revision.Expected here would misattribute every one of their auth events.
// The request revision is deliberately not revision.Expected.
func TestPlayerLogin_AuthEnvelopeTakesRevisionFromRequest(t *testing.T) {
	cap := &captureEmitter{}
	telemetry.Set(cap)
	t.Cleanup(telemetry.Reset)

	const otherRevision = uint32(revision.Expected) + 1

	h, _ := newTestHandler(t)
	if _, err := h.PlayerLogin(t.Context(), &loginpb.PlayerLoginRequest{
		NodeId:        7,
		Profile:       "beta",
		NodeMembers:   true,
		Username:      "originuser",
		Password:      "hunter2",
		Uid:           42,
		RemoteAddress: "192.168.1.1:12345",
		Revision:      otherRevision,
	}); err != nil {
		t.Fatalf("PlayerLogin: %v", err)
	}

	if len(cap.envelopes) != 1 {
		t.Fatalf("emitted %d envelopes, want 1", len(cap.envelopes))
	}
	if got := cap.envelopes[0].Revision; got != otherRevision {
		t.Errorf("Revision = %d, want %d (the request's revision, not this binary's %d)",
			got, otherRevision, uint32(revision.Expected))
	}
}

// TestPlayerLogin_AuthEnvelopeRevisionFallsBackToOwn pins the compatibility
// half: a world build that predates the request field sends revision 0, and
// the event then carries this login binary's own revision.Expected — exactly
// what was stamped before the field existed.
func TestPlayerLogin_AuthEnvelopeRevisionFallsBackToOwn(t *testing.T) {
	cap := &captureEmitter{}
	telemetry.Set(cap)
	t.Cleanup(telemetry.Reset)

	h, _ := newTestHandler(t)
	if _, err := h.PlayerLogin(t.Context(), &loginpb.PlayerLoginRequest{
		NodeId:        7,
		Profile:       "beta",
		NodeMembers:   true,
		Username:      "originuser",
		Password:      "hunter2",
		Uid:           42,
		RemoteAddress: "192.168.1.1:12345",
		Revision:      0, // pre-upgrade world build
	}); err != nil {
		t.Fatalf("PlayerLogin: %v", err)
	}

	if len(cap.envelopes) != 1 {
		t.Fatalf("emitted %d envelopes, want 1", len(cap.envelopes))
	}
	if got := cap.envelopes[0].Revision; got != uint32(revision.Expected) {
		t.Errorf("Revision = %d, want %d (fallback to this binary's revision.Expected)",
			got, uint32(revision.Expected))
	}
}
