package world

import (
	"bytes"
	"net"
	"testing"

	io2 "github.com/zsrv/goscape/pkg/io/isaac"
)

// inputTrackingTestSetup wires a Player against a Server with
// recordingBridges. Returns the tracking entity, player, the
// client-side test pipe, and the recorder.
func inputTrackingTestSetup(t *testing.T) (*InputTracking, *Player, net.Conn, *recordingBridges) {
	t.Helper()
	s := newTestServer(t)
	p, cc := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	rec := installRecordingBridges(s)
	tt := &InputTracking{player: p}
	return tt, p, cc, rec
}

// TestInputTrackingIsActiveMatrix pins the 4 corners of IsActive.
func TestInputTrackingIsActiveMatrix(t *testing.T) {
	cases := []struct {
		name        string
		currentTick int
		startAt     int
		endAt       int
		waiting     bool
		want        bool
	}{
		{"pre-window", 99, 100, 200, false, false},
		{"on-start", 100, 100, 200, false, true},
		{"mid-window", 150, 100, 200, false, true},
		{"on-end", 200, 100, 200, false, true},
		{"post-window", 201, 100, 200, false, false},
		{"post-window-but-waiting", 201, 100, 200, true, true},
		{"pre-window-but-waiting", 99, 100, 200, true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tt, _, _, _ := inputTrackingTestSetup(t)
			tt.startTrackingAt = tc.startAt
			tt.endTrackingAt = tc.endAt
			tt.waitingForRemainingData = tc.waiting
			got := tt.IsActive(tc.currentTick)
			if got != tc.want {
				t.Errorf("IsActive(%d) startAt=%d endAt=%d waiting=%v: got %v, want %v",
					tc.currentTick, tc.startAt, tc.endAt, tc.waiting, got, tc.want)
			}
		})
	}
}

// TestInputTrackingShouldSubmitTrackingDetailsMatrix pins the 2x2 OR
// of (player.submitInput, cfg.NodeSubmitInput).
func TestInputTrackingShouldSubmitTrackingDetailsMatrix(t *testing.T) {
	cases := []struct {
		name         string
		playerSubmit bool
		cfgSubmit    bool
		want         bool
	}{
		{"both-false", false, false, false},
		{"player-only", true, false, true},
		{"cfg-only", false, true, true},
		{"both-true", true, true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tt, p, _, _ := inputTrackingTestSetup(t)
			p.submitInput = tc.playerSubmit
			p.client.server.cfg.NodeSubmitInput = tc.cfgSubmit
			got := tt.ShouldSubmitTrackingDetails()
			if got != tc.want {
				t.Errorf("ShouldSubmitTrackingDetails: playerSubmit=%v cfgSubmit=%v: got %v, want %v",
					tc.playerSubmit, tc.cfgSubmit, got, tc.want)
			}
		})
	}
}

// TestInputTrackingRecord pins blob accumulation and size totalisation.
func TestInputTrackingRecord(t *testing.T) {
	tt, _, _, _ := inputTrackingTestSetup(t)
	tt.Record([]byte{1, 2, 3})
	tt.Record([]byte{4, 5})
	if got, want := len(tt.recordedBlobs), 2; got != want {
		t.Errorf("recordedBlobs len: got %d, want %d", got, want)
	}
	if got, want := tt.recordedBlobsSizeTotal, 5; got != want {
		t.Errorf("recordedBlobsSizeTotal: got %d, want %d", got, want)
	}
	if !bytes.Equal(tt.recordedBlobs[0], []byte{1, 2, 3}) {
		t.Errorf("recordedBlobs[0]: got %x, want 010203", tt.recordedBlobs[0])
	}
}

// TestInputTrackingEnable pins enable's state transitions and the
// EnableTracking server-packet write.
func TestInputTrackingEnable(t *testing.T) {
	tt, p, cc, _ := inputTrackingTestSetup(t)
	tt.enabled = false
	tt.startTrackingAt = 1000 // will be overwritten to currentTick

	received := drainConn(t, cc)
	tt.enable(500)
	p.client.flushWrite()
	got := <-received

	if !tt.enabled {
		t.Error("enabled: must be true after enable()")
	}
	if want := 500; tt.startTrackingAt != want {
		t.Errorf("startTrackingAt: got %d, want %d (currentTick at enable)", tt.startTrackingAt, want)
	}
	if want := 500 + inputTrackingTime; tt.endTrackingAt != want {
		t.Errorf("endTrackingAt: got %d, want %d", tt.endTrackingAt, want)
	}

	// Verify EnableTracking packet was written. OpEnableTracking has 0 payload
	// so the wire is a single ISAAC-encrypted byte.
	if len(got) != 1 {
		t.Fatalf("client out: got %d bytes, want 1 (EnableTracking opcode)", len(got))
	}
	// Decode the ISAAC-encrypted byte against a parallel encryptor seeded
	// the same way to verify the opcode.
	parallel := io2.New([4]uint32{1, 2, 3, 4})
	wantByte := byte((226 + parallel.GetNext()) & 0xff)
	if got[0] != wantByte {
		t.Errorf("EnableTracking opcode (encrypted): got %d, want %d", got[0], wantByte)
	}
}

