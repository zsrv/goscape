package gamemap

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestInitHandlesMissingDir(t *testing.T) {
	gm := New(discardLogger())
	err := gm.Init(t.TempDir())
	if err != nil {
		t.Errorf("Init on empty dir: got %v, want nil", err)
	}
}

func TestInitHandlesMissingCsv(t *testing.T) {
	tmp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmp, "maps"), 0755); err != nil {
		t.Fatal(err)
	}
	gm := New(discardLogger())
	err := gm.Init(tmp)
	if err != nil {
		t.Errorf("Init with missing CSVs: got %v, want nil", err)
	}
	if gm.IsMulti(1000, 2000, 0) {
		t.Error("IsMulti should default false when multimap CSV missing")
	}
	if gm.IsFreeToPlay(1000, 2000) {
		t.Error("IsFreeToPlay should default false when freemap CSV missing")
	}
}

func TestInitLoadsCsvMaps(t *testing.T) {
	tmp := t.TempDir()
	mapsDir := filepath.Join(tmp, "maps")
	if err := os.MkdirAll(mapsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mapsDir, "multiway.csv"), []byte("0,1000,2000\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mapsDir, "free2play.csv"), []byte("0,1500,2500\n"), 0644); err != nil {
		t.Fatal(err)
	}

	gm := New(discardLogger())
	if err := gm.Init(tmp); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if !gm.IsMulti(1000, 2000, 0) {
		t.Error("expected (1000,2000,0) to be multi")
	}
	if !gm.IsFreeToPlay(1500, 2500) {
		t.Error("expected (1500,2500) to be F2P")
	}
}
