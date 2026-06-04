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
	damage2Amt, damage2Type                        int
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
func (f *fakeSource) Active() bool            { return true }
func (f *fakeSource) Visibility() Visibility  { return VisibilityDefault }
func (f *fakeSource) StaffModLevel() int32    { return 0 }
func (f *fakeSource) Masks() int              { return f.masks }
func (f *fakeSource) EntityMask() int         { return f.entityMask }
func (f *fakeSource) AppearanceBytes() []byte { return f.appearance }
func (f *fakeSource) AnimID() int             { return f.animID }
func (f *fakeSource) AnimDelay() int          { return f.animDelay }
func (f *fakeSource) FaceEntity() int         { return f.faceEntity }
func (f *fakeSource) SayText() []byte         { return f.sayText }
func (f *fakeSource) DamageAmt() int          { return f.damageAmt }
func (f *fakeSource) DamageType() int         { return f.damageType }
func (f *fakeSource) Damage2Amt() int         { return f.damage2Amt }
func (f *fakeSource) Damage2Type() int        { return f.damage2Type }
func (f *fakeSource) CurHP() int              { return f.curHP }
func (f *fakeSource) BaseHP() int             { return f.baseHP }
func (f *fakeSource) FaceSquareX() int        { return f.faceSquareX }
func (f *fakeSource) FaceSquareZ() int        { return f.faceSquareZ }
func (f *fakeSource) ChatColour() int         { return f.chatColour }
func (f *fakeSource) ChatEffect() int         { return f.chatEffect }
func (f *fakeSource) ChatRights() int         { return f.chatRights }
func (f *fakeSource) ChatBytes() []byte       { return f.chatBytes }
func (f *fakeSource) SpotAnimID() int         { return f.spotanimID }
func (f *fakeSource) SpotAnimHeight() int     { return f.spotanimHeight }
func (f *fakeSource) SpotAnimDelay() int      { return f.spotanimDelay }
func (f *fakeSource) ExactStartX() int        { return f.exactStartX }
func (f *fakeSource) ExactStartZ() int        { return f.exactStartZ }
func (f *fakeSource) ExactEndX() int          { return f.exactEndX }
func (f *fakeSource) ExactEndZ() int          { return f.exactEndZ }
func (f *fakeSource) ExactBegin() int         { return f.exactBegin }
func (f *fakeSource) ExactFinish() int        { return f.exactFinish }
func (f *fakeSource) ExactDir() int           { return f.exactDir }
func (f *fakeSource) WalkDir() int            { return -1 }
func (f *fakeSource) RunDir() int             { return -1 }
func (f *fakeSource) Tele() bool              { return f.tele }
func (f *fakeSource) Jump() bool              { return f.jump }
func (f *fakeSource) LastTickX() int          { return -1 }
func (f *fakeSource) LastTickZ() int          { return -1 }
func (f *fakeSource) LastLevel() int          { return -1 }
func (f *fakeSource) OriginX() int            { return f.originX }
func (f *fakeSource) OriginZ() int            { return f.originZ }

// bytesWritten returns all bytes written to the packet so far. The Packet type
// appends writes to Data (growing it via len(Data)) while Pos is the read
// cursor. For a freshly-constructed write-only Packet, Pos is 0 and all
// written bytes live in Data.
func bytesWritten(p *packet.Packet) []byte { return p.Data }

