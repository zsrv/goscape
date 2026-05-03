// Package world: NAI-73 InputTracking per-player anti-cheat input-recording
// state machine. Line-by-line port of TS engine/entity/tracking/
// InputTracking.ts. Closes NAI-72-D-INPUT-RECORDING-NOT-PORTED.
package world

import (
	"math/rand/v2"
)

// Timing constants — mirror TS InputTracking.ts:10-14.
const (
	// inputTrackingRate is the number of ticks between scheduled tracking
	// sessions (~120s at 600ms/tick). TS InputTracking.TRACKING_RATE.
	inputTrackingRate = 200
	// inputTrackingTime is the duration of each tracking window
	// (~90s). TS InputTracking.TRACKING_TIME.
	inputTrackingTime = 150
	// inputTrackingRemainingDataUploadLeeway is the grace period after a
	// tracking window closes during which the client may still flush
	// trailing EVENT_TRACKING blobs (~10s). TS
	// InputTracking.REMAINING_DATA_UPLOAD_LEEWAY.
	inputTrackingRemainingDataUploadLeeway = 16
	// inputTrackingJitterRange is the absolute bound on the
	// per-player random offset added to the first scheduled
	// tracking-start tick. Yields uniform [-15, +15]. TS InputTracking.offset(15).
	inputTrackingJitterRange = 15
	// inputTrackingMaxBlobBytes is the per-packet upper bound on the
	// EVENT_TRACKING client payload (the handler-side gate). TS
	// EventTrackingHandler.ts:9 (`bytes.length > 500`).
	inputTrackingMaxBlobBytes = 500
)

// InputTracking is the per-player input-recording state machine. Mirrors
// TS InputTracking class. One instance per logged-in Player, allocated
// in processLogins. Owns scheduling (start/end window ticks), recorded
// blob accumulation, and end-of-window submission to the LoggerBridge.
type InputTracking struct {
	// player is the back-pointer used for:
	//  - reading submitInput (shouldSubmitTrackingDetails)
	//  - reading client.server.cfg (NodeSubmitInput, NodeDebug,
	//    NodeLimitBytesPerTrackingSession)
	//  - reading client.server.loggerBridge (submitEvents)
	//  - writing requestIdleLogout (submitEvents kick branch)
	//  - calling WriteEnableTracking / WriteFinishTracking (enable/disable)
	player *Player

	// hasSeenReport: at least one EVENT_TRACKING report received this
	// session window. Pinned by EventTrackingHandler.handle.
	// TS InputTracking.ts:19.
	hasSeenReport bool
	// waitingForRemainingData: tracking window has closed but the
	// REMAINING_DATA_UPLOAD_LEEWAY grace has not yet expired. TS
	// InputTracking.ts:21.
	waitingForRemainingData bool
	// enabled: tracking is currently active (between startTrackingAt and
	// endTrackingAt, inclusive). TS InputTracking.ts:24.
	enabled bool

	// startTrackingAt: tick at which the next/current tracking window opens.
	// TS InputTracking.ts:27.
	startTrackingAt int
	// endTrackingAt: tick at which the next/current tracking window closes.
	// TS InputTracking.ts:30.
	endTrackingAt int

	// recordedBlobs: accumulated EVENT_TRACKING payloads for this window.
	// Submitted (as recordedBlobs[0] only — TS quirk) at submitEvents.
	// TS InputTracking.ts:33.
	recordedBlobs [][]byte
	// recordedBlobsSizeTotal: byte total across all recordedBlobs. Compared
	// against cfg.NodeLimitBytesPerTrackingSession by the handler. TS
	// InputTracking.ts:35.
	recordedBlobsSizeTotal int
}

// NewInputTracking allocates a fresh InputTracking for player. Initial
// startTrackingAt is set to currentTick + inputTrackingRate + jitter
// (uniform [-15, +15]); endTrackingAt is startTrackingAt +
// inputTrackingTime. Mirrors TS InputTracking constructor (line 37-39
// + initial-value expressions on lines 27, 30).
func NewInputTracking(player *Player, currentTick int) *InputTracking {
	t := &InputTracking{player: player}
	t.startTrackingAt = t.nextScheduledTrackingStart(currentTick)
	t.endTrackingAt = t.startTrackingAt + inputTrackingTime
	return t
}

// nextScheduledTrackingStart returns the tick at which the next
// tracking session should start. Mirrors TS
// InputTracking.nextScheduledTrackingStart (lines 44-46).
func (t *InputTracking) nextScheduledTrackingStart(currentTick int) int {
	return currentTick + inputTrackingRate + offset(inputTrackingJitterRange)
}

// shouldStartTracking returns true when the current tick has reached or
// passed startTrackingAt. Mirrors TS line 58-60.
func (t *InputTracking) shouldStartTracking(currentTick int) bool {
	return currentTick >= t.startTrackingAt
}

