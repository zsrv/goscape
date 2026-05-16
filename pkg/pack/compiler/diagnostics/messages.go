package diagnostics

// DiagnosticMessage templates ported verbatim from TS
// src/compiler/diagnostics/DiagnosticMessage.ts at HEAD
// b8c338801fbb72d294ff9576a58925a8d3f6de47.
// Format strings are preserved char-for-char (%s placeholders,
// punctuation, trailing period).
//
// Tests assert on (template constant, args slice) — not formatted output —
// to stay deterministic across fmt.Sprintf evolution.
const (
	// Internal compiler errors
	MessageUnsupportedSymbolTypeToType = "Internal compiler error: Unsupported SymbolType -> Type conversion: %s"
	MessageCaseWithoutSwitch           = "Internal compiler error: Case without switch statement as parent."
	MessageReturnOrphan                = "Internal compiler error: Orphaned `return` statement, no parent `script` node found."
	MessageTriggerTypeNotFound         = "Internal compiler error: The trigger '%s' has no declaration."

	// Custom command handler errors
	MessageCustomHandlerNoType   = "Internal compiler error: Custom command handler did not assign return type."
	MessageCustomHandlerNoSymbol = "Internal compiler error: Custom command handler did not assign symbol."

	// Dynamic command type-argument validation (CheckTypeArgument)
	// Mirrors TS TypeCheckingContext.DIAGNOSTIC_TYPEREF_EXPECTED.
	MessageTypeRefExpected = "Type reference expected."

	// Code gen internal compiler errors
	MessageSymbolIsNull        = "Internal compiler error: Symbol has not been defined for the node."
	MessageTypeHasNoBaseType   = "Internal compiler error: Type has no defined base type: %s."
	MessageTypeHasNoDefault    = "Internal compiler error: Return type '%s' has no defined default value."
	MessageInvalidCondition    = "Internal compiler error: %s is not a supported expression type for conditions."
	MessageNullConstant        = "Internal compiler error: %s evaluated to 'null' constant value."
	MessageExpressionNoSubExpr = "Internal compiler error: No sub expression node."

	// Node type agnostic
	MessageGenericInvalidType      = "'%s' is not a valid type."
	MessageGenericTypeMismatch     = "Type mismatch: '%s' was given but '%s' was expected."
	MessageGenericUnresolvedSymbol = "'%s' could not be resolved to a symbol."
	MessageArithmeticInvalidType   = "Type mismatch: '%s' was given but 'int' or 'long' was expected."

	// Script node specific
	MessageScriptRedeclaration             = "[%s,%s] is already defined."
	MessageScriptLocalRedeclaration        = "'$%s' is already defined."
	MessageScriptTriggerInvalid            = "'%s' is not a valid trigger type."
	MessageScriptCommandOnly               = "Using a '*' is only allowed for commands."
	MessageScriptTriggerNoParameters       = "The trigger type '%s' is not allowed to have parameters defined."
	MessageScriptTriggerExpectedParameters = "The trigger type '%s' is expected to accept (%s)."
	MessageScriptTriggerNoReturns          = "The trigger type '%s' is not allowed to return values."
	MessageScriptTriggerExpectedReturns    = "The trigger type '%s' is expected to return (%s)."
	MessageScriptSubjectOnlyGlobal         = "Trigger '%s' only allows global subjects."
	MessageScriptSubjectNoGlobal           = "Trigger '%s' does not allow global subjects."
	MessageScriptSubjectNoCategory         = "Trigger '%s' does not allow category subjects."
	MessageScriptSubjectNoSpaces           = "Trigger '%s' does not allow spaces in subjects."

	// Switch statement
	MessageSwitchInvalidType      = "'%s' is not allowed within a switch statement."
	MessageSwitchDuplicateDefault = "Duplicate default label."
	MessageSwitchCaseNotConstant  = "Switch case value is not a constant expression."

	// Assignment
	MessageAssignMultiArray = "Arrays are not allowed in multi-assignment statements."

	// Expression statement
	MessageExpressionStatementNoSideEffect = "Value is discarded."

	// Condition
	MessageConditionInvalidNodeType = "Conditions are only allowed to be binary expressions."
	MessageConditionNotValid        = "Condition is not valid."

	// Binary expr
	MessageBinopInvalidTypes = "Operator '%s' cannot be applied to '%s', '%s'."
	MessageBinopTupleType    = "%s side of binary expressions can only have one type but has '%s'."

	// Call expr
	MessageCommandReferenceUnresolved      = "'%s' cannot be resolved to a command."
	MessageCommandNoArgsExpected           = "'%s' is expected to have no arguments but has '%s'."
	MessageProcReferenceUnresolved         = "'~%s' cannot be resolved to a proc."
	MessageProcNoArgsExpected              = "'~%s' is expected to have no arguments but has '%s'."
	MessageJumpReferenceUnresolved         = "'@%s' cannot be resolved to a label."
	MessageJumpNoArgsExpected              = "'@%s' is expected to have no arguments but has '%s'."
	MessageClientScriptReferenceUnresolved = "'%s' cannot be resolved to a clientscript."
	MessageClientScriptNoArgsExpected      = "'%s' is expected to have no arguments but has '%s'."
	MessageHookTransmitListUnexpected      = "Unexpected hook transmit list."

	// Local
	MessageLocalDeclarationInvalidType  = "'%s' is not allowed to be declared as a type."
	MessageLocalParameterInvalidType    = "'%s' is not allowed to be used as a parameter."
	MessageLocalReferenceUnresolved     = "'$%s' cannot be resolved to a local variable."
	MessageLocalReferenceNotArray       = "Access of indexed value of non-array type variable '$%s'."
	MessageLocalArrayInvalidType        = "'%s' is not allowed to be used as an array."
	MessageLocalArrayReferenceNoIndex   = "'$%s' is a reference to an array variable without specifying the index."

	// Game var
	MessageGameReferenceUnresolved = "'%%%s' cannot be resolved to a game variable."

	// Constant
	MessageConstantReferenceUnresolved = "'^%s' cannot be resolved to a constant."
	MessageConstantCyclicRef           = "Cyclic constant references are not permitted: %s."
	MessageConstantUnknownType         = "Unable to infer type for '^%s'."
	MessageConstantParseError          = "Unable to parse constant value of '%s' into type '%s'."
	MessageConstantNonConstant         = "Constant value of '%s' evaluated to a non-constant expression."

	// Feature flag
	MessageFeatureDisabledTrigger      = "Trigger '%s' is disabled."
	MessageFeatureDisabledCommand      = "Command '%s' is disabled."
	MessageFeatureDisabledType         = "Type '%s' is disabled."
	MessageFeatureDisabledLocal        = "Local variables are disabled."
	MessageFeatureDisabledBoolean      = "Boolean usage is disabled."
	MessageFeatureDisabledOperator     = "Operator '%s' is disabled."
	MessageFeatureDisabledCalc         = "calc(...) usage is disabled."
	MessageLocalDeclarationNotTopLevel = "Local variables may only be declared at the top level of a script."

	// Pointer
	MessagePointerUninitialized = "Attempt to access uninitialized pointer %s."
	MessagePointerCorrupted     = "Attempt to access corrupted pointer %s."
	MessagePointerCorruptedLoc  = "%s corrupted here."
	MessagePointerRequiredLoc   = "%s required here."

	// Mapzone / zone parse — TS uses inline string literals at ScriptRegistration
	// L294/300/304/326/333/339; exposed as constants here for consistency with
	// the rest of the messages surface.
	MessageMapzoneSubjectForm        = "Mapzone subject must be of the form: 'level_mx_mz'."
	MessageMapzoneInvalidCoord       = "Invalid mapzone coord."
	MessageMapzoneOnlyLevelZero      = "Mapzone affect all level, just specify '0'."
	MessageZoneSubjectForm           = "Zone subject must be of the form: 'level_mx_mz_lx_lz'."
	MessageZoneInvalidCoord          = "Invalid zone coord."
	MessageZoneLocalCoordMultipleOf8 = "Local zone coord must be a multiple of 8"
)
