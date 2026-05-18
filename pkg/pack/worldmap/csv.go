package worldmap

import (
	"log/slog"
	"strconv"
	"strings"

	"github.com/zsrv/goscape/pkg/coordgrid"
)

// label is one parsed entry from labels.txt.
type label struct {
	Text string
	X    int
	Z    int
	Type int
}

// processCsv expands a multiway / free2play / ignore CSV body into
// a set of packed CoordGrid coords. Lines starting with "//" or
// empty are skipped. Warnings are logged via lg; this function
// never returns an error (TS parity with printWarning).
//
// Two row formats:
//   - "level_mx_mz_lx_lz"                 — one 8×8 tile block
//   - "fromLine,toLine" (5 fields each)   — rectangle expansion
//
// In the range form only fromLevel is used; toLevel is discarded.
//
// Overlap detection is only performed in the range form; duplicate
// single-zone rows are silently merged (TS parity — the single-zone
// loop in TS Worldmap.ts:93-97 also has no duplicate check).
func processCsv(lines []string, name string, lg *slog.Logger) map[int]struct{} {
	result := make(map[int]struct{})
	for _, line := range lines {
		if strings.HasPrefix(line, "//") || line == "" {
			continue
		}
		parts := strings.Split(line, ",")
		if len(parts) == 2 {
			fromParts := strings.Split(parts[0], "_")
			toParts := strings.Split(parts[1], "_")
			if len(fromParts) != 5 || len(toParts) != 5 {
				continue
			}
			fromLevel := atoi(fromParts[0])
			fromMx := atoi(fromParts[1])
			fromMz := atoi(fromParts[2])
			fromLx := atoi(fromParts[3])
			fromLz := atoi(fromParts[4])
			toMx := atoi(toParts[1])
			toMz := atoi(toParts[2])
			toLx := atoi(toParts[3])
			toLz := atoi(toParts[4])

			if fromLx%8 != 0 || fromLz%8 != 0 || toLx%8 != 7 || toLz%8 != 7 ||
				fromMx > toMx || fromMz > toMz ||
				(fromMx <= toMx && fromMz <= toMz && (fromLx > toLx || fromLz > toLz)) {
				lg.Warn("map not aligned to a zone", "name", name, "row", line)
			}

			startX := (fromMx << 6) + fromLx
			startZ := (fromMz << 6) + fromLz
			endX := (toMx << 6) + toLx
			endZ := (toMz << 6) + toLz
			for x := startX; x <= endX; x++ {
				for z := startZ; z <= endZ; z++ {
					packed := coordgrid.PackCoord(fromLevel, x, z)
					if _, dup := result[packed]; dup {
						lg.Warn("Overlapping map", "name", name, "row", line)
					}
					result[packed] = struct{}{}
				}
			}
		} else {
			fields := strings.Split(line, "_")
			if len(fields) != 5 {
				continue
			}
			level := atoi(fields[0])
			mx := atoi(fields[1])
			mz := atoi(fields[2])
			lx := atoi(fields[3])
			lz := atoi(fields[4])
			for i := range 8 {
				for j := range 8 {
					result[coordgrid.PackCoord(level, (mx<<6)+lx+i, (mz<<6)+lz+j)] = struct{}{}
				}
			}
		}
	}
	return result
}

// parseLabels filters non-"=" lines and parses each remaining row
// as "text,x,z,type". The first 4 comma-separated fields are kept;
// trailing extras are absorbed into the last field (TS destructure
// discards them after parseInt-on-leading-digits coercion).
func parseLabels(src string) []label {
	rawLines := strings.Split(strings.ReplaceAll(src, "\r\n", "\n"), "\n")
	out := make([]label, 0, len(rawLines))
	for _, line := range rawLines {
		if !strings.HasPrefix(line, "=") {
			continue
		}
		fields := strings.SplitN(line[1:], ",", 4)
		if len(fields) != 4 {
			continue
		}
		out = append(out, label{
			Text: fields[0],
			X:    atoi(fields[1]),
			Z:    atoi(fields[2]),
			Type: atoi(fields[3]),
		})
	}
	return out
}

// atoi parses a trimmed base-10 integer, returning 0 on error.
// TS parseInt trims whitespace and accepts leading-digit strings
// ("5abc" → 5); strconv.Atoi rejects non-numeric tails. The callers
// here only receive purely numeric fields (coord components and
// label coordinates), so the divergence is harmless.
func atoi(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}
