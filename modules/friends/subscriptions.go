package friends

import (
	"log/slog"
	"sync"

	"github.com/zsrv/goscape/pkg/friendspb"
)

// subscriberBufferSize is the per-subscriber channel buffer. Tuned for
// modest broadcast bursts; oversize beyond plausible per-player update
// rate so steady-state never drops.
//
// NAI-S4A-D-DROP-ON-FULL — overflowing the buffer drops the newest
// update with a Warn log instead of blocking the RPC handler.
const subscriberBufferSize = 64

// subKey is the composite key for the subscriptions registry.
// Rev-244 multi-profile: the same username37 may be live under two
// profiles simultaneously (socketByWorld[profile][world] at
// FriendServer.ts:69-75), so profile must be part of the key.
type subKey struct {
	profile    string
	username37 uint64
}

// subscriber is a single open SubscribeUpdates stream for one
// (profile, worldId, username37) triple. ch is written by RPC
// handlers; the gRPC stream goroutine drains ch and calls stream.Send.
// done is closed by deregister to signal the gRPC goroutine to exit.
type subscriber struct {
	profile    string
	worldId    int32
	username37 uint64
	ch         chan *friendspb.FriendsUpdate
	done       chan struct{}
}

// newSubscriber allocates ch + done with the standard buffer size.
func newSubscriber(profile string, worldId int32, username37 uint64) *subscriber {
	return &subscriber{
		profile:    profile,
		worldId:    worldId,
		username37: username37,
		ch:         make(chan *friendspb.FriendsUpdate, subscriberBufferSize),
		done:       make(chan struct{}),
	}
}

// subscriptions is the per-(profile, player) subscriber registry.
// All methods are goroutine-safe.
type subscriptions struct {
	mu  sync.Mutex
	by  map[subKey]*subscriber
	log *slog.Logger
}

func newSubscriptions(log *slog.Logger) *subscriptions {
	return &subscriptions{
		by:  make(map[subKey]*subscriber),
		log: log,
	}
}

// register installs sub under its (profile, username37) key. If a prior
// subscriber exists for the same key, it is kicked (its done is closed)
// before sub replaces it. Generalizes TS FriendServer.initializeWorld
// terminate-then-replace (FriendServer.ts:412-419) from per-world to
// per-(profile, player).
func (s *subscriptions) register(sub *subscriber) {
	key := subKey{profile: sub.profile, username37: sub.username37}
	s.mu.Lock()
	defer s.mu.Unlock()
	if prior, ok := s.by[key]; ok {
		close(prior.done)
	}
	s.by[key] = sub
}

// deregister removes sub from the registry IFF it is still the
// currently registered subscriber for its (profile, username37) key
// (a rapid re-login may have replaced it under register).
func (s *subscriptions) deregister(sub *subscriber) {
	key := subKey{profile: sub.profile, username37: sub.username37}
	s.mu.Lock()
	defer s.mu.Unlock()
	if cur, ok := s.by[key]; ok && cur == sub {
		delete(s.by, key)
	}
}

// send pushes u to the subscriber for (profile, username37) (no-op if none).
// Non-blocking; on full channel, logs warn and drops the update.
func (s *subscriptions) send(profile string, username37 uint64, u *friendspb.FriendsUpdate) {
	key := subKey{profile: profile, username37: username37}
	s.mu.Lock()
	sub, ok := s.by[key]
	s.mu.Unlock()
	if !ok {
		return
	}
	select {
	case sub.ch <- u:
	default:
		s.log.Warn("friends subscriber buffer full; dropping update",
			slog.String("profile", profile),
			slog.Uint64("username37", username37))
	}
}
