package pack

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Validator is the optional pre-load hook on PackFile. When non-nil,
// PackFile.Reload calls Validator(pf) instead of the default Load
// path. Per-config packer slices (NAI-192+) supply validators.
type Validator func(pf *PackFile) error

// PackFile is a name↔id registry loaded from a .pack source file.
//
// NOT safe for concurrent use; callers must serialize Reload, Load,
// Save, Register, Delete*, RefreshNames.
//
// NAI-191-D-NO-GLOBAL-SRCDIR: TS PackFileBase reads
// Environment.BUILD_SRC_DIR; goscape takes srcDir as a struct field
// supplied at construction.
//
// TS source: tools/pack/PackFileBase.ts.
type PackFile struct {
	Type      string
	SrcDir    string
	Validator Validator
	Pack      map[int]string
	Names     map[string]struct{}
	NameToID  map[string]int
	Max       int
}

var packLineRE = regexp.MustCompile(`^\d+=`)

// NewPackFile constructs a PackFile and immediately Reloads.
//
// NAI-191-D-NO-PARENT-PORT: TS catches Reload errors and branches on
// parentPort (worker → printError, main → printFatalError). Goscape
// returns the error; callers handle logging policy.
func NewPackFile(srcDir, packType string, validator Validator) (*PackFile, error) {
	pf := &PackFile{
		Type:      packType,
		SrcDir:    srcDir,
		Validator: validator,
		Pack:      map[int]string{},
		Names:     map[string]struct{}{},
		NameToID:  map[string]int{},
	}
	if err := pf.Reload(); err != nil {
		return nil, err
	}
	return pf, nil
}

// Size returns the number of registered (id, name) entries.
func (pf *PackFile) Size() int { return len(pf.Pack) }

// Reload runs the Validator if non-nil, else Loads the default file
// at <SrcDir>/pack/<Type>.pack.
func (pf *PackFile) Reload() error {
	if pf.Validator != nil {
		return pf.Validator(pf)
	}
	return pf.Load(filepath.Join(pf.SrcDir, "pack", pf.Type+".pack"))
}

// Load reads an "id=name" file. Missing paths are not errors (empty
// registry). Lines that fail the `^\d+=` regex are skipped (TS parity:
// preserves comment/blank tolerance). Lines with empty names return
// an error.
func (pf *PackFile) Load(path string) error {
	pf.Pack = map[int]string{}
	if !FileExists(path) {
		pf.RefreshNames()
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := splitLinesCRLF(string(data))
	for i, line := range lines {
		if len(line) == 0 || !packLineRE.MatchString(line) {
			continue
		}
		eq := strings.IndexByte(line, '=')
		name := line[eq+1:]
		if len(name) == 0 {
			return fmt.Errorf("pack file has an empty name %s:%d", path, i+1)
		}
		id, err := strconv.Atoi(line[:eq])
		if err != nil {
			continue
		}
		pf.Register(id, name)
	}
	pf.RefreshNames()
	return nil
}

// Clear empties all four maps and resets Max.
func (pf *PackFile) Clear() {
	pf.Pack = map[int]string{}
	pf.Names = map[string]struct{}{}
	pf.NameToID = map[string]int{}
	pf.Max = 0
}

// Register inserts (id, name) into Pack and NameToID. Does NOT touch
// Names or Max — call RefreshNames if needed.
func (pf *PackFile) Register(id int, name string) {
	pf.Pack[id] = name
	pf.NameToID[name] = id
}

// Delete removes the entry at id and calls RefreshNames. No-op if id
// is absent.
func (pf *PackFile) Delete(id int) {
	name, ok := pf.Pack[id]
	if !ok {
		return
	}
	delete(pf.Pack, id)
	delete(pf.NameToID, name)
	pf.RefreshNames()
}

// DeleteByName removes the entry with the given name and calls
// RefreshNames. No-op if name is absent.
func (pf *PackFile) DeleteByName(name string) {
	id, ok := pf.NameToID[name]
	if !ok {
		return
	}
	delete(pf.NameToID, name)
	delete(pf.Pack, id)
	pf.RefreshNames()
}

// RefreshNames rebuilds Names from Pack values and recomputes Max as
// (max id + 1) when Names is non-empty. Does NOT rebuild NameToID
// (TS parity per spec §3.7).
func (pf *PackFile) RefreshNames() {
	pf.Names = make(map[string]struct{}, len(pf.Pack))
	for _, name := range pf.Pack {
		pf.Names[name] = struct{}{}
	}
	if len(pf.Names) == 0 {
		return
	}
	maxID := 0
	for id := range pf.Pack {
		if id > maxID {
			maxID = id
		}
	}
	pf.Max = maxID + 1
}

// Save writes the registry to <SrcDir>/pack/<Type>.pack, sorted by id
// ascending, "id=name\n" form with trailing newline. Creates the pack
// directory recursively if absent.
func (pf *PackFile) Save() error {
	packDir := filepath.Join(pf.SrcDir, "pack")
	if err := os.MkdirAll(packDir, 0o755); err != nil {
		return err
	}
	ids := make([]int, 0, len(pf.Pack))
	for id := range pf.Pack {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	var buf strings.Builder
	for _, id := range ids {
		buf.WriteString(strconv.Itoa(id))
		buf.WriteByte('=')
		buf.WriteString(pf.Pack[id])
		buf.WriteByte('\n')
	}
	return os.WriteFile(filepath.Join(packDir, pf.Type+".pack"), []byte(buf.String()), 0o644)
}

// GetByID returns the name at id, or "" if absent.
func (pf *PackFile) GetByID(id int) string { return pf.Pack[id] }

// GetByName returns the id for name, or -1 if absent.
func (pf *PackFile) GetByName(name string) int {
	if id, ok := pf.NameToID[name]; ok {
		return id
	}
	return -1
}
