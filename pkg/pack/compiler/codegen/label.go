package codegen

import "fmt"

// Label is a unique name assigned to a Block used by branching instructions.
// Mirrors TS Label (Label.ts).
type Label struct {
	Name string
}

// LabelGenerator produces unique Label names by appending an incrementing
// counter per name-prefix. Reset clears the counters. Mirrors TS
// LabelGenerator (LabelGenerator.ts).
type LabelGenerator struct {
	names map[string]int
}

func NewLabelGenerator() *LabelGenerator {
	return &LabelGenerator{names: map[string]int{}}
}

// Generate returns a Label whose Name is `<prefix>_<n>` where n is the
// count of prior calls with the same prefix.
func (g *LabelGenerator) Generate(prefix string) *Label {
	n := g.names[prefix]
	g.names[prefix] = n + 1
	return &Label{Name: fmt.Sprintf("%s_%d", prefix, n)}
}

// Reset clears the per-prefix counters. Called per-script in CodeGenerator
// visitScript.
func (g *LabelGenerator) Reset() {
	g.names = map[string]int{}
}
