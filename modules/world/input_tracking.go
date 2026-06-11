// Package world: InputTracking per-player input-event accumulation.
// 254 rewrite of TS engine/entity/tracking/InputTracking.ts @2e3bcf43.
// The 225/244-era windowed state machine (TRACKING_RATE scheduling,
// enable/disable windows, EVENT_TRACKING blob uploads) was replaced
// upstream by a simple event-accumulation model: the client sends four
// discrete event packets (mouse click/move, applet focus, camera
// position) and the server appends them into an accumulation buffer
// that is flushed to the logging backend.
//
// rev-254 A5 (43e02957 → 2e3bcf43 delta):
//   - max=500 replaced by softLimit=1500 (onCycle flush threshold).
//   - Per-event overflow checks now compare against the BUFFER CAPACITY
//     (`this.buf.pos + N >= this.buf.length`, where buf = Packet.alloc(1)
//     = 5000 bytes, Packet.ts:128 @2e3bcf43), not against the old max.
//   - seq removed; InputTrackingBlob.ts DELETED. flush submits the raw
//     buffer bytes via World.submitInputTracking(player, subarray) —
//     blob assembly (session_uuid/timestamp/base64) moved receiver-side
//     (World.ts:2326-2333 @2e3bcf43).
package world

import (
	"github.com/zsrv/goscape/pkg/io/packet"
)

// inputTrackingSoftLimit is the OnCycle flush threshold for the
// accumulation buffer. TS InputTracking.ts:21 @2e3bcf43
// (`softLimit: number = 1500`; replaces the 43e02957-era max=500).
const inputTrackingSoftLimit = 1500

// inputTrackingBufCap is the accumulation-buffer capacity used by the
// per-event overflow checks (`this.buf.pos + N >= this.buf.length`).
// TS buf = Packet.alloc(1) (InputTracking.ts:24) which allocates a
// 5000-byte Uint8Array (Packet.ts:128 @2e3bcf43); Packet.length is the
// view byteLength, i.e. the alloc'd capacity. goscape's buffer is a
// growable bytes.Buffer, so the capacity is modeled as this constant.
const inputTrackingBufCap = 5000

// Event-type tags inside the accumulated blob. TS InputTracking.ts:12-17
// (enum InputTrackingEvent — CAMERA_POSITION=1 with implicit increments
// at 2e3bcf43; values identical to the 43e02957 explicit ones).
const (
	inputEventCameraPosition = 1
	inputEventAppletFocus    = 2
	inputEventMouseClick     = 3
	inputEventMouseMove      = 4
)

// InputTracking accumulates client input events into a buffer and
// flushes the raw bytes to the logger bridge. 254 rewrite of the
// 225/244-era windowed state machine — there is no scheduling: Active
// is set externally (friends-server relay, report-abuse flag) and
// events are appended inline by the four EVENT_* packet handlers.
// TS InputTracking.ts @2e3bcf43.
type InputTracking struct {
	// player is the back-pointer used for:
	//  - the flush submission (TS flush passes this.player to
	//    World.submitInputTracking, InputTracking.ts:42)
	//  - client.server.loggerBridge access (SubmitInputTracking)
	player *Player

	// Active gates all recording and flushing. TS InputTracking.ts:23
	// (`active: boolean = false`). Set sites: friends relay RELAY_TRACK
	// (TS World.ts `player.input.active = state`), report-abuse
	// MACROING/BUG_ABUSE offender (TS World.ts:2314
	// `offenderPlayer.input.active = true` @2e3bcf43).
	Active bool
	// buf is the event-accumulation buffer. TS InputTracking.ts:24
	// (`buf: Packet = Packet.alloc(1)` — a 5000-byte backing array; the
	// buffer starts empty, hence NewPacket(nil); see inputTrackingBufCap).
	buf *packet.Packet
}

