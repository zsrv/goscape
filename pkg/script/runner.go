package script

import "fmt"

// Init creates a fresh ScriptState ready to execute script.
//
// int/string arguments are copied into the script's local arrays in declaration
// order (index 0 = first arg). self is wired to Self and PtrActivePlayer is set
// if self != nil. PtrProtectedActivePlayer is set when protect=true and self != nil.
//
// The PC starts at 0; the first instruction is executed on the first Execute tick.
func Init(script *ScriptFile, self ActivePlayer, protect bool, intArgs []int, stringArgs []string) *ScriptState {
	s := &ScriptState{
		Script:    script,
		PC:        0,
		Execution: Running,

		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),

		IntLocals:    make([]int, max(int(script.IntLocalCount), len(intArgs))),
		StringLocals: make([]string, max(int(script.StringLocalCount), len(stringArgs))),

		Frames: make([]Frame, FrameCapacity),

		Self: self,
	}

	copy(s.IntLocals, intArgs)
	copy(s.StringLocals, stringArgs)

	if self != nil {
		s.Pointers |= PtrActivePlayer
		if protect {
			s.Pointers |= PtrProtectedActivePlayer
		}
	}

	return s
}

// Execute runs s until s.Execution != Running.
//
// Returns nil when execution completes normally (Execution == Finished after
// OpReturn with an empty frame stack). Returns a non-nil error on any runtime
// fault, with Execution set to Aborted.
//
// Branch convention: handlers that modify PC set s.PC to (target - 1) so that
// the post-handler s.PC++ lands on the correct instruction. This mirrors the TS
// ScriptRunner convention where handlers leave pc pointing one before the next
// instruction and the loop's ++pc advances to it.
func Execute(s *ScriptState) error {
	for s.Execution == Running {
		if s.OpCount >= OpCountLimit {
			s.Execution = Aborted
			return fmt.Errorf("script %q: opcount limit exceeded at pc=%d", s.Script.Name, s.PC)
		}
		s.OpCount++

		if s.PC < 0 || s.PC >= len(s.Script.Opcodes) {
			s.Execution = Aborted
			return fmt.Errorf("script %q: pc %d out of range [0, %d)", s.Script.Name, s.PC, len(s.Script.Opcodes))
		}

		op := s.Script.Opcodes[s.PC]
		h, ok := handlers[op]
		if !ok {
			s.Execution = Aborted
			return fmt.Errorf("script %q: no handler for %s (opcode %d) at pc=%d",
				s.Script.Name, op.String(), op, s.PC)
		}

		if err := h(s); err != nil {
			s.Execution = Aborted
			return err
		}

		s.PC++
	}
	return nil
}
