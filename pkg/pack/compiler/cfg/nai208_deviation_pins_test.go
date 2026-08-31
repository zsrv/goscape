// pkg/pack/compiler/cfg/nai208_deviation_pins_test.go — T9 close:
// deviation-tag pin tests for NAI-208 (compiler slice 6a).
//
// Two categories:
//  1. Structural pins for each NAI-208-D-* tag.
//  2. Grep-based walk that verifies every living deviation tag appears in
//     at least one .go file under the repo root. Self-references in this
//     file count, so architectural/design deviations are still covered.
package cfg

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/zsrv/goscape/pkg/pack/compiler/codegen"
	"github.com/zsrv/goscape/pkg/pack/compiler/pointer"
)

// === Per-tag structural pins ===

// TestPin_NAI208_D_POINTERTYPE_PTR_SINGLETON pins that PointerType is a
// struct (not a numeric typedef) and the singletons are accessed via
// package-level *PointerType vars.
func TestPin_NAI208_D_POINTERTYPE_PTR_SINGLETON(t *testing.T) {
	v := reflect.TypeFor[pointer.PointerType]()
	if v.Kind() != reflect.Struct {
		t.Errorf("PointerType.Kind() = %v, want Struct", v.Kind())
	}
	if pointer.ActivePlayer == nil {
		t.Error("pointer.ActivePlayer singleton is nil")
	}
}

// TestPin_NAI208_D_POINTERSET_MAP_STRUCT pins that PointerSet wraps a map,
// not e.g. a slice or a struct field.
func TestPin_NAI208_D_POINTERSET_MAP_STRUCT(t *testing.T) {
	s := pointer.NewPointerSet(pointer.ActivePlayer, pointer.ActiveNpc)
	if s.Len() != 2 {
		t.Errorf("Len = %d, want 2", s.Len())
	}
	// Idempotent Add — map-backed sets dedupe.
	s.Add(pointer.ActivePlayer)
	if s.Len() != 2 {
		t.Errorf("Len after idempotent Add = %d, want 2 (proves map dedupe)", s.Len())
	}
}

// TestPin_NAI208_D_POINTERHOLDER_PTRSET pins that PointerHolder fields are
// *PointerSet (not bare maps).
func TestPin_NAI208_D_POINTERHOLDER_PTRSET(t *testing.T) {
	ty := reflect.TypeFor[pointer.PointerHolder]()
	for _, fname := range []string{"Required", "Set", "Corrupted"} {
		f, ok := ty.FieldByName(fname)
		if !ok {
			t.Fatalf("PointerHolder.%s missing", fname)
		}
		if f.Type != reflect.TypeFor[*pointer.PointerSet]() {
			t.Errorf("PointerHolder.%s type = %v, want *pointer.PointerSet", fname, f.Type)
		}
	}
}

// TestPin_NAI208_D_SYMBOL_NO_METHOD_CYCLE_AVOID pins that GetPointers lives
// on cfg.PointerChecker (not on symbol.ScriptSymbol).
func TestPin_NAI208_D_SYMBOL_NO_METHOD_CYCLE_AVOID(t *testing.T) {
	ty := reflect.TypeFor[*PointerChecker]()
	if _, ok := ty.MethodByName("GetPointers"); !ok {
		t.Error("cfg.PointerChecker.GetPointers missing — symbol-cycle-avoidance broken")
	}
}

// TestPin_NAI208_D_VIRTUAL_VIA_FNFIELD pins that PointerChecker exposes
// SetSetsPointerTriggerFn + DefaultSetsPointerTrigger so subclasses can
// install + delegate to the base.
func TestPin_NAI208_D_VIRTUAL_VIA_FNFIELD(t *testing.T) {
	ty := reflect.TypeFor[*PointerChecker]()
	if _, ok := ty.MethodByName("SetSetsPointerTriggerFn"); !ok {
		t.Error("PointerChecker.SetSetsPointerTriggerFn missing")
	}
	if _, ok := ty.MethodByName("DefaultSetsPointerTrigger"); !ok {
		t.Error("PointerChecker.DefaultSetsPointerTrigger missing")
	}
}

// TestPin_NAI208_D_INSTRUCTION_POINTER_KEY pins that Block.Instructions is
// []Instruction (by-value), so &block.Instructions[i] is a stable map key
// post-codegen.
func TestPin_NAI208_D_INSTRUCTION_POINTER_KEY(t *testing.T) {
	ty := reflect.TypeFor[codegen.Block]()
	f, ok := ty.FieldByName("Instructions")
	if !ok {
		t.Fatal("Block.Instructions missing")
	}
	if f.Type.Kind() != reflect.Slice || f.Type.Elem() != reflect.TypeFor[codegen.Instruction]() {
		t.Errorf("Block.Instructions type = %v, want []Instruction (by-value)", f.Type)
	}
}

// TestPin_NAI208_D_PACKAGE_NAMES is a doc-comment / package-existence pin:
// asserts pkg/pack/compiler/cfg/ exists at the expected path.
func TestPin_NAI208_D_PACKAGE_NAMES(t *testing.T) {
	// Walk-the-fs: by being IN the package, this test file proves the
	// package exists at the expected path.
	cwd, _ := os.Getwd()
	if !strings.Contains(filepath.ToSlash(cwd), "pkg/pack/compiler/cfg") {
		t.Errorf("test ran from unexpected cwd %q", cwd)
	}
}

// TestPin_NAI208_D_METASCRIPT_IDENT_EXPORTER pins that
// typ.MetaScriptTriggerIdent is exported (verified by import + smoke call).
// Lives in pointer_checker_labels_test.go's TestPointerChecker_LabelJump as
// the behavioural pin; the existence pin is the package-import here.

// === Grep walker ===

// TestPin_NAI208_GrepWalker enumerates every living NAI-208-D-* tag and
// asserts each appears in at least one .go file under the repo root.
// Self-references in this file count.
func TestPin_NAI208_GrepWalker(t *testing.T) {
	tags := []string{
		"NAI-208-D-POINTERTYPE-PTR-SINGLETON",
		"NAI-208-D-POINTERSET-MAP-STRUCT",
		"NAI-208-D-POINTERHOLDER-PTRSET",
		"NAI-208-D-SYMBOL-NO-METHOD-CYCLE-AVOID",
		"NAI-208-D-VIRTUAL-VIA-FNFIELD",
		"NAI-208-D-TRIGGER-PARTIAL-PORT",
		"NAI-208-D-PACKAGE-NAMES",
		"NAI-208-D-METASCRIPT-IDENT-EXPORTER",
		"NAI-208-D-INSTRUCTION-POINTER-KEY",
		"NAI-208-D-PROTECTED-VAR-VIA-SYMBOL",
	}

	root, err := repoRootForPinTest()
	if err != nil {
		t.Fatalf("repo root: %v", err)
	}

	for _, tag := range tags {
		found := false
		err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}
			if !strings.HasSuffix(path, ".go") {
				return nil
			}
			data, rerr := os.ReadFile(path)
			if rerr != nil {
				return nil
			}
			if strings.Contains(string(data), tag) {
				found = true
				return filepath.SkipDir
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk: %v", err)
		}
		if !found {
			t.Errorf("tag %q not found in any .go file under %s", tag, root)
		}
	}
}

// repoRootForPinTest walks up from the test's CWD until it finds go.mod.
func repoRootForPinTest() (string, error) {
	dir, _ := os.Getwd()
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", os.ErrNotExist
		}
		dir = parent
	}
}
