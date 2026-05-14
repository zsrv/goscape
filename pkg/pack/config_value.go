package pack

import "github.com/zsrv/goscape/pkg/objtype"

// ConfigValue is the typed value of a parsed key=value line.
// TS uses a discriminated union (string | number | boolean | ...);
// Go uses `any` plus per-packer type assertions. The set of permitted
// runtime types grows as more per-config packers land — NAI-193+ will
// add LocModelShape, ParamValue, HuntCheckVar, etc.
//
// TS source: tools/pack/config/PackShared.ts:131 (ConfigValue union).
type ConfigValue = any

// ConfigLine is one key=value pair parsed from a [name]-headed config
// block.
//
// TS source: tools/pack/config/PackShared.ts:132.
type ConfigLine struct {
	Key   string
	Value ConfigValue
}

// IsConfigBoolean reports whether v is one of the six accepted boolean
// literals (yes/no/true/false/1/0). Case-sensitive.
//
// TS source: tools/pack/config/PackShared.ts:31-33.
func IsConfigBoolean(v string) bool {
	return v == "yes" || v == "no" || v == "true" || v == "false" || v == "1" || v == "0"
}

// GetConfigBoolean returns true for "yes"/"true"/"1", false otherwise.
// Case-sensitive. Caller is expected to gate on IsConfigBoolean first.
//
// TS source: tools/pack/config/PackShared.ts:35-37.
func GetConfigBoolean(v string) bool {
	return v == "yes" || v == "true" || v == "1"
}

// ParamValue is the parser output of a `param=name,value` line in
// .struct (and any future config that emits typed-param values).
//
// Type is included so packStructConfigs can decide between p4 and
// pjstr at pack time without re-querying the ParamType registry.
//
// TS source: tools/pack/config/PackShared.ts ParamValue.
type ParamValue struct {
	ID    int
	Type  objtype.ScriptVarType
	Value any
}
