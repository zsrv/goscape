package script

import "errors"

// handlePPauseButton suspends the script until the client sends a
// RESUME_PAUSEBUTTON packet whose button id is in the active player's
// resumeButtons array. The tick / client handler sets Execution=Running
// and re-enters Execute.
func handlePPauseButton(s *ScriptState) error {
	if err := requireProtectedActivePlayer(s, "P_PAUSEBUTTON"); err != nil {
		return err
	}
	s.Execution = PauseButton
	return nil
}

// handlePCountDialog writes the P_COUNTDIALOG wire packet and suspends
// the script until the client sends a RESUME_P_COUNTDIALOG packet.
func handlePCountDialog(s *ScriptState) error {
	if err := requireProtectedActivePlayer(s, "P_COUNTDIALOG"); err != nil {
		return err
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

// handleLastInt pushes ScriptState.LastInt — the int injected by a
// resume event (RESUME_P_COUNTDIALOG passes the count via this field).
func handleLastInt(s *ScriptState) error {
	s.PushInt(s.LastInt)
	return nil
}

// handleLastItem / Slot / UseItem / UseSlot / TargetSlot push fields
// captured from recent OPHELD / OPUSE / INV_BUTTOND client packets.
// TS gates these behind a trigger-type whitelist; S5m skips the gate
// and always returns the stored value.

func handleLastItem(s *ScriptState) error {
	if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
		return errors.New("LAST_ITEM: no active player")
	}
	s.PushInt(s.Self.LastItem())
	return nil
}

func handleLastSlot(s *ScriptState) error {
	if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
		return errors.New("LAST_SLOT: no active player")
	}
	s.PushInt(s.Self.LastSlot())
	return nil
}

func handleLastUseItem(s *ScriptState) error {
	if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
		return errors.New("LAST_USEITEM: no active player")
	}
	s.PushInt(s.Self.LastUseItem())
	return nil
}

func handleLastUseSlot(s *ScriptState) error {
	if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
		return errors.New("LAST_USESLOT: no active player")
	}
	s.PushInt(s.Self.LastUseSlot())
	return nil
}

func handleLastTargetSlot(s *ScriptState) error {
	if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
		return errors.New("LAST_TARGETSLOT: no active player")
	}
	s.PushInt(s.Self.LastTargetSlot())
	return nil
}

// handleCamReset sends a CAM_RESET wire packet via the active player.
// Takes no args. Used by the LOGIN script and teleport-spell scripts
// to restore the default camera after cutscene-style manipulations.
func handleCamReset(s *ScriptState) error {
	if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
		return errors.New("CAM_RESET: no active player")
	}
	s.Self.CamReset()
	return nil
}

// handleStaffModLevel pushes the active player's staff moderation
// level (0 for regular players, >0 for mods/admins). Used by update_all
// and other login procs that branch on mod status.
func handleStaffModLevel(s *ScriptState) error {
	if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
		return errors.New("STAFFMODLEVEL: no active player")
	}
	s.PushInt(int(s.Self.StaffModLevel()))
	return nil
}

// handleUID pushes the active player's persistent account uid (from
// login RPC). Used by update_all and other procs that branch on
// per-account state. Matches TS: state.pushInt(state.activePlayer.uid).
func handleUID(s *ScriptState) error {
	if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
		return errors.New("UID: no active player")
	}
	s.PushInt(s.Self.UID())
	return nil
}
