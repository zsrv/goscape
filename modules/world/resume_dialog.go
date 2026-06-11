package world

import (
	"github.com/zsrv/goscape/pkg/io/packet"
	"github.com/zsrv/goscape/pkg/script"
)

// handleResumePauseButton handles client opcode 11 (RESUME_PAUSEBUTTON).
// Body: 2 bytes (component-id echoed by Java client) — IGNORED per TS
// ResumePauseButtonHandler.ts:7-14 (the TS decoder reads no body and the
// handler never inspects payload, lastCom, or resumeButtons). Resumes
// the active script if it is PauseButton-suspended; no-ops otherwise.
//
// A9 re-verified @2e3bcf43 — the handler is UNCHANGED at the 254 pin
// (still no resumeButtons validation):
//
//	handle(_message: ResumePauseButton, player: Player): boolean {
//	    if (!player.activeScript || player.activeScript.execution !== ScriptState.PAUSEBUTTON) {
//	        return false;
//	    }
//	    player.executeScript(player.activeScript, true, true);
//	    return true;
//	}
//
// resumeButtons validation lives ONLY in IfButtonHandler (the IF_BUTTON
// membership branch, handler_interface.go handleIfButton).
//
// Why this matters: the standard chatnpc proc (chat.rs2:303-311) never
// calls if_setresumebuttons, so resumeButtons is empty when "Click here
// to continue" fires. A resumeButtons match-gate would deadlock chat
// dialogs — confirmed at NAI-75 smoke. Match TS exactly.
func (s *Server) handleResumePauseButton(p *Player, buf *packet.Packet) error {
	_ = buf.G2() // consume the 2-byte com echo; payload value is ignored

	if p.activeScript == nil || p.activeScript.Execution != script.PauseButton {
		return nil
	}
	p.activeScript.Execution = script.Running
	s.resumeOrFinish(p.activeScript, p)
	return nil
}

// handleResumeCountDialog handles client opcode 190 (RESUME_P_COUNTDIALOG).
// Body: i32 count (signed). The count is stored as state.LastInt so the
// next LAST_INT opcode can read it, then execution resumes (S5m: matches
// TS semantics where RESUME_P_COUNTDIALOG writes state.lastInt).
func (s *Server) handleResumeCountDialog(p *Player, buf *packet.Packet) error {
	count := int32(buf.G4())

	if p.activeScript == nil || p.activeScript.Execution != script.CountDialog {
		return nil
	}

	p.activeScript.LastInt = int(count)
	p.activeScript.Execution = script.Running
	s.resumeOrFinish(p.activeScript, p)
	return nil
}
