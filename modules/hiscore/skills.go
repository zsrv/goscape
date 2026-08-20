// Package hiscore is the hiscores read API: a public, anonymous-safe
// JSON surface over the hiscore / hiscore_large central-database tables
// that modules/login populates on logout.
//
// This subsystem is a goscape extension. Engine-TS has no hiscore
// serving endpoint at any pinned revision, so it sits outside the TS
// fidelity ledger.
//
// Spec: docs/superpowers/specs/2026-08-19-hiscore-api-design.md
package hiscore

import (
	"sync"

	"github.com/zsrv/goscape/pkg/objtype"
)

// SkillOverall is the path/name selector for the aggregate board
// (hiscore type 0, stored in hiscore_large).
const SkillOverall = "overall"

// Skill is one selectable board.
type Skill struct {
	// Type is the hiscore `type` column: 0 for overall, stat+1 otherwise.
	Type int `json:"type"`
	// Name is the canonical lowercase selector.
	Name string `json:"name"`
}

var (
	skillsOnce sync.Once
	skillList  []Skill
	skillIndex map[string]Skill
)

// initSkills derives the board list from objtype rather than restating
// it, so the API and the write path in modules/login cannot drift.
func initSkills() {
	skillsOnce.Do(func() {
		skillIndex = map[string]Skill{SkillOverall: {Type: 0, Name: SkillOverall}}
		for stat := range objtype.PlayerStatCount {
			if !objtype.PlayerStatEnabled[stat] {
				continue
			}
			s := Skill{Type: stat + 1, Name: objtype.PlayerStatNames[stat]}
			skillList = append(skillList, s)
			skillIndex[s.Name] = s
		}
	})
}

// Skills returns the enabled per-stat boards in stat order. It excludes
// overall, which is not a stat.
func Skills() []Skill {
	initSkills()
	out := make([]Skill, len(skillList))
	copy(out, skillList)
	return out
}

// SkillByName resolves a selector, accepting "overall" in addition to
// the enabled stat names. Matching is exact and case-sensitive; callers
// normalize request input before calling.
func SkillByName(name string) (Skill, bool) {
	initSkills()
	s, ok := skillIndex[name]
	return s, ok
}

// TableForType returns the table holding rows of the given type. The
// result is a compile-time constant string, never request-derived, and
// is the only way a table name reaches a query.
func TableForType(typ int) string {
	if typ == 0 {
		return "hiscore_large"
	}
	return "hiscore"
}
