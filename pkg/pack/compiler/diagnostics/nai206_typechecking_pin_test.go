// pkg/pack/compiler/diagnostics/nai206_typechecking_pin_test.go
package diagnostics

import (
	"strings"
	"testing"
)

// TestNAI206_TypeCheckingTemplatesPresent enumerates every
// DiagnosticMessage.X constant referenced by
// src/compiler/semantics/TypeChecking.ts at HEAD
// b8c338801fbb72d294ff9576a58925a8d3f6de47, and pins each to its
// goscape Message* identifier. If any goscape Message* gets renamed
// or deleted, this test breaks at compile time — preventing silent
// regressions before NAI-206's walker arms wire them up.
//
// See plan docs/superpowers/plans/2026-05-15-nai-206-typechecking.md,
// T3. NAI-205 pre-landed all 48 templates with TS-verbatim format
// strings; T3 reduces to this audit-pin.
func TestNAI206_TypeCheckingTemplatesPresent(t *testing.T) {
	// Map TS constant → goscape const value (the message format).
	// Listing all 48 forces compile-error on rename.
	templates := map[string]string{
		"ARITHMETIC_INVALID_TYPE":             MessageArithmeticInvalidType,
		"ASSIGN_MULTI_ARRAY":                  MessageAssignMultiArray,
		"BINOP_INVALID_TYPES":                 MessageBinopInvalidTypes,
		"BINOP_TUPLE_TYPE":                    MessageBinopTupleType,
		"CASE_WITHOUT_SWITCH":                 MessageCaseWithoutSwitch,
		"CLIENTSCRIPT_NOARGS_EXPECTED":        MessageClientScriptNoArgsExpected,
		"CLIENTSCRIPT_REFERENCE_UNRESOLVED":   MessageClientScriptReferenceUnresolved,
		"COMMAND_NOARGS_EXPECTED":             MessageCommandNoArgsExpected,
		"COMMAND_REFERENCE_UNRESOLVED":        MessageCommandReferenceUnresolved,
		"CONDITION_INVALID_NODE_TYPE":         MessageConditionInvalidNodeType,
		"CONDITION_NOT_VALID":                 MessageConditionNotValid,
		"CONSTANT_CYCLIC_REF":                 MessageConstantCyclicRef,
		"CONSTANT_NONCONSTANT":                MessageConstantNonConstant,
		"CONSTANT_PARSE_ERROR":                MessageConstantParseError,
		"CONSTANT_REFERENCE_UNRESOLVED":       MessageConstantReferenceUnresolved,
		"CONSTANT_UNKNOWN_TYPE":               MessageConstantUnknownType,
		"CUSTOM_HANDLER_NOSYMBOL":             MessageCustomHandlerNoSymbol,
		"CUSTOM_HANDLER_NOTYPE":               MessageCustomHandlerNoType,
		"EXPRESSION_STATEMENT_NO_SIDE_EFFECT": MessageExpressionStatementNoSideEffect,
		"FEATURE_DISABLED_BOOLEAN":            MessageFeatureDisabledBoolean,
		"FEATURE_DISABLED_CALC":               MessageFeatureDisabledCalc,
		"FEATURE_DISABLED_COMMAND":            MessageFeatureDisabledCommand,
		"FEATURE_DISABLED_LOCAL":              MessageFeatureDisabledLocal,
		"FEATURE_DISABLED_OPERATOR":           MessageFeatureDisabledOperator,
		"FEATURE_DISABLED_TRIGGER":            MessageFeatureDisabledTrigger,
		"FEATURE_DISABLED_TYPE":               MessageFeatureDisabledType,
		"GAME_REFERENCE_UNRESOLVED":           MessageGameReferenceUnresolved,
		"GENERIC_INVALID_TYPE":                MessageGenericInvalidType,
		"GENERIC_TYPE_MISMATCH":               MessageGenericTypeMismatch,
		"GENERIC_UNRESOLVED_SYMBOL":           MessageGenericUnresolvedSymbol,
		"HOOK_TRANSMIT_LIST_UNEXPECTED":       MessageHookTransmitListUnexpected,
		"JUMP_NOARGS_EXPECTED":                MessageJumpNoArgsExpected,
		"JUMP_REFERENCE_UNRESOLVED":           MessageJumpReferenceUnresolved,
		"LOCAL_ARRAY_INVALID_TYPE":            MessageLocalArrayInvalidType,
		"LOCAL_ARRAY_REFERENCE_NOINDEX":       MessageLocalArrayReferenceNoIndex,
		"LOCAL_DECLARATION_INVALID_TYPE":      MessageLocalDeclarationInvalidType,
		"LOCAL_DECLARATION_NOT_TOPLEVEL":      MessageLocalDeclarationNotTopLevel,
		"LOCAL_REFERENCE_NOT_ARRAY":           MessageLocalReferenceNotArray,
		"LOCAL_REFERENCE_UNRESOLVED":          MessageLocalReferenceUnresolved,
		"PROC_NOARGS_EXPECTED":                MessageProcNoArgsExpected,
		"PROC_REFERENCE_UNRESOLVED":           MessageProcReferenceUnresolved,
		"RETURN_ORPHAN":                       MessageReturnOrphan,
		"SCRIPT_LOCAL_REDECLARATION":          MessageScriptLocalRedeclaration,
		"SWITCH_CASE_NOT_CONSTANT":            MessageSwitchCaseNotConstant,
		"SWITCH_DUPLICATE_DEFAULT":            MessageSwitchDuplicateDefault,
		"SWITCH_INVALID_TYPE":                 MessageSwitchInvalidType,
		"TRIGGER_TYPE_NOT_FOUND":              MessageTriggerTypeNotFound,
		"UNSUPPORTED_SYMBOLTYPE_TO_TYPE":      MessageUnsupportedSymbolTypeToType,
	}
	if got, want := len(templates), 48; got != want {
		t.Fatalf("template map size = %d, want %d (TypeChecking.ts at pinned HEAD references exactly 48 templates)", got, want)
	}
	for tsName, msg := range templates {
		if msg == "" {
			t.Errorf("%s: empty template (goscape const is empty string)", tsName)
		}
	}
}

