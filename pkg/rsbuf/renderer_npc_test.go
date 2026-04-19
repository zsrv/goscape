package rsbuf

import "testing"

func TestComputeNpcsHighDef(t *testing.T) {
	r := NewRenderer()
	n := &fakeNpcSource{nid: 5, masks: NpcMaskAnim, animID: 100, animDelay: 2, active: true}
	r.ComputeNpcs([]NpcSource{n})
	got := r.NpcHighDefOf(5)
	// header NpcMaskAnim=2, then ANIM payload p2(100) p1(2) = [2, 0, 100, 2]
	want := []byte{2, 0, 100, 2}
	if len(got) != len(want) {
		t.Fatalf("len: got %d, want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("byte[%d]: got %#x, want %#x", i, got[i], want[i])
		}
	}
}

func TestComputeNpcsLowDefForcesFaceCoord(t *testing.T) {
	r := NewRenderer()
	n := &fakeNpcSource{nid: 5, masks: 0, faceSquareX: 100, faceSquareZ: 200, active: true}
	r.ComputeNpcs([]NpcSource{n})
	low := r.NpcLowDefOf(5)
	if len(low) == 0 {
		t.Fatal("low-def should include FACE_COORD")
	}
	if low[0]&NpcMaskFaceCoord == 0 {
		t.Errorf("header byte: FACE_COORD bit should be set; got %d", low[0])
	}
}

func TestComputeNpcsHighDefSkipsZero(t *testing.T) {
	r := NewRenderer()
	n := &fakeNpcSource{nid: 5, masks: 0, entityMask: 0, active: true}
	r.ComputeNpcs([]NpcSource{n})
	if r.NpcHighDefOf(5) != nil {
		t.Error("HighDef should be nil for zero-mask")
	}
}
