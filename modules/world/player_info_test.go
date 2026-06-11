package world

import (
	"bytes"
	"testing"

	"github.com/zsrv/goscape/pkg/gamemap"
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/rsbuf"
)

// setupInfoPlayer constructs a Player with the full 3a/3b scaffolding bypassing
// the normal login/processLogins pipeline.
func setupInfoPlayer(t *testing.T, s *Server, slot, x, z, level int) *Player {
	t.Helper()
	p, _ := newTestPlayer(t)
	p.client.server = s
	enc, dec := isaacPair([4]uint32{uint32(slot), 2, 3, 4})
	p.client.encryptor = enc
	p.client.decryptor = dec
	p.x, p.z, p.level = x, z, level
	p.originX, p.originZ = x, z
	p.lastTickX, p.lastTickZ, p.lastLevel = x, z, level
	p.slot = slot
	s.players.set(slot, p)
	p.active = true
	if s.rsbuf != nil {
		s.rsbuf.AddPlayer(int32(slot))
	}
	return p
}

func TestTwoPlayersSeeEachOther(t *testing.T) {
	s := newTestServer(t)
	s.gamemap = gamemap.New(discardLogger())
	if err := s.gamemap.Init(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	s.renderer = rsbuf.NewRenderer()

	a := setupInfoPlayer(t, s, 1, 3094, 3106, 0)
	_ = setupInfoPlayer(t, s, 2, 3095, 3106, 0)

	s.processInfo()
	a.updatePlayers()

	if !s.rsbuf.HasPlayer(int32(a.slot), 2) {
		t.Errorf("a should track b after updatePlayers; HasPlayer(%d, 2) returned false", a.slot)
	}
}

func TestSayProducesChatMaskInHighDef(t *testing.T) {
	s := newTestServer(t)
	s.gamemap = gamemap.New(discardLogger())
	if err := s.gamemap.Init(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	s.renderer = rsbuf.NewRenderer()

	a := setupInfoPlayer(t, s, 1, 3094, 3106, 0)

	a.Say([]byte("hello"))
	s.processInfo()

	highDef := s.renderer.HighDefOf(1)
	if len(highDef) == 0 {
		t.Fatal("high-def should be non-empty after Say()")
	}
	// Header byte should include MaskSay (0x8). Note: rendering suppresses CHAT
	// (rsbuf.MaskChat=0x40) but MaskSay is the "speech bubble" and IS included.
	if highDef[0]&rsbuf.MaskSay == 0 {
		t.Errorf("high-def header should have MaskSay (0x8): got %d (%#x)", highDef[0], highDef[0])
	}
}

// TestDamage2ThreadsToWire_Player pins the T13 integration property for the
// player path: two Damage() calls in the same tick produce BOTH a DAMAGE block
// and a DAMAGE2 block in the renderer's high-def wire payload, with DAMAGE
// strictly BEFORE DAMAGE2 (player canonical order: DAMAGE2 last, info.rs:402-404).
//
// Flow: entity Damage()×2 → processInfo() → s.renderer.ComputePlayers (live
// PlayerSource adapter) → buildPayload → HighDefOf. This is the Arc-30 bug class
// (two parallel compute paths), so the pin goes through the full live Source
// adapter path rather than a fake/stub.
//
// Wire layout for masks = MaskDamage(0x10)|MaskDamage2(0x400) = 0x410:
//
//	header:  IP2(0x410|0x80=0x490) little-endian = [0x90, 0x04]
//	DAMAGE:  p1(3) p1(0) p1(curHP) p1(baseHP)   — written at canonical position 4 (before DAMAGE2)
//	DAMAGE2: p1(5) p1(1) p1(curHP) p1(baseHP)   — written LAST (info.rs:402-404)
func TestDamage2ThreadsToWire_Player(t *testing.T) {
	s := newTestServer(t)
	s.gamemap = gamemap.New(discardLogger())
	if err := s.gamemap.Init(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	s.renderer = rsbuf.NewRenderer()

	// slot=1 to satisfy ComputePlayers slot≥1 guard.
	p := setupInfoPlayer(t, s, 1, 3094, 3106, 0)
	p.baseLevels[objtype.PlayerStatHitpoints] = 50
	p.levels[objtype.PlayerStatHitpoints] = 50

	// Two hits same tick: slot 0 → DAMAGE, slot 1 → DAMAGE2.
	p.Damage(3, 0) // first hit: damageAmt=3, damageType=0
	p.Damage(5, 1) // second hit: damage2Amt=5, damage2Type=1
	// After: curHP = 50-3-5 = 42, baseHP = 50.

	s.processInfo()

	highDef := s.renderer.HighDefOf(1)
	if len(highDef) == 0 {
		t.Fatal("HighDefOf(1) is nil — ComputePlayers did not populate slot 1")
	}

	// Both mask bits must be advertised in the 2-byte IP2 header.
	// IP2(0x490) little-endian: byte[0]=0x90, byte[1]=0x04.
	const wantHeader0 = byte(0x90) // low byte of 0x490
	const wantHeader1 = byte(0x04) // high byte of 0x490
	if len(highDef) < 2 {
		t.Fatalf("highDef too short (%d bytes) to contain 2-byte IP2 header", len(highDef))
	}
	if highDef[0] != wantHeader0 || highDef[1] != wantHeader1 {
		t.Errorf("IP2 header: got [%#x %#x], want [%#x %#x] (MaskDamage|MaskDamage2|MaskBig)",
			highDef[0], highDef[1], wantHeader0, wantHeader1)
	}

	// Exact wire layout: 2-byte header + 4-byte DAMAGE block + 4-byte DAMAGE2 block = 10 bytes.
	// curHP=42 (0x2a), baseHP=50 (0x32).
	wantDamage := []byte{0x03, 0x00, 0x2a, 0x32}  // DAMAGE: amt=3, type=0, cur=42, base=50
	wantDamage2 := []byte{0x05, 0x01, 0x2a, 0x32} // DAMAGE2: amt=5, type=1, cur=42, base=50

	const wantLen = 2 + 4 + 4 // header + DAMAGE + DAMAGE2
	if len(highDef) != wantLen {
		t.Fatalf("highDef length: got %d, want %d (full=%#v)", len(highDef), wantLen, highDef)
	}

	// DAMAGE block is at offset 2 (immediately after the 2-byte header).
	if !bytes.Equal(highDef[2:6], wantDamage) {
		t.Errorf("DAMAGE block at [2:6]: got %#v, want %#v", highDef[2:6], wantDamage)
	}

	// DAMAGE2 block is at offset 6 (strictly AFTER DAMAGE, last in payload).
	if !bytes.Equal(highDef[6:10], wantDamage2) {
		t.Errorf("DAMAGE2 block at [6:10]: got %#v, want %#v", highDef[6:10], wantDamage2)
	}

	// Order invariant: DAMAGE bytes must appear strictly before DAMAGE2 bytes.
	dmgOff := bytes.Index(highDef, wantDamage)
	dmg2Off := bytes.Index(highDef, wantDamage2)
	if dmgOff < 0 {
		t.Errorf("DAMAGE block %#v not found in highDef %#v", wantDamage, highDef)
	}
	if dmg2Off < 0 {
		t.Errorf("DAMAGE2 block %#v not found in highDef %#v", wantDamage2, highDef)
	}
	if dmgOff >= 0 && dmg2Off >= 0 && dmgOff >= dmg2Off {
		t.Errorf("DAMAGE block (off=%d) must be strictly BEFORE DAMAGE2 block (off=%d)", dmgOff, dmg2Off)
	}
}

// TestDamage2ThreadsToWire_Npc pins the T13 integration property for the NPC
// path: two Damage() calls produce BOTH NpcMaskDamage2 and NpcMaskDamage blocks
// in the renderer's NPC high-def payload, with DAMAGE2 strictly FIRST (NPC
// canonical order: DAMAGE2 first, info.rs:683-685).
//
// Flow: entity Damage()×2 → processInfo() → s.renderer.ComputeNpcs (live
// NpcSource adapter) → writeNpcMaskPayloads → NpcHighDefOf.
//
// Wire layout for masks = NpcMaskDamage(0x10)|NpcMaskDamage2(0x01) = 0x11:
//
//	header:  p1(0x11)                            — 1-byte NPC mask header
//	DAMAGE2: p1(5) p1(1) p1(curHP) p1(baseHP)   — written FIRST (info.rs:683-685)
//	DAMAGE:  p1(3) p1(0) p1(curHP) p1(baseHP)   — written after DAMAGE2
func TestDamage2ThreadsToWire_Npc(t *testing.T) {
	s := newTestServer(t)
	s.renderer = rsbuf.NewRenderer()

	// makeInteractionNpc adds the NPC to s.npcLoop so processInfo's
	// ComputeNpcs pass picks it up. nid=1 satisfies the ComputeNpcs nid≥1 guard.
	n := makeInteractionNpc(t, s, 1, 3094, 3106, 0)
	// Give the NPC HP so clamping is deterministic: baseHP=50, curHP=50.
	n.baseLevels[objtype.NpcStatHitpoints] = 50
	n.levels[objtype.NpcStatHitpoints] = 50

	// Two hits same tick: slot 0 → NpcMaskDamage, slot 1 → NpcMaskDamage2.
	n.Damage(3, 0) // first hit: damageAmt=3, damageType=0
	n.Damage(5, 1) // second hit: damage2Amt=5, damage2Type=1
	// After: curHP = 50-3-5 = 42, baseHP = 50.

	s.processInfo()

	npcHighDef := s.renderer.NpcHighDefOf(1)
	if len(npcHighDef) == 0 {
		t.Fatal("NpcHighDefOf(1) is nil — ComputeNpcs did not populate nid 1")
	}

	// 1-byte header for masks=0x11.
	if npcHighDef[0] != 0x11 {
		t.Errorf("NPC mask header: got %#x, want 0x11 (NpcMaskDamage|NpcMaskDamage2)", npcHighDef[0])
	}

	// Exact wire layout: 1-byte header + 4-byte DAMAGE2 + 4-byte DAMAGE = 9 bytes.
	// curHP=42 (0x2a), baseHP=50 (0x32).
	wantDamage2 := []byte{0x05, 0x01, 0x2a, 0x32} // DAMAGE2: amt=5, type=1, cur=42, base=50 — FIRST
	wantDamage := []byte{0x03, 0x00, 0x2a, 0x32}  // DAMAGE:  amt=3, type=0, cur=42, base=50

	const wantLen = 1 + 4 + 4 // header + DAMAGE2 + DAMAGE
	if len(npcHighDef) != wantLen {
		t.Fatalf("npcHighDef length: got %d, want %d (full=%#v)", len(npcHighDef), wantLen, npcHighDef)
	}

	// DAMAGE2 block is at offset 1 (immediately after the 1-byte header) — FIRST.
	if !bytes.Equal(npcHighDef[1:5], wantDamage2) {
		t.Errorf("DAMAGE2 block at [1:5]: got %#v, want %#v", npcHighDef[1:5], wantDamage2)
	}

	// DAMAGE block is at offset 5 (strictly AFTER DAMAGE2).
	if !bytes.Equal(npcHighDef[5:9], wantDamage) {
		t.Errorf("DAMAGE block at [5:9]: got %#v, want %#v", npcHighDef[5:9], wantDamage)
	}

	// Order invariant: DAMAGE2 bytes must appear strictly before DAMAGE bytes.
	dmg2Off := bytes.Index(npcHighDef, wantDamage2)
	dmgOff := bytes.Index(npcHighDef, wantDamage)
	if dmg2Off < 0 {
		t.Errorf("DAMAGE2 block %#v not found in npcHighDef %#v", wantDamage2, npcHighDef)
	}
	if dmgOff < 0 {
		t.Errorf("DAMAGE block %#v not found in npcHighDef %#v", wantDamage, npcHighDef)
	}
	if dmg2Off >= 0 && dmgOff >= 0 && dmg2Off >= dmgOff {
		t.Errorf("DAMAGE2 block (off=%d) must be strictly BEFORE DAMAGE block (off=%d) for NPC", dmg2Off, dmgOff)
	}
}
