package telemetry

import (
	"sync/atomic"

	"github.com/zsrv/goscape/pkg/eventspb"
)

// Emitter publishes typed events to the telemetry pipeline.
// Implementations must be safe to call from many goroutines and must not block.
type Emitter interface {
	EmitAuth(*eventspb.AuthEnvelope)
	EmitWorld(*eventspb.WorldEnvelope)
	EmitPlayerInput(*eventspb.PlayerInputEnvelope)
	EmitWealth(*eventspb.WealthEnvelope)
}

var current atomic.Pointer[Emitter]

// Get returns the currently-installed Emitter, or a no-op if none.
func Get() Emitter {
	p := current.Load()
	if p == nil {
		return noopEmitter{}
	}
	return *p
}

// Set installs a new Emitter. Called by the telemetry module on start.
func Set(e Emitter) {
	current.Store(&e)
}

// Reset reverts to the no-op emitter. Called by the telemetry module on stop
// and by tests for isolation.
func Reset() {
	current.Store(nil)
}
