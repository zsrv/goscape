package script

import (
	"strings"
	"testing"

	"github.com/zsrv/goscape/pkg/fonttype"
	"github.com/zsrv/goscape/pkg/objtype"
)

// mockConfigs implements the Configs interface with in-memory fixture maps.
type mockConfigs struct {
	objs           map[int]*objtype.ObjType
	objsByName     map[string]*objtype.ObjType // NAI-162 B2: ObjByName index
	npcs           map[int]*objtype.NpcType
	locs           map[int]*objtype.LocType
	enums          map[int]*objtype.EnumType
	structs        map[int]*objtype.StructType
	params         map[int]*objtype.ParamType
	invs           map[int]*objtype.InvType
	idks           map[int]*objtype.IdkType
	spotAnimTypes  map[int]*objtype.SpotanimType
	varps          map[int]*objtype.VarPlayerType
	varns          map[int]*objtype.VarNpcType
	seqs           map[int]*objtype.SeqType
	hunts          map[int]*objtype.HuntType
	mesanims       map[int]*objtype.MesanimType
	mesanimsByName map[string]int
	fonts          map[int]*fonttype.FontType
}

func (m *mockConfigs) ObjType(id int) *objtype.ObjType           { return m.objs[id] }
func (m *mockConfigs) NpcType(id int) *objtype.NpcType           { return m.npcs[id] }
func (m *mockConfigs) LocType(id int) *objtype.LocType           { return m.locs[id] }
func (m *mockConfigs) EnumType(id int) *objtype.EnumType         { return m.enums[id] }
func (m *mockConfigs) StructType(id int) *objtype.StructType     { return m.structs[id] }
func (m *mockConfigs) ParamType(id int) *objtype.ParamType       { return m.params[id] }
func (m *mockConfigs) InvType(id int) *objtype.InvType           { return m.invs[id] }
func (m *mockConfigs) IdkType(id int) *objtype.IdkType           { return m.idks[id] }
func (m *mockConfigs) SpotAnimType(id int) *objtype.SpotanimType { return m.spotAnimTypes[id] }
func (m *mockConfigs) SeqType(id int) *objtype.SeqType           { return m.seqs[id] }
func (m *mockConfigs) HuntType(id int) *objtype.HuntType         { return m.hunts[id] }
func (m *mockConfigs) MesanimType(id int) *objtype.MesanimType   { return m.mesanims[id] }
func (m *mockConfigs) MesanimByName(name string) int {
	if m.mesanimsByName == nil {
		return -1
	}
	id, ok := m.mesanimsByName[name]
	if !ok {
		return -1
	}
	return id
}
func (m *mockConfigs) FontType(id int) *fonttype.FontType           { return m.fonts[id] }
func (m *mockConfigs) DbTableType(id int) *objtype.DbTableType      { return nil }
func (m *mockConfigs) DbRowType(id int) *objtype.DbRowType          { return nil }
func (m *mockConfigs) DbRowsInTable(tableID int) []int              { return nil }
func (m *mockConfigs) FindDbRowsInt(query int32, packed int) []int  { return nil }
func (m *mockConfigs) FindDbRowsStr(query string, packed int) []int { return nil }

func (m *mockConfigs) ObjByName(name string) *objtype.ObjType {
	if m.objsByName == nil {
		return nil
	}
	return m.objsByName[name]
}

func (m *mockConfigs) VarpType(id int) (objtype.ScriptVarType, bool) {
	v, ok := m.varps[id]
	if !ok || v == nil {
		return objtype.ScriptVarTypeInt, false
	}
	return v.Type, v.Protect
}

func (m *mockConfigs) VarnType(id int) objtype.ScriptVarType {
	v, ok := m.varns[id]
	if !ok || v == nil {
		return objtype.ScriptVarTypeInt
	}
	return v.Type
}

