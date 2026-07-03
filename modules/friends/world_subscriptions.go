package friends

import (
	"log/slog"
	"sync"

	"github.com/zsrv/goscape/pkg/friendspb"
)

// worldSubscriberBufferSize is the per-world-subscriber channel buffer.
// Same posture as subscriberBufferSize from subscriptions.go but a
// separate constant in case admin-burst rate differs from per-player
// update rate.
//
// NAI-S5A-D-WORLDEVENTS-DROP-ON-FULL — overflowing the buffer drops the
// newest event with a Warn log instead of blocking the RPC handler.
const worldSubscriberBufferSize = 64

// worldSubscriber owns one open SubscribeWorldEvents stream for one
// world ID. ch is written by RELAY_* handler methods; the gRPC stream
// goroutine drains ch and calls stream.Send. done is closed by a
// duplicate register to signal the prior stream goroutine to exit.
type worldSubscriber struct {
	worldId int32
	ch      chan *friendspb.WorldEvent
	done    chan struct{}
}

func newWorldSubscriber(worldId int32) *worldSubscriber {
	return &worldSubscriber{
		worldId: worldId,
		ch:      make(chan *friendspb.WorldEvent, worldSubscriberBufferSize),
		done:    make(chan struct{}),
	}
}

// worldSubscriptions is the per-world subscriber registry. All methods
// are goroutine-safe. Exactly one subscriber per worldId; re-subscribe
// kicks the prior (matches TS FriendServer.initializeWorld at
// FriendServer.ts:412-419 — `socket.terminate()` on re-WORLD_CONNECT).
type worldSubscriptions struct {
	mu  sync.Mutex
	by  map[int32]*worldSubscriber // worldId -> subscriber
	log *slog.Logger
}

func newWorldSubscriptions(log *slog.Logger) *worldSubscriptions {
	return &worldSubscriptions{
		by:  make(map[int32]*worldSubscriber),
		log: log,
	}
}

// register installs sub under sub.worldId. If a prior subscriber exists
// for the same worldId, it is kicked (its done is closed) before sub
// replaces it.
func (s *worldSubscriptions) register(sub *worldSubscriber) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if prior, ok := s.by[sub.worldId]; ok {
		close(prior.done)
	}
	s.by[sub.worldId] = sub
}

// deregister removes sub from the registry IFF it is still the currently
// registered subscriber for sub.worldId (a rapid re-subscribe may have
// replaced it under register).
func (s *worldSubscriptions) deregister(sub *worldSubscriber) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if cur, ok := s.by[sub.worldId]; ok && cur == sub {
		delete(s.by, sub.worldId)
	}
}

// closeAll releases every live subscriber's SubscribeWorldEvents loop
// (closes each done channel) and empties the registry. Mirrors
// (*subscriptions).closeAll — see subscriptions.go for the full
// arch-29.4 rationale (called at service stop; deleting under the lock
// keeps register's close-prior path from double-closing a later
// registration for the same key).
func (s *worldSubscriptions) closeAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, sub := range s.by {
		close(sub.done)
		delete(s.by, k)
	}
}

// send pushes ev to the subscriber for worldId (no-op if none).
// Non-blocking; on full channel, logs warn and drops the event.
func (s *worldSubscriptions) send(worldId int32, ev *friendspb.WorldEvent) {
	s.mu.Lock()
	sub, ok := s.by[worldId]
	s.mu.Unlock()
	if !ok {
		return
	}
	select {
	case sub.ch <- ev:
	default:
		s.log.Warn("world events subscriber buffer full; dropping event",
			slog.Int("world_id", int(worldId)))
	}
}
