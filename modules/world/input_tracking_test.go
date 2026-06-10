package world

import (
	"encoding/base64"
	"net"
	"testing"

	"github.com/zsrv/goscape/pkg/coordgrid"
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

// TestInputTrackingRecord pins basic blob accumulation and size totalisation.
// Detailed Record() shape (seq/coord/base64) is covered by
// TestInputTrackingRecordWrapsBlobs.
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
	wantData := base64.StdEncoding.EncodeToString([]byte{1, 2, 3})
	if tt.recordedBlobs[0].Data != wantData {
		t.Errorf("recordedBlobs[0].Data: got %q, want %q", tt.recordedBlobs[0].Data, wantData)
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
	wantByte := byte((28 + parallel.GetNext()) & 0xff) // OpEnableTracking=28 at 245.2; TS ServerGameProt.ts @3c16994c: ENABLE_TRACKING=28
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

	// FinishTracking packet was written (OpFinishTracking = 165, 0-payload = 1 wire byte).
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
	// blobsFromRaw is a helper to build []InputTrackingBlob from raw slices,
	// mirroring what Record() would produce (seq from 1, coord=0 for simplicity).
	blobsFromRaw := func(raws [][]byte) []InputTrackingBlob {
		if raws == nil {
			return nil
		}
		out := make([]InputTrackingBlob, len(raws))
		for i, r := range raws {
			out[i] = NewInputTrackingBlob(r, i+1, 0)
		}
		return out
	}
	cases := []struct {
		name               string
		hasSeenReport      bool
		shouldSubmit       bool
		nodeDebug          bool
		blobsRaw           [][]byte // raw bytes → converted to []InputTrackingBlob via helper
		wantBridgeCalls    int
		wantKick           bool
		wantBlobCount      int  // 244: ALL blobs sent (not just blob[0])
		wantSessionLogPush bool // NAI-74
	}{
		{
			name:               "report+submit→bridge",
			hasSeenReport:      true,
			shouldSubmit:       true,
			nodeDebug:          false,
			blobsRaw:           [][]byte{{0xAA}, {0xBB}, {0xCC}},
			wantBridgeCalls:    1,
			wantKick:           false,
			wantBlobCount:      3, // 244: all 3 blobs sent
			wantSessionLogPush: false,
		},
		{
			name:               "report+!submit→nothing",
			hasSeenReport:      true,
			shouldSubmit:       false,
			nodeDebug:          false,
			blobsRaw:           [][]byte{{0xAA}},
			wantBridgeCalls:    0,
			wantKick:           false,
			wantSessionLogPush: false,
		},
		{
			name:               "!report+!debug→kick",
			hasSeenReport:      false,
			shouldSubmit:       false,
			nodeDebug:          false,
			blobsRaw:           nil,
			wantBridgeCalls:    0,
			wantKick:           true,
			wantSessionLogPush: true, // NAI-74: TS InputTracking.ts:150
		},
		{
			name:               "!report+debug→nothing",
			hasSeenReport:      false,
			shouldSubmit:       false,
			nodeDebug:          true,
			blobsRaw:           nil,
			wantBridgeCalls:    0,
			wantKick:           false,
			wantSessionLogPush: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tt, p, _, rec := inputTrackingTestSetup(t)
			tt.hasSeenReport = tc.hasSeenReport
			tt.recordedBlobs = blobsFromRaw(tc.blobsRaw)
			tt.recordedBlobsSizeTotal = 0
			for _, b := range tc.blobsRaw {
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
			if tc.wantBridgeCalls > 0 {
				if got := len(rec.inputTracks[0].blobs); got != tc.wantBlobCount {
					t.Errorf("submitted blob count: got %d, want %d", got, tc.wantBlobCount)
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

// ---------------------------------------------------------------------------
// rev-244 B3: InputTrackingBlob + submit re-shape (Task 14)
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

// TestInputTrackingRecordWrapsBlobs pins that Record() wraps each raw
// payload into an InputTrackingBlob with:
//   - seq starting at 1 and incrementing
//   - coord = player's packed coord at record time
//   - Data = base64 of the raw payload
//   - sizeTotal accumulates RAW lengths (before push, per TS line 134)
//
// Moving the player between Record calls proves coord is captured per blob.
// Mirrors InputTracking.ts:133-135.
func TestInputTrackingRecordWrapsBlobs(t *testing.T) {
	tt, p, _, _ := inputTrackingTestSetup(t)

	// First record: player at level=0, x=100, z=200.
	p.level, p.x, p.z = 0, 100, 200
	raw1 := []byte{0x01, 0x02, 0x03}
	tt.Record(raw1)

	// Move player before second record.
	p.level, p.x, p.z = 1, 300, 400
	raw2 := []byte{0x04, 0x05}
	tt.Record(raw2)

	// sizeTotal accumulates raw lengths.
	if got, want := tt.recordedBlobsSizeTotal, 5; got != want {
		t.Errorf("recordedBlobsSizeTotal: got %d, want %d", got, want)
	}
	if got := len(tt.recordedBlobs); got != 2 {
		t.Fatalf("recordedBlobs len: got %d, want 2", got)
	}

	// First blob.
	b0 := tt.recordedBlobs[0]
	if b0.Seq != 1 {
		t.Errorf("blob[0].Seq: got %d, want 1", b0.Seq)
	}
	wantData0 := base64.StdEncoding.EncodeToString(raw1)
	if b0.Data != wantData0 {
		t.Errorf("blob[0].Data: got %q, want %q", b0.Data, wantData0)
	}
	wantCoord0 := coordgrid.PackCoord(0, 100, 200)
	if b0.Coord != wantCoord0 {
		t.Errorf("blob[0].Coord: got %d, want %d (level=0 x=100 z=200)", b0.Coord, wantCoord0)
	}

	// Second blob — different coord.
	b1 := tt.recordedBlobs[1]
	if b1.Seq != 2 {
		t.Errorf("blob[1].Seq: got %d, want 2", b1.Seq)
	}
	wantData1 := base64.StdEncoding.EncodeToString(raw2)
	if b1.Data != wantData1 {
		t.Errorf("blob[1].Data: got %q, want %q", b1.Data, wantData1)
	}
	wantCoord1 := coordgrid.PackCoord(1, 300, 400)
	if b1.Coord != wantCoord1 {
		t.Errorf("blob[1].Coord: got %d, want %d (level=1 x=300 z=400)", b1.Coord, wantCoord1)
	}
}

// TestInputTrackingSubmitPassesAllBlobs pins the 244 submit re-shape:
// the bridge receives username + session UUID + ALL recorded blobs (not
// just blob[0] as in 225). Mirrors InputTracking.ts:147 +
// World.ts:2343-2350.
func TestInputTrackingSubmitPassesAllBlobs(t *testing.T) {
	tt, p, _, rec := inputTrackingTestSetup(t)
	p.username = "alice"
	p.session = "test-session-uuid-1234"
	p.level, p.x, p.z = 0, 100, 200

	p.submitInput = true
	tt.hasSeenReport = true
	tt.waitingForRemainingData = true

	// Record 3 blobs.
	tt.Record([]byte{0xAA})
	tt.Record([]byte{0xBB, 0xCC})
	tt.Record([]byte{0xDD})

	tt.submitEvents()

	if got := len(rec.inputTracks); got != 1 {
		t.Fatalf("bridge calls: got %d, want 1", got)
	}
	call := rec.inputTracks[0]
	if call.username != "alice" {
		t.Errorf("username: got %q, want alice", call.username)
	}
	if call.sessionUUID != "test-session-uuid-1234" {
		t.Errorf("sessionUUID: got %q, want test-session-uuid-1234", call.sessionUUID)
	}
	if got := len(call.blobs); got != 3 {
		t.Errorf("blobs count: got %d, want 3", got)
	}
}

// TestInputTrackingSubmitSessionUUIDHeadless pins that a nil-client
// (headless) player sends "headless" as the session UUID. Mirrors TS
// InputTracking.ts:147 `player instanceof NetworkPlayer ? ... : 'headless'`.
func TestInputTrackingSubmitSessionUUIDHeadless(t *testing.T) {
	// "Headless" in goscape: p.session == "" (tick.go:394 sets "headless"
	// on login if the session UUID from the login service is empty).
	// submitEvents resolves the session string via:
	//   sessionUUID := p.session; if sessionUUID == "" { sessionUUID = "headless" }
	// which mirrors TS InputTracking.ts:147:
	//   player instanceof NetworkPlayer ? player.client.uuid : 'headless'
	s := newTestServer(t)
	p2, _ := newTestPlayer(t)
	p2.username = "headlessbot"
	p2.session = "" // headless sentinel — empty session → submitEvents emits "headless"
	p2.client.server = s
	rec := installRecordingBridges(s)

	tt := &InputTracking{player: p2}
	tt.hasSeenReport = true
	tt.waitingForRemainingData = true
	p2.submitInput = true
	tt.Record([]byte{0xFF})

	tt.submitEvents()

	if got := len(rec.inputTracks); got != 1 {
		t.Fatalf("bridge calls: got %d, want 1", got)
	}
	// p2.session="" → session string resolves to "headless"
	if call := rec.inputTracks[0]; call.sessionUUID != "headless" {
		t.Errorf("sessionUUID for empty session: got %q, want headless", call.sessionUUID)
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
			tt.recordedBlobs = []InputTrackingBlob{{Seq: 1, Data: "qg==", Coord: 0}} // populate so we can detect submitEvents reset
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
