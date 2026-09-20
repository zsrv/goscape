package packetcapture

import (
	"context"
	"sync"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/zsrv/goscape/pkg/tapper"
)

// Capture is the per-process packet-capture facade. It owns the ring buffer
// and exposes the Tap / SessionStarted / SessionEnded API consumed by
// modules/world.
//
// When enabled is false, all public methods short-circuit after a single
// atomic load. This is the byte-identical-to-today path: no allocation, no
// ring enqueue, no Kafka producer ever constructed (the dskit module skips
// constructing the client too).
type Capture struct {
	enabled  bool
	ring     *RingBuffer
	metrics  *Metrics
	worldID  int32
	revision uint16
	profile  string

	mu         sync.Mutex
	sessions   map[string]*sessionStats // keyed by session_id
	closedLast map[string]sessionStats  // last-known counters, kept for tests
}

type sessionStats struct {
	AccountID      int64
	StartedAt      time.Time
	PacketCountIn  uint64
	PacketCountOut uint64
	ByteCountIn    uint64
	ByteCountOut   uint64
}

// CaptureOpts holds the inputs to New; immutable post-construction.
type CaptureOpts struct {
	Enabled      bool
	WorldID      int32
	Revision     uint16
	Profile      string
	RingCapacity int
	Metrics      *Metrics
}

// New constructs a Capture. When Opts.Enabled is false, the returned Capture
// is the short-circuit form (no ring allocated).
func New(opts CaptureOpts) *Capture {
	if !opts.Enabled {
		return &Capture{enabled: false}
	}
	return &Capture{
		enabled:    true,
		ring:       NewRingBuffer(opts.RingCapacity),
		metrics:    opts.Metrics,
		worldID:    opts.WorldID,
		revision:   opts.Revision,
		profile:    opts.Profile,
		sessions:   make(map[string]*sessionStats),
		closedLast: make(map[string]sessionStats),
	}
}

// Ring returns the underlying ring buffer (consumed by the shipper).
func (c *Capture) Ring() *RingBuffer { return c.ring }

// SessionStarted records the start of a session. Safe when disabled.
func (c *Capture) SessionStarted(accountID int64, sessionID string, ts time.Time) {
	if !c.enabled {
		return
	}
	c.mu.Lock()
	if c.sessions == nil {
		c.sessions = make(map[string]*sessionStats)
	}
	c.sessions[sessionID] = &sessionStats{AccountID: accountID, StartedAt: ts}
	c.mu.Unlock()

	rec, err := EncodeSessionStarted(SessionRecord{
		WorldID:   c.worldID,
		Revision:  c.revision,
		Profile:   c.profile,
		AccountID: accountID,
		SessionID: sessionID,
		StartedAt: ts,
		TS:        ts,
	})
	if err != nil {
		return
	}
	c.pushSessionMarker(rec)
	c.metrics.SessionStarted.Add(context.Background(), 1)
}

// pushSessionMarker enqueues a SESSION STARTED / ENDED record and accounts for
// an eviction exactly as the packet path does, under a drop reason of its own:
// a marker is how a consumer knows where a session begins and ends, so losing
// one leaves a session that never opens or never closes and is worth telling
// apart from a lost packet.
func (c *Capture) pushSessionMarker(rec *kgo.Record) {
	if c.ring.Push(rec) {
		c.metrics.PacketDropped.Add(context.Background(), 1,
			metric.WithAttributes(attribute.String(AttrDropReason, DropReasonSessionMarkerRingbufFull)))
	}
}

// Tap records a single packet. Safe when disabled.
func (c *Capture) Tap(accountID int64, sessionID string, dir tapper.Direction, opcode uint8, payload []byte, ts time.Time) {
	if !c.enabled {
		return
	}
	c.mu.Lock()
	if s, ok := c.sessions[sessionID]; ok {
		if dir == tapper.DirIn {
			s.PacketCountIn++
			s.ByteCountIn += uint64(len(payload))
		} else {
			s.PacketCountOut++
			s.ByteCountOut += uint64(len(payload))
		}
	}
	c.mu.Unlock()

	rec, err := EncodePacket(PacketRecord{
		WorldID:   c.worldID,
		Revision:  c.revision,
		Profile:   c.profile,
		AccountID: accountID,
		SessionID: sessionID,
		Direction: dir,
		Opcode:    opcode,
		Payload:   payload,
		TS:        ts,
	})
	if err != nil {
		return
	}
	dropped := c.ring.Push(rec)
	dirAttr := attribute.String(AttrDirection, dir.String())
	c.metrics.PacketRecorded.Add(context.Background(), 1, metric.WithAttributes(dirAttr))
	c.metrics.PacketIO.Add(context.Background(), int64(len(payload)), metric.WithAttributes(dirAttr))
	c.metrics.PacketSize.Record(context.Background(), int64(len(payload)), metric.WithAttributes(dirAttr))
	if dropped {
		c.metrics.PacketDropped.Add(context.Background(), 1,
			metric.WithAttributes(attribute.String(AttrDropReason, DropReasonRingbufFull)))
	}
}

// SessionEnded records a session close, flushes a final SessionEvent with the
// accumulated counters, and forgets per-session state. Safe when disabled.
func (c *Capture) SessionEnded(accountID int64, sessionID string, ts time.Time, closeReason string) {
	if !c.enabled {
		return
	}
	c.mu.Lock()
	s, ok := c.sessions[sessionID]
	if !ok {
		c.mu.Unlock()
		return
	}
	stats := *s
	delete(c.sessions, sessionID)
	if c.closedLast == nil {
		c.closedLast = make(map[string]sessionStats)
	}
	c.closedLast[sessionID] = stats
	c.mu.Unlock()

	rec, err := EncodeSessionEnded(SessionRecord{
		WorldID:        c.worldID,
		Revision:       c.revision,
		Profile:        c.profile,
		AccountID:      accountID,
		SessionID:      sessionID,
		StartedAt:      stats.StartedAt,
		EndedAt:        ts,
		CloseReason:    closeReason,
		PacketCountIn:  stats.PacketCountIn,
		PacketCountOut: stats.PacketCountOut,
		ByteCountIn:    stats.ByteCountIn,
		ByteCountOut:   stats.ByteCountOut,
		TS:             ts,
	})
	if err != nil {
		return
	}
	c.pushSessionMarker(rec)
	c.metrics.SessionEnded.Add(context.Background(), 1,
		metric.WithAttributes(attribute.String(AttrSessionCloseReason, closeReason)))
}

// DroppedPackets reports the cumulative ring-buffer drop count for
// observability dashboards (exported in addition to the OTel counter).
func (c *Capture) DroppedPackets() uint64 {
	if !c.enabled {
		return 0
	}
	return c.ring.DroppedTotal()
}

// lastSessionStats is test-only: returns the counters of the most recently
// closed session with the given ID.
func (c *Capture) lastSessionStats(sessionID string) (sessionStats, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	s, ok := c.closedLast[sessionID]
	return s, ok
}

// Enabled reports whether the capture path is active. Used by the dskit
// wrapper to decide whether to start the shipper.
func (c *Capture) Enabled() bool { return c.enabled }