// newTestConfigs seeds a fresh mockConfigs with the canonical fixture used
// across handler tests.
func newTestConfigs() *mockConfigs {
	mc := &mockConfigs{
		objs:    make(map[int]*objtype.ObjType),
		npcs:    make(map[int]*objtype.NpcType),
		locs:    make(map[int]*objtype.LocType),
		enums:   make(map[int]*objtype.EnumType),
		structs: make(map[int]*objtype.StructType),
		params:  make(map[int]*objtype.ParamType),
	}

	// ObjType id 995: "Coins" — a minimal happy-path fixture.
	coins := objtype.NewObjType(995)
	coins.Name = "Coins"
	coins.DebugName = "coins"
	coins.Desc = "Lovely money!"
	coins.Cost = 1
	coins.Stackable = true
	coins.Members = false
	coins.Tradeable = true
	coins.Weight = 0
	coins.Category = 7
	coins.WearPos = -1
	coins.WearPos2 = -1
	coins.WearPos3 = -1
	coins.Params = objtype.ParamMap{
		1: uint32(42),
		// 0xFFFFFFFC bit pattern = int32(-4). Encoded this way because
		// DecodeParams reads param ints via Packet.G4() (uint32 return).
		5: uint32(0xFFFFFFFC),
	}
	mc.objs[995] = coins

	// ObjType id 1: a wearable item with wearpos set.
	hat := objtype.NewObjType(1)
	hat.Name = "Cool Hat"
	hat.DebugName = "cool_hat"
	hat.Desc = "A pointy hat."
	hat.Members = true
	hat.Tradeable = false
	hat.Weight = 100
	hat.Cost = 50
	hat.WearPos = 0
	hat.WearPos2 = 8
	hat.WearPos3 = 11
	hat.Category = 1
	hat.Params = objtype.ParamMap{2: "magical"}
	mc.objs[1] = hat

	// ObjType id 10: cert pair for OC_CERT — base item with certlink > 0.
	base := objtype.NewObjType(10)
	base.Name = "Logs"
	base.DebugName = "logs"
	base.CertLink = 11
	base.CertTemplate = -1
	mc.objs[10] = base

	// ObjType id 11: the cert / note version of item 10.
	note := objtype.NewObjType(11)
	note.Name = "Logs (note)"
	note.DebugName = "logs_note"
	note.CertLink = 10
	note.CertTemplate = 799
	mc.objs[11] = note

	// NpcType id 0: "man" with a 3-entry Op slice.
	man := objtype.NewNpcType(0)
	man.Name = "man"
	man.DebugName = "man"
	man.Desc = "An ordinary chap."
	man.Size = 1
	man.VisLevel = 2
	man.Category = 3
	man.Op = []string{"Talk-to", "", "Pickpocket"}
	man.Params = objtype.ParamMap{1: uint32(7)}
	mc.npcs[0] = man

	// NpcType id 1: minimal — no Name, has DebugName only.
	unnamed := objtype.NewNpcType(1)
	unnamed.DebugName = "unnamed_npc"
	mc.npcs[1] = unnamed

	// LocType id 0: a door-like loc.
	door := objtype.NewLocType(0)
	door.DebugName = "door"
	door.Desc = "A sturdy door."
	door.Category = 1
	door.Width = 1
	door.Length = 2
	door.Params = objtype.ParamMap{2: "wooden"}
	mc.locs[0] = door

	// EnumType id 0: int → string mapping with default "?".
	intToStr := objtype.NewEnumType(0)
	intToStr.DebugName = "stat_name_enum"
	intToStr.InputType = objtype.ScriptVarTypeInt
	intToStr.OutputType = objtype.ScriptVarTypeString
	intToStr.DefaultString = "?"
	intToStr.Values = map[int32]any{int32(1): "one", int32(2): "two"}
	mc.enums[0] = intToStr

	// EnumType id 1: int → int mapping with default -1.
	intToInt := objtype.NewEnumType(1)
	intToInt.DebugName = "double_enum"
	intToInt.InputType = objtype.ScriptVarTypeInt
	intToInt.OutputType = objtype.ScriptVarTypeInt
	intToInt.DefaultInt = -1
	intToInt.Values = map[int32]any{int32(3): int32(6), int32(4): int32(8)}
	mc.enums[1] = intToInt

	// StructType id 0: carries an int param.
	st0 := objtype.NewStructType(0)
	st0.DebugName = "struct_zero"
	st0.Params = objtype.ParamMap{1: uint32(99)}
	mc.structs[0] = st0

	// ParamType id 1: INT type.
	pInt := objtype.NewParamType(1)
	pInt.DebugName = "p_int"
	pInt.Type = objtype.ScriptVarTypeInt
	pInt.DefaultInt = 0
	mc.params[1] = pInt

	// ParamType id 2: STRING type.
	pStr := objtype.NewParamType(2)
	pStr.DebugName = "p_str"
	pStr.Type = objtype.ScriptVarTypeString
	pStr.DefaultString = ""
	mc.params[2] = pStr

	// ParamType id 3: STRING type with a non-empty default (for fallback tests).
	pStrDef := objtype.NewParamType(3)
	pStrDef.DebugName = "p_str_default"
	pStrDef.Type = objtype.ScriptVarTypeString
	pStrDef.DefaultString = "fallback"
	mc.params[3] = pStrDef

	// ParamType id 4: INT type with a non-zero default.
	pIntDef := objtype.NewParamType(4)
	pIntDef.DebugName = "p_int_default"
	pIntDef.Type = objtype.ScriptVarTypeInt
	pIntDef.DefaultInt = 77
	mc.params[4] = pIntDef

	// ParamType id 5: INT type, default 0. Used by the
	// negative-int32-sign-preservation test (param value bytes
	// 0xFFFFFFFC = int32(-4); RuneScape weapon configs encode
	// negative attack/defence bonuses this way).
	pIntNeg := objtype.NewParamType(5)
	pIntNeg.DebugName = "p_int_neg"
	pIntNeg.Type = objtype.ScriptVarTypeInt
	pIntNeg.DefaultInt = 0
	mc.params[5] = pIntNeg

	return mc
}

// runConfigOp executes a single-opcode script with pre-pushed int inputs
// against mc, and returns the resulting state.
func runConfigOp(t *testing.T, mc *mockConfigs, op Opcode, intInputs []int) *ScriptState {
	t.Helper()
	sf := &ScriptFile{
		Name:             "test_" + op.String(),
		Opcodes:          []Opcode{op, OpReturn},
		IntOperands:      []int32{0, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	state := Init(sf, nil, false, nil, nil)
	state.Configs = mc
	for _, v := range intInputs {
		state.PushInt(v)
	}
	if err := Execute(state); err != nil {
		t.Fatalf("%s: unexpected error: %v", op.String(), err)
	}
	return state
}

// runConfigOpExpectErr runs a single-op script and asserts that Execute
// returns an error whose string contains substr.
func runConfigOpExpectErr(t *testing.T, mc *mockConfigs, op Opcode, intInputs []int, substr string) {
	t.Helper()
	sf := &ScriptFile{
		Name:             "test_" + op.String(),
		Opcodes:          []Opcode{op, OpReturn},
		IntOperands:      []int32{0, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	state := Init(sf, nil, false, nil, nil)
	state.Configs = mc
	for _, v := range intInputs {
		state.PushInt(v)
	}
	err := Execute(state)
	if err == nil {
		t.Fatalf("%s: expected error containing %q, got nil", op.String(), substr)
	}
	if !strings.Contains(err.Error(), substr) {
		t.Fatalf("%s: expected error containing %q, got %q", op.String(), substr, err.Error())
	}
}

// -- Config-registry validator unit tests --

func TestCheckParamType(t *testing.T) {
	tests := []struct {
		name      string
		id        int
		setup     func() *mockConfigs
		wantErr   bool
		wantSubst string
	}{
		{
			name:    "valid id",
			id:      1,
			setup:   func() *mockConfigs { return &mockConfigs{params: map[int]*objtype.ParamType{1: {}}} },
			wantErr: false,
		},
		{
			name:      "unknown id",
			id:        100,
			setup:     func() *mockConfigs { return &mockConfigs{params: map[int]*objtype.ParamType{}} },
			wantErr:   true,
			wantSubst: "OP: no ParamType with value (100) found",
		},
		{
			name:      "negative id",
			id:        -1,
			setup:     func() *mockConfigs { return &mockConfigs{params: map[int]*objtype.ParamType{}} },
			wantErr:   true,
			wantSubst: "OP: no ParamType with value (-1) found",
		},
		{
			name:      "nil Configs",
			id:        0,
			setup:     func() *mockConfigs { return nil },
			wantErr:   true,
			wantSubst: "OP: no ParamType with value (0) found",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := &ScriptState{}
			if cfg := tc.setup(); cfg != nil {
				s.Configs = cfg
			}
			err := checkParamType(s, tc.id, "OP")
			if tc.wantErr {
				if err == nil {
					t.Fatalf("checkParamType(%d): want error, got nil", tc.id)
				}
				if !strings.Contains(err.Error(), tc.wantSubst) {
					t.Errorf("error message: got %q, want contains %q", err.Error(), tc.wantSubst)
				}
			} else {
				if err != nil {
					t.Fatalf("checkParamType(%d): want nil, got %v", tc.id, err)
				}
			}
		})
	}
}

func TestCheckEnumType(t *testing.T) {
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
			setup:   func() *mockConfigs { return &mockConfigs{enums: map[int]*objtype.EnumType{0: {}}} },
			wantErr: false,
		},
		{
			name:      "unknown id",
			id:        100,
			setup:     func() *mockConfigs { return &mockConfigs{enums: map[int]*objtype.EnumType{}} },
			wantErr:   true,
			wantSubst: "OP: no EnumType with value (100) found",
		},
		{
			name:      "negative id",
			id:        -1,
			setup:     func() *mockConfigs { return &mockConfigs{enums: map[int]*objtype.EnumType{}} },
			wantErr:   true,
			wantSubst: "OP: no EnumType with value (-1) found",
		},
		{
			name:      "nil Configs",
			id:        0,
			setup:     func() *mockConfigs { return nil },
			wantErr:   true,
			wantSubst: "OP: no EnumType with value (0) found",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := &ScriptState{}
			if cfg := tc.setup(); cfg != nil {
				s.Configs = cfg
			}
			err := checkEnumType(s, tc.id, "OP")
			if tc.wantErr {
				if err == nil {
					t.Fatalf("checkEnumType(%d): want error, got nil", tc.id)
				}
				if !strings.Contains(err.Error(), tc.wantSubst) {
					t.Errorf("error message: got %q, want contains %q", err.Error(), tc.wantSubst)
				}
			} else {
				if err != nil {
					t.Fatalf("checkEnumType(%d): want nil, got %v", tc.id, err)
				}
			}
		})
	}
}

