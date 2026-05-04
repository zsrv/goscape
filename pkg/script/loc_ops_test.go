package script

import "testing"

// fakeLocOps records all LocOps method calls for handler-side assertions.
type fakeLocOps struct {
	changeCalls []changeLocCall
	addCalls    []addLocCall
	removeCalls []removeLocCall
	animCalls   []animLocCall
	atCoord     []ActiveLoc
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
	if len(f.atCoord) > 0 {
		return f.atCoord[0], nil
	}
	return nil, nil
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

func TestScriptStateAcceptsLocOps(t *testing.T) {
	s := &ScriptState{}
	s.LocOps = &fakeLocOps{}
	if s.LocOps == nil {
		t.Error("LocOps field unsettable")
	}
}
