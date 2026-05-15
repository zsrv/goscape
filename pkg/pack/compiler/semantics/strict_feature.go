// pkg/pack/compiler/semantics/strict_feature.go
package semantics

// StrictFeatureLevel toggles feature-disabling at compile time.
// Mirrors TS src/compiler/StrictFeatureLevel.ts.
//
// NAI-205-D-STRICT-INVERTED-POLARITY: TS uses `{ procs?: boolean }`
// where missing-key = enabled (idiomatic in TS); goscape flips polarity
// to `DisableX bool` so the zero value (== TS empty record) corresponds
// to "nothing disabled". If you add fields, name them `DisableX`,
// NEVER `EnableX` — flipping back regresses test fixtures silently.
type StrictFeatureLevel struct {
	DisableProcs    bool // TS features.procs === false
	DisableEnums    bool
	DisableStructs  bool
	DisableDBTables bool
	DisableBooleans bool
}
