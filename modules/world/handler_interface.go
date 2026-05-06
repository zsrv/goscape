package world

import (
	"log/slog"

	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/script"
)

// designBodyColorCount holds the number of valid color values per body-part
// slot. Mirrors the lengths of TS Player.DESIGN_BODY_COLORS
// (Engine-TS/src/engine/entity/Player.ts:102-108).
var designBodyColorCount = [5]int{12, 16, 16, 6, 8}

// lookupComponent returns the registered component for id, or nil if
// the registry is unloaded or the id is out of range. Mirrors TS
// Component.get (Component.ts:252-254) which reads sparse-array slots
// returning undefined on miss.
func (s *Server) lookupComponent(id int) *objtype.ComponentType {
	if s.componentTypes == nil || id < 0 || id >= len(s.componentTypes.Configs) {
		return nil
	}
	return s.componentTypes.Configs[id]
}

// handleIfButton handles client opcode 155 (IF_BUTTON).
// Body: u16 component-id.
//
// Gates per TS IfButtonHandler.ts:14-22:
//   - Component must be registered AND have buttonType != NO_BUTTON
//   - Component must be IsComponentVisible to the player
//
// On pass, sets lastCom and either resumes a PauseButton-suspended script
// or fires [if_button,<comId>]. The trigger fires with protect = !root.Overlay
// (root = rootLayer's component).
func (s *Server) handleIfButton(p *Player, payload []byte) error {
	if len(payload) < 2 {
		return nil
	}
	comId := int(uint16(payload[0])<<8 | uint16(payload[1]))

	com := s.lookupComponent(comId)
	if com == nil || com.ButtonType == objtype.ButtonNone {
		return nil
	}
	if !p.IsComponentVisible(com) {
		return nil
	}

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

	if s.scriptProvider == nil {
		return nil
	}
	sf := s.scriptProvider.GetByTriggerSpecific(script.TriggerIfButton, comId, -1)
	root := s.lookupComponent(com.RootLayer)
	protect := root == nil || !root.Overlay
	s.runScript(sf, p, nil, protect, nil, nil)
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
	slog.Info("NAI-112 Bundle1.5 instr: TUT_CLICKSIDE entry", "tab", tab)
	if tab < 0 || tab > 13 { // tab < 0 unreachable (byte→int ≥ 0); preserved from TS source
		return nil
	}
	sf := s.scriptProvider.GetByTriggerSpecific(script.TriggerTutorial, -1, -1)
	slog.Info("NAI-112 Bundle1.5 instr: TUT_CLICKSIDE lookup", "tab", tab, "scriptFound", sf != nil)
	s.runScript(sf, p, nil, true, nil, nil)
	slog.Info("NAI-112 Bundle1.5 instr: TUT_CLICKSIDE postScript", "tab", tab)
	return nil
}
