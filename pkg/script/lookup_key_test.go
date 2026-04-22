package script

import "testing"

func TestLookupKeyForTypeExactFormula(t *testing.T) {
	got := LookupKeyForType(TriggerOpNpc1, 42)
	want := uint32(TriggerOpNpc1) | 0x200 | (uint32(42) << 10)
	if got != want {
		t.Errorf("LookupKeyForType: got 0x%x, want 0x%x", got, want)
	}
}

func TestLookupKeyForCategoryExactFormula(t *testing.T) {
	got := LookupKeyForCategory(TriggerOpNpc1, 7)
	want := uint32(TriggerOpNpc1) | 0x100 | (uint32(7) << 10)
	if got != want {
		t.Errorf("LookupKeyForCategory: got 0x%x, want 0x%x", got, want)
	}
}

func TestLookupKeyForGlobalIsJustTrigger(t *testing.T) {
	got := LookupKeyForGlobal(TriggerOpNpc1)
	want := uint32(TriggerOpNpc1)
	if got != want {
		t.Errorf("LookupKeyForGlobal: got 0x%x, want 0x%x", got, want)
	}
}

func TestLookupKeyBoundaryTypeIDZero(t *testing.T) {
	got := LookupKeyForType(TriggerOpNpc1, 0)
	want := uint32(TriggerOpNpc1) | 0x200
	if got != want {
		t.Errorf("typeID=0: got 0x%x, want 0x%x", got, want)
	}
}

func TestLookupKeyBoundaryCategoryIDZero(t *testing.T) {
	got := LookupKeyForCategory(TriggerOpNpc1, 0)
	want := uint32(TriggerOpNpc1) | 0x100
	if got != want {
		t.Errorf("categoryID=0: got 0x%x, want 0x%x", got, want)
	}
}

func TestLookupKeyDistinctnessAcrossSelectors(t *testing.T) {
	typeK := LookupKeyForType(TriggerOpNpc1, 7)
	catK := LookupKeyForCategory(TriggerOpNpc1, 7)
	globK := LookupKeyForGlobal(TriggerOpNpc1)
	if typeK == catK || typeK == globK || catK == globK {
		t.Errorf("selectors should produce distinct keys: type=0x%x cat=0x%x glob=0x%x", typeK, catK, globK)
	}
}
