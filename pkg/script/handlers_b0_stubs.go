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

// 2e3bcf43 (254 pin-advance): the five 244-era TS-unimplemented stubs
// IF_MULTIZONE / IF_OPENMAINOVERLAY / PLAYER_FINDALLZONE / PLAYER_FINDNEXT /
// LAST_COORD were deleted from the upstream enum entirely; their stub
// handlers left with them.