func TestCheckStructType(t *testing.T) {
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
			setup:   func() *mockConfigs { return &mockConfigs{structs: map[int]*objtype.StructType{0: {}}} },
			wantErr: false,
		},
		{
			name:      "unknown id",
			id:        100,
			setup:     func() *mockConfigs { return &mockConfigs{structs: map[int]*objtype.StructType{}} },
			wantErr:   true,
			wantSubst: "OP: no StructType with value (100) found",
		},
		{
			name:      "negative id",
			id:        -1,
			setup:     func() *mockConfigs { return &mockConfigs{structs: map[int]*objtype.StructType{}} },
			wantErr:   true,
			wantSubst: "OP: no StructType with value (-1) found",
		},
		{
			name:      "nil Configs",
			id:        0,
			setup:     func() *mockConfigs { return nil },
			wantErr:   true,
			wantSubst: "OP: no StructType with value (0) found",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := &ScriptState{}
			if cfg := tc.setup(); cfg != nil {
				s.Configs = cfg
			}
			err := checkStructType(s, tc.id, "OP")
			if tc.wantErr {
				if err == nil {
					t.Fatalf("checkStructType(%d): want error, got nil", tc.id)
				}
				if !strings.Contains(err.Error(), tc.wantSubst) {
					t.Errorf("error message: got %q, want contains %q", err.Error(), tc.wantSubst)
				}
			} else {
				if err != nil {
					t.Fatalf("checkStructType(%d): want nil, got %v", tc.id, err)
				}
			}
		})
	}
}

// -- EnumOps tests --

func TestEnumIntToString(t *testing.T) {
	mc := newTestConfigs()
	// Stack: [inputType, outputType, enumID, key]; key on top.
	state := runConfigOp(t, mc, OpEnum, []int{
		int(objtype.ScriptVarTypeInt),
		int(objtype.ScriptVarTypeString),
		0,
		1,
	})
	if got := state.PopString(); got != "one" {
		t.Errorf("ENUM(0,1): got %q, want %q", got, "one")
	}
}

func TestEnumIntToStringMissingKeyFallsBackToDefault(t *testing.T) {
	mc := newTestConfigs()
	state := runConfigOp(t, mc, OpEnum, []int{
		int(objtype.ScriptVarTypeInt),
		int(objtype.ScriptVarTypeString),
		0,
		999,
	})
	if got := state.PopString(); got != "?" {
		t.Errorf("ENUM(0,999): got %q, want default %q", got, "?")
	}
}

func TestEnumIntToInt(t *testing.T) {
	mc := newTestConfigs()
	state := runConfigOp(t, mc, OpEnum, []int{
		int(objtype.ScriptVarTypeInt),
		int(objtype.ScriptVarTypeInt),
		1,
		3,
	})
	if got := state.PopInt(); got != 6 {
		t.Errorf("ENUM int→int (1,3): got %d, want 6", got)
	}
}

func TestEnumIntToIntMissingKeyFallsBackToDefault(t *testing.T) {
	mc := newTestConfigs()
	state := runConfigOp(t, mc, OpEnum, []int{
		int(objtype.ScriptVarTypeInt),
		int(objtype.ScriptVarTypeInt),
		1,
		999,
	})
	if got := state.PopInt(); got != -1 {
		t.Errorf("ENUM int→int (1,999): got %d, want default -1", got)
	}
}

func TestEnumTypeMismatchErrors(t *testing.T) {
	mc := newTestConfigs()
	// Enum 0 is int→string; assert int→int fails validation.
	runConfigOpExpectErr(t, mc, OpEnum, []int{
		int(objtype.ScriptVarTypeInt),
		int(objtype.ScriptVarTypeInt),
		0,
		1,
	}, "type validation error")
}

func TestEnumUnknownIdErrors(t *testing.T) {
	mc := newTestConfigs()
	runConfigOpExpectErr(t, mc, OpEnum, []int{
		int(objtype.ScriptVarTypeInt),
		int(objtype.ScriptVarTypeString),
		999,
		1,
	}, "ENUM: no EnumType with value (999) found")
}

func TestEnumGetOutputCount(t *testing.T) {
	mc := newTestConfigs()
	state := runConfigOp(t, mc, OpEnumGetOutputCount, []int{0})
	if got := state.PopInt(); got != 2 {
		t.Errorf("ENUM_GETOUTPUTCOUNT(0): got %d, want 2", got)
	}
}

