package world

import (
	"uuid"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/zsrv/goscape/pkg/eventspb"
	"github.com/zsrv/goscape/pkg/telemetry"
)

// handleEventTracking handles client opcode 81 (EVENT_TRACKING),
// payload size -2 (2-byte length prefix), category RESTRICTED_EVENT.
//
// Mirrors TS EventTrackingHandler.handle (EventTrackingHandler.ts:7-28).
// Branch order:
//  1. len ∈ (0, 500] gate
//  2. p.input.IsActive(currentTick) gate
//  3. p.input.hasSeenReport = true
//  4. !p.input.ShouldSubmitTrackingDetails() → return early (no record)
//  5. p.input.recordedBlobsSizeTotal > cap gate
//  6. p.input.Record(payload)
//
// All gates that "fail" return nil (TS returns false from the handler;
// goscape signature is `error` — nil means "handled, drop").
func handleEventTracking(p *Player, payload []byte) error {
	if p.input == nil {
		return nil
	}
	n := len(payload)
	if n == 0 || n > inputTrackingMaxBlobBytes {
		return nil
	}
	if p.client == nil || p.client.server == nil {
		return nil
	}
	currentTick := p.client.server.currentTick
	if !p.input.IsActive(currentTick) {
		return nil
	}
	p.input.hasSeenReport = true
	if !p.input.ShouldSubmitTrackingDetails() {
		return nil
	}
	if p.input.recordedBlobsSizeTotal > p.client.server.cfg.NodeLimitBytesPerTrackingSession {
		return nil
	}
	// Defensive copy: payload may alias the read buffer.
	cp := make([]byte, n)
	copy(cp, payload)
	p.input.Record(cp)

	// NAI-Phase2: emit MouseMoveEvent. The EVENT_TRACKING payload is an
	// opaque packed format consumed by p.input.Record; the TS engine
	// (EventTrackingHandler.ts) likewise treats the bytes as opaque and
	// never decodes mouse coords server-side. The X/Y fields stay zero
	// here for TS-parity — the canonical mouse trail lives in the raw
	// blob persisted by p.input.Record. Surfacing per-event coords would
	// require a Go-side decoder for the client's packed format (no TS
	// precedent; not in Phase 2 scope).
	telemetry.Get().EmitPlayerInput(&eventspb.PlayerInputEnvelope{
		SchemaVersion: 1,
		EventId:       uuid.New().String(),
		Ts:            timestamppb.Now(),
		WorldId:       int32(p.client.server.cfg.NodeID),
		AccountId:     p.accountID,
		Payload: &eventspb.PlayerInputEnvelope_MouseMove{
			MouseMove: &eventspb.MouseMoveEvent{X: 0, Y: 0},
		},
	})
	return nil
}
