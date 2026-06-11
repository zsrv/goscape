package login

import (
	"errors"
	"fmt"
	"os"

	"github.com/zsrv/goscape/pkg/io/packet"
	"github.com/zsrv/goscape/pkg/objtype"
)

const (
	// savMagic / savMaxVersion mirror modules/world SavMagic / SavVersion
	// (player_load.go). The login service reads opaque .sav blobs and
	// only needs the header + trailing CRC to validate them, so the minimal
	// constants are duplicated here to avoid a login→world dependency.
	// A stale savMaxVersion SILENTLY DISCARDS every logout/autosave
	// (persistSaveIfValid warns then returns nil) — drift is pinned by
	// the cross-module test TestSavMaxVersionMatchesWorld.
	savMagic = 0x2004
	// 7 since rev-254 A6 (sparse varp encoding, SAV_VERSION 7).
	savMaxVersion = 7

	// savePlaytimeOffset is the byte offset of the int32 playtime field in a
	// SAV blob: magic(2)+version(2)+x(2)+z(2)+level(1)+body(7)+colors(5)+
	// gender(1)+runenergy(2) = 24. Mirrors (*Player).Save layout
	// (modules/world/player_save.go:40-53).
	savePlaytimeOffset = 24
)

// verifySave mirrors TS PlayerLoading.verify (PlayerLoading.ts:16-27): the blob
// must open with savMagic, carry a version <= savMaxVersion, and end with a
// 4-byte CRC over everything that precedes it.
func verifySave(save []byte) bool {
	if len(save) < 8 { // 2 magic + 2 version + 4-byte CRC trailer minimum
		return false
	}
	magic := uint16(save[0])<<8 | uint16(save[1])
	if magic != savMagic {
		return false
	}
	version := uint16(save[2])<<8 | uint16(save[3])
	if version > savMaxVersion {
		return false
	}
	n := len(save)
	stored := uint32(save[n-4])<<24 | uint32(save[n-3])<<16 | uint32(save[n-2])<<8 | uint32(save[n-1])
	return packet.GetCRC(save, 0, n-4) == stored
}

// savePlaytime extracts the int32 playtime field. Returns (0, false) when the
// blob is too short to contain it.
func savePlaytime(save []byte) (int32, bool) {
	if len(save) < savePlaytimeOffset+4 {
		return 0, false
	}
	o := savePlaytimeOffset
	return int32(uint32(save[o])<<24 | uint32(save[o+1])<<16 | uint32(save[o+2])<<8 | uint32(save[o+3])), true
}

// saveStats extracts the 21 per-stat XP values from a SAV blob. Mirrors the
// stat block PlayerLoading reads (modules/world/player_load.go:151-156): right
// after playtime, 21 entries of (i32 XP + u8 current level). Only the XP is
// returned — base levels derive from it via objtype.GetLevelByExp, exactly as
// TS PlayerLoading.ts:94 and player_load.go:154 do (the stored level byte is
// ignored). Version-aware: playtime is i32 for v2+ and u16 for v1
// (player_load.go:144-149). Returns (zero, false) when the blob is too short.
func saveStats(save []byte) ([objtype.PlayerStatCount]int32, bool) {
	var stats [objtype.PlayerStatCount]int32
	if len(save) < 4 {
		return stats, false
	}
	version := uint16(save[2])<<8 | uint16(save[3])
	statsOff := savePlaytimeOffset + 4 // v2+ playtime is i32
	if version < 2 {
		statsOff = savePlaytimeOffset + 2 // v1 playtime is u16
	}
	const stride = 5 // i32 XP + u8 current level
	if len(save) < statsOff+objtype.PlayerStatCount*stride {
		return stats, false
	}
	for i := range objtype.PlayerStatCount {
		o := statsOff + i*stride
		stats[i] = int32(uint32(save[o])<<24 | uint32(save[o+1])<<16 | uint32(save[o+2])<<8 | uint32(save[o+3]))
	}
	return stats, true
}

// wouldResetSaveFile reports whether persisting newSave would roll back the
// player's progress versus the existing .sav at savePath — i.e. the existing
// playtime (ticks logged in) exceeds the new one. Mirrors TS
// LoginServer.wouldResetSaveFile (LoginServer.ts:126-141): TS loads both saves
// via PlayerLoading.load (PlayerLoading.ts:55-68), which THROWS on bad
// magic/version/CRC of the existing save — propagating out of
// wouldResetSaveFile and aborting the persistSave path so the new save is NOT
// written over a corrupt existing one (fail-safe).
//
// login-server-6: a missing existing save is fine (no rollback possible); a
// PRESENT-but-corrupt existing save returns an error so the caller aborts the
// write — matching TS's thrown-error semantics. A blob too short to contain
// playtime is treated as a clean no-rollback (TS PlayerLoading.load returns
// a defaulted Player for `sav.data.length < 2`).
func wouldResetSaveFile(savePath string, newSave []byte) (bool, error) {
	existing, err := os.ReadFile(savePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	if !verifySave(existing) {
		return false, fmt.Errorf("existing save at %s failed verification: TS PlayerLoading.load throws on bad magic/version/CRC (login-server-6: LoginServer.ts:126-141 → PlayerLoading.ts:55-68)", savePath)
	}
	existingPlaytime, ok1 := savePlaytime(existing)
	newPlaytime, ok2 := savePlaytime(newSave)
	if !ok1 || !ok2 {
		return false, nil
	}
	return existingPlaytime > newPlaytime, nil
}
