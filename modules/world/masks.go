package world

// Player update masks — bit flags combined in Player.masks.
// Mirrors @2004scape/rsbuf's PlayerInfoProt enum.
const (
	MaskAppearance = 1
	MaskAnim       = 2
	MaskFaceEntity = 4
	MaskSay        = 8
	MaskDamage     = 16
	MaskFaceCoord  = 32
	MaskChat       = 64
	MaskBigUpdate  = 128
	MaskSpotAnim   = 256
	MaskExactMove  = 512
)

// NPC update masks — mirrors NpcInfoProt.
const (
	NpcMaskAnim       = 2
	NpcMaskFaceEntity = 4
	NpcMaskSay        = 8
	NpcMaskDamage     = 16
	NpcMaskChangeType = 32
	NpcMaskSpotAnim   = 64
	NpcMaskFaceCoord  = 128
)
