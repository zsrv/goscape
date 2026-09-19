package world

import (
	"context"
	"errors"
	"time"
)

// readyStaleAfter: a tick loop silent for this long is wedged — the
// world accepts TCP but strands players, which is exactly what the old
// tcpSocket readiness probe could not see (arch-29.6).
const readyStaleAfter = 10 * time.Second

// readyBootGrace: how long after the world server is constructed a world that
// has not yet completed its first tick is still treated as "starting" rather
// than "wedged". Construction happens during world module init (≈ process
// start) and the readiness surfaces answer concurrently with the world's
// startingFn, so this window spans the world's whole startup — cache load
// (cache.MakeCRCs), Listen, an async WorldStartup retry — not just the ~600ms
// tick cycle. 30s is generous for a normal cache; an abnormally slow
// cold-cache load could push the first tick past it and briefly report
// "no first tick", which is acceptable (it is self-healing on the first tick).
// Past the grace with still no first tick, a world is wedged during startup —
// the blind spot the pre-first-tick grace alone would otherwise hide forever
// (LastTick stays at its zero value, so time.Since is never consulted). This
// bound is the arch-29.6 follow-up.
const readyBootGrace = 30 * time.Second

// ErrStarting means the world is still inside its boot grace: it has not
// completed a first tick yet, but it has not been given long enough to call
// that wedged either. It is a not-ready verdict; callers decide what to do
// with it. The process-wide /readyz treats it as not ready (a pod must not
// receive players before its first tick), while the legacy ondemand /healthz
// route keeps answering 200 for it — see modules/ondemand/health.go and the
// adapter in cmd/goscape/app/modules.go.
var ErrStarting = errors.New("starting")

// errNoFirstTick / errTickStale are the two "wedged" verdicts. Their messages
// are the reason strings the ondemand /healthz body has always carried, so
// they are load-bearing: operators and the Helm chart match on them.
var (
	errNoFirstTick = errors.New("no first tick")
	errTickStale   = errors.New("tick stale")
)

// readyStatus decides the world's readiness verdict. It is pure so the
// boot-deadline branch is testable without a real clock: sinceBoot (how long
// the server has existed) is injected. The staleness branch still reads
// time.Since(s.LastTick), which tests control by choosing LastTick relative to
// time.Now().
//
// Boot-grace decision (arch-29.6): before the tick loop has completed a single
// tick, HealthSnapshot.LastTick is time.Unix(0, 0) (the atomic backing it
// defaults to zero) rather than a real timestamp, and Go's zero time.Time from
// a Server that never stamped anything reads the same way. Both satisfy
// LastTick.Unix() <= 0.
//
//   - before the first tick: ErrStarting while sinceBoot is within
//     readyBootGrace, then errNoFirstTick — a tick loop that never produced a
//     first tick within the grace window is wedged during startup, not merely
//     booting.
//   - after the first tick: errTickStale if the last tick is older than
//     readyStaleAfter (a stalled tick loop), else nil.
//
// Pinned by TestReadyStatus_* and, at the HTTP level, by modules/ondemand's
// TestHealthz* tests.
func readyStatus(s HealthSnapshot, sinceBoot time.Duration) error {
	if s.LastTick.Unix() <= 0 {
		if sinceBoot > readyBootGrace {
			return errNoFirstTick
		}
		return ErrStarting
	}
	if time.Since(s.LastTick) > readyStaleAfter {
		return errTickStale
	}
	return nil
}

// CheckReady reports whether this world is ready to receive players: nil when
// the tick loop is running normally, ErrStarting inside the boot grace, and
// errNoFirstTick / errTickStale when it is wedged.
//
// It is the world's contribution to the process-wide readiness definition
// (cmd/goscape/app.App.Ready), the analogue of Loki's Ingester.CheckReady. The
// boot-grace reference is the server's own construction time (bootTime, set in
// NewServer), so the verdict belongs to the world module rather than to
// whichever HTTP surface happens to ask. Safe to call from any goroutine:
// HealthSnapshot reads only atomics and bootTime is written once, before the
// Server escapes NewServer.
func (s *Server) CheckReady(_ context.Context) error {
	return readyStatus(s.HealthSnapshot(), time.Since(s.bootTime))
}
