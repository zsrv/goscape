// pkg/pack/compiler/runescript/compiler_type_info.go
package runescript

// CompilerTypeInfo carries the per-config / per-command metadata read from
// the external symbol files (one per config kind: command, runescript, loc,
// npc, obj, etc.). Mirrors TS src/runescript/CompilerTypeInfo.ts.
//
// All map fields are keyed by stringified numeric id ("0", "1", ...).
// The Vartype and Protect fields are populated only for some configs
// (varp/varn/dbcolumn). The Require/Set/Corrupt/Conditional fields are
// populated only for command symbols.
type CompilerTypeInfo struct {
	Max int

	// Map: id (as string) → symbol name.
	Map map[string]string

	// Vartype: id (as string) → comma-separated type list.
	Vartype map[string]string

	// Protect: id (as string) → whether the symbol is write-protected.
	Protect map[string]bool

	// Require / Set / Corrupt: id (as string) → comma-separated pointer-name list.
	Require  map[string]string
	Require2 map[string]string
	Set      map[string]string
	Set2     map[string]string
	Corrupt  map[string]string
	Corrupt2 map[string]string

	// Conditional: id (as string) → conditional-set marker.
	Conditional map[string]bool
}
