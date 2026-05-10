package script

import "testing"

// TestServerTriggerType_String pins the TS-faithful lowered enum name
// returned by (ServerTriggerType).String(). Mirrors TS
// LostCityRS/Engine-TS/.../ServerTriggerType.ts:166-170 toString
// (`ServerTriggerType[trigger].toLowerCase()`).
func TestServerTriggerType_String(t *testing.T) {
	cases := []struct {
		name string
		in   ServerTriggerType
		want string
	}{
		// Constant-prefix free of underscore.
		{"proc", TriggerProc, "proc"},
		{"label", TriggerLabel, "label"},
		{"debugproc", TriggerDebugProc, "debugproc"},

		// AP/OP families (1-5 + U + T).
		{"apnpc1", TriggerApNpc1, "apnpc1"},
		{"apnpcu", TriggerApNpcU, "apnpcu"},
		{"apnpct", TriggerApNpcT, "apnpct"},
		{"opnpc1", TriggerOpNpc1, "opnpc1"},
		{"opnpct", TriggerOpNpcT, "opnpct"},
		{"oploc1", TriggerOpLoc1, "oploc1"},
		{"oploct", TriggerOpLocT, "oploct"},
		{"opobj1", TriggerOpObj1, "opobj1"},
		{"opobjt", TriggerOpObjT, "opobjt"},
		{"opplayer1", TriggerOpPlayer1, "opplayer1"},
		{"opplayert", TriggerOpPlayerT, "opplayert"},
		{"opheld1", TriggerOpHeld1, "opheld1"},
		{"opheldt", TriggerOpHeldT, "opheldt"},

		// AI prefix retains underscore.
		{"ai_apnpc1", TriggerAiApNpc1, "ai_apnpc1"},
		{"ai_opnpc5", TriggerAiOpNpc5, "ai_opnpc5"},
		{"ai_aploc1", TriggerAiApLoc1, "ai_aploc1"},
		{"ai_opplayer1", TriggerAiOpPlayer1, "ai_opplayer1"},
		{"ai_queue4", TriggerAiQueue4, "ai_queue4"},
		{"ai_queue20", TriggerAiQueue20, "ai_queue20"},
		{"ai_timer", TriggerAiTimer, "ai_timer"},
		{"ai_walktrigger", TriggerAiWalkTrigger, "ai_walktrigger"},
		{"ai_spawn", TriggerAiSpawn, "ai_spawn"},
		{"ai_despawn", TriggerAiDespawn, "ai_despawn"},

		// Compound underscore retention.
		{"if_button", TriggerIfButton, "if_button"},
		{"if_close", TriggerIfClose, "if_close"},
		{"inv_button1", TriggerInvButton1, "inv_button1"},
		{"inv_buttond", TriggerInvButtonD, "inv_buttond"},

		// Single-token specials.
		{"queue", TriggerQueue, "queue"},
		{"softtimer", TriggerSoftTimer, "softtimer"},
		{"timer", TriggerTimer, "timer"},
		{"walktrigger", TriggerWalkTrigger, "walktrigger"},
		{"login", TriggerLogin, "login"},
		{"logout", TriggerLogout, "logout"},
		{"tutorial", TriggerTutorial, "tutorial"},
		{"advancestat", TriggerAdvanceStat, "advancestat"},
		{"mapzone", TriggerMapZone, "mapzone"},
		{"mapzoneexit", TriggerMapZoneExit, "mapzoneexit"},
		{"zone", TriggerZone, "zone"},
		{"zoneexit", TriggerZoneExit, "zoneexit"},
		{"changestat", TriggerChangeStat, "changestat"},

		// Unknown-value fallback.
		{"unknown_high", ServerTriggerType(9999), "trigger_9999"},
		{"gap_22", ServerTriggerType(22), "trigger_22"},
		{"unknown_negative", ServerTriggerType(-1), "trigger_-1"},
	}

	for _, c := range cases {
		got := c.in.String()
		if got != c.want {
			t.Errorf("%s: ServerTriggerType(%d).String() = %q, want %q", c.name, int(c.in), got, c.want)
		}
	}
}
