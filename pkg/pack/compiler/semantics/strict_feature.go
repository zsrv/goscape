// pkg/pack/compiler/semantics/strict_feature.go
package semantics

// StrictFeatureLevel toggles feature-disabling at compile time.
// Mirrors TS src/compiler/StrictFeatureLevel.ts at HEAD
// b8c338801fbb72d294ff9576a58925a8d3f6de47.
//
// NAI-205-D-STRICT-INVERTED-POLARITY: TS uses `{ procs?: boolean }`
// where missing-key = enabled (idiomatic in TS); goscape flips polarity
// to `DisableX bool` so the zero value (== TS empty record) corresponds
// to "nothing disabled". If you add fields, name them `DisableX`,
// NEVER `EnableX` — flipping back regresses test fixtures silently.
//
// TopLevelDefOnly is the lone non-Disable field: TS default is `false`
// (top-level def NOT enforced), matching Go's bool zero value. Naming
// it `DisableTopLevelDefOnly` would invert the meaning.
type StrictFeatureLevel struct {
	DisableProcs            bool // TS procs (default true; disabled → no proc decl/call + no locals)
	DisableEnums            bool // TS enums
	DisableStructs          bool // TS structs
	DisableDBTables         bool // TS dbtables
	DisableBooleans         bool // TS booleans
	DisableMacros           bool // TS macros (default true; disabled → no macro parse/expand)
	DisableLogicalAnd       bool // TS logicalAnd (default true; disabled → '&' rejected in conditions)
	DisableCalc             bool // TS calc (default true; disabled → calc(...) lowering rejected)
	DisableRelationalEquals bool // TS relationalEquals (default true; disabled → '<=' '>=' rejected)
	DisableQueueTyped       bool // TS queueTyped (default true; disabled → typed queue variants rejected)
	DisablePointerInversion bool // TS pointerInversion (default true; disabled → conditional pointer-setter inversion rejected)
	TopLevelDefOnly         bool // TS topLevelDefOnly (default false; enabling rejects non-top-level def_T)

	// DisableOverlayInterfaceProtection skips ServerPointerChecker's
	// IF_BUTTON/INV_BUTTON* overlay-aware P_ACTIVE_PLAYER override
	// (RuneScriptTS commit fe0ae0a "feat: Conditionally set
	// p_active_player on if_button/inv_button triggers"). When true the
	// override is short-circuited and the base default
	// `script.trigger.pointers.has(pt)` runs unchanged — matching the
	// behaviour of `@lostcityrs/runescript@0.9.4` bundled by Engine-TS.
	// RunServerCompiler sets this to true so goscape's compile output
	// stays Engine-TS-compatible against real Content; the strict (zero)
	// default tracks RuneScriptTS HEAD when callers wire Compile directly.
	DisableOverlayInterfaceProtection bool
	// DisableStaticLabelArgPropagation skips PointerChecker's
	// buildStaticLabelArgsByCall / addStaticLabelRequirements stage
	// (RuneScriptTS commit 50c9bb1 "feat: Opportunistic label reference
	// pointer checking"). When true, label-typed args passed
	// PushConstantSymbol-style do not contribute required-pointer
	// edges to the caller — matching the bundled v0.9.4 behaviour.
	// Same RunServerCompiler default as above.
	DisableStaticLabelArgPropagation bool
}
