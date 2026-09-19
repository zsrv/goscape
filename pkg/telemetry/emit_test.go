package telemetry_test

import (
	"testing"

	"github.com/zsrv/goscape/pkg/eventspb"
	"github.com/zsrv/goscape/pkg/telemetry"
)

func TestGet_DefaultsToNoop(t *testing.T) {
	telemetry.Reset()
	e := telemetry.Get()
	if e == nil {
		t.Fatal("Get() returned nil")
	}
	e.EmitAuth(&eventspb.AuthEnvelope{AccountId: 1})
}

func TestSetGetRoundTrip(t *testing.T) {
	telemetry.Reset()
	rec := &recorderEmitter{}
	telemetry.Set(rec)
	defer telemetry.Reset()

	telemetry.Get().EmitAuth(&eventspb.AuthEnvelope{AccountId: 99})
	if rec.authCount != 1 {
		t.Errorf("authCount = %d, want 1", rec.authCount)
	}
}

type recorderEmitter struct{ authCount int }

func (r *recorderEmitter) EmitAuth(*eventspb.AuthEnvelope)               { r.authCount++ }
func (r *recorderEmitter) EmitWorld(*eventspb.WorldEnvelope)             {}
func (r *recorderEmitter) EmitPlayerInput(*eventspb.PlayerInputEnvelope) {}
func (r *recorderEmitter) EmitWealth(*eventspb.WealthEnvelope)           {}
