// Package world — SAV codec for Player persistence.
// Mirrors Engine-TS PlayerLoading.ts. See
// docs/superpowers/specs/2026-05-18-playerloading-design.md.
package world

import (
	"errors"
	"fmt"

	"github.com/zsrv/goscape/pkg/inventory"
	"github.com/zsrv/goscape/pkg/io/packet"
	"github.com/zsrv/goscape/pkg/objtype"
)

const (
	// SavMagic is the on-disk magic at byte 0..1 of every SAV file.
	// Matches TS PlayerLoading.SAV_MAGIC.
	SavMagic uint16 = 0x2004

	// SavVersion is the current SAV format version emitted by (*Player).Save().
	// Decoder supports v1..SavVersion. Matches TS PlayerLoading.SAV_VERSION.
	SavVersion uint16 = 6
)

var (
	// ErrSavInvalidMagic is returned by LoadSave when the leading 2 bytes
	// do not match SavMagic. Mirrors TS 'Invalid save file' throw.
	ErrSavInvalidMagic = errors.New("playerloading: invalid save magic")

	// ErrSavUnsupportedVer is returned by LoadSave when the version byte
	// is 0 or greater than SavVersion. Mirrors TS 'Unsupported save version'.
	ErrSavUnsupportedVer = errors.New("playerloading: unsupported save version")

	// ErrSavCorrupt is returned by LoadSave when the trailing CRC does not
	// match the recomputed CRC of the leading payload. Mirrors TS
	// 'Incorrect save checksum'.
	ErrSavCorrupt = errors.New("playerloading: incorrect save checksum")
)

// VerifySave reports whether sav has a valid magic, a supported version,
// and a matching trailing CRC. Mirrors PlayerLoading.verify
// (PlayerLoading.ts:16-29).
func VerifySave(sav []byte) bool {
	// Minimum SAV: 2 (magic) + 2 (version) + 4 (CRC) = 8.
	if len(sav) < 8 {
		return false
	}
	p := packet.NewPacket(sav)
	if p.G2() != SavMagic {
		return false
	}
	version := p.G2()
	if version < 1 || version > SavVersion {
		return false
	}
	// CRC covers bytes [0, len-4); trailing 4 bytes are the CRC itself.
	bodyLen := len(sav) - 4
	expected := packet.GetCRC(sav, 0, bodyLen)
	p.Pos = bodyLen
	got := p.G4()
	return got == expected
}

