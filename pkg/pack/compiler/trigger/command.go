// pkg/pack/compiler/trigger/command.go
package trigger

// CommandTrigger is the sentinel trigger for `command` scripts.
// Mirrors TS CommandTrigger.ts. ScriptRegistration compares against this
// pointer to gate the `*`-suffix check and other command-only behaviour.
var CommandTrigger = &TriggerType{
	ID:              -1,
	Identifier:      "command",
	SubjectMode:     ModeName,
	AllowParameters: true,
	Parameters:      nil,
	AllowReturns:    true,
	Returns:         nil,
	Pointers:        nil,
}
