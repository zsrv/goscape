package login

import (
	"errors"
	"os"

	"github.com/zsrv/goscape/pkg/io/packet"
)

const (
	// savMagic / savMaxVersion mirror modules/world SavMagic / SavVersion
	// (player_load.go:18,22). The login service reads opaque .sav blobs and
	// only needs the header + trailing CRC to validate them, so the minimal
	// constants are duplicated here to avoid a login→world dependency. Keep
	// in sync if the world save format bumps.
	savMagic      = 0x2004
	savMaxVersion = 6

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

// wouldResetSaveFile reports whether persisting newSave would roll back the
// player's progress versus the existing .sav at savePath — i.e. the existing
// playtime (ticks logged in) exceeds the new one. Mirrors TS
// LoginServer.wouldResetSaveFile (LoginServer.ts:126-141). A missing or
// unparseable existing/new save is not treated as a rollback.
func wouldResetSaveFile(savePath string, newSave []byte) (bool, error) {
	existing, err := os.ReadFile(savePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	existingPlaytime, ok1 := savePlaytime(existing)
	newPlaytime, ok2 := savePlaytime(newSave)
	if !ok1 || !ok2 {
		return false, nil
	}
	return existingPlaytime > newPlaytime, nil
}
