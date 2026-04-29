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
// (info.rs:362-401, ascending bit-value): APPEARANCE -> ANIM -> FACE_ENTITY ->
// SAY -> DAMAGE -> FACE_COORD -> CHAT -> SPOT_ANIM -> EXACT_MOVE. Java client
// getPlayerExtended (client.java:10444-10559) reads in the same order.
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
