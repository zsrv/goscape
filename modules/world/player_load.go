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
func LoadSave(p *Player, sav []byte, invTypes *objtype.InvTypeConfigs) error {
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

	// Varps: u16 count, then count × i32.
	varpCount := int(pkt.G2())
	if cap(p.varps) < varpCount {
		p.varps = make([]int32, varpCount)
	} else {
		p.varps = p.varps[:varpCount]
	}
	for i := range varpCount {
		p.varps[i] = int32(pkt.G4())
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
		// Only write to perm-scoped invs. If invTypes provided, honour
		// it; if invTypes is nil (v5+ path with no config), assume perm
		// (all fixture invs are perm).
		if invTypes != nil {
			cfg := invTypes.Configs[typeID]
			if cfg == nil || cfg.Scope != objtype.InvTypeScopePerm {
				continue
			}
		}
		inv, ok := p.invs[typeID]
		if !ok {
			// Mirror TS getInventory() returning null: skip silently.
			continue
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

	// NAI-PLAYERLOADING-D-COMBAT-LEVEL-NOT-RECOMPUTED-ON-LOAD: TS
	// PlayerLoading.ts:156 recomputes player.combatLevel via
	// getCombatLevel(). Goscape has no equivalent method on Player;
	// combatLevel is set at appearance-rebuild time elsewhere in the
	// tick. Loaded baseLevels propagate to combat level on the next
	// appearance refresh.
	return nil
}
