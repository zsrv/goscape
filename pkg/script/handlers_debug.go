package script

import (
	"errors"
	"fmt"
)

// handleError aborts the script with a scripted error message. The
// error propagates up to runScript which logs it; Execution is set to
// Aborted by the dispatch loop.
func handleError(s *ScriptState) error {
	msg := s.PopString()
	return fmt.Errorf("ERROR: %s", msg)
}

// handleGetTimeSpent / handleTimeSpent push the active player's
// playtime. TS exposes both names; they have identical behavior.
func handleGetTimeSpent(s *ScriptState) error {
	if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
		return errors.New("GETTIMESPENT: no active player")
	}
	s.PushInt(s.Self.Playtime())
	return nil
}

func handleTimeSpent(s *ScriptState) error {
	if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
		return errors.New("TIMESPENT: no active player")
	}
	s.PushInt(s.Self.Playtime())
	return nil
}
