package world

import (
	"github.com/zsrv/goscape/pkg/io/packet"
	"github.com/zsrv/goscape/pkg/script"
)

// handleResumePauseButton handles client opcode 235 (RESUME_PAUSEBUTTON).
// Body: u16 component-id of the clicked button. If the button matches
// one of the player's pre-stored resumeButtons and the activeScript is
// in the PauseButton state, execution resumes.
func (s *Server) handleResumePauseButton(p *Player, buf *packet.Packet) error {
	com := int(buf.G2())
	p.lastCom = com

	if p.activeScript == nil || p.activeScript.Execution != script.PauseButton {
		return nil
	}
	matched := false
	for _, b := range p.resumeButtons {
		if b == com {
			matched = true
			break
		}
	}
	if !matched {
		return nil
	}

	p.activeScript.Execution = script.Running
	s.resumeOrFinish(p.activeScript, p)
	return nil
}

// handleResumeCountDialog handles client opcode 237 (RESUME_P_COUNTDIALOG).
// Body: i32 count (signed). The count is pushed onto the active script's
// int stack so the next opcode can pop it, then execution resumes.
func (s *Server) handleResumeCountDialog(p *Player, buf *packet.Packet) error {
	count := int32(buf.G4())

	if p.activeScript == nil || p.activeScript.Execution != script.CountDialog {
		return nil
	}

	p.activeScript.PushInt(int(count))
	p.activeScript.Execution = script.Running
	s.resumeOrFinish(p.activeScript, p)
	return nil
}
