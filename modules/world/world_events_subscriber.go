package world

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"time"

	"github.com/zsrv/goscape/pkg/friendspb"
)

// worldEventsSubscriberBackoff* tunes the supervisor reconnect cadence.
// Same posture as friendsSubscriberBackoff* but separate constants so
// future tuning can diverge.
const (
	worldEventsSubscriberBackoffMin = time.Second
	worldEventsSubscriberBackoffMax = 30 * time.Second
	worldEventsSubscriberSteady     = 60 * time.Second
)

// worldEventsSubscriber owns one world's SubscribeWorldEvents stream
// lifetime. Started by world.Server at process boot; stopped when the
// Server's ctx is canceled (Server.Shutdown).
//
// Each iteration:
//   - SubscribeWorldEvents(ctx, req) → stream
//   - Recv loop dispatches WorldEvent variants to WorldEventsDispatcher
//   - On error/EOF: log, exp-backoff, reconnect (unless ctx canceled)
//
// Structurally identical to friendsSubscriber (modules/world/friends_subscriber.go)
// but for the per-world stream + dispatcher.
type worldEventsSubscriber struct {
	client     FriendsClient
	worldID    int32
	profile    string
	dispatcher WorldEventsDispatcher
	log        *slog.Logger
}

func newWorldEventsSubscriber(client FriendsClient, worldID int32, profile string, dispatcher WorldEventsDispatcher, log *slog.Logger) *worldEventsSubscriber {
	return &worldEventsSubscriber{
		client:     client,
		worldID:    worldID,
		profile:    profile,
		dispatcher: dispatcher,
		log:        log,
	}
}

// run is the supervisor loop. Blocks until ctx is canceled. Caller
// should typically invoke as `go sub.run(ctx)`.
func (s *worldEventsSubscriber) run(ctx context.Context) {
	backoff := worldEventsSubscriberBackoffMin
	for {
		if ctx.Err() != nil {
			return
		}
		runStart := time.Now()
		err := s.runOnce(ctx)
		if ctx.Err() != nil {
			return
		}
		if time.Since(runStart) >= worldEventsSubscriberSteady {
			backoff = worldEventsSubscriberBackoffMin
		}
		if errors.Is(err, io.EOF) {
			s.log.Info("world events subscriber EOF; reconnecting",
				slog.Int("world_id", int(s.worldID)),
				slog.Duration("backoff", backoff))
		} else {
			s.log.Warn("world events subscriber disconnected; reconnecting",
				slog.Int("world_id", int(s.worldID)),
				slog.Duration("backoff", backoff),
				slog.Any("err", err))
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff = nextWorldEventsBackoff(backoff)
	}
}

func nextWorldEventsBackoff(d time.Duration) time.Duration {
	d *= 2
	if d > worldEventsSubscriberBackoffMax {
		d = worldEventsSubscriberBackoffMax
	}
	return d
}

// runOnce opens a single stream and drains it. Returns when the stream
// ends (error or EOF).
func (s *worldEventsSubscriber) runOnce(ctx context.Context) error {
	stream, err := s.client.SubscribeWorldEvents(ctx, &friendspb.SubscribeWorldEventsRequest{
		WorldId: s.worldID,
		Profile: s.profile,
	})
	if err != nil {
		return err
	}
	for {
		ev, err := stream.Recv()
		if err != nil {
			return err
		}
		s.dispatch(ev)
	}
}

// dispatch routes one WorldEvent variant to the dispatcher.
func (s *worldEventsSubscriber) dispatch(ev *friendspb.WorldEvent) {
	switch v := ev.Event.(type) {
	case *friendspb.WorldEvent_Mute:
		s.dispatcher.OnMute(v.Mute.Username37, v.Mute.MutedUntilMs)
	case *friendspb.WorldEvent_Kick:
		s.dispatcher.OnKick(v.Kick.Username37)
	case *friendspb.WorldEvent_Shutdown:
		s.dispatcher.OnShutdown(v.Shutdown.DurationTicks)
	case *friendspb.WorldEvent_Broadcast:
		s.dispatcher.OnBroadcast(v.Broadcast.Message)
	case *friendspb.WorldEvent_Track:
		s.dispatcher.OnTrack(v.Track.Username37, v.Track.State)
	case *friendspb.WorldEvent_Reload:
		s.dispatcher.OnReload()
	case *friendspb.WorldEvent_ClearLogins:
		s.dispatcher.OnClearLogins()
	case *friendspb.WorldEvent_ClearLogouts:
		s.dispatcher.OnClearLogouts()
	case *friendspb.WorldEvent_QueueScript:
		s.dispatcher.OnQueueScript(v.QueueScript.ScriptName, v.QueueScript.Username37)
	default:
		s.log.Warn("world events subscriber received unknown event variant",
			slog.Int("world_id", int(s.worldID)))
	}
}
