// Package world: NAI-73 InputTracking per-player anti-cheat input-recording
// state machine. Line-by-line port of TS engine/entity/tracking/
// InputTracking.ts. Closes NAI-72-D-INPUT-RECORDING-NOT-PORTED.
package world

import (
	"encoding/base64"
	"math/rand/v2"

	"github.com/zsrv/goscape/pkg/coordgrid"
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

// InputTrackingBlob is a single recorded EVENT_TRACKING payload, wrapped
// with sequence number and player coordinate. Mirrors TS
// InputTrackingBlob.ts:1-11.
//
//   - Seq: 1-based sequence index within the recording window.
//   - Data: base64-encoded raw client payload (mirrors TS
//     Buffer.from(data).toString('base64') at InputTrackingBlob.ts:8).
//   - Coord: packed player coordinate (coordgrid.PackCoord) at the
//     moment Record was called; mirrors TS InputTracking.ts:135
//     `this.player.coord`.
type InputTrackingBlob struct {
	Seq   int
	Data  string // base64
	Coord int
}

// NewInputTrackingBlob constructs an InputTrackingBlob from raw bytes.
// seq is the 1-based sequence index; coord is the packed player coord.
// Mirrors TS InputTrackingBlob constructor (InputTrackingBlob.ts:6-10).
func NewInputTrackingBlob(data []byte, seq, coord int) InputTrackingBlob {
	return InputTrackingBlob{
		Seq:   seq,
		Data:  base64.StdEncoding.EncodeToString(data),
		Coord: coord,
	}
}

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

	// recordedBlobs: accumulated EVENT_TRACKING payloads for this window,
	// each wrapped in an InputTrackingBlob (seq, base64 data, coord).
	// ALL blobs are submitted at submitEvents (244 changed from 225 which
	// sent only recordedBlobs[0]). TS InputTracking.ts:33.
	recordedBlobs []InputTrackingBlob
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

// Record wraps rawData in an InputTrackingBlob and appends to recordedBlobs.
// sizeTotal is updated with the RAW length BEFORE the push (TS line 134).
// seq = len(recordedBlobs) + 1 (1-based, evaluated before push, TS line 135).
// coord = player's packed coord at call time, mirrors TS `this.player.coord`.
// Mirrors TS record (InputTracking.ts:133-135). Caller is responsible for
// gating (the handler checks IsActive, ShouldSubmitTrackingDetails, and
// recordedBlobsSizeTotal cap before calling Record).
func (t *InputTracking) Record(rawData []byte) {
	t.recordedBlobsSizeTotal += len(rawData) // TS line 134: accumulate BEFORE push
	seq := len(t.recordedBlobs) + 1          // 1-based (TS: recordedBlobs.length + 1 before push)
	coord := coordgrid.PackCoord(t.player.level, t.player.x, t.player.z)
	t.recordedBlobs = append(t.recordedBlobs, NewInputTrackingBlob(rawData, seq, coord))
}

// enable transitions tracking to active. Mirrors TS enable (lines 94-103).
// Called from OnCycle only.
func (t *InputTracking) enable(currentTick int) {
	if t.enabled {
		return
	}
	t.enabled = true
	t.startTrackingAt = currentTick // enabled immediately
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
	t.endTrackingAt = currentTick                                 // disabled immediately
	t.waitingForRemainingData = true
	t.player.WriteFinishTracking()
}

// submitEvents finalises the window. Mirrors TS submitEvents (lines 140-158).
// Branches:
//   - hasSeenReport && shouldSubmit → loggerBridge.SubmitInputTracking(username,
//     sessionUUID, ALL recordedBlobs). 244 sends ALL blobs (TS line 147);
//     225 sent only recordedBlobs[0] — that quirk is removed.
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
			// 244 submit shape: username + session UUID + ALL blobs.
			// Session-string fork mirrors TS InputTracking.ts:147:
			//   player instanceof NetworkPlayer ? player.client.uuid : 'headless'
			// goscape (player.go:1469-1497): p.session is set from the login
			// UUID (NetworkPlayer path); empty session falls back to "headless".
			sessionUUID := t.player.session
			if sessionUUID == "" {
				sessionUUID = "headless"
			}
			s.loggerBridge.SubmitInputTracking(t.player.username, sessionUUID, t.recordedBlobs)
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
