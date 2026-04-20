package script

// Execution is the state of a running script. Only Running, Finished, and
// Aborted are exercised in S1; the remainder are defined so later sub-specs
// (S4/S5) don't churn this enum.
type Execution int

const (
	Running        Execution = iota // hot loop continues
	Finished                        // OpReturn with empty frame stack
	Aborted                         // runtime error (unknown opcode, opcount cap, handler error)
	Suspended                       // player-level pause; resumed by tick loop (S4)
	CountDialog                     // waiting for client count-dialog input (S5)
	PauseButton                     // waiting for button click (S5)
	NpcSuspended                    // NPC variant of Suspended (S6)
	WorldSuspended                  // world-scheduled wakeup (S4)
)
