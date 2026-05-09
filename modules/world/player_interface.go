package world

import (
	"github.com/zsrv/goscape/pkg/colorconv"
	"github.com/zsrv/goscape/pkg/io/packet"
	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
	"github.com/zsrv/goscape/pkg/objtype"
)

// This file wires Player to the 12 IF_SET* fire-and-forget wire emitters.
// Wire layouts are verified against LostCityRS/Engine-TS
// src/network/game/server/codec/IfSet*Encoder.ts. Each method builds a
// raw payload and calls writeOut, which handles opcode encryption and
// length prefixing. No server-side state is persisted.

// IfSetText emits IF_SETTEXT (com u16, text jstr). Dynamic payload size.
func (p *Player) IfSetText(com int, text string) {
	buf := packet.NewPacket(nil)
	buf.P2(uint16(com))
	buf.PJStrLF(text)
	p.writeOut(gameserver.OpIfSetText, buf.Bytes())
}

// IfSetModel emits IF_SETMODEL (com u16, modelID u16). 4-byte payload;
// TS encoder uses p2 for the model, not p4.
func (p *Player) IfSetModel(com, modelID int) {
	buf := packet.NewPacket(nil)
	buf.P2(uint16(com))
	buf.P2(uint16(modelID))
	p.writeOut(gameserver.OpIfSetModel, buf.Bytes())
}

// IfSetNpcHead emits IF_SETNPCHEAD (com u16, npcID u16). 4-byte payload.
func (p *Player) IfSetNpcHead(com, npcID int) {
	buf := packet.NewPacket(nil)
	buf.P2(uint16(com))
	buf.P2(uint16(npcID))
	p.writeOut(gameserver.OpIfSetNpcHead, buf.Bytes())
}

// IfSetPlayerHead emits IF_SETPLAYERHEAD (com u16). 2-byte payload.
func (p *Player) IfSetPlayerHead(com int) {
	buf := packet.NewPacket(nil)
	buf.P2(uint16(com))
	p.writeOut(gameserver.OpIfSetPlayerHead, buf.Bytes())
}

// IfSetAnim emits IF_SETANIM (com u16, seqID u16). 4-byte payload.
func (p *Player) IfSetAnim(com, seqID int) {
	buf := packet.NewPacket(nil)
	buf.P2(uint16(com))
	buf.P2(uint16(seqID))
	p.writeOut(gameserver.OpIfSetAnim, buf.Bytes())
}

// IfSetHide emits IF_SETHIDE (com u16, hide u8 as 0/1). 3-byte payload.
func (p *Player) IfSetHide(com int, hide bool) {
	buf := packet.NewPacket(nil)
	buf.P2(uint16(com))
	var v uint8
	if hide {
		v = 1
	}
	buf.P1(v)
	p.writeOut(gameserver.OpIfSetHide, buf.Bytes())
}

// IfSetTab emits IF_SETTAB (com u16, tab u8). 3-byte payload. Also
// writes p.tabs[tab] = com so IsComponentVisible's tab check sees the
// same set of root-layers the client sees. Mirrors TS Player.setTab
// (Player.ts:2042-2044) which performs the array write before writing
// the wire packet.
func (p *Player) IfSetTab(com, tab int) {
	if tab >= 0 && tab < len(p.tabs) {
		p.tabs[tab] = com
	}
	buf := packet.NewPacket(nil)
	buf.P2(uint16(com))
	buf.P1(uint8(tab))
	p.writeOut(gameserver.OpIfSetTab, buf.Bytes())
}

// IfSetObject emits IF_SETOBJECT (com u16, objID u16, scale u16). 6-byte
// payload; TS encoder uses p2 for all three fields.
func (p *Player) IfSetObject(com, objID, scale int) {
	buf := packet.NewPacket(nil)
	buf.P2(uint16(com))
	buf.P2(uint16(objID))
	buf.P2(uint16(scale))
	p.writeOut(gameserver.OpIfSetObject, buf.Bytes())
}

// IfSetColour emits IF_SETCOLOUR (com u16, colour u16). 4-byte payload.
func (p *Player) IfSetColour(com, colour int) {
	buf := packet.NewPacket(nil)
	buf.P2(uint16(com))
	buf.P2(uint16(colorconv.Rgb24to15(colour)))
	p.writeOut(gameserver.OpIfSetColour, buf.Bytes())
}

// IfSetPosition emits IF_SETPOSITION (com u16, x u16, y u16). 6-byte payload.
func (p *Player) IfSetPosition(com, x, y int) {
	buf := packet.NewPacket(nil)
	buf.P2(uint16(com))
	buf.P2(uint16(x))
	buf.P2(uint16(y))
	p.writeOut(gameserver.OpIfSetPosition, buf.Bytes())
}

// IfSetRecol emits IF_SETRECOL (com u16, src u16, dst u16). 6-byte payload.
func (p *Player) IfSetRecol(com, srcColour, dstColour int) {
	buf := packet.NewPacket(nil)
	buf.P2(uint16(com))
	buf.P2(uint16(srcColour))
	buf.P2(uint16(dstColour))
	p.writeOut(gameserver.OpIfSetRecol, buf.Bytes())
}

// IfSetTabActive emits IF_SETTABACTIVE (tab u8). 1-byte payload.
func (p *Player) IfSetTabActive(tab int) {
	buf := packet.NewPacket(nil)
	buf.P1(uint8(tab))
	p.writeOut(gameserver.OpIfSetTabActive, buf.Bytes())
}

// IsComponentVisible reports whether the given component's rootLayer
// is currently in any of the player's visible-modal slots. Mirrors TS
// Player.isComponentVisible (Player.ts:2047-2049).
//
// Goscape divergence from TS: TS gates each modal slot via raw
// equality against -1-defaulted fields; goscape uses the modalState
// bitmap (modalStateMain/Chat/Side) because modalMain/Chat/Side
// fields are not initialized to -1 (zero-valued by Go default).
// Functionally equivalent: a slot is "active" iff the corresponding
// bit is set, and only then is its component-id read.
//
// modalTutorial IS initialized to -1 (see newPlayer); the != -1 guard
// is direct because the field is write-empty until the IF_OPENTUT-
// equivalent opcode lands (DEVIATION NAI-59-D-MODALTUTORIAL-NO-PRODUCER).
func (p *Player) IsComponentVisible(com *objtype.ComponentType) bool {
	if com == nil {
		return false
	}
	if p.modalState&modalStateMain != 0 && com.RootLayer == p.modalMain {
		return true
	}
	if p.modalState&modalStateChat != 0 && com.RootLayer == p.modalChat {
		return true
	}
	if p.modalState&modalStateSide != 0 && com.RootLayer == p.modalSide {
		return true
	}
	for _, t := range p.tabs {
		if t == com.RootLayer {
			return true
		}
	}
	if p.modalTutorial != -1 && com.RootLayer == p.modalTutorial {
		return true
	}
	return false
}
