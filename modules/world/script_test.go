package world

import (
	"testing"

	io2 "github.com/zsrv/goscape/pkg/io/isaac"
	"github.com/zsrv/goscape/pkg/script"
)

// buildLoginScript returns a synthetic ScriptFile equivalent to:
//   mes "hi"
//   return
func buildLoginScript() *script.ScriptFile {
	return &script.ScriptFile{
		Name:             "[login,test]",
		LookupKey:        uint32(script.TriggerLogin), // global key
		Opcodes:          []script.Opcode{script.OpPushConstantString, script.OpMes, script.OpReturn},
		IntOperands:      []int32{0, 0, 0},
		StringOperands:   []string{"hi", "", ""},
		InstructionCount: 3,
	}
}

// testProviderWithLogin returns a Provider with exactly one LOGIN script.
func testProviderWithLogin(t *testing.T) *script.Provider {
	t.Helper()
	p := script.NewProvider()
	p.Register(buildLoginScript())
	return p
}

func TestRunScriptNilNoop(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = nil
	p, cc := newTestPlayer(t)
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	received := drainConn(t, cc)
	s.runScript(nil, p, true, nil, nil)
	p.client.flushWrite()
	got := <-received
	if len(got) != 0 {
		t.Errorf("nil script should produce 0 bytes; got %d", len(got))
	}
}

func TestRunScriptExecutesMesScript(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = testProviderWithLogin(t)
	p, cc := newTestPlayer(t)
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	sf := s.scriptProvider.GetByTrigger(script.TriggerLogin, -1, -1)
	if sf == nil {
		t.Fatal("provider did not return login script via global lookup")
	}

	received := drainConn(t, cc)
	s.runScript(sf, p, true, nil, nil)
	p.client.flushWrite()
	got := <-received

	// Expect OpMessageGame (opcode 4, -1) with payload PJStrLF("hi") = 3 bytes.
	// Wire = opcode(1) + len(1) + payload(3) = 5 bytes.
	if len(got) != 5 {
		t.Errorf("got %d bytes, want 5 (opcode + len prefix + 'hi\\n')", len(got))
	}
	// Payload bytes 2..4 should be 'h','i',0x0a.
	if string(got[2:4]) != "hi" || got[4] != 0x0a {
		t.Errorf("payload: got %v, want 'hi\\n'", got[2:])
	}
}

func TestRunScriptHandlesError(t *testing.T) {
	s := newTestServer(t)
	p, cc := newTestPlayer(t)
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	// Script with an unknown opcode. Execute returns error; runScript must
	// not panic, and must not write anything.
	bad := &script.ScriptFile{
		Name:             "bad",
		Opcodes:          []script.Opcode{9999},
		IntOperands:      []int32{0},
		StringOperands:   []string{""},
		InstructionCount: 1,
	}

	received := drainConn(t, cc)
	s.runScript(bad, p, true, nil, nil)
	p.client.flushWrite()
	got := <-received
	if len(got) != 0 {
		t.Errorf("bad script should produce 0 bytes; got %d", len(got))
	}
}

// buildDelayScript returns a synthetic ScriptFile equivalent to:
//
//	mes "before"
//	p_delay 1
//	mes "after"
//	return
func buildDelayScript() *script.ScriptFile {
	return &script.ScriptFile{
		Name: "[delay,test]",
		Opcodes: []script.Opcode{
			script.OpPushConstantString, // push "before"
			script.OpMes,
			script.OpPushConstantInt, // push 1
			script.OpPDelay,
			script.OpPushConstantString, // push "after"
			script.OpMes,
			script.OpReturn,
		},
		IntOperands:      []int32{0, 0, 1, 0, 0, 0, 0},
		StringOperands:   []string{"before", "", "", "", "after", "", ""},
		InstructionCount: 7,
	}
}

// buildGreetScript is a tiny queued-target script that emits a single
// one-character message.
func buildGreetScript(key uint32, ch string) *script.ScriptFile {
	return &script.ScriptFile{
		Name:             "[greet,test]",
		LookupKey:        key,
		Opcodes:          []script.Opcode{script.OpPushConstantString, script.OpMes, script.OpReturn},
		IntOperands:      []int32{0, 0, 0},
		StringOperands:   []string{ch, "", ""},
		InstructionCount: 3,
	}
}

func TestPDelayStoresActiveScript(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	startTick := s.currentTick

	sf := buildDelayScript()
	s.runScript(sf, p, true, nil, nil)

	if p.activeScript == nil {
		t.Fatal("activeScript: got nil, want non-nil")
	}
	if p.activeScript.Execution != script.Suspended {
		t.Errorf("Execution: got %v, want Suspended", p.activeScript.Execution)
	}
	if !p.delayed {
		t.Error("delayed: got false, want true")
	}
	if p.delayedUntil != startTick+2 {
		t.Errorf("delayedUntil: got %d, want %d", p.delayedUntil, startTick+2)
	}
}

