package world

import (
	"time"

	"github.com/zsrv/goscape/pkg/io/packet"
	util "github.com/zsrv/goscape/pkg/util/jstring"
)

// handleReportAbuse handles client opcode 190 (REPORT_ABUSE), payload
// 10 bytes: g8 offender, g1 reason, g1 moderatorMute(bool).
//
// Mirrors TS ReportAbuseHandler.ts:9-26. Branch order:
//  1. reportAbuseProtect early-return (no protect-set on this branch).
//  2. Out-of-range reason → automated 48h ban + early-return
//     (no protect-set; matches TS line 14).
//  3. Optional moderator-mute branch (gated 3-way: moderatorMute &&
//     p.staffModLevel > 0 && cfg.NodeProduction).
//  4. Logger bridge call.
//  5. MessageGame ack.
//  6. reportAbuseProtect = true.
//
// MACROING/BUG_ABUSE flips offenderPlayer.submitInput = true so the
// next InputTracking window submits detailed events to the logger
// bridge. Mirrors TS World.notifyPlayerReport at World.ts:2298-2304.
// Closes NAI-73's retroactive REPORT_ABUSE polish.
//
// Friends bridge is stubbed (noopBridges); see
// NAI-72-D-FRIENDS-SERVER-BRIDGE. Login bridge ships
// loginGRPCBridgeMod as of NAI-214. Logger bridge ships the
// slogLoggerBridge default impl as of NAI-73.
func handleReportAbuse(p *Player, payload []byte) error {
	if p.client == nil || p.client.server == nil {
		// goscape defensive; TS reaches via static accessor.
		return nil
	}
	if p.reportAbuseProtect {
		return nil
	}
	pk := packet.NewPacket(payload)
	offender := pk.G8()
	reason := ReportAbuseReason(pk.G1())
	moderatorMute := pk.G1() != 0

	s := p.client.server

	// Range gate: TS sets a 48h automated ban for out-of-range values
	// (anti-tamper guard against modified clients). No protect-set on
	// this branch — TS ReportAbuseHandler.ts:14-17.
	if reason < ReportAbuseOffensiveLanguage || reason > ReportAbuseRealWorldTrading {
		s.loginBridgeMod.NotifyPlayerBan("automated", p.username, time.Now().Add(48*time.Hour))
		return nil
	}

	// Moderator-mute branch: only fires if the reporting player has staff
	// level > 0 AND the bool flag is set AND we're in production. Mutes
	// the offender for 48h. (TS ReportAbuseHandler.ts:19-22.) Reads the
	// existing Player.staffModLevel int32 (set from login proto at
	// server.go:590).
	if moderatorMute && p.staffModLevel > 0 && s.cfg.NodeProduction {
		s.loginBridgeMod.NotifyPlayerMute(p.username, util.FromBase37(offender), time.Now().Add(48*time.Hour))
	}

	// NAI-73: MACROING/BUG_ABUSE → flip the offender's submitInput so
	// the next InputTracking window submits detailed events for offline
	// review. Mirrors TS World.notifyPlayerReport (World.ts:2298-2304).
	if reason == ReportAbuseMacroing || reason == ReportAbuseBugAbuse {
		if offenderPlayer := s.LookupPlayerByUsername(util.FromBase37(offender)); offenderPlayer != nil {
			offenderPlayer.submitInput = true
		}
	}

	s.loggerBridge.NotifyPlayerReport(p, util.FromBase37(offender), reasonLabel(reason))
	p.MessageGame("Thank-you, your abuse report has been received")
	p.reportAbuseProtect = true
	return nil
}
