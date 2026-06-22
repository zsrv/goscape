package compiler

// TestWriteCompilerSymbols_RefParity validates that WriteCompilerSymbols
// produces .sym files byte-identical to the reference output at
// Server245.2-ref/engine/data/symbols/*.sym.
//
// Activation: set GOSCAPE_REF245_DIR to the engine root of the reference
// checkout (the directory that contains data/pack and data/symbols):
//
//	GOSCAPE_REF245_DIR=/path/to/Server245.2-ref/engine \
//	  go test ./pkg/pack/compiler/ -run TestWriteCompilerSymbols_RefParity -v
//
// Without the env var the test is skipped (clean CI).
//
// Directory conventions assumed:
//   - refDir/data/pack    — packed server .dat files (outDir for loaders)
//   - refDir/../content   — srcDir (has scripts/ and pack/)
//   - refDir/data/symbols — reference .sym files to compare against
import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestWriteCompilerSymbols_RefParity(t *testing.T) {
	refDir := os.Getenv("GOSCAPE_REF245_DIR")
	if refDir == "" {
		t.Skip("GOSCAPE_REF245_DIR not set — skipping ref-parity test")
	}

	// srcDir = refDir/../content (has scripts/ and pack/ sub-dirs)
	srcDir := filepath.Join(refDir, "..", "content")
	// outDir = refDir/data/pack (packed .dat files for config loaders)
	outDir := filepath.Join(refDir, "data", "pack")
	// refSymDir = refDir/data/symbols (32 reference .sym files)
	refSymDir := filepath.Join(refDir, "data", "symbols")

	// Write symbols into a temp dir.
	tmpDir := t.TempDir()
	if err := WriteCompilerSymbols(srcDir, outDir, tmpDir); err != nil {
		t.Fatalf("WriteCompilerSymbols: %v", err)
	}

	// Collect reference .sym files.
	refEntries, err := os.ReadDir(refSymDir)
	if err != nil {
		t.Fatalf("ReadDir %s: %v", refSymDir, err)
	}
	var refSymFiles []string
	for _, e := range refEntries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sym") {
			refSymFiles = append(refSymFiles, e.Name())
		}
	}
	sort.Strings(refSymFiles)

	t.Logf("comparing %d .sym files from %s", len(refSymFiles), refSymDir)

	// For each reference file, compare against the generated output.
	// Track residuals for the final report (stop after 3 unexplained diffs).
	type residual struct {
		name string
		diff string
	}
	var residuals []residual

	for _, fname := range refSymFiles {
		refPath := filepath.Join(refSymDir, fname)
		gotPath := filepath.Join(tmpDir, fname)

		refData, err := os.ReadFile(refPath)
		if err != nil {
			t.Errorf("%s: cannot read reference: %v", fname, err)
			continue
		}
		gotData, err := os.ReadFile(gotPath)
		if err != nil {
			// File not generated.
			residuals = append(residuals, residual{
				name: fname,
				diff: fmt.Sprintf("NOT GENERATED (WriteCompilerSymbols did not produce this file)"),
			})
			if len(residuals) >= 3 {
				break
			}
			continue
		}

		if bytes.Equal(refData, gotData) {
			t.Logf("  MATCH  %s", fname)
			continue
		}

		// Files differ — build a line-level diff summary.
		diff := lineDiff(refData, gotData)
		residuals = append(residuals, residual{name: fname, diff: diff})
		if len(residuals) >= 3 {
			break
		}
	}

	// Also check for generated files not in the reference.
	gotEntries, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("ReadDir %s: %v", tmpDir, err)
	}
	refSet := make(map[string]bool, len(refSymFiles))
	for _, f := range refSymFiles {
		refSet[f] = true
	}
	for _, e := range gotEntries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sym") && !refSet[e.Name()] {
			t.Errorf("generated extra file not in reference: %s", e.Name())
		}
	}

	if len(residuals) > 0 {
		for _, r := range residuals {
			t.Errorf("MISMATCH %s:\n%s", r.name, r.diff)
		}
		t.Fatalf("ref-parity FAILED: %d residual(s) — see DONE_WITH_CONCERNS notes above", len(residuals))
	}
}

// lineDiff returns a human-readable summary of the first N differing lines
// between ref and got.
func lineDiff(ref, got []byte) string {
	refLines := strings.Split(strings.ReplaceAll(string(ref), "\r\n", "\n"), "\n")
	gotLines := strings.Split(strings.ReplaceAll(string(got), "\r\n", "\n"), "\n")

	var sb strings.Builder
	maxLines := max(len(refLines), len(gotLines))
	shown := 0
	for i := range maxLines {
		var rl, gl string
		if i < len(refLines) {
			rl = refLines[i]
		}
		if i < len(gotLines) {
			gl = gotLines[i]
		}
		if rl != gl {
			fmt.Fprintf(&sb, "  line %d: ref=%q got=%q\n", i+1, rl, gl)
			shown++
			if shown >= 20 {
				fmt.Fprintf(&sb, "  ... (truncated after 20 differing lines)\n")
				break
			}
		}
	}
	fmt.Fprintf(&sb, "  ref lines=%d got lines=%d", len(refLines), len(gotLines))
	return sb.String()
}
