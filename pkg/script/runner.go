package script

import (
	"fmt"
	"strings"
)

// Init creates a fresh ScriptState ready to execute script.
//
// int/string arguments are copied into the script's local arrays in declaration
// order (index 0 = first arg). self is wired to Self and PtrActivePlayer is set
// if self != nil. PtrProtectedActivePlayer is set when protect=true and self != nil.
//
// The PC starts at 0; the first instruction is executed on the first Execute tick.
func Init(script *ScriptFile, self ActivePlayer, protect bool, intArgs []int, stringArgs []string) *ScriptState {
	// PERF-3: the three fixed-capacity buffers come from the pool
	// (pool.go); a fresh-from-New bundle is zeroed by make, a recycled one
	// has stringStack/frames cleared by Release and an intentionally-dirty
	// intStack (unobservable — see pool.go). Locals stay freshly allocated:
	// RuneScript reads them before writing and relies on zero-init.
	b := buffersPool.Get().(*scriptBuffers)
	s := &ScriptState{
		Script:    script,
		PC:        0,
		Execution: Running,

		IntStack:    b.intStack,
		StringStack: b.stringStack,

		IntLocals:    make([]int, max(int(script.IntLocalCount), len(intArgs))),
		StringLocals: make([]string, max(int(script.StringLocalCount), len(stringArgs))),

		Frames: b.frames,

		Self: self,
		buf:  b,
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
		// TS (ScriptRunner.ts:144) checks `opcount > 500_000` before the
		// post-check `opcount++`, so it executes 500_001 opcodes before
		// aborting. `>=` here would abort one opcode early.
		if s.OpCount > OpCountLimit {
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
			// 244 delta: TS ScriptRunner.ts:151 drops the name-map lookup
			// (`Unknown opcode ${opcode}`) — the mnemonic is no longer
			// prepended. Go mirrors by removing op.String() from the message.
			return fmt.Errorf("script %q: unknown opcode %d at pc=%d",
				s.Script.Name, op, s.PC)
		}

		if err := h(s); err != nil {
			s.Execution = Aborted
			// script-core-1: TS ScriptRunner.ts:182-186 prepends the
			// (lowercased) opcode mnemonic to err.message — and prepends
			// '.' when the secondary bit of a VARP/VARN operand is set
			// (the protected-variant marker). Goscape mirrors that here
			// so callers see e.g. "goto invalid jump target ..." or
			// ".pop_varp foo not writable ...". Other fault paths in this
			// loop already encode the opcode in their fmt.Errorf format
			// strings, so the prefix is only applied to handler-returned
			// errors (the audit-cited runner.go:76-79 site).
			//
			// Goscape-convention deviation: pkg/script handlers historically
			// embed the opcode name into their own error strings
			// (e.g. "P_PAUSEBUTTON: script not protected"). When the
			// handler-emitted message already starts with the opcode name
			// (case-insensitive), the runner-level prefix is suppressed to
			// avoid redundant "p_pausebutton P_PAUSEBUTTON: ..." chains.
			// The TS-intent (the canonical opcode name accompanies the
			// error) is preserved either way; the divergence is purely
			// cosmetic. scriptOpcodePrefix handles this check internally.
			return fmt.Errorf("%s%w", scriptOpcodePrefix(s, err.Error()), err)
		}

		s.PC++
	}
	return nil
}

// scriptOpcodePrefix returns the lowercased opcode mnemonic at the current
// PC followed by a single space, optionally preceded by '.' when the
// VARP/VARN operand's bit 16 is set (the protected-variant marker, e.g.
// `.varp` / `.varn` in RuneScript source). Returns the empty string when
// the PC is out-of-range so the caller's error message is unchanged.
//
// Mirrors TS ScriptRunner.ts:170-186 — the secondary-flag derivation maps
// 1:1 to the TS switch:
//   - PUSH_VARP / POP_VARP / PUSH_VARN / POP_VARN: secondary = (op>>16)&1
//   - opcode <= POP_ARRAY_INT: secondary forced to 0
//   - any higher opcode: secondary = state.intOperand (TS quirk where any
//     non-zero operand on a large-operand opcode trips the '.' prefix).
//
// When existingMsg is provided and already begins with the opcode name
// (case-insensitive), the empty string is returned. This honors goscape's
// pre-existing convention of embedding `OPCODE_NAME:` directly in handler
// errors — adding a runner-level prefix on top would yield redundant
// "p_pausebutton P_PAUSEBUTTON: ..." chains. The TS-intent (the canonical
// opcode name accompanies the error) holds either way.
func scriptOpcodePrefix(s *ScriptState, existingMsg string) string {
	if s.PC < 0 || s.PC >= len(s.Script.Opcodes) {
		return ""
	}
	op := s.Script.Opcodes[s.PC]
	name := strings.ToLower(op.String())

	if existingMsg != "" && len(existingMsg) >= len(name) &&
		strings.EqualFold(existingMsg[:len(name)], name) {
		return ""
	}

	// IntOperands may be shorter than Opcodes in test fixtures that omit
	// the operand for a single-byte opcode; default to 0 so the secondary
	// derivation degrades gracefully.
	var operand int32
	if s.PC < len(s.Script.IntOperands) {
		operand = s.Script.IntOperands[s.PC]
	}
	var secondary int
	switch op {
	case OpPushVarp, OpPopVarp, OpPushVarn, OpPopVarn:
		secondary = int((operand >> 16) & 0x1)
	default:
		if op <= OpPopArrayInt {
			secondary = 0
		} else {
			secondary = int(operand)
		}
	}

	if secondary != 0 {
		return "." + name + " "
	}
	return name + " "
}

// Backtrace returns the per-frame stack-trace lines a script-error reporter
// should emit. The first entry is the literal header "stack backtrace:";
// subsequent entries are "    N: <name> - <fileName>:<line>" — frame 1 is
// the currently-executing script (state.Script @ state.PC), frames 2..N are
// the GOSUB call stack from most-recent (Frames[FrameSP-1]) down to
// Frames[1]. Frame 0 (the oldest GOSUB frame) is intentionally skipped.
//
// 244 delta: TS ScriptRunner.ts:196 (Player path) and ScriptRunner.ts:221
// (console path) both changed from `i >= 0` to `i > 0` — frame 0 is
// excluded from both traces. Go shares one Backtrace impl so this single
// change covers both paths, matching TS exactly.
//
// Source-line resolution uses ScriptFile.LineNumber, the PCs/Lines table
// accessor added alongside this helper (script-core-5 closure).
//
// Safe on a partially-initialised state — a nil/empty Script.PCs degrades
// to ":0" entries rather than panicking.
func Backtrace(s *ScriptState) []string {
	if s == nil || s.Script == nil {
		return nil
	}
	out := []string{"stack backtrace:"}
	out = append(out, fmt.Sprintf("    1: %s - %s:%d",
		s.Script.Name, s.Script.FileName, s.Script.LineNumber(s.PC)))
	trace := 1
	// 244: loop is i > 0, not i >= 0 — frame 0 (oldest GOSUB frame) is
	// skipped. Mirrors TS ScriptRunner.ts:196 and :221.
	for i := s.FrameSP - 1; i > 0; i-- {
		f := s.Frames[i]
		if f.Script == nil {
			continue
		}
		trace++
		out = append(out, fmt.Sprintf("    %d: %s - %s:%d",
			trace, f.Script.Name, f.Script.FileName, f.Script.LineNumber(f.PC)))
	}
	return out
}
