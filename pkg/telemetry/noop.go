package telemetry

import "github.com/zsrv/goscape/pkg/eventspb"

type noopEmitter struct{}

func (noopEmitter) EmitAuth(*eventspb.AuthEnvelope)               {}
func (noopEmitter) EmitWorld(*eventspb.WorldEnvelope)             {}
func (noopEmitter) EmitPlayerInput(*eventspb.PlayerInputEnvelope) {}
func (noopEmitter) EmitWealth(*eventspb.WealthEnvelope)           {}
