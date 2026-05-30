package script

import (
	"strings"
	"testing"

	"github.com/zsrv/goscape/pkg/coordgrid"
	"github.com/zsrv/goscape/pkg/fonttype"
	"github.com/zsrv/goscape/pkg/objtype"
)

// fakeActiveLoc is a minimal ActiveLoc implementation for handler tests.
type fakeActiveLoc struct {
	id          int
	x, z, level int
	angle       int
	shape       int
	layer       int
	active      bool
}

func (f fakeActiveLoc) LocType() int              { return f.id }
func (f fakeActiveLoc) Coords() (x, z, level int) { return f.x, f.z, f.level }
func (f fakeActiveLoc) Angle() int                { return f.angle }
func (f fakeActiveLoc) Shape() int                { return f.shape }
func (f fakeActiveLoc) Layer() int                { return f.layer }
func (f fakeActiveLoc) Active() bool              { return f.active }

// fakeConfigs implements the Configs interface with the LocType and
// ParamType paths wired for these tests; other methods return nil.
type fakeConfigs struct {
	locs   map[int]*objtype.LocType
	params map[int]*objtype.ParamType
	seqs   map[int]*objtype.SeqType
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
func (f *fakeConfigs) SeqType(id int) *objtype.SeqType              { return f.seqs[id] }
func (f *fakeConfigs) HuntType(id int) *objtype.HuntType            { return nil }
func (f *fakeConfigs) DbTableType(id int) *objtype.DbTableType      { return nil }
func (f *fakeConfigs) DbRowType(id int) *objtype.DbRowType          { return nil }
func (f *fakeConfigs) DbRowsInTable(tableID int) []int              { return nil }
func (f *fakeConfigs) FindDbRowsInt(query int32, packed int) []int  { return nil }
func (f *fakeConfigs) FindDbRowsStr(query string, packed int) []int { return nil }
func (f *fakeConfigs) VarpType(id int) (objtype.ScriptVarType, bool) {
	return objtype.ScriptVarTypeInt, false
}
func (f *fakeConfigs) VarnType(id int) objtype.ScriptVarType   { return objtype.ScriptVarTypeInt }
func (f *fakeConfigs) ObjByName(name string) *objtype.ObjType  { return nil }
func (f *fakeConfigs) MesanimType(id int) *objtype.MesanimType { return nil }
func (f *fakeConfigs) MesanimByName(name string) int           { return -1 }
func (f *fakeConfigs) FontType(id int) *fonttype.FontType      { return nil }

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

// -- checkLocType validator unit tests --

func TestCheckLocType(t *testing.T) {
	tests := []struct {
		name      string
		id        int
		setup     func() *mockConfigs
		wantErr   bool
		wantSubst string
	}{
		{
			name:    "valid id",
			id:      0,
			setup:   func() *mockConfigs { return &mockConfigs{locs: map[int]*objtype.LocType{0: {}}} },
			wantErr: false,
		},
		{
			name:      "unknown id",
			id:        100,
			setup:     func() *mockConfigs { return &mockConfigs{locs: map[int]*objtype.LocType{}} },
			wantErr:   true,
			wantSubst: "OP: no LocType with value (100) found",
		},
		{
			name:      "negative id",
			id:        -1,
			setup:     func() *mockConfigs { return &mockConfigs{locs: map[int]*objtype.LocType{}} },
			wantErr:   true,
			wantSubst: "OP: no LocType with value (-1) found",
		},
		{
			name:      "nil Configs",
			id:        0,
			setup:     func() *mockConfigs { return nil },
			wantErr:   true,
			wantSubst: "OP: no LocType with value (0) found",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := &ScriptState{}
			if cfg := tc.setup(); cfg != nil {
				s.Configs = cfg
			}
			err := checkLocType(s, tc.id, "OP")
			if tc.wantErr {
				if err == nil {
					t.Fatalf("checkLocType(%d): want error, got nil", tc.id)
				}
				if !strings.Contains(err.Error(), tc.wantSubst) {
					t.Errorf("error message: got %q, want contains %q", err.Error(), tc.wantSubst)
				}
			} else {
				if err != nil {
					t.Fatalf("checkLocType(%d): want nil, got %v", tc.id, err)
				}
			}
		})
	}
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
// slot pushes "" (an unset slot; "hidden" slots are now stored verbatim
// and would push "hidden", matching TS truthy reads).
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

// TestLocReadOpsOperandAware_ReadsSecondary pins NAI-119-D closure: LOC_* read
// handlers resolve .loc/.loc2 operand-aware via s.activeLoc() (TS
// ScriptState.activeLoc, ScriptState.ts:269-279). With IntOperand==1 (.loc2),
// LOC_COORD must read OtherActiveLoc, not the primary ActiveLoc.
func TestLocReadOpsOperandAware_ReadsSecondary(t *testing.T) {
	s := &ScriptState{
		IntStack:       make([]int, StackCapacity),
		StringStack:    make([]string, StackCapacity),
		Script:         &ScriptFile{IntOperands: []int32{1}},             // .loc2
		ActiveLoc:      fakeActiveLoc{id: 1, x: 1000, z: 1000, level: 0}, // primary — must NOT be read
		OtherActiveLoc: fakeActiveLoc{id: 2, x: 3200, z: 3300, level: 1}, // secondary
	}
	if err := handleLocCoord(s); err != nil {
		t.Fatalf("handleLocCoord (.loc2): %v", err)
	}
	want := coordgrid.PackCoord(1, 3200, 3300) // the SECONDARY coord
	if got := s.IntStack[0]; got != want {
		t.Errorf(".loc2 must read OtherActiveLoc: got %d, want %d", got, want)
	}
}

// TestLocReadOpsOperandAware_SecondaryOnly pins that requireActiveLoc resolves
// the operand: with .loc2 and only OtherActiveLoc set (primary nil) the read
// succeeds instead of erroring "no active loc".
func TestLocReadOpsOperandAware_SecondaryOnly(t *testing.T) {
	s := &ScriptState{
		IntStack:       make([]int, StackCapacity),
		StringStack:    make([]string, StackCapacity),
		Script:         &ScriptFile{IntOperands: []int32{1}},
		OtherActiveLoc: fakeActiveLoc{id: 2, x: 3200, z: 3300, level: 1},
	}
	if err := handleLocCoord(s); err != nil {
		t.Fatalf("handleLocCoord (.loc2, secondary-only): %v", err)
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

// -- LOC_CATEGORY tests --

func TestHandleLocCategoryHappyPath(t *testing.T) {
	lt := &objtype.LocType{
		ConfigType: objtype.ConfigType{ID: 42},
		Category:   17,
	}
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		ActiveLoc:   fakeActiveLoc{id: 42},
		Configs:     &fakeConfigs{locs: map[int]*objtype.LocType{42: lt}},
	}

	if err := handleLocCategory(s); err != nil {
		t.Fatalf("handleLocCategory: %v", err)
	}
	if s.ISP != 1 {
		t.Fatalf("ISP: got %d, want 1", s.ISP)
	}
	if got := s.IntStack[0]; got != 17 {
		t.Errorf("top of int stack: got %d, want 17", got)
	}
}

// TestHandleLocCategoryUnsetDefault pins the TS-faithful -1 default for
// LocType.Category when the config-file omits the `category=` line.
func TestHandleLocCategoryUnsetDefault(t *testing.T) {
	lt := &objtype.LocType{
		ConfigType: objtype.ConfigType{ID: 42},
		Category:   -1,
	}
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		ActiveLoc:   fakeActiveLoc{id: 42},
		Configs:     &fakeConfigs{locs: map[int]*objtype.LocType{42: lt}},
	}

	if err := handleLocCategory(s); err != nil {
		t.Fatalf("handleLocCategory: %v", err)
	}
	if got := s.IntStack[0]; got != -1 {
		t.Errorf("top of int stack: got %d, want -1", got)
	}
}

func TestHandleLocCategoryRequiresActiveLoc(t *testing.T) {
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Configs:     &fakeConfigs{locs: map[int]*objtype.LocType{}},
	}

	err := handleLocCategory(s)
	if err == nil {
		t.Fatal("handleLocCategory: expected error, got nil")
	}
	if got := err.Error(); got != "LOC_CATEGORY: no active loc" {
		t.Errorf("error: got %q, want \"LOC_CATEGORY: no active loc\"", got)
	}
}

