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

// NpcQueueRequest is an NPC-side enqueue entry. Unlike
// PlayerQueueRequest, it has no queue-type distinction — TS's NPC
// queue has no strong/weak/long variants. The Trigger is one of
// TriggerAiQueue1..TriggerAiQueue20 and identifies which script runs
// at fire time (resolved via scriptProvider.GetByTrigger on the
// NPC's type + category). Matches TS NpcQueueRequest at
// Engine-TS/src/engine/entity/NpcQueueRequest.ts.
type NpcQueueRequest struct {
	Trigger ServerTriggerType
	Delay   int
	IntArg  int
}
