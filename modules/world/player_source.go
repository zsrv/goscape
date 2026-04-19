package world

import (
	"hash/fnv"

	"github.com/zsrv/goscape/pkg/rsbuf"
)

// NOTE: Slot() and Coords() are already defined elsewhere on *Player (from sub-spec 2).

func (p *Player) Active() bool                 { return p.active }
func (p *Player) Visibility() rsbuf.Visibility { return p.visibility }
func (p *Player) StaffModLevel() int32         { return p.staffModLevel }

func (p *Player) Masks() int             { return p.masks }
func (p *Player) EntityMask() int        { return p.entitymask }
func (p *Player) AppearanceBytes() []byte { return p.appearanceBuf }

func (p *Player) AppearanceHash() uint64 {
	h := fnv.New64a()
	h.Write(p.appearanceBuf)
	return h.Sum64()
}

// Mask payload accessors.
func (p *Player) AnimID() int         { return p.animID }
func (p *Player) AnimDelay() int      { return p.animDelay }
func (p *Player) FaceEntity() int     { return p.faceEntity }
func (p *Player) SayText() []byte     { return p.sayText }
func (p *Player) DamageAmt() int      { return p.damageAmt }
func (p *Player) DamageType() int     { return p.damageType }
func (p *Player) CurHP() int          { return p.curHP }
func (p *Player) BaseHP() int         { return p.baseHP }
func (p *Player) FaceSquareX() int    { return p.faceSquareX }
func (p *Player) FaceSquareZ() int    { return p.faceSquareZ }
func (p *Player) ChatColour() int     { return p.chatColour }
func (p *Player) ChatEffect() int     { return p.chatEffect }
func (p *Player) ChatRights() int     { return p.chatRights }
func (p *Player) ChatBytes() []byte   { return p.chatBytes }
func (p *Player) SpotAnimID() int     { return p.spotanimID }
func (p *Player) SpotAnimHeight() int { return p.spotanimHeight }
func (p *Player) SpotAnimDelay() int  { return p.spotanimDelay }
func (p *Player) ExactStartX() int    { return p.exactStartX }
func (p *Player) ExactStartZ() int    { return p.exactStartZ }
func (p *Player) ExactEndX() int      { return p.exactEndX }
func (p *Player) ExactEndZ() int      { return p.exactEndZ }
func (p *Player) ExactBegin() int     { return p.exactBegin }
func (p *Player) ExactFinish() int    { return p.exactFinish }
func (p *Player) ExactDir() int       { return p.exactDir }

// Movement.
func (p *Player) WalkDir() int   { return p.walkDir }
func (p *Player) RunDir() int    { return p.runDir }
func (p *Player) Tele() bool     { return p.tele }
func (p *Player) Jump() bool     { return p.jump }
func (p *Player) LastTickX() int { return p.lastTickX }
func (p *Player) LastTickZ() int { return p.lastTickZ }
func (p *Player) LastLevel() int { return p.lastLevel }
func (p *Player) OriginX() int   { return p.originX }
func (p *Player) OriginZ() int   { return p.originZ }

// Compile-time check that *Player satisfies rsbuf.PlayerSource.
var _ rsbuf.PlayerSource = (*Player)(nil)
