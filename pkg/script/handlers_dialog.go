package script

import "errors"

// handlePPauseButton suspends the script until the client sends a
// RESUME_PAUSEBUTTON packet whose button id is in the active player's
// resumeButtons array. The tick / client handler sets Execution=Running
// and re-enters Execute.
func handlePPauseButton(s *ScriptState) error {
	if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
		return errors.New("P_PAUSEBUTTON: no active player")
	}
	s.Execution = PauseButton
	return nil
}

// handlePCountDialog writes the P_COUNTDIALOG wire packet and suspends
// the script until the client sends a RESUME_P_COUNTDIALOG packet.
func handlePCountDialog(s *ScriptState) error {
	if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
		return errors.New("P_COUNTDIALOG: no active player")
	}
	s.Self.SendCountDialog()
	s.Execution = CountDialog
	return nil
}

// handleLastCom pushes the active player's lastCom field — the
// component id most recently clicked.
func handleLastCom(s *ScriptState) error {
	if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
		return errors.New("LAST_COM: no active player")
	}
	s.PushInt(s.Self.LastCom())
	return nil
}