func TestEnumGetOutputCountUnknownErrors(t *testing.T) {
	mc := newTestConfigs()
	runConfigOpExpectErr(t, mc, OpEnumGetOutputCount, []int{999}, "ENUM_GETOUTPUTCOUNT: no EnumType with value (999) found")
}

// -- StructOps tests --

func TestStructParamInt(t *testing.T) {
	mc := newTestConfigs()
	// Stack: [structID, paramID]; paramID on top.
	state := runConfigOp(t, mc, OpStructParam, []int{0, 1})
	if got := state.PopInt(); got != 99 {
		t.Errorf("STRUCT_PARAM(0,1): got %d, want 99", got)
	}
}

func TestStructParamMissingKeyFallsBackToParamDefault(t *testing.T) {
	mc := newTestConfigs()
	// Struct 0 has no key 4; ParamType 4 (int) default = 77.
	state := runConfigOp(t, mc, OpStructParam, []int{0, 4})
	if got := state.PopInt(); got != 77 {
		t.Errorf("STRUCT_PARAM(0,4 missing): got %d, want default 77", got)
	}
}

func TestStructParamUnknownStructErrors(t *testing.T) {
	mc := newTestConfigs()
	runConfigOpExpectErr(t, mc, OpStructParam, []int{999, 1}, "STRUCT_PARAM: no StructType with value (999) found")
}

// -- LocConfigOps tests --

func TestLcName(t *testing.T) {
	mc := newTestConfigs()
	state := runConfigOp(t, mc, OpLcName, []int{0})
	// Loc 0 has DebugName only (no Name); falls back to DebugName.
	if got := state.PopString(); got != "door" {
		t.Errorf("LC_NAME(0): got %q, want %q", got, "door")
	}
}

func TestLcNameNullFallback(t *testing.T) {
	mc := newTestConfigs()
	// Seed an unnamed loc at id 1.
	mc.locs[1] = objtype.NewLocType(1)
	state := runConfigOp(t, mc, OpLcName, []int{1})
	if got := state.PopString(); got != "null" {
		t.Errorf("LC_NAME(1 unnamed): got %q, want %q", got, "null")
	}
}

func TestLcNamePrefersNameOverDebugName(t *testing.T) {
	mc := newTestConfigs()
	// Seed a loc with both Name and DebugName at id 2; Name wins.
	named := objtype.NewLocType(2)
	named.Name = "Door"
	named.DebugName = "door"
	mc.locs[2] = named
	state := runConfigOp(t, mc, OpLcName, []int{2})
	if got := state.PopString(); got != "Door" {
		t.Errorf("LC_NAME(2 named): got %q, want %q", got, "Door")
	}
}

func TestLcParamString(t *testing.T) {
	mc := newTestConfigs()
	state := runConfigOp(t, mc, OpLcParam, []int{0, 2})
	if got := state.PopString(); got != "wooden" {
		t.Errorf("LC_PARAM(0,2): got %q, want %q", got, "wooden")
	}
}

func TestLcCategory(t *testing.T) {
	mc := newTestConfigs()
	state := runConfigOp(t, mc, OpLcCategory, []int{0})
	if got := state.PopInt(); got != 1 {
		t.Errorf("LC_CATEGORY(0): got %d, want 1", got)
	}
}

func TestLcDesc(t *testing.T) {
	mc := newTestConfigs()
	state := runConfigOp(t, mc, OpLcDesc, []int{0})
	if got := state.PopString(); got != "A sturdy door." {
		t.Errorf("LC_DESC(0): got %q, want %q", got, "A sturdy door.")
	}
}

func TestLcDebugName(t *testing.T) {
	mc := newTestConfigs()
	state := runConfigOp(t, mc, OpLcDebugName, []int{0})
	if got := state.PopString(); got != "door" {
		t.Errorf("LC_DEBUGNAME(0): got %q, want %q", got, "door")
	}
}

func TestLcWidth(t *testing.T) {
	mc := newTestConfigs()
	state := runConfigOp(t, mc, OpLcWidth, []int{0})
	if got := state.PopInt(); got != 1 {
		t.Errorf("LC_WIDTH(0): got %d, want 1", got)
	}
}

func TestLcLength(t *testing.T) {
	mc := newTestConfigs()
	state := runConfigOp(t, mc, OpLcLength, []int{0})
	if got := state.PopInt(); got != 2 {
		t.Errorf("LC_LENGTH(0): got %d, want 2", got)
	}
}

// TestLcUnknownIdErrors pins canonical "<TAG>: no LocType with value (999)
// found" rejection at every Configs-side LC_* handler wired with checkLocType.
//
// LC_PARAM is the only 2-arg row: pushes [locID=999, paramID=0]; handler pops
// paramID first then locID, so checkLocType sees 999 before the (unreached)
// paramLookup.
func TestLcUnknownIdErrors(t *testing.T) {
	mc := newTestConfigs()
	cases := []struct {
		op     Opcode
		inputs []int
		tag    string
	}{
		{OpLcName, []int{999}, "LC_NAME"},
		{OpLcParam, []int{999, 0}, "LC_PARAM"},
		{OpLcCategory, []int{999}, "LC_CATEGORY"},
		{OpLcDesc, []int{999}, "LC_DESC"},
		{OpLcDebugName, []int{999}, "LC_DEBUGNAME"},
		{OpLcWidth, []int{999}, "LC_WIDTH"},
		{OpLcLength, []int{999}, "LC_LENGTH"},
	}
	for _, c := range cases {
		t.Run(c.tag, func(t *testing.T) {
			runConfigOpExpectErr(t, mc, c.op, c.inputs,
				c.tag+": no LocType with value (999) found")
		})
	}
}

// -- NpcConfigOps tests --

func TestNcName(t *testing.T) {
	mc := newTestConfigs()
	state := runConfigOp(t, mc, OpNcName, []int{0})
	if got := state.PopString(); got != "man" {
		t.Errorf("NC_NAME(0): got %q, want %q", got, "man")
	}
}

func TestNcNameFallsBackToDebugName(t *testing.T) {
	mc := newTestConfigs()
	// NPC id 1 has DebugName but no Name.
	state := runConfigOp(t, mc, OpNcName, []int{1})
	if got := state.PopString(); got != "unnamed_npc" {
		t.Errorf("NC_NAME(1): got %q, want %q", got, "unnamed_npc")
	}
}

