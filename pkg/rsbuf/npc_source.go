package rsbuf

// NPC mask bit constants — matches rsbuf branch 225 NpcInfoProt.
const (
	NpcMaskAnim       = 0x2
	NpcMaskFaceEntity = 0x4
	NpcMaskSay        = 0x8
	NpcMaskDamage     = 0x10
	NpcMaskChangeType = 0x20
	NpcMaskSpotAnim   = 0x40
	NpcMaskFaceCoord  = 0x80
)

// NpcSource exposes an NPC's state to the encoder without depending on modules/world.
type NpcSource interface {
	Nid() int
	TypeID() int
	Coords() (x, z, level int)
	Active() bool

	Masks() int
	EntityMask() int

	AnimID() int
	AnimDelay() int
	FaceEntity() int
	SayText() []byte
	DamageAmt() int
	DamageType() int
	CurHP() int
	BaseHP() int
	ChangeTypeID() int
	SpotAnimID() int
	SpotAnimHeight() int
	SpotAnimDelay() int
	FaceSquareX() int
	FaceSquareZ() int

	WalkDir() int
	RunDir() int
	Tele() bool
	LastTickX() int
	LastTickZ() int
	LastLevel() int
}
