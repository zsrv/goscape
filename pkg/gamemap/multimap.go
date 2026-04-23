package gamemap

import (
	"encoding/csv"
	"errors"
	"io/fs"
	"os"
	"strconv"
)

// packZoneCoord packs a level/x/z tuple into a single int matching the TS layout.
func packZoneCoord(x, z, level int) int {
	return (z & 0x3fff) | ((x & 0x3fff) << 14) | ((level & 0x3) << 28)
}

// IsMulti reports whether the tile at (x, z, level) is in a multi-combat zone.
func (gm *GameMap) IsMulti(x, z, level int) bool {
	return gm.multimap[packZoneCoord(x, z, level)]
}

// IsFreeToPlay reports whether the tile at (x, z) is in an F2P zone.
// F2P tables are level-agnostic in the TS reference.
func (gm *GameMap) IsFreeToPlay(x, z int) bool {
	return gm.freemap[packZoneCoord(x, z, 0)]
}

// SetMulti marks (or clears) the given tile as multi-combat. Intended for
// tests — production data flows from multiway.csv via Init. Exposing a
// setter avoids having to stand up a tempdir + CSV + Init for every
// cross-package test that needs a single multi-combat coord.
func (gm *GameMap) SetMulti(x, z, level int, multi bool) {
	gm.multimap[packZoneCoord(x, z, level)] = multi
}

// loadCsvMap parses a CSV of "level,x,z" rows and inserts into dst.
// Missing files are not errors.
func loadCsvMap(path string, dst map[int]bool) error {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	defer f.Close()

	r := csv.NewReader(f)
	r.FieldsPerRecord = -1
	rows, err := r.ReadAll()
	if err != nil {
		return err
	}

	for _, row := range rows {
		if len(row) < 3 {
			continue
		}
		level, err1 := strconv.Atoi(row[0])
		x, err2 := strconv.Atoi(row[1])
		z, err3 := strconv.Atoi(row[2])
		if err1 != nil || err2 != nil || err3 != nil {
			continue
		}
		dst[packZoneCoord(x, z, level)] = true
	}
	return nil
}
