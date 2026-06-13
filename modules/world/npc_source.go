package world

import (
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/rsbuf"
)

// Slot already defined in npc.go.

func (n *Npc) Nid() int                  { return n.nid }
func (n *Npc) TypeID() int               { return n.typeId }
func (n *Npc) Coords() (x, z, level int) { return n.x, n.z, n.level }
func (n *Npc) Active() bool              { return !n.dead }
func (n *Npc) Masks() int                { return n.masks }
func (n *Npc) EntityMask() int           { return n.entitymask }

func (n *Npc) AnimID() int         { return n.animID }
func (n *Npc) AnimDelay() int      { return n.animDelay }
func (n *Npc) FaceEntity() int     { return n.faceEntity }
func (n *Npc) SayText() []byte     { return n.sayText }
func (n *Npc) DamageAmt() int      { return n.damageAmt }
func (n *Npc) DamageType() int     { return n.damageType }
func (n *Npc) Damage2Amt() int     { return n.damage2Amt }
func (n *Npc) Damage2Type() int    { return n.damage2Type }
func (n *Npc) CurHP() int          { return n.levels[objtype.NpcStatHitpoints] }
func (n *Npc) BaseHP() int         { return n.baseLevels[objtype.NpcStatHitpoints] }
func (n *Npc) ChangeTypeID() int   { return n.changeTypeID }
func (n *Npc) SpotAnimID() int     { return n.spotanimID }
func (n *Npc) SpotAnimHeight() int { return n.spotanimHeight }
func (n *Npc) SpotAnimDelay() int  { return n.spotanimDelay }

// FaceSquareX/Z feed the rsbuf FACE_COORD mask payload (their only callers).
// They return the EFFECTIVE face coord — the active faceSquare when set, else
// the resting orientation (faceAngle, south on spawn) — so the always-forced
// FACE_COORD low-def orients a fresh NPC south instead of sending the raw
// faceSquare(-1) = wire 65535 = a far-north-east target. The rsbuf renderer
// reads these directly (ComputeNpcs); the buf.go ComputeNpc path is fed the
// same value separately from tick.go.
func (n *Npc) FaceSquareX() int { x, _ := n.effectiveFaceCoord(); return x }
func (n *Npc) FaceSquareZ() int { _, z := n.effectiveFaceCoord(); return z }

func (n *Npc) WalkDir() int   { return n.walkDir }
func (n *Npc) RunDir() int    { return n.runDir }
func (n *Npc) Tele() bool     { return n.tele }
func (n *Npc) Jump() bool     { return n.jump }
func (n *Npc) LastTickX() int { return n.lastTickX }
func (n *Npc) LastTickZ() int { return n.lastTickZ }
func (n *Npc) LastLevel() int { return n.lastLevel }

// Compile-time check.
var _ rsbuf.NpcSource = (*Npc)(nil)
