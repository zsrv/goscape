package objtype

import (
	"fmt"
	"path/filepath"
	"sort"

	packet2 "github.com/zsrv/goscape/pkg/io/packet"
)

// DbRowType describes a single DB row parsed from server/dbrow.dat.
// Like DbTableType, column values use parallel typed arrays
// (IntValues / StringValues, both allocated to the same flat length per
// column) per S7d-D1. A row may declare a subset of columns; undeclared
// slots remain nil.
type DbRowType struct {
	ConfigType
	TableID      int
	Types        [][]ScriptVarType
	IntValues    [][]int32
	StringValues [][]string
}

// NewDbRowType returns a zero-valued DbRowType with the given id.
func NewDbRowType(id int) *DbRowType {
	return &DbRowType{
		ID: id,
	}
}

// Decode mirrors TS DbRowType.ts:70.
func (r *DbRowType) Decode(code uint8, dat *packet2.Packet) error {
	switch code {
	case 3:
		numColumns := int(dat.G1())
		r.Types = make([][]ScriptVarType, numColumns)
		r.IntValues = make([][]int32, numColumns)
		r.StringValues = make([][]string, numColumns)

		for columnID := dat.G1(); columnID != 255; columnID = dat.G1() {
			typeCount := int(dat.G1())
			columnTypes := make([]ScriptVarType, typeCount)
			for i := range typeCount {
				columnTypes[i] = ScriptVarType(dat.G1())
			}
			r.Types[columnID] = columnTypes

			ints, strs := decodeDbValues(dat, columnTypes)
			r.IntValues[columnID] = ints
			r.StringValues[columnID] = strs
		}
	case 4:
		r.TableID = int(dat.G2())
	case 250:
		r.DebugName = dat.GJStrLF()
	default:
		return fmt.Errorf("unrecognized dbrow config code %d", code)
	}
	return nil
}

// GetValue mirrors TS DbRowType.ts:95. Returns per-tuple parallel slices
// for the given column and listIndex. On out-of-range listIndex (resulting
// in an empty slice), falls back to table.GetDefault(column). The caller
// (handler) passes *DbTableType explicitly since Go has no static registry
// (S7d-D2).
func (r *DbRowType) GetValue(column, listIndex int, table *DbTableType) (ints []int32, strs []string, types []ScriptVarType) {
	types = r.Types[column]
	tupLen := len(types)
	start := listIndex * tupLen
	end := start + tupLen

	if tupLen == 0 || end > len(r.IntValues[column]) {
		return table.GetDefault(column)
	}
	return r.IntValues[column][start:end], r.StringValues[column][start:end], types
}

// DbRowTypeConfigs is the loaded DbRowType catalogue. RowsByTable is
// pre-computed at load time (S7d-D4) so DB_LISTALL doesn't need to
// filter the full config slice per call.
type DbRowTypeConfigs struct {
	ConfigNames map[string]int
	Configs     []*DbRowType
	RowsByTable map[int][]int
}

// LoadDbRowTypes parses server/dbrow.dat from the given cache dir.
func LoadDbRowTypes(dir string) (*DbRowTypeConfigs, error) {
	server, err := packet2.Load(filepath.Join(dir, "server", "dbrow.dat"), false)
	if err != nil {
		return nil, err
	}
	return parseDbRowTypes(server)
}

func parseDbRowTypes(server *packet2.Packet) (*DbRowTypeConfigs, error) {
	count := int(server.G2())

	configs := make([]*DbRowType, count)
	configNames := make(map[string]int, count)
	rowsByTable := make(map[int][]int)

	for id := range count {
		config := NewDbRowType(id)
		if err := DecodeType(server, config); err != nil {
			return nil, err
		}
		configs[id] = config
		if config.DebugName != "" {
			configNames[config.DebugName] = id
		}
		rowsByTable[config.TableID] = append(rowsByTable[config.TableID], id)
	}

	// Ensure ascending-order invariant per column within each table (S7d-D4).
	// Append order is already ascending because id ranges 0..count, but
	// sorting is cheap and documents the invariant for any future refactor.
	for tableID := range rowsByTable {
		sort.Ints(rowsByTable[tableID])
	}

	return &DbRowTypeConfigs{
		ConfigNames: configNames,
		Configs:     configs,
		RowsByTable: rowsByTable,
	}, nil
}
