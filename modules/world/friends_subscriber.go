package world

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"time"

	"github.com/zsrv/goscape/pkg/friendspb"
)

// friendsSubscriberBackoffMin is the initial reconnect delay after a
// stream failure. Doubles up to friendsSubscriberBackoffMax; resets to
// the min after the most recent run lasted ≥ friendsSubscriberSteady.
// Mirrors the [[content-watcher-auto-restart]] supervisor cadence.
const (
	friendsSubscriberBackoffMin = time.Second
	friendsSubscriberBackoffMax = 30 * time.Second
	friendsSubscriberSteady     = 60 * time.Second
)

// friendsSubscriber owns one player's SubscribeUpdates stream lifetime.
// Started by world.Server when the player is admitted to the world;
// stopped by canceling its ctx when the player logs out / disconnects.
//
// Each iteration:
//   - SubscribeUpdates(ctx, req) → stream
//   - Recv loop dispatches updates to FriendsDispatcher
//   - On error/EOF: log, exp-backoff, reconnect (unless ctx canceled)
type friendsSubscriber struct {
	client     FriendsClient
	worldID    int32
	profile    string
	username37 uint64
	dispatcher FriendsDispatcher
	log        *slog.Logger
}

func newFriendsSubscriber(client FriendsClient, worldID int32, profile string, username37 uint64, dispatcher FriendsDispatcher, log *slog.Logger) *friendsSubscriber {
	return &friendsSubscriber{
		client:     client,
		worldID:    worldID,
		profile:    profile,
		username37: username37,
		dispatcher: dispatcher,
		log:        log,
	}
}

// run is the supervisor loop. Blocks until ctx is canceled. Caller
// should typically invoke as `go sub.run(ctx)`.
func (s *friendsSubscriber) run(ctx context.Context) {
	backoff := friendsSubscriberBackoffMin
	for {
		if ctx.Err() != nil {
			return
		}
		runStart := time.Now()
		err := s.runOnce(ctx)
		if ctx.Err() != nil {
			return
		}
		// Reset backoff if the failed run lasted long enough to count
		// as "steady". Distinct from a fast-fail loop that should keep
		// the longer backoff.
		if time.Since(runStart) >= friendsSubscriberSteady {
			backoff = friendsSubscriberBackoffMin
		}
		// EOF means the server closed cleanly (e.g., we got kicked by a
		// newer subscriber for the same username37). Log at Info rather
		// than Warn.
		if errors.Is(err, io.EOF) {
			s.log.Info("friends subscriber EOF; reconnecting",
				slog.Uint64("username37", s.username37),
				slog.Duration("backoff", backoff))
		} else {
			s.log.Warn("friends subscriber disconnected; reconnecting",
				slog.Uint64("username37", s.username37),
				slog.Duration("backoff", backoff),
				slog.Any("err", err))
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff = nextBackoff(backoff)
	}
}

func nextBackoff(d time.Duration) time.Duration {
	d *= 2
	if d > friendsSubscriberBackoffMax {
		d = friendsSubscriberBackoffMax
	}
	return d
}

// runOnce opens a single stream and drains it. Returns when the stream
// ends (error or EOF).
func (s *friendsSubscriber) runOnce(ctx context.Context) error {
	stream, err := s.client.SubscribeUpdates(ctx, &friendspb.SubscribeUpdatesRequest{
		WorldId:    s.worldID,
		Profile:    s.profile,
		Username37: s.username37,
	})
	if err != nil {
		return err
	}
	for {
		u, err := stream.Recv()
		if err != nil {
			return err
		}
		s.dispatch(u)
	}
}

// dispatch routes one FriendsUpdate to the appropriate dispatcher
// method based on the oneof variant.
func (s *friendsSubscriber) dispatch(u *friendspb.FriendsUpdate) {
	switch v := u.Update.(type) {
	case *friendspb.FriendsUpdate_Friendlist:
		s.dispatcher.OnFriendlistUpdate(s.username37, v.Friendlist.Entries)
	case *friendspb.FriendsUpdate_Ignorelist:
		s.dispatcher.OnIgnorelistUpdate(s.username37, v.Ignorelist.Username37)
	case *friendspb.FriendsUpdate_PrivateMessage:
		pm := v.PrivateMessage
		s.dispatcher.OnPrivateMessage(s.username37, pm.FromUsername37, pm.StaffLvl, pm.PmId, pm.Chat)
	default:
		s.log.Warn("friends subscriber received unknown update variant",
			slog.Uint64("username37", s.username37))
	}
}
