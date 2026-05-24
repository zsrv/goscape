package gamemap

import (
	"bufio"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"strconv"
	"strings"
)

// zoneIndex packs a tile (x, z, level) into the 8×8-zone index used as the
// multimap/freemap key, mirroring TS ZoneMap.zoneIndex (ZoneMap.ts:6-8). The
// x>>3 / z>>3 collapse every tile in a zone to one key, so multi-combat and
// F2P membership is stored and queried at zone granularity — matching how the
// runtime tests it (GameMap.isMulti/isFreeToPlay also route through zoneIndex).
func zoneIndex(x, z, level int) int {
	return ((x >> 3) & 0x7ff) | (((z >> 3) & 0x7ff) << 11) | ((level & 0x3) << 22)
}

// IsMulti reports whether the tile at (x, z, level) is in a multi-combat zone.
func (gm *GameMap) IsMulti(x, z, level int) bool {
	return gm.multimap[zoneIndex(x, z, level)]
}

// IsFreeToPlay reports whether the tile at (x, z) is in an F2P zone.
// F2P tables are level-agnostic in the TS reference.
func (gm *GameMap) IsFreeToPlay(x, z int) bool {
	return gm.freemap[zoneIndex(x, z, 0)]
}

// bordersFreeToPlay reports whether any of the four orthogonally-adjacent
// tiles is F2P. Mirrors TS GameMap.bordersFreeToPlay (GameMap.ts:295-297);
// loadGround/loadLocs use it so collision and static locs on the F2P border
// (but in a members zone) are still built on an F2P world.
func (gm *GameMap) bordersFreeToPlay(x, z int) bool {
	return gm.IsFreeToPlay(x+1, z) || gm.IsFreeToPlay(x-1, z) ||
		gm.IsFreeToPlay(x, z+1) || gm.IsFreeToPlay(x, z-1)
}

// SetMulti marks (or clears) the zone containing (x, z, level) as multi-combat.
// Intended for tests — production data flows from multiway.csv via Init.
// Exposing a setter avoids having to stand up a tempdir + CSV + Init for every
// cross-package test that needs a single multi-combat zone.
func (gm *GameMap) SetMulti(x, z, level int, multi bool) {
	gm.multimap[zoneIndex(x, z, level)] = multi
}

// loadCsvMap parses a multiway/free2play CSV and inserts one zone-index per
// "y_mx_mz_lx_lz" row into dst, mirroring TS GameMap.loadCsvMap
// (GameMap.ts:269-282). Lines starting with "//" or empty are skipped.
// Missing files are not errors.
//
// TS destructures `line.split('_').map(Number)` with no field-count check,
// coercing absent/garbage fields to NaN→0. We instead skip rows that aren't
// exactly 5 fields (matching the sibling worldmap-pack parser); the shipped
// Content CSVs are uniformly 5-field, so this never diverges in practice.
func loadCsvMap(path string, dst map[int]bool, log *slog.Logger) error {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r")
		if line == "" || strings.HasPrefix(line, "//") {
			continue
		}
		fields := strings.Split(line, "_")
		if len(fields) != 5 {
			continue
		}
		y := csvAtoi(fields[0])
		mx := csvAtoi(fields[1])
		mz := csvAtoi(fields[2])
		lx := csvAtoi(fields[3])
		lz := csvAtoi(fields[4])
		if lx%8 != 0 || lz%8 != 0 {
			log.Warn("CSV map line is not aligned to a zone", "line", line)
		}
		dst[zoneIndex((mx<<6)+lx, (mz<<6)+lz, y)] = true
	}
	return sc.Err()
}

// csvAtoi parses a trimmed base-10 int, returning 0 on error (mirrors TS
// Number()→bit-op coercion of NaN to 0 for malformed fields).
func csvAtoi(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}