func TestNcParamInt(t *testing.T) {
	mc := newTestConfigs()
	state := runConfigOp(t, mc, OpNcParam, []int{0, 1})
	if got := state.PopInt(); got != 7 {
		t.Errorf("NC_PARAM(0,1): got %d, want 7", got)
	}
}

// NPC_PARAM (opcode 2529) — int param push from active npc.
// Mirrors TS NpcOps.ts:132-141 (NPC_PARAM): pop only paramID,
// read npcID from state.activeNpc.type.
func TestNpcParamInt(t *testing.T) {
	mc := newTestConfigs()
	npc := &mockNpc{typeID: 0} // NPC id 0 has Params{1: uint32(7)} per newTestConfigs.
	state := runNpcOp(t, npc, mc, OpNpcParam, []int{1})
	if got := state.PopInt(); got != 7 {
		t.Errorf("NPC_PARAM(active=0, paramID=1): got %d, want 7", got)
	}
}

// NPC_PARAM string param. Extends NPC 0's ParamMap inline since
// newTestConfigs only seeds an int param for NPC 0.
func TestNpcParamString(t *testing.T) {
	mc := newTestConfigs()
	mc.npcs[0].Params[2] = "hello"
	npc := &mockNpc{typeID: 0}
	state := runNpcOp(t, npc, mc, OpNpcParam, []int{2})
	if got := state.PopString(); got != "hello" {
		t.Errorf("NPC_PARAM(active=0, paramID=2): got %q, want %q", got, "hello")
	}
}

