package pack

import (
	"fmt"
	"strings"

	"github.com/zsrv/goscape/pkg/objtype"
)

// parseDbRowConfigFor returns a ParseFn for .dbrow config blocks.
// The returned closure captures dbtablePack for resolving table= names
// to ids via GetByName.
//
// Accepted keys:
//   - table  → resolves name to id via dbtablePack.GetByName; rejects
//     unknown names with (nil, true, err).
//   - data   → returns the raw CSV string value.
//
// NAI-195-D-DEADBRANCH-OMITTED: TS DbRowConfig.ts:30-32 declares empty
// stringKeys/numberKeys/booleanKeys arrays — all three branches are dead
// and are omitted here.
//
// TS source: tools/pack/config/DbRowConfig.ts:29-82.
func parseDbRowConfigFor(dbtablePack *PackFile) ParseFn {
	return func(key, value string) (ConfigValue, bool, error) {
		switch key {
		case "table":
			idx := dbtablePack.GetByName(value)
			if idx == -1 {
				return nil, true, fmt.Errorf("unknown dbtable name %q", value)
			}
			return idx, true, nil
		case "data":
			return value, true, nil
		}
		return nil, false, nil
	}
}

// dbRowDataField holds one parsed data= line from a .dbrow config block:
// the column name and the ordered value strings for that field.
type dbRowDataField struct {
	column string
	values []string
}

// packDbRowConfigs walks every id ∈ [0, pf.Max), two-pass-walks each
// per-id config block (find table then collect data), and emits opcodes
// 3 (gated on data.length > 0), 4 (always when table resolved), and the
// 250-trailer per id.
//
// Server-only — TS allocates a client PackedData but never writes to it
// between Next() calls; goscape omits the dead client buffer entirely.
//
// Opcode layout:
//
//	3   → col-count | per-column(col-id, type-count, types, field-count, fields) | 255
//	4   → P2(table.id)
//	250 → debugname PJStr  (when len(name) > 0)
//
// TS source: tools/pack/config/DbRowConfig.ts:84-185.
func packDbRowConfigs(
	configs map[string][]ConfigLine,
	pf *PackFile,
	dbtablePF *PackFile,
	dbtableTypes *objtype.DbTableTypeConfigs,
	lk *paramLookups,
) (*PackedData, error) {
	pd := NewPackedData(pf.Max)

	for id := range pf.Max {
		name := pf.GetByID(id)
		if cfg, ok := configs[name]; ok {
			// Pass 1: find the table line and resolve to *DbTableType.
			tableID := -1
			for _, line := range cfg {
				if line.Key == "table" {
					tableID = line.Value.(int)
					break
				}
			}
			if tableID < 0 {
				return nil, packStepError(name, "No table defined for dbrow")
			}
			if tableID >= len(dbtableTypes.Configs) || dbtableTypes.Configs[tableID] == nil {
				return nil, packStepError(name, "table id %d out of range or nil", tableID)
			}
			table := dbtableTypes.Configs[tableID]

			// Pass 2: collect data= lines.
			var data []dbRowDataField
			for _, line := range cfg {
				if line.Key != "data" {
					continue
				}
				parts := parseCsv(line.Value.(string))
				column := parts[0]
				values := parts[1:]
				data = append(data, dbRowDataField{column: column, values: values})
			}

			// Opcode 3: only when data is non-empty.
			if len(data) > 0 {
				pd.P1(3)
				pd.P1(uint8(len(table.Types)))

				for i := range len(table.Types) {
					pd.P1(uint8(i)) // column index
					types := table.Types[i]
					pd.P1(uint8(len(types)))
					for _, t := range types {
						pd.P1(uint8(t))
					}

					colName := table.ColumnNames[i]
					// Collect fields matching this column name.
					var fields []dbRowDataField
					for _, d := range data {
						if d.column == colName {
							fields = append(fields, d)
						}
					}

					props := table.Props[i]
					if props&objtype.DbTableFlagRequired != 0 && len(fields) == 0 {
						return nil, packStepError(name, "%s column is marked REQUIRED, please add data for it", colName)
					}
					if props&objtype.DbTableFlagList == 0 && len(fields) > 1 {
						return nil, packStepError(name, "%s column has multiple data values but is not marked as LIST", colName)
					}

					pd.P1(uint8(len(fields)))
					for _, field := range fields {
						for k, t := range types {
							val, err := lookupParamValue(t, field.values[k], lk)
							if err != nil {
								return nil, packStepError(name, "Data invalid in row, double-check the reference exists: data=%s,%s",
									field.column, strings.Join(field.values, ","))
							}
							if t == objtype.ScriptVarTypeString {
								pd.PJStr(val.(string))
							} else {
								pd.P4(uint32(int32(val.(int))))
							}
						}
					}
				}
				pd.P1(255) // end-of-column-tuple sentinel
			}

			// Opcode 4: always emitted when table is resolved.
			pd.P1(4)
			pd.P2(uint16(table.ID))
		}

		if len(name) > 0 {
			pd.P1(250)
			pd.PJStr(name)
		}
		pd.Next()
	}

	return pd, nil
}
