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

// wsubKey is the composite key for the worldSubscriptions registry.
// Rev-244 multi-profile: socketByWorld[profile][world] at
// FriendServer.ts:69-75 scopes world subscriptions per profile.
type wsubKey struct {
	profile string
	worldId int32
}

// worldSubscriber owns one open SubscribeWorldEvents stream for one
// (profile, worldId) pair. ch is written by RELAY_* handler methods;
// the gRPC stream goroutine drains ch and calls stream.Send. done is
// closed by a duplicate register to signal the prior stream goroutine
// to exit.
type worldSubscriber struct {
	profile string
	worldId int32
	ch      chan *friendspb.WorldEvent
	done    chan struct{}
}

func newWorldSubscriber(profile string, worldId int32) *worldSubscriber {
	return &worldSubscriber{
		profile: profile,
		worldId: worldId,
		ch:      make(chan *friendspb.WorldEvent, worldSubscriberBufferSize),
		done:    make(chan struct{}),
	}
}

// worldSubscriptions is the per-(profile, world) subscriber registry.
// All methods are goroutine-safe. Exactly one subscriber per (profile,
// worldId); re-subscribe kicks the prior (matches TS
// FriendServer.initializeWorld at FriendServer.ts:412-419 @2e3bcf43 —
// single-profile `socketByWorld[world].terminate()` on re-WORLD_CONNECT;
// goscape keeps the (profile, world) key for its multi-profile registry
// while the 254 server itself is single-profile).
type worldSubscriptions struct {
	mu  sync.Mutex
	by  map[wsubKey]*worldSubscriber
	log *slog.Logger
}

func newWorldSubscriptions(log *slog.Logger) *worldSubscriptions {
	return &worldSubscriptions{
		by:  make(map[wsubKey]*worldSubscriber),
		log: log,
	}
}

// register installs sub under its (profile, worldId) key. If a prior
// subscriber exists for the same key, it is kicked (its done is closed)
// before sub replaces it.
func (s *worldSubscriptions) register(sub *worldSubscriber) {
	key := wsubKey{profile: sub.profile, worldId: sub.worldId}
	s.mu.Lock()
	defer s.mu.Unlock()
	if prior, ok := s.by[key]; ok {
		close(prior.done)
	}
	s.by[key] = sub
}

// deregister removes sub from the registry IFF it is still the currently
// registered subscriber for its (profile, worldId) key (a rapid
// re-subscribe may have replaced it under register).
func (s *worldSubscriptions) deregister(sub *worldSubscriber) {
	key := wsubKey{profile: sub.profile, worldId: sub.worldId}
	s.mu.Lock()
	defer s.mu.Unlock()
	if cur, ok := s.by[key]; ok && cur == sub {
		delete(s.by, key)
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

// send pushes ev to the subscriber for (profile, worldId) (no-op if none).
// Non-blocking; on full channel, logs warn and drops the event.
func (s *worldSubscriptions) send(profile string, worldId int32, ev *friendspb.WorldEvent) {
	key := wsubKey{profile: profile, worldId: worldId}
	s.mu.Lock()
	sub, ok := s.by[key]
	s.mu.Unlock()
	if !ok {
		return
	}
	select {
	case sub.ch <- ev:
	default:
		s.log.Warn("world events subscriber buffer full; dropping event",
			slog.String("profile", profile),
			slog.Int("world_id", int(worldId)))
	}
}
