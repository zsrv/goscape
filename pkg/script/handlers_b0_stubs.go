package script

import "fmt"

// handlePushVarbit (PUSH_VARBIT, opcode 25) is a TS-unimplemented stub.
// TS declares the opcode at ScriptOpcode.ts:22 (// official, see cs2)
// but has no handlers/* case-label entry. Per NAI-162 §3 deviation
// NAI-162-D-STUB-PUSHVARBIT, this stub returns an 'unimplemented'
// error rather than no-op so future TS sync re-ports the handler
// explicitly. Mirrors NAI-161 handlePOpHeld shape.
func handlePushVarbit(s *ScriptState) error {
	return fmt.Errorf("PUSH_VARBIT: unimplemented")
}

// handlePopVarbit (POP_VARBIT, opcode 27) — TS-unimplemented stub.
// NAI-162-D-STUB-POPVARBIT.
func handlePopVarbit(s *ScriptState) error {
	return fmt.Errorf("POP_VARBIT: unimplemented")
}

// handleSetGender (SET_GENDER, opcode 2099) — TS-unimplemented stub.
// NAI-162-D-STUB-SETGENDER.
func handleSetGender(s *ScriptState) error {
	return fmt.Errorf("SET_GENDER: unimplemented")
}

// handleLcOp (LC_OP, opcode 4105) — TS-unimplemented stub. Pairs with
// the future OPHELD trigger-plumbing cohort (NAI-161 forward-route).
// NAI-162-D-STUB-LCOP.
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
