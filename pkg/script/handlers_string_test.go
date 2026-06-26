package script

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zsrv/goscape/pkg/fonttype"
	"github.com/zsrv/goscape/pkg/objtype"
)

// -- Config-registry validator unit tests --

func TestCheckMesanimType(t *testing.T) {
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
			setup:   func() *mockConfigs { return &mockConfigs{mesanims: map[int]*objtype.MesanimType{0: {}}} },
			wantErr: false,
		},
		{
			name:      "unknown id",
			id:        100,
			setup:     func() *mockConfigs { return &mockConfigs{mesanims: map[int]*objtype.MesanimType{}} },
			wantErr:   true,
			wantSubst: "OP: no MesanimType with value (100) found",
		},
		{
			name:      "negative id",
			id:        -1,
			setup:     func() *mockConfigs { return &mockConfigs{mesanims: map[int]*objtype.MesanimType{}} },
			wantErr:   true,
			wantSubst: "OP: no MesanimType with value (-1) found",
		},
		{
			name:      "nil Configs",
			id:        0,
			setup:     func() *mockConfigs { return nil },
			wantErr:   true,
			wantSubst: "OP: no MesanimType with value (0) found",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := &ScriptState{}
			if cfg := tc.setup(); cfg != nil {
				s.Configs = cfg
			}
			err := checkMesanimType(s, tc.id, "OP")
			if tc.wantErr {
				if err == nil {
					t.Fatalf("checkMesanimType(%d): want error, got nil", tc.id)
				}
				if !strings.Contains(err.Error(), tc.wantSubst) {
					t.Errorf("error message: got %q, want contains %q", err.Error(), tc.wantSubst)
				}
			} else {
				if err != nil {
					t.Fatalf("checkMesanimType(%d): want nil, got %v", tc.id, err)
				}
			}
		})
	}
}

func TestCheckFontType(t *testing.T) {
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
			setup:   func() *mockConfigs { return &mockConfigs{fonts: map[int]*fonttype.FontType{0: {}}} },
			wantErr: false,
		},
		{
			name:      "unknown id",
			id:        100,
			setup:     func() *mockConfigs { return &mockConfigs{fonts: map[int]*fonttype.FontType{}} },
			wantErr:   true,
			wantSubst: "OP: no FontType with value (100) found",
		},
		{
			name:      "negative id",
			id:        -1,
			setup:     func() *mockConfigs { return &mockConfigs{fonts: map[int]*fonttype.FontType{}} },
			wantErr:   true,
			wantSubst: "OP: no FontType with value (-1) found",
		},
		{
			name:      "nil Configs",
			id:        0,
			setup:     func() *mockConfigs { return nil },
			wantErr:   true,
			wantSubst: "OP: no FontType with value (0) found",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := &ScriptState{}
			if cfg := tc.setup(); cfg != nil {
				s.Configs = cfg
			}
			err := checkFontType(s, tc.id, "OP")
			if tc.wantErr {
				if err == nil {
					t.Fatalf("checkFontType(%d): want error, got nil", tc.id)
				}
				if !strings.Contains(err.Error(), tc.wantSubst) {
					t.Errorf("error message: got %q, want contains %q", err.Error(), tc.wantSubst)
				}
			} else {
				if err != nil {
					t.Fatalf("checkFontType(%d): want nil, got %v", tc.id, err)
				}
			}
		})
	}
}