// LoadSave populates p from sav. If len(sav) < 2 it applies the
// empty-save bootstrap (21 stats=0, baseLevels=1, levels=1; hitpoints
// at level 10 with matching XP). Mirrors PlayerLoading.load
// (PlayerLoading.ts:31-159). Returns an error on magic mismatch,
// unsupported version, or CRC mismatch.
//
// invTypes is consulted only when decoding v1..v4 inv sections, which
// did not embed per-inv size and must look up InvType.Size by typeId.
// v5+ saves carry inv size inline; passing nil is acceptable for the
// empty-bootstrap branch and for v5+ saves.
//
// SAV-1 (Arc 18): pkt.G1/G2/G4 panic with io.EOF on truncated reads. A
// CRC mismatch normally catches truncation, but as defense-in-depth we
// wrap the body in a recover() that maps any io.EOF (or other read
// panic) to ErrSavCorrupt rather than propagating a slice-OOB up the
// connection-goroutine stack.
func LoadSave(p *Player, sav []byte, invTypes *objtype.InvTypeConfigs, varpTypes *objtype.VarpTypeConfigs) (retErr error) {
	defer func() {
		if r := recover(); r != nil {
			// io.EOF is the panic value used by pkg/io/packet on
			// short reads; anything else (slice OOB, type assertion)
			// also gets coerced to ErrSavCorrupt — the SAV is
			// untrusted input and corrupt data must never crash
			// the goroutine.
			retErr = fmt.Errorf("%w: panic during decode: %v", ErrSavCorrupt, r)
		}
	}()
	if len(sav) < 2 {
		// Empty-save bootstrap. Mirrors PlayerLoading.ts:41-53.
		for i := range objtype.PlayerStatCount {
			p.stats[i] = 0
			p.baseLevels[i] = 1
			p.levels[i] = 1
		}
		// Hitpoints starts at level 10.
		p.stats[objtype.PlayerStatHitpoints] = int32(objtype.GetExpByLevel(10))
		p.baseLevels[objtype.PlayerStatHitpoints] = 10
		p.levels[objtype.PlayerStatHitpoints] = 10
		return nil
	}

	// Header: magic + version.
	pkt := packet.NewPacket(sav)
	if pkt.G2() != SavMagic {
		return ErrSavInvalidMagic
	}
	version := pkt.G2()
	if version < 1 || version > SavVersion {
		return ErrSavUnsupportedVer
	}

	// CRC check: last 4 bytes are CRC of bytes [0, len-4).
	bodyLen := len(sav) - 4
	if bodyLen < 4 {
		return ErrSavCorrupt
	}
	pkt.Pos = bodyLen
	if pkt.G4() != packet.GetCRC(sav, 0, bodyLen) {
		return ErrSavCorrupt
	}

	// Rewind to body start (byte 4 — after magic + version).
	pkt.Pos = 4

	p.x = int(pkt.G2())
	p.z = int(pkt.G2())
	p.level = int(pkt.G1())
	for i := range 7 {
		b := int(pkt.G1())
		if b == 255 {
			b = -1
		}
		p.body[i] = b
	}
	for i := range 5 {
		p.colors[i] = int(pkt.G1())
	}
	p.gender = int(pkt.G1())
	p.runenergy = int(pkt.G2())

	// Playtime: v1 is u16, v2+ is i32. (TS comment: "oops playtime overflow".)
	if version >= 2 {
		p.playtime = int(int32(pkt.G4()))
	} else {
		p.playtime = int(pkt.G2())
	}

	// 21 stats: i32 exp + u8 current level. baseLevel derives from exp.
	for i := range objtype.PlayerStatCount {
		p.stats[i] = int32(pkt.G4())
		p.baseLevels[i] = uint8(objtype.GetLevelByExp(int(p.stats[i])))
		p.levels[i] = pkt.G1()
	}

	// Varps: u16 count, then count × i32, OVERLAID into the existing
	// p.varps. TS allocates player.vars once, registry-sized, in the
	// Player constructor (Player.ts:418 `new Int32Array(VarPlayerType.count)`
	// with per-type seeds at :424-432 — goscape mirror: initPlayerVarps),
	// then PlayerLoading.ts:98-101 writes saved values in WITHOUT resizing.
	// A save with fewer varps than the registry leaves the extra slots on
	// their constructor seeds; a save with more silently drops the extras
	// (JS Int32Array out-of-range writes are no-ops). Resizing p.varps to
	// the SAVE count here (the pre-rev-245.2 behavior) crashed the login
	// varp resync the first time the registry grew past an old save
	// (found live in the rev-245.2 client smoke: 244-era save with 302
	// varps vs 305 registry configs → index-out-of-range panic in
	// processLogins).
	//
	// NAI-220 defensive cleanup: zero non-PERM scope varps at load time
	// too. TS-faithful saves (post-NAI-220) already write 0 for these
	// slots, so this is a no-op for new saves. The defensive read-side
	// filter exists to retroactively scrub stale data from saves written
	// by pre-NAI-220 goscape builds (NAI-PLAYERLOADING-D-SAVE-VARPS-VERBATIM
	// era), where temp-scope combat varns like %lastcombat /
	// %aggressive_npc persisted across save→load and broke
	// player_in_combat_check on subsequent attacks. Nil-tolerant: if
	// varpTypes is nil, loads verbatim.
	varpCount := int(pkt.G2())
	for i := range varpCount {
		v := int32(pkt.G4())
		if i >= len(p.varps) {
			continue // save longer than registry: drop, like TS OOB writes
		}
		if varpTypes != nil && i < len(varpTypes.Configs) {
			if vt := varpTypes.Configs[i]; vt != nil && vt.Scope != objtype.VarpScopePerm {
				v = 0
			}
		}
		p.varps[i] = v
	}

	// Inventories: u1 count, then per-inv:
	//   typeId u2;
	//   v5+: size u2  (v1-v4: size from invTypes.Configs[typeId].Size);
	//   `size` × slot: obj u2 (id+1; 0 → -1 = empty slot, skip);
	//     count u1 (255 → read extended i32).
	invCount := int(pkt.G1())
	for range invCount {
		typeID := int(pkt.G2())
		var size int
		if version >= 5 {
			size = int(pkt.G2())
		} else {
			if invTypes == nil {
				return fmt.Errorf("playerloading: v%d decode requires invTypes; got nil", version)
			}
			if typeID < 0 || typeID >= len(invTypes.Configs) || invTypes.Configs[typeID] == nil {
				return fmt.Errorf("playerloading: unknown invType %d", typeID)
			}
			size = invTypes.Configs[typeID].Size
		}
		// Drain the slot payload first, then conditionally apply. This
		// matches TS PlayerLoading.ts:109-131 (always read all slots,
		// only write back when scope == SCOPE_PERM and inv exists).
		type slotEntry struct {
			slot, id, count int
		}
		var objs []slotEntry
		for slot := range size {
			objID := int(pkt.G2()) - 1
			if objID == -1 {
				continue
			}
			count := int(pkt.G1())
			if count == 255 {
				count = int(int32(pkt.G4()))
			}
			objs = append(objs, slotEntry{slot, objID, count})
		}
		// Only write to perm-scoped invs. Lazy-create the destination
		// container if not already present, mirroring TS
		// Player.getInventory (Engine-TS Player.ts:1415-1439) which
		// calls Inventory.fromType for any known PERM-scope inv not
		// yet in this.invs. Production login pre-creates only the Worn
		// inventory (tick.go:232-241); main inv + bank are lazy-created
		// by scripts during gameplay (server_invs.go), so a relogin
		// arrives here with most save-time invs absent from p.invs.
		// If invTypes is nil (v5+ test path with no configs) we lack
		// the type metadata to construct, so fall back to skip-if-missing.
		var cfg *objtype.InvType
		if invTypes != nil {
			cfg = invTypes.Configs[typeID]
			if cfg == nil || cfg.Scope != objtype.InvTypeScopePerm {
				continue
			}
		}
		inv, ok := p.invs[typeID]
		if !ok {
			if cfg == nil {
				continue
			}
			inv = inventory.FromType(cfg)
			if p.invs == nil {
				p.invs = map[int]*inventory.Inventory{}
			}
			p.invs[typeID] = inv
		}
		for _, o := range objs {
			inv.Set(o.slot, &inventory.Item{Id: o.id, Count: o.count})
		}
	}

	// v3+: afk zones. Count is u1, then `count` × i32; then lastAfkZone u2.
	// Goscape's p.afkZones is fixed [2]int32 — bound the loop at 2.
	if version >= 3 {
		afkCount := int(pkt.G1())
		for i := range afkCount {
			v := int32(pkt.G4())
			if i < len(p.afkZones) {
				p.afkZones[i] = v
			}
			// else: silently drop excess (TS would OOB-write Int32Array).
		}
		p.lastAfkZone = int(pkt.G2())
	}

	// v4+: chat modes packed into one u1 byte.
	if version >= 4 {
		packed := pkt.G1()
		p.publicChat = int((packed >> 4) & 0b11)
		p.privateChat = int((packed >> 2) & 0b11)
		p.tradeDuel = int(packed & 0b11)
	}

	// v6+: lastLoginTime is i64 unix-ms.
	if version >= 6 {
		p.lastLoginTime = int64(pkt.G8())
	}

	// Recompute combat level from loaded baseLevels — mirrors TS
	// PlayerLoading.ts:156 (player.combatLevel = player.getCombatLevel()).
	// triggerRebuild=false because the client has no appearance state
	// yet; first appearance generation post-login picks up the value.
	p.recomputeCombatLevel(false)
	return nil
}
