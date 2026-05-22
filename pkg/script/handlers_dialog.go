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
//
// DEFERRED: TS gates each of these behind a per-opcode trigger-type
// whitelist (PlayerOps.ts:259-340 for LAST_ITEM/SLOT/USEITEM/USESLOT,
// PlayerOps.ts:1026-1033 for LAST_TARGETSLOT). When the active script's
// state.trigger is NOT in the opcode's allowedTriggers slice, TS throws
// "is not safe to use in this trigger". Goscape currently always returns
// the stored value because ScriptState has no `Trigger ServerTriggerType`
// field — only Queue.Trigger exists (queue.go:56) for deferred-script
// dispatch, not for in-flight execution context.
//
// Allowlists per TS (verbatim):
//   LAST_ITEM:        OPHELD1..5, OPHELDU, OPHELDT,
//                     INV_BUTTON1..5
//   LAST_SLOT:        OPHELD1..5, OPHELDU, OPHELDT,
//                     INV_BUTTON1..5, INV_BUTTOND
//   LAST_USEITEM:     OPHELDU,
//                     APOBJU, APLOCU, APNPCU, APPLAYERU,
//                     OPOBJU, OPLOCU, OPNPCU, OPPLAYERU
//   LAST_USESLOT:     OPHELDU,
//                     APOBJU, APLOCU, APNPCU, APPLAYERU,
//                     OPOBJU, OPLOCU, OPNPCU, OPPLAYERU
//   LAST_TARGETSLOT:  INV_BUTTOND
//
// To honor these gates, the following plumbing is required:
//   1. Add `Trigger ServerTriggerType` field to ScriptState (zero value
//      TriggerProc=0 is correctly NOT in any LAST_* allowlist, so the
//      default-safe semantics fall through to "throw").
//   2. Thread the trigger through the three goscape script-construction
//      surfaces in modules/world (each currently lacks a trigger param):
//        a. (*Server).runScript and runScriptFn (script.go:97, server.go:273)
//        b. (*Server).buildPlayerScriptState (script.go:43)
//        c. (*Server).buildNpcScriptState (npc_script.go:329)
//      Update ~25 call sites:
//        - handler_opheld.go:106  → TriggerOpHeld1 + op-1
//        - handler_opheld.go:215  → TriggerOpHeldT
//        - handler_opheld.go:398  → TriggerOpHeldU
//        - handler_inv_button.go:65   → TriggerInvButton1 + op-1
//        - handler_inv_button.go:128  → TriggerInvButtonD
//        - handler_interface.go:66,147     → IF_BUTTON (TriggerProc-safe)
//        - handlers_game.go:535       → TriggerDebugProc
//        - interaction.go:365         → walktrigger (TriggerProc-safe)
//        - interaction_trigger.go:89  → fireOpTriggerNpc (`trigger`)
//        - interaction_trigger.go:176 → fireApTriggerNpc (`apTrigger`)
//        - interaction_trigger.go:368 → fireApTriggerNpc (`trigger`)
//        - interaction_trigger.go:465 → fireApTriggerLoc (`trigger`)
//                                       (and fireOpTriggerLoc precedes it)
//        - interaction_trigger.go:714 → fireOpTriggerObj (`trigger`)
//        - interaction_trigger.go:785 → fireApTriggerObj (`trigger`)
//        - player_interaction_trigger.go:83  → fireOpTriggerPlayer (`trigger`)
//        - player_interaction_trigger.go:140 → fireApTriggerPlayer (`trigger`)
//        - npc_script.go:336  → AI script (`trigger` from req.Trigger or AiTimer)
//        - npc_script.go:560  → TriggerAiTimer
//        - player_script.go:1052 → TriggerProc (login proc)
//        - tick.go:275 → TriggerProc (engine queue dispatch)
//        - tick.go:489,538 → queued script trigger (TriggerQueue1..20 from
//                            Queue.Trigger; queue.go:56)
//        - tick.go:589 → t.Type-derived (TriggerSoftTimer/TriggerNormalTimer)
//   3. Mirror Init signature change in test fixtures: every &ScriptState{}
//      literal that currently exercises LAST_* must add `Trigger: ...`
//      explicitly — though TriggerProc=0 is the zero value, so tests of
//      OTHER opcodes need no edit (Go semantics #91).
//   4. Add 5 guards in this file:
//        if !slices.Contains(allowedLastItem, s.Trigger) {
//          return errors.New("LAST_ITEM: not safe to use in this trigger")
//        }
//      Define the 5 allowlists as package-level `var` slices alongside
//      the handlers (or inline) — prefer package-level for test reuse.
//
// Scope estimate: M-L slice (~25 production sites + ~5 test-fixture
// updates for the 5 new LAST_* allowlist tests). Defer per Arc 9 #94
// ship-can-spec-deferral — the call-site count exceeds the in-session
// ≤5 ship threshold despite each site being unambiguous.
//
// Mirrors TS PlayerOps.ts:259-340,1026-1033.

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
