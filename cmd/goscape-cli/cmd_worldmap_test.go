package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestRunWorldmap_FlagParseError_ReturnsExit2(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	if got := runWorldmap([]string{"--unknown-flag"}, &stdout, &stderr); got != 2 {
		t.Errorf("exit = %d, want 2", got)
	}
}

func TestRunWorldmap_Help_ReturnsExit0(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	if got := runWorldmap([]string{"-h"}, &stdout, &stderr); got != 0 {
		t.Errorf("exit = %d, want 0", got)
	}
}

func TestRunWorldmap_MissingMapsDir_NoOpReturnsExit0(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	tmp := t.TempDir()
	args := []string{
		"--src-dir", tmp,
		"--out-dir", tmp,
		"--log.level", "error",
	}
	if got := runWorldmap(args, &stdout, &stderr); got != 0 {
		t.Errorf("exit = %d, want 0; stderr=%s", got, stderr.String())
	}
}

func TestRunWorldmap_PackErrorReturnsExit1(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	tmp := t.TempDir()
	// Create outDir/server/maps so the early-return doesn't fire,
	// but leave srcDir/maps absent so the CSV read errors out.
	if err := os.MkdirAll(filepath.Join(tmp, "server", "maps"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	args := []string{
		"--src-dir", tmp,
		"--out-dir", tmp,
		"--log.level", "error",
	}
	if got := runWorldmap(args, &stdout, &stderr); got != 1 {
		t.Errorf("exit = %d, want 1; stderr=%s", got, stderr.String())
	}
}

func TestDispatch_WorldmapRegistered(t *testing.T) {
	t.Parallel()
	found := false
	for _, v := range verbs {
		if v.name == "worldmap" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("worldmap verb not registered in verbs slice")
	}
}
