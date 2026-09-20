package packetcapture

import (
	"testing"
	"time"

	"go.opentelemetry.io/otel/metric/noop"

	"github.com/zsrv/goscape/pkg/eventspb"
	"github.com/zsrv/goscape/pkg/tapper"
	"google.golang.org/protobuf/proto"
)

// decodeEnvelope pops one record off the ring and decodes it.
func decodeEnvelope(t *testing.T, c *Capture) *eventspb.ReplayEnvelope {
	t.Helper()
	recs := c.ring.PopBatch(16)
	if len(recs) == 0 {
		t.Fatal("ring empty")
	}
	var env eventspb.ReplayEnvelope
	if err := proto.Unmarshal(recs[0].Value, &env); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	return &env
}

// TestCaptureOptsProfileReachesBothEnvelopes pins that the profile a Capture
// is constructed with is stamped on both records encode.go builds — the
// packet envelope and the session-lifecycle envelope. "beta" rather than the
// "main" default so a hard-coded value cannot pass.
func TestCaptureOptsProfileReachesBothEnvelopes(t *testing.T) {
	m, err := NewMetrics(noop.NewMeterProvider().Meter("test"))
	if err != nil {
		t.Fatalf("NewMetrics: %v", err)
	}
	c := New(CaptureOpts{
		Enabled:      true,
		WorldID:      7,
		Revision:     225,
		Profile:      "beta",
		RingCapacity: 8,
		Metrics:      m,
	})

	const sess = "11111111-2222-3333-4444-555555555555"
	now := time.Now()

	c.SessionStarted(42, sess, now)
	if got := decodeEnvelope(t, c).Profile; got != "beta" {
		t.Errorf("session envelope Profile = %q, want %q", got, "beta")
	}

	c.Tap(42, sess, tapper.DirOut, 181, []byte{1, 2, 3}, now)
	if got := decodeEnvelope(t, c).Profile; got != "beta" {
		t.Errorf("packet envelope Profile = %q, want %q", got, "beta")
	}

	c.SessionEnded(42, sess, now, "bye")
	if got := decodeEnvelope(t, c).Profile; got != "beta" {
		t.Errorf("session-ended envelope Profile = %q, want %q", got, "beta")
	}
}

// TestZeroValueCaptureOptsProfileIsEmpty pins the zero value: a capture built
// without a profile stamps "", the "emitted by a build/deploy that named no
// profile" sentinel, rather than inventing a default.
func TestZeroValueCaptureOptsProfileIsEmpty(t *testing.T) {
	rec, err := EncodePacket(PacketRecord{
		WorldID:   1,
		Revision:  225,
		AccountID: 42,
		SessionID: "11111111-2222-3333-4444-555555555555",
		Direction: tapper.DirIn,
		Opcode:    1,
		TS:        time.Now(),
	})
	if err != nil {
		t.Fatalf("EncodePacket: %v", err)
	}
	var env eventspb.ReplayEnvelope
	if err := proto.Unmarshal(rec.Value, &env); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if env.Profile != "" {
		t.Errorf("Profile = %q, want \"\"", env.Profile)
	}
}
