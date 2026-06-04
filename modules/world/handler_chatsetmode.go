package world

import "github.com/zsrv/goscape/pkg/io/packet"

// handleChatSetMode handles client opcode 98 (CHAT_SETMODE), payload
// 3 bytes: g1 publicChat, g1 privateChat, g1 tradeDuel.
//
// Mirrors TS ChatSetModeHandler.ts:7-13. No socialProtect gate (TS
// does not gate this opcode). Activates Player.publicChat / .privateChat
// / .tradeDuel which are declared at player.go:172 but were unwritten
// prior to NAI-72.
//
// Friends-server propagation: grpcFriendsBridge (bridges.go) fans the
// call out as a friendspb RPC; wired by NewServer at server.go:277 via
// defaultFriendsBridge.
func handleChatSetMode(p *Player, payload []byte) error {
	if p.client == nil || p.client.server == nil {
		// goscape defensive; TS reaches via static accessor.
		return nil
	}
	pk := packet.NewPacket(payload)
	p.publicChat = int(pk.G1())
	p.privateChat = int(pk.G1())
	p.tradeDuel = int(pk.G1())
	p.client.server.friendsBridge.SetChatMode(p.username, p.privateChat)
	return nil
}
