package script

import (
	"testing"

	"github.com/zsrv/goscape/pkg/coordgrid"
	"github.com/zsrv/goscape/pkg/objtype"
)

// fakeActiveLoc is a minimal ActiveLoc implementation for handler tests.
type fakeActiveLoc struct {
	id          int
	x, z, level int
	angle       int
	shape       int
}

func (f fakeActiveLoc) LocType() int              { return f.id }
func (f fakeActiveLoc) Coords() (x, z, level int) { return f.x, f.z, f.level }
func (f fakeActiveLoc) Angle() int                { return f.angle }
func (f fakeActiveLoc) Shape() int                { return f.shape }

// fakeConfigs implements the Configs interface with the LocType and
// ParamType paths wired for these tests; other methods return nil.
type fakeConfigs struct {
	locs   map[int]*objtype.LocType
	params map[int]*objtype.ParamType
}

func (f *fakeConfigs) ObjType(id int) *objtype.ObjType              { return nil }
func (f *fakeConfigs) NpcType(id int) *objtype.NpcType              { return nil }
func (f *fakeConfigs) LocType(id int) *objtype.LocType              { return f.locs[id] }
func (f *fakeConfigs) EnumType(id int) *objtype.EnumType            { return nil }
func (f *fakeConfigs) StructType(id int) *objtype.StructType        { return nil }
func (f *fakeConfigs) ParamType(id int) *objtype.ParamType          { return f.params[id] }
func (f *fakeConfigs) InvType(id int) *objtype.InvType              { return nil }
func (f *fakeConfigs) IdkType(id int) *objtype.IdkType              { return nil }
func (f *fakeConfigs) SpotAnimType(id int) *objtype.SpotanimType    { return nil }
func (f *fakeConfigs) DbTableType(id int) *objtype.DbTableType      { return nil }
func (f *fakeConfigs) DbRowType(id int) *objtype.DbRowType          { return nil }
func (f *fakeConfigs) DbRowsInTable(tableID int) []int              { return nil }
func (f *fakeConfigs) FindDbRowsInt(query int32, packed int) []int  { return nil }
func (f *fakeConfigs) FindDbRowsStr(query string, packed int) []int { return nil }

// newLocOpState builds a ScriptState with ActiveLoc bound, Configs wired,
// and a single int on the stack (the op index).
func newLocOpState(locID, op int, locType *objtype.LocType) *ScriptState {
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		ActiveLoc:   fakeActiveLoc{id: locID},
		Configs: &fakeConfigs{
			locs: map[int]*objtype.LocType{locID: locType},
		},
	}
	s.PushInt(op)
	return s
}

// TestHandleLocOpHappyPath verifies a valid op index returns the configured
// Op-slot string.
func TestHandleLocOpHappyPath(t *testing.T) {
	lt := &objtype.LocType{
		ConfigType: objtype.ConfigType{ID: 42},
		Op:         []string{"Chop", "Examine", "", "", ""},
	}
	s := newLocOpState(42, 1, lt)

	if err := handleLocOp(s); err != nil {
		t.Fatalf("handleLocOp: %v", err)
	}

	if s.SSP != 1 {
		t.Fatalf("SSP: got %d, want 1", s.SSP)
	}
	if got := s.StringStack[0]; got != "Chop" {
		t.Errorf("top of string stack: got %q, want \"Chop\"", got)
	}
}

// TestHandleLocOpRequiresActiveLoc verifies a nil ActiveLoc returns an
// error tagged "LOC_OP".
func TestHandleLocOpRequiresActiveLoc(t *testing.T) {
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushInt(1)

	err := handleLocOp(s)
	if err == nil {
		t.Fatal("handleLocOp: expected error, got nil")
	}
	if got := err.Error(); got != "LOC_OP: no active loc" {
		t.Errorf("error: got %q, want \"LOC_OP: no active loc\"", got)
	}
}

// TestHandleLocOpOutOfRangeLow verifies op=0 (below 1) pushes "".
func TestHandleLocOpOutOfRangeLow(t *testing.T) {
	lt := &objtype.LocType{
		ConfigType: objtype.ConfigType{ID: 42},
		Op:         []string{"Chop", "", "", "", ""},
	}
	s := newLocOpState(42, 0, lt)

	if err := handleLocOp(s); err != nil {
		t.Fatalf("handleLocOp: %v", err)
	}
	if got := s.StringStack[0]; got != "" {
		t.Errorf("got %q, want \"\" for op=0", got)
	}
}

// TestHandleLocOpOutOfRangeHigh verifies op=6 (above 5) pushes "".
func TestHandleLocOpOutOfRangeHigh(t *testing.T) {
	lt := &objtype.LocType{
		ConfigType: objtype.ConfigType{ID: 42},
		Op:         []string{"Chop", "", "", "", ""},
	}
	s := newLocOpState(42, 6, lt)

	if err := handleLocOp(s); err != nil {
		t.Fatalf("handleLocOp: %v", err)
	}
	if got := s.StringStack[0]; got != "" {
		t.Errorf("got %q, want \"\" for op=6", got)
	}
}