func TestHandleLocCategoryUnknownID(t *testing.T) {
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		ActiveLoc:   fakeActiveLoc{id: 999},
		Configs:     &fakeConfigs{locs: map[int]*objtype.LocType{}},
	}

	err := handleLocCategory(s)
	if err == nil {
		t.Fatal("handleLocCategory: expected error, got nil")
	}
	if got := err.Error(); got != "LOC_CATEGORY: no LocType with value (999) found" {
		t.Errorf("error: got %q, want \"LOC_CATEGORY: no LocType with value (999) found\"", got)
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
	if got := err.Error(); got != "LOC_TYPE: no LocType with value (999) found" {
		t.Errorf("error: got %q, want \"LOC_TYPE: no LocType with value (999) found\"", got)
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

// -- LOC_CHANGE tests --

// newLocChangeState builds a ScriptState ready for handleLocChange tests.
// Stack is empty; caller pushes [id, duration] in that order before calling.
func newLocChangeState(activeLoc ActiveLoc, locTypes map[int]*objtype.LocType) *ScriptState {
	return &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		ActiveLoc:   activeLoc,
		Configs:     &fakeConfigs{locs: locTypes},
		LocOps:      &fakeLocOps{},
	}
}

func TestLocChangeCallsLocOpsWithPoppedArgs(t *testing.T) {
	loc := fakeActiveLoc{id: 100, shape: 0, angle: 0}
	lt := &objtype.LocType{ConfigType: objtype.ConfigType{ID: 100}}
	lt200 := &objtype.LocType{ConfigType: objtype.ConfigType{ID: 200}}
	s := newLocChangeState(loc, map[int]*objtype.LocType{100: lt, 200: lt200})

	// stack: [..., id=200, duration=3]
	s.PushInt(200)
	s.PushInt(3)

	if err := handleLocChange(s); err != nil {
		t.Fatalf("handleLocChange: unexpected error %v", err)
	}
	ops := s.LocOps.(*fakeLocOps)
	if len(ops.changeCalls) != 1 {
		t.Fatalf("ChangeLoc calls: got %d, want 1", len(ops.changeCalls))
	}
	c := ops.changeCalls[0]
	if c.typ != 200 || c.dur != 3 {
		t.Errorf("ChangeLoc args: got typ=%d dur=%d, want 200/3", c.typ, c.dur)
	}
	if c.shape != 0 || c.angle != 0 {
		t.Errorf("ChangeLoc preserves activeLoc shape/angle: got shape=%d angle=%d", c.shape, c.angle)
	}
}

func TestLocChangeRequiresActiveLoc(t *testing.T) {
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushInt(200)
	s.PushInt(3)
	if err := handleLocChange(s); err == nil {
		t.Error("handleLocChange without ActiveLoc must return error")
	}
}

func TestLocChangeRejectsZeroOrNegativeDuration(t *testing.T) {
	for _, dur := range []int{0, -1, -100} {
		loc := fakeActiveLoc{id: 100}
		lt := &objtype.LocType{ConfigType: objtype.ConfigType{ID: 100}}
		s := newLocChangeState(loc, map[int]*objtype.LocType{100: lt})
		s.PushInt(100)
		s.PushInt(dur)
		if err := handleLocChange(s); err == nil {
			t.Errorf("handleLocChange dur=%d must reject", dur)
		}
	}
}

func TestLocChangeRejectsUnknownType(t *testing.T) {
	loc := fakeActiveLoc{id: 100}
	lt := &objtype.LocType{ConfigType: objtype.ConfigType{ID: 100}}
	s := newLocChangeState(loc, map[int]*objtype.LocType{100: lt})
	s.PushInt(9999) // unknown
	s.PushInt(3)
	if err := handleLocChange(s); err == nil {
		t.Error("handleLocChange with unknown type id must return error")
	}
}

// -- LOC_ADD tests --

func TestLocAddSameLayerCallsChangeOnExisting(t *testing.T) {
	// existing loc on layer 0 (wall); LOC_ADD with shape=0 (also wall layer 0)
	existing := fakeActiveLoc{id: 50, shape: 0, angle: 0, layer: 0}
	lt := &objtype.LocType{ConfigType: objtype.ConfigType{ID: 100}}
	s := &ScriptState{
		Script:      &ScriptFile{IntOperands: []int32{0}},
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Configs:     &fakeConfigs{locs: map[int]*objtype.LocType{100: lt}},
		LocOps:      &fakeLocOps{atCoord: []ActiveLoc{existing}},
	}

	level, x, z := 0, 3094, 3106
	coord := coordgrid.PackCoord(level, x, z)

	// stack: [coord, type=100, angle=0, shape=0 (wall→layer0), duration=3]
	s.PushInt(coord)
	s.PushInt(100)
	s.PushInt(0)
	s.PushInt(0) // ShapeWallStraight → LayerWall (0)
	s.PushInt(3)

	if err := handleLocAdd(s); err != nil {
		t.Fatalf("handleLocAdd: %v", err)
	}
	ops := s.LocOps.(*fakeLocOps)
	if len(ops.changeCalls) != 1 {
		t.Errorf("expected ChangeLoc on same-layer existing, got %d ChangeLoc calls", len(ops.changeCalls))
	}
	if len(ops.addCalls) != 0 {
		t.Errorf("expected no AddLoc when same-layer hit, got %d AddLoc calls", len(ops.addCalls))
	}
	if s.ActiveLoc != existing {
		t.Error("ActiveLoc must bind to the existing same-layer loc")
	}
}

func TestLocAddNoSameLayerCallsAddOnNew(t *testing.T) {
	// existing is on a DIFFERENT layer (groundDecor=3 vs wall=0)
	existing := fakeActiveLoc{id: 50, layer: 3}
	created := fakeActiveLoc{id: 100, shape: 0, angle: 0, layer: 0}
	lt := &objtype.LocType{ConfigType: objtype.ConfigType{ID: 100}}
	s := &ScriptState{
		Script:      &ScriptFile{IntOperands: []int32{0}},
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Configs:     &fakeConfigs{locs: map[int]*objtype.LocType{100: lt}},
		LocOps:      &fakeLocOps{atCoord: []ActiveLoc{existing}, addReturn: created},
	}

	coord := coordgrid.PackCoord(0, 3094, 3106)
	s.PushInt(coord)
	s.PushInt(100)
	s.PushInt(0)
	s.PushInt(0) // wall layer
	s.PushInt(3)

	if err := handleLocAdd(s); err != nil {
		t.Fatalf("handleLocAdd: %v", err)
	}
	ops := s.LocOps.(*fakeLocOps)
	if len(ops.addCalls) != 1 {
		t.Errorf("expected AddLoc, got %d AddLoc calls", len(ops.addCalls))
	}
	if len(ops.changeCalls) != 0 {
		t.Errorf("no same-layer hit should not call ChangeLoc, got %d", len(ops.changeCalls))
	}
}

// TestLocAddChangeBranchSlot0 pins LOC_ADD change-branch IntOperand=0:
// existing same-layer hit routes to ActiveLoc + PtrActiveLoc, leaves
// OtherActiveLoc/PtrActiveLoc2 untouched. Mirrors TS
// state.pointerAdd(ActiveLoc[state.intOperand]) at LocOps.ts:34.
func TestLocAddChangeBranchSlot0(t *testing.T) {
	existing := fakeActiveLoc{id: 50, shape: 0, angle: 0, layer: 0}
	lt := &objtype.LocType{ConfigType: objtype.ConfigType{ID: 100}}
	s := &ScriptState{
		Script:      &ScriptFile{IntOperands: []int32{0}},
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Configs:     &fakeConfigs{locs: map[int]*objtype.LocType{100: lt}},
		LocOps:      &fakeLocOps{atCoord: []ActiveLoc{existing}},
	}
	s.PushInt(coordgrid.PackCoord(0, 3094, 3106))
	s.PushInt(100)
	s.PushInt(0)
	s.PushInt(0)
	s.PushInt(3)

	if err := handleLocAdd(s); err != nil {
		t.Fatalf("handleLocAdd: %v", err)
	}
	if s.ActiveLoc != existing {
		t.Errorf("ActiveLoc: got %v, want existing %v", s.ActiveLoc, existing)
	}
	if s.OtherActiveLoc != nil {
		t.Errorf("OtherActiveLoc: got %v, want nil (slot-0 must NOT touch slot-1)", s.OtherActiveLoc)
	}
	if s.Pointers&PtrActiveLoc == 0 {
		t.Error("Pointers: PtrActiveLoc not set")
	}
	if s.Pointers&PtrActiveLoc2 != 0 {
		t.Error("Pointers: PtrActiveLoc2 must NOT be set for IntOperand=0")
	}
}

// TestLocAddChangeBranchSlot1 pins LOC_ADD change-branch IntOperand=1:
// existing same-layer hit routes to OtherActiveLoc + PtrActiveLoc2,
// leaves ActiveLoc/PtrActiveLoc untouched.
func TestLocAddChangeBranchSlot1(t *testing.T) {
	existing := fakeActiveLoc{id: 50, shape: 0, angle: 0, layer: 0}
	lt := &objtype.LocType{ConfigType: objtype.ConfigType{ID: 100}}
	s := &ScriptState{
		Script:      &ScriptFile{IntOperands: []int32{1}},
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Configs:     &fakeConfigs{locs: map[int]*objtype.LocType{100: lt}},
		LocOps:      &fakeLocOps{atCoord: []ActiveLoc{existing}},
	}
	s.PushInt(coordgrid.PackCoord(0, 3094, 3106))
	s.PushInt(100)
	s.PushInt(0)
	s.PushInt(0)
	s.PushInt(3)

	if err := handleLocAdd(s); err != nil {
		t.Fatalf("handleLocAdd: %v", err)
	}
	if s.OtherActiveLoc != existing {
		t.Errorf("OtherActiveLoc: got %v, want existing %v", s.OtherActiveLoc, existing)
	}
	if s.ActiveLoc != nil {
		t.Errorf("ActiveLoc: got %v, want nil (slot-1 must NOT touch slot-0)", s.ActiveLoc)
	}
	if s.Pointers&PtrActiveLoc2 == 0 {
		t.Error("Pointers: PtrActiveLoc2 not set")
	}
	if s.Pointers&PtrActiveLoc != 0 {
		t.Error("Pointers: PtrActiveLoc must NOT be set for IntOperand=1")
	}
}

// TestLocAddCreateBranchSlot0 pins LOC_ADD create-branch IntOperand=0:
// no same-layer hit → AddLoc → ActiveLoc + PtrActiveLoc. Mirrors TS
// state.pointerAdd(ActiveLoc[state.intOperand]) at LocOps.ts:42.
func TestLocAddCreateBranchSlot0(t *testing.T) {
	existing := fakeActiveLoc{id: 50, layer: 3}
	created := fakeActiveLoc{id: 100, shape: 0, angle: 0, layer: 0}
	lt := &objtype.LocType{ConfigType: objtype.ConfigType{ID: 100}}
	s := &ScriptState{
		Script:      &ScriptFile{IntOperands: []int32{0}},
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Configs:     &fakeConfigs{locs: map[int]*objtype.LocType{100: lt}},
		LocOps:      &fakeLocOps{atCoord: []ActiveLoc{existing}, addReturn: created},
	}
	s.PushInt(coordgrid.PackCoord(0, 3094, 3106))
	s.PushInt(100)
	s.PushInt(0)
	s.PushInt(0)
	s.PushInt(3)

	if err := handleLocAdd(s); err != nil {
		t.Fatalf("handleLocAdd: %v", err)
	}
	if s.ActiveLoc != created {
		t.Errorf("ActiveLoc: got %v, want created %v", s.ActiveLoc, created)
	}
	if s.OtherActiveLoc != nil {
		t.Errorf("OtherActiveLoc: got %v, want nil (slot-0 must NOT touch slot-1)", s.OtherActiveLoc)
	}
	if s.Pointers&PtrActiveLoc == 0 {
		t.Error("Pointers: PtrActiveLoc not set")
	}
	if s.Pointers&PtrActiveLoc2 != 0 {
		t.Error("Pointers: PtrActiveLoc2 must NOT be set for IntOperand=0")
	}
}

// TestLocAddCreateBranchSlot1 pins LOC_ADD create-branch IntOperand=1:
// no same-layer hit → AddLoc → OtherActiveLoc + PtrActiveLoc2.
func TestLocAddCreateBranchSlot1(t *testing.T) {
	existing := fakeActiveLoc{id: 50, layer: 3}
	created := fakeActiveLoc{id: 100, shape: 0, angle: 0, layer: 0}
	lt := &objtype.LocType{ConfigType: objtype.ConfigType{ID: 100}}
	s := &ScriptState{
		Script:      &ScriptFile{IntOperands: []int32{1}},
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Configs:     &fakeConfigs{locs: map[int]*objtype.LocType{100: lt}},
		LocOps:      &fakeLocOps{atCoord: []ActiveLoc{existing}, addReturn: created},
	}
	s.PushInt(coordgrid.PackCoord(0, 3094, 3106))
	s.PushInt(100)
	s.PushInt(0)
	s.PushInt(0)
	s.PushInt(3)

	if err := handleLocAdd(s); err != nil {
		t.Fatalf("handleLocAdd: %v", err)
	}
	if s.OtherActiveLoc != created {
		t.Errorf("OtherActiveLoc: got %v, want created %v", s.OtherActiveLoc, created)
	}
	if s.ActiveLoc != nil {
		t.Errorf("ActiveLoc: got %v, want nil (slot-1 must NOT touch slot-0)", s.ActiveLoc)
	}
	if s.Pointers&PtrActiveLoc2 == 0 {
		t.Error("Pointers: PtrActiveLoc2 not set")
	}
	if s.Pointers&PtrActiveLoc != 0 {
		t.Error("Pointers: PtrActiveLoc must NOT be set for IntOperand=1")
	}
}

func TestLocAddRejectsBadDuration(t *testing.T) {
	lt := &objtype.LocType{ConfigType: objtype.ConfigType{ID: 100}}
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Configs:     &fakeConfigs{locs: map[int]*objtype.LocType{100: lt}},
		LocOps:      &fakeLocOps{},
	}
	s.PushInt(coordgrid.PackCoord(0, 3094, 3106))
	s.PushInt(100)
	s.PushInt(0)
	s.PushInt(0)
	s.PushInt(0) // bad duration
	if err := handleLocAdd(s); err == nil {
		t.Error("handleLocAdd dur=0 must reject")
	}
}

