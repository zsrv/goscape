package rsbuf

import "github.com/zsrv/goscape/pkg/io/packet"

// writeMaskHeader writes the 1- or 2-byte mask header. If mask value > 0xff,
// MaskBig is OR'd in and the header is IP2 (little-endian u16).
func writeMaskHeader(buf *packet.Packet, masks int) {
	if masks > 0xff {
		buf.IP2(uint16(masks | MaskBig))
	} else {
		buf.P1(uint8(masks))
	}
}

// writeMaskPayloads writes mask payloads in rsbuf's fixed order:
// ANIM -> SAY -> EXACT_MOVE -> FACE_ENTITY -> FACE_COORD -> SPOT_ANIM ->
// APPEARANCE -> DAMAGE -> CHAT.
//
// forceMasks is the effective mask set to write (may differ from p.Masks() for
// low-def variants). suppressChat strips CHAT from the output.
func writeMaskPayloads(buf *packet.Packet, p PlayerSource, forceMasks int, suppressChat bool) {
	if forceMasks&MaskAnim != 0 {
		writeAnim(buf, p)
	}
	if forceMasks&MaskSay != 0 {
		writeSay(buf, p)
	}
	if forceMasks&MaskExactMove != 0 {
		writeExactMove(buf, p)
	}
	if forceMasks&MaskFaceEntity != 0 {
		writeFaceEntity(buf, p)
	}
	if forceMasks&MaskFaceCoord != 0 {
		writeFaceCoord(buf, p)
	}
	if forceMasks&MaskSpotAnim != 0 {
		writeSpotAnim(buf, p)
	}
	if forceMasks&MaskAppearance != 0 {
		writeAppearance(buf, p)
	}
	if forceMasks&MaskDamage != 0 {
		writeDamage(buf, p)
	}
	if forceMasks&MaskChat != 0 && !suppressChat {
		writeChat(buf, p)
	}
}

func writeAnim(buf *packet.Packet, p PlayerSource) {
	buf.P2(uint16(p.AnimID()))
	buf.P1Alt3(uint8(p.AnimDelay()))
}

func writeSay(buf *packet.Packet, p PlayerSource) {
	for _, b := range p.SayText() {
		buf.P1(b)
	}
	buf.P1(10) // line-feed terminator
}

func writeExactMove(buf *packet.Packet, p PlayerSource) {
	localOrigin := ((p.OriginX() >> 3) - 6) << 3
	localZOrigin := ((p.OriginZ() >> 3) - 6) << 3
	buf.P1Alt1(uint8(p.ExactStartX() - localOrigin))
	buf.P1Alt2(uint8(p.ExactStartZ() - localZOrigin))
	buf.P1Alt3(uint8(p.ExactEndX() - localOrigin))
	buf.P1(uint8(p.ExactEndZ() - localZOrigin))
	buf.P2(uint16(p.ExactBegin()))
	buf.P2Alt2(uint16(p.ExactFinish()))
	buf.P1(uint8(p.ExactDir()))
}

func writeFaceEntity(buf *packet.Packet, p PlayerSource) {
	buf.P2Alt2(uint16(p.FaceEntity()))
}

func writeFaceCoord(buf *packet.Packet, p PlayerSource) {
	buf.P2(uint16(p.FaceSquareX()))
	buf.P2(uint16(p.FaceSquareZ()))
}

func writeSpotAnim(buf *packet.Packet, p PlayerSource) {
	buf.P2Alt2(uint16(p.SpotAnimID()))
	buf.P4Alt2(uint32(p.SpotAnimHeight())<<16 | uint32(p.SpotAnimDelay()))
}

func writeAppearance(buf *packet.Packet, p PlayerSource) {
	app := p.AppearanceBytes()
	buf.P1(uint8(len(app)))
	buf.PData(app)
}

func writeDamage(buf *packet.Packet, p PlayerSource) {
	buf.P1Alt1(uint8(p.DamageAmt()))
	buf.P1Alt3(uint8(p.DamageType()))
	buf.P1Alt2(uint8(p.CurHP()))
	buf.P1(uint8(p.BaseHP()))
}

func writeChat(buf *packet.Packet, p PlayerSource) {
	buf.P1(uint8(p.ChatColour()))
	buf.P1(uint8(p.ChatEffect()))
	buf.P1Alt2(uint8(p.ChatRights()))
	buf.P1Alt1(uint8(len(p.ChatBytes())))
	buf.PDataAlt2(p.ChatBytes())
}
