package script

import "fmt"

// handleLcOp (LC_OP, opcode 4104) — TS-PARITY STUB (final). Opcode is
// declared in TS `ScriptOpcode.ts` but registers no handler entry;
// calling it in TS hits `handlers[X] === undefined`. Goscape's typed-error
// return is the semantic mirror (both raise at this opcode). No goscape
// follow-up; re-port only if upstream TS lands a real body.
// NAI-162-D-STUB-LCOP. See memory `nai164_declined_cohort.md`.
func handleLcOp(s *ScriptState) error {
	return fmt.Errorf("LC_OP: unimplemented")
}

// handleOcIop (OC_IOP, opcode 4205) — TS-unimplemented stub.
// NAI-162-D-STUB-OCIOP.
func handleOcIop(s *ScriptState) error {
	return fmt.Errorf("OC_IOP: unimplemented")
}

// handleOcOp (OC_OP, opcode 4208) — TS-unimplemented stub.
// NAI-162-D-STUB-OCOP.
func handleOcOp(s *ScriptState) error {
	return fmt.Errorf("OC_OP: unimplemented")
}

// handleIfMultizone (IF_MULTIZONE, opcode 2037) — TS-unimplemented at 244:
// declared in ScriptOpcode.ts ("moved to engine, remove this") with no
// handlers/* entry. rev-244-b4 stub posture per NAI-162.
func handleIfMultizone(s *ScriptState) error {
	return fmt.Errorf("IF_MULTIZONE: unimplemented")
}

// handleIfOpenMainOverlay (IF_OPENMAINOVERLAY, opcode 2112) — TS-unimplemented at 244.
func handleIfOpenMainOverlay(s *ScriptState) error {
	return fmt.Errorf("IF_OPENMAINOVERLAY: unimplemented")
}

// handlePlayerFindAllZone (PLAYER_FINDALLZONE, opcode 2091) — TS-unimplemented
// at 244 ("todo: replace with huntall").
func handlePlayerFindAllZone(s *ScriptState) error {
	return fmt.Errorf("PLAYER_FINDALLZONE: unimplemented")
}

// handlePlayerFindNext (PLAYER_FINDNEXT, opcode 2092) — TS-unimplemented at 244.
func handlePlayerFindNext(s *ScriptState) error {
	return fmt.Errorf("PLAYER_FINDNEXT: unimplemented")
}

// handleLastCoord (LAST_COORD, opcode 2126) — TS-unimplemented at 244
// (pointer row exists upstream, ScriptOpcodePointers.ts:528-531; handler does not).
func handleLastCoord(s *ScriptState) error {
	return fmt.Errorf("LAST_COORD: unimplemented")
}
