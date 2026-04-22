package script

// LookupKeyForType returns the specific-script lookup key for a
// (trigger, typeID) pair. Layout: bits 0-7 = trigger, bits 8-9 =
// selector (0b10 = type-specific), bits 10+ = typeID.
func LookupKeyForType(trigger ServerTriggerType, typeID int) uint32 {
	return uint32(trigger) | (0x2 << 8) | (uint32(typeID) << 10)
}

// LookupKeyForCategory returns the category-fallback lookup key.
// Bits 8-9 = 0b01 (category selector).
func LookupKeyForCategory(trigger ServerTriggerType, categoryID int) uint32 {
	return uint32(trigger) | (0x1 << 8) | (uint32(categoryID) << 10)
}

// LookupKeyForGlobal returns the global-fallback lookup key. Bits 8-9
// = 0b00 (no type/category).
func LookupKeyForGlobal(trigger ServerTriggerType) uint32 {
	return uint32(trigger)
}
