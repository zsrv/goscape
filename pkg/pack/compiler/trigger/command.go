// pkg/pack/compiler/trigger/command.go
package trigger

// CommandTrigger is the sentinel trigger for `command` scripts.
// Mirrors TS CommandTrigger.ts. ScriptRegistration compares against this
// pointer to gate the `*`-suffix check and other command-only behaviour.
//
// The fields of the pointed-to TriggerType must not be modified — treat
// this as a read-only singleton. TS declares the equivalent as
// `export const` with an interface type, which prevents mutation; Go has
// no const-pointer equivalent, so the contract is documentary.
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
