package world

import (
	"log/slog"
)

// actionWorldEventsDispatcher composes an inner WorldEventsDispatcher
// (typically slogWorldEventsDispatcher from bridges.go) with a
// WorldStateOps action surface. Slice 5b production wiring.
//
// Each wired On* method calls inner first (preserves slice-5a slog
// observability) and then routes to the WorldStateOps method that
// applies the world-state effect. QUEUESCRIPT remains slog-warn only
// (no ops route) until the runescript runtime can dispatch
// [queue,<name>] triggers — see NAI-S5B-D-NO-RUNESCRIPT-RUNTIME.
//
// The log field carries action-layer Warn lines (lookup misses are
// logged by *Server impls inside the queue closures, not here).
type actionWorldEventsDispatcher struct {
	inner WorldEventsDispatcher
	ops   WorldStateOps
	log   *slog.Logger
}

// Compile-time assertion.
var _ WorldEventsDispatcher = (*actionWorldEventsDispatcher)(nil)

func newActionWorldEventsDispatcher(inner WorldEventsDispatcher, ops WorldStateOps, log *slog.Logger) *actionWorldEventsDispatcher {
	return &actionWorldEventsDispatcher{inner: inner, ops: ops, log: log}
}

func (d *actionWorldEventsDispatcher) OnMute(username37 uint64, mutedUntilMs int64) {
	d.inner.OnMute(username37, mutedUntilMs)
	d.ops.SetPlayerMute(username37, mutedUntilMs)
}

func (d *actionWorldEventsDispatcher) OnKick(username37 uint64) {
	d.inner.OnKick(username37)
	d.ops.KickPlayer(username37)
}

func (d *actionWorldEventsDispatcher) OnShutdown(durationTicks int32) {
	d.inner.OnShutdown(durationTicks)
	d.ops.RelayShutdown(durationTicks)
}

func (d *actionWorldEventsDispatcher) OnBroadcast(message string) {
	d.inner.OnBroadcast(message)
	d.ops.BroadcastMessage(message)
}

func (d *actionWorldEventsDispatcher) OnTrack(username37 uint64, state int32) {
	d.inner.OnTrack(username37, state)
	d.ops.SetPlayerInputTracking(username37, state)
}

func (d *actionWorldEventsDispatcher) OnReload() {
	d.inner.OnReload()
	d.ops.RelayReload()
}

func (d *actionWorldEventsDispatcher) OnClearLogins() {
	d.inner.OnClearLogins()
	d.ops.ClearLogins()
}

func (d *actionWorldEventsDispatcher) OnClearLogouts() {
	d.inner.OnClearLogouts()
	d.ops.ClearLogouts()
}

// OnQueueScript stays slog-warn only — the WorldStateOps interface
// intentionally has no QueueScript method until the runescript runtime
// can resolve [queue,<name>] triggers by name and enqueue on a player.
//
// NAI-S5B-D-NO-RUNESCRIPT-RUNTIME — retires when the runescript runtime
// supports named-script dispatch to a player.
func (d *actionWorldEventsDispatcher) OnQueueScript(scriptName string, username37 uint64) {
	d.inner.OnQueueScript(scriptName, username37)
	d.log.Warn("RELAY_QUEUESCRIPT received but no runtime to dispatch (NAI-S5B-D-NO-RUNESCRIPT-RUNTIME)",
		slog.String("script_name", scriptName),
		slog.Uint64("username37", username37))
}
