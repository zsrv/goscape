package main

import (
	"bytes"
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
