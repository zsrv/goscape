// Package world: InputTracking per-player input-event accumulation.
// 254 rewrite of TS engine/entity/tracking/InputTracking.ts @43e02957.
// The 225/244-era windowed state machine (TRACKING_RATE scheduling,
// enable/disable windows, EVENT_TRACKING blob uploads) was replaced
// upstream by a simple event-accumulation model: the client sends four
// discrete event packets (mouse click/move, applet focus, camera
// position) and the server appends them into a 500-byte buffer that is
// flushed to the logging backend.
package world

import (
	"encoding/base64"

	"github.com/zsrv/goscape/pkg/coordgrid"
	"github.com/zsrv/goscape/pkg/io/packet"
)

// inputTrackingMax is the flush threshold for the accumulation buffer.
// TS InputTracking.ts:24 @43e02957 (`max: number = 500`).
const inputTrackingMax = 500

// Event-type tags inside the accumulated blob. TS InputTracking.ts:15-20
// (enum InputTrackingEvent).
const (
	inputEventCameraPosition = 1
	inputEventAppletFocus    = 2
	inputEventMouseClick     = 3
	inputEventMouseMove      = 4
)

// InputTrackingBlob is a single flushed accumulation-buffer snapshot,
// wrapped with sequence number and player coordinate. Mirrors TS
// InputTrackingBlob.ts:1-11 (identical between pins 3c16994c and
// 43e02957).
//
//   - Seq: 0-based monotonic sequence index within the player session
//     (254 semantics — TS InputTracking.ts:47 `this.seq++` post-increments,
//     so the first blob carries 0; the old 244 machinery was 1-based).
//   - Data: base64-encoded raw buffer bytes (mirrors TS
//     Buffer.from(data).toString('base64') at InputTrackingBlob.ts:8).
//   - Coord: packed player coordinate (coordgrid.PackCoord) at the
//     moment of the flush; mirrors TS InputTracking.ts:47
//     `this.player.coord`.
type InputTrackingBlob struct {
	Seq   int
	Data  string // base64
	Coord int
}

// NewInputTrackingBlob constructs an InputTrackingBlob from raw bytes.
// seq is the 0-based sequence index; coord is the packed player coord.
// Mirrors TS InputTrackingBlob constructor (InputTrackingBlob.ts:6-10).
func NewInputTrackingBlob(data []byte, seq, coord int) InputTrackingBlob {
	return InputTrackingBlob{
		Seq:   seq,
		Data:  base64.StdEncoding.EncodeToString(data),
		Coord: coord,
	}
}

// InputTracking accumulates client input events into a buffer and
// flushes them to the logger bridge as InputTrackingBlobs. 254 rewrite
// of the 225/244-era windowed state machine — there is no scheduling:
// Active is set externally (friends-server relay, report-abuse flag)
// and events are appended inline by the four EVENT_* packet handlers.
// TS InputTracking.ts @43e02957.
type InputTracking struct {
	// player is the back-pointer used for:
	//  - session/username/coord on Flush
	//  - client.server.loggerBridge access (SubmitInputTracking)
	player *Player

	// Active gates all recording and flushing. TS InputTracking.ts:26
	// (`active: boolean = false`). Set sites: friends relay RELAY_TRACK
	// (TS World.ts:2075 `player.input.active = state`), report-abuse
	// MACROING/BUG_ABUSE offender (TS World.ts:2334
	// `offenderPlayer.input.active = true`).
	Active bool
	// buf is the 500-byte event-accumulation buffer. TS
	// InputTracking.ts:27 (`buf: Packet = Packet.alloc(1)`).
	buf *packet.Packet
	// seq is the monotonically-increasing blob sequence number; never
	// reset for the life of the player session. TS InputTracking.ts:28.
	seq int
}

