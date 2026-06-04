package world

import (
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

// handleIfButton handles client opcode 39 (IF_BUTTON).
// Body: u16 component-id.
//
// Gates per TS IfButtonHandler.ts (9aadcec4):
//   - Component must be registered
//   - Component must be IsComponentVisible to the player
//
// Note: the buttonType != NO_BUTTON check was present in e1dea19f but
// removed at 9aadcec4 — any registered, visible component passes now.
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
	if com == nil || !p.IsComponentVisible(com) {
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
	s.runScript(sf, p, nil, script.TriggerIfButton, protect, nil, nil)
	return nil
}

// handleIfPlayerDesign handles client opcode 8 (IF_PLAYERDESIGN).
// Body: u8 gender | u8[7] idkit (255 → -1) | u8[5] color.
//
// Validates allowDesign, gender ≤ 1, idk disable+type (via IdkType registry),
// and color ranges. On pass: updates p.gender/body/colors and calls
// SetAppearanceInv with the Worn inv id to flag MaskAppearance.
// Mirrors TS IfPlayerDesignHandler.ts:8-57 (9aadcec4).
func (s *Server) handleIfPlayerDesign(p *Player, payload []byte) error {
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

	// IdkType validation — mirrors TS IdkSaveDesignHandler.ts:18-35.
	// TS calls IdkType.get(idkit[i]) which returns falsy when the
	// registry has no entry for that id — the `!idk` arm then drops
	// the design. net-client-h-social-5: goscape pre-fix wrapped the
	// loop in `if s.idkTypes != nil`, silently accepting any design
	// when the registry hadn't been populated (the TS-unsafe inverse
	// of "always validate"). Threading through a nil-safe configs
	// slice keeps the nil-registry rejection equivalent to TS's
	// IdkType.get → undefined → !idk.
	var configs []*objtype.IdkType
	if s.idkTypes != nil {
		configs = s.idkTypes.Configs
	}
	for i := range 7 {
		typ := i + gender*7
		if typ == 8 && idkit[i] == -1 { // female jaw exception (TS L25-28)
			continue
		}
		if idkit[i] < 0 || idkit[i] >= len(configs) {
			return nil
		}
		idk := configs[idkit[i]]
		if idk.Disable || idk.Type != typ {
			return nil
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
	// TS IfPlayerDesignHandler.ts:56: buildAppearance(InvType.WORN).
	// Use s.invTypes.Worn when populated; fall back to p.appearanceInv
	// (already set to Worn at login) when invTypes is nil (test paths).
	wornId := p.appearanceInv
	if s.invTypes != nil {
		wornId = s.invTypes.Worn
	}
	p.SetAppearanceInv(wornId)
	return nil
}

// handleTutorialClickSide handles client opcode 233 (TUTORIAL_CLICKSIDE).
// Body: u8 sidebar tab index. Fires [tutorial] if tab is in [0,13].
// Mirrors TS TutorialClickSideHandler.ts (9aadcec4).
func (s *Server) handleTutorialClickSide(p *Player, payload []byte) error {
	if len(payload) < 1 {
		return nil
	}
	tab := int(payload[0])
	if tab < 0 || tab > 13 { // tab < 0 unreachable (byte→int ≥ 0); preserved from TS source
		return nil
	}
	sf := s.scriptProvider.GetByTriggerSpecific(script.TriggerTutorial, -1, -1)
	s.runScript(sf, p, nil, script.TriggerTutorial, true, nil, nil)
	return nil
}
