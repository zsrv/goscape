package world

import (
	"bytes"
	"slices"
	"testing"
	"time"

	entitypkg "github.com/zsrv/goscape/pkg/entity"
	"github.com/zsrv/goscape/pkg/inventory"
	io2 "github.com/zsrv/goscape/pkg/io/isaac"
	"github.com/zsrv/goscape/pkg/io/packet"
	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/rsbuf"
	"github.com/zsrv/goscape/pkg/script"
)

// buildLoginScript returns a synthetic ScriptFile equivalent to:
//
//	mes "hi"
//	return
func buildLoginScript() *script.ScriptFile {
	return &script.ScriptFile{
		Name:             "[login,test]",
		LookupKey:        script.LookupKeyForGlobal(script.TriggerLogin), // global key
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
	s.runScript(nil, p, nil, true, nil, nil)
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
	s.runScript(sf, p, nil, true, nil, nil)
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
	s.runScript(bad, p, nil, true, nil, nil)
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
	s.runScript(sf, p, nil, true, nil, nil)

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

	s.runScript(buildDelayScript(), p, nil, true, nil, nil)
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
	s.runScript(buildDelayScript(), p, nil, true, nil, nil)
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
	p.EnqueueScriptArgs(0xAAAA, 1, nil, nil, script.QueueNormal)

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
	p.EnqueueScriptArgs(0xBBBB, 0, nil, nil, script.QueueNormal)
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
	s.runScript(sf, p, nil, false, nil, nil)
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
	p.EnqueueScriptArgs(0xCCC1, 0, nil, nil, script.QueueNormal)
	p.EnqueueScriptArgs(0xCCC2, 0, nil, nil, script.QueueNormal)
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
	s.runScript(popVarpScript(42), p, nil, false, nil, nil)
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
	s.runScript(popVarpScript(10000), p, nil, false, nil, nil)
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
	s.runScript(popVarpScript(42), p, nil, false, nil, nil)
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

	s.runScript(sf, p, nil, true, nil, nil)

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

	s.runScript(sf, p, nil, true, nil, nil)

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
	s.npcLookup = serverNpcLookup{s: s}

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

	s.runScript(sf, p, nil, true, nil, nil)

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
	s.npcLookup = serverNpcLookup{s: s}
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
	s.runScript(sf, p, nil, true, nil, nil)

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
	s.npcLookup = serverNpcLookup{s: s}
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
	s.runScript(sf, p, nil, true, nil, nil)
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
	s.npcLookup = serverNpcLookup{s: s}
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
	s.runScript(sf, p, nil, true, nil, nil)
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

// TestResumePauseButtonResumesEvenWithEmptyResumeButtons pins TS fidelity
// per ResumePauseButtonHandler.ts:7-14 — RESUME_PAUSEBUTTON resumes any
// PauseButton-suspended script regardless of payload value AND regardless
// of the resumeButtons array contents. This unblocks chatnpc dialogs,
// which never call if_setresumebuttons (chat.rs2:303-311).
func TestResumePauseButtonResumesEvenWithEmptyResumeButtons(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	s.configsView = serverConfigsView{s: s}
	s.invLookup = invLookupView{s: s}
	s.npcLookup = serverNpcLookup{s: s}
	p, cc := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	// resumeButtons LEFT AT ZERO VALUE (all zeros); payload com value
	// (9999) is ALSO not in resumeButtons. Both must be ignored under
	// TS fidelity.

	sf := &script.ScriptFile{
		Name: "[pausebutton,empty_resumebuttons]",
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
	s.runScript(sf, p, nil, true, nil, nil)
	p.client.flushWrite()
	<-received

	if p.activeScript == nil || p.activeScript.Execution != script.PauseButton {
		t.Fatalf("setup: expected PauseButton state, got activeScript=%v", p.activeScript)
	}

	// Send RESUME_PAUSEBUTTON with com=9999 (NOT in p.resumeButtons,
	// which is the zero-value [5]int{0,0,0,0,0}). TS-fidelity handler
	// ignores the payload and resumes anyway.
	received2 := drainConn(t, cc)
	buf := packet.NewPacket([]byte{0x27, 0x0F}) // 9999 = 0x270F
	if err := s.handleResumePauseButton(p, buf); err != nil {
		t.Fatalf("resume: %v", err)
	}
	p.client.flushWrite()
	second := <-received2

	if string(second[2:7]) != "after" {
		t.Errorf("post-resume payload: got %q, want \"after\"", second[2:])
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
	s.npcLookup = serverNpcLookup{s: s}

	p, cc := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	s.playerLoop = append(s.playerLoop, p)

	// Force the player into a busy (delayed) state.
	p.delayed = true
	p.delayedUntil = s.currentTick + 99

	received := drainConn(t, cc)

	// Enqueue a STRONG script with delay=0 — should fire even though delayed.
	p.EnqueueScriptArgs(0xBEEF, 0, nil, nil, script.QueueStrong)
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
	s.npcLookup = serverNpcLookup{s: s}

	p, cc := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	s.playerLoop = append(s.playerLoop, p)

	// Register a timer at interval=5, starting at current tick 0.
	p.SetTimer(0xA1, 5, nil, nil, script.TimerNormal)

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
	s.npcLookup = serverNpcLookup{s: s}

	p, cc := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	s.playerLoop = append(s.playerLoop, p)
	p.delayed = true
	p.delayedUntil = s.currentTick + 99

	p.SetTimer(0xB2, 1, nil, nil, script.TimerSoft)

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
	s.npcLookup = serverNpcLookup{s: s}

	p, cc := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	s.playerLoop = append(s.playerLoop, p)

	p.SetTimer(0xC3, 1, nil, nil, script.TimerNormal)
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
	s.npcLookup = serverNpcLookup{s: s}

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
	state.Npcs = s.npcLookup
	state.LineValidator = s.scriptLineValidator()
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
	s.npcLookup = serverNpcLookup{s: s}

	p, cc := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	s.playerLoop = append(s.playerLoop, p)

	p.delayed = true
	p.delayedUntil = s.currentTick + 99

	received := drainConn(t, cc)
	p.EnqueueScriptArgs(0xBEE2, 0, nil, nil, script.QueueNormal)
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

func TestOpNpc1FiresScriptAndEmitsSay(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()

	// Register [opnpc1, type=7] = push "cluck cluck" + NPC_SAY + RETURN.
	key := script.LookupKeyForType(script.TriggerOpNpc1, 7)
	s.scriptProvider.Register(&script.ScriptFile{
		Name:             "[opnpc1,chicken]",
		LookupKey:        key,
		Opcodes:          []script.Opcode{script.OpPushConstantString, script.OpNpcSay, script.OpReturn},
		IntOperands:      []int32{0, 0, 0},
		StringOperands:   []string{"cluck cluck", "", ""},
		InstructionCount: 3,
	})

	p, _ := newTestPlayer(t)
	p.client.server = s

	// Place an NPC of type 7 adjacent to the player so reach is immediate.
	npcType := &objtype.NpcType{
		ConfigType: objtype.ConfigType{ID: 7, DebugName: "chicken"},
		Op:         []string{"Talk-to", "", "", "", ""},
	}
	npc := NewNpc(0, 7, p.x+1, p.z, p.level, npcType)
	// s.npcs is a fixed-size array; slot 0 is always valid.
	s.npcs[0] = npc

	// Wire rsbuf so HasNpc(p.slot, nid=0) returns true.
	p.slot = 1
	s.players[1] = p
	rsbufSeesNpc(t, s, 1, 0)

	// Build the OPNPC1 payload (p2(slot=0)) and fire it through the handler.
	payload := []byte{0x00, 0x00}
	if err := handleOpNpc1(p, payload); err != nil {
		t.Fatalf("handleOpNpc1: %v", err)
	}
	if p.target != npc {
		t.Fatalf("post-click: target=%v, want npc", p.target)
	}

	// Drive one processInteraction tick — player is already adjacent, so
	// reach succeeds immediately and tryFireOpTrigger runs.
	p.processInteraction()

	if string(npc.sayText) != "cluck cluck" {
		t.Errorf("sayText: got %q, want 'cluck cluck'", npc.sayText)
	}
	if npc.masks&rsbuf.NpcMaskSay == 0 {
		t.Error("NpcMaskSay bit: not set on npc.masks")
	}
	if p.target != nil {
		t.Error("target: expected cleared after script Finished")
	}
	// NAI-44 T6 cascade: interactionFired is cleared by the post-fire auto-clear
	// (TS L1261-1263: interacted && !apRangeCalled → ClearInteraction). Dispatch
	// is proven by sayText + NpcMaskSay above; the interactionFired flag check
	// is dropped as it's a transient in-tick signal cleared by auto-clear.
}

func TestOpNpc1FiresScriptAndEmitsAnimPlusSay(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	s.seqTypes = buildSeqTypes(50) // NPC_ANIM opcode calls n.Animate(42, 3); needs seqTypes.Count() > 42

	// [opnpc1, type=7]: push seq=42 + push delay=3 + NPC_ANIM +
	//                   push "cluck" + NPC_SAY + RETURN.
	key := script.LookupKeyForType(script.TriggerOpNpc1, 7)
	s.scriptProvider.Register(&script.ScriptFile{
		Name:      "[opnpc1,chicken]",
		LookupKey: key,
		Opcodes: []script.Opcode{
			script.OpPushConstantInt,    // seq
			script.OpPushConstantInt,    // delay
			script.OpNpcAnim,            // consume (seq, delay)
			script.OpPushConstantString, // "cluck"
			script.OpNpcSay,             // consume string
			script.OpReturn,
		},
		IntOperands:      []int32{42, 3, 0, 0, 0, 0},
		StringOperands:   []string{"", "", "", "cluck", "", ""},
		InstructionCount: 6,
	})

	p, _ := newTestPlayer(t)
	p.client.server = s

	npcType := &objtype.NpcType{
		ConfigType: objtype.ConfigType{ID: 7, DebugName: "chicken"},
		Op:         []string{"Talk-to", "", "", "", ""},
	}
	npc := NewNpc(0, 7, p.x+1, p.z, p.level, npcType)
	npc.server = s // wire server so Animate gate can reach s.seqTypes
	s.npcs[0] = npc

	// Wire rsbuf so HasNpc(p.slot, nid=0) returns true.
	p.slot = 1
	s.players[1] = p
	rsbufSeesNpc(t, s, 1, 0)

	// Fire OPNPC1 click.
	payload := []byte{0x00, 0x00}
	if err := handleOpNpc1(p, payload); err != nil {
		t.Fatalf("handleOpNpc1: %v", err)
	}

	// Drive one tick — player is already adjacent, reach succeeds,
	// tryFireOpTrigger dispatches the compound script.
	p.processInteraction()

	if npc.animID != 42 {
		t.Errorf("animID: got %d, want 42", npc.animID)
	}
	if npc.animDelay != 3 {
		t.Errorf("animDelay: got %d, want 3", npc.animDelay)
	}
	if string(npc.sayText) != "cluck" {
		t.Errorf("sayText: got %q, want 'cluck'", npc.sayText)
	}
	if npc.masks&rsbuf.NpcMaskAnim == 0 {
		t.Error("NpcMaskAnim bit: not set — compound mask writes may be broken")
	}
	if npc.masks&rsbuf.NpcMaskSay == 0 {
		t.Error("NpcMaskSay bit: not set — compound mask writes may be broken")
	}
	if p.target != nil {
		t.Error("target: expected cleared after script Finished")
	}
	// NAI-44 T6 cascade: interactionFired is cleared by the post-fire auto-clear
	// (TS L1261-1263: interacted && !apRangeCalled → ClearInteraction). Dispatch
	// is proven by animID/animDelay/sayText + NpcMaskAnim/NpcMaskSay above.
}

// --- NAI-37 Task 10: player-path WorldSuspended producer test --------------

// TestResumeOrFinish_WorldSuspended_EnqueuesAndPreservesActiveScript pins
// the player-path producer: a player-bound script whose Execute
// returned Execution=WorldSuspended (with the wakeup-tick on the int
// stack) is dispatched by resumeOrFinish to (a) pop the wakeup-tick,
// (b) enqueue to s.worldScriptQueue with that delay, and (c) PRESERVE
// the player's active script pointer. Mirrors TS Player.ts:2143-2150
// (only Finished/Aborted nulls activeScript; WorldSuspended does not).
//
// NAI-44 T1 closed NAI-37-D-WORLDSUSPEND-CLEARS-ACTIVE-SCRIPT: the
// previous defensive ClearActiveScript() call has been deleted; the
// pointer is retained and the resume loop is guarded by
// Execution==Suspended only (tick.go:213-214), so holding the pointer
// across the WorldSuspended transition is safe.
//
// The test constructs the post-Execute ScriptState directly (skipping
// script.Execute) so it isolates the resumeOrFinish branch under test:
// we want to verify the WorldSuspended dispatch arm, not the bytecode
// path that produces it (which is covered by pkg/script tests).
//
// Note: resumeOrFinish itself calls script.Execute first. To bypass it
// for this isolated test, we use a single-instruction RETURN script —
// Execute runs it as a no-op (Execution stays WorldSuspended only if
// we set it AFTER Execute returns). Instead we build a script whose
// bytecode sets WorldSuspended via the WORLD_DELAY opcode, then drive
// resumeOrFinish through it end-to-end. This matches how the real
// dispatch path works in production.
func TestResumeOrFinish_WorldSuspended_EnqueuesAndPreservesActiveScript(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	// Script: push 5, world_delay, return.
	// WORLD_DELAY pops the int and sets Execution=WorldSuspended,
	// leaving the popped value... actually per pkg/script/handlers_server.go
	// WORLD_DELAY does NOT pop (the wakeup-tick stays on the stack for
	// the producer to consume). So the producer (resumeOrFinish) is
	// what pops it.
	sf := &script.ScriptFile{
		Name: "[worlddelay,test]",
		Opcodes: []script.Opcode{
			script.OpPushConstantInt,
			script.OpWorldDelay,
			script.OpReturn,
		},
		IntOperands:      []int32{5, 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}

	// Init + drive through resumeOrFinish (which itself runs Execute).
	state := script.Init(sf, p, true, nil, nil)
	state.Provider = s.scriptProvider
	state.World = s.worldVars
	state.Configs = s.configsView
	state.Inv = s.invLookup
	state.Npcs = s.npcLookup
	state.LineValidator = s.scriptLineValidator()

	// Pre-set activeScript so we can verify it gets cleared. (In
	// production this would be set by StoreActiveScript on a prior
	// suspension; here we wire it directly so the assertion is
	// meaningful.)
	p.activeScript = state

	s.resumeOrFinish(state, p)

	if got, want := len(s.worldScriptQueue), 1; got != want {
		t.Fatalf("worldScriptQueue length: got %d, want %d", got, want)
	}
	if got := s.worldScriptQueue[0].delay; got != 6 {
		t.Errorf("enqueued delay: got %d, want 6 (popped 5 from script stack, stored as 5+1=6 per TS World.enqueueScript)", got)
	}
	if got := s.worldScriptQueue[0].script; got != state {
		t.Errorf("enqueued script identity: got %p, want %p", got, state)
	}
	// NAI-44 T1 cascade: post-T1 the WorldSuspended arm no longer calls
	// ClearActiveScript(). TS Player.ts:2143-2150 only nulls activeScript on
	// FINISHED/ABORTED; holding the pointer is safe (processActiveScripts
	// gates resume on Execution==Suspended only; tick.go:213-214).
	if got := p.activeScript; got != state {
		t.Errorf("player.activeScript: got %p, want %p (WorldSuspended must NOT clear)", got, state)
	}
}

// TestProcessPlayerQueueDeliversAllArgs validates the NAI-26 Bundle 1
// plumbing under realistic queue-fire conditions: a queue request
// carrying IntArgs=[100, 200] is fired through processPlayerQueue and
// the target script runs (proven by the queue draining + the script's
// wire-output landing). The integration test confirms that the
// parallel-slice plumbing reaches runScript.
//
// Spec § Bundle 2 § "Integration test".
func TestProcessPlayerQueueDeliversAllArgs(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	// Register a 2-int-arg script that emits a fixed "ok\n" mes —
	// confirms execution; the args themselves are validated by the
	// pkg/script-level TestQueueOpcode + TestStrongQueue* tests.
	s.scriptProvider.RegisterAt(0xD1D1, buildGreetScript(0xD1D1, "k"))

	p, cc := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	s.playerLoop = append(s.playerLoop, p)

	received := drainConn(t, cc)
	// Enqueue with 2 int args via the new parallel-slice signature.
	if err := p.EnqueueScriptArgs(0xD1D1, 0, []int{100, 200}, nil, script.QueueNormal); err != nil {
		t.Fatalf("EnqueueScriptArgs: %v", err)
	}
	// Verify the queue entry carries both args (Bundle 1 plumbing pin).
	if len(p.queue) != 1 {
		t.Fatalf("queue len after enqueue: got %d, want 1", len(p.queue))
	}
	if !slices.Equal(p.queue[0].IntArgs, []int{100, 200}) {
		t.Errorf("queue[0].IntArgs: got %v, want [100 200]", p.queue[0].IntArgs)
	}
	if p.queue[0].StringArgs != nil {
		t.Errorf("queue[0].StringArgs: got %v, want nil", p.queue[0].StringArgs)
	}

	s.processActiveScripts()
	p.client.flushWrite()
	got := <-received

	// Drain confirms the script fired through the parallel-slice path.
	if len(got) != 4 {
		t.Fatalf("queue fire: got %d bytes, want 4", len(got))
	}
	if len(p.queue) != 0 {
		t.Errorf("queue after fire: len=%d, want 0", len(p.queue))
	}
}

// --- NAI-39 Task 3: buildPlayerScriptState target-dispatch tests ----------
//
// Direct mirror of buildNpcScriptState target-dispatch coverage at
// npc_script_test.go:472-560. Verifies the rails work even though no
// production producer fires through them yet — closes the dual-pin
// (presence-of-rails) per ts_asymmetry_dual_pin.md.

// TestBuildPlayerScriptState_NilTarget — nil target leaves Self2 nil and
// PtrActivePlayer2 unset; only the primary PtrActivePlayer is set.
func TestBuildPlayerScriptState_NilTarget(t *testing.T) {
	s := newTestServer(t)
	p, _ := newTestPlayer(t)
	sf := &script.ScriptFile{Name: "noop"}

	state := s.buildPlayerScriptState(sf, p, nil, false, nil, nil)

	if state.Self2 != nil {
		t.Error("Self2: non-nil, want nil")
	}
	if state.Pointers&script.PtrActivePlayer2 != 0 {
		t.Error("Pointers: PtrActivePlayer2 flag set, want unset")
	}
	if state.Pointers&script.PtrActivePlayer == 0 {
		t.Error("Pointers: PtrActivePlayer flag unset, want set (primary)")
	}
}

// TestBuildPlayerScriptState_PlayerTarget — *Player target lands in
// state.Self2 with PtrActivePlayer2 set; Self (primary) is unchanged.
// Mirrors TS ScriptRunner.init: self=Player, target=Player → _activePlayer2.
func TestBuildPlayerScriptState_PlayerTarget(t *testing.T) {
	s := newTestServer(t)
	p, _ := newTestPlayer(t)
	p2, _ := newTestPlayer(t)
	sf := &script.ScriptFile{Name: "noop"}

	state := s.buildPlayerScriptState(sf, p, p2, false, nil, nil)

	if state.Self == nil {
		t.Error("Self: nil, want primary set")
	}
	if state.Self2 == nil {
		t.Error("Self2: nil, want set (ActivePlayer target)")
	}
	if state.Pointers&script.PtrActivePlayer2 == 0 {
		t.Error("Pointers: PtrActivePlayer2 flag unset, want set")
	}
	if state.Self2 != p2 {
		t.Errorf("Self2: got %v, want %v (target Player)", state.Self2, p2)
	}
}

// TestBuildPlayerScriptState_NpcTarget — *Npc target lands in
// state.ActiveNpc with PtrActiveNpc set.
func TestBuildPlayerScriptState_NpcTarget(t *testing.T) {
	s := newTestServer(t)
	p, _ := newTestPlayer(t)
	npc := newNpcForScriptTest(t)
	sf := &script.ScriptFile{Name: "noop"}

	state := s.buildPlayerScriptState(sf, p, npc, false, nil, nil)

	if state.ActiveNpc == nil {
		t.Error("ActiveNpc: nil, want set")
	}
	if state.Pointers&script.PtrActiveNpc == 0 {
		t.Error("Pointers: PtrActiveNpc flag unset, want set")
	}
}

// TestBuildPlayerScriptState_LocTarget — *Loc target lands in
// state.ActiveLoc with PtrActiveLoc set.
func TestBuildPlayerScriptState_LocTarget(t *testing.T) {
	s := newTestServer(t)
	p, _ := newTestPlayer(t)
	loc := entitypkg.NewLoc(0, 100, 100, 1, 1, entitypkg.LifecycleRespawn, 42, 10, 0)
	sf := &script.ScriptFile{Name: "noop"}

	state := s.buildPlayerScriptState(sf, p, loc, false, nil, nil)

	if state.ActiveLoc == nil {
		t.Error("ActiveLoc: nil, want set")
	}
	if state.Pointers&script.PtrActiveLoc == 0 {
		t.Error("Pointers: PtrActiveLoc flag unset, want set")
	}
}

// TestBuildPlayerScriptState_ObjTarget — *Obj target lands in
// state.ActiveObj with PtrActiveObj set.
func TestBuildPlayerScriptState_ObjTarget(t *testing.T) {
	s := newTestServer(t)
	p, _ := newTestPlayer(t)
	obj := entitypkg.NewObj(0, 100, 100, entitypkg.LifecycleRespawn, 42, 1)
	sf := &script.ScriptFile{Name: "noop"}

	state := s.buildPlayerScriptState(sf, p, obj, false, nil, nil)

	if state.ActiveObj == nil {
		t.Error("ActiveObj: nil, want set")
	}
	if state.Pointers&script.PtrActiveObj == 0 {
		t.Error("Pointers: PtrActiveObj flag unset, want set")
	}
}

// TestOpPlayer1_E2E_HintPlOnClicker — full path: simulate an OPPLAYER1
// client packet → handleOpPlayer1 sets interaction → tryFireOpTrigger
// fires fireOpTriggerPlayer → runScript routes through
// buildPlayerScriptState's case-ActivePlayer arm → script runs with
// Self=clicker, Self2=target → HINT_PL emits to clicker's outbound
// (TS Player.ts:1129 + ScriptRunner.ts:84-87 binding; NAI-70).
//
// Closes NAI-39-D-ACTIVEPLAYER2-NO-OPPLAYER-PRODUCER by adding
// handler-entry coverage on top of the direct fire-helper pin in
// TestFireOpTriggerPlayer_BindsSelf2ToTarget.
//
// Approach: Option A — drive handleOpPlayer1 with an OPPLAYER1 payload,
// then mark clicker.interacted = true (the gate processInteraction
// would set on adjacency) and call tryFireOpTrigger directly. This keeps
// the test free of the movement/path-finding machinery while still
// exercising the full handler→trigger→script→wire pipeline.
func TestOpPlayer1_E2E_HintPlOnClicker(t *testing.T) {
	s, clicker, target, clickerConn := makeOpPlayerFixture(t)
	rsbufSeesPlayer(t, s, clicker.slot, target.slot)

	// Compute expected first wire byte using a parallel encryptor seeded
	// identically to clicker.client.encryptor (set by makeOpPlayerFixture).
	wantEnc, _ := isaacPair([4]uint32{1, 2, 3, 4})

	s.scriptProvider = script.NewProvider()
	s.scriptProvider.Register(buildOpPlayerHintPlScript(script.TriggerOpPlayer1))

	// Drive the OPPLAYER1 wire packet through the handler.
	if err := handleOpPlayer1(clicker, p2Payload(target.slot)); err != nil {
		t.Fatalf("handleOpPlayer1: %v", err)
	}
	if clicker.target != target {
		t.Fatalf("post-handler: clicker.target = %v, want %p (target)", clicker.target, target)
	}
	if clicker.targetOp != 1 {
		t.Fatalf("post-handler: clicker.targetOp = %d, want 1", clicker.targetOp)
	}

	// Simulate processInteraction's adjacency gate (the bit
	// tryFireOpTrigger reads).
	clicker.interacted = true

	received := drainConn(t, clickerConn)
	tryFireOpTrigger(clicker)
	clicker.client.flushWrite()
	got := <-received

	want := []byte{
		byte((int(gameserver.OpHintArrow.Opcode) + int(wantEnc.GetNext())) & 0xff),
		0x0A,                                      // p1: type = 10 (player hint)
		byte(target.slot >> 8), byte(target.slot), // p2: slot (target's)
		0x00, 0x00, // p2: 0
		0x00, // p1: 0
	}
	if !bytes.Equal(got, want) {
		t.Errorf("HINT_ARROW wire bytes: got %#x, want %#x", got, want)
	}
	if !clicker.interactionFired {
		t.Error("interactionFired: got false, want true after fire")
	}
}

// TestResumeOrFinishWorldSuspendedDoesNotClearActiveScript pins NAI-44 T1:
// when a player-anchored script transitions to WorldSuspended, the
// player's activeScript slot retains the state pointer (TS Player.ts:2143-2150
// only nulls activeScript on FINISHED/ABORTED).
func TestResumeOrFinishWorldSuspendedDoesNotClearActiveScript(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	sf := &script.ScriptFile{
		Name: "[worlddelay,nai44t1]",
		Opcodes: []script.Opcode{
			script.OpPushConstantInt,
			script.OpWorldDelay,
			script.OpReturn,
		},
		IntOperands:      []int32{5, 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}

	state := script.Init(sf, p, true, nil, nil)
	state.Provider = s.scriptProvider
	state.World = s.worldVars
	state.Configs = s.configsView
	state.Inv = s.invLookup
	state.Npcs = s.npcLookup
	state.LineValidator = s.scriptLineValidator()

	// Pre-condition: store the state so the assertion is meaningful.
	p.activeScript = state

	s.resumeOrFinish(state, p)

	if p.activeScript != state {
		t.Errorf("activeScript: got %p, want %p (WorldSuspended must NOT clear)", p.activeScript, state)
	}
	if len(s.worldScriptQueue) != 1 {
		t.Errorf("worldScriptQueue length: got %d, want 1 (state should have been enqueued)", len(s.worldScriptQueue))
	}
}

// TestSuspendedThenWorldSuspendedNoDoubleFire — NAI-44 T2 R5 regression.
// Pre-NAI-44, the defensive ClearActiveScript at the WorldSuspended arm
// guarded against double-fire if the same state pointer was held by both
// the player slot and the world queue. NAI-44 T1 deletes that clear,
// leaving the player slot pointing at a WorldSuspended state.
//
// Verify the gating logic in processActiveScripts still prevents double-fire:
// a state with Execution == WorldSuspended in the player's activeScript slot
// is NOT re-fired by processActiveScripts, which gates on Execution ==
// Suspended only (tick.go:213-214). A re-fire would reset Execution to
// Running and change it; we pin that Execution stays WorldSuspended.
func TestSuspendedThenWorldSuspendedNoDoubleFire(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	// Build a synthetic WorldSuspended state wired to the player.
	state := &script.ScriptState{
		Script:      &script.ScriptFile{Name: "[worlddelay,nai44t2r5]", Opcodes: []script.Opcode{}},
		Execution:   script.WorldSuspended,
		IntStack:    make([]int, script.StackCapacity),
		StringStack: make([]string, script.StackCapacity),
		Self:        p,
	}

	// Simulate: NAI-44 T1's no-clear leaves the player slot pointing at the
	// WorldSuspended state (production path: Suspended→WorldSuspended transition
	// enqueues into world queue but does NOT null activeScript).
	p.activeScript = state

	// Register player so processActiveScripts iterates over it.
	s.playersMu.Lock()
	s.playerLoop = append(s.playerLoop, p)
	s.playersMu.Unlock()

	// processActiveScripts must NOT re-fire: gate is Execution == Suspended only.
	s.processActiveScripts()

	if state.Execution != script.WorldSuspended {
		t.Errorf("after processActiveScripts: state.Execution got %v, want WorldSuspended (must not re-fire)", state.Execution)
	}
	if p.activeScript != state {
		t.Errorf("after processActiveScripts: p.activeScript got %p, want %p (slot must remain)", p.activeScript, state)
	}
}

// TestResumeOrFinish_PreservesUnrelatedSuspendedScript pins the
// NAI-54 Suspended-clobber bug fix end-to-end via resumeOrFinish.
// A fresh script Y that Finished must NOT null an unrelated suspended
// activeScript X already stored on the player. Mirrors TS
// Player.ts:2143 `if (script === this.activeScript)` guard.
func TestResumeOrFinish_PreservesUnrelatedSuspendedScript(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	// Pre-seed: an unrelated PauseButton-suspended X stored on the player.
	stored := &script.ScriptState{
		Script:    &script.ScriptFile{Name: "stored-paused"},
		Execution: script.PauseButton,
	}
	p.activeScript = stored

	// Y: a fresh script that returns immediately (Finished after Execute).
	sf := &script.ScriptFile{
		Name: "[fresh,test]",
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

	if p.activeScript != stored {
		t.Errorf("activeScript: got %p, want preserved %p (NAI-54 guard: fresh-Y finishing must not null unrelated stored X)",
			p.activeScript, stored)
	}
}

// TestResumeOrFinish_ExecuteError_PreservesUnrelatedSuspendedScript pins
// the NAI-55 error-path match-guard: a fresh script Y that errors during
// script.Execute must NOT null an unrelated stored activeScript X on the
// player. Mirrors TS ScriptRunner.execute setting Execution=ABORTED on
// throw (ScriptRunner.ts:228), then Player.executeScript re-entering the
// (script === this.activeScript) guard (Player.ts:2143).
func TestResumeOrFinish_ExecuteError_PreservesUnrelatedSuspendedScript(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	stored := &script.ScriptState{
		Script:    &script.ScriptFile{Name: "stored-paused"},
		Execution: script.PauseButton,
	}
	p.activeScript = stored

	// Y: bad-opcode script. Execute hits the "no handler" arm at
	// runner.go:69-72, which sets Execution=Aborted and returns the error.
	sf := &script.ScriptFile{
		Name:    "[err,test]",
		Opcodes: []script.Opcode{script.Opcode(0xFFFF)},
	}
	state := script.Init(sf, p, false, nil, nil)
	state.Provider = s.scriptProvider
	state.World = s.worldVars
	state.Configs = s.configsView
	state.Inv = s.invLookup
	state.Npcs = s.npcLookup
	state.LineValidator = s.scriptLineValidator()

	s.resumeOrFinish(state, p)

	if p.activeScript != stored {
		t.Errorf("activeScript: got %p, want preserved %p (NAI-55 error-path guard: fresh-Y erroring must not null unrelated stored X)",
			p.activeScript, stored)
	}
}

// TestResumeOrFinish_ExecuteError_ClearsMatchingActiveScript pins
// the NAI-55 error-path match arm: when the fresh state IS the player's
// activeScript and Execute errors, activeScript is nulled AND
// CloseModal(false) fires when no MAIN modal is open. Mirrors TS
// Player.ts:2143-2148 reached after ScriptRunner.execute returned ABORTED.
func TestResumeOrFinish_ExecuteError_ClearsMatchingActiveScript(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	sf := &script.ScriptFile{
		Name:    "[err,match]",
		Opcodes: []script.Opcode{script.Opcode(0xFFFF)},
	}
	state := script.Init(sf, p, false, nil, nil)
	state.Provider = s.scriptProvider
	state.World = s.worldVars
	state.Configs = s.configsView
	state.Inv = s.invLookup
	state.Npcs = s.npcLookup
	state.LineValidator = s.scriptLineValidator()

	p.activeScript = state // match-arm: state IS the player's activeScript
	p.modalState = modalStateChat
	p.modalChat = 100
	p.refreshModalClose = false

	s.resumeOrFinish(state, p)

	if p.activeScript != nil {
		t.Errorf("activeScript: got non-nil, want nil (match-arm must clear on error)")
	}
	if p.modalState != modalStateNone {
		t.Errorf("modalState: got %#x, want %#x (CloseModal(false) must fire on no-MAIN error)",
			p.modalState, modalStateNone)
	}
	if !p.refreshModalClose {
		t.Errorf("refreshModalClose: got false, want true (CloseModal must fire)")
	}
	if p.modalChat != -1 {
		t.Errorf("modalChat: got %d, want -1 (CloseModal must reset slot)", p.modalChat)
	}
}
