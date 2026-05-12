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

// -- SPLIT_* dialog pagination handlers (NAI-75 light-fidelity port;
// NAI-179 wired font.Split + MesanimType resolution).

// handleSplitInit ports TS SPLIT_INIT (StringOps.ts:76-96). Pops
// (text, maxWidth, linesPerPage, fontId), parses any leading <p,name>
// mesanim prefix (resolving NAME to a MesanimType id via
// Configs.MesanimByName), splits the prefix-stripped text into lines
// via FontType.Split (width-aware word-wrap), and chunks the lines
// into pages of linesPerPage each.
//
// On invalid fontId (Configs.FontType returns nil — TS FontTypeValid
// would throw) the handler logs slog.Warn and falls back to the
// NAI-75 light-fidelity '|'-only split. Goscape defensive per
// defensive_gate_doc_comment_label.md.
func handleSplitInit(s *ScriptState) error {
	fontId := s.PopInt()
	linesPerPage := s.PopInt()
	maxWidth := s.PopInt()
	text := s.PopString()

	s.SplitMesanim = -1
	if strings.HasPrefix(text, "<p,") {
		if end := strings.IndexByte(text, '>'); end != -1 {
			name := text[3:end]
			s.SplitMesanim = int32(s.Configs.MesanimByName(name))
			text = text[end+1:]
		}
	}

	var lines []string
	if font := s.Configs.FontType(fontId); font != nil {
		lines = font.Split(text, maxWidth)
	} else {
		slog.Warn("SPLIT_INIT: invalid fontId; falling back to '|' split",
			"script", s.Script.Name, "fontId", fontId)
		lines = strings.Split(text, "|")
	}

	if linesPerPage < 1 {
		// Defensive: TS would divide-by-zero on splice(0, 0). Goscape
		// defensive (TS throws).
		s.SplitPages = [][]string{lines}
		return nil
	}
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

// handleSplitGet ports TS SPLIT_GET (StringOps.ts:98-102). Pops
// (page, line); pushes s.SplitPages[page][line]. Out-of-bounds pushes
// empty string (goscape defensive; TS throws — labelled per
// defensive_gate_doc_comment_label.md).
func handleSplitGet(s *ScriptState) error {
	line := s.PopInt()
	page := s.PopInt()
	if page < 0 || page >= len(s.SplitPages) {
		s.PushString("")
		slog.Debug("SPLIT_GET out of page range",
			"script", s.Script.Name, "page", page, "pages", len(s.SplitPages))
		return nil
	}
	pg := s.SplitPages[page]
	if line < 0 || line >= len(pg) {
		s.PushString("")
		slog.Debug("SPLIT_GET out of line range",
			"script", s.Script.Name, "page", page, "line", line, "lines", len(pg))
		return nil
	}
	s.PushString(pg[line])
	return nil
}

// handleSplitGetAnim ports TS SPLIT_GETANIM (StringOps.ts:114-122).
// Pops page; pushes MesanimType.Len[lineCount-1] where lineCount =
// len(SplitPages[page]). When SplitMesanim is negative (no prefix),
// MesanimType lookup is nil, or any index is out-of-range, pushes -1
// (TS MesanimValid would throw; goscape defensive per
// defensive_gate_doc_comment_label.md).
func handleSplitGetAnim(s *ScriptState) error {
	page := s.PopInt()
	if s.SplitMesanim < 0 {
		s.PushInt(-1)
		return nil
	}
	typ := s.Configs.MesanimType(int(s.SplitMesanim))
	if typ == nil {
		s.PushInt(-1)
		return nil
	}
	if page < 0 || page >= len(s.SplitPages) {
		s.PushInt(-1)
		return nil
	}
	idx := len(s.SplitPages[page]) - 1
	if idx < 0 || idx >= len(typ.Len) {
		s.PushInt(-1)
		return nil
	}
	s.PushInt(typ.Len[idx])
	return nil
}

// handleSplitLineCount ports TS SPLIT_LINECOUNT (StringOps.ts:108-112).
// Pops page; pushes len(s.SplitPages[page]). Out-of-bounds pushes 0
// (goscape defensive; TS throws — labelled per
// defensive_gate_doc_comment_label.md).
func handleSplitLineCount(s *ScriptState) error {
	page := s.PopInt()
	if page < 0 || page >= len(s.SplitPages) {
		s.PushInt(0)
		slog.Debug("SPLIT_LINECOUNT out of page range",
			"script", s.Script.Name, "page", page, "pages", len(s.SplitPages))
		return nil
	}
	s.PushInt(len(s.SplitPages[page]))
	return nil
}

// handleSplitPageCount ports TS SPLIT_PAGECOUNT (StringOps.ts:104-106).
// Pushes len(s.SplitPages). Returns 0 before any SPLIT_INIT call
// (Go zero-value: SplitPages is nil, len(nil) == 0).
func handleSplitPageCount(s *ScriptState) error {
	s.PushInt(len(s.SplitPages))
	return nil
}
