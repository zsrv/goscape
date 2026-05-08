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
// NPC's type + category).
//
// LastInt is the queued integer arg; processNpcQueue copies it into
// state.LastInt before executing the dispatched script (mirrors TS
// Npc.ts:554-555). The dispatched ai_queueN script reads it via the
// last_int opcode — it is NOT a positional script arg.
//
// NAI-123 DEVIATION-D1: TS NpcQueueRequest has separate args[] +
// lastInt fields. The args[] field is always [] at the one enqueue
// site (TS Npc.ts:242), so goscape collapses to a single LastInt
// field. Retire when a future content surface uses positional
// ai_queue args.
//
// Matches TS NpcQueueRequest at
// Engine-TS/src/engine/entity/NpcQueueRequest.ts:17.
type NpcQueueRequest struct {
	Trigger ServerTriggerType
	Delay   int
	LastInt int
}