// NewInputTracking allocates a fresh InputTracking for player. Mirrors
// the TS InputTracking constructor (InputTracking.ts:30-32); allocated
// from the Player constructor (TS Player.ts:428
// `this.input = new InputTracking(this)`).
func NewInputTracking(player *Player) *InputTracking {
	return &InputTracking{player: player, buf: packet.NewPacket(nil)}
}

// OnCycle flushes when the buffer has reached the threshold.
// TS InputTracking.ts:34-38; called per tick from
// Player.processInputTracking (TS Player.ts:1289-1291).
func (t *InputTracking) OnCycle() {
	if t.buf.Len() >= inputTrackingMax {
		t.Flush()
	}
}

// Flush submits the accumulated buffer as ONE blob via the logger
// bridge and resets the buffer. TS InputTracking.ts:40-52. No-op when
// inactive — TS checks active FIRST, so an inactive flush neither
// submits nor resets buffered bytes. An empty buffer submits nothing
// and does not bump seq (the seq++ sits inside the buf.pos > 0 branch,
// TS line 45-49). Also called from the logout cleanup path (TS
// Player.cleanup, Player.ts:458).
func (t *InputTracking) Flush() {
	if !t.Active {
		return
	}
	if t.buf.Len() > 0 {
		// Session-string fork mirrors TS InputTracking.ts:46:
		//   player instanceof NetworkPlayer ? player.client.uuid : 'headless'
		// goscape: p.session is set from the login UUID (NetworkPlayer
		// path); empty session falls back to "headless".
		sessionUUID := t.player.session
		if sessionUUID == "" {
			sessionUUID = "headless"
		}
		blob := NewInputTrackingBlob(t.buf.Bytes(), t.seq,
			coordgrid.PackCoord(t.player.level, t.player.x, t.player.z))
		t.seq++
		// nil-guards are goscape-defensive (test/teardown paths); TS
		// reaches World.submitInputTracking via static accessor.
		if t.player.client != nil && t.player.client.server != nil {
			t.player.client.server.loggerBridge.SubmitInputTracking(
				t.player.username, sessionUUID, []InputTrackingBlob{blob})
		}
	}
	t.buf.Reset()
}

// CameraPosition appends event 1: p1(tag) p2(pitch) p2(yaw).
// TS InputTracking.ts:54-66.
func (t *InputTracking) CameraPosition(pitch, yaw uint16) {
	if !t.Active {
		return
	}
	if t.buf.Len()+5 > inputTrackingMax {
		t.Flush()
	}
	t.buf.P1(inputEventCameraPosition)
	t.buf.P2(pitch)
	t.buf.P2(yaw)
}

// AppletFocus appends event 2: p1(tag) p1(focus).
// TS InputTracking.ts:68-79.
func (t *InputTracking) AppletFocus(focus uint8) {
	if !t.Active {
		return
	}
	if t.buf.Len()+2 > inputTrackingMax {
		t.Flush()
	}
	t.buf.P1(inputEventAppletFocus)
	t.buf.P1(focus)
}

// MouseClick appends event 3: p1(tag) p4(info).
// TS InputTracking.ts:81-92.
func (t *InputTracking) MouseClick(info uint32) {
	if !t.Active {
		return
	}
	if t.buf.Len()+5 > inputTrackingMax {
		t.Flush()
	}
	t.buf.P1(inputEventMouseClick)
	t.buf.P4(info)
}

// MouseMove appends event 4: p1(tag) p1(len) pdata; rejects len==0 or
// len>160. NOTE the TS overflow check adds only len(data) — NOT the 2
// tag/len bytes — so the buffer can overshoot 500 by up to 2 bytes;
// that quirk is kept. TS InputTracking.ts:94-106.
func (t *InputTracking) MouseMove(data []byte) {
	if !t.Active || len(data) == 0 || len(data) > 160 {
		return
	}
	if t.buf.Len()+len(data) > inputTrackingMax {
		t.Flush()
	}
	t.buf.P1(inputEventMouseMove)
	t.buf.P1(uint8(len(data)))
	t.buf.PData(data)
}