func TestLocAddRejectsUnknownType(t *testing.T) {
	lt := &objtype.LocType{ConfigType: objtype.ConfigType{ID: 100}}
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Configs:     &fakeConfigs{locs: map[int]*objtype.LocType{100: lt}},
		LocOps:      &fakeLocOps{},
	}
	s.PushInt(coordgrid.PackCoord(0, 3094, 3106))
	s.PushInt(9999) // unknown
	s.PushInt(0)
	s.PushInt(0)
	s.PushInt(3)
	if err := handleLocAdd(s); err == nil {
		t.Error("handleLocAdd unknown type must reject")
	}
}

func TestLocAddRejectsBadShape(t *testing.T) {
	lt := &objtype.LocType{ConfigType: objtype.ConfigType{ID: 100}}
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Configs:     &fakeConfigs{locs: map[int]*objtype.LocType{100: lt}},
		LocOps:      &fakeLocOps{},
	}
	s.PushInt(coordgrid.PackCoord(0, 3094, 3106))
	s.PushInt(100)
	s.PushInt(0)
	s.PushInt(99) // shape > 22 → invalid
	s.PushInt(3)
	if err := handleLocAdd(s); err == nil {
		t.Error("handleLocAdd bad shape must reject")
	}
}

func TestLocAddRejectsBadAngle(t *testing.T) {
	lt := &objtype.LocType{ConfigType: objtype.ConfigType{ID: 100}}
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Configs:     &fakeConfigs{locs: map[int]*objtype.LocType{100: lt}},
		LocOps:      &fakeLocOps{},
	}
	s.PushInt(coordgrid.PackCoord(0, 3094, 3106))
	s.PushInt(100)
	s.PushInt(99) // angle > 3 → invalid
	s.PushInt(0)
	s.PushInt(3)
	if err := handleLocAdd(s); err == nil {
		t.Error("handleLocAdd bad angle must reject")
	}
}

