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

// TestComputeNpcsHighDef_PersistentEntityMaskMasksZero pins the NAI-116
// regression: when an NPC has Masks()==0 but EntityMask()!=0 (e.g.
// persistent FaceEntity carried across ticks per pkg/rsbuf/npc.go:74),
// ComputeNpcs MUST produce a nil highDef so the NpcInfo encoder takes
// the idle leaf and emits no orphan 0x00 mask-header byte.
//
// Pre-NAI-116, the renderer wrote writeNpcMaskHeader(buf, 0) → [0x00]
// (1-byte orphan), which the encoder appended to the wire as a Walk/Run/
// Extend leaf payload. Java client opcode 1 (NpcInfo) decoded the leaf,
// read mask=0 with no following payload bytes, and crashed "Error: T2".
//
// Reproducer: Tutorial Island Master Chef has FaceEntity set across the
// cutscene; first tick after walking out where transient masks are clear
// → orphan byte → T2.
func TestComputeNpcsHighDef_PersistentEntityMaskMasksZero(t *testing.T) {
	r := NewRenderer()
	n := &fakeNpcSource{
		nid: 5, masks: 0, entityMask: NpcMaskFaceEntity,
		faceEntity: 12345, active: true,
	}
	r.ComputeNpcs([]NpcSource{n})
	if got := r.NpcHighDefOf(5); got != nil {
		t.Errorf("HighDef should be nil for masks==0 even when EntityMask!=0; got %#v", got)
	}
	// Low-def safety pin: lowMasks always includes NpcMaskFaceCoord, so
	// npcLowDef must always be at least header(1) + FACE_COORD payload(4)
	// = 5 bytes. Pins the doc-comment claim at renderer.go (line below
	// the gate fix).
	low := r.NpcLowDefOf(5)
	if len(low) < 5 {
		t.Errorf("LowDef should include FACE_COORD payload (header+4 bytes); got %#v", low)
	}
}
