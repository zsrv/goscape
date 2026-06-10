package rsbuf

import "github.com/zsrv/goscape/pkg/io/packet"

// Renderer caches per-slot mask-payload byte slices for the current tick.
// ComputePlayers must run once per tick before any encoder reads.
type Renderer struct {
	highDef         [2048][]byte // CHAT stripped from header AND payload (consumed by writeLocalPlayer for self per info.rs:289-291)
	highDefWithChat [2048][]byte // CHAT preserved (consumed by writePlayers for tracked others)
	lowDefFull      [2048][]byte // includes forced APPEARANCE + FACE_COORD
	lowDefNoApp     [2048][]byte // forces FACE_COORD but NOT APPEARANCE

	npcHighDef [16384][]byte
	npcLowDef  [16384][]byte // forces FACE_COORD baseline
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
			r.highDefWithChat[slot] = nil
		} else {
			r.highDef[slot] = buildPayload(p, masks, true)          // CHAT stripped (self per info.rs:289-291)
			r.highDefWithChat[slot] = buildPayload(p, masks, false) // CHAT preserved (tracked others)
		}

		// Low-def carries baselines for newly-visible players (appearance
		// + face). Kept alive by EntityMask so a player with no live masks
		// but a persistent face-entity target is still fully describable
		// to new observers. CHAT is PRESERVED: Rust `lowdefinition`
		// (info.rs:296-346) never strips CHAT — the strip lives only in
		// `highdefinition` (info.rs:282-293) for the self-echo path. A
		// player becoming visible the same tick they chat must include
		// CHAT in the add block so new observers hear the line.
		if masks == 0 && p.EntityMask() == 0 {
			r.lowDefFull[slot] = nil
		} else {
			fullMasks := masks | MaskAppearance | MaskFaceCoord
			r.lowDefFull[slot] = buildPayload(p, fullMasks, false)
		}

		// lowDefNoApp always includes FACE_COORD so newly-visible players
		// whose appearance is cached client-side still get a face target.
		// CHAT preserved per Rust `lowdefinition` (same rationale as
		// lowDefFull above).
		noAppMasks := (masks | MaskFaceCoord) &^ MaskAppearance
		r.lowDefNoApp[slot] = buildPayload(p, noAppMasks, false)
	}
}

// HighDefOf returns the high-def mask payload bytes (nil if no masks).
func (r *Renderer) HighDefOf(slot int) []byte {
	if slot < 1 || slot >= len(r.highDef) {
		return nil
	}
	return r.highDef[slot]
}

// HighDefWithChatOf returns the high-def mask payload bytes with CHAT
// preserved (nil if no masks). Consumed by writePlayers for
// tracked-other reads — other players' chat is preserved per upstream
// info.rs::write_blocks (only self strips CHAT per info.rs:289-291).
func (r *Renderer) HighDefWithChatOf(slot int) []byte {
	if slot < 1 || slot >= len(r.highDefWithChat) {
		return nil
	}
	return r.highDefWithChat[slot]
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
		// High-def carries only the NPC's live mask updates. When masks
		// is 0, leave highDef nil so the encoder takes the idle path with
		// no extend bit and no orphan mask-header byte leaking into the
		// packet (mirrors upstream NpcRenderer::compute_info early-return
		// at 2004scape/rsbuf/src/renderer.rs:258-260, and parallels the
		// PlayerInfo gate at renderer.go:34-40).
		//
		// NAI-116: a persistent FaceEntity (EntityMask != 0) on a tick
		// with masks==0 previously fell through to the else branch, writing
		// a single 0x00 mask header byte. The encoder's Walk/Run/Extend
		// leaves saw hdLen=1 and appended it to the wire, producing a
		// 4-byte NpcInfo payload [0x01, 0x9F, 0xFF, 0x00] (count + Extend
		// leaf + terminator + orphan 0x00) → Java client `Error: T2` on
		// opcode 1.
		if masks == 0 {
			r.npcHighDef[nid] = nil
		} else {
			buf := packet.NewPacket(nil)
			writeNpcMaskHeader(buf, masks)
			writeNpcMaskPayloads(buf, n, masks)
			r.npcHighDef[nid] = append([]byte(nil), buf.Data...)
		}
		// Low-def: always recomputed. lowMasks always includes
		// NpcMaskFaceCoord (line below), so the orphan-byte hazard
		// doesn't apply here — the cache always has at least the
		// FACE_COORD payload (4 bytes) behind its 1-byte mask header.
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
