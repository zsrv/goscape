package pack

import (
	"errors"
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
