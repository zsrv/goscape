package rsbuf

// PlayerSource exposes a player's state to the encoder without a dependency on
// the modules/world Player type. Accessor methods return zero values when the
// corresponding mask bit isn't set.
type PlayerSource interface {
	// identity + lifecycle
	Slot() int
	Coords() (x, z, level int)
	Active() bool
	Visibility() Visibility
	StaffModLevel() int32

	// masks
	Masks() int
	EntityMask() int

	// appearance
	AppearanceBytes() []byte

	// mask payload accessors
	AnimID() int
	AnimDelay() int
	FaceEntity() int
	SayText() []byte
	DamageAmt() int
	DamageType() int
	CurHP() int
	BaseHP() int
	FaceSquareX() int
	FaceSquareZ() int
	ChatColour() int
	ChatEffect() int
	ChatRights() int
	ChatBytes() []byte
	SpotAnimID() int
	SpotAnimHeight() int
	SpotAnimDelay() int
	ExactStartX() int
	ExactStartZ() int
	ExactEndX() int
	ExactEndZ() int
	ExactBegin() int
	ExactFinish() int
	ExactDir() int

	// movement
	WalkDir() int
	RunDir() int
	Tele() bool
	Jump() bool
	LastTickX() int
	LastTickZ() int
	LastLevel() int
	OriginX() int
	OriginZ() int
}
