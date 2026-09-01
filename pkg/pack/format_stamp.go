package pack

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
)

// FormatVersion identifies the byte layout this packer produces.
//
// BUMP THIS whenever a change alters any packed byte — a config opcode moving,
// a jagfile's member order changing, a sidecar format changing, anything an
// output file would notice. It does not track the packer's source, only its
// observable format.
//
// Why it exists: incremental packing is keyed on SOURCE mtimes, so it cannot
// see that the PACKER changed. This was observed on rev-274, where an engine
// sync moved npc wanderrange/maxrange and extracheck_var to new opcodes and a
// plain `make pack` afterwards left 2,909 artifacts unrebuilt, including a
// hunt.dat still carrying pre-sync opcodes. The server then refused to boot on
// the loud path and would have served wrong bytes on the quiet one. Nothing
// about that hazard is specific to that sync or that branch: any change to a
// packed byte on any branch reaches the same trap, which is why the guard is
// back-ported here rather than left where it was found.
//
// You do not have to remember to bump this unprompted:
// TestPackFormatVersionGolden in format_stamp_test.go hashes the packers'
// output over fixed fixtures and fails, naming this constant, the moment a
// packed byte moves. That test is the enforceable half of this mechanism —
// see docs/superpowers/specs/2026-08-31-pack-staleness-guard-design.md §6.
//
// History:
//
//	1 — initial, at Engine-TS 3c16994c / Content cbcfe670.
const FormatVersion = 1

// formatStampName is the stamp file, placed under a .stamps/ directory to
// mirror TS's data/pack/.stamps/*.txt layout (FsCache.ts:165 @1d25566c) so the
// two trees stay comparable side by side. TS stamps
// "<packer source path>=<mtime>"; goscape stamps a format version instead —
// DEVIATION PSG-D2, forced by goscape shipping a compiled binary with no .go
// files at runtime. The mechanism is equivalent in effect; only the identity
// function differs.
const formatStampName = "pack-format.txt"

// forceRebuild latches "the packer format changed, treat every output as
// stale" for the duration of a pack run. It is process-global because the
// freshness helpers are free functions called from every stage; BeginPack sets
// it and returns a restore func.
//
// A pack run is not concurrent with another pack run in the same process
// (PackAll opens the cache with createNew=true, which truncates), so a single
// latch is sufficient. Tests that exercise the helpers directly can set it via
// SetForceRebuild.
var forceRebuild atomic.Bool

// ForceRebuild reports whether this run must rebuild every output regardless
// of mtimes. Stages that decide freshness WITHOUT the ShouldBuild* helpers
// must consult it explicitly — currently only the worldmap gate in
// pkg/pack/maps.
func ForceRebuild() bool { return forceRebuild.Load() }

// SetForceRebuild sets the latch and returns a func restoring the previous
// value. Exported for tests and for callers that orchestrate packing
// themselves.
func SetForceRebuild(v bool) (restore func()) {
	prev := forceRebuild.Swap(v)
	return func() { forceRebuild.Store(prev) }
}

// FormatStampPath is the stamp file's location for a given output directory.
func FormatStampPath(outDir string) string {
	return filepath.Join(outDir, ".stamps", formatStampName)
}

// ReadFormatStamp returns the format version recorded in outDir, and whether a
// readable stamp was found. A missing, unreadable or malformed stamp reports
// found=false, which BeginPack treats as stale — the safe direction, since an
// output tree we cannot identify is one we cannot trust.
func ReadFormatStamp(outDir string) (version int, found bool) {
	raw, err := os.ReadFile(FormatStampPath(outDir))
	if err != nil {
		return 0, false
	}
	_, value, ok := strings.Cut(strings.TrimSpace(string(raw)), "=")
	if !ok {
		return 0, false
	}
	v, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0, false
	}
	return v, true
}

// WriteFormatStamp records FormatVersion for outDir. Call it only after a
// SUCCESSFUL pack: a stamp written up front would claim the tree matches this
// packer even if the run then failed halfway, leaving genuinely stale outputs
// behind an all-clear.
func WriteFormatStamp(outDir string) error {
	path := FormatStampPath(outDir)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("WriteFormatStamp: %w", err)
	}
	// Keep TS's "key=value" shape so the file reads the same way theirs does.
	if err := os.WriteFile(path, []byte(fmt.Sprintf("format=%d\n", FormatVersion)), 0o644); err != nil {
		return fmt.Errorf("WriteFormatStamp: %w", err)
	}
	return nil
}

// BeginPack compares outDir's recorded format against this binary's and, on a
// mismatch, latches a full rebuild for the run. It returns whether the rebuild
// was forced (for logging) and a restore func the caller must defer.
//
// An output tree with NO stamp is treated as stale. That makes the first pack
// after this guard ships a full one, which is correct: those trees were built
// by a packer that never recorded its format, so nothing is known about them.
func BeginPack(outDir string) (forced bool, restore func()) {
	version, found := ReadFormatStamp(outDir)
	stale := !found || version != FormatVersion
	restore = SetForceRebuild(stale)
	return stale, restore
}
