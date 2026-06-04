package world

import (
	"net"
	"testing"

	"github.com/zsrv/goscape/pkg/coordgrid"
	entitypkg "github.com/zsrv/goscape/pkg/entity"
	io2 "github.com/zsrv/goscape/pkg/io/isaac"
	gameclient "github.com/zsrv/goscape/pkg/io/protocol/game/client"
	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
	"github.com/zsrv/goscape/pkg/script"
)

// TestProcessIn_DecodedThisTickResetAtStart pins T1a: at the top of
// processIn, decodedThisTick is reset to false BEFORE the decode loop
// runs. Sentinel: pre-set to true; a no-read processIn must reset.
func TestProcessIn_DecodedThisTickResetAtStart(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.decodedThisTick = true // poison from prior tick
	// No bytes in c.in → decode loop reads zero packets.

	p.processIn(0)

	if p.decodedThisTick {
		t.Error("decodedThisTick: want false after no-read processIn (must reset before decode loop)")
	}
}

// TestProcessIn_DecodedThisTickStaysFalseOnNoRead pins T1c: after a
// processIn tick that read zero packets, decodedThisTick is false.
// Equivalent intent to TS decodeIn() returning false.
func TestProcessIn_DecodedThisTickStaysFalseOnNoRead(t *testing.T) {
	p, _ := newTestPlayer(t)
	// No bytes in c.in.

	p.processIn(0)

	if p.decodedThisTick {
		t.Error("decodedThisTick: want false on no-read tick")
	}
}

// TestProcessIn_DecodedThisTickSetAfterRead pins T1b: after processIn
// reads ≥1 packet, decodedThisTick is true. Uses NO_TIMEOUT (opcode 107 at 244,
// 0-payload) — same pattern as TestReadPacketNoTimeoutConsumesAndResetsOpcode.
func TestProcessIn_DecodedThisTickSetAfterRead(t *testing.T) {
	enc, dec := isaacPair([4]uint32{10, 20, 30, 40})
	p, _ := newTestPlayer(t)
	p.client.decryptor = dec

	// NO_TIMEOUT: opcode 107 (244), payload 0.
	p.client.in.Write([]byte{encryptOpcode(enc, gameclient.OpcNoTimeout)})

	p.processIn(0)

	if !p.decodedThisTick {
		t.Error("decodedThisTick: want true after reading ≥1 packet")
	}
}

// TestPlayer_UnsetMapFlag_ClearsWaypointAndEmitsPacket pins T2: the
// new helper bundles clearWaypoints (waypointIndex=-1) + the
// OpUnsetMapFlag wire write. Mirrors TS Player.unsetMapFlag
// (Player.ts:2169-2172). Sibling pattern to
// TestTeleCheat_UnsetMapFlag_ClearsWaypointAndEmitsPacket.
func TestPlayer_UnsetMapFlag_ClearsWaypointAndEmitsPacket(t *testing.T) {
	p, cc := newTestPlayer(t)
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	p.waypointIndex = 5

	// Sibling decoder seeded from same key for opcode comparison.
	sibling := io2.New([4]uint32{1, 2, 3, 4})

	// Start drain BEFORE the action; drainConn requires this ordering.
	received := drainConn(t, cc)
	p.unsetMapFlag()
	p.client.flushWrite()
	emitted := <-received

	if p.waypointIndex != -1 {
		t.Errorf("waypointIndex: got %d, want -1 (clearWaypoints arm of bundle)", p.waypointIndex)
	}
	if len(emitted) == 0 {
		t.Fatalf("no bytes emitted; expected OpUnsetMapFlag (opcode %d)", gameserver.OpUnsetMapFlag.Opcode)
	}
	wantEnc := byte((int(gameserver.OpUnsetMapFlag.Opcode) + int(sibling.GetNext())) & 0xff)
	if emitted[0] != wantEnc {
		t.Errorf("first emitted byte: got %d, want %d (encrypted OpUnsetMapFlag)", emitted[0], wantEnc)
	}
}