// runStringOp builds a one-instruction script that runs `op`, pushing
// string inputs first (bottom of string stack) then int inputs (bottom
// of int stack). Returns the top-of-int and top-of-string after execute.
func runStringOp(t *testing.T, op Opcode, intInputs []int, stringInputs []string) (topInt int, topStr string) {
	t.Helper()
	sf := &ScriptFile{
		Name:             "test_str_" + op.String(),
		Opcodes:          []Opcode{op, OpReturn},
		IntOperands:      []int32{0, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	state := Init(sf, nil, false, nil, nil)
	for _, v := range stringInputs {
		state.PushString(v)
	}
	for _, v := range intInputs {
		state.PushInt(v)
	}
	if err := Execute(state); err != nil {
		t.Fatalf("%s: unexpected error: %v", op.String(), err)
	}
	return state.PopInt(), state.PopString()
}

func TestStringAppend(t *testing.T) {
	_, got := runStringOp(t, OpAppend, nil, []string{"foo", "bar"})
	if got != "foobar" {
		t.Errorf("Append: got %q, want %q", got, "foobar")
	}
}

func TestStringAppendNum(t *testing.T) {
	_, got := runStringOp(t, OpAppendNum, []int{42}, []string{"n="})
	if got != "n=42" {
		t.Errorf("AppendNum: got %q, want %q", got, "n=42")
	}
}

func TestStringAppendChar(t *testing.T) {
	_, got := runStringOp(t, OpAppendChar, []int{'!'}, []string{"hi"})
	if got != "hi!" {
		t.Errorf("AppendChar: got %q, want %q", got, "hi!")
	}
}

// TestStringAppendCharLatin1SingleByte pins L26: a char in 128-255 appends a
// single raw byte (TS String.fromCharCode → one code unit → one wire byte), not
// the 2-byte UTF-8 encoding string(rune(ch)) used to produce. é == 0xE9.
func TestStringAppendCharLatin1SingleByte(t *testing.T) {
	_, got := runStringOp(t, OpAppendChar, []int{0xE9}, []string{""})
	if len(got) != 1 || got[0] != 0xE9 {
		t.Errorf("AppendChar(0xE9): got % x (len %d), want [e9] (len 1)", got, len(got))
	}
}

// TestStringAppendSignNum verifies APPEND_SIGNNUM mirrors TS
// StringOps.ts:18-27 — non-negative values get an explicit '+' prefix
// (including zero, since `num >= 0`), negative values render with the
// '-' that strconv.Itoa produces.
func TestStringAppendSignNum(t *testing.T) {
	cases := []struct {
		name string
		n    int
		base string
		want string
	}{
		{"positive", 42, "X", "X+42"},
		{"negative", -7, "X", "X-7"},
		{"zero", 0, "X", "X+0"},
		{"large_positive", 1000000, "X", "X+1000000"},
		{"large_negative", -1000000, "X", "X-1000000"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, got := runStringOp(t, OpAppendSignNum, []int{tc.n}, []string{tc.base})
			if got != tc.want {
				t.Errorf("AppendSignNum(%q, %d): got %q, want %q", tc.base, tc.n, got, tc.want)
			}
		})
	}
}

func TestStringLowercase(t *testing.T) {
	_, got := runStringOp(t, OpLowercase, nil, []string{"HeLLo"})
	if got != "hello" {
		t.Errorf("Lowercase: got %q, want %q", got, "hello")
	}
}

func TestStringCompare(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"a", "b", -1},
		{"b", "a", 1},
		{"foo", "foo", 0},
		// M19: magnitude (Java compareTo), not sign. 'a'(97)-'c'(99) = -2.
		{"a", "c", -2},
		{"c", "a", 2},
		// First differing char dominates regardless of later chars.
		{"az", "ca", 'a' - 'c'},
		// Equal prefix → length difference.
		{"ab", "a", 1},
		{"a", "abc", -2},
	}
	for _, tc := range cases {
		t.Run(tc.a+"_vs_"+tc.b, func(t *testing.T) {
			got, _ := runStringOp(t, OpCompare, nil, []string{tc.a, tc.b})
			if got != tc.want {
				t.Errorf("Compare(%q,%q): got %d, want %d", tc.a, tc.b, got, tc.want)
			}
		})
	}
}

func TestStringLength(t *testing.T) {
	got, _ := runStringOp(t, OpStringLength, nil, []string{"hello"})
	if got != 5 {
		t.Errorf("StringLength: got %d, want 5", got)
	}
}

