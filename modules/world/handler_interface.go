package world

import "github.com/zsrv/goscape/pkg/script"

// designBodyColorCount holds the number of valid color values per body-part
// slot. Mirrors the lengths of TS Player.DESIGN_BODY_COLORS
// (Engine-TS/src/engine/entity/Player.ts:102-108).
var designBodyColorCount = [5]int{12, 16, 16, 6, 8}

// handleTutClickSide handles client opcode 175 (TUT_CLICKSIDE).
// Body: u8 sidebar tab index. Fires [tutorial] if tab is in [0,13].
// Mirrors TS TutClickSideHandler.ts.
func (s *Server) handleTutClickSide(p *Player, payload []byte) error {
	if len(payload) < 1 {
		return nil
	}
	tab := int(payload[0])
	if tab < 0 || tab > 13 {
		return nil
	}
	sf := s.scriptProvider.GetByTriggerSpecific(script.TriggerTutorial, -1, -1)
	s.runScript(sf, p, nil, true, nil, nil)
	return nil
}
