package world

import (
	"bytes"
	"testing"
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
// appended only while Active. TS InputTracking.ts:48-60 @2e3bcf43.
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
// TS InputTracking.ts:62-73 @2e3bcf43.
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
// TS InputTracking.ts:75-86 @2e3bcf43.
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
// the len==0 and len>160 rejections. TS InputTracking.ts:88-100 @2e3bcf43.
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

// TestInputTrackingOnCycleFlushThreshold pins OnCycle's >=1500 soft-limit
// flush (rev-254 A5: softLimit=1500 replaces the 43e02957-era max=500):
// the raw buffer bytes are submitted to the logger bridge with the
// player — no blob wrapper, no seq, no coord (TS InputTracking.ts:30-34,
// 36-46 @2e3bcf43).
func TestInputTrackingOnCycleFlushThreshold(t *testing.T) {
	tt, p, rec := inputTrackingTestSetup(t)
	tt.Active = true
	p.username = "alice"
	p.session = "sess-uuid-1"

	// 300 mouse clicks x 5 bytes = exactly 1500 buffered bytes.
	for range 300 {
		tt.MouseClick(0x11223344)
	}
	if got := tt.buf.Len(); got != 1500 {
		t.Fatalf("preflight buf len: got %d, want 1500", got)
	}

	tt.OnCycle()

	if got := len(rec.inputTracks); got != 1 {
		t.Fatalf("bridge calls after OnCycle at 1500: got %d, want 1", got)
	}
	call := rec.inputTracks[0]
	if call.player != p {
		t.Errorf("player: got %p, want %p (TS submits this.player)", call.player, p)
	}
	if got := len(call.buf); got != 1500 {
		t.Errorf("submitted buf len: got %d, want 1500 (raw bytes, no base64 sender-side)", got)
	}
	if !bytes.Equal(call.buf[:5], []byte{0x03, 0x11, 0x22, 0x33, 0x44}) {
		t.Errorf("submitted buf head: got % X, want 03 11 22 33 44", call.buf[:5])
	}
	if got := tt.buf.Len(); got != 0 {
		t.Errorf("buf len after flush: got %d, want 0", got)
	}

	// OnCycle below threshold is a no-op — 1499 bytes must NOT flush.
	for range 299 {
		tt.MouseClick(0x11223344)
	}
	tt.buf.P1(0)
	tt.buf.P1(0)
	tt.buf.P1(0)
	tt.buf.P1(0)
	if got := tt.buf.Len(); got != 1499 {
		t.Fatalf("preflight buf len: got %d, want 1499", got)
	}
	tt.OnCycle()
	if got := len(rec.inputTracks); got != 1 {
		t.Fatalf("bridge calls after sub-threshold OnCycle: got %d, want 1", got)
	}

	// Second explicit flush submits the remaining bytes.
	tt.Flush()
	if got := len(rec.inputTracks); got != 2 {
		t.Fatalf("bridge calls after second flush: got %d, want 2", got)
	}
	if got := len(rec.inputTracks[1].buf); got != 1499 {
		t.Errorf("second flush buf len: got %d, want 1499", got)
	}
}

// TestInputTrackingAppendOverflowFlushesFirst pins the rev-254 A5
// pre-append overflow flush: the per-event checks now compare against
// the BUFFER CAPACITY (`this.buf.pos + N >= this.buf.length`, where
// buf.length = 5000 — Packet.alloc(1), Packet.ts:128 @2e3bcf43), with
// >= replacing the 43e02957-era > against max=500.
// TS InputTracking.ts:53-55 (and the parallel checks per event method).
func TestInputTrackingAppendOverflowFlushesFirst(t *testing.T) {
	t.Run("4995-plus-5-flushes", func(t *testing.T) {
		tt, _, rec := inputTrackingTestSetup(t)
		tt.Active = true
		fillInputBuffer(t, tt, 4995)
		tt.MouseClick(0xCAFEBABE) // 4995+5 >= 5000 → flush first, then append
		if got := len(rec.inputTracks); got != 1 {
			t.Fatalf("bridge calls: got %d, want 1 (overflow must flush first)", got)
		}
		if got := len(rec.inputTracks[0].buf); got != 4995 {
			t.Errorf("flushed buf carries pre-append bytes: len got %d, want 4995", got)
		}
		want := []byte{0x03, 0xCA, 0xFE, 0xBA, 0xBE}
		if got := tt.buf.Bytes(); !bytes.Equal(got, want) {
			t.Errorf("buf after overflow append: got % X, want % X", got, want)
		}
	})
	t.Run("4994-plus-5-appends", func(t *testing.T) {
		tt, _, rec := inputTrackingTestSetup(t)
		tt.Active = true
		fillInputBuffer(t, tt, 4994)
		tt.MouseClick(0xCAFEBABE) // 4994+5 == 4999 < 5000 → no flush
		if got := len(rec.inputTracks); got != 0 {
			t.Fatalf("bridge calls: got %d, want 0 (4994+5 < 5000 must append)", got)
		}
		if got := tt.buf.Len(); got != 4999 {
			t.Errorf("buf len: got %d, want 4999", got)
		}
	})
	t.Run("mousemove-overflow-counts-data-only", func(t *testing.T) {
		// TS quirk: mouseMove's overflow check adds only data.length —
		// NOT the 2 tag/len bytes. At bufLen=4997 with len(data)=2,
		// 4997+2 == 4999 is NOT >= 5000, so it appends WITHOUT flushing
		// and the buffer overshoots the capacity check to 5001.
		// TS InputTracking.ts:93 @2e3bcf43.
		tt, _, rec := inputTrackingTestSetup(t)
		tt.Active = true
		fillInputBuffer(t, tt, 4997)
		tt.MouseMove([]byte{0xEE, 0xFF})
		if got := len(rec.inputTracks); got != 0 {
			t.Fatalf("bridge calls: got %d, want 0 (quirk: tag/len bytes not counted)", got)
		}
		if got := tt.buf.Len(); got != 5001 {
			t.Errorf("buf len: got %d, want 5001 (4997 + tag + len + 2 data)", got)
		}
	})
}

// fillInputBuffer appends applet-focus events (2 bytes each) plus
// raw padding until the accumulation buffer holds exactly n bytes.
// Bypasses OnCycle, so n may exceed the 1500 soft limit (matching the
// TS model: events accumulate freely between cycles; only the per-event
// capacity checks and the per-cycle soft limit flush).
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
// buffer submits nothing. TS InputTracking.ts:41
// (`if (this.buf.pos > 0)` wraps the submit).
func TestInputTrackingFlushEmptyBuffer(t *testing.T) {
	tt, _, rec := inputTrackingTestSetup(t)
	tt.Active = true

	tt.Flush()
	if got := len(rec.inputTracks); got != 0 {
		t.Fatalf("bridge calls: got %d, want 0", got)
	}

	// Next real flush submits the buffered event.
	tt.MouseClick(1)
	tt.Flush()
	if got := len(rec.inputTracks); got != 1 {
		t.Fatalf("bridge calls: got %d, want 1", got)
	}
	if got := len(rec.inputTracks[0].buf); got != 5 {
		t.Errorf("buf len: got %d, want 5", got)
	}
}

// TestInputTrackingFlushInactiveNoop pins that Flush while !Active is a
// complete no-op even with buffered bytes — TS checks active FIRST
// (InputTracking.ts:37-39 @2e3bcf43), so the buffer is neither
// submitted nor reset.
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
}

// ---------------------------------------------------------------------------
// 254 event-packet handlers (handler_events.go)
// ---------------------------------------------------------------------------

// TestHandleEventPackets pins the four thin handlers' decode + dispatch
// against the TS codec files @2e3bcf43:
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
