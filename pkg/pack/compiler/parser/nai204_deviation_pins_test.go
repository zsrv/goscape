package parser

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// readAllGoFiles returns the concatenated contents of every non-test
// .go file in dir (non-recursive). Test helper for grepping for
// documentation markers across the package.
func readAllGoFiles(t *testing.T, dir string) string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir %s: %v", dir, err)
	}
	var sb strings.Builder
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !strings.HasSuffix(e.Name(), ".go") {
			continue
		}
		if strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		sb.Write(b)
		sb.WriteByte('\n')
	}
	return sb.String()
}

// TestPin_NAI204D_AstNoVisitor pins that the ast package's Node
// interface doc-comment carries the NAI-204-D-AST-NO-VISITOR tag.
// Removing the tag without re-introducing a Visitor pattern is a bug.
func TestPin_NAI204D_AstNoVisitor(t *testing.T) {
	src := readAllGoFiles(t, "../ast")
	if !strings.Contains(src, "NAI-204-D-AST-NO-VISITOR") {
		t.Fatal("ast package missing deviation tag NAI-204-D-AST-NO-VISITOR")
	}
}

func TestPin_NAI204D_AstNoParent(t *testing.T) {
	src := readAllGoFiles(t, "../ast")
	if !strings.Contains(src, "NAI-204-D-AST-NO-PARENT") {
		t.Fatal("ast package missing deviation tag NAI-204-D-AST-NO-PARENT")
	}
}

func TestPin_NAI204D_AstNoAttributes(t *testing.T) {
	src := readAllGoFiles(t, "../ast")
	if !strings.Contains(src, "NAI-204-D-AST-NO-ATTRIBUTES") {
		t.Fatal("ast package missing deviation tag NAI-204-D-AST-NO-ATTRIBUTES")
	}
}

func TestPin_NAI204D_AstNoTypeFields(t *testing.T) {
	src := readAllGoFiles(t, "../ast")
	if !strings.Contains(src, "NAI-204-D-AST-NO-TYPE-FIELDS") {
		t.Fatal("ast package missing deviation tag NAI-204-D-AST-NO-TYPE-FIELDS")
	}
}

func TestPin_NAI204D_ParserPanicSync(t *testing.T) {
	src := readAllGoFiles(t, ".")
	if !strings.Contains(src, "NAI-204-D-PARSER-PANIC-SYNC") {
		t.Fatal("parser package missing deviation tag NAI-204-D-PARSER-PANIC-SYNC")
	}
}
