package script

import "testing"

// fakeLocOps records all LocOps method calls for handler-side assertions.
type fakeLocOps struct {
	changeCalls []changeLocCall
	addCalls    []addLocCall
	removeCalls []removeLocCall
	animCalls   []animLocCall
	atCoord     []ActiveLoc
	inZone      []ActiveLoc
	addReturn   ActiveLoc // returned from AddLoc
}

type changeLocCall struct {
	loc                    ActiveLoc
	typ, shape, angle, dur int
}

type addLocCall struct {
	level, x, z, typ, shape, angle, dur int
}

type removeLocCall struct {
	loc ActiveLoc
	dur int
}

type animLocCall struct {
	loc ActiveLoc
	seq int
}

func (f *fakeLocOps) ChangeLoc(loc ActiveLoc, typ, shape, angle, dur int) error {
	f.changeCalls = append(f.changeCalls, changeLocCall{loc, typ, shape, angle, dur})
	return nil
}

func (f *fakeLocOps) AddLoc(level, x, z, typ, shape, angle, dur int) (ActiveLoc, error) {
	f.addCalls = append(f.addCalls, addLocCall{level, x, z, typ, shape, angle, dur})
	return f.addReturn, nil
}

func (f *fakeLocOps) RemoveLoc(loc ActiveLoc, dur int) error {
	f.removeCalls = append(f.removeCalls, removeLocCall{loc, dur})
	return nil
}

func (f *fakeLocOps) AnimLoc(loc ActiveLoc, seq int) error {
	f.animCalls = append(f.animCalls, animLocCall{loc, seq})
	return nil
}

func (f *fakeLocOps) LocsAtCoord(level, x, z int) []ActiveLoc {
	return f.atCoord
}

func (f *fakeLocOps) AllLocsInZone(level, x, z int) []ActiveLoc {
	return f.inZone
}

func TestScriptStateAcceptsLocOps(t *testing.T) {
	s := &ScriptState{}
	s.LocOps = &fakeLocOps{}
	if s.LocOps == nil {
		t.Error("LocOps field unsettable")
	}
}

// TestLocOpsInterfaceHasAllLocsInZone confirms LocOps surfaces the
// zone-wide loc enumeration MAP_LOCADDUNSAFE needs (distinct from
// LocsAtCoord which filters by exact tile). NAI-114.
func TestLocOpsInterfaceHasAllLocsInZone(t *testing.T) {
	var ops LocOps = &fakeLocOps{}
	got := ops.AllLocsInZone(0, 100, 200)
	if got != nil {
		t.Errorf("fakeLocOps.AllLocsInZone(empty): got %v, want nil", got)
	}
}
