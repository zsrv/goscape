package script

// handlers maps each Opcode to its implementation function.
// Populated in full in Task 5; see handlers.go for all 19 MVP entries.
var handlers = map[Opcode]func(*ScriptState) error{}
