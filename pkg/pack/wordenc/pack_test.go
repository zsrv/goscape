package wordenc

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zsrv/goscape/pkg/io/filestream"
)

// TestPack_WritesToCache pins the rev-244 behaviour: Pack reads the raw
// wordenc blob from <rawDir>/wordenc and writes it to cache(0, 7).
func TestPack_WritesToCache(t *testing.T) {
	// Build a synthetic wordenc blob (any non-empty bytes suffice for
	// the round-trip check; real format is validated by encfilter tests).
	blobBytes := []byte{0x01, 0x02, 0x03, 0x04}

	rawDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(rawDir, "wordenc"), blobBytes, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cacheDir := t.TempDir()
	fs := filestream.New(cacheDir, true, false)
	defer fs.Close()

	if err := Pack(rawDir, fs); err != nil {
		t.Fatalf("Pack: %v", err)
	}

	if !fs.Has(0, 7) {
		t.Fatal("cache(0,7) has no entry after Pack")
	}
	got := fs.Read(0, 7, false)
	if string(got) != string(blobBytes) {
		t.Errorf("cache(0,7) = %v, want %v", got, blobBytes)
	}
}

// TestPack_NilCache_NoError pins that Pack with a nil cache returns nil
// (file read succeeds, write is skipped).
func TestPack_NilCache_NoError(t *testing.T) {
	rawDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(rawDir, "wordenc"), []byte{0xAB}, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := Pack(rawDir, nil); err != nil {
		t.Errorf("Pack(nil cache): %v, want nil", err)
	}
}

// TestPack_MissingBlob_ReturnsError pins that a missing data/raw/wordenc
// blob causes Pack to return an error (fail-fast; no silent no-op).
func TestPack_MissingBlob_ReturnsError(t *testing.T) {
	rawDir := t.TempDir()
	// No wordenc file written.
	if err := Pack(rawDir, nil); err == nil {
		t.Error("Pack(missing blob): want error, got nil")
	}
}