// NPC_PARAM with no active npc → error tagged NPC_PARAM.
func TestNpcParamNoActiveNpcErrors(t *testing.T) {
	mc := newTestConfigs()
	sf := &ScriptFile{
		Name:             "npc_param_no_active",
		Opcodes:          []Opcode{OpNpcParam, OpReturn},
		IntOperands:      []int32{0, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	state := Init(sf, nil, false, nil, nil)
	state.Configs = mc
	state.PushInt(1)
	err := Execute(state)
	if err == nil || !strings.Contains(err.Error(), "NPC_PARAM") {
		t.Errorf("expected NPC_PARAM error, got %v", err)
	}
}

// NPC_PARAM with nil Configs → error tagged NPC_PARAM (via requireConfigs).
func TestNpcParamNilConfigsErrors(t *testing.T) {
	sf := &ScriptFile{
		Name:             "npc_param_nil_configs",
		Opcodes:          []Opcode{OpNpcParam, OpReturn},
		IntOperands:      []int32{0, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	state := Init(sf, nil, false, nil, nil)
	state.ActiveNpc = &mockNpc{typeID: 0}
	state.PushInt(1)
	err := Execute(state)
	if err == nil || !strings.Contains(err.Error(), "NPC_PARAM") {
		t.Errorf("expected NPC_PARAM-tagged error, got %v", err)
	}
}

// NPC_PARAM where active npc has unknown type id → error.
func TestNpcParamUnknownNpcIdErrors(t *testing.T) {
	mc := newTestConfigs()
	sf := &ScriptFile{
		Name:             "npc_param_unknown_npc",
		Opcodes:          []Opcode{OpNpcParam, OpReturn},
		IntOperands:      []int32{0, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	state := Init(sf, nil, false, nil, nil)
	state.ActiveNpc = &mockNpc{typeID: 999}
	state.Configs = mc
	state.PushInt(1)
	err := Execute(state)
	if err == nil || !strings.Contains(err.Error(), "NPC_PARAM") {
		t.Errorf("expected NPC_PARAM-tagged error, got %v", err)
	}
}

// NPC_PARAM with valid active-npc typeID but unknown paramID → error from
// paramLookup's checkParamType gate. Complements TestNpcParamUnknownNpcIdErrors
// (one gate earlier, checkNpcType) and closes the deliberate-structural
// carry-forward from [[lc-param-enum-struct-unknown-id-symmetry-close]]:
// runConfigOpExpectErr doesn't set ActiveNpc, so the active-npc form needs
// runNpcOpExpectErr instead.
func TestNpcParamUnknownParamErrors(t *testing.T) {
	npc := &mockNpc{typeID: 0}
	mc := newTestConfigs()
	runNpcOpExpectErr(t, npc, mc, OpNpcParam, []int{999},
		"NPC_PARAM: no ParamType with value (999) found")
}

func TestNcCategory(t *testing.T) {
	mc := newTestConfigs()
	state := runConfigOp(t, mc, OpNcCategory, []int{0})
	if got := state.PopInt(); got != 3 {
		t.Errorf("NC_CATEGORY(0): got %d, want 3", got)
	}
}

func TestNcDesc(t *testing.T) {
	mc := newTestConfigs()
	state := runConfigOp(t, mc, OpNcDesc, []int{0})
	if got := state.PopString(); got != "An ordinary chap." {
		t.Errorf("NC_DESC(0): got %q, want %q", got, "An ordinary chap.")
	}
}

func TestNcDebugName(t *testing.T) {
	mc := newTestConfigs()
	state := runConfigOp(t, mc, OpNcDebugName, []int{0})
	if got := state.PopString(); got != "man" {
		t.Errorf("NC_DEBUGNAME(0): got %q, want %q", got, "man")
	}
}

func TestNcOp1Based(t *testing.T) {
	mc := newTestConfigs()
	// Op[0] = "Talk-to"; NC_OP with op=1 pushes it.
	state := runConfigOp(t, mc, OpNcOp, []int{0, 1})
	if got := state.PopString(); got != "Talk-to" {
		t.Errorf("NC_OP(0, 1): got %q, want %q", got, "Talk-to")
	}
}

func TestNcOpThird(t *testing.T) {
	mc := newTestConfigs()
	// Op[2] = "Pickpocket"; NC_OP with op=3 pushes it.
	state := runConfigOp(t, mc, OpNcOp, []int{0, 3})
	if got := state.PopString(); got != "Pickpocket" {
		t.Errorf("NC_OP(0, 3): got %q, want %q", got, "Pickpocket")
	}
}

func TestNcOpOutOfRangeReturnsEmpty(t *testing.T) {
	mc := newTestConfigs()
	state := runConfigOp(t, mc, OpNcOp, []int{0, 99})
	if got := state.PopString(); got != "" {
		t.Errorf("NC_OP(0, 99 OOB): got %q, want \"\"", got)
	}
}

func TestNcOpZeroIndexReturnsEmpty(t *testing.T) {
	mc := newTestConfigs()
	// op=0 is out of 1-based range → empty string.
	state := runConfigOp(t, mc, OpNcOp, []int{0, 0})
	if got := state.PopString(); got != "" {
		t.Errorf("NC_OP(0, 0): got %q, want \"\"", got)
	}
}

func TestNcOpNilOpSliceReturnsEmpty(t *testing.T) {
	mc := newTestConfigs()
	// NPC id 1 has no Op slice.
	state := runConfigOp(t, mc, OpNcOp, []int{1, 1})
	if got := state.PopString(); got != "" {
		t.Errorf("NC_OP(1, 1 nil Op): got %q, want \"\"", got)
	}
}

// TestHandleNcOpNullRejected pins NC_OP: TS wraps op with NumberNotNull
// (NpcConfigOps.ts:43). An op value of -1 (the null sentinel) must return an
// error rather than silently returning an empty string.
func TestHandleNcOpNullRejected(t *testing.T) {
	mc := newTestConfigs()
	runConfigOpExpectErr(t, mc, OpNcOp, []int{0, -1}, "NC_OP: input number was null(-1)")
}

func TestNcSize(t *testing.T) {
	mc := newTestConfigs()
	state := runConfigOp(t, mc, OpNcSize, []int{0})
	if got := state.PopInt(); got != 1 {
		t.Errorf("NC_SIZE(0): got %d, want 1", got)
	}
}

func TestNcVisLevel(t *testing.T) {
	mc := newTestConfigs()
	state := runConfigOp(t, mc, OpNcVisLevel, []int{0})
	if got := state.PopInt(); got != 2 {
		t.Errorf("NC_VISLEVEL(0): got %d, want 2", got)
	}
}

// TestNcUnknownIdErrors pins canonical "<TAG>: no NpcType with value (999)
// found" rejection at every Configs-side NC_* handler wired with checkNpcType
// that pops an explicit npcID. NPC_PARAM (active-npc form) reads typeID from
// state.ActiveNpc and is covered separately by TestNpcParamUnknownNpcIdErrors.
//
// 2-arg rows (NC_PARAM, NC_OP) push [npcID=999, secondArg=1]; handlers pop
// the second arg first then the npcID, so checkNpcType sees 999.
func TestNcUnknownIdErrors(t *testing.T) {
	mc := newTestConfigs()
	cases := []struct {
		op     Opcode
		inputs []int
		tag    string
	}{
		{OpNcName, []int{999}, "NC_NAME"},
		{OpNcParam, []int{999, 1}, "NC_PARAM"},
		{OpNcCategory, []int{999}, "NC_CATEGORY"},
		{OpNcDesc, []int{999}, "NC_DESC"},
		{OpNcDebugName, []int{999}, "NC_DEBUGNAME"},
		{OpNcOp, []int{999, 1}, "NC_OP"},
		{OpNcSize, []int{999}, "NC_SIZE"},
		{OpNcVisLevel, []int{999}, "NC_VISLEVEL"},
	}
	for _, c := range cases {
		t.Run(c.tag, func(t *testing.T) {
			runConfigOpExpectErr(t, mc, c.op, c.inputs,
				c.tag+": no NpcType with value (999) found")
		})
	}
}

// -- ObjConfigOps tests --

func TestOcName(t *testing.T) {
	mc := newTestConfigs()
	state := runConfigOp(t, mc, OpOcName, []int{995})
	if got := state.PopString(); got != "Coins" {
		t.Errorf("OC_NAME(995): got %q, want %q", got, "Coins")
	}
}

// TestOcUnknownIdErrors pins canonical "<TAG>: no ObjType with value (999)
// found" rejection at every Configs-side OC_* handler wired with checkObjType.
// OC_PARAM's unknown-PARAM path (vs unknown-OBJ path pinned here) is covered
// separately by TestOcParamUnknownParamErrors.
//
// OC_PARAM pushes [objID=999, paramID=1]; handler pops paramID first then
// objID, so checkObjType sees 999 before the (unreached) paramLookup.
func TestOcUnknownIdErrors(t *testing.T) {
	mc := newTestConfigs()
	cases := []struct {
		op     Opcode
		inputs []int
		tag    string
	}{
		{OpOcName, []int{999}, "OC_NAME"},
		{OpOcParam, []int{999, 1}, "OC_PARAM"},
		{OpOcCategory, []int{999}, "OC_CATEGORY"},
		{OpOcDesc, []int{999}, "OC_DESC"},
		{OpOcMembers, []int{999}, "OC_MEMBERS"},
		{OpOcWeight, []int{999}, "OC_WEIGHT"},
		{OpOcWearPos, []int{999}, "OC_WEARPOS"},
		{OpOcWearPos2, []int{999}, "OC_WEARPOS2"},
		{OpOcWearPos3, []int{999}, "OC_WEARPOS3"},
		{OpOcCost, []int{999}, "OC_COST"},
		{OpOcTradeable, []int{999}, "OC_TRADEABLE"},
		{OpOcDebugName, []int{999}, "OC_DEBUGNAME"},
		{OpOcCert, []int{999}, "OC_CERT"},
		{OpOcUncert, []int{999}, "OC_UNCERT"},
		{OpOcStackable, []int{999}, "OC_STACKABLE"},
	}
	for _, c := range cases {
		t.Run(c.tag, func(t *testing.T) {
			runConfigOpExpectErr(t, mc, c.op, c.inputs,
				c.tag+": no ObjType with value (999) found")
		})
	}
}

func TestOcParamInt(t *testing.T) {
	mc := newTestConfigs()
	state := runConfigOp(t, mc, OpOcParam, []int{995, 1})
	if got := state.PopInt(); got != 42 {
		t.Errorf("OC_PARAM(995,1): got %d, want 42", got)
	}
}

func TestOcParamString(t *testing.T) {
	mc := newTestConfigs()
	state := runConfigOp(t, mc, OpOcParam, []int{1, 2})
	if got := state.PopString(); got != "magical" {
		t.Errorf("OC_PARAM(1,2): got %q, want %q", got, "magical")
	}
}

func TestOcParamMissingKeyIntFallback(t *testing.T) {
	mc := newTestConfigs()
	// Obj 995 has no param 4; ParamType 4 default = 77.
	state := runConfigOp(t, mc, OpOcParam, []int{995, 4})
	if got := state.PopInt(); got != 77 {
		t.Errorf("OC_PARAM(995,4 missing): got %d, want default 77", got)
	}
}

func TestOcParamMissingKeyStringFallback(t *testing.T) {
	mc := newTestConfigs()
	// Obj 995 has no param 3; ParamType 3 default = "fallback".
	state := runConfigOp(t, mc, OpOcParam, []int{995, 3})
	if got := state.PopString(); got != "fallback" {
		t.Errorf("OC_PARAM(995,3 missing): got %q, want default %q", got, "fallback")
	}
}

// TestOcParamMissingKeyStringFallback_NullLiteral pins h-obj-3 / h-config-4:
// TS ParamHelper.getStringParam (ParamHelper.ts:10-16) returns
// `defaultValue ?? 'null'` — when the ParamType's defaultString is unset
// (TS field default is `null`), TS pushes the literal string "null".
// goscape stores DefaultString as a Go `string` (zero-value ""), so an unset
// defaultString surfaces as "" — the absent-string-default fallback now
// pushes "null" to match TS instead of pushing "" (the bug).
// Obj 995 (Coins) has no param 2; ParamType 2 has DefaultString="" (unset).
func TestOcParamMissingKeyStringFallback_NullLiteral(t *testing.T) {
	mc := newTestConfigs()
	state := runConfigOp(t, mc, OpOcParam, []int{995, 2})
	if got := state.PopString(); got != "null" {
		t.Errorf("OC_PARAM(995,2 missing, empty DefaultString): got %q, want %q (TS ParamHelper.ts:13 defaultValue ?? 'null')", got, "null")
	}
}

func TestOcParamUnknownParamErrors(t *testing.T) {
	mc := newTestConfigs()
	runConfigOpExpectErr(t, mc, OpOcParam, []int{995, 999}, "OC_PARAM: no ParamType with value (999) found")
}

// TestParamLookupUnknownParamErrors pins canonical "<TAG>: no ParamType with
// value (999) found" rejection from the shared paramLookup gate at every
// handler that delegates to it with a popped paramID. Inputs push
// [validTypeID=0, unknownParamID=999]; the handler pops paramID first, then
// validates the type id, then calls paramLookup which hits checkParamType.
//
// NPC_PARAM (active-npc form) reads typeID from state.ActiveNpc and needs a
// separate fixture; OC_PARAM is pinned by TestOcParamUnknownParamErrors above.
func TestParamLookupUnknownParamErrors(t *testing.T) {
	mc := newTestConfigs()
	cases := []struct {
		op     Opcode
		inputs []int
		tag    string
	}{
		{OpNcParam, []int{0, 999}, "NC_PARAM"},
		{OpLcParam, []int{0, 999}, "LC_PARAM"},
		{OpStructParam, []int{0, 999}, "STRUCT_PARAM"},
	}
	for _, c := range cases {
		t.Run(c.tag, func(t *testing.T) {
			runConfigOpExpectErr(t, mc, c.op, c.inputs,
				c.tag+": no ParamType with value (999) found")
		})
	}
}

// TestOcParamInt_NegativeSignPreserved pins NAI-122 in-scope-stretch fix:
// negative-int32 param values stored as their uint32 bit pattern (e.g.
// 0xFFFFFFFC = int32(-4)) must pop as the signed int32 value, not as
// the unsigned reading. Surfaced at NAI-122 smoke handoff: bronze-weapon
// crush/slash/stab bonuses showed as 4294967292 / 4294967294 (= -4 / -2
// as int32) in the equipment UI because paramLookup's `int(iv)`
// conversion did not sign-extend.
func TestOcParamInt_NegativeSignPreserved(t *testing.T) {
	mc := newTestConfigs()
	// Coins (995) param 5 holds the bit pattern for int32(-4).
	state := runConfigOp(t, mc, OpOcParam, []int{995, 5})
	if got := state.PopInt(); got != -4 {
		t.Errorf("OC_PARAM(995, 5): got %d, want -4 (uint32 0xFFFFFFFC must sign-extend through int32)", got)
	}
}

// TestOcParam_TypeMismatchFallsThroughToDefault pins h-config-3.
// TS ParamHelper.getStringParam / getIntParam (ParamHelper.ts:10-24)
// return the default when the stored value is not the expected JS type
// (`typeof value !== 'string'` / `!== 'number'`) — they do NOT throw.
// goscape's paramLookup pre-fix returned a hard error on type mismatch,
// aborting the script. The fix is to fall through to the default branch
// (same path as a missing key).
func TestOcParam_TypeMismatchFallsThroughToDefault(t *testing.T) {
	t.Run("string-typed param with int value pushes default 'null'", func(t *testing.T) {
		mc := newTestConfigs()
		// ParamType 2 is STRING (DefaultString="" → pushes "null" per
		// h-config-4). Stomp coins.Params[2] with a uint32 value to
		// force the type-mismatch branch.
		mc.objs[995].Params[2] = uint32(42)
		state := runConfigOp(t, mc, OpOcParam, []int{995, 2})
		if got := state.PopString(); got != "null" {
			t.Errorf("OC_PARAM(995, 2) with string-typed param holding uint32: got %q, want %q (TS ParamHelper.getStringParam returns default on type mismatch, ParamHelper.ts:10-16)", got, "null")
		}
	})
	t.Run("int-typed param with string value pushes default 77", func(t *testing.T) {
		mc := newTestConfigs()
		// ParamType 4 is INT (DefaultInt=77). Stomp coins.Params[4] with
		// a string value to force the type-mismatch branch.
		mc.objs[995].Params[4] = "not a number"
		state := runConfigOp(t, mc, OpOcParam, []int{995, 4})
		if got := state.PopInt(); got != 77 {
			t.Errorf("OC_PARAM(995, 4) with int-typed param holding string: got %d, want 77 (TS ParamHelper.getIntParam returns default on type mismatch, ParamHelper.ts:18-24)", got)
		}
	})
}

func TestOcCategory(t *testing.T) {
	mc := newTestConfigs()
	state := runConfigOp(t, mc, OpOcCategory, []int{995})
	if got := state.PopInt(); got != 7 {
		t.Errorf("OC_CATEGORY(995): got %d, want 7", got)
	}
}

func TestOcDesc(t *testing.T) {
	mc := newTestConfigs()
	state := runConfigOp(t, mc, OpOcDesc, []int{995})
	if got := state.PopString(); got != "Lovely money!" {
		t.Errorf("OC_DESC(995): got %q, want %q", got, "Lovely money!")
	}
}

func TestOcMembersFalse(t *testing.T) {
	mc := newTestConfigs()
	state := runConfigOp(t, mc, OpOcMembers, []int{995})
	if got := state.PopInt(); got != 0 {
		t.Errorf("OC_MEMBERS(995): got %d, want 0", got)
	}
}

func TestOcMembersTrue(t *testing.T) {
	mc := newTestConfigs()
	state := runConfigOp(t, mc, OpOcMembers, []int{1})
	if got := state.PopInt(); got != 1 {
		t.Errorf("OC_MEMBERS(1): got %d, want 1", got)
	}
}

func TestOcWeight(t *testing.T) {
	mc := newTestConfigs()
	state := runConfigOp(t, mc, OpOcWeight, []int{1})
	if got := state.PopInt(); got != 100 {
		t.Errorf("OC_WEIGHT(1): got %d, want 100", got)
	}
}

func TestOcWearPos(t *testing.T) {
	mc := newTestConfigs()
	state := runConfigOp(t, mc, OpOcWearPos, []int{1})
	if got := state.PopInt(); got != 0 {
		t.Errorf("OC_WEARPOS(1): got %d, want 0", got)
	}
}

func TestOcWearPos2(t *testing.T) {
	mc := newTestConfigs()
	state := runConfigOp(t, mc, OpOcWearPos2, []int{1})
	if got := state.PopInt(); got != 8 {
		t.Errorf("OC_WEARPOS2(1): got %d, want 8", got)
	}
}

func TestOcWearPos3(t *testing.T) {
	mc := newTestConfigs()
	state := runConfigOp(t, mc, OpOcWearPos3, []int{1})
	if got := state.PopInt(); got != 11 {
		t.Errorf("OC_WEARPOS3(1): got %d, want 11", got)
	}
}

func TestOcCost(t *testing.T) {
	mc := newTestConfigs()
	state := runConfigOp(t, mc, OpOcCost, []int{995})
	if got := state.PopInt(); got != 1 {
		t.Errorf("OC_COST(995): got %d, want 1", got)
	}
}

func TestOcTradeableTrue(t *testing.T) {
	mc := newTestConfigs()
	state := runConfigOp(t, mc, OpOcTradeable, []int{995})
	if got := state.PopInt(); got != 1 {
		t.Errorf("OC_TRADEABLE(995): got %d, want 1", got)
	}
}

func TestOcTradeableFalse(t *testing.T) {
	mc := newTestConfigs()
	state := runConfigOp(t, mc, OpOcTradeable, []int{1})
	if got := state.PopInt(); got != 0 {
		t.Errorf("OC_TRADEABLE(1): got %d, want 0", got)
	}
}

func TestOcDebugName(t *testing.T) {
	mc := newTestConfigs()
	state := runConfigOp(t, mc, OpOcDebugName, []int{995})
	if got := state.PopString(); got != "coins" {
		t.Errorf("OC_DEBUGNAME(995): got %q, want %q", got, "coins")
	}
}

func TestOcCertSwapsBaseToNote(t *testing.T) {
	mc := newTestConfigs()
	// Obj 10 is base (certtemplate=-1, certlink=11) → OC_CERT returns 11.
	state := runConfigOp(t, mc, OpOcCert, []int{10})
	if got := state.PopInt(); got != 11 {
		t.Errorf("OC_CERT(10 base): got %d, want 11", got)
	}
}

func TestOcCertPassthroughForNote(t *testing.T) {
	mc := newTestConfigs()
	// Obj 11 is already a note (certtemplate=799, not -1) → returns its own id.
	state := runConfigOp(t, mc, OpOcCert, []int{11})
	if got := state.PopInt(); got != 11 {
		t.Errorf("OC_CERT(11 note): got %d, want 11", got)
	}
}

func TestOcCertPassthroughNoLink(t *testing.T) {
	mc := newTestConfigs()
	// Obj 995 has CertLink = -1 (default) → returns 995.
	state := runConfigOp(t, mc, OpOcCert, []int{995})
	if got := state.PopInt(); got != 995 {
		t.Errorf("OC_CERT(995 no link): got %d, want 995", got)
	}
}

func TestOcUncertSwapsNoteToBase(t *testing.T) {
	mc := newTestConfigs()
	// Obj 11 is note (certtemplate=799>=0, certlink=10>=0) → returns 10.
	state := runConfigOp(t, mc, OpOcUncert, []int{11})
	if got := state.PopInt(); got != 10 {
		t.Errorf("OC_UNCERT(11 note): got %d, want 10", got)
	}
}

func TestOcUncertPassthroughForBase(t *testing.T) {
	mc := newTestConfigs()
	// Obj 10 is base (certtemplate=-1) → returns its own id.
	state := runConfigOp(t, mc, OpOcUncert, []int{10})
	if got := state.PopInt(); got != 10 {
		t.Errorf("OC_UNCERT(10 base): got %d, want 10", got)
	}
}

func TestOcStackableTrue(t *testing.T) {
	mc := newTestConfigs()
	state := runConfigOp(t, mc, OpOcStackable, []int{995})
	if got := state.PopInt(); got != 1 {
		t.Errorf("OC_STACKABLE(995): got %d, want 1", got)
	}
}

func TestOcStackableFalse(t *testing.T) {
	mc := newTestConfigs()
	state := runConfigOp(t, mc, OpOcStackable, []int{1})
	if got := state.PopInt(); got != 0 {
		t.Errorf("OC_STACKABLE(1): got %d, want 0", got)
	}
}

// -- Missing Configs tests --

func TestHandlerWithoutConfigsErrors(t *testing.T) {
	// Configs = nil → handlers must return an error rather than panic.
	sf := &ScriptFile{
		Name:             "test_nil_configs",
		Opcodes:          []Opcode{OpOcName, OpReturn},
		IntOperands:      []int32{0, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	state := Init(sf, nil, false, nil, nil)
	state.PushInt(995)
	err := Execute(state)
	if err == nil {
		t.Fatal("OC_NAME with nil Configs: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "Configs not set") {
		t.Errorf("OC_NAME nil Configs: got %q, want containing %q", err.Error(), "Configs not set")
	}
}
