package world

import (
	"io"
	"log/slog"
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/script"
)

func newTestLogger(t *testing.T) *slog.Logger {
	t.Helper()
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func newNpcForScriptTest(t *testing.T) *Npc {
	t.Helper()
	typ := &objtype.NpcType{
		ConfigType: objtype.ConfigType{ID: 0, DebugName: "test_npc"},
	}
	return NewNpc(1, 0, 3094, 3106, 0, typ)
}

func TestNpcStoreAndClearActiveScript(t *testing.T) {
	n := newNpcForScriptTest(t)
	state := &script.ScriptState{}

	n.StoreActiveScript(state)
	if n.activeScript != state {
		t.Errorf("StoreActiveScript: got %v, want %v", n.activeScript, state)
	}

	n.ClearActiveScript()
	if n.activeScript != nil {
		t.Errorf("ClearActiveScript: got %v, want nil", n.activeScript)
	}
}

func TestNpcSetDelayed(t *testing.T) {
	n := newNpcForScriptTest(t)
	s := &Server{}
	s.currentTick = 100
	n.server = s

	n.SetDelayed(5)

	if !n.delayed {
		t.Errorf("delayed: got false, want true")
	}
	want := 100 + 1 + 5
	if n.delayedUntil != want {
		t.Errorf("delayedUntil: got %d, want %d", n.delayedUntil, want)
	}
}

// newServerForScriptTest builds a minimal *Server wired for running
// NPC-anchored scripts. Reuses the pattern from script_test.go:939-952.
func newServerForScriptTest(t *testing.T) *Server {
	t.Helper()
	return &Server{
		log: newTestLogger(t),
	}
}

func TestRunNpcScriptFiresAndFinishes(t *testing.T) {
	s := newServerForScriptTest(t)
	s.currentTick = 100
	n := newNpcForScriptTest(t)
	n.server = s

	sf := &script.ScriptFile{
		Name:    "trivial_return",
		Opcodes: []script.Opcode{script.OpReturn},
	}

	s.runNpcScript(sf, n, nil, nil)

	if n.activeScript != nil {
		t.Errorf("activeScript: got %v, want nil (script finished)", n.activeScript)
	}
	if n.delayed {
		t.Errorf("delayed: got true, want false")
	}
}

func TestRunNpcScriptSuspendsOnNpcDelay(t *testing.T) {
	s := newServerForScriptTest(t)
	s.currentTick = 100
	n := newNpcForScriptTest(t)
	n.server = s

	sf := &script.ScriptFile{
		Name:        "npc_delay_3_return",
		Opcodes:     []script.Opcode{script.OpPushConstantInt, script.OpNpcDelay, script.OpReturn},
		IntOperands: []int32{3},
	}

	s.runNpcScript(sf, n, nil, nil)

	if n.activeScript == nil {
		t.Fatalf("activeScript: got nil, want stored state")
	}
	if n.activeScript.Execution != script.NpcSuspended {
		t.Errorf("Execution: got %v, want NpcSuspended", n.activeScript.Execution)
	}
	if !n.delayed {
		t.Errorf("delayed: got false, want true")
	}
	want := 100 + 1 + 3
	if n.delayedUntil != want {
		t.Errorf("delayedUntil: got %d, want %d", n.delayedUntil, want)
	}
}
