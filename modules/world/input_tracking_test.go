package world

import (
	"bytes"
	"encoding/base64"
	"testing"

	"github.com/zsrv/goscape/pkg/coordgrid"
)

// inputTrackingTestSetup wires a Player against a Server with
// recordingBridges. Returns the tracking entity, player, and the
// recorder. The InputTracking is built via NewInputTracking (the same
// ctor newPlayer uses) with Active left false — tests flip it per-case.
func inputTrackingTestSetup(t *testing.T) (*InputTracking, *Player, *recordingBridges) {
	t.Helper()
	s := newTestServer(t)
	p, _ := newTestPlayer(t)
	p.client.server = s
	rec := installRecordingBridges(s)
	tt := NewInputTracking(p)
	p.input = tt
	return tt, p, rec
}

// TestInputTrackingCameraPosition pins event 1: p1(tag) p2(pitch) p2(yaw)
// appended only while Active. TS InputTracking.ts:54-66 @43e02957.
func TestInputTrackingCameraPosition(t *testing.T) {
	t.Run("active-appends", func(t *testing.T) {
		tt, _, _ := inputTrackingTestSetup(t)
		tt.Active = true
		tt.CameraPosition(0x1234, 0x5678)
		want := []byte{0x01, 0x12, 0x34, 0x56, 0x78}
		if got := tt.buf.Bytes(); !bytes.Equal(got, want) {
			t.Errorf("buf: got % X, want % X", got, want)
		}
	})
	t.Run("inactive-noop", func(t *testing.T) {
		tt, _, _ := inputTrackingTestSetup(t)
		tt.Active = false
		tt.CameraPosition(0x1234, 0x5678)
		if got := tt.buf.Len(); got != 0 {
			t.Errorf("buf len: got %d, want 0 (inactive must not record)", got)
		}
	})
}

// TestInputTrackingAppletFocus pins event 2: p1(tag) p1(focus).
// TS InputTracking.ts:68-79.
func TestInputTrackingAppletFocus(t *testing.T) {
	tt, _, _ := inputTrackingTestSetup(t)
	tt.Active = true
	tt.AppletFocus(1)
	want := []byte{0x02, 0x01}
	if got := tt.buf.Bytes(); !bytes.Equal(got, want) {
		t.Errorf("buf: got % X, want % X", got, want)
	}
}

// TestInputTrackingMouseClick pins event 3: p1(tag) p4(info).
// TS InputTracking.ts:81-92.
func TestInputTrackingMouseClick(t *testing.T) {
	tt, _, _ := inputTrackingTestSetup(t)
	tt.Active = true
	tt.MouseClick(0xDEADBEEF)
	want := []byte{0x03, 0xDE, 0xAD, 0xBE, 0xEF}
	if got := tt.buf.Bytes(); !bytes.Equal(got, want) {
		t.Errorf("buf: got % X, want % X", got, want)
	}
}

// TestInputTrackingMouseMove pins event 4: p1(tag) p1(len) pdata, with
// the len==0 and len>160 rejections. TS InputTracking.ts:94-106.
func TestInputTrackingMouseMove(t *testing.T) {
	cases := []struct {
		name string
		data []byte
		want []byte // nil = no-op
	}{
		{"len-0-noop", []byte{}, nil},
		{"len-161-noop", make([]byte, 161), nil},
		{"len-160-appends", make([]byte, 160), append([]byte{0x04, 160}, make([]byte, 160)...)},
		{"len-3-appends", []byte{0xAA, 0xBB, 0xCC}, []byte{0x04, 0x03, 0xAA, 0xBB, 0xCC}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tt, _, _ := inputTrackingTestSetup(t)
			tt.Active = true
			tt.MouseMove(tc.data)
			got := tt.buf.Bytes()
			if tc.want == nil {
				if len(got) != 0 {
					t.Errorf("buf: got % X, want empty (no-op)", got)
				}
				return
			}
			if !bytes.Equal(got, tc.want) {
				t.Errorf("buf: got % X, want % X", got, tc.want)
			}
		})
	}
}

