// Package world — SAV codec for Player persistence.
// Mirrors Engine-TS PlayerLoading.ts. See
// docs/superpowers/specs/2026-05-18-playerloading-design.md.
package world

import (
	"errors"

	"github.com/zsrv/goscape/pkg/io/packet"
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
func LoadSave(p *Player, sav []byte) error {
	// TODO(T3+): implement
	return nil
}
