package world

// gameHandlers is indexed by decrypted game opcode. Nil means no handler
// registered for that opcode; handleGame() silently discards such packets
// (they still must be in Ops[] to be accepted at all).
var gameHandlers [256]func(*client, []byte) error
