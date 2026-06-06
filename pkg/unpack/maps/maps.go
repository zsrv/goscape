// Package maps — maps.go implements the map-family unpack entry point that
// mirrors TS tools/unpack/map/Unpack.ts (264 lines, Engine-TS 9aadcec4).
//
// The unpack reads archive 0 / file 5 (versionlist) from the cache, iterates
// the map_index jagfile entry (7 bytes per record), and for each region reads
// the land and loc cache files to emit a .jm2 text file under
// <srcDir>/maps/m{X}_{Z}.jm2.  Any pre-existing ==== NPC ==== and ==== OBJ ====
// sections from the existing .jm2 are preserved and appended after the LOC
// section.
//
// TS source: tools/unpack/map/Unpack.ts.
package maps

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/zsrv/goscape/pkg/io/filestream"
	"github.com/zsrv/goscape/pkg/io/jagfile"
	"github.com/zsrv/goscape/pkg/io/packet"
	"github.com/zsrv/goscape/pkg/pack"
)

// Options holds all inputs for a map-family unpack run.
type Options struct {
	// CacheDir is the directory containing main_file_cache.dat/idx0-4.
	// TS: new FileStream('data/unpack', false, true).
	CacheDir string

	// SrcDir is the content tree root (BUILD_SRC_DIR in TS). All output files
	// are written relative to this directory.
	SrcDir string

	// Out is the printWarning channel.  nil = discard.
	// (printInfo is commented out in TS source line 171; no info output is emitted.)
	Out io.Writer

	// Errorf is the console.error sink; nil = no-op.
	Errorf func(format string, args ...any)
}

// landData holds the six per-tile arrays decoded by readLand.
// Each array is indexed [level][x][z] where level∈[0,4), x∈[0,64), z∈[0,64).
// All values are initialised to -1 (= absent).
type landData struct {
	heightmap       [4][64][64]int
	overlayIds      [4][64][64]int
	overlayShape    [4][64][64]int
	overlayRotation [4][64][64]int
	flags           [4][64][64]int
	underlay        [4][64][64]int
}

// loc holds a single location entry decoded by readLocs.
type loc struct {
	id    int
	shape int
	angle int
}

// readLand decodes land tile data from pkt.
// Mirrors TS readLand (Unpack.ts:23-88).
//
// Code ladder for each tile (while-loop with break):
//   - 0            → break (no data for this tile)
//   - 1            → heightmap = g1; break
//   - 2..49        → overlayIds = g1b (signed byte); overlayShape = (code-2)/4;
//     overlayRotation = (code-2)&3; loop continues
//   - 50..81       → flags = code-49; loop continues
//   - 82+          → underlay = code-81; loop continues
func readLand(pkt *packet.Packet) landData {
	var d landData
	for level := range 4 {
		for x := range 64 {
			for z := range 64 {
				d.heightmap[level][x][z] = -1
				d.overlayIds[level][x][z] = -1
				d.overlayShape[level][x][z] = -1
				d.overlayRotation[level][x][z] = -1
				d.flags[level][x][z] = -1
				d.underlay[level][x][z] = -1

				for {
					code := int(pkt.G1())
					if code == 0 {
						break
					}
					if code == 1 {
						d.heightmap[level][x][z] = int(pkt.G1())
						break
					}
					if code <= 49 {
						d.overlayIds[level][x][z] = int(pkt.G1B())
						d.overlayShape[level][x][z] = (code - 2) / 4
						d.overlayRotation[level][x][z] = (code - 2) & 3
					} else if code <= 81 {
						d.flags[level][x][z] = code - 49
					} else {
						d.underlay[level][x][z] = code - 81
					}
				}
			}
		}
	}
	return d
}

// readLocs decodes loc entries from pkt.
// Mirrors TS readLocs (Unpack.ts:96-146).
//
// Encoding: outer loop uses gsmarts() delta for locId (0 = stop); inner loop
// uses gsmarts() delta for packed locData (0 = stop).  locData encodes
// z = data&0x3f, x = (data>>6)&0x3f, level = data>>12.  A g1 byte follows:
// shape = info>>2, angle = info&3.
func readLocs(pkt *packet.Packet) [4][64][64][]loc {
	var locs [4][64][64][]loc

	locId := -1
	for {
		deltaId := int(pkt.GSmartS())
		if deltaId == 0 {
			break
		}
		locId += deltaId

		locData := 0
		for {
			deltaData := int(pkt.GSmartS())
			if deltaData == 0 {
				break
			}
			locData += deltaData - 1

			locZ := locData & 0x3f
			locX := (locData >> 6) & 0x3f
			locLevel := locData >> 12

			info := int(pkt.G1())
			locShape := info >> 2
			locAngle := info & 3

			locs[locLevel][locX][locZ] = append(locs[locLevel][locX][locZ], loc{
				id:    locId,
				shape: locShape,
				angle: locAngle,
			})
		}
	}
	return locs
}

