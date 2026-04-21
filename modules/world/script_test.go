package world

import (
	"testing"
	"time"

	"github.com/zsrv/goscape/pkg/inventory"
	io2 "github.com/zsrv/goscape/pkg/io/isaac"
	"github.com/zsrv/goscape/pkg/io/packet"
	"github.com/zsrv/goscape/pkg/objtype"
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
	s.scriptProvider.RegisterAt(0xAAAA, buildGreetScript(0xAAAA, "g"))

	p, cc := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	s.playerLoop = append(s.playerLoop, p)

	received := drainConn(t, cc)
	p.EnqueueScriptTyped(0xAAAA, 1, 0, script.QueueNormal)

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
	s.scriptProvider.RegisterAt(0xBBBB, buildGreetScript(0xBBBB, "g"))

	p, cc := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	s.playerLoop = append(s.playerLoop, p)

	received := drainConn(t, cc)
	p.EnqueueScriptTyped(0xBBBB, 0, 0, script.QueueNormal)
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

func TestPlaytimeViaScriptMessageGame(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	p, cc := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	p.playtime = 42

	// Script:
	//   pushstr "n="
	//   timespent       (pushes 42)
	//   append_num      (pops 42 + "n=", pushes "n=42")
	//   mes             (sends "n=42" on the wire)
	//   return
	sf := &script.ScriptFile{
		Name: "[timespent,test]",
		Opcodes: []script.Opcode{
			script.OpPushConstantString,
			script.OpTimeSpent,
			script.OpAppendNum,
			script.OpMes,
			script.OpReturn,
		},
		IntOperands:      []int32{0, 0, 0, 0, 0},
		StringOperands:   []string{"n=", "", "", "", ""},
		InstructionCount: 5,
	}

	received := drainConn(t, cc)
	s.runScript(sf, p, false, nil, nil)
	p.client.flushWrite()
	got := <-received

	// Wire = opcode(1) + len(1) + PJStrLF("n=42") = 1+1+5 = 7 bytes.
	if len(got) != 7 {
		t.Fatalf("wire: got %d bytes, want 7", len(got))
	}
	if string(got[2:6]) != "n=42" || got[6] != 0x0a {
		t.Errorf("payload: got %q, want 'n=42\\n'", got[2:])
	}
}

func TestQueueMultipleEntriesPreservesOrder(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	s.scriptProvider.RegisterAt(0xCCC1, buildGreetScript(0xCCC1, "1"))
	s.scriptProvider.RegisterAt(0xCCC2, buildGreetScript(0xCCC2, "2"))

	p, cc := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	s.playerLoop = append(s.playerLoop, p)

	received := drainConn(t, cc)
	p.EnqueueScriptTyped(0xCCC1, 0, 0, script.QueueNormal)
	p.EnqueueScriptTyped(0xCCC2, 0, 0, script.QueueNormal)
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

// seedVarpTypes installs a minimal VarpTypeConfigs on s with a single
// varp (id 0, debugname "test", transmit as given) so player_varp.go
// wire logic has a config to consult.
func seedVarpTypes(s *Server, transmit bool) {
	t0 := objtype.NewVarPlayerType(0)
	t0.DebugName = "test"
	t0.Transmit = transmit
	s.varpTypes = &objtype.VarpTypeConfigs{
		ConfigNames: map[string]int{"test": 0},
		Configs:     []*objtype.VarPlayerType{t0},
	}
}

// popVarpScript builds: push_constant_int N, pop_varp 0, return.
func popVarpScript(value int32) *script.ScriptFile {
	return &script.ScriptFile{
		Name: "[popvarp,test]",
		Opcodes: []script.Opcode{
			script.OpPushConstantInt,
			script.OpPopVarp,
			script.OpReturn,
		},
		IntOperands:      []int32{value, 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
}

func TestVarpWireSyncSmall(t *testing.T) {
	s := newTestServer(t)
	seedVarpTypes(s, true)
	p, cc := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	p.varps = make([]int32, 1)

	received := drainConn(t, cc)
	s.runScript(popVarpScript(42), p, false, nil, nil)
	p.client.flushWrite()
	got := <-received

	// VARP_SMALL wire = opcode(1) + P2(id=0)(2) + P1(val=42)(1) = 4 bytes.
	if len(got) != 4 {
		t.Fatalf("VARP_SMALL wire: got %d bytes, want 4", len(got))
	}
	if got[1] != 0 || got[2] != 0 {
		t.Errorf("varp id bytes: got %v, want [0 0]", got[1:3])
	}
	if got[3] != 42 {
		t.Errorf("varp value byte: got %d, want 42", got[3])
	}
	if p.varps[0] != 42 {
		t.Errorf("server varps[0]: got %d, want 42", p.varps[0])
	}
}

func TestVarpWireSyncLarge(t *testing.T) {
	s := newTestServer(t)
	seedVarpTypes(s, true)
	p, cc := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	p.varps = make([]int32, 1)

	received := drainConn(t, cc)
	s.runScript(popVarpScript(10000), p, false, nil, nil)
	p.client.flushWrite()
	got := <-received

	// VARP_LARGE wire = opcode(1) + P2(id=0)(2) + P4(val=10000)(4) = 7 bytes.
	if len(got) != 7 {
		t.Fatalf("VARP_LARGE wire: got %d bytes, want 7", len(got))
	}
	if got[1] != 0 || got[2] != 0 {
		t.Errorf("varp id bytes: got %v, want [0 0]", got[1:3])
	}
	want := []byte{0x00, 0x00, 0x27, 0x10}
	for i, b := range want {
		if got[3+i] != b {
			t.Errorf("varp value byte %d: got %#x, want %#x", i, got[3+i], b)
		}
	}
}

func TestVarpTransmitFalseNoWire(t *testing.T) {
	s := newTestServer(t)
	seedVarpTypes(s, false)
	p, cc := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	p.varps = make([]int32, 1)

	received := drainConn(t, cc)
	s.runScript(popVarpScript(42), p, false, nil, nil)
	p.client.flushWrite()
	got := <-received

	if len(got) != 0 {
		t.Errorf("transmit=false varp: got %d wire bytes, want 0", len(got))
	}
	if p.varps[0] != 42 {
		t.Errorf("server varps[0]: got %d, want 42 (server-side write must still happen)", p.varps[0])
	}
}

func TestTelejumpViaScript(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	// Packed coord: (level=0, x=3222, z=3222) -> (0<<28) | (3222<<14) | 3222.
	packed := int32((0 << 28) | (3222 << 14) | 3222)

	// Script:
	//   push_constant_int <packed_coord>
	//   p_telejump
	//   return
	sf := &script.ScriptFile{
		Name: "[telejump,test]",
		Opcodes: []script.Opcode{
			script.OpPushConstantInt,
			script.OpPTeleJump,
			script.OpReturn,
		},
		IntOperands:      []int32{packed, 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}

	s.runScript(sf, p, true, nil, nil)

	if p.x != 3222 {
		t.Errorf("p.x: got %d, want 3222", p.x)
	}
	if p.z != 3222 {
		t.Errorf("p.z: got %d, want 3222", p.z)
	}
	if p.level != 0 {
		t.Errorf("p.level: got %d, want 0", p.level)
	}
	if p.tele == false {
		t.Error("p.tele: got false, want true")
	}
	if p.jump == false {
		t.Error("p.jump: got false, want true")
	}
}

func TestStatAdvanceViaScript(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	p.stats[3] = 100

	// Script:
	//   push_constant_int 3    (stat id)
	//   push_constant_int 50   (xp to add)
	//   stat_advance
	//   return
	sf := &script.ScriptFile{
		Name: "[stat_advance,test]",
		Opcodes: []script.Opcode{
			script.OpPushConstantInt,
			script.OpPushConstantInt,
			script.OpStatAdvance,
			script.OpReturn,
		},
		IntOperands:      []int32{3, 50, 0, 0},
		StringOperands:   []string{"", "", "", ""},
		InstructionCount: 4,
	}

	s.runScript(sf, p, true, nil, nil)

	if int(p.stats[3]) != 150 {
		t.Errorf("p.stats[3]: got %d, want 150", int(p.stats[3]))
	}
}

func TestOcNameViaScript(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()

	// Seed a fake ObjType at id 995 named "Coins". Override whatever
	// the real cache loaded.
	s.objTypes = &objtype.ObjTypeConfigs{
		ConfigNames: map[string]int{"coins": 995},
		Configs:     make([]*objtype.ObjType, 996),
	}
	s.objTypes.Configs[995] = &objtype.ObjType{
		ConfigType: objtype.ConfigType{ID: 995, DebugName: "coins"},
		Name:       "Coins",
	}
	s.configsView = serverConfigsView{s: s}

	p, _ := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	// Script: push_constant_int 995, oc_name, return
	// After OC_NAME pops the id and pushes the name string, we need
	// to verify the string stack got "Coins".
	sf := &script.ScriptFile{
		Name: "[ocname,test]",
		Opcodes: []script.Opcode{
			script.OpPushConstantInt,
			script.OpOcName,
			script.OpReturn,
		},
		IntOperands:      []int32{995, 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}

	// runScript owns the state; we can't pop from it. Instead, inline
	// Init+Execute so we can inspect the string stack afterwards.
	state := script.Init(sf, p, false, nil, nil)
	state.Provider = s.scriptProvider
	state.World = s.worldVars
	state.Configs = s.configsView
	if err := script.Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := state.PopString(); got != "Coins" {
		t.Errorf("OC_NAME(995): got %q, want %q", got, "Coins")
	}
}

// TestInvAddGrantsItemsViaScript is the S5e end-to-end pipeline test:
// handler -> InvLookup -> *Player downcast -> inventory.Add. It runs a
// 4-instruction inv_add(main_inv, 995, 42) script against a freshly
// allocated main inventory and asserts GetItemCount(995) == 42.
func TestInvAddGrantsItemsViaScript(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()

	// Seed a synthetic InvType at id 0 ("inv") with StackAll=true so a
	// count of 42 collapses into a single slot regardless of ObjType
	// stackability (INV_ADD consults inv.StackType only). Size matches
	// the real main_inv capacity.
	mainID := 0
	s.invTypes = &objtype.InvTypeConfigs{
		ConfigNames: map[string]int{"inv": mainID},
		Configs:     make([]*objtype.InvType, 1),
		Inv:         mainID,
		Worn:        -1,
	}
	s.invTypes.Configs[mainID] = &objtype.InvType{
		ConfigType: objtype.ConfigType{ID: mainID, DebugName: "inv"},
		Scope:      objtype.InvTypeScopeTemp,
		Size:       28,
		StackAll:   true,
	}
	s.invLookup = invLookupView{s: s}

	p, _ := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	invType := s.invTypes.Configs[mainID]
	if invType == nil {
		t.Fatalf("InvType %d not loaded", mainID)
	}
	if p.invs == nil {
		p.invs = make(map[int]*inventory.Inventory)
	}
	p.invs[mainID] = inventory.FromType(invType)

	// Script: push mainID, push 995 (coins), push 42, inv_add, return.
	sf := &script.ScriptFile{
		Name: "[invadd,test]",
		Opcodes: []script.Opcode{
			script.OpPushConstantInt,
			script.OpPushConstantInt,
			script.OpPushConstantInt,
			script.OpInvAdd,
			script.OpReturn,
		},
		IntOperands:      []int32{int32(mainID), 995, 42, 0, 0},
		StringOperands:   []string{"", "", "", "", ""},
		InstructionCount: 5,
	}

	s.runScript(sf, p, true, nil, nil)

	inv := p.invs[mainID]
	if inv == nil {
		t.Fatal("main inv not present after runScript")
	}
	if got := inv.GetItemCount(995); got != 42 {
		t.Errorf("inv.GetItemCount(995): got %d, want 42", got)
	}
}

func TestIfOpenMainSetsModalState(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	s.configsView = serverConfigsView{s: s}
	s.invLookup = invLookupView{s: s}
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	sf := &script.ScriptFile{
		Name: "[ifopen,test]",
		Opcodes: []script.Opcode{
			script.OpPushConstantInt,
			script.OpIfOpenMain,
			script.OpReturn,
		},
		IntOperands:      []int32{1234, 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	s.runScript(sf, p, true, nil, nil)

	if p.modalMain != 1234 {
		t.Errorf("modalMain: got %d, want 1234", p.modalMain)
	}
	if p.modalState&modalStateMain == 0 {
		t.Error("modalState: main bit not set")
	}
	if !p.refreshModal {
		t.Error("refreshModal: want true")
	}
}

func TestIfSetTextEmitsWire(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	s.configsView = serverConfigsView{s: s}
	s.invLookup = invLookupView{s: s}
	p, cc := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	// Script: push "hi" (string), push 7 (com), if_settext, return
	// TS pop order: text = popString(), com = popInt() — so push
	// string first then int, making each the top of its respective stack.
	sf := &script.ScriptFile{
		Name: "[ifsettext,test]",
		Opcodes: []script.Opcode{
			script.OpPushConstantString,
			script.OpPushConstantInt,
			script.OpIfSetText,
			script.OpReturn,
		},
		IntOperands:      []int32{0, 7, 0, 0},
		StringOperands:   []string{"hi", "", "", ""},
		InstructionCount: 4,
	}

	received := drainConn(t, cc)
	s.runScript(sf, p, true, nil, nil)
	p.client.flushWrite()
	got := <-received

	// OpIfSetText has PayloadSize -2 (2-byte length prefix).
	// Wire = opcode(1) + len2(2) + P2(com=7)(2) + PJStrLF("hi")(3) = 8 bytes.
	if len(got) != 8 {
		t.Fatalf("wire: got %d bytes, want 8", len(got))
	}
	// got[1..2] = 2-byte payload length (= 5)
	// got[3..4] = P2(7)
	// got[5..6] = "hi"
	// got[7]    = 0x0a
	if got[3] != 0 || got[4] != 7 {
		t.Errorf("com: got %d, want 7", (int(got[3])<<8)|int(got[4]))
	}
	if string(got[5:7]) != "hi" || got[7] != 0x0a {
		t.Errorf("text: got %q", got[5:])
	}
}

func TestPauseButtonResumesAfterClick(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	s.configsView = serverConfigsView{s: s}
	s.invLookup = invLookupView{s: s}
	p, cc := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	p.resumeButtons = [5]int{7, 0, 0, 0, 0}

	// Script: push "before", mes, p_pausebutton, push "after", mes, return
	sf := &script.ScriptFile{
		Name: "[pausebutton,test]",
		Opcodes: []script.Opcode{
			script.OpPushConstantString,
			script.OpMes,
			script.OpPPauseButton,
			script.OpPushConstantString,
			script.OpMes,
			script.OpReturn,
		},
		IntOperands:      []int32{0, 0, 0, 0, 0, 0},
		StringOperands:   []string{"before", "", "", "after", "", ""},
		InstructionCount: 6,
	}

	received := drainConn(t, cc)
	s.runScript(sf, p, true, nil, nil)
	p.client.flushWrite()
	first := <-received

	if p.activeScript == nil {
		t.Fatal("expected activeScript to be set after p_pausebutton")
	}
	if p.activeScript.Execution != script.PauseButton {
		t.Errorf("Execution: got %v, want PauseButton", p.activeScript.Execution)
	}

	// Simulate RESUME_PAUSEBUTTON with com=7.
	received2 := drainConn(t, cc)
	buf := packet.NewPacket([]byte{0, 7})
	if err := s.handleResumePauseButton(p, buf); err != nil {
		t.Fatalf("resume: %v", err)
	}
	p.client.flushWrite()
	second := <-received2

	// first payload bytes 2..7 = "before" then 0x0a (newline)
	if string(first[2:8]) != "before" {
		t.Errorf("first payload: got %q", first[2:])
	}
	if string(second[2:7]) != "after" {
		t.Errorf("second payload: got %q", second[2:])
	}
	if p.activeScript != nil {
		t.Errorf("activeScript after resume-and-finish: got %v, want nil", p.activeScript)
	}
}

// TestStrongQueueFiresWhileDelayed verifies STRONG-tagged queue entries
// fire through processPlayerQueue even when p.delayed=true. This gates
// the STRONG queue variant introduced in sub-spec S5h.
func TestStrongQueueFiresWhileDelayed(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	s.scriptProvider.RegisterAt(0xBEEF, buildGreetScript(0xBEEF, "s"))
	s.configsView = serverConfigsView{s: s}
	s.invLookup = invLookupView{s: s}

	p, cc := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	s.playerLoop = append(s.playerLoop, p)

	// Force the player into a busy (delayed) state.
	p.delayed = true
	p.delayedUntil = s.currentTick + 99

	received := drainConn(t, cc)

	// Enqueue a STRONG script with delay=0 — should fire even though delayed.
	p.EnqueueScriptTyped(0xBEEF, 0, 0, script.QueueStrong)
	s.processActiveScripts()
	p.client.flushWrite()
	got := <-received

	if len(got) != 4 {
		t.Fatalf("STRONG fire: got %d bytes, want 4", len(got))
	}
}

// TestSetTimerFiresAfterInterval verifies a timer set with interval 5
// does not fire in ticks 0..4 and fires on tick 5.
func TestSetTimerFiresAfterInterval(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	s.scriptProvider.RegisterAt(0xA1, buildGreetScript(0xA1, "t"))
	s.configsView = serverConfigsView{s: s}
	s.invLookup = invLookupView{s: s}

	p, cc := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	s.playerLoop = append(s.playerLoop, p)

	// Register a timer at interval=5, starting at current tick 0.
	p.SetTimer(0xA1, 5, 0, script.TimerNormal)

	received := drainConn(t, cc)

	// Tick 0..4: no fire.
	for i := 0; i < 5; i++ {
		s.processPlayerTimers()
		s.currentTick++
	}
	// Now currentTick = 5, timer fires.
	s.processPlayerTimers()
	p.client.flushWrite()
	got := <-received
	if len(got) != 4 {
		t.Fatalf("fire at interval: got %d bytes, want 4", len(got))
	}
	if string(got[2:3]) != "t" {
		t.Errorf("payload: got %q, want 't'", got[2:])
	}
}

// TestSoftTimerFiresWhileDelayed verifies SOFT-typed timers fire
// regardless of p.delayed.
func TestSoftTimerFiresWhileDelayed(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	s.scriptProvider.RegisterAt(0xB2, buildGreetScript(0xB2, "s"))
	s.configsView = serverConfigsView{s: s}
	s.invLookup = invLookupView{s: s}

	p, cc := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	s.playerLoop = append(s.playerLoop, p)
	p.delayed = true
	p.delayedUntil = s.currentTick + 99

	p.SetTimer(0xB2, 1, 0, script.TimerSoft)

	received := drainConn(t, cc)
	s.currentTick = 1
	s.processPlayerTimers()
	p.client.flushWrite()
	got := <-received
	if len(got) != 4 {
		t.Errorf("Soft timer while delayed: got %d bytes, want 4 (fire)", len(got))
	}
}

// TestClearTimerStopsFiring verifies clearing a timer removes it from
// the registration map so it does not fire.
func TestClearTimerStopsFiring(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	s.scriptProvider.RegisterAt(0xC3, buildGreetScript(0xC3, "x"))
	s.configsView = serverConfigsView{s: s}
	s.invLookup = invLookupView{s: s}

	p, cc := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	s.playerLoop = append(s.playerLoop, p)

	p.SetTimer(0xC3, 1, 0, script.TimerNormal)
	p.ClearTimer(0xC3)

	received := drainConn(t, cc)
	for i := 0; i < 10; i++ {
		s.currentTick++
		s.processPlayerTimers()
	}
	p.client.flushWrite()

	select {
	case got := <-received:
		if len(got) > 0 {
			t.Errorf("cleared timer fired: got %d bytes, want 0", len(got))
		}
	case <-time.After(50 * time.Millisecond):
		// expected
	}
}

// TestNpcNameViaScript drives the full NPC_NAME -> MES pipeline against a
// real *Npc. Builds an NpcType at id 7 named "Hans", binds it as the
// active NPC on a ScriptState, runs NPC_NAME + MES + RETURN, and asserts
// "Hans\n" reaches the client wire.
func TestNpcNameViaScript(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	s.configsView = serverConfigsView{s: s}
	s.invLookup = invLookupView{s: s}

	// Seed an NpcType at id 7 named "Hans".
	s.npcTypes = &objtype.NPCTypeConfigs{
		Configs: make([]*objtype.NpcType, 8),
	}
	s.npcTypes.Configs[7] = &objtype.NpcType{
		ConfigType: objtype.ConfigType{ID: 7, DebugName: "hans"},
		Name:       "Hans",
	}

	p, cc := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	// Build an Npc instance of type 7.
	npc := NewNpc(0 /* nid */, 7 /* typeId */, 3222, 3222, 0, s.npcTypes.Configs[7])

	// Script: npc_name -> mes -> return.
	sf := &script.ScriptFile{
		Name: "[npcname,test]",
		Opcodes: []script.Opcode{
			script.OpNpcName,
			script.OpMes,
			script.OpReturn,
		},
		IntOperands:      []int32{0, 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}

	received := drainConn(t, cc)

	// Inline runScript steps so we can set ActiveNpc.
	state := script.Init(sf, p, false, nil, nil)
	state.Provider = s.scriptProvider
	state.World = s.worldVars
	state.Configs = s.configsView
	state.Inv = s.invLookup
	state.ActiveNpc = npc
	if err := script.Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	p.client.flushWrite()
	got := <-received

	// Wire = opcode(1) + len(1) + PJStrLF("Hans") = 7 bytes.
	if len(got) != 7 {
		t.Fatalf("wire: got %d bytes, want 7", len(got))
	}
	if string(got[2:6]) != "Hans" || got[6] != 0x0a {
		t.Errorf("payload: got %q, want 'Hans\\n'", got[2:])
	}
}

// TestNormalQueueWaitsForIdle verifies NORMAL-tagged queue entries do
// NOT fire while the player is delayed — they wait for idle.
func TestNormalQueueWaitsForIdle(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	s.scriptProvider.RegisterAt(0xBEE2, buildGreetScript(0xBEE2, "n"))
	s.configsView = serverConfigsView{s: s}
	s.invLookup = invLookupView{s: s}

	p, cc := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	s.playerLoop = append(s.playerLoop, p)

	p.delayed = true
	p.delayedUntil = s.currentTick + 99

	received := drainConn(t, cc)
	p.EnqueueScriptTyped(0xBEE2, 0, 0, script.QueueNormal)
	s.processActiveScripts()
	p.client.flushWrite()

	// Expect nothing fired — read with timeout.
	select {
	case got := <-received:
		if len(got) > 0 {
			t.Errorf("NORMAL: got %d bytes fired while delayed; want 0", len(got))
		}
	case <-time.After(50 * time.Millisecond):
		// expected: nothing fired
	}
}
