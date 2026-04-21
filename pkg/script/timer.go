package script

// PlayerTimerType mirrors TS PlayerTimerType. Normal timers wait for
// idle before firing; Soft timers fire regardless of busy state.
type PlayerTimerType uint8

const (
	TimerNormal PlayerTimerType = iota
	TimerSoft
)

func (t PlayerTimerType) String() string {
	switch t {
	case TimerNormal:
		return "Normal"
	case TimerSoft:
		return "Soft"
	default:
		return "Unknown"
	}
}
