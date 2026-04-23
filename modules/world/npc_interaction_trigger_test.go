package world

import (
	"fmt"
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/script"
)

type triggerMapCase struct {
	op   int
	want script.ServerTriggerType
}

func runTriggerMapTest(t *testing.T, name string, fn func(int) script.ServerTriggerType, cases []triggerMapCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(fmt.Sprintf("%s/op=%d", name, tc.op), func(t *testing.T) {
			if got := fn(tc.op); got != tc.want {
				t.Errorf("%s(%d) = %d, want %d", name, tc.op, got, tc.want)
			}
		})
	}
}

func TestAiApNpcTriggerForOp(t *testing.T) {
	cases := []triggerMapCase{
		{objtype.NPCModeApNpc1, script.TriggerAiApNpc1},
		{objtype.NPCModeApNpc2, script.TriggerAiApNpc2},
		{objtype.NPCModeApNpc3, script.TriggerAiApNpc3},
		{objtype.NPCModeApNpc4, script.TriggerAiApNpc4},
		{objtype.NPCModeApNpc5, script.TriggerAiApNpc5},
		{objtype.NPCModeApNpc5 + 1, 0}, // out-of-range high
		{objtype.NPCModeApNpc1 - 1, 0}, // out-of-range low
		{objtype.NPCModeApPlayer1, 0},  // wrong category
	}
	runTriggerMapTest(t, "aiApNpcTriggerForOp", aiApNpcTriggerForOp, cases)
}

func TestAiOpNpcTriggerForOp(t *testing.T) {
	cases := []triggerMapCase{
		{objtype.NPCModeOpNpc1, script.TriggerAiOpNpc1},
		{objtype.NPCModeOpNpc5, script.TriggerAiOpNpc5},
		{objtype.NPCModeOpNpc5 + 1, 0},
		{objtype.NPCModeApNpc1, 0},
	}
	runTriggerMapTest(t, "aiOpNpcTriggerForOp", aiOpNpcTriggerForOp, cases)
}

func TestAiApPlayerTriggerForOp(t *testing.T) {
	cases := []triggerMapCase{
		{objtype.NPCModeApPlayer1, script.TriggerAiApPlayer1},
		{objtype.NPCModeApPlayer5, script.TriggerAiApPlayer5},
		{objtype.NPCModeApPlayer5 + 1, 0},
		{objtype.NPCModeOpPlayer1, 0},
	}
	runTriggerMapTest(t, "aiApPlayerTriggerForOp", aiApPlayerTriggerForOp, cases)
}

func TestAiOpPlayerTriggerForOp(t *testing.T) {
	cases := []triggerMapCase{
		{objtype.NPCModeOpPlayer1, script.TriggerAiOpPlayer1},
		{objtype.NPCModeOpPlayer5, script.TriggerAiOpPlayer5},
		{objtype.NPCModeOpPlayer5 + 1, 0},
	}
	runTriggerMapTest(t, "aiOpPlayerTriggerForOp", aiOpPlayerTriggerForOp, cases)
}

func TestAiApLocTriggerForOp(t *testing.T) {
	cases := []triggerMapCase{
		{objtype.NPCModeApLoc1, script.TriggerAiApLoc1},
		{objtype.NPCModeApLoc5, script.TriggerAiApLoc5},
		{objtype.NPCModeApLoc5 + 1, 0},
	}
	runTriggerMapTest(t, "aiApLocTriggerForOp", aiApLocTriggerForOp, cases)
}

func TestAiOpLocTriggerForOp(t *testing.T) {
	cases := []triggerMapCase{
		{objtype.NPCModeOpLoc1, script.TriggerAiOpLoc1},
		{objtype.NPCModeOpLoc5, script.TriggerAiOpLoc5},
		{objtype.NPCModeOpLoc5 + 1, 0},
	}
	runTriggerMapTest(t, "aiOpLocTriggerForOp", aiOpLocTriggerForOp, cases)
}

func TestAiApObjTriggerForOp(t *testing.T) {
	cases := []triggerMapCase{
		{objtype.NPCModeApObj1, script.TriggerAiApObj1},
		{objtype.NPCModeApObj5, script.TriggerAiApObj5},
		{objtype.NPCModeApObj5 + 1, 0},
	}
	runTriggerMapTest(t, "aiApObjTriggerForOp", aiApObjTriggerForOp, cases)
}

func TestAiOpObjTriggerForOp(t *testing.T) {
	cases := []triggerMapCase{
		{objtype.NPCModeOpObj1, script.TriggerAiOpObj1},
		{objtype.NPCModeOpObj5, script.TriggerAiOpObj5},
		{objtype.NPCModeOpObj5 + 1, 0},
	}
	runTriggerMapTest(t, "aiOpObjTriggerForOp", aiOpObjTriggerForOp, cases)
}
