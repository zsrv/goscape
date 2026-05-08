package inventory

// Transaction is the result of an Add or Remove operation.
type Transaction struct {
	Requested int    // units the caller asked for
	Completed int    // units actually added/removed
	Items     []Item // items moved (used by Transfer)

	// Added lists the (slot, item) pairs actually written by Add.
	// Mirrors TS InventoryTransaction.added (Inventory.ts:194 etc.).
	// Populated on every Add path (stack and non-stack); empty for
	// dry-run, no-op, or Remove.
	Added []SlotEntry
}

// SlotEntry pairs a slot index with the Item value written there.
// Used by Transaction.Added to record per-slot writes during Add.
type SlotEntry struct {
	Slot int
	Item Item
}
