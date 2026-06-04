package world

import (
	"github.com/zsrv/goscape/pkg/io/packet"
	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/rsbuf"
)

// sendUpdatePid writes one UPDATE_PID packet. TS UpdatePidEncoder (244):
// p2(pid) pbool(members). TS passes p.pid and p.members at
// Player.ts:501 via `new UpdatePid(this.pid, this.members)` — pid is
// the player's slot/protocol-identity field; members is the player's own
// membership flag. NAI-182; extended in rev-244 B2 Task 3.
func sendUpdatePid(p *Player, pid int, members bool) {
	buf := packet.NewPacket(nil)
	buf.P2(uint16(pid))
	if members {
		buf.P1(1)
	} else {
		buf.P1(0)
	}
	p.writeOut(gameserver.OpUpdatePid, buf.Bytes())
}

// sendResetClientVarCache writes one RESET_CLIENT_VARCACHE packet
// (0-byte payload). NAI-182.
func sendResetClientVarCache(p *Player) {
	p.writeOut(gameserver.OpResetClientVarCache, nil)
}

// sendResetAnims writes one RESET_ANIMS packet (0-byte payload). NAI-182.
func sendResetAnims(p *Player) {
	p.writeOut(gameserver.OpResetAnims, nil)
}

// onReconnect runs the resync sequence for a reconnecting player.
// Called from processLogins when p.reconnecting == true. Mirrors TS
// Player.onReconnect (Player.ts:516-574). LoadSave runs upstream in
// processLogins (tick.go) before this branch, so resync packets carry
// restored save state.
func onReconnect(s *Server, p *Player) {
	// (a) RESET_CLIENT_VARCACHE
	sendResetClientVarCache(p)

	// (b) varp transmit-loop
	if s.varpTypes != nil {
		for i, vt := range s.varpTypes.Configs {
			if vt != nil && vt.Transmit {
				p.writeVarp(i, p.varps[i])
			}
		}
	}

	// (c) buildArea clear + rebuild — already handled by
	// p.reconnecting==true → shouldRebuild path at player.go:694-696.
	// No new code; rebuildNormal fires in processInfo this tick.

	// (d) reboot-timer if pending
	if s.shutdownTick != -1 {
		sendUpdateRebootTimer(p, s.shutdownTick-s.currentTick)
	}

	// (e) closeModal() — mirrors TS Player.onReconnect (Player.ts:543).
	// TS calls `this.closeModal()` with no args; the default
	// `clearWeakQueue=true` (Player.ts:741) drops any pre-disconnect
	// QueueWeak entries so the resync starts with a clean weak queue.
	// Per-slot IF_CLOSE dispatch fires only when the player still had a
	// modal open across the disconnect (modalState != None). player-net-2.
	p.CloseModal(true)

	// (f) per-tab IF_SETTAB resync. Tabs default to 0 ("no tab
	// assigned"); skip zero entries.
	for tab, com := range p.tabs {
		if com != 0 {
			p.IfSetTab(com, tab)
		}
	}

	// (g) refreshInvs — flip every invListener's FirstSeen back to true
	// so the NEXT updateInvs tick re-emits each as UpdateInvFull.
	// Map-value addressability dance mirrors player.go:884-888.
	for com, l := range p.invListeners {
		l.FirstSeen = true
		p.invListeners[com] = l
	}

	// (h) per-stat UPDATE_STAT for all 21 skills.
	for i := 0; i < objtype.PlayerStatCount; i++ {
		sendUpdateStat(p, i, int(p.stats[i]), int(p.levels[i]))
	}

	// (i) UPDATE_RUN_ENERGY.
	sendUpdateRunEnergy(p, p.runenergy)

	// (j) RESET_ANIMS.
	sendResetAnims(p)

	// (k) masks |= entitymask | appearance — resync face_entity AND
	// appearance on the next mask block. Mirrors TS Player.onReconnect
	// (Player.ts:554-555). The appearance OR is load-bearing for
	// newly-visible observers post-resync — without it, an appearance
	// change that occurred just before the disconnect (or between login
	// and reconnect) would not propagate, since the mask-block emitter
	// gates the appearance payload on this bit. player-net-3.
	p.masks |= p.entitymask
	p.masks |= rsbuf.MaskAppearance

	// (l) force moveSpeed back to INSTANT. Mirrors TS
	// Player.onReconnect (Player.ts:556 — `this.moveSpeed =
	// MoveSpeed.INSTANT`). p.tele and p.jump are set upstream in
	// processLogins (tick.go) before this branch, so the full
	// TS L556-558 triple is covered.
	p.moveSpeed = MoveSpeedInstant
}
