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

	// Expect OpMessageGame (opcode 4, -1) with payload PJStrNUL("hi") = 3 bytes.
	// Wire = opcode(1) + len(1) + payload(3) = 5 bytes.
	if len(got) != 5 {
		t.Errorf("got %d bytes, want 5 (opcode + len prefix + 'hi\\0')", len(got))
	}
	// Payload bytes 2..4 should be 'h','i',0x00.
	if string(got[2:4]) != "hi" || got[4] != 0x00 {
		t.Errorf("payload: got %v, want 'hi\\0'", got[2:])
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
