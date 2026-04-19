package rsbuf

import "testing"

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
	// header=MaskAnim=2 (1 byte), then p2(100) p1_alt3(2) = [0x00, 0x64, (-2)&0xff=0xfe]
	// = [2, 0, 100, 254]
	want := []byte{2, 0, 100, 254}
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