// newPostDecodeTestPlayerWithConn wires a Player with the minimum
// scaffolding required to drive (p *Player) processPostDecode
// end-to-end:
//   - p.client.server (with default cfg: PLAYERPACKET, routefinder=true)
//   - p.decodedThisTick = true       (outer gate satisfied)
//   - p.userPath set to one packed coord (outer gate satisfied)
//   - p.faceEntity = -1              (faceEntity branch no-op by default)
//   - p.moveClickRequest = false     (sentinel)
//
// Returns (player, server, conn). Most branch tests don't need the
// conn (use the wrapper newPostDecodeTestPlayer). Wire-asserting
// branches (T3b delayed) drain the conn directly.
func newPostDecodeTestPlayerWithConn(t *testing.T) (*Player, *Server, net.Conn) {
	t.Helper()
	p, cc := newTestPlayer(t)
	s := &Server{
		log: discardLogger(),
		cfg: Config{
			NodeWalktriggerSetting: WalkTriggerSettingPlayerpacket,
			NodeClientRoutefinder:  true,
		},
	}
	p.client.server = s
	p.decodedThisTick = true
	p.userPath = []int{0x12345}
	p.faceEntity = -1
	p.moveClickRequest = false
	return p, s, cc
}

// newPostDecodeTestPlayer is a wrapper that discards the conn for
// branch tests that don't drain wire output.
func newPostDecodeTestPlayer(t *testing.T) (*Player, *Server) {
	t.Helper()
	p, s, _ := newPostDecodeTestPlayerWithConn(t)
	return p, s
}

// TestProcessPostDecode_OuterGateSkipsWhenNotDecoded pins TS L611
// `decodeIn()` short-circuit. With decodedThisTick=false the entire
// block is skipped — moveClickRequest is NOT set even when userPath
// AND opcalled would otherwise satisfy L613.
func TestProcessPostDecode_OuterGateSkipsWhenNotDecoded(t *testing.T) {
	p, _ := newPostDecodeTestPlayer(t)
	p.decodedThisTick = false
	p.opcalled = true // would otherwise satisfy L613

	p.processPostDecode()

	if p.moveClickRequest {
		t.Error("moveClickRequest: want false (block skipped on !decodedThisTick)")
	}
}

// TestProcessPostDecode_OuterGateSkipsWhenIdle pins TS L613 outer
// gate. With userPath empty AND !opcalled, the block returns early.
// moveClickRequest stays at its sentinel.
func TestProcessPostDecode_OuterGateSkipsWhenIdle(t *testing.T) {
	p, _ := newPostDecodeTestPlayer(t)
	p.userPath = nil
	p.opcalled = false

	p.processPostDecode()

	if p.moveClickRequest {
		t.Error("moveClickRequest: want false (block skipped on !userPath && !opcalled)")
	}
}

// TestProcessPostDecode_DelayedFiresUnsetMapFlagAndReturns pins TS
// L614-617: when delayed AND outer gate satisfied (userPath set),
// unsetMapFlag fires (waypointIndex=-1 + OpUnsetMapFlag) and the
// block returns BEFORE the faceEntity reset / moveClickRequest setter.
//
// newPostDecodeTestPlayerWithConn (defined above) returns the conn
// alongside the player so this test can drainConn the wire output.
func TestProcessPostDecode_DelayedFiresUnsetMapFlagAndReturns(t *testing.T) {
	p, _, cc := newPostDecodeTestPlayerWithConn(t)
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	sibling := io2.New([4]uint32{1, 2, 3, 4})

	p.delayed = true
	p.waypointIndex = 5       // would clear under unsetMapFlag bundle
	p.faceEntity = 42         // would reset if delayed branch DIDN'T return
	p.moveClickRequest = true // sentinel — must NOT be touched after return

	received := drainConn(t, cc)
	p.processPostDecode()
	p.client.flushWrite()
	emitted := <-received

	if p.waypointIndex != -1 {
		t.Errorf("waypointIndex: got %d, want -1 (delayed → unsetMapFlag bundle)", p.waypointIndex)
	}
	if len(emitted) == 0 {
		t.Fatalf("no bytes emitted; expected OpUnsetMapFlag (opcode %d)", gameserver.OpUnsetMapFlag.Opcode)
	}
	wantEnc := byte((int(gameserver.OpUnsetMapFlag.Opcode) + int(sibling.GetNext())) & 0xff)
	if emitted[0] != wantEnc {
		t.Errorf("first emitted byte: got %d, want %d (encrypted OpUnsetMapFlag)", emitted[0], wantEnc)
	}
	if p.faceEntity != 42 {
		t.Errorf("faceEntity: got %d, want 42 (delayed branch must return BEFORE faceEntity reset)", p.faceEntity)
	}
	if !p.moveClickRequest {
		t.Error("moveClickRequest: want true (sentinel preserved — delayed branch must return BEFORE setter)")
	}
}

