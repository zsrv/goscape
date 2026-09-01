package pack

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func writeScript(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCrawlConfigNamesReturnsHeadersInOrder(t *testing.T) {
	srcDir := t.TempDir()
	scripts := filepath.Join(srcDir, "scripts")
	writeScript(t, scripts, "a.obj", "[coins]\nmodel=c\n[bronze_dagger]\nmodel=bd")
	ClearFsCache()
	got, err := CrawlConfigNames(srcDir, ".obj", false)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"coins", "bronze_dagger"}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestCrawlConfigNamesDedups(t *testing.T) {
	srcDir := t.TempDir()
	scripts := filepath.Join(srcDir, "scripts")
	writeScript(t, scripts, "a.obj", "[coins]\nmodel=a")
	writeScript(t, scripts, "b.obj", "[coins]\nmodel=b\n[oak_log]\nmodel=o")
	ClearFsCache()
	got, err := CrawlConfigNames(srcDir, ".obj", false)
	if err != nil {
		t.Fatal(err)
	}
	// "coins" appears once; "oak_log" follows.
	if len(got) != 2 {
		t.Fatalf("got %d names (want 2): %v", len(got), got)
	}
	if !slices.Contains(got, "coins") || !slices.Contains(got, "oak_log") {
		t.Fatalf("missing names: %v", got)
	}
}

func TestCrawlConfigNamesSkipsEngineRs2(t *testing.T) {
	srcDir := t.TempDir()
	scripts := filepath.Join(srcDir, "scripts")
	writeScript(t, scripts, "engine.rs2", "[command_signature]\nfoo=bar")
	writeScript(t, scripts, "real.rs2", "[real_proc]\nfoo=bar")
	ClearFsCache()
	got, err := CrawlConfigNames(srcDir, ".rs2", false)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"real_proc"}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v (engine.rs2 must be skipped)", got, want)
	}
}

func TestCrawlConfigNamesIncludeBrackets(t *testing.T) {
	srcDir := t.TempDir()
	scripts := filepath.Join(srcDir, "scripts")
	writeScript(t, scripts, "a.obj", "[coins]\nmodel=c")
	ClearFsCache()
	got, err := CrawlConfigNames(srcDir, ".obj", true)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"[coins]"}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

// TestCrawlConfigCategoriesOrderAndDedup pins the category-name harvest.
// Categories have no source files of their own — Content ships zero
// .category files — so their names are collected from `category=` lines in
// .loc, .npc and .obj, in that file-extension order, first-seen wins.
//
// TS source: tools/pack/PackFile.ts:crawlConfigCategories.
func TestCrawlConfigCategoriesOrderAndDedup(t *testing.T) {
	srcDir := t.TempDir()
	scripts := filepath.Join(srcDir, "scripts")
	writeScript(t, scripts, "a.loc", "[gate]\ncategory=hemenster_gate\n")
	writeScript(t, scripts, "a.npc", "[guard]\ncategory=guards\n")
	// duplicate across families, plus a new one from .obj
	writeScript(t, scripts, "a.obj", "[key]\ncategory=hemenster_gate\n[log]\ncategory=logs\n")
	ClearFsCache()

	got, err := CrawlConfigCategories(srcDir)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"hemenster_gate", "guards", "logs"}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}
