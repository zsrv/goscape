// pkg/pack/compiler/trigger/manager_test.go
package trigger

import (
	"strings"
	"testing"
)

func newTestTrigger(name string) *TriggerType {
	return &TriggerType{
		ID:              -1,
		Identifier:      name,
		SubjectMode:     ModeNone,
		AllowParameters: false,
		AllowReturns:    false,
	}
}

func TestTriggerManager_RegisterAndFind(t *testing.T) {
	m := NewTriggerManager()
	tg := newTestTrigger("proc")
	if err := m.Register("proc", tg); err != nil {
		t.Fatalf("Register proc: %v", err)
	}
	got, err := m.Find("proc")
	if err != nil {
		t.Fatalf("Find proc: %v", err)
	}
	if got != tg {
		t.Fatalf("Find proc returned different pointer")
	}
}

func TestTriggerManager_RegisterTrigger_UsesIdentifier(t *testing.T) {
	m := NewTriggerManager()
	tg := newTestTrigger("label")
	if err := m.RegisterTrigger(tg); err != nil {
		t.Fatalf("RegisterTrigger: %v", err)
	}
	if got, _ := m.Find("label"); got != tg {
		t.Fatalf("RegisterTrigger did not register under .Identifier")
	}
}

func TestTriggerManager_DoubleRegisterErrors(t *testing.T) {
	m := NewTriggerManager()
	_ = m.Register("proc", newTestTrigger("proc"))
	if err := m.Register("proc", newTestTrigger("proc")); err == nil {
		t.Fatal("double Register: nil err, want collision")
	}
}

func TestTriggerManager_FindOrNil_Miss(t *testing.T) {
	m := NewTriggerManager()
	if got := m.FindOrNil("nope"); got != nil {
		t.Fatalf("FindOrNil miss = %v, want nil", got)
	}
}

func TestTriggerManager_FindErrorMessageContainsName(t *testing.T) {
	m := NewTriggerManager()
	_, err := m.Find("nope")
	if err == nil {
		t.Fatal("Find miss: nil err")
	}
	if !strings.Contains(err.Error(), "nope") {
		t.Fatalf("err = %v; want to mention 'nope'", err)
	}
}

func TestTriggerManager_RegisterAll(t *testing.T) {
	m := NewTriggerManager()
	triggers := []*TriggerType{
		newTestTrigger("proc"),
		newTestTrigger("label"),
	}
	if err := m.RegisterAll(triggers); err != nil {
		t.Fatalf("RegisterAll: %v", err)
	}
	if _, err := m.Find("proc"); err != nil {
		t.Fatalf("Find proc after RegisterAll: %v", err)
	}
	if _, err := m.Find("label"); err != nil {
		t.Fatalf("Find label after RegisterAll: %v", err)
	}
}
