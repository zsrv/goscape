package world

import "testing"

func TestComponentConstantsAreDistinct(t *testing.T) {
	all := []string{compWorld, compServer, compNet, compTick, compScript, compFriends, compLogin, compContent, compReport}
	seen := map[string]bool{}
	for _, c := range all {
		if c == "" {
			t.Error("empty component constant")
		}
		if seen[c] {
			t.Errorf("duplicate component %q", c)
		}
		seen[c] = true
	}
	if compWorld != "world" || compNet != "world.net" {
		t.Errorf("unexpected component naming: %q %q", compWorld, compNet)
	}
}
