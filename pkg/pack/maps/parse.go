// Package maps ports tools/pack/map/Pack.js — the inline .jm2 text parser
// plus the four per-zone encoders (land, loc, npc, obj).
package maps

import (
	"strconv"
	"strings"
)

// packKey encodes (level, x, z) per TS map/Pack.js:13-15.
func packKey(level, x, z int) int {
	return (level << 12) | (x << 6) | z
}

type landTile struct {
	H, OverlayID, OverlayShape, OverlayRot, Flags, Underlay int
}

type locEntry struct {
	ID, Shape, Angle int
}

type objEntry struct {
	ID, Count int
}

type mapData struct {
	Land map[int]landTile
	Loc  map[int][]locEntry
	Npc  map[int][]int
	Obj  map[int][]objEntry
}

// readMap parses TS map source lines into per-section maps. Ports
// map/Pack.js:17-105.
func readMap(lines []string) mapData {
	out := mapData{
		Land: map[int]landTile{},
		Loc:  map[int][]locEntry{},
		Npc:  map[int][]int{},
		Obj:  map[int][]objEntry{},
	}
	section := ""

	for _, line := range lines {
		if len(line) == 0 {
			continue
		}
		if line[0] == '=' {
			// TS: line.slice(4, -4).slice(1, 4) on "==== SEC ===="
			// yields the 3-char section name in caps.
			if len(line) < 12 {
				continue
			}
			section = line[5:8]
			continue
		}
		colon := strings.Index(line, ":")
		sp1 := strings.Index(line, " ")
		if sp1 < 0 || colon < 0 {
			continue
		}
		sp2off := strings.Index(line[sp1+1:], " ")
		if sp2off < 0 {
			continue
		}
		sp2 := sp1 + 1 + sp2off

		level := int(line[0] - '0')
		x, _ := strconv.Atoi(line[sp1+1 : sp2])
		z, _ := strconv.Atoi(line[sp2+1 : colon])
		key := packKey(level, x, z)
		data := line[colon+2:]

		switch section {
		case "MAP":
			out.Land[key] = parseMapTile(data)
		case "LOC":
			parts := strings.Split(data, " ")
			id, _ := strconv.Atoi(parts[0])
			shape := 10
			angle := 0
			if len(parts) > 1 {
				shape, _ = strconv.Atoi(parts[1])
			}
			if len(parts) > 2 {
				angle, _ = strconv.Atoi(parts[2])
			}
			out.Loc[key] = append(out.Loc[key], locEntry{ID: id, Shape: shape, Angle: angle})
		case "NPC":
			id, _ := strconv.Atoi(data)
			out.Npc[key] = append(out.Npc[key], id)
		case "OBJ":
			sp := strings.Index(data, " ")
			if sp < 0 {
				continue
			}
			id, _ := strconv.Atoi(data[:sp])
			cnt, _ := strconv.Atoi(data[sp+1:])
			out.Obj[key] = append(out.Obj[key], objEntry{ID: id, Count: cnt})
		}
	}
	return out
}

// parseMapTile parses a space-separated token stream:
//
//	h<int> o<id>;<shape>;<rot> f<flags> u<underlay>
func parseMapTile(data string) landTile {
	t := landTile{H: 0, OverlayID: -1, OverlayShape: -1, OverlayRot: -1, Flags: -1, Underlay: -1}
	for token := range strings.SplitSeq(data, " ") {
		if len(token) == 0 {
			continue
		}
		typ := token[0]
		info := token[1:]
		switch typ {
		case 'h':
			t.H, _ = strconv.Atoi(info)
		case 'o':
			parts := strings.Split(info, ";")
			t.OverlayID, _ = strconv.Atoi(parts[0])
			if len(parts) > 1 {
				t.OverlayShape, _ = strconv.Atoi(parts[1])
			}
			if len(parts) > 2 {
				t.OverlayRot, _ = strconv.Atoi(parts[2])
			}
		case 'f':
			t.Flags, _ = strconv.Atoi(info)
		case 'u':
			t.Underlay, _ = strconv.Atoi(info)
		}
	}
	return t
}
