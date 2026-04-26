package rsbuf

// Player is the per-tick state snapshot that the encoder reads from.
// Mirrors upstream player.rs Player (2004scape/rsbuf branch 225,
// src/player.rs:5-37). Field order matches upstream for side-by-side
// review.
//
// Concurrency: tick-goroutine-owned. Allocated by *Buf.AddPlayer;
// populated by *Buf.ComputePlayer; cleaned up to transient defaults
// by *Buf.Cleanup at end-of-tick.
//
// Coord encoding: stored as int packed via pkg/coordgrid.PackCoord
// (level, x, z). Layout matches upstream CoordGrid::from at coord.rs:13-19.
type Player struct {
	Coord      int // pkg/coordgrid.PackCoord(level, x, z)
	Origin     int // pkg/coordgrid.PackCoord(level, originX, originZ)
	PID        int32
	Tele       bool
	Jump       bool
	RunDir     int8 // -1 sentinel = no run this tick
	WalkDir    int8 // -1 sentinel = no walk this tick
	Visibility Visibility
	Active     bool
	Build      *BuildArea // populated by *Buf.AddPlayer (Bundle 3)
	Masks      uint32
	Appearance     []byte
	LastAppearance int32
	FaceEntity     int32
	FaceX, FaceZ   int32
	OrientationX, OrientationZ      int32
	DamageTaken, DamageType         int32
	CurrentHitpoints, BaseHitpoints int32
	AnimID, AnimDelay               int32
	Say                             *string // nil = no say this tick
	Chat                            *Chat
	GraphicID, GraphicHeight, GraphicDelay int32
	ExactMove *ExactMove
}

// Chat carries chat-message payload + formatting. Mirrors upstream
// player.rs Chat (src/player.rs:39-45).
type Chat struct {
	Bytes   []byte
	Color   uint8
	Effect  uint8
	Ignored uint8
}

// ExactMove carries exact-move animation parameters. Mirrors upstream
// player.rs ExactMove (src/player.rs:47-56).
type ExactMove struct {
	StartX, StartZ int32
	EndX, EndZ     int32
	Begin, Finish  int32
	Dir            int32
}

// newPlayer constructs a Player at zero-coord with sentinel defaults.
// Mirrors upstream Player::new at player.rs:60-93. Build is nil at
// construction; *Buf.AddPlayer assigns a fresh BuildArea before the
// player becomes addressable to ComputePlayer.
func newPlayer(pid int32) *Player {
	return &Player{
		Coord:            0,
		Origin:           0,
		PID:              pid,
		Tele:             false,
		Jump:             false,
		RunDir:           -1,
		WalkDir:          -1,
		Visibility:       VisibilityDefault,
		Active:           false,
		Build:            nil, // *Buf.AddPlayer fills this in
		Masks:            0,
		Appearance:       nil,
		LastAppearance:   -1,
		FaceEntity:       -1,
		FaceX:            -1,
		FaceZ:            -1,
		OrientationX:     -1,
		OrientationZ:     -1,
		DamageTaken:      -1,
		DamageType:       -1,
		CurrentHitpoints: -1,
		BaseHitpoints:    -1,
		AnimID:           -1,
		AnimDelay:        -1,
		Say:              nil,
		Chat:             nil,
		GraphicID:        -1,
		GraphicHeight:    -1,
		GraphicDelay:     -1,
		ExactMove:        nil,
	}
}

// cleanup zeros transient per-tick state but preserves persistent
// state (Appearance, LastAppearance, FaceEntity, OrientationX,
// OrientationZ) per upstream player.rs:96-121 commented-out lines.
//
// Called by *Buf.Cleanup once per tick after info encoding completes.
func (p *Player) cleanup() {
	p.WalkDir = -1
	p.RunDir = -1
	p.Jump = false
	p.Tele = false
	p.Masks = 0
	// Appearance / LastAppearance / FaceEntity / OrientationX/Z preserved
	// per upstream commented-out clears at player.rs:102-108.
	p.FaceX = -1
	p.FaceZ = -1
	p.DamageTaken = -1
	p.DamageType = -1
	p.CurrentHitpoints = -1
	p.BaseHitpoints = -1
	p.AnimID = -1
	p.AnimDelay = -1
	p.Say = nil
	p.Chat = nil
	p.GraphicID = -1
	p.GraphicHeight = -1
	p.GraphicDelay = -1
	p.ExactMove = nil
}
