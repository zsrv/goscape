package rsbuf

import (
	"testing"

	"github.com/zsrv/goscape/pkg/io/packet"
)

// fakeNpcSource — NPC analogue of fakeSource (player) for byte-level tests.
type fakeNpcSource struct {
	nid, typeID                               int
	x, z, level                               int
	active                                    bool
	masks, entityMask                         int
	animID, animDelay                         int
	faceEntity                                int
	sayText                                   []byte
	damageAmt, damageType, curHP, baseHP      int
	damage2Amt, damage2Type                   int
	changeTypeID                              int
	spotanimID, spotanimHeight, spotanimDelay int
	faceSquareX, faceSquareZ                  int
}

func (f *fakeNpcSource) Nid() int                { return f.nid }
func (f *fakeNpcSource) TypeID() int             { return f.typeID }
func (f *fakeNpcSource) Coords() (int, int, int) { return f.x, f.z, f.level }
func (f *fakeNpcSource) Active() bool            { return f.active }
func (f *fakeNpcSource) Masks() int              { return f.masks }
func (f *fakeNpcSource) EntityMask() int         { return f.entityMask }
func (f *fakeNpcSource) AnimID() int             { return f.animID }
func (f *fakeNpcSource) AnimDelay() int          { return f.animDelay }
func (f *fakeNpcSource) FaceEntity() int         { return f.faceEntity }
func (f *fakeNpcSource) SayText() []byte         { return f.sayText }
func (f *fakeNpcSource) DamageAmt() int          { return f.damageAmt }
func (f *fakeNpcSource) DamageType() int         { return f.damageType }
func (f *fakeNpcSource) Damage2Amt() int         { return f.damage2Amt }
func (f *fakeNpcSource) Damage2Type() int        { return f.damage2Type }
func (f *fakeNpcSource) CurHP() int              { return f.curHP }
func (f *fakeNpcSource) BaseHP() int             { return f.baseHP }
func (f *fakeNpcSource) ChangeTypeID() int       { return f.changeTypeID }
func (f *fakeNpcSource) SpotAnimID() int         { return f.spotanimID }
func (f *fakeNpcSource) SpotAnimHeight() int     { return f.spotanimHeight }
func (f *fakeNpcSource) SpotAnimDelay() int      { return f.spotanimDelay }
func (f *fakeNpcSource) FaceSquareX() int        { return f.faceSquareX }
func (f *fakeNpcSource) FaceSquareZ() int        { return f.faceSquareZ }
func (f *fakeNpcSource) WalkDir() int            { return -1 }
func (f *fakeNpcSource) RunDir() int             { return -1 }
func (f *fakeNpcSource) Tele() bool              { return false }
func (f *fakeNpcSource) LastTickX() int          { return -1 }
func (f *fakeNpcSource) LastTickZ() int          { return -1 }
func (f *fakeNpcSource) LastLevel() int          { return -1 }