// shouldEndTracking returns true when the current tick has reached or
// passed endTrackingAt. Mirrors TS line 65-67.
func (t *InputTracking) shouldEndTracking(currentTick int) bool {
	return currentTick >= t.endTrackingAt
}

// IsActive reports whether the tracking window is currently open or in
// the post-close grace period. Mirrors TS isActive (lines 117-120).
// Consumed by the EVENT_TRACKING handler as its second gate.
func (t *InputTracking) IsActive(currentTick int) bool {
	withinTicks := currentTick >= t.startTrackingAt && currentTick <= t.endTrackingAt
	return withinTicks || t.waitingForRemainingData
}

// ShouldSubmitTrackingDetails reports whether the player should
// actually submit blob data (vs just acknowledging IsActive). Mirrors
// TS shouldSubmitTrackingDetails (lines 126-128).
func (t *InputTracking) ShouldSubmitTrackingDetails() bool {
	if t.player == nil || t.player.client == nil || t.player.client.server == nil {
		return false
	}
	return t.player.submitInput || t.player.client.server.cfg.NodeSubmitInput
}

// Record appends rawData to recordedBlobs and grows the size total.
// Mirrors TS record (lines 130-133). Caller is responsible for any
// gating (the handler checks IsActive, ShouldSubmitTrackingDetails,
// and recordedBlobsSizeTotal cap before calling Record).
func (t *InputTracking) Record(rawData []byte) {
	t.recordedBlobsSizeTotal += len(rawData)
	t.recordedBlobs = append(t.recordedBlobs, rawData)
}

// enable transitions tracking to active. Mirrors TS enable (lines 94-103).
// Called from OnCycle only.
func (t *InputTracking) enable(currentTick int) {
	if t.enabled {
		return
	}
	t.enabled = true
	t.startTrackingAt = currentTick                         // enabled immediately
	t.endTrackingAt = t.startTrackingAt + inputTrackingTime
	t.player.WriteEnableTracking()
}

// disable transitions tracking to inactive and starts the
// REMAINING_DATA_UPLOAD_LEEWAY grace. Mirrors TS disable (lines 105-115).
// Called from OnCycle only.
func (t *InputTracking) disable(currentTick int) {
	if !t.enabled {
		return
	}
	t.enabled = false
	t.startTrackingAt = t.nextScheduledTrackingStart(currentTick) // at the next interval
	t.endTrackingAt = currentTick                                  // disabled immediately
	t.waitingForRemainingData = true
	t.player.WriteFinishTracking()
}

// submitEvents finalises the window. Mirrors TS submitEvents (lines 140-158).
// Branches:
//   - hasSeenReport && shouldSubmit → loggerBridge.SubmitInputTracking(player, recordedBlobs[0])
//     (TS submits only blob index 0, even when multiple blobs were recorded — quirk preserved).
//   - !hasSeenReport && !cfg.NodeDebug → ENGINE session log "Client did
//     not submit an input tracking report" + requestIdleLogout = true
//     (TS InputTracking.ts:150; ported in NAI-74).
//
// All branches reset waitingForRemainingData / recordedBlobs /
// recordedBlobsSizeTotal / hasSeenReport (TS lines 154-157).
func (t *InputTracking) submitEvents() {
	s := t.player.client.server
	if t.hasSeenReport {
		if t.ShouldSubmitTrackingDetails() {
			s.loggerBridge.SubmitInputTracking(t.player, t.recordedBlobs[0])
		}
	} else if !s.cfg.NodeDebug {
		// NAI-74: NAI-73-D close. Per TS InputTracking.ts:150 — emits
		// an ENGINE session log noting the missed report alongside the
		// idle-logout request.
		t.player.AddSessionLog(LoggerEventTypeEngine,
			"Client did not submit an input tracking report")
		t.player.requestIdleLogout = true
	}
	t.waitingForRemainingData = false
	t.recordedBlobs = nil
	t.recordedBlobsSizeTotal = 0
	t.hasSeenReport = false
}

// OnCycle is the per-tick state-machine dispatch. Mirrors TS onCycle
// (lines 73-92). Called from Player.processInputTracking, which is
// called from the last line of Player.processIn (mirrors TS World.ts:646
// — same per-player iteration of the client-input phase).
func (t *InputTracking) OnCycle(currentTick int) {
	if t.waitingForRemainingData {
		if t.endTrackingAt+inputTrackingRemainingDataUploadLeeway < currentTick {
			t.submitEvents()
		}
		return
	}
	if t.shouldStartTracking(currentTick) && !t.enabled {
		t.enable(currentTick)
		return
	}
	if t.shouldEndTracking(currentTick) && t.enabled {
		t.disable(currentTick)
		return
	}
}

// offset returns a uniform random integer in [-n, +n]. Mirrors TS
// offset (lines 160-162) which uses Math.random; goscape uses
// math/rand/v2 package-level rand.IntN per the existing convention
// (npc_interaction.go:86, npc_hunt.go:82).
func offset(n int) int {
	return rand.IntN(n*2+1) - n
}
