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

// handleCamShake reads (axis, random, amplitude, rate) from the int stack
// and dispatches to ActivePlayer.CamShake. Args were pushed left-to-right
// at the script call site (engine.rs2:120 `cam_shake(int $axis, int $random,
// int $amplitude, int $rate)`); goscape's PopInt returns them in reverse.
// Mirrors TS PlayerOps.ts:220-224.
func handleCamShake(s *ScriptState) error {
	if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
		return errors.New("CAM_SHAKE: no active player")
	}
	rate := s.PopInt()
	amplitude := s.PopInt()
	random := s.PopInt()
	axis := s.PopInt()
	s.Self.CamShake(axis, random, amplitude, rate)
	return nil
}

// handleCamMoveTo reads (coord, height, rate, rate2) from the int stack,
// validates coord via checkCoord (mirrors TS CoordValid at
// ScriptValidators.ts:109), and dispatches to ActivePlayer.CamMoveTo
// with the unpacked (x, z). Args were pushed left-to-right; PopInt
// reverses, so we pop rate2, rate, height, coord. Mirrors TS
// PlayerOps.ts:213-218.
func handleCamMoveTo(s *ScriptState) error {
	if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
		return errors.New("CAM_MOVETO: no active player")
	}
	rate2 := s.PopInt()
	rate := s.PopInt()
	height := s.PopInt()
	coord := s.PopInt()
	_, x, z, err := checkCoord(coord, "CAM_MOVETO")
	if err != nil {
		return err
	}
	s.Self.CamMoveTo(x, z, height, rate, rate2)
	return nil
}

// handleCamLookAt is identical to handleCamMoveTo except it dispatches
// to CamLookAt (kind=1). Mirrors TS PlayerOps.ts:206-211.
func handleCamLookAt(s *ScriptState) error {
	if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
		return errors.New("CAM_LOOKAT: no active player")
	}
	rate2 := s.PopInt()
	rate := s.PopInt()
	height := s.PopInt()
	coord := s.PopInt()
	_, x, z, err := checkCoord(coord, "CAM_LOOKAT")
	if err != nil {
		return err
	}
	s.Self.CamLookAt(x, z, height, rate, rate2)
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
