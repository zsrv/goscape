package script

import "github.com/zsrv/goscape/pkg/objtype"

// Configs is the config-type lookup surface for config-read opcodes
// (OC_*, NC_*, LC_*, ENUM, STRUCT_PARAM, DB_*). Implementations return nil
// when the type isn't loaded or the id is out of range. DbRowsInTable
// returns nil when no rows match or the catalogue is absent. FindDbRows*
// surface DbTableIndex-backed lookups for DB_FIND* handlers.
type Configs interface {
	ObjType(id int) *objtype.ObjType
	NpcType(id int) *objtype.NpcType
	LocType(id int) *objtype.LocType
	EnumType(id int) *objtype.EnumType
	StructType(id int) *objtype.StructType
	ParamType(id int) *objtype.ParamType
	InvType(id int) *objtype.InvType
	IdkType(id int) *objtype.IdkType
	SpotAnimType(id int) *objtype.SpotanimType
	SeqType(id int) *objtype.SeqType
	DbTableType(id int) *objtype.DbTableType
	DbRowType(id int) *objtype.DbRowType
	DbRowsInTable(tableID int) []int

	// FindDbRowsInt returns row IDs whose indexed column (encoded in
	// packed) has any stored INT value equal to query. packed uses the
	// bytecode 1-based tuple-nibble convention; normalization happens
	// inside the *DbTableIndex. Returns nil if the column is not
	// INDEXED or no row matches.
	FindDbRowsInt(query int32, packed int) []int

	// FindDbRowsStr — string-valued variant of FindDbRowsInt.
	FindDbRowsStr(query string, packed int) []int

	// VarpType returns the type and protect bit for a player-var id.
	// Out-of-range or unloaded id returns (ScriptVarTypeInt, false) —
	// degraded mode lets opcode dispatch fall through to int-side
	// (DEVIATION-NAI-121-D3; goscape defensive; TS check() throws).
	VarpType(id int) (typ objtype.ScriptVarType, protect bool)

	// VarnType returns the type for an NPC-var id. Out-of-range or
	// unloaded id returns ScriptVarTypeInt (DEVIATION-NAI-121-D3).
	VarnType(id int) objtype.ScriptVarType

	// ObjByName resolves an ObjType by debugname. Used by WEALTH_EVENT
	// (opcode 2131) to mirror TS ObjType.getByName. Returns nil when the
	// name is unknown. NAI-162 B2.
	ObjByName(name string) *objtype.ObjType
}
