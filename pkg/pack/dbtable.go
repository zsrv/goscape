package pack

import (
	"strings"

	"github.com/zsrv/goscape/pkg/objtype"
)

// parseDbTableConfig is the per-key=value parser for .dbtable config
// blocks. Both 'column' and 'default' return their raw string values;
// CSV parsing is deferred to packDbTableConfigs because column resolution
// depends on the column-list state built during the first pass.
//
// NAI-195-D-DEADBRANCH-OMITTED: TS DbTableConfig.ts:29-31 declares
// empty stringKeys/numberKeys/booleanKeys arrays — all three branches
// are dead and are omitted here.
//
// TS source: tools/pack/config/DbTableConfig.ts:28-76.
func parseDbTableConfig(key, value string) (ConfigValue, bool, error) {
	switch key {
	case "column", "default":
		return value, true, nil
	}
	return nil, false, nil
}

// dbTableColumn holds the parsed state for a single .dbtable column
// declaration: the column name, its zero or more ScriptVarTypes (the
// tuple members), and any property tokens (INDEXED, REQUIRED, …).
type dbTableColumn struct {
	name       string
	types      []objtype.ScriptVarType
	properties []string
}

// packDbTableConfigs walks every id ∈ [0, pf.Max), two-pass-walks each
// per-id config block (columns then defaults), and emits opcodes
// 1/251/252 (gated on columns.length > 0) plus the 250-trailer per id.
//
// Server-only — TS allocates a client PackedData but never writes to it
// between Next() calls; goscape omits the dead client buffer entirely.
//
// Opcode layout:
//
//	1   → column-count | per-column(flags,type-count,types[,field-count,defaults]) | 255
//	251 → column-count | per-column-name PJStr
//	252 → column-count | per-column props-byte
//	250 → debugname PJStr  (when len(name) > 0)
//
// TS source: tools/pack/config/DbTableConfig.ts:78-224.
func packDbTableConfigs(configs map[string][]ConfigLine, pf *PackFile, lk *paramLookups) (*PackedData, error) {
	pd := NewPackedData(pf.Max)

	for id := range pf.Max {
		name := pf.GetByID(id)
		if cfg, ok := configs[name]; ok {
			var columns []dbTableColumn
			defaults := map[int][]string{}

			// Pass 1: collect columns in declaration order.
			for _, line := range cfg {
				if line.Key != "column" {
					continue
				}
				parts := parseCsv(line.Value.(string))
				if len(parts) == 0 {
					continue
				}
				col := dbTableColumn{name: parts[0]}
				for _, part := range parts[1:] {
					if part == "" {
						continue
					}
					if part == strings.ToUpper(part) {
						col.properties = append(col.properties, part)
					} else {
						t, ok := objtype.ScriptVarTypeFromName(part)
						if !ok {
							return nil, packStepError(name, "unknown column type %q", part)
						}
						col.types = append(col.types, t)
					}
				}
				hasIndexed := false
				hasRequired := false
				for _, p := range col.properties {
					switch p {
					case "INDEXED":
						hasIndexed = true
					case "REQUIRED":
						hasRequired = true
					}
				}
				if hasIndexed && !hasRequired {
					return nil, packStepError(name, "INDEXED columns must be marked REQUIRED as well")
				}
				columns = append(columns, col)
			}

			// Pass 2: collect per-column defaults.
			for _, line := range cfg {
				if line.Key != "default" {
					continue
				}
				parts := parseCsv(line.Value.(string))
				if len(parts) == 0 {
					continue
				}
				colName := parts[0]
				values := parts[1:]
				colIdx := -1
				for i, c := range columns {
					if c.name == colName {
						colIdx = i
						break
					}
				}
				if colIdx == -1 {
					return nil, packStepError(name, "unknown default column %q", colName)
				}
				for _, p := range columns[colIdx].properties {
					if p == "REQUIRED" {
						return nil, packStepError(name, "%s cannot have a default value because it is marked REQUIRED", colName)
					}
				}
				defaults[colIdx] = values
			}

			if len(columns) > 0 {
				// Opcode 1: column-type block.
				pd.P1(1)
				pd.P1(uint8(len(columns)))
				for i, col := range columns {
					flags := uint8(i)
					if defaults[i] != nil {
						flags |= 0x80
					}
					pd.P1(flags)
					pd.P1(uint8(len(col.types)))
					for _, t := range col.types {
						pd.P1(uint8(t))
					}
					if flags&0x80 != 0 {
						pd.P1(1) // field-count (always 1 per TS line 163)
						for j, t := range col.types {
							resolved, err := lookupParamValue(t, defaults[i][j], lk)
							if err != nil {
								return nil, packStepError(name, "default[%d]: %v", j, err)
							}
							if t == objtype.ScriptVarTypeString {
								pd.PJStr(resolved.(string))
							} else {
								pd.P4(uint32(int32(resolved.(int))))
							}
						}
					}
				}
				pd.P1(255) // end-of-column-tuple sentinel

				// Opcode 251: column-name list.
				pd.P1(251)
				pd.P1(uint8(len(columns)))
				for _, col := range columns {
					pd.PJStr(col.name)
				}

				// Opcode 252: per-column property-bits.
				pd.P1(252)
				pd.P1(uint8(len(columns)))
				for _, col := range columns {
					var props uint8
					for _, p := range col.properties {
						switch p {
						case "INDEXED":
							props |= objtype.DbTableFlagIndexed
						case "REQUIRED":
							props |= objtype.DbTableFlagRequired
						case "LIST":
							props |= objtype.DbTableFlagList
						case "CLIENTSIDE":
							props |= objtype.DbTableFlagClientside
						}
					}
					pd.P1(props)
				}
			}
		}

		if len(name) > 0 {
			pd.P1(250)
			pd.PJStr(name)
		}
		pd.Next()
	}

	return pd, nil
}
