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
