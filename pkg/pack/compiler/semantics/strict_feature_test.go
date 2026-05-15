// pkg/pack/compiler/semantics/strict_feature_test.go
package semantics

import "testing"

func TestStrictFeatureLevel_ZeroValue_AllEnabled(t *testing.T) {
	// Zero value = nothing disabled. ScriptRegistration treats every
	// `f.DisableX` as false.
	f := StrictFeatureLevel{}
	if f.DisableProcs || f.DisableEnums || f.DisableStructs || f.DisableDBTables || f.DisableBooleans {
		t.Fatalf("zero value has a Disable* set: %+v", f)
	}
}
