package client

import "testing"

func TestOpCategories(t *testing.T) {
	cases := []struct {
		opcode   int
		name     string
		category int
	}{
		// CLIENT_EVENT
		{108, "NO_TIMEOUT", CategoryClientEvent},
		{70, "IDLE_TIMER", CategoryClientEvent},
		{189, "EVENT_CAMERA_POSITION", CategoryClientEvent},
		{7, "ANTICHEAT_OPLOGIC1", CategoryClientEvent},
		{233, "ANTICHEAT_CYCLELOGIC1", CategoryClientEvent},
		// RESTRICTED_EVENT
		{81, "EVENT_TRACKING", CategoryRestrictedEvent},
		{150, "REBUILD_GETMAPS", CategoryRestrictedEvent},
		// USER_EVENT
		{181, "MOVE_GAMECLICK", CategoryUserEvent},
		{93, "MOVE_OPCLICK", CategoryUserEvent},
		{165, "MOVE_MINIMAPCLICK", CategoryUserEvent},
		{4, "CLIENT_CHEAT", CategoryUserEvent},
		{140, "OPOBJ1", CategoryUserEvent},
		{194, "OPNPC1", CategoryUserEvent},
		{245, "OPLOC1", CategoryUserEvent},
		{164, "OPPLAYER1", CategoryUserEvent},
		{195, "OPHELD1", CategoryUserEvent},
		{31, "INV_BUTTON1", CategoryUserEvent},
		{155, "IF_BUTTON", CategoryUserEvent},
		{231, "CLOSE_MODAL", CategoryUserEvent},
		{158, "MESSAGE_PUBLIC", CategoryUserEvent},
	}
	for _, tc := range cases {
		op := Ops[tc.opcode]
		if op.Name != tc.name {
			t.Errorf("Ops[%d].Name = %q, want %q", tc.opcode, op.Name, tc.name)
		}
		if op.Category != tc.category {
			t.Errorf("Ops[%d] (%s): Category = %d, want %d", tc.opcode, tc.name, op.Category, tc.category)
		}
	}
}
