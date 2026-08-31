// pkg/pack/compiler/trigger/server_trigger_type_test.go
package trigger

import "testing"

func TestServerTriggerTypeAll_Count(t *testing.T) {
	const want = 156
	if got := len(ServerTriggerTypeAll); got != want {
		t.Errorf("ServerTriggerTypeAll length = %d, want %d", got, want)
	}
}

func TestServerTriggerProc(t *testing.T) {
	if ServerTriggerProc.Identifier != "proc" {
		t.Errorf("ServerTriggerProc.Identifier = %q, want \"proc\"", ServerTriggerProc.Identifier)
	}
	if ServerTriggerProc.ID != 0 {
		t.Errorf("ServerTriggerProc.ID = %d, want 0", ServerTriggerProc.ID)
	}
}

func TestServerTriggerLabel(t *testing.T) {
	if ServerTriggerLabel.Identifier != "label" {
		t.Errorf("ServerTriggerLabel.Identifier = %q, want \"label\"", ServerTriggerLabel.Identifier)
	}
}

func TestServerTriggerTypeAll_AllRegisterable(t *testing.T) {
	for i, tr := range ServerTriggerTypeAll {
		if tr.Identifier == "" {
			t.Errorf("ServerTriggerTypeAll[%d].Identifier empty", i)
		}
	}
}

// Cross-referenced IDs (also pinned in runescript/server_pointer_checker.go).
func TestServerTriggerCrossRefIDs(t *testing.T) {
	cases := []struct {
		tr   *TriggerType
		want int
		name string
	}{
		{ServerTriggerIfButton, 147, "ServerTriggerIfButton"},
		{ServerTriggerInvButton1, 149, "ServerTriggerInvButton1"},
		{ServerTriggerInvButton2, 150, "ServerTriggerInvButton2"},
		{ServerTriggerInvButton3, 151, "ServerTriggerInvButton3"},
		{ServerTriggerInvButton4, 152, "ServerTriggerInvButton4"},
		{ServerTriggerInvButton5, 153, "ServerTriggerInvButton5"},
		{ServerTriggerInvButtonD, 154, "ServerTriggerInvButtonD"},
	}
	for _, c := range cases {
		if c.tr.ID != c.want {
			t.Errorf("%s.ID = %d, want %d", c.name, c.tr.ID, c.want)
		}
	}
}

// TestServerTriggerLogoutHasNoReturn pins that the logout trigger no longer
// declares a boolean return. @lostcityrs/runescript 0.9.7 dropped it, and
// Content 687b6a1a1 ("Compatible with compiler/packer changes") rewrote
// scripts/login_logout/logout.rs2 from `[logout,_]()(boolean)` + `return(true)`
// to a bare `[logout,_]`. The old content even carried a
// "// TODO: Change compiler to remove return value" comment.
//
// The engine never read the value: World.ts:780-790 @1d25566c inits and
// executes the logout script and discards the result at both the old and new
// Engine-TS pins.
func TestServerTriggerLogoutHasNoReturn(t *testing.T) {
	if ServerTriggerLogout.AllowReturns {
		t.Error("ServerTriggerLogout.AllowReturns: got true, want false")
	}
	if ServerTriggerLogout.Returns != nil {
		t.Errorf("ServerTriggerLogout.Returns: got %v, want nil", ServerTriggerLogout.Returns)
	}
}