func TestStringSubstring(t *testing.T) {
	// args are [start, end] (end pushed last → popped first by the handler).
	cases := []struct {
		name       string
		start, end int
		want       string
	}{
		{"basic", 1, 4, "ell"},
		// M20: JS substring swaps when start > end.
		{"start_gt_end_swaps", 4, 1, "ell"},
		// M20: negative end clamps to 0 (no slice panic), then swaps.
		{"negative_end_clamps_and_swaps", 3, -1, "hel"},
		{"negative_start_clamps", -2, 3, "hel"},
		{"end_past_len_clamps", 2, 99, "llo"},
		{"both_negative_empty", -5, -1, ""},
		{"full", 0, 5, "hello"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, got := runStringOp(t, OpSubstring, []int{tc.start, tc.end}, []string{"hello"})
			if got != tc.want {
				t.Errorf("Substring(%d,%d): got %q, want %q", tc.start, tc.end, got, tc.want)
			}
		})
	}
}

func TestStringIndexOfChar(t *testing.T) {
	got, _ := runStringOp(t, OpStringIndexOfChar, []int{'l'}, []string{"hello"})
	if got != 2 {
		t.Errorf("IndexOfChar: got %d, want 2", got)
	}
}

// TestStringIndexOfCharLatin1 pins L26: searching for a 128-255 char finds the
// raw byte by its unit index. The prior IndexRune hunted for the char's UTF-8
// encoding, which never appears in goscape's raw-byte strings, so a present
// Latin-1 char reported -1.
func TestStringIndexOfCharLatin1(t *testing.T) {
	src := string([]byte{'a', 0xE9, 'b'})
	got, _ := runStringOp(t, OpStringIndexOfChar, []int{0xE9}, []string{src})
	if got != 1 {
		t.Errorf("IndexOfChar(0xE9) in % x: got %d, want 1", src, got)
	}
}

// TestStringLengthRawBytes pins L26: STRING_LENGTH counts bytes, which equals
// TS's UTF-16 unit count because gjstr maps each wire byte to one code unit and
// goscape stores those bytes raw. Two Latin-1 bytes → length 2.
func TestStringLengthRawBytes(t *testing.T) {
	got, _ := runStringOp(t, OpStringLength, nil, []string{string([]byte{0xE9, 0xE9})})
	if got != 2 {
		t.Errorf("StringLength(2 Latin-1 bytes): got %d, want 2", got)
	}
}

func TestStringIndexOfString(t *testing.T) {
	got, _ := runStringOp(t, OpStringIndexOfString, nil, []string{"hello world", "world"})
	if got != 6 {
		t.Errorf("IndexOfString: got %d, want 6", got)
	}
}

func TestHandleTextSwitch(t *testing.T) {
	cases := []struct {
		name       string
		value      int
		s1, s2     string
		wantPushed string
	}{
		{"value=1 picks s1", 1, "apple", "banana", "apple"},
		{"value=0 picks s2", 0, "apple", "banana", "banana"},
		{"value=-1 picks s2", -1, "apple", "banana", "banana"},
		{"value=2 picks s2 (strict ===1)", 2, "apple", "banana", "banana"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st := newTestState(minimalScript(OpReturn))
			// Push order: s1 first, s2 second (so s2 is top), then value (top).
			st.PushString(tc.s1)
			st.PushString(tc.s2)
			st.PushInt(tc.value)
			if err := handleTextSwitch(st); err != nil {
				t.Fatalf("handleTextSwitch: unexpected error: %v", err)
			}
			got := st.PopString()
			if got != tc.wantPushed {
				t.Fatalf("pushed: got %q, want %q", got, tc.wantPushed)
			}
		})
	}
}

// runSplitInit pushes (text, maxWidth, linesPerPage, fontId) and runs
// SPLIT_INIT against a fresh state, then returns the state for assertion.
func runSplitInit(t *testing.T, text string, maxWidth, linesPerPage, fontId int) *ScriptState {
	t.Helper()
	return runSplitInitWithConfigs(t, newTestConfigs(), text, maxWidth, linesPerPage, fontId)
}