// Unpack is the top-level entry point for the map-family unpack.
// It mirrors TS tools/unpack/map/Unpack.ts.
//
// The function:
//  1. Opens the FileStream cache (readOnly=true) and reads archive 0 / file 5
//     (versionlist); fatal error if absent.
//  2. Parses the versionlist as a Jagfile and reads its map_index entry.
//  3. mkdir <srcDir>/maps.
//  4. Iterates each 7-byte map_index record: registers land/loc files in MapPack,
//     reads tile data, preserves any existing NPC/OBJ sections, and writes the
//     m{X}_{Z}.jm2 file.
//  5. Calls MapPack.save().
//
// TS source: tools/unpack/map/Unpack.ts.
//
// Note: console.time/timeEnd diagnostics are not ported (timing only).
func Unpack(opts Options) error {
	errorf := opts.Errorf
	if errorf == nil {
		errorf = func(string, ...any) {}
	}

	printWarning := func(msg string) {
		if opts.Out != nil {
			fmt.Fprintf(opts.Out, "%s\n", msg)
		}
	}

	// TS line 10: new FileStream('data/unpack', false, true)  — readOnly=true.
	cache := filestream.New(opts.CacheDir, false, true)
	defer cache.Close()

	// TS lines 12-15: cache.read(0, 5) → versionlist Jagfile.
	// decompress=false mirrors the TS default (Jagfile handles its own decompression).
	data := cache.Read(0, 5, false)
	if data == nil {
		return fmt.Errorf("No versionlist in cache")
	}

	versionlist, err := jagfile.NewJagfile(packet.NewPacket(data))
	if err != nil {
		return fmt.Errorf("parse versionlist jagfile: %w", err)
	}

	// TS lines 19-21: mkdir <srcDir>/maps.
	mapsDir := filepath.Join(opts.SrcDir, "maps")
	if err := os.MkdirAll(mapsDir, 0o755); err != nil {
		return fmt.Errorf("mkdir maps: %w", err)
	}

	// TS line 148: MapPack.clear()
	reg := &pack.Registry{SrcDir: opts.SrcDir}
	mapPack, err := reg.EnsureMap()
	if err != nil {
		return fmt.Errorf("ensure map pack: %w", err)
	}
	mapPack.Clear()

	// TS line 151: versionlist.read('map_index')
	mapIndex, err := versionlist.Read("map_index")
	if err != nil {
		return fmt.Errorf("read map_index: %w", err)
	}

	// TS lines 152-261: main loop — 7 bytes per record.
	total := mapIndex.Length() / 7
	for range total {
		region := int(mapIndex.G2())
		landFile := int(mapIndex.G2())
		locFile := int(mapIndex.G2())
		_ = mapIndex.GBool() // _members — consumed but not used

		mapX := (region >> 8) & 0xFF
		mapZ := region & 0xFF

		// TS lines 161-162: register land + loc files.
		mapPack.Register(landFile, fmt.Sprintf("m%d_%d", mapX, mapZ))
		mapPack.Register(locFile, fmt.Sprintf("l%d_%d", mapX, mapZ))

		// TS lines 164-169: read land and loc data; warn + skip if missing.
		landBytes := cache.Read(4, landFile, true)
		locBytes := cache.Read(4, locFile, true)
		if landBytes == nil || locBytes == nil {
			printWarning(fmt.Sprintf("Missing map file for %d_%d", mapX, mapZ))
			continue
		}

		// TS lines 174-189: read existing .jm2 and collect NPC/OBJ sections.
		jm2Path := filepath.Join(mapsDir, fmt.Sprintf("m%d_%d.jm2", mapX, mapZ))
		var saved []string
		if existing, readErr := os.ReadFile(jm2Path); readErr == nil {
			// TS line 176: replace \r, split by \n.
			content := strings.ReplaceAll(string(existing), "\r", "")
			lines := strings.Split(content, "\n")
			hasNpcObj := false
			for _, line := range lines {
				if strings.HasPrefix(line, "==== NPC ====") || strings.HasPrefix(line, "==== OBJ ====") {
					hasNpcObj = true
				}
				if hasNpcObj {
					saved = append(saved, line)
				}
			}
		}

		// TS lines 191-230: readLand → build MAP section → writeFileSync.
		land := readLand(packet.NewPacket(landBytes))

		var mapSection strings.Builder
		for level := range 4 {
			for x := range 64 {
				for z := range 64 {
					var str strings.Builder

					if land.heightmap[level][x][z] != -1 {
						fmt.Fprintf(&str, "h%d ", land.heightmap[level][x][z])
					}

					if land.overlayIds[level][x][z] != -1 {
						shape := land.overlayShape[level][x][z]
						rot := land.overlayRotation[level][x][z]
						// TS lines 205-211: conditional shape/rotation omission.
						// Both shape!=0 and rotation!=0 → oID;shape;rot
						// Only shape!=0                  → oID;shape
						// Otherwise                      → oID
						if shape != -1 && shape != 0 && rot != -1 && rot != 0 {
							fmt.Fprintf(&str, "o%d;%d;%d ", land.overlayIds[level][x][z], shape, rot)
						} else if shape != -1 && shape != 0 {
							fmt.Fprintf(&str, "o%d;%d ", land.overlayIds[level][x][z], shape)
						} else {
							fmt.Fprintf(&str, "o%d ", land.overlayIds[level][x][z])
						}
					}

					if land.flags[level][x][z] != -1 {
						fmt.Fprintf(&str, "f%d ", land.flags[level][x][z])
					}

					if land.underlay[level][x][z] != -1 {
						fmt.Fprintf(&str, "u%d ", land.underlay[level][x][z])
					}

					if str.Len() > 0 {
						// TS line 223: trimEnd() removes trailing space.
						fmt.Fprintf(&mapSection, "%d %d %d: %s\n", level, x, z, strings.TrimRight(str.String(), " "))
					}
				}
			}
		}

		// TS line 229: writeFileSync → MAP header + section (creates/overwrites file).
		mapContent := "==== MAP ====\n" + mapSection.String()
		if err := os.WriteFile(jm2Path, []byte(mapContent), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", jm2Path, err)
		}

		// TS lines 232-256: readLocs → build LOC section → appendFileSync.
		locs := readLocs(packet.NewPacket(locBytes))

		var locSection strings.Builder
		for level := range 4 {
			for x := range 64 {
				for z := range 64 {
					if len(locs[level][x][z]) == 0 {
						continue
					}
					for _, l := range locs[level][x][z] {
						// TS lines 245-249: angle omitted when 0.
						if l.angle == 0 {
							fmt.Fprintf(&locSection, "%d %d %d: %d %d\n", level, x, z, l.id, l.shape)
						} else {
							fmt.Fprintf(&locSection, "%d %d %d: %d %d %d\n", level, x, z, l.id, l.shape, l.angle)
						}
					}
				}
			}
		}

		// TS line 255: appendFileSync → "\n==== LOC ====\n" + loc section.
		if err := appendString(jm2Path, "\n==== LOC ====\n"+locSection.String()); err != nil {
			return fmt.Errorf("append loc section to %s: %w", jm2Path, err)
		}

		// TS lines 258-260: if saved.length → appendFileSync → "\n" + saved.join("\n").
		if len(saved) > 0 {
			if err := appendString(jm2Path, "\n"+strings.Join(saved, "\n")); err != nil {
				return fmt.Errorf("append npc/obj sections to %s: %w", jm2Path, err)
			}
		}
	}

	// TS line 264: MapPack.save()
	if err := mapPack.Save(); err != nil {
		return fmt.Errorf("save map pack: %w", err)
	}

	return nil
}

// appendString opens path for append (creating if absent), writes s, and closes
// the file.  Mirrors the appendFileSync pattern used by the config driver.
func appendString(path, s string) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	_, werr := f.WriteString(s)
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	if werr != nil {
		return fmt.Errorf("write %s: %w", path, werr)
	}
	return nil
}