func TestAnimPayload(t *testing.T) {
	p := &fakeSource{masks: MaskAnim, animID: 0x1234, animDelay: 5}
	buf := packet.NewPacket(nil)
	writeMaskPayloads(buf, p, MaskAnim)
	got := bytesWritten(buf)
	// ANIM: p2(0x1234) p1(5) = [0x12, 0x34, 0x05]
	want := []byte{0x12, 0x34, 0x05}
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
	writeMaskPayloads(buf, p, MaskFaceCoord)
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
	writeMaskPayloads(buf, p, MaskAppearance)
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
	writeMaskPayloads(buf, p, MaskChat)
	got := bytesWritten(buf)
	// p1(1) p1(2) p1(3) p1(len=2) pdata("yo") = 'y'=0x79, 'o'=0x6f
	want := []byte{0x01, 0x02, 0x03, 0x02, 0x79, 0x6f}
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
	writeMaskPayloads(buf, p, MaskDamage)
	got := bytesWritten(buf)
	// p1(10) p1(1) p1(40) p1(50)
	want := []byte{0x0a, 0x01, 0x28, 0x32}
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

// TestWriteMaskPayloads_CanonicalOrder pins the canonical rsbuf write order
// (info.rs:362-404): APPEARANCE -> ANIM -> FACE_ENTITY -> SAY -> DAMAGE ->
// FACE_COORD -> CHAT -> SPOT_ANIM -> EXACT_MOVE -> DAMAGE2 (244: appended
// last). Java client getPlayerExtended reads in the same order.
//
// This is the regression pin for NAI-32 Bundle 3 Stage 4: pre-fix, goscape
// wrote ANIM before APPEARANCE, FACE_COORD before APPEARANCE, etc., causing
// the 2-client smoke crash (pos:320 psize:114, 206-byte over-read in
// getPlayerExtended's appearance-first read consuming a FACE_COORD byte
// as appearance length).
func TestWriteMaskPayloads_CanonicalOrder(t *testing.T) {
	// MaskAppearance (0x1) + MaskFaceCoord (0x20) — exact mask combo that
	// triggered the 2-client smoke crash. Canonical order writes Appearance
	// first (1 byte length + 3 bytes data = 4 bytes), then FaceCoord (4 bytes).
	p := &fakeSource{
		masks:       MaskAppearance | MaskFaceCoord,
		appearance:  []byte{0xaa, 0xbb, 0xcc},
		faceSquareX: 0x1234,
		faceSquareZ: 0x5678,
	}
	buf := packet.NewPacket(nil)
	writeMaskPayloads(buf, p, MaskAppearance|MaskFaceCoord)

	// Expected: Appearance(len=3, 0xaa, 0xbb, 0xcc) + FaceCoord(P2(0x1234), P2(0x5678))
	// = [0x03, 0xaa, 0xbb, 0xcc, 0x12, 0x34, 0x56, 0x78]
	want := []byte{0x03, 0xaa, 0xbb, 0xcc, 0x12, 0x34, 0x56, 0x78}
	got := bytesWritten(buf)
	if len(got) != len(want) {
		t.Fatalf("byte length: got %d, want %d; bytes %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("byte[%d]: got 0x%02x, want 0x%02x (full=%#v)", i, got[i], want[i], got)
		}
	}
}

// TestMaskDamage2BitValue pins MaskDamage2 == 0x400 (rsbuf 244 prot.rs DAMAGE2=0x400).
func TestMaskDamage2BitValue(t *testing.T) {
	if MaskDamage2 != 0x400 {
		t.Errorf("MaskDamage2: got 0x%x, want 0x400", MaskDamage2)
	}
}

// TestDamage2Payload pins the DAMAGE2 wire encoding: p1(amt) p1(type) p1(cur) p1(base),
// identical to DAMAGE (rsbuf 244 renderer.rs PlayerInfoDamage::new with damage_taken2/damage_type2).
func TestDamage2Payload(t *testing.T) {
	p := &fakeSource{masks: MaskDamage2, damage2Amt: 5, damage2Type: 2, curHP: 30, baseHP: 45}
	buf := packet.NewPacket(nil)
	writeMaskPayloads(buf, p, MaskDamage2)
	got := bytesWritten(buf)
	// p1(5) p1(2) p1(30) p1(45)
	want := []byte{0x05, 0x02, 0x1e, 0x2d}
	if len(got) != len(want) {
		t.Fatalf("length: got %d, want %d (bytes=%#v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("byte[%d]: got %#x, want %#x", i, got[i], want[i])
		}
	}
}

// TestPlayerDamage2WrittenLast pins DAMAGE2 as STRICTLY LAST in the wire stream —
// after EXACT_MOVE, which is the immediately-preceding writer in canonical order
// (rsbuf 244 info.rs:362-404). Composing MaskDamage|MaskFaceCoord|MaskSpotAnim|
// MaskExactMove|MaskDamage2 ensures a regression placing writeDamage2 between
// FACE_COORD and CHAT (or anywhere before EXACT_MOVE) will fail the byte-exact check.
//
// Canonical order for this mask set (ascending bit-value):
//
//	DAMAGE      [bits 0x10] → p1(amt)  p1(type)  p1(curHP)  p1(baseHP)      4 bytes
//	FACE_COORD  [bits 0x20] → p2(faceX) p2(faceZ)                            4 bytes
//	SPOT_ANIM   [bits 0x100]→ p2(id) p4((height<<16)|delay)                  6 bytes
//	EXACT_MOVE  [bits 0x200]→ p1(sx) p1(sz) p1(ex) p1(ez) p2(begin) p2(fin) p1(dir)  9 bytes
//	DAMAGE2     [bits 0x400]→ p1(amt2) p1(type2) p1(curHP) p1(baseHP)        4 bytes  ← LAST
//
// originX=originZ=3200 → localOrigin = ((3200>>3)-6)<<3 = 3152.
// exactStart{X,Z}=3160 → offset 8; exactEnd{X,Z}=3168 → offset 16.
func TestPlayerDamage2WrittenLast(t *testing.T) {
	p := &fakeSource{
		masks: MaskDamage | MaskFaceCoord | MaskSpotAnim | MaskExactMove | MaskDamage2,
		// DAMAGE
		damageAmt: 10, damageType: 1, curHP: 40, baseHP: 50,
		// FACE_COORD
		faceSquareX: 0x0182, faceSquareZ: 0x0184,
		// SPOT_ANIM
		spotanimID: 7, spotanimHeight: 3, spotanimDelay: 4,
		// EXACT_MOVE
		originX: 3200, originZ: 3200,
		exactStartX: 3160, exactStartZ: 3160,
		exactEndX: 3168, exactEndZ: 3168,
		exactBegin: 10, exactFinish: 20, exactDir: 3,
		// DAMAGE2
		damage2Amt: 5, damage2Type: 2,
	}
	buf := packet.NewPacket(nil)
	writeMaskPayloads(buf, p, MaskDamage|MaskFaceCoord|MaskSpotAnim|MaskExactMove|MaskDamage2)
	got := bytesWritten(buf)

	// DAMAGE:     p1(10)  p1(1)   p1(40)  p1(50)
	// FACE_COORD: p2(0x0182) p2(0x0184)
	// SPOT_ANIM:  p2(7)   p4((3<<16)|4 = 0x00030004)
	// EXACT_MOVE: p1(8) p1(8) p1(16) p1(16) p2(10) p2(20) p1(3)
	// DAMAGE2:    p1(5)   p1(2)   p1(40)  p1(50)           ← LAST
	want := []byte{
		// DAMAGE (4 bytes)
		0x0a, 0x01, 0x28, 0x32,
		// FACE_COORD (4 bytes)
		0x01, 0x82, 0x01, 0x84,
		// SPOT_ANIM (6 bytes): p2(7)=0x00,0x07; p4(0x00030004)=0x00,0x03,0x00,0x04
		0x00, 0x07, 0x00, 0x03, 0x00, 0x04,
		// EXACT_MOVE (9 bytes): sx=8,sz=8,ex=16,ez=16,begin=10,finish=20,dir=3
		0x08, 0x08, 0x10, 0x10, 0x00, 0x0a, 0x00, 0x14, 0x03,
		// DAMAGE2 (4 bytes) — strictly LAST
		0x05, 0x02, 0x28, 0x32,
	}
	if len(got) != len(want) {
		t.Fatalf("length: got %d, want %d (bytes=%#v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("byte[%d]: got %#x, want %#x (full=%#v)", i, got[i], want[i], got)
		}
	}
}

// TestBuildPayload_HeaderPayloadConsistent_ChatStripped pins the
// invariant that buildPayload's chat-strip path produces a header
// AND payload that are mutually consistent: when CHAT is stripped
// from the body, the header byte must NOT advertise the CHAT bit.
//
// Without the fix at buildPayload (`if suppressChat { masks &^= MaskChat }`),
// writeMaskHeader writes the CHAT bit but writeMaskPayloads omits the
// CHAT body — the receiving client mis-parses (reads CHAT header bit,
// expects body, consumes the next player's bytes). NAI-32 surfaces and
// retires this latent bug.
func TestBuildPayload_HeaderPayloadConsistent_ChatStripped(t *testing.T) {
	p := &fakeSource{
		masks:      MaskChat | MaskAnim,
		animID:     0x1234,
		animDelay:  5,
		chatColour: 1,
		chatEffect: 2,
		chatRights: 3,
		chatBytes:  []byte("hello"),
	}
	out := buildPayload(p, MaskChat|MaskAnim, true)

	// MaskAnim = 0x2, MaskChat = 0x40. Sum = 0x42 < 0x100 → 1-byte header.
	// After strip, header byte should be MaskAnim only = 0x2.
	if len(out) == 0 {
		t.Fatalf("buildPayload returned empty; expected at least header byte")
	}
	if out[0]&byte(MaskChat) != 0 {
		t.Errorf("header has CHAT bit set: got 0x%02x; want CHAT bit clear", out[0])
	}
	if out[0] != byte(MaskAnim) {
		t.Errorf("header byte: got 0x%02x, want 0x%02x (MaskAnim only)", out[0], MaskAnim)
	}

	// Payload must be anim-only: P2(0x1234) + P1(5) (per existing TestAnimPayload).
	want := []byte{byte(MaskAnim), 0x12, 0x34, 0x05}
	if len(out) != len(want) {
		t.Fatalf("payload length: got %d, want %d (header + 3 anim bytes); bytes %#v", len(out), len(want), out)
	}
	for i := range want {
		if out[i] != want[i] {
			t.Errorf("byte[%d]: got 0x%02x, want 0x%02x (full=%#v)", i, out[i], want[i], out)
		}
	}
}
