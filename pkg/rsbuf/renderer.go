package rsbuf

import "github.com/zsrv/goscape/pkg/io/packet"

// Renderer caches per-slot mask-payload byte slices for the current tick.
// ComputePlayers must run once per tick before any encoder reads.
type Renderer struct {
	highDef     [2048][]byte
	lowDefFull  [2048][]byte // includes forced APPEARANCE + FACE_COORD
	lowDefNoApp [2048][]byte // forces FACE_COORD but NOT APPEARANCE
}

// NewRenderer returns an empty renderer.
func NewRenderer() *Renderer { return &Renderer{} }

// ComputePlayers builds the three per-slot caches for the current tick.
func (r *Renderer) ComputePlayers(players []PlayerSource) {
	for _, p := range players {
		slot := p.Slot()
		if slot < 1 || slot >= len(r.highDef) {
			continue
		}
		masks := p.Masks()
		zeroMask := masks == 0 && p.EntityMask() == 0

		if zeroMask {
			r.highDef[slot] = nil
			r.lowDefFull[slot] = nil
		} else {
			r.highDef[slot] = buildPayload(p, masks, true)

			fullMasks := masks | MaskAppearance | MaskFaceCoord
			r.lowDefFull[slot] = buildPayload(p, fullMasks, true)
		}

		// lowDefNoApp always includes FACE_COORD so newly-visible players
		// whose appearance is cached client-side still get a face target.
		noAppMasks := (masks | MaskFaceCoord) &^ MaskAppearance
		r.lowDefNoApp[slot] = buildPayload(p, noAppMasks, true)
	}
}

// HighDefOf returns the high-def mask payload bytes (nil if no masks).
func (r *Renderer) HighDefOf(slot int) []byte {
	if slot < 1 || slot >= len(r.highDef) {
		return nil
	}
	return r.highDef[slot]
}

// LowDefFullOf returns the low-def payload bytes including APPEARANCE.
func (r *Renderer) LowDefFullOf(slot int) []byte {
	if slot < 1 || slot >= len(r.lowDefFull) {
		return nil
	}
	return r.lowDefFull[slot]
}

// LowDefNoAppOf returns the low-def payload bytes WITHOUT APPEARANCE.
func (r *Renderer) LowDefNoAppOf(slot int) []byte {
	if slot < 1 || slot >= len(r.lowDefNoApp) {
		return nil
	}
	return r.lowDefNoApp[slot]
}

func buildPayload(p PlayerSource, masks int, suppressChat bool) []byte {
	buf := packet.NewPacket(nil)
	writeMaskHeader(buf, masks)
	writeMaskPayloads(buf, p, masks, suppressChat)
	// packet.Packet writes append to Data; Pos is the read cursor and stays 0.
	return append([]byte(nil), buf.Data...)
}