// TestInputTrackingMultiEventConcatenation hand-computes a
// camera+focus+click buffer and asserts the exact concatenation —
// the self-review byte-layout check.
func TestInputTrackingMultiEventConcatenation(t *testing.T) {
	tt, _, _ := inputTrackingTestSetup(t)
	tt.Active = true
	tt.CameraPosition(0x0102, 0x0304)
	tt.AppletFocus(0)
	tt.MouseClick(0x0A0B0C0D)
	want := []byte{
		0x01, 0x01, 0x02, 0x03, 0x04, // camera: tag p2(pitch) p2(yaw)
		0x02, 0x00, // focus: tag p1(focus)
		0x03, 0x0A, 0x0B, 0x0C, 0x0D, // click: tag p4(info)
	}
	if got := tt.buf.Bytes(); !bytes.Equal(got, want) {
		t.Errorf("buf: got % X, want % X", got, want)
	}
}

// TestInputTrackingOnCycleFlushThreshold pins OnCycle's >=500 flush:
// one blob via the logger bridge with seq 0, then the NEXT flush uses
// seq 1 — seq never resets. TS InputTracking.ts:34-38,47.
func TestInputTrackingOnCycleFlushThreshold(t *testing.T) {
	tt, p, rec := inputTrackingTestSetup(t)
	tt.Active = true
	p.username = "alice"
	p.session = "sess-uuid-1"
	p.level, p.x, p.z = 0, 3200, 3200

	// 100 mouse clicks x 5 bytes = exactly 500 buffered bytes.
	for range 100 {
		tt.MouseClick(0x11223344)
	}
	if got := tt.buf.Len(); got != 500 {
		t.Fatalf("preflight buf len: got %d, want 500", got)
	}

	tt.OnCycle()

	if got := len(rec.inputTracks); got != 1 {
		t.Fatalf("bridge calls after OnCycle at 500: got %d, want 1", got)
	}
	call := rec.inputTracks[0]
	if call.username != "alice" {
		t.Errorf("username: got %q, want alice", call.username)
	}
	if call.sessionUUID != "sess-uuid-1" {
		t.Errorf("sessionUUID: got %q, want sess-uuid-1", call.sessionUUID)
	}
	if got := len(call.blobs); got != 1 {
		t.Fatalf("blobs per flush: got %d, want 1 (ONE blob per flush at 254)", got)
	}
	b := call.blobs[0]
	if b.Seq != 0 {
		t.Errorf("first blob Seq: got %d, want 0 (254 is 0-based)", b.Seq)
	}
	wantCoord := coordgrid.PackCoord(0, 3200, 3200)
	if b.Coord != wantCoord {
		t.Errorf("Coord: got %d, want %d", b.Coord, wantCoord)
	}
	if wantLen := base64.StdEncoding.EncodedLen(500); len(b.Data) != wantLen {
		t.Errorf("Data base64 len: got %d, want %d", len(b.Data), wantLen)
	}
	if got := tt.buf.Len(); got != 0 {
		t.Errorf("buf len after flush: got %d, want 0", got)
	}

	// OnCycle below threshold is a no-op.
	tt.MouseClick(0x11223344)
	tt.OnCycle()
	if got := len(rec.inputTracks); got != 1 {
		t.Fatalf("bridge calls after sub-threshold OnCycle: got %d, want 1", got)
	}

	// Second explicit flush carries seq 1 — never reset.
	tt.Flush()
	if got := len(rec.inputTracks); got != 2 {
		t.Fatalf("bridge calls after second flush: got %d, want 2", got)
	}
	if got := rec.inputTracks[1].blobs[0].Seq; got != 1 {
		t.Errorf("second blob Seq: got %d, want 1 (monotonic, never reset)", got)
	}
}

