package app

import (
	"testing"
)

// TestDisabledModulesYieldNoService pins arch-29.8: a disabled module
// contributes no service — no vacuous "Running" state (previously each
// disabled leaf module returned services.NewIdleService(nil, nil)), and no
// vacuously-satisfied AwaitRunning for dependents. newAppForTest disables
// all four leaf modules by default, so --target=all (SingleBinary) should
// resolve to zero services once every disabled initFn returns (nil, nil).
func TestDisabledModulesYieldNoService(t *testing.T) {
	a, _ := newAppForTest(t, SingleBinary)
	svcs, err := a.ModuleManager.InitModuleServices(SingleBinary)
	if err != nil {
		t.Fatalf("init with everything disabled: %v", err)
	}
	if len(svcs) != 0 {
		names := make([]string, 0, len(svcs))
		for name := range svcs {
			names = append(names, name)
		}
		t.Fatalf("want 0 services for all-disabled target, got %d: %v", len(svcs), names)
	}
}
