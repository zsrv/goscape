package pack

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// scanPkgPack reads every .go file under pkg/pack/ and returns concatenated
// content. Used by absence/presence pins on doc-comment tags.
// Excludes _test.go files (pins are for production-code doc-comment state).
func scanPkgPack(t *testing.T) string {
	t.Helper()
	var sb strings.Builder
	err := filepath.WalkDir("..", func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel("..", path)
		if err != nil {
			return err
		}
		if strings.HasPrefix(rel, "pack/") && strings.HasSuffix(rel, ".go") && !strings.HasSuffix(rel, "_test.go") {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			sb.Write(data)
			sb.WriteString("\n")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk pkg/pack: %v", err)
	}
	return sb.String()
}

func TestNAI196_AbsencePin_ParamAfterVars(t *testing.T) {
	src := scanPkgPack(t)
	if strings.Contains(src, "NAI-194-D-PARAM-AFTER-VARS") {
		t.Error("NAI-194-D-PARAM-AFTER-VARS tag should be retired but still appears in pkg/pack production code")
	}
}

func TestNAI196_AbsencePin_ConfigOrderExtends(t *testing.T) {
	src := scanPkgPack(t)
	if strings.Contains(src, "NAI-195-D-CONFIG-ORDER-EXTENDS-PARAM-AFTER-VARS") {
		t.Error("NAI-195-D-CONFIG-ORDER-EXTENDS-PARAM-AFTER-VARS tag should be retired but still appears")
	}
}

func TestNAI196_AbsencePin_FreshClientJagfile(t *testing.T) {
	src := scanPkgPack(t)
	if strings.Contains(src, "NAI-193-D-FRESH-CLIENT-JAGFILE") {
		t.Error("NAI-193-D-FRESH-CLIENT-JAGFILE tag should be retired but still appears")
	}
}

func TestNAI196_PresencePin_UnconditionalClientPack(t *testing.T) {
	src := scanPkgPack(t)
	if !strings.Contains(src, "NAI-196-D-UNCONDITIONAL-CLIENT-PACK") {
		t.Error("NAI-196-D-UNCONDITIONAL-CLIENT-PACK tag should be documented in pkg/pack production code but is absent")
	}
}

func TestNAI196_SanityPin_NoClientJagDirty(t *testing.T) {
	src := scanPkgPack(t)
	if strings.Contains(src, "clientJagDirty") {
		t.Error("clientJagDirty identifier should be removed but still appears in pkg/pack production code")
	}
}
