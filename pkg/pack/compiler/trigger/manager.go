// pkg/pack/compiler/trigger/manager.go
package trigger

import "fmt"

// TriggerManager is a name → *TriggerType registry. Mirrors TS TriggerManager.ts.
type TriggerManager struct {
	nameToTrigger map[string]*TriggerType
}

// NewTriggerManager returns an empty TriggerManager.
func NewTriggerManager() *TriggerManager {
	return &TriggerManager{nameToTrigger: map[string]*TriggerType{}}
}

// Register inserts t under name. Errors on duplicate name. Mirrors TS L15-19.
func (m *TriggerManager) Register(name string, t *TriggerType) error {
	if _, ok := m.nameToTrigger[name]; ok {
		return fmt.Errorf("trigger %q is already registered", name)
	}
	m.nameToTrigger[name] = t
	return nil
}

// RegisterTrigger registers t under t.Identifier. Mirrors TS L24-26.
func (m *TriggerManager) RegisterTrigger(t *TriggerType) error {
	return m.Register(t.Identifier, t)
}

// RegisterAll registers every trigger via RegisterTrigger. Mirrors TS L31-35.
func (m *TriggerManager) RegisterAll(triggers []*TriggerType) error {
	for _, t := range triggers {
		if err := m.RegisterTrigger(t); err != nil {
			return err
		}
	}
	return nil
}

// Find returns the named trigger or an error. Mirrors TS L40-46.
func (m *TriggerManager) Find(name string) (*TriggerType, error) {
	if t, ok := m.nameToTrigger[name]; ok {
		return t, nil
	}
	return nil, fmt.Errorf("unable to find trigger %q", name)
}

// FindOrNil returns the named trigger or nil. Mirrors TS L53-55.
func (m *TriggerManager) FindOrNil(name string) *TriggerType {
	return m.nameToTrigger[name]
}
