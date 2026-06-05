package friends

import (
	"testing"

	"github.com/zsrv/goscape/pkg/friendspb"
)

func TestSubscriptions_RegisterDeregister(t *testing.T) {
	s := newSubscriptions(noopLogger())
	sub := newSubscriber("main", 1, 100)
	s.register(sub)
	s.send("main", 100, &friendspb.FriendsUpdate{})
	select {
	case u := <-sub.ch:
		if u == nil {
			t.Fatalf("unexpected nil update")
		}
	default:
		t.Fatalf("expected update on sub.ch")
	}
	s.deregister(sub)
	s.send("main", 100, &friendspb.FriendsUpdate{}) // no-op now
	select {
	case <-sub.ch:
		t.Fatalf("expected no update after deregister")
	default:
	}
}

func TestSubscriptions_DupRegisterKicksPrior(t *testing.T) {
	s := newSubscriptions(noopLogger())
	a := newSubscriber("main", 1, 100)
	b := newSubscriber("main", 1, 100)
	s.register(a)
	s.register(b)
	select {
	case <-a.done:
	default:
		t.Fatalf("expected prior subscriber done to be closed")
	}
	// b should still be in registry; send routes to b
	s.send("main", 100, &friendspb.FriendsUpdate{})
	select {
	case <-b.ch:
	default:
		t.Fatalf("expected update on new subscriber b.ch")
	}
	// a should not receive
	select {
	case <-a.ch:
		t.Fatalf("expected no update on prior subscriber a.ch")
	default:
	}
}

func TestSubscriptions_DropOnFull(t *testing.T) {
	s := newSubscriptions(noopLogger())
	sub := newSubscriber("main", 1, 100)
	s.register(sub)
	// Fill buffer.
	for range subscriberBufferSize {
		s.send("main", 100, &friendspb.FriendsUpdate{})
	}
	// Next send drops (no panic, no block).
	s.send("main", 100, &friendspb.FriendsUpdate{})
	// Drain to verify exactly subscriberBufferSize updates queued.
	got := 0
	for {
		select {
		case <-sub.ch:
			got++
			continue
		default:
		}
		break
	}
	if got != subscriberBufferSize {
		t.Fatalf("got %d updates, want %d", got, subscriberBufferSize)
	}
}

func TestSubscriptions_DeregisterIgnoresStale(t *testing.T) {
	s := newSubscriptions(noopLogger())
	a := newSubscriber("main", 1, 100)
	b := newSubscriber("main", 1, 100)
	s.register(a)
	s.register(b)   // kicks a
	s.deregister(a) // a is stale; b should remain
	s.send("main", 100, &friendspb.FriendsUpdate{})
	select {
	case <-b.ch:
	default:
		t.Fatalf("expected b to still be registered")
	}
}

func TestSubscriptions_SendUnknownNoop(t *testing.T) {
	s := newSubscriptions(noopLogger())
	// No panic, no block.
	s.send("main", 999, &friendspb.FriendsUpdate{})
}

// TestSubscriptions_ProfileIsolation pins the (profile, username37) re-key
// introduced for rev-244 multi-profile: sending to "beta"/username37 must
// reach only the beta subscriber, leaving the "main" subscriber's channel
// empty. The test fails if the map is re-keyed back to plain username37.
func TestSubscriptions_ProfileIsolation(t *testing.T) {
	s := newSubscriptions(noopLogger())
	const username37 uint64 = 0xAAAA

	mainSub := newSubscriber("main", 1, username37)
	betaSub := newSubscriber("beta", 1, username37)
	s.register(mainSub)
	s.register(betaSub)

	update := &friendspb.FriendsUpdate{}
	s.send("beta", username37, update)

	// betaSub must receive the update.
	select {
	case got := <-betaSub.ch:
		if got != update {
			t.Fatalf("betaSub: got %v, want %v", got, update)
		}
	default:
		t.Fatal("betaSub: expected update; got none")
	}

	// mainSub must remain empty — profiles are isolated.
	select {
	case <-mainSub.ch:
		t.Fatal("mainSub: received update targeted at beta profile; profiles are not isolated")
	default:
	}
}
