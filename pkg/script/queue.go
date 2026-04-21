package script

// PlayerQueueType mirrors TS PlayerQueueType. Determines when a
// queued script fires relative to the player's busy state.
type PlayerQueueType uint8

const (
	QueueNormal PlayerQueueType = iota
	QueueStrong
	QueueWeak
	QueueLong
	QueueEngine // reserved
	QueueSoft   // reserved
)

func (q PlayerQueueType) String() string {
	switch q {
	case QueueNormal:
		return "Normal"
	case QueueStrong:
		return "Strong"
	case QueueWeak:
		return "Weak"
	case QueueLong:
		return "Long"
	case QueueEngine:
		return "Engine"
	case QueueSoft:
		return "Soft"
	default:
		return "Unknown"
	}
}
