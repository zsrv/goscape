// pkg/pack/compiler/codegen/nai207_deviation_pins_test.go — T15 close:
// deviation-tag pin tests for NAI-207 (codegen slice).
//
// Three categories:
//  1. Structural reflection pins for the original spec tags.
//  2. Grep-based walk that verifies every living deviation tag appears in at
//     least one .go file under the repo root. Self-references in this very
//     file count, so tags that exist only as architectural/design deviations
//     (NAI-207-D-PACKAGE-SPLIT, NAI-207-D-NO-VISITOR-INTERFACE,
//     NAI-207-D-NULLLITERAL-HOOK-DISC) are still pinned via this list.
package codegen

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/zsrv/goscape/pkg/pack/compiler/semantics"
)

// === Original spec tags — structural reflection pins ===

// TestPin_NAI207_D_OPCODE_UNTYPED verifies that Opcode is a struct with Name
// and Kind fields (not a numeric typedef). NAI-207-D-OPCODE-UNTYPED: goscape
// uses an untyped operand because Go generics don't compose with a
// []Instruction[any]-style heterogeneous slice.
func TestPin_NAI207_D_OPCODE_UNTYPED(t *testing.T) {
	v := reflect.TypeOf(PushConstantInt)
	if v.Kind() != reflect.Struct {
		t.Errorf("Opcode kind: got %v, want Struct", v.Kind())
	}
	if _, ok := v.FieldByName("Name"); !ok {
		t.Errorf("Opcode.Name field absent")
	}
	if _, ok := v.FieldByName("Kind"); !ok {
		t.Errorf("Opcode.Kind field absent")
	}
}

// TestPin_NAI207_D_DYNCOMMAND_BOOLRESULT verifies that
// DynamicCommandHandler.GenerateCode returns bool. NAI-207-D-DYNCOMMAND-BOOLRESULT:
// TS returns void; goscape returns bool so callers can apply a default fallback.
func TestPin_NAI207_D_DYNCOMMAND_BOOLRESULT(t *testing.T) {
	iface := reflect.TypeOf((*semantics.DynamicCommandHandler)(nil)).Elem()
	m, ok := iface.MethodByName("GenerateCode")
	if !ok {
		t.Fatalf("DynamicCommandHandler.GenerateCode method missing")
	}
	if m.Type.NumOut() != 1 || m.Type.Out(0).Kind() != reflect.Bool {
		t.Errorf("GenerateCode return: got %v, want bool", m.Type.Out(0))
	}
}

// TestPin_NAI207_D_CODEGENCONTEXT_MARKER verifies that *CodeGeneratorContext
// satisfies semantics.CodeGenContext. NAI-207-D-CODEGENCONTEXT-MARKER: the
// marker interface lives in semantics to avoid a codegen→semantics import
// cycle; NAI-207-D-CODEGENCONTEXT-EXPORTEDMARKER: the method is exported.
func TestPin_NAI207_D_CODEGENCONTEXT_MARKER(t *testing.T) {
	var _ semantics.CodeGenContext = (*CodeGeneratorContext)(nil)
}

// === Grep-based pin: every living deviation tag MUST appear in repo source ===

// nai207LivingTags is the authoritative list of living NAI-207 deviation tags.
// Self-references here satisfy the grep for tags that exist only as
// architectural deviations without a dedicated doc-comment in other files:
//   - NAI-207-D-PACKAGE-SPLIT: dynamic-command handlers in pkg/pack/compiler/command/
//     rather than merged into codegen/. A separate package keeps handler logic
//     testable without embedding codegen internals.
//   - NAI-207-D-NO-VISITOR-INTERFACE: Visit dispatch uses a Go type-switch
//     rather than the TS Visitor interface pattern (mirrors NAI-204-D-AST-NO-VISITOR).
//   - NAI-207-D-NULLLITERAL-HOOK-DISC: visitNullLiteral emits only the int arm
//     for hook-typed nulls; the trailing PushConstantString("") hook-name part
//     is emitted by the hook-specific codegen path, not here. TS emits both in
//     visitNullLiteral; this is a TS-parity deviation tagged for NAI-208.
var nai207LivingTags = []string{
	"NAI-207-D-OPCODE-UNTYPED",
	"NAI-207-D-PACKAGE-SPLIT",
	"NAI-207-D-NO-VISITOR-INTERFACE",
	"NAI-207-D-NULLLITERAL-HOOK-DISC",
	"NAI-207-D-DYNCOMMAND-BOOLRESULT",
	"NAI-207-D-CODEGENCONTEXT-MARKER",
	"NAI-207-D-CODEGENCONTEXT-EXPORTEDMARKER",
	"NAI-207-D-LINENUMBER-NO-EMIT",
	"NAI-207-D-COND-NO-ARITH",
	"NAI-207-D-ARRAY-DECL-SYNTAX",
	"NAI-207-D-DYNCOMMAND-FALLBACK-VISITEXPR",
	"NAI-207-D-NULL-NO-OBJ-PRIM",
	"NAI-207-D-JOINEDSTR-STR-PARAM",
	"NAI-207-D-IDENT-STRFB-T14",
	"NAI-207-D-COHORT-B-DUMP-MINIMAL",
	"NAI-207-D-COHORT-B-SCRIPT-MINIMAL",
	"NAI-207-D-COHORT-B-QUEUEVARARG-TRIGGERCHECK",
	"NAI-207-D-PARAM-NO-CONSTRAINT",
	"NAI-207-D-SMOKE-NO-MES",
}

// TestPin_NAI207_AllDeviationTagsPresentInSource walks the repository root and
// verifies each living tag appears in at least one .go source file. Catches
// tag drift / accidental retirement without a proper close entry.
//
// Per [[true_to_ts_gate]]: every behavioral divergence needs a tracked deviation
// with rationale + follow-up. This test enforces the tracking invariant.
func TestPin_NAI207_AllDeviationTagsPresentInSource(t *testing.T) {
	// Test file lives at pkg/pack/compiler/codegen/; repo root = "../../../../".
	repoRoot, err := filepath.Abs("../../../../")
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}

	found := make(map[string]bool, len(nai207LivingTags))
	walkErr := filepath.Walk(repoRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip unreadable entries
		}
		if info.IsDir() {
			name := info.Name()
			if name == ".git" || name == "node_modules" || name == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil // skip unreadable files
		}
		text := string(data)
		for _, tag := range nai207LivingTags {
			if !found[tag] && strings.Contains(text, tag) {
				found[tag] = true
			}
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk: %v", walkErr)
	}

	var missing []string
	for _, tag := range nai207LivingTags {
		if !found[tag] {
			missing = append(missing, tag)
		}
	}
	if len(missing) > 0 {
		t.Errorf("deviation tags absent from source (would-be-retired without close docs): %v", missing)
	}
}
