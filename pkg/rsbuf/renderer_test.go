package rsbuf

import (
	"bytes"
	"testing"
)

func TestComputePlayersSkipsZeroMask(t *testing.T) {
	r := NewRenderer()
	p := &fakeSource{slot: 5, masks: 0, entityMask: 0}
	r.ComputePlayers([]PlayerSource{p})
	if r.HighDefOf(5) != nil {
		t.Error("HighDefOf(zero-mask) should be nil")
	}
	if r.LowDefFullOf(5) != nil {
		t.Error("LowDefFullOf(zero-mask) should be nil")
	}
}

func TestComputePlayersHighDef(t *testing.T) {
	r := NewRenderer()
	p := &fakeSource{slot: 5, masks: MaskAnim, animID: 100, animDelay: 2}
	r.ComputePlayers([]PlayerSource{p})
	got := r.HighDefOf(5)
	// header=MaskAnim=2 (1 byte), then p2(100) p1(2) = [0x00, 0x64, 0x02]
	// = [2, 0, 100, 2]
	want := []byte{2, 0, 100, 2}
	if len(got) != len(want) {
		t.Fatalf("length: got %d, want %d (bytes=%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("byte[%d]: got %#x, want %#x", i, got[i], want[i])
		}
	}
}

func TestComputePlayersLowDefForcesAppearance(t *testing.T) {
	r := NewRenderer()
	p := &fakeSource{
		slot: 5, masks: 0, entityMask: MaskFaceCoord,
		appearance:  []byte{1, 2, 3},
		faceSquareX: 100, faceSquareZ: 200,
	}
	r.ComputePlayers([]PlayerSource{p})
	lowFull := r.LowDefFullOf(5)
	if len(lowFull) == 0 {
		t.Fatal("LowDefFullOf should include APPEARANCE + FACE_COORD")
	}
	// First byte: mask header with APPEARANCE|FACE_COORD = 1|32 = 33.
	if lowFull[0] != 33 {
		t.Errorf("header byte: got %d, want 33 (APPEARANCE|FACE_COORD)", lowFull[0])
	}
}

func TestComputePlayersLowDefNoApp(t *testing.T) {
	r := NewRenderer()
	p := &fakeSource{slot: 5, masks: 0, entityMask: 0, faceSquareX: 100, faceSquareZ: 200}
	r.ComputePlayers([]PlayerSource{p})
	lowNo := r.LowDefNoAppOf(5)
	if len(lowNo) == 0 {
		t.Fatal("LowDefNoAppOf should include FACE_COORD at minimum")
	}
	if lowNo[0]&MaskAppearance != 0 {
		t.Errorf("header byte: APPEARANCE bit should be unset in lowDefNoApp; got %d", lowNo[0])
	}
	if lowNo[0]&MaskFaceCoord == 0 {
		t.Errorf("header byte: FACE_COORD bit should be set; got %d", lowNo[0])
	}
}

// TestComputePlayers_DualHighDef_ChatPresent pins the dual-cache
// contract for a player with CHAT in their masks: HighDefOf yields
// chat-stripped bytes (correct for self-read at writeLocalPlayer),
// HighDefWithChatOf yields chat-preserved bytes (correct for
// tracked-other read at writePlayers).
func TestComputePlayers_DualHighDef_ChatPresent(t *testing.T) {
	p := &fakeSource{
		slot:       5,
		masks:      MaskChat | MaskAnim,
		animID:     0x1234,
		animDelay:  5,
		chatColour: 1,
		chatEffect: 2,
		chatRights: 3,
		chatBytes:  []byte("yo"),
	}
	r := NewRenderer()
	r.ComputePlayers([]PlayerSource{p})

	stripped := r.HighDefOf(5)
	withChat := r.HighDefWithChatOf(5)

	if stripped == nil {
		t.Fatalf("HighDefOf(5) is nil; expected chat-stripped bytes")
	}
	if withChat == nil {
		t.Fatalf("HighDefWithChatOf(5) is nil; expected chat-preserved bytes")
	}

	// Header byte: chat-stripped should be MaskAnim only (0x2);
	// chat-preserved should be MaskAnim | MaskChat (0x42).
	if stripped[0] != byte(MaskAnim) {
		t.Errorf("HighDefOf header: got 0x%02x, want 0x%02x (MaskAnim only)", stripped[0], MaskAnim)
	}
	if withChat[0] != byte(MaskAnim|MaskChat) {
		t.Errorf("HighDefWithChatOf header: got 0x%02x, want 0x%02x (MaskAnim|MaskChat)", withChat[0], MaskAnim|MaskChat)
	}

	// Length: stripped is 1 (header) + 3 (anim) = 4 bytes.
	// With-chat is 4 + 6 (chat: colour + effect + rights + len + 2 chars) = 10 bytes.
	// Per existing TestChatPayload at mask_payload_test.go:122 — chat body for "yo" is
	// p1(1) p1(2) p1(3) p1(2) pdata('y','o')={0x79,0x6f}.
	if len(stripped) != 4 {
		t.Errorf("HighDefOf length: got %d, want 4 (header + anim); bytes %#v", len(stripped), stripped)
	}
	if len(withChat) != 10 {
		t.Errorf("HighDefWithChatOf length: got %d, want 10 (header + anim + chat); bytes %#v", len(withChat), withChat)
	}
}

// TestComputePlayers_DualHighDef_NoChat_Identical pins that the
// dual-cache change does not drift non-CHAT outputs: when masks does
// not include MaskChat, both cache variants are byte-identical.
func TestComputePlayers_DualHighDef_NoChat_Identical(t *testing.T) {
	p := &fakeSource{slot: 5, masks: MaskAnim, animID: 100, animDelay: 2}
	r := NewRenderer()
	r.ComputePlayers([]PlayerSource{p})

	stripped := r.HighDefOf(5)
	withChat := r.HighDefWithChatOf(5)

	if stripped == nil || withChat == nil {
		t.Fatalf("both cache variants must be non-nil for masks=MaskAnim; got stripped=%v, withChat=%v", stripped, withChat)
	}
	if !bytes.Equal(stripped, withChat) {
		t.Errorf("non-CHAT masks should produce byte-identical caches:\nHighDefOf:         %#v\nHighDefWithChatOf: %#v", stripped, withChat)
	}
}

// TestComputePlayers_DualHighDef_MasksZero_BothNil pins the
// no-mask case: both cache variants are nil so encoders take the
// idle path with no orphan mask-header byte.
func TestComputePlayers_DualHighDef_MasksZero_BothNil(t *testing.T) {
	p := &fakeSource{slot: 5, masks: 0, entityMask: 0}
	r := NewRenderer()
	r.ComputePlayers([]PlayerSource{p})

	if r.HighDefOf(5) != nil {
		t.Errorf("HighDefOf(5) for masks=0: got %#v, want nil", r.HighDefOf(5))
	}
	if r.HighDefWithChatOf(5) != nil {
		t.Errorf("HighDefWithChatOf(5) for masks=0: got %#v, want nil", r.HighDefWithChatOf(5))
	}
}
