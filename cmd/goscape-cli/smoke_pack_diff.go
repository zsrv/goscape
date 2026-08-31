package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
)

// stageSnapshot maps a forward-slash relpath under outDir to the sha256
// hex of the regular file's contents. Used to detect which files a stage
// added or modified relative to the prior stage's snapshot.
type stageSnapshot map[string]string

// snapshotOutDir walks dir recursively and returns a stageSnapshot
// keyed by forward-slash relpaths. Missing dir → empty map, nil error
// (parity with walkOutDir's tolerance during stage failures).
// Symbolic links and non-regular files are skipped.
func snapshotOutDir(dir string) (stageSnapshot, error) {
	out := stageSnapshot{}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		if !d.Type().IsRegular() {
			return nil
		}
		rel, relErr := filepath.Rel(dir, path)
		if relErr != nil {
			return relErr
		}
		b, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		sum := sha256.Sum256(b)
		out[filepath.ToSlash(rel)] = hex.EncodeToString(sum[:])
		return nil
	})
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return out, err
	}
	return out, nil
}

// deltaFiles returns the sorted slice of relpaths in next whose hash
// differs from prev[relpath] — i.e. files this stage added or modified.
// Files present in prev but absent from next (deletions) are excluded;
// the smoke audits production, not removal.
func deltaFiles(prev, next stageSnapshot) []string {
	var out []string
	for rel, h := range next {
		if prev[rel] != h {
			out = append(out, rel)
		}
	}
	slices.Sort(out)
	return out
}

// fileDiff describes one byte-level divergence between an outDir file
// and its same-relpath counterpart under refDir.
type fileDiff struct {
	Path    string // relpath under outDir / refDir
	Kind    string // "DIFF" | "SIZE" | "MISS" | "ERR"
	Offset  int64  // first-mismatch byte offset (Kind=="DIFF")
	Got     byte   // outDir byte at Offset
	Want    byte   // refDir byte at Offset
	OutSize int64  // Kind=="SIZE"
	RefSize int64  // Kind=="SIZE"
	Note    string // Kind=="ERR" — wrapped reason
}

// diffOneFile compares two regular files byte-by-byte. Returns nil
// (matching files) or a *fileDiff describing the kind of divergence.
// Path is left blank for the caller to fill in with the canonical
// relpath.
func diffOneFile(outPath, refPath string) (*fileDiff, error) {
	refInfo, refErr := os.Stat(refPath)
	if refErr != nil {
		if errors.Is(refErr, fs.ErrNotExist) {
			return &fileDiff{Kind: "MISS"}, nil
		}
		return &fileDiff{Kind: "ERR", Note: refErr.Error()}, nil
	}
	outBytes, outErr := os.ReadFile(outPath)
	if outErr != nil {
		return nil, fmt.Errorf("read out %s: %w", outPath, outErr)
	}
	refBytes, refReadErr := os.ReadFile(refPath)
	if refReadErr != nil {
		return &fileDiff{Kind: "ERR", Note: refReadErr.Error()}, nil
	}
	if int64(len(outBytes)) != refInfo.Size() {
		return &fileDiff{Kind: "SIZE", OutSize: int64(len(outBytes)), RefSize: refInfo.Size()}, nil
	}
	for i := range outBytes {
		if outBytes[i] != refBytes[i] {
			return &fileDiff{Kind: "DIFF", Offset: int64(i), Got: outBytes[i], Want: refBytes[i]}, nil
		}
	}
	return nil, nil
}
