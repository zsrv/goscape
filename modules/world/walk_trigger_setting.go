package world

// WalkTriggerSetting selects how walktriggers are dispatched relative to
// the per-tick interaction loop. Mirrors TS WalkTriggerSetting.ts.
//
// PLAYERPACKET (default): walktriggers fire from the move-click packet
// handler (handleMoveGameClick → processWalktrigger). The per-tick
// fallback path (TS World.ts:635-641) is skipped.
//
// PLAYERSETUP: walktriggers fire from the per-tick fallback when
// !opcalled && hasWaypoints; handler-side dispatch is skipped.
//
// PLAYERMOVEMENT: handler-side dispatch is skipped; per-tick fallback
// re-paths userPath each tick but does NOT fire walktriggers.
type WalkTriggerSetting int

const (
	WalkTriggerSettingPlayerpacket   WalkTriggerSetting = 0
	WalkTriggerSettingPlayersetup    WalkTriggerSetting = 1
	WalkTriggerSettingPlayermovement WalkTriggerSetting = 2
)
