package world

import (
	"fmt"

	"github.com/zsrv/goscape/pkg/script"
)

// triggerMapzone fires the [mapzone,0_X_Z] cache script when content
// is registered for the entered 64-tile mapzone. Mirrors TS
// Player.ts:561-567 (NAI-142-D-R-D2). Silent no-op when the
// scriptProvider returns nil — EnqueueScriptFile nil-guards sf
// internally (player_script.go:70-72).
//
// EnqueueScriptFile (file-based variant) matches TS
// `enqueueScript(trigger, ENGINE)` shape exactly — closer than the
// ID-roundtrip EnqueueScriptArgs form. Aligned with changeStat /
// advanceStat precedent (player_script.go:587-615).
//
// (goscape defensive; TS skips this check — TS uses a static
// ScriptProvider.getByName with no client/server chain dereference.)
func (p *Player) triggerMapzone(x, z int) {
	if p.client == nil || p.client.server == nil || p.client.server.scriptProvider == nil {
		return
	}
	name := fmt.Sprintf("[mapzone,0_%d_%d]", x>>6, z>>6)
	sf := p.client.server.scriptProvider.GetByName(name)
	p.EnqueueScriptFile(sf, 0, nil, nil, script.QueueEngine)
}

// triggerMapzoneExit fires the [mapzoneexit,0_X_Z] cache script for
// the exited 64-tile mapzone. Mirrors TS Player.ts:569-574. NOTE:
// exit key has NO underscore between `mapzoneexit` and the level
// segment — verified against LostCityRS/Content 2026-05-09 (17
// [mapzoneexit,...] real declarations).
//
// (goscape defensive; TS skips this check — TS uses a static
// ScriptProvider.getByName with no client/server chain dereference.)
func (p *Player) triggerMapzoneExit(x, z int) {
	if p.client == nil || p.client.server == nil || p.client.server.scriptProvider == nil {
		return
	}
	name := fmt.Sprintf("[mapzoneexit,0_%d_%d]", x>>6, z>>6)
	sf := p.client.server.scriptProvider.GetByName(name)
	p.EnqueueScriptFile(sf, 0, nil, nil, script.QueueEngine)
}

// triggerZone fires the [zone,L_MX_MZ_LX_LZ] cache script for the
// entered 8-tile zone. Mirrors TS Player.ts:576-585. The 5-segment
// key encodes mapsquare (MX,MZ) plus zone-local 8-tile-aligned
// offset within the mapsquare (LX,LZ).
//
// (goscape defensive; TS skips this check — TS uses a static
// ScriptProvider.getByName with no client/server chain dereference.)
func (p *Player) triggerZone(level, x, z int) {
	if p.client == nil || p.client.server == nil || p.client.server.scriptProvider == nil {
		return
	}
	mx := x >> 6
	mz := z >> 6
	lx := ((x & 0x3f) >> 3) << 3
	lz := ((z & 0x3f) >> 3) << 3
	name := fmt.Sprintf("[zone,%d_%d_%d_%d_%d]", level, mx, mz, lx, lz)
	sf := p.client.server.scriptProvider.GetByName(name)
	p.EnqueueScriptFile(sf, 0, nil, nil, script.QueueEngine)
}

// triggerZoneExit fires the [zoneexit,L_MX_MZ_LX_LZ] cache script
// for the exited 8-tile zone. Mirrors TS Player.ts:587-596. NO
// underscore between `zoneexit` and the level segment — verified
// against LostCityRS/Content 2026-05-09 (5 [zoneexit,...] real
// declarations).
//
// (goscape defensive; TS skips this check — TS uses a static
// ScriptProvider.getByName with no client/server chain dereference.)
func (p *Player) triggerZoneExit(level, x, z int) {
	if p.client == nil || p.client.server == nil || p.client.server.scriptProvider == nil {
		return
	}
	mx := x >> 6
	mz := z >> 6
	lx := ((x & 0x3f) >> 3) << 3
	lz := ((z & 0x3f) >> 3) << 3
	name := fmt.Sprintf("[zoneexit,%d_%d_%d_%d_%d]", level, mx, mz, lx, lz)
	sf := p.client.server.scriptProvider.GetByName(name)
	p.EnqueueScriptFile(sf, 0, nil, nil, script.QueueEngine)
}