// TestInputTrackingAppendOverflowFlushesFirst pins the pre-append
// overflow flush: when bufLen + n > 500 the buffer is flushed FIRST,
// then the event appended. TS InputTracking.ts:59-61 (and the parallel
// checks in each event method).
func TestInputTrackingAppendOverflowFlushesFirst(t *testing.T) {
	t.Run("496-plus-5-flushes", func(t *testing.T) {
		tt, _, rec := inputTrackingTestSetup(t)
		tt.Active = true
		fillInputBuffer(t, tt, 496)
		tt.MouseClick(0xCAFEBABE) // 496+5 > 500 → flush first, then append
		if got := len(rec.inputTracks); got != 1 {
			t.Fatalf("bridge calls: got %d, want 1 (overflow must flush first)", got)
		}
		if got := base64.StdEncoding.EncodedLen(496); len(rec.inputTracks[0].blobs[0].Data) != got {
			t.Errorf("flushed blob carries pre-append bytes: data len got %d, want %d",
				len(rec.inputTracks[0].blobs[0].Data), got)
		}
		want := []byte{0x03, 0xCA, 0xFE, 0xBA, 0xBE}
		if got := tt.buf.Bytes(); !bytes.Equal(got, want) {
			t.Errorf("buf after overflow append: got % X, want % X", got, want)
		}
	})
	t.Run("495-plus-5-appends", func(t *testing.T) {
		tt, _, rec := inputTrackingTestSetup(t)
		tt.Active = true
		fillInputBuffer(t, tt, 495)
		tt.MouseClick(0xCAFEBABE) // 495+5 == 500, NOT > 500 → no flush
		if got := len(rec.inputTracks); got != 0 {
			t.Fatalf("bridge calls: got %d, want 0 (495+5==500 must append)", got)
		}
		if got := tt.buf.Len(); got != 500 {
			t.Errorf("buf len: got %d, want 500", got)
		}
	})
	t.Run("mousemove-overflow-counts-data-only", func(t *testing.T) {
		// TS quirk: mouseMove's overflow check adds only data.length —
		// NOT the 2 tag/len bytes. At bufLen=498 with len(data)=2,
		// 498+2 == 500 is NOT > 500, so it appends WITHOUT flushing and
		// the buffer overshoots to 502. TS InputTracking.ts:99.
		tt, _, rec := inputTrackingTestSetup(t)
		tt.Active = true
		fillInputBuffer(t, tt, 498)
		tt.MouseMove([]byte{0xEE, 0xFF})
		if got := len(rec.inputTracks); got != 0 {
			t.Fatalf("bridge calls: got %d, want 0 (quirk: tag/len bytes not counted)", got)
		}
		if got := tt.buf.Len(); got != 502 {
			t.Errorf("buf len: got %d, want 502 (498 + tag + len + 2 data)", got)
		}
	})
}

// fillInputBuffer appends applet-focus events (2 bytes each) plus
// mouse-move padding until the accumulation buffer holds exactly n bytes.
func fillInputBuffer(t *testing.T, tt *InputTracking, n int) {
	t.Helper()
	for tt.buf.Len()+2 <= n {
		tt.AppletFocus(1)
	}
	if rem := n - tt.buf.Len(); rem > 0 {
		// Odd remainder: a 1-byte raw write keeps the exact target;
		// only the length matters for threshold tests.
		tt.buf.P1(0)
	}
	if tt.buf.Len() != n {
		t.Fatalf("fillInputBuffer: got %d, want %d", tt.buf.Len(), n)
	}
}

// TestInputTrackingFlushEmptyBuffer pins that Flush with an empty
// buffer submits nothing and does NOT bump seq. TS InputTracking.ts:45
// (`if (this.buf.pos > 0)` wraps both the submit and the seq++).
func TestInputTrackingFlushEmptyBuffer(t *testing.T) {
	tt, _, rec := inputTrackingTestSetup(t)
	tt.Active = true

	tt.Flush()
	if got := len(rec.inputTracks); got != 0 {
		t.Fatalf("bridge calls: got %d, want 0", got)
	}
	if tt.seq != 0 {
		t.Errorf("seq after empty flush: got %d, want 0 (must not bump)", tt.seq)
	}

	// Next real flush still carries seq 0.
	tt.MouseClick(1)
	tt.Flush()
	if got := len(rec.inputTracks); got != 1 {
		t.Fatalf("bridge calls: got %d, want 1", got)
	}
	if got := rec.inputTracks[0].blobs[0].Seq; got != 0 {
		t.Errorf("Seq: got %d, want 0", got)
	}
}

// TestInputTrackingFlushInactiveNoop pins that Flush while !Active is a
// complete no-op even with buffered bytes — TS checks active FIRST
// (InputTracking.ts:41-43), so the buffer is neither submitted nor reset.
func TestInputTrackingFlushInactiveNoop(t *testing.T) {
	tt, _, rec := inputTrackingTestSetup(t)
	tt.Active = true
	tt.MouseClick(0x01020304)
	tt.Active = false

	tt.Flush()

	if got := len(rec.inputTracks); got != 0 {
		t.Errorf("bridge calls: got %d, want 0", got)
	}
	if got := tt.buf.Len(); got != 5 {
		t.Errorf("buf len: got %d, want 5 (inactive flush must not reset)", got)
	}
	if tt.seq != 0 {
		t.Errorf("seq: got %d, want 0", tt.seq)
	}
}

