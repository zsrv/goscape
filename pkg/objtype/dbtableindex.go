package objtype

// DbTableIndex is a build-time precomputed lookup over all INDEXED columns
// in DbTableType: packedKey → query → row IDs. Packed key is
// (tableID<<12) | (column<<4) | typeID, where typeID uses TS-build
// convention (0-based in the low nibble). Handlers pass the 1-based
// query form (matching bytecode's tupleIndex+1 encoding); DbTableIndex
// normalizes on Find by subtracting 1 from a non-zero low nibble.
//
// Int-valued and string-valued queries are split into parallel maps,
// consistent with the IntValues/StringValues split in DbRowType
// (deviation S7d-D1 applied consistently).
//
// Mirrors TS LostCityRS/Engine-TS/src/cache/config/DbTableIndex.ts.
type DbTableIndex struct {
	intRows map[int]map[int32][]int  // packedKey → (intQuery → rowIDs)
	strRows map[int]map[string][]int // packedKey → (strQuery → rowIDs)
}

// BuildDbTableIndex precomputes the lookup index over every INDEXED
// column across all tables. Called once at world bootstrap after
// LoadDbTableTypes / LoadDbRowTypes. Never fails; returns a non-nil
// *DbTableIndex even for empty configs.
func BuildDbTableIndex(tables *DbTableTypeConfigs, rows *DbRowTypeConfigs) *DbTableIndex {
	idx := &DbTableIndex{
		intRows: make(map[int]map[int32][]int),
		strRows: make(map[int]map[string][]int),
	}
	if tables == nil || rows == nil {
		return idx
	}

	for _, table := range tables.Configs {
		if table == nil {
			continue
		}
		// Skip tables with no INDEXED columns (matches TS early-return
		// at DbTableIndex.ts:24).
		anyIndexed := false
		for col := range table.Props {
			if table.Props[col]&DbTableFlagIndexed != 0 {
				anyIndexed = true
				break
			}
		}
		if !anyIndexed {
			continue
		}

		// Use table.ID — not the slice index — both for the RowsByTable
		// lookup and for the packed key. In production Configs[id]==
		// tableID, but driving off table.ID mirrors TS (DbTableIndex.ts:43
		// uses table.id) and avoids a silent coupling to slice position.
		for _, rowID := range rows.RowsByTable[table.ID] {
			if rowID < 0 || rowID >= len(rows.Configs) {
				continue
			}
			row := rows.Configs[rowID]
			if row == nil {
				continue
			}
			for col, types := range row.Types {
				if types == nil {
					continue
				}
				if col >= len(table.Props) || table.Props[col]&DbTableFlagIndexed == 0 {
					continue
				}
				if len(types) > 1 {
					idx.indexTuple(table.ID, col, types, row, rowID)
				} else {
					idx.indexList(table.ID, col, types[0], row, rowID)
				}
			}
		}
	}
	return idx
}

// indexTuple handles the multi-type (tuple) column path. packed key
// includes the 0-based typeID in the low nibble. fieldCount is the
// number of stored field-records per column; index lookup uses
// typeID + fieldID*len(types).
func (x *DbTableIndex) indexTuple(tableID, col int, types []ScriptVarType, row *DbRowType, rowID int) {
	// IntValues length is authoritative for fieldCount: decodeDbValues
	// (dbtabletype.go) allocates both IntValues and StringValues to
	// fieldCount*len(types), so a pure-string tuple column still has
	// an IntValues[col] slice of the right length (zero-filled).
	fieldCount := len(row.IntValues[col]) / len(types)
	for fieldID := range fieldCount {
		for typeID, t := range types {
			packed := ((tableID & 0xffff) << 12) | ((col & 0x7f) << 4) | (typeID & 0xf)
			valueIdx := typeID + fieldID*len(types)
			if t == ScriptVarTypeString {
				x.addStr(packed, row.StringValues[col][valueIdx], rowID)
			} else {
				x.addInt(packed, row.IntValues[col][valueIdx], rowID)
			}
		}
	}
}

// indexList handles the single-type-per-column path (including LIST
// columns with multiple stored values). packed key has tuple nibble = 0.
// Every stored value indexes to the same bucket.
func (x *DbTableIndex) indexList(tableID, col int, t ScriptVarType, row *DbRowType, rowID int) {
	packed := ((tableID & 0xffff) << 12) | ((col & 0x7f) << 4)
	if t == ScriptVarTypeString {
		for _, v := range row.StringValues[col] {
			x.addStr(packed, v, rowID)
		}
	} else {
		for _, v := range row.IntValues[col] {
			x.addInt(packed, v, rowID)
		}
	}
}

func (x *DbTableIndex) addInt(packed int, query int32, rowID int) {
	bucket, ok := x.intRows[packed]
	if !ok {
		bucket = make(map[int32][]int)
		x.intRows[packed] = bucket
	}
	bucket[query] = append(bucket[query], rowID)
}

func (x *DbTableIndex) addStr(packed int, query string, rowID int) {
	bucket, ok := x.strRows[packed]
	if !ok {
		bucket = make(map[string][]int)
		x.strRows[packed] = bucket
	}
	bucket[query] = append(bucket[query], rowID)
}

// FindInt returns row IDs whose indexed INT column contains query. packed
// uses the bytecode 1-based tuple-nibble convention; FindInt normalizes by
// subtracting 1 from a non-zero low nibble. Returns nil when the column
// is not INDEXED or no row matches. Returned slice is the map's
// underlying storage — callers must treat it as read-only.
func (x *DbTableIndex) FindInt(query int32, packed int) []int {
	key := packed
	if packed&0xf != 0 {
		key = packed - 1
	}
	bucket, ok := x.intRows[key]
	if !ok {
		return nil
	}
	return bucket[query]
}

// FindStr — symmetric to FindInt, over string-valued columns. Same
// aliasing contract: the returned slice is the map's underlying
// storage and must be treated as read-only.
func (x *DbTableIndex) FindStr(query string, packed int) []int {
	key := packed
	if packed&0xf != 0 {
		key = packed - 1
	}
	bucket, ok := x.strRows[key]
	if !ok {
		return nil
	}
	return bucket[query]
}