// TestProcessPostDecode_FaceEntityResetForLocTarget pins TS L619-622
// for *Loc target: faceEntity reset to -1, masks |= entitymask.
func TestProcessPostDecode_FaceEntityResetForLocTarget(t *testing.T) {
	p, _ := newPostDecodeTestPlayer(t)
	p.target = &entitypkg.Loc{} // any *Loc satisfies the type-switch
	p.faceEntity = 42
	p.masks = 0
	p.opcalled = true // satisfies outer L613 gate

	p.processPostDecode()

	if p.faceEntity != -1 {
		t.Errorf("faceEntity: got %d, want -1 (Loc target → reset)", p.faceEntity)
	}
	if p.masks&p.entitymask == 0 {
		t.Errorf("masks: entitymask bit (%d) not set; got masks=%d", p.entitymask, p.masks)
	}
}

// TestProcessPostDecode_FaceEntityResetForObjTarget pins same for *Obj.
func TestProcessPostDecode_FaceEntityResetForObjTarget(t *testing.T) {
	p, _ := newPostDecodeTestPlayer(t)
	p.target = &entitypkg.Obj{}
	p.faceEntity = 42
	p.masks = 0
	p.opcalled = true

	p.processPostDecode()

	if p.faceEntity != -1 {
		t.Errorf("faceEntity: got %d, want -1 (Obj target → reset)", p.faceEntity)
	}
	if p.masks&p.entitymask == 0 {
		t.Errorf("masks: entitymask bit (%d) not set; got masks=%d", p.entitymask, p.masks)
	}
}

// TestProcessPostDecode_FaceEntityResetForNilTarget pins TS L619 nil
// target arm: nil target + faceEntity!=-1 → reset.
func TestProcessPostDecode_FaceEntityResetForNilTarget(t *testing.T) {
	p, _ := newPostDecodeTestPlayer(t)
	p.target = nil
	p.faceEntity = 42
	p.masks = 0
	p.opcalled = true

	p.processPostDecode()

	if p.faceEntity != -1 {
		t.Errorf("faceEntity: got %d, want -1 (nil target → reset)", p.faceEntity)
	}
}

// TestProcessPostDecode_FaceEntityPreservedForPlayerTarget pins the
// negative arm: PathingEntity targets (Player/Npc) do NOT trigger
// the faceEntity reset.
func TestProcessPostDecode_FaceEntityPreservedForPlayerTarget(t *testing.T) {
	s := newTestServer(t)
	other := newTestPlayerAt(t, s, 2, 3200, 3200, 0)
	p, _ := newPostDecodeTestPlayer(t)
	p.target = other
	p.faceEntity = 42
	p.masks = 0
	p.opcalled = true

	p.processPostDecode()

	if p.faceEntity != 42 {
		t.Errorf("faceEntity: got %d, want 42 (Player target → preserved)", p.faceEntity)
	}
	if p.masks != 0 {
		t.Errorf("masks: got %d, want 0 (Player target → masks NOT touched)", p.masks)
	}
}

// TestProcessPostDecode_FaceEntityNoOpWhenAlreadyMinusOne pins TS L620
// guard: when faceEntity is already -1, masks is NOT touched.
func TestProcessPostDecode_FaceEntityNoOpWhenAlreadyMinusOne(t *testing.T) {
	p, _ := newPostDecodeTestPlayer(t)
	p.target = &entitypkg.Loc{}
	p.faceEntity = -1 // guard: already cleared
	p.masks = 0
	p.opcalled = true

	p.processPostDecode()

	if p.faceEntity != -1 {
		t.Errorf("faceEntity: got %d, want -1 (no-op preserves)", p.faceEntity)
	}
	if p.masks != 0 {
		t.Errorf("masks: got %d, want 0 (faceEntity already -1 → masks NOT touched)", p.masks)
	}
}

// TestProcessPostDecode_MoveClickRequest_NotBusyOpcalled pins TS L624-625
// branch: !busy() && opcalled → moveClickRequest = false.
func TestProcessPostDecode_MoveClickRequest_NotBusyOpcalled(t *testing.T) {
	p, _ := newPostDecodeTestPlayer(t)
	p.opcalled = true
	p.delayed = false
	p.modalState = modalStateNone
	p.moveClickRequest = true // sentinel — must flip to false
	p.targetOp = -1           // not 3 → !followingPlayer (prerequisite for the gate, not its suppressor)
	// Real pathToTarget suppressor: !followingPlayer && opcalled &&
	// (len(userPath)==0 || !routefinder). With routefinder=true AND
	// len(userPath)>0 (both default in fixture), the disjunction is
	// false → pathToTarget gate fails → setter is reached.

	p.processPostDecode()

	if p.moveClickRequest {
		t.Error("moveClickRequest: want false (!Busy + opcalled)")
	}
}

