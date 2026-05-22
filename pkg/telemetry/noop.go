package telemetry

import "github.com/zsrv/goscape/pkg/eventspb"

type noopEmitter struct{}

func (noopEmitter) EmitAuth(*eventspb.AuthEnvelope) {}