// TestNAI206_TemplateFormatStringParity pins 5 representative
// templates to their TS-verbatim format string at HEAD
// b8c338801fbb72d294ff9576a58925a8d3f6de47. Catches accidental
// reword that diverges from TS.
func TestNAI206_TemplateFormatStringParity(t *testing.T) {
	cases := []struct {
		name string
		got  string
		want string
	}{
		{
			"FEATURE_DISABLED_BOOLEAN",
			MessageFeatureDisabledBoolean,
			"Boolean usage is disabled.",
		},
		{
			"FEATURE_DISABLED_CALC",
			MessageFeatureDisabledCalc,
			"calc(...) usage is disabled.",
		},
		{
			"LOCAL_DECLARATION_NOT_TOPLEVEL",
			MessageLocalDeclarationNotTopLevel,
			"Local variables may only be declared at the top level of a script.",
		},
		{
			"CONSTANT_NONCONSTANT",
			MessageConstantNonConstant,
			"Constant value of '%s' evaluated to a non-constant expression.",
		},
		{
			"HOOK_TRANSMIT_LIST_UNEXPECTED",
			MessageHookTransmitListUnexpected,
			_tsHookTransmitListUnexpected,
		},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s: format %q, want %q (TS at pinned HEAD)", c.name, c.got, c.want)
		}
	}
	// Defensive: ensure constants are not empty.
	for _, c := range cases {
		if strings.TrimSpace(c.got) == "" {
			t.Errorf("%s: empty format string", c.name)
		}
	}
}

// _tsHookTransmitListUnexpected is the TS-verbatim format string
// for DiagnosticMessage.HOOK_TRANSMIT_LIST_UNEXPECTED at HEAD
// b8c338801fbb72d294ff9576a58925a8d3f6de47, hoisted to a package-level
// const so the test can compare byte-for-byte without escaping issues.
const _tsHookTransmitListUnexpected = "Unexpected hook transmit list."
