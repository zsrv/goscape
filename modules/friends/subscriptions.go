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

// subscriber is a single open SubscribeUpdates stream for one
// (worldId, username37) pair. ch is written by RPC handlers; the
// gRPC stream goroutine drains ch and calls stream.Send. done is
// closed by deregister to signal the gRPC goroutine to exit.
type subscriber struct {
	worldId    int32
	username37 uint64
	ch         chan *friendspb.FriendsUpdate
	done       chan struct{}
}

// newSubscriber allocates ch + done with the standard buffer size.
func newSubscriber(worldId int32, username37 uint64) *subscriber {
	return &subscriber{
		worldId:    worldId,
		username37: username37,
		ch:         make(chan *friendspb.FriendsUpdate, subscriberBufferSize),
		done:       make(chan struct{}),
	}
}

// subscriptions is the per-player subscriber registry. All methods are
// goroutine-safe.
type subscriptions struct {
	mu  sync.Mutex
	by  map[uint64]*subscriber // username37 -> subscriber
	log *slog.Logger
}

func newSubscriptions(log *slog.Logger) *subscriptions {
	return &subscriptions{
		by:  make(map[uint64]*subscriber),
		log: log,
	}
}

// register installs sub under sub.username37. If a prior subscriber
// exists for the same username37, it is kicked (its done is closed)
// before sub replaces it. Generalizes TS FriendServer.initializeWorld
// terminate-then-replace (FriendServer.ts:412-419) from per-world to
// per-player.
func (s *subscriptions) register(sub *subscriber) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if prior, ok := s.by[sub.username37]; ok {
		close(prior.done)
	}
	s.by[sub.username37] = sub
}

// deregister removes sub from the registry IFF it is still the
// currently registered subscriber for sub.username37 (a rapid
// re-login may have replaced it under register).
func (s *subscriptions) deregister(sub *subscriber) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if cur, ok := s.by[sub.username37]; ok && cur == sub {
		delete(s.by, sub.username37)
	}
}

// send pushes u to the subscriber for username37 (no-op if none).
// Non-blocking; on full channel, logs warn and drops the update.
func (s *subscriptions) send(username37 uint64, u *friendspb.FriendsUpdate) {
	s.mu.Lock()
	sub, ok := s.by[username37]
	s.mu.Unlock()
	if !ok {
		return
	}
	select {
	case sub.ch <- u:
	default:
		s.log.Warn("friends subscriber buffer full; dropping update",
			slog.Uint64("username37", username37))
	}
}