// TestLocAddRejectsBadCoord pins the 2026-05-28 audit row h-loc-3.
// TS LOC_ADD (LocOps.ts:23) calls `check(coord, CoordValid)` BEFORE any
// other validator; CoordValid (ScriptValidators.ts:109) rejects packed
// coords outside [0, 2147483647] (negative coords incl. -1 / high-bit
// set). goscape's pre-fix handleLocAdd skipped this check and proceeded
// to UnpackCoord(coord) + LocsAtCoord with the garbage coords, then fell
// through to AddLoc on a bogus position. This guard mirrors the
// existing checkCoord usage at handlers_dialog.go:190 and elsewhere.
func TestLocAddRejectsBadCoord(t *testing.T) {
	lt := &objtype.LocType{ConfigType: objtype.ConfigType{ID: 100}}
	s := &ScriptState{
		Script:      &ScriptFile{IntOperands: []int32{0}},
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Configs:     &fakeConfigs{locs: map[int]*objtype.LocType{100: lt}},
		LocOps:      &fakeLocOps{},
	}
	// Negative packed coord (high-bit set) — outside CoordValid [0, 2^31-1].
	s.PushInt(-1)
	s.PushInt(100)
	s.PushInt(0)
	s.PushInt(0)
	s.PushInt(3)

	err := handleLocAdd(s)
	if err == nil {
		t.Fatal("handleLocAdd with negative coord (-1): expected error, got nil (TS LocOps.ts:23 must reject coord<0 via CoordValid)")
	}
	if !strings.Contains(err.Error(), "coord out of range") {
		t.Errorf("error message: got %q, want substring 'coord out of range' (matches checkCoord at handlers_npc.go:57)", err.Error())
	}
	ops := s.LocOps.(*fakeLocOps)
	if len(ops.addCalls) != 0 || len(ops.changeCalls) != 0 {
		t.Errorf("validator must short-circuit before LocOps calls; got %d addCalls, %d changeCalls (pre-fix proceeded with garbage coord)", len(ops.addCalls), len(ops.changeCalls))
	}
}

