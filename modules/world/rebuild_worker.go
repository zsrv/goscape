package world

import (
	"fmt"
	"time"
)

// rebuildResult is the completion event posted by runRebuildWorker and
// drained on the tick goroutine. Spec 2026-05-18-rebuild-async-fsnotify §3.
// startedAt is the time.Now() snapshot taken immediately before packFn;
// used by handleRebuildResult to write CachePath/.pack-stamp on the
// success path so the next runWatchSession can detect content edits made
// after this pack (spec 2026-05-18-content-watcher-replay §3.5).
type rebuildResult struct {
	err       error
	duration  time.Duration
	invoker   *Player // nil for fsnotify-triggered rebuilds
	startedAt time.Time
}

// dispatchRebuildRequest is the single non-blocking-send helper used by
// both the ::rebuild handler and the contentWatcher. Sets rebuildPending
// under rebuildMu, then tries a non-blocking send on rebuildReq. The
// pending flag + chan together implement TS DevThread's
// active/processNextQueue/processNextTimeout coalescing.
func (s *Server) dispatchRebuildRequest() {
	s.rebuildMu.Lock()
	s.rebuildPending = true
	s.rebuildMu.Unlock()

	select {
	case s.rebuildReq <- struct{}{}:
	default:
	}
}

// runRebuildWorker is the body of the long-lived pack worker goroutine.
// Exits when s.quit is closed. Mirrors TS DevThread.processChangedFiles.
func (s *Server) runRebuildWorker() {
	for {
		select {
		case <-s.quit:
			return
		case <-s.rebuildReq:
		}

		s.rebuildMu.Lock()
		s.rebuildBusy = true
		s.rebuildPending = false
		invoker := s.rebuildManualInvoker
		s.rebuildMu.Unlock()

		start := time.Now()
		err := s.packFn(s.cfg.ContentPath, s.cfg.CachePath, s.cfg.CachePath)
		elapsed := time.Since(start)

		// Blocking send + s.quit select: back-pressure ensures tick
		// drains and runs Reload before worker accepts the next
		// request. Worst-case wait ≈ 1 tick.
		select {
		case s.rebuildResult <- rebuildResult{err: err, duration: elapsed, invoker: invoker, startedAt: start}:
		case <-s.quit:
			return
		}

		s.rebuildMu.Lock()
		s.rebuildBusy = false
		s.rebuildManualInvoker = nil
		pending := s.rebuildPending
		s.rebuildPending = false
		s.rebuildMu.Unlock()

		if pending {
			select {
			case s.rebuildReq <- struct{}{}:
			default:
			}
		}
	}
}

// handleRebuildResult runs on the tick goroutine. Calls reloadFn on
// success and broadcasts the outcome to invoker (if any) + all online
// staff (modlvl >= 4) via broadcastRebuildStaff.
func (s *Server) handleRebuildResult(r rebuildResult) {
	if r.err != nil {
		s.log.Error("rebuild: PackAll failed", "err", r.err)
		s.broadcastRebuildStaff(r.invoker, "Rebuild failed: "+r.err.Error())
		return
	}
	if err := s.reloadFn(true); err != nil {
		s.log.Error("rebuild: Reload failed", "err", err)
		s.broadcastRebuildStaff(r.invoker, "Rebuild failed: reload returned error (see server log).")
		return
	}
	msg := fmt.Sprintf("Rebuilt: %s.", r.duration.Round(time.Millisecond))
	s.broadcastRebuildStaff(r.invoker, msg)
}

// broadcastRebuildStaff sends msg privately to invoker (if non-nil)
// AND to every online player with staffModLevel >= 4. Deduplicates.
// Spec §4.5.
func (s *Server) broadcastRebuildStaff(invoker *Player, msg string) {
	s.playersMu.RLock()
	players := make([]*Player, len(s.playerLoop))
	copy(players, s.playerLoop)
	s.playersMu.RUnlock()

	delivered := false
	for _, p := range players {
		if p.staffModLevel < 4 && p != invoker {
			continue
		}
		p.MessageGame(msg)
		if p == invoker {
			delivered = true
		}
	}
	if invoker != nil && !delivered {
		// Invoker not in playerLoop (e.g., test scaffolding) — deliver
		// directly so per-invoker messages still pin.
		invoker.MessageGame(msg)
	}
}
