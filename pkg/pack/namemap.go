package pack

import (
	"os"
	"strconv"
	"strings"
)

// NameMapCallback is the per-file callback for LoadDirExact /
// NameMapLoadDir. It receives the raw line slice, the file basename,
// and the parent directory path.
type NameMapCallback func(src []string, file, path string)

// LoadOrder reads numeric-only lines from path (one int per line,
// blank lines filtered). Returns nil for missing path.
//
// TS source: tools/pack/NameMap.ts:loadOrder.
func LoadOrder(path string) []int {
	if !FileExists(path) {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	out := []int{}
	for _, line := range splitLinesCRLF(string(data)) {
		if line == "" {
			continue
		}
		n, err := strconv.Atoi(line)
		if err != nil {
			continue
		}
		out = append(out, n)
	}
	return out
}

// LoadPack reads "id=name" lines into a sparse string slice indexed by
// id. Gaps in id space are represented by empty strings. Returns nil
// for missing path.
//
// TS source: tools/pack/NameMap.ts:loadPack.
func LoadPack(path string) []string {
	if !FileExists(path) {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	out := []string{}
	for _, line := range splitLinesCRLF(string(data)) {
		if line == "" {
			continue
		}
		idStr, name, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		id, err := strconv.Atoi(idStr)
		if err != nil {
			continue
		}
		for len(out) <= id {
			out = append(out, "")
		}
		out[id] = name
	}
	return out
}

// NameMapLoadDir invokes cb for each file under path with the given
// extension. Empty lines are filtered out before the callback.
//
// TS source: tools/pack/NameMap.ts:loadDir. Renamed in goscape to
// disambiguate from Parse.LoadDir (which has different callback shape).
func NameMapLoadDir(path, ext string, cb NameMapCallback) {
	for _, f := range ListDir(path) {
		if strings.HasSuffix(f, "/") {
			continue
		}
		if !strings.HasSuffix(f, ext) {
			continue
		}
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		lines := splitLinesCRLF(string(data))
		filtered := make([]string, 0, len(lines))
		for _, l := range lines {
			if l != "" {
				filtered = append(filtered, l)
			}
		}
		dir, base, _ := strings.CutLast(f, "/")
		cb(filtered, base, dir)
	}
}

// LoadDirExact is NameMapLoadDir but does NOT filter empty lines
// (TS-parity per spec §8.1 namemap_test (d)).
//
// TS source: tools/pack/NameMap.ts:loadDirExact.
func LoadDirExact(path, ext string, cb NameMapCallback) {
	for _, f := range ListDir(path) {
		if strings.HasSuffix(f, "/") {
			continue
		}
		if !strings.HasSuffix(f, ext) {
			continue
		}
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		dir, base, _ := strings.CutLast(f, "/")
		cb(splitLinesCRLF(string(data)), base, dir)
	}
}