func TestNpcAnimPayload(t *testing.T) {
	n := &fakeNpcSource{masks: NpcMaskAnim, animID: 100, animDelay: 5}
	buf := packet.NewPacket(nil)
	writeNpcMaskPayloads(buf, n, NpcMaskAnim)
	want := []byte{0x00, 0x64, 0x05}
	got := buf.Data
	if len(got) != 3 || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestNpcDamagePayload(t *testing.T) {
	n := &fakeNpcSource{masks: NpcMaskDamage, damageAmt: 10, damageType: 1, curHP: 40, baseHP: 50}
	buf := packet.NewPacket(nil)
	writeNpcMaskPayloads(buf, n, NpcMaskDamage)
	want := []byte{10, 1, 40, 50}
	for i := range want {
		if buf.Data[i] != want[i] {
			t.Errorf("byte[%d]: got %d, want %d", i, buf.Data[i], want[i])
		}
	}
}

func TestNpcSpotAnimPayload(t *testing.T) {
	n := &fakeNpcSource{masks: NpcMaskSpotAnim, spotanimID: 0x1234, spotanimHeight: 50, spotanimDelay: 10}
	buf := packet.NewPacket(nil)
	writeNpcMaskPayloads(buf, n, NpcMaskSpotAnim)
	// p2(0x1234) = [0x12, 0x34]; p4((50<<16)|10 = 0x0032000A) = [0x00, 0x32, 0x00, 0x0A]
	want := []byte{0x12, 0x34, 0x00, 0x32, 0x00, 0x0A}
	for i := range want {
		if buf.Data[i] != want[i] {
			t.Errorf("byte[%d]: got %#x, want %#x", i, buf.Data[i], want[i])
		}
	}
}

func TestNpcFaceCoordPayload(t *testing.T) {
	n := &fakeNpcSource{masks: NpcMaskFaceCoord, faceSquareX: 0x0182, faceSquareZ: 0x0184}
	buf := packet.NewPacket(nil)
	writeNpcMaskPayloads(buf, n, NpcMaskFaceCoord)
	want := []byte{0x01, 0x82, 0x01, 0x84}
	for i := range want {
		if buf.Data[i] != want[i] {
			t.Errorf("byte[%d]: got %#x, want %#x", i, buf.Data[i], want[i])
		}
	}
}

func TestNpcMaskHeader1Byte(t *testing.T) {
	buf := packet.NewPacket(nil)
	writeNpcMaskHeader(buf, NpcMaskAnim|NpcMaskFaceCoord) // 2|128 = 130
	if buf.Data[0] != 130 {
		t.Errorf("header: got %d, want 130", buf.Data[0])
	}
}

// TestNpcMaskDamage2BitValue pins NpcMaskDamage2 == 0x1 (rsbuf 244 prot.rs DAMAGE2=0x1).
func TestNpcMaskDamage2BitValue(t *testing.T) {
	if NpcMaskDamage2 != 0x1 {
		t.Errorf("NpcMaskDamage2: got 0x%x, want 0x1", NpcMaskDamage2)
	}
}

// TestNpcDamage2Payload pins the DAMAGE2 wire encoding for NPCs:
// p1(amt) p1(type) p1(cur) p1(base), identical to DAMAGE payload shape
// (rsbuf 244 renderer.rs NpcInfoDamage::new with damage_taken2/damage_type2).
func TestNpcDamage2Payload(t *testing.T) {
	n := &fakeNpcSource{masks: NpcMaskDamage2, damage2Amt: 7, damage2Type: 3, curHP: 25, baseHP: 60}
	buf := packet.NewPacket(nil)
	writeNpcMaskPayloads(buf, n, NpcMaskDamage2)
	got := buf.Data
	// p1(7) p1(3) p1(25) p1(60)
	want := []byte{0x07, 0x03, 0x19, 0x3c}
	if len(got) != len(want) {
		t.Fatalf("length: got %d, want %d (bytes=%#v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("byte[%d]: got %#x, want %#x", i, got[i], want[i])
		}
	}
}

// TestNpcDamage2WrittenFirst pins DAMAGE2 appearing BEFORE ANIM in the NPC wire stream
// (rsbuf 244 info.rs write_blocks: DAMAGE2 at line 683-685 — FIRST, before ANIM at 686-688).
func TestNpcDamage2WrittenFirst(t *testing.T) {
	n := &fakeNpcSource{
		masks:      NpcMaskDamage2 | NpcMaskAnim,
		damage2Amt: 7, damage2Type: 3, curHP: 25, baseHP: 60,
		animID: 100, animDelay: 5,
	}
	buf := packet.NewPacket(nil)
	writeNpcMaskPayloads(buf, n, NpcMaskDamage2|NpcMaskAnim)
	got := buf.Data
	// DAMAGE2 first: p1(7) p1(3) p1(25) p1(60)
	// ANIM after:    p2(100) p1(5) = [0x00, 0x64, 0x05]
	want := []byte{0x07, 0x03, 0x19, 0x3c, 0x00, 0x64, 0x05}
	if len(got) != len(want) {
		t.Fatalf("length: got %d, want %d (bytes=%#v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("byte[%d]: got %#x, want %#x (full=%#v)", i, got[i], want[i], got)
		}
	}
}
