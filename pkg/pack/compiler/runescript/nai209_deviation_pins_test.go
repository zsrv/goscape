// pkg/pack/compiler/runescript/nai209_deviation_pins_test.go
package runescript_test

import (
	"os"
	"strings"
	"testing"
)

// nai209DeviationTags is the canonical inventory of NAI-209's deviation tags.
// Each tag must appear in at least one production-source doc comment so a
// future reader can grep from the test to the rationale.
var nai209DeviationTags = []struct {
	tag       string
	rationale string // human-readable hint for failure messages
}{
	{"NAI-209-D-BYTEPACKET-DEFER", "BytePacket deferred to NAI-210"},
	{"NAI-209-D-SYMMAPPER-DIAG-CTOR", "SymbolMapper takes diagnostics in ctor"},
	{"NAI-209-D-PUSHLONG-PANIC", "WritePushConstantLong panics on TS throw parity"},
	{"NAI-209-D-MAPZONE-COORD-PARSE-PANIC", "Atoi failure panics, not silent NaN"},
	{"NAI-209-D-OPCODE-WRITER-INTERFACE", "TS abstract class -> Go interface"},
	{"NAI-209-D-BINARYOUTPUT-INTERFACE", "TS abstract outputScript -> Go interface"},
	{"NAI-209-D-LINENUMBER-ORDER-SLICE", "Map iteration randomized -> parallel slice"},
	{"NAI-209-D-DEBUGPROC-TRIGGER-STRING-CHECK", "DEBUGPROC trigger singleton not yet ported"},
	{"NAI-209-D-LONGBRANCH-OBJBRANCH-PANIC", "LongBranch/ObjBranch opcodes panic (TS-faithful unsupported)"},
	{"NAI-209-D-LONGMATH-PANIC", "LongMath opcodes panic (TS-faithful unsupported)"},
	{"NAI-209-D-TYPEMARKER-CATEGORY-DISCRIMINATOR", "TS per-subject vs goscape per-trigger semantic"},
}

// productionFiles lists every NAI-209 production source. The pin test
// reads each file and asserts each tag appears in at least one file's
// content. Mirrors [[pin_test_self_trigger_production_doc]].
var productionFiles = []string{
	"binary_context.go",
	"binary_writer.go",
	"symbol_mapper.go",
	"../writer/base_writer.go",
	"../writer/base_context.go",
	"../writer/helpers.go",
}

func TestNAI209_DeviationTags_PinnedToProductionDocs(t *testing.T) {
	combined := readAllNAI209(t, productionFiles)
	for _, c := range nai209DeviationTags {
		if !strings.Contains(combined, c.tag) {
			t.Errorf("deviation tag %s missing from production docs (%s)", c.tag, c.rationale)
		}
	}
}

func readAllNAI209(t *testing.T, files []string) string {
	t.Helper()
	var sb strings.Builder
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		sb.Write(b)
		sb.WriteByte('\n')
	}
	return sb.String()
}
