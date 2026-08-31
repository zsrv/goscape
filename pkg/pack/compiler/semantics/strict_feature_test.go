// pkg/pack/compiler/semantics/strict_feature_test.go
package semantics

import (
	"reflect"
	"testing"
)

func TestStrictFeatureLevel_ZeroValue_AllEnabled(t *testing.T) {
	// Zero value = nothing disabled. ScriptRegistration treats every
	// `f.DisableX` as false.
	f := StrictFeatureLevel{}
	if f.DisableProcs || f.DisableEnums || f.DisableStructs || f.DisableDBTables || f.DisableBooleans {
		t.Fatalf("zero value has a Disable* set: %+v", f)
	}
}

// TestStrictFeatureLevel_HasNAI206Fields pins all TS-StrictFeatureLevel
// fields onto the Go struct. NAI-205 shipped 5; NAI-206 added 7. Two
// goscape-only fields gate RuneScriptTS HEAD-vs-v0.9.4 pointer-checker
// features (memory pointer_checker_runescriptts_version_divergence).
// Field naming follows NAI-205-D-STRICT-INVERTED-POLARITY: DisableX bool
// (zero == TS missing-key == "enabled"). TopLevelDefOnly is the lone
// non-inverted field because TS default is false (matching Go's bool zero).
func TestStrictFeatureLevel_HasNAI206Fields(t *testing.T) {
	want := []string{
		"DisableBooleans",         // TS booleans (default true)
		"DisableProcs",            // TS procs
		"DisableMacros",           // TS macros (NAI-206-add)
		"DisableEnums",            // TS enums
		"DisableStructs",          // TS structs
		"DisableDBTables",         // TS dbtables
		"DisableLogicalAnd",       // TS logicalAnd (NAI-206-add)
		"DisableCalc",             // TS calc (NAI-206-add)
		"DisableRelationalEquals", // TS relationalEquals (NAI-206-add)
		"DisableQueueTyped",       // TS queueTyped (NAI-206-add)
		"TopLevelDefOnly",         // TS topLevelDefOnly (default false; NOT inverted)
		"DisablePointerInversion", // TS pointerInversion (NAI-206-add)
		// goscape-only gates: HEAD-features post-v0.9.4 (Engine-TS bundled).
		"DisableOverlayInterfaceProtection", // RuneScriptTS fe0ae0a
		"DisableStaticLabelArgPropagation",  // RuneScriptTS 50c9bb1
	}
	sf := reflect.TypeFor[StrictFeatureLevel]()
	for _, name := range want {
		if _, ok := sf.FieldByName(name); !ok {
			t.Errorf("StrictFeatureLevel missing field %s", name)
		}
	}
	if got, wantCount := sf.NumField(), len(want); got != wantCount {
		t.Errorf("StrictFeatureLevel has %d fields, want %d (no extras/missing)", got, wantCount)
	}
}
