package pack

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// trivialParse is a ParseFn used in read_typed tests: it accepts every
// key and returns the substituted value verbatim.
func trivialParse(key, value string) (ConfigValue, bool, error) {
	return value, true, nil
}

func TestReadTypedConfigs_HappyPath(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "scripts", "test.varn"),
		"[npctier]\ntype=int\n[npchealth]\ntype=int\n")
	ClearFsCache()
	cfgs, err := ReadTypedConfigs(dir, ".varn", nil, trivialParse, Constants{})
	if err != nil {
		t.Fatal(err)
	}
	if len(cfgs) != 2 {
		t.Fatalf("got %d configs, want 2", len(cfgs))
	}
	if got := cfgs["npctier"]; len(got) != 1 || got[0].Key != "type" || got[0].Value != "int" {
		t.Fatalf("npctier=%v", got)
	}
	if got := cfgs["npchealth"]; len(got) != 1 || got[0].Key != "type" || got[0].Value != "int" {
		t.Fatalf("npchealth=%v", got)
	}
}

func TestReadTypedConfigs_ConstantsSubstitution(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "scripts", "test.varn"),
		"[npctier]\ntype=^MYTYPE\n")
	ClearFsCache()
	c := Constants{"MYTYPE": "int"}
	cfgs, err := ReadTypedConfigs(dir, ".varn", nil, trivialParse, c)
	if err != nil {
		t.Fatal(err)
	}
	if cfgs["npctier"][0].Value != "int" {
		t.Fatalf("substituted value=%v, want \"int\"", cfgs["npctier"][0].Value)
	}
}

func TestReadTypedConfigs_MissingSeparatorErrors(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "scripts", "test.varn"),
		"[npctier]\nno_equals_here\n")
	ClearFsCache()
	_, err := ReadTypedConfigs(dir, ".varn", nil, trivialParse, Constants{})
	if err == nil || !strings.Contains(err.Error(), "missing property separator") {
		t.Fatalf("err=%v", err)
	}
}

func TestReadTypedConfigs_DuplicateNameErrors(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "scripts", "test.varn"),
		"[npctier]\ntype=int\n[npctier]\ntype=string\n")
	ClearFsCache()
	_, err := ReadTypedConfigs(dir, ".varn", nil, trivialParse, Constants{})
	if err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("err=%v", err)
	}
}

func TestReadTypedConfigs_MissingClosingBracketErrors(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "scripts", "test.varn"),
		"[npctier\ntype=int\n")
	ClearFsCache()
	_, err := ReadTypedConfigs(dir, ".varn", nil, trivialParse, Constants{})
	if err == nil || !strings.Contains(err.Error(), "missing closing bracket") {
		t.Fatalf("err=%v", err)
	}
}

func TestReadTypedConfigs_EmptyNameErrors(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "scripts", "test.varn"),
		"[]\ntype=int\n")
	ClearFsCache()
	_, err := ReadTypedConfigs(dir, ".varn", nil, trivialParse, Constants{})
	if err == nil || !strings.Contains(err.Error(), "empty config name") {
		t.Fatalf("err=%v", err)
	}
}

func TestReadTypedConfigs_ParseFnOkFalseInvalidKey(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "scripts", "test.varn"),
		"[npctier]\nbad_key=anything\n")
	ClearFsCache()
	parseFn := func(key, value string) (ConfigValue, bool, error) {
		return nil, false, nil
	}
	_, err := ReadTypedConfigs(dir, ".varn", nil, parseFn, Constants{})
	if err == nil || !strings.Contains(err.Error(), "invalid property key") {
		t.Fatalf("err=%v", err)
	}
}

func TestReadTypedConfigs_ParseFnErrorInvalidValue(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "scripts", "test.varn"),
		"[npctier]\ntype=bogus\n")
	ClearFsCache()
	parseFn := func(key, value string) (ConfigValue, bool, error) {
		return nil, true, errors.New("rejected")
	}
	_, err := ReadTypedConfigs(dir, ".varn", nil, parseFn, Constants{})
	if err == nil || !strings.Contains(err.Error(), "invalid property value") {
		t.Fatalf("err=%v", err)
	}
}

