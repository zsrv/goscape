package script

import (
	"log/slog"
	"strconv"
	"strings"
)

func handleAppend(s *ScriptState) error {
	suffix := s.PopString()
	base := s.PopString()
	s.PushString(base + suffix)
	return nil
}

func handleAppendNum(s *ScriptState) error {
	n := s.PopInt()
	base := s.PopString()
	s.PushString(base + strconv.Itoa(n))
	return nil
}

func handleAppendChar(s *ScriptState) error {
	ch := s.PopInt()
	base := s.PopString()
	s.PushString(base + string(rune(ch)))
	return nil
}

func handleAppendSignNum(s *ScriptState) error {
	n := s.PopInt()
	base := s.PopString()
	// TS appendsignnum: negative keeps the leading '-' that strconv
	// produces; positive values have no '+' prefix; zero prints as "0".
	s.PushString(base + strconv.Itoa(n))
	return nil
}

func handleLowercase(s *ScriptState) error {
	s.PushString(strings.ToLower(s.PopString()))
	return nil
}

func handleCompare(s *ScriptState) error {
	rhs := s.PopString()
	lhs := s.PopString()
	s.PushInt(strings.Compare(lhs, rhs))
	return nil
}

func handleStringLength(s *ScriptState) error {
	s.PushInt(len(s.PopString()))
	return nil
}

func handleSubstring(s *ScriptState) error {
	end := s.PopInt()
	start := s.PopInt()
	src := s.PopString()
	if start < 0 {
		start = 0
	}
	if end > len(src) {
		end = len(src)
	}
	if start > end {
		start = end
	}
	s.PushString(src[start:end])
	return nil
}

func handleStringIndexOfChar(s *ScriptState) error {
	ch := s.PopInt()
	src := s.PopString()
	s.PushInt(strings.IndexRune(src, rune(ch)))
	return nil
}

func handleStringIndexOfString(s *ScriptState) error {
	needle := s.PopString()
	haystack := s.PopString()
	s.PushInt(strings.Index(haystack, needle))
	return nil
}

// handleTextSwitch is a stub until string-switch tables land in the
// file decoder. TS branches based on a string key; for S5a we pop the
// key and fall through.
func handleTextSwitch(s *ScriptState) error {
	_ = s.PopString()
	slog.Debug("TEXT_SWITCH not implemented; falling through",
		"script", s.Script.Name, "pc", s.PC)
	return nil
}

// -- SPLIT_* stubs (dialog pagination — deferred to later sub-spec) --

func handleSplitInit(s *ScriptState) error {
	// TS pops (text, fontId, maxWidth); we don't keep them.
	_ = s.PopInt()
	_ = s.PopInt()
	_ = s.PopString()
	slog.Debug("SPLIT_INIT stub invoked", "script", s.Script.Name)
	return nil
}

func handleSplitGet(s *ScriptState) error {
	// TS pops (page, line).
	_ = s.PopInt()
	_ = s.PopInt()
	s.PushString("")
	return nil
}

func handleSplitGetAnim(s *ScriptState) error {
	_ = s.PopInt()
	_ = s.PopInt()
	s.PushInt(-1)
	return nil
}

func handleSplitLineCount(s *ScriptState) error {
	_ = s.PopInt()
	s.PushInt(0)
	return nil
}

func handleSplitPageCount(s *ScriptState) error {
	s.PushInt(0)
	return nil
}