// TestProcessPostDecode_MoveClickRequest_BusyOpcalled pins TS L626-627
// branch: Busy + opcalled → moveClickRequest = true.
func TestProcessPostDecode_MoveClickRequest_BusyOpcalled(t *testing.T) {
	p, _ := newPostDecodeTestPlayer(t)
	p.opcalled = true
	p.modalState = modalStateMain // Busy() = true
	p.delayed = false
	p.moveClickRequest = false // sentinel — must flip to true

	p.processPostDecode()

	if !p.moveClickRequest {
		t.Error("moveClickRequest: want true (Busy + opcalled)")
	}
}

// TestProcessPostDecode_MoveClickRequest_NotBusyNotOpcalled pins TS
// L626-627 else-branch: !Busy + !opcalled + userPath set → moveClickRequest = true.
func TestProcessPostDecode_MoveClickRequest_NotBusyNotOpcalled(t *testing.T) {
	p, _ := newPostDecodeTestPlayer(t)
	p.opcalled = false
	p.delayed = false
	p.modalState = modalStateNone
	p.moveClickRequest = false // sentinel

	p.processPostDecode()

	if !p.moveClickRequest {
		t.Error("moveClickRequest: want true (!Busy + !opcalled + userPath set)")
	}
}

// TestProcessPostDecode_PathToTargetFiresAndReturns pins TS L630-633:
// when !followingPlayer && opcalled && (userPath==0 || !routefinder),
// pathToTarget runs and the block returns BEFORE the walktrigger
// fallback. Sentinel: walktrigger=42 with PLAYERSETUP cfg would be
// consumed by the fallback if it ran; we assert it survives.
func TestProcessPostDecode_PathToTargetFiresAndReturns(t *testing.T) {
	p, s := newPostDecodeTestPlayer(t)
	s.cfg.NodeWalktriggerSetting = WalkTriggerSettingPlayersetup
	s.cfg.NodeClientRoutefinder = false // forces the pathToTarget gate
	p.opcalled = true
	p.targetOp = 1 // not 3 → !followingPlayer
	p.target = nil // pathToTarget short-circuits to no-op (interaction.go:672)
	p.delayed = false
	p.modalState = modalStateNone
	p.walktrigger = 42 // sentinel — would be consumed by fallback if it ran
	p.userPath = []int{coordgrid.PackCoord(p.level, p.x+1, p.z)}

	p.processPostDecode()

	if p.walktrigger != 42 {
		t.Errorf("walktrigger: got %d, want 42 (pathToTarget branch must return BEFORE walktrigger fallback)", p.walktrigger)
	}
}

// TestProcessPostDecode_PathToTargetSkippedForFollowingPlayer pins
// TS L630 followingPlayer guard: targetOp==3 → pathToTarget NOT called
// → walktrigger fallback proceeds.
//
// Signal: waypointIndex starts at -1; if fallback runs,
// pathToMoveClick → queueWaypoints sets it to >=0.
// (We can't use walktrigger here: PLAYERSETUP fallback only fires
// processWalktrigger when !opcalled, and we need opcalled=true to
// satisfy the other clauses of the pathToTarget gate.)
func TestProcessPostDecode_PathToTargetSkippedForFollowingPlayer(t *testing.T) {
	p, s := newPostDecodeTestPlayer(t)
	s.cfg.NodeWalktriggerSetting = WalkTriggerSettingPlayermovement // re-paths but no walktrigger
	s.cfg.NodeClientRoutefinder = false                             // gate's userPath/routefinder clause WOULD pass
	p.opcalled = true                                               // satisfies opcalled clause
	p.targetOp = 3                                                  // followingPlayer → BLOCKS gate at first clause
	p.delayed = false
	p.modalState = modalStateNone
	p.userPath = []int{coordgrid.PackCoord(p.level, p.x+1, p.z)}
	p.waypointIndex = -1 // sentinel

	p.processPostDecode()

	if !p.hasWaypoints() {
		t.Errorf("waypointIndex: got %d, want >= 0 (followingPlayer skips pathToTarget → fallback's pathToMoveClick re-paths)", p.waypointIndex)
	}
}