// TestInputTrackingFlushHeadlessSession pins the empty-session →
// "headless" fallback. TS InputTracking.ts:46
// (`player instanceof NetworkPlayer ? player.client.uuid : 'headless'`).
func TestInputTrackingFlushHeadlessSession(t *testing.T) {
	tt, p, rec := inputTrackingTestSetup(t)
	tt.Active = true
	p.username = "headlessbot"
	p.session = ""

	tt.AppletFocus(1)
	tt.Flush()

	if got := len(rec.inputTracks); got != 1 {
		t.Fatalf("bridge calls: got %d, want 1", got)
	}
	if got := rec.inputTracks[0].sessionUUID; got != "headless" {
		t.Errorf("sessionUUID: got %q, want headless", got)
	}
}

// ---------------------------------------------------------------------------
// InputTrackingBlob (unchanged at 254 — InputTrackingBlob.ts identical
// between pins 3c16994c and 43e02957)
// ---------------------------------------------------------------------------

// TestInputTrackingBlobCtor pins InputTrackingBlob construction:
// seq stored verbatim, data is base64 of rawData, coord stored verbatim.
// Mirrors InputTrackingBlob.ts:6-10.
func TestInputTrackingBlobCtor(t *testing.T) {
	raw := []byte{0xDE, 0xAD, 0xBE, 0xEF}
	b := NewInputTrackingBlob(raw, 3, 0xC0DE)
	if b.Seq != 3 {
		t.Errorf("Seq: got %d, want 3", b.Seq)
	}
	wantData := base64.StdEncoding.EncodeToString(raw)
	if b.Data != wantData {
		t.Errorf("Data: got %q, want %q", b.Data, wantData)
	}
	if b.Coord != 0xC0DE {
		t.Errorf("Coord: got %d, want 0xC0DE", b.Coord)
	}
}

// TestInputTrackingBlobCtorZeroCoord verifies that a zero coord is
// stored (not dropped), matching InputTrackingBlob.ts:4 (coord?: number).
func TestInputTrackingBlobCtorZeroCoord(t *testing.T) {
	b := NewInputTrackingBlob([]byte{0x01}, 1, 0)
	if b.Coord != 0 {
		t.Errorf("Coord: got %d, want 0", b.Coord)
	}
}

// ---------------------------------------------------------------------------
// 254 event-packet handlers (handler_events.go)
// ---------------------------------------------------------------------------

// TestHandleEventPackets pins the four thin handlers' decode + dispatch
// against the TS codec files @43e02957:
//   - EVENT_MOUSE_CLICK:     info  = g4  (EventMouseClickDecoder.ts)
//   - EVENT_MOUSE_MOVE:      data  = raw payload (EventMouseMoveDecoder.ts)
//   - EVENT_APPLET_FOCUS:    focus = g1  (EventAppletFocusDecoder.ts)
//   - EVENT_CAMERA_POSITION: pitch = g2, yaw = g2 (EventCameraPositionDecoder.ts)
func TestHandleEventPackets(t *testing.T) {
	cases := []struct {
		name    string
		handler func(*Player, []byte) error
		payload []byte
		want    []byte
	}{
		{"mouse-click", handleEventMouseClick,
			[]byte{0xDE, 0xAD, 0xBE, 0xEF},
			[]byte{0x03, 0xDE, 0xAD, 0xBE, 0xEF}},
		{"mouse-move", handleEventMouseMove,
			[]byte{0x10, 0x20, 0x30},
			[]byte{0x04, 0x03, 0x10, 0x20, 0x30}},
		{"applet-focus", handleEventAppletFocus,
			[]byte{0x01},
			[]byte{0x02, 0x01}},
		{"camera-position", handleEventCameraPosition,
			[]byte{0x12, 0x34, 0x56, 0x78},
			[]byte{0x01, 0x12, 0x34, 0x56, 0x78}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tt, p, _ := inputTrackingTestSetup(t)
			tt.Active = true
			if err := tc.handler(p, tc.payload); err != nil {
				t.Fatalf("handler: %v", err)
			}
			if got := tt.buf.Bytes(); !bytes.Equal(got, tc.want) {
				t.Errorf("buf: got % X, want % X", got, tc.want)
			}
		})
	}
}
