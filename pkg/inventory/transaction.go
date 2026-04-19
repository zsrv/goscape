package inventory

// Transaction is the result of an Add or Remove operation.
type Transaction struct {
	Requested int    // units the caller asked for
	Completed int    // units actually added/removed
	Items     []Item // items moved (used by Transfer)
}