// runSplitInitWithConfigs is the same as runSplitInit but lets callers
// supply a pre-seeded mockConfigs (e.g. with a fake FontType or a
// mesanim debugname → id mapping).
func runSplitInitWithConfigs(t *testing.T, cfg *mockConfigs, text string, maxWidth, linesPerPage, fontId int) *ScriptState {
	t.Helper()
	sf := &ScriptFile{
		Name:             "test_split_init",
		Opcodes:          []Opcode{OpSplitInit, OpReturn},
		IntOperands:      []int32{0, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	state := Init(sf, nil, false, nil, nil)
	state.Configs = cfg
	state.PushString(text)
	state.PushInt(maxWidth)
	state.PushInt(linesPerPage)
	state.PushInt(fontId)
	if err := Execute(state); err != nil {
		t.Fatalf("SPLIT_INIT: unexpected error: %v", err)
	}
	return state
}

func TestSplitInitNoMesanimPrefix(t *testing.T) {
	s := runSplitInit(t, "first line|second line", 380, 4, 8)
	if s.SplitMesanim != -1 {
		t.Errorf("SplitMesanim: got %d, want -1 (no prefix)", s.SplitMesanim)
	}
	if len(s.SplitPages) != 1 {
		t.Fatalf("len(SplitPages): got %d, want 1", len(s.SplitPages))
	}
	if got, want := s.SplitPages[0], []string{"first line", "second line"}; !equalStrings(got, want) {
		t.Errorf("SplitPages[0]: got %v, want %v", got, want)
	}
}

func TestSplitInitMesanimPrefixStripped_UnknownNameStaysNegOne(t *testing.T) {
	// mockConfigs has no entry for "neutral" → MesanimByName returns -1.
	// (The previous NAI-179-retired deviation pinned -1 unconditionally;
	// now -1 only when the name is absent from the registry.)
	s := runSplitInit(t, "<p,neutral>Greetings|stranger", 380, 4, 8)
	if s.SplitMesanim != -1 {
		t.Errorf("SplitMesanim: got %d, want -1 (name not in registry)", s.SplitMesanim)
	}
	if len(s.SplitPages) != 1 {
		t.Fatalf("len(SplitPages): got %d, want 1", len(s.SplitPages))
	}
	// Prefix stripped: text is "Greetings|stranger" → mockConfigs.FontType
	// returns nil → defensive fallback uses strings.Split(text, "|") → 2 lines.
	if got, want := s.SplitPages[0], []string{"Greetings", "stranger"}; !equalStrings(got, want) {
		t.Errorf("SplitPages[0]: got %v, want %v (prefix should be stripped)", got, want)
	}
}

func TestSplitInitMultiPageChunking(t *testing.T) {
	// 5 lines, linesPerPage=4 → 2 pages: page 0 = 4 lines, page 1 = 1 line.
	s := runSplitInit(t, "a|b|c|d|e", 380, 4, 8)
	if len(s.SplitPages) != 2 {
		t.Fatalf("len(SplitPages): got %d, want 2", len(s.SplitPages))
	}
	if got, want := s.SplitPages[0], []string{"a", "b", "c", "d"}; !equalStrings(got, want) {
		t.Errorf("SplitPages[0]: got %v, want %v", got, want)
	}
	if got, want := s.SplitPages[1], []string{"e"}; !equalStrings(got, want) {
		t.Errorf("SplitPages[1]: got %v, want %v", got, want)
	}
}

func TestSplitInitReplacesNotAppends(t *testing.T) {
	// Multi-call SAME ScriptState: second SPLIT_INIT must replace SplitPages,
	// not append. Mirrors chatnpc's repeated calls within Welcome flow.
	sf := &ScriptFile{
		Name:             "test_split_init_replace",
		Opcodes:          []Opcode{OpSplitInit, OpSplitInit, OpReturn},
		IntOperands:      []int32{0, 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	state := Init(sf, nil, false, nil, nil)
	state.Configs = newTestConfigs()
	// Push order matters: stack is LIFO, both opcodes execute in instruction
	// order, so the FIRST opcode pops what was pushed LAST. Push the
	// SECOND call's args FIRST (deepest), then the FIRST call's args
	// (top of stack — popped by the first SPLIT_INIT instruction).
	//
	// Second SPLIT_INIT: 1 line.
	state.PushString("only")
	state.PushInt(380)
	state.PushInt(4)
	state.PushInt(8)
	// First SPLIT_INIT: 3 lines.
	state.PushString("x|y|z")
	state.PushInt(380)
	state.PushInt(4)
	state.PushInt(8)
	if err := Execute(state); err != nil {
		t.Fatalf("SPLIT_INIT chain: %v", err)
	}
	// After two SPLIT_INIT calls, SplitPages reflects the SECOND call's
	// result ("only"), proving replacement (not append) semantics.
	if len(state.SplitPages) != 1 {
		t.Fatalf("len(SplitPages) after second SPLIT_INIT: got %d, want 1", len(state.SplitPages))
	}
	if got, want := state.SplitPages[0], []string{"only"}; !equalStrings(got, want) {
		t.Errorf("SplitPages[0]: got %v, want %v (second call must replace, not append)", got, want)
	}
}

func TestSplitInitEmptyText(t *testing.T) {
	s := runSplitInit(t, "", 380, 4, 8)
	// Empty text → strings.Split("", "|") returns [""] → 1 page, 1 line.
	// This matches TS font.split("") which returns [""].
	if len(s.SplitPages) != 1 {
		t.Fatalf("len(SplitPages) for empty text: got %d, want 1", len(s.SplitPages))
	}
	if got, want := s.SplitPages[0], []string{""}; !equalStrings(got, want) {
		t.Errorf("SplitPages[0]: got %v, want %v", got, want)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// runSplitInitThen runs SPLIT_INIT then a single follow-up opcode in the
// same script, returning the state. Used by SPLIT_GET/PAGECOUNT/etc tests.
func runSplitInitThen(t *testing.T, initText string, linesPerPage int, follow Opcode, followInts []int) *ScriptState {
	t.Helper()
	ops := []Opcode{OpSplitInit, follow, OpReturn}
	sf := &ScriptFile{
		Name:             "test_split_init_then_" + follow.String(),
		Opcodes:          ops,
		IntOperands:      make([]int32, len(ops)),
		StringOperands:   make([]string, len(ops)),
		InstructionCount: uint32(len(ops)),
	}
	state := Init(sf, nil, false, nil, nil)
	state.Configs = newTestConfigs()
	// Push in reverse execution order: follow-up args first (deepest in
	// stack), then SPLIT_INIT args on top (popped first by the first opcode).
	// Follow-up opcode args (e.g. page index for SPLIT_GET).
	for _, v := range followInts {
		state.PushInt(v)
	}
	// SPLIT_INIT args: text, maxWidth, linesPerPage, fontId (fontId on top).
	state.PushString(initText)
	state.PushInt(380)
	state.PushInt(linesPerPage)
	state.PushInt(8)
	if err := Execute(state); err != nil {
		t.Fatalf("SPLIT_INIT+%s: unexpected error: %v", follow.String(), err)
	}
	return state
}

func TestSplitPageCountAfterInit(t *testing.T) {
	// 5 lines, linesPerPage=4 → 2 pages.
	s := runSplitInitThen(t, "a|b|c|d|e", 4, OpSplitPageCount, nil)
	got := s.PopInt()
	if got != 2 {
		t.Errorf("SPLIT_PAGECOUNT: got %d, want 2", got)
	}
}

func TestSplitPageCountBeforeInit(t *testing.T) {
	// No SPLIT_INIT call — SplitPages is nil; len(nil) = 0.
	sf := &ScriptFile{
		Name:             "test_split_pagecount_uninit",
		Opcodes:          []Opcode{OpSplitPageCount, OpReturn},
		IntOperands:      []int32{0, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	state := Init(sf, nil, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("SPLIT_PAGECOUNT uninit: %v", err)
	}
	if got := state.PopInt(); got != 0 {
		t.Errorf("SPLIT_PAGECOUNT (no prior SPLIT_INIT): got %d, want 0", got)
	}
}

func TestSplitLineCountAfterInit(t *testing.T) {
	// 5 lines, linesPerPage=4 → page 0 has 4 lines, page 1 has 1.
	s := runSplitInitThen(t, "a|b|c|d|e", 4, OpSplitLineCount, []int{0})
	if got := s.PopInt(); got != 4 {
		t.Errorf("SPLIT_LINECOUNT(0): got %d, want 4", got)
	}
	s = runSplitInitThen(t, "a|b|c|d|e", 4, OpSplitLineCount, []int{1})
	if got := s.PopInt(); got != 1 {
		t.Errorf("SPLIT_LINECOUNT(1): got %d, want 1", got)
	}
}

func TestSplitLineCountOutOfBounds(t *testing.T) {
	// Defensive: TS would throw; goscape pushes 0 and logs debug.
	s := runSplitInitThen(t, "a|b", 4, OpSplitLineCount, []int{99})
	if got := s.PopInt(); got != 0 {
		t.Errorf("SPLIT_LINECOUNT(99) on 1-page state: got %d, want 0 (defensive)", got)
	}
}

func TestSplitGetAfterInit(t *testing.T) {
	s := runSplitInitThen(t, "first|second|third", 4, OpSplitGet, []int{0, 1})
	if got := s.PopString(); got != "second" {
		t.Errorf("SPLIT_GET(0,1): got %q, want %q", got, "second")
	}
	s = runSplitInitThen(t, "first|second|third", 4, OpSplitGet, []int{0, 0})
	if got := s.PopString(); got != "first" {
		t.Errorf("SPLIT_GET(0,0): got %q, want %q", got, "first")
	}
}

func TestSplitGetOutOfBounds(t *testing.T) {
	// Defensive: TS would throw on undefined access; goscape pushes "".
	s := runSplitInitThen(t, "a", 4, OpSplitGet, []int{99, 99})
	if got := s.PopString(); got != "" {
		t.Errorf("SPLIT_GET(99,99): got %q, want \"\" (defensive)", got)
	}
}

func TestSplitGetAnim_ResolvesLen(t *testing.T) {
	// Seed: MesanimType id 3 with Len[0]=10, Len[1]=20.
	cfg := newTestConfigs()
	cfg.mesanims = map[int]*objtype.MesanimType{
		3: {Len: [4]int{10, 20, 30, 40}},
	}
	cfg.mesanimsByName = map[string]int{"shopkeeper": 3}
	// SPLIT_INIT writes SplitMesanim=3 and SplitPages with 2 lines on page 0.
	s := runSplitInitThenWithConfigs(t, cfg, "<p,shopkeeper>hello|world", 4, OpSplitGetAnim, []int{0})
	got := s.PopInt()
	// Page 0 has 2 lines → Len[lineCount-1] = Len[1] = 20.
	if got != 20 {
		t.Errorf("SPLIT_GETANIM(0): got %d, want 20 (Len[1])", got)
	}
}

func TestSplitGetAnim_NoMesanimReturnsNegOne(t *testing.T) {
	s := runSplitInitThen(t, "no prefix here", 4, OpSplitGetAnim, []int{0})
	got := s.PopInt()
	if got != -1 {
		t.Errorf("SPLIT_GETANIM with SplitMesanim=-1: got %d, want -1", got)
	}
}

func TestSplitGetAnim_NilConfigTypeReturnsNegOne(t *testing.T) {
	// SplitMesanim is set to a non-negative id, but mockConfigs.MesanimType
	// returns nil for it → defensive -1 (TS MesanimValid would throw).
	cfg := newTestConfigs()
	cfg.mesanimsByName = map[string]int{"ghost": 42}
	// mesanims map is empty → MesanimType(42) returns nil.
	s := runSplitInitThenWithConfigs(t, cfg, "<p,ghost>hello", 4, OpSplitGetAnim, []int{0})
	got := s.PopInt()
	if got != -1 {
		t.Errorf("SPLIT_GETANIM with nil MesanimType: got %d, want -1", got)
	}
}

func runSplitInitThenWithConfigs(t *testing.T, cfg *mockConfigs, initText string, linesPerPage int, follow Opcode, followInts []int) *ScriptState {
	t.Helper()
	ops := []Opcode{OpSplitInit, follow, OpReturn}
	sf := &ScriptFile{
		Name:             "test_split_init_then_" + follow.String(),
		Opcodes:          ops,
		IntOperands:      make([]int32, len(ops)),
		StringOperands:   make([]string, len(ops)),
		InstructionCount: uint32(len(ops)),
	}
	state := Init(sf, nil, false, nil, nil)
	state.Configs = cfg
	for _, v := range followInts {
		state.PushInt(v)
	}
	state.PushString(initText)
	state.PushInt(380)
	state.PushInt(linesPerPage)
	state.PushInt(8)
	if err := Execute(state); err != nil {
		t.Fatalf("SPLIT_INIT+%s: unexpected error: %v", follow.String(), err)
	}
	return state
}

func TestSplitInitMesanimPrefixResolves(t *testing.T) {
	// Seed mockConfigs with a fake MesanimType named "neutral" at id 7.
	cfg := newTestConfigs()
	cfg.mesanimsByName = map[string]int{"neutral": 7}
	cfg.mesanims = map[int]*objtype.MesanimType{
		7: {Len: [4]int{10, 20, 30, 40}},
	}
	s := runSplitInitWithConfigs(t, cfg, "<p,neutral>hi|there", 380, 4, 8)
	if s.SplitMesanim != 7 {
		t.Errorf("SplitMesanim: got %d, want 7 (resolved id)", s.SplitMesanim)
	}
	// Text stripped: "hi|there"; fontType nil → '|' fallback path.
	if len(s.SplitPages) != 1 {
		t.Fatalf("len(SplitPages): got %d, want 1", len(s.SplitPages))
	}
	if got, want := s.SplitPages[0], []string{"hi", "there"}; !equalStrings(got, want) {
		t.Errorf("SplitPages[0]: got %v, want %v", got, want)
	}
}

// ref245FontPackDir resolves the rev-245.2 reference cache pack dir from
// GOSCAPE_REF245_DIR, or "" when unset/unavailable (see the cross-revision
// cache note on the fonttype tests).
func ref245FontPackDir() string {
	if ref := os.Getenv("GOSCAPE_REF245_DIR"); ref != "" {
		dir := filepath.Join(ref, "data", "pack")
		if _, err := os.Stat(filepath.Join(dir, "client", "title")); err == nil {
			return dir
		}
	}
	return ""
}

func TestSplitInitFontWrap_BreaksOnMaxWidth(t *testing.T) {
	packDir := ref245FontPackDir()
	if packDir == "" {
		t.Skip("Server245.2-ref cache not available; set GOSCAPE_REF245_DIR")
	}
	fonts, err := fonttype.Load(packDir)
	if err != nil {
		t.Fatalf("fonttype.Load: %v", err)
	}
	cfg := newTestConfigs()
	cfg.fonts = map[int]*fonttype.FontType{0: fonts[0]}
	// Long ASCII sentence, no '|', tight maxWidth → forces ≥ 1 wrap.
	text := "alpha bravo charlie delta echo foxtrot golf hotel india juliet"
	maxWidth := fonts[0].StringWidth(text) / 3
	s := runSplitInitWithConfigs(t, cfg, text, maxWidth, 100, 0)
	if len(s.SplitPages) != 1 {
		t.Fatalf("len(SplitPages): got %d, want 1", len(s.SplitPages))
	}
	if len(s.SplitPages[0]) < 2 {
		t.Errorf("font.Split should have produced ≥2 lines with maxWidth=%d; got %v",
			maxWidth, s.SplitPages[0])
	}
}

func TestSplitInitInvalidFontFallsBackToPipeSplit(t *testing.T) {
	cfg := newTestConfigs()
	// cfg.fonts is nil → mockConfigs.FontType returns nil for any id.
	s := runSplitInitWithConfigs(t, cfg, "a|b", 380, 1, 999)
	// Defensive fallback splits on '|'; linesPerPage=1 → 2 pages of 1 line.
	if len(s.SplitPages) != 2 {
		t.Fatalf("len(SplitPages): got %d, want 2", len(s.SplitPages))
	}
	if got, want := s.SplitPages[0], []string{"a"}; !equalStrings(got, want) {
		t.Errorf("SplitPages[0]: got %v, want %v", got, want)
	}
	if got, want := s.SplitPages[1], []string{"b"}; !equalStrings(got, want) {
		t.Errorf("SplitPages[1]: got %v, want %v", got, want)
	}
}
