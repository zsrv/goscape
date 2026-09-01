package pack

import (
	"os"
	"path/filepath"
)

// GetModified returns the mtime of path in milliseconds since epoch,
// or 0 if the path is missing.
//
// TS source: tools/pack/PackFile.ts:getModified.
func GetModified(path string) int64 {
	if !FileExists(path) {
		return 0
	}
	info, err := FileStat(path)
	if err != nil {
		return 0
	}
	return info.ModTime().UnixMilli()
}

// GetLatestModified returns the most-recent mtime (ms) across all
// files under path with the given extension.
//
// TS source: tools/pack/PackFile.ts:getLatestModified.
func GetLatestModified(path, ext string) int64 {
	var latest int64
	for _, f := range ListFilesExt(path, ext) {
		info, err := FileStat(f)
		if err != nil {
			continue
		}
		if m := info.ModTime().UnixMilli(); m > latest {
			latest = m
		}
	}
	return latest
}

// ShouldBuild returns true if out is missing or older than the
// most-recent matching-extension file under srcPath.
//
// TS source: tools/pack/PackFile.ts:shouldBuild.
func ShouldBuild(srcPath, ext, out string) bool {
	// PSG: a packer-format change makes every output stale regardless of
	// mtimes. See FormatVersion in format_stamp.go.
	if ForceRebuild() {
		return true
	}

	if !FileExists(out) {
		return true
	}
	info, err := FileStat(out)
	if err != nil {
		return true
	}
	return info.ModTime().UnixMilli() < GetLatestModified(srcPath, ext)
}

// ShouldBuildFile returns true if dest is missing or older than src.
//
// TS source: tools/pack/PackFile.ts:shouldBuildFile.
func ShouldBuildFile(src, dest string) bool {
	// PSG: a packer-format change makes every output stale regardless of
	// mtimes. See FormatVersion in format_stamp.go.
	if ForceRebuild() {
		return true
	}

	if !FileExists(dest) {
		return true
	}
	destInfo, err := FileStat(dest)
	if err != nil {
		return true
	}
	srcInfo, err := FileStat(src)
	if err != nil {
		return true
	}
	return destInfo.ModTime().UnixMilli() < srcInfo.ModTime().UnixMilli()
}

// ShouldBuildFileAny returns true if dest is missing or older than ANY
// file (recursive) under path.
//
// TS source: tools/pack/PackFile.ts:shouldBuildFileAny.
func ShouldBuildFileAny(path, dest string) bool {
	// PSG: a packer-format change makes every output stale regardless of
	// mtimes. See FormatVersion in format_stamp.go.
	if ForceRebuild() {
		return true
	}

	if !FileExists(dest) {
		return true
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return false
	}
	for _, e := range entries {
		target := filepath.Join(path, e.Name())
		if e.IsDir() {
			if ShouldBuildFileAny(target, dest) {
				return true
			}
		} else {
			if ShouldBuildFile(target, dest) {
				return true
			}
		}
	}
	return false
}
