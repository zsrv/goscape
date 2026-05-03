package world

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
	return nil
}
