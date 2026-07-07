package world

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// friendsDispatchWarnThresholds are the queue-depth points at which
// friendsMutationDispatcher logs one Warn (not one per enqueue) so
// operators can see a wedged friends-server backing the queue up. This is
// goscape-only operational visibility — TS's postMessage queue has no
// equivalent instrumentation — layered on top of an otherwise
// TS-faithful unbounded queue (see the type doc).
var friendsDispatchWarnThresholds = []int{256, 1024}

// friendsMutationDispatcher is the single global ordered FIFO queue for
// every friends-server MUTATION RPC (arch-29.13), restoring the TS
// ordering guarantee that goscape's per-RPC goroutine fan-out had lost.
//
// TS posts EVERY friends-server mutation through one
// World.friendThread.postMessage channel — strictly FIFO, globally,
// across every player. Before this dispatcher, goscape fired one
// independent goroutine per RPC (grpcFriendsBridge's Add/RemoveFriend,
// Add/RemoveIgnore, SetChatMode, PrivateMessage; the inline
// PlayerLogin call in tick.go's processLogins; the inline PlayerLogout
// call in server.go's removePlayerOnTick), so gRPC gave no cross-RPC
// ordering guarantee: a login/logout pair for the same player could
// apply to the friends server as logout-then-login (leaving the player
// permanently "online" from the friends server's point of view, still
// counted against its world cap, until a fresh login re-registers them),
// and an add-then-delete pair could apply as delete-then-add. This is
// the 2026-07-02 five-agent architecture review's MED finding "friends
// RPC fan-out loses TS FIFO ordering."
//
// Naming note: this is deliberately NOT named "friendsDispatcher" —
// that identifier (and the Server.friendsDispatcher field) already
// belongs to the pre-existing FriendsDispatcher interface (bridges.go),
// which is the opposite-direction sink for friends-server -> world
// pushed updates (OnFriendlistUpdate / OnIgnorelistUpdate /
// OnPrivateMessage, delivered over the SubscribeUpdates stream). This
// type is the world -> friends-server outbound mutation queue; the two
// are unrelated and intentionally distinguishable at a glance.
//
// Design is a SINGLE global queue, NOT one queue per player: TS's
// postMessage channel has no per-player partitioning, so splitting the
// queue by player would still let unrelated players' mutations
// interleave in ways TS's single channel forbids — and, more to the
// point, would not even fix the motivating login/logout race for a
// SINGLE player, since that player's login and logout must serialize
// through the very same queue processed by the very same worker.
//
// Unbounded by design: never drops, never reorders. This mirrors TS's
// own postMessage queue, which is also unbounded — a wedged
// friends-server backs goscape's queue up exactly the way it would back
// TS's up. friendsDispatchWarnThresholds gives operators visibility into
// that pressure without making the queue drop-capable.
//
// Exactly ONE worker goroutine (run) dequeues and executes actions
// strictly in enqueue order, each wrapped in its own
// context.WithTimeout(bridgesCtx, callTimeout). The timeout bounds the
// gRPC CALL inside the action — ctx expiry makes the RPC return, so a
// hung friends-server NETWORK PATH cannot wedge the queue beyond
// callTimeout per entry — but it cannot preempt anything the action
// runs synchronously after the RPC returns. In particular, the
// PlayerLogin onResponse callback executes on this worker goroutine
// outside any cancellation reach: callbacks (and any other post-RPC
// code inside an enqueued action) MUST be fast and non-blocking,
// because a blocking one stalls EVERY queued friends mutation — the
// old per-call goroutine fan-out isolated such a stall to one
// goroutine; the single worker does not. Today's only callback is a
// Warn log (tick.go processLogins), which satisfies the contract. A
// slow-but-legal call MAY head-of-line-block every mutation queued
// behind it for up to callTimeout. That head-of-line blocking is
// deliberate and TS-faithful: TS's single friend-thread also processes
// postMessage entries strictly one at a time. Total-outage fast-fail is
// handled upstream by arch-29.2's gRPC keepalives (a dead
// friends-server surfaces as UNAVAILABLE quickly rather than hanging
// out each call's full timeout).
//
// Lifecycle: run must be started exactly once, from NewWorldService's
// startingBody (alongside the retryBridgeRegistration calls — this
// branch predates arch-29.8's Listen()/subscriber-in-starting
// refactor, so the world-events subscriber itself still starts from
// NewServer; the dispatcher worker does not depend on that and starts
// from startingBody directly), folded into Server.bridgeWg so
// Shutdown's existing bridgeWg.Wait() (called after bridgesCancel)
// joins it. run exits as soon as its ctx (bridgesCtx) is Done, WHETHER
// OR NOT the queue is empty — it does not attempt to drain remaining
// entries first. By the time bridgesCancel fires, the tick goroutine
// (the sole producer for every converted call site) has already
// exited (Server.Shutdown joins tickWg before cancelling bridgesCtx),
// so any items still queued at that point are best-effort presence
// traffic that will never be produced again this process lifetime —
// dropping them here matches TS, which also loses its unprocessed
// postMessage queue on process exit.
type friendsMutationDispatcher struct {
	mu    sync.Mutex
	queue []func(context.Context)

	// notify wakes the worker when the queue transitions from empty to
	// non-empty. Buffered 1: multiple enqueues before the worker wakes
	// coalesce into a single wake, which is fine — run always drains the
	// whole queue before blocking again.
	notify chan struct{}

	// warned tracks which depth thresholds have already fired their
	// one-shot Warn. Never reset: a queue that drains and later refills
	// past the same threshold does not warn again — operators already
	// know that threshold is reachable.
	warned map[int]bool

	// callTimeout bounds the context passed to each dequeued action —
	// i.e. the gRPC call inside it; it cannot preempt synchronous
	// post-RPC code such as the PlayerLogin onResponse callback (see the
	// type doc's callback contract). Defaults to bridgeCallTimeout;
	// tests shrink it to keep the deterministic head-of-line-blocking
	// proofs (friends_dispatcher_test.go) fast. Tests write this field
	// without holding mu, which is safe only because they do so BEFORE
	// enqueuing any work: enqueue's mu critical section then run's mu
	// acquire for that dequeue establish the happens-before edge from
	// the write to the worker's read, and run re-reads the field on
	// every iteration after that acquire rather than caching it once
	// before the loop — keep it that way.
	callTimeout time.Duration

	log *slog.Logger
}

