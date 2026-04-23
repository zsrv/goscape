package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
)

func TestCheckOpTrigger(t *testing.T) {
	ops := []struct {
		name string
		op   int
		want bool
	}{
		{"OpPlayer1", objtype.NPCModeOpPlayer1, true},
		{"OpPlayer5", objtype.NPCModeOpPlayer5, true},
		{"OpLoc1", objtype.NPCModeOpLoc1, true},
		{"OpLoc5", objtype.NPCModeOpLoc5, true},
		{"OpObj1", objtype.NPCModeOpObj1, true},
		{"OpObj5", objtype.NPCModeOpObj5, true},
		{"OpNpc1", objtype.NPCModeOpNpc1, true},
		{"OpNpc5", objtype.NPCModeOpNpc5, true},
		{"ApPlayer1 — NOT op", objtype.NPCModeApPlayer1, false},
		{"ApNpc5 — NOT op", objtype.NPCModeApNpc5, false},
		{"PatrolMode", objtype.NPCModePatrol, false},
		{"NoneMode", objtype.NPCModeNone, false},
		{"Queue1", objtype.NPCModeQueue1, false},
		{"Null", objtype.NPCModeNull, false},
	}
	for _, tc := range ops {
		t.Run(tc.name, func(t *testing.T) {
			if got := checkOpTrigger(tc.op); got != tc.want {
				t.Errorf("checkOpTrigger(%d) = %t, want %t", tc.op, got, tc.want)
			}
		})
	}
}

func TestCheckApTrigger(t *testing.T) {
	ops := []struct {
		name string
		op   int
		want bool
	}{
		{"ApPlayer1", objtype.NPCModeApPlayer1, true},
		{"ApPlayer5", objtype.NPCModeApPlayer5, true},
		{"ApLoc1", objtype.NPCModeApLoc1, true},
		{"ApLoc5", objtype.NPCModeApLoc5, true},
		{"ApObj1", objtype.NPCModeApObj1, true},
		{"ApObj5", objtype.NPCModeApObj5, true},
		{"ApNpc1", objtype.NPCModeApNpc1, true},
		{"ApNpc5", objtype.NPCModeApNpc5, true},
		{"OpPlayer1 — NOT ap", objtype.NPCModeOpPlayer1, false},
		{"OpNpc5 — NOT ap", objtype.NPCModeOpNpc5, false},
		{"PatrolMode", objtype.NPCModePatrol, false},
		{"Queue20", objtype.NPCModeQueue20, false},
	}
	for _, tc := range ops {
		t.Run(tc.name, func(t *testing.T) {
			if got := checkApTrigger(tc.op); got != tc.want {
				t.Errorf("checkApTrigger(%d) = %t, want %t", tc.op, got, tc.want)
			}
		})
	}
}
