package revision

import "testing"

func TestExpectedRevisionIs225(t *testing.T) {
	if Expected != 225 {
		t.Fatalf("revision.Expected = %d, want 225", Expected)
	}
}