// -- LOC_DEL tests --

func TestLocDelCallsLocOps(t *testing.T) {
	loc := fakeActiveLoc{id: 100}
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		ActiveLoc:   loc,
		LocOps:      &fakeLocOps{},
	}
	s.PushInt(5)
	if err := handleLocDel(s); err != nil {
		t.Fatalf("handleLocDel: %v", err)
	}
	ops := s.LocOps.(*fakeLocOps)
	if len(ops.removeCalls) != 1 || ops.removeCalls[0].dur != 5 || ops.removeCalls[0].loc != loc {
		t.Errorf("RemoveLoc call: %+v", ops.removeCalls)
	}
}

func TestLocDelRequiresActiveLoc(t *testing.T) {
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushInt(5)
	if err := handleLocDel(s); err == nil {
		t.Error("handleLocDel without ActiveLoc must error")
	}
}

func TestLocDelRejectsBadDuration(t *testing.T) {
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		ActiveLoc:   fakeActiveLoc{},
		LocOps:      &fakeLocOps{},
	}
	s.PushInt(0)
	if err := handleLocDel(s); err == nil {
		t.Error("handleLocDel dur=0 must reject")
	}
}

// -- LOC_ANIM tests --

func TestLocAnimCallsLocOps(t *testing.T) {
	loc := fakeActiveLoc{id: 100}
	seq := &objtype.SeqType{ConfigType: objtype.ConfigType{ID: 42}}
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		ActiveLoc:   loc,
		Configs:     &fakeConfigs{seqs: map[int]*objtype.SeqType{42: seq}},
		LocOps:      &fakeLocOps{},
	}
	s.PushInt(42)
	if err := handleLocAnim(s); err != nil {
		t.Fatalf("handleLocAnim: %v", err)
	}
	ops := s.LocOps.(*fakeLocOps)
	if len(ops.animCalls) != 1 || ops.animCalls[0].seq != 42 {
		t.Errorf("AnimLoc call: %+v", ops.animCalls)
	}
}

func TestLocAnimRequiresActiveLoc(t *testing.T) {
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushInt(42)
	if err := handleLocAnim(s); err == nil {
		t.Error("handleLocAnim without ActiveLoc must error")
	}
}

func TestLocAnimRejectsUnknownSeq(t *testing.T) {
	s := &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		ActiveLoc:   fakeActiveLoc{},
		Configs:     &fakeConfigs{seqs: map[int]*objtype.SeqType{}},
		LocOps:      &fakeLocOps{},
	}
	s.PushInt(9999)
	if err := handleLocAnim(s); err == nil {
		t.Error("handleLocAnim unknown seq must reject")
	}
}

// --- NAI-119: LOC_FINDALLZONE handler tests --------------------------

