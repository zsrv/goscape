package worldmap

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/zsrv/goscape/pkg/coordgrid"
)

func TestProcessCsv_SingleZoneExpands8x8(t *testing.T) {
	t.Parallel()
	got := processCsv([]string{"0_50_50_0_0"}, "test", slog.Default())
	if want := 64; len(got) != want {
		t.Fatalf("len = %d, want %d", len(got), want)
	}
	for _, c := range []struct{ x, z int }{
		{50 << 6, 50 << 6},
		{(50 << 6) + 7, (50 << 6) + 7},
	} {
		if _, ok := got[coordgrid.PackCoord(0, c.x, c.z)]; !ok {
			t.Errorf("missing coord (0, %d, %d)", c.x, c.z)
		}
	}
}

func TestProcessCsv_RangeExpandsRectangle(t *testing.T) {
	t.Parallel()
	got := processCsv([]string{"0_10_10_0_0,0_10_10_7_7"}, "test", slog.Default())
	if want := 64; len(got) != want {
		t.Fatalf("len = %d, want %d", len(got), want)
	}
}

func TestProcessCsv_CommentAndEmpty(t *testing.T) {
	t.Parallel()
	got := processCsv([]string{"// comment", "", "0_0_0_0_0"}, "test", slog.Default())
	if want := 64; len(got) != want {
		t.Fatalf("len = %d, want %d", len(got), want)
	}
}

func TestProcessCsv_AlignmentWarning(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	lg := slog.New(slog.NewTextHandler(&buf, nil))
	_ = processCsv([]string{"0_5_5_1_0,0_5_5_7_7"}, "multiway", lg)
	if !strings.Contains(buf.String(), "not aligned") {
		t.Errorf("expected alignment warning, got %q", buf.String())
	}
	if !strings.Contains(buf.String(), "multiway") {
		t.Errorf("expected name in warning, got %q", buf.String())
	}
}

func TestProcessCsv_OverlapWarning(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	lg := slog.New(slog.NewTextHandler(&buf, nil))
	_ = processCsv([]string{"0_0_0_0_0", "0_0_0_0_0,0_0_0_7_7"}, "ignore", lg)
	if !strings.Contains(buf.String(), "Overlapping") {
		t.Errorf("expected overlap warning, got %q", buf.String())
	}
}

func TestParseLabels_FiltersAndParses(t *testing.T) {
	t.Parallel()
	src := strings.Join([]string{
		"// comment",
		"=Lumbridge,3222,3218,0",
		"not_a_label_line",
		"=Falador,2965,3380,1",
		"",
	}, "\n")
	got := parseLabels(src)
	if want := 2; len(got) != want {
		t.Fatalf("len = %d, want %d", len(got), want)
	}
	if got[0].Text != "Lumbridge" || got[0].X != 3222 || got[0].Z != 3218 || got[0].Type != 0 {
		t.Errorf("got[0] = %+v", got[0])
	}
	if got[1].Text != "Falador" || got[1].X != 2965 || got[1].Z != 3380 || got[1].Type != 1 {
		t.Errorf("got[1] = %+v", got[1])
	}
}
