package login

import (
	"testing"

	"github.com/zsrv/goscape/pkg/eventspb"
	"github.com/zsrv/goscape/pkg/loginpb"
	"github.com/zsrv/goscape/pkg/telemetry"
)

type captureEmitter struct {
	envelopes []*eventspb.AuthEnvelope
}

func (c *captureEmitter) EmitAuth(env *eventspb.AuthEnvelope) {
	c.envelopes = append(c.envelopes, env)
}

func (c *captureEmitter) EmitWorld(*eventspb.WorldEnvelope)             {}
func (c *captureEmitter) EmitPlayerInput(*eventspb.PlayerInputEnvelope) {}
func (c *captureEmitter) EmitWealth(*eventspb.WealthEnvelope)           {}

func TestPlayerLogin_EmitsAuthEnvelope(t *testing.T) {
	cap := &captureEmitter{}
	telemetry.Set(cap)
	defer telemetry.Reset()

	h, _ := newTestHandler(t)
	resp, err := h.PlayerLogin(t.Context(), &loginpb.PlayerLoginRequest{
		NodeId:        7,
		Profile:       "main",
		NodeMembers:   true,
		Username:      "emituser",
		Password:      "hunter2",
		Uid:           42,
		RemoteAddress: "192.168.1.1:12345",
	})
	if err != nil {
		t.Fatalf("PlayerLogin: %v", err)
	}
	if resp.Result != loginpb.LoginResult_LOGIN_RESULT_NEW_PLAYER {
		t.Fatalf("unexpected Result: %v", resp.Result)
	}
	if len(cap.envelopes) != 1 {
		t.Fatalf("emitted %d envelopes, want 1", len(cap.envelopes))
	}
	env := cap.envelopes[0]
	if env.GetLogin() == nil {
		t.Fatal("envelope payload is not Login")
	}
	if env.WorldId != 7 {
		t.Errorf("WorldId = %d, want 7", env.WorldId)
	}
	if got := env.GetLogin().GetIp(); got != "192.168.1.1" {
		t.Errorf("Ip = %q, want %q", got, "192.168.1.1")
	}
}
