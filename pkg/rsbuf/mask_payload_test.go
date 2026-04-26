package rsbuf

import (
	"testing"

	"github.com/zsrv/goscape/pkg/io/packet"
)

// fakeSource is a minimal PlayerSource for byte-layout tests.
type fakeSource struct {
	slot                                           int
	masks, entityMask                              int
	appearance                                     []byte
	animID, animDelay                              int
	faceEntity                                     int
	sayText                                        []byte
	damageAmt, damageType, curHP, baseHP           int
	faceSquareX, faceSquareZ                       int
	chatColour, chatEffect, chatRights             int
	chatBytes                                      []byte
	spotanimID, spotanimHeight, spotanimDelay      int
	exactStartX, exactStartZ, exactEndX, exactEndZ int
	exactBegin, exactFinish, exactDir              int
	originX, originZ                               int
	x, z, level                                    int
	tele, jump                                     bool
}

func (f *fakeSource) Slot() int               { return f.slot }
func (f *fakeSource) Coords() (int, int, int) { return f.x, f.z, f.level }
func (f *fakeSource) Active() bool             { return true }
func (f *fakeSource) Visibility() Visibility   { return VisibilityDefault }
func (f *fakeSource) StaffModLevel() int32     { return 0 }
func (f *fakeSource) Masks() int                { return f.masks }
func (f *fakeSource) EntityMask() int           { return f.entityMask }
func (f *fakeSource) AppearanceBytes() []byte   { return f.appearance }
func (f *fakeSource) AnimID() int               { return f.animID }
func (f *fakeSource) AnimDelay() int            { return f.animDelay }
func (f *fakeSource) FaceEntity() int           { return f.faceEntity }
func (f *fakeSource) SayText() []byte           { return f.sayText }
func (f *fakeSource) DamageAmt() int            { return f.damageAmt }
func (f *fakeSource) DamageType() int           { return f.damageType }
func (f *fakeSource) CurHP() int                { return f.curHP }
func (f *fakeSource) BaseHP() int               { return f.baseHP }
func (f *fakeSource) FaceSquareX() int          { return f.faceSquareX }
func (f *fakeSource) FaceSquareZ() int          { return f.faceSquareZ }
func (f *fakeSource) ChatColour() int           { return f.chatColour }
func (f *fakeSource) ChatEffect() int           { return f.chatEffect }
func (f *fakeSource) ChatRights() int           { return f.chatRights }
func (f *fakeSource) ChatBytes() []byte         { return f.chatBytes }
func (f *fakeSource) SpotAnimID() int           { return f.spotanimID }
func (f *fakeSource) SpotAnimHeight() int       { return f.spotanimHeight }
func (f *fakeSource) SpotAnimDelay() int        { return f.spotanimDelay }
func (f *fakeSource) ExactStartX() int          { return f.exactStartX }
func (f *fakeSource) ExactStartZ() int          { return f.exactStartZ }
func (f *fakeSource) ExactEndX() int            { return f.exactEndX }
func (f *fakeSource) ExactEndZ() int            { return f.exactEndZ }
func (f *fakeSource) ExactBegin() int           { return f.exactBegin }
func (f *fakeSource) ExactFinish() int          { return f.exactFinish }
func (f *fakeSource) ExactDir() int             { return f.exactDir }
func (f *fakeSource) WalkDir() int              { return -1 }
func (f *fakeSource) RunDir() int               { return -1 }
func (f *fakeSource) Tele() bool                { return f.tele }
func (f *fakeSource) Jump() bool                { return f.jump }
func (f *fakeSource) LastTickX() int            { return -1 }
func (f *fakeSource) LastTickZ() int            { return -1 }
func (f *fakeSource) LastLevel() int            { return -1 }
func (f *fakeSource) OriginX() int              { return f.originX }
func (f *fakeSource) OriginZ() int              { return f.originZ }

// bytesWritten returns all bytes written to the packet so far. The Packet type
// appends writes to Data (growing it via len(Data)) while Pos is the read
// cursor. For a freshly-constructed write-only Packet, Pos is 0 and all
// written bytes live in Data.
func bytesWritten(p *packet.Packet) []byte { return p.Data }