// TestInputTrackingEnableIdempotent pins that calling enable() when
// already enabled is a no-op.
func TestInputTrackingEnableIdempotent(t *testing.T) {
	tt, p, cc, _ := inputTrackingTestSetup(t)
	tt.enabled = true
	tt.startTrackingAt = 100
	tt.endTrackingAt = 250

	received := drainConn(t, cc)
	tt.enable(500)
	p.client.flushWrite()
	got := <-received

	if want := 100; tt.startTrackingAt != want {
		t.Errorf("startTrackingAt should not change: got %d, want %d", tt.startTrackingAt, want)
	}
	if len(got) != 0 {
		t.Errorf("idempotent enable should not write: got %d bytes", len(got))
	}
}

// TestInputTrackingDisable pins disable's state transitions and the
// FinishTracking server-packet write.
func TestInputTrackingDisable(t *testing.T) {
	tt, p, cc, _ := inputTrackingTestSetup(t)
	tt.enabled = true
	tt.startTrackingAt = 100
	tt.endTrackingAt = 250

	received := drainConn(t, cc)
	tt.disable(300)
	p.client.flushWrite()
	got := <-received

	if tt.enabled {
		t.Error("enabled: must be false after disable()")
	}
	if !tt.waitingForRemainingData {
		t.Error("waitingForRemainingData: must be true after disable()")
	}
	if want := 300; tt.endTrackingAt != want {
		t.Errorf("endTrackingAt: got %d, want %d (currentTick at disable)", tt.endTrackingAt, want)
	}
	// startTrackingAt rescheduled to a future tick — exact value depends on
	// jitter, but it must be in [300+inputTrackingRate-15, 300+inputTrackingRate+15].
	wantMin := 300 + inputTrackingRate - inputTrackingJitterRange
	wantMax := 300 + inputTrackingRate + inputTrackingJitterRange
	if tt.startTrackingAt < wantMin || tt.startTrackingAt > wantMax {
		t.Errorf("startTrackingAt: got %d, want in [%d, %d]", tt.startTrackingAt, wantMin, wantMax)
	}

	// FinishTracking packet was written (OpFinishTracking = 133, 0-payload = 1 wire byte).
	if len(got) != 1 {
		t.Fatalf("client out: got %d bytes, want 1 (FinishTracking opcode)", len(got))
	}
}

// TestInputTrackingDisableIdempotent pins that disable() when already
// disabled is a no-op.
func TestInputTrackingDisableIdempotent(t *testing.T) {
	tt, p, cc, _ := inputTrackingTestSetup(t)
	tt.enabled = false

	received := drainConn(t, cc)
	tt.disable(300)
	p.client.flushWrite()
	got := <-received

	if tt.waitingForRemainingData {
		t.Error("waitingForRemainingData should not be set on no-op disable")
	}
	if len(got) != 0 {
		t.Errorf("idempotent disable should not write: got %d bytes", len(got))
	}
}

