package world

// actionWorldEventsDispatcher composes an inner WorldEventsDispatcher
// (typically slogWorldEventsDispatcher from bridges.go) with a
// WorldStateOps action surface. Slice 5b production wiring.
//
// Each wired On* method calls inner first (preserves slice-5a slog
// observability) and then routes to the WorldStateOps method that
// applies the world-state effect. All 9 RELAY_* opcodes route to
// real ops as of the runtime-fixups-cluster slice (NAI-S5B-D-NO-
// RUNESCRIPT-RUNTIME retired).
//
// Lookup misses and other action-layer diagnostics are logged by
// *Server impls inside the queue closures, not here.
type actionWorldEventsDispatcher struct {
	inner WorldEventsDispatcher
	ops   WorldStateOps
}

// Compile-time assertion.
var _ WorldEventsDispatcher = (*actionWorldEventsDispatcher)(nil)

func newActionWorldEventsDispatcher(inner WorldEventsDispatcher, ops WorldStateOps) *actionWorldEventsDispatcher {
	return &actionWorldEventsDispatcher{inner: inner, ops: ops}
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

func (d *actionWorldEventsDispatcher) OnQueueScript(scriptName string, username37 uint64) {
	d.inner.OnQueueScript(scriptName, username37)
	d.ops.QueueScript(scriptName, username37)
}