func TestAnimPayload(t *testing.T) {
	p := &fakeSource{masks: MaskAnim, animID: 0x1234, animDelay: 5}
	buf := packet.NewPacket(nil)
	writeMaskPayloads(buf, p, MaskAnim, false)
	got := bytesWritten(buf)
	// ANIM: p2(0x1234) p1_alt3(5) = [0x12, 0x34, 0xfb]  (0xfb = (-5)&0xff)
	want := []byte{0x12, 0x34, 0xfb}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("byte[%d]: got %#x, want %#x (full=%v)", i, got[i], want[i], got)
			break
		}
	}
}

func TestFaceCoordPayload(t *testing.T) {
	p := &fakeSource{masks: MaskFaceCoord, faceSquareX: 0x0182, faceSquareZ: 0x0184}
	buf := packet.NewPacket(nil)
	writeMaskPayloads(buf, p, MaskFaceCoord, false)
	got := bytesWritten(buf)
	want := []byte{0x01, 0x82, 0x01, 0x84}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("byte[%d]: got %#x, want %#x", i, got[i], want[i])
		}
	}
}

func TestAppearancePayload(t *testing.T) {
	p := &fakeSource{masks: MaskAppearance, appearance: []byte{1, 2, 3}}
	buf := packet.NewPacket(nil)
	writeMaskPayloads(buf, p, MaskAppearance, false)
	got := bytesWritten(buf)
	// rev 225 sends appearance as plain pdata (no +128 scrambling) — client's
	// PlayerEntity.read uses plain g1/g2 and relies on the empty-slot sentinel
	// 0x00 being transmitted literally. See the T2 crash reproducer in the
	// earlier commit message for this file.
	want := []byte{3, 1, 2, 3}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("byte[%d]: got %#x, want %#x", i, got[i], want[i])
		}
	}
}

func TestChatPayload(t *testing.T) {
	p := &fakeSource{masks: MaskChat, chatColour: 1, chatEffect: 2, chatRights: 3, chatBytes: []byte("yo")}
	buf := packet.NewPacket(nil)
	writeMaskPayloads(buf, p, MaskChat, false)
	got := bytesWritten(buf)
	// p1(1) p1(2) p1_alt2(3)=125 p1_alt1(len=2)=130 pdata_alt2("yo")
	// 'y'=121, 128-121=7; 'o'=111, 128-111=17
	want := []byte{1, 2, 125, 130, 7, 17}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("byte[%d]: got %#x, want %#x (full=%v)", i, got[i], want[i], got)
			break
		}
	}
}

func TestDamagePayload(t *testing.T) {
	p := &fakeSource{masks: MaskDamage, damageAmt: 10, damageType: 1, curHP: 40, baseHP: 50}
	buf := packet.NewPacket(nil)
	writeMaskPayloads(buf, p, MaskDamage, false)
	got := bytesWritten(buf)
	// p1_alt1(10)=138 p1_alt3(1)=255 p1_alt2(40)=88 p1(50)
	want := []byte{138, 255, 88, 50}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("byte[%d]: got %#x, want %#x", i, got[i], want[i])
		}
	}
}

func TestMaskHeaderSmall(t *testing.T) {
	buf := packet.NewPacket(nil)
	writeMaskHeader(buf, MaskAnim|MaskFaceCoord)
	if bytesWritten(buf)[0] != 34 {
		t.Errorf("header byte: got %d, want 34 (2|32)", bytesWritten(buf)[0])
	}
}

func TestMaskHeaderLarge(t *testing.T) {
	buf := packet.NewPacket(nil)
	writeMaskHeader(buf, MaskAnim|MaskSpotAnim) // 258
	// Should write IP2(258|128) = IP2(386) little-endian = [0x82, 0x01]
	got := bytesWritten(buf)
	if got[0] != 0x82 || got[1] != 0x01 {
		t.Errorf("large header: got %v, want [0x82 0x01]", got)
	}
}