func TestResumeAfterDelayExpires(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	s.playerLoop = append(s.playerLoop, p)

	s.runScript(buildDelayScript(), p, true, nil, nil)
	if p.activeScript == nil {
		t.Fatal("precondition: activeScript should be non-nil after suspension")
	}

	s.currentTick += 3
	s.processActiveScripts()

	if p.activeScript != nil {
		t.Errorf("activeScript after expiry: got %v, want nil", p.activeScript)
	}
	if p.delayed {
		t.Error("delayed after expiry: got true, want false")
	}
}

func TestResumedScriptEmitsMessageGame(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	p, cc := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	s.playerLoop = append(s.playerLoop, p)

	received := drainConn(t, cc)
	s.runScript(buildDelayScript(), p, true, nil, nil)
	p.client.flushWrite()
	first := <-received

	// First packet = PJStrLF("before") payload.
	// Wire = opcode(1) + len(1) + payload(7) = 9 bytes.
	if len(first) != 9 {
		t.Fatalf("first packet: got %d bytes, want 9", len(first))
	}
	if string(first[2:8]) != "before" || first[8] != 0x0a {
		t.Errorf("first payload: got %q, want 'before\\n'", first[2:])
	}

	// Advance ticks and resume. Should emit the "after" MessageGame.
	received2 := drainConn(t, cc)
	s.currentTick += 3
	s.processActiveScripts()
	p.client.flushWrite()
	second := <-received2

	// Wire = opcode(1) + len(1) + PJStrLF("after") = 1+1+6 = 8 bytes.
	if len(second) != 8 {
		t.Fatalf("second packet: got %d bytes, want 8", len(second))
	}
	if string(second[2:7]) != "after" || second[7] != 0x0a {
		t.Errorf("second payload: got %q, want 'after\\n'", second[2:])
	}
}

func TestQueueFiresAtDelayExpiry(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	s.scriptProvider.Register(buildGreetScript(0xAAAA, "g"))

	p, cc := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	s.playerLoop = append(s.playerLoop, p)

	received := drainConn(t, cc)
	p.EnqueueScript(0xAAAA, 1, 0)

	// Pre-decrement semantics: delay 1 -> 0, 0 <= 0 fires immediately.
	s.processActiveScripts()
	p.client.flushWrite()
	got := <-received

	// "g\n" wire = opcode(1) + len(1) + payload(2) = 4 bytes.
	if len(got) != 4 {
		t.Fatalf("queue fire: got %d bytes, want 4", len(got))
	}
	if string(got[2:3]) != "g" || got[3] != 0x0a {
		t.Errorf("queue payload: got %q, want 'g\\n'", got[2:])
	}
	if len(p.queue) != 0 {
		t.Errorf("queue after fire: len=%d, want 0", len(p.queue))
	}
}

func TestQueueZeroDelayFiresSameTick(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	s.scriptProvider.Register(buildGreetScript(0xBBBB, "g"))

	p, cc := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	s.playerLoop = append(s.playerLoop, p)

	received := drainConn(t, cc)
	p.EnqueueScript(0xBBBB, 0, 0)
	s.processActiveScripts()
	p.client.flushWrite()
	got := <-received

	if len(got) != 4 {
		t.Fatalf("zero-delay fire: got %d bytes, want 4", len(got))
	}
	if len(p.queue) != 0 {
		t.Errorf("queue not drained: len=%d, want 0", len(p.queue))
	}
}

func TestQueueMultipleEntriesPreservesOrder(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	s.scriptProvider.Register(buildGreetScript(0xCCC1, "1"))
	s.scriptProvider.Register(buildGreetScript(0xCCC2, "2"))

	p, cc := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	s.playerLoop = append(s.playerLoop, p)

	received := drainConn(t, cc)
	p.EnqueueScript(0xCCC1, 0, 0)
	p.EnqueueScript(0xCCC2, 0, 0)
	s.processActiveScripts()
	p.client.flushWrite()
	got := <-received

	// Each packet = 4 bytes. Both may coalesce into one Read.
	switch len(got) {
	case 8:
		if got[2] != '1' || got[6] != '2' {
			t.Errorf("coalesced order: got %c,%c; want 1,2", got[2], got[6])
		}
	case 4:
		got2 := <-received
		if len(got2) != 4 {
			t.Fatalf("second packet: got %d, want 4", len(got2))
		}
		if got[2] != '1' || got2[2] != '2' {
			t.Errorf("order: got %c,%c; want 1,2", got[2], got2[2])
		}
	default:
		t.Fatalf("unexpected packet length: %d", len(got))
	}
}
