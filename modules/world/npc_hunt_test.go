package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
)

// newConsumeHuntTargetFixture builds a Server + Npc ready to exercise
// consumeHuntTarget: s.huntTypes is populated with a single hunt config
// at slot 0, n.huntMode = 0, and n.huntTarget is left for the caller to
// set (a nil-target test should leave it nil).
//
// The returned hunt config has Type=HuntModeNpc, Rate=1, FindKeepHunting=true,
// FindNewMode=NPCModeNone (interaction-branch default). Tests mutate these
// fields in place to exercise specific branches.
func newConsumeHuntTargetFixture(t *testing.T) (*Server, *Npc, *objtype.HuntType) {
	t.Helper()
	s := newServerForScriptTest(t)
	n := newNpcForLifecycleTest(t)
	n.server = s
	hunt := &objtype.HuntType{
		ConfigType:      objtype.ConfigType{ID: 0},
		Type:            objtype.HuntModeNpc,
		Rate:            1,
		FindKeepHunting: true,
		FindNewMode:     objtype.NPCModeNone,
	}
	s.huntTypes = &objtype.HuntTypeConfigs{
		Configs: []*objtype.HuntType{hunt},
	}
	n.huntMode = 0
	return s, n, hunt
}

func TestConsumeHuntTargetNilHuntTargetNoOp(t *testing.T) {
	s, n, _ := newConsumeHuntTargetFixture(t)
	n.huntTarget = nil
	n.target = nil
	n.targetOp = 99
	n.huntClock = 7
	n.huntMode = 5

	s.consumeHuntTarget(n)

	if n.target != nil {
		t.Errorf("target: got %v, want nil (no-op expected)", n.target)
	}
	if n.targetOp != 99 {
		t.Errorf("targetOp: got %d, want 99 (unchanged)", n.targetOp)
	}
	if n.huntClock != 7 {
		t.Errorf("huntClock: got %d, want 7 (unchanged)", n.huntClock)
	}
	if n.huntMode != 5 {
		t.Errorf("huntMode: got %d, want 5 (unchanged)", n.huntMode)
	}
}

func TestConsumeHuntTargetHuntModeOffNoOp(t *testing.T) {
	s, n, hunt := newConsumeHuntTargetFixture(t)
	other := newNpcForLifecycleTest(t)
	n.huntTarget = other
	n.huntClock = 3
	hunt.Type = objtype.HuntModeOff

	s.consumeHuntTarget(n)

	if n.huntTarget != other {
		t.Errorf("huntTarget: got %v, want unchanged (HuntModeOff gate)", n.huntTarget)
	}
	if n.huntClock != 3 {
		t.Errorf("huntClock: got %d, want 3 (unchanged)", n.huntClock)
	}
}

func TestConsumeHuntTargetInvalidHuntModeNoOp(t *testing.T) {
	s, n, _ := newConsumeHuntTargetFixture(t)
	other := newNpcForLifecycleTest(t)

	// Case 1: huntMode = -1 (below lower bound).
	n.huntTarget = other
	n.huntMode = -1
	s.consumeHuntTarget(n)
	if n.huntTarget != other {
		t.Errorf("huntMode=-1: huntTarget should be unchanged, got %v", n.huntTarget)
	}

	// Case 2: huntMode = len(Configs) (above upper bound).
	n.huntTarget = other
	n.huntMode = len(s.huntTypes.Configs) // == 1
	s.consumeHuntTarget(n)
	if n.huntTarget != other {
		t.Errorf("huntMode=OOB: huntTarget should be unchanged, got %v", n.huntTarget)
	}
}
