package objtype

import "testing"

func TestInvTypeConfigs_ByName_HitViaConfigNames(t *testing.T) {
	c := &InvTypeConfigs{
		Configs: []*InvType{
			{ConfigType: ConfigType{ID: 0, DebugName: "first"}},
			{ConfigType: ConfigType{ID: 1, DebugName: "second"}},
		},
		ConfigNames: map[string]int{"first": 0, "second": 1},
	}
	got := c.ByName("second")
	if got == nil {
		t.Fatalf("ByName(second) = nil, want non-nil")
	}
	if got.ID != 1 || got.DebugName != "second" {
		t.Errorf("ByName(second) = {ID:%d, DebugName:%q}, want {ID:1, DebugName:\"second\"}", got.ID, got.DebugName)
	}
}

func TestInvTypeConfigs_ByName_MissReturnsNil(t *testing.T) {
	c := &InvTypeConfigs{
		Configs:     []*InvType{{ConfigType: ConfigType{ID: 0, DebugName: "only"}}},
		ConfigNames: map[string]int{"only": 0},
	}
	if got := c.ByName("absent"); got != nil {
		t.Errorf("ByName(absent) = %+v, want nil", got)
	}
}

func TestInvTypeConfigs_ByName_NilReceiverReturnsNil(t *testing.T) {
	var c *InvTypeConfigs
	if got := c.ByName("anything"); got != nil {
		t.Errorf("nil-receiver ByName = %+v, want nil", got)
	}
}

func TestInvTypeConfigs_ByName_StaleIndexFallsThroughToLinearScan(t *testing.T) {
	c := &InvTypeConfigs{
		Configs: []*InvType{
			{ConfigType: ConfigType{ID: 0, DebugName: "other"}},
			{ConfigType: ConfigType{ID: 1, DebugName: "fresh"}},
		},
		ConfigNames: map[string]int{"fresh": 5},
	}
	got := c.ByName("fresh")
	if got == nil {
		t.Fatalf("stale-index ByName(fresh) = nil; want fallback hit at id=1")
	}
	if got.ID != 1 {
		t.Errorf("stale-index ByName(fresh).ID = %d, want 1", got.ID)
	}
}

func TestInvTypeConfigs_ByName_LinearScanWhenConfigNamesEmpty(t *testing.T) {
	c := &InvTypeConfigs{
		Configs:     []*InvType{{ConfigType: ConfigType{ID: 0, DebugName: "scan_me"}}},
		ConfigNames: nil,
	}
	got := c.ByName("scan_me")
	if got == nil || got.ID != 0 {
		t.Errorf("ByName(scan_me) with nil ConfigNames = %+v, want non-nil id=0", got)
	}
}
