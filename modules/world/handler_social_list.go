package world

import (
	"github.com/zsrv/goscape/pkg/io/packet"
	util "github.com/zsrv/goscape/pkg/util/jstring"
)

// socialListAction enumerates the four bridge methods invoked by the
// Friend/Ignore handler family.
type socialListAction int

const (
	socialAddFriend    socialListAction = iota // op 118
	socialRemoveFriend                         // op 11
	socialAddIgnore                            // op 79
	socialRemoveIgnore                         // op 171
)

// handleSocialList is the shared body of FRIENDLIST_ADD/DEL and
// IGNORELIST_ADD/DEL. All four:
//  1. Decode g8 username (uint64 base37).
//  2. Early-return if socialProtect is set OR the username decodes to
//     the "invalid_name" sentinel.
//  3. Dispatch to the appropriate FriendsBridge method.
//  4. Set socialProtect = true.
//
// Mirrors TS {Friend,Ignore}List{Add,Del}Handler.ts:8-15 (all four share
// an identical body shape modulo the World call).
//
// Friends-server propagation deferred via NAI-72-D-FRIENDS-SERVER-BRIDGE.
func handleSocialList(p *Player, payload []byte, action socialListAction) error {
	if p.client == nil || p.client.server == nil {
		// goscape defensive; TS reaches via static accessor.
		return nil
	}
	pk := packet.NewPacket(payload)
	username := pk.G8()

	if p.socialProtect || util.FromBase37(username) == "invalid_name" {
		return nil
	}

	fb := p.client.server.friendsBridge
	switch action {
	case socialAddFriend:
		fb.AddFriend(p.username, username)
	case socialRemoveFriend:
		fb.RemoveFriend(p.username, username)
	case socialAddIgnore:
		fb.AddIgnore(p.username, username)
	case socialRemoveIgnore:
		fb.RemoveIgnore(p.username, username)
	}
	p.socialProtect = true
	return nil
}

func handleFriendListAdd(p *Player, payload []byte) error {
	return handleSocialList(p, payload, socialAddFriend)
}
func handleFriendListDel(p *Player, payload []byte) error {
	return handleSocialList(p, payload, socialRemoveFriend)
}
func handleIgnoreListAdd(p *Player, payload []byte) error {
	return handleSocialList(p, payload, socialAddIgnore)
}
func handleIgnoreListDel(p *Player, payload []byte) error {
	return handleSocialList(p, payload, socialRemoveIgnore)
}
