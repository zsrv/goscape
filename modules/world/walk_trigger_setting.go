package world

// WalkTriggerSetting selects how walktriggers are dispatched relative to
// the per-tick interaction loop. Mirrors TS WalkTriggerSetting.ts.
//
// rev-254 (Engine-TS f0ccbe8a): ALL engine consumers were removed upstream —
// the MoveClick handler now queues waypoints + fires processWalktrigger
// directly and World.ts dropped the NODE_WALKTRIGGER_SETTING fallback
// blocks. TS keeps WalkTriggerSetting.ts + the Environment.NODE_WALKTRIGGER_-
// SETTING definition (only the .env.example line was removed), so goscape
// keeps the type + config field for parity; the setting is currently inert.
type WalkTriggerSetting int

const (
	WalkTriggerSettingPlayerpacket   WalkTriggerSetting = 0
	WalkTriggerSettingPlayersetup    WalkTriggerSetting = 1
	WalkTriggerSettingPlayermovement WalkTriggerSetting = 2
)
