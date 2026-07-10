package world

import (
	"testing"

	io2 "github.com/zsrv/goscape/pkg/io/isaac"
	"github.com/zsrv/goscape/pkg/script"
)

// PERF-3: the terminal arms of the three resumeOrFinish* dispatchers must
// recycle the state's pooled stack buffers — observable as Release having
// nil'd the state's slices. Suspend paths must leave them intact.

func TestProcessWorldQueueReleasesTerminalState(t *testing.T) {
	s := newTestServer(t)
	state := newReturnImmediatelyScript(t)
	s.EnqueueWorldScript(state, 0) // stored as delay=1 per TS World.enqueueScript
	s.processWorldQueue()          // tick 1: delay gate, no fire
	s.processWorldQueue()          // tick 2: fires; script returns → Finished
	if state.Execution != script.Finished {
		t.Fatalf("precondition: Execution = %v, want Finished", state.Execution)
	}
	if state.IntStack != nil || state.StringStack != nil || state.Frames != nil {
		t.Error("terminal world-queue state still holds pooled buffers (Release not called in resumeOrFinishWorld)")
	}
}

func TestEnqueuedWorldScriptRetainsBuffersWhileWaiting(t *testing.T) {
	s := newTestServer(t)
	state := newReturnImmediatelyScript(t)
	s.EnqueueWorldScript(state, 3)
	s.processWorldQueue() // still waiting
	if state.IntStack == nil {
		t.Error("waiting world-queue state lost its buffers (premature Release)")
	}
}

// --- Player dispatcher (resumeOrFinish) ------------------------------------
//
// Construction mirrors TestResumeOrFinish_PreservesUnrelatedSuspendedScript
// (script_test.go): script.Init a fresh return-immediately ScriptFile bound
// to a *Player, wire the scripting fields resumeOrFinish's dispatch needs,
// then call resumeOrFinish directly to get a handle on the resulting state.

func TestResumeOrFinishReleasesTerminalState(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	sf := &script.ScriptFile{
		Name: "[fresh-terminal,test]",
		Opcodes: []script.Opcode{
			script.OpReturn,
		},
		IntOperands:      []int32{0},
		StringOperands:   []string{""},
		InstructionCount: 1,
	}
	state := script.Init(sf, p, false, nil, nil)
	state.Provider = s.scriptProvider
	state.World = s.worldVars
	state.Configs = s.configsView
	state.Inv = s.invLookup
	state.Npcs = s.npcLookup
	state.LineValidator = s.scriptLineValidator()

	s.resumeOrFinish(state, p)

	if state.Execution != script.Finished {
		t.Fatalf("precondition: Execution = %v, want Finished", state.Execution)
	}
	if state.IntStack != nil || state.StringStack != nil || state.Frames != nil {
		t.Error("terminal player-anchored state still holds pooled buffers (Release not called in resumeOrFinish)")
	}
}

// TestResumeOrFinishRetainsSuspendedBuffers forces Execution=Suspended
// before dispatch (script.Init always sets Running; overriding it before
// resumeOrFinish's script.Execute call short-circuits Execute's `for
// s.Execution == Running` loop — the same construction idiom used by
// TestResumeOrFinishNpcDefaultBranchClearsScript in npc_script_test.go).
// The Suspended/PauseButton/CountDialog arm stores the state on the player
// and must NOT release its buffers.
func TestResumeOrFinishRetainsSuspendedBuffers(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	sf := &script.ScriptFile{
		Name: "[suspend-buffers,test]",
		Opcodes: []script.Opcode{
			script.OpReturn,
		},
		IntOperands:      []int32{0},
		StringOperands:   []string{""},
		InstructionCount: 1,
	}
	state := script.Init(sf, p, false, nil, nil)
	state.Provider = s.scriptProvider
	state.World = s.worldVars
	state.Configs = s.configsView
	state.Inv = s.invLookup
	state.Npcs = s.npcLookup
	state.LineValidator = s.scriptLineValidator()
	state.Execution = script.Suspended // force suspend before dispatch

	s.resumeOrFinish(state, p)

	if p.activeScript != state {
		t.Fatalf("precondition: p.activeScript = %p, want %p (Suspended arm must store)", p.activeScript, state)
	}
	if state.IntStack == nil {
		t.Error("suspended player-anchored state lost its buffers (premature Release)")
	}
}

// --- NPC dispatcher (resumeOrFinishNpc) ------------------------------------
//
// Construction mirrors TestResumeOrFinishNpc_PreservesUnrelatedSuspendedScript
// (npc_script_test.go): buildNpcForIntegration for the server+npc fixture,
// buildNpcScriptState for a fresh return-immediately state, then call
// resumeOrFinishNpc directly to get a handle on the resulting state.

func TestResumeOrFinishNpcReleasesTerminalState(t *testing.T) {
	s, n := buildNpcForIntegration(t)

	sf := &script.ScriptFile{
		Name: "[fresh-npc-terminal,test]",
		Opcodes: []script.Opcode{
			script.OpReturn,
		},
		IntOperands:      []int32{0},
		StringOperands:   []string{""},
		InstructionCount: 1,
	}
	state := s.buildNpcScriptState(sf, n, nil, script.TriggerProc, nil, nil)

	s.resumeOrFinishNpc(state, n)

	if state.Execution != script.Finished {
		t.Fatalf("precondition: Execution = %v, want Finished", state.Execution)
	}
	if state.IntStack != nil || state.StringStack != nil || state.Frames != nil {
		t.Error("terminal npc-anchored state still holds pooled buffers (Release not called in resumeOrFinishNpc)")
	}
}

// TestResumeOrFinishNpcRetainsSuspendedBuffers forces Execution=NpcSuspended
// before dispatch (same script.Init-then-override idiom as
// TestResumeOrFinishNpcDefaultBranchClearsScript). The NpcSuspended arm
// stores the state on the npc and must NOT release its buffers.
func TestResumeOrFinishNpcRetainsSuspendedBuffers(t *testing.T) {
	s, n := buildNpcForIntegration(t)

	sf := &script.ScriptFile{
		Name:             "[npc-suspend-buffers,test]",
		Opcodes:          []script.Opcode{script.OpReturn},
		IntOperands:      []int32{0},
		StringOperands:   []string{""},
		InstructionCount: 1,
	}
	state := script.Init(sf, nil, false, nil, nil)
	state.ActiveNpc = n
	state.Execution = script.NpcSuspended // force suspend before dispatch

	s.resumeOrFinishNpc(state, n)

	if n.activeScript != state {
		t.Fatalf("precondition: n.activeScript = %p, want %p (NpcSuspended arm must store)", n.activeScript, state)
	}
	if state.IntStack == nil {
		t.Error("suspended npc-anchored state lost its buffers (premature Release)")
	}
}
