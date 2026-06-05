package script

import (
	"fmt"
)

// enqueueTimer is the shared body for SETTIMER / SOFTTIMER.
//
// NAI-27 Bundle 2: activates popScriptArgs (mirrors TS PlayerOps.ts:826,834);
// activates script-missing error propagation via (*Player).SetTimer return
// (mirrors EnqueueScriptArgs pattern at modules/world/player_script.go:102-118
// for the queue family).
func enqueueTimer(s *ScriptState, ttype PlayerTimerType, op string) error {
	if err := requireActivePlayer(s, op); err != nil {
		return err
	}
	intArgs, stringArgs := popScriptArgs(s)
	interval := s.PopInt()
	scriptID := uint32(s.PopInt())
	return s.activePlayer().SetTimer(scriptID, interval, intArgs, stringArgs, ttype)
}

func handleSetTimer(s *ScriptState) error  { return enqueueTimer(s, TimerNormal, "SETTIMER") }
func handleSoftTimer(s *ScriptState) error { return enqueueTimer(s, TimerSoft, "SOFTTIMER") }

func handleClearTimer(s *ScriptState) error {
	if err := requireActivePlayer(s, "CLEARTIMER"); err != nil {
		return err
	}
	scriptID := uint32(s.PopInt())
	s.activePlayer().ClearTimer(scriptID)
	return nil
}

func handleClearSoftTimer(s *ScriptState) error {
	if err := requireActivePlayer(s, "CLEARSOFTTIMER"); err != nil {
		return err
	}
	scriptID := uint32(s.PopInt())
	s.activePlayer().ClearTimer(scriptID)
	return nil
}

// handleGetTimer (GETTIMER, opcode 2019) pops scriptID, validates it
// resolves to a registered script (TS PlayerOps.ts:852-854), and pushes
// the absolute clock tick (TS PlayerOps.ts:858) of the matching timer
// or -1 if no timer is registered (TS PlayerOps.ts:863).
//
// NAI-27 Bundle 2: handler-side script-missing check (vs entity-side
// for SETTIMER/SOFTTIMER) because (*Player).GetTimer returns int
// (with -1 sentinel) — pattern parallels handleGosubWithParams at
// pkg/script/handlers.go:541-554.
func handleGetTimer(s *ScriptState) error {
	if err := requireActivePlayer(s, "GETTIMER"); err != nil {
		return err
	}
	scriptID := uint32(s.PopInt())
	if s.Provider == nil {
		return fmt.Errorf("GETTIMER: Provider not set on ScriptState")
	}
	if s.Provider.GetByID(scriptID) == nil {
		return fmt.Errorf("GETTIMER: unable to find timer script: %d", scriptID)
	}
	s.PushInt(s.activePlayer().GetTimer(scriptID))
	return nil
}
