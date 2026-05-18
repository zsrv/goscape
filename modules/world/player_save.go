package world

// Save serializes p to a fresh SAV byte slice at version SavVersion.
// Inventories iterate over typeIds in ascending order (deviation
// NAI-PLAYERLOADING-D-INVS-SORTED-BY-TYPEID). Mirrors Player.save()
// (Player.ts:190-270).
func (p *Player) Save() []byte {
	// TODO(T10): implement
	return nil
}
