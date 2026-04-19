package rsbuf

import "github.com/zsrv/goscape/pkg/io/packet"

// writeNpcMaskHeader writes the 1-byte NPC mask header. NPC mask values fit in
// 0x80, so no MaskBig equivalent is needed.
func writeNpcMaskHeader(buf *packet.Packet, masks int) {
	buf.P1(uint8(masks))
}

// writeNpcMaskPayloads writes payloads in fixed order:
// ANIM → FACE_ENTITY → SAY → DAMAGE → CHANGE_TYPE → SPOT_ANIM → FACE_COORD
// All straight big-endian (no alt-byte variants).
func writeNpcMaskPayloads(buf *packet.Packet, n NpcSource, forceMasks int) {
	if forceMasks&NpcMaskAnim != 0 {
		buf.P2(uint16(n.AnimID()))
		buf.P1(uint8(n.AnimDelay()))
	}
	if forceMasks&NpcMaskFaceEntity != 0 {
		buf.P2(uint16(n.FaceEntity()))
	}
	if forceMasks&NpcMaskSay != 0 {
		for _, b := range n.SayText() {
			buf.P1(b)
		}
		buf.P1(10)
	}
	if forceMasks&NpcMaskDamage != 0 {
		buf.P1(uint8(n.DamageAmt()))
		buf.P1(uint8(n.DamageType()))
		buf.P1(uint8(n.CurHP()))
		buf.P1(uint8(n.BaseHP()))
	}
	if forceMasks&NpcMaskChangeType != 0 {
		buf.P2(uint16(n.ChangeTypeID()))
	}
	if forceMasks&NpcMaskSpotAnim != 0 {
		buf.P2(uint16(n.SpotAnimID()))
		buf.P4(uint32(n.SpotAnimHeight())<<16 | uint32(n.SpotAnimDelay()))
	}
	if forceMasks&NpcMaskFaceCoord != 0 {
		buf.P2(uint16(n.FaceSquareX()))
		buf.P2(uint16(n.FaceSquareZ()))
	}
}