// TestHandleLocOpEmptySlot verifies an in-range op with an empty Op
// slot pushes "" (this is the common post-"hidden"-coercion case).
func TestHandleLocOpEmptySlot(t *testing.T) {
	lt := &objtype.LocType{
		ConfigType: objtype.ConfigType{ID: 42},
		Op:         []string{"Chop", "", "", "", ""},
	}
	s := newLocOpState(42, 2, lt) // Op[1] == ""

	if err := handleLocOp(s); err != nil {
		t.Fatalf("handleLocOp: %v", err)
	}
	if got := s.StringStack[0]; got != "" {
		t.Errorf("got %q, want \"\" for empty slot", got)
	}
}

// TestHandleLocOpLocTypeNotLoaded verifies a nil LocType lookup pushes "".
func TestHandleLocOpLocTypeNotLoaded(t *testing.T) {
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		ActiveLoc:   fakeActiveLoc{id: 999}, // id not in fakeConfigs
		Configs:     &fakeConfigs{locs: map[int]*objtype.LocType{}},
	}
	s.PushInt(1)

	if err := handleLocOp(s); err != nil {
		t.Fatalf("handleLocOp: %v", err)
	}
	if got := s.StringStack[0]; got != "" {
		t.Errorf("got %q, want \"\" for missing LocType", got)
	}
}

func TestHandleLocCoordHappyPath(t *testing.T) {
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		ActiveLoc:   fakeActiveLoc{id: 42, x: 3200, z: 3200, level: 0},
	}

	if err := handleLocCoord(s); err != nil {
		t.Fatalf("handleLocCoord: %v", err)
	}

	if s.ISP != 1 {
		t.Fatalf("ISP: got %d, want 1", s.ISP)
	}
	want := coordgrid.PackCoord(0, 3200, 3200)
	if got := s.IntStack[0]; got != want {
		t.Errorf("top of int stack: got %d, want %d", got, want)
	}
}

func TestHandleLocCoordRequiresActiveLoc(t *testing.T) {
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}

	err := handleLocCoord(s)
	if err == nil {
		t.Fatal("handleLocCoord: expected error, got nil")
	}
	if got := err.Error(); got != "LOC_COORD: no active loc" {
		t.Errorf("error: got %q, want \"LOC_COORD: no active loc\"", got)
	}
}

func TestHandleLocAngleHappyPath(t *testing.T) {
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		ActiveLoc:   fakeActiveLoc{id: 42, angle: 2},
	}

	if err := handleLocAngle(s); err != nil {
		t.Fatalf("handleLocAngle: %v", err)
	}

	if s.ISP != 1 {
		t.Fatalf("ISP: got %d, want 1", s.ISP)
	}
	if got := s.IntStack[0]; got != 2 {
		t.Errorf("top of int stack: got %d, want 2", got)
	}
}

func TestHandleLocAngleRequiresActiveLoc(t *testing.T) {
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}

	err := handleLocAngle(s)
	if err == nil {
		t.Fatal("handleLocAngle: expected error, got nil")
	}
	if got := err.Error(); got != "LOC_ANGLE: no active loc" {
		t.Errorf("error: got %q, want \"LOC_ANGLE: no active loc\"", got)
	}
}

// -- LOC_TYPE tests --

func TestHandleLocTypeHappyPath(t *testing.T) {
	lt := &objtype.LocType{
		ConfigType: objtype.ConfigType{ID: 42},
	}
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		ActiveLoc:   fakeActiveLoc{id: 42},
		Configs:     &fakeConfigs{locs: map[int]*objtype.LocType{42: lt}},
	}

	if err := handleLocType(s); err != nil {
		t.Fatalf("handleLocType: %v", err)
	}

	if s.ISP != 1 {
		t.Fatalf("ISP: got %d, want 1", s.ISP)
	}
	if got := s.IntStack[0]; got != 42 {
		t.Errorf("top of int stack: got %d, want 42", got)
	}
}

func TestHandleLocTypeRequiresActiveLoc(t *testing.T) {
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Configs:     &fakeConfigs{locs: map[int]*objtype.LocType{}},
	}

	err := handleLocType(s)
	if err == nil {
		t.Fatal("handleLocType: expected error, got nil")
	}
	if got := err.Error(); got != "LOC_TYPE: no active loc" {
		t.Errorf("error: got %q, want \"LOC_TYPE: no active loc\"", got)
	}
}

func TestHandleLocTypeUnknownID(t *testing.T) {
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		ActiveLoc:   fakeActiveLoc{id: 999},
		Configs:     &fakeConfigs{locs: map[int]*objtype.LocType{}},
	}

	err := handleLocType(s)
	if err == nil {
		t.Fatal("handleLocType: expected error, got nil")
	}
	if got := err.Error(); got != "LOC_TYPE: unknown loc id 999" {
		t.Errorf("error: got %q, want \"LOC_TYPE: unknown loc id 999\"", got)
	}
}

// -- LOC_NAME tests --

