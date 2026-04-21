package script

import (
	"errors"
	"fmt"
)

// enqueueTimer is the shared body for SETTIMER / SOFTTIMER.
func enqueueTimer(s *ScriptState, ttype PlayerTimerType, op string) error {
	if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
		return fmt.Errorf("%s: no active player", op)
	}
	arg := int(s.PopInt())
	interval := int(s.PopInt())
	scriptID := uint32(s.PopInt())
	s.Self.SetTimer(scriptID, interval, arg, ttype)
	return nil
}

func handleSetTimer(s *ScriptState) error  { return enqueueTimer(s, TimerNormal, "SETTIMER") }
func handleSoftTimer(s *ScriptState) error { return enqueueTimer(s, TimerSoft, "SOFTTIMER") }

func handleClearTimer(s *ScriptState) error {
	if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
		return errors.New("CLEARTIMER: no active player")
	}
	scriptID := uint32(s.PopInt())
	s.Self.ClearTimer(scriptID)
	return nil
}

func handleClearSoftTimer(s *ScriptState) error {
	if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
		return errors.New("CLEARSOFTTIMER: no active player")
	}
	scriptID := uint32(s.PopInt())
	s.Self.ClearTimer(scriptID)
	return nil
}

func handleGetTimer(s *ScriptState) error {
	if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
		return errors.New("GETTIMER: no active player")
	}
	scriptID := uint32(s.PopInt())
	s.PushInt(s.Self.GetTimer(scriptID))
	return nil
}
