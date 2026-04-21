package script

import (
	"encoding/binary"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
)

// CompilerVersion is the expected RuneScript compiler version stored in script.dat.
// Any mismatch is a fatal configuration error.
const CompilerVersion = 26

// Provider holds all loaded scripts indexed by lookup key and name.
type Provider struct {
	scripts []*ScriptFile
	byKey   map[uint32]*ScriptFile
	byName  map[string]*ScriptFile
}

// NewProvider allocates an empty Provider.
func NewProvider() *Provider {
	return &Provider{
		byKey:  make(map[uint32]*ScriptFile),
		byName: make(map[string]*ScriptFile),
	}
}

// Load reads script.dat and script.idx from cacheDir, validates the compiler
// version, decodes every non-empty entry, and populates the lookup tables.
//
// cacheDir is typically "data/pack/server" relative to the project root.
//
// The script.dat format is:
//
//	[u32 entryCount][u32 version][blob_0][blob_1]...
//
// The script.idx format is:
//
//	[u32 entryCount (ignored)][u32 size_0][u32 size_1]...
func (p *Provider) Load(cacheDir string) error {
	datPath := filepath.Join(cacheDir, "script.dat")
	idxPath := filepath.Join(cacheDir, "script.idx")

	dat, err := os.ReadFile(datPath)
	if err != nil {
		return fmt.Errorf("script.Load: read dat: %w", err)
	}
	idx, err := os.ReadFile(idxPath)
	if err != nil {
		return fmt.Errorf("script.Load: read idx: %w", err)
	}

	if len(dat) < 8 {
		return fmt.Errorf("script.Load: dat file too short (%d bytes)", len(dat))
	}
	if len(idx) < 4 {
		return fmt.Errorf("script.Load: idx file too short (%d bytes)", len(idx))
	}

	entryCount := int(binary.BigEndian.Uint32(dat[0:4]))
	version := int(binary.BigEndian.Uint32(dat[4:8]))
	if version != CompilerVersion {
		return fmt.Errorf("script.Load: compiler version mismatch: got %d, want %d", version, CompilerVersion)
	}

	// idx: first 4 bytes = entryCount (skip), then u32 per entry.
	idxOffset := 4
	datOffset := 8

	p.scripts = make([]*ScriptFile, entryCount)

	for id := range entryCount {
		if idxOffset+4 > len(idx) {
			break
		}
		size := int(binary.BigEndian.Uint32(idx[idxOffset : idxOffset+4]))
		idxOffset += 4

		if size == 0 {
			continue
		}
		if datOffset+size > len(dat) {
			slog.Warn("script.Load: dat truncated", "id", id, "need", size)
			continue
		}

		blob := dat[datOffset : datOffset+size]
		datOffset += size

		f, err := Decode(blob)
		if err != nil {
			slog.Warn("script.Load: decode failed", "id", id, "err", err)
			continue
		}

		p.scripts[id] = f
		p.byName[f.Name] = f
		if f.LookupKey != 0xFFFFFFFF {
			p.byKey[f.LookupKey] = f
		}
	}

	return nil
}

// GetByTrigger performs the TS-standard three-step lookup for a trigger + type/category:
//  1. Specific: trigger | (0x2 << 8) | (typeID << 10)
//  2. Category: trigger | (0x1 << 8) | (categoryID << 10)
//  3. Global:   trigger
//
// Returns nil if no script is found at any level.
func (p *Provider) GetByTrigger(trigger ServerTriggerType, typeID, categoryID int) *ScriptFile {
	specific := uint32(trigger) | (0x2 << 8) | (uint32(typeID) << 10)
	if f, ok := p.byKey[specific]; ok {
		return f
	}
	category := uint32(trigger) | (0x1 << 8) | (uint32(categoryID) << 10)
	if f, ok := p.byKey[category]; ok {
		return f
	}
	if f, ok := p.byKey[uint32(trigger)]; ok {
		return f
	}
	return nil
}

// GetByName returns the script with the given identifier, or nil if not found.
func (p *Provider) GetByName(name string) *ScriptFile {
	return p.byName[name]
}

// GetByLookupKey returns a script by its raw uint32 key (as stored in
// byKey). Returns nil if unknown. Used for trigger-based dispatch
// where the scriptID comes from an encoded (trigger | typeID) key.
func (p *Provider) GetByLookupKey(key uint32) *ScriptFile {
	return p.byKey[key]
}

// GetByID returns the script at the given script-id slot (the zero-
// based index in the loaded scripts array). Used by GOSUB_WITH_PARAMS,
// JUMP_WITH_PARAMS, QUEUE and similar opcodes whose operand is a raw
// script id, matching TS ScriptProvider.get(id). Returns nil if id is
// out of range or the script at that slot failed to decode.
func (p *Provider) GetByID(id uint32) *ScriptFile {
	if int(id) < 0 || int(id) >= len(p.scripts) {
		return nil
	}
	return p.scripts[id]
}

// Register adds a pre-built ScriptFile to the provider. Intended for tests
// that want to exercise the provider without loading a real cache.
// Duplicate names/keys overwrite; the caller is responsible.
func (p *Provider) Register(f *ScriptFile) {
	p.scripts = append(p.scripts, f)
	if f.Name != "" {
		p.byName[f.Name] = f
	}
	if f.LookupKey != 0xFFFFFFFF {
		p.byKey[f.LookupKey] = f
	}
}

// RegisterAt places a pre-built ScriptFile at the given script-id slot,
// growing the scripts slice as needed. Used by tests that target
// specific script ids via GOSUB/JUMP/QUEUE/TIMER ops which look up by
// id, not by LookupKey.
func (p *Provider) RegisterAt(id uint32, f *ScriptFile) {
	for uint32(len(p.scripts)) <= id {
		p.scripts = append(p.scripts, nil)
	}
	p.scripts[id] = f
	if f.Name != "" {
		p.byName[f.Name] = f
	}
	if f.LookupKey != 0xFFFFFFFF {
		p.byKey[f.LookupKey] = f
	}
}

// Count returns the number of successfully loaded scripts.
func (p *Provider) Count() int {
	count := 0
	for _, f := range p.scripts {
		if f != nil {
			count++
		}
	}
	return count
}
