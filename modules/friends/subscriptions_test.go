package friends

import (
	"testing"

	"github.com/zsrv/goscape/pkg/friendspb"
)

func TestSubscriptions_RegisterDeregister(t *testing.T) {
	s := newSubscriptions(noopLogger())
	sub := newSubscriber(1, 100)
	s.register(sub)
	s.send(100, &friendspb.FriendsUpdate{})
	select {
	case u := <-sub.ch:
		if u == nil {
			t.Fatalf("unexpected nil update")
		}
	default:
		t.Fatalf("expected update on sub.ch")
	}
	s.deregister(sub)
	s.send(100, &friendspb.FriendsUpdate{}) // no-op now
	select {
	case <-sub.ch:
		t.Fatalf("expected no update after deregister")
	default:
	}
}

func TestSubscriptions_DupRegisterKicksPrior(t *testing.T) {
	s := newSubscriptions(noopLogger())
	a := newSubscriber(1, 100)
	b := newSubscriber(1, 100)
	s.register(a)
	s.register(b)
	select {
	case <-a.done:
	default:
		t.Fatalf("expected prior subscriber done to be closed")
	}
	// b should still be in registry; send routes to b
	s.send(100, &friendspb.FriendsUpdate{})
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
	sub := newSubscriber(1, 100)
	s.register(sub)
	// Fill buffer.
	for range subscriberBufferSize {
		s.send(100, &friendspb.FriendsUpdate{})
	}
	// Next send drops (no panic, no block).
	s.send(100, &friendspb.FriendsUpdate{})
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
	a := newSubscriber(1, 100)
	b := newSubscriber(1, 100)
	s.register(a)
	s.register(b) // kicks a
	s.deregister(a) // a is stale; b should remain
	s.send(100, &friendspb.FriendsUpdate{})
	select {
	case <-b.ch:
	default:
		t.Fatalf("expected b to still be registered")
	}
}

func TestSubscriptions_SendUnknownNoop(t *testing.T) {
	s := newSubscriptions(noopLogger())
	// No panic, no block.
	s.send(999, &friendspb.FriendsUpdate{})
}
