package codegen

// SwitchCase is one case of a SwitchTable. Keys are typed at TS as `any[]`;
// goscape mirrors that with []any. The label points at the block that the
// switch jumps to on key match. Mirrors TS SwitchCase (SwitchTable.ts).
type SwitchCase struct {
	Label *Label
	Keys  []any
}

// SwitchTable carries all SwitchCases for one switch statement, plus a
// unique ID within the enclosing RuneScript. Mirrors TS SwitchTable.
type SwitchTable struct {
	ID    int
	cases []SwitchCase
}

// NewSwitchTable returns a fresh SwitchTable with the given ID.
func NewSwitchTable(id int) *SwitchTable {
	return &SwitchTable{ID: id}
}

// Cases returns an immutable view of the table's cases (slice header is
// copied; SwitchCase.Keys element content is shared). Mirrors TS
// SwitchTable.cases getter (TS uses Readonly<>; goscape uses copy-on-read).
func (s *SwitchTable) Cases() []SwitchCase {
	out := make([]SwitchCase, len(s.cases))
	copy(out, s.cases)
	return out
}

// AddCase appends a case to the table.
func (s *SwitchTable) AddCase(c SwitchCase) {
	s.cases = append(s.cases, c)
}
