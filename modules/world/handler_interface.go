package world

import "github.com/zsrv/goscape/pkg/script"

// designBodyColorCount holds the number of valid color values per body-part
// slot. Mirrors the lengths of TS Player.DESIGN_BODY_COLORS
// (Engine-TS/src/engine/entity/Player.ts:102-108).
var designBodyColorCount = [5]int{12, 16, 16, 6, 8}

// handleIfButton handles client opcode 155 (IF_BUTTON).
// Body: u16 component-id.
//
// Sets lastCom, then:
//   - If comId is in resumeButtons and activeScript is in PauseButton state →
//     resumes the suspended script (mirrors TS IfButtonHandler.ts:20-23).
//   - Otherwise → looks up [if_button,<comId>] and runs it with protect=true.
//
// DEVIATION NAI-45-D1: buttonType and isComponentVisible checks skipped —
// no component registry (same cluster as S6m-D2, S6o-D1, NAI-40-D-COMPONENT-
// REGISTRY-VALIDATION-SKIPPED). Closure: component-registry sub-spec.
//
// DEVIATION NAI-45-D2: protect=true always; TS uses root.overlay==false
// which requires the component registry. Closure: component-registry sub-spec.
func (s *Server) handleIfButton(p *Player, payload []byte) error {
	if len(payload) < 2 {
		return nil
	}
	comId := int(uint16(payload[0])<<8 | uint16(payload[1]))
	p.lastCom = comId

	for _, b := range p.resumeButtons {
		if b == comId {
			if p.activeScript != nil && p.activeScript.Execution == script.PauseButton {
				p.activeScript.Execution = script.Running
				s.resumeOrFinish(p.activeScript, p)
			}
			return nil
		}
	}

	sf := s.scriptProvider.GetByTriggerSpecific(script.TriggerIfButton, comId, -1)
	s.runScript(sf, p, nil, true, nil, nil)
	return nil
}

// handleIdkSaveDesign handles client opcode 52 (IDK_SAVEDESIGN).
// Body: u8 gender | u8[7] idkit (255 → -1) | u8[5] color.
//
// Validates allowDesign, gender ≤ 1, idk disable+type (via IdkType registry),
// and color ranges. On pass: updates p.gender/body/colors and calls
// SetAppearanceInv to flag MaskAppearance.
// Mirrors TS IdkSaveDesignHandler.ts:7-38.
func (s *Server) handleIdkSaveDesign(p *Player, payload []byte) error {
	if len(payload) < 13 {
		return nil
	}
	if !p.allowDesign {
		return nil
	}

	gender := int(payload[0])
	if gender > 1 {
		return nil
	}

	var idkit [7]int
	for i := range 7 {
		v := int(payload[1+i])
		if v == 255 {
			v = -1
		}
		idkit[i] = v
	}

	// IdkType validation — mirrors TS IdkSaveDesignHandler.ts:18-33.
	// TS order: idk loop before color loop.
	if s.idkTypes != nil {
		for i := range 7 {
			typ := i + gender*7
			if typ == 8 && idkit[i] == -1 { // female jaw exception (TS L21-23)
				continue
			}
			if idkit[i] < 0 || idkit[i] >= len(s.idkTypes.Configs) {
				return nil
			}
			idk := s.idkTypes.Configs[idkit[i]]
			if idk.Disable || idk.Type != typ {
				return nil
			}
		}
	}

	var color [5]int
	for i := range 5 {
		color[i] = int(payload[8+i])
	}

	for i, c := range color {
		if c >= designBodyColorCount[i] {
			return nil
		}
	}

	p.gender = gender
	p.body = idkit
	p.colors = color
	p.SetAppearanceInv(p.appearanceInv)
	return nil
}

// handleTutClickSide handles client opcode 175 (TUT_CLICKSIDE).
// Body: u8 sidebar tab index. Fires [tutorial] if tab is in [0,13].
// Mirrors TS TutClickSideHandler.ts.
func (s *Server) handleTutClickSide(p *Player, payload []byte) error {
	if len(payload) < 1 {
		return nil
	}
	tab := int(payload[0])
	if tab < 0 || tab > 13 { // tab < 0 unreachable (byte→int ≥ 0); preserved from TS source
		return nil
	}
	sf := s.scriptProvider.GetByTriggerSpecific(script.TriggerTutorial, -1, -1)
	s.runScript(sf, p, nil, true, nil, nil)
	return nil
}