// TestInputTrackingSubmitEventsMatrix pins all 4 branches of
// submitEvents (TS InputTracking.submitEvents at lines 140-158).
func TestInputTrackingSubmitEventsMatrix(t *testing.T) {
	cases := []struct {
		name               string
		hasSeenReport      bool
		shouldSubmit       bool
		nodeDebug          bool
		blobsBefore        [][]byte
		wantBridgeCalls    int
		wantKick           bool
		wantSubmittedBlob  []byte
		wantSessionLogPush bool // NAI-74
	}{
		{
			name:              "report+submit→bridge",
			hasSeenReport:     true,
			shouldSubmit:      true,
			nodeDebug:         false,
			blobsBefore:       [][]byte{{0xAA}, {0xBB}, {0xCC}},
			wantBridgeCalls:   1,
			wantKick:          false,
			wantSubmittedBlob: []byte{0xAA},
			wantSessionLogPush: false,
		},
		{
			name:               "report+!submit→nothing",
			hasSeenReport:      true,
			shouldSubmit:       false,
			nodeDebug:          false,
			blobsBefore:        [][]byte{{0xAA}},
			wantBridgeCalls:    0,
			wantKick:           false,
			wantSessionLogPush: false,
		},
		{
			name:               "!report+!debug→kick",
			hasSeenReport:      false,
			shouldSubmit:       false,
			nodeDebug:          false,
			blobsBefore:        nil,
			wantBridgeCalls:    0,
			wantKick:           true,
			wantSessionLogPush: true, // NAI-74: TS InputTracking.ts:150
		},
		{
			name:               "!report+debug→nothing",
			hasSeenReport:      false,
			shouldSubmit:       false,
			nodeDebug:          true,
			blobsBefore:        nil,
			wantBridgeCalls:    0,
			wantKick:           false,
			wantSessionLogPush: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tt, p, _, rec := inputTrackingTestSetup(t)
			tt.hasSeenReport = tc.hasSeenReport
			tt.recordedBlobs = tc.blobsBefore
			tt.recordedBlobsSizeTotal = 0
			for _, b := range tc.blobsBefore {
				tt.recordedBlobsSizeTotal += len(b)
			}
			tt.waitingForRemainingData = true
			p.submitInput = tc.shouldSubmit
			p.client.server.cfg.NodeSubmitInput = false
			p.client.server.cfg.NodeDebug = tc.nodeDebug
			p.requestIdleLogout = false

			tt.submitEvents()

			if got := len(rec.inputTracks); got != tc.wantBridgeCalls {
				t.Errorf("bridge calls: got %d, want %d", got, tc.wantBridgeCalls)
			}
			if tc.wantBridgeCalls > 0 && tc.wantSubmittedBlob != nil {
				if !bytes.Equal(rec.inputTracks[0].blob, tc.wantSubmittedBlob) {
					t.Errorf("submitted blob: got %x, want %x", rec.inputTracks[0].blob, tc.wantSubmittedBlob)
				}
			}
			if got := p.requestIdleLogout; got != tc.wantKick {
				t.Errorf("requestIdleLogout: got %v, want %v", got, tc.wantKick)
			}

			// NAI-74: NAI-73-D close — kick branch must push one ENGINE
			// session-log "Client did not submit an input tracking report".
			if tc.wantSessionLogPush {
				if got := len(p.client.server.sessionLogs); got != 1 {
					t.Errorf("sessionLogs: got %d, want 1", got)
				} else {
					lg := p.client.server.sessionLogs[0]
					if lg.EventType != LoggerEventTypeEngine {
						t.Errorf("EventType: got %d, want ENGINE(%d)", lg.EventType, LoggerEventTypeEngine)
					}
					if lg.Event != "Client did not submit an input tracking report" {
						t.Errorf("Event: got %q, want %q", lg.Event,
							"Client did not submit an input tracking report")
					}
				}
			} else {
				if got := len(p.client.server.sessionLogs); got != 0 {
					t.Errorf("sessionLogs: got %d, want 0", got)
				}
			}

			// Reset invariants — every branch must clear state.
			if tt.waitingForRemainingData {
				t.Error("waitingForRemainingData: must be false after submitEvents")
			}
			if tt.recordedBlobs != nil {
				t.Errorf("recordedBlobs: must be nil after submitEvents, got %v", tt.recordedBlobs)
			}
			if tt.recordedBlobsSizeTotal != 0 {
				t.Errorf("recordedBlobsSizeTotal: must be 0 after submitEvents, got %d", tt.recordedBlobsSizeTotal)
			}
			if tt.hasSeenReport {
				t.Error("hasSeenReport: must be false after submitEvents")
			}
		})
	}
}

// TestInputTrackingOnCycleDispatch pins OnCycle's branch dispatch.
// Each case pins one of: enable, disable, grace-expired-submit, or no-op.
func TestInputTrackingOnCycleDispatch(t *testing.T) {
	cases := []struct {
		name             string
		startAt          int
		endAt            int
		enabled          bool
		waiting          bool
		currentTick      int
		wantEnabled      bool
		wantWaiting      bool
		wantClearedBlobs bool
	}{
		{"pre-window-noop", 100, 250, false, false, 50, false, false, false},
		{"on-start-enable", 100, 250, false, false, 100, true, false, false},
		{"mid-window-noop", 100, 250, true, false, 175, true, false, false},
		{"on-end-disable", 100, 250, true, false, 250, false, true, false},
		{"waiting-grace-not-expired", 100, 250, false, true, 250 + inputTrackingRemainingDataUploadLeeway, false, true, false},
		{"waiting-grace-expired-submit", 100, 250, false, true, 250 + inputTrackingRemainingDataUploadLeeway + 1, false, false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tt, p, _, _ := inputTrackingTestSetup(t)
			tt.startTrackingAt = tc.startAt
			tt.endTrackingAt = tc.endAt
			tt.enabled = tc.enabled
			tt.waitingForRemainingData = tc.waiting
			tt.recordedBlobs = [][]byte{{0xAA}} // populate so we can detect submitEvents reset
			tt.recordedBlobsSizeTotal = 1
			p.client.server.cfg.NodeDebug = true // suppress kick

			tt.OnCycle(tc.currentTick)

			if got := tt.enabled; got != tc.wantEnabled {
				t.Errorf("enabled: got %v, want %v", got, tc.wantEnabled)
			}
			if got := tt.waitingForRemainingData; got != tc.wantWaiting {
				t.Errorf("waitingForRemainingData: got %v, want %v", got, tc.wantWaiting)
			}
			cleared := tt.recordedBlobs == nil
			if cleared != tc.wantClearedBlobs {
				t.Errorf("recordedBlobs cleared: got %v, want %v", cleared, tc.wantClearedBlobs)
			}
		})
	}
}
