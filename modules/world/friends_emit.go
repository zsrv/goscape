package world

import (
	"github.com/zsrv/goscape/pkg/io/packet"
	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
	"github.com/zsrv/goscape/pkg/wordenc/wordpack"
)

// sendUpdateFriendList writes one UPDATE_FRIENDLIST packet for a single
// friend-entry update. Callers loop over entries; one packet per entry.
// worldId == 0 conveys offline/hidden per slice-3 friends-server contract.
// Mirrors TS UpdateFriendListEncoder (p8(name); p1(nodeId)).
//
// uint8(worldId) silently truncates worldId > 255. The wire byte is a
// single u1 (TS nodeId is also one byte), so worldId values above 255
// are unrepresentable by design (spec §5-5).
func sendUpdateFriendList(p *Player, username37 uint64, worldId int) {
	buf := packet.NewPacket(nil)
	buf.P8(username37)
	buf.P1(uint8(worldId))
	p.writeOut(gameserver.OpUpdateFriendList, buf.Bytes())
}

// sendUpdateIgnoreList writes one UPDATE_IGNORELIST packet carrying the
// complete ignorelist snapshot. Mirrors TS UpdateIgnoreListEncoder
// (for name in names: p8(name)). Empty slice produces a zero-length
// payload (still emitted; matches TS `player.write(new UpdateIgnoreList([]))`).
func sendUpdateIgnoreList(p *Player, ignored []uint64) {
	buf := packet.NewPacket(nil)
	for _, name := range ignored {
		buf.P8(name)
	}
	p.writeOut(gameserver.OpUpdateIgnoreList, buf.Bytes())
}

// sendFriendlistLoaded reports friends-list bootstrap state.
// p1(status): 0 loading, 1 connecting to friendserver, 2 online.
// TS FriendlistLoadedEncoder.ts @43e02957; sends at TS Player.ts:496-501
// (login) and World.ts:2008 (after the friend-list relay completes).
func sendFriendlistLoaded(p *Player, status int) {
	buf := packet.NewPacket(nil)
	buf.P1(uint8(status))
	p.writeOut(gameserver.OpFriendlistLoaded, buf.Bytes())
}

// sendChatFilterSettings writes one CHAT_FILTER_SETTINGS packet carrying
// the chat-mode triple. Mirrors TS ChatFilterSettingsEncoder
// (p1(publicChat); p1(privateChat); p1(tradeDuel)).
func sendChatFilterSettings(p *Player, publicChat, privateChat, tradeDuel int) {
	buf := packet.NewPacket(nil)
	buf.P1(uint8(publicChat))
	buf.P1(uint8(privateChat))
	buf.P1(uint8(tradeDuel))
	p.writeOut(gameserver.OpChatFilterSettings, buf.Bytes())
}

// sendMessagePrivate writes one MESSAGE_PRIVATE packet to the recipient.
// from is the sender's username37. pmId is the friends-server-assigned
// PM correlation id. staffLvl is the sender's staff level; the wire
// applies the TS-faithful `+1 if > 0` adjustment so the client renders
// the correct staff icon. chat is the unpacked text; goscape applies
// WordEnc.filter (via s.wordenc) and then WordPack.Pack's the result
// for the wire — mirrors TS MessagePrivateEncoder.ts:20.
func sendMessagePrivate(p *Player, from uint64, pmId uint32, staffLvl int32, chat string) {
	adjusted := staffLvl
	if adjusted > 0 {
		adjusted += 1
	}
	buf := packet.NewPacket(nil)
	buf.P8(from)
	buf.P4(uint32(pmId))
	buf.P1(uint8(adjusted))
	filtered := p.client.server.wordenc.Filter(chat)
	wordpack.Pack(buf, filtered)
	p.writeOut(gameserver.OpMessagePrivate, buf.Bytes())
}
