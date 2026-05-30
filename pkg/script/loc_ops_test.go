package script

import "testing"

// fakeLocOps records all LocOps method calls for handler-side assertions.
type fakeLocOps struct {
	changeCalls      []changeLocCall
	addCalls         []addLocCall
	removeCalls      []removeLocCall
	animCalls        []animLocCall
	atCoord          []ActiveLoc
	inZone           []ActiveLoc // raw — returned from AllLocsInZone (MAP_LOCADDUNSAFE source)
	inZoneSafe       []ActiveLoc // filtered+reversed by test setup — returned from AllLocsSafe (LocIterator source)
	addReturn        ActiveLoc   // returned from AddLoc
	getLocCalls      []getLocCall
	getLocReturn     ActiveLoc // returned from GetLoc; default nil = miss
	allLocsSafeCalls []allLocsSafeCall
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

type getLocCall struct {
	level, x, z, typ int
}

type allLocsSafeCall struct {
	level, x, z int
	reverse     bool
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

// AllLocsSafe returns f.inZoneSafe (independent of f.inZone — see comment
// on the struct field) for the requested reverse flag. The fake stores
// two slices so tests can independently assert AllLocsInZone (unsafe
// MAP_LOCADDUNSAFE consumer) and AllLocsSafe (filtered+reversed
// LocIterator consumer) behavior.
func (f *fakeLocOps) AllLocsSafe(level, x, z int, reverse bool) []ActiveLoc {
	f.allLocsSafeCalls = append(f.allLocsSafeCalls, allLocsSafeCall{level, x, z, reverse})
	return f.inZoneSafe
}

func (f *fakeLocOps) GetLoc(level, x, z, typ int) ActiveLoc {
	f.getLocCalls = append(f.getLocCalls, getLocCall{level, x, z, typ})
	return f.getLocReturn
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
