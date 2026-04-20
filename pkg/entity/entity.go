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

// UpdateLifecycle reports whether the given tick exactly matches the
// scheduled transition AND this entity is not a static.
func (e *Entity) UpdateLifecycle(tick int) bool {
	return e.LifecycleTick == tick && e.Lifecycle != LifecycleForever
}

// CheckLifecycle reports whether this entity is currently in the world at
// `tick`. Statics are always alive; Respawn entities are alive once their
// respawn tick has passed; Despawn entities are alive until their despawn
// tick has passed. Equal-to-transition-tick counts as the "dead" half.
func (e *Entity) CheckLifecycle(tick int) bool {
	switch e.Lifecycle {
	case LifecycleForever:
		return true
	case LifecycleRespawn:
		return e.LifecycleTick < tick
	case LifecycleDespawn:
		return e.LifecycleTick > tick
	default:
		return false
	}
}

// SetLifecycle schedules the next transition at `transitionTick`, recording
// `currentTick` as the tick on which the transition was scheduled.
func (e *Entity) SetLifecycle(transitionTick, currentTick int) {
	e.LifecycleTick = transitionTick
	e.LastLifecycleTick = currentTick
}
