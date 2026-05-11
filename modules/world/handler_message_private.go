package world

import (
	"time"

	"github.com/zsrv/goscape/pkg/coordgrid"
	"github.com/zsrv/goscape/pkg/io/packet"
	util "github.com/zsrv/goscape/pkg/util/jstring"
	"github.com/zsrv/goscape/pkg/wordenc/wordpack"
)

// handleMessagePrivate handles client opcode 148 (MESSAGE_PRIVATE),
// dynamic 1-byte length. Wire: G8 target(base37) + word-packed input
// bytes (variable length, payload tail). NAI-158.
//
// Mirrors TS MessagePrivateHandler.ts:10-35. Gate order (no protect-set
// on any early return):
//  1. socialProtect || len(input) > 100 → return.
//  2. mutedUntil active → return.
//  3. invalid_name base37 → automated 48h ban; return.
//  4. WordPack.Unpack; friendsBridge.PrivateMessage; socialProtect=true.
//
// Friends-server propagation deferred via NAI-72-D-FRIENDS-SERVER-BRIDGE
// (PrivateMessage bridge is a stub). LoginBridgeMod.NotifyPlayerBan is
// the same stub pattern used by handler_reportabuse.go:50.
func handleMessagePrivate(p *Player, payload []byte) error {
	if p.client == nil || p.client.server == nil {
		// goscape defensive; TS reaches via static accessor.
		return nil
	}
	pk := packet.NewPacket(payload)
	target := pk.G8()
	inputLen := len(payload) - 8
	if p.socialProtect || inputLen > 100 {
		return nil
	}
	if !p.mutedUntil.IsZero() && time.Now().Before(p.mutedUntil) {
		return nil
	}
	s := p.client.server
	if util.FromBase37(target) == "invalid_name" {
		s.loginBridgeMod.NotifyPlayerBan("automated", p.username, time.Now().Add(48*time.Hour))
		return nil
	}
	msg := wordpack.Unpack(pk, inputLen)
	coord := coordgrid.PackCoord(p.level, p.x, p.z)
	s.friendsBridge.PrivateMessage(p.username, p.staffModLevel, s.nextPmId(), target, msg, coord)
	p.socialProtect = true
	return nil
}
