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

// handleSplitInit ports TS SPLIT_INIT (StringOps.ts:76-96). Pops
// (text, maxWidth, linesPerPage, fontId), parses any leading <p,name>
// mesanim prefix, splits the prefix-stripped text on '|' (the explicit
// line-break char used in chatnpc strings), and chunks the lines into
// pages of linesPerPage lines each. Stores results in s.SplitPages +
// s.SplitMesanim.
//
// NAI-75-D-FONT-WRAP-NAIVE: maxWidth + fontId are popped but unused
// (no font-aware word-wrap; relies on '|' breaks). Closure: future
// FontType cache loader sub-spec calls font.split(text, maxWidth) here.
//
// NAI-75-D-MESANIM-NOT-PORTED: <p,name> prefix is parsed and stripped
// but SplitMesanim is left at -1 (no MesanimType.getId lookup yet).
// Closure: future MesanimType cache loader sub-spec resolves the id.
func handleSplitInit(s *ScriptState) error {
	// Pop order matches TS popInts(3) semantics: top of stack is fontId.
	_ = s.PopInt() // fontId — unused per NAI-75-D-FONT-WRAP-NAIVE
	linesPerPage := s.PopInt()
	_ = s.PopInt() // maxWidth — unused per NAI-75-D-FONT-WRAP-NAIVE
	text := s.PopString()

	s.SplitMesanim = -1
	if strings.HasPrefix(text, "<p,") {
		if end := strings.IndexByte(text, '>'); end != -1 {
			// Prefix recognised; light-fidelity skips MesanimType lookup.
			// SplitMesanim stays -1 per NAI-75-D-MESANIM-NOT-PORTED.
			text = text[end+1:]
		}
	}

	if linesPerPage < 1 {
		// Defensive: TS would divide-by-zero on splice(0, 0); we no-op
		// to avoid an infinite chunking loop. Goscape defensive (TS
		// throws); labelled per defensive_gate_doc_comment_label.md.
		s.SplitPages = [][]string{{text}}
		return nil
	}

	lines := strings.Split(text, "|")
	pages := make([][]string, 0, (len(lines)+linesPerPage-1)/linesPerPage)
	for i := 0; i < len(lines); i += linesPerPage {
		end := i + linesPerPage
		if end > len(lines) {
			end = len(lines)
		}
		pages = append(pages, lines[i:end])
	}
	s.SplitPages = pages
	slog.Debug("SPLIT_INIT processed",
		"script", s.Script.Name, "pages", len(pages), "mesanim", s.SplitMesanim)
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
