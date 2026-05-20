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
// the correct staff icon. chat is the unpacked text; goscape
// WordPack.Pack's it here for the wire.
//
// DEVIATION-NAI-182-D5-NO-WORDENC-FILTER — TS calls
// `WordPack.pack(buf, WordEnc.filter(message.msg))`; goscape has no
// WordEnc.filter port yet (only WordPack). The chat is packed verbatim.
// Retires when wordenc filter is ported.
func sendMessagePrivate(p *Player, from uint64, pmId uint32, staffLvl int32, chat string) {
	adjusted := staffLvl
	if adjusted > 0 {
		adjusted += 1
	}
	buf := packet.NewPacket(nil)
	buf.P8(from)
	buf.P4(uint32(pmId))
	buf.P1(uint8(adjusted))
	wordpack.Pack(buf, chat)
	p.writeOut(gameserver.OpMessagePrivate, buf.Bytes())
}
