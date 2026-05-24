package entity

// Entity is the shared base for non-pathing world entities (Loc, Obj).
// The spawn pose (Level/X/Z/Width/Length) is fixed at construction; the
// lifecycle fields advance as the tick counter does.
type Entity struct {
	Level, X, Z, Width, Length int
	Lifecycle                  Lifecycle

	LifecycleTick     int // tick on which the next transition fires
	LastLifecycleTick int // tick on which the last transition fired
}

// NewEntity constructs the immutable portion of an Entity. Runtime fields
// (LifecycleTick, LastLifecycleTick) start at their zero values.
func NewEntity(level, x, z, width, length int, lc Lifecycle) Entity {
	return Entity{
		Level: level, X: x, Z: z, Width: width, Length: length,
		Lifecycle: lc,
	}
}

// NOTE: goscape deliberately has no tick-derived liveness predicate.
// TS Entity (Engine-TS/.../Entity.ts:32-34) determines liveness solely
// from the stored `isActive` flag (`isValid()` returns it); there is no
// `checkLifeCycle`/`updateLifeCycle` in TS. Earlier Go ports carried a
// `CheckLifecycle(tick)` (Respawn alive once LifecycleTick<tick, etc.) and
// an `UpdateLifecycle(tick)` helper; both were removed for L49 because they
// had no TS analog and no production callers — production gates on the
// stored IsActive flag (loc_turn.go / obj_turn.go) and inlines the
// transition check (`LifecycleTick != now`). Using the tick-derived form
// was the root cause of the M30 obj-follow bug, so the construct is
// intentionally absent to prevent reintroducing that drift.

// SetLifecycle schedules the next transition at `transitionTick`, recording
// `currentTick` as the tick on which the transition was scheduled.
func (e *Entity) SetLifecycle(transitionTick, currentTick int) {
	e.LifecycleTick = transitionTick
	e.LastLifecycleTick = currentTick
}
