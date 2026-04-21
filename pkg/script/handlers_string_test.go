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
