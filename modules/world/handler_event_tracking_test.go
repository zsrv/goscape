package world

import (
	"testing"

	io2 "github.com/zsrv/goscape/pkg/io/isaac"
)

func eventTrackingTestSetup(t *testing.T) (*Player, *recordingBridges) {
	t.Helper()
	s := newTestServer(t)
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	rec := installRecordingBridges(s)
	p.input = &InputTracking{player: p}
	return p, rec
}

// TestHandleEventTrackingLenZeroReturnsFalse: empty payloads are dropped
// without state mutation. TS EventTrackingHandler.ts:9-11.
func TestHandleEventTrackingLenZeroReturnsFalse(t *testing.T) {
	p, _ := eventTrackingTestSetup(t)
	p.input.startTrackingAt = 0
	p.input.endTrackingAt = 1000
	if err := handleEventTracking(p, []byte{}); err != nil {
		t.Fatalf("handleEventTracking: %v", err)
	}
	if p.input.hasSeenReport {
		t.Error("hasSeenReport: must remain false on empty payload")
	}
	if len(p.input.recordedBlobs) != 0 {
		t.Errorf("recordedBlobs: got %d, want 0", len(p.input.recordedBlobs))
	}
}

// TestHandleEventTrackingLenOver500ReturnsFalse: payloads >500 bytes
// are dropped. TS EventTrackingHandler.ts:9-11.
func TestHandleEventTrackingLenOver500ReturnsFalse(t *testing.T) {
	p, _ := eventTrackingTestSetup(t)
	p.input.startTrackingAt = 0
	p.input.endTrackingAt = 1000
	payload := make([]byte, 501)
	if err := handleEventTracking(p, payload); err != nil {
		t.Fatalf("handleEventTracking: %v", err)
	}
	if p.input.hasSeenReport {
		t.Error("hasSeenReport: must remain false on oversize payload")
	}
}

// TestHandleEventTrackingNotActiveReturnsFalse: blobs received outside
// the active window are dropped. TS EventTrackingHandler.ts:12-14.
func TestHandleEventTrackingNotActiveReturnsFalse(t *testing.T) {
	p, _ := eventTrackingTestSetup(t)
	// Window starts in the future: not active at processIn currentTick.
	p.input.startTrackingAt = 1000
	p.input.endTrackingAt = 2000
	// IsActive() reads s.currentTick — which here is 0 (newTestServer default).
	if err := handleEventTracking(p, []byte{0xAA}); err != nil {
		t.Fatalf("handleEventTracking: %v", err)
	}
	if p.input.hasSeenReport {
		t.Error("hasSeenReport: must remain false when !IsActive")
	}
}

// TestHandleEventTrackingActiveButNotShouldSubmitShortCircuit:
// active but submitInput+cfg.NodeSubmitInput both false → returns true
// after setting hasSeenReport=true, but DOES NOT call Record. TS
// EventTrackingHandler.ts:18-20.
func TestHandleEventTrackingActiveButNotShouldSubmitShortCircuit(t *testing.T) {
	p, _ := eventTrackingTestSetup(t)
	p.input.startTrackingAt = 0
	p.input.endTrackingAt = 1000
	p.submitInput = false
	p.client.server.cfg.NodeSubmitInput = false

	if err := handleEventTracking(p, []byte{0xAA}); err != nil {
		t.Fatalf("handleEventTracking: %v", err)
	}
	if !p.input.hasSeenReport {
		t.Error("hasSeenReport: must be true after first valid blob")
	}
	if len(p.input.recordedBlobs) != 0 {
		t.Errorf("recordedBlobs: must remain empty (short-circuit), got %d", len(p.input.recordedBlobs))
	}
}

// TestHandleEventTrackingCapExceededReturnsFalse: when
// recordedBlobsSizeTotal > cfg.NodeLimitBytesPerTrackingSession, the
// handler returns false without recording. TS
// EventTrackingHandler.ts:21-25.
func TestHandleEventTrackingCapExceededReturnsFalse(t *testing.T) {
	p, _ := eventTrackingTestSetup(t)
	p.input.startTrackingAt = 0
	p.input.endTrackingAt = 1000
	p.submitInput = true
	p.client.server.cfg.NodeLimitBytesPerTrackingSession = 100
	p.input.recordedBlobsSizeTotal = 101 // already over

	if err := handleEventTracking(p, []byte{0xAA}); err != nil {
		t.Fatalf("handleEventTracking: %v", err)
	}
	// hasSeenReport set first (TS line 16), THEN cap check (line 21);
	// both true cases here — hasSeenReport=true but Record skipped.
	if !p.input.hasSeenReport {
		t.Error("hasSeenReport: must be true (set before cap check)")
	}
	if len(p.input.recordedBlobs) != 0 {
		t.Errorf("recordedBlobs: must NOT grow on cap-exceeded, got %d", len(p.input.recordedBlobs))
	}
}

// TestHandleEventTrackingHappyPathRecords: active + shouldSubmit +
// under cap → Record called, hasSeenReport=true, returns true.
func TestHandleEventTrackingHappyPathRecords(t *testing.T) {
	p, _ := eventTrackingTestSetup(t)
	p.input.startTrackingAt = 0
	p.input.endTrackingAt = 1000
	p.submitInput = true
	p.client.server.cfg.NodeLimitBytesPerTrackingSession = 50000

	if err := handleEventTracking(p, []byte{0xAA, 0xBB, 0xCC}); err != nil {
		t.Fatalf("handleEventTracking: %v", err)
	}
	if !p.input.hasSeenReport {
		t.Error("hasSeenReport: must be true on happy path")
	}
	if got := len(p.input.recordedBlobs); got != 1 {
		t.Errorf("recordedBlobs: got %d, want 1", got)
	}
	if got := p.input.recordedBlobsSizeTotal; got != 3 {
		t.Errorf("recordedBlobsSizeTotal: got %d, want 3", got)
	}
}