// newFriendsMutationDispatcher constructs an idle dispatcher: no worker
// goroutine is started (callers must call run separately — see the type
// doc's Lifecycle section). Safe to enqueue against before run starts;
// the first enqueue's notify simply sits buffered until a worker exists
// to consume it.
func newFriendsMutationDispatcher(log *slog.Logger) *friendsMutationDispatcher {
	return &friendsMutationDispatcher{
		notify:      make(chan struct{}, 1),
		warned:      make(map[int]bool, len(friendsDispatchWarnThresholds)),
		callTimeout: bridgeCallTimeout,
		log:         log,
	}
}

// enqueue appends action to the tail of the FIFO queue and wakes the
// worker if it is blocked waiting for work. Safe to call from any
// goroutine, at any time — including before run has started, and after
// ctx has been cancelled and run has returned. A post-shutdown enqueue
// is a harmless no-op in effect: enqueue itself never spawns a
// goroutine, so there is nothing to leak; the closure is appended to the
// queue and simply never dequeued (no worker remains to run it).
func (d *friendsMutationDispatcher) enqueue(action func(context.Context)) {
	d.mu.Lock()
	d.queue = append(d.queue, action)
	depth := len(d.queue)
	d.mu.Unlock()

	// Ascending thresholds: once depth is below one, it is below every
	// later (larger) one too.
	for _, threshold := range friendsDispatchWarnThresholds {
		if depth < threshold {
			break
		}
		d.mu.Lock()
		already := d.warned[threshold]
		if !already {
			d.warned[threshold] = true
		}
		d.mu.Unlock()
		if !already {
			d.log.Warn("friends dispatch queue depth crossed threshold",
				slog.Int("depth", depth), slog.Int("threshold", threshold))
		}
	}

	select {
	case d.notify <- struct{}{}:
	default:
	}
}

// run is the single dedicated worker: dequeues in strict FIFO order and
// executes each action synchronously, so a slow action head-of-line
// blocks everything queued behind it (deliberate — see type doc). Exits
// promptly once ctx is Done, even with a non-empty queue (see the type
// doc's Lifecycle section: remaining entries are abandoned, not
// drained). Must be started exactly once.
func (d *friendsMutationDispatcher) run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		d.mu.Lock()
		for len(d.queue) == 0 {
			d.mu.Unlock()
			select {
			case <-ctx.Done():
				return
			case <-d.notify:
			}
			d.mu.Lock()
		}
		action := d.queue[0]
		d.queue[0] = nil // don't retain the closure in the backing array
		d.queue = d.queue[1:]
		d.mu.Unlock()

		callCtx, cancel := context.WithTimeout(ctx, d.callTimeout)
		action(callCtx)
		cancel()
	}
}
