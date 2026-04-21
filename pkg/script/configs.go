package script

import "github.com/zsrv/goscape/pkg/objtype"

// Configs is the config-type lookup surface for config-read opcodes
// (OC_*, NC_*, LC_*, ENUM, STRUCT_PARAM). Implementations return nil
// when the type isn't loaded or the id is out of range.
type Configs interface {
	ObjType(id int) *objtype.ObjType
	NpcType(id int) *objtype.NpcType
	LocType(id int) *objtype.LocType
	EnumType(id int) *objtype.EnumType
	StructType(id int) *objtype.StructType
	ParamType(id int) *objtype.ParamType
}
