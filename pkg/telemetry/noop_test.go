package telemetry

import (
	"testing"

	"github.com/zsrv/goscape/pkg/eventspb"
)

func TestGetReturnsNoopByDefault(t *testing.T) {
	Reset()
	Get().EmitAuth(&eventspb.AuthEnvelope{})
	Get().EmitWorld(&eventspb.WorldEnvelope{})
	Get().EmitPlayerInput(&eventspb.PlayerInputEnvelope{})
	Get().EmitWealth(&eventspb.WealthEnvelope{})
}
