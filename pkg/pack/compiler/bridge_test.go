package compiler

import (
	"reflect"
	"testing"

	"github.com/zsrv/goscape/pkg/pack/compiler/runescript"
)

// TestToCompilerTypeInfo_Empty pins NAI-212 spec §4 edge case: empty
// source (Max=-1, all maps empty) → dst with all maps initialized,
// Max=-1.
func TestToCompilerTypeInfo_Empty(t *testing.T) {
	src := newTypeInfo()
	dst := ToCompilerTypeInfo(src)
	if dst == nil {
		t.Fatal("dst is nil, want non-nil")
	}
	if dst.Max != -1 {
		t.Errorf("Max=%d, want -1", dst.Max)
	}
	checkMap(t, "Map", dst.Map, map[string]string{})
	checkBoolMap(t, "Protect", dst.Protect, map[string]bool{})
	checkBoolMap(t, "Conditional", dst.Conditional, map[string]bool{})
}

// TestToCompilerTypeInfo_NumericIDs pins NAI-212 spec §4 rule 2: int
// keys are stringified into dst.Map.
func TestToCompilerTypeInfo_NumericIDs(t *testing.T) {
	src := newTypeInfo()
	src.Add(0, "alpha", true)
	src.Add(42, "beta", true)
	dst := ToCompilerTypeInfo(src)
	if dst.Max != 43 { // Add updates Max to id+1
		t.Errorf("Max=%d, want 43", dst.Max)
	}
	checkMap(t, "Map", dst.Map, map[string]string{"0": "alpha", "42": "beta"})
}

// TestToCompilerTypeInfo_NameMap pins NAI-212 spec §4 rule 3: NameMap
// entries (constantInfo shape) merge into dst.Map.
func TestToCompilerTypeInfo_NameMap(t *testing.T) {
	src := LoadRecords(map[string]string{"FOO": "100", "BAR": "hello"}, false)
	dst := ToCompilerTypeInfo(src)
	// LoadRecords stores values as-is (case-preserved); keys unchanged.
	checkMap(t, "Map", dst.Map, map[string]string{"FOO": "100", "BAR": "hello"})
}

// TestToCompilerTypeInfo_AuxiliaryMaps pins NAI-212 spec §4 table: all
// auxiliary fields (VarType/Protect/Require/Set/Corrupt/Conditional)
// carry over with stringified keys.
func TestToCompilerTypeInfo_AuxiliaryMaps(t *testing.T) {
	src := newTypeInfo()
	src.Add(5, "v5", true)
	src.VarType[5] = "int"
	src.Protect[5] = true
	src.Add(7, "v7", true)
	src.Require[7] = "active_player"
	src.Require2[7] = "active_npc"
	src.Set[7] = "x"
	src.Set2[7] = "y"
	src.Corrupt[7] = "c1"
	src.Corrupt2[7] = "c2"
	src.Conditional[7] = true
	dst := ToCompilerTypeInfo(src)

	checkMap(t, "Vartype", dst.Vartype, map[string]string{"5": "int"})
	checkBoolMap(t, "Protect", dst.Protect, map[string]bool{"5": true})
	checkMap(t, "Require", dst.Require, map[string]string{"7": "active_player"})
	checkMap(t, "Require2", dst.Require2, map[string]string{"7": "active_npc"})
	checkMap(t, "Set", dst.Set, map[string]string{"7": "x"})
	checkMap(t, "Set2", dst.Set2, map[string]string{"7": "y"})
	checkMap(t, "Corrupt", dst.Corrupt, map[string]string{"7": "c1"})
	checkMap(t, "Corrupt2", dst.Corrupt2, map[string]string{"7": "c2"})
	checkBoolMap(t, "Conditional", dst.Conditional, map[string]bool{"7": true})
}

// TestToCompilerTypeInfo_CollisionNumericWins pins NAI-212 spec §4
// rule 3 collision precedence: when an int key (stringified) collides
// with a NameMap key, the int-keyed value wins. Empirically impossible
// in TS-canonical call sites (loadRecords is only used for constants);
// defensive rule.
func TestToCompilerTypeInfo_CollisionNumericWins(t *testing.T) {
	src := newTypeInfo()
	src.Add(3, "from-int", true)
	src.NameMap["3"] = "from-str"
	dst := ToCompilerTypeInfo(src)
	if got := dst.Map["3"]; got != "from-int" {
		t.Errorf("Map[\"3\"]=%q, want \"from-int\" (numeric precedence)", got)
	}
}

// --- helpers ---

func checkMap(t *testing.T, name string, got, want map[string]string) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s = %#v, want %#v", name, got, want)
	}
}

func checkBoolMap(t *testing.T, name string, got, want map[string]bool) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s = %#v, want %#v", name, got, want)
	}
}

// runescript import sanity: the package must be reachable from the
// bridge tests without cycle. Confirmed pre-flight; this var keeps the
// import even if some test refactor temporarily removes the only use.
var _ = runescript.CompilerTypeInfo{}
