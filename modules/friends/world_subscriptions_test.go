package friends

import (
	"testing"

	"github.com/zsrv/goscape/pkg/friendspb"
)

func TestWorldSubscriptions_RegisterDeregister(t *testing.T) {
	s := newWorldSubscriptions(noopLogger())
	sub := newWorldSubscriber(1)
	s.register(sub)
	// Send routes to the registered subscriber.
	ev := &friendspb.WorldEvent{Event: &friendspb.WorldEvent_Reload{Reload: &friendspb.ReloadEvent{}}}
	s.send(1, ev)
	select {
	case got := <-sub.ch:
		if got != ev {
			t.Fatalf("got %v, want %v", got, ev)
		}
	default:
		t.Fatal("expected event in channel; got none")
	}
	s.deregister(sub)
	// Now send is a silent no-op (no subscriber).
	s.send(1, ev)
	select {
	case <-sub.ch:
		t.Fatal("expected no event after deregister")
	default:
	}
}

func TestWorldSubscriptions_DupRegisterKicksPrior(t *testing.T) {
	s := newWorldSubscriptions(noopLogger())
	prior := newWorldSubscriber(1)
	s.register(prior)
	next := newWorldSubscriber(1)
	s.register(next)
	// Prior's done is closed; next is current.
	select {
	case <-prior.done:
	default:
		t.Fatal("expected prior.done to be closed by register-on-conflict")
	}
	ev := &friendspb.WorldEvent{Event: &friendspb.WorldEvent_Reload{Reload: &friendspb.ReloadEvent{}}}
	s.send(1, ev)
	select {
	case <-next.ch:
	default:
		t.Fatal("expected event to route to next, not prior")
	}
	select {
	case <-prior.ch:
		t.Fatal("event routed to prior; should have gone to next only")
	default:
	}
}

func TestWorldSubscriptions_DeregisterIdentityChecked(t *testing.T) {
	s := newWorldSubscriptions(noopLogger())
	prior := newWorldSubscriber(1)
	s.register(prior)
	next := newWorldSubscriber(1)
	s.register(next) // kicks prior
	// Deregistering the (now-stale) prior must NOT remove next.
	s.deregister(prior)
	// next must still be registered: send routes to it.
	ev := &friendspb.WorldEvent{Event: &friendspb.WorldEvent_Reload{Reload: &friendspb.ReloadEvent{}}}
	s.send(1, ev)
	select {
	case <-next.ch:
	default:
		t.Fatal("expected event after stale deregister; got none")
	}
}

// TestWorldSubscriptions_CloseAll mirrors TestSubscriptions_CloseAll
// (arch-29.4) for the world-events registry: closeAll releases every
// live subscriber's done channel exactly once and empties the registry
// so a subsequent register does not see (and re-close) a stale entry.
func TestWorldSubscriptions_CloseAll(t *testing.T) {
	s := newWorldSubscriptions(noopLogger())
	a := newWorldSubscriber(1)
	b := newWorldSubscriber(2)
	s.register(a)
	s.register(b)

	s.closeAll()

	for _, sub := range []*worldSubscriber{a, b} {
		select {
		case <-sub.done:
		default:
			t.Fatalf("expected sub.done closed by closeAll")
		}
	}
	s.mu.Lock()
	n := len(s.by)
	s.mu.Unlock()
	if n != 0 {
		t.Fatalf("expected registry emptied by closeAll, got %d entries", n)
	}

	// A late registration for a key closeAll already cleared must not
	// panic and must not itself be pre-closed.
	c := newWorldSubscriber(1)
	s.register(c)
	select {
	case <-c.done:
		t.Fatalf("newly registered subscriber's done must not be closed")
	default:
	}
}

func TestWorldSubscriptions_SendNoSubscriberSilent(t *testing.T) {
	s := newWorldSubscriptions(noopLogger())
	ev := &friendspb.WorldEvent{Event: &friendspb.WorldEvent_Reload{Reload: &friendspb.ReloadEvent{}}}
	// Should not panic, should not block.
	s.send(42, ev)
}

func TestWorldSubscriptions_DropOnFull(t *testing.T) {
	s := newWorldSubscriptions(noopLogger())
	sub := newWorldSubscriber(1)
	s.register(sub)
	ev := &friendspb.WorldEvent{Event: &friendspb.WorldEvent_Reload{Reload: &friendspb.ReloadEvent{}}}
	// Fill the buffer.
	for range worldSubscriberBufferSize {
		s.send(1, ev)
	}
	// Overflow event is dropped (not blocking the caller).
	s.send(1, ev)
	// Drain to verify exactly worldSubscriberBufferSize events queued.
	got := 0
drain:
	for {
		select {
		case <-sub.ch:
			got++
		default:
			break drain
		}
	}
	if got != worldSubscriberBufferSize {
		t.Fatalf("got %d events, want %d", got, worldSubscriberBufferSize)
	}
}