func TestHandleLocNameHappyPath(t *testing.T) {
	lt := &objtype.LocType{
		ConfigType: objtype.ConfigType{ID: 42},
		Name:       "door",
	}
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		ActiveLoc:   fakeActiveLoc{id: 42},
		Configs:     &fakeConfigs{locs: map[int]*objtype.LocType{42: lt}},
	}

	if err := handleLocName(s); err != nil {
		t.Fatalf("handleLocName: %v", err)
	}

	if s.SSP != 1 {
		t.Fatalf("SSP: got %d, want 1", s.SSP)
	}
	if got := s.StringStack[0]; got != "door" {
		t.Errorf("top of string stack: got %q, want \"door\"", got)
	}
}

func TestHandleLocNameNullFallback(t *testing.T) {
	lt := &objtype.LocType{
		ConfigType: objtype.ConfigType{ID: 42},
		// Name left empty — verifies "null" fallback per TS `name ?? 'null'`.
	}
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		ActiveLoc:   fakeActiveLoc{id: 42},
		Configs:     &fakeConfigs{locs: map[int]*objtype.LocType{42: lt}},
	}

	if err := handleLocName(s); err != nil {
		t.Fatalf("handleLocName: %v", err)
	}

	if got := s.StringStack[0]; got != "null" {
		t.Errorf("top of string stack: got %q, want \"null\"", got)
	}
}

func TestHandleLocNameRequiresActiveLoc(t *testing.T) {
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Configs:     &fakeConfigs{locs: map[int]*objtype.LocType{}},
	}

	err := handleLocName(s)
	if err == nil {
		t.Fatal("handleLocName: expected error, got nil")
	}
	if got := err.Error(); got != "LOC_NAME: no active loc" {
		t.Errorf("error: got %q, want \"LOC_NAME: no active loc\"", got)
	}
}

// -- LOC_SHAPE tests --

func TestHandleLocShapeHappyPath(t *testing.T) {
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		ActiveLoc:   fakeActiveLoc{id: 42, shape: 10},
	}

	if err := handleLocShape(s); err != nil {
		t.Fatalf("handleLocShape: %v", err)
	}

	if s.ISP != 1 {
		t.Fatalf("ISP: got %d, want 1", s.ISP)
	}
	if got := s.IntStack[0]; got != 10 {
		t.Errorf("top of int stack: got %d, want 10", got)
	}
}

func TestHandleLocShapeRequiresActiveLoc(t *testing.T) {
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}

	err := handleLocShape(s)
	if err == nil {
		t.Fatal("handleLocShape: expected error, got nil")
	}
	if got := err.Error(); got != "LOC_SHAPE: no active loc" {
		t.Errorf("error: got %q, want \"LOC_SHAPE: no active loc\"", got)
	}
}

// -- LOC_PARAM tests --

func TestHandleLocParamHappyPathInt(t *testing.T) {
	lt := &objtype.LocType{
		ConfigType: objtype.ConfigType{ID: 42},
		Params:     objtype.ParamMap{1: uint32(7)},
	}
	pInt := objtype.NewParamType(1)
	pInt.Type = objtype.ScriptVarTypeInt
	pInt.DefaultInt = 0

	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		ActiveLoc:   fakeActiveLoc{id: 42},
		Configs: &fakeConfigs{
			locs:   map[int]*objtype.LocType{42: lt},
			params: map[int]*objtype.ParamType{1: pInt},
		},
	}
	s.PushInt(1) // paramID

	if err := handleLocParam(s); err != nil {
		t.Fatalf("handleLocParam: %v", err)
	}

	if s.ISP != 1 {
		t.Fatalf("ISP: got %d, want 1", s.ISP)
	}
	if got := s.IntStack[0]; got != 7 {
		t.Errorf("top of int stack: got %d, want 7", got)
	}
}

func TestHandleLocParamHappyPathString(t *testing.T) {
	lt := &objtype.LocType{
		ConfigType: objtype.ConfigType{ID: 42},
		Params:     objtype.ParamMap{2: "hello"},
	}
	pStr := objtype.NewParamType(2)
	pStr.Type = objtype.ScriptVarTypeString
	pStr.DefaultString = ""

	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		ActiveLoc:   fakeActiveLoc{id: 42},
		Configs: &fakeConfigs{
			locs:   map[int]*objtype.LocType{42: lt},
			params: map[int]*objtype.ParamType{2: pStr},
		},
	}
	s.PushInt(2) // paramID

	if err := handleLocParam(s); err != nil {
		t.Fatalf("handleLocParam: %v", err)
	}

	if s.SSP != 1 {
		t.Fatalf("SSP: got %d, want 1", s.SSP)
	}
	if got := s.StringStack[0]; got != "hello" {
		t.Errorf("top of string stack: got %q, want \"hello\"", got)
	}
}

func TestHandleLocParamRequiresActiveLoc(t *testing.T) {
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Configs:     &fakeConfigs{locs: map[int]*objtype.LocType{}},
	}
	s.PushInt(1) // paramID — present so the no-active-loc gate fires before any pop

	err := handleLocParam(s)
	if err == nil {
		t.Fatal("handleLocParam: expected error, got nil")
	}
	if got := err.Error(); got != "LOC_PARAM: no active loc" {
		t.Errorf("error: got %q, want \"LOC_PARAM: no active loc\"", got)
	}
}