// TestProcessPostDecode_PathToTargetSkippedWhenRoutefinderAndUserPath
// pins TS L630 third gate-clause: with NodeClientRoutefinder=true AND
// userPath non-empty, the pathToTarget branch is skipped (the
// disjunction `len(userPath)==0 || !routefinder` is false →
// pathToTarget gate fails) → walktrigger fallback proceeds and
// re-paths userPath via pathToMoveClick.
//
// Signal: waypointIndex starts at -1 sentinel; if fallback runs,
// pathToMoveClick → queueWaypoints sets it to >=0.
// (We can't use walktrigger as signal here: opcalled=true is required
// to satisfy the OTHER clauses of the pathToTarget gate, but the
// PLAYERSETUP walktrigger sub-branch requires !opcalled — so
// walktrigger would NOT be consumed even if fallback proceeded.)
func TestProcessPostDecode_PathToTargetSkippedWhenRoutefinderAndUserPath(t *testing.T) {
	p, s := newPostDecodeTestPlayer(t)
	s.cfg.NodeWalktriggerSetting = WalkTriggerSettingPlayermovement // re-path only; no walktrigger
	s.cfg.NodeClientRoutefinder = true
	p.opcalled = true // satisfies pathToTarget opcalled clause
	p.targetOp = 1    // !followingPlayer
	p.delayed = false
	p.modalState = modalStateNone
	p.userPath = []int{coordgrid.PackCoord(p.level, p.x+1, p.z)}
	p.waypointIndex = -1 // sentinel; fallback's pathToMoveClick should set >=0

	p.processPostDecode()

	if !p.hasWaypoints() {
		t.Errorf("waypointIndex: got %d, want >= 0 (routefinder+userPath skips pathToTarget → fallback's pathToMoveClick re-paths)", p.waypointIndex)
	}
}

// TestProcessPostDecode_WalktriggerFallback_PlayerpacketNoOp pins TS
// L635 cfg gate: under PLAYERPACKET (default) the fallback is a no-op.
// Sentinel: walktrigger=42 + hasWaypoints — walktrigger MUST survive.
func TestProcessPostDecode_WalktriggerFallback_PlayerpacketNoOp(t *testing.T) {
	p, _ := newPostDecodeTestPlayer(t)
	// Default cfg in fixture is PLAYERPACKET.
	p.opcalled = false
	p.delayed = false
	p.modalState = modalStateNone
	p.walktrigger = 42
	p.waypointIndex = 0 // hasWaypoints → true

	p.processPostDecode()

	if p.walktrigger != 42 {
		t.Errorf("walktrigger: got %d, want 42 (PLAYERPACKET cfg → fallback no-op)", p.walktrigger)
	}
}

// TestProcessPostDecode_WalktriggerFallback_Playersetup_FiresWhenNotOpcalled
// pins TS L638: PLAYERSETUP + !opcalled + hasWaypoints →
// processWalktrigger fires (consumes walktrigger field).
func TestProcessPostDecode_WalktriggerFallback_Playersetup_FiresWhenNotOpcalled(t *testing.T) {
	p, s := newPostDecodeTestPlayer(t)
	s.cfg.NodeWalktriggerSetting = WalkTriggerSettingPlayersetup
	p.opcalled = false
	p.delayed = false
	p.modalState = modalStateNone
	p.walktrigger = 42
	p.userPath = []int{coordgrid.PackCoord(p.level, p.x+1, p.z)}

	s.scriptProvider = script.NewProvider()
	s.scriptProvider.Register(&script.ScriptFile{
		Name:        "[walk_test_setup_fires]",
		LookupKey:   42,
		Opcodes:     []script.Opcode{script.OpReturn},
		IntOperands: []int32{0}, StringOperands: []string{""}, InstructionCount: 1,
	})

	p.processPostDecode()

	if p.walktrigger != -1 {
		t.Errorf("walktrigger: got %d, want -1 (PLAYERSETUP + !opcalled → processWalktrigger consumed)", p.walktrigger)
	}
}

// TestProcessPostDecode_WalktriggerFallback_Playersetup_SkipsWhenOpcalled
// pins TS L638 !opcalled guard: opcalled=true → re-path runs but
// processWalktrigger does NOT fire.
func TestProcessPostDecode_WalktriggerFallback_Playersetup_SkipsWhenOpcalled(t *testing.T) {
	p, s := newPostDecodeTestPlayer(t)
	s.cfg.NodeWalktriggerSetting = WalkTriggerSettingPlayersetup
	s.cfg.NodeClientRoutefinder = true // also forces pathToTarget gate to skip (userPath set)
	p.opcalled = true
	p.targetOp = 3 // followingPlayer → bypasses pathToTarget branch
	p.delayed = false
	p.modalState = modalStateNone
	p.walktrigger = 42
	p.userPath = []int{coordgrid.PackCoord(p.level, p.x+1, p.z)}

	p.processPostDecode()

	if p.walktrigger != 42 {
		t.Errorf("walktrigger: got %d, want 42 (PLAYERSETUP + opcalled=true → walktrigger NOT fired)", p.walktrigger)
	}
}
