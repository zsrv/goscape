package world

import (
	"fmt"
	"path/filepath"
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
	if err := writePackStamp(filepath.Join(s.cfg.CachePath, ".pack-stamp"), r.startedAt); err != nil {
		s.log.Warn("rebuild: writePackStamp failed", "err", err)
		// continue — next session triggers one spurious replay, self-heals.
	}
	msg := fmt.Sprintf("Rebuilt: %s.", r.duration.Round(time.Millisecond))
	s.broadcastRebuildStaff(r.invoker, msg)
}

// broadcastRebuildStaff sends msg privately to invoker (if non-nil)
// AND to every online player with staffModLevel >= 4. Deduplicates.
// Spec §4.5 (docs/superpowers/specs/2026-05-18-rebuild-async-fsnotify-design.md).
//
// GAP-WORLD-RELOAD-EVENTS-3-D-STAFF-ONLY-REBUILD-BROADCAST — CONFIRMED EXCEPTION
// (closes gap-world-reload-events-3 from the 2026-05-28 fresh audit).
// TS World's rebuild-progress / outcome messages (the
// `dev_progress`, `dev_failure`, and `dev_thread exit` paths at
// World.ts:1750-1758, 1803-1811) all funnel into
// World.broadcastMes (World.ts:1808-1816), which iterates the FULL
// playerLoop — every connected player receives every "Rebuilding…",
// "Rebuilt: Ns.", and "Rebuild failed: …" line.
//
// goscape deliberately narrows the broadcast to staff (modlvl >= 4)
// plus the manual invoker. The §4.5 spec entry chose this scope to
// avoid spamming every connected player with rebuild churn during
// development sessions — content authors and operators see the
// progress they care about; non-staff players see nothing. The same
// trade-off has already shipped through the rebuild-async-fsnotify
// design review (#2026-05-18) and the content-watcher-replay design
// (#docs/superpowers/plans/2026-05-18-content-watcher-replay.md);
// promoting the broadcast back to "all players" would re-litigate
// that decision against the operator-noise rationale that motivated
// the spec.
//
// Coalesced manual invokers, fsnotify-triggered rebuilds with no
// invoker, and the `::rebuild` ContentPath-empty private-error path
// are covered by the staff-broadcast tests at
// modules/world/rebuild_broadcast_test.go
// (TestBroadcastRebuildStaff_DeliversToInvokerAndStaffOnly,
// TestBroadcastRebuildStaff_FsnotifyTriggered_NoInvoker) and
// modules/world/handler_cheat_rebuild_test.go
// (TestHandleClientCheat_Rebuild_DispatchesAndBroadcastsInProgress,
// TestHandleClientCheat_Rebuild_Async_PackError_BroadcastsFailureAndSkipsReload).
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
