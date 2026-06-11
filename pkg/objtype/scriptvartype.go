package objtype

// ScriptVarType is the single-byte type code stored in cache .dat files.
// The numeric value equals the ASCII codepoint of the legacy type
// letter (e.g. 'i' = 105 for int).
//
// TS source: src/cache/config/ScriptVarType.ts.
type ScriptVarType int

const (
	ScriptVarTypeInt       ScriptVarType = 105 // i
	ScriptVarTypeAutoInt   ScriptVarType = 255 // ÿ
	ScriptVarTypeString    ScriptVarType = 115 // s
	ScriptVarTypeEnum      ScriptVarType = 103 // g
	ScriptVarTypeObj       ScriptVarType = 111 // o
	ScriptVarTypeLoc       ScriptVarType = 108 // l
	ScriptVarTypeComponent ScriptVarType = 73  // I
	ScriptVarTypeNamedObj  ScriptVarType = 79  // O
	ScriptVarTypeStruct    ScriptVarType = 74  // J
	ScriptVarTypeBoolean   ScriptVarType = 49  // 1
	ScriptVarTypeCoord     ScriptVarType = 99  // c
	ScriptVarTypeCategory  ScriptVarType = 121 // y
	ScriptVarTypeSpotanim  ScriptVarType = 116 // t
	ScriptVarTypeNPC       ScriptVarType = 110 // n
	ScriptVarTypeInv       ScriptVarType = 118 // v
	ScriptVarTypeSynth     ScriptVarType = 80  // P
	ScriptVarTypeSeq       ScriptVarType = 65  // A
	ScriptVarTypeStat      ScriptVarType = 83  // S
	ScriptVarTypeInterface ScriptVarType = 97  // a
	ScriptVarTypeVarp      ScriptVarType = 86  // V
	ScriptVarTypePlayerUid ScriptVarType = 112 // p
	ScriptVarTypeNpcUid    ScriptVarType = 78  // N
	ScriptVarTypeNpcStat   ScriptVarType = 254 // þ
	ScriptVarTypeIdkit     ScriptVarType = 75  // K
	ScriptVarTypeDbrow     ScriptVarType = 208 // Ð
	// ScriptVarTypeMidi — TS src/cache/config/ScriptVarType.ts:27
	// @2e3bcf43 `static readonly MIDI = 77; // M` (new at the rev-254
	// pin-advance). Engine-runtime consumers are limited to type-name
	// mapping (TS ScriptVarType.getType / ParamType.getType debug
	// naming); the heavy lifting (midi-name symbol resolution) happens
	// in the pack compiler (pkg/pack/compiler/type ScriptVarMidi). A10.
	ScriptVarTypeMidi ScriptVarType = 77 // M
)

// ScriptVarTypeFromName returns the ScriptVarType code for a type
// name, or (0, false) for unknown names. Matches TS
// ScriptVarType.getTypeChar.
//
// TS source: src/cache/config/ScriptVarType.ts:85-170.
func ScriptVarTypeFromName(name string) (ScriptVarType, bool) {
	switch name {
	case "int":
		return ScriptVarTypeInt, true
	case "autoint":
		return ScriptVarTypeAutoInt, true
	case "string":
		return ScriptVarTypeString, true
	case "enum":
		return ScriptVarTypeEnum, true
	case "obj":
		return ScriptVarTypeObj, true
	case "loc":
		return ScriptVarTypeLoc, true
	case "component":
		return ScriptVarTypeComponent, true
	case "namedobj":
		return ScriptVarTypeNamedObj, true
	case "struct":
		return ScriptVarTypeStruct, true
	case "boolean":
		return ScriptVarTypeBoolean, true
	case "coord":
		return ScriptVarTypeCoord, true
	case "category":
		return ScriptVarTypeCategory, true
	case "spotanim":
		return ScriptVarTypeSpotanim, true
	case "npc":
		return ScriptVarTypeNPC, true
	case "inv":
		return ScriptVarTypeInv, true
	case "synth":
		return ScriptVarTypeSynth, true
	case "seq":
		return ScriptVarTypeSeq, true
	case "stat":
		return ScriptVarTypeStat, true
	case "varp":
		return ScriptVarTypeVarp, true
	case "player_uid":
		return ScriptVarTypePlayerUid, true
	case "npc_uid":
		return ScriptVarTypeNpcUid, true
	case "interface":
		return ScriptVarTypeInterface, true
	case "npc_stat":
		return ScriptVarTypeNpcStat, true
	case "idkit":
		return ScriptVarTypeIdkit, true
	case "dbrow":
		return ScriptVarTypeDbrow, true
	case "midi":
		return ScriptVarTypeMidi, true
	}
	return 0, false
}