// NewInputTracking allocates a fresh InputTracking for player. Mirrors
// the TS InputTracking constructor (InputTracking.ts:26-28); allocated
// from the Player constructor (TS Player.ts
// `this.input = new InputTracking(this)`).
func NewInputTracking(player *Player) *InputTracking {
	return &InputTracking{player: player, buf: packet.NewPacket(nil)}
}

// OnCycle flushes when the buffer has reached the soft limit.
// TS InputTracking.ts:30-34 (`if (this.buf.pos >= this.softLimit)`);
// called per tick from Player.processInputTracking.
func (t *InputTracking) OnCycle() {
	if t.buf.Len() >= inputTrackingSoftLimit {
		t.Flush()
	}
}

// Flush submits the accumulated raw bytes via the logger bridge and
// resets the buffer. TS InputTracking.ts:36-46:
//
//	World.submitInputTracking(this.player, this.buf.data.subarray(0, this.buf.pos))
//
// No-op when inactive — TS checks active FIRST, so an inactive flush
// neither submits nor resets buffered bytes. An empty buffer submits
// nothing (the submit sits inside the buf.pos > 0 branch). seq and the
// InputTrackingBlob wrapper are GONE at 2e3bcf43 — session_uuid,
// timestamp, and base64 assembly live receiver-side
// (World.submitInputTracking, World.ts:2326-2333). Also called from
// the logout cleanup path (TS Player.cleanup).
func (t *InputTracking) Flush() {
	if !t.Active {
		return
	}
	if t.buf.Len() > 0 {
		// nil-guards are goscape-defensive (test/teardown paths); TS
		// reaches World.submitInputTracking via static accessor. The
		// byte slice aliases the buffer (TS subarray); bridge impls
		// must copy/encode before returning.
		if t.player.client != nil && t.player.client.server != nil {
			t.player.client.server.loggerBridge.SubmitInputTracking(t.player, t.buf.Bytes())
		}
	}
	t.buf.Reset()
}

// CameraPosition appends event 1: p1(tag) p2(pitch) p2(yaw).
// TS InputTracking.ts:48-60 (overflow: pos + 5 >= buf.length).
func (t *InputTracking) CameraPosition(pitch, yaw uint16) {
	if !t.Active {
		return
	}
	if t.buf.Len()+5 >= inputTrackingBufCap {
		t.Flush()
	}
	t.buf.P1(inputEventCameraPosition)
	t.buf.P2(pitch)
	t.buf.P2(yaw)
}

// AppletFocus appends event 2: p1(tag) p1(focus).
// TS InputTracking.ts:62-73 (overflow: pos + 2 >= buf.length).
func (t *InputTracking) AppletFocus(focus uint8) {
	if !t.Active {
		return
	}
	if t.buf.Len()+2 >= inputTrackingBufCap {
		t.Flush()
	}
	t.buf.P1(inputEventAppletFocus)
	t.buf.P1(focus)
}

// MouseClick appends event 3: p1(tag) p4(info).
// TS InputTracking.ts:75-86 (overflow: pos + 5 >= buf.length).
func (t *InputTracking) MouseClick(info uint32) {
	if !t.Active {
		return
	}
	if t.buf.Len()+5 >= inputTrackingBufCap {
		t.Flush()
	}
	t.buf.P1(inputEventMouseClick)
	t.buf.P4(info)
}

// MouseMove appends event 4: p1(tag) p1(len) pdata; rejects len==0 or
// len>160. NOTE the TS overflow check adds only len(data) — NOT the 2
// tag/len bytes — so the append can overshoot the capacity check by up
// to 2 bytes (TS InputTracking.ts:93; quirk kept — goscape's growable
// buffer just grows where the fixed TS array would be at its edge).
func (t *InputTracking) MouseMove(data []byte) {
	if !t.Active || len(data) == 0 || len(data) > 160 {
		return
	}
	if t.buf.Len()+len(data) >= inputTrackingBufCap {
		t.Flush()
	}
	t.buf.P1(inputEventMouseMove)
	t.buf.P1(uint8(len(data)))
	t.buf.PData(data)
}