// newLocFindAllZoneState builds a ScriptState with a coord on the int
// stack, World wired (for CurrentTick), LocOps wired. Mirror of
// newNpcFindNextState (handlers_npc_test.go) plus a stack-prepushed coord.
func newLocFindAllZoneState(t *testing.T, tick int, ops LocOps, coord int) *ScriptState {
	t.Helper()
	mw := newMockWorld()
	mw.tick = tick
	s := &ScriptState{
		Script:      &ScriptFile{IntOperands: []int32{0}},
		PC:          0,
		World:       mw,
		LocOps:      ops,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushInt(coord)
	return s
}

// TestLocFindAllZoneStoresIterator pins LOC_FINDALLZONE: pop coord →
// store iterator with creationTick from World.CurrentTick + level/x/z
// from coord.
func TestLocFindAllZoneStoresIterator(t *testing.T) {
	ops := newLocIterTestOps(nil)
	coord := coordgrid.PackCoord(0, 3200, 3300)
	s := newLocFindAllZoneState(t, 100, ops, coord)

	if err := handleLocFindAllZone(s); err != nil {
		t.Fatalf("handleLocFindAllZone: %v", err)
	}
	if s.locIterator == nil {
		t.Fatal("locIterator: got nil, want set")
	}
	if s.locIterator.creationTick != 100 {
		t.Errorf("creationTick: got %d, want 100 (from World.CurrentTick)",
			s.locIterator.creationTick)
	}
	if s.locIterator.level != 0 || s.locIterator.x != 3200 || s.locIterator.z != 3300 {
		t.Errorf("coord: got (%d, %d, %d), want (0, 3200, 3300)",
			s.locIterator.level, s.locIterator.x, s.locIterator.z)
	}
}

// TestLocFindAllZoneNilLocOpsDegrades pins the parallel-NPC nil-ops
// degradation: handler returns nil, locIterator stays nil.
func TestLocFindAllZoneNilLocOpsDegrades(t *testing.T) {
	coord := coordgrid.PackCoord(0, 3200, 3300)
	s := newLocFindAllZoneState(t, 100, nil, coord)
	// LocOps is nil — explicitly set to confirm.
	s.LocOps = nil

	if err := handleLocFindAllZone(s); err != nil {
		t.Fatalf("handleLocFindAllZone: got err %v, want nil (degrade silently)", err)
	}
	if s.locIterator != nil {
		t.Errorf("locIterator: got %v, want nil (no iterator on nil-ops)", s.locIterator)
	}
}

// TestLocFindAllZoneCoordValid pins the checkCoord error path: invalid
// coord (-1) yields the wrapped error.
func TestLocFindAllZoneCoordValid(t *testing.T) {
	ops := newLocIterTestOps(nil)
	s := newLocFindAllZoneState(t, 100, ops, -1)

	err := handleLocFindAllZone(s)
	if err == nil {
		t.Fatal("handleLocFindAllZone(-1): want error, got nil")
	}
	// Error string should be checkCoord's format; assert opcode tag.
	if want := "LOC_FINDALLZONE"; !strings.Contains(err.Error(), want) {
		t.Errorf("err: got %q, want substring %q", err.Error(), want)
	}
}

// --- NAI-119: LOC_FINDNEXT handler tests -----------------------------

// newLocFindNextState builds a ScriptState with World wired (for
// CurrentTick), an optional locIterator pre-installed, and IntOperands
// supplied for setActiveLocSlot to read. Mirror of newNpcFindNextState.
func newLocFindNextState(t *testing.T, tick int, iter *LocIterator, intOperand int32) *ScriptState {
	t.Helper()
	mw := newMockWorld()
	mw.tick = tick
	s := &ScriptState{
		Script:      &ScriptFile{IntOperands: []int32{intOperand}},
		PC:          0,
		World:       mw,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.locIterator = iter
	return s
}

// TestLocFindNextNoIterator pins the nil-iterator branch: pushes 0,
// no error, ActiveLoc/OtherActiveLoc untouched.
func TestLocFindNextNoIterator(t *testing.T) {
	s := newLocFindNextState(t, 100, nil, 0)

	if err := handleLocFindNext(s); err != nil {
		t.Fatalf("handleLocFindNext: %v", err)
	}
	if got := s.PopInt(); got != 0 {
		t.Errorf("nil iterator: got push %d, want 0", got)
	}
	if s.ActiveLoc != nil {
		t.Error("ActiveLoc should remain nil")
	}
	if s.OtherActiveLoc != nil {
		t.Error("OtherActiveLoc should remain nil")
	}
}

// TestLocFindNextHitPrimarySlot pins LOC_FINDNEXT IntOperand=0:
// pushes 1, sets ActiveLoc + PtrActiveLoc.
func TestLocFindNextHitPrimarySlot(t *testing.T) {
	loc := fakeActiveLoc{id: 100}
	ops := newLocIterTestOps([]ActiveLoc{loc})
	iter := NewZoneLocIterator(ops, 100, 0, 3200, 3300)
	s := newLocFindNextState(t, 100, iter, 0)

	if err := handleLocFindNext(s); err != nil {
		t.Fatalf("handleLocFindNext: %v", err)
	}
	if got := s.PopInt(); got != 1 {
		t.Errorf("hit: got push %d, want 1", got)
	}
	if s.ActiveLoc == nil || s.ActiveLoc.LocType() != 100 {
		t.Errorf("ActiveLoc: got %v, want id=100", s.ActiveLoc)
	}
	if s.OtherActiveLoc != nil {
		t.Error("OtherActiveLoc should remain nil for IntOperand=0")
	}
	if s.Pointers&PtrActiveLoc == 0 {
		t.Error("PtrActiveLoc should be set")
	}
	if s.Pointers&PtrActiveLoc2 != 0 {
		t.Error("PtrActiveLoc2 should NOT be set for IntOperand=0")
	}
}

// TestLocFindNextHitSecondarySlot pins LOC_FINDNEXT IntOperand=1:
// pushes 1, sets OtherActiveLoc + PtrActiveLoc2 (primary slot
// untouched). Closes NAI-119 dual-slot decision.
func TestLocFindNextHitSecondarySlot(t *testing.T) {
	loc := fakeActiveLoc{id: 200}
	ops := newLocIterTestOps([]ActiveLoc{loc})
	iter := NewZoneLocIterator(ops, 100, 0, 3200, 3300)
	s := newLocFindNextState(t, 100, iter, 1)

	if err := handleLocFindNext(s); err != nil {
		t.Fatalf("handleLocFindNext: %v", err)
	}
	if got := s.PopInt(); got != 1 {
		t.Errorf("hit: got push %d, want 1", got)
	}
	if s.OtherActiveLoc == nil || s.OtherActiveLoc.LocType() != 200 {
		t.Errorf("OtherActiveLoc: got %v, want id=200", s.OtherActiveLoc)
	}
	if s.ActiveLoc != nil {
		t.Error("ActiveLoc should remain nil for IntOperand=1")
	}
	if s.Pointers&PtrActiveLoc2 == 0 {
		t.Error("PtrActiveLoc2 should be set")
	}
	if s.Pointers&PtrActiveLoc != 0 {
		t.Error("PtrActiveLoc should NOT be set for IntOperand=1")
	}
}

// TestLocFindNextExhaustionPushesZero pins post-exhaustion behavior:
// FINDNEXT pushes 0, leaves ActiveLoc/OtherActiveLoc untouched.
func TestLocFindNextExhaustionPushesZero(t *testing.T) {
	ops := newLocIterTestOps([]ActiveLoc{}) // empty zone
	iter := NewZoneLocIterator(ops, 100, 0, 3200, 3300)
	s := newLocFindNextState(t, 100, iter, 0)

	if err := handleLocFindNext(s); err != nil {
		t.Fatalf("handleLocFindNext: %v", err)
	}
	if got := s.PopInt(); got != 0 {
		t.Errorf("exhaustion: got push %d, want 0", got)
	}
	if s.ActiveLoc != nil {
		t.Error("ActiveLoc should remain nil on exhaustion")
	}
}

// TestLocFindNextStaleErrors pins the stale-iterator error path:
// creationTick=99, currentTick=100 → handler returns the canonical
// "tried to use an old iterator" error matching the NPC family wording.
func TestLocFindNextStaleErrors(t *testing.T) {
	loc := fakeActiveLoc{id: 100}
	ops := newLocIterTestOps([]ActiveLoc{loc})
	iter := NewZoneLocIterator(ops, 99, 0, 3200, 3300) // creationTick=99
	s := newLocFindNextState(t, 100, iter, 0)          // currentTick=100

	err := handleLocFindNext(s)
	if err == nil {
		t.Fatal("stale: want error, got nil")
	}
	want := "LOC_FINDNEXT: tried to use an old iterator. Create a new iterator instead."
	if err.Error() != want {
		t.Errorf("err: got %q, want %q", err.Error(), want)
	}
}

// --- NAI-159: LOC_FIND handler tests ---------------------------------

// newLocFindState builds a ScriptState with [coord, locId] prepushed,
// IntOperand set for setActiveLocSlot to read, Configs wired (with locId
// pre-registered), and the given LocOps installed.
func newLocFindState(t *testing.T, ops LocOps, intOperand int32, coord, locId int) *ScriptState {
	t.Helper()
	cfg := &fakeConfigs{
		locs: map[int]*objtype.LocType{
			locId: {ConfigType: objtype.ConfigType{ID: locId}},
		},
	}
	s := &ScriptState{
		Script:      &ScriptFile{IntOperands: []int32{intOperand}},
		PC:          0,
		Configs:     cfg,
		LocOps:      ops,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushInt(coord) // first pushed → second popped (TS coord)
	s.PushInt(locId) // last pushed  → first popped (TS locId)
	return s
}

// TestHandleLocFindHitSlot0 pins the primary-slot hit arm: IntOperand=0
// routes to ActiveLoc + PtrActiveLoc; push 1.
func TestHandleLocFindHitSlot0(t *testing.T) {
	stub := fakeActiveLoc{id: 42, x: 3094, z: 3106, level: 0}
	ops := &fakeLocOps{getLocReturn: stub}
	coord := coordgrid.PackCoord(0, 3094, 3106)
	s := newLocFindState(t, ops, 0, coord, 42)

	if err := handleLocFind(s); err != nil {
		t.Fatalf("handleLocFind: %v", err)
	}
	if s.ActiveLoc != stub {
		t.Errorf("ActiveLoc: got %v, want stub %v", s.ActiveLoc, stub)
	}
	if s.OtherActiveLoc != nil {
		t.Errorf("OtherActiveLoc: got %v, want nil (slot-0 must NOT touch slot-1)", s.OtherActiveLoc)
	}
	if s.Pointers&PtrActiveLoc == 0 {
		t.Error("Pointers: PtrActiveLoc not set")
	}
	if got := s.PopInt(); got != 1 {
		t.Errorf("top of int stack: got %d, want 1", got)
	}
}

// TestHandleLocFindHitSlot1 pins the secondary-slot hit arm: IntOperand=1
// routes to OtherActiveLoc + PtrActiveLoc2.
func TestHandleLocFindHitSlot1(t *testing.T) {
	stub := fakeActiveLoc{id: 42, x: 3094, z: 3106, level: 0}
	ops := &fakeLocOps{getLocReturn: stub}
	coord := coordgrid.PackCoord(0, 3094, 3106)
	s := newLocFindState(t, ops, 1, coord, 42)

	if err := handleLocFind(s); err != nil {
		t.Fatalf("handleLocFind: %v", err)
	}
	if s.OtherActiveLoc != stub {
		t.Errorf("OtherActiveLoc: got %v, want stub %v", s.OtherActiveLoc, stub)
	}
	if s.ActiveLoc != nil {
		t.Errorf("ActiveLoc: got %v, want nil (slot-1 must NOT touch slot-0)", s.ActiveLoc)
	}
	if s.Pointers&PtrActiveLoc2 == 0 {
		t.Error("Pointers: PtrActiveLoc2 not set")
	}
	if got := s.PopInt(); got != 1 {
		t.Errorf("top of int stack: got %d, want 1", got)
	}
}

// TestHandleLocFindMissLeavesActiveLocUnchanged pins the miss-arm
// invariant: ActiveLoc is NOT mutated when GetLoc returns nil. Critical —
// TS only writes state.activeLoc inside the hit arm (LocOps.ts:91).
func TestHandleLocFindMissLeavesActiveLocUnchanged(t *testing.T) {
	sentinel := fakeActiveLoc{id: 7, x: 100, z: 200, level: 0}
	ops := &fakeLocOps{getLocReturn: nil} // miss
	coord := coordgrid.PackCoord(0, 3094, 3106)
	s := newLocFindState(t, ops, 0, coord, 42)
	s.ActiveLoc = sentinel
	prePointers := s.Pointers

	if err := handleLocFind(s); err != nil {
		t.Fatalf("handleLocFind: %v", err)
	}
	if s.ActiveLoc != sentinel {
		t.Errorf("ActiveLoc on miss: got %v, want sentinel %v", s.ActiveLoc, sentinel)
	}
	if s.Pointers != prePointers {
		t.Errorf("Pointers on miss: got %x, want unchanged %x", s.Pointers, prePointers)
	}
	if got := s.PopInt(); got != 0 {
		t.Errorf("top of int stack: got %d, want 0", got)
	}
}

// TestHandleLocFindNilConfigsErrors pins requireConfigs precondition.
func TestHandleLocFindNilConfigsErrors(t *testing.T) {
	ops := &fakeLocOps{}
	coord := coordgrid.PackCoord(0, 3094, 3106)
	s := newLocFindState(t, ops, 0, coord, 42)
	s.Configs = nil

	err := handleLocFind(s)
	if err == nil {
		t.Fatal("handleLocFind nil-Configs: want error, got nil")
	}
	if got, want := err.Error(), "LOC_FIND: Configs not set on ScriptState"; got != want {
		t.Errorf("error: got %q, want %q", got, want)
	}
}

// TestHandleLocFindNilLocOpsErrors pins the goscape-defensive nil-LocOps
// gate. Configs is populated; LocOps is nil; expect explicit error.
func TestHandleLocFindNilLocOpsErrors(t *testing.T) {
	coord := coordgrid.PackCoord(0, 3094, 3106)
	s := newLocFindState(t, nil, 0, coord, 42)
	s.LocOps = nil

	err := handleLocFind(s)
	if err == nil {
		t.Fatal("handleLocFind nil-LocOps: want error, got nil")
	}
	if got, want := err.Error(), "LOC_FIND: LocOps unavailable"; got != want {
		t.Errorf("error: got %q, want %q", got, want)
	}
}

// TestHandleLocFindUnknownLocIdErrors pins the LocTypeValid analogue:
// Configs.LocType returns nil for the popped locId → error, no push.
func TestHandleLocFindUnknownLocIdErrors(t *testing.T) {
	ops := &fakeLocOps{}
	coord := coordgrid.PackCoord(0, 3094, 3106)
	// fakeConfigs registers locId=42; pop locId=999 → unknown.
	s := newLocFindState(t, ops, 0, coord, 42)
	// Overwrite int stack: drop the locId=42 push and replace with 999.
	s.IntStack = make([]int, StackCapacity)
	s.ISP = 0
	s.PushInt(coord)
	s.PushInt(999)

	err := handleLocFind(s)
	if err == nil {
		t.Fatal("handleLocFind unknown locId: want error, got nil")
	}
	if got, want := err.Error(), "LOC_FIND: no LocType with value (999) found"; got != want {
		t.Errorf("error: got %q, want %q", got, want)
	}
}

// TestHandleLocFindBadCoordErrors pins checkCoord wrapping.
func TestHandleLocFindBadCoordErrors(t *testing.T) {
	ops := &fakeLocOps{}
	s := newLocFindState(t, ops, 0, 0, 42)
	// Replace stack: bad coord = -1.
	s.IntStack = make([]int, StackCapacity)
	s.ISP = 0
	s.PushInt(-1)
	s.PushInt(42)

	err := handleLocFind(s)
	if err == nil {
		t.Fatal("handleLocFind bad coord: want error, got nil")
	}
	if !strings.Contains(err.Error(), "LOC_FIND") {
		t.Errorf("error: got %q, want substring %q", err.Error(), "LOC_FIND")
	}
	if !strings.Contains(err.Error(), "coord out of range") {
		t.Errorf("error: got %q, want substring %q", err.Error(), "coord out of range")
	}
}

// TestHandleLocFindPopOrderAndUnpack pins (a) the locId-then-coord pop
// order (catches accidental swap) and (b) checkCoord unpacking the right
// (level, x, z) — via the recorded args on fakeLocOps. Per
// handler_pop_order_test_masking.md.
func TestHandleLocFindPopOrderAndUnpack(t *testing.T) {
	stub := fakeActiveLoc{id: 42, x: 3094, z: 3106, level: 0}
	ops := &fakeLocOps{getLocReturn: stub}
	coord := coordgrid.PackCoord(0, 3094, 3106)
	s := newLocFindState(t, ops, 0, coord, 42)

	if err := handleLocFind(s); err != nil {
		t.Fatalf("handleLocFind: %v", err)
	}
	if len(ops.getLocCalls) != 1 {
		t.Fatalf("getLocCalls: got %d, want 1", len(ops.getLocCalls))
	}
	got := ops.getLocCalls[0]
	want := getLocCall{level: 0, x: 3094, z: 3106, typ: 42}
	if got != want {
		t.Errorf("getLocCall: got %+v, want %+v", got, want)
	}
}
