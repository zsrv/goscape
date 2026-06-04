// Package rsbuf is a pure-Go port of the @2004scape/rsbuf Rust crate's
// PlayerInfo bitstream encoder. Reference: github.com/2004scape/rsbuf branch 225.
package rsbuf

// Visibility controls who can see a player.
type Visibility int

const (
	VisibilityDefault Visibility = iota // normal: everyone sees
	VisibilitySoft                      // only staff sees (admin invisibility)
	VisibilityHard                      // nobody sees (hidden-online, invis-to-all)
)

// PlayerInfo mask bit constants — matches rsbuf branch 244 PlayerInfoProt.
const (
	MaskAppearance = 0x1
	MaskAnim       = 0x2
	MaskFaceEntity = 0x4
	MaskSay        = 0x8
	MaskDamage     = 0x10
	MaskFaceCoord  = 0x20
	MaskChat       = 0x40
	MaskBig        = 0x80
	MaskSpotAnim   = 0x100
	MaskExactMove  = 0x200
	MaskDamage2    = 0x400 // rsbuf 244 prot.rs DAMAGE2 — appended LAST in write order (info.rs:402-404)
)
