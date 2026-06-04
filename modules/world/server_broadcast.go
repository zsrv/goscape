package world

import (
	"strings"

	"github.com/zsrv/goscape/pkg/fonttype"
)

// BroadcastMes sends a MESSAGE_GAME packet to every logged-in player, mirroring
// TS World.broadcastMes (World.ts:1803-1811):
//
//	for (const player of this.players) {
//	    if (message.includes('\n')) {
//	        message.split('\n').forEach(wrap => player.wrappedMessageGame(wrap));
//	    } else {
//	        player.wrappedMessageGame(message);
//	    }
//	}
//
// where Player.wrappedMessageGame (Player.ts:2153-2159) further splits each
// segment via FontType.get(1).split(mes, 456) and emits one MESSAGE_GAME per
// wrapped line. world-ops-1: goscape previously called MessageGame(msg)
// directly — no '\n' split, no font-wrap. Long banner messages or any
// multi-line broadcast arrived as a single oversized chat line that the
// client would clip or render off-screen.
//
// FontType[1] (p12) drives the pixel-width wrap at 456px. When the font cache
// failed to load (tests or a misconfigured server), fall back to plain
// MessageGame on each '\n'-split segment so the broadcast still reaches every
// player rather than silently dropping. Holds Server.playersMu.RLock for the
// duration of the fan-out — callers must NOT hold playersMu.
func (s *Server) BroadcastMes(msg string) {
	var font *fonttype.FontType
	if len(s.fontTypes) > 1 {
		font = s.fontTypes[1]
	}
	s.playersMu.RLock()
	defer s.playersMu.RUnlock()
	for p := range s.players.all() {
		for segment := range strings.SplitSeq(msg, "\n") {
			if font == nil {
				p.MessageGame(segment)
				continue
			}
			for _, wrap := range font.Split(segment, 456) {
				p.MessageGame(wrap)
			}
		}
	}
}
