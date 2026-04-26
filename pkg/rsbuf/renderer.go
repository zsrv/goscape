package rsbuf

import "github.com/zsrv/goscape/pkg/io/packet"

// Renderer caches per-slot mask-payload byte slices for the current tick.
// ComputePlayers must run once per tick before any encoder reads.
type Renderer struct {
	highDef     [2048][]byte
	lowDefFull  [2048][]byte // includes forced APPEARANCE + FACE_COORD
	lowDefNoApp [2048][]byte // forces FACE_COORD but NOT APPEARANCE

	npcHighDef [8192][]byte
	npcLowDef  [8192][]byte // forces FACE_COORD baseline
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

		// High-def carries only the player's live mask updates. When masks
		// is 0, leave highDef nil so the encoder takes the idle path with
		// no extend bit and no orphan mask-header byte leaking into the
		// packet (mirrors TS PlayerRenderer.computeInfo: `if masks === 0
		// return;`, renderer.ts:41-43).
		if masks == 0 {
			r.highDef[slot] = nil
		} else {
			r.highDef[slot] = buildPayload(p, masks, true)
		}

		// Low-def carries baselines for newly-visible players (appearance
		// + face). Kept alive by EntityMask so a player with no live masks
		// but a persistent face-entity target is still fully describable
		// to new observers.
		if masks == 0 && p.EntityMask() == 0 {
			r.lowDefFull[slot] = nil
		} else {
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

// ComputeNpcs builds per-nid NPC payload caches for the current tick.
func (r *Renderer) ComputeNpcs(npcs []NpcSource) {
	for _, n := range npcs {
		nid := n.Nid()
		if nid < 1 || nid >= len(r.npcHighDef) {
			continue
		}
		masks := n.Masks()
		if masks == 0 && n.EntityMask() == 0 {
			r.npcHighDef[nid] = nil
		} else {
			buf := packet.NewPacket(nil)
			writeNpcMaskHeader(buf, masks)
			writeNpcMaskPayloads(buf, n, masks)
			r.npcHighDef[nid] = append([]byte(nil), buf.Data...)
		}
		// Low-def: force FACE_COORD baseline so new observers know where to look.
		lowMasks := masks | NpcMaskFaceCoord
		buf := packet.NewPacket(nil)
		writeNpcMaskHeader(buf, lowMasks)
		writeNpcMaskPayloads(buf, n, lowMasks)
		r.npcLowDef[nid] = append([]byte(nil), buf.Data...)
	}
}

// NpcHighDefOf returns cached NPC high-def bytes (nil if no masks).
func (r *Renderer) NpcHighDefOf(nid int) []byte {
	if nid < 1 || nid >= len(r.npcHighDef) {
		return nil
	}
	return r.npcHighDef[nid]
}

// NpcLowDefOf returns cached NPC low-def bytes (FACE_COORD always included).
func (r *Renderer) NpcLowDefOf(nid int) []byte {
	if nid < 1 || nid >= len(r.npcLowDef) {
		return nil
	}
	return r.npcLowDef[nid]
}

func buildPayload(p PlayerSource, masks int, suppressChat bool) []byte {
	if suppressChat {
		masks &^= MaskChat // CHAT bit stripped per info.rs:289-291; header AND payload omit CHAT
	}
	buf := packet.NewPacket(nil)
	writeMaskHeader(buf, masks)
	writeMaskPayloads(buf, p, masks)
	// packet.Packet writes append to Data; Pos is the read cursor and stays 0.
	return append([]byte(nil), buf.Data...)
}