func TestReadTypedConfigs_RequiredPropertyMissingErrors(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "scripts", "test.varn"),
		"[npctier]\nother=ignored\n")
	ClearFsCache()
	_, err := ReadTypedConfigs(dir, ".varn", []string{"type"}, trivialParse, Constants{})
	if err == nil || !strings.Contains(err.Error(), "missing required property") {
		t.Fatalf("err=%v", err)
	}
}

func TestReadTypedConfigs_RequiredPropertyPresentAtFileEnd(t *testing.T) {
	// Required-property check must run at file-end, not just on the next
	// [header] line.
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "scripts", "test.varn"),
		"[npctier]\nother=ignored\n") // file ends mid-config
	ClearFsCache()
	_, err := ReadTypedConfigs(dir, ".varn", []string{"type"}, trivialParse, Constants{})
	if err == nil {
		t.Fatal("want missing-required-property error at file end")
	}
}

func TestReadTypedConfigs_MissingScriptsDirReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	ClearFsCache()
	cfgs, err := ReadTypedConfigs(dir, ".varn", nil, trivialParse, Constants{})
	if err != nil {
		t.Fatal(err)
	}
	if len(cfgs) != 0 {
		t.Fatalf("want empty, got %v", cfgs)
	}
}

// TestReadTypedConfigs_PreservesTrailingWhitespace pins that a config value's
// trailing whitespace survives parsing. TS PackShared.readConfigs
// (PackShared.ts:217-260 @1d25566c) reads raw readline output and slices at the
// first '=', so `param=name,value ` keeps its trailing space; config files are
// never routed through loadFileFull's per-line .trim().
//
// This closes NAI-192-D-COMMENT-STRIP-EAGER's "harmless" claim, which Content
// 2b62ae68d falsified: fishing_equipment.struct's fish_equipment_big_net
// failmessage gained a trailing space, and goscape's trim made
// server/struct.dat one byte shorter than the reference.
func TestReadTypedConfigs_PreservesTrailingWhitespace(t *testing.T) {
	dir := t.TempDir()
	scripts := filepath.Join(dir, "scripts")
	if err := os.MkdirAll(scripts, 0o755); err != nil {
		t.Fatal(err)
	}
	src := "[block]\nkey=value with trailing space \n"
	if err := os.WriteFile(filepath.Join(scripts, "a.tst"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	cfgs, err := ReadTypedConfigs(dir, ".tst", nil, trivialParse, Constants{})
	if err != nil {
		t.Fatalf("ReadTypedConfigs: %v", err)
	}

	lines := cfgs["block"]
	if len(lines) != 1 {
		t.Fatalf("lines: got %d, want 1", len(lines))
	}
	got := lines[0].Value.(string)
	want := "value with trailing space "
	if got != want {
		t.Errorf("value: got %q, want %q", got, want)
	}
}

// TestReadTypedConfigs_SkipsCommentAndEmptyLines pins the rest of the TS
// readConfigs line contract: fully-empty lines and lines starting with '//' are
// skipped, and nothing else is stripped.
func TestReadTypedConfigs_SkipsCommentAndEmptyLines(t *testing.T) {
	dir := t.TempDir()
	scripts := filepath.Join(dir, "scripts")
	if err := os.MkdirAll(scripts, 0o755); err != nil {
		t.Fatal(err)
	}
	src := "// leading comment\n\n[block]\n// about the next line\nkey=value\n\n"
	if err := os.WriteFile(filepath.Join(scripts, "a.tst"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	cfgs, err := ReadTypedConfigs(dir, ".tst", nil, trivialParse, Constants{})
	if err != nil {
		t.Fatalf("ReadTypedConfigs: %v", err)
	}
	lines := cfgs["block"]
	if len(lines) != 1 {
		t.Fatalf("lines: got %d, want 1 (%+v)", len(lines), lines)
	}
	if got := lines[0].Value.(string); got != "value" {
		t.Errorf("value: got %q, want %q", got, "value")
	}
}
