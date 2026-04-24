package objtype

import (
	"fmt"
	"path/filepath"

	packet2 "github.com/zsrv/goscape/pkg/io/packet"
)

// Per-column flag bits used in DbTableType.Props (code 252). Mirrors TS
// DbTableType.ts:12-15. Constants are currently consumed only by future
// DB_FIND* handlers (not in S7d scope) — kept exported so the wire-level
// meaning is documented.
const (
	DbTableFlagIndexed    uint8 = 0x1
	DbTableFlagRequired   uint8 = 0x2
	DbTableFlagList       uint8 = 0x4
	DbTableFlagClientside uint8 = 0x8
)

// DbTableType describes a DB table schema parsed from server/dbtable.dat.
// Column values use Approach 1 (parallel typed arrays) per S7d-D1:
//
//   - Types[col][typeID] is the ScriptVarType of the typeID-th slot in the
//     column's tuple. Types[col] == nil for columns not declared in code 1.
//   - DefaultInts[col][typeID + fieldID*len(Types[col])] holds the stored
//     default int value where Types[col][typeID] != STRING; same stride for
//     DefaultStrs where the type == STRING. Both are nil if no default was
//     stored (use GetDefault to synthesize).
type DbTableType struct {
	ConfigType
	Types       [][]ScriptVarType
	DefaultInts [][]int32
	DefaultStrs [][]string
	ColumnNames []string
	Props       []uint8
}

// NewDbTableType returns a zero-valued DbTableType with the given id.
func NewDbTableType(id int) *DbTableType {
	return &DbTableType{
		ConfigType: ConfigType{ID: id},
	}
}

// Decode mirrors TS DbTableType.ts:72.
func (t *DbTableType) Decode(code uint8, dat *packet2.Packet) error {
	switch code {
	case 1:
		columnCount := int(dat.G1())
		t.Types = make([][]ScriptVarType, columnCount)
		t.DefaultInts = make([][]int32, columnCount)
		t.DefaultStrs = make([][]string, columnCount)

		for setting := dat.G1(); setting != 255; setting = dat.G1() {
			column := int(setting & 0x7f)
			hasDefault := setting&0x80 != 0

			typeCount := int(dat.G1())
			columnTypes := make([]ScriptVarType, typeCount)
			for i := range typeCount {
				columnTypes[i] = ScriptVarType(dat.G1())
			}
			t.Types[column] = columnTypes

			if hasDefault {
				ints, strs := decodeDbValues(dat, columnTypes)
				t.DefaultInts[column] = ints
				t.DefaultStrs[column] = strs
			}
		}
	case 250:
		t.DebugName = dat.GJStrLF()
	case 251:
		n := int(dat.G1())
		t.ColumnNames = make([]string, n)
		for i := range n {
			t.ColumnNames[i] = dat.GJStrLF()
		}
	case 252:
		n := int(dat.G1())
		t.Props = make([]uint8, n)
		for i := range n {
			t.Props[i] = dat.G1()
		}
	default:
		return fmt.Errorf("unrecognized dbtable config code %d", code)
	}
	return nil
}

// decodeDbValues reads a field-count-prefixed tuple-values block from dat
// into parallel int/string slices. The layout is striped: the value at
// index (typeID + fieldID*len(types)) is an int32 read via G4 when
// types[typeID] != STRING, or a string read via GJStrLF when it is.
// Unused slots (opposite type) are zero-initialised. Length of both
// returned slices equals fieldCount * len(types).
func decodeDbValues(dat *packet2.Packet, types []ScriptVarType) (ints []int32, strs []string) {
	fieldCount := int(dat.G1())
	n := fieldCount * len(types)
	ints = make([]int32, n)
	strs = make([]string, n)
	for fieldID := range fieldCount {
		for typeID, t := range types {
			idx := typeID + fieldID*len(types)
			if t == ScriptVarTypeString {
				strs[idx] = dat.GJStrLF()
			} else {
				ints[idx] = int32(dat.G4())
			}
		}
	}
	return ints, strs
}

// GetDefault returns per-tuple parallel arrays of length len(t.Types[column]).
// If a stored default exists, returns the column's stored slices verbatim.
// Otherwise synthesises per-slot defaults via scriptVarTypeDefault. The
// third return (types) echoes t.Types[column] for caller convenience;
// callers that already have the types in scope may ignore it.
func (t *DbTableType) GetDefault(column int) (ints []int32, strs []string, types []ScriptVarType) {
	types = t.Types[column]
	if t.DefaultInts[column] != nil {
		return t.DefaultInts[column], t.DefaultStrs[column], types
	}
	ints = make([]int32, len(types))
	strs = make([]string, len(types))
	for i, vt := range types {
		ints[i], strs[i] = scriptVarTypeDefault(vt)
	}
	return ints, strs, types
}

// scriptVarTypeDefault mirrors TS ScriptVarType.ts:172 — STRING → "",
// BOOLEAN → 0, else → -1. Unexported until a second consumer materialises
// (YAGNI).
func scriptVarTypeDefault(t ScriptVarType) (intVal int32, strVal string) {
	switch t {
	case ScriptVarTypeString:
		return 0, ""
	case ScriptVarTypeBoolean:
		return 0, ""
	default:
		return -1, ""
	}
}

// DbTableTypeConfigs is the loaded DbTableType catalogue.
type DbTableTypeConfigs struct {
	ConfigNames map[string]int
	Configs     []*DbTableType
}

// LoadDbTableTypes parses server/dbtable.dat from the given cache dir.
func LoadDbTableTypes(dir string) (*DbTableTypeConfigs, error) {
	server, err := packet2.Load(filepath.Join(dir, "server", "dbtable.dat"), false)
	if err != nil {
		return nil, err
	}
	return parseDbTableTypes(server)
}

func parseDbTableTypes(server *packet2.Packet) (*DbTableTypeConfigs, error) {
	count := int(server.G2())

	configs := make([]*DbTableType, count)
	configNames := make(map[string]int, count)

	for id := range count {
		config := NewDbTableType(id)
		if err := DecodeType(server, config); err != nil {
			return nil, err
		}
		configs[id] = config
		if config.DebugName != "" {
			configNames[config.DebugName] = id
		}
	}

	return &DbTableTypeConfigs{
		ConfigNames: configNames,
		Configs:     configs,
	}, nil
}
