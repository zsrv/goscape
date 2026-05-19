package world

// enqueueRelayAction posts a closure onto the relay action queue.
// Non-blocking: drops the action if the queue is full (matches
// NAI-S5A-D-WORLDEVENTS-DROP-ON-FULL server-side posture). Called by
// WorldStateOps methods on the per-world subscriber goroutine.
//
// A dropped action represents one lost RELAY_* event. Logged at Warn
// so operators see queue pressure. In practice the queue is sized
// generously (64) and tick cadence is sub-second, so drops should be
// rare.
func (s *Server) enqueueRelayAction(action func()) {
	select {
	case s.relayActionQueue <- action:
	default:
		s.log.Warn("relay action queue full; dropping action")
	}
}

// drainRelayActions runs every pending action on the queue. Must be
// invoked from the tick goroutine. Non-blocking — exits as soon as the
// queue is empty. Actions are executed in FIFO order in the same
// iteration; they observe and mutate tick-owned state directly.
//
// Placement: top of tick loop body, between the rebuildResult drain and
// processShutdown so that a RELAY_SHUTDOWN that arrived this iteration
// can take effect on this same tick.
func (s *Server) drainRelayActions() {
	for {
		select {
		case action := <-s.relayActionQueue:
			action()
		default:
			return
		}
	}
}
