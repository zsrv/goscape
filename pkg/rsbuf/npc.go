package rsbuf

// Npc is the per-tick state snapshot that the encoder reads from.
// Mirrors upstream npc.rs Npc (2004scape/rsbuf branch 225,
// src/npc.rs:3-29). Field order matches upstream verbatim.
//
// Concurrency: tick-goroutine-owned. Allocated by *Buf.AddNpc;
// populated by *Buf.ComputeNpc; cleaned up to transient defaults
// by *Buf.Cleanup at end-of-tick.
//
// Observers is a counter — not cleared by cleanup; modified only
// by *Buf.RemovePlayer (decrements for npcs in the player's BuildArea)
// and by NAI-30's encoder (increments on add to a tracking set,
// decrements on remove from a tracking set).
type Npc struct {
	Coord                                  int // pkg/coordgrid.PackCoord(level, x, z)
	NID                                    int32
	NType                                  int32
	Tele                                   bool
	RunDir                                 int8 // -1 sentinel
	WalkDir                                int8 // -1 sentinel
	Active                                 bool
	Masks                                  uint32
	FaceEntity                             int32
	FaceX, FaceZ                           int32
	OrientationX, OrientationZ             int32
	DamageTaken, DamageType                int32
	DamageTaken2, DamageType2              int32 // rsbuf 244 npc.rs:20-21
	CurrentHitpoints, BaseHitpoints        int32
	AnimID, AnimDelay                      int32
	Say                                    *string
	GraphicID, GraphicHeight, GraphicDelay int32
	Observers                              int32
}

// newNpc constructs an Npc at zero-coord with sentinel defaults and
// observer count 0. Mirrors upstream Npc::new at npc.rs:32-60.
func newNpc(nid, ntype int32) *Npc {
	return &Npc{
		Coord:            0,
		NID:              nid,
		NType:            ntype,
		Tele:             false,
		RunDir:           -1,
		WalkDir:          -1,
		Active:           false,
		Masks:            0,
		FaceEntity:       -1,
		FaceX:            -1,
		FaceZ:            -1,
		OrientationX:     -1,
		OrientationZ:     -1,
		DamageTaken:      -1,
		DamageType:       -1,
		DamageTaken2:     -1,
		DamageType2:      -1,
		CurrentHitpoints: -1,
		BaseHitpoints:    -1,
		AnimID:           -1,
		AnimDelay:        -1,
		Say:              nil,
		GraphicID:        -1,
		GraphicHeight:    -1,
		GraphicDelay:     -1,
		Observers:        0,
	}
}

// cleanup zeros transient per-tick state but preserves FaceEntity +
// OrientationX/Z + Observers per upstream npc.rs:62-83 commented-out
// clears.
func (n *Npc) cleanup() {
	n.WalkDir = -1
	n.RunDir = -1
	n.Tele = false
	n.Masks = 0
	// FaceEntity / OrientationX/Z preserved per upstream npc.rs:68-71
	// commented-out clears.
	n.FaceX = -1
	n.FaceZ = -1
	n.DamageTaken = -1
	n.DamageType = -1
	n.DamageTaken2 = -1
	n.DamageType2 = -1
	n.CurrentHitpoints = -1
	n.BaseHitpoints = -1
	n.AnimID = -1
	n.AnimDelay = -1
	n.Say = nil
	n.GraphicID = -1
	n.GraphicHeight = -1
	n.GraphicDelay = -1
	// Observers preserved (persistent counter).
}
