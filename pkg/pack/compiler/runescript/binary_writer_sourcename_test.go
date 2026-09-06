// pkg/pack/compiler/runescript/binary_writer_sourcename_test.go
package runescript_test

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/zsrv/goscape/pkg/pack/compiler/codegen"
	"github.com/zsrv/goscape/pkg/pack/compiler/runescript"
	"github.com/zsrv/goscape/pkg/pack/compiler/symbol"
	"github.com/zsrv/goscape/pkg/pack/compiler/trigger"
	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

// scriptWithSource builds a minimal one-block script carrying an explicit
// SourceName, which minimalScript hard-codes.
func scriptWithSource(t *testing.T, sourceName string) *codegen.RuneScript {
	t.Helper()
	procTrig := &trigger.TriggerType{ID: 5, Identifier: "proc", SubjectMode: trigger.ModeName, AllowParameters: true, AllowReturns: true}
	ss := &symbol.ServerScriptSymbol{
		Trigger:    procTrig,
		Name:       "foo",
		Parameters: typ.MetaUnit,
		Returns:    typ.MetaUnit}
	s := codegen.NewRuneScript(sourceName, ss, procTrig, "foo", nil)
	s.Blocks = []*codegen.Block{codegen.NewBlock(&codegen.Label{Name: "e"})}
	return s
}

// headerSourceName decodes the second NUL-terminated string of a Finish()
// blob, which is the source name (the first is FullName).
func headerSourceName(t *testing.T, blob []byte) string {
	t.Helper()
	i := bytes.IndexByte(blob, 0)
	if i < 0 {
		t.Fatalf("no NUL terminator for fullName in %x", blob)
	}
	rest := blob[i+1:]
	j := bytes.IndexByte(rest, 0)
	if j < 0 {
		t.Fatalf("no NUL terminator for sourceName in %x", rest)
	}
	return string(rest[:j])
}

func writeWithRoots(t *testing.T, roots []string, sourceName string) []byte {
	t.Helper()
	out := &recOutput{}
	w := runescript.NewBinaryScriptWriter(stubIdProvider{}, out)
	w.SourceRoots = roots
	w.Write(scriptWithSource(t, sourceName))
	return out.data
}

// TestBinaryWriter_SourceNameRelativeToRoot pins that a script under a source
// root is recorded by its root-relative path, not the absolute one it was
// parsed from.
func TestBinaryWriter_SourceNameRelativeToRoot(t *testing.T) {
	root := filepath.Join(string(filepath.Separator), "tmp", "tmp.abc123", "scripts")
	src := filepath.Join(root, "areas", "area_alkharid", "dommik.rs2")

	got := headerSourceName(t, writeWithRoots(t, []string{root}, src))

	want := "areas/area_alkharid/dommik.rs2"
	if got != want {
		t.Errorf("sourceName = %q, want %q", got, want)
	}
}

// TestBinaryWriter_SourceNameIndependentOfRootPath is the regression this
// change exists for: the same sources packed from two differently-named
// checkout directories must produce byte-identical script blobs. Before the
// fix the absolute path went into the header, so every `mktemp -d` clone
// packed to a different bundle digest.
func TestBinaryWriter_SourceNameIndependentOfRootPath(t *testing.T) {
	rel := filepath.Join("areas", "area_alkharid", "dommik.rs2")
	rootA := filepath.Join(string(filepath.Separator), "tmp", "tmp.sMpZlKKc2w", "scripts")
	rootB := filepath.Join(string(filepath.Separator), "home", "runner", "a-much-longer-path", "scripts")

	blobA := writeWithRoots(t, []string{rootA}, filepath.Join(rootA, rel))
	blobB := writeWithRoots(t, []string{rootB}, filepath.Join(rootB, rel))

	if !bytes.Equal(blobA, blobB) {
		t.Errorf("blobs differ across source roots:\n A(%d) = %x\n B(%d) = %x",
			len(blobA), blobA, len(blobB), blobB)
	}
}

// TestBinaryWriter_SourceNameOutsideRoots pins the fallback: a script that
// lives under none of the roots keeps the name it was parsed with, rather
// than gaining a "../.." path that escapes the root.
func TestBinaryWriter_SourceNameOutsideRoots(t *testing.T) {
	root := filepath.Join(string(filepath.Separator), "tmp", "srcroot", "scripts")
	src := filepath.Join(string(filepath.Separator), "elsewhere", "stray.rs2")

	got := headerSourceName(t, writeWithRoots(t, []string{root}, src))

	if got != src {
		t.Errorf("sourceName = %q, want it left alone as %q", got, src)
	}
}

// TestBinaryWriter_SourceNameNoRoots pins that a writer with no configured
// roots — every sink that builds its own BinaryScriptWriter — is unchanged.
func TestBinaryWriter_SourceNameNoRoots(t *testing.T) {
	src := filepath.Join(string(filepath.Separator), "tmp", "srcroot", "scripts", "x.rs2")

	got := headerSourceName(t, writeWithRoots(t, nil, src))

	if got != src {
		t.Errorf("sourceName = %q, want %q", got, src)
	}
}
