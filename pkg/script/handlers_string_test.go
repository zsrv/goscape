package script

import (
	"testing"
)

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
	_, got := runStringOp(t, OpSubstring, []int{1, 4}, []string{"hello"})
	if got != "ell" {
		t.Errorf("Substring: got %q, want %q", got, "ell")
	}
}

func TestStringIndexOfChar(t *testing.T) {
	got, _ := runStringOp(t, OpStringIndexOfChar, []int{'l'}, []string{"hello"})
	if got != 2 {
		t.Errorf("IndexOfChar: got %d, want 2", got)
	}
}

func TestStringIndexOfString(t *testing.T) {
	got, _ := runStringOp(t, OpStringIndexOfString, nil, []string{"hello world", "world"})
	if got != 6 {
		t.Errorf("IndexOfString: got %d, want 6", got)
	}
}

func TestSplitStubsReturnZeroes(t *testing.T) {
	got, _ := runStringOp(t, OpSplitLineCount, []int{0}, nil)
	if got != 0 {
		t.Errorf("SplitLineCount stub: got %d, want 0", got)
	}
	got, _ = runStringOp(t, OpSplitPageCount, nil, nil)
	if got != 0 {
		t.Errorf("SplitPageCount stub: got %d, want 0", got)
	}
}

// runSplitInit pushes (text, maxWidth, linesPerPage, fontId) and runs
// SPLIT_INIT against a fresh state, then returns the state for assertion.
func runSplitInit(t *testing.T, text string, maxWidth, linesPerPage, fontId int) *ScriptState {
	t.Helper()
	sf := &ScriptFile{
		Name:             "test_split_init",
		Opcodes:          []Opcode{OpSplitInit, OpReturn},
		IntOperands:      []int32{0, 0},
		StringOperands:   []string{"", ""},
		InstructionCount: 2,
	}
	state := Init(sf, nil, false, nil, nil)
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

func TestSplitInitMesanimPrefixStripped(t *testing.T) {
	s := runSplitInit(t, "<p,neutral>Greetings|stranger", 380, 4, 8)
	// NAI-75-D-MESANIM-NOT-PORTED: prefix parsed but id lookup deferred,
	// so SplitMesanim stays -1 even when a prefix is present.
	if s.SplitMesanim != -1 {
		t.Errorf("SplitMesanim: got %d, want -1 (NAI-75-D-MESANIM-NOT-PORTED pin)", s.SplitMesanim)
	}
	if len(s.SplitPages) != 1 {
		t.Fatalf("len(SplitPages): got %d, want 1", len(s.SplitPages))
	}
	// Prefix stripped: text is "Greetings|stranger" → 2 lines.
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
